---
title: E6 — Re-grade Run 1 + Run 2 with don't-know=FAIL
date: 2026-05-02
---

# E6 — Re-grade with don't-know=FAIL

## Why

Phase D-veto's results-doc analysis identified a misalignment between RUBRIC §2's PASS rule (counting "I don't know" as PASS for syntax prompts) and the gate's stated purpose ("useful junior+ coding"). A graceful refusal is not a useful answer; counting it as PASS inflates the score with uninformative output.

This re-grade applies an alternative rule: **for syntax prompts, "I don't know" / "the references do not contain the answer" counts as FAIL.** All other rubric criteria unchanged.

## Re-grade table

Pulling original grades from the committed results docs:

- Run 1: `eval/coding-gate/results-2026-05-01.md` (commit 15a3ec8b)
- Run 2: `eval/coding-gate/results-2026-05-01-postveto.md` (commit f173fd8c)

| ID | Run 1 (orig) | Run 1 (don't-know=FAIL) | Run 2 (orig) | Run 2 (don't-know=FAIL) |
|---|---|---|---|---|
| syntax-bash | PASS | PASS | PASS | PASS |
| syntax-python | PASS | PASS | PASS | PASS |
| syntax-go | PASS¹ | **FAIL** | FAIL² | FAIL |
| syntax-rust | PASS | PASS | PASS | PASS |
| syntax-html | PASS | PASS | PASS | PASS |
| syntax-css | PASS¹ | **FAIL** | PASS | PASS |
| syntax-javascript | PASS¹ | **FAIL** | PASS | PASS |
| review-bash | PASS | PASS | FAIL | FAIL |
| review-python | PASS | PASS | PASS | PASS |
| review-go | PASS | PASS | PASS | PASS |
| review-rust | PASS | PASS | PASS | PASS |
| review-html | FAIL | FAIL | PASS | PASS |
| review-css | 🔴 | 🔴 | PASS | PASS |
| review-javascript | PASS | PASS | 🔴 | 🔴 |

¹ Original "I don't know" graceful refusal — flips to FAIL under E6 rule.
² Run 2 syntax-go gave Rust+Python instead of Go — already FAIL under both rubrics.

## Aggregate comparison

| Rule | Run 1 PASS | Run 2 PASS | Δ | 🔴 |
|---|---|---|---|---|
| Original (don't-know=PASS) | 12/14 | 11/14 | -1 | both 1 |
| E6 (don't-know=FAIL) | **9/14** | **11/14** | **+2** | both 1 |

## Verdict comparison

| Rule | Run 1 verdict | Run 2 verdict |
|---|---|---|
| Original | H2 (1 🔴 vetoes H1) | H2 (1 🔴 vetoes H1) |
| E6 | H2 (9/14 in 7-9 range, 1 🔴, halves 4+5/7) | H2 (1 🔴 vetoes H1; 11 PASS otherwise H1) |

## Conclusion

**Under the E6 rule, Run 2 outperforms Run 1 by 2 PASS** — confirming the post-D-veto narrative that Run 2 is substantively better despite the same H2 verdict in the original rubric. Both runs still land H2 because the 🔴 floor of 1 vetoes H1 in both cases.

**Recommendation: adopt the E6 rule going forward.** Update RUBRIC.md §2 to specify that *"I don't know"* on a syntax prompt is FAIL when the question is well-known textbook material (the gate's intent) — only allow it as PASS for genuinely ambiguous or out-of-scope questions. The seven syntax prompts in the fixture are all textbook; none should be answered with "I don't know" by a model that has the knowledge.

This rubric correction is a meta-finding from the probe: the original rubric design accidentally rewarded the over-restrictive Phase B system prompt for the wrong reason.
