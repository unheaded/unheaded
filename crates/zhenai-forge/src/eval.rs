// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! # WAVE10F Learning Gate — eval harness
//!
//! Infrastructure for distinguishing *memorization* from *learning* in
//! zhenai-forge Gemma 4 training. See `notes/wave10f-learning-gate-plan.md`
//! for the experimental protocol (Scientist + Developer joint plan).
//!
//! All corpora are synthetic, deterministic from a seed, and pre-tokenized —
//! forge does not yet include a Gemma 4 SentencePiece tokenizer. The
//! synthetic task is a *prefix-to-suffix injective mapping* `Y` that the
//! model must learn in order to generalize to held-out prefixes.

use crate::eval_stats::{bootstrap_ci_95, Lcg};
use crate::forward;
use crate::gemma4::{CpuWeightsGemma4, Gemma4LoraAdapters};
use crate::gemma4_gpu::Gemma4GpuWeights;

/// Compute mean CE loss over an arbitrary slice of sequences — shares the
/// `EvalHarness::forward_loss` path but works on any corpus (used by Exp 5
/// to measure train-set loss at the same checkpoints as eval-set loss).
pub fn compute_mean_loss_over(
    harness: &EvalHarness,
    cpu: &CpuWeightsGemma4,
    gpu: Option<&Gemma4GpuWeights>,
    lora: Option<&Gemma4LoraAdapters>,
    sequences: &[Vec<u32>],
) -> Result<f32, String> {
    let mut sum = 0.0f32;
    let mut n = 0usize;
    for tokens in sequences {
        sum += harness.forward_loss(cpu, gpu, lora, tokens)?;
        n += 1;
    }
    assert!(n > 0);
    Ok(sum / n as f32)
}

/// Mutate a `Gemma4LoraAdapters` so every LoRA-A matrix is zero. Leaves the
/// (non-zero) B matrices alone — the forward residual becomes `B·0·x = 0`,
/// making the combined model identical to the base. Used by Exp 3.
pub fn zero_lora_a_in_place(lora: &mut Gemma4LoraAdapters) {
    for layer in lora.layers.iter_mut() {
        for slot in layer.iter_mut() {
            if let Some(ll) = slot.as_mut() {
                for v in ll.a.iter_mut() { *v = 0.0; }
            }
        }
    }
}

/// Default sequence layout used by the synthetic corpus generator.
pub const DEFAULT_PREFIX_LEN: usize = 5;
pub const DEFAULT_SUFFIX_LEN: usize = 6;
pub const DEFAULT_SEQ_LEN: usize = DEFAULT_PREFIX_LEN + 1 + DEFAULT_SUFFIX_LEN; // = 12
/// The separator token ID that lives between prefix and suffix.
pub const SEPARATOR_TOKEN: u32 = 1;
/// Prefix token IDs drawn from this range in TRAIN corpus.
pub const TRAIN_PREFIX_POOL: std::ops::Range<u32> = 100..4096;
/// Prefix token IDs drawn from this range in EVAL corpus (disjoint).
pub const EVAL_PREFIX_POOL: std::ops::Range<u32> = 4096..8192;
/// Suffix tokens drawn from this pool via the injective `Y` map.
pub const SUFFIX_POOL: std::ops::Range<u32> = 10000..20000;

/// Loss statistics at a checkpoint.
#[derive(Clone, Debug)]
pub struct EvalStats {
    pub mean: f32,
    pub per_seq: Vec<f32>,
    pub ci95: (f32, f32),
}

/// Full training trajectory: per-step train losses + periodic eval stats.
#[derive(Clone, Debug)]
pub struct LearningTrajectory {
    pub train: Vec<f32>,
    pub checkpoints: Vec<(usize, EvalStats)>,
}

impl LearningTrajectory {
    /// Initial eval (t=0) if captured. Panics if empty.
    pub fn initial_eval(&self) -> &EvalStats {
        &self.checkpoints.first().expect("no checkpoints").1
    }
    /// Final eval (t=last). Panics if empty.
    pub fn final_eval(&self) -> &EvalStats {
        &self.checkpoints.last().expect("no checkpoints").1
    }
}

/// Training + evaluation harness with disjoint train/eval corpora.
pub struct EvalHarness {
    pub train: Vec<Vec<u32>>,
    pub eval: Vec<Vec<u32>>,
    pub answer_start: usize,
    pub vocab_size: usize,
}

impl EvalHarness {
    /// Build a synthetic "learn Y" harness with `n_train`/`n_eval` sequences.
    ///
    /// The generator:
    ///   1. Picks a fixed injective map `Y: prefix_tok → suffix_tok` from
    ///      the concatenated pools using `seed`.
    ///   2. Draws train prefixes from TRAIN_PREFIX_POOL and eval prefixes
    ///      from EVAL_PREFIX_POOL (disjoint ID ranges).
    ///   3. Emits `[prefix | SEPARATOR | Y(prefix)]` of length DEFAULT_SEQ_LEN.
    ///
    /// A model that *memorizes* train prefix→suffix pairs cannot score on
    /// eval (never saw those prefixes). A model that *learns* Y generalizes.
    pub fn synthetic(seed: u64, n_train: usize, n_eval: usize, vocab: usize) -> Self {
        assert!(vocab >= SUFFIX_POOL.end as usize,
            "synthetic corpus requires vocab ≥ {}, got {}", SUFFIX_POOL.end, vocab);
        let y_map = build_y_map(seed);
        let train = gen_sequences(seed.wrapping_add(1), n_train, TRAIN_PREFIX_POOL, &y_map);
        let eval = gen_sequences(seed.wrapping_add(2), n_eval, EVAL_PREFIX_POOL, &y_map);
        Self {
            train,
            eval,
            answer_start: DEFAULT_PREFIX_LEN, // loss only on suffix positions
            vocab_size: vocab,
        }
    }

