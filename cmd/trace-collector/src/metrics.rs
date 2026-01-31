//! Prometheus metrics endpoint for the trace collector.
//!
//! Provides internal observability for:
//! - Events processed (by type)
//! - Publishing latency
//! - Backpressure events
//! - BPF reader statistics

use std::net::SocketAddr;
use std::sync::atomic::{AtomicU64, Ordering};

use anyhow::Result;
use hyper::server::conn::http1;
use hyper::service::service_fn;
use hyper::{body::Incoming, Request, Response, StatusCode};
use hyper_util::rt::TokioIo;
use http_body_util::Full;
use bytes::Bytes;
use once_cell::sync::Lazy;
use prometheus::{
    Counter, CounterVec, Gauge, GaugeVec, Histogram, HistogramOpts, HistogramVec, Opts, Registry,
    TextEncoder,
};
use tokio::net::TcpListener;
use tracing::{debug, error, info};

/// Global metrics registry
static REGISTRY: Lazy<Registry> = Lazy::new(Registry::new);

/// Events received counter (by type and source)
static EVENTS_RECEIVED: Lazy<CounterVec> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_events_received_total", "Total events received")
        .namespace("whispering_void");
    let counter = CounterVec::new(opts, &["type", "source"]).unwrap();
    REGISTRY.register(Box::new(counter.clone())).unwrap();
    counter
});

/// Events published counter
static EVENTS_PUBLISHED: Lazy<CounterVec> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_events_published_total", "Total events published")
        .namespace("whispering_void");
    let counter = CounterVec::new(opts, &["status"]).unwrap();
    REGISTRY.register(Box::new(counter.clone())).unwrap();
    counter
});

/// Events dropped counter (due to backpressure)
static EVENTS_DROPPED: Lazy<Counter> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_events_dropped_total", "Events dropped due to backpressure")
        .namespace("whispering_void");
    let counter = Counter::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(counter.clone())).unwrap();
    counter
});

/// Batch size histogram
static BATCH_SIZE: Lazy<Histogram> = Lazy::new(|| {
    let opts = HistogramOpts::new("trace_collector_batch_size", "Event batch sizes")
        .namespace("whispering_void")
        .buckets(vec![1.0, 10.0, 50.0, 100.0, 500.0, 1000.0, 5000.0]);
    let histogram = Histogram::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(histogram.clone())).unwrap();
    histogram
});

/// Publishing latency histogram (microseconds)
static PUBLISH_LATENCY: Lazy<HistogramVec> = Lazy::new(|| {
    let opts = HistogramOpts::new("trace_collector_publish_latency_us", "Publishing latency in microseconds")
        .namespace("whispering_void")
        .buckets(vec![1.0, 5.0, 10.0, 50.0, 100.0, 500.0, 1000.0, 5000.0, 10000.0]);
    let histogram = HistogramVec::new(opts, &["result"]).unwrap();
    REGISTRY.register(Box::new(histogram.clone())).unwrap();
    histogram
});

/// Event processing latency (nanoseconds)
static PROCESSING_LATENCY_NS: Lazy<Histogram> = Lazy::new(|| {
    let opts = HistogramOpts::new("trace_collector_processing_latency_ns", "Event processing latency in nanoseconds")
        .namespace("whispering_void")
        .buckets(vec![100.0, 500.0, 1000.0, 5000.0, 10000.0, 50000.0, 100000.0]);
    let histogram = Histogram::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(histogram.clone())).unwrap();
    histogram
});

/// Queue depth gauge
static QUEUE_DEPTH: Lazy<Gauge> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_queue_depth", "Current event queue depth")
        .namespace("whispering_void");
    let gauge = Gauge::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(gauge.clone())).unwrap();
    gauge
});

/// Ring buffer consumer position
static RINGBUF_CONSUMER_POS: Lazy<Gauge> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_ringbuf_consumer_pos", "Ring buffer consumer position")
        .namespace("whispering_void");
    let gauge = Gauge::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(gauge.clone())).unwrap();
    gauge
});

/// Ring buffer producer position
static RINGBUF_PRODUCER_POS: Lazy<Gauge> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_ringbuf_producer_pos", "Ring buffer producer position")
        .namespace("whispering_void");
    let gauge = Gauge::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(gauge.clone())).unwrap();
    gauge
});

/// Perf events per CPU
static PERF_EVENTS_CPU: Lazy<GaugeVec> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_perf_events", "Perf events by CPU")
        .namespace("whispering_void");
    let gauge = GaugeVec::new(opts, &["cpu"]).unwrap();
    REGISTRY.register(Box::new(gauge.clone())).unwrap();
    gauge
});

/// Connection state gauge
static CONNECTION_STATE: Lazy<GaugeVec> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_connection_state", "Connection state (1=connected, 0=disconnected)")
        .namespace("whispering_void");
    let gauge = GaugeVec::new(opts, &["endpoint"]).unwrap();
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

