//! Latency event parsing.
//!
//! Handles latency probe events from eBPF programs that measure
//! function execution times, request latencies, etc.

use serde::{Deserialize, Serialize};

use super::EventError;

/// Maximum command name length
const TASK_COMM_LEN: usize = 16;

/// Maximum probe name length
const PROBE_NAME_LEN: usize = 64;

/// Latency probe type
#[repr(u8)]
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum ProbeType {
    /// Kernel function (kprobe/kretprobe)
    Kprobe = 0,
    /// User function (uprobe/uretprobe)
    Uprobe = 1,
    /// Tracepoint
    Tracepoint = 2,
    /// USDT probe
    Usdt = 3,
    /// Custom/application probe
    Custom = 4,
}

impl From<u8> for ProbeType {
    fn from(v: u8) -> Self {
        match v {
            0 => ProbeType::Kprobe,
            1 => ProbeType::Uprobe,
            2 => ProbeType::Tracepoint,
            3 => ProbeType::Usdt,
            _ => ProbeType::Custom,
        }
    }
}

/// Latency bucket for histogram aggregation
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum LatencyBucket {
    /// < 1 microsecond
    SubMicrosecond,
    /// 1-10 microseconds
    Microseconds1_10,
    /// 10-100 microseconds
    Microseconds10_100,
    /// 100-1000 microseconds
    Microseconds100_1000,
    /// 1-10 milliseconds
    Milliseconds1_10,
    /// 10-100 milliseconds
    Milliseconds10_100,
    /// 100-1000 milliseconds
    Milliseconds100_1000,
    /// > 1 second
    Seconds,
}

impl LatencyBucket {
    pub fn from_ns(ns: u64) -> Self {
        match ns {
            0..=999 => LatencyBucket::SubMicrosecond,
            1_000..=9_999 => LatencyBucket::Microseconds1_10,
            10_000..=99_999 => LatencyBucket::Microseconds10_100,
            100_000..=999_999 => LatencyBucket::Microseconds100_1000,
            1_000_000..=9_999_999 => LatencyBucket::Milliseconds1_10,
            10_000_000..=99_999_999 => LatencyBucket::Milliseconds10_100,
            100_000_000..=999_999_999 => LatencyBucket::Milliseconds100_1000,
            _ => LatencyBucket::Seconds,
        }
    }
}

/// Raw latency event structure from BPF
#[repr(C, packed)]
struct RawLatencyEvent {
    /// Process ID
    pid: u32,
    /// Thread ID
    tid: u32,
    /// Probe type
    probe_type: u8,
    /// Padding
    _pad: [u8; 7],
    /// Latency in nanoseconds
    latency_ns: u64,
    /// Entry timestamp (for correlation)
    entry_ts: u64,
    /// Exit timestamp
    exit_ts: u64,
    /// Stack ID (if captured)
    stack_id: i32,
    /// Padding
    _pad2: [u8; 4],
    /// Probe name/function name
    probe_name: [u8; PROBE_NAME_LEN],
    /// Command name
    comm: [u8; TASK_COMM_LEN],
}

/// Parsed latency event
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LatencyEvent {
    /// Process ID
    pub pid: u32,
    /// Thread ID
    pub tid: u32,
    /// Command name
    pub comm: String,
    /// Probe type
    pub probe_type: ProbeType,
    /// Probe name (function, tracepoint, etc.)
    pub probe_name: String,
    /// Latency in nanoseconds
    pub latency_ns: u64,
    /// Entry timestamp
    pub entry_ts: u64,
    /// Exit timestamp
    pub exit_ts: u64,
    /// Stack ID (for stack trace lookup)
    pub stack_id: Option<i32>,
    /// Latency bucket
    pub bucket: LatencyBucket,
}

impl LatencyEvent {
    /// Parse latency event from raw bytes
    pub fn from_bytes(data: &[u8]) -> Result<Self, EventError> {
        let size = std::mem::size_of::<RawLatencyEvent>();
        if data.len() < size {
            return Err(EventError::InsufficientData {
                expected: size,
                actual: data.len(),
            });
        }

        // SAFETY: We've verified the length
        let raw: RawLatencyEvent =
            unsafe { std::ptr::read_unaligned(data.as_ptr() as *const RawLatencyEvent) };

        let comm = parse_comm(&raw.comm);
        let probe_name = parse_probe_name(&raw.probe_name);
        let bucket = LatencyBucket::from_ns(raw.latency_ns);

        let stack_id = if raw.stack_id >= 0 {
            Some(raw.stack_id)
        } else {
            None
        };

        Ok(Self {
            pid: raw.pid,
            tid: raw.tid,
            comm,
            probe_type: ProbeType::from(raw.probe_type),
            probe_name,
            latency_ns: raw.latency_ns,
            entry_ts: raw.entry_ts,
            exit_ts: raw.exit_ts,
            stack_id,
            bucket,
        })
    }

