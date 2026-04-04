# WAVE 8 BATTLE PLAN — Zhenai Forge Production Hardening

**Forged**: 2026-04-04
**Round Table**: BlackMage (lead), Scientist, Micromanager, Developer, Warmonger
**Prerequisite**: Wave 7 Steps 1-4 proven — loss decreases through real Mistral-7B weights
**Target**: Ship a kingdom.zlora that ACTUALLY improves Zhenai's inference quality

---

## PROGRESS (2026-04-04)
- [x] Phase 1: Tests green (34/34) ✓
- [x] Phase 2: Full 32K vocab + gradient clipping ✓
- [x] Phase 3: Scaled to 8 layers ✓ 
- [x] Phase 4: Q/K/V/O LoRA in forward + backward ✓
- [x] Phase 5: FFN (SwiGLU) in forward pass ✓
- [~] Phase 6: Production training — Epoch 1 DONE (14.33→9.26), Epoch 2 in progress
- [ ] Phase 7: Validate + deploy (blocked on Phase 6)

## ROUND TABLE ASSESSMENT

### What's PROVEN (do not re-do)
- GGUF reader: reads real model, mmap zero-copy ✓
- Tokenizer: extracts 32000 vocab from GGUF, greedy longest-match ✓
- Q5_K dequantization: verified correct ✓
- Q6_K dequantization: fixed zero-gradient bug ✓
- HIP GPU FFI: alloc, memcpy, hipBLAS sgemm verified ✓
- Forward pass: real logits from real weights ✓
- Backward pass: analytical gradients, chain rule works ✓
- Adam optimizer: weights update, loss decreases ✓
- Loss: 6.54 → 5.10 over 800 steps (REAL learning) ✓

### What's BROKEN (fix immediately)
- **5 tests failing** — B init change broke lora_forward test, GPU tests fail when GPU busy with training
- **Residual scaling removed** — may cause gradient explosion at scale
- **LR hardcoded in train.rs** — sed edits scattered, should use config
- **Debug prints left in** — need clean/conditional compile

### What's INSUFFICIENT for production (scale up)
- Only 100 of 32000 vocab tokens → model can only predict 100 tokens
- Only 4 of 32 layers → missing 87.5% of model capacity
- Only Q projection LoRA → missing K/V/O attention adapters
- No FFN layers → missing ~2/3 of each transformer layer
- No gradient clipping → potential NaN explosion at scale
- No checkpoint saving during training → lose progress on crash

---

## BLACKMAGE SECURITY REVIEW

Before shipping any .zlora file:
1. Verify no training data leaks into the adapter weights (memorization check)
2. Verify .zlora file format doesn't contain raw tokens or paths
3. Verify the training process can't be poisoned via malicious JSONL input
4. Verify GPU memory is properly freed (no VRAM leak across training runs)

---

## EXECUTION PLAN (ordered by impact, each step is independent commit)

### PHASE 1: FIX BROKEN TESTS (30 min)

- [ ] **1.1** Fix lora_forward test — B is no longer zero, update expected output
- [ ] **1.2** Fix GPU tests — add `if training_running { skip }` guard or run sequentially  
- [ ] **1.3** Remove debug print statements (or gate behind `--verbose` flag)
- [ ] **1.4** Restore config-driven LR (undo sed hacks, use TrainConfig properly)
- [ ] **1.5** Run full test suite — must be 34/34 green before proceeding
- [ ] **1.6** COMMIT: "fix: restore 34/34 tests after B init + Q6_K changes"

### PHASE 2: SCALE VOCAB (1 hour)

- [ ] **2.1** Change vocab_subset from 100 to full 32000
  - File: `src/train.rs` (CpuWeights::forward_loss + training loop)
  - This makes loss computation meaningful — model predicts real tokens
  - RAM impact: logits array grows from 400B to 128KB (trivial)
  - Speed impact: softmax over 32K instead of 100 (~32x slower per step)
- [ ] **2.2** Add gradient clipping (max_norm=1.0) to prevent NaN explosion
- [ ] **2.3** Verify loss starts near `ln(32000) ≈ 10.37` and decreases
- [ ] **2.4** COMMIT: "feat: full 32K vocab prediction"

