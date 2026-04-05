# WAVE 10 BATTLE PLAN — RAFT Quality Revolution

**Forged**: 2026-04-05
**Codename**: The Correct Training
**Round Table**: Architect (lead), BlackMage (depth), Developer (perf), Scientist (validation), Warmonger (plan)
**Prerequisite**: Wave 9 complete, 6000+ QA pairs, zhenai-forge 34/34 tests
**Target**: Zhenai answers Kingdom questions accurately — not generic, not repetitive
**Estimated Duration**: 6-8 hours across 4 phases
**Commit Cadence**: Every 3 steps

## LEGEND

[B] = Bash | [V] = Verify | [D] = Debug | [W] = Wire | [CODE] = Implementation
[TEST] = Test | [C] = Commit | [SEQ] = Sequential

## ROOT CAUSE ANALYSIS (Scientist)

Loss went 14.33→8.77 (v2) and 10→9.9 (v3) but LoRA output is degenerate. Why?

**Finding 1: format_prompt ignores RAFT context.**
Current: `[INST] Question: X [/INST] Answer </s>` — naked Q/A.
RAFT paper requires: question + source_content + distractor_chunks → answer.
The model never learned to extract from context. It memorizes answers instead of
learning retrieval-augmented reasoning.

**Finding 2: Answers are too short.**
Avg 123 tokens per prompt. Model learns 2-sentence factoid responses.
Zhenai needs 500+ token operational explanations with specifics.

**Finding 3: 7.3GB VRAM unused.**
batch_size=1, max_seq_len=2048 never triggered (max actual: 553 tokens).
Gradient accumulation is free speedup. Tokenizer caching saves 15%.

**Finding 4: Training data has distractor fields — unused.**
`raft_dataset_combined.jsonl` contains `source_content` and `distractor_chunks`
but `data.rs format_prompt()` strips them out.

## CRITICAL FINDINGS (Developer/Scientist review 2026-04-05)

**BUG 1 (CRITICAL): format_prompt was DEAD CODE.** train.rs loaded raw JSON
strings via load_training_jsonl(), fed them directly to the tokenizer. All v1-v3
training runs learned to predict JSON syntax, not natural language. This explains
degenerate output despite decreasing loss — the loss measured JSON prediction quality.
FIX: load_and_format_training_data() now parses JSON → TrainingExample → format_prompt().

**BUG 2: Gradient accumulation leaked.** Gradients from previous accumulation windows
were never zeroed, contaminating subsequent updates.
FIX: Zero grad_a/grad_b after each Adam step.

**BUG 3 (Scientist): Stale FAISS index.** Built Apr 2, missing 188 commits. If v5 LoRA
is deployed, the model "knows" about Wave 9/10 work but RAG can't retrieve supporting
documents. Re-index MUST happen before v5 deployment.

## PROGRESS

- [x] Phase 1: Fix RAFT Prompt Format (Steps 1-8) — DONE + format_prompt wired into train.rs
- [~] Phase 2: Deepen Training Data (Steps 9-16) — 286 Claude pairs, 11K+ total
- [x] Phase 3: Training Speed Optimizations (Steps 17-24) — accum=4, tokenizer cache, grad zero fix
- [ ] Phase 4: Train + Validate kingdom-v5 (Steps 25-32) — BLOCKED on GPU (distillation running)
- [ ] Phase 5 (NEW): Re-index Zhenai FAISS corpus (required before v5 deployment)

---

## PHASE 1: FIX RAFT PROMPT FORMAT (Steps 1-8)

**Goal**: format_prompt includes source context + distractors per RAFT paper.
**Impact**: Highest ROI change. Model learns context-grounded reasoning.
**Time**: ~1.5 hours
**Files**: `crates/zhenai-forge/src/data.rs`

The RAFT paper (Zhang et al. 2024) trains on: question + oracle document +
distractor documents → answer. The model learns which document contains the
answer and how to extract it. We have this data. We just don't use it.

