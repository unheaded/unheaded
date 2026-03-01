// SPDX-License-Identifier: GPL-2.0-only
// Unheaded Kingdom — AF_XDP Benchmark Harness
//
// Sacred Law: NO external deps. Rust std only.
// No criterion, no test::Bencher — hand-rolled harness with
// percentile calculation, JSON output, and markdown tables.

use std::time::{Duration, Instant};

// =============================================================================
// BenchResult — single benchmark output record
// =============================================================================

/// A single benchmark result in the JSON output schema.
pub struct BenchResult {
    pub name: &'static str,
    pub timestamp: u64,
    pub iterations: u32,
    pub avg_latency_ns: u64,
    pub p50_ns: u64,
    pub p99_ns: u64,
    pub throughput: f64,
    pub unit: &'static str,
}

impl BenchResult {
    /// Serialize to JSON string (no serde, hand-rolled).
    pub fn to_json(&self) -> String {
        format!(
            concat!(
                "{{",
                "\"name\":\"{}\",",
                "\"timestamp\":{},",
                "\"iterations\":{},",
                "\"avg_latency_ns\":{},",
                "\"p50_ns\":{},",
                "\"p99_ns\":{},",
                "\"throughput\":{:.2},",
                "\"unit\":\"{}\"",
                "}}"
            ),
            self.name,
            self.timestamp,
            self.iterations,
            self.avg_latency_ns,
            self.p50_ns,
            self.p99_ns,
            self.throughput,
            self.unit,
        )
    }

    /// Format as a markdown table row.
    pub fn to_markdown_row(&self) -> String {
        format!(
            "| {:<40} | {:>10} | {:>12} | {:>10} | {:>10} | {:>14} {} |",
            self.name,
            self.iterations,
            self.avg_latency_ns,
            self.p50_ns,
            self.p99_ns,
            format!("{:.2}", self.throughput),
            self.unit,
        )
    }
}

// =============================================================================
// BenchRunner — drives a single benchmark
// =============================================================================

/// Benchmark runner: executes a closure N times, collects durations,
/// computes percentiles and throughput.
pub struct BenchRunner {
    pub name: &'static str,
    pub iterations: u32,
    pub results: Vec<Duration>,
}

impl BenchRunner {
    /// Create a new benchmark runner.
    pub fn new(name: &'static str, iterations: u32) -> Self {
        BenchRunner {
            name,
            iterations,
            results: Vec::with_capacity(iterations as usize),
        }
    }

    /// Run the benchmark: execute `f` for `self.iterations` times,
    /// recording each invocation's duration.
    ///
    /// The closure receives the iteration index.
    pub fn run<F: FnMut(u32)>(&mut self, mut f: F) {
        self.results.clear();
        for i in 0..self.iterations {
            let start = Instant::now();
            f(i);
            self.results.push(start.elapsed());
        }
    }

    /// Run the benchmark with a setup/teardown wrapper.
    /// `setup` is called once before all iterations.
    /// `f` receives the shared state and iteration index.
    pub fn run_with_setup<S, F: FnMut(&mut S, u32)>(&mut self, mut state: S, mut f: F) {
        self.results.clear();
        for i in 0..self.iterations {
            let start = Instant::now();
            f(&mut state, i);
            self.results.push(start.elapsed());
        }
    }

    /// Compute the report from collected results.
    /// `throughput_unit` describes what's being measured (e.g., "frames/sec", "ops/sec").
    /// `items_per_iter` is the number of logical items per iteration (for throughput calc).
    pub fn report(&mut self, throughput_unit: &'static str, items_per_iter: u64) -> BenchResult {
        assert!(!self.results.is_empty(), "must call run() before report()");

        // Sort for percentile calculation
        self.results.sort();

        let total_ns: u128 = self.results.iter().map(|d| d.as_nanos()).sum();
        let count = self.results.len() as u128;
        let avg_ns = (total_ns / count) as u64;
        let p50_ns = percentile(&self.results, 50).as_nanos() as u64;
        let p99_ns = percentile(&self.results, 99).as_nanos() as u64;

        // Throughput: items_per_iter * iterations / total_seconds
        let total_secs = total_ns as f64 / 1_000_000_000.0;
        let throughput = if total_secs > 0.0 {
            (items_per_iter as f64) * (self.iterations as f64) / total_secs
        } else {
            0.0
        };

        // Timestamp: seconds since UNIX epoch (std only, no chrono)
        let timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        BenchResult {
            name: self.name,
            timestamp,
            iterations: self.iterations,
            avg_latency_ns: avg_ns,
            p50_ns,
            p99_ns,
            throughput,
            unit: throughput_unit,
        }
    }
}

