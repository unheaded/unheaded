//! Unheaded Kingdom - Common Types for eBPF Programs
//!
//! THE WHISPERING VOID speaks through these types, shared between
//! kernel space (eBPF) and user space.
//!
//! All types here are #[repr(C)] for stable ABI across the kernel boundary.

#![no_std]

/// 128-bit Trace ID for distributed tracing across the kingdom.
/// Follows W3C Trace Context format: 16 bytes = 128 bits.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct TraceId {
    pub high: u64,
    pub low: u64,
}

impl TraceId {
    pub const fn new(high: u64, low: u64) -> Self {
        Self { high, low }
    }

    pub const fn zero() -> Self {
        Self { high: 0, low: 0 }
    }

    pub const fn is_zero(&self) -> bool {
        self.high == 0 && self.low == 0
    }

    /// Create from raw bytes (big-endian).
    pub const fn from_bytes(bytes: [u8; 16]) -> Self {
        let high = u64::from_be_bytes([
            bytes[0], bytes[1], bytes[2], bytes[3],
            bytes[4], bytes[5], bytes[6], bytes[7],
        ]);
        let low = u64::from_be_bytes([
            bytes[8], bytes[9], bytes[10], bytes[11],
            bytes[12], bytes[13], bytes[14], bytes[15],
        ]);
        Self { high, low }
    }

    /// Convert to raw bytes (big-endian).
    pub const fn to_bytes(&self) -> [u8; 16] {
        let h = self.high.to_be_bytes();
        let l = self.low.to_be_bytes();
        [
            h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7],
            l[0], l[1], l[2], l[3], l[4], l[5], l[6], l[7],
        ]
    }
}

/// 64-bit Span ID for individual operations within a trace.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct SpanId(pub u64);

/// 5-tuple flow key for connection tracking.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct FlowKey {
    pub src_addr: u32,      // IPv4 source address (network byte order)
    pub dst_addr: u32,      // IPv4 destination address (network byte order)
    pub src_port: u16,      // Source port (network byte order)
    pub dst_port: u16,      // Destination port (network byte order)
    pub protocol: u8,       // IP protocol (TCP=6, UDP=17)
    pub _pad: [u8; 3],      // Alignment padding
}

impl FlowKey {
    /// Compute a simple hash for the flow key.
    pub const fn hash(&self) -> u32 {
        // Simple FNV-1a style hash
        let mut h: u32 = 2166136261;
        h = h.wrapping_mul(16777619) ^ self.src_addr;
        h = h.wrapping_mul(16777619) ^ self.dst_addr;
        h = h.wrapping_mul(16777619) ^ (self.src_port as u32);
        h = h.wrapping_mul(16777619) ^ (self.dst_port as u32);
        h = h.wrapping_mul(16777619) ^ (self.protocol as u32);
        h
    }
}

/// Connection state for flow tracking.
#[repr(u8)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum ConnectionState {
    #[default]
    Unknown = 0,
    SynSent = 1,
    SynReceived = 2,
    Established = 3,
    FinWait1 = 4,
    FinWait2 = 5,
    CloseWait = 6,
    Closing = 7,
    LastAck = 8,
    TimeWait = 9,
    Closed = 10,
}

/// Flow tracking state stored in BPF maps.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct FlowState {
    pub trace_id: TraceId,          // Associated trace ID
    pub start_ns: u64,              // Connection start timestamp (nanoseconds)
    pub last_seen_ns: u64,          // Last packet timestamp
    pub packets_in: u64,            // Inbound packet count
    pub packets_out: u64,           // Outbound packet count
    pub bytes_in: u64,              // Inbound bytes
    pub bytes_out: u64,             // Outbound bytes
    pub state: ConnectionState,     // Connection state
    pub _pad: [u8; 7],              // Alignment
}

/// Packet event sent to userspace via ring buffer.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct PacketEvent {
    pub timestamp_ns: u64,          // Kernel timestamp
    pub trace_id: TraceId,          // Trace ID from packet
    pub flow_key: FlowKey,          // 5-tuple
    pub packet_len: u32,            // Total packet length
    pub action: PacketAction,       // What action was taken
    pub direction: Direction,       // Ingress or egress
    pub _pad: [u8; 2],              // Alignment
}

