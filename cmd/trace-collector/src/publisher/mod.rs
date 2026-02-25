//! Wotan Publisher - gRPC client for publishing events to the message bus.
//!
//! THE WHISPERING VOID speaks through Wotan, delivering kernel whispers
//! to the realm of userspace services.
//!
//! Features:
//! - Batched event publishing for efficiency
//! - Automatic reconnection with exponential backoff
//! - Compression support (gzip)
//! - Backpressure handling
//! - Graceful shutdown
//!
//! ## Submodules
//!
//! - `batch` - Event batching logic
//! - `grpc` - Low-level gRPC client

pub mod batch;
pub mod grpc;

// Re-export batch types
pub use batch::{
    AdaptiveBatcher, BatchPayload, BatchState, BatchStats, EventBatcher, DEFAULT_BATCH_SIZE,
    DEFAULT_BATCH_TIMEOUT, MAX_BATCH_BYTES,
};

// Re-export gRPC types
pub use grpc::{WotanClient, ClientState, ClientStats, StreamingClient};

use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use bytes::Bytes;
use crossbeam::channel::Receiver;
use parking_lot::RwLock;
use prost::Message;
use serde::{Deserialize, Serialize};
use tokio::sync::mpsc;
use tokio::time::timeout;
use tonic::transport::{Channel, Endpoint};
use tonic::{Request, Status};
use tracing::{debug, error, info, trace, warn};

use crate::events::{Event, EventBatch, EventData, EventType};
use crate::metrics;

/// Maximum message size (4MB)
const MAX_MESSAGE_SIZE: usize = 4 * 1024 * 1024;

/// Maximum retry attempts
const MAX_RETRIES: u32 = 5;

/// Base retry backoff
const BASE_RETRY_BACKOFF: Duration = Duration::from_millis(100);

/// Maximum retry backoff
const MAX_RETRY_BACKOFF: Duration = Duration::from_secs(30);

/// Connection timeout
const CONNECT_TIMEOUT: Duration = Duration::from_secs(5);

/// Request timeout
const REQUEST_TIMEOUT: Duration = Duration::from_secs(10);

/// Wotan gRPC service client
///
/// This is a simplified representation. In production, you'd generate
/// this from protobuf definitions using tonic-build.
pub mod wotan_proto {
    /// Trace event message for Wotan
    #[derive(Clone, PartialEq, prost::Message)]
    pub struct TraceEvent {
        /// Event type
        #[prost(enumeration = "EventType", tag = "1")]
        pub event_type: i32,
        /// Timestamp in nanoseconds since Unix epoch
        #[prost(uint64, tag = "2")]
        pub timestamp_ns: u64,
        /// CPU where event originated
        #[prost(uint32, tag = "3")]
        pub cpu: u32,
        /// Process ID
        #[prost(uint32, tag = "4")]
        pub pid: u32,
        /// Thread ID
        #[prost(uint32, tag = "5")]
        pub tid: u32,
        /// Process/command name
        #[prost(string, tag = "6")]
        pub comm: String,
        /// Serialized event data (JSON for flexibility)
        #[prost(bytes = "vec", tag = "7")]
        pub data: Vec<u8>,
        /// Raw event bytes (optional, for forwarding)
        #[prost(bytes = "vec", tag = "8")]
        pub raw: Vec<u8>,
    }

    /// Event type enumeration
    #[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, prost::Enumeration)]
    #[repr(i32)]
    pub enum EventType {
        Packet = 1,
        Syscall = 2,
        Latency = 3,
        Process = 4,
        FileSystem = 5,
        Memory = 6,
        Custom = 255,
    }

