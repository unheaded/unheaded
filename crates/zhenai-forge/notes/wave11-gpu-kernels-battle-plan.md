# WAVE11 GpuKernelsBackend Battle Plan — 11 Phases, ~300 Steps

**Date forged:** 2026-04-21
**Sprint:** WAVE11 — move attention / softmax / RoPE / RMSNorm / GELU off single-threaded CPU onto RX 7700 XT (gfx1101, ROCm 6.4) so zhenai-forge can train Gemma 4 E2B at seq=384 in real time.
**Prerequisite:** ADR-048 landed (`ForgeBackend` trait at `83e1b472`); `HybridMatmulBackend` operational; Learning Gate + regression audit green; Kingdom corpus at `/tmp/24h-kingdom-{train,eval}.jsonl`.
**Target:** `zhenai-forge train-gemma4 --data /tmp/24h-kingdom-train.jsonl` trains at seq=384 ≤ 5 s/step warm with eval descent confirmed on held-out Kingdom corpus.
**Estimated Duration:** 3 weeks across ~20 focused sessions (+1-2 days Phase 1 diagnostic).
**Agent Strategy:** solo/coordinator. GPU serializes all runs; kernel work is write-compile-test-debug loops with frequent iteration. No parallelization across phases.
**Commit Cadence:** every 5 steps + phase-exit commit mandatory (`commit_interval = max(3, min(5, 300/20)) = 5`).
**Stuck Protocol:** skip after 3× estimated time or 2 failed debug attempts; commit known-good state before skipping; log STUCK marker with symptom + attempts + needs.

---

## LEGEND

```
[B]        bash command                  [V]  verification gate (must pass)
[D]        debug branch (on V fail)      [W]  write file
[R]        read file                     [C]  commit checkpoint
[CODE]     in-source edit                [BUILD] cargo / hipcc build
[TEST]     cargo test invocation         [DECIDE] autonomous choice
[GPU:t=X]  explicit GPU time budget      [ESCALATE] STOP for human
[STUCK]    skipped per Skip Protocol     [BLOCKED] blocked by upstream STUCK
[ABORT-IF] per-phase kill criterion      [DESIGN] design decision (w/ rationale)
```

`[STUCK]` is only for Skip Protocol skips. `[BLOCKED]` only marks downstream consequences. `[DECIDE]` is autonomous (agent picks recommendation). `[ESCALATE]` is rare — actually stop.

---

## CRITICAL FACTS (cached for the executing agent)

- **Repo:** `/home/govan/tmp/unheaded/` (branch `main`)
- **Crate:** `crates/zhenai-forge/`
- **GGUF:** `/var/zhen/models/gemma-4-E2B-it.gguf` (8.7 GB)
- **Kingdom corpus (tokenized):** `/tmp/24h-kingdom-{train,eval}.jsonl` (3568 + 397 seqs, MAX_TOKENS=384)
- **Tokenizer script:** `scripts/tokenize-kingdom-for-gemma4.py` (+ Python venv `/home/govan/tmp/gemma4-venv`)
- **ROCm:** 6.4.2 (`hipcc --version`: HIP 6.4.43484). LLVM 19, clang from `roc-6.4.2`.
- **GPU:** AMD Radeon RX 7700 XT (Navi 32, gfx1101, RDNA 3), 12.87 GB VRAM.
- **ForgeBackend trait:** `src/backend.rs`. Plug-in point: `pub struct GpuKernelsBackend { ... }`, `impl ForgeBackend for GpuKernelsBackend { type Handle = GpuKernelsHandle; ... }`.
- **Existing HIP glue:** `src/hip.rs` (555 LOC) — `GpuBuffer`, `BlasHandle`, `sgemm_bf16_ex`, memcpy helpers. We EXTEND it with a kernel-launch FFI (not replace).
- **CPU references (the correctness oracle):**
  - `src/forward.rs`: `rmsnorm`, `softmax`, `gelu_tanh_approx`, `cross_entropy_loss`, `embedding_lookup`.
  - `src/backward.rs`: `rmsnorm_backward`, softmax Jacobian helpers, `cross_entropy_softmax_backward`.
  - `src/gemma4.rs`: `forward_gemma4_with_lora`, `backward_gemma4_with_lora`, per-head norm backward, `gelu_tanh_approx_prime`, `rope_backward_partial`.
- **License:** crate is GPL-3.0. All new HIP/C++ source headers get `SPDX-License-Identifier: GPL-3.0-or-later`.
- **Regression baseline (must stay bit-identical at every commit):**
  - `test_gemma4_backward_grad_health`: healthy=35/35 zero=0 nan=0.
  - `test_gemma4_gpu_train_step_loss_descent`: 2.01 s/step warm, trajectory 19.9595 → 15.9253 → 5.7022.
  - `test_learning_exp1_held_out_eval`: ratio 0.5806 CI (0.567, 0.596).
  - `test_learning_exp3_lora_zero_identity`: lora-zero-A eval 21.369324 ≡ base.
  - `test_learning_exp4_dataset_size_scaling`: eval(|T|=64)/eval(|T|=8) = 0.63.
  - `test_learning_exp5_generalization_gap_beta`: β=0.266 CI (−0.112, 0.644).

---

## TIME BUDGET SUMMARY

| Phase | Name | Wall-clock | GPU time | Cumulative |
|------:|------|-----------:|---------:|-----------:|
| 0 | Preflight + env | 1 h | 5 min | 1 h |
| 1 | seq=64 RAFT diagnostic | 8 h | ~4 h | 9 h |
| 2 | HIP FFI scaffolding + first kernel | 1 day | 30 min | 2 d |
| 3 | RMSNorm kernel (fwd+bwd) | 2 days | 2 h | 4 d |
| 4 | GELU kernel (fwd+bwd) | 0.5 day | 30 min | 4.5 d |
| 5 | Softmax kernel (fwd+bwd) | 2 days | 2 h | 6.5 d |
| 6 | RoPE partial-rotary kernel (fwd+bwd) | 1 day | 1 h | 7.5 d |
| 7 | Attention kernel (fwd+bwd, fused) | 4 days | 4 h | 11.5 d |
| 8 | GpuKernelsBackend integration + residency | 2 days | 3 h | 13.5 d |
| 9 | Regression + Phase 5 RAFT retry at seq=384 | 1 day | 3 h | 14.5 d |
| 10 | Docs + ADR + session handoff | 0.5 day | 0 | 15 d |

Total ~3 weeks including Phase 1 diagnostic day. Critical path is Phases 2→3→5→7→8→9 (kernel pipeline). Phases 4 and 6 (GELU, RoPE) are parallelizable AFTER the FFI is proven (Phase 2), but with solo/coordinator they serialize in practice.

---

## PHASE 0: PREFLIGHT + ENVIRONMENT (Steps 1-18) — 1 h

**Goal:** Lock the toolchain state, baseline all tests, confirm HIP compiler works, fail fast if anything's broken BEFORE we write kernel code.
**Prerequisite:** Fresh session; branch `main` at `83e1b472` (ADR-048 landed) or descendant; working tree clean.
**ABORT-IF:** HIP compiler missing OR any pre-session-green test fails baseline OR VRAM <9 GB free → STOP, escalate.

- [ ] **Step 1** [B] ~5s: `cd ~/tmp/unheaded && git status && git log --oneline -5`
- [ ] **Step 2** [V]: HEAD is `83e1b472` (or descendant); working tree clean.
- [ ] **Step 3** [B] ~2s: `hipcc --version && rocm-smi | head -20`
- [ ] **Step 4** [V]: HIP 6.4+ available; one Navi 32 card present; VRAM mostly free.
- [ ] **Step 5** [B] ~2s: check hipBLAS + HIP runtime libs are discoverable:
  ```bash
  for lib in libhipblas.so libamdhip64.so; do
    ldconfig -p | grep "$lib" | head -1 || echo "MISSING: $lib"
  done
  ```
- [ ] **Step 6** [V]: both libraries present. If MISSING → `apt-get install rocm-dev` style install or escalate.
- [ ] **Step 7** [B] ~2s: find HIP include dir for kernel compilation later:
  ```bash
  for d in /opt/rocm/include/hip /usr/include/hip; do
    test -d "$d" && echo "HIP_INC=$d" && break
  done
  ```
- [ ] **Step 8** [W]: record findings into `/tmp/wave11-env.txt` — ROCm version, gfx target (gfx1101), HIP include, LIB paths.
- [ ] **Step 9** [B] ~1m: build the crate clean.
  ```bash
  cd ~/tmp/unheaded/crates/zhenai-forge && cargo clean && cargo build --release --tests 2>&1 | tail -3
  ```
