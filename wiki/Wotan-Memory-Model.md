# Wotan Memory Model

Wotan is the memory layer of the [Unheaded Protocol](Unheaded-Protocol): **LOCAL** BPF maps on each Shim plus **DISTRIBUTED** gRPC topic streaming between hosts. Spec: five-level hierarchy L0 Monad, L1 BPF hash maps, L2 per-flow ring buffers, L3 persistent WAL, L4 Sophia dictionaries ([draft-bellis-unheaded-wotan-memory-03](../docs/protocol/draft-bellis-unheaded-wotan-memory-03.md)).

Wotan serves a **triple role** in the Unheaded architecture:

1. **High-Speed Ring Buffer** — Lock-free eBPF perf_event pattern for zero-copy packet event delivery
2. **Event Bus** — Pub/sub messaging via gRPC/HTTP with topic-based routing
3. **Protocol RAM** — BPF map memory substrate where Monad computations read/write state

## Cluster Redundancy

Wotan uses **subscription mirroring** (not Raft/Paxos consensus):

- **DNS-like hierarchy:** Root Wotans → Regional Wotans → Edge Wotans
- **BGP-style advertisement:** Each Wotan advertises its topic catalog to peers
- **n+1 availability:** Subscribers mirror from primary; failover is automatic
- **Scale spectrum:** 1 Wotan (dev) → 3 (HA) → 1000 (global)

---

> **Spec:** [draft-bellis-unheaded-wotan-memory-01](../docs/protocol/draft-bellis-unheaded-wotan-memory-01.md)
