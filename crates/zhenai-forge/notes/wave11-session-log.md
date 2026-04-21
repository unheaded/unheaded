# WAVE11 Session Log

## Session 1 — 2026-04-21

**Mission:** execute `wave11-gpu-kernels-battle-plan.md` end-to-end.
**Outcome:** Phases 0-7 complete (all kernels shipped); Phase 8 integration next.

### Phase outcomes

| Phase | Name | Status | Commit |
|------:|------|:------:|--------|
| 0 | Preflight + env | ✅ DONE | (no commit — setup only) |
| 1 | seq=64 RAFT diagnostic | ✅ DONE | `5ceb3ca0` |
| 2 | HIP FFI scaffolding + identity kernel | ✅ DONE | (identity commit) |
| 3 | RMSNorm fwd+bwd | ✅ DONE | (cosine=1.000000) |
| 4 | GELU × 4 variants | ✅ DONE | (cosine=1.000000) |
| 5 | Softmax fwd + masked + bwd | ✅ DONE | (cosine=1.000000) |
| 6 | RoPE partial-rotary fwd+bwd | ✅ DONE | (cosine=1.000000) |
| 7 | Attention fwd+bwd (4 grad kernels + E2E) | ✅ DONE | **21 kernel tests cosine=1.000** |
| 8 | GpuKernelsBackend integration | 🚧 next session |
| 9 | Regression + RAFT retry at seq=384 | pending |
| 10 | Docs + ADR-049 + handoff | pending |

### Key empirical findings this session

**Phase 1 diagnostic (seq=64 RAFT on HybridMatmulBackend):**
- Warm step-time: **7.51 s/step** stable
- Loss descends: 21.36 → 8.33 (61% drop) over 200 steps
- No NaN, no GPU starvation
- Conclusion: attention IS dominant at long sequences, but there's other
  linear overhead too. GpuKernelsBackend should yield 3-5× speedup at
  seq=384, not 100×. Plan calibrated accordingly.

**Every kernel so far: cosine = 1.000000 vs CPU reference.**
- RMSNorm fwd+bwd on real shapes (rows=4, d=1536)
- GELU × 4 variants (including fused gelu*up)
- Softmax fwd + masked + bwd (causal mask zeros clean)
- RoPE partial-rotary fwd+bwd (passthrough dims unchanged)
- **Attention: 7 kernels (scores_fwd, output_fwd, E2E vs CPU, grad_v, grad_probs, grad_q, grad_k)** —
  GQA-broadcast built into all of them via `h_kv = h / (n_heads / n_kv_heads)`. No atomics.
  Backward kernels write to per-(sk, h_kv, d) addresses with iteration
  over sq AND group-head, giving unique writes per destination.

The f32 accumulation pattern in each kernel matches the CPU reference
bit-for-bit on realistic shapes. **21 WAVE11 kernel tests landed in
one session. Every single one at cosine=1.000000.**

### Engineering decisions pre-seeded by plan, confirmed by execution

1. **Hand-written HIP C++ via hipcc**. Build.rs discovers `kernels/*.hip.cpp`,
   produces `libwave11_kernels.so` in `$OUT_DIR`, cargo links. No new crate
   deps (avoids ADR-004 friction). Clean, debuggable, matches existing
   hip.rs FFI pattern.

2. **Static link, not dlopen**. Originally the plan specified libloading
   + runtime `dlopen`. Amendment in Phase 2 committed: plain extern "C" FFI
   to a statically-linked `.so` is simpler and doesn't add a dep.

3. **Correctness-first**. Every kernel has a cosine test vs its CPU
   reference with threshold 0.9999. All hitting 1.000000 so far.

### What's in the repo now

