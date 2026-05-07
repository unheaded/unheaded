//! Shared types for eBPF kernel programs and userspace collector.
//!
//! All structs are `#[repr(C)]` for stable ABI between BPF programs
//! and the userspace loader reading from ring buffers.

#![no_std]

/// IPv6-native address with embedded mesh metadata.
///
/// Layout for IPv4 (af=2):
///   [0..10]  = MeshMeta (10 bytes FREE per addr, 20 bytes total per event)
///   [10..12] = 0xFF 0xFF (v4-mapped sentinel)
///   [12..16] = IPv4 address
///
/// Layout for IPv6 (af=10):
///   [0..16]  = full IPv6 address (metadata lives elsewhere)
///
/// The 20 "free" bytes (src_addr[0:10] + dst_addr[0:10]) on IPv4 traffic
/// become our service mesh metadata bus. The eBPF programs stamp context
/// into these fields per-event — it never touches wire packets.
///
///   src_addr[0:10] "source mesh meta":
///     [0]      = mesh_version (protocol version, currently 1)
///     [1]      = service_id (which service/cgroup emitted this, 0=unknown)
///     [2]      = hop_count (incremented per eBPF program touch)
///     [3]      = flow_flags (bitfield: SYN_SEEN|EST|FIN_SEEN|RST|RETRANSMIT|OOO)
///     [4..8]   = trace_hash_lo (lower 32 bits of trace correlation hash)
///     [8..10]  = qos_class (priority/traffic class from tc, 16-bit)
///
///   dst_addr[0:10] "destination mesh meta":
///     [0]      = dst_service_id (resolved destination service, 0=external)
///     [1]      = nat_type (0=none, 1=SNAT, 2=DNAT, 3=masquerade)
///     [2..4]   = orig_port (pre-NAT port if NATted, 0 otherwise)
///     [4..8]   = latency_hint_ns (per-hop latency delta as u32 nanoseconds)
///     [8..10]  = reserved (future: load-balancer backend ID, canary tag, etc.)
#[repr(C)]
#[derive(Clone, Copy)]
pub struct PacketEvent {
    pub timestamp_ns: u64,
    pub src_addr: [u8; 16], // IPv6-native OR IPv4-mapped + 10 bytes mesh meta
    pub dst_addr: [u8; 16], // IPv6-native OR IPv4-mapped + 10 bytes mesh meta
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: u8,
    pub af: u8, // AF_INET=2 or AF_INET6=10
    pub _pad: [u8; 2],
    pub packet_len: u32,
    pub direction: u8, // 0=ingress, 1=egress
    pub _pad2: [u8; 3],
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct LatencyEvent {
    pub timestamp_ns: u64,
    pub pid: u32,
    pub tid: u32,
    pub latency_ns: u64,
    pub operation: u8, // 0=tcp_send, 1=tcp_recv, 2=tcp_connect
    pub _pad: [u8; 7],
}

// Constants
pub const AF_INET: u8 = 2;
pub const AF_INET6: u8 = 10;

// Flow flag bits
pub const FLOW_FLAG_SYN_SEEN: u8 = 1 << 0;
pub const FLOW_FLAG_EST: u8 = 1 << 1;
pub const FLOW_FLAG_FIN_SEEN: u8 = 1 << 2;
pub const FLOW_FLAG_RST: u8 = 1 << 3;
pub const FLOW_FLAG_RETRANSMIT: u8 = 1 << 4;
pub const FLOW_FLAG_OOO: u8 = 1 << 5;

// NAT types
pub const NAT_NONE: u8 = 0;
pub const NAT_SNAT: u8 = 1;
pub const NAT_DNAT: u8 = 2;
pub const NAT_MASQUERADE: u8 = 3;

// Operation types
pub const OP_TCP_SEND: u8 = 0;
pub const OP_TCP_RECV: u8 = 1;
pub const OP_TCP_CONNECT: u8 = 2;

// Ring buffer size
pub const RING_BUF_SIZE: u32 = 256 * 1024; // 256KB

#[cfg(test)]
mod tests {
    use super::*;
    use core::mem::{align_of, size_of};

    #[test]
    fn test_packet_event_size() {
        // u64 + [u8;16] + [u8;16] + u16 + u16 + u8 + u8 + [u8;2] + u32 + u8 + [u8;3]
        // =  8  +   16   +   16    +  2  +  2  + 1  + 1  +   2    +  4  + 1  +   3  = 56
        assert_eq!(size_of::<PacketEvent>(), 56);
    }

    #[test]
    fn test_latency_event_size() {
        // u64 + u32 + u32 + u64 + u8 + [u8;7]
        // =  8 +  4  +  4  +  8  + 1  +   7   = 32
        assert_eq!(size_of::<LatencyEvent>(), 32);
    }

    #[test]
    fn test_packet_event_alignment() {
        assert_eq!(align_of::<PacketEvent>(), 8);
    }

    #[test]
    fn test_latency_event_alignment() {
        assert_eq!(align_of::<LatencyEvent>(), 8);
    }
}
