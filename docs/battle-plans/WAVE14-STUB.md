# WAVE14 STUB — BackwardScratch + KV-cache

> **Conditional on Track A or Track C.** This stub activates when `docs/decisions/2026-04-29-track-call.md` locks Track A (forge-first) or Track C (twin-track). If Track B is locked, this stub is deferred to Age 4+ and may be revised before resumption.

**Drafted**: 2026-04-27 from Cowork-on-Macbook (Computermancer + Architect hats)
**Prerequisite**: WAVE13 closed (Phase 2 verdict logged, ADR-051 finalized, forge HTTP serve mode landed if Phase 2 = SHIP)
**Target deliverable**: forge generation latency at production-acceptable first-token TTFT (≤200ms warm) + sustained throughput (≥10 tok/sec) on Gemma-4 E2B with optional Kingdom LoRA, on the WEST/EAST GPU stack
**Estimated scope**: 8–12 phases, ~250–350 numbered steps (Warmonger pass converts this stub into the full plan)
**Estimated wall-clock**: 2–3 weeks of focused work, single agent

---

## North star

WAVE12 (ADR-050) made the forward chain GPU-resident — until backward needed the layer cache, which got downloaded 1:1 negating the savings. WAVE14 closes that loop with a `BackwardScratch` mirror of the forward path, then unlocks production-grade serving with a KV-cache.

Two milestones, in order:

1. **BackwardScratch** — keeps backward-pass tensors GPU-resident. Expected forward pre-step time savings: 3–5s on the WAVE12-baseline 10.2s/step warm. With this in place, training-step wall-clock drops to ~5s warm.
2. **KV-cache** — eliminates per-token re-running of the full prefix. Current generation: 0.4–1s/token at seq≈400. With KV-cache: target ≤50ms per new token after the first. Unlocks forge HTTP serve mode at production latency.

---

## Phase outline (high-level, awaiting Warmonger detailed pass)

### Phase 0 — Preflight + WAVE13 closure verification
- Confirm WAVE13 Phase 2 verdict logged
- Confirm ADR-051 status: Accepted
- Confirm WAVE12 ForwardScratch + matmul_xwt_gpu_in_out + HIP kernels regression-green
- Confirm GPU dev box (WEST or EAST) ready with HIP/ROCm 6.2 stack

### Phase 1 — BackwardScratch design
- Mirror ADR-050's ForwardScratch pattern for backward path
- New ADR-054 candidate: scope, GPU-residency invariants, kernel allocation
- Identify backward kernels that can be GPU-in-out vs forced CPU-bridge
- Profile current backward chain (add `WAVE14_PROFILE=1` toggle paralleling `WAVE12_PROFILE=1`)

### Phase 2 — BackwardScratch implementation
- `crates/zhenai-forge/src/backward.rs` (new) — `BackwardScratch` struct + lifecycle
- Wire into `lora_backward` + `rmsnorm_backward` + attention grad chain
- Kernel: `f32_to_bf16_grad` mirror, `add_f32_grad` mirror as needed
- Regression pass: training step times must DROP, not stay flat (Phase 5 of WAVE12 found 1:1 cancellation — must avoid here)

### Phase 3 — BackwardScratch integration test
- Re-run WAVE12 Kingdom RAFT 500 steps
- Compare per-step wall-clock vs WAVE12 baseline (10.2s warm)
- Target: ≤7s warm (saved 3+s); stretch ≤5s
- Eval Δ on held-out: should match WAVE12 (-14.32) ± 0.5 — backward correctness check

### Phase 4 — KV-cache design
- Per-layer KV tensor allocation in GPU memory
- Cache invalidation on prefix change vs append
- Memory budget audit (Gemma-4 E2B at seq≤1024 fits comfortably; document the budget)
- Architect ADR candidate: KV-cache eviction policy + multi-request memory pooling

### Phase 5 — KV-cache implementation
- `crates/zhenai-forge/src/generate.rs` enhanced: `--use-kv-cache` flag (default ON)
- Forward path branches: full-prefix (current) vs cached-prefix-plus-new-token
- Cache reuse across `generate-gemma4` invocations within the same process

### Phase 6 — Generation throughput benchmark
- Target: ≤200ms TTFT, ≥10 tok/sec sustained on Gemma-4 E2B + Kingdom LoRA
- Compare base vs +LoRA throughput (should be ≥95% of base)
- Document in `crates/zhenai-forge/notes/wave14-benchmark.md`

### Phase 7 — Forge HTTP serve mode (if Phase 6 hits target)
- `zhenai-forge serve --port 20100` becomes a real serving endpoint
- Per-request `lora=on/off` toggle (per accepted WAVE13 plan)
- Health endpoint at `/health`, readiness at `/ready`, metrics Prometheus-native
- Auth: pluggable per `pkg/auth/` framework
- Wotan registration on startup (optional, if Track A/C also wires service-mesh integration)

### Phase 8 — Zhen-inference cutover (per Track decision)
- If Track A: Zhen primary inference path swaps from llama.cpp/Mistral-7B to forge/Gemma-4+LoRA
- If Track C: Zhen has BOTH paths; per-query routing decision (e.g., Kingdom queries → forge; general → llama.cpp)
- Update `services/zhen-inference/` config, runbook, dashboard

### Phase 9 — ADR-054 + handoff doc
- ADR-054 finalized: WAVE14 BackwardScratch + KV-cache architecture
- Update CLAUDE.md Age 3 status (Track A) or Age 4 (Track C kicks Phase 8 to Age 4)
- Handoff doc for next session at `crates/zhenai-forge/notes/wave14-handoff.md`

---

## Risks tracked from WAVE12 lessons

| Risk | WAVE12 finding | Mitigation in WAVE14 |
|---|---|---|
| Forward-resident savings cancelled by backward downloads | Phase 5 confirmed 1:1 cancellation | Phase 2-3 explicitly profile and gate on actual wall-clock drop |
| Wishful "matmul-compute-dominated" hypothesis | Phase 3 falsified at 0.2% sgemm | Phase 1 starts with profile toggle; budget set on profile, not on belief |
| Memorization vs generalization | Learning Gate Exp 2 deferred | Phase 3 eval Δ must match WAVE12 within ±0.5 |
| HIP kernel correctness | WAVE11 cosine=1.000 baseline | Reuse the WAVE11 kernel test harness for any new kernels |

---

## Dependencies

- WAVE12 ForwardScratch + matmul_xwt_gpu_in_out + 2 new HIP kernels (`f32_to_bf16`, `add_f32`) — all on main as of `2f44309c`
- WAVE13 Phase 2 verdict — drives Phase 7+8 conditionally
- ADR-048 ForgeBackend trait — WAVE14 backend stays `HybridMatmulBackend` until Phase 6 demonstrates KV-cache wins, then potentially `GpuKernelsBackend`
- ADR-050 GPU-resident activations — WAVE14 builds the BackwardScratch counterpart

---

## Next action when Track A or C locks

1. Stevie reads `docs/decisions/2026-04-29-track-call.md`
2. If Track A or C → invoke Warmonger to convert this stub into a numbered-step plan (`docs/battle-plans/WAVE14-DETAILED.md`)
3. Add ADR-054 placeholder to `docs/adr/ADR-INDEX.md`
4. Begin Phase 0 on the next GPU dev box session

---

*WAVE14 stub forged 2026-04-27 from Cowork-on-Macbook. Activates on Track A or C lock.*
