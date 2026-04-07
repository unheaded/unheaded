# S10C — PROPER BACKPROPAGATION BATTLE PLAN — 4 Phases, ~35 Steps (SCAFFOLD)

**Date**: 2026-04-07
**Sprint**: S10C — Implement chain-rule backprop through 32 transformer layers
**Prerequisite**: Wave 10B complete (GPU forward + cached weights working, loss starts at 11.02)
**Target**: Loss decreases monotonically; LoRA produces coherent Kingdom-grounded answers
**Estimated Duration**: 12-20 hours across 3-5 sessions
**Schedule**: Begin Friday after weekly limits reset
**Agent Strategy**: Solo (Phase 1-3 sequential), Phase 4 optional
**Commit Cadence**: Every phase exit gate
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

> **NOTE**: This is a SCAFFOLD. Detailed steps to be expanded Friday when limits reset.

---

## LEGEND

[B]      = Bash command
[V]      = Verification step (gate)
[D]      = Debug step
[CODE]   = Code implementation
[TEST]   = Test execution
[MATH]   = Math derivation
[C]      = Commit checkpoint

---

## ROOT CAUSE (one paragraph)

Every LoRA layer (0..31) currently receives the SAME `grad_hidden` from the output projection. There is no chain rule propagation through the residual stream and per-layer matmuls. Layer 0's adapter sees layer 31's gradient — mathematically wrong by 31 layers of compositions. Loss diverges monotonically post-warmup at every learning rate (1e-5, 1e-4, 2e-4, 8e-4). The forward pass is correct (loss starts at ~11.02 ≈ ln(32000)). The fix is purely in `train.rs`'s backward loop and `backward.rs`'s chain-rule helpers.

---

## PHASE 1: Math + Numerical Validation (Steps 1-10) — BLOCKING GATE

**Goal**: Derive backprop equations and verify them via numerical gradient check on a toy model BEFORE touching the real training loop.

