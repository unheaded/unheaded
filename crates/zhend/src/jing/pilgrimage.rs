//! Pilgrimage — deep retrieval from Jing strata.
//!
//! When De surfaces a fragment ID that exists only in L3 (Jing),
//! a Pilgrimage is initiated. This is intentionally slow — seconds
//! to minutes depending on archive depth.
//!
//! The Pilgrimage represents the philosophical cost of deep knowledge:
//! truth that hasn't been accessed recently requires effort to reach.
//! The journey IS the retrieval.

use crate::pu::fragment::FragmentId;
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

/// Status of an ongoing Pilgrimage.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PilgrimageStatus {
    /// The fragment being sought.
    pub target: FragmentId,
    /// When the pilgrimage began.
    pub started_at: DateTime<Utc>,
    /// Current state.
    pub state: PilgrimageState,
    /// Entries scanned so far.
    pub entries_scanned: u64,
    /// Total archive entries (if known).
    pub total_entries: Option<u64>,
}

/// State machine for a Pilgrimage.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum PilgrimageState {
    /// Scanning Jing archive sequentially.
    Scanning,
    /// Fragment found — promoting to L1.
    Found,
    /// Fragment not in local Jing — may exist on remote peers.
    NotFoundLocally,
    /// Pilgrimage failed (I/O error, corrupt archive, etc.).
    Failed,
}

// TODO: async pilgrimage runner that reports progress via channel
// TODO: distributed pilgrimage — ask remote peers' Jing archives
// TODO: pilgrimage cache — remember recent pilgrimage results to avoid rescan
