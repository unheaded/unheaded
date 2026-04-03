# ADR-030: Zhenai Forge — Custom Rust LoRA Training on Heterogeneous Compute

**Status:** In Progress
**Date:** 2026-04-03
**Deciders:** Scientist, BlackMage, Computermancer, Developer
**Priority:** CRITICAL — unblocks RAFT fine-tuning on 14GB RAM hardware

## Context

Python-based QLoRA training (bitsandbytes + transformers + peft) fails on our hardware:
- 14GB system RAM is insufficient — bitsandbytes decompresses 15GB of fp16 weights to RAM before quantizing to GPU
- Training stalls at 79% weight load, swap-thrashes indefinitely
- The RX 7700 XT has 12GB VRAM that sits mostly idle during the RAM-bottlenecked load phase

This is a fundamental mismatch: Python ML frameworks assume 64GB+ RAM workstations. Our WEST box is a 14GB dev machine with a powerful GPU. The solution isn't a bigger machine — it's a smarter tool.

## Decision

Build **zhenai-forge** — a custom Rust LoRA fine-tuning tool that:
1. Reads GGUF model directly (no fp16 decompression)
2. Places frozen base weights on GPU VRAM (5GB quantized)
3. Trains only LoRA adapter matrices in system RAM (~200MB)
4. Uses all available compute: CPU, iGPU, dGPU, RAM, VRAM, swap
5. Outputs a GGUF LoRA adapter that llama-server loads natively

### Why Rust

- Direct memory control — place each tensor on the right device
- No Python overhead (GIL, garbage collector, pip dependency hell)
- ROCm/HIP via `hip-sys` or raw FFI — same GPU, no abstraction tax
- GGUF parsing already proven in the ecosystem (`gguf` crate)
- Aligns with ADR-004 (build our own when we can outperform)
- Aligns with dependency age rule (no dep on bitsandbytes, created 2022)

### Why Not llama-finetune

`llama-finetune` exists in our llama.cpp build and could work. But:
- It's C++ with limited customization
- No ROCm GPU training support (CPU only for fine-tuning)
- Fixed training loop — can't add Kingdom-specific optimizations
- Our Rust tool can evolve into the Zhenai Engine (custom inference too)

## Architecture

### Multi-Device Compute Map

```
┌─────────────────────────────────────────────────────────┐
│ SYSTEM RAM (14GB + 33GB swap)                           │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐ │
│  │ Training Data │  │ LoRA Adapters│  │ Optimizer     │ │
│  │ (tokenized)   │  │ (~200MB)     │  │ State (~400MB)│ │
│  │ ~500MB        │  │ Trainable!   │  │ Adam moments  │ │
│  └──────────────┘  └──────┬───────┘  └───────┬───────┘ │
│                           │                   │         │
└───────────────────────────┼───────────────────┼─────────┘
                            │                   │
        ┌───────────────────┼───────────────────┼─────────┐
        │ RX 7700 XT VRAM (12GB)                │         │
        │                   │                   │         │
        │  ┌────────────────▼────────────────┐  │         │
        │  │ Base Model (Q5_K_M, ~5GB)       │  │         │
        │  │ FROZEN — never modified         │  │         │
        │  └────────────────┬────────────────┘  │         │
        │                   │                   │         │
        │  ┌────────────────▼────────────────┐  │         │
        │  │ Forward Pass (GPU compute)      │◄─┘         │
        │  │ - Input embeddings              │            │
        │  │ - Attention (Q/K/V + LoRA)      │            │
        │  │ - FFN layers                    │            │
        │  │ - Output logits                 │            │
        │  └────────────────┬────────────────┘            │
        │                   │                             │
        │  ┌────────────────▼────────────────┐            │
        │  │ Backward Pass (gradients)       │            │
        │  │ - Only for LoRA parameters      │            │
        │  │ - Base model gradients skipped  │            │
        │  └────────────────┬────────────────┘            │
        │                   │                             │
        └───────────────────┼─────────────────────────────┘
                            │
        ┌───────────────────▼─────────────────────────────┐
        │ CPU (12 threads)                                │
        │  ┌─────────────────────────────────────┐        │
        │  │ Data Pipeline (parallel)            │        │
        │  │ - Read JSONL training pairs         │        │
        │  │ - Tokenize via sentencepiece        │        │
        │  │ - Batch assembly                    │        │
        │  │ - Shuffle + epoch management        │        │
        │  └─────────────────────────────────────┘        │
        │  ┌─────────────────────────────────────┐        │
        │  │ Gradient Accumulation               │        │
        │  │ - Receive gradients from GPU        │        │
        │  │ - Update LoRA weights (Adam step)   │        │
        │  │ - Push updated LoRA back to GPU     │        │
        │  └─────────────────────────────────────┘        │
        └─────────────────────────────────────────────────┘

        ┌─────────────────────────────────────────────────┐
        │ iGPU (2 CUs, shared RAM) — OPTIONAL             │
        │  - Tokenization acceleration                    │
        │  - Embedding lookup offload                     │
        │  - Or: unused (2 CUs negligible vs 54 CU dGPU)  │
        └─────────────────────────────────────────────────┘
```

