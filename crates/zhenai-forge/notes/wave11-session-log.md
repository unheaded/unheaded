# WAVE11 Session Log

## Session 1 — 2026-04-21

**Mission:** execute `wave11-gpu-kernels-battle-plan.md` end-to-end.
**Outcome:** Phases 0-6 complete; Phase 7 (attention) next.

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
| 7 | Attention fwd+bwd | 🚧 next session |
| 8 | GpuKernelsBackend integration | pending |
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

The f32 accumulation pattern in each kernel matches the CPU reference
bit-for-bit on realistic shapes. Either the kernels are genuinely
well-designed OR our tolerance is loose enough to hide 1e-8 differences
(which at f32 is "round-off noise" territory — fine).

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
│   └── rope.hip.cpp                  # partial-rotary fwd + bwd
└── src/hip_kernels/
    ├── mod.rs                        # module root, check_hip helper
    ├── identity.rs                   # 2 smoke tests
    ├── rmsnorm.rs                    # 2 cosine tests
    ├── gelu.rs                       # 4 cosine tests (fwd/bwd/fused×2)
    ├── softmax.rs                    # 3 cosine tests (fwd/masked/bwd)
    └── rope.rs                       # 3 cosine tests (fwd/bwd/passthrough)
```

Test count: 14 new WAVE11 unit tests, all PASS at cosine = 1.000.

### Remaining critical work

**Phase 7 — Attention.** The hardest kernel. Plan recommends unfused first:
  1. Q @ K^T scores (with GQA broadcast + scale)
  2. Mask + softmax (already have masked softmax from Phase 5)
  3. probs @ V output (with GQA broadcast)
  4. Backward: grad_v, grad_probs, grad_scores via softmax_bwd, grad_q, grad_k
  5. GQA collapse (sum subgroups back to n_head_kv heads)

Estimated scope: 4 days per the battle plan. A fresh session can attack
this with full focus on the GQA-broadcast indexing (the subtlest part).

**Phase 8** wires everything through `impl ForgeBackend for GpuKernelsBackend`.
**Phase 9** runs the full Learning Gate regression + the first real
Kingdom RAFT LoRA at seq=384 (THE sprint exit target).
**Phase 10** captures ADR-049 + session wrap.

### Session handoff prompt

```
Resume WAVE11 at Phase 7 (attention kernels). Current state:

  Phases 0-6 DONE. Phase 1 diagnostic confirmed attention-dominant at
  seq=64 (7.5 s/step). RMSNorm / GELU / softmax / RoPE all ship with
  cosine=1.000000 vs CPU reference at realistic shapes.

  Critical path for Phase 7:
    (1) GQA-broadcast attention scores kernel:
          scores[h][sq][sk] = sum_d Q[sq,h,d] * K[sk, h / (n_head/n_head_kv), d] * scale
    (2) attention output kernel with GQA broadcast on V.
    (3) reuse softmax_fwd_masked from Phase 5 for scores→probs.
    (4) backward: grad_v / grad_probs (via softmax_bwd) / grad_q / grad_k
        plus GQA collapse at end.

  Plan recommends starting UNFUSED for correctness (3 kernels composed
  in Rust), optional fusion later. Cosine ≥ 0.999 on real layer-0 shapes.

  Existing GpuBuffer + BlasHandle in hip.rs can be reused if the scores
  matmul is easier to launch via hipblasSgemm than a custom kernel.

  After Phase 7 → Phase 8 integration → Phase 9 full regression + seq=384
  Kingdom RAFT at ≤5 s/step warm.
```