- [ ] **Step 10** [V]: build green.
- [ ] **Step 11** [TEST] [GPU:t=3min]: baseline grad-health.
  ```bash
  cargo test --release --bin zhenai-forge test_gemma4_backward_grad_health -- --test-threads=1 2>&1 | tail -5
  ```
- [ ] **Step 12** [V]: test result `ok. 1 passed; 0 failed`. Else → [D] inspect, rebuild, re-run.
- [ ] **Step 13** [TEST] [GPU:t=3min]: baseline loss-descent.
  ```bash
  cargo test --release --bin zhenai-forge test_gemma4_gpu_train_step_loss_descent -- --test-threads=1 --nocapture 2>&1 | grep -E "loss=|test result" | head -5
  ```
- [ ] **Step 14** [V]: warm step ≤ 2.5 s; trajectory starts 19.9595 (bit-identical). Else → [D] inspect.
- [ ] **Step 15** [B]: confirm tokenized Kingdom corpus still present.
  ```bash
  wc -l /tmp/24h-kingdom-{train,eval}.jsonl
  ```
- [ ] **Step 16** [D, conditional]: if tokenized Kingdom missing (lost across reboot), regenerate:
  ```bash
  /home/govan/tmp/gemma4-venv/bin/python \
    scripts/tokenize-kingdom-for-gemma4.py < raft/training/train.jsonl > /tmp/24h-kingdom-train.jsonl
  /home/govan/tmp/gemma4-venv/bin/python \
    scripts/tokenize-kingdom-for-gemma4.py < raft/training/eval.jsonl  > /tmp/24h-kingdom-eval.jsonl
  ```
- [ ] **Step 17** [V]: `wc -l` shows 3568 train + 397 eval.
- [ ] **Step 18** [V]: **PHASE 0 EXIT GATE** — toolchain verified, baselines bit-identical, corpus present. Commit nothing (no source touched).

---

## PHASE 1: seq=64 RAFT DIAGNOSTIC (Steps 19-55) — 8 h

**Goal:** Diagnose whether attention at seq=384 IS the bottleneck (hypothesis from 24h session) or whether something else is hiding. If seq=64 trains fast, attention is confirmed and Phase 2+ is the right plan. If seq=64 is ALSO slow, abort and investigate before kernels.
**Prerequisite:** Phase 0 green.
**ABORT-IF:** seq=64 training step takes ≥10 s warm → attention is NOT the sole bottleneck → halt, escalate, do NOT start Phase 2.

### Corpus regeneration at seq=64

- [ ] **Step 19** [CODE]: in `scripts/tokenize-kingdom-for-gemma4.py`, bump `MAX_TOKENS` from 384 to 64. (One-line constant change.)
- [ ] **Step 20** [B] ~3m: regenerate tokenized corpus:
  ```bash
  /home/govan/tmp/gemma4-venv/bin/python \
    scripts/tokenize-kingdom-for-gemma4.py < raft/training/train.jsonl > /tmp/wave11-kingdom-64-train.jsonl 2>/tmp/wave11-tok64-train.err
  /home/govan/tmp/gemma4-venv/bin/python \
    scripts/tokenize-kingdom-for-gemma4.py < raft/training/eval.jsonl  > /tmp/wave11-kingdom-64-eval.jsonl  2>/tmp/wave11-tok64-eval.err
  ```
- [ ] **Step 21** [V]: `wc -l` shows 3568 train + 397 eval (non-zero; we expect more skipped-too-short with MAX_TOKENS=64 but not a catastrophic drop).
- [ ] **Step 22** [B]: inspect distribution:
  ```bash
  python3 -c "
  import json
  lens=[len(json.loads(l)['tokens']) for l in open('/tmp/wave11-kingdom-64-train.jsonl')]
  ans=[len(json.loads(l)['tokens'])-json.loads(l)['answer_start'] for l in open('/tmp/wave11-kingdom-64-train.jsonl')]
  lens.sort(); ans.sort()
  print(f'seq p50={lens[len(lens)//2]} p95={lens[int(len(lens)*0.95)]} max={max(lens)}')
  print(f'ans p50={ans[len(ans)//2]} p95={ans[int(len(ans)*0.95)]} max={max(ans)}')
  "
  ```
- [ ] **Step 23** [V]: max seq_len ≤ 64; median answer length ≥ 8 tokens (enough signal). If median <8, raise MAX_TOKENS to 96 and redo. `[DECIDE]` — proceed with ≥8-median corpus.
- [ ] **Step 24** [CODE]: revert `MAX_TOKENS` in the script back to 384 (keep it default for other callers); the seq=64 corpora are already generated to their `/tmp/wave11-kingdom-64-*` paths.
- [ ] **Step 25** [C]: commit the corpus regeneration notes (no source changes beyond the tokenizer hack reverted):
  ```bash
  git add -A && git commit --no-gpg-sign -m "[PLAN W11] Step 19-24: seq=64 Kingdom corpus regenerated for Phase 1 diagnostic"
  ```

### Smoke-run config + launch

- [ ] **Step 26** [W]: scratch config scratch at `/tmp/wave11-phase1-config.md` — record lr, steps, expected duration.
- [ ] **Step 27** [DECIDE]: lr choice. **RECOMMENDATION**: `lr=1e-3` per the Phase 2 lr-sweep finding (stable at 100 steps). Override only if Phase 1 produces NaN at 1e-3 (unlikely at seq=64).
- [ ] **Step 28** [DECIDE]: steps. **RECOMMENDATION**: 200 steps. Gives ~400s budget at 2 s/step target, 1000s at 5 s/step worst-case tolerable.
- [ ] **Step 29** [B] [GPU:t=15-40min]: run the smoke via CLI (NOT a test) to also exercise main.rs path:
  ```bash
  ~/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge \
    train-gemma4 --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/wave11-kingdom-64-train.jsonl \
    --steps 200 --lr 1e-3 \
    --output /tmp/wave11-phase1-lora.zlg4 \
    2>&1 | tee /tmp/wave11-phase1.log | tail -60
  ```
- [ ] **Step 30** [V]: log ends with TRAINING COMPLETE. If process hung >40 min → kill, trigger ABORT-IF of Phase 1.
- [ ] **Step 31** [V]: capture numbers.
  ```bash
  grep -E "step_time=" /tmp/wave11-phase1.log | tail -20
  ```
- [ ] **Step 32** [DECIDE]: is attention the bottleneck?
  - Warm step ≤ 5 s → attention CONFIRMED as bottleneck at seq=384. Proceed to Phase 2 with confidence.
  - 5 s < warm ≤ 10 s → attention is DOMINANT but other overhead exists. Still proceed to Phase 2; flag that GpuKernelsBackend won't deliver 1:1 seq=384 / seq=64 speedup.
  - Warm > 10 s → attention is NOT solely responsible. **ABORT PHASE 1**. Do NOT build kernels yet; need root-cause investigation of the 22× slowdown seen in Exp 6.

### Eval descent on Kingdom held-out

- [ ] **Step 33** [CODE]: in `src/eval.rs` tests, add `test_raft_kingdom_smoke_seq64` [ignore] that loads `/tmp/wave11-kingdom-64-{train,eval}.jsonl` and runs a 100-step `run_until_plateau` at lr=1e-3, plateau_window=10, plateau_eps=0.05.
- [ ] **Step 34** [BUILD] + [V]: build green.
- [ ] **Step 35** [TEST] [GPU:t=15-30min]: run the test.
- [ ] **Step 36** [V]: eval descent ≥ 3% (loose — seq=64 on Kingdom is a harder corpus than synthetic Y). Below 3% → something's wrong; log to `/tmp/wave11-phase1-outcome.md` and `[ESCALATE]`.

### Decision + commit

- [ ] **Step 37** [W]: `crates/zhenai-forge/notes/wave11-phase1-diagnostic.md` documenting:
  - seq=64 corpus stats.
  - Observed warm step-time.
  - Observed eval descent.
  - Decision: proceed to kernels vs. root-cause-first.
- [ ] **Step 38** [C]: commit `[PLAN W11] Phase 1 DIAGNOSTIC outcome — [attention-bound | orthogonal]`.
- [ ] **Step 39** [V]: **PHASE 1 EXIT GATE** — diagnostic decision committed; proceed to Phase 2 only if attention-bound. Else STOP and change plan.

**Steps 40-55 reserved** for Phase 1 debug: if NaN at lr=1e-3, retry at 3e-4; if smoke ran but eval flat, inspect logits / per-layer gradient norms; if >10 s/step, start a forensics effort (profiling, `rocprof` run, CPU perf record) but DO NOT start Phase 2.

---

## PHASE 2: HIP FFI SCAFFOLDING + FIRST KERNEL (Steps 56-100) — 1 day

