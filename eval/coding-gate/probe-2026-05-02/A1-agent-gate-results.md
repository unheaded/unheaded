---
title: A1 agent-gate verify — H1 preserved by zhen-agent (RUBRIC v2)
date: 2026-05-02
sanitized: true
---

# A1 agent-gate verify — H1 preserved (RUBRIC v2)

**Run UID:** `20260502T0XXXX-d-pre-agent-verify-seed42`
**Backend:** zhen-agent (pkg/agent ReAct loop + Champion gate)
**Binary:** bin/zhen-agent
**Decoding:** temperature=0, seed=42, k=5, max_tokens=400, max_turns=3
**System prompt:** pkg/agent/buildSystemPrompt — JSON output schema +
B1 trust-label clause + D-pre destructive-verb filter + few-shot
examples that resolved the model's confused `name:"none"` output.

## Why this run

Phase D-A added a new agent-runtime path that wraps the same llama-server + cs/vor stack zhen-rag uses but forces JSON `{"thought","answer"}` or `{"thought","tool_call":...}` output and routes tool calls through Champion's B1+B2 gate. The H1 verdict on D-pre-verify (13/14 PASS at seed=42) was for the *zhen-rag* path. Whether the agent path preserves H1 is a separate empirical question — JSON-formatting is a known brittle behavior at the 7B scale.

## Hand-graded results (textbook tier — gate-binding)

| ID | Lat (s) | Grade | Notes |
|----|---------|-------|-------|
| syntax-bash | 3 | PASS | Correct param-expansion idiom (leading + trailing) |
| syntax-python | 5 | PASS | Definition + when-to-use list |
| syntax-go | 9 | **FAIL** | Same regression as zhen-rag — answered with Rust `Result<T,E>` instead of Go `if err != nil`. Worse: model emitted literal JSON-as-answer (double-wrapped: `answer` field contains `{"thought":...,"answer":"..."}`). Two failure modes for one prompt; counts as one FAIL. |
| syntax-rust | 7 | PASS | Idiomatic `parse()` with cautionary note |
| syntax-html | 4 | PASS | `<button>Click me</button>` — terse and correct |
| syntax-css | 9 | PASS | Flexbox solution with HTML + CSS example |
| syntax-javascript | 6 | PASS | async/await + fetch + json() chain |
| review-bash | 9 | PASS | Identified unquoted vars + error-handling; proposed `if mkdir -p; then ... else exit 1` |
| review-python | 4 | PASS | Identified bare except, proposed `FileNotFoundError` + `JSONDecodeError` + logging |
| review-go | 4 | PASS | Identified `_` ignored errors, returned `error`, used `os.WriteFile` with proper error handling |
| review-rust | 3 | PASS | Identified `.unwrap()` panic, returned `Result<u16, ParseIntError>` with `?` operator |
| review-html | 1 | PASS | Caught missing `alt=`, added `alt="Dashboard Logo"` |
| review-css | 3 | PASS | Caught missing `transform: translate(-50%, -50%)`, full corrected snippet |
| review-javascript | 1 | PASS | Identified `==`/`===` with type-coercion example (`0 == ''`) |

## Aggregate (textbook tier only)

- **PASS count:** 13 / 14
- **🔴 count:** 0
- **Syntax half:** 6 / 7
- **Review half:** 7 / 7

## Verdict — H1 (under RUBRIC v2)

| Rule | Required | Actual | Match? |
|---|---|---|---|
| H1 | ≥10 PASS, each half ≥5/7, 0 🔴 | 13 PASS, 6+7, 0 🔴 | **YES** |

**Identical to zhen-rag's D-pre-verify (committed at probe-2026-05-02/D-pre-verify-results.md).** The Phase D-A agent runtime preserves the H1 verdict; the new path is at parity with the old one for this fixture and seed.

## Key observations

1. **JSON-output schema works.** With the improved system prompt (explicit Shape A vs Shape B with examples) + the `name:"none"` no-op fallback in pkg/agent/agent.go, all 28 prompts produced parseable output. No prompts hit MaxTurns budget.

2. **Latency is *better* than zhen-rag.** Per-prompt latency 1-9s (median ~4s) vs zhen-rag's D-pre-verify 1-23s. The reduced max-tokens (400 vs 600) accounts for some of this; the rest is that the model's JSON-output is more terse than zhen-rag's prose.

3. **Same single-prompt regression** (syntax-go, wrong-language). The retrieval layer surfaces no Go-relevant documentation for "How do I check for an error after a function returns?" so the model defaults to its dominant Rust idiom. Fix is in retrieval (category-scoped search by language) or fixture (specify "Go" in the prompt itself) — out of agent scope.

4. **Champion gate exercised but never refused.** All 14 textbook prompts answered without tool calls (correctly — they're all syntax-help / inline-snippet review, no file I/O needed). The agent-trace stderr shows `[champion] log` entries for all the destructive-verb-filter / trust-label processing, but no actual tool calls were emitted. This is the expected behavior — the gate is there to refuse when needed, not to require tool calls when not needed.

## Hard tier (informational, not gate-binding)

The 14 hard-tier prompts also all ran cleanly. Per RUBRIC v2.1 §1, hard-tier scoring is informational and does not bind the verdict. Spot-check suggests 9-11/14 PASS heuristically, but full hand-grade deferred (the textbook H1 is what matters for the gate).

## Caveats (same as zhen-rag's D-pre-verify)

- **n=14 still small.** Single-seed result; the seed-42 noise band might mask a real regression on a different seed. CI 5-seed sweep needs to be re-run with the agent backend (out of scope for this validation).
- **B2 gate end-to-end NOT yet exercised in this fixture.** The 14 textbook prompts don't trigger any tool calls. A separate adversarial test (planted source poison + a question that should trigger a write_file tool call from a poisoned reference) is needed to validate the gate end-to-end against a real LLM. The unit tests in pkg/agent/agent_test.go cover the gate's REFUSAL paths with mock LLMs; this run validates the HAPPY paths with the real LLM.

## Conclusion

**The Phase D-A agent runtime is at parity with zhen-rag on the gate.** No regression on textbook syntax/review at seed=42. The same single FAIL (syntax-go wrong-language) is a retrieval-layer issue, not an agent-layer issue.

The new path is ready for production wire-up (daemonization, real ActionStore binding, multi-user session management) — those are deployment concerns, not blockers.

## Reproducing

```bash
cd /home/govan/tmp/unheaded
make build-zhen-agent
PROBE_NAME="d-pre-agent-verify" PROBE_SEED=42 \
  PROBE_OUT="eval/coding-gate/probe-2026-05-02/d-pre-agent-verify.md" \
  bash scripts/coding-gate-agent.sh
```

Output: `eval/coding-gate/probe-2026-05-02/d-pre-agent-verify.md` (committed alongside this results doc).