### Memory Budget

| Component | Location | Size | Notes |
|-----------|----------|------|-------|
| Base model (Q5_K_M) | GPU VRAM | ~5GB | Frozen, loaded once |
| KV cache (ctx=2048) | GPU VRAM | ~2GB | Per-batch attention cache |
| Activation scratch | GPU VRAM | ~3GB | Forward/backward temp buffers |
| **GPU total** | | **~10GB** | Fits in 12GB with 2GB headroom |
| LoRA A matrices (rank 16) | RAM | ~8MB | 32 layers × 4 targets × rank × dim |
| LoRA B matrices (rank 16) | RAM | ~8MB | Same |
| Optimizer states (Adam) | RAM | ~32MB | 2 moments per parameter |
| Training batch | RAM | ~50MB | Tokenized input sequences |
| Gradient buffer | RAM | ~32MB | LoRA gradients from GPU |
| Tokenizer (sentencepiece) | RAM | ~5MB | Mistral tokenizer model |
| **RAM total** | | **~135MB** | Fits trivially in 14GB |

**This is why Python failed and Rust will succeed:** Python loads 15GB of fp16 to RAM then quantizes. Rust loads 5GB of already-quantized GGUF directly to GPU. RAM usage: 135MB vs 15GB.

### GGUF Direct Loading

The GGUF format stores quantized tensors in a memory-mappable layout. Rust can:
1. `mmap()` the GGUF file (zero-copy, OS manages paging)
2. Send quantized tensor blocks directly to GPU via HIP
3. Never decompress to fp16 in RAM

```rust
// Pseudocode — actual implementation in src/gguf.rs
let mmap = unsafe { MmapOptions::new().map(&file)? };
let tensors = GgufReader::new(&mmap).parse_tensors()?;

for tensor in &tensors {
    // Send quantized blocks directly to GPU — no fp16 conversion
    hip_memcpy_htod(gpu_ptr, tensor.data_ptr(), tensor.byte_size())?;
}
```

### LoRA Mathematics

LoRA decomposes weight updates as: `W' = W + α/r × (B × A)`

Where:
- `W` is the frozen base weight (stays on GPU, quantized)
- `A` is a `dim × rank` matrix (trainable, in RAM)
- `B` is a `rank × dim` matrix (trainable, in RAM)
- `r` is the LoRA rank (16)
- `α` is the scaling factor (32)

For Mistral-7B with rank 16, targeting Q/K/V/O projections across 32 layers:
- Parameters per layer: 4 × (4096 × 16 + 16 × 4096) = 4 × 131,072 = 524,288
- Total trainable: 32 × 524,288 = 16,777,216 (~16M params, ~32MB fp16)
- Compare to base: 7.24B params. LoRA trains 0.23% of the model.

### Training Loop

```
for epoch in 0..num_epochs:
    for batch in training_data.batches(batch_size=1):
        1. Tokenize batch on CPU (12 threads)
        2. Copy token IDs to GPU
        3. Forward pass through frozen base + LoRA adapters (GPU)
        4. Compute cross-entropy loss (GPU)
        5. Backward pass — compute gradients for LoRA only (GPU)
        6. Copy LoRA gradients to CPU
        7. Adam optimizer step on CPU (update LoRA A and B)
        8. Copy updated LoRA to GPU
        9. Log loss every N steps
    
    Save checkpoint (LoRA adapters as GGUF)
    Run eval on held-out set
```

