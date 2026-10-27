<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-WELL-SPRING progress log

Plan: `docs/battle-plans/S-WELL-SPRING-battle-plan.md`. This file is the session log the plan refers to, and where
`[STUCK]` / `[ESCALATE]` markers live; step ticks live in the plan.

Skip Protocol acknowledgment: "I will skip any step after 3x its estimate or 2 failed debug attempts, mark
it [STUCK] with what I tried, commit known-good state first, continue with the next step that does not depend
on it, and finish with a STUCK/ESCALATE report ordered by how many steps each item unblocks."

## Session S24, 2026-10-27 evening (unattended, Stevie's "churn unattended multi agent")

Base: develop `1f44cc46` (S-CLEAN-SLATE shipped; Go known failures 0). Same pattern as Sprint 1: a coordinator
on develop and worktree agents under one brief (`EXEC-BRIEF-v2`: reset to develop first, never push, no host
changes, no shared-ledger edits, gate exit codes read directly). Phase 0 premises are checked per slice by each
agent, since SITUATION was written at `a7fe3acd` and Sprint 1 changed many of them.

First wave: Phases 6-7 (watchdog, postmortems, alert rules) in a worktree agent; next: Phase 8 (metrics) after S-IRON-GATE Phase 11 lands, Phases 10-12 beside; the coordinator owns Phase 1's ledger steps and every live Well window (E-A, E-B).

### Phase 9 (coordinator): Steps 197-207 done; 208-209 wait for E-D

- 198 decision 24 default: first-party container stdout via the compose journald driver. Scope: the twelve
  first-party (build) services; third-party images and Vector itself keep json-file caps (Vector must not ingest
  its own output). Gate: journald accepted only while the profile of every host in `compliance/hosts/` applies
  `journald-caps`; proved failing three ways. Meta-gate probe regex updated for the alias line.
- Vector labels container lines `unit = CONTAINER_NAME` (the plan said `.service`; the existing schema and metrics use
  `unit`). **Step 209's query should read** `unheaded_log_lines_total{unit="unheaded-wotan"}`, not `{service=...}`.
- 203 decision 27 default: ADR-076 re-scoped (one dashboard-backend endpoint), ADR-086 Superseded by Vector,
  ADR-092 amended, ADR-INDEX rows updated.
- 205: `config/vector/vector-east.yaml` (transforms verbatim from vector.yaml; `test-vector-config.sh` fails on any
  difference), disk buffer 256 MiB `when_full: block`, exporter on 192.168.13.1:9598; `deploy/systemd/vector-east.service`
  (unheaded-vector, uid 16706 pinned in sysusers, `SupplementaryGroups=systemd-journal`, full sandbox). Password via
  `EnvironmentFile` (0600 root) rather than the plan's `VECTOR_SINK_PASSWORD_FILE`: Vector 0.43 reads `${VAR}`, and
  systemd reads the file before dropping privileges. `.deb` pinned in tool-pins.sh; apt.vector.dev and
  packages.timber.io serve different bytes for 0.43.1-1 (pinned apt.vector.dev's). Recorded in THIRD_PARTY.md
  (MPL-2.0, separate process). Vector 0.43.1 is old on both hosts; bumping image + deb together is a follow-on.
- Host window E-D also needs: west ClickHouse listening on 192.168.13.2:8123 (east only, nft), and a Prometheus
  scrape job for 192.168.13.1:9598.
