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

### Phases 6-7 merged (c2c7c0fb) and Phase 8 merged (374984fc)

6-7: Watchdog rule; heartbeat on its own port 19131 (`/heartbeat/{host}` only, loopback until `HEARTBEAT_LISTEN`);
postmortem template + four incidents (Well postmortem dated 2026-09-30, the outage; 10-13 was the audit);
dead alerts deleted; trace-collector moved to its own scrape job for both hosts (16671 was scraped as "wotan");
alert-mailer now scraped; `alert-policy -runbooks` gate (every alert names a runbook); ci-develop SLO with its
own runbook; textfile collector; error-budget dashboard 08. uids 16711/16712.
LIVE (E-D / monitoring reload), in order: ci-status sysusers + script + timer + `setfacl` on
`/var/lib/unheaded/textfile` BEFORE Prometheus loads the rules (missing data counts as bad and would mail);
heartbeat token on west (0400 uid 16710) and east; `HEARTBEAT_HOSTS=east`, `HEARTBEAT_LISTEN=192.168.13.2`;
east sysusers + heartbeat timer; then `docker compose up -d prometheus alertmanager alert-mailer grafana`.
Expect mail: ci-develop ticket while CI is red; five ErrorBudgetBurn tickets firing since 17:26:47Z look like a
Prometheus restart, not a burn.

8: heimdall-daemon `-metrics-addr` (default 127.0.0.1:19101), counters are `heimdall_drift_publish_{failed,dropped}_total`
+ `heimdall_scan_duration_seconds`; `pkg/logagg.RegisterCollectors`; `WriteMetrics` deleted with a golden test pinning
the old bytes; 8 pqc + 4 secrets metrics deleted (not 9/12). `HeimdallDriftPublishFailing` landed in collector.yml
(record-only; no heimdall unit runs, so both targets read `up == 0` until one runs on an overlay address).
Stale descriptor: root `services.yaml` lists `shield-waf` on 19101 (Shield is 19009), not changed.
Merge fixes by coordinator: Well lint red since Champion Phase 4 (verb table read as DDL; files excluded with a
reason, ca501174); pkg/uids walker read agent worktrees (skips `.claude` now); ruff red (67cf4b07).
