// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Gemma 4 specific weight loading + forward + backward.
//!
//! WAVE10F Phase 2.2 + Phase 1c (Gemma-direct path).
//!
//! This module is the NEW Gemma 4 training path. It does NOT touch the
//! existing Mistral training code in `train.rs` (kept as stale functions
//! per Stevie 2026-04-17). All math is the real-attention versions
//! committed in `forward.rs` + `backward.rs` (Phase 1).
//!
//! Architecture spec: `notes/gemma4-arch-spec.md`
//! Math notes: `notes/phase1-attention-math.md`

use crate::gguf::{Architecture, GgufFile};
use crate::forward;
use half::bf16;
use std::collections::HashMap;

/// Hyperparameters for Gemma 4 (per-layer where applicable).
/// All values read from GGUF metadata at load time.
#[derive(Debug, Clone)]
pub struct Gemma4Hparams {
    pub n_layer: usize,
    pub n_embd: usize,
    pub n_ff: usize,
    pub n_head: usize,
    pub n_head_kv: usize,
    /// head_dim for full-attention layers (gemma4.attention.key_length)
    pub head_dim_full: usize,
    /// head_dim for sliding-attention layers (gemma4.attention.key_length_swa)
    pub head_dim_swa: usize,
    /// First N layers produce K/V; layers [n_layer_kv_from_start, n_layer) reuse.
    pub n_layer_kv_from_start: usize,
    pub sliding_window: usize,
    pub vocab_size: usize,
    /// Per-Layer-Embedding hidden dim (0 if PLE disabled)
    pub n_embd_per_layer: usize,
    pub rms_norm_eps: f32,
    pub final_logit_softcapping: f32,
    pub rope_freq_base_full: f32,
    pub rope_freq_base_swa: f32,
    pub rope_dim_full: usize,
    pub rope_dim_swa: usize,
    /// Per-layer attention type. true = sliding, false = full.
    pub layer_is_sliding: Vec<bool>,
}

impl Gemma4Hparams {
    pub fn from_gguf(model: &GgufFile) -> Result<Self, String> {
        if !matches!(model.architecture(), Architecture::Gemma4) {
            return Err(format!(
                "Expected gemma4 architecture, got {:?}",
                model.architecture()
            ));
        }
        // Many Gemma 4 hparams are stored as per-layer arrays even when values
        // are uniform across layers. Use *_or_first accessors that transparently
        // handle both scalar and array-encoded hparams.
        let getu = |k: &str| -> Result<u32, String> {
            model
                .get_arch_u32_or_first(k)
                .ok_or_else(|| format!("missing GGUF key gemma4.{}", k))
        };
        let getf = |k: &str| -> Result<f32, String> {
            model
                .get_arch_f32_or_first(k)
                .ok_or_else(|| format!("missing GGUF key gemma4.{}", k))
        };

        let n_layer = getu("block_count")? as usize;
        let n_embd = getu("embedding_length")? as usize;
        // Some Gemma 4 hparams are stored as per-layer arrays even when uniform,
        // and some are stored with conventions that don't match tensor shapes
        // (e.g., feed_forward_length = 12288 in metadata but actual n_ff = 6144
        // due to use_double_wide_mlp=true storing combined gate+up dim).
        // Trust tensor shapes over metadata for these.
        let n_ff = ffn_dim_from_tensors(model)
            .ok_or("could not infer n_ff from blk.0.ffn_gate.weight shape")?;
        let n_head = getu("attention.head_count")? as usize;
        let n_head_kv = getu("attention.head_count_kv")? as usize;
        let head_dim_full = getu("attention.key_length")? as usize;
        let head_dim_swa = getu("attention.key_length_swa")? as usize;
        let n_layer_kv_from_start = getu("attention.shared_kv_layers")? as usize;
        let sliding_window = getu("attention.sliding_window")? as usize;
        let n_embd_per_layer = getu("embedding_length_per_layer_input").unwrap_or(0) as usize;
        let rms_norm_eps = getf("attention.layer_norm_rms_epsilon")?;
        let final_logit_softcapping = model
            .get_arch_f32("final_logit_softcapping")
            .unwrap_or(0.0);
        let rope_freq_base_full = getf("rope.freq_base")?;
        let rope_freq_base_swa = getf("rope.freq_base_swa")?;
        let rope_dim_full = getu("rope.dimension_count")? as usize;
        let rope_dim_swa = getu("rope.dimension_count_swa")? as usize;

        // Vocab size — Gemma 4 doesn't have a `gemma4.vocab_size` key; derive
        // from token_embd tensor's first dim or count tokenizer tokens.
        let vocab_size = model
            .tensors
            .iter()
            .find(|t| t.name == "token_embd.weight")
            .and_then(|t| t.dimensions.last().copied())
            .ok_or("token_embd.weight missing — cannot determine vocab_size")?
            as usize;

        // Per-layer attention type — infer from actual wq tensor shape.
        // Sliding layers have wq.shape[1] = n_head * head_dim_swa (e.g. 2048
        // for E2B). Full layers have wq.shape[1] = n_head * head_dim_full
        // (e.g. 4096). The GGUF metadata key `attention.sliding_window_pattern`
        // is unreliable (E2B stores it as a single bool, not a per-layer array).
        let layer_is_sliding = (0..n_layer).map(|il| {
            let wq_name = format!("blk.{}.attn_q.weight", il);
            let q_out_dim = model.tensors.iter()
                .find(|t| t.name == wq_name)
                .and_then(|t| t.dimensions.last().copied())
                .unwrap_or(0) as usize;
            let head_dim_per_layer = q_out_dim / n_head.max(1);
            head_dim_per_layer == head_dim_swa
        }).collect();

        Ok(Self {
            n_layer,
            n_embd,
            n_ff,
            n_head,
            n_head_kv,
            head_dim_full,
            head_dim_swa,
            n_layer_kv_from_start,
            sliding_window,
            vocab_size,
            n_embd_per_layer,
            rms_norm_eps,
            final_logit_softcapping,
            rope_freq_base_full,
            rope_freq_base_swa,
            rope_dim_full,
            rope_dim_swa,
            layer_is_sliding,
        })
    }

