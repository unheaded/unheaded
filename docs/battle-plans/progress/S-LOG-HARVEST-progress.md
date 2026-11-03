<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-LOG-HARVEST progress

Plan: `docs/battle-plans/S-LOG-HARVEST-battle-plan.md` (12 phases, 214 steps). This file holds the three sections Step 11 asks for: Decisions, Step log, Stuck.

## Decisions

Asked 2026-10-31, answered by Stevie 2026-11-03 (rapid-fire sitting; recorded from `~/tmp/next.md`).

| # | Question (short) | Answer | Effect |
|---|---|---|---|
| D1 | Keep Vector → ClickHouse as collector and store while the third-party verdicts are on hold? | **Yes, configure only** (as recommended) | Phases 3, 4, 6, 7, 9 unblocked (code and config steps; runtime `[V]` steps wait for the stack, FND-001) |
| D2 | What receives syslog from devices and the Pi-hole container? | **Native `cmd/syslog-relay`** (Go, stdlib) (as recommended) | Phase 5 goes ahead |
| D3 | How do Pi-hole's logs leave the container? | **rsyslog inside the container → relay** (as recommended) | Phase 6 unblocked |
| D4 | How does east ship? | **Install the prepared `vector-east` in a host window** (as recommended) | Phase 7 unblocked; Step 170 is Stevie's host window |
| D5 | Third-party container stdout → journald driver? | **Yes** (as recommended) | Phase 4 unblocked |
| D6 | Audit records: journald-audit socket or file source? | **File source**, auditd `log_group = adm` (as recommended) | Phase 3 audit steps unblocked |
| D7 | Network devices: which models, syslog-capable, allow 514 from their IPs? | **Stevie later** (device list not given) | **Phase 8 BLOCKED** until the list arrives |

Step 2 is done by this record. Sequencing from the same sitting: E1 stack up (attended) is the focus; this sprint's runtime steps follow it.

Related decisions recorded the same day: ADR-116 (Q1-Q6), ADR-114 (Q0-Q7), ADR-115 (Q2, Q6, Q7 → ADR-117).

## Step log

| Date | Steps | Note |
|---|---|---|
| 2026-11-03 | 2, 11 | Decisions recorded (above); this file created. Unattended churn, `churn/2026-11-03`. |
| 2026-11-03 | 13-34 | Phase 1 source inventory: `config/logging/sources.yaml` written from the live hosts (Steps 13-19 outputs in this file's gap map below), unique ids, parses with the CI YAML reader, retention classes from retention-policy-v1 (audit / operational), privacy note in the file header. |
| 2026-11-03 | 35-58 | Phase 2 static coverage gate: `pkg/logsources` (Load/Validate, six tests), `scripts/check-log-sources.sh`, meta-gate provocation `check-log-sources` (a unit file with no row), CI step in security.yml next to the log-caps gate, `docs/policy/log-sources.md`. Provoked red three ways (stray unit, stray compose service, duplicate id), each naming the culprit; gosec ratchet green; no new module (yaml.v3 was already direct). |

## Gap map (burn-down)

Sources by status on 2026-11-03 (Phase 1): **170 entries**: shipped 0, gap 58, blocked 41, excluded 71.
`blocked:FND-001` = Vector is built and configured on west but the stack is down; `blocked:D7` = LAN devices; `gap` = no path at all (east until vector-east, files on both hosts, json-file containers until Phase 4, Pi-hole until Phase 6).

Inventory facts (Steps 13-19, 2026-11-03): west runs 40 services, east 37; files written in the last 7 days outside the journal: west 14 (audit, auth, dmesg, fail2ban, frr, haproxy, kern, nginx x4, syslog, ubuntu-advantage, unattended-upgrades), east 5; haproxy and frr log to syslog (journal) and rsyslog also writes file copies; `systemd-journald-audit.socket` disabled on both (D6: file source); compose: 12 first-party services on journald, 9 third-party on json-file (D5); Pi-hole has pihole.log + FTL.log (webserver.log absent), rsyslog active in the container; LXD runs only `pihole`; LAN neighbours seen from west: router .129, laptop .132, unnamed .148 (D7). east naming drift: the deploy files `unheaded-daemon`, `unheaded-dashboard`, `unheaded-kanban` are installed on east as `unheaded-unheaded-daemon`, `unheaded-dashboard-backend`, `unheaded-kanban-app`.

Privacy (Step 28): the Pi-hole query log names every LAN client and every domain it looks up; the nginx access logs carry client IPs. Personal data under retention-policy-v1; they ship to ClickHouse on west (loopback, one database user) and nowhere else.

## Stuck

- Phase 8 (Steps 179-192): waits on D7, the LAN device list. Stevie's hands.
- Runtime `[V]` steps in Phases 3, 4, 6, 9: wait on the compose stack (E1, attended).
