//! Event batching for efficient publishing.
//!
//! THE WHISPERING VOID gathers whispers before speaking,
//! for efficiency in the realm of messages.
//!
//! This module provides batching logic to efficiently group events
//! before publishing them to Busboy.

use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{Duration, Instant};

use anyhow::Result;
use bytes::Bytes;
use flate2::write::GzEncoder;
use flate2::Compression;
use prost::Message;
use std::io::Write;
use tracing::{debug, trace};

use crate::events::Event;
use crate::proto::{EventType as ProtoEventType, TraceEvent, TraceEventBatch};

/// Default batch size
pub const DEFAULT_BATCH_SIZE: usize = 1000;

/// Default batch timeout
pub const DEFAULT_BATCH_TIMEOUT: Duration = Duration::from_millis(10);

/// Maximum batch size (bytes)
pub const MAX_BATCH_BYTES: usize = 4 * 1024 * 1024; // 4MB

/// Compression threshold (bytes)
pub const COMPRESSION_THRESHOLD: usize = 1024;

/// Batch state
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum BatchState {
    /// Empty batch
    Empty,
    /// Collecting events
    Collecting,
    /// Ready to flush
    Ready,
    /// Flushing in progress
    Flushing,
}

/// Batch statistics
#[derive(Debug, Default)]
pub struct BatchStats {
    /// Total batches created
    pub batches_created: AtomicU64,
    /// Total batches flushed
    pub batches_flushed: AtomicU64,
    /// Total events batched
    pub events_batched: AtomicU64,
    /// Total bytes before compression
    pub bytes_raw: AtomicU64,
    /// Total bytes after compression
    pub bytes_compressed: AtomicU64,
    /// Compression savings (bytes)
    pub compression_savings: AtomicU64,
}

impl BatchStats {
    pub fn new() -> Self {
        Self::default()
    }

    /// Get compression ratio
    pub fn compression_ratio(&self) -> f64 {
        let raw = self.bytes_raw.load(Ordering::Relaxed) as f64;
        let compressed = self.bytes_compressed.load(Ordering::Relaxed) as f64;
        if compressed > 0.0 {
            raw / compressed
        } else {
            1.0
        }
    }
}

/// Event batcher for efficient publishing
pub struct EventBatcher {
    /// Collector ID
    collector_id: String,
    /// Maximum batch size (events)
    max_size: usize,
    /// Batch timeout
    timeout: Duration,
    /// Enable compression
    compression: bool,
    /// Current batch
    events: Vec<Event>,
    /// First event timestamp
    first_timestamp: u64,
    /// Last event timestamp
    last_timestamp: u64,
    /// Current batch bytes (estimated)
    current_bytes: usize,
    /// Batch creation time
    created_at: Instant,
    /// Sequence counter
    sequence: u64,
    /// Statistics
    stats: BatchStats,
}

impl EventBatcher {
    /// Create a new event batcher
    pub fn new(collector_id: String, max_size: usize, timeout: Duration) -> Self {
        Self {
            collector_id,
            max_size,
            timeout,
            compression: true,
            events: Vec::with_capacity(max_size),
            first_timestamp: 0,
            last_timestamp: 0,
            current_bytes: 0,
            created_at: Instant::now(),
            sequence: 0,
            stats: BatchStats::new(),
        }
    }

    /// Create with default settings
    pub fn with_defaults(collector_id: String) -> Self {
        Self::new(collector_id, DEFAULT_BATCH_SIZE, DEFAULT_BATCH_TIMEOUT)
    }

    /// Enable or disable compression
    pub fn set_compression(&mut self, enabled: bool) {
        self.compression = enabled;
    }

    /// Get current batch size
    pub fn len(&self) -> usize {
        self.events.len()
    }

    /// Check if batch is empty
    pub fn is_empty(&self) -> bool {
        self.events.is_empty()
    }

    /// Get current state
    pub fn state(&self) -> BatchState {
        if self.events.is_empty() {
            BatchState::Empty
        } else if self.should_flush() {
            BatchState::Ready
        } else {
            BatchState::Collecting
        }
    }