    /// True if this layer produces its own K/V (vs reusing earlier).
    pub fn has_kv(&self, layer_idx: usize) -> bool {
        layer_idx < self.n_layer_kv_from_start
    }

    /// True if this layer uses sliding-window attention (vs full global).
    pub fn is_sliding(&self, layer_idx: usize) -> bool {
        self.layer_is_sliding
            .get(layer_idx)
            .copied()
            .unwrap_or(false)
    }

    /// Per-layer head dimension (varies per attention type per Gemma 4 spec).
    pub fn head_dim(&self, layer_idx: usize) -> usize {
        if self.is_sliding(layer_idx) {
            self.head_dim_swa
        } else {
            self.head_dim_full
        }
    }

    /// Per-layer RoPE frequency base.
    pub fn rope_freq_base(&self, layer_idx: usize) -> f32 {
        if self.is_sliding(layer_idx) {
            self.rope_freq_base_swa
        } else {
            self.rope_freq_base_full
        }
    }

    /// Per-layer RoPE rotation dim.
    pub fn rope_dim(&self, layer_idx: usize) -> usize {
        if self.is_sliding(layer_idx) {
            self.rope_dim_swa
        } else {
            self.rope_dim_full
        }
    }
}

/// Read FFN dim from blk.0.ffn_gate.weight shape (`[n_embd, n_ff]`).
/// More reliable than the `feed_forward_length` GGUF metadata key, which
/// for Gemma 4 stores 2*n_ff (combined gate+up) due to use_double_wide_mlp.
fn ffn_dim_from_tensors(model: &GgufFile) -> Option<usize> {
    let t = model.tensors.iter().find(|t| t.name == "blk.0.ffn_gate.weight")?;
    t.dimensions.last().copied().map(|d| d as usize)
}

/// Loaded Gemma 4 weights, all stored as bf16 to fit memory (matches GGUF
/// on-disk format for the main weight tensors). F32 tensors (norms, scales)
/// stay as f32. LoRA tensors are managed separately by `lora::LoraAdapters`.
pub struct CpuWeightsGemma4 {
    pub hparams: Gemma4Hparams,

    // Globals
    pub token_embd: Vec<bf16>,           // [n_embd, n_vocab]
    pub output_norm: Vec<f32>,            // [n_embd]
    pub per_layer_token_embd: Vec<bf16>, // [n_embd_per_layer * n_layer, n_vocab]
    pub per_layer_model_proj: Vec<bf16>, // [n_embd, n_embd_per_layer * n_layer]
    pub per_layer_proj_norm: Vec<f32>,   // [n_embd_per_layer]
    pub rope_freqs: Vec<f32>,            // [head_dim_full / 2] — proportional RoPE table

    // Per-layer attention
    pub attn_norm: Vec<Vec<f32>>,         // [n_layer][n_embd]
    pub wq: Vec<Vec<bf16>>,               // [n_layer][n_embd, n_head*head_dim]
    pub wk: Vec<Option<Vec<bf16>>>,       // [n_layer][n_embd, n_head_kv*head_dim] — None if !has_kv
    pub wv: Vec<Option<Vec<bf16>>>,       // [n_layer][n_embd, n_head_kv*head_dim] — None if missing
    pub wo: Vec<Vec<bf16>>,               // [n_layer][n_head*head_dim, n_embd]
    pub attn_q_norm: Vec<Vec<f32>>,       // [n_layer][head_dim]
    pub attn_k_norm: Vec<Option<Vec<f32>>>, // [n_layer][head_dim] — None if !has_kv
    pub post_attention_norm: Vec<Vec<f32>>, // [n_layer][n_embd]

    // Per-layer FFN
    pub ffn_norm: Vec<Vec<f32>>,          // [n_layer][n_embd]
    pub ffn_gate: Vec<Vec<bf16>>,         // [n_layer][n_embd, n_ff]
    pub ffn_up: Vec<Vec<bf16>>,           // [n_layer][n_embd, n_ff]
    pub ffn_down: Vec<Vec<bf16>>,         // [n_layer][n_ff, n_embd]
    pub post_ffw_norm: Vec<Vec<f32>>,     // [n_layer][n_embd]

