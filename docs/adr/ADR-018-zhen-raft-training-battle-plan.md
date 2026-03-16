# ADR-018: Zhen RAFT Training — Battle Plan

**Status:** Planned
**Date:** 2026-03-16
**Deciders:** Developer, Scientist
**Depends on:** ADR-017 (context window optimization — complete)

## Goal

Upgrade Zhen from base Mistral-7B (RAG only) to a RAFT-tuned Mistral-7B that has learned how to use retrieved context effectively. Same retrieval pipeline, smarter model underneath.

**Current state:** Zhen answers from raw Mistral-7B + RAG context stuffing. The model has no training on Unheaded-specific material — it treats retrieved chunks as generic text.

**Target state:** Zhen answers from a fine-tuned model that understands Unheaded terminology, architecture patterns, and how to extract answers from noisy retrieval results (source doc + distractor chunks = RAFT).

## What RAFT Is (vs RAG)

**RAG** = Retrieval-Augmented Generation. At query time, retrieve chunks from FAISS, stuff into prompt. Model reads them cold.

**RAFT** = Retrieval-Augmented Fine-Tuning. Train the model on QA pairs that include retrieved context (oracle + distractor chunks), so it learns:
- How to identify the relevant chunk among noise
- How to extract precise answers from Unheaded-specific documentation
- Unheaded terminology, architecture names, port numbers, service roles

The model gets better at the specific task of answering questions with retrieval augmentation.

## Hardware

| Component | Spec | Constraint |
|-----------|------|------------|
| GPU | AMD RX 7700 XT | 12 GB VRAM (shared with inference) |
| RAM | 16 GB DDR5 | Limits batch size during training |
| ROCm | 6.4.2 | Installed, llama.cpp uses it for inference |
| Storage | ~100 GB free | Merged model + GGUF intermediates need ~30 GB |

## Current State of Pipeline

### Already Built
| Script | Purpose | Status |
|--------|---------|--------|
| `05_generate_qa.py` | Generate QA pairs from Ring 1 corpus via Mistral | Works |
| `11_generate_qa_ring234.py` | Generate QA pairs from Ring 2-4 corpus | Works |
| `07_prepare_training.py` | Format QA → Mistral instruct training examples | Works |
| `08_train_qlora.py` | QLoRA fine-tuning (4-bit, LoRA rank 16) | Untested (dep issue) |
| `09_merge_and_quantize.sh` | Merge adapter → GGUF → Q5_K_M quantize | Untested |
| `10_hotswap_index.py` | Swap active FAISS index without restart | Works |

### Training Data
| File | Entries | Notes |
|------|---------|-------|
| `raft_dataset.jsonl` | 401 | Ring 1 QA pairs |
| `raft_dataset_ring234.jsonl` | 215 | Ring 2-4 QA pairs |
| `raft_dataset_combined.jsonl` | 616 | Combined |
| `training/train.jsonl` | 360 | 90% split (Ring 1 only!) |
| `training/eval.jsonl` | 41 | 10% split |

**Problem:** Only 360 training examples. This is far too few for meaningful fine-tuning. Need 2K-5K minimum, ideally 10K+.

### Dependency Blocker
| Package | Installed | Issue |
|---------|-----------|-------|
| PyTorch | 2.10.0+cu128 (CUDA) | **Wrong build.** Need ROCm build for GPU training. |
| ROCm | 6.4.2 | Installed, but no ROCm PyTorch wheel for Python 3.13 yet. |
| bitsandbytes | Not installed | Needed for 4-bit QLoRA. ROCm support is experimental. |
| peft | 0.18.1 | OK |
| trl | 0.29.0 | OK |
| transformers | 5.3.0 | OK |
| accelerate | 1.13.0 | OK |

**Root cause:** Python 3.13 + ROCm 6.4 = no prebuilt PyTorch wheel available from pytorch.org. Options:
1. Build PyTorch from source with ROCm 6.4 support (complex, 1-2 hours)
2. Use Python 3.12 in a separate venv (easiest)
3. Use `unsloth` which bundles its own ROCm-compatible torch

## Battle Plan

### Phase 1: Expand Training Data (No GPU needed)

Generate more QA pairs using the existing Mistral-7B inference server (already running at `-c 16384`).

**Step 1.1:** Run `05_generate_qa.py --count 2000` against the full Ring 1 corpus.
- Uses the running llama-server to generate questions from corpus chunks
- Each QA pair includes the source chunk + distractor chunks from FAISS
- Output: `raft_dataset.jsonl` expanded from 401 → ~2000 entries
- **Time:** ~2-4 hours (depends on generation speed, 60 tok/s = ~6s per QA)

