# Session — 2026-04-29 / 30 — WAVE14 H6 root cause + parser fix + Run B

**Status:** Track 1 verdict reached. Pivot decision pending Stevie's call (Path A / B / C, see end).
**Commits this session:** 14 (008727c5 → present, all on `main`).

---

## TL;DR (3 sentences)

A JSONL line-parser bug in `cmd_train_gemma4` had been silently absorbing the JSON `answer_start` field as a stray training token for 11 days across WAVE10F, WAVE12, and WAVE14 Run A. We landed Phase 1-3 of the surgical fix (`serde_json::from_str` parser + per-example `answer_start` plumbing through both train and eval), shipped a regression test that catches the bug 10/10 records, hardened the test runner against the OOM event the original investigation triggered, and ran Run B on the corrected pipeline. **G-CE Δ −13.71 PASSED** the pre-registered threshold but **all behavioral gates failed** — the LoRA collapsed to repeating `\n` (token 107). Same Pyrrhic-CE failure mode as WAVE12 and Run A, just on a different attractor. Verdict: per-token-frequency-fit local optimum, not generation skill.

---

## Phases shipped

| # | Description | Commit |
|--:|---|---|
| **WAVE14 P1** | `main.rs` parser → `serde_json::from_str` + per-example `answer_start` plumbing | `008727c5` |
| **WAVE14 P2** | `eval.rs` parser migration + `EvalHarness::train_answer_starts` field + per-example wiring in `run_with_backend` and `run_until_plateau_with_backend` | `57f9820b` |
| **WAVE14 P3** | Regression test `test_load_tokenized_jsonl_real_corpus` — gated on corpus presence, asserts `tokens.len()==384`, `answer_start in [0,len)`, `tokens.last() != answer_start`. Proven to fail 10/10 records on pre-fix parser via Python simulation | `b9045605` |
| Doc correction | east/west reality (east is *smaller*, not bigger — 8 GB vs 14 GB) | `33c4f69f` |
| **T1.1 H7 fix** | `cmd_eval_gemma4` had the same naive parser; migrated to `serde_json` so eval pipeline reads correct data. BlackMage's H7 prediction caught and fixed before T1.4 | `b221ce0d` |
| **T1.2** | Run B pre-registration — frozen quality gates (G-CE Δ≥8.0, G-OPEN ≥3/8 corpus openers, G-TOPIC ≥1/8 Kingdom allowlist, G-NO-COLLAPSE 0/8 Run-A-style `\t}\Question:`) | `ab30a939` |
| Wrapper fix | `scripts/forge-train.sh` was hardcoding `train` as the binary subcommand; broke `train-gemma4`/`eval-gemma4`. Now forwards `$@` verbatim | `efd5b999` |
| T1.4 scorer | `scripts/wave14-score-runB.py` — mechanical pass/fail per pre-reg gates | `22a659dc` |
| T1.5 scaffold | `scripts/wave14-rag-baseline.py` — RAG control arm scorer (executes after Run B, never in parallel) | `73fa4150` |
| **Test-runner hardening** | 18 GGUF-loading tests `#[ignore]`'d; `crates/zhenai-forge/.cargo/config.toml` jobs=4; `notes/test-runner-hardening.md` documents the OOM event | `807daa6f` |
| Top-level cleanup | 12 stray docs/scripts moved into subdirs; root 38 → 25 files; orphan `wotan` binary deleted | `3e0c4266` |
| Recipe doc | `notes/wave14-runB-recipe.md` — copy-pasteable T1.4/T1.5 execution plan | `e908ea54` |
| Hardening note correction | east/west reality propagated into the hardening doc | `33c4f69f` (also) |
| **T1.4 verdict** | Run B FAIL — G-CE PASS Δ−13.71 but behavioral 0/3 honest. LoRA collapsed to 100% `\n` token | latest commit |

---

## What we proved

1. **The H6 parser bug is real and shipped 11 days unnoticed.** Audit showed 100% corruption rate on 50 sampled records; pre-fix parser added the JSON `answer_start` integer as a stray trailing token in every training example. WAVE10F + WAVE12 + Run A all trained on this corruption.
2. **The H6 fix is causal for CE.** Run B at step 200 with corrected parser reached Δ −13.71 — 96% of WAVE12's full 500-step descent at 40% of the training budget.
3. **CE descent ≠ generation skill.** Run B's LoRA collapses to repeating `\n` at greedy T=0; produces gibberish at T=0.7. The model fit the corpus's high-frequency tokens (within the loss-masked region) without learning contextual generation. Distinct from Run A's structural-anchor collapse, same outcome.
4. **The Scientist's pre-registration paid off.** G-CE alone would have falsely declared victory; the behavioral gates caught the underlying failure. The hardware also paid off — 14 GB box surviving training + eval validates the hardening sprint.

---

## What we caught en route

**The OOM that crashed tmux:** during early Phase 1 exit-gate testing, `cargo test --bin zhenai-forge --tests` OOM-killed the user session. Memory dump in dmesg showed a single test process at 11.5 GB RSS on a 14 GB host — multiple parallel test threads each loading the 9 GB Gemma-4 GGUF. Hardening landed (`#[ignore]` on 18 tests, `jobs=4`, `scripts/forge-train.sh` cgroup-capped at MemoryMax=10G). No further OOM after.