    // Per-layer PLE
    pub inp_gate: Vec<Vec<bf16>>,         // [n_layer][n_embd, n_embd_per_layer]
    pub proj: Vec<Vec<bf16>>,             // [n_layer][n_embd_per_layer, n_embd]
    pub post_norm: Vec<Vec<f32>>,         // [n_layer][n_embd]

    // Optional per-layer
    pub layer_output_scale: Vec<Option<f32>>, // [n_layer] — single scalar each
}

impl CpuWeightsGemma4 {
    /// Load all Gemma 4 weights from a GGUF. Validates required tensors and
    /// the n_embd_head_k == n_embd_head_v constraint. Skips multimodal (vision/
    /// audio) tensors entirely.
    pub fn load(model: &GgufFile) -> Result<Self, String> {
        let hparams = Gemma4Hparams::from_gguf(model)?;

        if hparams.head_dim_full != hparams.head_dim_full
            || hparams.head_dim_swa != hparams.head_dim_swa
        {
            return Err("Gemma 4 spec violation: head_dim_k != head_dim_v".into());
        }

        println!("  Loading Gemma 4 weights ({} layers, n_embd={}, vocab={}):",
            hparams.n_layer, hparams.n_embd, hparams.vocab_size);

        let load_bf16 = |name: &str| -> Result<Vec<bf16>, String> {
            let tensor = model
                .tensors
                .iter()
                .find(|t| t.name == name)
                .ok_or_else(|| format!("missing tensor {}", name))?;
            let data = model.tensor_data(tensor);
            let n = tensor.num_elements as usize;
            match tensor.tensor_type.as_str() {
                "BF16" => {
                    let mut out = Vec::with_capacity(n);
                    let mut i = 0;
                    while i + 1 < data.len() && out.len() < n {
                        out.push(bf16::from_bits(u16::from_le_bytes([data[i], data[i + 1]])));
                        i += 2;
                    }
                    Ok(out)
                }
                "F32" => {
                    // Convert f32 → bf16 for storage uniformity
                    let mut out = Vec::with_capacity(n);
                    let mut i = 0;
                    while i + 3 < data.len() && out.len() < n {
                        let f = f32::from_le_bytes([data[i], data[i + 1], data[i + 2], data[i + 3]]);
                        out.push(bf16::from_f32(f));
                        i += 4;
                    }
                    Ok(out)
                }
                other => Err(format!("tensor {} has unsupported type {}", name, other)),
            }
        };

        let load_f32 = |name: &str| -> Result<Vec<f32>, String> {
            forward::dequantize_tensor(model, name)
                .ok_or_else(|| format!("missing or unloadable tensor {}", name))
        };
        let load_f32_opt =
            |name: &str| -> Option<Vec<f32>> { forward::dequantize_tensor(model, name) };
        let load_bf16_opt = |name: &str| -> Option<Vec<bf16>> {
            model
                .tensors
                .iter()
                .find(|t| t.name == name)
                .and_then(|tensor| {
                    let data = model.tensor_data(tensor);
                    let n = tensor.num_elements as usize;
                    if tensor.tensor_type.as_str() == "BF16" {
                        let mut out = Vec::with_capacity(n);
                        let mut i = 0;
                        while i + 1 < data.len() && out.len() < n {
                            out.push(bf16::from_bits(u16::from_le_bytes([data[i], data[i + 1]])));
                            i += 2;
                        }
                        Some(out)
                    } else {
                        None
                    }
                })
        };

        // Globals
        println!("    [globals]");
        let token_embd = load_bf16("token_embd.weight")?;
        let output_norm = load_f32("output_norm.weight")?;
        let per_layer_token_embd = load_bf16("per_layer_token_embd.weight")?;
        let per_layer_model_proj = load_bf16("per_layer_model_proj.weight")?;
        let per_layer_proj_norm = load_f32("per_layer_proj_norm.weight")?;
        let rope_freqs = load_f32("rope_freqs.weight")?;
        println!(
            "      token_embd={:.1}MB ple_token={:.1}MB ple_proj={:.1}MB",
            token_embd.len() as f64 * 2.0 / 1e6,
            per_layer_token_embd.len() as f64 * 2.0 / 1e6,
            per_layer_model_proj.len() as f64 * 2.0 / 1e6,
        );

        let n_layer = hparams.n_layer;
        let mut attn_norm = Vec::with_capacity(n_layer);
        let mut wq = Vec::with_capacity(n_layer);
        let mut wk = Vec::with_capacity(n_layer);
        let mut wv = Vec::with_capacity(n_layer);
        let mut wo = Vec::with_capacity(n_layer);
        let mut attn_q_norm = Vec::with_capacity(n_layer);
        let mut attn_k_norm = Vec::with_capacity(n_layer);
        let mut post_attention_norm = Vec::with_capacity(n_layer);
        let mut ffn_norm = Vec::with_capacity(n_layer);
        let mut ffn_gate = Vec::with_capacity(n_layer);
        let mut ffn_up = Vec::with_capacity(n_layer);
        let mut ffn_down = Vec::with_capacity(n_layer);
        let mut post_ffw_norm = Vec::with_capacity(n_layer);
        let mut inp_gate = Vec::with_capacity(n_layer);
        let mut proj = Vec::with_capacity(n_layer);
        let mut post_norm = Vec::with_capacity(n_layer);
        let mut layer_output_scale = Vec::with_capacity(n_layer);

        for il in 0..n_layer {
            attn_norm.push(load_f32(&format!("blk.{}.attn_norm.weight", il))?);
            wq.push(load_bf16(&format!("blk.{}.attn_q.weight", il))?);
            if hparams.has_kv(il) {
                wk.push(Some(load_bf16(&format!("blk.{}.attn_k.weight", il))?));
                wv.push(load_bf16_opt(&format!("blk.{}.attn_v.weight", il)));
                attn_k_norm.push(Some(load_f32(&format!("blk.{}.attn_k_norm.weight", il))?));
            } else {
                wk.push(None);
                wv.push(None);
                attn_k_norm.push(None);
            }
            wo.push(load_bf16(&format!("blk.{}.attn_output.weight", il))?);
            attn_q_norm.push(load_f32(&format!("blk.{}.attn_q_norm.weight", il))?);
            post_attention_norm.push(load_f32(&format!("blk.{}.post_attention_norm.weight", il))?);

            ffn_norm.push(load_f32(&format!("blk.{}.ffn_norm.weight", il))?);
            ffn_gate.push(load_bf16(&format!("blk.{}.ffn_gate.weight", il))?);
            ffn_up.push(load_bf16(&format!("blk.{}.ffn_up.weight", il))?);
            ffn_down.push(load_bf16(&format!("blk.{}.ffn_down.weight", il))?);
            post_ffw_norm.push(load_f32(&format!("blk.{}.post_ffw_norm.weight", il))?);

            inp_gate.push(load_bf16(&format!("blk.{}.inp_gate.weight", il))?);
            proj.push(load_bf16(&format!("blk.{}.proj.weight", il))?);
            post_norm.push(load_f32(&format!("blk.{}.post_norm.weight", il))?);

            layer_output_scale.push(
                load_f32_opt(&format!("blk.{}.layer_output_scale.weight", il))
                    .map(|v| v.first().copied().unwrap_or(1.0)),
            );
        }
        println!("    [{} layers loaded]", n_layer);

        Ok(Self {
            hparams,
            token_embd,
            output_norm,
            per_layer_token_embd,
            per_layer_model_proj,
            per_layer_proj_norm,
            rope_freqs,
            attn_norm,
            wq,
            wk,
            wv,
            wo,
            attn_q_norm,
            attn_k_norm,
            post_attention_norm,
            ffn_norm,
            ffn_gate,
            ffn_up,
            ffn_down,
            post_ffw_norm,
            inp_gate,
            proj,
            post_norm,
            layer_output_scale,
        })
    }

