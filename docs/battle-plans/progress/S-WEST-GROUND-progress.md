<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-WEST-GROUND progress

Plan: `docs/battle-plans/S-WEST-GROUND-battle-plan.md` (ADR-147 plan 4, 13 phases, 324 steps). Branch `sprint/s-west-ground`, worktree `~/tmp/wt/S-WEST-GROUND`, cut from develop `8d35785d` on 2026-11-15 21:41Z. Compose lane (2E): branch `sprint/s-west-ground-compose`, worktree `~/tmp/wt/S-WEST-GROUND-compose`, held unmerged until Phase 7.

**Run mode (2026-11-15, coordinator's word under Step 3):** the evidence week runs until 2026-11-17 18:34Z (`WEEK_RUNNING`). Only Step 1, Steps 12-13, Phase 2 and Phase 3, repository only. Nothing on the host is read with sudo or changed. Nothing is merged to develop: where the plan says "merge to develop", the branch stops and is marked "ready for the coordinator".

## Decisions

| # | Question | Recommendation | Ruling |
|---|---|---|---|
| WG1 | Move ClickHouse to the NVMe before plan 2's batch fix | A: move all five now | **Repository side taken on the plan's recommendation (A), Stevie to confirm**: the fstab fragment and Docker's drop-in carry all five volumes. The live move is Phase 5, attended. Step 11 (read only, 21:42Z): no `batch:` in the ClickHouse sink on develop and no S-WASTE-OUT progress file yet, so WG1 is asked at Step 17. |
| WG2 | Backup sets off the NVMe | A: `/srv/backups` a bind mount of `/var/backups/srv-backups` on `sdb` | **Taken on the plan's recommendation (A), Stevie to confirm.** Shapes only the fstab fragment line. |
| WG3 | Kanban task list and dashboard `/metrics` on the LAN | A: credential from outside loopback, bridges, east, overlay | **Taken on the plan's recommendation (A), Stevie to confirm.** Phase 3 runs; Step 85's compose line is on the held lane only. |
| WG4 | Alertmanager and Prometheus without a credential on loopback | A | not asked (Step 20, attended) |
| WG5 | West's filter before east's | A | not asked (Step 21, attended) |
| WG6 | Host or repository wins per file | host wins (sysctl: except `vm.nr_hugepages`) | not asked (Step 250) |
| WG7 | read-ahead and scheduler on `sdb` | keep 256 only on a 20% fall | not asked (Step 286) |
| WG8 | Reboot west at the end of sitting D | A | not asked (Step 292) |

## Progress table

| id | Seat and row | Evidence | Fix | Owner | Before | After | Status |
|---|---|---|---|---|---|---|---|
| W-01 | Architect A9, R1 layout 1; Erda rec 3; Archimedes rec 3 | G2, G4, G6 | database volumes on the NVMe | Phase 5 | | | not started (no before: Phase 1 waits on the week) |
| W-02 | Architect R1 "must be true first" | G5 | backup sets off the NVMe | Phase 5 | | | not started |
| W-03 | Architect A8; BlackMage F1; Archimedes F9 | G7 | swap off `sdb`, three files deleted | Phase 4 | | | not started |
| W-04 | Architect A3; Archimedes F5 | G8 | huge pages released | Phase 4 | | | not started |
| W-05 | BlackMage F9; Architect A14 | G10, G11 | input filter | Phase 6 | | | not started |
| W-06 | BlackMage F9 (task list, dashboard `/metrics`) | G12, G22 | WG3 A: source-gated routes | Phases 3, 7 | | | not started |
| W-07 | BlackMage F10 | G13 | WG4 | Phase 6 | | | not started |
| W-08 | decision 22; Archimedes 1.2 | G19 | `iperf3`, the link and WireGuard measured | Phase 9 | | | not started |
| W-09 | BlackMage F19; Architect A10 | G15 | pid limits | Phase 7 | | | not started |
| W-10 | Architect A11, R7, 4a | G16 | slices, structure only | Phase 7 | | | not started |
| W-11 | Architect A5, A12 | G8, G17 | one source for host settings, drift gate | Phases 2, 8 | | | not started |
| W-12 | Architect A16 | G18 | read-ahead and scheduler | Phase 10 | | | not started |
| W-13 | grounding G20 | G20 | startup scripts and the override | Phases 0, 2 | | | not started |

## Log

- 2026-11-15 21:41Z: Step 1 read (ADR-147 answers 1, 11, 12, 13, 16, 22; Grounding; seat rows A3, A5, A10-A16, F9, F10, F19). Step 12: worktree at develop `8d35785d`. Step 13: this file.

## For Stevie

## Blocked

- Steps 2-11, 14-24 (Phase 0 except 1, 12, 13) and Phase 1 (Steps 25-41): `[BLOCKED: needs Stevie / evidence week]` (sudo reads, live readings, rulings in the room). Phases 4-12: attended, not in this run.