**Step 1.2:** Run `11_generate_qa_ring234.py --count 1000` for Ring 2-4.
- Covers GitHub repos, RFCs, research papers, Stack Overflow
- Output: `raft_dataset_ring234.jsonl` expanded from 215 → ~1000 entries

**Step 1.3:** Combine and deduplicate.
```bash
cat raft_dataset.jsonl raft_dataset_ring234.jsonl > raft_dataset_combined.jsonl
# Deduplicate by question field
python3 -c "
import json
seen = set()
with open('raft_dataset_combined.jsonl') as f, open('raft_dataset_deduped.jsonl', 'w') as out:
    for line in f:
        q = json.loads(line).get('question', '')
        if q not in seen:
            seen.add(q)
            out.write(line)
"
```

**Step 1.4:** Rebuild training split.
- Update `07_prepare_training.py` to read `raft_dataset_deduped.jsonl`
- Bump `max_seq_length` consideration: training examples should match production context
- 90/10 train/eval split
- Target: 2500+ train, 250+ eval

**Milestone:** 3000+ QA pairs in `training/train.jsonl`, formatted as Mistral instruct.

### Phase 2: Fix PyTorch ROCm (Required for GPU Training)

**Option A (Recommended): Separate Python 3.12 venv for training**
```bash
# Install Python 3.12 if not present
sudo apt install python3.12 python3.12-venv

# Create training-specific venv
python3.12 -m venv ~/.venv/zhen-train

# Install ROCm PyTorch
~/.venv/zhen-train/bin/pip install torch --index-url https://download.pytorch.org/whl/rocm6.2

# Install training deps
~/.venv/zhen-train/bin/pip install transformers peft trl datasets accelerate

# bitsandbytes for ROCm (experimental)
~/.venv/zhen-train/bin/pip install bitsandbytes
# If that doesn't work with ROCm, build from source:
# git clone https://github.com/TimDettmers/bitsandbytes.git
# cd bitsandbytes && ROCM_HOME=/opt/rocm pip install .
```

The inference venv (`~/.venv/zhen`) stays on Python 3.13 — it doesn't need GPU PyTorch (it talks to llama.cpp via HTTP).

**Option B: Build PyTorch from source for 3.13 + ROCm 6.4**
- Complex, fragile, 1-2 hours compile time
- Only worth it if Option A fails

**Option C: unsloth**
- Claims 2x faster QLoRA, has ROCm support
- `pip install unsloth` — may pull its own compatible torch
- Worth trying if Options A/B are painful

**Milestone:** `torch.cuda.is_available() == True` in training venv, with AMD GPU detected.

### Phase 3: Training Config Updates

Before training, update `08_train_qlora.py`:

```python
CONFIG = {
    "base_model": "mistralai/Mistral-7B-Instruct-v0.2",
    "max_seq_length": 4096,    # was 2048 — match expanded context window

    # LoRA — keep conservative for 12GB VRAM
    "lora_rank": 16,
    "lora_alpha": 32,
    "lora_dropout": 0.05,
    "target_modules": ["q_proj", "k_proj", "v_proj", "o_proj"],

    # Training — adjust for larger dataset
    "learning_rate": 2e-4,
    "per_device_train_batch_size": 1,     # was 2 — safer for 12GB with 4096 seq
    "gradient_accumulation_steps": 8,     # was 4 — effective batch 8
    "num_train_epochs": 2,                # was 3 — more data means fewer epochs needed
    "warmup_ratio": 0.03,
    "weight_decay": 0.01,
    "bf16": True,
    "gradient_checkpointing": True,       # critical for VRAM
    "optim": "paged_adamw_8bit",
}
```

**Key changes:**
- `max_seq_length: 4096` — expose model to longer contexts during training
- `batch_size: 1` — safer for 12GB VRAM with longer sequences
- `gradient_accumulation: 8` — maintain effective batch size
- `epochs: 2` — more data, fewer passes needed

### Phase 4: Train

```bash
# Stop inference server to free VRAM (training needs full 12GB)
kill $(lsof -ti :20100)

# Activate training venv
source ~/.venv/zhen-train/bin/activate
cd ~/tmp/unheaded/raft/scripts

# Run training
python3 08_train_qlora.py
```

**Expected:**
- Duration: 1-3 hours depending on dataset size and batch config
- VRAM: ~10-11 GB (4-bit base + LoRA adapters + gradient checkpointing)
- Output: `training/checkpoints/final-adapter/` (LoRA weights, ~100 MB)
- Checkpoints saved every 50 steps for recovery