    /// Batch of trace events
    #[derive(Clone, PartialEq, prost::Message)]
    pub struct TraceEventBatch {
        /// Source collector ID
        #[prost(string, tag = "1")]
        pub collector_id: String,
        /// Batch sequence number
        #[prost(uint64, tag = "2")]
        pub sequence: u64,
        /// Events in this batch
        #[prost(message, repeated, tag = "3")]
        pub events: Vec<TraceEvent>,
        /// First event timestamp
        #[prost(uint64, tag = "4")]
        pub first_timestamp_ns: u64,
        /// Last event timestamp
        #[prost(uint64, tag = "5")]
        pub last_timestamp_ns: u64,
        /// Compression (0=none, 1=gzip)
        #[prost(uint32, tag = "6")]
        pub compression: u32,
    }

    /// Publish response
    #[derive(Clone, PartialEq, prost::Message)]
    pub struct PublishResponse {
        /// Success flag
        #[prost(bool, tag = "1")]
        pub success: bool,
        /// Number of events accepted
        #[prost(uint64, tag = "2")]
        pub accepted: u64,
        /// Number of events rejected
        #[prost(uint64, tag = "3")]
        pub rejected: u64,
        /// Error message (if any)
        #[prost(string, tag = "4")]
        pub error: String,
        /// Server-side sequence (for acknowledgment)
        #[prost(uint64, tag = "5")]
        pub server_sequence: u64,
    }

    /// Health check request
    #[derive(Clone, PartialEq, prost::Message)]
    pub struct HealthRequest {}

    /// Health check response
    #[derive(Clone, PartialEq, prost::Message)]
    pub struct HealthResponse {
        #[prost(bool, tag = "1")]
        pub healthy: bool,
        #[prost(string, tag = "2")]
        pub version: String,
    }

    /// Subscribe request for receiving events
    #[derive(Clone, PartialEq, prost::Message)]
    pub struct SubscribeRequest {
        /// Topic pattern (e.g., "trace.*", "trace.packet")
        #[prost(string, tag = "1")]
        pub topic_pattern: String,
        /// Subscriber ID
        #[prost(string, tag = "2")]
        pub subscriber_id: String,
    }
}

/// Publisher state
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PublisherState {
    /// Not connected
    Disconnected,
    /// Attempting to connect
    Connecting,
    /// Connected and ready
    Connected,
    /// Shutting down
    ShuttingDown,
}

/// Publisher statistics
#[derive(Debug, Default)]
pub struct PublisherStats {
    /// Total events published
    pub events_published: AtomicU64,
    /// Total batches published
    pub batches_published: AtomicU64,
    /// Total publish errors
    pub publish_errors: AtomicU64,
    /// Total reconnections
    pub reconnections: AtomicU64,
    /// Current batch size
    pub current_batch_size: AtomicU64,
    /// Last publish latency (microseconds)
    pub last_publish_latency_us: AtomicU64,
}

impl PublisherStats {
    pub fn new() -> Self {
        Self::default()
    }
}

/// Wotan publisher for trace events
pub struct WotanPublisher {
    /// gRPC endpoint URL
    endpoint: String,
    /// Collector identifier
    collector_id: String,
    /// gRPC channel (connection)
    channel: RwLock<Option<Channel>>,
    /// Batch size
    batch_size: usize,
    /// Batch timeout
    batch_timeout: Duration,
    /// Enable compression
    compression: bool,
    /// Current state
    state: RwLock<PublisherState>,
    /// Sequence number
    sequence: AtomicU64,
    /// Statistics
    stats: Arc<PublisherStats>,
}

impl WotanPublisher {
    /// Create a new Wotan publisher
    pub async fn new(
        endpoint: &str,
        batch_size: usize,
        batch_timeout: Duration,
    ) -> Result<Self> {
        let collector_id = format!(
            "trace-collector-{}",
            hostname::get()
                .map(|h| h.to_string_lossy().to_string())
                .unwrap_or_else(|_| "unknown".to_string())
        );

        info!(
            endpoint = endpoint,
            collector_id = collector_id,
            batch_size = batch_size,
            "Creating Wotan publisher"
        );

        let publisher = Self {
            endpoint: endpoint.to_string(),
            collector_id,
            channel: RwLock::new(None),
            batch_size,
            batch_timeout,
            compression: true,
            state: RwLock::new(PublisherState::Disconnected),
            sequence: AtomicU64::new(0),
            stats: Arc::new(PublisherStats::new()),
        };

        // Try initial connection
        publisher.connect().await?;

        Ok(publisher)
    }

