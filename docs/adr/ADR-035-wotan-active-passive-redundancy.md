# ADR-035: Wotan Active-Passive Redundancy

## Status: SUPERSEDED by ADR-064 (2026-05-05) — Phases 0-2 code reused; active-passive design retired in favour of 3-node active/active K8s cluster (rationale: "active passive works but doesn't scale")

## Date: 2026-04-05
## Decision Makers: Stevie Bellis (Principal), Claude (Advisor)
## Battle Plan: `docs/battle-plans/WAVE9-TWIN-RAVENS.md`

---

## Context

Wotan is the Kingdom's message bus — every service publishes and subscribes through it.
Currently single-instance, in-memory, no persistence. If Wotan dies, messages are lost
and inter-service communication halts until Akira auto-restarts it.

With WEST + EAST bare metal online, we need Wotan to survive a node failure.

## Decision

**Active-passive replication with PostgreSQL persistence + WAL write-ahead buffering.**

- WEST is always the configured primary (divine right)
- EAST is standby, receives replicated WAL entries via gRPC + mTLS
- PostgreSQL (The Well) provides durable message persistence on both nodes
- WAL provides <1ms write latency (async flush to PG in batches)
- Failover is Akira-driven (66.67% consensus threshold)
- Split-brain prevented by monotonic epoch counter in PG

### Architecture

```
Publish → WAL (fast, fsync) → async flush to PostgreSQL → replicate WAL to standby
Failover: Akira detects primary down → promotes standby → clients failover via circuit breaker
```

### What Already Existed (leveraged, not rebuilt)

- `pkg/storage/wal/wal.go` (480 LOC) — Full WAL with segments, CRC32, fsync
- `services/wotan/internal/store/` — MessageStore interface + factory
- `services/wotan/proto/topic.proto` — node_id + since_seq fields ready
- `pkg/transport/mtls/` — mTLS with cert rotation
- `pkg/wotan-client/` — gRPC-first with circuit breaker + fallback queue
- `pkg/health/akira.go` — 66.67% consensus + auto-restart

### What's New (Wave 9)

- `services/wotan/internal/store/pg_store.go` — PostgreSQL-backed MessageStore
- `services/wotan/internal/store/encoding.go` — Message serialization
- `services/wotan/proto/replication.proto` — WAL replication gRPC service
- `services/wotan/internal/cluster/` — Config, replication server/client, failover
- Persistent sequence counters (PG-backed, survive restarts)
- Cluster epoch for split-brain prevention

## Consequences

### Positive
- Messages survive node failure (PG persistence)
- Automatic failover via Akira (<15s total)
- Zero overhead in standalone mode (cluster code doesn't load)
- Clients already have circuit breaker for transparent failover

### Negative
- PostgreSQL dependency for persistence (was optional, now required for cluster mode)
- Replication lag window (async — messages in WAL but not yet on standby could be lost)
- Two-node cluster can't do true consensus (Akira is the tiebreaker)

## References

- Battle plan: `docs/battle-plans/WAVE9-TWIN-RAVENS.md`
- ADR-034: gRPC mTLS Default Transport
- ADR-029: Wotan Consensus Health (Akira)
