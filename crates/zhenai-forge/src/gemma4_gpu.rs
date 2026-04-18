// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! GPU-resident Gemma 4 weights + forward/backward (Phase 7 GPU port).
//!
//! Plan: `crates/zhenai-forge/notes/wave10f-gpu-port-plan.md`.
//!
//! This module starts with Step A: upload weights from CpuWeightsGemma4 to
//! GPU buffers and validate VRAM budget. Forward/backward GPU ports come
//! in subsequent commits.

use crate::gemma4::CpuWeightsGemma4;
use crate::gemma4::Gemma4Hparams;
use crate::hip::{BlasHandle, GpuBuffer, GpuDevice, HipError};
use half::bf16;

/// GPU-resident Gemma 4 weights. Each tensor is uploaded as bf16 (matching
/// on-disk + on-CPU storage). Output of matmuls accumulates in f32.
pub struct Gemma4GpuWeights {
    pub hparams: Gemma4Hparams,

    // Globals (bf16)
    pub token_embd: GpuBuffer,
    pub per_layer_token_embd: Option<GpuBuffer>,
    pub per_layer_model_proj: GpuBuffer,
    // Globals (f32)
    pub output_norm: GpuBuffer,
    pub per_layer_proj_norm: GpuBuffer,
    pub rope_freqs: GpuBuffer,

    // Per-layer (bf16 weights)
    pub wq: Vec<GpuBuffer>,
    pub wk: Vec<Option<GpuBuffer>>,
    pub wv: Vec<Option<GpuBuffer>>,
    pub wo: Vec<GpuBuffer>,
    pub ffn_gate: Vec<GpuBuffer>,
    pub ffn_up: Vec<GpuBuffer>,
    pub ffn_down: Vec<GpuBuffer>,
    pub inp_gate: Vec<GpuBuffer>,
    pub proj: Vec<GpuBuffer>,

    // Per-layer (f32 norms — small, kept on GPU for in-kernel access later)
    pub attn_norm: Vec<GpuBuffer>,
    pub attn_q_norm: Vec<GpuBuffer>,
    pub attn_k_norm: Vec<Option<GpuBuffer>>,
    pub post_attention_norm: Vec<GpuBuffer>,
    pub ffn_norm: Vec<GpuBuffer>,
    pub post_ffw_norm: Vec<GpuBuffer>,
    pub post_norm: Vec<GpuBuffer>,
    pub layer_output_scale: Vec<Option<GpuBuffer>>,

    pub blas: BlasHandle,
    pub vram_used_bytes: u64,
}

/// Upload mode for PLE — large table can stay on CPU if VRAM is tight.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PleMode {
    Gpu,  // upload per_layer_token_embd to GPU (4.7 GB)
    Cpu,  // keep on CPU, lookup-stream per-token rows (slower but saves 4.7 GB)
}

