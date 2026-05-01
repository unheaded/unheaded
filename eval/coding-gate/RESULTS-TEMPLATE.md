# Coding-Gate Results — &lt;date&gt;

**Date:** YYYY-MM-DD
**Grader:** Stevie
**Run by:** scripts/run-coding-gate.sh
**Binary:** bin/zhen-rag (HEAD &lt;short-sha&gt;)
**Backend:** llama-server &lt;url&gt; · model qwen2.5-coder-7b-instruct-q4_k_m · ctx=16384 · GPU layers=999
**Retrieval:** cs/vor at &lt;url&gt; · sources/unheaded symlink: &lt;target&gt;
**Decoding:** temperature=0.0 · k=5 · max_tokens=600

---

## Integrity checks (per RUBRIC §6)

- [ ] vor reachable on :9876
- [ ] llama-server reachable on :8081, ctx=16384
- [ ] zhen-rag built from current HEAD
- [ ] Smoke prompt grounded (WAVE14 H6 returns the session doc reference)
- [ ] Greedy determinism (same prompt twice → identical output)

---

## Per-prompt grades

| ID | Language | Kind | Latency (s) | Grade | Notes |
|----|----------|------|-------------|-------|-------|
| syntax-bash | bash | syntax | _ | _ | _ |
| syntax-python | python | syntax | _ | _ | _ |
| syntax-go | go | syntax | _ | _ | _ |
| syntax-rust | rust | syntax | _ | _ | _ |
| syntax-html | html | syntax | _ | _ | _ |
| syntax-css | css | syntax | _ | _ | _ |
| syntax-javascript | javascript | syntax | _ | _ | _ |
| review-bash | bash | review | _ | _ | _ |
| review-python | python | review | _ | _ | _ |
| review-go | go | review | _ | _ | _ |
| review-rust | rust | review | _ | _ | _ |
| review-html | html | review | _ | _ | _ |
| review-css | css | review | _ | _ | _ |
| review-javascript | javascript | review | _ | _ | _ |

Grades: PASS / FAIL / 🔴 (FAIL + veto). See `RUBRIC.md` §2.

---

## Aggregates

- **PASS count:** _ / 14
- **🔴 count:** _
- **Syntax half:** _ / 7
- **Review half:** _ / 7
- **Per-language**:

| Language | Syntax | Review | Total |
|---|---|---|---|
| bash | _ | _ | _ / 2 |
| python | _ | _ | _ / 2 |
| go | _ | _ | _ / 2 |
| rust | _ | _ | _ / 2 |
| html | _ | _ | _ / 2 |
| css | _ | _ | _ / 2 |
| javascript | _ | _ | _ / 2 |

---

## Verdict

Per `RUBRIC.md` §4 decision rule:

> _ (one of: H1 / H2 / H3 / H4)

**Justification:** _ (1-2 sentences citing the aggregate counts above)

**Next plan:** _ (one of: Phase D-A agent runtime / Phase D-B narrow LoRA / Phase D-C base swap / Phase D-veto system-prompt hardening)

---

## Raw outputs

(The runner appends one section per prompt below this line. Each section has the prompt ID, prompt text, latency, retrieved references, and full model output. Hand-grading happens by reading these sections and filling in the table above.)
