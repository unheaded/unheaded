# WAVE12 Session Log — GPU-Resident Forge @ seq=384

**Sprint:** WAVE12 — close 2× gap from 10.5 s/step warm → ≤5 s/step target, then ship 500-step Kingdom RAFT LoRA with eval descent.
**Plan:** `crates/zhenai-forge/notes/wave12-gpu-resident-battle-plan.md` (9 phases, 195 steps).
**Forged:** 2026-04-22.
**Agent:** Coordinator (solo).
**Commit cadence:** every 5 steps; phase-exit commit mandatory; `--no-gpg-sign` per AFK policy.

---

## Per-phase results

| Phase | Name | Gate | Measured | Status |
|------:|------|-----|---------|-------:|
| 0 | Preflight + baseline | warm step 3 ∈ [9.5, 12] s | **10.2 s** @ loss 21.21→18.63→16.47 | ✅ |
| 1 | Shared mask cache | warm ≤ 10.5 s (no regression) | **10.65 s median** (4 warm samples) | ⚠ noise-floor |
| 2 | Backward GPU ops | warm ≤ 8.5 s | **9.9 s** (4 warm samples) | ⚠ below expectation (−0.7s vs −2-4s) |
| 3 | Checkpoint + decision | median warm | **10.5 s** (8 warm samples) | ⚠ at baseline |
| 4 | Matmul GPU-in/out | matmul tests green, no regression | cosine 1.0 exact, 0 abs err | ✅ |
| 5 | GPU-resident forward | warm ≤ 5.5 s | — | pending |
| 6 | Full regression | Learning Gate 4/5 pass, warm ≤ 5.5 s | — | pending |
| 7 | 500-step RAFT | eval loss@500 < eval loss@0, final train loss < 8 | — | pending |
| 8 | ADR-050 + handoff | DoD all true | — | pending |

---

## Phase 0 — Preflight + baseline re-confirm (2026-04-22)

**Duration:** ~15 min wall (plan budgeted 30m).

**Ground state verified:**
- HEAD `93c137fe` (WAVE12 plan forged), descends from `5d32f9d5` (WAVE11 Phase 8c shipped). Tree clean.
- GGUF `/var/zhen/models/gemma-4-E2B-it.gguf` present (8.7 GB on disk; plan notes 9.3 GB — likely filesystem rounding, file is the one W11 was trained against).
- `MemAvailable`: 10 GiB.
- No zombie forge processes. GPU[0] VRAM 203 MB (<<1 GB).

**Corpus recovery (Appendix A1 deviation):**
- `/tmp/24h-kingdom-{train,eval}.jsonl` were absent (not reboot, but `/tmp` cleaned).
- Appendix A1's suggested CLI flags (`--input`, `--output`, `--model`, `--max-tokens`) are not implemented in `scripts/tokenize-kingdom-for-gemma4.py`; the script reads stdin → stdout and uses a hardcoded tokenizer path. Ran via the documented venv pattern instead:
  ```bash
  /home/govan/tmp/gemma4-venv/bin/python scripts/tokenize-kingdom-for-gemma4.py \
    < raft/training/train.jsonl > /tmp/24h-kingdom-train.jsonl
  ```
- Result: train `in=3568 out=3568 err=0 short=0 trunc=3561`; eval `in=397 out=397 err=0 short=0 trunc=397`. Line counts match plan expectations (3568 + 397).

**Regression gates:**
- `cargo build --release`: 0 errors, 101 pre-existing warnings.
- `cargo test -- hip_kernels::`: 21 passed / 0 failed / 108 filtered in 4.80 s.
- `test_gemma4_gpu_forward_matches_cpu`: cosine **0.998549**, argmax CPU=5646, GPU=5646 → top-1 match, top-5 overlap 4/5. ok.
- `test_gemma4_gpu_train_step_loss_descent`: 3-step toy loss 21.10 → 13.29 → 6.48 monotonic, avg 2.17 s/step (toy, not seq=384). ok.

