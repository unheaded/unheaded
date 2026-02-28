//! Prometheus metrics HTTP server.
//!
//! THE WHISPERING VOID reveals its secrets through metrics,
//! observable by those who seek to understand the kingdom's health.
//!
//! This module provides an HTTP server that exposes Prometheus metrics
//! for monitoring the trace collector.

use std::net::SocketAddr;
use std::time::Instant;

use anyhow::Result;
use bytes::Bytes;
use http_body_util::Full;
use hyper::server::conn::http1;
use hyper::service::service_fn;
use hyper::{body::Incoming, Request, Response, StatusCode};
use hyper_util::rt::TokioIo;
use once_cell::sync::Lazy;
use prometheus::{
    Counter, CounterVec, Gauge, GaugeVec, Histogram, HistogramOpts, HistogramVec, Opts,
    Registry, TextEncoder,
};
use tokio::net::TcpListener;
use tokio::sync::broadcast;
use tracing::{debug, error, info, warn};

/// Prometheus metrics registry
static REGISTRY: Lazy<Registry> = Lazy::new(Registry::new);

// ============================================================================
// METRIC DEFINITIONS
// ============================================================================

/// Events received total (by type and source)
static EVENTS_TOTAL: Lazy<CounterVec> = Lazy::new(|| {
    let opts = Opts::new(
        "trace_collector_events_total",
        "Total events received by type and source",
    )
    .namespace("whispering_void");
    let counter = CounterVec::new(opts, &["type", "source"]).unwrap();
    REGISTRY.register(Box::new(counter.clone())).unwrap();
    counter
});

/// Events published total
static EVENTS_PUBLISHED_TOTAL: Lazy<CounterVec> = Lazy::new(|| {
    let opts = Opts::new(
        "trace_collector_events_published_total",
        "Total events published to Wotan",
    )
    .namespace("whispering_void");
    let counter = CounterVec::new(opts, &["status"]).unwrap();
    REGISTRY.register(Box::new(counter.clone())).unwrap();
    counter
});

/// Batch size histogram
static BATCH_SIZE: Lazy<Histogram> = Lazy::new(|| {
    let opts = HistogramOpts::new(
        "trace_collector_batch_size",
        "Event batch sizes",
    )
    .namespace("whispering_void")
    .buckets(vec![1.0, 10.0, 50.0, 100.0, 250.0, 500.0, 1000.0, 2500.0, 5000.0]);
    let histogram = Histogram::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(histogram.clone())).unwrap();
    histogram
});

/// Publish latency histogram (seconds)
static PUBLISH_LATENCY_SECONDS: Lazy<HistogramVec> = Lazy::new(|| {
    let opts = HistogramOpts::new(
        "trace_collector_publish_latency_seconds",
        "Publishing latency in seconds",
    )
    .namespace("whispering_void")
    .buckets(vec![
        0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0,
    ]);
    let histogram = HistogramVec::new(opts, &["result"]).unwrap();
    REGISTRY.register(Box::new(histogram.clone())).unwrap();
    histogram
});

/// Ring buffer reads total
static RINGBUF_READS_TOTAL: Lazy<Counter> = Lazy::new(|| {
    let opts = Opts::new(
        "trace_collector_ringbuf_reads_total",
        "Total ring buffer read operations",
    )
    .namespace("whispering_void");
    let counter = Counter::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(counter.clone())).unwrap();
    counter
});

/// Errors total (by type)
static ERRORS_TOTAL: Lazy<CounterVec> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_errors_total", "Total errors by type")
        .namespace("whispering_void");
    let counter = CounterVec::new(opts, &["error_type"]).unwrap();
    REGISTRY.register(Box::new(counter.clone())).unwrap();
    counter
});

/// Events dropped total
static EVENTS_DROPPED_TOTAL: Lazy<Counter> = Lazy::new(|| {
    let opts = Opts::new(
        "trace_collector_events_dropped_total",
        "Total events dropped due to backpressure",
    )
    .namespace("whispering_void");
    let counter = Counter::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(counter.clone())).unwrap();
    counter
});

/// Current queue depth
static QUEUE_DEPTH: Lazy<Gauge> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_queue_depth", "Current event queue depth")
        .namespace("whispering_void");
    let gauge = Gauge::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(gauge.clone())).unwrap();
    gauge
});

/// Connection state
static CONNECTION_STATE: Lazy<GaugeVec> = Lazy::new(|| {
    let opts = Opts::new(
        "trace_collector_connection_state",
        "Connection state (1=connected, 0=disconnected)",
    )
    .namespace("whispering_void");
    let gauge = GaugeVec::new(opts, &["endpoint"]).unwrap();
    REGISTRY.register(Box::new(gauge.clone())).unwrap();
    gauge
});

