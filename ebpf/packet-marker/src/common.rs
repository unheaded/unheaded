//! Unheaded Kingdom - Packet Marker Common Types
//!
//! Shared types between the packet_marker XDP program and the Go trace-collector
//! userspace component. These types define the BPF map entry layouts that cross
//! the kernel/userspace boundary.
//!
//! IMPORTANT: These types MUST match the Go definitions in
//! `cmd/trace-collector-go/types.go` byte-for-byte. Any changes here must be
//! reflected there, and vice versa.
//!
//! Layout is #[repr(C)] with explicit padding to ensure identical memory layout
//! across Rust (kernel) and Go (userspace).

/// Trace entry written to the TRACE_MAP by the XDP program.
///
/// This structure is read by the Go trace-collector via BPF map iteration.
/// Total size: 68 bytes.
///
/// Wire layout (little-endian unless noted):
/// ```text
/// Offset  Size  Field
/// 0x00    16    trace_id        (UUIDv7, big-endian)
/// 0x10    8     timestamp_ns    (u64 LE, ktime_get_ns())
/// 0x18    16    src_ip          (IPv4-mapped-IPv6 or native IPv6, network byte order)
/// 0x28    16    dst_ip          (same)
/// 0x38    2     src_port        (u16 BE, network byte order)
/// 0x3A    2     dst_port        (u16 BE, network byte order)
/// 0x3C    1     protocol        (u8, IP protocol number)
/// 0x3D    1     flags           (u8, TCP flags or ICMP type)
/// 0x3E    2     packet_len      (u16 LE, total packet length)
/// 0x40    1     hop_count       (u8, number of XDP hops observed)
/// 0x41    3     _pad            (alignment padding, must be zero)
/// ```
#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct TraceEntry {
    /// UUIDv7 trace identifier (128-bit, big-endian).
    pub trace_id: [u8; 16],

    /// Kernel timestamp from bpf_ktime_get_ns() (nanoseconds since boot).
    pub timestamp_ns: u64,

    /// Source IP address.
    /// For IPv4: stored as IPv4-mapped-IPv6 (::ffff:a.b.c.d).
    /// For IPv6: native 128-bit address.
    pub src_ip: [u8; 16],

    /// Destination IP address (same format as src_ip).
    pub dst_ip: [u8; 16],

    /// Source port in network byte order (big-endian).
    pub src_port: u16,

    /// Destination port in network byte order (big-endian).
    pub dst_port: u16,

    /// IP protocol number (TCP=6, UDP=17, ICMPv6=58, ICMP=1).
    pub protocol: u8,

    /// TCP flags (SYN, ACK, FIN, RST, etc.) or ICMP type.
    pub flags: u8,

    /// Total packet length in bytes.
    pub packet_len: u16,

    /// Number of XDP hops this packet has traversed.
    pub hop_count: u8,

    /// Alignment padding (must be zero).
    pub _pad: [u8; 3],
}

/// Statistics entry aggregated by the XDP program.
///
/// Each XDP program instance maintains a per-CPU stats map. The Go
/// trace-collector reads and aggregates across CPUs.
///
/// Wire layout (all u64 LE):
/// ```text
/// Offset  Size  Field
/// 0x00    8     packets_total
/// 0x08    8     bytes_total
/// 0x10    8     traces_created
/// 0x18    8     traces_updated
/// 0x20    8     errors
/// ```
#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct StatsEntry {
    /// Total packets processed by this XDP program.
    pub packets_total: u64,

    /// Total bytes across all processed packets.
    pub bytes_total: u64,

    /// Number of new trace IDs generated.
    pub traces_created: u64,

    /// Number of existing traces updated with additional info.
    pub traces_updated: u64,

    /// Processing errors (malformed packets, map write failures, etc.).
    pub errors: u64,
}

/// Flow key used for the FLOW_STATE and TRACE_INJECT BPF maps.
///
/// This is a re-export from unheaded-common for convenience; the canonical
/// definition lives in `ebpf/common/src/lib.rs`.
///
/// 16 bytes total: src_addr(4) + dst_addr(4) + src_port(2) + dst_port(2) + protocol(1) + pad(3).
pub use unheaded_common::FlowKey;

