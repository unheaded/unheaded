# WAVE10F 24h Phase 6 — Regression Audit

**Executed:** 2026-04-20 (10:10–10:47 UTC)
**Outcome:** ✅ ALL 6 TESTS GREEN — zero regressions from the 24h session.

## Summary

| Test | Kind | Result | Wall-clock | Key metric |
|------|------|--------|-----------:|------------|
| `test_gemma4_backward_grad_health` | `#[test]` | ✅ PASS | 144 s | `healthy=35/35 zero=0 nan=0` |
| `test_gemma4_gpu_train_step_loss_descent` | `#[test]` | ✅ PASS | 114 s | 1.97 s/step avg, loss 19.96 → 5.70 |
| `test_learning_exp1_held_out_eval` | `#[ignore]` | ✅ PASS | 267 s | ratio 0.5806, CI (0.567, 0.596) |
| `test_learning_exp3_lora_zero_identity` | `#[ignore]` | ✅ PASS | 180 s | A=0 eval ≡ base model |
| `test_learning_exp4_dataset_size_scaling` | `#[ignore]` | ✅ PASS | 354 s | \|T\|=64 vs \|T\|=8 = 37% improvement |
| `test_learning_exp5_generalization_gap_beta` | `#[ignore]` | ✅ PASS | 327 s | β=0.266, CI (−0.11, 0.64) |

All numbers match prior Learning Gate baseline within fp noise.

## First-attempt audit bug (documented for next time)

The initial audit script passed `-- --ignored` to all six tests. That
flag tells cargo to run ONLY ignored tests, which *filtered out* the
two non-ignored tests (`test_gemma4_backward_grad_health` and
`test_gemma4_gpu_train_step_loss_descent`). Cargo reported
`0 passed; 0 failed; 0 ignored; 106 filtered out` and my grep for
`"test result: ok\. 1 passed"` spuriously flagged them FAIL.

Recovery: dropped the `--ignored` flag for those two tests, reran
individually. Both passed.

Lesson for next-session audit script: use `--include-ignored` if
mixing ignored and non-ignored targets, or run each test with the
correct flag for its annotation.

## No-regression confirmation

GPU warm-step time unchanged at 1.97 s vs prior baseline 2.05 s
(noise within ±5%). Loss trajectory bit-identical: 19.9595, 15.9253,
5.7022 across three test runs (Step C, Learning Gate C8, and this
24h Phase 6). The 24h session's code changes — `run_until_plateau`,
`synthetic_multi_y`, `compute_eval_loss_per_group`,
`harness_from_jsonl`, Exp 2 rewrite, Exp 1 extended + lr sweep,
Exp 6 multi-Y, RAFT smoke — are purely additive. Nothing touched
the core forward/backward/Adam path.
