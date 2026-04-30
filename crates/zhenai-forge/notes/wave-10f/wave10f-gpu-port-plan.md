# WAVE10F Phase 7 — Gemma 4 GPU Port Plan

**Status:** ✅ DONE 2026-04-20 (Steps A–E all landed; ~2s/step warm achieved)
**Goal:** make forge train Gemma 4 E2B at a tractable pace (target: ≤2s/step on west's RX 7700 XT, vs current 55-95s/step on CPU)

**Outcome:** warm-path 2.05s/step on real `/var/zhen/models/gemma-4-E2B-it.gguf`. Target met. See `wave10f-step-c-timing.md` for full numbers.
**Why now:** profile data (commit `bde57ebd`) showed the 4-token Gemma 4 forward spends ~95% of its time in bf16→f32 conversion + CPU matmul. The actual matmul math is sub-second; the conversion + memory-bandwidth-bound CPU compute is the bottleneck. GPU sgemm_bf16 reads bf16 directly, eliminating both.

## Target architecture

**`Gemma4GpuWeights`** — sibling struct to `CpuWeightsGemma4`. Holds `GpuBuffer` for each weight tensor instead of `Vec<bf16>`. Built from a `CpuWeightsGemma4` via an `upload()` function. Stays resident across training steps.

Memory budget on west (12.87 GB VRAM):
- token_embd: 805 MB
- per_layer_token_embd (PLE): 4.7 GB
- per_layer_model_proj: 27 MB
- 35-layer attention + FFN weights: ~3 GB (per estimate)
- Activations + LoRA + transient: 2 GB headroom
- **Total: ~10.5 GB. Fits in 12.87 GB with 2.3 GB safety margin.**

If tight, fall back: leave PLE on CPU, upload-on-demand per token. Saves 4.7 GB.

## API additions (in `crates/zhenai-forge/src/gemma4_gpu.rs` — new module)

```rust
pub struct Gemma4GpuWeights {
    pub hparams: Gemma4Hparams,
    // Globals
    pub token_embd: GpuBuffer,
    pub output_norm: GpuBuffer,
    pub per_layer_token_embd: Option<GpuBuffer>,  // None if PLE-on-CPU mode
    pub per_layer_model_proj: GpuBuffer,
    pub per_layer_proj_norm: GpuBuffer,
    pub rope_freqs: GpuBuffer,
    // Per-layer (Vec of GpuBuffer per attribute)
    pub attn_norm: Vec<GpuBuffer>,
    pub wq: Vec<GpuBuffer>,
    pub wk: Vec<Option<GpuBuffer>>,
    pub wv: Vec<Option<GpuBuffer>>,
    pub wo: Vec<GpuBuffer>,
    pub attn_q_norm: Vec<GpuBuffer>,
    pub attn_k_norm: Vec<Option<GpuBuffer>>,
    pub post_attention_norm: Vec<GpuBuffer>,
    pub ffn_norm: Vec<GpuBuffer>,
    pub ffn_gate: Vec<GpuBuffer>,
    pub ffn_up: Vec<GpuBuffer>,
    pub ffn_down: Vec<GpuBuffer>,
    pub post_ffw_norm: Vec<GpuBuffer>,
    pub inp_gate: Vec<GpuBuffer>,
    pub proj: Vec<GpuBuffer>,
    pub post_norm: Vec<GpuBuffer>,
    pub layer_output_scale: Vec<Option<GpuBuffer>>,
    pub blas: BlasHandle,
}

impl Gemma4GpuWeights {
    pub fn upload(cpu: &CpuWeightsGemma4) -> Result<Self, String>;
    pub fn vram_used(&self) -> u64;
}

pub fn forward_gemma4_gpu(
    weights: &Gemma4GpuWeights,
    lora: Option<&Gemma4LoraAdapters>,  // LoRA stays on CPU (small)
    tokens: &[u32],
) -> (Vec<f32>, Vec<Gemma4GpuLayerCache>);  // logits + activation cache for backward

pub fn backward_gemma4_gpu(
    weights: &Gemma4GpuWeights,
    lora: Option<&mut Gemma4LoraAdapters>,
    caches: &[Gemma4GpuLayerCache],
    logits: &[f32],
    tokens: &[u32],
    answer_start: usize,
) -> (f32, Vec<LayerGradHealth>);

pub fn train_step_gemma4_gpu(...) -> f32;  // analogous wrapper
```

## Strategy: hybrid GPU/CPU

Phase 7a (this plan):
- All weight matmuls on GPU via `hip::sgemm_bf16` (reads bf16 directly — no f32 conversion)
- RMSNorm, softmax (in attention), RoPE, GELU, mul: stay on CPU
- Attention scores Q@K^T: GPU matmul (hipblasGemmEx with bf16)
- Activations move CPU↔GPU per call (small for short seq)
- LoRA: CPU contribution computed and added to base output after download

Phase 7b (future, if needed):
- Move RMSNorm + softmax + GELU to GPU kernels (write custom HIP kernels)
- Eliminate CPU↔GPU shuffling per call
- Fully GPU-resident activations

## Critical replacements (forward path)

Each `matmul_x_wt(...)` call site becomes:

```rust
// Before (CPU):
let wq_f32 = bf16_to_f32_vec(&weights.wq[il]);
let q = matmul_x_wt(&normed, &wq_f32, seq, q_out_dim, n_embd);

// After (GPU):
let q = self.blas.sgemm_bf16(
    &normed_gpu,        // input on GPU
    &gpu_weights.wq[il], // weight resident on GPU
    seq, q_out_dim, n_embd,
)?;
let q = q.to_host();    // download for CPU norm/RoPE/etc
```

Or batch: keep activations on GPU through a contiguous block of matmul ops.

## Phase 7 acceptance gate

Same `test_gemma4_train_step_loss_descent` semantics, but GPU-backed:
- Loss descends over 3 steps on real GGUF
- Step time ≤ 2s per step (target — ~30x speedup from current ~55s)
- All previous tests still pass

## Risks

- **R1 — VRAM overflow on PLE:** 4.7 GB just for the table. Mitigation: PLE-on-CPU fallback (keeps it as bf16 on CPU, look up + upload per-token slices ~17 KB each).
- **R2 — Per-call upload/download overhead:** small activations (≤24 KB at seq=4) are fast over PCIe (~100 µs). For larger seq, batch transfers.
- **R3 — sgemm_bf16 wrapper bugs:** existing wrapper is tested on Mistral path. Re-verify on Gemma 4 dimensions (per-layer-variable head_dim).
- **R4 — Backward complexity:** more matmul backward sites than forward (matmul_grad_x reconstructs many). Each becomes a sgemm with transposed args. Audit carefully.

## Estimated scope

- New module + GpuBuffer wiring: ~400 LOC
- forward_gemma4_gpu rewrite of forward_gemma4_with_lora using GPU calls: ~400 LOC
- backward_gemma4_gpu: ~500 LOC
- Tests: ~150 LOC
- Memory budget verification + tuning: ~150 LOC

Total: ~1500 LOC, 2-4 focused sessions. Verifiable at every milestone (each test passes incrementally).

## Sequencing within Phase 7

1. **Step A — Gemma4GpuWeights upload + vram budget check.** ✅ DONE (commit `c9180e42`).
2. **Step B — forward_gemma4_gpu (full forward).** ✅ DONE (commits `6c49d671`, `04e441fb`). 10.7× forward speedup, top-5 logit match on real GGUF.
3. **Step C — backward_gemma4_gpu.** ✅ DONE 2026-04-20 — all 14 matmul sites in `backward_gemma4_with_lora` now dispatch to `Gemma4GpuWeights::matmul_grad_x` / `matmul_xwt` behind an `Option<&Gemma4GpuWeights>` param. Every site verified at cosine ≥ 0.999998 vs CPU (Phase 1 harness, commit `63a2af6b`). Loss descent preserved. Commits: `dbec6c84` (3A), `1cb10a73` (3B), `2c51ef1e` (3C), `66d0b7b7` (3D), `f35eb04a` (3E), `a5147aee` (3F).
4. **Step D — train_step_gemma4_gpu + loss descent.** ✅ DONE 2026-04-20 (commits `152fc95c` flip, `d1b1f21f` canonical entry + test). 2.05s/step warm on real GGUF. 10-step descent: 19.96 → 0.043 (465× reduction). Phase 5 perf gate met 7× below plan target.
5. **Step E — train-gemma4 CLI uses GPU path by default.** ✅ DONE 2026-04-20 (commit `bd65ffce`). `--cpu` flag falls back to the CPU path; upload failure also falls back with a warning.

Cleanup (commit `06a16f67`): deleted the stale `train_step_gemma4_hybrid` shim — the name is no longer accurate now that backward is GPU-accelerated too.

## After Phase 7

- Phase 7.1: Gemma 4 SentencePiece tokenizer (so `--data` can take text JSONL, not pre-tokenized)
- Phase 7.2: Run actual Kingdom Q&A RAFT training on full 15991 examples × 3 epochs (now feasible at <2s/step → ~25 hours)
- Phase 7.3: A/B test trained LoRA vs base on hand-picked Kingdom questions
- Phase 7.4: Save → llama.cpp interop for inference

---

*Plan written 2026-04-18 after profiling identified bf16 conversion as bottleneck. Existing GpuBuffer/BlasHandle/sgemm_bf16 infrastructure in `crates/zhenai-forge/src/hip.rs` is the foundation — it was built for Wave 10D and proven to work on the Mistral path.*
