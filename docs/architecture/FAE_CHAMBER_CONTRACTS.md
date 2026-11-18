# Fae Chamber Message Contracts
## The Sacred Protocols of the Message Bus

**Version:** 1.0.0
**Last Updated:** January 29, 2026
**Domain:** The Fae Chamber (Wotan Message Bus)

---

## Overview

The Fae Chamber is the message bus that enables all services in the Unheaded Kingdom to communicate. This document defines the topic naming conventions, message formats, and routing contracts.

**The Sacred Law:** Every service MUST communicate through the Fae Chamber. Direct service-to-service calls are forbidden except for health checks.

---

## Topic Naming Convention

### Format
```
<domain>.<event_type>[.<subtype>]
```

### Domains

| Domain | Owner | Description |
|--------|-------|-------------|
| `tasks` | Kanban | Task lifecycle events |
| `timeline` | Timeguru | Timeline and milestone updates |
| `alerts` | All | Cross-service alert routing |
| `metrics` | Vambraces | Observability data |
| `state` | Cuirass | Control plane state changes |

`decisions.*` and `architecture.*` were the captain and architect services' domains; those services and micromanager were removed 2026-11 (ADR-147).

---

## Topic Catalog

### Task Events (Kanban)
```
tasks.created      # New task created
tasks.updated      # Task modified
tasks.completed    # Task marked complete (no publisher since 2026-11, ADR-147)
tasks.deleted      # Task removed
tasks.assigned     # Task assigned to owner
tasks.blocked      # Task blocked by dependency
tasks.*            # Wildcard subscription for all task events
```

### Timeline Events (Timeguru)
```
timeline.updates          # Timeline.md changed
timeline.milestone.hit    # Milestone achieved
timeline.milestone.missed # Milestone deadline passed
timeline.phase.started    # New phase begun
timeline.phase.completed  # Phase finished
timeline.sync             # Manual sync request
```

### Alert Events (Cross-Service)
```
alerts.critical    # P0 - Immediate attention required
alerts.warning     # P1 - Needs attention soon
alerts.info        # P2 - Informational
alerts.resolved    # Alert resolved
alerts.escalated   # Alert escalated
```

### State Events (Cuirass)
```
state.container.created  # Container spawned
state.container.deleted  # Container removed
state.container.healthy  # Health check passed
state.container.unhealthy # Health check failed
state.drift.detected     # Desired != Actual state
state.drift.resolved     # State reconciled
state.reconcile.started  # Reconciliation began
state.reconcile.complete # Reconciliation finished
```

### Metrics Events (Vambraces)
```
metrics.collected   # Metrics batch collected
metrics.threshold   # Threshold exceeded
metrics.anomaly     # Anomaly detected
```

---

## Message Format

### Standard Message Envelope
```json
{
  "message_id": "uuid-v4",
  "topic": "domain.event_type",
  "sender_id": "service-name",
  "timestamp": "2026-01-29T12:00:00Z",
  "seq": 12345,
  "payload": { ... },
  "metadata": {
    "correlation_id": "uuid-v4",
    "trace_id": "ebpf-trace-id",
    "version": "1.0.0"
  }
}
```

### Task Payload
```json
{
  "task_id": "task-123",
  "title": "Implement feature X",
  "status": "in-progress",
  "priority": "high",
  "owner": "muck",
  "assignee": "developer-agent",
  "epic_id": "epic-2.1",
  "created_at": "2026-01-29T12:00:00Z",
  "updated_at": "2026-01-29T12:30:00Z"
}
```

### Timeline Payload
```json
{
  "event": "milestone.hit",
  "milestone_id": "alpha-1.0",
  "phase": "Age 1",
  "progress_before": 24,
  "progress_after": 25,
  "source": "timeguru"
}
```

### Alert Payload
```json
{
  "alert_id": "alert-789",
  "severity": "critical",
  "source": "vambraces",
  "title": "Container health check failed",
  "description": "wotan-1 unhealthy for 3 checks",
  "affected_service": "wotan",
  "metadata": {
    "container_id": "wotan-1",
    "last_healthy": "2026-01-29T11:55:00Z"
  }
}
```

---

## Routing Matrix

### Who Publishes What

| Service | Primary Topics |
|---------|----------------|
| Timeguru | `timeline.*` |
| Kanban | `tasks.created`, `tasks.updated`, `tasks.deleted` |
| Cuirass | `state.*` |
| Vambraces | `metrics.*`, `alerts.*` |
| All Services | `alerts.critical` (on critical failure) |

### Who Subscribes to What

| Service | Subscribed Topics | Purpose |
|---------|-------------------|---------|
| **Timeguru** | `timeline.*`, `tasks.completed` | Progress tracking |
| **Kanban** | `tasks.*`, `timeline.updates` | UI updates |
| **Cuirass** | `state.*`, `alerts.*` | Control plane orchestration |
| **Vambraces** | `metrics.*`, `alerts.*` | Observability aggregation |

---

## Cross-Service Event Flows

### Alert Escalation
```
Vambraces detects anomaly
   ↓
Vambraces publishes: alerts.warning
   ↓
Cuirass receives → prepares remediation
   ↓
If unresolved after timeout:
   ↓
Vambraces publishes: alerts.critical
```

---

## Client Configuration

### Service Bootstrap Pattern
```go
// Standard service initialization
func initWotan(addr string, serviceName string, topics []string) (*wotanClient.Client, error) {
    client, err := wotanClient.NewClient(addr)
    if err != nil {
        return nil, fmt.Errorf("create client: %w", err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    for _, topic := range topics {
        sub, err := client.Subscribe(ctx, topic, serviceName)
        if err != nil {
            log.Printf("WARNING: failed to subscribe to %s: %v", topic, err)
            continue
        }
        log.Printf("Subscribed to %s (status: %s)", topic, sub.Status)
    }

    return client, nil
}
```

### Required Subscriptions by Service

**Timeguru:**
```go
[]string{"timeline.*", "tasks.completed"}
```

---

## Error Handling

### Retry Policy
- **Transient failures:** Exponential backoff (1s, 2s, 4s, 8s, 16s max)
- **Permanent failures:** Dead letter to `dlq.<original_topic>`
- **Timeout:** 5 seconds for publish, 30 seconds for subscribe

### Circuit Breaker States
1. **Closed:** Normal operation
2. **Open:** Failures exceed threshold, fail fast
3. **Half-Open:** Testing recovery

---

## Metrics

Each service MUST export these Wotan metrics:
```
wotan_messages_published_total{service, topic}
wotan_messages_received_total{service, topic}
wotan_publish_duration_seconds{service, topic}
wotan_subscription_status{service, topic, status}
```

---

## The Sacred Laws

1. **All state changes MUST publish events** - No silent mutations
2. **All subscriptions MUST have handlers** - No ignored messages
3. **All failures MUST escalate** - `alerts.critical` is always listened to
4. **All services MUST be graceful** - Handle reconnection, shutdown cleanly
5. **The Fae Chamber is the only truth** - If it's not published, it didn't happen

---

**THE FAE CHAMBER DANCES. THE MESSAGES FLOW. THE KINGDOM COMMUNICATES.**

🧚✨
