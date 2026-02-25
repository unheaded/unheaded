//! Multi-source eBPF reader for unified event collection.
//!
//! THE WHISPERING VOID listens to all corners of the kernel.
//! This module provides a unified interface to read from multiple eBPF
//! ring buffers simultaneously:
//!
//! - packet-marker (XDP): Packet events with trace ID extraction/injection
//! - flow-tracker (TC): TCP/UDP flow state tracking
//! - latency-probe (kprobe): TCP latency measurements
//! - syscall-tracer (tracepoint): System call auditing
//!
//! All events are correlated by trace ID and forwarded through a unified channel.

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use crossbeam::channel::Sender;
use parking_lot::RwLock;
use tracing::{debug, error, info, trace, warn};

use crate::events::{Event, EventData, EventType};
use crate::metrics;

/// Default paths for eBPF ring buffer maps
pub mod paths {
    pub const PACKET_EVENTS: &str = "/sys/fs/bpf/unheaded/packet_events";
    pub const FLOW_EVENTS: &str = "/sys/fs/bpf/unheaded/flow_events";
    pub const LATENCY_EVENTS: &str = "/sys/fs/bpf/unheaded/latency_events";
    pub const SYSCALL_EVENTS: &str = "/sys/fs/bpf/unheaded/syscall_events";

    // Pinned maps for statistics and control
    pub const PACKET_STATS: &str = "/sys/fs/bpf/unheaded/packet_stats";
    pub const FLOW_STATS: &str = "/sys/fs/bpf/unheaded/flow_stats";
    pub const LATENCY_STATS: &str = "/sys/fs/bpf/unheaded/latency_stats";
    pub const SYSCALL_STATS: &str = "/sys/fs/bpf/unheaded/syscall_stats";

    // Flow state and trace association maps
    pub const FLOW_STATE: &str = "/sys/fs/bpf/unheaded/flow_state";
    pub const TRACE_INJECT: &str = "/sys/fs/bpf/unheaded/trace_inject";
    pub const SOCK_TRACE: &str = "/sys/fs/bpf/unheaded/sock_trace";
}

/// Event source identifier
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum EventSource {
    /// XDP packet marker
    PacketMarker,
    /// TC flow tracker
    FlowTracker,
    /// Kprobe latency probe
    LatencyProbe,
    /// Tracepoint syscall tracer
    SyscallTracer,
}

impl EventSource {
    /// Get the display name for this source
    pub fn name(&self) -> &'static str {
        match self {
            Self::PacketMarker => "packet-marker",
            Self::FlowTracker => "flow-tracker",
            Self::LatencyProbe => "latency-probe",
            Self::SyscallTracer => "syscall-tracer",
        }
    }

    /// Get the default ring buffer path for this source
    pub fn default_path(&self) -> &'static str {
        match self {
            Self::PacketMarker => paths::PACKET_EVENTS,
            Self::FlowTracker => paths::FLOW_EVENTS,
            Self::LatencyProbe => paths::LATENCY_EVENTS,
            Self::SyscallTracer => paths::SYSCALL_EVENTS,
        }
    }

    /// Get the event type produced by this source
    pub fn event_type(&self) -> EventType {
        match self {
            Self::PacketMarker => EventType::Packet,
            Self::FlowTracker => EventType::Packet, // Flow events are packet-related
            Self::LatencyProbe => EventType::Latency,
            Self::SyscallTracer => EventType::Syscall,
        }
    }
}

/// Configuration for a single event source
#[derive(Debug, Clone)]
pub struct SourceConfig {
    /// Source identifier
    pub source: EventSource,
    /// Path to ring buffer map
    pub ringbuf_path: PathBuf,
    /// Path to stats map (optional)
    pub stats_path: Option<PathBuf>,
    /// Enable this source
    pub enabled: bool,
    /// Ring buffer size hint
    pub ringbuf_size: usize,
}

impl SourceConfig {
    /// Create a new source configuration with defaults
    pub fn new(source: EventSource) -> Self {
        Self {
            source,
            ringbuf_path: PathBuf::from(source.default_path()),
            stats_path: None,
            enabled: true,
            ringbuf_size: 256 * 1024, // 256KB default
        }
    }

    /// Set the ring buffer path
    pub fn with_path(mut self, path: impl Into<PathBuf>) -> Self {
        self.ringbuf_path = path.into();
        self
    }

