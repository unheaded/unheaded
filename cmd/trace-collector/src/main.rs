//! THE WHISPERING VOID - Ultra-lean eBPF event collector
//!
//! This is the heart of the trace collection infrastructure for the Unheaded Kingdom.
//! It reads events from eBPF ring buffers and perf event arrays with zero-copy semantics,
//! parses kernel events, and publishes them to the Busboy message bus.
//!
//! Design principles:
//! - Sub-microsecond latency where possible
//! - Lock-free data structures
//! - Zero-copy event reading
//! - Backpressure-aware publishing
//! - Observable by default
//!
//! Architecture:
//! ```text
//!  ┌─────────────────────────────────────────────────────────────────┐
//!  │                    THE WHISPERING VOID                          │
//!  │                                                                 │
//!  │  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐        │
//!  │  │ Ring Buffer │    │ Perf Events │    │  BPF Maps   │        │
//!  │  │   Reader    │    │   Reader    │    │   Reader    │        │
//!  │  └──────┬──────┘    └──────┬──────┘    └──────┬──────┘        │
//!  │         │                  │                  │                │
//!  │         └────────────┬─────┴──────────────────┘                │
//!  │                      │                                         │
//!  │               ┌──────▼──────┐                                  │
//!  │               │   Event     │                                  │
//!  │               │   Channel   │                                  │
//!  │               │ (lock-free) │                                  │
//!  │               └──────┬──────┘                                  │
//!  │                      │                                         │
//!  │               ┌──────▼──────┐                                  │
//!  │               │   Busboy    │                                  │
//!  │               │  Publisher  │─────────────► Busboy Message Bus │
//!  │               │  (batched)  │                                  │
//!  │               └─────────────┘                                  │
//!  │                                                                 │
//!  │  ┌─────────────┐                                               │
//!  │  │  Metrics    │─────────────► Prometheus (:9090/metrics)      │
//!  │  │  Server     │                                               │
//!  │  └─────────────┘                                               │
//!  └─────────────────────────────────────────────────────────────────┘
//! ```

use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use clap::{Parser, Subcommand, ValueEnum};
use tokio::signal;
use tokio::sync::broadcast;
use tracing::{debug, error, info, warn, Level};
use tracing_subscriber::{fmt, prelude::*, EnvFilter};

mod bpf;
mod collector;
mod config;
mod correlation;
mod events;
mod metrics;
mod proto;
mod publisher;
mod websocket;

use config::Config;
use metrics::MetricsServer;

// ============================================================================
// CLI DEFINITIONS
// ============================================================================

/// THE WHISPERING VOID - eBPF event collector for the Unheaded Kingdom
///
/// Reads kernel events from eBPF programs and publishes them to Busboy.
/// Part of the Unheaded infrastructure for end-to-end observability.
#[derive(Parser, Debug)]
#[command(name = "trace-collector")]
#[command(author = "Unheaded Kingdom Infrastructure Team")]
#[command(version = env!("CARGO_PKG_VERSION"))]
#[command(about = "THE WHISPERING VOID - Ultra-lean eBPF event collector", long_about = None)]
#[command(propagate_version = true)]
struct Cli {
    /// Configuration file path (TOML format)
    #[arg(short, long, env = "TRACE_COLLECTOR_CONFIG")]
    config: Option<PathBuf>,

    /// Log level (trace, debug, info, warn, error)
    #[arg(short, long, default_value = "info", env = "TRACE_COLLECTOR_LOG_LEVEL")]
    log_level: String,

    /// Enable JSON structured logging (for production)
    #[arg(long, env = "TRACE_COLLECTOR_JSON_LOG")]
    json_log: bool,

    /// Metrics bind address (Prometheus endpoint)
    #[arg(long, default_value = "0.0.0.0:9090", env = "TRACE_COLLECTOR_METRICS_ADDR")]
    metrics_addr: String,

    /// Busboy gRPC endpoint for event publishing
    #[arg(long, default_value = "http://localhost:50051", env = "BUSBOY_ENDPOINT")]
    busboy_endpoint: String,

    /// Output format for CLI commands
    #[arg(long, default_value = "text", env = "TRACE_COLLECTOR_OUTPUT_FORMAT")]
    output_format: OutputFormat,

    #[command(subcommand)]
    command: Option<Commands>,
}

/// Output format for CLI commands
#[derive(Debug, Clone, Copy, ValueEnum)]
enum OutputFormat {
    /// Plain text output
    Text,
    /// JSON output
    Json,
    /// YAML output
    Yaml,
}

