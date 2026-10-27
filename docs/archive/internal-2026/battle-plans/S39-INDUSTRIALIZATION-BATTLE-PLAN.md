# S39 INDUSTRIALIZATION BATTLE PLAN
## Unheaded v0.1.0-alpha Ship-Ready Campaign

**Status**: EPIC SPRINT (320-380 steps)
**Forged by**: The Warmonger
**Date**: 2026-02-24
**Repo**: ~/tmp/unheaded (Go 1.24 + Rust + eBPF)
**Production LOC**: ~260K, ~464K with tests, 20+ service binaries

---

## LEGEND

| Symbol | Meaning |
|--------|---------|
| [B] | Battle Objective (major milestone) |
| [V] | Victory Condition (gates, metrics, verification) |
| [D] | Deployment/Deliverable |
| [W] | Warmonger Review Point |
| [R] | Race Detection / Concurrency Test |
| [S] | Security Test / Audit |
| [P] | Performance / Load Test |
| [C] | Commit & Code Review |

---

## PREREQUISITE VALIDATION

**Assumptions at S39 Start**:
- S36 (Four Pillars) COMPLETE: Doom Range ports, gRPC-first, log aggregation, service discovery
- S37 (LICENSE+SBOM) COMPLETE: BSL 1.1, SBOM scanned, compliance framework
- S38 (eBPF Production) COMPLETE: All eBPF programs in production, metrics flowing
- Wotan on gRPC 18001, HTTP 18000
- Dashboard on 20000, Gateway on 21000/21443
- pkg/auth/ skeleton: JWT + API key implemented but NOT wired to all services
- pkg/transport/ exists: gRPC-first with cascade
- pkg/logagg/ exists: Centralized logging operational
- pkg/discovery/ exists: Three-layer service discovery operational
- Sacred Laws enforceable: ZERO customer data access, security first, TDD, race detection always

**If any assumption violated**: HALT. Roll back to S38 completion check.

---

## PHASE DEPENDENCY GRAPH

```
Phase 0 (Verification) → Sequential Gate
                ↓
Phase 1 (Auth Hardening) ────────┐
                                  ├→ Phase 3 (Lich D1-D6)
Phase 2 (mTLS Service Mesh) ─────┤    ↓
                                  ├→ Phase 4 (Lich Remediation)
Phase 5 (Wotan Hardening) ────────┤    ↓
                                  ├→ Phase 6 (Container Security)
Phase 7 (E2E Integration) ────────┤
                                  ├→ Phase 8 (Deployment Pipeline)
                                  ↓
Phase 9 (Documentation) ──────────→
                                  ↓
Phase 10 (Ship Gate) ──────────────
```

**Parallelization Strategy**:
- Phases 1, 2, 5, 7 can run in parallel (different services/layers)
- Phases 3-4 must be sequential (test → remediate)
- Phase 6 begins once Phase 1 complete (auth + container security linked)
- Phase 8 begins once Phases 1-7 complete
- Phase 9 ongoing parallel with all phases
- Phase 10 final sequential gate

---

## PHASE 0: ENVIRONMENT VERIFICATION + S38 VALIDATION

**Objective**: Confirm all S36-S38 prerequisites met, baseline metrics captured.

### 0.1 [B] Verify S38 eBPF Production Status
**Time**: 15 min | **Agent**: A

```bash
# In ~/tmp/unheaded
make verify-ebpf-production
# Should show: all eBPF programs compiled, loaded, metrics flowing
# Check: /var/log/unheaded/ebpf-metrics.log has <60s old entries
```

**[V] Victory**:
- All eBPF programs loaded (`cat /proc/kallsyms | grep unheaded`)
- Metrics streaming to Wotan (verify via dashboard)
- Zero segfaults in dmesg (last 1h)
- Performance baseline: <100µs kernel entry latency

---

### 0.2 [B] Baseline Metrics Capture
**Time**: 10 min | **Agent**: A

```bash
# Capture baseline before any changes
mkdir -p /tmp/s39-baseline
docker stats --no-stream > /tmp/s39-baseline/container-baseline.txt
go tool pprof -http=:6060 http://localhost:6060/debug/pprof/profile?seconds=5 &
# Services health check
curl -s http://localhost:21000/health | jq .
curl -s http://localhost:21443/health | jq .
```

**[V] Victory**:
- Baseline CPU/memory captured
- All 20+ service binaries running
- Gateway (21000/21443) responding
- Dashboard (20000) operational

---

### 0.3 [C] Commit Checkpoint: S38 Validation Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.0.3: S38 validation baseline captured"
```

---

### 0.4 [B] Verify pkg/auth/ Skeleton
**Time**: 10 min | **Agent**: B

```bash
# Check JWT implementation
grep -r "jwt.Sign\|jwt.Verify" pkg/auth/ --include="*.go"
# Check API key impl
grep -r "ValidateAPIKey" pkg/auth/ --include="*.go"
# Confirm NOT wired to all services yet
grep -r "pkg/auth" cmd/ --include="*.go" | wc -l
# Should be very few (0-3 references only)
```

**[V] Victory**:
- JWT signing/verification logic present
- API key validation logic present
- RBAC skeleton exists (pkg/auth/rbac.go or similar)
- Auth NOT yet integrated into service middleware

---

### 0.5 [B] Verify pkg/transport/ gRPC-First Cascade
**Time**: 10 min | **Agent**: B

```bash
# Confirm all services have gRPC listeners
grep -r "grpc.NewServer\|grpc.Dial" cmd/ --include="*.go" | wc -l
# Verify HTTP cascade exists
grep -r "http.Server\|http.Handler" pkg/transport/ --include="*.go" | wc -l
```

**[V] Victory**:
- All services have gRPC server instantiation
- HTTP cascade configured in pkg/transport/
- Port registry matches (18000/18001, 21000/21443, 20000)

---

### 0.6 [B] Verify pkg/logagg/ Centralized Logging
**Time**: 10 min | **Agent**: B

```bash
# Check centralized log sink
curl -s http://localhost:18001/debug/logs | head -20
# Verify all services sending logs
grep -r "logagg.Send\|logagg.Log" cmd/ --include="*.go" | wc -l
```

**[V] Victory**:
- Centralized log aggregation receiving logs
- All service logs timestamped, structured
- Zero missing services in log stream

---

### 0.7 [B] Verify pkg/discovery/ Three-Layer Service Discovery
**Time**: 15 min | **Agent**: B

```bash
# Layer 1: DNS
nslookup wotan.services.internal
# Layer 2: Consul (if running)
curl -s http://localhost:8500/v1/catalog/services | jq .
# Layer 3: Direct registry
curl -s http://localhost:18001/debug/services | jq .
```

**[V] Victory**:
- DNS resolution working for all services
- Service discovery returning all 20+ services
- Health check endpoints responding (<100ms)

---

### 0.8 [C] Commit Checkpoint: Phase 0 Validation Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.0.8: Phase 0 prerequisites validation complete"
```

---

## PHASE 1: AUTH HARDENING - JWT + API KEY + SERVICE TOKENS ON ALL ENDPOINTS

**Objective**: Wire authentication to every endpoint, implement RBAC, issue service tokens.

### 1.1 [B] Audit All Endpoints for Auth Coverage
**Time**: 45 min | **Agent**: C

```bash
# Find all gRPC service definitions
find cmd/ -name "*.proto" -exec grep -h "rpc " {} \; | wc -l
# Find all HTTP handlers
grep -r "http.HandleFunc\|gin.GET\|gin.POST" cmd/ --include="*.go" | wc -l
# Create audit matrix: endpoint → auth method → status
# Output: /tmp/s39-auth-audit.csv
# Columns: Service,Endpoint,CurrentAuth,RequiredAuth,Status
```

**[V] Victory**:
- Complete inventory of all endpoints (gRPC + HTTP)
- Auth coverage matrix: 0% baseline
- No endpoints should have auth yet (wiring begins next)

---

### 1.2 [B] Implement JWT Middleware for All Services
**Time**: 90 min | **Agents**: C, D

```bash
# Create middleware/jwt.go in each service
# Template:
# - Extract JWT from Authorization header or gRPC metadata
# - Verify signature against public key
# - Extract claims (sub, aud, iat, exp)
# - Inject claims into request context
# - Return 401 for invalid/expired tokens

# For gRPC: implement unary interceptor
# For HTTP: implement gin middleware

# Services to update (in parallel):
# - cmd/gateway (HTTP)
# - cmd/wotan (gRPC)
# - cmd/dashboard (HTTP)
# - cmd/auth-service
# - cmd/threat-engine
# - cmd/vector-service
# - cmd/observation-engine
# - [all 20+ service binaries]
```

**[V] Victory**:
- All services have JWT middleware
- Middleware passes JWT claims in context
- No requests bypass middleware
- JWT validation <5ms latency

**[R] Race Detection**:
```bash
go test -race ./... -run TestJWTMiddleware
```

---

### 1.3 [B] Implement API Key Validation on All Endpoints
**Time**: 60 min | **Agents**: C, D

```bash
# Create pkg/auth/apikey_validator.go
# - Load API keys from config
# - Validate X-API-Key header (HTTP) / metadata (gRPC)
# - Store API key hashes (not plaintext)
# - Track API key usage (for audit log)
# - Implement rotation mechanism

# Wire to all services
# Priority order:
# 1. Gateway (21000/21443)
# 2. Dashboard (20000)
# 3. All internal services
```

**[V] Victory**:
- All endpoints accept X-API-Key header
- API key validation <5ms
- No plaintext API keys in logs
- API key rotation implemented

---

### 1.4 [B] Implement Service Token (mTLS Pre-Auth)
**Time**: 60 min | **Agents**: C, D

```bash
# Create pkg/auth/service_token.go
# - Each service gets unique service token (ephemeral, JWT-like)
# - Service token signed by control plane
# - Service token includes service name, version, capabilities
# - Tokens rotated every 24h (or on deployment)
# - Tokens used for service-to-service gRPC calls

# Implementation:
# - Service identity: /etc/unheaded/service-identity.json
# - Token stored in memory, never persisted
# - Token refresh on startup + hourly
```

**[V] Victory**:
- All services have unique service token
- Service tokens expire after 24h
- Service tokens used in gRPC metadata
- Zero service token collisions

---

### 1.5 [B] Implement RBAC (Role-Based Access Control)
**Time**: 120 min | **Agents**: D, E

```bash
# Create pkg/auth/rbac.go
# Role definitions:
# - admin: all operations
# - operator: deploy, scale, restart (no secrets)
# - observer: read-only (logs, metrics, state)
# - service: service-to-service operations
# - guest: public endpoints only (health, status)

# Role assignments per endpoint:
# - Gateway admin endpoints: admin role
# - Deployment/config endpoints: operator role
# - Metrics/logs endpoints: observer role
# - Service RPC endpoints: service role
# - Public endpoints: guest role (no auth required)

# RBAC middleware:
# - Extract user/service role from JWT claims
# - Check endpoint requires_role
# - Return 403 for insufficient permissions
# - Log all RBAC denials to audit trail
```

**[V] Victory**:
- 5 roles defined and documented
- All endpoints have role requirements
- RBAC middleware <2ms latency
- Zero unauthorized access allowed

**[S] Security Test**:
```bash
# Attempt guest user accessing admin endpoint → 403
# Attempt observer accessing operator endpoint → 403
# Attempt valid role accessing valid endpoint → 200
```

---

### 1.6 [B] Wire Auth to Gateway (21000/21443)
**Time**: 90 min | **Agent**: E

```bash
# cmd/gateway/main.go updates:
# 1. Load JWT public key from /etc/unheaded/jwt-public.key
# 2. Load API keys from /etc/unheaded/api-keys.json
# 3. Add JWT middleware to all routes except /health, /status
# 4. Add API key middleware to all routes except /health, /status
# 5. Add RBAC check middleware
# 6. Inject claims into downstream gRPC calls

# Gateway routes requiring auth:
# - POST /api/v1/threats
# - GET /api/v1/threats/:id
# - DELETE /api/v1/threats/:id
# - POST /api/v1/deploy
# - GET /api/v1/status
# - [all non-health endpoints]

# Public endpoints (no auth):
# - GET /health
# - GET /status
```

**[V] Victory**:
- Gateway validates JWT on all protected endpoints
- Gateway validates API key on all protected endpoints
- Gateway enforces RBAC
- Gateway passes auth context to downstream services
- Public endpoints accessible without auth
- Auth latency <10ms per request

**[R] Race Detection**:
```bash
go test -race ./cmd/gateway -run TestAuthMiddleware
```

---

### 1.7 [B] Wire Auth to Dashboard (20000)
**Time**: 60 min | **Agent**: E

