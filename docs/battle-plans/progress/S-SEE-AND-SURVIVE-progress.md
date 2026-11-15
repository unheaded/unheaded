<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-SEE-AND-SURVIVE progress

Plan: `docs/battle-plans/S-SEE-AND-SURVIVE-battle-plan.md` (20 phases, 387 steps; ADR-147 plan 1).
Branch `sprint/s-see-and-survive` in `/home/govan/tmp/wt/S-SEE-AND-SURVIVE`, off develop `8d35785d`. Lanes branch off it and merge back into it, never into develop.
This session runs Phases 0 to 11 (code) unattended (lane brief `~/tmp/adr147-table/LANE-BRIEF.md`). Phase 12 onward waits for the evidence week (ends 2026-11-17 18:34Z) and the coordinator.

## Decisions

Stevie said (2026-11-15, late) "focus only on current churn - no questions for now". Steps 5 and 6 were not asked. Each choice below only shapes code on this branch (reversible) unless said; it is **taken on the plan's recommendation, Stevie to confirm**.

| # | Question | Taken | How | Status |
|---|---|---|---|---|
| SS1 | Which alerts mail, and how | **A**: the "Proposed urgency" table (pages on ContainerStopped, MemoryKill, HostMemoryPressure, UnitFailed, HostDiskFull; tickets on the rest it lists) | plan's recommendation, 2026-11-15 | Stevie to confirm before Phase 15 loads it live |
| SS2 | Run-day caps | **A**: `mailer.run_days`, 24 a day and 6 an hour on listed dates | plan's recommendation | Stevie to confirm |
| SS3 | Host units restart for ever | **A**: 10 starts in 15 min, then `failed` and UnitFailed pages | plan's recommendation (repository unit files only; the west drop-ins are Phase 14, live) | Stevie to confirm |
| SS4 | Latency and error SLIs | not asked: Step 354 (Phase 17) | | open |
| SS5 | Store sets offsite on east | **A** (with plan 6) recorded for the wiki text only; nothing here copies to east | plan's recommendation | Stevie to confirm |
| SS6 | What counts as load on orb | **A** recorded; stays an open gate for plan 8 | plan's recommendation | open gate (plan 8) |
| SS7 | orion's nginx logs request time | **A**: the two lines for orion are under "For Stevie" (his hands; nothing here touches orion) | plan's recommendation | Stevie to act or decline |

## Fixes

Befores taken 2026-11-15 21:40-21:52Z on west, read only (lane brief: no sudo, nothing that burdens Vector, ClickHouse or Postgres). Commands are the plan's Steps 10-20 unless said.