#[derive(Subcommand, Debug)]
enum Commands {
    /// Run the collector daemon (default)
    Run {
        /// BPF program path (if loading custom programs)
        #[arg(long)]
        bpf_prog: Option<PathBuf>,

        /// Ring buffer map path (pinned in bpffs)
        #[arg(long, default_value = "/sys/fs/bpf/unheaded/trace_events")]
        ringbuf_path: PathBuf,

        /// Perf event array path (pinned in bpffs)
        #[arg(long, default_value = "/sys/fs/bpf/unheaded/perf_events")]
        perf_path: PathBuf,

        /// Number of worker threads for event processing
        #[arg(long, default_value = "4")]
        workers: usize,

        /// Batch size for event publishing (higher = more throughput, more latency)
        #[arg(long, default_value = "1000")]
        batch_size: usize,

        /// Batch timeout in milliseconds (flush even if batch not full)
        #[arg(long, default_value = "10")]
        batch_timeout_ms: u64,

        /// Enable dry-run mode (don't publish, just log)
        #[arg(long)]
        dry_run: bool,

        /// Filter events by type (comma-separated: packet,syscall,latency)
        #[arg(long)]
        event_filter: Option<String>,
    },

    /// Validate configuration file
    Validate {
        /// Show effective configuration after merging defaults
        #[arg(long)]
        show_effective: bool,
    },

    /// Show version and build information
    Version,

    /// Check BPF map status
    Status {
        /// BPF filesystem path
        #[arg(long, default_value = "/sys/fs/bpf/unheaded")]
        bpffs_path: PathBuf,
    },

    /// Dump events to stdout (debugging tool)
    Dump {
        /// Ring buffer map path
        #[arg(long, default_value = "/sys/fs/bpf/unheaded/trace_events")]
        ringbuf_path: PathBuf,

        /// Maximum events to dump (0 = unlimited)
        #[arg(long, default_value = "100")]
        max_events: usize,

        /// Output raw bytes in hex
        #[arg(long)]
        raw: bool,
    },

    /// Generate sample configuration file
    GenerateConfig {
        /// Output path (- for stdout)
        #[arg(default_value = "-")]
        output: String,
    },
}

// ============================================================================
// GLOBAL STATE
// ============================================================================

/// Global shutdown flag for coordinated shutdown
static SHUTDOWN: AtomicBool = AtomicBool::new(false);

/// Global startup timestamp (for uptime calculation)
static STARTUP_TIME: AtomicU64 = AtomicU64::new(0);

// ============================================================================
// MAIN ENTRY POINT
// ============================================================================

#[tokio::main]
async fn main() -> Result<()> {
    let cli = Cli::parse();

    // Record startup time
    STARTUP_TIME.store(
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs(),
        Ordering::SeqCst,
    );

    // Initialize logging
    init_logging(&cli.log_level, cli.json_log)?;

    info!(
        version = env!("CARGO_PKG_VERSION"),
        pid = std::process::id(),
        "THE WHISPERING VOID awakens"
    );

    // Load configuration
    let config = if let Some(config_path) = &cli.config {
        info!(path = %config_path.display(), "Loading configuration from file");
        Config::from_file(config_path).context("Failed to load configuration")?
    } else {
        debug!("Using default configuration");
        Config::default()
    };

    // Execute command
    let result = match cli.command {
        Some(Commands::Run {
            bpf_prog,
            ringbuf_path,
            perf_path,
            workers,
            batch_size,
            batch_timeout_ms,
            dry_run,
            event_filter,
        }) => {
            run_daemon(RunConfig {
                config,
                busboy_endpoint: cli.busboy_endpoint,
                metrics_addr: cli.metrics_addr,
                bpf_prog,
                ringbuf_path,
                perf_path,
                workers,
                batch_size,
                batch_timeout_ms,
                dry_run,
                event_filter,
            })
            .await
        }
        Some(Commands::Validate { show_effective }) => {
            validate_config(&config, show_effective, cli.output_format)
        }
        Some(Commands::Version) => {
            print_version(cli.output_format);
            Ok(())
        }
        Some(Commands::Status { bpffs_path }) => {
            check_status(&bpffs_path, cli.output_format).await
        }
        Some(Commands::Dump {
            ringbuf_path,
            max_events,
            raw,
        }) => dump_events(&ringbuf_path, max_events, raw).await,
        Some(Commands::GenerateConfig { output }) => generate_config(&output),
        None => {
            // Default: run daemon with defaults
            run_daemon(RunConfig {
                config,
                busboy_endpoint: cli.busboy_endpoint,
                metrics_addr: cli.metrics_addr,
                bpf_prog: None,
                ringbuf_path: PathBuf::from("/sys/fs/bpf/unheaded/trace_events"),
                perf_path: PathBuf::from("/sys/fs/bpf/unheaded/perf_events"),
                workers: 4,
                batch_size: 1000,
                batch_timeout_ms: 10,
                dry_run: false,
                event_filter: None,
            })
            .await
        }
    };

    match &result {
        Ok(()) => info!("THE WHISPERING VOID sleeps"),
        Err(e) => error!(error = %e, "THE WHISPERING VOID encountered an error"),
    }

    result
}

