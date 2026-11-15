<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-SEE-AND-SURVIVE BATTLE PLAN: 20 Phases, 387 Steps

**Date**: 2026-11-15
**Sprint**: S-SEE-AND-SURVIVE, plan 1 of the ADR-147 series (`docs/battle-plans/SPRINT-SERIES-2026-11-15-ADR-147.md`). Alarms that reach a person, instruments that can read a load, backups of what had none.
**Prerequisite**: ADR-147 "Stevie's answers to the decisions" (2026-11-15): 1 (restart stays `no`; a stopped container alerts), 2 (mail on; a stopped container and a memory kill mail; the cap is raised for run days; `urgency: none` rules read again), 4 (a planned stop spends no budget; `ci-develop` leaves the alert set), 6 (back up ClickHouse and VictoriaMetrics, each with a restore drill, before any load), 12 (alerts on swap use and memory pressure), 20 (instruments before load), 23 (vor gets an access log; nginx logs request time; orb's load stays an open gate). Seven choices this plan found are asked at Steps 5 and 6 (table "Decisions"); the phases that need one wait, the rest run.
**Target**: On west: every container of the stack that stops, and every memory kill in the stack, mails Stevie within 5 minutes, proved by a drill. The mailer is in `send`, and a silent alert path (Prometheus or Alertmanager down) mails him too. Rules read pressure, swap, temperature, restarts, failed units and the monitoring path itself, each with a promtool test. Latency buckets start at 0.1 ms. Every service and container has CPU, memory, I/O, pressure and memory-kill series per cgroup. A planned stop spends no error budget and `ci-develop` pages nobody. ClickHouse and VictoriaMetrics have nightly sets on the NVMe and a weekly restore drill that has passed once. `scripts/qa-smoke.sh` can pass. vor logs every request and nginx logs request time. Every row SSV-01 to SSV-28 has a before, an after and a status.
**Estimated duration**: 26 to 34 h of work over 6 to 8 sessions. Phases 0 to 11 are code (no host change) and run before the evidence week ends. Phases 12 to 18 change west and wait for the E-GATE. A 24 h wait sits between Phases 16 and 17. One `[STEVIE]` receipt check in Phase 15; east is not changed by this plan.
**Agent strategy**: Coordinator (Opus) for Phases 0, 12 to 19. Code lanes in their own worktrees: `alerting` (Phases 1, 2, 7, 8 in order: one set of files), `buckets` (3), `huginn` (4), `state` (5, 6), `backup` (9), `smoke` (10), `vor` (11). At most three build at once (series rule 3). Lanes merge into `sprint/s-see-and-survive` one at a time; nothing merges to `develop` before the E-GATE.
**Commit cadence**: every 5 steps, at every exit gate, before every skip. Signed (`-S`); if gpg-agent has timed out and Stevie is away, `--no-gpg-sign`, logged in the progress file for `scripts/resign-unpushed.py`. Named files only, never `git add -A`. Message `[PLAN S-SEE-AND-SURVIVE] Steps X-Y: ...`, then the two trailers the executing session's attribution reminder gives. Stevie pushes, here and in the vor repository.
**Stuck protocol**: skip after 3x the step estimate or two failed debug attempts. Mark the step `[STUCK]`, its dependents `[BLOCKED]`, commit, go on to the next independent phase. Inside a live phase "stuck" means roll back first (each phase names how), then mark. Haiku and Sonnet agents never improvise past a stop line (series 2026-11-04 rule 12).

## Grounding (checked 2026-11-15 ~20:00-21:00Z against develop `1393e5ee`, read only on west; east not touched)

Seat rows are cited as seat and row id (reports in `~/tmp/adr147-table/` on west). "Read" means the forger measured it again.

| Row | Seat or ADR said | Found | Plan does |
|---|---|---|---|
| Micromanager 1.2, D1; BlackMage F2 | 18 of 23 containers do not restart; nothing alerts on a stopped one | True (read: `docker inspect`, 18 `no`, 5 `unless-stopped`; `docker-compose.override.yml` has 18 `restart: "no"` lines). **And:** 12 exited containers sit on the host outside the 23: three compose one-shots (`unheaded-volume-init`, `monitoring-volume-init-1`, `unheaded-clickhouse-schema`, exit 0 by design), `unheaded-haproxy-edge`, `unheaded-haproxy-internal`, `unheaded-sword` (2 weeks), five `netbox-docker-*` and `jenkins` (7 months). A rule on "not running" would fire on nine of them for ever | SSV-01: Phase 5 reports state for projects `unheaded-dev` and `monitoring`, one-shots skipped; Phase 7's rule fires only for a container that ran in the last 7 days. Restart policy untouched |
| BlackMage F2 | A memory kill is counted by Vector and read by nothing | `node_vmstat_oom_kill` = 6 on west (host total since boot; Micromanager: the kills were sandbox processes of user 1000). No per-cgroup series. A container killed as PID 1 exits and its cgroup goes with it, so a per-container counter is never read; `memory.events` of the parent slice is hierarchical and stays | SSV-02: Phase 4 reads `memory.events` per cgroup; Phase 7 alerts on the stack's slices; Phase 5 reports Docker's `OOMKilled` |
| Micromanager D7 | The mailer is in dry-run since 11-10 | True (read: `/v1/status` mode `dry-run`, `max_per_day` 6, 9 open tickets; start-up log `relay_configured: true`, so the relay settings are in place). `policy.yaml` says "back to send at S-E1-LIVE Steps 81-83 and 102-109, with Stevie": decision 2 now answers that | SSV-03: `mode: send` (Phase 1), live with a test mail (Phase 15) |
| BlackMage 1.7 | 47 of 87 rules are `urgency: none` | Consistent, read differently: all 55 hand-written alerts carry `urgency: none` by rule (`cmd/alert-policy/main.go:379-416` refuses anything else); 8 are promoted to ticket or page in `policy.yaml` `promote:`; 32 are generated SLO alerts. 55 - 8 = 47 | SSV-04: a rule mails by a `promote:` entry with a reason (SS1) |
| Scientist F3 | No alert reads a resource | True (read: no alert name or expression has pressure, swap, oom, temp or restart). The series exist in Prometheus: `node_pressure_*`, `node_vmstat_pswpin`, `node_memory_Swap*`, `node_hwmon_temp_celsius` with `_crit_celsius` for 7 sensors, `node_vmstat_oom_kill`. 5-day peaks on west: memory full 0.5%, I/O full 13% (15 m), CPU wait 2.1%, swap-in 327 pages/s | SSV-06, SSV-07: Phase 7 |
| BlackMage F16 | Start limits cannot trip | True on west (read: wiki-server, huginn, kev-poller, vor run with systemd's default 10 s / 5 starts and `RestartSec` 5 to 30 s; wiki-server `NRestarts=15`). The trace collector already trips (600 s / 60, `RestartSec` 5 s). Installed unit files differ from the repository for huginn, trace-collector, wiki-server and zhen-agentd | SSV-09: SS3; a gate, and drop-ins on west (never a copy over a drifted file) |
| Micromanager D8; BlackMage 1.7 | The watchers are unwatched | True (read: no rule for VictoriaMetrics, remote write, Alertmanager, the mailer or huginn; Prometheus has no `victoria` job; `AlertPipelineHeartbeat` always fires and goes to receiver `none`). ADR-116 R4 (parked) describes a delivery receipt; decision 3 (east watches west) is plan 6's | SSV-10, SSV-11: Phases 2 and 7 |
| ADR-147, S-WATCH-TRUE For Stevie | `TraceCollectorHighLatency` cannot fire | True and more (read: `trace_collector_batch_latency_seconds_count 0`; the live histogram is `trace_publisher_flush_duration_seconds`, whose top bucket is 0.1 s, so the rule's 1 s bar could not be read from it either) | SSV-12: Phase 7 reads the share of flushes over 0.1 s |
| BlackMage F13 | `VectorSinkDroppingEvents` watches a buffer that blocks | True, and the counter it reads, `vector_buffer_discarded_events_total`, has no series for any component: Vector makes it on the first discard and restarted 2026-11-15 02:11 | SSV-13: one rule for any component that discards |
| BlackMage F14 | Both AIDE rules read a series that does not exist | True, because `unheaded-aide-check.timer` is not installed on west. The rules are right; the producer is S-EVIDENCE-RUNS Phase 10 (attended) | SSV-14: no change; closed "handed", with the reason |
| Scientist F2, F4; Micromanager D6 | SLOs cannot fail; six budgets negative at idle | True (read: dashboard -31.6, well -28.5, log-shipping -28.3, well-backups -17.5, wotan -15.3, ci-develop -99; only `ci-develop` fires, four tiers). `scripts/monitoring/maintenance.sh` silences mail but not the budget. `kingdom-shutdown.sh:118` and `kingdom-startup.sh:150` already call it | SSV-17, SSV-18: Phase 8 |
| Micromanager D3 | The Wotan SLO says both hosts and measures one | True (read: one target; east's Wotan is off until plan 6) | SSV-19: the text says west |
| Scientist F1 | The latency histogram cannot see our latencies | True (read: p99 0.00495 for 13 services; 86 label sets, 1,056 bucket series; `pkg/httputil/middleware.go:56` and Wotan's own `services/wotan/internal/metrics/metrics.go:74` use `DefBuckets`). dashboard-backend's scraper derives quantiles from whatever buckets exist | SSV-15: Phase 3, 16 buckets from 0.1 ms to 10 s (about +480 series) |
| Scientist F10, A1 | CPU and memory only for Go processes | True (read: 24 targets export `process_cpu_seconds_total`; huginn reads `/proc`, not cgroups; huginn's unit has `ProtectControlGroups=yes`, which leaves cgroupfs readable) | SSV-16: Phase 4 |
| Erda F15 | ClickHouse (audit logs, 455 days) and VictoriaMetrics have no backup | True (no backup in wiki, runbooks or units). VictoriaMetrics holds 219 MB (`vm_data_size_bytes`); its snapshot API needs the key in volume `unheaded-dev_victoria-auth` (`docker-compose.yml:340-348`). ClickHouse is 362 MiB (Erda) to 0.8 GiB (Architect): measured again at Phase 13. ClickHouse has no `backups` config; it runs read-only as uid 16707. The Well's pattern: sets on the NVMe (`/srv/backups`), an other-disk guard, `LATEST`, a weekly drill, `.prom` on success | SSV-21, SSV-22: Phase 9 (code), Phase 13 (live) |
| Micromanager D5 | `qa-smoke.sh` probes plain http against TLS | True (read: `http://...:19000/health` 400; with `--cacert configs/certs/dev/ca.pem`, `https` answers 200 on 19000, 18000, 20000, 20001, 19100, 19020, 20002). Also: its flows check reads a dashboard API that needs a login since the BFF; its Well check is a `psql` session (E-GATE) | SSV-23: Phase 10 |
| Scientist F9; decision 23 | vor logs nothing; nginx has no request time | True: vor serves `http.ListenAndServe(addr, mux)` with no logging (`~/tmp/projects/vor/cmd/vor/main.go:1071`; west runs `vff4c7e4`). West's nginx has one site, `unheaded-apt.conf` (same as `deploy/nginx/unheaded-apt.conf`), default format. The blog and site run on orion, outside Unheaded | SSV-24, SSV-25: Phase 11 (code), Phase 18 (live); orion is SS7 |
| Decision 23 | What counts as load on orb | Open, a gate | SSV-26: SS6, carried to plan 8 |
| Micromanager D2 | `/health` answers 200 when degraded | `pkg/transport/health.go:112-116`: DEGRADED means one of two transports is down, 200 by design. The SLOs read `up`, not `/health` | SSV-28: not changed here. SLIs move to latency and errors (Phase 17); `/ready` semantics go to "For Stevie". S-WASTE-OUT Phase 6 names each stream's transport |
| Scientist A5, A7 | A second recorder off `sdb`; a run marker series | Both serve a load run | SSV-27: plan 8's run controller; closed here with that reason |
| Decisions 9, 10 (S-WASTE-OUT D4) | Prometheus keeps hours | Nine rules read 28 or 30 days | This plan keeps the 28 d SLO window; S-WASTE-OUT D4 decides retention |

**Marshal (scope).** IN: decisions 1, 2, 4, 6, 12 (the alerts), 20 (the instruments: buckets, per-cgroup series, restart counters, resource alerts, latency and error SLIs), 23 (vor, nginx), and stage 1's rows: the rules that cannot fire, the watchers, a smoke check that can pass, start limits. OUT, with a home: east watching west, east's units and huginn on east (plan 6); swap files, huge pages, the input filter, auth on 9090/9093 (plan 4); removing role services, `alerts.critical`, Akira, Wotan's polls, Vector's batch, Postgres's limit, Prometheus retention (plan 2); Zhen's rules, schedules and targets (plan 3); the run controller, second recorder, run marker, stop rules, pass marks (plan 8); the AIDE producer (S-EVIDENCE-RUNS Phase 10). Rules: restart policy stays `no`; no new module, crate, image or tool (ClickHouse's `BACKUP`, VictoriaMetrics' snapshot API, `jq`, `curl` and the pinned images are already here); no page or panel is built (series rule 22).

**Micromanager (QA / DoD).** Code is red then green under `go test -race`. Every code phase closes on `gofmt -l`, `go vet`, `go test -race` for the packages it touched and `./scripts/check-gosec-ratchet.sh`. Rules pass `scripts/check-prometheus-rules.sh` (every new rule file has a test file) and `go run ./cmd/alert-policy --check`; runbooks pass `tests/runbooks/test_alert_runbooks.py` and `scripts/runbook-manifest.sh --write` (the seal is Stevie's). Shell passes `shellcheck` and its own `test-*.sh`. A new gate is registered in `scripts/check-gates-can-fail.sh` and shown to fail. **Done is a row in the progress file with a before and an after measured the same way**, and for every alert that is a stop mark (ContainerStopped, MemoryKill, the pipeline mail) a drill that made it fire and mail, with the time. No new label carries free text; every label set is bounded.

**Busboy (handoffs).**
- **The evidence week** (S-E1-LIVE Step 284, about 2026-11-17 18:34Z). Until the E-GATE (Step 247): no merge to `develop`, no image build, no container or unit restart, no SQL session on the Well or ClickHouse, nothing written under `/var` (scratch lives on the NVMe), no throwaway container test. Promtool runs (`check-prometheus-rules.sh`) are allowed: they touch no store.
- **Plan 2 (S-WASTE-OUT)** edits `cmd/alert-mailer` (Phase 8, the `alerts.critical` publisher), `scripts/qa-smoke.sh` (drops five services), and adds a file beside `pkg/httputil/middleware.go`. Whoever merges second merges `develop` in first; a conflict is `[STUCK]`. Its Phase 21 restarts Vector and ClickHouse: not in the same hour as Phase 13 here. Plan 5's fair week starts after both, with the nightly store backups already running, so they are part of its steady state.
- **Plan 3 (S-ZHEN-SWITCH)** moves Zhen's rules out of `rules/zhen.yml` and edits `policy.yaml` `promote:`. Keep both sides of `policy.yaml`.
- **Plan 4 (S-WEST-GROUND)** Step 7 greps this plan's progress file for `clickhouse.*backup`, `victoria.*backup`, `restore`, `swap`, `pressure`, `stopped container`, `oom`, and Step 10 reads `wiki/Well-Backups.md` for the store backups' commands and directories: the closing rows use those words, and Phase 19 writes that wiki section. When plan 4 moves the database volumes to the NVMe, the sets in `/srv/backups` share a disk with the data and the other-disk guard refuses: the set roots are one `Environment=` line each, for plan 4 to move. Plan 4 also stops the stack: its stop is the first live test of the planned-stop marker (read `slo:good` across it).
- **S-E1-LIVE** Steps 81-83 and 102-109 (the mail steps) are done by Phase 15 here; Phase 19 writes a dated line in that progress file.
- **S-EVIDENCE-RUNS** owns the AIDE producer; when it lands, `AIDECheckStale` can fire.
- Runbooks: `--write` only; the seal and `status: approved` are Stevie's.

## Decisions

Asked at Steps 5 and 6 as AskUserQuestion, four at a time, recommended option first. "Away" is what runs if Stevie is not there.

| # | Question | Options | Recommended | Away |
|---|---|---|---|---|
| SS1 | Which alerts mail, and how (table "Proposed urgency" below)? | (A) as the table. (B) only ContainerStopped and MemoryKill page; everything else new stays `none` (visible, not mailed) | **A**: decision 2 asks that a rule that should mail gets an urgency; the table pages only on what stays broken or stalls the host | B |
| SS2 | Decision 2: "the cap of six a day is raised for run days". How? | (A) `mailer.run_days`: dates listed in `policy.yaml` get 24 a day and 6 an hour; plan 8 adds its dates. (B) the same with 50 and 12. (C) no mechanism: edit `max_per_day` by hand on the day | **A**: reviewed in git, dated, undone by deleting a line | A |
| SS3 | Host units restart for ever today. | (A) limits that trip: 10 starts in 15 min, then `failed` and `UnitFailed` pages. (B) keep looping; `UnitRestartLoop` (a ticket) shows it | **A**: a loop that never ends is a failure nobody is told about; 10 in 15 min covers a boot race (wiki-server needed 15 tries before its drop-in) | B |
| SS4 | Asked at Phase 17, after 24 h of the new buckets: latency and error SLIs for Wotan and dashboard-backend | (A) "good" = share of requests under T, T the smallest bucket bound at least 3 times the measured idle p99; objective 99%; errors: share not 5xx, objective 99.9%; tickets only. (B) record the SLIs, no alert, until plan 8 sets pass marks | **A**: the pass mark P1 ("p99 at 85% at most three times idle") becomes an SLO that can fail | B |
| SS5 | The ClickHouse and VictoriaMetrics sets: also offsite on east? | (A) with plan 6 (east as second home: its chroot account gets two more directories). (B) attended, in this plan after Phase 13. (C) local only, written in the wiki | **A** | A |
| SS6 | Decision 23's open gate: what counts as load on orb | (A) a ramp of viewers, found by measurement in plan 8. (B) a fixed number of viewers at 1 s refresh. (C) orb is measured at idle only | **A** | stays open for plan 8 |
| SS7 | orion is outside Unheaded. Should its nginx log request time before the recorded day is taken? | (A) Stevie adds the same `log_format` on orion (his hands; the text is in Phase 11). (B) no; request time is measured on the cluster only | **A**: the replayed day then carries the public server's own times to compare with | B |

**Proposed urgency (SS1).** All rules are written `urgency: none` (the generator requires it); mail comes from a `promote:` entry with a reason. Bars come from 5 days of west's own series (Grounding); a bar is a number for an alert, not a share (decision 13 does not apply).

| Alert | Fires when | for | West today | Urgency (A) |
|---|---|---|---|---|
| ContainerStopped (new) | a watched container is not running, ran in the last 7 d, no planned-stop marker | 2m | 0 | page |
| MemoryKill (new) | `oom_kill` rises in `system.slice` or `unheaded.slice` | 0m, kept 30m | 0 in the stack in 14 d | page |
| HostOOMKill (new) | `node_vmstat_oom_kill` rises (login sessions too) | 0m | 6 since boot | none |
| HostMemoryPressure (new) | memory `full` over 10% (5 m rate) | 10m | max 0.5% | page |
| HostSwapping (new) | swap-in over 1,000 pages/s (10 m rate) | 15m | max 327 | ticket |
| HostSwapNearFull (new) | swap used over 80% of swap | 30m | 7% (of 34 GiB) | ticket |
| HostIOPressure (new) | I/O `full` over 30% (15 m rate) | 30m | p95 4.3%, max 13% | ticket |
| HostCPUPressure (new) | CPU `some` over 50% (15 m rate) | 30m | max 2.1% | none |
| HostTemperatureNearCritical (new) | a sensor within 5 °C of its critical mark | 10m | east CPU 60.75 of 70 | ticket |
| UnitFailed (new) | a watched unit is `failed` | 5m | 0 | page (SS3 A), ticket (B) |
| UnitRestartLoop (new) | 3 restarts in 30 min | 0m | wiki-server 15 at boot | ticket |
| ContainerUnhealthy, ContainerCPUThrottled, ContainerMemoryNearLimit (new) | Docker health; over 25% of periods throttled; over 95% of its limit | 5m, 30m, 30m | akira 44% throttled | none |
| VictoriaMetricsDown, HuginnDown, HostStateStale, PrometheusRemoteWriteFailing (new) | the monitoring path breaks | 5m to 15m | 0 | ticket |
| AlertMailerDown, AlertmanagerNotReached (new) | the mailer or Alertmanager is unreachable | 5m, 10m | 0 | none (they cannot mail about themselves; the mailer's pipeline mail and plan 6 cover them) |
| HostDiskFull (exists) | under 3% free | 5m | never fired | none → page |
| HostDiskAlmostFull, NodeExporterDown (exist) | | | fired 578 min; never | none → ticket |

---

## LEGEND

- `[B]`: bash command, run directly
- `[V]`: verification; must pass before going on
- `[D]`: debug step, only if the step before it failed
- `[W]`: write or change a file (Write or Edit tool, never a shell heredoc)
- `[R]`: read or inspect
- `[S]`: needs sudo (always Opus)
- `[P]`: can run beside other `[P]` phases
- `[C]`: commit checkpoint (signed, named files)
- `[STEVIE]`: Stevie runs it or says yes first. `[STEVIE][DECIDE]`: he chooses
- `[STUCK]` / `[BLOCKED]`: set by the Skip Protocol

`~Nm` is the expected wall-clock time. Paths, written out in every command:
`REPO=/home/govan/tmp/unheaded`; `WT=/home/govan/tmp/wt/S-SEE-AND-SURVIVE` (branch `sprint/s-see-and-survive`); lane worktrees `/home/govan/tmp/wt/S-SEE-AND-SURVIVE-<lane>` (branch `sprint/s-see-and-survive-<lane>`); `S=/home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch` (NVMe; never `/var/tmp` or `/tmp` before the E-GATE); `PROG=docs/battle-plans/progress/S-SEE-AND-SURVIVE-progress.md` in whichever tree is being committed; `LIB=/home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh` (Step 3; each block that needs it sources it).

**GO-GATE `<lane> <dirs>`**, with `<dirs>` as `./pkg/x/... ./cmd/y/...`:
```bash
cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-<lane> && test -z "$(gofmt -l $(echo <dirs> | sed 's#/\.\.\.##g'))" && go vet <dirs> && GOMAXPROCS=4 go test -race -count=1 <dirs> 2>&1 | tail -8 && ./scripts/check-gosec-ratchet.sh 2>&1 | tail -2
```
Expected: no `FAIL`, every test line `ok` or `no test files`, the ratchet `[PASS]`.

**RULES-GATE `<lane>`**:
```bash
cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-<lane> && scripts/check-prometheus-rules.sh 2>&1 | tail -4 && go run ./cmd/alert-policy --check 2>&1 | tail -4 && /var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/runbooks/test_alert_runbooks.py 2>&1 | tail -2
```
Expected: promtool `SUCCESS` for every file, `[PASS]` lines from alert-policy, pytest `passed` with no `failed`.

---

## PHASE 0: PRECONDITIONS, DECISIONS, THE "BEFORE" READINGS (Steps 1-22)

**Goal**: Worktree and scratch ready, the seven choices answered or defaulted, and a before for every row that can be read without touching a store.
**Prerequisite**: west up with the stack (VictoriaMetrics `127.0.0.1:8428`, Prometheus `127.0.0.1:9090`).
**Time**: 50 min.
**Model**: Opus. **Agent**: Coordinator.

- [ ] **Step 1** [R] ~15m: Read ADR-147 (all, the answers last), the series file, this plan's Grounding and Decisions, `observability/alerting/policy.yaml`, `cmd/alert-policy/main.go` 40-130 and 224-420, `cmd/alert-mailer/main.go` 50-260, `monitoring/prometheus/prometheus.yml`, `wiki/Well-Backups.md`, and the Phase lists of S-WASTE-OUT, S-WEST-GROUND and S-ZHEN-SWITCH.
- [ ] **Step 2** [B] ~1m: Worktree and scratch (the scratch directory is on the NVMe, in the user's home).
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && git worktree add /home/govan/tmp/wt/S-SEE-AND-SURVIVE -b sprint/s-see-and-survive develop && mkdir -p /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch && ln -sfn /home/govan/tmp/projects /home/govan/tmp/wt/projects && git -C /home/govan/tmp/wt/S-SEE-AND-SURVIVE log --oneline -1
  ```
- [ ] **Step 3** [W] ~3m: Create `/home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh` with exactly:
  ```bash
  # S-SEE-AND-SURVIVE helpers. Read only.
  q()  { curl -s -m10 -G http://127.0.0.1:8428/api/v1/query --data-urlencode "query=$1" | python3 -c 'import json,sys; [print(r["metric"], r["value"][1]) for r in json.load(sys.stdin)["data"]["result"]]'; }
  pq() { curl -s -m10 -G http://127.0.0.1:9090/api/v1/query --data-urlencode "query=$1" | python3 -c 'import json,sys; [print(r["metric"], r["value"][1]) for r in json.load(sys.stdin)["data"]["result"]]'; }
  firing() { pq 'ALERTS{alertstate="firing"}' | python3 -c 'import sys,ast; [print(ast.literal_eval(l.rsplit(" ",1)[0]).get("alertname"), ast.literal_eval(l.rsplit(" ",1)[0]).get("name",""), ast.literal_eval(l.rsplit(" ",1)[0]).get("slo","")) for l in sys.stdin]'; }
  outbox() { curl -s -m5 127.0.0.1:19130/v1/outbox | python3 -c 'import json,sys; [print(e.get("time"), e.get("outcome"), e.get("subject")) for e in json.load(sys.stdin)[-8:]]'; }
  ```
- [ ] **Step 4** [W] ~5m: Create `$PROG` in the worktree with sections `## Decisions`, `## Fixes` (a table: id, seat and row, evidence, fix, owner, before, after, status; one row per SSV-01 to SSV-28 from Appendix C, before and after empty, status `open`), `## Log`, `## For Stevie`, `## Blocked`.
- [ ] **Step 5** [STEVIE][DECIDE] ~5m: AskUserQuestion, four questions: SS1 (show the "Proposed urgency" table), SS2, SS3, SS5, options and recommendation as in "Decisions". Stevie away → the "Away" column.
- [ ] **Step 6** [STEVIE][DECIDE] ~3m: AskUserQuestion: SS6, SS7, and a note that SS4 is asked at Step 354.
- [ ] **Step 7** [W] ~2m: The answers verbatim under "Decisions" in `$PROG`, each dated; "Away" defaults marked "default, Stevie away".
- [ ] **Step 8** [B] ~1m: Tools (never run a missing one).
  ```bash
  for t in go docker jq curl python3 shellcheck promtool; do command -v $t >/dev/null && echo "$t ok" || echo "$t MISSING"; done; test -x /var/tmp/unheaded-sprint/civenv/bin/pytest && echo "pytest ok"; docker info >/dev/null 2>&1 && echo "docker without sudo ok"
  ```
- [ ] **Step 9** [V] ~1m: All `ok` except `promtool` (it runs from the Prometheus image inside `check-prometheus-rules.sh`). `shellcheck` missing → write it under "For Stevie" and use `bash -n` in its place; nothing is installed.
- [ ] **Step 10** [B] ~2m: SSV-01 before: restart policies and the exited containers.
  ```bash
  for c in $(docker ps -aq); do docker inspect -f '{{.Name}} {{.State.Status}} {{.State.ExitCode}} {{.HostConfig.RestartPolicy.Name}} {{index .Config.Labels "com.docker.compose.project"}}' $c; done | sort -k5,5 -k1,1 | tr -d / | column -t
  ```
  Reference 2026-11-15: 23 running (18 `no`, 5 `unless-stopped`), 12 exited (Grounding row 1).
- [ ] **Step 11** [B] ~2m: SSV-03 to SSV-05 before: the mailer.
  ```bash
  source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; curl -s -m5 127.0.0.1:19130/v1/status | python3 -c 'import json,sys; d=json.load(sys.stdin); d.pop("outbox",None); print(d)'; pq 'sum by (urgency, outcome) (unheaded_alert_mailer_messages_total)'
  ```
- [ ] **Step 12** [B] ~2m: SSV-04, SSV-06, SSV-07, SSV-10 before: the rule inventory.
  ```bash
  cd /home/govan/tmp/unheaded && grep -h 'alert:' monitoring/prometheus/rules/*.yml | grep -vc slo; grep -c '^  - alert:' observability/alerting/policy.yaml; grep -l -i -E 'pressure|swap|oom|temp_celsius|restart|job="victoria"|job="huginn"|remote_storage' monitoring/prometheus/rules/*.yml | grep -v slo.yml | wc -l
  ```
  Reference: 55 hand-written alerts plus 32 generated (the first number counts lines with `alert:` outside `slo.yml`), 8 promotions, 0 files.
- [ ] **Step 13** [B] ~2m: SSV-15 before: the buckets.
  ```bash
  source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; q 'histogram_quantile(0.5, sum by (service,le)(rate(unheaded_http_request_duration_seconds_bucket[1h])))'; q 'histogram_quantile(0.99, sum by (service,le)(rate(unheaded_http_request_duration_seconds_bucket[1h])))'; q 'sum by (service) (rate(unheaded_http_request_duration_seconds_sum[1h])) / sum by (service) (rate(unheaded_http_request_duration_seconds_count[1h]))'; q 'count(unheaded_http_request_duration_seconds_bucket)'
  ```
  Reference: p50 0.0025 and p99 0.00495 for every service but logstore-writer; Wotan's mean about 0.00005 to 0.00014 s; 1,056 bucket series.
- [ ] **Step 14** [B] ~1m: SSV-16 before: `q 'count(count by (job, instance) (process_cpu_seconds_total))'; q 'count({__name__=~"huginn_cgroup_.*"})'`. Reference: 24 and 0.
- [ ] **Step 15** [B] ~2m: SSV-08, SSV-09 before: the long-running units.
  ```bash
  for u in $(systemctl list-units --type=service --state=running --no-legend 'unheaded-*' 'huginn*' 'zhen-*' | awk '{print $1}'); do systemctl show -p Id,Restart,RestartUSec,StartLimitIntervalUSec,StartLimitBurst,NRestarts $u | tr '\n' ' '; echo; done
  ```
- [ ] **Step 16** [B] ~1m: SSV-17 to SSV-19 before: `source $LIB; pq 'slo:error_budget_remaining'; firing`. Reference: six negative; `ErrorBudgetBurn` ci-develop four tiers firing.
- [ ] **Step 17** [B] ~1m: SSV-12, SSV-13 before: `curl -s 127.0.0.1:16671/metrics | grep -E '^trace_(collector_batch_latency|publisher_flush_duration)_seconds_count'; source $LIB; pq 'count by (component_id) (vector_buffer_discarded_events_total)'`. Reference: batch latency count 0; seven flush counts above 0; no discarded series.
- [ ] **Step 18** [B] ~1m: SSV-21, SSV-22 before (no walk: one directory listing, one metric): `ls -la /srv/backups/; source $LIB; q 'sum(vm_data_size_bytes)'`. Reference: `unheaded-well` only; 219 MB.
- [ ] **Step 19** [B] ~2m: SSV-24, SSV-25 before. vor's journal (counts only) and nginx's format (field count only: the apt log holds LAN addresses, never print a line).
  ```bash
  journalctl -u unheaded-vor --since -7d --no-pager -q | wc -l; journalctl -u unheaded-vor --since -7d --no-pager -q | grep -c '"uri"'; grep -n 'access_log\|log_format' /etc/nginx/nginx.conf /etc/nginx/sites-enabled/unheaded-apt.conf
  ```
  Reference: about 171 lines, 0 request lines; `access_log` with no format name (the default `combined`, no request time).
- [ ] **Step 20** [B] ~11m: huginn's own cost before (for Phase 4's after): `a=$(systemctl show -p CPUUsageNSec --value huginn); sleep 600; b=$(systemctl show -p CPUUsageNSec --value huginn); echo "huginn cpu share: $(( (b-a)/600000000 ))e-3 of a core"`. Run Steps 21-22 while it waits.
- [ ] **Step 21** [W] ~5m: Every before from Steps 10-20 into the Fixes table (one cell per row, with the command's name and the UTC time). SSV-23 (qa-smoke): "taken at Step 263 (it opens a psql session)". SSV-02: "0 rules; `node_vmstat_oom_kill` 6".
- [ ] **Step 22** [C][V] ~1m: **PHASE 0 EXIT GATE**: `$PROG` has Decisions and a before in every row except SSV-23.
  ```bash
  cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE && grep -c '^| SSV-' docs/battle-plans/progress/S-SEE-AND-SURVIVE-progress.md && git add docs/battle-plans/progress/S-SEE-AND-SURVIVE-progress.md && git commit -S -m "[PLAN S-SEE-AND-SURVIVE] Steps 1-22: decisions and the before readings"
  ```
  Expected: `28`, then the commit.

---

## PHASE 1: MAIL ON, AND A CAP FOR RUN DAYS (Steps 23-42)

**Goal**: `policy.yaml` says `mode: send`; the mailer and the generator know `mailer.run_days` (SS2), and a listed day gets the higher caps.
**Prerequisite**: Phase 0 gate.
**Time**: 1.5 h.
**Model**: Sonnet. **Agent**: lane `alerting` (Phases 1, 2, 7, 8 in this order, one worktree).

- [ ] **Step 23** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting -b sprint/s-see-and-survive-alerting sprint/s-see-and-survive`
- [ ] **Step 24** [R] ~10m: `cmd/alert-mailer/main.go` 50-100 (`loadSettings`), `cmd/alert-mailer/mailer.go` 40-200 (`Settings`, `NewMailer`, `deliver`, `rollDay`), `cmd/alert-mailer/mailer_test.go` (how a fake clock and a fake `Sender` are built), `cmd/alert-policy/main.go` 49-196.
- [ ] **Step 25** [W] ~15m: Red. In `cmd/alert-mailer/mailer_test.go`:
  - `TestRunDayRaisesDailyCap`: `Settings{Mode:"send", MaxPerDay:2, MaxPerHour:2, RunDayMaxPerDay:4, RunDayMaxPerHour:4, RunDates: map[string]bool{"2026-12-01": true}}`, clock at 2026-12-01 10:00 in `America/Chicago`. Four pages an hour apart (distinct alert names) are all `sent`; the fifth is `capped`. Same settings on 2026-12-02: the third is `capped`.
  - `TestRunDayRaisesHourlyCap`: on a run day with `MaxPerHour:1, RunDayMaxPerHour:3`, three pages inside one hour are `sent`, the fourth `capped-hour`.
  - `TestStatusShowsEffectiveCap`: `Status(false).MaxPerDay` is 4 on the run day and 2 the day after.
  In `cmd/alert-mailer/main_test.go` (create it if absent; `package main`), `TestLoadSettingsRunDays`, a table over policy files written to `t.TempDir()`:
  - no `run_days` block → `RunDates` empty, no error;
  - `run_days: {max_per_day: 24, max_per_hour: 6, dates: ["2026-12-01"]}` → loaded;
  - date `2026-13-01` → error naming `mailer.run_days.dates`;
  - `max_per_day: 3` (below `max_per_day` 6) → error "at least mailer.max_per_day";
  - `max_per_day: 51` → error "1..50";
  - `max_per_hour: 30` with `max_per_day: 24` → error "1..run_days.max_per_day".
- [ ] **Step 26** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && go test -count=1 ./cmd/alert-mailer -run 'RunDay|EffectiveCap|LoadSettingsRunDays' 2>&1 | tail -5`
- [ ] **Step 27** [V] ~1m: Red: `unknown field RunDayMaxPerDay` (or `undefined`). Any other failure → stop line, `[STUCK]`.
- [ ] **Step 28** [W] ~20m: Green.
  - `Settings` gains `RunDayMaxPerDay, RunDayMaxPerHour int` and `RunDates map[string]bool` (key `YYYY-MM-DD` in `Loc`).
  - `loadSettings`: the `Mailer` struct gains `RunDays struct { MaxPerDay int \`yaml:"max_per_day"\`; MaxPerHour int \`yaml:"max_per_hour"\`; Dates []string \`yaml:"dates"\` } \`yaml:"run_days"\``. With no block, the run caps equal the normal caps. With a block: each date parses with `time.Parse("2006-01-02", d)`; `MaxPerDay` in 1..50 and at least the normal `max_per_day`; `MaxPerHour` in 1..`run_days.max_per_day` and at least the normal `max_per_hour`. Error texts as Step 25 names them.
  - `mailer.go`: one method `func (m *Mailer) caps() (day, hour int)` returns the run caps when `m.cfg.RunDates[m.today()]`, else the normal ones. `deliver` and `Status` use it in place of `m.cfg.MaxPerDay` / `MaxPerHour`. The footer line "Mail %d of at most %d today" uses it. The `unheaded_alert_mailer_max_per_day` gauge reads `Status(false).MaxPerDay`.
- [ ] **Step 29** [B] ~2m: Step 26 again with `-race`.
- [ ] **Step 30** [V] ~1m: `ok`. Then the whole package: `go test -race -count=1 ./cmd/alert-mailer 2>&1 | tail -2` → `ok`.
- [ ] **Step 31** [C] ~1m: **COMMIT** `cmd/alert-mailer/main.go cmd/alert-mailer/mailer.go cmd/alert-mailer/mailer_test.go cmd/alert-mailer/main_test.go`: "Steps 23-31: alert-mailer raises its caps on listed run days".
- [ ] **Step 32** [W] ~10m: Red then green in `cmd/alert-policy`. `TestValidateRunDays` in `main_test.go` with the same four refusals as Step 25 (the generator decodes with `KnownFields(true)`, so without the field every policy with `run_days` is refused: that is the red). Then add the same `RunDays` struct to `Policy.Mailer` and the same checks to `validate()`.
- [ ] **Step 33** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && go test -race -count=1 ./cmd/alert-policy 2>&1 | tail -3`
- [ ] **Step 34** [V] ~1m: `ok`.
- [ ] **Step 35** [W] ~5m: `observability/alerting/policy.yaml`, `mailer:` block. Replace the four comment lines above `mode:` and the value with:
  ```yaml
    # dry-run: decide and log every mail, send none.
    # 2026-11-10 (Stevie): dry-run while the kingdom ran as a test cut on west.
    # 2026-11-15 (Stevie, ADR-147 decision 2): mail on. A stopped container and
    # a memory kill each mail (promote: below). S-SEE-AND-SURVIVE Phase 15.
    mode: send
  ```
  After `timezone:` add (SS2 A; with B use 50 and 12; with C skip this block and write "SS2 C" in `$PROG`):
  ```yaml
    # Run days (ADR-147 decision 2): the caps above are raised on these dates
    # (YYYY-MM-DD in timezone) so a stop mark that fires late in a load run is
    # still mailed. Add a date before the run; delete it after.
    run_days:
      max_per_day: 24
      max_per_hour: 6
      dates: []
  ```
- [ ] **Step 36** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && go run ./cmd/alert-policy --check 2>&1 | tail -3 && go run ./cmd/alert-mailer --help >/dev/null && echo help-ok`
- [ ] **Step 37** [V] ~1m: `[PASS]` (the generated files do not change: `run_days` is the mailer's alone) and `help-ok`.
- [ ] **Step 38** [W] ~15m: `scripts/monitoring/test-alert-path.sh`: a case 6, "a run day raises the cap": in the test policy it writes, set `max_per_day: 2`, `run_days: {max_per_day: 3, max_per_hour: 3, dates: [<today in America/Chicago>]}`; fire three distinct pages; expect three mails in Mailpit. Follow the file's existing case pattern (its own counters and `check` lines). Do not run it: throwaway containers wait for the E-GATE (Step 257).
- [ ] **Step 39** [B] ~1m: `bash -n scripts/monitoring/test-alert-path.sh && shellcheck scripts/monitoring/test-alert-path.sh 2>&1 | tail -3`
- [ ] **Step 40** [B] ~3m: GO-GATE `alerting ./cmd/alert-mailer/... ./cmd/alert-policy/...`
- [ ] **Step 41** [V] ~1m: All `ok`, ratchet `[PASS]`.
- [ ] **Step 42** [C][V] ~1m: **PHASE 1 EXIT GATE**. **COMMIT** `cmd/alert-policy/main.go cmd/alert-policy/main_test.go observability/alerting/policy.yaml scripts/monitoring/test-alert-path.sh`: "Steps 32-42: mail on in policy; run-day caps in the generator; the alert path test covers them". SSV-03 and SSV-05: status `in work (code)`.

---

## PHASE 2: THE ALERT PATH WATCHES ITSELF (Steps 43-62)

**Goal**: Alertmanager hands `AlertPipelineHeartbeat` to the mailer every 5 minutes. When none arrives for `pipeline.silent_after` (20 min), the mailer mails "alert pipeline silent" once, and "alert pipeline back" once when receipts return. Prometheus down, Alertmanager down or a broken route are then all mailed. (ADR-116 R4, in part.)
**Prerequisite**: Phase 1 gate.
**Time**: 2.5 h.
**Model**: Opus (time, state and concurrency in the only sender). **Agent**: lane `alerting`.

- [ ] **Step 43** [R] ~10m: `cmd/alert-mailer/mailer.go` (`State`, `save`, `Tick`, `Page`), `heartbeat.go` (the dead-man for hosts: its shape is the model), `main.go` 200-260 (handlers), `cmd/alert-policy/main.go` 288-378.
- [ ] **Step 44** [W] ~20m: Red, `cmd/alert-mailer/pipeline_test.go` (new), with the fake clock and fake sender of `mailer_test.go`, `Settings.PipelineSilentAfter = 20*time.Minute`:
  - `TestPipelineSilentMailsOnce`: start at t0, one receipt at t0+1m, ticks every minute to t0+60m → exactly one mail, subject containing `alert pipeline silent`, sent at the first tick after t0+21m.
  - `TestPipelineBackMailsOnce`: after that, a receipt at t0+61m → one mail `alert pipeline back`; further receipts and ticks → nothing.
  - `TestPipelineGraceAfterStart`: no receipt ever; the first mail comes at the first tick past start+20m, not before.
  - `TestPipelineOffWhenZero`: `PipelineSilentAfter = 0` → no mail in 2 h of ticks.
  - `TestPipelineStatePersists`: alerted state and last receipt survive `NewMailer` on the same state dir (no second "silent" mail after a restart while still silent).
  - `TestPipelineMailRespectsCap`: with the day's cap spent, the silent mail is `capped` and counted, not sent.
  - `TestPipelineHandler`: `POST /v1/alerts/pipeline` with a valid Alertmanager webhook body → 200 and the receipt time set; `GET` → 405; a body over `maxWebhookBytes` → 413.
- [ ] **Step 45** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && go test -count=1 ./cmd/alert-mailer -run Pipeline 2>&1 | tail -5`
- [ ] **Step 46** [V] ~1m: Red (undefined symbols).
- [ ] **Step 47** [W] ~40m: Green.
  - `Settings.PipelineSilentAfter time.Duration`, loaded from `pipeline.silent_after` (`parseDuration`; absent → 0, off; present → at least 10m, else an error).
  - `State` gains `PipelineLast time.Time` and `PipelineAlerted bool` (persisted with the rest; an old `state.json` without them loads).
  - `func (m *Mailer) PipelineSeen()`: under `m.mu`, sets `PipelineLast = now`; if `PipelineAlerted`, clears it and delivers a page message "alert pipeline back: receipts from Alertmanager resumed at <time>"; saves.
  - In `Tick()`: if `PipelineSilentAfter > 0` and `now - max(PipelineLast, m.started) > PipelineSilentAfter` and not `PipelineAlerted`: set it, deliver a page message "alert pipeline silent: no heartbeat from Alertmanager since <time>; Prometheus, Alertmanager or the route is down" with the runbook path `runbooks/observe/alert-pipeline-silent.yaml`, save.
  - Both messages go through `deliver` (the caps apply), urgency label `page`.
  - `main.go`: `mux.HandleFunc("POST /v1/alerts/pipeline", ...)` reading the body through the same size limit as `hook`; a gauge `unheaded_alert_mailer_pipeline_last_seen_timestamp_seconds`.
- [ ] **Step 48** [B] ~2m: Step 45 with `-race`, then `go test -race -count=1 ./cmd/alert-mailer 2>&1 | tail -2`.
- [ ] **Step 49** [V] ~1m: `ok` both.
- [ ] **Step 50** [C] ~1m: **COMMIT** `cmd/alert-mailer/*.go` by name: "Steps 43-50: the mailer mails when the alert pipeline goes silent, and when it is back".
- [ ] **Step 51** [W] ~15m: Red then green in `cmd/alert-policy`:
  - `Policy` gains `Pipeline struct { SilentAfter string \`yaml:"silent_after"\` } \`yaml:"pipeline"\``; `validate`: empty, or a duration of at least 10m.
  - `renderAlertmanager`: when set, the **first** route is `{receiver: pipeline, matchers: [alertname="AlertPipelineHeartbeat"], group_wait: 30s, group_interval: 1m, repeat_interval: 5m}` and a receiver `pipeline` with webhook `base + "pipeline"`, `send_resolved: false`.
  - Tests: `TestPipelineRouteFirst` (route 0 matches the heartbeat), `TestPipelineReceiverURL` (`http://alert-mailer:19130/v1/alerts/pipeline`), `TestPipelineAbsentNoRoute`, `TestValidatePipeline` (`5m` refused).
- [ ] **Step 52** [W] ~3m: `policy.yaml`, after the `page:` block:
  ```yaml
  # The alert path's own receipt (ADR-116 R4, ADR-147 stage 1): Alertmanager
  # hands AlertPipelineHeartbeat to the mailer every 5 minutes. When none has
  # arrived for silent_after, the mailer mails once that Prometheus,
  # Alertmanager or the route is down, and once more when receipts return.
  pipeline:
    silent_after: 20m
  ```
- [ ] **Step 53** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && go test -race -count=1 ./cmd/alert-policy 2>&1 | tail -2 && go run ./cmd/alert-policy && git diff --stat monitoring/alertmanager/alertmanager.yml`
- [ ] **Step 54** [V] ~1m: `ok`; `alertmanager.yml` gains one route and one receiver, nothing else.
- [ ] **Step 55** [W] ~15m: `runbooks/observe/alert-pipeline-silent.yaml` in the format of `runbooks/observe/wotan-topic-refused.yaml` (`status: draft`, `authored_by: claude`, empty approvers, `requires_sudo: false`). Notes: what the mail means, why the mailer and not a rule (a rule cannot report its own evaluator), the three causes. Steps: `docker ps --filter name=unheaded-prometheus --filter name=unheaded-alertmanager`; `curl -s 127.0.0.1:9090/-/healthy`; `curl -s 127.0.0.1:9093/-/healthy`; `curl -s 127.0.0.1:9093/api/v2/alerts | grep -c AlertPipelineHeartbeat` (expected 1); `docker logs --since 30m unheaded-alertmanager 2>&1 | tail -20`; start what is down with `cd /home/govan/tmp/unheaded/monitoring && sudo docker compose up -d <service>`.
- [ ] **Step 56** [W] ~5m: `runbooks/observe/heartbeat-missing.yaml` notes: one paragraph saying `AlertPipelineHeartbeat`'s absence is now mailed by the mailer (`alert-pipeline-silent.yaml`); its own steps are unchanged.
- [ ] **Step 57** [W] ~15m: `scripts/monitoring/test-alert-path.sh`, case 7, "the route reaches the mailer": the test policy sets `pipeline: {silent_after: 10m}` (the validator's floor; no override flag is added). Within 90 s of Prometheus's first evaluation, `curl` the throwaway mailer's `/metrics` and expect `unheaded_alert_mailer_pipeline_last_seen_timestamp_seconds` above 0. The silent and back mails are proved by the unit tests (Step 44) and the live drill (Step 332).
- [ ] **Step 58** [B] ~1m: `bash -n scripts/monitoring/test-alert-path.sh && shellcheck scripts/monitoring/test-alert-path.sh 2>&1 | tail -3`
- [ ] **Step 59** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && scripts/runbook-manifest.sh --write && git diff --stat runbooks/MANIFEST.sha256`
- [ ] **Step 60** [V] ~1m: One line added (the new runbook), one changed (heartbeat-missing).
- [ ] **Step 61** [B] ~5m: GO-GATE `alerting ./cmd/alert-mailer/... ./cmd/alert-policy/...`, then RULES-GATE `alerting`.
- [ ] **Step 62** [C][V] ~1m: **PHASE 2 EXIT GATE**: all green. **COMMIT** `cmd/alert-policy/main.go cmd/alert-policy/main_test.go observability/alerting/policy.yaml monitoring/alertmanager/alertmanager.yml runbooks/observe/alert-pipeline-silent.yaml runbooks/observe/heartbeat-missing.yaml runbooks/MANIFEST.sha256 scripts/monitoring/test-alert-path.sh`: "Steps 51-62: Alertmanager hands the pipeline heartbeat to the mailer". SSV-11: `in work (code)`.

---

## PHASE 3: BUCKETS THAT SEE A TENTH OF A MILLISECOND (Steps 63-75) [P]

**Goal**: `unheaded_http_request_duration_seconds` and Wotan's `wotan_http_request_duration_seconds` use 16 buckets from 0.1 ms to 10 s.
**Prerequisite**: Phase 0 gate.
**Time**: 45 min.
**Model**: Sonnet. **Agent**: lane `buckets`.

- [ ] **Step 63** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-SEE-AND-SURVIVE-buckets -b sprint/s-see-and-survive-buckets sprint/s-see-and-survive`
- [ ] **Step 64** [R] ~5m: `pkg/metrics/prom/prom.go` 60-90, `pkg/httputil/middleware.go` 30-100 and its tests, `services/wotan/internal/metrics/metrics.go` 60-80.
- [ ] **Step 65** [W] ~10m: Red.
  - `pkg/metrics/prom/prom_test.go` (or the package's existing test file): `TestLatencyBuckets`: `LatencyBuckets` has 16 entries, first `0.0001`, last `10`, strictly increasing, and contains `0.005`, `0.01`, `0.025` (the old bounds from 5 ms up stay, so old and new series share those `le` values).
  - `pkg/httputil/middleware_test.go`: `TestRequestDurationSeesSubMillisecond`: build `NewServiceMetrics` with a unique service name, serve one request through `Instrument`, scrape the registry through the same handler the package's tests already use, and assert a line with `le="0.00025"` for that service exists.
- [ ] **Step 66** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-buckets && go test -count=1 ./pkg/metrics/prom ./pkg/httputil -run 'LatencyBuckets|SubMillisecond' 2>&1 | tail -4`
- [ ] **Step 67** [V] ~1m: Red: `undefined: LatencyBuckets`.
- [ ] **Step 68** [W] ~5m: Green. In `pkg/metrics/prom/prom.go`, below `DefBuckets`:
  ```go
  // LatencyBuckets are request-latency buckets from 0.1 ms to 10 s (ADR-147,
  // S-SEE-AND-SURVIVE): DefBuckets start at 5 ms, and our services answer in
  // 0.1 to 0.4 ms, so every p50 read 2.5 ms and every p99 4.95 ms. Steps of
  // 2 to 2.5; the bounds from 5 ms up are DefBuckets' own.
  var LatencyBuckets = []float64{0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
  ```
  `pkg/httputil/middleware.go:56` and `services/wotan/internal/metrics/metrics.go:74`: `Buckets: prom.LatencyBuckets,`.
- [ ] **Step 69** [B] ~2m: Step 66 again with `-race`.
- [ ] **Step 70** [V] ~1m: `ok`.
- [ ] **Step 71** [B] ~5m: Readers of the old bounds: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-buckets && grep -rn 'le="0\.005"\|le="0\.0025"' --include=*.go --include=*.yml --include=*.json cmd pkg services config monitoring observability | grep -v _test | head`
- [ ] **Step 72** [V] ~1m: No hit, or only ones that still exist in the new set. A hit on a removed bound (none expected) → change it to the nearest new bound and log it.
- [ ] **Step 73** [B] ~5m: GO-GATE `buckets ./pkg/metrics/... ./pkg/httputil/... ./services/wotan/... ./cmd/dashboard-backend/...`
- [ ] **Step 74** [V] ~1m: Green. dashboard-backend's scraper tests pass unchanged (it reads any buckets).
- [ ] **Step 75** [C][V] ~1m: **PHASE 3 EXIT GATE**. **COMMIT** `pkg/metrics/prom/prom.go pkg/metrics/prom/prom_test.go pkg/httputil/middleware.go pkg/httputil/middleware_test.go services/wotan/internal/metrics/metrics.go`: "Steps 63-75: request latency buckets from 0.1 ms". SSV-15: `in work (code)`.

---

## PHASE 4: HUGINN READS EVERY CGROUP (Steps 76-94) [P]

**Goal**: huginn exports CPU, throttling, memory, swap, memory events, I/O by device, pressure and pids for every service, docker scope and slice, bounded, read-only, on whichever host it runs.
**Prerequisite**: Phase 0 gate.
**Time**: 3 h.
**Model**: Sonnet. **Agent**: lane `huginn`.

- [ ] **Step 76** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-SEE-AND-SURVIVE-huginn -b sprint/s-see-and-survive-huginn sprint/s-see-and-survive`
- [ ] **Step 77** [R] ~10m: `cmd/huginn/main.go` (`allMetrics`, flags 389-440), `cmd/huginn/logdirs.go` (the walk-with-budget pattern), `cmd/huginn/textfile.go`, `deploy/systemd/huginn.service`. Read one real cgroup's files to see the formats: `cat /sys/fs/cgroup/system.slice/huginn.service/{cpu.stat,memory.events,io.stat,memory.pressure}` (world-readable).
- [ ] **Step 78** [W] ~30m: Red. `cmd/huginn/cgroups_test.go`, each test building a fake tree in `t.TempDir()`:
  - `TestCgroupsSelect`: dirs `system.slice/huginn.service`, `system.slice/docker-<64 hex>.scope`, `system.slice/session-3.scope`, `user.slice/user-1000.slice/session-9.scope`, `unheaded.slice/unheaded-env.slice/docker-<64 hex>.scope`, `system.slice/foo.mount` → selected: `system.slice`, `system.slice/huginn.service`, the two docker scopes, `user.slice`, `user.slice/user-1000.slice`, `unheaded.slice`, `unheaded.slice/unheaded-env.slice`. Not selected: the session scopes, the mount.
  - `TestCgroupDockerID`: the docker scope's `cgroup` label is `system.slice/docker-<first 12 hex>.scope` and `container_id` is the 12 hex.
  - `TestCgroupCPU`: `cpu.stat` with `usage_usec 2500000`, `nr_periods 10`, `nr_throttled 4`, `throttled_usec 500000` → `huginn_cgroup_cpu_usage_seconds_total 2.5`, `huginn_cgroup_cpu_periods_total 10`, `huginn_cgroup_cpu_throttled_periods_total 4`, `huginn_cgroup_cpu_throttled_seconds_total 0.5`.
  - `TestCgroupMemory`: `memory.current 1048576`, `memory.max max`, `memory.swap.current 4096` → bytes 1048576, swap 4096, **no** `huginn_cgroup_memory_max_bytes` line; with `memory.max 2097152` the line is there.
  - `TestCgroupMemoryEvents`: `low 0 high 1 max 7 oom 2 oom_kill 2` → five lines `huginn_cgroup_memory_events_total{...,event="oom_kill"} 2` and so on.
  - `TestCgroupPressure`: `some avg10=0.00 avg60=0.00 avg300=0.00 total=1500000` / `full ... total=250000` in `io.pressure` → `huginn_cgroup_pressure_seconds_total{resource="io",kind="some"} 1.5` and `kind="full"} 0.25`.
  - `TestCgroupIOStat`: `259:0 rbytes=100 wbytes=200 rios=1 wios=2 dbytes=0 dios=0` with a device map `259:0 → nvme0n1` → `huginn_cgroup_io_bytes_total{...,device="nvme0n1",dir="read"} 100`, write 200, ops read 1, write 2; an unmapped `8:99` uses `device="8:99"`.
  - `TestCgroupTruncated`: 600 service dirs with `max 512` → 512 groups, `huginn_cgroups_truncated 1`, `huginn_cgroups_seen 512`.
  - `TestCgroupMissingFilesSkipped`: a dir with only `cgroup.procs` → its lines are absent and nothing errors.
  - `TestPromTextCgroupsAcceptedByTextfileRegex`: every emitted sample line matches `textfileSample` (the same regex huginn applies to textfiles).
- [ ] **Step 79** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-huginn && go test -count=1 ./cmd/huginn -run Cgroup 2>&1 | tail -5`
- [ ] **Step 80** [V] ~1m: Red (undefined).
- [ ] **Step 81** [W] ~60m: Green. `cmd/huginn/cgroups.go` (SPDX header; a header comment saying what is read, why the docker id is shortened, and that `memory.events` is hierarchical so a slice keeps the count of a scope that has gone):
  - `type CgroupStat struct { Path, ContainerID string; CPU map[string]float64; Mem map[string]float64; Events map[string]float64; Pressure map[[2]string]float64; IO map[[2]string]float64; Pids float64; HasPids bool }` or an equivalent shape.
  - `func collectCgroups(root string, maxDepth, maxGroups int, devs map[string]string) (stats []CgroupStat, truncated bool)`: walk with `os.ReadDir` to `maxDepth` (4); select per Step 78; stop at `maxGroups`.
  - `func blockDevices(sysDevBlock string) map[string]string`: reads `/sys/dev/block/<maj:min>/uevent` `DEVNAME=`; cached, refreshed every 10 minutes.
  - `func promTextCgroups(host string, stats []CgroupStat, truncated bool) string`: labels `host`, `cgroup`, `container_id` (docker scopes only), plus `event`, `resource`/`kind`, `device`/`dir` where Step 78 names them; values in seconds and bytes.
  - `main.go`: flag `-cgroup-root` (default `/sys/fs/cgroup`, empty turns the collector off); `allMetrics` appends `promTextCgroups(label, collectCgroups(*cgroupRoot, 4, 512, devs))`.
- [ ] **Step 82** [B] ~2m: Step 79 with `-race`.
- [ ] **Step 83** [V] ~1m: `ok`, 10 tests.
- [ ] **Step 84** [C] ~1m: **COMMIT** `cmd/huginn/cgroups.go cmd/huginn/cgroups_test.go cmd/huginn/main.go`: "Steps 76-84: huginn reads every service, docker scope and slice cgroup".
- [ ] **Step 85** [B] ~3m: Read the real tree once as the normal user (no write, no root): `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-huginn && go build -o /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/huginn ./cmd/huginn && /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/huginn --help | grep -c cgroup-root`
- [ ] **Step 86** [W] ~10m: A test-only entry point is not added. Instead write `TestCgroupsOnThisHost` guarded by `HUGINN_CGROUP_LIVE=1` (skips with a message otherwise): it calls `collectCgroups("/sys/fs/cgroup", 4, 512, blockDevices("/sys/dev/block"))` and asserts at least one docker scope, `system.slice/huginn.service` and `system.slice` are present, and that `truncated` is false.
- [ ] **Step 87** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-huginn && HUGINN_CGROUP_LIVE=1 go test -count=1 ./cmd/huginn -run TestCgroupsOnThisHost -v 2>&1 | tail -4`
- [ ] **Step 88** [V] ~1m: `PASS`. A reading of cgroupfs is not a reading of `/var`; it disturbs nothing.
  - Fails on `truncated` → Step 89.
- [ ] **Step 89** [D] ~5m: Count the selected groups the test logs. Over 512 means a branch is selected that should not be (a user's sessions, snap scopes): tighten the rule in Step 78's terms, add a test row for it. Two tries.
- [ ] **Step 90** [W] ~5m: `deploy/systemd/huginn.service`: comment block above `ExecStart` gains one line "Reads cgroupfs (read-only under ProtectControlGroups=yes) for per-cgroup series (ADR-147)". No other change: the sandbox already allows it.
- [ ] **Step 91** [B] ~5m: GO-GATE `huginn ./cmd/huginn/...` and `scripts/check-unit-ceilings.sh deploy/systemd/huginn.service 2>&1 | tail -1`.
- [ ] **Step 92** [V] ~1m: Green.
- [ ] **Step 93** [W] ~5m: `wiki/Monitoring.md` (or the page `grep -rl huginn wiki | head -3` names as huginn's): a short section "Per-cgroup series" listing the metric names of Appendix C and the selection rule.
- [ ] **Step 94** [C][V] ~1m: **PHASE 4 EXIT GATE**. **COMMIT** `cmd/huginn/cgroups_test.go deploy/systemd/huginn.service` and the wiki page: "Steps 85-94: live read of the cgroup tree; docs". SSV-02 and SSV-16: `in work (code)`.

---

## PHASE 5: WHAT EVERY CONTAINER AND UNIT IS DOING (Steps 95-114)

**Goal**: A root timer writes `host_state.prom` every 30 s: each watched container's running state, exit code, `OOMKilled`, health and id; each long-running unit's active, failed and restart count. huginn forwards it.
**Prerequisite**: Phase 0 gate.
**Time**: 2.5 h.
**Model**: Opus (a script that runs as root on a live host, series rule 13). **Agent**: lane `state`.

- [ ] **Step 95** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-SEE-AND-SURVIVE-state -b sprint/s-see-and-survive-state sprint/s-see-and-survive`
- [ ] **Step 96** [R] ~10m: `scripts/monitoring/ci-status-textfile.sh` (the house textfile script), `deploy/systemd/unheaded-well-backup.service` (a root unit's hardening), `cmd/huginn/textfile.go` 30-60 (what huginn accepts: no free comments, 64 KiB, 64 files).
- [ ] **Step 97** [W] ~30m: Red: `scripts/monitoring/test-host-state-textfile.sh` (SPDX header, `set -euo pipefail`). It puts fakes first on `PATH` in a temp dir:
  - `docker`: `ps -aq --filter label=com.docker.compose.project` prints four ids; `inspect <ids>` prints a fixture JSON array: `unheaded-wotan` running healthy (project `unheaded-dev`, service `wotan`); `unheaded-postgres` exited 137 with `OOMKilled: true`; `unheaded-volume-init` exited 0 (service `volume-init`); `netbox-docker-redis-1` (project `netbox-docker`); one name containing a double quote.
  - `systemctl`: `show` prints fixture blocks for `unheaded-kev-poller.service` (active, Type=simple, NRestarts=0), `unheaded-wiki-server.service` (failed, Type=simple, NRestarts=15), `unheaded-well-backup.service` (inactive, Type=oneshot).
  Cases, each a `check` line:
  1. `unheaded_container_running{name="unheaded-wotan",project="unheaded-dev",service="wotan"} 1`
  2. postgres: `running ... 0`, `unheaded_container_exit_code{name="unheaded-postgres"} 137`, `unheaded_container_oom_killed{name="unheaded-postgres"} 1`
  3. `unheaded_container_healthy{name="unheaded-wotan"} 1`; no `healthy` line for a container without a healthcheck
  4. no line names `unheaded-volume-init` (one-shot) or `netbox` (project not watched)
  5. the quoted name is escaped (`\"`) and the line passes huginn's sample regex
  6. `unheaded_unit_failed{unit="unheaded-wiki-server.service"} 1`, `unheaded_unit_restarts{unit="unheaded-wiki-server.service"} 15`, `unheaded_unit_active{unit="unheaded-kev-poller.service"} 1`; no line for the oneshot
  7. `unheaded_host_state_last_run_timestamp_seconds` present; `unheaded_host_state_docker_ok 1`
  8. with the fake `docker` exiting 1: units still written, `unheaded_host_state_docker_ok 0`, no container lines
  9. every line of the file matches huginn's `textfileSample` or `textfileComment` regex (copied into the test with a comment naming `cmd/huginn/textfile.go`), and the file is under 64 KiB
  10. the write is atomic: the file is replaced by `mv` from a temp file in the same directory (the test checks no `*.tmp` is left).
- [ ] **Step 98** [B] ~1m: `bash /home/govan/tmp/wt/S-SEE-AND-SURVIVE-state/scripts/monitoring/test-host-state-textfile.sh 2>&1 | tail -3`
- [ ] **Step 99** [V] ~1m: Red: the script under test does not exist.
- [ ] **Step 100** [W] ~45m: Green: `scripts/monitoring/host-state-textfile.sh <dir>`, house style of `ci-status-textfile.sh` (header says what it writes, why root: the Docker socket is root-equivalent, as the Well backup's unit says). Settings by environment, validated against `^[a-z0-9 _.*-]+$`: `HOST_STATE_PROJECTS` (default `unheaded-dev monitoring`), `HOST_STATE_ONESHOT` (compose service names skipped; default `volume-init clickhouse-schema`), `HOST_STATE_UNITS` (default `unheaded-*.service huginn.service zhen-*.service docker.service containerd.service nginx.service`). Containers: `docker ps -aq --filter label=com.docker.compose.project`, then one `docker inspect`, filtered and formatted with `jq` (escape `\`, `"` and newline in every label value). Units: `systemctl show --property=Id,Type,ActiveState,NRestarts $HOST_STATE_UNITS`, skipping `Type=oneshot`. Output `<dir>/host_state.prom` through a temp file and `mv`. Only `# HELP` and `# TYPE` comments.
- [ ] **Step 101** [B] ~2m: Step 98, then `shellcheck scripts/monitoring/host-state-textfile.sh scripts/monitoring/test-host-state-textfile.sh 2>&1 | tail -3`.
- [ ] **Step 102** [V] ~1m: 10 of 10 `PASS`; shellcheck clean.
- [ ] **Step 103** [C] ~1m: **COMMIT** the two scripts: "Steps 95-103: host-state textfile: containers and units".
- [ ] **Step 104** [W] ~15m: `deploy/systemd/unheaded-host-state.service`, header first (what, why root, the install lines as the Well backup's unit writes them):
  ```ini
  # Install (S-SEE-AND-SURVIVE Phase 14):
  #   sudo install -d -m 0755 /usr/local/lib/unheaded/monitoring
  #   sudo install -m 0755 scripts/monitoring/host-state-textfile.sh /usr/local/lib/unheaded/monitoring/
  #   sudo cp deploy/systemd/unheaded-host-state.{service,timer} /etc/systemd/system/
  #   sudo systemctl daemon-reload && sudo systemctl enable --now unheaded-host-state.timer
  [Unit]
  Description=Container and unit state for huginn's textfile directory (ADR-147)
  After=docker.service
  [Service]
  Type=oneshot
  ExecStart=/usr/local/lib/unheaded/monitoring/host-state-textfile.sh /var/lib/unheaded/textfile
  TimeoutStartSec=20s
  Nice=10
  IOSchedulingClass=idle
  MemoryMax=64M
  CPUQuota=20%
  UMask=0022
  NoNewPrivileges=yes
  ProtectSystem=strict
  ReadWritePaths=/var/lib/unheaded/textfile
  ProtectHome=yes
  PrivateTmp=yes
  PrivateDevices=yes
  PrivateNetwork=yes
  ProtectKernelTunables=yes
  ProtectKernelModules=yes
  ProtectKernelLogs=yes
  ProtectHostname=yes
  ProtectClock=yes
  RestrictNamespaces=yes
  RestrictRealtime=yes
  LockPersonality=yes
  MemoryDenyWriteExecute=yes
  RestrictAddressFamilies=AF_UNIX
  CapabilityBoundingSet=
  SystemCallArchitectures=native
  SystemCallFilter=@system-service
  ```
  The Docker socket and systemd's are path sockets, so `PrivateNetwork=yes` keeps them. `CapabilityBoundingSet=` is empty: root needs no capability to talk to either socket.
- [ ] **Step 105** [W] ~3m: `deploy/systemd/unheaded-host-state.timer`: `OnBootSec=1min`, `OnUnitActiveSec=30s`, `AccuracySec=5s`, `[Install] WantedBy=timers.target`.
- [ ] **Step 106** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-state && systemd-analyze verify deploy/systemd/unheaded-host-state.service deploy/systemd/unheaded-host-state.timer 2>&1 | grep -v 'host-state-textfile.sh' | head -5; scripts/check-unit-binaries.sh 2>&1 | tail -1; scripts/check-unit-ceilings.sh 2>&1 | tail -1`
- [ ] **Step 107** [V] ~1m: No warning except the missing ExecStart path (not installed yet); both gates `PASS`.
  - `check-unit-binaries.sh` refuses a script path → Step 108.
- [ ] **Step 108** [D] ~5m: Read how it exempts `unheaded-well-backup.service` (also a script under `/usr/local/lib/unheaded/`). Use the same exemption (name the script in the same list), nothing broader. Two tries.
- [ ] **Step 109** [W] ~5m: `config/logging/sources.yaml`: an entry for `unheaded-host-state.service` the way `unheaded-ci-status.service` has one (S-LOG-HARVEST's `check-log-sources.sh` fails a unit without one).
- [ ] **Step 110** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-state && ls scripts/check-log-sources.sh >/dev/null 2>&1 && scripts/check-log-sources.sh 2>&1 | tail -2`
- [ ] **Step 111** [V] ~1m: `PASS` (or the script does not exist: write that in `$PROG`).
- [ ] **Step 112** [W] ~5m: `wiki/Systemd-Units.md`: one row for `unheaded-host-state` (root, every 30 s, writes `host_state.prom`, what it reads).
- [ ] **Step 113** [B] ~1m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-state && git status --short`
- [ ] **Step 114** [C][V] ~1m: **PHASE 5 EXIT GATE**: test green, unit verified. **COMMIT** `deploy/systemd/unheaded-host-state.service deploy/systemd/unheaded-host-state.timer config/logging/sources.yaml wiki/Systemd-Units.md` (and the gate's list if Step 108 ran): "Steps 104-114: unheaded-host-state unit and timer". SSV-01 and SSV-08: `in work (code)`.

---

## PHASE 6: START LIMITS THAT CAN TRIP (Steps 115-130)

**Goal**: Under SS3 A, every long-running unit's start limit can trip (interval longer than burst times `RestartSec`), a gate keeps it so, and west's installed units get the same limits through a drop-in.
**Prerequisite**: Phase 5 gate (same lane). SS3 answered.
**Time**: 1 h.
**Model**: Opus (unit files that run on live hosts). **Agent**: lane `state`.

- [ ] **Step 115** [V] ~1m: SS3 in `$PROG`. **A** → Step 116. **B** → mark Steps 116-129 `[BLOCKED by SS3 B]`, write "units keep looping; UnitRestartLoop shows it" in SSV-09, go to Step 130.
- [ ] **Step 116** [W] ~25m: Red: `scripts/check-unit-start-limits.sh` (SPDX header; usage `scripts/check-unit-start-limits.sh [UNIT_FILE...]`, default `systemd/*.service deploy/systemd/*.service`). For each file whose `Restart=` is set and not `no`: read `StartLimitIntervalSec` (default 10 s; `0` or `infinity` means "never trips": FAIL), `StartLimitBurst` (default 5), `RestartSec` (default 100 ms). Units: bare numbers are seconds; suffixes `ms`, `s`, `min`, `h`. Pass when interval > burst x RestartSec. Output one `[FAIL] <file>: interval <i>s <= burst <b> x RestartSec <r>s` per failure and `[PASS] <n> restarting units can trip their start limit`, exit 1 on any failure, 2 on a parse error. Header comment: why (BlackMage F16; ADR-147 SS3).
- [ ] **Step 117** [B] ~1m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-state && bash scripts/check-unit-start-limits.sh 2>&1 | tail -40`
- [ ] **Step 118** [V] ~2m: Red: `[FAIL]` lines. `unheaded-trace-collector.service` and `unheaded-vor-refresh.service` are **not** among them (they already trip). Copy the list of failing files into `$PROG`; that count is SSV-09's before (repository side).
- [ ] **Step 119** [W] ~15m: Green. In each failing file's `[Unit]` section, after the last existing line of that section, add:
  ```ini
  # ADR-147 (SS3): after 10 starts in 15 minutes the unit stays failed, and
  # UnitFailed mails, instead of restarting for ever.
  StartLimitIntervalSec=15min
  StartLimitBurst=10
  ```
  A file whose `RestartSec` is over 90 s (10 x 90 s = 15 min) needs a longer interval: none on 2026-11-15; if one appears, use `StartLimitIntervalSec` = 12 x its `RestartSec` and log it.
- [ ] **Step 120** [B] ~1m: Step 117 again.
- [ ] **Step 121** [V] ~1m: `[PASS] <n> restarting units can trip their start limit`, exit 0.
- [ ] **Step 122** [W] ~5m: `deploy/systemd/dropins/10-start-limit.conf`:
  ```ini
  # SPDX-License-Identifier: GPL-3.0-or-later
  # Copyright (c) 2026 Stevie Bellis.
  # Installed as /etc/systemd/system/<unit>.d/10-start-limit.conf on hosts
  # whose installed unit files have drifted from deploy/systemd/ (ADR-147,
  # S-SEE-AND-SURVIVE Phase 14). Same lines the repository's units carry.
  [Unit]
  StartLimitIntervalSec=15min
  StartLimitBurst=10
  ```
- [ ] **Step 123** [W] ~10m: `scripts/check-gates-can-fail.sh`: a REGISTRY line beside `check-unit-ceilings`:
  `check-unit-start-limits|provoke_unit_start_limits|fast|StartLimitIntervalSec removed from unheaded-kev-poller.service`
  and a function `provoke_unit_start_limits()` copied from `provoke_unit_ceilings()` (the shape at its line 1076: `backup`, `sed -i '/^StartLimitIntervalSec=/d'` on `deploy/systemd/unheaded-kev-poller.service`, the `cmp` guard).
- [ ] **Step 124** [W] ~5m: Wire the gate where `check-unit-ceilings.sh` runs: `grep -n 'check-unit-ceilings' .github/workflows/*.yml scripts/ci-local.sh` and add `scripts/check-unit-start-limits.sh` beside each hit, same form.
- [ ] **Step 125** [B] ~1m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-state && shellcheck scripts/check-unit-start-limits.sh 2>&1 | tail -2 && systemd-analyze verify deploy/systemd/unheaded-kev-poller.service 2>&1 | grep -i startlimit | head -3`
- [ ] **Step 126** [V] ~1m: shellcheck clean; no StartLimit warning.
- [ ] **Step 127** [C] ~1m: **COMMIT** `scripts/check-unit-start-limits.sh scripts/check-gates-can-fail.sh deploy/systemd/dropins/10-start-limit.conf`, the unit files changed in Step 119 by name, and the CI files of Step 124: "Steps 115-127: start limits that can trip, and a gate".
- [ ] **Step 128** [B] ~3m: The gate can fail (clean tree now): `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-state && scripts/check-gates-can-fail.sh --only check-unit-start-limits 2>&1 | tail -3`
- [ ] **Step 129** [V] ~1m: `[PASS]` for the provocation (the gate failed when provoked).
- [ ] **Step 130** [C][V] ~1m: **PHASE 6 EXIT GATE**. `wiki/Systemd-Units.md`: one paragraph "Start limits" (or, under B, "Units restart for ever; UnitRestartLoop is the signal"). **COMMIT** it and `$PROG`. SSV-09: `in work (code)` or `declined (SS3 B)`.

---

## PHASE 7: RULES THAT READ A RESOURCE, A CONTAINER AND THE WATCHERS (Steps 131-160)

**Goal**: Four rule files with promtool tests: containers, resources, watchers, and the two repaired rules; a Prometheus job for VictoriaMetrics; the promotions SS1 chose; a runbook per alert family.
**Prerequisite**: Phase 2 gate (same lane). Metric names from Appendix C (Phases 4 and 5 may still be in flight: the tests use synthetic series).
**Time**: 4 h.
**Model**: Sonnet. **Agent**: lane `alerting`.

- [ ] **Step 131** [R] ~10m: `monitoring/prometheus/rules/host.yml` and `tests/host_test.yml` (house format: header comment naming producer, scrape job and why; `labels: {severity, urgency: none}`; `runbook_url`), `rules/collector.yml` 30-50, `rules/logs.yml` (VectorSinkDroppingEvents), their tests, `scripts/check-prometheus-rules.sh` (what it requires of a new file).
- [ ] **Step 132** [W] ~30m: Red: `monitoring/prometheus/rules/tests/containers_test.yml` (`rule_files: [../containers.yml]`, `evaluation_interval: 1m`). Series carry `{job="huginn", host="west"}` plus the names below. Cases:
  1. ContainerStopped fires: `unheaded_container_running{name="unheaded-captain",project="unheaded-dev",service="captain"}` `1x10 0x10` → firing at 14m, label `name="unheaded-captain"`.
  2. Quiet for a container that has not run in 7 days: interval 1h, `0x200` → quiet at 190h.
  3. Quiet under a scope mark: same as 1 plus `unheaded_planned_stop_until_timestamp_seconds{scope="unheaded-captain"}` `1e10x20` → quiet at 14m.
  4. Quiet under `scope="all"`.
  5. Fires once a mark has expired: mark value `600` (promtool time starts at 0), container 0 from minute 0 after one 1 → firing at 14m.
  6. MemoryKill fires: `huginn_cgroup_memory_events_total{cgroup="system.slice",event="oom_kill"}` `0 0 1x40` → firing at 3m and still at 30m (keep_firing_for); `cgroup="user.slice"` rising → quiet.
  7. HostOOMKill fires on `node_vmstat_oom_kill{job="node_exporter",instance="[fd00:dead:beef::1]:9100"}` `6 6 7x20`.
  8. ContainerUnhealthy fires after 5m of `unheaded_container_healthy 0` with `running 1`; quiet with `running 0` (ContainerStopped owns that).
  9. ContainerCPUThrottled fires after 30m at a 0.5 ratio; quiet at 0.1.
  10. ContainerMemoryNearLimit fires at 0.97 for 31m; quiet when `huginn_cgroup_memory_max_bytes` is absent.
- [ ] **Step 133** [W] ~30m: Red: `tests/resources_test.yml`, series on `{job="node_exporter",instance="[fd00:dead:beef::1]:9100"}` or `{job="huginn",host="west"}`:
  1. HostMemoryPressure: `node_pressure_memory_stalled_seconds_total` `0+12x30` (0.2 s/s) → firing at 16m; `0+3x30` → quiet.
  2. HostSwapping: `node_vmstat_pswpin` `0+120000x40` (2,000/s) → firing at 26m; `0+6000x40` → quiet.
  3. HostSwapNearFull: `node_memory_SwapTotal_bytes 4294967296`, `node_memory_SwapFree_bytes 536870912` for 40m → firing at 35m; total `0` → quiet.
  4. HostIOPressure: `node_pressure_io_stalled_seconds_total` `0+24x60` (0.4 s/s) → firing at 50m; `0+6x60` → quiet.
  5. HostCPUPressure: `node_pressure_cpu_waiting_seconds_total` `0+36x60` → firing at 50m.
  6. HostTemperatureNearCritical: `node_hwmon_temp_celsius{chip="pci0000:00_0000:00:18_3",sensor="temp1"} 66` with `node_hwmon_temp_crit_celsius{...} 70` → firing at 12m; `60` → quiet; a sensor with no crit series → quiet.
  7. UnitFailed: `unheaded_unit_failed{unit="unheaded-wiki-server.service"} 1x10` → firing at 6m.
  8. UnitRestartLoop: `unheaded_unit_restarts{unit=...}` `0 0 1 2 3 3 3` → firing at 4m; `15x40` → quiet.
  9. HostStateStale: `unheaded_host_state_last_run_timestamp_seconds 0x20` → firing at 11m (time() - 0 > 300 for 5m); `unheaded_host_state_docker_ok 0x10` → firing at 6m.
- [ ] **Step 134** [W] ~20m: Red: `tests/watchers_test.yml`:
  1. VictoriaMetricsDown: `up{job="victoria",instance="[fd00:dead:beef::1]:8428"} 0x10` → firing at 6m.
  2. HuginnDown: `up{job="huginn"} 0x10` → firing at 6m.
  3. AlertMailerDown: `up{job="alert-mailer"} 0x10` → firing at 6m.
  4. PrometheusRemoteWriteFailing: `prometheus_remote_storage_samples_failed_total{job="prometheus"}` `0+10x30` → firing at 26m; lag: `prometheus_remote_storage_highest_timestamp_in_seconds 0+60x30` with `prometheus_remote_storage_queue_highest_sent_timestamp_seconds{remote_name="x",url="y"} 0x30` → firing at 26m.
  5. AlertmanagerNotReached: `prometheus_notifications_alertmanagers_discovered 0x20` → firing at 11m.
- [ ] **Step 135** [W] ~15m: Red: replace the `TraceCollectorHighLatency` cases in `tests/collector_test.yml`: series `trace_publisher_flush_duration_seconds_count{job="trace-collector",instance="i",host="west",topic="traces.flow"}` `0+100x40` with `..._bucket{...,le="0.1"}` `0+95x40` → firing at 26m (5% over 0.1 s); `0+99.8x40` → quiet; count flat → quiet.
- [ ] **Step 136** [W] ~10m: Red: replace the `VectorSinkDroppingEvents` cases in `tests/logs_test.yml`: `vector_buffer_discarded_events_total{job="vector",instance="v",component_id="well_logs"}` `0 0 5 5` → firing at 2m; no series → quiet.
- [ ] **Step 137** [B] ~5m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && scripts/check-prometheus-rules.sh 2>&1 | tail -8`
- [ ] **Step 138** [V] ~1m: Red: the three new test files name rule files that do not exist; the collector and logs cases fail.
- [ ] **Step 139** [W] ~25m: `monitoring/prometheus/rules/containers.yml`. Header comment: producers (`unheaded-host-state.timer` through huginn's textfile directory; huginn's own `huginn_cgroup_*`), job `huginn`, decisions 1 and 2, the planned-stop mark. Rules (each `labels: {severity: <s>, urgency: none}`, an `annotations.summary`, `description` and `runbook_url`):
  ```yaml
  - alert: ContainerStopped
    expr: |
      (
        unheaded_container_running == 0
        and on (name) max_over_time(unheaded_container_running[7d]) == 1
      )
      unless on (name) label_replace(unheaded_planned_stop_until_timestamp_seconds > time(), "name", "$1", "scope", "(.+)")
      unless on () (max(unheaded_planned_stop_until_timestamp_seconds{scope="all"}) > time())
    for: 2m
    labels: {severity: critical, urgency: none}
    annotations:
      summary: "Container {{ $labels.name }} on {{ $labels.host }} is stopped; nothing restarts it"
      description: "It ran in the last 7 days. Restart policy is no (ADR-147 decision 1). Why: unheaded_container_exit_code and unheaded_container_oom_killed for this name; docker logs --tail 50 {{ $labels.name }}."
      runbook_url: "runbooks/observe/container-stopped.yaml"
  - alert: MemoryKill
    expr: sum by (host, cgroup) (increase(huginn_cgroup_memory_events_total{event="oom_kill", cgroup=~"system.slice|unheaded.slice"}[10m])) > 0
    keep_firing_for: 30m
    labels: {severity: critical, urgency: none}
    annotations: {summary: "A process in {{ $labels.cgroup }} on {{ $labels.host }} was killed for memory", description: "memory.events oom_kill rose in the stack's slice. It is hierarchical, so it keeps the count of a container that died with its scope. Which one: huginn_cgroup_memory_events_total{event=\"oom_kill\"} by cgroup, unheaded_container_oom_killed == 1.", runbook_url: "runbooks/observe/memory-kill.yaml"}
  - alert: HostOOMKill
    expr: increase(node_vmstat_oom_kill[10m]) > 0
    keep_firing_for: 30m
    labels: {severity: warning, urgency: none}
  - alert: ContainerUnhealthy
    expr: unheaded_container_healthy == 0 and on (name) unheaded_container_running == 1
    for: 5m
    labels: {severity: warning, urgency: none}
  - alert: ContainerCPUThrottled
    expr: sum by (host, cgroup, container_id) (rate(huginn_cgroup_cpu_throttled_periods_total[15m])) / sum by (host, cgroup, container_id) (rate(huginn_cgroup_cpu_periods_total[15m])) > 0.25
    for: 30m
    labels: {severity: warning, urgency: none}
  - alert: ContainerMemoryNearLimit
    expr: huginn_cgroup_memory_bytes / huginn_cgroup_memory_max_bytes > 0.95
    for: 30m
    labels: {severity: warning, urgency: none}
  ```
  The last four take annotations in the same form (HostOOMKill and the container ones link `memory-kill.yaml`, `container-stopped.yaml`, `host-pressure.yaml`, `memory-kill.yaml` in that order).
- [ ] **Step 140** [W] ~20m: `monitoring/prometheus/rules/resources.yml`. Header: producers node_exporter (job `node_exporter`, both hosts) and host-state; decision 12 for swap and memory pressure; the 5-day peaks of the Grounding as the reason for each bar. Rules, each `urgency: none`:
  - `HostMemoryPressure`: `rate(node_pressure_memory_stalled_seconds_total[5m]) > 0.10`, for 10m, critical.
  - `HostSwapping`: `rate(node_vmstat_pswpin[10m]) > 1000`, for 15m, warning.
  - `HostSwapNearFull`: `(node_memory_SwapTotal_bytes - node_memory_SwapFree_bytes) / node_memory_SwapTotal_bytes > 0.8 and node_memory_SwapTotal_bytes > 0`, for 30m, warning.
  - `HostIOPressure`: `rate(node_pressure_io_stalled_seconds_total[15m]) > 0.30`, for 30m, warning.
  - `HostCPUPressure`: `rate(node_pressure_cpu_waiting_seconds_total[15m]) > 0.50`, for 30m, warning.
  - `HostTemperatureNearCritical`: `node_hwmon_temp_celsius >= on (instance, chip, sensor) (node_hwmon_temp_crit_celsius - 5)`, for 10m, warning.
  - `UnitFailed`: `unheaded_unit_failed == 1`, for 5m, critical.
  - `UnitRestartLoop`: `delta(unheaded_unit_restarts[30m]) >= 3`, warning.
  - `HostStateStale`: `(time() - unheaded_host_state_last_run_timestamp_seconds > 300) or (unheaded_host_state_docker_ok == 0)`, for 5m, warning.
  Runbooks: the pressure, swap and temperature alerts `runbooks/observe/host-pressure.yaml` (swap ones may link the existing `runbooks/observe/swap-monitoring.yaml` instead: read it, choose, log); units `runbooks/observe/unit-restart.yaml`; HostStateStale `runbooks/observe/monitoring-path.yaml`.
- [ ] **Step 141** [W] ~15m: `monitoring/prometheus/rules/watchers.yml`. Header: "the watchers are unwatched" (ADR-147); what each cannot do (a rule cannot report its own evaluator: the mailer's pipeline mail does that, Phase 2; west lost is plan 6's). Rules, `urgency: none`, `runbooks/observe/monitoring-path.yaml`:
  - `VictoriaMetricsDown`: `up{job="victoria"} == 0`, for 5m, critical.
  - `HuginnDown`: `up{job="huginn"} == 0`, for 5m, critical.
  - `AlertMailerDown`: `up{job="alert-mailer"} == 0`, for 5m, critical.
  - `PrometheusRemoteWriteFailing`: `rate(prometheus_remote_storage_samples_failed_total[10m]) > 0 or (prometheus_remote_storage_highest_timestamp_in_seconds - ignoring (remote_name, url) group_right prometheus_remote_storage_queue_highest_sent_timestamp_seconds > 300)`, for 15m, warning.
  - `AlertmanagerNotReached`: `rate(prometheus_notifications_errors_total[10m]) > 0 or prometheus_notifications_alertmanagers_discovered < 1`, for 10m, warning.
- [ ] **Step 142** [W] ~10m: `rules/collector.yml`, `TraceCollectorHighLatency`:
  ```yaml
  # Share of flushes to Wotan over 100 ms, only while flushes happen. The
  # histogram is trace_publisher_flush_duration_seconds (top bucket 0.1 s);
  # trace_collector_batch_latency_seconds was never observed (S-WATCH-TRUE).
  - alert: TraceCollectorHighLatency
    expr: |
      (
        sum by (job, instance, host) (rate(trace_publisher_flush_duration_seconds_count[10m]))
        - sum by (job, instance, host) (rate(trace_publisher_flush_duration_seconds_bucket{le="0.1"}[10m]))
      )
      / sum by (job, instance, host) (rate(trace_publisher_flush_duration_seconds_count[10m])) > 0.01
      and sum by (job, instance, host) (rate(trace_publisher_flush_duration_seconds_count[10m])) > 0
    for: 15m
  ```
  Keep its labels and runbook; summary "Trace collector on {{ $labels.host }}: over 1% of flushes to Wotan take over 100 ms".
  `rules/logs.yml`, `VectorSinkDroppingEvents`: `expr: sum by (instance, component_id) (increase(vector_buffer_discarded_events_total{job="vector"}[5m])) > 0`; description "Sink {{ $labels.component_id }} discarded events: a buffer with when_full: drop_newest is full (well_logs today; the ClickHouse sink blocks instead)".
- [ ] **Step 143** [W] ~5m: `monitoring/prometheus/prometheus.yml`, after the `huginn` job:
  ```yaml
    # VictoriaMetrics itself (ADR-147, watchers.yml VictoriaMetricsDown). Its
    # /metrics is open; its admin endpoints need the key (docker-compose.yml).
    - job_name: victoria
      static_configs:
        - targets: ['[fd00:dead:beef::1]:8428']
          labels: {host: west}
  ```
- [ ] **Step 144** [W] ~40m: Runbooks in `runbooks/observe/`, format of `wotan-topic-refused.yaml` (`status: draft`, `authored_by: claude`, empty approvers):
  - `container-stopped.yaml`: why it stays down (decision 1); read `docker inspect -f '{{.State.ExitCode}} {{.State.OOMKilled}} {{.State.FinishedAt}}' <name>`, `docker logs --tail 50 <name>`; start it with `cd /home/govan/tmp/unheaded && sudo docker compose up -d <service>` (or `monitoring/`); a stop on purpose: `scripts/monitoring/planned-stop.sh <name> <duration>`. `requires_sudo: true`.
  - `memory-kill.yaml`: which cgroup (`huginn_cgroup_memory_events_total{event="oom_kill"}` by cgroup), `journalctl -k --since -30min | grep -i -E 'killed process|oom'`, the container's limit; a limit is raised only after the growth is explained (Micromanager 4.5).
  - `host-pressure.yaml`: memory, I/O and CPU pressure, swap: `cat /proc/pressure/{memory,io,cpu}`, the top cgroups by `huginn_cgroup_pressure_seconds_total` rate, `free -m`, `swapon --show`; never walk `/var`.
  - `host-temperature.yaml`: sensors, east's 70 °C mark, the trace collector on east as the known load.
  - `unit-restart.yaml`: `systemctl status <unit>`, `journalctl -u <unit> -n 50`, `systemctl reset-failed <unit>` after the cause is fixed.
  - `monitoring-path.yaml`: VictoriaMetrics, huginn, host-state, remote write, mailer, Alertmanager: one check and one start command each.
- [ ] **Step 145** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && scripts/runbook-manifest.sh --write && git diff --stat runbooks/MANIFEST.sha256`
- [ ] **Step 146** [V] ~1m: Six lines added, nothing else changed.
- [ ] **Step 147** [B] ~8m: RULES-GATE `alerting`.
- [ ] **Step 148** [V] ~2m: Green: every file `SUCCESS`, including the three new tests and the two changed ones.
  - A case fails → Step 149.
- [ ] **Step 149** [D] ~10m: promtool prints the expected and got alert sets. A `for` boundary off by one interval → move the `eval_time` one interval later, never loosen the rule. `keep_firing_for` refused by the 3.10 image → it is supported from 2.42: read the image tag the gate prints. A `label_replace` on an empty vector is fine; an error on `group_right` means a label set is not unique: add the missing label to `ignoring`. Two tries, then `[STUCK]`.
- [ ] **Step 150** [C] ~1m: **COMMIT** the five rule files, five test files, `prometheus.yml`, six runbooks, `runbooks/MANIFEST.sha256`: "Steps 131-150: rules for containers, memory kills, pressure, swap, heat, units and the monitoring path; two rules that could not fire repaired".
- [ ] **Step 151** [W] ~20m: `observability/alerting/policy.yaml` `promote:`, per SS1. Under **A**, append one entry per row of "Proposed urgency" whose column is not `none`, each with a `reason:` of two to four lines in the house voice, naming the decision where one applies:
  - ContainerStopped, page: "ADR-147 decision 1: restart stays no on west, so a container that stops stays down until a person starts it; decision 2: it mails. No SLO covers 16 of the containers."
  - MemoryKill, page: "Decision 2. A killed container stays down; a killed child leaves its container up and broken. No SLO sees either."
  - HostMemoryPressure, page: "Decision 12. A host stalled on memory stalls every service at once, and on west the next swap is the database disk."
  - HostSwapping and HostSwapNearFull, ticket: "Decision 12: swap use alerts."
  - HostIOPressure, ticket: "West's databases share one 7,200 rpm disk; a sustained stall slows every store, and no SLO reads latency yet."
  - HostTemperatureNearCritical, ticket: "East has read 68 °C against a 70 °C mark; no SLO sees heat."
  - UnitFailed, page (SS3 A) or ticket (B): "A unit that has given up stays down like a stopped container."
  - UnitRestartLoop, ticket: "A unit restarting for ever is a failure nobody is told about."
  - VictoriaMetricsDown, HuginnDown, HostStateStale, PrometheusRemoteWriteFailing, ticket: "The SLO data and the stop-mark series come through these; without them the SLOs cannot burn."
  - HostDiskFull, page: "A full /var stops the Well, ClickHouse and Prometheus together; the SLOs burn only afterwards."
  - HostDiskAlmostFull and NodeExporterDown, ticket.
  Under **B**: only ContainerStopped and MemoryKill, as pages, with the same reasons.
- [ ] **Step 152** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && go run ./cmd/alert-policy && git diff --stat monitoring/alertmanager/alertmanager.yml && grep -c 'alertname=' monitoring/alertmanager/alertmanager.yml`
- [ ] **Step 153** [V] ~1m: One route per promotion added (the count rises by the number of entries added in Step 151); `slo.yml` unchanged.
- [ ] **Step 154** [B] ~8m: RULES-GATE `alerting`.
- [ ] **Step 155** [V] ~1m: Green.
- [ ] **Step 156** [C] ~1m: **COMMIT** `observability/alerting/policy.yaml monitoring/alertmanager/alertmanager.yml`: "Steps 151-156: what mails (SS1 <A|B>)".
- [ ] **Step 157** [B] ~2m: No new rule reads a service plan 2 removes: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && grep -n -E 'architect|captain|micromanager|monad|sophia|cuirass' monitoring/prometheus/rules/{containers,resources,watchers}.yml | grep -v '^\s*#'`
- [ ] **Step 158** [V] ~1m: Only the test fixture name `unheaded-captain` in tests (fixtures are not rules); no hit in the three rule files.
- [ ] **Step 159** [W] ~10m: `wiki/Alerting.md`: a table of the new alerts (name, what, urgency, runbook) and one line each for the two repaired rules.
- [ ] **Step 160** [C][V] ~1m: **PHASE 7 EXIT GATE**: RULES-GATE green. **COMMIT** `wiki/Alerting.md` and `$PROG`. SSV-01, 02, 04, 06, 07, 08, 10, 12, 13: `in work (rules)`.

---

## PHASE 8: SLOS THAT MEAN SOMETHING (Steps 161-187)

**Goal**: A planned stop, marked by `scripts/monitoring/planned-stop.sh`, counts as good in every SLI it names; `ci-develop` is recorded and notifies nobody; the Wotan SLO says what it measures; a `store-backups` SLO and backup rules exist for ClickHouse and VictoriaMetrics.
**Prerequisite**: Phase 7 gate (same lane).
**Time**: 3 h.
**Model**: Opus (a script that writes as root; the generator). **Agent**: lane `alerting`.

- [ ] **Step 161** [W] ~20m: Red: `scripts/monitoring/test-planned-stop.sh` (SPDX, `set -euo pipefail`; `PLANNED_STOP_DIR` a temp dir, `PLANNED_STOP_SUDO=` empty so it writes directly). Cases:
  1. `planned-stop.sh all 30m` → the file has `unheaded_planned_stop_until_timestamp_seconds{scope="all"} <now+1800 ±5>`.
  2. `planned-stop.sh wotan 1h` → two sample lines.
  3. `planned-stop.sh all 15m` → the `all` line replaced (still two lines), value now+900 ±5.
  4. `planned-stop.sh --end wotan` → one line left.
  5. A line already past its time is dropped on the next write.
  6. `planned-stop.sh 'Bad Scope' 10m` → exit 2; `planned-stop.sh all 31d` → exit 2; `planned-stop.sh all 10x` → exit 2.
  7. `planned-stop.sh --list` prints `all <UTC time>`.
  8. Every line passes huginn's `textfileSample` or `textfileComment` regex (copied, with a comment naming `cmd/huginn/textfile.go`); no other comment lines.
  9. `planned-stop.sh --end` with no scope empties the marks (the file holds only `# HELP` and `# TYPE`).
- [ ] **Step 162** [B] ~1m: `bash /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting/scripts/monitoring/test-planned-stop.sh 2>&1 | tail -3` → red (no script).
- [ ] **Step 163** [W] ~30m: Green: `scripts/monitoring/planned-stop.sh`. Usage block as Step 161 names; scope `^[a-z0-9][a-z0-9_.-]{0,63}$` (`all`, an SLO name, or a container name); duration `^[0-9]+(m|h|d)$`, at most 30d. File `${PLANNED_STOP_DIR:-/var/lib/unheaded/textfile}/planned_stop.prom`. It builds the new content in a `mktemp` file, then: as root, `install -m 0644 <tmp> <file>`; else `${PLANNED_STOP_SUDO-sudo -n} install -m 0644 <tmp> <file>`; a refused sudo exits 1 with "planned-stop: cannot write <file>: run with sudo". Header comment: decision 4, who calls it, that huginn forwards it, that the SLO generator and ContainerStopped read it.
- [ ] **Step 164** [B] ~1m: Step 162 again, then `shellcheck scripts/monitoring/planned-stop.sh scripts/monitoring/test-planned-stop.sh 2>&1 | tail -2`.
- [ ] **Step 165** [V] ~1m: 9 of 9 `PASS`; shellcheck clean.
- [ ] **Step 166** [W] ~15m: `scripts/monitoring/maintenance.sh`: a flag `--planned[=SCOPE]` before the duration (`maintenance.sh --planned 30m "reboot west"`, scope default `all`). After the silences are made it runs `"$(dirname "$0")/planned-stop.sh" "$scope" "$dur"`; `--end` also runs `planned-stop.sh --end`. The header's usage lines say so, and that a mark stops the SLO budgets from burning (decision 4) while a silence only stops mail.
  `scripts/kingdom-shutdown.sh` after line 118: `run "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/monitoring/planned-stop.sh" all 30d >/dev/null 2>&1 || log_warn "planned-stop mark not written: the SLO budgets will burn while the stack is down"`.
  `scripts/kingdom-startup.sh` after line 150: the same call with `all 15m` (it replaces the 30d mark: the warm-up is the planned tail).
- [ ] **Step 167** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && bash -n scripts/monitoring/maintenance.sh scripts/kingdom-shutdown.sh scripts/kingdom-startup.sh && shellcheck scripts/monitoring/maintenance.sh 2>&1 | tail -2 && scripts/monitoring/maintenance.sh --help 2>&1 | grep -c planned`
- [ ] **Step 168** [V] ~1m: Parses; shellcheck clean; at least 1.
- [ ] **Step 169** [C] ~1m: **COMMIT** `scripts/monitoring/planned-stop.sh scripts/monitoring/test-planned-stop.sh scripts/monitoring/maintenance.sh scripts/kingdom-shutdown.sh scripts/kingdom-startup.sh`: "Steps 161-169: a planned stop is marked, not only silenced".
- [ ] **Step 170** [W] ~20m: Red in `cmd/alert-policy/main_test.go`:
  - `TestGoodHonoursPlannedStop`: the rendered `slo:good` for `wotan` is exactly `clamp_max(((avg(up{job="wotan"})) or on() vector(0)) + ((max(unheaded_planned_stop_until_timestamp_seconds{scope=~"all|wotan"}) > bool time()) or on() vector(0)), 1)`.
  - `TestNotifyFalse`: an SLO with `notify: false` renders `slo:good`, `slo:objective` and `slo:error_budget_remaining` and no `ErrorBudgetBurn` with its name.
  - `TestNotifyDefaultTrue`: without the field, four tiers render.
  `monitoring/prometheus/rules/tests/slo_test.yml`, three cases on `wotan`: `up{job="wotan"} 0x30` alone → `slo:good{slo="wotan"}` 0 at 10m; with `unheaded_planned_stop_until_timestamp_seconds{scope="all"} 1e10x30` → 1; with the mark at `300` → 0 at 10m. And: no `ErrorBudgetBurn{slo="ci-develop"}` fires with `unheaded_ci_run_success 0x300`.
- [ ] **Step 171** [B] ~3m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && go test -count=1 ./cmd/alert-policy -run 'PlannedStop|Notify' 2>&1 | tail -4`
- [ ] **Step 172** [V] ~1m: Red.
- [ ] **Step 173** [W] ~20m: Green. `SLO` gains `Notify *bool \`yaml:"notify,omitempty"\``; `renderRules` skips the tiers when it is false; `slo:good` becomes the expression of Step 170 (`fmt.Sprintf` with `oneLine(s.Good)` and `s.Name`). The generated file's header line two gains: "A planned stop (scripts/monitoring/planned-stop.sh) counts as good."
- [ ] **Step 174** [W] ~10m: `policy.yaml`:
  - `ci-develop`: add `notify: false` and above it the comment `# ADR-147 decision 4 (Stevie, 2026-11-15): out of the alert set until CI is fixed. Recorded, not notified.`
  - `wotan`: `description: The message bus answers on west` and a comment `# East's Wotan is off until S-E1-LIVE Evening 10B; plan 6 adds its target and says "both hosts" again.`
  - After `ci-develop`, a new SLO:
    ```yaml
      # ADR-147 decision 6: ClickHouse (audit logs, 455 days) and VictoriaMetrics
      # get nightly sets and a weekly restore drill, like the Well.
      - name: store-backups
        service: backups
        description: A restorable copy of ClickHouse and of VictoriaMetrics is under a day old
        objective: 99.0
        page: false
        good: >-
          (time() - max(unheaded_store_backup_last_success_timestamp_seconds{store="clickhouse"}) < bool 26 * 3600)
          * (time() - max(unheaded_store_backup_last_success_timestamp_seconds{store="victoria"}) < bool 26 * 3600)
          * (time() - max(unheaded_store_drill_last_success_timestamp_seconds{store="clickhouse"}) < bool 8 * 86400)
          * (time() - max(unheaded_store_drill_last_success_timestamp_seconds{store="victoria"}) < bool 8 * 86400)
        checks: >-
          journalctl -u unheaded-clickhouse-backup -u unheaded-victoria-backup -u unheaded-store-drill
          on west; cat /srv/backups/unheaded-{clickhouse,victoria}/LATEST.
        runbook: runbooks/data/store-backup-late.yaml
    ```
- [ ] **Step 175** [W] ~15m: `monitoring/prometheus/rules/backups.yml` and `tests/backups_test.yml`, modelled on `rules/well.yml` and its test: `StoreBackupLate` (> 26 h, for 45m, warning), `StoreBackupMissing` (> 50 h, for 45m, critical), `StoreDrillLate` (> 8 d, for 45m, warning), each `by (store)`, `urgency: none`, runbook `runbooks/data/store-backup-late.yaml`. Tests: late fires for `store="clickhouse"` at a timestamp 27 h old; quiet at 2 h.
- [ ] **Step 176** [W] ~15m: `runbooks/data/store-backup-late.yaml` (format of the others; `requires_sudo: true`): `systemctl status unheaded-clickhouse-backup unheaded-victoria-backup unheaded-store-drill`; `journalctl -u <unit> -n 40`; `sudo cat /srv/backups/unheaded-clickhouse/LATEST /srv/backups/unheaded-victoria/LATEST`; run one now with `sudo systemctl start <unit>`; the other-disk refusal and what it means after plan 4.
- [ ] **Step 177** [B] ~3m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && go test -race -count=1 ./cmd/alert-policy 2>&1 | tail -2 && go run ./cmd/alert-policy && git diff --stat monitoring/prometheus/rules/slo.yml monitoring/alertmanager/alertmanager.yml`
- [ ] **Step 178** [V] ~1m: `ok`; `slo.yml` changes (every `slo:good`, the store-backups rules, four `ci-develop` alerts gone).
- [ ] **Step 179** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-alerting && grep -c 'alert: ErrorBudgetBurn' monitoring/prometheus/rules/slo.yml`
- [ ] **Step 180** [V] ~1m: 32 (8 SLOs x 4 tiers): ci-develop's four gone, store-backups' four added.
- [ ] **Step 181** [B] ~2m: `scripts/runbook-manifest.sh --write && git diff --stat runbooks/MANIFEST.sha256` → one line added.
- [ ] **Step 182** [B] ~8m: RULES-GATE `alerting`, then GO-GATE `alerting ./cmd/alert-policy/...`.
- [ ] **Step 183** [V] ~1m: Green.
  - `slo_test.yml` fails on the marker case → Step 184.
- [ ] **Step 184** [D] ~10m: `time()` in promtool starts at 0; the mark `1e10` is "far future", `300` is "past at 10m". `> bool time()` on an aggregated vector keeps no labels, so `+` matches one-to-one; an error naming "many-to-many" means `good` kept a label: wrap it in `sum()` in the test fixture's terms, never in the generator. Two tries.
- [ ] **Step 185** [C] ~1m: **COMMIT** `cmd/alert-policy/main.go cmd/alert-policy/main_test.go observability/alerting/policy.yaml monitoring/prometheus/rules/slo.yml monitoring/prometheus/rules/backups.yml monitoring/prometheus/rules/tests/backups_test.yml monitoring/prometheus/rules/tests/slo_test.yml monitoring/alertmanager/alertmanager.yml runbooks/data/store-backup-late.yaml runbooks/MANIFEST.sha256`: "Steps 170-185: planned stops spend no budget; ci-develop recorded only; store-backups SLO".
- [ ] **Step 186** [W] ~5m: `wiki/Alerting.md`: a paragraph "Planned stops" (`planned-stop.sh`, `maintenance.sh --planned`, what each does), and `ci-develop`'s state.
- [ ] **Step 187** [C][V] ~1m: **PHASE 8 EXIT GATE**: RULES-GATE and GO-GATE green. **COMMIT** `wiki/Alerting.md` and `$PROG`. SSV-17, 18, 19: `in work (code)`.

---

## PHASE 9: BACKUPS FOR CLICKHOUSE AND VICTORIAMETRICS, IN CODE (Steps 188-218) [P]

**Goal**: Four root scripts (a backup and a drill per store), a drill wrapper, three units with timers, ClickHouse's `backups` config and mount in compose, and an end-to-end test on throwaway containers (run at the E-GATE). The Well's pattern throughout: sets on another physical disk, `LATEST`, `SHA256SUMS`, newest 14 kept, `.prom` on success only.
**Prerequisite**: Phase 0 gate.
**Time**: 5 h.
**Model**: Opus (root scripts, `docker exec` into the stores, a key). **Agent**: lane `backup`.

- [ ] **Step 188** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-SEE-AND-SURVIVE-backup -b sprint/s-see-and-survive-backup sprint/s-see-and-survive`
- [ ] **Step 189** [R] ~20m: `scripts/db-backup.sh` (the other-disk guard 64-94, retention, `LATEST`, the `.prom` writer), `scripts/well/restore-drill.sh`, `scripts/well/test-backup-restore.sh` (throwaway containers, `--network none`, the cleanup trap), `deploy/systemd/unheaded-well-{backup,drill}.{service,timer}`, `docker-compose.yml` 310-370 (victoria) and 467-520 (clickhouse), `config/clickhouse/schema/001-logs.sql`.
- [ ] **Step 190** [W] ~5m: `config/clickhouse/backups.xml`:
  ```xml
  <!-- SPDX-License-Identifier: GPL-3.0-or-later -->
  <!-- ADR-147 decision 6: BACKUP ... TO File('<name>') writes under /backups,
       which compose binds to the set root on another disk (the Well's rule).
       scripts/backup/clickhouse-backup.sh, clickhouse-drill.sh. -->
  <clickhouse>
      <backups>
          <allowed_path>/backups/</allowed_path>
      </backups>
  </clickhouse>
  ```
- [ ] **Step 191** [W] ~5m: `docker-compose.yml`, clickhouse `volumes:`, after the `system-logs.xml` line:
  ```yaml
        - ./config/clickhouse/backups.xml:/etc/clickhouse-server/config.d/backups.xml:ro
        # ADR-147 decision 6: backup sets, owned 16707, on a disk the data is not on.
        - ${CLICKHOUSE_BACKUP_DIR:-/srv/backups/unheaded-clickhouse}:/backups
  ```
- [ ] **Step 192** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-backup && docker compose -f docker-compose.yml config --quiet && echo compose-ok`
- [ ] **Step 193** [V] ~1m: `compose-ok` (a parse only: nothing starts).
- [ ] **Step 194** [W] ~10m: `scripts/backup/lib-disks.sh`: `disks_of_dev` and `disks_of_path` moved verbatim from `scripts/db-backup.sh:64-83` with a header naming the source; a function `refuse_shared_disk <data-path> <set-root> <tag>` that does what `db-backup.sh:84-94` does. `scripts/db-backup.sh` is **not** changed (the Well's installed copy is standalone; refactoring it is not this plan's).
- [ ] **Step 195** [W] ~40m: `scripts/backup/clickhouse-backup.sh` (root; `set -euo pipefail`; `umask 077`). Environment: `CH_CONTAINER` (default `unheaded-clickhouse`), `CH_BACKUP_DIR` (`/srv/backups/unheaded-clickhouse`), `CH_BACKUP_KEEP` (14), `CH_BACKUP_DB` (`unheaded_logs`), `CH_BACKUP_OTHER_DISK` (0; the unit sets 1), `STORE_METRICS_DIR` (empty: no `.prom`). Steps:
  1. The container mounts `CH_BACKUP_DIR` at `/backups` (`docker inspect` mounts), else exit 1 "ClickHouse has no /backups mount: recreate it from docker-compose.yml".
  2. With `CH_BACKUP_OTHER_DISK=1`, `refuse_shared_disk` on the `/var/lib/clickhouse` mount's source and the set root.
  3. `ch()` runs `docker exec -i "$CH_CONTAINER" sh -c 'exec clickhouse-client --user "${CLICKHOUSE_USER:-default}" --password "${CLICKHOUSE_PASSWORD:-}" --query "$1"' sh "<query>"`: the password stays in the container's environment and never reaches the host's argv.
  4. `name=ch-$(date -u +%Y%m%dT%H%M%SZ)`, `cutoff=$(( $(date +%s) - 300 ))`, `rows=$(ch "SELECT count() FROM $DB.logs WHERE timestamp < toDateTime64($cutoff, 3, 'UTC')")`.
  5. `ch "BACKUP DATABASE $DB TO File('$name')"`; the second field of its answer must be `BACKUP_CREATED`, else exit 1 with the answer.
  6. `"$ROOT/$name.SHA256SUMS"` from inside the set (`find . -type f -print0 | sort -z | xargs -0 sha256sum`), `"$ROOT/$name.meta.env"` with `NAME`, `DB`, `CUTOFF`, `ROWS`, `IMAGE` (`docker inspect -f '{{.Config.Image}}'`), `BYTES` (`du -sb` of the one set directory just written).
  7. `LATEST` (0644) through a temp file and `mv`.
  8. Keep the newest `KEEP` sets matching `^ch-[0-9]{8}T[0-9]{6}Z$`, removing each older set with its two side files.
  9. With `STORE_METRICS_DIR`: `store_backup_clickhouse.prom` with `unheaded_store_backup_last_success_timestamp_seconds{store="clickhouse"}`, `unheaded_store_backup_bytes{store="clickhouse"}`, `unheaded_store_backup_duration_seconds{store="clickhouse"}`, `# HELP`/`# TYPE` only, written atomically.
- [ ] **Step 196** [W] ~40m: `scripts/backup/clickhouse-drill.sh [set-name]` (default: `LATEST`). Environment as above plus `CH_DRILL_SCRATCH` (`/srv/backups/.drill-clickhouse`), `CH_UID` (16707), `CH_BACKUPS_XML` (`/usr/local/lib/unheaded/backup/clickhouse-backups.xml`). Steps: `sha256sum -c --quiet` against the side file (a mismatch exits 1, "set damaged"); source `meta.env` through a parser that accepts only `^[A-Z]+=[A-Za-z0-9_:./@-]*$` lines; the scratch directory must be absent or empty, made 0700 and owned `CH_UID`; `docker run -d --name ch-drill-$$ --network none --user $CH_UID:$CH_UID --read-only --memory 2g --tmpfs /tmp:size=64m --tmpfs /var/log/clickhouse-server:uid=$CH_UID,gid=$CH_UID,size=16m --tmpfs /etc/clickhouse-server/users.d:uid=$CH_UID,gid=$CH_UID,size=1m -v "$scratch":/var/lib/clickhouse -v "$ROOT":/backups:ro -v "$CH_BACKUPS_XML":/etc/clickhouse-server/config.d/backups.xml:ro "$IMAGE"`; wait up to 90 s for `SELECT 1`; `RESTORE DATABASE $DB FROM File('$NAME')` must answer `RESTORED`; `count()` before `CUTOFF` must equal `ROWS`, else exit 1 naming both numbers. A trap removes the container and the scratch directory on every exit. On success, `store_drill_clickhouse.prom` (`unheaded_store_drill_last_success_timestamp_seconds{store="clickhouse"}`, `unheaded_store_drill_duration_seconds{store="clickhouse"}`).
- [ ] **Step 197** [C] ~1m: **COMMIT** `config/clickhouse/backups.xml docker-compose.yml scripts/backup/lib-disks.sh scripts/backup/clickhouse-backup.sh scripts/backup/clickhouse-drill.sh`: "Steps 188-197: ClickHouse backup and restore drill (code)".
- [ ] **Step 198** [W] ~40m: `scripts/backup/victoria-backup.sh` (root). Environment: `VM_CONTAINER` (`unheaded-victoria`), `VM_URL` (`http://127.0.0.1:8428`), `VM_BACKUP_DIR` (`/srv/backups/unheaded-victoria`), `VM_BACKUP_KEEP` (14), `VM_BACKUP_OTHER_DISK` (0), `STORE_METRICS_DIR`. Steps:
  1. From `docker inspect` mounts: the source of `/victoria-data` (`$DATA`) and of `/victoria-auth` (`$KEY=$src/admin.key`, which must exist; never printed).
  2. Other-disk guard on `$DATA`.
  3. `cutoff=$(( $(date +%s) - 120 ))`; `series` = the value of `count({__name__!=""})` at `time=$cutoff` (`curl -fsS -G "$VM_URL/api/v1/query" --data-urlencode ... | jq -r '.data.result[0].value[1]'`).
  4. `vmcurl()` passes the key through `curl --config -` on stdin (`printf 'data-urlencode = "authKey=%s"\n' "$(<"$KEY")"`), so it never appears in argv; `snap=$(vmcurl -G "$VM_URL/snapshot/create" | jq -r 'select(.status=="ok").snapshot')`, empty → exit 1. A trap calls `vmcurl -G "$VM_URL/snapshot/delete" --data-urlencode "snapshot=$snap"` on every exit after this point.
  5. `name=vm-$(date -u +%Y%m%dT%H%M%SZ)`; `mkdir -m 0700 "$ROOT/$name"`; `cp -rL "$DATA/snapshots/$snap/." "$ROOT/$name/"` (a snapshot holds symlinks; `-L` copies the data).
  6. Side files, `LATEST`, retention and `store_backup_victoria.prom` as Step 195 (with `SERIES` in place of `ROWS`, plus `SNAPSHOT`).
- [ ] **Step 199** [W] ~30m: `scripts/backup/victoria-drill.sh [set-name]`. `VM_DRILL_SCRATCH` (`/srv/backups/.drill-victoria`), `VM_UID` (16703), `VM_DRILL_PORT` (26428). Checksums; meta parser as Step 196; `cp -a "$ROOT/$NAME/." "$scratch/"`, chown `VM_UID` when root; `docker run -d --name vm-drill-$$ --user $VM_UID:$VM_UID --read-only --memory 1g -p 127.0.0.1:$VM_DRILL_PORT:8428 -v "$scratch":/victoria-data "$IMAGE" -storageDataPath=/victoria-data -retentionPeriod=100y -httpListenAddr=:8428` (no scrape config: it fetches nothing); wait up to 60 s for `/health`; `count({__name__!=""})` at `time=$CUTOFF` must equal `SERIES`; trap cleanup; `store_drill_victoria.prom` on success.
- [ ] **Step 200** [W] ~10m: `scripts/backup/store-drill.sh`: runs `clickhouse-drill.sh` and `victoria-drill.sh` one after the other, both always, and exits 1 if either failed (so one broken store never hides the other's result).
- [ ] **Step 201** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-backup && shellcheck scripts/backup/{lib-disks,clickhouse-backup,clickhouse-drill,victoria-backup,victoria-drill,store-drill}.sh 2>&1 | tail -5`
- [ ] **Step 202** [V] ~1m: Clean.
- [ ] **Step 203** [C] ~1m: **COMMIT** `scripts/backup/victoria-backup.sh scripts/backup/victoria-drill.sh scripts/backup/store-drill.sh`: "Steps 198-203: VictoriaMetrics backup and restore drill (code)".
- [ ] **Step 204** [W] ~45m: `scripts/backup/test-store-backups.sh`, modelled on `scripts/well/test-backup-restore.sh`, never touching the live stores. Images read from `docker-compose.yml` (the deployed digests). As the normal user (`CH_UID` and `VM_UID` set to `id -u`; `*_OTHER_DISK=0`):
  1. ClickHouse source: a throwaway container (`--network none`, data on tmpfs, `/backups` bound to a temp dir, `backups.xml` from the tree), schema `001-logs.sql` applied, 1,000 rows inserted with timestamps an hour old.
  2. `clickhouse-backup.sh` with `CH_CONTAINER` set: one set, `LATEST`, side files, `ROWS=1000`.
  3. `clickhouse-drill.sh`: passes.
  4. A flipped byte in one set file → the drill exits 1 with "set damaged".
  5. `ROWS=999` in a copy's meta (its checksums recomputed) → exit 1 naming 999 and 1000.
  6. Three backups with `CH_BACKUP_KEEP=2` → two sets.
  7. VictoriaMetrics source: a throwaway container on a random loopback port, `-snapshotAuthKey=file:///victoria-auth/admin.key` with a temp key, 50 series imported through `/api/v1/import/prometheus` two minutes in the past; `victoria-backup.sh`, then `victoria-drill.sh` with a second free port: passes; a flipped byte: refused.
  8. With `CH_BACKUP_OTHER_DISK=1` and the set root on the same disk as the data: exit 1 with "refusing".
  9. No `ch-drill-*` or `vm-drill-*` container is left (`docker ps -a`).
  Not run now: the E-GATE runs it (Step 260).
- [ ] **Step 205** [W] ~20m: Units, each with the Well backup unit's header form (what, why root, install lines) and hardening (`ProtectSystem=strict`, `NoNewPrivileges`, `PrivateTmp`, `ProtectHome`, `ProtectKernel*`, `RestrictNamespaces`, `MemoryDenyWriteExecute`, `SystemCallFilter=@system-service`, `Nice=10`, `IOSchedulingClass=idle`, `UMask=0077`):
  - `deploy/systemd/unheaded-clickhouse-backup.service`: `Environment=CH_BACKUP_DIR=/srv/backups/unheaded-clickhouse CH_BACKUP_KEEP=14 CH_BACKUP_OTHER_DISK=1 STORE_METRICS_DIR=/var/lib/unheaded/textfile`, `ExecStart=/usr/local/lib/unheaded/backup/clickhouse-backup.sh`, `ReadWritePaths=/srv/backups/unheaded-clickhouse /var/lib/unheaded/textfile`, `RestrictAddressFamilies=AF_UNIX`, `CapabilityBoundingSet=CAP_DAC_OVERRIDE CAP_DAC_READ_SEARCH CAP_CHOWN CAP_FOWNER`, `TimeoutStartSec=30min`, `After=docker.service unheaded-well-backup.service`, `Requires=docker.service`. Timer `OnCalendar=*-*-* 04:15:00`, `Persistent=true`, `RandomizedDelaySec=5min`.
  - `deploy/systemd/unheaded-victoria-backup.service`: the same shape; `VM_*` settings; `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`, `IPAddressDeny=any`, `IPAddressAllow=localhost`. Timer 04:45.
  - `deploy/systemd/unheaded-store-drill.service`: `ExecStart=/usr/local/lib/unheaded/backup/store-drill.sh`, `ReadWritePaths=/srv/backups /var/lib/unheaded/textfile`, address families as the VictoriaMetrics unit, `TimeoutStartSec=45min`. Timer `OnCalendar=Sun *-*-* 05:40:00` (after the Well's drill at 05:00), `Persistent=true`.
  Install lines in each header: `sudo install -d -m 0755 /usr/local/lib/unheaded/backup`; `sudo install -m 0755 scripts/backup/{lib-disks,clickhouse-backup,clickhouse-drill,victoria-backup,victoria-drill,store-drill}.sh /usr/local/lib/unheaded/backup/`; `sudo install -m 0644 config/clickhouse/backups.xml /usr/local/lib/unheaded/backup/clickhouse-backups.xml`; `sudo install -d -m 0700 -o 16707 -g 16707 /srv/backups/unheaded-clickhouse`; `sudo install -d -m 0700 /srv/backups/unheaded-victoria`; copy the units; `daemon-reload`; `enable --now` the three timers.
- [ ] **Step 206** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-backup && systemd-analyze verify deploy/systemd/unheaded-{clickhouse-backup,victoria-backup,store-drill}.{service,timer} 2>&1 | grep -v -E 'clickhouse-backup.sh|victoria-backup.sh|store-drill.sh' | head -5; scripts/check-unit-binaries.sh 2>&1 | tail -1`
- [ ] **Step 207** [V] ~1m: Only "not found" for the not-yet-installed scripts; gate `PASS` (if it refuses a script path, use the Well backup's exemption, as Step 108).
- [ ] **Step 208** [W] ~5m: `config/logging/sources.yaml`: entries for the three units, the way the Well's backup units have them.
- [ ] **Step 209** [W] ~5m: `.github/workflows/security.yml`: a step `run: ./scripts/backup/test-store-backups.sh` beside `./scripts/well/test-backup-restore.sh` (line 634), same job.
- [ ] **Step 210** [B] ~2m: `bash -n scripts/backup/test-store-backups.sh && shellcheck scripts/backup/test-store-backups.sh 2>&1 | tail -2; ls scripts/check-log-sources.sh >/dev/null 2>&1 && scripts/check-log-sources.sh 2>&1 | tail -1`
- [ ] **Step 211** [V] ~1m: Clean; `PASS`.
- [ ] **Step 212** [C] ~1m: **COMMIT** the test, the six unit files, `config/logging/sources.yaml`, `.github/workflows/security.yml`: "Steps 204-212: store backup units, timers and an end-to-end test".
- [ ] **Step 213** [W] ~10m: `wiki/Well-Backups.md`: a section "ClickHouse and VictoriaMetrics" (Plan 4 reads it): the table of where copies live (`/srv/backups/unheaded-clickhouse/<ch-…>`, `/srv/backups/unheaded-victoria/<vm-…>`, NVMe, newest 14; offsite per SS5), the schedule (04:15, 04:45, Sunday 05:40), what a set holds, the drill, the commands (`sudo systemctl start unheaded-clickhouse-backup.service`, …), the alerts and SLO, and "After S-WEST-GROUND moves the database volumes to the NVMe, the sets must leave it: change `CH_BACKUP_DIR`/`VM_BACKUP_DIR` and `CLICKHOUSE_BACKUP_DIR`; the units refuse to run until then."
- [ ] **Step 214** [W] ~5m: `docs/battle-plans/progress/S-SEE-AND-SURVIVE-progress.md` in this lane is not edited (one writer: the coordinator). Instead list in the commit message body the paths a later step must read.
- [ ] **Step 215** [B] ~1m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-backup && git status --short`
- [ ] **Step 216** [V] ~1m: Only `wiki/Well-Backups.md` changed.
- [ ] **Step 217** [C] ~1m: **COMMIT** `wiki/Well-Backups.md`: "Step 213: the store backups in the wiki".
- [ ] **Step 218** [V] ~1m: **PHASE 9 EXIT GATE**: scripts, units, test and wiki committed; nothing run against a live store. SSV-21, SSV-22: `in work (code)`.

---

## PHASE 10: A SMOKE CHECK THAT CAN PASS (Steps 219-228) [P]

**Goal**: `scripts/qa-smoke.sh` probes TLS listeners over https with the deployment CA, reads what the services answer today, and scores nothing that cannot pass without a login.
**Prerequisite**: Phase 0 gate.
**Time**: 45 min.
**Model**: Sonnet. **Agent**: lane `smoke`.

Measured 2026-11-15 with `--cacert configs/certs/dev/ca.pem`: `https://localhost:20000/` 200, `:20001/` 200, `http://localhost:8428/` 200, `:8123/` 200, `:3001/` 302, `https://localhost:19100/health` 200, `:19006/health` 200, `:20002/health` 200, `:20002/wiki/` **401** (the wiki needs a login since 2026-11-11), HEAD 200 and BREW 405 on the probes, Akira **13/13** OK.

- [ ] **Step 219** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-SEE-AND-SURVIVE-smoke -b sprint/s-see-and-survive-smoke sprint/s-see-and-survive`
- [ ] **Step 220** [W] ~25m: `scripts/qa-smoke.sh`, these changes and no others (plan 2 edits the same service lists; keep the diff small):
  1. After `set -uo pipefail`: `R="$(cd "$(dirname "$0")/.." && pwd)"; cd "$R"`, `CA="${QA_CA:-$R/configs/certs/dev/ca.pem}"`, and `DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)`. Every `docker` call becomes `"${DOCKER[@]}"`.
  2. `http()` and `http_method()` pass `--cacert "$CA"` to curl.
  3. `http/dashboard` and `http/kanban`: `https://`. The `health/*` and `probe-methods/*` loops: `https://localhost:${sp##*:}/health`. Akira's status URL: `https://`.
  4. Akira: `check "akira/consensus-ok" "all" "<all if ok == total and total > 0, else ok/total>"` in place of the fixed `10/10`.
  5. `wiki/http`: `https://localhost:20002/health`, 200. `wiki/pages`: expected `401`, comment "the wiki needs a login since 2026-11-11; 401 proves the gate is up".
  6. `flows/advancing`: moved into the unscored INFO block with the comment "the dashboard API needs a login since the BFF (pkg/bff); not scorable without a credential, and the script holds none".
  7. The header gains: "TLS listeners are probed over https with the deployment CA (QA_CA). The Well check opens a psql session."
- [ ] **Step 221** [B] ~1m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-smoke && bash -n scripts/qa-smoke.sh && shellcheck scripts/qa-smoke.sh 2>&1 | tail -3`
- [ ] **Step 222** [V] ~1m: Parses; no new shellcheck finding (compare with `shellcheck` on develop's copy).
- [ ] **Step 223** [B] ~1m: No `http://localhost:` remains for a TLS port: `grep -n 'http://localhost:\(19[0-9]\{3\}\|18000\|2000[012]\)' scripts/qa-smoke.sh`
- [ ] **Step 224** [V] ~1m: No output.
- [ ] **Step 225** [C] ~1m: **COMMIT** `scripts/qa-smoke.sh`: "Steps 219-225: qa-smoke probes TLS over https and scores only what can pass". Not run now: it opens a psql session on the Well (Step 263 takes the before, Step 367 the after).
- [ ] **Step 226** [W] ~3m: `$PROG` is not in this lane; note the run steps in the commit body.
- [ ] **Step 227** [B] ~1m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE-smoke && git status --short`
- [ ] **Step 228** [V] ~1m: **PHASE 10 EXIT GATE**: clean tree, one commit. SSV-23: `in work (code)`.

---

## PHASE 11: VOR'S ACCESS LOG AND NGINX'S REQUEST TIME (Steps 229-246) [P]

**Goal**: vor writes one JSON line per request (time, method, URI, status, bytes, microseconds; no client address) to its journal. A `log_format unheaded_timed` with request time and no client address exists for nginx; west's one site uses it.
**Prerequisite**: Phase 0 gate. SS7 answered.
**Time**: 1.5 h.
**Model**: Sonnet. **Agent**: lane `vor` (vor's own repository, and the nginx files in this one).

- [ ] **Step 229** [B] ~1m: `git -C /home/govan/tmp/projects/vor status --short | head -3; git -C /home/govan/tmp/projects/vor worktree add /home/govan/tmp/wt/vor-access-log -b access-log main && git -C /home/govan/tmp/wt/vor-access-log log --oneline -1`
  - vor's checkout has local changes → they are Stevie's: the worktree is made from `main` regardless; never touch the checkout.
- [ ] **Step 230** [R] ~10m: `/home/govan/tmp/projects/vor/CLAUDE.md` (commit and test rules there), `cmd/vor/main.go` 1047-1110 (`doServe`, `newMux`), `cmd/vor/search_api_test.go` (test style), `Makefile` `build`.
- [ ] **Step 231** [W] ~20m: Red: `cmd/vor/accesslog_test.go`:
  - `TestAccessLogLine`: a handler writing `{"ok":true}`; `GET /api/search?q=bgp` → one line, valid JSON, `method` `GET`, `uri` `/api/search?q=bgp`, `status` 200, `bytes` 11, `dur_us` ≥ 0, `ts` parses as RFC 3339.
  - `TestAccessLogStatus`: a handler calling `WriteHeader(404)` → `status` 404.
  - `TestAccessLogTruncatesURI`: a 5,000-byte query → `uri` is 2,048 bytes and `uri_truncated` true.
  - `TestAccessLogMethodFolded`: method `BREW` → `OTHER`.
  - `TestAccessLogNoClientAddress`: with `r.RemoteAddr` set, the line has no field holding it.
  - `TestAccessLogConcurrent`: 100 goroutines, 100 lines, each valid JSON.
- [ ] **Step 232** [B] ~1m: `cd /home/govan/tmp/wt/vor-access-log && go test -count=1 ./cmd/vor -run AccessLog 2>&1 | tail -3` → red (`undefined: accessLog`).
- [ ] **Step 233** [W] ~25m: Green: `cmd/vor/accesslog.go`: `func accessLog(next http.Handler, w io.Writer, now func() time.Time) http.Handler`; a `ResponseWriter` wrapper counting bytes and keeping the status (200 when unset); methods outside GET, HEAD, POST, PUT, DELETE, OPTIONS, PATCH fold to `OTHER`; one `json.Marshal` per request plus `\n`, written under a mutex. In `doServe`: `http.ListenAndServe(addr, accessLog(mux, os.Stderr, time.Now))` and one banner line "access log: one JSON line per request on stderr (ADR-147 decision 23 in the unheaded repository)".
- [ ] **Step 234** [B] ~2m: `cd /home/govan/tmp/wt/vor-access-log && go vet ./cmd/vor && go test -race -count=1 ./cmd/vor 2>&1 | tail -3`
- [ ] **Step 235** [V] ~1m: `ok`.
- [ ] **Step 236** [B] ~2m: One live line on a spare loopback port, as the normal user, stopped by its own pid: `cd /home/govan/tmp/wt/vor-access-log && make build >/dev/null && (./vor serve 26110 2>/home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/vor-stderr.log & echo $! > /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/vor.pid) && sleep 8 && curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:26110/api/health; kill "$(cat /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/vor.pid)"; grep -c '"uri":"/api/health"' /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/vor-stderr.log`
- [ ] **Step 237** [V] ~1m: `200` and `1`. (`vor serve` binds 127.0.0.1 by default: `cmd/vor/main.go:332-337`, read 2026-11-15.)
- [ ] **Step 238** [C] ~1m: **COMMIT** in the vor worktree (signed; message per vor's CLAUDE.md, for example `feat(serve): one JSON access-log line per request, no client address`), files `cmd/vor/accesslog.go cmd/vor/accesslog_test.go cmd/vor/main.go`.
- [ ] **Step 239** [W] ~5m: This repository's side of the lane: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-SEE-AND-SURVIVE-vor -b sprint/s-see-and-survive-vor sprint/s-see-and-survive`. Create `deploy/nginx/conf.d/unheaded-timed.conf`:
  ```nginx
  # SPDX-License-Identifier: GPL-3.0-or-later
  # Copyright (c) 2026 Stevie Bellis.
  # ADR-147 decision 23: every request's time, for baselines and for replaying
  # a recorded day. No client address: a recorded day needs no scrubbing.
  # Installed as /etc/nginx/conf.d/unheaded-timed.conf (inside http {}).
  log_format unheaded_timed '$time_iso8601 $request_method "$request_uri" $status $body_bytes_sent rt=$request_time urt=$upstream_response_time host=$host';
  ```
- [ ] **Step 240** [W] ~2m: `deploy/nginx/unheaded-apt.conf` line 36: `access_log /var/log/nginx/unheaded-apt.access.log unheaded_timed;` with the comment `# format: /etc/nginx/conf.d/unheaded-timed.conf (ADR-147 decision 23)` above it.
- [ ] **Step 241** [W] ~3m: `/home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/nginx-test.conf` (scratch, not committed):
  ```nginx
  error_log stderr;
  pid /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/nginx-test.pid;
  events {}
  http {
      include /home/govan/tmp/wt/S-SEE-AND-SURVIVE-vor/deploy/nginx/conf.d/unheaded-timed.conf;
      access_log /dev/null unheaded_timed;
  }
  ```
- [ ] **Step 242** [B] ~1m: `nginx -t -e stderr -c /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/nginx-test.conf 2>&1 | tail -2`
- [ ] **Step 243** [V] ~1m: `syntax is ok` (a warning about the pid file's directory as a normal user is fine; "test is successful" may need root and is checked live at Step 365).
- [ ] **Step 244** [W] ~5m: SS7 A: under "For Stevie" in `$PROG` (the coordinator writes it at Step 269): the two lines for orion (the `log_format` inside `http {}`, and `unheaded_timed` on the blog's and site's `access_log`), and that this format holds no client address. SS7 B: "orion untouched; request time measured on the cluster only".
- [ ] **Step 245** [C] ~1m: **COMMIT** `deploy/nginx/conf.d/unheaded-timed.conf deploy/nginx/unheaded-apt.conf`: "Steps 239-245: nginx logs request time, no client address".
- [ ] **Step 246** [V] ~1m: **PHASE 11 EXIT GATE**: vor green with one live line; nginx syntax ok. SSV-24, SSV-25: `in work (code)`.

---

## PHASE 12: THE E-GATE, THE CONTAINER TESTS, THE MERGE (Steps 247-270)

**Goal**: The evidence week is over; the tests that need throwaway containers pass; the lanes are one branch; `develop` holds it.
**Prerequisite**: Phases 1 to 11 gates (a `[STUCK]` lane merges without its stuck part, or not at all).
**Time**: 1.5 h.
**Model**: Opus. **Agent**: Coordinator.

- [ ] **Step 247** [V] ~2m: **The E-GATE.** `date -u +%FT%TZ; grep -n -i 'log store week' /home/govan/tmp/unheaded/docs/battle-plans/progress/S-E1-LIVE-progress.md | tail -3`. Open only if that file records the week's end (S-E1-LIVE Step 284 closed) or Stevie says so in this session. A date alone does not open it. Closed → stop here; Phases 12 to 18 wait; write "E-GATE closed <time>" in `$PROG`.
- [ ] **Step 248** [W] ~1m: "E-GATE open <time>, evidence: <line or Stevie's words>" in `$PROG`.
- [ ] **Step 249** [B] ~5m: Merge the lanes, one at a time, in this order: alerting, buckets, huginn, state, backup, smoke, vor.
  ```bash
  cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE && for l in alerting buckets huginn state backup smoke vor; do git merge --no-ff -S sprint/s-see-and-survive-$l -m "Merge lane $l into sprint/s-see-and-survive" || { echo "CONFLICT in $l"; break; }; done; git log --oneline -9
  ```
- [ ] **Step 250** [V] ~2m: No conflict. The only file two lanes touch is `config/logging/sources.yaml` (state, backup) and `wiki/Systemd-Units.md`: keep both sides' entries. Anything else → `[STUCK]`.
- [ ] **Step 251** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE && git merge --no-ff -S develop -m "Merge develop into sprint/s-see-and-survive"`
- [ ] **Step 252** [V] ~5m: Conflicts allowed by the Busboy notes only: `policy.yaml` (keep both sides' entries), `scripts/qa-smoke.sh` (plan 2's five removals and this plan's https), `cmd/alert-mailer` (plan 2's publisher and this plan's pipeline: both), ADR index rows (both, in number order). Generated files are regenerated, never hand-merged (Step 253). Anything else → `[STUCK]`.
- [ ] **Step 253** [B] ~2m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE && go run ./cmd/alert-policy && scripts/runbook-manifest.sh --write && git status --short`
- [ ] **Step 254** [B] ~10m: GO-GATE on the sprint tree: `./cmd/alert-mailer/... ./cmd/alert-policy/... ./cmd/huginn/... ./pkg/metrics/... ./pkg/httputil/... ./services/wotan/... ./cmd/dashboard-backend/...` (run from `/home/govan/tmp/wt/S-SEE-AND-SURVIVE`).
- [ ] **Step 255** [B] ~8m: RULES-GATE on the sprint tree.
- [ ] **Step 256** [B] ~3m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE && bash scripts/monitoring/test-host-state-textfile.sh 2>&1 | tail -1 && bash scripts/monitoring/test-planned-stop.sh 2>&1 | tail -1 && scripts/check-unit-start-limits.sh 2>&1 | tail -1`
- [ ] **Step 257** [B] ~10m: The alert path on throwaway containers (now allowed): `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE && scripts/monitoring/test-alert-path.sh 2>&1 | tail -14`
- [ ] **Step 258** [V] ~1m: Every case `PASS`, cases 6 (run day) and 7 (pipeline route) included.
- [ ] **Step 259** [D] ~10m: A failing case prints its expected and got. Fix it in the lane's files on the sprint branch, commit, rerun Step 257. Two tries, then `[STUCK]` (Phase 15 is then `[BLOCKED]`).
- [ ] **Step 260** [B] ~10m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE && scripts/backup/test-store-backups.sh 2>&1 | tail -15`
- [ ] **Step 261** [V] ~1m: 9 of 9.
- [ ] **Step 262** [D] ~15m: Likely: the answer words differ by version (`BACKUP_CREATED`, `RESTORED`: read what the pinned ClickHouse prints and match it exactly); a snapshot path (`$DATA/snapshots/<name>` holds `data` and `indexdb`: `ls` it in the test); the drill container not ready in 90 s (raise to 180 s, log it). Two tries, then `[STUCK]` (Phase 13 is then `[BLOCKED]`).
- [ ] **Step 263** [B] ~3m: SSV-23 **before**, develop's own copy in the main checkout (unchanged until Step 267): `cd /home/govan/tmp/unheaded && scripts/qa-smoke.sh 2>&1 | tail -6`. Log "passed N, failed M" and the failing names.
- [ ] **Step 264** [B] ~3m: The new gate fails when provoked, on the merged tree: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE && git status --short | wc -l && scripts/check-gates-can-fail.sh --only check-unit-start-limits 2>&1 | tail -2`
- [ ] **Step 265** [V] ~1m: `0` and `[PASS]` (SS3 B: the gate does not exist; skip).
- [ ] **Step 266** [C] ~1m: **COMMIT** `$PROG` (and any generated file Step 253 changed, by name): "Steps 247-266: E-GATE open; container tests green; lanes merged".
- [ ] **Step 267** [B] ~2m: Merge to develop.
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && git merge --no-ff -S sprint/s-see-and-survive -m "Merge sprint/s-see-and-survive: S-SEE-AND-SURVIVE code (Phases 1-11)" || { git merge --abort; git merge --no-ff --no-gpg-sign sprint/s-see-and-survive -m "Merge sprint/s-see-and-survive: S-SEE-AND-SURVIVE code (Phases 1-11)"; }; git log --oneline -1
  ```
- [ ] **Step 268** [V] ~1m: A merge commit. From now the bind-mounted files on west are the new ones: Prometheus and Alertmanager read them at the next HUP (Step 314), the mailer at its next start (Step 320). One of them restarting by itself loads tested files: acceptable, and logged if seen.
- [ ] **Step 269** [W] ~3m: `$PROG` on develop: the merge commit, an unsigned merge if there was one, and SS7's text for Stevie (Step 244).
- [ ] **Step 270** [C][V] ~1m: **PHASE 12 EXIT GATE**. **COMMIT** `$PROG` on develop (named file).

---

## PHASE 13: LIVE 1: THE STORES GET BACKUPS AND A DRILL (Steps 271-296)

**Goal**: ClickHouse runs with its `/backups` mount; one set of each store exists on the NVMe; the drill restored both and the counts matched; the timers run nightly and weekly.
**Prerequisite**: Phase 12 gate. No other plan restarts ClickHouse or Vector in the same hour (read the last lines of `docs/battle-plans/progress/S-WASTE-OUT-progress.md`).
**Time**: 1.5 h.
**Model**: Opus. **Agent**: Coordinator, sudo.

- [ ] **Step 271** [B] ~2m: ClickHouse's size and the client identity (a short SQL read, allowed after the E-GATE):
  ```bash
  sudo docker exec unheaded-clickhouse sh -c 'exec clickhouse-client --user "${CLICKHOUSE_USER:-default}" --password "${CLICKHOUSE_PASSWORD:-}" --multiquery --query "SELECT currentUser(); SELECT database, formatReadableSize(sum(bytes_on_disk)), sum(rows) FROM system.parts WHERE active GROUP BY database ORDER BY 1; SHOW GRANTS"' 2>&1 | grep -v -i password
  ```
- [ ] **Step 272** [V] ~1m: A user name, the `unheaded_logs` size (expected 0.4 to 0.8 GiB), and a grant with `ALL` or `BACKUP` on `*.*`. Log size and user (not the grant text).
  - No such grant → Step 273.
- [ ] **Step 273** [D] ~5m: Granting is a ClickHouse configuration change: write it under "For Stevie" with the grant line needed, mark Steps 276-292 `[BLOCKED]`, go to Phase 14.
- [ ] **Step 274** [S][B] ~2m: Install (the lines in `deploy/systemd/unheaded-clickhouse-backup.service`'s header):
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -d -m 0755 /usr/local/lib/unheaded/backup && sudo install -m 0755 scripts/backup/{lib-disks,clickhouse-backup,clickhouse-drill,victoria-backup,victoria-drill,store-drill}.sh /usr/local/lib/unheaded/backup/ && sudo install -m 0644 config/clickhouse/backups.xml /usr/local/lib/unheaded/backup/clickhouse-backups.xml && sudo install -d -m 0700 -o 16707 -g 16707 /srv/backups/unheaded-clickhouse && sudo install -d -m 0700 /srv/backups/unheaded-victoria && sudo cp deploy/systemd/unheaded-{clickhouse-backup,victoria-backup,store-drill}.{service,timer} /etc/systemd/system/ && sudo systemctl daemon-reload
  ```
- [ ] **Step 275** [V] ~1m: `sudo stat -c '%U:%G %a %n' /srv/backups/unheaded-clickhouse /srv/backups/unheaded-victoria` → `16707:16707 700` (or the uid's name) and `root:root 700`.
- [ ] **Step 276** [S][B] ~3m: Recreate ClickHouse with its new mounts (Vector's buffer holds the journal rows meanwhile; restart policy stays as the override sets it):
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose up -d --no-deps clickhouse && date -u +%FT%TZ
  ```
- [ ] **Step 277** [V] ~3m: Within 90 s `sudo docker ps --filter name=unheaded-clickhouse --format '{{.Status}}'` says `healthy`; `sudo docker inspect -f '{{range .Mounts}}{{println .Destination}}{{end}}' unheaded-clickhouse | grep -c '^/backups$'` is 1; five minutes later `source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; q 'sum(rate(vector_component_sent_events_total{component_id="clickhouse"}[5m]))'` is above 0. Log the recreate time in `$PROG` and in `docs/battle-plans/progress/S-E1-LIVE-progress.md` ("ClickHouse recreated <time>, S-SEE-AND-SURVIVE Step 276; the log store week had ended").
  - Not healthy in 3 min → Step 278.
- [ ] **Step 278** [D] ~10m: `sudo docker logs --tail 40 unheaded-clickhouse`. A refused `backups.xml` names the element. **Rollback**: a signed revert of Step 191's two compose lines on develop (`git revert --no-edit <commit>` touches only `docker-compose.yml` if Step 191 was its own commit; else edit the two lines out and commit), then Step 276 again. Then `[STUCK]` with the log lines.
- [ ] **Step 279** [S][B] ~5m: The first ClickHouse set: `sudo systemctl start unheaded-clickhouse-backup.service; systemctl status --no-pager unheaded-clickhouse-backup | tail -4; sudo cat /srv/backups/unheaded-clickhouse/LATEST; journalctl -u unheaded-clickhouse-backup -n 15 --no-pager`
- [ ] **Step 280** [V] ~1m: `status=0/SUCCESS`; `LATEST` names `ch-<UTC>`; the journal shows rows, bytes and duration. Log them.
  - "refusing ... share disk" → Step 281.
- [ ] **Step 281** [D] ~5m: Read the two disk lists in the message. On 2026-11-15 `/srv` is the NVMe and the data is on `sdb`. If plan 4 has already moved the volumes, the sets need another disk: `[STEVIE]` question (which root), Steps 282-292 `[BLOCKED]`.
- [ ] **Step 282** [S][B] ~5m: The first VictoriaMetrics set: `sudo systemctl start unheaded-victoria-backup.service; systemctl status --no-pager unheaded-victoria-backup | tail -4; sudo cat /srv/backups/unheaded-victoria/LATEST; journalctl -u unheaded-victoria-backup -n 15 --no-pager`
- [ ] **Step 283** [V] ~1m: Success, `vm-<UTC>`, series count, bytes, duration, and "snapshot deleted". Log them.
- [ ] **Step 284** [S][B] ~15m: The drill: `sudo systemctl start unheaded-store-drill.service; systemctl status --no-pager unheaded-store-drill | tail -4; journalctl -u unheaded-store-drill -n 30 --no-pager`
- [ ] **Step 285** [V] ~1m: Success; for each store "restored N rows (or series) before the cutoff, the set says N". Log both durations.
  - A count differs → Step 286.
- [ ] **Step 286** [D] ~15m: ClickHouse: a `ReplacingMergeTree` merge between the count and the `BACKUP` removes duplicate rows Vector resent, so the set can hold fewer than `ROWS`. Run Step 279 and Step 284 once more. Still off by a few → change both scripts to count `uniqExact(entry)` before the cutoff (stable under merges), add the case to `test-store-backups.sh`, commit, merge, reinstall (Step 274's install line for the scripts only). VictoriaMetrics: a sample that arrived after the count with a timestamp before the cutoff; move the cutoff to 5 minutes before the snapshot and retry. Two tries, then `[STUCK]`.
- [ ] **Step 287** [B] ~1m: `source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; pq 'unheaded_store_backup_last_success_timestamp_seconds'; pq 'unheaded_store_drill_last_success_timestamp_seconds'; pq 'huginn_textfile_rejected_lines{file=~"store_.*"}'`
- [ ] **Step 288** [V] ~1m: Two and two series; rejected lines 0.
- [ ] **Step 289** [S][B] ~1m: `sudo systemctl enable --now unheaded-clickhouse-backup.timer unheaded-victoria-backup.timer unheaded-store-drill.timer && systemctl list-timers --no-pager | grep -c -E 'clickhouse-backup|victoria-backup|store-drill'`
- [ ] **Step 290** [V] ~1m: `3`.
- [ ] **Step 291** [W] ~5m: SSV-21 and SSV-22 after in `$PROG`: "clickhouse backup <bytes> in <s>; restore drill passed, <rows> rows, <s>" and "victoria backup <bytes> in <s>; restore drill passed, <series> series, <s>". `wiki/Well-Backups.md`: the measured sizes and times in its new section.
- [ ] **Step 292** [C] ~1m: **COMMIT** on develop `$PROG wiki/Well-Backups.md docs/battle-plans/progress/S-E1-LIVE-progress.md`: "Steps 271-292: ClickHouse and VictoriaMetrics backed up and restored once".
- [ ] **Step 293** [STEVIE] ~2m: SS5 **B** only: with Stevie, the offsite copy to east (east's `well-backup-copy` chroot gets `clickhouse/` and `victoria/`; the pattern is `scripts/well/offsite-copy.sh`). Under A: "offsite with plan 6" in SSV-21/22's status. Under C: "local only" in the wiki section.
- [ ] **Step 294** [B] ~1m: Rollback copy of nothing is needed: the sets and units add, they replace nothing. Record the undo in `$PROG`: `sudo systemctl disable --now` the three timers; the ClickHouse mount stays harmless.
- [ ] **Step 295** [B] ~1m: `git -C /home/govan/tmp/unheaded status --short | head`
- [ ] **Step 296** [V] ~1m: **PHASE 13 EXIT GATE**: one set per store, a passed drill, three timers, series in Prometheus, nothing uncommitted.

---

## PHASE 14: LIVE 2: HUGINN, HOST STATE, START LIMITS (Steps 297-312)

**Goal**: west's huginn exports the per-cgroup series; `unheaded-host-state` writes container and unit state every 30 s; under SS3 A the installed units carry the start-limit drop-in.
**Prerequisite**: Phase 12 gate.
**Time**: 45 min (plus 10 min of CPU reading).
**Model**: Opus. **Agent**: Coordinator, sudo.

- [ ] **Step 297** [S][B] ~3m: Build, keep the old binary, install, restart.
  ```bash
  cd /home/govan/tmp/unheaded && go build -trimpath -ldflags "-X unheaded/pkg/cmdline.Version=$(git describe --always)" -o /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/huginn ./cmd/huginn && sudo install -d -m 0750 /var/backups/s-see-and-survive && sudo cp -p /usr/local/bin/huginn /var/backups/s-see-and-survive/huginn.old && sudo install -m 0755 /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/huginn /usr/local/bin/huginn && sudo systemctl restart huginn && date -u +%FT%TZ
  ```
- [ ] **Step 298** [V] ~2m: `systemctl is-active huginn` → `active`; `curl -s 'http://[fd00:dead:beef::1]:9110/metrics' | grep -c '^huginn_cgroup_'` above 500; `... | grep '^huginn_cgroups_truncated'` ends in ` 0`.
  - Not active, or a restart loop → Step 299.
- [ ] **Step 299** [D] ~5m: `journalctl -u huginn -n 30 --no-pager`. **Rollback**: `sudo cp -p /var/backups/s-see-and-survive/huginn.old /usr/local/bin/huginn && sudo systemctl restart huginn`, then `[STUCK]`.
- [ ] **Step 300** [B] ~11m: huginn's cost after, the same way as Step 20. Log both readings in SSV-16.
- [ ] **Step 301** [V] ~1m: After minus before is at most 0.01 of a core. More than 0.02 → write it under "For Stevie" with the number; the collector stays (the series are decision 20's), and the walk interval is a follow-up.
- [ ] **Step 302** [S][B] ~2m: host-state: `cd /home/govan/tmp/unheaded && sudo install -d -m 0755 /usr/local/lib/unheaded/monitoring && sudo install -m 0755 scripts/monitoring/host-state-textfile.sh /usr/local/lib/unheaded/monitoring/ && sudo cp deploy/systemd/unheaded-host-state.{service,timer} /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now unheaded-host-state.timer && sudo systemctl start unheaded-host-state.service; systemctl status --no-pager unheaded-host-state | tail -3`
- [ ] **Step 303** [V] ~2m: `status=0/SUCCESS`; `grep -c '^unheaded_container_running' /var/lib/unheaded/textfile/host_state.prom` equals `docker ps -a --filter label=com.docker.compose.project=unheaded-dev -q | wc -l` plus the monitoring project's, minus the one-shots; `grep '^unheaded_unit_restarts{unit="unheaded-wiki-server.service"}' /var/lib/unheaded/textfile/host_state.prom` equals Step 15's `NRestarts`.
- [ ] **Step 304** [B] ~2m: Prometheus sees it through huginn: `source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; pq 'count(unheaded_container_running)'; pq 'unheaded_host_state_docker_ok'; pq 'huginn_textfile_rejected_lines{file="host_state.prom"}'`
- [ ] **Step 305** [V] ~1m: The count of Step 303; `1`; `0`. SSV-08 and SSV-16 after logged.
- [ ] **Step 306** [V] ~1m: SS3 in `$PROG`: **A** → Step 307. **B** → Steps 307-309 `[BLOCKED by SS3 B]`, go to Step 310.
- [ ] **Step 307** [S][B] ~2m: Drop-ins on west's installed long-running units (not the trace collector: it trips already), then reload. No unit restarts.
  ```bash
  for u in huginn unheaded-chaos-controller unheaded-kev-poller unheaded-syslog-relay unheaded-vor unheaded-wiki-server unheaded-zhen-ui-front zhen-agentd; do systemctl cat $u.service >/dev/null 2>&1 && sudo install -D -m 0644 /home/govan/tmp/unheaded/deploy/systemd/dropins/10-start-limit.conf /etc/systemd/system/$u.service.d/10-start-limit.conf; done; sudo systemctl daemon-reload
  ```
- [ ] **Step 308** [V] ~1m: `for u in huginn unheaded-kev-poller unheaded-wiki-server unheaded-vor; do systemctl show -p Id,StartLimitIntervalUSec,StartLimitBurst,NRestarts $u | tr '\n' ' '; echo; done` → `15min` and `10` each; `NRestarts` as before (nothing restarted).
- [ ] **Step 309** [W] ~2m: SSV-09 after: the units and their new limits. Undo: `sudo rm /etc/systemd/system/<unit>.service.d/10-start-limit.conf && sudo systemctl daemon-reload`.
- [ ] **Step 310** [C] ~1m: **COMMIT** `$PROG` on develop: "Steps 297-310: huginn per-cgroup series, host state, start limits live on west".
- [ ] **Step 311** [B] ~1m: `systemctl --failed --no-legend | wc -l`
- [ ] **Step 312** [V] ~1m: **PHASE 14 EXIT GATE**: the same failed-unit count as before Step 297 (0 expected); series live.

---

## PHASE 15: LIVE 3: RULES LOADED, MAIL ON, FOUR DRILLS (Steps 313-334)

**Goal**: Prometheus and Alertmanager run the new rules and routes; the mailer sends; a stopped container, a memory kill, a restart loop and a silent alert path each fired, and the two pages and the pipeline mail reached the outbox as `sent`, timed.
**Prerequisite**: Phases 13 and 14 gates (the series the rules read exist).
**Time**: 2 h (the pipeline drill holds 30 minutes).
**Model**: Opus. **Agent**: Coordinator, sudo; Stevie for one receipt.

- [ ] **Step 313** [B] ~1m: `source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; firing | sort > /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/firing-before.txt; cat /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/firing-before.txt`
- [ ] **Step 314** [S][B] ~2m: `sudo docker kill -s HUP unheaded-prometheus && sleep 20 && sudo docker kill -s HUP unheaded-alertmanager && sleep 10 && sudo docker logs --since 1m unheaded-prometheus 2>&1 | grep -i -E 'error|reload' | tail -4`
- [ ] **Step 315** [V] ~2m: "Completed loading of configuration file" and no error. `curl -s 127.0.0.1:9090/api/v1/rules | python3 -c 'import json,sys; n=[r["name"] for g in json.load(sys.stdin)["data"]["groups"] for r in g["rules"]]; print(all(x in n for x in ["ContainerStopped","MemoryKill","HostMemoryPressure","VictoriaMetricsDown","StoreBackupLate"]))'` → `True`. After a minute `pq 'up{job="victoria"}'` → 1. `curl -s 127.0.0.1:9093/api/v2/status | grep -c 'alerts/pipeline'` ≥ 1 (the loaded config holds the pipeline receiver's URL).
  - A config error → Step 316.
- [ ] **Step 316** [D] ~5m: Prometheus keeps its old config on a failed reload, so nothing is lost. Read the error; it names the file. A rule file → `git revert` its commit on develop (signed), HUP again, `[STUCK]` for that file only.
- [ ] **Step 317** [B] ~1m: `firing | sort > /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/firing-after.txt; comm -13 /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/firing-before.txt /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/firing-after.txt` (write both files first: uutils `comm` fails on two pipes).
- [ ] **Step 318** [V] ~2m: Each new firing alert is understood and written in `$PROG` (expected: `ContainerCPUThrottled` for akira, 44% throttled, urgency none, plan 2's fix). **No promoted alert may be firing before the mailer goes to send**: one that is → find why; fix, or mark it with `scripts/monitoring/planned-stop.sh` if it is an ordered stop; never silence a real fault to pass this step.
- [ ] **Step 319** [W] ~3m: SS2 A or B: on develop, `policy.yaml` `run_days.dates: ["<today in America/Chicago, YYYY-MM-DD>"]` (the drills send up to seven mails). **COMMIT** it signed, named file: "Step 319: today is a drill day (run_days)". SS2 C: spread Steps 325-332 over two days.
- [ ] **Step 320** [S][B] ~5m: The mailer goes to send: rebuild (it carries Phases 1-3) and recreate.
  ```bash
  cd /home/govan/tmp/unheaded/monitoring && sudo docker compose build alert-mailer && sudo docker compose up -d --no-deps alert-mailer && date -u +%FT%TZ
  ```
- [ ] **Step 321** [V] ~2m: `curl -s 127.0.0.1:19130/v1/status | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["mode"], d["max_per_day"])'` → `send 24` (or 50; or `send 6` under C); `sudo docker logs --since 2m unheaded-alert-mailer 2>&1 | grep -o '"relay_configured":[a-z]*'` → `true`.
  - Anything else → Step 322.
- [ ] **Step 322** [D] ~5m: `sudo docker logs --tail 20 unheaded-alert-mailer`. **Rollback**: `mode: dry-run` in `policy.yaml` on develop (signed commit), `sudo docker restart unheaded-alert-mailer`, `[STUCK]`. Mail stays off; Phase 15's drills are `[BLOCKED]`.
- [ ] **Step 323** [B] ~5m: A test page through Alertmanager (it reaches the mailer exactly as a rule's page would):
  ```bash
  end=$(date -u -d '+10 min' +%FT%TZ); curl -s -XPOST 127.0.0.1:9093/api/v2/alerts -H 'Content-Type: application/json' -d "[{\"labels\":{\"alertname\":\"MailPathTest\",\"urgency\":\"page\",\"severity\":\"none\"},\"annotations\":{\"summary\":\"S-SEE-AND-SURVIVE: mail is on (ADR-147 decision 2). No action.\"},\"endsAt\":\"$end\"}]"; sleep 200; source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; outbox
  ```
- [ ] **Step 324** [V][STEVIE] ~2m: The outbox shows `sent` for `MailPathTest`. Ask Stevie: "Did 'S-SEE-AND-SURVIVE: mail is on' reach your inbox?" Away: write "sent by the relay at <time>; receipt not confirmed" under "For Stevie". SSV-03 after.
- [ ] **Step 325** [S][B] ~8m: **Drill 1, ContainerStopped.** The container: the first that exists of `unheaded-captain`, `unheaded-architect`, `unheaded-micromanager`, `unheaded-monad`, `unheaded-sophia` (no callers; plan 2 removes them), else `unheaded-timeguru`.
  ```bash
  c=$(for n in unheaded-captain unheaded-architect unheaded-micromanager unheaded-monad unheaded-sophia unheaded-timeguru; do docker ps --format '{{.Names}}' | grep -qx $n && { echo $n; break; }; done); echo "$c"; t0=$(date +%s); sudo docker stop "$c" >/dev/null; source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; for i in $(seq 1 16); do sleep 30; firing | grep -q "ContainerStopped $c" && { echo "firing after $(( $(date +%s) - t0 )) s"; break; }; done; sleep 150; outbox | grep -i containerstopped; sudo docker start "$c" >/dev/null; echo "started $(date -u +%T)"
  ```
- [ ] **Step 326** [V] ~2m: Firing within 300 s; a `sent` page whose subject is `[page] ContainerStopped: firing` (the mailer names alerts in the subject; the container's name is in the body); the container `healthy` again within 90 s (`docker ps --filter name=$c`). Log the seconds to firing and to mail (SSV-01 after).
  - No firing in 8 min → Step 327.
- [ ] **Step 327** [D] ~10m: `grep "$c" /var/lib/unheaded/textfile/host_state.prom` (state written?), `pq "unheaded_container_running{name=\"$c\"}"` (forwarded?), `pq "max_over_time(unheaded_container_running{name=\"$c\"}[7d])"` (1?), `pq 'unheaded_planned_stop_until_timestamp_seconds'` (a stale mark?). Fix where it broke; two tries; **start the container again in every case**.
- [ ] **Step 328** [S][B] ~6m: **Drill 2, MemoryKill.** A throwaway container (pinned image already here, no network) that runs out of memory:
  ```bash
  t0=$(date +%s); sudo docker run --name ssv-oom-drill --network none --memory 32m --memory-swap 32m postgres:16.15-alpine sh -c 'tail /dev/zero' ; sudo docker inspect -f 'oom={{.State.OOMKilled}} exit={{.State.ExitCode}}' ssv-oom-drill; source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; for i in $(seq 1 10); do sleep 30; firing | grep -q MemoryKill && { echo "firing after $(( $(date +%s) - t0 )) s"; break; }; done; sleep 150; outbox | grep -i memorykill; sudo docker rm ssv-oom-drill
  ```
- [ ] **Step 329** [V] ~1m: `oom=true exit=137`; MemoryKill firing within 120 s with `cgroup="system.slice"`; a `sent` page. `HostOOMKill` fires too (urgency none). SSV-02 after, with the seconds.
  - No MemoryKill → read `pq 'increase(huginn_cgroup_memory_events_total{event="oom_kill",cgroup="system.slice"}[10m])'`; 0 means the docker scope sits elsewhere: `cat /proc/self/cgroup` of a running container's pid tells where; widen the rule's `cgroup` match to that slice (test case added), commit, HUP. Two tries.
- [ ] **Step 330** [S][B] ~6m: **Drill 3, UnitRestartLoop.** A transient unit that fails every 5 s (its start limit, systemd's default, never trips at that pace):
  ```bash
  sudo systemd-run --unit=unheaded-ssv-drill --property=Restart=always --property=RestartSec=5 /bin/false; sleep 150; sudo systemctl start unheaded-host-state.service; sleep 90; source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; firing | grep UnitRestartLoop; sudo systemctl stop unheaded-ssv-drill.service; sudo systemctl reset-failed unheaded-ssv-drill.service 2>/dev/null; systemctl status unheaded-ssv-drill.service --no-pager 2>&1 | head -2
  ```
- [ ] **Step 331** [V] ~1m: `UnitRestartLoop` firing for `unheaded-ssv-drill.service` (a ticket: the digest carries it, no mail now); the unit gone. SSV-08 after.
- [ ] **Step 332** [S][B] ~32m: **Drill 4, the alert path goes silent.** Mark Alertmanager's stop so ContainerStopped stays quiet, stop it, wait, start it.
  ```bash
  cd /home/govan/tmp/unheaded && sudo scripts/monitoring/planned-stop.sh unheaded-alertmanager 45m && t0=$(date +%s) && (cd monitoring && sudo docker compose stop alertmanager) && sleep 1500 && source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh && outbox | grep -i 'pipeline silent'; (cd monitoring && sudo docker compose up -d --no-deps alertmanager); sleep 420; outbox | grep -i 'pipeline back'; sudo scripts/monitoring/planned-stop.sh --end unheaded-alertmanager; echo "elapsed $(( $(date +%s) - t0 )) s"
  ```
- [ ] **Step 333** [V] ~2m: "alert pipeline silent" `sent` between 20 and 23 minutes after the stop; "alert pipeline back" `sent` within 7 minutes of the start; Alertmanager `healthy`. SSV-11 after, with both times.
  - No silent mail → `curl -s 127.0.0.1:19130/metrics | grep pipeline_last_seen` (was a receipt ever recorded? if 0, the route never delivered: compare `alertmanager.yml` on disk with `/api/v2/status`), then the mailer's log. Alertmanager is started again in every case. Two tries, then `[STUCK]`.
- [ ] **Step 334** [C][V] ~2m: **PHASE 15 EXIT GATE**: rules loaded, mail on, four drills logged with times (or `[STUCK]` with readings). **COMMIT** `$PROG` on develop: "Steps 313-334: rules live, mail on, four drills". SSV-01, 02, 03, 04, 06, 07, 08, 10, 11, 12, 13: after filled.

---

## PHASE 16: LIVE 4: THE BUCKETS ROLL OUT (Steps 335-348)

**Goal**: Every running service built from this tree serves the 0.1 ms buckets; Wotan's p50 reads near its true mean.
**Prerequisite**: Phase 15 gate (a container stopped by mistake during the rollout now mails). No other builder running (series rule 3).
**Time**: 1 h plus a 1 h wait.
**Model**: Opus. **Agent**: Coordinator, sudo.

- [ ] **Step 335** [B] ~1m: A planned stop for the rollout (silences mail, marks the budgets): `cd /home/govan/tmp/unheaded && sudo scripts/monitoring/maintenance.sh --planned 60m "S-SEE-AND-SURVIVE: latency buckets rollout"`
- [ ] **Step 336** [S][B] ~15m: Tag the running images for rollback, then build the services that use `pkg/httputil` and are running:
  ```bash
  cd /home/govan/tmp/unheaded && svcs=$(for s in wotan dashboard-backend kanban-app logstore-writer auth cuirass timeguru architect captain micromanager monad sophia; do sudo docker compose ps --format '{{.Service}}' | grep -qx $s && echo $s; done | tr '\n' ' '); echo "$svcs" > /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/bucket-svcs.txt; for s in $svcs; do sudo docker tag unheaded-dev-$s:latest unheaded-dev-$s:pre-ssv; done; sudo docker compose build $svcs 2>&1 | tail -3
  ```
- [ ] **Step 337** [S][B] ~10m: Recreate one at a time, Wotan last (every client reconnects to it):
  ```bash
  cd /home/govan/tmp/unheaded && for s in $(tr ' ' '\n' < /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/bucket-svcs.txt | grep -v '^wotan$') wotan; do sudo docker compose up -d --no-deps $s >/dev/null; for i in $(seq 1 24); do st=$(sudo docker inspect -f '{{.State.Health.Status}}' $(sudo docker compose ps -q $s)); [ "$st" = healthy ] && break; sleep 5; done; echo "$s $st"; done
  ```
- [ ] **Step 338** [V] ~2m: Every line ends `healthy`.
  - One is not healthy after 2 min → Step 339.
- [ ] **Step 339** [D] ~5m: `sudo docker compose logs --tail 30 <svc>`. **Rollback that service**: `sudo docker tag unheaded-dev-<svc>:pre-ssv unheaded-dev-<svc>:latest && sudo docker compose up -d --no-deps --no-build <svc>`, then `[STUCK]` for it.
- [ ] **Step 340** [S][B] ~8m: Host units on the same middleware, one at a time, old binary kept:
  ```bash
  cd /home/govan/tmp/unheaded && for b in kev-poller wiki-server chaos-controller; do u=unheaded-$b; systemctl is-active --quiet $u || continue; go build -trimpath -ldflags "-X unheaded/pkg/cmdline.Version=$(git describe --always)" -o /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/$b ./cmd/$b && sudo cp -p /usr/local/bin/$b /var/backups/s-see-and-survive/$b.old && sudo install -m 0755 /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/$b /usr/local/bin/$b && sudo systemctl restart $u && sleep 5 && echo "$u $(systemctl is-active $u)"; done
  ```
  A unit not active → `sudo cp -p /var/backups/s-see-and-survive/<b>.old /usr/local/bin/<b> && sudo systemctl restart unheaded-<b>`, `[STUCK]` for it.
- [ ] **Step 341** [B] ~1m: `sudo scripts/monitoring/maintenance.sh --end` (ends the silence and the mark).
- [ ] **Step 342** [B] ~60m: Wait an hour, then Step 13's commands.
- [ ] **Step 343** [V] ~2m: No service's p99 is exactly 0.00495; Wotan's p50 is under 0.0005 and within 2.5 times its mean (sum over count). Bucket series count logged (expected about 1,056 x 17/12, less any service plan 2 removed). SSV-15 after.
- [ ] **Step 344** [B] ~1m: `source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; firing | grep -c ContainerStopped` → 0.
- [ ] **Step 345** [W] ~3m: `$PROG`: rollout time (Phase 17 waits until 24 h after it), the services, the readings.
- [ ] **Step 346** [S][B] ~1m: The pre-rollout tags are kept until Phase 19 (Step 371 removes them).
- [ ] **Step 347** [C] ~1m: **COMMIT** `$PROG` on develop: "Steps 335-347: latency buckets live".
- [ ] **Step 348** [V] ~1m: **PHASE 16 EXIT GATE**: all healthy, buckets live, no stray alert.

---

## PHASE 17: LIVE 5: LATENCY AND ERROR SLIS FROM WHAT WAS MEASURED (Steps 349-362)

**Goal**: Wotan and dashboard-backend each have a latency SLI and an error SLI built on 24 h of the new buckets (SS4).
**Prerequisite**: Phase 16 gate, and 24 h since its rollout.
**Time**: 1 h.
**Model**: Opus. **Agent**: Coordinator; Stevie for SS4.

- [ ] **Step 349** [V] ~1m: 24 h have passed since the rollout time in `$PROG`.
- [ ] **Step 350** [B] ~2m: How Prometheus labels the two (target labels rename the middleware's `service` to `exported_service`): `source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; pq 'count by (job, instance, service, exported_service) (unheaded_http_request_duration_seconds_count{job=~"wotan|dashboard-backend"})'`
- [ ] **Step 351** [V] ~1m: Write the two selectors that name west's series alone, expected `{job="wotan"}` and `{job="dashboard-backend",instance="[fd00:dead:beef::1]:20000"}`. Use those in Steps 352 and 355.
- [ ] **Step 352** [B] ~2m: Idle readings over 24 h, with `<sel>` each selector: `pq 'histogram_quantile(0.99, sum by (le) (rate(unheaded_http_request_duration_seconds_bucket<sel>[24h])))'`; `pq 'sum(rate(unheaded_http_requests_total<sel,status=~"5..">[24h])) / sum(rate(unheaded_http_requests_total<sel>[24h]))'`.
- [ ] **Step 353** [W] ~3m: In `$PROG` a table: service, idle p99, 3 x p99, T (the smallest bound of `LatencyBuckets` at or above it), 5xx share.
- [ ] **Step 354** [STEVIE][DECIDE] ~3m: SS4 with that table. Away: B.
- [ ] **Step 355** [W] ~15m: In the sprint worktree (`git -C /home/govan/tmp/wt/S-SEE-AND-SURVIVE merge --ff-only develop` first), four SLOs in `policy.yaml` after `wotan` and `dashboard` (A: as written; B: each also `notify: false`), `T` from Step 353 written as the bound's text (for example `"0.0005"`):
  ```yaml
    - name: wotan-latency
      service: wotan
      description: Wotan answers 99% of requests in under <T> s (ADR-147 SS4, 3x idle p99)
      objective: 99.0
      page: false
      good: >-
        (sum(rate(unheaded_http_request_duration_seconds_bucket{job="wotan",le="<T>"}[5m]))
         / (sum(rate(unheaded_http_request_duration_seconds_count{job="wotan"}[5m])) > 0))
        or on() (sum(rate(unheaded_http_request_duration_seconds_count{job="wotan"}[5m])) == bool 0)
      checks: >-
        histogram_quantile(0.99, sum by (le, path) (rate(unheaded_http_request_duration_seconds_bucket{job="wotan"}[5m]))) names the path.
    - name: wotan-errors
      service: wotan
      description: Wotan answers 99.9% of requests without a 5xx
      objective: 99.9
      page: false
      good: >-
        (1 - sum(rate(unheaded_http_requests_total{job="wotan",status=~"5.."}[5m]))
             / (sum(rate(unheaded_http_requests_total{job="wotan"}[5m])) > 0))
        or on() (sum(rate(unheaded_http_requests_total{job="wotan"}[5m])) == bool 0)
      checks: >-
        sum by (path, status) (rate(unheaded_http_requests_total{job="wotan",status=~"5.."}[5m])); docker logs unheaded-wotan --since 15m.
  ```
  and `dashboard-latency`, `dashboard-errors` the same with the dashboard selector. A promtool case per SLO in `tests/slo_test.yml`: no traffic → good 1; all fast → 1; half slow → 0.5.
- [ ] **Step 356** [B] ~10m: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE && go run ./cmd/alert-policy && go test -race -count=1 ./cmd/alert-policy 2>&1 | tail -2` and RULES-GATE (run in this tree: `cd /home/govan/tmp/wt/S-SEE-AND-SURVIVE && scripts/check-prometheus-rules.sh 2>&1 | tail -4 && go run ./cmd/alert-policy --check 2>&1 | tail -2`).
- [ ] **Step 357** [V] ~1m: Green.
- [ ] **Step 358** [C] ~1m: **COMMIT** `observability/alerting/policy.yaml monitoring/prometheus/rules/slo.yml monitoring/prometheus/rules/tests/slo_test.yml monitoring/alertmanager/alertmanager.yml`: "Steps 349-358: latency and error SLIs (SS4 <A|B>)". Merge to develop as Step 267 does.
- [ ] **Step 359** [S][B] ~1m: `sudo docker kill -s HUP unheaded-prometheus && sudo docker kill -s HUP unheaded-alertmanager`
- [ ] **Step 360** [B] ~5m: Five minutes later: `source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh; pq 'slo:good{slo=~"wotan-latency|wotan-errors|dashboard-latency|dashboard-errors"}'`
- [ ] **Step 361** [V] ~1m: Four series, each near 1 at idle. A value under 0.9 at idle means T is too tight for some path: log the path from `checks:`, and under SS4 A ask Stevie before loosening.
- [ ] **Step 362** [C][V] ~1m: **PHASE 17 EXIT GATE**. SSV-20 after. **COMMIT** `$PROG` on develop.

---

## PHASE 18: LIVE 6: VOR, NGINX AND THE SMOKE CHECK (Steps 363-370)

**Goal**: vor logs requests on west; nginx logs request time; `qa-smoke.sh` passes.
**Prerequisite**: Phase 12 gate.
**Time**: 40 min.
**Model**: Opus. **Agent**: Coordinator, sudo.

- [ ] **Step 363** [S][B] ~6m: vor: merge, build, keep the old binary, install, restart, time the warm-up.
  ```bash
  git -C /home/govan/tmp/projects/vor merge --no-ff -S access-log -m "Merge access-log: one JSON line per request" && make -C /home/govan/tmp/projects/vor build >/dev/null && sudo cp -p /usr/local/bin/vor /var/backups/s-see-and-survive/vor.old && sudo install -m 0755 /home/govan/tmp/projects/vor/vor /usr/local/bin/vor && t0=$(date +%s) && sudo systemctl restart unheaded-vor && until curl -s -o /dev/null -m2 http://127.0.0.1:20110/api/health; do sleep 1; done; echo "ready after $(( $(date +%s) - t0 )) s"; curl -s -o /dev/null 'http://127.0.0.1:20110/api/search?q=bgp&limit=1'; journalctl -u unheaded-vor --since -2min --no-pager -q | grep -c '"uri"'
  ```
  vor's checkout had local changes at Step 229 → do not merge into it: write "vor merge waits for Stevie's checkout" under "For Stevie", build from `/home/govan/tmp/wt/vor-access-log` instead, and install that.
- [ ] **Step 364** [V] ~1m: Ready time logged; at least 2 request lines. Not ready in 120 s → `sudo cp -p /var/backups/s-see-and-survive/vor.old /usr/local/bin/vor && sudo systemctl restart unheaded-vor`, `[STUCK]`. SSV-24 after.
- [ ] **Step 365** [S][B] ~3m: nginx: keep the old site file, install both, test, reload, one request.
  ```bash
  cd /home/govan/tmp/unheaded && sudo cp -p /etc/nginx/sites-enabled/unheaded-apt.conf /var/backups/s-see-and-survive/unheaded-apt.conf.old && sudo install -m 0644 deploy/nginx/conf.d/unheaded-timed.conf /etc/nginx/conf.d/unheaded-timed.conf && sudo install -m 0644 deploy/nginx/unheaded-apt.conf /etc/nginx/sites-enabled/unheaded-apt.conf && sudo nginx -t 2>&1 | tail -2 && sudo systemctl reload nginx && curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18888/ && sudo tail -1 /var/log/nginx/unheaded-apt.access.log | grep -o 'rt=[0-9.]*'
  ```
- [ ] **Step 366** [V] ~1m: `test is successful`; an `rt=` value. Any failure before the reload → `sudo rm /etc/nginx/conf.d/unheaded-timed.conf && sudo cp -p /var/backups/s-see-and-survive/unheaded-apt.conf.old /etc/nginx/sites-enabled/unheaded-apt.conf && sudo nginx -t && sudo systemctl reload nginx`, `[STUCK]`. SSV-25 after.
- [ ] **Step 367** [B] ~3m: SSV-23 after: `cd /home/govan/tmp/unheaded && scripts/qa-smoke.sh 2>&1 | tail -8`
- [ ] **Step 368** [V] ~2m: `failed 0`, or each FAIL is a real fault written in `$PROG` (not a probe that cannot pass). Compare with Step 263.
- [ ] **Step 369** [C] ~1m: **COMMIT** `$PROG` on develop: "Steps 363-369: vor and nginx log request time; qa-smoke passes".
- [ ] **Step 370** [V] ~1m: **PHASE 18 EXIT GATE**.

---

## PHASE 19: CLOSE (Steps 371-387)

**Goal**: Every row closed or declined with a reason; the gates green on develop; the records say the same thing; next.md written.
**Prerequisite**: Phases 13 to 18 gates, or their `[STUCK]` rows written.
**Time**: 1.5 h.
**Model**: Opus. **Agent**: Coordinator.

- [ ] **Step 371** [S][B] ~2m: Housekeeping: `for s in $(cat /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/bucket-svcs.txt); do sudo docker rmi unheaded-dev-$s:pre-ssv >/dev/null 2>&1; done`; remove today's date from `run_days.dates` on develop (signed commit, named file) and restart the mailer (`sudo docker restart unheaded-alert-mailer`). Keep `/var/backups/s-see-and-survive/` for 3 days: write its removal under "For Stevie / housekeeping".
- [ ] **Step 372** [B] ~20m: The full local CI, the CI venv last on PATH: `cd /home/govan/tmp/unheaded && PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin scripts/ci-local.sh 2>&1 | tail -25`
- [ ] **Step 373** [V] ~2m: Each gate on its own line passes, or its FAIL is shown on develop before this sprint (rerun that gate in a clean clone of `1393e5ee`).
- [ ] **Step 374** [D] ~10m: A FAIL this sprint caused → fix it in its files, commit, rerun Step 372. Two tries.
- [ ] **Step 375** [W] ~15m: The closing table in `$PROG`: SSV-01 to SSV-28, each `closed` with its after and commit, or `declined`/`handed` with the reason: SSV-14 (AIDE producer: S-EVIDENCE-RUNS Phase 10), SSV-26 (orb: SS6, plan 8), SSV-27 (second recorder and run marker: plan 8), SSV-28 (`/health` semantics: "For Stevie"). Plan 4 greps this file: the rows say "clickhouse backup", "victoria backup", "restore drill", "swap", "memory pressure", "stopped container" and "oom" in plain words.
- [ ] **Step 376** [W] ~10m: `docs/adr/ADR-147-every-service-earns-its-place.md`: a dated section "S-SEE-AND-SURVIVE (YYYY-MM-DD)": one table, the 28 rows' before and after, and the drills' times. Status unchanged.
- [ ] **Step 377** [W] ~3m: `docs/adr/ADR-116-monitor-the-monitors.md`: a dated note: R4's receipt is built for the alert path (the mailer's pipeline mail, S-SEE-AND-SURVIVE Phase 2); the warden, mutual heartbeats and the out-of-band paths are not.
- [ ] **Step 378** [W] ~3m: `docs/battle-plans/progress/S-E1-LIVE-progress.md`: a dated line: the mailer went to `send` on <date> under ADR-147 decision 2 (S-SEE-AND-SURVIVE Step 320); Steps 81-83 and 102-109's mail part is done there.
- [ ] **Step 379** [B] ~2m: Docs that still say dry-run: `cd /home/govan/tmp/unheaded && grep -rn -i 'dry-run' wiki/*.md CLAUDE.md | grep -i -E 'mailer|alert' | head`
- [ ] **Step 380** [W] ~10m: Each hit that describes the current state is corrected in place (dated records are not edited). `wiki/Alerting.md`, `wiki/Systemd-Units.md`, `wiki/Well-Backups.md`, the huginn page: read once more against what is live.
- [ ] **Step 381** [C] ~1m: **COMMIT** the ADRs, progress files and wiki pages by name: "Steps 375-381: records".
- [ ] **Step 382** [B] ~1m: Worktrees: `for w in -alerting -buckets -huginn -state -backup -smoke -vor ""; do git -C /home/govan/tmp/unheaded worktree remove /home/govan/tmp/wt/S-SEE-AND-SURVIVE$w 2>/dev/null; done; git -C /home/govan/tmp/projects/vor worktree remove /home/govan/tmp/wt/vor-access-log 2>/dev/null; git -C /home/govan/tmp/unheaded worktree list | grep -c S-SEE-AND-SURVIVE`
- [ ] **Step 383** [V] ~1m: `0`.
- [ ] **Step 384** [W] ~10m: `~/tmp/next.md`: a section at the top: the before and after table (short), the drills' times, what waits on Stevie (SS answers given while away, the test mail's receipt, SS5 offsite, the orion lines if SS7 A, the vor merge if its checkout was dirty, runbook seals, `/var/backups/s-see-and-survive/` removal), every `[STUCK]`, and the push counts here and in vor.
- [ ] **Step 385** [V] ~1m: `git -C /home/govan/tmp/unheaded status --short` shows no tracked change; `git -C /home/govan/tmp/unheaded log origin/develop..develop --format='%G? %s' | grep -vc '^G'` is 0, or the unsigned ones are listed for `scripts/resign-unpushed.py`.
- [ ] **Step 386** [V] ~1m: `source /home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch/lib.sh 2>/dev/null || true; curl -s 127.0.0.1:19130/v1/status | grep -o '"mode":"[a-z-]*"'` → `"mode":"send"`; `systemctl list-timers --no-pager | grep -c -E 'host-state|clickhouse-backup|victoria-backup|store-drill'` → 4.
- [ ] **Step 387** [V] ~1m: **PHASE 19 EXIT GATE**: develop holds the sprint; gates green; rows closed or reasoned; next.md written. Stevie pushes.

---

## Appendix A: Emergency procedures

1. **Mail floods after Step 320.** The cap holds (24 on a run day, 6 otherwise); past it the mailer counts and holds. To stop at once: `cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh 2h "mail flood: reading"` (silences pages and tickets). To go back to dry-run: `mode: dry-run` in `policy.yaml` on develop, signed commit, `sudo docker restart unheaded-alert-mailer`. Then find the rule: `outbox` and `firing` from the lib.
2. **A new rule fires on everything** (a selector too wide). It is `urgency: none` unless promoted, so it mails nothing by itself; a promoted one mails once per group. `git revert` the rule file's commit on develop, HUP Prometheus, fix the test that let it through, `[STUCK]` for that rule.
3. **ClickHouse does not come back after Step 276.** Step 278's rollback. Vector blocks while ClickHouse is down (`when_full: block`), so journal rows wait in its 256 MiB buffer, nothing is dropped; `JournalNotShipping` fires after 15 min.
4. **Docker made `/srv/backups/unheaded-clickhouse` as root.** If `docker compose up` ran before Step 274, the bind source exists `root:root` and ClickHouse cannot write. `sudo chown 16707:16707 /srv/backups/unheaded-clickhouse && sudo chmod 0700 /srv/backups/unheaded-clickhouse`; no restart needed.
5. **A drill container or a VictoriaMetrics snapshot was left behind** (a killed drill). `docker ps -a --filter name=ch-drill --filter name=vm-drill` → `sudo docker rm -f <name>`; `sudo rm -rf /srv/backups/.drill-clickhouse /srv/backups/.drill-victoria`. A snapshot: list with `printf 'data-urlencode = "authKey=%s"\n' "$(sudo cat <victoria-auth source>/admin.key)" | sudo curl -fsS -G --config - http://127.0.0.1:8428/snapshot/list` and delete by name with `/snapshot/delete` the same way. Never print the key.
6. **The backups refuse after plan 4 moved the volumes** ("share disk"). Expected: the sets must leave the NVMe. Stevie names the root; change `CH_BACKUP_DIR`, `VM_BACKUP_DIR` (units) and `CLICKHOUSE_BACKUP_DIR` (compose `.env`), recreate ClickHouse, run both backups and the drill again.
7. **huginn crash-loops after Step 297.** Step 299's rollback. With SS3 A, after 10 starts in 15 min it stays `failed` and `UnitFailed` pages (if huginn is the failed unit, host-state still reports it, but nothing forwards it: `HuginnDown` from Prometheus's own scrape covers that).
8. **host_state.prom stops changing.** `HostStateStale` fires. `systemctl status unheaded-host-state.timer unheaded-host-state.service`; `journalctl -u unheaded-host-state -n 20`. While it is stale, ContainerStopped cannot fire: treat as urgent.
9. **The planned-stop mark is stale** (a stop marked 30d and the startup never ran). ContainerStopped and every SLO read good for a stopped stack. `scripts/monitoring/planned-stop.sh --list`; `sudo scripts/monitoring/planned-stop.sh --end`.
10. **Prometheus or Alertmanager will not reload.** Both keep the old config on a failed reload. Read `docker logs --since 5m`, revert the file's commit, HUP again. Never `docker restart` Prometheus to clear a config error: it would then not start.

## Appendix B: Agent assignment and dependencies

| Phase | Lane / agent | Model | Parallel | Depends on | Time |
|---|---|---|---|---|---|
| 0 Preconditions | Coordinator | Opus | no | none | 50m |
| 1 Mail on, run days | alerting | Sonnet | [P] with 3, 4, 5, 9, 10, 11 | 0 | 1.5h |
| 2 Pipeline receipt | alerting | Opus | in lane | 1 | 2.5h |
| 3 Buckets | buckets | Sonnet | [P] | 0 | 45m |
| 4 huginn cgroups | huginn | Sonnet | [P] | 0 | 3h |
| 5 Host state | state | Opus | [P] | 0 | 2.5h |
| 6 Start limits | state | Opus | in lane | 5, SS3 | 1h |
| 7 Rules | alerting | Sonnet | in lane | 2 | 4h |
| 8 SLOs, planned stop | alerting | Opus | in lane | 7 | 3h |
| 9 Store backups (code) | backup | Opus | [P] | 0 | 5h |
| 10 Smoke | smoke | Sonnet | [P] | 0 | 45m |
| 11 vor, nginx | vor | Sonnet | [P] | 0, SS7 | 1.5h |
| 12 E-GATE, merge | Coordinator | Opus | no | 1-11, the E-GATE | 1.5h |
| 13 Live backups | Coordinator, sudo | Opus | no | 12 | 1.5h |
| 14 Live huginn, state | Coordinator, sudo | Opus | no | 12 | 1h |
| 15 Live rules, mail, drills | Coordinator, sudo, Stevie (receipt) | Opus | no | 13, 14 | 2h |
| 16 Live buckets | Coordinator, sudo | Opus | no | 15 | 2h |
| 17 SLIs | Coordinator, Stevie (SS4) | Opus | no | 16 + 24 h | 1h |
| 18 vor, nginx, smoke live | Coordinator, sudo | Opus | beside 16-17 | 12 | 40m |
| 19 Close | Coordinator | Opus | no | 13-18 | 1.5h |

**Swarm (code, before the E-GATE).** Disjoint files: `alerting` (`cmd/alert-mailer`, `cmd/alert-policy`, `observability/`, `monitoring/`, `runbooks/observe`, `runbooks/data`, `scripts/monitoring/{maintenance,planned-stop,test-planned-stop,test-alert-path}.sh`, `scripts/kingdom-*.sh`, `wiki/Alerting.md`), `buckets` (`pkg/metrics/prom`, `pkg/httputil`, `services/wotan/internal/metrics`), `huginn` (`cmd/huginn`, `deploy/systemd/huginn.service`, the huginn wiki page), `state` (`scripts/monitoring/host-state-*`, `scripts/check-unit-start-limits.sh`, `scripts/check-gates-can-fail.sh`, `deploy/systemd/unheaded-host-state.*`, `deploy/systemd/dropins/`, unit files' `[Unit]` lines, `wiki/Systemd-Units.md`), `backup` (`scripts/backup/`, `config/clickhouse/backups.xml`, `docker-compose.yml` two lines, its units, `wiki/Well-Backups.md`, `.github/workflows/security.yml` one step), `smoke` (`scripts/qa-smoke.sh`), `vor` (the vor repository, `deploy/nginx/`). Shared: `config/logging/sources.yaml` (state, backup; both sides kept at Step 250). At most three lanes build at once; doc-only work does not count.

**Critical path**: 0 → 1 → 2 → 7 → 8 (about 11 h of code) → the E-GATE → 12 → 13, 14 → 15 → 16 → 24 h → 17 → 19. Phases 3, 4, 5, 6, 9, 10, 11 fit beside the alerting lane. Phase 18 runs beside 16 and 17.

## Appendix C: Quick reference

**Rows (the progress file's Fixes table)**

| id | Seat, row | Fix | Phase(s) |
|---|---|---|---|
| SSV-01 | Micromanager D1; BlackMage F2; decision 1 | ContainerStopped (host state, rule, page, drill) | 5, 7, 14, 15 |
| SSV-02 | BlackMage F2; decision 2 | MemoryKill (cgroup events, rule, page, drill) | 4, 7, 14, 15 |
| SSV-03 | Micromanager D7; decision 2 | Mailer in send, test mail | 1, 15 |
| SSV-04 | BlackMage 1.7; decision 2 | What mails (SS1 promotions) | 7, 15 |
| SSV-05 | Decision 2 | Run-day caps (SS2) | 1, 15 |
| SSV-06 | Scientist F3; BlackMage R4 | Pressure, CPU, heat, throttling, near-limit rules | 7, 15 |
| SSV-07 | Decision 12 | Swap and memory pressure rules | 7, 15 |
| SSV-08 | Micromanager D9; Scientist A4 | Restart counters, UnitFailed, UnitRestartLoop, drill | 5, 7, 14, 15 |
| SSV-09 | BlackMage F16 | Start limits that trip (SS3), gate, drop-ins | 6, 14 |
| SSV-10 | Micromanager D8; BlackMage 1.7 | Watcher rules; Prometheus job `victoria` | 7, 15 |
| SSV-11 | ADR-116 R4; Micromanager rank 4 | The mailer's pipeline mail, drill | 2, 15 |
| SSV-12 | ADR-147; S-WATCH-TRUE | TraceCollectorHighLatency reads the live histogram | 7, 15 |
| SSV-13 | BlackMage F13 | VectorSinkDroppingEvents, any component | 7, 15 |
| SSV-14 | BlackMage F14 | AIDE rules: producer is S-EVIDENCE-RUNS Phase 10 | handed |
| SSV-15 | Scientist F1 | Latency buckets from 0.1 ms | 3, 16 |
| SSV-16 | Scientist F10, A1 | Per-cgroup series in huginn | 4, 14 |
| SSV-17 | Scientist F4; decision 4 | Planned stops spend no budget | 8, 15 |
| SSV-18 | Micromanager D6; decision 4 | `ci-develop` recorded, not notified | 8, 15 |
| SSV-19 | Micromanager D3 | Wotan SLO says west | 8, 15 |
| SSV-20 | Scientist F2, A8 | Latency and error SLIs (SS4) | 17 |
| SSV-21 | Erda F15; decision 6 | ClickHouse backup and restore drill | 9, 13 |
| SSV-22 | Erda F15; decision 6 | VictoriaMetrics backup and restore drill | 9, 13 |
| SSV-23 | Micromanager D5 | qa-smoke can pass | 10, 12, 18 |
| SSV-24 | Scientist F9; decision 23 | vor access log | 11, 18 |
| SSV-25 | Scientist F9; decision 23 | nginx request time (SS7 for orion) | 11, 18 |
| SSV-26 | Decision 23 | orb's load: open gate (SS6) | handed to plan 8 |
| SSV-27 | Scientist A5, A7 | Second recorder, run marker | handed to plan 8 |
| SSV-28 | Micromanager D2 | `/health` 200 when degraded | For Stevie |

**New series**

| Series | Labels | From |
|---|---|---|
| `unheaded_container_running`, `_exit_code`, `_oom_killed`, `_healthy`, `_started_timestamp_seconds`, `_info` | `host, name` (+ `project, service` on running; `container_id` on info) | host-state (textfile) |
| `unheaded_unit_active`, `unheaded_unit_failed`, `unheaded_unit_restarts` | `host, unit` | host-state |
| `unheaded_host_state_last_run_timestamp_seconds`, `unheaded_host_state_docker_ok` | `host` | host-state |
| `huginn_cgroup_cpu_usage_seconds_total`, `_cpu_periods_total`, `_cpu_throttled_periods_total`, `_cpu_throttled_seconds_total` | `host, cgroup, container_id?` | huginn |
| `huginn_cgroup_memory_bytes`, `_memory_max_bytes`, `_memory_swap_bytes`, `_memory_events_total{event}` | same | huginn |
| `huginn_cgroup_io_bytes_total{device,dir}`, `_io_ops_total{device,dir}`, `_pressure_seconds_total{resource,kind}`, `_pids` | same | huginn |
| `huginn_cgroups_seen`, `huginn_cgroups_truncated` | `host` | huginn |
| `unheaded_planned_stop_until_timestamp_seconds` | `host, scope` | planned-stop.sh (textfile) |
| `unheaded_store_backup_last_success_timestamp_seconds`, `_bytes`, `_duration_seconds` | `host, store` | store backups (textfile) |
| `unheaded_store_drill_last_success_timestamp_seconds`, `_duration_seconds` | `host, store` | store drill (textfile) |
| `unheaded_alert_mailer_pipeline_last_seen_timestamp_seconds` | none | alert-mailer |

**New alerts**: ContainerStopped, MemoryKill, HostOOMKill, ContainerUnhealthy, ContainerCPUThrottled, ContainerMemoryNearLimit (`containers.yml`); HostMemoryPressure, HostSwapping, HostSwapNearFull, HostIOPressure, HostCPUPressure, HostTemperatureNearCritical, UnitFailed, UnitRestartLoop, HostStateStale (`resources.yml`); VictoriaMetricsDown, HuginnDown, AlertMailerDown, PrometheusRemoteWriteFailing, AlertmanagerNotReached (`watchers.yml`); StoreBackupLate, StoreBackupMissing, StoreDrillLate (`backups.yml`). **Changed**: TraceCollectorHighLatency, VectorSinkDroppingEvents. **New SLOs**: store-backups; wotan-latency, wotan-errors, dashboard-latency, dashboard-errors (Phase 17). **Changed SLOs**: every `slo:good` honours a planned stop; ci-develop `notify: false`; wotan's description.

**Paths**: textfile dir `/var/lib/unheaded/textfile`; sets `/srv/backups/unheaded-clickhouse/ch-<UTC>`, `/srv/backups/unheaded-victoria/vm-<UTC>` (side files `<name>.meta.env`, `<name>.SHA256SUMS`, `LATEST`); drill scratch `/srv/backups/.drill-{clickhouse,victoria}`; installed scripts `/usr/local/lib/unheaded/backup/`, `/usr/local/lib/unheaded/monitoring/`; rollback copies `/var/backups/s-see-and-survive/`; mailer `127.0.0.1:19130` (`/v1/status`, `/v1/outbox`, `POST /v1/alerts/pipeline`); Prometheus `127.0.0.1:9090`; Alertmanager `127.0.0.1:9093`; VictoriaMetrics `127.0.0.1:8428`; huginn `[fd00:dead:beef::1]:9110`; deployment CA `configs/certs/dev/ca.pem`; VictoriaMetrics drill port `127.0.0.1:26428`.

**Gates**: `scripts/check-prometheus-rules.sh`, `go run ./cmd/alert-policy --check`, `tests/runbooks/test_alert_runbooks.py`, `scripts/runbook-manifest.sh --write`, `scripts/check-unit-start-limits.sh` (new), `scripts/check-unit-ceilings.sh`, `scripts/check-unit-binaries.sh`, `scripts/check-gates-can-fail.sh --only <gate>`, `scripts/monitoring/test-host-state-textfile.sh`, `scripts/monitoring/test-planned-stop.sh`, `scripts/monitoring/test-alert-path.sh`, `scripts/backup/test-store-backups.sh`, `scripts/ci-local.sh`.

---

*S-SEE-AND-SURVIVE Battle Plan, forged 2026-11-15*
*20 Phases. 387 Steps. A container that stops, a process killed for memory and a silent alert path each reach Stevie's inbox; the instruments can read a load; the two stores that had no copy have one, restored.*