```bash
# cmd/dashboard/main.go updates:
# 1. Require JWT on all endpoints except /health, /login
# 2. Implement /login endpoint: accept API key, return JWT
# 3. JWT valid for 8h, includes user info and role
# 4. Add RBAC checks for admin-only dashboard functions
# 5. Log all dashboard access to audit trail

# Dashboard endpoints:
# - GET /health (public)
# - POST /login (api_key) → JWT
# - GET /metrics (jwt required)
# - GET /logs (jwt required)
# - POST /deploy (jwt + admin role)
# - [all other endpoints] (jwt required)
```

**[V] Victory**:
- Dashboard login endpoint functional
- JWT issued for valid API key
- All dashboard endpoints require JWT
- Dashboard RBAC enforced
- Session timeout after 8h inactivity

---

### 1.8 [B] Wire Auth to Wotan (18000/18001)
**Time**: 60 min | **Agent**: E

```bash
# cmd/wotan/main.go updates:
# 1. Add JWT interceptor to gRPC server
# 2. Add API key interceptor to gRPC server
# 3. Verify service token for service-to-service calls
# 4. Add RBAC check for message routing endpoints
# 5. Allow unauthenticated access only to health checks

# Wotan endpoints requiring auth:
# - All gRPC services (except Health)
# - All HTTP proxy endpoints
```

**[V] Victory**:
- Wotan requires auth on all RPC endpoints
- Service tokens accepted and verified
- Wotan propagates auth context downstream
- Zero unauthorized messages delivered

---

### 1.9 [B] Wire Auth to Threat Engine, Vector Service, Observation Engine
**Time**: 120 min | **Agents**: D, E, F

```bash
# For each service:
# cmd/threat-engine/main.go
# cmd/vector-service/main.go
# cmd/observation-engine/main.go
# cmd/auth-service/main.go
# [all remaining internal services]

# Template:
# 1. Add JWT/service token validation interceptor
# 2. Add RBAC middleware
# 3. Verify claims match service expectations
# 4. Reject requests from unauthorized services
# 5. Log auth events

# Key: These are internal services, so:
# - Primarily accept service tokens (not user JWTs)
# - Health endpoints public
# - All RPC endpoints require service token
```

**[V] Victory**:
- All 20+ services have auth middleware
- No service accepts unauthenticated RPC calls
- Service tokens required for service-to-service
- Auth enforcement consistent across all services

**[R] Race Detection**:
```bash
for svc in cmd/*; do
  go test -race "./$svc" -run TestAuth
done
```

---

### 1.10 [B] Public Key Distribution + API Key Management
**Time**: 45 min | **Agent**: F

```bash
# Create pkg/auth/key_distribution.go
# 1. JWT public key served via /debug/jwt-public-key
# 2. Public key cached locally in all services
# 3. Key rotation: new key issued, old key valid for 30 days
# 4. API keys stored in secure vault:
#    - Development: /etc/unheaded/api-keys.json (git-ignored)
#    - Production: HashiCorp Vault integration
# 5. API key rotation: issue new, disable old, audit trail

# Implementation:
# - pkg/auth/key_distribution.go handles distribution
# - Each service polls /debug/jwt-public-key on startup + every 24h
# - API keys hashed using bcrypt (cost 12)
# - API key usage logged (service, timestamp, endpoint, result)
```

**[V] Victory**:
- JWT public key available for all services
- API keys stored securely (hashed, never plaintext)
- API key rotation mechanism working
- Key distribution <5ms latency

---

### 1.11 [C] Commit Checkpoint: Phase 1a Auth Wiring Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.1.11: Auth wiring to all endpoints complete (JWT + API key + service tokens)"
```

---

### 1.12 [B] Test Auth End-to-End: Single Request Flow
**Time**: 45 min | **Agent**: F

```bash
# Test 1: User → Gateway → Wotan → Internal Service
# 1. Generate JWT for user
# 2. Send request to Gateway with JWT
# 3. Verify Gateway validates JWT
# 4. Verify Gateway creates service token
# 5. Verify Wotan receives service token
# 6. Verify internal service receives and validates token
# 7. Verify response flows back through all layers

# Test 2: Dashboard Login
# 1. POST /login with API key
# 2. Verify JWT returned
# 3. Verify JWT valid for 8h
# 4. Use JWT to access protected endpoints

# Test 3: Service-to-Service
# 1. Verify service token issued to each service
# 2. Verify service can call another service with token
# 3. Verify invalid token rejected
```

**[V] Victory**:
- Auth chain: user JWT → gateway → service token → internal RPC
- Dashboard login flow working
- All RBAC checks passing
- Zero auth failures for valid credentials
- <10ms auth latency per request

**[R] Race Detection**:
```bash
# Concurrent auth requests
ab -n 10000 -c 100 -H "Authorization: Bearer $JWT" http://localhost:21000/api/v1/status
# Should see zero race conditions in race detector
```

---

### 1.13 [B] Auth Configuration + Secrets Management
**Time**: 60 min | **Agent**: F

```bash
# Create /etc/unheaded/auth-config.json
# {
#   "jwt": {
#     "issuer": "unheaded.local",
#     "audience": "unheaded-services",
#     "expiry_hours": 8,
#     "key_rotation_days": 30,
#     "public_key_path": "/etc/unheaded/jwt-public.key"
#   },
#   "api_keys": {
#     "vault_enabled": false,  // true in production
#     "local_path": "/etc/unheaded/api-keys.json",
#     "rotation_days": 90
#   },
#   "rbac": {
#     "enforce": true,
#     "audit_log_path": "/var/log/unheaded/auth-audit.log"
#   }
# }

# Secrets init script:
# - Generate JWT keypair (if not exists)
# - Initialize API keys file (if not exists)
# - Set correct permissions (0600 for all secrets)
# - Verify readability by service accounts
```

**[V] Victory**:
- Auth config properly documented
- JWT keypair generated
- API keys initialized
- Secrets have correct permissions (0600)
- All services can read required secrets

---

### 1.14 [B] Audit Trail: Log All Auth Events
**Time**: 45 min | **Agent**: F

```bash
# Create pkg/auth/audit.go
# Log all auth events:
# - JWT validation (success/failure)
# - API key validation (success/failure)
# - Service token validation (success/failure)
# - RBAC check (pass/fail)
# - Unauthorized access attempts (with context)

# Audit log format:
# {
#   "timestamp": "2026-02-24T12:34:56Z",
#   "event_type": "jwt_validation",
#   "result": "success|failure",
#   "user": "user@example.com",
#   "service": "gateway",
#   "endpoint": "/api/v1/threats",
#   "ip": "10.0.0.1",
#   "reason": "expired token" | null
# }

# Log destination: /var/log/unheaded/auth-audit.log
# Retention: 90 days
# Analysis: grep for failures, unauthorized attempts, anomalies
```

**[V] Victory**:
- All auth events logged in structured format
- Audit log includes success and failure
- Log includes enough context for forensics
- Audit log retained for 90 days
- Zero auth events missing from audit trail

---

### 1.15 [C] Commit Checkpoint: Phase 1 Auth Hardening Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.1.15: Phase 1 complete - JWT, API key, service tokens, RBAC, audit trail wired to all endpoints"
```

---

## PHASE 2: mTLS SERVICE MESH - ZERO PLAINTEXT BETWEEN SERVICES

**Objective**: Implement mutual TLS between all services, eliminate HTTP fallback for sensitive channels.

### 2.1 [B] Generate Root CA Certificate
**Time**: 30 min | **Agent**: G

```bash
# Create pkg/transport/mTLS/root-ca-init.sh
# Generate root CA (valid 10 years):
openssl genrsa -out /etc/unheaded/pki/root-ca.key 4096
openssl req -new -x509 -days 3650 \
  -key /etc/unheaded/pki/root-ca.key \
  -out /etc/unheaded/pki/root-ca.crt \
  -subj "/C=US/O=Unheaded/CN=Unheaded Root CA"

# Store root CA:
# - Key: /etc/unheaded/pki/root-ca.key (chmod 0400)
# - Cert: /etc/unheaded/pki/root-ca.crt (public)
```

**[V] Victory**:
- Root CA key generated (4096-bit RSA)
- Root CA cert valid 10 years
- Root CA stored in /etc/unheaded/pki/
- Permissions correct (key 0400, cert readable)

---

### 2.2 [B] Generate Service Certificates for All 20+ Services
**Time**: 120 min | **Agents**: G, H

```bash
# For each service, create cert + key:
for svc in gateway wotan dashboard auth-service threat-engine vector-service \
           observation-engine sensor-node policy-engine risk-engine \
           event-aggregator metric-collector response-engine \
           learning-engine compliance-engine audit-service config-service; do

  # Generate service key
  openssl genrsa -out /etc/unheaded/pki/${svc}.key 2048

  # Create CSR (Certificate Signing Request)
  openssl req -new \
    -key /etc/unheaded/pki/${svc}.key \
    -out /etc/unheaded/pki/${svc}.csr \
    -subj "/C=US/O=Unheaded/CN=${svc}.services.internal"

  # Sign with root CA (valid 1 year)
  openssl x509 -req -in /etc/unheaded/pki/${svc}.csr \
    -CA /etc/unheaded/pki/root-ca.crt \
    -CAkey /etc/unheaded/pki/root-ca.key \
    -CAcreateserial -out /etc/unheaded/pki/${svc}.crt \
    -days 365 \
    -sha256 \
    -extfile <(printf "subjectAltName=DNS:${svc}.services.internal,DNS:localhost,IP:127.0.0.1")
done

# Store certs:
# - Service key: /etc/unheaded/pki/${svc}.key (chmod 0400)
# - Service cert: /etc/unheaded/pki/${svc}.crt (readable)
# - Root CA: distributed to all services
```

**[V] Victory**:
- All 20+ services have unique certificates
- All certificates signed by root CA
- All certificates valid 1 year
- All certificates include SAN (Subject Alternative Name)
- All keys have correct permissions (0400)

---

### 2.3 [B] Implement mTLS in gRPC Transport Layer
**Time**: 90 min | **Agents**: G, H

```bash
# Create pkg/transport/mtls/grpc_mtls.go
#
# GRPCMTLSConfig:
# - Load service certificate
# - Load service key
# - Load root CA certificate
# - Create tls.Config with mutual authentication
#
# Server setup:
# creds := credentials.NewTLS(&tls.Config{
#   Certificates: []tls.Certificate{serviceCert},
#   ClientAuth: tls.RequireAndVerifyClientCert,
#   ClientCAs: rootCAPool,
# })
# server := grpc.NewServer(grpc.Creds(creds))
#
# Client setup:
# creds := credentials.NewTLS(&tls.Config{
#   Certificates: []tls.Certificate{serviceCert},
#   RootCAs: rootCAPool,
#   ServerName: "target-service.services.internal",
# })
# conn, _ := grpc.Dial(target, grpc.WithTransportCredentials(creds))

# Implementation:
# - Load certs on service startup
# - Use certrotation library for automatic rotation
# - Gracefully handle cert rotation (0 downtime)
```

**[V] Victory**:
- All gRPC servers require mTLS
- All gRPC clients use mTLS
- No gRPC traffic in plaintext
- Certificate validation <5ms latency
- Zero mTLS handshake failures

**[R] Race Detection**:
```bash
go test -race ./pkg/transport/mtls -run TestGRPCmTLS
```

---

### 2.4 [B] Disable Plaintext gRPC Fallback
**Time**: 30 min | **Agent**: H

```bash
# Update all service main.go files:
# Remove or disable:
# - grpc.NewServer() without credentials
# - grpc.Dial() without TLS
# - HTTP2 without TLS
# - Any protocol fallback mechanism

# Verification:
# - netstat should show only TLS ports (18001, etc.)
# - tcpdump should show TLS handshakes, not plaintext
# - Attempting plaintext connection should fail immediately
```

**[V] Victory**:
- Zero plaintext gRPC ports listening
- All gRPC traffic TLS-encrypted
- netstat shows only secure ports
- Plaintext connection attempts rejected

**[S] Security Test**:
```bash
# Attempt plaintext gRPC connection
grpcurl -plaintext localhost:18001 list 2>&1 | grep -i "tls\|certificate"
# Should fail with TLS error
```

---

### 2.5 [B] HTTP → HTTPS Migration on Gateway
**Time**: 60 min | **Agent**: H

```bash
# cmd/gateway/main.go:
# 1. Load TLS cert + key
# 2. Start HTTPS server on 21443
# 3. Start HTTP server on 21000 (redirect only)
# 4. Redirect all HTTP to HTTPS

# HTTP server (21000):
# router.RedirectTrailingSlash = true
# http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
#   https_url := "https://" + strings.TrimPrefix(r.Host, "...") + r.RequestURI
#   http.Redirect(w, r, https_url, http.StatusMovedPermanently)
# })

# HTTPS server (21443):
# Listen on TLS socket
# Serve all application routes
# Enforce HTTPS Strict-Transport-Security header
```

