# WAVE 10D — GPU BACKWARD + FIRST REAL TRAINING RUN BATTLE PLAN — 8 Phases, ~110 Steps

**Date**: 2026-04-11
**Sprint**: Wave 10D — GPU-accelerated backward pass + first real Mistral-7B Kingdom LoRA
**Prerequisite**: Wave 10C complete (`crates/zhenai-forge` 46/46 tests pass, 32-layer toy descent proof)
**Target**: Mistral-7B + LoRA rank-16 trained on RAFT data, loss decreases monotonically over 500 steps, A/B test shows Kingdom-specific improvement
**Estimated Duration**: 12-20 hours across 2-4 sessions
**Agent Strategy**: Solo (Coordinator on WEST — needs sudo for GPU + iterative debugging)
**Commit Cadence**: Every 4 steps
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

---

## VARIABLES

```
$PROJECT_ROOT  = git rev-parse --show-toplevel  (resolved Step 1)
$FORGE_DIR     = $PROJECT_ROOT/crates/zhenai-forge
$MODEL_PATH    = /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf
$TRAIN_DATA    = /var/zhen/raft_dataset_v4.jsonl
$TINY_DATA     = /tmp/tiny_train.jsonl  (20-100 examples for fast iteration)
$LORA_OUT      = /var/zhen/models/kingdom-v6-lora.gguf
```

---

## LEGEND

```
[B]            = Bash command
[V]            = Verification step (MUST pass to proceed)
[D]            = Debug step (only if prior step fails)
[W]            = Write/create file
[R]            = Read/inspect file
[CODE]         = Code implementation
[TEST]         = Test execution
[BUILD]        = Build/compilation
[GPU]          = GPU operation (requires VRAM available)
[BENCH]        = Benchmark / performance measurement
[C]            = Commit checkpoint
[GATE]         = Phase exit gate
[KILL-CHECK]   = K1-K4 evaluation
```

**Time tags**: `~30s`, `~2m`, `~5m` — wall-clock estimate. Stuck Protocol triggers at 3× value.

---

## PREFLIGHT HYPOTHESES

| # | Hypothesis | Step | Pass Condition |
|---|---|---|---|
| H1 | Wave 10C tests still pass on current main | 4 | 46/46 green |
| H2 | Mistral-7B GGUF loadable, ~5GB | 6 | `info` command runs |
| H3 | RAFT v4 dataset present | 8 | `wc -l` ≥ 15000 |
| H4 | GPU has ≥10GB free VRAM | 10 | `rocm-smi` reports |
| H5 | hipBLAS sgemm_ex round-trip works | 12 | existing test green |
| H6 | RAM has ≥10GB free for all-layer attention | 14 | `free -h` |

---

## PHASE 0: PREFLIGHT (Steps 1-15) ~30m

**Goal**: Verify environment, baseline tests, no regressions from Wave 10C.
**Agent**: Coordinator

- [ ] **Step 1** [B] ~30s: Resolve project root
  ```bash
  cd ~/tmp/unheaded && export PROJECT_ROOT=$(git rev-parse --show-toplevel) && echo $PROJECT_ROOT
  ```
- [ ] **Step 2** [B] ~30s: Confirm on main, clean
  ```bash
  cd "$PROJECT_ROOT" && git status && git branch --show-current
  ```
- [ ] **Step 3** [B] ~30s: Pull latest if behind
  ```bash
  git fetch && git status -uno | head -3
  ```
- [ ] **Step 4** [V][TEST] ~2m: **H1 GATE** — Wave 10C tests pass (46/46)
  ```bash
  cd "$PROJECT_ROOT/crates/zhenai-forge" && cargo test --release 2>&1 | tail -3
  ```
  - If pass → Step 5
  - If fail → STOP. Wave 10C regression — fix before proceeding.
- [ ] **Step 5** [B] ~30s: Verify GGUF exists and is readable
  ```bash
  ls -lh /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf
  ```