    /// Build an Exp-2 "scrambled-labels-on-train" variant: same eval corpus
    /// as `base` (real Y on held-out prefixes), but training sequences have
    /// their suffix tokens replaced with RANDOM full-vocab draws. No
    /// structural bias toward a narrow SUFFIX_POOL — the training labels
    /// carry zero useful signal for the eval task. If the model "learns"
    /// under this regime (eval descent), it's fitting accidental structure;
    /// under genuine Y-learning, eval descent should be ~0.
    pub fn with_scrambled_train(base: &EvalHarness, scramble_seed: u64) -> Self {
        let mut rng = Lcg::new(scramble_seed);
        let vocab = base.vocab_size;
        let scrambled_train: Vec<Vec<u32>> = base.train.iter().map(|src| {
            let mut seq = src.clone();
            // Replace the suffix tokens (positions DEFAULT_PREFIX_LEN+1..end)
            // with uniform draws over the full vocab.
            for pos in DEFAULT_PREFIX_LEN + 1..seq.len() {
                seq[pos] = rng.next_range(vocab as u64) as u32;
            }
            seq
        }).collect();
        Self {
            train: scrambled_train,
            eval: base.eval.clone(),
            answer_start: base.answer_start,
            vocab_size: vocab,
        }
    }

    /// Legacy full-scrambled generator (both train and eval have random
    /// suffixes from SUFFIX_POOL). Kept as `#[deprecated]` because the
    /// SUFFIX_POOL bias leaks structure across splits and produces false
    /// positives in the Exp 2 control — see /tmp/exp2-run-v2.log.
    #[deprecated(note = "use with_scrambled_train for Exp 2")]
    pub fn synthetic_scrambled(seed: u64, n_train: usize, n_eval: usize, vocab: usize) -> Self {
        assert!(vocab >= SUFFIX_POOL.end as usize);
        let train = gen_sequences_scrambled(
            seed.wrapping_add(1), n_train, TRAIN_PREFIX_POOL, seed.wrapping_add(11),
        );
        let eval = gen_sequences_scrambled(
            seed.wrapping_add(2), n_eval, EVAL_PREFIX_POOL, seed.wrapping_add(22),
        );
        Self { train, eval, answer_start: DEFAULT_PREFIX_LEN, vocab_size: vocab }
    }

    /// Compute mean cross-entropy loss over `self.eval`. Uses the GPU
    /// forward path if `gpu` is Some, CPU otherwise. Pass `lora = None` for
    /// a pure base-model baseline. Returns per-sequence losses, mean, and
    /// bootstrap 95% CI on the mean.
    pub fn compute_eval_loss(
        &self,
        cpu: &CpuWeightsGemma4,
        gpu: Option<&Gemma4GpuWeights>,
        lora: Option<&Gemma4LoraAdapters>,
    ) -> Result<EvalStats, String> {
        let mut per_seq = Vec::with_capacity(self.eval.len());
        for (i, tokens) in self.eval.iter().enumerate() {
            let loss = self.forward_loss(cpu, gpu, lora, tokens).map_err(|e|
                format!("eval seq {}: {}", i, e))?;
            per_seq.push(loss);
        }
        let mean = per_seq.iter().sum::<f32>() / per_seq.len() as f32;
        let ci95 = bootstrap_ci_95(&per_seq, 1000, 0xEDA1_u64);
        Ok(EvalStats { mean, per_seq, ci95 })
    }

