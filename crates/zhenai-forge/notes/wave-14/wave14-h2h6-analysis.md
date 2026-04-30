# WAVE14 — H2 + H6 Combined Analysis (joint Micromanager + Scientist ruling)

**Date:** 2026-04-28
**Corpus under test:** `/tmp/24h-kingdom-train.jsonl` (3568 records, 384 tokens each, with per-example `answer_start`)
**Script:** `/tmp/h2-h6-analysis.py`
**Runtime:** ~30 s, no GPU.

---

## TL;DR

| Hypothesis | Pre-registered threshold | Actual result | Verdict |
|---|---|---|---|
| **H2 — corpus structural leakage** ("Question:" / "}" as answer opener) | "Question:" >10% openers | "Question:" not in top-200 openers; 75% of answers open with " The" (natural English) | **FALSIFIED for "Question:"** ; but corpus is good |
| **H6 — parser absorbs `answer_start` JSON field as a stray token** | 90%+ of records | **100% of records (50/50)** | **CONFIRMED** |

**Implication:** the "Question:" attractor in WAVE14 Run A is **not** corpus-shape leakage. The corpus is clean — 75% of answers open with " The" (token 818, expected English explainer pattern). The model produced an attractor that does NOT exist in the corpus, which means the corruption came from somewhere else in the training pipeline.

**H6 is the parsing bug ADR-050 negative section warned about, and it has been training on corrupted data for ~11 days (since `cmd_train_gemma4` shipped in WAVE10F on 2026-04-17).** Every training example feeds the model `[real_tokens..., answer_start_int]` and the LoRA learns to predict the trailing integer.

---

## H2 result (corpus opening-token frequency)

Top-5 opening tokens at `tokens[answer_start]` across 3568 records:

| token_id | decoded | count | pct |
|---|---|---|---|
| 818 | ` The` | 2685 | **75.25 %** |
| 236776 | `A` | 87 | 2.44 % |
| 10450 | `According` | 79 | 2.21 % |
| 4420 | `When` | 75 | 2.10 % |
| 2267 | `An` | 73 | 2.05 % |

`Question:` first token = 14977. Does **not** appear in the top-200 openers (count < 8 = below 0.22 %).
`\t}` first token = 255968. Does **not** appear in the top-200 openers either.
`<end_of_turn>` (EOS, token 1) does not appear as an answer opener.

**Conclusion:** corpus answers open with natural English explainer prefixes (" The", "A", "According", "When", "An" = 84.05% combined). The corpus is **NOT** the source of the "Question:" attractor.

---

## H6 result (cmd_train_gemma4 line-parser corruption)

Parser at `crates/zhenai-forge/src/main.rs:126-137`:

```rust
let toks: Vec<u32> = line
    .split(|c: char| !c.is_ascii_digit() && c != '-')
    .filter(|s| !s.is_empty())
    .filter_map(|s| s.parse().ok())
    .collect();
```

This regex splits on every non-digit character, which includes `}`, `,`, `"`, `:`, etc. So the JSON line:

```json
{"tokens":[1713,96300,...,107],"answer_start":360}
```

becomes the integer sequence `[1713, 96300, ..., 107, 360]` — the trailing integer 360 (the JSON `answer_start` field) is silently appended to the token vector.

**Audit on first 50 records:**
- 50/50 records: `parsed.len() == tokens.len() + 1` AND `parsed[-1] == answer_start`
- **100 % corruption rate**

**Demonstration on record 0:**
- `json["tokens"][-5:]    = [236771, 236812, 236761, 106, 107]`
- `rust_parser(line)[-5:] = [236812, 236761, 106, 107, 360]`
- `json["answer_start"]   = 360`

The proper `<end_of_turn>` ending token 106 + newline 107 is followed by a stray integer 360, which the model is then asked to predict.

**Compounding bug — `--answer-start` flag is unused per-example:**
The training loop at `main.rs:189-196` uses the **CLI flag** value (default `1`), not the per-example JSON `answer_start`. Combined with H6, every example is trained as:
- `effective_answer_start = 1` (so loss is applied across **all 384 tokens including the question**, not just the answer)
- last training target = `answer_start` integer (the stray-token bug)