- [ ] **Step 6** [V] ~30s: **H2 GATE** — file exists, ~5GB
- [ ] **Step 7** [B] ~30s: Verify RAFT data
  ```bash
  ls -lh /var/zhen/raft_dataset_v4.jsonl && wc -l /var/zhen/raft_dataset_v4.jsonl
  ```
- [ ] **Step 8** [V] ~30s: **H3 GATE** — ≥15000 lines
- [ ] **Step 9** [B] ~30s: Check GPU VRAM
  ```bash
  rocm-smi --showmeminfo vram | head -10
  ```
- [ ] **Step 10** [V] ~30s: **H4 GATE** — ≥10GB free VRAM on GPU 0
- [ ] **Step 11** [B][TEST] ~1m: Run hipBLAS test
  ```bash
  cd "$PROJECT_ROOT/crates/zhenai-forge" && cargo test --release test_hipblas_sgemm 2>&1 | tail -5
  ```
- [ ] **Step 12** [V] ~30s: **H5 GATE** — sgemm test green
- [ ] **Step 13** [B] ~30s: Check RAM
  ```bash
  free -h | head -2
  ```
- [ ] **Step 14** [V] ~30s: **H6 GATE** — ≥10GB free
- [ ] **Step 15** [V][C][GATE] ~30s: **PHASE 0 EXIT GATE** — All preflight passed
  ```bash
  git add -A && git -c commit.gpgsign=false commit --allow-empty -m "[PLAN W10D] Phase 0 preflight passed" 2>&1 | tail -3
  ```

---

## PHASE 1: GPU MATMUL GRADIENT FUNCTIONS (Steps 16-35) ~3h

**Goal**: Add GPU sgemm wrappers for backward pass: `gpu_matmul_grad_input` (W^T @ grad).
**Agent**: Solo (TDD)

- [ ] **Step 16** [R] ~5m: Read existing `crates/zhenai-forge/src/hip.rs::sgemm_ex` signature
- [ ] **Step 17** [DESIGN] ~10m: Note transpose flags for backward case
  - Forward: `grad_C @ W^T = grad_input` (we want this — A=grad, B=W, transB=true)
  - Skip: `grad_W` (frozen base weights)
- [ ] **Step 18** [W][CODE] ~30m: Add `gpu_matmul_grad_input()` to `train.rs::GpuTrainer`
  - Input: `grad_output (n_pos × out_dim)`, `weight (out_dim × in_dim)` cached on GPU
  - Output: `grad_input (n_pos × in_dim)` = `grad_output @ weight`
  - Reuse: existing `cached_batched_matmul` pattern
- [ ] **Step 19** [W][TEST] ~15m: Add unit test `test_gpu_matmul_grad_input`
  - Compare GPU result against CPU `matmul_backward_a`
  - Tolerance: rel error < 1e-3 (f32 floor per Scientist warning)
- [ ] **Step 20** [B][TEST] ~2m: Run test
  ```bash
  cd "$PROJECT_ROOT/crates/zhenai-forge" && cargo test --release test_gpu_matmul_grad_input 2>&1 | tail -10
  ```
- [ ] **Step 21** [V] ~30s: Test passes
  - If fail → Step 21a [D] dump shapes, check transpose flags
- [ ] **Step 22** [W][CODE] ~30m: Add `gpu_attn_layer_backward()` for cached layers
  - Calls `gpu_matmul_grad_input` for each Q/K/V/O target
  - Returns `grad_normed` (sums of 4 contributions × 0.25 scale)
- [ ] **Step 23** [W][TEST] ~15m: Test against CPU `attn_only_layer_backward`
- [ ] **Step 24** [B][TEST] ~2m: Run test
- [ ] **Step 25** [V] ~30s: GPU and CPU agree within rel error 1e-3
- [ ] **Step 26** [BENCH] ~5m: Time GPU vs CPU on 4096-dim attention layer
  ```bash
  cargo test --release test_attn_backward_bench --nocapture 2>&1 | tail -10
  ```
