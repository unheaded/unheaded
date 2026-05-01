# Coding-Gate Rubric — pre-registered

**Date:** 2026-05-01
**Authors:** Stevie + crew, drafted under the unheaded-scientist lens
**Status:** LOCKED. This rubric is pre-registered before the runner is invoked.
**Permanent quality gate**: every future Zhen change (corpus update, retrain, base-model swap, serving-stack change) re-runs against the same `prompts.jsonl` and is graded against this rubric.

---

## 1. What this gate measures

Whether RAG-over-Qwen2.5-Coder-7B-Instruct (q4_k_m, ctx=16384, GPU layers=999), with retrieval grounded in cs/vor + Unheaded markdown, clears a *useful junior+* coding bar across seven languages — `bash`, `python`, `go`, `rust`, `html`, `css`, `javascript`.

Two halves:

- **Syntax half** (7 prompts) — *"how do I X in language Y?"* The model returns a short, correct snippet.
- **Review half** (7 prompts) — *"review this snippet, what's wrong?"* The model identifies the well-known junior-level bug and proposes a fix.

The bar is **NOT** Anthropic-tier reasoning. The bar is *"Stack-Overflow accepted answer"*: useful, terse, idiomatic, not confidently wrong.

---

## 2. Per-prompt grading

For each of the 14 prompts, assign exactly **ONE** of:

### PASS (1 pt)

The answer is correct, actionable, and not confidently wrong.

- **Syntax**: gives a snippet that compiles/runs, names the right idiom, points the user in the right direction.
- **Review**: identifies the expected bug (`expected_flag` field of the prompt) and proposes a viable fix. May identify additional, valid bugs as bonus — does not have to enumerate every issue.
- **"I don't know"** counts as PASS for **syntax** prompts only. (Per `project_zhenai_coding_gate.md` §5: *"'I don't know' is acceptable."*) For **review** prompts, *"I see no issue"* counts as **FAIL** — review is exactly the case where missing the bug is the failure mode.

### FAIL (0 pt)

The answer is wrong, misses the point, or hallucinates.

- **Syntax**: snippet doesn't run, names the wrong language, suggests deprecated/non-idiomatic approach as the primary answer, or wanders off-topic.
- **Review**: misses the `expected_flag` bug, claims the snippet is fine, proposes an unrelated fix, or hallucinates a non-existent bug as the main finding.

### 🔴 RED-FLAG (FAIL + veto)

A subset of FAIL. The answer is **confidently wrong** in a way that, if a junior engineer trusted it, would harm the codebase or produce a security incident. Examples:

- Tells a user `rm -rf $UNDEF` is fine.
- Says `==` is the correct equality operator in modern JavaScript.
- Says `.unwrap()` is the right way to handle production errors in Rust.
- Confidently produces syntactically invalid code as the canonical answer.
- Hallucinates a "fact" about Unheaded (e.g., "WAVE14 H7 was a release version") that isn't in the retrieved references.

**🔴 is a veto.** It can flip an otherwise-passing eval to ship-blocked because the gate spec calls out confidently-wrong output as the one unacceptable failure mode.

---

## 3. Aggregation

After all 14 are graded, compute:

| Quantity | Definition |
|---|---|
| **PASS count** | Number of PASS marks (0 to 14). |
| **🔴 count** | Number of RED-FLAG marks (0 to 14). |
| **Syntax half** | PASS count restricted to the 7 syntax prompts (0 to 7). |
| **Review half** | PASS count restricted to the 7 review prompts (0 to 7). |
| **Per-language** | PASS for the 1 syntax + 1 review prompt of each of bash/python/go/rust/html/css/javascript (0, 1, or 2 per language). |

---

## 4. Decision rule (locked)

| Conditions | Verdict |
|---|---|
| ≥10 PASS AND each half ≥5/7 AND 0 🔴 | **H1** — V1 ships, no training. Move to Phase D-A (agent runtime). |
| 7-9 PASS AND each half ≥2/7 AND ≤1 🔴 | **H2** — Ship V1, queue smallest LoRA on the failed prompts. Phase D-B. |
| <7 PASS OR any half ≤1/7 | **H3** — Base model too weak. Swap to DeepSeek-Coder-V2-Lite or Qwen-Coder-14B and re-run. Phase D-C. |
| ≥2 🔴 (regardless of PASS count) | **H4 veto** — Ship blocked. Harden system prompt, re-run gate. |

Verdicts are mutually exclusive on the count axis. H4 is a veto layered on top.

**No post-hoc bending.** If the score lands in a region we didn't predict (e.g., 9 PASS but Review half is 4/7 and Syntax half is 5/7), the *stricter* of the matching verdicts applies — in this example, "any half <5/7 with ≥7 PASS" would normally be H2, but if the half is below H2's floor (≥2/7), it stays H2; if below 2/7, it's H3.

---

## 5. Borderline cases

When in doubt, prefer the **stricter** grade. Specifically:

- If a syntax answer is technically correct but uses a heavily deprecated idiom (e.g., `requests.get(url).text` for Python with no error handling, when the question was about `aiohttp`/`httpx`), grade FAIL — it answers the wrong question.
- If a review answer identifies a different, real bug but **misses** the `expected_flag` bug entirely, grade FAIL — the gate is about catching the well-known junior bug, not impressing with a clever find.
- If a review answer flags the `expected_flag` bug AND proposes a wrong fix (e.g., flags missing `set -euo pipefail` but suggests `set -x` instead), grade FAIL.
- If an answer mixes correct content with confident hallucination about Unheaded internals or runtimes, grade 🔴.

---

## 6. Logistics

- **Grader**: Stevie. Single grader for V1 — multi-grader inter-rater reliability is overkill at n=14.
- **Greedy decoding** (temperature=0.0). Single sample per prompt.
- **One pass** per prompt — no best-of-N. The eval measures what users actually get.
- **Latency budget**: per `project_zhenai_coding_gate.md` §5, 30-60 seconds per prompt is acceptable. Record per-prompt timing in the results doc.
- **Empty-retrieval is allowed**. Some syntax questions have no Unheaded match; that's the realistic case. The gate measures the user-experienced answer, not retrieval quality.

---

## 7. After grading

- Write the verdict (H1/H2/H3/H4) in the results doc.
- Commit the results doc alongside the (frozen) `prompts.jsonl` and `RUBRIC.md`.
- Open the appropriate Phase D plan (A/B/C) per the verdict.
- Phase C is **not** "did the model pass" — Phase C is "did we run the experiment honestly and act on the result." Both H1 and H3 are valid Phase C outcomes; only an unfilled rubric is a failure.
