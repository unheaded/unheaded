---
name: unheaded-messagebus
description: |
  Infrastructure message bus skill for Unheaded. Understands the Busboy Protocol (BP/1), REST control plane, gRPC data plane, ring buffer architecture, pub/sub patterns, and subscriber approval workflow. Use this skill when working on message bus features, inter-service communication, event-driven patterns, or debugging message flow. Knows where the code lives, what's shipped, and what's planned. NOTE: This is different from unheaded-busboy (the coordinator skill). This is for the MESSAGE BUS infrastructure component. Triggers: message bus, messagebus, pubsub, pub/sub, gRPC streaming, ring buffer, topics, subscribers, events, inter-service, messaging, BP/1, busboy server, busboy client.
---

# Unheaded Message Bus

**The infrastructure message bus that ties everything together.**

The Busboy message bus is Unheaded's nervous system - every service talks through it. This skill knows the protocol, the architecture, and the code.

> **Note**: Don't confuse this with `unheaded-busboy` - that's the coordinator skill that helps navigate between team skills. THIS skill is about the infrastructure message bus component (the code in `/busboy/`).

## What Busboy Is

**NOT a chat app.** Busboy is infrastructure messaging:
- Service-to-service communication
- Event distribution (eBPF data, task updates, alerts)
- Log aggregation pipeline
- Metrics routing
- Alert dispatch

## Protocol: BP/1

Busboy speaks two protocols:

### REST Control Plane (Port 8080)

For management operations:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/topics` | GET | List all topics |
| `/api/v1/topics/{topic}` | GET | Get topic details |
| `/api/v1/topics/{topic}/subscribe` | POST | Request subscription |
| `/api/v1/topics/{topic}/subscribers` | GET | List subscribers |
| `/api/v1/topics/{topic}/subscribers/{id}/approve` | POST | Approve subscriber |
| `/api/v1/topics/{topic}/publish` | POST | Publish message |
| `/api/v1/topics/{topic}/messages` | GET | Get messages |
| `/api/v1/health` | GET | Health check |
| `/api/v1/metrics` | GET | Prometheus metrics |

### gRPC Data Plane (Port 9090)

For high-throughput messaging:

```protobuf
service BusboyService {
  rpc Subscribe(SubscribeRequest) returns (SubscribeResponse);
  rpc Publish(PublishRequest) returns (PublishResponse);
  rpc StreamMessages(StreamRequest) returns (stream Message);
}
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      BUSBOY SERVER                               │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────────┐  ┌──────────────────┐                     │
│  │   REST Control   │  │   gRPC Data      │                     │
│  │   Plane (:8080)  │  │   Plane (:9090)  │                     │
│  └────────┬─────────┘  └────────┬─────────┘                     │
│           │                     │                                │
│           ▼                     ▼                                │
│  ┌──────────────────────────────────────────┐                   │
│  │            PUB/SUB ENGINE                 │                   │
│  │  ┌──────────┐  ┌──────────┐  ┌─────────┐ │                   │
│  │  │  Topics  │  │Subscribers│  │ Fanout  │ │                   │
│  │  └──────────┘  └──────────┘  └─────────┘ │                   │
│  └────────────────────┬─────────────────────┘                   │
│                       │                                          │
│                       ▼                                          │
│  ┌──────────────────────────────────────────┐                   │
│  │            RING BUFFER                    │                   │
│  │    ┌─────────────────────────────────┐   │                   │
│  │    │ msg │ msg │ msg │ ... │ msg │   │   │                   │
│  │    └─────────────────────────────────┘   │                   │
│  │    Thread-safe, configurable size        │                   │
│  └──────────────────────────────────────────┘                   │
└─────────────────────────────────────────────────────────────────┘
```

## Code Locations

```
/unheaded/
├── busboy/                    # The message bus server
│   ├── cmd/busboy/           # Server binary
│   ├── cmd/busboy-cli/       # CLI client
│   ├── internal/
│   │   ├── buffer/           # Ring buffer implementation
│   │   ├── pubsub/           # Pub/sub engine
│   │   ├── grpc/             # gRPC service
│   │   ├── rest/             # REST API
│   │   ├── middleware/       # Rate limiting, circuit breaker
│   │   └── metrics/          # Prometheus metrics
│   ├── ARCHITECTURE.md       # Detailed architecture
│   ├── SPEC.md               # Protocol specification
│   ├── QUICKSTART.md         # Getting started
│   └── PROGRESS.md           # Development progress
│
└── unheaded/
    └── pkg/busboy-client/    # Go client library
        ├── client.go         # Full client implementation
        └── mock/             # Mock for testing
            └── mock.go       # Test doubles