// ============================================================================
// LOGGING INITIALIZATION
// ============================================================================

fn init_logging(level: &str, json: bool) -> Result<()> {
    let level = match level.to_lowercase().as_str() {
        "trace" => Level::TRACE,
        "debug" => Level::DEBUG,
        "info" => Level::INFO,
        "warn" => Level::WARN,
        "error" => Level::ERROR,
        _ => Level::INFO,
    };

    let filter = EnvFilter::from_default_env()
        .add_directive(level.into())
        // Reduce noise from dependencies
        .add_directive("hyper=warn".parse().unwrap())
        .add_directive("tokio=warn".parse().unwrap())
        .add_directive("tonic=info".parse().unwrap());

    if json {
        tracing_subscriber::registry()
            .with(filter)
            .with(
                fmt::layer()
                    .json()
                    .with_current_span(true)
                    .with_span_list(true)
                    .with_thread_ids(true)
                    .with_target(true),
            )
            .init();
    } else {
        tracing_subscriber::registry()
            .with(filter)
            .with(
                fmt::layer()
                    .with_target(true)
                    .with_thread_ids(false)
                    .with_file(false)
                    .with_line_number(false),
            )
            .init();
    }

    Ok(())
}

// ============================================================================
// DAEMON RUNNER
// ============================================================================

/// Configuration for the daemon runner
struct RunConfig {
    config: Config,
    busboy_endpoint: String,
    metrics_addr: String,
    bpf_prog: Option<PathBuf>,
    ringbuf_path: PathBuf,
    perf_path: PathBuf,
    workers: usize,
    batch_size: usize,
    batch_timeout_ms: u64,
    dry_run: bool,
    event_filter: Option<String>,
}

