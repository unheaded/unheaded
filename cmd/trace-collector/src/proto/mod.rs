//! Protocol Buffer definitions for the trace-collector.
//!
//! THE WHISPERING VOID speaks in protocol buffers, a language understood
//! by all services in the Unheaded Kingdom.
//!
//! These definitions match the Busboy service protobuf schema and enable
//! efficient serialization of trace events.

use prost::Message;
use serde::{Deserialize, Serialize};

// ============================================================================
// TRACE EVENT DEFINITIONS
// ============================================================================

/// TraceId uniquely identifies a distributed trace across services.
/// Format: 128-bit UUID as two u64s (high and low)
#[derive(Clone, PartialEq, prost::Message, Serialize, Deserialize)]
pub struct TraceId {
    /// High 64 bits of trace ID
    #[prost(uint64, tag = "1")]
    pub high: u64,
    /// Low 64 bits of trace ID
    #[prost(uint64, tag = "2")]
    pub low: u64,
}

impl TraceId {
    /// Create a new random trace ID
    pub fn new_random() -> Self {
        use std::time::SystemTime;
        let now = SystemTime::now()
            .duration_since(SystemTime::UNIX_EPOCH)
            .unwrap_or_default();

        // Simple random using timestamp and process ID
        let high = now.as_nanos() as u64;
        let low = (std::process::id() as u64) << 32 | (now.subsec_nanos() as u64);

        Self { high, low }
    }

    /// Create from hex string (32 characters)
    pub fn from_hex(hex: &str) -> Option<Self> {
        if hex.len() != 32 {
            return None;
        }
        let high = u64::from_str_radix(&hex[0..16], 16).ok()?;
        let low = u64::from_str_radix(&hex[16..32], 16).ok()?;
        Some(Self { high, low })
    }

    /// Convert to hex string
    pub fn to_hex(&self) -> String {
        format!("{:016x}{:016x}", self.high, self.low)
    }

    /// Check if trace ID is zero/empty
    pub fn is_zero(&self) -> bool {
        self.high == 0 && self.low == 0
    }
}

impl std::fmt::Display for TraceId {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.to_hex())
    }
}

// Default is derived by prost::Message

/// SpanId identifies a span within a trace
#[derive(Clone, PartialEq, prost::Message, Serialize, Deserialize)]
pub struct SpanId {
    /// 64-bit span identifier
    #[prost(uint64, tag = "1")]
    pub id: u64,
}

impl SpanId {
    /// Create a new random span ID
    pub fn new_random() -> Self {
        use std::time::SystemTime;
        let now = SystemTime::now()
            .duration_since(SystemTime::UNIX_EPOCH)
            .unwrap_or_default();
        Self {
            id: now.as_nanos() as u64,
        }
    }

    /// Convert to hex string
    pub fn to_hex(&self) -> String {
        format!("{:016x}", self.id)
    }
}

// Default is derived by prost::Message

/// FlowKey uniquely identifies a network flow (5-tuple + trace context)
#[derive(Clone, PartialEq, prost::Message, Serialize, Deserialize)]
pub struct FlowKey {
    /// Source IP address (IPv4 as u32, IPv6 as 4xu32)
    #[prost(bytes = "vec", tag = "1")]
    pub src_ip: Vec<u8>,
    /// Destination IP address
    #[prost(bytes = "vec", tag = "2")]
    pub dst_ip: Vec<u8>,
    /// Source port
    #[prost(uint32, tag = "3")]
    pub src_port: u32,
    /// Destination port
    #[prost(uint32, tag = "4")]
    pub dst_port: u32,
    /// Protocol (TCP=6, UDP=17)
    #[prost(uint32, tag = "5")]
    pub protocol: u32,
    /// Associated trace ID (if known)
    #[prost(message, optional, tag = "6")]
    pub trace_id: Option<TraceId>,
}

impl FlowKey {
    /// Create a flow key from IPv4 addresses
    pub fn from_ipv4(
        src_ip: std::net::Ipv4Addr,
        dst_ip: std::net::Ipv4Addr,
        src_port: u16,
        dst_port: u16,
        protocol: u8,
    ) -> Self {
        Self {
            src_ip: src_ip.octets().to_vec(),
            dst_ip: dst_ip.octets().to_vec(),
            src_port: src_port as u32,
            dst_port: dst_port as u32,
            protocol: protocol as u32,
            trace_id: None,
        }
    }

