// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Forward pass implementation for transformer inference + LoRA training.
//!
//! Architecture: streaming dequantization
//! - Base model weights stay on GPU as quantized blocks
//! - Per-layer: dequantize → matmul with hipBLAS → add LoRA contribution
//! - Peak additional RAM: ~130MB (one layer's weights in f32)

use crate::gguf::GgufFile;
use crate::hip::{GpuBuffer, BlasHandle, self};
use crate::lora::LoraAdapters;
use crate::quant;

/// Dequantize a named tensor from the GGUF model to f32 on CPU.
/// Returns the f32 data ready for GPU upload.
pub fn dequantize_tensor(model: &GgufFile, name: &str) -> Option<Vec<f32>> {
    let tensor = model.tensors.iter().find(|t| t.name == name)?;
    let data = model.tensor_data(tensor);

    match tensor.tensor_type.as_str() {
        "Q5_K" => {
            Some(quant::dequantize_q5_k(data, tensor.num_elements as usize))
        }
        "F32" => {
            // Already f32 — just reinterpret bytes
            let floats: Vec<f32> = data.chunks_exact(4)
                .map(|b| f32::from_le_bytes([b[0], b[1], b[2], b[3]]))
                .collect();
            Some(floats)
        }
        "Q6_K" => {
            Some(quant::dequantize_q6_k(data, tensor.num_elements as usize))
        }
        _ => {
            eprintln!("  Unknown tensor type: {} for {}", tensor.tensor_type, name);
            None
        }
    }
}

/// Perform embedding lookup: token_ids → embedding vectors.
/// embedding_weight: (vocab_size × embed_dim) — dequantized to f32
/// token_ids: list of token indices
/// Returns: (seq_len × embed_dim) matrix
pub fn embedding_lookup(embedding_weight: &[f32], embed_dim: usize, token_ids: &[u32]) -> Vec<f32> {
    let mut output = Vec::with_capacity(token_ids.len() * embed_dim);
    for &tok in token_ids {
        let start = tok as usize * embed_dim;
        let end = start + embed_dim;
        if end <= embedding_weight.len() {
            output.extend_from_slice(&embedding_weight[start..end]);
        } else {
            // Out of vocab — zero vector
            output.extend(std::iter::repeat(0.0f32).take(embed_dim));
        }
    }
    output
}

/// RMSNorm: normalize a vector using root mean square.
/// output[i] = (input[i] / rms) * weight[i]
pub fn rmsnorm(input: &[f32], weight: &[f32], eps: f32) -> Vec<f32> {
    let n = input.len();
    let ss: f32 = input.iter().map(|x| x * x).sum::<f32>() / n as f32;
    let rms = (ss + eps).sqrt();

    input.iter().zip(weight.iter())
        .map(|(&x, &w)| (x / rms) * w)
        .collect()
}

/// SiLU activation: x * sigmoid(x)
pub fn silu(x: f32) -> f32 {
    x / (1.0 + (-x).exp())
}

/// Softmax over a slice, in-place.
pub fn softmax(logits: &mut [f32]) {
    let max = logits.iter().cloned().fold(f32::NEG_INFINITY, f32::max);
    let mut sum = 0.0f32;
    for v in logits.iter_mut() {
        *v = (*v - max).exp();
        sum += *v;
    }
    for v in logits.iter_mut() {
        *v /= sum;
    }
}

/// Cross-entropy loss for a single token prediction.
/// logits: (vocab_size,) — raw model output
/// target: the correct token index
/// Returns: -log(softmax(logits)[target])
pub fn cross_entropy_loss(logits: &[f32], target: u32) -> f32 {
    let mut probs = logits.to_vec();
    softmax(&mut probs);
    let p = probs[target as usize].max(1e-10); // Avoid log(0)
    -p.ln()
}