- [ ] **Step 27** [V] ~30s: GPU is ≥5x faster than CPU on 4096 dim
  - If not → Step 27a [D] check upload/download overhead
- [ ] **Step 28** [C] ~30s: **COMMIT 1**
  ```bash
  git add -A && git -c commit.gpgsign=false commit -m "[PLAN W10D] Steps 16-28: GPU backward sgemm + tests"
  ```
- [ ] **Step 29** [W][CODE] ~30m: Plumb `gpu_attn_layer_backward` into `train.rs:826` chain rule
  - For layers in `[gpu_layer_offset..n_layers]`: use GPU
  - For layers in `[0..gpu_layer_offset]`: keep CPU `attn_only_layer_backward` with all_attn_*
- [ ] **Step 30** [B][BUILD] ~2m: Compile
  ```bash
  cargo build --release 2>&1 | tail -5
  ```
- [ ] **Step 31** [V] ~30s: Build clean
- [ ] **Step 32** [B][TEST] ~5m: Full test suite
  ```bash
  cargo test --release 2>&1 | tail -3
  ```
- [ ] **Step 33** [V] ~30s: **REGRESSION GATE** — 46/46 still pass + new GPU tests pass
- [ ] **Step 34** [C] ~30s: **COMMIT 2**
- [ ] **Step 35** [V][C][GATE] ~30s: **PHASE 1 EXIT GATE** — GPU backward path wired and tested

---

## PHASE 2: TINY-DATA DESCENT VALIDATION (Steps 36-55) ~2h

**Goal**: Train on 20 examples, prove loss decreases on REAL Mistral.
**Prerequisite**: Phase 1 GATE
**Agent**: Coordinator

- [ ] **Step 36** [B] ~30s: Create tiny dataset
  ```bash
  head -20 /var/zhen/raft_dataset_v4.jsonl > /tmp/tiny_train.jsonl && wc -l /tmp/tiny_train.jsonl
  ```
- [ ] **Step 37** [V] ~30s: 20 lines
- [ ] **Step 38** [B] ~10m: Tiny training run, observe loss trajectory
  ```bash
  ./target/release/zhenai-forge train --model /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf \
    --data /tmp/tiny_train.jsonl --output /tmp/test-lora.gguf --epochs 5 --lr 5e-3 --rank 4 2>&1 | tee /tmp/tiny.log | tail -30
  ```
- [ ] **Step 39** [V] ~30s: First step under 5s (was 46s before GPU backward)
  - If still slow → Step 39a [D] check GPU utilization with `rocm-smi`
- [ ] **Step 40** [V][KILL-CHECK] ~30s: **K4 KILL** — single step ≤ 5s
- [ ] **Step 41** [V][KILL-CHECK] ~30s: **K2 KILL** — no NaN, loss starts at ~11 not 20+
- [ ] **Step 42** [B] ~30s: Extract loss trajectory
  ```bash
  grep "Loss:" /tmp/tiny.log | head -20
  ```
- [ ] **Step 43** [V] ~30s: **DESCENT GATE** — loss decreases at least 5% from epoch 1 to epoch 5
  - If flat → Step 43a [D] check LR (try 1e-2), check gradient norms
  - If diverging → Step 43b [D] check chain rule shapes, gradient clipping
- [ ] **Step 44** [C] ~30s: **COMMIT 3**
- [ ] **Step 45** [B] ~5m: Try 100 examples for stronger signal
  ```bash
  head -100 /var/zhen/raft_dataset_v4.jsonl > /tmp/small_train.jsonl
  ./target/release/zhenai-forge train --model /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf \
    --data /tmp/small_train.jsonl --output /tmp/test-lora2.gguf --epochs 3 --lr 1e-3 --rank 16 2>&1 | tee /tmp/small.log | tail -20
  ```
