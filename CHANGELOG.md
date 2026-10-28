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
  - Corrected 2026-10-27: the middleware was wired into each service, but with `AUTH_ENABLED` unset it installs a no-op authenticator and the deployed stack did not set it; authentication was enforced only from S-IRON-GATE (see Unreleased).

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

- All 8 application services enforce JWT/API key authentication (corrected 2026-10-27: wired, off by default; see Authentication above)
- RBAC enforced on all API endpoints
- Audit logging records all authentication events
- Container images run as non-root (uid 1000) (since 2026-10-18 each service has its own uid, `pkg/uids`)
- Read-only root filesystem prevents runtime tampering
- All Linux capabilities dropped except NET_BIND_SERVICE
- seccomp profiles restrict available syscalls
- No secrets in code, environment, or logs

### Known Issues

- eBPF programs require Linux 6.x kernel with BTF support
- Live E2E tests require Docker Compose environment
- TLS certificate rotation requires manual PKI initialization

## [Unreleased] - v0.2.0-alpha (not tagged)

What `develop` holds since v0.1.0-alpha, by area. Sources: the sprint
battle plans and progress logs under `docs/battle-plans/`, ADRs as cited.
Sprints S-IRON-GATE and S-WELL-SPRING are in progress; their live (host)
halves wait on Stevie's escalations.

### Added

- **Service identity and auth (ADR-103, S-IRON-GATE)**: Ed25519 service tokens (EdDSA, 5-minute, per-target audience), one key per service (`unheaded-keygen`), `trust.d` binding each identity to its public key, per-service caller allowlists (403), `uhk_` API keys (hashed; memory, file or Well store), OIDC user tokens verified by JWKS, built-in issuer `unheaded-auth` (port 19020), dashboard and kanban as BFFs with HttpOnly session cookies. All 13 first-party services run with auth on in compose and units; Wotan verifies tokens and auto-approves verified identities, not display names. HS256 is test-only.
- **Internal TLS (ADR-105, FND-017)**: with `UNHEADED_TLS_ENABLED=true` every listener and first-party link is TLS 1.3 with hybrid ML-KEM (`X25519MLKEM768`), no plaintext fallback; dev CA and per-service certificates from `scripts/gen-dev-certs.sh`.
- **Signed topics**: ML-DSA-65 signatures required on `config.`, `drift.`, `audit.`, `alerts.critical.`, `system.health.consensus.`, `system.health.remediate.`; producers sign or refuse.
- **Post-quantum work (S-QUANTUM-ARMOR, ADR-105)**: hybrid key exchange profiles in `pkg/tls/pq.go`, rustls on aws-lc-rs for the WAF and zhend, a CycloneDX 1.6 crypto inventory (`compliance/crypto/cbom.cdx.json`) with gates, `yggdrasil-evidence` (ML-DSA-65 evidence seals); SLH-DSA (FIPS 205) via cloudflare/circl.
- **Deploy and chaos (S-REAL-STEEL, ADR-107)**: Cuirass drives LXD over HTTPS with a restricted client certificate; Sword (19011) runs rolling, blue-green, canary and recreate with canary analysis and automatic rollback; Yaldabaoth chaos engine in `cmd/chaos-controller` (19012) inside an allowlist, three abort paths and a revert journal. First game day: a degraded canary rolled itself back in 62.4 s.
- **Health and alerting**: Akira (19100) health consensus per ADR-029, exec checks (cert expiry), signed consensus and remediation topics; Heimdall drift monitor unit; alert rules each with a runbook URL; a dead-man heartbeat.
- **The Well**: PostgreSQL 16 on west, three databases with least-privilege and NOLOGIN owner roles, TLS 1.3 only, migrations through `cmd/well-migrate` with a ledger, nightly backups with restore drills and an offsite copy on east, a retention job (`well-retention`).
- **Zhen**: Champion gate (trust allowlist, requester-bound confirm tokens, write cap, audit fail-closed, openat2 sandbox), one shared prompt and verb table (`pkg/zhenrag`), runbook manifest and model SHA pins, `zhen-agentd` with scheduler, worker and overnight modes, Go MCP server (`cmd/zhen-mcp`), Zhen/Claude routing with privacy tiers (S-TWO-BRAINS, ADR-106), Gemma-4 LoRA training in `zhenai-forge` (ADR-048..050).
- **Compliance (ADR-097/098)**: host baseline agent (`unheaded-baseline`, audit mode) with an enforcing gate in every package's preinst; findings register; Lich red-team campaigns and fuzz harnesses.
- **UPC (ASCEND-LINUX, S5)**: xv6 boots on the UPC to an interactive shell that runs `ls`, `cat`, `echo`, `wc` against its own disk image (kernel 100 % Unheaded-authored); signed MBC cartridges run per flow (`ebpf/cartridge-ebpf`, `cmd/upc-cartridgectl`, ADR-108).
- **Release and packaging (S-IRON-GATE Phases 12, 14)**: `release.yml` builds every binary, deb and eBPF object on a tag, signs every file with keyless cosign, attests the SBOM and adds build provenance (`scripts/verify-release.sh`); Scorecard and CodeQL workflows. One deb per binary a unit runs, from one table (`scripts/lib/deb-packages.sh`), plus `unheaded-keygen` and `unheaded-configs`; a signed apt repository for west (`scripts/apt-repo.sh`, port 18888).
- **Observability**: `unheaded_http_*` metrics on every service through `pkg/httputil`, Huginn host metrics agent, first-party containers log through journald.

### Changed

- Every service runs as its own nologin account and uid (`deploy/sysusers.d`, `pkg/uids`), in compose and in systemd; no shared `unheaded` account.
- Container images are pinned to exact versions, the remaining floating set may only shrink (ADR-102, `scripts/check-image-tags.sh`).
- Packages: `unheaded-unheaded-daemon` is now `unheaded-daemon`; units ship under the repository's names (`deploy/systemd/README.md`).
- Branches: work lands on `develop`, promoted to `staging` and then `main` (ADR-089).

### Removed

- The mTLS service-mesh package `pkg/transport/mtls` and the uncalled transport `Connect()` cascade (2026-10-26); 108 unreached packages (ADR-090).
- Suricata IDS (2026-10-22).

### Not yet

- Hosts on the new auth, TLS and debs: install windows and the apt signing key are Stevie's (S-IRON-GATE E3, E4); the evidence build key (E1) and drift keys (E2).
- Public access with optional auth (M-035, M-055); the packet-to-browser p99 benchmark (M-089).
- IPv6 header-space transport, Kubernetes/Helm as a supported deployment, and observability backend adapters beyond Prometheus, Loki and Grafana remain plans.

