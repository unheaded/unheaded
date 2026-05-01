# Coding-Gate Results — 2026-05-01 (post Phase D-veto)

**Date:** 2026-05-01 (second run, ~30 min after first)
**Grader:** Stevie + crew (drafted by claude-opus-4-7 under unheaded-scientist lens; awaiting Stevie sign-off)
**Run by:** scripts/run-coding-gate.sh
**Binary:** bin/zhen-rag (HEAD 15a3ec8b + Phase D-veto unstaged: system-prompt rewrite splitting Unheaded-specific vs general-programming)
**Backend:** llama-server http://127.0.0.1:8081 · model qwen2.5-coder-7b-instruct-q4_k_m · ctx=16384 · GPU layers=999
**Retrieval:** cs/vor at http://127.0.0.1:9876 · sources/unheaded symlink: /home/govan/tmp/unheaded
**Decoding:** temperature=0.0 · seed=42 · k=5 · max_tokens=600 · max_topic_chars=10000

This re-eval tests Phase D-veto's hypothesis: the over-restrictive system prompt was the dominant cause of the run-1 verdict failures (3 graceful "I don't know" syntax refusals + 1 🔴 + 1 FAIL on review). The new system prompt splits Unheaded-specific facts (must come from refs, never invent) from general programming knowledge (model uses training directly), and explicitly tells the model to identify bugs in code-review prompts even when refs don't mention them.

**Comparison fixture frozen.** Same `prompts.jsonl`, same `RUBRIC.md`, same `seed=42`, same model. Only variable: the system prompt in `cmd/zhen-rag/main.go::main`.

---

## Integrity checks (per RUBRIC §6)

- [x] vor reachable on :9876
- [x] llama-server reachable on :8081, ctx=16384
- [x] zhen-rag rebuilt with new system prompt
- [x] Smoke prompt grounded
- [x] Greedy determinism (greedy + seed=42 byte-identical across reruns)

---

## Per-prompt grades

| ID | Lang | Kind | Lat (s) | Run-1 | Run-2 | Δ | Notes (run 2) |
|----|------|------|---------|-------|-------|---|---------------|
| syntax-bash | bash | syntax | 10 | PASS | PASS | — | full param-expansion answer |
| syntax-python | python | syntax | 10 | PASS | PASS | — | full list-comp + filter examples |
| syntax-go | go | syntax | 10 | PASS¹ | **FAIL** | ⬇ | answered with Rust+Python examples, never showed Go's `if err != nil` |
| syntax-rust | rust | syntax | 8 | PASS | PASS | — | idiomatic match |
| syntax-html | html | syntax | 4 | PASS | PASS | — | "`<button>`" — terse and correct |
| syntax-css | css | syntax | 15 | PASS¹ | PASS | ⬆ | run 1 was "don't know"; run 2 gave 4 methods (flexbox/grid/text-align/margin) |
| syntax-javascript | js | syntax | 9 | PASS¹ | PASS | ⬆ | run 1 was "don't know"; run 2 gave full async/await + try/catch |
| review-bash | bash | review | 4 | PASS² | **FAIL** | ⬇ | flagged command-injection (real, but not the expected_flag); missed strict mode + unquoted vars + mkdir error handling; "fix" still has unquoted `$DIR` |
| review-python | python | review | 5 | PASS | PASS | — | named bare-except, FileNotFoundError, JSONDecodeError |
| review-go | go | review | 4 | PASS | PASS | — | flagged `_` errors, returned `error` (less Unheaded-style than run 1 — used raw `return err` instead of `fmt.Errorf("…: %w", err)`) |
| review-rust | rust | review | 2 | PASS | PASS | — | flagged `.unwrap()`, returned `Result<u16, String>` with map_err |
| review-html | html | review | 2 | FAIL | **PASS** | ⬆ | caught missing alt attribute, fixed with `alt="Logo"` |
| review-css | css | review | 1 | 🔴 | **PASS** | ⬆ | caught missing `transform: translate(-50%, -50%)`, fixed exactly |
| review-javascript | js | review | 0 | PASS | **🔴** | ⬇ | "The JavaScript snippet is correct. There are no issues with it." — confidently wrong on `==` vs `===` |

¹ Graceful "I don't know" — counted PASS for syntax per rubric §2 but not a useful answer.
² Partial: flagged unquoted vars + suggested `set -e` (not full `-euo pipefail`).

---

## Aggregates

