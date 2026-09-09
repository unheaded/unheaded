// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Forward pass implementation for transformer inference + LoRA training.
//!
//! Architecture: streaming dequantization
//! - Base model weights stay on GPU as quantized blocks
//! - Per-layer: dequantize → matmul with hipBLAS → add LoRA contribution
//! - Peak additional RAM: ~130MB (one layer's weights in f32)

use crate::gguf::GgufFile;
use crate::quant;

/// Dequantize a named tensor from the GGUF model to f32 on CPU.
/// Returns the f32 data ready for GPU upload.
pub fn dequantize_tensor(model: &GgufFile, name: &str) -> Option<Vec<f32>> {
    let tensor = model.tensors.iter().find(|t| t.name == name)?;
    let data = model.tensor_data(tensor);

    match tensor.tensor_type.as_str() {
        "Q5_K" => Some(quant::dequantize_q5_k(data, tensor.num_elements as usize)),
        "F32" => {
            // Already f32 — just reinterpret bytes
            let floats: Vec<f32> = data
                .chunks_exact(4)
                .map(|b| f32::from_le_bytes([b[0], b[1], b[2], b[3]]))
                .collect();
            Some(floats)
        }
        "Q6_K" => Some(quant::dequantize_q6_k(data, tensor.num_elements as usize)),
        "BF16" => {
            // Gemma 4 base weights ship as bf16 (318 of 601 tensors in E2B-it).
            Some(quant::dequantize_bf16(data, tensor.num_elements as usize))
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
            output.extend(std::iter::repeat_n(0.0f32, embed_dim));
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

    input
        .iter()
        .zip(weight.iter())
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

/// FFN forward pass: SwiGLU (gate * silu(up)) then down projection.
/// gate_weight: (ffn_dim × embed_dim), up_weight: (ffn_dim × embed_dim), down_weight: (embed_dim × ffn_dim)
/// Input: (embed_dim,), Output: (embed_dim,)
pub fn ffn_forward(
    input: &[f32],
    gate_w: &[f32],
    up_w: &[f32],
    down_w: &[f32],
    n_embd: usize,
    n_ff: usize,
) -> Vec<f32> {
    let dims = n_embd.min(512); // Partial dims — 512 for reasonable quality/speed
    let ff_dims = n_ff.min(512);

    // Gate projection: gate = gate_w × input
    let mut gate = vec![0.0f32; ff_dims];
    for i in 0..ff_dims {
        let mut sum = 0.0f32;
        for j in 0..dims {
            sum += gate_w[i * n_embd + j] * input[j];
        }
        gate[i] = silu(sum);
    }

    // Up projection: up = up_w × input
    let mut up = vec![0.0f32; ff_dims];
    for i in 0..ff_dims {
        let mut sum = 0.0f32;
        for j in 0..dims {
            sum += up_w[i * n_embd + j] * input[j];
        }
        up[i] = sum;
    }

    // Element-wise: hidden = gate * up (SwiGLU)
    for i in 0..ff_dims {
        gate[i] *= up[i];
    }

    // Down projection: output = down_w × hidden
    let mut output = vec![0.0f32; n_embd];
    for i in 0..dims {
        let mut sum = 0.0f32;
        for j in 0..ff_dims {
            sum += down_w[i * n_ff + j] * gate[j];
        }
        output[i] = sum;
    }

    output
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

// =============================================================================
// WAVE10F Phase 1 — real attention forward (vanilla GQA).
// Math derivations: crates/zhenai-forge/notes/phase1-attention-math.md
// Backward counterparts in backward.rs; numerical-gradient tests in both.
// =============================================================================

/// Precompute RoPE cos/sin tables for a given sequence length and head dim.
///
/// freqs[s, d/2] = (cos(s * theta_d), sin(s * theta_d))
/// where theta_d = 1.0 / base^(2*d / head_dim)
///
/// Returns (cos_table, sin_table), each of shape [seq_len, head_dim/2].
pub fn rope_freqs(seq_len: usize, head_dim: usize, base: f32) -> (Vec<f32>, Vec<f32>) {
    let half = head_dim / 2;
    let mut cos_table = vec![0.0f32; seq_len * half];
    let mut sin_table = vec![0.0f32; seq_len * half];
    for d in 0..half {
        let theta = 1.0f32 / base.powf(2.0 * d as f32 / head_dim as f32);
        for s in 0..seq_len {
            let angle = s as f32 * theta;
            cos_table[s * half + d] = angle.cos();
            sin_table[s * half + d] = angle.sin();
        }
    }
    (cos_table, sin_table)
}

/// Apply RoPE rotation in-place along (even, odd) dim pairs.
///
/// Input shape: [seq_len, head_dim] for a single head; caller iterates heads.
/// freqs_cos, freqs_sin: shape [seq_len, head_dim/2]
///
/// Rotation: [x_even', x_odd'] = [x_even * cos - x_odd * sin,
///                                x_even * sin + x_odd * cos]
pub fn rope_apply(
    x: &[f32],
    freqs_cos: &[f32],
    freqs_sin: &[f32],
    seq_len: usize,
    head_dim: usize,
) -> Vec<f32> {
    let half = head_dim / 2;
    debug_assert_eq!(x.len(), seq_len * head_dim);
    debug_assert_eq!(freqs_cos.len(), seq_len * half);
    debug_assert_eq!(freqs_sin.len(), seq_len * half);

    let mut out = vec![0.0f32; seq_len * head_dim];
    for s in 0..seq_len {
        for d in 0..half {
            let cos = freqs_cos[s * half + d];
            let sin = freqs_sin[s * half + d];
            let xe = x[s * head_dim + 2 * d];
            let xo = x[s * head_dim + 2 * d + 1];
            out[s * head_dim + 2 * d] = xe * cos - xo * sin;
            out[s * head_dim + 2 * d + 1] = xe * sin + xo * cos;
        }
    }
    out
}

/// Mask type for attention.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AttnMask {
    /// Standard causal mask: position i can attend to positions [0, i].
    Causal,
    /// Sliding-window causal mask: position i can attend to positions
    /// [max(0, i - window + 1), i].
    SlidingWindow(usize),
}

impl AttnMask {
    /// Returns true if query position `i` is allowed to attend to key position `j`.
    pub fn allows(&self, i: usize, j: usize) -> bool {
        match self {
            AttnMask::Causal => j <= i,
            AttnMask::SlidingWindow(w) => j <= i && i - j < *w,
        }
    }
}

/// Vanilla GQA scaled-dot-product attention forward (CPU reference).
///
/// Inputs:
///   q_rot: post-RoPE Q, shape [seq_len, n_heads, head_dim]
///   k_rot: post-RoPE K, shape [seq_len, n_kv_heads, head_dim]
///   v: V, shape [seq_len, n_kv_heads, head_dim]
///   n_heads, n_kv_heads, head_dim, seq_len: dims
///   mask: causal or sliding-window
///
/// Returns (output, attn_cache) where:
///   output: shape [seq_len, n_heads, head_dim] — attention output (before O proj)
///   attn_cache: shape [n_heads, seq_len, seq_len] — softmax weights, needed by backward
///
/// Mistral-7B example: n_heads=32, n_kv_heads=8, head_dim=128.
/// GQA expansion happens inside (each KV head consumed by 4 query heads).
#[allow(clippy::too_many_arguments)] // numerical/GPU kernel signature — see crate note
pub fn attention_forward(
    q_rot: &[f32],
    k_rot: &[f32],
    v: &[f32],
    n_heads: usize,
    n_kv_heads: usize,
    head_dim: usize,
    seq_len: usize,
    mask: AttnMask,
) -> (Vec<f32>, Vec<f32>) {
    debug_assert_eq!(q_rot.len(), seq_len * n_heads * head_dim);
    debug_assert_eq!(k_rot.len(), seq_len * n_kv_heads * head_dim);
    debug_assert_eq!(v.len(), seq_len * n_kv_heads * head_dim);
    debug_assert_eq!(n_heads % n_kv_heads, 0);

    let group_size = n_heads / n_kv_heads;
    let scale = 1.0f32 / (head_dim as f32).sqrt();

    let mut output = vec![0.0f32; seq_len * n_heads * head_dim];
    let mut attn_cache = vec![0.0f32; n_heads * seq_len * seq_len];

    for h in 0..n_heads {
        let h_kv = h / group_size; // GQA grouping: query head h reads from KV head h_kv
        let attn_off = h * seq_len * seq_len;

        // Step 1: scores[i, j] = (Q[i, h] . K[j, h_kv]) * scale, masked
        for i in 0..seq_len {
            // Compute row of pre-softmax scores
            let mut row = vec![f32::NEG_INFINITY; seq_len];
            #[allow(clippy::needless_range_loop)] // strided tensor index — see crate note
            for j in 0..seq_len {
                if !mask.allows(i, j) {
                    continue; // leave as -inf
                }
                let mut dot = 0.0f32;
                for d in 0..head_dim {
                    let q_id = (i * n_heads + h) * head_dim + d;
                    let k_jd = (j * n_kv_heads + h_kv) * head_dim + d;
                    dot += q_rot[q_id] * k_rot[k_jd];
                }
                row[j] = dot * scale;
            }

            // Step 2: softmax with numerical stability (max-subtract)
            softmax(&mut row);

            // Step 3: cache attention weights for backward
            for j in 0..seq_len {
                attn_cache[attn_off + i * seq_len + j] = row[j];
            }

            // Step 4: output[i, h] = sum_j attn[i, j] * V[j, h_kv]
            for d in 0..head_dim {
                let mut sum = 0.0f32;
                #[allow(clippy::needless_range_loop)] // strided tensor index — see crate note
                for j in 0..seq_len {
                    let v_jd = (j * n_kv_heads + h_kv) * head_dim + d;
                    sum += row[j] * v[v_jd];
                }
                output[(i * n_heads + h) * head_dim + d] = sum;
            }
        }
    }

    (output, attn_cache)
}

/// GELU activation, tanh approximation (matches Gemma 4's `gelu_pytorch_tanh`).
///
/// gelu_tanh(x) = 0.5 * x * (1 + tanh(sqrt(2/pi) * (x + 0.044715 * x^3)))
///
/// This is the approximation used by Gemma 4 / Llama / many transformer
/// implementations. It differs from the exact GELU (`x * Φ(x)`) by ~1e-3.
pub fn gelu_tanh_approx(x: f32) -> f32 {
    const SQRT_2_OVER_PI: f32 = 0.797_884_6; // sqrt(2.0 / PI)
    const ALPHA: f32 = 0.044715;
    let inner = SQRT_2_OVER_PI * (x + ALPHA * x * x * x);
    0.5 * x * (1.0 + inner.tanh())
}

/// Logit softcapping (Gemma 4 final layer): bounds logits via tanh.
/// `out = tanh(x / cap) * cap`
/// For cap=30, logits saturate smoothly at ±30.
pub fn logit_softcap(x: f32, cap: f32) -> f32 {
    (x / cap).tanh() * cap
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_embedding_lookup() {
        // 4 vocab × 3 dim embedding
        let weights = vec![
            1.0, 2.0, 3.0, // token 0
            4.0, 5.0, 6.0, // token 1
            7.0, 8.0, 9.0, // token 2
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
        assert!(
            (sum - 1.0).abs() < 1e-5,
            "Softmax should sum to 1, got {}",
            sum
        );
        assert!(logits[2] > logits[1] && logits[1] > logits[0]);
    }

    #[test]
    fn test_cross_entropy() {
        let logits = vec![1.0, 5.0, 1.0]; // Token 1 has highest logit
        let loss_correct = cross_entropy_loss(&logits, 1); // Correct prediction
        let loss_wrong = cross_entropy_loss(&logits, 0); // Wrong prediction
        assert!(
            loss_correct < loss_wrong,
            "Loss for correct token should be lower"
        );
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

    // === WAVE10F Phase 1 forward tests ===

    #[test]
    fn test_rope_apply_orthogonal() {
        // RoPE rotation preserves L2 norm per position.
        let seq_len = 4;
        let head_dim = 8;
        let (cos, sin) = rope_freqs(seq_len, head_dim, 10000.0);

        let mut x = Vec::with_capacity(seq_len * head_dim);
        for i in 0..seq_len * head_dim {
            x.push(((i as f32) * 0.137).sin());
        }
        let y = rope_apply(&x, &cos, &sin, seq_len, head_dim);

        for s in 0..seq_len {
            let nx: f32 = (0..head_dim)
                .map(|d| {
                    let v = x[s * head_dim + d];
                    v * v
                })
                .sum();
            let ny: f32 = (0..head_dim)
                .map(|d| {
                    let v = y[s * head_dim + d];
                    v * v
                })
                .sum();
            assert!(
                (nx - ny).abs() < 1e-4,
                "RoPE not norm-preserving at pos {}: |x|^2={} |y|^2={}",
                s,
                nx,
                ny
            );
        }
    }

    #[test]
    fn test_rope_freqs_position_zero_identity() {
        // At position 0, all angles are 0 → cos=1, sin=0 → no rotation.
        let head_dim = 8;
        let (cos, sin) = rope_freqs(2, head_dim, 10000.0);
        for d in 0..head_dim / 2 {
            assert!((cos[d] - 1.0).abs() < 1e-6);
            assert!(sin[d].abs() < 1e-6);
        }
    }

    #[test]
    fn test_attention_forward_causal_mask() {
        // First token's attention should put all weight on itself (mask blocks all later).
        let seq_len = 3;
        let n_heads = 1;
        let n_kv_heads = 1;
        let head_dim = 4;

        let q = vec![1.0; seq_len * n_heads * head_dim];
        let k = vec![1.0; seq_len * n_kv_heads * head_dim];
        let v = vec![0.5; seq_len * n_kv_heads * head_dim];

        let (out, attn) = attention_forward(
            &q,
            &k,
            &v,
            n_heads,
            n_kv_heads,
            head_dim,
            seq_len,
            AttnMask::Causal,
        );

        // Position 0 only attends to position 0 → attn[0, 0, 0] = 1.0
        assert!(
            (attn[0] - 1.0).abs() < 1e-6,
            "attn[0,0,0] should be 1.0, got {}",
            attn[0]
        );
        assert_eq!(attn[1], 0.0, "attn[0,0,1] should be 0 (masked)");
        assert_eq!(attn[2], 0.0, "attn[0,0,2] should be 0 (masked)");

        // Output[0] should equal V[0] (since attention is one-hot on position 0)
        for (d, &o) in out.iter().take(head_dim).enumerate() {
            assert!((o - 0.5).abs() < 1e-5, "out[{d}] should be 0.5");
        }
    }

    #[test]
    fn test_attention_forward_sliding_window() {
        // SlidingWindow(2): position 2 can only attend to positions [1, 2]
        let seq_len = 3;
        let n_heads = 1;
        let n_kv_heads = 1;
        let head_dim = 4;

        let q = vec![1.0; seq_len * n_heads * head_dim];
        let k = vec![1.0; seq_len * n_kv_heads * head_dim];
        let v = vec![1.0; seq_len * n_kv_heads * head_dim];

        let (_out, attn) = attention_forward(
            &q,
            &k,
            &v,
            n_heads,
            n_kv_heads,
            head_dim,
            seq_len,
            AttnMask::SlidingWindow(2),
        );

        // Position 2's attention: [0]=0 (out of window), [1]=0.5, [2]=0.5
        let row2_off = 2 * seq_len;
        assert_eq!(
            attn[row2_off], 0.0,
            "pos 2 should not attend to pos 0 (window=2)"
        );
        assert!((attn[row2_off + 1] - 0.5).abs() < 1e-5);
        assert!((attn[row2_off + 2] - 0.5).abs() < 1e-5);
    }

    #[test]
    fn test_gelu_tanh_approx_basic() {
        // gelu(0) = 0
        assert!(gelu_tanh_approx(0.0).abs() < 1e-6);
        // gelu is monotonically increasing
        assert!(gelu_tanh_approx(0.5) < gelu_tanh_approx(1.0));
        assert!(gelu_tanh_approx(-1.0) < gelu_tanh_approx(0.0));
        // Asymptotically: large positive → x; large negative → 0
        assert!(gelu_tanh_approx(10.0) > 9.99);
        assert!(gelu_tanh_approx(-10.0).abs() < 0.01);
    }

    #[test]
    fn test_logit_softcap_bounds() {
        let cap = 30.0;
        // Within cap, near-linear
        let small = logit_softcap(5.0, cap);
        assert!(
            (small - 5.0).abs() < 0.5,
            "small input should be near-linear"
        );
        // Asymptote at cap (tanh saturates to exactly 1.0 in f32 for large inputs).
        let big = logit_softcap(1000.0, cap);
        assert!(
            big <= cap,
            "softcap output {} must not exceed cap {}",
            big,
            cap
        );
        assert!(big > cap * 0.99, "softcap should approach cap, got {}", big);
        // Symmetric
        assert!((logit_softcap(7.0, cap) + logit_softcap(-7.0, cap)).abs() < 1e-5);
    }

    #[test]
    #[ignore] // heavy: loads ~5 GB Mistral-7B q5_k_m GGUF — OOM risk on 14 GB dev box; run on east/west or via `cargo test -- --ignored`
    fn test_dequantize_real_tensor() {
        let model_path = "/var/zhen/models/mistral-7b-instruct-q5_k_m.gguf";
        if !std::path::Path::new(model_path).exists() {
            println!("Model not found, skipping");
            return;
        }

        let model = GgufFile::open(model_path).expect("Failed to open model");

        // Dequantize the output_norm weight (F32 tensor, small)
        if let Some(data) = dequantize_tensor(&model, "output_norm.weight") {
            println!(
                "output_norm.weight: {} elements, first 5: {:?}",
                data.len(),
                &data[..5.min(data.len())]
            );
            assert_eq!(data.len(), 4096, "output_norm should be 4096 dim");
            // F32 tensor should have non-zero values
            assert!(
                data.iter().any(|&v| v != 0.0),
                "output_norm should have non-zero values"
            );
        }

        // Dequantize a Q5_K tensor (attention weight)
        if let Some(data) = dequantize_tensor(&model, "blk.0.attn_q.weight") {
            println!(
                "blk.0.attn_q.weight: {} elements, first 5: {:?}",
                data.len(),
                &data[..5.min(data.len())]
            );
            assert!(!data.is_empty());
            // Q5_K dequantized should have non-zero values
            assert!(
                data.iter().any(|&v| v != 0.0),
                "Q5_K dequant should produce non-zero values"
            );
        }
    }
}
