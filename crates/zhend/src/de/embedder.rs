//! Local embedding model inference via ONNX Runtime.
//!
//! Like Ollama but ONLY embeddings — no full LLM inference.
//! Quantized models (int8/uint8) for compact fragments and fast inference.
//!
//! When no model is configured, Zhen still works — fragments just lack
//! embedding vectors, and De (relevance) falls back to metadata matching.

/// Embedding model wrapper.
///
/// TODO: integrate `ort` crate for ONNX Runtime inference.
/// Default model: all-MiniLM-L6-v2 (384 dims, ~22MB quantized).
pub struct Embedder {
    dims: usize,
    // session: Option<ort::Session>,
}

impl Embedder {
    /// Create embedder with given dimensions. Model loaded lazily.
    pub fn new(dims: usize) -> Self {
        Self { dims }
    }

    /// Embed raw text into a quantized uint8 vector.
    ///
    /// Returns empty vec if no model loaded (graceful degradation).
    pub fn embed_text(&self, _text: &str) -> Vec<u8> {
        // TODO: actual ONNX inference
        // For now, return empty — Zhen works without embeddings.
        tracing::trace!("embedding not yet implemented — returning empty vector");
        vec![]
    }

    /// Embed raw bytes (content-type aware in future).
    pub fn embed_bytes(&self, _bytes: &[u8], _content_type: Option<&str>) -> Vec<u8> {
        // TODO: dispatch to appropriate embedding pipeline based on content_type
        vec![]
    }

    /// Configured embedding dimensions.
    pub fn dims(&self) -> usize {
        self.dims
    }

    /// Whether a model is loaded and ready for inference.
    pub fn is_ready(&self) -> bool {
        false // TODO: check ort session
    }
}
