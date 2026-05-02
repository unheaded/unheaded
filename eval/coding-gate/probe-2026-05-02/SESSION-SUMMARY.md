---
title: Session summary — D-pre hardening (2026-05-01 night → 2026-05-02 morning)
date: 2026-05-02
---

# D-pre hardening — session summary

Single-pager for Stevie to skim in the morning. Full detail in the per-finding docs.

## Verdict status

| Run | Rubric | PASS | 🔴 | Verdict |
|---|---|---|---|---|
| Phase C run-1 (committed `15a3ec8b`) | v1 | 12/14 | 1 | H2 |
| Phase D-veto run-2 (committed `f173fd8c`) | v1 | 11/14 | 1 | H2 |
| D-pre verify (committed `fff92dca`) | v2 | 13/14 | 0 | **H1** |

First H1 on the fixture. Drivers: destructive-verb filter (D-pre.1) + RUBRIC v2 (don't-know=FAIL for textbook syntax). Single seed; 5-seed CI run pending.

## Commits this session

| SHA | What |
|---|---|
| `d5979294` | Probe — scientist + blackmage findings (n=14 noise sweep, source-poison demo, prompt-injection, vor fuzz, E6 re-grade) |
| `fff92dca` | D-pre.1–5 + D-pre.7 (filter, rubric v2, sanitization, gate verify, CI gate, +14 hard prompts) |
| `7670eec4` | DEFERRED.md — note B3 (vor harden) progressed to drafted on cs branch |
| `7b472feb` | B1 + B2 designs — source provenance schema + Champion tool-call envelope |
| (cs) `fa46000` | cs branch `harden/api-dos-and-traversal` — MaxQueryLength + path-traversal reject |

Total: 4 commits in unheaded + 1 commit in cs (separate repo, separate PR). All local; ready to push.

## What's protected now

The chained source-poison + meta-instruction attack from probe A1+A2 — which would have RCE'd via Champion tool calls in Phase D-A — is mitigated at three layers:

1. **LLM layer (D-pre.1, shipped):** zhen-rag's system prompt rejects destructive shell verbs in retrieved references regardless of source. Verified: model now emits a fixed safety message and nothing else when the poisoned content recommends `rm -rf …`.
2. **Retrieval layer (B1, designed):** cs/vor will label content by source kind (`embedded` / `user-custom` / `user-source`) and rank embedded above user-source for tied relevance. Implementation is multi-repo work, scheduled.
3. **Agent layer (B2, designed):** Champion tool calls accept a `justification` chain; mutating tools with user-source justification require out-of-band user confirmation. Implementation is gated on B1 landing AND Phase D-A scope.

(1) ships now and is the first line. (2)+(3) are the structural fail-safe — the model can be coerced past (1) by sufficiently clever prompting; (2)+(3) are not coerceable.

## What's blocked / what's open

**Blocked on multi-repo coordination:**
- B1 (source provenance) — needs cs schema change + minor version bump.
- B2 (Champion tool-call gating) — needs B1 + Phase D-A scope.

**Open for Stevie:**
- Push 4 unheaded commits to origin/main (SSH key not loaded in this session): `! git push origin main`
- Review + PR cs branch `harden/api-dos-and-traversal` (commit `fa46000`) to bellistech/cs upstream.
- Decide whether B1 implementation goes ahead this week or after the CI gate has more soak time.

**Open in unheaded backlog:**
- 5-seed CI gate run currently in progress; SUMMARY in `eval/coding-gate/ci-runs/<UTC>/` once finished. (gitignored — rerunnable artifact.)
- Fixture at 28 prompts (14 textbook + 14 hard); SYNTHESIS target was 70. Half done; the remaining 42 is incremental work, not blocking.
- syntax-go regression — model gives Rust answer for "how do I check an error after a function returns" because vor's search returns Go-irrelevant docs and the model defaults to its dominant Rust idiom. Fix is in retrieval (category-scoped search by language) or fixture (specify language in the prompt itself) — neither is in this session's scope.

## What this session DID NOT do

- Phase D-A (agent runtime) — explicitly out of scope; B1+B2 still required first.
- LoRA training (D-B) — explicitly out of scope; H1 verdict means no narrow LoRA needed for V1.
- Base model swap (D-C) — explicitly out of scope; not triggered.
- Forge work, Doom work, kingdom-mode work — all out of scope under the marshal lane.

## Sleep well

The verdict moved from H2 to H1. The chained attack is mitigated at the LLM layer today; the structural fail-safes are designed and ready to implement when the cs/vor schema change lands. The probe artifacts are sanitized so the docs themselves are not new attack vectors. No outstanding security findings ≥ HIGH severity (one MEDIUM in cs/vor on the topic branch awaiting upstream PR).

— Marshal signing off.
