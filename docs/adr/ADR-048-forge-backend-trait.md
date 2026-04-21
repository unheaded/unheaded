# ADR-048 — ForgeBackend Trait: Pluggable Kernel Provider for zhenai-forge

**Status:** Accepted
**Date:** 2026-04-21
**Deciders:** Stevie Bellis + unheaded-scientist + unheaded-marshal
**Context owner:** zhenai-forge (training)

---

## Context

`zhenai-forge` has two parallel implementations of the Gemma 4 forward
and backward passes:

- `gemma4::forward_gemma4_with_lora` — pure CPU reference.
- `gemma4_gpu::forward_gemma4_gpu` — CPU attention / softmax / RoPE /
  RMSNorm / GELU plus hipBLAS matmuls (the "hybrid matmul" backend).

Every architectural change has had to be made in both, in lockstep,
with no static guarantee they stay compatible. When the 24h session
of 2026-04-20 identified CPU-bound attention as the Phase 5 blocker,
the proposed fix — a GPU-kernels backend for attention/softmax/norms
— would have created a THIRD fork of the same math.

The pattern generalizes. A hypothetical `LlamaCppBackend` (borrow a
production engine for demos), a future `TritonBackend` (kernels via
MLIR/Triton), or a `MockBackend` (for deterministic property tests)
would each have become another fork under the copy-paste pattern.

## Decision

Introduce a `ForgeBackend` trait in `crates/zhenai-forge/src/backend.rs`:

```rust
pub trait ForgeBackend {
    type Handle;
    fn name(&self) -> &'static str;
    fn upload_weights(&self, cpu: &CpuWeightsGemma4) -> Result<Self::Handle, String>;
    fn forward(&self, cpu, handle, lora, tokens)
        -> Result<(Vec<f32>, Vec<Gemma4LayerCache>), String>;
    fn backward(&self, cpu, handle, lora, caches, logits, tokens, answer_start)
        -> Result<(f32, Vec<LayerGradHealth>), String>;
    fn train_step(&self, ...) -> Result<f32, String>;  // default impl
}
```

The crate ships two impls out of the gate:

- `CpuBackend { }` — `Handle = ()`. Delegates to
  `forward_gemma4_with_lora` + `backward_gemma4_with_lora(gpu=None)`.
- `HybridMatmulBackend { ple_mode }` — `Handle = Gemma4GpuWeights`.
  Delegates to `forward_gemma4_gpu` + `backward_gemma4_with_lora(gpu=Some)`.

All training-harness APIs (`EvalHarness::run`, `::run_until_plateau`,
`::compute_eval_loss`, `::compute_eval_top1`, `::forward_loss*`,
standalone `compute_mean_loss_over`) get a backend-parameterized
`_with_backend<B>` form. Existing `Option<&Gemma4GpuWeights>`-based
signatures become thin shims that pick `CpuBackend` when `None` and
`HybridMatmulBackend::default()` when `Some`, so the Learning Gate
tests and the Kingdom RAFT smoke test continue to work unchanged.

`main.rs::cmd_train_gemma4` collapses from `match &gpu_weights {
Some => train_step_gemma4_gpu, None => train_step_gemma4 }` to a
generic `run_loop<B: ForgeBackend>(...)` called once per branch.

## Why this form (rejected alternatives)

1. **Enum of backends (`enum BackendHandle { Cpu, Gpu(Gemma4GpuWeights) }`).**
   Simpler today but closes the door on opaque handles (future
   `GpuKernelsBackend { state: OpaqueKernelState }`, `LlamaCppBackend
   { ctx: LlamaCppCtx }`) without cramming everything into a central
   enum. Associated-type `Handle` is zero-cost and open-ended.

2. **`Box<dyn ForgeBackend<Handle = ...>>`.** Associated types make
   object-safe dyn dispatch awkward. We'd need either a second
   type-erased trait or a hand-rolled vtable. For forge's small
   finite set of backends, generic-parameterization is ergonomic
   enough and keeps the runtime cost at zero.

