<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-LOG-WEEK-2 BATTLE PLAN: 8 Phases, 96 Steps

**Date**: 2026-11-17
**Sprint**: S-LOG-WEEK-2, plan 5 of the ADR-147 series (`docs/battle-plans/SPRINT-SERIES-2026-11-15-ADR-147.md`). A second, fair log store week: the same rows in ClickHouse and in the Well, after Vector batches its inserts and Postgres has ClickHouse's budget, on the new disk layout.
**Prerequisite**: ADR-147 decision 5. Week 1 is closed (S-E1-LIVE Step 284 answered). Plan 2 (S-WASTE-OUT) Steps 376 and 388 are live and their 24 h "after" readings are written (Phases 21 and 22). Plan 4 (S-WEST-GROUND) is closed: the databases are on the NVMe and the reboot of its Phase 11 is past. Phase 2 proves each or stops. Phases 0 and 1 need none of them.
**Target**: Seven days of readings, one a day with none missed, taken by the same script against the same bar as week 1, with a 24 h "before" for the Well's own latency and for the backup. A verdict table that one command prints. Stevie says which store stays; the other is switched off.
**Estimated duration**: 5 h of repository work (Phase 1, unattended, may start the day week 1 ends); 20 min with Stevie for the rulings; 30 min to start; 24 h baseline; 7 days of waiting with one automatic reading a day; 1 h to close; one more day if the index rider runs (LW2 = C).
**Agent strategy**: Opus. Phase 1 is one lane in its own worktree. Phases 2 to 7 are one coordinator; nothing runs in parallel on west's stores.
**Commit cadence**: every 3 to 5 steps and at every exit gate. Signed; on a gpg timeout use `--no-gpg-sign` and say so in the progress file. Named files only. Message `[PLAN S-LOG-WEEK-2] Steps X-Y: ...` plus the executing session's two trailers. Stevie pushes.
**Stuck protocol**: skip after 3x the step estimate or two failed debug attempts: mark `[STUCK]`, dependents `[BLOCKED]`, commit, go on to the next independent step. A reading that fails is never repeated in a loop: one retry, then it stands as recorded.

**Forged ahead of its turn.** The series index says plans 5 to 11 are forged when the plan before them closes. This one was written on 2026-11-17, before plans 2 and 4 ran live, on Stevie's word that day ("ensure we have some sprints set up for tomorrow"), because its Phase 1 is repository work that can run beside the attended sittings. Nothing in it rests on a number plans 2 and 4 have not yet measured: Phase 2 reads those numbers from their progress files and stops if one is missing.

## Grounding (checked 2026-11-17 03:45 to 05:00Z against `integrate/adr147` `38c3e513`; read only, no query sent to a store)

| # | Claim (source) | What is true now | The plan does |
|---|---|---|---|
| G1 | "Vector's batching and Postgres's limit are fixed, and a second, fair week is run" (ADR-147 decision 5) | Neither fix is live. `config/vector/vector.yaml` sink `clickhouse` has no `batch` block (plan 2 Step 368 adds one: 30 s). `docker-compose.yml` `postgres` is at `memory: 256M`; ClickHouse is at `2G`, `1.0` CPU (plan 2 Step 384 gives Postgres the same, `shared_buffers = 512MB`). | Phase 2 reads plan 2's "Handoffs" lines for the times of Steps 376 and 388 and checks both live. |
| G2 | The bar (S-E1-LIVE Step 278, answered 2026-11-10: (A) as written) | 2x disk, 2x query p95, under 10% movement of the Well's p95 and of backup time, zero stalls. Week 1 against it: `docs/battle-plans/progress/S-E1-LIVE-progress.md`, "Step 284, prepared for Stevie". | LW1. The same bar, so week 2 is an "after" of week 1 (series rule 15). |
| G3 | **Two rows cannot change.** | Disk: 1,454 to 1,464 bytes a row in Postgres against 64 to 72 in ClickHouse (20x to 23x). That is row format against column compression; neither fix touches it. Backup: the copy is inside the Well's nightly set (57 to 72 s and 80 to 102 MB by day 6, from 1.5 s and 0.35 MB). | Said here so the week is not read as open on those rows. What the week can move: the two slow queries (2 GiB instead of 256 MiB), the Well's own latency, the stall count. |
| G4 | The Well's latency row rested on one "before" reading (0.178 ms) | True, and the probe runs after the three queries in `scripts/logstore-measure.sh` (lines 117-133): by day 6 that is 14 minutes of sequential scans through a 256 MiB cache just before the probe. The probe may have measured the reading itself. | Phase 1 adds a second probe before the queries (the old one stays, so the row is still comparable). Phase 3 takes four "before" readings with the writer stopped. |
| G5 | The 11-16 reading held `NA` for q3 on Postgres | Twenty runs did not fit the script's `timeout 600` (line 81). A slow query read as a broken reading. | Phase 1: a time budget, the count of runs that finished is recorded, `NA` only when fewer than five did. |
| G6 | Vector's counters "0 in every other reading" | They start again at every Vector start (three in week 1), so a later reading covers only the time since. VictoriaMetrics scrapes Vector every 15 s (`config/victoria/scrape.yml:183`). | Phase 1: the two counts are also read as `increase(...[24h])` from VictoriaMetrics. |
| G7 | Two days of week 1 have no reading (11-13, 11-14) | The reading ran only when a session ran it. West's `sudo -n` works for the operator, and two user timers already run that way (`deploy/systemd/user/`). | LW4: a user timer, daily at 00:15Z (week 1's readings were taken near 00:10Z). |
| G8 | The copy's ceiling | `cmd/logstore-writer/main.go:58`: `--max-bytes` 4 GiB; over it `/ingest` answers 503 and Vector drops at that sink. The copy grows about 361 MB a day and held 2.19 GB on 11-17: it reaches 4 GiB about 2026-11-22 20Z, which would fall inside any second week that keeps the rows. Migration 048 keeps 14 days (5 GB at this rate). | LW3: start from an empty table (2.5 GB by day 7, under the ceiling). |
| G9 | Stopping the writer pages | After plan 1, `ContainerStopped` (`monitoring/prometheus/rules/containers.yml:28`) pages for a container that ran in the last 7 days. `scripts/monitoring/planned-stop.sh <name> <duration>` marks a stop as planned. | Step 47 marks it before Step 48 stops it. |
| G10 | Stopping the writer does not touch the record | The `well_logs` sink has its own memory buffer of 5,000 rows, `when_full: drop_newest` (`config/vector/vector.yaml:490-493`); the `clickhouse` sink has a disk buffer and `when_full: block`. With the writer down, Vector drops at `well_logs` only and counts it. No Vector restart is needed to pause the copy. | Step 50 proves it on the live counters before the baseline starts. |
| G11 | Neither slow query has an index that serves it | 048 creates the table partitioned by day with no index beyond what Erda set at Step 277. q2 is `message LIKE '%session opened%'` over 24 h; q3 is a 7-day count by `unit`. | LW2. Recommended: the week runs as week 1 did, then a one-day rider builds two indexes and reads twice more. |
| G12 | No test covers `scripts/logstore-measure.sh` | `git ls-files 'scripts/test-*logstore*'` is empty; the script is not in `scripts/check-gates-can-fail.sh`. | Phase 1 writes the test first (stubs for `sudo`, `docker`, `curl`, `systemctl` on `PATH`, no container). |

