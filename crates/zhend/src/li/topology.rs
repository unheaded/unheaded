//! Traffic pattern analysis -> knowledge geography.
//!
//! Observes which fragments are accessed together, building an implicit
//! co-access graph. Over time, clusters emerge -- Li reveals itself.

use std::collections::{HashMap, HashSet, VecDeque};

use serde::{Deserialize, Serialize};

use crate::pu::fragment::FragmentId;

/// Tracks co-access patterns between fragments.
///
/// Every time a fragment is accessed, we record it. Fragments accessed
/// within the same sliding window are considered "co-accessed" -- their
/// edge weight in the co-access graph increases.
///
/// Over time, clusters emerge: fragments that are frequently accessed
/// together form communities. Li observes but never imposes.
pub struct TopologyTracker {
    /// Sliding window of recent accesses (most recent at back).
    recent_accesses: VecDeque<FragmentId>,

    /// Maximum window size. Accesses beyond this are evicted.
    window_size: usize,

    /// Sparse co-access graph: (id_a, id_b) -> access count.
    /// Keys are always ordered so that id_a <= id_b (canonical form).
    co_access: HashMap<(FragmentId, FragmentId), u64>,

    /// Total access count per fragment (for node metadata).
    access_counts: HashMap<FragmentId, u64>,
}

impl TopologyTracker {
    /// Create a new tracker with the given sliding window size.
    pub fn new(window_size: usize) -> Self {
        Self {
            recent_accesses: VecDeque::with_capacity(window_size),
            window_size,
            co_access: HashMap::new(),
            access_counts: HashMap::new(),
        }
    }

    /// Record a fragment access. This updates the sliding window and
    /// increments co-access edges between this fragment and all other
    /// fragments currently in the window.
    pub fn record_access(&mut self, id: &FragmentId) {
        // Bump per-fragment access count.
        *self.access_counts.entry(id.clone()).or_insert(0) += 1;

        // Evict oldest if window is full (before pairing, so the window
        // represents the active co-access context).
        if self.recent_accesses.len() >= self.window_size {
            self.recent_accesses.pop_front();
        }

        // Pair with every distinct fragment currently in the window.
        // Use a set to avoid double-counting when the same fragment
        // appears multiple times in the window.
        let mut seen = HashSet::new();
        for existing in self.recent_accesses.iter() {
            if existing != id && seen.insert(existing.clone()) {
                let key = canonical_pair(existing, id);
                *self.co_access.entry(key).or_insert(0) += 1;
            }
        }

        // Add to window.
        self.recent_accesses.push_back(id.clone());
    }

    /// Return the set of all fragment IDs that have been observed.
    pub fn known_fragments(&self) -> HashSet<FragmentId> {
        self.access_counts.keys().cloned().collect()
    }

    /// Return the co-access weight between two fragments, or 0 if none.
    pub fn edge_weight(&self, a: &FragmentId, b: &FragmentId) -> u64 {
        let key = canonical_pair(a, b);
        self.co_access.get(&key).copied().unwrap_or(0)
    }

    /// All edges in the co-access graph: ((id_a, id_b), weight).
    pub fn edges(&self) -> Vec<((FragmentId, FragmentId), u64)> {
        self.co_access
            .iter()
            .map(|(k, v)| (k.clone(), *v))
            .collect()
    }

    /// Run label propagation community detection on the co-access graph.
    ///
    /// Each fragment starts with its own label. On each iteration, every
    /// fragment adopts the label most common among its neighbors (weighted).
    /// Converges when labels stabilize or max_iterations is reached.
    ///
    /// Returns a map: fragment_id -> cluster_label (u64).
    pub fn detect_communities(&self, max_iterations: usize) -> HashMap<FragmentId, u64> {
        let fragments: Vec<FragmentId> = self.known_fragments().into_iter().collect();
        if fragments.is_empty() {
            return HashMap::new();
        }

        // Assign initial labels: index as label.
        let mut labels: HashMap<FragmentId, u64> = HashMap::new();
        for (i, frag) in fragments.iter().enumerate() {
            labels.insert(frag.clone(), i as u64);
        }

        // Build adjacency list for efficiency.
        let mut adjacency: HashMap<FragmentId, Vec<(FragmentId, u64)>> = HashMap::new();
        for ((a, b), weight) in &self.co_access {
            adjacency
                .entry(a.clone())
                .or_default()
                .push((b.clone(), *weight));
            adjacency
                .entry(b.clone())
                .or_default()
                .push((a.clone(), *weight));
        }

        for _iter in 0..max_iterations {
            let mut changed = false;

            for frag in &fragments {
                let neighbors = match adjacency.get(frag) {
                    Some(n) => n,
                    None => continue, // isolated node keeps its label
                };

                // Tally weighted votes for each label among neighbors.
                let mut label_votes: HashMap<u64, u64> = HashMap::new();
                for (neighbor, weight) in neighbors {
                    if let Some(&label) = labels.get(neighbor) {
                        *label_votes.entry(label).or_insert(0) += weight;
                    }
                }

                // Pick the label with highest weighted vote.
                if let Some((&best_label, _)) = label_votes.iter().max_by_key(|(_, &v)| v) {
                    let current = labels[frag];
                    if best_label != current {
                        labels.insert(frag.clone(), best_label);
                        changed = true;
                    }
                }
            }

            if !changed {
                break;
            }
        }

        labels
    }

