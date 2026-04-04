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

**Design principle:** Short-lived certs, automated rotation. We own the CA, the
rotation tooling (`CertRotator` + fsnotify), and the health daemon (Akira). There
is zero reason to issue long-lived certs. A compromised cert's blast radius equals
its remaining validity — shorter = strictly better.

Let's Encrypt proved 90-day certs work at internet scale with ACME automation.
We have something better: we own every layer. Apple enforces 398-day max for public
TLS. Google/Chrome are pushing toward 90-day. We follow the industry, not fight it.

| Level | Certificate | Purpose | Lifetime | Rotation Trigger |
|-------|------------|---------|----------|-----------------|
| Root CA | `ca.crt` / `ca.key` | Signs all service + host certs | **2 years** | Manual ceremony at 18 months |
| Service | `<svc>.crt` / `<svc>.key` | Per-service identity | **90 days** | Akira auto-rotates at 60 days (2/3 lifetime) |
| Node | `<host>.crt` / `<host>.key` | Per-host identity (Akira, daemon) | **90 days** | Akira auto-rotates at 60 days |

**Why 2-year CA, not 10:** If the CA key is compromised, every cert it ever signed
is suspect. A 2-year CA limits the blast radius. Planned rotation ceremony at 18
months gives a 6-month overlap where both old and new CA are trusted (cross-signing).

**Why 90-day service certs, not 1 year:** Stolen service cert = max 90 days of valid
impersonation vs 365. With Akira monitoring cert expiry as a health check and
triggering rotation at 60 days, the actual exposure window is ~30 days worst case.

**Akira cert health check:** Akira runs an exec check every cycle that reads each
service cert's `Not After` date. If any cert is within 30 days of expiry, Akira
triggers the rotation runbook automatically. If rotation fails, Akira escalates
to WARN (cert expiring) → ERROR (< 7 days) → CRITICAL (< 24 hours) via the
standard consensus severity thresholds.

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

### Cert Rotation — Akira-Driven Automation

Two layers: the `CertRotator` (per-service, reactive) and Akira (cluster-wide, proactive).

**CertRotator (pkg/transport/mtls/)** — already implemented:
1. Watches cert files via fsnotify
2. When file changes: hot-reload TLS config
3. Zero downtime — no service restart needed
4. Handles the "last mile" of rotation

**Akira (pkg/health/)** — cert expiry as a health check:
1. Every health cycle, Akira reads `Not After` from each service cert
2. At 60 days remaining (2/3 of 90-day lifetime): trigger rotation runbook
3. Akira calls `openssl` to generate new cert signed by CA, writes to cert path
4. CertRotator detects the file change, hot-reloads — loop complete

**Rotation timeline for a 90-day cert:**
```
Day 0:  Cert issued (valid 90 days)
Day 60: Akira triggers rotation → new cert issued (valid 90 days)
Day 90: Old cert expires (but was replaced 30 days ago)
```

**Failure escalation (if auto-rotation fails):**
```
30 days remaining: Akira retries rotation, logs WARN
 7 days remaining: Akira escalates to ERROR, publishes to system.outage.reports
24 hours remaining: CRITICAL — consensus alert, all dependent services notified
 0 days remaining: Cert expired — mTLS handshakes fail, service isolated
```

**CA rotation ceremony (manual, every 18 months):**
1. Generate new CA on WEST (overlapping validity with old CA)
2. Cross-sign: new CA signs old CA's cert (trust chain continuity)
3. Distribute new CA cert to all nodes (add to trust bundle, don't replace yet)
4. Re-issue all service certs signed by new CA (Akira batch rotation)
5. Remove old CA from trust bundle after all certs are re-issued
6. Age-encrypt new CA key, archive old CA key

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
- Runbook: `runbooks/pki/generate-kingdom-certs.yaml` — CA + 14 service certs (90-day)
- Runbook: `runbooks/pki/rotate-service-cert.yaml` — per-service zero-downtime rotation
- Runbook: `runbooks/pki/akira-cert-expiry-check.yaml` — automated expiry monitoring + rotation trigger
- Bootstrap: certs generated on WEST, distributed via `scp` or sealed cask before first boot
- Debug: `curl --cert <svc>.crt --key <svc>.key` or use plain HTTP health endpoints on localhost
- Akira: cert expiry is a first-class health check — auto-rotate, escalate on failure

## Implementation Plan

1. **Phase 1**: Generate CA (2yr) + service certs (90d) using `openssl` (runbook)
2. **Phase 2**: Distribute certs to EAST via `scp` (runbook)
3. **Phase 3**: Wire `pkg/transport/mtls/` into all 10 services at startup
4. **Phase 4**: Update systemd units to set `PKI_DIR=/var/lib/unheaded/pki`
5. **Phase 5**: Verify gRPC+mTLS connections between all service pairs
6. **Phase 6**: Lock down REST endpoints (TLS required for non-localhost)
7. **Phase 7**: Register Akira cert-expiry exec check (6-hour cycle)
8. **Phase 8**: Test auto-rotation end-to-end (issue short-lived test cert, wait for Akira trigger)

## Security Review (BlackMage / MoatGhost / Sentinel)

**BlackMage (crypto):** 2-year CA is the maximum defensible lifetime for an
internal root. The key is age-encrypted at rest, only decrypted during signing
ceremonies. Ed25519 or ECDSA P-384 would be preferred over RSA for new deployments,
but RSA-4096 is acceptable for compatibility with existing tooling.

**MoatGhost (perimeter):** 90-day cert lifetime limits blast radius of key
compromise to max 90 days (realistically ~30 days since Akira rotates at 60).
Combined with CRL/OCSP (future: Phase 9), revocation can shrink this further.
Localhost-only health endpoints are acceptable — they're not reachable across
the network boundary.

**Sentinel (ops security):** Akira-driven rotation is the correct pattern.
Human-operated cert rotation is the #1 cause of outages at scale (see: every
cloud provider's postmortem about expired certs). Akira treats cert expiry as
a health signal with graduated severity — same consensus model as service health.
The 6-hour check interval means worst case 6 hours between detection and rotation
trigger, well within the 30-day buffer.

## References

- `pkg/transport/mtls/` — existing mTLS implementation (mtls.go, integration.go, 8 tests)
- `pkg/transport/cascade.go` — gRPC-first cascade with HTTP fallback
- `pkg/health/aggregator.go` — Akira health check framework (exec checks for cert monitoring)
- `runbooks/pki/generate-kingdom-certs.yaml` — CA bootstrap + 14 service certs
- `runbooks/pki/rotate-service-cert.yaml` — zero-downtime per-service rotation
- `runbooks/pki/akira-cert-expiry-check.yaml` — automated expiry monitoring + rotation trigger
- ADR-015 — Go-Fiber HTTP Layer (REST fallback framework)
- ADR-008 — Security Hardening Baseline
- ADR-029 — Wotan Consensus Health (Akira)