**Marshal (scope).** IN: the instrument's five changes (G4 to G7 and a reckoner), the timer, the baseline, the week, the verdict table, the rider if chosen, the off switch for the loser (S-E1-LIVE Steps 285 to 288, run here once). OUT: any change to ClickHouse's or Vector's settings (plan 2 did them; a change inside the week voids it); a migration of the log pipeline to Postgres if Postgres wins (a parked ADR, as S-E1-LIVE Step 285 says); retention of either store; any page or panel (rule 22). No new third party: `pg_trgm` ships inside the pinned Postgres image.

**Micromanager (QA / DoD).** Done is the progress table with no empty cell: LW-01 (instrument) red then green and proven to fail; LW-02 (baseline) four readings; LW-03 (the week) eight readings, one a day, each `[PASS]` or its `NA` explained; LW-04 (rider) two readings or "not chosen"; LW-05 (verdict) Stevie's words and the off switch's time.

**Busboy (handoffs).** Plan 2 gives the times of Steps 376 and 388. Plan 4 gives the date the new layout began and the reboot's time (its Phase 12). S-E1-LIVE: its Steps 285 to 288 are marked "run in S-LOG-WEEK-2" (Step 91), so the off switch is not run twice. Plan 8 (S-PROVING-RUN) wants five to seven quiet days on the new layout: whether this week may count as those days is asked of Stevie in Step 12 and is not assumed. Plan 6 (east) is not blocked by this week: nothing here touches east, and rule 16 forbids only what restarts or burdens west's three stores.

## Decisions

| # | Question | Options | Recommendation | Status |
|---|---|---|---|---|
| LW1 | The bar for week 2. | (A) bar (A) of 2026-11-10, unchanged; (B) the same, with absolute budgets for disk and backup written beforehand | **A.** Rule 15: an "after" is measured as the "before" was. G3 already says which rows cannot pass. | open, Step 8 |
| LW2 | Do the two slow queries get indexes on the Postgres side? | (A) no; (B) yes, from the first day; (C) the week runs without, then a one-day rider builds a trigram index on `message` and a `(host, ts, unit)` index, and reads twice more | **C.** The seven days stay comparable with week 1, and the question "would an index save it" gets a number with its cost in disk and insert time. | open, Step 9 |
| LW3 | What the copy holds when the week starts. | (A) empty: writer stopped, table truncated, 24 h baseline, then start; (B) keep the rows and raise `--max-bytes` to 8 GiB | **A.** Week 1 started empty; the ceiling stays as written; the baseline gives the Well's latency and the backup a true "before". Cost: the Postgres copy of 11-10 to today is deleted (ClickHouse keeps every row for 90 days). | open, Step 10 |
| LW4 | Who takes the daily reading. | (A) a user timer at 00:15Z; (B) a session, by hand | **A.** | open, Step 11 |
| LW5 | Does this week count as plan 8's quiet baseline? | (A) no, plan 8 measures its own after plans 6 and 7; (B) yes, if no restart falls inside it | **A.** Plan 8's baseline is for the layout with east and a tenant in it. | open, Step 12 |

---

## LEGEND

- `[B]`: bash command, run directly
- `[V]`: verification; must pass before going on
- `[D]`: debug step, only if the step before it failed
- `[W]`: write or create a file
- `[R]`: read or inspect a file
- `[S]`: needs sudo (Opus only)
- `[C]`: commit checkpoint (signed, named files)
- `[STEVIE]`: Stevie runs it, watches it or says yes first
- `[DECIDE]`: a ruling with options and a recommendation; ask it as one AskUserQuestion, recommended option first
- `[STUCK]` / `[BLOCKED]`: set by the Skip Protocol

`~Nm` is the expected wall-clock time. Paths: `REPO=/home/govan/tmp/unheaded`, `WT=/home/govan/tmp/wt/S-LOG-WEEK-2`, `PROG=docs/battle-plans/progress/S-LOG-WEEK-2-progress.md`, `TSV1=/home/govan/.local/state/unheaded/S-E1-LIVE-logstore.tsv` (week 1, never written again after its closing reading), `TSV2=/home/govan/.local/state/unheaded/S-LOG-WEEK-2-logstore.tsv`, `S=/home/govan/.local/state/unheaded/log-week-2` (start and end times, the disturbance log; on the host, never in the repository). Compose for the main project is always `cd /home/govan/tmp/unheaded && sudo docker compose ...` with no `-f`, and `stop` / `start`, never `down`. The Well is reached as the script reaches it: `sudo docker exec -i unheaded-postgres sh -c 'psql -X -q -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d unheaded_ops'` with the statement on standard input (written `PSQL_OPS` below). No step prints `.env`, a key file, a container's environment, or a log row: counts and sizes only.

**The progress table.** `$PROG` holds one row per item: id, source, evidence, fix, owner, before, after, status.

| id | Source | Item | Phase |
|---|---|---|---|
| LW-01 | G4, G5, G6, G12 | the instrument: probe before the queries, time budget, 24 h increases, restart times, a test, a reckoner | 1 |
| LW-02 | G4, G7 | the timer; the 24 h baseline with the writer stopped | 1, 3 |
| LW-03 | decision 5 | the seven days | 4, 5 |
| LW-04 | G11, LW2 | the index rider | 6 |
| LW-05 | S-E1-LIVE Steps 284 to 288 | the verdict; the loser switched off; ADR-117 amended | 7 |

---

## PHASE 0: THE GATE AND THE RULINGS (Steps 1-14)

