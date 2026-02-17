# ADR-008: Security Hardening Baseline

## Status: Accepted

## Date: 2026-02-16

## Context

The February 9 security audit identified several areas requiring hardening before the platform moves from alpha to beta. While the architecture enforces zero customer data access by design (ADR-007 covers container isolation), the service-to-service communication and external-facing endpoints lacked authentication, input validation, and secrets externalization.

### Threat Model

1. **Unauthenticated access**: External actors reaching internal service APIs
2. **Man-in-the-middle**: Network interception between services on the mesh
3. **Cross-origin abuse**: Browser-based attacks via permissive CORS
4. **Input injection**: Malformed payloads causing unexpected behavior
5. **Credential leakage**: Secrets in source code, logs, or environment dumps

## Decision

We adopt a layered security baseline addressing all five threat categories:

### Layer 1: Authentication — API Key Middleware

**Implementation:** `services/gateway/middleware/auth.go`

- All service endpoints require an `X-API-Key` header
- API keys stored in environment variables (`BUSBOY_API_KEY`, etc.), never in source
- Keys validated per-request with map-based lookup
- Returns 401 Unauthorized when key missing, 403 Forbidden when invalid
- Admin endpoints use a separate `ADMIN_API_KEY` with elevated privileges
- API key hashing in logs prevents credential exposure

**Future:** Migrate to short-lived JWT tokens with OIDC provider for production.

### Layer 2: Transport Security — mTLS (The Sacred Sigils)

**Implementation:** `pkg/mesh/mtls/`

- Mutual TLS with SPIFFE identity verification between all services
- CA + service certificate generation via `pkg/mesh/mtls/certs.go`
- Server: `tls.Config{ClientAuth: tls.RequireAndVerifyClientCert}`
- Client: Peer certificate validation with SPIFFE ID extraction
- TLS 1.2 minimum enforced (TLS 1.3 preferred)
- Certificate rotation supported via the cert manager

**Key decisions:**
- SPIFFE IDs encode service identity (`spiffe://unheaded.local/service-name`)
- Development mode uses self-signed CA; production will use Vault PKI

### Layer 3: CORS Lockdown

**Implementation:** `services/gateway/middleware/cors.go`

- Replaced wildcard `*` origin with explicit allowed origins list
- Configurable via `--cors-origins` flag
- Default: `http://localhost:8081,http://localhost:8080` (dashboard + kanban)
- Preflight caching: `Access-Control-Max-Age: 86400` (24 hours)
- Only `GET`, `POST`, `PUT`, `DELETE` methods allowed
- Credentials mode requires exact origin match (no wildcards)

### Layer 4: Input Validation

**Implementation:** `pkg/httputil/request.go`

- Content-Type enforcement: `application/json` required on POST/PUT
- Payload size limit: 1MB default (`DefaultMaxBodySize = 1 * 1024 * 1024`)
- Request body read via `io.LimitReader` to prevent memory exhaustion
- Topic name validation: alphanumeric + dots + wildcards only
- Path parameter validation: ID format and length bounds checked

### Layer 5: Container Hardening

**Implementation:** `Dockerfile`, `docker-compose.yml`, `nix/containers/*.nix`

See ADR-007 for the full container hardening strategy. Key additions in this pass:

- `USER unheaded` (UID 1000) on all container stages
- `no-new-privileges: true` security option
- Capability bounding: only `CAP_NET_ADMIN` where needed (eBPF)
- Docker default seccomp profile applied
- Multi-stage builds: only runtime binaries in final image

### Layer 6: Secrets Management

**Implementation:** `pkg/secrets/`

- Pluggable storage backends: memory, file (age-encrypted), Vault
- Envelope encryption with Key Encryption Keys (KEK)
- Lease-based secret access with automatic expiration
- Audit logging of all secret operations
- Secret rotation framework with configurable intervals
- `.env.example` with placeholder values; `.env` in `.gitignore`
- Docker Compose services use `env_file: .env`

**Sacred Law:** Zero customer data in secrets store. Infrastructure credentials only.

## Consequences

### Positive

- All endpoints authenticated — no anonymous access to internal APIs
- Transport encrypted — mTLS prevents interception between services
- CORS restricted — browser-based attacks limited to known origins
- Input validated — malformed payloads rejected at the edge
- Secrets externalized — no credentials in source code or version control

### Negative

- Development friction: developers need API keys and certificates for local testing
- Complexity: mTLS certificate management adds operational overhead
- Performance: TLS handshake adds ~1ms to initial connections (amortized via connection pooling)

### Mitigations

- Development mode: self-signed CA auto-generated on first run
- Certificate rotation automated via cert manager
- Connection pooling for gRPC and HTTP clients reduces TLS overhead

## Compliance

This baseline addresses the February 9 audit findings:

| Finding | Status | ADR Section |
|---------|--------|-------------|
| No authentication on APIs | RESOLVED | Layer 1 |
| Plaintext service communication | RESOLVED | Layer 2 |
| Wildcard CORS policy | RESOLVED | Layer 3 |
| No input validation | RESOLVED | Layer 4 |
| Hardcoded secrets | RESOLVED | Layer 6 |
| Container runs as root | RESOLVED | Layer 5 |

## References

- ADR-007: Container Hardening Strategy
- OWASP Top 10 (2021)
- SPIFFE/SPIRE specification: https://spiffe.io/
- NixOS Security Hardening: https://nixos.wiki/wiki/Security