**Baseline smoke (seq=384, 3 steps, lr=1e-3, answer_start=1):**
```
[step 1/3] loss=21.2099 step_time=42.7s  (cold)
[step 2/3] loss=18.6311 step_time=10.3s  (JIT warm)
[step 3/3] loss=16.4732 step_time=10.2s  (steady)
```
Warm step 3 = **10.2 s** → inside the plan's 9.5-12 s baseline window. **Baseline gate ✅**.

**Notes carried forward:**
- Cold step (42.7 s) is noticeably faster than plan's 70-80 s expectation; likely filesystem cache was already warm from prior sessions today. Not a regression signal.
- Budget to close: 10.2 → 5.0 s = 5.2 s/step to find across Phases 1-5 (mask cache ~0.3, backward GPU ~2-4, GPU-resident forward + eliminating round-trips ~1-3).

---

## Phase 1 — Shared mask cache (2026-04-22)

**Duration:** ~30 min wall (plan budgeted 45m).

**Implementation:**
- Added `MaskCache` struct in `src/gemma4_gpu.rs` (just above `attention_forward_gpu_kernels`).
  - `causal: Option<GpuBuffer>` — exactly one cache slot (Gemma-4 has one causal variant).
  - `sliding: Vec<(usize, GpuBuffer)>` — keyed by window size.
  - `get_or_build(mask, n_heads, seq_len) -> Result<&GpuBuffer, String>`.
  - `build(...)` replicates the original per-(i,j) `mask.allows()` loop, one GpuBuffer alloc + upload per unique mask.
- Changed `attention_forward_gpu_kernels` to take `&mut MaskCache`. Removed local `mask_cpu` build + `mask_buf` alloc/upload. Reads cached buffer via `mask_cache.get_or_build(...)`.
- `forward_gemma4_gpu` instantiates `let mut mask_cache = MaskCache::new();` before the 35-layer loop and passes `&mut mask_cache` into the attention helper call.

**Regression (bit-identical outputs):**
- `test_gemma4_gpu_forward_matches_cpu`: cosine 0.998549, argmax 5646, top-5 identical to Phase 0. ok.
- `test_gemma4_gpu_train_step_loss_descent`: loss 21.1045 → 13.2858 → 6.4775 (bit-identical to Phase 0). ok.

**Kingdom smoke (seq=384, steps=5, warm samples 2-5):**
```
[step 1/5] 50.4s   (cold)
[step 2/5] 10.8s
[step 3/5] 10.6s
[step 4/5] 10.7s
[step 5/5] 10.6s
```
Warm median: **10.65 s**. Phase 0 baseline: 10.2 s (n=1). Delta = +0.45 s but
Phase 0 was single-sample and Phase 1 shows a 4-sample band of 10.6-10.8 →
Phase 0's 10.2 is the lower tail of noise. The mask cache's expected 0.2-0.5 s
win (per plan Step 35 — "almost certainly NO ≤5 s; proceed to Phase 2")
matches the variance floor of this hardware on seq=384.

**Decision:** Kept the change. Correctness bit-identical, 34/35 mask uploads
eliminated per step (≈160 MB/step of PCIe traffic), and Phase 2's expected
2-4 s/step win will dominate any residual noise. No [D] branch needed —
cache hit rate is provably 34/35 by construction (1 causal variant + 1
sliding window variant in Gemma-4).

---

## Phase 2 — Backward rmsnorm/gelu/rope on GPU (2026-04-22)

**Duration:** ~1 h wall (plan budgeted 2h).

