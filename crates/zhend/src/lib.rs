#![deny(unsafe_code)]
//! # Zhen (真) — Layer 0 Anti-Fragile Knowledge Substrate
//!
//! The Dao beneath the Protocol. Knowledge that cannot be burned,
//! cannot be seized, cannot lose quorum — because it has no building,
//! no root, and no consensus requirement.
//!
//! ## Mythological Foundation: Pillar 4 — The Dao
//!
//! | Concept | Role |
//! |---------|------|
//! | Zhen (真) | Layer 0 itself — original truth before naming |
//! | Pu (朴) | Fragment format — uncarved blocks |
//! | Qi (氣) | Gossip propagation — vital breath |
//! | Wu Wei (無為) | No-index design — knowledge surfaces through non-action |
//! | De (德) | Relevance gravity — context attracts |
//! | Jing (經) | Deep strata — ancient knowledge |
//! | Li (理) | Emergent structure — pattern in the wood grain |
//!
//! ## Architecture
//!
//! ```text
//! ┌─────────────────────────────────────────┐
//! │              zhend daemon               │
//! ├──────┬──────┬──────┬──────┬──────┬──────┤
//! │  pu  │  qi  │  de  │  li  │ jing │ api  │
//! │ frag │ goss │ rel  │ topo │ deep │ grpc │
//! │ store│ prop │ emb  │ obs  │ arch │ quic │
//! ├──────┴──────┴──────┴──────┴──────┴──────┤
//! │         crypto (PQ-hardened)             │
//! │   ML-KEM-768 + X25519 | ML-DSA-65       │
//! │   AES-256-GCM | HKDF-SHA256 | zeroize   │
//! ├─────────────────────────────────────────┤
//! │              monad bridge               │
//! ├─────────────────────────────────────────┤
//! │        IPv6 HbH / Monad transport       │
//! └─────────────────────────────────────────┘
//! ```
//!
//! GPL-3.0-only — The Library cannot be enclosed.

pub mod pu;
pub mod qi;
pub mod de;
pub mod li;
pub mod jing;
pub mod api;
pub mod monad;
pub mod crypto;

/// Generated protobuf / tonic types from `proto/zhen.proto`.
pub mod proto {
    pub mod zhen {
        pub mod v1 {
            tonic::include_proto!("zhen.v1");
        }
    }
}

use thiserror::Error;

/// Unified error type for all Zhen operations.
#[derive(Error, Debug)]
pub enum ZhenError {
    #[error("fragment integrity failure: blake3 mismatch for {hash}")]
    IntegrityFailure { hash: String },

    #[error("storage error: {0}")]
    Storage(#[from] sled::Error),

    #[error("serialization error: {0}")]
    Serialization(String),

    #[error("gossip error: {0}")]
    Gossip(String),

    #[error("embedding error: {0}")]
    Embedding(String),

    #[error("transport error: {0}")]
    Transport(String),

    #[error("io error: {0}")]
    Io(#[from] std::io::Error),
}

pub type ZhenResult<T> = Result<T, ZhenError>;

/// Global configuration for a zhend instance.
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct ZhenConfig {
    /// Data directory for all storage tiers.
    pub data_dir: std::path::PathBuf,

    /// gRPC listen address.
    pub grpc_addr: String,

    /// QUIC listen address.
    pub quic_addr: String,

    /// Gossip listen address (UDP).
    pub gossip_addr: String,

    /// Seed peers for initial gossip join.
    pub seed_peers: Vec<String>,

    /// L1 → L2 migration threshold (seconds since last access).
    pub l1_to_l2_secs: u64,

    /// L2 → L3 migration threshold (seconds since last access).
    pub l2_to_l3_secs: u64,

    /// Embedding model path (ONNX).
    pub embedding_model: Option<std::path::PathBuf>,

    /// Embedding vector dimensions.
    pub embedding_dims: usize,

    /// Maximum gossip fanout per cycle.
    pub gossip_fanout: usize,

    /// Gossip cycle interval (milliseconds).
    pub gossip_interval_ms: u64,
}

impl Default for ZhenConfig {
    fn default() -> Self {
        Self {
            data_dir: std::path::PathBuf::from("/var/lib/zhend"),
            grpc_addr: "[::1]:7300".into(),
            quic_addr: "[::1]:7301".into(),
            gossip_addr: "[::]:7302".into(),
            seed_peers: vec![],
            l1_to_l2_secs: 3600,       // 1 hour
            l2_to_l3_secs: 86400 * 7,  // 1 week
            embedding_model: None,
            embedding_dims: 384,        // MiniLM default
            gossip_fanout: 3,
            gossip_interval_ms: 1000,
        }
    }
}
