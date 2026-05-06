# ADR-030: Zhenai Forge — Custom Rust LoRA Training on Heterogeneous Compute

**Status:** ACCEPTED — kingdom-v5 training running (2026-04-06). Wave 10 critical bugs fixed: format_prompt was dead code (train.rs loaded raw JSON, never called format_prompt — all v1-v3 trained on JSON syntax), gradient accumulation never zeroed after Adam step. V5: 16309 QA pairs, RAFT prompt format, accum=4, cosine LR, step 13150/16304, loss 0.047. Scientist overfitting warning — test epoch 1 checkpoint before epochs 2+3.  
**Date:** 2026-04-03

> Wiki stub generated 2026-05-06 by Marshal overnight sweep (NORTH-STAR Appendix A B5). See the canonical ADR for full text and rationale.

## Canonical

[docs/adr/ADR-030-zhenai-forge-rust-training.md](../docs/adr/ADR-030-zhenai-forge-rust-training.md)

## Cross-references

- [ADR Index](ADR-Index.md)
- [Architecture overview](Architecture.md)