**Implementation:**
- Added 5 batch backward helpers in `src/gemma4_gpu.rs`, mirroring the Phase 8c forward helpers: `rmsnorm_batch_bwd_on_gpu`, `per_head_rmsnorm_batch_bwd_on_gpu`, `gelu_batch_bwd_on_gpu`, `gelu_mul_batch_bwd_on_gpu`, `rope_batch_bwd_on_gpu`.
- Swapped 11 CPU backward call sites in `src/gemma4.rs::backward_gemma4_with_lora` to use the GPU helpers, guarded by `if gpu.is_some()`:
  - Site 6: post_attention_norm rmsnorm_bwd
  - Site 9: post_ffw_norm rmsnorm_bwd
  - Site 10: ffn_norm rmsnorm_bwd (accumulating add)
  - Site 11: fused gelu_mul_bwd (FFN)
  - Site 12: partial-RoPE bwd × 2 (Q, K)
  - Site 13: per-head rmsnorm_bwd × 2 (Q, K)
  - Site 15: attn_norm rmsnorm_bwd (accumulating add)
  - Site 16: PLE post_norm rmsnorm_bwd
  - Site 17: PLE unfused gelu_bwd
  - Site 18: output_norm rmsnorm_bwd
- Site 14 (v_norm_backward, weightless) kept on CPU per plan — tensor is tiny (seq*n_head_kv=1*head_dim) and synthesizing a ones-weight GpuBuffer would dwarf the CPU cost.
- Plan's Site 19 (per_layer_proj_norm) is used only in PLE forward, not backward — no swap needed.

**Phase 8b postmortem quirk recurred in rope_bwd:**
KV-reuse consumer layers have `grad_k_rot.len() > seq * n_head_kv * head_dim` because `attention_backward_gpu_kernels` allocates `grad_k = vec![0.0; k_rot.len()]` from actual cache size (e.g., swa consumer inheriting 512-stride K from full producer → 3072-element grad_k_rot when derived shape says 1536). CPU `rope_backward_partial` silently tolerated this via `grad_rotated.to_vec()` passthrough of the tail; my first GPU helper asserted the derived shape and panicked. Fix: allocate buffers from actual `grad_out.len()`, pre-seed `grad_in_buf` with `grad_out` so the passthrough tail matches CPU semantics, kernel overwrites only the first `head_dim` stride per head. Documented at the helper body.

**Regression:**
- `test_gemma4_gpu_forward_matches_cpu`: unchanged (forward untouched) — cosine 0.9985, argmax 5646, ok.
- `test_gemma4_gpu_train_step_loss_descent`: loss **21.1045 → 13.4454 → 7.1765** (monotonic, within bf16 variance vs CPU 21.10 → 13.29 → 6.48). ok.

**Kingdom smoke (seq=384, steps=5, warm samples 2-5):**
```
[step 1/5] 62.4s  (cold, JIT-warmed 11 new bwd kernels)
[step 2/5] 10.0s
[step 3/5]  9.9s
[step 4/5]  9.9s
[step 5/5]  9.9s
```
Warm median: **9.9 s**. Plan gate was ≤8.5 s. Saved ~0.75 s/step vs Phase 1 baseline (10.65 → 9.9) — much less than the plan's 2-4 s expectation.

**Diagnosis (honest):**
The per-call PCIe round-trip overhead now dominates the backward path, not CPU compute. Each of the ~175 backward kernel calls per step (35 layers × ~5 bwd ops each) does alloc + upload + download ≈ 2.4-9.4 MB round-trip. Net PCIe traffic for backward swaps alone ≈ 880 MB/step. The CPU backward we replaced was ~9 ms/layer × 35 ≈ 0.3 s of compute — that's the actual savings ceiling for Phase 2 before Phase 5 eliminates the round-trips.

**Decision:** The remaining 4.9 s/step gap (9.9 → 5.0 target) genuinely requires Phase 4+5 (GPU-resident activations). The Phase 2 swaps are still *necessary* infrastructure for Phase 5 (which needs gpu-in/gpu-out bwd kernels), just not the performance unlock by themselves. Moving forward with the plan.

---

## Commit ledger

