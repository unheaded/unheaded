//! Trace Storage Backend
//!
//! In-memory and file-based storage for trace data with query capabilities.
//! Optimized for recent trace access patterns.

use std::collections::{BTreeMap, HashMap, VecDeque};
use std::fs::{self, File, OpenOptions};
use std::io::{BufRead, BufReader, BufWriter, Write};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use parking_lot::RwLock;
use serde::{Deserialize, Serialize};
use tracing::{debug, info};

use super::{Span, TraceContext, TraceId, TraceSummary};

/// Configuration for the trace store
#[derive(Debug, Clone)]
pub struct TraceStoreConfig {
    /// Maximum traces to keep in memory
    pub max_memory_traces: usize,
    /// Maximum trace age in memory before eviction
    pub memory_retention: Duration,
    /// Enable file-based persistence
    pub persistence_enabled: bool,
    /// Directory for persisted traces
    pub storage_path: PathBuf,
    /// Maximum file size before rotation (bytes)
    pub max_file_size: u64,
    /// Maximum number of trace files to keep
    pub max_files: usize,
    /// Flush interval for writing to disk
    pub flush_interval: Duration,
}

impl Default for TraceStoreConfig {
    fn default() -> Self {
        Self {
            max_memory_traces: 10000,
            memory_retention: Duration::from_secs(300), // 5 minutes
            persistence_enabled: false,
            storage_path: PathBuf::from("/var/lib/unheaded/traces"),
            max_file_size: 100 * 1024 * 1024, // 100MB
            max_files: 10,
            flush_interval: Duration::from_secs(5),
        }
    }
}

/// Query parameters for trace search
#[derive(Debug, Clone, Default)]
pub struct TraceQuery {
    /// Filter by service name
    pub service: Option<String>,
    /// Filter by minimum duration (nanoseconds)
    pub min_duration_ns: Option<u64>,
    /// Filter by maximum duration (nanoseconds)
    pub max_duration_ns: Option<u64>,
    /// Filter by status
    pub status: Option<String>,
    /// Filter by start time (Unix timestamp nanoseconds)
    pub start_time_ns: Option<u64>,
    /// Filter by end time
    pub end_time_ns: Option<u64>,
    /// Filter by operation name (contains)
    pub operation: Option<String>,
    /// Maximum results to return
    pub limit: usize,
    /// Offset for pagination
    pub offset: usize,
    /// Sort order (true = newest first)
    pub newest_first: bool,
}

impl TraceQuery {
    pub fn new() -> Self {
        Self {
            limit: 100,
            newest_first: true,
            ..Default::default()
        }
    }

    pub fn service(mut self, service: impl Into<String>) -> Self {
        self.service = Some(service.into());
        self
    }

    pub fn min_duration(mut self, duration_ns: u64) -> Self {
        self.min_duration_ns = Some(duration_ns);
        self
    }

    pub fn max_duration(mut self, duration_ns: u64) -> Self {
        self.max_duration_ns = Some(duration_ns);
        self
    }

    pub fn status(mut self, status: impl Into<String>) -> Self {
        self.status = Some(status.into());
        self
    }

    pub fn time_range(mut self, start_ns: u64, end_ns: u64) -> Self {
        self.start_time_ns = Some(start_ns);
        self.end_time_ns = Some(end_ns);
        self
    }

    pub fn limit(mut self, limit: usize) -> Self {
        self.limit = limit;
        self
    }

    pub fn offset(mut self, offset: usize) -> Self {
        self.offset = offset;
        self
    }
}

/// Result of a trace query
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TraceQueryResult {
    /// Matching traces
    pub traces: Vec<TraceSummary>,
    /// Total matching count (before limit/offset)
    pub total_count: usize,
    /// Query execution time (milliseconds)
    pub query_time_ms: u64,
    /// Whether there are more results
    pub has_more: bool,
}

