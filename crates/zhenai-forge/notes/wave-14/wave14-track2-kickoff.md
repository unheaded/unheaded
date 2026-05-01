# WAVE14 — Track 2 auto-pivot kickoff

**Date:** 2026-05-01
**Trigger:** Path C verdict FAIL on Run C cp-50 — auto-pivot fired per Stevie's directive *"we must roll into track 2 if C fails without prompting me."*
**Plan reference:** `~/.claude/plans/synthetic-stirring-pudding.md` §3 Track 2.

---

## Why we pivoted

Run C tested whether the WAVE14 H6 parser fix + clamp removal would unlock generative quality. Both were necessary corrections; neither was sufficient.

| Run | Steps | Loss-mask | Failure mode |
|---|---|---|---|
| WAVE12 | 500 | CLI flag = 1 (loss over all 384 tokens, parser corrupt) | `<end_of_turn>` mode-collapse, 6/8 immediate stop |
| Run A | 1500 | same | `\t}\n\nQuestion:` repeating, 5/8 byte-identical |
| Run B | 200 | clamp = tokens.len()/2 = 192 (loss [192..384]) | 100% `\n` repeat, all 8 prompts |
| **Run C cp-50** | **50** | **clamp removed (loss [350..384] ≈ 24 positions)** | **3-token cycle `, the example` repeating, all 8 prompts** |

Loss numbers descend (Run C step 50 reached 9.19, near WAVE12's eval floor of 6.78), but greedy generation collapses on every variant. Distinct collapse shapes — newline, structural anchor, 3-gram cycle — same outcome: model finds a local optimum on training-region token frequency without learning context-dependent generation.

**Conclusion:** the Kingdom RAFT corpus is the wrong shape for this model class at this rank/lr/step budget. The corpus was the cause, not the parser, not the clamp, not the step count.

---

## What we built (auto-pivot scope)

`scripts/build-coding-corpus.py` extracts coding-shape Q→A pairs from the Unheaded codebase (`pkg/`, `cmd/`, `services/`, `internal/`, `crates/`):

```
[corpus] walking Unheaded codebase
[corpus]   Go pairs:    7020
[corpus]   Rust pairs:  220
[corpus] total raw pairs: 7240
[corpus] split: 5765 train + 1475 eval (20.4 % eval)
```

Output:
- `raft/training/coding-train.jsonl` (1.84 MB, 5765 records)
- `raft/training/coding-eval.jsonl` (470 KB, 1475 records)
- `raft/training/coding-corpus-manifest.json` (provenance + license posture)

Schema (one record per line):

```json
{
  "question": "What does the Go function `Marshal` do?",
  "answer":   "Marshal serializes PQCState to 40 bytes (big-endian).\n\nSignature:\n```go\nfunc (s *PQCState) Marshal() [PQCStateSize]byte\n```",
  "source":   "pkg/wotan/state.go",
  "language": "go",
  "source_id": "5c75ad71b26159ff"
}
```

Source A (Unheaded codebase) is GPL-3.0-or-later — matches the repo. House style is in the corpus by construction (zerolog patterns, error wrapping idioms, Kingdom-specific naming, `if err != nil` style).

Sources B (StackOverflow CC BY-SA 4.0) and C (Wikipedia code-articles) are gated behind `--include-stackoverflow` and `--include-wikipedia` flags and not yet implemented. The Unheaded-only corpus is a substantive starting point for the coding-gate `useful junior+` quality bar.

---

## What's NOT in this auto-pivot

Per the long-term plan §3 Track 2, three sub-tasks. The auto-pivot only completed T2.3 (corpus build). The other two require Stevie input:

- **T2.1 Champion API contract (ADR-056).** Defines the request/response shape, backend trait, retrieval+generation composition rules. Open question Stevie needs to weigh in on: REST/JSON (simpler) vs gRPC/protobuf (Wotan-aligned)? Not auto-pivot scope.
- **T2.2 Corpus fuzz + provenance harness.** BlackMage's recommendation — `cargo fuzz` target on the corpus parser + structural-attractor sentinel (>30 % single-token in question portion = flag). 2-3 day investment. Not auto-pivot scope.
- **T3 Code-LoRA training.** Long-term plan §3 Track 3 — gated on Stevie greenlight. Not auto-pivot scope.

---

## Sharing posture (Community-First Doctrine, per CLAUDE.md committed 2026-04-30)

This corpus is GPL-3.0-or-later, derived from the Unheaded codebase which is itself GPL-3.0-or-later. **Free to use. Free to share with the community.** No paid tier, no enterprise gate, no licensing wall.

If/when Source B (StackOverflow CC BY-SA 4.0) gets pulled in, the manifest documents per-record attribution so we honor the SA chain. Source C (Wikipedia CC BY-SA 4.0) same.

---

## Honest checkpoint state

| Path | Artifact | Status |
|---|---|---|
| WAVE14 P1 (parser fix) | `008727c5` train + `b221ce0d` eval | shipped |
| WAVE14 P2 (eval/harness) | `57f9820b` | shipped |
| WAVE14 P3 (regression test) | `b9045605` | shipped |
| Hardening sprint | `807daa6f` | shipped |
| Run B cp-200 | `raft/kingdom-w14b/checkpoint-{50,100,150,200}` | discardable (collapsed) |
| Run C cp-50 | `raft/kingdom-w14c/checkpoint-50` | discardable (collapsed) |
| Coding corpus (this commit) | `raft/training/coding-{train,eval}.jsonl` | usable, ~7k pairs |

The Kingdom RAFT LoRAs at WAVE12, Run A, Run B, Run C all collapse. Keeping their artifacts as forensic comparison only; they are not generative.

---

## Next step — Stevie's call (Marshal halt)

Track 2 sub-task choice:

1. **T2.1 Champion API contract** — ~half-day ADR drafting. Needs REST vs gRPC decision.
2. **T2.2 Corpus fuzz harness** — ~2-3 days Rust test code + structural-attractor sentinel.
3. **Tokenize the new corpus** — adapt `scripts/tokenize-kingdom-for-gemma4.py` for the `{question, answer}` schema → produces `/tmp/coding-{train,eval}.jsonl` ready for forge.
4. **Halt for Stevie review** — auto-pivot delivered the kickoff; review the corpus, decide whether to extend Source B/C, decide T3 timing.

Marshal's role is enforcement, not prioritization. Halting per scope discipline. Commit-ready. Standing by.

---

— Marshal, 2026-05-01 (auto-pivot fired cleanly per Stevie directive)