**Goal:** Build-system can compile a `.hip.cpp` source into device code, load it at runtime, launch a trivial kernel from Rust, and get the right answer. This proves the pipeline BEFORE we write anything complex.
**Prerequisite:** Phase 1 concluded "attention-bound" OR Stevie approves proceeding anyway.
**ABORT-IF:** `hipcc` can't produce an `.so` we can `dlopen` OR kernel launches fail with persistent "invalid device function" despite correct gfx target → Phase 2 STUCK, escalate to Architect.

### Design decision (toolchain)

- [ ] **Step 56** [DECIDE]: kernel toolchain. **RECOMMENDATION**: hand-written HIP C++ compiled to a `.so` via `hipcc --shared -fPIC`, loaded via `libloading` at crate startup, launched via `hipModuleLaunchKernel` through our existing `hip.rs` FFI pattern. Rationale:
  - Zero new crate deps beyond what ROCm ships.
  - Fully debuggable with `hipprof`, `gdb`, `amdgpu-ps`.
  - Matches the existing hip.rs pattern of "FFI to the C runtime"; no Rust→device-IR JIT magic.
  - rocWMMA and Composable Kernels are EXCLUDED for this sprint — each is a large new dep tree and their API fits tile-based matmul specifically (not soft-max, not attention fusion).
  - Triton-on-ROCm is EXCLUDED — still experimental on gfx1101 as of 2026-04; would need Python at runtime.
  Override only if (a) `hipcc --shared` produces an `.so` we can't dlopen, OR (b) the Rust team (Stevie) overrides.

### File layout

- [ ] **Step 57** [DESIGN]: file layout.
  - `crates/zhenai-forge/src/hip_kernels/` — new subdir for the Rust wrappers.
  - `crates/zhenai-forge/src/hip_kernels/mod.rs` — module entry, types, error mapping.
  - `crates/zhenai-forge/src/hip_kernels/loader.rs` — `KernelModule` struct (dlopen + symbol table).
  - `crates/zhenai-forge/src/hip_kernels/<op>.rs` — one file per kernel family (rmsnorm, gelu, softmax, rope, attention).
  - `crates/zhenai-forge/kernels/` — the C++ HIP source.
  - `crates/zhenai-forge/kernels/common.hip.hpp` — shared helpers (block reduce, bf16 conversion, index helpers).
  - `crates/zhenai-forge/kernels/<op>.hip.cpp` — one file per kernel family.
  - `crates/zhenai-forge/build.rs` — invoke hipcc to produce `target/wave11-kernels.so`.

### Build script

- [ ] **Step 58** [W]: create `crates/zhenai-forge/build.rs` that:
  1. Locates `hipcc` via `which` or `ROCM_PATH/bin/hipcc`.
  2. Runs `hipcc --offload-arch=gfx1101 --shared -fPIC -O3 kernels/*.hip.cpp -o $OUT_DIR/wave11-kernels.so`.
  3. Emits `cargo:rustc-env=WAVE11_KERNELS_SO=$OUT_DIR/wave11-kernels.so` so runtime can find the lib.
  4. Emits `cargo:rerun-if-changed=kernels/` so rebuild triggers on any kernel source change.
- [ ] **Step 59** [W]: placeholder `kernels/common.hip.hpp` with bf16 helpers + `__device__ float block_reduce_sum(float x)`.
- [ ] **Step 60** [W]: first kernel `kernels/identity.hip.cpp` — one trivial kernel `identity_f32(float* out, const float* in, int n)` that copies in→out. This is a smoke test for the whole pipeline.
- [ ] **Step 61** [CODE]: add `hip_kernels::loader::KernelModule` — uses `libloading::Library` to dlopen the `.so` and resolve a function pointer by name. Errors map to `String` to match existing hip.rs style.
- [ ] **Step 62** [CODE]: add `hip_kernels::identity` module that wraps the launch: takes `GpuBuffer` in + out + length, performs `hipModuleLaunchKernel` or direct function-pointer call.
- [ ] **Step 63** [CODE]: in `src/main.rs` add `mod hip_kernels;`.
- [ ] **Step 64** [BUILD] ~1m: `cargo build --release --tests 2>&1 | tail -5`
- [ ] **Step 65** [V]: build green. `.so` present at `target/release/build/.../wave11-kernels.so`. Else → [D] inspect hipcc stderr.
- [ ] **Step 66** [C]: commit `[PLAN W11] Steps 56-65: HIP build system + identity kernel scaffolding`.

### First launch — the hello-world

- [ ] **Step 67** [CODE]: in `src/hip_kernels/identity.rs` add a `#[test]` that creates two GpuBuffers, writes `[1.0, 2.0, 3.0, 4.0]` into the input, launches `identity_f32`, reads output, asserts equality.
- [ ] **Step 68** [TEST] [GPU:t=5s]: run it.
  ```bash
  cargo test --release --bin zhenai-forge hip_kernels::identity -- --test-threads=1 --nocapture 2>&1 | tail -10
  ```
- [ ] **Step 69** [V]: **FIRST KERNEL HEARTBEAT** — `[1,2,3,4] → [1,2,3,4]` on GPU. Else → [D]:
  - `[D]` **Step 69a**: is the `.so` load failing? Print `dlerror`.
  - `[D]` **Step 69b**: is the kernel name mangled? `nm wave11-kernels.so | grep identity`.
  - `[D]` **Step 69c**: is the grid/block launch config valid for gfx1101? Launch `<<<1, 32>>>` as safest minimum.
  - After 2 failed attempts → [STUCK], escalate.

### Stability probes

- [ ] **Step 70** [CODE]: extend the identity test to 1 M elements — catch any latent allocation / launch-config bug.
- [ ] **Step 71** [TEST] [GPU:t=5s]: 1 M elements smoke.
- [ ] **Step 72** [V]: outputs match inputs at every position.
- [ ] **Step 73** [C]: commit `[PLAN W11] Steps 67-72: identity kernel launches, first GPU-compute heartbeat`.

### FFI error handling + timing helpers

- [ ] **Step 74** [CODE]: in `src/hip_kernels/mod.rs`, add `launch_sync_and_time<F: FnOnce()>(f: F) -> Duration` that wraps a kernel launch in `hipStreamSynchronize` + `std::time::Instant::now` for timing-sensitive steps later.
- [ ] **Step 75** [CODE]: add a per-kernel error enum or at minimum standardize "kernel launch failure" → `Err(format!("wave11 kernel {name} launch: hip={:?}", hip_err))`.
- [ ] **Step 76** [BUILD] + [TEST]: rebuild + rerun identity test.
- [ ] **Step 77** [V]: still green. Timing is sub-millisecond.
- [ ] **Step 78** [C]: commit `[PLAN W11] Steps 74-77: kernel timing helpers + error handling`.

- [ ] **Step 79** [V]: **PHASE 2 EXIT GATE** — (a) build.rs produces `.so`, (b) Rust can dlopen + launch a kernel, (c) identity test green at 1M elements, (d) timing helper works. All four checked.

**Steps 80-100 reserved** for debug: amdgpu-pro vs amdgpu-dkms divergences, `HIP_VISIBLE_DEVICES` issues, `ROCR_VISIBLE_DEVICES`, gfx arch mismatches, Rust-ABI quirks with `__global__` symbol linkage.

---

## PHASE 3: RMSNORM KERNEL (Steps 101-140) — 2 days

**Goal:** Ship fully correct forward + backward RMSNorm GPU kernels, plugged in as option in `forward_gemma4_gpu` / `backward_gemma4_with_lora`. RMSNorm is everywhere (attn_norm, post_attention_norm, ffn_norm, post_ffw_norm, output_norm) and is the simplest non-elementwise op, so it validates the reduce pattern before softmax needs it.
**Prerequisite:** Phase 2 green.
**ABORT-IF:** cosine < 0.999 on real weights even after kernel debug → Phase 3 STUCK, revert, investigate numerical stability (Welford reduction? fp32 accumulator?).

### Forward kernel

- [ ] **Step 101** [R]: re-read CPU reference `forward::rmsnorm` — exact algorithm (Gemma 4 uses 1+weight formulation: `out[d] = (1 + w[d]) * x[d] * rsqrt(mean(x²) + eps)`).
- [ ] **Step 102** [W]: `kernels/rmsnorm.hip.cpp` with `rmsnorm_fwd_f32(out, in, weight, eps, D)` — one block per row, thread-per-element, shared-memory reduction for `sum(x*x)`.
- [ ] **Step 103** [W]: Rust wrapper `hip_kernels::rmsnorm::rmsnorm_fwd(out_buf, in_buf, weight_buf, eps, seq, n_embd)`.
- [ ] **Step 104** [BUILD]: rebuild (kernel source change → build.rs reruns).
- [ ] **Step 105** [CODE]: add `#[test]` `test_rmsnorm_fwd_matches_cpu` — load real weights, 4-token random input, compute CPU + GPU, cosine ≥ 0.9999.
- [ ] **Step 106** [C]: commit step 101-105 scaffolding.
- [ ] **Step 107** [TEST] [GPU:t=30s]: run cosine test.
- [ ] **Step 108** [V]: cosine ≥ 0.9999. Else → [D]:
  - **Step 108a**: are we using the `1+w` formulation? Gemma 4 diff.
  - **Step 108b**: is `eps` applied before `rsqrt`?
  - **Step 108c**: is the reduction fp32 even though input is f32 (not bf16)? Accumulator precision matters.
  - After 2 fails → [STUCK].