    /// Full training + periodic-evaluation protocol.
    ///
    /// - Runs `n_steps` training steps via `train_step_gemma4_gpu` when
    ///   `gpu` is Some, `train_step_gemma4` otherwise.
    /// - Each step draws one sequence from `self.train` cycling by index.
    /// - Before step 1 and after every `eval_every` steps, runs
    ///   `compute_eval_loss` on the entire held-out set and records the
    ///   checkpoint. Always records a final checkpoint at step `n_steps`.
    ///
    /// Returns the trajectory for the Learning Gate tests to analyze.
    pub fn run(
        &self,
        cpu: &CpuWeightsGemma4,
        gpu: Option<&Gemma4GpuWeights>,
        lora: &mut Gemma4LoraAdapters,
        n_steps: usize,
        eval_every: usize,
        lr: f32,
    ) -> Result<LearningTrajectory, String> {
        assert!(!self.train.is_empty(), "train corpus empty");
        assert!(eval_every >= 1, "eval_every must be ≥1");

        let mut traj = LearningTrajectory {
            train: Vec::with_capacity(n_steps),
            checkpoints: Vec::new(),
        };

        // t=0 eval — baseline before any gradient step.
        let eval0 = self.compute_eval_loss(cpu, gpu, Some(lora))
            .map_err(|e| format!("initial eval: {}", e))?;
        traj.checkpoints.push((0, eval0));

        for step in 1..=n_steps {
            let example = &self.train[(step - 1) % self.train.len()];
            let answer_start = self.answer_start.min(example.len() / 2);
            let loss = match gpu {
                Some(g) => crate::gemma4_gpu::train_step_gemma4_gpu(
                    cpu, g, lora, example, answer_start, lr, step as u32,
                )?,
                None => crate::gemma4::train_step_gemma4(
                    cpu, lora, example, answer_start, lr, step as u32,
                ),
            };
            if !loss.is_finite() {
                return Err(format!("non-finite train loss at step {}: {}", step, loss));
            }
            traj.train.push(loss);

            let should_eval = step % eval_every == 0 || step == n_steps;
            if should_eval {
                let stats = self.compute_eval_loss(cpu, gpu, Some(lora))
                    .map_err(|e| format!("eval @ step {}: {}", step, e))?;
                traj.checkpoints.push((step, stats));
            }
        }
        Ok(traj)
    }

    /// Train until the train loss plateaus or until `max_steps`. Plateau is
    /// detected when the train-loss window `[t-K .. t)` has peak-to-peak range
    /// < `plateau_eps`. Eval is recorded at t=0 and then every
    /// `max(plateau_window/2, 5)` steps, plus a final eval at the stopping
    /// step. Returns `(trajectory, steps_used)`.
    pub fn run_until_plateau(
        &self,
        cpu: &CpuWeightsGemma4,
        gpu: Option<&Gemma4GpuWeights>,
        lora: &mut Gemma4LoraAdapters,
        lr: f32,
        max_steps: usize,
        plateau_window: usize,
        plateau_eps: f32,
    ) -> Result<(LearningTrajectory, usize), String> {
        assert!(!self.train.is_empty());
        assert!(plateau_window >= 2);
        let eval_every = (plateau_window / 2).max(5);
        let mut traj = LearningTrajectory {
            train: Vec::with_capacity(max_steps),
            checkpoints: Vec::new(),
        };
        let eval0 = self.compute_eval_loss(cpu, gpu, Some(lora))
            .map_err(|e| format!("initial eval: {}", e))?;
        traj.checkpoints.push((0, eval0));

        let mut stopped_at = max_steps;
        for step in 1..=max_steps {
            let example = &self.train[(step - 1) % self.train.len()];
            let a_start = self.answer_start.min(example.len() / 2);
            let loss = match gpu {
                Some(g) => crate::gemma4_gpu::train_step_gemma4_gpu(
                    cpu, g, lora, example, a_start, lr, step as u32,
                )?,
                None => crate::gemma4::train_step_gemma4(
                    cpu, lora, example, a_start, lr, step as u32,
                ),
            };
            if !loss.is_finite() {
                return Err(format!("non-finite train loss at step {}: {}", step, loss));
            }
            traj.train.push(loss);

            // Periodic eval checkpoint.
            if step % eval_every == 0 {
                let stats = self.compute_eval_loss(cpu, gpu, Some(lora))
                    .map_err(|e| format!("eval @ step {}: {}", step, e))?;
                traj.checkpoints.push((step, stats));
            }

            // Plateau detection: once we have at least `plateau_window` losses,
            // compare peak-to-peak of the last window.
            if traj.train.len() >= plateau_window {
                let tail = &traj.train[traj.train.len() - plateau_window..];
                let mn = tail.iter().copied().fold(f32::INFINITY, f32::min);
                let mx = tail.iter().copied().fold(f32::NEG_INFINITY, f32::max);
                if mx - mn < plateau_eps {
                    stopped_at = step;
                    break;
                }
            }
        }
        // Always record a final eval at stopping step (if not already captured).
        let last_step_logged = traj.checkpoints.last().map(|(s, _)| *s).unwrap_or(0);
        if last_step_logged != stopped_at {
            let stats = self.compute_eval_loss(cpu, gpu, Some(lora))
                .map_err(|e| format!("final eval: {}", e))?;
            traj.checkpoints.push((stopped_at, stats));
        }
        Ok((traj, stopped_at))
    }

    /// Single-sequence forward + cross-entropy loss over the answer region.
    /// Returns the mean per-position loss so short and long sequences can
    /// be compared.
    pub fn forward_loss(
        &self,
        cpu: &CpuWeightsGemma4,
        gpu: Option<&Gemma4GpuWeights>,
        lora: Option<&Gemma4LoraAdapters>,
        tokens: &[u32],
    ) -> Result<f32, String> {
        self.forward_loss_and_top1(cpu, gpu, lora, tokens).map(|(l, _)| l)
    }

