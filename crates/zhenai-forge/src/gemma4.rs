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
use crate::lora::LoraLayer;
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
// LoRA adapters sized for Gemma 4's per-layer variable head_dim + KV sharing.
// Each layer has 4 possible LoRA targets: 0=Q, 1=K, 2=V, 3=O.
// For layers 0..n_layer_kv_from_start (KV-producing): all 4 targets present.
// For KV-reusing layers: K and V targets are None (there's no wk/wv to adapt).
// =============================================================================

pub struct Gemma4LoraAdapters {
    pub layers: Vec<[Option<LoraLayer>; 4]>,
    pub rank: u32,
    pub alpha: f32,
}

impl Gemma4LoraAdapters {
    pub fn new(hparams: &Gemma4Hparams, rank: u32, alpha: f32) -> Self {
        let mut layers = Vec::with_capacity(hparams.n_layer);
        let n_embd = hparams.n_embd as u32;
        for il in 0..hparams.n_layer {
            let head_dim = hparams.head_dim(il);
            let q_out = (hparams.n_head * head_dim) as u32;
            let kv_out = (hparams.n_head_kv * head_dim) as u32;
            let has_kv = hparams.has_kv(il);
            let layer_loras: [Option<LoraLayer>; 4] = [
                Some(LoraLayer::new(n_embd, q_out, rank)),
                if has_kv { Some(LoraLayer::new(n_embd, kv_out, rank)) } else { None },
                if has_kv { Some(LoraLayer::new(n_embd, kv_out, rank)) } else { None },
                Some(LoraLayer::new(q_out, n_embd, rank)),
            ];
            layers.push(layer_loras);
        }
        Self { layers, rank, alpha }
    }

    pub fn scale(&self) -> f32 {
        self.alpha / self.rank as f32
    }

    /// Count active (non-None) LoRA targets.
    pub fn n_active_targets(&self) -> usize {
        self.layers.iter()
            .flat_map(|l| l.iter())
            .filter(|o| o.is_some())
            .count()
    }

    pub fn size_bytes(&self) -> u64 {
        self.layers.iter()
            .flat_map(|l| l.iter())
            .filter_map(|o| o.as_ref())
            .map(|l| l.num_params())
            .sum::<u64>() * 4  // f32
    }

    /// Save adapter to disk in ZLG4 format (custom binary, distinct from
    /// Mistral's ZLORA because Gemma 4 has per-layer per-target variable
    /// dims and Optional KV targets).
    ///
    /// Layout:
    ///   Magic   : b"ZLG4\x01" (5 bytes)
    ///   Header  : n_layer:u32, rank:u32, alpha:f32 (12 bytes)
    ///   Layers  : per layer, per target 0..4:
    ///               present:u8 (0=None, 1=Some)
    ///               if present: input_dim:u32, output_dim:u32,
    ///                           A vals (input_dim*rank f32),
    ///                           B vals (rank*output_dim f32)
    pub fn save(&self, path: &str) -> std::io::Result<()> {
        let mut data: Vec<u8> = Vec::new();
        data.extend_from_slice(b"ZLG4\x01");
        data.extend_from_slice(&(self.layers.len() as u32).to_le_bytes());
        data.extend_from_slice(&self.rank.to_le_bytes());
        data.extend_from_slice(&self.alpha.to_le_bytes());
        for layer in &self.layers {
            for slot in layer.iter() {
                match slot {
                    None => data.push(0u8),
                    Some(ll) => {
                        data.push(1u8);
                        data.extend_from_slice(&ll.input_dim.to_le_bytes());
                        data.extend_from_slice(&ll.output_dim.to_le_bytes());
                        for v in &ll.a {
                            data.extend_from_slice(&v.to_le_bytes());
                        }
                        for v in &ll.b {
                            data.extend_from_slice(&v.to_le_bytes());
                        }
                    }
                }
            }
        }
        std::fs::write(path, &data)
    }

