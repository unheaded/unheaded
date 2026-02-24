# Transport Cascade — gRPC-First

All inter-service communication uses a gRPC-first transport with automatic HTTP fallback.

## Priority Order

1. **gRPC** (primary) — Wotan gRPC streaming on port 18001
2. **HTTP** (fallback) — Wotan HTTP API on port 18000

## Package: `pkg/transport/`

- `transport.go` — Core types: `Type`, `Config`, `Connection` interface
- `grpc.go` — gRPC connection implementation
- `http.go` — HTTP connection implementation
- `cascade.go` — Auto-detection: try gRPC first, fall back to HTTP
- `health.go` — Health server with HEALTHY/DEGRADED/DOWN states, HTTP endpoints
- `flags.go` — CLI flags and environment variable configuration

## Connection Interface

```go
type Connection interface {
    Type() Type
    Publish(ctx context.Context, topic string, data []byte) error
    Subscribe(ctx context.Context, topic string) (<-chan []byte, error)
    Close() error
    Healthy() bool
}
```

## Health States

| State | HTTP Response | Meaning |
|-------|--------------|---------|
| HEALTHY | 200 | Service fully operational |
| DEGRADED | 503 | Partial functionality |
| DOWN | 503 | Service unavailable |

## Configuration

Environment variables: `TRANSPORT_TYPE`, `TRANSPORT_GRPC_ADDR`, `TRANSPORT_HTTP_ADDR`, `TRANSPORT_TIMEOUT`, `TRANSPORT_FALLBACK`

All 10 services are wired via `transport.ConfigFromEnv()`.