    /// Export the topology as a JSON-serializable snapshot.
    ///
    /// Format: { nodes: [{id, cluster}], edges: [{source, target, weight}] }
    pub fn export_snapshot(&self, max_iterations: usize) -> TopologySnapshot {
        let communities = self.detect_communities(max_iterations);

        let nodes: Vec<TopologyNode> = communities
            .iter()
            .map(|(id, &cluster)| TopologyNode {
                id: id.to_hex(),
                cluster,
                access_count: self.access_counts.get(id).copied().unwrap_or(0),
            })
            .collect();

        let edges: Vec<TopologyEdge> = self
            .co_access
            .iter()
            .map(|((a, b), &weight)| TopologyEdge {
                source: a.to_hex(),
                target: b.to_hex(),
                weight,
            })
            .collect();

        TopologySnapshot { nodes, edges }
    }

    /// Number of edges in the co-access graph.
    pub fn edge_count(&self) -> usize {
        self.co_access.len()
    }

    /// Number of unique fragments observed.
    pub fn node_count(&self) -> usize {
        self.access_counts.len()
    }
}

/// Canonical key ordering: ensure (a, b) where a <= b byte-wise.
fn canonical_pair(a: &FragmentId, b: &FragmentId) -> (FragmentId, FragmentId) {
    if a.0 <= b.0 {
        (a.clone(), b.clone())
    } else {
        (b.clone(), a.clone())
    }
}

/// JSON-exportable topology snapshot for dashboard consumption.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TopologySnapshot {
    pub nodes: Vec<TopologyNode>,
    pub edges: Vec<TopologyEdge>,
}

/// A node in the topology graph.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TopologyNode {
    pub id: String,
    pub cluster: u64,
    pub access_count: u64,
}

