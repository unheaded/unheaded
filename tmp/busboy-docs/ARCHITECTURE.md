# Busboy Architecture

This document describes the internal architecture of Busboy, the message bus and broker component of the Unheaded infrastructure platform.

## Table of Contents

1. [Overview](#overview)
2. [Core Components](#core-components)
3. [Data Plane](#data-plane)
4. [Control Plane](#control-plane)
5. [Observability](#observability)
6. [Reliability](#reliability)
7. [Security](#security)
8. [Performance](#performance)
9. [Deployment](#deployment)

## Overview

Busboy is a high-throughput, low-latency message bus designed for service-to-service communication in distributed systems. It follows a dual-plane architecture:

| Plane | Transport | Purpose | Latency Target |
|-------|-----------|---------|----------------|
| Control | REST/HTTP | Management, admin, health | < 50ms |
| Data | gRPC | Message streaming | < 5ms |

### Technology Stack

- **Language**: Go 1.21+
- **Protocols**: HTTP/2 (REST), gRPC (streaming), TLS 1.3
- **Storage**: In-memory ring buffers
- **Observability**: Prometheus metrics, zerolog structured logging
- **Container**: Multi-stage Docker build with distroless base

## Core Components

### Ring Buffer

Location: `server/internal/ringbuffer/`

The ring buffer provides fixed-capacity, time-ordered message storage per topic.

```go
type RingBuffer struct {
    mu       sync.RWMutex
    buffer   []*Message
    size     int
    head     int
    count    int
    wrapping bool
}
```

Characteristics:

- Fixed size declared at startup (immutable)
- Circular write: oldest messages overwritten when full
- Thread-safe with `sync.RWMutex`
- O(1) write, O(n) read operations
- Sequence numbers monotonically increase (never reset on wrap)

### Topic Management

Location: `server/internal/room/`

Topics (internally called "rooms" in current implementation) provide isolated message namespaces.

```go
type Topic struct {
    ID            string
    Buffer        *ringbuffer.RingBuffer
    KeyValueStore map[string]string
    mu            sync.RWMutex
}
```

Features:

- Isolated ring buffer per topic
- Per-topic key-value metadata store
- Concurrent topic access
- Topic-level statistics

### Pub/Sub Engine

Location: `server/internal/busboy/`

The pub/sub engine manages subscriptions and message fanout.

```go
type Busboy struct {
    subscriptions map[string][]*Subscription
    mu            sync.RWMutex
}
```

Event types:

- `MESSAGE_CREATED`: New message published
- `MESSAGE_DELETED`: Message tombstoned

Delivery:

- Buffered channels per subscription
- Non-blocking fanout (slow consumers don't block publishers)
- Automatic cleanup on unsubscribe

### Subscriber Management

Location: `server/internal/member/`

Manages subscriber identity and access control.

```go
type Subscriber struct {
    ID          uuid.UUID
    Status      Status  // pending, approved, denied, revoked
    RequestedAt time.Time
    ApprovedBy  string
}
```

State machine: `pending` -> `approved` | `denied` -> (optionally) `revoked`

## Data Plane

### gRPC Streaming Service

Location: `server/internal/grpc/`

Provides bidirectional streaming for real-time message delivery.

```protobuf
service BusboyStream {
    rpc StreamMessages(StreamRequest) returns (stream StreamEvent);
    rpc Ping(PingRequest) returns (PingResponse);
}
```

Features:

- Historical replay with timestamp filtering
- Real-time message broadcasting
- Keep-alive ping/pong
- Graceful stream termination

Authorization:

- Streams require approved subscriber status
- Unauthorized requests receive `PERMISSION_DENIED`

### Message Format

```protobuf
message StreamEvent {
    oneof event {
        MessageEvent message = 1;
        TombstoneEvent tombstone = 2;
        ControlEvent control = 3;
    }
}

message MessageEvent {
    string message_id = 1;
    string topic = 2;
    string sender_id = 3;
    int64 seq = 4;
    int64 created_unix_ms = 5;
    bytes payload = 6;
}
```

## Control Plane

### REST API

Location: `server/internal/api/`

Endpoints:

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/topics/{topic}/subscribe` | Request subscription |
| POST | `/api/v1/topics/{topic}/publish` | Publish message |
| GET | `/api/v1/topics/{topic}/messages` | Fetch historical messages |
| POST | `/api/v1/topics/{topic}/messages/{id}/delete` | Delete message |
| GET | `/api/v1/admin/pending` | List pending subscriptions |
| POST | `/api/v1/admin/approve` | Approve subscription |
| POST | `/api/v1/admin/deny` | Deny subscription |
| GET | `/health` | Health check |
| GET | `/ready` | Readiness probe |
| GET | `/metrics` | Prometheus metrics |

### Middleware Stack

Location: `server/internal/middleware/`

Applied in order:

1. Recovery (panic handling)
2. Request ID injection
3. Structured logging
4. Metrics collection
5. Security headers
6. Rate limiting
7. Timeout (30s default)

## Observability

### Structured Logging

Location: `server/internal/logger/`

Uses zerolog for zero-allocation JSON logging.

Log format:

```json
{
    "level": "info",
    "service": "busboy",
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "topic": "events.deploy",
    "method": "POST",
    "path": "/api/v1/topics/events.deploy/publish",
    "status_code": 201,
    "duration_ms": 2.15,
    "message": "request_complete",
    "timestamp": "2026-01-26T12:00:00Z"
}
```

### Prometheus Metrics

Location: `server/internal/metrics/`

HTTP metrics:

- `busboy_http_requests_total{method, path, status}`
- `busboy_http_request_duration_seconds{method, path}`
- `busboy_http_request_size_bytes{method, path}`
- `busboy_http_response_size_bytes{method, path}`

Topic metrics:

- `busboy_topics_total`
- `busboy_topic_buffer_size{topic}`
- `busboy_topic_buffer_usage{topic}`
- `busboy_topic_buffer_wrapped_total{topic}`

Message metrics:

- `busboy_messages_published_total{topic}`
- `busboy_messages_deleted_total{topic}`

Stream metrics:

- `busboy_streams_active{topic}`
- `busboy_stream_messages_sent_total{topic}`
- `busboy_stream_errors_total{topic, error_type}`

System metrics:

- `busboy_goroutines_active`
- `busboy_memory_allocated_bytes`

## Reliability

### Rate Limiting

Location: `server/internal/middleware/ratelimit.go`

Token bucket algorithm with per-client limits.

Configuration:

- Rate: 100 requests/second (configurable)
- Burst: 200 requests (configurable)
- Key: Client IP
- Cleanup: 5-minute interval for inactive limiters

### Circuit Breakers

Location: `server/internal/circuitbreaker/`

States: Closed -> Open -> Half-Open -> Closed

Configuration:

- Failure threshold: 60%
- Minimum requests: 10
- Open timeout: 60s
- Half-open max requests: 3

### Graceful Shutdown

Sequence:

1. Stop accepting new connections
2. Allow 15s for in-flight requests
3. Close all gRPC streams
4. Flush logs

## Security

### TLS Configuration

```go
tlsConfig := &tls.Config{
    MinVersion: tls.VersionTLS13,
    CurvePreferences: []tls.CurveID{
        tls.X25519,
        tls.CurveP256,
    },
}
```

### Security Headers

Applied to all HTTP responses:

- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`
- `Strict-Transport-Security: max-age=31536000; includeSubDomains`
- `Content-Security-Policy: default-src 'self'`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Permissions-Policy: geolocation=(), microphone=(), camera=()`

### Input Validation

- All API inputs validated before processing
- Message deletion requires ownership verification
- Subscriber access verified before stream delivery

## Performance

### Concurrency Model

- HTTP: Goroutine per request
- gRPC: Goroutine per stream
- Pub/sub: Buffered channels for non-blocking fanout

### Memory Efficiency

- Ring buffers: Fixed allocation at startup
- Zero persistence: No disk I/O in critical path
- Distroless container: ~2MB runtime overhead

### Expected Characteristics

| Metric | Target |
|--------|--------|
| Message latency (local) | < 5ms |
| Publish throughput | 100,000+ msg/sec |
| Concurrent streams | 10,000+ |
| Memory per message | ~100 bytes + payload |

## Deployment

### Docker

```bash
# Build
docker build -t unheaded/busboy:latest .

# Run
docker run -p 8080:8080 -p 9090:9090 unheaded/busboy:latest
```

### Kubernetes

Resources:

- Deployment with HPA
- Service (ClusterIP for internal, LoadBalancer for external)
- ConfigMap for configuration
- Secret for TLS certificates
- PodDisruptionBudget for availability

Probes:

- Liveness: `/health`
- Readiness: `/ready`

### Bare Metal

```bash
# Install
cp busboy /usr/local/bin/

# Systemd service
systemctl enable busboy
systemctl start busboy
```

## Future Enhancements

### Planned

- Write-ahead log for message persistence
- Multi-node clustering with consistent hashing
- Message acknowledgment and at-least-once delivery
- Dead letter queues
- HTTP/3 + QUIC support

### Under Consideration

- Schema registry integration
- Message compression
- Multi-tenancy
- Geographic distribution

## References

- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
- [Prometheus Best Practices](https://prometheus.io/docs/practices/)
- [gRPC Performance](https://grpc.io/docs/guides/performance/)
- [Twelve-Factor App](https://12factor.net/)
