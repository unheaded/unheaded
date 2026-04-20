# WAVE10F 24-Hour Battle Plan — Post-Learning-Gate Consolidation

**Date forged:** 2026-04-20
**Sprint:** 24-hour GPU/CPU block on west (RX 7700 XT, 12.87 GB VRAM)
**Prerequisite:** WAVE10F Step C done, Learning Gate 4/5 passing, branch `main` clean.
**Target:** Exp 2 promoted to hard gate, lr sweep published, multi-Y-map probe, Kingdom QA RAFT kickoff (if corpus tokenizable), regression audit, session handoff.
**Estimated Duration:** 24 wall-clock hours on west
**Agent Strategy:** solo/coordinator — GPU serializes runs
**Commit Cadence:** every 3-5 steps, phase-exit commit mandatory
**Stuck Protocol:** skip after 3× time estimate or 2 failed debug attempts

---

## LEGEND

```
[B]  bash command            [V]  verification gate
[D]  debug branch            [W]  write file
[R]  read file               [C]  commit checkpoint
[CODE] in-source edit        [TEST] cargo test run
[DECIDE] autonomous choice   [ESCALATE] STOP for human
[STUCK] skipped per protocol [BLOCKED] blocked by upstream STUCK
[GPU:t=Xs] explicit GPU time budget
[ABORT-IF] per-phase kill criterion
```

---

## CRITICAL FACTS (cached for executing agent)

- **Repo**: `/home/govan/tmp/unheaded/` (branch `main`, +54 commits ahead of origin)
- **Crate**: `crates/zhenai-forge/`
- **Model**: `/var/zhen/models/gemma-4-E2B-it.gguf` (9.3 GB, E2B Gemma 4)
- **Kingdom RAFT corpus**: `raft/training/train.jsonl` (3568 ex), `raft/training/eval.jsonl` (397 ex). **Raw text** in Mistral chat template (`<s>[INST] ... [/INST]`), NOT pre-tokenized. Gemma 4 tokenization is a prerequisite for any real RAFT run.
- **Warm GPU step**: ~2.0s / 2.0s warm after ~22s cold upload
- **Setup overhead**: ~45s per test (weights load + GPU upload)
- **Current commits of interest**: `df56efbd` (Learning Gate C8), `0f4ec29f` (Exp 4), `55d00362` (Exp 2 diagnostic).
- **Existing harness in `src/eval.rs`**: EvalHarness, synthetic/with_scrambled_train, compute_eval_loss, compute_eval_top1, zero_lora_a_in_place, run.
- **Stats helpers in `src/eval_stats.rs`**: Lcg (xorshift64*), bootstrap_ci_95, bootstrap_ratio_ci_95, linear_fit, fit_power_law_beta.

---

## TIME BUDGET SUMMARY

| Phase | Name | Wall-clock | GPU time | Cumulative |
|------:|------|-----------:|---------:|-----------:|
| 0 | Recon + preflight | 30 min | 0 | 0.5 h |
| 1 | Exp 2 hard gate (plateau) | 1 h | ~40 min | 1.5 h |
| 2 | Exp 1 extended + lr sweep | 3 h | ~2.5 h | 4.5 h |
| 3 | Multi-Y-map capacity probe | 2 h | ~1.5 h | 6.5 h |
| 4 | Tokenizer vertical slice | 6 h | 0–30 min | 12.5 h |
| 5 | Kingdom QA RAFT kickoff | 8 h | up to 7 h | 20.5 h |
| 6 | Regression + timing audit | 2 h | ~1 h | 22.5 h |
| 7 | Docs + session handoff | 1.5 h | 0 | 24 h |

GPU-heavy phases (2, 3, 5) serialize on the GPU. If Phase 5 bails early (no tokenizer), budget rolls into Phases 2/3/6 extensions or starts Phase 7 ahead of schedule.

---

## PHASE 0: RECON + PREFLIGHT (Steps 1-20) — 30 min

**Goal:** Verify ground state, capture baselines, re-read Learning Gate artifacts. No code changes.
**Prerequisite:** fresh session, working tree clean.
**ABORT-IF:** GGUF missing OR working tree dirty OR any existing green test fails in baseline sweep.

- [ ] **Step 1** [B] ~5s: `cd ~/tmp/unheaded && git status && git log --oneline -5`
- [ ] **Step 2** [V]: HEAD is `df56efbd` or descendant; tree clean.
- [ ] **Step 3** [B]: `ls -lh /var/zhen/models/gemma-4-E2B-it.gguf && awk '/^MemAvailable/ {printf "Mem: %.1f GB\n", $2/1024/1024}' /proc/meminfo`
- [ ] **Step 4** [V]: GGUF present (≈9.3 GB), MemAvailable ≥ 9 GB.
- [ ] **Step 5** [R]: Skim `crates/zhenai-forge/notes/wave10f-learning-gate-plan.md` outcome section (DONE banner).
- [ ] **Step 6** [R]: Skim `crates/zhenai-forge/src/eval.rs` (harness + tests).
- [ ] **Step 7** [R]: Skim `crates/zhenai-forge/src/eval_stats.rs` (stats helpers).
- [ ] **Step 8** [B] ~5s: `find /home/govan/tmp/unheaded/raft/training -name "*.jsonl" -exec wc -l {} +`
- [ ] **Step 9** [V]: Kingdom corpus present. Note format in scratch file:
  ```bash
  head -1 /home/govan/tmp/unheaded/raft/training/train.jsonl > /tmp/24h-kingdom-sample.json
  ```