### Backward kernel

- [ ] **Step 109** [R]: re-read CPU reference `backward::rmsnorm_backward`. The gradient formula is:
  ```
  grad_x[d] = rsqrt * (1 + w[d]) * grad_out[d]
            - x[d] * rsqrt^3 / D * sum_k (1 + w[k]) * grad_out[k] * x[k]
  ```
  (with D = n_embd).
- [ ] **Step 110** [W]: `kernels/rmsnorm.hip.cpp` append `rmsnorm_bwd_f32(grad_x, grad_out, in, weight, eps, D)` — same block-per-row + reduction pattern.
- [ ] **Step 111** [W]: Rust wrapper `hip_kernels::rmsnorm::rmsnorm_bwd`.
- [ ] **Step 112** [CODE]: `#[test]` `test_rmsnorm_bwd_matches_cpu`.
- [ ] **Step 113** [TEST] [GPU:t=30s].
- [ ] **Step 114** [V]: cosine ≥ 0.9999.
- [ ] **Step 115** [C]: commit.

### Per-head RMSNorm variant

Gemma 4's attention uses per-head Q/K-norm with a *weightless V-norm*. The kernel differs in sharding (per-head, not per-row) and must support weight-free path.

- [ ] **Step 116** [R]: CPU reference `gemma4::per_head_rmsnorm` + `per_head_rmsnorm_backward` + `v_norm_backward`.
- [ ] **Step 117** [W]: `kernels/rmsnorm.hip.cpp` append `rmsnorm_per_head_fwd`, `rmsnorm_per_head_bwd`, `v_norm_per_head_bwd`.
- [ ] **Step 118** [W]: Rust wrappers.
- [ ] **Step 119** [CODE]: 3 cosine tests (per-head fwd, per-head bwd, weightless V-norm bwd).
- [ ] **Step 120** [TEST] [GPU:t=1min] all three.
- [ ] **Step 121** [V]: all cosines ≥ 0.9999.
- [ ] **Step 122** [C]: commit.

### Performance baseline

- [ ] **Step 123** [CODE]: `#[test]` `test_rmsnorm_fwd_speedup` — time 100 launches of a 384-row RMSNorm on GPU vs 100 on CPU. Log ratio.
- [ ] **Step 124** [TEST] [GPU:t=1min].
- [ ] **Step 125** [V]: GPU ≥ 5× faster than CPU for 384 × 1536.

- [ ] **Step 126** [V]: **PHASE 3 EXIT GATE** — 5 RMSNorm kernels (fwd, bwd, per_head_fwd, per_head_bwd, v_norm_bwd) all ship with cosine ≥ 0.9999 + performance beats CPU. Integration into `GpuKernelsBackend` happens in Phase 8.

**Steps 127-140 reserved** for numerical stability debug (Welford, Kahan) and shared-memory bank-conflict optimization (not critical for correctness).

---

## PHASE 4: GELU KERNEL (Steps 141-160) — 0.5 day

**Goal:** Ship GELU-tanh-approx forward + backward kernels. Trivial elementwise; mostly exists to validate the pattern and tick a checkbox.
**Prerequisite:** Phase 3 green.
**ABORT-IF:** cosine < 0.9999 despite known-correct formula → weird compiler bug, escalate.

- [ ] **Step 141** [R]: CPU reference `forward::gelu_tanh_approx` + `gemma4::gelu_tanh_approx_prime`.
- [ ] **Step 142** [W]: `kernels/gelu.hip.cpp` — `gelu_tanh_fwd`, `gelu_tanh_bwd`. Elementwise, one thread per scalar.
- [ ] **Step 143** [W]: Rust wrappers `hip_kernels::gelu::{fwd, bwd}`.
- [ ] **Step 144** [BUILD] + [TEST] cosine tests [GPU:t=30s].
- [ ] **Step 145** [V]: cosine ≥ 0.9999 fwd + bwd.
- [ ] **Step 146** [C]: commit.

### GELU with multiply (fused)

Gemma 4's FFN forward is `gelu(gate_pre) * up_pre`. Fuse the multiply into the GELU kernel — saves one kernel launch + one memory pass per FFN per layer.

- [ ] **Step 147** [W]: `kernels/gelu.hip.cpp` append `gelu_tanh_mul_fwd(out, gate_pre, up_pre, n)` and `gelu_tanh_mul_bwd(grad_gate_pre, grad_up_pre, grad_out, gate_pre, up_pre, n)`.
- [ ] **Step 148** [W]: Rust wrappers.
- [ ] **Step 149** [CODE] + [TEST] cosine tests.
- [ ] **Step 150** [V]: cosine ≥ 0.9999 for fused fwd + bwd.
- [ ] **Step 151** [C]: commit.

- [ ] **Step 152** [V]: **PHASE 4 EXIT GATE** — 4 GELU kernels (plain fwd, plain bwd, fused-mul fwd, fused-mul bwd) all correct.

**Steps 153-160 reserved** for optimization or removal of the plain variants if fused is always used.

---

## PHASE 5: SOFTMAX KERNEL (Steps 161-200) — 2 days

**Goal:** Ship numerically-stable softmax fwd + bwd kernels. Softmax is the core of attention — correctness here is load-bearing.
**Prerequisite:** Phase 3 green (same reduce pattern).
**ABORT-IF:** cosine < 0.999 on real logits after exp range normalization attempts → stability issue; escalate to Scientist.

### Forward — stable softmax

- [ ] **Step 161** [R]: CPU reference `forward::softmax` (max-sub + exp + normalize).
- [ ] **Step 162** [DESIGN]: for attention, we'll softmax over `[seq, seq]` scores per head per layer. Shape is `[n_head, seq, seq]` total. Kernel: one block per `(head, row)`, threads reduce across `seq` dimension.
- [ ] **Step 163** [W]: `kernels/softmax.hip.cpp` — `softmax_fwd_f32(probs, scores, rows, cols)`:
  1. First pass: each block reduces `max(scores[row])` into shared mem.
  2. Barrier.
  3. Second pass: compute `exp(scores[row][col] - max)` and accumulate `sum` into shared mem.
  4. Barrier.
  5. Third pass: `probs[row][col] = exp(...) / sum`.
- [ ] **Step 164** [W]: Rust wrapper.
- [ ] **Step 165** [CODE]: `test_softmax_fwd_matches_cpu` — real attention-shaped scores, cosine ≥ 0.9999.
- [ ] **Step 166** [BUILD] + [TEST] [GPU:t=30s].
- [ ] **Step 167** [V]: cosine ≥ 0.9999. Else → [D]:
  - `[D]` 167a: max-sub applied before exp?
  - `[D]` 167b: fp32 accumulator for sum (even if input is f16/bf16 later)?
  - `[D]` 167c: division by zero edge case (all -inf column)?
- [ ] **Step 168** [C]: commit.

### Forward with mask

Gemma 4 uses causal + sliding-window masks. Kernel applies mask before softmax (add -inf to masked positions).

- [ ] **Step 169** [W]: extended kernel `softmax_fwd_masked(probs, scores, mask_bits, rows, cols)` where `mask_bits` is a per-`(row, col)` bitmask (1 = keep, 0 = -inf).
- [ ] **Step 170** [W]: Rust wrapper.
- [ ] **Step 171** [CODE] + [TEST]: test with causal mask, cosine ≥ 0.9999.
- [ ] **Step 172** [V]: cosine + check mask actually masked (probs at masked positions = 0).
- [ ] **Step 173** [C]: commit.

### Backward — softmax Jacobian

Softmax-backward formula: `grad_scores[i] = probs[i] * (grad_probs[i] - sum_k(probs[k] * grad_probs[k]))`.

- [ ] **Step 174** [R]: CPU reference exists in `backward::` — find it or derive. Actually the fused path in Gemma 4 attention backward sidesteps this (uses `softmax_jvp` or similar). Re-derive from the formula above.
- [ ] **Step 175** [W]: `kernels/softmax.hip.cpp` append `softmax_bwd_f32(grad_scores, grad_probs, probs, rows, cols)`.
- [ ] **Step 176** [W]: Rust wrapper.
- [ ] **Step 177** [CODE]: `test_softmax_bwd_matches_cpu`.
- [ ] **Step 178** [BUILD] + [TEST] [GPU:t=30s].
- [ ] **Step 179** [V]: cosine ≥ 0.9999.
- [ ] **Step 180** [C]: commit.