**Goal**: Week 1 is closed, the rulings are recorded, the worktree exists.
**Prerequisite**: `integrate/adr147` is merged into `develop` (this plan's file and `scripts/monitoring/planned-stop.sh` arrive with it).
**Time**: 30 min.
**Model**: Opus. **Agent**: Coordinator; Stevie for Steps 7 to 12.

- [ ] **Step 1** [R] ~10m: Read ADR-147 "Stevie's answers", item 5; this plan's Grounding; `docs/battle-plans/progress/S-E1-LIVE-progress.md` from "Evening 16" to the end of "Step 284, prepared for Stevie"; `scripts/logstore-measure.sh` in full; `docs/battle-plans/S-WASTE-OUT-battle-plan.md` Phases 21 and 22.
- [ ] **Step 2** [B] ~1m: Week 1's closing reading exists.
  ```bash
  cut -f1 /home/govan/.local/state/unheaded/S-E1-LIVE-logstore.tsv | grep -E '^2026' | sort -u | tail -1
  ```
- [ ] **Step 3** [V] ~1m: A time at or after `2026-11-17T18:34`. Earlier → take it (`cd /home/govan/tmp/unheaded && scripts/logstore-measure.sh; echo rc=$?`), once, and go on.
- [ ] **Step 4** [B] ~1m: `grep -n -c 'Step 284' /home/govan/tmp/unheaded/docs/battle-plans/progress/S-E1-LIVE-progress.md; grep -n -i 'step 284.*answered\|which store stays' /home/govan/tmp/unheaded/docs/battle-plans/progress/S-E1-LIVE-progress.md | tail -3`
- [ ] **Step 5** [V] ~1m: A line records Stevie's answer at Step 284. None → Phase 1 may still run; Phases 2 to 7 are `[BLOCKED]` until he answers (his words go into S-E1-LIVE's progress file as a new dated entry, never into an old one).
- [ ] **Step 6** [B] ~2m: The worktree.
  ```bash
  cd /home/govan/tmp/unheaded && git worktree add -b sprint/s-log-week-2 /home/govan/tmp/wt/S-LOG-WEEK-2 develop && mkdir -p /home/govan/.local/state/unheaded/log-week-2 && git -C /home/govan/tmp/wt/S-LOG-WEEK-2 log --oneline -1
  ```
- [ ] **Step 7** [STEVIE] ~3m: Say plainly what G3 says: disk and backup cannot pass bar (A) in any week; the week can move the two slow queries, the Well's own latency and the stall count. He may still want the week (decision 5 says so); his words go in `$PROG`.
- [ ] **Step 8** [STEVIE][DECIDE] ~2m: **LW1.**
- [ ] **Step 9** [STEVIE][DECIDE] ~2m: **LW2.**
- [ ] **Step 10** [STEVIE][DECIDE] ~2m: **LW3.** Say the cost aloud: the Postgres copy of week 1 is deleted; `TSV1` keeps its numbers.
- [ ] **Step 11** [STEVIE][DECIDE] ~1m: **LW4.**
- [ ] **Step 12** [STEVIE][DECIDE] ~1m: **LW5.**
- [ ] **Step 13** [W] ~5m: In `WT`, create `$PROG` with the SPDX header, the table above (before, after and status empty), a "Rulings" section with each answer in his words and the time, a "Handoffs" section and a "For Stevie" section. Stevie away → write "not asked; the recommendation is taken for Phase 1 only", and Phases 3 to 7 wait for him.
- [ ] **Step 14** [C][V] ~1m: **PHASE 0 EXIT GATE.** **COMMIT** `$PROG` by name.

---

## PHASE 1: THE INSTRUMENT (Steps 15-38)

**Goal**: The reading cannot mistake a slow query for a broken one, measures the Well before it loads it, survives a Vector restart, runs without a session, and has a test that fails when it is broken. One command prints the verdict table.
**Prerequisite**: Step 6. Nothing in this phase touches a running store: every run of the script here is against stubs.
**Time**: 5 h.
**Model**: Opus. **Agent**: one lane in `WT`.

- [ ] **Step 15** [W] ~40m: The test first: `scripts/test-logstore-measure.sh` (SPDX header; `set -euo pipefail`; a temporary directory removed on exit). It puts stub `sudo`, `docker`, `curl`, `systemctl`, `timeout` and `du` first on `PATH`: `sudo` runs its arguments; `docker exec` prints what a file named by `STUB_MODE` says for the statement it is given (a count, or twenty lines of times); `curl` prints a fixed Vector metrics page for port 9598 and a fixed VictoriaMetrics answer for port 8428. Cases, each asserting on the block appended to a temporary TSV:
  1. all good: exit 0, `[PASS]`, every metric of week 1 present, plus the new ones of Steps 17 to 21.
  2. the Postgres q3 stub sleeps past a budget of 2 s after six runs: `pg_q3_units_7d_runs` is `6`, p50 and p95 are numbers, exit 0.
  3. the same with two runs: p50 and p95 are `NA`, `pg_q3_units_7d_runs` is `2`, exit 1.
  4. `well_probe_first_p95_ms` appears on an earlier line of the block than `ch_q1_last_hour_p50_ms`, and `well_probe_p95_ms` on a later line than `pg_q3_units_7d_p95_ms`.
  5. VictoriaMetrics unreachable: the two `_24h` metrics are `NA`, the raw counters are numbers, exit 1.
  6. no row, message or unit name reaches the TSV: the stub's rows carry the word `CANARY`; `grep -c CANARY` on the TSV is `0`.
