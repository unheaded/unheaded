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

### Phase 10 merged

Harness `cmd/perf-benchmark -scenario e2e-trace,container-start,http-load,http3,all` (perf/v1 report);
`scripts/check-perf-regression.sh` (16/16 fixture tests); VERDICTS H1-H8 scored (H5 SUPPORTED with a protocol
deviation: llama.cpp measured, not vLLM; H3 untested: 2 of 10 trials); AF_XDP label fixed. **No number measured
yet**: `docs/BENCHMARKS.md` section 5 reads "pending"; no `docs/policy/perf-baseline.json`.
Premises that were false: Wotan `auto_approve` holds display names (added `perf-benchmark`, not `bench.perf`;
Phase 14 attack-surface row); dashboard-backend never ingested `bench.*` (its eBPF ingestor now also consumes
`bench.perf.packet`, RFC 2544 addresses 198.18.0.1->.2, shows as one bench flow); container-start uses its own
`steel-echo-bench-<i>` instances, not Sword's.
LIVE: before 217, restart Wotan (reloads bind-mounted `configs/wotan.yaml`) and rebuild/up dashboard-backend; 217-218
as written (http3 will be `skipped`: west's curl 8.14.1 lacks HTTP/3, no Go HTTP/3 client); 219-220 after E-D:
`CUIRASS_API_KEY=... go run ./cmd/perf-benchmark -scenario container-start -allow-lxd -iterations 5 -output /tmp/perf-cs.json`
(Cuirass now also accepts a minted token); 225 `check-perf-regression.sh --update /tmp/perf-*.json`, then without.

### Phase 12 merged; Phase 11 merged (3882e911)

12: nothing signed. Attestation drafts in `docs/compliance/drafts/attestations/`; Stevie signs each with
`scripts/compliance/sign-attestation.sh <name>` (E-C); `expires_at` = each control's freshness window (access review
90 days, decision D10). Decisions D1-D10 with defaults: `docs/compliance/privacy/decisions-for-stevie.md`. SSP
`docs/compliance/SSP-generated.md` has 49 sections (50 once Phase 3's UH-SLO-01 lands: re-run `go run
./cmd/compliance-evidence ssp -out docs/compliance/SSP-generated.md`); 24 FAIL controls today (plan expected 13).
Rebuild exercise is a draft runbook, scheduled not run. `scripts/east-bootstrap.sh` still assumes NixOS and
`fd00:dead:beef::`; east runs Ubuntu 25.10. New items without M-ids: eBPF "no payload bytes leave the kernel" static
check (DPIA R1); backup of `/etc/unheaded`, `/var/lib/unheaded`, ClickHouse logs (contingency C1/C2).

11: `pkg/healthreport` in all 12 discovered services; `system.health.remediate` advisory (no subscriber acts);
Vector `unheaded_ssh_accepted_total`; `detection.yml` (SSHLoginNewSource uses "seen in 30 d", since `increase()` cannot
fire on a new address; outbound watches enp*/wlp* with a 256 KiB/s floor; burst points at a read-only runbook).
Vote change: a service's own report is a vote, so during a partition a service with healthy `/health` reads 50 %
(ERROR) not 100 % (PANIC); a down service's vote expires in 90 s. RT-32 (`CPU_MAP` single-writer, not enforced)
proposed lab-accepted: needs Stevie. LICH id collision: ADR-064 pre-registered 014-016; 015/016 reused since; LICH-017
reserved for the baseline agent. LIVE 235-236 (E-D): deploy reporter builds on both hosts, read both topics, paste one
consensus message into ADR-029; rollout also needs Vector restart (both hosts), Prometheus + Alertmanager reload.
Expect one SSHLoginNewSource ticket digest of known operator addresses at first evaluation.

### Phase 1 Steps 19-43 merged (b5aedf68)

Premise corrections: kanban-app's and timeguru's `CREATE TABLE IF NOT EXISTS` create their own local SQLite files,
not Well tables (ERDA 3.3 left them on purpose; Step 27 n/a, timeguru has no Well backend). The real gap: kanban-app
silently fell back to SQLite when the Well answered but was unmigrated (split board); now `RequireMigrations` 003/007
and a fatal exit; unset/unreachable Well still falls back for dev. New `038_app_ledger_read.sql` (superuser; app_kanban
SELECT on the ledger). Rollback cannot be `pg_restore --clean` (leaves the batch's new tables; ledger is append-only and
superuser-owned): batch-rollback drops and re-creates each touched database as the superuser and drops roles the batch
created. `well_health.sql` does not exist; the index-drop script carries its own read-only query and refuses with
index stats under 28 days old. Appendix C: this sprint's new migrations start at 039.
Coordinator re-ran `scripts/well/test-batch-apply.sh`: all checks passed (dry run lists exactly 030-038, part-way
failure + rollback, real batch, rollback, re-apply).
**LIVE 44-50 (E-A):** do NOT redeploy kanban-app before the batch (without 038 it refuses to start). `batch-apply`
backs up to `/srv/backups/unheaded-well/pre-batch-<UTC>/` via `db-backup.sh` as `well_backup` (needs write access, or set
`WELL_BATCH_BACKUP_ROOT`). Follow-on: first live window after 2026-11-15, run `scripts/well/index-drops-after-2026-11-15.sh`,
commit the reviewed DROP list as a `notx` migration per database. Retention (Phase 4): `zhen_agent_runs.card_id` references
`kanban_tasks` with no ON DELETE; purging cards must not orphan run history.

### Phase 3 merged (107ef3fe)

Migration `040_ops_slo_definitions.sql` (plan's 037); column `slo_window` (`window` is reserved); history via a
SECURITY DEFINER trigger; ops_writer has DELETE (`-sync-well` removes an SLO that left the YAML); 8 SLOs (ci-develop
added since the plan); objectives limited to two decimals (numeric(6,4)). `alert-policy -check` compares YAML and the
Well when `WELL_DB` is set. Compliance: `slo` evidence kind, UH-SLO-01 (50 controls; SSP regenerated with its 50th
section). **UH-SLO-01 reads FAIL** on today's data (spent: wotan, dashboard, well, well-backups, log-shipping;
passing: overlay, kev-feed; ci-develop no series yet), so failing controls go 24 -> 25 at the next collection.
CI: `test-slo-definitions.sh` runs in the well-schema job (throwaway Well); `ci-local.sh` runs a live `-check` when
`WELL_DB` is set. LIVE 105-106 after E-A applies 040: `WELL_DB=unheaded_ops WELL_PASSWORD=<OPS_WRITER_PASSWORD> go run
./cmd/alert-policy -sync-well`, confirm 8 rows, then `-check` (no drift). Flake seen under load:
`pkg/deploy/rollback TestManager_Rollback_DuplicateInProgress` (passes 20/20 alone).

### Phase 2 merged (5ceef55f)

Migration `039_ops_deploy_chaos.sql`: `deploy_history` + `deploy_history_transitions` (status/times per transition,
whole record as jsonb), `chaos_journal` + `chaos_journal_transitions` (a revert is a new row), views
`deploy_history_current`, `chaos_journal_open`; append-only triggers; rows per host (`KINGDOM_NODE`, else hostname).
No `actor` column (would touch auth). `WELL_DB` set but Well unreachable or 039 missing refuses to start (never falls
back); file state imported into the Well once. `revert-all` reads every open entry (`OpenOlderThan` exists, unused).
**E-A order:** apply the batch (now 030-041, 12 files) BEFORE rebuilding Sword: compose sets `WELL_DB` for it.
Chaos-controller: put `WELL_DB` in a host's `chaos.env` only if its Well starts before the unit and outlives it
(else ExecStopPost `revert-all` exits 2); east stays in file mode. LIVE 78-80: Sword has no dry-run, so 78 needs a real
rolling deploy of steel-echo.

### Phase 4 merged: retention

Migrations `042_retention_runner.sql` (postgres + unheaded_app, branching like 007) and `043_ops_retention.sql`
(`retention_runs`, `retention_archive`); pending batch is now **030-043 (15 files)**, and 042/043 must be applied before
Step 128. `cmd/well-retention` archives then purges; `compliance/retention/classes.yaml` says `ratified: false` and live
runs exit 2 until it says true (dry runs always work). Premises corrected: zhen_captures age is 180 days (033's
trigger; the job calls 033's purge and refuses a disagreeing classes file); `llm_requests`,
`service_health_transitions`, `audit_events` are append-only (never deleted; partition-trigger check only); no
well-drill deb existed, so a separate `unheaded-well-retention` deb. `_pg_log` in zhen_app.py redacts every conversation
row with `raft/zhen_redact.py` (stores nothing if the rules cannot load); `pkg/zhencapture` gained an `email` pattern
(captures now redact emails; zhen-flywheel tags instead of quarantining). Cards referenced by `zhen_agent_runs` are held.
Live read (Step 112): 126 of 226 conversations are older than 90 days, none older than 180; service_health_transitions'
oldest row turns 90 days on 2027-01-16. **E-C (Stevie):** ratify `docs/policy/retention-policy-v1-2026-05-06.md` §11
(proposed conversations 90 d, health transitions 90 d); then flip `ratified: true` in the same commit as any age change.
LIVE 127-131 (role password file 0400, deb install + dry run, enable timer only after E-C). Not done: a
`check-gates-can-fail.sh` entry for `test-retention.sh`. Connection budget 44/47; `app_auth`'s 40-connection limit is
not in that table (pre-existing gap).

### Phase 13 code merged (b3d4407a); E-E filed

Parity: 104/104 subtests, 52 goldens of the retired `remediate.sh` output (`TestExecutorMatchesRetiredScript`);
`remediate.sh` is a shim (prints the `unheaded-remediate` command, exit 2). New fixes rocm-log-perms, aide-install,
apparmor-enforce (Xorg only); `unheaded-remediate check` and `-record-existing`. Bug fixed: undo reloaded a fix's service
before restoring its files. The plan's `checks:` field already existed as `closes:`; the executor already lived in
`pkg/compliance/executor`. Ansible: hand-written roles in `deploy/ansible/kingdom/` (the compliance tree is generated);
ansible-core 2.21.5 + resolvelib admitted to the dependency baseline with reasons. Enforcing path behind a flapping
breaker + preinst gate; every mode file stays audit. LICH-017: 7 cases, 0 bypasses. E-E filed in
`docs/compliance/privacy/decisions-for-stevie.md`. LIVE 280-282: east then west debs, `-record-existing` for the 8 lab
fixes (`-applied-by "remediate.sh (2026-10-13)"` for journald-caps and time-p2p), apply the new fixes; then sync the Well
mirrors of fixes/profiles (`compliance-evidence well`). ADR-098 line 193 still names `scripts/host/remediate.sh`
(generated file: regenerate after the register changes). Flaky dashboard eBPF broadcast test fixed (33f96fdc).
