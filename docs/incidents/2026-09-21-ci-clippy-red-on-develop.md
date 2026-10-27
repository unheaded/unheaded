<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->

# 2026-09-21: Static Analysis (clippy) red on develop for at least nine days

**Status:** draft (written 2026-10-27 from the session record; not yet reviewed by Stevie)
**Services:** GitHub Actions on develop: the Static Analysis workflow's clippy job

## Summary

The clippy job in Static Analysis failed on every develop push from at least
2026-09-21 to 2026-09-30, while clippy was clean on west. The runners lacked
`protoc`, which the tonic build scripts in `cmd/ebpf-collector` and
`crates/zhend` need, so the job died before linting anything. Because the job
was red for an environmental reason, real lint failures pushed in that window
would have been invisible. Found when Stevie pasted the log on 2026-09-30.

## Impact

- **Who/what:** the Rust lint gate on develop, and every push's signal from
  it. No production service was affected.
- **How long:** at least 9 days (the oldest red run checked was 2026-09-21).
- **Data:** none. The cost was trust in the gate: a red workflow people learn
  to ignore is worse than none.

## Timeline (UTC)

| Time (UTC) | Event |
|---|---|
| 2026-09-21 | Earliest red clippy run on develop found (it may have started earlier) |
| 2026-09-30 ~23:15 | Stevie pushes, pastes the failing log; cause: `protoc` missing on the runner |
| 2026-09-30 | `a9ac4528`: apt `protobuf-compiler` in the clippy and meta-gate jobs; `check-clippy.sh` prints the tail; zhend's build script fails loudly |
| later (October) | Two more CI-only clippy breaks of the same kind (a GPU build script needing `hipcc` on runners without ROCm; a toolchain step leaving nightly as the default), each fixed as found |

## Detection

A person reading the Actions page, after more than a week.

What should have: **a "CI red on develop" alert** (ADR-100 section 1, Phase 4:
"CI on develop" availability SLO). It did not exist then; S-WELL-SPRING Step
179 adds the `ci-develop` SLO from the public Actions API (M-087). A local
gate that runs the CI job's own setup (the meta-gate in a clean clone,
`check-gates-can-fail.sh`) catches the environment half of the problem before
a push.

## Root cause

1. The workflow assumed tools that west has and the hosted runners do not
   (`protoc`; later `hipcc` and a stable default toolchain).
2. The job's failure looked like any lint failure from the outside, so its
   cause was not obvious without reading the log.
3. Nothing watched develop's workflow results; the only feedback loop was a
   person opening the Actions page.

## What went well

- Once the log was read, the fix was one package install, and the build script
  was changed to fail with a clear message instead of a panic deep in a
  dependency.
- Later CI-only breaks were caught faster by reproducing CI in a clean clone
  (the recipe S-CLEAN-SLATE Phase 1 used).

## Action items

| Action | Tracked as | State |
|---|---|---|
| Install protoc on the runners; fail loudly without it | done: `a9ac4528` | done |
| `ci-develop` SLO from the Actions API, with a runbook | M-087, S-WELL-SPRING Step 179 | built; live after the E-D window installs the timer |
| Reproduce CI in a clean clone before pushing a batch | done: `check-gates-can-fail.sh`, meta-gate job (S-CLEAN-SLATE `74e82559`) | done |

## Threshold check

Meets it as a silent failure longer than a day: a quality gate was broken
for at least nine days without anyone being told. ADR-100 names it as one of
its three founding incidents.
