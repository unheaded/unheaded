# S72-R REVISED BATTLE PLAN — Remaining Work After Swarm Execution

**Date**: 2026-02-27
**Sprint**: S72-R — Verification + Wiring + Build Validation
**Prerequisite**: S72 swarm complete (7 commits: f9d7b58..09844d8). Git clean on main.
**Target**: Compile all Go code, run all tests with race detector, wire auth+mTLS into every service binary, verify SBOM tooling, achieve >=80% auth coverage. Ship a green build.
**Estimated Duration**: 4-6 hours across 1-2 sessions (requires Go 1.24 toolchain)
**Agent Strategy**: Phase 1 sequential (toolchain). Phases 2-3 parallelizable (auth verify + mTLS wiring). Phase 4 sequential (integration). Phase 5 sequential (SBOM). Phase 6 sequential (final gate).
**Commit Cadence**: Every 4 steps (max(3, min(5, 95/20)) = 4)
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts. Log STUCK marker. Commit before skip.
**Hardware Required**: Machine with Go 1.24, Rust nightly, protoc. Bare metal NOT required.

---

## LEGEND

- `[B]` = Bash command (run directly)
- `[V]` = Verification step (MUST pass before proceeding)
- `[D]` = Debug step (only if prior step fails)
- `[W]` = Write/create file
- `[R]` = Read/inspect file
- `[S]` = Sudo required
- `[P]` = Parallelizable with other marked steps
- `[C]` = Commit checkpoint
- `[STUCK]` = Step skipped via Skip Protocol
- `[BLOCKED]` = Blocked by upstream STUCK

---

## WHAT'S DONE (DO NOT REPEAT)

```
✅ Phase 0:  Environment file checks (no toolchain — deferred to this plan)
✅ Phase 1:  Monad draft-05 (2,595 lines, patches M1-M8, Shield/Anamnesis/Kingdom)
✅ Phase 2:  Sophia draft-02 (2,120 lines, patches S1-S8, BPF maps/entry types/sync)
✅ Phase 3:  Wotan draft-02 (2,395 lines, patches W1-W8, ring buffer/gRPC/triple-role)
✅ Phase 4:  Cross-references (OpenAPI v2.0, alignment notes, status matrix, error registry)
✅ Phase 5A: Auth code written (jwt.go hardened, rbac_test.go expanded, gRPC interceptor, testutil)
✅ Phase 6A: mTLS integration.go written (SetupServiceTLS helper)
✅ Phase 8:  SBOM script + CI workflow + compliance gate doc
✅ Phase 9:  GHA hardened (8 workflows, permissions, timeouts, security.yml, sbom-audit.yml)
✅ Phase 10: Jenkins scaffolded (5 Jenkinsfiles + shared library + README)
✅ Phase 11: Host-A boot runbook + preflight + validation scripts
✅ Phase 12: Host-B boot runbook + WireGuard tunnel doc + cross-host validation
```

## WHAT REMAINS

```
❌ Go toolchain verification + go build + go test (original Steps 1-11)
❌ Auth test execution — code written but NEVER compiled/run (original Steps 84-109)
❌ Auth coverage gate >=80% (original Step 108)
❌ mTLS wiring into ALL service binaries (original Steps 126-130)
❌ Auth middleware wiring into ALL HTTP/gRPC routes (original Steps 137-146)
❌ Dev cert bootstrap script execution (original Steps 117-120)
❌ Proto regeneration if needed (original Steps 75-76)
❌ Full build + test + lint + security scan (original Steps 203-209)
❌ SBOM generation execution (original Step 150-152 run)
❌ Final verification gate (original Phase 13)
```

---

## PRIORITY MAP

```
P0: Toolchain + Build — get Go compiling                     (Phase 1)
P1: Auth Verification — run tests on written code             (Phase 2)
P2: mTLS + Auth Wiring — wire into all service binaries       (Phase 3)
P3: Integration Testing — full test suite with race detector  (Phase 4)
P4: SBOM + Lint + Security — tooling execution                (Phase 5)
P5: Final Verification Gate                                   (Phase 6)
```

---

# ═══════════════════════════════════════════════════════════
# PHASE 1: TOOLCHAIN VERIFICATION & FIRST BUILD (Steps 1-15)
# ═══════════════════════════════════════════════════════════

## PHASE 1: TOOLCHAIN + BUILD (Steps 1-15)