    /// Check if batch should be flushed
    pub fn should_flush(&self) -> bool {
        if self.events.is_empty() {
            return false;
        }

        // Flush if size limit reached
        if self.events.len() >= self.max_size {
            return true;
        }

        // Flush if byte limit reached
        if self.current_bytes >= MAX_BATCH_BYTES {
            return true;
        }

        // Flush if timeout elapsed
        if self.created_at.elapsed() >= self.timeout {
            return true;
        }

        false
    }

    /// Add an event to the batch
    pub fn add(&mut self, event: Event) -> bool {
        let event_bytes = event.size_bytes();

        // Check if adding would exceed byte limit
        if self.current_bytes + event_bytes > MAX_BATCH_BYTES && !self.events.is_empty() {
            return false;
        }

        // Track timestamps
        if self.events.is_empty() {
            self.first_timestamp = event.timestamp_ns;
            self.created_at = Instant::now();
        }
        self.last_timestamp = event.timestamp_ns;

        // Add event
        self.current_bytes += event_bytes;
        self.events.push(event);
        self.stats.events_batched.fetch_add(1, Ordering::Relaxed);

        true
    }

    /// Build a protobuf batch from current events
    pub fn build_proto_batch(&self) -> TraceEventBatch {
        let events: Vec<TraceEvent> = self.events.iter().map(event_to_proto).collect();

        TraceEventBatch {
            collector_id: self.collector_id.clone(),
            sequence: self.sequence,
            events,
            first_timestamp_ns: self.first_timestamp,
            last_timestamp_ns: self.last_timestamp,
            compression: 0,
            total_events: self.events.len() as u32,
        }
    }

    /// Flush the batch and return serialized payload
    pub fn flush(&mut self) -> Option<BatchPayload> {
        if self.events.is_empty() {
            return None;
        }

        let batch = self.build_proto_batch();
        let event_count = batch.events.len();

        // Encode
        let mut raw_bytes = Vec::with_capacity(batch.encoded_len());
        batch.encode(&mut raw_bytes).ok()?;

        let raw_len = raw_bytes.len();
        self.stats.bytes_raw.fetch_add(raw_len as u64, Ordering::Relaxed);

        // Compress if enabled and beneficial
        let (payload, compressed) = if self.compression && raw_len > COMPRESSION_THRESHOLD {
            match compress_gzip(&raw_bytes) {
                Ok(compressed_bytes) if compressed_bytes.len() < raw_len => {
                    let savings = raw_len - compressed_bytes.len();
                    self.stats
                        .compression_savings
                        .fetch_add(savings as u64, Ordering::Relaxed);
                    self.stats
                        .bytes_compressed
                        .fetch_add(compressed_bytes.len() as u64, Ordering::Relaxed);
                    (compressed_bytes, true)
                }
                _ => {
                    self.stats
                        .bytes_compressed
                        .fetch_add(raw_len as u64, Ordering::Relaxed);
                    (raw_bytes, false)
                }
            }
        } else {
            self.stats
                .bytes_compressed
                .fetch_add(raw_len as u64, Ordering::Relaxed);
            (raw_bytes, false)
        };

        let result = BatchPayload {
            sequence: self.sequence,
            event_count,
            raw_bytes: raw_len,
            payload_bytes: payload.len(),
            compressed,
            first_timestamp: self.first_timestamp,
            last_timestamp: self.last_timestamp,
            payload: Bytes::from(payload),
        };

        // Reset batch state
        self.events.clear();
        self.first_timestamp = 0;
        self.last_timestamp = 0;
        self.current_bytes = 0;
        self.sequence += 1;
        self.created_at = Instant::now();

        self.stats.batches_flushed.fetch_add(1, Ordering::Relaxed);

        debug!(
            sequence = result.sequence,
            events = result.event_count,
            raw_bytes = result.raw_bytes,
            payload_bytes = result.payload_bytes,
            compressed = result.compressed,
            "Flushed batch"
        );

        Some(result)
    }

    /// Clear the batch without flushing
    pub fn clear(&mut self) {
        self.events.clear();
        self.first_timestamp = 0;
        self.last_timestamp = 0;
        self.current_bytes = 0;
        self.created_at = Instant::now();
    }