/// Uptime gauge
static UPTIME_SECONDS: Lazy<Gauge> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_uptime_seconds", "Collector uptime in seconds")
        .namespace("whispering_void");
    let gauge = Gauge::with_opts(opts).unwrap();
    REGISTRY.register(Box::new(gauge.clone())).unwrap();
    gauge
});

/// Build info
static BUILD_INFO: Lazy<GaugeVec> = Lazy::new(|| {
    let opts = Opts::new("trace_collector_build_info", "Build information")
        .namespace("whispering_void");
    let gauge = GaugeVec::new(opts, &["version", "rust_version"]).unwrap();
    REGISTRY.register(Box::new(gauge.clone())).unwrap();
    gauge
});

// ============================================================================
// METRIC RECORDING FUNCTIONS
// ============================================================================

/// Record an event received
pub fn record_event(event_type: &str, source: &str) {
    EVENTS_TOTAL.with_label_values(&[event_type, source]).inc();
}

/// Record events published
pub fn record_events_published(count: usize, success: bool) {
    let status = if success { "success" } else { "error" };
    EVENTS_PUBLISHED_TOTAL
        .with_label_values(&[status])
        .inc_by(count as f64);
}

/// Record batch size
pub fn record_batch_size(size: usize) {
    BATCH_SIZE.observe(size as f64);
}

/// Record publish latency
pub fn record_publish_latency_seconds(latency_seconds: f64, success: bool) {
    let result = if success { "success" } else { "error" };
    PUBLISH_LATENCY_SECONDS
        .with_label_values(&[result])
        .observe(latency_seconds);
}

/// Record ring buffer read
pub fn record_ringbuf_read() {
    RINGBUF_READS_TOTAL.inc();
}

/// Record an error
pub fn record_error(error_type: &str) {
    ERRORS_TOTAL.with_label_values(&[error_type]).inc();
}

/// Record dropped event
pub fn record_dropped() {
    EVENTS_DROPPED_TOTAL.inc();
}

/// Set queue depth
pub fn set_queue_depth(depth: usize) {
    QUEUE_DEPTH.set(depth as f64);
}

/// Set connection state
pub fn set_connection_state(endpoint: &str, connected: bool) {
    CONNECTION_STATE
        .with_label_values(&[endpoint])
        .set(if connected { 1.0 } else { 0.0 });
}

/// Set uptime
pub fn set_uptime(seconds: f64) {
    UPTIME_SECONDS.set(seconds);
}

/// Set build info
pub fn set_build_info(version: &str, rust_version: &str) {
    BUILD_INFO
        .with_label_values(&[version, rust_version])
        .set(1.0);
}

// ============================================================================
// HTTP SERVER
// ============================================================================

/// Metrics server configuration
#[derive(Debug, Clone)]
pub struct ServerConfig {
    /// Bind address
    pub addr: SocketAddr,
    /// Metrics path
    pub metrics_path: String,
    /// Health path
    pub health_path: String,
    /// Ready path
    pub ready_path: String,
}

impl Default for ServerConfig {
    fn default() -> Self {
        Self {
            addr: "0.0.0.0:9090".parse().unwrap(),
            metrics_path: "/metrics".to_string(),
            health_path: "/health".to_string(),
            ready_path: "/ready".to_string(),
        }
    }
}

/// Metrics HTTP server
pub struct MetricsHttpServer {
    config: ServerConfig,
    start_time: Instant,
}

impl MetricsHttpServer {
    /// Create a new metrics server
    pub fn new(config: ServerConfig) -> Self {
        // Set build info
        set_build_info(
            env!("CARGO_PKG_VERSION"),
            option_env!("RUSTC_VERSION").unwrap_or("unknown"),
        );

        Self {
            config,
            start_time: Instant::now(),
        }
    }

    /// Create with default configuration
    pub fn with_defaults() -> Self {
        Self::new(ServerConfig::default())
    }

    /// Create from address string
    pub fn from_addr(addr: &str) -> Result<Self> {
        let socket_addr: SocketAddr = addr.parse()?;
        let mut config = ServerConfig::default();
        config.addr = socket_addr;
        Ok(Self::new(config))
    }

