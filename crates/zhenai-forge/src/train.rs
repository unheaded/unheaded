// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Training loop orchestration — coordinates CPU ↔ GPU data flow.
//!
//! Architecture:
//!   1. GGUF model mmap'd, quantized tensors sent to GPU VRAM
//!   2. Training data tokenized on CPU (rayon parallel)
//!   3. Forward pass: GPU computes base model + LoRA output
//!   4. Loss computed on GPU
//!   5. Backward pass: gradients for LoRA params only (base frozen)
//!   6. Gradients copied to CPU, Adam step updates LoRA
//!   7. Updated LoRA copied back to GPU
//!   8. Repeat

use crate::forward;
use crate::gguf::GgufFile;
use crate::hip::{GpuBuffer, GpuDevice, self};
use crate::lora::LoraAdapters;
use crate::tokenizer::{Tokenizer, extract_vocabulary_from_gguf};
use std::time::Instant;

// Wave 10D post-mortem: on a 14 GB host with shared-VRAM AMD APU semantics,
// unconditional upload of all 32 Mistral-7B attention layers pushes allocations
// into ROCm HMM/SVM paging and livelocks the driver (observed 2026-04-12, see
// crates/zhenai-forge/notes/wave-10d-postmortem/README.md for budget derivation).
//
// Budget: 14 − 5.1 (model) − 1.0 (activations) − 2.0 (headroom) − 0.07 (LoRA)
//       ≈ 5.83 GB for bw_attn; at 0.27 GB/layer that's 21 layers theoretical.
// Cap at 16 to preserve ~1.6 GB host safety margin.
pub const BW_ATTN_MAX_LAYERS: usize = 16;

/// Refuse to start another bw_attn upload if host MemAvailable drops below this.
pub const MIN_FREE_HOST_RAM_BYTES: u64 = 2 * 1024 * 1024 * 1024;

/// Read /proc/meminfo MemAvailable in bytes. Returns 0 if unreadable —
/// callers must treat 0 as "unknown" and fail closed.
fn host_mem_available_bytes() -> u64 {
    let Ok(s) = std::fs::read_to_string("/proc/meminfo") else { return 0 };
    for line in s.lines() {
        if let Some(rest) = line.strip_prefix("MemAvailable:") {
            let kb: u64 = rest.trim().split_whitespace().next()
                .and_then(|tok| tok.parse().ok()).unwrap_or(0);
            return kb * 1024;
        }
    }
    0
}

/// Find the boundary between instruction and answer tokens.
/// Looks for the "]" token after "[/INST" in the token sequence.
/// Returns the index of the first ANSWER token (the one after [/INST]).
/// If not found, returns 3/4 of the sequence length as fallback.
fn find_answer_start(token_ids: &[u32], tokenizer: &Tokenizer) -> usize {
    // Decode tokens to find [/INST] boundary
    // In Mistral tokenizer, [/INST] is typically encoded as multiple tokens
    // We search for the pattern by decoding and finding the substring
    let mut text_so_far = String::new();
    for (i, &id) in token_ids.iter().enumerate() {
        if let Some(tok) = tokenizer.get_token(id) {
            text_so_far.push_str(&tok.replace('▁', " "));
        }
        if text_so_far.contains("[/INST]") {
            return (i + 1).min(token_ids.len() - 1);
        }
    }
    // Fallback: use 3/4 of the sequence
    token_ids.len() * 3 / 4
}

/// Cosine annealing with linear warmup.
/// Returns LR for the given step.
fn cosine_lr(peak_lr: f32, step: u32, total_steps: u32, warmup_steps: u32) -> f32 {
    let min_lr = peak_lr * 0.1; // decay to 10% of peak
    if step < warmup_steps {
        // Linear warmup
        min_lr + (peak_lr - min_lr) * (step as f32 / warmup_steps as f32)
    } else {
        // Cosine decay
        let progress = (step - warmup_steps) as f32 / (total_steps - warmup_steps).max(1) as f32;
        min_lr + 0.5 * (peak_lr - min_lr) * (1.0 + (std::f32::consts::PI * progress).cos())
    }
}

/// Training configuration.
pub struct TrainConfig {
    pub model_path: String,
    pub data_path: String,
    pub output_path: String,
    pub rank: u32,
    pub epochs: u32,
    pub lr: f32,
    pub batch_size: usize,
    pub max_seq_len: usize,
    pub log_interval: usize,
    pub save_interval: usize,
    pub accum_steps: usize,
}

impl Default for TrainConfig {
    fn default() -> Self {
        Self {
            model_path: String::new(),
            data_path: String::new(),
            output_path: "kingdom.zlora".into(),
            rank: 16,
            epochs: 1,
            lr: 1e-5,
            batch_size: 1,
            max_seq_len: 4096,
            log_interval: 50,
            save_interval: 500,
            accum_steps: 4,
        }
    }
}

/// Loaded model on GPU — base weights in VRAM, LoRA in RAM.
pub struct GpuModel {
    /// Base model tensor buffers on GPU
    pub weight_buffers: Vec<GpuBuffer>,
    /// Total VRAM used by base model
    pub vram_used: usize,
    /// Model metadata
    pub n_layers: u32,
    pub n_embd: u32,
    pub n_vocab: u32,
    pub n_head: u32,
    pub n_head_kv: u32,
}

impl GpuModel {
    /// Load quantized model tensors from GGUF directly to GPU VRAM.
    /// This is the key innovation: no fp16 decompression in RAM.
    pub fn load_from_gguf(model: &GgufFile) -> Result<Self, String> {
        let device = GpuDevice::init(0).map_err(|e| format!("GPU init failed: {}", e))?;
        println!("  GPU: {} ({:.1} GB VRAM, {} CUs)",
            device.name, device.vram_bytes as f64 / 1e9, device.compute_units);

        let n_layers = model.get_u32("llama.block_count").unwrap_or(32);
        let n_embd = model.get_u32("llama.embedding_length").unwrap_or(4096);
        let n_vocab = model.get_u32("llama.vocab_size")
            .or_else(|| model.get_u32("tokenizer.ggml.tokens").map(|_| 32000))
            .unwrap_or(32000);
        let n_head = model.get_u32("llama.attention.head_count").unwrap_or(32);
        let n_head_kv = model.get_u32("llama.attention.head_count_kv").unwrap_or(8);

        println!("  Architecture: {} layers, {} dim, {} vocab, {} heads ({} kv)",
            n_layers, n_embd, n_vocab, n_head, n_head_kv);

        // Allocate GPU buffers for each tensor
        let mut weight_buffers = Vec::new();
        let mut total_vram = 0usize;
        let mut loaded = 0;
        let total = model.tensors.len();

        for tensor in &model.tensors {
            let size = tensor.byte_size as usize;
            if size == 0 {
                continue;
            }

            match GpuBuffer::alloc(size) {
                Ok(buf) => {
                    // Copy REAL tensor data from mmap to GPU VRAM
                    let tensor_bytes = model.tensor_data(tensor);
                    if tensor_bytes.len() >= size {
                        buf.copy_from_host(&tensor_bytes[..size])
                            .map_err(|e| format!("GPU memcpy failed for {}: {}", tensor.name, e))?;
                    } else {
                        // Tensor data shorter than expected — zero-fill remainder
                        buf.zero().map_err(|e| format!("GPU zero failed: {}", e))?;
                    }
                    total_vram += size;
                    weight_buffers.push(buf);
                    loaded += 1;
                }
                Err(e) => {
                    println!("  WARNING: Failed to alloc {} bytes for {}: {}",
                        size, tensor.name, e);
                    // Continue — some tensors may be optional
                }
            }

            if loaded % 50 == 0 {
                println!("  Loading tensors: {}/{} ({:.1} MB VRAM)",
                    loaded, total, total_vram as f64 / 1e6);
            }
        }

        hip::sync().map_err(|e| format!("GPU sync failed: {}", e))?;
        println!("  Loaded: {}/{} tensors, {:.1} MB VRAM used", loaded, total, total_vram as f64 / 1e6);

        Ok(GpuModel {
            weight_buffers,
            vram_used: total_vram,
            n_layers,
            n_embd,
            n_vocab,
            n_head,
            n_head_kv,
        })
    }
}

/// GPU-accelerated trainer — uses hipBLAS sgemm for matrix multiply.
/// Dequantizes weights per-layer on CPU, uploads to GPU, runs sgemm.
/// Element-wise ops (RMSNorm, SiLU, softmax) stay on CPU.
/// Per-layer cached weight buffers on GPU.
pub struct GpuLayerWeights {
    pub attn_q: GpuBuffer,  // (n_embd × n_embd) f32
    pub attn_k: GpuBuffer,  // (kv_dim × n_embd) f32
    pub attn_v: GpuBuffer,  // (kv_dim × n_embd) f32
    pub attn_o: GpuBuffer,  // (n_embd × n_embd) f32
    pub ffn_gate: GpuBuffer, // (n_ff × n_embd) f32
    pub ffn_up: GpuBuffer,   // (n_ff × n_embd) f32
    pub ffn_down: GpuBuffer, // (n_embd × n_ff) f32
}

/// Wave 10D — per-layer attention-only weights cached on GPU for backward chain rule.
/// Uploaded once at init for ALL layers (no FFN — gradient-through-residual is enough).
pub struct GpuBwAttnWeights {
    pub attn_q: GpuBuffer,
    pub attn_k: GpuBuffer,
    pub attn_v: GpuBuffer,
    pub attn_o: GpuBuffer,
}

pub struct GpuTrainer {
    pub blas: hip::BlasHandle,
    /// Cached layer weights on GPU — uploaded ONCE at init, reused every step
    pub layer_weights: Vec<GpuLayerWeights>,
    /// Wave 10D: ALL-layer attention weights for chain rule backward (uploaded once)
    pub bw_attn: Vec<GpuBwAttnWeights>,
    /// Output projection cached on GPU: (n_vocab × n_embd) f32
    pub output_weight: GpuBuffer,
    /// GPU buffer for hidden states (batch × n_embd)
    pub hidden_buf: GpuBuffer,
    /// GPU buffer for output/result matrix
    pub result_buf: GpuBuffer,
    /// Scratch buffer for weight uploads (used only during init)
    pub weight_buf: GpuBuffer,
    /// Dimensions
    pub n_embd: usize,
    pub n_ff: usize,
    pub n_vocab: usize,
    pub n_cached_layers: usize,
}

