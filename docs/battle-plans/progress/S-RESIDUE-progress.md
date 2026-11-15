<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-RESIDUE progress

Plan: `docs/battle-plans/S-RESIDUE-battle-plan.md` (10 phases, 176 steps).

## Decisions

| # | Question (short) | Answer | Effect |
|---|---|---|---|
| A-19 | `~/.config/cs` → `~/.config/vor` | Docs and `raft/` now; the prompt text, its goldens and the ADR-024 eval rerun later as a gated phase (Stevie, 2026-11-12) | Phase 1 runs; Phase 8 waits on llama-server |

A-56 (egress rule for llama-server) stays open; Step 157 is blocked on it.

## Step log

| Date | Steps | Note |
|---|---|---|
| 2026-11-15 | 1-16 | Phase 0. Ruff gate was red with the 12 findings the plan lists (`tests/runbooks/test_ioc_audit.py`); fixed by hand, no `noqa`, no exclusion; ruff PASS, 36 tests pass. Go race baseline of the touched packages green. Step 12 as written cannot run: the meta-gate refuses a dirty tree and the untracked `demos/doom/` counts as dirty. The baseline runs in a clean clone instead (same as the CI replay recipe). |

## Parked

Nothing yet.

## Stuck

Nothing yet.
