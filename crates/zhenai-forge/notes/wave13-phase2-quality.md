# WAVE13 Phase 2 Quality Report — SKELETON (pending remote run)

> **Pending remote execution.** This file is a skeleton forged 2026-04-27 from
> Cowork-on-Macbook. Fill in after running `docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md`
> on a Linux dev box with GPU + Gemma-4 GGUF + Kingdom LoRA.

**Date executed**: <YYYY-MM-DD>
**Executor**: <name>
**HEAD**: <git rev at execution time>
**Forge binary**: target/release/zhenai-forge (release build)
**Source-of-truth plan**: `docs/battle-plans/WAVE13-INFERENCE.md` (in-tree per ADR-052)

---

## Per-prompt rows (8 held-out Kingdom prompts)

### Prompt 1
- **Source tokens.json**: /tmp/wave13-phase2/p1.tokens.json
- **First 80 chars decoded**: "<fill in>"
- **Base output (decoded)**: "<fill in>"
- **LoRA output (decoded)**: "<fill in>"
- **Qualitative tag**: [coherent | mode-collapsed | multilingual-noise | gibberish | kingdom-relevant]
- **LoRA better than base?**: [Y / N / TIE]
- **Notes**: <fill in>

### Prompt 2
- **Source tokens.json**: /tmp/wave13-phase2/p2.tokens.json
- **First 80 chars decoded**: "<fill in>"
- **Base output (decoded)**: "<fill in>"
- **LoRA output (decoded)**: "<fill in>"
- **Qualitative tag**: [coherent | mode-collapsed | multilingual-noise | gibberish | kingdom-relevant]
- **LoRA better than base?**: [Y / N / TIE]
- **Notes**: <fill in>

### Prompt 3
- **Source tokens.json**: /tmp/wave13-phase2/p3.tokens.json
- **First 80 chars decoded**: "<fill in>"
- **Base output (decoded)**: "<fill in>"
- **LoRA output (decoded)**: "<fill in>"
- **Qualitative tag**: [coherent | mode-collapsed | multilingual-noise | gibberish | kingdom-relevant]
- **LoRA better than base?**: [Y / N / TIE]
- **Notes**: <fill in>

### Prompt 4
- **Source tokens.json**: /tmp/wave13-phase2/p4.tokens.json
- **First 80 chars decoded**: "<fill in>"
- **Base output (decoded)**: "<fill in>"
- **LoRA output (decoded)**: "<fill in>"
- **Qualitative tag**: [coherent | mode-collapsed | multilingual-noise | gibberish | kingdom-relevant]
- **LoRA better than base?**: [Y / N / TIE]
- **Notes**: <fill in>

### Prompt 5
- **Source tokens.json**: /tmp/wave13-phase2/p5.tokens.json
- **First 80 chars decoded**: "<fill in>"
- **Base output (decoded)**: "<fill in>"
- **LoRA output (decoded)**: "<fill in>"
- **Qualitative tag**: [coherent | mode-collapsed | multilingual-noise | gibberish | kingdom-relevant]
- **LoRA better than base?**: [Y / N / TIE]
- **Notes**: <fill in>

### Prompt 6
- **Source tokens.json**: /tmp/wave13-phase2/p6.tokens.json
- **First 80 chars decoded**: "<fill in>"
- **Base output (decoded)**: "<fill in>"
- **LoRA output (decoded)**: "<fill in>"
- **Qualitative tag**: [coherent | mode-collapsed | multilingual-noise | gibberish | kingdom-relevant]
- **LoRA better than base?**: [Y / N / TIE]
- **Notes**: <fill in>

### Prompt 7
- **Source tokens.json**: /tmp/wave13-phase2/p7.tokens.json
- **First 80 chars decoded**: "<fill in>"
- **Base output (decoded)**: "<fill in>"
- **LoRA output (decoded)**: "<fill in>"
- **Qualitative tag**: [coherent | mode-collapsed | multilingual-noise | gibberish | kingdom-relevant]
- **LoRA better than base?**: [Y / N / TIE]
- **Notes**: <fill in>

### Prompt 8
- **Source tokens.json**: /tmp/wave13-phase2/p8.tokens.json
- **First 80 chars decoded**: "<fill in>"
- **Base output (decoded)**: "<fill in>"
- **LoRA output (decoded)**: "<fill in>"
- **Qualitative tag**: [coherent | mode-collapsed | multilingual-noise | gibberish | kingdom-relevant]
- **LoRA better than base?**: [Y / N / TIE]
- **Notes**: <fill in>

---

## Aggregate

- **Win-rate (LoRA-better count / 8)**: ___ / 8 = ___%
- **Mean qualitative shift**: <one-line summary>
- **CE-vs-quality alignment**: WAVE12 reported held-out CE Δ −14.32. Does qualitative output match that magnitude? [yes | no | partial]
- **Mode-collapse incidents**: ___ / 8
- **Kingdom-relevant LoRA outputs**: ___ / 8
- **Kingdom-relevant base outputs**: ___ / 8

---

## Decision (WAVE13 Phase 3)

**Verdict**: [SHIP | RETRAIN | RANK-UP | DATA-FIX]

**Rationale** (cite specific numbers from Aggregate):
- <fill in>

**Owner of next action**: <name>
**Next step**: <one paragraph — concrete next move>

---

## Notes from Phase 1 commit (cached for context)

The WAVE13 Phase 1 commit (`5d413699`, 2026-04-25) explicitly flagged:

> EARLY QUALITY FINDING (informs Phase 2 verdict ahead of formal test):
> Pipeline works end-to-end, but the LoRA is *under-trained*, not memorized
> or broken. Math:
>   - WAVE12 eval CE 6.78 → mean P(target|context) ≈ 0.001 = 0.1%.
>   - Confident generation needs P > 0.5 → CE < 0.7. We're 10× off.
>   - Greedy on a diffuse distribution picks long-tail tokens → gibberish or
>     mode-collapsed repetition (saw "if if if" with LoRA on Kingdom prompt;
>     saw multilingual noise on base prompts).
>   - 500 steps × 3568 examples ≈ 14% of one epoch. Real RAFT runs use 48K+
>     steps.

**Hypothesis to confirm/falsify in Phase 2**: the verdict will most likely be **RETRAIN** unless the qualitative output is surprisingly more Kingdom-relevant than the under-training math predicts.

If the formal Phase 2 numbers strongly contradict the early hypothesis, that itself is a finding worth recording.
