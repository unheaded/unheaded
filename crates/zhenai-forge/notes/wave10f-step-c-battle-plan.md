# WAVE10F Phase 7 Step C Battle Plan — Backward GPU Port

**Date forged**: 2026-04-18
**Sprint**: WAVE10F Phase 7 Step C — port `backward_gemma4_with_lora` matmuls to GPU
**Prerequisite**: branch `main` at `b5f3da24` (Step A + B + hybrid landed). All existing gemma4 tests green.
**Target**: per-step time ≤15s on real `/var/zhen/models/gemma-4-E2B-it.gguf` (vs 43s hybrid, 55-95s pure CPU). Loss descent test still passes within bf16 precision.
**Estimated Duration**: 6-8 hours focused session
**Agent Strategy**: solo / coordinator (test runs serialize on the GPU, sequential discipline matters)
**Commit Cadence**: every 3-5 steps, plus immediate after each phase exit gate
**Stuck Protocol**: skip after 3x time estimate or 2 failed debug attempts; per-site cosine fallout = revert site + escalate

---

## LEGEND

```
[B]            bash command (run as-is)
[V]            verification (must pass to proceed)
[D]            debug (only on V failure)
[W]            write file
[R]            read file
[C]            commit checkpoint
[CODE]         in-source edit (not a single bash line)
[TEST]         cargo test invocation
[BUILD]        cargo build invocation
[DECIDE]       decision with pre-seeded recommendation
[ESCALATE]     STOP — needs Stevie input
[STUCK]        skipped per Skip Protocol
[BLOCKED]      blocked by upstream STUCK
[ROLLBACK]     revert + re-test path
```

`[STUCK]` is for Skip Protocol skips. `[BLOCKED]` only marks downstream consequences. `[DECIDE]` is autonomous (agent picks the recommendation). `[ESCALATE]` is rare and means actually stop.

---

## CRITICAL FACTS (cached for the executing agent)

- Repo: `/home/govan/tmp/unheaded/`
- Crate: `crates/zhenai-forge/`
- Source files touched: `src/gemma4.rs` (signature change + dispatch at matmul sites), `src/gemma4_gpu.rs` (no new function — single-function approach)
- Source files UNTOUCHED: `src/main.rs` (until Step E), `src/train.rs` (Mistral path stays stale)
- GPU weight handles all named on `Gemma4GpuWeights`:
  `wq[il]`, `wk[il]: Option`, `wv[il]: Option`, `wo[il]`,
  `ffn_gate[il]`, `ffn_up[il]`, `ffn_down[il]`,
  `inp_gate[il]`, `proj[il]`, `token_embd`, `per_layer_model_proj`
- Helpers already present and verified (cosine ≥0.999):
  `gpu.matmul_xwt(w_buf, input_f32, m, n, k) → Vec<f32>`  (forward style: input @ w^T)
  `gpu.matmul_grad_x(w_buf, grad_f32, m, n, k) → Vec<f32>` (backward style: grad @ w)
- Sync rule: `download_f32` is synchronous (hipMemcpy D2H). DO NOT call `crate::hip::sync()` after a matmul before download.
- Forward GPU is at `forward_gemma4_gpu(cpu, gpu, lora, tokens) -> Result<(logits, caches), String>` in `src/gemma4_gpu.rs`.
- Backward CPU is at `backward_gemma4_with_lora(weights: &CpuWeightsGemma4, lora, caches, logits, tokens, answer_start) -> (loss, health)` in `src/gemma4.rs` starting line ~1399.
- Hybrid step at `train_step_gemma4_hybrid(cpu, gpu, lora, tokens, answer_start, lr, step) -> Result<f32, String>` in `src/gemma4_gpu.rs`.
- Memory: 4.57 GB VRAM with PleMode::Cpu. Plenty of headroom. Don't allocate huge transient GPU buffers in tight loops; use the existing per-call GpuBuffer alloc pattern.

---

## MATMUL SITES TO MIGRATE (14 total)

Numbered per their order in backward_gemma4_with_lora's reverse loop.