    /// Connect to Wotan
    async fn connect(&self) -> Result<()> {
        *self.state.write() = PublisherState::Connecting;

        info!(endpoint = self.endpoint, "Connecting to Wotan");

        let endpoint = Endpoint::from_shared(self.endpoint.clone())
            .context("Invalid endpoint URL")?
            .connect_timeout(CONNECT_TIMEOUT)
            .timeout(REQUEST_TIMEOUT)
            .tcp_keepalive(Some(Duration::from_secs(30)));

        match timeout(CONNECT_TIMEOUT, endpoint.connect()).await {
            Ok(Ok(channel)) => {
                *self.channel.write() = Some(channel);
                *self.state.write() = PublisherState::Connected;
                metrics::set_connection_state(&self.endpoint, true);
                info!("Connected to Wotan");
                Ok(())
            }
            Ok(Err(e)) => {
                *self.state.write() = PublisherState::Disconnected;
                metrics::set_connection_state(&self.endpoint, false);
                Err(anyhow::anyhow!("Connection failed: {}", e))
            }
            Err(_) => {
                *self.state.write() = PublisherState::Disconnected;
                metrics::set_connection_state(&self.endpoint, false);
                Err(anyhow::anyhow!("Connection timeout"))
            }
        }
    }

    /// Reconnect with exponential backoff
    async fn reconnect(&self) -> Result<()> {
        let mut backoff = BASE_RETRY_BACKOFF;
        let mut attempts = 0;

        while attempts < MAX_RETRIES {
            attempts += 1;
            warn!(
                attempt = attempts,
                backoff_ms = backoff.as_millis(),
                "Attempting to reconnect to Wotan"
            );

            tokio::time::sleep(backoff).await;

            match self.connect().await {
                Ok(()) => {
                    self.stats.reconnections.fetch_add(1, Ordering::Relaxed);
                    return Ok(());
                }
                Err(e) => {
                    error!(
                        attempt = attempts,
                        error = %e,
                        "Reconnection attempt failed"
                    );
                }
            }

            // Exponential backoff with jitter
            backoff = (backoff * 2).min(MAX_RETRY_BACKOFF);
            let jitter = Duration::from_millis(rand_jitter(100));
            backoff += jitter;
        }

        Err(anyhow::anyhow!(
            "Failed to reconnect after {} attempts",
            MAX_RETRIES
        ))
    }

    /// Get current state
    pub fn state(&self) -> PublisherState {
        *self.state.read()
    }

    /// Check if connected
    pub fn is_connected(&self) -> bool {
        self.state() == PublisherState::Connected && self.channel.read().is_some()
    }

    /// Convert Event to protobuf TraceEvent
    fn event_to_proto(event: &Event) -> wotan_proto::TraceEvent {
        let event_type = match event.event_type {
            EventType::Packet => wotan_proto::EventType::Packet,
            EventType::Syscall => wotan_proto::EventType::Syscall,
            EventType::Latency => wotan_proto::EventType::Latency,
            EventType::Process => wotan_proto::EventType::Process,
            EventType::FileSystem => wotan_proto::EventType::FileSystem,
            EventType::Memory => wotan_proto::EventType::Memory,
            EventType::Custom => wotan_proto::EventType::Custom,
        };

        // Serialize event data to JSON for flexibility
        let data = serde_json::to_vec(&event.data).unwrap_or_default();

        let raw = event
            .raw
            .as_ref()
            .map(|b| b.to_vec())
            .unwrap_or_default();

        wotan_proto::TraceEvent {
            event_type: event_type as i32,
            timestamp_ns: event.timestamp_ns,
            cpu: event.cpu as u32,
            pid: event.pid,
            tid: event.tid,
            comm: event.comm.clone(),
            data,
            raw,
        }
    }