impl GpuTrainer {
    /// Initialize GPU trainer and cache layer weights to VRAM.
    /// Dequantizes CPU weights → uploads to persistent GPU buffers.
    /// After init, zero weight transfers needed per training step.
    pub fn new(cpu_weights: &CpuWeights, n_embd: usize, n_ff: usize, n_vocab: usize, max_seq: usize) -> Result<Self, String> {
        let blas = hip::BlasHandle::new()
            .map_err(|e| format!("hipBLAS init failed: {}", e))?;

        let n_layers = cpu_weights.attn_q.len();
        let kv_dim = n_embd * cpu_weights.n_head_kv / cpu_weights.n_head;

        // Allocate and upload cached layer weights
        println!("  Caching {} layer weights to GPU VRAM...", n_layers);
        let mut layer_weights = Vec::new();
        let mut total_cached = 0usize;

        for l in 0..n_layers {
            let alloc_and_upload = |data: &[f32], name: &str| -> Result<GpuBuffer, String> {
                let size = data.len() * 4;
                let buf = GpuBuffer::alloc(size)
                    .map_err(|e| format!("GPU alloc {} layer {}: {}", name, l, e))?;
                buf.upload_f32(data)
                    .map_err(|e| format!("GPU upload {} layer {}: {}", name, l, e))?;
                Ok(buf)
            };

            let lw = GpuLayerWeights {
                attn_q: alloc_and_upload(&cpu_weights.attn_q[l], "attn_q")?,
                attn_k: alloc_and_upload(&cpu_weights.attn_k[l], "attn_k")?,
                attn_v: alloc_and_upload(&cpu_weights.attn_v[l], "attn_v")?,
                attn_o: alloc_and_upload(&cpu_weights.attn_o[l], "attn_o")?,
                ffn_gate: alloc_and_upload(&cpu_weights.ffn_gate[l], "ffn_gate")?,
                ffn_up: alloc_and_upload(&cpu_weights.ffn_up[l], "ffn_up")?,
                ffn_down: alloc_and_upload(&cpu_weights.ffn_down[l], "ffn_down")?,
            };

            let layer_size = cpu_weights.attn_q[l].len() + cpu_weights.attn_k[l].len()
                + cpu_weights.attn_v[l].len() + cpu_weights.attn_o[l].len()
                + cpu_weights.ffn_gate[l].len() + cpu_weights.ffn_up[l].len()
                + cpu_weights.ffn_down[l].len();
            total_cached += layer_size * 4;
            layer_weights.push(lw);
        }

        // Cache output projection
        let output_weight = {
            let size = cpu_weights.output.len() * 4;
            let buf = GpuBuffer::alloc(size)
                .map_err(|e| format!("GPU alloc output: {}", e))?;
            buf.upload_f32(&cpu_weights.output)
                .map_err(|e| format!("GPU upload output: {}", e))?;
            total_cached += size;
            buf
        };

        // Wave 10D: cache attention weights for backward chain rule.
        // Bounded by BW_ATTN_MAX_LAYERS and a live /proc/meminfo gate — see
        // post-mortem in crates/zhenai-forge/notes/wave-10d-postmortem/.
        let n_total = cpu_weights.all_attn_q.len();
        let layer_cap = n_total.min(BW_ATTN_MAX_LAYERS);
        let start_free = host_mem_available_bytes();
        if start_free < MIN_FREE_HOST_RAM_BYTES {
            return Err(format!(
                "bw_attn upload refused: host MemAvailable {:.2} GB < required {:.2} GB floor. \
                 Stop services to reclaim RAM before retrying.",
                start_free as f64 / 1e9,
                MIN_FREE_HOST_RAM_BYTES as f64 / 1e9));
        }
        println!("  Wave 10D: caching {} of {} attention layers (cap={}, start_free={:.2} GB)...",
            layer_cap, n_total, BW_ATTN_MAX_LAYERS, start_free as f64 / 1e9);
        let mut bw_attn: Vec<GpuBwAttnWeights> = Vec::with_capacity(layer_cap);
        let mut bw_total = 0usize;
        for l in 0..layer_cap {
            let q = &cpu_weights.all_attn_q[l];
            let k = &cpu_weights.all_attn_k[l];
            let v = &cpu_weights.all_attn_v[l];
            let o = &cpu_weights.all_attn_o[l];
            if q.is_empty() || k.is_empty() || v.is_empty() || o.is_empty() {
                return Err(format!("bw_attn upload: layer {} has empty weights — aborting, \
                    dataset/model mismatch suspected", l));
            }
            let free_now = host_mem_available_bytes();
            if free_now != 0 && free_now < MIN_FREE_HOST_RAM_BYTES {
                return Err(format!(
                    "bw_attn upload aborted at layer {}: host MemAvailable fell to {:.2} GB \
                     (< {:.2} GB floor). Cached {} layers so far. Budget derivation is wrong \
                     or the host shed RAM mid-run — return to Phase A.",
                    l, free_now as f64 / 1e9,
                    MIN_FREE_HOST_RAM_BYTES as f64 / 1e9, bw_attn.len()));
            }
            let alloc_up = |data: &[f32], name: &str| -> Result<GpuBuffer, String> {
                let buf = GpuBuffer::alloc(data.len() * 4)
                    .map_err(|e| format!("bw_attn alloc {} layer {}: {}", name, l, e))?;
                buf.upload_f32(data)
                    .map_err(|e| format!("bw_attn upload {} layer {}: {}", name, l, e))?;
                Ok(buf)
            };
            let q_buf = alloc_up(q, "attn_q")?;
            let k_buf = alloc_up(k, "attn_k")?;
            let v_buf = alloc_up(v, "attn_v")?;
            let o_buf = alloc_up(o, "attn_o")?;
            bw_total += (q.len() + k.len() + v.len() + o.len()) * 4;
            bw_attn.push(GpuBwAttnWeights { attn_q: q_buf, attn_k: k_buf, attn_v: v_buf, attn_o: o_buf });
            if (l + 1) % 4 == 0 {
                println!("    bw cache: {}/{} layers ({:.2} GB used, {:.2} GB host free)",
                    l + 1, layer_cap, bw_total as f64 / 1e9, free_now as f64 / 1e9);
            }
        }
        total_cached += bw_total;
        let end_free = host_mem_available_bytes();
        println!("  Wave 10D bw_attn: {}/{} layers cached, {:.2} GB used, host_free {:.2}→{:.2} GB",
            bw_attn.len(), n_total, bw_total as f64 / 1e9,
            start_free as f64 / 1e9, end_free as f64 / 1e9);

        hip::sync().map_err(|e| format!("GPU sync after cache: {}", e))?;
        println!("  Cached: {:.1} MB weights in GPU VRAM ({} fwd layers + {} bw layers + output)",
            total_cached as f64 / 1e6, n_layers, bw_attn.len());

        // Working buffers (not cached — reused per step)
        let max_batch = 8;
        let hidden_size = max_batch * n_ff.max(n_embd).max(n_vocab) * 4;
        let hidden_buf = GpuBuffer::alloc(hidden_size)
            .map_err(|e| format!("GPU alloc hidden_buf: {}", e))?;

        let result_size = (n_vocab * max_batch).max(n_ff * max_batch) * 4;
        let result_buf = GpuBuffer::alloc(result_size)
            .map_err(|e| format!("GPU alloc result_buf: {}", e))?;

        // Scratch buffer (reused for misc uploads)
        let weight_buf = GpuBuffer::alloc(n_ff * n_embd * 4)
            .map_err(|e| format!("GPU alloc weight_buf: {}", e))?;

        println!("  Working buffers: hidden={:.1}MB result={:.1}MB",
            hidden_size as f64 / 1e6, result_size as f64 / 1e6);

        Ok(GpuTrainer {
            blas, layer_weights, bw_attn, output_weight,
            hidden_buf, result_buf, weight_buf,
            n_embd, n_ff, n_vocab, n_cached_layers: n_layers,
        })
    }

    /// GPU matmul: result = input × weight^T (row-major)
    /// input: (1 × in_dim), weight: (out_dim × in_dim), result: (1 × out_dim)
    /// Uses hipBLAS column-major: to compute row-major A×B^T, we do col-major B×A^T
    pub fn matmul_weight_t(
        &self,
        input: &[f32],      // (1 × in_dim) — one position's hidden state
        weight: &[f32],     // (out_dim × in_dim) — row-major weight matrix
        out_dim: usize,
        in_dim: usize,
    ) -> Result<Vec<f32>, String> {
        // Upload input to hidden_buf
        self.hidden_buf.upload_f32(input)
            .map_err(|e| format!("upload input: {}", e))?;

        // Upload weight to weight_buf (stored row-major = column-major transpose)
        self.weight_buf.upload_f32(&weight[..out_dim * in_dim])
            .map_err(|e| format!("upload weight: {}", e))?;

        // Zero result
        self.result_buf.zero().map_err(|e| format!("zero result: {}", e))?;

        // hipBLAS column-major: C = alpha * op(A) * op(B) + beta * C
        // We want: result(1×out) = input(1×in) × weight^T(in×out)
        // Col-major: result(out×1) = weight(out×in) × input(in×1)
        // So: m=out, n=1, k=in, A=weight(out×in), B=input(in×1), C=result(out×1)
        // weight is row-major (out×in) which IS column-major layout for (out×in) matrix
        // Actually: row-major(out×in) = col-major(in×out) transposed
        // Use transpose flag on weight: C = weight^T × input won't work...
        //
        // Simplest: weight is (out_dim × in_dim) row-major.
        // In col-major, this is a (in_dim × out_dim) matrix.
        // We want: result = weight × input (row-major)
        //        = input^T × weight^T (also row-major, same thing for vectors)
        // Col-major: result(out×1) = weight_colmaj^T(out×in) × input(in×1)
        // weight_colmaj is (in_dim × out_dim) so weight_colmaj^T is (out_dim × in_dim) ✓
        self.blas.sgemm_ex(
            true,   // transpose A (weight stored as col-major in×out, transpose to out×in)
            false,  // no transpose B
            out_dim as i32, // m = rows of result
            1,              // n = 1 (single position)
            in_dim as i32,  // k = inner dimension
            1.0,
            &self.weight_buf, in_dim as i32,  // lda = in_dim (col-major stride of the stored matrix)
            &self.hidden_buf, in_dim as i32,  // ldb = in_dim (column height of input vector)
            0.0,
            &self.result_buf, out_dim as i32, // ldc = out_dim
        ).map_err(|e| format!("sgemm: {}", e))?;

        hip::sync().map_err(|e| format!("sync: {}", e))?;

        // Download result
        let mut result = vec![0.0f32; out_dim];
        self.result_buf.download_f32(&mut result)
            .map_err(|e| format!("download result: {}", e))?;

        Ok(result)
    }
}

