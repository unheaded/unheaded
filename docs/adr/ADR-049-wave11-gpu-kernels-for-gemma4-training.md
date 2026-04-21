# ADR-049 — WAVE11: Custom HIP Kernels for Gemma-4 Training at seq=384

**Status:** Accepted
**Date:** 2026-04-21
**Deciders:** Stevie Bellis + unheaded-warmonger + unheaded-marshal + unheaded-scientist
**Context owner:** zhenai-forge (training)

---

## Context

The 24h WAVE10F consolidation block (2026-04-20) hit a hard wall on
Phase 5 — the Kingdom RAFT smoke at seq=384 produced **zero training
output in 64 minutes** and was aborted. Profiling traced the stall to
CPU-bound scaled-dot-product attention: forge's `HybridMatmulBackend`
dispatched matmuls to hipBLAS but kept softmax, RoPE, RMSNorm, GELU,
and the O(seq²·d) QKV attention loop single-threaded on CPU. At
seq=384 with 35 layers, that attention loop alone projected to ~20 s
per step; backward added another ~20 s; full step ≫ 60 s — before
counting everything else.

The two acceptable outcomes for the Kingdom RAFT were:

1. Move attention (and friends) onto the GPU.
2. Truncate sequences aggressively (seq ≤ 64), losing long-context
   training for Kingdom corpus.

(2) defeated the point of Kingdom training. (1) required writing HIP
kernels — without adding crate-level dependencies (ADR-004), without
dlopen, and without breaking ForgeBackend (ADR-048).

## Decision

Land custom HIP kernels for every per-layer op except the bf16 matmuls
that already hit hipBLAS. Integrate them incrementally into the
existing `forward_gemma4_gpu` / `backward_gemma4_with_lora` paths
without a new backend struct, keeping the ForgeBackend contract intact.

**Kernels shipped (21 unit tests at cosine=1.000 on synthetic inputs):**

| Op | Fwd | Bwd | Notes |
|----|----:|----:|-------|
| RMSNorm | ✅ | ✅ | `rmsnorm_fwd_f32`, `rmsnorm_bwd_f32` |
| GELU (pytorch_tanh) | ✅ | ✅ | 4 variants incl. fused `gelu·up` |
| Softmax (masked) | ✅ | ✅ | additive mask for causal / sliding |
| RoPE (partial rotary) | ✅ | ✅ | passthrough for dims ≥ rope_dim |
| Attention scores | ✅ | (via grad_q/k) | GQA-native `h_kv = h/(n_h/n_kv)` |
| Attention output | ✅ | (via grad_v) | GQA-native |
| Attention backward | ✅ | ✅ | grad_v, grad_probs, grad_q, grad_k |

**Build system:** `build.rs` discovers `kernels/*.hip.cpp`, compiles
with `hipcc --offload-arch=gfx1101 --shared -fPIC -O3` into
`libwave11_kernels.so` in `$OUT_DIR`; cargo statically links.
Plain `extern "C"` FFI — no libloading, no new crate deps.

**Integration (Phase 8a + 8b):**

- `gemma4_gpu::attention_forward_gpu_kernels` (Phase 8a, commit
  `6d620c9e`): drop-in replacement for `forward::attention_forward`,
  composes `attn_scores_fwd` → `softmax_fwd_masked` → `attn_output_fwd`.
  Called inside `forward_gemma4_gpu` per layer.
- `gemma4_gpu::attention_backward_gpu_kernels` (Phase 8b, commit
  `5ebceb5a`): GQA-native replacement for `backward::attention_backward`.
  Kernels collapse group heads inside `attn_grad_v` / `attn_grad_k`, so
  caller skips `gqa_expand` / `gqa_collapse`. Called inside
  `backward_gemma4_with_lora` when `gpu.is_some()`.

Buffers are sized from actual slice lengths (not derived shape
formulas) to survive Gemma-4 E2B's KV-reuse pattern, where some
consumer layers inherit K/V from producers at a different head_dim.

**Deferred:** swapping the per-token CPU ops (rmsnorm, rope,
gelu, per-head rmsnorm) in `forward_gemma4_gpu` onto their kernels
(Phase 8c), and a full `impl ForgeBackend for GpuKernelsBackend`
with GPU-resident activations (Phase 8d). Both are follow-ons;
ADR-048 keeps the door open.

## Why not dlopen / dynamic kernels?

Considered and rejected in Phase 2 amendment. Runtime `libloading` +
`dlopen` of `libwave11_kernels.so` would have added `libloading` as a
crate dep (violates ADR-004) and complicated error handling. Static
linking of `.so` via cargo is simpler, faster-to-load, and keeps the
error surface exactly like existing `hip.rs` / `hipblas` FFI.