    /// Same as `forward_loss` but also returns the per-sequence mean top-1
    /// accuracy (fraction of answer-region positions where argmax(logits)
    /// matches the ground-truth next token). Top-1 is a harder learning
    /// signal than CE loss: smoothing the output distribution lowers CE,
    /// but to score on top-1 the model must actually *pick* the right
    /// token — which it can only do if it learned the underlying mapping.
    pub fn forward_loss_and_top1(
        &self,
        cpu: &CpuWeightsGemma4,
        gpu: Option<&Gemma4GpuWeights>,
        lora: Option<&Gemma4LoraAdapters>,
        tokens: &[u32],
    ) -> Result<(f32, f32), String> {
        let (logits, _caches) = match gpu {
            Some(g) => crate::gemma4_gpu::forward_gemma4_gpu(cpu, g, lora, tokens)?,
            None => crate::gemma4::forward_gemma4_with_lora(cpu, lora, tokens),
        };
        let vocab = cpu.hparams.vocab_size;
        let seq = tokens.len();
        let loss_start = self.answer_start.max(1);
        if seq <= loss_start {
            return Err(format!("sequence too short: {} ≤ answer_start {}", seq, loss_start));
        }
        let mut total_loss = 0.0f32;
        let mut n_correct = 0usize;
        let mut n = 0usize;
        for pos in loss_start..seq.saturating_sub(1) {
            let row = &logits[pos * vocab..(pos + 1) * vocab];
            let target = tokens[pos + 1];
            total_loss += forward::cross_entropy_loss(row, target);
            // argmax via single pass — no softmax needed.
            let mut arg = 0usize;
            let mut max_v = f32::NEG_INFINITY;
            for (i, &v) in row.iter().enumerate() {
                if v > max_v { max_v = v; arg = i; }
            }
            if arg == target as usize { n_correct += 1; }
            n += 1;
        }
        if n == 0 { return Err("no loss positions".into()); }
        Ok((total_loss / n as f32, n_correct as f32 / n as f32))
    }

    /// Mean top-1 next-token accuracy over `self.eval`.
    pub fn compute_eval_top1(
        &self,
        cpu: &CpuWeightsGemma4,
        gpu: Option<&Gemma4GpuWeights>,
        lora: Option<&Gemma4LoraAdapters>,
    ) -> Result<f32, String> {
        let mut acc_sum = 0.0f32;
        for tokens in self.eval.iter() {
            let (_, top1) = self.forward_loss_and_top1(cpu, gpu, lora, tokens)?;
            acc_sum += top1;
        }
        Ok(acc_sum / self.eval.len() as f32)
    }
}

/// Build the injective prefix→suffix map used by a given seed. We pre-
/// compute the full set of training prefixes + eval prefixes (the union of
/// both pools) and assign each one a fixed random suffix from SUFFIX_POOL.
/// Injectivity isn't strictly required for the learning task (multiple
/// prefixes can share a suffix) but we enforce uniqueness where possible
/// to keep Y well-defined.
fn build_y_map(seed: u64) -> std::collections::HashMap<u32, u32> {
    let mut rng = Lcg::new(seed.wrapping_add(0xBEEF));
    let mut map = std::collections::HashMap::new();
    let suffix_count = SUFFIX_POOL.end - SUFFIX_POOL.start;
    for pfx in TRAIN_PREFIX_POOL.chain(EVAL_PREFIX_POOL) {
        let offset = rng.next_range(suffix_count as u64) as u32;
        map.insert(pfx, SUFFIX_POOL.start + offset);
    }
    map
}

fn gen_sequences(
    seed: u64,
    n: usize,
    prefix_pool: std::ops::Range<u32>,
    y_map: &std::collections::HashMap<u32, u32>,
) -> Vec<Vec<u32>> {
    let mut rng = Lcg::new(seed);
    let pool_size = (prefix_pool.end - prefix_pool.start) as u64;
    let mut out = Vec::with_capacity(n);
    for _ in 0..n {
        // Pick DEFAULT_PREFIX_LEN prefix tokens.
        let mut seq = Vec::with_capacity(DEFAULT_SEQ_LEN);
        let mut prefix_toks = Vec::with_capacity(DEFAULT_PREFIX_LEN);
        for _ in 0..DEFAULT_PREFIX_LEN {
            let pfx = prefix_pool.start + rng.next_range(pool_size) as u32;
            seq.push(pfx);
            prefix_toks.push(pfx);
        }
        seq.push(SEPARATOR_TOKEN);
        // Suffix = deterministic Y applied to (prefix_tok rotated) for length.
        // We cycle prefix_toks to fill DEFAULT_SUFFIX_LEN positions. This
        // ensures each position's correct answer depends on the matching
        // prefix position, which is what attention needs to learn.
        for i in 0..DEFAULT_SUFFIX_LEN {
            let src = prefix_toks[i % DEFAULT_PREFIX_LEN];
            let tgt = *y_map.get(&src).expect("y_map covers prefix pools");
            seq.push(tgt);
        }
        out.push(seq);
    }
    out
}

