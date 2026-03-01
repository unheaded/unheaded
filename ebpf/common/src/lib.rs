//! Unheaded Kingdom - Common Types for eBPF Programs
//!
//! THE WHISPERING VOID speaks through these types, shared between
//! kernel space (eBPF) and user space.
//!
//! All types here are #[repr(C)] for stable ABI across the kernel boundary.

#![cfg_attr(not(test), no_std)]

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
            bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5], bytes[6], bytes[7],
        ]);
        let low = u64::from_be_bytes([
            bytes[8], bytes[9], bytes[10], bytes[11], bytes[12], bytes[13], bytes[14], bytes[15],
        ]);
        Self { high, low }
    }

    /// Convert to raw bytes (big-endian).
    pub const fn to_bytes(&self) -> [u8; 16] {
        let h = self.high.to_be_bytes();
        let l = self.low.to_be_bytes();
        [
            h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7], l[0], l[1], l[2], l[3], l[4], l[5],
            l[6], l[7],
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
    pub src_addr: u32, // IPv4 source address (network byte order)
    pub dst_addr: u32, // IPv4 destination address (network byte order)
    pub src_port: u16, // Source port (network byte order)
    pub dst_port: u16, // Destination port (network byte order)
    pub protocol: u8,  // IP protocol (TCP=6, UDP=17)
    pub _pad: [u8; 3], // Alignment padding
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
    pub trace_id: TraceId,      // Associated trace ID
    pub start_ns: u64,          // Connection start timestamp (nanoseconds)
    pub last_seen_ns: u64,      // Last packet timestamp
    pub packets_in: u64,        // Inbound packet count
    pub packets_out: u64,       // Outbound packet count
    pub bytes_in: u64,          // Inbound bytes
    pub bytes_out: u64,         // Outbound bytes
    pub state: ConnectionState, // Connection state
    pub _pad: [u8; 7],          // Alignment
}

/// Packet event sent to userspace via ring buffer.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct PacketEvent {
    pub timestamp_ns: u64,    // Kernel timestamp
    pub trace_id: TraceId,    // Trace ID from packet
    pub flow_key: FlowKey,    // 5-tuple
    pub packet_len: u32,      // Total packet length
    pub action: PacketAction, // What action was taken
    pub direction: Direction, // Ingress or egress
    pub _pad: [u8; 2],        // Alignment
}

/// Action taken on a packet.
#[repr(u8)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum PacketAction {
    #[default]
    Pass = 0,
    Drop = 1,
    Marked = 2,    // Trace ID was written
    Extracted = 3, // Trace ID was read
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
    Anomaly = 4,
}

/// Latency measurement event from kprobes.
#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct LatencyEvent {
    pub timestamp_ns: u64, // When measurement was taken
    pub trace_id: TraceId, // Associated trace (if known)
    pub pid: u32,          // Process ID
    pub tid: u32,          // Thread ID
    pub latency_ns: u64,   // Measured latency in nanoseconds
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
    pub syscall_nr: i64, // Syscall number
    pub args: [u64; 6],  // Syscall arguments
    pub ret: i64,        // Return value (if exit event)
    pub comm: [u8; 16],  // Process name
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

#[cfg(test)]
mod tests {
    use super::*;

    // ── TraceId ──────────────────────────────────────────────

    #[test]
    fn test_trace_id_zero() {
        let z = TraceId::zero();
        assert!(z.is_zero());
        assert_eq!(z.high, 0);
        assert_eq!(z.low, 0);
    }

    #[test]
    fn test_trace_id_non_zero() {
        assert!(!TraceId::new(1, 0).is_zero());
        assert!(!TraceId::new(0, 1).is_zero());
        assert!(!TraceId::new(1, 1).is_zero());
    }

    #[test]
    fn test_trace_id_bytes_roundtrip() {
        let id = TraceId::new(0x0123456789abcdef, 0xfedcba9876543210);
        let bytes = id.to_bytes();
        let restored = TraceId::from_bytes(bytes);
        assert_eq!(id, restored);
    }

    #[test]
    fn test_trace_id_bytes_big_endian() {
        let id = TraceId::new(0x0102030405060708, 0x090a0b0c0d0e0f10);
        let bytes = id.to_bytes();
        assert_eq!(
            bytes,
            [
                0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e,
                0x0f, 0x10
            ]
        );
    }

    #[test]
    fn test_trace_id_zero_bytes_roundtrip() {
        let z = TraceId::zero();
        let bytes = z.to_bytes();
        assert_eq!(bytes, [0u8; 16]);
        let restored = TraceId::from_bytes(bytes);
        assert!(restored.is_zero());
    }

