# WAVE 10E — Gemma 4 E2B Forge Adaptation

**Date:** 2026-04-17
**Status:** PLANNED
**Predecessor:** Wave 10D (12 commits, pipeline E2E, Mistral-7B doesn't learn on 14GB APU)
**Decision:** Adapt zhenai-forge for Gemma 4 E2B instead of fighting Mistral-7B VRAM ceiling

---

## WHY

Mistral-7B Q5_K_M (4.8 GB) leaves insufficient VRAM on west (12.87 GB AMD Radeon RX 7700 XT) for full forward cache. Training with `fwd_cache=0` runs but doesn't learn (loss flat at 14.37 × 3 epochs). `fwd_cache>0` diverges due to forward/backward path mismatch with partial layer coverage. The hardware ceiling is real and architectural — no code fix closes the gap.

Gemma 4 E2B at Q4_0 = **3.2 GB**. Leaves ~9.7 GB for full fwd_cache + bw_attn + working buffers. Every layer cached, real forward, real backward. No partial-coverage hacks.

## VARIABLES

| Var | Value |
|-----|-------|
| MODEL | Gemma 4 E2B (2B effective params, Q4_0 quantization) |
| GGUF_SIZE | ~3.2 GB |
| HOST | west (14 GB RAM, AMD Radeon RX 7700 XT, 12.87 GB VRAM, ROCm 6+) |
| DATASET | /var/zhen/raft_dataset_v4.jsonl (15991 examples) |
| FORGE | crates/zhenai-forge/ (Rust, custom training loop) |
| INFRA_REUSE | bf16 storage, hipblasGemmEx, Path C lazy, Path 1 mem::take, harness scripts |

---

## PHASE 0: RECON (~2h)

Goal: Understand Gemma 4 architecture before touching code.

- [ ] **Step 1** Download Gemma 4 E2B GGUF from HuggingFace or community conversion
  - Check: bartowski, TheBloke, or official Google repos for GGUF
  - If no GGUF exists: convert via llama.cpp `convert_hf_to_gguf.py`
  - Target: Q4_0 or Q5_K_M quantization
  - Verify: `ls -lh` shows ~3-5 GB

- [ ] **Step 2** Read Gemma 4 architecture spec (model card + HF config.json)
  - Attention: multi-head? grouped-query? multi-query? head counts?
  - FFN: SwiGLU? GeGLU? gate/up/down structure?
  - Normalization: RMSNorm? LayerNorm? pre-norm or post-norm?
  - Embeddings: PLE (Per-Layer Embeddings) — unique to Gemma 4 small models
  - Vocabulary size + tokenizer type (SentencePiece)
  - Number of layers, hidden dim, intermediate dim

- [ ] **Step 3** Inspect GGUF tensor names via `zhenai-forge info <model.gguf>`
  - Map Gemma tensor names to forge's expected `blk.N.attn_q.weight` etc.
  - Identify any new tensor types (PLE embeddings, rotary embedding params)
  - Check quantization type per tensor

- [ ] **Step 4** Read llama.cpp Gemma 4 support (if merged)
  - `git log --oneline --grep="gemma"` in llama.cpp repo
  - Architecture-specific code in `llama.cpp` for attention, FFN, tokenizer

**Exit gate:** Architecture fully mapped. Tensor name mapping documented. GGUF on disk.

---

## PHASE 1: GGUF LOADER (~3h)

Goal: `zhenai-forge info <gemma4.gguf>` prints correct architecture stats.

- [ ] **Step 5** Add Gemma 4 architecture detection in `gguf.rs`
  - Key: `general.architecture` = "gemma2" or "gemma4" (check actual string)
  - Parse: layer count, hidden dim, head counts, vocab size, FFN dim

- [ ] **Step 6** Update tensor name mapping in `forward.rs`
  - Gemma may use different names: `model.layers.N.self_attn.q_proj.weight` vs `blk.N.attn_q.weight`
  - Add a name-mapping layer or architecture-aware tensor lookup

- [ ] **Step 7** Handle PLE (Per-Layer Embeddings) if present
  - E2B uses PLE: each decoder layer has its own token embedding table
  - These are large but used for lookups only (not matmul)
  - May need special handling in forward pass

- [ ] **Step 8** Update tokenizer
  - Gemma uses SentencePiece (`.model` file or embedded in GGUF)
  - Current forge tokenizer assumes Mistral format
  - May need to extract vocab from GGUF metadata differently

- [ ] **Step 9** TDD: `test_gemma4_gguf_load` — loads model, prints stats, asserts layer count

**Exit gate:** `cargo test --release test_gemma4_gguf_load` green. `info` subcommand prints correct architecture.

---

## PHASE 2: FORWARD PASS (~4h)

Goal: Forward pass produces finite logits for a Gemma 4 input sequence.

- [ ] **Step 10** Adapt `GpuModel::load_from_gguf` for Gemma 4 weight layout
  - Different tensor shapes (head_dim may differ)
  - May have different rotary embedding configuration (RoPE base, scaling)

- [ ] **Step 11** Adapt `CpuWeights::load` for Gemma 4
  - Layer count, FFN dims, attention dims
  - PLE handling (if forward uses per-layer embeddings)

- [ ] **Step 12** Adapt forward pass computations
  - Attention: check if Gemma 4 E2B uses GQA (grouped-query attention)
  - FFN: check gating variant (SwiGLU vs GeGLU)
  - RMSNorm: check epsilon, weight shape
  - RoPE: check frequency base, scaling method

- [ ] **Step 13** Adapt `dequantize_tensor` for any new quantization types
  - Q4_0 is already supported
  - Check if Gemma 4 GGUF uses any new block formats

- [ ] **Step 14** TDD: `test_gemma4_forward_finite` — 1 token in, logits out, all finite, vocab-sized

**Exit gate:** Forward pass produces finite logits. No NaN/Inf.

---

## PHASE 3: BACKWARD + LORA (~3h)

Goal: One training step completes with loss + gradient update.

- [ ] **Step 15** Adapt LoRA initialization for Gemma 4 dimensions
  - `LoraAdapters::new(n_layers, n_embd, rank)` — update dims
  - LoRA targets: Q/K/V/O attention projections (same strategy as Mistral)

- [ ] **Step 16** Adapt backward chain rule for Gemma 4 attention layout
  - `attn_only_layer_backward` may need dimension changes
  - GQA backward differs from MHA backward if head counts differ

- [ ] **Step 17** Verify bf16 storage + hipblasGemmEx works with Gemma 4 dimensions
  - Different matrix shapes may expose sgemm_bf16 edge cases
  - Run existing `test_hipblas_sgemm_bf16` (should still pass — shape-independent)

- [ ] **Step 18** Adapt `GpuTrainer::new` for Gemma 4
  - Full fwd_cache (ALL layers — we have VRAM now!)
  - bw_attn for all layers (bf16, should fit)
  - No more Path C lazy / Path 1 mem::take needed if everything fits

- [ ] **Step 19** TDD: `test_gemma4_one_step` — 1 example, 1 step, loss finite, LoRA updated

**Exit gate:** One training step completes. Loss finite. Exit 0.

---

## PHASE 4: TRAINING LADDER (~2h)

Goal: Prove learning (loss descent over multiple epochs).

- [ ] **Step 20** E0: 1 example, 1 step — code path smoke test
- [ ] **Step 21** E1: 5 examples, 5 steps — gradient flow
- [ ] **Step 22** E2: 20 examples, 10 steps — stability
- [ ] **Step 23** E3: 50 examples, 50 steps — descent signal (THIS is the real test)
- [ ] **Step 24** Multi-epoch: 50 examples × 5 epochs — must show loss decrease epoch-over-epoch

**Exit gate:** E3 shows descent. Multi-epoch loss decreases. Optimizer verified working.

---

## PHASE 5: FULL TRAINING + A/B (~4h)

Goal: Produce a usable Gemma 4 E2B LoRA for Kingdom Q&A.

- [ ] **Step 25** Full RAFT training: 500+ examples, 3+ epochs, lr=3e-4
- [ ] **Step 26** Save LoRA as GGUF
- [ ] **Step 27** A/B test: base Gemma 4 E2B vs trained LoRA on 10 Kingdom questions
  - Use existing `zhenai-forge eval` subcommand
  - Compare: answer quality, relevance, hallucination rate
- [ ] **Step 28** Wire into Zhen inference pipeline (llama.cpp or forge eval mode)
- [ ] **Step 29** Commit final LoRA to `/var/zhen/models/kingdom-gemma4-lora.gguf`

**Exit gate:** A/B test shows trained LoRA outperforms base on Kingdom questions.

---

## PHASE 6: INTEGRATION + DOCS (~1h)

- [ ] **Step 30** Update CLAUDE.md with Gemma 4 status
- [ ] **Step 31** Update Zhen MCP server to point at Gemma 4 model + LoRA
- [ ] **Step 32** Wiki page: `wiki/Wave-10E-Gemma4.md`
- [ ] **Step 33** Final commit: "Wave 10E complete: Gemma 4 E2B LoRA shipped"

---

## INFRASTRUCTURE REUSE (from Wave 10D)

These carry over unchanged:

| Component | Commit | Status |
|-----------|--------|--------|
| bf16 storage + hipblasGemmEx | `ba3e922c` | ✅ |
| Path C lazy re-dequant | `604ebd69` | ✅ (may not need if all layers fit) |
| Path 1 mem::take | `9d5ec084` | ✅ (may not need if all layers fit) |
| Per-step lazy cache | `d28921ba` | ✅ (may not need) |
| forge-train.sh + watchdog | `16fd732a` + `b57f166c` | ✅ |
| Budget gate + /proc/meminfo | `9d27dec2` | ✅ |
| Fwd/bwd gradient consistency fix | `9fd6856c` | ✅ |
| FORGE_FWD_CACHE_LAYERS env | `9d5ec084` | ✅ (set to ALL for Gemma 4) |
| FORGE_MAX_LOSS_POSITIONS env | `3f6e8a81` | ✅ (can increase now) |

## ESTIMATED TOTAL: ~19h across 3-4 sessions

| Phase | Hours | Blocker |
|-------|-------|---------|
| 0 Recon | 2 | GGUF availability |
| 1 Loader | 3 | Architecture mapping |
| 2 Forward | 4 | Largest code change |
| 3 Backward + LoRA | 3 | Dimension adaptation |
| 4 Ladder | 2 | Verification |
| 5 Full training + A/B | 4 | Quality gate |
| 6 Integration | 1 | Docs |

## HARD KILL CRITERIA

- K1: GGUF not available and conversion fails → STOP, wait for community
- K2: Gemma 4 architecture too different from transformer-decoder (unlikely) → reassess
- K3: Q4_0 still doesn't fit in VRAM with full cache → try Q2_K or E2B at lower quant
- K4: Loss doesn't descend after 50 steps with full forward → architecture bug in adaptation
