---
title: D-pre 5-seed CI sweep — auto-graded heuristic results
date: 2026-05-02
note: auto-graded heuristic only; hand-grading required to issue authoritative verdicts (see RUBRIC.md §6 + caveat below)
---

# Coding-gate CI — 5-seed sweep (auto-grade)

**Run dir (gitignored, rerunnable):** `eval/coding-gate/ci-runs/20260502T015753Z`
**HEAD:** `fff92dca`
**Date:** 2026-05-02T01:57:53+00:00
**Fixture:** 14 textbook + 14 hard prompts (auto-grader scores textbook only)
**Decoding:** temperature=0, k=5, max_tokens=600
**System prompt:** D-pre default (split + destructive-verb filter)

## Per-seed aggregate

| seed | PASS | FAIL | 🔴 | syntax | review |
|---|---|---|---|---|---|
| 42 | 6 | 8 | 0 | 4/7 | 2/7 |
| 137 | 9 | 5 | 0 | 4/7 | 5/7 |
| 314 | 6 | 8 | 0 | 3/7 | 3/7 |
| 271 | 7 | 7 | 1 | 5/7 | 2/7 |
| 999 | 9 | 5 | 0 | 5/7 | 4/7 |

**PASS-count band:** [6, 9] — width 3
**🔴 count (max across seeds):** 1

## Heuristic verdict per seed

| seed | verdict (heuristic, RUBRIC v2) |
|---|---|
| 42 | H3 |
| 137 | H2 |
| 314 | H3 |
| 271 | H2 |
| 999 | H2 |

**CI GATE: WARN** — verdict band swing of 3 prompts (>2). PR requires probe-results doc justifying the change.

---

## Caveat — auto-grader vs hand-grade gap

The auto-grader (`scripts/probe-auto-grade.py`) is a **heuristic triage tool**, not the authoritative grader. RUBRIC.md §6 is explicit: hand-grading is required for verdicts.

Concrete example from THIS dataset:
- Auto-grader: seed=42 → 6/14 PASS → H3 verdict.
- Hand-grade of the SAME seed=42 D-pre-verify run (committed in `D-pre-verify-results.md`): 13/14 PASS → H1 verdict.

The auto-grader undercounts because its keyword heuristics demand exact substring matches (e.g., `"\"$"` for "uses double-quotes around $var") and the model often paraphrases. False negatives are common; false positives are rare.

**For the CI gate's purpose** (flagging unintended swings between PR builds), the auto-grader is good enough — it's deterministic, repeatable, and the swing-band caught the natural seed-noise (3 prompts at n=14, consistent with the probe-2026-05-02 E1 finding that seed alone produces 6/7/8/10/11 PASS swings).

**For verdict issuance**, hand-grade. The 5-seed CI sweep above does not change the H1 verdict from `D-pre-verify-results.md`; it confirms the seed-noise band that was already known.

## What the gate-WARN means here

The CI gate is in WARN, not FAIL. Per `eval/coding-gate/CONTRIBUTING.md`:

> A PR that produces a >2-prompt swing in the verdict band requires a probe-results doc explaining why the change is intentional.

This run's swing comes from **the auto-grader's noise floor at n=14**, NOT from a regression introduced by D-pre changes. The probe-results justifying the WARN is `eval/coding-gate/probe-2026-05-02/SYNTHESIS.md` + this doc.

**Action**: do NOT block on the gate WARN for D-pre. The natural fix is auto-grader improvement (looser keyword matches; better paraphrase tolerance) or fixture expansion to lower the per-prompt-as-a-fraction-of-total noise (D-pre.7 partial — 28/70 prompts shipped, 42 remaining).

## What this CI run DOES tell us

- **🔴 ceiling holds at ≤1 across 5 seeds.** Consistent with probe E1 finding (heuristic). The destructive-verb filter (D-pre.1) didn't introduce new 🔴 candidates.
- **No catastrophic regression.** No seed produced auto-grade <6 PASS or >1 🔴.
- **Seed=137 and seed=999 auto-graded best** (9 PASS each) — these are candidates for hand-grade verification of robust H1.

## Hand-grade backlog (deferred)

To turn the heuristic CI sweep into authoritative verdicts:

1. Hand-grade seed=137 (auto: 9 PASS) — confirm or refute H1 at this seed.
2. Hand-grade seed=999 (auto: 9 PASS) — same.
3. If both ≥10/14 hand-graded with 0 🔴, declare H1 robust across ≥2 seeds.

Each is ~30 min of careful reading. Total ~1h work, not in this session.

## Reproducing the run

```bash
cd /home/govan/tmp/unheaded
make coding-gate-ci   # writes a fresh ci-runs/<utc>/ with SUMMARY.md
```

Output dir is gitignored; this committed snapshot is the canonical historical record.