impl Gemma4GpuWeights {
    /// Upload all weights from CPU to GPU. Returns Ok with VRAM usage stats
    /// printed to stdout.
    pub fn upload(cpu: &CpuWeightsGemma4, ple_mode: PleMode) -> Result<Self, String> {
        // Initialize GPU device 0 (west's RX 7700 XT)
        let _device = GpuDevice::init(0)
            .map_err(|e| format!("GPU init failed: {:?}", e))?;
        let blas = BlasHandle::new()
            .map_err(|e| format!("hipBLAS init failed: {:?}", e))?;

        let mut total_vram: u64 = 0;
        let h = cpu.hparams.clone();
        let n_layer = h.n_layer;

        let upload_bf16 = |data: &[bf16]| -> Result<GpuBuffer, String> {
            let n_bytes = data.len() * 2;
            let buf = GpuBuffer::alloc(n_bytes)
                .map_err(|e| format!("alloc {} bytes failed: {:?}", n_bytes, e))?;
            // SAFETY: bf16 is 2 bytes, data layout matches what GPU expects.
            let bytes = unsafe {
                std::slice::from_raw_parts(data.as_ptr() as *const u8, n_bytes)
            };
            buf.copy_from_host(bytes)
                .map_err(|e| format!("copy bf16 to GPU failed: {:?}", e))?;
            Ok(buf)
        };
        let upload_f32 = |data: &[f32]| -> Result<GpuBuffer, String> {
            let n_bytes = data.len() * 4;
            let buf = GpuBuffer::alloc(n_bytes)
                .map_err(|e| format!("alloc {} bytes failed: {:?}", n_bytes, e))?;
            buf.upload_f32(data)
                .map_err(|e| format!("upload f32 failed: {:?}", e))?;
            Ok(buf)
        };

        println!("Uploading Gemma 4 weights to GPU...");

        // Globals
        let token_embd = upload_bf16(&cpu.token_embd)?;
        total_vram += (cpu.token_embd.len() * 2) as u64;
        println!("  token_embd:           {:>7.1} MB", cpu.token_embd.len() as f64 * 2.0 / 1e6);

        let per_layer_token_embd = if ple_mode == PleMode::Gpu {
            let buf = upload_bf16(&cpu.per_layer_token_embd)?;
            total_vram += (cpu.per_layer_token_embd.len() * 2) as u64;
            println!("  per_layer_token_embd: {:>7.1} MB", cpu.per_layer_token_embd.len() as f64 * 2.0 / 1e6);
            Some(buf)
        } else {
            println!("  per_layer_token_embd: CPU-resident (PleMode::Cpu)");
            None
        };

        let per_layer_model_proj = upload_bf16(&cpu.per_layer_model_proj)?;
        total_vram += (cpu.per_layer_model_proj.len() * 2) as u64;
        let output_norm = upload_f32(&cpu.output_norm)?;
        total_vram += (cpu.output_norm.len() * 4) as u64;
        let per_layer_proj_norm = upload_f32(&cpu.per_layer_proj_norm)?;
        total_vram += (cpu.per_layer_proj_norm.len() * 4) as u64;
        let rope_freqs = upload_f32(&cpu.rope_freqs)?;
        total_vram += (cpu.rope_freqs.len() * 4) as u64;

        // Per-layer
        let mut wq = Vec::with_capacity(n_layer);
        let mut wk = Vec::with_capacity(n_layer);
        let mut wv = Vec::with_capacity(n_layer);
        let mut wo = Vec::with_capacity(n_layer);
        let mut ffn_gate = Vec::with_capacity(n_layer);
        let mut ffn_up = Vec::with_capacity(n_layer);
        let mut ffn_down = Vec::with_capacity(n_layer);
        let mut inp_gate = Vec::with_capacity(n_layer);
        let mut proj = Vec::with_capacity(n_layer);
        let mut attn_norm = Vec::with_capacity(n_layer);
        let mut attn_q_norm = Vec::with_capacity(n_layer);
        let mut attn_k_norm = Vec::with_capacity(n_layer);
        let mut post_attention_norm = Vec::with_capacity(n_layer);
        let mut ffn_norm = Vec::with_capacity(n_layer);
        let mut post_ffw_norm = Vec::with_capacity(n_layer);
        let mut post_norm = Vec::with_capacity(n_layer);
        let mut layer_output_scale = Vec::with_capacity(n_layer);

        for il in 0..n_layer {
            wq.push(upload_bf16(&cpu.wq[il])?);
            total_vram += (cpu.wq[il].len() * 2) as u64;
            if let Some(w) = &cpu.wk[il] {
                wk.push(Some(upload_bf16(w)?));
                total_vram += (w.len() * 2) as u64;
            } else {
                wk.push(None);
            }
            if let Some(w) = &cpu.wv[il] {
                wv.push(Some(upload_bf16(w)?));
                total_vram += (w.len() * 2) as u64;
            } else {
                wv.push(None);
            }
            wo.push(upload_bf16(&cpu.wo[il])?);
            total_vram += (cpu.wo[il].len() * 2) as u64;
            ffn_gate.push(upload_bf16(&cpu.ffn_gate[il])?);
            total_vram += (cpu.ffn_gate[il].len() * 2) as u64;
            ffn_up.push(upload_bf16(&cpu.ffn_up[il])?);
            total_vram += (cpu.ffn_up[il].len() * 2) as u64;
            ffn_down.push(upload_bf16(&cpu.ffn_down[il])?);
            total_vram += (cpu.ffn_down[il].len() * 2) as u64;
            inp_gate.push(upload_bf16(&cpu.inp_gate[il])?);
            total_vram += (cpu.inp_gate[il].len() * 2) as u64;
            proj.push(upload_bf16(&cpu.proj[il])?);
            total_vram += (cpu.proj[il].len() * 2) as u64;

            attn_norm.push(upload_f32(&cpu.attn_norm[il])?);
            total_vram += (cpu.attn_norm[il].len() * 4) as u64;
            attn_q_norm.push(upload_f32(&cpu.attn_q_norm[il])?);
            total_vram += (cpu.attn_q_norm[il].len() * 4) as u64;
            if let Some(n) = &cpu.attn_k_norm[il] {
                attn_k_norm.push(Some(upload_f32(n)?));
                total_vram += (n.len() * 4) as u64;
            } else {
                attn_k_norm.push(None);
            }
            post_attention_norm.push(upload_f32(&cpu.post_attention_norm[il])?);
            total_vram += (cpu.post_attention_norm[il].len() * 4) as u64;
            ffn_norm.push(upload_f32(&cpu.ffn_norm[il])?);
            total_vram += (cpu.ffn_norm[il].len() * 4) as u64;
            post_ffw_norm.push(upload_f32(&cpu.post_ffw_norm[il])?);
            total_vram += (cpu.post_ffw_norm[il].len() * 4) as u64;
            post_norm.push(upload_f32(&cpu.post_norm[il])?);
            total_vram += (cpu.post_norm[il].len() * 4) as u64;

            if let Some(s) = cpu.layer_output_scale[il] {
                layer_output_scale.push(Some(upload_f32(&[s])?));
                total_vram += 4;
            } else {
                layer_output_scale.push(None);
            }
        }

        println!("  per-layer weights:    {:>7.1} MB ({} layers)",
            (total_vram as f64 / 1e6) - (cpu.token_embd.len() as f64 * 2.0 / 1e6)
                - (per_layer_token_embd.as_ref().map(|_| cpu.per_layer_token_embd.len() as f64 * 2.0 / 1e6).unwrap_or(0.0))
                - (cpu.per_layer_model_proj.len() as f64 * 2.0 / 1e6)
                - (cpu.output_norm.len() as f64 * 4.0 / 1e6)
                - (cpu.per_layer_proj_norm.len() as f64 * 4.0 / 1e6)
                - (cpu.rope_freqs.len() as f64 * 4.0 / 1e6),
            n_layer);
        println!("  TOTAL VRAM USED:      {:>7.2} GB", total_vram as f64 / 1e9);

        Ok(Self {
            hparams: h,
            token_embd,
            per_layer_token_embd,
            per_layer_model_proj,
            output_norm,
            per_layer_proj_norm,
            rope_freqs,
            wq, wk, wv, wo,
            ffn_gate, ffn_up, ffn_down,
            inp_gate, proj,
            attn_norm, attn_q_norm, attn_k_norm,
            post_attention_norm, ffn_norm, post_ffw_norm, post_norm,
            layer_output_scale,
            blas,
            vram_used_bytes: total_vram,
        })
    }

