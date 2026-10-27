# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [v0.1.0-alpha] - 2026-02-24

### Added

- **Authentication**: JWT + API key + service token authentication on all endpoints
  - HMAC-SHA256 JWT signing with configurable expiry
  - API key validation with rate limiting
  - Service-to-service token authentication
  - RBAC enforcement with 5 roles: admin, operator, observer, service, guest
  - Structured JSON audit logging for all auth events

- **mTLS library** (`pkg/transport/mtls/`): ECDSA P-256 root CA, per-service certificates, CertRotator, server/client TLS config factories
  - Corrected 2026-10-27: this entry used to say "mutual TLS between all services". Only Wotan cluster replication uses the package; internal service links are plaintext. Internal TLS is tracked as FND-017 / S-IRON-GATE.

- **Lich Security Test Suite**: 6 offensive security campaigns (D1-D6)
  - D1: Auth bypass (10 tests)
  - D2: Injection attacks (8 tests)
  - D3: Transport security (6 tests)
  - D4: Secrets management (6 tests)
  - D5: Privilege escalation (5 tests)
  - D6: Denial of service (5 tests)
  - Markdown report generation

- **Wotan Message Delivery Hardening**
  - PublishWithAck: acknowledgment-based publishing with retry
  - Exponential backoff retry (100ms, 200ms, 400ms, 3 attempts)
  - Dead letter queue for exhausted retries
  - OrderedPublisher: per-destination FIFO message delivery
  - IdempotencyCache: duplicate message detection with 24h TTL
  - PublishWithTimeout: 30s per-message delivery deadline
  - Prometheus metrics for all reliability features

- **Container Security**
  - Read-only root filesystem on all Docker Compose services
  - Capability dropping (ALL dropped, NET_BIND_SERVICE added)
  - no-new-privileges security option
  - seccomp profile with minimal syscall whitelist
  - AppArmor profile template
  - NixOS hardening module (hardening.nix)
  - Containerd OCI specs with security constraints

- **E2E Integration Tests**
  - Full pipeline tests (auth, health, metrics, trace propagation)
  - Security E2E tests (auth enforcement, token validation)
  - Performance E2E tests (latency, throughput)
  - Kanban smoke tests

- **Deployment Pipeline**
  - `make deploy` with build, test, compose up, health check
  - `make deploy-health` for service health verification
  - `make deploy-rollback` for quick rollback
  - `make test-e2e` for full E2E test suite execution
  - `make test-security` for Lich security campaigns

- **Infrastructure**
  - Port Authority: Doom Range 16666-26666 for all services
  - gRPC-first transport with HTTP cascade fallback
  - Centralized log aggregation with ring buffer and zerolog hook
  - 4-layer service discovery (Wotan/port-scan/convention/static)
  - Docker Compose multi-network fabric (control/data/observe)
  - LXD container profiles with kernel hardening
  - NixOS container definitions for all services

### Security

- All 8 application services enforce JWT/API key authentication
- RBAC enforced on all API endpoints
- Audit logging records all authentication events
- Container images run as non-root (uid 1000)
- Read-only root filesystem prevents runtime tampering
- All Linux capabilities dropped except NET_BIND_SERVICE
- seccomp profiles restrict available syscalls
- No secrets in code, environment, or logs

### Known Issues

- eBPF programs require Linux 6.x kernel with BTF support
- Live E2E tests require Docker Compose environment
- TLS certificate rotation requires manual PKI initialization

## [Unreleased]

- S-CLEAN-SLATE (2026-10-27): CI green on `develop`, Wotan admin key and client resilience, live-service fixes, public-repo security items, documentation corrected against the code (`docs/battle-plans/S-CLEAN-SLATE-battle-plan.md`). The full rebuild of this file from sprint summaries is M-064 (S-IRON-GATE).
- IPv6 header-space transport (experimental)
- Kubernetes manifests and Helm charts
- Production deployment automation
- Observability backend adapters (Grafana, Jaeger, ELK)
