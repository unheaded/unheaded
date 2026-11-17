<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# ADR-147 series: the live run order after the evidence week

**Written** 2026-11-17 (about 04:00Z), before the week ends at 18:34Z, from a read of the four plans' remaining phases against `integrate/adr147` `2dcb1d68`. Nothing in it has been run. It adds no scope: every step it names is a step of plan 1 to 5. Where it and a plan disagree about order or about a step listed under "Amendments", this page wins; everything else is the plan's.

**Why it exists.** Each plan was written to merge its own branch and deploy alone. The four were merged into one branch instead (`docs/battle-plans/progress/ADR147-INTEGRATION.md`), so one tree now carries all four: any image built from it holds every plan's code, and any migration batch sees every plan's files. Read alone, the steps listed under "Amendments" are stale. Run in the order below with those amendments, they are not.

## The plans and what is left

| # | Plan | Done | Left | Mode |
|---|---|---|---|---|
| 1 | S-SEE-AND-SURVIVE | Steps 1-246 (code) | Phases 12-19, Steps 247-387 | sudo throughout; Stevie for the gate, SS1-SS4, the inbox check |
| 2 | S-WASTE-OUT | Steps 1-304 (code) | Phases 17-23, Steps 305-414 | Stevie at 305, 306, 317, 338, 340, 341, 383, 394 |
| 3 | S-ZHEN-SWITCH | Steps 1-237 (code) | Phases 8-10, Steps 238-330 | 8A sudo; 8B (the Well) with Stevie |
| 4 | S-WEST-GROUND | Steps 1, 12-13, 42-101, 104-119 | Phases 0, 1, 4-12 | four attended sittings, A to D |
| 5 | S-LOG-WEEK-2 | forged 2026-11-17 | all 96 steps | Phase 1 unattended; the rest after plans 2 and 4 |

## Before anything runs: Stevie, about 30 minutes

| # | What | Why it gates |
|---|---|---|
| 1 | Unlock the GPG key (any signed commit), then `scripts/resign-unpushed.py`, then push develop | Every commit since 2026-11-16 20:45Z is unsigned, on develop and on `integrate/adr147`. The merge of block C should be signed. |
| 2 | S-E1-LIVE Step 284: read the week against the bar (`docs/battle-plans/progress/S-E1-LIVE-progress.md`, "Step 284, prepared for Stevie") and say the week is closed | Three plans' gates open only on that record or on your word: plan 1 Step 247, plan 2 Step 305, plan 3 Step 238. A date alone opens none of them. |
| 3 | The rulings, in one round (table below) | Plan 3 forbids a merge to develop on an unanswered D1 to D6. |
| 4 | Say go for block B (the gates) and block C (the merge) | The merge changes files that running containers read at their next restart. |

**The rulings.** "Lane took" is what the code already does on the plan's recommendation; a different answer means rework before the live phase.

| Plan | Id | Question, short | Recommended | Lane took | Needed by |
|---|---|---|---|---|---|
| 1 | SS1 | Which alerts page | A, the urgency table | A | Phase 15 |
| 1 | SS2 | Mail cap on run days | A, 24 a day / 6 an hour | A | Phase 15 |
| 1 | SS3 | Start limits that trip | A (now 180 starts in 2 h, REV-3) | A | Phase 14 |
| 1 | SS4 | New SLOs alert as tickets | A | not asked | Phase 17 |
| 1 | | Is zhen-agentd the exception to the start limit | | plan 3 kept "never give up" | Phase 14 |
| 2 | D1-D7 | Publisher, subscriber, discovery reader, Prometheus keep-list, Akira's cert check, `FLOW_EVENTS`, `FLOW_STATE` | A, B, A, A, A, A, A | as recommended | Phase 19 |
| 2 | D8 | Postgres's budget | A: 2 GiB, 1.0 CPU, `shared_buffers` 512MB | not taken | Phase 22 |
| 2 | Step 317 | Where the two public keys live (A, B or C); you make both ML-DSA-65 keys | | | live publishing on `alerts.critical`; the deploy itself does not wait |
| 3 | D1-D5 | Switch is a systemd target; own database; fix the sweep; vor outside the switch; allowlist for private addresses | as listed | as recommended | block C |
| 3 | D6 | `app_zhen` connection limit; off-site copy | 12; local only | **20** (REV-5); local only | Phase 8B |
| 3 | D7, D8 | Approve `ioc-audit.yaml` and seal; Zhen on or off at the end | yours; on | not taken | Phase 9 |
| 4 | WG1-WG5 | Move ClickHouse before the batch fix; backup sets to `sdb`; the two LAN paths; Alertmanager and Prometheus on loopback; west's filter before east's | A each | WG1-WG3 as recommended | sittings A and B |
| 5 | LW1-LW5 | Bar; indexes; start empty; timer; plan 8's baseline | A, C, A, A, A | | its Phases 3 to 7 |

