//! Geological layer management — observing sedimentation patterns.
//!
//! Tracks how fragments migrate between tiers over time.
//! Visualizes the "geological cross-section" of a node's knowledge.

use crate::pu::Tier;
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

/// Snapshot of a node's geological state at a point in time.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StrataSnapshot {
    /// When this snapshot was taken.
    pub timestamp: DateTime<Utc>,
    /// Fragment count in L1 (hot).
    pub l1_count: u64,
    /// Fragment count in L2 (warm).
    pub l2_count: u64,
    /// Fragment count in L3 (cold/Jing).
    pub l3_count: u64,
    /// Total bytes across all tiers.
    pub total_bytes: u64,
    /// Migrations since last snapshot (L1→L2).
    pub migrations_l1_l2: u64,
    /// Migrations since last snapshot (L2→L3).
    pub migrations_l2_l3: u64,
    /// Promotions since last snapshot (any tier → L1 via access).
    pub promotions_to_l1: u64,
}

/// Rolling history of strata snapshots for trend observation.
#[derive(Debug)]
pub struct StrataHistory {
    /// Ring buffer of snapshots. Oldest evicted when full.
    snapshots: Vec<StrataSnapshot>,
    max_snapshots: usize,
}

impl StrataHistory {
    pub fn new(max_snapshots: usize) -> Self {
        Self {
            snapshots: Vec::with_capacity(max_snapshots),
            max_snapshots,
        }
    }

    pub fn push(&mut self, snapshot: StrataSnapshot) {
        if self.snapshots.len() >= self.max_snapshots {
            self.snapshots.remove(0);
        }
        self.snapshots.push(snapshot);
    }

    /// The geological trend: is this node accumulating or shedding knowledge?
    pub fn trend(&self) -> GeologicalTrend {
        if self.snapshots.len() < 2 {
            return GeologicalTrend::Insufficient;
        }

        let first = &self.snapshots[0];
        let last = self.snapshots.last().unwrap();
        let total_first = first.l1_count + first.l2_count + first.l3_count;
        let total_last = last.l1_count + last.l2_count + last.l3_count;

        match total_last.cmp(&total_first) {
            std::cmp::Ordering::Greater => GeologicalTrend::Accumulating,
            std::cmp::Ordering::Less => GeologicalTrend::Eroding, // should be rare — fragments don't die
            std::cmp::Ordering::Equal => GeologicalTrend::Stable,
        }
    }

    pub fn latest(&self) -> Option<&StrataSnapshot> {
        self.snapshots.last()
    }

    pub fn len(&self) -> usize {
        self.snapshots.len()
    }

    pub fn is_empty(&self) -> bool {
        self.snapshots.is_empty()
    }
}

/// Geological trend of a node's knowledge accumulation.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum GeologicalTrend {
    /// Growing — more fragments over time. Healthy.
    Accumulating,
    /// Stable — fragment count unchanging.
    Stable,
    /// Shrinking — fragments disappearing. Should not happen.
    /// If observed, investigate data loss.
    Eroding,
    /// Not enough data points to determine trend.
    Insufficient,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_snapshot(l1: u64, l2: u64, l3: u64) -> StrataSnapshot {
        StrataSnapshot {
            timestamp: Utc::now(),
            l1_count: l1,
            l2_count: l2,
            l3_count: l3,
            total_bytes: (l1 + l2 + l3) * 1024,
            migrations_l1_l2: 0,
            migrations_l2_l3: 0,
            promotions_to_l1: 0,
        }
    }

    #[test]
    fn test_trend_accumulating() {
        let mut history = StrataHistory::new(100);
        history.push(make_snapshot(10, 5, 0));
        history.push(make_snapshot(15, 10, 5));
        assert_eq!(history.trend(), GeologicalTrend::Accumulating);
    }

    #[test]
    fn test_trend_stable() {
        let mut history = StrataHistory::new(100);
        history.push(make_snapshot(10, 5, 5));
        history.push(make_snapshot(5, 10, 5));
        assert_eq!(history.trend(), GeologicalTrend::Stable);
    }

    #[test]
    fn test_trend_insufficient() {
        let mut history = StrataHistory::new(100);
        assert_eq!(history.trend(), GeologicalTrend::Insufficient);
        history.push(make_snapshot(10, 0, 0));
        assert_eq!(history.trend(), GeologicalTrend::Insufficient);
    }

    #[test]
    fn test_ring_buffer_eviction() {
        let mut history = StrataHistory::new(3);
        history.push(make_snapshot(1, 0, 0));
        history.push(make_snapshot(2, 0, 0));
        history.push(make_snapshot(3, 0, 0));
        history.push(make_snapshot(4, 0, 0)); // evicts first
        assert_eq!(history.len(), 3);
        assert_eq!(history.latest().unwrap().l1_count, 4);
    }
}