/// Stored trace record (for persistence)
#[derive(Debug, Clone, Serialize, Deserialize)]
struct StoredTrace {
    /// Trace summary
    pub summary: TraceSummary,
    /// All spans in the trace
    pub spans: Vec<Span>,
    /// Storage timestamp
    pub stored_at: u64,
}

/// In-memory trace index by time
struct TraceIndex {
    /// Traces indexed by start time (for time-range queries)
    by_time: BTreeMap<u64, TraceId>,
    /// Traces indexed by service
    by_service: HashMap<String, Vec<TraceId>>,
}

impl TraceIndex {
    fn new() -> Self {
        Self {
            by_time: BTreeMap::new(),
            by_service: HashMap::new(),
        }
    }

    fn insert(&mut self, trace_id: TraceId, summary: &TraceSummary) {
        self.by_time.insert(summary.start_time_ns, trace_id);
        for service in &summary.services {
            self.by_service
                .entry(service.clone())
                .or_default()
                .push(trace_id);
        }
    }

    fn remove(&mut self, summary: &TraceSummary) {
        self.by_time.remove(&summary.start_time_ns);
        for service in &summary.services {
            if let Some(traces) = self.by_service.get_mut(service) {
                traces.retain(|id| id.to_hex() != summary.trace_id);
            }
        }
    }
}

/// Store statistics
#[derive(Debug, Default)]
pub struct StoreStats {
    /// Traces stored
    pub traces_stored: AtomicU64,
    /// Traces evicted
    pub traces_evicted: AtomicU64,
    /// Queries executed
    pub queries_executed: AtomicU64,
    /// Bytes written to disk
    pub bytes_written: AtomicU64,
    /// File rotations
    pub file_rotations: AtomicU64,
}

/// Trace store
pub struct TraceStore {
    /// Configuration
    config: TraceStoreConfig,
    /// In-memory traces
    traces: RwLock<HashMap<String, TraceSummary>>,
    /// Full trace data (for recent traces)
    trace_data: RwLock<HashMap<String, Vec<Span>>>,
    /// LRU order for eviction
    lru: RwLock<VecDeque<String>>,
    /// Search index
    index: RwLock<TraceIndex>,
    /// Statistics
    stats: Arc<StoreStats>,
    /// Write buffer for persistence
    write_buffer: RwLock<Vec<StoredTrace>>,
    /// Current trace file
    current_file: RwLock<Option<PathBuf>>,
    /// Last flush time
    last_flush: RwLock<Instant>,
}

impl TraceStore {
    /// Create a new trace store
    pub fn new(config: TraceStoreConfig) -> Result<Self> {
        if config.persistence_enabled {
            fs::create_dir_all(&config.storage_path)
                .context("Failed to create trace storage directory")?;
        }

        Ok(Self {
            config,
            traces: RwLock::new(HashMap::new()),
            trace_data: RwLock::new(HashMap::new()),
            lru: RwLock::new(VecDeque::new()),
            index: RwLock::new(TraceIndex::new()),
            stats: Arc::new(StoreStats::default()),
            write_buffer: RwLock::new(Vec::new()),
            current_file: RwLock::new(None),
            last_flush: RwLock::new(Instant::now()),
        })
    }

    /// Get store statistics
    pub fn stats(&self) -> &Arc<StoreStats> {
        &self.stats
    }

    /// Get trace count
    pub fn trace_count(&self) -> usize {
        self.traces.read().len()
    }

