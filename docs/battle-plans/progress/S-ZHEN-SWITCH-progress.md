<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-ZHEN-SWITCH progress (ADR-147 plan 3)

Plan: `docs/battle-plans/S-ZHEN-SWITCH-battle-plan.md`. Branch `sprint/s-zhen-switch`, worktree `/home/govan/tmp/wt/S-ZHEN-SWITCH`, cut from develop `8d35785d` on 2026-11-15 21:40Z. This lane runs Phases 0 to 7 (Steps 1-237) unattended under `/home/govan/tmp/adr147-table/LANE-BRIEF.md`; Phase 8 onwards is not started.

G-EW: **closed** (2026-11-15 21:41Z). `S-E1-LIVE-progress.md` records no end of the log store week (Step 284 not closed); the week runs to 2026-11-17 18:34Z. Every step that starts a container, a throwaway Postgres server or a sudo read is pending until then.

ADR-ZS = ADR-149 (ADR-148 is taken on develop; no sibling plan writes 149).
Migrations: **051, 052, 053** (S-WASTE-OUT takes 050 for `050_app_timeline_milestones.sql`, plan Step 284). The plan's text says 050, 051, 052; the mapping is 050→051, 051→052, 052→053. For the coordinator: S-WASTE-OUT merges first, so its 050 line lands before these three in `db/migrations/MANIFEST`.
uids: 16714-16717 as written (free on `8d35785d`).

## Decisions

Stevie's word for this run: "focus only on current churn - no questions for now". Steps 5 and 6 were not asked. Each decision below that only shapes code on this branch is **taken on the plan's recommendation, Stevie to confirm**; nothing live and nothing merged rests on it.

| # | Ruling | Status |
|---|---|---|
| D1 | (a) the systemd target `unheaded-zhen.target` | unanswered, 2026-11-15; coded on the recommendation, Stevie to confirm |
| D2 | (a) database `unheaded_zhen`, two-step claim | unanswered, 2026-11-15; coded on the recommendation, Stevie to confirm |
| D3 | (a) fix the sweep, platform timer | unanswered, 2026-11-15; coded on the recommendation, Stevie to confirm |
| D4 | (a) vor outside the switch | unanswered, 2026-11-15; coded on the recommendation, Stevie to confirm |
| D5 | (a) wildcard and global always fail; private only if allowlisted | unanswered, 2026-11-15; coded on the recommendation, Stevie to confirm |
| D6 | limit 12; Zhen's set local only | unanswered, 2026-11-15; coded on the recommendation, Stevie to confirm |
| D7 | approve `runbooks/security/ioc-audit.yaml` | Stevie's; not taken. The timer ships disabled |
| D8 | the switch's state on west | live; not taken (Phase 9) |

## Findings