**[V] Victory**:
- Gateway HTTPS on 21443 with TLS cert
- Gateway HTTP on 21000 redirects to HTTPS
- All requests to 21000 redirected (302 response)
- HTTPS Strict-Transport-Security header present
- Zero plaintext HTTP on 21443

---

### 2.6 [B] HTTP → HTTPS Migration on Dashboard
**Time**: 45 min | **Agent**: H

```bash
# Similar to gateway:
# - Dashboard HTTPS on port 20443
# - Dashboard HTTP on 20000 (redirect only)
# - HSTS header on all responses
# - Update service discovery entries

# Note: Dashboard may be admin-only, so redirect is acceptable
# But consider: should it only listen on 20443?
# Decision: Keep HTTP for backward compat, but all traffic redirected
```

**[V] Victory**:
- Dashboard HTTPS on 20443
- Dashboard HTTP on 20000 redirects
- No session data transmitted over plaintext

---

### 2.7 [B] Certificate Rotation: Automated Renewal
**Time**: 90 min | **Agents**: G, H

```bash
# Create pkg/transport/mtls/cert_rotation.go
#
# CertRotation manager:
# - Monitor cert expiry (alert if <30 days)
# - Issue new cert when <30 days to expiry
# - Rotate in-memory cert (0 downtime)
# - Keep old cert valid for 7 days (grace period)
# - Log all rotations to audit trail
#
# Rotation flow:
# 1. Check cert expiry every 24h
# 2. If <30 days: issue new cert
# 3. Call root CA to sign new CSR
# 4. Write new cert to disk
# 5. Reload cert in memory
# 6. Signal gRPC server to use new cert
# 7. No connection drops
#
# Implementation:
# - Use github.com/cloudflare/certmgr or custom impl
# - Store previous 3 certs (for grace period)
# - Verify new cert validity before activating
```

**[V] Victory**:
- Certs rotated automatically
- No manual cert renewal needed
- 0 downtime during rotation
- Old certs remain valid for 7 days
- Zero failed handshakes during rotation

---

### 2.8 [B] mTLS Verification: All Service Pairs
**Time**: 120 min | **Agents**: G, H

```bash
# Create test_mtls_matrix.sh
# Test every service-to-service pair:
#
# For each (service_a, service_b) pair:
# 1. Service A calls Service B via gRPC
# 2. Verify TLS handshake succeeds
# 3. Verify certs exchanged and validated
# 4. Verify plaintext connection rejected
# 5. Verify client cert verified
# 6. Verify server cert verified
#
# Matrix size: 20 services × 20 = 400 pairs
# Test result: /tmp/s39-mtls-matrix.csv
#
# Sample test:
# grpcurl -d '{}' \
#   -cacert /etc/unheaded/pki/root-ca.crt \
#   -cert /etc/unheaded/pki/service-a.crt \
#   -key /etc/unheaded/pki/service-a.key \
#   service-b.services.internal:18001 \
#   unheaded.v1.ServiceB/Health
```

**[V] Victory**:
- All 400 service pairs tested
- 100% TLS handshake success rate
- All certs validated correctly
- Zero plaintext connections accepted
- <100ms average handshake time

---

### 2.9 [C] Commit Checkpoint: Phase 2 mTLS Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.2.9: Phase 2 complete - mTLS service mesh, zero plaintext, cert rotation automated"
```

---

## PHASE 3: LICH CAMPAIGNS D1-D6 - OFFENSIVE SECURITY TESTS

**Objective**: Run 6 offensive security campaigns against full stack, document all findings.

### 3.1 [B] Setup Lich Security Test Framework
**Time**: 60 min | **Agent**: C

```bash
# Create cmd/lich-security/ (if not exists)
# Lich: offensive security test suite
#
# Directory structure:
# cmd/lich-security/
#   ├── main.go
#   ├── campaigns/
#   │   ├── d1_auth_bypass.go
#   │   ├── d2_injection.go
#   │   ├── d3_transport_security.go
#   │   ├── d4_secrets.go
#   │   ├── d5_privilege_escalation.go
#   │   └── d6_denial_of_service.go
#   └── report.go
#
# Each campaign:
# - Tests specific attack vector
# - Returns pass/fail status
# - Documents findings
# - Provides remediation guidance
#
# Execution:
# make lich-d1, make lich-d2, ..., make lich-d6
# make lich-all (runs all campaigns, generates report)
```

**[V] Victory**:
- Lich framework compiles and runs
- All 6 campaign modules present
- Campaign runner works
- Reports generated in /tmp/lich-reports/

---

### 3.2 [B] [S] CAMPAIGN D1: Auth Bypass Tests
**Time**: 90 min | **Agent**: C

**Tests**:
1. Missing auth header → 401
2. Malformed JWT → 401
3. Expired JWT → 401
4. JWT signed with wrong key → 401
5. Modified JWT claims → 401
6. Empty API key → 401
7. Invalid API key → 401
8. Missing service token (service-to-service) → 401
9. Cross-service token reuse (token from service A used by service B) → 401
10. Replay attack (reuse old JWT) → 401

```bash
# cmd/lich-security/campaigns/d1_auth_bypass.go
func TestD1_AuthBypass(ctx context.Context) Report {
  // Test each endpoint without auth
  // Verify 401 response
  // Test with malformed tokens
  // Test with expired tokens
  // Test cross-service token attacks
  // Return: 0 findings = pass
}
```

**[V] Victory**:
- Zero auth bypass vulnerabilities found
- All endpoints reject unauthenticated requests
- All token validation strict
- D1 report: PASS (0 findings)

---

### 3.3 [B] [S] CAMPAIGN D2: Injection Attacks
**Time**: 90 min | **Agent**: D

**Tests**:
1. SQL injection (if any SQL used)
2. gRPC message injection (malformed protobuf)
3. Log injection (newlines in log messages)
4. JSON injection (malformed JSON payloads)
5. Command injection (if shell invoked)
6. LDAP injection (if LDAP used)
7. XML injection (if XML parsed)
8. Format string attacks (printf with user input)

```bash
# cmd/lich-security/campaigns/d2_injection.go
func TestD2_Injection(ctx context.Context) Report {
  // For each endpoint, send malicious payloads
  // - SQL: " OR '1'='1
  // - gRPC: truncated/extended messages
  // - Log: "message\nERROR"
  // - JSON: unclosed braces, null bytes
  // Return: 0 findings = pass
}
```

**[V] Victory**:
- Zero injection vulnerabilities found
- All inputs validated before use
- All outputs properly escaped
- D2 report: PASS (0 findings)

---

### 3.4 [B] [S] CAMPAIGN D3: Transport Security
**Time**: 90 min | **Agent**: D

**Tests**:
1. Plaintext gRPC connections → rejected
2. mTLS with invalid cert → rejected
3. mTLS with untrusted CA → rejected
4. HTTP without HTTPS redirect → checked
5. Weak TLS cipher suites → not offered
6. Old TLS versions (1.0, 1.1) → not offered
7. Man-in-the-middle (MITM) attack simulation
8. Certificate pinning (if implemented)

```bash
# cmd/lich-security/campaigns/d3_transport_security.go
func TestD3_TransportSecurity(ctx context.Context) Report {
  // Attempt plaintext gRPC
  // Attempt invalid mTLS certs
  // Check TLS version (must be 1.2+)
  // Check cipher suites (modern only)
  // Verify HTTPS redirect
  // Return: 0 findings = pass
}
```

**[V] Victory**:
- Zero transport security vulnerabilities
- All connections use TLS 1.2+
- All cipher suites modern (ECDHE, AES-256)
- No plaintext allowed
- D3 report: PASS (0 findings)

---

### 3.5 [B] [S] CAMPAIGN D4: Secrets Management
**Time**: 90 min | **Agent**: E

**Tests**:
1. API keys in environment variables (should be in vault)
2. Secrets in logs (grep for "password", "secret", "key")
3. Secrets in error messages
4. Secrets in debug output
5. Hardcoded secrets in code
6. Secrets in git history
7. Insecure secret storage permissions
8. Secret exposure via side channels (timing attacks)

```bash
# cmd/lich-security/campaigns/d4_secrets.go
func TestD4_Secrets(ctx context.Context) Report {
  // Scan logs for secrets
  // Scan debug output for secrets
  // Check file permissions on secret files
  // Attempt to read secret files as unprivileged user
  // Check git history for exposed secrets
  // Return: 0 findings = pass
}
```

**[V] Victory**:
- Zero hardcoded secrets found
- Zero secrets in logs
- Zero secrets in error messages
- Secret files have correct permissions (0400, 0600)
- D4 report: PASS (0 findings)

---

### 3.6 [B] [S] CAMPAIGN D5: Privilege Escalation
**Time**: 90 min | **Agent**: E

**Tests**:
1. Guest user accessing admin endpoints → 403
2. Observer role accessing operator endpoints → 403
3. Unprivileged service accessing privileged operations
4. RBAC bypass (JWT claim modification)
5. Horizontal privilege escalation (user A accessing user B's data)
6. Vertical privilege escalation (normal user → admin)
7. Race condition in RBAC checks
8. RBAC check bypass (missing check on some endpoint)

```bash
# cmd/lich-security/campaigns/d5_privilege_escalation.go
func TestD5_PrivilegeEscalation(ctx context.Context) Report {
  // For each endpoint requiring auth:
  // - Access with guest token → 403
  // - Access with observer token (if admin needed) → 403
  // - Modify JWT claims, try again → 403
  // - Concurrent requests (race condition) → all 403
  // Return: 0 findings = pass
}
```

**[V] Victory**:
- Zero privilege escalation vulnerabilities
- RBAC enforced on all protected endpoints
- No broken access control
- JWT claims not modifiable
- D5 report: PASS (0 findings)

---

### 3.7 [B] [S] CAMPAIGN D6: Denial of Service (DoS)
**Time**: 120 min | **Agent**: F

**Tests**:
1. Rate limiting (high request rate → 429 Too Many Requests)
2. Large payload attack (1GB POST body)
3. Slow client attack (send headers 1 byte/second)
4. Algorithmic complexity attack (worst-case input)
5. Connection exhaustion (max open connections)
6. Memory exhaustion (allocate large arrays)
7. CPU exhaustion (compute-heavy operations)
8. gRPC streaming exhaustion (infinite stream)

```bash
# cmd/lich-security/campaigns/d6_denial_of_service.go
func TestD6_DenialOfService(ctx context.Context) Report {
  // Rate limit test: 10000 requests/sec → verify 429
  // Payload test: 1GB body → verify rejection
  // Slow client: 1 byte/sec for 10s → verify timeout
  // Connection test: open 100000 connections → verify limit
  // Return: 0 findings = pass (assuming rate limits in place)
}
```

**[V] Victory**:
- Rate limiting working (<100 requests/sec → 429)
- Large payloads rejected (>100MB)
- Slow client timeouts (<30s)
- Connection limits enforced
- CPU/memory bounded
- D6 report: PASS (0 findings)

---

### 3.8 [B] Generate Lich Comprehensive Report
**Time**: 45 min | **Agent**: F

```bash
# cmd/lich-security/report.go
# Generates: /tmp/lich-reports/S39-SECURITY-AUDIT.md
#
# Report sections:
# 1. Executive Summary (findings count per campaign)
# 2. Campaign Results (D1-D6 pass/fail)
# 3. Detailed Findings (all issues found, with CVE severity)
# 4. Remediation Plan (prioritized fixes)
# 5. Timeline (when each fix deployed)
# 6. Sign-Off (Warmonger + Lead Security)
#
# Format: Markdown, structured, ready for compliance review
```

**[V] Victory**:
- Report generated in /tmp/lich-reports/
- All 6 campaigns complete
- All findings documented
- Remediation plan attached
- Ready for Phase 4 (remediation)

---

### 3.9 [C] Commit Checkpoint: Phase 3 Lich Campaigns Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.3.9: Phase 3 complete - Lich D1-D6 campaigns executed, 0 findings (target: fully remediated)"
```

---

## PHASE 4: LICH FINDINGS REMEDIATION

**Objective**: Fix every issue found by Lich (D1-D6), re-test, document.

### 4.1 [B] Analyze Lich Findings Categorically
**Time**: 45 min | **Agent**: C

```bash
# Parse /tmp/lich-reports/S39-SECURITY-AUDIT.md
# Group findings by:
# - Campaign (D1-D6)
# - Severity (critical, high, medium, low)
# - Component (gateway, wotan, service X, etc.)
# - Remediation complexity
#
# Output: /tmp/s39-remediation-plan.csv
# Columns: FindingID, Campaign, Severity, Component, Remediation, Effort, Owner
#
# If all findings are pass → Phase 4 is 5 min (skip to 4.11)
# If findings exist → prioritize critical → high → medium → low
```