    /// Store a completed trace
    pub fn store(&self, context: &TraceContext) {
        let summary = TraceSummary::from_context(context);
        let trace_id = summary.trace_id.clone();

        // Check capacity and evict if needed
        self.ensure_capacity();

        // Store summary
        {
            let mut traces = self.traces.write();
            traces.insert(trace_id.clone(), summary.clone());
        }

        // Store full span data
        {
            let mut trace_data = self.trace_data.write();
            trace_data.insert(trace_id.clone(), context.spans.clone());
        }

        // Update LRU
        {
            let mut lru = self.lru.write();
            lru.push_back(trace_id.clone());
        }

        // Update index
        {
            let mut index = self.index.write();
            let trace_id_parsed = TraceId::from_hex(&trace_id).unwrap_or_default();
            index.insert(trace_id_parsed, &summary);
        }

        self.stats.traces_stored.fetch_add(1, Ordering::Relaxed);

        // Add to write buffer for persistence
        if self.config.persistence_enabled {
            let stored = StoredTrace {
                summary,
                spans: context.spans.clone(),
                stored_at: SystemTime::now()
                    .duration_since(UNIX_EPOCH)
                    .unwrap_or_default()
                    .as_nanos() as u64,
            };

            self.write_buffer.write().push(stored);

            // Check if we should flush
            if self.last_flush.read().elapsed() >= self.config.flush_interval
                || self.write_buffer.read().len() >= 100
            {
                let _ = self.flush();
            }
        }

        debug!(trace_id = trace_id, "Stored trace");
    }

    /// Store a trace summary (without full span data)
    pub fn store_summary(&self, summary: TraceSummary) {
        let trace_id = summary.trace_id.clone();

        self.ensure_capacity();

        {
            let mut traces = self.traces.write();
            traces.insert(trace_id.clone(), summary.clone());
        }

        {
            let mut lru = self.lru.write();
            lru.push_back(trace_id.clone());
        }

        {
            let mut index = self.index.write();
            let trace_id_parsed = TraceId::from_hex(&trace_id).unwrap_or_default();
            index.insert(trace_id_parsed, &summary);
        }

        self.stats.traces_stored.fetch_add(1, Ordering::Relaxed);
    }

    /// Ensure capacity by evicting old traces
    fn ensure_capacity(&self) {
        let mut traces = self.traces.write();
        let mut trace_data = self.trace_data.write();
        let mut lru = self.lru.write();
        let mut index = self.index.write();

        while traces.len() >= self.config.max_memory_traces {
            if let Some(oldest_id) = lru.pop_front() {
                if let Some(summary) = traces.remove(&oldest_id) {
                    index.remove(&summary);
                    trace_data.remove(&oldest_id);
                    self.stats.traces_evicted.fetch_add(1, Ordering::Relaxed);
                }
            } else {
                break;
            }
        }
    }

    /// Get a trace by ID
    pub fn get(&self, trace_id: &str) -> Option<TraceSummary> {
        let traces = self.traces.read();
        traces.get(trace_id).cloned()
    }

    /// Get full trace data (spans) by ID
    pub fn get_spans(&self, trace_id: &str) -> Option<Vec<Span>> {
        let trace_data = self.trace_data.read();
        trace_data.get(trace_id).cloned()
    }

    /// Query traces
    pub fn query(&self, query: &TraceQuery) -> TraceQueryResult {
        let start = Instant::now();
        self.stats.queries_executed.fetch_add(1, Ordering::Relaxed);

        let traces = self.traces.read();

        // Filter traces
        let mut matching: Vec<&TraceSummary> = traces
            .values()
            .filter(|t| self.matches_query(t, query))
            .collect();

        let total_count = matching.len();

        // Sort
        if query.newest_first {
            matching.sort_by_key(|a| std::cmp::Reverse(a.start_time_ns));
        } else {
            matching.sort_by_key(|a| a.start_time_ns);
        }

        // Apply offset and limit
        let result_traces: Vec<TraceSummary> = matching
            .into_iter()
            .skip(query.offset)
            .take(query.limit)
            .cloned()
            .collect();

        let has_more = total_count > query.offset + result_traces.len();

        TraceQueryResult {
            traces: result_traces,
            total_count,
            query_time_ms: start.elapsed().as_millis() as u64,
            has_more,
        }
    }