## The order

Blocks run in this order. A block's steps are the plan's own, with the amendments of the next section.

| Block | What | Steps | Who | Time | Must come after |
|---|---|---|---|---|---|
| A | **The befores, on a quiet west.** No container is built or started here. | The closing log store reading (`scripts/logstore-measure.sh`); plan 2 Phase 17 (Steps 305-319); plan 3 Step 15; plan 1 Step 263 (the smoke check on develop as it is) | Claude with sudo; Stevie at 305, 306, 317 | 2.5 h, two 1 h gaps inside | 18:34Z and row 2 above |
| B | **The gates that start a container**, on a clean copy of the integration branch | `~/tmp/adr147-table/runs/after-week.sh` (32 gates; add `--meta` for the slow provocations). It covers plan 1 Steps 255, 257, 260; plan 2 Steps 321, 323 and the eBPF load gate of Steps 273-277; plan 3's throwaway-Well tests; plan 4 Steps 75-76 | Claude, unattended | 1.5 to 2 h | A (the gates load the host; the befores want it quiet) |
| C | **One merge**, `integrate/adr147` into `develop`, in place of each plan's own | See "The merge" | Claude on Stevie's go | 15 min | every gate of B green, or its failure fixed on the integration branch and that gate rerun |
| D | Plan 1 live, part 1: backups and a drill; huginn, host state, start limits; rules loaded, mail on, four drills | Phases 13, 14, 15 (Steps 271-334) | Claude with sudo; Stevie's inbox at Step 324 | 4.25 h | C |
| E | Plan 2: tag the old, build, deploy. Plan 1's bucket rollout rides on it. | Plan 2 Steps 327-358; plan 1 Steps 342-348 an hour later | Stevie at 338, 340, 341 | 3 h, then a 1 h wait | D (a container stopped by mistake now mails) |
| F | Plan 2: the 1 h afters; Vector's batch; the Well sitting. Plan 3: the switch installed. | Plan 2 Phase 20 (first reading), Phases 21 and 22; plan 3 Phase 8A (Steps 238, 242-264) | Stevie at 383, 394 | 4 h | E plus 1 h |
| G | Plan 3: Zhen's database; the switch proved. Plan 2: the 24 h afters. Plan 1: SLOs from what was measured; vor and nginx. | Plan 3 Phase 8B and Phase 9; plan 2 Steps 379-380, 392-393 and the rest of Phase 20; plan 1 Phases 17 and 18 | Stevie for 8B and Steps 308, 310 | 5 h | F plus 24 h; 8B never in the same sitting as plan 4's move |
| H | The three closes | Plan 1 Phase 19; plan 2 Phase 23; plan 3 Phase 10 | Claude; Stevie pushes | 4 h | G |
| I | Plan 4, sittings A to D | Phases 0, 1, then 4-5; 6-7; 8-9; 10-12 | Stevie present | 2.5 h, 3 h, 2 h, 2 h over 14 h | D (its Step 7 needs plan 1's backups and swap alerts live); plan 2's 24 h afters taken, or its befores taken again on the new disk |
| J | Plan 5, the second week | Phases 2 to 7 | Stevie at the start and the verdict | 24 h baseline, 7 days | I closed (the reboot of its Phase 11 past) |

**Beside the blocks, unattended, any time after C:** plan 5 Phase 1 (the instrument: 5 h of repository work in its own worktree, tests against stubs, nothing live). And before sitting B of plan 4: merge develop into `sprint/s-west-ground-compose` and rerun `scripts/check-compose-pids.sh` (the lane still edits the five removed services; a conflict is likely; about 40 minutes).

**Why this order and not the series order plan by plan.**

| Constraint | Consequence |
|---|---|
| Plan 2's befores (Step 311 on, cgroup and store readings) must be read before anything is rebuilt from the merged tree. | Block A comes first, before the gates and before plan 1's rollout. |
| Plan 1 Phase 16 rebuilds wotan, dashboard-backend, kanban-app, logstore-writer, auth, timeguru. From the merged tree timeguru needs migration 050 and its role's password (plan 2 Steps 340-341), and the other five are in plan 2's build list anyway. | Phase 16's build and recreate (Steps 335-341) are done by plan 2 Steps 327-355. Its readings (Steps 342-348) run an hour after plan 2 Step 355. |
| Plan 1 Step 325's drill stops one of the role services; plan 2 Step 349 removes them. | Plan 1 Phase 15 before plan 2 Phase 19. |
| Migrations 050 (plan 2) and 051 to 053 (plan 3) are in one manifest; 053 refuses until the rows are copied. | Plan 2 Step 341 applies with `--through 050` (added 2026-11-17, `2dcb1d68`); plan 3 Step 267 then finds exactly its three. |
| Plan 4 Step 7 proves plan 1's store backups and swap alerts are live. | Plan 4 after block D. |
| Plan 2 retakes a before if plan 4 moves the volumes before its after. | Plan 4's move (Phase 5) after plan 2's 24 h afters. |
| Plan 3 Phase 9 turns Zhen off and on; plan 2 Phase 22 restarts Postgres. | Not in the same hour. |

## The merge (block C)

It replaces plan 1 Steps 249-253 and 267, plan 2 Steps 320 and 325-326, plan 3 Steps 239-241, plan 4 Steps 102 and 120 (Merge A).

```bash
cd /home/govan/tmp/unheaded && git switch develop && test -z "$(git status --short --untracked-files=no)" && git merge --no-ff -S integrate/adr147 -m "Merge integrate/adr147: ADR-147 plans 1 to 4, the code (S-SEE-AND-SURVIVE, S-WASTE-OUT, S-ZHEN-SWITCH, S-WEST-GROUND)" && git log --oneline -1
for w in S-SEE-AND-SURVIVE S-WASTE-OUT S-ZHEN-SWITCH S-WEST-GROUND; do git -C /home/govan/tmp/wt/$w merge --ff-only develop | tail -1; done
```

- develop is already inside the integration branch, so the merge cannot conflict. A gpg timeout: the same with `--no-gpg-sign`, said in each plan's progress file.
- The four lane branches are inside the integration branch, so each `--ff-only` moves a lane worktree to develop. Later steps that say "in `WT`" then work on the merged tree.
- From this moment west's bind-mounted files are the new ones (rules, alert policy, `configs/wotan.yaml`, scrape files, the compose files). Nothing reads them until a HUP or a recreate; restart policy is `no`, so nothing recreates by itself. `docker-compose.yml` no longer names the five role services: their containers keep running as orphans until plan 2 Step 349.
- Then `scripts/ci-local.sh` on develop once (plan 1 Step 372's command), and each plan's progress file gets the merge commit.
- Not merged here: `sprint/s-west-ground-compose` (plan 4 Step 223).

## Amendments, by plan

A step not listed runs as written.

### Plan 1, S-SEE-AND-SURVIVE

| Step | As written | Instead |
|---|---|---|
| 247 | Opens on a line in S-E1-LIVE's progress file | No such line yet: it opens on Stevie's answer at Step 284, written there as a new dated entry |
| 249-253, 267 | Merge seven lanes, then the sprint branch | Block C. Six of the seven lanes never existed |
| 254-262, 264 | Gates on the sprint tree | Block B ran them on the merged tree; read `SUMMARY.txt` of that run |
| 263 | After the merge steps | In block A, before the merge |
| 308 | Expects `15min` and `10` | `2h` and `180` (`deploy/systemd/dropins/10-start-limit.conf`, REV-3) |
| 335-341 | Tag, build, recreate seven services and three host units | Done by plan 2 Steps 327-355. Take the planned stop of Step 335 before plan 2 Step 339 and end it (Step 341) after plan 2 Step 356. `unheaded-wiki-server` is rebuilt by plan 3 Step 256 |
| 346, 371 | `:pre-ssv` tags | The rollback tags are plan 2's `:pre-s-waste-out` |
| 350, 352 | `pq` on `unheaded_http_*` | Prometheus drops those series after plan 2's keep-list (Step 355); read VictoriaMetrics (`q`) |
| 355-358 | Four SLOs into policy and rules | Also `python3 scripts/gen-prom-keep-list.py` and the SLO source rows in `compliance/catalog/controls.yaml`, in the same commit |
| 166, Emergency 9 | "30d" | The code keeps a mark at most 2 h past its end; read `scripts/monitoring/planned-stop.sh` |

### Plan 2, S-WASTE-OUT

| Step | As written | Instead |
|---|---|---|
| 320, 325-326 | Merge `sprint/s-waste-out` | Block C |
| 321, 323-324 | Container gates and ci-local on the sprint tree | Block B. The sprint branch alone fails `check-image-tags` and the image digest test; the fixes are integration commits |
| 273-277 (never run) | eBPF load gate and instruction ratchet | Block B (`ebpf-loads`, `ebpf-insns`). Step 343 installs nothing that gate has not passed |
| before 339 | No mark | `sudo scripts/monitoring/maintenance.sh --planned 60m "S-WASTE-OUT deploy"`; end it after Step 356 (plan 1's rules are live by then) |
| 340 | `env -u` of eight variables | Amended in the plan: every `*_PASSWORD` but `APP_TIMEGURU_PASSWORD` is unset in a subshell (the script also knows `POSTGRES`, `WELL_BACKUP`, `RETENTION_RUNNER` and `LOG_WRITER`) |
| 341 | `--from 050`, expects `pending (1)` | Amended in the plan: `--from 050 --through 050`; expects `pending (1) through 050` and three files left for a later batch |
| 371 | Add job `victoria`; rule in `rules/host.yml` | The job exists (plan 1); the rule goes in `rules/watchers.yml` beside `VictoriaMetricsDown` |
| 372 | "Step 198's promtool" in a lane worktree | `scripts/check-prometheus-rules.sh` in `WT`; no lane worktree was made |
| 359 (W-25) | Bar "tmpfs: none" | timeguru keeps its `/data` tmpfs on purpose (progress file, Phase 16); record the row as it is |
| 367, 375, 384, 387 | `git merge --ff-only` between `WT` and develop | Work after block C; a "not possible to fast-forward" is handled as Step 375 says |

### Plan 3, S-ZHEN-SWITCH

| Step | As written | Instead |
|---|---|---|
| 239-241 | Merge the sprint branch | Block C |
| 247 | `HEAD^1` is the unit before the merge | Compare against `9887b666` (develop before block C) |
| 252, 301 | Expects `30s auto-reload-config` | The tree passes `--config.auto-reload`; read what the command prints and record it |
| 257 | `curl http://127.0.0.1:20002/auth/nav.js` | The wiki unit serves TLS: `https` with the CA file the unit names |
| 266 | `052_app_drop_zhen.sql` | Amended in the plan: `053_app_drop_zhen.sql` |
| 267 | Stops unless only its three are pending | True once plan 2 Step 341 has applied 050 |
| 273-275, 278, 281, Appendix A procedure 4 | Files named 050, 051, 052 | They are 051 (database and role), 052 (schema), 053 (drop). Procedure 4's ledger delete names `051_zhen_database.sql` |
| 274, 287, 322 | Connection limit `12` | Amended in the plan at Step 274: `20` (migration 051, REV-5). The same at 287 and 322 |
| 283 | `/etc/unheaded/zhen-agentd.env` | The unit in the tree reads `/etc/zhen-agentd.env`; Step 247's diff shows which the host has. Edit the one the installed unit names |
| after 283 | Nothing | Two things the progress file promises "at 8B": `scripts/write-zhen-app-env.sh` (REV-2), and `WELL_ZHEN_DSN` in the overnight user unit's env file (REV-1) |
| 319, Appendix A procedure 7 | Baseline `1393e5ee` | `9887b666` |
| 322 | | Also the budget table in `docs/architecture/THE_WELL_OPERATIONS.md`: say that `app_zhen`'s role limit is 20 and its pools are 2 x 5 |
| 323 | A wiki link check script | `tests/scripts/test_doc_links.py` under the CI venv |

### Plan 4, S-WEST-GROUND

| Step | As written | Instead |
|---|---|---|
| 102, 120 | Merge A and the Phase 3 merge | Block C |
| 7 | Plan 1's backup and swap-alert rows read done | True only after block D; until then Phases 4 and 5 are `[BLOCKED]`, as the step says |
| 147, 171 | Checks the Well's backup timer | Also `unheaded-clickhouse-backup`, `unheaded-victoria-backup` and `unheaded-store-drill` timers (plan 1): none due inside the window |
| 157, 226, 295 | `maintenance.sh 60m` | `maintenance.sh --planned 60m "<reason>"`: a silence alone lets `ContainerStopped` and the SLO budgets count the stop |
| 215, 217 | Merge develop into the compose lane | Expect a conflict on the five removed services; 18 services remain; the limits then sum to 12,800 (11.1% of the thread limit) |
| 218-219 | A grep for slice names | It also matches huginn's test fixtures; `cmd/huginn/cgroups.go` walks any slice to depth 4, so a hit in a fixture is not a defect |
| 246 | Expects `rc=1` | The gate exits 4 on drift (REV-2) |
| 296 | Reboot | `~/tmp/west-hold.conf` holds a block lock while Claude or tmux runs: `sudo reboot` is refused. Lifting it is Stevie's (`hold=false`, or `sudo systemctl reboot -i`); Claude never does either |

## What could go wrong on the day, and what is already in place

| Risk | In place |
|---|---|
| A gate of block B fails on the merged tree. Several were written or changed in the week and have never met a container: the hadolint and trivy baselines (edited by hand), the Zhen test scripts moved to `unheaded_zhen`, the new cases of `test-batch-apply.sh` | Nothing is merged until it is fixed on the integration branch and rerun. Each gate has its own log under `~/tmp/adr147-table/runs/after-week-<time>/` |
| The Postgres copy of the logs reaches the writer's 4 GiB ceiling about 2026-11-22 20Z | It then answers 503 and Vector drops at that sink only; the record in ClickHouse is not touched. Plan 5 empties the table at its Step 52. To stop it sooner: plan 5 Steps 47-48 (a mark, then `compose stop logstore-writer`), no Vector restart |
| After plan 2 Step 349 a role container reads as stopped | They are removed in the same step; the mark of the amended Step 339 covers the minutes between |
| A sitting ends between blocks | Every block ends at a plan's exit gate. D, E and F each leave west in a state the next morning's alerts describe truthfully |

*Not executed. Step numbers were read from the plans at `integrate/adr147` `2dcb1d68`; four read-only agents checked the remaining phases against the tree, and the findings cited as amendments to Steps 308, 340, 341, 266 and 274 were verified by hand.*
