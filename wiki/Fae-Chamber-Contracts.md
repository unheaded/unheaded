# Fae Chamber Contracts

Topic and message contracts for Wotan (the Fae Chamber): which service publishes what, what subscribers can rely on. Wotan itself: [[Service-Wotan]].

## Topic naming

`<domain>.<event_type>[.<subtype>]`; subscribers may use a trailing wildcard (`tasks.*`).

| Domain | Owner | Topics used in code |
|--------|-------|----------------------------------|
| `tasks` | Micromanager / kanban | `tasks.created`, `.updated`, `.completed`, `.deleted`, `.created.bulk` |
| `timeline` | Timeguru | `timeline.updates`, `.updated`, `.sync`, `.task.{created,updated,removed}`, `.phase.{started,completed}`, `.milestone.{hit,missed}` |
| `decisions` | Captain | `decisions.created` |
| `architecture` | Architect | `architecture.updates` |
| `alerts` | everyone | `alerts.critical` (every service subscribes) |
| `state` | unheaded-daemon | `state.changed` |
| `system` | Akira, discovery | `system.health.reports`, `system.health.consensus` (ADR-029), `system.discovery` (TTL registration) |
| `logs` | every service | `logs.<service>.<level>` (`pkg/logagg`) |
| `traces` | trace collectors | `traces.packet`, `.flow`, `.latency`, `.correlated`, `.stats`, `.collected` |
| `anamnesis` | Shield / hop eBPF | `anamnesis.birth`, `.hop`, `.death`, `.chaos`, `.anomaly`, `.unknown` |
| `network` | failover | `network.failover`, `network.recovery`, `network.policy.events` |
| `compliance` | compliance eBPF | `compliance.violations` |
| `config` | control plane | `config.*` must carry an ML-DSA-65 signature ([[Wotan-Topic-Signing]]) |

The source doc's fuller catalog (e.g. `tasks.blocked`, `state.drift.*`, `metrics.anomaly`) is design intent; the table above is what code publishes or subscribes to today.

## Message shape

What Wotan stores and returns per topic message (`services/wotan/internal/api/topics.go`):

```json
{ "message_id": "…", "topic": "tasks.created", "sender_id": "kanban-app",
  "created_at": "2026-10-16T12:00:00Z", "seq": 12345, "payload": "<string>", "deleted": false }
```

`seq` is global across topics and monotonic (in-memory store, the default; the Postgres store numbers per topic); consumers page with `seq` / `after_seq` and resume from their cursor after a Wotan restart. Include `trace_id` and a Unix-ms timestamp in the payload; keep payloads under 1 MB.

## Rules

1. Every state change publishes an event: no silent mutations.
2. Every subscription has a handler.
3. Failures escalate: `alerts.critical` is always listened to.
4. Services reconnect and shut down cleanly.
5. Publish retries back off exponentially; Wotan counts retries, dead letters, drops and timeouts (`wotan_publish_retries_total`, `wotan_dead_letters_total`, …).

---

> **Source:** [docs/architecture/FAE_CHAMBER_CONTRACTS.md](../docs/architecture/FAE_CHAMBER_CONTRACTS.md) (2026-02) · topic strings in `pkg/`, `services/`, `cmd/`
