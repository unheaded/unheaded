//! Shared types for the latency-probe BPF program.
//!
//! These types define the LATENCY_MAP layout: a per-flow RTT statistics
//! map keyed by 5-tuple (LatencyKey) with aggregated min/max/avg RTT
//! values (LatencyEntry). Go userspace mirrors these byte-for-byte.

/// Key for LATENCY_MAP — identifies a flow by 5-tuple.
///
/// IPs are stored as IPv6 (IPv4 addresses use ::ffff:a.b.c.d mapping).
/// Size: 16 + 16 + 2 + 2 + 1 + 3 = 40 bytes.
#[repr(C)]
#[derive(Clone, Copy, Default, Debug, PartialEq, Eq)]
pub struct LatencyKey {
    pub src_ip: [u8; 16],
    pub dst_ip: [u8; 16],
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: u8,
    pub _pad: [u8; 3],
}

/// Value for LATENCY_MAP — aggregated RTT statistics per flow.
///
/// Updated atomically on each RTT sample. The reader computes
/// average as (running sum from rtt_ns * samples) / samples.
/// Size: 8 + 8 + 8 + 8 + 8 = 40 bytes.
#[repr(C)]
#[derive(Clone, Copy, Default, Debug)]
pub struct LatencyEntry {
    pub rtt_ns: u64,           // Most recent RTT sample
    pub min_rtt_ns: u64,       // Minimum observed RTT
    pub max_rtt_ns: u64,       // Maximum observed RTT
    pub samples: u64,          // Number of RTT samples
    pub last_update_ns: u64,   // ktime_get_ns() of last update
}

/// Size of LatencyKey in bytes.
#[allow(dead_code)]
pub const LATENCY_KEY_SIZE: usize = 40;

/// Size of LatencyEntry in bytes.
#[allow(dead_code)]
pub const LATENCY_ENTRY_SIZE: usize = 40;

/// Maximum number of entries in the LATENCY_MAP.
pub const MAX_LATENCY_MAP_ENTRIES: u32 = 65536;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_latency_key_size() {
        assert_eq!(
            core::mem::size_of::<LatencyKey>(),
            LATENCY_KEY_SIZE,
            "LatencyKey size mismatch"
        );
    }

    #[test]
    fn test_latency_entry_size() {
        assert_eq!(
            core::mem::size_of::<LatencyEntry>(),
            LATENCY_ENTRY_SIZE,
            "LatencyEntry size mismatch"
        );
    }

    #[test]
    fn test_latency_key_default() {
        let key = LatencyKey::default();
        assert_eq!(key.src_ip, [0u8; 16]);
        assert_eq!(key.dst_ip, [0u8; 16]);
        assert_eq!(key.src_port, 0);
        assert_eq!(key.dst_port, 0);
        assert_eq!(key.protocol, 0);
    }

    #[test]
    fn test_latency_entry_default() {
        let entry = LatencyEntry::default();
        assert_eq!(entry.rtt_ns, 0);
        assert_eq!(entry.min_rtt_ns, 0);
        assert_eq!(entry.max_rtt_ns, 0);
        assert_eq!(entry.samples, 0);
        assert_eq!(entry.last_update_ns, 0);
    }
}