    /// Check if a trace matches the query
    fn matches_query(&self, trace: &TraceSummary, query: &TraceQuery) -> bool {
        // Service filter
        if let Some(ref service) = query.service {
            if !trace.services.iter().any(|s| s.contains(service)) {
                return false;
            }
        }

        // Duration filter
        if let Some(min_dur) = query.min_duration_ns {
            if trace.duration_ns < min_dur {
                return false;
            }
        }

        if let Some(max_dur) = query.max_duration_ns {
            if trace.duration_ns > max_dur {
                return false;
            }
        }

        // Status filter
        if let Some(ref status) = query.status {
            if trace.status != *status {
                return false;
            }
        }

        // Time range filter
        if let Some(start) = query.start_time_ns {
            if trace.start_time_ns < start {
                return false;
            }
        }

        if let Some(end) = query.end_time_ns {
            if trace.end_time_ns > end {
                return false;
            }
        }

        // Operation filter
        if let Some(ref operation) = query.operation {
            if !trace.root_operation.contains(operation) {
                return false;
            }
        }

        true
    }

    /// Get recent traces
    pub fn recent(&self, limit: usize) -> Vec<TraceSummary> {
        self.query(&TraceQuery::new().limit(limit)).traces
    }

    /// Get slow traces (above threshold)
    pub fn slow_traces(&self, threshold_ns: u64, limit: usize) -> Vec<TraceSummary> {
        self.query(&TraceQuery::new().min_duration(threshold_ns).limit(limit))
            .traces
    }

    /// Get error traces
    pub fn error_traces(&self, limit: usize) -> Vec<TraceSummary> {
        self.query(&TraceQuery::new().status("error").limit(limit))
            .traces
    }

    /// Flush write buffer to disk
    pub fn flush(&self) -> Result<()> {
        if !self.config.persistence_enabled {
            return Ok(());
        }

        let traces_to_write: Vec<StoredTrace> = {
            let mut buffer = self.write_buffer.write();
            std::mem::take(&mut *buffer)
        };

        if traces_to_write.is_empty() {
            return Ok(());
        }

        // Get or create current file
        let file_path = self.get_or_create_file()?;

        let file = OpenOptions::new()
            .create(true)
            .append(true)
            .open(&file_path)
            .context("Failed to open trace file")?;

        let mut writer = BufWriter::new(file);
        let mut bytes_written = 0u64;

        for trace in traces_to_write {
            let json = serde_json::to_string(&trace)?;
            writeln!(writer, "{}", json)?;
            bytes_written += json.len() as u64 + 1;
        }

        writer.flush()?;

        self.stats
            .bytes_written
            .fetch_add(bytes_written, Ordering::Relaxed);
        *self.last_flush.write() = Instant::now();

        // Check if we need to rotate
        let file_size = fs::metadata(&file_path).map(|m| m.len()).unwrap_or(0);

        if file_size > self.config.max_file_size {
            self.rotate_files()?;
        }

        debug!(bytes = bytes_written, "Flushed traces to disk");
        Ok(())
    }

    /// Get or create the current trace file
    fn get_or_create_file(&self) -> Result<PathBuf> {
        let mut current = self.current_file.write();

        if current.is_none() {
            let timestamp = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap_or_default()
                .as_secs();

            let path = self
                .config
                .storage_path
                .join(format!("traces-{}.jsonl", timestamp));

            *current = Some(path);
        }

        Ok(current.clone().unwrap())
    }

    /// Rotate trace files
    fn rotate_files(&self) -> Result<()> {
        // Clear current file
        *self.current_file.write() = None;

        // List existing files
        let mut files: Vec<_> = fs::read_dir(&self.config.storage_path)?
            .filter_map(|e| e.ok())
            .filter(|e| {
                e.path()
                    .extension()
                    .map(|ext| ext == "jsonl")
                    .unwrap_or(false)
            })
            .collect();

        // Sort by modification time (oldest first)
        files.sort_by_key(|e| e.metadata().and_then(|m| m.modified()).ok());

        // Remove oldest files if we have too many
        while files.len() >= self.config.max_files {
            if let Some(oldest) = files.first() {
                fs::remove_file(oldest.path())?;
                files.remove(0);
            }
        }

        self.stats.file_rotations.fetch_add(1, Ordering::Relaxed);
        info!("Rotated trace files");

        Ok(())
    }