- [ ] **Step 16** [B][V] ~2m: `cd /home/govan/tmp/wt/S-LOG-WEEK-2 && bash scripts/test-logstore-measure.sh; echo rc=$?` → red: cases 2 to 5 fail (case 1 fails on the new metrics). A green run here means the stubs are not reached: fix the test, not the script.
- [ ] **Step 17** [W] ~30m: `scripts/logstore-measure.sh`: the time budget. `QUERY_BUDGET_S` (default `900`, a number of 60 or more). `ch_times` and `pg_times` run the statement one at a time and stop when `RUNS` runs are done or the budget is spent, whichever is first; each `timed` call also writes `ch_<name>_runs` and `pg_<name>_runs`. `quantiles` gives p50 and p95 from what finished when that is five or more (p95 is the nearest rank), else nothing, and the metric is `NA`. The outer `timeout 600` becomes `timeout $((QUERY_BUDGET_S + 120))`. Header comment: the budget, and "a slow query is a number, not a broken reading (the 2026-11-16 reading)".
- [ ] **Step 18** [W] ~10m: The same file: the Well's own latency is read twice. Move nothing; add, before the block "The three queries", the same `kanban_tasks` probe written as `well_probe_first_p50_ms` and `well_probe_first_p95_ms`, with the comment "before the queries load the Well's cache; `well_probe_*` below stays where week 1 had it".
- [ ] **Step 19** [W] ~20m: The same file: Vector over 24 h, from VictoriaMetrics (`VICTORIA_URL`, default `http://127.0.0.1:8428`), by `curl -fsS -m 10 -G "$VICTORIA_URL/api/v1/query" --data-urlencode "query=..."` and a `python3 -c` that prints the first value or nothing: `vector_record_sends_over_1s_24h` = `sum(increase(vector_buffer_send_duration_seconds_count{component_id="clickhouse"}[24h])) - sum(increase(vector_buffer_send_duration_seconds_bucket{component_id="clickhouse",le="1"}[24h]))`, rounded; `vector_well_dropped_24h` = `sum(increase(vector_component_discarded_events_total{component_id="well_logs"}[24h]))`, an empty answer for this one is `0` (no series means no drop). The four raw metrics stay as they are.
- [ ] **Step 20** [W] ~15m: The same file: what restarted. `ch_started_unix`, `pg_started_unix`, `vector_started_unix`, `writer_started_unix`: `sudo docker inspect -f '{{.State.StartedAt}}' <container>` through `date -u -d ... +%s` (`NA` when the container is not running; for `unheaded-logstore-writer` during the baseline that is expected, so this one metric does not set the failure flag). `host_boot_unix`: `date -u -d "$(uptime -s)" +%s`. `io_some_avg300`: the `avg300` field of the `some` line of `/proc/pressure/io`.
- [ ] **Step 21** [W] ~5m: The same file: the header's metric list gains the new names; the default output stays `TSV1`'s path (week 2 passes `LOGSTORE_TSV`).
- [ ] **Step 22** [B][V] ~3m: `cd /home/govan/tmp/wt/S-LOG-WEEK-2 && bash scripts/test-logstore-measure.sh; echo rc=$?; shellcheck scripts/logstore-measure.sh scripts/test-logstore-measure.sh && echo SC_OK` → six `PASS`, `rc=0`, `SC_OK`.
- [ ] **Step 23** [W] ~15m: Register the test: a row for `test-logstore-measure` in `scripts/check-gates-can-fail.sh` (read the file's header for the row format; provocation: in a copy, the run count's floor of five becomes zero, which case 3 must catch), a line in `scripts/ci-local.sh` beside the other `scripts/test-*.sh` calls, and the step in `.github/workflows/` that runs those tests (find it: `git grep -n 'test-planned-stop' -- .github scripts/ci-local.sh`).
- [ ] **Step 24** [B][V] ~3m: `cd /home/govan/tmp/wt/S-LOG-WEEK-2 && scripts/check-gates-can-fail.sh --only test-logstore-measure 2>&1 | tail -2` → `[PASS]`: the provoked copy fails.
- [ ] **Step 25** [C] ~1m: **COMMIT** the script, its test and the three registration files by name.
- [ ] **Step 26** [W] ~30m: The reckoner's test first: `tests/scripts/test_logstore_reckon.py`. It writes small TSVs and runs `scripts/logstore-reckon.py`: (1) week 1's shape (a baseline reading, six readings) gives a table with the seven rows of the bar and, for a file built from the figures in S-E1-LIVE's "Step 284, prepared", the same verdict on each row; (2) `--start` and `--end` limit the readings used; `--baseline-until` names the readings that are the "before"; (3) a reading with `NA` is listed by time and metric and left out of that row's range, never read as zero; (4) with `_runs` present, a row whose p95 came from fewer than `RUNS` runs is marked `(n runs)`; (5) output holds numbers and metric names only.
- [ ] **Step 27** [B][V] ~1m: `cd /home/govan/tmp/wt/S-LOG-WEEK-2 && /var/tmp/unheaded-sprint/civenv/bin/python -m pytest -q tests/scripts/test_logstore_reckon.py 2>&1 | tail -3` → red: the script does not exist.
- [ ] **Step 28** [W] ~45m: `scripts/logstore-reckon.py` (SPDX header; standard library only; `argparse`): reads a readings file, prints a Markdown table: test, bar, ClickHouse range, Postgres range, ratio range, verdict. Rows: disk bytes a row; q1, q2, q3 p95; the Well's probe (both probes, against the baseline's median); backup seconds and bytes against the baseline; sends over 1 s and rows dropped (raw and `_24h`). Under the table: readings used, readings with `NA`, restarts seen (a `_started_unix` or `host_boot_unix` that changed between two readings, with the two times), the highest `io_some_avg300`. Exit 0 when it printed a table, 2 on a file it cannot read.
- [ ] **Step 29** [B][V] ~2m: Step 27's command → all pass. Then against week 1's real file, which is the check that it reads true data: `cd /home/govan/tmp/wt/S-LOG-WEEK-2 && python3 scripts/logstore-reckon.py /home/govan/.local/state/unheaded/S-E1-LIVE-logstore.tsv --baseline-until 2026-11-10T19 --start 2026-11-11T12` → the ratios of S-E1-LIVE's "Step 284, prepared" table (20x to 23x; 177x to 619x; 391x to 1,833x). A different figure → find which is wrong before going on; the progress file's Python block is the reference.
- [ ] **Step 30** [C] ~1m: **COMMIT** both files by name.
- [ ] **Step 31** [W] ~15m: The timer, if LW4 = A: `deploy/systemd/user/unheaded-logstore-measure.service` (`Type=oneshot`; `WorkingDirectory=%h/tmp/unheaded`; `Environment=LOGSTORE_TSV=%h/.local/state/unheaded/S-LOG-WEEK-2-logstore.tsv`; `ExecStart=%h/tmp/unheaded/scripts/logstore-measure.sh`; `TimeoutStartSec=90min`; `SuccessExitStatus=1`, with the comment "1 is a reading with an NA in it: recorded, read at the close, not a failed unit"; an install comment modelled on `unheaded-compliance-evidence.service`) and `unheaded-logstore-measure.timer` (`OnCalendar=*-*-* 00:15:00 UTC`; `Persistent=true`; no randomized delay: the readings are compared by hour). A row in `wiki/Systemd-Units.md`.
- [ ] **Step 32** [B][V] ~1m: `systemd-analyze verify --user /home/govan/tmp/wt/S-LOG-WEEK-2/deploy/systemd/user/unheaded-logstore-measure.service /home/govan/tmp/wt/S-LOG-WEEK-2/deploy/systemd/user/unheaded-logstore-measure.timer 2>&1 | grep -v 'unheaded-logstore-measure.service: Command .* is not executable' | head -5` → empty.
- [ ] **Step 33** [B] ~10m: The gates a script and a unit must pass: `cd /home/govan/tmp/wt/S-LOG-WEEK-2 && scripts/check-spdx.sh 2>&1 | tail -1; scripts/check-unit-start-limits.sh 2>&1 | tail -1; scripts/check-live-paths.sh 2>&1 | tail -1; scripts/check-ci-verdicts.sh 2>&1 | tail -1` (a gate named here that does not exist under that name: `ls scripts/check-*.sh | grep -i -E 'spdx|unit|live|verdict'` and run what it shows).
- [ ] **Step 34** [V] ~1m: Each passes, or the new files are added to its list as that gate's message says.
- [ ] **Step 35** [W] ~10m: `wiki/Log-Aggregation.md` (or the page that names `logstore-measure.sh`: `git grep -ln 'logstore-measure' -- wiki`): the new metrics, the budget, the timer, the reckoner's command.
- [ ] **Step 36** [C] ~1m: **COMMIT** the units, the wiki pages and any gate list by name.
- [ ] **Step 37** [B] ~6m: Merge to develop (guarded, as the series does it).
  ```bash
  cd /home/govan/tmp/wt/S-LOG-WEEK-2 && git merge --no-ff -S develop -m "Merge develop into sprint/s-log-week-2" && bash scripts/test-logstore-measure.sh | tail -1 && cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && git merge --no-ff -S sprint/s-log-week-2 -m "[PLAN S-LOG-WEEK-2] Phase 1: the instrument" && git log --oneline -1
  ```
  A gpg timeout → the same two merges with `--no-gpg-sign`, and say so in `$PROG`.
