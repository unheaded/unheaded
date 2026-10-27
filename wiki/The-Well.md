# The Well

The Well is Unheaded's PostgreSQL: one Postgres 16 cluster with three databases, one per kind of state. It runs as the `unheaded-postgres` container on west. Backups, restore and offsite copies are on [[Well Backups|Well-Backups]].

## Where things live (west)

| What | Where |
|---|---|
| Server | container `unheaded-postgres`, image `postgres:16.15-alpine` (pinned, ADR-102) |
| Data | Docker volume `unheaded-dev_postgres-data`, `/var/lib/docker/volumes/...` on disk **sdb** (`/var`); data checksums on |
| Config | `db/postgresql.conf`, `db/pg_hba.conf` (repo, mounted read-only) |
| TLS | private CA + server cert in `/etc/unheaded/well-tls` (outside the repo), made by `scripts/well/make-tls.sh`; cert valid to 2029-01-17 |
| Passwords | repo-root `.env` (0600, gitignored); never in the repo. Names in `db/.env.example`. A second copy: west's `/etc/huginn.env` (`PGPASSWORD`, huginn_reader). Rotate with `scripts/well/set-role-passwords.sh` (value passed by env name, never argv), update both files, restart `huginn.service`; last rotation 2026-10-27 (all nine roles). The host has no `psql`: check a login with a `postgres:16.15-alpine` client container on `--network host` |
| Migrations | `db/migrations/` routed by `db/migrations/MANIFEST`; applied by `cmd/well-migrate` (ADR-101) |
| Backups | `/srv/backups/unheaded-well` on the NVMe, plus east. See [[Well-Backups]] |
| Backup/drill scripts | `/usr/local/lib/unheaded/well/` (root-owned copies of `scripts/`) |
| Metrics | Huginn (`pg_stat_database`); backup/drill/offsite in `/var/lib/unheaded/textfile` |
| Alerts | `monitoring/prometheus/rules/well.yml` |

## Databases

| Database | Holds | Owner (NOLOGIN) |
|---|---|---|
| `unheaded_app` | kanban tasks, timeline milestones, Zhen conversations/memories/actions, API keys (`api_keys`: SHA-256 of each `uhk_` key, owner, role, expiry, revocation; migration 036) | `well_app_owner` |
| `unheaded_ops` | service health, audit events, compliance evidence and findings, LLM request telemetry (`llm_requests`), retention records (`retention_runs`, `retention_archive`; 043) | `well_ops_owner` |
| `unheaded_config` | kingdom_config (+ history), service registry, compliance catalogue | `well_config_owner` |
| `postgres` | `pg_stat_statements` only | `unheaded` |

`unheaded_ops.llm_requests` (030) holds one append-only row per model request from either Zhen brain: counts, cost and timing, never text; written by `ops_writer` when `zhen-claude`/`zhen-agentd` run with `-audit-well`; rows are never deleted; the retention job measures when it needs time partitions (see Retention below). See [[Zhen Audit|Zhen-Audit]].

Each database has its own `well_schema_migrations` ledger. The legacy `unheaded` database was dropped on 2026-10-15; its final dump is archived in `~/backups/well/2026-10-15/`.

## Roles

| Role | Reaches | Used by |
|---|---|---|
| `app_kanban`, `app_timeguru`, `app_zhen` | `unheaded_app` (+ read `unheaded_config`) | kanban-app, timeguru, Zhen |
| `app_auth` | `unheaded_app` `api_keys` only: SELECT, INSERT, UPDATE of `revoked_at`/`last_used_at` (no DELETE), 40 conns; password `APP_AUTH_PASSWORD` (migration 035) | `unheaded apikey`, services with `AUTH_APIKEY_STORE=well` (login `AUTH_WELL_USER`/`AUTH_WELL_PASSWORD`) |
| `ops_writer`, `ops_reader` | `unheaded_ops` | dashboard-backend, zhen-claude / zhen-agentd audit sinks (`llm_requests`, `audit_events`), read views |
| `config_admin`, `config_reader` | `unheaded_config` | unheaded-daemon, services |
| `huginn_reader` | all three, `pg_monitor`, 3 conns | Huginn |
| `well_backup` | all, `pg_read_all_data`, 3 conns | backups, restore drill |
| `retention_runner` | `unheaded_app` + `unheaded_ops`, 4 conns: DELETE on `zhen_conversations`, `kanban_tasks`, `retention_archive` only; `zhen_captures_purge()`; age columns of the append-only tables; INSERT on `retention_runs`; password `RETENTION_RUNNER_PASSWORD` (migrations 042/043) | `well-retention` (`unheaded-well-retention.timer`) |
| `unheaded` | superuser, TLS from the host only | well-migrate, operators |

