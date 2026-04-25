# ADR-050 — WAVE12: GPU-Resident Forge Activations + Kingdom RAFT LoRA

**Status:** Accepted
**Date:** 2026-04-23
**Deciders:** Stevie Bellis + unheaded-warmonger + unheaded-marshal + unheaded-scientist + unheaded-developer
**Context owner:** zhenai-forge (training)

---

## Context

WAVE11 (ADR-049) shipped custom HIP kernels for attention/rmsnorm/gelu/
rope and integrated them into forge's Gemma-4 training. End-state at
seq=384 on Kingdom corpus: **warm 10.5 s/step**, all 21 kernel unit
tests at cosine=1.000, 11s baseline running real RAFT smokes through.
WAVE10F's earlier 2.05 s/step number was on a 6-token toy — not
comparable to seq=384 quadratic-attention reality.

WAVE12 was forged on 2026-04-22 with the goal of cutting the warm step
to ≤5 s/step and shipping a 500-step Kingdom RAFT LoRA with held-out
eval descent. The plan attributed the 10.5 s warm step to per-op PCIe
round-trips (alloc + upload + kernel + download per kernel call across
~350 launches/step) and proposed a GPU-resident activation chain via a
`ForwardScratch` struct — projected savings ~3-5 s/step.

## What WAVE12 Did

The plan executed in eight phases (battle plan at
`crates/zhenai-forge/notes/wave12-gpu-resident-battle-plan.md`):

| Phase | Shipped | Wall-time delta |
|------:|---------|-----------------|
| 0 | Preflight + baseline confirmation | 0 (baseline 10.2 s) |
| 1 | `MaskCache` — 1× upload per (variant, seq) instead of per-layer | ~0 (jitter floor) |
| 2 | 11 of 13 backward call sites moved to GPU batch helpers (rmsnorm/gelu/rope/per_head_rmsnorm) | -0.6 s nominal (jitter) |
| 3 | Profiling instrumentation, **falsified the matmul-compute hypothesis** | informational |
| 4 | `matmul_xwt_gpu_in_out` / `matmul_grad_x_gpu_in_out` + `f32_to_bf16` HIP kernel + `add_f32` HIP kernel | infrastructure |
| 5A-D | `ForwardScratch` struct; pre-attn (rmsnorm→Q/K/V) and O→post_attn→FFN→post_ffw chains made GPU-resident | -0.4 s on matmul, +0.4 s on cache downloads → net 0 |
| 7 | 500-step Kingdom RAFT LoRA at seq=384 + held-out eval descent | _TBD on completion_ |
| 8 | This ADR + memory + handoff | docs |

## Phase 3: The Profiling Surprise

`WAVE12_PROFILE=1` instruments `matmul_xwt` / `matmul_grad_x` with
atomic ns counters around (a) the pure `sgemm_bf16_ex` call and
(b) the full method (bf16 conv + alloc + upload + sgemm + download).
Warm-step measurement on seq=384 Kingdom data:

| measurement | warm value | share of 10.5 s step |
|-------------|-----------:|--------------------:|
| pure sgemm compute | 0.02 s | **0.2 %** |
| matmul_method (bf16 + I/O) | 1.55 s | 15 % |
| matmul round-trip overhead | 1.55 s | 15 % |
| **non-matmul (attention, rmsnorm/gelu/rope helpers, residuals, glue)** | **~8.4 s** | **~84 %** |

This **falsified** the implicit hypothesis that bf16 matmul compute
dominates step time — RX 7700 XT's hipBLAS chews through 718
matmul/step in 20 ms total. The actual overhead is per-call
alloc/upload/download/sync across all kernels, dominated by non-matmul
helpers. The plan's cost model was directionally right on round-trips
but assigned them to the wrong kernels.

## Phase 5: The 1:1 Tradeoff

Phase 5's `ForwardScratch` reduces matmul method overhead from 1.55 s
to 1.17 s (-0.38 s as predicted) by chaining rmsnorm→f32_to_bf16→
matmul_xwt_gpu_in_out without host bf16 conversion or per-call
alloc/upload. **But** the backward pass still consumes CPU `Vec<f32>`
layer cache (`ffn_normed`, `gate_pre`, `up_pre`, `ffn_hidden`,
`post_attn_residual`, `post_ffw_residual`, `q_rot`, `k_rot`, `v`,
`attn_cache`, `attn_out`), forcing per-layer downloads of those values
from scratch buffers. 6 extra downloads × 35 layers ≈ 0.4 s.

**Net Phase 5 wall-time: 0 s.** Forward is now genuinely
GPU-resident; the savings are locked behind a future WAVE13 backward
rewrite.