/// Flow state stored per connection.
pub use unheaded_common::FlowState;

/// Trace ID (128-bit).
pub use unheaded_common::TraceId;

/// Packet event for ring buffer transport.
pub use unheaded_common::PacketEvent;

/// Direction of packet flow.
pub use unheaded_common::Direction;

/// Action taken on a packet.
pub use unheaded_common::PacketAction;

/// Map names pinned to the BPF filesystem.
///
/// These constants must match the map names used in the XDP program
/// and the paths expected by the Go trace-collector.
pub mod map_names {
    /// Flow state map: FlowKey -> FlowState
    pub const FLOW_STATE: &str = "FLOW_STATE";

    /// Trace injection map: FlowKey -> TraceId (userspace -> kernel)
    pub const TRACE_INJECT: &str = "TRACE_INJECT";

    /// Packet events ring buffer (kernel -> userspace)
    pub const PACKET_EVENTS: &str = "PACKET_EVENTS";

    /// Statistics counters: u32 key -> u64 value
    pub const STATS: &str = "STATS";
}

/// Statistics counter keys (must match XDP program constants).
pub mod stat_keys {
    pub const PACKETS_TOTAL: u32 = 0;
    pub const PACKETS_IPV4: u32 = 1;
    pub const PACKETS_TCP: u32 = 2;
    pub const PACKETS_UDP: u32 = 3;
    pub const TRACE_EXTRACTED: u32 = 4;
    pub const TRACE_INJECTED: u32 = 5;
    pub const EVENTS_SENT: u32 = 6;
    pub const EVENTS_DROPPED: u32 = 7;
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_trace_entry_size() {
        // Verify the structure is exactly 68 bytes
        assert_eq!(
            core::mem::size_of::<TraceEntry>(),
            68,
            "TraceEntry must be exactly 68 bytes for Go compatibility"
        );
    }

    #[test]
    fn test_stats_entry_size() {
        // Verify the structure is exactly 40 bytes
        assert_eq!(
            core::mem::size_of::<StatsEntry>(),
            40,
            "StatsEntry must be exactly 40 bytes for Go compatibility"
        );
    }

    #[test]
    fn test_trace_entry_alignment() {
        // Must be 8-byte aligned for atomic access
        assert_eq!(
            core::mem::align_of::<TraceEntry>() % 8,
            0,
            "TraceEntry must be 8-byte aligned"
        );
    }

    #[test]
    fn test_stats_entry_alignment() {
        assert_eq!(
            core::mem::align_of::<StatsEntry>() % 8,
            0,
            "StatsEntry must be 8-byte aligned"
        );
    }

    #[test]
    fn test_trace_entry_default_zeroed() {
        let te = TraceEntry::default();
        assert_eq!(te.trace_id, [0u8; 16]);
        assert_eq!(te.timestamp_ns, 0);
        assert_eq!(te.src_ip, [0u8; 16]);
        assert_eq!(te.dst_ip, [0u8; 16]);
        assert_eq!(te.src_port, 0);
        assert_eq!(te.dst_port, 0);
        assert_eq!(te.protocol, 0);
        assert_eq!(te.flags, 0);
        assert_eq!(te.packet_len, 0);
        assert_eq!(te.hop_count, 0);
        assert_eq!(te._pad, [0u8; 3]);
    }

    #[test]
    fn test_stat_keys_unique() {
        let keys = [
            stat_keys::PACKETS_TOTAL,
            stat_keys::PACKETS_IPV4,
            stat_keys::PACKETS_TCP,
            stat_keys::PACKETS_UDP,
            stat_keys::TRACE_EXTRACTED,
            stat_keys::TRACE_INJECTED,
            stat_keys::EVENTS_SENT,
            stat_keys::EVENTS_DROPPED,
        ];
        for i in 0..keys.len() {
            for j in (i + 1)..keys.len() {
                assert_ne!(keys[i], keys[j], "stat keys must be unique");
            }
        }
    }
}
