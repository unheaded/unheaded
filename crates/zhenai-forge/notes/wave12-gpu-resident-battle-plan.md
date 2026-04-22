# WAVE12 BATTLE PLAN — GPU-Resident Forge @ seq=384 (≤5s/step, 500-step RAFT)

**Date forged:** 2026-04-22
**Sprint:** WAVE12 — close the 2× gap from WAVE11's 10.5 s/step to the battle-plan gate of ≤5 s/step warm, then run a full 500-step Kingdom RAFT LoRA with eval descent on held-out.
**Prerequisite:** WAVE11 Phase 8c landed (HEAD `5d32f9d5` or descendant). seq=384 Kingdom RAFT runs at ~10.5s/step warm. All 21 WAVE11 kernel unit tests green. `/var/zhen/models/gemma-4-E2B-it.gguf` present (9.3 GB). `/tmp/24h-kingdom-train.jsonl` present (3568 examples, seq=384 tokenized).
**Target:** `zhenai-forge train-gemma4 --model gemma-4-E2B-it.gguf --data /tmp/24h-kingdom-train.jsonl --steps 500 --save-every 100 --output raft/kingdom-w12.lora.gguf` completes at ≤5 s/step warm AND held-out eval loss descends below the training-loss baseline at step 500.
**Estimated Duration:** 8-12 wall-clock hours across 1-2 sessions on west (RX 7700 XT, gfx1101, ROCm 6.4).
**Agent Strategy:** solo/coordinator — GPU serializes runs; all phases executed in-session by the main agent.
**Commit Cadence:** every **5** steps (150 total / 20 ≈ 7.5 → clamp to 5). Phase-exit commit mandatory.
**Stuck Protocol:** skip after 3× time estimate or 2 failed debug attempts. Preserve known-good state before skip.

---

## LEGEND

```
[B]            Bash command (run directly)
[V]            Verification gate (MUST pass before proceeding)
[D]            Debug branch (only if prior step fails)
[W]            Write/create file
[R]            Read/inspect file
[CODE]         In-source edit (Rust / HIP)
[BUILD]        Build step (cargo / hipcc)
[TEST]         Test execution
[C]            Commit checkpoint (git commit with --no-gpg-sign per session policy)
[DECIDE]       Decision point WITH pre-seeded recommendation — agent proceeds autonomously
[ESCALATE]     Requires human input — STOP. (Used sparingly.)
[STUCK]        Step skipped via Skip Protocol
[BLOCKED]      Blocked by upstream STUCK (NOT for decisions)
[GPU:t=Xs]     Explicit GPU wall-clock budget for this step
[ABORT-IF]     Per-phase kill criterion
```

---

## CRITICAL FACTS (cached for executing agent)

- **Repo:** `/home/govan/tmp/unheaded/` (branch `main`, commits unsigned per session policy).
- **Crate:** `crates/zhenai-forge/` — all edits live here unless noted.
- **Binary:** `crates/zhenai-forge/target/release/zhenai-forge` (CLI entry `train-gemma4`).
- **Model:** `/var/zhen/models/gemma-4-E2B-it.gguf` — 35 layers, n_embd=1536, n_ff=6144, n_head=8, n_head_kv=1, head_dim_full=512, head_dim_swa=256, vocab=262144, 28 sliding + 7 full attention layers, KV-shared from layer 20.
- **Training corpus:** `/tmp/24h-kingdom-train.jsonl` (3568 ex, tokenized at seq=384), `/tmp/24h-kingdom-eval.jsonl` (397 ex, held-out).
- **GPU:** RX 7700 XT, 12.87 GB VRAM, gfx1101. Warm-path requires PleMode::Cpu (4.57 GB VRAM) to leave headroom for activation pools.
- **Baseline to beat (Phase 8c):** cold ≈78 s, warm ≈10.5 s/step, 10-step loss 21.21 → 10.69. ≤5 s/step target is ~2× improvement.
- **Kernel library:** `$OUT_DIR/libwave11_kernels.so` (rebuilt by `build.rs` from `crates/zhenai-forge/kernels/*.hip.cpp` via `hipcc --offload-arch=gfx1101 --shared -fPIC -O3`).
- **Integration surface:** `src/gemma4_gpu.rs::forward_gemma4_gpu` (forward), `src/gemma4.rs::backward_gemma4_with_lora` (backward), `src/hip_kernels/{rmsnorm,gelu,softmax,rope,attn}.rs` (FFI wrappers), `src/backend.rs` (trait).
- **Relevant ADRs:** ADR-004 (no new crate deps), ADR-048 (ForgeBackend trait), ADR-049 (WAVE11 GPU kernels).
- **Known quirks:**
  - Gemma-4 E2B KV-reuse crosses head_dim types for some consumer layers — allocate buffers from actual `.len()`, not derived shape formulas (see Phase 8b postmortem).
  - `head_dim > blockDim.x=256` (layer 34 at 512) was the latent-stride bug caught in Phase 8a — any NEW `threadIdx.x per d` kernel must use `for (d = tid; d < head_dim; d += blockDim.x)`.
  - Memory pressure at 14 GB RAM with full Gemma-4 load + swap → stalled GPU uploads. Keep RSS under 11 GB; avoid running tests in parallel with training smoke on GPU.
  - JIT warmup of new kernel variants adds ~8-10 s to first step after code change. Discount in timing.

---

## PREFLIGHT HYPOTHESES (verified in Phase 0)

| # | Hypothesis | Verification |
|--:|-----------|--------------|
| H1 | WAVE11 HEAD clean at `5d32f9d5`+, tree clean | `git log -1; git status` |
| H2 | Kingdom corpus at seq=384 still cached at `/tmp/24h-kingdom-*.jsonl` | `ls -lh /tmp/24h-kingdom-*.jsonl` |
| H3 | 21 WAVE11 kernel tests still pass | `cargo test ... -- hip_kernels::` |
| H4 | `test_gemma4_gpu_train_step_loss_descent` still green | `cargo test ... test_gemma4_gpu_train_step_loss_descent` |
| H5 | 3-step seq=384 smoke still 10-11 s warm | repro smoke, record |
| H6 | ≥10 GB free RAM at start | `free -h` |

If ANY hypothesis fails in Phase 0 → resolve before any W12 code change.

---

## KNOWN FAILURES BASELINE

Run `cargo test --release --tests 2>&1 | tail -5` in Phase 0. Record the baseline count of ignored/skipped tests. Any NEW failure in subsequent phases is a regression.

Expected baseline: 0 failures, small count (likely 2-5) of model-dependent tests that early-return when GGUF is absent — those pass trivially when GGUF is present. No NaN, no crash.

---

## TIME BUDGET SUMMARY

| Phase | Name | Wall | GPU | Cumulative |
|------:|------|----:|----:|-----------:|
| 0 | Preflight + baseline re-confirm | 30m | 10m | 0.5 h |
| 1 | Shared mask cache (low-risk quick win) | 45m | 5m | 1.25 h |
| 2 | Backward rmsnorm/gelu/rope on GPU | 2 h | 15m | 3.25 h |
| 3 | First-checkpoint measurement | 20m | 10m | 3.6 h |
| 4 | Matmul-on-GPU helpers (GpuBuffer in/out) | 1.5 h | 10m | 5.1 h |
| 5 | GPU-resident forward chain (no CPU round-trip) | 3 h | 30m | 8.1 h |
| 6 | Forward+backward regression + second checkpoint | 1 h | 30m | 9.1 h |
| 7 | 500-step Kingdom RAFT + eval descent | 2 h | 100m | 11.1 h |
| 8 | ADR-050 + session log + memory + handoff | 1 h | 0 | 12.1 h |

Critical path: 0 → 1 → 2 → 3 → (decide) → 4 → 5 → 6 → 7 → 8.

Short-circuit: if Phase 3 measures ≤5 s/step warm AND Learning Gate green, **skip Phases 4-6** and go directly to Phase 7. Saves ~4.5 hours.

---

## PHASE 0: PREFLIGHT + BASELINE RE-CONFIRM (Steps 1-18) — 30 min

**Goal:** Verify ground state; confirm WAVE11 Phase 8c is live and the 10.5 s/step warm baseline reproduces.
**Prerequisite:** fresh session, working tree clean.
**Agent:** Coordinator (solo).
**ABORT-IF:** HEAD dirty OR GGUF missing OR any baseline hypothesis fails.