    /// Enable or disable this source
    pub fn with_enabled(mut self, enabled: bool) -> Self {
        self.enabled = enabled;
        self
    }
}

/// Statistics for a single event source
#[derive(Debug, Default)]
pub struct SourceStats {
    /// Events read from this source
    pub events_read: AtomicU64,
    /// Events dropped (queue full)
    pub events_dropped: AtomicU64,
    /// Read errors
    pub read_errors: AtomicU64,
    /// Bytes processed
    pub bytes_processed: AtomicU64,
    /// Last event timestamp
    pub last_event_ns: AtomicU64,
}

impl SourceStats {
    pub fn new() -> Self {
        Self::default()
    }

    #[inline]
    pub fn record_read(&self, count: u64, bytes: u64) {
        self.events_read.fetch_add(count, Ordering::Relaxed);
        self.bytes_processed.fetch_add(bytes, Ordering::Relaxed);
    }

    #[inline]
    pub fn record_drop(&self) {
        self.events_dropped.fetch_add(1, Ordering::Relaxed);
    }

    #[inline]
    pub fn record_error(&self) {
        self.read_errors.fetch_add(1, Ordering::Relaxed);
    }

    #[inline]
    pub fn set_last_event(&self, timestamp_ns: u64) {
        self.last_event_ns.store(timestamp_ns, Ordering::Relaxed);
    }
}

/// Configuration for the multi-source reader
#[derive(Debug, Clone)]
pub struct MultiSourceConfig {
    /// Configurations for each source
    pub sources: Vec<SourceConfig>,
    /// Poll timeout in milliseconds
    pub poll_timeout_ms: u64,
    /// Enable parallel reading
    pub parallel_read: bool,
    /// Number of worker threads for parallel reading
    pub workers: usize,
}

impl Default for MultiSourceConfig {
    fn default() -> Self {
        Self {
            sources: vec![
                SourceConfig::new(EventSource::PacketMarker),
                SourceConfig::new(EventSource::FlowTracker),
                SourceConfig::new(EventSource::LatencyProbe),
                SourceConfig::new(EventSource::SyscallTracer),
            ],
            poll_timeout_ms: 100,
            parallel_read: true,
            workers: 4,
        }
    }
}

impl MultiSourceConfig {
    /// Create a new configuration with only packet sources
    pub fn packets_only() -> Self {
        Self {
            sources: vec![
                SourceConfig::new(EventSource::PacketMarker),
                SourceConfig::new(EventSource::FlowTracker),
            ],
            ..Default::default()
        }
    }

    /// Create a new configuration with all sources
    pub fn all_sources() -> Self {
        Self::default()
    }

    /// Add a custom source configuration
    pub fn with_source(mut self, config: SourceConfig) -> Self {
        // Replace if exists, otherwise add
        if let Some(pos) = self.sources.iter().position(|s| s.source == config.source) {
            self.sources[pos] = config;
        } else {
            self.sources.push(config);
        }
        self
    }

    /// Set poll timeout
    pub fn with_poll_timeout(mut self, timeout_ms: u64) -> Self {
        self.poll_timeout_ms = timeout_ms;
        self
    }
}

/// Unified event from any source, with source metadata
#[derive(Debug, Clone)]
pub struct SourcedEvent {
    /// The event data
    pub event: Event,
    /// Source that produced this event
    pub source: EventSource,
    /// Local receive timestamp
    pub received_at: Instant,
}

impl SourcedEvent {
    pub fn new(event: Event, source: EventSource) -> Self {
        Self {
            event,
            source,
            received_at: Instant::now(),
        }
    }

    /// Get the latency from kernel event to userspace receipt
    pub fn kernel_to_user_latency_ns(&self, boot_time_ns: u64) -> u64 {
        // Approximate: use the difference between receive time and event timestamp
        // This is not perfectly accurate but gives a reasonable estimate
        let now_ns = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos() as u64;

        now_ns.saturating_sub(self.event.timestamp_ns)
    }
}

/// State for a single reader task
struct ReaderState {
    source: EventSource,
    ringbuf_path: PathBuf,
    ringbuf_size: usize,
    stats: Arc<SourceStats>,
}