/// Record an event received
pub fn record_event_received(event_type: &str, source: &str) {
    EVENTS_RECEIVED.with_label_values(&[event_type, source]).inc();
    ATOMIC_METRICS.inc_received();
}

/// Record events published
pub fn record_events_published(count: usize, success: bool) {
    let status = if success { "success" } else { "error" };
    EVENTS_PUBLISHED.with_label_values(&[status]).inc_by(count as f64);
    if success {
        ATOMIC_METRICS.inc_published(count as u64);
    }
}

/// Record an event dropped
pub fn record_event_dropped() {
    EVENTS_DROPPED.inc();
    ATOMIC_METRICS.inc_dropped();
}

/// Record batch size
pub fn record_batch_size(size: usize) {
    BATCH_SIZE.observe(size as f64);
}

/// Record publish latency in microseconds
pub fn record_publish_latency(latency_us: f64, success: bool) {
    let result = if success { "success" } else { "error" };
    PUBLISH_LATENCY.with_label_values(&[result]).observe(latency_us);
}

/// Record processing latency in nanoseconds
pub fn record_processing_latency_ns(latency_ns: u64) {
    PROCESSING_LATENCY_NS.observe(latency_ns as f64);
}

/// Update queue depth
pub fn set_queue_depth(depth: usize) {
    QUEUE_DEPTH.set(depth as f64);
}

/// Update ring buffer positions
pub fn set_ringbuf_positions(consumer: u64, producer: u64) {
    RINGBUF_CONSUMER_POS.set(consumer as f64);
    RINGBUF_PRODUCER_POS.set(producer as f64);
}

/// Update perf events for a CPU
pub fn set_perf_events_cpu(cpu: usize, count: u64) {
    PERF_EVENTS_CPU.with_label_values(&[&cpu.to_string()]).set(count as f64);
}

/// Set connection state
pub fn set_connection_state(endpoint: &str, connected: bool) {
    CONNECTION_STATE.with_label_values(&[endpoint]).set(if connected { 1.0 } else { 0.0 });
}

/// Prometheus metrics server
pub struct MetricsServer {
    addr: SocketAddr,
}

impl MetricsServer {
    pub fn new(addr: &str) -> Result<Self> {
        let addr: SocketAddr = addr.parse()?;
        Ok(Self { addr })
    }

    pub async fn run(self) -> Result<()> {
        let listener = TcpListener::bind(self.addr).await?;
        info!(addr = %self.addr, "Metrics server listening");

        loop {
            let (stream, remote_addr) = listener.accept().await?;
            debug!(remote_addr = %remote_addr, "Metrics request");

            let io = TokioIo::new(stream);

            tokio::spawn(async move {
                if let Err(e) = http1::Builder::new()
                    .serve_connection(io, service_fn(handle_metrics))
                    .await
                {
                    error!(error = %e, "Metrics connection error");
                }
            });
        }
    }
}

async fn handle_metrics(req: Request<Incoming>) -> Result<Response<Full<Bytes>>, hyper::Error> {
    match req.uri().path() {
        "/metrics" => {
            let encoder = TextEncoder::new();
            let metric_families = REGISTRY.gather();
            let mut buffer = String::new();

            if let Err(e) = encoder.encode_utf8(&metric_families, &mut buffer) {
                error!(error = %e, "Failed to encode metrics");
                return Ok(Response::builder()
                    .status(StatusCode::INTERNAL_SERVER_ERROR)
                    .body(Full::new(Bytes::from("Failed to encode metrics")))
                    .unwrap());
            }

            Ok(Response::builder()
                .status(StatusCode::OK)
                .header("Content-Type", "text/plain; charset=utf-8")
                .body(Full::new(Bytes::from(buffer)))
                .unwrap())
        }
        "/health" | "/healthz" => Ok(Response::builder()
            .status(StatusCode::OK)
            .body(Full::new(Bytes::from("OK")))
            .unwrap()),
        "/ready" | "/readyz" => {
            // Check if we're ready to receive events
            let atomic_received = ATOMIC_METRICS.events_received.load(Ordering::Relaxed);
            if atomic_received > 0 {
                Ok(Response::builder()
                    .status(StatusCode::OK)
                    .body(Full::new(Bytes::from("READY")))
                    .unwrap())
            } else {
                Ok(Response::builder()
                    .status(StatusCode::SERVICE_UNAVAILABLE)
                    .body(Full::new(Bytes::from("NOT_READY")))
                    .unwrap())
            }
        }
        _ => Ok(Response::builder()
            .status(StatusCode::NOT_FOUND)
            .body(Full::new(Bytes::from("Not Found")))
            .unwrap()),
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
}