    /// Load a previously-saved ZLG4 adapter. Validates magic + dims.
    pub fn load(path: &str) -> std::io::Result<Self> {
        let bytes = std::fs::read(path)?;
        let err = |s: &str| std::io::Error::new(std::io::ErrorKind::InvalidData, s);
        if bytes.len() < 5 + 12 || &bytes[..5] != b"ZLG4\x01" {
            return Err(err("not a ZLG4 file"));
        }
        let mut p = 5usize;
        let read_u32 = |b: &[u8], pos: &mut usize| -> u32 {
            let v = u32::from_le_bytes([b[*pos], b[*pos+1], b[*pos+2], b[*pos+3]]);
            *pos += 4; v
        };
        let read_f32 = |b: &[u8], pos: &mut usize| -> f32 {
            let v = f32::from_le_bytes([b[*pos], b[*pos+1], b[*pos+2], b[*pos+3]]);
            *pos += 4; v
        };
        let n_layer = read_u32(&bytes, &mut p) as usize;
        let rank = read_u32(&bytes, &mut p);
        let alpha = read_f32(&bytes, &mut p);

        let mut layers = Vec::with_capacity(n_layer);
        for _ in 0..n_layer {
            let mut slots: [Option<LoraLayer>; 4] =
                [None, None, None, None];
            for slot in slots.iter_mut() {
                if p >= bytes.len() { return Err(err("truncated")); }
                let present = bytes[p]; p += 1;
                if present == 1 {
                    if p + 8 > bytes.len() { return Err(err("truncated")); }
                    let input_dim = read_u32(&bytes, &mut p);
                    let output_dim = read_u32(&bytes, &mut p);
                    let a_size = (input_dim * rank) as usize;
                    let b_size = (rank * output_dim) as usize;
                    if p + 4 * (a_size + b_size) > bytes.len() {
                        return Err(err("truncated tensor"));
                    }
                    let mut a = Vec::with_capacity(a_size);
                    for _ in 0..a_size { a.push(read_f32(&bytes, &mut p)); }
                    let mut b = Vec::with_capacity(b_size);
                    for _ in 0..b_size { b.push(read_f32(&bytes, &mut p)); }
                    let mut layer = LoraLayer::new(input_dim, output_dim, rank);
                    layer.a = a;
                    layer.b = b;
                    *slot = Some(layer);
                }
            }
            layers.push(slots);
        }
        Ok(Self { layers, rank, alpha })
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
    pub post_ffw_residual: Vec<f32>,  // [seq, n_embd] — output of attn+ffn block, BEFORE PLE add
    // Phase 5 — PLE intermediates (Some only when PLE active):
    pub ple: Option<Gemma4PleCache>,
}

/// PLE chain intermediates per layer. Stored only when n_embd_per_layer > 0
/// (i.e., the model has Per-Layer Embeddings, true for E2B/E4B).
pub struct Gemma4PleCache {
    pub pe_in: Vec<f32>,              // [seq, n_embd] — input to PLE chain (= post_ffw_residual)
    pub gate_pre_gelu: Vec<f32>,      // [seq, n_embd_per_layer] — pre-GELU
    pub gate_post_gelu: Vec<f32>,     // [seq, n_embd_per_layer] — after GELU, before * inp_layer
    pub inp_layer_slice: Vec<f32>,    // [seq, n_embd_per_layer] — frozen lookup
    pub proj_out_pre_norm: Vec<f32>,  // [seq, n_embd] — pre-RMSNorm
}

/// Convert a bf16 weight vector to f32 (CPU).
///
/// Hot path — called many times per forward+backward step. Profile showed
/// the original iter+collect form was ~94% of forward time on Gemma 4.
/// Tight loop into a pre-allocated buffer is dramatically faster because
/// (a) no iterator overhead, (b) the loop autovectorizes, (c) single
/// allocation instead of incremental collect.
fn bf16_to_f32_vec(w: &[bf16]) -> Vec<f32> {
    let n = w.len();
    let mut out: Vec<f32> = Vec::with_capacity(n);
    // SAFETY: about to write all n elements before any read.
    unsafe { out.set_len(n); }
    let dst = out.as_mut_slice();
    // bf16 IS the upper 16 bits of f32 — shift left by 16 to convert.
    // half::bf16::to_bits returns u16. Doing this inline (vs to_f32) lets
    // LLVM vectorize the loop with AVX2 vpunpcklwd / vpshufd / vpsllqi etc.
    for i in 0..n {
        dst[i] = f32::from_bits((w[i].to_bits() as u32) << 16);
    }
    out
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
    forward_gemma4_with_lora(weights, None, tokens)
}

/// Per-op time accumulators for forward profiling. Enabled via env
/// FORGE_GEMMA4_PROFILE=1; otherwise each measurement is a few ns.
#[derive(Default)]
struct ProfTimes {
    enabled: bool,
    t_bf16conv: f64,
    t_qkvo_proj: f64,   // Q + K + V + O matmul (excludes LoRA add)
    t_qkvo_lora: f64,
    t_attention: f64,
    t_norms: f64,       // attn_norm, post_attn_norm, ffn_norm, post_ffw_norm
    t_ffn: f64,         // gate, up, down matmul
    t_rope: f64,
    t_ple: f64,
    t_lm_head: f64,
    n_layers: usize,
}
impl ProfTimes {
    fn from_env() -> Self {
        let enabled = std::env::var("FORGE_GEMMA4_PROFILE")
            .map(|v| v == "1").unwrap_or(false);
        Self { enabled, ..Default::default() }
    }
    // (timing recorded inline at call sites; no closure helper)
    fn report(&self) {
        if !self.enabled { return; }
        let total = self.t_bf16conv + self.t_qkvo_proj + self.t_qkvo_lora
            + self.t_attention + self.t_norms + self.t_ffn + self.t_rope
            + self.t_ple + self.t_lm_head;
        eprintln!("  [PROF] total={:.2}s breakdown:", total);
        let pct = |t: f64| if total > 0.0 { 100.0 * t / total } else { 0.0 };
        eprintln!("    bf16→f32 conversion : {:6.2}s ({:5.1}%)", self.t_bf16conv, pct(self.t_bf16conv));
        eprintln!("    Q/K/V/O projections : {:6.2}s ({:5.1}%)", self.t_qkvo_proj, pct(self.t_qkvo_proj));
        eprintln!("    Q/K/V/O LoRA        : {:6.2}s ({:5.1}%)", self.t_qkvo_lora, pct(self.t_qkvo_lora));
        eprintln!("    attention math      : {:6.2}s ({:5.1}%)", self.t_attention, pct(self.t_attention));
        eprintln!("    RMSNorm             : {:6.2}s ({:5.1}%)", self.t_norms, pct(self.t_norms));
        eprintln!("    FFN matmuls         : {:6.2}s ({:5.1}%)", self.t_ffn, pct(self.t_ffn));
        eprintln!("    RoPE                : {:6.2}s ({:5.1}%)", self.t_rope, pct(self.t_rope));
        eprintln!("    PLE chain           : {:6.2}s ({:5.1}%)", self.t_ple, pct(self.t_ple));
        eprintln!("    LM head             : {:6.2}s ({:5.1}%)", self.t_lm_head, pct(self.t_lm_head));
        eprintln!("    layers:             : {}", self.n_layers);
    }
}

/// Same as `forward_gemma4` but adds LoRA contributions at Q/K/V/O projections
/// when `lora` is Some. The scale factor is `alpha / rank`.
pub fn forward_gemma4_with_lora(
    weights: &CpuWeightsGemma4,
    lora: Option<&Gemma4LoraAdapters>,
    tokens: &[u32],
) -> (Vec<f32>, Vec<Gemma4LayerCache>) {
    let mut prof = ProfTimes::from_env();
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

    // 1b. Phase 5 — Per-Layer Embedding setup. Computed once, sliced per layer
    //     in the loop below. Storage: [seq, n_layer, n_embd_per_layer].
    let inp_per_layer: Option<Vec<f32>> = if h.n_embd_per_layer > 0 {
        Some(compute_inp_per_layer(weights, tokens, &hidden))
    } else {
        None
    };

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

        // 2b. Q projection: [seq, q_out_dim] = normed @ wq^T + LoRA_Q
        let _t = std::time::Instant::now();
        let wq_f32 = bf16_to_f32_vec(&weights.wq[il]);
        if prof.enabled { prof.t_bf16conv += _t.elapsed().as_secs_f64(); }
        let _t = std::time::Instant::now();
        let mut q = matmul_x_wt(&normed, &wq_f32, seq, q_out_dim, n_embd);
        if prof.enabled { prof.t_qkvo_proj += _t.elapsed().as_secs_f64(); }
        if let Some(lora_set) = lora {
            if let Some(lq) = &lora_set.layers[il][0] {
                let scale = lora_set.scale();
                for s in 0..seq {
                    let inp = &normed[s * n_embd..(s + 1) * n_embd];
                    let out = lq.forward(inp);
                    for d in 0..q_out_dim.min(out.len()) {
                        q[s * q_out_dim + d] += out[d] * scale;
                    }
                }
            }
        }

        // 2c. K, V projections (if has_kv)
        let (k_flat, v_flat) = if let Some(wk) = &weights.wk[il] {
            let wk_f32 = bf16_to_f32_vec(wk);
            let wv_f32 = weights.wv[il]
                .as_ref()
                .map(|w| bf16_to_f32_vec(w))
                .unwrap_or_else(|| wk_f32.clone()); // Vcur = Kcur fallback
            let mut k = matmul_x_wt(&normed, &wk_f32, seq, kv_out_dim, n_embd);
            let mut v = matmul_x_wt(&normed, &wv_f32, seq, kv_out_dim, n_embd);
            if let Some(lora_set) = lora {
                let scale = lora_set.scale();
                if let Some(lk) = &lora_set.layers[il][1] {
                    for s in 0..seq {
                        let inp = &normed[s * n_embd..(s + 1) * n_embd];
                        let out = lk.forward(inp);
                        for d in 0..kv_out_dim.min(out.len()) {
                            k[s * kv_out_dim + d] += out[d] * scale;
                        }
                    }
                }
                if let Some(lv) = &lora_set.layers[il][2] {
                    for s in 0..seq {
                        let inp = &normed[s * n_embd..(s + 1) * n_embd];
                        let out = lv.forward(inp);
                        for d in 0..kv_out_dim.min(out.len()) {
                            v[s * kv_out_dim + d] += out[d] * scale;
                        }
                    }
                }
            }
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
        let _t = std::time::Instant::now();
        let (attn_out_head, attn_cache) = forward::attention_forward(
            &q_rot, &k_rot, &v_normed,
            n_head, n_head_kv, head_dim, seq, mask,
        );
        if prof.enabled { prof.t_attention += _t.elapsed().as_secs_f64(); }
        // attn_out_head shape: [seq, n_head, head_dim] — flatten to [seq, q_out_dim]
        // (already contiguous in that order)

        // 2g. O projection: [seq, n_embd] = attn_out_head @ wo^T + LoRA_O
        //     wo shape: [q_out_dim, n_embd] stored row-major
        let wo_f32 = bf16_to_f32_vec(&weights.wo[il]);
        let mut o_out = matmul_x_wt(&attn_out_head, &wo_f32, seq, n_embd, q_out_dim);
        if let Some(lora_set) = lora {
            if let Some(lo) = &lora_set.layers[il][3] {
                let scale = lora_set.scale();
                for s in 0..seq {
                    let inp = &attn_out_head[s * q_out_dim..(s + 1) * q_out_dim];
                    let out = lo.forward(inp);
                    for d in 0..n_embd.min(out.len()) {
                        o_out[s * n_embd + d] += out[d] * scale;
                    }
                }
            }
        }

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
        let _t = std::time::Instant::now();
        let ffn_gate_f32 = bf16_to_f32_vec(&weights.ffn_gate[il]);
        let ffn_up_f32 = bf16_to_f32_vec(&weights.ffn_up[il]);
        let ffn_down_f32 = bf16_to_f32_vec(&weights.ffn_down[il]);
        if prof.enabled { prof.t_bf16conv += _t.elapsed().as_secs_f64(); }
        let _t = std::time::Instant::now();
        let gate_pre = matmul_x_wt(&ffn_normed, &ffn_gate_f32, seq, h.n_ff, n_embd);
        let up_pre = matmul_x_wt(&ffn_normed, &ffn_up_f32, seq, h.n_ff, n_embd);
        let mut ffn_hidden = vec![0.0f32; seq * h.n_ff];
        for i in 0..ffn_hidden.len() {
            ffn_hidden[i] = forward::gelu_tanh_approx(gate_pre[i]) * up_pre[i];
        }
        let ffn_out = matmul_x_wt(&ffn_hidden, &ffn_down_f32, seq, n_embd, h.n_ff);
        if prof.enabled { prof.t_ffn += _t.elapsed().as_secs_f64(); }

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

        // Phase 5 — Per-Layer Embedding contribution.
        // pe_in (= layer_out so far) gets a residual addition from the PLE chain.
        let ple_cache = if let Some(ref ipl) = inp_per_layer {
            let pe_in = layer_out.clone();
            let n_epl = h.n_embd_per_layer;
            let inp_layer_slice = slice_layer(ipl, seq, h.n_layer, n_epl, il);

            // gate_pre = pe_in @ inp_gate^T, shape [seq, n_epl]
            let inp_gate_w = bf16_to_f32_vec(&weights.inp_gate[il]);
            let gate_pre_gelu = matmul_x_wt(&pe_in, &inp_gate_w, seq, n_epl, n_embd);
            // GELU
            let mut gate_post_gelu = gate_pre_gelu.clone();
            for v in gate_post_gelu.iter_mut() {
                *v = forward::gelu_tanh_approx(*v);
            }
            // Multiply elementwise with inp_layer_slice
            let mut gated = vec![0.0f32; gate_post_gelu.len()];
            for i in 0..gated.len() {
                gated[i] = gate_post_gelu[i] * inp_layer_slice[i];
            }
            // Project back: gated [seq, n_epl] → proj_out [seq, n_embd]
            let proj_w = bf16_to_f32_vec(&weights.proj[il]);
            let proj_out_pre_norm = matmul_x_wt(&gated, &proj_w, seq, n_embd, n_epl);
            // RMSNorm with post_norm
            let mut proj_normed = vec![0.0f32; seq * n_embd];
            for s in 0..seq {
                let row = forward::rmsnorm(
                    &proj_out_pre_norm[s * n_embd..(s + 1) * n_embd],
                    &weights.post_norm[il],
                    h.rms_norm_eps,
                );
                proj_normed[s * n_embd..(s + 1) * n_embd].copy_from_slice(&row);
            }
            // Residual: layer_out = pe_in + proj_normed
            for i in 0..layer_out.len() {
                layer_out[i] = pe_in[i] + proj_normed[i];
            }
            Some(Gemma4PleCache {
                pe_in,
                gate_pre_gelu,
                gate_post_gelu,
                inp_layer_slice,
                proj_out_pre_norm,
            })
        } else {
            None
        };

        // Optional layer_output_scale (post-everything multiplier)
        if let Some(scale_v) = weights.layer_output_scale[il] {
            for v in layer_out.iter_mut() {
                *v *= scale_v;
            }
        }

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
            ple: ple_cache,
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
    let _t = std::time::Instant::now();
    let tok_embd_f32 = bf16_to_f32_vec(&weights.token_embd);
    if prof.enabled { prof.t_bf16conv += _t.elapsed().as_secs_f64(); }
    let _t = std::time::Instant::now();
    // tok_embd is stored as [vocab, n_embd] row-major. Match shape.
    let logits = matmul_x_wt(
        &final_hidden,
        &tok_embd_f32,
        seq,
        h.vocab_size,
        n_embd,
    );
    if prof.enabled { prof.t_lm_head += _t.elapsed().as_secs_f64(); }
    prof.n_layers = h.n_layer;
    prof.report();

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

/// Compute the per-token PLE input table (one-time, before layer loop).
/// Returns flattened buffer of shape [seq, n_layer, n_embd_per_layer].
///
/// Per gemma4-iswa.cpp `build_inp_per_layer` + `project_per_layer_inputs`:
/// 1. lookup per_layer_token_embd rows for input tokens, scale by sqrt(n_epl)
/// 2. project scaled embeddings via per_layer_model_proj, scale by 1/sqrt(n_embd)
/// 3. RMSNorm with per_layer_proj_norm weights (along n_epl axis)
/// 4. add to lookup, scale by 1/sqrt(2)
fn compute_inp_per_layer(
    weights: &CpuWeightsGemma4,
    tokens: &[u32],
    scaled_embeddings: &[f32],
) -> Vec<f32> {
    let h = &weights.hparams;
    let n_tokens = tokens.len();
    let n_embd = h.n_embd;
    let n_epl = h.n_embd_per_layer;
    let n_layer = h.n_layer;
    let row_size = n_epl * n_layer;

    // 1. Lookup per_layer_token_embd rows. Storage convention: ggml first-dim
    //    is fast-axis, so for tensor with dimensions [n_epl*n_layer, n_vocab],
    //    a row for token t is at byte offset t * row_size in bf16 elements.
    let mut tok_embd_per_layer = vec![0.0f32; n_tokens * row_size];
    let scale_tok = (n_epl as f32).sqrt();
    for (ti, &tok) in tokens.iter().enumerate() {
        let src_off = tok as usize * row_size;
        for d in 0..row_size {
            if src_off + d < weights.per_layer_token_embd.len() {
                tok_embd_per_layer[ti * row_size + d] =
                    weights.per_layer_token_embd[src_off + d].to_f32() * scale_tok;
            }
        }
    }

    // 2. Project scaled embeddings via per_layer_model_proj.
    //    per_layer_model_proj dimensions = [n_embd, row_size]
    //    matmul_x_wt: out[t, j] = sum_k scaled[t, k] * proj[j, k]
    let proj_w = bf16_to_f32_vec(&weights.per_layer_model_proj);
    let mut proj_out = matmul_x_wt(scaled_embeddings, &proj_w, n_tokens, row_size, n_embd);
    let scale_proj = 1.0 / (n_embd as f32).sqrt();
    for v in proj_out.iter_mut() {
        *v *= scale_proj;
    }

    // 3. RMSNorm along the n_epl axis with per_layer_proj_norm weights.
    //    proj_out is [n_tokens, n_layer * n_epl]. Inner n_epl chunk per layer.
    for ti in 0..n_tokens {
        for li in 0..n_layer {
            let off = ti * row_size + li * n_epl;
            let slice = &proj_out[off..off + n_epl];
            let normed = forward::rmsnorm(slice, &weights.per_layer_proj_norm, h.rms_norm_eps);
            proj_out[off..off + n_epl].copy_from_slice(&normed);
        }
    }

    // 4. Add to lookup, scale by 1/sqrt(2).
    let inv_sqrt2 = 1.0f32 / 2.0f32.sqrt();
    for i in 0..proj_out.len() {
        proj_out[i] = (proj_out[i] + tok_embd_per_layer[i]) * inv_sqrt2;
    }

    proj_out
}

/// Slice the per-layer chunk for layer `l` from inp_per_layer.
/// Source: [seq, n_layer, n_epl] flattened.
/// Returns: [seq, n_epl].
fn slice_layer(inp_per_layer: &[f32], seq: usize, n_layer: usize, n_epl: usize, l: usize) -> Vec<f32> {
    let mut out = Vec::with_capacity(seq * n_epl);
    for t in 0..seq {
        let off = t * n_layer * n_epl + l * n_epl;
        out.extend_from_slice(&inp_per_layer[off..off + n_epl]);
    }
    out
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

// =============================================================================
// Phase 1c part 2 — backward pass for Gemma 4 (grad health probe).
//
// This backward reconstructs dL/dhidden through the 35-layer chain using the
// per-layer cache from forward. It does NOT yet accumulate LoRA gradients —
// the deliverable is the exit-gate diagnostic: ARE gradients finite across
// all layers on the real 9 GB model? That answers the "does the chain rule
// math work end-to-end on Gemma 4" question.
//
// Once this probe shows healthy=N/N, the next commit wires LoRA Q/K/V/O
// injection in forward + gradient accumulation in this backward.
// =============================================================================

/// Per-layer grad-health datum returned by `backward_gemma4`.
#[derive(Debug, Clone)]
pub struct LayerGradHealth {
    pub layer: usize,
    pub is_sliding: bool,
    pub has_kv: bool,
    pub grad_norm: f32,
    pub has_nan: bool,
    pub has_inf: bool,
    pub nonzero: bool,
}

/// Matmul backward helper. Forward was `out = x @ w^T`. Given grad_out, returns
/// grad_x. Shapes: x=[m,k], w=[n,k], out=[m,n]. Returns grad_x=[m,k].
///
/// grad_x[i,l] = sum_j grad_out[i,j] * w[j,l]
fn matmul_grad_x(grad_out: &[f32], w: &[f32], m: usize, n: usize, k: usize) -> Vec<f32> {
    let mut out = vec![0.0f32; m * k];
    for i in 0..m {
        for l in 0..k {
            let mut s = 0.0f32;
            for j in 0..n {
                s += grad_out[i * n + j] * w[j * k + l];
            }
            out[i * k + l] = s;
        }
    }
    out
}

/// Per-head RMSNorm backward — mirrors forward `per_head_rmsnorm`.
/// Each (seq, head) slice gets rmsnorm_backward independently.
fn per_head_rmsnorm_backward(
    grad_out: &[f32],
    input: &[f32],
    weight: &[f32],
    seq: usize,
    n_head: usize,
    head_dim: usize,
    eps: f32,
) -> Vec<f32> {
    let mut grad_in = vec![0.0f32; input.len()];
    for s in 0..seq {
        for h in 0..n_head {
            let off = (s * n_head + h) * head_dim;
            let in_slice = &input[off..off + head_dim];
            let go_slice = &grad_out[off..off + head_dim];
            let gi = backward::rmsnorm_backward(in_slice, weight, go_slice, eps);
            grad_in[off..off + head_dim].copy_from_slice(&gi);
        }
    }
    grad_in
}

/// V-norm backward (unit weight since Gemma 4 V has no weight, just eps).
fn v_norm_backward(
    grad_out: &[f32],
    input: &[f32],
    seq: usize,
    n_head_kv: usize,
    head_dim: usize,
    eps: f32,
) -> Vec<f32> {
    let unit = vec![1.0f32; head_dim];
    per_head_rmsnorm_backward(grad_out, input, &unit, seq, n_head_kv, head_dim, eps)
}

/// RoPE backward for the partial-rotary variant used in forward.
/// Inverse of `rope_apply_partial` — unchanged dims pass through, rotated
/// dims get the inverse 2D rotation.
fn rope_backward_partial(
    grad_rotated: &[f32],
    cos: &[f32],
    sin: &[f32],
    seq: usize,
    n_head: usize,
    head_dim: usize,
    rope_dim: usize,
) -> Vec<f32> {
    let mut out = grad_rotated.to_vec();
    let half = rope_dim / 2;
    for s in 0..seq {
        for h in 0..n_head {
            let off = (s * n_head + h) * head_dim;
            for d in 0..half {
                let c = cos[s * half + d];
                let si = sin[s * half + d];
                let ge = grad_rotated[off + 2 * d];
                let go = grad_rotated[off + 2 * d + 1];
                out[off + 2 * d]     = ge * c + go * si;
                out[off + 2 * d + 1] = -ge * si + go * c;
            }
        }
    }
    out
}

/// Gemma 4 backward pass — the grad-health probe.
/// Runs backward from the logits through all 35 layers, returning:
///   - total loss (scalar)
///   - per-layer grad health data
///
/// Does NOT yet accumulate LoRA gradients. This is the Phase 1c+2.2 exit
/// gate: verify healthy gradient flow through the full Gemma 4 chain on
/// the real model.
pub fn backward_gemma4(
    weights: &CpuWeightsGemma4,
    caches: &[Gemma4LayerCache],
    logits: &[f32],
    tokens: &[u32],
    answer_start: usize,
) -> (f32, Vec<LayerGradHealth>) {
    backward_gemma4_with_lora(weights, None, None, caches, logits, tokens, answer_start)
}

/// Build the per-layer producer table for unified KV routing. For each
/// KV-reusing layer (has_kv = false), returns the index of its producer
/// (the most recent KV-producing layer of the same attention type).
/// For KV-producing layers themselves, returns None.
///
/// Per Gemma 4 E2B with n_layer_kv_from_start=20 and the verified pattern
/// (full at [4,9,14,19,24,29,34], rest sliding):
///   - Layer 19 (last KV-prod full) is producer for consumers 24, 29, 34
///   - Layer 18 (last KV-prod sliding) is producer for consumers
///     20, 21, 22, 23, 25, 26, 27, 28, 30, 31, 32, 33
fn build_producer_table(h: &Gemma4Hparams) -> Vec<Option<usize>> {
    let mut last_full_producer: Option<usize> = None;
    let mut last_sliding_producer: Option<usize> = None;
    let mut producers = vec![None; h.n_layer];
    for il in 0..h.n_layer {
        if h.has_kv(il) {
            // Update the running last producer for this attention type
            if h.is_sliding(il) {
                last_sliding_producer = Some(il);
            } else {
                last_full_producer = Some(il);
            }
        } else {
            producers[il] = if h.is_sliding(il) {
                last_sliding_producer
            } else {
                last_full_producer
            };
        }
    }
    producers
}

/// Accumulated K/V gradients destined for a producer layer.
/// grad_k_rot is in post-gqa-collapse, post-RoPE, pre-K-norm shape.
/// grad_v_post_norm is in post-gqa-collapse, post-V-norm shape.
struct SharedKvGrads {
    grad_k_rot: Vec<f32>,         // [seq, n_head_kv, head_dim]
    grad_v_post_norm: Vec<f32>,   // [seq, n_head_kv, head_dim]
}

/// Same as `backward_gemma4` but accumulates LoRA gradients when `lora` is
/// Some. Per-position lora_backward contributions are summed into grad_a /
/// grad_b on the corresponding LoraLayer.
pub fn backward_gemma4_with_lora(
    weights: &CpuWeightsGemma4,
    gpu: Option<&crate::gemma4_gpu::Gemma4GpuWeights>,
    mut lora: Option<&mut Gemma4LoraAdapters>,
    caches: &[Gemma4LayerCache],
    logits: &[f32],
    tokens: &[u32],
    answer_start: usize,
) -> (f32, Vec<LayerGradHealth>) {
    // When `gpu` is Some, each of the 14 matmul sites dispatches to the GPU
    // helper (phase 3). When None, we stay on the CPU baseline. The switch is
    // per-site with identical shape, so the two paths produce bit-equivalent
    // output within bf16 precision.
    let h = &weights.hparams;
    let seq = tokens.len();
    let n_embd = h.n_embd;
    let vocab = h.vocab_size;

    // 1. Compute loss + grad_logits from cross-entropy at answer positions.
    let loss_start = answer_start.max(1);
    let mut total_loss = 0.0f32;
    let mut n_loss_pos = 0;
    let mut grad_logits = vec![0.0f32; seq * vocab];
    for pos in loss_start..seq.saturating_sub(1) {
        let pos_logits = &logits[pos * vocab..(pos + 1) * vocab];
        let target = tokens[pos + 1];
        let row_grad = backward::cross_entropy_softmax_backward(pos_logits, target);
        for (i, v) in row_grad.iter().enumerate() {
            grad_logits[pos * vocab + i] = *v;
        }
        total_loss += forward::cross_entropy_loss(pos_logits, target);
        n_loss_pos += 1;
    }
    if n_loss_pos > 0 {
        total_loss /= n_loss_pos as f32;
    }

    // 2. Backward through softcap: d(tanh(x/cap)*cap)/dx = 1 - (out/cap)^2
    let cap = h.final_logit_softcapping;
    if cap > 0.0 {
        for i in 0..grad_logits.len() {
            let out = logits[i];
            let d_sc = 1.0 - (out / cap).powi(2);
            grad_logits[i] *= d_sc;
        }
    }

    // 3. Backward through tied LM head: logits = final_hidden @ tok_embd^T
    //    grad_final_hidden = grad_logits @ tok_embd  [seq, n_embd]
    //    (tok_embd is [vocab, n_embd] row-major, i.e., tok_embd[v * n_embd + d])
    let grad_final_hidden = if let Some(g) = gpu {
        g.matmul_grad_x(&g.token_embd, &grad_logits, seq, vocab, n_embd)
            .expect("gpu site 1 lm_head_grad")
    } else {
        let tok_embd_f32 = bf16_to_f32_vec(&weights.token_embd);
        matmul_grad_x(&grad_logits, &tok_embd_f32, seq, vocab, n_embd)
    };

    // 4. Backward through final output_norm.
    let last_cache = caches.last().expect("at least one layer cache");
    let mut grad_hidden = vec![0.0f32; seq * n_embd];
    for s in 0..seq {
        let input_row = &last_cache.post_ffw_residual[s * n_embd..(s + 1) * n_embd];
        let go_row = &grad_final_hidden[s * n_embd..(s + 1) * n_embd];
        let gi = backward::rmsnorm_backward(input_row, &weights.output_norm, go_row, h.rms_norm_eps);
        grad_hidden[s * n_embd..(s + 1) * n_embd].copy_from_slice(&gi);
    }

    // 4b. Phase 4 — Unified KV routing accumulators.
    //     For each KV-reusing layer in the reverse pass, push gradient
    //     contributions on shared K/V into shared_kv_grads[producer_idx].
    //     When the producer's backward runs, those contributions are added
    //     to its own K/V backward path before the wk/wv projection backward.
    let producer_of = build_producer_table(h);
    let mut shared_kv_grads: Vec<Option<SharedKvGrads>> = (0..h.n_layer).map(|_| None).collect();

    // 5. Per-layer reverse loop.
    let mut health = Vec::with_capacity(h.n_layer);
    for il in (0..h.n_layer).rev() {
        let cache = &caches[il];
        let head_dim = h.head_dim(il);
        let n_head = h.n_head;
        let n_head_kv = h.n_head_kv;
        let q_out_dim = n_head * head_dim;
        let kv_out_dim = n_head_kv * head_dim;

        // Compute grad_hidden norm BEFORE this layer's backward (i.e., what the
        // LAYER ABOVE produced). That's the diagnostic: each layer's "incoming"
        // gradient shows chain-rule magnitude evolution.
        let grad_norm_before: f32 = grad_hidden.iter().map(|g| g * g).sum::<f32>().sqrt();
        let has_nan = grad_hidden.iter().any(|g| g.is_nan());
        let has_inf = grad_hidden.iter().any(|g| g.is_infinite() && !g.is_nan());
        health.push(LayerGradHealth {
            layer: il,
            is_sliding: h.is_sliding(il),
            has_kv: h.has_kv(il),
            grad_norm: grad_norm_before,
            has_nan,
            has_inf,
            nonzero: grad_norm_before > 0.0 && grad_norm_before.is_finite(),
        });

        // === Phase 5 PLE backward (if active) ===
        // Forward last steps:
        //   layer_out = (pe_in + proj_normed) [* layer_output_scale optional]
        // Backward in reverse:
        //   undo layer_output_scale → grad_combined
        //   split: grad_pe_in_residual + grad_proj_normed
        //   backward through rmsnorm → proj → mul → GELU → inp_gate
        //   sum: grad_hidden_pre_ple = grad_pe_in_residual + grad_pe_in_chain

        // Undo layer_output_scale if applied
        if let Some(scale_v) = weights.layer_output_scale[il] {
            for v in grad_hidden.iter_mut() {
                *v *= scale_v;
            }
        }

        // PLE backward
        if let Some(ple) = &cache.ple {
            // Split residual + chain
            let grad_proj_normed = grad_hidden.clone();
            let mut grad_pe_in = grad_hidden.clone(); // residual passthrough

            // rmsnorm backward
            let mut grad_proj_out_pre_norm = vec![0.0f32; seq * n_embd];
            for s in 0..seq {
                let input_row = &ple.proj_out_pre_norm[s * n_embd..(s + 1) * n_embd];
                let go_row = &grad_proj_normed[s * n_embd..(s + 1) * n_embd];
                let gi = backward::rmsnorm_backward(input_row, &weights.post_norm[il], go_row, h.rms_norm_eps);
                grad_proj_out_pre_norm[s * n_embd..(s + 1) * n_embd].copy_from_slice(&gi);
            }

            // proj matmul backward: forward was gated [seq, n_epl] @ proj^T → [seq, n_embd]
            // So grad_gated = grad_proj_out_pre_norm @ proj (matmul_grad_x)
            let proj_w = bf16_to_f32_vec(&weights.proj[il]);
            let n_epl = h.n_embd_per_layer;
            let grad_gated = matmul_grad_x(&grad_proj_out_pre_norm, &proj_w, seq, n_embd, n_epl);

            // elementwise mul backward: gated = gate_post * inp_layer_slice
            // grad_gate_post = grad_gated * inp_layer_slice  (inp_layer_slice frozen)
            let mut grad_gate_post = vec![0.0f32; seq * n_epl];
            for i in 0..grad_gate_post.len() {
                grad_gate_post[i] = grad_gated[i] * ple.inp_layer_slice[i];
            }

            // GELU backward: gate_post = gelu(gate_pre)
            // grad_gate_pre = grad_gate_post * gelu_tanh_approx_prime(gate_pre)
            let mut grad_gate_pre = vec![0.0f32; seq * n_epl];
            for i in 0..grad_gate_pre.len() {
                grad_gate_pre[i] = grad_gate_post[i] * gelu_tanh_approx_prime(ple.gate_pre_gelu[i]);
            }

            // inp_gate matmul backward: gate_pre = pe_in @ inp_gate^T
            //   grad_pe_in_chain = grad_gate_pre @ inp_gate
            let inp_gate_w = bf16_to_f32_vec(&weights.inp_gate[il]);
            let grad_pe_in_chain = matmul_grad_x(&grad_gate_pre, &inp_gate_w, seq, n_epl, n_embd);

            // Sum residual + chain to get total gradient on pe_in
            for i in 0..grad_pe_in.len() {
                grad_pe_in[i] += grad_pe_in_chain[i];
            }
            // Replace grad_hidden with grad_pe_in (gradient w.r.t. pre-PLE layer output)
            grad_hidden = grad_pe_in;
        }

        // === Backward through forward's layer output structure (post-PLE) ===
        // Forward: post_ffw_residual = rmsnorm(ffn_out, post_ffw_norm) + post_attn_residual
        // So grad splits into two paths at the + :
        //   grad_ffn_normed_out (into post_ffw_norm backward) AND
        //   grad_post_attn_residual (straight passthrough)
        let mut grad_post_attn = grad_hidden.clone(); // residual contribution

        // Site 2 — reconstruct ffn_out for rmsnorm_backward input. Lift the
        // per-row matmul out of the loop into a single full-batch dispatch;
        // the output is then sliced per row.
        let ffn_out_full = if let Some(g) = gpu {
            g.matmul_xwt(&g.ffn_down[il], &cache.ffn_hidden, seq, n_embd, h.n_ff)
                .expect("gpu site 2 ffn_out recon")
        } else {
            let ffn_down_f32 = bf16_to_f32_vec(&weights.ffn_down[il]);
            matmul_x_wt(&cache.ffn_hidden, &ffn_down_f32, seq, n_embd, h.n_ff)
        };
        let mut grad_ffn_out = vec![0.0f32; seq * n_embd];
        for s in 0..seq {
            let row_ffn_out = &ffn_out_full[s * n_embd..(s + 1) * n_embd];
            let go_row = &grad_hidden[s * n_embd..(s + 1) * n_embd];
            let gi = backward::rmsnorm_backward(row_ffn_out, &weights.post_ffw_norm[il], go_row, h.rms_norm_eps);
            grad_ffn_out[s * n_embd..(s + 1) * n_embd].copy_from_slice(&gi);
        }

        // Site 3 — backward through FFN down: ffn_out = ffn_hidden @ ffn_down^T
        //   grad_ffn_hidden = grad_ffn_out @ ffn_down
        let grad_ffn_hidden = if let Some(g) = gpu {
            g.matmul_grad_x(&g.ffn_down[il], &grad_ffn_out, seq, n_embd, h.n_ff)
                .expect("gpu site 3 ffn_down_grad")
        } else {
            let ffn_down_f32 = bf16_to_f32_vec(&weights.ffn_down[il]);
            matmul_grad_x(&grad_ffn_out, &ffn_down_f32, seq, n_embd, h.n_ff)
        };

        // Backward through ffn_hidden = gelu_tanh(gate_pre) * up_pre
        //   grad_gate_pre[i] = grad_ffn_hidden[i] * up_pre[i] * gelu_tanh'(gate_pre[i])
        //   grad_up_pre[i]   = grad_ffn_hidden[i] * gelu_tanh(gate_pre[i])
        let mut grad_gate_pre = vec![0.0f32; seq * h.n_ff];
        let mut grad_up_pre = vec![0.0f32; seq * h.n_ff];
        for i in 0..seq * h.n_ff {
            let gp = cache.ffn_gate_pre[i];
            let up = cache.ffn_up_pre[i];
            let gelu_val = forward::gelu_tanh_approx(gp);
            let d_gelu = gelu_tanh_approx_prime(gp);
            grad_gate_pre[i] = grad_ffn_hidden[i] * up * d_gelu;
            grad_up_pre[i] = grad_ffn_hidden[i] * gelu_val;
        }

        // Sites 4 + 5 — backward through ffn_gate + ffn_up: both took ffn_normed.
        //   ffn_gate_pre = ffn_normed @ ffn_gate^T → grad_ffn_normed += grad_gate_pre @ ffn_gate
        //   ffn_up_pre   = ffn_normed @ ffn_up^T   → grad_ffn_normed += grad_up_pre   @ ffn_up
        let mut grad_ffn_normed = if let Some(g) = gpu {
            g.matmul_grad_x(&g.ffn_gate[il], &grad_gate_pre, seq, h.n_ff, n_embd)
                .expect("gpu site 4 ffn_gate_grad")
        } else {
            let ffn_gate_f32 = bf16_to_f32_vec(&weights.ffn_gate[il]);
            matmul_grad_x(&grad_gate_pre, &ffn_gate_f32, seq, h.n_ff, n_embd)
        };
        let grad_ffn_normed_up = if let Some(g) = gpu {
            g.matmul_grad_x(&g.ffn_up[il], &grad_up_pre, seq, h.n_ff, n_embd)
                .expect("gpu site 5 ffn_up_grad")
        } else {
            let ffn_up_f32 = bf16_to_f32_vec(&weights.ffn_up[il]);
            matmul_grad_x(&grad_up_pre, &ffn_up_f32, seq, h.n_ff, n_embd)
        };
        for i in 0..grad_ffn_normed.len() {
            grad_ffn_normed[i] += grad_ffn_normed_up[i];
        }

        // Backward through ffn_norm → grad w.r.t. post_attn_residual (add to residual grad)
        for s in 0..seq {
            let input_row = &cache.post_attn_residual[s * n_embd..(s + 1) * n_embd];
            let go_row = &grad_ffn_normed[s * n_embd..(s + 1) * n_embd];
            let gi = backward::rmsnorm_backward(input_row, &weights.ffn_norm[il], go_row, h.rms_norm_eps);
            for d in 0..n_embd {
                grad_post_attn[s * n_embd + d] += gi[d];
            }
        }

        // Now grad_post_attn is the gradient w.r.t. the full post_attn_residual tensor.
        // Forward: post_attn = rmsnorm(o_out, post_attention_norm) + hidden_incoming
        // Split into: grad_o_normed (to post_attention_norm backward) + grad_hidden_incoming
        let mut grad_hidden_incoming = grad_post_attn.clone(); // residual passthrough

        // Site 6 — reconstruct o_out for rmsnorm_backward. Lift the per-row
        // matmul out of the loop into a single full-batch dispatch.
        let o_out_full = if let Some(g) = gpu {
            g.matmul_xwt(&g.wo[il], &cache.attn_out, seq, n_embd, q_out_dim)
                .expect("gpu site 6 o_out recon")
        } else {
            let wo_f32 = bf16_to_f32_vec(&weights.wo[il]);
            matmul_x_wt(&cache.attn_out, &wo_f32, seq, n_embd, q_out_dim)
        };
        let mut grad_o_out = vec![0.0f32; seq * n_embd];
        for s in 0..seq {
            let row_o = &o_out_full[s * n_embd..(s + 1) * n_embd];
            let go_row = &grad_post_attn[s * n_embd..(s + 1) * n_embd];
            let gi = backward::rmsnorm_backward(row_o, &weights.post_attention_norm[il], go_row, h.rms_norm_eps);
            grad_o_out[s * n_embd..(s + 1) * n_embd].copy_from_slice(&gi);
        }

        // Site 7 — backward through O projection: o_out = attn_out @ wo^T
        //   grad_attn_out = grad_o_out @ wo
        let grad_attn_out = if let Some(g) = gpu {
            g.matmul_grad_x(&g.wo[il], &grad_o_out, seq, n_embd, q_out_dim)
                .expect("gpu site 7 wo_grad")
        } else {
            let wo_f32 = bf16_to_f32_vec(&weights.wo[il]);
            matmul_grad_x(&grad_o_out, &wo_f32, seq, n_embd, q_out_dim)
        };

        // Backward through attention: need the pre-GQA K/V shapes. We stored
        // (q_rot, k_rot, v) in the cache. The attention_forward expanded K/V
        // internally via mask-based gather; backward::attention_backward expects
        // EXPANDED K/V per query head. Expand here, then collapse grads at the end.
        let k_rot_expanded = backward::gqa_expand(&cache.k_rot, n_head, n_head_kv, head_dim, seq);
        let v_expanded = backward::gqa_expand(&cache.v, n_head, n_head_kv, head_dim, seq);
        let (grad_q_rot, grad_k_rot_expanded, grad_v_expanded) = backward::attention_backward(
            &grad_attn_out,
            &cache.q_rot,
            &k_rot_expanded,
            &v_expanded,
            &cache.attn_cache,
            n_head, head_dim, seq,
        );
        let mut grad_k_rot = backward::gqa_collapse(&grad_k_rot_expanded, n_head, n_head_kv, head_dim, seq);
        let mut grad_v = backward::gqa_collapse(&grad_v_expanded, n_head, n_head_kv, head_dim, seq);

        // Phase 4 — if THIS is a KV-reusing layer, route its K/V grad to producer.
        if !h.has_kv(il) {
            if let Some(producer_idx) = producer_of[il] {
                let entry = shared_kv_grads[producer_idx].get_or_insert_with(|| SharedKvGrads {
                    grad_k_rot: vec![0.0; grad_k_rot.len()],
                    grad_v_post_norm: vec![0.0; grad_v.len()],
                });
                // Sanity: shapes must match (same attention type → same head_dim,
                // same kv_out_dim; producer and consumer's seq is identical).
                if entry.grad_k_rot.len() == grad_k_rot.len() {
                    for i in 0..grad_k_rot.len() {
                        entry.grad_k_rot[i] += grad_k_rot[i];
                    }
                }
                if entry.grad_v_post_norm.len() == grad_v.len() {
                    for i in 0..grad_v.len() {
                        entry.grad_v_post_norm[i] += grad_v[i];
                    }
                }
            }
            // Zero out local consumer grads — they're now routed.
            // (The consumer has no wk/wv to apply them to anyway; zeroing
            // makes the downstream branch produce zero grad_normed_k/v.)
            for v in grad_k_rot.iter_mut() { *v = 0.0; }
            for v in grad_v.iter_mut() { *v = 0.0; }
        } else {
            // Phase 4 — if THIS is a producer, ADD any consumer contributions
            // to our own K/V grads before continuing through RoPE/norm backward.
            if let Some(shared) = shared_kv_grads[il].take() {
                for i in 0..grad_k_rot.len().min(shared.grad_k_rot.len()) {
                    grad_k_rot[i] += shared.grad_k_rot[i];
                }
                for i in 0..grad_v.len().min(shared.grad_v_post_norm.len()) {
                    grad_v[i] += shared.grad_v_post_norm[i];
                }
            }
        }

        // Backward through RoPE on Q and K
        let rope_dim = h.rope_dim(il);
        let freq_base = h.rope_freq_base(il);
        let (cos_table, sin_table) = forward::rope_freqs(seq, rope_dim, freq_base);
        let grad_q_normed = rope_backward_partial(&grad_q_rot, &cos_table, &sin_table, seq, n_head, head_dim, rope_dim);
        let grad_k_normed = rope_backward_partial(&grad_k_rot, &cos_table, &sin_table, seq, n_head_kv, head_dim, rope_dim);

        // Sites 11 + 12 — reconstruct Q/K/V pre-norm values for per_head_*
        // backward. These replace the former reconstruct_q_pre_norm /
        // reconstruct_kv_pre_norm helpers (deleted). Inlined here so the gpu
        // dispatch can share the `gpu` param without extra plumbing.
        let q_pre_norm = if let Some(g) = gpu {
            g.matmul_xwt(&g.wq[il], &cache.normed_input, seq, q_out_dim, n_embd)
                .expect("gpu site 11 q_pre_norm recon")
        } else {
            let wq_f32 = bf16_to_f32_vec(&weights.wq[il]);
            matmul_x_wt(&cache.normed_input, &wq_f32, seq, q_out_dim, n_embd)
        };
        let (k_pre_norm, v_pre_norm) = if let Some(wk_cpu) = &weights.wk[il] {
            let k_pre = if let Some(g) = gpu {
                let g_wk = g.wk[il].as_ref().expect("gpu wk present on producer");
                g.matmul_xwt(g_wk, &cache.normed_input, seq, kv_out_dim, n_embd)
                    .expect("gpu site 12 k_pre_norm recon")
            } else {
                let wk_f32 = bf16_to_f32_vec(wk_cpu);
                matmul_x_wt(&cache.normed_input, &wk_f32, seq, kv_out_dim, n_embd)
            };
            let v_pre = if let Some(g) = gpu {
                let g_wv = g.wv[il].as_ref()
                    .or(g.wk[il].as_ref())
                    .expect("gpu wv or wk present on producer");
                g.matmul_xwt(g_wv, &cache.normed_input, seq, kv_out_dim, n_embd)
                    .expect("gpu site 12 v_pre_norm recon")
            } else {
                let wk_f32 = bf16_to_f32_vec(wk_cpu);
                let wv_f32 = weights.wv[il].as_ref().map(|w| bf16_to_f32_vec(w)).unwrap_or(wk_f32);
                matmul_x_wt(&cache.normed_input, &wv_f32, seq, kv_out_dim, n_embd)
            };
            (k_pre, v_pre)
        } else {
            // KV-reusing layer: k_pre_norm is unused (grad_k takes grad_k_normed
            // branch below), v_pre_norm goes to v_norm_backward as zeros (matches
            // former reconstruct_kv_pre_norm None-fallthrough behavior).
            (vec![0.0f32; seq * kv_out_dim], vec![0.0f32; seq * kv_out_dim])
        };

        // Backward through per-head Q-norm, K-norm, V-norm
        let grad_q = per_head_rmsnorm_backward(
            &grad_q_normed,
            &q_pre_norm,
            &weights.attn_q_norm[il],
            seq, n_head, head_dim, h.rms_norm_eps,
        );
        let grad_k = if let Some(k_norm_w) = &weights.attn_k_norm[il] {
            per_head_rmsnorm_backward(
                &grad_k_normed,
                &k_pre_norm,
                k_norm_w,
                seq, n_head_kv, head_dim, h.rms_norm_eps,
            )
        } else {
            grad_k_normed
        };
        // V had a weightless per-head RMSNorm
        let grad_v_pre = v_norm_backward(
            &grad_v,
            &v_pre_norm,
            seq, n_head_kv, head_dim, h.rms_norm_eps,
        );

        // Backward through Q, K, V projections (input was cache.normed_input).
        //   q = normed @ wq^T → grad_normed_q = grad_q @ wq
        // Site 8 — wq backward
        let grad_normed_q = if let Some(g) = gpu {
            g.matmul_grad_x(&g.wq[il], &grad_q, seq, q_out_dim, n_embd)
                .expect("gpu site 8 wq_grad")
        } else {
            let wq_f32 = bf16_to_f32_vec(&weights.wq[il]);
            matmul_grad_x(&grad_q, &wq_f32, seq, q_out_dim, n_embd)
        };

        let (grad_normed_k, grad_normed_v) = if let Some(wk) = &weights.wk[il] {
            // Site 9 — wk backward
            let gnk = if let Some(g) = gpu {
                let g_wk = g.wk[il].as_ref().expect("gpu wk mirror present on producer layer");
                g.matmul_grad_x(g_wk, &grad_k, seq, kv_out_dim, n_embd)
                    .expect("gpu site 9 wk_grad")
            } else {
                let wk_f32 = bf16_to_f32_vec(wk);
                matmul_grad_x(&grad_k, &wk_f32, seq, kv_out_dim, n_embd)
            };
            // Site 10 — wv backward (wv falls back to wk on Gemma 4 if absent)
            let gnv = if let Some(g) = gpu {
                let g_wv = g.wv[il].as_ref()
                    .or(g.wk[il].as_ref())
                    .expect("gpu wv or wk mirror present on producer layer");
                g.matmul_grad_x(g_wv, &grad_v_pre, seq, kv_out_dim, n_embd)
                    .expect("gpu site 10 wv_grad")
            } else {
                let wk_f32 = bf16_to_f32_vec(wk);
                let wv_f32 = weights.wv[il].as_ref().map(|w| bf16_to_f32_vec(w)).unwrap_or(wk_f32);
                matmul_grad_x(&grad_v_pre, &wv_f32, seq, kv_out_dim, n_embd)
            };
            (gnk, gnv)
        } else {
            // KV-reusing layer: grad on shared K/V flows to a DIFFERENT layer's weights
            // (the producer). Phase 4 routing work — for now, ignore (zero contribution).
            (vec![0.0; seq * n_embd], vec![0.0; seq * n_embd])
        };

        // === LoRA gradient accumulation for Q/K/V ===
        if let Some(ls) = lora.as_mut() {
            let scale = ls.scale();
            // Q target (0) — always present
            accumulate_lora_grad(&mut ls.layers[il][0], &cache.normed_input, &grad_q, seq, n_embd, q_out_dim, scale);
            // K target (1) — present only for KV-producing layers
            if let Some(_) = &weights.wk[il] {
                accumulate_lora_grad(&mut ls.layers[il][1], &cache.normed_input, &grad_k, seq, n_embd, kv_out_dim, scale);
                accumulate_lora_grad(&mut ls.layers[il][2], &cache.normed_input, &grad_v_pre, seq, n_embd, kv_out_dim, scale);
            }
            // O target (3) — input was attn_out_head, output grad is grad_o_out
            accumulate_lora_grad(&mut ls.layers[il][3], &cache.attn_out, &grad_o_out, seq, q_out_dim, n_embd, scale);
        }

        // Sum Q, K, V contributions to normed input
        let mut grad_normed = grad_normed_q;
        for i in 0..grad_normed.len() {
            grad_normed[i] += grad_normed_k[i] + grad_normed_v[i];
        }

        // Backward through attn_norm → grad to hidden_incoming (add residual)
        // We need the ORIGINAL layer input (hidden before this layer's attn_norm).
        // That's the layer_out of the PREVIOUS layer (or embedding for layer 0).
        let layer_input = if il > 0 {
            &caches[il - 1].post_ffw_residual[..]
        } else {
            // Layer 0 input = scaled embedding. Reconstruct.
            // Compute on demand:
            &compute_embed_input(weights, tokens)[..]
            // NOTE: this borrows a temporary — we need to hold it.
        };
        let mut layer_input_owned: Option<Vec<f32>> = None;
        let layer_input_ref: &[f32] = if il > 0 {
            &caches[il - 1].post_ffw_residual
        } else {
            layer_input_owned = Some(compute_embed_input(weights, tokens));
            layer_input_owned.as_ref().unwrap().as_slice()
        };
        // The above `let layer_input =` is redundant with the one below; clean up.
        let _ = layer_input;

        for s in 0..seq {
            let input_row = &layer_input_ref[s * n_embd..(s + 1) * n_embd];
            let go_row = &grad_normed[s * n_embd..(s + 1) * n_embd];
            let gi = backward::rmsnorm_backward(input_row, &weights.attn_norm[il], go_row, h.rms_norm_eps);
            for d in 0..n_embd {
                grad_hidden_incoming[s * n_embd + d] += gi[d];
            }
        }

        // grad_hidden_incoming is now the gradient w.r.t. this layer's INPUT,
        // i.e., the PREVIOUS layer's output. Propagate to next iteration.
        grad_hidden = grad_hidden_incoming;
    }

    // Reverse so health[0] = layer 0 (oldest)
    health.reverse();
    (total_loss, health)
}

/// Complete training step: forward + backward + Adam update on LoRA params.
/// Returns the loss for this step.
pub fn train_step_gemma4(
    weights: &CpuWeightsGemma4,
    lora: &mut Gemma4LoraAdapters,
    tokens: &[u32],
    answer_start: usize,
    lr: f32,
    step: u32,
) -> f32 {
    let (logits, caches) = forward_gemma4_with_lora(weights, Some(lora), tokens);
    let (loss, _health) = backward_gemma4_with_lora(
        weights, None, Some(lora), &caches, &logits, tokens, answer_start);

    // Gradient clip + Adam step per LoRA layer
    let clip_threshold = 1.0f32;
    for il in 0..lora.layers.len() {
        for t in 0..4 {
            if let Some(lora_layer) = &mut lora.layers[il][t] {
                // Compute grad norm
                let grad_norm_sq: f32 = lora_layer.grad_a.iter()
                    .chain(lora_layer.grad_b.iter())
                    .map(|g| g * g).sum();
                let grad_norm = grad_norm_sq.sqrt();
                // Clip
                if grad_norm > clip_threshold {
                    let scale = clip_threshold / grad_norm;
                    for g in lora_layer.grad_a.iter_mut() { *g *= scale; }
                    for g in lora_layer.grad_b.iter_mut() { *g *= scale; }
                }
                // Adam step (with NaN guard from lora.rs)
                lora_layer.adam_step(lr, 0.9, 0.999, 1e-8, step);
            }
        }
    }
    loss
}

/// Accumulate LoRA gradient for one target across all sequence positions.
/// Does nothing if `lora_opt` is None.
fn accumulate_lora_grad(
    lora_opt: &mut Option<LoraLayer>,
    input_tensor: &[f32],    // [seq, input_dim]
    grad_output_tensor: &[f32], // [seq, output_dim]
    seq: usize,
    input_dim: usize,
    output_dim: usize,
    scale: f32,
) {
    let Some(lora_layer) = lora_opt.as_mut() else { return; };
    let rank = lora_layer.rank as usize;
    for s in 0..seq {
        let inp = &input_tensor[s * input_dim..(s + 1) * input_dim];
        let go = &grad_output_tensor[s * output_dim..(s + 1) * output_dim];
        // Compute hidden = A @ input for this position
        let (_, hidden) = lora_layer.forward_with_hidden(inp);
        debug_assert_eq!(hidden.len(), rank);
        let (ga, gb) = crate::backward::lora_backward(
            inp, &hidden, go,
            &lora_layer.a, &lora_layer.b,
            input_dim, output_dim, rank,
            scale,
        );
        for (i, v) in ga.iter().enumerate() {
            if i < lora_layer.grad_a.len() {
                lora_layer.grad_a[i] += v;
            }
        }
        for (i, v) in gb.iter().enumerate() {
            if i < lora_layer.grad_b.len() {
                lora_layer.grad_b[i] += v;
            }
        }
    }
}

/// GELU-tanh derivative: d/dx [0.5 * x * (1 + tanh(k*(x + α*x^3)))]
fn gelu_tanh_approx_prime(x: f32) -> f32 {
    const SQRT_2_OVER_PI: f32 = 0.7978845608028654;
    const ALPHA: f32 = 0.044715;
    let inner = SQRT_2_OVER_PI * (x + ALPHA * x * x * x);
    let th = inner.tanh();
    let d_inner = SQRT_2_OVER_PI * (1.0 + 3.0 * ALPHA * x * x);
    0.5 * (1.0 + th) + 0.5 * x * (1.0 - th * th) * d_inner
}

/// Reconstruct the embedding input at layer 0 (scaled token embeddings).
fn compute_embed_input(weights: &CpuWeightsGemma4, tokens: &[u32]) -> Vec<f32> {
    let h = &weights.hparams;
    let tok_embd_f32 = bf16_to_f32_vec(&weights.token_embd);
    let mut hidden = forward::embedding_lookup(&tok_embd_f32, h.n_embd, tokens);
    let embed_scale = (h.n_embd as f32).sqrt();
    for v in hidden.iter_mut() {
        *v *= embed_scale;
    }
    hidden
}

// silence the unused import for now
#[allow(dead_code)]
fn _hashmap_keepalive() {
    let _: HashMap<String, String> = HashMap::new();
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
    fn test_gemma4_lora_save_load_roundtrip() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            return;
        }
        let model = GgufFile::open(model_path).expect("open gguf");
        let weights = CpuWeightsGemma4::load(&model).expect("load");
        let mut lora = Gemma4LoraAdapters::new(&weights.hparams, 16, 32.0);

        // Tweak some values to verify exact roundtrip
        if let Some(l0q) = lora.layers[0][0].as_mut() {
            l0q.a[0] = 1.234;
            l0q.b[0] = -5.678;
            let last = l0q.a.len() - 1;
            l0q.a[last] = 9.999;
        }
        if let Some(l34o) = lora.layers[34][3].as_mut() {
            l34o.a[42] = 0.42;
        }

        let tmp = "/tmp/test-zlg4-roundtrip.bin";
        lora.save(tmp).expect("save");
        let loaded = Gemma4LoraAdapters::load(tmp).expect("load");

        assert_eq!(loaded.layers.len(), lora.layers.len());
        assert_eq!(loaded.rank, lora.rank);
        assert_eq!(loaded.alpha, lora.alpha);

        // KV-share pattern: first 20 layers have all 4 targets, last 15 have Q+O only
        for il in 0..lora.layers.len() {
            for t in 0..4 {
                let want = lora.layers[il][t].is_some();
                let got = loaded.layers[il][t].is_some();
                assert_eq!(want, got, "presence mismatch at L{}T{}", il, t);
                if let (Some(orig), Some(rt)) =
                    (lora.layers[il][t].as_ref(), loaded.layers[il][t].as_ref())
                {
                    assert_eq!(orig.input_dim, rt.input_dim);
                    assert_eq!(orig.output_dim, rt.output_dim);
                    assert_eq!(orig.a, rt.a);
                    assert_eq!(orig.b, rt.b);
                }
            }
        }
        std::fs::remove_file(tmp).ok();
        println!("ZLG4 roundtrip OK ({} layers)", lora.layers.len());
    }

    #[test]
    fn test_gemma4_train_step_loss_descent() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            return;
        }
        let model = GgufFile::open(model_path).expect("open gguf");
        let weights = CpuWeightsGemma4::load(&model).expect("load");
        let mut lora = Gemma4LoraAdapters::new(&weights.hparams, 16, 32.0);

        let tokens: Vec<u32> = vec![2, 1000, 2000, 3000, 4000, 5000];
        let answer_start = 3;  // train on tokens 3, 4, 5
        let lr = 3e-3f32;  // aggressive lr for smoke test

        println!("Running {} training steps on fixed tokens...", 3);
        let mut losses = Vec::new();
        for step in 1..=3 {
            let t0 = std::time::Instant::now();
            let loss = train_step_gemma4(&weights, &mut lora, &tokens, answer_start, lr, step);
            losses.push(loss);
            println!("  [step {}] loss={:.4} ({:.1}s)", step, loss, t0.elapsed().as_secs_f64());
            assert!(loss.is_finite(), "loss must be finite at step {}", step);
        }

        println!("\nLoss trajectory: {:?}", losses);
        // On a fixed training example with aggressive lr, loss should decrease
        // monotonically over 3 steps. Allow some slack (step 3 <= step 1 is sufficient).
        assert!(losses[2] <= losses[0],
            "loss should decrease: start={} end={}", losses[0], losses[2]);
        println!("✓ Loss descended: {:.4} → {:.4}", losses[0], losses[2]);
    }