/// Action taken on a packet.
#[repr(u8)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum PacketAction {
    #[default]
    Pass = 0,
    Drop = 1,
    Marked = 2,         // Trace ID was written
    Extracted = 3,      // Trace ID was read
    Redirect = 4,
}

/// Packet direction.
#[repr(u8)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Direction {
    #[default]
    Ingress = 0,
    Egress = 1,
}

/// Flow event sent to userspace via ring buffer.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct FlowEvent {
    pub timestamp_ns: u64,
    pub flow_key: FlowKey,
    pub state: FlowState,
    pub event_type: FlowEventType,
    pub _pad: [u8; 7],
}

/// Type of flow event.
#[repr(u8)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum FlowEventType {
    #[default]
    New = 0,
    StateChange = 1,
    Expired = 2,
    Update = 3,
}

/// Latency measurement event from kprobes.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct LatencyEvent {
    pub timestamp_ns: u64,          // When measurement was taken
    pub trace_id: TraceId,          // Associated trace (if known)
    pub pid: u32,                   // Process ID
    pub tid: u32,                   // Thread ID
    pub latency_ns: u64,            // Measured latency in nanoseconds
    pub operation: LatencyOperation,
    pub _pad: [u8; 7],
}

/// Type of latency operation being measured.
#[repr(u8)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum LatencyOperation {
    #[default]
    TcpSend = 0,
    TcpRecv = 1,
    TcpConnect = 2,
    TcpAccept = 3,
}

/// Syscall audit event from raw tracepoints.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct SyscallEvent {
    pub timestamp_ns: u64,
    pub pid: u32,
    pub tid: u32,
    pub uid: u32,
    pub gid: u32,
    pub syscall_nr: i64,            // Syscall number
    pub args: [u64; 6],             // Syscall arguments
    pub ret: i64,                   // Return value (if exit event)
    pub comm: [u8; 16],             // Process name
    pub event_type: SyscallEventType,
    pub _pad: [u8; 7],
}

/// Syscall event type.
#[repr(u8)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum SyscallEventType {
    #[default]
    Enter = 0,
    Exit = 1,
}

/// IP header option for trace context propagation.
/// Uses IP option type 0x1F (experimental).
pub const TRACE_OPTION_TYPE: u8 = 0x1F;
pub const TRACE_OPTION_LEN: u8 = 18; // 2 (type+len) + 16 (trace_id)

/// TCP option for trace context (experimental).
pub const TCP_TRACE_OPTION_KIND: u8 = 253; // Experimental
pub const TCP_TRACE_OPTION_LEN: u8 = 18;

/// Maximum number of flows to track.
pub const MAX_FLOWS: u32 = 65536;

/// Maximum entries in latency tracking map.
pub const MAX_LATENCY_ENTRIES: u32 = 8192;

/// Ring buffer size (must be power of 2).
pub const RING_BUFFER_SIZE: u32 = 256 * 1024; // 256KB

/// Flow expiration timeout in nanoseconds (30 seconds).
pub const FLOW_TIMEOUT_NS: u64 = 30_000_000_000;

/// Ethernet header size.
pub const ETH_HLEN: usize = 14;

/// IPv4 header minimum size.
pub const IPV4_MIN_HLEN: usize = 20;

/// TCP header minimum size.
pub const TCP_MIN_HLEN: usize = 20;

/// UDP header size.
pub const UDP_HLEN: usize = 8;

/// IP protocol numbers.
pub const IPPROTO_TCP: u8 = 6;
pub const IPPROTO_UDP: u8 = 17;

/// Ethernet types.
pub const ETH_P_IP: u16 = 0x0800;
pub const ETH_P_IPV6: u16 = 0x86DD;

/// TCP flags.
pub const TCP_FLAG_FIN: u8 = 0x01;
pub const TCP_FLAG_SYN: u8 = 0x02;
pub const TCP_FLAG_RST: u8 = 0x04;
pub const TCP_FLAG_PSH: u8 = 0x08;
pub const TCP_FLAG_ACK: u8 = 0x10;
pub const TCP_FLAG_URG: u8 = 0x20;
