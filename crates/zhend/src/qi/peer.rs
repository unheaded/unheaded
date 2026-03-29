//! Peer discovery and membership for the Qi gossip mesh.

use std::net::SocketAddr;
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

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
}

impl Peer {
    pub fn new(addr: SocketAddr) -> Self {
        Self {
            addr,
            state: PeerState::Alive,
            last_seen: Utc::now(),
            incarnation: 0,
            fragment_count: 0,
        }
    }

    pub fn is_alive(&self) -> bool {
        self.state == PeerState::Alive
    }
}