**[V] Victory**:
- All findings categorized
- Remediation effort estimated
- Owner assigned to each finding
- Prioritization clear

---

### 4.2-4.10 [B] Remediate Each Finding (Template)
**Time**: 30-120 min per finding | **Agents**: C-H (parallel)

For each finding:
```bash
# 1. Create feature branch
git checkout -b remediate-lich-${finding_id}

# 2. Implement fix (location varies by finding)
# Example: Auth bypass fix
vim cmd/gateway/auth_middleware.go
# Add stricter validation, test fix

# 3. Test locally
make test-lich-d1  # re-run campaign D1

# 4. Verify fix doesn't break anything
make race-test  # race detection
make test       # all tests

# 5. Commit
git add -A && git commit -m "Remediate lich-${finding_id}: [description]"

# 6. Merge to main
git push origin remediate-lich-${finding_id}
# Create PR, review, merge
```

**[V] Victory** (per finding):
- Finding verified fixed
- No new findings introduced
- Tests passing
- Race detection passing
- Committed to main

---

### 4.11 [B] Re-Run All Lich Campaigns D1-D6 (Post-Remediation)
**Time**: 90 min | **Agent**: C

```bash
make lich-all
# Verify all campaigns PASS
# Zero findings acceptable
cat /tmp/lich-reports/S39-SECURITY-AUDIT-POST-REMEDIATION.md
```

**[V] Victory**:
- All Lich campaigns PASS
- Zero findings remaining
- Report generated
- Ready to proceed to Phase 5

---

### 4.12 [C] Commit Checkpoint: Phase 4 Remediation Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.4.12: Phase 4 complete - All Lich findings remediated and re-verified (0 findings)"
```

---

## PHASE 5: WOTAN HARDENING - MESSAGE DELIVERY RELIABILITY

**Objective**: Implement ack/nack, retry, dead letter queues, message ordering.

### 5.1 [B] Design Wotan Message Protocol Enhancement
**Time**: 60 min | **Agent**: G

```bash
# Create docs/wotan-protocol-v2.md
#
# Current (v1): Fire-and-forget gRPC calls
# Enhanced (v2):
# - Ack/Nack: Sender waits for ack or nack
# - Retry: Exponential backoff (1s, 2s, 4s, 8s, max 32s)
# - Dead Letter Queue (DLQ): Failed messages after 5 retries
# - Message Ordering: Per-queue ordering guarantee
# - Idempotency: Duplicate messages rejected (via message ID)
# - Timeout: 30s per message, then retry
#
# Protocol:
# Message {
#   id: uuid,
#   source_service: string,
#   destination_service: string,
#   priority: enum(high, normal, low),
#   created_at: timestamp,
#   payload: bytes,
# }
#
# Ack {
#   message_id: uuid,
#   status: enum(ack, nack),
#   reason: string (if nack),
# }
```

**[V] Victory**:
- Protocol v2 designed and documented
- Backward compatible with v1
- Ready for implementation

---

### 5.2 [B] Implement Ack/Nack in Wotan
**Time**: 90 min | **Agent**: G

```bash
# cmd/wotan/delivery.go (new file)
#
# AckNackManager:
# - Track sent messages
# - Wait for ack/nack from recipient
# - Timeout if no response in 30s
# - Return status to sender
#
# Implementation:
# 1. Sender calls wotan.SendMessage(msg)
# 2. Wotan stores msg in memory (id → msg)
# 3. Wotan forwards to recipient via gRPC
# 4. Recipient processes, calls wotan.Ack(msg_id)
# 5. Wotan notifies sender of ack
# 6. Sender continues
# 7. If nack or timeout → sender initiates retry
#
# Data structure:
# type InFlightMessage struct {
#   ID string
#   Msg []byte
#   SentAt time.Time
#   AckChan chan AckResult
# }
```

**[V] Victory**:
- All messages tracked in flight
- Ack/nack working end-to-end
- Timeout working (30s)
- No lost messages

**[R] Race Detection**:
```bash
go test -race ./cmd/wotan -run TestAckNack
```

---

### 5.3 [B] Implement Retry Logic with Exponential Backoff
**Time**: 60 min | **Agent**: G

```bash
# cmd/wotan/retry.go (new file)
#
# RetryManager:
# - Track failed messages
# - Implement exponential backoff: 1s, 2s, 4s, 8s, 16s, 32s
# - Max 5 retries
# - Jitter: add random 0-1s to backoff
# - Log all retries
#
# Implementation:
# type RetryQueue struct {
#   Queue []InFlightMessage
#   RetryCount map[string]int
# }
#
# func (r *RetryQueue) Retry(msg InFlightMessage) {
#   retries := r.RetryCount[msg.ID]
#   if retries >= 5 {
#     SendToDLQ(msg)
#     return
#   }
#   backoff := exponentialBackoff(retries)
#   time.Sleep(backoff)
#   SendMessage(msg)
#   r.RetryCount[msg.ID]++
# }
```

**[V] Victory**:
- Exponential backoff working
- Max 5 retries enforced
- Jitter working (random delay)
- Messages don't retry beyond 5x

---

### 5.4 [B] Implement Dead Letter Queue (DLQ)
**Time**: 60 min | **Agent**: H

```bash
# cmd/wotan/dlq.go (new file)
#
# DeadLetterQueue:
# - Store messages that failed 5 retries
# - Persist DLQ to disk (durable)
# - Expose DLQ via HTTP/gRPC for inspection
# - Alert on DLQ growth (>100 messages)
# - Cleanup: keep DLQ messages for 30 days
#
# Implementation:
# type DLQEntry struct {
#   Message []byte
#   FailedAt time.Time
#   RetryCount int
#   LastError string
# }
#
# Storage: /var/spool/unheaded/wotan-dlq/
# Format: one file per message (date-based rotation)
```

**[V] Victory**:
- Failed messages go to DLQ after 5 retries
- DLQ persistent (survives restart)
- DLQ queryable via API
- DLQ cleanup working (30 day retention)

---

### 5.5 [B] Implement Message Ordering (Per-Queue)
**Time**: 90 min | **Agent**: H

```bash
# cmd/wotan/ordering.go (new file)
#
# MessageOrdering:
# - Each destination service has a queue
# - Messages delivered in order (FIFO)
# - Ordering enforced per queue, not globally
# - If message A fails, message B waits
#
# Implementation:
# type OrderedQueue struct {
#   DestinationService string
#   Queue []InFlightMessage
#   Mu sync.Mutex
# }
#
# func (q *OrderedQueue) Dequeue() *InFlightMessage {
#   q.Mu.Lock()
#   defer q.Mu.Unlock()
#   if len(q.Queue) == 0 {
#     return nil
#   }
#   msg := q.Queue[0]
#   q.Queue = q.Queue[1:]
#   return msg
# }
#
# Verification:
# - Msg A queued first, Msg B second
# - Msg A delivered before Msg B
# - If Msg A fails + retry, Msg B still waits
```

**[V] Victory**:
- Messages delivered in order per destination
- No messages out of order
- Failed messages don't unblock queue

**[R] Race Detection**:
```bash
go test -race ./cmd/wotan -run TestOrdering
```

---

### 5.6 [B] Implement Idempotency (Duplicate Detection)
**Time**: 60 min | **Agent**: H

```bash
# cmd/wotan/idempotency.go (new file)
#
# IdempotencyManager:
# - Each message has unique ID (UUID)
# - Recipient tracks seen message IDs
# - Duplicate message ID → return cached result
# - Cache expires after 24h (then can process again)
#
# Implementation:
# type IdempotencyCache struct {
#   SeenMessages map[string]*ProcessResult
#   Mu sync.Mutex
# }
#
# func (ic *IdempotencyCache) Process(msg *Message) *Result {
#   ic.Mu.Lock()
#   if result, ok := ic.SeenMessages[msg.ID]; ok {
#     ic.Mu.Unlock()
#     return result
#   }
#   ic.Mu.Unlock()
#
#   result := ProcessMessage(msg)
#
#   ic.Mu.Lock()
#   ic.SeenMessages[msg.ID] = result
#   ic.Mu.Unlock()
#
#   return result
# }
#
# Cleanup: remove entries older than 24h
```

**[V] Victory**:
- Duplicate messages rejected
- Same result returned for duplicates
- No side effects from duplicate processing

---

### 5.7 [B] Implement Message Timeout (30s)
**Time**: 45 min | **Agent**: G

```bash
# cmd/wotan/timeout.go (new file)
#
# TimeoutManager:
# - Each message has 30s to be delivered
# - If no ack in 30s → timeout
# - Timeout triggers retry
# - After 5 retries + timeout → DLQ
#
# Implementation:
# ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
# defer cancel()
#
# select {
# case result := <-AckChan:
#   // Got ack within 30s
# case <-ctx.Done():
#   // Timeout after 30s
#   Retry(msg)
# }
```

**[V] Victory**:
- Messages timeout after 30s
- Timeout triggers retry
- No messages stuck waiting forever

---

### 5.8 [B] Wotan Hardening Integration Test
**Time**: 120 min | **Agent**: G

```bash
# cmd/wotan/integration_test.go
#
# Test scenarios:
# 1. Happy path: send msg → recv msg → ack → done
# 2. Ack timeout: send msg → wait 30s → retry
# 3. Nack: send msg → nack → retry
# 4. Max retries: send msg → nack 5x → DLQ
# 5. Ordering: send A, B, C → verify delivered A, B, C (not B, C, A)
# 6. Idempotency: send msg → send same ID → get same result
# 7. DLQ: failed message appears in DLQ after 5 retries
# 8. Concurrent: 1000 concurrent messages → all delivered correctly
#
# Run: make test-wotan-hardening
```

**[V] Victory**:
- All scenarios passing
- No race conditions (race detector)
- No lost messages
- Ordering preserved
- Idempotency working

---

### 5.9 [P] Load Test Wotan: 10K Msg/Sec
**Time**: 60 min | **Agent**: H

```bash
# Load test:
# - Send 10,000 messages/sec
# - Monitor latency (p50, p95, p99)
# - Monitor memory growth
# - Monitor CPU usage
# - Check for dropped messages
# - Check DLQ (should be 0)
#
# make load-test-wotan
# Results: /tmp/wotan-load-test.txt
```

**[V] Victory**:
- 10K msg/sec sustained
- p99 latency <500ms
- Memory stable (no leaks)
- Zero dropped messages
- Zero spurious DLQ entries

---

### 5.10 [C] Commit Checkpoint: Phase 5 Wotan Hardening Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.5.10: Phase 5 complete - Wotan hardening (ack/nack, retry, DLQ, ordering, idempotency, timeout)"
```

---

## PHASE 6: CONTAINER SECURITY - IMAGE SCANNING, HARDENING, RUNTIME ISOLATION

**Objective**: Secure all container images, harden runtime, implement read-only rootfs.

### 6.1 [B] Container Image Security Scanning
**Time**: 90 min | **Agent**: D

```bash
# Scan all container images for vulnerabilities
# Tool: Trivy (trivy image <image>)
#
# For each service container:
# 1. Build image
# 2. Scan with Trivy: trivy image --severity CRITICAL,HIGH <image>
# 3. Document findings
# 4. Fix: update base image, dependencies
# 5. Rescan: verify fixes
#
# Base image strategy:
# - Use distroless/base:nonroot (minimal attack surface)
# - Or alpine:3.19 (lightweight)
# - NOT ubuntu, centos (large, many CVEs)
#
# Example Dockerfile:
# FROM golang:1.24-alpine AS builder
# WORKDIR /app
# COPY . .
# RUN go build -o service cmd/service/main.go
#
# FROM distroless/base:nonroot
# COPY --from=builder /app/service /service
# USER nonroot
# ENTRYPOINT ["/service"]
```

**[V] Victory**:
- All images scanned
- Zero CRITICAL vulnerabilities
- ≤3 HIGH vulnerabilities (documented + plan to fix)
- Base images from distroless/alpine only
- Scan results: /tmp/s39-image-scans/

---

### 6.2 [B] Container Runtime Security: seccomp + AppArmor
**Time**: 90 min | **Agent**: D

```bash
# seccomp (Secure Computing Mode):
# - Restrict syscalls available to container
# - Each service gets minimal syscall whitelist
#
# Example seccomp profile (default.json):
# {
#   "defaultAction": "SCMP_ACT_ERRNO",
#   "defaultErrnoRet": 1,
#   "archMap": [{"architecture": "SCMP_ARCH_X86_64", ...}],
#   "syscalls": [
#     {"names": ["read", "write", "exit", ...], "action": "SCMP_ACT_ALLOW"}
#   ]
# }
#
# Docker: --security-opt seccomp=default.json
#
# AppArmor:
# - Mandatory access control
# - Restrict file access, network access, capabilities
#
# Example AppArmor profile:
# #include <tunables/global>
# /usr/bin/service {
#   #include <abstractions/base>
#   /etc/unheaded/** r,
#   /var/log/unheaded/** w,
#   deny /etc/shadow r,
# }
#
# Load: sudo apparmor_parser -r /etc/apparmor.d/unheaded.service
```

