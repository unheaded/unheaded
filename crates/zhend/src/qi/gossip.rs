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

use rand::seq::SliceRandom;
use rand::SeedableRng;
use std::net::SocketAddr;
use std::sync::Arc;
use tokio::sync::{mpsc, RwLock};

use crate::pu::{codec, TieredStore};
use crate::ZhenConfig;

#[cfg(feature = "pq")]
use crate::crypto::sign::{PeerIdentity, PqSigningKeypair};

use super::message::{GossipMessage, MAX_DIGESTS_PER_BATCH, MAX_FRAGMENT_BATCH_BYTES};
use super::peer::PeerList;
use super::transport::{UdpTransport, MAX_MSG_SIZE};

/// Gossip engine manages periodic fragment dissemination.
pub struct GossipEngine {
    store: Arc<TieredStore>,
    config: ZhenConfig,
    shutdown: mpsc::Receiver<()>,
    peers: Arc<RwLock<PeerList>>,
    /// Our PQ identity for peer authentication.
    #[cfg(feature = "pq")]
    local_identity: Arc<Option<PeerIdentity>>,
}

impl GossipEngine {
    /// Create a new gossip engine.
    ///
    /// If a PQ signing keypair is provided, generates a local PeerIdentity
    /// for authenticated gossip. Without it, the engine runs unauthenticated
    /// (all peers admitted without identity verification).
    #[cfg(feature = "pq")]
    pub fn new(
        store: Arc<TieredStore>,
        config: ZhenConfig,
        shutdown: mpsc::Receiver<()>,
        signing_keypair: Option<&PqSigningKeypair>,
    ) -> Self {
        let local_identity = signing_keypair
            .map(|kp| PeerIdentity::new(kp, None).expect("local identity generation must succeed"));

        Self {
            store,
            config,
            shutdown,
            peers: Arc::new(RwLock::new(PeerList::new())),
            local_identity: Arc::new(local_identity),
        }
    }

    /// Create a new gossip engine without PQ authentication (non-pq build).
    #[cfg(not(feature = "pq"))]
    pub fn new(store: Arc<TieredStore>, config: ZhenConfig, shutdown: mpsc::Receiver<()>) -> Self {
        Self {
            store,
            config,
            shutdown,
            peers: Arc::new(RwLock::new(PeerList::new())),
        }
    }

