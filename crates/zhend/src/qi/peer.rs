//! Peer discovery and membership for the Qi gossip mesh.
//!
//! SWIM-style failure detection:
//! Alive → Suspect (missed pings) → Dead (confirmed unreachable)
//!
//! Incarnation numbers disambiguate stale state: a peer that rejoins
//! with a higher incarnation supersedes any prior Suspect/Dead state.

use std::collections::HashMap;
use std::net::SocketAddr;
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

#[cfg(feature = "pq")]
use crate::crypto::sign::PeerIdentity;

/// State of a known peer in the gossip mesh.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum PeerState {
    /// Peer is alive and responding.
    Alive,
    /// Peer suspected dead (missed pings).
    Suspect,
    /// Peer confirmed dead (removed from active set).
    Dead,
}

/// A peer in the Zhen gossip mesh.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Peer {
    /// Peer's gossip address.
    pub addr: SocketAddr,
    /// Current membership state.
    pub state: PeerState,
    /// Last time we heard from this peer.
    pub last_seen: DateTime<Utc>,
    /// Incarnation number (SWIM protocol — disambiguates stale state).
    pub incarnation: u64,
    /// Fragment count reported by peer (informational).
    pub fragment_count: u64,
    /// How many consecutive pings have gone unanswered.
    pub missed_pings: u32,
    /// PQ peer identity (ML-DSA-65). None until identity exchange completes.
    #[cfg(feature = "pq")]
    pub identity: Option<PeerIdentity>,
    /// Whether this peer has been authenticated via PQ identity verification.
    #[cfg(feature = "pq")]
    pub authenticated: bool,
}

impl Peer {
    pub fn new(addr: SocketAddr) -> Self {
        Self {
            addr,
            state: PeerState::Alive,
            last_seen: Utc::now(),
            incarnation: 0,
            fragment_count: 0,
            missed_pings: 0,
            #[cfg(feature = "pq")]
            identity: None,
            #[cfg(feature = "pq")]
            authenticated: false,
        }
    }

    pub fn is_alive(&self) -> bool {
        self.state == PeerState::Alive
    }

    /// Record a successful contact from this peer.
    pub fn mark_alive(&mut self, incarnation: u64, fragment_count: u64) {
        // Only update if incarnation is >= what we know.
        // Higher incarnation = peer restarted or refuted suspicion.
        if incarnation >= self.incarnation {
            self.incarnation = incarnation;
            self.fragment_count = fragment_count;
            self.state = PeerState::Alive;
            self.last_seen = Utc::now();
            self.missed_pings = 0;
        }
    }

    /// Record a missed ping. Transitions Alive → Suspect after threshold.
    pub fn record_missed_ping(&mut self, suspect_threshold: u32) {
        self.missed_pings = self.missed_pings.saturating_add(1);
        if self.missed_pings >= suspect_threshold && self.state == PeerState::Alive {
            self.state = PeerState::Suspect;
            tracing::warn!(
                peer = %self.addr,
                missed = self.missed_pings,
                "peer suspected dead"
            );
        }
    }

    /// Set peer identity after verifying self-attestation.
    /// ALL INPUTS HOSTILE — verify before trusting.
    #[cfg(feature = "pq")]
    pub fn set_identity(&mut self, identity: PeerIdentity) -> crate::ZhenResult<()> {
        // Verify the self-attestation. If it fails, this peer is lying.
        if !identity.verify_self()? {
            return Err(crate::ZhenError::Gossip(
                "peer identity self-attestation failed verification".into(),
            ));
        }
        self.identity = Some(identity);
        self.authenticated = true;
        tracing::info!(peer = %self.addr, "peer authenticated via ML-DSA-65");
        Ok(())
    }

    /// Whether this peer has been authenticated via PQ identity.
    #[cfg(feature = "pq")]
    pub fn is_authenticated(&self) -> bool {
        self.authenticated
    }

    /// Confirm death. Transitions Suspect → Dead.
    pub fn confirm_dead(&mut self) {
        if self.state == PeerState::Suspect {
            self.state = PeerState::Dead;
            tracing::warn!(peer = %self.addr, "peer confirmed dead");
        }
    }
}

/// Number of consecutive missed pings before suspecting a peer.
const SUSPECT_THRESHOLD: u32 = 3;

/// Number of consecutive missed pings before confirming dead.
const DEAD_THRESHOLD: u32 = 6;

/// Manages the set of known peers in the gossip mesh.
pub struct PeerList {
    peers: HashMap<SocketAddr, Peer>,
    /// Our own incarnation number, incremented on startup.
    pub local_incarnation: u64,
}

impl Default for PeerList {
    fn default() -> Self {
        Self::new()
    }
}

impl PeerList {
    pub fn new() -> Self {
        Self {
            peers: HashMap::new(),
            local_incarnation: 1,
        }
    }

