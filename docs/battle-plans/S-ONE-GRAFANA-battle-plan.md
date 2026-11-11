<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-ONE-GRAFANA BATTLE PLAN — 3 Phases, 14 Steps

**Date**: 2026-11-11
**Sprint**: S-ONE-GRAFANA. ADR-117, on Stevie's word of 2026-11-11 ("go for it in background", after "why 2 grafanas?"). Not a lane of `SPRINT-SERIES-2026-11-04.md`; ADR-112's freeze is lifted for this ADR only.
**Prerequisite**: `develop` at `6793ce20`.
**Target**: west runs one Grafana (11.5.1, port 3001) and one metrics store (VictoriaMetrics, 8428); the monitoring stack is the alert path only (Prometheus as rule evaluator, Alertmanager, alert-mailer); a host gate fails when a container and its compose pin disagree.
**Branch**: `sprint/s-one-grafana` in worktree `~/tmp/wt/S-ONE-GRAFANA`. Phase 1 changes files only. Phase 2 is attended on west.
**Scope fence**: no new image, plugin, module or service. No rule evaluator is invented. Prometheus, Alertmanager, alert-mailer, the silence and `mailer.mode` are not touched. The log store (ClickHouse or the Well) is S-E1-LIVE Evening 16's question, not this plan's.

## GROUND TRUTH

`docs/sweeps/ONE-GRAFANA-INVENTORY-2026-11-11.md`. In short: both Grafanas served the same 14 files from the main store; Loki 0 streams; second VictoriaMetrics 0 series; Prometheus 46 targets with 31 down; no native rule evaluator exists.

## Phase 1: files (done, unattended)

- [x] **Step 1**: inventory of west, read-only. `docs/sweeps/ONE-GRAFANA-INVENTORY-2026-11-11.md`.
- [x] **Step 2**: main Grafana provisions eight dashboards with a fixed datasource uid; plugin fetch at start disabled. `9c815de8`.
- [x] **Step 3**: VictoriaMetrics scrapes both hosts over the overlay (network `scrape`, `-enableTCP6`); Prometheus drops four dead jobs and forwards only rule results and its own jobs. `99e18bbb`.
- [x] **Step 4**: proofs in throwaway containers: Grafana loads eight dashboards and no plugin; VictoriaMetrics reaches 21 of 27 targets; panels answer (kingdom services 18/18, host overview 8/8, postgres 8/8, host resources 9/9, wireguard 3/3, infrastructure 12/12, SLO 5/5, compliance 0/4 with its emitter not running).
- [x] **Step 5**: second Grafana, Loki and second VictoriaMetrics leave `monitoring/docker-compose.yml` with their config, docs and log-source rows. `2613b0c2`.
- [x] **Step 6**: `docker/telemetry/` deleted (unpinned, started by nothing). `8162fa55`.
- [x] **Step 7**: `scripts/check-image-drift.sh` and its meta-gate row. `5c300889`.
- [x] **Step 8**: third-party note, runbook, ADR-117 status, index row, this plan.

## Phase 2: west (attended; `docs/runbooks/one-grafana-cutover.md`)

- [ ] **Step 9**: merge to `develop`, then runbook steps 0 to 2 (lint, recreate Grafana).
- [ ] **Step 10**: runbook steps 3 to 6 (recreate VictoriaMetrics, check targets and the overlay publish, reload Prometheus, look at the dashboards).
- [ ] **Step 11** (Stevie's word): runbook steps 7 and 8 (copy the old Grafana database, remove the three containers). `check-image-drift.sh` passes.
- [ ] **Step 12** (Stevie's word): runbook step 9 (remove the three volumes), a day later.
- [ ] **Step 13**: runbook step 10 (LAN forward on 3000, next.md rows).

## Phase 3: waits on an answer

- [ ] **Step 14**: Prometheus retires when Stevie answers ADR-117's evaluator question (keep until ADR-116's warden, build a native evaluator, `vmalert`, or Grafana alerting). Nothing is built toward it here.

## ESCALATIONS (Stevie)

- **E1.** The evaluator question, ADR-117 "Status 2026-11-11".
- **E2.** The four smaller questions in the same section (Grafana's calls to grafana.com, ClickHouse plugin, the seven unported dashboards, sophia-eye's Prometheus).
- **E3.** `tomb/dark-mirror/docker-compose.yml` carries a literal Grafana admin password that no secret gate matches. Lab default, or used anywhere else?
- **E4.** next.md question 19 (`syslog-relay` on west) is still open; its target is down in both scrape files until answered.

## Not verified until Phase 2

- The publish of 8428 on the overlay address once the container has an IPv6 network (runbook step 3 checks it and names the rollback).
- The `scrape` bridge on west: it exists only after `compose up`. The proof used the monitoring stack's bridge, which has the same shape.
- `tests/scripts/test_image_digests.py`: west has no pytest; CI runs it.
