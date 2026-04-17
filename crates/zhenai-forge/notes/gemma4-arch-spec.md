# Gemma 4 Architecture Spec — Forge Implementation Reference

**Source of truth:** `~/tmp/unheaded/llama.cpp/src/models/gemma4-iswa.cpp` (322 LOC, post-pull 2026-04-17) + `convert_hf_to_gguf.py:7666-7820` (Gemma4Model + Gemma4VisionAudioModel registrations) + Google's official model card (ai.google.dev, last updated 2026-04-17).

**Purpose:** Forge-implementer-facing distillation. What forge must compute, in what order, with what tensor inputs. No prose — facts.

---

## Architecture string

GGUF metadata key `general.architecture` = `"gemma4"`. Detection in `gguf.rs`: switch on this string. Conversion class is `Gemma4ForConditionalGeneration` → `Gemma4Model` (extends `Gemma3Model`).

## Tokenizer

- Model name in GGUF: `"gemma4"`
- Vocab type: `LlamaHfVocab` (SentencePiece base)
- Vocab size: 262144 (E2B/E4B/26B-A4B); same for 31B per model card
- Visible special tokens (USER_DEFINED type, never stripped): `<|channel>`, `<channel|>`, `<|tool_call>`, `<tool_call|>`, `<|tool_response>`, `<tool_response|>`, `<|"|>`
- Standard chat-template tokens: per Unsloth doc, uses `<|turn>user`, `<|turn>model`, etc. + `<|think|>`/`<|channel>thought`/`<channel|>` for reasoning mode

## Per-size hparams (from Google model card)

| Size | Layers | Sliding window | Context | Vocab | Modalities | head_dim |
|------|--------|----------------|---------|-------|-----------|----------|
| E2B  | 35 | 512 | 128K | 262144 | T+I+A | (n_embd_per_layer-derived) |
| E4B  | 42 | 512 | 128K | 262144 | T+I+A | same pattern |
| 26B-A4B (MoE) | 30 | 1024 | 256K | 262144 | T+I | 8 active / 128 total / 1 shared experts |
| 31B Dense | 60 | 1024 | 256K | 262144 | T+I | — |

E2B effective params: 2.3B (5.1B with embeddings). The "E" = effective; PLE tables inflate the param count without proportionally inflating compute.

**Per-layer hparams** (gemma4-iswa.cpp:41-49) — all variable per layer index `il`:
- `n_embd_head_k(il)` = `n_embd_head_v(il)` (asserted equal)
- `n_head(il)`
- `n_head_kv(il)`
- `n_rot(il)` (RoPE rotation dim per layer)
- `get_rope_freq_base(cparams, il)` — per-layer RoPE base (sliding ≠ global per Google card: 10K vs 1M)
- `get_rope_freq_scale(cparams, il)` — per-layer RoPE scale
- `is_swa(il)` → bool — sliding-window vs full global
- `has_kv(il)` → bool — KV-producing vs KV-reusing layer (the unified-K/V mechanism)

## Forward graph (E2B, dense path)

Per gemma4-iswa.cpp:10-260. Sequence:

1. **Input embedding** (line 17-21):
   - `inpL = tok_embd[token_ids]` (or raw image embeddings if multimodal)
   - Scale: `inpL *= sqrt(n_embd)` if token input, `*= 1.0` if raw embeddings

2. **Per-Layer-Embedding precompute** (line 31-38, only if `model.per_layer_tok_embd` exists):
   - `inp_per_layer = build_inp_per_layer()` — looks up `per_layer_tok_embd[token_id]` reshaped to `[n_embd_per_layer, n_layer, n_tokens]`, scales by `sqrt(n_embd_per_layer)`
   - `inp_per_layer = project_per_layer_inputs(inpL, inp_per_layer)`:
     - `per_layer_proj = per_layer_model_proj @ inpL` (matmul) scaled by `1/sqrt(n_embd)`, reshaped to `[n_embd_per_layer, n_layer, n_tokens]`
     - `per_layer_proj = RMSNorm(per_layer_proj, per_layer_proj_norm, eps=f_norm_rms_eps)`
     - `inp_per_layer = (per_layer_proj + inp_per_layer) * (1/sqrt(2))`
     - Permute to `[n_embd_per_layer, n_tokens, n_layer]`
   - This becomes a per-layer constant supplied to step 7 below