### Current format_prompt (WRONG):
```
<s>[INST] You are Zhenai, champion of the Unheaded Kingdom.
Answer using the context provided.

Question: {question} [/INST] {answer} </s>
```

### Correct RAFT format:
```
<s>[INST] You are Zhenai, champion of the Unheaded Kingdom.
Answer the question using ONLY the provided documents.

Document 1 (source): {source_content}
Document 2: {distractor_1}
Document 3: {distractor_2}

Question: {question} [/INST] {answer} </s>
```

### Steps

- [ ] **Step 1** [CODE]: Update TrainingExample struct in data.rs
  - Add `source_content: Option<String>` field
  - Add `distractor_chunks: Option<Vec<DistractorChunk>>` field
  - Add `DistractorChunk { content: String, source: String }` struct
  - Serde deserialize from existing JSONL format (fields already exist in data)

- [ ] **Step 2** [CODE]: Rewrite format_prompt() for RAFT
  - If source_content + distractors present: full RAFT format with numbered docs
  - Shuffle document order (source not always first — prevents positional bias)
  - Truncate each document to ~300 tokens to fit context window
  - If no source_content (Claude/Mistral distilled pairs): use simple Q/A format
  - Total prompt target: 400-800 tokens (up from 123)

- [ ] **Step 3** [TEST]: Test format_prompt with RAFT data
  - Load first 5 examples from raft_dataset_combined.jsonl
  - Verify source_content and distractors are included
  - Verify total token count is in 400-800 range
  - Verify document order is randomized

- [ ] **Step 4** [V]: Verify JSONL fields parse correctly
  ```bash
  python3 -c "import json; d=json.loads(open('/var/zhen/raft_dataset_combined.jsonl').readline()); print(list(d.keys()))"
  ```
  - Expected: question, answer, source_chunk_id, source_content, distractor_chunks, source_file

- [ ] **Step 5** [C]: Commit RAFT prompt fix

- [ ] **Step 6** [CODE]: Update max_seq_len default to 4096
  - Current data won't exceed this even with RAFT context
  - VRAM impact: negligible (2MB → 8MB for forward buffers, 7.3GB headroom)

- [ ] **Step 7** [TEST]: Verify build + all 34 tests pass
  ```bash
  cd crates/zhenai-forge && cargo test
  ```

- [ ] **Step 8** [V]: **PHASE 1 EXIT GATE**
  - format_prompt includes source context + distractors when available
  - Simple Q/A fallback for distilled pairs without context
  - Token count per example: 400-800 range for RAFT, 80-200 for simple
  - All tests green

---

## PHASE 2: DEEPEN TRAINING DATA (Steps 9-16)

**Goal**: Claude-generated pairs with longer, deeper answers. Include RAFT context.
**Impact**: Higher quality signal. Model learns operational reasoning, not just facts.
**Time**: ~2 hours
**Files**: `/var/zhen/distilled_qa_claude.jsonl`, `raft/scripts/distill_qa.py`

BlackMage's directive: 500+ token answers explaining HOW and WHY, not just WHAT.

### Steps

- [ ] **Step 9** [CODE]: Generate deep-answer Claude pairs (Batch A: Architecture)
  - 20 pairs about architecture with 500+ token answers
  - Include: layer interactions, failure modes, recovery procedures
  - Example: "Explain how a message flows from a service through Wotan to the dashboard,
    including what happens if Wotan is down"

- [ ] **Step 10** [CODE]: Generate deep-answer Claude pairs (Batch B: Operations)
  - 20 pairs about operational scenarios
  - Include: step-by-step troubleshooting, what to check first, escalation paths
  - Example: "A service on EAST is showing 100% failure rate in Akira. Walk through
    the full debugging process from detection to resolution"