### Output Format

The trained LoRA adapter is saved as a GGUF file:
```
/var/zhen/models/kingdom-lora-r16-e2.gguf  (~32MB)
```

Served by llama-server:
```bash
./bin/llama-server \
  -m /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf \
  --lora /var/zhen/models/kingdom-lora-r16-e2.gguf \
  -ngl 40 -c 16384 --port 20100
```

### Crate Structure

```
crates/zhenai-forge/
├── Cargo.toml
├── src/
│   ├── main.rs          # CLI entry point
│   ├── gguf.rs          # GGUF file reader (mmap, tensor extraction)
│   ├── lora.rs          # LoRA adapter matrices + forward/backward
│   ├── optim.rs         # Adam optimizer (CPU-side)
│   ├── data.rs          # Training data loader (JSONL → tokens)
│   ├── tokenizer.rs     # Sentencepiece tokenizer binding
│   ├── hip.rs           # ROCm/HIP GPU interface (FFI)
│   ├── train.rs         # Training loop orchestration
│   └── eval.rs          # Evaluation + metrics
└── tests/
    ├── test_gguf.rs
    ├── test_lora.rs
    └── test_training.rs
```

### Dependencies

| Crate | Purpose | Age Check |
|-------|---------|-----------|
| `memmap2` | Memory-mapped file I/O | 2020 (fork of memmap, 2015) — PASS |
| `half` | f16 type support | 2017 — PASS |
| `rayon` | Parallel data loading | 2016 — PASS |
| `serde` + `serde_json` | JSONL parsing | 2015 — PASS |
| `hip-sys` or raw FFI | ROCm GPU access | Build our own FFI if too new |

All deps predate July 2019 or we build our own.

## Implementation Phases

### Phase 1: GGUF Reader + LoRA Math (1 session)
- Parse GGUF file format in Rust
- Implement LoRA A/B matrix initialization
- Unit tests for tensor shapes and LoRA forward pass

### Phase 2: GPU Interface (1 session)
- HIP FFI for GPU memory allocation and kernel dispatch
- Load quantized tensors to VRAM via mmap → HIP
- Verify model loads in 5GB VRAM

### Phase 3: Training Loop (1-2 sessions)
- Forward pass through frozen model + LoRA
- Cross-entropy loss computation
- Backward pass for LoRA gradients only
- Adam optimizer step on CPU
- Checkpoint saving as GGUF LoRA

### Phase 4: Data Pipeline + CLI (1 session)
- Sentencepiece tokenizer binding
- JSONL training data reader with batching
- CLI with progress reporting
- Eval on held-out set

### Phase 5: Optimization + Production (1 session)
- Benchmark against llama-finetune
- Memory profiling (must stay under 14GB RAM)
- A/B test fine-tuned model vs baseline
- Deploy and update ADR-018

## Success Criteria

1. `zhenai-forge train` completes on WEST (14GB RAM, RX 7700 XT)
2. Training runs in <4 hours for 3965 examples, 2 epochs
3. Peak RAM usage <6GB (leaving headroom for OS)
4. Fine-tuned model scores >10% better on Kingdom questions
5. Output LoRA loads in llama-server without modification
6. Zero Python in the training pipeline

## Consequences

### Positive
- Unblocks RAFT fine-tuning on current hardware
- Zero Python dependency for training (Rust binary only)
- Exploits all hardware: CPU + dGPU + RAM efficiently
- Foundation for future Zhenai Engine (custom inference)
- Kingdom IP: our own training tool, not a wrapper

### Negative
- Significant Rust engineering effort (5-8 sessions)
- Must implement LoRA math + gradient computation from scratch
- HIP/ROCm FFI is non-trivial
- No ecosystem support if we hit GPU kernel bugs

### Risks
- ROCm HIP FFI may have undocumented quirks on gfx1101
  - Mitigate: fall back to CPU-only training (slow but correct)
- GGUF format may change in future llama.cpp versions
  - Mitigate: pin to GGUF V3, version-check on load
- Gradient computation bugs could produce garbage LoRA
  - Mitigate: verify against llama-finetune output on small dataset