- **PASS count:** 11 / 14 (run 1: 12 / 14)
- **🔴 count:** 1 (same as run 1, but moved from review-css → review-javascript)
- **Syntax half:** 6 / 7 (run 1: 7/7 with 3 don't-knows)
- **Review half:** 5 / 7 (run 1: 5/7)

| Language | Syntax | Review | Total | Run 1 |
|---|---|---|---|---|
| bash | PASS | FAIL | 1/2 | 2/2 |
| python | PASS | PASS | 2/2 | 2/2 |
| go | FAIL | PASS | 1/2 | 2/2 |
| rust | PASS | PASS | 2/2 | 2/2 |
| html | PASS | PASS | 2/2 | 1/2 |
| css | PASS | PASS | 2/2 | 1/2 |
| javascript | PASS | 🔴 | 1/2 | 2/2 |

---

## Verdict — H2 confirmed (twice)

Per `RUBRIC.md` §4 decision rule:

| Rule | Required | Run-2 actual | Match? |
|---|---|---|---|
| H1 | ≥10 PASS, each half ≥5/7, **0 🔴** | 11 PASS, 6+5, 1 🔴 | **No** (🔴 blocks) |
| H2 | 7-9 PASS, each half ≥2/7, ≤1 🔴 | 11 PASS (above range) | numeric range fails |
| H3 | <7 PASS or any half ≤1/7 | 11, 6+5 | No |
| H4 | ≥2 🔴 | 1 🔴 | No |

Same gap as run 1 — between H1 and H2. Per RUBRIC §4 "stricter of matching": **H2**.

### What Phase D-veto established

Phase D-veto's hypothesis was: *the system prompt is the dominant cause of failures; loosening it produces H1.*

**Result: hypothesis partially confirmed, partially falsified.**

Confirmed:
- 3 syntax "don't know" refusals → 2 substantive PASSes + 1 off-topic FAIL (CSS + JS now answer correctly; Go went off-language).
- 1 review FAIL (html missing alt) → PASS (model now flags the alt issue).
- 1 review 🔴 (css centering misconception) → PASS (model now flags missing transform).

Falsified:
- The 🔴 didn't disappear — it migrated from review-css to review-javascript. Run 1: model caught `==`/`===`. Run 2: model said "no issues with it" on the same snippet. The "for code-review prompts: identify bugs even when no reference mentions them" instruction added in the new system prompt did NOT prevent confident "no issues" output on a different review prompt.
- review-bash regressed from partial-PASS to FAIL — model went off-script and flagged command injection instead of the expected strict-mode + unquoted-vars + error-handling cluster.

**Conclusion**: system-prompt iteration moves *which* prompts pass and fail, but does not eliminate the "confident 'looks fine'" failure mode in code review at the 7B base + RAG layer. The 🔴 ceiling appears to be at 1, not 0, regardless of system-prompt phrasing — at least for this fixture and seed.

Per RUBRIC §10 risk mitigation: *"Risk: system-prompt iteration loops. Hard cap: 2 iterations. If a third is needed, treat as H3 (model is the problem, not the prompt)."*

We are at iteration 1 (the original) + iteration 2 (this run). Spirit-of-the-cap says: don't iterate the prompt a third time. The 🔴 is structural, not prompt-shaped.

### Substance vs count

**Run 2 is substantively better than run 1 even at the same H2 verdict.** Run 1's 12 PASS includes 3 graceful refusals (uninformative) + 1 🔴 (review-css confabulation about centering — the well-known web-tutorial misconception). Run 2's 11 PASS is 11 substantive answers across 7 languages with no graceful-refusal padding. The 🔴 moved to a different prompt but is similar in shape.

For the user-facing experience — which is what the gate measures per RUBRIC §1 ("the eval measures what users actually get") — run 2's system prompt is the better one to ship.

### Next plan

**Lock in run 2's system prompt as the V1 default; move to Phase D-A** (Champion + zhen-rag agent runtime) with two explicit guardrails for code-review prompts:

1. **Review-prompt guard**: in any agent flow that accepts pasted-snippet review, prepend the user prompt with the assertion *"this snippet contains at least one bug; identify it"* — front-loads the expectation. This is a 1-line change in the agent layer, not the model.
2. **Self-critique pass**: after the model produces a "no issues" answer, fire a follow-up turn ("are you sure? look again at equality operators / accessibility / unwrap usage / ...") before showing the final answer to the user. This costs 1 extra round-trip but eliminates the confident-no-issue failure mode at the cost we have budget for (latency was 0-15s; budget 30-60s per RUBRIC §6).

**Phase D-B** (narrow LoRA on review-confabulation gap) is **deferred**, not killed. If Phase D-A's two guardrails don't drop the 🔴 rate to 0 in extended use, Phase D-B re-opens with empirical training data (collected from real Phase D-A failures, not synthesized).

**Phase D-C** (base swap) remains NOT triggered — 11/14 substantive PASSes across 7 languages confirms the model has the knowledge; the gap is review-orchestration, not base-model capability.

---

## Recommendation summary

| Decision | Verdict |
|---|---|
| Verdict (RUBRIC §4) | **H2** (same as run 1) |
| Ship system prompt | **Run-2 prompt** (substantively better answers, same verdict) |
| Next phase | **Phase D-A** (agent runtime) with review-prompt + self-critique guardrails |
| LoRA training | Deferred until Phase D-A produces real failure data |
| Base swap | Not triggered |