async fn run_daemon(run_config: RunConfig) -> Result<()> {
    info!(
        workers = run_config.workers,
        batch_size = run_config.batch_size,
        batch_timeout_ms = run_config.batch_timeout_ms,
        dry_run = run_config.dry_run,
        "Starting trace collector daemon"
    );

    // Create shutdown broadcast channel
    let (shutdown_tx, _) = broadcast::channel::<()>(1);
    let shutdown_flag = Arc::new(AtomicBool::new(false));

    // =========================================================================
    // Initialize Correlation Engine and Trace Store
    // =========================================================================

    use crate::correlation::{CorrelationConfig, CorrelationEngine, TraceStore, TraceStoreConfig};
    use crate::websocket::{WebSocketConfig, WebSocketServer};

    // Create trace store
    let trace_store = Arc::new(
        TraceStore::new(TraceStoreConfig::default())
            .context("Failed to create trace store")?
    );
    info!("Trace store initialized");

    // Create correlation engine
    let correlation_engine = Arc::new(CorrelationEngine::new(CorrelationConfig::default()));
    info!("Correlation engine initialized");

    // Subscribe to completed traces and store them
    let mut completed_rx = correlation_engine.subscribe_completed();
    let store_for_completed = Arc::clone(&trace_store);
    let mut completed_shutdown_rx = shutdown_tx.subscribe();

    let completed_handle = tokio::spawn(async move {
        loop {
            tokio::select! {
                result = completed_rx.recv() => {
                    match result {
                        Ok(summary) => {
                            store_for_completed.store_summary(summary);
                        }
                        Err(broadcast::error::RecvError::Closed) => break,
                        Err(broadcast::error::RecvError::Lagged(n)) => {
                            warn!(missed = n, "Completed trace receiver lagged");
                        }
                    }
                }
                _ = completed_shutdown_rx.recv() => break,
            }
        }
    });

    // Start correlation engine cleanup task
    let engine_for_cleanup = Arc::clone(&correlation_engine);
    let cleanup_shutdown_rx = shutdown_tx.subscribe();
    let cleanup_handle = tokio::spawn(async move {
        engine_for_cleanup.run_cleanup(cleanup_shutdown_rx).await;
    });

    // =========================================================================
    // Initialize WebSocket Server
    // =========================================================================

    let ws_config = WebSocketConfig {
        bind_addr: "0.0.0.0:9091".to_string(),
        ..Default::default()
    };

    let ws_server = Arc::new(
        WebSocketServer::new(ws_config)
            .with_trace_store(Arc::clone(&trace_store))
            .with_correlation_engine(Arc::clone(&correlation_engine))
    );

    // Forward completed traces to WebSocket clients
    let mut ws_completed_rx = correlation_engine.subscribe_completed();
    let ws_for_broadcast = Arc::clone(&ws_server);
    let mut ws_shutdown_rx = shutdown_tx.subscribe();

    let ws_broadcast_handle = tokio::spawn(async move {
        loop {
            tokio::select! {
                result = ws_completed_rx.recv() => {
                    match result {
                        Ok(summary) => {
                            ws_for_broadcast.broadcast_summary(&summary);
                        }
                        Err(broadcast::error::RecvError::Closed) => break,
                        Err(_) => continue,
                    }
                }
                _ = ws_shutdown_rx.recv() => break,
            }
        }
    });

    // Start WebSocket server
    let ws_server_clone = Arc::clone(&ws_server);
    let ws_handle = tokio::spawn(async move {
        if let Err(e) = ws_server_clone.run().await {
            error!(error = %e, "WebSocket server error");
        }
    });
    info!(addr = "0.0.0.0:9091", "WebSocket server started");

    // =========================================================================
    // Start Metrics Server
    // =========================================================================

    let metrics_server = MetricsServer::new(&run_config.metrics_addr)?;
    let metrics_handle = {
        let mut shutdown_rx = shutdown_tx.subscribe();
        tokio::spawn(async move {
            tokio::select! {
                result = metrics_server.run() => {
                    if let Err(e) = result {
                        error!(error = %e, "Metrics server error");
                    }
                }
                _ = shutdown_rx.recv() => {
                    info!("Metrics server shutting down");
                }
            }
        })
    };
    info!(addr = run_config.metrics_addr, "Metrics server started");

    // =========================================================================
    // Create Publisher
    // =========================================================================

    let publisher = if run_config.dry_run {
        info!("Dry-run mode: events will be logged but not published");
        None
    } else {
        Some(
            publisher::BusboyPublisher::new(
                &run_config.busboy_endpoint,
                run_config.batch_size,
                Duration::from_millis(run_config.batch_timeout_ms),
            )
            .await
            .context("Failed to create Busboy publisher")?,
        )
    };

    let publisher = publisher.map(Arc::new);

    // Create event channel (lock-free MPMC bounded queue)
    let (event_tx, event_rx) = crossbeam::channel::bounded(run_config.config.event_queue_size);

    // =========================================================================
    // Start Event Processing Pipeline with Correlation
    // =========================================================================

    // Create a channel for correlated events
    let (correlated_tx, correlated_rx) = crossbeam::channel::bounded(run_config.config.event_queue_size);

    // Correlation processor: reads raw events, correlates them, forwards to publisher
    let correlation_engine_for_proc = Arc::clone(&correlation_engine);
    let event_rx_for_correlation = event_rx.clone();
    let shutdown_flag_for_correlation = Arc::clone(&shutdown_flag);
    let mut correlation_shutdown_rx = shutdown_tx.subscribe();

    let correlation_handle = tokio::spawn(async move {
        let mut processed = 0u64;

        loop {
            tokio::select! {
                _ = correlation_shutdown_rx.recv() => break,
                _ = tokio::time::sleep(Duration::from_micros(100)) => {
                    // Process events in batches for efficiency
                    for _ in 0..100 {
                        match event_rx_for_correlation.try_recv() {
                            Ok(event) => {
                                // Correlate the event
                                let _trace_id = correlation_engine_for_proc.process_event(&event);

                                // Forward to publisher
                                if correlated_tx.try_send(event).is_err() {
                                    // Queue full, drop oldest
                                }

                                processed += 1;
                            }
                            Err(crossbeam::channel::TryRecvError::Empty) => break,
                            Err(crossbeam::channel::TryRecvError::Disconnected) => return,
                        }
                    }

                    if shutdown_flag_for_correlation.load(Ordering::Relaxed) {
                        break;
                    }
                }
            }
        }

        info!(events = processed, "Correlation processor finished");
    });

    // Start publisher task
    let publisher_handle = if let Some(ref publisher) = publisher {
        let publisher = Arc::clone(publisher);
        let shutdown_flag = Arc::clone(&shutdown_flag);
        let mut shutdown_rx = shutdown_tx.subscribe();

        Some(tokio::spawn(async move {
            tokio::select! {
                result = publisher.run(correlated_rx, shutdown_flag) => {
                    if let Err(e) = result {
                        error!(error = %e, "Publisher error");
                    }
                }
                _ = shutdown_rx.recv() => {
                    info!("Publisher shutting down");
                }
            }
        }))
    } else {
        // Dry-run: just drain and log events
        let shutdown_flag = Arc::clone(&shutdown_flag);
        let mut shutdown_rx = shutdown_tx.subscribe();

        Some(tokio::spawn(async move {
            let mut count = 0u64;
            let start = Instant::now();

            loop {
                tokio::select! {
                    _ = shutdown_rx.recv() => break,
                    _ = tokio::time::sleep(Duration::from_millis(1)) => {
                        while let Ok(event) = correlated_rx.try_recv() {
                            count += 1;
                            if count % 1000 == 0 {
                                let rate = count as f64 / start.elapsed().as_secs_f64();
                                debug!(
                                    count = count,
                                    rate = rate,
                                    event_type = ?event.event_type,
                                    "Dry-run: received event"
                                );
                            }
                        }
                        if shutdown_flag.load(Ordering::Relaxed) {
                            break;
                        }
                    }
                }
            }

            info!(total_events = count, "Dry-run: finished");
        }))
    };

    // =========================================================================
    // Start Multi-Source eBPF Readers
    // =========================================================================

    let mut reader_handles = Vec::new();

    // Try to use multi-source reader first (reads from all eBPF programs)
    let multi_source_config = bpf::MultiSourceConfig::all_sources();
    let multi_source_reader = bpf::MultiSourceReader::new(multi_source_config);

    // Check available sources
    let available_sources = multi_source_reader.check_sources();
    let has_multi_source = available_sources.iter().any(|(_, available)| *available);

    if has_multi_source {
        info!("Starting multi-source eBPF reader");
        for (source, available) in &available_sources {
            if *available {
                info!(source = source.name(), "eBPF source available");
            } else {
                debug!(source = source.name(), "eBPF source not available");
            }
        }

        let event_tx_clone = event_tx.clone();
        let shutdown_flag_clone = Arc::clone(&shutdown_flag);
        let mut shutdown_rx = shutdown_tx.subscribe();

        let handle = tokio::spawn(async move {
            tokio::select! {
                result = multi_source_reader.run(event_tx_clone) => {
                    if let Err(e) = result {
                        error!(error = %e, "Multi-source reader error");
                    }
                }
                _ = shutdown_rx.recv() => {
                    multi_source_reader.shutdown();
                    info!("Multi-source reader shutting down");
                }
            }
        });
        reader_handles.push(handle);
    } else {
        // Fall back to individual readers
        info!("No multi-source eBPF programs found, trying individual readers");

        // Ring buffer reader
        if run_config.ringbuf_path.exists() {
            info!(path = %run_config.ringbuf_path.display(), "Starting ring buffer reader");

            let ringbuf_reader =
                bpf::RingBufReader::new(&run_config.ringbuf_path, run_config.config.ringbuf_size)?;
            let event_tx = event_tx.clone();
            let shutdown_flag = Arc::clone(&shutdown_flag);
            let mut shutdown_rx = shutdown_tx.subscribe();

            let handle = tokio::spawn(async move {
                tokio::select! {
                    result = ringbuf_reader.run(event_tx, shutdown_flag) => {
                        if let Err(e) = result {
                            error!(error = %e, "Ring buffer reader error");
                        }
                    }
                    _ = shutdown_rx.recv() => {
                        info!("Ring buffer reader shutting down");
                    }
                }
            });
            reader_handles.push(handle);
        } else {
            warn!(
                path = %run_config.ringbuf_path.display(),
                "Ring buffer path not found, skipping. Make sure eBPF programs are loaded."
            );
        }

        // Perf event reader (one per CPU for optimal performance)
        if run_config.perf_path.exists() {
            let num_cpus = num_cpus();
            let cpus_to_use = num_cpus.min(run_config.workers);

            info!(
                path = %run_config.perf_path.display(),
                num_cpus = num_cpus,
                workers = cpus_to_use,
                "Starting perf event readers"
            );

            for cpu in 0..cpus_to_use {
                let perf_reader = bpf::PerfEventReader::new(
                    &run_config.perf_path,
                    cpu,
                    run_config.config.perf_buffer_pages,
                )?;
                let event_tx = event_tx.clone();
                let shutdown_flag = Arc::clone(&shutdown_flag);
                let mut shutdown_rx = shutdown_tx.subscribe();

                let handle = tokio::spawn(async move {
                    tokio::select! {
                        result = perf_reader.run(event_tx, shutdown_flag) => {
                            if let Err(e) = result {
                                error!(cpu = cpu, error = %e, "Perf event reader error");
                            }
                        }
                        _ = shutdown_rx.recv() => {
                            info!(cpu = cpu, "Perf event reader shutting down");
                        }
                    }
                });
                reader_handles.push(handle);
            }
        } else {
            warn!(
                path = %run_config.perf_path.display(),
                "Perf event path not found, skipping. Make sure eBPF programs are loaded."
            );
        }
    }

    // Drop the sender so publisher knows when all readers are done
    drop(event_tx);

    // Print startup banner
    info!(
        readers = reader_handles.len(),
        active_traces = correlation_engine.active_trace_count(),
        "Collector running. Press Ctrl+C to stop."
    );
    info!("  - Metrics: http://{}/metrics", run_config.metrics_addr);
    info!("  - WebSocket: ws://0.0.0.0:9091");
    info!("  - Busboy: {}", run_config.busboy_endpoint);

    // Wait for shutdown signal
    tokio::select! {
        _ = signal::ctrl_c() => {
            info!("Received SIGINT, initiating graceful shutdown");
        }
        _ = async {
            let mut sigterm = signal::unix::signal(signal::unix::SignalKind::terminate())
                .expect("Failed to install SIGTERM handler");
            sigterm.recv().await
        } => {
            info!("Received SIGTERM, initiating graceful shutdown");
        }
    }

    // Signal shutdown to all tasks
    SHUTDOWN.store(true, Ordering::SeqCst);
    shutdown_flag.store(true, Ordering::SeqCst);
    ws_server.shutdown();
    let _ = shutdown_tx.send(());

    // Wait for tasks to complete with timeout
    let shutdown_timeout = Duration::from_secs(5);
    info!(
        timeout_secs = shutdown_timeout.as_secs(),
        "Waiting for tasks to complete"
    );

    let shutdown_start = Instant::now();

    tokio::select! {
        _ = async {
            // Wait for all reader handles
            for handle in reader_handles {
                let _ = handle.await;
            }
            // Wait for correlation
            let _ = correlation_handle.await;
            // Wait for publisher
            if let Some(handle) = publisher_handle {
                let _ = handle.await;
            }
            // Wait for WebSocket components
            let _ = ws_handle.await;
            let _ = ws_broadcast_handle.await;
            // Wait for cleanup and completed handlers
            let _ = cleanup_handle.await;
            let _ = completed_handle.await;
            // Wait for metrics server
            let _ = metrics_handle.await;
        } => {
            info!(
                elapsed_ms = shutdown_start.elapsed().as_millis(),
                traces_processed = correlation_engine.stats().events_processed.load(Ordering::Relaxed),
                traces_created = correlation_engine.stats().traces_created.load(Ordering::Relaxed),
                "Graceful shutdown complete"
            );
        }
        _ = tokio::time::sleep(shutdown_timeout) => {
            warn!(
                elapsed_ms = shutdown_start.elapsed().as_millis(),
                "Shutdown timeout exceeded, forcing exit"
            );
        }
    }

    // Flush any remaining traces to disk
    if let Err(e) = trace_store.flush() {
        warn!(error = %e, "Failed to flush trace store");
    }

    Ok(())
}