impl GpuTrainer {
    /// Batched GPU matmul using CACHED weight buffer (zero upload overhead).
    /// inputs: (n_pos × in_dim) on CPU, weight: already on GPU, results: downloaded to CPU.
    pub fn cached_batched_matmul(
        &self,
        inputs: &[f32],
        weight_gpu: &GpuBuffer,
        n_pos: usize,
        out_dim: usize,
        in_dim: usize,
    ) -> Result<Vec<f32>, String> {
        // Upload only the inputs — clamp to buffer size
        let upload_elems = (n_pos * in_dim).min(self.hidden_buf.len() / 4);
        self.hidden_buf.upload_f32(&inputs[..upload_elems])
            .map_err(|e| format!("upload inputs ({} elems, buf {} bytes): {}", upload_elems, self.hidden_buf.len(), e))?;

        self.result_buf.zero().map_err(|e| format!("zero result: {}", e))?;

        // sgemm with cached weight — NO weight upload
        self.blas.sgemm_ex(
            true, false,
            out_dim as i32, n_pos as i32, in_dim as i32,
            1.0,
            weight_gpu, in_dim as i32,
            &self.hidden_buf, in_dim as i32,
            0.0,
            &self.result_buf, out_dim as i32,
        ).map_err(|e| format!("cached sgemm: {}", e))?;

        hip::sync().map_err(|e| format!("sync: {}", e))?;

        let download_elems = n_pos * out_dim;
        let max_elems = self.result_buf.len() / 4; // buffer size in f32
        let actual_elems = download_elems.min(max_elems);
        let mut results = vec![0.0f32; actual_elems];
        self.result_buf.download_f32(&mut results)
            .map_err(|e| format!("download ({} elems, buf {} bytes): {}", actual_elems, self.result_buf.len(), e))?;
        // Extend to full size if buffer was smaller (shouldn't happen with correct sizing)
        results.resize(download_elems, 0.0);

        Ok(results)
    }

    /// Wave 10D — GPU backward pass: compute grad_input = grad_output @ weight
    ///
    /// Forward: result(n_pos × out_dim) = input(n_pos × in_dim) @ weight^T
    /// Backward: grad_input(n_pos × in_dim) = grad_output(n_pos × out_dim) @ weight(out_dim × in_dim)
    ///
    /// Col-major BLAS: grad_input_cm(in_dim, n_pos) = weight_cm(in_dim, out_dim) @ grad_output_cm(out_dim, n_pos)
    /// (no transpose on either operand)
    pub fn gpu_matmul_grad_input(
        &self,
        grad_output: &[f32],   // (n_pos × out_dim) row-major
        weight_gpu: &GpuBuffer, // cached weight (out_dim × in_dim) row-major
        n_pos: usize,
        out_dim: usize,
        in_dim: usize,
    ) -> Result<Vec<f32>, String> {
        // Upload grad_output into hidden_buf (reuse staging)
        let upload_elems = (n_pos * out_dim).min(self.hidden_buf.len() / 4);
        self.hidden_buf.upload_f32(&grad_output[..upload_elems])
            .map_err(|e| format!("upload grad_output ({} elems): {}", upload_elems, e))?;

        self.result_buf.zero().map_err(|e| format!("zero result: {}", e))?;

        // Col-major: result_cm(in_dim, n_pos) = weight_cm(in_dim, out_dim) @ grad_output_cm(out_dim, n_pos)
        // weight is row-major (out_dim × in_dim), col-major view = (in_dim × out_dim), lda = in_dim
        // grad_output is row-major (n_pos × out_dim), col-major view = (out_dim × n_pos), ldb = out_dim
        // result is row-major (n_pos × in_dim), col-major view = (in_dim × n_pos), ldc = in_dim
        self.blas.sgemm_ex(
            false, false,
            in_dim as i32, n_pos as i32, out_dim as i32,
            1.0,
            weight_gpu, in_dim as i32,
            &self.hidden_buf, out_dim as i32,
            0.0,
            &self.result_buf, in_dim as i32,
        ).map_err(|e| format!("grad_input sgemm: {}", e))?;

        hip::sync().map_err(|e| format!("sync: {}", e))?;

        let download_elems = n_pos * in_dim;
        let max_elems = self.result_buf.len() / 4;
        let actual_elems = download_elems.min(max_elems);
        let mut results = vec![0.0f32; actual_elems];
        self.result_buf.download_f32(&mut results)
            .map_err(|e| format!("download grad_input: {}", e))?;
        results.resize(download_elems, 0.0);
        Ok(results)
    }

    /// Batched GPU matmul: results = inputs × weight^T
    /// inputs: (n_pos × in_dim), weight: (out_dim × in_dim), results: (n_pos × out_dim)
    /// Uploads weight ONCE, all positions in one sgemm call.
    pub fn batched_matmul_weight_t(
        &self,
        inputs: &[f32],     // (n_pos × in_dim) — multiple positions concatenated
        weight: &[f32],     // (out_dim × in_dim) — row-major weight matrix
        n_pos: usize,
        out_dim: usize,
        in_dim: usize,
    ) -> Result<Vec<f32>, String> {
        // Upload inputs (n_pos × in_dim) to GPU
        self.hidden_buf.upload_f32(&inputs[..n_pos * in_dim])
            .map_err(|e| format!("upload inputs: {}", e))?;

        // Upload weight (out_dim × in_dim) to GPU — ONCE
        self.weight_buf.upload_f32(&weight[..out_dim * in_dim])
            .map_err(|e| format!("upload weight: {}", e))?;

        // Zero result buffer
        self.result_buf.zero().map_err(|e| format!("zero result: {}", e))?;

        // Batched sgemm: result(n_pos × out_dim) = inputs(n_pos × in_dim) × weight^T(in_dim × out_dim)
        // Col-major: result_cm(out_dim × n_pos) = weight_cm^T(out_dim × in_dim) × inputs_cm(in_dim × n_pos)
        self.blas.sgemm_ex(
            true,               // transpose A (weight)
            false,              // no transpose B (inputs)
            out_dim as i32,     // m = rows of result
            n_pos as i32,       // n = columns of result (number of positions)
            in_dim as i32,      // k = inner dimension
            1.0,
            &self.weight_buf, in_dim as i32,
            &self.hidden_buf, in_dim as i32,
            0.0,
            &self.result_buf, out_dim as i32,
        ).map_err(|e| format!("batched sgemm: {}", e))?;

        hip::sync().map_err(|e| format!("sync: {}", e))?;

        // Download results (n_pos × out_dim)
        let mut results = vec![0.0f32; n_pos * out_dim];
        self.result_buf.download_f32(&mut results)
            .map_err(|e| format!("download results: {}", e))?;

        Ok(results)
    }

