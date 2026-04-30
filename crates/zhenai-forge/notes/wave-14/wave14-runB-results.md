# WAVE14 Run B — T1.4 results & verdict

**Date:** 2026-04-30
**Checkpoint under test:** `raft/kingdom-w14b/kingdom.lora.gguf.checkpoint-200`
**Pre-registered gate:** `notes/wave14-runB-pre-registration.md` (frozen 2026-04-29 BEFORE Run B trained)
**Plan exit branch:** long-term plan §3 Track 1

---

## Headline verdict — **FAIL**

Run B **passes G-CE massively but fails all behavioral gates.**

This is exactly the failure mode the Scientist warned against at pre-registration: *"WAVE13 P2's lesson: behavioral probes gate the verdict, not CE descent. Loss-only success is not a pass."*

Per pre-registered rule (G-CE PASS plus ≥2 of 3 behavioral PASS = Run B PASS): **only G-CE passes; behavioral gates 0/3 honest** → **FAIL**.

Per long-term plan §3 Track 1 exit branch: *"G-CE PASS, <2 of 3 behavioral PASS → STOP, postmortem, decide push vs pivot."*

---

## G-CE: ✅ PASS (Δ −13.71)

| | Value |
|---|---|
| Base mean CE | 21.1763 (n=397) |
| LoRA cp-200 mean CE | **7.4708** (n=397) |
| Delta (base − LoRA) | **+13.7055** |
| Pre-registered threshold | ≥ 8.0 |
| Reproducibility | First base eval (standalone): 21.1763 — identical to second base eval (paired with LoRA): 21.1763. Bit-identical. |
| Comparison | WAVE12 (500 steps, corrupt parser): Δ −14.32. Run B (200 steps, corrected parser): Δ −13.71. **96% of WAVE12's CE descent at 40% of training steps.** |

**Interpretation:** the H6 fix unlocked dramatically more CE-efficient learning per step. But — see below — CE is fitting the corpus's high-frequency tokens, not learning generation.

---

## G-OPEN: ❌ FAIL (0/8)

Greedy generation at T=0.0 on the 8 pinned prompts produced ZERO outputs whose first non-special token is in the corpus's top-5 answer openers (` The`, `A`, `According`, `When`, `An`).

Threshold: ≥3/8. Actual: **0/8**.

---

## G-TOPIC: ❌ FAIL (0/8 honest)

The scorer initially reported 8/8 PASS. **This was a false positive** — the scorer's init-line stripping missed the `Loaded LoRA: raft/kingdom-w14b/kingdom.lora.gguf.checkpoint-200` line, which contains the literal substring "raft" and "Kingdom" (in the path) — both on the allowlist. The scorer then matched those words from the init region rather than from any generated content.

Honest score after manual review of raw completion tokens: **0/8**. The actual generated tokens contain no Kingdom-allowlist hits.

Action: extend the scorer's INIT_PREFIXES to strip `Loaded LoRA:` lines. (Not changing the gate threshold — that stays pre-registered.)

---

## G-NO-COLLAPSE: ❌ FAIL (honest 0/8) — gate had a blind spot

The scorer reported 8/8 PASS because it only checks for the WAVE14 Run-A attractor pattern (`\t}\n\nQuestion:\n` repeating). Run B's checkpoint-200 collapses to a **different** attractor: pure `\n` (token 107) repeating.

**Raw completion tokens for p1, T=0.0, cp-200:**
```
[107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107,
 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107,
 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107, 107,
 107, 107, 107, 107, 107, 107, 107]
```

All 52 generated tokens = newline. Same shape across all 8 prompts. After `skip_special_tokens=False` decode: 52 newlines (which `trim_end()` strips → empty string in the .txt output, which fooled my init-strip).

**Earlier checkpoint cp-50** (less trained) collapses to a *different* token: `[236770, 236764, 506, 236764, 506, 236764, ...]` = `1, the, the, the, the, ...`. So *both* checkpoints collapsed; cp-200 collapsed to a higher-frequency token (`\n`) than cp-50 (`, the,`).

**Sampled generation (T=0.7, top-k=40, seed=42)** on cp-200, p1: produced `2\n\n25... the.` then hit `<end_of_turn>`. Even with sampling the LoRA produces gibberish, not Kingdom answers — confirming the underlying distribution has no coherent generative structure.

Honest score: **0/8** (the gate didn't catch it because it was scoped to Run A's attractor, not a general "is this collapse?" check).

Action: extend the scorer's collapse detection to flag any single-token-dominance pattern (e.g., max-frequency-token / total > 0.6). (For the *next* run; not retroactive on this gate.)

---

## Behavioral aggregate: 0/3 honest PASS

The pre-registered rule was: G-CE plus ≥2 of (OPEN, TOPIC, NO-COLLAPSE) = PASS. Run B has G-CE alone. Behavioral aggregate is 0/3 once both false-positives are corrected.

**Verdict: FAIL.**

---

## Why does CE descend so far yet generation collapse?