/// An edge in the topology graph.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TopologyEdge {
    pub source: String,
    pub target: String,
    pub weight: u64,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_id(seed: &[u8]) -> FragmentId {
        FragmentId::from_payload(seed)
    }

    #[test]
    fn test_co_access_tracking_records_pairs() {
        let mut tracker = TopologyTracker::new(10);

        let a = make_id(b"fragment_a");
        let b = make_id(b"fragment_b");
        let c = make_id(b"fragment_c");

        // Access a, then b — they should be co-accessed.
        tracker.record_access(&a);
        tracker.record_access(&b);

        assert_eq!(tracker.edge_weight(&a, &b), 1);
        assert_eq!(tracker.edge_weight(&a, &c), 0); // c not seen yet

        // Access a again — co-access with b increases (b still in window).
        tracker.record_access(&a);
        assert_eq!(tracker.edge_weight(&a, &b), 2);

        // Access c — co-accessed with both a and b in window.
        tracker.record_access(&c);
        assert_eq!(tracker.edge_weight(&a, &c), 1);
        assert_eq!(tracker.edge_weight(&b, &c), 1);
    }

    #[test]
    fn test_sliding_window_eviction() {
        // Window of 3: only last 3 accesses form co-access pairs.
        let mut tracker = TopologyTracker::new(3);

        let a = make_id(b"alpha");
        let b = make_id(b"beta");
        let c = make_id(b"gamma");
        let d = make_id(b"delta");

        tracker.record_access(&a);
        tracker.record_access(&b);
        tracker.record_access(&c);
        // Window: [a, b, c]

        // Now access d — a gets evicted from window.
        tracker.record_access(&d);
        // Window: [b, c, d]

        // d should be co-accessed with b and c (still in window),
        // but the a-d pair never formed because a was evicted before d arrived.
        assert_eq!(tracker.edge_weight(&b, &d), 1);
        assert_eq!(tracker.edge_weight(&c, &d), 1);
        // a-d never co-occurred in the same window state:
        assert_eq!(tracker.edge_weight(&a, &d), 0);
    }

    #[test]
    fn test_community_detection_finds_clusters() {
        let mut tracker = TopologyTracker::new(20);

        // Cluster 1: fragments a, b, c accessed together heavily.
        let a = make_id(b"cluster1_a");
        let b = make_id(b"cluster1_b");
        let c = make_id(b"cluster1_c");

        // Cluster 2: fragments x, y, z accessed together heavily.
        let x = make_id(b"cluster2_x");
        let y = make_id(b"cluster2_y");
        let z = make_id(b"cluster2_z");

        // Build strong co-access within cluster 1.
        for _ in 0..10 {
            tracker.record_access(&a);
            tracker.record_access(&b);
            tracker.record_access(&c);
            // Clear window between clusters to avoid cross-cluster edges.
            tracker.recent_accesses.clear();
        }

        // Build strong co-access within cluster 2.
        for _ in 0..10 {
            tracker.record_access(&x);
            tracker.record_access(&y);
            tracker.record_access(&z);
            tracker.recent_accesses.clear();
        }

        let communities = tracker.detect_communities(20);

        // Verify: fragments within a cluster share a label.
        assert_eq!(communities[&a], communities[&b]);
        assert_eq!(communities[&b], communities[&c]);
        assert_eq!(communities[&x], communities[&y]);
        assert_eq!(communities[&y], communities[&z]);

        // Verify: the two clusters have different labels.
        assert_ne!(communities[&a], communities[&x]);
    }

    #[test]
    fn test_json_export_produces_valid_output() {
        let mut tracker = TopologyTracker::new(10);

        let a = make_id(b"json_a");
        let b = make_id(b"json_b");

        tracker.record_access(&a);
        tracker.record_access(&b);

        let snapshot = tracker.export_snapshot(10);

        // Serialize to JSON — must not panic.
        let json = serde_json::to_string_pretty(&snapshot).expect("JSON serialization must work");

        // Deserialize back — round-trip.
        let parsed: TopologySnapshot =
            serde_json::from_str(&json).expect("JSON deserialization must work");

        assert_eq!(parsed.nodes.len(), 2);
        assert_eq!(parsed.edges.len(), 1);
        assert_eq!(parsed.edges[0].weight, 1);
    }

    #[test]
    fn test_empty_tracker() {
        let tracker = TopologyTracker::new(10);
        assert_eq!(tracker.node_count(), 0);
        assert_eq!(tracker.edge_count(), 0);

        let communities = tracker.detect_communities(10);
        assert!(communities.is_empty());

        let snapshot = tracker.export_snapshot(10);
        assert!(snapshot.nodes.is_empty());
        assert!(snapshot.edges.is_empty());
    }

    #[test]
    fn test_single_fragment_no_edges() {
        let mut tracker = TopologyTracker::new(10);
        let a = make_id(b"alone");

        tracker.record_access(&a);
        tracker.record_access(&a);
        tracker.record_access(&a);

        assert_eq!(tracker.node_count(), 1);
        assert_eq!(tracker.edge_count(), 0); // self-edges excluded
    }

    #[test]
    fn test_access_count_tracking() {
        let mut tracker = TopologyTracker::new(10);
        let a = make_id(b"counted");

        tracker.record_access(&a);
        tracker.record_access(&a);
        tracker.record_access(&a);

        let snapshot = tracker.export_snapshot(10);
        assert_eq!(snapshot.nodes.len(), 1);
        assert_eq!(snapshot.nodes[0].access_count, 3);
    }

    #[test]
    fn test_integration_access_patterns_create_visible_clusters() {
        // Simulate a realistic access pattern: two topics of interest.
        let mut tracker = TopologyTracker::new(5);

        // "Topic A" fragments.
        let a1 = make_id(b"rust_ownership");
        let a2 = make_id(b"rust_borrowing");
        let a3 = make_id(b"rust_lifetimes");

        // "Topic B" fragments.
        let b1 = make_id(b"docker_compose");
        let b2 = make_id(b"docker_networking");
        let b3 = make_id(b"docker_volumes");

        // User studies Rust topics together, then Docker topics together.
        for _ in 0..15 {
            // Rust session
            tracker.record_access(&a1);
            tracker.record_access(&a2);
            tracker.record_access(&a3);
            tracker.recent_accesses.clear();

            // Docker session
            tracker.record_access(&b1);
            tracker.record_access(&b2);
            tracker.record_access(&b3);
            tracker.recent_accesses.clear();
        }

        let communities = tracker.detect_communities(50);

        // All Rust fragments cluster together.
        assert_eq!(communities[&a1], communities[&a2]);
        assert_eq!(communities[&a2], communities[&a3]);

        // All Docker fragments cluster together.
        assert_eq!(communities[&b1], communities[&b2]);
        assert_eq!(communities[&b2], communities[&b3]);

        // Rust cluster != Docker cluster.
        assert_ne!(communities[&a1], communities[&b1]);

        // Export and verify structure.
        let snapshot = tracker.export_snapshot(50);
        assert_eq!(snapshot.nodes.len(), 6);

        // Should have intra-cluster edges: C(3,2) = 3 edges per cluster = 6 total.
        assert_eq!(snapshot.edges.len(), 6);

        // Verify JSON round-trip.
        let json = serde_json::to_string(&snapshot).unwrap();
        let _: TopologySnapshot = serde_json::from_str(&json).unwrap();
    }

    #[test]
    fn test_canonical_pair_ordering() {
        let a = make_id(b"aaa");
        let b = make_id(b"bbb");

        let pair1 = canonical_pair(&a, &b);
        let pair2 = canonical_pair(&b, &a);

        assert_eq!(pair1, pair2, "canonical pair must be order-independent");
    }
}