    /// GPU-accelerated forward loss — BATCHED hipBLAS sgemm.
    /// Returns (loss, per_layer_normed_inputs) — the normed inputs are reused in backward.
    pub fn forward_loss(
        &self,
        cpu_weights: &CpuWeights,
        token_ids: &[u32],
        lora: &LoraAdapters,
        answer_start: usize,
    ) -> Result<(f32, Vec<Vec<Vec<f32>>>), String> {
        if token_ids.len() < 2 { return Ok((10.0, Vec::new())); }

        let n = cpu_weights.n_embd;
        let seq_len = token_ids.len();
        let loss_start = answer_start.max(1);
        let max_positions: usize = 8;
        let raw_n = if seq_len > loss_start + 1 { seq_len - loss_start - 1 } else { 0 };
        if raw_n == 0 { return Ok((10.0, Vec::new())); }
        let stride = if raw_n > max_positions { raw_n / max_positions } else { 1 };

        // Collect the positions we'll process
        let mut positions: Vec<usize> = Vec::new();
        let mut s = loss_start;
        while s < seq_len {
            positions.push(s);
            s += stride;
        }
        let n_pos = positions.len();

        // 1. Embedding lookup (CPU — fast indexing)
        let embeddings = forward::embedding_lookup(&cpu_weights.embed, n, token_ids);
        let mut hidden = embeddings;

        // 2. Process last 8 layers — BATCHED: all positions per weight matrix in one sgemm
        let n_layers_fwd = cpu_weights.n_layers.min(lora.layers.len()).min(4);
        let layer_offset = cpu_weights.n_layers.saturating_sub(n_layers_fwd);
        let scale = lora.alpha / lora.rank as f32;
        let kv_dim = n * cpu_weights.n_head_kv / cpu_weights.n_head;

        // Save per-layer normed inputs for backward pass reuse
        let mut gpu_layer_normed: Vec<Vec<Vec<f32>>> = Vec::new();

        for l_idx in 0..n_layers_fwd {
            let l_abs = layer_offset + l_idx;
            if l_idx >= cpu_weights.attn_q.len() || l_idx >= cpu_weights.attn_norm.len() { break; }

            // Batch: extract + normalize hidden states at all positions
            let mut normed_batch = vec![0.0f32; n_pos * n];
            let mut pos_normed: Vec<Vec<f32>> = Vec::new();
            for (pi, &pos) in positions.iter().enumerate() {
                let input = &hidden[pos * n..(pos + 1) * n];
                let normed = forward::rmsnorm(input, &cpu_weights.attn_norm[l_idx], 1e-5);
                normed_batch[pi * n..(pi + 1) * n].copy_from_slice(&normed);
                pos_normed.push(normed);
            }

            // Q/K/V/O projections — CACHED GPU weights, zero upload per step
            let proj_dims = [n, kv_dim, kv_dim, n];
            let cached_w = [
                &self.layer_weights[l_idx].attn_q, &self.layer_weights[l_idx].attn_k,
                &self.layer_weights[l_idx].attn_v, &self.layer_weights[l_idx].attn_o,
            ];

            let mut attn_batch = vec![0.0f32; n_pos * n];

            for (t, gpu_w) in cached_w.iter().enumerate() {
                let out_dim = proj_dims[t];

                // CACHED sgemm — weight already on GPU, only upload 128KB of hidden states
                let proj_batch = self.cached_batched_matmul(&normed_batch, gpu_w, n_pos, out_dim, n)?;

                // Add LoRA contribution per position (CPU — tiny)
                for pi in 0..n_pos {
                    let normed_slice = &normed_batch[pi * n..(pi + 1) * n];
                    let lora_out = if l_abs < lora.layers.len() {
                        lora.layers[l_abs][t].forward(normed_slice)
                    } else {
                        vec![0.0; out_dim]
                    };

                    let effective_dim = out_dim.min(n);
                    for i in 0..effective_dim {
                        let proj_val = proj_batch[pi * out_dim + i];
                        let lora_val = if i < lora_out.len() { lora_out[i] } else { 0.0 };
                        attn_batch[pi * n + i] += (proj_val + lora_val * scale) * 0.25;
                    }
                }
            }

            // Residual connection for all positions
            for (pi, &pos) in positions.iter().enumerate() {
                for i in 0..n {
                    hidden[pos * n + i] += attn_batch[pi * n + i];
                }
            }

            // FFN — also batched
            if l_idx < cpu_weights.ffn_gate.len() && l_idx < cpu_weights.ffn_norm.len() {
                let n_ff = cpu_weights.n_ff;

                // Batch FFN inputs
                let mut ffn_normed_batch = vec![0.0f32; n_pos * n];
                for (pi, &pos) in positions.iter().enumerate() {
                    let input = &hidden[pos * n..(pos + 1) * n];
                    let normed = forward::rmsnorm(input, &cpu_weights.ffn_norm[l_idx], 1e-5);
                    ffn_normed_batch[pi * n..(pi + 1) * n].copy_from_slice(&normed);
                }

                // Gate + Up via CACHED GPU weights
                let gate_batch = self.cached_batched_matmul(&ffn_normed_batch, &self.layer_weights[l_idx].ffn_gate, n_pos, n_ff, n)?;
                let up_batch = self.cached_batched_matmul(&ffn_normed_batch, &self.layer_weights[l_idx].ffn_up, n_pos, n_ff, n)?;

                // SiLU + element-wise (CPU)
                let mut ffn_hidden_batch = vec![0.0f32; n_pos * n_ff];
                for pi in 0..n_pos {
                    for i in 0..n_ff {
                        ffn_hidden_batch[pi * n_ff + i] = forward::silu(gate_batch[pi * n_ff + i]) * up_batch[pi * n_ff + i];
                    }
                }

                // Down via CACHED GPU weight
                let ffn_out_batch = self.cached_batched_matmul(&ffn_hidden_batch, &self.layer_weights[l_idx].ffn_down, n_pos, n, n_ff)?;

                // FFN residual
                for (pi, &pos) in positions.iter().enumerate() {
                    for i in 0..n {
                        hidden[pos * n + i] += ffn_out_batch[pi * n + i];
                    }
                }
            }
            gpu_layer_normed.push(pos_normed);
        }

        // 3. Compute loss — batch output projection too
        let mut normed_out_batch = vec![0.0f32; n_pos * n];
        for (pi, &pos) in positions.iter().enumerate() {
            let h = &hidden[pos * n..(pos + 1) * n];
            let normed = forward::rmsnorm(h, &cpu_weights.output_norm, 1e-5);
            normed_out_batch[pi * n..(pi + 1) * n].copy_from_slice(&normed);
        }

        // Logits via CACHED output weight on GPU
        let logits_batch = self.cached_batched_matmul(&normed_out_batch, &self.output_weight, n_pos, cpu_weights.n_vocab, n)?;

        let mut total_loss = 0.0f32;
        let mut count = 0u32;
        for (pi, &pos) in positions.iter().enumerate() {
            if pos + 1 >= seq_len { continue; }
            let logits = &logits_batch[pi * cpu_weights.n_vocab..(pi + 1) * cpu_weights.n_vocab];
            let target = token_ids[pos + 1] % cpu_weights.n_vocab as u32;
            total_loss += forward::cross_entropy_loss(logits, target);
            count += 1;
        }

        let avg_loss = if count > 0 { total_loss / count as f32 } else { 10.0 };
        Ok((avg_loss, gpu_layer_normed))
    }
}

/// Training state and metrics.
pub struct TrainState {
    pub epoch: u32,
    pub step: u32,
    pub total_loss: f64,
    pub best_eval_loss: f64,
    pub start_time: Instant,
}

