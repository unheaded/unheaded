// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//! # Qi (氣) — Vital Breath
//!
//! Gossip propagation for Zhen fragments. SWIM-variant protocol over UDP.
//!
//! Riding existing Monad packets (a Zhen Hop-by-Hop option) was the plan and
//! was never wired. The option codec and the Monad bridge that would pick
//! fragments for it are archived in `unheaded-zhend-subsystems` (ADR-152).

pub mod gossip;
pub mod message;
pub mod peer;
pub mod transport;

pub use gossip::GossipEngine;
pub use message::GossipMessage;
pub use peer::{Peer, PeerList, PeerState};
pub use transport::UdpTransport;