- [ ] **Step 46** [V] ~30s: Loss decreases >10% across 3 epochs
- [ ] **Step 47** [B] ~30s: Save loss curves
  ```bash
  grep "Loss:" /tmp/small.log > /tmp/small-curve.txt && cat /tmp/small-curve.txt
  ```
- [ ] **Step 48** [W] ~5m: Document tiny+small descent in `crates/zhenai-forge/notes/wave-10d-descent.md`
- [ ] **Step 49** [C] ~30s: **COMMIT 4**
- [ ] **Step 50** [B] ~30s: Memory check
  ```bash
  free -h && rocm-smi --showmeminfo vram | head -10
  ```
- [ ] **Step 51** [V] ~30s: RSS ≤ 13GB, VRAM ≤ 11GB (within budget)
- [ ] **Step 52** [V][KILL-CHECK] ~30s: **K1 KILL** — no shape mismatches in last 500 lines of log
  ```bash
  grep -i "shape\|mismatch\|sgemm" /tmp/small.log | head
  ```
- [ ] **Step 53** [B] ~30s: Speed measurement
  ```bash
  grep "steps/s" /tmp/small.log | tail -5
  ```
- [ ] **Step 54** [V] ~30s: ≥0.2 steps/s sustained
- [ ] **Step 55** [V][C][GATE] ~30s: **PHASE 2 EXIT GATE** — Real Mistral descent confirmed on 20+100 examples

---

## PHASE 3: 500-STEP VALIDATION RUN (Steps 56-72) ~4h

**Goal**: Train kingdom-v6-lora.gguf for 500 steps on full RAFT v4. This is the real run.
**Prerequisite**: Phase 2 GATE
**Agent**: Coordinator (long-running, monitor periodically)

- [ ] **Step 56** [B] ~30s: Free GPU (kill llama-server, distill, docker if running)
  ```bash
  pgrep -af "llama-server\|distill" || echo "free"
  ```
- [ ] **Step 57** [B] ~30s: Calculate total step budget
  - 500 steps × 4 accum = 2000 sample passes
  - At 0.3 steps/s = ~28 minutes
- [ ] **Step 58** [B] ~30m+: Launch 500-step run with checkpointing
  ```bash
  ./target/release/zhenai-forge train --model /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf \
    --data /var/zhen/raft_dataset_v4.jsonl --output /var/zhen/models/kingdom-v6-lora.gguf \
    --epochs 1 --lr 1e-4 --rank 16 2>&1 | tee /tmp/v6-run.log
  ```
- [ ] **Step 59** [V] ~30s: Run started, no immediate crash
- [ ] **Step 60** [B] ~5m: Periodic check at step ~50
  ```bash
  grep "Loss:" /tmp/v6-run.log | tail -10
  ```
- [ ] **Step 61** [V] ~30s: Loss at step 50 ≤ 11 (still in expected range)
- [ ] **Step 62** [B] ~10m: Periodic check at step ~200
- [ ] **Step 63** [V] ~30s: Loss at step 200 ≤ 10 (decreasing)
- [ ] **Step 64** [B] ~20m: Wait for 500 steps
- [ ] **Step 65** [V] ~30s: Final loss ≤ 7 (Wave 10C battle plan target)
  - If 7-9 → ACCEPTABLE (still descent, just slower)
  - If >9 → Step 65a [D] check LR schedule, examine last 100 steps
- [ ] **Step 66** [B] ~30s: Verify checkpoint written
  ```bash
  ls -lh /var/zhen/models/kingdom-v6-lora.gguf
  ```
- [ ] **Step 67** [V] ~30s: kingdom-v6-lora.gguf exists, > 0 bytes
- [ ] **Step 68** [C] ~30s: **COMMIT 5**
- [ ] **Step 69** [W] ~10m: Save loss curve to `crates/zhenai-forge/notes/wave-10d-v6-curve.md`
- [ ] **Step 70** [B] ~30s: Sanity check trained vs base file size
  ```bash
  ls -lh /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf /var/zhen/models/kingdom-v6-lora.gguf
  ```