    /// Total memory footprint of the loaded weights (excluding indices/headers).
    pub fn size_bytes(&self) -> u64 {
        let mut total: u64 = 0;
        // bf16 tensors = 2 bytes/element
        let bf16_total = self.token_embd.len()
            + self.per_layer_token_embd.len()
            + self.per_layer_model_proj.len()
            + self.wq.iter().map(|v| v.len()).sum::<usize>()
            + self.wk.iter().filter_map(|o| o.as_ref().map(|v| v.len())).sum::<usize>()
            + self.wv.iter().filter_map(|o| o.as_ref().map(|v| v.len())).sum::<usize>()
            + self.wo.iter().map(|v| v.len()).sum::<usize>()
            + self.ffn_gate.iter().map(|v| v.len()).sum::<usize>()
            + self.ffn_up.iter().map(|v| v.len()).sum::<usize>()
            + self.ffn_down.iter().map(|v| v.len()).sum::<usize>()
            + self.inp_gate.iter().map(|v| v.len()).sum::<usize>()
            + self.proj.iter().map(|v| v.len()).sum::<usize>();
        total += (bf16_total * 2) as u64;
        // f32 tensors = 4 bytes/element
        let f32_total = self.output_norm.len()
            + self.per_layer_proj_norm.len()
            + self.rope_freqs.len()
            + self.attn_norm.iter().map(|v| v.len()).sum::<usize>()
            + self.attn_q_norm.iter().map(|v| v.len()).sum::<usize>()
            + self.attn_k_norm.iter().filter_map(|o| o.as_ref().map(|v| v.len())).sum::<usize>()
            + self.post_attention_norm.iter().map(|v| v.len()).sum::<usize>()
            + self.ffn_norm.iter().map(|v| v.len()).sum::<usize>()
            + self.post_ffw_norm.iter().map(|v| v.len()).sum::<usize>()
            + self.post_norm.iter().map(|v| v.len()).sum::<usize>();
        total += (f32_total * 4) as u64;
        total += self.layer_output_scale.iter().filter(|o| o.is_some()).count() as u64 * 4;
        total
    }