### Cross-entropy (final-layer softmax + loss)

Final logits get softmax + CE — fused kernel `ce_softmax_fwd_bwd` computes loss + grad_logits in one pass. The CPU path is `forward::cross_entropy_loss` + `backward::cross_entropy_softmax_backward`.

- [ ] **Step 181** [W]: `kernels/softmax.hip.cpp` append `ce_softmax_loss_and_grad(loss, grad_logits, logits, target, vocab, n_loss_positions)`.
- [ ] **Step 182** [W]: Rust wrapper — returns `(mean_loss, GpuBuffer<grad_logits>)`.
- [ ] **Step 183** [CODE] + [TEST]: cosine vs CPU, both loss and grad.
- [ ] **Step 184** [V]: |loss_gpu - loss_cpu| < 1e-4 AND cosine(grad_gpu, grad_cpu) ≥ 0.999.
- [ ] **Step 185** [C]: commit.

### Performance

- [ ] **Step 186** [TEST] [GPU:t=1min]: 35-layer × 384-seq softmax batch benchmark.
- [ ] **Step 187** [V]: GPU ≥ 20× CPU for this shape.

- [ ] **Step 188** [V]: **PHASE 5 EXIT GATE** — softmax fwd, masked fwd, bwd, CE-fused all correct + fast.

**Steps 189-200 reserved** for fused attention-softmax experiments (optional optimization — if we fuse softmax into the attention kernel directly in Phase 7, some of this becomes internal).

---

## PHASE 6: ROPE PARTIAL-ROTARY KERNEL (Steps 201-225) — 1 day

**Goal:** Ship partial-rotary RoPE forward + backward kernels. Gemma 4 applies RoPE only to the first `rope_dim` of each head (not the full head_dim), with different `freq_base` for sliding vs full layers. Kernel needs to respect that.
**Prerequisite:** Phase 5 green.
**ABORT-IF:** cosine < 0.9999 despite correct cos/sin tables → likely a partial-rotary masking bug; fix or STUCK.

- [ ] **Step 201** [R]: CPU reference `forward::rope_apply` (or equivalent in `forward.rs`) + `gemma4::rope_backward_partial`.
- [ ] **Step 202** [R]: `forward::rope_freqs(seq, rope_dim, freq_base)` — returns (cos_table, sin_table) shape `[seq, rope_dim/2]`.
- [ ] **Step 203** [DESIGN]: kernel input shapes — `x [seq, n_head, head_dim]`, `cos/sin [seq, rope_dim/2]`. Kernel does in-place rotation on the first `rope_dim` dims only; copies the tail `head_dim - rope_dim` unchanged.
- [ ] **Step 204** [W]: `kernels/rope.hip.cpp` — `rope_partial_fwd(out, in, cos, sin, seq, n_head, head_dim, rope_dim)`.
- [ ] **Step 205** [W]: Rust wrapper.
- [ ] **Step 206** [BUILD] + [TEST] cosine [GPU:t=30s].
- [ ] **Step 207** [V]: cosine ≥ 0.9999 on real layer-0 Q shape.
- [ ] **Step 208** [C]: commit.

### Backward

- [ ] **Step 209** [R]: CPU reference `gemma4::rope_backward_partial` — the inverse rotation (just swap sin sign effectively: `grad_x_even = cos*grad_out_even + sin*grad_out_odd`, `grad_x_odd = -sin*grad_out_even + cos*grad_out_odd`).
- [ ] **Step 210** [W]: `kernels/rope.hip.cpp` append `rope_partial_bwd(grad_in, grad_out, cos, sin, seq, n_head, head_dim, rope_dim)`.
- [ ] **Step 211** [W]: Rust wrapper.
- [ ] **Step 212** [BUILD] + [TEST] cosine.
- [ ] **Step 213** [V]: cosine ≥ 0.9999.
- [ ] **Step 214** [C]: commit.

### Freqs table upload

Upload cos/sin tables once per training session (not per step). Cache them in the `GpuKernelsHandle`.

- [ ] **Step 215** [CODE]: `hip_kernels::rope::RopeFreqsCache` struct holding uploaded (cos_buf, sin_buf) keyed by `(seq, rope_dim, freq_base)` tuple.
- [ ] **Step 216** [CODE]: `ensure_freqs(seq, rope_dim, freq_base) -> (&GpuBuffer cos, &GpuBuffer sin)` method — uploads on miss.
- [ ] **Step 217** [CODE] + [TEST]: unit test that ensure_freqs is idempotent (second call same cache) AND that cos/sin contents match `forward::rope_freqs` exactly.
- [ ] **Step 218** [V]: both assertions pass.
- [ ] **Step 219** [C]: commit.

### Integration smoke

- [ ] **Step 220** [TEST]: end-to-end — real Q tensor from layer 0 cache, apply GPU RoPE, compare to CPU-RoPE applied to same tensor.
- [ ] **Step 221** [V]: cosine ≥ 0.9999.

- [ ] **Step 222** [V]: **PHASE 6 EXIT GATE** — rope_partial fwd, bwd, + freqs cache all correct.

**Steps 223-225 reserved** for debug (wrong rotation convention, partial-rotary tail-dim bug).

---

## PHASE 7: ATTENTION KERNEL (Steps 226-275) — 4 days

**Goal:** The big one. Ship fused attention forward + backward. Gemma 4 has per-layer choice of `sliding` (local window) or `full` (causal) mask; q_rot/k_rot/v are post-norm post-rope; GQA expansion from `n_head_kv` to `n_head` via broadcast; softmax + mask + V-matmul.
**Prerequisite:** Phases 3/5 green (norm + softmax kernels exist).
**ABORT-IF:** cosine < 0.99 on real layer-0 attention output after 3 debug attempts → fall-back to unfused (Q@K^T via sgemm_bf16, softmax_fwd_masked kernel, score@V via sgemm_bf16 — three sequential launches). If even unfused < 0.99 → STUCK, escalate.

### Design

- [ ] **Step 226** [R]: CPU reference `gemma4::forward_gemma4_with_lora` — extract the attention block:
  - Q @ K^T → scores `[n_head, seq, seq]`, scale by `1/sqrt(head_dim)`.
  - Apply sliding OR causal mask (Gemma 4 layer_is_sliding(il) bool).
  - Softmax per row.
  - scores @ V → attn_out `[seq, n_head*head_dim]`.
- [ ] **Step 227** [DESIGN]: fuse OR compose?
  - **Recommendation:** Start UNFUSED (three kernels: score matmul, softmax_masked, output matmul). This lets us reuse Phase 5 softmax and existing hipBLAS sgemm_bf16. Verify correctness first. FUSE as an optimization in a follow-up if wall-clock demands it.
  - Rationale: correctness > speed. Fused Flash-Attention is the canonical final form but is 5× more complex to debug. Get the straight path working.
- [ ] **Step 228** [DESIGN]: K/V handling — `n_head_kv` < `n_head` on Gemma 4 (GQA). Two paths:
  - (a) Expand K/V CPU-side before kernel call (wasteful — 8× memory).
  - (b) Kernel reads K/V with broadcasting index `kv_head = q_head / (n_head / n_head_kv)`.
  - **Recommendation:** (b). Saves VRAM, matches existing backward's `gqa_expand` pattern.

### Forward — unfused

- [ ] **Step 229** [W]: `kernels/attn.hip.cpp` with `attn_scores_fwd(scores, q_rot, k_rot, n_head, n_head_kv, seq, head_dim)` — per-block Q@K^T with scale, broadcasting kv_head.
- [ ] **Step 230** [W]: `kernels/attn.hip.cpp` append `attn_mask_build(mask_bits, seq, window_size)` — produces sliding OR causal mask bitmap. Or pass precomputed mask from Rust.
- [ ] **Step 231** [W]: Rust wrapper `hip_kernels::attn::scores_fwd` + `build_mask`.
- [ ] **Step 232** [W]: `kernels/attn.hip.cpp` append `attn_output_fwd(attn_out, probs, v, n_head, n_head_kv, seq, head_dim)` — per-block probs@V, broadcasting.
- [ ] **Step 233** [W]: Rust wrappers.
- [ ] **Step 234** [CODE]: `#[test]` `test_attn_fwd_layer0_matches_cpu` — loads real layer-0 weights, runs Q/K/V proj + RoPE + norms on CPU, then attention forward on GPU vs CPU, cosine ≥ 0.9999.
- [ ] **Step 235** [BUILD] + [TEST] [GPU:t=2min].
- [ ] **Step 236** [V]: cosine ≥ 0.999 on layer 0 (slightly looser than 0.9999 because of accumulated bf16 through multiple ops).
- [ ] **Step 237** [DECIDE]: if cosine 0.99-0.999, check mask correctness + rope orientation. If < 0.99, debug. If > 0.999, celebrate.
- [ ] **Step 238** [C]: commit.