/// Multi-source eBPF event reader
///
/// Reads events from multiple eBPF ring buffers concurrently and
/// forwards them through a unified channel.
pub struct MultiSourceReader {
    /// Configuration
    config: MultiSourceConfig,
    /// Per-source statistics
    stats: HashMap<EventSource, Arc<SourceStats>>,
    /// Global statistics
    global_stats: Arc<GlobalStats>,
    /// Shutdown flag
    shutdown: Arc<AtomicBool>,
}

/// Global statistics across all sources
#[derive(Debug, Default)]
pub struct GlobalStats {
    pub total_events: AtomicU64,
    pub total_dropped: AtomicU64,
    pub total_errors: AtomicU64,
    pub total_bytes: AtomicU64,
}

impl MultiSourceReader {
    /// Create a new multi-source reader
    pub fn new(config: MultiSourceConfig) -> Self {
        let mut stats = HashMap::new();
        for source_config in &config.sources {
            stats.insert(source_config.source, Arc::new(SourceStats::new()));
        }

        Self {
            config,
            stats,
            global_stats: Arc::new(GlobalStats::default()),
            shutdown: Arc::new(AtomicBool::new(false)),
        }
    }

    /// Get statistics for a specific source
    pub fn source_stats(&self, source: EventSource) -> Option<&Arc<SourceStats>> {
        self.stats.get(&source)
    }

    /// Get global statistics
    pub fn global_stats(&self) -> &Arc<GlobalStats> {
        &self.global_stats
    }

    /// Signal shutdown
    pub fn shutdown(&self) {
        self.shutdown.store(true, Ordering::SeqCst);
    }

    /// Check which sources are available
    pub fn check_sources(&self) -> Vec<(EventSource, bool)> {
        self.config
            .sources
            .iter()
            .map(|s| {
                let available = s.enabled && s.ringbuf_path.exists();
                (s.source, available)
            })
            .collect()
    }

    /// Run the multi-source reader
    ///
    /// This spawns reader tasks for each available source and forwards
    /// all events through the provided channel.
    pub async fn run(&self, event_tx: Sender<Event>) -> Result<()> {
        info!("Multi-source reader starting");

        let mut handles = Vec::new();

        // Discover available sources
        let available_sources: Vec<_> = self
            .config
            .sources
            .iter()
            .filter(|s| s.enabled && s.ringbuf_path.exists())
            .cloned()
            .collect();

        if available_sources.is_empty() {
            warn!("No eBPF ring buffer sources found. Make sure eBPF programs are loaded.");
            // Wait for shutdown
            while !self.shutdown.load(Ordering::Relaxed) {
                tokio::time::sleep(Duration::from_millis(100)).await;
            }
            return Ok(());
        }

        info!(
            sources = available_sources.len(),
            "Found available eBPF sources"
        );

        // Log each available source
        for source in &available_sources {
            info!(
                source = source.source.name(),
                path = %source.ringbuf_path.display(),
                "Source available"
            );
        }

        // Spawn reader tasks
        for source_config in available_sources {
            let source = source_config.source;
            let path = source_config.ringbuf_path.clone();
            let size = source_config.ringbuf_size;
            let stats = self.stats.get(&source).cloned().unwrap_or_default();
            let global_stats = Arc::clone(&self.global_stats);
            let shutdown = Arc::clone(&self.shutdown);
            let tx = event_tx.clone();
            let poll_timeout = self.config.poll_timeout_ms;

            let handle = tokio::spawn(async move {
                run_source_reader(source, path, size, stats, global_stats, shutdown, tx, poll_timeout)
                    .await
            });

            handles.push((source, handle));
        }

        // Wait for all readers to complete
        for (source, handle) in handles {
            match handle.await {
                Ok(Ok(())) => {
                    debug!(source = source.name(), "Reader completed normally");
                }
                Ok(Err(e)) => {
                    error!(source = source.name(), error = %e, "Reader error");
                }
                Err(e) => {
                    error!(source = source.name(), error = %e, "Reader task panicked");
                }
            }
        }

        info!("Multi-source reader stopped");
        Ok(())
    }
}