**Goal**: Verify Go 1.24, Rust, protoc are available. Get `go build ./...` passing.
**Prerequisite**: Git clean on main at commit 09844d8.
**Time**: ~15 minutes
**Agent**: Coordinator

- [ ] **Step 1** [B] ~30s: Verify Go toolchain
  ```bash
  go version | grep -q '1.2[4-9]' && echo "GO OK: $(go version)" || echo "GO MISSING — install Go 1.24+"
  ```

- [ ] **Step 2** [B] ~30s: Verify Rust toolchain
  ```bash
  rustc --version && cargo --version || echo "RUST MISSING"
  ```

- [ ] **Step 3** [B] ~30s: Verify protoc compiler
  ```bash
  protoc --version 2>/dev/null || echo "PROTOC MISSING — install protobuf-compiler"
  ```

- [ ] **Step 4** [B] ~30s: Verify golangci-lint
  ```bash
  golangci-lint --version 2>/dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
  ```

- [ ] **Step 5** [B] ~30s: Verify gosec
  ```bash
  gosec --version 2>/dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
  ```

- [ ] **Step 6** [B] ~30s: Verify syft (SBOM generator)
  ```bash
  syft version 2>/dev/null || echo "SYFT MISSING — curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin"
  ```

- [ ] **Step 7** [B] ~1m: Go mod tidy
  ```bash
  go mod tidy 2>&1 | tail -10
  ```

- [ ] **Step 8** [B] ~2m: Go build — first compile
  ```bash
  go build ./... 2>&1 | tail -20
  ```

- [ ] **Step 9** [V] ~30s: Build MUST pass
  - If pass → Step 12
  - If fail → Step 10

- [ ] **Step 10** [D] ~3m: Debug build failures
  ```bash
  go build ./... 2>&1 | head -50
  ```
  - Common: import cycle from new middleware_grpc.go, missing dependencies
  - Fix imports, run `go mod tidy`, rebuild

- [ ] **Step 11** [D] ~3m: If still failing, check specific packages
  ```bash
  go build ./pkg/auth/... 2>&1
  go build ./pkg/transport/mtls/... 2>&1
  go build ./cmd/... 2>&1
  go build ./services/... 2>&1
  ```

- [ ] **Step 12** [B] ~1m: Smoke test — run existing tests
  ```bash
  go test -count=1 -timeout 60s ./pkg/auth/... ./pkg/transport/... 2>&1 | tail -15
  ```

- [ ] **Step 13** [V] ~30s: Smoke tests pass (or expected failures from new code documented)
  - If pass → Step 14
  - If fail → Step 13a [D]: `go test -v -run "TestFailing" ./pkg/auth/... 2>&1 | tail -40`

- [ ] **Step 14** [C] ~30s: **COMMIT CHECKPOINT** — Build verified, smoke tests pass
  ```bash
  git add go.sum && git commit -m "[PLAN S72-R] Steps 1-14: Toolchain verified, build passes, smoke tests green" || echo "nothing to commit"
  ```

- [ ] **Step 15** [V] ~30s: **PHASE 1 EXIT GATE** — Go builds, tests run, lint/sec tools available
  - `go build ./...` passes → pass
  - `go test ./pkg/auth/... ./pkg/transport/...` passes → pass
  - If any fail → DO NOT PROCEED. Fix here.

---

# ═══════════════════════════════════════════════════════════
# PHASE 2: AUTH TEST EXECUTION & HARDENING VERIFICATION (Steps 16-40)
# ═══════════════════════════════════════════════════════════

## PHASE 2: AUTH VERIFICATION (Steps 16-40)

**Goal**: Run ALL auth tests written during swarm. Fix any compilation or logic errors. Achieve >=80% auth coverage.
**Prerequisite**: Phase 1 passed. `go build ./...` green.
**Time**: ~1.5 hours
**Agent**: Developer (TDD — tests already written, now verify they pass)

### 2A: JWT Tests

- [ ] **Step 16** [B] ~1m: Run JWT tests
  ```bash
  go test -v -run "TestJWT" ./pkg/auth/... 2>&1 | tail -30
  ```

- [ ] **Step 17** [V] ~30s: JWT tests pass
  - If pass → Step 20
  - If fail → Step 18