### PHASE 3: SCALE LAYERS (1-2 hours)

- [ ] **3.1** Increase from 4 to 16 layers (streaming dequant, ~2GB more RAM)
- [ ] **3.2** Verify loss improves vs 4-layer baseline
- [ ] **3.3** If RAM allows, push to 32 layers
- [ ] **3.4** COMMIT: "feat: 16-32 layer forward pass"

### PHASE 4: ADD K/V/O LORA TARGETS (1 hour)

- [ ] **4.1** Dequantize attn_k, attn_v, attn_output weights per layer
- [ ] **4.2** Add LoRA forward/backward for K, V, O projections (not just Q)
- [ ] **4.3** This 4x the gradient signal — each layer has 4 LoRA targets
- [ ] **4.4** COMMIT: "feat: Q/K/V/O LoRA targets"

### PHASE 5: ADD FFN LAYERS (2 hours)

- [ ] **5.1** Dequantize ffn_gate, ffn_up, ffn_down per layer
- [ ] **5.2** Implement FFN forward: gate_proj → SiLU → up_proj → element_mul → down_proj
- [ ] **5.3** Add FFN to forward_loss and backward pass
- [ ] **5.4** COMMIT: "feat: FFN layers in forward/backward"

### PHASE 6: CHECKPOINT + PRODUCTION RUN (1 hour)

- [ ] **6.1** Save .zlora checkpoint every 500 steps
- [ ] **6.2** Add `--resume` flag to continue from checkpoint
- [ ] **6.3** Full production training: 3965 examples, 2 epochs, all layers, full vocab
- [ ] **6.4** Verify loss curve is smooth and converging
- [ ] **6.5** COMMIT: "feat: production training with checkpoints"

### PHASE 7: VALIDATE + DEPLOY (1 hour)

- [ ] **7.1** Load kingdom-real.zlora into llama-server (need .zlora → GGUF LoRA converter)
- [ ] **7.2** A/B test: 30 Kingdom questions, base Mistral vs fine-tuned
- [ ] **7.3** If >10% improvement: deploy as default
- [ ] **7.4** Update ADR-018, ADR-030 status to COMPLETE
- [ ] **7.5** COMMIT: "milestone: production kingdom.zlora deployed"

---

## CRITICAL PATH

```
Phase 1 (tests) → Phase 2 (vocab) → Phase 3 (layers) → Phase 6 (production run)
                                   → Phase 4 (K/V/O) → Phase 5 (FFN) ↗
```

Phases 2-5 are incremental improvements. Each produces a better .zlora.
Phase 6 can run after ANY of 2-5 (doesn't need all).
Phase 7 needs a .zlora from Phase 6.

**Minimum viable: Phase 1 + Phase 2 + Phase 6 = production .zlora with full vocab**
**Full quality: Phase 1-6 + Phase 7 = best possible .zlora from current architecture**

## ESTIMATED TIME

| Phase | Time | Blocking? |
|-------|------|-----------|
| 1. Fix tests | 30 min | YES — must be green |
| 2. Full vocab | 1 hr | YES — needed for meaningful training |
| 3. More layers | 1-2 hr | No — improves quality |
| 4. K/V/O LoRA | 1 hr | No — improves quality |
| 5. FFN layers | 2 hr | No — improves quality |
| 6. Production run | 1 hr | YES — produces the output |
| 7. Validate + deploy | 1 hr | YES — ships it |

**Minimum path: 3.5 hours (Phases 1, 2, 6, 7)**
**Full path: 7-8 hours (all phases)**

---

## EXECUTION RULES

1. Fix tests FIRST — no code changes until 34/34 green
2. Each phase is one commit with clear verification
3. Training runs in background while coding next phase
4. Kill stale training before starting new run
5. After Phase 7: update ALL battle plans (Wave 6, 7, 8) as COMPLETE
6. Then re-read Wave 6 for remaining non-Forge sprints

---

*Wave 8 Battle Plan — Forged 2026-04-04*
*7 Phases. From proof-of-concept to production fine-tuning.*
*BlackMage certified. Micromanager gated. Scientist approved.*
*The forge fires burn true.*