/// Run the full training loop.
pub fn train(config: &TrainConfig) -> Result<(), String> {
    println!("============================================================");
    println!("ZHENAI FORGE — Training Loop");
    println!("============================================================\n");

    // 1. Load GGUF model metadata
    let model = GgufFile::open(&config.model_path)
        .map_err(|e| format!("Failed to open model: {}", e))?;
    println!("Model: {} tensors, {:.1} MB\n", model.tensors.len(), model.file_size as f64 / 1e6);

    // 2. Load model to GPU
    println!("Loading model to GPU...");
    let gpu_model = GpuModel::load_from_gguf(&model)?;
    println!("  VRAM used: {:.1} MB / {:.1} GB available\n",
        gpu_model.vram_used as f64 / 1e6,
        GpuDevice::init(0).map(|d| d.vram_bytes as f64 / 1e9).unwrap_or(0.0));

    // 3. Initialize LoRA adapters (in RAM)
    let mut lora = LoraAdapters::new(gpu_model.n_layers, gpu_model.n_embd, config.rank);
    println!("LoRA initialized:");
    println!("  Rank: {}, Alpha: {}", config.rank, lora.alpha);
    println!("  Trainable params: {} ({:.1} MB)",
        lora.num_params(), lora.size_bytes() as f64 / 1e6);
    println!("  Optimizer states: {:.1} MB\n",
        lora.size_bytes() as f64 / 1e6 * 4.0); // m, v for A and B

    // 4. Build tokenizer from GGUF vocabulary
    println!("Loading tokenizer from GGUF...");
    let vocab = extract_vocabulary_from_gguf(&config.model_path)
        .map_err(|e| format!("Failed to extract vocabulary: {}", e))?;
    let bos = model.get_u32("tokenizer.ggml.bos_token_id").unwrap_or(1);
    let eos = model.get_u32("tokenizer.ggml.eos_token_id").unwrap_or(2);
    let tokenizer = Tokenizer::from_tokens(vocab, bos, eos);
    println!("  Vocabulary: {} tokens (BOS={}, EOS={})\n", tokenizer.vocab_size, bos, eos);

    // 5. Load training data
    let data = load_and_format_training_data(&config.data_path)?;
    println!("Training data: {} examples\n", data.len());

    // 4b. Dequantize essential weights to CPU for forward pass
    let cpu_weights = CpuWeights::load(&model, gpu_model.n_layers)?;
    println!();

    // 4c. Initialize GPU trainer — cache layer weights to VRAM (zero upload per step)
    println!("Initializing GPU trainer (caching weights to VRAM)...");
    let gpu_trainer = GpuTrainer::new(
        &cpu_weights, cpu_weights.n_embd, cpu_weights.n_ff, cpu_weights.n_vocab, config.max_seq_len,
    )?;
    println!();

    // 5. Training loop
    let start = Instant::now();
    let mut state = TrainState {
        epoch: 0,
        step: 0,
        total_loss: 0.0,
        best_eval_loss: f64::MAX,
        start_time: start,
    };

    let total_steps = config.epochs as u32 * data.len() as u32;
    let warmup_steps = (total_steps / 20).max(50).min(200); // 5% warmup, clamped 50-200
    println!("  LR schedule: cosine annealing, warmup={warmup_steps} steps, peak={:.1e}, total={total_steps}", config.lr);
    println!("  Gradient accumulation: {} steps (effective batch size {})", config.accum_steps, config.accum_steps);
    println!("  Max sequence length: {} tokens", config.max_seq_len);

    // Tokenizer cache: tokenize all examples once, reuse across epochs
    println!("  Caching tokenized sequences...");
    let cached_tokens: Vec<Vec<u32>> = data.iter()
        .map(|example| {
            let mut ids = tokenizer.encode(example);
            ids.truncate(config.max_seq_len);
            ids
        })
        .collect();

    // Cache answer boundaries: find [/INST] position for each example
    let cached_answer_starts: Vec<usize> = cached_tokens.iter()
        .map(|ids| find_answer_start(ids, &tokenizer))
        .collect();

    let avg_answer_tokens: f64 = cached_tokens.iter().zip(cached_answer_starts.iter())
        .map(|(ids, &start)| (ids.len() - start) as f64)
        .sum::<f64>() / cached_tokens.len() as f64;
    println!("  Cached {} sequences (avg {:.0} tokens, avg {:.0} answer tokens)",
        cached_tokens.len(),
        cached_tokens.iter().map(|t| t.len()).sum::<usize>() as f64 / cached_tokens.len() as f64,
        avg_answer_tokens);

    for epoch in 0..config.epochs {
        state.epoch = epoch;
        let mut epoch_loss = 0.0f64;
        let mut epoch_steps = 0u32;

        println!("--- Epoch {}/{} ---", epoch + 1, config.epochs);

        for (i, token_ids) in cached_tokens.iter().enumerate() {
            let answer_start = cached_answer_starts[i];
            let step_start = Instant::now();
            // Verbose progress: emit BEGIN line every step so user sees liveness
            println!("  [step {}/{}] begin (tokens={}, answer_start={})", i + 1, cached_tokens.len(), token_ids.len(), answer_start);
            use std::io::Write;
            let _ = std::io::stdout().flush();

            // Forward pass via GPU hipBLAS — returns loss + normed inputs for backward reuse
            let (loss, gpu_normed) = if token_ids.len() >= 2 && answer_start < token_ids.len() - 1 {
                match gpu_trainer.forward_loss(&cpu_weights, &token_ids, &lora, answer_start) {
                    Ok((l, normed)) => (l, normed),
                    Err(e) => {
                        eprintln!("  GPU forward error at step {}: {}", state.step, e);
                        (10.0, Vec::new())
                    }
                }
            } else {
                (10.0, Vec::new())
            };

            // Analytical backpropagation — multi-position gradient accumulation.
            // Computes gradients from EACH answer position, not just the last token.
            // This teaches the model to generate coherent answer text.
            {
                use crate::backward;

                let n = cpu_weights.n_embd;
                // Process ALL 32 layers via stream-dequantization from GGUF.
                // Each layer dequantized on demand, used, then dropped.
                // Peak RAM: 1 layer's weights (~870MB) + hidden states + LoRA.
                let n_layers_total = lora.layers.len(); // 32
                let loss_start = answer_start.max(1);
                let loss_end = token_ids.len();
                let max_loss_positions: usize = 4; // fewer positions = faster per step
                let raw_n_positions = if loss_end > loss_start + 1 { loss_end - loss_start - 1 } else { 1 };
                let n_loss_positions = raw_n_positions.min(max_loss_positions);
                let stride = if raw_n_positions > max_loss_positions {
                    raw_n_positions / max_loss_positions
                } else { 1 };

                // === Backward uses normed inputs from GPU forward pass ===
                // GPU forward processed layers [layer_offset..layer_offset+n_cached_layers]
                // with full base weights. Those normed inputs are in `gpu_normed`.
                // For layers NOT covered by GPU (0..layer_offset-1), compute LoRA-only.
                let gpu_layer_offset = cpu_weights.n_layers.saturating_sub(gpu_trainer.n_cached_layers);
                let scale = lora.alpha / lora.rank as f32;

                // Build the unified normed_per_pos array for all 32 layers
                let mut layer_normed_per_pos: Vec<Vec<Vec<f32>>> = Vec::new();

                // For layers 0..gpu_layer_offset: LoRA-only forward to get normed inputs
                let embeddings = forward::embedding_lookup(&cpu_weights.embed, n, &token_ids);
                let mut hidden_bw = embeddings.clone();

                for l in 0..n_layers_total {
                    if l >= gpu_layer_offset && (l - gpu_layer_offset) < gpu_normed.len() {
                        // Use normed inputs from GPU forward (has base weights baked in)
                        layer_normed_per_pos.push(gpu_normed[l - gpu_layer_offset].clone());
                    } else {
                        // Layers not covered by GPU — stream dequantize norm, LoRA-only forward
                        let attn_norm = forward::dequantize_tensor(&model, &format!("blk.{}.attn_norm.weight", l))
                            .unwrap_or_else(|| vec![1.0; n]);

                        let mut pos_normed: Vec<Vec<f32>> = Vec::new();
                        let mut s = loss_start;
                        while s < loss_end {
                            let input = hidden_bw[s * n..(s + 1) * n].to_vec();
                            let normed = forward::rmsnorm(&input, &attn_norm, 1e-5);
                            if s < loss_end - 1 {
                                pos_normed.push(normed.clone());
                            }
                            // LoRA-only residual for hidden state propagation
                            for t in 0..4 {
                                if l < lora.layers.len() {
                                    let lora_out = lora.layers[l][t].forward(&normed);
                                    for ii in 0..n.min(lora_out.len()) {
                                        hidden_bw[s * n + ii] += lora_out[ii] * scale * 0.25;
                                    }
                                }
                            }
                            s += stride;
                        }
                        layer_normed_per_pos.push(pos_normed);
                    }
                }

                // === Backward pass: accumulate gradients from ALL answer positions ===
                let vocab_subset = cpu_weights.n_vocab;

                for pos_idx in 0..n_loss_positions {
                    let t_pos = loss_start + pos_idx * stride; // strided position in sequence
                    if t_pos + 1 >= token_ids.len() { break; }

                    // Output logits at this position
                    let h = &hidden_bw[t_pos * n..(t_pos + 1) * n];
                    let normed_out = forward::rmsnorm(h, &cpu_weights.output_norm, 1e-5);
                    let mut logits = vec![0.0f32; vocab_subset];
                    for v in 0..vocab_subset {
                        for ii in 0..n {
                            logits[v] += normed_out[ii] * cpu_weights.output[v * n + ii];
                        }
                    }

                    // Gradient from this position's loss
                    let target = token_ids[t_pos + 1] % vocab_subset as u32;
                    let grad_logits = backward::cross_entropy_softmax_backward(&logits, target);

                    // Scale gradient by 1/n_loss_positions for averaging
                    let pos_scale = 1.0 / n_loss_positions as f32;

                    let mut grad_normed = vec![0.0f32; n];
                    for ii in 0..n {
                        for v in 0..vocab_subset {
                            grad_normed[ii] += grad_logits[v] * cpu_weights.output[v * n + ii];
                        }
                        grad_normed[ii] *= pos_scale;
                    }

                    let mut grad_hidden = backward::rmsnorm_backward(
                        h, &cpu_weights.output_norm, &grad_normed, 1e-5);

                    // Through each layer (reverse) — accumulate LoRA gradients
                    // AND propagate grad_hidden through base weights (chain rule fix)
                    for l in (0..n_layers_total.min(layer_normed_per_pos.len())).rev() {
                        // Get saved normed input for this position at this layer
                        let normed_input = if pos_idx < layer_normed_per_pos[l].len() {
                            &layer_normed_per_pos[l][pos_idx]
                        } else {
                            continue;
                        };

                        // LoRA backward for all 4 targets
                        for t in 0..4 {
                            let target_grad: Vec<f32> = grad_hidden.iter().map(|&g| g * 0.25).collect();
                            let (_lora_out, lora_h) = lora.layers[l][t].forward_with_hidden(normed_input);
                            let (ga, gb) = backward::lora_backward(
                                normed_input, &lora_h, &target_grad,
                                &lora.layers[l][t].a, &lora.layers[l][t].b,
                                n, n, lora.rank as usize, lora.alpha,
                            );

                            for (ii, &g) in ga.iter().enumerate() {
                                if ii < lora.layers[l][t].grad_a.len() {
                                    lora.layers[l][t].grad_a[ii] += g;
                                }
                            }
                            for (ii, &g) in gb.iter().enumerate() {
                                if ii < lora.layers[l][t].grad_b.len() {
                                    lora.layers[l][t].grad_b[ii] += g;
                                }
                            }
                        }

                        // Wave 10D Phase 1.5: chain rule via GPU sgemm for ALL layers.
                        // bw_attn[l] holds Q/K/V/O for layer l, uploaded once at startup.
                        // Drops the per-step CPU attn_only_layer_backward bottleneck.
                        let kv_dim = n * cpu_weights.n_head_kv / cpu_weights.n_head;
                        if l < gpu_trainer.bw_attn.len() && l < cpu_weights.all_attn_norm.len() {
                            let bw = &gpu_trainer.bw_attn[l];
                            // grad_normed = sum_t W_t^T @ (grad_hidden * 0.25)
                            let scaled: Vec<f32> = grad_hidden.iter().map(|g| g * 0.25).collect();
                            let grad_q = gpu_trainer.gpu_matmul_grad_input(&scaled, &bw.attn_q, 1, n, n).unwrap_or_default();
                            let scaled_kv = &scaled[..kv_dim.min(scaled.len())];
                            let grad_k = gpu_trainer.gpu_matmul_grad_input(scaled_kv, &bw.attn_k, 1, kv_dim, n).unwrap_or_default();
                            let grad_v = gpu_trainer.gpu_matmul_grad_input(scaled_kv, &bw.attn_v, 1, kv_dim, n).unwrap_or_default();
                            let grad_o = gpu_trainer.gpu_matmul_grad_input(&scaled, &bw.attn_o, 1, n, n).unwrap_or_default();
                            let mut grad_normed = vec![0.0f32; n];
                            for j in 0..n {
                                if j < grad_q.len() { grad_normed[j] += grad_q[j]; }
                                if j < grad_k.len() { grad_normed[j] += grad_k[j]; }
                                if j < grad_v.len() { grad_normed[j] += grad_v[j]; }
                                if j < grad_o.len() { grad_normed[j] += grad_o[j]; }
                            }
                            let grad_input_attn = backward::rmsnorm_backward(
                                normed_input, &cpu_weights.all_attn_norm[l], &grad_normed, 1e-5);
                            for i in 0..n {
                                grad_hidden[i] = grad_hidden[i] + grad_input_attn[i];
                            }
                        } else if l < cpu_weights.all_attn_q.len() {
                            // CPU backward. Two sub-cases:
                            //   (a) eager layer: Vec populated, use directly
                            //   (b) Path C lazy layer: Vec empty, re-dequantize from
                            //       GGUF mmap into locals that drop at block end.
                            // The GPU path above covers layers 0..bw_attn.len()
                            // (= eager_cap), so this branch runs for layers
                            // bw_attn.len()..n_total — always the lazy range
                            // unless bw_attn upload was short.
                            let eager = !cpu_weights.all_attn_q[l].is_empty();
                            let (q_vec, k_vec, v_vec, o_vec);
                            let (q_ref, k_ref, v_ref, o_ref): (&[f32], &[f32], &[f32], &[f32]);
                            if eager {
                                q_ref = &cpu_weights.all_attn_q[l];
                                k_ref = &cpu_weights.all_attn_k[l];
                                v_ref = &cpu_weights.all_attn_v[l];
                                o_ref = &cpu_weights.all_attn_o[l];
                            } else {
                                q_vec = forward::dequantize_tensor(&model, &format!("blk.{}.attn_q.weight", l))
                                    .unwrap_or_default();
                                k_vec = forward::dequantize_tensor(&model, &format!("blk.{}.attn_k.weight", l))
                                    .unwrap_or_default();
                                v_vec = forward::dequantize_tensor(&model, &format!("blk.{}.attn_v.weight", l))
                                    .unwrap_or_default();
                                o_vec = forward::dequantize_tensor(&model, &format!("blk.{}.attn_output.weight", l))
                                    .unwrap_or_default();
                                if q_vec.is_empty() || k_vec.is_empty() || v_vec.is_empty() || o_vec.is_empty() {
                                    // Tensor genuinely missing from model — skip this layer's backward
                                    continue;
                                }
                                q_ref = &q_vec;
                                k_ref = &k_vec;
                                v_ref = &v_vec;
                                o_ref = &o_vec;
                            }
                            grad_hidden = backward::attn_only_layer_backward(
                                &grad_hidden,
                                normed_input,
                                &cpu_weights.all_attn_norm[l],
                                q_ref, k_ref, v_ref, o_ref,
                                n, kv_dim, kv_dim, n,
                                None, 0, 0.0,
                                n,
                            );
                            // Lazy Vecs drop here, releasing ~170 MB/layer to the allocator.
                        }
                    }
                } // end per-position gradient loop

                // Gradient accumulation: clip + Adam step every accum_steps
                if (i + 1) % config.accum_steps == 0 || i == data.len() - 1 {
                    let scale = 1.0 / config.accum_steps as f32;
                    let lr = cosine_lr(config.lr, state.step, total_steps, warmup_steps);

                    for l in 0..n_layers_total.min(lora.layers.len()) {
                        for t in 0..4 {
                            // Scale
                            for g in lora.layers[l][t].grad_a.iter_mut() { *g *= scale; }
                            for g in lora.layers[l][t].grad_b.iter_mut() { *g *= scale; }

                            // Clip
                            let grad_norm: f32 = lora.layers[l][t].grad_a.iter()
                                .chain(lora.layers[l][t].grad_b.iter())
                                .map(|g| g * g).sum::<f32>().sqrt();
                            if grad_norm > 1.0 {
                                let clip = 1.0 / grad_norm;
                                for g in lora.layers[l][t].grad_a.iter_mut() { *g *= clip; }
                                for g in lora.layers[l][t].grad_b.iter_mut() { *g *= clip; }
                            }

                            // Adam
                            lora.layers[l][t].adam_step(lr, 0.9, 0.999, 1e-8, state.step + 1);

                            // Zero
                            for g in lora.layers[l][t].grad_a.iter_mut() { *g = 0.0; }
                            for g in lora.layers[l][t].grad_b.iter_mut() { *g = 0.0; }
                        }
                    }
                }
            }
            epoch_loss += loss as f64;
            epoch_steps += 1;
            state.step += 1;
            state.total_loss += loss as f64;

            // Log EVERY step — Wave 10D requirement (per-step progress + ETA)
            let step_secs = step_start.elapsed().as_secs_f64();
            let elapsed = start.elapsed().as_secs_f64();
            let steps_per_sec = state.step as f64 / elapsed;
            let avg_loss = epoch_loss / epoch_steps as f64;
            let remaining_in_epoch = (data.len() as f64) - (i as f64 + 1.0);
            let remaining_epochs = (config.epochs - epoch - 1) as f64;
            let eta_s = remaining_in_epoch / steps_per_sec.max(0.0001)
                + remaining_epochs * data.len() as f64 / steps_per_sec.max(0.0001);
            let eta_min = eta_s / 60.0;

            println!("  [step {}/{}] DONE loss={:.4} avg={:.4} step_time={:.1}s | sps={:.3} | ETA={:.0}m",
                i + 1, cached_tokens.len(), loss, avg_loss, step_secs, steps_per_sec, eta_min);
            let _ = std::io::stdout().flush();

            // Save checkpoint
            if state.step > 0 && state.step as usize % config.save_interval == 0 {
                let cp_path = format!("{}.checkpoint-{}", config.output_path, state.step);
                lora.save_zlora(&cp_path, &model)
                    .map_err(|e| format!("Checkpoint save failed: {}", e))?;
                println!("  Checkpoint: {} ({:.1} MB)", cp_path,
                    std::fs::metadata(&cp_path).map(|m| m.len() as f64 / 1e6).unwrap_or(0.0));
            }
        }

        let avg_epoch_loss = epoch_loss / epoch_steps as f64;
        println!("  Epoch {} complete | Avg loss: {:.4} | Time: {:.0}s\n",
            epoch + 1, avg_epoch_loss, start.elapsed().as_secs_f64());
    }

    // 6. Save final LoRA
    println!("Saving final LoRA adapter...");
    lora.save_zlora(&config.output_path, &model)
        .map_err(|e| format!("Save failed: {}", e))?;

    let total_time = start.elapsed().as_secs_f64();
    let output_size = std::fs::metadata(&config.output_path)
        .map(|m| m.len() as f64 / 1e6).unwrap_or(0.0);

    println!("\n============================================================");
    println!("TRAINING COMPLETE");
    println!("============================================================");
    println!("  Output:    {} ({:.1} MB)", config.output_path, output_size);
    println!("  Steps:     {}", state.step);
    println!("  Time:      {:.0}s ({:.1} min)", total_time, total_time / 60.0);
    println!("  Avg loss:  {:.4}", state.total_loss / state.step as f64);
    println!("  VRAM used: {:.1} MB (base) + {:.1} MB (LoRA)",
        gpu_model.vram_used as f64 / 1e6, lora.size_bytes() as f64 / 1e6);
    println!("  RAM used:  {:.1} MB (LoRA + optimizer)",
        lora.size_bytes() as f64 / 1e6 * 5.0); // weights + grads + m + v
    Ok(())
}