- [ ] **Step 18** [D] ~5m: Debug JWT test failures
  ```bash
  go test -v -run "TestJWT" ./pkg/auth/... 2>&1
  ```
  - Check: alg whitelist logic, claim validation, clock skew handling
  - Fix jwt.go and/or jwt_test.go

- [ ] **Step 19** [B] ~1m: Re-run JWT tests after fix
  ```bash
  go test -v -run "TestJWT" ./pkg/auth/... 2>&1 | tail -20
  ```

### 2B: RBAC Tests

- [ ] **Step 20** [B] ~1m: Run RBAC tests
  ```bash
  go test -v -run "TestRBAC" ./pkg/auth/... 2>&1 | tail -30
  ```

- [ ] **Step 21** [V] ~30s: RBAC tests pass
  - If pass → Step 24
  - If fail → Step 22

- [ ] **Step 22** [D] ~5m: Debug RBAC failures — check role hierarchy, endpoint matrix
  ```bash
  go test -v -run "TestRBAC" ./pkg/auth/... 2>&1
  ```

- [ ] **Step 23** [C] ~30s: **COMMIT CHECKPOINT** — JWT + RBAC verified
  ```bash
  git add pkg/auth/ && git commit -m "[PLAN S72-R] Steps 16-23: JWT + RBAC tests verified green" || echo "nothing to commit"
  ```

### 2C: Service Token + gRPC Interceptor Tests

- [ ] **Step 24** [B] ~1m: Run service token tests
  ```bash
  go test -v -run "TestServiceToken" ./pkg/auth/... 2>&1 | tail -20
  ```

- [ ] **Step 25** [V] ~30s: Service token tests pass
  - If fail → Step 25a [D]: Debug expiry, identity extraction, grace period logic

- [ ] **Step 26** [B] ~1m: Run gRPC interceptor tests
  ```bash
  go test -v -run "TestGRPC" ./pkg/auth/... 2>&1 | tail -20
  ```

- [ ] **Step 27** [V] ~30s: gRPC auth tests pass
  - If fail → Step 27a [D]: Check metadata extraction, context propagation

- [ ] **Step 28** [C] ~30s: **COMMIT CHECKPOINT** — Service token + gRPC interceptor verified
  ```bash
  git add pkg/auth/ && git commit -m "[PLAN S72-R] Steps 24-28: Service token + gRPC interceptor tests verified" || echo "nothing to commit"
  ```

### 2D: Full Auth Suite with Race Detector

- [ ] **Step 29** [B] ~2m: Run ALL auth tests with race detector
  ```bash
  go test -v -count=1 -race ./pkg/auth/... 2>&1 | tail -40
  ```

- [ ] **Step 30** [V] ~30s: All auth tests pass with -race
  - If fail → Step 30a [D]: Race detected — check audit logger mutex, rate limiter concurrency

- [ ] **Step 31** [B] ~1m: Check auth test coverage
  ```bash
  go test -coverprofile=/tmp/auth-coverage.out ./pkg/auth/... && go tool cover -func=/tmp/auth-coverage.out | tail -1
  ```

- [ ] **Step 32** [V] ~30s: Auth coverage >= 80%
  - If >= 80% → Step 35
  - If < 80% → Step 33

- [ ] **Step 33** [D] ~10m: Add tests for uncovered paths
  ```bash
  go tool cover -func=/tmp/auth-coverage.out | grep -v "100.0%" | head -20
  ```
  - Write tests for top uncovered functions
  - Focus: error paths, edge cases, boundary conditions

- [ ] **Step 34** [B] ~1m: Re-check coverage after adding tests
  ```bash
  go test -coverprofile=/tmp/auth-coverage.out ./pkg/auth/... && go tool cover -func=/tmp/auth-coverage.out | tail -1
  ```

- [ ] **Step 35** [C] ~30s: **COMMIT CHECKPOINT** — Auth fully verified
  ```bash
  git add pkg/auth/ && git commit -m "[PLAN S72-R] Steps 29-35: Auth suite green — race-clean, coverage verified"
  ```

### 2E: mTLS Package Tests

- [ ] **Step 36** [B] ~1m: Run mTLS tests
  ```bash
  go test -v -race ./pkg/transport/mtls/... 2>&1 | tail -20
  ```

- [ ] **Step 37** [V] ~30s: mTLS tests pass with race detector
  - If fail → Step 37a [D]: Check cert generation, ECDSA P-256 validation, rotator goroutine

