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
- A cost of the ClickHouse runs going one `docker exec` at a time: a reading now calls `sudo` about 80 times where week 1's called it about 15. Each call is a few journal lines ("session opened" among them, which q2 counts), and they reach both stores alike, so the comparison is not tilted; against some 300,000 rows a day it is small. Said so that it is not found later as a surprise.

### Steps 26 to 30: the reckoner

`scripts/logstore-reckon.py` (standard library) and `tests/scripts/test_logstore_reckon.py`. Beyond the step's list:

- **The backup's "before" is the last baseline reading's figure, not a median.** The first baseline reading still carries the night before the truncate (a backup with the whole copy in it); the backup taken inside the baseline is the "before". The script says whether a backup ran inside the baseline.
- **Which probe decides the Well's row**: the one before the queries when the file has it; both are shown with their own verdict. (For Stevie, below.)
- **The five tests and decision pack row 12.** Under the table: which of the five tests of bar (A) Postgres fails (disk; the three queries' p95 together; the Well's latency; the backup's seconds; sends over 1 s, by the 24 h count when the file has it), then row 12's consequence in its words: one failed test, "ClickHouse stays and the plan closes"; all five passed, "stop and ask Stevie"; a test not judged with none failed, or no baseline named, "no verdict from this file". The plan's Step 87 carries a note of today saying so.
- **The copy against the writer's ceiling (G8)**: bytes at the last reading, growth a day, days to `--ceiling-bytes` (default the writer's `--max-bytes`, 4 GiB; `pg_bytes` is the same sum `log_events_maintain()` returns to the writer).
- The bar's two numbers are options (`--ratio-bar`, `--move-bar`), default bar (A).
- Rows dropped at the Well sink are shown and never judged (the bar says the Well sink may drop).

### Results (Steps 22, 24, 27, 29, 32 to 34)

- Step 22: `scripts/test-logstore-measure.sh`: 47 passed, 0 failed, about 50 s. `shellcheck` on the script, its test and `scripts/well/test-logstore-budget.sh`: clean at the default severity.
- Step 23: registered in `scripts/check-gates-can-fail.sh` (row `test-logstore-measure`, fast), `scripts/ci-local.sh` (step 13) and `.github/workflows/static-analysis.yml` (job `manifests`, beside the other stubbed tests). The plan's `git grep -n 'test-planned-stop' -- .github scripts/ci-local.sh` finds nothing today: `test-planned-stop.sh`, `test-host-state-textfile.sh` and `test-boot-event.sh` are called by no workflow and not by `ci-local.sh` (named under "Found outside the task").
- Step 24: the provocation (`MIN_RUNS=5` to `0`) makes the test fail: `test-logstore-measure      OK — bites when provoked (exit 1)`, `PASS: 1 gate(s) proven to fail when violated`. **Run in a throwaway copy of the tree, not in the worktree**: on develop the meta-gate stops at its coverage check before any row, `FAIL: gate script(s) with no classification: well/batch-apply` (`runbooks/data/well-pg18-upgrade.yaml` names `scripts/well/batch-apply.sh`; true at `91d63e30` and at `eb9af319`). The copy carried one extra line classifying that script; the branch does not: it is not this lane's to classify. `scripts/test-check-gates-can-fail.sh` fails on develop for the same reason.
- Step 27: red: without the script, 16 of 16 fail. Green: 16 passed.
- Step 29, week 1's real file (numbers only):
  - The plan's command as written (`--baseline-until 2026-11-10T19 --start 2026-11-11T12`): disk 20x to 23x; q2 **137x** to 619x; q3 391x to 1,833x; the Well's probe -9% to +33%. Two figures differ from the prepared table and both are explained: the file now holds the closing reading of 2026-11-17T18:57Z, which the prepared table was written before (its q2 ratio is 137x), and `--baseline-until 2026-11-10T19` takes two readings as the "before" (18:28Z and 18:37Z, median 0.217 ms) where the prepared table used the first (0.178 ms).
  - With `--baseline-until 2026-11-10T18:30 --start 2026-11-11T12 --end 2026-11-17T12` (the readings the prepared table had): ClickHouse 64 to 72 and Postgres 1,454 to 1,464 bytes a row, **20x to 23x**; q1 7.0 to 30 ms against 0.727 to 0.932 ms; q2 20 to 33 ms against 4.3 to 16.1 s, **177x to 619x**; q3 13 to 23 ms against 5.1 to 31.0 s, **391x to 1,833x**; the Well's probe 0.178 ms before, 0.197 to 0.289 ms, +11% to +62%; one send over 1 s and 263 rows dropped; the copy growing 361 MB a day, 5.8 days to 4 GiB. Every figure is the prepared table's and its Python block's.
