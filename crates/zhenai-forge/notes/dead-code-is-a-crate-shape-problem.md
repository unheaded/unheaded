# zhenai-forge: the `dead_code` warnings are a crate-shape problem

**Date:** 2026-07-31, resolved 2026-08-03
**Context:** working the tree-wide clippy ratchet down (348 → 32)
**Status:** FIXED — the lib/bin split described below was carried out on
2026-08-03 with Stevie's approval. `crates/zhenai-forge` now reports **0**
clippy warnings, down from 96. This note is kept as the reasoning record; the
sections below are written in the pre-fix present tense.

## The number

`crates/zhenai-forge` accounts for 96 of the tree's remaining clippy warnings,
and **84 of those 96 are `dead_code`** — "function/constant/method/field/struct
is never used", spread across every module:

| file | dead items | file | dead items |
|---|---|---|---|
| `eval.rs` | 22 | `data.rs` | 5 |
| `train.rs` | 8 | `backward.rs` | 5 |
| `hip.rs` | 8 | `forward.rs` | 3 |
| `gemma4_gpu.rs` | 7 | `hip_kernels/*` | 6 |
| `gemma4.rs` | 7 | others | 9 |
| `eval_stats.rs` | 6 | | |

## Why they are (mostly) not real

zhenai-forge is a **binary crate**. There is no `lib.rs`, so `pub` grants no
exemption from dead-code analysis — an item is "used" only if the `zhenai-forge`
binary reaches it. That mislabels three legitimate categories:

1. **Test-exercised research paths.** `eval.rs`'s Learning Gate experiment
   runners (`run_with_backend`, `run_until_plateau*`) are driven by the test
   suite, not the CLI — `main.rs` constructs a *thin* `EvalHarness` directly
   and calls a subset. The experiments are the deliverable; running them via
   `cargo test` is the design.

2. **Cascading false positives.** `eval_stats::bootstrap_ci_95` is flagged as
   never used, but it is called at `eval.rs:344` and `eval.rs:407`. It is only
   "dead" because *its caller* is dead. Several of the 84 are this shape, so
   the number overstates the real surface considerably.

3. **Deliberate extension surface.** `hip_kernels/*` are `extern "C"` FFI
   declarations mirroring the `.hip.cpp` kernels — the block is the ABI, whether
   or not today's code path calls every entry. `backend.rs` is ADR-048's
   pluggable `ForgeBackend`, explicitly designed so new backends drop in.

## Why I did not "fix" it

**Not by deleting.** These are tested, documented, ADR-backed. CLAUDE.md also
records that the Mistral path in `train.rs` is retained on purpose ("side
stuff" rule) — deleting it would reverse a decision already made.

**Not by `#![allow(dead_code)]` at crate root.** It would clear ~84 warnings in
one line, and that is exactly the problem: it also permanently blinds the crate
to *future* dead code. That is the same shape as the `continue-on-error: true`
that hid the structurally-broken clippy job for months — green because it can
no longer fail. Trading a real signal for a smaller number is not an
improvement.

## What the actual fix is

**Split into `src/lib.rs` + a thin `src/main.rs`.** Then `pub` means "API",
dead-code analysis becomes meaningful again, and the false positives resolve
*legitimately* rather than being suppressed.

Feasibility was checked and it is mechanical: no module references anything
defined in `main.rs` (verified by grepping every `crate::` path — they all
resolve to sibling modules). The change is `[lib]` in Cargo.toml, a `lib.rs`
holding the `pub mod` declarations, and `main.rs` importing from it.

It was not done here because restructuring a 14K-line crate is an architectural
decision, not warning cleanup, and this crate's test suite cannot currently be
run end-to-end on this box (the Gemma-4 GGUF was deleted 2026-07-31 — the
artifact on disk was an 8.7 GiB quant, not the 3.2 GB Q4 the plan specified).
Verifying a restructure with a partial suite is not verifying it.

## What was done instead

The two *other* forge categories were resolved properly, taking it 143 → 96:

- **22 × `too_many_arguments`** — annotated at each site. These are BLAS
  wrappers and transformer kernels; `sgemm(m, n, k, alpha, a, lda, b, ldb,
  beta, c, ldc)` is the standard BLAS signature, not bloat. Rationale recorded
  in the `main.rs` crate docs, including why the parameter-struct refactor was
  declined unattended (a mistake shuffling tensor arguments yields silently
  wrong gradients, not a compile error).
- **25 × `needless_range_loop`** — 4 rewritten where the index really was a
  plain cursor (element-wise gradient accumulation now zips; the scrambled
  corpus fill uses `iter_mut().skip()`); the remaining 21 annotated, because
  they index flattened 2-D tensors with a computed stride
  (`b[r * output_dim + o]`) where the counter is arithmetic input to an offset.

Gradient tests (`backward::` numerical checks) pass after both.

---

## Resolution (2026-08-03)

The split was done as described: `src/lib.rs` holds the crate docs and the
`pub mod` declarations, `src/main.rs` is the CLI on top of it. Cargo infers the
lib name (`zhenai_forge`) from the package name, so `Cargo.toml` needed no
`[lib]` stanza.

It cost three compile errors, all the same shape: `main.rs` reached into
`gemma4_gpu`'s `pub(crate)` profiling helpers (`profile_reset`,
`profile_enabled`, `profile_snapshot`). They are now `pub` — with the split
they are genuinely library API, since the CLI is an external consumer.

**96 → 0.** The prediction in this note held: most of the 84 `dead_code`
warnings evaporated because `pub` started meaning "API" again. What remained
was a short list of real findings, which were then fixed properly rather than
annotated:

- **A duplicated layer-0 embedding computation** in `gemma4.rs`'s backward
  pass. A redundant `let layer_input = ...` binding called
  `compute_embed_input`, and its result was discarded via `let _ =` under the
  author's own "clean up" note. Layer 0 computed the scaled embedding twice on
  every backward pass. Now computed once.
  *Caveat: `test_gemma4_backward_grad_health`, the test that would exercise
  this end-to-end, is `#[ignore]`d because it needs the deleted GGUF. The
  numerical backward tests pass and the rewrite is a direct equivalence, but it
  is not covered by a running test.*
- **`GpuTrainer::new` and `GpuTrainer::forward_loss` were `pub` while taking
  `&CpuWeights`** — a type whose fields *and* loader are private, so no caller
  outside the crate could ever construct one. The methods are now
  `pub(crate)`, matching reality, rather than widening `CpuWeights` to paper
  over the leak.
- Three no-op `*mut c_void -> *mut c_void` casts in `hip.rs`; a missing
  `is_empty` beside `GpuBuffer::len`; `checked_div` for two hand-written
  divide-guards in `data.rs`; `clamp` for a `.max(50).min(200)`; and three
  `type` aliases (`NormedInputs`, `LazyAttnCache`, `AttnInputGrads`) for
  signatures clippy flagged as very complex.

Four items were genuinely unused and kept deliberately, each annotated with
its reason rather than a blanket allow: `CpuWeights::forward_loss` and
`CpuWeights::n_total_layers` belong to the Mistral path that CLAUDE.md retains
under the "side stuff" rule, and `per_head_rmsnorm_cpu` /
`gelu_mul_batch_on_gpu` in `gemma4_gpu.rs` are a CPU validation reference and a
working-but-unwired fused-GELU kernel wrapper — both now `pub`, because in a
library that is what they are.

All 104 tests still pass.
