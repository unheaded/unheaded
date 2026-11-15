<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-WEST-GROUND BATTLE PLAN: 13 Phases, 324 Steps

**Date**: 2026-11-15
**Sprint**: S-WEST-GROUND, plan 4 of the ADR-147 series (`docs/battle-plans/SPRINT-SERIES-2026-11-15-ADR-147.md`). West's ground put right: the databases off the spinning disk, swap that cannot stall them, 2 GiB of memory given back, an input filter, pid limits, the slices, one source for host settings.
**Prerequisite**: ADR-147 decisions 11, 12, 13, 16 and 22 (answered 2026-11-15). The log store week has ended (it started 2026-11-10 18:34Z, so not before 2026-11-17 18:34Z). Plan 1 (S-SEE-AND-SURVIVE) has delivered backups with restore drills for ClickHouse and VictoriaMetrics, and alerts for a stopped container, a memory kill, swap use and memory pressure. Phase 0 proves all of it or stops.
**Target**: The five database volumes live on a 32 GiB logical volume on the NVMe and a cache flush under them costs 3 ms or less (17.9 ms today). West's only swap is the 4 GiB file on the NVMe. No huge page is reserved. West's input policy is drop, and what decision 16 names answers loopback, the link to east and WireGuard only. Every container has a pid limit and the limits sum to under 15% of the kernel's thread limit. The slices exist and carry no number. Every host setting this plan touches has one file in the repository and a gate that fails on drift. Every row of the progress table has a before and an after.
**Estimated duration**: 8 h of repository work (Phases 2 and 3, unattended), then four attended sittings with Stevie: A (Phases 4 and 5, 2.5 h), B (Phases 6 and 7, 3 h), C (Phases 8 and 9, 2 h), D (Phases 10 to 12, 2 h of work spread over at least 14 h because Phase 10 measures six hours each side).
**Agent strategy**: Opus throughout (series 2026-11-04 rules 13 and 15: every file this plan writes ends up read or run by root on a live host). Phases 0 to 3 are the coordinator and up to three lane agents in separate worktrees. Phases 4 to 11 are one coordinator with Stevie present: no swarm, no cheaper model, one change at a time.
**Commit cadence**: every 3 to 5 steps, at every exit gate, before every skip, never inside an open firewall window or while the stack is stopped. Signed; on a gpg timeout use `--no-gpg-sign` and say so in the progress file. Named files only. Message `[PLAN S-WEST-GROUND] Steps X-Y: ...` plus the executing session's two trailers (series 2026-11-04 rule 2). Stevie pushes.
**Stuck protocol**: skip after 3x the step estimate or two failed debug attempts: mark `[STUCK]`, dependents `[BLOCKED]`, commit, go on to the next independent phase. Inside an attended window "stuck" means roll back first (each phase names how), then mark.

## Grounding (checked 2026-11-15 19:30 to 20:10Z against develop `f9b08c88`; rechecked 20:30 to 20:50Z against `1393e5ee`, whose one commit touches ADR-113 only; read only, no root, no walk of `/var`)

