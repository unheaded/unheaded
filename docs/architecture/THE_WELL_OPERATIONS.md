# The Well (PostgreSQL) -- Operational Guide

*SPDX-License-Identifier: GPL-3.0-or-later*

**Date:** 2026-03-15
**Status:** Operational
**ADR:** ADR-016-postgres-the-well.md

---

## Quick Start

```bash
# Start PostgreSQL (requires sudo for Docker)
sudo docker compose up -d postgres

# Verify it's running
sudo docker compose ps postgres
sudo docker exec unheaded-postgres pg_isready -U unheaded

# Connect via psql
sudo docker exec -it unheaded-postgres psql -U unheaded -d unheaded_app
```

---

## Architecture

### 3 Databases

| Database | Purpose | Pattern (ADR-001) |
|----------|---------|-------------------|
| `unheaded_app` | Application data: kanban tasks, timeline milestones, Zhen conversations | Anamnesis (memory) |
| `unheaded_ops` | Operations: service health, state transitions, hourly aggregates, audit events | Kenoma (actual state) |
| `unheaded_config` | Configuration: kingdom key-value config, config history, service registry | Pleroma (desired state) |

### 7 Users (Principle of Least Privilege)

| User | Database | Permissions | Used By |
|------|----------|-------------|---------|
| `app_kanban` | unheaded_app | CRUD on kanban_tasks; SELECT on others | kanban-app service |
| `app_timeguru` | unheaded_app | CRUD on timeline_milestones; SELECT on others | timeguru service |
| `app_zhen` | unheaded_app | CRUD on zhen_conversations; SELECT on others | Zhen AI champion |
| `ops_writer` | unheaded_ops | CRUD on all ops tables | dashboard-backend, health checkers |
| `ops_reader` | unheaded_ops | SELECT only | dashboard read-only views |
| `config_admin` | unheaded_config | CRUD on all config tables | unheaded-daemon, admin tools |
| `config_reader` | unheaded_config | SELECT only | all services (read config) |

**Cross-database access:**
- All `app_*` users can CONNECT to `unheaded_config` and SELECT from `kingdom_config`, `service_registry`
- `ops_reader` can CONNECT to `unheaded_app` and SELECT from `kanban_tasks`, `timeline_milestones`, `zhen_conversations`

### Tables Summary

**unheaded_app:**
- `kanban_tasks` -- task board with status, priority, tags, assignee
- `timeline_milestones` -- sprint tracking with progress percentage
- `zhen_conversations` -- AI chat history with full-text search (tsvector)

**unheaded_ops:**
- `service_health_current` -- latest health state per service+host (upsert target)
- `service_health_transitions` -- append-only state change log
- `service_health_hourly` -- aggregated hourly health stats
- `audit_events` -- tamper-evident audit trail with hash chain

**unheaded_config:**
- `kingdom_config` -- key-value store with typed JSONB values and change tracking
- `kingdom_config_history` -- automatic change log (trigger-driven)
- `service_registry` -- service metadata, ports, dependencies, health endpoints

---

## Initialization Flow

`db/init.sh` is the only file in `/docker-entrypoint-initdb.d/`; the
migrations are mounted at `/migrations`. On an empty volume it applies every
file `db/migrations/MANIFEST` routes, in order, to its database, and records
each in that database's `well_schema_migrations` ledger (mode `initdb`).
An existing Well is migrated with `cmd/well-migrate` (ADR-101), never by
piping files into psql. `POSTGRES_DB` is `postgres`: there is no database
outside the manifest (the legacy `unheaded` database was retired
2026-10-15). `001_initial_schema.sql` is `skip`ped (ADR-091).

## Connection Strings

**From the `data` network (services in containers):**

```
postgresql://app_kanban:<password>@postgres:5432/unheaded_app?sslmode=require
postgresql://app_timeguru:<password>@postgres:5432/unheaded_app?sslmode=require
postgresql://app_zhen:<password>@postgres:5432/unheaded_app?sslmode=require
postgresql://ops_writer:<password>@postgres:5432/unheaded_ops?sslmode=require
postgresql://ops_reader:<password>@postgres:5432/unheaded_ops?sslmode=require
postgresql://config_admin:<password>@postgres:5432/unheaded_config?sslmode=require
postgresql://config_reader:<password>@postgres:5432/unheaded_config?sslmode=require
```

**From the host:** `localhost` instead of `postgres` (compose publishes
`127.0.0.1:5432` only). For `sslmode=verify-full`, add
`sslrootcert=/etc/unheaded/well-tls/ca.crt`; the certificate names
`postgres`, `unheaded-postgres`, `localhost` and `127.0.0.1`.