// ============================================================================
// UTILITY COMMANDS
// ============================================================================

fn validate_config(config: &Config, show_effective: bool, format: OutputFormat) -> Result<()> {
    info!("Configuration is valid");

    if show_effective {
        match format {
            OutputFormat::Json => {
                println!("{}", serde_json::to_string_pretty(config)?);
            }
            OutputFormat::Yaml => {
                println!("{}", serde_json::to_string_pretty(config)?); // Use JSON for now
            }
            OutputFormat::Text => {
                println!("Effective Configuration:");
                println!("  event_queue_size: {}", config.event_queue_size);
                println!("  ringbuf_size: {} bytes", config.ringbuf_size);
                println!("  perf_buffer_pages: {}", config.perf_buffer_pages);
                println!("  publisher.batch_size: {}", config.publisher.batch_size);
                println!(
                    "  publisher.batch_timeout_ms: {}",
                    config.publisher.batch_timeout_ms
                );
                println!("  publisher.compression: {}", config.publisher.compression);
                println!("  filters.packets: {}", config.filters.packets);
                println!("  filters.syscalls: {}", config.filters.syscalls);
                println!("  filters.latency: {}", config.filters.latency);
            }
        }
    }

    Ok(())
}

fn print_version(format: OutputFormat) {
    let version_info = VersionInfo {
        name: "trace-collector".to_string(),
        version: env!("CARGO_PKG_VERSION").to_string(),
        description: "THE WHISPERING VOID - Ultra-lean eBPF event collector".to_string(),
        project: "Unheaded Kingdom Infrastructure".to_string(),
        rust_version: option_env!("RUSTC_VERSION").unwrap_or("unknown").to_string(),
        target: option_env!("TARGET").unwrap_or(std::env::consts::ARCH).to_string(),
    };

    match format {
        OutputFormat::Json => {
            println!("{}", serde_json::to_string_pretty(&version_info).unwrap());
        }
        OutputFormat::Yaml => {
            println!("{}", serde_json::to_string_pretty(&version_info).unwrap());
        }
        OutputFormat::Text => {
            println!("{} v{}", version_info.name, version_info.version);
            println!("{}", version_info.description);
            println!("Part of {}", version_info.project);
            println!();
            println!("Target: {}", version_info.target);
        }
    }
}

