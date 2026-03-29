//! # zhend — Zhen Layer 0 Daemon
//!
//! The anti-fragile knowledge substrate for the Unheaded Protocol.
//!
//! ```text
//! 真 — Truth. The Uncarved Block. The Library that cannot burn.
//! ```
//!
//! Usage:
//!   zhend                     # Start with default config
//!   zhend --config /etc/zhend/config.toml
//!   zhend --data-dir /var/lib/zhend
//!   zhend --grpc-addr [::1]:7300
//!   zhend --seed-peer [::1]:7302
//!
//! GPL-3.0-only

use std::sync::Arc;
use clap::Parser;
use tokio::sync::mpsc;
use zhend::ZhenConfig;
use zhend::pu::TieredStore;
use zhend::qi::GossipEngine;
use zhend::de::embedder::Embedder;
use zhend::li::strata::StrataHistory;

/// Zhen Layer 0 — Anti-fragile knowledge substrate.
#[derive(Parser, Debug)]
#[command(name = "zhend")]
#[command(about = "真 — The Library that cannot burn.")]
#[command(version)]
struct Cli {
    /// Data directory for all storage tiers.
    #[arg(long, default_value = "/var/lib/zhend")]
    data_dir: String,

    /// gRPC listen address.
    #[arg(long, default_value = "[::1]:7300")]
    grpc_addr: String,

    /// QUIC listen address.
    #[arg(long, default_value = "[::1]:7301")]
    quic_addr: String,

    /// Gossip listen address (UDP).
    #[arg(long, default_value = "[::]:7302")]
    gossip_addr: String,

    /// Seed peers for initial gossip join (repeatable).
    #[arg(long = "seed-peer")]
    seed_peers: Vec<String>,

    /// L1 → L2 sedimentation threshold in seconds.
    #[arg(long, default_value = "3600")]
    l1_to_l2_secs: u64,

    /// L2 → L3 sedimentation threshold in seconds.
    #[arg(long, default_value = "604800")]
    l2_to_l3_secs: u64,

    /// Embedding model path (ONNX). Omit for no-embedding mode.
    #[arg(long)]
    embedding_model: Option<String>,

    /// Embedding dimensions.
    #[arg(long, default_value = "384")]
    embedding_dims: usize,

    /// Gossip fanout per cycle.
    #[arg(long, default_value = "3")]
    gossip_fanout: usize,

    /// Gossip cycle interval in milliseconds.
    #[arg(long, default_value = "1000")]
    gossip_interval_ms: u64,

    /// Log format: "json" or "pretty".
    #[arg(long, default_value = "pretty")]
    log_format: String,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let cli = Cli::parse();

    // Initialize tracing.
    init_tracing(&cli.log_format);

    tracing::info!(
        data_dir = %cli.data_dir,
        grpc_addr = %cli.grpc_addr,
        quic_addr = %cli.quic_addr,
        gossip_addr = %cli.gossip_addr,
        seed_peers = ?cli.seed_peers,
        "真 zhend starting — the library that cannot burn"
    );

    // Build config.
    let config = ZhenConfig {
        data_dir: cli.data_dir.into(),
        grpc_addr: cli.grpc_addr,
        quic_addr: cli.quic_addr,
        gossip_addr: cli.gossip_addr,
        seed_peers: cli.seed_peers,
        l1_to_l2_secs: cli.l1_to_l2_secs,
        l2_to_l3_secs: cli.l2_to_l3_secs,
        embedding_model: cli.embedding_model.map(Into::into),
        embedding_dims: cli.embedding_dims,
        gossip_fanout: cli.gossip_fanout,
        gossip_interval_ms: cli.gossip_interval_ms,
    };

    // Open tiered store.
    let store = Arc::new(TieredStore::open(&config.data_dir)?);
    tracing::info!("pu tiered store opened");

    // Initialize embedder.
    let embedder = Arc::new(Embedder::new(config.embedding_dims));
    if embedder.is_ready() {
        tracing::info!(dims = config.embedding_dims, "de embedder loaded");
    } else {
        tracing::warn!("de embedder not configured — running without De (relevance). fragments will lack embeddings.");
    }

    // Initialize strata history for Li observation.
    let _strata_history = StrataHistory::new(1440); // ~24h at 1/min

    // Shutdown channels.
    let (gossip_tx, gossip_rx) = mpsc::channel(1);
    let (sediment_tx, mut sediment_rx) = mpsc::channel::<()>(1);