    /// Publish a batch of events
    async fn publish_batch(&self, batch: EventBatch) -> Result<u64> {
        if batch.is_empty() {
            return Ok(0);
        }

        let start = Instant::now();
        let batch_len = batch.len();

        // Convert events to protobuf
        let events: Vec<wotan_proto::TraceEvent> =
            batch.events.iter().map(Self::event_to_proto).collect();

        let sequence = self.sequence.fetch_add(1, Ordering::SeqCst);

        let proto_batch = wotan_proto::TraceEventBatch {
            collector_id: self.collector_id.clone(),
            sequence,
            events,
            first_timestamp_ns: batch.first_timestamp,
            last_timestamp_ns: batch.last_timestamp,
            compression: if self.compression { 1 } else { 0 },
        };

        // Encode the batch
        let mut buf = Vec::with_capacity(proto_batch.encoded_len());
        proto_batch
            .encode(&mut buf)
            .context("Failed to encode batch")?;

        // Compress if enabled and beneficial
        let (payload, compressed) = if self.compression && buf.len() > 1024 {
            match compress_gzip(&buf) {
                Ok(compressed) if compressed.len() < buf.len() => (compressed, true),
                _ => (buf, false),
            }
        } else {
            (buf, false)
        };

        trace!(
            batch_size = batch_len,
            payload_bytes = payload.len(),
            compressed = compressed,
            sequence = sequence,
            "Publishing batch"
        );

        // Get channel
        let channel = {
            let guard = self.channel.read();
            guard.clone()
        };

        let channel = match channel {
            Some(c) => c,
            None => {
                self.reconnect().await?;
                self.channel
                    .read()
                    .clone()
                    .ok_or_else(|| anyhow::anyhow!("No channel after reconnect"))?
            }
        };

        // Make the gRPC call
        // In a real implementation, you'd use the generated client stub
        // For now, we'll simulate the call
        let result = self.do_publish(channel, payload, sequence).await;

        let latency_us = start.elapsed().as_micros() as u64;
        self.stats
            .last_publish_latency_us
            .store(latency_us, Ordering::Relaxed);

        match result {
            Ok(accepted) => {
                metrics::record_events_published(accepted as usize, true);
                metrics::record_publish_latency(latency_us as f64, true);
                metrics::record_batch_size(batch_len);

                self.stats
                    .events_published
                    .fetch_add(accepted, Ordering::Relaxed);
                self.stats.batches_published.fetch_add(1, Ordering::Relaxed);

                debug!(
                    accepted = accepted,
                    latency_us = latency_us,
                    "Batch published successfully"
                );

                Ok(accepted)
            }
            Err(e) => {
                metrics::record_events_published(0, false);
                metrics::record_publish_latency(latency_us as f64, false);

                self.stats.publish_errors.fetch_add(1, Ordering::Relaxed);

                // Check if we need to reconnect
                if matches!(
                    e.downcast_ref::<tonic::Status>(),
                    Some(s) if s.code() == tonic::Code::Unavailable
                ) {
                    warn!("Wotan unavailable, scheduling reconnection");
                    *self.state.write() = PublisherState::Disconnected;
                    *self.channel.write() = None;
                    metrics::set_connection_state(&self.endpoint, false);
                }

                Err(e)
            }
        }
    }

    /// Actually perform the publish (simulated gRPC call)
    async fn do_publish(
        &self,
        _channel: Channel,
        payload: Vec<u8>,
        _sequence: u64,
    ) -> Result<u64> {
        // In a real implementation, this would use the generated gRPC client
        // For demonstration, we decode the batch to get the count

        // Simulate network latency
        tokio::time::sleep(Duration::from_micros(100)).await;

        // Decode to get event count (in real impl, server returns this)
        let batch = wotan_proto::TraceEventBatch::decode(&payload[..])
            .context("Failed to decode batch")?;

        Ok(batch.events.len() as u64)
    }