    pub fn vram_used_gb(&self) -> f64 {
        self.vram_used_bytes as f64 / 1e9
    }

    /// Generic GPU matmul: out[m, n] = input_f32[m, k] @ w_gpu_bf16[n, k]^T.
    /// Mirrors CPU matmul_x_wt's call convention. Input is converted to bf16
    /// on host then uploaded; output is downloaded as f32.
    /// Per-call overhead: ~50-100 µs (PCIe + sync). For seq=4 this is dwarfed
    /// by the matmul itself; for larger seq, batch transfers help.
    pub fn matmul_xwt(
        &self,
        w_gpu_bf16: &GpuBuffer,
        input_f32: &[f32],
        m: usize, n: usize, k: usize,
    ) -> Result<Vec<f32>, String> {
        debug_assert_eq!(input_f32.len(), m * k);
        // f32 → bf16 host-side. Cheap (~ns/element).
        let mut input_bf16: Vec<bf16> = Vec::with_capacity(input_f32.len());
        for &f in input_f32 {
            input_bf16.push(bf16::from_f32(f));
        }
        let n_bytes_in = input_bf16.len() * 2;
        let input_buf = GpuBuffer::alloc(n_bytes_in)
            .map_err(|e| format!("alloc input: {:?}", e))?;
        let bytes = unsafe {
            std::slice::from_raw_parts(input_bf16.as_ptr() as *const u8, n_bytes_in)
        };
        input_buf.copy_from_host(bytes)
            .map_err(|e| format!("upload input: {:?}", e))?;

        let out_buf = GpuBuffer::alloc(m * n * 4)
            .map_err(|e| format!("alloc output: {:?}", e))?;

        self.blas.sgemm_bf16_ex(
            true, false,
            n as i32, m as i32, k as i32,
            1.0,
            w_gpu_bf16, k as i32,
            &input_buf, k as i32,
            0.0,
            &out_buf, n as i32,
        ).map_err(|e| format!("sgemm_bf16: {:?}", e))?;
        crate::hip::sync().map_err(|e| format!("sync: {:?}", e))?;

        let mut out = vec![0.0f32; m * n];
        out_buf.download_f32(&mut out)
            .map_err(|e| format!("download: {:?}", e))?;
        Ok(out)
    }

    /// Test helper: matmul one input vector through layer `il`'s wq weight
    /// using GPU sgemm_bf16. Returns output [seq, q_out_dim] as f32.
    ///
    /// Forward analog of the matmul_x_wt(input, wq_f32, seq, q_out_dim, n_embd)
    /// call in forward_gemma4_with_lora step 2b.
    ///
    /// hipBLAS is column-major; to compute row-major out[m,n] = a[m,k] @ w[n,k]^T,
    /// we transpose-A and call gemm with op_a=true, op_b=false on swapped
    /// dimensions. See: gemma4_gpu test_gemma4_gpu_wq_matches_cpu.
    pub fn wq_matmul(
        &self,
        input: &[f32],
        seq: usize,
        il: usize,
    ) -> Result<Vec<f32>, String> {
        let h = &self.hparams;
        let head_dim = h.head_dim(il);
        let n = h.n_head * head_dim; // q_out_dim
        let k = h.n_embd;
        let m = seq;

        // Convert input f32 → bf16 on host, upload
        let input_bf16: Vec<bf16> = input.iter().map(|f| bf16::from_f32(*f)).collect();
        let n_in_bytes = input_bf16.len() * 2;
        let input_buf = GpuBuffer::alloc(n_in_bytes)
            .map_err(|e| format!("alloc input failed: {:?}", e))?;
        let in_bytes = unsafe {
            std::slice::from_raw_parts(input_bf16.as_ptr() as *const u8, n_in_bytes)
        };
        input_buf.copy_from_host(in_bytes)
            .map_err(|e| format!("copy input to GPU: {:?}", e))?;

        // Output buffer f32
        let out_size = m * n;
        let out_buf = GpuBuffer::alloc(out_size * 4)
            .map_err(|e| format!("alloc output failed: {:?}", e))?;

        // sgemm_bf16_ex with op_a=transpose(W), op_b=none(input)
        // Compute (in col-major view): C_cm[n,m] = W_cm^T[n,k] @ A_cm[k,m]
        // which is equivalent to row-major C[m,n] = A[m,k] @ W[n,k]^T.
        // Args: M_hipblas=n, N_hipblas=m, K_hipblas=k.
        // Leading dims (col-major): lda=k for W (transposed), ldb=k for A,
        // ldc=n for C.
        self.blas.sgemm_bf16_ex(
            true, false,
            n as i32, m as i32, k as i32,
            1.0,
            &self.wq[il], k as i32,
            &input_buf, k as i32,
            0.0,
            &out_buf, n as i32,
        ).map_err(|e| format!("sgemm_bf16 failed: {:?}", e))?;
        crate::hip::sync().map_err(|e| format!("sync: {:?}", e))?;

        let mut out = vec![0.0f32; out_size];
        out_buf.download_f32(&mut out)
            .map_err(|e| format!("download output: {:?}", e))?;
        Ok(out)
    }
}