The training loss with my Phase 1 clamp:
```rust
let effective_answer_start = example.answer_start
    .unwrap_or(cli_answer_start)
    .min(example.tokens.len() / 2);
```

For the Kingdom corpus (`tokens.len() = 384`, `answer_start ≈ 350`), this clamps to `192`. Loss is computed on positions [192..384] = 192 positions per example. That's actually *most* of the answer region (positions 350-384) **plus the bottom half of the question** (positions 192-350).

Within that 192-token window, the most frequent tokens by raw count are:
- `\n` (token 107) — appears as separators, in code blocks, end-of-line markers
- `, the,` patterns — common English bigram
- punctuation, common English filler

The LoRA found a degenerate optimum: predict the highest-frequency token at every position. The CE *number* drops because corpus CE is dominated by these high-frequency tokens. But the model learns no contextual generation — it's a unigram-frequency fit.

**This is the per-token-frequency-fit failure mode, distinct from Run A's "structural-anchor-fit" failure mode.** Both are local optima in the loss landscape that don't correspond to generation skill.

---

## What this rules in / out

- ✅ **The H6 parser fix is real and dramatically effective on CE.** The H6 analysis was correct.
- ✅ **CE descent confirms the data pipeline is now correct.** The fix is causal for the loss objective.
- ❌ **CE descent ≠ generation skill.** WAVE13 P2's lesson confirmed for the third time (now WAVE12, Run A, Run B).
- ❌ **The Kingdom RAFT corpus, even with corrected parser + per-example answer_start, does not produce a generative LoRA at rank=16/alpha=32 within 200 steps.** Whether more steps help is the open question.
- ⚠️ **The clamp-at-tokens.len()/2 may be biasing loss to high-frequency token regions.** Worth removing the clamp in a future run.

---

## What to do next (per long-term plan §3 Track 1 exit branch)

The plan branches:

> "G-CE PASS, <2 of 3 behavioral PASS → STOP. File postmortem. Decide whether to push to 3000-5000 steps OR pivot to coding-corpus per Track 2 directly."

Two paths:

### Path A — Push: continue training (Run C, 3000-5000 steps)

Hypothesis: undertraining is still the issue. WAVE12 ran 500 steps (corrupt) and got CE 6.78 / collapsed gen. Run B ran 200 steps (corrected) and got CE 7.47 / collapsed gen. 3000-5000 steps with corrected parser MIGHT escape the local optimum and learn coherent generation.

Cost: ~30+ GPU-hours given current step rate (~80 s/step warm).
Risk: high. Run A already did 1500 corrupt steps and got `\t}\n\nQuestion:` collapse. There's no evidence longer training escapes this kind of basin.
Variation worth trying: REMOVE the `.min(tokens.len() / 2)` clamp so loss is masked correctly to *only* the answer region (positions 350-384 ≈ 24 positions per example). With per-example answer_start now plumbed correctly, the clamp is no longer needed (it was a hedge against pre-fix corruption). Tighter loss masking should remove the unigram-frequency-fit pathway.

### Path B — Pivot: build coding-shape corpus (Track 2)

Hypothesis: the Kingdom RAFT corpus is the wrong shape for the actual goal. The user goal is offline coding agent, not Kingdom Q&A. Even if Run C succeeded, it would produce a Kingdom-RAFT specialist, not a coding helper.

Cost: 3-7 days of corpus engineering (Track 2 plan), plus T3 LoRA training.
Risk: lower. The coding corpus is the original goal; Track 2 was already designed for it.

### Path C — Combination: fix the clamp first, retrain Run C, decide pivot AFTER

Hypothesis: the clamp is a real bug masking actual quality of the H6 fix. Cheap to test (1-line edit, single re-run at e.g. 500 steps). If Run C with no clamp generates coherent text, Track 1 was correct. If still collapsed, pivot to Track 2.

Cost: ~3-5 GPU-hours (500 steps).
Risk: medium. Fastest way to know if the pre-registration was just measuring the wrong thing.

**Recommendation: Path C.** Cheapest experiment, decisive answer. Then PASS or PIVOT with confidence.

---

## Files generated by T1.4

- `crates/zhenai-forge/notes/wave14-phase2-quality/lora-w14b-p[1-8].txt` — 8 raw forge generation outputs (greedy T=0)
- `/tmp/wave14-debug-base-p1.txt` — base model on p1 (multilingual gibberish)
- `/tmp/wave14-debug-cp50-p1.txt` — cp-50 on p1 (collapsed to "the,")
- `/tmp/wave14-debug-temp07-p1.txt` — cp-200 on p1 with T=0.7 (still gibberish)
- `/tmp/wave14-eval-base.log` — standalone base CE eval (21.1763)
- `/tmp/wave14-eval-runB.log` — paired base + LoRA eval (Δ −13.71)

---

## Marshal sign-off

T1.4 done. Verdict: FAIL by pre-registered rule. The gate did its job — it caught a Pyrrhic CE win.

Standing by for Stevie's choice between Path A / B / C before T1.5 RAG control arm.