#[derive(serde::Serialize)]
struct VersionInfo {
    name: String,
    version: String,
    description: String,
    project: String,
    rust_version: String,
    target: String,
}

async fn check_status(bpffs_path: &PathBuf, format: OutputFormat) -> Result<()> {
    use std::fs;

    let mut status = BpfStatus {
        bpffs_mounted: false,
        maps: Vec::new(),
        programs: Vec::new(),
    };

    // Check if bpffs is mounted
    if bpffs_path.exists() {
        status.bpffs_mounted = true;

        // List maps and programs
        if let Ok(entries) = fs::read_dir(bpffs_path) {
            for entry in entries.flatten() {
                let path = entry.path();
                let name = entry.file_name().to_string_lossy().to_string();

                if path.is_file() {
                    // Try to determine if it's a map or program
                    // In practice, you'd use bpf syscalls to get this info
                    status.maps.push(BpfMapStatus {
                        name: name.clone(),
                        path: path.display().to_string(),
                        accessible: true,
                    });
                } else if path.is_dir() {
                    // Recurse into subdirectories
                    if let Ok(sub_entries) = fs::read_dir(&path) {
                        for sub_entry in sub_entries.flatten() {
                            let sub_name = format!(
                                "{}/{}",
                                name,
                                sub_entry.file_name().to_string_lossy()
                            );
                            status.maps.push(BpfMapStatus {
                                name: sub_name,
                                path: sub_entry.path().display().to_string(),
                                accessible: sub_entry.path().exists(),
                            });
                        }
                    }
                }
            }
        }
    }

    match format {
        OutputFormat::Json => {
            println!("{}", serde_json::to_string_pretty(&status)?);
        }
        OutputFormat::Yaml => {
            println!("{}", serde_json::to_string_pretty(&status)?);
        }
        OutputFormat::Text => {
            println!("BPF Status:");
            println!(
                "  BPFFS Mounted: {}",
                if status.bpffs_mounted { "yes" } else { "no" }
            );
            println!("  Path: {}", bpffs_path.display());
            println!();
            if status.maps.is_empty() {
                println!("  No BPF objects found.");
                println!("  Hint: Load eBPF programs first with the eBPF loader.");
            } else {
                println!("  BPF Objects:");
                for map in &status.maps {
                    let status_icon = if map.accessible { "[OK]" } else { "[ERR]" };
                    println!("    {} {}", status_icon, map.name);
                }
            }
        }
    }

    Ok(())
}