**Monitoring:**
- Training loss should decrease steadily
- Eval loss should track training loss (not diverge = not overfitting)
- If OOM: reduce `max_seq_length` to 2048, or `lora_rank` to 8

### Phase 5: Merge + Quantize + Deploy

```bash
# Still in training venv
cd ~/tmp/unheaded/raft/scripts
./09_merge_and_quantize.sh
```

This produces:
1. `training/merged-model/` — full Mistral-7B with LoRA weights merged in (~14 GB)
2. `models/zhen-raft-mistral-7b-f16.gguf` — GGUF float16 (~14 GB)
3. `models/zhen-raft-mistral-7b-q5km.gguf` — Q5_K_M quantized (~5 GB)

**Deploy:**
```bash
# Update start-zhen.sh to use the RAFT model
# Change model path from:
#   mistral-7b-instruct-q5_k_m.gguf
# To:
#   zhen-raft-mistral-7b-q5km.gguf

# Or use env var:
export ZHEN_MODEL=~/tmp/unheaded/raft/models/zhen-raft-mistral-7b-q5km.gguf
```

### Phase 6: Validate

**A/B comparison** — run the same 20 questions against both models:

```bash
# Baseline: original Mistral-7B
llama-server -m models/mistral-7b-instruct-q5_k_m.gguf -ngl 40 -c 16384 --port 20100

# RAFT: fine-tuned model
llama-server -m models/zhen-raft-mistral-7b-q5km.gguf -ngl 40 -c 16384 --port 20100
```

**Test categories:**
1. Unheaded-specific: "What port does Wotan use?" (should answer 18000/18001 precisely)
2. Architecture: "Explain the 6 layers" (should match CLAUDE.md exactly)
3. Retrieval quality: Give it 3 chunks (1 relevant, 2 distractors) — does it pick the right one?
4. General knowledge: "What is eBPF?" (should not degrade general ability)
5. Conversation follow-up: multi-turn with "elaborate on #3" style queries

**Success criteria:**
- Unheaded-specific questions: >80% factually correct (vs ~50% now)
- No regression on general knowledge
- Speed: within 5% of baseline (should be identical — same architecture, same quant)

### Phase 7: Continuous Improvement Loop

Once the base RAFT pipeline works:

1. **Conversation mining:** Export good Q&A from `zhen_conversations` table → add to training set
2. **Remember → Train loop:** "Remembered" answers (via UI button) become future training examples
3. **Teach → Train loop:** `/api/v1/teach` content gets embedded for RAG immediately, and queued for next training batch
4. **Periodic retraining:** Monthly or after 500+ new QA pairs accumulated
5. **Model versioning:** Keep old GGUFs so you can roll back (`models/zhen-raft-v1.gguf`, `v2`, etc.)

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| PyTorch ROCm doesn't work on this hardware | Training blocked | Option B/C, or rent cloud GPU for training only |
| 12GB VRAM OOM during training | Training crashes | Reduce batch_size=1, seq_len=2048, lora_rank=8 |
| RAFT model degrades general knowledge | Worse at non-Unheaded questions | Eval set includes general knowledge questions |
| QA generation produces low-quality pairs | Model learns garbage | Manual review of sample, filter by answer length/quality |
| bitsandbytes ROCm broken | Can't do 4-bit QLoRA | Use 8-bit or full precision with aggressive gradient checkpointing |

## Cost

Zero. All local, all open-source, all on hardware you own.

## Timeline Estimate

| Phase | Effort | Calendar |
|-------|--------|----------|
| Phase 1: Expand training data | ~4 hours (automated) | Day 1 |
| Phase 2: Fix PyTorch ROCm | ~1-2 hours | Day 1 |
| Phase 3: Update training config | ~15 minutes | Day 1 |
| Phase 4: Train | ~2-3 hours (automated) | Day 1-2 |
| Phase 5: Merge + quantize | ~30 minutes (automated) | Day 2 |
| Phase 6: Validate | ~1 hour | Day 2 |

**Total:** ~1-2 days from start to RAFT-tuned Zhen in production.

## References

- Existing pipeline: `raft/scripts/05_generate_qa.py` through `09_merge_and_quantize.sh`
- Training data: `raft/raft_dataset_combined.jsonl` (616 existing pairs)
- Model: `raft/models/mistral-7b-instruct-q5_k_m.gguf` (base, 5.1 GB)
- ADR-017: Context window benchmark results (16384 tokens, zero speed loss)
- Benchmark data: `raft/experiments/context_window_benchmark.json`
- [RAFT paper](https://arxiv.org/abs/2403.10131): "RAFT: Adapting Language Model to Domain Specific RAG"
- Karpathy autoresearch pattern (see memory: `reference_autoresearch.md`)