- [ ] **Step 38** [C] ~30s: **COMMIT CHECKPOINT** — mTLS verified
  ```bash
  git add pkg/transport/ && git commit -m "[PLAN S72-R] Steps 36-38: mTLS tests verified — cert gen + rotation" || echo "nothing to commit"
  ```

- [ ] **Step 39** [B] ~30s: Run dev cert bootstrap
  ```bash
  bash scripts/gen-dev-certs.sh 2>&1 | tail -15
  ```

- [ ] **Step 40** [V] ~30s: **PHASE 2 EXIT GATE** — All auth + mTLS tests pass, coverage >=80%, certs generated
  - `go test -race ./pkg/auth/...` passes → pass
  - `go test -race ./pkg/transport/mtls/...` passes → pass
  - Auth coverage >= 80% → pass
  - If any fail → DO NOT PROCEED.

---

# ═══════════════════════════════════════════════════════════
# PHASE 3: SERVICE WIRING — mTLS + AUTH MIDDLEWARE (Steps 41-70)
# ═══════════════════════════════════════════════════════════

## PHASE 3: SERVICE WIRING (Steps 41-70)

**Goal**: Wire mTLS into all service binaries. Wire auth middleware into all HTTP/gRPC routes.
**Prerequisite**: Phase 2 passed. Auth + mTLS verified green.
**Time**: ~2 hours
**Agent**: Developer

### 3A: mTLS Wiring — Template Pattern

- [ ] **Step 41** [R] ~2m: Read services/wotan/cmd/wotan/main.go — identify gRPC server creation point
  ```bash
  grep -n "grpc.NewServer\|net.Listen\|tls\|Serve" services/wotan/cmd/wotan/main.go
  ```

- [ ] **Step 42** [W] ~10m: Wire mTLS into services/wotan/cmd/wotan/main.go (TEMPLATE)
  - Import `"unheaded/pkg/transport/mtls"`
  - Add TLS config loading: `tlsCfg, err := mtls.SetupServiceTLS("wotan", certsDir)`
  - Add mTLS to gRPC server options: `grpc.Creds(credentials.NewTLS(tlsCfg))`
  - Add cert path CLI flag: `--certs-dir` (default: `/etc/unheaded/pki`)
  - Keep HTTP health endpoint on plaintext (for load balancers)
  - Graceful fallback: if `--tls-enabled=false`, skip TLS setup (backward compat)

- [ ] **Step 43** [B] ~30s: Verify wotan still builds
  ```bash
  go build ./services/wotan/... 2>&1 | tail -5
  ```

- [ ] **Step 44** [V] ~30s: Wotan builds with mTLS wiring
  - If fail → Step 44a [D]: Check import paths, missing grpc/credentials dependency

- [ ] **Step 45** [C] ~30s: **COMMIT CHECKPOINT** — Wotan mTLS wired (template)
  ```bash
  git add services/wotan/ && git commit -m "[PLAN S72-R] Steps 41-45: Wotan mTLS wired — template pattern established"
  ```

### 3B: mTLS Wiring — Apply Template to Remaining Services

- [ ] **Step 46** [W] ~20m: Apply mTLS pattern to remaining gRPC services
  - services/timeguru/cmd/timeguru/main.go
  - services/captain/cmd/captain/main.go
  - services/architect/cmd/architect/main.go
  - services/micromanager/cmd/micromanager/main.go

- [ ] **Step 47** [W] ~15m: Apply mTLS pattern to cmd/ services
  - cmd/protocol-api/main.go (if not already wired)
  - cmd/dashboard-backend/main.go
  - cmd/kanban-app/main.go (if exists)
  - cmd/wiki-server/main.go
  - cmd/unheaded-daemon/main.go

- [ ] **Step 48** [B] ~2m: Verify ALL services build
  ```bash
  go build ./cmd/... ./services/... 2>&1 | tail -10
  ```

- [ ] **Step 49** [V] ~30s: All services build with mTLS
  - If fail → Step 49a [D]: Check specific failures: `go build ./cmd/dashboard-backend/... 2>&1`

- [ ] **Step 50** [C] ~30s: **COMMIT CHECKPOINT** — All services mTLS wired
  ```bash
  git add services/ cmd/ && git commit -m "[PLAN S72-R] Steps 46-50: mTLS wired into all service binaries"
  ```

### 3C: Auth Middleware Wiring — HTTP Routes