    // Spawn gossip engine (Qi).
    let gossip_store = Arc::clone(&store);
    let gossip_config = config.clone();
    let gossip_handle = tokio::spawn(async move {
        let mut engine = GossipEngine::new(gossip_store, gossip_config, gossip_rx, None);
        if let Err(e) = engine.run().await {
            tracing::error!(error = %e, "qi gossip engine failed");
        }
    });

    // Spawn sedimentation loop (L1 → L2 migration).
    let sediment_store = Arc::clone(&store);
    let sediment_interval = config.l1_to_l2_secs;
    let sediment_handle = tokio::spawn(async move {
        let interval = tokio::time::Duration::from_secs(
            std::cmp::max(sediment_interval / 10, 60) // check every 1/10th of threshold, min 60s
        );

        loop {
            tokio::select! {
                _ = tokio::time::sleep(interval) => {
                    match sediment_store.sediment(sediment_interval) {
                        Ok(n) if n > 0 => {
                            tracing::info!(migrated = n, "sedimentation cycle: L1 → L2");
                        }
                        Ok(_) => {} // nothing to migrate
                        Err(e) => {
                            tracing::warn!(error = %e, "sedimentation cycle failed");
                        }
                    }
                }
                _ = sediment_rx.recv() => {
                    tracing::info!("sedimentation loop shutting down");
                    break;
                }
            }
        }
    });

    // Spawn deep sedimentation loop (L2 → L3 migration).
    let deep_store = Arc::clone(&store);
    let deep_interval_secs = config.l2_to_l3_secs;
    let (deep_tx, mut deep_rx) = mpsc::channel::<()>(1);
    let deep_handle = tokio::spawn(async move {
        // Check every 1/10th of threshold, minimum 600s (10 min).
        let interval = tokio::time::Duration::from_secs(
            std::cmp::max(deep_interval_secs / 10, 600)
        );

        loop {
            tokio::select! {
                _ = tokio::time::sleep(interval) => {
                    match deep_store.deep_sediment(deep_interval_secs) {
                        Ok(n) if n > 0 => {
                            tracing::info!(migrated = n, "deep sedimentation cycle: L2 → L3");
                        }
                        Ok(_) => {}
                        Err(e) => {
                            tracing::warn!(error = %e, "deep sedimentation cycle failed");
                        }
                    }
                }
                _ = deep_rx.recv() => {
                    tracing::info!("deep sedimentation loop shutting down");
                    break;
                }
            }
        }
    });

    // Spawn gRPC server.
    let grpc_store = Arc::clone(&store);
    let grpc_embedder = Arc::clone(&embedder);
    let grpc_config = config.clone();
    let grpc_addr = config.grpc_addr.clone();
    let grpc_handle = tokio::spawn(async move {
        let service = zhend::api::grpc::ZhenService::new(grpc_store, grpc_embedder, grpc_config);
        if let Err(e) = zhend::api::grpc::serve(service, &grpc_addr).await {
            tracing::error!(error = %e, "gRPC server failed");
        }
    });

    // TODO: spawn QUIC server
    // TODO: spawn Monad bridge listener
    // TODO: spawn Li topology observer
    // TODO: spawn strata snapshot collector

    // Wait for shutdown signal.
    tracing::info!("真 zhend ready — knowledge flows through non-action");

    tokio::signal::ctrl_c().await?;
    tracing::info!("shutdown signal received — graceful shutdown");

    // Snapshot L1 to disk before shutting down subsystems.
    match store.snapshot() {
        Ok(n) => tracing::info!(fragments = n, "L1 snapshot persisted to disk"),
        Err(e) => tracing::error!(error = %e, "L1 snapshot FAILED — hot fragments may be lost on restart"),
    }

    // Signal all subsystems.
    let _ = gossip_tx.send(()).await;
    let _ = sediment_tx.send(()).await;
    let _ = deep_tx.send(()).await;

    // Abort gRPC server (it blocks on serve()).
    grpc_handle.abort();

    // Wait for subsystems.
    let _ = tokio::time::timeout(
        tokio::time::Duration::from_secs(5),
        async {
            let _ = gossip_handle.await;
            let _ = sediment_handle.await;
            let _ = deep_handle.await;
        }
    ).await;

    tracing::info!("真 zhend shutdown complete — the library endures");
    Ok(())
}

fn init_tracing(format: &str) {
    use tracing_subscriber::EnvFilter;

    let filter = EnvFilter::try_from_default_env()
        .unwrap_or_else(|_| EnvFilter::new("zhend=info"));

    match format {
        "json" => {
            tracing_subscriber::fmt()
                .with_env_filter(filter)
                .json()
                .init();
        }
        _ => {
            tracing_subscriber::fmt()
                .with_env_filter(filter)
                .init();
        }
    }
}
