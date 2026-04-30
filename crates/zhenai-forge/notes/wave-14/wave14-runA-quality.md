# WAVE14 Run A — Phase 2 re-eval

**Date:** 2026-04-28
**LoRA under test:** `raft/kingdom-w14a/kingdom.lora.gguf` (1500 steps, rank=16, alpha=32, on `/tmp/24h-kingdom-train.jsonl`)
**Prompts:** identical 8-prompt slice as WAVE13 Phase 2 (`/tmp/wave13-phase2/p[1-8].tokens.json`)
**Mode:** greedy, T=0.0, max_new_tokens=100
**Hypothesis under test:** **H1 — undertraining**. WAVE12 ran 500 steps × 3568 examples ≈ 14% of one epoch. Run A triples that to 1500 steps to test whether more training breaks the WAVE13 immediate-stop / mode-collapse pattern.

---

## Result summary

| | WAVE12 (500 steps) | Run A (1500 steps) |
|---|---|---|
| Immediate `<end_of_turn>` (6/8 in WAVE13) | 6/8 | **0/8** |
| `\tif` mode-collapse (2/8 in WAVE13) | 2/8 | 0/8 |
| Repeating `\t}\n\nQuestion:` attractor | 0/8 | **8/8** |
| Kingdom-quality answer | 0/8 | **0/8** |

**H1 verdict: PARTIALLY CONFIRMED, BUT INSUFFICIENT.**
3× more training broke the WAVE12 stop-token attractor (6/8 → 0/8). But it did NOT produce Kingdom-quality answers; it just substituted a *different* structural attractor. Per-prompt outputs all converge on the same repeating sequence:

```
\t}
\n
Question:\n
\t}
\n
Question:\n
…
```

This is a **5/8 IDENTICAL completion** (same byte sequence across 5 of 8 prompts; the other 3 differ only in number of repetitions before truncation at 100 tokens).

---

## What this rules in / out

- **H1 ALONE:** insufficient. More training shifts which token the LoRA collapses onto, but doesn't reach generative competence. Continued naive training (Run B at 3000 steps) is now low-prior — the loss-slope decay implies marginal returns are also dropping.
- **H2 (corpus shape):** elevated prior. The repeating attractor matches a *training-data structural artifact*. Specifically: code answers ending in `\t}` followed by literal "Question:" between examples is a corpus-stitching pattern, not a generation skill.
- **H6 (parser bug):** new hypothesis from ADR-050 negative section — `cmd_train_gemma4`'s naive line parser may absorb the JSON `answer_start` integer field into the token stream. If so, every training example may include a stray integer token. Could explain "Question:" attractor if the int decodes to that string range.
- **H4 (rank starvation), H5 (chat-template mismatch):** still possible but lower prior than H2/H6.

---

## Per-prompt outputs (raw, decoded last-line of each .txt file)

All 8 outputs in `wave14-runA-quality/lora-w14a-p[1-8].txt`. Pattern: every output ends with the repeating `\t}\n\nQuestion:\n` sequence, regardless of input prompt content.

---

## Next step (P1, Marshal-cleared)

Combined H2 + H6 Python analysis on `raft/training/train.jsonl`:

1. **H2 — opening-token frequency:** count distribution of FIRST token in answer portion of each training example. If `Question:` (or its first token) is >10% → corpus has structural leakage; if <2% → H2 falsified, H6 carries weight.
2. **H6 — parser-corruption check:** simulate `cmd_train_gemma4`'s line parser on the first 50 training records. Verify whether `answer_start` integer is absorbed into the token sequence. Pre-registered: if any record's tokens contain a literal stray integer at the position predicted by the parser bug, H6 fires.

ETA: ~10 min, no GPU. Stevie review of H2/H6 results decides Run B (more training) vs corpus fix vs parser fix.

**No GPU work proceeds until H2 + H6 results are in hand.**
