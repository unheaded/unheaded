# ADR-035: Wotan Active-Passive Redundancy

## Status: ACCEPTED (reinstated 2026-11-03 by decision pack DP-214, which superseded ADR-064: active/passive with Akira failover is the two-host design; a third node reopens ADR-064). PostgreSQL persistence withdrawn 2026-11-09 (amendment below).

Previously: SUPERSEDED by ADR-064 (2026-05-05) — "active passive works but doesn't scale".

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

## Amendment 2026-11-09: PostgreSQL persistence dropped

Stevie, 2026-11-09. Wotan is a fast in-memory store (ADR-005), and the
PostgreSQL persistence above is withdrawn:

- `pg_store.go` and `hybrid_store.go` are deleted; `postgres` and `hybrid`
  are unknown store types, and `--store-conn-str` is gone. The PG-backed
  sequence counter and cluster epoch (`PersistentSequenceCounter`,
  `EpochManager`) went with them; neither had a caller.
- Memory is the default store. The local WAL store (`--store-type wal`) is
  the only persistence, an explicit opt-in: a wal store that cannot open
  stops startup rather than falling back to memory.
- The WAL is also the replication substrate: `services/wotan/internal/cluster/`
  ships WAL entries to the standby and never used PostgreSQL. It is kept.
- "PostgreSQL dependency for persistence" under Negative no longer applies;
  split-brain prevention needs an epoch that does not live in PG, to be
  decided when cluster mode is wired.