3. **For each layer `il` in [0, n_layer):**

   3a. **Attention norm:** `cur = RMSNorm(inpL, attn_norm[il], eps=f_norm_rms_eps)`

   3b. **RoPE freq factors** (line 55-59): `freq_factors = (is_swa(il)) ? null : rope_freqs[il]` — proportional RoPE (`p-RoPE`) only on full-attention layers via the `rope_freqs` tensor

   3c. **Q projection always** (line 61-76):
       - `Qcur = wq[il] @ cur` (with optional LoRA scale `wq_s[il]`)
       - Reshape to `[n_embd_head, n_head, n_tokens]`
       - `Qcur = RMSNorm(Qcur, attn_q_norm[il], eps=f_norm_rms_eps)` — **per-head Q-norm AFTER projection**
       - `Qcur = RoPE(Qcur, inp_pos, freq_factors, n_rot, freq_base, freq_scale, ...)`

   3d. **If `has_kv(il)` (this layer produces its own K/V)** (line 79-104):
       - `Kcur = wk[il] @ cur` (with optional `wk_s[il]`)
       - `Vcur = wv[il] @ cur` if `wv[il]` exists, else `Vcur = Kcur`
       - Both reshape to `[n_embd_head, n_head_kv, n_tokens]`
       - `Kcur = RMSNorm(Kcur, attn_k_norm[il], eps=f_norm_rms_eps)` — per-head K-norm
       - `Vcur = ggml_rms_norm(Vcur, eps=f_norm_rms_eps)` — V-norm has NO weight tensor, just eps
       - `Kcur = RoPE(Kcur, inp_pos, freq_factors, n_rot, freq_base, freq_scale, ...)`
       - `cur = attention(Qcur, Kcur, Vcur, wo[il], wo_s[il], scale=f_attention_scale, mask=is_swa(il) ? sliding_window : full_causal)`
       - K, V cached for downstream KV-reusing layers

   3e. **Else (KV-reusing layer)** (line 105-110):
       - `cur = attention(Qcur, K=null, V=null, wo[il], wo_s[il], scale=f_attention_scale)` — uses earlier-cached K/V via the `inp_attn` graph input

   3f. **Post-attention norm + residual:** `attn_out = RMSNorm(cur, attn_post_norm[il]) + inpL`

   3g. **FFN** (line 126-193):
       - **Dense path** (E2B/E4B/31B, when `ffn_gate_inp[il] == null`):
         - `cur = RMSNorm(attn_out, ffn_norm[il])`
         - `cur = FFN(cur, up=ffn_up[il], gate=ffn_gate[il], down=ffn_down[il], activation=GELU, mode=PARALLEL)` — gate and up multiplied parallel-style (LLM_FFN_PAR)
       - **MoE path** (26B-A4B, when `ffn_gate_inp[il] != null`):
         - Shared expert: `cur_mlp = RMSNorm(attn_out, ffn_norm[il])` → FFN (GELU, PAR) → `RMSNorm(ffn_post_norm_1[il])`
         - Sparse experts:
           - `cur_moe = RMSNorm(attn_out, ffn_pre_norm_2[il])`
           - Router logits: `tmp = RMSNorm(attn_out, eps); tmp = tmp * (1/sqrt(n_embd)) * ffn_gate_inp_s[il]; logits = ffn_gate_inp[il] @ tmp`
           - `cur_moe = MoE(cur_moe, down_exps=ffn_down_exps[il], gate_up_exps=ffn_gate_up_exps[il], gating=SOFTMAX, n_active=8, n_total=128)`
           - `cur_moe = RMSNorm(cur_moe, ffn_post_norm_2[il])`
         - `cur = cur_mlp + cur_moe`

   3h. **Post-FFN norm + residual:** `cur = RMSNorm(cur, ffn_post_norm[il]) + attn_out`

   3i. **Per-Layer Embedding contribution** (line 202-224, only if `inp_per_layer` exists from step 2):
       - `pe_in = cur` (saved for residual)
       - `cur = per_layer_inp_gate[il] @ cur` → projects to `[n_embd_per_layer, n_tokens]`
       - `cur = GELU(cur)`
       - `inp_this_layer = inp_per_layer[..., il]` (slice the per-layer dim)
       - `cur = cur * inp_this_layer` (elementwise mul)
       - `cur = per_layer_proj[il] @ cur` → projects back to `[n_embd, n_tokens]`
       - `cur = RMSNorm(cur, per_layer_post_norm[il])`
       - `cur = pe_in + cur` (residual)

   3j. **Optional output scale** (line 227-230, if `out_scale[il]` exists):
       - `cur = cur * out_scale[il]` (per-layer scalar multiplier)

   3k. `inpL = cur` for next iteration

