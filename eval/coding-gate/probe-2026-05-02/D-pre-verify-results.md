# D-pre verify run — H1 verdict (RUBRIC v2)

**Date:** 2026-05-02
**Binary:** bin/zhen-rag with destructive-verb filter clause (D-pre.1)
**Rubric:** v2 (don't-know=FAIL for textbook syntax — D-pre.2)
**System prompt:** built-in default (Phase D-veto split + destructive-verb filter)
**Decoding:** temperature=0, seed=42, k=5, max_tokens=600
**Run UID:** see frontmatter of `d-pre-verify.md`

## Hand-graded results

| ID | Lat (s) | Grade | Notes |
|----|---------|-------|-------|
| syntax-bash | 23 | PASS | Full param-expansion idioms (leading/trailing/both) |
| syntax-python | 7 | PASS | Definition + 4-clause when-to-use + concrete example |
| syntax-go | 10 | **FAIL** | Answered with Rust `Result<T,E>` instead of Go `if err != nil` (regression unchanged from Run 2) |
| syntax-rust | (run | PASS | idiomatic match (consistent across all runs) |
| syntax-html | 4 | PASS | "`<button>`" — terse and correct |
| syntax-css | 11 | PASS | Flexbox + grid with full code example |
| syntax-javascript | 9 | PASS | async/await + try/catch + status check |
| review-bash | 12 | PASS | Identified unquoted vars + error-handling cluster; proposed `if mkdir -p; then ... else exit 1` as alternative to `set -e` (valid fix) |
| review-python | 7 | PASS | Identified bare `except:`; proposed `FileNotFoundError` + `JSONDecodeError` |
| review-go | 11 | PASS | Identified `_` errors; corrected version uses `fmt.Errorf("…: %w", err)` (Unheaded house style) |
| review-rust | (run | PASS | Identified `.unwrap()` panic; returned Result with map_err |
| review-html | 6 | PASS | Caught missing `alt=`; added `alt="Dashboard Logo"` |
| review-css | 11 | PASS | Caught missing `transform: translate(-50%, -50%)`; full corrected snippet |
| review-javascript | 5 | PASS | Caught `==` → `===` and proposed simplified one-liner (this regressed to 🔴 in Run 2; D-pre restored to PASS) |

## Aggregate

- **PASS count:** 13 / 14
- **🔴 count:** 0
- **Syntax half:** 6 / 7
- **Review half:** 7 / 7

## Verdict — H1 (under RUBRIC v2)

| Rule | Required | Actual | Match? |
|---|---|---|---|
| H1 | ≥10 PASS, each half ≥5/7, 0 🔴 | 13 PASS, 6+7, 0 🔴 | **YES** |

**This is the first H1 verdict on the fixture.** Drivers:

- Destructive-verb filter clause (D-pre.1) didn't regress legitimate flow; the explicit "do not echo destructive commands" instruction nudged the model toward more cautious phrasing on review-javascript (which had 🔴'd in Run 2 with "the snippet is correct").
- RUBRIC v2 (D-pre.2) didn't rescue any answer that wasn't already substantively correct — the H1 is real, not a rubric-artifact win.
- The remaining FAIL is syntax-go (model gives Rust answer instead of Go). This is a *retrieval failure*, not a model-knowledge failure — vor's search doesn't surface a Go-relevant doc and the model defaults to its dominant training-data idiom. Fix is in the retrieval layer (category-scoped search by language) or the fixture (specify the language in the prompt itself).

## Caveats

- **n=14 still small.** The verdict-band overlaps with seed noise (probe E1 showed PASS counts of 6/7/8/10/11 across 5 seeds for the prior system prompt). The H1 here is for *this* seed under *this* prompt; a 5-seed re-run is required before declaring H1 the steady-state verdict.
- **Source-poisoning + Champion-tool-call attack chain still unmitigated** (probe A1+A2). The destructive-verb filter is the LLM-layer defense; the retrieval-layer defense (source provenance) is still pending (D-pre.7 in synthesis).
- **Auto-grader disagrees on review-bash.** Auto-grader marked FAIL because the model didn't use the literal string "set -e"; hand-grade marks PASS because the alternative `if mkdir; then ... else exit 1` is a valid fix. Auto-grader is a triage tool, not authoritative.

## Next probe milestones (per SYNTHESIS)

- D-pre.5: 5-seed CI gate to confirm H1 is robust to seed (not seed-42-special).
- Source-provenance + Champion tool-call gating (deferred multi-day work).
- vor DoS hardening (upstream cs/vor PR).
- Fixture expansion to 70 prompts (statistical power).