**[V] Victory**:
- seccomp profiles created for all services
- AppArmor profiles created for all services
- Profiles enforce minimal privileges
- No legitimate function blocked
- Runtime violations logged

---

### 6.3 [B] Read-Only Root Filesystem
**Time**: 60 min | **Agent**: D

```bash
# Run containers with read-only root filesystem
# Docker: --read-only
#
# Writeable mounts only:
# - /tmp (tempfiles, PID files)
# - /var/log/unheaded (logs)
# - /var/spool/unheaded (spools, DLQ)
#
# Example docker-compose:
# services:
#   gateway:
#     image: unheaded/gateway:v0.1.0
#     read_only: true
#     volumes:
#       - /tmp
#       - /var/log/unheaded
#       - /var/spool/unheaded
#
# Verification:
# - Attempt to write to /etc → EROFS
# - Attempt to write to /usr → EROFS
# - Write to /tmp → success
```

**[V] Victory**:
- All containers run with read-only root
- Writeable mounts limited to 3 locations
- No unauthorized writes allowed
- Error handling for read-only violations

---

### 6.4 [B] Container Capability Dropping
**Time**: 60 min | **Agent**: D

```bash
# Drop unnecessary Linux capabilities
# Default: keep minimal set
#
# Capabilities to keep:
# - NET_BIND_SERVICE (bind to port)
# - CHOWN (own files in /var/log)
#
# Capabilities to drop:
# - SYS_ADMIN, SYS_PTRACE, NET_ADMIN, etc.
#
# Docker example:
# cap_drop:
#   - ALL
# cap_add:
#   - NET_BIND_SERVICE
#   - CHOWN
#
# Verification: docker inspect <container> | jq .HostConfig.CapAdd/CapDrop
```

**[V] Victory**:
- All containers drop ALL capabilities
- Only necessary caps added back
- Zero dangerous capabilities present

---

### 6.5 [B] Container Network Policy (Segmentation)
**Time**: 90 min | **Agent**: E

```bash
# Kubernetes NetworkPolicy / Docker network segmentation
# Restrict network traffic between containers
#
# Policies:
# 1. Gateway can receive from external (0.0.0.0)
# 2. Gateway can call Wotan, Dashboard
# 3. Wotan can call all services
# 4. Internal services can call other internal services
# 5. No external traffic allowed (except Gateway)
#
# Example Kubernetes NetworkPolicy:
# apiVersion: networking.k8s.io/v1
# kind: NetworkPolicy
# metadata:
#   name: unheaded-gateway
# spec:
#   podSelector:
#     matchLabels:
#       app: gateway
#   policyTypes:
#   - Ingress
#   - Egress
#   ingress:
#   - from:
#     - podSelector: {}  # Any pod in namespace
#     ports:
#     - protocol: TCP
#       port: 21443
#   egress:
#   - to:
#     - podSelector:
#         matchLabels:
#           app: wotan
#     ports:
#     - protocol: TCP
#       port: 18001
```

**[V] Victory**:
- Network policies defined for all services
- Traffic segmentation working
- No inter-service bypasses
- Gateway isolated from internal services

---

### 6.6 [B] Container Runtime Monitoring
**Time**: 60 min | **Agent**: E

```bash
# Monitor container activity:
# - File access violations (read-only root)
# - Capability denials
# - seccomp violations
# - AppArmor denials
# - Network policy violations
#
# Implementation:
# - auditd rules for containers
# - Log to /var/log/unheaded/container-audit.log
# - Alert on violations
#
# Example auditd rule:
# -a always,exit -F dir=/etc/ -F perm=w -k container_write
#
# Verification:
# sudo ausearch -k container_write | tail -20
```

**[V] Victory**:
- Runtime violations logged
- Zero malicious activity detected
- Audit trail maintained

---

### 6.7 [B] Build Pipeline Security
**Time**: 60 min | **Agent**: E

```bash
# Secure container build process:
# 1. Build on secure builder machine (not developer laptop)
# 2. Sign all images (cosign or similar)
# 3. Scan before pushing to registry
# 4. No pushing if scan fails
# 5. Use immutable image tags (digest, not :latest)
#
# Makefile:
# build-container:
#   docker build -t unheaded/gateway:$(GIT_COMMIT) .
#   trivy image unheaded/gateway:$(GIT_COMMIT)
#   # If scan fails, exit 1
#   docker push unheaded/gateway:$(GIT_COMMIT)
#   docker sign unheaded/gateway:$(GIT_COMMIT)
#
# Deploy uses: unheaded/gateway:sha256:abc123... (digest)
```

**[V] Victory**:
- All images signed
- All images scanned before push
- All deployments use immutable digests

---

### 6.8 [C] Commit Checkpoint: Phase 6 Container Security Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.6.8: Phase 6 complete - Container security (scanning, seccomp, AppArmor, read-only rootfs, capabilities, network policies)"
```

---

## PHASE 7: E2E INTEGRATION TEST SUITE

**Objective**: Full pipeline tests: browser → gateway → services → eBPF → Wotan → dashboard.

### 7.1 [B] E2E Test Framework Setup
**Time**: 60 min | **Agent**: B

```bash
# Create cmd/e2e-tests/ or tests/e2e/
#
# Structure:
# tests/e2e/
#   ├── main.go
#   ├── scenarios/
#   │   ├── auth_flow.go
#   │   ├── threat_detection.go
#   │   ├── message_delivery.go
#   │   ├── dashboard_flow.go
#   │   └── security_response.go
#   └── utils/
#       ├── docker_compose.go
#       ├── client.go
#       └── assertions.go
#
# Execution:
# make e2e-test (brings up docker-compose, runs tests, teardown)
# Duration: ~10-15 minutes
```

**[V] Victory**:
- E2E test framework compiles
- docker-compose brings up full stack
- Tests can make HTTP/gRPC calls
- Tests can verify metrics
- Teardown clean

---

### 7.2 [B] E2E Scenario 1: Auth Flow
**Time**: 90 min | **Agent**: B

**Test**: User login → JWT → API call → verify auth context

```bash
# tests/e2e/scenarios/auth_flow.go
#
# Steps:
# 1. User submits API key to /login
# 2. Verify JWT returned
# 3. Use JWT to call /api/v1/status
# 4. Verify auth context in request (user = "api_key_user")
# 5. Verify audit log records auth events
# 6. Use expired JWT → verify 401
#
# Assertions:
# - Login returns JWT with 8h expiry
# - Protected endpoint accessible with JWT
# - Protected endpoint returns 401 without JWT
# - Audit log has 3+ entries (login, status call, auth success)
```

**[V] Victory**:
- Auth flow end-to-end working
- JWT valid for full request chain
- Audit trail recorded
- Invalid tokens rejected

---

### 7.3 [B] E2E Scenario 2: Threat Detection Pipeline
**Time**: 120 min | **Agent**: B

**Test**: Threat created → processed → stored → visible in dashboard

```bash
# tests/e2e/scenarios/threat_detection.go
#
# Steps:
# 1. POST /api/v1/threats (gateway)
# 2. Gateway routes to threat-engine
# 3. Threat-engine analyzes threat
# 4. Result stored in threat database
# 5. Wotan delivers result to dashboard
# 6. Dashboard makes query to retrieve threat
# 7. Verify threat visible with all details
#
# Test data:
# {
#   "name": "e2e-test-threat-001",
#   "description": "E2E test",
#   "severity": "high",
#   "payload": "test payload"
# }
#
# Assertions:
# - Threat created (200 response)
# - Threat stored (visible in database)
# - Threat delivered to dashboard (via Wotan)
# - Dashboard query returns threat with all fields
# - Timeline: <2s from creation to dashboard visibility
```

**[V] Victory**:
- Threat pipeline end-to-end working
- Data flows through all services
- Wotan delivery reliable
- Dashboard reflects changes <2s

---

### 7.4 [B] E2E Scenario 3: Message Delivery Reliability
**Time**: 90 min | **Agent**: B

**Test**: Send 1000 messages → verify all delivered, no loss

```bash
# tests/e2e/scenarios/message_delivery.go
#
# Steps:
# 1. Send 1000 messages via Wotan (rapid fire)
# 2. Each message has unique ID
# 3. Recipient tracks received IDs
# 4. After 60s, verify all 1000 received
# 5. Verify ordering preserved (per-queue)
# 6. Verify no duplicates
#
# Scenario: Network hiccup
# 1. Send 100 messages
# 2. Pause recipient (simulate network delay)
# 3. Wait 35s (past timeout)
# 4. Resume recipient
# 5. Verify messages retried and delivered
# 6. Verify no duplicates (idempotency worked)
#
# Assertions:
# - All 1000 messages delivered
# - No duplicates
# - Ordering preserved
# - Retry mechanism triggered on pause
```

**[V] Victory**:
- Message delivery reliable
- No lost messages
- No duplicates
- Retry working as expected

---

### 7.5 [B] E2E Scenario 4: Dashboard Workflow
**Time**: 90 min | **Agent**: B

**Test**: Dashboard login → create threat → view threat → export report

```bash
# tests/e2e/scenarios/dashboard_flow.go
#
# Steps (simulating browser):
# 1. GET /dashboard/login (public)
# 2. POST /dashboard/login (API key)
# 3. Get JWT from response
# 4. GET /dashboard/home (with JWT)
# 5. POST /dashboard/threat/create (create threat)
# 6. GET /dashboard/threat/:id (view threat)
# 7. GET /dashboard/reports/export (export as CSV)
# 8. Verify CSV contains threat data
#
# Browser simulation: curl or HTTP client
#
# Assertions:
# - Login returns JWT
# - Dashboard pages accessible
# - Threat creation works
# - Threat visible immediately
# - Export includes all threats
```

**[V] Victory**:
- Dashboard workflow end-to-end
- All pages load correctly
- Data persists across requests
- Export working

---

### 7.6 [B] E2E Scenario 5: Security Response Workflow
**Time**: 120 min | **Agent**: B

**Test**: Threat detected → response action triggered → verify remediation

```bash
# tests/e2e/scenarios/security_response.go
#
# Steps:
# 1. Create threat (severity: critical)
# 2. Response engine auto-triggers
# 3. Response action: "isolate service"
# 4. Verify isolation action executed
# 5. Verify service responds but no external traffic
# 6. Verify audit trail records response
#
# Example response:
# - Threat: "DDoS attack detected"
# - Action: "Rate limit source IP"
# - Verification: "Subsequent requests from IP get 429"
#
# Assertions:
# - Response triggered within 5s
# - Isolation applied correctly
# - Audit trail has response entry
```

**[V] Victory**:
- Security response workflow working
- Actions executed correctly
- Audit trail recorded

---

### 7.7 [B] E2E Scenario 6: High Availability / Failover
**Time**: 120 min | **Agent**: B

**Test**: Kill a service → verify failover → traffic redirected

```bash
# tests/e2e/scenarios/ha_failover.go
#
# Steps:
# 1. Start 2 instances of threat-engine
# 2. Send 100 requests
# 3. Kill instance 1 (simulate failure)
# 4. Remaining 50 requests → instance 2 handles them
# 5. Verify no dropped requests
# 6. Verify client experiences no 5xx errors
#
# Assertions:
# - No request failures during failover
# - All 100 requests processed
# - Failover time <2s
```

**[V] Victory**:
- Failover working
- No request loss during failover
- High availability verified

---

### 7.8 [B] E2E Scenario 7: Metrics + Observability
**Time**: 90 min | **Agent**: B

**Test**: Execute threat pipeline → verify metrics in dashboard

```bash
# tests/e2e/scenarios/metrics_observability.go
#
# Steps:
# 1. Execute complete threat pipeline
# 2. Query /debug/metrics (Prometheus)
# 3. Verify metrics present:
#    - http_requests_total (count)
#    - grpc_requests_total (count)
#    - request_duration_seconds (histogram)
#    - threats_processed_total (counter)
#    - wotan_messages_delivered (counter)
# 4. Query dashboard for metrics graph
# 5. Verify graph shows threat pipeline
#
# Assertions:
# - All metrics exported
# - Metrics have correct values
# - Metrics queryable from dashboard
```

**[V] Victory**:
- Metrics flowing through all services
- Dashboard displays metrics correctly
- Observability working end-to-end

---

### 7.9 [B] E2E Test Suite Execution + Reporting
**Time**: 60 min | **Agent**: B

```bash
# make e2e-test
#
# Execution:
# 1. docker-compose up (full stack)
# 2. Wait for health checks
# 3. Run all 7 scenarios in parallel
# 4. Collect results
# 5. Generate report
# 6. docker-compose down
#
# Report: /tmp/e2e-test-report.md
# - Scenario results (pass/fail)
# - Timing (how long each scenario took)
# - Failures (if any, with error details)
# - Recommendations
```

**[V] Victory**:
- All 7 scenarios PASS
- Report generated
- No failures
- Full stack working end-to-end

**[P] Performance Observations**:
- Threat creation to dashboard visibility: <2s
- Message delivery: <100ms per message
- Auth latency: <10ms per request
- Failover time: <2s

---

### 7.10 [C] Commit Checkpoint: Phase 7 E2E Tests Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.7.10: Phase 7 complete - E2E integration test suite (7 scenarios, all passing)"
```