// =============================================================================
// Percentile calculation
// =============================================================================

/// Compute the p-th percentile from a sorted slice of durations.
/// Uses nearest-rank method.
fn percentile(sorted: &[Duration], p: u32) -> Duration {
    assert!(!sorted.is_empty(), "cannot compute percentile of empty slice");
    assert!(p <= 100, "percentile must be 0..=100");
    if sorted.len() == 1 {
        return sorted[0];
    }
    // Nearest-rank: index = ceil(p/100 * N) - 1
    let n = sorted.len();
    let rank = ((p as f64 / 100.0) * n as f64).ceil() as usize;
    let idx = rank.min(n).saturating_sub(1);
    sorted[idx]
}

// =============================================================================
// CPU time measurement via getrusage syscall
// =============================================================================

/// Raw timeval structure (matches kernel's struct timeval).
#[repr(C)]
#[derive(Copy, Clone, Default)]
pub struct Timeval {
    pub tv_sec: i64,
    pub tv_usec: i64,
}

/// Partial rusage structure — we only care about ru_utime and ru_stime.
/// Full struct is 144 bytes on x86_64; we read 144 bytes but only use first 32.
#[repr(C)]
#[derive(Copy, Clone)]
pub struct Rusage {
    pub ru_utime: Timeval,
    pub ru_stime: Timeval,
    _pad: [u8; 112], // remaining fields we don't use
}

impl Default for Rusage {
    fn default() -> Self {
        // Safety: all-zeros is valid for this struct (timeval + padding)
        unsafe { std::mem::zeroed() }
    }
}

const RUSAGE_SELF: i32 = 0;
const SYS_GETRUSAGE: u64 = 98;

/// Get current process CPU usage via getrusage(RUSAGE_SELF).
pub fn get_cpu_usage() -> Result<(Duration, Duration), &'static str> {
    let mut usage: Rusage = Rusage::default();
    let ret: i64;
    unsafe {
        std::arch::asm!(
            "syscall",
            inlateout("rax") SYS_GETRUSAGE as i64 => ret,
            in("rdi") RUSAGE_SELF as u64,
            in("rsi") &mut usage as *mut Rusage as u64,
            lateout("rdx") _,
            lateout("rcx") _,
            lateout("r11") _,
            options(nostack),
        );
    }
    if ret < 0 {
        return Err("getrusage failed");
    }

    let user = Duration::new(
        usage.ru_utime.tv_sec as u64,
        (usage.ru_utime.tv_usec * 1000) as u32,
    );
    let system = Duration::new(
        usage.ru_stime.tv_sec as u64,
        (usage.ru_stime.tv_usec * 1000) as u32,
    );
    Ok((user, system))
}

/// Measure CPU time consumed by a closure.
/// Returns (result, user_time, system_time).
pub fn measure_cpu<F: FnOnce() -> R, R>(f: F) -> (R, Duration, Duration) {
    let (u0, s0) = get_cpu_usage().unwrap_or((Duration::ZERO, Duration::ZERO));
    let result = f();
    let (u1, s1) = get_cpu_usage().unwrap_or((Duration::ZERO, Duration::ZERO));
    let user = u1.saturating_sub(u0);
    let sys = s1.saturating_sub(s0);
    (result, user, sys)
}

// =============================================================================
// JSON array helper
// =============================================================================

