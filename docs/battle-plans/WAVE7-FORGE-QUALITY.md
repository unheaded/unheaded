# WAVE 7 BATTLE PLAN — Zhenai Forge Quality Sprint

**Forged**: 2026-04-04
**Sprint**: WAVE-7 — From Garbage Gradients to Real Training
**Prerequisite**: zhenai-forge runs real forward passes (Wave 6 Sprint A1-A5 done)
**Target**: kingdom.zlora trained with proper tokenization, 32 layers, analytical backprop

## PROBLEM STATEMENT

zhenai-forge currently:
- ✓ Reads real GGUF model weights
- ✓ Loads 4.8GB quantized data to GPU
- ✓ Dequantizes Q5_K tensors correctly
- ✓ Computes real forward pass through model weights
- ✓ Estimates numerical gradients and runs Adam optimizer
- ✗ Uses byte-hashing instead of real tokenizer (garbage tokens)
- ✗ Only uses 2 of 32 layers (6% of model)
- ✗ Numerical gradients are 1000x slower than backprop

Fix order matters: Tokenizer → Layers → Backprop (each builds on the last)

## STEP 1: GGUF-NATIVE TOKENIZER

**Goal**: Real sentencepiece tokenization from GGUF metadata
**Time**: 1-2 hours
**Files**: `crates/zhenai-forge/src/tokenizer.rs` (new), `src/train.rs`

- [ ] **1.1** Extract vocabulary from GGUF `tokenizer.ggml.tokens` metadata
  - GOTO: `crates/zhenai-forge/src/gguf.rs` (metadata parsing)
  - The vocabulary is stored as an array of 32000 strings
  - Build HashMap<String, u32> for token → ID lookup

- [ ] **1.2** Implement greedy longest-match tokenizer
  - Iterate through input text
  - At each position, find longest matching token in vocabulary
  - If no match, fall back to byte-level tokens
  - Handle BOS/EOS tokens (token 1 = `<s>`, token 2 = `</s>`)

- [ ] **1.3** Wire into training loop (replace byte-hashing)
  - GOTO: `crates/zhenai-forge/src/train.rs` (token_ids generation)

- [ ] **1.4** Test: tokenize "What is Wotan?" and verify reasonable output
  - Should produce ~5-8 tokens, not 500+ byte-hashes

## STEP 2: SCALE TO 32 LAYERS

**Goal**: Forward pass through all 32 transformer layers
**Time**: 1-2 hours
**Files**: `crates/zhenai-forge/src/train.rs` (CpuWeights)

- [ ] **2.1** Modify CpuWeights::load() to dequantize all 32 layers
  - Stream: dequant layer N, process, store only what's needed
  - Monitor RAM — should stay under 10GB total

- [ ] **2.2** Modify forward_loss() to iterate all 32 layers
  - Currently processes 2 layers, skip to next
  - Add all attention + FFN per layer

- [ ] **2.3** Verify: loss values change meaningfully with 32 layers vs 2

## STEP 3: FULL FORWARD PASS

**Goal**: Complete transformer layer (attention + FFN, not just Q projection)
**Time**: 2-3 hours
**Files**: `crates/zhenai-forge/src/forward.rs`

- [ ] **3.1** Implement attention_layer() — full Q/K/V/O with LoRA
- [ ] **3.2** Implement ffn_layer() — gate/up/down projections with SiLU
- [ ] **3.3** Wire into forward_loss() — replace simplified Q-only path
- [ ] **3.4** Verify: loss is lower than Q-only (more compute = better predictions)

## STEP 4: ANALYTICAL BACKPROPAGATION

**Goal**: Replace numerical gradients with chain-rule backprop
**Time**: 3-4 hours
**Files**: `crates/zhenai-forge/src/backward.rs` (new)

- [ ] **4.1** Implement backward for cross_entropy_loss → softmax
- [ ] **4.2** Implement backward for matmul (∂C/∂A = B^T, ∂C/∂B = A^T)
- [ ] **4.3** Implement backward for RMSNorm
- [ ] **4.4** Implement backward for LoRA (∂L/∂A, ∂L/∂B from chain rule)
- [ ] **4.5** Wire into training loop — 1 forward + 1 backward per step
- [ ] **4.6** Verify: training speed ~1000x faster, loss still decreases

## STEP 5: VERIFY + DEPLOY

- [ ] **5.1** Train 100 examples, verify loss curve
- [ ] **5.2** Full training run on 3965 QA pairs
- [ ] **5.3** Deploy kingdom.zlora and test with llama-server
- [ ] **5.4** Update Wave 6 Sprint A as COMPLETE
- [ ] **5.5** Update ADR-030 status

## EXECUTION RULES
1. After each step: commit, re-read this plan, start next step
2. If blocked: skip to next independent step
3. After all steps: update `docs/battle-plans/WAVE6-REMAINING-WORK.md` Sprint A → DONE
4. Then re-read Wave 6 for remaining sprints and continue

---

*Wave 7 Battle Plan — Forged 2026-04-04*
*5 Steps. From garbage tokens to real training.*
*The forge fires get hotter.*
