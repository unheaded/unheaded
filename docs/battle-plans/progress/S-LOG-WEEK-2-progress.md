<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-LOG-WEEK-2 progress

Plan: `docs/battle-plans/S-LOG-WEEK-2-battle-plan.md` (8 phases, 96 steps). Branch `sprint/s-log-week-2`, worktree `/home/govan/tmp/wt/S-LOG-WEEK-2`, off develop `91d63e30`. Phase 1 is executed by one lane, unattended, under `~/tmp/adr147-table/LANE-BRIEF-2026-11-21.md`: nothing live, no container, no sudo, no merge into develop. Phases 2 to 7 are the coordinator's and are not started here.

## The table

| id | Source | Evidence | Fix | Owner | Before | After | Status |
|---|---|---|---|---|---|---|---|
| LW-01 | G4, G5, G6, G12 | the probe ran after 14 minutes of scans; a slow q3 read as `NA` on 11-16; Vector's counters start again at each start; no test | Phase 1: probe before the queries, time budget, 24 h increases, restart times, a test, a reckoner | lane | no test; `timeout 600`; one probe, after the queries | see "2026-11-21: Phase 1" below | see below |
| LW-02 | G4, G7 | one "before" reading; two days of week 1 without a reading | the timer (Phase 1); the 24 h baseline with the writer stopped (Phase 3) | lane, then coordinator | no timer | units written, not installed | Phase 1 part done; Phase 3 pending |
| LW-03 | decision 5 | | the seven days (Phases 4, 5) | coordinator | | | pending |
| LW-04 | G11, LW2 | | the index rider (Phase 6) | coordinator | | | pending |
| LW-05 | S-E1-LIVE Steps 284 to 288 | | the verdict; the loser switched off; ADR-117 amended (Phase 7) | coordinator, Stevie | | | pending |

## Rulings

Stevie was not asked in this session. What is on record:

| # | Ruling | Where it is recorded |
|---|---|---|
| LW1 | A: bar (A) of 2026-11-10, unchanged | `docs/battle-plans/ADR147-LIVE-RUN-ORDER.md`, the rulings table, plan 5: "Ruled 2026-11-17: as recommended (A, C, A, A, A)" |
| LW2 | C: the week runs without indexes, then a one-day rider | the same row; also `S-E1-LIVE-progress.md`, "The closing reading" |
| LW3 | A: the copy starts empty | the same row; `S-E1-LIVE-progress.md`, "Step 284: Stevie's verdict": "plan 5 (S-LOG-WEEK-2) empties it when the second week starts" |
| LW4 | A: a user timer at 00:15Z | the same row |
| LW5 | A: this week is not plan 8's baseline | the same row |
| Steps 49, 87 | Truncate; ClickHouse stays unless Postgres passes all five tests; if it passes all five, stop and ask | `DECISION-PACK-2026-11-18.md` row 12 |
| Step 40 | The week starts without plan 4's reboot; the gate reads "new layout since <time>"; a reboot inside the week moves its end | `DECISION-PACK-2026-11-18.md` row 47 |

Step 7 (saying G3 to Stevie aloud) was not done in this session: it is his sitting. The plan's Grounding row G3 stands as written.

## Handoffs

Empty until Phase 2 (the three times of Step 40, the readings of Steps 41 and 45).

## For Stevie

See the entries below; each question names the option taken.

## 2026-11-21: Phase 0, read only (lane)

- Step 1: read. ADR-147 decision 5, the plan's Grounding, S-E1-LIVE's record from "Evening 16" to "Step 284: Stevie's verdict", `scripts/logstore-measure.sh` in full. S-WASTE-OUT Phases 21 and 22 were not read in full: they are live steps of block F and Phase 1 rests on none of them.
- Steps 2, 3: week 1's closing reading exists: the last reading in `~/.local/state/unheaded/S-E1-LIVE-logstore.tsv` is `2026-11-17T18:57:52Z` (after 18:34Z). Twelve readings, 31 metrics each. Nothing was taken.
- Steps 4, 5: Stevie's answer at Step 284 is on record: `S-E1-LIVE-progress.md`, "Step 284: Stevie's verdict (2026-11-18, about 00:30Z)": the week is closed, ClickHouse stays, the copy is left running until plan 5 empties it.
- Step 6: the worktree is `/home/govan/tmp/wt/S-LOG-WEEK-2` on `sprint/s-log-week-2`, made from develop `91d63e30` (the plan's command, without the `mkdir` under `~/.local/state`: that directory is Phase 3's and is the coordinator's to make). Scratch: `/home/govan/tmp/wt/S-LOG-WEEK-2-scratch`.
- Steps 7 to 12: `[STEVIE]`. Not asked; the rulings above are on record since 2026-11-17 and 2026-11-18.
- Steps 13, 14: this file, committed on the branch.