    /// Add a peer (or update if already known with higher incarnation).
    pub fn add_or_update(&mut self, addr: SocketAddr, incarnation: u64, fragment_count: u64) {
        match self.peers.get_mut(&addr) {
            Some(peer) => {
                peer.mark_alive(incarnation, fragment_count);
            }
            None => {
                let mut peer = Peer::new(addr);
                peer.incarnation = incarnation;
                peer.fragment_count = fragment_count;
                tracing::info!(%addr, "new peer discovered");
                self.peers.insert(addr, peer);
            }
        }
    }

    /// Add seed peers from config (initial bootstrap).
    pub fn add_seeds(&mut self, seeds: &[SocketAddr]) {
        for addr in seeds {
            if !self.peers.contains_key(addr) {
                self.peers.insert(*addr, Peer::new(*addr));
                tracing::info!(%addr, "seed peer added");
            }
        }
    }

    /// Get all alive peers.
    pub fn alive_peers(&self) -> Vec<&Peer> {
        self.peers.values().filter(|p| p.is_alive()).collect()
    }

    /// Get all alive AND authenticated peers (for fragment data exchange).
    #[cfg(feature = "pq")]
    pub fn authenticated_alive_peers(&self) -> Vec<&Peer> {
        self.peers.values().filter(|p| p.is_alive() && p.authenticated).collect()
    }

    /// Get a mutable reference to a peer by address.
    pub fn get_mut(&mut self, addr: &SocketAddr) -> Option<&mut Peer> {
        self.peers.get_mut(addr)
    }

    /// Get all non-dead peers (alive + suspect — still worth trying).
    pub fn active_peers(&self) -> Vec<&Peer> {
        self.peers.values().filter(|p| p.state != PeerState::Dead).collect()
    }

    /// Record a missed ping for a peer. Handles state transitions.
    pub fn record_missed_ping(&mut self, addr: &SocketAddr) {
        if let Some(peer) = self.peers.get_mut(addr) {
            peer.record_missed_ping(SUSPECT_THRESHOLD);
            if peer.missed_pings >= DEAD_THRESHOLD && peer.state == PeerState::Suspect {
                peer.confirm_dead();
            }
        }
    }

    /// Record successful contact from a peer.
    pub fn record_contact(&mut self, addr: SocketAddr, incarnation: u64, fragment_count: u64) {
        self.add_or_update(addr, incarnation, fragment_count);
    }

    /// Number of known peers (any state).
    pub fn len(&self) -> usize {
        self.peers.len()
    }

    /// Check if peer list is empty.
    pub fn is_empty(&self) -> bool {
        self.peers.is_empty()
    }

    /// Direct access to the peers map (for auth checks in gossip engine).
    pub fn peers_ref(&self) -> &HashMap<SocketAddr, Peer> {
        &self.peers
    }