| # | Site | CPU shape | GPU helper | Per-layer? |
|---|------|-----------|------------|----------|
| 1 | LM head backward (`grad_logits @ tok_embd`) | `[seq,vocab] @ [vocab,n_embd]` | `matmul_grad_x(token_embd, grad_logits, seq, vocab, n_embd)` | No (global) |
| 2 | reconstruct ffn_out per row (inline) | per-row `[1,n_ff] @ [n_embd,n_ff]^T` | LIFT: full-batch `matmul_xwt(ffn_down, ffn_hidden, seq, n_embd, n_ff)` | Yes |
| 3 | ffn down backward (`grad_ffn_out @ ffn_down`) | `[seq,n_embd] @ [n_embd,n_ff]` wait — see notes | `matmul_grad_x(ffn_down, grad_ffn_out, seq, n_embd, n_ff)` | Yes |
| 4 | ffn gate backward | `[seq,n_ff] @ [n_ff,n_embd]` | `matmul_grad_x(ffn_gate, grad_gate_pre, seq, n_ff, n_embd)` | Yes |
| 5 | ffn up backward | same as gate | `matmul_grad_x(ffn_up, grad_up_pre, seq, n_ff, n_embd)` | Yes |
| 6 | reconstruct o_out per row (inline) | per-row `[1,q_out] @ [n_embd,q_out]^T` | LIFT: `matmul_xwt(wo, attn_out, seq, n_embd, q_out_dim)` | Yes |
| 7 | wo backward (`grad_o_out @ wo`) | `[seq,n_embd] @ [n_embd,q_out]` | `matmul_grad_x(wo, grad_o_out, seq, n_embd, q_out_dim)` | Yes |
| 8 | wq backward | `[seq,q_out] @ [q_out,n_embd]` | `matmul_grad_x(wq, grad_q, seq, q_out_dim, n_embd)` | Yes |
| 9 | wk backward (has_kv) | `[seq,kv_out] @ [kv_out,n_embd]` | `matmul_grad_x(wk, grad_k, seq, kv_out_dim, n_embd)` | Yes (cond) |
| 10 | wv backward (has_kv) | `[seq,kv_out] @ [kv_out,n_embd]` | `matmul_grad_x(wv, grad_v_pre, seq, kv_out_dim, n_embd)` | Yes (cond) |
| 11 | reconstruct_q_pre_norm helper | `[seq,n_embd] @ [n_embd,q_out]^T` | `matmul_xwt(wq, normed_input, seq, q_out_dim, n_embd)` | Yes |
| 12 | reconstruct_kv_pre_norm helper | `[seq,n_embd] @ [n_embd,kv_out]^T` | `matmul_xwt(wk_or_wv, normed_input, seq, kv_out_dim, n_embd)` | Yes (cond) |
| 13 | PLE proj backward | `[seq,n_embd] @ [n_embd,n_epl]` | `matmul_grad_x(proj, grad_proj_out_pre_norm, seq, n_embd, n_epl)` | Yes (PLE) |
| 14 | PLE inp_gate backward | `[seq,n_epl] @ [n_epl,n_embd]` | `matmul_grad_x(inp_gate, grad_gate_pre_ple, seq, n_epl, n_embd)` | Yes (PLE) |

**Verify each site independently** before flipping the production train step.

---

## PHASE 0: RECON + BASELINE LOCK (Steps 1-15) — ~30 min

**Goal**: Confirm current state, capture baselines so regressions are detectable, lock the working tree.

- [ ] **Step 1** [B] ~30s: `cd ~/tmp/unheaded && git status && git log --oneline -3`
- [ ] **Step 2** [V] ~10s: working tree clean; HEAD is `b5f3da24` or descendant. Otherwise STOP.
- [ ] **Step 3** [B] ~5s: `cd ~/tmp/unheaded/crates/zhenai-forge && grep -n "fn backward_gemma4_with_lora\|fn matmul_x_wt\|fn matmul_grad_x\b" src/gemma4.rs | head -10` — pin line numbers. Expected: `backward_gemma4_with_lora` near line 1399.
- [ ] **Step 4** [B] ~5s: `awk '/let.*bf16_to_f32_vec.*weights\.(wq|wk|wv|wo|ffn_|inp_gate|proj|token_embd|per_layer_model_proj)/{print NR": "$0}' src/gemma4.rs | head -25` — list every bf16→f32 conversion site touching trainable weights. Should yield ~14 hits inside `backward_gemma4_with_lora` plus 2 in the `reconstruct_*` helpers.
- [ ] **Step 5** [W]: scratch the line numbers from Step 4 into `/tmp/wave10f-step-c-sites.txt` for Phase 3 reference.
- [ ] **Step 6** [B]: `awk '/^MemAvailable/ {printf "MemAvailable: %.1f GB\n", $2/1024/1024}' /proc/meminfo` — confirm ≥9 GB free before any cargo test (CpuWeights load needs ~9 GB).
- [ ] **Step 7** [V]: ≥9 GB MemAvailable. If not, kill RAM-heavy procs first (`docker compose down`, etc.).
- [ ] **Step 8** [TEST] ~3min: baseline correctness — `cargo test --release --bin zhenai-forge test_gemma4_backward_grad_health --no-fail-fast -- --nocapture --test-threads=1 2>&1 | tee /tmp/baseline-grad-health.log`
- [ ] **Step 9** [V]: log shows `healthy=35/35 zero=0 nan=0`. Otherwise STOP — fix CPU baseline before porting.
- [ ] **Step 10** [TEST] ~6min: baseline loss descent — `cargo test --release --bin zhenai-forge test_gemma4_train_step_loss_descent --no-fail-fast -- --nocapture --test-threads=1 2>&1 | tee /tmp/baseline-loss-descent.log`
- [ ] **Step 11** [V]: log shows monotonic descent ending at loss <8 on step 3.
- [ ] **Step 12** [TEST] ~6min: hybrid baseline — `cargo test --release --bin zhenai-forge test_gemma4_hybrid_train_step_loss_descent --no-fail-fast -- --nocapture --test-threads=1 2>&1 | tee /tmp/baseline-hybrid.log`
- [ ] **Step 13** [V]: hybrid baseline shows ~43s/step and final loss <8.
- [ ] **Step 14** [B]: `git stash list && git status` — make sure no untracked drift before edits begin.
- [ ] **Step 15** [V]: **PHASE 0 EXIT GATE** — three baseline logs in `/tmp/`, working tree clean, sites file written. If any baseline failed, do NOT proceed.

**Phase 0 exit commit (no code change yet)**: skip — only commit code changes.

---

## PHASE 1: SITE-BY-SITE VERIFICATION HARNESS (Steps 16-40) — ~1 hour

**Goal**: For each of the 14 matmul shapes that will move to GPU, prove the GPU helper produces the right answer at production scale. Builds confidence before touching the live function.

The harness lives in `src/gemma4_gpu.rs` test module. Pattern: load real weights, build random input matching the production shape, run CPU reference + GPU candidate, assert cosine ≥0.9999.

