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

On first start, Docker runs the init scripts from `db/migrations/` (mounted to
`/docker-entrypoint-initdb.d/`):

```
001_initial_schema.sql   -- bootstrap (if any legacy setup)
002_create_databases.sql -- CREATE DATABASE x3, CREATE USER x7
003_app_schema.sql       -- kanban_tasks, timeline_milestones, zhen_conversations + indexes + grants
004_ops_schema.sql       -- service_health_*, audit_events + indexes + grants
005_config_schema.sql    -- kingdom_config, kingdom_config_history, service_registry + triggers + grants
006_seed_data.sql        -- initial config values and service registry entries
007_grants.sql           -- cross-database CONNECT grants
```

Orchestrated by `db/init.sh`, which runs each SQL file against the appropriate database.

---

## Connection Strings

**From Docker network (services in containers):**

```
postgresql://app_kanban:kanban_dev@postgres:5432/unheaded_app
postgresql://app_timeguru:timeguru_dev@postgres:5432/unheaded_app
postgresql://app_zhen:zhen_dev@postgres:5432/unheaded_app
postgresql://ops_writer:ops_writer_dev@postgres:5432/unheaded_ops
postgresql://ops_reader:ops_reader_dev@postgres:5432/unheaded_ops
postgresql://config_admin:config_admin_dev@postgres:5432/unheaded_config
postgresql://config_reader:config_reader_dev@postgres:5432/unheaded_config
```

**From host (local development):**

Replace `postgres` with `localhost` in the above strings.

**Go package:** `pkg/database/` provides `Connect()` with retry logic.
Configure via environment variables: `DB_HOST`, `DB_PORT`, `DB_USER`,
`DB_PASSWORD`, `DB_NAME`.

---

## Authentication

The Well uses `scram-sha-256` for all network connections. See `db/pg_hba.conf`:

- Superuser `postgres`: local peer auth only (no remote access)
- Legacy `unheaded` user: scram-sha-256 from Docker/LXD networks (172.0.0.0/8, 10.0.0.0/8)
- Service-scoped users: scram-sha-256, restricted to their database
- Everything else: **reject**

---

## Monitoring

```bash
# Check container health
sudo docker inspect --format='{{.State.Health.Status}}' unheaded-postgres

# Connection count
sudo docker exec unheaded-postgres psql -U unheaded -d unheaded_ops \
  -c "SELECT count(*) FROM pg_stat_activity;"

# Database sizes
sudo docker exec unheaded-postgres psql -U unheaded \
  -c "SELECT datname, pg_size_pretty(pg_database_size(datname)) FROM pg_database WHERE datname LIKE 'unheaded%';"

# Table row counts (app)
sudo docker exec unheaded-postgres psql -U unheaded -d unheaded_app \
  -c "SELECT relname, n_live_tup FROM pg_stat_user_tables ORDER BY n_live_tup DESC;"

# Slow queries
sudo docker exec unheaded-postgres psql -U unheaded -d unheaded_ops \
  -c "SELECT query, calls, mean_exec_time FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10;"
```

---

## Backup

```bash
# Full backup (all databases)
sudo docker exec unheaded-postgres pg_dumpall -U postgres > backup_$(date +%Y%m%d).sql

# Single database backup
sudo docker exec unheaded-postgres pg_dump -U postgres unheaded_app > backup_app_$(date +%Y%m%d).sql

# Restore
cat backup_20260315.sql | sudo docker exec -i unheaded-postgres psql -U postgres
```

**Volume location:** Docker volume `postgres-data` (persistent across container restarts).

---

## Resource Limits

From `docker-compose.yml`:
- **Memory:** 256MB
- **CPU:** 0.5 cores
- **Health check:** `pg_isready -U unheaded`, every 10s, 5 retries

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