4. **Final norm + LM head** (line 240-258):
   - `cur = RMSNorm(inpL, output_norm, eps=f_norm_rms_eps)`
   - `cur = output @ cur` (LM head, optional LoRA scale)
   - **Optional logit softcapping** (if `f_final_logit_softcapping > 0`):
     - `cur = tanh(cur / softcap) * softcap`
   - These are the logits

## Global tensors (shared across all layers)

- `tok_embd` — primary token embedding `[n_embd, vocab_size]`
- `output_norm` — final RMSNorm weights `[n_embd]`
- `output` — LM head `[vocab_size, n_embd]` (often tied to tok_embd)
- `per_layer_tok_embd` — `[n_embd_per_layer * n_layer, vocab_size]` reshaped — the giant PLE lookup table
- `per_layer_model_proj` — `[n_embd_per_layer, n_embd]` — projects hidden → per_layer space
- `per_layer_proj_norm` — `[n_embd_per_layer]` — RMSNorm weights for the projection

## Per-layer tensors (forge must load all of these per `blk.{il}.*`)

**Always present:**
- `attn_norm`, `attn_post_norm`, `attn_q_norm`, `attn_k_norm`
- `wq` (always), `wo` (always)
- `wk`, `wv` (when `has_kv(il)` is true; absent for KV-reusing layers)
- LoRA scale companions: `wq_s`, `wk_s`, `wv_s`, `wo_s` (optional)
- `ffn_norm`, `ffn_post_norm`
- `ffn_up`, `ffn_gate`, `ffn_down` + optional `_s` LoRA scales

**Optional:**
- `rope_freqs` — only on full-attention layers (the p-RoPE freq scaling)
- `out_scale` — per-layer scalar output multiplier
- `per_layer_inp_gate`, `per_layer_proj`, `per_layer_post_norm` — only if PLE active

**MoE-only (26B-A4B):**
- `ffn_gate_inp`, `ffn_gate_inp_s` — router weights
- `ffn_pre_norm_2`, `ffn_post_norm_1`, `ffn_post_norm_2`
- `ffn_down_exps`, `ffn_gate_up_exps` — expert weights

**Multimodal-only (E2B, E4B, 26B-A4B for vision; E2B, E4B for audio):**
- `audio_tower.*`, `embed_audio.*` — audio encoder
- Vision encoder tensors via standard CLIP-style naming

## Hparams forge must read from GGUF metadata

Per `convert_hf_to_gguf.py` and llama-arch entries (need to grep specific names but pattern follows `gemma4.*`):
- `gemma4.block_count` (n_layer)
- `gemma4.embedding_length` (n_embd)
- `gemma4.feed_forward_length` (n_ff)
- `gemma4.attention.head_count` (per-layer if Gemma 3 pattern; varies per layer)
- `gemma4.attention.head_count_kv`
- `gemma4.attention.key_length`, `value_length` (per-head dims)
- `gemma4.attention.layer_norm_rms_epsilon`
- `gemma4.attention.sliding_window`
- `gemma4.attention.scale` (the `f_attention_scale`)
- `gemma4.rope.freq_base`, `freq_scale`
- `gemma4.embedding_length_per_layer_input` (PLE n_embd_per_layer)
- `gemma4.expert_count`, `expert_used_count` (MoE only)
- Per-layer attention-type pattern (sliding vs full) — likely encoded as a list/array hparam

