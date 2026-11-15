<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# Sprint series, 2026-11-15: ADR-147, the stack and both hosts measured, fixed, loaded and tuned

Source of every decision: `docs/adr/ADR-147-every-service-earns-its-place.md`: "The table's answers" (what seven seats found) and "Stevie's answers to the decisions" (25 answers, by number; they override every seat's recommendation). Stevie's word of 2026-11-15 lifts ADR-112's freeze for exactly these plans. New ideas found while executing become parked `Proposed` ADRs, never scope.

The seats' full reports are on west in `~/tmp/adr147-table/` (not in the repository, decision 25). A plan cites a seat's row by seat and row id (for example "Erda F1").

## What the series is for (Stevie, 2026-11-15)

- A tuned machine: the application stack and the two hosts under it. Not an interface.
- "Highly available and scalable design is the end goal with everything - build it as if we have a real production target." East and west are treated as two production servers.
- Done means fixed: every finding is implemented or has a written reason why not, and every optimization is measured before and after.
- The platform is whole without Zhen; Zhen is a module behind one switch, never reachable from the public side of a host.
- Plain writing: tables and numbers.

## The plans

| # | Plan | Stage | Mode | Decisions it carries | State |
|---|---|---|---|---|---|
| 1 | S-SEE-AND-SURVIVE | See and survive: alarms that reach a person, instruments that can read a load, backups | unattended code; live config on west | 1 (a stopped container alerts), 2, 4, 6, 20 (instruments), 23 (vor's access log, request time in nginx) | forged 2026-11-15 |
| 2 | S-WASTE-OUT | Waste out, each fix measured before and after | unattended; steps on Vector, ClickHouse and Postgres wait for the evidence week to end | 7, 8, 9, 10, 19, 24, timeguru; then 5's two fixes | forged 2026-11-15 |
| 3 | S-ZHEN-SWITCH | The platform whole without Zhen; one switch turns it on | unattended code; attended where the Well changes | 15 | forged 2026-11-15 |
| 4 | S-WEST-GROUND | West's ground: databases off the spinning disk, swap, huge pages, input filter, pid limits | ATTENDED, after the evidence week | 11, 12, 16, 22 | forged 2026-11-15 |
| 5 | S-LOG-WEEK-2 | A second, fair log store week | unattended, 7 days | 5 | after plans 2 and 4 |
| 6 | S-EAST-PAIR | East as a second home: the collector fix, Evening 10B, its own uplink, it watches west, two wired paths with BGP carrying real routes | ATTENDED, hardware Stevie adds | 3, 17, 18 | needs a design ADR for the pair first (what fails over, and how) |
| 7 | S-TENANT-DOOR | One way in for a tenant: an LXD instance placed by Cuirass; Zhen moves into one | attended | 14 | after plans 3, 4 and 6 |
| 8 | S-PROVING-RUN | Baseline of five to seven quiet days, the rails, then the 38 conditions at 80 to 90% of real work | Stevie present for east | 13 (measured here, no numbers before), 20, 21 | after plans 1 to 7 |
| 9 | S-FINE-TUNE | Tuning, one change at a time: a first pass keeping 20% changes, a second keeping 5% | unattended between attended changes | 20 | after plan 8 |
| 10 | S-LONG-SOAK | Seven days at load; close every finding | watched | 20 | after plan 9 |
| 11 | S-TEN-NODES | Unheaded on ten or more nodes in a public cloud, before 1.0 | Stevie's cost and word | 21; ADR-143 | after plan 10 |

Plans 5 to 11 are forged when the plan before them closes, from what it measured. Forging them now would be guessing.

## Gates that stay open

- What counts as load on orb (decision 23).
- Whether a tenant may share the GPU (decision 13).
- Every number for the shares between the stack, tenants and login sessions (decision 13): an output of plan 8.

## Rules every plan and every agent follows

Rules 0 to 14 of `docs/battle-plans/SPRINT-SERIES-2026-11-04.md` apply unchanged. Added for this series:

15. **Every fix carries a before and an after,** measured the same way, written in the plan's progress file as one row: id, seat and row, evidence, fix, owner, before, after, status. A fix with no before is not started.
16. **The evidence week.** Until the log store week ends (about 2026-11-17), nothing restarts or burdens Vector, ClickHouse or Postgres, and nothing stops the stack. That includes reading: no walk of `/var` or of Docker's data (`du`, `find`, `docker system df`). A walk on 2026-11-15 raised I/O pressure from 6% to 39%.
17. **Restart policy stays `no`** on west (decision 1). A plan never changes it; it makes a stopped container raise an alert instead.
18. **East is attended.** Reads over ssh are fine; any change waits for Stevie.
19. **No traffic leaves for a host Stevie does not own.** No test starts the rss daemon or the weather daemon.
20. **No numbers for the shares** (decision 13) before plan 8 has measured them.
21. **Tools.** `iperf3` from the distribution's package is approved. `fio` and `stress-ng` are not: load is made by the real applications and the repository's own generators.
22. **The interface is not the goal.** No step builds or restyles a page. A result is read from a metric or a terminal.