    #[test]
    fn test_trace_id_max_values() {
        let id = TraceId::new(u64::MAX, u64::MAX);
        let bytes = id.to_bytes();
        assert_eq!(bytes, [0xff; 16]);
        let restored = TraceId::from_bytes(bytes);
        assert_eq!(id, restored);
    }

    // ── FlowKey ──────────────────────────────────────────────

    #[test]
    fn test_flow_key_hash_deterministic() {
        let key = FlowKey {
            src_addr: 0x0a0a0a01,
            dst_addr: 0x0a0a0a02,
            src_port: 8080,
            dst_port: 443,
            protocol: IPPROTO_TCP,
            _pad: [0; 3],
        };
        let h1 = key.hash();
        let h2 = key.hash();
        assert_eq!(h1, h2);
    }

    #[test]
    fn test_flow_key_hash_differs_for_different_keys() {
        let key1 = FlowKey {
            src_addr: 0x0a0a0a01,
            dst_addr: 0x0a0a0a02,
            src_port: 8080,
            dst_port: 443,
            protocol: IPPROTO_TCP,
            _pad: [0; 3],
        };
        let key2 = FlowKey {
            src_addr: 0x0a0a0a02,
            dst_addr: 0x0a0a0a01,
            src_port: 443,
            dst_port: 8080,
            protocol: IPPROTO_TCP,
            _pad: [0; 3],
        };
        // Reversed src/dst should produce a different hash
        assert_ne!(key1.hash(), key2.hash());
    }

    #[test]
    fn test_flow_key_hash_protocol_matters() {
        let tcp_key = FlowKey {
            src_addr: 1,
            dst_addr: 2,
            src_port: 100,
            dst_port: 200,
            protocol: IPPROTO_TCP,
            _pad: [0; 3],
        };
        let udp_key = FlowKey {
            src_addr: 1,
            dst_addr: 2,
            src_port: 100,
            dst_port: 200,
            protocol: IPPROTO_UDP,
            _pad: [0; 3],
        };
        assert_ne!(tcp_key.hash(), udp_key.hash());
    }

    // ── ConnectionState ──────────────────────────────────────

    #[test]
    fn test_connection_state_repr_values() {
        assert_eq!(ConnectionState::Unknown as u8, 0);
        assert_eq!(ConnectionState::SynSent as u8, 1);
        assert_eq!(ConnectionState::SynReceived as u8, 2);
        assert_eq!(ConnectionState::Established as u8, 3);
        assert_eq!(ConnectionState::FinWait1 as u8, 4);
        assert_eq!(ConnectionState::FinWait2 as u8, 5);
        assert_eq!(ConnectionState::CloseWait as u8, 6);
        assert_eq!(ConnectionState::Closing as u8, 7);
        assert_eq!(ConnectionState::LastAck as u8, 8);
        assert_eq!(ConnectionState::TimeWait as u8, 9);
        assert_eq!(ConnectionState::Closed as u8, 10);
    }

    #[test]
    fn test_connection_state_default() {
        let state = ConnectionState::default();
        assert_eq!(state, ConnectionState::Unknown);
    }

    // ── Enum repr values ─────────────────────────────────────

    #[test]
    fn test_packet_action_repr_values() {
        assert_eq!(PacketAction::Pass as u8, 0);
        assert_eq!(PacketAction::Drop as u8, 1);
        assert_eq!(PacketAction::Marked as u8, 2);
        assert_eq!(PacketAction::Extracted as u8, 3);
        assert_eq!(PacketAction::Redirect as u8, 4);
    }

    #[test]
    fn test_direction_repr_values() {
        assert_eq!(Direction::Ingress as u8, 0);
        assert_eq!(Direction::Egress as u8, 1);
    }

    #[test]
    fn test_flow_event_type_repr_values() {
        assert_eq!(FlowEventType::New as u8, 0);
        assert_eq!(FlowEventType::StateChange as u8, 1);
        assert_eq!(FlowEventType::Expired as u8, 2);
        assert_eq!(FlowEventType::Update as u8, 3);
        assert_eq!(FlowEventType::Anomaly as u8, 4);
    }

    #[test]
    fn test_latency_operation_repr_values() {
        assert_eq!(LatencyOperation::TcpSend as u8, 0);
        assert_eq!(LatencyOperation::TcpRecv as u8, 1);
        assert_eq!(LatencyOperation::TcpConnect as u8, 2);
        assert_eq!(LatencyOperation::TcpAccept as u8, 3);
    }

    #[test]
    fn test_syscall_event_type_repr_values() {
        assert_eq!(SyscallEventType::Enter as u8, 0);
        assert_eq!(SyscallEventType::Exit as u8, 1);
    }

    // ── Struct sizes & layout ────────────────────────────────

