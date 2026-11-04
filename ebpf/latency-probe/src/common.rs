// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Stevie Bellis.
//! Shared types for the latency-probe BPF program.
//!
//! These types define the LATENCY_MAP layout: per-thread RTT statistics
//! keyed by trace id, pid and tid (LatencyTraceKey) with aggregated
//! min/max/avg RTT values (LatencyEntry). A kprobe on tcp_sendmsg has no
//! 5-tuple to key on. pkg/protocol/bpfschema mirrors these byte-for-byte.

/// Key for LATENCY_MAP — one thread's samples under one trace id.
///
/// trace_id is the socket's TraceId in big-endian bytes (zero when no socket
/// trace is associated); pid and tid are the full kernel ids.
/// Size: 16 + 4 + 4 = 24 bytes.
#[repr(C)]
#[derive(Clone, Copy, Default, Debug, PartialEq, Eq)]
pub struct LatencyTraceKey {
    pub trace_id: [u8; 16],
    pub pid: u32,
    pub tid: u32,
}

/// Value for LATENCY_MAP — aggregated RTT statistics per flow.
///
/// Updated atomically on each RTT sample. The reader computes
/// average as (running sum from rtt_ns * samples) / samples.
/// Size: 8 + 8 + 8 + 8 + 8 = 40 bytes.
#[repr(C)]
#[derive(Clone, Copy, Default, Debug)]
pub struct LatencyEntry {
    pub rtt_ns: u64,         // Most recent RTT sample
    pub min_rtt_ns: u64,     // Minimum observed RTT
    pub max_rtt_ns: u64,     // Maximum observed RTT
    pub samples: u64,        // Number of RTT samples
    pub last_update_ns: u64, // ktime_get_ns() of last update
}

/// Size of LatencyTraceKey in bytes.
#[allow(dead_code)]
pub const LATENCY_TRACE_KEY_SIZE: usize = 24;

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
            core::mem::size_of::<LatencyTraceKey>(),
            LATENCY_TRACE_KEY_SIZE,
            "LatencyTraceKey size mismatch"
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
        let key = LatencyTraceKey::default();
        assert_eq!(key.trace_id, [0u8; 16]);
        assert_eq!(key.pid, 0);
        assert_eq!(key.tid, 0);
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