- [ ] **Step 11** [CODE]: Generate deep-answer Claude pairs (Batch C: Security)
  - 20 pairs about security architecture
  - Include: threat models, defense layers, incident response
  - Example: "Explain the complete certificate lifecycle from generation through
    rotation to revocation, including what Akira does at each stage"

- [ ] **Step 12** [CODE]: Generate deep-answer Claude pairs (Batch D: Protocol)
  - 20 pairs about the Monad/Sophia/Wotan protocol stack
  - Include: wire format details, packet flow, BPF processing
  - Example: "Trace a Monad packet from creation through 6 hops, explaining what
    each eBPF program does to the register file at each hop"

- [ ] **Step 13** [CODE]: Generate RAFT-format pairs with source context
  - For Claude pairs: add source_content field (the actual file content)
  - Generate 3 distractor chunks per pair (similar but wrong context)
  - Use FAISS similarity search to find realistic distractors

- [ ] **Step 14** [V]: Verify deep pairs quality
  ```bash
  python3 -c "import json; [print(len(json.loads(l)['answer'])) for l in open('/var/zhen/distilled_qa_claude.jsonl').readlines()[-20:]]"
  ```
  - Last 20 pairs should average 500+ chars (125+ tokens)

- [ ] **Step 15** [C]: Commit deep training data

- [ ] **Step 16** [V]: **PHASE 2 EXIT GATE**
  - 80+ new deep-answer pairs (500+ tokens each)
  - RAFT-format pairs with source_content + distractors where available
  - Total dataset: 6000+ pairs with mixed depth

---

## PHASE 3: TRAINING SPEED OPTIMIZATIONS (Steps 17-24)

**Goal**: 3-4x faster training without quality loss.
**Impact**: Train in 7h instead of 28h for 10K pairs.
**Time**: ~1.5 hours
**Files**: `crates/zhenai-forge/src/train.rs`, `crates/zhenai-forge/src/data.rs`

### Steps

- [ ] **Step 17** [CODE]: Add gradient accumulation (accum_steps=4)
  - Zero gradients at start of accumulation window
  - Accumulate gradients for accum_steps forward+backward passes
  - Adam step only when step % accum_steps == 0
  - Scale learning rate by accum_steps (or scale gradients by 1/accum_steps)
  - Effect: same as batch_size=4 but no forward pass code changes

- [ ] **Step 18** [TEST]: Verify gradient accumulation produces same loss trajectory
  - Train 100 steps with accum=1 (baseline)
  - Train 100 steps with accum=4 (should converge similarly)

- [ ] **Step 19** [CODE]: Add tokenizer caching
  - First epoch: tokenize each example, store in Vec<Vec<u32>>
  - Subsequent epochs: skip tokenization, use cached token_ids
  - Memory: ~6000 examples × 123 avg tokens × 4 bytes = ~3MB (trivial)
  - Speedup: ~15% (tokenization is greedy longest-match, not fast)

- [ ] **Step 20** [CODE]: Add --accum-steps CLI flag (default 4)
  - Also add --grad-clip flag (default 1.0, already hardcoded)

- [ ] **Step 21** [V]: Benchmark speedup
  ```bash
  # Train 200 steps with accum=1 vs accum=4, compare wall-clock
  ```

- [ ] **Step 22** [C]: Commit speed optimizations

- [ ] **Step 23** [CODE]: Add training metrics to output
  - Log: tokens/sec, grad_norm, lr, VRAM usage per step
  - These help diagnose training issues

- [ ] **Step 24** [V]: **PHASE 3 EXIT GATE**
  - gradient accumulation working (accum=4)
  - Tokenizer caching working (skip on epoch 2+)
  - 3x+ speedup verified on 200-step benchmark
  - All 34 tests green

---

## PHASE 4: TRAIN + VALIDATE KINGDOM-V5 (Steps 25-32)

**Goal**: Train kingdom-v5.zlora with RAFT format + deep data + speed optimizations.
**Impact**: First LoRA that actually improves Zhenai's answers.
**Time**: ~7-8 hours (training) + 1 hour (validation)
**Prerequisite**: Phases 1-3 complete

