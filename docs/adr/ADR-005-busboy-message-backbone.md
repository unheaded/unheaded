# ADR-005: Busboy as Message Backbone (vs. NATS, RabbitMQ, Kafka)

## Status: Accepted

## Date: 2026-01-26

## Context

Unheaded's microservice architecture requires a message bus for inter-service communication. The architecture mandates that all services communicate through the message bus rather than direct service-to-service calls (the "Sacred Law" in `docs/FAE_CHAMBER_CONTRACTS.md`). This decoupling is essential for:

1. **Fault isolation**: A crashed service does not cascade to its dependents.
2. **Observability**: All inter-service communication flows through a single, monitorable hub.
3. **Scalability**: Adding new services requires only subscribing to existing topics, not modifying existing services.
4. **Auditability**: Every message is logged, timestamped, and correlated by trace ID.

The message bus must support:

- Pub/sub with topic-based routing (e.g., `tasks.created`, `timeline.updates`, `state.drift.detected`)
- gRPC streaming for high-throughput trace data from the eBPF trace-collector
- Sub-5ms publish-to-subscribe latency within the container network
- Wildcard subscriptions (e.g., `tasks.*`)
- Message envelopes with correlation IDs, trace IDs, and sequence numbers

### Alternatives Evaluated

**NATS** -- Lightweight, fast, Go-native. Strong pub/sub model with JetStream for persistence. However, NATS is an external dependency (violates ADR-004), and JetStream's persistence model is more complex than needed for the alpha.

**RabbitMQ** -- Mature, feature-rich, supports AMQP. However, RabbitMQ is written in Erlang, is operationally heavy (requires Erlang runtime), and is overkill for an architecture that needs simple topic-based pub/sub without complex routing, exchange bindings, or dead-letter queues at the infrastructure level.

**Apache Kafka** -- Industry standard for high-throughput event streaming. However, Kafka requires ZooKeeper (or KRaft), has significant operational overhead, is designed for persistent log storage at massive scale (millions of messages/sec), and is fundamentally a distributed commit log -- not a lightweight message bus. Its resource footprint is inappropriate for an LXD container running on a single host.

**Redis Pub/Sub** -- Simple and fast, but messages are fire-and-forget with no persistence, no replay, and no built-in gRPC support.

**Custom (Busboy)** -- A purpose-built Go message bus that was already proven in Phase 0/1 of the project. Busboy was built as the first Unheaded component (13,500+ LOC), demonstrating the team's capability to build production-grade infrastructure.

## Decision

We use **Busboy** -- our custom-built Go + gRPC message bus -- as the sole message backbone for all inter-service communication.

Busboy provides:

- **HTTP REST API** (port 8080) for simple publish/subscribe from any HTTP client
- **gRPC streaming** (port 9090) for high-throughput, low-latency trace data
- **Topic-based pub/sub** with hierarchical topic naming (`<domain>.<event_type>[.<subtype>]`)
- **Wildcard subscriptions** (e.g., `tasks.*` matches `tasks.created`, `tasks.completed`)
- **Ring buffer** for bounded message storage (prevents unbounded memory growth)
- **Message envelope** with `message_id`, `sender_id`, `timestamp`, `seq`, `correlation_id`, and `trace_id`
- **Go client library** (`pkg/busboy-client/`) with automatic reconnection and health checks

Busboy runs as a dedicated LXD container at `10.10.10.10` (Docker: `172.28.1.1`), functioning as "The Fae Chamber" -- the coordination layer of the Kingdom.

All services connect to Busboy on startup, subscribe to their relevant topics (defined in `docs/FAE_CHAMBER_CONTRACTS.md`), and publish state changes as events. The dashboard-backend subscribes to all topics for real-time UI updates.

## Consequences

### Positive

- **Zero external dependencies**: Busboy is internal code, fully aligned with ADR-004. No upstream supply chain risk, no version churn, no license concerns.
- **Purpose-built**: Busboy is designed for exactly the Unheaded use case -- moderate-throughput pub/sub within a container network. It has no features we do not need and all features we do.
- **Proven**: Busboy was the first component built (Phase 0) and has been running reliably through Phase 1 and the alpha. It has 13,500+ lines of battle-tested code.
- **Operationally simple**: A single Go binary with no external dependencies (no ZooKeeper, no Erlang, no JVM). Starts in under a second, restarts cleanly, and fits in a minimal LXD container.
- **Full control**: We can optimize Busboy's ring buffer size, topic routing, and gRPC streaming behavior for our exact workload without waiting for upstream features.
- **Deep observability**: Because Busboy is our code, we can instrument it at any depth -- every message publish/subscribe is a potential eBPF trace point.
- **Dual interface**: HTTP REST for simple integrations (curl, scripts, testing) and gRPC streaming for high-throughput data paths (trace-collector). This flexibility is difficult to get from off-the-shelf solutions without running two systems.

### Negative

- **Not battle-tested at scale**: Busboy has one user (Unheaded). NATS, RabbitMQ, and Kafka have thousands of production deployments across diverse workloads. Edge cases that those systems have already discovered and fixed may surprise us.
- **No built-in clustering**: Busboy currently runs as a single instance. Multi-node clustering and geographic distribution are Phase 2 roadmap items. If the single Busboy instance fails, all inter-service communication halts until restart.
- **No persistent message replay**: Unlike Kafka's commit log or NATS JetStream, Busboy's ring buffer is bounded and in-memory. Messages that overflow the buffer are lost. WAL-based persistence is a Phase 2 feature (to be implemented in Anamnesis).
- **Maintenance burden**: Every bug fix, performance optimization, and protocol enhancement must be developed internally. There is no community contributing improvements.
- **Migration cost**: If Busboy proves insufficient at production scale, migrating to NATS or Kafka would require rewriting all service integrations. The `pkg/busboy-client/` abstraction layer would help, but topic semantics and message envelopes would need adaptation.

## References

- `services/busboy/` -- Busboy source (external: `github.com/unheaded/busboy`, 13,500+ LOC)
- `pkg/busboy-client/` -- Go client library (1,853 LOC)
- `docs/FAE_CHAMBER_CONTRACTS.md` -- Topic naming, message formats, routing matrix
- `docs/ARCHITECTURE.md` -- Message Bus Topics table, Layer 3 specification
- `docs/MICROSERVICES.md` -- Communication Pattern section