- Step 32: `systemd-analyze verify --user` on the two units: no line.
- Steps 33, 34: `check-spdx.sh` OK; `check-unit-start-limits.sh` PASS; `check-systemd-units.sh` PASS (70 unit files); `check-unit-ceilings.sh` OK; `check-unit-flags.sh` PASS; `check-ci-verdicts.sh` PASS; `check-manifest-yaml.sh` PASS; `check-actions-pinned.sh` PASS. `scripts/check-live-paths.sh` does not exist under that or a like name. No list needed a new entry.

### Steps 37, 38: the exit gate

Step 37 as the lane's rules have it: develop (`eb9af319`) was merged **into** the branch (signed); the branch was not merged into develop. That merge is the coordinator's.

| The gate's line | Result |
|---|---|
| Six cases | green: 47 checks over the plan's six cases and three more |
| The gate proven to fail | yes, in a copy (see Step 24): the row cannot run on develop until `well/batch-apply` is classified |
| The reckoner agreeing with week 1's table | yes, figure for figure, on the readings the table had |
| `$PROG` committed by name | this commit |
| No week 1 timer runs | `systemctl --user list-timers --all` has no `logstore-measure` line; no unit copied to `~/.config/systemd/user` |
| Nothing live has changed | nothing: no docker, sudo, store query or unit; week 1's readings file is as it was (17,435 bytes, 2026-11-17 19:07) |

Also run on the merged branch: `go build ./...` and `go vet ./pkg/... ./cmd/... ./services/...` (exit 0; no Go file is touched, so no package test, no golangci-lint); `ruff` on the two Python files and `scripts/check-ruff.sh` (0 findings); `bandit` with CI's skips on the two Python files (nothing); `pytest tests/scripts` (236 passed); `shellcheck -S warning` on the five touched scripts (clean); `check-nosec-ratchet.sh` (PASS, 845, none above baseline); `check-gosec-ratchet.sh` (PASS). No migration was added: `check-well-lint.sh` not run. No generated file is affected (no binary, runbook or crypto change).

LW-01: done but for the container test below. LW-02: the units are written and verified; installing them is Step 54.

### For the coordinator: needs a container (not run)

1. `scripts/well/test-logstore-budget.sh` (about a minute; `postgres:16.15-alpine`, no network). It is the only proof of the budget against a real psql and server: the stub test cannot evaluate `\if`, `\gset` or `statement_timeout`. **Run it before the first live reading.** If it fails, the first thing to read is whether psql printed the marker (`logstore-run-ok`) after each finished run.
2. `scripts/check-gates-can-fail.sh --only well/test-logstore-budget` (twice the above), once `well/batch-apply` is classified; and `--only test-logstore-measure` on the real tree then.
3. `scripts/ci-local.sh`: two new steps, "log store reading budget (throwaway Well)" and "log store reading (stubs)".
4. Live, read only, before Step 55: that VictoriaMetrics answers the three 24 h queries with Vector's series (`curl -s -G http://127.0.0.1:8428/api/v1/query --data-urlencode 'query=count by (__name__) ({job="vector",__name__=~"vector_buffer_send_duration_seconds_count|vector_buffer_discarded_events_total|vector_component_discarded_events_total"})'`). `config/victoria/scrape.yml` keeps every series of the `vector` job, so they should be there; a reading whose `vector_record_sends_over_1s_24h` is `NA` says they are not.

### Phases 2 to 7 against today's tree (a list, nothing fixed)