---

## PHASE 8: DEPLOYMENT PIPELINE - MAKE DEPLOY

**Objective**: Single command deployment: `make deploy` works, health verification, rollback.

### 8.1 [B] Design Deployment Architecture
**Time**: 60 min | **Agent**: H

```bash
# Create docs/deployment-architecture.md
#
# Deployment strategy: Blue-Green
# 1. Blue: Current production (running)
# 2. Green: New version (staged)
# 3. Health checks on green
# 4. Switch traffic to green
# 5. Keep blue as rollback target
#
# Deployment targets:
# - Development: Local docker-compose
# - Staging: K8s cluster (staging namespace)
# - Production: K8s cluster (prod namespace, canary first)
#
# Deployment flow:
# 1. make deploy (target=staging)
#    → Build images
#    → Tag with version + commit
#    → Push to registry
#    → Update K8s manifests
#    → Apply manifests
#    → Wait for rollout
#    → Run health checks
#    → Report status
# 2. make deploy (target=prod) (manual approval)
#    → Same as staging, but canary first (10% traffic)
#    → Monitor metrics for 5 min
#    → If good: 100% traffic
#    → If bad: rollback
```

**[V] Victory**:
- Deployment architecture designed
- Blue-green strategy documented
- Health check strategy clear
- Rollback procedure documented

---

### 8.2 [B] Implement make deploy Target
**Time**: 120 min | **Agent**: H

```bash
# Makefile additions:
#
# make deploy target=staging
#
# .PHONY: deploy
# deploy:
#   @echo "Deploying to $(target) environment..."
#   $(MAKE) build-all
#   $(MAKE) push-images
#   $(MAKE) update-k8s-manifests VERSION=$(VERSION)
#   kubectl apply -f k8s/$(target)/ --namespace=$(target)
#   $(MAKE) wait-for-rollout NAMESPACE=$(target)
#   $(MAKE) health-check TARGET=$(target)
#   @echo "Deployment to $(target) complete!"
#
# Sub-targets:
# make build-all
#   → Build all 20+ service images
#   → Tag with VERSION (e.g., v0.1.0-alpha)
#   → Tag with GIT_COMMIT
#
# make push-images
#   → Push images to registry
#   → Verify push success
#
# make update-k8s-manifests
#   → Update image digests in k8s/*/deployment.yaml
#   → Update tags to new version
#
# make wait-for-rollout
#   → kubectl rollout status (all deployments)
#   → Timeout after 10 min
#   → Fail if any rollout fails
#
# make health-check
#   → Verify all services healthy
#   → Verify all endpoints responding
#   → Verify E2E tests passing
#   → Fail if any check fails
```

**[V] Victory**:
- `make deploy target=staging` works
- All services deployed
- Health checks passing
- Rollout successful

---

### 8.3 [B] Health Check Endpoint Implementation
**Time**: 60 min | **Agent**: H

```bash
# All services expose /health endpoint (HTTP + gRPC)
#
# HTTP /health response:
# {
#   "status": "healthy|degraded|down",
#   "timestamp": "2026-02-24T12:34:56Z",
#   "version": "v0.1.0-alpha",
#   "commit": "abc123",
#   "dependencies": {
#     "database": "healthy",
#     "wotan": "healthy",
#     "discovery": "healthy"
#   }
# }
#
# Health check logic:
# 1. Service is up → healthy
# 2. Service can reach dependencies → healthy
# 3. Service has log aggregation → healthy
# 4. Service has metrics exporter → healthy
# 5. Any dependency down → degraded (service still up, but limited)
# 6. Service crash → down
#
# Readiness check (/readiness):
# - Service ready to receive traffic
# - All dependencies ready
# - Startup tasks complete
#
# Liveness check (/liveness):
# - Service is alive (heartbeat)
# - Graceful shutdown in progress → fail (to trigger restart)
```

**[V] Victory**:
- All services have /health endpoint
- Health check aggregates dependencies
- Readiness and liveness distinct
- Health check response <100ms

---

### 8.4 [B] Implement Rollback Mechanism
**Time**: 90 min | **Agent**: H

```bash
# make rollback target=staging
#
# Rollback strategy:
# 1. Current prod version = v0.1.0-alpha-rev1
# 2. Previous version = v0.1.0-alpha-rev0
# 3. Rollback: revert to v0.1.0-alpha-rev0
# 4. Health checks on rollback
#
# Implementation:
# 1. Keep last 3 versions in registry (don't delete)
# 2. Store deployment manifest per version
# 3. Rollback = apply previous manifest
#
# Makefile:
# .PHONY: rollback
# rollback:
#   @echo "Rolling back $(target)..."
#   PREV_VERSION=$$(kubectl rollout history deployment -n $(target) | tail -2 | head -1 | awk '{print $$1}')
#   kubectl rollout undo deployment -n $(target) --to-revision=$(PREV_VERSION)
#   $(MAKE) wait-for-rollout NAMESPACE=$(target)
#   $(MAKE) health-check TARGET=$(target)
#   @echo "Rollback to $(PREV_VERSION) complete!"
#
# Verification:
# - Previous version running
# - All health checks passing
# - Logs show rollback event
```

**[V] Victory**:
- Rollback mechanism working
- One command rollback
- Previous version recovers
- Health checks verify success

---

### 8.5 [B] Canary Deployment for Production
**Time**: 90 min | **Agent**: H

```bash
# make deploy target=prod (with canary)
#
# Canary strategy:
# 1. Deploy new version to 10% of pods
# 2. Monitor metrics for 5 minutes
#   - Error rate (must be <0.5% higher than previous)
#   - P95 latency (must be <20% higher than previous)
#   - CPU/memory (must be <10% higher than previous)
# 3. If metrics good: increase to 100% (100% traffic)
# 4. If metrics bad: rollback immediately
#
# Implementation (using Flagger):
# apiVersion: flagger.app/v1beta1
# kind: Canary
# metadata:
#   name: unheaded-gateway
# spec:
#   targetRef:
#     apiVersion: apps/v1
#     kind: Deployment
#     name: gateway
#   progressDeadlineSeconds: 60
#   service:
#     port: 21443
#   analysis:
#     interval: 1m
#     threshold: 5
#     maxWeight: 100
#     stepWeight: 10
#     metrics:
#     - name: request-success-rate
#       thresholdRange:
#         min: 99
#       interval: 1m
#     - name: request-duration
#       thresholdRange:
#         max: 500
#       interval: 1m
#   skipAnalysis: false
```

**[V] Victory**:
- Canary deployment working
- 10% traffic → new version initially
- Metrics monitored automatically
- Automatic promotion to 100% on success
- Automatic rollback on failure

---

### 8.6 [B] Deployment Secrets Management
**Time**: 60 min | **Agent**: H

```bash
# Secrets needed for deployment:
# 1. Container registry credentials (push images)
# 2. K8s cluster credentials (kubectl access)
# 3. JWT keypair (for services)
# 4. API keys (for services)
# 5. TLS certs (for services)
#
# Management:
# - Store in HashiCorp Vault (production)
# - Store in .env.local (development, git-ignored)
# - Inject into CI/CD pipeline securely
#
# Example GitHub Actions secrets:
# - DOCKER_REGISTRY_URL
# - DOCKER_USERNAME
# - DOCKER_PASSWORD
# - K8S_CLUSTER_URL
# - K8S_API_TOKEN
# - VAULT_ADDR
# - VAULT_TOKEN
#
# Deployment job:
# - Fetch secrets from Vault
# - Build and push images
# - Deploy to K8s
# - Verify health
# - All secrets removed after deployment
```

**[V] Victory**:
- Secrets managed securely
- No hardcoded secrets in code
- CI/CD can access secrets
- Secrets not logged

---

### 8.7 [B] Deployment Monitoring + Alerts
**Time**: 60 min | **Agent**: H

```bash
# Monitor deployment in progress:
# 1. Watch pod status (kubectl get pods -w)
# 2. Watch rollout progress (kubectl rollout status)
# 3. Stream logs (kubectl logs -f deployment/xxx)
# 4. Monitor metrics (Prometheus)
#
# Alerts during deployment:
# - Pod crash loop → stop deployment, alert
# - Health check failing → stop deployment, alert
# - Metrics anomaly → escalate to manual review
# - Deployment timeout (>10min) → rollback
#
# Implementation:
# - Prometheus rules for deployment metrics
# - AlertManager for notifications
# - Slack integration for alerts
# - PagerDuty for critical issues
```

**[V] Victory**:
- Deployment monitoring active
- Alerts working
- Human oversight during deployment

---

### 8.8 [D] Document Deployment Runbook
**Time**: 45 min | **Agent**: H

```bash
# Create docs/deployment-runbook.md
#
# Runbook sections:
# 1. Prerequisites (cluster ready, images built)
# 2. Staging deployment (make deploy target=staging)
# 3. Verify staging (health checks, E2E tests)
# 4. Production deployment (make deploy target=prod)
# 5. Monitor deployment (metrics, logs, alerts)
# 6. Rollback procedure (make rollback target=prod)
# 7. Troubleshooting (common issues + fixes)
# 8. Post-deployment (audit, documentation)
#
# Quick start:
# make deploy target=staging  # → 5-10 min
# make e2e-test              # → 15 min
# make deploy target=prod    # → 10-15 min (with canary)
```

**[V] Victory**:
- Runbook documented
- Clear steps for operators
- Troubleshooting guide present

---

