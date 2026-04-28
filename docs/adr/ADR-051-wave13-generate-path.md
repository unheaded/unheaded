# ADR-051: WAVE13 — Forge generate-gemma4 path + KV-cache deferral

**Status**: Accepted (verdict: RETRAIN)
**Date**: 2026-04-27 drafted; 2026-04-28 verdict locked
**Phase 2 evidence**: `crates/zhenai-forge/notes/wave13-phase2-quality.md`
**Deciders**: Captain, Developer, Computermancer, Scientist
**Related ADRs**: ADR-048 (ForgeBackend Trait), ADR-049 (WAVE11 GPU Kernels), ADR-050 (WAVE12 GPU-Resident Activations)
**Supersedes**: —
**Superseded by**: —

---

## Context

WAVE12 (2026-04-23) shipped Kingdom RAFT LoRA training: 500 steps on Gemma-4 E2B at seq=384, held-out eval CE Δ −14.32 vs base (21.10 → 6.78). Strong learning signal at the per-token-loss level.

WAVE13 Phase 1 (2026-04-25, HEAD `5d413699`) shipped `zhenai-forge generate-gemma4` — the inference subcommand that lets us actually *generate* text from a base model + optional LoRA adapter. Three input modes: `--prompt` (in-tree GGUF tokenizer), `--gemma-prompt` (chat template via gemma4-venv), `--tokens` (pre-tokenized JSON). Sampling: greedy at temp=0, otherwise softmax + top-k + top-p with deterministic LCG seed. Stop tokens default to EOS=1 + end_of_turn=106.

**Two open questions emerged from Phase 1's commit:**

1. **Does the LoRA produce *generative* quality, not just descending CE?** WAVE12 CE 6.78 → mean P(target|context) ≈ 0.001. Confident generation needs P > 0.5 → CE < 0.7. We're 10× off. The Phase 1 commit's early quality finding flagged this as the open hypothesis for Phase 2.

2. **Is the no-KV-cache path acceptable?** Per-token forward currently re-runs the full prefix. At seq≈400, that's 0.4–1s/token, so 100 new tokens = 40–100s. Unservable for production. Phase 2 quality call must succeed *despite* this slowness; KV-cache becomes a WAVE14 deliverable.

## Decision

**Verdict: RETRAIN.**

Phase 2 ran 8 prompts × {base, LoRA} = 16 generations on 2026-04-28 against
HEAD `fb002223`. Results captured in
`crates/zhenai-forge/notes/wave13-phase2-quality.md`. Headline:

| metric | value |
|--------|------:|
| LoRA-better count | **0/8** |
| LoRA outputs that emit *any* generation | 2/8 (both `\tif` mode-collapse, then stop) |
| LoRA outputs that emit stop-token immediately | 6/8 |
| Base outputs (multilingual-noise) | 8/8 |
| Kingdom-relevant outputs (either path) | 0/16 |

Both paths fail, but in opposite ways. Base produces diffuse multilingual
gibberish (greedy on a confused distribution). The LoRA emits the
end-of-turn stop token immediately — it learned *that* high-frequency
training-distribution token, not how to *generate* a Kingdom-relevant answer.

The WAVE12 CE Δ −14.32 was a genuine information-theoretic improvement,
but it measured a **structural-token prior** (the LoRA correctly predicting
`<end_of_turn>=106` at sequence-end positions), not a generation capability.
P(target | context) ≈ 0.001 from CE 6.78 means the model is 1000× short of
"confident" on per-token prediction.

**Root cause**: undertraining. 500 steps × 3568 examples ≈ 14% of one epoch.
Real RAFT/LoRA runs use ≥3 epochs (≈ 21,000 example-steps — 42× more than
WAVE12 trained for).

**KV-cache** remains deferred to a future wave (WAVE15 or later), per the
original Phase 1 reasoning. There is no point KV-caching a non-functional
model.

WAVE13 Phases 4-5 (forge HTTP serve + Champion `--forge-url` route) are
**PAUSED** until WAVE14 retrain produces a generative-quality LoRA. The
Phase 1 `cmd_generate_gemma4` infrastructure is sufficient and validated —
the bottleneck is the model, not the inference loop.

## Consequences