    /// Remove dead peers that have been dead for too long (garbage collection).
    pub fn reap_dead(&mut self, max_dead_age: chrono::Duration) {
        let now = Utc::now();
        self.peers.retain(|_, peer| {
            if peer.state == PeerState::Dead {
                if let Ok(age) = now.signed_duration_since(peer.last_seen).to_std() {
                    if age > max_dead_age.to_std().unwrap_or_default() {
                        tracing::info!(peer = %peer.addr, "reaping dead peer");
                        return false;
                    }
                }
            }
            true
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_addr(port: u16) -> SocketAddr {
        SocketAddr::from(([127, 0, 0, 1], port))
    }

    #[cfg(feature = "pq")]
    mod pq_auth_tests {
        use super::*;
        use crate::crypto::sign::{PeerIdentity, PqSigningKeypair};

        #[test]
        fn test_set_valid_identity_succeeds() {
            let mut peer = Peer::new(test_addr(5000));
            assert!(!peer.authenticated);
            assert!(peer.identity.is_none());

            let kp = PqSigningKeypair::generate();
            let identity = PeerIdentity::new(&kp, Some("legit-node".into())).unwrap();

            peer.set_identity(identity).expect("valid identity should succeed");
            assert!(peer.authenticated);
            assert!(peer.identity.is_some());
        }

        #[test]
        fn test_set_tampered_identity_fails() {
            let mut peer = Peer::new(test_addr(5001));

            let kp = PqSigningKeypair::generate();
            let mut identity = PeerIdentity::new(&kp, Some("honest-node".into())).unwrap();

            // Tamper: change the name after signing.
            identity.name = Some("evil-node".into());

            let result = peer.set_identity(identity);
            assert!(result.is_err(), "tampered identity must be rejected");
            assert!(!peer.authenticated);
            assert!(peer.identity.is_none());
        }

        #[test]
        fn test_authenticated_alive_peers_filter() {
            let mut pl = PeerList::new();
            let addr1 = test_addr(5010);
            let addr2 = test_addr(5011);
            let addr3 = test_addr(5012);

            pl.add_or_update(addr1, 1, 0);
            pl.add_or_update(addr2, 1, 0);
            pl.add_or_update(addr3, 1, 0);

            // All alive, none authenticated.
            assert_eq!(pl.alive_peers().len(), 3);
            assert_eq!(pl.authenticated_alive_peers().len(), 0);

            // Authenticate peer 1 and 3.
            let kp1 = PqSigningKeypair::generate();
            let id1 = PeerIdentity::new(&kp1, Some("node-1".into())).unwrap();
            pl.get_mut(&addr1).unwrap().set_identity(id1).unwrap();

            let kp3 = PqSigningKeypair::generate();
            let id3 = PeerIdentity::new(&kp3, Some("node-3".into())).unwrap();
            pl.get_mut(&addr3).unwrap().set_identity(id3).unwrap();

            assert_eq!(pl.authenticated_alive_peers().len(), 2);

            // Kill peer 3 — should no longer appear in authenticated alive.
            for _ in 0..SUSPECT_THRESHOLD {
                pl.record_missed_ping(&addr3);
            }
            // peer3 is now Suspect, not Alive.
            assert_eq!(pl.authenticated_alive_peers().len(), 1);
            assert_eq!(pl.authenticated_alive_peers()[0].addr, addr1);
        }
    }

    #[test]
    fn test_peer_state_transitions_alive_to_suspect() {
        let mut peer = Peer::new(test_addr(1000));
        assert_eq!(peer.state, PeerState::Alive);

        // Miss pings up to threshold.
        for _ in 0..SUSPECT_THRESHOLD - 1 {
            peer.record_missed_ping(SUSPECT_THRESHOLD);
            assert_eq!(peer.state, PeerState::Alive);
        }

        // One more miss → Suspect.
        peer.record_missed_ping(SUSPECT_THRESHOLD);
        assert_eq!(peer.state, PeerState::Suspect);
    }

    #[test]
    fn test_peer_state_transitions_suspect_to_dead() {
        let mut peer = Peer::new(test_addr(1001));

        // Drive to Suspect.
        for _ in 0..SUSPECT_THRESHOLD {
            peer.record_missed_ping(SUSPECT_THRESHOLD);
        }
        assert_eq!(peer.state, PeerState::Suspect);

        // Confirm dead.
        peer.confirm_dead();
        assert_eq!(peer.state, PeerState::Dead);
    }

    #[test]
    fn test_peer_refutes_suspicion_with_higher_incarnation() {
        let mut peer = Peer::new(test_addr(1002));

        // Drive to Suspect.
        for _ in 0..SUSPECT_THRESHOLD {
            peer.record_missed_ping(SUSPECT_THRESHOLD);
        }
        assert_eq!(peer.state, PeerState::Suspect);

        // Peer comes back with higher incarnation → Alive again.
        peer.mark_alive(5, 100);
        assert_eq!(peer.state, PeerState::Alive);
        assert_eq!(peer.incarnation, 5);
        assert_eq!(peer.missed_pings, 0);
    }

    #[test]
    fn test_peer_stale_incarnation_ignored() {
        let mut peer = Peer::new(test_addr(1003));
        peer.mark_alive(10, 500);
        assert_eq!(peer.incarnation, 10);

        // Stale incarnation (lower) should not update.
        peer.mark_alive(5, 50);
        // State should remain from incarnation 10.
        assert_eq!(peer.incarnation, 10);
        assert_eq!(peer.fragment_count, 500);
    }

    #[test]
    fn test_peer_list_add_seeds() {
        let mut pl = PeerList::new();
        let seeds = vec![test_addr(2000), test_addr(2001)];
        pl.add_seeds(&seeds);
        assert_eq!(pl.len(), 2);
        assert_eq!(pl.alive_peers().len(), 2);
    }

    #[test]
    fn test_peer_list_active_excludes_dead() {
        let mut pl = PeerList::new();
        let addr = test_addr(3000);
        pl.add_or_update(addr, 1, 0);

        // Kill the peer.
        for _ in 0..SUSPECT_THRESHOLD {
            pl.record_missed_ping(&addr);
        }
        for _ in 0..DEAD_THRESHOLD - SUSPECT_THRESHOLD {
            pl.record_missed_ping(&addr);
        }

        // Peer should be dead.
        assert_eq!(pl.alive_peers().len(), 0);
        assert_eq!(pl.active_peers().len(), 0);
    }

    #[test]
    fn test_peer_list_record_contact() {
        let mut pl = PeerList::new();
        let addr = test_addr(4000);

        // First contact creates the peer.
        pl.record_contact(addr, 1, 42);
        assert_eq!(pl.len(), 1);
        let peers = pl.alive_peers();
        assert_eq!(peers[0].fragment_count, 42);

        // Subsequent contact updates.
        pl.record_contact(addr, 2, 100);
        let peers = pl.alive_peers();
        assert_eq!(peers[0].incarnation, 2);
        assert_eq!(peers[0].fragment_count, 100);
    }
}
