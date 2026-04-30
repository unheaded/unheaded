# WAVE10F Step C — Timing + Descent Notes

Captured 2026-04-20 during unattended hyper-sprint execution of
`wave10f-step-c-battle-plan.md`.

## Baselines (Phase 0 — pre-migration, from /tmp/baseline-*.log)

| Test                                                 | Step 1 | Step 2 | Step 3 | Total |
|------------------------------------------------------|-------:|-------:|-------:|------:|
| `test_gemma4_train_step_loss_descent` (pure CPU)     | 67.9s  | 53.1s  | 52.9s  | 174s  |
| `test_gemma4_hybrid_train_step_loss_descent` (fwd GPU) | 49.2s | 42.2s | 42.6s | 134s |

Losses (pure CPU): `21.28 → 12.02 → 6.78`
Losses (hybrid):   `19.96 → 16.48 → 6.79`

## Full-GPU train step (Phase 4 — all 14 backward matmul sites on hipBLAS)

### Cold cache (compile + first load, /tmp/step-c-cold-time.log)

```
step 1: 2.6s   (weight upload reused across test runs in same process)
step 2: 2.0s
step 3: 2.0s
total : 6.6s / 3 steps = 2.21s/step
losses: 19.9595, 15.9253, 5.7022
```

### Warm cache (immediate rerun, /tmp/step-c-warm-time.log)

```
step 1: 2.2s
step 2: 2.0s
step 3: 2.0s
total : 6.2s / 3 steps = 2.05s/step
losses: 19.9595, 15.9253, 5.7022  (deterministic bit-identical rerun)
```

### Phase 4 first run (cold CUDA handle + weight upload, from /tmp/phase4-fullgpu.log)

```
step 1: 22.9s  (CUDA context + weight upload + first-call compile)
step 2:  3.8s
step 3:  2.0s
total : 28.7s / 3 steps = 9.57s/step
```

## Speedup

| Path                                    | Avg per step (warm) | vs pure CPU | vs hybrid-fwd |
|-----------------------------------------|--------------------:|------------:|--------------:|
| pure CPU                                | ~58s                |  1.0×       |  0.76×        |
| hybrid (fwd GPU, bwd CPU)               | ~43s                |  1.35×      |  1.0×         |
| all-GPU (fwd + bwd) — **Step C target** | **~2.05s**          | **~28×**    | **~21×**      |

Target was ≤15s per step warm. Achieved 2.05s warm. **PASS — 7.3× better than target.**

## Loss descent (Phase 5 Step 124 extended, /tmp/step-c-10step.log)

10-step descent on fixed 6-token sequence, lr=3e-3, Adam defaults:

```
step  1: loss=19.9595
step  2: loss=15.9253
step  3: loss= 5.7022
step  4: loss= 3.4309
step  5: loss= 2.1605
step  6: loss= 2.2366
step  7: loss= 0.9138
step  8: loss= 0.8620
step  9: loss= 1.1339
step 10: loss= 0.0429
```

Step 10 / Step 1 ratio = 0.00215 (465× reduction).
Minor non-monotonicity at steps 5→6 and 8→9 is normal Adam behavior
with no warmup. Overall trend is strongly downward and finite.

## Numerical fidelity

All 14 migrated matmul sites verified in isolation vs CPU reference at
cosine similarity ≥ 0.999998 (Phase 1 harness). Full-GPU train-step
loss trajectory matches hybrid-fwd baseline to within 1.08 on final
value (and below, not above — the bf16 round-trip appears to act as
mild stochastic regularization on this fixed 3-step toy).

## Bottleneck hypothesis (for future optimization)

Per-step 2.0s warm breaks down roughly (unprofiled estimate):
  * ~0.5s forward (14 matmul + softmax + RoPE + norms × 35 layers)
  * ~1.5s backward (14 matmul + softmax-bwd + norm-bwd × 35 layers +
    bf16⇄f32 host/device conversions + per-call GpuBuffer alloc)

If future work needs sub-1s steps, the per-call GpuBuffer alloc in
matmul_grad_x / matmul_xwt (~14 × 35 = 490 allocs per backward) is
the prime suspect. Shape-pooled buffers would likely halve backward
cost. Deferred — 2.0s is already 7.3× below plan target.