3. **Keep the two code paths, add GPU kernels as a third.** The
   status quo. Rejected because every refactor already costs 2× the
   work; making it 3× would have made every future optimization a
   three-way copy-paste exercise.

4. **Switch to PyTorch/llama.cpp wholesale.** Rejected. The stated
   Unheaded direction (`project_gemma4_decision.md`, ADR-030,
   `project_plumbing_strategy.md`) is Rust-native with pluggable
   backends — *many faucets, one pipe*. A `LlamaCppBackend` is
   a future addition UNDER the trait, not a replacement for it.

## Consequences

### Positive

- **New backend = one `impl` block.** No changes to EvalHarness, CLI,
  Learning Gate tests, or regression audits.
- **Correctness diffs are cheap.** Run the Learning Gate first with
  `CpuBackend` (reference), then with any new backend, compare
  cosine. Catches kernel bugs before they hit production.
- **Optimizer lives in ONE place.** `train_step` default impl calls
  `lora_adam_step` — a single function — so gradient-clip or Adam
  hyperparameter changes land once and propagate to every backend.
- **Bit-exact compatibility preserved for the shipped refactor.**
  Regression on the six Learning Gate tests (grad_health,
  loss_descent_gpu, exp1, exp3, exp4, exp5) all GREEN with BIT-
  IDENTICAL metrics after the refactor (commits `cca45996` through
  `219f9da2`).

### Negative

- **Generic-parameter consumers.** Every new training helper must be
  either `fn foo<B: ForgeBackend>(...)` or a shim over two concrete
  backends. Marginal ergonomic cost; no runtime cost.
- **Associated-type can't be erased to `dyn`.** If we later want a
  `Vec<Box<dyn ForgeBackend>>` for side-by-side comparison, we'd
  need a second type-erased trait (straightforward to add).

### Neutral

- Shims preserve the old `Option<&Gemma4GpuWeights>` signatures as
  long as useful. They can be deprecated and removed when the last
  caller migrates (likely after Phase 7.2 real-data RAFT lands).

## Implementation landmarks

Landed across 5 commits during the 2026-04-21 session:

| Commit | Step | Scope |
|--------|------|-------|
| `cca45996` | R1 | `backend.rs` with trait + `CpuBackend` + `HybridMatmulBackend` + 2 unit tests. |
| `2a5c0a55` | R2+R3 | Backend-parameterized EvalHarness methods; legacy `Option<&Gemma4GpuWeights>` signatures become shims. |
| `219f9da2` | R4 | `main.rs::cmd_train_gemma4` uses the trait via generic `run_loop<B>`. |
| `<next>` | R5 | GPU regression: grad_health, loss_descent, exp3 all bit-identical. |
| `<next>` | R6 | This ADR + CLAUDE.md + memory updates. |

## Follow-ups (out of scope for this ADR)

- Write a `GpuKernelsBackend` that moves attention/softmax/RoPE/norms
  to GPU kernels (WAVE11). This unblocks Phase 5 real-data RAFT.
- Add a `MockBackend` for property-based tests over the trait
  contract (e.g., "forward(tokens).shape == [seq*vocab]", "backward
  grad norms are finite").
- Consider a `LlamaCppBackend` as a ground-truth oracle for
  correctness diffs.
- Evaluate whether `train_step` default impl is the right place for
  the gradient clipper (could factor into its own `LoraOptimizer`
  trait if we ever want SGD / AdamW / Muon variants).

## References

- `crates/zhenai-forge/src/backend.rs` — trait definition + impls.
- `crates/zhenai-forge/notes/wave10f-24h-session-2026-04-20.md` —
  the session that surfaced the three-forks risk.
- `crates/zhenai-forge/notes/wave10f-step-c-battle-plan.md` — the
  14-site migration that demonstrated how expensive the copy-paste
  pattern is.
- ADR-030 (Rust training direction), ADR-045 (WAVE10D GPU backward).