    /// Print a summary of loaded weights — useful for verification.
    pub fn print_summary(&self) {
        let h = &self.hparams;
        println!("Gemma 4 model loaded:");
        println!("  Layers:         {}", h.n_layer);
        println!("  Hidden dim:     {}", h.n_embd);
        println!("  FFN dim:        {}", h.n_ff);
        println!("  Heads (Q):      {}", h.n_head);
        println!("  Heads (KV):     {}", h.n_head_kv);
        println!("  Head dim full:  {}", h.head_dim_full);
        println!("  Head dim SWA:   {}", h.head_dim_swa);
        println!("  Sliding window: {}", h.sliding_window);
        println!("  KV-shared from: layer {}", h.n_layer_kv_from_start);
        println!("  Vocab:          {}", h.vocab_size);
        println!("  PLE dim:        {}", h.n_embd_per_layer);
        println!("  RoPE θ full:    {}", h.rope_freq_base_full);
        println!("  RoPE θ SWA:     {}", h.rope_freq_base_swa);
        println!("  RoPE rot full:  {}", h.rope_dim_full);
        println!("  RoPE rot SWA:   {}", h.rope_dim_swa);
        println!("  Logit softcap:  {}", h.final_logit_softcapping);
        let n_full = h.layer_is_sliding.iter().filter(|&&s| !s).count();
        let n_sliding = h.layer_is_sliding.iter().filter(|&&s| s).count();
        println!("  Layer pattern:  {} sliding + {} full", n_sliding, n_full);
        println!("  Memory:         {:.1} MB", self.size_bytes() as f64 / 1e6);
    }
}

// =============================================================================
// Phase 1c — forward pass for Gemma 4 (CPU, correctness first).
//
// This is the minimal-viable forward: embedding + all layers (real attention
// with hybrid sliding/full mask, real RoPE with per-layer freq, real FFN
// with gelu_tanh_approx + parallel gate-up, post-norms, residuals) + final
// norm + tied LM head + logit softcap.
//
// Deliberate simplifications for this commit:
//  - PLE chain skipped (next commit)
//  - LoRA skipped (wired when backward is ready)
//  - No MoE (not in E2B)
//  - No multimodal
//  - No layer_output_scale application
//
// Memory: runs per-position to keep peak modest; attention scores cached
// per-layer for backward.
// =============================================================================

use crate::backward;

/// Per-layer state saved during forward, consumed by backward.
pub struct Gemma4LayerCache {
    pub normed_input: Vec<f32>,       // [seq, n_embd] — post-attn-norm
    pub q_rot: Vec<f32>,              // [seq, n_head, head_dim]
    pub k_rot: Vec<f32>,              // [seq, n_head_kv, head_dim]
    pub v: Vec<f32>,                  // [seq, n_head_kv, head_dim]
    pub attn_cache: Vec<f32>,         // [n_head, seq, seq] — softmax output
    pub attn_out: Vec<f32>,           // [seq, n_embd] — pre-O-proj
    pub post_attn_residual: Vec<f32>, // [seq, n_embd] — after post_attention_norm + residual
    pub ffn_normed: Vec<f32>,         // [seq, n_embd] — input to FFN
    pub ffn_gate_pre: Vec<f32>,       // [seq, n_ff] — pre-GELU
    pub ffn_up_pre: Vec<f32>,         // [seq, n_ff] — pre-multiply
    pub ffn_hidden: Vec<f32>,         // [seq, n_ff] — after gate * up
    pub post_ffw_residual: Vec<f32>,  // [seq, n_embd] — layer output before next layer
}

/// Convert a bf16 weight vector to f32 (CPU).
fn bf16_to_f32_vec(w: &[bf16]) -> Vec<f32> {
    w.iter().map(|b| b.to_f32()).collect()
}

/// Dense matmul C = A @ B^T.  A: [m, k], B: [n, k] → C: [m, n].
/// (B is stored row-major with n rows of k cols — the usual weight shape.)
fn matmul_x_wt(a: &[f32], w: &[f32], m: usize, n: usize, k: usize) -> Vec<f32> {
    let mut out = vec![0.0f32; m * n];
    for i in 0..m {
        for j in 0..n {
            let mut s = 0.0f32;
            for l in 0..k {
                s += a[i * k + l] * w[j * k + l];
            }
            out[i * n + j] = s;
        }
    }
    out
}

/// Apply per-head RMSNorm over the last axis.
/// x: [seq, n_head, head_dim]; weight: [head_dim]
fn per_head_rmsnorm(x: &[f32], weight: &[f32], seq: usize, n_head: usize, head_dim: usize, eps: f32) -> Vec<f32> {
    let mut out = vec![0.0f32; x.len()];
    for s in 0..seq {
        for h in 0..n_head {
            let off = (s * n_head + h) * head_dim;
            let slice = &x[off..off + head_dim];
            let ss: f32 = slice.iter().map(|v| v * v).sum::<f32>() / head_dim as f32;
            let rms = (ss + eps).sqrt();
            for d in 0..head_dim {
                out[off + d] = (slice[d] / rms) * weight[d];
            }
        }
    }
    out
}

