# Unheaded Kingdom — Claude Code Task List

**Generated:** February 6, 2026
**Source of Truth:** timeline.md, CLAUDE.md, codebase exploration
**Purpose:** Machine-parseable task list for Claude Code agents to execute autonomously
**Project Root:** `~/tmp/unheaded/`

---

IP="$(tart ip ubuntu)"
echo "VM IP: $IP"
route -n get "$IP" | egrep 'interface|gateway'

ssh admin@$(tart ip ubuntu)

route -n get "$(tart ip ubuntu)" | egrep 'interface|gateway'


## How To Use This File

Each task block follows this structure for easy agent consumption:

```
### TASK-{ID}: {Title}
- **Priority:** P0/P1/P2/P3
- **Parallelizable:** Yes/No
- **Dependencies:** TASK-{IDs} or None
- **Scope:** {files/directories affected}
- **Acceptance Criteria:** {what "done" looks like}
- **Commands:** {verification commands}
- **Estimated Effort:** S/M/L/XL
```

Agents should read `CLAUDE.md` before starting any task.

---

## PHASE A: CRITICAL BLOCKERS & ALPHA COMPLETION

These tasks close the last 2% to Alpha. Execute in priority order.

---

### TASK-001: Kanban E2E Smoke Test — Manual Verification Script

- **Priority:** P0 — SHIP BLOCKER
- **Parallelizable:** No (must verify full stack)
- **Dependencies:** None
- **Scope:** `tests/e2e/kanban_smoke_test.go`, `cmd/kanban-app/`
- **Acceptance Criteria:**
  - Kanban-app binary starts and serves on expected port
  - Board loads in browser (HTTP GET returns 200 with HTML)
  - Create task via API → task appears on board
  - Move task between columns via API → state persists
  - Complete task → verify persistence in timeline.md format
  - Verify Busboy event published for each mutation
  - All 8 existing E2E tests pass: `go test ./tests/e2e/... -v`
- **Commands:**
  ```bash
  cd ~/tmp/unheaded
  go build ./cmd/kanban-app/
  go test ./tests/e2e/... -v -count=1 -race
  ```
- **Estimated Effort:** M

---

### TASK-002: Dashboard Frontend Polish — 85% → 95%