/// Dequantized model weights for CPU forward pass.
/// Only holds the tensors needed for loss computation — not all 291.
struct CpuWeights {
    /// Token embedding: (vocab_size × embed_dim)
    embed: Vec<f32>,
    /// Output projection: (vocab_size × embed_dim)
    output: Vec<f32>,
    /// Output norm: (embed_dim,)
    output_norm: Vec<f32>,
    /// Per-layer attention weights: [layer] → (embed_dim × embed_dim)
    /// NOTE: indexed [0..max_layers] = the LAST max_layers of the model.
    attn_q: Vec<Vec<f32>>,
    attn_k: Vec<Vec<f32>>,
    attn_v: Vec<Vec<f32>>,
    attn_o: Vec<Vec<f32>>,
    /// Per-layer FFN weights
    ffn_gate: Vec<Vec<f32>>,
    ffn_up: Vec<Vec<f32>>,
    ffn_down: Vec<Vec<f32>>,
    /// Per-layer norms: [layer] → (embed_dim,)
    attn_norm: Vec<Vec<f32>>,
    ffn_norm: Vec<Vec<f32>>,

    /// ALL-LAYER attention weights for chain rule backward (Wave 10C).
    /// Indexed [0..n_total_layers]. Loaded once at startup, ~5.4GB for Mistral-7B.
    /// FFN weights NOT included (no LoRA on FFN, FFN backward not needed).
    all_attn_q: Vec<Vec<f32>>,
    all_attn_k: Vec<Vec<f32>>,
    all_attn_v: Vec<Vec<f32>>,
    all_attn_o: Vec<Vec<f32>>,
    all_attn_norm: Vec<Vec<f32>>,

    n_vocab: usize,
    n_embd: usize,
    n_ff: usize,
    n_layers: usize,
    n_total_layers: usize,
    n_head: usize,
    n_head_kv: usize,
}

impl CpuWeights {
    /// Load essential weights from GGUF by dequantizing on CPU.
    /// This is the streaming approach — only load what's needed.
    fn load(model: &GgufFile, n_layers: u32) -> Result<Self, String> {
        println!("  Dequantizing essential weights to CPU...");

        let embed = forward::dequantize_tensor(model, "token_embd.weight")
            .ok_or("Failed to dequantize token_embd.weight")?;
        let n_vocab = model.get_u32("llama.vocab_size").unwrap_or(32000) as usize;
        let n_embd = model.get_u32("llama.embedding_length").unwrap_or(4096) as usize;
        println!("    token_embd: {} elements ({:.1} MB)", embed.len(), embed.len() as f64 * 4.0 / 1e6);

        let output = forward::dequantize_tensor(model, "output.weight")
            .ok_or("Failed to dequantize output.weight")?;
        println!("    output: {} elements ({:.1} MB)", output.len(), output.len() as f64 * 4.0 / 1e6);

        let output_norm = forward::dequantize_tensor(model, "output_norm.weight")
            .ok_or("Failed to dequantize output_norm.weight")?;

        // Load LAST 4 layers — fits in VRAM alongside quantized model.
        // 4 layers × ~870MB f32 = ~3.5GB + 4.8GB quantized = 8.3GB of 12GB.
        let max_layers = n_layers.min(4) as usize;
        let layer_start = (n_layers as usize).saturating_sub(max_layers);
        let mut attn_q = Vec::new();
        let mut attn_k = Vec::new();
        let mut attn_v = Vec::new();
        let mut attn_o = Vec::new();
        let mut ffn_gate = Vec::new();
        let mut ffn_up = Vec::new();
        let mut ffn_down = Vec::new();
        let mut attn_norm = Vec::new();
        let mut ffn_norm = Vec::new();

        for l in layer_start..(layer_start + max_layers) {
            // Q/K/V/O projections
            for (name_suffix, vec) in [
                ("attn_q", &mut attn_q),
                ("attn_k", &mut attn_k),
                ("attn_v", &mut attn_v),
                ("attn_output", &mut attn_o),
            ] {
                let tensor_name = format!("blk.{}.{}.weight", l, name_suffix);
                if let Some(w) = forward::dequantize_tensor(model, &tensor_name) {
                    if name_suffix == "attn_q" {
                        println!("    blk.{} Q/K/V/O: {:.1} MB each", l, w.len() as f64 * 4.0 / 1e6);
                    }
                    vec.push(w);
                }
            }
            // Norms
            if let Some(n) = forward::dequantize_tensor(model, &format!("blk.{}.attn_norm.weight", l)) {
                attn_norm.push(n);
            }
            if let Some(n) = forward::dequantize_tensor(model, &format!("blk.{}.ffn_norm.weight", l)) {
                ffn_norm.push(n);
            }

            // FFN weights (gate, up, down)
            for (suffix, vec) in [
                ("ffn_gate", &mut ffn_gate),
                ("ffn_up", &mut ffn_up),
                ("ffn_down", &mut ffn_down),
            ] {
                let name = format!("blk.{}.{}.weight", l, suffix);
                if let Some(w) = forward::dequantize_tensor(model, &name) {
                    if suffix == "ffn_gate" {
                        println!("    blk.{} FFN gate/up/down: {:.1} MB each", l, w.len() as f64 * 4.0 / 1e6);
                    }
                    vec.push(w);
                }
            }
        }

        // Load attention weights for the chain rule (Wave 10C + 10D Path C).
        // Path C: eagerly load only layers 0..BW_ATTN_MAX_LAYERS (bw_attn will
        // upload these to GPU). Layers BW_ATTN_MAX_LAYERS..n_total are left
        // empty at load time and re-dequantized lazily per-backward-step from
        // the GGUF mmap (see lazy_dequantize_attn_layer). This avoids the
        // ~2.7 GB peak-memory cliff that broke E0 on the 14 GB west host.
        // attn_norm is always loaded (small: 16 KB/layer, 512 KB total).
        println!("  Loading attention weights for chain rule (Wave 10C + 10D Path C)...");
        let n_total_layers = n_layers as usize;
        let eager_cap = BW_ATTN_MAX_LAYERS.min(n_total_layers);
        let mut all_attn_q: Vec<Vec<f32>> = Vec::with_capacity(n_total_layers);
        let mut all_attn_k: Vec<Vec<f32>> = Vec::with_capacity(n_total_layers);
        let mut all_attn_v: Vec<Vec<f32>> = Vec::with_capacity(n_total_layers);
        let mut all_attn_o: Vec<Vec<f32>> = Vec::with_capacity(n_total_layers);
        let mut all_attn_norm: Vec<Vec<f32>> = Vec::with_capacity(n_total_layers);
        for l in 0..n_total_layers {
            if l < eager_cap {
                all_attn_q.push(forward::dequantize_tensor(model, &format!("blk.{}.attn_q.weight", l))
                    .unwrap_or_default());
                all_attn_k.push(forward::dequantize_tensor(model, &format!("blk.{}.attn_k.weight", l))
                    .unwrap_or_default());
                all_attn_v.push(forward::dequantize_tensor(model, &format!("blk.{}.attn_v.weight", l))
                    .unwrap_or_default());
                all_attn_o.push(forward::dequantize_tensor(model, &format!("blk.{}.attn_output.weight", l))
                    .unwrap_or_default());
            } else {
                // Path C lazy — placeholder empty Vecs; re-dequantized on demand in backward.
                all_attn_q.push(Vec::new());
                all_attn_k.push(Vec::new());
                all_attn_v.push(Vec::new());
                all_attn_o.push(Vec::new());
            }
            // attn_norm is small and always needed (both GPU path's rmsnorm_backward
            // and CPU-fallback use it). Keep eager for all layers.
            all_attn_norm.push(forward::dequantize_tensor(model, &format!("blk.{}.attn_norm.weight", l))
                .unwrap_or_else(|| vec![1.0; n_embd]));
            if (l + 1) % 8 == 0 {
                println!("    Loaded {}/{} attn_norm layers ({} Q/K/V/O eager)",
                    l + 1, n_total_layers, (l + 1).min(eager_cap));
            }
        }
        let all_attn_mb: f64 = all_attn_q.iter().chain(all_attn_k.iter())
            .chain(all_attn_v.iter()).chain(all_attn_o.iter())
            .map(|v| v.len()).sum::<usize>() as f64 * 4.0 / 1e6;
        let lazy_count = n_total_layers - eager_cap;
        println!("    All-layer attention: {:.1} MB eager ({} layers), {} lazy layers",
            all_attn_mb, eager_cap, lazy_count);

        let total_elements: usize = embed.len() + output.len() + output_norm.len()
            + attn_q.iter().chain(attn_k.iter()).chain(attn_v.iter()).chain(attn_o.iter())
                .chain(ffn_gate.iter()).chain(ffn_up.iter()).chain(ffn_down.iter())
                .chain(attn_norm.iter()).chain(ffn_norm.iter())
                .map(|v| v.len()).sum::<usize>();
        let total_mb = total_elements as f64 * 4.0 / 1e6;
        println!("    Total CPU weights: {:.1} MB (+ {:.1} MB chain rule)", total_mb, all_attn_mb);

        let n_ff = model.get_u32("llama.feed_forward_length").unwrap_or(14336) as usize;

        let n_head = model.get_u32("llama.attention.head_count").unwrap_or(32) as usize;
        let n_head_kv = model.get_u32("llama.attention.head_count_kv").unwrap_or(8) as usize;

        Ok(CpuWeights {
            embed, output, output_norm,
            attn_q, attn_k, attn_v, attn_o,
            ffn_gate, ffn_up, ffn_down,
            attn_norm, ffn_norm,
            all_attn_q, all_attn_k, all_attn_v, all_attn_o, all_attn_norm,
            n_ff,
            n_vocab, n_embd, n_layers: max_layers,
            n_total_layers,
            n_head, n_head_kv,
        })
    }