Each `<password>` is that role's variable in `.env` (`APP_KANBAN_PASSWORD`,
..., `HUGINN_READER_PASSWORD`; see `db/.env.example`): db/init.sh sets them
on an empty volume, `scripts/well/set-role-passwords.sh` on an existing
Well (also the superuser's, from `POSTGRES_PASSWORD`). No password is in the
repo. Go clients (`pkg/database`, `pkg/compliance/well`, `cmd/well-migrate`)
default to `WELL_SSLMODE=require`.

---

## Authentication

`db/pg_hba.conf` (ERDA 4.2), TLS 1.3 and scram-sha-256 on every network
connection:

- Socket (`docker exec` only, already root on the host): trust. Backups,
  drills and set-role-passwords.sh use it.
- Superuser `unheaded`: TLS from `172.28.1.1` (the host) only.
- Every other role: TLS from the `data` network (`172.28.1.0/24`).
- Anything without TLS, or from elsewhere: **reject**.

Which database a role reaches is decided by CONNECT grants (025), not
pg_hba: PUBLIC has none. Each database is owned by a NOLOGIN role
(`well_app_owner`, `well_ops_owner`, `well_config_owner`); migrations run as
it (ADR-101 §7). TLS material: `scripts/well/make-tls.sh
/etc/unheaded/well-tls` (private CA; server certificate valid 825 days,
renew with `--renew`).

---

## Connection Budget (ERDA 4.5)

`max_connections = 50`, 3 reserved for the superuser: 47 for everyone else.

| Client | Role | Pool |
|---|---|---|
| kanban-app (`pkg/database`) | app_kanban | 10 |
| dashboard-backend (`pkg/database`) | ops_writer | 10 |
| zhen-agentd (kanban + action stores) | `WELL_DSN` | 2 x 5 |
| Huginn (one per database) | huginn_reader | 3 (role limit 3) |
| db-backup.sh / restore drill | well_backup | 3 (role limit 3) |
| well-migrate | superuser | 2 |
| compliance-evidence, zhen python, psql | various | ~4 |
| **Total** | | **42 of 47** |

A new pooled client means a new row here first; PgBouncer only when the
sum no longer fits.

---

## Monitoring

```bash
# Container health
sudo docker inspect --format='{{.State.Health.Status}}' unheaded-postgres

# Everything at once, read-only (ERDA skill: well_health.sql), per database
sudo docker exec -i -e PGOPTIONS='-c role=huginn_reader' unheaded-postgres \
  psql -X -U unheaded -d unheaded_app < well_health.sql

# Who is connected, and over TLS?
sudo docker exec unheaded-postgres psql -U unheaded -d postgres \
  -c "SELECT a.usename, a.datname, a.client_addr, s.ssl, s.version FROM pg_stat_activity a JOIN pg_stat_ssl s USING (pid) WHERE a.usename IS NOT NULL;"

# Slowest statements (pg_stat_statements lives in `postgres`, covers every database)
sudo docker exec unheaded-postgres psql -U unheaded -d postgres \
  -c "SELECT calls, round(mean_exec_time::numeric, 1) AS ms, left(query, 80) FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10;"
```

Huginn exports `pg_stat_database` metrics for the three databases; slow
statements (> 500 ms), lock waits and DDL are in the container log.

---

## Backup

`unheaded-well-backup.timer` (nightly 03:30) runs `scripts/db-backup.sh` as
the read-only role `well_backup`: globals (no password hashes) and every
database, row counts from each dump's snapshot, SHA256SUMS, 14 sets kept in
`/var/backups/unheaded-well` (`LATEST` names the newest).
`unheaded-well-drill.timer` (Sundays) restores the newest set into a
throwaway server and compares every row count.

```bash
sudo systemctl start unheaded-well-backup.service     # a set now
sudo /usr/local/lib/unheaded/well/restore-drill.sh "/var/backups/unheaded-well/$(cat /var/backups/unheaded-well/LATEST)"
```

After restoring a set for real, set role passwords again
(`scripts/well/set-role-passwords.sh`): sets carry none.

**Volume:** Docker volume `unheaded-dev_postgres-data`. Data checksums are on.

---

## Resource Limits

From `docker-compose.yml`:
- **Memory:** 256MB (`shared_buffers` 64MB, `work_mem` 2MB, see `db/postgresql.conf`)
- **CPU:** 0.5 cores
- **Health check:** `pg_isready -U unheaded -d postgres`, every 10s, 5 retries

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Connection refused on 5432 | Container not running | `sudo docker compose up -d postgres` |
| "password authentication failed" | Wrong user/password combo | Check `db/migrations/002_create_databases.sql` for passwords |
| "permission denied for table X" | User lacks grants | Re-run `007_grants.sql` against the target database |
| Init scripts not running | Database already initialized | Remove volume: `sudo docker volume rm unheaded_postgres-data` and restart |
| Disk full | Volume grown too large | Check with `docker system df`, prune if needed |

---

*See also: `docs/adr/ADR-016-postgres-the-well.md` (design rationale), `db/pg_hba.conf` (access control)*