    #[test]
    fn test_trace_id_size() {
        assert_eq!(std::mem::size_of::<TraceId>(), 16);
    }

    #[test]
    fn test_flow_key_size() {
        // 4+4+2+2+1+3 = 16 bytes
        assert_eq!(std::mem::size_of::<FlowKey>(), 16);
    }

    #[test]
    fn test_packet_event_size_aligned() {
        // repr(C) struct should have predictable alignment
        let size = std::mem::size_of::<PacketEvent>();
        assert_eq!(
            size % 8,
            0,
            "PacketEvent should be 8-byte aligned, got size {}",
            size
        );
    }

    #[test]
    fn test_flow_event_size_aligned() {
        let size = std::mem::size_of::<FlowEvent>();
        assert_eq!(
            size % 8,
            0,
            "FlowEvent should be 8-byte aligned, got size {}",
            size
        );
    }

    #[test]
    fn test_latency_event_size_aligned() {
        let size = std::mem::size_of::<LatencyEvent>();
        assert_eq!(
            size % 8,
            0,
            "LatencyEvent should be 8-byte aligned, got size {}",
            size
        );
    }

    #[test]
    fn test_syscall_event_size_aligned() {
        let size = std::mem::size_of::<SyscallEvent>();
        assert_eq!(
            size % 8,
            0,
            "SyscallEvent should be 8-byte aligned, got size {}",
            size
        );
    }

    // ── Constants ────────────────────────────────────────────

    #[test]
    fn test_ring_buffer_size_is_power_of_two() {
        assert!(RING_BUFFER_SIZE.is_power_of_two());
    }

    #[test]
    fn test_protocol_constants() {
        assert_eq!(IPPROTO_TCP, 6);
        assert_eq!(IPPROTO_UDP, 17);
    }

    #[test]
    fn test_ethernet_constants() {
        assert_eq!(ETH_P_IP, 0x0800);
        assert_eq!(ETH_P_IPV6, 0x86DD);
        assert_eq!(ETH_HLEN, 14);
    }

    #[test]
    fn test_header_sizes() {
        assert_eq!(IPV4_MIN_HLEN, 20);
        assert_eq!(TCP_MIN_HLEN, 20);
        assert_eq!(UDP_HLEN, 8);
    }

    #[test]
    fn test_tcp_flags() {
        // Flags should be individual bits
        assert_eq!(TCP_FLAG_FIN, 0x01);
        assert_eq!(TCP_FLAG_SYN, 0x02);
        assert_eq!(TCP_FLAG_RST, 0x04);
        assert_eq!(TCP_FLAG_PSH, 0x08);
        assert_eq!(TCP_FLAG_ACK, 0x10);
        assert_eq!(TCP_FLAG_URG, 0x20);
        // No two flags overlap
        let all =
            TCP_FLAG_FIN | TCP_FLAG_SYN | TCP_FLAG_RST | TCP_FLAG_PSH | TCP_FLAG_ACK | TCP_FLAG_URG;
        assert_eq!(all.count_ones(), 6);
    }

    #[test]
    fn test_trace_option_constants() {
        assert_eq!(TRACE_OPTION_TYPE, 0x1F);
        assert_eq!(TRACE_OPTION_LEN, 18); // 2 header + 16 trace_id
        assert_eq!(TCP_TRACE_OPTION_KIND, 253);
        assert_eq!(TCP_TRACE_OPTION_LEN, 18);
    }

    // ── Default impls ────────────────────────────────────────

    #[test]
    fn test_flow_state_default() {
        let fs = FlowState::default();
        assert!(fs.trace_id.is_zero());
        assert_eq!(fs.start_ns, 0);
        assert_eq!(fs.packets_in, 0);
        assert_eq!(fs.packets_out, 0);
        assert_eq!(fs.bytes_in, 0);
        assert_eq!(fs.bytes_out, 0);
        assert_eq!(fs.state, ConnectionState::Unknown);
    }

    #[test]
    fn test_packet_event_default() {
        let pe = PacketEvent::default();
        assert_eq!(pe.timestamp_ns, 0);
        assert!(pe.trace_id.is_zero());
        assert_eq!(pe.packet_len, 0);
        assert_eq!(pe.action, PacketAction::Pass);
        assert_eq!(pe.direction, Direction::Ingress);
    }

    #[test]
    fn test_syscall_event_default() {
        let se = SyscallEvent::default();
        assert_eq!(se.pid, 0);
        assert_eq!(se.tid, 0);
        assert_eq!(se.syscall_nr, 0);
        assert_eq!(se.ret, 0);
        assert_eq!(se.event_type, SyscallEventType::Enter);
        assert_eq!(se.comm, [0u8; 16]);
    }
}