The model is therefore being asked to:
1. Predict the question (which it shouldn't — questions aren't generated, they're conditioned on)
2. Predict an arbitrary integer as the final token after `<end_of_turn>` (which is structural noise)

`answer_start` value distribution across 3568 records is concentrated in 340–380 (top values: 355, 357, 345, 350, ...), so the model has learned a high-confidence prior that "after `\n` (107), emit some integer in 340–380 range". When sampled greedily at generation time, this likely interacts pathologically with `<end_of_turn>` and produces the structural attractor we see.

---

## Why "Question:" specifically? — FULLY EXPLAINED

Followup scan of token 14977 (`Question:`) prevalence in **question** portions of training records:

- **99.8 % of records (3562/3568)** contain token 14977 in the question portion.
- **0 / 3568 records** contain it in the answer portion.
- It appears at positions 300–330 (mid-question, before the answer at `answer_start ≈ 340–380`).

This is the universal **RAFT-corpus structural marker** — every training example is shaped like:
```
Document: …
Question: <the actual question>
[answer_start]
The <answer text>
<end_of_turn>
```

### The full root-cause chain

1. **H6 — line parser** (`main.rs:126-137`) absorbs `answer_start` integer as a stray training target.
2. **`--answer-start` flag default = 1** ignored per-example JSON, so `effective_answer_start = 1.min(385/2) = 1` and **loss is applied across all 384 tokens including the entire question portion**.
3. The loss therefore trains the model to predict the question, including the high-frequency literal "Question:" token at position ≈ 320 in every example.
4. After 1500 steps × 3568 examples, the model has very high prior on:
   - `\t}\n` — code-style sequence endings inherited from question-portion code blocks
   - `Question:\n` — the universal corpus marker
   - The `answer_start` integer — H6 stray target
5. At generation time, after `<end_of_turn>` (token 106), the model collapses onto the highest-prior tokens in its training distribution. Greedy decode → repeating `\t}\n\nQuestion:\n` cycle.

**This is not undertraining. This is training the model on the wrong objective.** WAVE12's eval Δ −14.32 was real, but it measured how well the LoRA can predict question structure, not how well it generates Kingdom answers. The CE descent is a *side-effect* of mass-fitting to high-frequency structural tokens.

---

## Recommended fix path

### P0 — Parser fix (1 hour)

Replace the naive line parser with `serde_json::from_str`:

```rust
#[derive(serde::Deserialize)]
struct TrainExample {
    tokens: Vec<u32>,
    #[serde(default)]
    answer_start: Option<usize>,
}
```

Then plumb `answer_start` per-example through `run_loop` and into `backend.train_step`. This is a focused refactor — touches `cmd_train_gemma4`, `cmd_eval_gemma4`, and the `train_step` signatures (which already accept `answer_start: usize`).

### P0 — `--answer-start` semantics fix

Make the CLI flag the **fallback default** when the JSON record has no `answer_start`. When the JSON has `answer_start`, **prefer the JSON value**.

### P1 — Run B (corrected parser, 1500 steps)

After the fixes:
1. Smoke test: train 10 steps, verify train-step loss drops monotonically and the per-step token count matches `tokens.len()` from JSON (no off-by-one).
2. Run B: 1500 steps with corrected parser + per-example `answer_start`.
3. Re-eval with the same 8-prompt slice as WAVE13 Phase 2 + Run A.

If Run B's outputs open with " The" (token 818) on the majority of prompts, **the parser bug was the root cause** and undertraining (H1) was a co-conspirator at most.

### P2 — Audit eval path

`cmd_eval_gemma4` uses the **same line parser** at `main.rs:393-461` (per the grep result). Eval CE numbers may also be affected. The WAVE12 held-out eval Δ −14.32 may need re-running once parser is fixed.

---

## ADR / memory updates needed

- **ADR-051** — append "WAVE14 Run A finding" subsection: H1 directionally confirmed (broke WAVE12 attractor) but H6 dominates. Verdict adjusted from RETRAIN → **PARSER-FIX-THEN-RETRAIN**.
- **ADR-050 negative section** — promoted from "warning" to "confirmed root cause of WAVE14 mode-collapse". Should reference this analysis.
- **Memory entry** `project_wave14_h6_root_cause.md` — long-term: "the line parser bug was real and shipped. Always validate JSONL parsing end-to-end on the real training data."

---

## Estimate to "functional zhenai" gate

- Parser fix + smoke test: **1 h**
- Run B (1500 steps, corrected): **30 min** (per WAVE12 ADR-050 GPU-resident chain)
- Re-eval: **15 min**
- Total: **~2 hours wall-clock to know if this fixes it.**

Smaller than Run A alone (which was 1 h training + 25 min eval). Recommended over Run B-on-broken-parser.

---

**Joint MICROMANAGER + SCIENTIST ruling:** before any GPU training, fix the parser AND the `--answer-start` semantics. The data has been quietly broken for 11 days (since `cmd_train_gemma4` shipped in WAVE10F on 2026-04-17) — WAVE10F, WAVE12, and Run A all trained on this corruption. WAVE12's eval Δ −14.32 was a measure of structural-token prior fit, not generation skill, which is why Phase 2 generation failed catastrophically.

Standing by for Stevie's go/no-go on parser fix + answer_start plumbing.