- [ ] **Step 38** [C][V] ~2m: **PHASE 1 EXIT GATE.** `$PROG`: LW-01's "after" (six cases, the gate proven to fail, the reckoner agreeing with week 1's table). **COMMIT** `$PROG` by name. The running week 1 timer does not exist and nothing live has changed.

---

## PHASE 2: THE PRECONDITIONS, PROVEN (Steps 39-46)

**Goal**: Proof that the week would be fair, read from the other plans' records and from the running system.
**Prerequisite**: Phase 1 gate; Step 5 passed.
**Time**: 20 min.
**Model**: Opus. **Agent**: Coordinator.

- [ ] **Step 39** [B] ~2m: `grep -n -i 'plan 5' /home/govan/tmp/unheaded/docs/battle-plans/progress/S-WASTE-OUT-progress.md | tail -4; grep -n -i 'plan 5\|new layout' /home/govan/tmp/unheaded/docs/battle-plans/progress/S-WEST-GROUND-progress.md | tail -4`
- [ ] **Step 40** [V] ~2m: Three lines with times: "Vector batched (30 s) since ..." (plan 2 Step 381), "Postgres at 2G / 1.0 CPU since ..." (Step 393), and plan 4's date for the new layout with its reboot past. One missing → stop: this plan is `[BLOCKED]` on that plan, and `$PROG` says which line.
- [ ] **Step 41** [S][B] ~2m: The same, read from what runs.
  ```bash
  grep -n -A4 'max_events: 100000' /home/govan/tmp/unheaded/config/vector/vector.yaml | head -6; sudo docker inspect unheaded-postgres -f '{{.HostConfig.Memory}} {{.HostConfig.NanoCpus}}'; sudo docker inspect unheaded-clickhouse -f '{{.HostConfig.Memory}} {{.HostConfig.NanoCpus}}'; findmnt -n -o SOURCE,TARGET -T "$(sudo docker volume inspect unheaded-dev_postgres-data -f '{{.Mountpoint}}')"
  ```
- [ ] **Step 42** [V] ~1m: The batch block with `timeout_secs: 30`; the same two numbers for both containers (`2147483648 1000000000`); the Postgres volume on the NVMe's logical volume, not on `sdb`. Anything else → stop, as Step 40.
- [ ] **Step 43** [B] ~2m: Both of plan 2's 24 h "after" readings exist: `grep -n -E '^\| *W-(28|30) ' /home/govan/tmp/unheaded/docs/battle-plans/progress/S-WASTE-OUT-progress.md | cut -c1-300`
- [ ] **Step 44** [V] ~1m: Both rows have an "after". A row marked `[STUCK]` or with a missed bar → `[STEVIE]`: the week may start only on his word, and `$PROG` records that the fix it was meant to follow did not meet its bar.
- [ ] **Step 45** [S][B] ~1m: Room: `df -B1G --output=avail "$(sudo docker volume inspect unheaded-dev_postgres-data -f '{{.Mountpoint}}')" | tail -1` → at least `12` (2.5 GB of rows, the rider's indexes, ClickHouse's growth and headroom). Less → `[STEVIE]`.
- [ ] **Step 46** [C][V] ~1m: **PHASE 2 EXIT GATE.** `$PROG` "Handoffs" holds the three times and the readings of Steps 41 and 45. **COMMIT** `$PROG` on develop by name.

---

## PHASE 3: THE BASELINE (Steps 47-60)

**Goal**: The copy is paused and empty; four readings and one nightly backup say what the Well does without it on the new layout.
**Prerequisite**: Phase 2 gate; LW3 = A (LW3 = B: skip to Step 61 after raising the ceiling; see the note under Step 61). Stevie's yes for Step 49.
**Time**: 30 min, then 24 h.
**Model**: Opus. **Agent**: Coordinator.