### Steps

- [ ] **Step 25** [B]: Merge all datasets
  ```bash
  cat /var/zhen/raft_dataset_combined.jsonl \
      /var/zhen/distilled_qa_claude.jsonl \
      /var/zhen/distilled_local_repo.jsonl \
    > /var/zhen/raft_dataset_v5.jsonl
  wc -l /var/zhen/raft_dataset_v5.jsonl
  ```

- [ ] **Step 26** [B]: Start v5 training
  ```bash
  cd ~/tmp/unheaded/crates/zhenai-forge && \
  LD_LIBRARY_PATH=/opt/rocm/lib ./target/release/zhenai-forge train \
    --model /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf \
    --data /var/zhen/raft_dataset_v5.jsonl \
    --output /var/zhen/models/kingdom-v5.zlora \
    --epochs 3
  ```

- [ ] **Step 27** [V]: Monitor training — loss should start near ln(32000)≈10.37, decrease steadily
  - Check: tail -f /tmp/forge-v5.log
  - Expect: loss < 8.0 by end of epoch 1 (with RAFT context, model has more signal)

- [ ] **Step 28** [B]: Convert v5 .zlora → GGUF
  ```bash
  python3 scripts/zlora-to-gguf.py -i /var/zhen/models/kingdom-v5.zlora -o /var/zhen/models/kingdom-v5-lora.gguf
  ```

- [ ] **Step 29** [B]: A/B test — 30 Kingdom questions, base vs LoRA
  - Start llama-server with --lora flag
  - Run same 30 questions on base model and LoRA model
  - Compare: accuracy, specificity, hallucination rate

- [ ] **Step 30** [V]: Quality gate — LoRA must:
  - Answer Kingdom-specific questions correctly (>70% accuracy)
  - Not produce repetitive/degenerate output
  - Reference actual port numbers, file paths, service names
  - Handle "I don't know" for out-of-domain questions

- [ ] **Step 31** [B]: If quality gate passes → deploy as default
  ```bash
  # Update llama-server startup to include --lora
  ```

- [ ] **Step 32** [V]: **PHASE 4 EXIT GATE**
  - kingdom-v5.zlora trained with RAFT context format
  - A/B test shows measurable improvement
  - Deployed to production inference (or documented why not)
  - ADR-018 and ADR-030 updated

---

## DEPENDENCY CHAIN

```
Phase 1 (RAFT format) ──→ Phase 2 (deep data) ──→ Phase 4 (train v5)
                                                      ↑
Phase 3 (speed opts) ─────────────────────────────────┘
```

Phase 3 is independent of 1+2. Can parallelize:
- Agent A: Phase 1 (data.rs changes)
- Agent B: Phase 2 (generate Claude QA pairs) — can start immediately
- Agent C: Phase 3 (train.rs optimizations) — independent

## VERIFIED HARDWARE NUMBERS (Scientist audit 2026-04-05)

| Metric | Value | Source |
|--------|-------|--------|
| VRAM total | 12,272 MB | rocm-smi |
| VRAM used (model) | 4,829 MB | forge-v3.log |
| VRAM used (LoRA) | 67 MB | forge-v3.log |
| VRAM headroom | 7,376 MB | calculated |
| RAM total | 14 GB | free -m |
| RAM used (training) | 8,364 MB (CPU weights + LoRA) | forge-v3.log |
| Avg tokens/example | 123 | measured from 6242 pairs |
| Max tokens/example | 553 | measured |
| Training speed | 0.3 steps/s | forge-v3.log |
| With accum=4 | ~1.2 effective steps/s | estimated |

---

*Wave 10 Battle Plan — Forged 2026-04-05*
*4 Phases. 32 Steps. The model learns to read, not just recite.*
*"Context is everything. Without it, knowledge is just noise."*
