# ADR-034: gRPC with mTLS as Default Inter-Service Transport

## Status: ACCEPTED

## Date: 2026-04-02
## Decision Makers: Stevie Bellis (Principal), Claude (Advisor)

---

## Context

The Unheaded Kingdom's inter-service communication currently supports both gRPC and HTTP REST APIs. The `pkg/transport/` cascade system tries gRPC first and falls back to HTTP. However, in production:

1. **gRPC is faster** — binary protobuf vs JSON, persistent HTTP/2 connections, bidirectional streaming
2. **mTLS is non-negotiable** — zero-trust architecture requires mutual authentication between all services
3. **REST is a crutch** — useful for debugging and browser access, but should never be the primary path
4. **Cert management is the hard part** — generating, distributing, rotating, and revoking certs across hosts

The existing `pkg/transport/mtls/` package already implements:
- Server and client TLS configs with certificate verification
- Certificate rotation via `CertRotator` (file-watch based)
- Integration helpers (`SetupServiceTLS`, `SetupServiceClientTLS`)
- RequireAndVerifyClientCert enforcement

What's missing: a standardized PKI bootstrap, cert generation tooling, and operational runbooks for deploying mTLS across the Kingdom.

## Decision

**gRPC with mTLS is the MANDATORY default transport for all inter-service communication.** REST/HTTP is the worst-case fallback only — for debugging, health checks from external tooling, and browser-based UIs.

### Architecture

```
┌──────────────────────────────────────────────────────┐
│                    PKI Root (WEST)                     │
│              /var/lib/unheaded/pki/ca/                │
│         ca.crt + ca.key (offline, age-encrypted)      │
└──────────────────┬───────────────────────────────────┘
                   │ signs
        ┌──────────┼──────────┐
        ▼          ▼          ▼
   ┌─────────┐ ┌─────────┐ ┌─────────┐
   │ wotan   │ │timeguru │ │captain  │  ... (per-service certs)
   │ .crt    │ │ .crt    │ │ .crt    │
   │ .key    │ │ .key    │ │ .key    │
   └─────────┘ └─────────┘ └─────────┘

Transport Priority:
  1. gRPC + mTLS (port :gRPC) ← DEFAULT, REQUIRED
  2. HTTP + TLS  (port :HTTP) ← fallback for browsers/debugging
  3. HTTP plain  (localhost only) ← health checks, metrics scraping
```

### Certificate Hierarchy

| Level | Certificate | Purpose | Lifetime |
|-------|------------|---------|----------|
| Root CA | `ca.crt` / `ca.key` | Signs all service certs | 10 years |
| Service | `<svc>.crt` / `<svc>.key` | Per-service identity | 1 year |
| Node | `<host>.crt` / `<host>.key` | Per-host identity (for Akira, daemon) | 1 year |

### PKI Directory Layout

```
/var/lib/unheaded/pki/
├── ca/
│   ├── ca.crt              # CA certificate (distributed to all nodes)
│   └── ca.key.age          # CA private key (age-encrypted, WEST only)
├── services/
│   ├── wotan.crt
│   ├── wotan.key
│   ├── timeguru.crt
│   ├── timeguru.key
│   ├── captain.crt
│   ├── captain.key
│   └── ...                 # One cert+key per service
└── hosts/
    ├── west.crt
    ├── west.key
    ├── east.crt
    └── east.key
```

### Service Configuration

Each service reads its cert/key from a well-known path:

```go
// In service startup (standardized via pkg/service/)
tlsConfig, err := mtls.SetupServiceTLS("wotan", "/var/lib/unheaded/pki")
if err != nil {
    log.Fatal().Err(err).Msg("mTLS setup failed — refusing to start without certs")
}

// gRPC server with mTLS
grpcServer := grpc.NewServer(
    grpc.Creds(credentials.NewTLS(tlsConfig)),
)

// gRPC client with mTLS
clientTLS, _ := mtls.SetupServiceClientTLS("timeguru", "/var/lib/unheaded/pki")
conn, _ := grpc.Dial(addr, grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
```

### Cert Rotation

The existing `CertRotator` watches cert files via fsnotify. Rotation workflow:

1. Generate new cert signed by same CA
2. Write to same path (atomic rename)
3. `CertRotator` detects change, hot-reloads
4. Zero downtime — no service restart needed

### REST Fallback Rules

REST/HTTP endpoints remain available but are secondary:

| Endpoint | Transport | Auth |
|----------|-----------|------|
| `/health`, `/ready` | HTTP plain (localhost only) | None (local only) |
| `/metrics` | HTTP plain (internal network) | None (Prometheus scrape) |
| `/api/v1/*` | HTTPS + mTLS | Client cert required |
| gRPC services | gRPC + mTLS | Client cert required |

Binding rules:
- gRPC: `0.0.0.0:<grpc_port>` (mTLS enforced)
- HTTP API: `0.0.0.0:<http_port>` (TLS enforced for `/api/v1/*`)
- Health/metrics: `127.0.0.1:<http_port>` or `0.0.0.0:<http_port>` with no auth on `/health`

## Consequences

### Positive
- **Zero-trust by default** — no service can talk to another without a valid cert signed by Kingdom CA
- **Performance** — gRPC binary framing + HTTP/2 multiplexing + persistent connections
- **Streaming** — bidirectional gRPC streaming for Wotan subscriptions, log tailing, health reporting
- **Cert rotation** — hot-reload without restart, already implemented in `pkg/transport/mtls/`
- **Auditability** — every connection has a verified identity (service name in cert CN/SAN)

### Negative
- **Operational complexity** — cert generation, distribution, and rotation require tooling and runbooks
- **Bootstrap chicken-and-egg** — first service start needs certs before Wotan is up to distribute them
- **Debugging harder** — can't just `curl` without a client cert (mitigated by localhost health endpoints)

### Mitigations
- Runbook: `runbooks/pki/generate-kingdom-certs.yaml` — automated cert generation
- Runbook: `runbooks/pki/rotate-service-cert.yaml` — per-service cert rotation
- Bootstrap: certs generated on WEST, distributed via `scp` or sealed cask before first boot
- Debug: `curl --cert <svc>.crt --key <svc>.key` or use plain HTTP health endpoints on localhost

## Implementation Plan

1. **Phase 1**: Generate CA + service certs using `openssl` (runbook)
2. **Phase 2**: Distribute certs to EAST via `scp` (runbook)
3. **Phase 3**: Wire `pkg/transport/mtls/` into all 10 services at startup
4. **Phase 4**: Update systemd units to set `PKI_DIR=/var/lib/unheaded/pki`
5. **Phase 5**: Verify gRPC+mTLS connections between all service pairs
6. **Phase 6**: Lock down REST endpoints (TLS required for non-localhost)

## References

- `pkg/transport/mtls/` — existing mTLS implementation (mtls.go, integration.go, 8 tests)
- `pkg/transport/cascade.go` — gRPC-first cascade with HTTP fallback
- ADR-015 — Go-Fiber HTTP Layer (REST fallback framework)
- ADR-008 — Security Hardening Baseline