| # | Claim (source) | What is true now | The plan does |
|---|---|---|---|
| G1 | "One 32 GiB volume in `ubuntu-vg` (538 GiB free)" (decision 11, Architect R1) | `lsblk`: `nvme0n1p3` 928.5G is the PV, one LV of 390G. Free space needs root (`vgs`); the seats' 538.46 GiB and 578.2 GB are the same figure in two units. | Step 36 reads it as root before anything is created. |
| G2 | The five volumes (Architect 1.3) | `docker inspect`: `/var/lib/docker/volumes/unheaded-dev_postgres-data/_data`, `unheaded-dev_clickhouse-data`, `unheaded-dev_victoria-data`, `unheaded-dev_vector-data`, `monitoring_prometheus_data`. Prometheus is in the `monitoring` compose project, whose three containers are `unless-stopped`. | All five move. The monitoring project's Prometheus is stopped for the move too; Alertmanager and the mailer stay up. |
| G3 | "Undone by an unmount, the old data untouched underneath" (decision 11) | True until the stack is started on the new volume. After that the copy underneath is stale, and an unmount loses what was written since. | Rollback table in Phase 5: an unmount inside the sitting, a reverse copy after it. |
| G4 | "5 to 10 minutes of stack downtime" (ADR-147) | The stores hold about 6 GB (Erda 1.7; the Well's log table adds 0.3 GB a day). A checksum pass reads it twice. | Planned as 10 to 15 minutes. The extra is the verification. |
| G5 | **The Well's backup sets** (Architect R1 "must be true first"; `wiki/Well-Backups.md:10,14`) | **Not carried by decision 11.** The sets are in `/srv/backups/unheaded-well` on the NVMe, and `scripts/db-backup.sh:64-93` (`WELL_BACKUP_OTHER_DISK=1`, set in `deploy/systemd/unheaded-well-backup.service:25`) refuses to run when the sets and the data share a disk. After the move they would, and the first nightly backup would fail. The path is named in 36 tracked files (`git grep -l srv/backups develop`). | WG2: `/srv/backups` becomes a bind mount of a directory on `sdb`. The path in every file stays; the guard then reads "data on nvme0n1, sets on sdb". Done in the same sitting, before the stack starts on the new volume. |
| G6 | "Move ClickHouse unfixed and the NVMe inherits 26 TB a year" (Erda rec 3, Archimedes rec 3) | `config/vector/vector.yaml` sink still has no `batch` block; the fix is plan 2's and waits for the same week. NVMe: 39.1 TB written = 10% used. 71 GB a day is 0.018% of the drive a day. | Step 11 reads plan 2's state. If the batch fix is not live, WG1 asks; the recommendation is to move anyway and record the days. |
| G7 | Swap: 30 GiB on `sdb`, two unused files (decision 12) | `/proc/swaps`: `/swap.img` 4 GiB, 3.1 GiB used; `/var/swapfile4` 30 GiB, 0 used. `/etc/fstab` has both. `/var/swapfile2` (8 GiB) and `/var/swapfile3` (5 GiB) exist, dated 2026-03-15, in neither. | Phase 4. |
| G8 | Huge pages set by `/etc/sysctl.d/99-unheaded.conf`, written by `scripts/bootstrap-llm-lab.sh` (prompt, Architect A3) | True: `vm.nr_hugepages = 1024` in the file, `bootstrap-llm-lab.sh:866` inside a heredoc at lines 809-886. `HugePages_Total 1024, Free 988`. `db/postgresql.conf` does not set `huge_pages`, so Postgres runs the default `try` and falls back by itself. The live file and the heredoc already differ (a comment, a trailing duplicate line). | Phase 2B makes one file the source; Phase 4 releases the pages. |
| G9 | "No alert on swap or memory pressure; plan 1 adds them" (decision 12) | Prometheus has 85 alerting rules; none has `swap`, `pressure`, `oom` or `container` in its name. | Step 8 is a gate: Phase 4 is `[BLOCKED]` until plan 1's rules are loaded. |
| G10 | "West has no input filter" (BlackMage F9, Architect A14) | True on the host: `unheaded-nft.service` is not installed, `/usr/local/lib/unheaded/nft/` does not exist. **But the filter is written**: `deploy/nftables/west.nft` (input policy drop, Stevie's LAN scope of 2026-11-03, published ports scoped in the forward chain), the lockout-safe apply `scripts/host/nft-apply-confirm.sh` (rollback timer armed before the apply), the boot unit, and an attended procedure, S-E1-LIVE Evening 7 (Steps 136-150), not yet run. The account `unheaded-nft` (uid 969) exists. | Phase 6 runs Evening 7's procedure, with a rehearsal that is left to revert by itself first. It writes no new ruleset. S-E1-LIVE's progress file is told, so Evening 7 is not run twice. |
| G11 | "They answer the wifi LAN" (ADR-147 "Network and cluster") | Worse than stated: `ss -tln` shows 9100, 9110, 9323, 19120, 19012 and 8443 on the wildcard address, and `wlp17s0` holds a global IPv6 address. Whether the router filters inbound IPv6 is not known. | Phase 6 tests from a phone on cellular, before and after. |
| G12 | Decision 16: the kanban task list and the dashboard's `/metrics` answer loopback, east's link and WireGuard only; the fronts stay reachable | A port filter cannot do it. Ports 20000 and 20001 are published on the LAN address on purpose (`${UNHEADED_LAN_BIND}`, `docker-compose.yml:1371,1453`) and are in the 2026-11-03 LAN scope. Kanban reads are public by design (`cmd/kanban-app/auth_bff.go:27`, `OpenReads: "/"`); `/metrics` is open on every service (`pkg/auth/service_handler.go:80`). | WG3. Recommended: the two paths ask for a credential from any source outside loopback, the Docker bridges, east's link and the overlay (Phase 3). |
| G13 | "Alertmanager and Prometheus on loopback take no credential" (BlackMage F10) | True, and deliberate: `monitoring/docker-compose.yml:71,116` bind both to 127.0.0.1 because they have no auth; the admin and lifecycle APIs are off (`:83-85`). | WG4. See "What a tenant could do" below. |
| G14 | The firewall finding says west goes "after east has run clean" (`compliance/findings/register.yaml:128-134`) | East's filter is also not applied (`ssh east systemctl is-enabled unheaded-nft` → `not-found`); east is plan 6. Decision 16 puts west in this plan. | WG5: west first, with the self-reverting rehearsal on west itself standing in for the east run. |
| G15 | "Seven scopes at the default pass the thread limit" (Architect A10, BlackMage F19) | True: every container `pids.max` 17,320, `threads-max` 115,473, no `pids_limit` in any compose file. Peaks since 2026-11-12: ClickHouse 739, Akira 40, every other container 30 or fewer. | 512 for each, 2,048 for ClickHouse (BlackMage R10): 13,312 for the main file's 23 services and 2,048 for monitoring's four, 15,360 in all, 13.3% of the limit (less once plans 2 and 3 remove services). |
| G16 | Slices (Architect 4a) | Only `system.slice` and `user.slice` exist. Docker uses the systemd cgroup driver (`docker-<id>.scope` under `system.slice`). `deploy/systemd/user-1000.slice.d/50-unheaded-caps.conf` holds `TasksMax=4096`, `MemoryMax=12G` from the 2026-11-04 incident. | Four slice units with accounting on and no share set. The user slice's two existing figures are not touched: they predate decision 13 and plan 8 owns them. A slice with nothing set still has the kernel's default weight, 100: stated, not chosen. |
| G17 | "The WireGuard and bird files in the repository differ from what is deployed" (Architect A5) | Confirmed for west without root: the repository says `Address = fd00:dead:beef::1/64` and a `/48` route in PostUp (`config/wireguard/west.conf:14,19`); the interface holds `/128`. `/etc/wireguard` and `/etc/frr` are root only, so the full diff is a root step. bird is east's. | Phase 8, west's files only. East's two files are plan 6. |
| G18 | `read_ahead_kb` 8,192 on the database disk; "where it is set was not looked for" (Architect A16) | No udev rule, tmpfiles line or script sets it (`/etc/udev`, `/usr/lib/udev`, `/usr/lib/tmpfiles.d`, the bootstrap script searched). Both rotational disks on west read 8,192 and the NVMe and the SSD read 128: it is this kernel's default for a rotational device. Scheduler `mq-deadline`; `bfq` is a module, not loaded. | Phase 10, after the move, one change. |
| G19 | `iperf3` (decision 22) | Not installed on either host. Candidate `3.18-2ubuntu0.1` from the distribution on both. East's CPU reads 61.4 °C at idle against a critical mark of 70. | Phase 9, with a temperature gate at 64 °C to start and a stop at 67 °C. |
| G20 | Restart policy stays `no` (decision 1) | The policy lives in the host's untracked `docker-compose.override.yml`. `scripts/kingdom-startup.sh:160` and `kingdom-shutdown.sh:181` call `docker compose -f docker-compose.yml`, and by Compose's rule an explicit `-f` does not load the override. Not tested here: Compose needs root to read `.env` on west. `kingdom-shutdown.sh` also uses `pkill -f` (rule 14) and `compose down`. | Step 15 tests it. This plan never passes `-f` for the main project, uses `stop` and `start` (never `down`, never the two scripts), and compares the policy of every container after every start. If Step 15 shows the flip, the scripts are fixed in Phase 2 (row W-13). |
| G21 | Sibling plans | At 20:50Z `S-WASTE-OUT` and `S-ZHEN-SWITCH` are on disk (untracked); `S-SEE-AND-SURVIVE` is not. Plan 2 removes five containers and expects plan 4 to stop the stack (its line 59); plan 3 hands the huge pages to this plan (its G5). | Phase 0 reads their progress files by name; a missing file is a failed gate, not a pass. Stop and start lists are read from the running project (Steps 157, 226). |
| G22 | The fronts reach the two services as "the bridge" | Traefik (`config/traefik/dynamic.yml`) forwards `/dashboard` and `/kanban` from the LAN to the backends from a 172.28.x address, which Phase 3's list allows. It does not strip the prefix, so a LAN request arrives as `/kanban/api/...` or `/dashboard/metrics`, not under a gated path. Not measured what each backend answers for those. | Step 239 tests both through Traefik from the laptop; anything but a 404 or 401 is a finding before W-06 closes. |
| G23 | Step 11's test for plan 2's batch fix | The first version of Step 11's `awk` read on past the ClickHouse sink into `well_logs`, whose `batch:` (`config/vector/vector.yaml:486`) would have printed `BATCH_IN_REPO` today. | Fixed: the `awk` stops at the next sink. |

**What a tenant could do with the unauthenticated Alertmanager and Prometheus, and what this plan does.** A process in west's own network namespace can post a silence to `127.0.0.1:9093` that mutes every alert, post false alerts, and read every series from `127.0.0.1:9090` and `:8428`. It cannot delete series or stop Prometheus (those APIs are off). A tenant under decision 14 is an LXD instance: it has its own network namespace, cannot address west's loopback, and reaches west only on `lxdbr0`, where the input chain accepts DNS, DHCP and syslog and nothing else. So after Phase 6 a tenant cannot reach either API, and Step 202 proves it from an LXD instance. (Before the filter a tenant can reach `192.168.13.2:8428`, VictoriaMetrics' write API, because west answers on its own addresses from any interface; Steps 191 and 202 probe that too.) What stays exposed is every process on the host itself: host units and login sessions. This plan records that as an open finding with plan 7 as its owner (WG4) and does not add credentials: both programs would need them wired into Prometheus, the mailer, Grafana, `maintenance.sh` and every health check while plan 1 is rewriting the same files.

**Marshal (scope).** IN: the seven items of the prompt, plus four things grounding found that are the same work: the backup sets (G5), the startup scripts and the override (G20), the cellular IPv6 test (G11), the firewall order (G14). OUT, each with its owner: alerts, backups and the journal (plans 1 and 2; the journal stays on `sdb` by decision 11); Vector's batch and Postgres's memory limit (plan 2); anything of Zhen's, its units and slices included (plans 3 and 7); east's WireGuard and bird files, east's filter, the idle FRR daemons, BGP carrying real routes (plan 6); every number for a share (plan 8); the Architect's R11 values for dirty limits and socket buffers (plan 9, one change at a time); deleting the old copies under the mounts (Stevie, after plan 5's week); `sdc` and `sda` (not touched). No new third party except `iperf3` from the distribution (decision 22).

**Micromanager (QA / DoD).** A fix is done when its row in the progress table has a before, an after measured the same way (Appendix C, readings R1 to R8), and a status. Code is red then green under `go test -race`; new gates are registered in `scripts/check-gates-can-fail.sh` and proven to fail. Every live change has its rollback written beside it and a file to restore from. Done for the plan is Phase 12's table with no empty cell, not a green test run.

**Busboy (handoffs).** Plan 1 before this plan (Step 7). Plan 2 edits `docker-compose.yml` and `config/vector/vector.yaml`; plan 3 removes services from the same compose file: this plan's compose lane (Phase 2E) is merged last, in Phase 7, after merging develop into it, and its stop and start lists are read from the running project, never from a fixed list of names. Plan 5 (the second log store week) starts only after Phase 5 and plan 2's two fixes; Phase 12 writes the date the new layout began. Plan 6 gets the `iperf3` figures and east's half of the host-settings manifest. Plan 7 gets `tenant.slice` and finding WG4. Plan 8 gets the slices and sets their numbers. S-E1-LIVE: its log store week section is told when the week is closed (Step 14) and its Evening 7 is marked as run here (Step 211).

## Decisions

| # | Question | Options | Recommendation | Status |
|---|---|---|---|---|
| WG1 | ClickHouse writes 71 GB a day until plan 2's batch fix is live. Move it to the NVMe before that? Asked only if Step 11 finds the fix missing. | (A) move all five now and record the days; (B) wait for plan 2 | **A.** 0.018% of the drive a day; the fix is days away; decision 11 names all five. | open, Step 17 |
| WG2 | The backup sets cannot stay on the NVMe once the data is there (G5). | (A) `/srv/backups` becomes a bind mount of `/var/backups/srv-backups` on `sdb`; (B) change the path in 36 files; (C) switch the guard off | **A.** One fstab line, no path changes, undone by an unmount. C is never right. | open, Step 18 |
| WG3 | The kanban task list and the dashboard's `/metrics` on the LAN (G12). | (A) those paths need a credential from outside loopback, the bridges, east and the overlay (Phase 3, about 2 h of code); (B) take 20000 and 20001 off the LAN in `west.nft` and reach both by `ssh -L`; (C) leave both, with the reason in writing | **A.** It is what decision 16 says: the fronts stay, the two paths close. | open, Step 19 |
| WG4 | Alertmanager and Prometheus take no credential on loopback (G13). | (A) prove a tenant cannot reach them, record the rest as a finding owned by plan 7; (B) basic auth on both now | **A.** | open, Step 20 |
| WG5 | West's filter before east's (G14). | (A) west now, after a rehearsal apply that reverts by itself; (B) east first, in plan 6, and west after | **A.** | open, Step 21 |
| WG6 | Where a host file and its repository copy differ, which side wins. Asked per file in Phase 8. | host wins; repository wins | **Host wins** for WireGuard and FRR (the tunnel and the session work; changing them is plan 6's design). For sysctl the host wins except `vm.nr_hugepages` (decision 12). | open, Step 250 |
| WG7 | `read_ahead_kb` and the scheduler on `sdb`. | keep or revert the measured change; `mq-deadline` or `bfq` | Keep 256 only if reads fall 20% (decision 20). `mq-deadline` stays until plan 8 wants I/O weights. | open, Step 286 |
| WG8 | Reboot west to prove the mounts, the filter, swap and sysctls come back in order. | (A) reboot at the end of sitting D; (B) at the next planned reboot | **A.** Nothing else proves fstab ordering before Docker. | open, Step 292 |

---

## LEGEND

- `[B]`: bash command, run directly
- `[V]`: verification; must pass before going on
- `[D]`: debug step, only if the step before it failed
- `[W]`: write or create a file
- `[R]`: read or inspect a file
- `[S]`: needs sudo (Opus only)
- `[P]`: can run in parallel with other `[P]` lanes
- `[C]`: commit checkpoint (signed, named files)
- `[STEVIE]`: Stevie runs it, watches it or says yes first
- `[DECIDE]`: a ruling with options and a recommendation; ask it as one AskUserQuestion, recommended option first
- `[STUCK]` / `[BLOCKED]`: set by the Skip Protocol

`~Nm` is the expected wall-clock time. Paths: `REPO=/home/govan/tmp/unheaded`, `WT=/home/govan/tmp/wt/S-WEST-GROUND`, `WTC=/home/govan/tmp/wt/S-WEST-GROUND-compose`, `PROG=$WT/docs/battle-plans/progress/S-WEST-GROUND-progress.md`, `S=/home/govan/.local/state/unheaded/west-ground` (readings; on the NVMe, never in the repository), `W='[fd00:dead:beef::1]:9100'` (west's node_exporter instance label). Compose for the main project is always `cd /home/govan/tmp/unheaded && sudo docker compose ...` with no `-f` (G20). No step prints `.env`, a key file or a container's environment.

**The progress table.** `$PROG` holds one row per fix: id, seat and row, evidence, fix, owner, before, after, status.

| id | Seat and row | Fix | Phase |
|---|---|---|---|
| W-01 | Architect A9, R1 layout 1; Erda rec 3; Archimedes rec 3 | database volumes on the NVMe | 5 |
| W-02 | Architect R1 "must be true first" | backup sets off the NVMe | 5 |
| W-03 | Architect A8; BlackMage F1; Archimedes F9 | swap off `sdb`, three files deleted | 4 |
| W-04 | Architect A3; Archimedes F5 | huge pages released | 4 |
| W-05 | BlackMage F9; Architect A14 | input filter | 6 |
| W-06 | BlackMage F9 (task list, dashboard `/metrics`) | WG3 | 3, 7 |
| W-07 | BlackMage F10 | WG4 | 6 |
| W-08 | decision 22; Archimedes 1.2 "not known" | `iperf3`, the link and WireGuard measured | 9 |
| W-09 | BlackMage F19; Architect A10 | pid limits | 7 |
| W-10 | Architect A11, R7, 4a | slices, structure only | 7 |
| W-11 | Architect A5, A12 | one source for host settings, drift gate | 2, 8 |
| W-12 | Architect A16 | read-ahead and scheduler | 10 |
| W-13 | grounding G20 | startup scripts and the override | 0, 2 |

---

## PHASE 0: THE GATE (Steps 1-24)

**Goal**: Proof that the log store week has ended and that plan 1's backups and alerts exist; the rulings; the worktree.
**Prerequisite**: none. Nothing in this phase changes the host.
**Time**: 45 min.
**Model**: Opus. **Agent**: Coordinator, Stevie present for Steps 6 and 17 to 21.

- [ ] **Step 1** [R] ~15m: Read ADR-147 "Stevie's answers", items 1, 11, 12, 13, 16, 22; this plan's Grounding; `~/tmp/adr147-table/architect.md` sections 1.2 to 1.4, 4 (R1, R4, R5, R7) and 4a; `erda.md` 1.7 and 1.8; `blackmage.md` 1.10 and rows F1, F9, F10, F19; `docs/battle-plans/S-E1-LIVE-battle-plan.md` lines 1071-1128 and Appendix A2; `wiki/Well-Backups.md`; `scripts/host/nft-apply-confirm.sh`.
- [ ] **Step 2** [B] ~1m: The clock.
  ```bash
  test "$(date -u +%s)" -ge "$(date -u -d 2026-11-17T18:34:00Z +%s)" && echo WEEK_OVER || echo WEEK_RUNNING
  ```
- [ ] **Step 3** [V] ~1m: `WEEK_OVER`. `WEEK_RUNNING`: stop here. Only Steps 1 and 12 to 13 and Phases 2 and 3 (repository only) may go on before the week ends, and only on the coordinator's word.
- [ ] **Step 4** [B] ~1m: The week's last reading.
  ```bash
  cut -f1 /home/govan/.local/state/unheaded/S-E1-LIVE-logstore.tsv | grep -E '^2026' | sort -u | tail -1
  ```
  Expect: a time at or after `2026-11-17T18:34`. Earlier → Step 5.
- [ ] **Step 5** [D][S] ~10m: Take the closing reading (read only on both stores; it is the week's own instrument).
  ```bash
  cd /home/govan/tmp/unheaded && scripts/logstore-measure.sh; echo rc=$?
  ```
  `rc=0` and Step 4 now passes. `rc=1` with an `NA` line: record which value, go on (the week's record is S-E1-LIVE's).
- [ ] **Step 6** [STEVIE] ~2m: "The log store week is closed as it stands (decision 5). Vector, ClickHouse and Postgres may now be stopped. Yes?" A no → the plan stops; write why under "For Stevie".
- [ ] **Step 7** [R][V] ~5m: Plan 1's state.
  ```bash
  f=/home/govan/tmp/unheaded/docs/battle-plans/progress/S-SEE-AND-SURVIVE-progress.md; test -s "$f" && grep -n -i -E 'clickhouse.*backup|backup.*clickhouse|victoria.*backup|backup.*victoria|restore|swap|pressure|stopped container|oom' "$f" | cut -c1-200
  ```
  Expect: rows for a ClickHouse backup and a VictoriaMetrics backup, each with status done and an after that names a restore drill and its date; rows for the swap, memory pressure, stopped-container and memory-kill alerts with status done. A missing file or a row not done: **Phases 4 and 5 are `[BLOCKED]`** (no store moves without a backup, no swap leaves without its alert). Phases 2, 3, 6, 8 and 9 may go on.
- [ ] **Step 8** [B][V] ~1m: The alerts are loaded, not only written.
  ```bash
  curl -s -m5 127.0.0.1:9090/api/v1/rules | python3 -c 'import json,sys; n=[r["name"] for g in json.load(sys.stdin)["data"]["groups"] for r in g["rules"] if r["type"]=="alerting"]; print(len(n), sorted(x for x in n if any(k in x.lower() for k in ("swap","pressure","oom","container"))))'
  ```
  Expect: a list with at least one name each for swap, memory pressure, a stopped container and a memory kill. On 2026-11-15 this printed `85 []`. Empty → as Step 7.
- [ ] **Step 9** [S][B][V] ~2m: The Well's backup is current and its drill is not late.
  ```bash
  sudo cat /srv/backups/unheaded-well/LATEST; curl -s -m5 127.0.0.1:9090/api/v1/alerts | python3 -c 'import json,sys; print(sorted({a["labels"]["alertname"] for a in json.load(sys.stdin)["data"]["alerts"] if a["labels"]["alertname"].startswith("Well")}))'
  ```
  Expect: a set dated today or yesterday; `[]`. Else: fix the backup first (`wiki/Well-Backups.md`), `[BLOCKED]` for Phase 5.
- [ ] **Step 10** [R][V] ~5m: From plan 1's progress file and `wiki/Well-Backups.md` (plan 1 writes its backup facts to the wiki), record in `$S/backups.txt` (created in Step 25; hold the values until then): for ClickHouse and for VictoriaMetrics, the command or unit that takes a backup now, and the directory the sets land in. Expect: two commands, two directories. Else: `[BLOCKED]` for Phase 5.
- [ ] **Step 11** [B] ~1m: Plan 2's batch fix.
  ```bash
  cd /home/govan/tmp/unheaded && awk '/^  clickhouse:/{f=1;next} f&&/^  [a-z_]+:$/{exit} f&&/batch:/{print "BATCH_IN_REPO"; exit}' config/vector/vector.yaml; grep -n -i 'batch' docs/battle-plans/progress/S-WASTE-OUT-progress.md 2>/dev/null | cut -c1-160 | head -5
  ```
  `BATCH_IN_REPO` and a done row with an after → WG1 is not asked. Else → Step 17.
- [ ] **Step 12** [B] ~1m: Worktree off develop.
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && git worktree add /home/govan/tmp/wt/S-WEST-GROUND -b sprint/s-west-ground develop && git -C /home/govan/tmp/wt/S-WEST-GROUND log --oneline -1
  ```
- [ ] **Step 13** [W] ~8m: Create `$PROG`: header; "Decisions" (WG1 to WG8, empty rulings); the progress table with the thirteen rows above and the columns id, seat and row, evidence, fix, owner, before, after, status; "Log"; "For Stevie".
- [ ] **Step 14** [W] ~2m: In `$WT/docs/battle-plans/progress/S-E1-LIVE-progress.md`, append to the log store week section: `- **<date>: the week is closed as it stands (ADR-147 decision 5, Stevie's word in S-WEST-GROUND Step 6).** Last reading <time from Step 4>. The stores may be stopped from here.`
- [ ] **Step 15** [S][V] ~2m: Does an explicit `-f` drop the host's override (G20)? Counts only; nothing is printed from the rendered file.
  ```bash
  cd /home/govan/tmp/unheaded && echo "plain: $(sudo docker compose config 2>/dev/null | grep -c 'restart: "no"')  with -f: $(sudo docker compose -f docker-compose.yml config 2>/dev/null | grep -c 'restart: "no"')"
  ```
  Expect: `plain:` 18 or the number of services the override names. `with -f:` the same number → W-13 is closed as "not a defect", note it. `with -f: 0` → W-13 is real: Steps 56 to 59 run, and "For Stevie" gets: "`kingdom-startup.sh` recreated containers with `unless-stopped` whenever it recreated any".
- [ ] **Step 16** [B] ~1m: Record every container's restart policy; every later start is compared with this file.
  ```bash
  mkdir -p /home/govan/.local/state/unheaded/west-ground && docker ps -a --filter name=unheaded --format '{{.Names}}' | sort | while read -r c; do echo "$c $(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' "$c")"; done | tee /home/govan/.local/state/unheaded/west-ground/restart-policy.before | awk '{print $2}' | sort | uniq -c
  ```
  Expect (2026-11-15): 18 `no`, 5 `unless-stopped`.
- [ ] **Step 17** [STEVIE][DECIDE] ~3m: **WG1**, only if Step 11 said so. Recommended A. B → Phase 5 moves four volumes and leaves ClickHouse; its two fstab lines and its copy are skipped and a row "ClickHouse waits on plan 2" goes under "For Stevie".
- [ ] **Step 18** [STEVIE][DECIDE] ~3m: **WG2.** Recommended A. B → `[STUCK]` for W-02 and Phase 5 is `[BLOCKED]` until the 36 files are changed by a plan of their own. C is not offered as a choice; it is listed so the refusal is on record.
- [ ] **Step 19** [STEVIE][DECIDE] ~3m: **WG3.** Recommended A. A → Phase 3 runs. B → Phase 3 is skipped and Step 97 edits `west.nft`. C → write Stevie's reason in W-06's row, status "accepted".
- [ ] **Step 20** [STEVIE][DECIDE] ~2m: **WG4.** Recommended A. B → `[BLOCKED]` here, a row under "For Stevie" for plan 1's owner; Step 202's proof still runs.
- [ ] **Step 21** [STEVIE][DECIDE] ~2m: **WG5.** Recommended A. B → Phase 6 is `[BLOCKED]` until plan 6 has applied east's filter; W-05 and W-07 stay open.
- [ ] **Step 22** [B] ~1m: Tools (never install a missing one here).
  ```bash
  for t in rsync lvcreate mkfs.ext4 nft bpftrace smartctl findmnt systemd-run python3 curl shellcheck go; do command -v $t >/dev/null && echo "$t ok" || echo "$t MISSING"; done; test -x /snap/bin/lxc && echo "lxc ok"
  ```
- [ ] **Step 23** [V] ~1m: All ok. `bpftrace` missing → reading R3 is skipped in both the before and the after, and W-01's "sync callers" cell says so. Any other missing → `[STUCK]`.
- [ ] **Step 24** [C][V] ~2m: **PHASE 0 EXIT GATE**: the week is over by the clock, by a reading and by Stevie's word; Steps 7 to 10 passed or the blocked phases are marked; WG2 to WG5 are ruled; `$PROG` exists.
  ```bash
  cd /home/govan/tmp/wt/S-WEST-GROUND && git add docs/battle-plans/progress/S-WEST-GROUND-progress.md docs/battle-plans/progress/S-E1-LIVE-progress.md && git commit -S -m "[PLAN S-WEST-GROUND] Steps 1-24: the gate, the rulings, the progress table"
  ```

---

## PHASE 1: THE "BEFORE" READINGS (Steps 25-41)

**Goal**: Every reading the fixes will be judged by, taken the same way they will be taken after (Appendix C).
**Prerequisite**: Phase 0 gate with `WEEK_OVER`. West quiet: no build, no backup running.
**Time**: 45 min (two readings wait ten minutes).
**Model**: Opus. **Agent**: Coordinator, sudo for three steps.

- [ ] **Step 25** [B] ~1m: `mkdir -p /home/govan/.local/state/unheaded/west-ground && cd $_ && date -u +%FT%TZ | tee before.start`. Write Step 10's two commands and two directories to `backups.txt` here.
- [ ] **Step 26** [B] ~11m: **R1**, flush time per device, ten minutes (Appendix C). Output to `before.R1`.
- [ ] **Step 27** [V] ~1m: An `sdb` line with `ms_per_flush` between 12 and 25 and `per_s` between 5 and 10 (seats: 17.9 ms, 7.9 a second). An `nvme0n1` line under 3 ms. Outside those ranges: something else is loading the disk; find it (`cat /proc/pressure/io`), wait, repeat once, then record what was read.
- [ ] **Step 28** [B] ~1m: **R2**, I/O pressure now and the six-hour mean. Output to `before.R2`.
- [ ] **Step 29** [S][B] ~1m: **R3**, who calls sync, ten seconds. Output to `before.R3`.
- [ ] **Step 30** [V] ~1m: The callers include VictoriaMetrics (`vm_`-prefixed or `victoria-metric` threads), `postgres` and `vector` (seat: 41 calls in 10 s, all from those three). `Operation not permitted` or no output → `sudo sysctl kernel.unprivileged_bpf_disabled` is not the cause for root; record "R3 not available" and go on.
- [ ] **Step 31** [B] ~11m: **R4**, per store: bytes written to `sdb`, I/O stall and limit hits, two reads ten minutes apart. Output to `before.R4`.
- [ ] **Step 32** [B] ~1m: **R5**, memory, swap, free space. Output to `before.R5`.
- [ ] **Step 33** [B] ~1m: **R6**, pid limits and peaks. Output to `before.R6`.
- [ ] **Step 34** [B] ~1m: **R7**, listeners beyond loopback. Output to `before.R7`.
- [ ] **Step 35** [S][B] ~1m: The volume group and the firewall tables.
  ```bash
  sudo vgs --units g -o vg_name,vg_size,vg_free ubuntu-vg; sudo lvs -o lv_name,lv_size ubuntu-vg; sudo nft list tables; sudo nft list ruleset | grep -c 'hook input'
  ```
- [ ] **Step 36** [V] ~1m: `vg_free` at least 100g (seats: 538). One LV, `ubuntu-lv`. No `unheaded-data`. Tables: no `inet unheaded` (only `inet unheaded_chaos_guard` and Docker's and LXD's). `unheaded-data` already exists → someone started this plan: read `$PROG`, do not create a second.
- [ ] **Step 37** [S][B] ~3m: Size of each store from its own counters (no walk).
  ```bash
  sudo docker exec -i unheaded-postgres sh -c 'psql -X -q -At -U "$POSTGRES_USER" -d postgres -c "select sum(pg_database_size(datname)) from pg_database"'
  sudo docker exec unheaded-clickhouse sh -c 'clickhouse-client --user unheaded --password "$CLICKHOUSE_PASSWORD" -q "select sum(bytes_on_disk) from system.parts"'
  curl -s -m5 127.0.0.1:8428/metrics | awk '/^vm_data_size_bytes/{s+=$2} END{printf "victoria %d\n", s}'
  curl -s -m5 -G 127.0.0.1:9090/api/v1/query --data-urlencode 'query=prometheus_tsdb_storage_blocks_bytes + prometheus_tsdb_wal_storage_size_bytes + prometheus_tsdb_head_chunks_storage_size_bytes' | python3 -c 'import json,sys; print("prometheus", json.load(sys.stdin)["data"]["result"][0]["value"][1])'
  curl -s -m5 http://172.17.0.1:9598/metrics | awk '/^vector_buffer_byte_size/{s+=$2} END{printf "vector %d\n", s}'
  ```
- [ ] **Step 38** [V] ~1m: Five numbers; their sum under 16 GiB (17,179,869,184), half the volume. Expected about 6 to 8 GB. Over → stop: `[STEVIE][DECIDE]` a larger volume (64 GiB) before Phase 5; write the five numbers under "For Stevie".
- [ ] **Step 39** [S][B] ~1m: The NVMe's wear, for W-01's row and WG1.
  ```bash
  sudo smartctl -A /dev/nvme0n1 | grep -E 'Percentage Used|Data Units Written|Unsafe Shutdowns|Temperature:'
  ```
- [ ] **Step 40** [W] ~8m: Fill the "before" cell of W-01, W-03, W-04, W-05, W-09 and W-12 in `$PROG` from `before.R1` to `before.R7` and Steps 35 to 39: numbers and the time of the reading, no prose.
- [ ] **Step 41** [C][V] ~1m: **PHASE 1 EXIT GATE**: `ls /home/govan/.local/state/unheaded/west-ground/before.R{1,2,4,5,6,7}` lists six files, none empty; six before cells filled. Commit `$PROG`: "Steps 25-41: the before readings".

---

## PHASE 2: THE REPOSITORY SIDE (Steps 42-103)

**Goal**: Every file the sittings will install exists in the repository, with its gate, and is on develop: the mounts, the sysctl source, the slices, the host-settings gate with its timer and alert, the pid limits (held on a lane until Phase 7), the ruleset checked.
**Prerequisite**: Phase 0 Steps 12, 13, 15 and 19. No host change in this phase: it may run before the week ends.
**Time**: 6 h.
**Model**: Opus (rule 13: these files are read or run by root on west). **Agent**: lanes 2A+2B+2C (one agent), 2D (one agent), 2E (one agent, its own worktree) are `[P]`; 2F and the merge are the coordinator's.

### 2A: The mounts (W-01, W-02)

- [ ] **Step 42** [W] ~5m: `$WT/deploy/host-settings/west-fstab.fragment` (new directory). Comment header: what it is, that `scripts/check-host-settings.sh` requires every line of it in west's `/etc/fstab`, and that the bind lines are appended only with the stack stopped (Phase 5). Then exactly these lines (a line for a volume WG1 excluded is left out; WG2's line only on ruling A):
  ```
  /dev/ubuntu-vg/unheaded-data /srv/unheaded-data ext4 defaults,nofail,x-systemd.device-timeout=30s 0 2
  /srv/unheaded-data/postgres /var/lib/docker/volumes/unheaded-dev_postgres-data/_data none bind,nofail,x-systemd.requires-mounts-for=/srv/unheaded-data 0 0
  /srv/unheaded-data/clickhouse /var/lib/docker/volumes/unheaded-dev_clickhouse-data/_data none bind,nofail,x-systemd.requires-mounts-for=/srv/unheaded-data 0 0
  /srv/unheaded-data/victoria /var/lib/docker/volumes/unheaded-dev_victoria-data/_data none bind,nofail,x-systemd.requires-mounts-for=/srv/unheaded-data 0 0
  /srv/unheaded-data/prometheus /var/lib/docker/volumes/monitoring_prometheus_data/_data none bind,nofail,x-systemd.requires-mounts-for=/srv/unheaded-data 0 0
  /srv/unheaded-data/vector /var/lib/docker/volumes/unheaded-dev_vector-data/_data none bind,nofail,x-systemd.requires-mounts-for=/srv/unheaded-data 0 0
  /var/backups/srv-backups /srv/backups none bind,nofail,x-systemd.requires-mounts-for=/var 0 0
  ```
  `nofail` so a lost volume leaves west bootable with sshd up; the next file is what keeps Docker from starting on the stale copies underneath. The LV line and the `/srv/backups` line are appended in Phase 5 before the stack stops; the five data binds only while it is stopped.
- [ ] **Step 43** [W] ~3m: `$WT/deploy/systemd/docker.service.d/10-unheaded-data.conf` (new directory): SPDX header, a comment ("Docker does not start unless the five database mounts are in place: without this a failed volume would let the stores start on the old copies underneath"), then
  ```
  [Unit]
  RequiresMountsFor=/var/lib/docker/volumes/unheaded-dev_postgres-data/_data /var/lib/docker/volumes/unheaded-dev_clickhouse-data/_data /var/lib/docker/volumes/unheaded-dev_victoria-data/_data /var/lib/docker/volumes/monitoring_prometheus_data/_data /var/lib/docker/volumes/unheaded-dev_vector-data/_data
  ```
- [ ] **Step 44** [B][V] ~1m: Six fields on every line; five data binds.
  ```bash
  cd /home/govan/tmp/wt/S-WEST-GROUND && awk 'NF && $1 !~ /^#/ && NF!=6 {bad=1; print "BAD: " $0} END{exit bad}' deploy/host-settings/west-fstab.fragment && grep -c '^/srv/unheaded-data/' deploy/host-settings/west-fstab.fragment && for t in $(awk '$1 ~ /^\/srv\/unheaded-data\// {print $2}' deploy/host-settings/west-fstab.fragment); do grep -q -- "$t" deploy/systemd/docker.service.d/10-unheaded-data.conf || echo "MISSING $t"; done
  ```
  Expect: `5` (4 under WG1 ruling B) and no `BAD`, no `MISSING`.
- [ ] **Step 45** [C] ~1m: **COMMIT** both files: "Steps 42-45: fstab fragment and Docker's mount requirement for the database volume".

### 2B: One sysctl file (W-04, W-11)

- [ ] **Step 46** [B] ~1m: The live file becomes the source (it is world-readable; no secret in it).
  ```bash
  mkdir -p /home/govan/tmp/wt/S-WEST-GROUND/deploy/sysctl.d && cp /etc/sysctl.d/99-unheaded.conf /home/govan/tmp/wt/S-WEST-GROUND/deploy/sysctl.d/99-unheaded-west.conf
  ```
- [ ] **Step 47** [W] ~5m: Three edits in `deploy/sysctl.d/99-unheaded-west.conf`, nothing else:
  - line `# Target: LLM Lab Server (AMD GPU, heavy inference workloads)` → `# Host: west. Source: deploy/sysctl.d/99-unheaded-west.conf; scripts/check-host-settings.sh fails on drift.` and an SPDX line above the banner;
  - the two lines `# Huge pages for GPU/model loading` and `vm.nr_hugepages = 1024` → `# Huge pages: none reserved (ADR-147 decision 12: 1.93 GiB of 2 GiB sat idle). Postgres runs huge_pages=try and falls back.` and `vm.nr_hugepages = 0`;
  - delete the last line of the file, the third `net.ipv4.ip_forward = 1` (a duplicate after the bridge lines).
  Every other value stays as west runs it. The Architect's R11 values are plan 9's, one at a time.
- [ ] **Step 48** [V] ~1m: `diff /etc/sysctl.d/99-unheaded.conf /home/govan/tmp/wt/S-WEST-GROUND/deploy/sysctl.d/99-unheaded-west.conf | grep -c '^[<>]'` prints 7 or 8 (the banner, the two huge page lines each way, the duplicate, the SPDX line). More → an edit went too far: redo from Step 46.
- [ ] **Step 49** [W] ~10m: `scripts/bootstrap-llm-lab.sh`: replace lines 809-886 (from `    cat > /etc/sysctl.d/99-unheaded.conf << 'SYSCTL'` through the line `SYSCTL`) with one line that installs the repository file: `    install -m 0644 "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/deploy/sysctl.d/99-unheaded-west.conf" /etc/sysctl.d/99-unheaded.conf`. If the script already defines its own directory variable (`grep -n 'SCRIPT_DIR=\|REPO_ROOT=' scripts/bootstrap-llm-lab.sh`), use that instead. Line 907's fallback `sysctl -p /etc/sysctl.d/99-unheaded.conf` stays.
- [ ] **Step 50** [B][V] ~2m: `cd /home/govan/tmp/wt/S-WEST-GROUND && bash -n scripts/bootstrap-llm-lab.sh && shellcheck -S error scripts/bootstrap-llm-lab.sh; grep -c 'nr_hugepages' scripts/bootstrap-llm-lab.sh`. Expect: no output from the first two, then `0`.
- [ ] **Step 51** [C] ~1m: **COMMIT** `deploy/sysctl.d/99-unheaded-west.conf scripts/bootstrap-llm-lab.sh`: "Steps 46-51: west's sysctls have one source; no huge pages reserved".

### 2C: The slices, structure only (W-10)

- [ ] **Step 52** [W] ~10m: Four files in `$WT/deploy/systemd/`, each with an SPDX header, a comment "Structure only (ADR-147 decision 13): no weight, floor or ceiling until plan 8 has measured them. Accounting is on so that it can.", a `[Unit]` with `Description=` and `Documentation=` naming ADR-147, and exactly this `[Slice]` section:
  ```
  [Slice]
  CPUAccounting=yes
  MemoryAccounting=yes
  IOAccounting=yes
  TasksAccounting=yes
  ```
  - `unheaded.slice`: "Unheaded: the stack";
  - `unheaded-host.slice`: "Unheaded: what runs on every host";
  - `unheaded-env.slice`: "Unheaded: what runs once per environment (the containers, wiki, kev-poller)";
  - `tenant.slice`: "Tenants: programs that are not Unheaded. Empty until plan 7."
  Login sessions already have `user.slice`; nothing is written for it.
- [ ] **Step 53** [W] ~3m: Two drop-ins in a new directory `$WT/deploy/systemd/slice-dropins/`: `unheaded-host.conf` containing `[Service]` and `Slice=unheaded-host.slice`; `unheaded-env.conf` containing `[Service]` and `Slice=unheaded-env.slice`. Each with the SPDX header and one comment line naming the units Phase 7 installs it for.
- [ ] **Step 54** [B] ~2m: `cd /home/govan/tmp/wt/S-WEST-GROUND && for f in unheaded unheaded-host unheaded-env tenant; do systemd-analyze verify --man=no deploy/systemd/$f.slice 2>&1 | grep -F "$f.slice"; done; grep -c -E '^(CPUWeight|CPUQuota|MemoryMin|MemoryLow|MemoryHigh|MemoryMax|MemorySwapMax|IOWeight|TasksMax)=' deploy/systemd/*.slice`
- [ ] **Step 55** [V] ~1m: No line from `systemd-analyze`; every count `0`. Then **COMMIT** the six files: "Steps 52-55: the slices, with no number set".

### 2B': The startup scripts (W-13, only if Step 15 showed the flip)

- [ ] **Step 56** [R] ~3m: `scripts/kingdom-startup.sh` lines 85-92 and 153-170; `scripts/kingdom-shutdown.sh` lines 172-184.
- [ ] **Step 57** [W] ~8m: In both scripts replace `docker compose -f "$COMPOSE_FILE"` with `docker compose --project-directory "$PROJECT_ROOT" -f "$COMPOSE_FILE" -f "$OVERRIDE_FILE"` when `$OVERRIDE_FILE` (`$PROJECT_ROOT/docker-compose.override.yml`) exists, and leave it as it is when it does not. Define `OVERRIDE_FILE` beside `COMPOSE_FILE`. Add a comment: "An explicit -f does not load the override; west's restart policy lives there (ADR-147 decision 1)." Do not touch the `pkill -f` lines: this plan does not use the scripts, and their rewrite is a row under "For Stevie".
- [ ] **Step 58** [B][V] ~2m: `bash -n` and `shellcheck -S error` on both: no output. `grep -c 'OVERRIDE_FILE' scripts/kingdom-startup.sh scripts/kingdom-shutdown.sh`: 3 or more in each.
- [ ] **Step 59** [C] ~1m: **COMMIT** both scripts: "Steps 56-59: the startup and shutdown scripts load the host's compose override".

### 2D: The host-settings gate (W-11) [P]

- [ ] **Step 60** [R] ~10m: `scripts/check-compose-publish-bind.sh` and `scripts/test-check-compose-publish-bind.sh` (the house shape for a gate and its test); `scripts/check-gates-can-fail.sh` lines 1-80, `provoke_unit_ceilings` and the registration line 1402; `deploy/systemd/unheaded-well-offsite-check.service` (a root-adjacent oneshot that writes a textfile metric); `monitoring/prometheus/rules/well.yml` and `monitoring/prometheus/rules/tests/`.
- [ ] **Step 61** [W] ~20m: Red. `scripts/test-check-host-settings.sh`: builds a fake root and a fake repository under `mktemp -d`, with a manifest of one row per mode, and asserts:
  1. identical files → exit 0, every line starts `OK`;
  2. one byte changed in the `exact` file → exit 1, a line `DRIFT<TAB>exact<TAB><path>`;
  3. a `contains` row whose host file lacks one line of the repository file → exit 1;
  4. a `redact` row whose two sides differ only after `PrivateKey = `, in a `neighbor 192.0.2.1 password` value and in comment lines → exit 0;
  5. a `redact` row with a different `AllowedIPs` → exit 1;
  6. neither the string used as the fake private key nor the fake password ever appears in the gate's output in cases 4 and 5;
  7. an `absent` row whose host path exists → exit 1;
  8. a host file made unreadable (`chmod 000`, test skipped when run as root) → exit 3 and a `SKIP` line; with `--allow-skip` → exit 0;
  9. a manifest line with two fields → exit 2;
  10. the self-check (no arguments) fails when the sysctl file says `vm.nr_hugepages = 1024`, and fails when a `.slice` file in the manifest carries `MemoryLow=3G` with a message naming decision 13.
- [ ] **Step 62** [B] ~1m: `cd /home/govan/tmp/wt/S-WEST-GROUND && bash scripts/test-check-host-settings.sh; echo rc=$?`
- [ ] **Step 63** [V] ~1m: Red: the gate script does not exist, `rc` not 0.
- [ ] **Step 64** [W] ~8m: `deploy/host-settings/west.tsv`: SPDX and a comment naming the columns `host_path<TAB>repo_path<TAB>mode`, then:
  ```
  /etc/sysctl.d/99-unheaded.conf	deploy/sysctl.d/99-unheaded-west.conf	exact
  /etc/fstab	deploy/host-settings/west-fstab.fragment	contains
  /etc/systemd/system/docker.service.d/10-unheaded-data.conf	deploy/systemd/docker.service.d/10-unheaded-data.conf	exact
  /etc/systemd/system/unheaded.slice	deploy/systemd/unheaded.slice	exact
  /etc/systemd/system/unheaded-host.slice	deploy/systemd/unheaded-host.slice	exact
  /etc/systemd/system/unheaded-env.slice	deploy/systemd/unheaded-env.slice	exact
  /etc/systemd/system/tenant.slice	deploy/systemd/tenant.slice	exact
  /usr/local/lib/unheaded/nft/west.nft	deploy/nftables/west.nft	exact
  /etc/wireguard/wg0.conf	config/wireguard/west.conf	redact
  /etc/frr/frr.conf	routing/frr/west.conf	exact
  /etc/frr/daemons	routing/frr/daemons	exact
  /var/swapfile2	-	absent
  /var/swapfile3	-	absent
  /var/swapfile4	-	absent
  ```
- [ ] **Step 65** [W] ~40m: Green. `scripts/check-host-settings.sh`, `set -euo pipefail`, SPDX, a header that says what it is for.
  - `check-host-settings.sh` (no arguments): the repository self-check. Every manifest under `deploy/host-settings/*.tsv` is well formed (three tab-separated fields, a known mode); every `repo_path` other than `-` exists; `deploy/sysctl.d/99-unheaded-west.conf` contains the line `vm.nr_hugepages = 0`; no `.slice` file named in a manifest matches `^(CPUWeight|CPUQuota|MemoryMin|MemoryLow|MemoryHigh|MemoryMax|MemorySwapMax|IOWeight|TasksMax)=` (message: "ADR-147 decision 13: plan 8 sets the shares"). Exit 0 or 1; a missing manifest directory is exit 2, never a pass.
  - `check-host-settings.sh --host NAME [--repo DIR] [--root DIR] [--textfile DIR] [--allow-skip]`: for each row of `deploy/host-settings/NAME.tsv`, compare `DIR(root)+host_path` with `DIR(repo)/repo_path`. `exact`: `cmp -s`. `contains`: every line of the repository file that is not blank and does not start with `#` is a whole line of the host file. `redact`: both sides through `sed -E 's/^([[:space:]]*(PrivateKey|PresharedKey|PublicKey))[[:space:]]*=.*/\1 = <redacted>/; s/(password)[[:space:]]+[^[:space:]]+/\1 <redacted>/'` and then `grep -v -E '^[[:space:]]*(#|!|$)'` (comments and blank lines dropped: the repository copy keeps its header, the host file does not), then compared. `absent`: the host path does not exist. One output line per row: `OK`, `DRIFT` or `SKIP` (the host file exists and cannot be read), a tab, the mode, a tab, the host path. **It never prints a line of any file.**
  - Runtime rows, only with `--host` and without `--root`: every `key = value` of the `exact` sysctl row equals `sysctl -n key` with runs of whitespace folded (`DRIFT<TAB>sysctl<TAB>key`); for every `bind` line of the `contains` fstab row, `findmnt -no SOURCE --mountpoint <target>` is not empty (`DRIFT<TAB>mount<TAB>target`); `awk 'NR>1{print $1}' /proc/swaps` is exactly `/swap.img` (`DRIFT<TAB>swap<TAB>...`).
  - Exit: 2 on usage or a malformed manifest; else 1 if any `DRIFT`; else 3 if any `SKIP` and no `--allow-skip`; else 0.
  - `--textfile DIR`: write `DIR/unheaded_host_settings.prom` through a temporary file in `DIR`, `chmod 0644` (huginn reads it as its own user; `mktemp` makes 0600), and `mv`: `unheaded_host_settings_drift_files{host="NAME"} N`, `unheaded_host_settings_skipped_files{host="NAME"} N`, `unheaded_host_settings_checked_timestamp_seconds{host="NAME"} T`. Written whatever the exit code.
- [ ] **Step 66** [B] ~2m: `cd /home/govan/tmp/wt/S-WEST-GROUND && bash scripts/test-check-host-settings.sh; echo rc=$?; shellcheck -S warning scripts/check-host-settings.sh scripts/test-check-host-settings.sh; scripts/check-host-settings.sh; echo self=$?`
- [ ] **Step 67** [V] ~1m: `rc=0`, ten cases pass; shellcheck silent; `self=0`.
  - Fail → Step 68.
- [ ] **Step 68** [D] ~10m: Read the first failing case. A key string in the output (case 6) is the one that must never be waved through: find the `echo` or `diff` that printed file content and remove it. Two tries, then `[STUCK]`; Phase 8 is then done by hand with the `sed` and `diff` lines of Step 249 and W-11 stays open.
- [ ] **Step 69** [W] ~8m: Register the provocation in `scripts/check-gates-can-fail.sh`: a function `provoke_host_settings` modelled on `provoke_unit_ceilings` that backs up `deploy/sysctl.d/99-unheaded-west.conf` and runs `sed -i 's/^vm.nr_hugepages = 0$/vm.nr_hugepages = 1024/'` on it, with the same `cmp` guard; and the line `check-host-settings|provoke_host_settings|fast|vm.nr_hugepages set back to 1024 in west's sysctl source` beside line 1402.
- [ ] **Step 70** [C] ~1m: **COMMIT** `scripts/check-host-settings.sh scripts/test-check-host-settings.sh deploy/host-settings/west.tsv scripts/check-gates-can-fail.sh` (the meta-gate needs a clean tree): "Steps 60-70: a gate that fails when a host setting drifts from its file".
- [ ] **Step 71** [B][V] ~2m: `cd /home/govan/tmp/wt/S-WEST-GROUND && ./scripts/check-gates-can-fail.sh --only check-host-settings 2>&1 | tail -3`. Expect: the gate failed when provoked, tree restored, PASS. Else: the provocation changed nothing or the self-check ignores the line: fix, amend with a new commit, rerun. Two tries.
- [ ] **Step 72** [W] ~15m: `deploy/systemd/unheaded-host-settings.service` and `.timer`. Service: `Type=oneshot`; `ExecStart=/usr/local/lib/unheaded/check-host-settings.sh --host west --repo /home/govan/tmp/unheaded --textfile /var/lib/unheaded/textfile`; `SuccessExitStatus=1 3` (drift is a reading: the metric and the alert carry it); runs as root because `/etc/wireguard` and `/etc/frr` are root only, with `CapabilityBoundingSet=CAP_DAC_READ_SEARCH`, `NoNewPrivileges=yes`, `ProtectSystem=strict`, `ProtectHome=read-only`, `ReadWritePaths=/var/lib/unheaded/textfile`, `PrivateTmp=yes`, `PrivateDevices=yes`, `ProtectKernelModules=yes`, `RestrictAddressFamilies=AF_UNIX`, `MemoryMax=64M`, `TasksMax=32`. Header comment: the install lines, and "the script is a root-owned copy; the checkout is read as data only". Timer: `OnBootSec=10min`, `OnCalendar=daily`, `Persistent=true`, `WantedBy=timers.target`.
- [ ] **Step 73** [W] ~15m: `monitoring/prometheus/rules/host-settings.yml` (new, house header: producer, how it reaches Prometheus, why) with group `host-settings`:
  - `HostSettingsDrift`: `unheaded_host_settings_drift_files > 0`, `for: 30m`, `severity: warning`, `urgency: ticket`, summary "{{ $value }} host setting(s) on {{ $labels.host }} differ from the repository";
  - `HostSettingsCheckStale`: `time() - unheaded_host_settings_checked_timestamp_seconds > 172800`, `for: 1h`, same labels;
  - both with `runbook_url: "runbooks/infra/host-settings-drift.yaml"`.
  And `monitoring/prometheus/rules/tests/host-settings_test.yml`: drift 0 for 2 h → quiet; drift 2 for 40 m → `HostSettingsDrift` fires with `host="west"`; a timestamp 3 days old → `HostSettingsCheckStale` fires; a fresh timestamp → quiet.
- [ ] **Step 74** [W] ~15m: `runbooks/infra/host-settings-drift.yaml`, `status: draft`, in the shape of `runbooks/infra/storage-health.yaml`: step 1 runs `sudo /usr/local/lib/unheaded/check-host-settings.sh --host west --repo /home/govan/tmp/unheaded` and reads the `DRIFT` lines; step 2 says, per mode, how to see the difference without printing a key (the `sed` line for `redact`); step 3: decide which side is right, change that side, rerun. No step changes a file by itself.
- [ ] **Step 75** [B] ~5m: `cd /home/govan/tmp/wt/S-WEST-GROUND && scripts/check-prometheus-rules.sh 2>&1 | tail -5; PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin pytest -q tests/runbooks 2>&1 | tail -2; scripts/check-runbooks.sh 2>&1 | tail -2; scripts/runbook-manifest.sh --write 2>&1 | tail -1; scripts/check-spdx.sh 2>&1 | tail -1`
- [ ] **Step 76** [V] ~1m: Rules pass with the four new test cases; runbook tests pass; the manifest is rewritten (the seal is Stevie's: note it under "For Stevie"); SPDX passes.
  - Fail → Step 77.
- [ ] **Step 77** [D] ~10m: `check-prometheus-rules.sh` names what it wants (a runbook that exists, a label, a policy row in `observability/alerting/policy.yaml`). Give it exactly that for these two alerts and nothing else; if plan 1 has changed the policy format, follow plan 1's newest rule file as the model. Two tries, then `[STUCK]`: the gate and timer still ship, the alert goes under "For Stevie".
- [ ] **Step 78** [C] ~1m: **COMMIT** the two units, the rule file, its test, the runbook and `runbooks/MANIFEST.sha256`: "Steps 72-78: the drift check on a timer, its metric, its alert and runbook".

### 2E: Pid limits and the cgroup parent, on a lane (W-09, W-10, W-06) [P]

- [ ] **Step 79** [B] ~1m: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-WEST-GROUND-compose -b sprint/s-west-ground-compose develop`. This branch is **not** merged before Phase 7: once on develop, the next `compose up` by anyone would apply it unattended.
- [ ] **Step 80** [W] ~15m: Red. `scripts/test-check-compose-pids.sh`: with a temporary compose file, asserts that `scripts/check-compose-pids.sh FILE` exits 0 when every service has `pids_limit` between 1 and 4096 and `cgroup_parent: unheaded-env.slice`; exits 1 naming the service when one lacks `pids_limit`; exits 1 when one has `pids_limit: -1` or `0`; exits 1 when one lacks `cgroup_parent`; exits 1 when the sum of all `pids_limit` in the file passes 20,000; exits 2 on a file with no services (never a vacuous pass). YAML merge keys (`<<: *ground`) count.
- [ ] **Step 81** [B][V] ~1m: `cd /home/govan/tmp/wt/S-WEST-GROUND-compose && bash scripts/test-check-compose-pids.sh; echo rc=$?` → red (`check-compose-pids.sh` missing).
- [ ] **Step 82** [W] ~15m: Green. `scripts/check-compose-pids.sh`: with no argument it checks `docker-compose.yml` and `monitoring/docker-compose.yml`; it parses with `python3` and `yaml.safe_load` the way `scripts/check-compose-hardening.sh` does (PyYAML resolves merge keys); rules as in Step 80. Header: BlackMage F19, Architect A10, and the arithmetic (`kernel.threads-max` 115,473 on west).
- [ ] **Step 83** [W] ~20m: `docker-compose.yml`. After the `x-logging-journald` block (line 44 and its body) add:
  ```yaml
  # S-WEST-GROUND (ADR-147; BlackMage F19, Architect A10, A11). Every container
  # gets a pid ceiling (the default was systemd's 17,320, and seven of those
  # pass the kernel's thread limit) and runs under unheaded-env.slice. The
  # slice carries no weight or limit: plan 8 measures them (decision 13).
  x-ground: &ground
    pids_limit: 512
    cgroup_parent: unheaded-env.slice
  ```
  Then, as the first key of **every** service under `services:` (find them with `awk '/^services:/{f=1;next} f&&/^  [a-z][a-z0-9-]*:$/{print NR": "$0}' docker-compose.yml`; on 2026-11-15: traefik, volume-init, victoria, postgres, clickhouse, clickhouse-schema, vector, grafana, coredns, wotan, timeguru, captain, architect, micromanager, akira, monad, sophia, dashboard-backend, kanban-app, cuirass, auth, logstore-writer, sword; fewer once plans 2 and 3 have removed services), add the line `    <<: *ground`. In `clickhouse` add, directly under it, `    pids_limit: 2048   # 739 threads at idle (2026-11-15)`.
- [ ] **Step 84** [W] ~5m: `monitoring/docker-compose.yml`: the same `x-ground` block above `services:` (anchors do not cross files) and `    <<: *ground` as the first key of every service there (volume-init, prometheus, alertmanager, alert-mailer).
- [ ] **Step 85** [W] ~3m: Only under WG3 ruling A. In `docker-compose.yml`, in the `environment:` of `dashboard-backend` and of `kanban-app`, add: `AUTH_ANONYMOUS_FROM: "${AUTH_ANONYMOUS_FROM:-127.0.0.0/8,::1/128,172.16.0.0/12,192.168.13.0/30,fd00:dead:beef::/48,fd00:d0c0::/32}"   # ADR-147 decision 16: /metrics and the task list need a credential from anywhere else`. The two IPv6 prefixes are the overlay and Docker's (`docker-compose.yml:100`, `monitoring/docker-compose.yml:30`), not all of `fd00::/8`: a router that hands the LAN a ULA prefix would otherwise open both paths. Use the file's own form for environment entries (list or map: match the neighbours).
- [ ] **Step 86** [B] ~3m: `cd /home/govan/tmp/wt/S-WEST-GROUND-compose && bash scripts/test-check-compose-pids.sh && scripts/check-compose-pids.sh; echo pids=$?; for g in scripts/check-compose-hardening.sh scripts/check-compose-log-caps.sh scripts/check-compose-publish-bind.sh scripts/check-compose-bind-nesting.sh scripts/check-image-digests.sh; do $g >/dev/null 2>&1 && echo "ok $g" || echo "FAIL $g"; done; go test -count=1 ./pkg/uids/ 2>&1 | tail -1; python3 -c 'import yaml,sys; d=yaml.safe_load(open("docker-compose.yml")); print(sum(int(s.get("pids_limit",0)) for s in d["services"].values()), len(d["services"]))'`
- [ ] **Step 87** [V] ~1m: `pids=0`; five `ok`; `ok unheaded/pkg/uids`; the two numbers `SUM N` satisfy SUM = 512 x (N - 1) + 2,048 (13,312 and 23 on 2026-11-15; fewer once plans 2 and 3 have removed services). A `FAIL` on a gate that also fails on develop is not this lane's: prove it with the same command in `/home/govan/tmp/unheaded` and note it.
- [ ] **Step 88** [W] ~5m: Register `check-compose-pids|provoke_compose_pids|fast|pids_limit removed from the ground anchor` with a `provoke_compose_pids` that deletes the line `  pids_limit: 512` from `docker-compose.yml` (same shape as `provoke_compose_hardening`).
- [ ] **Step 89** [C] ~1m: **COMMIT** on the lane: `scripts/check-compose-pids.sh scripts/test-check-compose-pids.sh scripts/check-gates-can-fail.sh docker-compose.yml monitoring/docker-compose.yml`: "Steps 79-89: a pid limit and a slice for every container (held for Phase 7)".
- [ ] **Step 90** [B][V] ~2m: `cd /home/govan/tmp/wt/S-WEST-GROUND-compose && ./scripts/check-gates-can-fail.sh --only check-compose-pids 2>&1 | tail -3` → the gate failed when provoked, PASS.
- [ ] **Step 91** [B] ~2m: Who reads a container's cgroup by its old path? `cd /home/govan/tmp/wt/S-WEST-GROUND-compose && git merge -q --no-edit develop 2>&1 | tail -1; grep -rn --include='*.go' --include='*.sh' --include='*.py' --include='*.yml' --include='*.yaml' --include='*.toml' -F 'system.slice/docker-' . | grep -v '^./docs/\|^./wiki/' | cut -c1-160`
- [ ] **Step 92** [V] ~1m: No hit (none on 2026-11-15). A hit (plan 1's per-cgroup series is the likely one) → Step 93.
- [ ] **Step 93** [D] ~15m: Change that reader to find the scope wherever it is: from `/proc/<pid>/cgroup` of the container's main pid, or by globbing `/sys/fs/cgroup/**/docker-<id>.scope`, with a test that passes for both `system.slice/` and `unheaded.slice/unheaded-env.slice/`. If the reader is plan 1's and not yet merged, write the requirement under "For Stevie" and in plan 1's progress file, and make Step 231 check it live.
- [ ] **Step 94** [C][V] ~1m: **LANE 2E EXIT**: the lane's tree is committed, gates green, **not merged**. `git -C /home/govan/tmp/unheaded branch --contains sprint/s-west-ground-compose | grep -c develop` prints 0.

### 2F: The ruleset and the merge

- [ ] **Step 95** [R] ~3m: `$WT/deploy/nftables/west.nft` lines 37-60. Decision 16 against it, port by port: 9100, 9110, 9323 and 19120 are not in the LAN set (closed); they are accepted from `docker0` and `br-*`, from east on `enp16s0`, and from `wg0` (open where decision 16 says). Nothing to edit for those.
- [ ] **Step 96** [W] ~3m: `$PROG` "For Stevie": "`west.nft`'s LAN set still lists 16666 (doom-bridge, moved out 2026-11-04, nothing listens) and 20103 (Zhen's UI: decision 15 keeps it off a public side; the house LAN is not public). Left as you set them on 2026-11-03; say if either should go."
- [ ] **Step 97** [W] ~5m: Only under WG3 ruling B. In `west.nft`: in the input line `iifname "wlp17s0" ip saddr 192.168.69.128/25 tcp dport { 7375, 80, 443, 18888, 16666, 20000, 20001, 20002, 20103 } accept` remove `20000, 20001, `; in the forward line `ct original proto-dst { 53, 3000, 3001, 16666, 20000, 20001, 20002, 20103 } accept` remove `20000, 20001, `; extend the comment above the input line: "20000 and 20001 left the LAN on 2026-11-<day> (ADR-147 decision 16): reach them with ssh -L."
- [ ] **Step 98** [B][V] ~2m: `cd /home/govan/tmp/wt/S-WEST-GROUND && GOMAXPROCS=4 go test -count=1 ./deploy/nftables/ 2>&1 | tail -1; unshare -Urn nft -c -f deploy/nftables/west.nft && echo NFT_OK`. Expect: `ok`, `NFT_OK` (a private namespace: no root, nothing changed). Else two tries, `[STUCK]`, Phase 6 `[BLOCKED]`.
- [ ] **Step 99** [C] ~1m: **COMMIT** `$PROG` and, under ruling B, `deploy/nftables/west.nft`.
- [ ] **Step 100** [B] ~10m: Merge develop into the sprint worktree and run the gates there (rule 15).
  ```bash
  cd /home/govan/tmp/wt/S-WEST-GROUND && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-west-ground' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-west-ground' || { git merge --abort; echo MERGE_CONFLICT; false; }; }; } && scripts/check-host-settings.sh && bash scripts/test-check-host-settings.sh >/dev/null && scripts/check-prometheus-rules.sh >/dev/null 2>&1 && scripts/check-spdx.sh >/dev/null 2>&1 && GOMAXPROCS=4 go test -count=1 ./deploy/nftables/ >/dev/null && echo SPRINT_GATE_OK
  ```
- [ ] **Step 101** [V] ~1m: `SPRINT_GATE_OK`. `MERGE_CONFLICT` → `[STUCK]`, no merge (rule 15's exceptions aside).
- [ ] **Step 102** [C] ~3m: **MERGE A** to develop, guarded.
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-west-ground -m '[PLAN S-WEST-GROUND] Phase 2: the repository side (mounts, sysctl source, slices, host-settings gate)' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-west-ground -m '[PLAN S-WEST-GROUND] Phase 2: the repository side (mounts, sysctl source, slices, host-settings gate)'; }; }; git log -1 --format='%h %G? %s' | cut -c1-80
  ```
  Add the session's two trailers with further `-m` arguments. Expect a hash, `G`, the subject. `N` → unsigned: say so in `$PROG`.
- [ ] **Step 103** [V] ~1m: **PHASE 2 EXIT GATE**: on develop, `ls deploy/host-settings/west.tsv deploy/host-settings/west-fstab.fragment deploy/sysctl.d/99-unheaded-west.conf deploy/systemd/unheaded.slice deploy/systemd/docker.service.d/10-unheaded-data.conf scripts/check-host-settings.sh` lists six files; `grep -c 'x-ground' docker-compose.yml` on develop prints 0 (the lane is held).

---

## PHASE 3: TWO PATHS THAT ASK WHO IS CALLING (Steps 104-122)

**Goal**: With `AUTH_ANONYMOUS_FROM` set, a caller outside the listed networks gets no open route under the paths a service names: kanban's `/api` and `/metrics`, the dashboard's `/metrics`. Unset, nothing changes.
**Prerequisite**: WG3 ruling A. Under B or C this phase is skipped whole.
**Time**: 2.5 h.
**Model**: Opus (authentication logic). **Agent**: one Developer agent in `$WT`, after Merge A.

- [ ] **Step 104** [R] ~10m: `pkg/auth/service_handler.go` (all), `pkg/auth/auth.go:190-220` (`SkipAuthPaths`), `pkg/auth/service_handler_test.go`, `LoadServiceAuthConfig` (find it: `grep -n 'func LoadServiceAuthConfig' pkg/auth/*.go`), `cmd/kanban-app/auth_bff.go`, `cmd/dashboard-backend/internal/server/auth_bff.go:60-100`.
- [ ] **Step 105** [W] ~25m: Red. In `pkg/auth/service_handler_test.go`, `TestSourceGatedOpenRoutes`, a table over `httptest` requests with `RemoteAddr` set by hand, auth on, one test authenticator that accepts the header `Authorization: Bearer good`:
  | AnonymousFrom | SourceGated | OpenReads | Request | RemoteAddr | Credential | Want |
  |---|---|---|---|---|---|---|
  | empty | `/metrics` | none | GET `/metrics` | `192.168.69.50:1234` | none | 200 (as today) |
  | `127.0.0.0/8, 172.16.0.0/12` | `/metrics` | none | GET `/metrics` | `172.28.1.9:40000` | none | 200 |
  | same | `/metrics` | none | GET `/metrics` | `192.168.69.50:1234` | none | 401 |
  | same | `/metrics` | none | GET `/metrics` | `192.168.69.50:1234` | good | 200 |
  | same | `/metrics` | none | GET `/health` | `192.168.69.50:1234` | none | 200 (not gated) |
  | same | `/api` | `/` | GET `/api/v1/tasks` | `192.168.69.50:1234` | none | 401 |
  | same | `/api` | `/` | GET `/` | `192.168.69.50:1234` | none | 200 (the page loads so a person can log in) |
  | same | `/api` | `/` | GET `/api/v1/tasks` | `127.0.0.1:5555` | none | 200 |
  | same | `/metrics` | none | GET `/metrics` | `[::ffff:172.28.1.9]:40000` | none | 200 (mapped address unwrapped) |
  | same | `/metrics` | none | GET `/metrics` | `not-an-address` | none | 401 |
  | same | `/metrics` | none | GET `/metrics` with `X-Forwarded-For: 127.0.0.1` | `192.168.69.50:1234` | none | 401 (the header is never read) |
  And `TestParseAnonymousFrom`: `"127.0.0.0/8, fd00::/8"` → two prefixes; `""` → nil, no error; `"10.0.0.0/33"` → error; `"127.0.0.1"` (no length) → error.
- [ ] **Step 106** [B] ~1m: `cd /home/govan/tmp/wt/S-WEST-GROUND && go test -count=1 ./pkg/auth -run 'TestSourceGatedOpenRoutes|TestParseAnonymousFrom' 2>&1 | tail -4`
- [ ] **Step 107** [V] ~1m: Red: `unknown field AnonymousFrom` or `undefined: ParseAnonymousFrom`.
- [ ] **Step 108** [W] ~30m: Green, in `pkg/auth`:
  - `ServiceRoutes` gains `AnonymousFrom []netip.Prefix` ("callers that may use the routes in SourceGated without a credential; empty: everyone") and `SourceGated []string` ("path prefixes, matched like OpenPaths, that are open only to AnonymousFrom");
  - `func ParseAnonymousFrom(s string) ([]netip.Prefix, error)`: comma-separated CIDRs, spaces trimmed, `netip.ParsePrefix` on each, an empty string is nil;
  - `ServiceAuthConfig` gains `AnonymousFrom []netip.Prefix`, filled by `LoadServiceAuthConfig` from `AUTH_ANONYMOUS_FROM`; a value that does not parse must make `NewServiceChain` return an error (the service refuses to start), in whatever way that function already reports a bad setting;
  - in `NewServiceChain`'s handler, before the open-path checks: `if len(routes.AnonymousFrom) > 0 && canonicalPath(r.URL) && underAny(r.URL.Path, routes.SourceGated) && !remoteIn(r.RemoteAddr, routes.AnonymousFrom) { strict.ServeHTTP(w, r); return }`, where `strict` is the authentication middleware **without** `SkipAuthPaths` wrapped round `rbac(h)`;
  - `func remoteIn(remoteAddr string, allow []netip.Prefix) bool`: `netip.ParseAddrPort`, `Unmap()`, false on any error. `r.RemoteAddr` only.
- [ ] **Step 109** [B] ~2m: `cd /home/govan/tmp/wt/S-WEST-GROUND && gofmt -l pkg/auth; go vet ./pkg/auth && go test -race -count=1 ./pkg/auth/... 2>&1 | tail -3`
- [ ] **Step 110** [V] ~1m: gofmt prints nothing; vet clean; `ok`. Fail → read the first failing row; a 200 where 401 is wanted on `/metrics` means `strict` still has the skip list. Two tries, `[STUCK]`.
- [ ] **Step 111** [C] ~1m: **COMMIT** `pkg/auth/service_handler.go pkg/auth/service_handler_test.go` and the file holding `LoadServiceAuthConfig`: "Steps 104-111: open routes can be limited to named networks (off unless AUTH_ANONYMOUS_FROM is set)".
- [ ] **Step 112** [W] ~15m: Red. `cmd/kanban-app/auth_bff_test.go`: `TestTaskListNeedsCredentialFromOutside` (auth on, `AnonymousFrom` = `127.0.0.0/8`): `GET /api/v1/tasks` from `192.168.69.50:1` → 401; from `127.0.0.1:1` → 200; `GET /` from `192.168.69.50:1` → 200; `GET /metrics` from `192.168.69.50:1` → 401. In the dashboard's server package, beside `auth_bff.go`: `TestMetricsNeedsCredentialFromOutside`: `GET /metrics` from `192.168.69.50:1` → 401, from `172.28.2.5:1` (with `172.16.0.0/12` allowed) → 200.
- [ ] **Step 113** [B][V] ~2m: `go test -count=1 ./cmd/kanban-app/ ./cmd/dashboard-backend/... -run 'NeedsCredentialFromOutside' 2>&1 | tail -4` → red (200 where 401 is wanted).
- [ ] **Step 114** [W] ~10m: Green. `cmd/kanban-app/auth_bff.go:27`: after `routes := ...` add `routes.AnonymousFrom = cfg.AnonymousFrom` and `routes.SourceGated = []string{"/api", "/metrics"}`, and change the file's header comment "Reads stay public (The Meta Moment)" to "The page is public; with AUTH_ANONYMOUS_FROM set, reads under /api need a credential from outside those networks (ADR-147 decision 16)". `cmd/dashboard-backend/internal/server/auth_bff.go:79`: in the `auth.ServiceRoutes{` literal add `AnonymousFrom: cfg.AnonymousFrom, SourceGated: []string{"/metrics"},`.
- [ ] **Step 115** [B] ~8m: `cd /home/govan/tmp/wt/S-WEST-GROUND && gofmt -l cmd/kanban-app cmd/dashboard-backend pkg/auth; GOMAXPROCS=4 go vet ./pkg/auth/... ./cmd/kanban-app/... ./cmd/dashboard-backend/... && GOMAXPROCS=4 go test -race -count=1 ./pkg/auth/... ./cmd/kanban-app/... ./cmd/dashboard-backend/... 2>&1 | tail -5; ./scripts/check-gosec-ratchet.sh 2>&1 | tail -1; scripts/check-netip.sh 2>&1 | tail -1; scripts/compliance/check-service-auth.sh 2>&1 | tail -1`
- [ ] **Step 116** [V] ~1m: All `ok` and PASS.
  - Fail → Step 117.
- [ ] **Step 117** [D] ~10m: An existing kanban or dashboard test that now gets 401 was calling from a non-loopback `RemoteAddr` with the variable set in its environment: the default must stay "unset means everyone", so fix the code path, never the old test. Two tries, `[STUCK]`: W-06 stays open, Step 85's line is removed from the lane.
- [ ] **Step 118** [W] ~8m: `wiki/Security.md`: one paragraph under the listeners or authentication heading: what `AUTH_ANONYMOUS_FROM` does, its compose default, that `r.RemoteAddr` is the only input, and that east's host units leave it unset. `CLAUDE.md`, "Authentication Framework", after the sentence "`/health`, `/ready`, `/metrics` stay open.": add "With `AUTH_ANONYMOUS_FROM` (CIDRs) set, a service's `SourceGated` routes, `/metrics` among them, are open only to those networks (ADR-147 decision 16)."
- [ ] **Step 119** [C] ~1m: **COMMIT** the two `auth_bff.go` files, the two test files, `wiki/Security.md`, `CLAUDE.md`: "Steps 112-119: kanban's /api and both services' /metrics ask for a credential from outside the listed networks".
- [ ] **Step 120** [B] ~10m: Steps 100 to 102 again for this phase's commits (merge develop in, the Step 115 gates, then the guarded merge to develop with subject `[PLAN S-WEST-GROUND] Phase 3: source-gated open routes`). The code is inert until Step 85's line reaches develop in Phase 7.
- [ ] **Step 121** [V] ~1m: `git -C /home/govan/tmp/unheaded log -1 --format='%h %G? %s' | cut -c1-80` shows the merge.
- [ ] **Step 122** [V] ~1m: **PHASE 3 EXIT GATE**: on develop, `go test -count=1 ./pkg/auth -run TestSourceGatedOpenRoutes` is `ok`; the compose file on develop does not yet name `AUTH_ANONYMOUS_FROM`.

---

## PHASE 4: SWAP OFF THE DATABASE DISK, HUGE PAGES RELEASED (Steps 123-144)

**Goal**: West's only active swap is `/swap.img` on the NVMe; `/var/swapfile4` is out of use and out of fstab; the two unused files are gone; no huge page is reserved (W-03, W-04).
**Prerequisite**: Phase 0 gate with `WEEK_OVER` and Steps 7 and 8 passed (the swap and memory-pressure alerts are loaded: no swap leaves without its alert); Phase 1 gate; Merge A on develop (Step 102). Sitting A: Stevie at the desk, west's console in reach.
**Time**: 50 min, of which 30 are a quiet watch that overlaps Phase 5's Steps 145 to 155.
**Model**: Opus. **Agent**: Coordinator with Stevie.

**Rollback for this phase.** Swap: `sudo swapon /var/swapfile4` and restore the fstab backup of Step 125; the file stays on disk until Phase 11 proves a boot without it. Huge pages: `sudo sysctl -w vm.nr_hugepages=1024` and copy back `/var/backups/west-ground/99-unheaded.conf.before`. The two unused swap files cannot be restored, only recreated; they are deleted on Stevie's word only (Step 133).

- [ ] **Step 123** [STEVIE] ~2m: "Sitting A: swap off the database disk, huge pages released, then the stack stopped for 10 to 15 minutes for the move. You are at the desk with the console? Yes?" A no → stop; nothing in Phases 4 and 5 runs unattended.
- [ ] **Step 124** [S][B] ~2m: The console path, proven before any fstab change.
  ```bash
  sudo passwd -S root | awk '{print "root:", $2}'; grep -E '^GRUB_TIMEOUT(_STYLE)?=' /etc/default/grub; test -d /sys/firmware/efi && echo UEFI
  ```
  Expect (2026-11-15): `root: L` (locked) or `root: P`, `GRUB_TIMEOUT_STYLE=hidden`, `GRUB_TIMEOUT=0`, `UEFI`. With `L`, emergency mode cannot open a shell: the way in is the GRUB menu (Esc during boot) and `init=/bin/bash` (Appendix A, E1). Write the three values to `/home/govan/.local/state/unheaded/west-ground/console.txt`. Stevie looks at the console and sees a login prompt.
- [ ] **Step 125** [S][B] ~1m: First fstab backup; the parser's baseline.
  ```bash
  TS=$(date -u +%Y%m%dT%H%M%SZ); sudo cp -p /etc/fstab /etc/fstab.west-ground.$TS && echo /etc/fstab.west-ground.$TS | tee -a /home/govan/.local/state/unheaded/west-ground/fstab.backups; sudo findmnt --verify 2>&1 | tail -1
  ```
  Expect a path, then a summary line `0 parse errors, 0 errors, N warnings`. Record N; every later `findmnt --verify` must print the same or fewer.
- [ ] **Step 126** [B] ~1m: Before.
  ```bash
  cat /proc/swaps; grep -E '^(MemAvailable|SwapFree|HugePages_(Total|Free|Rsvd|Surp)):' /proc/meminfo | tee /home/govan/.local/state/unheaded/west-ground/phase4.before
  ```
- [ ] **Step 127** [V] ~1m: `/var/swapfile4` shows `Used` 0 (2026-11-15: 0), or less than MemAvailable minus 2 GiB. Otherwise → Step 128; else → Step 129.
- [ ] **Step 128** [D] ~10m: Wait ten minutes for a quieter moment and read again. Still short of room → `[STUCK]`, Phase 4 `[BLOCKED]`: never `swapoff` with less free memory than is in use.
- [ ] **Step 129** [S][B] ~2m: Out of use.
  ```bash
  sudo swapoff /var/swapfile4 && cat /proc/swaps
  ```
  **Rollback:** `sudo swapon /var/swapfile4`
- [ ] **Step 130** [V] ~1m: `/proc/swaps` lists `/swap.img` and nothing else.
- [ ] **Step 131** [S][W] ~2m: Out of fstab.
  ```bash
  sudo sed -i '\#^/var/swapfile4[[:space:]]\+none[[:space:]]\+swap[[:space:]]#d' /etc/fstab && grep -c swapfile /etc/fstab; sudo findmnt --verify 2>&1 | tail -1; sudo systemctl daemon-reload && systemctl list-units --all --type=swap --no-legend --plain | awk '{print $1, $3}'
  ```
  Expect `0`; Step 125's summary; `swap.img.swap active` only.
  **Rollback:** `sudo cp -p "$(tail -1 /home/govan/.local/state/unheaded/west-ground/fstab.backups)" /etc/fstab && sudo systemctl daemon-reload && sudo swapon /var/swapfile4`
- [ ] **Step 132** [V] ~1m: The other two are unused: `grep -c 'swapfile[23]' /proc/swaps /etc/fstab; ls -l --time-style=+%F /var/swapfile2 /var/swapfile3`. Expect `0` and `0`; both files dated 2026-03-15, 8 GiB and 5 GiB. Anything else → `[STUCK]`, do not delete.
- [ ] **Step 133** [STEVIE] ~1m: "Delete `/var/swapfile2` and `/var/swapfile3` (13 GiB, in no fstab and no swap list since 2026-03-15; decision 12)? They can be recreated, not restored." A no → W-03 records them as kept.
- [ ] **Step 134** [S][B] ~1m: `sudo rm /var/swapfile2 /var/swapfile3; ls /var/swapfile2 /var/swapfile3 2>&1 | grep -c 'No such'` → `2`. `/var/swapfile4` stays on disk, inactive, until Step 305.
- [ ] **Step 135** [C] ~1m: **COMMIT** `$PROG` (Log: the times of Steps 129 to 134): "Steps 123-135: swap off the database disk".
- [ ] **Step 136** [S][B] ~1m: What Postgres asks for, and who holds pages.
  ```bash
  sudo docker exec unheaded-postgres sh -c 'psql -X -q -At -U "$POSTGRES_USER" -d postgres -c "show huge_pages"'; grep -E '^HugePages_(Total|Free):' /proc/meminfo
  ```
  Expect `try`; `1024` and about `988`.
- [ ] **Step 137** [V] ~1m: `try` (or `off`) → Step 139. `on` → Step 138.
- [ ] **Step 138** [D] ~5m: With `on`, Postgres refuses to start without pages, and Phase 5 restarts it. Stop: W-04 `[STUCK]`, "For Stevie": "`huge_pages = on` is set somewhere (`db/postgresql.conf`, the command line or `ALTER SYSTEM`); decision 12 waits for it to be `try`". Phase 5 may still run: it does not need W-04.
- [ ] **Step 139** [S][B] ~2m: Keep the live file, install the source.
  ```bash
  sudo install -d -m 0750 /var/backups/west-ground && sudo cp -p /etc/sysctl.d/99-unheaded.conf /var/backups/west-ground/99-unheaded.conf.before && sudo install -m 0644 -o root -g root /home/govan/tmp/unheaded/deploy/sysctl.d/99-unheaded-west.conf /etc/sysctl.d/99-unheaded.conf && diff /var/backups/west-ground/99-unheaded.conf.before /etc/sysctl.d/99-unheaded.conf | grep -c '^[<>]'
  ```
  Expect 7 or 8 (the same count as Step 48). More → someone changed the live file since Step 46: restore the backup, `[STUCK]`.
- [ ] **Step 140** [S][B] ~1m: One value, at run time.
  ```bash
  sudo sysctl -w vm.nr_hugepages=0; grep -E '^(MemAvailable|HugePages_(Total|Free|Surp)):' /proc/meminfo
  ```
  Expect `HugePages_Total` equal to the pages Postgres holds (about 36), `Free 0`, `Surp` the same as `Total`; MemAvailable about 1.9 GiB above Step 126. The last pages go when Postgres restarts in Phase 5 (Step 177).
  **Rollback:** `sudo sysctl -w vm.nr_hugepages=1024 && sudo cp -p /var/backups/west-ground/99-unheaded.conf.before /etc/sysctl.d/99-unheaded.conf` (on fragmented memory fewer than 1,024 may come back: record `HugePages_Total`).
- [ ] **Step 141** [V] ~1m: Nothing else moved: every key in the installed file equals the running value.
  ```bash
  grep -E '^[a-z]' /etc/sysctl.d/99-unheaded.conf | while IFS='=' read -r k v; do k=$(echo $k); v=$(echo $v); l=$(sysctl -n "$k" 2>/dev/null | tr -s '[:space:]' ' '); [ "$l" = "$(echo "$v" | tr -s '[:space:]' ' ')" ] || echo "DIFF $k"; done; echo checked
  ```
  Expect `checked` and no `DIFF`.
- [ ] **Step 142** [B] ~31m: Thirty quiet minutes (Phase 5's Steps 145 to 155 run meanwhile; they stop nothing). Then:
  ```bash
  cat /proc/pressure/memory; sudo journalctl -k --since '-35 min' --no-pager | grep -c -i -E 'out of memory|oom-kill'
  ```
  Expect `full avg300` under 0.5 and `0`. A kill → Appendix A, E8.
- [ ] **Step 143** [W] ~5m: Take R5 (Appendix C) to `after.R5.phase4`. In `$PROG`: W-03 after: "active swap `/swap.img` only; `swapfile4` out of use and fstab, file kept until Step 305; `swapfile2`/`3` deleted (or kept, Step 133)", status "done but Step 305". W-04 after: `HugePages_Total` and MemAvailable before and after, status "done when Step 177 reads 0".
- [ ] **Step 144** [C][V] ~1m: **PHASE 4 EXIT GATE**: `awk 'NR>1{print $1}' /proc/swaps` prints `/swap.img` only; `grep -c swapfile /etc/fstab` is 0; `sysctl -n vm.nr_hugepages` is 0; no kill in Step 142. Commit `$PROG`: "Steps 136-144: huge pages released; the swap and memory readings after".

---

## PHASE 5: THE DATABASES ON THE NVMe (Steps 145-186)

**Goal**: The five database volumes are bind mounts of a 32 GiB ext4 LV on the NVMe; the Well's backup sets live on `sdb` at the same path; Docker cannot start without the mounts; the stack runs as before with the same restart policies (W-01, W-02).
**Prerequisite**: Phase 0 Steps 7 to 10 passed (ClickHouse and VictoriaMetrics backups with restore drills; the Well's set current); WG1 and WG2 ruled; Step 38's sum under 16 GiB; Merge A on develop (the fragment and the drop-in). Sitting A, after Phase 4's Step 141.
**Time**: 1.5 h; the stack is stopped 10 to 15 minutes (Steps 158 to 166).
**Model**: Opus. **Agent**: Coordinator with Stevie.

**Rollback table** (read it with Stevie at Step 145; Appendix A, E3 and E4 are the procedures).

| Point reached | State | Rollback |
|---|---|---|
| before Step 158 | LV made, sets copied, nothing of the stores touched | `sudo umount /srv/backups /srv/unheaded-data`; restore the fstab backup of Step 149; `sudo lvremove -y ubuntu-vg/unheaded-data` |
| Steps 158 to 165 | stack stopped, copies made, binds mounted | E3 steps 2 to 5: drop-in out, unmount the binds, fstab back, start from the lists. Nothing lost: nothing was written. |
| after Step 166, same sitting | the stores write to the LV | E3 whole: the writes since `start.time` are lost (minutes, recorded), or E4 to keep them |
| after the sitting | the copy underneath is stale | E4 only (reverse copy); an unmount alone loses everything written since the move |

- [ ] **Step 145** [R] ~3m: The rollback table above, read aloud with Stevie. Note in `$PROG` that it was read.
- [ ] **Step 146** [S][B] ~2m: Pre-checks: room, no earlier LV, no backup about to run, no existing Docker drop-in, who mounts each volume.
  ```bash
  sudo vgs --noheadings --units g -o vg_free ubuntu-vg; sudo lvs --noheadings -o lv_name ubuntu-vg; systemctl list-timers unheaded-well-backup.timer unheaded-well-drill.timer --no-legend --plain | awk '{print $1, $2, $3, $NF}'; ls /etc/systemd/system/docker.service.d 2>&1; for v in unheaded-dev_postgres-data unheaded-dev_clickhouse-data unheaded-dev_victoria-data unheaded-dev_vector-data monitoring_prometheus_data; do echo "$v: $(docker ps -a --filter volume=$v --format '{{.Names}}:{{.State}}' | tr '\n' ' ')"; done
  ```
- [ ] **Step 147** [V] ~1m: free 32g or more; only `ubuntu-lv`; neither timer due in the next 2 h; `docker.service.d` absent (or holding only files named in `$PROG`); each volume names its store (`unheaded-postgres`, `-clickhouse`, `-victoria`, `-vector`, `-prometheus`) and at most the exited one-shots `volume-init` and `clickhouse-schema`. Any other running container on a volume joins the stop in Step 158 by name. A timer due → wait for it to finish first.
- [ ] **Step 148** [S][B] ~2m: The volume.
  ```bash
  sudo lvcreate -L 32G -n unheaded-data ubuntu-vg && sudo mkfs.ext4 -q -L unheaded-data -m 1 /dev/ubuntu-vg/unheaded-data && sudo install -d -m 0755 /srv/unheaded-data && sudo blkid -s LABEL -s TYPE /dev/ubuntu-vg/unheaded-data
  ```
  Expect `LABEL="unheaded-data" TYPE="ext4"`.
  **Rollback:** `sudo lvremove -y ubuntu-vg/unheaded-data && sudo rmdir /srv/unheaded-data`
- [ ] **Step 149** [S][W] ~2m: Its fstab line, from the fragment, after a backup.
  ```bash
  TS=$(date -u +%Y%m%dT%H%M%SZ); sudo cp -p /etc/fstab /etc/fstab.west-ground.$TS && echo /etc/fstab.west-ground.$TS >> /home/govan/.local/state/unheaded/west-ground/fstab.backups && grep '^/dev/ubuntu-vg/unheaded-data ' /home/govan/tmp/unheaded/deploy/host-settings/west-fstab.fragment | sudo tee -a /etc/fstab && sudo findmnt --verify 2>&1 | tail -1 && sudo systemctl daemon-reload && sudo mount /srv/unheaded-data && findmnt -no SOURCE,FSTYPE /srv/unheaded-data && df -h --output=avail /srv/unheaded-data | tail -1
  ```
  Expect the line echoed, Step 125's summary, `/dev/mapper/ubuntu--vg-unheaded--data ext4`, about `31G`.
  **Rollback:** `sudo umount /srv/unheaded-data && sudo cp -p /etc/fstab.west-ground.$TS /etc/fstab && sudo systemctl daemon-reload`
- [ ] **Step 150** [S][B] ~1m: `sudo install -d -m 0700 /srv/unheaded-data/postgres /srv/unheaded-data/clickhouse /srv/unheaded-data/victoria /srv/unheaded-data/prometheus /srv/unheaded-data/vector` (rsync gives each the owner and mode of its source in Step 160). Under WG1 ruling B leave out `clickhouse` here and in Steps 160, 161 and 164.
- [ ] **Step 151** [S][B] ~3m: The backup sets' home on `sdb`, and a copy of every set (no backup runs now: Step 147).
  ```bash
  sudo install -d -o root -g root -m 0755 /var/backups/srv-backups && sudo rsync -aHAX --numeric-ids /srv/backups/ /var/backups/srv-backups/ && sudo rsync -aHAXn --numeric-ids --checksum --itemize-changes /srv/backups/ /var/backups/srv-backups/ | wc -l; sudo stat -c '%U:%G %a' /srv/backups /var/backups/srv-backups
  ```
  Expect `0` (nothing left to copy) and two identical lines (`root:root 755` on 2026-11-15).
- [ ] **Step 152** [V] ~1m: As expected. A count above 0 → run the two `rsync` lines once more; still above 0 → `[STUCK]`, Phase 5 `[BLOCKED]` (the move needs WG2).
- [ ] **Step 153** [S][W] ~2m: The bind, after a backup of fstab.
  ```bash
  TS=$(date -u +%Y%m%dT%H%M%SZ); sudo cp -p /etc/fstab /etc/fstab.west-ground.$TS && echo /etc/fstab.west-ground.$TS >> /home/govan/.local/state/unheaded/west-ground/fstab.backups && grep '^/var/backups/srv-backups ' /home/govan/tmp/unheaded/deploy/host-settings/west-fstab.fragment | sudo tee -a /etc/fstab && sudo findmnt --verify 2>&1 | tail -1 && sudo systemctl daemon-reload && sudo mount /srv/backups && findmnt -no SOURCE /srv/backups && sudo cat /srv/backups/unheaded-well/LATEST
  ```
  Expect the line, Step 125's summary, `/dev/sdb[/backups/srv-backups]`, the same set as Step 9.
  **Rollback:** copy any set newer than the bind back first (`sudo rsync -aHAX --numeric-ids /var/backups/srv-backups/ /srv/backups.hold/`), then `sudo umount /srv/backups`, restore this step's fstab backup, `sudo systemctl daemon-reload`, and `sudo rsync -aHAX /srv/backups.hold/ /srv/backups/`. The sets underneath on the NVMe are the ones of Step 151.
- [ ] **Step 154** [V] ~1m: As expected. The old sets remain on the NVMe under the mount; they are Stevie's to delete after plan 5's week (Step 315).
- [ ] **Step 155** [C] ~1m: **COMMIT** `$PROG`: "Steps 145-155: the volume and the backup sets' new home".
- [ ] **Step 156** [STEVIE] ~1m: "Stop the stack now for the move (10 to 15 minutes; Alertmanager and the mailer stay up)? Yes?"
- [ ] **Step 157** [B] ~2m: Silence, the firing alerts, and what runs.
  ```bash
  S=/home/govan/.local/state/unheaded/west-ground; cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh 60m "S-WEST-GROUND: database volumes to the NVMe" 2>&1 | tail -1 | cut -c1-60; curl -s -m5 127.0.0.1:9090/api/v1/alerts | python3 -c 'import json,sys; print("\n".join(sorted({a["labels"]["alertname"] for a in json.load(sys.stdin)["data"]["alerts"] if a["state"]=="firing"})))' > $S/alerts.before-move; sudo docker compose ps --services --status running | sort > $S/running.main; (cd monitoring && sudo docker compose ps --services --status running | sort > $S/running.monitoring); wc -l $S/running.main $S/running.monitoring $S/alerts.before-move; date -u +%FT%TZ | tee $S/stop.time
  ```
  Expect the silence line and three counts (main about 19 to 23, monitoring 3).
- [ ] **Step 158** [S][B] ~3m: Stop the main project and Prometheus (never `down`, never the kingdom scripts: G20).
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose stop 2>&1 | tail -2; cd monitoring && sudo docker compose stop prometheus 2>&1 | tail -1; docker ps --format '{{.Names}}' | sort | tr '\n' ' '
  ```
  Expect `unheaded-alertmanager` and `unheaded-alert-mailer` still listed, none of the five stores.
  **Rollback:** `cd /home/govan/tmp/unheaded && sudo docker compose start $(cat /home/govan/.local/state/unheaded/west-ground/running.main) && cd monitoring && sudo docker compose start prometheus`
- [ ] **Step 159** [V] ~1m: `docker inspect -f '{{.Name}} {{.State.Running}} {{.State.ExitCode}}' unheaded-postgres unheaded-clickhouse unheaded-victoria unheaded-vector unheaded-prometheus`. Expect `false` with 0 or 143 on each. 137 (killed at the grace period) is recorded, not a stop: committed data is safe and that database recovers at start.
- [ ] **Step 160** [S][B] ~8m: Copy.
  ```bash
  D=/var/lib/docker/volumes; N=/srv/unheaded-data; for p in postgres:unheaded-dev_postgres-data clickhouse:unheaded-dev_clickhouse-data victoria:unheaded-dev_victoria-data prometheus:monitoring_prometheus_data vector:unheaded-dev_vector-data; do n=${p%%:*}; v=${p#*:}; sudo rsync -aHAX --numeric-ids "$D/$v/_data/" "$N/$n/" && echo "$n copied $(date -u +%T)" || { echo "FAIL $n"; break; }; done
  ```
  Expect five `copied` lines.
- [ ] **Step 161** [S][B] ~5m: Verify by checksum (G4) and by owner and mode.
  ```bash
  D=/var/lib/docker/volumes; N=/srv/unheaded-data; for p in postgres:unheaded-dev_postgres-data clickhouse:unheaded-dev_clickhouse-data victoria:unheaded-dev_victoria-data prometheus:monitoring_prometheus_data vector:unheaded-dev_vector-data; do n=${p%%:*}; v=${p#*:}; echo "$n diff=$(sudo rsync -aHAXn --numeric-ids --checksum --itemize-changes "$D/$v/_data/" "$N/$n/" | wc -l) $(sudo stat -c '%u:%g %a' "$D/$v/_data") $(sudo stat -c '%u:%g %a' "$N/$n")"; done
  ```
- [ ] **Step 162** [V] ~1m: Every line `diff=0` and two identical owner and mode fields. Else → Step 163.
- [ ] **Step 163** [D] ~10m: Rerun Step 160 for that store only, then Step 161 (the stores are stopped, so a second pass is complete). Still not 0 → roll back to the running stack (Step 158's rollback; nothing was mounted yet), `[STUCK]`.
- [ ] **Step 164** [S][W] ~2m: The five binds, after a backup of fstab.
  ```bash
  TS=$(date -u +%Y%m%dT%H%M%SZ); sudo cp -p /etc/fstab /etc/fstab.west-ground.$TS && echo /etc/fstab.west-ground.$TS >> /home/govan/.local/state/unheaded/west-ground/fstab.backups && grep '^/srv/unheaded-data/' /home/govan/tmp/unheaded/deploy/host-settings/west-fstab.fragment | sudo tee -a /etc/fstab >/dev/null && sudo findmnt --verify 2>&1 | tail -1 && sudo systemctl daemon-reload && for t in $(awk '$1 ~ /^\/srv\/unheaded-data\// {print $2}' /home/govan/tmp/unheaded/deploy/host-settings/west-fstab.fragment); do sudo mount "$t" && echo "$(findmnt -no SOURCE "$t") $t"; done
  ```
  Expect Step 125's summary, then five lines `/dev/mapper/ubuntu--vg-unheaded--data[/<name>] /var/lib/docker/volumes/<volume>/_data`.
  **Rollback:** `for t in $(awk '$1 ~ /^\/srv\/unheaded-data\// {print $2}' /home/govan/tmp/unheaded/deploy/host-settings/west-fstab.fragment); do sudo umount "$t"; done; sudo cp -p /etc/fstab.west-ground.$TS /etc/fstab && sudo systemctl daemon-reload`
- [ ] **Step 165** [S][W] ~2m: Docker's requirement. Docker is not restarted: Phase 11's reboot proves the order.
  ```bash
  sudo install -d -m 0755 /etc/systemd/system/docker.service.d && sudo install -m 0644 /home/govan/tmp/unheaded/deploy/systemd/docker.service.d/10-unheaded-data.conf /etc/systemd/system/docker.service.d/ && sudo systemctl daemon-reload && systemctl show docker.service -p RequiresMountsFor --value | tr ' ' '\n' | grep -c '^/var/lib/docker/volumes/'
  ```
  Expect `5` (4 under WG1 B).
  **Rollback:** `sudo rm /etc/systemd/system/docker.service.d/10-unheaded-data.conf && sudo systemctl daemon-reload`. Always before any unmount of a bind (Appendix A, E3).
- [ ] **Step 166** [S][B] ~3m: Start what ran.
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose start $(cat /home/govan/.local/state/unheaded/west-ground/running.main) 2>&1 | tail -2; cd monitoring && sudo docker compose start prometheus 2>&1 | tail -1; date -u +%FT%TZ | tee /home/govan/.local/state/unheaded/west-ground/start.time
  ```
- [ ] **Step 167** [B][V] ~3m: Everything that ran runs; restart policies unchanged (decision 1).
  ```bash
  S=/home/govan/.local/state/unheaded/west-ground; cd /home/govan/tmp/unheaded && sudo docker compose ps --services --status running | sort | diff - $S/running.main && echo MAIN_SAME; (cd monitoring && sudo docker compose ps --services --status running | sort | diff - $S/running.monitoring && echo MON_SAME); docker ps -a --filter name=unheaded --format '{{.Names}}' | sort | while read -r c; do echo "$c $(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' "$c")"; done | diff - $S/restart-policy.before && echo POLICY_SAME
  ```
  Expect `MAIN_SAME`, `MON_SAME`, `POLICY_SAME`. A `<` or `>` line for a container plan 2 or 3 removed since Step 16 is not a failure: find its name in their progress file and note it. A missing container → Step 168. A changed policy → Appendix A, E5.
- [ ] **Step 168** [D] ~10m: `docker logs --tail 40 <name>`; `sudo stat -c '%u:%g %a'` on both sides of its volume; `findmnt <its target>`. Ownership or mode → `sudo chown`/`chmod` the top directory to the Step 161 values, start it again. Two tries, then Appendix A, E3, and `[STUCK]`.
- [ ] **Step 169** [B][V] ~3m: The stores answer and hold what they held: Step 37's five size commands, then `sudo docker exec unheaded-postgres pg_isready -q && echo PG_OK; curl -s -m5 127.0.0.1:8428/health; echo; curl -s -m5 127.0.0.1:9090/-/ready`. Expect sizes within 5% of Step 37 (merges and the minutes between move ClickHouse and VictoriaMetrics a little), `PG_OK`, `OK`, `Prometheus Server is Ready.`
- [ ] **Step 170** [S][B] ~2m: Each store sees the LV from inside its own mount namespace.
  ```bash
  for c in unheaded-postgres unheaded-clickhouse unheaded-victoria unheaded-vector unheaded-prometheus; do echo "$c $(sudo grep -c 'ubuntu--vg-unheaded--data' /proc/$(docker inspect -f '{{.State.Pid}}' $c)/mountinfo)"; done
  ```
  Expect a count of 1 or more on every line (ClickHouse 0 under WG1 B).
- [ ] **Step 171** [S][B] ~3m: WG2's proof: a real backup on the new layout.
  ```bash
  sudo systemctl start unheaded-well-backup.service; sudo journalctl -u unheaded-well-backup.service --since '-6 min' --no-pager | grep -E '\[well\] (data on|refusing)' | tail -1; sudo cat /srv/backups/unheaded-well/LATEST
  ```
  Expect `[well] data on nvme0n1, sets on sdb` and a set dated now. `refusing` or no line → Step 172.
- [ ] **Step 172** [D] ~10m: The message names the two disks. Sets on `nvme0n1` → `/srv/backups` is not mounted (`findmnt /srv/backups`); data on `sdb` → Postgres still sees the old directory (Step 170). Two tries, then WG2's rollback (Step 153) and `[STUCK]`. Before leaving the sitting the nightly backup must either pass or be known to fail: write which under "For Stevie".
- [ ] **Step 173** [B] ~1m: `cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh --end 2>&1 | tail -1 | cut -c1-60`
- [ ] **Step 174** [V] ~10m: Ten minutes later, the firing set against Step 157's: `curl -s -m5 127.0.0.1:9090/api/v1/alerts | python3 -c 'import json,sys; print("\n".join(sorted({a["labels"]["alertname"] for a in json.load(sys.stdin)["data"]["alerts"] if a["state"]=="firing"})))' | comm -13 /home/govan/.local/state/unheaded/west-ground/alerts.before-move -`. Expect nothing. A store's alert → Step 168.
- [ ] **Step 175** [W] ~3m: `$PROG` Log: `stop.time`, `start.time`, the downtime in minutes, the copy and checksum times, Step 37's and Step 169's sizes; "the new layout began `<start.time>`" (plan 5 and plan 2 need this date: Step 315).
- [ ] **Step 176** [C] ~1m: **COMMIT** `$PROG`: "Steps 156-176: the five database volumes on the NVMe; the stack back".
- [ ] **Step 177** [B][V] ~1m: W-04 closes: Postgres restarted in Step 166. `grep -E '^HugePages_(Total|Free|Surp):' /proc/meminfo` → all `0`. Not 0 → `grep -l HugetlbPages /proc/[0-9]*/status 2>/dev/null | head -3` names who holds them; record, W-04 stays open.
- [ ] **Step 178** [B] ~11m: One hour after `start.time`: R1 (Appendix C) to `after.R1`.
- [ ] **Step 179** [V] ~1m: `nvme0n1` `ms_per_flush` 3 or less (target); `sdb` `per_s` well below `before.R1` (journald still flushes there). Over 3 ms → Step 180.
- [ ] **Step 180** [D] ~10m: R3 now: if a store still issues flushes to `8:16`, its bind is wrong (Step 170). If every store flushes to the NVMe and it is just slower than 3 ms (a DRAM-less drive under five writers), record the number: W-01's after shows the miss, "For Stevie" gets it, nothing is rolled back for a missed target.
- [ ] **Step 181** [B] ~1m: R2 to `after.R2`.
- [ ] **Step 182** [S][B] ~1m: R3 to `after.R3`.
- [ ] **Step 183** [B] ~11m: R4 to `after.R4`.
- [ ] **Step 184** [W] ~5m: W-01 after: flush ms and flushes a second per device (R1), I/O pressure now and the 6 h mean (R2, the 6 h mean is retaken in Step 309), sync callers per device (R3), bytes per store per device in ten minutes (R4). W-02 after: "`[well] data on nvme0n1, sets on sdb`, set `<LATEST>`". Status: W-01 "moved; 24 h reading Step 309", W-02 "done".
- [ ] **Step 185** [W] ~2m: `$PROG` "Handoffs": "Plan 2 (S-WASTE-OUT): the volumes moved at `<start.time>`; a before taken earlier on Vector, ClickHouse or Postgres is retaken on the new disk (its own rule). Plan 5: the layout it measures began `<start.time>`."
- [ ] **Step 186** [C][V] ~1m: **PHASE 5 EXIT GATE**: `findmnt -no SOURCE` for `/srv/unheaded-data`, `/srv/backups` and the five targets shows the LV, `sdb` and five LV sub-paths; Step 165 printed 5; Steps 167, 169, 170 and 171 passed; W-01 and W-02 have before and after. Commit `$PROG`.

---

## PHASE 6: THE INPUT FILTER (Steps 187-214)

**Goal**: West's `inet unheaded` input policy is drop and survives a boot; the metric ports answer loopback, east and WireGuard only; a tenant in LXD reaches nothing but DNS, DHCP and syslog (W-05, W-07). This is S-E1-LIVE Evening 7, run here, with a rehearsal that reverts by itself first.
**Prerequisite**: WG5 ruling A; Step 98 printed `NFT_OK`; Merge A on develop (and under WG3 B, Step 97's edit). Sitting B: Stevie at the desk with his laptop and phone on the LAN, west's console in reach. West is east's only path out: this phase has the widest blast.
**Time**: 1.5 h.
**Model**: Opus. **Agent**: Coordinator with Stevie.

- [ ] **Step 187** [STEVIE] ~1m: "Sitting B: west's input filter (decision 16). Laptop and phone on the LAN, console in reach? Yes?"
- [ ] **Step 188** [B] ~2m: Listeners and the scrape count before.
  ```bash
  S=/home/govan/.local/state/unheaded/west-ground; ss -tlnH | awk '{print $4}' | grep -vE '^(127\.|\[::1\]|\[::ffff:127\.)' | sort -u | tr '\n' ' ' | tee $S/before.R7b; echo; curl -s -G -m5 127.0.0.1:9090/api/v1/query --data-urlencode 'query=sum(up)' | python3 -c 'import json,sys; print("up", json.load(sys.stdin)["data"]["result"][0]["value"][1])' | tee $S/up.before-filter
  ```
  Expect the same listeners as `before.R7`. A new one → name it (`sudo ss -tlnp | grep <port>`) before going on: the ruleset ignores it, so a needed one would silently stop.
- [ ] **Step 189** [STEVIE][V] ~5m: From outside, before. On the laptop:
  ```bash
  for p in 7375 80 443 3001 8443 9100 9110 9323 19120 20000 20001 20002; do nc -z -w3 192.168.69.184 $p && echo "$p open" || echo "$p closed"; done; curl -sk -o /dev/null -w 'dash-metrics %{http_code}\n' https://192.168.69.184:20000/metrics; curl -sk -o /dev/null -w 'kanban-tasks %{http_code}\n' https://192.168.69.184:20001/api/v1/tasks
  ```
  On the phone, Wi-Fi off, on cellular: `http://[<west's global address from ip -6 -br addr show wlp17s0 scope global>]:9100/metrics` (a browser or a shell app). Note open or closed for each; the address itself is not written down.
- [ ] **Step 190** [W] ~2m: W-05 before: the open ports from the laptop and the cellular result. (Before 2026-11-15 the expectation was 9100, 9110, 9323, 19120 open; G11.)
- [ ] **Step 191** [B] ~2m: A tenant's view, before (WG4). From the LXD instance `pihole`:
  ```bash
  A=$(ip -4 -br addr show lxdbr0 | awk '{print $3}' | cut -d/ -f1); for t in $A:9090 $A:9093 $A:9100 $A:9110 $A:9323 $A:19120 192.168.13.2:8428; do /snap/bin/lxc exec pihole --project default -- timeout 3 bash -c "echo > /dev/tcp/${t%:*}/${t##*:}" 2>/dev/null && echo "$t open" || echo "$t closed"; done | tee /home/govan/.local/state/unheaded/west-ground/tenant.before
  ```
  Expect 9090 and 9093 closed (loopback binds), the others open. `bash: not found` → use the instance's `nc -z -w3` in place of the `bash -c`, once; else `[STUCK]` for W-07's proof only.
- [ ] **Step 192** [S][B] ~2m: Stable paths, a silence, the tables before (S-E1-LIVE Step 139).
  ```bash
  cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh 60m "S-WEST-GROUND: west input filter" 2>&1 | tail -1 | cut -c1-60; sudo install -D -m 0755 scripts/host/nft-apply-confirm.sh /usr/local/lib/unheaded/nft-apply-confirm.sh && sudo install -D -m 0644 deploy/nftables/west.nft /usr/local/lib/unheaded/nft/west.nft && sudo nft -c -f /usr/local/lib/unheaded/nft/west.nft && echo NFT_OK; sudo nft list tables | sort | tee /home/govan/.local/state/unheaded/west-ground/nft-tables.before
  ```
- [ ] **Step 193** [V] ~1m: `NFT_OK`; no `table inet unheaded` in the list (Step 36). Else `[STUCK]`.
- [ ] **Step 194** [S][B] ~1m: **Rehearsal**: apply for 120 seconds and do not confirm.
  ```bash
  sudo /usr/local/lib/unheaded/nft-apply-confirm.sh apply /usr/local/lib/unheaded/nft/west.nft 120; date -u +%T
  ```
  Expect `... rolls back in 120s unless you run: ... confirm`.
  **Rollback:** automatic at 120 s, or at once with `sudo /usr/local/lib/unheaded/nft-apply-confirm.sh rollback`.
- [ ] **Step 195** [B][V] ~1m: Inside the 120 s: `ssh -o BatchMode=yes -o ConnectTimeout=5 east echo EAST_OK; sudo nft list chain inet unheaded input | grep -c 'policy drop'` → `EAST_OK`, `1`. The session this runs in is still alive.
- [ ] **Step 196** [B] ~3m: After 180 s: `sudo nft list tables | sort | diff - /home/govan/.local/state/unheaded/west-ground/nft-tables.before && echo REVERTED; systemctl list-timers --all --no-legend | grep -c unheaded-nft-rollback`
- [ ] **Step 197** [V] ~1m: `REVERTED` and `0`. The table still there → `sudo /usr/local/lib/unheaded/nft-apply-confirm.sh rollback`, `[STUCK]`: the self-revert is the safety net, and the real apply never runs without it.
- [ ] **Step 198** [C] ~1m: **COMMIT** `$PROG`: "Steps 187-198: the before, and a rehearsal that reverted by itself".
- [ ] **Step 199** [S][B] ~1m: **Apply, 600 seconds.** Walk Steps 200 to 203 inside the window.
  ```bash
  sudo /usr/local/lib/unheaded/nft-apply-confirm.sh apply /usr/local/lib/unheaded/nft/west.nft 600; date -u +%T
  ```
  **Rollback:** automatic at 600 s, or `sudo /usr/local/lib/unheaded/nft-apply-confirm.sh rollback`.
- [ ] **Step 200** [STEVIE][V] ~4m: On the laptop, a **new** terminal: `ssh -p 7375 govan@192.168.69.184` logs in; `https://192.168.69.184/` loads; Step 189's loop. Expect open: 7375, 80, 443, 3001, 20000, 20001, 20002 (8443 only from the laptop at .132); closed: 9100, 9110, 9323, 19120. Phone on Wi-Fi: a page loads (DNS works). Phone on cellular: 9100 on the global address is refused or times out. Anything else: do nothing, it rolls back; write what failed.
- [ ] **Step 201** [B][V] ~2m: From west, inside the window: east, the way out, LXD, Docker, the scrapes.
  ```bash
  ssh -o BatchMode=yes -o ControlPath=none east 'echo EAST_OK; getent hosts archive.ubuntu.com | head -1 | cut -c1-40'; dig +short +time=2 @10.10.20.252 archive.ubuntu.com A | head -1; /snap/bin/lxc exec pihole --project default -- ping -c1 -W2 1.1.1.1 | grep -c ' 0% packet loss'; docker exec unheaded-prometheus wget -qO- -T3 'http://[fd00:dead:beef::2]:9100/metrics' | grep -c '^node_uname_info'; docker logs --since 3m unheaded-prometheus 2>&1 | grep -ci 'remote_write.*\(error\|failed\)'; sudo nft list chain inet unheaded forward | grep -c 'ct status dnat'; curl -s -G -m5 127.0.0.1:9090/api/v1/query --data-urlencode 'query=sum(up)' | python3 -c 'import json,sys; print("up", json.load(sys.stdin)["data"]["result"][0]["value"][1])'
  ```
  Expect `EAST_OK` and an address (east resolves through west), an address, `1`, `1`, `0`, `3`, and the `up` of Step 188. Else: leave it, it rolls back.
- [ ] **Step 202** [B] ~2m: A tenant's view inside the window: Step 191's command, to `tenant.after`.
- [ ] **Step 203** [V] ~1m: Every line `closed`. One `open` → do not confirm (it rolls back); `[STUCK]` with that port.
- [ ] **Step 204** [S][B] ~1m: Confirm; the boot unit.
  ```bash
  cd /home/govan/tmp/unheaded && sudo /usr/local/lib/unheaded/nft-apply-confirm.sh confirm && sudo install -m 0644 deploy/systemd/unheaded-nft.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable unheaded-nft.service 2>&1 | tail -1 | cut -c1-60; sudo systemctl start unheaded-nft.service; systemctl is-active unheaded-nft.service; sudo nft list tables | grep -c 'inet unheaded$'
  ```
  Expect `confirmed`, `active`, `1`.
  **Rollback:** `sudo systemctl disable --now unheaded-nft.service` (its `ExecStop` deletes the table); check `sudo nft list tables | grep -c 'inet unheaded$'` is 0.
- [ ] **Step 205** [V] ~1m: As expected.
- [ ] **Step 206** [B] ~11m: Ten quiet minutes, then the drop log (S-E1-LIVE Step 146).
  ```bash
  sudo journalctl -k --since '-10 min' --no-pager | grep -E 'unheaded-(drop|fwd-drop)' | sed -E 's/.*(IN=[^ ]*).*(SRC=[^ ]*).*(DPT=[^ ]*).*/\1 \2 \3/' | sort | uniq -c | sort -rn | head; docker ps -q | wc -l
  ```
- [ ] **Step 207** [V] ~1m: Scanner noise or nothing; the container count of Step 157's list. A real client on the list (east, a scrape, the laptop) → add its accept to `$WT/deploy/nftables/west.nft`, Step 98's check, commit, and Steps 192 to 205 again (a fresh apply).
- [ ] **Step 208** [B] ~1m: R7 to `after.R7` (what listens has not changed; Steps 200 and 202 say what answers).
- [ ] **Step 209** [W] ~3m: W-05 after: the laptop's open and closed lists, the cellular result, the drop-log summary; status "done". W-07 after: `tenant.before` → `tenant.after`; status "a tenant reaches neither API (proved); any process on the host still can: open, owner plan 7".
- [ ] **Step 210** [W] ~5m: `$WT/compliance/findings/register.yaml`: `host-probe:west:firewall-inbound-deny` gets `resolved: {on: <date>, evidence: "inet unheaded input policy drop (S-WEST-GROUND Steps 187-205), published ports scoped in forward, phone on cellular refused on the global IPv6 address, unheaded-nft enabled"}`, **only if** Step 200's cellular test passed. And one new entry for WG4: the next free id (`grep -o 'FND-[0-9]*' compliance/findings/register.yaml | sort -V | tail -1`, plus one; first check that `S-WASTE-OUT` and `S-ZHEN-SWITCH` progress files have not taken it), title "Alertmanager (9093) and Prometheus (9090) take no credential on loopback: any process on west can silence every alert", owner plan 7 (S-TENANT-DOOR), evidence BlackMage F10 and Step 202. If Step 189 found 9100 open on cellular, one more entry: it was open from the internet for as long as node_exporter ran.
- [ ] **Step 211** [W] ~3m: `$WT/docs/battle-plans/progress/S-E1-LIVE-progress.md`: "- **<date>: Evening 7 (Steps 136-150) was run by S-WEST-GROUND Steps 187-210** (rehearsal reverted by itself first). Input policy drop live, `unheaded-nft` enabled. Do not run it again." `wiki/Security.md` (line 31 area): west's input filter is live and how it is applied. `wiki/Overlay-Network.md`: one line, published ports are scoped in the forward chain.
- [ ] **Step 212** [B] ~1m: `cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh --end 2>&1 | tail -1 | cut -c1-60`
- [ ] **Step 213** [B][V] ~3m: `cd /home/govan/tmp/wt/S-WEST-GROUND && GOMAXPROCS=4 go test -count=1 ./cmd/compliance-evidence/... 2>&1 | tail -1; scripts/check-spdx.sh 2>&1 | tail -1`. Expect `ok` and PASS (the register still parses).
- [ ] **Step 214** [C][V] ~1m: **PHASE 6 EXIT GATE**: `sudo nft list chain inet unheaded input | grep -c 'policy drop'` is 1; `systemctl is-enabled unheaded-nft` is `enabled`; W-05 and W-07 filled. **COMMIT** `compliance/findings/register.yaml docs/battle-plans/progress/S-E1-LIVE-progress.md wiki/Security.md wiki/Overlay-Network.md` and `$PROG`: "Steps 199-214: west's input filter live; Evening 7 recorded as run".

---

## PHASE 7: PID LIMITS, SLICES, AND THE TWO PATHS LIVE (Steps 215-244)

**Goal**: Every container has a pid limit and runs in `unheaded-env.slice`; the host units sit in `unheaded-host.slice` or `unheaded-env.slice`; under WG3 A, kanban's `/api` and both `/metrics` ask the LAN for a credential (W-09, W-10, W-06). No slice carries a number.
**Prerequisite**: Lane 2E exit (Step 94); under WG3 A, Phase 3 on develop (Step 121). Sitting B, after Phase 6 (the same day or another).
**Time**: 1.5 h; every container is recreated once (Step 227, about 5 minutes).
**Model**: Opus. **Agent**: Coordinator with Stevie.

- [ ] **Step 215** [B] ~5m: Develop into the lane, then the lane's gates.
  ```bash
  cd /home/govan/tmp/wt/S-WEST-GROUND-compose && { git merge --no-ff -S -q develop -m 'Merge develop into sprint/s-west-ground-compose' || { git merge --abort; git merge --no-ff --no-gpg-sign -q develop -m 'Merge develop into sprint/s-west-ground-compose' || { git merge --abort; echo MERGE_CONFLICT; }; }; }; bash scripts/test-check-compose-pids.sh >/dev/null && scripts/check-compose-pids.sh && echo PIDS_OK; for g in scripts/check-compose-hardening.sh scripts/check-compose-log-caps.sh scripts/check-compose-publish-bind.sh scripts/check-compose-bind-nesting.sh scripts/check-image-digests.sh; do $g >/dev/null 2>&1 && echo "ok $g" || echo "FAIL $g"; done
  ```
- [ ] **Step 216** [V] ~1m: `PIDS_OK` and five `ok`. `MERGE_CONFLICT` (plans 2 or 3 removed a service the lane edited) → Step 217.
- [ ] **Step 217** [D] ~40m: A fresh lane: `cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/S-WEST-GROUND-compose-2 -b sprint/s-west-ground-compose-2 develop && cd /home/govan/tmp/wt/S-WEST-GROUND-compose-2 && git checkout sprint/s-west-ground-compose -- scripts/check-compose-pids.sh scripts/test-check-compose-pids.sh`, then Steps 83 to 90 in it (the service list from develop as it now is), and use this lane in Step 223. Two tries, then `[STUCK]`: W-09 and W-10 open.
- [ ] **Step 218** [B] ~2m: Who reads a container's cgroup by its old path, in the tree and on the host.
  ```bash
  cd /home/govan/tmp/unheaded && git grep -n -F 'system.slice/docker-' develop -- ':!docs' ':!wiki' | cut -c1-160; sudo grep -rln -F 'system.slice/docker-' /usr/local/bin /usr/local/lib/unheaded /etc/systemd/system 2>/dev/null | head
  ```
- [ ] **Step 219** [V] ~1m: No hit, or each hit fixed by Step 93's change on develop. A hit not fixed → remove the `cgroup_parent` line from the lane's anchor (pids only), commit, W-10 `[STUCK]` with the reader's name.
- [ ] **Step 220** [S][B] ~2m: The four slices on the host.
  ```bash
  cd /home/govan/tmp/unheaded/deploy/systemd && sudo install -m 0644 unheaded.slice unheaded-host.slice unheaded-env.slice tenant.slice /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl start unheaded.slice unheaded-host.slice unheaded-env.slice tenant.slice && for s in unheaded unheaded-host unheaded-env tenant; do echo "$s $(systemctl is-active $s.slice) $(systemctl show $s.slice -p ControlGroup --value) $(systemctl show $s.slice -p MemoryLow --value) $(systemctl show $s.slice -p MemoryMax --value)"; done
  ```
  Expect four `active`, groups `/unheaded.slice`, `/unheaded.slice/unheaded-host.slice`, `/unheaded.slice/unheaded-env.slice`, `/tenant.slice`, and `0 infinity` on each (no number: decision 13).
  **Rollback:** (only with no member left) `sudo systemctl stop tenant.slice unheaded-env.slice unheaded-host.slice unheaded.slice && sudo rm /etc/systemd/system/{unheaded,unheaded-host,unheaded-env,tenant}.slice && sudo systemctl daemon-reload`
- [ ] **Step 221** [V] ~1m: As expected.
- [ ] **Step 222** [B] ~1m: Where the containers are now: `ls -d /sys/fs/cgroup/system.slice/docker-*.scope | wc -l` (23 or 24 on 2026-11-15).
- [ ] **Step 223** [C] ~3m: **MERGE B** (the lane) to develop, guarded. From here the next `compose up` by anyone applies it, so Step 227 follows in the same sitting.
  ```bash
  cd /home/govan/tmp/unheaded && test -z "$(git status --short --untracked-files=no)" && { git merge --no-ff -S -q sprint/s-west-ground-compose -m '[PLAN S-WEST-GROUND] Phase 7: a pid limit and the env slice for every container' || { git merge --abort; git merge --no-ff --no-gpg-sign -q sprint/s-west-ground-compose -m '[PLAN S-WEST-GROUND] Phase 7: a pid limit and the env slice for every container'; }; }; git log -1 --format='%h %G? %s' | cut -c1-80
  ```
  Add the session's two trailers with further `-m` arguments. (`-compose-2` if Step 217 made it.)
- [ ] **Step 224** [S][B] ~2m: It renders, and the override still applies (counts only).
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose config -q && echo CONFIG_OK; c=$(sudo docker compose config 2>/dev/null); echo "no-policy $(grep -c 'restart: "no"' <<<"$c") pids $(grep -c 'pids_limit:' <<<"$c") parent $(grep -c 'cgroup_parent: unheaded-env.slice' <<<"$c") anon $(grep -c 'AUTH_ANONYMOUS_FROM' <<<"$c")"; unset c
  ```
  Expect `CONFIG_OK`; `no-policy` equal to Step 15's `plain:` count; `pids` and `parent` equal to the number of services; `anon` 2 under WG3 A, 0 otherwise.
- [ ] **Step 225** [STEVIE] ~1m: "Recreate every container now with its pid limit and slice (about 5 minutes)? Yes?"
- [ ] **Step 226** [B] ~2m: Step 157's command again (silence "S-WEST-GROUND: pid limits and slices"; the firing set to `alerts.before-recreate` in place of `alerts.before-move`; the two running lists); and `curl` Step 188's `sum(up)` to `up.before-recreate`.
- [ ] **Step 227** [S][B] ~6m: Recreate what ran, with the override (no `-f`).
  ```bash
  S=/home/govan/.local/state/unheaded/west-ground; cd /home/govan/tmp/unheaded && sudo docker compose up -d --no-build --no-deps $(cat $S/running.main) 2>&1 | tail -4; cd monitoring && sudo docker compose up -d --no-build --no-deps $(cat $S/running.monitoring) 2>&1 | tail -2
  ```
  **Rollback:** `git revert -S -m 1 <Step 223's merge>` on develop, then this same command: the containers come back without limits in `system.slice`. The slices stay, empty.
- [ ] **Step 228** [B][V] ~3m: Step 167's command: `MAIN_SAME`, `MON_SAME`, `POLICY_SAME`. A changed policy → Appendix A, E5.
- [ ] **Step 229** [B][V] ~2m: Every container has its limit and its slice.
  ```bash
  for c in $(docker ps --filter name=unheaded --format '{{.Names}}' | sort); do id=$(docker inspect -f '{{.Id}}' $c); p=/sys/fs/cgroup/unheaded.slice/unheaded-env.slice/docker-$id.scope; echo "$c $(docker inspect -f '{{.HostConfig.PidsLimit}}' $c) $( [ -d $p ] && cat $p/pids.max || echo NOT_IN_SLICE)"; done | tee /home/govan/.local/state/unheaded/west-ground/after.R6.containers; ls -d /sys/fs/cgroup/system.slice/docker-*.scope 2>/dev/null | wc -l
  ```
  Expect `512 512` on every line, `2048 2048` for ClickHouse, no `NOT_IN_SLICE`, and `0`.
- [ ] **Step 230** [D] ~10m: `NOT_IN_SLICE` → `docker inspect -f '{{.HostConfig.CgroupParent}}' <c>`: empty means compose did not recreate it: `sudo docker compose up -d --no-build --no-deps --force-recreate <service>`. Docker refused the parent → `sudo journalctl -u docker --since -10min --no-pager | tail -5`. Two tries, then Step 227's rollback and `[STUCK]`.
- [ ] **Step 231** [B][V] ~2m: A reader named in Step 218 or Step 93 still reads: its series (named in plan 1's progress file) in VictoriaMetrics counts the same containers as before the recreate. No reader → write "none" and go on.
- [ ] **Step 232** [B] ~1m: Ten minutes after Step 227: R6 (Appendix C) to `after.R6`.
- [ ] **Step 233** [V] ~1m: `awk '{s+=$2} END{print s, s/115473}' /home/govan/.local/state/unheaded/west-ground/after.R6.containers` → the sum (15,360 or less) and a share under 0.15. No container's `pids.events` shows `max` above 0.
- [ ] **Step 234** [S][W] ~5m: The host units' slices (drop-ins of Step 53). Zhen's units (`unheaded-vor`, `zhen-*`, `unheaded-zhen-ui-front`) are plan 3's and are not touched.
  ```bash
  cd /home/govan/tmp/unheaded/deploy/systemd/slice-dropins && for u in huginn node-exporter unheaded-trace-collector unheaded-chaos-controller unheaded-baseline; do sudo install -D -m 0644 unheaded-host.conf /etc/systemd/system/$u.service.d/50-slice.conf; done && for u in unheaded-wiki-server unheaded-kev-poller unheaded-syslog-relay unheaded-well-backup unheaded-well-drill; do sudo install -D -m 0644 unheaded-env.conf /etc/systemd/system/$u.service.d/50-slice.conf; done && sudo systemctl daemon-reload && echo DROPINS_OK
  ```
  **Rollback (per unit):** `sudo rm /etc/systemd/system/<u>.service.d/50-slice.conf && sudo systemctl daemon-reload && sudo systemctl restart <u>`
- [ ] **Step 235** [S][B] ~5m: Restart the running ones, one at a time (the oneshots move at their next run).
  ```bash
  for u in huginn node-exporter unheaded-trace-collector unheaded-chaos-controller unheaded-wiki-server unheaded-kev-poller unheaded-syslog-relay; do sudo systemctl restart $u.service; echo "$u $(systemctl is-active $u.service) $(systemctl show -p ControlGroup --value $u.service)"; done
  ```
- [ ] **Step 236** [V] ~2m: Seven `active`, each group under `/unheaded.slice/`. Two minutes later `sum(up)` equals `up.before-recreate`. A unit that does not come back → its rollback line, `[STUCK]` for it alone.
- [ ] **Step 237** [W] ~3m: W-09 after: the sum, the share, the largest `pids.peak` (ClickHouse) against its limit. W-10 after: `ls -d /sys/fs/cgroup/unheaded.slice/unheaded-env.slice/docker-*.scope | wc -l` containers and the twelve unit names by slice; "no number set (decision 13)".
- [ ] **Step 238** [B][V] ~2m: (WG3 A) From west and from east, which are allowed.
  ```bash
  curl -sk -o /dev/null -w 'lo dash-metrics %{http_code}\n' https://127.0.0.1:20000/metrics; curl -sk -o /dev/null -w 'lo kanban-tasks %{http_code}\n' https://127.0.0.1:20001/api/v1/tasks; ssh -o BatchMode=yes east "curl -sk -o /dev/null -w 'overlay dash-metrics %{http_code}\n' 'https://[fd00:dead:beef::1]:20000/metrics'"; curl -s -G -m5 127.0.0.1:9090/api/v1/query --data-urlencode 'query=up{job=~".*dashboard.*|.*kanban.*"}' | python3 -c 'import json,sys; print([r["value"][1] for r in json.load(sys.stdin)["data"]["result"]])'
  ```
  Expect `200`, `200`, `200`, and every scrape `1`. (Port 20000 on loopback: if `ss -tln` shows it on another local address only, use that one.) A 401 here → Step 117's reasoning: the default list missed a source; fix the list in `docker-compose.yml`, never the code path.
- [ ] **Step 239** [STEVIE][V] ~3m: (WG3 A) From the laptop.
  ```bash
  for u in https://192.168.69.184:20000/metrics https://192.168.69.184:20001/api/v1/tasks https://192.168.69.184:20001/ http://192.168.69.184:21080/kanban/api/v1/tasks http://192.168.69.184:21080/dashboard/metrics; do curl -sk -o /dev/null -w "%{http_code} $u\n" "$u"; done
  ```
  Expect `401`, `401`, `200`, then `404` or `401` for the two through Traefik (G22). A `200` on any of the four → the path is still open: rollback below, W-06 `[STUCK]` with the URL.
  **Rollback (Stevie's hands: `.env` holds secrets and is never printed):** add `AUTH_ANONYMOUS_FROM=0.0.0.0/0,::/0` to `/home/govan/tmp/unheaded/.env`, then `cd /home/govan/tmp/unheaded && sudo docker compose up -d --no-build --no-deps dashboard-backend kanban-app`. Every source is then allowed: the old behaviour.
- [ ] **Step 240** [W] ~2m: W-06 after: the five codes of Step 239 and the three of Step 238; status "done" (or "accepted" under WG3 C, or "closed by the filter" under B).
- [ ] **Step 241** [B][V] ~10m: `scripts/monitoring/maintenance.sh --end`; ten minutes later Step 174's comparison against `alerts.before-recreate`. Expect nothing new.
- [ ] **Step 242** [C] ~1m: **COMMIT** `$PROG`: "Steps 215-242: pid limits and slices live; the two paths ask for a credential".
- [ ] **Step 243** [C] ~10m: **MERGE C**: Step 100's command on `$WT` (develop into the sprint branch and its gates), then Step 102's guarded merge with subject `[PLAN S-WEST-GROUND] Phases 4-7: the live record`.
- [ ] **Step 244** [V] ~1m: **PHASE 7 EXIT GATE**: on develop `grep -c '<<: \*ground' docker-compose.yml` equals the number of services; Step 229 passed; Step 236 passed; W-06, W-09 and W-10 filled.

---

## PHASE 8: ONE SOURCE FOR WEST'S HOST SETTINGS, LIVE (Steps 245-263)

**Goal**: The drift check runs daily as root on west, its metric reaches VictoriaMetrics, its alert is loaded, and it reads no drift; the repository's WireGuard and FRR files for west are what west runs, with no secret in them (W-11).
**Prerequisite**: Phases 4 to 7 done (every file in `west.tsv` that this plan installs is installed); Merge A on develop. Sitting C.
**Time**: 1 h.
**Model**: Opus (root, WireGuard and FRR files, secrets). **Agent**: Coordinator with Stevie for Step 250.

- [ ] **Step 245** [S][B] ~2m: Install the gate as a root-owned copy; the before.
  ```bash
  sudo install -D -m 0755 -o root -g root /home/govan/tmp/unheaded/scripts/check-host-settings.sh /usr/local/lib/unheaded/check-host-settings.sh && sudo /usr/local/lib/unheaded/check-host-settings.sh --host west --repo /home/govan/tmp/unheaded > /home/govan/.local/state/unheaded/west-ground/hostsettings.before 2>&1; echo rc=$?; cut -f1 /home/govan/.local/state/unheaded/west-ground/hostsettings.before | sort | uniq -c; grep -v '^OK' /home/govan/.local/state/unheaded/west-ground/hostsettings.before
  ```
  The gate prints mode and path, never a line of a file (Step 61, case 6).
- [ ] **Step 246** [V] ~1m: `rc=1`. Expected `DRIFT` rows: `redact /etc/wireguard/wg0.conf` (G17); possibly `exact /etc/frr/frr.conf` and `/etc/frr/daemons`; `absent /var/swapfile4` (until Step 305). Any `DRIFT` on a file this plan installed (sysctl, fstab, the drop-in, the four slices, `west.nft`) or on a runtime row (`sysctl`, `mount`, `swap`) → Step 247. A `SKIP` → the run was not as root: rerun with `sudo`.
- [ ] **Step 247** [D] ~10m: Someone changed one side since it was installed. These files hold no secret: `sudo diff <host path> /home/govan/tmp/unheaded/<repo path>`. Reinstall from develop with the step that installed it (sysctl 139, fstab lines 149/153/164, drop-in 165, slices 220, ruleset 192 to 205), or, if the host side was a deliberate change, ask Stevie under WG6. Two tries.
- [ ] **Step 248** [S][B] ~1m: Secret-bearing lines in the third-party files, counted, never printed.
  ```bash
  sudo grep -c -i -E 'password|secret|key' /etc/wireguard/wg0.conf /etc/frr/frr.conf /etc/frr/daemons
  ```
  Record the three counts. A file with a count above 0 is written to the repository only through the redaction of Step 251; an `exact` row for it becomes `redact` in `west.tsv`.
- [ ] **Step 249** [S][B] ~3m: The difference, keys and passwords redacted on both sides (the sed of Step 65).
  ```bash
  R='s/^([[:space:]]*(PrivateKey|PresharedKey|PublicKey))[[:space:]]*=.*/\1 = <redacted>/; s/(password)[[:space:]]+[^[:space:]]+/\1 <redacted>/'; cd /home/govan/tmp/unheaded && for p in /etc/wireguard/wg0.conf:config/wireguard/west.conf /etc/frr/frr.conf:routing/frr/west.conf /etc/frr/daemons:routing/frr/daemons; do echo "== ${p%%:*}"; diff <(sudo sed -E "$R" "${p%%:*}" | grep -v -E '^[[:space:]]*(#|!|$)') <(sed -E "$R" "${p#*:}" | grep -v -E '^[[:space:]]*(#|!|$)'); done
  ```
  Expect only `<redacted>` where a key or a password stands. A long base64-looking string in the output → stop, clear the terminal scrollback, and fix the sed before anything is written.
- [ ] **Step 250** [STEVIE][DECIDE] ~5m: **WG6**, per file, with Step 249's output on screen. Recommended: the host wins for WireGuard and FRR (the tunnel and the session work; changing them is plan 6's design). Repo wins for a file → that row is removed from `west.tsv` with the comment "east and west's routing files: plan 6 (S-EAST-PAIR)", and nothing on the host changes.
- [ ] **Step 251** [W] ~10m: Host wins: the repository file from the host, secrets replaced by the repository's own placeholders, the repository's header kept.
  ```bash
  cd /home/govan/tmp/wt/S-WEST-GROUND && git merge -q --no-edit develop && { sed -n '1,11p' config/wireguard/west.conf; sudo sed -E 's/^([[:space:]]*PrivateKey)[[:space:]]*=.*/\1 = <WEST_PRIVATE_KEY>/; s/^([[:space:]]*PresharedKey)[[:space:]]*=.*/\1 = <PRESHARED_KEY>/; s/^([[:space:]]*PublicKey)[[:space:]]*=.*/\1 = <EAST_PUBLIC_KEY>/' /etc/wireguard/wg0.conf | grep -v '^#'; } > config/wireguard/west.conf.new && mv config/wireguard/west.conf.new config/wireguard/west.conf; grep -c -E '[A-Za-z0-9+/]{42}[A-Za-z0-9+/=]=' config/wireguard/west.conf
  ```
  Expect `0` (no WireGuard key left). Then edit header line 6 (`# Overlay: fd00:dead:beef::1/64`) to the address the host holds. For each FRR file ruled "host wins": with Step 248's count 0, `sudo cat /etc/frr/frr.conf > routing/frr/west.conf` (and `daemons`); with a count above 0, the same through `sed -E 's/(password)[[:space:]]+[^[:space:]]+/\1 <BGP_PASSWORD>/'` and its row becomes `redact`. Keep the SPDX header if the repository file had one (`git diff` shows it removed → put it back as the first line).
- [ ] **Step 252** [B][V] ~3m: The gates.
  ```bash
  cd /home/govan/tmp/wt/S-WEST-GROUND && scripts/check-host-settings.sh && echo SELF_OK; sudo /usr/local/lib/unheaded/check-host-settings.sh --host west --repo /home/govan/tmp/wt/S-WEST-GROUND | grep -v '^OK'; scripts/check-gitleaks.sh 2>&1 | tail -1; scripts/check-spdx.sh 2>&1 | tail -1
  ```
  Expect `SELF_OK`; at most `DRIFT absent /var/swapfile4`; gitleaks and SPDX pass. A gitleaks finding → `git checkout -- <file>`, `[STUCK]`, the row stays drifted with the reason.
- [ ] **Step 253** [C] ~1m: **COMMIT** the files Step 251 changed and `deploy/host-settings/west.tsv` if a mode changed: "Steps 245-253: west's WireGuard and FRR files are what west runs (keys redacted)".
- [ ] **Step 254** [W] ~3m: `wiki/Overlay-Network.md`: the sentence calling the repository files "copies of what is deployed" (`grep -n 'copies of what is deployed' wiki/*.md`) becomes true for west, checked daily by `unheaded-host-settings.timer`; east's files are plan 6's. Commit it.
- [ ] **Step 255** [C] ~10m: **MERGE D** (Step 100 on `$WT`, then Step 102 with subject `[PLAN S-WEST-GROUND] Phase 8: west's host files match the host`), so the timer reads them from the main checkout.
- [ ] **Step 256** [S][B] ~3m: The timer.
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -m 0644 deploy/systemd/unheaded-host-settings.service deploy/systemd/unheaded-host-settings.timer /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now unheaded-host-settings.timer && sudo systemctl start unheaded-host-settings.service; systemctl show unheaded-host-settings.service -p Result --value; cat /var/lib/unheaded/textfile/unheaded_host_settings.prom; stat -c '%a' /var/lib/unheaded/textfile/unheaded_host_settings.prom
  ```
  Expect `success`; `drift_files` 1 (`swapfile4`) or 0; `skipped_files 0`; `644`.
  **Rollback:** `sudo systemctl disable --now unheaded-host-settings.timer && sudo rm /etc/systemd/system/unheaded-host-settings.service /etc/systemd/system/unheaded-host-settings.timer /var/lib/unheaded/textfile/unheaded_host_settings.prom && sudo systemctl daemon-reload`
- [ ] **Step 257** [D] ~10m: Result not `success`: `journalctl -u unheaded-host-settings -n 20 --no-pager`. A hardening option refusing (most likely `ProtectHome=read-only` against the checkout, or the capability set against `/etc/wireguard`): relax that one option in `deploy/systemd/unheaded-host-settings.service` with a comment saying why, commit, merge, reinstall. Two tries.
- [ ] **Step 258** [B] ~3m: The rule loaded, the series stored.
  ```bash
  docker kill -s HUP unheaded-prometheus >/dev/null; curl -s -m5 127.0.0.1:9090/api/v1/rules | python3 -c 'import json,sys; print(sorted(r["name"] for g in json.load(sys.stdin)["data"]["groups"] for r in g["rules"] if r["name"].startswith("HostSettings")))'; curl -s -G -m5 127.0.0.1:8428/api/v1/query --data-urlencode 'query=unheaded_host_settings_drift_files' | python3 -c 'import json,sys; print([(r["metric"].get("host"), r["value"][1]) for r in json.load(sys.stdin)["data"]["result"]])'
  ```
- [ ] **Step 259** [V] ~1m: `['HostSettingsCheckStale', 'HostSettingsDrift']` and `[('west', '1')]` (or `'0'`). An empty series list two scrape intervals later → huginn cannot read the file (mode, Step 256) or does not read that directory: `[STUCK]`, the alert half of W-11 open.
- [ ] **Step 260** [S][B] ~3m: Proved live: a drift appears and goes.
  ```bash
  sudo cp -p /etc/sysctl.d/99-unheaded.conf /var/backups/west-ground/sysctl.provoke && echo '# provoke S-WEST-GROUND' | sudo tee -a /etc/sysctl.d/99-unheaded.conf >/dev/null && sudo systemctl start unheaded-host-settings.service; grep drift_files /var/lib/unheaded/textfile/unheaded_host_settings.prom; sudo cp -p /var/backups/west-ground/sysctl.provoke /etc/sysctl.d/99-unheaded.conf && sudo systemctl start unheaded-host-settings.service; grep drift_files /var/lib/unheaded/textfile/unheaded_host_settings.prom
  ```
  Expect the count one higher, then back. **Rollback:** the second `cp` is it; check `cmp /etc/sysctl.d/99-unheaded.conf /home/govan/tmp/unheaded/deploy/sysctl.d/99-unheaded-west.conf` is silent.
- [ ] **Step 261** [W] ~2m: W-11 after: Step 245's counts → the counts now; "east's half: plan 6".
- [ ] **Step 262** [W] ~5m: `wiki/Systemd-Units.md`: rows for `unheaded-host-settings.service`/`.timer`, the four slices (what each holds, "no number: decision 13"), and the `50-slice.conf` drop-ins with the twelve units.
- [ ] **Step 263** [C][V] ~1m: **PHASE 8 EXIT GATE**: the textfile reads `drift_files` 0, or 1 with `absent /var/swapfile4` as the only `DRIFT` line of a manual run; the timer is enabled; W-11 filled. **COMMIT** `wiki/Systemd-Units.md` and `$PROG` in `$WT`.

---

## PHASE 9: THE LINK AND WIREGUARD MEASURED (Steps 264-279)

**Goal**: `iperf3` from the distribution on both hosts, and four numbers on record: TCP each way over the direct link and over WireGuard, with retransmits, CPU and east's temperature (W-08, decision 22).
**Prerequisite**: Phase 6 gate (west's filter accepts east on both paths). East is attended: every step that changes east is `[STEVIE]`. Sitting C.
**Time**: 40 min.
**Model**: Opus. **Agent**: Coordinator with Stevie.

- [ ] **Step 264** [STEVIE] ~1m: "Install `iperf3` from the distribution on west and east, and run four 30-second tests between them? East is loaded for 30 s at a time and the series stops at 67 °C." A no → Phase 9 `[BLOCKED]`; W-08 stays "not known".
- [ ] **Step 265** [B] ~1m: `apt-cache policy iperf3 | sed -n '2,3p'; ssh east 'apt-cache policy iperf3 | sed -n "2,3p"'`. Expect `Installed: (none)` and `Candidate: 3.18-2ubuntu0.1` on both (G19). Another candidate from the distribution is fine: record it.
- [ ] **Step 266** [S][B] ~2m: West.
  ```bash
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends iperf3 2>&1 | tail -1; iperf3 --version | head -1; systemctl is-enabled iperf3 2>&1
  ```
  Expect a version line and `disabled` or `not-found` (no daemon at boot). `enabled` → `sudo systemctl disable --now iperf3`.
  **Rollback:** `sudo apt-get purge -y iperf3 libiperf0`
- [ ] **Step 267** [STEVIE][S][B] ~2m: East (sudo there may ask Stevie's password).
  ```bash
  ssh -t east 'sudo DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends iperf3 2>&1 | tail -1; iperf3 --version | head -1; systemctl is-enabled iperf3 2>&1'
  ```
  Same expectations and rollback, on east.
- [ ] **Step 268** [V] ~1m: Both installed, neither runs a daemon.
- [ ] **Step 269** [B] ~1m: East's temperature (the `k10temp` sensor, found by name: hwmon numbers move between boots).
  ```bash
  ssh east 'for h in /sys/class/hwmon/hwmon*; do [ "$(cat $h/name)" = k10temp ] && echo "east_cpu_c $(( $(cat $h/temp1_input) / 1000 ))"; done'
  ```
  64 or less → go on (61 at idle on 2026-11-15). Above → wait ten minutes and read again; still above → `[BLOCKED]`, W-08 records the reading.
- [ ] **Step 270** [B] ~1m: The link's counters before, both ends: `ip -s link show enp16s0 | sed -n '3,6p'; cat /sys/class/net/enp16s0/speed /sys/class/net/enp16s0/duplex; ssh east 'ip -s link show enp2s0 | sed -n "3,6p"'`. Expect `1000`, `full`; record the error and drop counts.
- [ ] **Step 271** [STEVIE][B] ~2m: **L1**, west to east over the link: a one-shot server on east's link address (it exits after one test), then the client. Then Step 269.
  ```bash
  ssh -f east 'iperf3 -s -1 -B 192.168.13.1 >/dev/null 2>&1'; iperf3 -c 192.168.13.1 -t 30 -J > /home/govan/.local/state/unheaded/west-ground/L1.json; IPERF_SUM /home/govan/.local/state/unheaded/west-ground/L1.json
  ```
  (`IPERF_SUM` is the one-liner in Appendix C.) `Connection refused` → the server was not up yet: run the line once more.
- [ ] **Step 272** [STEVIE][B] ~2m: **L2**, east to west over the link: Step 271 with `-R` on the client, output `L2.json`. Then Step 269.
- [ ] **Step 273** [STEVIE][B] ~2m: **L3**, west to east over WireGuard: `ssh -f east 'iperf3 -s -1 -B fd00:dead:beef::2 >/dev/null 2>&1'; iperf3 -6 -c fd00:dead:beef::2 -t 30 -J > .../L3.json`, `IPERF_SUM`. Then Step 269.
- [ ] **Step 274** [STEVIE][B] ~2m: **L4**, Step 273 with `-R`, output `L4.json`. Then Step 269.
- [ ] **Step 275** [V] ~1m: Every temperature read after a run is under 67 °C; at 67 or above the series stops there and the reading is east's ceiling for now (decision 17). Expected: L1 and L2 900 to 941 Mbit/s with few retransmits (gigabit, full duplex); L3 and L4 lower and not known (east's A8-5500 does ChaCha20 in software): recorded as measured. L1 or L2 under 900 → Step 276.
- [ ] **Step 276** [D] ~10m: Step 270 again: errors or drops that rose; `ethtool -S enp16s0 2>/dev/null | grep -i -E 'err|drop|crc' | grep -v ': 0$'`; east's offloads (`ssh east 'ethtool -k enp2s0 2>/dev/null | grep -E "^(tx-checksumming|scatter-gather|tcp-segmentation-offload):"'`; Architect 4b row 9). Record; nothing is tuned here (plan 9).
- [ ] **Step 277** [B] ~1m: Step 270 again: the error and drop counts did not rise.
- [ ] **Step 278** [W] ~3m: W-08 before "not known (Archimedes 1.2)"; after: four rows of Mbit/s, retransmits, CPU at each end and east's peak temperature. `$PROG` "Handoffs": the four rows to plan 6.
- [ ] **Step 279** [C][V] ~1m: **PHASE 9 EXIT GATE**: four runs recorded, or the temperature stop recorded; nothing left listening on 5201: `ss -tlnH | grep -c ':5201 '; ssh east "ss -tlnH | grep -c ':5201 '"` → `0`, `0`. Commit `$PROG`.

---

## PHASE 10: READ-AHEAD ON sdb, ONE CHANGE, MEASURED (Steps 280-291)

**Goal**: `read_ahead_kb` on `sdb` is 256 if six hours of reads at 256 fall at least 20% below six hours at 8,192, and 8,192 otherwise; the result is written down either way (W-12, WG7, decision 20).
**Prerequisite**: Phase 5 ended at least 24 h ago (the disk's load has settled without the stores). Nothing else changes on west for the 12 hours: no deploys, no image builds, no stack restarts (decision 20). Sitting D starts here.
**Time**: 20 min of work across about 13 h.
**Model**: Opus. **Agent**: Coordinator; Stevie for Steps 280 and 286.

- [ ] **Step 280** [STEVIE] ~1m: "Freeze west for about 13 hours (no deploys, no builds, no restarts) for the read-ahead measurement?" A no → Phase 10 waits for a quiet day.
- [ ] **Step 281** [B] ~1m: `date -u +%s | tee /home/govan/.local/state/unheaded/west-ground/r8.before.start; cat /sys/block/sdb/queue/read_ahead_kb /sys/block/sdb/queue/scheduler` → a time, `8192`, `[mq-deadline]`.
- [ ] **Step 282** [B] ~2m: Six hours later: R8 (Appendix C) with `END` = Step 281's time plus 21,600, to `before.R8`.
- [ ] **Step 283** [S][B] ~1m: The change, at run time only.
  ```bash
  echo 256 | sudo tee /sys/block/sdb/queue/read_ahead_kb; date -u +%s | tee /home/govan/.local/state/unheaded/west-ground/r8.after.start
  ```
  **Rollback:** `echo 8192 | sudo tee /sys/block/sdb/queue/read_ahead_kb` (a boot also restores 8,192 until Step 288 persists 256).
- [ ] **Step 284** [B] ~2m: Six hours later: R8 with `END` = Step 283's time plus 21,600, to `after.R8`.
- [ ] **Step 285** [V] ~2m: Mean `read_B_s` after at most 0.8 of before, and mean `r_await_ms` after at most 1.2 of before → keep. Otherwise revert with Step 283's rollback and W-12 reads "reverted: no 20% gain". Six blocks that swing more than the difference (one block a build) → the comparison is void: say so, revert, repeat on another day.
- [ ] **Step 286** [STEVIE][DECIDE] ~2m: **WG7** with the numbers: keep 256 or revert. The scheduler stays `mq-deadline` (recommended: `IOWeight` needs `bfq`, and plan 8 decides weights).
- [ ] **Step 287** [W] ~10m: (Keep only.) `udevadm info --query=property --name=/dev/sdb | grep '^ID_MODEL='` → starts with `ST2000DM001` (the model, never the serial, goes in the repository). Then `$WT/deploy/udev/60-unheaded-readahead.rules` (new directory): SPDX, a comment (ADR-147, Architect A16, the before and after of Step 285), and
  ```
  ACTION=="add|change", SUBSYSTEM=="block", ENV{DEVTYPE}=="disk", ENV{ID_MODEL}=="ST2000DM001*", ATTR{queue/read_ahead_kb}="256"
  ```
  And a row in `deploy/host-settings/west.tsv`: `/etc/udev/rules.d/60-unheaded-readahead.rules	deploy/udev/60-unheaded-readahead.rules	exact`.
- [ ] **Step 288** [S][B] ~2m: (Keep only.)
  ```bash
  sudo install -m 0644 /home/govan/tmp/wt/S-WEST-GROUND/deploy/udev/60-unheaded-readahead.rules /etc/udev/rules.d/ && sudo udevadm control --reload && sudo udevadm trigger --action=change --sysname-match=sdb && for d in sda sdb sdc nvme0n1; do echo "$d $(cat /sys/block/$d/queue/read_ahead_kb)"; done
  ```
  Expect `sdb 256` and the other three unchanged (8192, 128, 128).
  **Rollback:** `sudo rm /etc/udev/rules.d/60-unheaded-readahead.rules && sudo udevadm control --reload && echo 8192 | sudo tee /sys/block/sdb/queue/read_ahead_kb`
- [ ] **Step 289** [B][V] ~2m: `cd /home/govan/tmp/wt/S-WEST-GROUND && scripts/check-host-settings.sh && bash scripts/test-check-host-settings.sh >/dev/null && scripts/check-spdx.sh 2>&1 | tail -1` → silent pass, pass.
- [ ] **Step 290** [C] ~1m: **COMMIT** (keep only) `deploy/udev/60-unheaded-readahead.rules deploy/host-settings/west.tsv` and `$PROG`: "Steps 280-290: sdb read-ahead 256, measured".
- [ ] **Step 291** [V] ~1m: **PHASE 10 EXIT GATE**: W-12 holds the six blocks before, the six after, the ruling and the status.

---

## PHASE 11: THE REBOOT THAT PROVES IT (Steps 292-308)

**Goal**: After one reboot west comes up with the LV and the six binds mounted before Docker starts, the filter loaded, `/swap.img` the only swap, no huge pages, the slices and the read-ahead in place, and the main stack still down until a person starts it (decision 1). Then the last swap file goes.
**Prerequisite**: Phases 4 to 10 gates. Sitting D, Stevie at the console.
**Time**: 45 min.
**Model**: Opus. **Agent**: Coordinator with Stevie.

- [ ] **Step 292** [STEVIE][DECIDE] ~2m: **WG8.** Recommended A: reboot now. B → Steps 293 to 303 wait for the next planned reboot (a row under "For Stevie"), and Step 305 waits with them.
- [ ] **Step 293** [B] ~2m: The state before, and what runs.
  ```bash
  S=/home/govan/.local/state/unheaded/west-ground; { findmnt -rno TARGET,SOURCE | grep -E 'unheaded--data|srv/backups|srv-backups'; awk 'NR>1{print $1}' /proc/swaps; grep '^HugePages_Total' /proc/meminfo; systemctl is-enabled unheaded-nft unheaded-host-settings.timer; for s in unheaded unheaded-host unheaded-env tenant; do echo "$s $(systemctl is-active $s.slice)"; done; cat /sys/block/sdb/queue/read_ahead_kb; } > $S/reboot.before 2>&1; cd /home/govan/tmp/unheaded && sudo docker compose ps --services --status running | sort > $S/running.main; (cd monitoring && sudo docker compose ps --services --status running | sort > $S/running.monitoring); wc -l $S/reboot.before $S/running.main
  ```
- [ ] **Step 294** [S][B] ~1m: The last look at fstab, and a copy named for the reboot.
  ```bash
  sudo findmnt --verify 2>&1 | tail -1; sudo cp -p /etc/fstab /etc/fstab.west-ground.prereboot && ls /etc/fstab.west-ground.* | tee -a /home/govan/.local/state/unheaded/west-ground/console.txt
  ```
  Expect Step 125's summary. The list of backups is now on paper beside the console (Stevie writes the name of the one from Step 125: it is the fstab before any change).
- [ ] **Step 295** [STEVIE] ~1m: At the console. Silence: `cd /home/govan/tmp/unheaded && scripts/monitoring/maintenance.sh 30m "S-WEST-GROUND: reboot" 2>&1 | tail -1 | cut -c1-60`. East loses its way out while west is down.
- [ ] **Step 296** [S][B] ~1m: `date -u +%FT%TZ | tee /home/govan/.local/state/unheaded/west-ground/reboot.time; sudo systemctl reboot`
- [ ] **Step 297** [STEVIE][V] ~5m: From the laptop: `ssh -p 7375 govan@192.168.69.184 uptime` answers within 5 minutes. Not in 10 → Appendix A, E1 (stuck in boot) or E2 (up but no ssh), at the console.
- [ ] **Step 298** [B][V] ~3m: The order held: the LV and each bind were mounted before Docker started.
  ```bash
  d=$(systemctl show -p ExecMainStartTimestampMonotonic --value docker.service); for t in /srv/unheaded-data $(awk '$1 ~ /^\/srv\/unheaded-data\// {print $2}' /home/govan/tmp/unheaded/deploy/host-settings/west-fstab.fragment); do u=$(systemd-escape --path --suffix=mount "$t"); m=$(systemctl show -p ActiveEnterTimestampMonotonic --value "$u"); [ "$m" -gt 0 ] && [ "$m" -lt "$d" ] && echo "BEFORE $t $(findmnt -no SOURCE "$t")" || echo "NOT_BEFORE $t"; done; systemctl is-active docker
  ```
  Expect six `BEFORE` lines with the LV as source, and `active`. A `NOT_BEFORE` → Docker started on a stale directory: stop Docker at once (`sudo systemctl stop docker.socket docker.service`), Appendix A, E9.
- [ ] **Step 299** [B][V] ~2m: The rest came back: Step 293's first block again, to `reboot.after`; `diff /home/govan/.local/state/unheaded/west-ground/reboot.before /home/govan/.local/state/unheaded/west-ground/reboot.after`. Expect no difference, except `tenant inactive` (a slice with no member is not started at boot: stated, not a failure). `sudo nft list chain inet unheaded input | grep -c 'policy drop'` → 1.
- [ ] **Step 300** [B][V] ~1m: Decision 1 at work: `docker ps --format '{{.Names}}' | sort | tr '\n' ' '`. Expect the monitoring containers (`unless-stopped`) and none of the main project's.
- [ ] **Step 301** [S][B] ~4m: Start what ran (Step 166's command), then Step 167's checks: `MAIN_SAME`, `MON_SAME`, `POLICY_SAME`.
- [ ] **Step 302** [B][V] ~3m: Step 169's health line and Step 170's mount check: every store answers and sees the LV.
- [ ] **Step 303** [S][B] ~1m: `sudo systemctl start unheaded-host-settings.service; grep drift_files /var/lib/unheaded/textfile/unheaded_host_settings.prom` → 1 (`swapfile4` only).
- [ ] **Step 304** [STEVIE] ~1m: "West booted without `/var/swapfile4`. Delete it (30 GiB on `sdb`)?" A no → W-03 records it kept and its `absent` row is removed from `west.tsv` (commit), so the gate does not drift for ever.
- [ ] **Step 305** [S][B] ~1m: `sudo rm /var/swapfile4 && sudo systemctl start unheaded-host-settings.service; grep drift_files /var/lib/unheaded/textfile/unheaded_host_settings.prom` → `0`.
  **Rollback (only if Stevie asks for swap on `sdb` again):** `sudo fallocate -l 30G /var/swapfile4 && sudo chmod 600 /var/swapfile4 && sudo mkswap /var/swapfile4 && sudo swapon /var/swapfile4` and its fstab line back.
- [ ] **Step 306** [B][V] ~10m: `scripts/monitoring/maintenance.sh --end`; ten minutes later Step 174's comparison against `alerts.before-recreate`. Expect nothing new.
- [ ] **Step 307** [W] ~3m: W-03 status "done"; `$PROG` Log: the reboot time, minutes to ssh, the six `BEFORE` lines, `drift_files 0`.
- [ ] **Step 308** [C][V] ~1m: **PHASE 11 EXIT GATE**: Steps 298 to 305 passed; the textfile reads 0. Commit `$PROG`.

---

## PHASE 12: CLOSE (Steps 309-324)

**Goal**: The 24-hour readings taken, every row of the progress table full, the record written where it belongs, develop holding the plan, `next.md` written.
**Prerequisite**: Phases 4 to 11 gates, or their `[STUCK]` markers.
**Time**: 1.5 h.
**Model**: Opus. **Agent**: Coordinator.

- [ ] **Step 309** [B] ~12m: At least 24 h after `start.time` (Step 166): R1 and R2 to `after24.R1` and `after24.R2`. The target: `nvme0n1` 3 ms or less a flush; I/O pressure's 6 h mean against `before.R2`.
- [ ] **Step 310** [S][B] ~1m: The NVMe's wear after: Step 39's command. In W-01: `Data Units Written` per day since Step 39 (one unit is 512,000 bytes) against the seats' 71 GB a day for ClickHouse (WG1).
- [ ] **Step 311** [B] ~2m: R5, R6 and R7 to `final.R5`, `final.R6`, `final.R7`.
- [ ] **Step 312** [V] ~3m: No empty cell in the progress table.
  ```bash
  awk -F'|' '/^\| W-[0-9]+ /{for(i=2;i<=9;i++) if($i ~ /^[[:space:]]*$/) print "EMPTY", $2, "column", i-1}' /home/govan/tmp/wt/S-WEST-GROUND/docs/battle-plans/progress/S-WEST-GROUND-progress.md
  ```
  Expect no output. A row that cannot be filled gets its reason in the cell ("not done: Step N `[STUCK]`, because ..."), never a blank.
- [ ] **Step 313** [W] ~15m: Ops facts in the wiki. New `wiki/West-Storage.md` (terse, like the other host pages): the LV (`ubuntu-vg/unheaded-data`, 32 GiB ext4, `/srv/unheaded-data`), the five binds and the `/srv/backups` bind with their fstab lines, Docker's drop-in and why, swap (`/swap.img` only) and huge pages (0), `sdb`'s read-ahead, where the fstab backups are, the console path (Esc for GRUB, `init=/bin/bash`), and one line of rollback per item (Appendix A). A link in `wiki/_Sidebar.md` beside Well-Backups. `wiki/Well-Backups.md` lines 9 and 10: live data on the NVMe (LV, bind), backup sets on `sdb` (`/var/backups/srv-backups` bound at `/srv/backups`), and why (the guard).
- [ ] **Step 314** [W] ~8m: ADR-147, a dated section at the end, `## Update <date> (plan 4, S-WEST-GROUND, closed)`: one table, W-01 to W-13, before → after, status; then who owns what is left: WG4 and `tenant.slice` (plan 7), east's host files and the `iperf3` numbers (plan 6), the slices' numbers (plan 8), the R11 dirty limits and `vm.overcommit_memory = 1` (plan 9), the second log week on this layout from `<start.time>` (plan 5). Nothing above it is edited.
- [ ] **Step 315** [W] ~5m: `$PROG` "For Stevie", in order of age: the runbook manifest seal (Step 76); the old copies of the five volumes under their binds on `sdb` (about 6 GB) and the old backup sets under `/srv/backups` on the NVMe: delete after plan 5's week; the WG4 finding; any `[STUCK]` with its step.
- [ ] **Step 316** [STEVIE] ~1m: One line for Stevie, not a step of this plan: the stopped `jenkins` container and its image on west are his to remove (Jenkins left the repository 2026-11-15); this plan did not touch them.
- [ ] **Step 317** [B] ~15m: In `$WT`, after `git merge -q --no-edit develop`: `PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin scripts/ci-local.sh 2>&1 | tail -25`
- [ ] **Step 318** [V] ~2m: Every gate passes, or each failure is shown red on develop before this plan's first merge (Appendix A, E10).
- [ ] **Step 319** [C] ~1m: **COMMIT** `wiki/West-Storage.md wiki/_Sidebar.md wiki/Well-Backups.md docs/adr/ADR-147-every-service-earns-its-place.md` and `$PROG`: "Steps 309-319: the 24 h readings and the record".
- [ ] **Step 320** [C] ~10m: **MERGE E**: Step 100 on `$WT`, then Step 102 with subject `[PLAN S-WEST-GROUND] Phases 9-12: read-ahead, the reboot, the record`.
- [ ] **Step 321** [B] ~1m: `cd /home/govan/tmp/unheaded && for w in S-WEST-GROUND S-WEST-GROUND-compose S-WEST-GROUND-compose-2; do git worktree remove /home/govan/tmp/wt/$w 2>/dev/null; done; git worktree list | grep -c S-WEST-GROUND` → `0`.
- [ ] **Step 322** [W] ~10m: `~/tmp/next.md`, a section at the top: the before and after of W-01 to W-13 in one table, what waits on Stevie (Step 315's list, Step 316's line), the handoffs to plans 5 to 9, and the number of unpushed commits.
- [ ] **Step 323** [V] ~1m: `git -C /home/govan/tmp/unheaded status --short --untracked-files=no` prints nothing; `git -C /home/govan/tmp/unheaded log origin/develop..develop --format='%G? %s' | grep -vc '^G'` is 0, or the unsigned ones are listed in `next.md` for `scripts/resign-unpushed.py`.
- [ ] **Step 324** [V] ~1m: **PHASE 12 EXIT GATE**: develop holds the plan; the progress table is full; `next.md` is written. Stevie pushes.

---

## Appendix A: Emergency procedures

**E1. West does not boot past fstab** (emergency mode; with root locked it says "Cannot open access to console, the root account is locked").
1. Power off and on. As the firmware hands over, press Esc a few times: GRUB's menu is hidden with a timeout of 0 (Step 124).
2. On the first entry press `e`; at the end of the line that starts `linux` add ` init=/bin/bash`; Ctrl-X.
3. `mount -o remount,rw /` (root and `/etc` are on the NVMe; nothing on `sdb` or the LV is needed).
4. `ls /etc/fstab.west-ground.*`; copy back the one taken just before the last change (`fstab.backups` lists them in order; the first is the fstab before this plan): `cp /etc/fstab.west-ground.<TS> /etc/fstab`.
5. If the last change was Docker's drop-in: `rm /etc/systemd/system/docker.service.d/10-unheaded-data.conf`.
6. `sync; reboot -f`. Then Step 298's check tells which mount failed; fix it attended, never by removing `nofail`.

**E2. West is up but ssh does not answer.** At the console log in as `govan`. `sudo nft list chain inet unheaded input | head -3`: if the filter is there, `sudo nft delete table inet unheaded` and try ssh from the laptop. ssh back → the filter was the cause: `sudo systemctl disable unheaded-nft`, read the drop log (Step 206's command), fix `west.nft`, Steps 192 to 205 again. Not the filter → `systemctl status ssh`.

**E3. A database does not come back after the move** (same sitting).
1. Stop the stack (Step 158's command).
2. `sudo rm /etc/systemd/system/docker.service.d/10-unheaded-data.conf && sudo systemctl daemon-reload` **first**: Docker requires the mounts, and an unmount must not take Docker down with it.
3. Unmount the five binds (Step 164's rollback loop). The old data is underneath, as it was at `stop.time`.
4. Copy back the fstab backup taken in Step 164; `sudo systemctl daemon-reload`.
5. Start from the lists (Step 166's command); Step 167's checks. Writes made on the LV since `start.time` are lost; record the minutes. Keep `/srv/unheaded-data` mounted: it holds what the stores wrote, for a later look.

**E4. Moving back after the sitting** (the copy underneath is stale; an unmount alone would lose the days since).
1. Stop the stack. Remove Docker's drop-in (E3 step 2).
2. For each store: `sudo umount <target>`, then `sudo rsync -aHAX --numeric-ids --delete /srv/unheaded-data/<name>/ /var/lib/docker/volumes/<volume>/_data/` and the checksum check of Step 161 (expect 0).
3. Remove the five bind lines from fstab (a backup first); `sudo systemctl daemon-reload`. Start; Step 167's checks. The LV stays until Stevie removes it.

**E5. A restart policy changed.** Never change the policy (decision 1). The cause is a compose run that skipped the host's override (an explicit `-f`, G20). Recreate that container from `/home/govan/tmp/unheaded` with no `-f`: `sudo docker compose up -d --no-build --no-deps <service>`, then compare with `restart-policy.before` again.

**E6. Locked out during a firewall window.** Do nothing: the timer rolls back within 120 or 600 s. At the console: `sudo /usr/local/lib/unheaded/nft-apply-confirm.sh rollback`. After a confirm: `sudo systemctl disable --now unheaded-nft.service` (its stop deletes the table).

**E7. The LV fills.** Postgres stops on a full WAL, VictoriaMetrics turns read-only. `df -h /srv/unheaded-data`; with Stevie's yes, grow it online: `sudo lvextend -r -L +16G ubuntu-vg/unheaded-data` (ext4 grows mounted). Then start what stopped.

**E8. Memory kills after swap left `sdb`.** `sudo journalctl -k --no-pager | grep -i -E 'killed process|oom-kill' | tail -5` names the process. With restart `no` a killed container stays down: start it (`sudo docker compose start <service>`). Kills that repeat inside the sitting: `sudo swapon /var/swapfile4` as a stopgap (the file exists until Step 305), `[STUCK]` for W-03, "For Stevie".

**E9. Docker did not start after the reboot, or started before a mount.** `systemctl status 'srv-unheaded\x2ddata.mount'`; `sudo lvs ubuntu-vg`; `sudo fsck -n /dev/ubuntu-vg/unheaded-data`. A mount that failed for a reason found and fixed → `sudo mount -a`, then `sudo systemctl start docker`. The LV lost → E3 steps 2 to 5 bring the stack up on the copies of `stop.time`; the Well comes back from the last nightly set on `sdb` or east (`wiki/Well-Backups.md`), ClickHouse and VictoriaMetrics from plan 1's backups (Step 10's directories).

**E10. A gate was red on develop before this plan.** Prove it in a clean clone at the commit before Merge A, log it under "For Stevie", and do not fix it here.

**E11. East heats up during Phase 9.** Stop the series (no further run), read Step 269 every two minutes until under 64, record the peak. Nothing is changed on east to cool it.

## Appendix B: Agent assignment and dependencies

| Phase | Agent | Parallel | Depends on | Time |
|---|---|---|---|---|
| 0 The gate | Coordinator + Stevie | no | the week's end for Steps 2 to 11 | 45m |
| 1 Before readings | Coordinator (sudo) | no | 0 | 45m |
| 2A-2C, 2B' Mounts, sysctl, slices, scripts | one agent | `[P]` with 2D, 2E | 0 (Steps 12, 13, 15) | 1.5h |
| 2D Host-settings gate | one agent | `[P]` | 0 | 2.5h |
| 2E Compose lane | one agent, own worktree | `[P]` | 0 (Step 19) | 1.5h |
| 2F Ruleset and Merge A | Coordinator | no | 2A-2E | 30m |
| 3 Source-gated routes | Developer | `[P]` with 2E | Merge A, WG3 A | 2.5h |
| 4 Swap, huge pages | Coordinator + Stevie (sitting A) | no | 0 (Steps 7, 8), 1, Merge A | 50m |
| 5 The move | Coordinator + Stevie (sitting A) | no | 0 (Steps 7 to 10), 1, 4 | 1.5h |
| 6 Input filter | Coordinator + Stevie (sitting B) | no | Merge A, WG5 A | 1.5h |
| 7 Pids, slices, paths live | Coordinator + Stevie (sitting B) | no | 2E, 3 | 1.5h |
| 8 Host settings live | Coordinator + Stevie (sitting C) | no | 4 to 7 | 1h |
| 9 Link measured | Coordinator + Stevie (sitting C) | no | 6 | 40m |
| 10 Read-ahead | Coordinator (sitting D) | no | 5 + 24 h | 20m over 13h |
| 11 Reboot | Coordinator + Stevie (sitting D) | no | 4 to 10 | 45m |
| 12 Close | Coordinator | no | all | 1.5h |

**Critical path**: the week's end → 0 → 1 → 4 → 5 → (24 h) → 10 (13 h) → 11 → 12. Phases 2 and 3 run before the week ends (repository only). Phases 6 to 9 fit between Phase 5 and Phase 10. At most three agents build at once (series rule 3); every live phase is the coordinator's, one change at a time.

## Appendix C: Quick reference

**Readings.** Each is taken the same way before and after; files go to `S=/home/govan/.local/state/unheaded/west-ground` as `<tag>.R<n>` (`before`, `after`, `after24`, `final`).

R1, flushes per device over ten minutes (`/proc/diskstats` field 19 = flushes completed, field 20 = ms flushing):
```bash
awk '$3 ~ /^(sdb|nvme0n1)$/ {print $3, $19, $20}' /proc/diskstats > $S/<tag>.R1.a
# ten minutes later:
awk '$3 ~ /^(sdb|nvme0n1)$/ {print $3, $19, $20}' /proc/diskstats > $S/<tag>.R1.b; dt=$(( $(date +%s) - $(stat -c %Y $S/<tag>.R1.a) )); paste $S/<tag>.R1.a $S/<tag>.R1.b | awk -v dt=$dt '{n=$5-$2; ms=$6-$3; printf "%s flushes=%d per_s=%.2f ms_per_flush=%.2f dt=%ds\n", $1, n, n/dt, (n ? ms/n : 0), dt}' | tee $S/<tag>.R1
```

R2, I/O pressure now and its 6 h mean:
```bash
cat /proc/pressure/io; for m in waiting stalled; do curl -s -m5 -G 127.0.0.1:8428/api/v1/query --data-urlencode "query=100*rate(node_pressure_io_${m}_seconds_total{instance=\"[fd00:dead:beef::1]:9100\"}[6h])" | python3 -c "import json,sys; r=json.load(sys.stdin)['data']['result']; print('io_${m}_6h_pct', r[0]['value'][1] if r else 'NA')"; done
```

R3, who calls sync and which device flushes, ten seconds (root; dev 8388624 = `sdb` 8:16, 271581184 = `nvme0n1` 259:0, 264241152 + N = `dm-N`):
```bash
sudo timeout -s INT 15 bpftrace -q -e 'tracepoint:syscalls:sys_enter_fsync,tracepoint:syscalls:sys_enter_fdatasync { @sync[comm] = count(); } tracepoint:block:block_rq_issue /args.rwbs[0] == 70/ { @flush[args.dev] = count(); } interval:s:10 { exit(); }'
```

R4, per store: bytes by device, I/O stall, memory-limit hits; two snapshots ten minutes apart, then the difference (`8:16` is `sdb`, `252:*` the LVs, `259:0` the NVMe):
```bash
for c in unheaded-postgres unheaded-clickhouse unheaded-victoria unheaded-vector unheaded-prometheus; do g=/sys/fs/cgroup$(cut -d: -f3 /proc/$(docker inspect -f '{{.State.Pid}}' $c)/cgroup); while read -r dev rest; do echo "$c io $dev $rest"; done < $g/io.stat; echo "$c iofull_us $(awk '/^full/{sub("total=","",$5); print $5}' $g/io.pressure)"; echo "$c memmax $(awk '$1=="max"{print $2}' $g/memory.events)"; done > $S/<tag>.R4.a
# ten minutes later the same into $S/<tag>.R4.b, then:
python3 -c 'import sys
def rd(f):
  d={}
  for l in open(f):
    w=l.split()
    if w[1]=="io":
      for kv in w[3:]:
        k,v=kv.split("="); d[(w[0],w[2],k)]=int(v)
    else: d[(w[0],w[1],"")]=int(w[2])
  return d
a,b=rd(sys.argv[1]),rd(sys.argv[2])
for k in sorted(b):
  if k in a and k[2] in ("wbytes","rbytes",""): print(*k, b[k]-a[k])' $S/<tag>.R4.a $S/<tag>.R4.b | tee $S/<tag>.R4
```

R5, memory, swap, space:
```bash
grep -E '^(MemTotal|MemAvailable|SwapTotal|SwapFree|HugePages_Total|HugePages_Free):' /proc/meminfo; awk 'NR>1{print $1, $3, $4}' /proc/swaps; df -B1 --output=target,avail / /var /srv/unheaded-data 2>/dev/null; sysctl vm.nr_hugepages vm.overcommit_memory vm.swappiness
```

R6, pid limits and peaks:
```bash
for c in $(docker ps --format '{{.Names}}' | sort); do g=/sys/fs/cgroup$(cut -d: -f3 /proc/$(docker inspect -f '{{.State.Pid}}' $c)/cgroup); echo "$c $(cat $g/pids.max) $(cat $g/pids.current) $(cat $g/pids.peak 2>/dev/null || echo NA)"; done; echo "threads-max $(cat /proc/sys/kernel/threads-max) now $(cut -d' ' -f4 /proc/loadavg | cut -d/ -f2)"
```

R7, listeners beyond loopback (what listens; Steps 189, 191, 200 and 202 say what answers):
```bash
ss -tlnH | awk '{print $4}' | grep -vE '^(127\.|\[::1\]|\[::ffff:127\.)' | sort -u | tr '\n' ' '; echo; ss -ulnH | awk '{print $4}' | grep -vE '^(127\.|\[::1\])' | sort -u | tr '\n' ' '; echo
```

R8, `sdb` reads in six one-hour blocks ending at `END` (epoch seconds):
```bash
python3 -c 'import json,sys,urllib.request,urllib.parse as u
end=int(sys.argv[1]); s="device=\"sdb\",instance=\"[fd00:dead:beef::1]:9100\""
Q={"read_B_s":"rate(node_disk_read_bytes_total{%s}[1h])"%s,"reads_s":"rate(node_disk_reads_completed_total{%s}[1h])"%s,"r_await_ms":"1000*rate(node_disk_read_time_seconds_total{%s}[1h])/rate(node_disk_reads_completed_total{%s}[1h])"%(s,s)}
for i in range(5,-1,-1):
  t=end-i*3600; row=[]
  for k,q in Q.items():
    r=json.load(urllib.request.urlopen("http://127.0.0.1:8428/api/v1/query?"+u.urlencode({"query":q,"time":t})))["data"]["result"]
    row.append("%s=%.3f"%(k,float(r[0]["value"][1])) if r else k+"=NA")
  print(t," ".join(row))' "$END" | tee $S/<tag>.R8
```

`IPERF_SUM`, one line from an `iperf3 -J` file:
```bash
python3 -c 'import json,sys; d=json.load(open(sys.argv[1]))["end"]; print("Mbit/s %.0f retrans %d cpu_local %.0f%% cpu_remote %.0f%%" % (d["sum_received"]["bits_per_second"]/1e6, d["sum_sent"].get("retransmits",0), d["cpu_utilization_percent"]["host_total"], d["cpu_utilization_percent"]["remote_total"]))' FILE
```

**What this plan installs on west, and how each comes out**

| Item | Path | Installed | Out |
|---|---|---|---|
| sysctl source | `/etc/sysctl.d/99-unheaded.conf` | Step 139 | Step 140's rollback |
| the LV | `ubuntu-vg/unheaded-data` at `/srv/unheaded-data` | Steps 148, 149 | rollback table, Phase 5 |
| backup sets' bind | `/var/backups/srv-backups` at `/srv/backups` | Steps 151, 153 | Step 153's rollback |
| five data binds | `/srv/unheaded-data/<name>` at `/var/lib/docker/volumes/<volume>/_data` | Step 164 | E3 or E4 |
| Docker's mount requirement | `/etc/systemd/system/docker.service.d/10-unheaded-data.conf` | Step 165 | Step 165's rollback, before any unmount |
| input filter | `inet unheaded`, `/usr/local/lib/unheaded/nft/west.nft`, `/var/lib/unheaded/nft/confirmed.nft`, `unheaded-nft.service` | Steps 192 to 204 | E6 |
| slices | `/etc/systemd/system/{unheaded,unheaded-host,unheaded-env,tenant}.slice` | Step 220 | Step 220's rollback |
| units' slice drop-ins | `/etc/systemd/system/<unit>.service.d/50-slice.conf` | Step 234 | Step 234's rollback |
| pid limits, env slice | `docker-compose.yml`, `monitoring/docker-compose.yml` (develop) | Steps 223, 227 | Step 227's rollback |
| drift check | `/usr/local/lib/unheaded/check-host-settings.sh`, `unheaded-host-settings.{service,timer}` | Steps 245, 256 | Step 256's rollback |
| `iperf3` | distribution package, both hosts | Steps 266, 267 | `apt-get purge -y iperf3 libiperf0` |
| read-ahead | `/etc/udev/rules.d/60-unheaded-readahead.rules` | Step 288 | Step 288's rollback |
| removed | `/var/swapfile2`, `/var/swapfile3` (Step 134), `/var/swapfile4` (Step 305) | | recreate only |

**Paths**: readings `/home/govan/.local/state/unheaded/west-ground/`; fstab backups `/etc/fstab.west-ground.*` (on the NVMe root; their order in `fstab.backups`); sysctl and provoke backups `/var/backups/west-ground/`; VictoriaMetrics `127.0.0.1:8428`; Prometheus `127.0.0.1:9090`; Alertmanager `127.0.0.1:9093`; textfile directory `/var/lib/unheaded/textfile`; east's link address `192.168.13.1`, west's `192.168.13.2`; overlay `fd00:dead:beef::1` (west) and `::2` (east).

**Gates**: `scripts/check-host-settings.sh` (self-check) and `--host west` (root), `scripts/test-check-host-settings.sh`, `scripts/check-compose-pids.sh`, `scripts/test-check-compose-pids.sh`, `scripts/check-gates-can-fail.sh --only <gate>`, `scripts/check-prometheus-rules.sh`, `scripts/check-runbooks.sh`, `scripts/runbook-manifest.sh --write`, `scripts/check-spdx.sh`, `scripts/check-gitleaks.sh`, `scripts/check-gosec-ratchet.sh`, `scripts/check-netip.sh`, `scripts/compliance/check-service-auth.sh`, `go test -race ./pkg/auth/... ./cmd/kanban-app/... ./cmd/dashboard-backend/...`, `go test ./deploy/nftables/`, `scripts/ci-local.sh`.

---

*S-WEST-GROUND Battle Plan, forged 2026-11-15 (Phases 0 to 3), finished 2026-11-15 (Phases 4 to 12, appendices; Phases 0 to 3 regrounded against `1393e5ee`)*
*13 Phases. 324 Steps. The databases leave the spinning disk, the memory comes back, the doors close, and every setting has one file and a check.*