PUBLIC has no CONNECT or TEMP anywhere: a role reaches only the databases it is granted.

## Retention

One job, `well-retention` (`cmd/well-retention`, package `unheaded-well-retention`, unit + weekly timer Sunday 04:10 as account `unheaded-well-retention`), logs in as `retention_runner` and applies `compliance/retention/classes.yaml` (installed as the conffile `/etc/unheaded/well/retention-classes.yaml`). The ages are a proposal until Stevie ratifies them (E-C): policy table in `docs/policy/retention-policy-v1-2026-05-06.md` §11. Until then the file says `ratified: false`, a live run exits 2, and the timer is not enabled.

| Class | Table | Age | What happens |
|---|---|---|---|
| zhen-conversations | `zhen_conversations` | 90 d | purged in batches; no archive (operator text) |
| zhen-captures | `zhen_captures` | 180 d (033's trigger) | `zhen_captures_purge()`; promoted ones stay, quarantined go at once |
| kanban-soft-deleted | `kanban_tasks`, `deleted_at` set | 90 d | redacted copy into `retention_archive`, then deleted; a card any `zhen_agent_runs` row names is kept (held) |
| retention-archive | `retention_archive` | 365 d | purged |
| llm-requests, service-health-transitions, audit-events, compliance-findings-history, deploy-history, chaos-journal, zhen-agent-runs | append-only history | 90-365 d | **never deleted**: partition check only (ERDA 5.4: due at 1 M rows, 1 GB, or 90 days before the oldest row reaches its age); the partition migration is a reviewed change |

- **Audit rows are never deleted** by this job; `retention_runner` cannot (permission denied, proved by `scripts/well/test-retention.sh`).
- **Every run is recorded**: one `unheaded_ops.retention_runs` row per class (dry runs too): cutoff, eligible, deleted, archived, held, partition verdict in `detail`. Read: `SELECT class, action, dry_run, eligible, deleted, held, detail FROM retention_runs ORDER BY id DESC LIMIT 20` (as `ops_reader`).
- **Dry run**: `sudo systemd-run --wait --pipe --uid=unheaded-well-retention -p LoadCredential=well-password:/etc/unheaded/well/retention.password /opt/unheaded/bin/well-retention -classes /etc/unheaded/well/retention-classes.yaml -dry-run`. Counts only.
- **Password**: root's 0400 `/etc/unheaded/well/retention.password`, handed to the unit as the credential `well-password`; same value as `RETENTION_RUNNER_PASSWORD` in `.env`.
- **PII**: archived rows pass the shared redactor (`pkg/zhencapture/redact_patterns.json`, emails included) first; the Zhen web UI redacts conversation rows the same way before they are written. The job logs counts, never contents.
- **Legal hold**: `hold: true` on a class stops its deletes (recorded as `held`).
- **Metrics**: `/var/lib/unheaded/textfile/well_retention.prom` (`unheaded_well_retention_last_success_timestamp_seconds`, `_rows_deleted`, `_partition_due`, `_class_failed`).
- First live run (counted read-only 2026-10-27): 126 of 226 conversations are past 90 days; `service_health_transitions` is due for partitions under 90 d.

## Connecting

- **Key exchange is classical (P-256)** until E1: PostgreSQL 16's `ssl_ecdh_curve` takes one curve. PostgreSQL 18 with `ssl_groups=X25519MLKEM768:X25519:prime256v1` (`db/postgresql.pg18.conf.d/10-pq-groups.conf`) negotiates the hybrid with every client we run; proof suite `scripts/well/test-pq-groups.sh`, upgrade runbook `runbooks/data/well-pg18-upgrade.yaml` (dump and restore into a new volume, the PG 16 volume kept as the rollback). FND-015; see [[Post-Quantum Key Exchange|Post-Quantum-Key-Exchange]].
- **TLS 1.3 only** for every network connection; anything else is rejected (`db/pg_hba.conf`). Clients use `sslmode=require` (Go clients: `WELL_SSLMODE`, default `require`; `pkg/database` refuses `allow` and `prefer`, and `disable` except to a loopback host or socket); `verify-full` with `sslrootcert=/etc/unheaded/well-tls/ca.crt`.
- **On Kubernetes** (`kubernetes/manifests/base/the-well/`) the same: the StatefulSet runs `db/postgresql.conf` with `db/pg_hba.k8s.conf` (TLS + SCRAM for pods from private address space to the four Well databases by name, the superuser only inside the pod) and the certificate from the `the-well-tls` secret, made by `make-tls.sh` (`WELL_TLS_EXTRA_DNS=the-well,the-well.unheaded.svc` for `verify-full`). Proved on kind 2026-10-19.
- From containers: host `postgres:5432` on the `data` network (`172.28.1.0/24`).
- From west itself: `127.0.0.1:5432` (published to localhost only).
- `docker exec` over the socket needs no password (root on the host already).
- Budget: `max_connections` 50, 44 of 47 allotted. A new pooled client gets a row in the operations guide first.

## Rules

- **No DB = read-only mode.** Mutable UI is never allowed without persistence.
- **Ledger only.** Schema changes are new migrations through `cmd/well-migrate`; no service creates tables at startup, no hand-run SQL files. Services check the ledger instead: `pkg/database.RequireMigrations` (kanban-app requires 003 and 007 and refuses to start against a Well that answers without them, rather than falling back to its local SQLite board; `app_kanban` reads `unheaded_app`'s ledger through migration 038). kanban-app's and timeguru's `CREATE TABLE IF NOT EXISTS` are their own local SQLite files, not the Well.
- **Batches go through `scripts/well/batch-apply.sh`.** `--dry-run [--from NNN]` lists what is pending (MANIFEST minus ledgers) and changes nothing; without it the script backs up the whole cluster (`scripts/db-backup.sh` layout) into `/srv/backups/unheaded-well/pre-batch-<UTC>/` with `pending.tsv`, applies, and prints the ledger rows added. A batch that fails part-way: do not re-run; `scripts/well/batch-rollback.sh --yes <pre-batch-dir>` drops and restores only the databases the batch touched and drops roles it created (writes since the backup are lost). Rehearsal: `scripts/well/test-batch-apply.sh` (CI). Pending live as of 2026-10-27: 030-043.
- **Unused indexes** (ERDA 5.3): `scripts/well/index-drops-after-2026-11-15.sh` prints `DROP INDEX CONCURRENTLY` drafts for indexes still at `idx_scan = 0`; it refuses before 2026-11-15 or on statistics younger than 28 days and never drops anything. The reviewed list becomes a `notx` migration.
- **No credentials in the repo**, and every `kingdom_config` change states a reason (`well.change_reason`, 028).
- **Backup before change:** `sudo systemctl start unheaded-well-backup.service`.

---

> **Source:** [THE_WELL_OPERATIONS.md](../docs/architecture/THE_WELL_OPERATIONS.md) · [ADR-016](../docs/adr/ADR-016-postgres-the-well.md) · [ADR-101](../docs/adr/ADR-101-well-migration-ledger.md) · [ERDA audit](../docs/battle-plans/WELL-ERDA-AUDIT-2026-10-13.md)