    /// Get latency in microseconds
    pub fn latency_us(&self) -> f64 {
        self.latency_ns as f64 / 1000.0
    }

    /// Get latency in milliseconds
    pub fn latency_ms(&self) -> f64 {
        self.latency_ns as f64 / 1_000_000.0
    }

    /// Check if latency exceeds threshold
    pub fn exceeds_threshold(&self, threshold_ns: u64) -> bool {
        self.latency_ns > threshold_ns
    }

    /// Check if this is a slow event (> 1ms)
    pub fn is_slow(&self) -> bool {
        self.latency_ns > 1_000_000
    }

    /// Check if this is a very slow event (> 100ms)
    pub fn is_very_slow(&self) -> bool {
        self.latency_ns > 100_000_000
    }
}

/// Parse null-terminated command name
fn parse_comm(comm: &[u8; TASK_COMM_LEN]) -> String {
    let end = comm.iter().position(|&c| c == 0).unwrap_or(TASK_COMM_LEN);
    String::from_utf8_lossy(&comm[..end]).to_string()
}

/// Parse null-terminated probe name
fn parse_probe_name(name: &[u8; PROBE_NAME_LEN]) -> String {
    let end = name.iter().position(|&c| c == 0).unwrap_or(PROBE_NAME_LEN);
    String::from_utf8_lossy(&name[..end]).to_string()
}

/// Statistics for latency aggregation
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct LatencyStats {
    pub count: u64,
    pub sum_ns: u64,
    pub min_ns: u64,
    pub max_ns: u64,
    pub p50_ns: u64,
    pub p90_ns: u64,
    pub p99_ns: u64,
}

impl LatencyStats {
    pub fn new() -> Self {
        Self {
            count: 0,
            sum_ns: 0,
            min_ns: u64::MAX,
            max_ns: 0,
            p50_ns: 0,
            p90_ns: 0,
            p99_ns: 0,
        }
    }

    pub fn add(&mut self, latency_ns: u64) {
        self.count += 1;
        self.sum_ns += latency_ns;
        self.min_ns = self.min_ns.min(latency_ns);
        self.max_ns = self.max_ns.max(latency_ns);
    }

    pub fn mean_ns(&self) -> f64 {
        if self.count == 0 {
            0.0
        } else {
            self.sum_ns as f64 / self.count as f64
        }
    }

    pub fn mean_us(&self) -> f64 {
        self.mean_ns() / 1000.0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_latency_bucket() {
        assert_eq!(LatencyBucket::from_ns(500), LatencyBucket::SubMicrosecond);
        assert_eq!(
            LatencyBucket::from_ns(5000),
            LatencyBucket::Microseconds1_10
        );
        assert_eq!(
            LatencyBucket::from_ns(50000),
            LatencyBucket::Microseconds10_100
        );
        assert_eq!(
            LatencyBucket::from_ns(5_000_000),
            LatencyBucket::Milliseconds1_10
        );
        assert_eq!(
            LatencyBucket::from_ns(5_000_000_000),
            LatencyBucket::Seconds
        );
    }

    #[test]
    fn test_latency_stats() {
        let mut stats = LatencyStats::new();
        stats.add(1000);
        stats.add(2000);
        stats.add(3000);

        assert_eq!(stats.count, 3);
        assert_eq!(stats.sum_ns, 6000);
        assert_eq!(stats.min_ns, 1000);
        assert_eq!(stats.max_ns, 3000);
        assert_eq!(stats.mean_ns(), 2000.0);
    }

    #[test]
    fn test_probe_type() {
        assert_eq!(ProbeType::from(0), ProbeType::Kprobe);
        assert_eq!(ProbeType::from(1), ProbeType::Uprobe);
        assert_eq!(ProbeType::from(99), ProbeType::Custom);
    }
}