- [ ] **Step 71** [V] ~30s: LoRA file 50-300 MB (rank 16 expected size)
- [ ] **Step 72** [V][C][GATE] ~30s: **PHASE 3 EXIT GATE** — Kingdom-v6-lora trained, descent confirmed

---

## PHASE 4: A/B QUALITY TEST (Steps 73-85) ~2h

**Goal**: Compare base Mistral vs trained LoRA on 3 Kingdom questions.
**Prerequisite**: Phase 3 GATE; llama.cpp inference path
**Agent**: Coordinator

- [ ] **Step 73** [W] ~10m: Write 3 Kingdom test prompts to `/tmp/kingdom-prompts.txt`
  ```
  Q1: What port does Wotan listen on for gRPC?
  Q2: What is the layout of the Monad register (20 bytes)?
  Q3: What does ADR-043 establish?
  ```
- [ ] **Step 74** [B] ~5m: Generate base model answers
  ```bash
  for q in "Wotan gRPC port?" "Monad register layout?" "What is ADR-043?"; do
    echo "=== $q ==="
    llama-cli -m /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf -p "[INST] $q [/INST]" -n 200 2>&1 | tail -20
  done > /tmp/base-answers.txt
  ```
- [ ] **Step 75** [B] ~5m: Generate LoRA-loaded answers
  ```bash
  for q in "Wotan gRPC port?" "Monad register layout?" "What is ADR-043?"; do
    echo "=== $q ==="
    llama-cli -m /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf \
      --lora /var/zhen/models/kingdom-v6-lora.gguf -p "[INST] $q [/INST]" -n 200 2>&1 | tail -20
  done > /tmp/lora-answers.txt
  ```
- [ ] **Step 76** [V] ~30s: Both files exist and contain answers
  - If LoRA load fails → Step 76a [D] check GGUF compatibility, may need format conversion
- [ ] **Step 77** [R] ~10m: Read both files, score by hand
  - Q1 expected mention: 18001 (gRPC) or "18000/18001"
  - Q2 expected mention: 20 bytes, IPv6 HbH, register file
  - Q3 expected mention: Mímir's Law / Gleipnir / baseline / drift
- [ ] **Step 78** [DECIDE] ~10m: Score (0-3 per question, 9 total)
  - Base score: ___ / 9
  - LoRA score: ___ / 9
- [ ] **Step 79** [V] ~30s: **K3 KILL CHECK** — LoRA score must be ≥ base score
  - If LoRA < base → Step 79a [D] training degraded the model, investigate
- [ ] **Step 80** [V] ~30s: **QUALITY GATE** — LoRA score ≥ base + 1 on at least 2 questions
- [ ] **Step 81** [W] ~15m: A/B report at `crates/zhenai-forge/notes/wave-10d-ab-test.md`
- [ ] **Step 82** [C] ~30s: **COMMIT 6**
- [ ] **Step 83** [V] ~30s: Validate no Kingdom hallucination (e.g., "Wotan listens on port 80")
- [ ] **Step 84** [V] ~30s: Validate coherent English (no degenerate output)
- [ ] **Step 85** [V][C][GATE] ~30s: **PHASE 4 EXIT GATE** — A/B shows Kingdom improvement

---

## PHASE 5: INFERENCE INTEGRATION (Steps 86-95) ~2h

**Goal**: Wire trained LoRA into Champion's inference path so MCP serves Kingdom-fluent answers.
**Prerequisite**: Phase 4 GATE
**Agent**: Solo

- [ ] **Step 86** [R] ~10m: Read `pkg/champion/` to find current inference path
- [ ] **Step 87** [R] ~10m: Read `raft/zhen_mcp_server.py` to find inference call site
- [ ] **Step 88** [DESIGN] ~10m: Decide loading strategy
  - Option A: Champion loads model + LoRA at startup
  - Option B: MCP server proxies to llama-server with LoRA flag
  - **RECOMMEND Option B**: minimal change, llama.cpp already supports `--lora`