    /// Get statistics
    pub fn stats(&self) -> &BatchStats {
        &self.stats
    }

    /// Get batch duration
    pub fn duration(&self) -> Duration {
        self.created_at.elapsed()
    }
}

/// Flushed batch payload
#[derive(Debug, Clone)]
pub struct BatchPayload {
    /// Sequence number
    pub sequence: u64,
    /// Number of events
    pub event_count: usize,
    /// Raw bytes (before compression)
    pub raw_bytes: usize,
    /// Payload bytes (after compression)
    pub payload_bytes: usize,
    /// Whether payload is compressed
    pub compressed: bool,
    /// First event timestamp
    pub first_timestamp: u64,
    /// Last event timestamp
    pub last_timestamp: u64,
    /// Serialized payload
    pub payload: Bytes,
}

impl BatchPayload {
    /// Get compression ratio
    pub fn compression_ratio(&self) -> f64 {
        if self.payload_bytes > 0 {
            self.raw_bytes as f64 / self.payload_bytes as f64
        } else {
            1.0
        }
    }

    /// Get batch duration in nanoseconds
    pub fn duration_ns(&self) -> u64 {
        self.last_timestamp.saturating_sub(self.first_timestamp)
    }
}

/// Convert Event to protobuf TraceEvent
fn event_to_proto(event: &Event) -> TraceEvent {
    let event_type = match event.event_type {
        crate::events::EventType::Packet => ProtoEventType::Packet,
        crate::events::EventType::Syscall => ProtoEventType::Syscall,
        crate::events::EventType::Latency => ProtoEventType::Latency,
        crate::events::EventType::Process => ProtoEventType::Process,
        crate::events::EventType::FileSystem => ProtoEventType::FileSystem,
        crate::events::EventType::Memory => ProtoEventType::Memory,
        crate::events::EventType::Custom => ProtoEventType::Custom,
    };

    // Serialize event data to JSON
    let data = serde_json::to_vec(&event.data).unwrap_or_default();

    let raw = event.raw.as_ref().map(|b| b.to_vec()).unwrap_or_default();

    TraceEvent {
        event_type: event_type as i32,
        timestamp_ns: event.timestamp_ns,
        cpu: event.cpu as u32,
        pid: event.pid,
        tid: event.tid,
        comm: event.comm.clone(),
        trace_id: None,
        parent_span_id: None,
        span_id: None,
        data,
        raw,
        flags: 0,
        hostname: hostname::get()
            .map(|h| h.to_string_lossy().to_string())
            .unwrap_or_default(),
    }
}

/// Compress data with gzip
fn compress_gzip(data: &[u8]) -> Result<Vec<u8>> {
    let mut encoder = GzEncoder::new(Vec::new(), Compression::fast());
    encoder.write_all(data)?;
    Ok(encoder.finish()?)
}

/// Decompress gzip data
#[allow(dead_code)]
fn decompress_gzip(data: &[u8]) -> Result<Vec<u8>> {
    use flate2::read::GzDecoder;
    use std::io::Read;

    let mut decoder = GzDecoder::new(data);
    let mut result = Vec::new();
    decoder.read_to_end(&mut result)?;
    Ok(result)
}

/// Adaptive batcher that adjusts batch size based on throughput
pub struct AdaptiveBatcher {
    /// Inner batcher
    inner: EventBatcher,
    /// Target throughput (events/second)
    target_throughput: u64,
    /// Minimum batch size
    min_batch_size: usize,
    /// Maximum batch size
    max_batch_size: usize,
    /// Current adaptive batch size
    current_batch_size: usize,
    /// Events processed in last window
    events_in_window: u64,
    /// Window start time
    window_start: Instant,
    /// Window duration
    window_duration: Duration,
}

impl AdaptiveBatcher {
    /// Create a new adaptive batcher
    pub fn new(
        collector_id: String,
        target_throughput: u64,
        min_batch_size: usize,
        max_batch_size: usize,
    ) -> Self {
        Self {
            inner: EventBatcher::new(collector_id, max_batch_size, DEFAULT_BATCH_TIMEOUT),
            target_throughput,
            min_batch_size,
            max_batch_size,
            current_batch_size: DEFAULT_BATCH_SIZE,
            events_in_window: 0,
            window_start: Instant::now(),
            window_duration: Duration::from_secs(1),
        }
    }

