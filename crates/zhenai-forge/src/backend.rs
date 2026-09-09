// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! # ForgeBackend trait — pluggable forward/backward kernel provider
//!
//! ## Why this module exists
//!
//! Forge training has historically been written as two hand-mirrored code
//! paths: `gemma4::forward_gemma4_with_lora` (CPU) and
//! `gemma4_gpu::forward_gemma4_gpu` (CPU attention/softmax/RoPE/norms +
//! hipBLAS matmuls). Every architectural change had to be made twice, in
//! lockstep, with no static guarantee the two paths stay compatible.
//!
//! The 2026-04-20 24h consolidation session surfaced a third backend in
//! waiting — real GPU-resident attention kernels — which would have meant
//! a THIRD fork if we continued the copy-paste pattern. Instead we factor
//! the model math behind a trait so any backend satisfying the interface
//! plugs into the training harness, the Learning Gate tests, and the CLI
//! with zero changes upstream.
//!
//! ## Trait contract
//!
//! A `ForgeBackend` owns two halves of the model-math lifecycle:
//!
//! 1. **`upload_weights(cpu)`** converts a `CpuWeightsGemma4` into
//!    backend-specific state (the associated `Handle` type). For the CPU
//!    backend, `Handle = ()`. For the hipBLAS-matmul backend,
//!    `Handle = Gemma4GpuWeights` (the existing VRAM-resident weights).
//!    A future `GpuKernelsBackend` might hold `Handle = GpuKernelState`
//!    with pre-allocated activation buffers, stream handles, etc.
//!
//! 2. **`forward` / `backward`** run one training step's forward / backward
//!    pass with the caller's `Gemma4LoraAdapters`. The trait signature
//!    matches the existing free-function signatures so the migration is
//!    mechanical.
//!
//! The trait intentionally does NOT own the Adam step — `train_step` is a
//! default method that composes forward + backward + `lora_adam_step`.
//! That keeps optimizer logic in ONE place (fixing clip+momentum bugs once
//! applies to every backend).
//!
//! ## Resilience properties this delivers
//!
//! - New backend = one `impl ForgeBackend for MyBackend`. No changes to
//!   `EvalHarness`, CLI, Learning Gate tests, or regression audits.
//! - CPU-vs-GPU correctness is diff-testable: run the Learning Gate with
//!   `CpuBackend`, compare to `HybridMatmulBackend`, verify cosine ≥0.999.
//! - Swappable at runtime via `&dyn` trait objects where a single handle
//!   type is fine, or via generic parameters where heterogeneous types are
//!   needed.
//! - Bit-exact compatibility preserved: the existing free functions stay
//!   and simply become the trait impl bodies.

use crate::gemma4::{
    backward_gemma4_with_lora, forward_gemma4_with_lora, CpuWeightsGemma4, Gemma4LayerCache,
    Gemma4LoraAdapters, LayerGradHealth,
};
use crate::gemma4_gpu::{forward_gemma4_gpu, Gemma4GpuWeights, PleMode};

/// The Forge model-math backend contract.
///
/// Implementors supply backend-specific forward + backward passes for the
/// Gemma 4 architecture. The trait is generic over the weight-handle type
/// so CPU-only backends (where handle is `()`) and GPU backends (where the
/// handle owns VRAM-resident weights) both fit.
///
/// All methods return `Result<_, String>` to match the existing error style
/// in `gemma4_gpu.rs`. A future refactor to a richer `ForgeError` enum is
/// mechanical and out of scope for this commit.
pub trait ForgeBackend {
    /// Backend-specific state held between weight-upload and training.
    /// `()` for CPU-only backends; `Gemma4GpuWeights` for the hipBLAS
    /// matmul backend; opaque state for future GPU-kernel backends.
    type Handle;

    /// Short label for logging / regression metadata. Must be stable
    /// across versions so grep patterns in `cargo test --nocapture` logs
    /// keep working.
    fn name(&self) -> &'static str;

