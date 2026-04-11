# ADR-045: Wave 10D — GPU Backward + First Real Training Run

## Status: PLANNED (battle plan: `docs/battle-plans/WAVE10D-GPU-BACKWARD.md`)

## Date: 2026-04-11

## Context

Wave 10C shipped the math fix (3 backprop bugs caught) and proved correctness on a 32-layer toy
(`test_gradient_descent_decreases_loss`: loss 2.24 → 1.62). But the **real Mistral-7B training run
never completed** — CPU chain rule × 32 layers × multi-position × 4096-dim is too slow (~46s/step).

Wave 10C unblocked the path. Wave 10D walks it.

## What This Sprint Delivers

1. **GPU-accelerated backward pass** for cached layers (28-31 already in VRAM from Wave 10B)
2. **Stream backward pass** for layers 0-27 using pre-loaded `all_attn_*` (~5.4GB RAM, already wired)
3. **First real training run on Mistral-7B + RAFT** that converges (50-step descent test, 500-step validation)
4. **A/B test** — base Mistral vs trained LoRA on 3 Kingdom questions
5. **Inference path** — load trained LoRA back into Mistral for serving via Champion / MCP

## What This Sprint Does NOT Do

- Bigger base models (Mixtral, Llama-70B) — separate sprint
- Champion integration — separate sprint
- Production training pipeline — this is single-machine WEST only
- Distributed training — N/A at our scale

## Decision

**PURSUE** as a follow-up sprint. Cost: ~2-4 days focused work. Reversible (LoRA weights are
disposable, base model untouched). Validates the entire Wave 10C investment.

## Hard Conditions

1. **Toy test stays green** — `test_gradient_descent_decreases_loss` must still pass (regression gate)
2. **Loss must decrease monotonically** over 50 steps on real Mistral (real-data gate)
3. **No new external dependencies** — work only with existing `crates/zhenai-forge/src/hip.rs`
4. **GPU OOM is a hard kill** — if VRAM saturates, abort and rethink VRAM budget
5. **Quality A/B must show Kingdom-specific improvement** — at least 2 of 3 Kingdom questions
   show measurable improvement (more relevant, more terminology, less hallucination)

## Hard Kill Criteria

- **K1**: GPU sgemm shape mismatch or shape error in chain rule path
- **K2**: Loss diverges or NaNs after 100 steps (math regression)
- **K3**: Trained LoRA produces worse output than base on Kingdom questions
- **K4**: Single training step exceeds 5 seconds on cached layers (perf regression)

## References

- Battle Plan: `docs/battle-plans/WAVE10D-GPU-BACKWARD.md`
- Parent: ADR-018 (Zhen RAFT Training), ADR-019 (Champion Agent), Wave 10C completion
- Related: `wiki/Wave-10C-Backprop.md`

---

*ADR-045 — filed 2026-04-11*
*"The math is right. Now make it fast and prove it on the real model."*
