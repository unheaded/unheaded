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
| 1 | S-SEE-AND-SURVIVE | See and survive: alarms that reach a person, instruments that can read a load, backups | unattended code; live config on west | 1 (a stopped container alerts), 2, 4, 6, 20 (instruments), 23 (vor's access log, request time in nginx) | forged 2026-11-15 (20 phases, 387 steps; committed). 2026-11-18: code (Steps 1-246) merged to develop, `eacbd344`; block B green, 32 of 32 on `1351ee73`; live Phases 12 to 19 run unattended on `DECISION-PACK-2026-11-18.md`, from block D on (block A, the befores, running since 16:05Z) |
| 2 | S-WASTE-OUT | Waste out, each fix measured before and after | unattended; steps on Vector, ClickHouse and Postgres wait for the evidence week to end | 7, 8, 9, 10, 19, 24, timeguru; then 5's two fixes | forged 2026-11-15 (24 phases, 414 steps; finished after a cutoff, committed). 2026-11-18: code (Steps 1-304) merged to develop, `eacbd344`; block B green, its eBPF gates (Steps 273-277) included; live Phases 17 to 23 run unattended on `DECISION-PACK-2026-11-18.md`: Phase 17's befores are block A, running since 16:05Z; the deploy is block E |
| 3 | S-ZHEN-SWITCH | The platform whole without Zhen; one switch turns it on | unattended code; attended where the Well changes | 15 | forged 2026-11-15 (11 phases, 330 steps; finished after a cutoff, committed; its ADR takes the next free number, 149 since ADR-148). 2026-11-18: code (Steps 1-237) merged to develop, `eacbd344`; block B green; live Phases 8 to 10 run unattended on `DECISION-PACK-2026-11-18.md` (8A in block F, 8B and Phase 9 in block G); 8B first needs the non-uuid `trace_id` option in `scripts/well/zhen-copy-rows.sh` (pack row 15) |
| 4 | S-WEST-GROUND | West's ground: databases off the spinning disk, swap, huge pages, input filter, pid limits | ATTENDED, after the evidence week | 11, 12, 16, 22 | forged 2026-11-15 (13 phases, 324 steps; finished after a cutoff, committed `S-WEST-GROUND-battle-plan.md`). 2026-11-18: code (Phases 2 and 3) merged to develop, `eacbd344`; the compose lane stays held for Phase 7; block B green; live Phases 0, 1 and 4 to 12 are block I and run unattended on `DECISION-PACK-2026-11-18.md` (rows 1, 3, 5, 9, 10, 13); the reboot proof of Phase 11 is skipped for now and stays open (row 4) |
| 5 | S-LOG-WEEK-2 | A second, fair log store week | unattended, 7 days | 5 | forged 2026-11-17 ahead of its turn, on Stevie's word (8 phases, 96 steps; not executed). Its Phase 1 is repository work and may run beside plans 1 to 4; the week itself starts after plans 2 and 4. 2026-11-18: still not executed; its two questions inside the week are answered ahead (`DECISION-PACK-2026-11-18.md` row 12: truncate at Step 49; at Step 87 ClickHouse stays unless Postgres passes all five tests, then stop and ask). Not settled: its Step 40 wants plan 4's reboot past, and row 4 skips that reboot for now (`ADR147-LIVE-RUN-ORDER.md`, "Not settled") |
| 6 | S-EAST-PAIR | East as a second home: the collector fix, Evening 10B, its own uplink, it watches west, two wired paths with BGP carrying real routes | ATTENDED, hardware Stevie adds | 3, 17, 18 | needs a design ADR for the pair first (what fails over, and how): drafted 2026-11-17 as ADR-150 (Proposed, parked); its 15 choices are Stevie's before the forge. 2026-11-18: ADR-150 Accepted, all 15 answered (`DECISION-PACK-2026-11-18.md` round 6); forgeable when the plans before it close |
| 7 | S-TENANT-DOOR | One way in for a tenant: an LXD instance placed by Cuirass; Zhen moves into one | attended | 14 | after plans 3, 4 and 6 |
| 8 | S-PROVING-RUN | Baseline of five to seven quiet days, the rails, then the 38 conditions at 80 to 90% of real work | Stevie present for east | 13 (measured here, no numbers before), 20, 21 | after plans 1 to 7 |
| 9 | S-FINE-TUNE | Tuning, one change at a time: a first pass keeping 20% changes, a second keeping 5% | unattended between attended changes | 20 | after plan 8 |
| 10 | S-LONG-SOAK | Seven days at load; close every finding | watched | 20 | after plan 9 |
| 11 | S-TEN-NODES | Unheaded on ten or more nodes in a public cloud, before 1.0 | Stevie's cost and word | 21; ADR-143 | after plan 10 |

Plans 6 to 11 are forged when the plan before them closes, from what it measured. Forging them now would be guessing. Plan 5 reads plan 2's and plan 4's numbers at its Phase 2 and stops without them.

**The live order of plans 1 to 5** after the evidence week, with the steps that the single integration merge made stale: `docs/battle-plans/ADR147-LIVE-RUN-ORDER.md` (2026-11-17).

## Gates that stay open

- What counts as load on orb (decision 23).
- Whether a tenant may share the GPU (decision 13).
- Every number for the shares between the stack, tenants and login sessions (decision 13): an output of plan 8.
- Added 2026-11-21 (decision pack 2026-11-18, row 41, M-178.1): ADR-144's item 3 (a target footprint for a small adopter host) and item 6 (a per-component CPU and memory budget gate, checked after a soak) are outputs of plan 8's baseline, the same way the shares above are. ADR-147's "To settle" item 1 placed item 3 in this series without naming a plan. Plan 8 proposes the figures from what it measures; the target number itself is Stevie's (ADR-144: "Stevie sets it"). ADR-144 stays parked (pack row 26). No plan of its own (ADR-112).

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

**2026-11-18:** Rule 18 (east is attended) is lifted by DECISION-PACK-2026-11-18 row 5 for everything but the OS upgrade; firewall and sshd steps on east go behind a self-reverting rehearsal.