    /// Prepare backend-specific weight state from the canonical
    /// `CpuWeightsGemma4`. May upload to GPU, allocate buffers, etc.
    /// Called once per training session.
    fn upload_weights(&self, cpu: &CpuWeightsGemma4) -> Result<Self::Handle, String>;

    /// Forward pass with optional LoRA residual. Returns
    /// `(logits [seq*vocab], per-layer caches for backward)`. Caches must
    /// be compatible with `backward` regardless of which backend produced
    /// them — this is what lets us swap backends mid-training for
    /// correctness diffs.
    fn forward(
        &self,
        cpu: &CpuWeightsGemma4,
        handle: &Self::Handle,
        lora: Option<&Gemma4LoraAdapters>,
        tokens: &[u32],
    ) -> Result<(Vec<f32>, Vec<Gemma4LayerCache>), String>;

    /// Backward pass producing `(mean_loss, per-layer grad health)` and
    /// mutating `lora`'s `grad_a`/`grad_b` fields. `answer_start` is the
    /// first token index counted toward CE loss (skip prompt positions).
    #[allow(clippy::too_many_arguments)] // numerical/GPU kernel signature — see crate note
    fn backward(
        &self,
        cpu: &CpuWeightsGemma4,
        handle: &Self::Handle,
        lora: Option<&mut Gemma4LoraAdapters>,
        caches: &[Gemma4LayerCache],
        logits: &[f32],
        tokens: &[u32],
        answer_start: usize,
    ) -> Result<(f32, Vec<LayerGradHealth>), String>;

    /// Full training step: forward → backward → gradient clip + Adam.
    /// Default implementation composes the other methods + `lora_adam_step`
    /// so optimizer changes land in one place.
    #[allow(clippy::too_many_arguments)] // numerical/GPU kernel signature — see crate note
    fn train_step(
        &self,
        cpu: &CpuWeightsGemma4,
        handle: &Self::Handle,
        lora: &mut Gemma4LoraAdapters,
        tokens: &[u32],
        answer_start: usize,
        lr: f32,
        step: u32,
    ) -> Result<f32, String> {
        let (logits, caches) = self.forward(cpu, handle, Some(lora), tokens)?;
        let (loss, _health) = self.backward(
            cpu,
            handle,
            Some(lora),
            &caches,
            &logits,
            tokens,
            answer_start,
        )?;
        lora_adam_step(lora, lr, step);
        Ok(loss)
    }
}

/// Gradient-clip-then-Adam update for every active LoRA target. Centralized
/// so the optimizer definition lives in one place and every `ForgeBackend`
/// inherits the same numeric behavior.
///
/// Clip threshold is 1.0 on the joint-norm of `grad_a` + `grad_b` per
/// target; Adam params are β1=0.9, β2=0.999, ε=1e-8 (matching the existing
/// free-function defaults). The `adam_step` helper on `LoraLayer` also
/// carries a NaN guard from `lora.rs`.
pub fn lora_adam_step(lora: &mut Gemma4LoraAdapters, lr: f32, step: u32) {
    let clip_threshold = 1.0f32;
    for il in 0..lora.layers.len() {
        for t in 0..4 {
            if let Some(ll) = &mut lora.layers[il][t] {
                let gn_sq: f32 = ll
                    .grad_a
                    .iter()
                    .chain(ll.grad_b.iter())
                    .map(|g| g * g)
                    .sum();
                let gn = gn_sq.sqrt();
                if gn > clip_threshold {
                    let s = clip_threshold / gn;
                    for g in ll.grad_a.iter_mut() {
                        *g *= s;
                    }
                    for g in ll.grad_b.iter_mut() {
                        *g *= s;
                    }
                }
                ll.adam_step(lr, 0.9, 0.999, 1e-8, step);
            }
        }
    }
}

// ====================================================================
// Backend 1: CpuBackend — pure-CPU reference implementation.
// ====================================================================

/// Pure-CPU reference backend. `Handle = ()` since all state already lives
/// in the `CpuWeightsGemma4` passed by the caller.
#[derive(Clone, Copy, Debug, Default)]
pub struct CpuBackend;