- [ ] **Step 47** [S][B] ~1m: `cd /home/govan/tmp/unheaded && scripts/monitoring/planned-stop.sh logstore-writer 26h && scripts/monitoring/planned-stop.sh --list`
- [ ] **Step 48** [S][B] ~1m: `cd /home/govan/tmp/unheaded && sudo docker compose stop logstore-writer && sudo docker ps -a --filter name=unheaded-logstore-writer --format '{{.Status}}' && date -u +%FT%TZ | tee /home/govan/.local/state/unheaded/log-week-2/writer-stopped`
- [ ] **Step 49** [STEVIE] ~1m: "Delete the Postgres copy of week 1 now (`TRUNCATE unheaded_ops.log_events`)? ClickHouse keeps every row; `TSV1` keeps the numbers." No → LW3 = B after all; go to the note under Step 61.
- [ ] **Step 50** [S][B] ~2m: The record is untouched while the copy's sink fails (G10): `curl -fsS -m 10 http://172.17.0.1:9598/metrics | grep -E '^vector_(buffer_events|component_sent_events_total|component_discarded_events_total)\{' | grep -E 'component_id="(clickhouse|well_logs)"' | cut -c1-160`, twice, 2 minutes apart. Expect: `clickhouse` sent events rising and its buffer near 0; `well_logs` discards rising or its buffer at 5,000.
- [ ] **Step 51** [V] ~1m: As expected. The `clickhouse` buffer climbing → the record is being held up by the copy, which G10 says cannot happen: `cd /home/govan/tmp/unheaded && sudo docker compose start logstore-writer`, end the mark (`scripts/monitoring/planned-stop.sh --end logstore-writer`), `[STUCK]`, and write what the counters showed.
- [ ] **Step 52** [S][B] ~1m: Empty the copy; sizes only.
  ```bash
  printf '%s\n' "SELECT pg_size_pretty(coalesce(sum(pg_total_relation_size(inhrelid)), 0)) FROM pg_inherits WHERE inhparent = 'public.log_events'::regclass;" "TRUNCATE public.log_events;" "SELECT count(*) FROM log_events;" | sudo docker exec -i unheaded-postgres sh -c 'psql -X -q -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d unheaded_ops'
  ```
- [ ] **Step 53** [V] ~1m: A size, then `0`. An error → nothing was deleted (one transaction per statement, the `TRUNCATE` failed): read the message, one retry, then `[STUCK]` and Step 51's restart.
- [ ] **Step 54** [B] ~2m: The timer, if LW4 = A (it reads the baseline too).
  ```bash
  mkdir -p /home/govan/.config/systemd/user && cp /home/govan/tmp/unheaded/deploy/systemd/user/unheaded-logstore-measure.service /home/govan/tmp/unheaded/deploy/systemd/user/unheaded-logstore-measure.timer /home/govan/.config/systemd/user/ && systemctl --user daemon-reload && systemctl --user enable --now unheaded-logstore-measure.timer && systemctl --user list-timers unheaded-logstore-measure.timer --no-pager | head -3
  ```
- [ ] **Step 55** [S][B] ~25m: Baseline reading 1 of 4, now: `cd /home/govan/tmp/unheaded && LOGSTORE_TSV=/home/govan/.local/state/unheaded/S-LOG-WEEK-2-logstore.tsv scripts/logstore-measure.sh; echo rc=$?`. The Postgres side reads an empty table: its three queries are fast and its `rows` are 0; that is the baseline, not a fault. `writer_started_unix` is `NA` by design.
- [ ] **Step 56** [B] ~1m each: Readings 2, 3 and 4 at about 6, 12 and 18 hours after Step 55 (the timer's 00:15Z run may be one of them). A session that is not there for one of them: three readings stand; fewer than three → the baseline runs 12 hours longer and the mark of Step 47 is renewed first.
- [ ] **Step 57** [B] ~1m: After the nightly backup (03:37Z): `systemctl show unheaded-well-backup.service -p ExecMainStartTimestamp -p ExecMainExitTimestamp -p Result --no-pager` → `Result=success` and a start after Step 52's time. The next reading carries its seconds and bytes: the backup's "before".
- [ ] **Step 58** [B][V] ~2m: `python3 /home/govan/tmp/unheaded/scripts/logstore-reckon.py /home/govan/.local/state/unheaded/S-LOG-WEEK-2-logstore.tsv` → at least three readings, no `NA` but `writer_started_unix`, no restart seen, a backup taken inside the baseline.
- [ ] **Step 59** [W] ~5m: `$PROG` LW-02: the readings' times, the medians of both probes, the backup's seconds and bytes, the highest `io_some_avg300`.
- [ ] **Step 60** [C][V] ~1m: **PHASE 3 EXIT GATE.** **COMMIT** `$PROG` on develop by name.

---

## PHASE 4: THE WEEK STARTS (Steps 61-68)

**Goal**: Both stores take the same rows from one moment, and that moment is written down.
**Prerequisite**: Phase 3 gate.
**Time**: 15 min.
**Model**: Opus. **Agent**: Coordinator.

- [ ] **Step 61** [S][B] ~1m: `cd /home/govan/tmp/unheaded && sudo docker compose start logstore-writer && date -u +%FT%TZ | tee /home/govan/.local/state/unheaded/log-week-2/T0 && scripts/monitoring/planned-stop.sh --end logstore-writer`
  **LW3 = B instead:** no stop, no truncate, no baseline. In `WT`, `docker-compose.yml`, `logstore-writer` `command`: `--max-bytes=8589934592` with the comment "the second log store week keeps 14 days (5 GB); ADR-147 decision 5"; `scripts/check-compose-hardening.sh`; commit; merge to develop; `sudo docker compose up -d --no-build --no-deps logstore-writer`; write that time as `T0`. The Well's latency and backup rows then have no "before" on the new layout, and the verdict table says so.
- [ ] **Step 62** [S][B] ~3m: After 2 minutes: `printf '%s\n' "SELECT count(*) FROM log_events;" | sudo docker exec -i unheaded-postgres sh -c 'psql -X -q -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d unheaded_ops'`, twice, a minute apart; `sudo docker ps --filter name=unheaded-logstore-writer --format '{{.Status}}'`.
- [ ] **Step 63** [V] ~1m: The count rises; the writer is `healthy`. Not rising → `sudo docker logs unheaded-logstore-writer --since 5m 2>&1 | grep -ciE 'error|refused|401|503'` (a count, not the lines); a rejected token or a 503 is read from the count's kind with `grep -c` per word. Two tries, then `[STUCK]`; the baseline stays valid for 48 h.
- [ ] **Step 64** [B] ~1m: The end: `date -u -d "$(cat /home/govan/.local/state/unheaded/log-week-2/T0) + 7 days" +%FT%TZ | tee /home/govan/.local/state/unheaded/log-week-2/T-END`
- [ ] **Step 65** [W] ~5m: `$PROG`: `T0`, `T-END`, and the week's rules in four lines: nothing restarts, rebuilds or recreates Vector, ClickHouse, Postgres or the writer; nothing stops the stack; no walk of the databases' volume or of `/var`; no image build on west. What may run: repository work, gates that need no container, anything on east (attended), reads of VictoriaMetrics. A disturbance that happens anyway is one line in `/home/govan/.local/state/unheaded/log-week-2/disturbances` (time, what, how long) and in `$PROG` at the close.
- [ ] **Step 66** [W] ~3m: `~/tmp/next.md`, at the top: `T0`, `T-END`, the four rules, "the reading is taken by the timer at 00:15Z; check it with `systemctl --user status unheaded-logstore-measure.service`".
- [ ] **Step 67** [C] ~1m: **COMMIT** `$PROG` on develop by name.
- [ ] **Step 68** [V] ~1m: **PHASE 4 EXIT GATE**: `T0` and `T-END` exist; the count rises; next.md says so.

---

## PHASE 5: SEVEN DAYS (Steps 69-76)

**Goal**: Eight readings from `T0` to `T-END`, one a day, none missed.
**Prerequisite**: Phase 4 gate.
**Time**: 7 days; 2 minutes a day.
**Model**: Opus. **Agent**: whichever session is open; nothing here needs Stevie.

- [ ] **Step 69** [S][B] ~25m: The first reading, one hour after `T0` (the copy holds an hour, so q1 has rows): as Step 55.
- [ ] **Step 70** [B] ~2m, each day: `systemctl --user show unheaded-logstore-measure.service -p ExecMainExitTimestamp -p ExecMainStatus --no-pager; cut -f1 /home/govan/.local/state/unheaded/S-LOG-WEEK-2-logstore.tsv | sort -u | tail -2`
- [ ] **Step 71** [V] ~1m: A reading dated today, status `0` or `1`. No reading by 01:30Z → take it by hand (Step 55's command) and write one line in `disturbances` saying the timer missed and why (`journalctl --user -u unheaded-logstore-measure.service --since -2h --no-pager | tail -5`).
- [ ] **Step 72** [D] ~5m: Status `1`: `tail -40 /home/govan/.local/state/unheaded/S-LOG-WEEK-2-logstore.tsv | grep -P '\tNA$'`. `_runs` under 5 on a Postgres query → the query now takes over 3 minutes a run: that is a result, recorded as such. A ClickHouse or Vector `NA` → one retry by hand an hour later; both readings stand.
- [ ] **Step 73** [D] ~5m: A restart inside the week (the reckoner's "restarts seen", or a page). Vector, ClickHouse, Postgres or the writer restarted once, for under 10 minutes → the week goes on; `disturbances` and `$PROG` get the line. Twice, or the host rebooted, or a store was down over 10 minutes → `T-END` moves out by one day for each day touched; rewrite `T-END` and next.md. The table is never emptied again mid-week.
- [ ] **Step 74** [D] ~5m: The writer answers 503 (`unheaded_logstore_stored_bytes` at `unheaded_logstore_max_bytes`; with LW3 = A not before day 11): not expected inside seven days; if seen, `[STEVIE]`, and the reading of that day says the copy was full.
- [ ] **Step 75** [W] ~2m, on days 3 and 5: one line in `$PROG`: readings so far, any `NA`, any disturbance.
- [ ] **Step 76** [V] ~1m: **PHASE 5 EXIT GATE**: the clock is past `T-END` (`test "$(date -u +%s)" -ge "$(date -u -d "$(cat /home/govan/.local/state/unheaded/log-week-2/T-END)" +%s)" && echo OVER`).

---

## PHASE 6: THE CLOSE, AND THE RIDER (Steps 77-86)

**Goal**: The closing reading; the table; if LW2 = C, what two indexes buy and cost.
**Prerequisite**: Phase 5 gate.
**Time**: 45 min; with the rider, one more day.
**Model**: Opus. **Agent**: Coordinator.

- [ ] **Step 77** [S][B] ~25m: The closing reading, by hand, after `T-END`: Step 55's command. Then `echo "$(date -u +%FT%TZ)" > /home/govan/.local/state/unheaded/log-week-2/CLOSED`.
- [ ] **Step 78** [B] ~1m: `python3 /home/govan/tmp/unheaded/scripts/logstore-reckon.py /home/govan/.local/state/unheaded/S-LOG-WEEK-2-logstore.tsv --baseline-until "$(cat /home/govan/.local/state/unheaded/log-week-2/T0)" --start "$(cat /home/govan/.local/state/unheaded/log-week-2/T0)" --end "$(cat /home/govan/.local/state/unheaded/log-week-2/CLOSED)"`
- [ ] **Step 79** [W] ~15m: `$PROG` LW-03: the table as printed, week 1's table beside it (from S-E1-LIVE's progress file), the readings with `NA`, the disturbances file's lines, the restarts seen. No verdict is written here: that is Stevie's (Step 87).
- [ ] **Step 80** [C] ~1m: **COMMIT** `$PROG` on develop by name. LW2 = A or B → go to Phase 7.
- [ ] **Step 81** [R] ~10m: (LW2 = C.) Erda pass (`unheaded-erda`) on the two indexes before they are built: the default is `CREATE EXTENSION IF NOT EXISTS pg_trgm` and `CREATE INDEX log_events_message_trgm ON log_events USING gin (message gin_trgm_ops)` for q2, `CREATE INDEX log_events_host_ts_unit ON log_events (host, ts, unit)` followed by `VACUUM (ANALYZE) log_events` for q3 (an index-only scan needs the visibility map). Erda may change the shape; what is built is written in `$PROG` before it is built.
- [ ] **Step 82** [S][B] ~20m: Build, timing each statement (`\timing on` first on standard input), through `PSQL_OPS`. An index on the partitioned parent cannot be built concurrently: inserts wait while each partition's index is built, the writer answers slowly, and Vector may drop at `well_logs` for those minutes. That is why the rider runs after the closing reading. Record each statement's time and `SELECT pg_size_pretty(sum(pg_relation_size(indexrelid))) FROM pg_index WHERE indrelid IN (SELECT inhrelid FROM pg_inherits WHERE inhparent = 'public.log_events'::regclass)`.
- [ ] **Step 83** [S][B] ~25m each: Two readings, 12 hours apart, into a file of their own: `cd /home/govan/tmp/unheaded && LOGSTORE_TSV=/home/govan/.local/state/unheaded/S-LOG-WEEK-2-rider.tsv scripts/logstore-measure.sh; echo rc=$?`. The timer stays on its own file and its 00:15Z reading is not part of the rider.
- [ ] **Step 84** [B] ~2m: `python3 /home/govan/tmp/unheaded/scripts/logstore-reckon.py /home/govan/.local/state/unheaded/S-LOG-WEEK-2-rider.tsv`, and the writer's insert time before and after from VictoriaMetrics: `curl -s -m5 -G http://127.0.0.1:8428/api/v1/query --data-urlencode 'query=histogram_quantile(0.95, sum by (le) (rate(unheaded_logstore_insert_duration_seconds_bucket[6h])))'` at the closing reading's time (`&time=`) and now (the metric's name is read from `curl -s http://127.0.0.1:8428/api/v1/label/__name__/values | tr ',' '\n' | grep logstore` if this one is absent).
- [ ] **Step 85** [W] ~10m: `$PROG` LW-04: q2 and q3 p95 with the indexes against without, the indexes' size and the disk row with them added, build time, insert time before and after.
- [ ] **Step 86** [C][V] ~1m: **PHASE 6 EXIT GATE.** **COMMIT** `$PROG` on develop by name.

---

## PHASE 7: THE VERDICT AND THE OFF SWITCH (Steps 87-96)

**Goal**: Stevie says which store stays. The other stops taking rows. The records say so.
**Prerequisite**: Phase 6 gate; Stevie present.
**Time**: 1 h.
**Model**: Opus. **Agent**: Coordinator with Stevie.

- [ ] **Step 87** [STEVIE][V] ~20m: Read LW-03's two tables (and LW-04) with him against the bar he chose at LW1. He says which store stays. His words, dated, go in `$PROG` and as a new entry in `docs/battle-plans/progress/S-E1-LIVE-progress.md`.
- [ ] **Step 88** [B] ~2m: Stop the timer either way: `systemctl --user disable --now unheaded-logstore-measure.timer && systemctl --user list-timers --no-pager | grep -c logstore-measure` → `0`.
- [ ] **Step 89** [W][S] ~20m: **ClickHouse stays** (what week 1 points to): the off switch of S-E1-LIVE's Step 281 entry. In `WT` (after `git merge --ff-only develop`): remove the `well_logs` sink and the `well_rows` transform from `config/vector/vector.yaml`; `scripts/test-vector-config.sh`; commit; merge to develop. Then `cd /home/govan/tmp/unheaded && scripts/monitoring/planned-stop.sh logstore-writer 7d && sudo docker restart unheaded-vector && sudo docker compose stop logstore-writer`. After 2 minutes Vector's `clickhouse` sent events are rising and its log holds no `error` line (a count). The table, the role, the service's code and its compose entry stay until Stevie says drop: `DROP TABLE` and the removal of the writer are a `[STEVIE]` line in `$PROG` "For Stevie", with the size it frees. **Postgres stays**: nothing is switched off; Vector to ClickHouse stays as it is until a sprint moves the readers; that sprint is a parked ADR (next free number), not this plan's work.
- [ ] **Step 90** [W] ~20m: `docs/adr/ADR-117-*.md`: a dated amendment at the end with both weeks' tables, the rider, the decision and what was switched off; nothing above it is edited. `docs/adr/ADR-INDEX.md` row 117. `docs/adr/ADR-147-every-service-earns-its-place.md`: a dated update, "decision 5 closed", three lines. `wiki/Log-Aggregation.md` and `wiki/The-Well.md`: which store holds the logs.
- [ ] **Step 91** [W] ~5m: `docs/battle-plans/S-E1-LIVE-battle-plan.md` Steps 285 to 288: marked "run in S-LOG-WEEK-2 Steps 89 to 93" (the steps' text stays). `docs/battle-plans/SPRINT-SERIES-2026-11-15-ADR-147.md` row 5: closed, with the date.
- [ ] **Step 92** [B] ~20m: `cd /home/govan/tmp/unheaded && PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin scripts/ci-local.sh 2>&1 | tail -25` → no failure that this plan's files cause.
- [ ] **Step 93** [C] ~1m: **COMMIT** the ADRs, the index, the two plans' files, the wiki pages and `$PROG` by name.
- [ ] **Step 94** [W] ~10m: `$PROG` "Handoffs": **plan 8**: whether this week counts as its baseline (LW5), and the readings file's path; **plan 9**: the slow statements' times under 2 GiB. "For Stevie": the `DROP TABLE` and the writer's removal, or the parked ADR's number; the backup sets that carry the copy age out by themselves in 14 nights.
- [ ] **Step 95** [W] ~10m: `~/tmp/next.md`: the verdict, what was switched off and when, what waits on him, the push count. Remove the worktree: `git -C /home/govan/tmp/unheaded worktree remove /home/govan/tmp/wt/S-LOG-WEEK-2`.
- [ ] **Step 96** [C][V] ~1m: **PHASE 7 EXIT GATE.** Every row LW-01 to LW-05 has a before, an after and a status. **COMMIT** `$PROG` by name. Stevie pushes.

---

## Emergency procedures

| # | What happens | What to do |
|---|---|---|
| E1 | The reading holds the Well busy and a service times out (the queries run up to 15 minutes each side) | Stop the reading (`systemctl --user stop unheaded-logstore-measure.service`, or Ctrl-C). The block written so far stays; the day's reading is the partial one. Lower `QUERY_BUDGET_S` to 300 for the rest of the week in the unit (a drop-in, one line in `disturbances`); the count of runs says what each p95 stands on. |
| E2 | The NVMe's volume fills | `df` on the volume; the writer's ceiling (4 GiB) bounds the copy, so look at ClickHouse first (`SELECT sum(bytes_on_disk) FROM system.parts WHERE active`). Nothing is deleted without Stevie. |
| E3 | Vector's `clickhouse` buffer climbs while the writer is stopped or slow | The record comes first: `sudo docker compose start logstore-writer`; if it still climbs, the off switch of Step 89 (ClickHouse stays branch) is the rollback, and the week is over with what it measured. |
| E4 | West must be rebooted or the stack stopped inside the week | Do what the host needs. Step 73 moves `T-END`. A week that loses more than two days is restarted from Phase 3 only on Stevie's word. |
| E5 | `TRUNCATE` (Step 52) or the index build (Step 82) hangs | It waits on a lock: `SELECT pid, state, wait_event_type, left(query, 40) FROM pg_stat_activity WHERE datname = 'unheaded_ops'` (no row text of the table is shown). Cancel the statement (Ctrl-C); never kill the writer's backend from inside the Well while Vector is sending. |

## Appendix A: what a reading holds after Phase 1

Week 1's metrics, unchanged in name and meaning: `ch_rows_yesterday`, `pg_rows_yesterday`, `ch_rows_7d`, `pg_rows_7d`, `ch_bytes`, `ch_rows`, `pg_bytes`, `pg_rows`, `pg_bytes_yesterday`, `{ch,pg}_{q1_last_hour,q2_text_24h,q3_units_7d}_{p50,p95}_ms`, `well_probe_{p50,p95}_ms`, `well_calls_total`, `well_exec_ms_total`, `backup_seconds`, `backup_bytes`, `vector_record_buffer_events`, `vector_record_buffer_bytes`, `vector_record_sends_over_1s_total`, `vector_well_dropped_total`.

New: `{ch,pg}_<query>_runs`; `well_probe_first_{p50,p95}_ms`; `vector_record_sends_over_1s_24h`; `vector_well_dropped_24h`; `ch_started_unix`, `pg_started_unix`, `vector_started_unix`, `writer_started_unix`, `host_boot_unix`; `io_some_avg300`.

---

*Forged 2026-11-17 by Claude (Fable 5.2) from ADR-147 decision 5, week 1's record and the tree at `integrate/adr147` `38c3e513`. Not executed. The rulings LW1 to LW5 are Stevie's.*
