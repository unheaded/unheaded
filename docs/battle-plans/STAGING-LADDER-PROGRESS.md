<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Staging Ladder — Live Progress Log

Execution log for `docs/battle-plans/STAGING-LADDER-2026-08-03.md`.
**Stevie: read this first.** One row per completed phase, in ladder order.

**Branch**: `staging` (from `develop` @ `b39fb207`)
**Started**: 2026-08-03, overnight autonomous run
**Signing**: gpg-agent has no cached key overnight → commits are `--no-gpg-sign`.
**Every commit below needs re-signing before it crosses into `main`** (ADR-089 §exit gate).

---

## KNOWN FAILURES BASELINE (recorded before any edit)

**`next.md` claims "979 tests pass, 0 failed" at `b39fb207`. That is not reproducible.**
Measured on this machine at `b39fb207` with only documentation files added:

| Package | Failures | Cause |
|---|---|---|
| `cmd/wiki-server` | 4 | **Stale tests.** `879c91cf` ("drop redundant Home list entry") intentionally changed `listPages` to skip `README.md`; the tests still assert the old behaviour. Product code is correct. |
| `cmd/dashboard-backend/internal/server` | 2 | `websocket: bad handshake` — under investigation |

Also emitted (not a failure, expected without a DB):
`Postgres unavailable — health persistence disabled: password authentication failed`.

These 6 failures are **pre-existing** and are not caused by this plan. Every later phase
compares against this baseline, not against zero.

### Other baselines at `b39fb207`

| Linter | Baseline |
|---|---|
| ruff | 427 |
| bandit | 212 (0 HIGH / 51 MEDIUM / 161 LOW) |
| shellcheck | 350 (0 error / 98 warning / 252 note) |
| eslint | unmeasured — no config, no `package.json` |
| clippy | 0 (gating) |
| `monad-cpu-ebpf --features ascend-linux` | 901,888 bytes |

---

## Commits on `staging`, in ladder order

| # | Rung | Phase | Commit | What | Baseline delta | Signed |
|---|---|---|---|---|---|---|
| 1 | — | 0 | _pending_ | Plan, ADR-089, ADR-090, progress scaffold | — | no |

---

## Deviations from the plan as written

- **H5 falsified at Phase 0.** The plan assumed a green suite. It is not green. The
  KNOWN FAILURES BASELINE above is the corrected starting point, and a new phase
  (Phase 1.5, rung R1, test-only) was added to fix the stale wiki-server tests since
  they are cheap, low-risk, and were discovered during preflight.

---

## Stuck items

_none yet_