    /// Load traces from a file (for recovery/analysis)
    pub fn load_file(&self, path: &Path) -> Result<usize> {
        let file = File::open(path).context("Failed to open trace file")?;
        let reader = BufReader::new(file);
        let mut count = 0;

        for line in reader.lines() {
            let line = line?;
            if line.is_empty() {
                continue;
            }

            if let Ok(stored) = serde_json::from_str::<StoredTrace>(&line) {
                self.store_summary(stored.summary);
                count += 1;
            }
        }

        info!(path = %path.display(), count = count, "Loaded traces from file");
        Ok(count)
    }

    /// Clean up old data
    pub fn cleanup(&self) {
        let _now = Instant::now();
        let mut to_remove = Vec::new();

        {
            let traces = self.traces.read();
            let now_ns = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap_or_default()
                .as_nanos() as u64;

            for (id, summary) in traces.iter() {
                let age_ns = now_ns.saturating_sub(summary.start_time_ns);
                if age_ns > self.config.memory_retention.as_nanos() as u64 {
                    to_remove.push(id.clone());
                }
            }
        }

        if !to_remove.is_empty() {
            let mut traces = self.traces.write();
            let mut trace_data = self.trace_data.write();
            let mut index = self.index.write();
            let mut lru = self.lru.write();

            for id in to_remove {
                if let Some(summary) = traces.remove(&id) {
                    index.remove(&summary);
                    trace_data.remove(&id);
                    lru.retain(|i| i != &id);
                    self.stats.traces_evicted.fetch_add(1, Ordering::Relaxed);
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_summary(id: &str, duration_ns: u64) -> TraceSummary {
        TraceSummary {
            trace_id: id.to_string(),
            root_operation: "test-operation".to_string(),
            services: vec!["service-a".to_string()],
            span_count: 5,
            duration_ns,
            start_time_ns: 1000,
            end_time_ns: 1000 + duration_ns,
            status: "ok".to_string(),
            avg_latency_ns: Some(100),
            p95_latency_ns: Some(200),
            packet_count: 10,
            error_count: 0,
        }
    }

    #[test]
    fn test_store_and_retrieve() {
        let store = TraceStore::new(TraceStoreConfig::default()).unwrap();
        let summary = create_test_summary("test-trace-1", 5000);

        store.store_summary(summary.clone());

        let retrieved = store.get("test-trace-1");
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().trace_id, "test-trace-1");
    }

    #[test]
    fn test_query() {
        let store = TraceStore::new(TraceStoreConfig::default()).unwrap();

        store.store_summary(create_test_summary("trace-1", 1000));
        store.store_summary(create_test_summary("trace-2", 5000));
        store.store_summary(create_test_summary("trace-3", 10000));

        let result = store.query(&TraceQuery::new().min_duration(3000));
        assert_eq!(result.total_count, 2);

        let result = store.query(&TraceQuery::new().limit(1));
        assert_eq!(result.traces.len(), 1);
        assert!(result.has_more);
    }

    #[test]
    fn test_capacity_eviction() {
        let mut config = TraceStoreConfig::default();
        config.max_memory_traces = 2;

        let store = TraceStore::new(config).unwrap();

        store.store_summary(create_test_summary("trace-1", 1000));
        store.store_summary(create_test_summary("trace-2", 2000));
        store.store_summary(create_test_summary("trace-3", 3000));

        assert_eq!(store.trace_count(), 2);
        assert!(store.get("trace-1").is_none()); // Oldest should be evicted
    }

    #[test]
    fn test_recent_traces() {
        let store = TraceStore::new(TraceStoreConfig::default()).unwrap();

        for i in 0..5 {
            store.store_summary(create_test_summary(
                &format!("trace-{}", i),
                1000 * i as u64,
            ));
        }

        let recent = store.recent(3);
        assert_eq!(recent.len(), 3);
    }
}
