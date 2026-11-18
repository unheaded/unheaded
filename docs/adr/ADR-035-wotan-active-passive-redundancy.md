# ADR-035: Wotan Active-Passive Redundancy

## Status: ACCEPTED (reinstated 2026-11-03 by decision pack DP-214, which superseded ADR-064: active/passive with Akira failover is the two-host design; a third node reopens ADR-064). PostgreSQL persistence withdrawn 2026-11-09 (amendment below). For the east and west pair each host runs its own bus and nothing is replicated: ADR-150 choice 6, option A, Stevie Bellis 2026-11-18 (amendment below).

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

## Amendment 2026-11-18: on the two-host pair, a bus per host

Stevie Bellis, 2026-11-18 (`docs/battle-plans/DECISION-PACK-2026-11-18.md`
round 6, row 21): ADR-150 choice 6 is answered "(A) a bus per host", "with
an ADR-035 amendment". This is that amendment. It is taken from ADR-150's
section "Wotan" (`ADR-150-east-and-west-as-a-pair.md`) and adds no design.

ADR-150's words for the choice, quoted (first the table row for reading A,
its three cells in order: reading, what it is, cost):

> A. A bus per host
>
> Each host runs its own Wotan for its own services, always. Nothing is
> replicated. When either host or the link is lost, each bus keeps working
>
> The bus is per host, not once per environment. What must cross hosts
> goes by other roads: metrics to VictoriaMetrics, logs to the log store,
> heartbeats between the mailers. A critical alert on west does not reach
> east's services, and Akira on one host does not see the other's
> announcements, since a process holds one Wotan client
> (`pkg/wotan-client/policy_test.go:204`)

> **Recommended: A now.** It is the plain reading of decision 17 and what
> east's units already dial. In-memory state has nothing to replicate, and
> losing a host or the link costs each side nothing it would not lose in a
> restart. This departs from ADR-035's accepted text, so it is Stevie's to
> settle (To settle 6). B's moving address can be added later for a
> consumer that needs one environment-wide bus. C and ADR-064 return with a
> third node.

> Late joiners under A or B rebuild from their publishers, with no log:
> discovery within 15 s, health within 30 s, critical alerts at the next
> 5-minute snapshot.

| | This ADR as accepted | For the east and west pair, from 2026-11-18 |
|---|---|---|
| Instances | West primary, east standby | One Wotan on each host, each serving its own host's services |
| Replication | WAL entries shipped to the standby | None |
| Failover | Akira-driven promotion of the standby | None: there is nothing to promote. When a host or the link is lost each bus keeps working |
| What a lost host costs | The replication lag window | What that host's bus buffered, the same as a restart; publishers refill it |
| Across hosts | One bus for the environment | Metrics, logs and heartbeats go by their own roads. A critical alert published on west does not reach east's services, and Akira on one host does not see the other's announcements |

**What still stands of this ADR.**

- The 2026-11-09 amendment: memory is the default store, the local WAL
  store is an explicit opt-in, PostgreSQL persistence is withdrawn.
- The design above as the record of active-passive replication (ADR-150's
  reading C). ADR-150: "C and ADR-064 return with a third node". The status
  line's "a third node reopens ADR-064" is unchanged.
- `services/wotan/internal/cluster/` stays in the tree. This amendment
  removes and wires nothing.
- The open item of 2026-11-09 stays open: an epoch that does not live in
  PostgreSQL, "to be decided when cluster mode is wired".

**The state of the code, read 2026-11-18.** The pair's option A needs none
of this; it is what reading C would have had to build first.

- Cluster mode refuses to start. With `--cluster-mode cluster` the binary
  validates the cluster configuration and then exits with "cluster mode is
  not yet implemented in this binary; refusing to start. Drop --cluster-*
  flags to run standalone." (`services/wotan/cmd/wotan/main.go:193-201`;
  the message is line 201).
- Snapshot transfer is not implemented. `TransferSnapshot` answers
  `codes.Unimplemented`, "snapshot transfer not implemented; catch up from
  seq 0 (ADR-064)" (`services/wotan/internal/cluster/replication_server.go:95-104`).

Not decided here: ADR-150's reading B (one bus at an address that moves),
and anything about three or more nodes (ADR-064, ADR-142).
