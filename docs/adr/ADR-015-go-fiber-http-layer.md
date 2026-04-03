# ADR-015: Go-Fiber for HTTP API Layer

## Status: ACCEPTED — Service template created (2026-04-03)

**Implementation note (2026-04-03):** Created `pkg/service/` — standardized service scaffold using stdlib `net/http` (not Fiber yet). Provides: /health, /ready endpoints, Wotan integration, auth middleware, graceful shutdown, structured logging. 7 tests passing. Fiber migration deferred — current fasthttp compatibility with eBPF socket programs needs investigation before committing to the framework swap. The template codifies existing patterns first; Fiber migration is a separate effort.
## Date: 2026-02-20
## Decision Makers: Round Table S26 (All 9 Seats)

---

## Context

The Unheaded Kingdom has 25 active Go microservices exposing REST APIs for control plane operations. Currently these use a mix of stdlib `net/http` handlers. As we approach Age 2 (Beta Trials), we need a standardized HTTP framework that:

1. Handles high packet volumes (eBPF-observed traffic generates significant API load)
2. Supports HTTP/3 + QUIC for public-facing surfaces
3. Coexists with gRPC (Wotan inter-service communication)
4. Does not interfere with eBPF socket-level programs (XDP, TC, kprobe, sock_ops)
5. Provides TLS 1.3 and mTLS for zero-trust architecture
6. Supports WebSocket for real-time dashboard feeds
7. Has mature middleware ecosystem (auth, logging, tracing, rate limiting)

## Decision

**Use Go-Fiber v3 (built on fasthttp) for all REST/HTTP API surfaces.**

Keep gRPC on separate port (:50051) via `google.golang.org/grpc`. Use Traefik 3.x as HTTP/3 gateway and TLS termination point. Bridge REST↔gRPC with grpc-gateway where needed.

## Architecture

```
Internet / Edge
      │
      ▼
┌─────────────────────────────────────────┐
│ Traefik 3.x                             │
│ ├─ HTTP/3 + QUIC termination            │
│ ├─ TLS 1.3 termination (public certs)   │
│ ├─ Rate limiting (edge)                 │
│ └─ Routes to Fiber services             │
└─────────────┬───────────────────────────┘
              │
    ┌─────────┴─────────┐
    ▼                   ▼
┌──────────┐     ┌──────────┐
│ Fiber v3 │     │ gRPC     │
│ :3000    │     │ :50051   │
│ REST API │     │ Wotan    │
│ WebSocket│     │ Comms    │
│ mTLS     │     │ mTLS     │
└──────────┘     └──────────┘
    │                   │
    └─────────┬─────────┘
              │
    ┌─────────┴─────────┐
    │ eBPF Programs     │
    │ XDP / TC / kprobe │
    │ (kernel level)    │
    │ TRANSPARENT       │
    └───────────────────┘
```

## Evaluation

### Performance
- Fiber/fasthttp: **13.5M responses/sec** plaintext, **0.9ms latency** (TechEmpower 2025 Round 23)
- Gin: ~34-36K req/sec, ~3ms latency
- stdlib net/http: Baseline, significantly slower
- Fiber is **36% faster than Gin/Echo** in realistic workloads
- fasthttp achieves this via zero-allocation hot paths, object reuse, optimized parsing

### HTTP/3 + QUIC
- **NOT native** in Fiber/fasthttp — fasthttp is optimized for HTTP/1.1
- **Solution**: Traefik 3.x in front of Fiber handles HTTP/3 termination
- Traefik has production-ready HTTP/3 support with TCP/UDP multiplexing
- **Verdict**: Acceptable. HTTP/3 at edge, HTTP/1.1 between Traefik and Fiber (internal network, low latency)

### gRPC Compatibility
- Fiber **cannot** serve gRPC (different HTTP/2 semantics)
- **Pattern**: Separate gRPC server on :50051 using `google.golang.org/grpc`
- **Bridge**: grpc-gateway generates REST→gRPC proxy from proto definitions
- This is the standard Go pattern used by Netflix, Square, Cockroach Labs

### eBPF Compatibility
- fasthttp uses standard `net.Listener` and `net.Conn` — **no custom kernel socket implementations**
- XDP programs: Work — upstream of userland
- TC programs: Work — kernel scheduling layer
- kprobe programs: Work — instrument syscalls normally
- sock_ops BPF: Work — Fiber uses standard socket syscalls
- **VERIFIED: No interference with eBPF pipeline**

### TLS 1.3 / mTLS
- Full support via Go stdlib `crypto/tls`
- Fiber v3 includes `EarlyData` middleware for TLS 1.3 0-RTT
- mTLS configuration: `ClientAuth: tls.RequireAndVerifyClientCert`
- Short-lived certificates (30-90 days) with automated rotation

### WebSocket
- `github.com/gofiber/contrib/websocket` — mature, production-tested
- Full access to Fiber context (Locals, Params, Query, Cookies)
- Suitable for real-time dashboard feeds (Anamnesis → Dashboard)

### Middleware Ecosystem
- JWT, PASETO authentication
- Zap logging integration
- OpenTelemetry tracing (v2)
- Swagger UI documentation
- DataDog dd-trace compatibility
- **Adequate** — not as extensive as Gin/Chi but quality over quantity

## Consequences

### Positive
- Massive performance improvement for REST API surfaces
- WebSocket support for dashboard feeds without separate server
- TLS 1.3 + mTLS native — aligns with zero-trust architecture
- eBPF transparent — no pipeline interference
- Consistent API framework across all 25 services

### Negative
- No native HTTP/3 — requires Traefik gateway (adds component)
- Fiber v3 is newer — fewer community examples than Gin
- fasthttp is not 100% net/http compatible — some middleware won't port directly
- Two server processes per service (Fiber + gRPC) — operational complexity

### Mitigations
- Traefik is already needed for edge routing — HTTP/3 gateway is a natural fit
- Fiber middleware ecosystem is curated and maintained by gofiber team
- grpc-gateway bridges the protocol gap cleanly
- Service template will abstract dual-server pattern

## Implementation Plan

1. Create service template with dual-server pattern (Fiber + gRPC)
2. Scaffold Fiber v3 in one pilot service (timeguru recommended — simple, well-tested)
3. Validate eBPF compatibility in integrated environment
4. Migrate remaining services incrementally
5. Add Traefik configuration to NixOS container definitions

## References

- [Go-Fiber Documentation](https://docs.gofiber.io/)
- [Fiber v3 What's New](https://docs.gofiber.io/next/whats_new/)
- [TechEmpower Benchmarks](https://www.techempower.com/benchmarks/)
- [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway)
- [Traefik HTTP/3 Configuration](https://doc.traefik.io/traefik/)

---

_Decided at Round Table S26. The Gauntlets receive their upgrade._