    /// Simplified forward pass: embed → (simplified attention × layers) → norm → output → loss
    /// Returns cross-entropy loss for the given token sequence.
    /// Forward pass computing loss across ALL answer tokens (multi-position).
    /// For each position t in [answer_start, len-1), predicts token[t+1] from hidden[t].
    /// Returns the average cross-entropy loss across all answer positions.
    fn forward_loss(&self, token_ids: &[u32], lora: &LoraAdapters, answer_start: usize) -> f32 {
        if token_ids.len() < 2 {
            return 10.0;
        }

        let n = self.n_embd;
        let seq_len = token_ids.len();

        // Sample up to 16 answer positions evenly for speed.
        let loss_start = answer_start.max(1);
        let loss_end = seq_len;
        let max_positions: usize = 16;
        let raw_n = if loss_end > loss_start + 1 { loss_end - loss_start - 1 } else { 0 };

        if raw_n == 0 {
            return 10.0;
        }

        let stride = if raw_n > max_positions { raw_n / max_positions } else { 1 };

        // 1. Embedding lookup for FULL sequence (needed for context)
        let embeddings = forward::embedding_lookup(&self.embed, n, token_ids);

        // 2. Process through transformer layers
        // For memory efficiency, only process last N positions through the expensive layers
        // but we need the full sequence for proper context in the hidden states.
        let mut hidden = embeddings.clone();

        // Only process last 8 layers for speed (matches backward pass)
        let n_layers_fwd = self.n_layers.min(lora.layers.len()).min(4);
        let fwd_layer_offset = self.n_layers.saturating_sub(n_layers_fwd);

        for l_idx in 0..n_layers_fwd {
            let l = fwd_layer_offset + l_idx;
            if l >= self.attn_q.len() || l >= self.attn_norm.len() {
                break;
            }

            let scale = lora.alpha / lora.rank as f32;

            // Process sampled answer positions through this layer (strided)
            let mut s = loss_start;
            while s < seq_len {
                let input = &hidden[s * n..(s + 1) * n].to_vec();
                let normed = forward::rmsnorm(input, &self.attn_norm[l], 1e-5);

                // Combined Q/K/V/O projection with LoRA (all 4 targets)
                let mut attn_output = vec![0.0f32; n];
                let weights = [
                    &self.attn_q, &self.attn_k, &self.attn_v, &self.attn_o,
                ];

                for (t, w) in weights.iter().enumerate() {
                    if l >= w.len() { continue; }

                    // LoRA contribution (full dims, rank-16 = fast)
                    let lora_out = lora.layers[l][t].forward(&normed);

                    // Base projection: use 512 dims for reasonable quality/speed balance
                    let dims = n.min(512);
                    let mut proj = vec![0.0f32; n];
                    for i in 0..dims {
                        let mut sum = 0.0f32;
                        for j in 0..dims {
                            sum += normed[j] * w[l][i * n + j];
                        }
                        proj[i] = sum;
                    }

                    // Add LoRA
                    for i in 0..n.min(lora_out.len()) {
                        proj[i] += lora_out[i] * scale;
                    }

                    for i in 0..n {
                        attn_output[i] += proj[i] * 0.25;
                    }
                }

                // Residual
                for i in 0..n {
                    hidden[s * n + i] += attn_output[i];
                }

                // FFN
                if l < self.ffn_gate.len() && l < self.ffn_norm.len() {
                    let ffn_input = hidden[s * n..(s + 1) * n].to_vec();
                    let ffn_normed = forward::rmsnorm(&ffn_input, &self.ffn_norm[l], 1e-5);
                    let ffn_out = forward::ffn_forward(
                        &ffn_normed,
                        &self.ffn_gate[l], &self.ffn_up[l], &self.ffn_down[l],
                        n, self.n_ff,
                    );
                    for i in 0..n {
                        hidden[s * n + i] += ffn_out[i];
                    }
                }
                s += stride;
            }
        }

        // 3. Compute loss at sampled answer positions (strided for speed)
        let vocab_subset = self.n_vocab;
        let mut total_loss = 0.0f32;
        let mut count = 0u32;

        let mut t = loss_start;
        while t < loss_end - 1 {
            // Process this position
            // Get hidden state at position t
            let h = &hidden[t * n..(t + 1) * n];
            let normed = forward::rmsnorm(h, &self.output_norm, 1e-5);

            // Output projection → logits
            let mut logits = vec![0.0f32; vocab_subset];
            for v in 0..vocab_subset {
                let mut sum = 0.0f32;
                for i in 0..n {
                    sum += normed[i] * self.output[v * n + i];
                }
                logits[v] = sum;
            }

            // Cross-entropy loss: predict token[t+1]
            let target = token_ids[t + 1] % vocab_subset as u32;
            total_loss += forward::cross_entropy_loss(&logits, target);
            count += 1;

            t += stride;
        }

        if count > 0 { total_loss / count as f32 } else { 10.0 }
    }
}