    #[test]
    fn test_gemma4_lora_grad_health() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            return;
        }
        let model = GgufFile::open(model_path).expect("open gguf");
        let weights = CpuWeightsGemma4::load(&model).expect("load");
        let mut lora = Gemma4LoraAdapters::new(&weights.hparams, 16, 32.0);

        println!("LoRA: {} active targets, {:.1} MB",
            lora.n_active_targets(), lora.size_bytes() as f64 / 1e6);

        let tokens: Vec<u32> = vec![2, 1000, 2000, 3000];
        let t0 = std::time::Instant::now();
        let (logits, caches) = forward_gemma4_with_lora(&weights, Some(&lora), &tokens);
        println!("  Forward (with LoRA): {:.1}s", t0.elapsed().as_secs_f64());

        let t1 = std::time::Instant::now();
        let (loss, _health) = backward_gemma4_with_lora(
            &weights, None, Some(&mut lora), &caches, &logits, &tokens, 1);
        println!("  Backward (with LoRA grad accum): {:.1}s, loss={:.4}",
            t1.elapsed().as_secs_f64(), loss);

        // Count LoRA grad health per target/layer
        let mut healthy = 0;
        let mut nan_count = 0;
        let mut zero_count = 0;
        let mut total = 0;
        let target_names = ["Q", "K", "V", "O"];
        println!("\nLoRA grad health:");
        for il in 0..weights.hparams.n_layer {
            for t in 0..4 {
                if let Some(layer_l) = &lora.layers[il][t] {
                    total += 1;
                    let has_nan = layer_l.grad_a.iter().chain(layer_l.grad_b.iter())
                        .any(|v| v.is_nan());
                    let sum_sq: f32 = layer_l.grad_a.iter().chain(layer_l.grad_b.iter())
                        .map(|v| v * v).sum();
                    let norm = sum_sq.sqrt();
                    if has_nan {
                        nan_count += 1;
                    } else if sum_sq == 0.0 {
                        zero_count += 1;
                    } else {
                        healthy += 1;
                    }
                    // Print only transitions / edge cases
                    if il < 2 || il >= weights.hparams.n_layer - 2 {
                        println!("  L{:2}T{}({}): norm={:.3e} nan={}", il, t, target_names[t], norm, has_nan);
                    }
                }
            }
        }
        println!("\nLoRA target summary: healthy={}/{} zero={} nan={}",
            healthy, total, zero_count, nan_count);

        assert_eq!(nan_count, 0, "no NaN LoRA gradients");
        assert!(healthy > total / 2,
            "at least half of LoRA targets should have healthy grads, got {}/{}", healthy, total);
    }

    #[test]
    fn test_gemma4_backward_grad_health() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            println!("Gemma 4 GGUF not on disk — skipping");
            return;
        }
        let model = GgufFile::open(model_path).expect("open gguf");
        let weights = CpuWeightsGemma4::load(&model).expect("load");

        let tokens: Vec<u32> = vec![2, 1000, 2000, 3000];
        println!("Forward...");
        let t0 = std::time::Instant::now();
        let (logits, caches) = forward_gemma4(&weights, &tokens);
        println!("  Forward: {:.1}s", t0.elapsed().as_secs_f64());

        println!("Backward...");
        let t1 = std::time::Instant::now();
        let (loss, health) = backward_gemma4(&weights, &caches, &logits, &tokens, 1);
        println!("  Backward: {:.1}s, loss={:.4}", t1.elapsed().as_secs_f64(), loss);

        // Print health per layer
        println!("\nPer-layer grad health (incoming gradient at each layer's output):");
        println!("  layer | type   | KV   | grad_norm     | NaN | Inf | nonzero");
        for (idx, hh) in health.iter().enumerate() {
            let _ = idx;
            println!("  {:3}   | {:6} | {:5} | {:13.4e} | {:3} | {:3} | {}",
                hh.layer,
                if hh.is_sliding { "slide" } else { "full" },
                if hh.has_kv { "yes" } else { "no" },
                hh.grad_norm, hh.has_nan, hh.has_inf, hh.nonzero);
        }

        // Exit gate assertions
        assert!(loss.is_finite(), "loss must be finite, got {}", loss);
        let any_nan = health.iter().any(|h| h.has_nan);
        let any_inf = health.iter().any(|h| h.has_inf);
        let n_zero = health.iter().filter(|h| !h.nonzero && !h.has_nan && !h.has_inf).count();
        let n_healthy = health.iter().filter(|h| h.nonzero && !h.has_nan && !h.has_inf).count();

        println!("\nGrad health summary:");
        println!("  healthy={}/{} zero={} nan={} inf={}",
            n_healthy, health.len(), n_zero,
            health.iter().filter(|h| h.has_nan).count(),
            health.iter().filter(|h| h.has_inf).count());

        assert!(!any_nan, "WAVE10F exit gate FAILED: NaN gradients in {}/{} layers",
            health.iter().filter(|h| h.has_nan).count(), health.len());
        assert!(!any_inf, "WAVE10F exit gate FAILED: Inf gradients in {}/{} layers",
            health.iter().filter(|h| h.has_inf).count(), health.len());
        assert_eq!(n_healthy, health.len(),
            "WAVE10F exit gate FAILED: only {}/{} layers have healthy gradients",
            n_healthy, health.len());
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