| Step | Hash | Phase | Summary |
|-----:|------|------:|---------|
| 20 | `9338d9e5` | 0 | `docs(forge): [PLAN W12] Phase 0 — preflight, baseline confirmed @ ~10.2s/step warm` |
| 34 | `9df2cba5` | 1 | `feat(forge): [PLAN W12] Phase 1 — MaskCache eliminates 34/35 per-layer mask uploads` |
| 74 | `1ea58922` | 2 | `feat(forge): [PLAN W12] Phase 2 — 11 backward sites on GPU (rope passthrough fix)` |
| 88 | *pending* | 3 | TBD — Phase 3 checkpoint recorded, awaiting route decision |

---

## Phase 3 — Checkpoint + decision (2026-04-22)

**10-step steady-state smoke (seq=384, lr=1e-3):**
```
step  1: 65.7s (cold)
step  2: 10.7s
step  3: 10.5s
step  4: 10.6s
step  5: 10.5s
step  6: 10.6s
step  7: 10.5s
step  8: 10.5s
step  9: 10.5s
step 10: 10.5s
```
Warm median (3-10): **10.5 s**. Warm range: 10.5-10.7 s.

**Loss descent:** 21.21 → 18.61 → 17.07 → 14.33 → 12.94 → 11.72 → 11.44 → 11.16 → 10.55 → **9.79**. Kingdom corpus learning is working — 10 steps cut loss >50% on fixed mini-batch.

**Honest delta from Phase 0 baseline (10.2 s, n=1):** essentially zero. Phase 1's mask cache and Phase 2's 11 backward kernel swaps both produced correct-but-wall-neutral outputs. The net gain (if any) is within the 0.3 s jitter band.

