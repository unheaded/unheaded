# Fae Chamber Message Contracts
## The Sacred Protocols of the Message Bus

**Version:** 1.0.0
**Last Updated:** January 29, 2026
**Domain:** The Fae Chamber (Busboy Message Bus)

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
| `tasks` | Micromanager | Task lifecycle events |
| `timeline` | Timeguru | Timeline and milestone updates |
| `decisions` | Captain | Strategic decisions |
| `architecture` | Architect | Design and ADR events |
| `alerts` | All | Cross-service alert routing |
| `metrics` | Vambraces | Observability data |
| `state` | Cuirass | Control plane state changes |

---

## Topic Catalog

### Task Events (Micromanager)
```
tasks.created      # New task created
tasks.updated      # Task modified
tasks.completed    # Task marked complete
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

### Decision Events (Captain)
```
decisions.created   # New decision logged
decisions.approved  # Decision approved
decisions.rejected  # Decision rejected
decisions.archived  # Decision archived
decisions.escalated # Decision escalated to Muck
```

### Architecture Events (Architect)
```
architecture.updates     # General architecture change
architecture.adr.created # New ADR logged
architecture.adr.updated # ADR modified
architecture.service.added    # New service defined
architecture.service.removed  # Service deprecated
architecture.review.requested # Design review needed
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

### Decision Payload
```json
{
  "decision_id": "decision-456",
  "title": "Adopt BGP for all networking",
  "content": "Full mesh BGP with BFD...",
  "owner": "architect",
  "priority": 2,
  "status": "approved"
}
```

### Alert Payload
```json
{
  "alert_id": "alert-789",
  "severity": "critical",
  "source": "vambraces",
  "title": "Container health check failed",
  "description": "busboy-1 unhealthy for 3 checks",
  "affected_service": "busboy",
  "metadata": {
    "container_id": "busboy-1",
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
| Captain | `decisions.*`, `alerts.escalated` |
| Micromanager | `tasks.*` |
| Architect | `architecture.*` |
| Cuirass | `state.*` |
| Vambraces | `metrics.*`, `alerts.*` |
| All Services | `alerts.critical` (on critical failure) |

### Who Subscribes to What

| Service | Subscribed Topics | Purpose |
|---------|-------------------|---------|
| **Captain** | `alerts.critical`, `timeline.milestone.*` | Strategic oversight |
| **Micromanager** | `alerts.critical`, `tasks.*`, `timeline.sync` | Task coordination |
| **Timeguru** | `timeline.*`, `tasks.completed` | Progress tracking |
| **Architect** | `architecture.*`, `decisions.approved` | Design tracking |
| **Kanban** | `tasks.*`, `timeline.updates` | UI updates |
| **Cuirass** | `state.*`, `alerts.*` | Control plane orchestration |
| **Vambraces** | `metrics.*`, `alerts.*` | Observability aggregation |

---

## Cross-Service Event Flows

### Task Completion → Timeline Update
```
Micromanager publishes: tasks.completed
   ↓
Timeguru receives: tasks.completed
   ↓
Timeguru updates timeline.md
   ↓
Timeguru publishes: timeline.updates
   ↓
Kanban receives: timeline.updates → refreshes UI
Captain receives: timeline.updates → tracks progress
```

### Alert Escalation
```
Vambraces detects anomaly
   ↓
Vambraces publishes: alerts.warning
   ↓
Micromanager receives → creates task
Cuirass receives → prepares remediation
   ↓
If unresolved after timeout:
   ↓
Vambraces publishes: alerts.critical
   ↓
Captain receives: alerts.critical → executive notification
```

### Decision → Implementation
```
Captain publishes: decisions.created
   ↓
Architect receives → validates technical feasibility
   ↓
Captain publishes: decisions.approved
   ↓
Micromanager receives → creates implementation tasks
Architect receives → creates ADRs
   ↓
Tasks cascade through normal flow
```

---

## Client Configuration

### Service Bootstrap Pattern
```go
// Standard service initialization
func initBusboy(addr string, serviceName string, topics []string) (*busboyClient.Client, error) {
    client, err := busboyClient.NewClient(addr)
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

**Captain:**
```go
[]string{"alerts.critical", "timeline.milestone.*", "decisions.*"}
```

**Micromanager:**
```go
[]string{"alerts.critical", "tasks.*", "timeline.sync", "decisions.approved"}
```

**Architect:**
```go
[]string{"architecture.*", "decisions.approved"}
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

Each service MUST export these Busboy metrics:
```
busboy_messages_published_total{service, topic}
busboy_messages_received_total{service, topic}
busboy_publish_duration_seconds{service, topic}
busboy_subscription_status{service, topic, status}
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
