# WAVE10F 24-Hour Session — 2026-04-20

**Plan executed:** `crates/zhenai-forge/notes/wave10f-24h-battle-plan.md`
**Enforcement:** `unheaded-marshal` on duty; Stevie AFK.
**Status:** IN PROGRESS (this doc is live-updated; final Phase 7 commit
finalizes).

---

## Overall phase outcome

| Phase | Name | Outcome | Time |
|------:|------|---------|------|
| 0 | Recon + preflight | ✅ PASS | 30 min |
| 1 | Exp 2 hard gate (plateau) | 🔍 DIAG (honest — log(vocab) floor) | ~1 h |
| 2 | Exp 1 extended + lr sweep | TBD | TBD |
| 3 | Multi-Y capacity probe | TBD | TBD |
| 4 | Tokenizer vertical slice | ✅ PASS | <1 h (pipelined) |
| 5 | Kingdom QA RAFT kickoff | TBD | TBD |
| 6 | Regression audit | TBD | TBD |
| 7 | Docs + handoff | in this doc | ~1 h |

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

## Phase 2 — Exp 1 Extended + LR Sweep (TBD)

_[results pending]_

## Phase 3 — Multi-Y Capacity Probe (TBD)

_[results pending]_

## Phase 4 — Tokenizer Vertical Slice (DONE, pipelined while GPU busy)

- `scripts/tokenize-kingdom-for-gemma4.py` landed at commit `<hash>`
- Full Kingdom corpus pre-tokenized to `/tmp/24h-kingdom-{train,eval}.jsonl`
  (3568 / 397 sequences, p50 seq=384, p50 answer=48)
- Python venv `/home/govan/tmp/gemma4-venv` + `transformers.AutoTokenizer`
  avoids PEP 668 pip restriction
- Notes: `notes/wave10f-tokenizer-slice.md`

## Phase 5 — Kingdom QA RAFT Kickoff (TBD)

_[results pending]_

## Phase 6 — Regression + Timing Audit (TBD)

_[results pending]_

## Phase 7 — Docs + Session Handoff (in progress)

- This document.
- `notes/wave10f-learning-gate-plan.md` updated with iter-5 finding.
- `/tmp/24h-exp2-outcome.md` persisted as full Exp 2 record.
- `/tmp/24h-regression-audit.sh` script staged for Phase 6.
- CLAUDE.md update pending Phase 5 results.

## Commit chain (this session)

_[filled in by Phase 7 final]_

## Next-session prompt

```
WAVE10F post-24h-block. Current state (2026-04-20):
- Exp 2: DIAG — log(vocab) floor makes synthetic scrambled-train
  discrimination impossible. Real negative control in Phase 7.2.
- Optimal lr: [X] (from Phase 2 sweep)
- Multi-Y capacity: [verdict] (from Phase 3)
- Kingdom RAFT: [first LoRA at /tmp/...]
- Tokenizer: Python wrapper at scripts/tokenize-kingdom-for-gemma4.py

Open items:
- [per-phase: filled in by Phase 7]

Next likely sprint: [Phase 7.1 full Rust SentencePiece OR Phase 7.3 LoRA A/B eval]
```