## Decision

Accept that WAVE12's warm-step target was a manufactured number (not
business-driven) and ship the Kingdom RAFT LoRA at the unchanged
~10 s/step. The actual sprint goal is the LoRA + held-out eval
descent — the warm-step target was wishful extrapolation introduced
when this plan was authored. Time-pressuring a backward rewrite to
hit a self-imposed gate produces 5-7 h of risky refactor for
uncertain payoff.

**The artifacts of WAVE12 that DO matter:**

1. **`MaskCache`** (~165 MB/step PCIe saved, no measurable wall-time
   on this hardware but architecturally correct).
2. **`matmul_xwt_gpu_in_out` + `matmul_grad_x_gpu_in_out`** —
   pre-allocated GPU input/output variants. Used by the forward chain;
   future backward rewrite needs them.
3. **`f32_to_bf16` HIP kernel** — RNE-rounded, bit-identical to
   `half::bf16::from_f32` over 10k deterministic values. Eliminates
   host bf16 conversions in the chain.
4. **`add_f32` HIP kernel** — residual stream stays GPU-resident.
5. **`ForwardScratch` struct** — pre-allocated activation buffers
   sized over max layer dims. ~70 MB at seq=512.
6. **GPU-resident forward layer body** — pre-attn chain, O/post-attn
   chain, FFN chain, post-ffw residual all flow through scratch
   buffers without round-tripping the matmul or norm outputs. Cached
   layer fields are downloaded once at the end of each layer where
   backward needs them.
7. **Profiling instrumentation** — `WAVE12_PROFILE=1` toggle, runs
   at near-zero cost when off. Useful for any future kernel-by-kernel
   triage.
8. **`eval-gemma4` CLI subcommand** — replaces the stub `eval` cmd
   for Gemma-4 LoRAs. Loads LoRA, runs forward over eval set, prints
   mean CE loss with/without LoRA, formal Phase-7-exit-gate verdict.

These are the foundations a WAVE13 "GPU-resident backward" would
build on.

## Consequences

**Positive:**

- Forward chain is now structurally GPU-resident; future work plugs
  in cleanly.
- `f32_to_bf16` and `add_f32` HIP kernels are general-purpose,
  unit-tested, bit-identical to host references.
- `WAVE12_PROFILE` lets us re-run discriminating experiments without
  custom test harnesses.
- The plan's cost model is empirically corrected — backward
  round-trips, not matmul compute, are the next target.
- The 5.5 s/step target is officially retired as wishful (no business
  justification was ever attached).

**Neutral:**

- Wall-time at seq=384 is unchanged from WAVE11 baseline (~10 s/step).
  Kingdom RAFT runs in ~90 min, fine for overnight training.
- The `eval-gemma4` CLI is the first non-stub eval command in forge.

**Negative:**

- Some forward-side complexity added without immediate wall-time
  payoff (the 6× cache-download per layer is now explicit code).
- The `_with_lora` LoRA O download/re-upload pattern in Stage 5C is
  ugly but contained (only triggers when LoRA is Some, which is
  always for training).
- Layer-cache shape coupling between forward-resident path and
  CPU-side backward is fragile — any backward refactor will need to
  re-evaluate the cache invariants.

## Future Work

- **WAVE13 — GPU-resident backward.** `BackwardScratch` mirroring
  `ForwardScratch`. Replace the per-call `*_batch_bwd_on_gpu` helpers
  with direct kernel calls on shared scratch buffers. Eliminate the
  forward-side cache downloads by retaining the GPU values for
  backward consumption. Expected savings: 3-5 s/step.
- **`gemma4_gpu::matmul_xwt_f32_input_gpu_in_out`** — variant that
  takes f32 input on GPU, internally converts to bf16, no host
  round-trip. Plan Step 100; deferred since the explicit
  `f32_to_bf16` + `matmul_xwt_gpu_in_out` chain in 5B-D already
  does this.
- **Hybrid CPU/GPU LoRA application.** Currently the LoRA O step
  downloads o_out, modifies on CPU, re-uploads. A `lora_forward_gpu`
  helper would keep the chain unbroken.

## References

- Battle plan: `crates/zhenai-forge/notes/wave12-gpu-resident-battle-plan.md`
- Session log: `crates/zhenai-forge/notes/wave12-session-log.md`
- Profile data: log entries in session-log under "Phase 3 Profiling experiment"
- ADR-049 — WAVE11 custom HIP kernels (predecessor)
- ADR-048 — ForgeBackend trait (uphold; no new backend added)
- ADR-004 — Crate dependency policy (uphold; no new dependencies added)