/// Load JSONL training data and format as Mistral instruct prompts.
/// Uses Dataset::format_prompt for RAFT-aware formatting (source context + distractors).
fn load_and_format_training_data(path: &str) -> Result<Vec<String>, String> {
    use crate::data::{Dataset, TrainingExample};

    let content = std::fs::read_to_string(path)
        .map_err(|e| format!("Failed to read {}: {}", path, e))?;

    let mut raft_count = 0;
    let mut simple_count = 0;

    let prompts: Vec<String> = content.lines()
        .filter(|l| !l.trim().is_empty())
        .filter_map(|l| {
            let example: TrainingExample = serde_json::from_str(l).ok()?;
            if example.has_raft_context() {
                raft_count += 1;
            } else {
                simple_count += 1;
            }
            Some(Dataset::format_prompt(&example))
        })
        .collect();

    if prompts.is_empty() {
        return Err(format!("No valid training examples found in {}", path));
    }

    println!("  Formatted {} prompts ({} RAFT with context, {} simple Q/A)",
        prompts.len(), raft_count, simple_count);

    Ok(prompts)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_train_config_defaults() {
        let cfg = TrainConfig::default();
        assert_eq!(cfg.rank, 16);
        assert_eq!(cfg.epochs, 1);
        assert_eq!(cfg.batch_size, 1);
        assert_eq!(cfg.max_seq_len, 4096);
        assert_eq!(cfg.accum_steps, 4);
    }

    #[test]
    fn test_gpu_model_load() {
        // Only run if GPU is available
        if GpuDevice::init(0).is_err() {
            println!("No GPU, skipping");
            return;
        }

        let model_path = "/var/zhen/models/mistral-7b-instruct-q5_k_m.gguf";
        if !std::path::Path::new(model_path).exists() {
            println!("Model not found, skipping");
            return;
        }

        let model = GgufFile::open(model_path).expect("Failed to open GGUF");
        let gpu_model = GpuModel::load_from_gguf(&model).expect("Failed to load to GPU");

        assert_eq!(gpu_model.n_layers, 32);
        assert_eq!(gpu_model.n_embd, 4096);
        assert!(gpu_model.vram_used > 0);
        println!("GPU model loaded: {:.1} MB VRAM", gpu_model.vram_used as f64 / 1e6);
    }

    /// Wave 10D Path C invariant: CpuWeights::load must eagerly dequantize only
    /// the first BW_ATTN_MAX_LAYERS layers of all_attn_q/k/v/o, leaving the rest
    /// as empty Vecs (lazy-dequantized in the backward path). all_attn_norm is
    /// small (~16 KB/layer) and is kept eager for all layers.
    #[test]
    fn test_cpu_weights_path_c_lazy_skip() {
        let model_path = "/var/zhen/models/mistral-7b-instruct-q5_k_m.gguf";
        if !std::path::Path::new(model_path).exists() {
            println!("Model not found, skipping");
            return;
        }
        let model = GgufFile::open(model_path).expect("Failed to open GGUF");
        let n_layers = model.get_u32("llama.block_count").unwrap_or(32);
        let cpu = CpuWeights::load(&model, n_layers).expect("CpuWeights::load failed");

        assert_eq!(cpu.all_attn_q.len(), n_layers as usize, "all_attn_q index range");
        assert_eq!(cpu.all_attn_norm.len(), n_layers as usize, "all_attn_norm index range");

        // Eager layers: populated.
        for l in 0..BW_ATTN_MAX_LAYERS {
            assert!(!cpu.all_attn_q[l].is_empty(), "layer {} attn_q should be eager", l);
            assert!(!cpu.all_attn_k[l].is_empty(), "layer {} attn_k should be eager", l);
            assert!(!cpu.all_attn_v[l].is_empty(), "layer {} attn_v should be eager", l);
            assert!(!cpu.all_attn_o[l].is_empty(), "layer {} attn_o should be eager", l);
        }
        // Lazy layers: Q/K/V/O empty, norm still populated.
        for l in BW_ATTN_MAX_LAYERS..(n_layers as usize) {
            assert!(cpu.all_attn_q[l].is_empty(), "layer {} attn_q should be lazy", l);
            assert!(cpu.all_attn_k[l].is_empty(), "layer {} attn_k should be lazy", l);
            assert!(cpu.all_attn_v[l].is_empty(), "layer {} attn_v should be lazy", l);
            assert!(cpu.all_attn_o[l].is_empty(), "layer {} attn_o should be lazy", l);
            assert!(!cpu.all_attn_norm[l].is_empty(), "layer {} attn_norm stays eager", l);
        }

        // Footprint budget: eager all_attn total < 3 GB (16 layers × ~168 MB ≈ 2.7 GB).
        let bytes: usize = cpu.all_attn_q.iter().chain(cpu.all_attn_k.iter())
            .chain(cpu.all_attn_v.iter()).chain(cpu.all_attn_o.iter())
            .map(|v| v.len() * 4).sum();
        assert!(bytes < 3 * 1024 * 1024 * 1024,
            "Path C eager all_attn footprint exceeded 3 GB budget: {} bytes", bytes);
        println!("Path C OK: eager all_attn {:.2} GB, {} lazy layers",
            bytes as f64 / 1e9, (n_layers as usize) - BW_ATTN_MAX_LAYERS);
    }

    #[test]
    fn test_gpu_trainer_matmul() {
        if GpuDevice::init(0).is_err() {
            println!("No GPU, skipping");
            return;
        }

        // Test GPU batched matmul with hipBLAS sgemm
        // Allocate GPU buffers manually for test (no CpuWeights needed)
        let blas = hip::BlasHandle::new().expect("hipblas");
        let weight: Vec<f32> = vec![
            1.0, 0.0, 0.0, 0.0, // row 0: picks input[0]
            0.0, 1.0, 0.0, 0.0, // row 1: picks input[1]
            0.0, 0.0, 1.0, 0.0, // row 2
            0.0, 0.0, 0.0, 1.0, // row 3
            1.0, 1.0, 0.0, 0.0, // row 4: input[0]+input[1]
            0.0, 0.0, 1.0, 1.0, // row 5: input[2]+input[3]
            1.0, 1.0, 1.0, 1.0, // row 6: sum all
            2.0, 0.0, 0.0, 0.0, // row 7: 2*input[0]
        ];
        let input: Vec<f32> = vec![1.0, 2.0, 3.0, 4.0];

        // Upload weight to GPU and test cached_batched_matmul pattern
        let w_buf = GpuBuffer::alloc(weight.len() * 4).expect("alloc w");
        w_buf.upload_f32(&weight).expect("upload w");
        let h_buf = GpuBuffer::alloc(input.len() * 4).expect("alloc h");
        h_buf.upload_f32(&input).expect("upload h");
        let r_buf = GpuBuffer::alloc(8 * 4).expect("alloc r");
        r_buf.zero().expect("zero r");

        // sgemm: result(8×1) = weight^T(8×4) × input(4×1)
        blas.sgemm_ex(true, false, 8, 1, 4, 1.0, &w_buf, 4, &h_buf, 4, 0.0, &r_buf, 8)
            .expect("sgemm");
        hip::sync().expect("sync");

        let mut result = vec![0.0f32; 8];
        r_buf.download_f32(&mut result).expect("download");

        println!("GPU cached matmul result: {:?}", result);
        assert!((result[0] - 1.0).abs() < 0.01, "row0: {} expected 1.0", result[0]);
        assert!((result[1] - 2.0).abs() < 0.01, "row1: {} expected 2.0", result[1]);
        assert!((result[4] - 3.0).abs() < 0.01, "row4: {} expected 3.0", result[4]);
        assert!((result[6] - 10.0).abs() < 0.01, "row6: {} expected 10.0", result[6]);
        assert!((result[7] - 2.0).abs() < 0.01, "row7: {} expected 2.0", result[7]);
        println!("GPU cached matmul verified");
    }

    /// Wave 10D — verify gpu_matmul_grad_input against CPU reference
    /// Forward: result = input @ weight^T
    /// Backward: grad_input = grad_output @ weight
    /// CPU reference: backward::matmul_backward_a
    #[test]
    fn test_gpu_matmul_grad_input() {
        use crate::backward;

        if GpuDevice::init(0).is_err() {
            println!("No GPU, skipping");
            return;
        }

        // Small but non-trivial: n_pos=4, in_dim=8, out_dim=12
        let n_pos = 4usize;
        let in_dim = 8usize;
        let out_dim = 12usize;

        // Random-ish weight (out_dim × in_dim) row-major
        let weight: Vec<f32> = (0..out_dim * in_dim)
            .map(|i| ((i as f32) * 0.13).sin() * 0.5)
            .collect();
        // Random-ish grad_output (n_pos × out_dim) row-major
        let grad_output: Vec<f32> = (0..n_pos * out_dim)
            .map(|i| ((i as f32) * 0.27).cos() * 0.3)
            .collect();

        // CPU reference: grad_input = grad_output @ weight
        // matmul_backward_a expects: grad_C(m × n), B(k × n), returns grad_A(m × k)
        // We have grad_output(n_pos × out_dim) @ weight(out_dim × in_dim) = grad_input(n_pos × in_dim)
        // So m=n_pos, n=in_dim... wait, that's not how matmul_backward_a is shaped.
        //
        // Just compute the reference directly (one matmul):
        let mut cpu_grad_input = vec![0.0f32; n_pos * in_dim];
        for p in 0..n_pos {
            for i in 0..in_dim {
                let mut s = 0.0f32;
                for o in 0..out_dim {
                    s += grad_output[p * out_dim + o] * weight[o * in_dim + i];
                }
                cpu_grad_input[p * in_dim + i] = s;
            }
        }

        // GPU path: build a tiny GpuTrainer-equivalent
        let blas = hip::BlasHandle::new().expect("blas");
        let max_buf = (n_pos * in_dim).max(n_pos * out_dim).max(out_dim * in_dim) * 4;
        let weight_buf = GpuBuffer::alloc(out_dim * in_dim * 4).expect("alloc weight");
        weight_buf.upload_f32(&weight).expect("upload weight");
        let hidden_buf = GpuBuffer::alloc(max_buf).expect("alloc hidden");
        let result_buf = GpuBuffer::alloc(max_buf).expect("alloc result");

        // Inline gpu_matmul_grad_input logic (mirror the method exactly)
        hidden_buf.upload_f32(&grad_output).expect("upload grad_output");
        result_buf.zero().expect("zero result");
        blas.sgemm_ex(
            false, false,
            in_dim as i32, n_pos as i32, out_dim as i32,
            1.0,
            &weight_buf, in_dim as i32,
            &hidden_buf, out_dim as i32,
            0.0,
            &result_buf, in_dim as i32,
        ).expect("sgemm");
        hip::sync().expect("sync");

        let mut gpu_grad_input = vec![0.0f32; n_pos * in_dim];
        result_buf.download_f32(&mut gpu_grad_input).expect("download");

        // Compare
        let mut max_err = 0.0f32;
        for i in 0..cpu_grad_input.len() {
            let err = (cpu_grad_input[i] - gpu_grad_input[i]).abs();
            let rel = if cpu_grad_input[i].abs() > 1e-6 { err / cpu_grad_input[i].abs() } else { err };
            if rel > max_err { max_err = rel; }
            if i < 4 {
                println!("  [{}] cpu={:.6} gpu={:.6} rel_err={:.6}",
                    i, cpu_grad_input[i], gpu_grad_input[i], rel);
            }
        }
        println!("GPU vs CPU max rel error: {:.6}", max_err);
        assert!(max_err < 1e-3, "GPU backward disagrees with CPU: max_err={}", max_err);
    }

    #[test]
    fn test_cosine_lr() {
        let peak = 3e-4;
        let total = 1000;
        let warmup = 100;

        // Step 0: start of warmup (near min_lr)
        let lr0 = cosine_lr(peak, 0, total, warmup);
        assert!(lr0 < peak * 0.2, "lr0={lr0} should be near min");

        // Step 100: end of warmup (at peak)
        let lr100 = cosine_lr(peak, warmup, total, warmup);
        assert!((lr100 - peak).abs() < 1e-7, "lr100={lr100} should be peak={peak}");

        // Step 500: midpoint (between peak and min)
        let lr500 = cosine_lr(peak, 500, total, warmup);
        assert!(lr500 < peak && lr500 > peak * 0.1, "lr500={lr500} should be between peak and min");

        // Step 1000: end (near min_lr)
        let lr1000 = cosine_lr(peak, total, total, warmup);
        assert!((lr1000 - peak * 0.1).abs() < 1e-6, "lr1000={lr1000} should be near min={}", peak * 0.1);

        // Monotonic decrease after warmup
        let mut prev = peak;
        for s in (warmup..total).step_by(10) {
            let lr = cosine_lr(peak, s, total, warmup);
            assert!(lr <= prev + 1e-7, "LR should decrease: step {s}, lr={lr}, prev={prev}");
            prev = lr;
        }
    }
}
