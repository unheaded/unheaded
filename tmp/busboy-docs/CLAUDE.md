# Development Context

This document provides context for developers and AI assistants working on Busboy.

## Project Overview

**Busboy** is a high-performance message bus and broker, part of the Unheaded infrastructure automation platform.

| Attribute | Value |
|-----------|-------|
| Language | Go 1.21+ |
| Primary Protocol | gRPC (streaming) |
| Secondary Protocol | REST (control plane) |
| Storage | In-memory ring buffers |
| Status | Pre-Alpha |

## Repository Structure

```
busboy/
├── cmd/busboy/           # Application entry point
├── server/
│   ├── cmd/server/       # Server main
│   └── internal/
│       ├── api/          # REST handlers
│       ├── busboy/       # Pub/sub engine
│       ├── circuitbreaker/
│       ├── grpc/         # gRPC streaming service
│       ├── logger/       # Structured logging
│       ├── member/       # Subscriber management
│       ├── metrics/      # Prometheus instrumentation
│       ├── middleware/   # HTTP middleware
│       ├── ringbuffer/   # Ring buffer implementation
│       └── room/         # Topic management
├── client/terminal/      # CLI client
├── proto/                # Protocol buffer definitions
├── init-db/              # Database initialization (if applicable)
└── scripts/              # Build and utility scripts
```

## Building

```bash
# Generate protobuf code
cd proto && ./generate.sh && cd ..

# Build server
cd server && go build -o ../bin/busboy ./cmd/server

# Build CLI
cd client/terminal && go build -o ../../bin/busboy-cli .
```

## Running Locally

```bash
# Development mode (pretty logs)
./bin/busboy --buffer-size 10000 --http-port 8080 --grpc-port 9090 --log-pretty

# With TLS
./bin/busboy --enable-tls --tls-cert server.crt --tls-key server.key
```

## Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go | Performance, concurrency primitives, single binary deployment |
| Streaming | gRPC | Bidirectional streaming, protobuf efficiency, HTTP/2 |
| Control API | REST | Simplicity for admin operations, broad client support |
| Storage | In-memory | Maximum throughput, ephemeral by design |
| Persistence | None (planned) | Ship fast, add durability in Phase 2 |

## Code Conventions

### Error Handling

Return errors with context:

```go
if err != nil {
    return fmt.Errorf("failed to publish to topic %s: %w", topicID, err)
}
```

### Logging

Use structured logging with context:

```go
logger.Info(ctx, "message_published",
    "topic", topicID,
    "size_bytes", len(payload))
```

### Metrics

Register metrics in `internal/metrics/metrics.go`:

```go
var MessagesPublished = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: "busboy",
        Name:      "messages_published_total",
        Help:      "Total messages published",
    },
    []string{"topic"},
)
```

### gRPC Status Codes

| Condition | Code |
|-----------|------|
| Invalid input | `InvalidArgument` |
| Resource not found | `NotFound` |
| Authorization failed | `PermissionDenied` |
| Rate limited | `ResourceExhausted` |
| Internal error | `Internal` |

## Testing

```bash
# Run tests
cd server && go test ./...

# With race detection
go test -race ./...

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## API Overview

### REST Endpoints

| Method | Endpoint | Purpose |
|--------|----------|---------|
| POST | `/api/v1/topics/{topic}/subscribe` | Request subscription |
| POST | `/api/v1/topics/{topic}/publish` | Publish message |
| GET | `/api/v1/topics/{topic}/messages` | Fetch messages |
| POST | `/api/v1/admin/approve` | Approve subscriber |
| POST | `/api/v1/admin/deny` | Deny subscriber |
| GET | `/health` | Health check |
| GET | `/metrics` | Prometheus metrics |

### gRPC Services

```protobuf
service BusboyStream {
    rpc StreamMessages(StreamRequest) returns (stream StreamEvent);
    rpc Ping(PingRequest) returns (PingResponse);
}
```

## Current Implementation Status

### Completed

- Ring buffer (thread-safe, configurable size)
- Pub/sub engine with fanout
- gRPC bidirectional streaming
- REST control plane
- Prometheus metrics
- Structured logging (zerolog)
- Rate limiting (token bucket)
- Circuit breakers
- TLS 1.3 support
- Subscriber approval workflow

### In Progress

- Comprehensive test suite
- Performance benchmarks
- Documentation updates

### Planned

- Message persistence (WAL)
- Clustering (multi-node)
- Message acknowledgment
- Dead letter queues
- HTTP/3 + QUIC

## Development Guidelines

### When Adding Features

1. Design the API contract first
2. Add metrics instrumentation
3. Include structured logging
4. Write tests (unit + integration)
5. Update documentation

### When Modifying Core Components

1. Ensure thread-safety with appropriate synchronization
2. Maintain backward compatibility where possible
3. Update benchmarks to validate performance
4. Consider impact on observability

### Performance Considerations

- Avoid allocations in hot paths
- Use buffered channels appropriately
- Profile before optimizing
- Document performance characteristics

## Related Projects

| Project | Purpose |
|---------|---------|
| [unheaded/chat](https://github.com/unheaded/chat) | Chat application built on Busboy |
| [unheaded](https://github.com/unheaded) | Infrastructure automation platform |

## Contact

For questions about the codebase or architecture decisions, refer to the documentation or open an issue in the repository.