- [ ] **Step 1** [B] ~5s: Verify git state.
  ```bash
  cd /home/govan/tmp/unheaded && git log --oneline -5 && git status --short
  ```

- [ ] **Step 2** [V]: HEAD is `5d32f9d5` or descendant; working tree clean.
  - If dirty → STOP. Reconcile or `git stash` before proceeding.

- [ ] **Step 3** [B] ~5s: Check GGUF + corpus + memory.
  ```bash
  ls -lh /var/zhen/models/gemma-4-E2B-it.gguf /tmp/24h-kingdom-train.jsonl /tmp/24h-kingdom-eval.jsonl; free -h | head -3
  ```

- [ ] **Step 4** [V]: GGUF ≈9.3 GB present; train/eval jsonl present; MemAvailable ≥ 9 GB.
  - If corpus missing → re-tokenize with `scripts/tokenize-kingdom-for-gemma4.py` (see Appendix A1).

- [ ] **Step 5** [B] ~5s: Confirm no other forge processes holding GPU.
  ```bash
  pgrep -af "zhenai-forge|rocm-smi" | grep -v grep || echo "clean"
  rocm-smi --showmeminfo vram 2>&1 | grep "GPU\[0\]" | head -2
  ```

- [ ] **Step 6** [V]: No zombie forge processes; GPU[0] VRAM < 1 GB used.
  - If zombies → `pkill -9 -f zhenai-forge && sleep 3`.

- [ ] **Step 7** [BUILD] ~60s: Build forge release.
  ```bash
  cargo build --manifest-path crates/zhenai-forge/Cargo.toml --release 2>&1 | tail -3
  ```

- [ ] **Step 8** [V]: `Finished release profile` printed; zero rustc errors.

- [ ] **Step 9** [TEST] ~30s: All 21 WAVE11 kernel tests pass.
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- hip_kernels:: 2>&1 | tail -5
  ```

- [ ] **Step 10** [V]: `test result: ok. 21 passed; 0 failed`.

- [ ] **Step 11** [C] **COMMIT CHECKPOINT** — no code changes yet, but record baseline in notes.
  ```bash
  : # no-op placeholder; first real commit lands at Step 20
  ```

- [ ] **Step 12** [TEST] ~3m: Forward-matches-cpu regression (confirms WAVE11 kernels still OK on real Gemma-4).
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- gemma4_gpu::tests::test_gemma4_gpu_forward_matches_cpu --nocapture 2>&1 | tail -10
  ```

- [ ] **Step 13** [V]: `top-1 match: true`; cosine ≥ 0.998; test `ok`.
  - If cosine drops unexpectedly → STOP. Regression in WAVE11 kernels (should not happen on clean HEAD).

- [ ] **Step 14** [TEST] ~3m: Training descent regression.
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- gemma4_gpu::tests::test_gemma4_gpu_train_step_loss_descent --nocapture 2>&1 | tail -10
  ```

- [ ] **Step 15** [V]: Loss descends (step 1 > step 2 > step 3), no NaN, `ok`.

- [ ] **Step 16** [B] ~10m: Reproduce Phase 8c baseline on seq=384 (3-step smoke).
  ```bash
  crates/zhenai-forge/target/release/zhenai-forge train-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-kingdom-train.jsonl \
    --steps 3 --lr 1e-3 --answer-start 1 \
    > /tmp/w12-phase0-baseline.log 2>&1
  ```

- [ ] **Step 17** [V]: **PHASE 0 BASELINE GATE** — warm step (step 3) between 9.5s and 12s.
  ```bash
  grep "step_time" /tmp/w12-phase0-baseline.log | tail -3
  ```
  - Expected: step 1 ≈ 70-80 s (cold), step 2 ≈ 10-12 s, step 3 ≈ 10-11 s.
  - If step 3 > 12 s → [D] Check VRAM pressure / check swap via `free -h`.
  - If step 3 < 9 s → unusually fast; proceed but record for later comparison.
  - If smoke panics → STOP. Reconcile HEAD.

- [ ] **Step 18** [W] ~3m: Initialize WAVE12 session log.
  ```
  Create crates/zhenai-forge/notes/wave12-session-log.md with header + Phase 0 baseline line.
  ```

- [ ] **Step 19** [V]: Session log file exists with baseline recorded.

- [ ] **Step 20** [C] ~10s: **PHASE 0 COMMIT**
  ```bash
  git add crates/zhenai-forge/notes/wave12-session-log.md && \
  git commit --no-gpg-sign -m "docs(forge): [PLAN W12] Phase 0 — preflight, baseline confirmed @ ~10.5s/step warm"
  ```

---

## PHASE 1: SHARED MASK CACHE (Steps 21-40) — 45 min

**Goal:** Stop rebuilding and uploading the attention mask on every layer call. Gemma-4 has exactly two mask shapes per step (causal for 7 full layers, sliding-window(512) for 28 sliding layers); each is ~4.7 MB at seq=384 × n_heads=8. Caching saves ~165 MB/step of PCIe traffic.
**Prerequisite:** Phase 0 exit gate passed.
**Agent:** Coordinator (solo).
**Time:** 45 min wall, ~5 min GPU.
**ABORT-IF:** regression breaks cosine test after mask change.

### Design

Introduce a `MaskCache` struct on the caller's stack (local to `forward_gemma4_gpu`), keyed by `AttnMask` enum value. First use uploads; subsequent uses of the same mask type in the same step reuse the already-uploaded GpuBuffer. Rebuilt per-step (masks depend only on seq, constant within a step).

Structure:
```rust
struct MaskCache {
    causal: Option<GpuBuffer>,
    sliding: std::collections::HashMap<usize, GpuBuffer>,  // keyed by window
}
impl MaskCache { fn get_or_build(&mut self, mask: AttnMask, n_heads, seq) -> Result<&GpuBuffer, String>; }
```

### Steps

- [ ] **Step 21** [R] ~2m: Re-read `attention_forward_gpu_kernels` in `src/gemma4_gpu.rs` (around line 771) to locate mask build.

- [ ] **Step 22** [CODE] ~10m: Add `MaskCache` struct near the top of the helper region (above `attention_forward_gpu_kernels`).
  ```
  Add struct MaskCache { causal: Option<GpuBuffer>, sliding: Vec<(usize, GpuBuffer)> }
  with fn new(), fn get(&mut self, mask, n_heads, seq_len).
  The build path moves from the helper body into MaskCache::build_*.
  ```

- [ ] **Step 23** [CODE] ~5m: Change `attention_forward_gpu_kernels` signature to take `&mut MaskCache` alongside `mask`. Pull the mask buffer via `cache.get(...)`. Remove the local `mask_cpu` build + `mask_buf.alloc/upload`.

- [ ] **Step 24** [CODE] ~5m: In `forward_gemma4_gpu` (around line 403), add `let mut mask_cache = MaskCache::new();` before the layer loop. Pass `&mut mask_cache` to the attention helper call (around line 534).

- [ ] **Step 25** [C] **COMMIT CHECKPOINT** (Steps 21-25).
  ```bash
  cargo build --manifest-path crates/zhenai-forge/Cargo.toml --release 2>&1 | grep -E "^error|Finished" | tail -3
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 1 — MaskCache struct + forward integration"
  ```

- [ ] **Step 26** [V]: Build green, no rustc errors.
  - If error → [D] likely borrow issue from `&mut MaskCache` through closure. Fix via `&mut` pass-through or extract mask to local before each call.

- [ ] **Step 27** [TEST] ~3m: Forward-matches-cpu regression after mask cache.
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- gemma4_gpu::tests::test_gemma4_gpu_forward_matches_cpu --nocapture 2>&1 | tail -6
  ```

- [ ] **Step 28** [V]: cosine ≥ 0.998, top-1 match, test `ok`.
  - If regression → [D] mask layout bug. Compare against Phase 0 log file (causal `j <= i`, sliding `j <= i && i - j < w`).

