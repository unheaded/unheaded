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
| 1 | — | 0 | `e21aea4b` | Plan, ADR-089, ADR-090, progress scaffold | — | no |
| 2 | R0 | 1 | `63f81e93` | Untracked file disposition | — | no |
| 3 | R0 | 2 | `0584650e` | Notebook repair + notebook syntax gate | **ruff 427 → 335** | no |
| 4 | R1 | 1.5 | `e634508e` | wiki-server stale test assertions | failures 6 → 2 | no |
| 5 | R1 | 1.5 | `cc0e0390` | dashboard-backend WebSocket Origin header | **failures 2 → 0** | no |

**Current state**: `go test ./...` = 244 packages ok, **0 failures**. ruff 335.

---

## Findings worth Stevie's attention

1. **`next.md`'s "979 tests pass, 0 failed" was not reproducible.** Six tests failed at
   `b39fb207`. All six were stale tests that had never been updated when the production
   behaviour they assert was intentionally changed — four by `879c91cf` (wiki nav), two
   by the WebSocket origin hardening. No product bugs. The suite is green now.

2. **`notebooks/02_hypothesis_matrix.ipynb` has never been runnable.** Three code cells
   fail to parse, and they fail to parse in `3dbd7eee`, the only commit that ever touched
   the file. It was committed without being executed. Twenty two-line plot labels were
   written with real newlines inside single-quoted strings.

3. **A GATING check was blind to a whole artifact class.** `check-python-syntax.sh` only
   globbed `*.py`, so it reported "all 70 files parse" while a tracked notebook in the
   tree could not. This is the same failure shape the script was originally written to
   catch, one artifact class over. Now extended to `*.ipynb`, verified red-then-green.

4. **One broken quote was 24% of the ruff backlog.** Syntax errors cascade in the parser:
   3 unterminated strings generated 104 of 427 findings. The ruff number was never as bad
   as it looked.

---

## Deviations from the plan as written

- **H5 falsified at Phase 0.** The plan assumed a green suite; it was not green. The
  KNOWN FAILURES BASELINE above is the corrected starting point.
- **Phase 1.5 added** (rung R1, test-only) to fix the six stale tests, since they were
  cheap, genuinely low-risk, and blocking any honest "no regressions" claim later.
- **H3 partially falsified.** The plan said all 104 invalid-syntax findings were one
  notebook *cell*; it was one notebook, three cells, twenty sites. The plan's Step 26
  debug branch covered exactly this and was followed.

---

## Stuck items

_none yet_