/// CPU matrix multiply: C = A × B
/// A: (m × k), B: (k × n), C: (m × n)
/// Used as reference and for small matrices (LoRA adapters)
pub fn matmul_cpu(a: &[f32], b: &[f32], m: usize, n: usize, k: usize) -> Vec<f32> {
    let mut c = vec![0.0f32; m * n];
    for i in 0..m {
        for j in 0..n {
            let mut sum = 0.0f32;
            for l in 0..k {
                sum += a[i * k + l] * b[l * n + j];
            }
            c[i * n + j] = sum;
        }
    }
    c
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_embedding_lookup() {
        // 4 vocab × 3 dim embedding
        let weights = vec![
            1.0, 2.0, 3.0,  // token 0
            4.0, 5.0, 6.0,  // token 1
            7.0, 8.0, 9.0,  // token 2
            10.0, 11.0, 12.0, // token 3
        ];
        let tokens = vec![2, 0, 3];
        let result = embedding_lookup(&weights, 3, &tokens);
        assert_eq!(result, vec![7.0, 8.0, 9.0, 1.0, 2.0, 3.0, 10.0, 11.0, 12.0]);
    }

    #[test]
    fn test_rmsnorm() {
        let input = vec![1.0, 2.0, 3.0, 4.0];
        let weight = vec![1.0, 1.0, 1.0, 1.0];
        let result = rmsnorm(&input, &weight, 1e-5);
        // RMS = sqrt((1+4+9+16)/4) = sqrt(7.5) ≈ 2.739
        // result[0] = 1.0 / 2.739 ≈ 0.365
        assert!((result[0] - 0.365).abs() < 0.01);
    }

    #[test]
    fn test_softmax() {
        let mut logits = vec![1.0, 2.0, 3.0];
        softmax(&mut logits);
        let sum: f32 = logits.iter().sum();
        assert!((sum - 1.0).abs() < 1e-5, "Softmax should sum to 1, got {}", sum);
        assert!(logits[2] > logits[1] && logits[1] > logits[0]);
    }

    #[test]
    fn test_cross_entropy() {
        let logits = vec![1.0, 5.0, 1.0]; // Token 1 has highest logit
        let loss_correct = cross_entropy_loss(&logits, 1); // Correct prediction
        let loss_wrong = cross_entropy_loss(&logits, 0);   // Wrong prediction
        assert!(loss_correct < loss_wrong, "Loss for correct token should be lower");
    }

    #[test]
    fn test_matmul_cpu() {
        // 2×2 matmul: [[1,2],[3,4]] × [[5,6],[7,8]] = [[19,22],[43,50]]
        let a = vec![1.0, 2.0, 3.0, 4.0];
        let b = vec![5.0, 6.0, 7.0, 8.0];
        let c = matmul_cpu(&a, &b, 2, 2, 2);
        assert!((c[0] - 19.0).abs() < 0.01);
        assert!((c[1] - 22.0).abs() < 0.01);
        assert!((c[2] - 43.0).abs() < 0.01);
        assert!((c[3] - 50.0).abs() < 0.01);
    }

    #[test]
    fn test_silu() {
        assert!((silu(0.0) - 0.0).abs() < 0.01);
        assert!(silu(1.0) > 0.5); // SiLU(1) ≈ 0.731
        assert!(silu(-1.0) < 0.0); // SiLU(-1) ≈ -0.269
    }

    #[test]
    fn test_dequantize_real_tensor() {
        let model_path = "/var/zhen/models/mistral-7b-instruct-q5_k_m.gguf";
        if !std::path::Path::new(model_path).exists() {
            println!("Model not found, skipping");
            return;
        }

        let model = GgufFile::open(model_path).expect("Failed to open model");

        // Dequantize the output_norm weight (F32 tensor, small)
        if let Some(data) = dequantize_tensor(&model, "output_norm.weight") {
            println!("output_norm.weight: {} elements, first 5: {:?}",
                data.len(), &data[..5.min(data.len())]);
            assert_eq!(data.len(), 4096, "output_norm should be 4096 dim");
            // F32 tensor should have non-zero values
            assert!(data.iter().any(|&v| v != 0.0), "output_norm should have non-zero values");
        }

        // Dequantize a Q5_K tensor (attention weight)
        if let Some(data) = dequantize_tensor(&model, "blk.0.attn_q.weight") {
            println!("blk.0.attn_q.weight: {} elements, first 5: {:?}",
                data.len(), &data[..5.min(data.len())]);
            assert!(data.len() > 0);
            // Q5_K dequantized should have non-zero values
            assert!(data.iter().any(|&v| v != 0.0), "Q5_K dequant should produce non-zero values");
        }
    }
}
