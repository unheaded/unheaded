# Transport Cascade — gRPC-First

All inter-service communication uses a gRPC-first transport with automatic HTTP fallback.

## Priority Order

1. **gRPC** (primary) — Wotan gRPC streaming on port 18001
2. **HTTP** (fallback) — Wotan HTTP API on port 18000

## Streams: gRPC, polling only while gRPC is down (ADR-147, 2026-11)

- A stream on `wotanClient.Client` (`NewClientWithGRPC`) is a gRPC stream. One loop runs it for its whole life (`pkg/wotan-client/stream_loop.go`): if gRPC fails past its reconnect budget, the client degrades, polls over HTTP every 500 ms from the last delivered seq, and re-probes `grpc.health.v1` every 30 s; when the probe answers it stops polling and reopens gRPC from the seq the poller last delivered. Until 2026-11 a stream that fell back polled for good (68 GET/s on west).
- A client with no gRPC address (`NewClient`) refuses to stream (`ErrNoStreamTransport`) unless built `WithHTTPPollStreams()`; `policy_test.go` refuses that option under `cmd/` and `services/*/cmd/`.
- `/health` shows each stream's transport: `stream_transport` (`grpc` or `http-poll`); `transport` is `gRPC` only while no stream polls.
- Series: `unheaded_wotan_client_polls_total{topic}` (each HTTP poll; 0 for a gRPC stream) and `unheaded_wotan_client_stream_transport{topic,transport}` (1 for the transport in use, the other absent).
- `pkg/transport`'s health server says `"grpc": null` until the service sets it (it said `true` by default).

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
| DEGRADED | 200 (body says `DEGRADED`) | Partial functionality; `/ready` is 200 too |
| DOWN | 503 | Service unavailable; `/ready` is 503 too |

DEGRADED means one of the two transports is down: the gRPC stream has fallen back to HTTP polling, which is the designed fallback, and the service still serves. So `/ready` stays 200 on DEGRADED and turns 503 only on DOWN; a 503 there would take a working service out of anything that honours readiness. `TestHealthServer_ReadyEndpoint_Degraded` (`pkg/transport`) holds it. The SLOs read `up`, latency and errors, not `/health` or `/ready` (settled 2026-11-21; decision pack 2026-11-18, row 41, M-167.1).

## Configuration

`transport.ConfigFromEnv()` reads:

| Env | Sets |
|-----|------|
| `TRANSPORT_TYPE` | `grpc`, `http` or `auto` |
| `WOTAN_GRPC_ADDR` | Wotan gRPC address |
| `WOTAN_ADDR`, `WOTAN_HTTP_ADDR` | Wotan HTTP address, `host:port` or URL; `WOTAN_HTTP_ADDR` wins if both are set |
| `CONNECT_TIMEOUT` | connect timeout (Go duration) |

`pkg/wotan-client` itself reads one more, in every binary:

| Env | Sets |
|-----|------|
| `WOTAN_TLS_SERVER_NAME` | the name the client verifies Wotan's certificate against, on gRPC and HTTP, when that is not the host it dials (a host name or an IP address; unset: the dialled host). For a client that reaches Wotan at an address its certificate does not carry, such as alert-mailer over the host's overlay address ([[Wotan Topic Signing|Wotan-Topic-Signing]]). Verification stays on: the chain must end at the trusted CA and the certificate must carry this name. A value with a port, a scheme or brackets stops the client from being built |

`transport.RegisterFlags()` defines `--transport`, `--wotan-grpc-addr`, `--wotan-http-addr`, `--connect-timeout` and `--fallback`, but no binary calls it: services take their own `--wotan`-style flags or the environment.

7 binaries call `ConfigFromEnv()`: timeguru, dashboard-backend, kanban-app, unheaded-daemon, trace-collector-go, chaos-controller, sword.
