# WAVE 10B BATTLE PLAN — Fix Zhenai-Forge Training Loop

**Forged**: 2026-04-06
**Codename**: The Real Training
**Round Table**: Scientist (diagnosis), Architect (design), Developer (implementation), BlackMage (validation), Marshal (enforcement), Warmonger (this plan)
**Prerequisite**: Wave 10 epoch 1 complete, all checkpoints degenerate, root cause identified
**Target**: Zhenai-forge produces LoRA adapters that improve Mistral-7B on Kingdom questions
**Commit Cadence**: Every phase

## LEGEND

[B] = Bash | [V] = Verify | [D] = Debug | [CODE] = Implementation
[TEST] = Test | [C] = Commit | [SEQ] = Sequential

---

## ROOT CAUSE ANALYSIS (Scientist)

**Observation**: v5 training loss dropped 12.21 → 0.038 (epoch 1) → 0.000 (epoch 2). All checkpoints produce degenerate output (hallucinated nonsense, repetitive text, no Kingdom knowledge).

**Root cause: TWO critical bugs in `crates/zhenai-forge/src/train.rs`**:

### Bug 1: Loss computed on LAST TOKEN ONLY (train.rs:667-684)
```rust
let last_pos = token_ids.len() - 1;
// ...
let target = token_ids[last_pos] % vocab_subset as u32;
forward::cross_entropy_loss(&logits, target)
```
For `<s>[INST]...Question...[/INST] Answer... </s>`, the last token is `</s>` (EOS=2).
The model ONLY learns to predict EOS. Loss drops to zero because predicting EOS is trivial.
**The model never learns to generate answers.**

### Bug 2: Forward pass only processes last position (train.rs:271-345)
```rust
let last_pos = token_ids.len() - 1;
for l in 0..n_layers_used {
    let layer_in = hidden[last_pos * n..(last_pos + 1) * n].to_vec();
    // ... only processes last position through layers
}
```
Backward pass also only computes gradients from last position.
**LoRA adapters learn to modify the hidden state at ONE position, not the full sequence.**

### Bug 3 (Minor): Partial dimension computation (forward.rs:96, train.rs:300)
`n.min(256)` limits projections to first 256 dims of 4096. FFN uses `n_ff.min(256)` of 14336.
This loses 94% of the information in each layer.

### Why loss still decreased:
Predicting EOS gets easier as the LoRA changes accumulate — the model learns "if I see a long sequence, output EOS" which is trivially correct for the training data format. No actual language generation is learned.

---

## THE FIX (Architect + Scientist)

### Strategy: Multi-Position Loss

Instead of computing loss on just the last token, compute loss across ALL answer tokens.
For each position `t` in the sequence, predict `token[t+1]` from `hidden[t]`.

**Key constraint**: Only compute loss on answer tokens (after `[/INST]`).
Context/question tokens provide the conditioning but shouldn't contribute to loss.
This prevents the model from learning to memorize context verbatim.

### Implementation approach:
1. Find the `[/INST]` boundary in token IDs
2. For each position `t >= inst_boundary` to `len-1`:
   - Get hidden state at position `t` (requires processing ALL positions through layers)
   - Project to logits via output matrix
   - Compute cross-entropy loss against `token[t+1]`
3. Average loss across all answer positions
4. Backward pass: accumulate gradients from all answer positions

### Simplified approach (Developer recommendation):
Full multi-position forward through 32 transformer layers at 4096 dims is expensive.
**Compromise**: Process a WINDOW of positions around the answer tokens.
- Process last N positions (where N = answer token count, capped at 128)
- This gives the model the full answer context while staying memory-bounded
- Use full 4096 dims (not 256) for at least the final 3-4 layers

---

## PHASE 1: Fix Forward Pass — Multi-Position Loss (Steps 1-10)

**Goal**: `forward_loss` computes loss across multiple answer tokens, not just EOS.
**Impact**: The training signal teaches actual language generation.
**Files**: `crates/zhenai-forge/src/train.rs`, `crates/zhenai-forge/src/forward.rs`

### Steps

