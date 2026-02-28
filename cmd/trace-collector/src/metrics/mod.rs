//! Metrics module for THE WHISPERING VOID.
//!
//! Provides Prometheus metrics and HTTP server for observability.
//!
//! ## Metrics Exposed
//!
//! - `trace_collector_events_total` - Counter of events received by type/source
//! - `trace_collector_events_published_total` - Counter of events published
//! - `trace_collector_batch_size` - Histogram of batch sizes
//! - `trace_collector_publish_latency_seconds` - Histogram of publish latency
//! - `trace_collector_ringbuf_reads_total` - Counter of ring buffer reads
//! - `trace_collector_errors_total` - Counter of errors by type
//! - `trace_collector_events_dropped_total` - Counter of dropped events
//! - `trace_collector_queue_depth` - Gauge of current queue depth
//! - `trace_collector_connection_state` - Gauge of connection state
//! - `trace_collector_uptime_seconds` - Gauge of uptime

pub mod server;

// Re-export server types
pub use server::{
    record_batch_size, record_events_published, set_connection_state,
    set_queue_depth, MetricsHttpServer, ServerConfig,
};

use std::sync::atomic::{AtomicU64, Ordering};

use anyhow::Result;
use once_cell::sync::Lazy;
use prometheus::{
    Gauge, GaugeVec, Histogram, HistogramOpts, Opts,
    Registry,
};

/// Global metrics registry (shared with server module)
pub(crate) static REGISTRY: Lazy<Registry> = Lazy::new(Registry::new);

// ============================================================================
// ADDITIONAL METRICS (complementing server.rs)
// ============================================================================

/// Processing latency (nanoseconds)
static PROCESSING_LATENCY_NS: Lazy<Histogram> = Lazy::new(|| {
    let opts = HistogramOpts::new(
        "trace_collector_processing_latency_ns",
        "Event processing latency in nanoseconds",
    )
    .namespace("whispering_void")
    .buckets(vec![
        100.0, 500.0, 1000.0, 5000.0, 10000.0, 50000.0, 100000.0, 500000.0, 1000000.0,
    ]);
    let histogram = Histogram::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(histogram.clone())).unwrap();
    histogram
});

/// Ring buffer positions
static RINGBUF_CONSUMER_POS: Lazy<Gauge> = Lazy::new(|| {
    let opts = Opts::new(
        "trace_collector_ringbuf_consumer_pos",
        "Ring buffer consumer position",
    )
    .namespace("whispering_void");
    let gauge = Gauge::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(gauge.clone())).unwrap();
    gauge
});

static RINGBUF_PRODUCER_POS: Lazy<Gauge> = Lazy::new(|| {
    let opts = Opts::new(
        "trace_collector_ringbuf_producer_pos",
        "Ring buffer producer position",
    )
    .namespace("whispering_void");
    let gauge = Gauge::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(gauge.clone())).unwrap();
    gauge
});

/// Perf events by CPU
static PERF_EVENTS_CPU: Lazy<GaugeVec> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_perf_events_cpu", "Perf events by CPU")
        .namespace("whispering_void");
    let gauge = GaugeVec::new(opts, &["cpu"]).unwrap();
    REGISTRY.register(Box::new(gauge.clone())).unwrap();
    gauge
});

/// High-performance atomic counters for hot paths
pub struct AtomicMetrics {
    pub events_received: AtomicU64,
    pub events_published: AtomicU64,
    pub events_dropped: AtomicU64,
    pub bytes_processed: AtomicU64,
}

impl AtomicMetrics {
    pub const fn new() -> Self {
        Self {
            events_received: AtomicU64::new(0),
            events_published: AtomicU64::new(0),
            events_dropped: AtomicU64::new(0),
            bytes_processed: AtomicU64::new(0),
        }
    }

    #[inline]
    pub fn inc_received(&self) {
        self.events_received.fetch_add(1, Ordering::Relaxed);
    }

    #[inline]
    pub fn inc_published(&self, count: u64) {
        self.events_published.fetch_add(count, Ordering::Relaxed);
    }

    #[inline]
    pub fn inc_dropped(&self) {
        self.events_dropped.fetch_add(1, Ordering::Relaxed);
    }

    #[inline]
    pub fn add_bytes(&self, bytes: u64) {
        self.bytes_processed.fetch_add(bytes, Ordering::Relaxed);
    }
}

/// Global atomic metrics for hot paths
pub static ATOMIC_METRICS: AtomicMetrics = AtomicMetrics::new();

// ============================================================================
// METRIC RECORDING FUNCTIONS (hot path optimized)
// ============================================================================

/// Record event received (hot path optimized)
#[inline]
pub fn record_event_received(event_type: &str, source: &str) {
    server::record_event(event_type, source);
    ATOMIC_METRICS.inc_received();
}

/// Record event dropped
#[inline]
pub fn record_event_dropped() {
    server::record_dropped();
    ATOMIC_METRICS.inc_dropped();
}

/// Record processing latency in nanoseconds
#[inline]
pub fn record_processing_latency_ns(latency_ns: u64) {
    PROCESSING_LATENCY_NS.observe(latency_ns as f64);
}

/// Update ring buffer positions
pub fn set_ringbuf_positions(consumer: u64, producer: u64) {
    RINGBUF_CONSUMER_POS.set(consumer as f64);
    RINGBUF_PRODUCER_POS.set(producer as f64);
}

/// Update perf events for a CPU
pub fn set_perf_events_cpu(cpu: usize, count: u64) {
    PERF_EVENTS_CPU
        .with_label_values(&[&cpu.to_string()])
        .set(count as f64);
}

/// Record publish latency (microseconds, for backward compatibility)
pub fn record_publish_latency(latency_us: f64, success: bool) {
    // Convert to seconds for the histogram
    server::record_publish_latency_seconds(latency_us / 1_000_000.0, success);
}

// ============================================================================
// LEGACY METRICS SERVER (for backward compatibility)
// ============================================================================

/// Legacy metrics server wrapper
pub struct MetricsServer {
    inner: MetricsHttpServer,
}

impl MetricsServer {
    /// Create a new metrics server
    pub fn new(addr: &str) -> Result<Self> {
        Ok(Self {
            inner: MetricsHttpServer::from_addr(addr)?,
        })
    }

    /// Run the server
    pub async fn run(self) -> Result<()> {
        self.inner.run().await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_atomic_metrics() {
        let metrics = AtomicMetrics::new();
        metrics.inc_received();
        metrics.inc_received();
        assert_eq!(metrics.events_received.load(Ordering::Relaxed), 2);

        metrics.inc_published(10);
        assert_eq!(metrics.events_published.load(Ordering::Relaxed), 10);
    }

    #[test]
    fn test_record_functions() {
        // These should not panic
        record_event_received("packet", "ringbuf");
        record_event_dropped();
        record_processing_latency_ns(1000);
        set_ringbuf_positions(100, 200);
        set_perf_events_cpu(0, 50);
    }
}