#[derive(serde::Serialize)]
struct BpfStatus {
    bpffs_mounted: bool,
    maps: Vec<BpfMapStatus>,
    programs: Vec<BpfProgramStatus>,
}

#[derive(serde::Serialize)]
struct BpfMapStatus {
    name: String,
    path: String,
    accessible: bool,
}

#[derive(serde::Serialize)]
struct BpfProgramStatus {
    name: String,
    prog_type: String,
    attached: bool,
}

async fn dump_events(ringbuf_path: &PathBuf, max_events: usize, raw: bool) -> Result<()> {
    use crate::events::Event;

    if !ringbuf_path.exists() {
        anyhow::bail!(
            "Ring buffer path does not exist: {}",
            ringbuf_path.display()
        );
    }

    info!(
        path = %ringbuf_path.display(),
        max_events = max_events,
        "Dumping events from ring buffer"
    );

    // Create a channel to receive events
    let (tx, rx) = crossbeam::channel::bounded(1000);
    let shutdown = Arc::new(AtomicBool::new(false));

    // Create reader
    let reader = bpf::RingBufReader::new(ringbuf_path, 16 * 1024 * 1024)?;

    let shutdown_clone = Arc::clone(&shutdown);
    let reader_handle = tokio::spawn(async move {
        let _ = reader.run(tx, shutdown_clone).await;
    });

    let mut count = 0;
    let timeout = Duration::from_secs(5);
    let start = Instant::now();

    println!("Waiting for events (timeout: {}s)...\n", timeout.as_secs());

    loop {
        match rx.recv_timeout(Duration::from_millis(100)) {
            Ok(event) => {
                count += 1;

                if raw {
                    if let Some(ref bytes) = event.raw {
                        println!("Event #{} (raw {} bytes):", count, bytes.len());
                        print_hex(bytes);
                    }
                } else {
                    println!("Event #{}:", count);
                    println!("  Type: {:?}", event.event_type);
                    println!("  Timestamp: {} ns", event.timestamp_ns);
                    println!("  CPU: {}", event.cpu);
                    println!("  PID: {}", event.pid);
                    println!("  TID: {}", event.tid);
                    println!("  Comm: {}", event.comm);
                    println!("  Data: {:?}", event.data);
                }
                println!();

                if max_events > 0 && count >= max_events {
                    break;
                }
            }
            Err(crossbeam::channel::RecvTimeoutError::Timeout) => {
                if start.elapsed() > timeout {
                    println!("Timeout reached.");
                    break;
                }
            }
            Err(crossbeam::channel::RecvTimeoutError::Disconnected) => {
                break;
            }
        }
    }

    shutdown.store(true, Ordering::SeqCst);
    let _ = reader_handle.await;

    println!("Total events dumped: {}", count);

    Ok(())
}