    /// Run the gossip loop. Blocks until shutdown signal.
    pub async fn run(&mut self) -> crate::ZhenResult<()> {
        // Bind UDP transport.
        let transport = Arc::new(UdpTransport::bind(&self.config.gossip_addr).await?);

        // Add seed peers.
        {
            let mut peers = self.peers.write().await;
            let seed_addrs: Vec<SocketAddr> = self
                .config
                .seed_peers
                .iter()
                .filter_map(|s| s.parse().ok())
                .collect();
            peers.add_seeds(&seed_addrs);
        }

        let interval = tokio::time::Duration::from_millis(self.config.gossip_interval_ms);

        tracing::info!(
            fanout = self.config.gossip_fanout,
            interval_ms = self.config.gossip_interval_ms,
            seeds = self.config.seed_peers.len(),
            "qi gossip engine started — vital breath flowing"
        );

        // Send IdentityOffer to all seed peers on startup.
        #[cfg(feature = "pq")]
        {
            if let Some(ref identity) = *self.local_identity {
                let offer = GossipMessage::IdentityOffer {
                    identity: identity.clone(),
                };
                let peer_list = self.peers.read().await;
                let seed_addrs: Vec<SocketAddr> =
                    peer_list.active_peers().iter().map(|p| p.addr).collect();
                drop(peer_list);

                for addr in &seed_addrs {
                    if let Err(e) = Self::send_message_static(&transport, &offer, *addr).await {
                        tracing::warn!(error = %e, peer = %addr, "failed to send identity offer to seed");
                    }
                }
                tracing::info!(
                    seeds = seed_addrs.len(),
                    "identity offers sent to seed peers"
                );
            }
        }

        // Allocate recv buffer outside the loop.
        let mut buf = vec![0u8; MAX_MSG_SIZE];

        loop {
            tokio::select! {
                _ = tokio::time::sleep(interval) => {
                    if let Err(e) = Self::cycle_static(
                        &self.store,
                        &self.config,
                        &self.peers,
                        &transport,
                        #[cfg(feature = "pq")]
                        &self.local_identity,
                    ).await {
                        tracing::warn!(error = %e, "gossip cycle failed");
                    }
                }
                result = transport.recv_from(&mut buf) => {
                    match result {
                        Ok((n, from)) => {
                            match GossipMessage::decode(&buf[..n]) {
                                Ok(msg) => {
                                    if let Err(e) = Self::handle_message_static(
                                        &self.store,
                                        &self.peers,
                                        &transport,
                                        msg,
                                        from,
                                        #[cfg(feature = "pq")]
                                        &self.local_identity,
                                    ).await {
                                        tracing::warn!(error = %e, peer = %from, "failed handling gossip message");
                                    }
                                }
                                Err(e) => {
                                    tracing::warn!(error = %e, peer = %from, "failed decoding gossip message");
                                }
                            }
                        }
                        Err(e) => {
                            tracing::warn!(error = %e, "gossip recv error");
                        }
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

    /// Handle an incoming gossip message from a peer (static version to avoid borrow conflicts).
    async fn handle_message_static(
        store: &Arc<TieredStore>,
        peers: &Arc<RwLock<PeerList>>,
        transport: &Arc<UdpTransport>,
        msg: GossipMessage,
        from: SocketAddr,
        #[cfg(feature = "pq")] local_identity: &Arc<Option<PeerIdentity>>,
    ) -> crate::ZhenResult<()> {
        // PQ admission control: if WE have an identity and the peer is unauthenticated
        // and sends a data message (not Ping/PingAck/Identity*), send an IdentityOffer
        // and DROP their message. They get zero fragment data until authenticated.
        // If we have no identity (None keypair), gossip runs open (pre-auth mode).
        #[cfg(feature = "pq")]
        if local_identity.is_some() {
            let is_identity_msg = matches!(
                msg,
                GossipMessage::IdentityOffer { .. }
                    | GossipMessage::IdentityAck { .. }
                    | GossipMessage::Ping { .. }
                    | GossipMessage::PingAck { .. }
            );

            if !is_identity_msg {
                let peer_list = peers.read().await;
                let peer_authenticated = peer_list
                    .peers_ref()
                    .get(&from)
                    .is_some_and(|p| p.authenticated);
                drop(peer_list);

                if !peer_authenticated {
                    tracing::warn!(
                        peer = %from,
                        "unauthenticated peer sent data message — sending IdentityOffer, dropping message"
                    );
                    if let Some(ref identity) = **local_identity {
                        let offer = GossipMessage::IdentityOffer {
                            identity: identity.clone(),
                        };
                        Self::send_message_static(transport, &offer, from).await?;
                    }
                    return Ok(());
                }
            }
        }

        match msg {
            GossipMessage::Ping { incarnation } => {
                // Record contact and reply with PingAck.
                {
                    let mut peer_list = peers.write().await;
                    peer_list.record_contact(from, incarnation, 0);
                }

                let count = store.count()? as u64;
                let local_incarnation = peers.read().await.local_incarnation;
                let ack = GossipMessage::PingAck {
                    incarnation: local_incarnation,
                    fragment_count: count,
                };

                Self::send_message_static(transport, &ack, from).await?;
            }

            GossipMessage::PingAck {
                incarnation,
                fragment_count,
            } => {
                let mut peer_list = peers.write().await;
                peer_list.record_contact(from, incarnation, fragment_count);
            }

            GossipMessage::DigestBatch { digests } => {
                // ALL INPUTS HOSTILE — cap digest count.
                if digests.len() > MAX_DIGESTS_PER_BATCH {
                    return Err(crate::ZhenError::Gossip(format!(
                        "digest batch too large: {} > {}",
                        digests.len(),
                        MAX_DIGESTS_PER_BATCH
                    )));
                }

                // Check which fragments we don't have.
                let mut wanted = Vec::new();
                for digest in &digests {
                    let id = crate::pu::FragmentId(*digest);
                    if !store.contains(&id)? {
                        wanted.push(*digest);
                    }
                }

                if !wanted.is_empty() {
                    tracing::debug!(
                        wanted = wanted.len(),
                        offered = digests.len(),
                        peer = %from,
                        "requesting fragments from peer"
                    );

                    let want = GossipMessage::WantBatch { wanted };
                    Self::send_message_static(transport, &want, from).await?;
                }

                // Also record contact (peer is alive).
                {
                    let mut peer_list = peers.write().await;
                    peer_list.record_contact(from, 0, 0);
                }
            }

            GossipMessage::WantBatch { wanted } => {
                // Peer wants these fragments — look them up and send.
                // ALL INPUTS HOSTILE — cap request size.
                if wanted.len() > MAX_DIGESTS_PER_BATCH {
                    return Err(crate::ZhenError::Gossip(format!(
                        "want batch too large: {} > {}",
                        wanted.len(),
                        MAX_DIGESTS_PER_BATCH
                    )));
                }

                let mut fragments = Vec::new();
                let mut total_bytes = 0usize;

                for hash in &wanted {
                    let id = crate::pu::FragmentId(*hash);
                    if let Some(frag) = store.get(&id)? {
                        let encoded = codec::encode_for_gossip(&frag)?;
                        total_bytes += encoded.len();

                        // Chunk: don't exceed max batch size.
                        if total_bytes > MAX_FRAGMENT_BATCH_BYTES {
                            // Send what we have so far.
                            let batch = GossipMessage::FragmentBatch {
                                fragments: std::mem::take(&mut fragments),
                            };
                            Self::send_message_static(transport, &batch, from).await?;
                            total_bytes = encoded.len();
                        }

                        fragments.push(encoded);
                    }
                }

                // Send remaining fragments.
                if !fragments.is_empty() {
                    let batch = GossipMessage::FragmentBatch { fragments };
                    Self::send_message_static(transport, &batch, from).await?;
                }
            }

            #[cfg(feature = "pq")]
            GossipMessage::IdentityOffer { identity } => {
                // Verify the offered identity's self-attestation.
                match identity.verify_self() {
                    Ok(true) => {
                        // Mark peer as authenticated.
                        {
                            let mut peer_list = peers.write().await;
                            peer_list.add_or_update(from, 0, 0);
                            if let Some(peer) = peer_list.get_mut(&from) {
                                peer.identity = Some(identity);
                                peer.authenticated = true;
                            }
                        }
                        tracing::info!(peer = %from, "peer identity verified — authenticated");

                        // Reply with our own identity (IdentityAck).
                        if let Some(ref our_identity) = **local_identity {
                            let ack = GossipMessage::IdentityAck {
                                identity: our_identity.clone(),
                            };
                            Self::send_message_static(transport, &ack, from).await?;
                        }
                    }
                    Ok(false) => {
                        tracing::warn!(peer = %from, "peer identity self-attestation FAILED — rejecting");
                    }
                    Err(e) => {
                        tracing::warn!(error = %e, peer = %from, "peer identity verification error");
                    }
                }
            }

            #[cfg(feature = "pq")]
            GossipMessage::IdentityAck { identity } => {
                // Verify the ack identity's self-attestation.
                match identity.verify_self() {
                    Ok(true) => {
                        let mut peer_list = peers.write().await;
                        peer_list.add_or_update(from, 0, 0);
                        if let Some(peer) = peer_list.get_mut(&from) {
                            peer.identity = Some(identity);
                            peer.authenticated = true;
                        }
                        tracing::info!(peer = %from, "peer identity ack verified — authenticated");
                    }
                    Ok(false) => {
                        tracing::warn!(peer = %from, "peer identity ack self-attestation FAILED — rejecting");
                    }
                    Err(e) => {
                        tracing::warn!(error = %e, peer = %from, "peer identity ack verification error");
                    }
                }
            }

            GossipMessage::FragmentBatch { fragments } => {
                // Ingest received fragments.
                let mut ingested = 0u64;
                for encoded in &fragments {
                    match codec::decode(encoded) {
                        Ok(mut frag) => {
                            // Increment hop count.
                            frag.hop_count = frag.hop_count.saturating_add(1);

                            match store.ingest(frag) {
                                Ok(true) => ingested += 1,
                                Ok(false) => {} // duplicate, fine
                                Err(e) => {
                                    tracing::warn!(
                                        error = %e,
                                        peer = %from,
                                        "rejected fragment from peer"
                                    );
                                }
                            }
                        }
                        Err(e) => {
                            tracing::warn!(
                                error = %e,
                                peer = %from,
                                "failed to decode fragment from peer"
                            );
                        }
                    }
                }

                if ingested > 0 {
                    tracing::info!(
                        ingested,
                        peer = %from,
                        "fragments received via gossip"
                    );
                }

                // Record contact.
                {
                    let mut peer_list = peers.write().await;
                    peer_list.record_contact(from, 0, 0);
                }
            }
        }

        Ok(())
    }

    /// Send a gossip message to a target peer (static version).
    async fn send_message_static(
        transport: &Arc<UdpTransport>,
        msg: &GossipMessage,
        target: SocketAddr,
    ) -> crate::ZhenResult<()> {
        let encoded = msg.encode()?;
        transport.send_to(&encoded, target).await?;
        Ok(())
    }

    /// Execute one gossip cycle (static version to avoid borrow conflicts).
    #[allow(unused_variables)]
    async fn cycle_static(
        store: &Arc<TieredStore>,
        config: &ZhenConfig,
        peers: &Arc<RwLock<PeerList>>,
        transport: &Arc<UdpTransport>,
        #[cfg(feature = "pq")] local_identity: &Arc<Option<PeerIdentity>>,
    ) -> crate::ZhenResult<()> {
        // 1. Get L1 fragment IDs.
        let ids = store.l1_ids()?;
        if ids.is_empty() {
            return Ok(());
        }

        // 2. Select random subset (up to gossip_fanout fragments).
        let mut rng = rand::rngs::StdRng::from_entropy();
        let mut selected = ids;
        selected.shuffle(&mut rng);
        selected.truncate(config.gossip_fanout);

        // 3. Build digest batch.
        let digests: Vec<[u8; 32]> = selected.iter().map(|id| id.0).collect();

        // 4. Get peers for digest sends.
        // With PQ auth: only send fragment data to authenticated peers.
        // Without PQ: send to all active peers.
        let peer_list = peers.read().await;

        // With PQ auth and a local identity: only send digests to authenticated peers.
        // Without identity or without PQ feature: send to all active peers.
        #[cfg(feature = "pq")]
        let digest_targets: Vec<SocketAddr> = if local_identity.is_some() {
            peer_list
                .authenticated_alive_peers()
                .iter()
                .map(|p| p.addr)
                .collect()
        } else {
            peer_list.active_peers().iter().map(|p| p.addr).collect()
        };

        #[cfg(not(feature = "pq"))]
        let digest_targets: Vec<SocketAddr> =
            peer_list.active_peers().iter().map(|p| p.addr).collect();

        // All active peers still get pings (membership protocol is pre-auth).
        let active_addrs: Vec<SocketAddr> =
            peer_list.active_peers().iter().map(|p| p.addr).collect();

        let local_incarnation = peer_list.local_incarnation;
        drop(peer_list); // release read lock before sending

        // 5. Send DigestBatch only to authenticated peers (or all, without PQ).
        if !digest_targets.is_empty() {
            let msg = GossipMessage::DigestBatch { digests };

            tracing::trace!(
                fragment_count = selected.len(),
                peer_count = digest_targets.len(),
                "gossip cycle: broadcasting digests to authenticated peers"
            );

            for addr in &digest_targets {
                if let Err(e) = Self::send_message_static(transport, &msg, *addr).await {
                    tracing::warn!(
                        error = %e,
                        peer = %addr,
                        "failed to send digest batch"
                    );
                }
            }
        }

        // 6. Send Ping to ALL active peers for membership health.
        let ping = GossipMessage::Ping {
            incarnation: local_incarnation,
        };
        for addr in &active_addrs {
            if let Err(e) = Self::send_message_static(transport, &ping, *addr).await {
                tracing::warn!(error = %e, peer = %addr, "failed to send ping");
                let mut peer_list = peers.write().await;
                peer_list.record_missed_ping(addr);
            }
        }

        Ok(())
    }

    /// Access to the peer list (for testing and inspection).
    pub fn peers(&self) -> &Arc<RwLock<PeerList>> {
        &self.peers
    }
}