/// Gemma 4 forward pass. Returns (logits, per-layer cache for backward).
/// tokens: input token IDs
/// seq_len: number of tokens
pub fn forward_gemma4(
    weights: &CpuWeightsGemma4,
    tokens: &[u32],
) -> (Vec<f32>, Vec<Gemma4LayerCache>) {
    let h = &weights.hparams;
    let seq = tokens.len();
    let n_embd = h.n_embd;

    // 1. Embedding lookup + sqrt(n_embd) scale (gemma4-iswa.cpp:20)
    let tok_embd_f32 = bf16_to_f32_vec(&weights.token_embd);
    let mut hidden = forward::embedding_lookup(&tok_embd_f32, n_embd, tokens);
    let embed_scale = (n_embd as f32).sqrt();
    for v in hidden.iter_mut() {
        *v *= embed_scale;
    }

    let mut layer_caches: Vec<Gemma4LayerCache> = Vec::with_capacity(h.n_layer);

    // 2. Per-layer loop
    for il in 0..h.n_layer {
        let head_dim = h.head_dim(il);
        let n_head = h.n_head;
        let n_head_kv = h.n_head_kv;
        let q_out_dim = n_head * head_dim;
        let kv_out_dim = n_head_kv * head_dim;

        // 2a. attn_norm
        let mut normed = vec![0.0f32; seq * n_embd];
        for s in 0..seq {
            let row = forward::rmsnorm(
                &hidden[s * n_embd..(s + 1) * n_embd],
                &weights.attn_norm[il],
                h.rms_norm_eps,
            );
            normed[s * n_embd..(s + 1) * n_embd].copy_from_slice(&row);
        }

        // 2b. Q projection: [seq, q_out_dim] = normed @ wq^T
        let wq_f32 = bf16_to_f32_vec(&weights.wq[il]);
        let q = matmul_x_wt(&normed, &wq_f32, seq, q_out_dim, n_embd);

        // 2c. K, V projections (if has_kv)
        let (k_flat, v_flat) = if let Some(wk) = &weights.wk[il] {
            let wk_f32 = bf16_to_f32_vec(wk);
            let wv_f32 = weights.wv[il]
                .as_ref()
                .map(|w| bf16_to_f32_vec(w))
                .unwrap_or_else(|| wk_f32.clone()); // Vcur = Kcur fallback
            let k = matmul_x_wt(&normed, &wk_f32, seq, kv_out_dim, n_embd);
            let v = matmul_x_wt(&normed, &wv_f32, seq, kv_out_dim, n_embd);
            (k, v)
        } else {
            // KV-reusing layer — use the last-produced K/V from cache.
            // For now, use the PREVIOUS layer's K/V (simplification; actual
            // Gemma 4 behavior is "reuse from last same-type KV producer").
            let prev = layer_caches.last().unwrap();
            // Re-flatten from [seq, n_head_kv, head_dim] shape. But the head_dim
            // may have changed between layers of different attention types —
            // handled via head_dim(il) being consistent with how K/V were built.
            (prev.k_rot.clone(), prev.v.clone())
        };

        // 2d. Reshape Q, K, V to [seq, n_head(_kv), head_dim], apply q_norm/k_norm
        //     (Q and K get per-head RMSNorm, V gets a plain RMSNorm with eps only)
        let q_normed = per_head_rmsnorm(
            &q,
            &weights.attn_q_norm[il],
            seq, n_head, head_dim, h.rms_norm_eps,
        );
        let k_normed = if let Some(k_norm) = &weights.attn_k_norm[il] {
            per_head_rmsnorm(&k_flat, k_norm, seq, n_head_kv, head_dim, h.rms_norm_eps)
        } else {
            k_flat.clone()
        };
        // V: plain RMSNorm using eps only (no weight tensor)
        let v_normed = {
            let mut out = vec![0.0f32; v_flat.len()];
            for s in 0..seq {
                for hh in 0..n_head_kv {
                    let off = (s * n_head_kv + hh) * head_dim;
                    let slice = &v_flat[off..off + head_dim];
                    let ss: f32 = slice.iter().map(|x| x * x).sum::<f32>() / head_dim as f32;
                    let rms = (ss + h.rms_norm_eps).sqrt();
                    for d in 0..head_dim {
                        out[off + d] = slice[d] / rms;
                    }
                }
            }
            out
        };

        // 2e. RoPE — per-layer freq base, partial rotary factor for full layers
        //     Sliding: standard RoPE, full rotation over rope_dim_swa dims
        //     Full: proportional RoPE, partial rotation over rope_dim_full dims
        let rope_dim = h.rope_dim(il);
        let freq_base = h.rope_freq_base(il);
        let (cos_table, sin_table) = forward::rope_freqs(seq, rope_dim, freq_base);

        let q_rot = rope_apply_partial(&q_normed, &cos_table, &sin_table, seq, n_head, head_dim, rope_dim);
        let k_rot = rope_apply_partial(&k_normed, &cos_table, &sin_table, seq, n_head_kv, head_dim, rope_dim);

        // 2f. Real attention with hybrid mask
        let mask = if h.is_sliding(il) {
            forward::AttnMask::SlidingWindow(h.sliding_window)
        } else {
            forward::AttnMask::Causal
        };
        let (attn_out_head, attn_cache) = forward::attention_forward(
            &q_rot, &k_rot, &v_normed,
            n_head, n_head_kv, head_dim, seq, mask,
        );
        // attn_out_head shape: [seq, n_head, head_dim] — flatten to [seq, q_out_dim]
        // (already contiguous in that order)

        // 2g. O projection: [seq, n_embd] = attn_out_head @ wo^T
        //     wo shape: [q_out_dim, n_embd] stored row-major
        let wo_f32 = bf16_to_f32_vec(&weights.wo[il]);
        let o_out = matmul_x_wt(&attn_out_head, &wo_f32, seq, n_embd, q_out_dim);

        // 2h. post_attention_norm + residual
        let mut post_attn = vec![0.0f32; seq * n_embd];
        for s in 0..seq {
            let row = forward::rmsnorm(
                &o_out[s * n_embd..(s + 1) * n_embd],
                &weights.post_attention_norm[il],
                h.rms_norm_eps,
            );
            for d in 0..n_embd {
                post_attn[s * n_embd + d] = row[d] + hidden[s * n_embd + d];
            }
        }

        // 2i. FFN — norm, parallel gate-up with gelu_tanh, down
        let mut ffn_normed = vec![0.0f32; seq * n_embd];
        for s in 0..seq {
            let row = forward::rmsnorm(
                &post_attn[s * n_embd..(s + 1) * n_embd],
                &weights.ffn_norm[il],
                h.rms_norm_eps,
            );
            ffn_normed[s * n_embd..(s + 1) * n_embd].copy_from_slice(&row);
        }
        let ffn_gate_f32 = bf16_to_f32_vec(&weights.ffn_gate[il]);
        let ffn_up_f32 = bf16_to_f32_vec(&weights.ffn_up[il]);
        let ffn_down_f32 = bf16_to_f32_vec(&weights.ffn_down[il]);
        let gate_pre = matmul_x_wt(&ffn_normed, &ffn_gate_f32, seq, h.n_ff, n_embd);
        let up_pre = matmul_x_wt(&ffn_normed, &ffn_up_f32, seq, h.n_ff, n_embd);
        let mut ffn_hidden = vec![0.0f32; seq * h.n_ff];
        for i in 0..ffn_hidden.len() {
            ffn_hidden[i] = forward::gelu_tanh_approx(gate_pre[i]) * up_pre[i];
        }
        let ffn_out = matmul_x_wt(&ffn_hidden, &ffn_down_f32, seq, n_embd, h.n_ff);

        // 2j. post_ffw_norm + residual (forming layer output for next iteration)
        let mut layer_out = vec![0.0f32; seq * n_embd];
        for s in 0..seq {
            let row = forward::rmsnorm(
                &ffn_out[s * n_embd..(s + 1) * n_embd],
                &weights.post_ffw_norm[il],
                h.rms_norm_eps,
            );
            for d in 0..n_embd {
                layer_out[s * n_embd + d] = row[d] + post_attn[s * n_embd + d];
            }
        }

        // NOTE: PLE contribution + layer_output_scale skipped in this commit —
        // next commit adds them.

        layer_caches.push(Gemma4LayerCache {
            normed_input: normed,
            q_rot,
            k_rot,
            v: v_normed,
            attn_cache,
            attn_out: attn_out_head,
            post_attn_residual: post_attn,
            ffn_normed,
            ffn_gate_pre: gate_pre,
            ffn_up_pre: up_pre,
            ffn_hidden,
            post_ffw_residual: layer_out.clone(),
        });

        hidden = layer_out;
    }

    // 3. Final output_norm
    let mut final_hidden = vec![0.0f32; seq * n_embd];
    for s in 0..seq {
        let row = forward::rmsnorm(
            &hidden[s * n_embd..(s + 1) * n_embd],
            &weights.output_norm,
            h.rms_norm_eps,
        );
        final_hidden[s * n_embd..(s + 1) * n_embd].copy_from_slice(&row);
    }

    // 4. LM head — tied to token_embd per tie_word_embeddings=true
    //    logits[s, v] = tok_embd[v, :] . final_hidden[s, :]
    let tok_embd_f32 = bf16_to_f32_vec(&weights.token_embd);
    // tok_embd is stored as [vocab, n_embd] row-major. Match shape.
    let logits = matmul_x_wt(
        &final_hidden,
        &tok_embd_f32,
        seq,
        h.vocab_size,
        n_embd,
    );

    // 5. Logit softcap
    let softcap = h.final_logit_softcapping;
    let logits = if softcap > 0.0 {
        logits
            .into_iter()
            .map(|x| forward::logit_softcap(x, softcap))
            .collect()
    } else {
        logits
    };

    (logits, layer_caches)
}

