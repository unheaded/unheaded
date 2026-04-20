# WAVE 10F — Forge Real Attention Forward+Backward for Gemma 4 E2B

**Status:** Phases 1-7 all landed as of 2026-04-20. Phase 7 GPU port (Steps A-E) complete — 2.05s/step warm on real `/var/zhen/models/gemma-4-E2B-it.gguf`, full loss descent verified, CLI defaults to GPU. Real-Kingdom-Q&A fine-tuning is now the next track.
**Estimated scope:** 10-14 weeks of focused work, blocks other Zhen tracks
**Decision date:** 2026-04-17
**Supersedes:** `WAVE10E-GEMMA4-FORGE.md` (the original Gemma 4 plan, invalidated when grad-norm instrumentation revealed forge's simplified backward as the real blocker, not VRAM)
**Companion docs:**
- `crates/zhenai-forge/notes/gemma4-arch-spec.md` — verified hparams + arch spec from llama.cpp source
- `crates/zhenai-forge/notes/wave-10d-postmortem/` — prior memory-budget findings

**Decision basis:** Stevie selected the multi-month path on 2026-04-17 after Experiment B confirmed forge's simplified backward produces NaN gradients in 21 of 32 layers, and Gemma 4's architectural novelty was confirmed via Google's official model card.

---

## Context

The WAVE10E forge bug hunt (commit `2e0c44f5`) established three facts that drove this plan:

1. **Forge's `attn_only_layer_backward` is fundamentally simplified** — sums `W_t^T @ grad * 0.25` across all 4 attention targets without per-layer normalization. Gradient magnitude grows ~5×/layer; exceeds f32 viable range at iteration 12 of the reverse loop. The NaN-grad guard committed in `2e0c44f5` prevents weight poisoning but only allows the late 11 layers (L21-L31) to train. Bug A is architectural, not a one-line fix.

2. **Forge's forward is also simplified** — when `fwd_cache=0` (the working memory config), `train.rs:973-1003` does LoRA-only forward with no real Q/K/V/softmax/RoPE for any layer outside the GPU cache. There is no real attention math anywhere in the current pipeline.

3. **Gemma 4 was confirmed real** (verified via blog.google + ai.google.dev model card, Apr 2 2026 release; HF safetensors clone validated 2026-04-17). E2B is the relevant size for west's 12.87 GB VRAM, but its architecture is novel: hybrid local-sliding-window + full-global attention, Per-Layer Embeddings (PLE), Proportional RoPE (p-RoPE) on global layers, unified K/V across global layers (`num_kv_shared_layers=20`), 262K SentencePiece vocab, multimodal (text + image + audio encoders even on text-only training), `gelu_pytorch_tanh` activation, final logit softcapping at 30.0.

The intended outcome: a fully custom-Rust Gemma 4 E2B trainer with mathematically correct forward + backward, producing a usable Kingdom Q&A LoRA on west, with no dependency on PyTorch/transformers/Unsloth for training. Forge becomes a real transformer trainer, not a LoRA-residual approximator.

**Off-ramp clauses.** This plan should be abortable at any phase boundary. If Phase 1 (vanilla GQA) takes >4 weeks instead of 3, or if Phase 4 (p-RoPE / unified K/V) reveals a math derivation that exceeds available time, fall back to Path C (Unsloth/transformers+peft+bitsandbytes-rocm with day-one Gemma 4 ecosystem support) — preserving the 11-layer Mistral training that already works under the NaN guard.

---

## Critical files

**Forge source (all in `crates/zhenai-forge/src/`):**
- `forward.rs` — has `embedding_lookup`, `rmsnorm`, `silu`, `softmax`, `ffn_forward`, `cross_entropy_loss`, `matmul_cpu`, `dequantize_tensor`. Missing: real attention forward, GELU-tanh-approx, RoPE, p-RoPE, GQA expand, sliding-window mask, logit softcap.
- `backward.rs` — has `cross_entropy_softmax_backward`, `matmul_backward_a/b`, `lora_backward`, `rmsnorm_backward`, `silu_backward`, `attn_only_layer_backward` (the simplified one), `transformer_layer_backward_with_saved` (unused stub). Missing: softmax backward (Jacobian-vector), scaled-dot-product backward, RoPE backward, GQA collapse backward, p-RoPE backward, PLE backward, unified-KV backward, sliding-window-mask handling, GELU-tanh-approx backward, logit-softcap backward.
- `train.rs` — main training loop. Forward at `:961-1003` (LoRA-only when fwd_cache=0). Backward at `:1073-1158`. Optimizer block at `:1162-1195`. NaN-grad guard already in `lora.rs:adam_step`.
- `lora.rs` — `LoraLayer` (a/b/m/v/grad), `LoraAdapters` collection, `adam_step` with NaN guard. Will need extension for new LoRA targets per-layer dispatch (per-layer Q dims).
- `gguf.rs` — Mistral-arch hardcoded tensor names. Needs Gemma 4 architecture detection + tensor mapping (note: Gemma 4's wq has different output dim per layer type).
- `tokenizer.rs` — current implementation Mistral-specific. Gemma 4 uses 262K SentencePiece — separate path.
- `hip.rs` — GPU primitives: `hipMalloc`, `hipMemcpy`, `hipblasSgemm`, `hipblasGemmEx` (bf16). Will need: softmax kernel (CPU fallback OK), batched matmul for per-head attention, possibly sliding-window mask kernel.
- `quant.rs` — Q5_K, Q6_K, F32 dequant. Gemma 4 GGUFs (after conversion + quantize) may use Q4_0 / Q4_K_M / Q8_0 — verify and add as needed.

**Reference implementations (read, do not modify):**
- `~/tmp/unheaded/llama.cpp/src/models/gemma4-iswa.cpp` (322 LOC, post-pull `30dce2c`) — Gemma 4 forward graph, source of truth for arch
- `~/tmp/unheaded/llama.cpp/src/models/gemma3.cpp`, `gemma3n-iswa.cpp`, `gemma2-iswa.cpp` — earlier-generation references for sliding-window patterns and PLE
- `~/tmp/unheaded/llama.cpp/convert_hf_to_gguf.py` — `Gemma4Model(Gemma3Model)` registration at line 7666; `Gemma4VisionAudioModel` at 7791
- `~/tmp/unheaded/crates/zhenai-forge/notes/wave-10d-postmortem/` — prior memory-budget findings, livelock root cause, Path C lazy-dequant rationale
- `~/tmp/unheaded/crates/zhenai-forge/notes/gemma4-arch-spec.md` — verified E2B hparams + ops sequence

**Source HF clones** (already on disk):
- `/home/govan/tmp/gemma-4-E2B-it/` (NVMe SSD copy, primary)
- `/var/gemma/` (HDD copy, backup; root-owned)
- Both verified identical via config.json sha256

**Existing utilities to reuse:**
- `forward::softmax` (`forward.rs:80`) — exists, used today for cross-entropy. Reuse for attention softmax (verify max-subtraction numerical stability).
- `forward::matmul_cpu` (`forward.rs:151`) — CPU fallback path.
- `backward::cross_entropy_softmax_backward` (`backward.rs:19`) — analytical softmax+CE backward for the loss layer; the *attention* softmax backward has a different structure (Jacobian-vector product) but the numerical stability tricks transfer.
- `backward::rmsnorm_backward` — already correct (verified by `test_rmsnorm_backward_numerical`); reuse unchanged.
- `lora::LoraLayer` + `adam_step` (with new NaN guard from `2e0c44f5`) — reuse unchanged.
- `hip::sgemm_bf16` (`hip.rs:186-210`) — bf16-input/fp32-output matmul, the workhorse for attention scores and projections.
- Numerical-gradient test pattern from `backward.rs:512` (`test_rmsnorm_backward_numerical`) — eps=1e-3, central diff `(f(x+eps)-f(x-eps))/(2*eps)`, relative-error threshold 0.1-0.15 for f32. Apply the same template to every new backward function.

---

## Phase 0 — Recon, GGUF conversion, environment update (week 1)

**Goal:** Local environment understands Gemma 4. Real GGUF on disk, loadable by updated llama.cpp.

**Source provenance (load-bearing — Stevie's prompt-injection warning):**
- HF safetensors already cloned from official source at `/home/govan/tmp/gemma-4-E2B-it/` and `/var/gemma/`
- Local conversion via llama.cpp's `convert_hf_to_gguf.py` — no third-party GGUF download
- Quarantine output to `/tmp/gemma4-staging/` first; only move to `/var/zhen/models/` after `forge info` succeeds and `file` reports a valid GGUF
- Do NOT pull pre-built GGUFs from `bartowski/`, `unsloth/`, `TheBloke/`, or `LM Studio Community` — local conversion only

**Steps (status as of 2026-04-17 marked):**
1. ✅ `git -C ~/tmp/unheaded/llama.cpp pull` (commit `30dce2c`)
2. ✅ Verified `LLM_ARCH_GEMMA4` constant + `Gemma4ForConditionalGeneration` registration in convert script
3. ⏳ `cd ~/tmp/unheaded/llama.cpp && cmake --build build --target llama-cli llama-quantize` (rebuild against new code)
4. ⏳ Convert HF → GGUF: `cd ~/tmp/unheaded/llama.cpp && python3 convert_hf_to_gguf.py /home/govan/tmp/gemma-4-E2B-it --outfile /var/zhen/models/gemma-4-E2B-it.gguf --outtype bf16`
5. ⏳ Optionally quantize: `~/tmp/unheaded/llama.cpp/build/bin/llama-quantize /var/zhen/models/gemma-4-E2B-it.gguf /var/zhen/models/gemma-4-E2B-it-Q4_0.gguf Q4_0`
6. ⏳ `~/tmp/unheaded/llama.cpp/build/bin/llama-cli -m /var/zhen/models/gemma-4-E2B-it*.gguf -p "test prompt"` confirms model loads and produces sane output
7. ✅ Read `~/tmp/unheaded/llama.cpp/src/models/gemma4-iswa.cpp` end-to-end → `crates/zhenai-forge/notes/gemma4-arch-spec.md` written
8. ⏳ Inspect actual GGUF metadata once converted; fill `gemma4-arch-spec.md` "Still TBD" section (exact metadata key names, tokenizer chat-template, post-conversion quant formats present)

**Exit gate:** llama.cpp inference works on Gemma 4 E2B locally. Architecture spec doc complete (no remaining TBDs).

---

## Phase 1 — Vanilla GQA real attention forward+backward (weeks 2-4)

**Goal:** Forge has mathematically correct attention forward+backward for vanilla GQA (no Gemma-specific features yet). Run against Mistral-7B GGUF as the test bed since we already have it on disk and it's pure GQA.

**1a — Forward pass new functions (in `forward.rs`):**
- `attention_forward(Q, K, V, n_heads, n_kv_heads, head_dim, mask) -> (out, scores_cache)` — full Q@K^T/sqrt(d), apply mask, softmax, @ V. Returns the output AND the post-softmax scores (needed for backward).
- `rope_apply(x, freqs_complex)` — standard RoPE using precomputed cos/sin tables.
- `gqa_expand(K, V, n_heads, n_kv_heads)` — repeat K/V `n_heads/n_kv_heads` times along the head dimension before scoring.
- Update `train.rs:961-1003` forward path to call the real attention chain instead of LoRA-only-residual.

**1b — Backward pass new functions (in `backward.rs`):**
- `softmax_backward(grad_out, softmax_out) -> grad_in` — Jacobian-vector product: `g - sum(g .* p) * p` per row, where p = softmax output.
- `attention_backward(grad_out, Q, K, V, scores) -> (grad_Q, grad_K, grad_V)` — reverse the chain through @V, softmax, scale, @K^T.
- `gqa_collapse(grad_K_expanded, grad_V_expanded, n_heads, n_kv_heads) -> (grad_K, grad_V)` — sum across the repeated heads.
- `rope_backward(grad_rotated, freqs_complex) -> grad_pre_rotation` — apply inverse rotation (RoPE's transpose for these complex rotations).
- Replace `attn_only_layer_backward` calls in `train.rs:1149` with a real per-layer attention backward that computes proper per-target gradients (no more `target_grad = grad_hidden * 0.25`).

**1c — Numerical gradient checks (`#[test]` blocks in `backward.rs`):**
- `test_softmax_backward_numerical` — central diff vs analytical, threshold 0.1
- `test_attention_backward_numerical` — small Q,K,V (e.g. seq=4, head_dim=8, n_heads=2), full backward chain check
- `test_rope_backward_numerical` — single token, single head
- `test_gqa_collapse_numerical` — verify sum across query heads matches
- `test_full_layer_backward_numerical` — end-to-end one transformer layer, gradient w.r.t. layer input, vs finite-difference

**1d — Memory budget for short-seq Mistral training:**
- Restrict `max_seq_len` to 256 tokens initially (256² × 32 heads × bf16 = 16 MB scores/layer × 32 layers = 512 MB scores cache)
- Existing fwd_cache infrastructure handles per-layer streaming; extend it to also stream attention scores out of GPU between forward and backward
- Document peak memory in notes; compare to the current Wave-10D budget (5.83 GB headroom)

**Exit gate:** All numerical gradient tests pass. Toy training (5 examples, accum=1, FORGE_DEBUG_GRAD_NORMS=1) shows `healthy=128/128 zero=0 nan=0` every step (vs current healthy=44/128 with simplified backward). Loss decreases over 50 examples on Mistral-7B with reduced sequence length.

---

## Phase 2 — Gemma 4 GGUF loader + tokenizer (week 5)

**Goal:** `forge info /var/zhen/models/gemma-4-E2B-it*.gguf` prints correct architecture stats (35 layers, hidden=1536, vocab=262144, etc per `gemma4-arch-spec.md`). Tokenizer roundtrips a Kingdom Q&A example.

**Steps:**
- Extend `gguf.rs` to detect `general.architecture = "gemma4"`.
- Add Gemma 4 tensor name mapping. Follows `blk.N.attn_q.weight` convention but with extra tensors per the spec doc: `attn_q_norm`, `attn_k_norm`, `attn_post_norm`, `ffn_post_norm`, `per_layer_inp_gate`, `per_layer_proj`, `per_layer_post_norm`, `out_scale`, `rope_freqs` (full layers only), and global tensors `per_layer_tok_embd`, `per_layer_model_proj`, `per_layer_proj_norm`.
- Implement Gemma 4 metadata parsing: layer count, sliding-window size, `num_kv_shared_layers`, p-RoPE config (`partial_rotary_factor=0.25` on full layers), PLE table dims (`hidden_size_per_layer_input=256`, `vocab_size_per_layer_input=262144`).
- Add SentencePiece-based tokenizer for the 262K vocab. Source the SP model from the GGUF metadata block.
- Test: `test_gemma4_gguf_load` — loads E2B model, asserts layer count = 35, n_embd = 1536, vocab = 262144, n_kv_shared_layers = 20.
- Test: `test_gemma4_tokenizer_roundtrip` — encode then decode sample Kingdom Q&A pair, byte-equal.

**Exit gate:** `forge info` shows correct E2B stats. Tokenizer roundtrips. No actual training yet.

---

## Phase 3 — Hybrid sliding/global attention + per-layer-variable Q dim (weeks 6-7)

**Goal:** Forward + backward correctly route through the per-layer attention type (sliding 512 vs global). Per-layer-variable Q output dim (256 sliding, 512 global) handled. Final layer always global.

**Steps:**
- Extend `attention_forward` from Phase 1 to accept a `mask_type` parameter: `Full`, `SlidingWindow(n)`. Sliding-window mask is upper-triangular AND `(j - i) < n`.
- Per-layer config table from GGUF metadata determines which layers are sliding vs global (verified pattern: every 5th layer full, indices [4, 9, 14, 19, 24, 29, 34]).
- **Per-layer Q dim dispatch:** the wq tensor has different output dim per layer (256×8=2048 sliding vs 512×8=4096 global). Forge must read the actual tensor shape per layer rather than assuming uniform.
- Backward: sliding-window mask zeros out gradients outside the window automatically (via the masked positions having softmax=0); existing softmax_backward handles this.
- Add `gelu_tanh_approx` and `gelu_tanh_approx_backward` (Gemma 4 uses `gelu_pytorch_tanh`, not plain GELU).
- Add `logit_softcap_forward` and `logit_softcap_backward` (capping at 30.0 per E2B config).
- Add `q_norm`/`k_norm` calls (Gemma 4 RMS-norms Q and K AFTER projection, before RoPE).
- Verification: forward output of forge for a fixed token sequence on a Gemma 4 layer must match llama.cpp's `llama-cli` output with `--logits-all` to within bf16 precision (~1e-2 relative).
- Test: `test_sliding_window_mask` — assert positions outside window have zero attention weight.
- Test: `test_per_layer_variable_q_dim` — load E2B, verify wq shapes match expected per-layer dims.
- Test: `test_gelu_tanh_approx_numerical`.
- Test: `test_logit_softcap_numerical`.

**Exit gate:** Forward outputs match llama.cpp reference within bf16 tolerance for both sliding and global layers. Backward gradient checks pass for sliding layers. Per-layer Q dispatch verified.

---

## Phase 4 — p-RoPE + unified K/V (weeks 8-9)

**Goal:** Global layers use p-RoPE and share K/V across layers. Both forward and backward correct.

**4a — Proportional RoPE (p-RoPE):**
- Per E2B config: `partial_rotary_factor=0.25` on full-attention layers means only 25% of `global_head_dim=512` (= 128 dims) are rotated. The remaining 384 dims pass through unchanged.
- Implement `prope_apply(x, freqs_complex, partial_rotary_factor)` in `forward.rs`: rotate first `partial_rotary_factor * head_dim` dims with standard RoPE math, leave the rest as-is.
- Implement `prope_backward` in `backward.rs`: inverse rotation on the rotated portion only; identity on the rest.
- Numerical gradient test for p-RoPE backward (`test_prope_backward_numerical`).

**4b — Unified K/V on global layers:**
- Per E2B config: `num_kv_shared_layers=20` means layers 0-19 produce their own K/V; layers 20-34 reuse from somewhere earlier. Exact source-layer mapping TBD — read from llama.cpp gemma4-iswa.cpp's iswa cache code (the `inp_attn` graph with `has_kv(il)` dispatch in build_attn).
- Forward: for KV-producing layers, compute K, V normally and cache. For KV-reusing layers, route the cached K, V into attention.
- Backward: gradient flows back through the SHARED K/V — each consuming layer contributes a `grad_K_partial`, `grad_V_partial`; sum these into the single `grad_K`, `grad_V` for the producing layer. Implement explicit accumulation step at the end of the per-layer backward loop.
- Test: `test_unified_kv_backward_sums` — multi-layer toy with 2 consumers sharing K/V; verify backward gradients on the producer K/V equal the sum of the analytical gradients from each consumer.

**Exit gate:** Toy multi-global-layer test gradient-checks. p-RoPE forward+backward gradient-check.

---

## Phase 5 — Per-Layer Embeddings (week 10)

**Goal:** PLE chain (lookup + project + gate + multiply + project-back + norm + residual) works in forward; gradient handling correct.

**Steps:**
- Per spec doc, PLE machinery is more elaborate than "just lookup":
  - Per-layer embedding table `per_layer_tok_embd` (giant — 4.7 GB at bf16) on CPU, lookup-stream
  - Per-layer trainable matmul weights `per_layer_inp_gate[il]` and `per_layer_proj[il]` (these ARE potential LoRA targets per the architecture, though Phase 7 deliberately excludes them)
  - Per-layer norm `per_layer_post_norm[il]`
  - Global tensors `per_layer_model_proj`, `per_layer_proj_norm` for the precompute step
- Forward (per gemma4-iswa.cpp:202-224): `pe_in = cur; cur = per_layer_inp_gate[il] @ cur; cur = GELU(cur); cur = cur * inp_per_layer[..., il]; cur = per_layer_proj[il] @ cur; cur = RMSNorm(cur, per_layer_post_norm[il]); cur = pe_in + cur`
- Backward: chain through residual → RMSNorm → matmul → elementwise mul → GELU → matmul. Gradient of the PLE table itself (`per_layer_tok_embd`) is sparse: only the rows for the input tokens accumulate. Gradient of `per_layer_inp_gate` and `per_layer_proj` are full matmul gradients.
- LoRA targets: explicitly EXCLUDE PLE tensors from LoRA in Phase 7 (don't fight Google's parameter-efficiency design).
- Memory: 4.7 GB PLE table on CPU; lookup-stream the per-token rows to GPU each forward step.
- Memory verify: peak GPU memory during a full Gemma 4 E2B forward+backward step on west.

**Exit gate:** Forward output (full Gemma 4 E2B chain incl. PLE) matches llama.cpp reference for a fixed sequence within bf16 tolerance. Gradient check still passes for layers downstream of PLE.

---

## Phase 6 — Multimodal mask-off (week 11)

**Goal:** Vision and audio encoders are loaded but not exercised on text-only training; their parameters never receive gradients.

**Steps:**
- Identify vision (~150M, 16 layers, hidden 768) and audio (~300M, 12 layers, hidden 1024) encoder tensors in the GGUF metadata. Conventional names: `audio_tower.*`, `embed_audio.*`, vision encoder follows CLIP-style convention.
- Forge skips loading these encoders into GPU. They stay on disk or in mmap'd CPU RAM.
- Forward path for text-only training does not invoke the encoders.
- LoRA targets explicitly exclude vision/audio tensors.
- If memory permits, optionally support image+audio inputs later — but defer to post-Phase 7.

**Exit gate:** Memory budget shows vision/audio NOT consuming GPU. Training loop runs end-to-end without touching them.

---

## Phase 7 — End-to-end Kingdom Q&A LoRA training (weeks 12+)

**Goal:** Produce a usable, A/B-tested Kingdom Q&A LoRA on Gemma 4 E2B.

**Steps:**
- LoRA targets: Q/K/V/O attention projections on all 35 layers (rank 16 to start, matches Wave 10D config). KV-reusing layers (20-34) only have wq/wo — adjust accordingly.
- Run E0 → E4 ladder analogous to Wave 10D's hardening: 1 example, 5 examples, 50 examples, 500 examples, full RAFT 15991 examples × 3 epochs.
- Verify per-step grad health via `FORGE_DEBUG_GRAD_NORMS=1`: expect `healthy=N/N` (where N = layer-target count after accounting for KV-shared layers having only Q/O targets) every step. Any NaN means a Phase 1-5 bug regressed.
- Loss must descend monotonically over an epoch on a held-out subset.
- Save final LoRA as GGUF: `/var/zhen/models/kingdom-gemma4-e2b-lora.gguf`.
- A/B test: base Gemma 4 E2B vs LoRA on 10 hand-picked Kingdom questions. Score via `forge eval` (or wire up llama.cpp eval mode). LoRA must outperform base on at least 7/10.

**Exit gate:** A/B passes. LoRA file exists. Memory plan verified — no swap thrash, no OOM events.

---

## Phase 8 — Hardening + docs (post-ship, ~1 week)

- Wiki page `wiki/Wave-10F-Forge-Real-Attention-Gemma4.md`
- ADR update: per-architecture decisions made (PLE freeze vs train, LoRA target choices, p-RoPE formula, per-layer-Q-dim handling)
- Update CLAUDE.md with Gemma 4 status
- Memory-budget runbook in notes/
- Mark `WAVE10E-GEMMA4-FORGE.md` as superseded by this plan; retain as historical record

---

## Verification approach (cross-cutting)

**Unit-level:** Every new `*_backward` function gets a numerical gradient check (`#[test]` in same file) using the existing `backward.rs:512` template (eps=1e-3, central diff, relative error < 0.1).

**Integration-level:** Forward output of forge for a fixed token sequence must match `~/tmp/unheaded/llama.cpp/build/bin/llama-cli` output for the same sequence, layer-by-layer where possible, end-to-end at minimum. Tolerance: bf16-precision relative error (~1e-2).

**End-to-end:** The grad-norm diagnostic from `2e0c44f5` (`FORGE_DEBUG_GRAD_NORMS=1`, `FORGE_ACCUM_STEPS=1`) is the production health monitor. After Phase 1, the diagnostic must show `healthy=128/128 zero=0 nan=0` for Mistral-7B. After Phase 7, similar all-healthy reading for Gemma 4 E2B (target count adjusted for KV-shared layers). Any deviation = a bug to track.

**A/B quality:** Phase 7 A/B test against base model is the only proof that the trained LoRA is actually useful. Without that, Phases 0-6 just produce a fancier random number generator.

---

## Risk register

- **R1 — p-RoPE math derivation wrong:** Most novel and least-documented backward. Mitigation: numerical gradient check before integrating; cross-reference llama.cpp source twice. Note `partial_rotary_factor=0.25` only rotates first 128 of 512 dims.
- **R2 — Unified K/V backward gradient summation bug:** Easy to forget to sum across all consumer layers. Mitigation: test with explicit 2-consumer toy before scaling.
- **R3 — Memory budget violated (PLE on GPU = +4.7 GB at bf16):** Mitigation: keep PLE on CPU, lookup-stream per token, profile each phase end memory.
- **R4 — Gemma 4 GGUF only available in formats forge can't dequantize:** Phase 0 step 8 verifies. Initial conversion is bf16; quantize to Q4_0/Q4_K_M/Q8_0 separately.
- **R5 — Updated llama.cpp introduces breaking changes for our trace-collector / runtime:** Don't update llama.cpp on production hosts; build a separate working copy for Phase 0 inspection.
- **R6 — Unsloth/transformers ships a faster path mid-implementation:** Would weaken the case for the multi-month investment. Re-evaluate at each phase boundary. Off-ramp to Path C remains viable until Phase 5.
- **R7 — Numerical instability in real attention without flash-attention tricks:** Mitigation: reuse `forward::softmax`'s max-subtraction stability pattern; bf16 storage with fp32 compute via `hipblasGemmEx`. Logit softcap at 30.0 also bounds the LM-head output.
- **R8 — Wave 10D's flat-loss gate (loss<7) miscalibrated for Gemma vs Mistral:** Don't treat the absolute number as a target; use relative descent + A/B as the truth signal.
- **R9 — Per-layer-variable Q dim trips up shape assumptions:** Forge currently assumes uniform shapes per layer. Mitigation: read tensor shapes from GGUF directly per layer; add shape-validation test.
- **R10 — PLE backward gradient routing wrong (residual + new chain):** Easy to accidentally double-count residual gradient. Mitigation: explicit toy test before scaling.

---

## Out of scope (explicit non-goals)

- Multimodal training (vision / audio) — Phase 7 ships text-only LoRA. Multimodal LoRA = future wave if Kingdom needs image Q&A.
- Distributed training across WEST + EAST — single-host on west only.
- Quantization-aware training — base model stays quantized, LoRA stays fp32.
- Inference optimization — inference path uses llama.cpp + the LoRA, not forge.
- Replacing llama.cpp — llama.cpp remains the production inference engine; forge is the trainer only.
- Training PLE tables (`per_layer_tok_embd`) — frozen.
- Training tied LM head / `tok_embd` — frozen.

---

*Plan written 2026-04-17. Owner: Stevie. Off-ramps documented at every phase boundary. Supersedes WAVE10E-GEMMA4-FORGE.md.*
