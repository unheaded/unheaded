# The Well

The Well is Unheaded's PostgreSQL: one Postgres 16 cluster with three databases, one per kind of state. It runs as the `unheaded-postgres` container on west. Backups, restore and offsite copies are on [[Well Backups|Well-Backups]].

## Where things live (west)

| What | Where |
|---|---|
| Server | container `unheaded-postgres`, image `postgres:16.15-alpine` (pinned, ADR-102) |
| Data | Docker volume `unheaded-dev_postgres-data`, `/var/lib/docker/volumes/...` on disk **sdb** (`/var`); data checksums on |
| Config | `db/postgresql.conf`, `db/pg_hba.conf` (repo, mounted read-only) |
| TLS | private CA + server cert in `/etc/unheaded/well-tls` (outside the repo), made by `scripts/well/make-tls.sh`; cert valid to 2029-01-17 |
| Passwords | repo-root `.env` (0600, gitignored); never in the repo. Names in `db/.env.example` |
| Migrations | `db/migrations/` routed by `db/migrations/MANIFEST`; applied by `cmd/well-migrate` (ADR-101) |
| Backups | `/srv/backups/unheaded-well` on the NVMe, plus east. See [[Well-Backups]] |
| Backup/drill scripts | `/usr/local/lib/unheaded/well/` (root-owned copies of `scripts/`) |
| Metrics | Huginn (`pg_stat_database`); backup/drill/offsite in `/var/lib/unheaded/textfile` |
| Alerts | `monitoring/prometheus/rules/well.yml` |

## Databases

| Database | Holds | Owner (NOLOGIN) |
|---|---|---|
| `unheaded_app` | kanban tasks, timeline milestones, Zhen conversations/memories/actions | `well_app_owner` |
| `unheaded_ops` | service health, audit events, compliance evidence and findings | `well_ops_owner` |
| `unheaded_config` | kingdom_config (+ history), service registry, compliance catalogue | `well_config_owner` |
| `postgres` | `pg_stat_statements` only | `unheaded` |

Each database has its own `well_schema_migrations` ledger. The legacy `unheaded` database was dropped on 2026-10-15; its final dump is archived in `~/backups/well/2026-10-15/`.

## Roles

| Role | Reaches | Used by |
|---|---|---|
| `app_kanban`, `app_timeguru`, `app_zhen` | `unheaded_app` (+ read `unheaded_config`) | kanban-app, timeguru, Zhen |
| `ops_writer`, `ops_reader` | `unheaded_ops` | dashboard-backend, read views |
| `config_admin`, `config_reader` | `unheaded_config` | unheaded-daemon, services |
| `huginn_reader` | all three, `pg_monitor`, 3 conns | Huginn |
| `well_backup` | all, `pg_read_all_data`, 3 conns | backups, restore drill |
| `unheaded` | superuser, TLS from the host only | well-migrate, operators |

PUBLIC has no CONNECT or TEMP anywhere: a role reaches only the databases it is granted.

## Connecting

- **Key exchange is classical (P-256)** until E1: PostgreSQL 16's `ssl_ecdh_curve` takes one curve. PostgreSQL 18 with `ssl_groups=X25519MLKEM768:X25519:prime256v1` (`db/postgresql.pg18.conf.d/10-pq-groups.conf`) negotiates the hybrid with every client we run; proof suite `scripts/well/test-pq-groups.sh`, upgrade runbook `runbooks/data/well-pg18-upgrade.yaml` (dump and restore into a new volume, the PG 16 volume kept as the rollback). FND-015; see [[Post-Quantum Key Exchange|Post-Quantum-Key-Exchange]].
- **TLS 1.3 only** for every network connection; anything else is rejected (`db/pg_hba.conf`). Clients use `sslmode=require` (Go clients: `WELL_SSLMODE`, default `require`; `pkg/database` refuses `allow` and `prefer`, and `disable` except to a loopback host or socket); `verify-full` with `sslrootcert=/etc/unheaded/well-tls/ca.crt`.
- **On Kubernetes** (`kubernetes/manifests/base/the-well/`) the same: the StatefulSet runs `db/postgresql.conf` with `db/pg_hba.k8s.conf` (TLS + SCRAM for pods from private address space to the four Well databases by name, the superuser only inside the pod) and the certificate from the `the-well-tls` secret, made by `make-tls.sh` (`WELL_TLS_EXTRA_DNS=the-well,the-well.unheaded.svc` for `verify-full`). Proved on kind 2026-10-19.
- From containers: host `postgres:5432` on the `data` network (`172.28.1.0/24`).
- From west itself: `127.0.0.1:5432` (published to localhost only).
- `docker exec` over the socket needs no password (root on the host already).
- Budget: `max_connections` 50, 42 of 47 allotted. A new pooled client gets a row in the operations guide first.

## Rules

- **No DB = read-only mode.** Mutable UI is never allowed without persistence.
- **Ledger only.** Schema changes are new migrations through `cmd/well-migrate`; no service creates tables at startup, no hand-run SQL files.
- **No credentials in the repo**, and every `kingdom_config` change states a reason (`well.change_reason`, 028).
- **Backup before change:** `sudo systemctl start unheaded-well-backup.service`.

---

> **Source:** [THE_WELL_OPERATIONS.md](../docs/architecture/THE_WELL_OPERATIONS.md) · [ADR-016](../docs/adr/ADR-016-postgres-the-well.md) · [ADR-101](../docs/adr/ADR-101-well-migration-ledger.md) · [ERDA audit](../docs/battle-plans/WELL-ERDA-AUDIT-2026-10-13.md)
