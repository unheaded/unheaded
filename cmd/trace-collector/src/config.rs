//! Configuration management for the trace collector.

use std::path::Path;

use anyhow::Result;
use serde::{Deserialize, Serialize};

/// Main configuration structure
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    /// Size of the event queue (for backpressure handling)
    #[serde(default = "default_event_queue_size")]
    pub event_queue_size: usize,

    /// Ring buffer size in bytes (must be power of 2)
    #[serde(default = "default_ringbuf_size")]
    pub ringbuf_size: usize,

    /// Number of pages for perf event buffer (per CPU)
    #[serde(default = "default_perf_buffer_pages")]
    pub perf_buffer_pages: usize,

    /// Busboy publisher configuration
    #[serde(default)]
    pub publisher: PublisherConfig,

    /// Event filtering configuration
    #[serde(default)]
    pub filters: FilterConfig,

    /// Metrics configuration
    #[serde(default)]
    pub metrics: MetricsConfig,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            event_queue_size: default_event_queue_size(),
            ringbuf_size: default_ringbuf_size(),
            perf_buffer_pages: default_perf_buffer_pages(),
            publisher: PublisherConfig::default(),
            filters: FilterConfig::default(),
            metrics: MetricsConfig::default(),
        }
    }
}

impl Config {
    /// Load configuration from a TOML file
    pub fn from_file(path: &Path) -> Result<Self> {
        let content = std::fs::read_to_string(path)?;
        let config: Config = toml::from_str(&content)?;
        Ok(config)
    }
}

/// Publisher configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PublisherConfig {
    /// Maximum batch size
    #[serde(default = "default_batch_size")]
    pub batch_size: usize,

    /// Batch timeout in milliseconds
    #[serde(default = "default_batch_timeout_ms")]
    pub batch_timeout_ms: u64,

    /// Maximum retries for failed publishes
    #[serde(default = "default_max_retries")]
    pub max_retries: u32,

    /// Retry backoff base in milliseconds
    #[serde(default = "default_retry_backoff_ms")]
    pub retry_backoff_ms: u64,

    /// Enable compression
    #[serde(default = "default_compression")]
    pub compression: bool,

    /// Connection timeout in milliseconds
    #[serde(default = "default_connect_timeout_ms")]
    pub connect_timeout_ms: u64,
}

impl Default for PublisherConfig {
    fn default() -> Self {
        Self {
            batch_size: default_batch_size(),
            batch_timeout_ms: default_batch_timeout_ms(),
            max_retries: default_max_retries(),
            retry_backoff_ms: default_retry_backoff_ms(),
            compression: default_compression(),
            connect_timeout_ms: default_connect_timeout_ms(),
        }
    }
}

/// Event filtering configuration
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct FilterConfig {
    /// Enable packet events
    #[serde(default = "default_true")]
    pub packets: bool,

    /// Enable syscall events
    #[serde(default = "default_true")]
    pub syscalls: bool,

    /// Enable latency events
    #[serde(default = "default_true")]
    pub latency: bool,

    /// Filter by process name (regex)
    pub process_filter: Option<String>,

    /// Filter by cgroup path (regex)
    pub cgroup_filter: Option<String>,

    /// Minimum latency threshold in nanoseconds (for latency events)
    #[serde(default)]
    pub min_latency_ns: u64,
}

/// Metrics configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MetricsConfig {
    /// Enable metrics collection
    #[serde(default = "default_true")]
    pub enabled: bool,

    /// Metrics endpoint path
    #[serde(default = "default_metrics_path")]
    pub path: String,

    /// Include detailed histograms
    #[serde(default)]
    pub histograms: bool,
}

impl Default for MetricsConfig {
    fn default() -> Self {
        Self {
            enabled: default_true(),
            path: default_metrics_path(),
            histograms: false,
        }
    }
}

// Default value functions
fn default_event_queue_size() -> usize {
    65536
}

fn default_ringbuf_size() -> usize {
    16 * 1024 * 1024 // 16 MB
}

fn default_perf_buffer_pages() -> usize {
    64 // 256 KB per CPU
}

fn default_batch_size() -> usize {
    1000
}

fn default_batch_timeout_ms() -> u64 {
    10
}

fn default_max_retries() -> u32 {
    3
}

fn default_retry_backoff_ms() -> u64 {
    100
}

fn default_compression() -> bool {
    true
}

fn default_connect_timeout_ms() -> u64 {
    5000
}

fn default_true() -> bool {
    true
}

fn default_metrics_path() -> String {
    "/metrics".to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_config() {
        let config = Config::default();
        assert_eq!(config.event_queue_size, 65536);
        assert_eq!(config.ringbuf_size, 16 * 1024 * 1024);
    }

    #[test]
    fn test_config_from_toml() {
        let toml = r#"
            event_queue_size = 32768
            ringbuf_size = 8388608

            [publisher]
            batch_size = 500
            compression = false

            [filters]
            packets = true
            syscalls = false
        "#;

        let config: Config = toml::from_str(toml).unwrap();
        assert_eq!(config.event_queue_size, 32768);
        assert_eq!(config.publisher.batch_size, 500);
        assert!(!config.publisher.compression);
        assert!(config.filters.packets);
        assert!(!config.filters.syscalls);
    }
}