- [ ] **Step 10** [DECIDE]: Determine corpus tokenization path. **RECOMMENDATION**: Python-side tokenization with llama.cpp converter is faster than full SentencePiece integration in Rust (Phase 4 vertical slice produces the bootstrap, Phase 5 uses it). Override only if `llama-cpp-python` or equivalent isn't importable on west.
- [ ] **Step 11** [B] ~30s: `python3 -c "import sentencepiece; print(sentencepiece.__version__)" 2>&1; python3 -c "import llama_cpp; print(llama_cpp.__version__)" 2>&1`
- [ ] **Step 12** [V]: record which tokenizer paths are available. (Either is fine; absence of both triggers [STUCK] flag for Phase 5 with rollover to Phases 2/3/6 deepening.)
- [ ] **Step 13** [B] ~90s: cargo sanity — tests still pass.
  ```bash
  cd ~/tmp/unheaded/crates/zhenai-forge && cargo test --release --bin zhenai-forge eval_stats:: eval::tests::test_synthetic --no-fail-fast 2>&1 | tail -5
  ```
- [ ] **Step 14** [V]: all unit tests green (baseline). If any fail → [ESCALATE] before any further change.
- [ ] **Step 15** [W]: create `/tmp/24h-known-failures.txt` — currently empty. Updated as session progresses.
- [ ] **Step 16** [B]: `nvidia-smi || rocm-smi 2>&1 | head -20` — record baseline VRAM.
- [ ] **Step 17** [V]: VRAM ~free (≥10 GB available). If a stale process is holding VRAM, kill it or escalate.
- [ ] **Step 18** [W]: scratch file `/tmp/24h-session-log.md` with header: date, commits of interest, tokenizer path chosen.
- [ ] **Step 19** [B]: `git stash list && git status` — final drift check.
- [ ] **Step 20** [V]: **PHASE 0 EXIT GATE** — all green. Proceed.

Rollback: if any verification fails, stop and escalate. No code touched yet.

---

## PHASE 1: EXP 2 HARD GATE — PLATEAU-BASED (Steps 21-60) — 1 h

**Goal:** Promote Exp 2 from diagnostic → hard gate via plateau-based training at lr=1e-2. Falsifiable prediction: `scrambled_train_final < 2.0 AND scrambled_eval_final > 0.7 × base_eval` while `|real_train − real_eval| < 1.0`.
**Prerequisite:** Phase 0 clean.
**ABORT-IF:** Exp 2 plateau-training produces NaN train loss on either regime → back off to lr=3e-3; if still NaN, [STUCK], skip to Phase 2.

### Implementation

- [ ] **Step 21** [CODE] in `src/eval.rs` — add a new method on `EvalHarness`:
  ```rust
  pub fn run_until_plateau(&self, cpu: &CpuWeightsGemma4, gpu: Option<&Gemma4GpuWeights>,
      lora: &mut Gemma4LoraAdapters, lr: f32, max_steps: usize,
      plateau_window: usize, plateau_eps: f32,
  ) -> Result<(LearningTrajectory, usize /* steps_used */), String>
  ```
  Detects plateau via rolling window: if `max(train[t-K..t]) − min(train[t-K..t]) < plateau_eps` for `K=plateau_window`, stop. Always logs per-step train loss. Periodic eval at every `max(K/2, 5)` steps.
- [ ] **Step 22** [BUILD] ~30s: `cargo build --release --tests 2>&1 | tail -3`.
- [ ] **Step 23** [V]: build green.
- [ ] **Step 24** [C]: commit `[PLAN 24h] Step 21-23: run_until_plateau added`.
- [ ] **Step 25** [CODE] in `src/eval.rs` tests — rewrite `test_learning_exp2_scrambled_labels_control` to use `run_until_plateau(max_steps=200, K=10, eps=0.1, lr=1e-2)`. Assert:
  - `real_traj` train converges (plateau detected OR max_steps).
  - `scr_traj` train converges (plateau detected OR max_steps).
  - `(eval0.mean - base_eval).abs() < 1e-3` as a sanity pre-check.
  - Gate: `scr_train_final < 2.0` AND `scr_eval_final > 0.7 × eval0` AND `abs(real_train_final − real_eval_final) < 1.0`.
- [ ] **Step 26** [CODE]: leave the diagnostic-mode prints (train/eval trajectory) so failures give actionable output.
- [ ] **Step 27** [BUILD] ~30s.
- [ ] **Step 28** [V]: build green.
- [ ] **Step 29** [C]: commit `[PLAN 24h] Step 25-28: Exp 2 plateau-based, lr=1e-2`.

### Execution (GPU)

- [ ] **Step 30** [TEST] [GPU:t=45s setup] ~25min: run Exp 2:
  ```bash
  cd ~/tmp/unheaded/crates/zhenai-forge && \
    cargo test --release --bin zhenai-forge test_learning_exp2_scrambled_labels_control -- \
    --ignored --test-threads=1 --nocapture 2>&1 | tee /tmp/24h-exp2.log
  ```
  Budget: 200 steps × 2s × 2 runs = ~800s + eval overhead ≈ 20-25 min.
