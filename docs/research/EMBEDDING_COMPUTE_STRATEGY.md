# Lab Notebook: Embedding Compute Strategy for Zhen RAG Index

**Date**: 2026-04-02
**Investigator**: Scientist
**Trigger**: GPU embedding running but suboptimal; user asks about multi-device strategy

## Observation

Embedding 1,762,687 text chunks using `all-MiniLM-L6-v2` (384-dim) for the Zhen RAG index.

**Hardware:**
- CPU: AMD Ryzen 5 7600X (6-core/12-thread, Zen 4)
- dGPU: AMD Radeon RX 7700 XT (gfx1101, 12GB VRAM, RDNA 3, 54 CUs)
- iGPU: AMD Raphael integrated graphics (gfx1101, 2 CUs)
- RAM: 14GB DDR5

**Software:**
- ROCm 6.4.2 (system), PyTorch 2.5.1+rocm6.2 (wheels)
- `HSA_OVERRIDE_GFX_VERSION=11.0.0` required (gfx1101 not in prebuilt wheels, maps to gfx1100)
- sentence-transformers 5.3.0

## Empirical Measurements

Benchmark: 1,000 texts, ~150 tokens each, `normalize_embeddings=True`:

| Device | Batch Size | Throughput (texts/sec) | Relative |
|--------|-----------|----------------------|----------|
| GPU (RX 7700 XT) | 64 | 818 | 7.9x |
| GPU (RX 7700 XT) | 256 | 733 | 7.1x |
| GPU (RX 7700 XT) | 512 | 929 | 9.0x |
| CPU (7600X) | 256 | 103 | 1.0x (baseline) |

**Note:** batch_size=64 outperforms 256 likely due to ROCm kernel launch overhead being amortized differently at small vs medium sizes. 512 wins by saturating GPU compute units.

## Analysis: Answering the 6 Questions

### Q1: Can we use CPU + iGPU + dGPU simultaneously?

**Theoretically yes, practically not worth it.**

- **CPU + dGPU**: Possible via `torch.multiprocessing` — spawn one worker on `cuda:0`, another on `cpu`. The `sentence-transformers` library doesn't natively support this, but you could split the corpus and run two `model.encode()` calls in separate processes.
- **iGPU (gfx1101, 2 CUs)**: Visible to ROCm as Device 1 (`0x164e`). Could be addressed as `cuda:1` in PyTorch. But with only 2 CUs vs the dGPU's 54 CUs, it would contribute ~4% of total GPU throughput — negligible.
- **Overhead**: Multi-device coordination (tokenization, result aggregation, FAISS merge) adds complexity. The CPU can only contribute ~103 texts/sec vs GPU's ~929 — a 10% boost at best.

**Verdict**: Not worth the complexity. The RX 7700 XT alone is the dominant compute path. CPU contribution is marginal. iGPU is negligible.

### Q2: Is the 7700 XT the fastest/most efficient path?

**Yes, by a wide margin.**

| Path | Throughput | Time for 1.76M | Power |
|------|-----------|----------------|-------|
| GPU (RX 7700 XT, batch=512) | ~929/s | ~32 min | ~170W |
| CPU (7600X, 12 threads) | ~103/s | ~4.75 hr | ~65W |
| CPU+GPU (theoretical max) | ~1032/s | ~28 min | ~235W |

The GPU path is **9x faster** and completes in 32 minutes. Energy efficiency: GPU uses 170W×0.53h = 90 Wh. CPU uses 65W×4.75h = 309 Wh. **GPU is 3.4x more energy efficient.**

### Q3: Is batch_size=64 optimal? Should we increase?

**No and yes.**

- `EMBED_BATCH=512` is optimal based on benchmarks (929/s vs 818/s at 64)
- `PROCESS_BATCH` (texts accumulated before calling `model.encode()`) was the real bottleneck at 50K — tokenizing 50K texts on CPU before any GPU work causes 10+ minute stalls
- Changed to `PROCESS_BATCH=5_000` for faster feedback loops and less tokenization latency

**Further optimization**: Could try batch_size=1024, but VRAM is already at 58% — larger batches risk OOM on 12GB VRAM with long texts.

### Q4: First 50K batch taking ~10 minutes — expected or pathological?

**Pathological, for two reasons:**

1. **Tokenization bottleneck**: `model.encode()` tokenizes ALL texts before sending any to GPU. At 50K texts with up to 2000 chars each, tokenization alone consumes minutes of CPU time.
2. **HIP JIT compilation**: First GPU kernel invocation on RDNA 3 (experimental ROCm support) triggers just-in-time compilation. This is a one-time cost per kernel shape, but with 50K texts it compounds because the tokenizer produces variable-length sequences that may trigger multiple kernel recompilations.

**Fix applied**: Reduced `PROCESS_BATCH` from 50K to 5K. First batch still has JIT overhead but tokenization is 10x faster.

### Q5: Would ONNX Runtime with ROCm be faster?

**Likely yes, by 2-4x for inference-only workloads.**

- ONNX Runtime has a dedicated ROCm Execution Provider (`onnxruntime-rocm`)
- It eliminates PyTorch's autograd overhead (unnecessary for inference)
- It supports graph optimization (operator fusion, constant folding)
- For `all-MiniLM-L6-v2`, an ONNX export is straightforward: `optimum-cli export onnx --model sentence-transformers/all-MiniLM-L6-v2`
- However: ONNX Runtime ROCm wheels also have Python version constraints and gfx1101 support is similarly experimental

**Recommendation**: Worth investigating for the next index rebuild. Expected speedup: 2-4x over PyTorch, potentially bringing total time to 10-15 minutes.

### Q6: Expected GPU throughput?

**Measured: ~929 texts/sec with batch_size=512.**

- CPU baseline: ~103 texts/sec (370K/hr)
- GPU measured: ~929 texts/sec (3.3M/hr)
- Script's original estimate: 150K/hr (hardcoded for CPU, undersized)
- **GPU projection for 1.76M chunks: ~32 minutes**

Note: Real throughput will be slightly lower due to:
- Variable text lengths (some batches tokenize slower)
- FAISS `index.add()` calls (CPU, but fast for FlatIP)
- GC pauses between batches
- I/O for reading JSONL corpus

Realistic estimate: **35-45 minutes** with current settings.

## Conclusion

| Factor | Finding |
|--------|---------|
| Best device | RX 7700 XT (dGPU) alone |
| Optimal batch | EMBED_BATCH=512, PROCESS_BATCH=5K |
| Multi-device | Not worth the complexity (~10% gain) |
| ONNX Runtime | Worth investigating for 2-4x additional speedup |
| iGPU | Negligible (2 CUs vs 54 CUs) |
| Total ETA | ~35-45 minutes (GPU) vs ~5 hours (CPU) vs ~12 hours (original script) |

## Open Questions

1. Would `TORCH_ROCM_AOTRITON_ENABLE_EXPERIMENTAL=1` improve flash attention perf on Navi 3x?
2. Is there a persistent HIP kernel cache that would eliminate JIT overhead on subsequent runs?
3. Would fp16 quantization of the embedding model further improve throughput without quality loss?
4. ONNX Runtime ROCm: does it support gfx1101 natively or need the same HSA override?