**Forge must implement metadata parsing for all the above. Names likely follow gemma3/gemma3n with `gemma4` prefix; verify against the GGUF once downloaded.**

## Multimodal handling (for forge text-only training, Phase 6)

Vision encoder: ~150M params (E2B/E4B), ~550M (26B/31B). Audio encoder: ~300M (E2B/E4B only).

For text-only training:
- Skip loading `audio_tower.*` and vision-encoder tensors entirely (don't `dequantize_tensor` them)
- The PLE branch (`build_inp_per_layer`) DOES still fire on text input — it's not multimodal-only despite handling multimodal input. It's the parameter-efficiency mechanism. **PLE is on the text path too.**
- Skip image-token embedding paths in tokenizer

## Backward implications (for forge backward implementation)

Each forward op needs a corresponding backward in `backward.rs`. New ones beyond what forge has today:

1. **Standard transformer backward (Phase 1):**
   - `softmax_backward` (Jacobian-vector product, per attention row)
   - `scaled_dot_product_backward` (chain through Q@K^T, scale, mask, softmax, @V)
   - `rope_backward` (inverse rotation — RoPE is its own inverse modulo conjugation)
   - `gqa_collapse_backward` (sum gradient across the repeated query heads back to KV head dim)
   - `gelu_backward` (we have silu_backward; need GELU variant — Gemma 4 FFN uses GELU not SiLU)
   - `parallel_ffn_backward` (gate * up multiplied; backward chains through both branches)

2. **Gemma 4 specific (Phase 3+):**
   - `sliding_window_attention_backward` (mask zeroes outside window — forward softmax already zeroes; backward inherits the zeros via softmax_backward)
   - `prope_backward` (proportional RoPE — uses `rope_freqs` per-layer factor; backward applies inverse rotation with the same factor)
   - `unified_kv_backward` (gradient on producer K/V layer = sum of all consumer-layer K/V gradients) — explicit accumulation step at the end of the per-layer backward loop
   - `q_norm_backward`, `k_norm_backward` — already covered by reusable `rmsnorm_backward`
   - `ple_backward`:
     - Backward through `per_layer_inp_gate` matmul
     - Backward through GELU
     - Backward through elementwise multiply with `inp_this_layer`
     - Backward through `per_layer_proj` matmul
     - Backward through `per_layer_post_norm` (rmsnorm_backward)
     - Backward through residual (gradient flows to BOTH `pe_in` and the per-layer-embedding chain)
     - The PLE table itself (`per_layer_tok_embd`) is sparse-gradient: only the rows for the input tokens accumulate
   - `out_scale_backward` (trivial: `grad_in = grad_out * out_scale`, `grad_out_scale = sum(grad_out * cur)`)
   - `logit_softcap_backward` (chain through `* softcap`, then `tanh_backward`, then `/ softcap`)

3. **MoE-specific (Phase 5+, only if 26B-A4B becomes a target — out of scope for E2B):**
   - Router gradient (sparse — only active experts get gradient)
   - Per-expert weight gradient
   - Defer until E2B works.

## LoRA target inventory (for forge LoRA placement)

The model has built-in LoRA scale tensors (`wq_s`, `wk_s`, `wv_s`, `wo_s`, `ffn_*_s`, `per_layer_*_s`, `output_s`) — meaning Gemma 4 was *designed* with LoRA-friendly weight layouts.

Recommended LoRA targets for Kingdom Q&A fine-tuning (Phase 7):
- All `wq`, `wk`, `wv`, `wo` per layer (when `has_kv(il)` is true; KV-reusing layers only have `wq`, `wo`)
- Optional FFN: `ffn_up`, `ffn_gate`, `ffn_down` (matches Mistral LoRA target choices)
- **Skip:** `per_layer_inp_gate`, `per_layer_proj` (PLE chain — don't fight Google's parameter-efficiency design)
- **Skip:** all encoder tensors (vision, audio)
- **Skip:** `per_layer_tok_embd`, `tok_embd`, `output` (embeddings + LM head — frozen)

## Verification reference

`~/tmp/unheaded/llama.cpp/build/bin/llama-cli -m <gemma-4-E2B.gguf> -p "<test prompt>" --logits-all -n 1 --temp 0`

This emits per-token logits forge must match within bf16 precision (~1e-2 relative error) once Phase 5 is complete.

## E2B verified hparams (from `/home/govan/tmp/gemma-4-E2B-it/config.json`, 2026-04-17)

Source-of-truth canonical values for forge implementation. HF safetensors clone — GGUF still TBD.

**Text model (the part forge trains):**

| Field | Value | Forge implication |
|-------|-------|---|
| `num_hidden_layers` | 35 | Loop iterates 0..35 |
| `hidden_size` | 1536 | n_embd |
| `intermediate_size` | 6144 | FFN inner dim |
| `num_attention_heads` | 8 | Q heads |
| `num_key_value_heads` | 1 | **Single KV head — extreme 8:1 GQA** |
| `head_dim` (sliding layers) | 256 | sliding Q out = 8×256 = 2048 |
| `global_head_dim` (full layers) | **512** | **Per-layer-type-variable head dim — full Q out = 8×512 = 4096** |
| `num_kv_shared_layers` | 20 | Layers 20-34 reuse K/V from earlier; only 20 KV-producing layers |
| `sliding_window` | 512 | Local attention window |
| `max_position_embeddings` | 131072 | 128K context |
| `vocab_size` | 262144 | |
| `hidden_size_per_layer_input` | 256 | PLE n_embd_per_layer |
| `vocab_size_per_layer_input` | 262144 | PLE table = 35 × 262144 × 256 × 2B = **~4.7 GB at bf16** |
| `rms_norm_eps` | 1e-6 | All RMSNorms |
| `final_logit_softcapping` | **30.0** | tanh-bounded logits enabled (Phase 1+ must implement) |
| `hidden_activation` | `"gelu_pytorch_tanh"` | **Tanh-approx GELU** — NOT plain GELU. Forge needs `gelu_tanh_approx` impl: `0.5*x*(1+tanh(sqrt(2/pi)*(x + 0.044715*x^3)))` |
| `tie_word_embeddings` | true | LM head = tok_embd transposed (no separate output weights) |
| `enable_moe_block` | false | E2B is dense — skip MoE branch entirely |
| `attention_bias` | false | No bias on Q/K/V/O projections |
| `attention_dropout` | 0.0 | No dropout |
| `use_double_wide_mlp` | true | Flag — verify FFN layout matches assumptions; may affect ffn_gate/ffn_up dim |
| `attention_k_eq_v` | false | V is separate from K (both projections present) |
| `pad_token_id` | 0 | |
| `bos_token_id` | 2 | |
| `eos_token_id` (text) | 1 | (also 106 in chat config) |

**RoPE config (per attention type):**

| Type | rope_theta | rope_type | partial_rotary_factor |
|------|-----------|-----------|----------------------|
| sliding_attention | 10000 | default | 1.0 (full rotation) |
| full_attention | **1000000** | **proportional** | **0.25** (only 25% of 512 = 128 dims rotated) |

The "proportional RoPE" mechanism (Google model card) = standard RoPE with partial rotation factor 0.25 on 512-dim heads. Combined with the high theta (1M) for the long-context global layers.

**Per-layer attention pattern (35 layers):**

```
Indices 0-3:    sliding
Index 4:        full
Indices 5-8:    sliding
Index 9:        full
Indices 10-13:  sliding
Index 14:       full
Indices 15-18:  sliding
Index 19:       full   ← last KV-producing full layer (per num_kv_shared_layers=20)
Indices 20-23:  sliding   ← KV-reusing
Index 24:       full      ← KV-reusing
Indices 25-28:  sliding   ← KV-reusing
Index 29:       full      ← KV-reusing
Indices 30-33:  sliding   ← KV-reusing
Index 34:       full      ← KV-reusing, also the final layer (model card: "final layer always global" ✓)
```

**Pattern:** every 5th layer is full-attention. 7 full layers, 28 sliding layers. KV-producing in layers 0-19; KV-reusing in 20-34.

**Open question (TBD):** Exact source-layer mapping for KV-reusing layers. Hypothesis: a KV-reusing layer reuses K/V from the most recent KV-producing layer of the same attention type. To confirm: read llama.cpp's iswa cache logic OR inspect `inp_attn` graph in gemma4-iswa.cpp build_attn calls. Stops being a blocker until Phase 4 (unified-KV implementation).

**Audio config** (skip for text training, Phase 6):
- 12 layers, 8 heads, hidden 1024, ~300M params (rough estimate matches model card)
- model_type `gemma4_audio`, intermediate via `output_proj_dims=1536` projector to text hidden

**Vision config** (skip for text training, Phase 6):
- 16 layers, 12 heads, hidden 768, intermediate 3072, ~150M params
- patch_size 16, position_embedding_size 10240
- model_type `gemma4_vision`

**Multimodal token IDs:**
- `<|image_token|>` = 258880, `<|boi|>` = 255999, `<|eoi|>` = 258882
- `<|audio_token|>` = 258881, `<|boa|>` = 256000, `<|eoa|>` = 258883
- `<|video_token|>` = 258884
- `vision_soft_tokens_per_image` = 280

## Forge architectural impact summary

Beyond what the plan already captured, the verified hparams add:
- **Per-layer-type-variable Q output dim** (256 vs 512). The wq tensor literally has different output dims per layer. Plan's Phase 1 (vanilla uniform-per-layer) holds; Phase 3 must dispatch per layer.
- **GELU is tanh-approximation variant**, not plain GELU. Forge `forward.rs` needs `gelu_tanh_approx` and `gelu_tanh_approx_backward`.
- **Final logit softcapping at 30.0** must be in the forward path AND have a backward (chain through tanh derivative + scale).
- **Tied word embeddings** simplifies LM head — but means the LM-head gradient flows back into the tok_embd table. If LoRA targets exclude both, no extra work; if either is a target, the tying creates a shared-gradient situation.
- **PLE memory at 4.7 GB bf16** matches plan risk R3. Mitigation: keep PLE table CPU-resident, mmap and lookup-stream rather than upload to GPU. `model.safetensors` is 9.6 GB total at bf16; PLE alone is half of that.

## Conversion command (HF → GGUF)

After llama.cpp pull (already done, commit `30dce2c`):

```bash
cd ~/tmp/unheaded/llama.cpp && python3 convert_hf_to_gguf.py \
  /home/govan/tmp/gemma-4-E2B-it \
  --outfile /var/zhen/models/gemma-4-E2B-it.gguf \
  --outtype bf16
```

Expected output: `/var/zhen/models/gemma-4-E2B-it.gguf` ≈ 5 GB at bf16. Source `model.safetensors` is 9.6 GB; bf16 GGUF deduplicates and quantizes embeddings tighter.

If smaller VRAM footprint needed for forge memory budget:
```bash
~/tmp/unheaded/llama.cpp/build/bin/llama-quantize /var/zhen/models/gemma-4-E2B-it.gguf /var/zhen/models/gemma-4-E2B-it-Q4_0.gguf Q4_0
```
Q4_0 brings to ~1.6 GB per the typical compression ratio.

## Still TBD (after GGUF lands)

- Exact GGUF metadata key names (read with `forge info` or `gguf-py`'s reader once we have the file)
- Tokenizer chat-template field in GGUF metadata
- Quantization formats actually present (post-conversion = bf16 only; quantize separately if needed)
- Confirmation that llama.cpp's `Gemma4VisionAudioModel` fires for `Gemma4ForConditionalGeneration` arch (it might during conversion — that's the multimodal projector path)

These come at Phase 0 final step (validate llama-cli inference on the converted GGUF).

## Key insight from spec

Gemma 4 has **trainable matmul weights inside the PLE chain** (`per_layer_inp_gate`, `per_layer_proj`) and **per-layer Q/K/V dimensions and RoPE configs** — the architecture is more parametric than a vanilla transformer. Forge's current "treat all layers identically" assumption breaks. The training loop must read per-layer hparams and dispatch per-layer forward/backward variants.

Phase 1 of the plan (vanilla GQA on Mistral) uses per-layer-uniform hparams as a simpler test bed; Phase 3+ (Gemma 4 features) introduces the per-layer parametrization.

---

*Spec written 2026-04-17 from llama.cpp@30dce2c source. Refresh after every llama.cpp pull during the project.*
