<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# The Well: ERDA audit and battle plan (2026-10-13)

## THE WELL REPORT: The Well (PostgreSQL) @ develop 643a12e0, 2026-10-13

**Verdict:** a fresh Well now bootstraps cleanly, but it still has no backup
of the databases that matter, no migration ledger, a superuser that owns
everything and serves the kanban app, writable "append-only" history, and a
deployed config that is the image's defaults rather than the repo's.
**P0 4 · P1 9 · P2 8 · P3 4** (consolidated below; `well_lint.py` raw count:
191 findings, P0 3 / P1 46 / P2 64 / P3 78).

Baseline: the ERDA skill's `well-baseline-2026-10-08.md` (read on `main`
0f443de). Each baseline row was re-checked on `develop`; the B-xx column
says which.

| ID | Sev | Where | Finding | Why it bites | Fix |
|----|-----|-------|---------|--------------|-----|
| W-01 | P0 | `scripts/db-backup.sh:13` | **Backups miss the real databases.** It dumps only the legacy `unheaded` DB; `unheaded_app/ops/config` and the roles (globals) are never dumped, nothing is copied off the host, no restore was ever run. Newest dump 2026-09-22 (manual). (B-04, verified) | A lost volume loses kanban, zhen memory/conversations, audit, compliance state. Law 3 | Phase 1 |
| W-02 | P0 | `docker-compose.yml` kanban-app env; FND-008 | **A service runs as the superuser**: kanban-app uses `WELL_USER=unheaded` (the image's `POSTGRES_USER`, superuser) on DB `unheaded`, `sslmode=disable`, while `app_kanban` exists. (B-05, lint P0) | A kanban bug or injection is cluster-wide superuser. Law 7 | Phase 2 |
| W-03 | P0 | `001:18`, `003:30`, `pkg/database/kanban_store.go:48` | **No ledger; tables defined several times with different shapes**: `kanban_tasks` x3, `zhen_conversations` x3 (verified: 003's shape wins, 010's `CREATE TABLE IF NOT EXISTS` is skipped), `audit_events` x2, `kingdom_config` x2, `timeline_milestones` x2, `zhen_actions` x2 (009 vs `pkg/champion/pgstore/schema.sql`). DDL also runs at startup from 9 code sites. (B-01, verified) | The first writer wins silently; a schema change cannot be made safely or known to have happened. Law 1 | Phase 3 |
| W-04 | P0 | `pkg/audit/storage/database.go:64` | **MySQL DDL** (`INDEX x (col)` inside `CREATE TABLE`); PostgreSQL rejects it. Also a format-verb table name and 6 `fmt.Sprintf` queries. Unreached today. (B-02, lint) | The moment anything wires it, audit storage fails at startup | Phase 5 (fix or delete) |
| W-05 | P1 | `004:110`, `005:107` | **"Append-only" history is writable**: `ops_writer` has UPDATE/DELETE on `audit_events` and `service_health_transitions`; `config_admin` on `kingdom_config_history`. (B-06, verified on a fresh bootstrap.) The 2026-10-01 compliance tables (013/014) do this right: INSERT/SELECT plus triggers that refuse UPDATE/DELETE/TRUNCATE even for the owner | Tamper-evidence claims fail. Law 6 | Phase 2, copy 013/014's pattern |
| W-06 | P1 | every DB | **The superuser owns every table** (verified: all 23 tables in 3 DBs owned by `unheaded`); no per-schema owner role | Migrations and apps share one identity; no least privilege. Law 7 | Phase 2 |
| W-07 | P1 | `002:6-12`, `012:6` | **8 passwords in migration files** (`*_dev`; the defaults that ship). Compose falls back to `POSTGRES_PASSWORD=unheaded_dev`. (B-11, lint) | Repo rule: no credentials in the repo | Phase 2 |
| W-08 | P1 | compose `postgres` service | **The repo's `postgresql.conf` and `pg_hba.conf` are not deployed.** Verified on a fresh bootstrap: running config is the image's (`ssl=off`, `max_connections=100`, `shared_buffers=128MB`, `data_checksums=off`, no `pg_stat_statements`, `log_min_duration_statement=-1`, no WAL archiving), and `pg_hba` allows password login from any address on the Docker network. The repo files' own findings (SSL off, `172.0.0.0/8`, superuser over the network, 256MB `shared_buffers` in a 256MB container) describe files nothing reads. (B-09/B-10, verified) | You cannot reason about the Well from the repo | Phase 4 |
| W-09 | P1 | `db/init.sh` | **initdb is the only schema path.** It runs once on an empty volume; 013/014 (2026-10-01) reach an existing Well only by hand (COMPLIANCE-WELL step 21, still open). B-03 (fresh bootstrap broken) is **fixed on develop**: verified, a throwaway container with compose's mounts reaches "The Well — Ready" | Every later migration needs manual, unrecorded psql | Phase 3 |
| W-10 | P1 | `010:40-48`, `kanban_store.go:62-66`, `pgstore/schema.sql:25-35` | **Lock hazards in startup and migration DDL**: 8 non-concurrent index builds on existing tables, no `lock_timeout`, and `ADD COLUMN guid UUID DEFAULT gen_random_uuid()` (volatile default: full rewrite under ACCESS EXCLUSIVE on boot). (B-14) | A restart can block the kanban table for the length of a rewrite. Law 4 | Phase 3 |
| W-11 | P1 | 9 Go sites (lint `string-formatted-sql`) | **SQL built with `fmt.Sprintf %s`**: `pkg/audit/storage` (6), dashboard events, unheaded-cli, champion (2), iac/kubernetes, metrics, kenoma, wotan circuitbreaker | Injection risk unless every verb is a constant; each needs a look | Phase 5 |
| W-12 | P1 | `pkg/database/bridge.go:15` vs `cmd/akira/main.go:328` | **Health persistence is not wired**: the bridge expects `system.health.report` with `{host,status,response_time_ms}`; Akira publishes `system.health.reports` with another shape. (B-12) | `service_health_*` tables stay empty or wrong | Phase 5 (decide: wire or delete) |
| W-13 | P1 | Postgres image | **PostgreSQL 16.13** locally; current 16.x minors carry fixes (the baseline verified on 16.15). Major 16 is supported into late 2028 | Missed security/bug-fix releases | Phase 4 |
| W-14 | P2 | `unheaded_app.zhen_conversations` | **Duplicate indexes** (verified): `{idx_zhen_search, idx_zhen_conversations_search}`, `{idx_zhen_created, idx_zhen_conversations_created}`; 9 indexes total. 15 low-cardinality single-column indexes across 003/004/005. (B-13) | Every write pays for unused indexes. Law 2 | Phase 5 |
| W-15 | P2 | 004, 009, 010 | **No retention or partitioning** on append-only tables (`audit_events`, `service_health_transitions`, `zhen_conversations`, `zhen_actions`, `zhen_action_snapshots`). (B-16) | Unbounded growth; retention policy unenforced | Phase 5 |
| W-16 | P2 | 008, 009, 010, 011 | psql meta-commands (`\connect`, `\c`, `\set`) inside migrations. (B-17) | Ties files to psql and hard-coded DB names | Phase 3 |
| W-17 | P2 | `pkg/database/connect.go:28`, `services/wotan/.../pg_store.go:32` | Connection budget: pools of 10 per service against the deployed `max_connections=100` (3 superuser-reserved). Fits today (~6 services); the baseline's "exceeded" assumed the undeployed 50. (B-08, downgraded) | Breaks with replicas or more services | Phase 4 |
| W-18 | P2 | `005_config_schema.sql` | Config history trigger logs UPDATE only, not INSERT/DELETE; `change_reason` never set. (B-19) | History incomplete | Phase 5 |
| W-19 | P2 | `013:90` | `compliance_host_bindings(profile)` FK without an index | Small table today; a profile delete scans bindings | Phase 5 |
| W-20 | P2 | `cmd/zhen-rag/main.go:242`, `pkg/agent/agent.go:763` | `DROP TABLE` in application code | Destructive DDL outside the ledger | Phase 3 |
| W-21 | P2 | 001/003/005 | Type and NULL smells: 19 `serial`, 12 nullable `DEFAULT ''`, 13 nullable timestamps, 2 `timestamp` without zone, 1 `JSON`, arrays of ids (`depends_on BIGINT[]`). (B-15) | Two kinds of empty, no integrity on id arrays | Phase 5 |
| W-22 | P3 | `001_initial_schema.sql` and DB `unheaded` | Legacy DB and schema: 001 is no longer applied by `init.sh`, but the live volume (created earlier) may still hold 001-era tables, and kanban writes to DB `unheaded` | Two homes for kanban data | Phase 0 shows what is there; Phase 3 retires it |
| W-23 | P3 | `008_zhen_memories.sql` | `embedding BYTEA` cannot be searched in the database. (B-22) | Fine until similarity search moves into the Well (ADR-056) | ADR-056 |
| W-24 | P3 | `pkg/database/validate.go` | Treats any `172.*` as private. (B-21) | Misclassifies public space | Phase 2 |
| W-25 | P3 | ADR-016 | Says `Connect()` retries with backoff (it does not); lists `service_logs` (no migration). (B-20) | ADR drift | Librarian |

**Already better on develop than the baseline:** fresh bootstrap works (B-03);
the compliance schema (013/014) is append-only with triggers, TRUNCATE guards
and history; `pkg/database` has no credential default (FND-007, fixed
2026-10-01); Postgres publishes on 127.0.0.1 only.

**Why the live Well is down:** `unheaded-postgres` exited 255 at
2026-09-30 05:06:26Z, not OOM-killed, with no shutdown lines in its log:
dockerd or the host went down under it (dockerd restarted 05:06:42). Its last
log lines are the failed-superuser-login flood traced to FND-007. Data volume:
`unheaded-dev_postgres-data`. Postgres will crash-recover on start.

**Fix order:** 0 back it up and look → 1 backups that cover everything and
are restored → 2 access (roles, grants, secrets, hba/TLS) → 3 one migration
trunk → 4 deployed config → 5 schema hygiene. Backups first because every
later phase changes the live Well.

**Verified vs inferred:**
- *Executed:* `well_lint.py` over develop; a throwaway `postgres:16-alpine`
  bootstrapped with compose's exact mounts and env (no network, no volume,
  removed): `init.sh` completes; `well_health.sql` on all four DBs; catalog
  queries for roles, table owners, duplicate indexes, grants on history
  tables, `zhen_conversations` shape, `pg_hba_file_rules`, settings.
  Read-only `docker inspect`/`logs` of the stopped container.
