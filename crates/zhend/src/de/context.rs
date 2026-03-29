//! Context vector construction for De ranking.
//!
//! A context vector represents "what is relevant right now." It can be
//! built from text queries, Monad flow metadata, or accumulated over
//! a rolling window of activity.
//!
//! The context vector is the lens through which De evaluates fragments.
//! High cosine similarity between context and fragment = fragment surfaces.

use super::embedder::Embedder;

/// Accumulated context from recent activity or explicit query.
#[derive(Debug, Clone)]
pub struct ContextVector {
    /// Quantized embedding representing current context.
    pub embedding: Vec<u8>,
    /// Number of flows/queries that contributed to this context.
    pub flow_count: u64,
}

impl ContextVector {
    /// Empty context — no signal. Will match nothing via cosine similarity.
    pub fn empty() -> Self {
        Self {
            embedding: vec![],
            flow_count: 0,
        }
    }

    /// Build a context vector from text using the given embedder.
    ///
    /// This is the primary entry point for De queries: "surface fragments
    /// relevant to this text."
    pub fn from_text(embedder: &Embedder, text: &str) -> Self {
        let embedding = embedder.embed_text(text);
        Self {
            embedding,
            flow_count: 1,
        }
    }

    /// Build a context vector from raw bytes.
    pub fn from_bytes(embedder: &Embedder, bytes: &[u8], content_type: Option<&str>) -> Self {
        let embedding = embedder.embed_bytes(bytes, content_type);
        Self {
            embedding,
            flow_count: 1,
        }
    }

    /// Whether this context has any signal.
    pub fn is_empty(&self) -> bool {
        self.embedding.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_empty_context() {
        let ctx = ContextVector::empty();
        assert!(ctx.is_empty());
        assert_eq!(ctx.flow_count, 0);
    }

    #[test]
    fn test_from_text_produces_embedding() {
        let emb = Embedder::default_hash();
        let ctx = ContextVector::from_text(&emb, "networking");
        assert!(!ctx.is_empty());
        assert_eq!(ctx.embedding.len(), 384);
        assert_eq!(ctx.flow_count, 1);
    }

    #[test]
    fn test_from_text_disabled_embedder() {
        let emb = Embedder::disabled();
        let ctx = ContextVector::from_text(&emb, "anything");
        assert!(ctx.is_empty());
    }

    #[test]
    fn test_from_bytes() {
        let emb = Embedder::default_hash();
        let ctx = ContextVector::from_bytes(&emb, b"binary data", Some("application/octet-stream"));
        assert!(!ctx.is_empty());
        assert_eq!(ctx.embedding.len(), 384);
    }

    #[test]
    fn test_from_text_deterministic() {
        let emb = Embedder::default_hash();
        let ctx1 = ContextVector::from_text(&emb, "same query");
        let ctx2 = ContextVector::from_text(&emb, "same query");
        assert_eq!(ctx1.embedding, ctx2.embedding);
    }
}