    /// Run the publisher loop
    pub async fn run(
        &self,
        event_rx: Receiver<Event>,
        shutdown: Arc<AtomicBool>,
    ) -> Result<()> {
        info!("Publisher starting");

        let mut batch = EventBatch::with_capacity(self.batch_size);
        let mut last_flush = Instant::now();

        while !shutdown.load(Ordering::Relaxed) {
            // Try to receive events with timeout
            match event_rx.recv_timeout(Duration::from_millis(1)) {
                Ok(event) => {
                    batch.push(event);

                    // Check if we should flush
                    let should_flush = batch.len() >= self.batch_size
                        || last_flush.elapsed() >= self.batch_timeout;

                    if should_flush {
                        self.flush_batch(&mut batch).await;
                        last_flush = Instant::now();
                    }
                }
                Err(crossbeam::channel::RecvTimeoutError::Timeout) => {
                    // Check if we have pending events and timeout elapsed
                    if !batch.is_empty() && last_flush.elapsed() >= self.batch_timeout {
                        self.flush_batch(&mut batch).await;
                        last_flush = Instant::now();
                    }

                    // Update queue depth metric
                    metrics::set_queue_depth(event_rx.len());
                }
                Err(crossbeam::channel::RecvTimeoutError::Disconnected) => {
                    info!("Event channel disconnected, flushing remaining events");
                    if !batch.is_empty() {
                        self.flush_batch(&mut batch).await;
                    }
                    break;
                }
            }
        }

        // Final flush
        if !batch.is_empty() {
            self.flush_batch(&mut batch).await;
        }

        *self.state.write() = PublisherState::ShuttingDown;
        info!("Publisher stopped");

        Ok(())
    }

    /// Flush the current batch
    async fn flush_batch(&self, batch: &mut EventBatch) {
        if batch.is_empty() {
            return;
        }

        let batch_to_send = std::mem::replace(batch, EventBatch::with_capacity(self.batch_size));

        match self.publish_batch(batch_to_send).await {
            Ok(_) => {}
            Err(e) => {
                error!(error = %e, "Failed to publish batch");
            }
        }
    }

    /// Get publisher statistics
    pub fn stats(&self) -> &Arc<PublisherStats> {
        &self.stats
    }
}

/// Compress data with gzip
fn compress_gzip(data: &[u8]) -> Result<Vec<u8>> {
    use std::io::Write;

    let mut encoder = flate2::write::GzEncoder::new(Vec::new(), flate2::Compression::fast());
    encoder.write_all(data)?;
    Ok(encoder.finish()?)
}

/// Simple random jitter (0 to max_ms)
fn rand_jitter(max_ms: u64) -> u64 {
    use std::time::SystemTime;
    let seed = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos() as u64;
    seed % max_ms
}

/// Flow event for publishing flow state changes
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FlowEvent {
    /// Flow 5-tuple hash
    pub flow_hash: u64,
    /// Source IP
    pub src_ip: String,
    /// Destination IP
    pub dst_ip: String,
    /// Source port
    pub src_port: u16,
    /// Destination port
    pub dst_port: u16,
    /// Protocol
    pub protocol: u8,
    /// Event type (new, update, close)
    pub event_type: FlowEventType,
    /// Timestamp
    pub timestamp_ns: u64,
    /// Packets in
    pub packets_in: u64,
    /// Packets out
    pub packets_out: u64,
    /// Bytes in
    pub bytes_in: u64,
    /// Bytes out
    pub bytes_out: u64,
    /// Connection state
    pub state: String,
}

/// Flow event type
#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
pub enum FlowEventType {
    New,
    Update,
    Close,
    Timeout,
}