New files:
```
crates/zhenai-forge/
├── build.rs                          # extended with hipcc kernel build
├── kernels/
│   ├── common.hip.hpp                # bf16 helpers, block_reduce_{sum,max}
│   ├── identity.hip.cpp              # smoke-test kernel
│   ├── rmsnorm.hip.cpp               # fwd + bwd
│   ├── gelu.hip.cpp                  # fwd + bwd + fused gelu*up
│   ├── softmax.hip.cpp               # fwd + masked + bwd
│   ├── rope.hip.cpp                  # partial-rotary fwd + bwd
│   └── attn.hip.cpp                  # scores, output, grad_v, grad_probs, grad_q, grad_k
└── src/hip_kernels/
    ├── mod.rs                        # module root, check_hip helper
    ├── identity.rs                   # 2 smoke tests
    ├── rmsnorm.rs                    # 2 cosine tests
    ├── gelu.rs                       # 4 cosine tests (fwd/bwd/fused×2)
    ├── softmax.rs                    # 3 cosine tests (fwd/masked/bwd)
    ├── rope.rs                       # 3 cosine tests (fwd/bwd/passthrough)
    └── attn.rs                       # 7 cosine tests (fwd×3 + bwd×4)
```

Test count: 21 new WAVE11 unit tests, all PASS at cosine = 1.000.

### Remaining critical work

**Phase 8 — GpuKernelsBackend integration.** Wire every kernel through
`impl ForgeBackend for GpuKernelsBackend`. Design includes:

  1. **`GpuKernelsHandle`**: holds `Gemma4GpuWeights` (existing) + a new
     `ActivationPool` pre-allocating per-layer GPU buffers at `max_seq`,
     + `RopeFreqsCache` mapping `(seq, rope_dim, freq_base)` → uploaded
     cos/sin pair.
  2. **`forward_one_layer_gpu(...)`**: replace each CPU op call in
     `forward_gemma4_with_lora` with the matching kernel launch from
     Phase 3-7, keeping activations GPU-resident.
  3. **`backward_one_layer_gpu(...)`**: mirror for backward.
  4. **Mask construction**: Rust builds an additive [n_heads, seq, seq]
     causal or sliding-window mask per layer; kernel reads it as-is.
  5. **GQA handling**: already baked into the attention kernels — just
     pass `n_heads` and `n_kv_heads`.

Plan estimate: 2 days. Entry point is `src/backend.rs` — add the new
struct + impl alongside `CpuBackend` and `HybridMatmulBackend`.

**Phase 9** runs full Learning Gate regression + Kingdom RAFT retry at
seq=384. Exit target: ≤5 s/step warm with eval descent on held-out.

**Phase 10** captures ADR-049 + session wrap.

### Session handoff prompt

```
Resume WAVE11 at Phase 8 (GpuKernelsBackend integration). Current state:

  Phases 0-7 DONE. 21 WAVE11 kernel tests ALL at cosine=1.000000.
  - RMSNorm (fwd+bwd)
  - GELU × 4 variants
  - Softmax (fwd + masked + bwd)
  - RoPE partial-rotary (fwd+bwd+passthrough)
  - Attention fwd (scores, output, E2E w/ causal mask)
  - Attention bwd (grad_v, grad_probs, grad_q, grad_k — all GQA-aware)

  softmax_bwd from Phase 5 composes with attn_grad_probs → attn_grad_q/k
  to complete the backward chain end-to-end.

  Critical path for Phase 8:
    (1) Design ActivationPool: [hidden, normed, q, k, v, attn_out,
        scores, probs, ffn_gate_pre, ffn_up_pre, ffn_hidden, logits] —
        size by max_seq=512, n_embd=1536, n_ff=6144, n_head=8,
        n_kv_heads=2, head_dim=256, vocab=262144.
    (2) GpuKernelsHandle = { gpu_weights, activations, rope_cache }.
    (3) forward_one_layer_gpu loops replace each matmul_x_wt/bf16→f32/
        rmsnorm/softmax/rope call with the corresponding kernel launch.
    (4) backward_one_layer_gpu — mirror.
    (5) impl ForgeBackend for GpuKernelsBackend uses these helpers.

  First milestone: layer-0-only forward test comparing against
  HybridMatmulBackend on real weights. Cosine ≥ 0.99 (slightly looser
  than per-kernel because of accumulated bf16 matmuls).

  Then Phase 9: full Learning Gate + first Kingdom RAFT LoRA at seq=384.
```