**Cost-model mismatch with plan:**
- Plan predicted Phase 2 saves 2-4 s/step. Actual: 0.0-0.75 s (jitter).
- Plan predicted Phase 5 (GPU-resident forward chain) saves ~3 s on top. If the cost model is systematically wrong by 3-4×, Phase 5's expected win of ~5 s might be ~1-1.5 s in practice.
- Where IS the 10.5 s going? Without a profiler pass (added `std::time::Instant` around major chunks → plan Step 131's [D] branch), we're guessing. The ~175 backward kernel launches per step at ~10-50 µs each = 2-9 ms total, not seconds. The bulk is probably still matmuls (30+ per layer × 35 layers), or ROCm stream synchronization.

**Phase 3 DECIDE (Step 85/86): routes forward.**

| # | Route | Eta | Risk | DoD status |
|--:|-------|----:|-----|-----------|
| A | **Short-circuit: skip Phases 4-6, run 500-step RAFT now.** 10.5s/step × 500 ≈ 88 min + cold + eval ≈ 2h to shipped LoRA. | ~2h | low | misses warm ≤5.5s; hits other 14 DoD items |
| B | **Continue Phases 4-5-6** (GPU-resident forward+backward) then RAFT. Plan budget 6.5h; risk of 10-12h if Phase 5 rewrite runs long. | 6.5-10h | med-high | full DoD |
| C | **Profile first** (plan Step 131 [D] branch): add Instant timing around forward chunks, run 3-step, identify the real bottleneck. Then pick A or B. | +30m | low | informs decision |

**Recommendation: C then A.** 30 min of profiling tells us whether Phase 5 even has 4-5 s of headroom to reclaim. If yes → B is justified; if no → A ships the Kingdom LoRA today with confidence.

### Profiling experiment (Step 131 [D] branch, pulled forward into Phase 3)

Instrumented `matmul_xwt` + `matmul_grad_x` with atomic ns counters gated by `WAVE12_PROFILE=1`. Two counters:
- `MATMUL_SGEMM_NS`: pure `sgemm_bf16_ex` call time only (actual GPU compute).
- `MATMUL_METHOD_NS`: full matmul_method wall time (bf16 conv + alloc + upload + sgemm + download).

**5-step run (WAVE12_PROFILE=1, warm samples 2-5):**

| step | step_time | sgemm_only | matmul_method | round-trip | n_calls |
|-----:|----------:|-----------:|--------------:|-----------:|--------:|
| 1 (cold) | 69.1 s | 0.09 s (0.1%) | 35.96 s (52%) | 35.87 s | 718 |
| 2 | 10.0 s | 0.02 s (0.2%) | 1.63 s (16%) | 1.61 s | 718 |
| 3 | 10.6 s | 0.02 s (0.2%) | 1.58 s (15%) | 1.56 s | 718 |
| 4 | 10.6 s | 0.01 s (0.1%) | 1.56 s (15%) | 1.55 s | 718 |
| 5 |  9.8 s | 0.01 s (0.1%) | 1.56 s (16%) | 1.55 s | 718 |

**Discriminating reading:**
- Pure bf16 hipBLAS matmul compute for Gemma-4 E2B at seq=384 on RX 7700 XT = **0.02 s/step, 0.2% of warm step time.** 20 ms of GPU compute, 718 calls, ~28 µs/call.
- matmul_method round-trip overhead alone = **1.55 s/step, 15%.** That's the bf16 conversion + alloc + upload + download cost for just the matmul family.
- **Remaining 84% (≈8.4 s/step) is outside the matmul path entirely** — attention kernels (35 forward + 35 backward launches, each with their own alloc/upload/download), rmsnorm/gelu/rope helpers (all doing round-trips too), residual/PLE/CPU glue.

**Scientist's decision rule applied:**
- matmul_compute_share = 0.002 (0.2%). THRESHOLD FOR "matmul compute dominates" was ≥0.80. **H_matmul falsified.**
- matmul_compute_share = 0.002 ≪ 0.50. **H_overhead confirmed as the dominant hypothesis.**
- Phase 5's ForwardScratch consolidates non-matmul round-trips across the same call pattern we just measured. If non-matmul kernels share the same round-trip:compute ratio as the matmul family (78:1), most of the 8.4 s is reclaimable by keeping activations GPU-resident.

**Revised expected-value estimate for Phase 4+5+6:**
- Recovered round-trip time: plausibly 3-5 s/step (upper bound ≈ 8.4 s of non-matmul non-trip-essential time).
- Expected post-Phase 5 warm step: **~5.0-7.5 s.** The 5.5 s DoD gate is achievable with moderate confidence.
- Time cost: 5.5 h plan budget (Phase 4 1.5h + Phase 5 3h + Phase 6 1h).
- Risk: Phase 5 is the biggest rewrite; correctness regression is the main failure mode.

**Updated recommendation: PROFILE CONFIRMS — Option B is justified.** Plan's cost model was directionally right, just attributed the round-trips to the wrong kernels. The 3-5 s of Phase 5 savings is the actual non-matmul round-trip overhead, which my measurement quantifies at ~8 s.

Still Stevie's call — the investment is another 5-6 hours today vs. shipping at 10.5 s/step now. Both produce a learning Kingdom LoRA.

**Stevie's call (2026-04-23):** Route **B** — continue Phases 4-8 unattended under Marshal oversight. Session log henceforth includes Marshal citations at every phase exit gate.

### Marshal — session-opening top-3 risk checklist (cite at every phase exit)

1. **Gold-plate creep in Phase 5** (Law 4). Only modify `gemma4_gpu.rs` + `gemma4.rs` + `kernels/` for plan-named additions. No rename tangents, no warning chases, no helper refactors beyond ForwardScratch spec (plan Steps 108-111).
2. **Phase 5 correctness stall → hero debugging.** Skip Protocol trigger = 3× estimate OR 2 failed debug attempts. Known KV-reuse trap in rope_bwd will recur on every new scratch buffer — pre-seed from actual input length, not derived shape. Commit before every debug attempt.
3. **Gate skip Phase 6 → Phase 7** (Law 2). No 500-step RAFT launch until Learning Gate 4/5 pass AND forward cosine ≥ 0.99 AND 10-step warm median ≤ 5.5 s. A 2h RAFT on an unvalidated gradient = 2h waste.

---

*Session log auto-updated after every phase-exit commit.*