- [ ] **Step 1** [CODE]: Add `find_inst_boundary()` helper to train.rs
  - Scan token_ids for the `[/INST]` token sequence
  - Return the index of the first answer token
  - Mistral uses token ID 29 for ], 28 for [ — check tokenizer
  - Fallback: if not found, use position len*3/4 (heuristic)

- [ ] **Step 2** [CODE]: Rewrite `CpuWeights::forward_loss()` for multi-position
  - Process ALL sequence positions through embedding + layers (not just last_pos)
  - For each position `t` from `inst_boundary` to `len-2`:
    - Extract hidden[t] → output_norm → logits
    - Cross-entropy loss against token[t+1]
  - Return average loss across all answer positions
  - Use full 4096 dims (remove the `.min(256)` truncations)

- [ ] **Step 3** [V]: Verify forward_loss produces non-trivial loss values
  - Load model, format one training example, compute loss
  - Loss should be near ln(32000) ≈ 10.37 at initialization (random prediction)
  - If loss < 5 on untrained model → bug in loss computation

- [ ] **Step 4** [CODE]: Update the backward pass (training loop lines 258-435)
  - Forward pass: save hidden states at ALL answer positions
  - Backward pass: compute gradients from ALL answer positions
  - Accumulate LoRA gradients across positions before Adam step
  - This is the most complex change — requires per-position activation caching

- [ ] **Step 5** [CODE]: Remove `.min(256)` dimension truncations
  - `forward.rs:96`: Remove `n_embd.min(256)` and `n_ff.min(256)` in ffn_forward
  - `train.rs:300`: Remove `n.min(256)` in attention projection
  - `train.rs:645`: Remove `.min(64)` in forward_loss Q projection
  - Use full dimensions: 4096 for embeddings, 14336 for FFN
  - WARNING: This increases memory and compute significantly
  - If OOM: keep partial dims but increase to 1024 (from 256)

- [ ] **Step 6** [TEST]: Unit test — multi-position loss vs single-position
  - Create a 10-token sequence, compute loss both ways
  - Multi-position should return non-trivial loss
  - Single-position (old) should return near-zero for EOS

- [ ] **Step 7** [V]: Compile and run all existing tests
  ```bash
  cd crates/zhenai-forge && cargo test 2>&1 | tail -20
  ```

- [ ] **Step 8** [C]: Commit "fix(forge): multi-position loss — train on answer tokens, not just EOS"

- [ ] **Step 9** [V]: **PHASE 1 EXIT GATE**
  - forward_loss processes all answer positions (not just last)
  - Loss on untrained model ≈ 10.37 (ln(vocab))
  - Full dimensions used (4096 or at minimum 1024)
  - All 34+ tests pass
  - cargo build --release succeeds

- [ ] **Step 10** [B]: Build release binary
  ```bash
  cd crates/zhenai-forge && cargo build --release
  ```

---

## PHASE 2: Lower Learning Rate + Add Dropout (Steps 11-15)

**Goal**: Prevent memorization with hyperparameter changes.
**Files**: `crates/zhenai-forge/src/train.rs`, `crates/zhenai-forge/src/lora.rs`

- [ ] **Step 11** [CODE]: Change default learning rate from 3e-4 to 1e-4
  - In TrainConfig::default(), set lr = 1e-4
  - Scientist: with multi-position loss, gradient magnitude increases ~N× (N=answer tokens)
  - Lower LR compensates for larger effective gradient

- [ ] **Step 12** [CODE]: Add dropout to LoRA forward pass
  - `lora.rs`: Add `dropout: f32` field to LoraLayer
  - During training: randomly zero 10% of LoRA output elements
  - During inference: no dropout (scale by 1-dropout_rate)
  - Add `--dropout` CLI flag (default 0.1)

- [ ] **Step 13** [CODE]: Add eval loss logging every 500 steps
  - After each 500-step block: compute loss on 10% held-out eval set
  - Log both train and eval loss
  - If eval_loss > 2× train_loss → print WARNING: possible overfitting

- [ ] **Step 14** [CODE]: Set default epochs to 1 (not 3)
  - With proper multi-position loss, 1 epoch should be sufficient
  - Add early stopping: if eval loss increases 3× in a row, stop

- [ ] **Step 15** [C]: Commit "feat(forge): lower LR, LoRA dropout, eval loss, early stopping"

---

## PHASE 3: Validate Fix — Short Training Run (Steps 16-22)

**Goal**: Train 500 steps with fixed code, verify non-degenerate output.
**This is the critical validation before a full training run.**

- [ ] **Step 16** [B]: Kill any existing training/inference processes
  ```bash
  pkill -f zhenai-forge; pkill -f llama-server; sleep 2
  ```

- [ ] **Step 17** [B]: Start short training run (500 steps = ~12 min)
  ```bash
  cd ~/tmp/unheaded/crates/zhenai-forge && \
  LD_LIBRARY_PATH=/opt/rocm/lib ./target/release/zhenai-forge train \
    --model /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf \
    --data /var/zhen/raft_dataset_v5.jsonl \
    --output /var/zhen/models/kingdom-v6-test.zlora \
    --epochs 1 --lr 1e-4 2>&1 | tee /tmp/forge-v6-test.log
  ```

- [ ] **Step 18** [V]: Verify loss trajectory
  - Step 1: loss should be near 10.37 (ln(32000))
  - Step 50: loss should be decreasing but still > 5
  - Step 500: loss should be 2-5 range (NOT < 1)
  - If loss < 1 at step 500 → still possible memorization, investigate

- [ ] **Step 19** [B]: Convert checkpoint to GGUF
  ```bash
  python3 ~/tmp/unheaded/scripts/zlora-to-gguf.py \
    -i /var/zhen/models/kingdom-v6-test.zlora.checkpoint-500 \
    -o /tmp/v6_test_lora.gguf
  ```

- [ ] **Step 20** [B]: Start llama-server with test LoRA and ask 3 questions
  ```bash
  ~/tmp/unheaded/llama.cpp/build/bin/llama-server \
    --model /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf \
    --lora /tmp/v6_test_lora.gguf --port 20200 --ctx-size 4096 \
    --n-gpu-layers 28 &
  sleep 5
  # Test 3 questions manually
  ```

- [ ] **Step 21** [V]: Quality check — LoRA output must be:
  - Coherent English (not repetitive nonsense)
  - Not worse than base model (at minimum)
  - Ideally shows some Kingdom-specific knowledge
  - NO hallucinated definitions ("Large-Scale Data Link" = FAIL)

- [ ] **Step 22** [V]: **PHASE 3 EXIT GATE**
  - Loss trajectory is reasonable (10→5→2-5 range, NOT dropping to 0)
  - LoRA output is coherent English
  - No degenerate repetition
  - Model is at least as good as base Mistral

---

## PHASE 4: Full Training Run (Steps 23-30)

**Only execute if Phase 3 passes. If Phase 3 fails, debug first.**

- [ ] **Step 23** [B]: Merge dataset with new Claude QA pairs
  ```bash
  cat /var/zhen/raft_dataset_combined.jsonl \
      /var/zhen/distilled_qa_claude.jsonl \
      /var/zhen/distilled_local_repo.jsonl \
    > /var/zhen/raft_dataset_v6.jsonl
  wc -l /var/zhen/raft_dataset_v6.jsonl
  ```

- [ ] **Step 24** [B]: Start full v6 training (1 epoch)
  ```bash
  cd ~/tmp/unheaded/crates/zhenai-forge && \
  LD_LIBRARY_PATH=/opt/rocm/lib nohup ./target/release/zhenai-forge train \
    --model /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf \
    --data /var/zhen/raft_dataset_v6.jsonl \
    --output /var/zhen/models/kingdom-v6.zlora \
    --epochs 1 --lr 1e-4 \
    > /tmp/forge-v6.log 2>&1 &
  echo "PID: $!"
  ```

- [ ] **Step 25** [V]: Monitor every 30 min — check loss is in expected range

- [ ] **Step 26** [B]: When complete, convert to GGUF
  ```bash
  python3 ~/tmp/unheaded/scripts/zlora-to-gguf.py \
    -i /var/zhen/models/kingdom-v6.zlora \
    -o /var/zhen/models/kingdom-v6-lora.gguf
  ```

- [ ] **Step 27** [B]: Run A/B test (30 Kingdom questions)
  ```bash
  ./scripts/ab_test_checkpoint.sh /var/zhen/models/kingdom-v6.zlora
  ```

- [ ] **Step 28** [V]: Quality gate — must pass ALL:
  - Memorization flags ≤ 5/30
  - LoRA answers reference real Kingdom terms (ports, services, architecture)
  - No degenerate/repetitive output
  - Base model coherence preserved

- [ ] **Step 29** [B]: If passes → deploy
  ```bash
  # Update llama-server to use v6 LoRA
  ```

- [ ] **Step 30** [C]: Commit "progress: kingdom-v6 trained with multi-position loss, A/B validated"
  - Update ADR-030 with v6 results
  - Update WAVE10 battle plan

---

## DEPENDENCY CHAIN

```
Phase 1 (fix forward pass) → Phase 2 (hyperparams) → Phase 3 (validate 500 steps)
                                                           ↓
                                                      Phase 4 (full run)
```

Phases 1+2 are sequential (code changes). Phase 3 is the gate. Phase 4 only if Phase 3 passes.

## MARSHAL ENFORCEMENT

- No skipping Phase 3 validation. 500-step test MUST pass before full training.
- Loss must be in expected range at Phase 3 Step 18.
- If Phase 3 fails, DO NOT proceed to Phase 4. Debug and fix.
- Commit after every phase.

---

*Wave 10B Battle Plan — Forged 2026-04-06*
*"The model learns to GENERATE, not memorize. The loss measures PREDICTION, not recognition."*
