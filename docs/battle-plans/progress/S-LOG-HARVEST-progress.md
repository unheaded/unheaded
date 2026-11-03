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

## Stuck

- Phase 8 (Steps 179-192): waits on D7, the LAN device list. Stevie's hands.
- Runtime `[V]` steps in Phases 3, 4, 6, 9: wait on the compose stack (E1, attended).
