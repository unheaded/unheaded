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
| DEGRADED | 200 (body says `DEGRADED`) | Partial functionality |
| DOWN | 503 | Service unavailable; `/ready` is 503 too |

## Configuration

`transport.ConfigFromEnv()` reads:

| Env | Sets |
|-----|------|
| `TRANSPORT_TYPE` | `grpc`, `http` or `auto` |
| `WOTAN_GRPC_ADDR` | Wotan gRPC address |
| `WOTAN_ADDR`, `WOTAN_HTTP_ADDR` | Wotan HTTP address, `host:port` or URL; `WOTAN_HTTP_ADDR` wins if both are set |
| `CONNECT_TIMEOUT` | connect timeout (Go duration) |

`transport.RegisterFlags()` defines `--transport`, `--wotan-grpc-addr`, `--wotan-http-addr`, `--connect-timeout` and `--fallback`, but no binary calls it: services take their own `--wotan`-style flags or the environment.

11 binaries call `ConfigFromEnv()`: timeguru, captain, architect, micromanager, monad, sophia, dashboard-backend, kanban-app, unheaded-daemon, trace-collector-go, chaos-controller.
