//! Local embedding inference — BLAKE3 hash-based fallback.
//!
//! When ONNX Runtime is unavailable (no native library), Zhen uses a
//! deterministic hash-based embedder: BLAKE3 keyed hash expanded to
//! 384 dimensions (uint8). This isn't semantic — "cat" and "kitten"
//! won't be close — but it enables the full De pipeline end-to-end.
//!
//! The hash embedder has one critical property: identical text always
//! produces identical vectors. Different text produces different vectors
//! (with cryptographic collision resistance).
//!
//! When ONNX becomes available, swap `HashEmbedder` for `OnnxEmbedder`
//! behind the same `Embedder` trait. The rest of De doesn't care.

/// Embedding dimensions for hash-based fallback.
/// Matches all-MiniLM-L6-v2 (384 dims) so the pipeline is drop-in
/// compatible when real semantic embeddings arrive.
pub const DEFAULT_DIMS: usize = 384;

/// Embedding model wrapper with graceful degradation.
///
/// - If an ONNX model path is provided and runtime is available: semantic embeddings.
/// - Otherwise: deterministic BLAKE3 hash-based embeddings (not semantic, but
///   enables full pipeline testing and identical-content matching).
pub struct Embedder {
    dims: usize,
    mode: EmbedderMode,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EmbedderMode {
    /// BLAKE3 hash expansion — deterministic, not semantic.
    Hash,
    /// No embedding at all — returns empty vectors.
    Disabled,
    // Future: Onnx { session: ... }
}

impl Embedder {
    /// Create embedder with given dimensions. Defaults to hash mode.
    ///
    /// Backward-compatible constructor — equivalent to `Embedder::hash(dims)`.
    pub fn new(dims: usize) -> Self {
        Self::hash(dims)
    }

    /// Create a hash-based embedder (BLAKE3 fallback).
    pub fn hash(dims: usize) -> Self {
        Self {
            dims,
            mode: EmbedderMode::Hash,
        }
    }

    /// Create a disabled embedder (returns empty vectors).
    pub fn disabled() -> Self {
        Self {
            dims: 0,
            mode: EmbedderMode::Disabled,
        }
    }

    /// Create embedder with default dimensions (384).
    pub fn default_hash() -> Self {
        Self::hash(DEFAULT_DIMS)
    }

    /// Embed raw text into a quantized uint8 vector.
    ///
    /// Hash mode: BLAKE3 keyed hash expanded to `dims` bytes.
    /// Disabled mode: returns empty vec.
    pub fn embed_text(&self, text: &str) -> Vec<u8> {
        match self.mode {
            EmbedderMode::Hash => self.blake3_embed(text.as_bytes()),
            EmbedderMode::Disabled => vec![],
        }
    }

    /// Embed raw bytes (content-type aware in future).
    pub fn embed_bytes(&self, bytes: &[u8], _content_type: Option<&str>) -> Vec<u8> {
        match self.mode {
            EmbedderMode::Hash => self.blake3_embed(bytes),
            EmbedderMode::Disabled => vec![],
        }
    }

    /// BLAKE3 hash expansion to `dims` uint8 values.
    ///
    /// Uses BLAKE3's XOF (extendable output function) to produce exactly
    /// `dims` bytes from any input. This is deterministic and collision-resistant.
    fn blake3_embed(&self, data: &[u8]) -> Vec<u8> {
        let mut hasher = blake3::Hasher::new();
        hasher.update(data);
        let mut output = vec![0u8; self.dims];
        let mut reader = hasher.finalize_xof();
        reader.fill(&mut output);
        output
    }

    /// Configured embedding dimensions.
    pub fn dims(&self) -> usize {
        self.dims
    }

    /// Whether this embedder is ready to produce non-empty vectors.
    pub fn is_ready(&self) -> bool {
        self.mode != EmbedderMode::Disabled
    }

    /// Current embedding mode.
    pub fn mode(&self) -> EmbedderMode {
        self.mode
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_hash_embedder_produces_correct_dims() {
        let emb = Embedder::hash(384);
        let vec = emb.embed_text("hello world");
        assert_eq!(vec.len(), 384);
    }

    #[test]
    fn test_hash_embedder_deterministic() {
        let emb = Embedder::hash(384);
        let v1 = emb.embed_text("networking protocols");
        let v2 = emb.embed_text("networking protocols");
        assert_eq!(v1, v2, "same input must produce same embedding");
    }

    #[test]
    fn test_hash_embedder_different_inputs_differ() {
        let emb = Embedder::hash(384);
        let v1 = emb.embed_text("networking");
        let v2 = emb.embed_text("cooking");
        assert_ne!(v1, v2, "different inputs must produce different embeddings");
    }

    #[test]
    fn test_hash_embedder_non_empty() {
        let emb = Embedder::hash(384);
        let vec = emb.embed_text("test");
        assert!(!vec.is_empty());
        // Verify not all zeros (astronomically unlikely with BLAKE3).
        assert!(vec.iter().any(|&b| b != 0));
    }

    #[test]
    fn test_disabled_embedder() {
        let emb = Embedder::disabled();
        assert!(!emb.is_ready());
        assert!(emb.embed_text("anything").is_empty());
    }

    #[test]
    fn test_embed_bytes() {
        let emb = Embedder::hash(384);
        let vec = emb.embed_bytes(b"raw binary data", None);
        assert_eq!(vec.len(), 384);
    }

    #[test]
    fn test_embed_bytes_matches_text() {
        let emb = Embedder::hash(384);
        let from_text = emb.embed_text("hello");
        let from_bytes = emb.embed_bytes(b"hello", None);
        assert_eq!(
            from_text, from_bytes,
            "text and bytes of same content must match"
        );
    }

    #[test]
    fn test_default_hash() {
        let emb = Embedder::default_hash();
        assert_eq!(emb.dims(), DEFAULT_DIMS);
        assert!(emb.is_ready());
    }

    #[test]
    fn test_empty_input() {
        let emb = Embedder::hash(384);
        let vec = emb.embed_text("");
        assert_eq!(vec.len(), 384);
        // Even empty input produces a deterministic non-trivial hash.
        assert!(vec.iter().any(|&b| b != 0));
    }
}