/// Async publisher that uses tokio channels
pub struct AsyncWotanPublisher {
    /// Inner publisher
    inner: Arc<WotanPublisher>,
    /// Event sender
    event_tx: mpsc::UnboundedSender<Event>,
}

impl AsyncWotanPublisher {
    /// Create a new async publisher
    pub async fn new(endpoint: &str, batch_size: usize, batch_timeout: Duration) -> Result<Self> {
        let inner = WotanPublisher::new(endpoint, batch_size, batch_timeout).await?;
        let (event_tx, mut event_rx) = mpsc::unbounded_channel::<Event>();

        let publisher = Arc::new(inner);
        let publisher_clone = Arc::clone(&publisher);

        // Spawn the publishing task
        tokio::spawn(async move {
            let mut batch = EventBatch::with_capacity(batch_size);
            let mut last_flush = Instant::now();

            loop {
                tokio::select! {
                    event = event_rx.recv() => {
                        match event {
                            Some(e) => {
                                batch.push(e);
                                if batch.len() >= batch_size {
                                    if let Err(e) = publisher_clone.publish_batch(std::mem::replace(
                                        &mut batch,
                                        EventBatch::with_capacity(batch_size),
                                    )).await {
                                        error!(error = %e, "Failed to publish batch");
                                    }
                                    last_flush = Instant::now();
                                }
                            }
                            None => {
                                // Channel closed
                                if !batch.is_empty() {
                                    let _ = publisher_clone.publish_batch(batch).await;
                                }
                                break;
                            }
                        }
                    }
                    _ = tokio::time::sleep(batch_timeout) => {
                        if !batch.is_empty() && last_flush.elapsed() >= batch_timeout {
                            if let Err(e) = publisher_clone.publish_batch(std::mem::replace(
                                &mut batch,
                                EventBatch::with_capacity(batch_size),
                            )).await {
                                error!(error = %e, "Failed to publish batch on timeout");
                            }
                            last_flush = Instant::now();
                        }
                    }
                }
            }
        });

        Ok(Self {
            inner: publisher,
            event_tx,
        })
    }

    /// Send an event for publishing
    pub fn send(&self, event: Event) -> Result<()> {
        self.event_tx
            .send(event)
            .map_err(|_| anyhow::anyhow!("Publisher channel closed"))
    }

    /// Check if connected
    pub fn is_connected(&self) -> bool {
        self.inner.is_connected()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_publisher_state() {
        assert_eq!(PublisherState::Disconnected, PublisherState::Disconnected);
        assert_ne!(PublisherState::Connected, PublisherState::Disconnected);
    }

    #[test]
    fn test_publisher_stats() {
        let stats = PublisherStats::new();
        stats.events_published.store(100, Ordering::SeqCst);
        assert_eq!(stats.events_published.load(Ordering::SeqCst), 100);
    }

    #[test]
    fn test_event_to_proto() {
        let event = Event::new(
            EventType::Packet,
            0,
            1000000,
            1234,
            1234,
            "test".to_string(),
            EventData::Raw(vec![1, 2, 3]),
        );

        let proto = WotanPublisher::event_to_proto(&event);
        assert_eq!(proto.event_type, wotan_proto::EventType::Packet as i32);
        assert_eq!(proto.pid, 1234);
        assert_eq!(proto.comm, "test");
    }

    #[test]
    fn test_flow_event_type() {
        let event_type = FlowEventType::New;
        let json = serde_json::to_string(&event_type).unwrap();
        assert!(json.contains("New"));
    }

    #[test]
    fn test_compress_gzip() {
        let data = b"Hello, World! This is some test data that should compress.";
        let compressed = compress_gzip(data).unwrap();
        // Compressed data should exist (may be larger for small inputs)
        assert!(!compressed.is_empty());
    }

    #[test]
    fn test_rand_jitter() {
        let jitter = rand_jitter(100);
        assert!(jitter < 100);
    }
}