### 8.9 [C] Commit Checkpoint: Phase 8 Deployment Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.8.9: Phase 8 complete - Deployment pipeline (make deploy, health checks, rollback, canary, monitoring)"
```

---

## PHASE 9: DOCUMENTATION FINAL PASS

**Objective**: All documentation updated, wiki complete, runbooks ready.

### 9.1 [B] API Documentation (OpenAPI/Swagger)
**Time**: 90 min | **Agent**: F

```bash
# Generate OpenAPI spec for all services
# Tool: go-swagger or protoc-gen-openapiv2
#
# Each service documents:
# 1. HTTP endpoints (path, method, auth required)
# 2. Request/response schemas (with examples)
# 3. Error codes (401, 403, 404, 500)
# 4. Auth requirements (JWT, API key, role)
#
# Output: /api-docs/openapi.yaml
#
# Swagger UI:
# - Host at /docs/api
# - Interactive API explorer
# - "Try it out" functionality (with auth token)
#
# Example endpoint:
# /api/v1/threats:
#   post:
#     summary: Create threat
#     security:
#     - bearerAuth: []
#     parameters: [...]
#     requestBody:
#       required: true
#       content:
#         application/json:
#           schema: ThreatRequest
#     responses:
#       '200':
#         description: Threat created
#         content:
#           application/json:
#             schema: ThreatResponse
#       '401':
#         description: Unauthorized
#       '403':
#         description: Forbidden
```

**[V] Victory**:
- OpenAPI spec complete
- All endpoints documented
- Swagger UI functional
- Examples working

---

### 9.2 [B] Architecture Documentation
**Time**: 90 min | **Agent**: F

```bash
# Create docs/architecture.md
#
# Sections:
# 1. System overview (diagram)
# 2. Service relationships (20+ services, how they interact)
# 3. Data flow (threat creation → response → dashboard)
# 4. Security layers (auth, mTLS, RBAC)
# 5. Message delivery (Wotan architecture)
# 6. Observability stack (logging, metrics, tracing)
# 7. Deployment architecture (K8s, namespaces, resources)
# 8. High availability (replicas, failover, load balancing)
#
# Diagrams:
# - System architecture diagram (Mermaid or PlantUML)
# - Data flow diagram
# - Deployment diagram
# - Network topology
#
# Audience: Architects, ops, new team members
```

**[V] Victory**:
- Architecture document complete
- All diagrams present
- Clear explanations
- Ready for review

---

### 9.3 [B] Security Documentation
**Time**: 90 min | **Agent**: F

```bash
# Create docs/security.md
#
# Sections:
# 1. Authentication scheme (JWT, API key, service tokens)
# 2. Authorization (RBAC, role definitions)
# 3. Transport security (mTLS, TLS versions, ciphers)
# 4. Data protection (encryption at rest, in transit)
# 5. Secret management (Vault, rotation, access controls)
# 6. Audit logging (what gets logged, retention)
# 7. Compliance (what standards met, certifications)
# 8. Incident response (how to handle security incidents)
# 9. Security testing (Lich campaigns, pentesting)
# 10. Known limitations (what's not yet secured)
#
# Audience: Security team, auditors, compliance officers
```

**[V] Victory**:
- Security document complete
- All controls documented
- Compliance framework clear

---

### 9.4 [B] Operational Documentation
**Time**: 90 min | **Agent**: F

```bash
# Create docs/operations.md
#
# Sections:
# 1. Deployment guide (make deploy, step-by-step)
# 2. Health checks (what to monitor, alert thresholds)
# 3. Scaling (add replicas, handle peak load)
# 4. Backup + restore (data, configs, secrets)
# 5. Maintenance (updates, patches, rotations)
# 6. Troubleshooting (common issues, logs to check)
# 7. Performance tuning (knobs to adjust)
# 8. Capacity planning (resource sizing)
# 9. On-call runbook (what to do during incident)
# 10. Communication (how to notify users during outage)
#
# Audience: Operations team, on-call engineers
```

**[V] Victory**:
- Ops documentation complete
- Ready for handoff to ops team

---

### 9.5 [B] Developer Documentation
**Time**: 90 min | **Agent**: F

```bash
# Create docs/development.md
#
# Sections:
# 1. Setup (clone, dependencies, local dev)
# 2. Build (make build, docker build)
# 3. Run locally (make run, docker-compose up)
# 4. Testing (make test, make race-test, make e2e-test)
# 5. Code style (linting, formatting, conventions)
# 6. Adding new service (template, checklist)
# 7. Adding new endpoint (template, auth, tests)
# 8. Debugging (logs, metrics, debugger)
# 9. Contributing (PR process, code review)
# 10. Architecture decisions (why design choices made)
#
# Audience: Developers, contributors
```

**[V] Victory**:
- Dev documentation complete
- New developers can onboard quickly

---

### 9.6 [B] README + GETTING STARTED
**Time**: 60 min | **Agent**: F

```bash
# Create / update README.md
#
# Sections:
# 1. Project overview (1 paragraph)
# 2. Key features (bullet points)
# 3. Quick start (clone, build, run, test)
# 4. Documentation links (architecture, security, ops, dev)
# 5. Contributing (link to CONTRIBUTING.md)
# 6. License (BSL 1.1)
# 7. Support (contact, bugs, features)
#
# Create GETTING_STARTED.md
# 1. Local setup (prerequisites, commands)
# 2. First threat creation (walk through example)
# 3. Dashboard access (login, exploring)
# 4. Common tasks (how to X, how to Y)
```

**[V] Victory**:
- README complete
- Getting started guide ready
- Onboarding smooth

---

### 9.7 [B] Wiki Complete + Searchable
**Time**: 60 min | **Agent**: F

```bash
# Create docs/wiki/ with searchable index
#
# Structure:
# docs/wiki/
#   ├── index.md (main page, links to all docs)
#   ├── glossary.md (terms, definitions)
#   ├── faq.md (common questions)
#   ├── troubleshooting.md (problems + solutions)
#   ├── checklists/
#   │   ├── deployment-checklist.md
#   │   ├── security-audit-checklist.md
#   │   └── release-checklist.md
#   └── examples/
#       ├── creating-threat.md
#       ├── custom-response.md
#       └── integration-guide.md
#
# Generate search index:
# - mkdocs or similar static site generator
# - Hosted at /docs (or wiki.unheaded.local)
# - Full-text search
# - Mobile friendly
```

**[V] Victory**:
- Wiki complete
- All docs searchable
- Navigation clear

---

### 9.8 [D] Create CHANGELOG + Release Notes (Draft)
**Time**: 45 min | **Agent**: F

```bash
# Create CHANGELOG.md
# Format: Keep a Changelog (https://keepachangelog.com/)
#
# [v0.1.0-alpha] - 2026-02-24
#
# Added:
# - JWT + API key authentication on all endpoints
# - mTLS service mesh (mutual TLS between all services)
# - Lich security test suite (D1-D6 campaigns)
# - Wotan message delivery hardening (ack/nack, retry, DLQ)
# - Container security (scanning, seccomp, AppArmor, read-only rootfs)
# - E2E integration test suite (7 scenarios)
# - Deployment pipeline (make deploy, health checks, rollback)
# - Complete documentation (API, architecture, security, ops, dev)
#
# Changed:
# - [list non-backward compatible changes]
#
# Fixed:
# - [list bug fixes]
#
# Security:
# - Dropped plaintext gRPC (now mTLS only)
# - Enforced RBAC on all endpoints
# - Implemented audit logging for all auth events
#
# Known Issues:
# - [list known issues, if any]
#
# Deployment Notes:
# - Requires JWT keypair generation (see docs/deployment-runbook.md)
# - Container image signatures required (see docs/security.md)
#
# Contributor Credits:
# [names of contributors]
```

**[V] Victory**:
- Changelog complete
- Release notes drafted
- Ready for final sign-off

---

### 9.9 [C] Commit Checkpoint: Phase 9 Documentation Complete
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.9.9: Phase 9 complete - All documentation (API, architecture, security, ops, dev, wiki, changelog)"
```

---

## PHASE 10: ALPHA SHIP GATE - v0.1.0-alpha TAG + COMPLIANCE

**Objective**: Final gate: v0.1.0-alpha tag, compliance snapshot, SBOM refresh, release, ship.

### 10.1 [B] Final Security Audit Review
**Time**: 120 min | **Agents**: C, W (Warmonger)

```bash
# Comprehensive security audit:
# 1. Review Lich findings (D1-D6) → all remediated ✓
# 2. Review auth implementation → all services protected ✓
# 3. Review mTLS → zero plaintext ✓
# 4. Review secrets management → no hardcoded secrets ✓
# 5. Review container security → all hardened ✓
# 6. Review audit logging → all events logged ✓
# 7. Dependency scan → zero known critical CVEs ✓
# 8. Code review → all PRs reviewed ✓
#
# Output: /tmp/S39-SECURITY-AUDIT-FINAL.md
# Sign-off: Warmonger + Lead Security
```

**[V] Victory**:
- Final audit complete
- All findings addressed
- Sign-off obtained
- Ready for compliance snapshot

---

### 10.2 [B] SBOM Refresh + Dependency Audit
**Time**: 90 min | **Agent**: G

```bash
# Generate fresh SBOM (Software Bill of Materials):
# - syft command for all container images
# - syft for Go dependencies (go list -json)
# - syft for Rust dependencies (cargo tree)
#
# syft unheaded:latest -o cyclonedx > sbom-latest.xml
#
# Output: /tmp/s39-sbom-final.json
#
# Dependency audit:
# - go mod tidy (update go.mod)
# - cargo update (update Cargo.lock, check for vulns)
# - docker image scan (Trivy)
# - License audit (all licenses compatible with BSL 1.1)
#
# Verify:
# - No GPL/AGPL dependencies (only permissive + BSD/Apache)
# - No CVEs (or CVEs with remediation plan)
# - All dependencies pinned to exact versions
```

**[V] Victory**:
- SBOM generated (CycloneDX format)
- Dependencies audited
- No license conflicts
- No unaddressed CVEs

---

### 10.3 [B] Compliance Snapshot
**Time**: 90 min | **Agent**: G

```bash
# Create compliance snapshot:
# /tmp/s39-compliance-snapshot/
#   ├── security-audit.md (Lich results)
#   ├── authentication.md (all endpoints protected)
#   ├── encryption.md (TLS/mTLS everywhere)
#   ├── audit-logs.md (logging comprehensive)
#   ├── secrets-management.md (no exposed secrets)
#   ├── container-security.md (scanning + hardening)
#   ├── e2e-tests.md (all scenarios pass)
#   ├── sbom.json (software bill of materials)
#   └── README.md (index + summary)
#
# Compliance standards met:
# - CIS Docker/K8s benchmarks
# - OWASP Top 10 (all controls)
# - NIST Cybersecurity Framework (identify, protect, detect, respond, recover)
# - SOC 2 Type I readiness (access control, audit logging, monitoring)
#
# Sign-off: Compliance officer or lead architect
```

**[V] Victory**:
- Compliance snapshot complete
- All standards met
- Sign-off obtained

---

### 10.4 [B] Test Suite Final Verification
**Time**: 120 min | **Agent**: B

```bash
# Run all tests:
# make test                # Unit tests
# make race-test           # Race condition detection
# make e2e-test            # End-to-end integration tests
# make lich-all            # Security campaigns
# make load-test           # Performance tests
# make deploy target=staging  # Deployment test (staging only)
#
# Results:
# - Unit tests: 100% passing
# - Race detection: 0 races
# - E2E scenarios: 7/7 passing
# - Lich campaigns: D1-D6 passing (0 findings)
# - Load test: 10K msg/sec, <500ms p99
# - Deployment: successful, health checks pass
#
# Generate test report: /tmp/s39-test-report.md
```

**[V] Victory**:
- All tests passing
- Test report generated
- Ready for release

---

### 10.5 [B] Performance Baseline + SLO Definition
**Time**: 60 min | **Agent**: P (Performance)

```bash
# Establish Service Level Objectives (SLOs):
#
# Availability:
# - 99.9% uptime (4 nines, 43 min/month downtime)
#
# Latency:
# - p50 latency: <50ms
# - p95 latency: <200ms
# - p99 latency: <500ms
#
# Error Rate:
# - <0.1% 5xx errors
# - <0.01% 4xx client errors (invalid input)
#
# Message Delivery:
# - 100% delivery (no lost messages)
# - <1s median delivery time
#
# Deployment:
# - <15 min deployment time (including rollback)
# - 0% failed deployments (all rollouts succeed)
#
# Documentation: /docs/slos.md
```

**[V] Victory**:
- SLOs defined
- Baselines established
- Monitoring configured
- Ready for production

---

### 10.6 [B] Create Release Branch + Version Tag
**Time**: 30 min | **Agent**: A

```bash
# Create release branch and tag:
git checkout -b release/v0.1.0-alpha
git tag -a v0.1.0-alpha -m "S39 Sprint Complete: Industrialization Complete, Ship Ready"
git push origin release/v0.1.0-alpha
git push origin v0.1.0-alpha

# Tag must be signed (optional, but recommended):
# git tag -s -a v0.1.0-alpha -m "..."

# Version file updates:
# VERSION=v0.1.0-alpha (in Makefile, version.go, etc.)
# BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
# GIT_COMMIT=$(git rev-parse HEAD)
#
# Verify:
git tag -v v0.1.0-alpha
git describe --tags --always
```

**[V] Victory**:
- Release branch created
- Version tag pushed
- Version metadata set

---

### 10.7 [B] Build + Sign Release Artifacts
**Time**: 60 min | **Agent**: H

```bash
# Build release artifacts:
# 1. Compile all binaries (release mode)
# 2. Build all container images
# 3. Push images to registry with tag v0.1.0-alpha
# 4. Sign all images (cosign)
# 5. Generate checksums (SHA256)
# 6. Sign checksums
#
# Artifacts:
# - Container images (20+ services, all tagged v0.1.0-alpha)
# - Binary archives (tar.gz, zip)
# - Checksums (checksums.txt)
# - Signature (checksums.txt.sig, cosign.pub)
# - SBOM (sbom.json, sbom.xml)
#
# Storage: GitHub Releases or artifact repository
#
# Makefile:
# make release VERSION=v0.1.0-alpha
#   → builds all, signs, pushes, generates checksums
```

**[V] Victory**:
- All artifacts built
- All artifacts signed
- Checksums verified
- Ready for distribution

---

### 10.8 [B] Generate Release Notes
**Time**: 60 min | **Agent**: F

