# WAVE10F 24-Hour Session — 2026-04-20

**Plan executed:** `crates/zhenai-forge/notes/wave10f-24h-battle-plan.md`
**Enforcement:** `unheaded-marshal` on duty; Stevie AFK.
**Status:** IN PROGRESS (this doc is live-updated; final Phase 7 commit
finalizes).

---

## Overall phase outcome

| Phase | Name | Outcome | Wall-clock |
|------:|------|---------|-----------:|
| 0 | Recon + preflight | ✅ PASS | 25 min |
| 1 | Exp 2 hard gate (plateau) | 🔍 DIAG (log(vocab) floor finding) | ~60 min |
| 2 | Exp 1 extended + lr sweep | MIXED (extended FAIL, sweep PASS — lr=1e-3 adopted) | ~65 min |
| 3 | Multi-Y capacity probe | ✅ 2-Y PASS (both groups ratio ~0.6); 3-Y skipped | ~80 min |
| 4 | Tokenizer vertical slice | ✅ PASS (pipelined while Exp 2 ran) | ~15 min |
| 5 | Kingdom QA RAFT kickoff | ❌ ABORT (CPU-bound attention at seq=384) | ~65 min wasted |
| 6 | Regression audit | ✅ ALL 6 TESTS GREEN (no regressions) | ~30 min |
| 7 | Docs + handoff | ✅ DONE (this doc + plan docs + memory) | ~20 min |

Session wall-clock: ~6 h of the 24 h budget. Aborted early at Phase 5
because the forward-path CPU bottleneck at long sequences was
out-of-scope to fix in this session. All scientific learning signals
held; the forge can learn, we just can't efficiently train it on
400-token real text yet.

---

## Phase 0 — Recon + preflight (DONE)

- HEAD: `072d4b50` (24h battle plan commit)
- GGUF present: 8.7 GB
- MemAvailable: 10.2 GB
- Tokenizer: `gemma4-venv` + `transformers 5.5.1` + HF tokenizer.json chosen
- Kingdom corpus: 3568 train + 397 eval sequences (Mistral chat format)
- Baseline unit tests: 9/9 eval_stats + 4/4 eval green

## Phase 1 — Exp 2 Hard Gate Resurrection (DIAG, honest outcome)

Hard-gate promotion attempted with plateau-based training at lr=1e-2
(max_steps=200, K=10, eps=0.1) per the Scientist's recommendation.

### Result: falsifiable prediction REJECTED

|Check|Predicted|Actual|
|---|---|---|
|scr_train < 2.0|true|12.46 (false)|
|scr_eval > 0.7×base (15.03)|true|12.52 (false)|
|\|real_train − real_eval\| < 1.0|true|11.72 (false)|

All three predictions violated. But the NATURE of the violation is
scientifically important:

- **scrambled_train plateaued at 12.47 ≈ log(262144) = 12.48.** This
  is the information-theoretic CE floor for uniform-random labels over
  a 262k vocabulary. No model can descend below this — the prediction
  `scr_train → 0` was physically impossible on this synthetic corpus.
- **real-Y train oscillated 7.9–13.8** — lr=1e-2 is too aggressive.
  Trajectory samples (step 1/21/41/.../200): 21.5→13.8→11.0→11.0→10.8
  →11.7→9.7→10.3→7.9→10.8→10.4. Classic "lr too high" pattern.
- **scrambled_eval dropped to 12.52** — the model's output distribution
  converged to the same log(vocab) uniform floor on eval too, because
  that's the information-theoretic lower bound for any model trained
  on uniform labels. Not a generalization signal; an artifact of
  labels carrying zero information.

### Resolution

Exp 2 stays DIAGNOSTIC. The hard-gate target is unreachable on this
synthetic corpus by construction. The real negative control for
memorization will come from Phase 7.2 — real-data RAFT on Kingdom
text, where non-uniform natural-language distributions make train → 0
memorization achievable in principle while eval stays high.