- [ ] **Step 89** [W][CODE] ~20m: Update llama-server launch script with `--lora` flag
- [ ] **Step 90** [B] ~5m: Restart llama-server with LoRA
- [ ] **Step 91** [B] ~2m: Smoke test via MCP corpus_search + inference
- [ ] **Step 92** [V] ~30s: MCP returns Kingdom-fluent response
- [ ] **Step 93** [W] ~10m: Update `wiki/Wave-10C-Backprop.md` with Wave 10D follow-up section
- [ ] **Step 94** [C] ~30s: **COMMIT 7**
- [ ] **Step 95** [V][C][GATE] ~30s: **PHASE 5 EXIT GATE** — Champion uses Kingdom LoRA via MCP

---

## PHASE 6: REGRESSION + STRESS (Steps 96-105) ~1h

**Goal**: Ensure nothing else broke; stress the inference path.
**Agent**: Solo

- [ ] **Step 96** [B][TEST] ~5m: Full forge test suite
  ```bash
  cd "$PROJECT_ROOT/crates/zhenai-forge" && cargo test --release 2>&1 | tail -3
  ```
- [ ] **Step 97** [V] ~30s: 46+ tests pass
- [ ] **Step 98** [B][TEST] ~5m: Full Go test suite
  ```bash
  cd "$PROJECT_ROOT" && go test ./... -count=1 -timeout 120s 2>&1 | tail -10
  ```
- [ ] **Step 99** [V] ~30s: All Go tests pass
- [ ] **Step 100** [B] ~10m: 100-query stress against MCP server
- [ ] **Step 101** [V] ~30s: No crashes, latency < 5s/query
- [ ] **Step 102** [V][KILL-CHECK] ~30s: **K2 KILL** — no NaN/divergence in inference
- [ ] **Step 103** [B] ~30s: GPU/RAM usage steady-state check
  ```bash
  rocm-smi && free -h
  ```
- [ ] **Step 104** [V] ~30s: No leaks, VRAM stable
- [ ] **Step 105** [V][C][GATE] ~30s: **PHASE 6 EXIT GATE** — Regression-free, stress-ready

---

## PHASE 7: DOC RIPPLE + FINAL COMMIT (Steps 106-115) ~1h

**Goal**: Doc web ripple via Librarian. Update CLAUDE.md, wiki, ADR.
**Agent**: Librarian / Solo

- [ ] **Step 106** [W] ~10m: Update `CLAUDE.md` "What's Done" with Wave 10D entry
- [ ] **Step 107** [W] ~10m: New page `wiki/Wave-10D-GPU-Training.md`
- [ ] **Step 108** [W] ~5m: Update `wiki/Wave-10C-Backprop.md` with "follow-up: Wave 10D" footer
- [ ] **Step 109** [W] ~5m: Update `wiki/_Sidebar.md` + `wiki/Home.md` with new page link
- [ ] **Step 110** [W] ~5m: Update `docs/adr/ADR-045-...md` status to "ACCEPTED — Shipped"
- [ ] **Step 111** [B] ~30s: Sync to wiki repo
  ```bash
  cp wiki/Wave-10D-GPU-Training.md wiki/_Sidebar.md wiki/Home.md ~/tmp/unheaded-wiki/
  ```
- [ ] **Step 112** [B] ~30s: Commit wiki repo
  ```bash
  cd ~/tmp/unheaded-wiki && git add -A && git -c commit.gpgsign=false commit -m "docs(wiki): Wave 10D GPU backward + first real training"
  ```
