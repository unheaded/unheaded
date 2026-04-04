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

**Aggressive defaults to prove the automation.** If daily rotation works flawlessly,
relaxing to 90-day is trivial. If it breaks, we find out immediately — not 89 days
from now when the first cert expires and takes down prod at 3am.

All lifetimes are configurable via `/var/lib/unheaded/pki/pki.conf`:

```ini
[lifetimes]
ca_days = 3
service_days = 1
host_days = 1

[rotation]
# Trigger rotation when this fraction of lifetime remains
trigger_ratio = 0.33
# Akira check interval
check_interval = 1h
```

**Default (aggressive — proving the automation):**

| Level | Certificate | Purpose | Lifetime | Rotation Trigger |
|-------|------------|---------|----------|-----------------|
| Root CA | `ca.crt` / `ca.key` | Signs all service + host certs | **3 days** | Akira auto-rotates at 2 days (2/3 lifetime) |
| Service | `<svc>.crt` / `<svc>.key` | Per-service identity | **1 day** | Akira auto-rotates at 16 hours (2/3 lifetime) |
| Node | `<host>.crt` / `<host>.key` | Per-host identity (Akira, daemon) | **1 day** | Akira auto-rotates at 16 hours |

**Production (once automation is proven stable):**

| Level | Certificate | Lifetime | Rotation Trigger |
|-------|------------|----------|-----------------|
| Root CA | `ca.crt` / `ca.key` | 90 days | 60 days |
| Service | `<svc>.crt` / `<svc>.key` | 7 days | ~4.5 days |
| Node | `<host>.crt` / `<host>.key` | 7 days | ~4.5 days |

**Why start at 1-day/3-day:** The entire point is to exercise the rotation
pipeline continuously. Every failure mode surfaces within 72 hours instead of
hiding for months. This is how you build confidence in automation — run it hot.

**Akira cert health check:** Akira runs an exec check every hour that reads each
cert's `Not After` date. Rotation triggers at 2/3 lifetime remaining.
If rotation fails, Akira escalates via standard consensus thresholds:
- 50% lifetime remaining: retry rotation, log WARN
- 25% lifetime remaining: ERROR, publish to system.outage.reports
- 10% lifetime remaining: CRITICAL — consensus alert
- 0%: expired — mTLS handshakes fail, service isolated

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

**Rotation timeline (1-day service cert, aggressive defaults):**
```
Hour 0:  Cert issued (valid 24 hours)
Hour 16: Akira triggers rotation → new cert issued (valid 24 hours)
Hour 24: Old cert expires (but was replaced 8 hours ago)
```

**Rotation timeline (3-day CA, aggressive defaults):**
```
Day 0: CA issued (valid 3 days)
Day 2: Akira triggers CA rotation → new CA issued, cross-signs old
Day 2: All service certs re-issued under new CA (batch)
Day 3: Old CA expires (replaced 1 day ago)
```

**Failure escalation (percentage-based, scales with any lifetime):**
```
33% remaining: Akira triggers rotation (normal lifecycle)
50% consumed without rotation: WARN — retry, log
75% consumed without rotation: ERROR — publish to system.outage.reports
90% consumed without rotation: CRITICAL — consensus alert
100% consumed: expired — mTLS handshakes fail, service isolated
```

**CA rotation (fully automated via Akira):**
1. Akira detects CA at 2/3 lifetime → triggers CA rotation runbook
2. Generate new CA on WEST (overlapping validity with old CA)
3. Cross-sign: new CA signs old CA's cert (trust chain continuity)
4. Distribute new CA cert to all nodes (add to trust bundle)
5. Batch re-issue all service certs signed by new CA
6. CertRotator hot-reloads on each service — zero downtime
7. Remove old CA from trust bundle after all certs re-issued
8. Age-encrypt new CA key, archive old

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

**BlackMage (crypto):** Aggressive short-lived certs are cryptographically sound —
the shorter the validity window, the less useful a stolen key. 1-day service certs
mean a compromised key is worthless within 24 hours without revocation infrastructure.
The CA key is age-encrypted at rest, only decrypted during automated signing.
Ed25519 or ECDSA P-384 preferred for new deployments long-term; RSA-4096 acceptable now.

**MoatGhost (perimeter):** 1-day service certs with hourly monitoring = max 24-hour
blast radius from key compromise, realistically ~8 hours (rotation at 16h mark).
This is better than most production systems achieve with CRL/OCSP. The aggressive
defaults also continuously exercise the alerting pipeline — every cert rotation
generates Akira health events, Wotan topic publishes, and consensus evaluations.
This means the monitoring infra is tested daily, not just when something breaks.

**Sentinel (ops security):** The aggressive defaults are the right call. Every
cloud provider postmortem about expired certs boils down to "rotation was manual
and someone forgot." By running 1-day certs from day one, we guarantee the
automation works before we depend on it. The hourly Akira check interval means
worst case 1 hour between detection and rotation trigger — well within the
8-hour buffer before escalation. All lifetimes configurable via pki.conf for
production tuning once confidence is established.

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