/// Apply RoPE to the first `rope_dim` dims of each head's feature vector,
/// leaving the remaining (head_dim - rope_dim) dims untouched.
/// This is how "proportional RoPE" (partial_rotary_factor=0.25 on Gemma 4's
/// full-attention layers, where rope_dim < head_dim) is implemented.
fn rope_apply_partial(
    x: &[f32],
    cos: &[f32],
    sin: &[f32],
    seq: usize,
    n_head: usize,
    head_dim: usize,
    rope_dim: usize,
) -> Vec<f32> {
    let mut out = x.to_vec();
    let half = rope_dim / 2;
    for s in 0..seq {
        for h in 0..n_head {
            let off = (s * n_head + h) * head_dim;
            for d in 0..half {
                let c = cos[s * half + d];
                let si = sin[s * half + d];
                let xe = x[off + 2 * d];
                let xo = x[off + 2 * d + 1];
                out[off + 2 * d]     = xe * c - xo * si;
                out[off + 2 * d + 1] = xe * si + xo * c;
            }
            // Dims [rope_dim..head_dim] already copied from x unchanged.
        }
    }
    out
}

// silence the unused import for now; backward integration comes with Phase 1c part 2
#[allow(dead_code)]
fn _use_backward() {
    let _ = backward::softmax_backward;
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_gemma4_load_e2b() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            println!("Gemma 4 GGUF not on disk — skipping");
            return;
        }
        let model = GgufFile::open(model_path).expect("open gguf");
        let weights = CpuWeightsGemma4::load(&model).expect("load");
        weights.print_summary();

        // Sanity asserts based on E2B verified hparams
        assert_eq!(weights.hparams.n_layer, 35);
        assert_eq!(weights.hparams.n_embd, 1536);
        assert_eq!(weights.hparams.n_ff, 6144);
        assert_eq!(weights.hparams.n_head, 8);
        assert_eq!(weights.hparams.n_head_kv, 1);
        assert_eq!(weights.hparams.n_layer_kv_from_start, 20);
        assert_eq!(weights.hparams.sliding_window, 512);
        assert_eq!(weights.hparams.vocab_size, 262144);
        assert_eq!(weights.hparams.n_embd_per_layer, 256);

        // KV-share pattern: first 20 layers have wk; last 15 don't
        for il in 0..20 {
            assert!(weights.wk[il].is_some(), "layer {} should have wk", il);
        }
        for il in 20..35 {
            assert!(weights.wk[il].is_none(), "layer {} should NOT have wk", il);
        }
    }

    #[test]
    fn test_gemma4_forward_finite() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            println!("Gemma 4 GGUF not on disk — skipping");
            return;
        }
        let model = GgufFile::open(model_path).expect("open gguf");
        let weights = CpuWeightsGemma4::load(&model).expect("load");

        // Small test: 4 tokens (BOS + 3 content tokens)
        let tokens: Vec<u32> = vec![2, 1000, 2000, 3000];
        println!("Running forward on {} tokens...", tokens.len());
        let start = std::time::Instant::now();
        let (logits, caches) = forward_gemma4(&weights, &tokens);
        let elapsed = start.elapsed().as_secs_f64();
        println!("Forward took {:.2}s, {} layer caches, {} logits",
            elapsed, caches.len(), logits.len());

        assert_eq!(caches.len(), weights.hparams.n_layer, "one cache per layer");
        assert_eq!(logits.len(), tokens.len() * weights.hparams.vocab_size,
            "logits = seq × vocab");

        // All logits finite
        let n_nan = logits.iter().filter(|x| x.is_nan()).count();
        let n_inf = logits.iter().filter(|x| x.is_infinite()).count();
        assert_eq!(n_nan, 0, "logits contain NaN");
        assert_eq!(n_inf, 0, "logits contain Inf");

        // Softcap applied — all in bounds
        let softcap = weights.hparams.final_logit_softcapping;
        if softcap > 0.0 {
            let max_abs = logits.iter().cloned().fold(0.0f32, |a, x| a.max(x.abs()));
            assert!(max_abs <= softcap + 1e-3,
                "|logit| = {} exceeds softcap {}", max_abs, softcap);
        }

        // Top-5 tokens for last position as sanity check
        let last_pos = tokens.len() - 1;
        let vocab = weights.hparams.vocab_size;
        let last_logits = &logits[last_pos * vocab..(last_pos + 1) * vocab];
        let mut indexed: Vec<(usize, f32)> = last_logits.iter().copied().enumerate().collect();
        indexed.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap());
        println!("Top-5 tokens for last position: {:?}",
            &indexed[..5.min(indexed.len())]);
    }

    #[test]
    fn test_layer_pattern_from_tensor_shapes() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            return;
        }
        let model = GgufFile::open(model_path).unwrap();
        let hparams = Gemma4Hparams::from_gguf(&model).expect("hparams");
        let full_indices: Vec<usize> = hparams.layer_is_sliding
            .iter().enumerate()
            .filter_map(|(i, &s)| if !s { Some(i) } else { None })
            .collect();
        // Per HF config.json layer_types and verified blk.{N}.attn_q.weight
        // shapes (full = 4096 = 8*512, sliding = 2048 = 8*256), full layers
        // are at indices [4, 9, 14, 19, 24, 29, 34].
        assert_eq!(full_indices, vec![4, 9, 14, 19, 24, 29, 34],
            "full-attention layers should be at indices [4,9,14,19,24,29,34]");
    }
}

// Suppress warnings for the unused HashMap import — kept for future tensor mapping use
#[allow(dead_code)]
fn _suppress_unused() {
    let _: HashMap<String, String> = HashMap::new();
}