1. **Steps 47, 61, 89: the mark's scope.** `ContainerStopped` matches a mark to the container's `name` label, which is the container's name (`unheaded_container_running{name="unheaded-logstore-writer"}`, `monitoring/prometheus/rules/containers.yml`). `planned-stop.sh logstore-writer 26h` marks nothing that rule reads; the scope is `unheaded-logstore-writer`. `ContainerStopped` is promoted in `observability/alerting/policy.yaml`, so the wrong scope puts the stopped writer in the mail.
2. **Step 50: the counter.** Its `grep` reads `vector_component_discarded_events_total`; a full `drop_newest` buffer counts in `vector_buffer_discarded_events_total`. Read both. `VectorSinkDroppingEvents` (urgency none, not promoted) will fire for the whole baseline.
3. **Step 40** and the plan's "Prerequisite" line still say plan 4's reboot is past; decision pack row 47 replaces that (the note is on Step 40). Plan 4's Step 175 writes "the new layout began <time>", not "new layout since"; Step 39's `grep 'new layout'` finds either. `ADR147-LIVE-RUN-ORDER.md` line 185 still reads "Not settled".
4. **Step 41** waits on plan 2's Steps 368 and 384 (block F): today `config/vector/vector.yaml`'s `clickhouse` sink has no `batch` block and `postgres` is at 256M. Not stale; not yet true.
5. **Step 57: "03:37Z"** was one night's time: `unheaded-well-backup.timer` is 03:30 with up to 20 minutes of random delay. Since block D three more backups run each night that week 1 did not have (Zhen 04:00, ClickHouse 04:15, VictoriaMetrics 04:45); `backup_seconds` reads the Well's unit only.
6. **The probe's table can change size inside the week.** The Well's latency is `SELECT * FROM kanban_tasks`, and decision pack rows 34 and 58 add and change cards "after the stack is up". The row's bar is 10%. Do the card work before the baseline or after the close, or write it in `disturbances`.
7. **Step 78 with `--start T0`** takes the reading of one hour after `T0` into the week: its 24 h counts still cover 23 hours of the baseline (the Well sink's buffer drops with the writer stopped; not a test row), and its bytes a row are over one hour of rows.
8. **Steps 81, 82 and 89 against ADR-151** (accepted in decision pack row 23 after this plan was forged: "the measurement writer becomes the Postgres driver"). The rider's `CREATE EXTENSION pg_trgm` and two indexes are made by hand on the live Well, outside the migration ledger; Step 89's "DROP TABLE and the removal of the writer are Stevie's" should be read with that ADR.
9. **Step 92**: `ci-local.sh` has two more steps (above). **Step 90**: `wiki/Log-Aggregation.md` now has a section, "The log store reading", to bring up to date at the close.
10. **The ceiling date.** "About 2026-11-22 20Z" (the run order, next.md) was reckoned before the stack was stopped from 11-19 00:44Z to 11-21 17:50Z; the copy took no rows then, so the date is later, and migration 048's 14 days start dropping 11-10's partitions on 11-24.
11. **A reading by hand (Steps 55, 69, 77)** can now take up to five Postgres sessions of `QUERY_BUDGET_S` each; Step 55's "~25m" holds only while the queries are no slower than week 1's.

### Found outside the task (named, not fixed)

- The meta-gate does not run on develop: `well/batch-apply` is unclassified (see Step 24). `scripts/test-check-gates-can-fail.sh` fails with it.
- `scripts/monitoring/test-planned-stop.sh`, `test-host-state-textfile.sh` and `test-boot-event.sh` are named as the proof of three `NOT_A_GATE` rows but no workflow and no `ci-local.sh` step runs them.
- `scripts/check-spdx.sh --langs py,sh`: 6 files without a header, all fixtures or archive (`eval/two-brains/fixtures/sh-*`, `cmd/crypto-inventory/testdata/.../setup.sh`, `docs/archive/.../S27-P0-nomenclature-fix.sh`). The default (`go,rs`) passes.
- West's `timeout` (uutils 0.2.2) adds about 0.1 s to every command it wraps. It does not touch a measured time; it is why the stub test takes 50 s.

### For Stevie

1. **Which probe decides the Well's latency row.** Taken: the probe before the three queries when a file has it (the plan's G4 is the reason: the one after them follows minutes of scans in the week and none in the baseline). Both are printed with their own verdict, so the other reading is one glance away. Say if the row should rest on the old probe, or on both.
2. **What "the five tests" of row 12 are.** Taken: disk; the three queries' p95 as one test (it fails if any of the three is over 2x); the Well's latency; the backup's seconds; sends over 1 s on the record sink. The bar as chosen on 2026-11-10 names these five ("2x disk, 2x query p95, under 10% on the Well's p95 and backup time, zero stalls"). Rows dropped at the Well sink are not a test.
3. **The stall test reads the 24 h count** when the file has it (the cumulative counter starts again at each Vector start and carries anything from before the week). Say if a stall before `T0` in the same Vector run should count.
4. **Two metrics more than the plan lists** (`vector_well_buffer_dropped_*`), on the reasoning under Steps 17 to 21. Nothing is judged on them.