    /// Compute a hash for this flow key
    pub fn hash(&self) -> u64 {
        use std::hash::{Hash, Hasher};
        let mut hasher = std::collections::hash_map::DefaultHasher::new();
        self.src_ip.hash(&mut hasher);
        self.dst_ip.hash(&mut hasher);
        self.src_port.hash(&mut hasher);
        self.dst_port.hash(&mut hasher);
        self.protocol.hash(&mut hasher);
        hasher.finish()
    }
}

// ============================================================================
// EVENT TYPE DEFINITIONS
// ============================================================================

/// Event type enumeration matching eBPF-side definitions
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, prost::Enumeration)]
#[repr(i32)]
pub enum EventType {
    /// Unknown event type
    Unknown = 0,
    /// Network packet event from XDP/TC
    Packet = 1,
    /// System call event
    Syscall = 2,
    /// Latency probe event
    Latency = 3,
    /// Process lifecycle event
    Process = 4,
    /// File system event
    FileSystem = 5,
    /// Memory event
    Memory = 6,
    /// Flow state change event
    Flow = 7,
    /// Custom/user-defined event
    Custom = 255,
}

impl EventType {
    /// Get string name for the event type
    pub fn as_str(&self) -> &'static str {
        match self {
            EventType::Unknown => "unknown",
            EventType::Packet => "packet",
            EventType::Syscall => "syscall",
            EventType::Latency => "latency",
            EventType::Process => "process",
            EventType::FileSystem => "filesystem",
            EventType::Memory => "memory",
            EventType::Flow => "flow",
            EventType::Custom => "custom",
        }
    }
}

// ============================================================================
// TRACE EVENT MESSAGES
// ============================================================================

/// A single trace event from eBPF
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
    /// Associated trace ID (if known)
    #[prost(message, optional, tag = "7")]
    pub trace_id: Option<TraceId>,
    /// Parent span ID (if known)
    #[prost(message, optional, tag = "8")]
    pub parent_span_id: Option<SpanId>,
    /// Span ID for this event
    #[prost(message, optional, tag = "9")]
    pub span_id: Option<SpanId>,
    /// Serialized event data (type-specific, JSON or binary)
    #[prost(bytes = "vec", tag = "10")]
    pub data: Vec<u8>,
    /// Raw event bytes from eBPF (for forwarding)
    #[prost(bytes = "vec", tag = "11")]
    pub raw: Vec<u8>,
    /// Event flags
    #[prost(uint32, tag = "12")]
    pub flags: u32,
    /// Hostname where event originated
    #[prost(string, tag = "13")]
    pub hostname: String,
}

/// Batch of trace events for efficient publishing
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
    /// First event timestamp in batch
    #[prost(uint64, tag = "4")]
    pub first_timestamp_ns: u64,
    /// Last event timestamp in batch
    #[prost(uint64, tag = "5")]
    pub last_timestamp_ns: u64,
    /// Compression (0=none, 1=gzip, 2=zstd)
    #[prost(uint32, tag = "6")]
    pub compression: u32,
    /// Total events in batch (before any filtering)
    #[prost(uint32, tag = "7")]
    pub total_events: u32,
}

// ============================================================================
// LATENCY DATA
// ============================================================================

/// Latency measurement data
#[derive(Clone, PartialEq, prost::Message, Serialize, Deserialize)]
pub struct LatencyData {
    /// Probe name (function, tracepoint, etc.)
    #[prost(string, tag = "1")]
    pub probe_name: String,
    /// Latency in nanoseconds
    #[prost(uint64, tag = "2")]
    pub latency_ns: u64,
    /// Entry timestamp
    #[prost(uint64, tag = "3")]
    pub entry_ts: u64,
    /// Exit timestamp
    #[prost(uint64, tag = "4")]
    pub exit_ts: u64,
    /// Probe type (0=kprobe, 1=uprobe, 2=tracepoint, 3=usdt)
    #[prost(uint32, tag = "5")]
    pub probe_type: u32,
    /// Stack trace ID (if captured)
    #[prost(int32, tag = "6")]
    pub stack_id: i32,
}