/// Format a slice of BenchResult as a JSON array string.
pub fn results_to_json(results: &[BenchResult]) -> String {
    let mut json = String::from("[\n");
    for (i, r) in results.iter().enumerate() {
        json.push_str("  ");
        json.push_str(&r.to_json());
        if i + 1 < results.len() {
            json.push(',');
        }
        json.push('\n');
    }
    json.push(']');
    json
}

/// Print results as a markdown table to stderr.
pub fn print_markdown_table(results: &[BenchResult]) {
    eprintln!();
    eprintln!("# AF_XDP Benchmark Results");
    eprintln!();
    eprintln!(
        "| {:<40} | {:>10} | {:>12} | {:>10} | {:>10} | {:>20} |",
        "Benchmark", "Iters", "Avg (ns)", "P50 (ns)", "P99 (ns)", "Throughput"
    );
    eprintln!(
        "|{:-<42}|{:-<12}|{:-<14}|{:-<12}|{:-<12}|{:-<22}|",
        "", "", "", "", "", ""
    );
    for r in results {
        eprintln!("{}", r.to_markdown_row());
    }
    eprintln!();
}

/// Print CPU time summary to stderr.
pub fn print_cpu_summary(label: &str, user: Duration, system: Duration) {
    eprintln!(
        "  CPU[{}]: user={:.3}ms system={:.3}ms total={:.3}ms",
        label,
        user.as_secs_f64() * 1000.0,
        system.as_secs_f64() * 1000.0,
        (user + system).as_secs_f64() * 1000.0,
    );
}

// =============================================================================
// Tests (self-test the harness)
// =============================================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_percentile_single() {
        let durations = vec![Duration::from_nanos(100)];
        assert_eq!(percentile(&durations, 50), Duration::from_nanos(100));
        assert_eq!(percentile(&durations, 99), Duration::from_nanos(100));
    }

    #[test]
    fn test_percentile_sorted() {
        let durations: Vec<Duration> = (1..=100)
            .map(|i| Duration::from_nanos(i))
            .collect();
        assert_eq!(percentile(&durations, 50), Duration::from_nanos(50));
        assert_eq!(percentile(&durations, 99), Duration::from_nanos(99));
        assert_eq!(percentile(&durations, 100), Duration::from_nanos(100));
    }

    #[test]
    fn test_bench_runner_basic() {
        let mut runner = BenchRunner::new("test_noop", 100);
        runner.run(|_i| {
            // noop — measure overhead
            std::hint::black_box(42);
        });
        let result = runner.report("ops/sec", 1);
        assert_eq!(result.name, "test_noop");
        assert_eq!(result.iterations, 100);
        assert!(result.avg_latency_ns < 1_000_000); // < 1ms per noop
        assert!(result.throughput > 0.0);
    }

    #[test]
    fn test_bench_result_json() {
        let r = BenchResult {
            name: "test",
            timestamp: 1234567890,
            iterations: 100,
            avg_latency_ns: 500,
            p50_ns: 450,
            p99_ns: 900,
            throughput: 2_000_000.0,
            unit: "ops/sec",
        };
        let json = r.to_json();
        assert!(json.contains("\"name\":\"test\""));
        assert!(json.contains("\"iterations\":100"));
        assert!(json.contains("\"p50_ns\":450"));
    }

    #[test]
    fn test_getrusage() {
        let result = get_cpu_usage();
        assert!(result.is_ok());
        let (user, sys) = result.unwrap();
        // Process should have consumed some user time by now
        assert!(user.as_nanos() > 0 || sys.as_nanos() > 0);
    }

    #[test]
    fn test_measure_cpu() {
        let (val, user, _sys) = measure_cpu(|| {
            // Burn some CPU
            let mut sum = 0u64;
            for i in 0..100_000 {
                sum = sum.wrapping_add(i);
            }
            std::hint::black_box(sum)
        });
        assert!(val > 0);
        // user time should be non-negative (could be 0 if too fast)
        let _ = user;
    }
}