Full 5-iteration history: `/tmp/24h-exp2-outcome.md`.

## Phase 2 — Exp 1 Extended + LR Sweep (MIXED — extended FAIL, sweep PASS)

`test_learning_exp1_extended` **FAILED** at lr=3e-3 (original Learning
Gate lr) with 100 training steps: training destabilizes after step ~30,
eval bounces back to baseline by step 100 (ratio 1.066). Classic "lr too
high for long training" signature.

`test_learning_exp1_lr_sweep` **PASSED**: 3 of 4 lrs below the 0.90 gate:

| lr   | ratio | CI95           | Status |
|------|------:|----------------|--------|
| 1e-3 | 0.700 | (0.664, 0.739) | ✅ BEST |
| 3e-3 | 1.066 | (1.042, 1.096) | ❌ cliff edge |
| 1e-2 | 0.779 | (0.740, 0.819) | ✅ |
| 3e-2 | 0.762 | (0.715, 0.812) | ✅ |

**Verdict: adopt lr=1e-3 for long training runs (Phase 5 RAFT)**.
Original 20-step Learning Gate used lr=3e-3 which works only for short
training.

Full analysis: `notes/wave10f-lr-sweep.md`.
Runtime: Exp 1 extended ~13 min, lr sweep ~49 min = ~1 h 2 min total.

## Phase 3 — Multi-Y Capacity Probe (2-Y PASS, 3-Y skipped)

**`test_learning_exp6_multi_y_capacity` PASSED** on the 2-Y gate after
4396s (73 min) — much slower than the 8-min estimate. Both groups
descend cleanly:

| Group | Base | Final | Ratio | Status |
|------:|-----:|------:|------:|:------:|
| 0 | 21.95 | 12.52 | **0.570** | ✅ |
| 1 | 20.77 | 12.55 | **0.604** | ✅ |

Gate: ratio ≤ 0.85 per group. Both passed with margin.

**Key finding:** forge learns two disjoint Y maps in parallel with
rank-16 LoRA. Capacity split across groups doesn't collapse either —
each group descends to roughly the single-Y baseline.

**3-Y informational stress test SKIPPED** per Skip Protocol: at 2×
the 2-Y runtime (~90 min), it would push Phase 3 past 3× estimate
and delay Phase 5. The 2-Y result is the hard gate; 3-Y was
informational only.

**Unexpected timing:** 2-Y ran 73 min instead of 8 min estimate
(~9× overshoot). Each training step appears to have taken ~44 s
instead of the expected 2 s. Hypothesis: mixed-sequence cycling
through 64 examples plus expensive per-group eval harness overhead
produced GPU-buffer allocation storms. Not investigated further;
logged as a follow-up.

## Phase 4 — Tokenizer Vertical Slice (DONE, pipelined while GPU busy)

- `scripts/tokenize-kingdom-for-gemma4.py` landed at commit `<hash>`
- Full Kingdom corpus pre-tokenized to `/tmp/24h-kingdom-{train,eval}.jsonl`
  (3568 / 397 sequences, p50 seq=384, p50 answer=48)
- Python venv `/home/govan/tmp/gemma4-venv` + `transformers.AutoTokenizer`
  avoids PEP 668 pip restriction
- Notes: `notes/wave10f-tokenizer-slice.md`

## Phase 5 — Kingdom QA RAFT Kickoff (ABORTED — CPU-bound attention at seq=384)

**RAFT smoke on real Kingdom text ran for 64 min without emitting any
training-loss output. Killed to free GPU for Phase 6.**

### Diagnosis