We already have two such tests (`test_gemma4_gpu_wq_matches_cpu`, `test_gemma4_gpu_matmul_grad_x_matches_cpu`). Generalize to a parameterized macro/helper to avoid 12× copy-paste.

- [ ] **Step 16** [CODE]: in `src/gemma4_gpu.rs#[cfg(test)] mod tests`, add a helper `fn check_gpu_matmul(name: &str, cpu_out: &[f32], gpu_out: &[f32], min_cosine: f32)` that prints (name, cosine, max_abs_err, vector lengths) and asserts cosine ≥ min_cosine.
- [ ] **Step 17** [CODE]: add `fn rand_f32(n: usize, seed: u64) -> Vec<f32>` deterministic LCG. Reuse pattern from existing `det_vec` in backward.rs tests.
- [ ] **Step 18** [BUILD] ~30s: `cargo build --release` — verify helpers compile.
- [ ] **Step 19** [V]: build green. Otherwise [D]: fix and retry.
- [ ] **Step 20** [C]: `git add -A && git commit --no-gpg-sign -m "[PLAN STEP C] Step 16-19: gpu_matmul harness helpers"`

For each of the 14 sites, write a `test_gpu_site_<N>_<name>` that:
1. Loads CpuWeightsGemma4 + uploads Gemma4GpuWeights (PleMode::Cpu).
2. Picks layer 0 (or layer 19 for KV-producing-only sites; layer 24 for KV-reusing skip).
3. Builds random input of the production shape.
4. Computes CPU reference (the existing matmul_x_wt or matmul_backward_a equivalent).
5. Computes GPU candidate via the appropriate helper.
6. Calls `check_gpu_matmul(...)`.

Sites already covered:
- Site 8 (wq backward) → `test_gemma4_gpu_matmul_grad_x_matches_cpu` — DONE
- Site 11 (reconstruct_q_pre_norm) → `test_gemma4_gpu_wq_matches_cpu` — DONE

Remaining sites get one test each:

- [ ] **Step 21** [CODE]: `test_gpu_site_1_lm_head` — input `grad_logits[seq=4, vocab=262144]`, weight `token_embd`, expected `[seq, n_embd=1536]`. Use `matmul_grad_x`.
- [ ] **Step 22** [TEST] ~3min: run `cargo test --release --bin zhenai-forge test_gpu_site_1 -- --test-threads=1 --nocapture`.
- [ ] **Step 23** [V]: cosine ≥0.9999. If fail → [D] inspect ldX values and shape derivation.
- [ ] **Step 24** [CODE+TEST]: sites 2 / 6 are reconstruct rows — they map to `matmul_xwt` with the SAME shape as forward (already covered by `test_gemma4_gpu_wq_matches_cpu`). Mark as covered by analogy; no new test.
- [ ] **Step 25** [CODE+TEST] ~3min: `test_gpu_site_3_ffn_down_grad` — input `grad_ffn_out[seq, n_embd]`, weight `ffn_down`, expected `[seq, n_ff]`. Verify via `check_gpu_matmul`.
- [ ] **Step 26** [V]: cosine ≥0.9999 else [D].
- [ ] **Step 27** [C]: commit (sites 1, 3 verified).
- [ ] **Step 28** [CODE+TEST]: sites 4 + 5 (ffn_gate, ffn_up backward) — same shape, just different weight buffer. Single test `test_gpu_site_4_ffn_gate_grad` is enough; assert both ffn_gate and ffn_up by swapping weight handle.
- [ ] **Step 29** [V]: cosine ≥0.9999 for both.
- [ ] **Step 30** [CODE+TEST]: site 7 (wo backward). Layer 0, shape `[seq, n_embd] @ [n_embd, q_out_dim_sliding=2048]`.
- [ ] **Step 31** [V]: cosine ≥0.9999.
- [ ] **Step 32** [C]: commit (sites 4, 5, 7 verified).
- [ ] **Step 33** [CODE+TEST]: sites 9 + 10 (wk, wv backward). Layer 0, shape `[seq, kv_out_dim_sliding=256] @ [kv_out_dim, n_embd]`.
- [ ] **Step 34** [V]: cosine ≥0.9999.
- [ ] **Step 35** [CODE+TEST]: site 12 (reconstruct_kv_pre_norm) — already covered by analogy with `matmul_xwt`. Spot-check with `test_gpu_site_12_kv_reconstruct` on layer 0 wk shape.
- [ ] **Step 36** [V]: cosine ≥0.9999.
- [ ] **Step 37** [CODE+TEST]: sites 13 + 14 (PLE proj + inp_gate backward). Shapes `[seq, n_embd=1536] @ [n_embd, n_epl=256]` for proj-grad and `[seq, n_epl] @ [n_epl, n_embd]` for inp_gate-grad.
- [ ] **Step 38** [V]: cosine ≥0.9999 for both.
- [ ] **Step 39** [C]: commit (sites 9, 10, 12, 13, 14 verified).
- [ ] **Step 40** [V]: **PHASE 1 EXIT GATE** — every shape in the migration table has a passing GPU vs CPU equivalence test. Otherwise no migration, escalate the failing site.

**Per-site fallout response**: if any cosine drops below 0.9999, do NOT modify the production function for that site. Instead [ESCALATE]: capture inputs/outputs in a saved file, write a minimal repro test, post to Stevie. The shape derivation (transpose flags, leading dims) needs human review.

---

## PHASE 2: SIGNATURE CHANGE (Steps 41-55) — ~30 min