- [ ] **Step 31** [V]: parse log. Record (real_train_final, real_eval_final, scr_train_final, scr_eval_final, steps_used_real, steps_used_scr, ratio).
- [ ] **Step 32** [DECIDE]: Did plateau detection trigger before max_steps?
  - If YES on both → training actually saturated; results are authoritative.
  - If NO (hit max_steps) → discriminative signal may still exist but we didn't reach floor. Re-run with max_steps=400 (budget allowing). **RECOMMENDATION**: bump only if scr_train_final > 5.0 (clearly not saturated).
- [ ] **Step 33** [V]: **EXP 2 GATE OUTCOME**:
  - PASS: falsifiable prediction holds → celebrate, update plan doc, promote Exp 2 to hard-gate in session log.
  - FAIL: prediction disconfirmed → do NOT fake-pass. Log actual numbers to `/tmp/24h-exp2-outcome.md`. Execute contingency below.
- [ ] **Step 34** [DECIDE — contingency if Exp 2 FAILS]: possible causes:
  - (A) LoRA capacity insufficient to fully memorize scrambled noise → rank=16 produces gentle smoothing, not hard memorization. Fix: double max_steps OR try rank=32 (capacity bump).
  - (B) Adam implicit regularization prevents train→0 on noise → try SGD-like step (β1=0.5) or higher lr.
  - (C) The scrambled corpus itself retains residual structure → audit with `synthetic_has_no_y_structure` variant.
  - **RECOMMENDATION**: run ONE follow-up attempt at `max_steps=400, lr=3e-2` before declaring Exp 2 "longer-training-only" signal and pushing to Phase 5 real-data regime.