fn gen_sequences_scrambled(
    seed: u64,
    n: usize,
    prefix_pool: std::ops::Range<u32>,
    suffix_seed: u64,
) -> Vec<Vec<u32>> {
    let mut rng = Lcg::new(seed);
    let mut suf_rng = Lcg::new(suffix_seed);
    let pool_size = (prefix_pool.end - prefix_pool.start) as u64;
    let suffix_count = (SUFFIX_POOL.end - SUFFIX_POOL.start) as u64;
    let mut out = Vec::with_capacity(n);
    for _ in 0..n {
        let mut seq = Vec::with_capacity(DEFAULT_SEQ_LEN);
        for _ in 0..DEFAULT_PREFIX_LEN {
            seq.push(prefix_pool.start + rng.next_range(pool_size) as u32);
        }
        seq.push(SEPARATOR_TOKEN);
        for _ in 0..DEFAULT_SUFFIX_LEN {
            seq.push(SUFFIX_POOL.start + suf_rng.next_range(suffix_count) as u32);
        }
        out.push(seq);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::eval_stats::bootstrap_ratio_ci_95;
    use crate::gemma4::Gemma4LoraAdapters;
    use crate::gemma4_gpu::{Gemma4GpuWeights, PleMode};
    use crate::gguf::GgufFile;

    const MODEL_PATH: &str = "/var/zhen/models/gemma-4-E2B-it.gguf";

    /// Shared setup for the Learning Gate integration tests. Skips (returns
    /// None) if the GGUF isn't present, matching the existing forge test
    /// pattern.
    fn setup_gpu() -> Option<(CpuWeightsGemma4, Gemma4GpuWeights)> {
        if !std::path::Path::new(MODEL_PATH).exists() {
            eprintln!("learning-gate: {} missing — skipping", MODEL_PATH);
            return None;
        }
        let model = GgufFile::open(MODEL_PATH).expect("open gguf");
        let cpu = CpuWeightsGemma4::load(&model).expect("load cpu");
        let gpu = Gemma4GpuWeights::upload(&cpu, PleMode::Cpu).expect("upload");
        Some((cpu, gpu))
    }

    #[test]
    #[ignore] // ~3 min — run with `cargo test -- --ignored`
    fn test_learning_exp1_held_out_eval() {
        // Exp 1 cornerstone: train on 32 seqs, eval on 32 held-out, verify
        // eval descent below 0.90 × baseline with bootstrap CI excluding 1.0.
        let Some((cpu, gpu)) = setup_gpu() else { return };
        let harness = EvalHarness::synthetic(0xE7A1, 32, 32, cpu.hparams.vocab_size);
        let mut lora = Gemma4LoraAdapters::new(&cpu.hparams, 16, 32.0);

        println!("Exp 1: 20-step training, 32 train seqs, 32 eval seqs");
        let start = std::time::Instant::now();
        let traj = harness.run(&cpu, Some(&gpu), &mut lora, 20, 5, 3e-3)
            .expect("learning run");
        println!("  wall-clock: {:.1}s", start.elapsed().as_secs_f64());

        for (step, stats) in &traj.checkpoints {
            println!("  t={:3}: eval={:.4}  CI95=({:.4},{:.4})",
                step, stats.mean, stats.ci95.0, stats.ci95.1);
        }
        let init = traj.initial_eval();
        let fin = traj.final_eval();
        let ratio = fin.mean / init.mean;
        let (ci_lo, ci_hi) = bootstrap_ratio_ci_95(
            &init.per_seq, &fin.per_seq, 2000, 0xBAA5);
        println!("  final/initial ratio = {:.4}  CI95=({:.4},{:.4})",
            ratio, ci_lo, ci_hi);

        assert!(
            ci_hi < 1.0,
            "Exp 1 FAIL: eval/init CI95 must exclude 1.0 — got ({:.4},{:.4}), \
             point ratio {:.4}. This is the memorization-vs-learning gate. \
             If eval didn't drop, forge trained ok on the batch but did not \
             learn the underlying Y map.",
            ci_lo, ci_hi, ratio
        );
        assert!(ratio <= 0.90,
            "Exp 1 THRESHOLD: ratio {:.4} > 0.90 — eval dropped but less \
             than the Scientist's 10% threshold. Consider more steps or \
             larger train set.", ratio);
    }

    #[test]
    #[ignore] // ~4 min — four runs at |T| ∈ {1, 8, 64, 256}
    fn test_learning_exp4_dataset_size_scaling() {
        // Exp 4: eval loss after training should decrease monotonically
        // as |T| grows when the model is genuinely learning Y. Under pure
        // memorization, eval loss would be flat regardless of |T| (the
        // model just fits its 1/8/64/256 seen examples).
        //
        // Gate: eval_final(|T|=64) < eval_final(|T|=8) with a
        //       non-trivial margin (≥2% relative).
        // The full sweep is logged for diagnostic value.
        let Some((cpu, gpu)) = setup_gpu() else { return };
        let n_steps = 20usize;
        let lr = 3e-3f32;
        let n_eval = 32usize;

        let big_harness = EvalHarness::synthetic(
            0xE7A4, 256, n_eval, cpu.hparams.vocab_size);

        let run_at_size = |n_train: usize, gpu: &Gemma4GpuWeights| -> f32 {
            // Use the same Y-map by slicing the largest harness's train.
            // This guarantees all four runs share the same Y across splits.
            let mut lora = Gemma4LoraAdapters::new(&cpu.hparams, 16, 32.0);
            for step in 1..=n_steps {
                let ex = &big_harness.train[(step - 1) % n_train];
                let a_start = big_harness.answer_start.min(ex.len() / 2);
                let _ = crate::gemma4_gpu::train_step_gemma4_gpu(
                    &cpu, gpu, &mut lora, ex, a_start, lr, step as u32,
                ).expect("train");
            }
            big_harness.compute_eval_loss(&cpu, Some(gpu), Some(&lora))
                .expect("eval").mean
        };

        println!("Exp 4: dataset-size scaling ({} steps, same eval set)", n_steps);
        let sizes = &[1usize, 8, 64, 256];
        let mut eval_finals = Vec::new();
        for &n_train in sizes {
            let t0 = std::time::Instant::now();
            let ef = run_at_size(n_train, &gpu);
            println!("  |T|={:3}  eval_final={:.4}  ({:.1}s)",
                n_train, ef, t0.elapsed().as_secs_f64());
            eval_finals.push(ef);
        }

        // Gate: |T|=64 must clearly beat |T|=8.
        let ef8 = eval_finals[1];
        let ef64 = eval_finals[2];
        let rel_improv = (ef8 - ef64) / ef8.max(1e-6);
        println!("Exp 4 gate: eval_final(8)={:.4}, eval_final(64)={:.4}, \
                  relative improvement = {:.3}", ef8, ef64, rel_improv);
        assert!(
            rel_improv > 0.02,
            "Exp 4 FAIL: eval at |T|=64 ({:.4}) must beat |T|=8 ({:.4}) \
             by ≥2%. If eval is flat or worse with more data, the model \
             isn't extracting shared Y structure — it's memorizing \
             per-sequence artifacts.", ef64, ef8
        );
    }

    #[test]
    #[ignore] // ~25 min — two plateau-based training runs (max_steps=200, lr=1e-2)
    fn test_learning_exp2_scrambled_labels_control() {
        // Exp 2 (iteration 5 — PLATEAU-BASED, 2026-04-20):
        //
        // Prior iterations 1-4 all ran at fixed 20 steps and failed to
        // discriminate because NEITHER regime had saturated. Per the
        // Scientist's rigor: step count is the wrong fixed parameter —
        // the principled saturation criterion is train-loss flattening.
        // This iteration trains both regimes until plateau (K=10 window,
        // eps=0.1) at lr=1e-2, up to 200 steps. If scrambled plateaus at
        // low train loss while eval stays high, we have the classic
        // memorization-without-generalization signature.
        //
        // Falsifiable prediction:
        //   scrambled_train_final < 2.0 (memorized noise)
        //   AND scrambled_eval_final > 0.7 × base_eval (no transfer)
        //   AND |real_train − real_eval| < 1.0 (learned + generalized)
        //
        // If the prediction holds → Exp 2 becomes a hard gate.
        // If not → honest documentation of what actually happened (e.g.
        // implicit regularization preventing train→0 on noise, or LoRA
        // rank ceiling).
        let Some((cpu, gpu)) = setup_gpu() else { return };

        let lr = 1e-2f32;
        let max_steps = 200usize;
        let plateau_window = 10usize;
        let plateau_eps = 0.1f32;
        let n_train = 32usize;
        let n_eval = 32usize;

        let run_plateau = |harness: &EvalHarness,
                           gpu: &Gemma4GpuWeights,
                           label: &str| -> (f32, f32, f32, usize) {
            let mut lora = Gemma4LoraAdapters::new(&cpu.hparams, 16, 32.0);
            let t0 = std::time::Instant::now();
            let (traj, steps_used) = harness.run_until_plateau(
                &cpu, Some(gpu), &mut lora, lr, max_steps, plateau_window, plateau_eps,
            ).expect("plateau run");
            let train_final = traj.train.last().copied().unwrap_or(f32::NAN);
            // Full train-set loss after training (one sample per step is noisy).
            let train_set_mean = compute_mean_loss_over(
                harness, &cpu, Some(gpu), Some(&lora), &harness.train,
            ).expect("train-set loss");
            let eval_final = traj.final_eval().mean;
            let eval0 = traj.initial_eval().mean;
            let elapsed = t0.elapsed().as_secs_f64();
            println!(
                "  {}: steps_used={}/{}  train_last_step={:.4}  train_set_mean={:.4}  \
                 eval(0)={:.4}  eval_final={:.4}  ({:.1}s)",
                label, steps_used, max_steps, train_final, train_set_mean,
                eval0, eval_final, elapsed,
            );
            // Log the train-loss trajectory for forensic analysis.
            print!("    train traj samples: ");
            for (i, &t) in traj.train.iter().enumerate() {
                if i % 20 == 0 || i + 1 == traj.train.len() {
                    print!("{}={:.3} ", i + 1, t);
                }
            }
            println!();
            (train_set_mean, eval0, eval_final, steps_used)
        };

        let real_h = EvalHarness::synthetic(0xE7A2, n_train, n_eval, cpu.hparams.vocab_size);
        let scr_h = EvalHarness::with_scrambled_train(&real_h, 0xBAD_5EED);

        println!("Exp 2 (iter 5): plateau-based, lr={}, max_steps={}, K={}, eps={}",
            lr, max_steps, plateau_window, plateau_eps);
        let (real_train, real_e0, real_ef, _) = run_plateau(&real_h, &gpu, "real Y");
        let (scr_train, _scr_e0, scr_ef, _) = run_plateau(&scr_h, &gpu, "scrambled");

        // Falsifiable prediction.
        let scr_train_low = scr_train < 2.0;
        let scr_eval_stays = scr_ef > 0.7 * real_e0;
        let real_converges = (real_train - real_ef).abs() < 1.0;
        println!(
            "Exp 2 prediction check:\n  \
             scr_train<2.0  : {} (got {:.4})\n  \
             scr_eval>0.7×base : {} (got {:.4} vs 0.7×{:.4}={:.4})\n  \
             |real_train − real_eval|<1.0 : {} (got |{:.4}−{:.4}|={:.4})",
            scr_train_low, scr_train,
            scr_eval_stays, scr_ef, real_e0, 0.7 * real_e0,
            real_converges, real_train, real_ef, (real_train - real_ef).abs(),
        );

        // Sanity: real Y must still descend on its own eval set.
        assert!(real_ef < real_e0,
            "Exp 2 sanity: real-Y eval must descend ({:.4} -> {:.4})", real_e0, real_ef);

        // Hard gate — all three predictions must hold.
        assert!(scr_train_low, "Exp 2 FAIL: scrambled_train {:.4} ≥ 2.0 — \
             scrambled regime did NOT saturate to memorization. Either \
             lr=1e-2 insufficient or LoRA rank ceiling hit.", scr_train);
        assert!(scr_eval_stays, "Exp 2 FAIL: scrambled_eval {:.4} ≤ 0.7 × \
             base_eval {:.4} — scrambled training generalized more than \
             expected (output smoothing? accidental structure?).",
             scr_ef, real_e0);
        assert!(real_converges, "Exp 2 FAIL: real_train {:.4} and real_eval \
             {:.4} differ by ≥1.0 — real-Y run did not converge cleanly.",
             real_train, real_ef);
        println!("Exp 2 GATE: PASS — scrambled memorizes without generalizing, \
             real Y learns and generalizes.");
    }

    #[test]
    #[ignore] // ~5 min — measures generalization-gap growth exponent
    fn test_learning_exp5_generalization_gap_beta() {
        // Exp 5: at checkpoints t ∈ {1, 3, 10, 30, 100} (scaled to the
        // budget: we use {1, 3, 10, 20}), fit gap(t) = α·t^β. Under
        // memorization β → 1; under learning β < 0.8.
        use crate::eval_stats::fit_power_law_beta;
        let Some((cpu, gpu)) = setup_gpu() else { return };
        let harness = EvalHarness::synthetic(0xE7A5, 32, 32, cpu.hparams.vocab_size);
        let mut lora = Gemma4LoraAdapters::new(&cpu.hparams, 16, 32.0);

        let checkpoints: &[usize] = &[1, 3, 10, 20];
        let lr = 3e-3f32;
        let mut gaps: Vec<f32> = Vec::new();
        let mut last_step = 0usize;
        println!("Exp 5: β-fit on gap(t) = eval(t) − train_set(t)");
        for &target_step in checkpoints {
            while last_step < target_step {
                last_step += 1;
                let ex = &harness.train[(last_step - 1) % harness.train.len()];
                let a_start = harness.answer_start.min(ex.len() / 2);
                let _ = crate::gemma4_gpu::train_step_gemma4_gpu(
                    &cpu, &gpu, &mut lora, ex, a_start, lr, last_step as u32,
                ).expect("train");
            }
            let train_mean = compute_mean_loss_over(
                &harness, &cpu, Some(&gpu), Some(&lora), &harness.train,
            ).expect("train-set eval");
            let eval_stats = harness.compute_eval_loss(&cpu, Some(&gpu), Some(&lora))
                .expect("eval");
            let gap = (eval_stats.mean - train_mean).max(1e-4); // log-safe
            println!("  t={:2}  train={:.4}  eval={:.4}  gap={:.4}",
                target_step, train_mean, eval_stats.mean, gap);
            gaps.push(gap);
        }
        let (beta, lo, hi) = fit_power_law_beta(checkpoints, &gaps);
        println!("Exp 5: β = {:.3}  95% CI = ({:.3}, {:.3})", beta, lo, hi);
        assert!(
            hi < 0.8,
            "Exp 5 FAIL: β CI upper {:.3} ≥ 0.8 — gap is growing too fast. \
             Either gradient plumbing is leaking noise OR the optimizer is \
             overfitting the train set faster than it generalizes.", hi
        );
    }

    #[test]
    #[ignore] // ~40s
    fn test_learning_exp3_lora_zero_identity() {
        // Exp 3: A=0 ⇒ B·A·x = 0 residual ⇒ model output must match
        // base model (no LoRA) within fp noise. Then verify training
        // reduces eval below the A=0 starting point.
        let Some((cpu, gpu)) = setup_gpu() else { return };
        let harness = EvalHarness::synthetic(0xE7A3, 8, 16, cpu.hparams.vocab_size);

        // Step 1: lora with A zeroed out.
        let mut lora = Gemma4LoraAdapters::new(&cpu.hparams, 16, 32.0);
        zero_lora_a_in_place(&mut lora);

        let eval_zero_a = harness.compute_eval_loss(&cpu, Some(&gpu), Some(&lora))
            .expect("eval lora-zero-A");
        let eval_base = harness.compute_eval_loss(&cpu, Some(&gpu), None)
            .expect("eval base");

        println!("Exp 3: lora-zero-A eval  = {:.6}", eval_zero_a.mean);
        println!("Exp 3: base-model  eval  = {:.6}", eval_base.mean);
        // Per-sequence equivalence within bf16 round-trip noise.
        for (i, (a, b)) in eval_zero_a.per_seq.iter()
            .zip(eval_base.per_seq.iter()).enumerate()
        {
            let rel = (a - b).abs() / b.abs().max(1e-6);
            assert!(rel < 1e-3,
                "Exp 3 identity FAIL seq {}: lora-zero-A={:.6} base={:.6} rel_err={:.2e}",
                i, a, b, rel);
        }

        // Step 2: training must reduce eval below the A=0 baseline.
        let traj = harness.run(&cpu, Some(&gpu), &mut lora, 15, 5, 3e-3)
            .expect("learning run");
        let fin = traj.final_eval();
        println!("Exp 3: post-train  eval  = {:.6}", fin.mean);
        assert!(fin.mean < eval_zero_a.mean,
            "Exp 3 FAIL: training must lower eval from the A=0 baseline. \
             baseline={:.4}, final={:.4}", eval_zero_a.mean, fin.mean);
    }

    #[test]
    fn test_synthetic_train_eval_disjoint_prefixes() {
        let h = EvalHarness::synthetic(7, 16, 16, 262_144);
        let train_pfx: std::collections::HashSet<u32> = h
            .train.iter()
            .flat_map(|s| s[..DEFAULT_PREFIX_LEN].iter().copied())
            .collect();
        let eval_pfx: std::collections::HashSet<u32> = h
            .eval.iter()
            .flat_map(|s| s[..DEFAULT_PREFIX_LEN].iter().copied())
            .collect();
        let intersection: Vec<_> = train_pfx.intersection(&eval_pfx).collect();
        assert!(intersection.is_empty(),
            "train/eval prefixes must be disjoint, got {:?}", intersection);
        assert!(!train_pfx.is_empty() && !eval_pfx.is_empty());
    }

    #[test]
    fn test_synthetic_shared_y_map_across_splits() {
        // Same seed → same Y. Train and eval suffix-for-prefix-X must match
        // when prefix pools overlap. They don't (disjoint by construction),
        // but the same Y must be used to build both, so if we independently
        // instantiate harnesses with the same seed we get identical Y-maps.
        let a = build_y_map(42);
        let b = build_y_map(42);
        assert_eq!(a, b);
        // Different seeds produce different maps.
        let c = build_y_map(43);
        assert_ne!(a, c);
    }

    #[test]
    fn test_synthetic_sequence_shape() {
        let h = EvalHarness::synthetic(1, 4, 4, 262_144);
        for seq in h.train.iter().chain(h.eval.iter()) {
            assert_eq!(seq.len(), DEFAULT_SEQ_LEN);
            assert_eq!(seq[DEFAULT_PREFIX_LEN], SEPARATOR_TOKEN);
            for &t in &seq[..DEFAULT_PREFIX_LEN] {
                assert!(TRAIN_PREFIX_POOL.contains(&t) || EVAL_PREFIX_POOL.contains(&t));
            }
            for &t in &seq[DEFAULT_PREFIX_LEN + 1..] {
                assert!(SUFFIX_POOL.contains(&t),
                    "suffix token {} outside SUFFIX_POOL", t);
            }
        }
    }

    #[test]
    fn test_with_scrambled_train_preserves_eval() {
        let base = EvalHarness::synthetic(11, 8, 16, 262_144);
        let scr = EvalHarness::with_scrambled_train(&base, 0xDEADB33F);
        // Eval set is identical (byte-for-byte) so both experiments score
        // against the same Y-mapped target distribution.
        assert_eq!(base.eval, scr.eval,
            "with_scrambled_train must preserve the eval corpus");
        // Train prefixes preserved; suffixes altered for most sequences.
        assert_eq!(base.train.len(), scr.train.len());
        let mut altered = 0;
        for (a, b) in base.train.iter().zip(scr.train.iter()) {
            assert_eq!(a[..DEFAULT_PREFIX_LEN + 1], b[..DEFAULT_PREFIX_LEN + 1],
                "train prefix+separator must be unchanged");
            if a[DEFAULT_PREFIX_LEN + 1..] != b[DEFAULT_PREFIX_LEN + 1..] {
                altered += 1;
            }
        }
        assert!(altered >= base.train.len() - 1,
            "scrambling should alter nearly every suffix, got {}/{}",
            altered, base.train.len());
    }
}
