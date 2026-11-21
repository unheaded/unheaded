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

## 2026-11-21: Phase 1, the instrument (lane)

Every run of the script in this phase was against stubs. Nothing live was read or changed: no docker, no sudo, no query to a store or to VictoriaMetrics, no unit installed.

### Steps 15, 16: the test first

`scripts/test-logstore-measure.sh`: stub `sudo`, `docker`, `curl`, `systemctl` and `du` first on `PATH`, a made-up `/proc` (`LOGSTORE_PROC`), a made-up backup directory. The plan's six cases, plus: the writer stopped (case 7), bad settings (case 8), the default file (case 9). Red against the unchanged script: 16 of 44 checks passed and 28 failed, each for the reason the plan names (no `_runs`, no first probe, no 24 h counts, no start times; the old in-container loop answered once, so the ClickHouse timings read `NA`). The stubs were reached: the Postgres p50 and p95 came from the stub's times.

Departures from the step's text:

- `timeout` is not stubbed. The real one is used, so the cut of a ClickHouse run past the budget is the real cut.
- The stub's rows carry `CANARY` in every timed answer (as if a result leaked into the output), and one case returns text where a count belongs: `NA`, exit 1, no row text in the file.
- Case 4 also reads the order in which the statements reached the stub, not only the order of the lines in the file.

### Steps 17 to 21: the script

- **The budget (G5).** `QUERY_BUDGET_S`, default 900. The plan asks for "a number of 60 or more" and tests with a budget of 2 s; both hold through `QUERY_BUDGET_MIN_S` (default 60), which the test lowers.
- **Postgres keeps the budget inside the session, not from the host.** Week 1 ran the twenty runs in one psql session and its timings come from one warm backend; a session a run would have changed what is measured. So the script still writes one session: a deadline on the server's clock, and before each run "how much is left", `\if`, `SET statement_timeout` to what is left, the statement, a marker. The server ends the run that would pass the deadline, so no scan outlives the measurement (the next thing a reading does after q3 is the second Well probe), and psql stops at that error.
- **A time psql prints for a statement the server cut is not a run.** With `\timing on` psql can print `Time:` after a statement that failed as well. Only a time followed by the marker counts; the marker is printed only after a statement that finished. Case 2 proves it: six runs, a seventh cut after 3 s whose time the stub prints, `pg_q3_units_7d_runs` is 6 and the p95 is the sixth run's.
- **ClickHouse**: one `docker exec` a run from the host, as the plan says, each under `timeout <what is left>`. A run counts only when the client exited 0 and ended on a number. The server time `--time` reports does not include the `docker exec`.
- `MIN_RUNS=5` is a constant, not a setting: it is part of what a reading means and both weeks are compared on it.
- **The second probe (G4)**: `well_probe_first_*` before the three queries; `well_probe_*` stays after them.
- **24 h counts (G6)**: as the plan writes the two queries. One refinement: an empty answer for the drop count is `0` only while VictoriaMetrics holds Vector's series at all (the sends query answered); otherwise it is `NA`. A missing scrape must not read as "no drop".
- **Start times (Step 20)**: `docker inspect -f '{{.State.Running}} {{.State.StartedAt}}'` (a stopped container still has a `StartedAt`, so `Running` decides the `NA`). `writer_started_unix` alone does not fail the reading. `host_boot_unix` is `btime` of `/proc/stat` and not `date -u -d "$(uptime -s)"`: `uptime -s` prints local time and is recomputed at each call, so it can differ by a second between two readings and would read as a reboot.
- **Added beyond the plan's list: `vector_well_buffer_dropped_total` and `vector_well_buffer_dropped_24h`.** Week 1's `vector_well_dropped_total` reads `vector_component_discarded_events_total{component_id="well_logs"}`: rows the sink itself gave up on. A buffer with `when_full: drop_newest` that is full counts in `vector_buffer_discarded_events_total` (the series `VectorSinkDroppingEvents` reads since 2026-11-15, `monitoring/prometheus/rules/logs.yml`), and that is what a stopped writer causes. The week 1 metric keeps its name and meaning; the two new ones sit beside it. Not checked against the running Vector (no live read in this phase): the coordinator's Step 50 shows which counter rises.
- West's `date` is uutils 0.2.2; it parses Docker's `2026-11-20T17:50:01.123456789Z`. Checked.

### Steps 26 to 30: the reckoner

`scripts/logstore-reckon.py` (standard library) and `tests/scripts/test_logstore_reckon.py`. Beyond the step's list:

- **The backup's "before" is the last baseline reading's figure, not a median.** The first baseline reading still carries the night before the truncate (a backup with the whole copy in it); the backup taken inside the baseline is the "before". The script says whether a backup ran inside the baseline.
- **Which probe decides the Well's row**: the one before the queries when the file has it; both are shown with their own verdict. (For Stevie, below.)
- **The five tests and decision pack row 12.** Under the table: which of the five tests of bar (A) Postgres fails (disk; the three queries' p95 together; the Well's latency; the backup's seconds; sends over 1 s, by the 24 h count when the file has it), then row 12's consequence in its words: one failed test, "ClickHouse stays and the plan closes"; all five passed, "stop and ask Stevie"; a test not judged with none failed, or no baseline named, "no verdict from this file". The plan's Step 87 carries a note of today saying so.
- **The copy against the writer's ceiling (G8)**: bytes at the last reading, growth a day, days to `--ceiling-bytes` (default the writer's `--max-bytes`, 4 GiB; `pg_bytes` is the same sum `log_events_maintain()` returns to the writer).
- The bar's two numbers are options (`--ratio-bar`, `--move-bar`), default bar (A).
- Rows dropped at the Well sink are shown and never judged (the bar says the Well sink may drop).