- *Read, not executed:* backup script, pool sizes, bridge/Akira topics,
  ADR-016. Nothing was run against the live volume (the Well is stopped;
  starting it is Phase 0 and Stevie's call).
- *Not done:* live sizes, bloat, vacuum, sequences, top statements: they need
  the real Well (Phase 0).

**Handoffs:** Architect: deploy path for config (compose vs nix vs k8s
`the-well`), off-host backup target (east), TLS. Developer: migration runner
(stdlib-only, ADR-004/096), removing startup DDL. MoatGhost: W-01/W-05/W-15
map to UH-BACKUP-01, audit integrity, retention policy. BlackMage: W-02,
W-08 (any container on the `data` network can try passwords against every
role), W-11. Librarian: ADR-016 drift.

---

## Battle plan

Gates for every phase: `well_lint.py` no new findings (ratchet), the
throwaway fresh-bootstrap check passes, tests pass, and anything touching
the live Well has a verified dump taken first. Steps marked **[Y]** change a
live system and need Stevie's yes.

### Phase 0: back up, then look (needs The Well running)

- [ ] 0.1 **[Y]** `sudo docker compose up -d postgres` (COMPLIANCE-WELL Phase 0
      step 4). Expect crash recovery in the log, then "ready".
- [ ] 0.2 Before anything else: `pg_dumpall --globals-only` plus `pg_dump -Fc`
      of `unheaded`, `unheaded_app`, `unheaded_ops`, `unheaded_config` to
      `~/backups/well/2026-10-13/`; `pg_restore -l` each; copy to east.
- [ ] 0.3 `well_health.sql` on each DB as `huginn_reader` (pg_monitor);
      save the output beside this plan as the live baseline.
- [ ] 0.4 Shape diff: live schema (`pg_dump --schema-only`) vs a fresh
      bootstrap of develop; record every difference (expect 001-era tables in
      `unheaded`, missing 013/014).
- [ ] 0.5 **[Y]** Apply 013/014 in a transaction (COMPLIANCE-WELL step 21),
      then `compliance-evidence well sync -dry-run`, then real.

### Phase 1: backups that cover everything, proven by restore (W-01)

- [x] 1.1 `scripts/db-backup.sh`: globals + every database (enumerate from
      `pg_database`, not a list), `-Fc`, sha256 per file, fail loudly on any
      error (`set -euo pipefail`, no `|| true`). *Done 2026-10-14:* sets at
      `~/backups/unheaded/<date>/well-<time>Z/` (cluster.env, globals.sql,
      `<db>.dump`, rowcounts.tsv taken in each dump's exported snapshot,
      SHA256SUMS), written as `.partial-*` and renamed when complete, `--keep N`
      retention, refuses a directory inside the repo. Probe
      `db-backup-recent` counts complete sets. Not yet run on the live Well
      (Phase 0).
- [ ] 1.2 Off-host copy to east over the P2P link (rsync as a dedicated
      user, 0600 files), retention per the retention policy.
- [x] 1.3 Restore drill script: restore the newest set into a throwaway
      container (the bootstrap pattern used for this audit), compare row
      counts per table with the source; time it (RTO). *Done 2026-10-14:*
      `scripts/well/restore-drill.sh [SET]`; bootstraps with the set's
      superuser (globals grant `GRANTED BY` it). `scripts/well/test-backup-restore.sh`
      runs backup then drill end to end on throwaway servers (5 DBs, 24 tables,
      restore 15 s) and makes the drill refuse a flipped byte, a wrong row
      count and an unlisted dump, each for its own reason.
- [ ] 1.4 systemd timer (daily dump, weekly drill); `unheaded_well_backup_*`
      metrics (age, size, drill result) and an alert; UH-BACKUP-01 reads them.
- [ ] 1.5 Decide PITR (WAL archiving) vs daily dumps: write the RPO down
      first (Law 3). Record in ADR-016 amendment.

### Phase 2: access (W-02, W-05, W-06, W-07, W-24)

- [~] 2.1 Roles without passwords in SQL: 002/012 create roles `LOGIN`
      without `PASSWORD`; `init.sh` sets each from `WELL_<ROLE>_PASSWORD`
      (unset = cannot log in, never a default). Compose `POSTGRES_PASSWORD`
      becomes `${POSTGRES_PASSWORD:?}` (do this when Phase 0 is done, so
      `compose up postgres` is not blocked meanwhile; that was FND-008's
      reason to wait). *Roles done 2026-10-14* with the Kubernetes secret's
      names (`APP_KANBAN_PASSWORD`, ..., `HUGINN_READER_PASSWORD`), psql
      `\getenv`, compose passes them through; checked (and provoked red) by
      `scripts/well/test-backup-restore.sh`. Only fresh volumes are affected:
      the live Well keeps its `*_dev` passwords until rotated in Phase 2.
      `POSTGRES_PASSWORD:?` still waits for Phase 0.
- [ ] 2.2 One owner role per database (`well_app_owner`, `well_ops_owner`,
      `well_config_owner`, NOLOGIN); `ALTER ... OWNER TO`; default privileges.
- [ ] 2.3 History tables INSERT/SELECT only for writers, plus the 013/014
      trigger pattern (refuse UPDATE/DELETE/TRUNCATE) on `audit_events`,
      `service_health_transitions`, `kingdom_config_history`. Extend
      `scripts/well/test-compliance-schema.sh` (or a sibling) with refusal
      checks, provoked red.
- [ ] 2.4 kanban-app to `app_kanban` on `unheaded_app` (FND-008); move its
      data out of DB `unheaded` (see 3.6).
- [ ] 2.5 Exact RFC 1918 check in `pkg/database/validate.go` (W-24).

### Phase 3: one trunk (W-03, W-09, W-10, W-16, W-20, W-22)

- [ ] 3.1 ADR: migration runner, stdlib-only Go (`cmd/well-migrate`), one
      `well_schema_migrations` ledger per DB, files named per DB, checksums
      recorded; `lock_timeout` and `statement_timeout` set per migration;
      no psql meta-commands (W-16).
- [ ] 3.2 Baseline: the post-Phase-0 live schema becomes version 1 per DB
      (golden `schema.sql` generated, checked in, diffed in CI).
- [ ] 3.3 Delete startup DDL: `pkg/database/kanban_store.go`,
      `pkg/champion/pgstore/schema.sql`, `raft/zhen_app.py`,
      `raft/scripts/zhen_rag.py`, `cmd/zhen-rag`, `cmd/zhen-cli`,
      `pkg/agent`, `pkg/audit/storage`; services check the ledger version
      and fail fast.
- [ ] 3.4 Reconcile the multiply-defined tables into one definition each
      (expand/contract where live data exists; batched backfills).
- [ ] 3.5 CI gates: fresh bootstrap in a throwaway container (as this audit
      did) and `well_lint.py` as a ratchet (exit 2 fails; findings set may
      only shrink); register both in the meta-gate.
- [ ] 3.6 Retire DB `unheaded` and 001 once 3.4 has moved its data.

### Phase 4: the deployed config is the repo's (W-08, W-13, W-17)

- [ ] 4.1 Mount a `postgresql.conf` sized from the container budget (256M
      limit: `shared_buffers` ~64MB, or raise the limit with a reason), with
      `pg_stat_statements`, `log_min_duration_statement`, `log_lock_waits`,
      `track_io_timing`; no global `statement_timeout`.
- [ ] 4.2 Mount `pg_hba.conf`: `hostssl` only, the `data` network's subnet
      only, superuser local-only; TLS with a certificate from the edge TLS
      decision (UH-TLS-01).
- [ ] 4.3 Data checksums: `pg_checksums --enable` offline during a
      maintenance window (or at the next initdb).
- [ ] 4.4 Pin the image to a current 16.x by digest; minor upgrade with a
      restore drill first.
- [ ] 4.5 Connection budget written down (services x pool <= max_connections
      - reserved); PgBouncer only if the arithmetic says so.

### Phase 5: schema hygiene (W-04, W-11, W-12, W-14, W-15, W-18, W-19, W-21)

- [ ] 5.1 `pkg/audit/storage`: delete if unreached (it is), or port to the
      ledger with Postgres DDL and parameters.
- [ ] 5.2 Review the 9 `fmt.Sprintf` SQL sites; parameterize, or annotate
      constant-only verbs.
- [ ] 5.3 Drop the duplicate and low-value indexes `CONCURRENTLY`, each
      backed by `pg_stat_user_indexes` from Phase 0.
- [ ] 5.4 Retention and partitioning for append-only tables per the
      retention policy (time partitions dropped, never rows deleted).
- [ ] 5.5 Health bridge: wire it to Akira's topic and shape with a contract
      test, or delete it and the `service_health_*` tables.
- [ ] 5.6 Config history trigger: INSERT/UPDATE/DELETE, `change_reason`
      required.
- [ ] 5.7 Index `compliance_host_bindings(profile)`; types: identity
      columns, `timestamptz NOT NULL`, `jsonb`, no nullable `DEFAULT ''`.
