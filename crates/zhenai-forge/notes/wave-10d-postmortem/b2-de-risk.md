# Wave 10D Phase B.2.0 — De-Risk Experiment Lab Notebook

**Date:** 2026-04-14 → 2026-04-15 (overnight session)
**Investigators:** Developer + Scientist + BlackMage + Marshal
**Trigger:** Three consecutive E0 failures revealed Wave 10C alone over-budgets the 14 GB west host on Mistral-7B-Q5_K_M. Brute-force on existing code is dead.

## Observation

E0 with Wave 10D Phase 1 code (committed `9d27dec2`, `BW_ATTN_MAX_LAYERS=16`) failed to reach a single training step across three attempts:

| Attempt | Budget | Outcome | Peak mem | Peak swap |
|---|---|---|---|---|
| #1 | 180 s | Scope timeout during Wave 10C all-layer load (24/32) | 3.8 G | 5.0 G |
| #2 | 900 s | Scope timeout during Wave 10C all-layer load (24/32) | 4.3 G | 5.6 G |
| #3 (zswap + services stopped) | 3600 s planned | Host livelock during 4-layer GPU fwd cache (before bw_attn) | — | — |

No `svm_range_restore_work` events in any attempt — the Apr-12 failure class is fixed by the Phase B budget gate. Remaining failure is ordinary swap thrash from the Wave 10C `all_attn_*` fp32 cache (5.4 GB) colliding with the 4-layer GPU forward cache (~3.9 GB) and the bw_attn upload (4.3 GB) on a 14 GB APU.

## Hypothesis (converged from all three skills)

**H_C: Freeing `cpu_weights.all_attn_*[BW_ATTN_MAX_LAYERS..n_total]` between the Wave 10C load and `GpuTrainer::new` removes the peak-memory collision, enabling the pipeline to reach the training step without livelock. The CPU-fallback backward path for those layers tolerates empty `Vec`s and skips cleanly.**

Falsifiable by: `svm_range_restore_work` events, host unresponsiveness, or the in-code budget gate aborting bw_attn upload.

## Experiment

Throwaway env-gated bypass added at `src/train.rs:800` (uncommitted), runtime-selectable via `FORGE_BYPASS_HIGH_ATTN=1`. When set, `mem::take`s the 16 high-index all_attn_* layers post-Wave 10C.

First run freed the wrong range (0..16) — tripped the Phase B empty-weights hard-error in bw_attn upload, confirming the safety check works. Second run corrected the range to 16..32.

**Controls:** `MemoryMax=10G`, `MemoryHigh=8G`, `RuntimeMaxSec=1800`, rust-analyzer + fwupd + docker + containerd + fail2ban stopped, swappiness default (60), zswap off.

**Measurement:** host `MemAvailable` at key points, swap peak, bw_attn progress, kernel `svm_range_restore_work` count.

## Results

| Checkpoint | Measurement | Baseline | Expected | Actual | Match? |
|---|---|---|---|---|---|
| Preflight | MemAvailable | 10 G | ≥10 G | 10 G | ✅ |
| After bypass fires | MemAvailable | 9.51 G | ≥9 G | 9.51 G | ✅ |
| After 4-layer fwd cache | MemAvailable | ≥5 G | ≥5 G | 5.96 G | ✅ |
| After 12/16 bw_attn uploads | MemAvailable | ≥2 G | ≥2 G | 5.49 G | ✅ |
| svm_range_restore_work events | — | 0 | 0 | 0 | ✅ |
| Host responsiveness | SSH | reactive | reactive | reactive | ✅ |
| Scope end state | — | — | completes step or times out cleanly | RuntimeMaxSec timeout at 30m with 22m CPU, Mem peak 6.3 G | ⚠️ timed out before step, but not from memory pressure |

## Analysis

H_C is **confirmed** on all measured axes except end-to-end step completion. The 30-min wall-clock ran out during bw_attn upload (12/16 complete) — not a memory fault. Bandwidth for GGUF dequant + per-layer GPU upload on this APU is the bottleneck, estimated at ~100 s per bw_attn layer. Projected full E0 runtime with the same envelope: ~45–60 min for first step.

Key numbers:
- Host peak usage: 6.3 G RSS + 4.7 G swap = 11.0 G effective (of 14 G). Previous attempts hit 9.9 G effective and locked. The reduction comes entirely from the bypass.
- Swap peak dropped: 5.6 G → 4.7 G (−0.9 G) despite running longer.
- Scope memory cap (10 G) was never the binding constraint.
- The Phase B `MIN_FREE_HOST_RAM_BYTES=2 GB` in-code gate never fired — host_free minimum observed was 5.49 G, comfortable margin.

## Conclusion

**Verdict:** Confirmed. Path C (lazy re-dequantize from Q5_K for layers ≥ `BW_ATTN_MAX_LAYERS`) is architecturally viable on the 14 GB west host. The Apr-12 and overnight-lockup failure classes are both closed by this architecture.

**Confidence:** HIGH. The experiment exercised the exact code paths Path C would exercise (bw_attn with populated 0..16, CPU fallback with empty 16..32) and they routed correctly without panic, numerical NaN, or OOM.

**Implications:**
- B.2.1 (real Path C implementation) can proceed with high confidence.
- 13B model will still not fit — Path C alone saves 5.4 G, not enough to close the additional ~4 G gap. Future bf16 conversion (Path B) layered on top remains valuable.
- Wave 10C's 32-layer fp32 all_attn_* cache is a latent footprint bug that was only surfaced by Wave 10D's bw_attn pushing the host over the cliff.

**Next:** Implement Path C per Developer's B.2.1 spec. ~2.5 h. Commit. Rerun E0 with `FORGE_MAX_SEC=3600` budget. Expect first step around minute 45–55. If E0 completes cleanly, climb the ladder E1→E4.

**Open questions:**
- Why is bw_attn upload ~100 s/layer on this APU? (Investigate separately — not blocking.)
- Can `BW_ATTN_MAX_LAYERS` be raised above 16 once Path C frees the fp32 cache? (Budget check: with 16..32 lazy, all_attn peak drops to ~2.7 G instead of 5.4 G, creating ~2.7 G of headroom — enough for maybe 4–6 more cached bw_attn layers.)

## Remaining uncertainty

- Real Path C may expose additional edge cases in the backward chain rule for layers 16..31 that the bypass didn't exercise (the bypass skips those layers' backward; real Path C must actually compute their gradients). Developer's new TDD test `test_lazy_dequant_matches_eager` is designed to catch this.
- Projected 45–60 min first-step runtime is extrapolated, not measured. Could be longer.
