# ADR-016: PostgreSQL — The Well (Kingdom Persistent Memory)

**Status:** Accepted
**Date:** 2026-03-15
**Deciders:** Architect, Developer, Scientist

## Context

The Unheaded Kingdom currently uses:
- **SQLite** (`modernc.org/sqlite`) for local cache in timeguru and kanban services
- **ClickHouse** for high-volume structured log ingestion and querying
- **VictoriaMetrics** for time-series metrics (Prometheus-compatible)

There is no relational database for structured application data that requires ACID guarantees, foreign key relationships, and complex queries. As the Kingdom grows beyond alpha, services need durable, queryable persistence for kanban tasks, timeline milestones, audit trails, service health records, and configuration state.

## Decision

Add **PostgreSQL 16** as the Kingdom's persistent relational storage layer, named **"The Well"** following the Anamnesis pattern from ADR-001 (Gnostic State Management). The Well is the memory of the Kingdom — where structured state is persisted and recalled.

### Naming Rationale (Gnostic Mythology)

Per ADR-001, the state management taxonomy is:
- **Pleroma** = desired state (configuration, Git)
- **Kenoma** = actual state (runtime metrics, VictoriaMetrics)
- **Anamnesis** = memory/history (event sourcing, audit trails)
- **Yaldabaoth** = chaos/testing

PostgreSQL fits the **Anamnesis** pattern. "The Well" evokes deep memory — the place where the Kingdom's structured knowledge is stored and retrieved. It complements the existing Chronicler's Well (log aggregation ring buffer in `pkg/logagg/`) by providing durable, queryable persistence.

### Tables

| Table | Purpose | Pattern |
|-------|---------|---------|
| `kanban_tasks` | Task board persistence | Anamnesis |
| `timeline_milestones` | Sprint/milestone tracking | Anamnesis |
| `audit_events` | Security and operational audit trail | Anamnesis |
| `service_health` | Service health snapshots | Kenoma |
| `service_logs` | Durable log storage (supplements ClickHouse) | Anamnesis |
| `zhen_conversations` | AI conversation history | Anamnesis |
| `kingdom_config` | Runtime configuration (JSONB) | Pleroma |

### Infrastructure

- **Image:** `postgres:16.15-alpine` (minimal footprint; exact tag per ADR-102)
- **Container:** `unheaded-postgres` in Docker Compose
- **Port:** 5432 (standard, not in the Doom Range — PostgreSQL is infrastructure, not a Kingdom service); published on `127.0.0.1` only, TLS 1.3 required (`db/pg_hba.conf`)
- **Volume:** `postgres-data` (on west: `unheaded-dev_postgres-data`, disk sdb); data checksums on
- **Init:** `db/init.sh` applies `db/migrations/MANIFEST` on an empty volume (ADR-091); an existing Well is migrated with `cmd/well-migrate` (ADR-101)
- **Networks:** data plane (service access)

Current databases, roles, tables and locations: `docs/architecture/THE_WELL_OPERATIONS.md`
and `wiki/The-Well.md`; backups: `wiki/Well-Backups.md`. The table list
below is the original 2026-03-15 design.

### Go Package

`pkg/database/` provides:
- `Config` and `DefaultConfig()` for environment-driven configuration
- `Connect()` with retry logic (5 attempts, backoff)
- `KanbanStore` as a reference implementation for PostgreSQL-backed stores
- Uses `github.com/lib/pq` (pure Go, no CGO dependency)

## Consequences

### Positive
- ACID transactions for structured data (kanban, milestones, config)
- JSONB columns for semi-structured data (audit details, health metadata)
- Proper indexing for query performance
- Migration-based schema evolution (`db/migrations/`)
- Enables future features: full-text search, materialized views, pg_cron
- Pure Go driver (`lib/pq`) — no CGO, cross-compilation friendly

### Negative
- Adds operational complexity (one more service to manage)
- Requires Docker for local development (already a dependency)
- Memory overhead (~128MB baseline for PostgreSQL)
- Schema migrations need coordination across deployments

### Neutral
- SQLite remains for local-only caching where appropriate
- ClickHouse remains the primary log analytics engine
- VictoriaMetrics remains the metrics store
- PostgreSQL is additive — no existing functionality is replaced

## Amendment 2026-10-15: recovery point (ERDA 1.5) — **Accepted**

The RPO is written down before choosing a mechanism (Law 3).

**What a restore would lose today**, by table: kanban cards (also in each
kanban-app's SQLite L1 and the timeline in git), compliance tables
(re-derived by `compliance-evidence well sync` from collected evidence),
health state (re-derived by Akira within a sweep), zhen conversations
(useful, not critical), audit_events (empty until pkg/audit is wired).
Nothing in The Well today is unrecoverable within a day by other means.

| | A. Nightly dumps (now) | B. WAL archiving (PITR) |
|---|---|---|
| RPO | <= 24 h (one night) | minutes (`archive_timeout`, e.g. 5 min) |
| RTO | ~15 s restore at today's size, drill-proven weekly | base backup + WAL replay; needs its own drill |
| Moving parts | db-backup.sh, 2 timers, offsite copy (all live, alerting) | + archive_command to a mounted dir, offsite WAL copy, weekly pg_basebackup, a PITR drill, WAL retention, an archiver-lag alert |
| Config | as deployed | `archive_mode = on` needs a restart; `wal_level = replica` is already set |

**Decision (Stevie, 2026-10-15): A, RPO 24 h.** West is not up 24/7:
the backup runs nightly when it is, and a missed night runs at the next
boot (`Persistent=`). Sets are kept on a different physical disk from the
data (`/srv/backups/unheaded-well` on the NVMe; the data volume is on the
/var disk, sdb), and `WELL_BACKUP_OTHER_DISK=1` makes the backup refuse
if that ever stops being true. Retention is two weeks: the newest 14 sets
on west and on east (by count, so a powered-off week does not age them
out). Revisit B when The Well first holds records
nothing else can rebuild within a day: audit_events written in production
by pkg/audit, or kanban-app running without its SQLite L1. The switch is
one restart away (wal_level is already `replica`), so deferring costs
little. The `conf-no-archive` lint finding stays in the baseline until then.