- [ ] **Step 35** [TEST] [GPU:t=20min, conditional on Step 34]: rerun Exp 2 with max_steps=400, lr=3e-2 (EDIT the test constants, don't add another function).
- [ ] **Step 36** [V]: did it discriminate? Record result.

### Cleanup + commit

- [ ] **Step 37** [CODE]: update Exp 2 test header comment with the FIVE total iterations and the FINAL resolution.
- [ ] **Step 38** [W]: `/tmp/24h-exp2-outcome.md` — data table of all six tries.
- [ ] **Step 39** [W]: append to `notes/wave10f-learning-gate-plan.md` — Exp 2 resolution block:
  - If hard gate achieved: update scorecard, mark ✅.
  - If still diagnostic: update attempts table, add Scientist's "ran into implicit-reg / capacity ceiling" finding, mark 🔍 with justification.
- [ ] **Step 40** [C]: commit `[PLAN 24h] Phase 1: Exp 2 plateau outcome — [PASS|DIAG]`.

- [ ] **Step 41** [V]: **PHASE 1 EXIT GATE**:
  - Exp 2 test exits with deterministic PASS or a documented honest FAIL.
  - `/tmp/24h-exp2-outcome.md` contains the numerical trajectory.
  - No green test went red (run `cargo test --release eval_stats:: eval::tests::test_synthetic` for sanity).

**Steps 42-60 reserved** for debug branches [D] if needed — NaN recovery, lr tuning, plateau criterion tweaks, corpus inspection. If unused, skip to Phase 2 at Step 61 and note phase ended early.

---

## PHASE 2: EXP 1 EXTENDED + LR SWEEP (Steps 61-120) — 3 h

**Goal:** Tighten Exp 1's CI by running longer (up to plateau), and sweep lr ∈ {1e-3, 3e-3, 1e-2, 3e-2} to measure sensitivity. Ship an lr-vs-eval-descent curve.
**Prerequisite:** Phase 1 resolved (either PASS or documented DIAG).
**ABORT-IF:** all four lr values produce `ratio > 0.90` → Exp 1 may be seed-sensitive → flag to Scientist before Phase 3.

### Setup

- [ ] **Step 61** [CODE] in `src/eval.rs` tests — add `test_learning_exp1_lr_sweep` [ignore] that runs Exp 1's protocol at each of four lrs with max_steps=100 and plateau detection. Records `(lr, steps_used, eval_final_ratio, ci_lo, ci_hi)` per lr.
- [ ] **Step 62** [CODE]: also add `test_learning_exp1_extended` [ignore] — Exp 1 at the current lr=3e-3 but with `run_until_plateau(max_steps=100, K=10, eps=0.1)`. Asserts same ratio ≤0.90 as before but reports whether plateau was reached.
- [ ] **Step 63** [BUILD] ~30s.
- [ ] **Step 64** [V]: build green.
- [ ] **Step 65** [C]: commit `[PLAN 24h] Step 61-64: Exp 1 extended + lr_sweep test scaffolding`.

### Execution

- [ ] **Step 66** [TEST] [GPU:t=10min]: `test_learning_exp1_extended -- --ignored --nocapture | tee /tmp/24h-exp1-ext.log`
- [ ] **Step 67** [V]: ratio ≤ 0.90, CI excludes 1.0, plateau status logged.
- [ ] **Step 68** [TEST] [GPU:t=40min]: `test_learning_exp1_lr_sweep -- --ignored --nocapture | tee /tmp/24h-exp1-lr.log`
  Budget: 4 lrs × ~100 steps × 2s = 800s + 4 evals × 10s ≈ 14 min on fastest path, up to 40 if plateaus trigger late.
- [ ] **Step 69** [V]: log shows finite metrics for all 4 lrs. Record table.
- [ ] **Step 70** [DECIDE]: which lr is optimal for the final ratio? **RECOMMENDATION**: whichever minimizes `ratio` with CI upper bound below 0.80. If tie, prefer lower lr (stability).

### Analysis + deliverable

- [ ] **Step 71** [W]: write `notes/wave10f-lr-sweep.md` — table of (lr, steps, ratio, CI), plus short conclusion.
- [ ] **Step 72** [C]: commit `[PLAN 24h] Phase 2: lr sweep notes + chosen optimal lr`.
- [ ] **Step 73** [V]: **PHASE 2 EXIT GATE**:
  - lr-sweep table in place.
  - At least 2 of 4 lrs pass the original gate.
  - Optimal lr noted — adopted as default for Phases 3-5.

- [ ] **Step 74** [CODE, conditional]: if optimal lr ≠ 3e-3, update Phase 3/5 tests to use it.

**Steps 75-120 reserved** for follow-on sensitivity runs (rank sweep r∈{4,16,32} if time allows), deeper CI tightening via n_eval=128 runs, debug branches. If optimal lr is decisively better and we have >30 min budget surplus, run ONE rank sweep to inform Phase 3:
- [ ] **Step 100** [TEST, conditional][GPU:t=30min]: rank-sweep at optimal lr — 3 ranks × ~100 steps. Log `notes/wave10f-rank-sweep.md`.

---

## PHASE 3: MULTI-Y-MAP CAPACITY PROBE (Steps 121-165) — 2 h

**Goal:** Does attention learn multiple disjoint Y maps simultaneously? Add a harness that generates training data drawn from two (or three) Y maps, each with a distinguishing prefix token. Test whether eval loss descends on BOTH Y maps after training.
**Prerequisite:** Phase 2 complete with optimal lr identified.
**ABORT-IF:** single-Y baseline drops 42% but dual-Y drops <10% on either map → attention saturated → abort probe, document the capacity ceiling, jump to Phase 4.

### Design

- [ ] **Step 121** [DESIGN]: define multi-Y schema:
  - Add a "context marker" token at position 0 of each sequence: 0xA0 = "Y_A", 0xA1 = "Y_B".
  - Build two independent injective maps `Y_A` and `Y_B` with different seeds.
  - Training data: half sequences prefixed 0xA0 with Y_A-mapped suffixes, half prefixed 0xA1 with Y_B.
  - Eval data: same structure, held-out prefixes, both Y_A and Y_B sequences in eval.
- [ ] **Step 122** [CODE] in `src/eval.rs`: add `EvalHarness::synthetic_multi_y(seed, n_train, n_eval, vocab, n_maps) -> Self` with metadata tracking which map each sequence belongs to.
- [ ] **Step 123** [CODE]: extend `compute_eval_loss` OR add `compute_per_group_eval` that returns per-Y-map breakdown: `Vec<(group_id, EvalStats)>`.
- [ ] **Step 124** [CODE]: unit test for the new generator — verifies (a) eval sequences come from both groups, (b) train/eval prefix pools disjoint within each group, (c) the two Y maps do not accidentally collide.
- [ ] **Step 125** [BUILD] + [TEST]: unit tests green.
- [ ] **Step 126** [C]: commit `[PLAN 24h] Step 121-125: multi-Y harness`.

### Integration test

- [ ] **Step 127** [CODE]: integration test `test_learning_exp6_multi_y_capacity` [ignore]:
  - Train on 64 sequences (32 per Y), up to 100 steps, optimal lr, rank=16.
  - Record per-Y eval descent.
  - Assert: EACH of Y_A eval ratio ≤ 0.85, Y_B eval ratio ≤ 0.85 (slightly looser than single-Y because capacity is split).
  - Bonus: run 3-Y variant with 96 training sequences as a stress probe; mark as optional/informational (no hard assert).
- [ ] **Step 128** [BUILD] + [TEST] [GPU:t=15min]: run.
  Budget: 100 steps × 2s + 2×eval-passes × 32 × 0.3s ≈ 5 min training + 2 min eval = ~7 min. Plus 3-Y variant ~10 min.
- [ ] **Step 129** [V]: both Y_A and Y_B descend per their thresholds.

### Analysis

- [ ] **Step 130** [W]: `notes/wave10f-multi-y-capacity.md`:
  - Single-Y baseline (from Phase 2): ratio X
  - 2-Y per-group ratios: Y_A, Y_B
  - 3-Y per-group ratios (if run): Y_A, Y_B, Y_C
  - Conclusion: does forge scale with Y count? At what point does each individual Y_i suffer?
- [ ] **Step 131** [C]: commit `[PLAN 24h] Phase 3: multi-Y capacity probe`.

- [ ] **Step 132** [V]: **PHASE 3 EXIT GATE**:
  - Multi-Y integration test passes (or cleanly aborts with documented capacity ceiling).
  - `notes/wave10f-multi-y-capacity.md` persisted.
  - No green test went red.

**Steps 133-165 reserved** for rank-sweep crossover experiments — if 2-Y fails at rank=16, try rank=32 and report whether capacity increase recovers; if succeeds, try rank=4 to find the floor. Document either way.

---

## PHASE 4: TOKENIZER VERTICAL SLICE (Steps 166-230) — 6 h

**Goal:** Produce a Python-side Gemma 4 tokenizer wrapper that emits `{"tokens": [int, ...]}` JSONL from Mistral-formatted Kingdom text — unblocks Phase 5. Full Rust SentencePiece integration deferred to a future sprint.
**Prerequisite:** Phase 0 Step 11/12 recorded which tokenizer is available (`llama_cpp` or `sentencepiece`).
**ABORT-IF:** neither `llama_cpp` nor `sentencepiece` importable AND `python3 -m pip install` restricted → [STUCK] the whole phase, skip to Phase 6.

### Tokenizer wrapper

- [ ] **Step 166** [DECIDE]: pick backend:
  - Preferred: `llama_cpp.Llama` with `create_embedding=False`, `tokenize` on raw text — matches the GGUF's own tokenizer exactly. No vocab file needed.
  - Fallback: `sentencepiece.SentencePieceProcessor` loaded from the Gemma 4 HF tokenizer.model (at `/home/govan/tmp/gemma-4-E2B-it/tokenizer.model` per CLAUDE.md).
- [ ] **Step 167** [B]: `test -f /home/govan/tmp/gemma-4-E2B-it/tokenizer.model && echo YES || echo NO`
- [ ] **Step 168** [V]: record availability.

### Implementation

- [ ] **Step 169** [W]: create `scripts/tokenize-kingdom-for-gemma4.py` that:
  1. Loads tokenizer (llama_cpp path preferred per Step 166).
  2. Reads Mistral-formatted JSONL from stdin (`{"text": "<s>[INST] ...[/INST] ..."}`).
  3. STRIPS the Mistral chat template markers and replaces them with Gemma 4 equivalents (`<bos>...<start_of_turn>user ... <end_of_turn>\n<start_of_turn>model ...<end_of_turn>` per Gemma 4 chat format).
  4. Tokenizes, emits `{"tokens":[int, ...], "answer_start": N}` where N is the token index where the model's response begins.
  5. Truncates sequences > 512 tokens (too long for our 2s/step budget at current seq length).
- [ ] **Step 170** [B] ~10s: smoke-test on 5 examples:
  ```bash
  head -5 /home/govan/tmp/unheaded/raft/training/train.jsonl | \
    python3 scripts/tokenize-kingdom-for-gemma4.py > /tmp/24h-tok-smoke.jsonl
  ```
- [ ] **Step 171** [V]: output has 5 lines, each parses as JSON with integer tokens and plausible answer_start.
- [ ] **Step 172** [B]: spot-check with `head -1 /tmp/24h-tok-smoke.jsonl | python3 -c "import json,sys;d=json.loads(sys.stdin.read());print(len(d['tokens']),'tokens, answer_start=',d['answer_start'])"`

### Full corpus pre-tokenization

- [ ] **Step 173** [B] ~2-3min: tokenize full train set:
  ```bash
  python3 scripts/tokenize-kingdom-for-gemma4.py < raft/training/train.jsonl > /tmp/24h-kingdom-train.jsonl
  python3 scripts/tokenize-kingdom-for-gemma4.py < raft/training/eval.jsonl  > /tmp/24h-kingdom-eval.jsonl
  wc -l /tmp/24h-kingdom-*.jsonl
  ```
- [ ] **Step 174** [V]: line counts match source (3568, 397).
- [ ] **Step 175** [B]: token length distribution:
  ```bash
  python3 -c "import json; lens=[len(json.loads(l)['tokens']) for l in open('/tmp/24h-kingdom-train.jsonl')]; lens.sort(); print('p50:',lens[len(lens)//2], 'p95:',lens[int(len(lens)*0.95)], 'max:',max(lens))"
  ```
- [ ] **Step 176** [V]: p95 ≤ 512 (truncation rate reasonable); max sensible.

### Handoff

- [ ] **Step 177** [W]: `notes/wave10f-tokenizer-slice.md` — document:
  - Backend chosen (llama_cpp vs sentencepiece)
  - Mistral → Gemma 4 chat template remapping
  - Truncation policy
  - Output JSONL schema
  - **Known limits**: no streaming, no detokenization, no special-token injection — full Rust integration is the long-term answer.
- [ ] **Step 178** [C]: commit `[PLAN 24h] Phase 4: Kingdom tokenizer vertical slice`.

- [ ] **Step 179** [V]: **PHASE 4 EXIT GATE**:
  - `/tmp/24h-kingdom-{train,eval}.jsonl` exist and parse.
  - Script checked into `scripts/`.
  - Notes doc persisted.
  - No green test went red (nothing in Rust changed).

**Steps 180-230 reserved** for debug (chat template mismatches, truncation regressions, vocab collisions), alternative tokenizer paths, fallback to sentencepiece if llama_cpp fails mid-way.

---

## PHASE 5: KINGDOM QA RAFT KICKOFF (Steps 231-290) — 8 h

**Goal:** Run the FIRST real-data LoRA training on forge's GPU path. Evidence of learning on genuine text + proof the 24h block delivers end-to-end signal.
**Prerequisite:** Phase 4 produced `/tmp/24h-kingdom-{train,eval}.jsonl`.
**ABORT-IF:** tokenized corpus absent → [STUCK] Phase 5 entirely, redirect 8 h to Phase 2/3 extensions + earlier Phase 6/7.

### Preflight

- [ ] **Step 231** [B]: subsample for a ≤10-minute smoke test:
  ```bash
  head -64 /tmp/24h-kingdom-train.jsonl > /tmp/24h-raft-smoke-train.jsonl
  head -32 /tmp/24h-kingdom-eval.jsonl  > /tmp/24h-raft-smoke-eval.jsonl
  ```
- [ ] **Step 232** [B] [GPU:t=3-5min]: smoke-run the CLI:
  ```bash
  ~/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge \
    train-gemma4 --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-raft-smoke-train.jsonl \
    --steps 10 --lr <OPTIMAL_FROM_PHASE_2> \
    --output /tmp/24h-raft-smoke.zlg4 2>&1 | tee /tmp/24h-raft-smoke.log
  ```
- [ ] **Step 233** [V]: loss descends over 10 steps; no NaN; output `.zlg4` exists.

### Full-corpus training script

- [ ] **Step 234** [CODE]: in `src/eval.rs` (or new `src/raft_eval.rs`), add a real-data EvalHarness constructor `from_jsonl(train_path, eval_path, answer_start_field) -> Result<Self>` that reads the tokenized JSONL. (Reuse existing `main.rs::cmd_train_gemma4` data-parsing logic; factor it.)
- [ ] **Step 235** [CODE]: add `test_raft_kingdom_smoke` [ignore] that:
  - Loads the smoke corpus.
  - Runs `EvalHarness::run(n_steps=20, eval_every=10, lr=optimal)`.
  - Asserts eval descends by ≥5% over 20 steps (loose — real text is harder than synthetic Y).
  - Logs full trajectory.
- [ ] **Step 236** [BUILD] + [TEST] [GPU:t=10min]: run smoke.
- [ ] **Step 237** [V]: eval descent ≥5%. If NOT, debug before committing to long training.
- [ ] **Step 238** [C]: commit `[PLAN 24h] Phase 5: RAFT smoke green`.

### Long training run

- [ ] **Step 239** [DECIDE]: budget for Phase 5 long-run training. Remaining GPU time after smoke ≈ 6-7 h. At 2s/step, that's ~11-12k steps. With 3568 training examples and 1 epoch ≈ 7136s (about 2 h) per epoch. **RECOMMENDATION**: 2 epochs (≈4 h), save LoRA every epoch. Leaves ~2 h for Phase 6.
- [ ] **Step 240** [B]: launch training (in foreground; logs to file):
  ```bash
  ~/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge \
    train-gemma4 --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-kingdom-train.jsonl \
    --steps 7136 --lr <OPTIMAL> \
    --save-every 3568 \
    --output /tmp/24h-raft-full.zlg4 \
    2>&1 | tee /tmp/24h-raft-full.log
  ```
  **[GPU:t=4h budgeted]**
- [ ] **Step 241** [V, mid-run, every ~30 min]: tail the log, confirm loss is still descending and no NaN. If NaN at step N, kill with Ctrl-C, inspect, reduce lr by 3×, restart from step 0. Record in session log.

### Eval on held-out

- [ ] **Step 242** [CODE]: standalone eval script `src/bin/eval_raft.rs` (or test `test_raft_eval_loaded_lora`) that loads a saved `.zlg4` + runs eval on `/tmp/24h-kingdom-eval.jsonl`, reporting mean loss + top-1.
- [ ] **Step 243** [BUILD].
- [ ] **Step 244** [TEST] [GPU:t=5min]: run eval on checkpoint-1-epoch and final checkpoint, compare.
- [ ] **Step 245** [V]: final checkpoint eval < epoch-1 eval (continued learning) AND final < base-model eval (net positive fine-tune).
- [ ] **Step 246** [W]: `notes/wave10f-first-raft-run.md` — record hyperparameters, step count, final metrics, checkpoint paths, known issues.

### Handoff

- [ ] **Step 247** [C]: commit `[PLAN 24h] Phase 5: first Kingdom-QA RAFT LoRA trained`.
- [ ] **Step 248** [V]: **PHASE 5 EXIT GATE**:
  - Final `.zlg4` exists (~22 MB).
  - Eval descended vs base model.
  - Trajectory log persisted.
  - No green test went red.

**Steps 249-290 reserved** for debug (NaN recovery, OOM handling, tokenizer bugs exposed by edge cases in the full corpus, chat template issues), mid-run lr adjustments, checkpoint diff inspection.

---

## PHASE 6: REGRESSION + TIMING AUDIT (Steps 291-320) — 2 h

**Goal:** Re-run the full Learning Gate and verify everything we added didn't break anything. Capture new timing baselines.
**Prerequisite:** Phases 1-5 complete or cleanly aborted.
**ABORT-IF:** ANY pre-existing Learning Gate test (1, 3, 4, 5) that was passing in Phase 0 now fails → HALT, bisect, revert, escalate.

### Regression run

- [ ] **Step 291** [TEST] [GPU:t=3min]: `test_gemma4_backward_grad_health` — grad health baseline.
- [ ] **Step 292** [V]: `healthy=35/35 zero=0 nan=0`.
- [ ] **Step 293** [TEST] [GPU:t=5min]: `test_gemma4_gpu_train_step_loss_descent`.
- [ ] **Step 294** [V]: warm step ≤ 3s.
- [ ] **Step 295** [TEST] [GPU:t=5min]: `test_learning_exp1_held_out_eval` original.
- [ ] **Step 296** [V]: ratio ≤ 0.90, CI excludes 1.0.
- [ ] **Step 297** [TEST] [GPU:t=3min]: `test_learning_exp3_lora_zero_identity`.
- [ ] **Step 298** [V]: identity + post-train descent.
- [ ] **Step 299** [TEST] [GPU:t=6min]: `test_learning_exp4_dataset_size_scaling`.
- [ ] **Step 300** [V]: eval(|T|=64) < eval(|T|=8) by ≥2%.
- [ ] **Step 301** [TEST] [GPU:t=6min]: `test_learning_exp5_generalization_gap_beta`.
- [ ] **Step 302** [V]: β CI upper < 0.8.
- [ ] **Step 303** [TEST] [GPU:t=5min]: Exp 2 at final form (from Phase 1).
- [ ] **Step 304** [V]: passes or cleanly reports.

### Timing audit

- [ ] **Step 305** [B]: record final timings.
  ```bash
  grep -h "step_time\|/step" /tmp/24h-*.log | sort -u
  ```
- [ ] **Step 306** [W]: update `notes/wave10f-step-c-timing.md` with 24h-block timings:
  - warm step (should still be ~2s, but log any drift)
  - Exp 1-5 wall-clocks (post-Phase 2 optimal lr)
  - Phase 5 full-corpus training total
  - Token throughput for Phase 5 (tokens/s, sequences/s)

### Regression exit

- [ ] **Step 307** [TEST] [GPU:t=10min]: full crate sweep.
  ```bash
  cargo test --release --bin zhenai-forge -- --test-threads=1 2>&1 | tail -15
  ```
- [ ] **Step 308** [V]: no unexpected failures. Any pre-existing `#[ignore]` remains ignored.
- [ ] **Step 309** [C]: commit `[PLAN 24h] Phase 6: regression audit green, timing recorded`.
- [ ] **Step 310** [V]: **PHASE 6 EXIT GATE**:
  - All 5 Learning Gate tests pass.
  - Core grad-health + loss-descent pass.
  - Timing notes updated.

**Steps 311-320 reserved** for debug (bisecting a regression with `git bisect` between C1 and head, rollback procedures).

---

## PHASE 7: DOCS + SESSION HANDOFF (Steps 321-360) — 1.5 h

**Goal:** Persist everything to repo, update CLAUDE.md, write the session log, and leave a crisp next-session prompt.
**Prerequisite:** Phase 6 green (or at least documented).
**ABORT-IF:** This phase MUST run — even if Phases 1-5 had partial aborts. Documentation ALWAYS lands.

- [ ] **Step 321** [W]: `crates/zhenai-forge/notes/wave10f-24h-session-2026-04-20.md` — full session report:
  - Objective / what we set out to do.
  - Per-phase outcome (PASS/DIAG/STUCK).
  - All numeric metrics in tables.
  - Commits landed (hash + subject).
  - Known failures at end (empty or enumerated).
  - Follow-ups / TODO for next session.
- [ ] **Step 322** [W]: update `notes/wave10f-learning-gate-plan.md` scorecard if Exp 2 was promoted.
- [ ] **Step 323** [W]: update top-level `CLAUDE.md` WAVE10F paragraph with:
  - Exp 2 status (PASS / DIAG with reason).
  - First RAFT LoRA trained? (yes/no + checkpoint path).
  - Optimal lr for future runs.
- [ ] **Step 324** [W]: update `notes/wave10f-step-c-battle-plan.md` STEP C DONE banner if any follow-up commits modify Step C code paths (unlikely; skip if n/a).
- [ ] **Step 325** [W]: auto-memory — add `project_wave10f_24h_session.md` with:
  - Date, scope, outcome.
  - Lessons re: lr, plateau training, tokenizer path.
  - Followup pointer to `wave10f-24h-session-2026-04-20.md`.
- [ ] **Step 326** [B]: `cd ~/tmp/unheaded && git status` — confirm only expected files changed.
- [ ] **Step 327** [C]: commit `[PLAN 24h] Phase 7: session log + CLAUDE.md + memory`.

### Next-session prompt

- [ ] **Step 328** [W]: in the session report, include this handoff prompt block (adjust for actual outcomes):

  ```
  WAVE10F after the 24h consolidation block. Current state:
  - Exp 2 status: [PASS at plateau lr=1e-2 | DIAG, implicit-reg ceiling documented]
  - Optimal lr for Gemma 4 forge training: [X]
  - Multi-Y capacity: [N disjoint maps learnable, per-Y ratios in notes/multi-y]
  - First RAFT LoRA: [checkpoint path] — [base eval X → finetuned eval Y]
  - Tokenizer: [Python wrapper at scripts/tokenize-kingdom-for-gemma4.py]

  Open questions:
  - [per-phase: list the top 1-2 per aborted/incomplete phase]

  Next likely sprint: [Phase 7.1 full Rust SentencePiece | Phase 7.3 LoRA A/B eval | ...]
  ```

### Final audit

- [ ] **Step 329** [B]: `git log --oneline <first-plan-commit>..HEAD` — full commit chain for the 24h.
- [ ] **Step 330** [V]: every commit prefix is `[PLAN 24h]` and every phase exit was committed.
- [ ] **Step 331** [B]: `git status` — tree clean.
- [ ] **Step 332** [V]: **24-HOUR EXIT GATE**:
  1. Exp 2 resolved (pass OR honest diag, either way documented).
  2. Optimal lr published in notes.
  3. Multi-Y capacity evidence (pass OR documented capacity ceiling).
  4. Tokenizer vertical slice script checked in.
  5. First RAFT LoRA trained OR documented reason we couldn't.
  6. All pre-existing Learning Gate tests still green.
  7. Session log + CLAUDE.md updated + memory entry written.
  8. Every `/tmp/24h-*` log file referenced in docs has a sibling in `notes/` OR is copied into the session log.
  9. Tree clean, commits chain unbroken.

**Steps 333-360 reserved** for last-mile doc polish, generating summary tables, screenshots of terminal output if useful, handoff prompt testing.

---

## APPENDIX A: EMERGENCY PROCEDURES

### A.1 NaN train loss mid-run
1. Stop the training loop (Ctrl-C or wait for step).
2. Record which step, which regime (real/scrambled).
3. Halve lr, rerun from step 0 (not a resume — fresh lora).
4. If recurs at halved lr → bisect to find dangerous lr.
5. If even lr=1e-4 NaNs → escalate; suspect shape/dispatch bug.

### A.2 GPU OOM
1. Confirm via `rocm-smi` / `nvidia-smi` — is another process holding VRAM?
2. Reduce seq_len (truncate harder in tokenizer).
3. Reduce n_eval (we only need ≥32 for stats).
4. If still OOM with minimum budget → escalate; likely forge-internal leak.

### A.3 Test went red that was green in Phase 0
1. `git bisect` between Phase 0 commit and HEAD.
2. Don't `git revert HEAD` blindly — identify the offending commit.
3. Write a minimal repro test for the regression before fixing.

### A.4 Tokenizer produces junk
1. Compare token IDs against `llama.cpp tokenize` CLI directly — is the wrapper consistent?
2. Check for BPE merges / special-token injection differences.
3. If wrapper is wrong, re-run with `sentencepiece` backend (Step 166 fallback).

### A.5 Exp 2 STILL doesn't discriminate after lr=3e-2, max_steps=400
1. This is a scientific finding, not a test failure.
2. Document precisely in session log AND in `notes/wave10f-learning-gate-plan.md`.
3. Mark Exp 2 as "defer to Phase 7.2 real-data regime" with Scientist's rationale.
4. Do NOT block Phase 2-7.

---

## APPENDIX B: CRITICAL PATH

```
Phase 0 (30m) → Phase 1 (1h) → Phase 2 (3h) → Phase 3 (2h) → Phase 4 (6h) → Phase 5 (8h) → Phase 6 (2h) → Phase 7 (1.5h)
```

If Phase 4 ABORTS (no tokenizer), Phase 5 becomes (longer runs of Exp 1/4 + Exp 6 multi-Y variants), and the 8-hour Phase 5 budget reallocates to:
  - +2h to Phase 2 (rank × lr joint sweep)
  - +2h to Phase 3 (5-Y and 10-Y stress tests)
  - +1h to Phase 6 (more exhaustive regression)
  - +3h to Phase 7 (deeper docs + a diagnostic `notes/` post explaining why RAFT bailed)

If Phase 1 Exp 2 resolves as clean PASS in ≤30 min, the 30 remaining minutes roll into Phase 2's rank-sweep optional step 100.

---

## APPENDIX C: QUICK REFERENCE

### Key file paths
- Repo root: `/home/govan/tmp/unheaded`
- Crate: `crates/zhenai-forge`
- Harness: `src/eval.rs`
- Stats: `src/eval_stats.rs`
- Plan doc: `notes/wave10f-learning-gate-plan.md`
- 24h session log: `notes/wave10f-24h-session-2026-04-20.md` (Phase 7)
- GGUF: `/var/zhen/models/gemma-4-E2B-it.gguf`
- Kingdom corpus: `raft/training/{train,eval}.jsonl`

### Key cargo invocations
```bash
cd ~/tmp/unheaded/crates/zhenai-forge

# Unit tests only (fast, no GGUF)
cargo test --release --bin zhenai-forge eval_stats:: eval::tests::test_synthetic

# Single ignored integration test
cargo test --release --bin zhenai-forge <test_name> -- --ignored --test-threads=1 --nocapture

# Full ignored suite (all Learning Gate)
cargo test --release --bin zhenai-forge test_learning_ -- --ignored --test-threads=1 --nocapture
```

### Expected warm GPU step
- ~2.0s / step steady-state after ~22s cold upload at test start.
- Eval forward per sequence: ~0.3s.

### Kingdom corpus schema (AFTER Phase 4 tokenization)
```json
{"tokens": [2, 105, 2364, ...], "answer_start": 87}
```

---

*WAVE10F 24-Hour Battle Plan — Forged 2026-04-20*
*7 Phases. 360 step slots. From Exp 2 resurrection to first real RAFT LoRA.*
*Plateau the noise. Sweep the lr. Stack the Ys. Tokenize the Kingdom. Ship the LoRA.*
