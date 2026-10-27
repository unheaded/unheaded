# Claude-Direct QA Generation for Zhenai Training

**Date**: 2026-04-05
**Status**: IN PROGRESS
**Output**: /var/zhen/distilled_qa_claude.jsonl
**ADR**: ADR-036

## What This Is

Claude Opus generates training QA pairs directly during development sessions.
Unlike the Mistral local distillation (auto-distill.sh), Claude actually
understands code architecture, design decisions, and cross-cutting concerns.
This produces higher-signal training data for Zhenai's LoRA fine-tuning.

## How It Works

1. Claude reads Kingdom source files (ADRs, docs, Go/Rust code, skills)
2. Generates QA pairs grounded in the actual content — not hallucinated
3. Appends to /var/zhen/distilled_qa_claude.jsonl (one JSON object per line)
4. Format matches zhenai-forge expectations: {"question", "answer", "source"}
5. Merged with other datasets for training

## Quality Differences

| Source | Generator | Quality | Count |
|--------|-----------|---------|-------|
| raft_dataset_combined.jsonl | Mistral-7B (self-generated) | Low | 3,965 |
| distilled_local_repo.jsonl | Mistral-7B (local distill) | Medium | ~3,000+ (running) |
| distilled_qa_claude.jsonl | Claude Opus (this file) | High | ~1,200 (target) |

## Progress

- [x] Batch 1: CLAUDE.md + core architecture (25 pairs)
- [ ] Batch 2: ADR-001 through ADR-010 (150 pairs)
- [ ] Batch 3: ADR-011 through ADR-020 (150 pairs)
- [ ] Batch 4: ADR-021 through ADR-038 (270 pairs)
- [ ] Batch 5: docs/doom/ (50 pairs)
- [ ] Batch 6: docs/lore/ (80 pairs)
- [ ] Batch 7: Key Go source (100 pairs)
- [ ] Batch 8: Key Rust source (80 pairs)
- [ ] Batch 9: Wotan service (100 pairs)
- [ ] Batch 10: Skills (200 pairs)
- [ ] Merge all datasets → raft_dataset_v4.jsonl
- [ ] Train kingdom-v4.zlora

## Target

~1,200 Claude pairs + ~3,000 Mistral pairs + 3,965 existing = ~8,000+ total

## Parallel Streams

- **auto-distill.sh** — Running unattended, processing 2,394 files via local Mistral
- **Claude sessions** — Generate high-quality pairs during active development
- **Future: Haiku API** — Batch run when API credits loaded (~$1.75 for 443 docs)