| id | Seat, row | Evidence | Fix | Owner | Before | After | Status |
|---|---|---|---|---|---|---|---|
| SSV-01 | Micromanager D1; BlackMage F2; decision 1 | Step 10 (`docker inspect`), 21:40:57Z | ContainerStopped: host state, rule, page, drill | plan 1 | 23 running (18 restart `no`, 5 `unless-stopped`); 12 exited (3 one-shots exit 0, haproxy-edge, haproxy-internal, sword, 5 netbox-docker, jenkins); 0 rules on a stopped container | | open |
| SSV-02 | BlackMage F2; decision 2 | Step 14 + Grounding | MemoryKill: cgroup events, rule, page, drill | plan 1 | 0 rules; `node_vmstat_oom_kill` 6 (Grounding, host total since boot); 0 `huginn_cgroup_*` series | | open |
| SSV-03 | Micromanager D7; decision 2 | Step 11 (`/v1/status`), 21:41:05Z | Mailer in send, test mail | plan 1 | mode `dry-run`, `max_per_day` 6, sent_today 1, open tickets 9; `unheaded_alert_mailer_messages_total{urgency="ticket",outcome="dry-run"}` 4 | | open |
| SSV-04 | BlackMage 1.7; decision 2 | Step 12 (grep), 21:41:05Z | What mails (SS1 promotions) | plan 1 | 55 hand-written alerts (all `urgency: none`), 32 generated SLO alerts, 8 promotions in `policy.yaml` | | open |
| SSV-05 | Decision 2 | Step 11 | Run-day caps (SS2) | plan 1 | one cap: 6 a day, no run-day mechanism | | open |
| SSV-06 | Scientist F3; BlackMage R4 | Step 12 | Pressure, CPU, heat, throttling, near-limit rules | plan 1 | 0 rules read pressure, swap, oom, temperature or restarts (3 files match the grep, all in comments) | | open |
| SSV-07 | Decision 12 | Step 12 | Swap and memory pressure rules | plan 1 | 0 rules | | open |
| SSV-08 | Micromanager D9; Scientist A4 | Step 15 (`systemctl show`), 21:41:17Z | Restart counters, UnitFailed, UnitRestartLoop, drill | plan 1 | 9 running units; 0 failed; wiki-server `NRestarts=15`; no restart or failed series exported | | open |
| SSV-09 | BlackMage F16 | Step 15 | Start limits that trip (SS3), gate, drop-ins | plan 1 | west: 8 of 9 running units on systemd's default 10 s / 5 starts with `RestartSec` 5 to 30 s (cannot trip); trace-collector 10 min / 60. Repository side: Step 118 | | open |
| SSV-10 | Micromanager D8; BlackMage 1.7 | Step 12 | Watcher rules; Prometheus job `victoria` | plan 1 | 0 rules on VictoriaMetrics, remote write, huginn, the mailer or Alertmanager; no `victoria` job | | open |
| SSV-11 | ADR-116 R4; Micromanager rank 4 | Step 16 (`ALERTS`), 21:41:20Z | The mailer's pipeline mail, drill | plan 1 | `AlertPipelineHeartbeat` firing, routed to receiver `none`; nothing mails when the path goes silent | | open |
| SSV-12 | ADR-147; S-WATCH-TRUE | Step 17 (`:16671/metrics`), 21:41:20Z | TraceCollectorHighLatency reads the live histogram | plan 1 | `trace_collector_batch_latency_seconds_count` 0; seven `trace_publisher_flush_duration_seconds_count` above 0 (5,295 to 109,627) | | open |
| SSV-13 | BlackMage F13 | Step 17 | VectorSinkDroppingEvents, any component | plan 1 | `vector_buffer_discarded_events_total`: no series for any component | | open |
| SSV-14 | BlackMage F14 | Grounding | AIDE rules: producer is S-EVIDENCE-RUNS Phase 10 | S-EVIDENCE-RUNS | `unheaded-aide-check.timer` not installed on west; rules unchanged | no change | handed (S-EVIDENCE-RUNS Phase 10, attended) |
| SSV-15 | Scientist F1 | Step 13 (VictoriaMetrics), 21:41:14Z | Latency buckets from 0.1 ms | plan 1 | p50 0.0025 and p99 0.00495 for 13 services (logstore-writer p50 0.0047, p99 1.56); means 0.00011 to 0.0019 s (wotan 0.000136); 1,080 bucket series | | open |
| SSV-16 | Scientist F10, A1 | Step 14 (VictoriaMetrics), 21:41:14Z; Step 20 | Per-cgroup series in huginn | plan 1 | 24 targets export `process_cpu_seconds_total`; 0 `huginn_cgroup_*` series; huginn's own cost 0.0058 core (Step 20, `CPUUsageNSec` over 600 s, 21:40:56-21:50:56Z) | | open |
| SSV-17 | Scientist F4; decision 4 | Step 16 (Prometheus), 21:41:20Z | Planned stops spend no budget | plan 1 | budgets: dashboard -31.45, well -28.42, log-shipping -28.15, well-backups -17.46, wotan -15.25, ci-develop -99; overlay 0.994, kev-feed 0.987 | | open |
| SSV-18 | Micromanager D6; decision 4 | Step 16 | `ci-develop` recorded, not notified | plan 1 | `ErrorBudgetBurn{slo="ci-develop"}` firing, four tiers | | open |
| SSV-19 | Micromanager D3 | Step 16 + `policy.yaml` | Wotan SLO says west | plan 1 | the `wotan` SLO text says both hosts; one target (west) | | open |
| SSV-20 | Scientist F2, A8 | Phase 17 | Latency and error SLIs (SS4) | plan 1 | taken at Phase 17 (needs 24 h of the new buckets) | | open |
| SSV-21 | Erda F15; decision 6 | Step 18 (`ls /srv/backups`), 21:41:20Z | ClickHouse backup and restore drill | plan 1 | no ClickHouse set; `/srv/backups` holds `unheaded-well` and `unheaded-grafana-10.4` only; size read at Phase 13 (SQL waits for the E-GATE) | | open |
| SSV-22 | Erda F15; decision 6 | Step 18 (VictoriaMetrics), 21:41:20Z | VictoriaMetrics backup and restore drill | plan 1 | no set; `sum(vm_data_size_bytes)` 221,484,125 (221 MB) | | open |
| SSV-23 | Micromanager D5 | | qa-smoke can pass | plan 1 | taken at Step 263 (it opens a psql session) | | open |
| SSV-24 | Scientist F9; decision 23 | Step 19 (`journalctl`), 21:41:22Z | vor access log | plan 1 | 171 journal lines in 7 d, 0 request lines | | open |
| SSV-25 | Scientist F9; decision 23 | Step 19 (`grep /etc/nginx`) | nginx request time (SS7 for orion) | plan 1 | `unheaded-apt.conf:36` `access_log` with no format (default `combined`, no request time); `nginx.conf:41` same | | open |
| SSV-26 | Decision 23 | | orb's load: open gate (SS6) | plan 8 | open | | handed to plan 8 |
| SSV-27 | Scientist A5, A7 | | Second recorder, run marker | plan 8 | none | | handed to plan 8 (run controller) |
| SSV-28 | Micromanager D2 | `pkg/transport/health.go:112-116` | `/health` 200 when degraded | Stevie | 200 on DEGRADED by design | | For Stevie |