- [ ] **Step 51** [W] ~15m: Wire auth middleware into all HTTP services
  - Import `"unheaded/pkg/auth"` in each service main.go
  - Call `auth.SetupMiddleware(cfg)` to get middleware
  - Wrap router: `auth.WrapHandler(router, middleware)`
  - Ensure /health, /ready bypass auth (SkipAuthPaths already handles this)
  - Ensure /metrics requires observer+ role

- [ ] **Step 52** [B] ~30s: Verify build
  ```bash
  go build ./cmd/... ./services/... 2>&1 | tail -10
  ```

- [ ] **Step 53** [V] ~30s: All services build with auth middleware
  - If fail → Step 53a [D]: Check import cycles, missing auth config initialization

- [ ] **Step 54** [C] ~30s: **COMMIT CHECKPOINT** — Auth middleware wired to HTTP routes
  ```bash
  git add services/ cmd/ && git commit -m "[PLAN S72-R] Steps 51-54: Auth middleware wired into all HTTP routes"
  ```

### 3D: Auth Middleware Wiring — gRPC Routes

- [ ] **Step 55** [W] ~10m: Wire gRPC auth interceptor into all gRPC servers
  - Import `"unheaded/pkg/auth"`
  - Add `grpc.UnaryInterceptor(auth.GRPCUnaryInterceptor(authenticator))` to server options
  - Allowlist: `grpc.health.v1.Health` (no auth required)

- [ ] **Step 56** [B] ~30s: Verify build
  ```bash
  go build ./cmd/... ./services/... 2>&1 | tail -10
  ```

- [ ] **Step 57** [V] ~30s: All services build with gRPC interceptor
  - If fail → Step 57a [D]: Check interceptor signature matches grpc.UnaryServerInterceptor

- [ ] **Step 58** [C] ~30s: **COMMIT CHECKPOINT** — gRPC auth wired
  ```bash
  git add services/ cmd/ && git commit -m "[PLAN S72-R] Steps 55-58: gRPC auth interceptor wired into all services"
  ```

### 3E: Integration Test Helpers

- [ ] **Step 59** [W] ~10m: Update existing integration tests with auth headers
  - All tests calling service HTTP endpoints need `Authorization: Bearer <test-token>`
  - Use `testutil.AdminToken()` for setup, `testutil.ObserverToken()` for read-only
  - Add `testutil.ServiceToken("wotan")` for inter-service test calls

- [ ] **Step 60** [B] ~2m: Run full test suite
  ```bash
  go test -race -count=1 -timeout 120s ./... 2>&1 | tail -40
  ```

- [ ] **Step 61** [V] ~30s: Full test suite passes
  - If fail → Step 62
  - If pass → Step 65

- [ ] **Step 62** [D] ~10m: Debug test failures — likely auth-related
  ```bash
  go test -v ./cmd/protocol-api/... 2>&1 | tail -40
  go test -v ./services/wotan/... 2>&1 | tail -40
  ```
  - Common: tests making HTTP requests without auth headers now get 401
  - Fix: add test setup that generates dev tokens

- [ ] **Step 63** [B] ~1m: Re-run after fixes
  ```bash
  go test -race -count=1 -timeout 120s ./... 2>&1 | tail -40
  ```

- [ ] **Step 64** [C] ~30s: **COMMIT CHECKPOINT** — Integration tests updated with auth
  ```bash
  git add . && git commit -m "[PLAN S72-R] Steps 59-64: Integration tests updated with auth headers"
  ```

- [ ] **Step 65** [V] ~1m: **PHASE 3 EXIT GATE** — All services build with mTLS + auth, all tests pass
  - `go build ./cmd/... ./services/...` passes → pass
  - `go test -race ./...` passes → pass
  - `grep -rn "grpc.NewServer" services/ cmd/ | wc -l` matches mTLS wiring count → pass
  - If any fail → DO NOT PROCEED.

---

# ═══════════════════════════════════════════════════════════
# PHASE 4: PROTO + FULL BUILD VERIFICATION (Steps 66-78)
# ═══════════════════════════════════════════════════════════

## PHASE 4: PROTO + FULL BUILD (Steps 66-78)

**Goal**: Regenerate proto if needed, full build, full test, full lint.
**Prerequisite**: Phase 3 passed.
**Time**: ~30 minutes
**Agent**: Coordinator

