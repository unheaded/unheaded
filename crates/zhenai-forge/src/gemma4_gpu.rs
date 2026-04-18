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

    #[allow(dead_code)]
    fn _suppress(_: HipError) {}
}