- [ ] **Step 1** [MATH]: Derive backward equations for one transformer layer (residual + RMSNorm + Q/K/V/O + FFN)
- [ ] **Step 2** [CODE]: `transformer_layer_backward()` in `backward.rs` — pure CPU reference
- [ ] **Step 3** [TEST]: Numerical gradient check on 2-layer 32-dim toy (rel error < 1e-4) — **THE GATE**
- [ ] **Step 4** [TEST]: Numerical check at realistic dim (4096), 1 layer of real Mistral weights
- [ ] **Step 5** [CODE]: `output_projection_backward()`
- [ ] **Step 6** [CODE]: Embedding layer = no-op (we don't train embeddings)
- [ ] **Step 7** [TEST]: End-to-end gradient check on full pipeline (16 sampled params)
- [ ] **Step 8** [V]: All gradient signs verified
- [ ] **Step 9** [V]: Memory budget verified — peak < 12GB RAM
- [ ] **Step 10** [V][C]: **PHASE 1 EXIT GATE** — numerical check passes; commit "feat(forge): backprop math + numerical validation"

---

## PHASE 2: GPU-Accelerated Backward (Steps 11-20)

**Goal**: Move matmul-heavy backward to GPU sgemm. Reuse cached weights from Wave 10B.

- [ ] **Step 11** [CODE]: `gpu_matmul_grad_a()` — `grad_X = grad_Y × W` via sgemm
- [ ] **Step 12** [CODE]: `gpu_matmul_grad_w()` — `grad_W = grad_Y^T × X` via sgemm_ex
- [ ] **Step 13** [CODE]: `gpu_layer_backward()` for cached layers (28-31)
- [ ] **Step 14** [CODE]: `cpu_stream_layer_backward()` for layers 0-27 (stream from GGUF)
- [ ] **Step 15** [TEST]: GPU backward matches CPU reference (rel error < 1e-3)
- [ ] **Step 16** [CODE]: Wire `gpu_backward_all_layers()` into training loop, replace broken loop
- [ ] **Step 17** [TEST]: Single-step overfit — train one example, loss must decrease
- [ ] **Step 18** [TEST]: 50-step run — loss must decrease monotonically
- [ ] **Step 19** [V]: GPU utilization > 30% (vs current 7%)
- [ ] **Step 20** [V][C]: **PHASE 2 EXIT GATE** — backward verified, 50-step descent confirmed; commit

---

## PHASE 3: Validation Run (Steps 21-28)

**Goal**: Train kingdom-v6 with proper backprop. A/B test against base.

- [ ] **Step 21** [B]: Free GPU resources (kill llama-server, distill, docker)
- [ ] **Step 22** [B]: 500-step validation run with `--lr 1e-4 --epochs 1`
- [ ] **Step 23** [V]: Loss trajectory: step 50 ≈ 11, step 200 ≈ 8-10, step 500 ≈ 5-7
- [ ] **Step 24** [B]: Convert checkpoint-500 to GGUF
- [ ] **Step 25** [B]: A/B test 3 Kingdom questions (Wotan ports, LXD bridge, zhenai-forge language)
- [ ] **Step 26** [V]: Quality gate — coherent English, Kingdom-specific terms, no degenerate hallucination
- [ ] **Step 27** [B]: If quality passes, run full epoch
- [ ] **Step 28** [V][C]: **PHASE 3 EXIT GATE** — loss < 7, coherent LoRA output; commit "progress: kingdom-v6 trained with proper backprop"

---

## PHASE 4: Optimization (Steps 29-35) — OPTIONAL

**Only after Phase 3 passes.** Speed up training for sustainable iteration.

- [ ] **Step 29** [CODE]: Real attention math (`softmax(QK^T/√d)V`)
- [ ] **Step 30** [CODE]: Cache more layers in GPU VRAM if budget allows
- [ ] **Step 31** [CODE]: Eliminate redundant CPU element-wise work
- [ ] **Step 32** [CODE]: Batch gradient accumulation across samples
- [ ] **Step 33** [V]: Steps/sec > 0.5 (vs current 0.2)
- [ ] **Step 34** [TEST]: Full epoch < 8 hours
- [ ] **Step 35** [C]: Commit speed optimizations

---

## CRITICAL FILES

| File | Action |
|------|--------|
| `crates/zhenai-forge/src/backward.rs` | ADD `transformer_layer_backward()` + chain rule helpers |
| `crates/zhenai-forge/src/train.rs` | REPLACE broken backward loop (lines ~720-900) |
| `crates/zhenai-forge/src/hip.rs` | REUSE — `sgemm_ex` already supports transpose |
| `docs/zhenai-forge/BACKPROP-MATH.md` | NEW — derivation document |

## EXISTING INFRASTRUCTURE TO REUSE (do NOT rebuild)

- `BlasHandle::sgemm_ex(transa, transb, ...)` — GPU matmul with transpose
- `GpuTrainer::cached_batched_matmul()` — cached weight matmul
- `GpuTrainer::layer_weights[l]` — pre-uploaded weight buffers
- `backward::matmul_backward_a/b`, `rmsnorm_backward`, `lora_backward`, `cross_entropy_softmax_backward`
- `forward::dequantize_tensor(&model, name)` — stream dequant from mmap'd GGUF
- `gpu_normed` from `forward_loss()` return — already plumbed for reuse

## SCIENTIST WARNINGS

1. **F32 precision floor**: rel error ~1e-3 is the physical limit for 4096-dim matmul. Don't chase tighter.
2. **Phase 1 numerical check is non-negotiable**. Skipping it is what produced kingdom-v5's degenerate output.
3. **Simplified attention is OK for Phase 2**: average Q/K/V/O direction is correct even without softmax. Real attention is Phase 4.
4. **Gradient clipping mandatory**: 32-layer chain rule explodes without clip-norm 1.0.
5. **Reset Adam state** between training runs — stale momentum accelerates divergence.

## RESUMPTION CHECKLIST (Friday)

- [ ] Read this scaffold
- [ ] Read `docs/philosophy/SEE-PAST-NOISE.md` to remember why
- [ ] `git log --oneline -10`
- [ ] `cargo test --release` passes (40+ tests)
- [ ] Stop docker, llama-server, distill — free GPU
- [ ] Expand Phase 1 steps with detail
- [ ] Begin Step 1 (math derivation)

---

*Wave 10C Battle Plan SCAFFOLD — Forged 2026-04-07*
*"The chain rule is not optional. It is the rule that chains the layers."*
*Detail to be expanded Friday after weekly limits reset.*
