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
| W-01 | Architect A9, R1 layout 1; Erda rec 3; Archimedes rec 3 | G2, G4, G6 | database volumes on the NVMe | Phase 5 | | | not started (no before: Phase 1 waits on the week). Repository side ready: `deploy/host-settings/west-fstab.fragment` (LV + five binds), `deploy/systemd/docker.service.d/10-unheaded-data.conf` (`ba04c2e9`) |
| W-02 | Architect R1 "must be true first" | G5 | backup sets off the NVMe | Phase 5 | | | not started. Repository side ready: the `/srv/backups` bind line in the fragment (WG2 A) |
| W-03 | Architect A8; BlackMage F1; Archimedes F9 | G7 | swap off `sdb`, three files deleted | Phase 4 | | | not started. The drift gate's `absent` rows and swap row will read it (`west.tsv`) |
| W-04 | Architect A3; Archimedes F5 | G8 | huge pages released | Phase 4 | | | not started. Repository side ready: `deploy/sysctl.d/99-unheaded-west.conf` says `vm.nr_hugepages = 0`; `scripts/bootstrap-llm-lab.sh` installs it instead of writing 1024 (`17c58623`) |
| W-05 | BlackMage F9; Architect A14 | G10, G11 | input filter | Phase 6 | | | not started |
| W-06 | BlackMage F9 (task list, dashboard `/metrics`) | G12, G22 | WG3 A: source-gated routes | Phases 3, 7 | | | not started (no before: Steps 189/191 from the LAN are Phase 6). Code ready: `pkg/auth` `AnonymousFrom`/`SourceGated`/`GatesRequest` (`480f23d5`), kanban `/api` + `/metrics`, dashboard `/metrics` (`fd7c26de`); inert until Step 85's compose line (held lane) reaches develop in Phase 7 |
| W-07 | BlackMage F10 | G13 | WG4 | Phase 6 | | | not started |
| W-08 | decision 22; Archimedes 1.2 | G19 | `iperf3`, the link and WireGuard measured | Phase 9 | | | not started |
| W-09 | BlackMage F19; Architect A10 | G15 | pid limits | Phase 7 | | | not started (no before: R6 is Phase 1). Code ready on the held lane `sprint/s-west-ground-compose` `dd590f29`: `x-ground` anchor (512, ClickHouse 2048) on all 23 + 4 services, sum 15,360 = 13.3% of threads-max 115,473; `scripts/check-compose-pids.sh` + test (14 cases), registered and proven to fail |
| W-10 | Architect A11, R7, 4a | G16 | slices, structure only | Phase 7 | | | not started. Code ready: four `.slice` units with accounting only, two drop-ins (`3f074303`); `cgroup_parent: unheaded-env.slice` on the held lane. No reader of `system.slice/docker-*` found (Step 92) |
| W-11 | Architect A5, A12 | G8, G17 | one source for host settings, drift gate | Phases 2, 8 | 2026-11-15 21:50Z, `scripts/check-host-settings.sh --host west` as govan (no root, read only; the timer's run is root and reads 3 more files): files 11 DRIFT (8 not installed yet, 3 swap files present), 3 SKIP (`/etc/wireguard/wg0.conf`, `/etc/frr/frr.conf`, `/etc/frr/daemons`: root only); sysctl 44 OK, 1 DRIFT (`vm.nr_hugepages`), 1 SKIP (`net.core.bpf_jit_harden`); mounts 6 DRIFT (not made); swap 1 DRIFT | | code ready (`652e33f8`, `cf680986`); after pending Phase 8 (root, attended) |
| W-12 | Architect A16 | G18 | read-ahead and scheduler | Phase 10 | | | not started |
| W-13 | grounding G20 | G20 | startup scripts and the override | Phases 0, 2 | 2026-11-15 21:43Z, static (Step 15 needs sudo for `.env`; not run): the host's `docker-compose.override.yml` sets `restart: "no"` on 18 services; `docker-compose.yml` sets `unless-stopped` on them; both scripts call `docker compose -f docker-compose.yml`, and Compose loads the override only without `-f` | | code ready (`41bfb2f5`): both scripts name the override when it exists. After pending: Step 15's two counts, then Step 16's policy file after a scripted start (attended) |

## Log

- 2026-11-15 21:41Z: Step 1 read (ADR-147 answers 1, 11, 12, 13, 16, 22; Grounding; seat rows A3, A5, A10-A16, F9, F10, F19). Step 12: worktree at develop `8d35785d`. Step 13: this file.
- Grounding recheck at `8d35785d`: every file Phases 2-3 touch is as the plan says (bootstrap heredoc at 809-886, sysctl live file 78 lines, 23 + 4 compose services, `west.nft` lines 37-60, auth BFF files). `check-unit-ceilings` registration is at line 1392, not 1402.
- 2A (Steps 42-45) `ba04c2e9`: Step 44 printed `5`, no BAD, no MISSING.
- 2B (Steps 46-51) `17c58623`: Step 48 printed `8` (SPDX, banner x2, huge pages x4, duplicate). `bash -n`, `shellcheck -S error` clean; `nr_hugepages` count in the bootstrap script 0.
- 2C (Steps 52-55) `3f074303`: `systemd-analyze verify` silent; share-key count 0 in all four slices. `Documentation=` uses the repository's GitHub URL form (as the other units).
- 2B' (Steps 56-59) `41bfb2f5`: run on the static reading in W-13's before (Step 15 not run). `OVERRIDE_FILE` 3 per script; `bash -n`, `shellcheck -S error` clean. The scripts were not executed (`kingdom-startup.sh` posts an Alertmanager silence even with `--dry-run`).
- 2D (Steps 60-78) `652e33f8`, `cf680986`: test red (gate missing), then green 29 of 29 (the plan's ten cases plus a second contains case, the textfile and the usage errors); `shellcheck -S warning` silent; self-check PASS (1 manifest, 14 rows); `check-gates-can-fail.sh --only check-host-settings` PASS. Units: `check-unit-ceilings`, `-binaries`, `-flags`, `check-systemd-units`, `check-scrape-publish` PASS; `check-log-sources` needed a journal-unit row (added, status gap). Alerts: `cmd/alert-policy` refuses `urgency: ticket` in a hand-written rule, so both rules carry `urgency: none` and are promoted to ticket in `observability/alerting/policy.yaml`; `alertmanager.yml` regenerated; `go run ./cmd/alert-policy --check` and `-runbooks` PASS. Runbook: `check-runbooks` 0 new findings (needed `command:` on every step and a dry-run test, `tests/runbooks/test_host_settings_drift.py`, 5 pass); manifest rewritten (93 runbooks); `pytest tests/runbooks` 445 passed, 4 skipped; SPDX OK.
- **promtool not run.** `scripts/check-prometheus-rules.sh` runs promtool in a container (`docker run --rm --network none`); I started it once at ~21:47Z, saw the container use, and stopped it within two minutes. The four `host-settings_test.yml` cases are unproven until it runs after the evidence week.
- 2E lane (Steps 79-94) `dd590f29` on `sprint/s-west-ground-compose`: test red then 14 of 14; `check-compose-pids` PASS (15,360); hardening, log-caps, publish-bind, bind-nesting, image-digests ok; `pkg/uids` ok; Step 87's numbers 13,312 and 23; meta-gate `--only check-compose-pids` PASS. Step 91: develop had not moved; no `system.slice/docker-` reader (Step 93 not needed). Lane not in develop (Step 94 printed 0).
- 2F (Steps 95-98): `west.nft` matches decision 16 for 9100, 9110, 9323, 19120 (not in the LAN set; open on `docker0`, `br-*`, east's link, `wg0`). Step 97 not run (ruling A). `go test ./deploy/nftables/` ok; `unshare -Urn nft -c` NFT_OK.
- Steps 100-101: develop (`8d35785d`, unmoved) merged in; SPRINT_GATE_OK with `go run ./cmd/alert-policy --check` in place of `check-prometheus-rules.sh` (container). **Step 102 not run: ready for the coordinator** (merge `sprint/s-west-ground` to develop; the compose lane stays held). Step 103 on the branch: six files listed, `x-ground` count 0.
- Phase 3 (Steps 104-119) `480f23d5`, `fd7c26de`: red (`cfg.AnonymousFrom undefined`; then the service tests at 200 where 401 was wanted), then green. 19 `pkg/auth` cases (the plan's 11 rows plus 5 path and credential edges, `ParseAnonymousFrom` 7 inputs, a bad env value refuses to start); kanban 7 cases; dashboard 7 cases including "unset changes nothing". gofmt clean; `go vet` clean; `go test -race` ok on `pkg/auth/...`, `cmd/kanban-app/...`, `cmd/dashboard-backend/...` (13 packages); gosec ratchet PASS; `check-netip` 68 (baseline 68); `check-service-auth` PASS (14); golangci-lint 0 issues on the three trees; `go build ./...` and `go vet ./...` clean.
- **Grounding finding (G22's sibling, fixed in Step 114):** dashboard-backend's `wrapAuth` serves every GET outside `/api/` and `/ws` before the auth chain (`isUIRead`), `/metrics` included, so gating it in `pkg/auth` alone would never have reached it. `auth.ServiceRoutes.GatesRequest` is now asked first; the dashboard test proves 401 from the LAN.
- Step 118: `wiki/Security.md` paragraph under "Default Deny"; `CLAUDE.md` Authentication Framework sentence.
- Steps 120-122: develop unmoved, merged in; Step 115's gates stand. **Merge to develop not run: ready for the coordinator.** Phase 3 exit on the branch: `TestSourceGatedOpenRoutes` ok; `docker-compose.yml` names `AUTH_ANONYMOUS_FROM` 0 times.
- `scripts/ci-local.sh` not run: it starts containers (throwaway Well, haproxy image) and runs `alert-policy -check` against the live Well, both out of bounds until 2026-11-17 18:34Z. Run it on the merged tree after the week.
- Commits: all signed (`G`). Trailer: Co-Authored-By only (this run has no session URL to add).

## For the coordinator (merge order)

- `sprint/s-west-ground` is ready to merge after plans 1-3. Files a sibling lane is likely to touch too: `scripts/check-gates-can-fail.sh` (one function, one registry line), `observability/alerting/policy.yaml` and the generated `monitoring/alertmanager/alertmanager.yml` (two promote entries: regenerate with `go run ./cmd/alert-policy` after the merge, never hand-merge), `config/logging/sources.yaml` (one row), `runbooks/MANIFEST.sha256` (regenerate with `scripts/runbook-manifest.sh --write`), `pkg/auth/service_handler.go`, `pkg/auth/setup.go`, `cmd/kanban-app/auth_bff.go`, `cmd/dashboard-backend/internal/server/auth_bff.go`, `CLAUDE.md`, `wiki/Security.md`, `scripts/kingdom-startup.sh`, `scripts/kingdom-shutdown.sh`, `scripts/bootstrap-llm-lab.sh`.
- `sprint/s-west-ground-compose` (`dd590f29`) is **held until Phase 7**. It touches `docker-compose.yml`, `monitoring/docker-compose.yml` and `scripts/check-gates-can-fail.sh`; plans 2 and 3 remove services from the same compose file. At Phase 7: merge develop into it, rerun `scripts/check-compose-pids.sh` (Step 87's sum is 512 x (N - 1) + 2,048 for the services left) and add `<<: *ground` to any service added since.

## For Stevie

- **Rulings taken on the plan's recommendation, Stevie to confirm:** WG1 A (repository side: all five volumes in the fragment and the drop-in), WG2 A (the `/srv/backups` bind line), WG3 A (Phase 3 and the compose line on the held lane).
- `west.nft`'s LAN set still lists 16666 (doom-bridge, moved out 2026-11-04, nothing listens) and 20103 (Zhen's UI: decision 15 keeps it off a public side; the house LAN is not public). Left as you set them on 2026-11-03; say if either should go.
- `runbooks/MANIFEST.sha256` was rewritten for the new runbook `infra/host-settings-drift.yaml` (draft); the seal is yours.
- `kingdom-shutdown.sh` still uses `pkill -f` and `compose down`; this plan does not use either script. Their rewrite is yours to schedule.
- `scripts/check-prometheus-rules.sh` is a container gate: it cannot run while the evidence week forbids containers, so the new rule tests wait for 2026-11-17 18:34Z.

## Blocked

- Steps 2-11, 14-24 (Phase 0 except 1, 12, 13) and Phase 1 (Steps 25-41): `[BLOCKED: needs Stevie / evidence week]` (sudo reads, live readings, rulings in the room). Step 11 was read once (read only) for WG1's note. Step 14 (S-E1-LIVE's week closed) waits on Step 6.
- Step 15 (does `-f` drop the override): needs sudo to read `.env`; W-13 code went ahead on the static reading, its after waits.
- Steps 75-76 promtool half, and every `scripts/check-prometheus-rules.sh` run: container gate, after the week.
- Steps 102 and 120 (merge to develop): the coordinator's.
- Phases 4-12 (Steps 123-324): attended sittings A to D, not in this run.