### Forward — verify on all layer types

Gemma 4 has both `sliding` (layers 0-19 except full) and `full` (layers 4, 9, 14, 19, 24, 29, 34) layers, and also KV-reusing layers (20-34 share K/V with an earlier producer). The kernel must handle all cases.

- [ ] **Step 239** [CODE]: extend `test_attn_fwd_layer0_matches_cpu` to cover layer 4 (full attention), layer 10 (sliding), layer 24 (KV-reusing).
- [ ] **Step 240** [TEST] [GPU:t=3min].
- [ ] **Step 241** [V]: all three layer types cosine ≥ 0.999.
- [ ] **Step 242** [C]: commit.

### Backward — attention_backward

Call it `attn_bwd(grad_q, grad_k, grad_v, grad_attn_out, q_rot, k_rot, v, probs, n_head, n_head_kv, seq, head_dim)`. The chain rule through attention:

```
grad_v      = probs^T @ grad_attn_out
grad_probs  = grad_attn_out @ V^T
grad_scores = softmax_jvp(probs, grad_probs)  (Phase 5 softmax_bwd)
grad_q      = grad_scores @ K * scale
grad_k      = grad_scores^T @ Q * scale
```

Then GQA-collapse grad_k / grad_v back to `n_head_kv` heads.

- [ ] **Step 243** [R]: CPU reference `backward::attention_backward` in `src/backward.rs` — full derivation.
- [ ] **Step 244** [W]: `kernels/attn.hip.cpp` append `attn_bwd_stage1(grad_v, grad_probs, probs, v, grad_attn_out, n_head, n_head_kv, seq, head_dim)` — stage 1 of the chain.
- [ ] **Step 245** [W]: uses `softmax_bwd` from Phase 5 for stage 2 (grad_scores).
- [ ] **Step 246** [W]: append `attn_bwd_stage3(grad_q, grad_k, grad_scores, q_rot, k_rot, n_head, n_head_kv, seq, head_dim, scale)` — Q/K gradient matmuls.
- [ ] **Step 247** [W]: Rust wrapper `hip_kernels::attn::backward` — composes stages 1+2+3.
- [ ] **Step 248** [CODE]: `test_attn_bwd_layer0_matches_cpu`.
- [ ] **Step 249** [BUILD] + [TEST] [GPU:t=2min].
- [ ] **Step 250** [V]: cosine ≥ 0.999 on grad_q, grad_k, grad_v each.
- [ ] **Step 251** [DECIDE]: if one of the three is < 0.999, isolate — run per-stage tests. If all three < 0.99, rethink stage boundaries.
- [ ] **Step 252** [C]: commit.

### Backward — all layer types

- [ ] **Step 253** [CODE]: extend backward test to layer 4, 10, 24.
- [ ] **Step 254** [TEST] [GPU:t=3min].
- [ ] **Step 255** [V]: all grads cosine ≥ 0.999.
- [ ] **Step 256** [C]: commit.

### GQA collapse

- [ ] **Step 257** [R]: CPU `backward::gqa_collapse(grad_k_expanded, n_head, n_head_kv, head_dim, seq)` — sums `n_head / n_head_kv` consecutive heads' gradients into a single kv_head gradient.
- [ ] **Step 258** [W]: `kernels/attn.hip.cpp` append `gqa_collapse_f32(out, in, n_head, n_head_kv, head_dim, seq)`.
- [ ] **Step 259** [W]: Rust wrapper.
- [ ] **Step 260** [CODE] + [TEST]: cosine.
- [ ] **Step 261** [V]: cosine ≥ 0.9999 (pure sum, should be near-exact).
- [ ] **Step 262** [C]: commit.

### Performance

- [ ] **Step 263** [CODE]: `test_attn_fwd_perf` — time 100 full-forward attention passes for seq=384 on GPU vs CPU.
- [ ] **Step 264** [TEST] [GPU:t=3min].
- [ ] **Step 265** [V]: GPU ≥ 30× CPU for seq=384 full attention. If < 10×, flag for fusion-optimization follow-up.

- [ ] **Step 266** [V]: **PHASE 7 EXIT GATE** — attention fwd + bwd correct on all 3 layer-types (sliding, full, KV-reusing), GQA collapse correct, performance competitive.

**Steps 267-275 reserved** for debug (most likely issues: mask off-by-one, RoPE-then-norm vs norm-then-RoPE ordering, V-broadcast indexing, scale factor `1/sqrt(head_dim)` dropped somewhere).

---

## PHASE 8: GpuKernelsBackend INTEGRATION (Steps 276-320) — 2 days

**Goal:** Write the actual `impl ForgeBackend for GpuKernelsBackend { ... }` that uses all the kernels from Phases 3-7. Activations stay GPU-resident across the whole forward pass (no CPU round-trips between ops). Phase 1 diagnostic confirmed attention was the bottleneck; this phase eliminates it.
**Prerequisite:** Phases 3-7 all green.
**ABORT-IF:** After integration, end-to-end forward cosine vs CpuBackend < 0.99 → there's a residency / upload ordering bug; debug. If > 3 debug attempts, fall back to step-at-a-time integration (one op at a time) as a hybrid intermediate.

### Handle design