- [ ] **Step 66** [R] ~1m: Check if proto changes are needed
  ```bash
  grep -n "Shield\|Anamnesis\|KingdomMode\|KingdomState" proto/unheaded/v1/protocol.proto 2>/dev/null | head -10
  ```
  - If new message types for Shield/Anamnesis/Kingdom exist → skip regeneration
  - If missing → Step 67
  - If no proto file exists → Step 69 (skip proto entirely)

- [ ] **Step 67** [W] ~10m: Add new protobuf messages if needed
  - ShieldStatus message
  - AnamnesisEvent message
  - KingdomState message
  - Subscribe/Unsubscribe RPCs for Wotan

- [ ] **Step 68** [B] ~1m: Regenerate Go bindings
  ```bash
  protoc --go_out=. --go-grpc_out=. proto/unheaded/v1/protocol.proto 2>&1 || echo "PROTOC SKIPPED"
  ```

- [ ] **Step 69** [B] ~2m: Full Go build
  ```bash
  go build ./... 2>&1 | tail -10
  ```

- [ ] **Step 70** [V] ~30s: Build passes
  - If fail → Step 70a [D]: Check proto generation errors, import paths

- [ ] **Step 71** [B] ~5m: Full test suite with race detector
  ```bash
  go test -race -count=1 -timeout 180s ./... 2>&1 | tail -30
  ```

- [ ] **Step 72** [V] ~30s: All tests pass
  - If fail → Step 72a [D]: Identify failing package, debug specific test

- [ ] **Step 73** [B] ~2m: Lint check
  ```bash
  golangci-lint run ./... 2>&1 | tail -20 || echo "LINT ISSUES — review but don't block"
  ```

- [ ] **Step 74** [B] ~1m: Security scan
  ```bash
  gosec ./... 2>&1 | tail -20 || echo "GOSEC ISSUES — review"
  ```

- [ ] **Step 75** [C] ~30s: **COMMIT CHECKPOINT** — Full build + test + lint verified
  ```bash
  git add . && git commit -m "[PLAN S72-R] Steps 66-75: Full build verified — tests green, lint clean, security scanned"
  ```

- [ ] **Step 76** [B] ~1m: Overall test coverage
  ```bash
  go test -coverprofile=/tmp/total-coverage.out ./... 2>&1 | tail -5 && go tool cover -func=/tmp/total-coverage.out | tail -1
  ```

- [ ] **Step 77** [V] ~30s: Total coverage >= 50% (auth >= 80%)
  - If below 50% overall → document in STUCK report, don't block
  - If auth below 80% → MUST add tests before proceeding

- [ ] **Step 78** [V] ~30s: **PHASE 4 EXIT GATE** — Build green, tests green, lint reviewed, security scanned
  - `go build ./...` passes → pass
  - `go test -race ./...` passes → pass
  - `golangci-lint` runs (warnings OK, errors fixed) → pass
  - If any fail → DO NOT PROCEED.

---

# ═══════════════════════════════════════════════════════════
# PHASE 5: SBOM EXECUTION + MAKEFILE VERIFICATION (Steps 79-90)
# ═══════════════════════════════════════════════════════════

## PHASE 5: SBOM + MAKEFILE (Steps 79-90)

**Goal**: Run SBOM generation, verify Makefile targets work, run license check.
**Prerequisite**: Phase 4 passed.
**Time**: ~30 minutes
**Agent**: Agent

- [ ] **Step 79** [B] ~2m: Run SBOM generation
  ```bash
  bash scripts/generate-sbom.sh 2>&1 | tail -20
  ```

- [ ] **Step 80** [V] ~30s: SBOM generated
  - If pass → Step 82
  - If fail (syft missing) → Step 81

- [ ] **Step 81** [D] ~3m: Install syft and retry
  ```bash
  curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin
  bash scripts/generate-sbom.sh 2>&1 | tail -20
  ```

- [ ] **Step 82** [B] ~1m: Verify Makefile sbom target
  ```bash
  make sbom 2>&1 | tail -10 || echo "make sbom not yet added — add it"
  ```

- [ ] **Step 83** [B] ~1m: Run make test
  ```bash
  make test-go 2>&1 | tail -20
  ```

- [ ] **Step 84** [V] ~30s: make test passes
  - If fail → Step 84a [D]: Check Makefile test target, coverage threshold

