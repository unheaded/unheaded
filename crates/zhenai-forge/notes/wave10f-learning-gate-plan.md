# WAVE10F Learning Gate — Scientist + Developer Joint Plan

**Forged:** 2026-04-20 in an unheaded-scientist + unheaded-developer duet.
**Motivation:** `test_gemma4_gpu_10step_descent` showing loss 19.96 → 0.043 on a
single fixed example proves the gradient plumbing is intact, not that the
model learns. Stevie flagged this: *"we must be cautious we are learning and
not just memorizing."* This plan is the honest test suite.

**Companion:** `wave10f-step-c-battle-plan.md` (GPU port — done) + 
`wave10f-step-c-timing.md` (perf) + `feedback_memorization_vs_learning.md`
(auto-memory rule).

---

## Scientist's Experimental Protocol (verbatim, &lt; 400 words)

### Core Hypothesis (H1)
*Forge's fwd+bwd+Adam pipeline, when run on multi-example training data,
produces LoRA weights whose generalization improves with training.*
Falsifier: held-out eval loss fails to drop meaningfully while train loss
collapses.

### Experiment 1 — Held-out Eval Probe (cornerstone)
Train on corpus T (≥32 sequences, disjoint topics). Every N steps, freeze and
measure mean loss on held-out corpus E (≥32 sequences, same distribution,
zero overlap with T).
- **Memorization shape:** `train → 0`, `eval` flat within 10% of `eval(t=0)`.
- **Learning shape:** both decrease; generalization gap `eval − train` grows
  slowly.
- **Pass:** `eval(t=final)/eval(t=0) ≤ 0.90` with bootstrapped 95% CI
  excluding 1.0 (N=32, n_boot=1000).
- **Fail:** ratio ≥ 0.95 or CI spans 1.0 → plumbing works but no learning.

### Experiment 2 — Scrambled-Label Control
Same protocol, but shuffle next-token targets within each sequence
(preserves distribution, destroys structure).
- **Pass:** scrambled train-loss descent slope &lt; 50% of real train-loss
  descent slope over first 20 steps.

### Experiment 3 — LoRA-Zero Identity Control
Set LoRA-A init to zero so residual is literally zero at t=0; verify
`eval(t=0) == base_model_eval` within fp noise. Confirm
`eval(t=0) > eval(t=final)` after training — rules out accidental baseline
shifts.

### Experiment 4 — Dataset-Size Scaling
Run full protocol at `|T| ∈ {1, 8, 64, 256}`.
- **Memorization:** eval loss curve identical regardless of |T|.
- **Learning:** eval loss descends faster / lower as |T| grows.

### Experiment 5 — Generalization Gap Exponent
At checkpoints `t ∈ {1, 3, 10, 30, 100}`, log `(train, eval, gap)`.
Fit `gap(t) = α·t^β`. **Under learning, β &lt; 1** (sublinear gap growth).
**Under memorization, β → 1**.

### Overall Gate (Learning Claim Requires)
1. Exp 1 PASS at |T|=32.
2. Exp 2 scrambled-slope ratio &lt; 0.5.
3. Exp 4 monotonic improvement in eval-final across |T| ∈ {8, 64}.
4. Exp 5 β &lt; 0.8 (95% CI).

Any single failure blocks the claim — but each failure is diagnostic, not
terminal.

---

## Developer's Implementation Plan

### File layout
```
crates/zhenai-forge/
├── src/
│   ├── eval.rs            # EvalHarness, corpus gen, eval loss
│   └── eval_stats.rs      # bootstrap_ci_95(), linear_fit()
├── tests/
│   └── learning_gate.rs   # integration tests, skip without GGUF
└── notes/
    └── wave10f-learning-gate-plan.md  # this doc
```
Zero new crate deps. Generators are deterministic from a seed.

