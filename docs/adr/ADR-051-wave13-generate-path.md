# ADR-051: WAVE13 — Forge generate-gemma4 path + KV-cache deferral

**Status**: Draft (pending WAVE13 Phase 2 verdict)
**Date**: 2026-04-27 (drafted from Cowork-on-Macbook)
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

**[PENDING — fill in after WAVE13 Phase 2 quality run; see `crates/zhenai-forge/notes/wave13-phase2-quality.md`]**

The decision will be one of:

- **SHIP**: LoRA quality is sufficient for the wiring phase (WAVE13 Phases 4-5: forge HTTP serve + zhen-inference shim). KV-cache deferred to WAVE14.
- **RETRAIN**: under-trained per the WAVE13 Phase 1 hypothesis. WAVE14 priority shifts to "more epochs first, KV-cache second."
- **RANK-UP**: rank-16 LoRA insufficient capacity. WAVE14 priority shifts to rank=32/64 retraining.
- **DATA-FIX**: training data has issues; WAVE14 starts with corpus re-curation before any further training.

Independent of the verdict above, **KV-cache is deferred to WAVE14**. Correctness first; serving throughput later.

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

### Conditional (depends on Phase 2 verdict)
- **If SHIP**: WAVE13 Phases 4-5 unblock immediately. WAVE14 starts with KV-cache.
- **If RETRAIN**: WAVE14 starts with extended-epoch training. Phases 4-5 of WAVE13 paused.
- **If RANK-UP**: WAVE14 starts with rank-32/64 training. ADR-050's GPU-resident matrices resized.
- **If DATA-FIX**: WAVE14 starts with corpus audit. Could expand into a multi-week effort.

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

- [ ] Captain — verdict aligns with Track A/B/C decision (see `docs/decisions/2026-04-29-track-call.md`)
- [ ] Developer — implementation matches commit `5d413699` and any Phase 2 fixes
- [ ] Computermancer — KV-cache deferral acknowledged; WAVE14 starting point confirmed
- [ ] Scientist — Phase 2 numbers (win-rate, CE alignment) verified
- [ ] RFC-Editor — ADR text reviewed; status flipped to Accepted

---

*Drafted 2026-04-27 from Cowork-on-Macbook. Decision section pending remote Phase 2 execution per `WAVE13-PHASE2-REMOTE-PACKET.md`.*