- [ ] **Step 85** [B] ~1m: Run make lint
  ```bash
  make lint 2>&1 | tail -20 || echo "LINT ISSUES — review"
  ```

- [ ] **Step 86** [B] ~1m: Run make security
  ```bash
  make security 2>&1 | tail -20 || echo "SECURITY ISSUES — review"
  ```

- [ ] **Step 87** [C] ~30s: **COMMIT CHECKPOINT** — SBOM + Makefile verified
  ```bash
  git add sbom-results/ Makefile && git commit -m "[PLAN S72-R] Steps 79-87: SBOM generated, Makefile targets verified" || echo "nothing to commit"
  ```

- [ ] **Step 88** [B] ~30s: Verify coverage trend script
  ```bash
  bash scripts/coverage-trend.sh 2>&1 | tail -10 || echo "coverage-trend needs Go test — expected"
  ```

- [ ] **Step 89** [V] ~30s: All tooling scripts functional

- [ ] **Step 90** [V] ~30s: **PHASE 5 EXIT GATE** — SBOM generated, make targets work
  - SBOM output exists in sbom-results/ → pass
  - `make test-go` passes → pass
  - If any fail → document, don't hard-block Phase 6.

---

# ═══════════════════════════════════════════════════════════
# PHASE 6: FINAL VERIFICATION GATE (Steps 91-100)
# ═══════════════════════════════════════════════════════════

## PHASE 6: FINAL GATE (Steps 91-100)

**Goal**: Full deliverable verification matching original S72 battle plan Phase 13.
**Prerequisite**: All prior phases complete.
**Time**: ~15 minutes
**Agent**: Coordinator

- [ ] **Step 91** [V] ~30s: Protocol Specs
  ```bash
  test -f docs/protocol/draft-bellis-unheaded-protocol-foundation-05.md && echo "MONAD ✓"
  test -f docs/protocol/draft-bellis-unheaded-sophia-dictionary-02.md && echo "SOPHIA ✓"
  test -f docs/protocol/draft-bellis-unheaded-wotan-memory-02.md && echo "WOTAN ✓"
  ```

- [ ] **Step 92** [V] ~30s: Auth Package — coverage + race-clean
  ```bash
  go test -race -coverprofile=/tmp/auth.out ./pkg/auth/... && go tool cover -func=/tmp/auth.out | tail -1
  ```
  - Coverage >= 80% → pass

- [ ] **Step 93** [V] ~30s: mTLS — cert gen + integration
  ```bash
  go test -race ./pkg/transport/mtls/... 2>&1 | tail -5
  ```

- [ ] **Step 94** [V] ~30s: All services build with auth + mTLS
  ```bash
  go build ./cmd/... ./services/... 2>&1 | tail -5 && echo "BUILD ✓"
  ```

- [ ] **Step 95** [V] ~30s: SBOM
  ```bash
  test -f scripts/generate-sbom.sh && echo "SBOM SCRIPT ✓"
  test -f .github/workflows/sbom-audit.yml && echo "SBOM CI ✓"
  ```

- [ ] **Step 96** [V] ~30s: CI/CD — 8 workflows + 5 Jenkinsfiles
  ```bash
  echo "Workflows: $(ls .github/workflows/*.yml | wc -l)"
  echo "Jenkinsfiles: $((1 + $(ls jenkins/Jenkinsfile.* | wc -l)))"
  ```

- [ ] **Step 97** [V] ~30s: Bare Metal — 2 runbooks + 5 scripts
  ```bash
  echo "Runbooks: $(ls docs/bare-metal/*.md | wc -l)"
  echo "Scripts: $(ls scripts/bare-metal/*.sh | wc -l)"
  ```

- [ ] **Step 98** [B] ~30s: Verify no unauthenticated endpoints (except health/ready)
  ```bash
  grep -rn "HandleFunc\|router\." services/ cmd/ | grep -v "health\|ready\|test\|_test.go" | head -20
  ```

- [ ] **Step 99** [C] ~30s: **FINAL COMMIT** — S72 complete
  ```bash
  git add . && git commit -m "[PLAN S72-R] S72 Sprint Complete — all phases verified, build green, tests pass, auth wired, SBOM generated" || echo "nothing to commit"
  ```