- [ ] **Step 29** [TEST] ~3m: Training descent regression (confirms cache doesn't break Adam step).
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- gemma4_gpu::tests::test_gemma4_gpu_train_step_loss_descent --nocapture 2>&1 | tail -10
  ```

- [ ] **Step 30** [V]: Loss descends, no NaN, `ok`.

- [ ] **Step 31** [B] ~10m: seq=384 3-step smoke.
  ```bash
  crates/zhenai-forge/target/release/zhenai-forge train-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-kingdom-train.jsonl \
    --steps 3 --lr 1e-3 --answer-start 1 \
    > /tmp/w12-phase1-timing.log 2>&1
  ```

- [ ] **Step 32** [V]: **PHASE 1 EXIT GATE** — warm step ≤ 10.5 s (not worse than baseline).
  ```bash
  grep "step_time" /tmp/w12-phase1-timing.log | tail -3
  ```
  - Expected improvement: 0.2-0.5 s/step. If no improvement, cache hit rate may be low → [D] check per-layer mask-build count with a temporary eprintln.
  - If slower → revert mask cache commit, investigate; do NOT proceed to Phase 2 with regression.

- [ ] **Step 33** [W] ~3m: Update `wave12-session-log.md` with Phase 1 result.

- [ ] **Step 34** [C] **COMMIT CHECKPOINT** (Steps 26-34).
  ```bash
  git add crates/zhenai-forge/notes/wave12-session-log.md && \
  git commit --no-gpg-sign -m "docs(forge): [PLAN W12] Phase 1 — mask cache timing recorded"
  ```

- [ ] **Step 35** [DECIDE]: Mask cache alone hit ≤5 s/step?
  - **RECOMMENDATION:** almost certainly NO (mask cache is a 0.2-0.5 s win). Proceed to Phase 2.
  - Override only if step 3 timing < 5.0 s, in which case skip to Phase 3 checkpoint.

- [ ] **Step 36** [B] ~1s: Reserved for intra-phase rework. No-op.
- [ ] **Step 37** [B] ~1s: Reserved. No-op.
- [ ] **Step 38** [B] ~1s: Reserved. No-op.
- [ ] **Step 39** [B] ~1s: Reserved. No-op.
- [ ] **Step 40** [V]: Phase 1 complete, session log updated.

---

## PHASE 2: BACKWARD rmsnorm / gelu / rope ON GPU (Steps 41-80) — 2 hours

**Goal:** Swap the per-row CPU backward loops (`backward::rmsnorm_backward`, `backward::gelu_backward`, `backward::rope_backward_partial`, `backward::per_head_rmsnorm_backward`, `backward::v_norm_backward`) for the already-shipped Phase 3-6 GPU backward kernels. Pattern repeats Phase 8b/8c — highest confidence, likely biggest remaining win.
**Prerequisite:** Phase 1 green.
**Agent:** Coordinator (solo).
**Time:** 2 h wall, ~15 min GPU.
**ABORT-IF:** backward regression breaks training descent (loss NOT monotonic on first 3 steps).

### Target call sites in `src/gemma4.rs::backward_gemma4_with_lora`

Grep target: `rmsnorm_backward\|gelu_backward\|rope_backward_partial\|per_head_rmsnorm_backward\|v_norm_backward` in `gemma4.rs`. Confirmed call sites (re-verify with grep at Step 42):

1. Site 6 — post_attention_norm rmsnorm_backward (per-row) before attention backward
2. Site 9 — post_ffw_norm rmsnorm_backward
3. Site 10 — ffn_norm rmsnorm_backward
4. Site 11 — gelu_mul backward (FFN fused)
5. Site 12 — rope_backward_partial (Q and K)
6. Site 13 — per_head_rmsnorm_backward (Q-norm and K-norm)
7. Site 14 — v_norm_backward (weightless V-norm)
8. Site 15 — attn_norm rmsnorm_backward (entry to layer)
9. Site 16 — PLE branch rmsnorm_backward (post_norm)
10. Site 17 — PLE branch gelu_backward (gate_post_gelu)
11. Site 18 — output_norm rmsnorm_backward (final)
12. Site 19 — per_layer_proj_norm rmsnorm_backward (PLE inner)

### Steps

- [ ] **Step 41** [R] ~2m: Re-read `src/hip_kernels/rmsnorm.rs::rmsnorm_bwd` + `src/hip_kernels/gelu.rs::gelu_bwd, gelu_mul_bwd` + `src/hip_kernels/rope.rs::rope_partial_bwd` signatures.

- [ ] **Step 42** [B] ~1m: Enumerate CPU backward call sites.
  ```bash
  grep -n "rmsnorm_backward\|gelu_backward\|gelu_mul_backward\|rope_backward_partial\|per_head_rmsnorm_backward\|v_norm_backward" crates/zhenai-forge/src/gemma4.rs | head -40
  ```

- [ ] **Step 43** [V]: Site count matches expected (≈12 sites). Record the list in scratch for cross-check as we swap.

- [ ] **Step 44** [CODE] ~10m: In `gemma4_gpu.rs`, add three backward batch helpers mirroring Phase 8c forward helpers:
  - `rmsnorm_batch_bwd_on_gpu(grad_out, input, weight_buf, eps, rows, d) -> Vec<f32>`
  - `gelu_batch_bwd_on_gpu(grad_out, input) -> Vec<f32>`
  - `gelu_mul_batch_bwd_on_gpu(grad_out, gate_pre, up_pre) -> (Vec<f32>, Vec<f32>)`
  - `rope_batch_bwd_on_gpu(grad_out, cos, sin, seq, n_head, head_dim, rope_dim) -> Vec<f32>`
  - `per_head_rmsnorm_batch_bwd_on_gpu(grad_out, input, weight_buf, eps, seq, n_head, head_dim) -> Vec<f32>` (uses rmsnorm_bwd with rows = seq*n_head, d = head_dim)

- [ ] **Step 45** [C] **COMMIT CHECKPOINT** (Steps 41-45).
  ```bash
  cargo build --manifest-path crates/zhenai-forge/Cargo.toml --release 2>&1 | grep -E "^error|Finished" | tail -3
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 2 — backward GPU batch helpers added"
  ```

- [ ] **Step 46** [V]: Build green.
  - If `gelu_mul_bwd` FFI signature mismatch → re-check `crates/zhenai-forge/src/hip_kernels/gelu.rs` line ~70.

- [ ] **Step 47** [CODE] ~8m: Swap Site 6 (post_attention_norm backward) in `gemma4.rs`. Replace the per-row CPU loop with `rmsnorm_batch_bwd_on_gpu(&grad_post_attn, &o_out, &gpu.post_attention_norm[il], eps, seq, n_embd)`.
  - Guard with `if gpu.is_some()` — CPU path unchanged.

- [ ] **Step 48** [CODE] ~8m: Swap Site 9 (post_ffw_norm backward). Same pattern.

- [ ] **Step 49** [CODE] ~8m: Swap Site 10 (ffn_norm backward). Same pattern.

- [ ] **Step 50** [C] **COMMIT CHECKPOINT** (Steps 46-50).
  ```bash
  cargo build --manifest-path crates/zhenai-forge/Cargo.toml --release 2>&1 | grep -E "^error|Finished" | tail -3
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 2 — rmsnorm_bwd swapped at sites 6/9/10"
  ```

- [ ] **Step 51** [V]: Build green.

- [ ] **Step 52** [TEST] ~3m: Training descent after 3-of-12 swaps.
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- gemma4_gpu::tests::test_gemma4_gpu_train_step_loss_descent --nocapture 2>&1 | tail -8
  ```

- [ ] **Step 53** [V]: Loss descends monotonically step 1 → 3. No NaN.
  - If regression → [D] per-site diff. Revert one site at a time to localize (binary search via commits).

- [ ] **Step 54** [CODE] ~8m: Swap Site 11 (gelu_mul backward — FFN fused).
  - Returns two grads (grad_gate_pre, grad_up_pre). Adjust callsite accordingly.

- [ ] **Step 55** [C] **COMMIT CHECKPOINT** (Steps 51-55).
  ```bash
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 2 — gelu_mul_bwd swapped (Site 11)"
  ```

- [ ] **Step 56** [CODE] ~8m: Swap Site 12 (rope_backward_partial for Q and K — two calls per layer).

- [ ] **Step 57** [CODE] ~8m: Swap Site 13 (per_head_rmsnorm_backward for Q-norm and K-norm).

- [ ] **Step 58** [CODE] ~5m: Site 14 — v_norm_backward (weightless). Uses rmsnorm_bwd with a `ones` weight buffer. Either: (a) keep on CPU (small), or (b) add `rmsnorm_bwd_no_weight` variant that internally generates ones.
  - **RECOMMENDATION [DECIDE]:** keep CPU for Site 14. The V-norm tensor is `seq * n_head_kv * head_dim` = tiny; alloc+upload+download of a ones buffer would cost more than the CPU loop. Override only if profiling shows Site 14 in top 5.

- [ ] **Step 59** [TEST] ~3m: Training descent after Sites 11/12/13.

- [ ] **Step 60** [C] **COMMIT CHECKPOINT** (Steps 56-60).
  ```bash
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 2 — rope/per-head-rms_bwd swapped (Sites 12/13)"
  ```

- [ ] **Step 61** [V]: Loss descends; no NaN.

- [ ] **Step 62** [CODE] ~8m: Swap Sites 15/16/18/19 (attn_norm, PLE post_norm, output_norm, per_layer_proj_norm backward).

- [ ] **Step 63** [CODE] ~5m: Swap Site 17 (PLE gelu_backward — unfused).

- [ ] **Step 64** [BUILD] ~20s: Rebuild.

- [ ] **Step 65** [C] **COMMIT CHECKPOINT** (Steps 61-65).
  ```bash
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 2 — all 11 remaining backward sites on GPU"
  ```

- [ ] **Step 66** [V]: Build green.

- [ ] **Step 67** [TEST] ~3m: Full training descent regression.
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- gemma4_gpu::tests::test_gemma4_gpu_train_step_loss_descent --nocapture 2>&1 | tail -10
  ```

- [ ] **Step 68** [V]: Loss trajectory healthy; Δ from baseline `20.78 → 10.74 → 5.74` within ±0.5 at each step.
  - If large divergence → [D] find which Site diverged. Revert newest commit; bisect.

- [ ] **Step 69** [TEST] ~3m: forward_matches_cpu (confirm FORWARD still works after backward changes — should be untouched).
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- gemma4_gpu::tests::test_gemma4_gpu_forward_matches_cpu --nocapture 2>&1 | tail -5
  ```

- [ ] **Step 70** [V]: cosine ≥ 0.998, top-1 match.

- [ ] **Step 71** [B] ~10m: seq=384 3-step smoke.
  ```bash
  crates/zhenai-forge/target/release/zhenai-forge train-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-kingdom-train.jsonl \
    --steps 3 --lr 1e-3 --answer-start 1 \
    > /tmp/w12-phase2-timing.log 2>&1
  ```

- [ ] **Step 72** [V]: **PHASE 2 EXIT GATE** — warm step ≤ 8.5 s (at least ~2 s/step savings from backward GPU swap).
  ```bash
  grep "step_time" /tmp/w12-phase2-timing.log | tail -3
  ```
  - Expected: step 2-3 between 6.5 and 8.5 s.
  - Loss step 3 ≈ 16.5-17.5 (matches Phase 9 trajectory at step 3).
  - If step 3 > 9 s → backward swaps didn't help as much as expected; profile with `/usr/bin/time -v` wrapper to confirm bottleneck moved.

- [ ] **Step 73** [W] ~3m: Update session log with Phase 2 timing + delta.

- [ ] **Step 74** [C] **COMMIT CHECKPOINT** (Steps 66-74).
  ```bash
  git add -A && git commit --no-gpg-sign -m "docs(forge): [PLAN W12] Phase 2 — backward GPU ops integrated, timing recorded"
  ```

- [ ] **Step 75** [B] ~1s: Reserved.
- [ ] **Step 76** [B] ~1s: Reserved.
- [ ] **Step 77** [B] ~1s: Reserved.
- [ ] **Step 78** [B] ~1s: Reserved.
- [ ] **Step 79** [V]: Phase 2 DoD — all 11 backward sites on GPU, training descent green, forward regression green, warm step ≤ 8.5 s.
- [ ] **Step 80** [B] ~1s: Phase 2 closeout — mark as DONE in session log.

---

## PHASE 3: FIRST-CHECKPOINT MEASUREMENT + SHORT-CIRCUIT (Steps 81-92) — 20 min

**Goal:** Decide whether we've already hit the ≤5 s/step target after Phases 1-2. If yes, jump directly to Phase 7 (500-step RAFT). If no, continue to Phase 4 (GPU-resident refactor).
**Prerequisite:** Phase 2 exit gate passed.
**Agent:** Coordinator (solo) — all of these are measurement + decision.
**Time:** 20 min wall, ~10 min GPU.

- [ ] **Step 81** [B] ~10m: 10-step steady-state smoke.
  ```bash
  crates/zhenai-forge/target/release/zhenai-forge train-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-kingdom-train.jsonl \
    --steps 10 --lr 1e-3 --answer-start 1 \
    > /tmp/w12-phase3-10step.log 2>&1
  ```

- [ ] **Step 82** [V]: 10-step completes, final loss healthy.

- [ ] **Step 83** [B] ~10s: Extract timing distribution.
  ```bash
  grep "step_time" /tmp/w12-phase3-10step.log | awk '{print $5}' | tr -d 'step_time=s' | sort -n
  ```

- [ ] **Step 84** [V]: Record median + min + max of warm steps (steps 3-10; ignore cold step 1 and JIT-warm step 2).

- [ ] **Step 85** [DECIDE]: Hit ≤5 s/step median warm?
  - **RECOMMENDATION:** unlikely after Phase 2 alone (backward GPU savings estimated at 2-4 s; current baseline 10.5 → projected 6.5-8.5). Expect NO; proceed to Phase 4.
  - **Override ONLY if** median warm step ≤ 5.0 s across steps 3-10 → skip to Phase 7.

- [ ] **Step 86** [DECIDE]: Hit ≤6.5 s/step (good-enough threshold for a shipped 500-step RAFT without further refactor)?
  - **RECOMMENDATION:** if yes AND Stevie is time-pressured, skip to Phase 7; otherwise push for ≤5 s via Phases 4-6.
  - Record decision in session log.

- [ ] **Step 87** [W] ~2m: Update session log with Phase 3 decision + rationale.

- [ ] **Step 88** [C] **PHASE 3 COMMIT**
  ```bash
  git add crates/zhenai-forge/notes/wave12-session-log.md && \
  git commit --no-gpg-sign -m "docs(forge): [PLAN W12] Phase 3 — checkpoint measurement + next-phase decision"
  ```

- [ ] **Step 89** [B] ~1s: Phase 3 gate: decision recorded.
- [ ] **Step 90** [B] ~1s: If decision = short-circuit to Phase 7 → goto Step 123.
- [ ] **Step 91** [B] ~1s: Otherwise proceed to Phase 4.
- [ ] **Step 92** [V]: Phase 3 exit: decision durable in git + session log.

---

## PHASE 4: MATMUL HELPERS (GpuBuffer in/out) (Steps 93-107) — 1.5 hours

**Goal:** Add `matmul_xwt_gpu_in_out` and `matmul_grad_x_gpu_in_out` variants on `Gemma4GpuWeights`. They take `&GpuBuffer` for input and write to `&GpuBuffer` output — no CPU round-trip. Unlocks GPU-resident forward in Phase 5.
**Prerequisite:** Phase 3 decision = "continue to Phase 4".
**Agent:** Coordinator (solo).
**Time:** 1.5 h wall, ~10 min GPU.
**ABORT-IF:** existing `matmul_xwt` unit tests regress.

### Design

Existing `matmul_xwt(&self, w_gpu_bf16, input_f32, m, n, k) -> Result<Vec<f32>, String>`:
1. CPU f32 → bf16 conversion
2. hipMalloc input_bf16 + upload
3. hipMalloc output_f32
4. sgemm_bf16_ex
5. download to Vec<f32>

New `matmul_xwt_gpu_in_out(&self, w_gpu_bf16, input_gpu_bf16: &GpuBuffer, output_gpu_f32: &GpuBuffer, m, n, k)`:
- Skip step 1 (caller pre-converts or keeps bf16 on GPU).
- Skip step 2 (input already on GPU).
- Caller owns output_f32 buffer (allocated once, reused).
- Skip step 5 (output stays on GPU).
- Returns `Result<(), String>`.

For activations chain: rmsnorm output is f32. Matmul expects bf16 input. Need a `f32_to_bf16_on_gpu` kernel → new Phase 4a inline sub-phase, or handle via small inline conversion kernel.

### Steps

- [ ] **Step 93** [R] ~2m: Re-read `matmul_xwt` body in `src/gemma4_gpu.rs` (line 285).

- [ ] **Step 94** [CODE] ~8m: Add `impl Gemma4GpuWeights` method `matmul_xwt_gpu_in_out` — same sgemm_bf16_ex call but receives pre-allocated GpuBuffers.

- [ ] **Step 95** [CODE] ~8m: Add `matmul_grad_x_gpu_in_out` — same pattern for the grad-x backward matmul (mirrors `matmul_grad_x` at line 234).

- [ ] **Step 96** [CODE] ~10m: Add `f32_to_bf16.hip.cpp` kernel + `hip_kernels::convert::f32_to_bf16(out_gpu_bf16, in_gpu_f32, n) -> Result<(), String>`. Grid-stride loop writes bf16 to output. Use `common.hip.hpp` bf16 bit-pun helper.

- [ ] **Step 97** [TEST] ~2m: Add unit test `test_f32_to_bf16_matches_cpu` in `src/hip_kernels/convert.rs`. Verify cosine ≥ 0.9999 vs host-side `bf16::from_f32` loop over 10,000 element det_vec.

- [ ] **Step 98** [C] **COMMIT CHECKPOINT** (Steps 93-98).
  ```bash
  cargo build --manifest-path crates/zhenai-forge/Cargo.toml --release 2>&1 | grep -E "^error|Finished" | tail -3
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 4 — matmul gpu-in/out helpers + f32→bf16 kernel"
  ```

- [ ] **Step 99** [V]: Build green; `test_f32_to_bf16_matches_cpu` passes.

- [ ] **Step 100** [CODE] ~8m: Parallel path: add `matmul_xwt_f32_input_gpu_in_out` variant that accepts f32 input (skip internal f32→bf16 by calling f32_to_bf16 kernel into a scratch GpuBuffer — see Phase 5).

- [ ] **Step 101** [CODE] ~5m: Add a focused unit test `test_matmul_xwt_gpu_in_out_matches_cpu` on the same shape as layer 0 (seq=4, n_embd=1536, q_out_dim=2048). Verify ≥ 0.9999 cosine vs CPU bf16 reference.

- [ ] **Step 102** [TEST] ~5m: Run the new test.

- [ ] **Step 103** [V]: `test_matmul_xwt_gpu_in_out_matches_cpu` passes.
  - If fail → [D] most likely cause is `k`/`n`/`ldx` confusion between col-major hipBLAS and row-major our-side. Diff against `matmul_xwt` line-by-line for op_a/op_b and lda/ldb/ldc.

- [ ] **Step 104** [TEST] ~3m: Existing `test_gemma4_gpu_wq_matches_cpu` + `test_gemma4_gpu_matmul_grad_x_matches_cpu` still pass.

- [ ] **Step 105** [V]: No regression in existing matmul tests.

- [ ] **Step 106** [C] **COMMIT CHECKPOINT** (Steps 99-106).
  ```bash
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 4 — matmul gpu-in/out shape-level tests green"
  ```

- [ ] **Step 107** [V]: **PHASE 4 EXIT GATE** — gpu-in/out matmul variants usable, f32→bf16 kernel shipped, no test regression.

---

## PHASE 5: GPU-RESIDENT FORWARD CHAIN (Steps 108-147) — 3 hours

**Goal:** Rewrite `forward_gemma4_gpu` so that per-layer activation tensors stay on GPU across the rmsnorm → Q/K/V matmul → per-head norm → RoPE → attention → O matmul → post_attention_norm → FFN → post_ffw_norm → residual chain. Download only the layer cache fields that backward still consumes on CPU (q_rot, k_rot, v, attn_cache, attn_out).
**Prerequisite:** Phase 4 exit gate passed.
**Agent:** Coordinator (solo).
**Time:** 3 h wall, ~30 min GPU.
**ABORT-IF:** forward cosine regression < 0.99 (vs current Phase 8c baseline 0.998).

### Design

Introduce a `ForwardScratch` struct holding GpuBuffers for the hot-path activations:
```rust
struct ForwardScratch {
    hidden_f32: GpuBuffer,        // seq * n_embd * 4
    hidden_bf16: GpuBuffer,       // seq * n_embd * 2
    normed_f32: GpuBuffer,        // seq * n_embd * 4
    normed_bf16: GpuBuffer,       // seq * n_embd * 2
    q_out_f32: GpuBuffer,         // seq * q_out_dim_max * 4
    kv_out_f32: GpuBuffer,        // seq * kv_out_dim_max * 4 (x2 for K + V)
    ffn_gate_f32: GpuBuffer,      // seq * n_ff * 4
    ffn_up_f32: GpuBuffer,        // seq * n_ff * 4
    ffn_hidden_f32: GpuBuffer,    // seq * n_ff * 4
    ffn_hidden_bf16: GpuBuffer,   // seq * n_ff * 2
    ffn_out_f32: GpuBuffer,       // seq * n_embd * 4
    attn_out_f32: GpuBuffer,      // seq * q_out_dim_max * 4
    attn_out_bf16: GpuBuffer,     // seq * q_out_dim_max * 2
    // ... sized at max over all layers
}
```

Allocated once at start of forward, reused across all 35 layers. Covers the hot path; cold path (layer caches downloaded for backward) is unchanged.

### Steps

- [ ] **Step 108** [DESIGN] ~10m: Write the scratch struct definition. Compute max dims: at seq=512 (generous), n_embd=1536, n_ff=6144, q_out_dim_max=4096 (layer 34 full, n_head*head_dim=8*512), kv_out_dim_max=512 (n_head_kv*head_dim_full=1*512). Total scratch VRAM: ≈ (512 × (1536*6 + 6144*3 + 4096*2 + 512*2)) × 4 ≈ 120 MB. Fits easily in ~7 GB headroom.

- [ ] **Step 109** [CODE] ~15m: Add `ForwardScratch` in `src/gemma4_gpu.rs`. `impl ForwardScratch { fn alloc(hparams, max_seq) -> Result<Self, String>; }`.

- [ ] **Step 110** [C] **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 5 — ForwardScratch struct scaffolded"
  ```

- [ ] **Step 111** [CODE] ~25m: Rewrite `forward_gemma4_gpu` to use `ForwardScratch`. First site: hidden (embedding lookup) uploaded to `scratch.hidden_f32` ONCE. Per layer:
  1. rmsnorm into `scratch.normed_f32` (GPU kernel with input=hidden_f32, output=normed_f32; weights already GPU)
  2. f32_to_bf16 into `scratch.normed_bf16`
  3. matmul_xwt_gpu_in_out Q into `scratch.q_out_f32` (bf16 input, f32 output)
  4. matmul_xwt_gpu_in_out K, V
  5. per-head rmsnorm (GPU) using `scratch.q_out_f32` in-place (or scratch.q_tmp)
  6. RoPE (GPU) in-place on q_out_f32, k_out_f32
  7. attention (GPU kernels) — writes to `scratch.attn_out_f32`
  8. f32_to_bf16 for attn_out
  9. O matmul into `scratch.attn_out_f32` (reuse, but now mixed shapes — may need a dedicated `scratch.o_out_f32`)
  10. post_attention_norm (GPU) + residual add (GPU) — writes back to `scratch.hidden_f32`
  11. FFN chain (GPU) — normed → gate/up matmuls → fused gelu*up → down matmul → residual
  12. If PLE: per_layer norm + proj matmul + gelu + norm + residual (all GPU)

  LoRA remains CPU for now. Download `q_rot`, `k_rot`, `v`, `attn_cache`, `attn_out` per layer for backward (`Gemma4LayerCache`).

- [ ] **Step 112** [CODE] ~5m: LoRA forward reads CPU `normed` — download once per layer after the attn_norm step if LoRA is Some. (Small cost; keep for now.)

- [ ] **Step 113** [BUILD] ~20s: `cargo build --manifest-path crates/zhenai-forge/Cargo.toml --release`.

- [ ] **Step 114** [V]: Build green.
  - If error → [D] likely lifetime / borrow issue around `&mut scratch` through loop. Refactor to non-aliased passing.

- [ ] **Step 115** [C] **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 5 — forward_gemma4_gpu GPU-resident rewrite scaffold"
  ```

- [ ] **Step 116** [TEST] ~3m: Forward regression.
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- gemma4_gpu::tests::test_gemma4_gpu_forward_matches_cpu --nocapture 2>&1 | tail -10
  ```

- [ ] **Step 117** [V]: cosine ≥ 0.99 (looser threshold accepted because bf16 matmul accumulates differently). top-1 match is the hard gate.
  - If top-1 mismatch → [D] most likely shape bug. Add per-layer cosine instrumentation (like WAVE11 Phase 8a diagnostic).

- [ ] **Step 118** [TEST] ~3m: Training descent regression.

- [ ] **Step 119** [V]: Loss descends step 1 → 3.
  - If regression → [D] GPU-resident forward may have broken cache shape for backward. Check `attn_cache` probs download matches expected `[n_heads, seq, seq]`.

- [ ] **Step 120** [C] **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 5 — forward correctness verified on GPU-resident path"
  ```

- [ ] **Step 121** [B] ~10m: seq=384 3-step smoke.
  ```bash
  crates/zhenai-forge/target/release/zhenai-forge train-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-kingdom-train.jsonl \
    --steps 3 --lr 1e-3 --answer-start 1 \
    > /tmp/w12-phase5-timing.log 2>&1
  ```

- [ ] **Step 122** [V]: **PHASE 5 MILESTONE** — warm step ≤ 6.0 s (expect ~4.5-5.5 s after eliminating per-op round-trips from forward alone).
  ```bash
  grep "step_time" /tmp/w12-phase5-timing.log | tail -3
  ```

- [ ] **Step 123** [DECIDE]: If warm ≤ 5.0 s → target hit, jump to Phase 7.
  - **RECOMMENDATION:** likely still above 5 s because backward still uses `matmul_grad_x` CPU-in/out even though Phase 2 made the per-token ops GPU. Continue to Step 124.

- [ ] **Step 124** [CODE] ~30m: (Optional if already under target) Apply the same GPU-resident pattern to `backward_gemma4_with_lora`. Allocate a `BackwardScratch` struct. Chain: grad_logits → grad_final_hidden → per-layer grad_hidden through matmul_grad_x_gpu_in_out → rmsnorm_bwd GPU → ... All using scratch buffers.

- [ ] **Step 125** [BUILD] ~20s: Rebuild.

- [ ] **Step 126** [V]: Build green.

- [ ] **Step 127** [TEST] ~3m: Training descent regression after backward refactor.

- [ ] **Step 128** [V]: Loss descends healthy.

- [ ] **Step 129** [C] **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 5 — backward GPU-resident chain"
  ```

- [ ] **Step 130** [B] ~10m: seq=384 3-step smoke (forward+backward GPU-resident).

- [ ] **Step 131** [V]: **PHASE 5 EXIT GATE** — warm step ≤ 5.5 s.
  ```bash
  grep "step_time" /tmp/w12-phase5-timing.log | tail -3
  ```
  - If still > 5.5 s → [D] profile: (a) add `std::time::Instant` around each major chunk in forward loop, (b) rebuild, (c) run 3-step, (d) dump per-chunk ms to stderr. Top 3 chunks are the remaining bottleneck.

- [ ] **Step 132** [W] ~3m: Update session log with Phase 5 delta + per-chunk profile (if run).

- [ ] **Step 133** [C] **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit --no-gpg-sign -m "docs(forge): [PLAN W12] Phase 5 — GPU-resident forward+backward timing"
  ```

- [ ] **Step 134** [B] ~1s: Reserved.
- [ ] **Step 135** [B] ~1s: Reserved.
- [ ] **Step 136** [B] ~1s: Reserved.
- [ ] **Step 137** [B] ~1s: Reserved.
- [ ] **Step 138** [B] ~1s: Reserved.
- [ ] **Step 139** [B] ~1s: Reserved.
- [ ] **Step 140** [B] ~1s: Reserved.
- [ ] **Step 141** [B] ~1s: Reserved.
- [ ] **Step 142** [B] ~1s: Reserved.
- [ ] **Step 143** [B] ~1s: Reserved.
- [ ] **Step 144** [B] ~1s: Reserved.
- [ ] **Step 145** [B] ~1s: Reserved.
- [ ] **Step 146** [B] ~1s: Reserved.
- [ ] **Step 147** [V]: Phase 5 DoD met: warm step ≤ 5.5 s, forward cosine ≥ 0.99, training descends.

---

## PHASE 6: FULL REGRESSION + LEARNING GATE (Steps 148-162) — 1 hour

**Goal:** Confirm WAVE11 correctness invariants still hold after the GPU-resident rewrite.
**Prerequisite:** Phase 5 exit gate passed.
**Agent:** Coordinator (solo).
**Time:** 1 h wall, ~30 min GPU.

- [ ] **Step 148** [TEST] ~5m: All `hip_kernels::` unit tests.
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- hip_kernels:: 2>&1 | tail -5
  ```

- [ ] **Step 149** [V]: 21 passed (22 if `test_f32_to_bf16_matches_cpu` added).

- [ ] **Step 150** [TEST] ~5m: All `gemma4_gpu::tests::` tests.
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- gemma4_gpu::tests:: 2>&1 | tail -10
  ```

- [ ] **Step 151** [V]: All green. Record count.

- [ ] **Step 152** [TEST] ~15m: Learning Gate tests (eval.rs). Runs in parallel with forge main path.
  ```bash
  cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- eval::tests:: 2>&1 | tail -15
  ```

- [ ] **Step 153** [V]: Learning Gate 4/5 pass (Exp 2 remains diagnostic, per WAVE10F).

- [ ] **Step 154** [B] ~10m: 10-step seq=384 steady-state smoke (repeat Phase 3 measurement for comparison).
  ```bash
  crates/zhenai-forge/target/release/zhenai-forge train-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-kingdom-train.jsonl \
    --steps 10 --lr 1e-3 --answer-start 1 \
    > /tmp/w12-phase6-10step.log 2>&1
  ```

- [ ] **Step 155** [V]: **PHASE 6 EXIT GATE** — median warm step (steps 3-10) ≤ 5.5 s AND final loss ≤ 12 (matches Phase 9 Kingdom trajectory).

- [ ] **Step 156** [W] ~3m: Record full Phase 6 metrics in session log.

- [ ] **Step 157** [C] **PHASE 6 COMMIT**
  ```bash
  git add crates/zhenai-forge/notes/wave12-session-log.md && \
  git commit --no-gpg-sign -m "docs(forge): [PLAN W12] Phase 6 — full regression green, warm ≤ 5.5 s"
  ```

- [ ] **Step 158** [B] ~1s: Reserved.
- [ ] **Step 159** [B] ~1s: Reserved.
- [ ] **Step 160** [B] ~1s: Reserved.
- [ ] **Step 161** [B] ~1s: Reserved.
- [ ] **Step 162** [V]: Phase 6 done — ready for the full Kingdom RAFT.

---

## PHASE 7: 500-STEP KINGDOM RAFT + EVAL DESCENT (Steps 163-180) — 2 hours

**Goal:** Run 500 steps of LoRA fine-tuning on Kingdom corpus at seq=384. Save LoRA every 100 steps. Run held-out eval at the end. Descend below log(vocab) floor — ideally sub-10 train loss, sub-10 eval loss.
**Prerequisite:** Phase 6 exit gate passed.
**Agent:** Coordinator (solo) — long run, mostly wait + checkpoint via Monitor.
**Time:** 2 h wall, ~100 min GPU (500 × ~5 s + cold + eval = ~45 min minimum, plus buffer).

- [ ] **Step 163** [B] ~5s: Make output dir.
  ```bash
  mkdir -p raft/kingdom-w12
  ```

- [ ] **Step 164** [B] ~5s: Confirm no stale processes on GPU.
  ```bash
  pgrep -af "zhenai-forge" | grep -v grep || echo "clean"
  ```

- [ ] **Step 165** [B] [GPU:t=6000s] ~100m: Kick off 500-step RAFT.
  ```bash
  crates/zhenai-forge/target/release/zhenai-forge train-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-kingdom-train.jsonl \
    --steps 500 --lr 1e-3 --answer-start 1 \
    --rank 16 --alpha 32 \
    --save-every 100 \
    --output raft/kingdom-w12/kingdom.lora.gguf \
    > /tmp/w12-phase7-raft.log 2>&1 &
  echo "RAFT PID: $!"
  ```

- [ ] **Step 166** [B]: Arm Monitor on step progress (fires every 60s).
  ```
  Monitor tail -n +1 -f /tmp/w12-phase7-raft.log | grep --line-buffered -E "step 100/|step 200/|step 300/|step 400/|step 500/|TRAINING COMPLETE|panicked|Killed|Error"
  ```

- [ ] **Step 167** [V]: First warm step (step 2) ≤ 6 s. If not → [D] confirm no swap pressure, no zombie tests.

- [ ] **Step 168** [V]: Step 100 checkpoint saved at `raft/kingdom-w12/kingdom.lora.gguf.step100` (if save-every flag honored — otherwise adjust to single final save and re-run).

- [ ] **Step 169** [V]: Step 250 loss < step 100 loss (monotonic-ish, stochastic variance allowed).

- [ ] **Step 170** [V]: Step 500 completes; average final loss < 8.

- [ ] **Step 171** [B] ~5s: Extract training trajectory for plot.
  ```bash
  grep "step_time" /tmp/w12-phase7-raft.log | awk 'NR%10==0' | tee /tmp/w12-phase7-trajectory.txt
  ```

- [ ] **Step 172** [B] ~30m: Run held-out eval on the final LoRA.
  ```bash
  crates/zhenai-forge/target/release/zhenai-forge eval \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --lora raft/kingdom-w12/kingdom.lora.gguf \
    --data /tmp/24h-kingdom-eval.jsonl \
    > /tmp/w12-phase7-eval.log 2>&1
  ```
  - If the CLI doesn't yet support LoRA eval with a saved gguf, use the in-process `EvalHarness` via a tiny cargo-test scaffold → [STUCK] flag + note.

- [ ] **Step 173** [V]: **PHASE 7 EXIT GATE** — held-out eval loss at step 500 < eval loss at step 0 (base model). Both printed in log.

- [ ] **Step 174** [W] ~5m: Write `crates/zhenai-forge/notes/wave12-kingdom-raft-results.md` with trajectory table, eval delta, final loss, step-time histogram.

- [ ] **Step 175** [C] **PHASE 7 COMMIT**
  ```bash
  git add raft/kingdom-w12/ crates/zhenai-forge/notes/wave12-kingdom-raft-results.md && \
  git commit --no-gpg-sign -m "feat(forge): [PLAN W12] Phase 7 — 500-step Kingdom RAFT LoRA shipped, eval descent confirmed"
  ```

- [ ] **Step 176** [B] ~1s: Reserved.
- [ ] **Step 177** [B] ~1s: Reserved.
- [ ] **Step 178** [B] ~1s: Reserved.
- [ ] **Step 179** [B] ~1s: Reserved.
- [ ] **Step 180** [V]: Kingdom RAFT complete, LoRA saved, eval confirmed below base.

---

## PHASE 8: ADR-050 + SESSION LOG + MEMORY + HANDOFF (Steps 181-195) — 1 hour

**Goal:** Write the WAVE12 decision record, close the session log, update memory index, stage a clean handoff.
**Prerequisite:** Phase 7 exit gate passed.
**Agent:** Coordinator (solo).
**Time:** 1 h wall, 0 GPU.

- [ ] **Step 181** [W] ~15m: Draft `docs/adr/ADR-050-wave12-gpu-resident-activations.md` mirroring ADR-049 structure. Capture: context (W11 left us at 10.5s/step, 2.2× over target), decision (GPU-resident activation chain + backward GPU ops + mask cache), results (final warm timing, eval deltas), consequences.

- [ ] **Step 182** [W] ~3m: Update `docs/adr/ADR-INDEX.md` with ADR-050 row.

- [ ] **Step 183** [W] ~5m: Finalize `crates/zhenai-forge/notes/wave12-session-log.md` with full commit ledger + per-phase results table.

- [ ] **Step 184** [W] ~5m: Update `/home/govan/.claude/projects/-home-govan-tmp/memory/project_wave11_complete.md` → rename to `project_wave12_complete.md` (or add new memory) with: target hit, 500-step LoRA shipped, remaining tech debt (per_layer_proj_norm edge cases, matformer KV-reuse quirk).

- [ ] **Step 185** [W] ~3m: Update `MEMORY.md` index entry.

- [ ] **Step 186** [C] **COMMIT CHECKPOINT**
  ```bash
  git add docs/adr/ADR-050* docs/adr/ADR-INDEX.md crates/zhenai-forge/notes/wave12-session-log.md && \
  git commit --no-gpg-sign -m "docs(adr): [PLAN W12] ADR-050 — GPU-resident activations for Gemma-4 training"
  ```

- [ ] **Step 187** [B] ~1s: Confirm memory files outside repo committed.
  ```bash
  ls -la /home/govan/.claude/projects/-home-govan-tmp/memory/MEMORY.md
  ```

- [ ] **Step 188** [B] ~5m: Update `/home/govan/tmp/unheaded/CLAUDE.md` Age-3 status block to reference WAVE12 shipped ("Kingdom RAFT 500-step LoRA trained at seq=384, warm 5 s/step on RX 7700 XT").

- [ ] **Step 189** [C] **FINAL COMMIT**
  ```bash
  git add CLAUDE.md && \
  git commit --no-gpg-sign -m "docs(claude): [PLAN W12] Age-3 status — WAVE12 shipped, Kingdom LoRA at seq=384"
  ```

- [ ] **Step 190** [B] ~5s: Final git log sanity.
  ```bash
  git log --oneline -20
  ```

- [ ] **Step 191** [V]: WAVE12 commits form a coherent story, end with `Kingdom LoRA at seq=384`.

- [ ] **Step 192** [B] ~1s: Reserved.
- [ ] **Step 193** [B] ~1s: Reserved.
- [ ] **Step 194** [B] ~1s: Reserved.
- [ ] **Step 195** [V]: **PHASE 8 EXIT GATE / WAVE12 COMPLETE** — ADR-050 accepted, LoRA on disk, eval descent confirmed, docs current, memory updated, `git status` clean.

---

# APPENDIX A: EMERGENCY PROCEDURES

## A1. Kingdom corpus cache missing from /tmp (survived reboot)

Symptom: Step 4 verification fails — `/tmp/24h-kingdom-train.jsonl` absent.
Recovery (6-8 min):
```bash
cd /home/govan/tmp/unheaded
python3 scripts/tokenize-kingdom-for-gemma4.py \
  --model /var/zhen/models/gemma-4-E2B-it.gguf \
  --input raft/training/train.jsonl \
  --output /tmp/24h-kingdom-train.jsonl \
  --max-tokens 384
python3 scripts/tokenize-kingdom-for-gemma4.py \
  --model /var/zhen/models/gemma-4-E2B-it.gguf \
  --input raft/training/eval.jsonl \
  --output /tmp/24h-kingdom-eval.jsonl \
  --max-tokens 384
```
Verify line counts: `wc -l /tmp/24h-kingdom-*.jsonl` (expect 3568 + 397).

## A2. GPU hipMalloc failure mid-run (VRAM fragmentation)

Symptom: `hipMalloc(...)` returns non-zero after several iterations.
Recovery:
1. Confirm no zombie processes: `pgrep -af zhenai-forge | grep -v grep`.
2. Free VRAM via process kill: `pkill -9 -f zhenai-forge; sleep 5`.
3. Check `rocm-smi --showmeminfo vram` — VRAM should drop back to <1 GB.
4. If VRAM stays pinned → full GPU reset: `sudo rocm-smi -r` (requires sudo).
5. Resume from last commit.

## A3. Swap pressure stalls GPU upload (14 GB RAM under pressure)

Symptom: forge process in state D for minutes, CPU% low, GPU% 0%, RSS > 10 GB.
Recovery:
1. Kill the stalled process: `kill -9 <pid>`.
2. `free -h` — confirm swap usage dropped.
3. Avoid parallel tests with training smokes. Do not run `cargo test gemma4_gpu::tests::` while a 500-step smoke is live.

## A4. head_dim > 256 under-writes (if NEW kernels added in Phase 4)

Symptom: forward_matches_cpu cosine inexplicably low on layer 34 only.
Recovery: audit any NEW kernel with `const int d = threadIdx.x; if (d >= head_dim) return;` pattern. Replace with `for (d = threadIdx.x; d < head_dim; d += blockDim.x)`. See WAVE11 Phase 8a postmortem.

## A5. Training loss NaN after GPU-resident refactor

Symptom: Step 2 or later produces loss=NaN in log.
Recovery:
1. Check grad_health summary in forge output — which layer NaN'd first.
2. Add f32 `assert!(x.is_finite())` guards inside `grad_q`/`grad_k`/`grad_v` download sites.
3. Most likely cause: uninitialized scratch buffer read. Audit `ForwardScratch` — every field must be written before read OR zeroed at alloc.
4. Revert latest commit; bisect.

## A6. LoRA save path not implemented in CLI

Symptom: Step 175 `--output` flag ignored or errors.
Recovery: use in-process scaffold. Create `crates/zhenai-forge/tests/w12_raft.rs` with:
```rust
#[test]
fn w12_500_step_raft_with_save() { /* calls run_loop, writes LoRA via LoraAdapters::save */ }
```
Run via `cargo test --release --test w12_raft -- --nocapture`. Saves LoRA manually at the end.

## A7. Phase 5 forward cosine < 0.99

Symptom: cosine drop after GPU-resident forward.
Recovery (same playbook as Phase 8a):
1. Add per-layer diagnostic: run CPU attention_forward AND GPU attention_forward on same inputs, log cos per layer.
2. Binary-search via `WAVE12_DIAG=1` env guard to emit shape info at each suspect site.
3. Most likely bug: f32→bf16 conversion dropped precision in an un-expected place. Keep bf16 only at matmul input boundaries; all other activations stay f32.

---

# APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Name | Agent | Parallelizable | Depends on | Time |
|------:|------|-------|:--------------:|-----------|-----:|
| 0 | Preflight | Coordinator | — | clean HEAD | 30m |
| 1 | Mask cache | Coordinator | — | Phase 0 | 45m |
| 2 | Backward GPU ops | Coordinator | — | Phase 1 | 2h |
| 3 | Checkpoint | Coordinator | — | Phase 2 | 20m |
| 4 | Matmul GPU-in/out | Coordinator | — | Phase 3 (decision) | 1.5h |
| 5 | GPU-resident forward | Coordinator | — | Phase 4 | 3h |
| 6 | Full regression | Coordinator | — | Phase 5 | 1h |
| 7 | 500-step RAFT | Coordinator | — | Phase 6 | 2h |
| 8 | Docs + handoff | Coordinator | Phase 7 tail | Phase 7 | 1h |

**Critical path (linear):** 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 = ~12 hours worst case.

**Short-circuit path (if Phase 3 decides target met):** 0 → 1 → 2 → 3 → 7 → 8 = ~7 hours.

No parallelization within the plan — GPU serializes single-machine runs. Phase 8 docs can technically start during Phase 7 tail (which is mostly waiting on the GPU).

---

# APPENDIX C: QUICK REFERENCE

## C1. Gemma-4 E2B key hparams

| Key | Value |
|-----|------:|
| n_layer | 35 |
| n_embd | 1536 |
| n_ff | 6144 |
| n_head | 8 |
| n_head_kv | 1 |
| head_dim_full | 512 |
| head_dim_swa | 256 |
| rope_dim_full | 512 |
| rope_dim_swa | 256 |
| rope_freq_base_full | 1,000,000 |
| rope_freq_base_swa | 10,000 |
| sliding_window | 512 |
| vocab_size | 262,144 |
| final_logit_softcapping | 30 |
| n_embd_per_layer (PLE) | 256 |
| n_layer_kv_from_start | 20 (layers 20-34 KV-reuse) |
| rms_norm_eps | 1e-6 |

## C2. Activation shapes at seq=384 (bytes in f32)

| Tensor | Shape | Bytes |
|--------|-------|-----:|
| hidden | [384, 1536] | 2.4 MB |
| normed | [384, 1536] | 2.4 MB |
| q_out (full layer) | [384, 8, 512] | 6.3 MB |
| q_out (sliding layer) | [384, 8, 256] | 3.1 MB |
| kv_out (full) | [384, 1, 512] | 0.8 MB |
| kv_out (sliding) | [384, 1, 256] | 0.4 MB |
| scores/probs/mask | [8, 384, 384] | 4.7 MB each |
| attn_out | [384, 8, head_dim] | 3.1-6.3 MB |
| ffn_gate_pre / up_pre | [384, 6144] | 9.4 MB each |
| ffn_hidden | [384, 6144] | 9.4 MB |
| ffn_out | [384, 1536] | 2.4 MB |
| logits | [384, 262144] | 400 MB — download once at end |

## C3. CLI recipes

Smoke (3 steps):
```bash
crates/zhenai-forge/target/release/zhenai-forge train-gemma4 \
  --model /var/zhen/models/gemma-4-E2B-it.gguf \
  --data /tmp/24h-kingdom-train.jsonl \
  --steps 3 --lr 1e-3 --answer-start 1
```

Full RAFT:
```bash
crates/zhenai-forge/target/release/zhenai-forge train-gemma4 \
  --model /var/zhen/models/gemma-4-E2B-it.gguf \
  --data /tmp/24h-kingdom-train.jsonl \
  --steps 500 --lr 1e-3 --answer-start 1 \
  --rank 16 --alpha 32 --save-every 100 \
  --output raft/kingdom-w12/kingdom.lora.gguf
```

Eval:
```bash
crates/zhenai-forge/target/release/zhenai-forge eval \
  --model /var/zhen/models/gemma-4-E2B-it.gguf \
  --lora raft/kingdom-w12/kingdom.lora.gguf \
  --data /tmp/24h-kingdom-eval.jsonl
```

Regression set:
```bash
cargo test --manifest-path crates/zhenai-forge/Cargo.toml --release --tests -- \
  hip_kernels:: gemma4_gpu::tests:: eval::tests:: 2>&1 | tail -25
```

## C4. Monitor pattern for long runs

```
Monitor: tail -n +1 -f /tmp/w12-phase7-raft.log \
  | grep --line-buffered -E "step (50|100|150|200|250|300|350|400|450|500)/|TRAINING COMPLETE|panicked|Killed|Error|NaN"
```

## C5. Commit message template

```
<type>(forge): [PLAN W12] Phase N[letter] — <one-line what>

<2-4 lines of why / what changed>

<regression note — tests green, timing number>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```

Types: `feat`, `docs`, `test`, `refactor`, `perf`.

---

# APPENDIX D: DEFINITION OF DONE (Micromanager gate)

WAVE12 is DONE when ALL of these are true:

- [ ] Warm step time at seq=384 ≤ 5.5 s (measured over 10-step steady-state)
- [ ] 500-step Kingdom RAFT completes without NaN, without crash
- [ ] Final training loss < 8 at step 500
- [ ] Held-out eval loss at step 500 < eval loss at step 0 (base model)
- [ ] LoRA saved to disk at `raft/kingdom-w12/kingdom.lora.gguf`
- [ ] `test_gemma4_gpu_forward_matches_cpu` passes (cosine ≥ 0.99, top-1 match)
- [ ] `test_gemma4_gpu_train_step_loss_descent` passes (monotonic descent)
- [ ] All 21+ WAVE11 kernel tests still pass
- [ ] Learning Gate 4/5 still pass (Exp 2 diagnostic stable)
- [ ] ADR-050 accepted + indexed
- [ ] Session log complete
- [ ] No new crate dependencies added (ADR-004 upheld)
- [ ] No new `#[cfg(test)]`-only shortcuts in production paths
- [ ] Attack surface unchanged (no new FFI surface; Phase 4 f32_to_bf16 kernel is internal to hip_kernels)
- [ ] CLAUDE.md Age-3 block updated
- [ ] `git status` clean, all work committed

---

*WAVE12 Battle Plan — Forged 2026-04-22*
*9 Phases. 195 Steps. The forge learns the Kingdom at full sequence length.*
*Close the gap. Ship the LoRA. The activations stay on the GPU.*