- [ ] **Step 113** [B] ~30s: Final main commit
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git -c commit.gpgsign=false commit -m "[PLAN W10D] Wave 10D complete: GPU backward + kingdom-v6 LoRA shipped"
  ```
- [ ] **Step 114** [B] ~30s: Show shipped state
  ```bash
  git log --oneline -5
  ```
- [ ] **Step 115** [V][C][GATE] ~30s: **PHASE 7 EXIT GATE** — All docs rippled, ADR status updated, wiki synced

---

## Appendix A: Emergency Procedures

### EMERGENCY 1: GPU OOM during chain rule
1. `rocm-smi` to confirm VRAM saturation
2. Reduce LoRA rank from 16 → 8
3. Reduce `n_loss_positions` in train.rs from 4 → 2
4. Reduce batch size to 1
5. If still OOM → STUCK, defer to smaller model

### EMERGENCY 2: Loss NaN
1. Check gradient clipping (must be ≤1.0 norm)
2. Lower LR by 10x
3. Verify no division by zero in RMSNorm (check `eps` value)
4. Inspect first NaN step's gradient magnitudes

### EMERGENCY 3: GPU sgemm shape mismatch
1. Print `m, n, k` for each call site
2. Verify transA/transB flags match the math
3. Compare against CPU reference output
4. If still wrong → STUCK, escalate to Computermancer

### EMERGENCY 4: A/B test shows LoRA WORSE than base
1. Confirm checkpoint loaded correctly (`llama-cli` log)
2. Verify training loss actually decreased
3. Try LoRA alpha adjustment (try 16 instead of 32)
4. Worst case: LoRA learned the wrong distribution → restart with different LR

### EMERGENCY 5: llama-server doesn't accept GGUF LoRA
1. Check llama.cpp version supports GGUF LoRA loading
2. If not → use llama-server's `--lora-base` and `--lora` flags
3. Worst case: convert GGUF LoRA → safetensors, use HF format

---

## Appendix B: Agent Assignment Matrix

| Phase | Agent | Time | Critical Path |
|---|---|---|---|
| 0 Preflight | Coordinator | 30m | yes |
| 1 GPU sgemm | Solo TDD | 3h | yes |
| 2 Tiny descent | Coordinator | 2h | yes |
| 3 500-step run | Coordinator | 4h | yes |
| 4 A/B test | Coordinator | 2h | yes |
| 5 Inference integration | Solo | 2h | yes |
| 6 Regression | Solo | 1h | yes |
| 7 Doc ripple | Librarian | 1h | yes |
| **TOTAL** | | **~15.5h** | |

Critical path: 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 (all sequential — each phase depends on the previous).

---

## Appendix C: Quick Reference

### Key file paths
| Path | Purpose |
|---|---|
| `crates/zhenai-forge/src/train.rs:826` | Backward loop chain rule call site |
| `crates/zhenai-forge/src/hip.rs::sgemm_ex` | GPU matmul with transpose |
| `crates/zhenai-forge/src/backward.rs::attn_only_layer_backward` | CPU reference |
| `pkg/champion/` | Champion harness (Phase 5) |
| `raft/zhen_mcp_server.py` | MCP inference proxy (Phase 5) |

### LoRA dimensions (Mistral-7B)
- n_embd = 4096
- n_layers = 32
- rank = 16 (production), 4 (tiny)
- alpha = 32
- per-layer params = 4 × (4096 × 16 + 16 × 4096) = 524288 = 524K
- total params (32 layers × 4 targets) = ~16.7M

### sgemm_ex transpose flags for backward
- `grad_input = grad_output @ W` → `sgemm_ex(noTrans, noTrans, ...)` if `W` is `(out, in)` row-major
- (Or reshape mentally: backward of `Y = X @ W^T` where `X = (n_pos, in_dim)`, `W = (out, in)` is `dX = dY @ W`)

### Hard kill criteria
- K1: GPU sgemm shape mismatch
- K2: Loss diverges/NaN after 100 steps
- K3: Trained LoRA < base on Kingdom questions
- K4: Single training step > 5s on cached layers

---

*Wave 10D Battle Plan — Forged 2026-04-11*
*8 phases. ~115 steps. Wave 10C proved the math. Wave 10D makes it real.*
*"GPU sgemm. Real Mistral. First Kingdom LoRA. Champion gets fluent."*