During the stall, `rocm-smi` reported **GPU% = 0**, while the
`zhenai_forge` process held 90.9% on a single CPU core. The forge
"GPU" forward path offloads only matmuls to hipBLAS; attention
softmax, RoPE, RMSNorm, GELU, and Adam all run single-threaded on
the CPU. At seq=384 (vs synthetic's seq=12), attention alone is:

    n_layers × n_heads × seq² × head_dim
  = 35 × 8 × 384² × 256
  ≈ 10.5 G ops per forward pass

Single-threaded CPU (≈1-3 GFLOP sustained Rust, no SIMD) gives ~5-10s
per forward × 16 baseline-eval sequences ≈ 80-160s just for baseline
eval. Training-step forward+backward is ~2× that. 20 training steps +
3 evals predicted ~20-40 min. Observed: >60 min with zero output
visible, suggesting additional pathological overhead (possibly
hipBLAS kernel-compilation cost on first sgemm at seq=384, or
VRAM/RAM thrashing — 9 GB zhenai_forge RSS + 4.57 GB GPU weights is
tight against west's 12.87 GB VRAM + 14 GB RAM).

### Why this surprised us

The 24h battle plan assumed real-text sequences at the same ~2 s/step
as synthetic-Y (seq=12). That assumption was wrong:

- **Attention scales as O(seq²)** on CPU, so seq=384 is 1024× worse
  than seq=12 for the attention block specifically.
- **Non-matmul CPU work wasn't tracked** in the Phase 7 Step C timing
  notes — those measured matmul speedup, not end-to-end long-seq
  latency.
- **Exp 6 multi-Y already hinted at the issue:** each step took ~44 s
  instead of ~2 s, a 22× overshoot; real-text seq=384 is the same
  problem at greater magnitude.

### Resolution

Phase 5 DEFERRED. This is not forge's "learning works" question — that
was answered by Exp 1/3/4/5/6. This is a "forge's forward-path
throughput" question:

- **Fix requires:** moving attention/softmax/RoPE onto the GPU (WAVE11?
  HIP kernels or Aya/Triton), OR using a much shorter sequence length
  (truncate Kingdom prompts more aggressively), OR a different training
  regime (batched single-token positions rather than full-prompt CE).
- **Honest 24h outcome:** the Python-side tokenizer slice landed
  (Phase 4), the tokenized Kingdom corpus is persisted at
  `/tmp/24h-kingdom-{train,eval}.jsonl`, but the CLI path to feed it
  through forge is only viable for sequences ≤64 tokens or so
  without further kernel work.

Per Appendix B reallocation, the 8 h Phase 5 budget that won't be
used is being redirected to Phase 6 regression (deeper sweep) and
Phase 7 (more thorough docs).

## Phase 6 — Regression + Timing Audit (ALL GREEN)

Ran the full Learning Gate + core GPU tests post-session. **Zero
regressions.** Details in `notes/wave10f-24h-regression.md`:

| Test | Result | Metric |
|------|:-----:|--------|
| `test_gemma4_backward_grad_health` | ✅ | healthy=35/35 |
| `test_gemma4_gpu_train_step_loss_descent` | ✅ | 1.97 s/step, loss 19.96→5.70 |
| `test_learning_exp1_held_out_eval` | ✅ | ratio 0.58 CI (0.567, 0.596) |
| `test_learning_exp3_lora_zero_identity` | ✅ | A=0 ≡ base |
| `test_learning_exp4_dataset_size_scaling` | ✅ | \|T\|=64 vs 8 = 37% |
| `test_learning_exp5_generalization_gap_beta` | ✅ | β=0.266 CI (−0.11, 0.64) |

GPU warm-step unchanged at 1.97 s (vs 2.05 s baseline, ±5% noise).
Loss trajectories bit-identical to prior runs. The 24h session's
additive code changes did not touch core forward/backward/Adam.

Audit-script bug (misused `--ignored` flag on non-ignored tests)
caught and documented for next session.

## Phase 7 — Docs + Session Handoff (DONE)

- This document.
- `notes/wave10f-learning-gate-plan.md` updated with iter-5 finding.
- `/tmp/24h-exp2-outcome.md` persisted as full Exp 2 record.
- `/tmp/24h-regression-audit.sh` script staged for Phase 6.
- CLAUDE.md update pending Phase 5 results.

## Commit chain (this session)

```
072d4b50  WAVE10F 24-hour battle plan
2b911ce7  run_until_plateau added
93d5a761  Exp 2 rewritten — plateau-based hard gate
4edfdf37  pipelined Phase 2/3/5 code while Exp 2 runs
25dc0d08  Kingdom tokenizer vertical slice (Phase 4)
22258105  Phase 1 outcome + Phase 7 session log scaffold
28e49674  Phase 2 complete — lr=1e-3 is the stable operating point
1e43a20e  Phase 3 — Exp 6 multi-Y 2-Y PASS; 3-Y skipped (Skip Protocol)
ed92ca9a  Phase 5 ABORT — forge forward is CPU-bound at seq=384
<next>     Phase 6 regression audit green; Phase 7 handoff
```

## Next-session prompt

```
WAVE10F post-24h-block. Ship state (2026-04-20):

SCIENTIFIC RESULTS:
  - Exp 2: DIAG. Scrambled-labels train_loss plateaus at log(262144)
    ≈ 12.48 — the information-theoretic CE floor for uniform labels
    over a vocab that size. Hard-gate impossible on synthetic noise
    by construction. Real negative control still pending Phase 7.2.
  - Optimal lr: 1e-3 at plateau over 100 steps on the synthetic Y
    task. lr=3e-3 IS a cliff edge — works for 20 steps, blows up at
    100 (eval reverts to baseline). Adopt 1e-3 for anything long.
  - Multi-Y capacity: 2 disjoint Y maps at rank=16 → both groups'
    eval ratio < 0.61 after 100 steps. Attention CAN partition.
  - 24h regression: ALL SIX Learning Gate + core tests GREEN
    (grad_health, loss_descent, exp1/3/4/5). Zero regressions.

ENGINEERING BLOCKER:
  - Forge's 'GPU forward' only accelerates matmuls; attention,
    softmax, RoPE, norms stay CPU. At seq=384 (real-text scale),
    single-threaded CPU attention is O(seq²) and swamps everything.
    RAFT smoke stalled 64 min with zero training output. GPU% = 0
    during stall. FIX needed before Phase 5 real-data training is
    viable.

ARTIFACTS PERSISTED:
  - scripts/tokenize-kingdom-for-gemma4.py    (Gemma 4 chat tokenizer
    wrapper via transformers.AutoTokenizer + HF tokenizer.json)
  - /tmp/24h-kingdom-{train,eval}.jsonl        (3568 + 397 tokenized
    sequences at MAX_TOKENS=384)
  - notes/wave10f-24h-battle-plan.md            (the plan executed)
  - notes/wave10f-24h-session-2026-04-20.md     (this doc)
  - notes/wave10f-lr-sweep.md                   (lr=1e-3 finding)
  - notes/wave10f-24h-regression.md             (no-regression proof)
  - notes/wave10f-tokenizer-slice.md            (tokenizer docs)
  - /tmp/24h-exp2-outcome.md                    (5-iter Exp 2 record)

NEXT SPRINT CANDIDATES (in descending value):

  (A) GPU attention kernels — move softmax + RoPE + attention scores
      to hipBLAS/HIP/Aya-kernels. Biggest leverage: unlocks long-seq
      training and Phase 5.
  (B) Reduce Kingdom prompts to ≤64 tokens (aggressive context trim)
      and retry RAFT. Easier/faster; gets us a real LoRA this week.
  (C) Per-token batching instead of full-prompt CE — shorten effective
      seq per gradient step.
  (D) Phase 7.1 full Rust SentencePiece tokenizer — lowest priority
      since Python wrapper works for the corpus we have.

Recommendation for next session: (B) first (validate real-learning at
seq≤64 with lr=1e-3), then (A) in parallel for the long-term solution.
```
