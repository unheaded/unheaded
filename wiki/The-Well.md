# The Well

The Well is Unheaded's PostgreSQL-based persistence layer, providing multi-database architecture for service state, health aggregation, and operational data.

## Architecture

```
┌──────────────────────────────────────┐
│            The Well                  │
├──────────┬──────────┬────────────────┤
│  Service │  Health  │  Operational   │
│  State   │  Agg.   │  Data          │
├──────────┴──────────┴────────────────┤
│         PostgreSQL Multi-DB          │
│         7 Database Users             │
└──────────────────────────────────────┘
```

## Database Layout

The Well uses a multi-database PostgreSQL deployment with strict separation of concerns:

- **Service state databases** — per-service persistent state (Kanban, Dashboard, Timeguru, etc.)
- **Health aggregation database** — consolidated health checks, uptime metrics, SLA tracking
- **Operational database** — configuration, deployment state, audit logs

## Users & Access Control

7 database users with role-based access:

- Each service gets a dedicated user with least-privilege access
- Health aggregation uses a read-only user across service databases
- Administrative access is restricted to deployment and migration operations

## Health Aggregation

The Well aggregates health data from all 34 active services:

- **Heartbeat collection** via Wotan event bus
- **SLA computation** with sliding window averages
- **Alerting integration** with observability backends
- **Historical trending** for capacity planning

## Design Principles

- **No DB = read-only mode** — mutable UI is never allowed without persistence
- **Write operations require DB connection** — enforced at the service layer
- **Migration-first** — all schema changes go through versioned migrations
- **Backup-aware** — WAL archiving and point-in-time recovery configured

---

*Last updated: March 17, 2026*