fn print_hex(data: &[u8]) {
    for (i, chunk) in data.chunks(16).enumerate() {
        print!("  {:08x}  ", i * 16);

        // Hex bytes
        for (j, byte) in chunk.iter().enumerate() {
            print!("{:02x} ", byte);
            if j == 7 {
                print!(" ");
            }
        }

        // Padding if needed
        for _ in chunk.len()..16 {
            print!("   ");
        }
        if chunk.len() <= 8 {
            print!(" ");
        }

        // ASCII
        print!(" |");
        for byte in chunk {
            if *byte >= 0x20 && *byte < 0x7f {
                print!("{}", *byte as char);
            } else {
                print!(".");
            }
        }
        println!("|");
    }
}

fn generate_config(output: &str) -> Result<()> {
    let sample_config = r#"# THE WHISPERING VOID - Trace Collector Configuration
# =====================================================
# Configuration file for the eBPF event collector.
# All values shown are defaults.

# Event queue size (bounded, for backpressure)
event_queue_size = 65536

# Ring buffer size in bytes (must be power of 2)
ringbuf_size = 16777216  # 16 MB

# Perf buffer pages per CPU
perf_buffer_pages = 64  # 256 KB per CPU

[publisher]
# Maximum batch size before flushing
batch_size = 1000

# Maximum time (ms) to wait before flushing a partial batch
batch_timeout_ms = 10

# Maximum retry attempts for failed publishes
max_retries = 3

# Base retry backoff in milliseconds
retry_backoff_ms = 100

# Enable gzip compression for batches
compression = true

# Connection timeout in milliseconds
connect_timeout_ms = 5000

[filters]
# Enable packet event collection
packets = true

# Enable syscall event collection
syscalls = true

# Enable latency probe collection
latency = true

# Filter by process name (regex, optional)
# process_filter = "nginx|envoy"

# Filter by cgroup path (regex, optional)
# cgroup_filter = "/system.slice/docker"

# Minimum latency threshold in nanoseconds (0 = all)
min_latency_ns = 0

[metrics]
# Enable Prometheus metrics
enabled = true

# Metrics endpoint path
path = "/metrics"

# Include detailed histograms (more memory)
histograms = false
"#;

    if output == "-" {
        println!("{}", sample_config);
    } else {
        std::fs::write(output, sample_config)?;
        info!(path = output, "Configuration file generated");
    }

    Ok(())
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

/// Get the number of available CPUs
fn num_cpus() -> usize {
    std::thread::available_parallelism()
        .map(|p| p.get())
        .unwrap_or(1)
}

/// Get uptime in seconds
#[allow(dead_code)]
fn uptime_secs() -> u64 {
    let startup = STARTUP_TIME.load(Ordering::SeqCst);
    if startup == 0 {
        return 0;
    }

    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs();

    now.saturating_sub(startup)
}

// ============================================================================
// TESTS
// ============================================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_num_cpus() {
        let cpus = num_cpus();
        assert!(cpus >= 1);
    }

    #[test]
    fn test_cli_parsing() {
        let cli = Cli::try_parse_from(["trace-collector", "--log-level", "debug"]).unwrap();
        assert_eq!(cli.log_level, "debug");
    }

    #[test]
    fn test_cli_run_command() {
        let cli = Cli::try_parse_from([
            "trace-collector",
            "run",
            "--workers",
            "8",
            "--batch-size",
            "500",
        ])
        .unwrap();

        if let Some(Commands::Run { workers, batch_size, .. }) = cli.command {
            assert_eq!(workers, 8);
            assert_eq!(batch_size, 500);
        } else {
            panic!("Expected Run command");
        }
    }

    #[test]
    fn test_version_info() {
        let info = VersionInfo {
            name: "test".to_string(),
            version: "1.0.0".to_string(),
            description: "Test".to_string(),
            project: "Test".to_string(),
            rust_version: "1.75".to_string(),
            target: "x86_64".to_string(),
        };

        let json = serde_json::to_string(&info).unwrap();
        assert!(json.contains("1.0.0"));
    }

    #[test]
    fn test_print_hex() {
        // Just verify it doesn't panic
        let data = vec![0x48, 0x65, 0x6c, 0x6c, 0x6f];
        print_hex(&data);
    }
}