**H7 (BlackMage's prediction):** `cmd_eval_gemma4` had a parser identical in shape to pre-fix `cmd_train_gemma4`. Would have absorbed `answer_start` into eval sequences, producing untrustworthy CE numbers for Run B's verdict. Caught at T1.1 audit, fixed before any eval ran.

**The forge-train.sh subcommand bug:** the launcher hardcoded `train` as the binary's first arg (correct for WAVE10D Mistral path, wrong for WAVE10F+ Gemma-4 path). Run B v3 silently invoked Mistral's `cmd_train` with garbage args until the log showed `32000 vocab` (Mistral) instead of `262144` (Gemma-4). Fixed at `efd5b999`.

**The amdgpu queue-eviction kill:** Run B v5 reached step 246 before the AMD driver evicted the GPU queue (`Freeing queue vital buffer ... queue evicted` in dmesg). 4 LoRA checkpoints saved (50/100/150/200) thanks to `--save-every 50`.

**The empty-decode silent collapse:** Run B's first T1.4 generation pass produced `.txt` files showing only init logs and `--- generated 52 tokens ---`, no decoded content. Added a debug print of raw completion token IDs; revealed 100% `\n` collapse. The decode-via-gemma-venv script's `skip_special_tokens=True` had hidden the failure; `trim_end()` then stripped the surviving newlines to empty.

---

## Gates that mattered (and the one that didn't)

| Gate | Pre-registered | Run B actual | Honest |
|---|---|---|---|
| G-CE | Δ ≥ 8.0 | Δ −13.71 | ✅ PASS |
| G-OPEN | ≥3/8 corpus openers | 0/8 | ❌ FAIL |
| G-TOPIC | ≥1/8 Kingdom allowlist | 8/8 (scorer false-positive on init lines) → 0/8 honest | ❌ FAIL |
| G-NO-COLLAPSE | 0/8 Run-A `\t}\Question:` | 8/8 (gate didn't catch newline-collapse) → 0/8 honest | ❌ FAIL |

The pre-registered rule was *G-CE plus ≥2 behavioral = PASS*. After honest scorer corrections: 0/3 behavioral. **VERDICT: FAIL.**

The G-NO-COLLAPSE blind spot is a real lesson: a gate scoped to one specific attractor pattern can miss other collapse shapes. Future gate should detect any single-token-dominance ratio, not just the historical attractor.

---

## Why CE descended yet generation collapsed

My Phase 1 fix included a defensive clamp:

```rust
let effective_answer_start = example.answer_start
    .unwrap_or(cli_answer_start)
    .min(example.tokens.len() / 2);
```

For Kingdom corpus (`tokens.len()=384`, `answer_start≈350`), this clamps to `192`. Loss is then computed on positions [192..384] = 192 positions per example — covering the bottom half of the question + the answer. Within that window, the highest-frequency tokens are `\n`, ` the`, `,`. The LoRA found a degenerate optimum: predict the highest-frequency token at every position. CE drops because corpus CE is dominated by these.

**The clamp was a hedge from before the H6 fix landed.** With per-example `answer_start` correctly plumbed (verified: 3568/3568 records show their JSON value), the clamp now FORCES loss masking to the wrong region.

This is the recommended fix in Path C (below).

---

## Decision tree (Stevie's call)

Per `~/.claude/plans/synthetic-stirring-pudding.md` §3 Track 1 exit branch:

> *"G-CE PASS, <2 of 3 behavioral PASS → STOP. File postmortem. Decide whether to push to 3000-5000 steps OR pivot to coding-corpus per Track 2 directly."*

Three paths drafted in `notes/wave14-runB-results.md`:

- **Path A — Push to 3000-5000 steps.** Run A at 1500 corrupt steps already collapsed; high cost (~30 GPU-hr) + low confidence.
- **Path B — Pivot to Track 2 coding-corpus.** Real long-term goal. Multi-day investment.
- **Path C (recommended) — Remove the clamp first, run Run C @ 500 steps.** ~10 GPU-hr. If Run C generates clean text → clamp was the bug. If still collapsed → pivot to Track 2 with hard evidence.

---

## Repo state at session end

- 14 commits this session, all on `main`, all pushed to origin (per Stevie's intermittent SSH-key push).
- Top-level files: 25 (down from 38 at session start).
- New scripts: `scripts/wave14-score-runB.py`, `scripts/wave14-rag-baseline.py`.
- New artifacts: `raft/kingdom-w14b/kingdom.lora.gguf.checkpoint-{50,100,150,200}` (~22 MB each).
- New notes: 9 in `crates/zhenai-forge/notes/` (wave14-* + test-runner-hardening + recipe).
- Open: `raft/kingdom-w14a/` (Run A artifacts, 346 MB) and `raft/kingdom-w14b/` (Run B artifacts) — kept for forensic comparison.

---

## Reading order for next session

1. `~/.claude/plans/synthetic-stirring-pudding.md` — long-term plan and Track 1 exit branch
2. `crates/zhenai-forge/notes/wave14-runB-results.md` — Run B verdict (THIS is what determines next move)
3. `crates/zhenai-forge/notes/wave14-runB-pre-registration.md` — frozen gates from before observation
4. `crates/zhenai-forge/notes/wave14-h2h6-analysis.md` — root cause confirmation
5. `crates/zhenai-forge/notes/test-runner-hardening.md` — what NOT to break (cargo test = OOM)

— Librarian, 2026-04-30