// ============================================================================
// FLOW DATA
// ============================================================================

/// Network flow state
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, prost::Enumeration)]
#[repr(i32)]
pub enum FlowState {
    Unknown = 0,
    New = 1,
    Established = 2,
    Closing = 3,
    Closed = 4,
    Timeout = 5,
}

/// Flow event data
#[derive(Clone, PartialEq, prost::Message, Serialize, Deserialize)]
pub struct FlowData {
    /// Flow key
    #[prost(message, optional, tag = "1")]
    pub flow_key: Option<FlowKey>,
    /// Flow state
    #[prost(enumeration = "FlowState", tag = "2")]
    pub state: i32,
    /// Packets received
    #[prost(uint64, tag = "3")]
    pub packets_rx: u64,
    /// Packets transmitted
    #[prost(uint64, tag = "4")]
    pub packets_tx: u64,
    /// Bytes received
    #[prost(uint64, tag = "5")]
    pub bytes_rx: u64,
    /// Bytes transmitted
    #[prost(uint64, tag = "6")]
    pub bytes_tx: u64,
    /// Flow start time
    #[prost(uint64, tag = "7")]
    pub start_ts: u64,
    /// Last activity time
    #[prost(uint64, tag = "8")]
    pub last_activity_ts: u64,
    /// RTT samples (nanoseconds)
    #[prost(uint64, repeated, tag = "9")]
    pub rtt_samples_ns: Vec<u64>,
}

// ============================================================================
// RPC MESSAGES
// ============================================================================

/// Publish request
#[derive(Clone, PartialEq, prost::Message)]
pub struct PublishRequest {
    /// Batch of events to publish
    #[prost(message, optional, tag = "1")]
    pub batch: Option<TraceEventBatch>,
    /// Topic to publish to (default: "trace.events")
    #[prost(string, tag = "2")]
    pub topic: String,
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
    /// Server-side sequence number
    #[prost(uint64, tag = "5")]
    pub server_sequence: u64,
}

/// Subscribe request
#[derive(Clone, PartialEq, prost::Message)]
pub struct SubscribeRequest {
    /// Topic pattern to subscribe to (supports wildcards)
    #[prost(string, tag = "1")]
    pub topic_pattern: String,
    /// Subscriber ID
    #[prost(string, tag = "2")]
    pub subscriber_id: String,
    /// Start from sequence number (0 = latest)
    #[prost(uint64, tag = "3")]
    pub from_sequence: u64,
}

/// Health check request
#[derive(Clone, PartialEq, prost::Message)]
pub struct HealthRequest {}

/// Health check response
#[derive(Clone, PartialEq, prost::Message)]
pub struct HealthResponse {
    /// Is service healthy
    #[prost(bool, tag = "1")]
    pub healthy: bool,
    /// Service version
    #[prost(string, tag = "2")]
    pub version: String,
    /// Uptime in seconds
    #[prost(uint64, tag = "3")]
    pub uptime_seconds: u64,
    /// Current message rate (events/second)
    #[prost(double, tag = "4")]
    pub message_rate: f64,
}

// ============================================================================
// HELPER IMPLEMENTATIONS
// ============================================================================

impl TraceEvent {
    /// Create a new trace event
    pub fn new(event_type: EventType, pid: u32, tid: u32, comm: String) -> Self {
        Self {
            event_type: event_type as i32,
            timestamp_ns: std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap_or_default()
                .as_nanos() as u64,
            cpu: 0,
            pid,
            tid,
            comm,
            trace_id: None,
            parent_span_id: None,
            span_id: None,
            data: Vec::new(),
            raw: Vec::new(),
            flags: 0,
            hostname: hostname::get()
                .map(|h| h.to_string_lossy().to_string())
                .unwrap_or_default(),
        }
    }

    /// Set trace context
    pub fn with_trace_context(mut self, trace_id: TraceId, span_id: SpanId) -> Self {
        self.trace_id = Some(trace_id);
        self.span_id = Some(span_id);
        self
    }

    /// Set event data
    pub fn with_data(mut self, data: Vec<u8>) -> Self {
        self.data = data;
        self
    }