### Dataset strategy
Synthetic pre-tokenized corpora generated at runtime from a seed. Each
sequence is `[prefix(5) | separator | suffix(6)]` of length 12. Suffix is an
injective deterministic map `Y: token_id → token_id` of the prefix. Train/
eval disjoint by prefix-ID partition (pool_0: train, pool_1: eval). Same `Y`
across both so learning `Y` generalizes; memorizing train prefixes does not.
Scrambled variant keeps prefixes, replaces suffixes with unrelated tokens.

### EvalHarness API (public)
```rust
pub struct EvalHarness {
    pub train: Vec<Vec<u32>>,
    pub eval: Vec<Vec<u32>>,
    pub answer_start: usize,
    pub vocab_size: usize,
}
impl EvalHarness {
    pub fn synthetic(seed: u64, n_train: usize, n_eval: usize,
                     vocab: usize) -> Self;
    pub fn synthetic_scrambled(seed: u64, n_train: usize, n_eval: usize,
                               vocab: usize) -> Self;
    pub fn compute_eval_loss(
        &self, cpu: &CpuWeightsGemma4,
        gpu: Option<&Gemma4GpuWeights>,
        lora: &Gemma4LoraAdapters,
    ) -> Result<EvalStats, String>;
    pub fn run(
        &self, cpu: &CpuWeightsGemma4,
        gpu: Option<&Gemma4GpuWeights>,
        lora: &mut Gemma4LoraAdapters,
        n_steps: usize, eval_every: usize, lr: f32,
    ) -> Result<LearningTrajectory, String>;
}
pub struct EvalStats { pub mean: f32, pub per_seq: Vec<f32>, pub ci95: (f32,f32) }
pub struct LearningTrajectory {
    pub train: Vec<f32>,
    pub checkpoints: Vec<(usize, EvalStats)>,
}
```

### Tests (all `#[ignore]`, run with `cargo test -- --ignored`)
| # | Test | Asserts |
|---|---|---|
| 1 | `test_learning_exp1_held_out_eval` | `ratio ≤ 0.90`, CI excludes 1.0 |
| 2 | `test_learning_exp2_scrambled_labels_control` | `scrambled < 0.5 × real` slope |
| 3 | `test_learning_exp3_lora_zero_identity` | `eval(lora_zero) == base_model_eval` |
| 4 | `test_learning_exp4_dataset_size_scaling` | eval-final monotonic on `|T| ∈ {8,64}` |
| 5 | `test_learning_exp5_generalization_gap_beta` | `β < 0.8` with 95% CI |

### Cost budget (warm GPU, ~2s/step, ~0.3s/eval-seq forward)
| Exp | Wall-clock |
|---|---|
| 1 | ~2.6 min |
| 2 | ~2 min |
| 3 | ~20 s |
| 4 | ~4 min |
| 5 | ~0 (reuses Exp 1) |

**Full suite ≈ 10 min. VRAM unchanged at 4.57 GB.**

### Commit cadence (all behind `#[ignore]`, zero risk to main suite)
- **C1** — `eval_stats.rs` + deterministic unit tests.
- **C2** — `eval.rs` EvalHarness + compute_eval_loss + unit test.
- **C3** — `EvalHarness::run()` wiring `train_step_gemma4_gpu`.
- **C4** — Exp 1 + Exp 3 tests (cheap pair).
- **C5** — Exp 5 β-fit.
- **C6** — Exp 2 scrambled.
- **C7** — Exp 4 scaling.
- **C8** — docs + memory.

### Threshold review after real data
Scientist's thresholds (0.90, 0.5, β&lt;0.8) are strict by design. If Exp 1
passes the "eval drops below baseline with CI excluding 1.0" test but the
ratio is 0.93 instead of 0.90, the honest action is to DOCUMENT the observed
value in timing notes and revise the threshold based on empirical signal —
not to fudge the test. The synthetic corpus may be too small for the
harshest thresholds; the direction matters more than the magnitude.

---

*"Training loss → 0 on a fixed tiny batch is memorization, not learning.
Always hold out an eval sequence when claiming descent."*
— feedback_memorization_vs_learning.md