- [ ] **Step 276** [DESIGN]: `GpuKernelsHandle` holds:
  - Everything `Gemma4GpuWeights` has (reuse, don't duplicate) — weights on GPU.
  - `RopeFreqsCache` — cos/sin tables per layer config.
  - `ActivationPool` — pre-allocated GPU buffers for hidden, attn_out, ffn_gate_pre, etc. Sized for max_seq.
  - `BlasHandle` — hipBLAS for matmuls.
- [ ] **Step 277** [CODE]: `src/backend.rs` add `pub struct GpuKernelsBackend { max_seq: usize, ple_mode: PleMode }`, `pub struct GpuKernelsHandle { gpu_weights: Gemma4GpuWeights, rope_cache: RopeFreqsCache, activations: ActivationPool }`.
- [ ] **Step 278** [CODE]: `impl ForgeBackend for GpuKernelsBackend { type Handle = GpuKernelsHandle; ... }` with `upload_weights` that builds the handle.
- [ ] **Step 279** [BUILD]: green, even though forward/backward are `unimplemented!()` stubs.
- [ ] **Step 280** [C]: commit.

### Activation pool

- [ ] **Step 281** [DESIGN]: buffers needed per forward at max_seq:
  - hidden `[max_seq, n_embd]` × 2 (ping-pong or residual).
  - normed_input, post_attn_residual, post_ffw_residual.
  - q, k, v (pre-norm, post-norm, post-rope variants — ~6 buffers).
  - attn_scores `[n_head, max_seq, max_seq]`.
  - attn_probs same shape.
  - ffn_gate_pre, ffn_up_pre, ffn_hidden `[max_seq, n_ff]`.
  - logits `[max_seq, vocab]`.
  Estimated footprint: ~500 MB at max_seq=512. Fits.
- [ ] **Step 282** [CODE]: `ActivationPool { hidden_a, hidden_b, q, k, v, ..., allocate_for(max_seq, hparams) }`.
- [ ] **Step 283** [CODE] + [TEST]: unit test — allocate pool for hparams of loaded Gemma 4 E2B, max_seq=512; assert VRAM used < 1 GB; assert all buffers non-null.
- [ ] **Step 284** [V]: pass.
- [ ] **Step 285** [C]: commit.

### Forward implementation — layer 0 only

Start with ONE layer to prove the path. Reuse the CPU `forward_gemma4_with_lora` overall structure but replace each op with its GPU counterpart, keeping activations on GPU.

- [ ] **Step 286** [CODE]: `forward_one_layer_gpu(handle, lora, layer_idx, hidden_in, hidden_out)` — single-layer forward using all Phase 3-7 kernels. 
- [ ] **Step 287** [CODE]: `impl ForgeBackend::forward` for GpuKernelsBackend — loops `forward_one_layer_gpu` over all 35 layers; final rmsnorm + LM head matmul + softcap.
- [ ] **Step 288** [CODE]: temporarily add a `#[test]` `test_gpu_kernels_forward_layer0_matches_cpu` — just run layer 0 forward, cosine vs CPU.
- [ ] **Step 289** [BUILD] + [TEST] [GPU:t=2min].
- [ ] **Step 290** [V]: cosine ≥ 0.99 on hidden_out after layer 0.
- [ ] **Step 291** [D, conditional]: if cosine < 0.99, bisect by disabling ops one at a time (e.g., replace GPU attention with CPU attention, keep everything else GPU, see if cosine recovers).
- [ ] **Step 292** [C]: commit layer-0 milestone.

### Forward — all 35 layers

- [ ] **Step 293** [CODE]: extend test to full forward (all 35 layers + LM head).
- [ ] **Step 294** [TEST] [GPU:t=2min].
- [ ] **Step 295** [V]: cosine of logits ≥ 0.99 vs CpuBackend. Top-5 overlap ≥ 4/5 at last position.
- [ ] **Step 296** [C]: commit.

### Backward implementation

- [ ] **Step 297** [CODE]: `impl ForgeBackend::backward` for GpuKernelsBackend — mirrors forward structure in reverse, using all Phase 3/5/6/7 bwd kernels.
- [ ] **Step 298** [CODE]: `test_gpu_kernels_backward_matches_cpu` — full backward pass, compare layer-by-layer grad_hidden cosine.
- [ ] **Step 299** [BUILD] + [TEST] [GPU:t=3min].
- [ ] **Step 300** [V]: ALL layers' grad_hidden cosine ≥ 0.99 vs CpuBackend. Grad health: `healthy=35/35 zero=0 nan=0`.
- [ ] **Step 301** [C]: commit.

### Train step full integration

- [ ] **Step 302** [CODE]: since `train_step` is the default trait method (forward+backward+Adam), it Just Works. Add `test_gpu_kernels_train_step_loss_descent` — 3 steps on the synthetic test, loss must descend.
- [ ] **Step 303** [TEST] [GPU:t=1min].
- [ ] **Step 304** [V]: loss descends, no NaN. Ideally trajectory matches `HybridMatmulBackend` within 0.5 on final loss.
- [ ] **Step 305** [C]: commit.

### Perf measurement

- [ ] **Step 306** [TEST] [GPU:t=5min]: time the new Backend at seq=384 on Kingdom smoke corpus.
- [ ] **Step 307** [V]: warm step-time ≤ 5 s. If 5-10 s, still a win vs CPU-blocked 64+ min. If > 10 s, [D] with rocprof to find hot spot.

- [ ] **Step 308** [V]: **PHASE 8 EXIT GATE** — GpuKernelsBackend trains on real seq=384 at ≤ 5 s/step warm.

**Steps 309-320 reserved** for optimization (fuse softmax into attention; eliminate unneeded syncs; reduce host↔device transfers).

---

## PHASE 9: REGRESSION + PHASE 5 RAFT RETRY (Steps 321-345) — 1 day

**Goal:** Verify all Learning Gate tests still pass on HybridMatmulBackend AND add equivalent tests on GpuKernelsBackend. Run the Kingdom RAFT training at seq=384 that Phase 5 of WAVE10F couldn't.
**Prerequisite:** Phase 8 green.
**ABORT-IF:** Any pre-existing Learning Gate test regresses on HybridMatmulBackend → revert, bisect. Non-negotiable.

### Regression on HybridMatmulBackend (incumbent must still work)

- [ ] **Step 321** [TEST] [GPU:t=3min]: `test_gemma4_backward_grad_health`.
- [ ] **Step 322** [V]: healthy=35/35 (bit-identical).
- [ ] **Step 323** [TEST] [GPU:t=3min]: `test_gemma4_gpu_train_step_loss_descent`.
- [ ] **Step 324** [V]: trajectory 19.9595 → 15.9253 → 5.7022 (bit-identical).
- [ ] **Step 325** [TEST] [GPU:t=5min]: `test_learning_exp1_held_out_eval`.
- [ ] **Step 326** [V]: ratio 0.5806 ± 0.01, CI excludes 1.0.
- [ ] **Step 327** [TEST] [GPU:t=6min]: `test_learning_exp3_lora_zero_identity`, `test_learning_exp4_dataset_size_scaling`, `test_learning_exp5_generalization_gap_beta`.
- [ ] **Step 328** [V]: all PASS with bit-identical metrics.

### Regression on GpuKernelsBackend (the new one)

- [ ] **Step 329** [CODE]: add `test_learning_exp1_gpu_kernels` — same as exp1 but explicit `GpuKernelsBackend`. Assertion: ratio ≤ 0.90, CI excludes 1.0.
- [ ] **Step 330** [TEST] [GPU:t=5min].
- [ ] **Step 331** [V]: pass. (Ratio may differ from HybridMatmulBackend within bf16 noise — that's acceptable.)
- [ ] **Step 332** [C]: commit.

### Phase 5 retry at seq=384

- [ ] **Step 333** [B] [GPU:t=~2h]: real RAFT training at seq=384.
  ```bash
  ~/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge \
    train-gemma4 --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-kingdom-train.jsonl \
    --steps 500 --lr 1e-3 \
    --output /tmp/wave11-kingdom-lora.zlg4 \
    2>&1 | tee /tmp/wave11-phase9-kingdom.log | tail -80
  ```
  (Assumes CLI uses GpuKernelsBackend by default after Phase 8. Add `--backend kernels` flag if needed.)
- [ ] **Step 334** [V]: completes 500 steps; warm step-time ≤ 5 s; no NaN; `.zlg4` output exists.
- [ ] **Step 335** [TEST] [GPU:t=5min]: eval the trained LoRA on held-out corpus — add `test_raft_kingdom_eval_trained` that loads the saved `.zlg4` and scores the eval set.
- [ ] **Step 336** [V]: eval loss < base-model eval loss on Kingdom corpus (the trained LoRA outperforms base). Margin ≥ 3%.
- [ ] **Step 337** [C]: commit the Kingdom LoRA results + perf notes.

- [ ] **Step 338** [V]: **PHASE 9 EXIT GATE** — all regression tests green on BOTH backends; first real Kingdom RAFT LoRA trained at seq=384 in reasonable wall-clock.

**Steps 339-345 reserved** for RAFT-specific debug (NaN, overflow, checkpoint recovery).

---

## PHASE 10: DOCS + ADR + HANDOFF (Steps 346-360) — 0.5 day

**Goal:** Persist everything that matters to the repo. Future sessions inherit the knowledge, not just the code.
**Prerequisite:** Phase 9 green.
**ABORT-IF:** Phase 7 aborted — this phase STILL runs to document the abort honestly.

- [ ] **Step 346** [W]: `docs/adr/ADR-049-gpu-kernels-backend.md` — architecture decision record covering: why hand-written HIP C++ over rocWMMA/Triton; op-by-op rollout sequence; correctness-first-speed-second philosophy; activation residency strategy; fall-back plan.
- [ ] **Step 347** [W]: `crates/zhenai-forge/notes/wave11-session-log.md` — session-by-session progress, what worked, what didn't, key numbers.
- [ ] **Step 348** [W]: `crates/zhenai-forge/notes/wave11-timing.md` — kernel micro-benchmarks + end-to-end perf.
- [ ] **Step 349** [W]: append a "WAVE11 DONE" banner to the top of `wave11-gpu-kernels-battle-plan.md` (this file) summarizing outcomes.
- [ ] **Step 350** [W]: update `CLAUDE.md` WAVE11 entry — add summary of the GPU kernels ship + first Kingdom LoRA.
- [ ] **Step 351** [W]: auto-memory — add `project_wave11_gpu_kernels.md` with the outcome + pointer to notes.
- [ ] **Step 352** [C]: commit doc batch.
- [ ] **Step 353** [B]: `cd ~/tmp/unheaded && git log --oneline $(git rev-list -n 1 --before=\"$(date -u '+%Y-%m-%d' -d '21 days ago')\" HEAD)..HEAD | head -40` — final commit log.
- [ ] **Step 354** [V]: commits all tagged `[PLAN W11]` prefix.
- [ ] **Step 355** [B]: `git status`.
- [ ] **Step 356** [V]: working tree clean.

- [ ] **Step 357** [V]: **WAVE11 EXIT GATE (10-point)**:
  1. HIP FFI pipeline builds and launches kernels.
  2. RMSNorm, GELU, Softmax, RoPE, Attention kernels all correct (cosine ≥ 0.999).
  3. All backward counterparts correct.
  4. GpuKernelsBackend integrated via ForgeBackend trait.
  5. HybridMatmulBackend regression bit-identical.
  6. Kingdom RAFT training at seq=384 completes 500 steps ≤ 5 s/step warm.
  7. Trained LoRA's eval beats base model on held-out Kingdom corpus.
  8. ADR-049 + session log + timing notes + CLAUDE.md updated.
  9. Auto-memory updated with outcome pointer.
  10. Commits chained under `[PLAN W11]` prefix; tree clean.

**Steps 358-360 reserved** for final review polish.

---

## APPENDIX A: EMERGENCY PROCEDURES

### A.1 `hipcc` compile failure on a new kernel

1. `hipcc --offload-arch=gfx1101 -c kernels/foo.hip.cpp -o /tmp/foo.o` — compile one file in isolation.
2. If it fails → read the error; most likely: missing include, wrong __global__ launch-bounds, bf16 conversion intrinsics unavailable on gfx1101.
3. If it succeeds standalone → build.rs linkage bug; inspect `cargo -vv build` to see the exact hipcc invocation.
4. Last resort: compile to bitcode and inspect with `llvm-dis` to see what the compiler emitted.

### A.2 "hipModuleLaunchKernel returned hipErrorInvalidDeviceFunction"

1. The kernel name in the `.so` doesn't match what Rust is looking up. Check `nm -D target/.../wave11-kernels.so | grep <kernel>`.
2. C++ name-mangling: either decorate all kernels with `extern "C"` in the `.hip.cpp` file, OR use `cdylib` and resolve mangled names explicitly.
3. Wrong gfx arch: the `.so` was compiled for a different target. Verify `hipcc --offload-arch=gfx1101`.

### A.3 Kernel launches but output is all NaN

1. Is the input valid (no prior-kernel NaN)? Download input, check.
2. Is shared memory initialized before reduction? Unsynchronized reads of undefined memory.
3. Is a divide-by-zero possible (e.g., softmax over all-mask row)? Add epsilon.
4. Is an out-of-bounds read happening (grid > input length)? Bounds-check in kernel.

### A.4 cosine ≥ 0.9999 on synthetic data but < 0.99 on real weights

1. Real weights are bf16 (not f32). Is the kernel reading as the right type?
2. Is the CPU reference using bf16→f32→compute pattern matching what the kernel does?
3. Is a stale weight buffer being reused (e.g., layer 0 weights on layer 5 computation)?

### A.5 Eval descent reversed after refactor (regression in an existing backend)

1. `git bisect` between the last known-good commit and HEAD.
2. Usual suspects in a kernels sprint: accidentally changed a shared helper (`forward::softmax`, `backward::rmsnorm_backward`) used by the CPU path.
3. Fix the offender; do NOT "fix" the test to match the regression.

### A.6 Phase 7 attention fwd cosine < 0.99 persistently

Fall-back plan (document + escalate): revert to unfused attention — three separate kernel launches (sgemm_bf16 for scores, softmax_fwd_masked, sgemm_bf16 for output). We lose ~30% perf but gain clean correctness. Document in ADR-049 as "unfused attention used; Flash-Attention-style fusion deferred to WAVE11.1."

### A.7 Kingdom RAFT NaN mid-run

1. Stop immediately.
2. Re-run with `lr=3e-4` (1/3 of 1e-3).
3. If still NaN, set per-layer clip threshold tighter (0.3 instead of 1.0).
4. If NaN persists at rank=16: drop to rank=8 temporarily to validate that the kernels themselves are correct — NaN from training dynamics is separable from NaN from a kernel bug.

---

## APPENDIX B: CRITICAL PATH + FALL-BACK

### Critical path (minimum serial time)

```
Phase 0 (1h) → Phase 1 (8h) → Phase 2 (1d) → Phase 3 (2d) → Phase 5 (2d)
                                             ↓                           ↓
                                             Phase 4 (0.5d, parallel after P2)
                                             ↓                           ↓
                                             Phase 6 (1d, parallel after P2)
                                             ↓                           ↓
                                             Phase 7 (4d, requires P5+P6)
                                             ↓
                                             Phase 8 (2d, requires all)
                                             ↓
                                             Phase 9 (1d)
                                             ↓
                                             Phase 10 (0.5d)
```

Serial dependencies: 0 → 1 → 2 → 3 → 5 → 7 → 8 → 9 → 10 = ~14 days.
With a second agent Phases 4 and 6 can parallelize with Phase 5 (save ~1.5 days); solo, they serialize.

### Fall-back plan if kernels stall

**Fall-back 1 (if Phase 3 RMSNorm stalls on correctness):**
The issue is almost certainly the `1+weight` formulation or eps placement. Spend max 2 days. If still stuck, SKIP RMSNorm and use rocWMMA's attention primitives (they have normalize included). Revert to hand-written approach later.

**Fall-back 2 (if Phase 7 attention stalls):**
Use unfused attention per Appendix A.6 — sgemm_bf16 + softmax_fwd_masked + sgemm_bf16. Accept ~30% perf tax. Ship the LoRA; re-attack fusion in a follow-up sprint.

**Fall-back 3 (if kernels pass correctness but perf is < 3× over HybridMatmulBackend at seq=384):**
Profile with `rocprof`. Expected hotspots: memory bandwidth (not compute), launch overhead (> 10 kernel launches per layer × 35 layers = 350 launches/forward). Batch kernel launches by fusing adjacent ops. If still < 3× after fusion, **the bottleneck was never attention — re-derive** from Phase 1 diagnostic data.

**Fall-back 4 (CATASTROPHIC — kernels just don't work on gfx1101):**
Pin down ROCm 5.x or test on a friend's CUDA card as sanity. If RDNA 3 has genuine issues, cut scope: ship only RMSNorm + GELU + Softmax kernels (no attention); accept CPU attention at seq ≤ 128; document the constraint.

### Shippable even if Phase 7 aborts

Phases 3-5 alone (RMSNorm + GELU + Softmax) remove the bulk of CPU work from forward/backward — our internal estimate is ~40% of end-to-end time. Shipping those + keeping CPU attention would still unlock seq=192 training at tolerable wall-clock. It's not the shiny answer but it's a shippable intermediate.

---

## APPENDIX C: QUICK REFERENCE

### Key file paths

```
crates/zhenai-forge/
├── build.rs                    # hipcc build
├── kernels/
│   ├── common.hip.hpp          # shared helpers
│   ├── identity.hip.cpp        # smoke test (Phase 2)
│   ├── rmsnorm.hip.cpp         # Phase 3
│   ├── gelu.hip.cpp            # Phase 4
│   ├── softmax.hip.cpp         # Phase 5
│   ├── rope.hip.cpp            # Phase 6
│   └── attn.hip.cpp            # Phase 7
├── src/
│   ├── hip_kernels/
│   │   ├── mod.rs              # types, loader
│   │   ├── loader.rs           # dlopen + symbol resolution
│   │   ├── identity.rs
│   │   ├── rmsnorm.rs
│   │   ├── gelu.rs
│   │   ├── softmax.rs
│   │   ├── rope.rs
│   │   └── attn.rs
│   ├── backend.rs              # add GpuKernelsBackend here (Phase 8)
│   └── ...existing files unchanged...
```

### Key commands

```bash
# Compile all kernels (dev, not via cargo):
hipcc --offload-arch=gfx1101 --shared -fPIC -O3 \
  crates/zhenai-forge/kernels/*.hip.cpp \
  -o /tmp/wave11-kernels-dev.so

# Inspect exported symbols:
nm -D /tmp/wave11-kernels-dev.so | grep ' T '

# Run a single kernel test:
cd ~/tmp/unheaded/crates/zhenai-forge && \
  cargo test --release --bin zhenai-forge test_rmsnorm_fwd_matches_cpu -- \
  --test-threads=1 --nocapture

# Profile an integration test:
rocprof --stats cargo test --release --bin zhenai-forge \
  test_gpu_kernels_train_step_loss_descent -- --test-threads=1 --nocapture
```

### Key cosine thresholds

- Per-op (RMSNorm, GELU, RoPE, Softmax individually): **cosine ≥ 0.9999**.
- Per-stage (attention fwd or bwd at one layer): **cosine ≥ 0.999**.
- End-to-end (full forward or full backward across 35 layers): **cosine ≥ 0.99**.
- Eval ratio regression on Learning Gate exp1: **within 0.02 of baseline 0.5806**.

### Gemma 4 architectural constants (from loaded GGUF)

```
n_layer   = 35
n_embd    = 1536
n_head    = 8
n_head_kv = 2        # GQA: 4 Q-heads per KV-head
head_dim_full  = 256
head_dim_swa   = 256
n_ff      = 6144     # per feed_forward_length tensor dim
vocab     = 262144
rms_norm_eps = 1e-6
final_logit_softcapping = 30.0
```

### ROCm environment

```
hipcc 6.4.43484 (LLVM 19, roc-6.4.2)
Target arch: gfx1101 (RDNA 3, Navi 32)
VRAM: 12.87 GB (4.57 GB used by Gemma4GpuWeights at PleMode::Cpu)
```

---

*WAVE11 GpuKernelsBackend Battle Plan — Forged 2026-04-21*
*11 Phases. ~360 step slots. From "GPU is idle" to "real Kingdom LoRA in 2 hours."*
*Write every kernel. Verify every gradient. Keep activations on the card. Let the silicon work.*