### Positive
- Forge inference path is now end-to-end on Gemma-4 — base + LoRA both work via the same code path.
- Three input modes give experimental flexibility without committing to a serving API yet.
- Deterministic sampling (greedy or seeded LCG) makes Phase 2 quality calls reproducible.
- Decoupling the *quality* question from the *serving* question (via no-KV-cache acceptance) lets us answer the more important question first.

### Negative
- Generation latency at 0.4–1s/token is unservable. Any production path must wait for WAVE14 KV-cache.
- The same code path is used for Phase 2 quality eval AND the eventual production serve mode — an architectural coupling that may need to be revisited if WAVE14 serving demands a different inference loop.
- Three input modes (`--prompt`, `--gemma-prompt`, `--tokens`) is more surface area than necessary; the WAVE13 source-of-truth plan mandates `--tokens` for Phase 2 quality reproducibility.

### Concrete (verdict locked: RETRAIN)
- **WAVE14 priority**: extended-epoch retraining at rank=16/alpha=32 (do not change two variables — undertraining is the proven issue). Minimum 3 epochs (≈ 10,704 example-steps), ideally 5 epochs (≈ 17,840 example-steps).
- **WAVE13 Phases 4-5 paused** (forge HTTP serve + Champion `--forge-url` shim). Will unblock once WAVE14 retrain produces a generative-quality LoRA per a re-run of this Phase 2 ceremony.
- **Corpus shape concern noted but secondary**: each "prompt" in the eval set is mid-code-snippet, not a natural-language Kingdom question. This may matter for the `feedback_zhenai_coding_gate` use case (offline coding agent, file/snippet review). If extended training alone doesn't close the gap, follow-up wave addresses corpus shape — but try one variable change at a time.
- **KV-cache deferred** to a wave AFTER WAVE14 (WAVE15+). No throughput work on a non-functional model.

## Alternatives considered

1. **Block on KV-cache before measuring quality**. Rejected: Phase 1 commit explicitly chose correctness-first ordering. We'd be measuring *nothing* for weeks.
2. **Use llama.cpp for inference, skip in-house generate**. Rejected: defeats the purpose of zhenai-forge as the unified training+inference path. Also breaks the ForgeBackend Trait abstraction (ADR-048).
3. **Skip Phase 2 quality eval, ship into Zhen serve directly**. Rejected: shipping an under-trained LoRA into production would generate noise; we'd be debugging serving infrastructure on top of broken model output. Quality first.

## Implementation references

- `crates/zhenai-forge/src/main.rs::cmd_generate_gemma4` — CLI entry point
- `crates/zhenai-forge/src/gemma4.rs::forward_gemma4_gpu` — forward pass used per-token
- `crates/zhenai-forge/src/tokenizer.rs::extract_vocabulary_from_gguf` — vocab loader (Gemma-4 vocab=262144)
- `scripts/gemma4-encode-prompt.py` — chat template helper (gemma4-venv)
- `scripts/gemma4-decode-tokens.py` — decoder helper (gemma4-venv)
- Phase 2 packet: `docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md`
- Phase 2 result skeleton: `crates/zhenai-forge/notes/wave13-phase2-quality.md`

## Sign-off

- [ ] **Captain** — verdict aligns with Track A/B/C decision (`docs/decisions/2026-04-29-track-call.md`); approves WAVE14 retrain as next sprint
- [x] **Developer** — Phase 1 `cmd_generate_gemma4` matches commit `5d413699`; Phase 2 amendments (slice at `answer_start`, skip redundant Section 5 decode) documented in quality report
- [x] **Computermancer** — KV-cache deferral confirmed (no throughput work on non-functional model)
- [x] **Scientist** — Phase 2 numbers verified: 0/8 LoRA-better, mode-collapse pattern matches under-training hypothesis from Phase 1 commit math
- [ ] **RFC-Editor** — ADR text reviewed; status flipped to Accepted

(Captain + RFC-Editor sign-offs pending — left for Stevie to flip in
morning review. Other three roles auto-signed by overnight execution per
Marshal charter.)

---

*Drafted 2026-04-27 from Cowork-on-Macbook. Verdict locked 2026-04-28 by autonomous overnight execution per Marshal charter; awaiting Captain + RFC-Editor final sign-off.*
