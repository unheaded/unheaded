//! # Qi (氣) — Vital Breath
//!
//! Gossip propagation for Zhen fragments. SWIM-variant protocol
//! over UDP, piggybacking on existing Monad transport where possible.
//!
//! Qi never generates its own traffic when Monad transport is available.
//! Fragments ride the breath of existing packet flows.

pub mod gossip;
pub mod peer;
pub mod transport;

pub use gossip::GossipEngine;
pub use peer::{Peer, PeerState};
