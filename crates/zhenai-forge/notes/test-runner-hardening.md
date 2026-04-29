# zhenai-forge test-runner hardening — 2026-04-29

## Why this exists

During WAVE14 Phase 1 (parser fix, commit `008727c5`), running `cargo test
--bin zhenai-forge --tests` on the 14 GB dev box (`west`) OOM-killed the
tmux session and required a hard reboot.

dmesg evidence (boot −1):
```
Apr 28 23:59:36 west kernel: Out of memory: Killed process 19065
  (zhenai_forge-68) total-vm:175233400kB, anon-rss:11556232kB
  cgroup=tmux-spawn-…/user@1000.service
```

Root cause: tests use `setup_gpu()` and `GgufFile::open()` to load the 9.3 GB
Gemma-4 E2B GGUF (or 5 GB Mistral-7B q5_k_m). With 12 CPUs and default
`cargo test` parallelism, multiple test threads each load the full model →
multi-fold memory amplification → OOM cgroup-kill of the tmux scope → "system
crashed" symptom.

## What this hardening does

### 1. `#[ignore]` on every GGUF-loading test (HARDEN-2)

18 test functions across 4 files newly marked `#[ignore]`:

| File | Tests marked |
|---|---|
| `src/gemma4.rs` | 7 (test_gemma4_load_e2b, _lora_save_load_roundtrip, _train_step_loss_descent, _lora_grad_health, _backward_grad_health, _forward_finite, test_layer_pattern_from_tensor_shapes) |
| `src/forward.rs` | 1 (test_dequantize_real_tensor) |
| `src/gemma4_gpu.rs` | 7 (test_gemma4_gpu_upload_full, _upload_cpu_ple, _wq_matches_cpu, _train_step_loss_descent, _matmul_grad_x_matches_cpu, test_matmul_xwt_gpu_in_out_matches_cpu, _forward_matches_cpu, _wq_speedup) |
| `src/train.rs` | 2 (test_gpu_model_load, test_cpu_weights_path_c_lazy_skip) |

Pre-existing `#[ignore]` annotations on 10 other heavy tests (Learning Gate
suite in `src/eval.rs` + `test_gemma4_gpu_10step_descent` in `gemma4_gpu.rs`)
are untouched.

Total heavy tests now gated: **28**. None will run during a default
`cargo test`.

### 2. `.cargo/config.toml` with `jobs=4` (HARDEN-3)

Caps concurrent rustc/hipcc compile jobs at 4 (down from default = nproc =
12). With ~2 GB peak per rustc job, build-side memory stays under ~10 GB.
Override per-invocation with `CARGO_BUILD_JOBS=8 cargo build` on bigger
hosts.

## How to run heavy tests

**Don't run them on `west` (14 GB dev box).** Two options:

1. **Bare-metal east/west bootstrap.** Per `~/.claude/.../memory/reference_east_west_hosts.md`,
   both bare-metal hosts have ROCm and significantly more RAM than the dev
   loop. Heavy tests should run there:

   ```
   ssh govan@east   # or west bare-metal
   cd /path/to/unheaded/crates/zhenai-forge
   cargo test -- --ignored
   ```

2. **Single test, careful manual run on dev box.** If you must run a single
   heavy test on `west`:

   ```
   systemd-run --user --scope -p MemoryMax=10G \
       cargo test --bin zhenai-forge --tests \
       test_gemma4_load_e2b -- --ignored --exact
   ```

   The `MemoryMax=10G` cgroup cap means OOM kills only the test subtree,
   never the tmux session. Pick one test at a time; don't run the whole
   `--ignored` suite this way.

## What is safe to run on the dev box

```
cargo check --tests --jobs 1     # type-check tests, no binary build (~2s warm)
cargo test --bin zhenai-forge    # runs only NON-ignored tests
                                 # (Learning Gate is excluded; light tests pass)
cargo build --release            # release build of zhenai-forge binary
```

## What's NOT included in this hardening

- **No `[profile.test] opt-level` change.** Would affect every developer's
  iteration cadence; not worth the trade-off.
- **No env-gate (`ZHENAI_HEAVY_TESTS=1`).** `#[ignore]` is the idiomatic
  cargo solution and matches the existing Learning Gate convention.
- **No automatic re-routing to east/west.** Manual SSH for now; could be a
  future Makefile target if heavy tests become routine.

## Verification

After the hardening commit:

```
cd /home/govan/tmp/unheaded/crates/zhenai-forge
cargo check --tests --jobs 1     # green, 0 errors
grep -rln '#\[ignore\]' src/     # 28 occurrences total
ls .cargo/config.toml             # exists
```

## Reverting (if you want full `cargo test` back later)

```
git revert <hardening commit>
```

…but only do this on a host with ≥32 GB RAM. The OOM behavior is real and
returning to default cargo behavior on a 14 GB box will reproduce the crash.

## See also

- WAVE14 H6 root-cause analysis: `notes/wave14-h2h6-analysis.md`
- WAVE14 Phase 1 commit: `008727c5`
- Battle plan (paused at Phase 2): `~/.claude/plans/synthetic-stirring-pudding.md`