// =============================================================================
// Phase 7 Step B — forward_gemma4_gpu.
//
// Drop-in GPU version of gemma4::forward_gemma4_with_lora. Replaces every
// matmul_x_wt call with self.matmul_xwt (bf16 weights stay GPU-resident).
// All other ops (rmsnorm, RoPE, attention, GELU, softcap, embedding lookup,
// PLE table lookup) stay on CPU — they're fast enough that GPU porting them
// would mostly be PCIe shuffling. Phase 7b would custom-kernel them.
//
// Returns (logits, layer_caches) compatible with backward_gemma4_with_lora —
// caller can interleave GPU forward with CPU backward (or eventually GPU
// backward when Step C lands).
// =============================================================================

use crate::gemma4::{Gemma4LoraAdapters, Gemma4LayerCache, Gemma4PleCache};

pub fn forward_gemma4_gpu(
    cpu: &CpuWeightsGemma4,
    gpu: &Gemma4GpuWeights,
    lora: Option<&Gemma4LoraAdapters>,
    tokens: &[u32],
) -> Result<(Vec<f32>, Vec<Gemma4LayerCache>), String> {
    use crate::forward;

    let h = &cpu.hparams;
    let seq = tokens.len();
    let n_embd = h.n_embd;
    let row_size = h.n_embd_per_layer * h.n_layer;

    // 1. Embedding lookup + sqrt(n_embd) scale (CPU — fast indexing).
    let tok_embd_f32: Vec<f32> = cpu.token_embd.iter().map(|b| b.to_f32()).collect();
    let mut hidden = forward::embedding_lookup(&tok_embd_f32, n_embd, tokens);
    let embed_scale = (n_embd as f32).sqrt();
    for v in hidden.iter_mut() { *v *= embed_scale; }

    // 1b. PLE setup (per_layer_model_proj matmul on GPU; lookup on CPU).
    let inp_per_layer: Option<Vec<f32>> = if h.n_embd_per_layer > 0 {
        Some(compute_inp_per_layer_gpu(cpu, gpu, tokens, &hidden)?)
    } else {
        None
    };

    let mut layer_caches: Vec<Gemma4LayerCache> = Vec::with_capacity(h.n_layer);

    for il in 0..h.n_layer {
        let head_dim = h.head_dim(il);
        let n_head = h.n_head;
        let n_head_kv = h.n_head_kv;
        let q_out_dim = n_head * head_dim;
        let kv_out_dim = n_head_kv * head_dim;

        // 2a. attn_norm (CPU, f32 weights).
        let mut normed = vec![0.0f32; seq * n_embd];
        for s in 0..seq {
            let row = forward::rmsnorm(
                &hidden[s * n_embd..(s + 1) * n_embd],
                &cpu.attn_norm[il],
                h.rms_norm_eps,
            );
            normed[s * n_embd..(s + 1) * n_embd].copy_from_slice(&row);
        }

        // 2b. Q projection (GPU) + LoRA Q (CPU)
        let mut q = gpu.matmul_xwt(&gpu.wq[il], &normed, seq, q_out_dim, n_embd)?;
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

        // 2c. K, V projections (GPU if has_kv) + LoRA K, V (CPU)
        let (k_flat, v_flat) = if let Some(wk_buf) = &gpu.wk[il] {
            let wv_buf = gpu.wv[il].as_ref().unwrap_or(wk_buf);
            let mut k = gpu.matmul_xwt(wk_buf, &normed, seq, kv_out_dim, n_embd)?;
            let mut v = gpu.matmul_xwt(wv_buf, &normed, seq, kv_out_dim, n_embd)?;
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
            // KV-reusing layer — use previous layer's cached K/V.
            let prev = layer_caches.last().unwrap();
            (prev.k_rot.clone(), prev.v.clone())
        };

        // 2d. Per-head Q-norm, K-norm, weightless V-norm (CPU, f32 weights)
        let q_normed = per_head_rmsnorm_cpu(
            &q, &cpu.attn_q_norm[il], seq, n_head, head_dim, h.rms_norm_eps);
        let k_normed = if let Some(k_norm) = &cpu.attn_k_norm[il] {
            per_head_rmsnorm_cpu(&k_flat, k_norm, seq, n_head_kv, head_dim, h.rms_norm_eps)
        } else {
            k_flat.clone()
        };
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

        // 2e. RoPE (CPU)
        let rope_dim = h.rope_dim(il);
        let freq_base = h.rope_freq_base(il);
        let (cos_table, sin_table) = forward::rope_freqs(seq, rope_dim, freq_base);
        let q_rot = rope_apply_partial_cpu(&q_normed, &cos_table, &sin_table, seq, n_head, head_dim, rope_dim);
        let k_rot = rope_apply_partial_cpu(&k_normed, &cos_table, &sin_table, seq, n_head_kv, head_dim, rope_dim);

        // 2f. Attention (CPU)
        let mask = if h.is_sliding(il) {
            forward::AttnMask::SlidingWindow(h.sliding_window)
        } else {
            forward::AttnMask::Causal
        };
        let (attn_out_head, attn_cache) = forward::attention_forward(
            &q_rot, &k_rot, &v_normed,
            n_head, n_head_kv, head_dim, seq, mask,
        );

        // 2g. O projection (GPU) + LoRA O (CPU)
        let mut o_out = gpu.matmul_xwt(&gpu.wo[il], &attn_out_head, seq, n_embd, q_out_dim)?;
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

        // 2h. post_attention_norm + residual (CPU)
        let mut post_attn = vec![0.0f32; seq * n_embd];
        for s in 0..seq {
            let row = forward::rmsnorm(
                &o_out[s * n_embd..(s + 1) * n_embd],
                &cpu.post_attention_norm[il],
                h.rms_norm_eps,
            );
            for d in 0..n_embd {
                post_attn[s * n_embd + d] = row[d] + hidden[s * n_embd + d];
            }
        }

        // 2i. FFN — norm (CPU), gate/up/down matmul (GPU), GELU + mul (CPU)
        let mut ffn_normed = vec![0.0f32; seq * n_embd];
        for s in 0..seq {
            let row = forward::rmsnorm(
                &post_attn[s * n_embd..(s + 1) * n_embd],
                &cpu.ffn_norm[il],
                h.rms_norm_eps,
            );
            ffn_normed[s * n_embd..(s + 1) * n_embd].copy_from_slice(&row);
        }
        let gate_pre = gpu.matmul_xwt(&gpu.ffn_gate[il], &ffn_normed, seq, h.n_ff, n_embd)?;
        let up_pre = gpu.matmul_xwt(&gpu.ffn_up[il], &ffn_normed, seq, h.n_ff, n_embd)?;
        let mut ffn_hidden = vec![0.0f32; seq * h.n_ff];
        for i in 0..ffn_hidden.len() {
            ffn_hidden[i] = forward::gelu_tanh_approx(gate_pre[i]) * up_pre[i];
        }
        let ffn_out = gpu.matmul_xwt(&gpu.ffn_down[il], &ffn_hidden, seq, n_embd, h.n_ff)?;

        // 2j. post_ffw_norm + residual
        let mut layer_out = vec![0.0f32; seq * n_embd];
        for s in 0..seq {
            let row = forward::rmsnorm(
                &ffn_out[s * n_embd..(s + 1) * n_embd],
                &cpu.post_ffw_norm[il],
                h.rms_norm_eps,
            );
            for d in 0..n_embd {
                layer_out[s * n_embd + d] = row[d] + post_attn[s * n_embd + d];
            }
        }

        // PLE chain (matmuls on GPU)
        let ple_cache = if let Some(ref ipl) = inp_per_layer {
            let pe_in = layer_out.clone();
            let n_epl = h.n_embd_per_layer;
            let inp_layer_slice = slice_layer_cpu(ipl, seq, h.n_layer, n_epl, il);

            let gate_pre_gelu = gpu.matmul_xwt(&gpu.inp_gate[il], &pe_in, seq, n_epl, n_embd)?;
            let mut gate_post_gelu = gate_pre_gelu.clone();
            for v in gate_post_gelu.iter_mut() { *v = forward::gelu_tanh_approx(*v); }
            let mut gated = vec![0.0f32; gate_post_gelu.len()];
            for i in 0..gated.len() {
                gated[i] = gate_post_gelu[i] * inp_layer_slice[i];
            }
            let proj_out_pre_norm = gpu.matmul_xwt(&gpu.proj[il], &gated, seq, n_embd, n_epl)?;
            let mut proj_normed = vec![0.0f32; seq * n_embd];
            for s in 0..seq {
                let row = forward::rmsnorm(
                    &proj_out_pre_norm[s * n_embd..(s + 1) * n_embd],
                    &cpu.post_norm[il],
                    h.rms_norm_eps,
                );
                proj_normed[s * n_embd..(s + 1) * n_embd].copy_from_slice(&row);
            }
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

        if let Some(scale_v) = cpu.layer_output_scale[il] {
            for v in layer_out.iter_mut() { *v *= scale_v; }
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

    // Final output_norm (CPU)
    let mut final_hidden = vec![0.0f32; seq * n_embd];
    for s in 0..seq {
        let row = forward::rmsnorm(
            &hidden[s * n_embd..(s + 1) * n_embd],
            &cpu.output_norm,
            h.rms_norm_eps,
        );
        final_hidden[s * n_embd..(s + 1) * n_embd].copy_from_slice(&row);
    }

    // LM head (GPU — token_embd is already on GPU, treated as W^T)
    let logits = gpu.matmul_xwt(&gpu.token_embd, &final_hidden, seq, h.vocab_size, n_embd)?;

    // Softcap (CPU)
    let logits = if h.final_logit_softcapping > 0.0 {
        let cap = h.final_logit_softcapping;
        logits.into_iter().map(|x| forward::logit_softcap(x, cap)).collect()
    } else {
        logits
    };

    let _ = row_size; // silence unused (used only when PLE active)
    Ok((logits, layer_caches))
}

/// PLE setup helper — GPU matmul for per_layer_model_proj.
fn compute_inp_per_layer_gpu(
    cpu: &CpuWeightsGemma4,
    gpu: &Gemma4GpuWeights,
    tokens: &[u32],
    scaled_embeddings: &[f32],
) -> Result<Vec<f32>, String> {
    use crate::forward;
    let h = &cpu.hparams;
    let n_tokens = tokens.len();
    let n_embd = h.n_embd;
    let n_epl = h.n_embd_per_layer;
    let n_layer = h.n_layer;
    let row_size = n_epl * n_layer;

    // Look up + scale (CPU, accesses CpuWeightsGemma4.per_layer_token_embd)
    let mut tok_embd_per_layer = vec![0.0f32; n_tokens * row_size];
    let scale_tok = (n_epl as f32).sqrt();
    for (ti, &tok) in tokens.iter().enumerate() {
        let src_off = tok as usize * row_size;
        for d in 0..row_size {
            if src_off + d < cpu.per_layer_token_embd.len() {
                tok_embd_per_layer[ti * row_size + d] =
                    cpu.per_layer_token_embd[src_off + d].to_f32() * scale_tok;
            }
        }
    }

    // Project on GPU
    let mut proj_out = gpu.matmul_xwt(
        &gpu.per_layer_model_proj, scaled_embeddings, n_tokens, row_size, n_embd)?;
    let scale_proj = 1.0 / (n_embd as f32).sqrt();
    for v in proj_out.iter_mut() { *v *= scale_proj; }

    // Rest is CPU
    for ti in 0..n_tokens {
        for li in 0..n_layer {
            let off = ti * row_size + li * n_epl;
            let slice = &proj_out[off..off + n_epl];
            let normed = forward::rmsnorm(slice, &cpu.per_layer_proj_norm, h.rms_norm_eps);
            proj_out[off..off + n_epl].copy_from_slice(&normed);
        }
    }
    let inv_sqrt2 = 1.0f32 / 2.0f32.sqrt();
    for i in 0..proj_out.len() {
        proj_out[i] = (proj_out[i] + tok_embd_per_layer[i]) * inv_sqrt2;
    }
    Ok(proj_out)
}

fn slice_layer_cpu(inp_per_layer: &[f32], seq: usize, n_layer: usize, n_epl: usize, l: usize) -> Vec<f32> {
    let mut out = Vec::with_capacity(seq * n_epl);
    for t in 0..seq {
        let off = t * n_layer * n_epl + l * n_epl;
        out.extend_from_slice(&inp_per_layer[off..off + n_epl]);
    }
    out
}

fn per_head_rmsnorm_cpu(x: &[f32], weight: &[f32], seq: usize, n_head: usize, head_dim: usize, eps: f32) -> Vec<f32> {
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

fn rope_apply_partial_cpu(
    x: &[f32], cos: &[f32], sin: &[f32],
    seq: usize, n_head: usize, head_dim: usize, rope_dim: usize,
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
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::gguf::GgufFile;

    #[test]
    fn test_gemma4_gpu_upload_full() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            println!("GGUF not on disk — skipping");
            return;
        }
        let model = GgufFile::open(model_path).expect("open");
        let weights = CpuWeightsGemma4::load(&model).expect("load");

        let t0 = std::time::Instant::now();
        let gpu_weights = match Gemma4GpuWeights::upload(&weights, PleMode::Gpu) {
            Ok(w) => w,
            Err(e) => panic!("Upload failed (PleMode::Gpu): {}", e),
        };
        let upload_secs = t0.elapsed().as_secs_f64();
        println!("Upload time: {:.1}s", upload_secs);
        println!("VRAM used: {:.2} GB", gpu_weights.vram_used_gb());
        // Sanity: ~9 GB expected for E2B
        assert!(gpu_weights.vram_used_gb() > 8.0 && gpu_weights.vram_used_gb() < 11.0,
            "VRAM should be 8-11 GB, got {:.2} GB", gpu_weights.vram_used_gb());
    }

    #[test]
    fn test_gemma4_gpu_upload_cpu_ple() {
        // Skips PLE — should drop ~4.7 GB
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            return;
        }
        let model = GgufFile::open(model_path).expect("open");
        let weights = CpuWeightsGemma4::load(&model).expect("load");
        let gpu_weights = match Gemma4GpuWeights::upload(&weights, PleMode::Cpu) {
            Ok(w) => w,
            Err(e) => panic!("Upload failed (PleMode::Cpu): {}", e),
        };
        println!("VRAM used (CPU PLE): {:.2} GB", gpu_weights.vram_used_gb());
        assert!(gpu_weights.vram_used_gb() < 6.0,
            "VRAM with CPU PLE should be < 6 GB, got {:.2}", gpu_weights.vram_used_gb());
    }

    #[test]
    fn test_gemma4_gpu_wq_matches_cpu() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            return;
        }
        let model = GgufFile::open(model_path).expect("open");
        let cpu = CpuWeightsGemma4::load(&model).expect("load cpu");
        let gpu = Gemma4GpuWeights::upload(&cpu, PleMode::Cpu).expect("upload");

        // Pick layer 0 (sliding, head_dim=256, q_out_dim=2048).
        let il = 0;
        let h = &cpu.hparams;
        let seq = 4;
        let n_embd = h.n_embd;
        let q_out_dim = h.n_head * h.head_dim(il);

        // Random f32 input [seq, n_embd]
        let mut input = Vec::with_capacity(seq * n_embd);
        let mut s = 0xbadcab1eu64;
        for _ in 0..(seq * n_embd) {
            s = s.wrapping_mul(6364136223846793005).wrapping_add(1442695040888963407);
            let u = ((s >> 33) as f32) / (u32::MAX as f32) * 2.0 - 1.0;
            input.push(u * 0.1);
        }

        // CPU reference: matmul_x_wt(input, wq_f32, seq, q_out_dim, n_embd)
        let wq_f32: Vec<f32> = cpu.wq[il].iter().map(|b| b.to_f32()).collect();
        let mut cpu_out = vec![0.0f32; seq * q_out_dim];
        for i in 0..seq {
            for j in 0..q_out_dim {
                let mut acc = 0.0f32;
                for l in 0..n_embd {
                    acc += input[i * n_embd + l] * wq_f32[j * n_embd + l];
                }
                cpu_out[i * q_out_dim + j] = acc;
            }
        }

        // GPU candidate
        let gpu_out = gpu.wq_matmul(&input, seq, il).expect("gpu matmul");

        assert_eq!(cpu_out.len(), gpu_out.len());

        // Magnitude-aware metrics. bf16 input precision (~3e-3 relative) plus
        // sqrt(k) accumulation in dot product means small-magnitude outputs
        // are naturally noisier in absolute terms. Use:
        //   - cosine similarity (scale-invariant, structural correctness)
        //   - rel err only on entries above 1% of max magnitude
        let max_mag = cpu_out.iter().map(|x| x.abs()).fold(0.0f32, f32::max);
        let mut dot_cpu_gpu = 0.0f64;
        let mut sq_cpu = 0.0f64;
        let mut sq_gpu = 0.0f64;
        let mut max_abs_err = 0.0f32;
        let mut worst_rel_err = 0.0f32;
        let mut n_above_threshold = 0;
        for i in 0..cpu_out.len() {
            let a = cpu_out[i];
            let b = gpu_out[i];
            assert!(a.is_finite() && b.is_finite());
            dot_cpu_gpu += (a as f64) * (b as f64);
            sq_cpu += (a as f64).powi(2);
            sq_gpu += (b as f64).powi(2);
            let abs_err = (a - b).abs();
            max_abs_err = max_abs_err.max(abs_err);
            if a.abs() > 0.01 * max_mag {
                n_above_threshold += 1;
                let rel = abs_err / a.abs();
                worst_rel_err = worst_rel_err.max(rel);
            }
        }
        let cos_sim = (dot_cpu_gpu / (sq_cpu.sqrt() * sq_gpu.sqrt())) as f32;
        println!("CPU vs GPU wq matmul (layer 0, seq=4, q_out_dim={})", q_out_dim);
        println!("  cosine sim:        {:.6}", cos_sim);
        println!("  max abs err:       {:.4e}", max_abs_err);
        println!("  CPU max magnitude: {:.4e}", max_mag);
        println!("  worst rel err on entries > 1% max_mag ({} entries): {:.4e}",
            n_above_threshold, worst_rel_err);

        assert!(cos_sim > 0.9999,
            "cosine similarity should be > 0.9999, got {}", cos_sim);
        // bf16 precision: input round-trip ~3e-3 per element, accumulated over
        // k=1536 dot product → expected per-output error ~ sqrt(k)*eps ~ 12% in
        // worst case. 10% threshold catches structural bugs without flagging
        // bf16 noise.
        assert!(worst_rel_err < 0.10,
            "rel err on significant entries should be < 10%, got {:.4e}", worst_rel_err);
    }

    #[test]
    fn test_gemma4_gpu_forward_matches_cpu() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            return;
        }
        let model = GgufFile::open(model_path).expect("open");
        let cpu = CpuWeightsGemma4::load(&model).expect("load cpu");
        let gpu = Gemma4GpuWeights::upload(&cpu, PleMode::Cpu).expect("upload");

        let tokens: Vec<u32> = vec![2, 1000, 2000, 3000];

        // CPU reference forward
        println!("CPU forward (reference)...");
        let t0 = std::time::Instant::now();
        let (cpu_logits, _cpu_caches) = crate::gemma4::forward_gemma4(&cpu, &tokens);
        let cpu_secs = t0.elapsed().as_secs_f64();
        println!("  CPU forward: {:.1}s", cpu_secs);

        // GPU forward
        println!("GPU forward (matmuls on hipBLAS sgemm_bf16)...");
        let t0 = std::time::Instant::now();
        let (gpu_logits, _gpu_caches) =
            super::forward_gemma4_gpu(&cpu, &gpu, None, &tokens).expect("gpu forward");
        let gpu_secs = t0.elapsed().as_secs_f64();
        println!("  GPU forward: {:.1}s", gpu_secs);
        println!("  Speedup:     {:.1}x", cpu_secs / gpu_secs);

        assert_eq!(cpu_logits.len(), gpu_logits.len());

        // Compare logits at the LAST position (where prediction matters most).
        let vocab = cpu.hparams.vocab_size;
        let last_pos = tokens.len() - 1;
        let cpu_last = &cpu_logits[last_pos * vocab..(last_pos + 1) * vocab];
        let gpu_last = &gpu_logits[last_pos * vocab..(last_pos + 1) * vocab];

        let mut dot = 0.0f64;
        let mut sq_cpu = 0.0f64;
        let mut sq_gpu = 0.0f64;
        let mut top1_match_at = 0;
        let mut argmax_cpu_val = f32::NEG_INFINITY;
        let mut argmax_cpu_idx = 0;
        let mut argmax_gpu_val = f32::NEG_INFINITY;
        let mut argmax_gpu_idx = 0;
        for i in 0..vocab {
            dot += cpu_last[i] as f64 * gpu_last[i] as f64;
            sq_cpu += (cpu_last[i] as f64).powi(2);
            sq_gpu += (gpu_last[i] as f64).powi(2);
            if cpu_last[i] > argmax_cpu_val { argmax_cpu_val = cpu_last[i]; argmax_cpu_idx = i; }
            if gpu_last[i] > argmax_gpu_val { argmax_gpu_val = gpu_last[i]; argmax_gpu_idx = i; }
        }
        let cos_sim = (dot / (sq_cpu.sqrt() * sq_gpu.sqrt())) as f32;
        if argmax_cpu_idx == argmax_gpu_idx { top1_match_at = 1; }

        // Top-5 overlap
        let mut cpu_top: Vec<(usize, f32)> = cpu_last.iter().copied().enumerate().collect();
        cpu_top.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap());
        let mut gpu_top: Vec<(usize, f32)> = gpu_last.iter().copied().enumerate().collect();
        gpu_top.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap());
        let cpu_top5: std::collections::HashSet<usize> = cpu_top[..5].iter().map(|x| x.0).collect();
        let gpu_top5: std::collections::HashSet<usize> = gpu_top[..5].iter().map(|x| x.0).collect();
        let top5_overlap = cpu_top5.intersection(&gpu_top5).count();

        println!("Logits comparison @ last position (vocab={}):", vocab);
        println!("  cosine sim:        {:.6}", cos_sim);
        println!("  argmax CPU:        token {} (logit {:.3})", argmax_cpu_idx, argmax_cpu_val);
        println!("  argmax GPU:        token {} (logit {:.3})", argmax_gpu_idx, argmax_gpu_val);
        println!("  top-1 match:       {}", top1_match_at == 1);
        println!("  top-5 overlap:     {}/5", top5_overlap);
        println!("  CPU top-5: {:?}", &cpu_top[..5]);
        println!("  GPU top-5: {:?}", &gpu_top[..5]);

        assert!(cos_sim > 0.99,
            "logits cosine similarity should be > 0.99, got {}", cos_sim);
        // Top-1 may differ if logits are very close — top-3 overlap is the
        // robust check.
        assert!(top5_overlap >= 3,
            "top-5 should overlap by ≥3, got {} (CPU vs GPU divergent)", top5_overlap);
    }

    #[test]
    fn test_gemma4_gpu_wq_speedup() {
        let model_path = "/var/zhen/models/gemma-4-E2B-it.gguf";
        if !std::path::Path::new(model_path).exists() {
            return;
        }
        let model = GgufFile::open(model_path).expect("open");
        let cpu = CpuWeightsGemma4::load(&model).expect("load cpu");
        let gpu = Gemma4GpuWeights::upload(&cpu, PleMode::Cpu).expect("upload");

        let il = 0;
        let h = &cpu.hparams;
        let seq = 4;
        let n_embd = h.n_embd;
        let q_out_dim = h.n_head * h.head_dim(il);

        let mut input = vec![0.0f32; seq * n_embd];
        for i in 0..input.len() {
            input[i] = ((i * 7 + 3) as f32 * 0.001).sin() * 0.1;
        }

        let n_iter = 10;

        // Warm up + time GPU
        for _ in 0..3 { let _ = gpu.wq_matmul(&input, seq, il).unwrap(); }
        let t0 = std::time::Instant::now();
        for _ in 0..n_iter {
            let _ = gpu.wq_matmul(&input, seq, il).unwrap();
        }
        let gpu_secs = t0.elapsed().as_secs_f64() / n_iter as f64;

        // Time CPU (matches forward_gemma4_with_lora's pattern: bf16→f32 + matmul)
        let cpu_matmul = |input: &[f32]| -> Vec<f32> {
            let wq_f32: Vec<f32> = cpu.wq[il].iter().map(|b| b.to_f32()).collect();
            let mut out = vec![0.0f32; seq * q_out_dim];
            for i in 0..seq {
                for j in 0..q_out_dim {
                    let mut acc = 0.0f32;
                    for l in 0..n_embd {
                        acc += input[i * n_embd + l] * wq_f32[j * n_embd + l];
                    }
                    out[i * q_out_dim + j] = acc;
                }
            }
            out
        };
        for _ in 0..1 { let _ = cpu_matmul(&input); }
        let t0 = std::time::Instant::now();
        for _ in 0..n_iter {
            let _ = cpu_matmul(&input);
        }
        let cpu_secs = t0.elapsed().as_secs_f64() / n_iter as f64;

        println!("wq matmul timing (4×1536 → 4×{}, n={} iterations):", q_out_dim, n_iter);
        println!("  CPU (incl bf16→f32 conversion): {:.4}s", cpu_secs);
        println!("  GPU (incl input upload + sync): {:.4}s", gpu_secs);
        println!("  Speedup:                         {:.1}x", cpu_secs / gpu_secs);
    }

    #[allow(dead_code)]
    fn _suppress(_: HipError) {}
}