- [ ] **Step 100** [V] ~1m: **S72 FINAL GATE**
  ```
  ALL CHECKS MUST PASS:
  ✓ Protocol specs — 3 new drafts (7,110 lines total)
  ✓ Auth — hardened, >=80% coverage, race-clean
  ✓ mTLS — cert gen + integration + wired into all services
  ✓ Auth middleware — wired into all HTTP + gRPC routes
  ✓ SBOM — automated
  ✓ CI/CD — 8 workflows hardened
  ✓ Jenkins — 5 Jenkinsfiles + shared library
  ✓ Bare Metal — 2 runbooks + 5 scripts
  ✓ Build passes
  ✓ Tests pass with race detector
  ✓ Lint clean (or issues documented)
  ```
  - ALL PASS → S72 COMPLETE. Update timeline. Celebrate. 🎉
  - ANY FAIL → Document in STUCK REPORT. Fix in S73.

---

## APPENDIX A: EMERGENCY PROCEDURES

### E1: Go Build Fails After Auth Wiring
```bash
# Symptom: import cycle or undefined references
# Fix: Check pkg/auth imports don't cycle through cmd/ or services/
go build ./pkg/auth/... 2>&1 | head -20
# If import cycle: move shared types to pkg/auth/types.go
```

### E2: Tests Fail After Auth Middleware Added
```bash
# Symptom: existing tests get 401 Unauthorized
# Fix: Add auth headers to test HTTP requests
# All test helpers should use testutil.AdminToken()
go test -v -run "TestFailing" ./cmd/protocol-api/... 2>&1 | tail -40
```

### E3: mTLS Wiring Breaks Service Startup
```bash
# Symptom: service crashes on startup with cert file not found
# Fix: Ensure --tls-enabled defaults to false for dev
# The env var AUTH_ENABLED=false should skip TLS setup
go run ./services/wotan/cmd/wotan/ --help 2>&1 | grep tls
```

### E4: Race Detector Finds Data Race
```bash
# Symptom: WARNING: DATA RACE in auth audit logger or rate limiter
# Fix: Add mutex to concurrent map access
go test -race -v -run "TestConcurrent" ./pkg/auth/... 2>&1
```

### E5: Coverage Below 80%
```bash
# Show uncovered lines
go tool cover -func=/tmp/auth-coverage.out | grep -v "100.0" | sort -t: -k3 -n
# Focus on error paths and edge cases
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent Type | Parallelizable | Dependencies | Time Est |
|-------|-----------|----------------|-------------|----------|
| 1 | Coordinator | No | None | 15m |
| 2 | Developer | Yes (with 3 on separate machine) | Phase 1 | 1.5h |
| 3 | Developer | No | Phase 2 | 2h |
| 4 | Coordinator | No | Phase 3 | 30m |
| 5 | Agent | No | Phase 4 | 30m |
| 6 | Coordinator | No | ALL | 15m |

**Critical Path**: 1 → 2 → 3 → 4 → 5 → 6 = ~5 hours minimum

---

## APPENDIX C: QUICK REFERENCE

### Auth Token Types
```
Bearer JWT:     External API access — user identity + roles
Service-Token:  Inter-service auth — identifies calling service
API-Key:        Third-party integrations — rate-limited
```

### Role Hierarchy
```
admin > operator > observer > service > guest
```

### Endpoint Auth Matrix
```
/health, /ready       → NO AUTH (bypass)
/metrics              → observer+
/api/v1/* GET         → observer+
/api/v1/* POST/PUT    → operator+
/api/v1/admin/*       → admin only
gRPC health.v1.Health → NO AUTH (bypass)
gRPC all other        → service+ (via interceptor)
```

### mTLS Wiring Pattern (per-service)
```go
import "unheaded/pkg/transport/mtls"

// In main():
if tlsEnabled {
    tlsCfg, err := mtls.SetupServiceTLS(serviceName, certsDir)
    grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
}
```

### Test Auth Helpers
```go
import "unheaded/pkg/auth"

token := auth.TestAdminToken()     // admin role
token := auth.TestObserverToken()  // observer role
token := auth.TestServiceToken("wotan")  // service role
```

---

*S72-R Revised Battle Plan — Forged 2026-02-27*
*6 Phases. 100 Steps. The code is written — now make it compile, test, and wire.*
*The swarm laid the foundation. The Revised Plan makes it real.*

```
THE WARMONGER HAS REVISED.
THE REMAINING WORK IS CLEAR.
100 STEPS TO GREEN BUILD.
THE KINGDOM ADVANCES.
```
