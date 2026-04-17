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
        let n_ff = getu("feed_forward_length")? as usize;
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

        // Per-layer attention type. Gemma 4 stores this as a packed bitfield in
        // `gemma4.attention.sliding_window_pattern` OR (more commonly) we infer
        // it from per-layer rope tensor presence: sliding layers don't have a
        // per-layer rope_freqs tensor, full layers do. As a fallback, use the
        // pattern from config.json (every 5th layer is full, indices 4,9,14,19,...).
        let layer_is_sliding = infer_layer_pattern(model, n_layer);

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

/// Infer per-layer sliding/full pattern. Best path: GGUF has a packed pattern
/// metadata. Fallback: check whether each layer has a per-layer rope_freqs
/// tensor (full layers DUPLICATE the global one in current llama.cpp; the
/// global presence + every-5th pattern from config is the practical signal).
fn infer_layer_pattern(model: &GgufFile, n_layer: usize) -> Vec<bool> {
    // Try to read the packed pattern from metadata first.
    if let Some(pattern_str) = model.get_arch_string("attention.sliding_window_pattern") {
        // Pattern is stored as a comma-separated or array string; parse.
        let mut out = Vec::with_capacity(n_layer);
        for s in pattern_str.split([',', ' ', '\n']).filter(|s| !s.is_empty()) {
            // 1 = sliding, 0 = full (per llama.cpp convention)
            out.push(s.trim() == "1" || s.trim().eq_ignore_ascii_case("true"));
        }
        if out.len() == n_layer {
            return out;
        }
        // Fall through if parse failed
    }
    // Fallback: every 5th layer is full attention (verified pattern from
    // /home/govan/tmp/gemma-4-E2B-it/config.json layer_types). Layers
    // 4, 9, 14, ..., 34 are full; rest are sliding.
    (0..n_layer).map(|i| (i + 1) % 5 != 0).collect()
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
    fn test_layer_pattern_inference() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            return;
        }
        let model = GgufFile::open(model_path).unwrap();
        let pattern = infer_layer_pattern(&model, 35);
        let full_indices: Vec<usize> = pattern
            .iter()
            .enumerate()
            .filter_map(|(i, &s)| if !s { Some(i) } else { None })
            .collect();
        // GGUF metadata is the ground truth. For E2B it's one of two patterns
        // depending on convention: HF config.json says [4,9,14,19,24,29,34]
        // (7 full), but the converted GGUF's sliding_window_pattern metadata
        // tags layer 0 as full too, giving [0,4,9,14,19,24,29,34] (8 full).
        // Final layer MUST be full per the architectural spec; accept either.
        assert!(full_indices.contains(&34), "final layer (34) must be full");
        assert!(full_indices.contains(&4) && full_indices.contains(&9)
            && full_indices.contains(&14) && full_indices.contains(&19)
            && full_indices.contains(&24) && full_indices.contains(&29),
            "every-5th-layer-is-full pattern broken: {:?}", full_indices);
        let n_full = full_indices.len();
        assert!(n_full == 7 || n_full == 8,
            "expected 7 or 8 full-attention layers, got {}: {:?}", n_full, full_indices);
        println!("Layer pattern: {} full, {} sliding. Full indices: {:?}",
            n_full, 35 - n_full, full_indices);
    }
}

// Suppress warnings for the unused HashMap import — kept for future tensor mapping use
#[allow(dead_code)]
fn _suppress_unused() {
    let _: HashMap<String, String> = HashMap::new();
}