/// Run a single source reader
async fn run_source_reader(
    source: EventSource,
    path: PathBuf,
    ringbuf_size: usize,
    stats: Arc<SourceStats>,
    global_stats: Arc<GlobalStats>,
    shutdown: Arc<AtomicBool>,
    event_tx: Sender<Event>,
    poll_timeout_ms: u64,
) -> Result<()> {
    use super::ringbuf::RingBufReader;

    debug!(
        source = source.name(),
        path = %path.display(),
        "Starting source reader"
    );

    // Create the ring buffer reader
    let mut reader = match RingBufReader::new(&path, ringbuf_size) {
        Ok(r) => r,
        Err(e) => {
            error!(
                source = source.name(),
                path = %path.display(),
                error = %e,
                "Failed to open ring buffer"
            );
            stats.record_error();
            return Err(e.into());
        }
    };

    // Run the reader loop
    reader
        .run(event_tx, shutdown)
        .await
        .map_err(|e| anyhow::anyhow!("Reader error: {}", e))?;

    debug!(
        source = source.name(),
        events = stats.events_read.load(Ordering::Relaxed),
        "Source reader stopped"
    );

    Ok(())
}

/// Builder for MultiSourceReader
pub struct MultiSourceReaderBuilder {
    config: MultiSourceConfig,
}

impl MultiSourceReaderBuilder {
    pub fn new() -> Self {
        Self {
            config: MultiSourceConfig::default(),
        }
    }

    /// Use a custom base path for all sources
    pub fn with_base_path(mut self, base: impl AsRef<Path>) -> Self {
        let base = base.as_ref();
        for source in &mut self.config.sources {
            source.ringbuf_path = base.join(format!("{}_events", source.source.name().replace('-', "_")));
        }
        self
    }

    /// Enable only packet-related sources
    pub fn packets_only(mut self) -> Self {
        for source in &mut self.config.sources {
            source.enabled = matches!(
                source.source,
                EventSource::PacketMarker | EventSource::FlowTracker
            );
        }
        self
    }

    /// Enable all sources
    pub fn all_sources(mut self) -> Self {
        for source in &mut self.config.sources {
            source.enabled = true;
        }
        self
    }

    /// Configure a specific source
    pub fn configure_source(mut self, source: EventSource, f: impl FnOnce(SourceConfig) -> SourceConfig) -> Self {
        if let Some(config) = self.config.sources.iter_mut().find(|s| s.source == source) {
            *config = f(config.clone());
        }
        self
    }

    /// Set poll timeout
    pub fn poll_timeout(mut self, timeout_ms: u64) -> Self {
        self.config.poll_timeout_ms = timeout_ms;
        self
    }

    /// Set number of workers
    pub fn workers(mut self, workers: usize) -> Self {
        self.config.workers = workers;
        self
    }

    /// Build the reader
    pub fn build(self) -> MultiSourceReader {
        MultiSourceReader::new(self.config)
    }
}

impl Default for MultiSourceReaderBuilder {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_event_source_names() {
        assert_eq!(EventSource::PacketMarker.name(), "packet-marker");
        assert_eq!(EventSource::FlowTracker.name(), "flow-tracker");
        assert_eq!(EventSource::LatencyProbe.name(), "latency-probe");
        assert_eq!(EventSource::SyscallTracer.name(), "syscall-tracer");
    }

    #[test]
    fn test_source_config() {
        let config = SourceConfig::new(EventSource::PacketMarker)
            .with_path("/custom/path")
            .with_enabled(false);

        assert_eq!(config.ringbuf_path, PathBuf::from("/custom/path"));
        assert!(!config.enabled);
    }

    #[test]
    fn test_multi_source_config() {
        let config = MultiSourceConfig::default();
        assert_eq!(config.sources.len(), 4);
        assert!(config.parallel_read);
    }

    #[test]
    fn test_builder() {
        let reader = MultiSourceReaderBuilder::new()
            .packets_only()
            .poll_timeout(50)
            .workers(2)
            .build();

        let available = reader.check_sources();
        // All sources should be configured, but only packet sources enabled
        assert_eq!(available.len(), 4);
    }

    #[test]
    fn test_source_stats() {
        let stats = SourceStats::new();
        stats.record_read(10, 1000);
        stats.record_drop();
        stats.record_error();

        assert_eq!(stats.events_read.load(Ordering::Relaxed), 10);
        assert_eq!(stats.bytes_processed.load(Ordering::Relaxed), 1000);
        assert_eq!(stats.events_dropped.load(Ordering::Relaxed), 1);
        assert_eq!(stats.read_errors.load(Ordering::Relaxed), 1);
    }
}
