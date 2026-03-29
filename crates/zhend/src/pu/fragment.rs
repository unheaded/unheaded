//! Core fragment type — the atom of Zhen.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::fmt;

/// BLAKE3 hash identifying a fragment. The fragment IS its hash.
/// No external ID. No auto-increment. No UUID. Content is identity.
#[derive(Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct FragmentId(pub [u8; 32]);

impl FragmentId {
    /// Compute the canonical FragmentId from raw payload bytes.
    pub fn from_payload(payload: &[u8]) -> Self {
        let hash = blake3::hash(payload);
        Self(*hash.as_bytes())
    }

    /// Hex representation for logging/display.
    pub fn to_hex(&self) -> String {
        hex::encode(self.0)
    }

    /// First 8 hex chars — short display for logs.
    pub fn short(&self) -> String {
        self.to_hex()[..8].to_string()
    }
}

impl fmt::Debug for FragmentId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "Pu({}…)", self.short())
    }
}

impl fmt::Display for FragmentId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.short())
    }
}

/// Which storage tier a fragment currently occupies.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum Tier {
    /// L1 — hot, mmap'd, sub-millisecond access.
    Hot,
    /// L2 — warm, sled embedded KV, single-digit ms.
    Warm,
    /// L3 — cold, append-only archive, seconds to minutes.
    Cold,
}

/// A single Zhen fragment — the Uncarved Block.
///
/// Immutable once created. The `id` is derived from `payload` via BLAKE3.
/// The `embedding` is a lossy projection for similarity search — original
/// content cannot be reconstructed from it (privacy by math).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Fragment {
    /// Content-addressed identity. BLAKE3(payload).
    pub id: FragmentId,

    /// Raw knowledge payload. Could be text, binary, protobuf, anything.
    /// Zhen doesn't interpret — it carries. Meaning emerges through De.
    pub payload: Vec<u8>,

    /// Embedding vector for similarity search. Quantized to u8 for
    /// compact storage and gossip transport. Empty if no embedding
    /// model is configured (Zhen still works — just without De).
    pub embedding: Vec<u8>,

    /// When this fragment was first observed by this node.
    pub first_seen: DateTime<Utc>,

    /// When this fragment was last accessed (read/surfaced via De).
    /// Drives tier migration: no access → sedimentation.
    pub last_accessed: DateTime<Utc>,

    /// How many times this fragment has been accessed on this node.
    /// High access_count + recent last_accessed = high local De.
    pub access_count: u64,

    /// Current storage tier.
    pub tier: Tier,

    /// Optional content-type hint (MIME-like). Not enforced — advisory.
    /// e.g., "text/plain", "application/protobuf", "application/octet-stream"
    pub content_type: Option<String>,

    /// Optional origin metadata. Which peer first gossiped this fragment.
    /// Not authenticated — informational only. Could be spoofed.
    pub origin_peer: Option<String>,

    /// Number of hops this fragment has traveled via gossip (Qi).
    /// Incremented at each peer. Not a TTL — fragments never die.
    /// Used for Li (topology observation) only.
    pub hop_count: u32,

    /// Optional sealed (PQ-encrypted) payload envelope.
    ///
    /// When present, `payload` contains the plaintext (if we're the owner
    /// or recipient who has unsealed) or is empty (if we're a relay node
    /// carrying sealed data we can't read).
    ///
    /// The FragmentId is ALWAYS computed from the PLAINTEXT, not the sealed
    /// envelope. Content identity transcends encryption.
    ///
    /// Sacred Law: "That which remembers forever must be armored against forever."
    /// Classical-only encryption is FORBIDDEN. See crypto::envelope.
    pub sealed_payload: Option<Vec<u8>>,
}

impl Fragment {
    /// Create a new fragment from raw payload.
    ///
    /// Computes BLAKE3 hash automatically. Embedding must be provided
    /// separately (or empty if no model configured).
    pub fn new(payload: Vec<u8>, embedding: Vec<u8>, content_type: Option<String>) -> Self {
        let id = FragmentId::from_payload(&payload);
        let now = Utc::now();
        Self {
            id,
            payload,
            embedding,
            first_seen: now,
            last_accessed: now,
            access_count: 0,
            tier: Tier::Hot,
            content_type,
            origin_peer: None,
            hop_count: 0,
            sealed_payload: None,
        }
    }

    /// Verify fragment integrity. The id MUST equal BLAKE3(payload).
    /// If this returns false, the fragment has been tampered with.
    /// ALL inputs are hostile.
    pub fn verify(&self) -> bool {
        let expected = FragmentId::from_payload(&self.payload);
        self.id == expected
    }

    /// Record an access event. Resets tier to Hot, bumps counters.
    pub fn touch(&mut self) {
        self.last_accessed = Utc::now();
        self.access_count = self.access_count.saturating_add(1);
        self.tier = Tier::Hot;
    }

    /// Payload size in bytes.
    pub fn payload_len(&self) -> usize {
        self.payload.len()
    }

    /// Total in-memory size estimate (for capacity planning).
    pub fn estimated_size(&self) -> usize {
        32 // id
        + self.payload.len()
        + self.embedding.len()
        + 8 + 8 + 8 // timestamps + count
        + 1 // tier
        + self.content_type.as_ref().map_or(0, |s| s.len())
        + self.origin_peer.as_ref().map_or(0, |s| s.len())
        + 4 // hop_count
        + self.sealed_payload.as_ref().map_or(0, |s| s.len())
    }

    /// Whether this fragment carries an encrypted payload.
    pub fn is_sealed(&self) -> bool {
        self.sealed_payload.is_some()
    }

    /// Whether this fragment has readable plaintext.
    /// A sealed fragment from a relay node may have empty payload.
    pub fn has_plaintext(&self) -> bool {
        !self.payload.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_fragment_creation_and_verify() {
        let payload = b"the dao that can be named is not the eternal dao".to_vec();
        let frag = Fragment::new(payload.clone(), vec![], None);

        assert!(frag.verify(), "fresh fragment must verify");
        assert_eq!(frag.tier, Tier::Hot);
        assert_eq!(frag.access_count, 0);
        assert_eq!(frag.hop_count, 0);
    }

    #[test]
    fn test_fragment_tamper_detection() {
        let payload = b"original truth".to_vec();
        let mut frag = Fragment::new(payload, vec![], None);

        // Tamper with payload — id no longer matches.
        frag.payload = b"corrupted lies".to_vec();
        assert!(!frag.verify(), "tampered fragment must fail verify");
    }

    #[test]
    fn test_fragment_touch_resets_tier() {
        let mut frag = Fragment::new(b"knowledge".to_vec(), vec![], None);
        frag.tier = Tier::Cold;
        frag.touch();
        assert_eq!(frag.tier, Tier::Hot);
        assert_eq!(frag.access_count, 1);
    }

    #[test]
    fn test_fragment_id_deterministic() {
        let payload = b"same input same hash".to_vec();
        let id1 = FragmentId::from_payload(&payload);
        let id2 = FragmentId::from_payload(&payload);
        assert_eq!(id1, id2);
    }

    #[test]
    fn test_fragment_id_short() {
        let payload = b"test".to_vec();
        let id = FragmentId::from_payload(&payload);
        assert_eq!(id.short().len(), 8);
    }

    #[test]
    fn test_empty_payload() {
        let frag = Fragment::new(vec![], vec![], None);
        assert!(frag.verify());
        assert_eq!(frag.payload_len(), 0);
    }
}
