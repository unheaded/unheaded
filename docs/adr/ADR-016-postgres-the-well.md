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

- **Image:** `postgres:16-alpine` (minimal footprint)
- **Container:** `unheaded-postgres` in Docker Compose
- **Port:** 5432 (standard, not in the Doom Range — PostgreSQL is infrastructure, not a Kingdom service)
- **Volume:** `postgres-data` for persistent storage
- **Init:** Migrations in `db/migrations/` mounted to `/docker-entrypoint-initdb.d`
- **Networks:** data plane (service access)

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