**Goal**: thread an optional `&Gemma4GpuWeights` parameter through `backward_gemma4_with_lora` and its callers. No semantic change yet — pass `None` everywhere, expect bit-identical behavior to baseline.

- [ ] **Step 41** [CODE]: in `src/gemma4.rs`, change `pub fn backward_gemma4_with_lora` signature to add a new param:
  ```rust
  pub fn backward_gemma4_with_lora(
      weights: &CpuWeightsGemma4,
      gpu: Option<&crate::gemma4_gpu::Gemma4GpuWeights>,  // NEW
      mut lora: Option<&mut Gemma4LoraAdapters>,
      caches: &[Gemma4LayerCache],
      logits: &[f32],
      tokens: &[u32],
      answer_start: usize,
  ) -> (f32, Vec<LayerGradHealth>)
  ```
- [ ] **Step 42** [CODE]: update `backward_gemma4` wrapper (same file) to pass `None` for the new `gpu` param:
  ```rust
  backward_gemma4_with_lora(weights, None, None, caches, logits, tokens, answer_start)
  ```
- [ ] **Step 43** [CODE]: in `src/gemma4_gpu.rs::train_step_gemma4_hybrid`, update the call to also pass `None` (we'll flip to `Some(gpu)` in Phase 4):
  ```rust
  let (loss, _health) = backward_gemma4_with_lora(
      cpu, None, Some(lora), &caches, &logits, tokens, answer_start);
  ```
- [ ] **Step 44** [BUILD] ~30s: `cargo build --release`. Expect compile errors at remaining call sites.
- [ ] **Step 45** [B] ~5s: `cargo build --release 2>&1 | grep -E "expected.*arguments|missing argument" | head -10` — list remaining bad call sites.
- [ ] **Step 46** [CODE]: fix each call site (the test functions in `src/gemma4.rs::tests`):
  - `test_gemma4_backward_grad_health`: `backward_gemma4(...)` already wraps; no change.
  - `test_gemma4_lora_grad_health`: passes `Some(&mut lora)` — also add `None` for new gpu param.
  - any other compile-error sites flagged by Step 45.
- [ ] **Step 47** [BUILD] ~30s: rebuild. Expect green.
- [ ] **Step 48** [V]: build green. If not [D] → fix remaining call sites.
- [ ] **Step 49** [C]: `git commit -m "[PLAN STEP C] Step 41-48: backward_gemma4_with_lora gains Option<&Gemma4GpuWeights>; callers pass None"`
- [ ] **Step 50** [TEST] ~3min: `cargo test --release --bin zhenai-forge test_gemma4_backward_grad_health -- --test-threads=1 --nocapture` — equivalent baseline.
- [ ] **Step 51** [V]: log shows `healthy=35/35 zero=0 nan=0`. Otherwise [ROLLBACK] to Phase 0.
- [ ] **Step 52** [TEST] ~6min: `cargo test --release --bin zhenai-forge test_gemma4_train_step_loss_descent -- --test-threads=1 --nocapture`.
- [ ] **Step 53** [V]: monotonic descent, final loss matches baseline within 0.5.
- [ ] **Step 54** [TEST] ~6min: hybrid still works — `cargo test --release --bin zhenai-forge test_gemma4_hybrid_train_step_loss_descent`.
- [ ] **Step 55** [V]: **PHASE 2 EXIT GATE** — signature changed, all baselines green. If any failed → [ROLLBACK] (`git revert HEAD`) and reconsider.

---

## PHASE 3: SITE-BY-SITE MIGRATION (Steps 56-130) — ~3 hours

**Goal**: replace each matmul site with a CPU/GPU dispatch. Verify after each cluster. The dispatch pattern at every site:

```rust
let result = if let Some(g) = gpu {
    g.matmul_grad_x(&g.<weight>[il], &<input>, m, n, k).expect("gpu matmul")
} else {
    let w_f32 = bf16_to_f32_vec(&weights.<weight>[il]);
    matmul_grad_x(&<input>, &w_f32, m, n, k)
};
```

Migrate in clusters by structural similarity. After each cluster: rebuild + run baseline tests with `gpu=None` (must stay green) AND with a quick `Some(gpu)` smoke check (correctness only, not speed).

### Cluster 3A: Globals (LM head + reconstruct rows lifted)

- [ ] **Step 56** [CODE]: site 1 (LM head backward). Find:
  ```rust
  let tok_embd_f32 = bf16_to_f32_vec(&weights.token_embd);
  let grad_final_hidden = matmul_grad_x(&grad_logits, &tok_embd_f32, seq, vocab, n_embd);
  ```
  Replace with the dispatch pattern using `g.token_embd`.
- [ ] **Step 57** [BUILD] ~30s: rebuild.
- [ ] **Step 58** [TEST] ~3min: `test_gemma4_backward_grad_health` (gpu=None path).
- [ ] **Step 59** [V]: still healthy=35/35.
- [ ] **Step 60** [C]: commit (site 1 migrated).

### Cluster 3B: FFN backward (sites 2, 3, 4, 5)

- [ ] **Step 61** [CODE]: site 2 — REPLACE the per-row inline `for s in 0..seq { let row_ffn_out = ... }` reconstruction with a single full-batch dispatch:
  ```rust
  let ffn_out_full = if let Some(g) = gpu {
      g.matmul_xwt(&g.ffn_down[il], &cache.ffn_hidden, seq, n_embd, h.n_ff).expect(...)
  } else {
      let ffn_down_f32 = bf16_to_f32_vec(&weights.ffn_down[il]);
      matmul_x_wt(&cache.ffn_hidden, &ffn_down_f32, seq, n_embd, h.n_ff)
  };
  ```
  Then in the per-row rmsnorm_backward loop, slice `&ffn_out_full[s*n_embd..(s+1)*n_embd]` instead of recomputing per row.
- [ ] **Step 62** [CODE]: site 3 (ffn_down backward). Dispatch on `g.ffn_down[il]`.
- [ ] **Step 63** [CODE]: site 4 (ffn_gate backward). Dispatch on `g.ffn_gate[il]`.
- [ ] **Step 64** [CODE]: site 5 (ffn_up backward). Dispatch on `g.ffn_up[il]`.
- [ ] **Step 65** [BUILD] ~30s: rebuild.
- [ ] **Step 66** [TEST] ~3min: backward grad-health (None path).
- [ ] **Step 67** [V]: still healthy=35/35. If divergent → [D]: bisect by reverting one site at a time.
- [ ] **Step 68** [C]: commit (cluster 3B done).

### Cluster 3C: Attention backward (sites 6, 7)

- [ ] **Step 69** [CODE]: site 6 — REPLACE the per-row inline `for s in 0..seq { /* reconstruct o_out row */ }` with a single full-batch dispatch:
  ```rust
  let o_out_full = if let Some(g) = gpu {
      g.matmul_xwt(&g.wo[il], &cache.attn_out, seq, n_embd, q_out_dim).expect(...)
  } else {
      let wo_f32 = bf16_to_f32_vec(&weights.wo[il]);
      matmul_x_wt(&cache.attn_out, &wo_f32, seq, n_embd, q_out_dim)
  };
  ```
  Then slice per-row in the rmsnorm_backward loop.
- [ ] **Step 70** [CODE]: site 7 (wo backward) — replace `bf16_to_f32_vec(&weights.wo[il])` and `matmul_grad_x(...)` with dispatch.
- [ ] **Step 71** [BUILD] + [TEST] ~3min.
- [ ] **Step 72** [V]: healthy=35/35 on None path.
- [ ] **Step 73** [C]: commit (cluster 3C).

### Cluster 3D: Q/K/V projection backward (sites 8, 9, 10)

- [ ] **Step 74** [CODE]: site 8 (wq backward) — dispatch on `g.wq[il]`.
- [ ] **Step 75** [CODE]: site 9 (wk backward, only when `weights.wk[il].is_some()`) — dispatch on `g.wk[il].as_ref().unwrap()`.
- [ ] **Step 76** [CODE]: site 10 (wv backward, with the wv-falls-back-to-wk Gemma 4 quirk) — dispatch on `g.wv[il].as_ref().unwrap_or(g.wk[il].as_ref().unwrap())` (guard the unwrap with the `weights.wk[il].is_some()` branch).
- [ ] **Step 77** [BUILD] + [TEST] ~3min.
- [ ] **Step 78** [V]: healthy=35/35.
- [ ] **Step 79** [C]: commit (cluster 3D).

### Cluster 3E: reconstruct helpers (sites 11, 12)

These are called from inside the backward loop. Two approaches:
- **(A)** thread `gpu: Option<&...>` into `reconstruct_q_pre_norm` and `reconstruct_kv_pre_norm` helpers.
- **(B)** inline the reconstruct bodies into the backward loop with the dispatch pattern, drop the helpers.

[DECIDE] — RECOMMENDATION (B): inline. The helpers are called once each per layer, body is small (single matmul + return), and inlining avoids lifetime/borrow gymnastics with the `gpu` param. Override only if call sites multiply later.

- [ ] **Step 80** [CODE]: at the call site of `reconstruct_q_pre_norm`, replace with inline dispatch (`matmul_xwt` for GPU path, `matmul_x_wt(input, bf16_to_f32_vec(weight))` for CPU). Delete the `reconstruct_q_pre_norm` function.
- [ ] **Step 81** [CODE]: same for `reconstruct_kv_pre_norm` (handle the wk-fallback for wv on Gemma 4).
- [ ] **Step 82** [BUILD] + [TEST] ~3min.
- [ ] **Step 83** [V]: healthy=35/35.
- [ ] **Step 84** [C]: commit (cluster 3E + helper deletion).

### Cluster 3F: PLE backward (sites 13, 14)

- [ ] **Step 85** [CODE]: site 13 (PLE proj backward) — dispatch on `g.proj[il]`.
- [ ] **Step 86** [CODE]: site 14 (PLE inp_gate backward) — dispatch on `g.inp_gate[il]`.
- [ ] **Step 87** [BUILD] + [TEST] ~3min: backward grad-health.
- [ ] **Step 88** [V]: healthy=35/35.
- [ ] **Step 89** [C]: commit (cluster 3F).

### Phase 3 cumulative verification

- [ ] **Step 90** [TEST] ~3min: lora grad-health — `cargo test test_gemma4_lora_grad_health`.
- [ ] **Step 91** [V]: `healthy=110/110 zero=0 nan=0`.
- [ ] **Step 92** [TEST] ~6min: full loss descent — `cargo test test_gemma4_train_step_loss_descent`.
- [ ] **Step 93** [V]: monotonic descent, final loss within 0.5 of baseline (6.78).
- [ ] **Step 94** [TEST] ~6min: hybrid (GPU forward + CPU backward, gpu=None) — `cargo test test_gemma4_hybrid_train_step_loss_descent`.
- [ ] **Step 95** [V]: hybrid still ~43s/step, loss within 0.5 of baseline.
- [ ] **Step 96** [V]: **PHASE 3 EXIT GATE** — all CPU-path tests pass, signatures threaded, every matmul site has dispatch. CPU baseline preserved bit-equivalent.

---

## PHASE 4: ACTIVATE GPU MODE IN HYBRID STEP (Steps 97-115) — ~30 min

**Goal**: flip `train_step_gemma4_hybrid` to pass `Some(gpu)` instead of `None` to `backward_gemma4_with_lora`. Verify loss still descends and grad health stays clean.

- [ ] **Step 97** [CODE]: in `src/gemma4_gpu.rs::train_step_gemma4_hybrid`, change the backward call:
  ```rust
  let (loss, _health) = crate::gemma4::backward_gemma4_with_lora(
      cpu, Some(gpu), Some(lora), &caches, &logits, tokens, answer_start);
  ```
- [ ] **Step 98** [BUILD] ~30s.
- [ ] **Step 99** [TEST] ~6min: `cargo test test_gemma4_hybrid_train_step_loss_descent -- --test-threads=1 --nocapture`. Capture wall time per step.
- [ ] **Step 100** [V]: loss descends monotonically; final loss within 0.5 of pure-CPU baseline (6.78); per-step time should drop noticeably (from 43s — anything ≤25s is excellent, ≤15s is target).
- [ ] **Step 101** [D] if loss diverges by more than 0.5: switch ONE migrated site at a time back to CPU (use a temporary local `let gpu_for_site_X = if cfg!(...) { gpu } else { None };`) and bisect which site introduces the drift. Likely culprit: shape derivation in a less-tested site (PLE backward most plausible).
- [ ] **Step 102** [D] if grad health regresses (NaN/Inf): same bisection, but the immediate suspect is the FFN cluster (3B) because the gate/up shapes are largest and most prone to a transpose mistake.
- [ ] **Step 103** [V]: **PHASE 4 INTERIM GATE** — full-GPU-backward path operational on real GGUF, loss descent verified.
- [ ] **Step 104** [C]: commit `[PLAN STEP C] Steps 97-103: backward_gemma4_with_lora now uses GPU when provided; hybrid step is full-GPU forward+backward`.

### Add a dedicated `train_step_gemma4_gpu` wrapper

Distinguish "all-GPU" from "hybrid (GPU fwd, CPU bwd)" so we can keep the latter as a fallback for debugging.

- [ ] **Step 105** [CODE]: add `pub fn train_step_gemma4_gpu(cpu, gpu, lora, tokens, answer_start, lr, step) -> Result<f32, String>` that's identical to `train_step_gemma4_hybrid` but explicitly named and documented as "all GPU".
- [ ] **Step 106** [CODE]: keep `train_step_gemma4_hybrid` as-is for now (it now also routes backward through GPU since both reference the same helpers — but the NAME is misleading). Add a deprecation doc-comment, defer rename to Phase 7.
- [ ] **Step 107** [CODE]: copy `test_gemma4_hybrid_train_step_loss_descent` into `test_gemma4_gpu_train_step_loss_descent` calling the new function. Both should pass identically — the new test is the canonical Phase 7 exit gate.
- [ ] **Step 108** [BUILD] + [TEST] ~6min.
- [ ] **Step 109** [V]: new test passes with same loss trajectory.
- [ ] **Step 110** [C]: commit `[PLAN STEP C] Steps 105-109: explicit train_step_gemma4_gpu + canonical loss descent test`.
- [ ] **Step 111** [TEST] ~6min: re-run full suite — `cargo test --release --bin zhenai-forge gemma4 -- --test-threads=1 2>&1 | tail -50`. All tests should be green.
- [ ] **Step 112** [V]: every gemma4 + gemma4_gpu test passes.
- [ ] **Step 113** [B]: `cargo test --release --bin zhenai-forge -- --test-threads=1 2>&1 | tail -10` — full crate test sweep, ensure non-gemma tests still pass.
- [ ] **Step 114** [V]: full crate green.
- [ ] **Step 115** [V]: **PHASE 4 EXIT GATE** — all-GPU train step proven correct, all tests green.

---

## PHASE 5: PERFORMANCE VERIFICATION (Steps 116-128) — ~1 hour

**Goal**: prove the per-step time meets the ≤15s target. If it doesn't, identify the bottleneck.

- [ ] **Step 116** [CODE]: extend `test_gemma4_gpu_train_step_loss_descent` to log per-step time AND total session time. Already done; check.
- [ ] **Step 117** [TEST] ~5-15min: cold-cache run — `sync && echo 3 | sudo tee /proc/sys/vm/drop_caches && cargo test test_gemma4_gpu_train_step_loss_descent -- --test-threads=1 --nocapture 2>&1 | tee /tmp/step-c-cold-time.log`
- [ ] **Step 118** [TEST] ~5-15min: warm-cache run — re-run immediately, capture `/tmp/step-c-warm-time.log`.
- [ ] **Step 119** [V] [DECIDE]: per-step warm time ≤15s → PASS, proceed to Phase 6.
  - If 15s < per-step ≤25s: PARTIAL WIN. Document limitation, proceed to Phase 6 (still useful), defer Phase 5.X optimization.
  - If per-step >25s: [D] profile.
- [ ] **Step 120** [D, conditional] if profile needed: enable `FORGE_GEMMA4_PROFILE=1` (extends the existing forward profile to backward — may need to add timers in `backward_gemma4_with_lora` similar to forward). Identify which step within the layer eats the most time.
- [ ] **Step 121** [D, conditional]: most likely bottleneck is per-call GpuBuffer alloc/free (each matmul allocates input/output buffers; 14 sites × 35 layers = 490 allocs per backward). Optimization: pre-allocate per-shape pool in `Gemma4GpuWeights`. Defer if speedup is acceptable without it.
- [ ] **Step 122** [V]: speed verdict captured in `/tmp/step-c-{cold,warm}-time.log`. Commit them via [W] copy into `crates/zhenai-forge/notes/wave10f-step-c-timing.md` for the record.
- [ ] **Step 123** [C]: commit timing notes.

### Extended descent (proves training is producing real updates, not just numerical noise)

- [ ] **Step 124** [TEST] ~10min: 10-step descent — write a temporary test `test_gemma4_gpu_10step_descent` that runs 10 training steps on the same fixed tokens. Loss should approach 0 monotonically.
- [ ] **Step 125** [V]: loss at step 10 < loss at step 1 by at least 10x.
- [ ] **Step 126** [V]: no NaN/Inf at any step.
- [ ] **Step 127** [C]: commit the 10-step test (mark `#[ignore]` so it doesn't run by default — it's slow).
- [ ] **Step 128** [V]: **PHASE 5 EXIT GATE** — speed within target (or documented as partial win), extended descent verified.

---

## PHASE 6: CLI INTEGRATION — STEP E (Steps 129-145) — ~30 min

**Goal**: wire the GPU train step into the `zhenai-forge train-gemma4` subcommand. Default to GPU; add `--cpu` opt-out.

- [ ] **Step 129** [CODE]: in `src/main.rs::cmd_train_gemma4`, add `--cpu` flag parsing alongside the existing args. Default `cpu_only = false`.
- [ ] **Step 130** [CODE]: after CpuWeightsGemma4::load, wrap the choice:
  ```rust
  let gpu_weights = if !cpu_only {
      Some(gemma4_gpu::Gemma4GpuWeights::upload(
          &weights, gemma4_gpu::PleMode::Cpu).map_err(|e| eprintln!("...")).ok())
  } else {
      None
  }.flatten();
  ```
- [ ] **Step 131** [CODE]: in the per-step training loop, dispatch:
  ```rust
  let loss = match &gpu_weights {
      Some(gpu) => gemma4_gpu::train_step_gemma4_gpu(
          &weights, gpu, &mut lora, example, ...
      ).expect("gpu step"),
      None => gemma4::train_step_gemma4(&weights, &mut lora, example, ...),
  };
  ```
- [ ] **Step 132** [CODE]: update the help text (early in `main.rs`) to advertise `--cpu` and note default is GPU.
- [ ] **Step 133** [BUILD] ~30s.
- [ ] **Step 134** [V]: build green.
- [ ] **Step 135** [B]: `~/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge 2>&1 | head -30` — confirm help text shows `--cpu`.
- [ ] **Step 136** [W]: create a tiny test data file `/tmp/gpu-cli-smoke.jsonl` with one line `{"tokens":[2,1000,2000,3000,4000,5000]}`.
- [ ] **Step 137** [B] ~5min: smoke test the CLI with GPU default — `~/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge train-gemma4 --model /var/zhen/models/gemma-4-E2B-it.gguf --data /tmp/gpu-cli-smoke.jsonl --steps 3 --lr 3e-3 --output /tmp/cli-test-lora.zlg4 2>&1 | tee /tmp/cli-gpu-run.log`
- [ ] **Step 138** [V]: run completes, loss descends, output file written, per-step time matches Phase 5 measurement.
- [ ] **Step 139** [B] ~7min: smoke test with `--cpu` — same command + `--cpu`.
- [ ] **Step 140** [V]: also descends, slower per-step.
- [ ] **Step 141** [B]: verify ZLG4 file readable — `ls -lh /tmp/cli-test-lora.zlg4 && file /tmp/cli-test-lora.zlg4`. Should be ~22 MB binary.
- [ ] **Step 142** [B]: cleanup — `rm /tmp/cli-test-lora.zlg4 /tmp/gpu-cli-smoke.jsonl`.
- [ ] **Step 143** [C]: commit `[PLAN STEP C] Steps 129-142: train-gemma4 CLI defaults to GPU, --cpu fallback`.
- [ ] **Step 144** [V]: full test suite still green — `cargo test --release --bin zhenai-forge -- --test-threads=1 2>&1 | tail -5`.
- [ ] **Step 145** [V]: **PHASE 6 EXIT GATE** — CLI works with default GPU + `--cpu` opt-out, all tests green.

---

## PHASE 7: DOCS + SESSION LOG (Steps 146-160) — ~30 min

**Goal**: persist the Step C completion record per the persist-plans-to-repo memory rule. Update CLAUDE.md, session log, plan doc.

- [ ] **Step 146** [CODE]: rename `train_step_gemma4_hybrid` → `train_step_gemma4_gpu_old` in `src/gemma4_gpu.rs` (or just delete it and update its test — `train_step_gemma4_gpu` is now the canonical entry). [DECIDE] — RECOMMENDATION: delete `train_step_gemma4_hybrid` and its test; the name is now stale (it's no longer hybrid since backward is GPU too). Override only if Stevie wants the function preserved.
- [ ] **Step 147** [CODE]: same for `test_gemma4_hybrid_train_step_loss_descent` — delete (replaced by `test_gemma4_gpu_train_step_loss_descent`).
- [ ] **Step 148** [BUILD] + [TEST]: green.
- [ ] **Step 149** [C]: commit cleanup.
- [ ] **Step 150** [W]: update `crates/zhenai-forge/notes/wave10f-session-2026-04-17.md` final outcome section with Step C results, commit count, per-step times, link to this battle plan as the executed playbook.
- [ ] **Step 151** [W]: update `CLAUDE.md` WAVE10F entry: change "GPU port: pending" → "GPU port: done, train-gemma4 defaults to GPU, ~Xs/step" with Phase 5 measured time.
- [ ] **Step 152** [W]: append a **STEP C DONE** banner to the top of `crates/zhenai-forge/notes/wave10f-step-c-battle-plan.md` (this file) with execution date, commits, results, and lessons learned. Future readers get the outcome before the plan body.
- [ ] **Step 153** [W]: update `crates/zhenai-forge/notes/wave10f-gpu-port-plan.md` → mark Steps A through E as DONE with commit refs.
- [ ] **Step 154** [W]: update `docs/battle-plans/WAVE10F-FORGE-REAL-ATTENTION-GEMMA4.md` Phase 7 row → done.
- [ ] **Step 155** [C]: commit doc updates.
- [ ] **Step 156** [B]: `cd ~/tmp/unheaded && git log --oneline -25 | head -25` — final commit log review.
- [ ] **Step 157** [V]: every step in this plan has been executed or marked `[STUCK]`.
- [ ] **Step 158** [B]: `git status` — working tree clean.
- [ ] **Step 159** [V]: clean working tree, all commits land on main.
- [ ] **Step 160** [V]: **PHASE 7 EXIT GATE** — Step C complete, docs in sync, repo durable.

---

## ROLLBACK PROCEDURE (per cluster, 3A through 3F)

If any cluster's verification fails (Steps 59, 67, 72, 78, 83, 88) AND a quick [D] doesn't resolve it within 30 min:

1. `git log --oneline -5` to identify the most recent cluster commit.
2. `git revert HEAD` (creates a clean revert commit, preserves history).
3. Re-run the baseline tests (`test_gemma4_backward_grad_health`).
4. Verify back to green.
5. Capture the broken state in a `/tmp/step-c-failed-cluster-N.txt` notes file.
6. [ESCALATE] — describe to Stevie which site failed, what the symptom was, and what was tried.

DO NOT proceed past a failed cluster. The cumulative migration depends on each cluster's correctness.

---

## EMERGENCY: GPU PAGE FAULT (per Phase 1 prior precedent)

If any GPU matmul triggers `Memory access fault by GPU node-1`:

1. The error is almost always wrong `lda`/`ldb`/`ldc` or wrong transpose flag.
2. Drop into the existing per-site test (Phase 1 test_gpu_site_N) and reproduce in isolation.
3. Compare against the working `wq_matmul` and `matmul_grad_x` patterns documented inline in `gemma4_gpu.rs`.
4. The documented hipBLAS col-major translation is:
   - matmul_xwt (input @ w^T): `transa=true, transb=false, M_h=n, N_h=m, K_h=k, lda=k, ldb=k, ldc=n`
   - matmul_grad_x (grad_out @ w): `transa=false, transb=false, M_h=k, N_h=m, K_h=n, lda=k, ldb=n, ldc=k`
5. If a site's shape doesn't fit either pattern, [ESCALATE].

---

## TIMING SCAFFOLD (for Phase 5 [D] step 120)

If full-step time exceeds 25s, add per-bucket timing to `backward_gemma4_with_lora` matching the existing `ProfTimes` struct in `forward_gemma4_with_lora`. Buckets:

- `t_lm_head_grad`
- `t_ffn_recon` (site 2)
- `t_ffn_grad` (sites 3, 4, 5)
- `t_attn_recon` (site 6)
- `t_attn_grad` (sites 7-10)
- `t_qkv_recon` (sites 11, 12)
- `t_ple_grad` (sites 13, 14)
- `t_softmax_bwd`
- `t_rope_bwd`
- `t_norms_bwd`
- `t_lora_accum`

Enable with `FORGE_GEMMA4_PROFILE=1` (same env var as forward profile). Run one training step, eyeball the report, identify the dominant bucket. Most likely answer: per-call GpuBuffer alloc — fix is buffer pool, deferred to Phase 7c.

---

## DEFINITION OF DONE (Step C overall)

- [ ] All 14 matmul sites in `backward_gemma4_with_lora` have CPU/GPU dispatch
- [ ] `backward_gemma4_with_lora` signature is `(cpu, gpu: Option<...>, lora, caches, logits, tokens, answer_start)`
- [ ] `backward_gemma4` wrapper unchanged externally (passes None internally)
- [ ] All gemma4 + gemma4_gpu tests pass on real GGUF
- [ ] `train_step_gemma4_gpu` exists and is the canonical full-GPU entry point
- [ ] Per-step time on real GGUF ≤15s warm cache (PASS) or documented as partial win
- [ ] `test_gemma4_gpu_train_step_loss_descent` confirms loss descends with full GPU path, final within 0.5 of CPU baseline
- [ ] `zhenai-forge train-gemma4` CLI defaults to GPU, `--cpu` flag forces CPU fallback
- [ ] Session log + CLAUDE.md + GPU port plan updated to reflect completion
- [ ] STEP C DONE banner appended to top of this file

---

*WAVE10F Phase 7 Step C Battle Plan — Forged 2026-04-18*
*7 Phases. 160 Steps. The backward path joins forward on the GPU, completing the WAVE10F vision.*
*Forge fast. Verify everything. Skip what's stuck. Commit what's verified. Let the silicon work.*
