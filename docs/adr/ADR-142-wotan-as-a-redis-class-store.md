<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-142: Wotan made fast and lean enough to stand beside Redis

**Status:** Proposed (parked). A pitch; no plan, no code.
**Date:** 2026-11-12
**Related:** ADR-005 (Wotan is an in-memory store), ADR-035 and its 2026-11-09 amendment (no PostgreSQL or hybrid store; WAL opt-in), ADR-140 (notifications ride Wotan), ADR-103 (service tokens), ADR-043 and ADR-105 (signed topics), FND-043 (per-identity topic permissions), ADR-112 (plan freeze), `docs/protocol/` wotan-memory-03, `wiki/Service-Wotan.md`.

## The pitch (Stevie, 2026-11-12, verbatim)

> Wotan may still be improved and developed on to increase its speed and efficiency too so it is a more comparable alternative to redis

Said while deciding that notifications ride Wotan (ADR-140): if the bus is going to carry more of the kingdom's live state, it should hold up next to the thing people would otherwise reach for.

## As understood

Not "become Redis", and not Redis's protocol or its command set. Wotan stays what it is, the kingdom's own bus and memory, and gets measured and tightened until an adopter who would have added a Redis for pub/sub, a small cache or a work queue has no reason to: one fewer third-party service on a small host. That is the footprint argument (Stevie, same day: Unheaded is meant to be what other applications run on) and the native-first rule (2026-10-29) pointing the same way.

## What is there

- **A message bus with a bounded memory.** Topics with wildcard subscriptions, a gRPC stream and an HTTP API, a ring of the newest 10,000 messages per topic by default (`services/wotan/internal/store`), in memory; a write-ahead log on local disk is opt-in. No PostgreSQL store since 2026-11-09.
- **Identity on every call.** EdDSA service tokens, an allowed-caller list, `topics.auto_approve`, and since 2026-11-12 per-identity read and publish lists with `unlisted: deny`. ML-DSA-65 signatures are enforced on the signed prefixes (`config.`, `drift.`, `audit.`, `alerts.critical.`, the health consensus topics).
- **TLS 1.3 with hybrid ML-KEM** on both listeners.
- **A second role on paper:** wotan-memory-03 describes Wotan as protocol RAM, local (BPF maps) and distributed. The service implements the bus; cluster mode exists in code and is not wired (WAL replication verifies signatures and halts on a rejected entry).
- **No numbers.** There are micro-benchmarks for topic matching and subscribe, and nothing for end-to-end publish rate, fan-out, latency under load or memory per message. The kingdom's own benchmark item (M-089) is unmeasured.

## What "Redis-class" would have to mean, and what it must not

Wanted, in the order an adopter would feel them:

1. **Measured speed.** Publish rate, fan-out to N subscribers, p50/p99 delivery latency, memory per retained message, reconnect storm behaviour. On west's hardware and on something small.
2. **Bounded, predictable memory.** A ceiling per topic and overall that holds under a flood, with a counted drop policy, not growth until the unit's `MemoryMax` kills it.
3. **A short list of data shapes beyond a topic ring**, only if a first-party user needs them: a key with a value and a time to live (the notification read marks and unread counts of ADR-140 are the first candidate), an atomic counter, a work queue with acknowledgement. Each one justified by a caller in this repository, not by Redis having it.
4. **Persistence that is honest.** The WAL either gives a stated recovery point or is called a debug aid. No silent middle.

Not wanted:

- The Redis wire protocol or a compatibility layer: it would invite third-party clients onto an internal bus (ADR-140: browsers and outside clients stay off Wotan).
- Scripting, modules, or anything that runs caller-supplied code in the bus.
- Speed bought from the edge. Stevie's rule stands: the engine is fast and efficient inside; cryptography at the edge is slow and robust, and is never traded for throughput. Token checks, topic permissions and signatures stay on every call, and the benchmark counts them.

## To settle in this ADR before any plan

1. **Measure first.** A benchmark harness and its first numbers come before any change, and decide whether there is a problem at all. Nothing is "faster" without a before.
2. **The comparison.** Against what, on what hardware, with what workload: Redis pub/sub and streams with TLS and ACLs on, since Wotan never runs without its checks. Running a Redis to compare against is bringing in a third-party tool for a lab measurement: Stevie's yes, per item.
3. **Which data shapes, if any.** Each named with its first caller.
4. **Where the time goes.** A profile (`--pprof`, `wiki/Profiling.md`) of the publish path: token verification per message or per stream, JSON on the HTTP path, locks around the ring, fan-out copies.
5. **Cluster mode.** In scope or not. A second node is what "stand beside Redis" means to most people, and it is the unwired half of the service.
6. **The targets.** Numbers that would count as done, written before the work.
7. **Encryption of each link**, in ADR-140's table form, for anything this adds.

## Decision

None. Parked under ADR-112.