## Why not a separate crate?

Considered and rejected. The kernels are tightly coupled to forge's
memory layouts (`[seq, n_heads, head_dim]`, `[n_heads, seq, seq]`,
etc.) and to the `hip::GpuBuffer` type. A separate crate would need
to either duplicate those types or take generic buffers — both worse.
Keeping kernels inside `crates/zhenai-forge/kernels/` and the Rust
bindings under `src/hip_kernels/` mirrors the existing `hip.rs`
organization.

## Results

**seq=384 Kingdom RAFT smoke (3 steps, lr=1e-3):**

| Config | Cold step | Warm step | Status |
|--------|----------:|----------:|-------|
| 24h session (CPU attention) | ∞ | ∞ | Stalled 64+ min, aborted |
| Phase 8a (GPU fwd, CPU bwd attn) | 227.5s | 154s | Descends, 30× over target |
| **Phase 8b (GPU fwd + bwd attn)** | **69.0s** | **~11s** | **Descends, 2.2× over** |

Loss trajectory identical between Phase 8a and Phase 8b
(21.24 → 19.6 → 16.8), confirming numerical correctness of the
GPU backward path on the real Gemma-4 E2B GGUF.

**Latent kernel bug caught during integration:** four attention
kernels and both RoPE kernels had a one-thread-per-d pattern that
silently under-wrote when `head_dim > blockDim.x` (clamped at 256).
Gemma-4 E2B's single global-attention layer runs at `head_dim_full
= 512`, so only the first 256 dims of its attention output were
written; the rest stayed at allocation garbage, wrecking final
logits (cos=-0.11 vs CPU). Fixed via strided
`for (d = tid; d < head_dim; d += blockDim.x)` in attn_output_fwd,
attn_grad_v/q/k, attn_grad_probs, and rope_partial_fwd/bwd. Unit
tests at head_dim=256 missed this; the Gemma-4
`test_gemma4_gpu_forward_matches_cpu` regression caught it.

## Files

- `crates/zhenai-forge/kernels/common.hip.hpp` — bf16 helpers,
  block_reduce_{sum,max}
- `crates/zhenai-forge/kernels/rmsnorm.hip.cpp`
- `crates/zhenai-forge/kernels/gelu.hip.cpp`
- `crates/zhenai-forge/kernels/softmax.hip.cpp`
- `crates/zhenai-forge/kernels/rope.hip.cpp`
- `crates/zhenai-forge/kernels/attn.hip.cpp`
- `crates/zhenai-forge/kernels/identity.hip.cpp` (smoke)
- `crates/zhenai-forge/src/hip_kernels/` (Rust FFI bindings + tests)
- `crates/zhenai-forge/build.rs` (hipcc compilation + linking)
- `crates/zhenai-forge/src/gemma4_gpu.rs::attention_forward_gpu_kernels`
- `crates/zhenai-forge/src/gemma4_gpu.rs::attention_backward_gpu_kernels`
- `crates/zhenai-forge/src/gemma4.rs::backward_gemma4_with_lora` (swap site)
- `crates/zhenai-forge/notes/wave11-gpu-kernels-battle-plan.md`
- `crates/zhenai-forge/notes/wave11-session-log.md`

## Consequences

- **Unblocked.** Kingdom RAFT at seq=384 now runs; 500-step LoRA
  projects to ~92 min (vs never-completing pre-W11).
- **Follow-on scope.** Phase 8c (per-token CPU ops on GPU) and
  Phase 8d (GpuKernelsBackend struct with GPU-resident activations)
  are tracked for a future session; current integration meets the
  "can train at seq=384" bar and keeps the architecture evolvable.
- **New maintenance surface.** Kernels are handwritten HIP and need
  re-benchmarking on any GPU arch change; gfx1101 (Navi 32) is the
  baseline target for this ADR.
- **ADR-004 untouched.** No new crate deps. No libloading, no
  bytemuck, no hipcc bindings — just `extern "C"` FFI to a linked `.so`.
- **ADR-048 extended cleanly.** The existing ForgeBackend abstraction
  absorbs the GPU-kernel work without new trait methods.

## Supersedes / relates to

- Supersedes the "Phase 5 blocked on CPU attention" exit state of
  `notes/wave10f-24h-battle-plan.md`.
- Relates to ADR-048 (ForgeBackend trait).
- Relates to ADR-045 (Wave 10D GPU backward — established the hipBLAS
  bf16_ex matmul path that W11 builds on).