```

## Subscriber Approval Workflow

Busboy uses explicit subscription approval:

1. **Request**: Client calls `/subscribe` with display name
2. **Pending**: Server creates subscription in `pending` state
3. **Approval**: Admin approves via REST API or auto-approve policy
4. **Active**: Client can now publish/consume messages

```go
// Client side
sub, err := client.Subscribe(ctx, "events.system", "my-service")
if sub.Status == "pending" {
    // Wait for approval or handle pending state
}

// Server side (auto-approve in dev)
// Or manual: POST /api/v1/topics/{topic}/subscribers/{id}/approve
```

## Session Start Protocol

When working on Busboy:

1. **Check PROGRESS.md** - What's shipped, what's pending
2. **Check CLAUDE.md** - Current focus, active work
3. **Understand the scope** - Busboy is infrastructure, not chat

## Topic Naming Convention

```
<domain>.<type>[.<detail>]

Examples:
- system.events       - Infrastructure events
- ebpf.packets        - eBPF packet trace data
- ebpf.latency        - Latency probe events
- tasks.created       - New kanban tasks
- tasks.updated       - Task updates
- metrics.collected   - Metrics pipeline
- alerts.triggered    - Alert events
```

## Client Usage

```go
import busboyClient "github.com/unheaded/unheaded/pkg/busboy-client"

// Connect
client, err := busboyClient.NewClient("localhost:9090")
if err != nil {
    return fmt.Errorf("connect: %w", err)
}
defer client.Close()

// Subscribe
sub, err := client.Subscribe(ctx, "system.events", "my-service")
if err != nil {
    return fmt.Errorf("subscribe: %w", err)
}

// Publish (requires approved subscription)
err = client.Publish(ctx, "system.events", []byte(`{"type":"started"}`))

// Stream messages
msgCh, err := client.StreamMessages(ctx, "system.events")
for msg := range msgCh {
    fmt.Printf("Received: %s\n", msg.Payload)
}
```

## Testing with Mock

```go
import "github.com/unheaded/unheaded/pkg/busboy-client/mock"

func TestMyService(t *testing.T) {
    // Create mock with auto-approve
    mockClient := mock.NewMockClient(mock.WithAutoApprove())
    defer mockClient.Close()

    // Subscribe
    sub, err := mockClient.Subscribe(ctx, "test.topic", "test")
    require.NoError(t, err)
    assert.Equal(t, "approved", sub.Status)

    // Publish
    err = mockClient.Publish(ctx, "test.topic", []byte("test"))
    require.NoError(t, err)

    // Verify
    assert.Equal(t, int64(1), mockClient.GetPublishCount())
}
```

## Current Status

**Phase 1: Core Message Bus - COMPLETE**

| Component | Status |
|-----------|--------|
| Ring Buffer | ✅ Complete |
| Topic Management | ✅ Complete |
| Pub/Sub Engine | ✅ Complete |
| gRPC Streaming | ✅ Complete |
| REST Control Plane | ✅ Complete |
| Prometheus Metrics | ✅ Complete |
| Rate Limiting | ✅ Complete |
| Circuit Breakers | ✅ Complete |
| TLS 1.3 Support | ✅ Complete |
| CLI Client | ✅ Complete |
| Go Client Library | ✅ Complete |
| Mock Client | ✅ Complete |
| Unit Tests | 🚧 In Progress |
| Benchmarks | ⏳ Pending |

## Reference Docs

- `/busboy/ARCHITECTURE.md` - Full architecture details
- `/busboy/SPEC.md` - Protocol specification (BP/1)
- `/busboy/QUICKSTART.md` - Getting started guide
- `/busboy/PROGRESS.md` - Development progress

## Cross-Skill Integration

| Skill | Busboy Provides | Busboy Needs |
|-------|----------------|--------------|
| **Developer** | Client library, mock for testing | Unit tests, code quality |
| **Architect** | Message routing, event distribution | Integration patterns |
| **Micromanager** | Progress updates | QA sign-off, DoD verification |
| **TimeGuru** | Milestone status | Timeline tracking |