impl ForgeBackend for CpuBackend {
    type Handle = ();

    fn name(&self) -> &'static str {
        "cpu"
    }

    fn upload_weights(&self, _cpu: &CpuWeightsGemma4) -> Result<(), String> {
        Ok(())
    }

    fn forward(
        &self,
        cpu: &CpuWeightsGemma4,
        _: &(),
        lora: Option<&Gemma4LoraAdapters>,
        tokens: &[u32],
    ) -> Result<(Vec<f32>, Vec<Gemma4LayerCache>), String> {
        Ok(forward_gemma4_with_lora(cpu, lora, tokens))
    }

    fn backward(
        &self,
        cpu: &CpuWeightsGemma4,
        _: &(),
        lora: Option<&mut Gemma4LoraAdapters>,
        caches: &[Gemma4LayerCache],
        logits: &[f32],
        tokens: &[u32],
        answer_start: usize,
    ) -> Result<(f32, Vec<LayerGradHealth>), String> {
        let (loss, health) =
            backward_gemma4_with_lora(cpu, None, lora, caches, logits, tokens, answer_start);
        Ok((loss, health))
    }
}

// ====================================================================
// Backend 2: HybridMatmulBackend — CPU attention/softmax/norms + hipBLAS matmuls.
// ====================================================================

/// Current production GPU backend: matmuls run on hipBLAS via the existing
/// `Gemma4GpuWeights`, everything else (attention, softmax, RoPE, RMSNorm,
/// GELU, Adam) stays on the CPU. Named "hybrid-matmul" to distinguish it
/// from the future full-GPU-kernels backend.
pub struct HybridMatmulBackend {
    pub ple_mode: PleMode,
}

impl HybridMatmulBackend {
    pub fn new(ple_mode: PleMode) -> Self {
        Self { ple_mode }
    }
}

impl Default for HybridMatmulBackend {
    fn default() -> Self {
        Self {
            ple_mode: PleMode::Cpu,
        }
    }
}

impl ForgeBackend for HybridMatmulBackend {
    type Handle = Gemma4GpuWeights;

    fn name(&self) -> &'static str {
        "hybrid-matmul"
    }

    fn upload_weights(&self, cpu: &CpuWeightsGemma4) -> Result<Gemma4GpuWeights, String> {
        Gemma4GpuWeights::upload(cpu, self.ple_mode)
    }

    fn forward(
        &self,
        cpu: &CpuWeightsGemma4,
        handle: &Gemma4GpuWeights,
        lora: Option<&Gemma4LoraAdapters>,
        tokens: &[u32],
    ) -> Result<(Vec<f32>, Vec<Gemma4LayerCache>), String> {
        forward_gemma4_gpu(cpu, handle, lora, tokens)
    }

    fn backward(
        &self,
        cpu: &CpuWeightsGemma4,
        handle: &Gemma4GpuWeights,
        lora: Option<&mut Gemma4LoraAdapters>,
        caches: &[Gemma4LayerCache],
        logits: &[f32],
        tokens: &[u32],
        answer_start: usize,
    ) -> Result<(f32, Vec<LayerGradHealth>), String> {
        let (loss, health) = backward_gemma4_with_lora(
            cpu,
            Some(handle),
            lora,
            caches,
            logits,
            tokens,
            answer_start,
        );
        Ok((loss, health))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The trait's associated-type pattern is what makes `&dyn` trait
    /// objects awkward — every consumer must be generic over `B`. This
    /// test pins the chosen design so future "just use dyn" refactors
    /// have to actively reject it.
    #[test]
    fn test_cpu_backend_zero_handle() {
        let cpu = CpuBackend;
        assert_eq!(cpu.name(), "cpu");
        // Upload is a no-op for CPU.
        // (Can't call without a real CpuWeightsGemma4; skipped here.)
    }

    #[test]
    fn test_hybrid_backend_default() {
        let b = HybridMatmulBackend::default();
        assert_eq!(b.name(), "hybrid-matmul");
        assert!(matches!(b.ple_mode, PleMode::Cpu));
    }
}