    /// Add an event
    pub fn add(&mut self, event: Event) -> bool {
        self.events_in_window += 1;
        self.adapt_batch_size();
        self.inner.add(event)
    }

    /// Check if should flush
    pub fn should_flush(&self) -> bool {
        self.inner.len() >= self.current_batch_size || self.inner.should_flush()
    }

    /// Flush the batch
    pub fn flush(&mut self) -> Option<BatchPayload> {
        self.inner.flush()
    }

    /// Adapt batch size based on throughput
    fn adapt_batch_size(&mut self) {
        if self.window_start.elapsed() >= self.window_duration {
            let current_throughput = self.events_in_window;

            if current_throughput > self.target_throughput {
                // High throughput: increase batch size for efficiency
                self.current_batch_size = (self.current_batch_size + 100).min(self.max_batch_size);
            } else if current_throughput < self.target_throughput / 2 {
                // Low throughput: decrease batch size for latency
                self.current_batch_size = (self.current_batch_size.saturating_sub(100)).max(self.min_batch_size);
            }

            self.events_in_window = 0;
            self.window_start = Instant::now();

            trace!(
                throughput = current_throughput,
                batch_size = self.current_batch_size,
                "Adapted batch size"
            );
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::events::{EventData, EventType};

    fn make_test_event() -> Event {
        Event::new(
            EventType::Packet,
            0,
            1000000,
            1234,
            1234,
            "test".to_string(),
            EventData::Raw(vec![1, 2, 3, 4]),
        )
    }

    #[test]
    fn test_batcher_empty() {
        let batcher = EventBatcher::with_defaults("test".to_string());
        assert!(batcher.is_empty());
        assert_eq!(batcher.state(), BatchState::Empty);
    }

    #[test]
    fn test_batcher_add() {
        let mut batcher = EventBatcher::with_defaults("test".to_string());
        let event = make_test_event();

        assert!(batcher.add(event));
        assert_eq!(batcher.len(), 1);
        assert_eq!(batcher.state(), BatchState::Collecting);
    }

    #[test]
    fn test_batcher_flush() {
        let mut batcher = EventBatcher::new("test".to_string(), 2, Duration::from_secs(10));

        batcher.add(make_test_event());
        batcher.add(make_test_event());

        let payload = batcher.flush();
        assert!(payload.is_some());

        let payload = payload.unwrap();
        assert_eq!(payload.event_count, 2);
        assert!(payload.payload_bytes > 0);
    }

    #[test]
    fn test_batcher_size_limit() {
        let mut batcher = EventBatcher::new("test".to_string(), 3, Duration::from_secs(10));

        batcher.add(make_test_event());
        batcher.add(make_test_event());
        assert!(!batcher.should_flush());

        batcher.add(make_test_event());
        assert!(batcher.should_flush());
    }

    #[test]
    fn test_compression() {
        let data = b"Hello, World! ".repeat(100);
        let compressed = compress_gzip(&data).unwrap();

        // Compressed should be smaller than original
        assert!(compressed.len() < data.len());

        // Decompress should match original
        let decompressed = decompress_gzip(&compressed).unwrap();
        assert_eq!(decompressed, data);
    }

    #[test]
    fn test_batch_payload_metrics() {
        let payload = BatchPayload {
            sequence: 1,
            event_count: 10,
            raw_bytes: 1000,
            payload_bytes: 500,
            compressed: true,
            first_timestamp: 1000,
            last_timestamp: 2000,
            payload: Bytes::new(),
        };

        assert_eq!(payload.compression_ratio(), 2.0);
        assert_eq!(payload.duration_ns(), 1000);
    }

    #[test]
    fn test_batch_stats() {
        let stats = BatchStats::new();
        stats.bytes_raw.store(1000, Ordering::SeqCst);
        stats.bytes_compressed.store(500, Ordering::SeqCst);

        assert_eq!(stats.compression_ratio(), 2.0);
    }
}
