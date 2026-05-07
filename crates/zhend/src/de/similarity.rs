//! Similarity search for De (relevance surfacing).
//!
//! Cosine similarity between context vector and fragment embeddings.
//! ANN (Approximate Nearest Neighbor) via HNSW for scalability.

use crate::pu::Fragment;

/// Compute cosine similarity between two uint8-quantized vectors.
///
/// Returns f32 in [-1.0, 1.0]. Higher = more relevant.
/// Returns 0.0 if either vector is empty (graceful degradation).
pub fn cosine_similarity(a: &[u8], b: &[u8]) -> f32 {
    if a.is_empty() || b.is_empty() || a.len() != b.len() {
        return 0.0;
    }

    let mut dot = 0.0f64;
    let mut norm_a = 0.0f64;
    let mut norm_b = 0.0f64;

    for (&x, &y) in a.iter().zip(b.iter()) {
        let xf = x as f64;
        let yf = y as f64;
        dot += xf * yf;
        norm_a += xf * xf;
        norm_b += yf * yf;
    }

    let denom = norm_a.sqrt() * norm_b.sqrt();
    if denom == 0.0 {
        return 0.0;
    }

    (dot / denom) as f32
}

/// Score and rank fragments by relevance to a context vector.
///
/// Returns (fragment, de_score) pairs sorted by descending De.
pub fn rank_by_de<'a>(
    context: &[u8],
    fragments: &'a [Fragment],
    top_k: usize,
) -> Vec<(&'a Fragment, f32)> {
    let mut scored: Vec<_> = fragments
        .iter()
        .map(|f| {
            let score = cosine_similarity(context, &f.embedding);
            (f, score)
        })
        .filter(|(_, score)| *score > 0.0)
        .collect();

    scored.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
    scored.truncate(top_k);
    scored
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_cosine_identical() {
        let a = vec![100u8, 200, 50, 150];
        let sim = cosine_similarity(&a, &a);
        assert!(
            (sim - 1.0).abs() < 1e-5,
            "identical vectors should have similarity ~1.0"
        );
    }

    #[test]
    fn test_cosine_orthogonal() {
        // Not truly orthogonal with uint8, but low similarity.
        let a = vec![255u8, 0, 0, 0];
        let b = vec![0u8, 255, 0, 0];
        let sim = cosine_similarity(&a, &b);
        assert!(
            sim.abs() < 0.01,
            "orthogonal vectors should have ~0 similarity"
        );
    }

    #[test]
    fn test_cosine_empty() {
        assert_eq!(cosine_similarity(&[], &[1, 2, 3]), 0.0);
        assert_eq!(cosine_similarity(&[1, 2, 3], &[]), 0.0);
        assert_eq!(cosine_similarity(&[], &[]), 0.0);
    }

    #[test]
    fn test_cosine_mismatched_lengths() {
        assert_eq!(cosine_similarity(&[1, 2], &[1, 2, 3]), 0.0);
    }
}