## Log

| Date (UTC) | Steps | Note |
|---|---|---|
| 2026-11-15 21:40 | 1-3 | Worktree `/home/govan/tmp/wt/S-SEE-AND-SURVIVE` (branch `sprint/s-see-and-survive`, off develop `8d35785d`); scratch `/home/govan/tmp/wt/S-SEE-AND-SURVIVE-scratch` with `lib.sh`. Main checkout left on develop (Step 2's `git switch develop` not run: it already was). |
| 2026-11-15 21:40 | 5-7 | Not asked (Stevie's word: no questions). Choices in "Decisions". |
| 2026-11-15 21:40 | 8-9 | go, docker (no sudo needed), jq, curl, python3, shellcheck, nginx, systemd-analyze, pytest: ok. promtool missing (runs from the Prometheus image in `check-prometheus-rules.sh`). |
| 2026-11-15 21:41 | 10-19 | Befores in the table. Grounding matched: 18 `no` + 5 `unless-stopped`; 12 exited. Step 12's resource grep matches 3 files, all in comments: 0 rules. |
| 2026-11-15 21:51 | 20-22 | huginn 0.0058 core over 10 min (Phase 4's after compares with this). Phase 0 gate: 28 rows, a before in each but SSV-20 (Phase 17) and SSV-23 (Step 263). |
| 2026-11-15 | lanes | One builder (this session; the lane brief forbids spawning agents). The `alerting` lane has its own worktree and branch as the plan says; the other lanes (buckets, huginn, state, backup, smoke, nginx side of vor) are committed straight on `sprint/s-see-and-survive`, one phase at a time, to keep one tree; the alerting branch is merged in at the end. vor's own commit is in vor's repository. |

## For Stevie

- **SS1, SS2, SS3, SS5, SS6, SS7** were taken on the plan's recommendation without asking (see Decisions). Nothing reaches west until Phase 12's merge and Phase 15's reloads, both after the evidence week and with the coordinator.
- **SSV-28**: `/health` answers 200 when one of two transports is down (`pkg/transport/health.go:112-116`, by design). Should `/ready` say 503 then? Not changed here.

## Blocked

| Step(s) | Why |
|---|---|
| Phases 12-19 (Steps 247-387) | Wait for the evidence week (ends 2026-11-17 18:34Z) and the coordinator. |
| The promtool half of every RULES-GATE (`scripts/check-prometheus-rules.sh`) | It runs promtool in a throwaway container from the Prometheus image; the lane brief forbids container-based gates until the evidence week ends (a container writes under `/var/lib/docker`). No promtool binary is on west. `alert-policy --check`, `-runbooks` and `tests/runbooks/test_alert_runbooks.py` run at every gate; the promtool tests written in Phases 7-8 are first run at Step 255. One run started at 22:06Z and was stopped before it finished (output none). |
