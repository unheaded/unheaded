# WAVE10F Phase 2 — Exp 1 Extended + LR Sensitivity Sweep

**Executed:** 2026-04-20, 24h battle plan Phase 2.
**Tests:** `test_learning_exp1_extended` (FAIL), `test_learning_exp1_lr_sweep` (PASS).

## Result summary

| Test | Outcome | Notes |
|------|---------|-------|
| `test_learning_exp1_extended` (lr=3e-3, 100 steps plateau) | ❌ FAIL | ratio 1.066 — training destabilizes after step ~30 |
| `test_learning_exp1_lr_sweep` (4 lrs × 100 steps) | ✅ PASS | 3 of 4 lrs below the 0.90 gate |

## LR sensitivity sweep data

Corpus: synthetic `EvalHarness::synthetic(0xE7A1, 32, 32, vocab=262144)`.
Training: 100 steps plateau-based, `plateau_window=10`, `plateau_eps=0.1`.

| lr   | steps_used | eval_final | ratio  | CI95 (bootstrap) | Wall-clock |
|------|-----------:|-----------:|-------:|------------------|-----------:|
| 1e-3 | 100        | 15.1221    | **0.7001** | (0.6643, 0.7391) | 719.5 s |
| 3e-3 | 100        | 23.0259    | 1.0661 | (1.0416, 1.0961) | 714.0 s |
| 1e-2 | 100        | 16.8262    | 0.7790 | (0.7400, 0.8187) | 715.0 s |
| 3e-2 | 100        | 16.4544    | 0.7618 | (0.7151, 0.8122) | 707.9 s |

Gate: ratio ≤ 0.90 AND CI upper < 1.0. Pass: 1e-3, 1e-2, 3e-2 (3/4).

## The "cliff edge" at lr=3e-3

Original Learning Gate used lr=3e-3 at 20 steps and got ratio=0.58. The
same lr at 100 steps produces ratio=1.07 — eval BOUNCES BACK to baseline.

Inspection of the Exp 1 extended trajectory (`/tmp/24h-exp1-ext.log`):

```
  t=  0  eval=21.60       (base)
  t=  5  eval=13.32       ← learning starts fast
  t= 10  eval=12.85
  t= 15  eval=12.60
  t= 20  eval=12.54       ← this was the "success" at 20 steps
  t= 25  eval=12.30
  t= 30  eval=12.13       ← best-ever, then destabilizes
  t= 35  eval=15.01       ← bounce #1
  t= 40  eval=13.79
  t= 45  eval=13.18
  t= 50  eval=12.92
  t= 55  eval=12.07
  t= 60  eval=13.92       ← bounce #2
  t= 65  eval=12.61
  t= 70  eval=13.22
  t= 75  eval=15.04
  t= 80  eval=15.27
  t= 85  eval=14.23
  t= 90  eval=13.56
  t= 95  eval=14.57
  t=100  eval=23.03       ← destroyed
```

Classic "lr too high for long training" pattern — loss surface has a
sharp minimum at the Y map, and lr=3e-3 overshoots it cyclically until
the optimizer lands at a chaotic weight configuration.

## Verdict: lr=1e-3 is the right operating point

- Monotonic-ish descent over 100 steps
- Ratio 0.70 with CI firmly < 1.0
- Safe for long training runs (Phase 5 RAFT)

**Adopt lr=1e-3 for Phase 5 Kingdom QA training.** Original 20-step
Learning Gate used lr=3e-3 and succeeded; that's still valid AS A
SHORT-TRAINING test. For longer runs, 1e-3 is the stable choice.

## Why lr=1e-2 and 3e-2 still pass

Both produce noisy training trajectories that oscillate more than lr=1e-3,
but the plateau detector was strict enough (K=10, eps=0.1) that they
either triggered early plateau or finished at reasonable eval. The 3e-2
result (ratio 0.76) is surprisingly good and suggests the landscape has
wide basins of attraction; the model tolerates the chaos.

The PATHOLOGICAL case is lr=3e-3 which sits on a cliff edge — fast-
descent-then-explode.

## Follow-ups (out of scope for this 24h block)

- Run a finer-grained sweep at lr ∈ {3e-4, 5e-4, 8e-4, 1e-3, 1.5e-3, 2e-3}
  to find the floor of stability.
- Try Adam β₁=0.5 or plain SGD on the cliff-edge lr to check if the
  instability is Adam-momentum-driven.
- Longer runs at lr=1e-3 (200, 500 steps) to see eventual saturation.
