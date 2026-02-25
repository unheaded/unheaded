//! Collector module - high-level eBPF event collection.
//!
//! THE WHISPERING VOID's ears: listening to the kernel's whispers
//! through ring buffers and perf events.
//!
//! This module provides a unified interface for collecting events from
//! multiple eBPF data sources:
//!
//! - Ring buffers (BPF_MAP_TYPE_RINGBUF) - modern, efficient, ordered
//! - Perf event arrays (BPF_MAP_TYPE_PERF_EVENT_ARRAY) - per-CPU, high throughput
//! - BPF maps - for flow state and statistics

pub mod events;
pub mod perf;
pub mod ringbuf;

use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use crossbeam::channel::{bounded, Receiver, Sender};
use parking_lot::RwLock;
use thiserror::Error;
use tokio::sync::broadcast;
use tracing::{debug, error, info, warn};

use crate::events::Event;
use crate::metrics;

pub use events::{EventFilter, EventProcessor};
pub use perf::PerfCollector;
pub use ringbuf::RingBufCollector;

/// Collector errors
#[derive(Error, Debug)]
pub enum CollectorError {
    #[error("Failed to initialize collector: {0}")]
    Init(String),

    #[error("BPF map not found: {0}")]
    MapNotFound(PathBuf),

    #[error("Reader error: {0}")]
    Reader(String),

    #[error("Channel error: {0}")]
    Channel(String),

    #[error("Shutdown requested")]
    Shutdown,
}

/// Collector statistics
#[derive(Debug, Default)]
pub struct CollectorStats {
    /// Total events read
    pub events_read: AtomicU64,
    /// Total events dropped
    pub events_dropped: AtomicU64,
    /// Total bytes processed
    pub bytes_processed: AtomicU64,
    /// Ring buffer reads
    pub ringbuf_reads: AtomicU64,
    /// Perf event reads
    pub perf_reads: AtomicU64,
    /// Read errors
    pub read_errors: AtomicU64,
    /// Last read timestamp
    pub last_read_ts: AtomicU64,
}

impl CollectorStats {
    pub fn new() -> Self {
        Self::default()
    }

    #[inline]
    pub fn record_read(&self, count: u64, bytes: u64, source: &str) {
        self.events_read.fetch_add(count, Ordering::Relaxed);
        self.bytes_processed.fetch_add(bytes, Ordering::Relaxed);
        self.last_read_ts.store(
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap_or_default()
                .as_nanos() as u64,
            Ordering::Relaxed,
        );

        match source {
            "ringbuf" => self.ringbuf_reads.fetch_add(1, Ordering::Relaxed),
            "perf" => self.perf_reads.fetch_add(1, Ordering::Relaxed),
            _ => 0,
        };
    }

    #[inline]
    pub fn record_drop(&self, count: u64) {
        self.events_dropped.fetch_add(count, Ordering::Relaxed);
    }

    #[inline]
    pub fn record_error(&self) {
        self.read_errors.fetch_add(1, Ordering::Relaxed);
    }
}

/// Collector state
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CollectorState {
    /// Initial state
    Idle,
    /// Starting up
    Starting,
    /// Running and collecting events
    Running,
    /// Gracefully stopping
    Stopping,
    /// Stopped
    Stopped,
    /// Error state
    Error,
}

/// Configuration for the collector
#[derive(Debug, Clone)]
pub struct CollectorConfig {
    /// Path to ring buffer map
    pub ringbuf_path: Option<PathBuf>,
    /// Path to perf event array map
    pub perf_path: Option<PathBuf>,
    /// Ring buffer size (for validation)
    pub ringbuf_size: usize,
    /// Perf buffer pages per CPU
    pub perf_buffer_pages: usize,
    /// Number of worker threads
    pub workers: usize,
    /// Event queue size
    pub event_queue_size: usize,
    /// Poll timeout in milliseconds
    pub poll_timeout_ms: u64,
    /// Enable event filtering
    pub filtering_enabled: bool,
    /// Event filter
    pub filter: Option<EventFilter>,
}

impl Default for CollectorConfig {
    fn default() -> Self {
        Self {
            ringbuf_path: Some(PathBuf::from("/sys/fs/bpf/unheaded/trace_events")),
            perf_path: Some(PathBuf::from("/sys/fs/bpf/unheaded/perf_events")),
            ringbuf_size: 16 * 1024 * 1024,
            perf_buffer_pages: 64,
            workers: 4,
            event_queue_size: 65536,
            poll_timeout_ms: 100,
            filtering_enabled: false,
            filter: None,
        }
    }
}

/// The main collector that orchestrates event collection from all sources
pub struct Collector {
    /// Configuration
    config: CollectorConfig,
    /// Current state
    state: RwLock<CollectorState>,
    /// Statistics
    stats: Arc<CollectorStats>,
    /// Shutdown flag
    shutdown: Arc<AtomicBool>,
    /// Event sender (to publisher)
    event_tx: Option<Sender<Event>>,
    /// Event receiver (for publisher)
    event_rx: Option<Receiver<Event>>,
}

impl Collector {
    /// Create a new collector with the given configuration
    pub fn new(config: CollectorConfig) -> Result<Self> {
        let (event_tx, event_rx) = bounded(config.event_queue_size);

        Ok(Self {
            config,
            state: RwLock::new(CollectorState::Idle),
            stats: Arc::new(CollectorStats::new()),
            shutdown: Arc::new(AtomicBool::new(false)),
            event_tx: Some(event_tx),
            event_rx: Some(event_rx),
        })
    }

    /// Take the event receiver (for the publisher)
    pub fn take_event_receiver(&mut self) -> Option<Receiver<Event>> {
        self.event_rx.take()
    }

    /// Get collector statistics
    pub fn stats(&self) -> &Arc<CollectorStats> {
        &self.stats
    }

