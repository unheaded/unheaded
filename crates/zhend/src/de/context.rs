//! Context vector construction from active Monad flow metadata.
//!
//! When a packet flow traverses the Kingdom, its Monad headers carry
//! service_id, action_id, and other metadata. Zhen constructs a
//! "context vector" from this flow — the lens through which De
//! (relevance) evaluates fragments.

/// Accumulated context from recent Monad flow activity.
#[derive(Debug, Clone)]
pub struct ContextVector {
    /// Quantized embedding representing current context.
    pub embedding: Vec<u8>,
    /// Number of flows that contributed to this context.
    pub flow_count: u64,
}

impl ContextVector {
    pub fn empty() -> Self {
        Self {
            embedding: vec![],
            flow_count: 0,
        }
    }

    /// Whether this context has any signal.
    pub fn is_empty(&self) -> bool {
        self.embedding.is_empty()
    }
}

// TODO: build context from Monad header metadata
// TODO: rolling window of recent flow embeddings averaged
// TODO: exponential decay weighting for temporal relevance
