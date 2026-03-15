# ADR-012: The Well — PostgreSQL Persistent Storage

## Status: Accepted

## Context

The Kingdom uses SQLite for local service state, ClickHouse for log analytics,
and VictoriaMetrics for metrics. There is no shared relational store for
structured application data (tasks, timeline, conversations, configuration).

Each service maintains its own SQLite — correct for isolation, but problematic
for data that must be shared across services or survive container restarts.

## Decision

Add PostgreSQL 16 as "The Well" — the Kingdom's persistent memory layer.

### Naming

**The Well** (Hebrew: Be'er, באר) — the gathering place where knowledge is drawn.
In the Gnostic model, The Well extends Anamnesis (memory) with durable, queryable storage.
Anamnesis provides event streams; The Well provides structured state.

### Data Classification

| Data Type | Current Storage | The Well Table | Migration Priority |
|-----------|----------------|----------------|-------------------|
| Kanban tasks | SQLite (kanban-app) | kanban_tasks | P0 — demo-critical |
| Timeline milestones | SQLite (timeguru) | timeline_milestones | P1 |
| Audit events | ClickHouse | audit_events (copy) | P2 |
| Service health | In-memory | service_health | P1 |
| Zhen conversations | None | zhen_conversations | P0 |
| Kingdom config | YAML files | kingdom_config | P2 |
| Service logs | ClickHouse | service_logs (recent) | P3 |

### Integration Pattern

Services should NOT connect directly to PostgreSQL. Instead:

```
Service → Wotan topic → Dashboard-backend → PostgreSQL
                            ↑
                   (single writer pattern)
```

Only dashboard-backend and kanban-app get direct DB connections.
Other services publish state changes to Wotan topics.
Dashboard-backend subscribes and persists.

This preserves:
- Service isolation (no DB dependency in core services)
- Wotan as the canonical communication channel
- Single-writer to avoid conflicts

### Migration Strategy

Phase 1 (immediate): Add Postgres to docker-compose, create schema, wire kanban-app
Phase 2 (week): Migrate timeguru, add Zhen conversation logging
Phase 3 (month): Add service health aggregation, kingdom config

## Consequences

- **Positive**: Persistent state survives restarts, queryable history, proper relational modeling
- **Positive**: Zhen conversations are logged and searchable
- **Negative**: Adds operational complexity (backups, migrations, monitoring)
- **Negative**: Another container to manage
- **Mitigated**: PostgreSQL is mature, well-understood, and already in most devs' toolboxes
