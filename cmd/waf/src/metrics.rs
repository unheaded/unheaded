// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Prometheus Metrics for THE SHIELD
//!
//! Implements Prometheus-compatible metrics export without external dependencies.
//! Provides counters, gauges, and histograms for monitoring WAF performance.

use std::collections::HashMap;
use std::sync::atomic::{AtomicI64, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::RwLock;

/// A simple atomic counter
#[derive(Debug, Default)]
pub struct Counter {
    value: AtomicU64,
}

impl Counter {
    /// Create a new counter
    pub fn new() -> Self {
        Self::default()
    }

    /// Increment the counter by 1
    pub fn inc(&self) {
        self.value.fetch_add(1, Ordering::Relaxed);
    }

    /// Increment the counter by n
    pub fn add(&self, n: u64) {
        self.value.fetch_add(n, Ordering::Relaxed);
    }

    /// Get the current value
    pub fn get(&self) -> u64 {
        self.value.load(Ordering::Relaxed)
    }

    /// Reset the counter
    pub fn reset(&self) {
        self.value.store(0, Ordering::Relaxed);
    }
}

/// A simple atomic gauge
#[derive(Debug, Default)]
pub struct Gauge {
    value: AtomicI64,
}

impl Gauge {
    /// Create a new gauge
    pub fn new() -> Self {
        Self::default()
    }

    /// Set the gauge value
    pub fn set(&self, v: i64) {
        self.value.store(v, Ordering::Relaxed);
    }

    /// Increment the gauge by 1
    pub fn inc(&self) {
        self.value.fetch_add(1, Ordering::Relaxed);
    }

    /// Decrement the gauge by 1
    pub fn dec(&self) {
        self.value.fetch_sub(1, Ordering::Relaxed);
    }

    /// Add to the gauge
    pub fn add(&self, n: i64) {
        self.value.fetch_add(n, Ordering::Relaxed);
    }

    /// Get the current value
    pub fn get(&self) -> i64 {
        self.value.load(Ordering::Relaxed)
    }
}

/// Histogram buckets for latency measurements
#[derive(Debug)]
pub struct Histogram {
    /// Bucket boundaries in milliseconds
    buckets: Vec<f64>,
    /// Count per bucket
    bucket_counts: Vec<AtomicU64>,
    /// Total sum of observations
    sum: AtomicU64,
    /// Total count of observations
    count: AtomicU64,
}

impl Histogram {
    /// Create a histogram with default buckets (suitable for HTTP latencies)
    pub fn new() -> Self {
        Self::with_buckets(vec![
            0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
        ])
    }

    /// Create a histogram with custom bucket boundaries (in seconds)
    pub fn with_buckets(buckets: Vec<f64>) -> Self {
        let bucket_counts = buckets.iter().map(|_| AtomicU64::new(0)).collect();
        Self {
            buckets,
            bucket_counts,
            sum: AtomicU64::new(0),
            count: AtomicU64::new(0),
        }
    }

    /// Observe a value (in seconds)
    pub fn observe(&self, value: f64) {
        // Update count and sum
        self.count.fetch_add(1, Ordering::Relaxed);
        // Store sum as microseconds to maintain precision
        let micros = (value * 1_000_000.0) as u64;
        self.sum.fetch_add(micros, Ordering::Relaxed);

        // Find the appropriate bucket. Each observation lands in EXACTLY
        // one bucket (non-cumulative). Values that exceed all bucket
        // boundaries are not added to any bucket — they are still counted
        // by `count()` and `sum()`, which gives the implicit +Inf-bucket
        // value as `count - sum(buckets)` (Prometheus convention). A future
        // `buckets()` extension to surface the +Inf bucket explicitly is
        // the right next step.
        for (i, boundary) in self.buckets.iter().enumerate() {
            if value <= *boundary {
                self.bucket_counts[i].fetch_add(1, Ordering::Relaxed);
                return;
            }
        }
        // Overflow: tracked only in count + sum, not in any user bucket.
    }

    /// Observe a duration
    pub fn observe_duration(&self, duration: Duration) {
        self.observe(duration.as_secs_f64());
    }

    /// Get the total count
    pub fn count(&self) -> u64 {
        self.count.load(Ordering::Relaxed)
    }

    /// Get the sum (in seconds)
    pub fn sum(&self) -> f64 {
        self.sum.load(Ordering::Relaxed) as f64 / 1_000_000.0
    }

    /// Get bucket data for export
    pub fn buckets(&self) -> Vec<(f64, u64)> {
        self.buckets
            .iter()
            .zip(self.bucket_counts.iter())
            .map(|(bound, count)| (*bound, count.load(Ordering::Relaxed)))
            .collect()
    }
}

impl Default for Histogram {
    fn default() -> Self {
        Self::new()
    }
}

/// Labels for metric dimensions
pub type Labels = HashMap<String, String>;

/// A metric's label values, ordered to match its `label_names`. Used as the
/// map key that identifies one label combination (one time series).
type LabelKey = Vec<(String, String)>;

/// Per-label-combination counter series.
type CounterSeries = RwLock<HashMap<LabelKey, Arc<Counter>>>;

/// Per-label-combination histogram series.
type HistogramSeries = RwLock<HashMap<LabelKey, Arc<Histogram>>>;

/// A counter with labels
pub struct LabeledCounter {
    counters: CounterSeries,
    label_names: Vec<String>,
}

impl LabeledCounter {
    /// Create a new labeled counter
    pub fn new(label_names: &[&str]) -> Self {
        Self {
            counters: RwLock::new(HashMap::new()),
            label_names: label_names.iter().map(|s| s.to_string()).collect(),
        }
    }

    /// Enforce the invariant documented on [`LabelKey`]: the label values
    /// handed to `with_labels` must match the declared `label_names`, in
    /// order.
    ///
    /// Prometheus requires every series of a metric family to carry the same
    /// label set. If two call sites pass different names — or the same names
    /// in a different order — the export produces series that disagree, and a
    /// scraper is entitled to reject the whole family. Nothing checked this
    /// before, which is why `label_names` read as dead.
    ///
    /// `debug_assert` rather than a runtime check on purpose: this sits in the
    /// per-request hot path, and the label set is fixed by the call site at
    /// compile time, not by request data. A mismatch is a coding error, caught
    /// by the test suite, and costs nothing in release.
    fn assert_label_schema(&self, labels: &[(&str, &str)]) {
        debug_assert!(
            labels.len() == self.label_names.len()
                && labels
                    .iter()
                    .zip(&self.label_names)
                    .all(|((k, _), declared)| k == declared),
            "label set {:?} does not match declared names {:?} — \
             Prometheus requires a consistent label set per metric family",
            labels.iter().map(|(k, _)| *k).collect::<Vec<_>>(),
            self.label_names,
        );
    }

    /// Get or create a counter for the given label values
    pub async fn with_labels(&self, labels: &[(&str, &str)]) -> Arc<Counter> {
        self.assert_label_schema(labels);

        let key: Vec<_> = labels
            .iter()
            .map(|(k, v)| (k.to_string(), v.to_string()))
            .collect();

        // Fast path: read lock
        {
            let counters = self.counters.read().await;
            if let Some(counter) = counters.get(&key) {
                return counter.clone();
            }
        }

        // Slow path: write lock
        let mut counters = self.counters.write().await;
        counters
            .entry(key)
            .or_insert_with(|| Arc::new(Counter::new()))
            .clone()
    }

    /// Get all counters for export
    pub async fn all(&self) -> Vec<(Vec<(String, String)>, u64)> {
        let counters = self.counters.read().await;
        counters
            .iter()
            .map(|(labels, counter)| (labels.clone(), counter.get()))
            .collect()
    }
}

/// A histogram with labels
pub struct LabeledHistogram {
    histograms: HistogramSeries,
    label_names: Vec<String>,
    buckets: Vec<f64>,
}

impl LabeledHistogram {
    /// Create a new labeled histogram
    pub fn new(label_names: &[&str]) -> Self {
        Self {
            histograms: RwLock::new(HashMap::new()),
            label_names: label_names.iter().map(|s| s.to_string()).collect(),
            buckets: vec![
                0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
            ],
        }
    }

    /// Enforce the same label-schema invariant as [`LabeledCounter`]; see
    /// `LabeledCounter::assert_label_schema` for why this is a `debug_assert`.
    fn assert_label_schema(&self, labels: &[(&str, &str)]) {
        debug_assert!(
            labels.len() == self.label_names.len()
                && labels
                    .iter()
                    .zip(&self.label_names)
                    .all(|((k, _), declared)| k == declared),
            "label set {:?} does not match declared names {:?} — \
             Prometheus requires a consistent label set per metric family",
            labels.iter().map(|(k, _)| *k).collect::<Vec<_>>(),
            self.label_names,
        );
    }

    /// Get or create a histogram for the given label values
    pub async fn with_labels(&self, labels: &[(&str, &str)]) -> Arc<Histogram> {
        self.assert_label_schema(labels);

        let key: Vec<_> = labels
            .iter()
            .map(|(k, v)| (k.to_string(), v.to_string()))
            .collect();

        // Fast path
        {
            let histograms = self.histograms.read().await;
            if let Some(hist) = histograms.get(&key) {
                return hist.clone();
            }
        }

        // Slow path
        let mut histograms = self.histograms.write().await;
        histograms
            .entry(key)
            .or_insert_with(|| Arc::new(Histogram::with_buckets(self.buckets.clone())))
            .clone()
    }

    /// Get all histograms for export
    pub async fn all(&self) -> Vec<(Vec<(String, String)>, Arc<Histogram>)> {
        let histograms = self.histograms.read().await;
        histograms
            .iter()
            .map(|(labels, hist)| (labels.clone(), hist.clone()))
            .collect()
    }
}

/// Main metrics registry for THE SHIELD
pub struct MetricsRegistry {
    /// Total requests received
    pub requests_total: LabeledCounter,
    /// Requests currently being processed
    pub requests_in_flight: Gauge,
    /// Request duration histogram
    pub request_duration: LabeledHistogram,
    /// Requests blocked by WAF
    pub blocked_requests: LabeledCounter,
    /// Rate limited requests
    pub rate_limited_requests: LabeledCounter,
    /// Backend errors
    pub backend_errors: LabeledCounter,
    /// Circuit breaker state changes
    pub circuit_breaker_state: LabeledCounter,
    /// Active connections
    pub active_connections: Gauge,
    /// Bytes received
    pub bytes_received: Counter,
    /// Bytes sent
    pub bytes_sent: Counter,
    /// Rule evaluation duration
    pub rule_evaluation_duration: Histogram,
    /// Start time for uptime calculation
    start_time: Instant,
}

impl MetricsRegistry {
    /// Create a new metrics registry
    pub fn new() -> Self {
        Self {
            requests_total: LabeledCounter::new(&["method", "path", "status"]),
            requests_in_flight: Gauge::new(),
            request_duration: LabeledHistogram::new(&["method", "path"]),
            blocked_requests: LabeledCounter::new(&["rule", "action"]),
            rate_limited_requests: LabeledCounter::new(&["path"]),
            backend_errors: LabeledCounter::new(&["backend", "error_type"]),
            circuit_breaker_state: LabeledCounter::new(&["backend", "state"]),
            active_connections: Gauge::new(),
            bytes_received: Counter::new(),
            bytes_sent: Counter::new(),
            rule_evaluation_duration: Histogram::with_buckets(vec![
                0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01,
            ]),
            start_time: Instant::now(),
        }
    }

    /// Get uptime in seconds
    pub fn uptime_seconds(&self) -> f64 {
        self.start_time.elapsed().as_secs_f64()
    }

    /// Export metrics in Prometheus format
    pub async fn export_prometheus(&self) -> String {
        let mut output = String::with_capacity(8192);

        // Uptime
        output.push_str("# HELP shield_uptime_seconds Time since the WAF started\n");
        output.push_str("# TYPE shield_uptime_seconds gauge\n");
        output.push_str(&format!(
            "shield_uptime_seconds {:.3}\n\n",
            self.uptime_seconds()
        ));

        // Requests in flight
        output.push_str(
            "# HELP shield_requests_in_flight Current number of requests being processed\n",
        );
        output.push_str("# TYPE shield_requests_in_flight gauge\n");
        output.push_str(&format!(
            "shield_requests_in_flight {}\n\n",
            self.requests_in_flight.get()
        ));

        // Active connections
        output.push_str("# HELP shield_active_connections Current number of active connections\n");
        output.push_str("# TYPE shield_active_connections gauge\n");
        output.push_str(&format!(
            "shield_active_connections {}\n\n",
            self.active_connections.get()
        ));

        // Bytes
        output.push_str("# HELP shield_bytes_received_total Total bytes received\n");
        output.push_str("# TYPE shield_bytes_received_total counter\n");
        output.push_str(&format!(
            "shield_bytes_received_total {}\n\n",
            self.bytes_received.get()
        ));

        output.push_str("# HELP shield_bytes_sent_total Total bytes sent\n");
        output.push_str("# TYPE shield_bytes_sent_total counter\n");
        output.push_str(&format!(
            "shield_bytes_sent_total {}\n\n",
            self.bytes_sent.get()
        ));

        // Requests total
        output.push_str("# HELP shield_requests_total Total number of HTTP requests\n");
        output.push_str("# TYPE shield_requests_total counter\n");
        for (labels, value) in self.requests_total.all().await {
            let label_str = format_labels(&labels);
            output.push_str(&format!(
                "shield_requests_total{{{}}} {}\n",
                label_str, value
            ));
        }
        output.push('\n');

        // Blocked requests
        output.push_str("# HELP shield_blocked_requests_total Requests blocked by WAF rules\n");
        output.push_str("# TYPE shield_blocked_requests_total counter\n");
        for (labels, value) in self.blocked_requests.all().await {
            let label_str = format_labels(&labels);
            output.push_str(&format!(
                "shield_blocked_requests_total{{{}}} {}\n",
                label_str, value
            ));
        }
        output.push('\n');

        // Rate limited requests
        output.push_str(
            "# HELP shield_rate_limited_requests_total Requests rejected by rate limiter\n",
        );
        output.push_str("# TYPE shield_rate_limited_requests_total counter\n");
        for (labels, value) in self.rate_limited_requests.all().await {
            let label_str = format_labels(&labels);
            output.push_str(&format!(
                "shield_rate_limited_requests_total{{{}}} {}\n",
                label_str, value
            ));
        }
        output.push('\n');

        // Backend errors
        output.push_str("# HELP shield_backend_errors_total Errors from backend servers\n");
        output.push_str("# TYPE shield_backend_errors_total counter\n");
        for (labels, value) in self.backend_errors.all().await {
            let label_str = format_labels(&labels);
            output.push_str(&format!(
                "shield_backend_errors_total{{{}}} {}\n",
                label_str, value
            ));
        }
        output.push('\n');

        // Circuit breaker
        output.push_str(
            "# HELP shield_circuit_breaker_state_changes_total Circuit breaker state changes\n",
        );
        output.push_str("# TYPE shield_circuit_breaker_state_changes_total counter\n");
        for (labels, value) in self.circuit_breaker_state.all().await {
            let label_str = format_labels(&labels);
            output.push_str(&format!(
                "shield_circuit_breaker_state_changes_total{{{}}} {}\n",
                label_str, value
            ));
        }
        output.push('\n');

        // Request duration histogram
        output.push_str("# HELP shield_request_duration_seconds Request duration in seconds\n");
        output.push_str("# TYPE shield_request_duration_seconds histogram\n");
        for (labels, hist) in self.request_duration.all().await {
            let label_str = format_labels(&labels);
            let mut cumulative = 0u64;

            for (bound, count) in hist.buckets() {
                cumulative += count;
                if label_str.is_empty() {
                    output.push_str(&format!(
                        "shield_request_duration_seconds_bucket{{le=\"{}\"}} {}\n",
                        bound, cumulative
                    ));
                } else {
                    output.push_str(&format!(
                        "shield_request_duration_seconds_bucket{{{},le=\"{}\"}} {}\n",
                        label_str, bound, cumulative
                    ));
                }
            }

            // +Inf bucket
            if label_str.is_empty() {
                output.push_str(&format!(
                    "shield_request_duration_seconds_bucket{{le=\"+Inf\"}} {}\n",
                    hist.count()
                ));
                output.push_str(&format!(
                    "shield_request_duration_seconds_sum {:.6}\n",
                    hist.sum()
                ));
                output.push_str(&format!(
                    "shield_request_duration_seconds_count {}\n",
                    hist.count()
                ));
            } else {
                output.push_str(&format!(
                    "shield_request_duration_seconds_bucket{{{},le=\"+Inf\"}} {}\n",
                    label_str,
                    hist.count()
                ));
                output.push_str(&format!(
                    "shield_request_duration_seconds_sum{{{}}} {:.6}\n",
                    label_str,
                    hist.sum()
                ));
                output.push_str(&format!(
                    "shield_request_duration_seconds_count{{{}}} {}\n",
                    label_str,
                    hist.count()
                ));
            }
        }
        output.push('\n');

        // Rule evaluation histogram
        output
            .push_str("# HELP shield_rule_evaluation_duration_seconds WAF rule evaluation time\n");
        output.push_str("# TYPE shield_rule_evaluation_duration_seconds histogram\n");
        let mut cumulative = 0u64;
        for (bound, count) in self.rule_evaluation_duration.buckets() {
            cumulative += count;
            output.push_str(&format!(
                "shield_rule_evaluation_duration_seconds_bucket{{le=\"{}\"}} {}\n",
                bound, cumulative
            ));
        }
        output.push_str(&format!(
            "shield_rule_evaluation_duration_seconds_bucket{{le=\"+Inf\"}} {}\n",
            self.rule_evaluation_duration.count()
        ));
        output.push_str(&format!(
            "shield_rule_evaluation_duration_seconds_sum {:.6}\n",
            self.rule_evaluation_duration.sum()
        ));
        output.push_str(&format!(
            "shield_rule_evaluation_duration_seconds_count {}\n",
            self.rule_evaluation_duration.count()
        ));

        output
    }
}

impl Default for MetricsRegistry {
    fn default() -> Self {
        Self::new()
    }
}

/// Format labels for Prometheus output
fn format_labels(labels: &[(String, String)]) -> String {
    labels
        .iter()
        .map(|(k, v)| format!("{}=\"{}\"", k, escape_label_value(v)))
        .collect::<Vec<_>>()
        .join(",")
}

/// Escape special characters in label values
fn escape_label_value(s: &str) -> String {
    let mut result = String::with_capacity(s.len());
    for c in s.chars() {
        match c {
            '\\' => result.push_str("\\\\"),
            '"' => result.push_str("\\\""),
            '\n' => result.push_str("\\n"),
            c => result.push(c),
        }
    }
    result
}

/// Timer for measuring duration
pub struct Timer {
    start: Instant,
}

impl Timer {
    /// Start a new timer
    pub fn start() -> Self {
        Self {
            start: Instant::now(),
        }
    }

    /// Get elapsed duration
    pub fn elapsed(&self) -> Duration {
        self.start.elapsed()
    }

    /// Observe the elapsed time in a histogram
    pub fn observe_in(self, histogram: &Histogram) {
        histogram.observe_duration(self.elapsed());
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn declared_label_names_are_enforced() {
        let counter = LabeledCounter::new(&["method", "path"]);

        // The declared schema, in the declared order, is accepted.
        counter
            .with_labels(&[("method", "GET"), ("path", "/")])
            .await;
    }

    #[tokio::test]
    #[should_panic(expected = "does not match declared names")]
    async fn wrong_label_order_is_rejected() {
        // Same names, swapped order. Prometheus treats this as a different
        // label set for the same family, so the export would disagree with
        // itself. Before `label_names` was actually read, this passed
        // silently.
        let counter = LabeledCounter::new(&["method", "path"]);
        counter
            .with_labels(&[("path", "/"), ("method", "GET")])
            .await;
    }

    #[tokio::test]
    #[should_panic(expected = "does not match declared names")]
    async fn undeclared_label_is_rejected() {
        let hist = LabeledHistogram::new(&["method"]);
        hist.with_labels(&[("backend", "api")]).await;
    }

    #[test]
    fn test_counter() {
        let counter = Counter::new();
        assert_eq!(counter.get(), 0);

        counter.inc();
        assert_eq!(counter.get(), 1);

        counter.add(5);
        assert_eq!(counter.get(), 6);

        counter.reset();
        assert_eq!(counter.get(), 0);
    }

    #[test]
    fn test_gauge() {
        let gauge = Gauge::new();
        assert_eq!(gauge.get(), 0);

        gauge.set(100);
        assert_eq!(gauge.get(), 100);

        gauge.inc();
        assert_eq!(gauge.get(), 101);

        gauge.dec();
        assert_eq!(gauge.get(), 100);

        gauge.add(-50);
        assert_eq!(gauge.get(), 50);
    }

    #[test]
    fn test_histogram() {
        let hist = Histogram::with_buckets(vec![0.1, 0.5, 1.0]);

        hist.observe(0.05);
        hist.observe(0.25);
        hist.observe(0.75);
        hist.observe(2.0);

        assert_eq!(hist.count(), 4);

        let buckets = hist.buckets();
        assert_eq!(buckets[0].1, 1); // <= 0.1
        assert_eq!(buckets[1].1, 1); // <= 0.5
        assert_eq!(buckets[2].1, 1); // <= 1.0 (overflow goes here)
    }

    #[tokio::test]
    async fn test_labeled_counter() {
        let counter = LabeledCounter::new(&["method", "status"]);

        let c1 = counter
            .with_labels(&[("method", "GET"), ("status", "200")])
            .await;
        c1.inc();
        c1.inc();

        let c2 = counter
            .with_labels(&[("method", "POST"), ("status", "201")])
            .await;
        c2.inc();

        let all = counter.all().await;
        assert_eq!(all.len(), 2);
    }

    #[tokio::test]
    async fn test_metrics_export() {
        let registry = MetricsRegistry::new();

        // Record some metrics
        registry.requests_in_flight.inc();
        registry.bytes_received.add(1024);

        let counter = registry
            .requests_total
            .with_labels(&[("method", "GET"), ("path", "/api"), ("status", "200")])
            .await;
        counter.inc();

        let output = registry.export_prometheus().await;

        assert!(output.contains("shield_uptime_seconds"));
        assert!(output.contains("shield_requests_in_flight 1"));
        assert!(output.contains("shield_bytes_received_total 1024"));
        assert!(output.contains("shield_requests_total"));
    }

    #[test]
    fn test_timer() {
        let timer = Timer::start();
        std::thread::sleep(Duration::from_millis(10));
        let elapsed = timer.elapsed();
        assert!(elapsed >= Duration::from_millis(10));
    }
}