    /// Run the server
    pub async fn run(&self) -> Result<()> {
        let listener = TcpListener::bind(self.config.addr).await?;
        info!(addr = %self.config.addr, "Metrics server listening");

        loop {
            match listener.accept().await {
                Ok((stream, remote_addr)) => {
                    debug!(remote_addr = %remote_addr, "Metrics request");

                    let io = TokioIo::new(stream);
                    let start_time = self.start_time;
                    let config = self.config.clone();

                    tokio::spawn(async move {
                        if let Err(e) = http1::Builder::new()
                            .serve_connection(
                                io,
                                service_fn(|req| handle_request(req, start_time, config.clone())),
                            )
                            .await
                        {
                            error!(error = %e, "Metrics connection error");
                        }
                    });
                }
                Err(e) => {
                    warn!(error = %e, "Failed to accept connection");
                }
            }
        }
    }

    /// Run with shutdown signal
    pub async fn run_with_shutdown(
        &self,
        mut shutdown: broadcast::Receiver<()>,
    ) -> Result<()> {
        let listener = TcpListener::bind(self.config.addr).await?;
        info!(addr = %self.config.addr, "Metrics server listening");

        loop {
            tokio::select! {
                result = listener.accept() => {
                    match result {
                        Ok((stream, remote_addr)) => {
                            debug!(remote_addr = %remote_addr, "Metrics request");

                            let io = TokioIo::new(stream);
                            let start_time = self.start_time;
                            let config = self.config.clone();

                            tokio::spawn(async move {
                                if let Err(e) = http1::Builder::new()
                                    .serve_connection(
                                        io,
                                        service_fn(|req| handle_request(req, start_time, config.clone())),
                                    )
                                    .await
                                {
                                    error!(error = %e, "Metrics connection error");
                                }
                            });
                        }
                        Err(e) => {
                            warn!(error = %e, "Failed to accept connection");
                        }
                    }
                }
                _ = shutdown.recv() => {
                    info!("Metrics server shutting down");
                    break;
                }
            }
        }

        Ok(())
    }
}

/// Handle HTTP request
async fn handle_request(
    req: Request<Incoming>,
    start_time: Instant,
    config: ServerConfig,
) -> Result<Response<Full<Bytes>>, hyper::Error> {
    // Update uptime
    set_uptime(start_time.elapsed().as_secs_f64());

    match req.uri().path() {
        path if path == config.metrics_path => {
            // Gather and encode metrics
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

        path if path == config.health_path || path == "/healthz" => {
            Ok(Response::builder()
                .status(StatusCode::OK)
                .header("Content-Type", "application/json")
                .body(Full::new(Bytes::from(r#"{"status":"healthy"}"#)))
                .unwrap())
        }

        path if path == config.ready_path || path == "/readyz" => {
            // Check readiness (e.g., connected to Wotan)
            let ready = true; // In production, check actual connection state

            if ready {
                Ok(Response::builder()
                    .status(StatusCode::OK)
                    .header("Content-Type", "application/json")
                    .body(Full::new(Bytes::from(r#"{"status":"ready"}"#)))
                    .unwrap())
            } else {
                Ok(Response::builder()
                    .status(StatusCode::SERVICE_UNAVAILABLE)
                    .header("Content-Type", "application/json")
                    .body(Full::new(Bytes::from(r#"{"status":"not_ready"}"#)))
                    .unwrap())
            }
        }

        "/" => {
            let html = format!(
                r#"<!DOCTYPE html>
<html>
<head><title>Trace Collector Metrics</title></head>
<body>
<h1>THE WHISPERING VOID</h1>
<h2>Trace Collector v{}</h2>
<ul>
<li><a href="{}">Metrics</a></li>
<li><a href="{}">Health</a></li>
<li><a href="{}">Ready</a></li>
</ul>
</body>
</html>"#,
                env!("CARGO_PKG_VERSION"),
                config.metrics_path,
                config.health_path,
                config.ready_path
            );

            Ok(Response::builder()
                .status(StatusCode::OK)
                .header("Content-Type", "text/html; charset=utf-8")
                .body(Full::new(Bytes::from(html)))
                .unwrap())
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
    fn test_record_event() {
        record_event("packet", "ringbuf");
        // Metric should be recorded without error
    }

    #[test]
    fn test_record_batch_size() {
        record_batch_size(100);
        record_batch_size(500);
    }

    #[test]
    fn test_record_latency() {
        record_publish_latency_seconds(0.001, true);
        record_publish_latency_seconds(0.1, false);
    }

    #[test]
    fn test_server_config_default() {
        let config = ServerConfig::default();
        assert_eq!(config.metrics_path, "/metrics");
        assert_eq!(config.health_path, "/health");
    }
}