    /// Get current state
    pub fn state(&self) -> CollectorState {
        *self.state.read()
    }

    /// Check if running
    pub fn is_running(&self) -> bool {
        self.state() == CollectorState::Running
    }

    /// Signal shutdown
    pub fn shutdown(&self) {
        self.shutdown.store(true, Ordering::SeqCst);
        *self.state.write() = CollectorState::Stopping;
    }

    /// Run the collector
    ///
    /// This spawns reader tasks for each configured data source and
    /// waits until shutdown is signaled.
    pub async fn run(&self) -> Result<()> {
        *self.state.write() = CollectorState::Starting;
        info!("Collector starting");

        let event_tx = match &self.event_tx {
            Some(tx) => tx.clone(),
            None => {
                return Err(anyhow::anyhow!("Event sender not available"));
            }
        };

        let mut handles = Vec::new();

        // Start ring buffer reader if configured
        if let Some(ref ringbuf_path) = self.config.ringbuf_path {
            if ringbuf_path.exists() {
                let reader = RingBufCollector::new(
                    ringbuf_path.clone(),
                    self.config.ringbuf_size,
                    Arc::clone(&self.stats),
                )?;

                let tx = event_tx.clone();
                let shutdown = Arc::clone(&self.shutdown);
                let filter = self.config.filter.clone();

                handles.push(tokio::spawn(async move {
                    reader.run(tx, shutdown, filter).await
                }));

                info!(path = %ringbuf_path.display(), "Ring buffer reader started");
            } else {
                warn!(path = %ringbuf_path.display(), "Ring buffer path not found");
            }
        }

        // Start perf event readers if configured
        if let Some(ref perf_path) = self.config.perf_path {
            if perf_path.exists() {
                let num_cpus = std::thread::available_parallelism()
                    .map(|p| p.get())
                    .unwrap_or(1);
                let cpus_to_use = num_cpus.min(self.config.workers);

                for cpu in 0..cpus_to_use {
                    let reader = PerfCollector::new(
                        perf_path.clone(),
                        cpu,
                        self.config.perf_buffer_pages,
                        Arc::clone(&self.stats),
                    )?;

                    let tx = event_tx.clone();
                    let shutdown = Arc::clone(&self.shutdown);
                    let filter = self.config.filter.clone();

                    handles.push(tokio::spawn(async move {
                        reader.run(tx, shutdown, filter).await
                    }));
                }

                info!(
                    path = %perf_path.display(),
                    cpus = cpus_to_use,
                    "Perf event readers started"
                );
            } else {
                warn!(path = %perf_path.display(), "Perf event path not found");
            }
        }

        if handles.is_empty() {
            warn!("No event sources configured, collector will idle");
        }

        *self.state.write() = CollectorState::Running;
        info!(readers = handles.len(), "Collector running");

        // Wait for all readers to complete
        for handle in handles {
            match handle.await {
                Ok(Ok(())) => {}
                Ok(Err(e)) => {
                    error!(error = %e, "Reader error");
                }
                Err(e) => {
                    error!(error = %e, "Reader task panicked");
                }
            }
        }

        *self.state.write() = CollectorState::Stopped;
        info!(
            events_read = self.stats.events_read.load(Ordering::Relaxed),
            events_dropped = self.stats.events_dropped.load(Ordering::Relaxed),
            "Collector stopped"
        );

        Ok(())
    }
}

/// Builder for creating collectors with custom configuration
pub struct CollectorBuilder {
    config: CollectorConfig,
}

impl CollectorBuilder {
    pub fn new() -> Self {
        Self {
            config: CollectorConfig::default(),
        }
    }

    pub fn ringbuf_path(mut self, path: impl Into<PathBuf>) -> Self {
        self.config.ringbuf_path = Some(path.into());
        self
    }

    pub fn perf_path(mut self, path: impl Into<PathBuf>) -> Self {
        self.config.perf_path = Some(path.into());
        self
    }

    pub fn ringbuf_size(mut self, size: usize) -> Self {
        self.config.ringbuf_size = size;
        self
    }

    pub fn perf_buffer_pages(mut self, pages: usize) -> Self {
        self.config.perf_buffer_pages = pages;
        self
    }

    pub fn workers(mut self, workers: usize) -> Self {
        self.config.workers = workers;
        self
    }

    pub fn event_queue_size(mut self, size: usize) -> Self {
        self.config.event_queue_size = size;
        self
    }

    pub fn filter(mut self, filter: EventFilter) -> Self {
        self.config.filter = Some(filter);
        self.config.filtering_enabled = true;
        self
    }

    pub fn build(self) -> Result<Collector> {
        Collector::new(self.config)
    }
}

impl Default for CollectorBuilder {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_collector_config_default() {
        let config = CollectorConfig::default();
        assert!(config.ringbuf_path.is_some());
        assert!(config.perf_path.is_some());
        assert_eq!(config.workers, 4);
    }

    #[test]
    fn test_collector_stats() {
        let stats = CollectorStats::new();
        stats.record_read(10, 1000, "ringbuf");
        assert_eq!(stats.events_read.load(Ordering::Relaxed), 10);
        assert_eq!(stats.bytes_processed.load(Ordering::Relaxed), 1000);
        assert_eq!(stats.ringbuf_reads.load(Ordering::Relaxed), 1);
    }

    #[test]
    fn test_collector_builder() {
        let collector = CollectorBuilder::new()
            .workers(8)
            .event_queue_size(1000)
            .build();

        assert!(collector.is_ok());
    }

    #[test]
    fn test_collector_state() {
        assert_eq!(CollectorState::Idle, CollectorState::Idle);
        assert_ne!(CollectorState::Running, CollectorState::Stopped);
    }
}