```bash
# Create release notes: /tmp/RELEASE-v0.1.0-alpha.md
#
# Sections:
# 1. Executive Summary
#    - v0.1.0-alpha: First shipping release
#    - 320-380 steps completed in S39 sprint
#    - Production-ready with operational concerns
#
# 2. What's New (S39 additions)
#    - JWT + API key authentication (all endpoints)
#    - mTLS service mesh (zero plaintext)
#    - Wotan hardening (ack/nack, retry, DLQ)
#    - Container security (scanning, hardening)
#    - E2E integration tests
#    - Deployment pipeline
#    - Complete documentation
#
# 3. Security
#    - All 6 Lich campaigns passing
#    - No known vulnerabilities
#    - Container images scanned + hardened
#    - Audit logging comprehensive
#
# 4. Performance
#    - Latency: p99 <500ms
#    - Throughput: 10K msg/sec
#    - Availability: 99.9% uptime target
#
# 5. Known Issues / Limitations
#    - [list any known issues]
#    - Roadmap for future releases
#
# 6. Installation
#    - Docker: docker run unheaded:v0.1.0-alpha
#    - K8s: kubectl apply -f k8s/prod-manifest.yaml
#    - See docs/deployment-runbook.md
#
# 7. Contributors
#    - Warmonger (battle plan, oversight)
#    - Agents A-H (implementation)
#
# 8. License
#    - BSL 1.1 (Business Source License)
#
# 9. Support + Feedback
#    - GitHub issues
#    - Discussions
#    - Email: support@unheaded.local
```

**[V] Victory**:
- Release notes complete
- Clear for users and ops

---

### 10.9 [W] WARMONGER FINAL REVIEW + SIGN-OFF
**Time**: 60 min | **Agent**: W (Warmonger)

```bash
# Warmonger reviews:
#
# Checklist:
# ✓ All 10 phases complete (0-10)
# ✓ All exit gates passed
# ✓ No critical findings unresolved
# ✓ All tests passing
# ✓ Documentation complete
# ✓ Compliance snapshot approved
# ✓ Release artifacts signed
# ✓ Release notes clear
# ✓ Emergency procedures documented (see Appendix A)
# ✓ Agent assignments honored (see Appendix B)
#
# Final decision:
# APPROVED FOR SHIP
#
# Warmonger signature:
# Date: 2026-02-24
# Authority: Unheaded Battle Master
# Seal: [Warmonger's mark]
```

**[V] Victory**:
- Warmonger sign-off obtained
- All gates passed
- Approved for ship

---

### 10.10 [B] Create GitHub Release + Announce
**Time**: 30 min | **Agent**: F

```bash
# Create GitHub release:
# 1. Go to Releases → Draft new release
# 2. Tag: v0.1.0-alpha
# 3. Release title: "v0.1.0-alpha: Production-Ready Alpha Release"
# 4. Description: [paste release notes from 10.8]
# 5. Attach artifacts:
#    - checksums.txt
#    - sbom.json
#    - sbom.xml
# 6. Mark as "pre-release" (it's alpha)
# 7. Publish
#
# Announce:
# - Blog post (if applicable)
# - Twitter/social media
# - Mailing list
# - Team Slack
#
# Message: "v0.1.0-alpha is live! 320+ steps, S39 complete. Ready to build on."
```

**[V] Victory**:
- GitHub release published
# - Announcement sent
- Community informed

---

### 10.11 [C] Commit Checkpoint: S39 COMPLETE
**Time**: 5 min | **Agent**: A

```bash
cd ~/tmp/unheaded
git add -A && git commit -m "S39.10.11: FINAL - v0.1.0-alpha released. 320-380 steps complete. Industrialization DONE. Ship ready."
```

---

## APPENDIX A: EMERGENCY PROCEDURES

### Emergency 1: Critical Security Finding (Post-Release)

```bash
# If critical CVE found after release:
# 1. Warmonger declares incident
# 2. Security team assesses risk
# 3. If risk > threshold:
#    a. Stop deployments
#    b. Develop patch
#    c. Tag v0.1.0-alpha-hotfix1
#    d. Re-run security tests (especially Lich campaigns)
#    e. Notify all deployed customers
#    f. Re-release with hotfix
#    g. Update advisory
```

### Emergency 2: Deployment Failure

```bash
# If make deploy fails:
# 1. Check error message
# 2. Verify cluster health (kubectl cluster-info)
# 3. Review logs (kubectl logs -f deployment/xxx)
# 4. If recoverable: fix and retry (make deploy)
# 5. If not: rollback (make rollback target=staging|prod)
# 6. If rollback fails: Warmonger involved (manual recovery)
```

### Emergency 3: Service Down (Post-Release)

```bash
# If service crashes after release:
# 1. Page on-call engineer
# 2. Verify health endpoints (curl /health)
# 3. Check logs for errors
# 4. If error clear: fix and re-deploy
# 5. If error unclear: rollback to previous version
# 6. Post-incident: root cause analysis
```

### Emergency 4: Data Corruption

```bash
# If data corruption detected:
# 1. Alert Warmonger immediately
# 2. Stop all writes (set to read-only)
# 3. Verify backup integrity
# 4. Restore from backup (last known good state)
# 5. Replay audit logs to recover data
# 6. Verify data integrity after restore
# 7. Root cause analysis + prevent recurrence
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Agent | Role | Phases | Responsibility |
|-------|------|--------|-----------------|
| A | DevOps / Release | All | Verification, commits, milestone gates, final sign-off |
| B | QA / Test | 0, 7, 10 | E2E test suite design, execution, verification |
| C | Security / Auth | 1, 3, 4 | Auth hardening, Lich campaigns, remediation |
| D | Security / Injection | 3, 6 | Injection testing, container security |
| E | Security / RBAC | 1, 6 | RBAC implementation, network policies, container runtime |
| F | Documentation / Release | 5, 9, 10 | Auth configuration, docs, release notes, announcements |
| G | Transport / mTLS | 2, 5 | mTLS implementation, cert management, Wotan hardening |
| H | Deployment / Ops | 2, 8 | Deployment pipeline, health checks, rollback, monitoring |
| W | Warmonger | All | Battle plan, phase gates, emergency decisions, final approval |

**Agent Availability**:
- Phases 1-5: Agents work in parallel (dependencies managed)
- Phase 6-7: Phases 1-5 must be mostly complete
- Phase 8: Phases 1-7 must be complete
- Phase 9: Ongoing throughout, finalized last
- Phase 10: Final gate, all agents contribute

**Escalation Path**:
- Agent encounters blocker → escalate to Warmonger
- Warmonger makes decision / provides guidance
- Resume with new approach

---

## APPENDIX C: QUICK REFERENCE

### Port Registry

| Service | HTTP | gRPC | Purpose |
|---------|------|------|---------|
| Gateway | 21000 | N/A | Public API (redirects to HTTPS) |
| Gateway | 21443 | N/A | Public API (secure) |
| Wotan | 18000 | 18001 | Message broker (HTTP cascade for debugging) |
| Dashboard | 20000 | N/A | Admin UI (redirects to HTTPS) |
| Dashboard | 20443 | N/A | Admin UI (secure) |
| Auth Service | 17001 | 17001 | Auth RPC |
| Threat Engine | 15001 | 15001 | Threat processing RPC |
| Vector Service | 16001 | 16001 | Vector operations RPC |
| Observation Engine | 19001 | 19001 | Observation RPC |
| [Others] | 1xNNN | 1xNNN | Internal services (all gRPC) |

### Auth Configuration

**JWT**:
- Issuer: `unheaded.local`
- Audience: `unheaded-services`
- Expiry: 8 hours (user), 24 hours (service)
- Key rotation: 30 days
- Algorithm: RS256 (RSA-2048)
- Public key: `/etc/unheaded/jwt-public.key`
- Private key: `/etc/unheaded/jwt-private.key` (secret)

**API Key**:
- Format: alphanumeric, min 32 chars
- Storage: hashed in vault
- Rotation: 90 days
- Usage: X-API-Key header (HTTP) or metadata (gRPC)

**Service Token**:
- Type: JWT
- Expiry: 24 hours
- Rotation: automatic on service startup
- Usage: gRPC metadata (authorization: bearer)

**RBAC Roles**:
- `admin`: all operations
- `operator`: deploy, scale, restart
- `observer`: read-only
- `service`: service-to-service
- `guest`: public endpoints only

### Transport Configuration

**gRPC**:
- Protocol: gRPC over HTTP/2
- TLS: mTLS required (client + server cert)
- Cipher suite: `TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384` (minimum)
- Certificate validation: strict (no self-signed in prod)
- Certificate path: `/etc/unheaded/pki/${service}.{crt,key}`

**HTTP/HTTPS**:
- Port 21000 (gateway): redirect to 21443
- Port 21443 (gateway): TLS 1.2+ required
- Port 20000 (dashboard): redirect to 20443
- Port 20443 (dashboard): TLS 1.2+ required
- HSTS header: `Strict-Transport-Security: max-age=31536000`
- Certificate pinning: not enforced (may add later)

### Logging Configuration

**Centralized Logging**:
- Sink: Wotan (gRPC 18001)
- Format: JSON (structured)
- Fields: timestamp, level, service, message, context
- Retention: 30 days
- Output: `/var/log/unheaded/app-logs.jsonl`

**Audit Logging**:
- Path: `/var/log/unheaded/audit.log`
- Fields: timestamp, event_type, user, service, action, result
- Retention: 90 days
- Critical events: all auth, all deployments, all config changes

**Debug Logging** (development only):
- Level: DEBUG
- Output: stdout
- Retention: not persisted

### Metrics Configuration

**Prometheus Endpoints**:
- All services: `/debug/metrics` (port gRPC + 1000)
- Format: Prometheus text format
- Scraped by: Prometheus (every 15s)
- Retention: 15 days

**Key Metrics**:
- `http_requests_total` (counter)
- `grpc_requests_total` (counter)
- `request_duration_seconds` (histogram)
- `container_memory_usage_bytes` (gauge)
- `container_cpu_usage_cores` (gauge)
- `wotan_messages_delivered_total` (counter)
- `threats_processed_total` (counter)

### Health Check Endpoints

**Readiness** (`/readiness`):
- Service is ready to receive traffic
- All dependencies healthy
- Startup tasks complete

**Liveness** (`/liveness`):
- Service is alive (heartbeat)
- Graceful shutdown in progress → fail (to trigger restart)

**Status** (`/status` or `/health`):
- Current health state (healthy, degraded, down)
- Dependency status
- Version + commit info

### Deployment Targets

**Development**:
- `make deploy target=dev` → docker-compose up
- Duration: ~2 minutes
- Health checks: basic (all services up)

**Staging**:
- `make deploy target=staging` → K8s staging namespace
- Duration: ~10 minutes
- Health checks: full E2E tests

**Production**:
- `make deploy target=prod` → K8s prod namespace (with canary)
- Duration: ~15 minutes (including canary)
- Health checks: full E2E tests + metrics validation
- Requires manual approval (Warmonger or lead)

### Common Make Targets

```bash
make build-all              # Build all services
make test                   # Unit tests
make race-test              # Race detection
make e2e-test               # E2E integration tests
make lich-all               # Security campaigns (D1-D6)
make load-test-wotan        # Load test (10K msg/sec)
make deploy target=staging  # Deploy to staging
make rollback target=staging # Rollback staging
make docker-compose-up      # Local dev environment
```

### Emergency Contacts

- **Warmonger**: Decide on critical issues, phase gates, release approval
- **Security Team**: Security findings, audit, compliance
- **Ops Team**: Deployment, health checks, monitoring
- **On-Call**: Page for production incidents
- **Escalation**: All emergencies go to Warmonger first

---

## FINAL BATTLE CRY

### S39 COMPLETE: INDUSTRIALIZATION DONE

**Timeline**: ~4 weeks (5 working days/week, parallel agents)
- Week 1: Phases 0-2 (verification, auth, mTLS)
- Week 2: Phases 3-5 (security, Wotan)
- Week 3: Phases 6-8 (container, tests, deployment)
- Week 4: Phases 9-10 (docs, release)

**Deliverables**:
- v0.1.0-alpha tag (production-ready)
- 20+ service binaries (all secured)
- Complete documentation (API, architecture, security, ops, dev)
- Compliance snapshot (all standards met)
- Release artifacts (signed, checksummed)

**Sacred Laws Upheld**:
- ZERO customer data access (architecture prevents it)
- Security first (every decision prioritizes security)
- TDD (all phases include tests)
- Race detection (go test -race on all code)
- Interchangeable backends (pluggable implementations)

**Known Limitations** (for S40+):
- No distributed tracing (Jaeger integration planned)
- No service mesh UI (Kiali integration planned)
- No auto-scaling (HPA policies not yet configured)
- No multi-region (single cluster focus)
- No chaos engineering tests (yet)

**What's Next** (Post-S39):
- S40: Performance optimization (latency, throughput)
- S41: Advanced observability (tracing, profiling)
- S42: Multi-region deployment
- S43: Chaos engineering + resilience
- S44: Scaling to 1M events/sec
- S45: AI-powered threat detection (optional)

**This is not the end. This is the beginning.**

**v0.1.0-alpha is SHIP READY.**

**WARMONGER SEAL: APPROVED FOR RELEASE**

Date: 2026-02-24
Authority: Unheaded War Master
Status: READY TO DEPLOY

---

*End of S39 Industrialization Battle Plan*