| id | seat and row | evidence | fix | owner | before | after | status |
|---|---|---|---|---|---|---|---|
| Z1 | Architect A7, Micromanager D11 | `cmd/akira/main.go:194,197,199`; `configs/akira-west.yaml:6-7` | defaults and the `inference` row gone | plan 3, Phase 3 | Step 10, 21:41Z: `3` defaults, `1` inference row; east's Akira named them in 240 lines in 10 min | | open |
| Z2 | Developer 1.7 | `config/dashboard/services.txt:22-29`; `health.go:778-781` | module fragment; module targets never decide the overall state | plan 3, Phase 3 | Step 11, 21:41Z: `2` Zhen rows. Step 15 (container log lines): **pending** (sudo docker logs, evidence week) | | open |
| Z3 | Developer D23, BlackMage F17 | `configs/zhen-schedules.yaml:27-33` | platform timer `unheaded-health-sweep` | plan 3, Phase 2 | Step 11: `1` job in the file. Step 12, 21:41Z: `service-health-sweep` last success 313,914 s ago (the daemon's start, 2026-11-12 06:41:09Z: never run); heartbeat 51 s; platform timers `0` | | open |
| Z4 | Micromanager D11, BlackMage F17, Developer 1.7 | `rules/zhen.yml`; `prometheus.yml:110-119`; `scrape.yml:253-257` | rules and scrape jobs in `modules/zhen`, loaded only while on | plan 3, Phase 4 | Step 11: `prometheus.yml` 5 Zhen lines, `scrape.yml` 2; `rules/zhen.yml` 4 `alert:` lines (3 rules: `ZhenSchedulerStalled` twice, `IOCAuditNotSucceeding`; the plan's "3" counted rules). Step 13, 21:41Z: group `[('zhen', 3)]`; `ZhenSchedulerStalled` firing samples over 14 d: 2,324; `up{job="zhen-agentd"}` 1 | | open |
| Z5 | Erda F8 | 10 of 13 tables in `unheaded_app` | `unheaded_zhen`, two-step claim | plan 3, Phases 7, 8B | Step 19 (G-EW closed): Erda's figures, 10 of 13 tables `zhen_*`, measured 2026-11-15; re-read at Step 268 | | open |
| Z6 | decision 15 (c), BlackMage | no check holds a bind; `-host 0.0.0.0` accepted | bind rule, static gate, guard, page | plan 3, Phase 6 | Step 14, 21:41Z: `127.0.0.1:20103`, `127.0.0.1:20110`, `127.0.0.1:20111`, `127.0.0.1:20112`, `[fd00:dead:beef::1]:20111`; no wildcard | | open |
| Z7 | Micromanager D11, Developer 1.7 | `unheaded-vor.service:2,19` | description corrected (D4 a) | plan 3, Phase 3 | Step 11: `1` | | open |
| Z8 | Developer 1.7 | `pkg/bff/nav.go:46` | entry only while on | plan 3, Phase 3 | Step 11: `1` | | open |
| Z9 | Developer 1.7 | `zhenoptin.go`; `runbooks.go:474-490` | silent and hidden while off | plan 3, Phase 3 | Step 15: **pending** (sudo journalctl and docker logs, evidence week) | | open |
| Z10 | own sweep | `Makefile:76` | `build-zhen`, out of `build-services` | plan 3, Phase 3 | Step 11: `1` | | open |
| Z11 | Architect A3, Archimedes F5 | 2 GiB huge pages for the model server | handoff to plan 4 (decision 12) | plan 4 | none (handoff) | plan 4 | handoff |
| Z12 | Developer 1.7, grounding G8 | IOC audit scheduled nowhere | platform timer, enabled on D7 | plan 3, Phases 2, 9 | Step 12, 21:41Z: no `schedule="ioc-audit"` series; timers `0` | | open |
| Z13 | grounding G8 | the sweep probes plain http against TLS | targets file, scheme per target, exit code | plan 3, Phase 2 | Step 16, 21:41Z: the runner's output stops after 7 of 18 targets (no `RESULT` line); `[OK]` trace-collector only; `[FAIL]` unheaded-daemon 000, protocol-api 000, wotan 400, timeguru 400, architect 400, captain 400; the runner still says `[SUCCESS]` (the step has no failing exit) | | open |
| Z14 | Erda F8, F6 | `app_zhen` no limit; 7 pools, 21 connections | limit 12; two pools, 8 | plan 3, Phases 7, 8B | Step 19 (G-EW closed): Erda's figures, limit -1, 3 sessions, measured 2026-11-15; re-read at Step 268 | | open |
| Z15 | Erda F8 | Zhen's rows ride the platform's set and its copy on east | excluded; own set, local, drilled | plan 3, Phases 7, 8B | Step 17, 21:41Z: `unheaded_well_backup_databases` 4, `unheaded_well_backup_rows` 1,945,324 | | open |

## Log

- 2026-11-15 21:40Z Step 2: worktree at develop `8d35785d`.
- Step 8-9: go, docker, python3, curl, ss, systemctl, shellcheck, pytest, PyYAML 6.0.2 present; promtool MISSING (the rule gate runs it in a container, which the evidence week forbids: every `scripts/check-prometheus-rules.sh` run in Phases 2, 4, 6, 7 is pending).
- Steps 10-17: readings above. Step 15 not taken (sudo; `docker logs` reads Docker's data during the evidence week).
- Step 18: G-EW closed.

## For Stevie

1. Decisions D1-D8 were not asked (no questions this run). D1-D6 are coded on the plan's recommendation on this branch only; D7 (approve `ioc-audit.yaml`) and D8 (switch state on west) wait for you.

## Blocked

- Step 15 (Z2, Z9 before): `[BLOCKED: needs Stevie / evidence week]` (sudo `docker logs`, sudo `journalctl`).
- Step 19: G-EW closed; Erda's figures used, re-read at Step 268.

## Handoffs

| Plan | What |
|---|---|
| 1 S-SEE-AND-SURVIVE | rule urgencies, `observability/alerting/policy.yaml`, the textfile ACL; any red non-Zhen row of the sweep |
| 2 S-WASTE-OUT | shared files: `cmd/akira/main.go`, `config/dashboard/services.txt`, `Makefile`, scrape configs, `docker-compose.yml`, `pkg/uids`; migration 050 is theirs |
| 4 S-WEST-GROUND | Z11 huge pages; `deploy/nftables/west.nft` 20103; backup disk rule; their database move never in the same sitting as 8B |
| 6 S-EAST-PAIR | east's Akira gets the new defaults with its next debs |
| 7 S-TENANT-DOOR | the target's units become an LXD instance |