- **Priority:** P0
- **Parallelizable:** Yes (with TASK-001)
- **Dependencies:** None
- **Scope:** `cmd/dashboard-backend/`, `dashboard/`, `kanban/`
- **Acceptance Criteria:**
  - Responsive layout works at 1024px, 1440px, 1920px breakpoints
  - Kingdom theming consistent: dark bg (#1a1a2e), gold accents (#ffd700)
  - Loading skeleton states for all async data
  - Error states display meaningful messages (not raw JSON)
  - WebSocket reconnection indicator visible in UI
  - Accessibility: keyboard nav (Tab/Enter/Escape), ARIA labels, WCAG AA contrast
  - No external CDN/font dependencies (all self-hosted, `embed` directive)
- **Commands:**
  ```bash
  cd ~/tmp/unheaded
  go build ./cmd/dashboard-backend/
  go build ./cmd/kanban-app/
  ```
- **Estimated Effort:** L

---

## PHASE B: TEST COVERAGE — ARMOR PIECE SERVICES

15 services have ZERO test files. Each task below is independently parallelizable. All follow the same pattern: write comprehensive tests for the service, achieve 80%+ coverage, run with `-race`.

**Pattern for each:** Read the service's `.go` file, understand its interfaces, write `_test.go` with table-driven tests, edge cases, error paths, and race-safe concurrent tests.

**Global Acceptance Criteria for all TASK-01x:**
- `go test ./services/{name}/... -v -race -coverprofile=cover.out`
- `go tool cover -func=cover.out` shows ≥ 80% coverage
- Zero race conditions detected
- Tests are self-contained (no external deps, mock everything)

---

### TASK-010: Tests for Shield the WAF

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/shield/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, WAF rule matching, DDoS rate limiting, IP blocklist, request filtering, XSS/SQLi detection paths tested
- **Estimated Effort:** M

---

### TASK-011: Tests for Sword the Deploy Pipeline

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/sword/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, canary deployment logic, blue-green switching, rolling update sequencing, rollback paths tested
- **Estimated Effort:** M

---

### TASK-012: Tests for Cuirass the Control Plane

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/cuirass/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, state machine transitions, drift detection, reconciliation loop, health aggregation tested
- **Estimated Effort:** M

---

### TASK-013: Tests for Hauberk the Service Mesh

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/hauberk/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, service discovery, circuit breaker states (closed/open/half-open), mTLS handshake mocking, config propagation tested
- **Estimated Effort:** M

---

### TASK-014: Tests for Pauldrons the Load Balancer

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/pauldrons/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, L4/L7 routing, Maglev consistent hashing, session persistence, backend health checks, weighted round-robin tested
- **Estimated Effort:** M

---

### TASK-015: Tests for Vambraces the Observability Stack

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/vambraces/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, metric collection, alert rule evaluation, SLO calculation, trace correlation tested
- **Estimated Effort:** M

---

### TASK-016: Tests for Gauntlets the CLI & API

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/gauntlets/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, CLI command parsing, API endpoint parity (Gauntlets Law verified), input validation, output formatting tested
- **Estimated Effort:** M

---

### TASK-017: Tests for Tassets the Data Layer

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/tassets/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, data pipeline operations, backup/restore, storage abstraction, encryption at rest tested
- **Estimated Effort:** M

---

### TASK-018: Tests for Sabatons the Bare Metal Agent

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/sabatons/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, PXE boot sequence, hardware inventory, IPMI interface mocking, provisioning state machine tested
- **Estimated Effort:** M

---

### TASK-019: Tests for Cape the Internal Framework

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/cape/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, HTTP handler registration, middleware chain, WebSocket upgrade, static asset serving tested
- **Estimated Effort:** M

---

### TASK-020: Tests for Cloak the User Dashboard

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/cloak/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, dashboard data aggregation, user session management, widget rendering logic, real-time update subscription tested
- **Estimated Effort:** M

---

### TASK-021: Tests for Timeguru Service

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/timeguru/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, timeline markdown parsing, JSON/YAML mirror generation, file watcher, REST API endpoints, Busboy event publishing tested
- **Estimated Effort:** M

---

### TASK-022: Tests for Micromanager Service

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/micromanager/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, task CRUD, progress tracking, priority sorting, blocker detection, QA gate logic tested
- **Note:** Some test files may already exist (service_test, api_test, store_test, task_test) — verify existing coverage first, augment to 80%+
- **Estimated Effort:** S

---

### TASK-023: Tests for Architect Service

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/architect/`
- **Acceptance Criteria:** 80%+ coverage, race-safe, ADR management, design review workflow, health monitoring, Busboy integration tested
- **Note:** Some test files may already exist (core_test, handlers_test) — verify existing coverage first, augment to 80%+
- **Estimated Effort:** S

---

### TASK-024: Tests for Busboy Service Integration

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `services/busboy/`
- **Acceptance Criteria:** 80%+ coverage for the service integration layer (not the standalone busboy repo), pub/sub wiring, topic routing, message serialization tested
- **Estimated Effort:** S

---

## PHASE C: TEST COVERAGE — pkg/ PACKAGES (NO TESTS)

9 packages under `pkg/` have no test files. Each is independently parallelizable.

---

### TASK-030: Tests for pkg/busboy-client

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `pkg/busboy-client/`
- **Acceptance Criteria:** 80%+ coverage, gRPC stream mocking, reconnection logic, backpressure handling, message serialization tested
- **Estimated Effort:** M

---

### TASK-031: Tests for pkg/lxd

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `pkg/lxd/`
- **Acceptance Criteria:** 80%+ coverage, REST API client mocking, container lifecycle (create/start/stop/delete), snapshot management, retry logic tested
- **Estimated Effort:** M

---

### TASK-032: Tests for pkg/netlink

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `pkg/netlink/`
- **Acceptance Criteria:** 80%+ coverage, netlink message encoding/decoding, RTNetlink operations, XDP attachment, TC filter management tested (mock syscalls)
- **Estimated Effort:** L

---

### TASK-033: Tests for pkg/network

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `pkg/network/`
- **Acceptance Criteria:** 80%+ coverage, iptables/nftables rule generation, default-deny enforcement, policy CRUD, violation tracking tested
- **Estimated Effort:** M

---

### TASK-034: Tests for pkg/nix

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `pkg/nix/`
- **Acceptance Criteria:** 80%+ coverage, flake parsing, NixOS build command generation, LXD image import, build queue ordering tested
- **Estimated Effort:** M

---

### TASK-035: Tests for pkg/state

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `pkg/state/`
- **Acceptance Criteria:** 80%+ coverage, state reconciliation (Pleroma vs Kenoma pattern), drift detection, action generation, rate limiting, event emission tested
- **Estimated Effort:** M

---

### TASK-036: Tests for pkg/telemetry

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `pkg/telemetry/`
- **Acceptance Criteria:** 80%+ coverage, span creation, context propagation, trace ID generation, OTLP export mocking tested
- **Estimated Effort:** M

---

### TASK-037: Tests for pkg/tracing

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `pkg/tracing/`
- **Acceptance Criteria:** 80%+ coverage, OTLP-compatible collection, kernel event correlation, memory-efficient storage, concurrent trace ingestion tested
- **Estimated Effort:** M

---

## PHASE D: NixOS CONTAINER HARDENING

These tasks harden the NixOS container definitions. Parallelizable.

---

### TASK-040: NixOS Container Definitions — Hardening Audit

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `nix/containers/*.nix`, `nix/modules/hardening.nix`
- **Acceptance Criteria:**
  - Every container definition includes: CapabilityBoundingSet, NoNewPrivileges, PrivateTmp, ProtectSystem=strict, ProtectHome, ReadOnlyPaths, SystemCallFilter (seccomp)
  - PrivateDevices, ProtectKernelTunables, ProtectControlGroups, RestrictRealtime, RestrictNamespaces applied
  - Default-deny network policy in every container
  - Only explicitly required ports allowed
  - Shared hardening module imported by all containers
  - Run `nix flake check` (if nix available) or validate syntax
- **Commands:**
  ```bash
  cd ~/tmp/unheaded
  # Validate nix syntax
  nix-instantiate --parse nix/flake.nix 2>&1 || echo "Nix not available, manual review"
  ```
- **Estimated Effort:** L

---

### TASK-041: NixOS Container Definitions — Service Completeness

- **Priority:** P1
- **Parallelizable:** Yes (with TASK-040)
- **Dependencies:** None
- **Scope:** `nix/containers/`, `nix/packages/`
- **Acceptance Criteria:**
  - Every service in `services/` has a corresponding container in `nix/containers/`
  - Every cmd binary in `cmd/` has a corresponding package in `nix/packages/`
  - Container networking uses 10.10.10.0/24 bridge (lxdbr0)
  - Container test file (`nix/tests/container-tests.nix`) covers all containers
  - Missing containers created following existing patterns
- **Estimated Effort:** L

---

## PHASE E: INTEGRATION & CROSS-CUTTING CONCERNS

---

### TASK-050: Busboy Integration Wiring — All Services

- **Priority:** P1
- **Parallelizable:** No (integration)
- **Dependencies:** TASK-010 through TASK-024 (tests should exist first)
- **Scope:** `services/*/`, `pkg/busboy-client/`
- **Acceptance Criteria:**
  - Every service connects to Busboy on startup
  - Every service subscribes to `alerts.critical` topic
  - Every service publishes state changes to service-specific topics
  - Every service includes `trace_id` in all Busboy messages
  - Every service disconnects gracefully on SIGTERM/SIGINT
  - Integration test verifies message flow between at least 3 services
- **Commands:**
  ```bash
  cd ~/tmp/unheaded
  go build ./...
  go test ./... -v -race -count=1
  ```
- **Estimated Effort:** L

---

### TASK-051: Health & Ready Endpoint Audit

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** All `services/*/`, all `cmd/*/`
- **Acceptance Criteria:**
  - Every service exposes GET `/health` (200 if healthy)
  - Every service exposes GET `/ready` (200 if ready to serve)
  - Every service exposes GET `/metrics` (Prometheus format)
  - Health checks verify downstream dependencies (Busboy connection, storage)
  - Tests verify health/ready/metrics endpoints for each service
- **Estimated Effort:** M

---

### TASK-052: Gauntlets Law Verification — CLI ↔ API Parity

- **Priority:** P2
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `cmd/unheaded-cli/`, `services/gauntlets/`, all API endpoints
- **Acceptance Criteria:**
  - Every REST API endpoint has a corresponding CLI command
  - CLI calls API (not direct service logic)
  - Test matrix: for each API endpoint, verify CLI command exists and produces equivalent result
  - Document any gaps as TODO items
- **Estimated Effort:** L

---

### TASK-053: Cross-Service Health Monitoring — Percentage-Based Consensus

- **Priority:** P2
- **Parallelizable:** Yes
- **Dependencies:** TASK-051
- **Scope:** `pkg/health/`, `services/*/`
- **Acceptance Criteria:**
  - Each service health-checks all services it depends on
  - Failures reported to Busboy topic `system.outage.reports`
  - Severity calculated by percentage formula: `(unique_reporters / total_dependent_services) × 100`
  - Thresholds: OK (0-12.49%), WARN (12.5-37.49%), ERROR (37.5-62.49%), CRITICAL (62.5-87.49%), PANIC (87.5-100%)
  - Tests verify severity calculation at each threshold boundary
- **Estimated Effort:** L

---

## PHASE F: SECURITY HARDENING

---

### TASK-060: Security Audit — Input Validation Sweep

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** All `services/*/`, all `cmd/*/`, `pkg/http/`
- **Acceptance Criteria:**
  - All HTTP handlers validate Content-Type
  - All request body parsers enforce size limits (e.g., `http.MaxBytesReader`)
  - All user-supplied strings sanitized with `html.EscapeString` before rendering
  - All command execution uses temp file + whitelisted interpreters (no `os/exec` with user input)
  - All SQL/query parameters use parameterized queries (no string concatenation)
  - Fuzz tests added for at least 3 critical input paths
- **Commands:**
  ```bash
  cd ~/tmp/unheaded
  grep -rn 'os/exec' services/ cmd/ pkg/ | head -20
  grep -rn 'fmt.Sprintf.*%s.*sql' services/ cmd/ pkg/ | head -20
  ```
- **Estimated Effort:** L

---

### TASK-061: Security Audit — Zero User Data Access Verification

- **Priority:** P0
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** All source code
- **Acceptance Criteria:**
  - Architectural review confirms no code path allows platform engineers to access user data
  - User data zones are network-segmented (separate VPC/VLAN config)
  - No shared credentials between platform and user zones
  - Observability sees METRICS not DATA (packet counts, not packet contents)
  - User data never appears in logs (grep for PII patterns)
  - Document verification in `docs/SECURITY_AUDIT.md` with date stamp
- **Estimated Effort:** M

---

### TASK-062: TLS & Secrets Configuration

- **Priority:** P1
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `pkg/secrets/`, `pkg/certs/`, `nix/modules/`
- **Acceptance Criteria:**
  - TLS 1.3 minimum enforced for all external traffic
  - mTLS configured for inter-service communication
  - SOPS + age encryption used for all secrets
  - Secrets mounted as files (not env vars)
  - Certificate rotation automated (short-lived certs)
  - Tests verify TLS handshake requirements
- **Estimated Effort:** M

---

## PHASE G: eBPF — THE WHISPERING VOID (LINUX ENV REQUIRED)

These tasks are BLOCKED on B1 (Linux/eBPF dev environment). Execute when unblocked.

---

### TASK-070: eBPF Packet Marker — Build & Verify

- **Priority:** P1 (blocked)
- **Parallelizable:** Yes (with TASK-071, 072, 073)
- **Dependencies:** B1 — Linux dev environment
- **Scope:** `ebpf/packet-marker/`
- **Acceptance Criteria:**
  - Rust Aya program compiles with `cargo build --release`
  - BPF verifier accepts program (no verifier errors)
  - Trace ID injection at XDP layer functional
  - Packet header parsing handles IPv4, IPv6, TCP, UDP
  - Ring buffer event emission verified
  - Unit tests pass
- **Estimated Effort:** L

---

### TASK-071: eBPF Flow Tracker — Build & Verify

- **Priority:** P1 (blocked)
- **Parallelizable:** Yes
- **Dependencies:** B1
- **Scope:** `ebpf/flow-tracker/`
- **Acceptance Criteria:**
  - TC program compiles and verifier accepts
  - Bidirectional flow tracking functional
  - TCP state machine correct
  - LRU map expiration works
- **Estimated Effort:** L

---

### TASK-072: eBPF Latency Probe — Build & Verify

- **Priority:** P1 (blocked)
- **Parallelizable:** Yes
- **Dependencies:** B1
- **Scope:** `ebpf/latency-probe/`
- **Acceptance Criteria:**
  - Kprobe attachment to tcp_sendmsg/tcp_recvmsg
  - RTT measurement accurate within 1µs
  - Trace ID correlation with packet marker
- **Estimated Effort:** M

---

### TASK-073: eBPF Trace Collector — Busboy Bridge

- **Priority:** P1 (blocked)
- **Parallelizable:** Yes
- **Dependencies:** B1, TASK-070
- **Scope:** `cmd/trace-collector/`, `ebpf/`
- **Acceptance Criteria:**
  - Rust binary reads from all eBPF ring buffers
  - Events correlated by trace_id
  - Events published to Busboy via gRPC streaming
  - Memory-efficient batching (configurable batch size/interval)
  - Graceful shutdown with buffer flush
- **Estimated Effort:** L

---

### TASK-074: eBPF Syscall Tracer — Security Audit Events

- **Priority:** P2 (blocked)
- **Parallelizable:** Yes
- **Dependencies:** B1
- **Scope:** `ebpf/syscall-tracer/`
- **Acceptance Criteria:**
  - Raw tracepoint attachment to sys_enter/sys_exit
  - Configurable syscall filtering (whitelist mode)
  - Security-relevant events flagged (exec, open, connect, bind)
  - Events emitted to ring buffer with trace_id
- **Estimated Effort:** M

---

## PHASE H: DOCUMENTATION & ALPHA PREP

---

### TASK-080: API Documentation — OpenAPI/Swagger Spec

- **Priority:** P2
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** All `services/*/`, `docs/api/`
- **Acceptance Criteria:**
  - OpenAPI 3.0 spec generated for every service
  - Every endpoint documented: method, path, request/response schema, error codes
  - Spec validates with `swagger-cli validate`
  - Published as `docs/api/openapi.yaml`
- **Estimated Effort:** L

---

### TASK-081: Architecture Decision Records — Gap Fill

- **Priority:** P2
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `docs/adr/`
- **Acceptance Criteria:**
  - ADRs exist for: Gnostic state management, Kingdom naming convention, eBPF Rust choice, No-external-deps policy, Busboy as message backbone, Vanilla JS frontend, Container hardening strategy
  - Follow standard ADR format: Status, Context, Decision, Consequences
  - Stored in Sage's Lair (docs/adr/)
- **Estimated Effort:** M

---

### TASK-082: Service Breakout Strategy — Pre-Work

- **Priority:** P3
- **Parallelizable:** Yes
- **Dependencies:** None
- **Scope:** `docs/SERVICE_BREAKOUT_STRATEGY.md`
- **Acceptance Criteria:**
  - Document identifies which services break out first
  - Go module boundaries defined for each service
  - Import path migration plan (github.com/unheaded/unheaded → github.com/unheaded/{service})
  - Shared package strategy (what stays in unheaded/pkg vs extracted)
  - Timeline aligned with Age 2 (Feb 16 - Mar 15)
- **Estimated Effort:** M

---

### TASK-083: Demo Video Script & Recording Prep

- **Priority:** P2
- **Parallelizable:** Yes
- **Dependencies:** TASK-001, TASK-002
- **Scope:** `docs/demo/`
- **Acceptance Criteria:**
  - Script covers: Kingdom intro, Kanban board demo, real-time updates, eBPF tracing (if available), architecture overview
  - Key demo flows documented step-by-step
  - Screenshots/assets prepared
  - Runtime target: 3-5 minutes
- **Estimated Effort:** S

---

## PHASE I: EARLY AGE 2 PREP (POST-ALPHA)

---

### TASK-090: Busboy Redundancy — HA Clustering Design

- **Priority:** P2 (critical for Age 2)
- **Parallelizable:** Yes
- **Dependencies:** Alpha completion
- **Scope:** `services/busboy/`, `pkg/busboy-client/`
- **Acceptance Criteria:**
  - Design doc: active-passive vs active-active analysis
  - Leader election mechanism chosen (Raft vs simple heartbeat)
  - State synchronization approach documented
  - Client reconnection with exponential backoff
  - Failover target: < 5s
  - Implementation plan with story-level breakdown
- **Estimated Effort:** L

---

### TASK-091: Real LXD Integration Testing

- **Priority:** P2
- **Parallelizable:** Yes
- **Dependencies:** B1 — Linux dev environment, TASK-040
- **Scope:** `pkg/lxd/`, `cmd/unheaded-daemon/`, `nix/`
- **Acceptance Criteria:**
  - unheaded-daemon creates real LXD containers from NixOS definitions
  - Container lifecycle (create, start, stop, delete) works end-to-end
  - Networking via lxdbr0 (10.10.10.0/24) verified
  - Service discovery works between containers
  - Drift detection catches manual container changes
- **Estimated Effort:** XL

---

### TASK-092: Load Testing Harness

- **Priority:** P2
- **Parallelizable:** Yes
- **Dependencies:** TASK-050
- **Scope:** `tests/load/`, `pkg/testing/`
- **Acceptance Criteria:**
  - Load test framework generates 1000 req/s sustained
  - Tests cover: Kanban CRUD, Busboy pub/sub, Gateway routing, WebSocket connections
  - Performance baselines recorded
  - Regression detection (alert if latency increases >20%)
  - Results output as JSON for dashboard consumption
- **Estimated Effort:** L

---

## QUICK REFERENCE — EXECUTION ORDER

### Wave 1 (NOW — Parallel, no deps)
`TASK-001` `TASK-002` `TASK-061`

### Wave 2 (Tests — ALL parallel, no deps)
`TASK-010` `TASK-011` `TASK-012` `TASK-013` `TASK-014` `TASK-015` `TASK-016` `TASK-017` `TASK-018` `TASK-019` `TASK-020` `TASK-021` `TASK-022` `TASK-023` `TASK-024`

### Wave 3 (pkg tests — ALL parallel, no deps)
`TASK-030` `TASK-031` `TASK-032` `TASK-033` `TASK-034` `TASK-035` `TASK-036` `TASK-037`

### Wave 4 (Integration — after Wave 2+3)
`TASK-040` `TASK-041` `TASK-050` `TASK-051` `TASK-052` `TASK-053`

### Wave 5 (Security & Docs — parallel)
`TASK-060` `TASK-062` `TASK-080` `TASK-081` `TASK-082` `TASK-083`

### Wave 6 (eBPF — BLOCKED on B1)
`TASK-070` `TASK-071` `TASK-072` `TASK-073` `TASK-074`

### Wave 7 (Age 2 Prep — post-alpha)
`TASK-090` `TASK-091` `TASK-092`

---

## BLOCKER REGISTER

| ID | Blocker | Impact | Owner | Status | Unblocks |
|----|---------|--------|-------|--------|----------|
| B1 | Linux/eBPF dev environment | HIGH | Muck | PENDING | TASK-070 through TASK-074, TASK-091 |

---

## METRICS TARGETS

| Metric | Current | Target | Gap |
|--------|---------|--------|-----|
| Alpha Progress | 98% | 100% | TASK-001, TASK-002 |
| Service Test Coverage | 8/23 with tests | 23/23 | Wave 2 |
| pkg/ Test Coverage | 23/32 with tests | 32/32 | Wave 3 |
| NixOS Container Coverage | ~35% hardened | 100% | Wave 4 |
| Security Audit | Partial | Complete | Wave 5 |
| eBPF Programs | 0/4 verified | 4/4 | Wave 6 (blocked) |

---

**THE KNIGHT IS NEVER WITHOUT ARMOR.**
**THE KINGDOM RISES.**

⚔️🛡️🏰