    /// Get encoded size
    pub fn encoded_size(&self) -> usize {
        self.encoded_len()
    }
}

impl TraceEventBatch {
    /// Create a new empty batch
    pub fn new(collector_id: String) -> Self {
        Self {
            collector_id,
            sequence: 0,
            events: Vec::new(),
            first_timestamp_ns: 0,
            last_timestamp_ns: 0,
            compression: 0,
            total_events: 0,
        }
    }

    /// Add an event to the batch
    pub fn add_event(&mut self, event: TraceEvent) {
        if self.events.is_empty() {
            self.first_timestamp_ns = event.timestamp_ns;
        }
        self.last_timestamp_ns = event.timestamp_ns;
        self.total_events += 1;
        self.events.push(event);
    }

    /// Get batch duration in nanoseconds
    pub fn duration_ns(&self) -> u64 {
        self.last_timestamp_ns.saturating_sub(self.first_timestamp_ns)
    }

    /// Check if batch is empty
    pub fn is_empty(&self) -> bool {
        self.events.is_empty()
    }

    /// Get number of events
    pub fn len(&self) -> usize {
        self.events.len()
    }
}

// ============================================================================
// COMPRESSION
// ============================================================================

/// Compression type
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(u32)]
pub enum Compression {
    None = 0,
    Gzip = 1,
    Zstd = 2,
}

impl From<u32> for Compression {
    fn from(v: u32) -> Self {
        match v {
            1 => Compression::Gzip,
            2 => Compression::Zstd,
            _ => Compression::None,
        }
    }
}

// ============================================================================
// TESTS
// ============================================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_trace_id() {
        let id = TraceId::new_random();
        assert!(!id.is_zero());

        let hex = id.to_hex();
        assert_eq!(hex.len(), 32);

        let parsed = TraceId::from_hex(&hex).unwrap();
        assert_eq!(parsed.high, id.high);
        assert_eq!(parsed.low, id.low);
    }

    #[test]
    fn test_trace_id_from_hex() {
        let hex = "0123456789abcdef0123456789abcdef";
        let id = TraceId::from_hex(hex).unwrap();
        assert_eq!(id.to_hex(), hex);
    }

    #[test]
    fn test_flow_key_hash() {
        let key1 = FlowKey::from_ipv4(
            "10.0.0.1".parse().unwrap(),
            "10.0.0.2".parse().unwrap(),
            80,
            12345,
            6,
        );
        let key2 = FlowKey::from_ipv4(
            "10.0.0.1".parse().unwrap(),
            "10.0.0.2".parse().unwrap(),
            80,
            12345,
            6,
        );
        let key3 = FlowKey::from_ipv4(
            "10.0.0.1".parse().unwrap(),
            "10.0.0.3".parse().unwrap(),
            80,
            12345,
            6,
        );

        assert_eq!(key1.hash(), key2.hash());
        assert_ne!(key1.hash(), key3.hash());
    }

    #[test]
    fn test_event_type() {
        assert_eq!(EventType::Packet.as_str(), "packet");
        assert_eq!(EventType::Syscall.as_str(), "syscall");
    }

    #[test]
    fn test_trace_event() {
        let event = TraceEvent::new(EventType::Packet, 1234, 1234, "test".to_string());
        assert_eq!(event.pid, 1234);
        assert!(event.timestamp_ns > 0);
    }

    #[test]
    fn test_trace_event_batch() {
        let mut batch = TraceEventBatch::new("test-collector".to_string());
        assert!(batch.is_empty());

        let event = TraceEvent::new(EventType::Packet, 1234, 1234, "test".to_string());
        batch.add_event(event);

        assert_eq!(batch.len(), 1);
        assert!(!batch.is_empty());
    }

    #[test]
    fn test_encode_decode() {
        let event = TraceEvent::new(EventType::Latency, 5678, 5678, "nginx".to_string());

        let mut buf = Vec::new();
        event.encode(&mut buf).unwrap();

        let decoded = TraceEvent::decode(&buf[..]).unwrap();
        assert_eq!(decoded.pid, event.pid);
        assert_eq!(decoded.comm, event.comm);
    }
}
