//! SWIM-variant gossip engine for fragment propagation.
//!
//! Each gossip cycle:
//! 1. Select random fragments from L1 (weighted by recency)
//! 2. Select random peers (fanout)
//! 3. Send fragment digests (BLAKE3 hashes only)
//! 4. Peers respond with "want" for unknown hashes
//! 5. Send full fragments for wanted hashes
//!
//! This is pull-on-demand, not push-everything. Bandwidth-efficient.

use std::sync::Arc;
use tokio::sync::mpsc;
use crate::pu::TieredStore;
use crate::ZhenConfig;

/// Gossip engine manages periodic fragment dissemination.
pub struct GossipEngine {
    store: Arc<TieredStore>,
    config: ZhenConfig,
    shutdown: mpsc::Receiver<()>,
}

impl GossipEngine {
    pub fn new(
        store: Arc<TieredStore>,
        config: ZhenConfig,
        shutdown: mpsc::Receiver<()>,
    ) -> Self {
        Self { store, config, shutdown }
    }

    /// Run the gossip loop. Blocks until shutdown signal.
    pub async fn run(&mut self) -> crate::ZhenResult<()> {
        let interval = tokio::time::Duration::from_millis(self.config.gossip_interval_ms);

        tracing::info!(
            fanout = self.config.gossip_fanout,
            interval_ms = self.config.gossip_interval_ms,
            "qi gossip engine started — vital breath flowing"
        );

        loop {
            tokio::select! {
                _ = tokio::time::sleep(interval) => {
                    if let Err(e) = self.cycle().await {
                        tracing::warn!(error = %e, "gossip cycle failed");
                    }
                }
                _ = self.shutdown.recv() => {
                    tracing::info!("qi gossip engine shutting down");
                    break;
                }
            }
        }

        Ok(())
    }

    /// Execute one gossip cycle.
    async fn cycle(&self) -> crate::ZhenResult<()> {
        // 1. Get L1 fragment IDs.
        let ids = self.store.l1_ids()?;
        if ids.is_empty() {
            return Ok(());
        }

        // 2. Select random subset (up to gossip_fanout fragments).
        let selected: Vec<_> = ids.iter()
            .take(self.config.gossip_fanout)
            .collect();

        tracing::trace!(
            fragment_count = selected.len(),
            "gossip cycle: broadcasting digests"
        );

        // TODO: actual UDP peer communication
        // For now, this is the structural skeleton.
        // Wire protocol: [digest_count: u16][blake3_hash: 32 bytes]*N

        Ok(())
    }
}
