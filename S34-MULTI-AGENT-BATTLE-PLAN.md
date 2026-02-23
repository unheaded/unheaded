# S34 MULTI-AGENT BATTLE PLAN — The Industrialization — 16 Phases, 320+ Steps

**Date**: 2026-02-23
**Sprint**: S34 — From Spectacle to Substance: Port Doom-Proven eBPF to Production Packet Tracing
**Prerequisite**: S33 complete. WS1 (Fenrir's Eye) DONE. WS3 (Go injector + profiler) DONE. WS4 (Wiki + docs) DONE. Hardening sprint DONE. Build passes. All tests pass.
**Target**: First real packet trace captured, decoded, and displayed in dashboard — the product IS tracing, not Doom
**Estimated Duration**: 80-120 hours across 3-4 weeks (Feb 24 — Mar 21, 2026)
**Agent Strategy**: 5 parallel workstreams, 3-4 concurrent agents max, coordinator for integration phases
**Commit Cadence**: Every 4 steps (calculated: max(3, min(5, 320/20)) = 5, reduced to 4 for safety)
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts. Commit before skip. Log everything.

---

## SITUATION REPORT — Ground Truth from Git

**What S33 shipped** (verified from `git log --since=2026-02-20`):
- `00144d3` WS1 COMPLETE — Fenrir's Eye doom-bridge service (WebSocket frame streaming)
- `20a9638` WS3 COMPLETE — Go packet injector + scaling profiler
- `f895128` WS4 COMPLETE — Wiki server, documentation, conference outline
- `a4305b9` Hardening sprint — script migration, gosec pinning
- `79a5215` Security P0/P1/P2 quick wins + auth skeleton
- `31dbbe2` P1 hardening — WebSocket JSON validation, CSP, Wotan error logging
- `13e43f1` 18 Go fuzz targets + auth package tests
- `92dbf78` CI coverage gate, Nix test stubs, credential cleanup
- `8a2c01d` Rate limiter XFF hardening, SBOM generation, TODO audit
- `e169a86` Multi-runtime container configs (containerd + LXD + Docker)
- `65efbf6` Dashboard responsive UI polish + 5 new test suites

**What remains** (from TODO.md + battle-plan.md analysis):
- WS2 (Lich Campaigns D1-D6) — NOT STARTED
- WS5 (Return to Core — production packet tracing) — NOT STARTED, THE MAIN EVENT
- P0 #9: eBPF programs are userspace stubs — core product non-functional
- P0 #13: SBOM generation — partially done (commit 8a2c01d)
- P0 #14: Captain /tmp data dir — needs verification
- P1 #16: No authentication on ANY endpoint
- P1 #19: Silent Wotan failures — no fallback behavior
- P1 #20/#26: No TLS between services / No TLS on gRPC
- P1 #25: Double-check locking race in wotan-client
- P2 #28: Go 1.21 → 1.22+ upgrade
- P2 #35: `make deploy` is a no-op
- P2 #37: Dead BroadcastJSON method
- Missing: Service discovery, frontend tests, real E2E suite, container scanning, secrets management
- Doom auto-restart crash loop bug (doesn't reload .data sections)

**Code Stats**: 465K+ LOC, 611 Go files, 14 Rust crates, 293 Rust tests, 135 Go packages, 23 services, 0 failures.

**Critical Insight**: The Doom PoC proved the ENTIRE eBPF toolchain works end-to-end:
Rust/Aya → BPF compilation → map pinning → XDP attachment → userspace reader.
WS5 is NOT "build eBPF from scratch." It's "port proven patterns to production programs."

---

## LEGEND

[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
[C] = Commit checkpoint (git commit at prescribed interval)
[STUCK] = Step skipped via Skip Protocol (needs human intervention)
[BLOCKED] = Step blocked by upstream STUCK step

---

## DEPENDENCY GRAPH

```
                    ┌──────────────────────────────────┐
                    │  PHASE 0: Environment Verify     │
                    └──────────────┬───────────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                     │
              ▼                    ▼                     ▼
    ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
    │ PHASE 1: Auth   │  │ PHASE 2: Lich   │  │ PHASE 3: Go     │
    │ Skeleton → Real │  │ D1-D6 Campaigns │  │ 1.22 + Cleanup  │
    │ [Agent A]       │  │ [Agent B]       │  │ [Agent C]       │
    └────────┬────────┘  └────────┬────────┘  └────────┬────────┘
             │                    │                     │
             ▼                    ▼                     │
    ┌─────────────────┐  ┌─────────────────┐           │
    │ PHASE 4: mTLS   │  │ PHASE 5: Lich   │           │
    │ Service Mesh    │  │ Findings Fix    │           │
    │ [Agent A]       │  │ [Agent B]       │           │
    └────────┬────────┘  └─────────────────┘           │
             │                                          │
             ├──────────────────────────────────────────┘
             ▼
    ┌─────────────────────────────────────────────────────┐
    │ PHASE 6: Production eBPF — packet_marker (XDP)     │
    │ THE MAIN EVENT — [Coordinator + Agent D]            │
    └────────────────────────┬────────────────────────────┘
                             │
              ┌──────────────┼──────────────────┐
              ▼              ▼                   ▼
    ┌──────────────┐ ┌──────────────┐  ┌──────────────┐
    │ PHASE 7:     │ │ PHASE 8:     │  │ PHASE 9:     │
    │ flow_tracker │ │ latency_probe│  │ trace-       │
    │ (TC/kprobe)  │ │ (tracepoint) │  │ collector    │
    │ [Agent D]    │ │ [Agent E]    │  │ [Agent F]    │
    └──────┬───────┘ └──────┬───────┘  └──────┬───────┘
           │                │                  │
           └────────────────┼──────────────────┘
                            ▼
    ┌─────────────────────────────────────────────────────┐
    │ PHASE 10: Observability Pipeline Integration        │
    │ trace-collector → Wotan → dashboard                 │
    │ [Coordinator]                                       │
    └────────────────────────┬────────────────────────────┘
                             │
              ┌──────────────┼──────────────────┐
              ▼              ▼                   ▼
    ┌──────────────┐ ┌──────────────┐  ┌──────────────┐
    │ PHASE 11:    │ │ PHASE 12:    │  │ PHASE 13:    │
    │ Dashboard    │ │ Service      │  │ Wotan        │
    │ Trace UI     │ │ Discovery    │  │ Hardening    │
    │ [Agent G]    │ │ [Agent H]    │  │ [Agent A]    │
    └──────┬───────┘ └──────┬───────┘  └──────┬───────┘
           │                │                  │
           └────────────────┼──────────────────┘
                            ▼
    ┌─────────────────────────────────────────────────────┐
    │ PHASE 14: E2E Integration Test Suite                │
    │ browser → gateway → services → eBPF → Wotan → dash │
    │ [Coordinator]                                       │
    └────────────────────────┬────────────────────────────┘
                             │
                             ▼
    ┌─────────────────────────────────────────────────────┐
    │ PHASE 15: Deployment Pipeline                       │
    │ make deploy actually works                          │
    │ [Agent C]                                           │
    └────────────────────────┬────────────────────────────┘
                             │
                             ▼
    ┌─────────────────────────────────────────────────────┐
    │ PHASE 16: Alpha Ship Gate                           │
    │ Final verification, compliance snapshot, tag v0.1.0 │
    │ [Coordinator + All]                                 │
    └─────────────────────────────────────────────────────┘
```

---

## PHASE 0: ENVIRONMENT VERIFICATION (Steps 1-18)

**Goal**: Confirm dev environment, toolchain, and codebase are ready for the industrialization sprint.
**Prerequisite**: S33 commits pushed. Clean working tree.
**Time**: ~20 minutes
**Agent**: Coordinator

### Verify Git State

- [ ] **Step 1** [B]: Check working tree is clean
  ```bash
  cd /home/muck/unheaded && git status --short
  ```

- [ ] **Step 2** [V]: Working tree clean (no uncommitted changes). If dirty → commit or stash before proceeding.

- [ ] **Step 3** [B]: Verify latest commits are pushed
  ```bash
  cd /home/muck/unheaded && git log --oneline -5 && echo "---" && git log --oneline origin/main -5
  ```

- [ ] **Step 4** [V]: Local and remote HEAD match. If behind → `git push origin main`.

### Verify Toolchain

- [ ] **Step 5** [B]: Check Go version
  ```bash
  go version
  ```

- [ ] **Step 6** [V]: Go >= 1.21 present. Record version for Phase 3 upgrade.

- [ ] **Step 7** [B]: Check Rust toolchain
  ```bash
  rustc --version && cargo --version && rustup target list --installed | grep bpf
  ```

- [ ] **Step 8** [V]: Rust nightly available. `bpfel-unknown-none` target installed. If missing:
  - If fail → Step 8a [D]: `rustup target add bpfel-unknown-none`

- [ ] **Step 9** [B]: Check eBPF tooling
  ```bash
  which bpftool && bpftool version && which llvm-objdump
  ```

- [ ] **Step 10** [D]: If bpftool missing:
  ```bash
  sudo apt-get install -y linux-tools-$(uname -r) linux-tools-generic
  ```

- [ ] **Step 11** [B]: Check kernel BPF support
  ```bash
  uname -r && cat /boot/config-$(uname -r) | grep -E "CONFIG_BPF|CONFIG_XDP|CONFIG_NET_CLS_BPF" | head -10
  ```

- [ ] **Step 12** [V]: Kernel >= 5.15 with BPF/XDP enabled.

### Verify Build

- [ ] **Step 13** [B] ~2m: Build all Go packages
  ```bash
  cd /home/muck/unheaded && go build ./... 2>&1 | tail -20
  ```

- [ ] **Step 14** [V]: Build passes with zero errors.

- [ ] **Step 15** [B] ~3m: Run full Go test suite
  ```bash
  cd /home/muck/unheaded && go test ./... 2>&1 | tail -40
  ```

- [ ] **Step 16** [V]: All tests pass. Zero failures.

- [ ] **Step 17** [B] ~2m: Build Rust eBPF crates
  ```bash
  cd /home/muck/unheaded && cargo test --manifest-path crates/monad-mbc/Cargo.toml 2>&1 | tail -20
  ```

- [ ] **Step 18** [V]: **PHASE 0 EXIT GATE** — Go build passes, Go tests pass, Rust tests pass, BPF toolchain available, git clean.
  - If ALL pass → proceed to Phases 1-3 (parallel)
  - If ANY fail → DO NOT PROCEED. Fix within this phase.

---

## PHASE 1: AUTHENTICATION — From Skeleton to Real (Steps 19-52) [P]

**Goal**: Implement real authentication on all service endpoints. mTLS between services, API key for external.
**Prerequisite**: Phase 0 exit gate passed
**Time**: ~4 hours
**Agent**: Agent A (Security Focus)

### Audit Existing Auth Skeleton

- [ ] **Step 19** [R]: Read existing auth package
  ```bash
  cat /home/muck/unheaded/pkg/auth/auth.go
  ```

- [ ] **Step 20** [R]: Identify all HTTP servers that need auth
  ```bash
  cd /home/muck/unheaded && grep -rn "http.Server{" --include="*.go" | grep -v test | grep -v vendor
  ```

- [ ] **Step 21** [V]: Auth skeleton exists. List of all endpoints requiring protection documented.

### Implement JWT Auth Middleware

- [ ] **Step 22** [W] ~30m: Implement `pkg/auth/jwt.go` — JWT token validation middleware
  - HMAC-SHA256 minimum, RS256 preferred
  - Token extraction from `Authorization: Bearer <token>` header
  - Claims validation: `exp`, `iss`, `sub`, `aud`
  - Role-based access control: `admin`, `service`, `readonly`
  - Deny by default — every endpoint requires explicit auth annotation

- [ ] **Step 23** [W] ~20m: Implement `pkg/auth/jwt_test.go` — comprehensive test suite
  - Valid token → 200
  - Expired token → 401
  - Missing token → 401
  - Invalid signature → 401
  - Wrong audience → 403
  - Role escalation attempt → 403

- [ ] **Step 24** [B] ~1m: Run auth tests
  ```bash
  cd /home/muck/unheaded && go test ./pkg/auth/... -v -count=1 2>&1 | tail -30
  ```

- [ ] **Step 25** [V]: All auth tests pass.

- [ ] **Step 26** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add pkg/auth/ && git commit -m "[PLAN S34] Steps 22-25: JWT auth middleware with full test suite

Phase 1: Authentication
Steps completed: 22, 23, 24, 25
Verification gates passed: auth unit tests
Next: Step 27 (API key auth)"
  ```

### Implement API Key Auth

- [ ] **Step 27** [W] ~20m: Implement `pkg/auth/apikey.go` — API key validation for external clients
  - Key stored as SOPS-encrypted secret (mount path configurable)
  - Key rotation support (accept current + previous key)
  - Rate limiting per key (integrated with existing rate limiter)
  - `X-API-Key` header extraction

- [ ] **Step 28** [W] ~15m: Implement `pkg/auth/apikey_test.go`
  - Valid key → 200
  - Invalid key → 401
  - Missing key → 401
  - Rotated key (previous) → 200
  - Rate limited key → 429

- [ ] **Step 29** [B] ~1m: Run API key tests
  ```bash
  cd /home/muck/unheaded && go test ./pkg/auth/... -v -count=1 -run "APIKey" 2>&1 | tail -20
  ```

- [ ] **Step 30** [V]: API key tests pass.

### Wire Auth Into Services

- [ ] **Step 31** [W] ~15m: Add auth middleware to kanban-app
  ```go
  // cmd/kanban-app/main.go — wrap router with auth middleware
  // Internal endpoints (/health, /ready, /metrics) — no auth (Prometheus scrape)
  // API endpoints (/api/v1/*) — JWT or API key required
  ```

- [ ] **Step 32** [W] ~15m: Add auth middleware to dashboard-backend
  ```go
  // WebSocket upgrade — validate JWT before upgrade
  // /api/v1/* — JWT or API key
  // /health, /ready, /metrics — no auth
  ```

- [ ] **Step 33** [W] ~10m: Add auth middleware to timeguru service

- [ ] **Step 34** [W] ~10m: Add auth middleware to captain service

- [ ] **Step 35** [W] ~10m: Add auth middleware to micromanager service

- [ ] **Step 36** [W] ~10m: Add auth middleware to architect service

- [ ] **Step 37** [W] ~10m: Add auth middleware to monad service

- [ ] **Step 38** [W] ~10m: Add auth middleware to sophia service

- [ ] **Step 39** [B] ~2m: Build all services with auth
  ```bash
  cd /home/muck/unheaded && go build ./... 2>&1 | tail -10
  ```

- [ ] **Step 40** [V]: Build passes with auth wired in.

- [ ] **Step 41** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "[PLAN S34] Steps 31-40: Auth middleware wired into all 8 services

Phase 1: Authentication
Steps completed: 31-40
Verification gates passed: build with auth
Next: Step 42 (service-to-service tokens)"
  ```

### Service-to-Service Token Generation

- [ ] **Step 42** [W] ~20m: Implement `pkg/auth/service_token.go`
  - Each service generates a short-lived JWT (5 min TTL) for inter-service calls
  - Service identity from environment: `UNHEADED_SERVICE_NAME`
  - Token includes `sub` = service name, `aud` = target service, `role` = `service`
  - Shared signing key loaded from mounted secret file

- [ ] **Step 43** [W] ~15m: Implement `pkg/auth/service_token_test.go`
  - Generate → validate round-trip
  - Expired token rejected
  - Wrong audience rejected
  - Service role grants appropriate permissions

- [ ] **Step 44** [B]: Run full auth test suite
  ```bash
  cd /home/muck/unheaded && go test ./pkg/auth/... -v -count=1 -race 2>&1 | tail -30
  ```

- [ ] **Step 45** [V]: All auth tests pass with race detector.

### Integration Verification

- [ ] **Step 46** [W] ~15m: Write auth integration test — `tests/integration/auth_test.go`
  - Start timeguru with auth enabled
  - Unauthenticated request → 401
  - Valid JWT request → 200
  - Valid API key request → 200
  - Service token request → 200

- [ ] **Step 47** [B] ~1m: Run auth integration test
  ```bash
  cd /home/muck/unheaded && go test ./tests/integration/ -run "Auth" -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 48** [V]: Auth integration test passes.

- [ ] **Step 49** [B] ~3m: Run FULL test suite to confirm no regressions
  ```bash
  cd /home/muck/unheaded && go test ./... 2>&1 | grep -E "FAIL|ok" | tail -40
  ```

- [ ] **Step 50** [V]: All tests pass. Zero regressions.

- [ ] **Step 51** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(auth): implement JWT + API key + service token authentication

- JWT middleware with HMAC-SHA256/RS256, role-based access control
- API key auth with rotation support
- Service-to-service tokens (5-min TTL, audience validation)
- Auth wired into all 8 services
- Integration test verifying auth enforcement
- All existing tests pass with zero regressions

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 52** [V]: **PHASE 1 EXIT GATE** — All endpoints require authentication. Unauthenticated requests return 401. Service tokens enable inter-service communication. All tests pass.
  - If pass → Phase 4 (mTLS) unlocked
  - If fail → DO NOT PROCEED to Phase 4. Debug within Phase 1.

---

## PHASE 2: LICH CAMPAIGNS D1-D6 — Offensive Security (Steps 53-79) [P]

**Goal**: Run 6 targeted attack campaigns against the Doom eBPF infrastructure. Document findings for WS5 hardening.
**Prerequisite**: Phase 0 exit gate passed. eBPF programs loadable on dev machine.
**Time**: ~6 hours
**Agent**: Agent B (BlackMage — Offensive Security)

### D1: ROM Injection Attack

- [ ] **Step 53** [R]: Inspect ROM_MAP BPF map permissions
  ```bash
  sudo bpftool map show pinned /sys/fs/bpf/unheaded/rom_map 2>/dev/null || echo "Map not pinned"
  ```

- [ ] **Step 54** [W] ~30m: Write `tests/security/doom/d1_rom_injection_test.go`
  - Attempt to write arbitrary bytes to ROM_MAP from unprivileged process
  - Attempt to modify ROM after BPF program load
  - Verify: does modified ROM change execution behavior?
  - Verify: can non-root process modify BPF maps?

- [ ] **Step 55** [B] ~2m: Run D1 campaign
  ```bash
  cd /home/muck/unheaded && sudo go test ./tests/security/doom/ -run "D1" -v -count=1 2>&1 | tail -30
  ```

- [ ] **Step 56** [V]: D1 results documented. Finding severity classified.

- [ ] **Step 57** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add tests/security/doom/ && git commit -m "[PLAN S34] Steps 53-56: Lich D1 — ROM injection campaign complete"
  ```

### D2: Framebuffer Exfiltration

- [ ] **Step 58** [W] ~25m: Write `tests/security/doom/d2_framebuffer_exfil_test.go`
  - Read RAM_MAP screen buffer (offset 0x100000) from unprivileged process
  - Verify: is screen data accessible without root?
  - Verify: can the framebuffer be read via Fenrir's Eye WebSocket without auth?
  - Rate: can continuous reads cause BPF map contention or DoS?

- [ ] **Step 59** [B] ~2m: Run D2 campaign
  ```bash
  cd /home/muck/unheaded && sudo go test ./tests/security/doom/ -run "D2" -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 60** [V]: D2 results documented.

### D3: Keyboard Injection

- [ ] **Step 61** [W] ~25m: Write `tests/security/doom/d3_keyboard_injection_test.go`
  - Publish forged SYSCALL messages to Wotan topic from unauthorized source
  - Inject arbitrary keystrokes via SYS_GET_KEY
  - Verify: does the BPF program accept injected input without validation?
  - Verify: can injection cause CPU state corruption?

- [ ] **Step 62** [B] ~2m: Run D3 campaign
  ```bash
  cd /home/muck/unheaded && sudo go test ./tests/security/doom/ -run "D3" -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 63** [V]: D3 results documented.

- [ ] **Step 64** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add tests/security/doom/ && git commit -m "[PLAN S34] Steps 58-63: Lich D2+D3 — framebuffer exfil + keyboard injection campaigns"
  ```

### D4: Flow Label Collision (Birthday Attack)

- [ ] **Step 65** [W] ~30m: Write `tests/security/doom/d4_flow_collision_test.go`
  - Generate N random 20-bit flow labels
  - Calculate collision probability: P(collision) = 1 - e^(-n²/2m) where m = 2^20
  - At n=1000: ~40% collision probability
  - At n=1448: ~50% collision probability (birthday bound)
  - Demonstrate actual collisions with randomized generation
  - Document: production MUST use 128-bit trace IDs, NOT 20-bit flow labels

- [ ] **Step 66** [B] ~1m: Run D4 campaign
  ```bash
  cd /home/muck/unheaded && go test ./tests/security/doom/ -run "D4" -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 67** [V]: D4 proves collision risk. Document recommendation for UUIDv7 in production.

### D5: SYSCALL Fuzzing

- [ ] **Step 68** [W] ~30m: Write `tests/security/doom/d5_syscall_fuzz_test.go`
  - Fuzz SYSCALL handler with invalid syscall numbers (0-65535)
  - Fuzz with malformed arguments
  - Fuzz with out-of-bounds memory addresses
  - Verify: does invalid input crash the BPF program or return gracefully?

- [ ] **Step 69** [B] ~5m: Run D5 campaign (fuzz for 60 seconds)
  ```bash
  cd /home/muck/unheaded && go test ./tests/security/doom/ -run "D5" -v -count=1 -fuzz "FuzzSyscall" -fuzztime 60s 2>&1 | tail -20
  ```

- [ ] **Step 70** [V]: D5 results documented. Any crashes classified as P0.

- [ ] **Step 71** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add tests/security/doom/ && git commit -m "[PLAN S34] Steps 65-70: Lich D4+D5 — flow collision + syscall fuzz campaigns"
  ```

### D6: ROM TOCTOU (Time-of-Check-Time-of-Use)

- [ ] **Step 72** [W] ~30m: Write `tests/security/doom/d6_rom_toctou_test.go`
  - Load ROM into BPF map
  - Start execution
  - Modify ROM map entries DURING execution from another goroutine
  - Verify: does execution diverge? Does it crash? Does it continue with stale data?
  - Measure race window: how many instructions between check and use?

- [ ] **Step 73** [B] ~3m: Run D6 campaign
  ```bash
  cd /home/muck/unheaded && sudo go test ./tests/security/doom/ -run "D6" -v -race -count=1 2>&1 | tail -30
  ```

- [ ] **Step 74** [V]: D6 results documented. TOCTOU window measured.

### Lich Campaign Summary Report

- [ ] **Step 75** [W] ~30m: Write `docs/security/lich-campaigns-s34.md`
  - Campaign summary table: D1-D6 results
  - Severity classifications: CRITICAL / HIGH / MEDIUM / LOW / INFO
  - Recommended fixes for each finding
  - WS5 security requirements derived from findings
  - Map to production hardening tasks

- [ ] **Step 76** [B] ~3m: Run ALL Lich campaigns together
  ```bash
  cd /home/muck/unheaded && sudo go test ./tests/security/doom/... -v -count=1 2>&1 | tail -50
  ```

- [ ] **Step 77** [V]: All 6 campaigns complete. Results documented.

- [ ] **Step 78** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "test(security): Lich campaigns D1-D6 — 6 offensive security tests against Doom eBPF

- D1: ROM injection — [RESULT]
- D2: Framebuffer exfiltration — [RESULT]
- D3: Keyboard injection — [RESULT]
- D4: Flow label collision (birthday attack proves 20-bit insufficient)
- D5: SYSCALL fuzzing — [RESULT]
- D6: ROM TOCTOU race condition — [RESULT]
- Full report: docs/security/lich-campaigns-s34.md

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 79** [V]: **PHASE 2 EXIT GATE** — All 6 Lich campaigns executed. All findings documented with severity. Recommendations for WS5 hardening written.
  - If pass → Phase 5 (fix findings) unlocked
  - If fail → complete remaining campaigns before proceeding.

---

## PHASE 3: GO 1.22 UPGRADE + CODEBASE CLEANUP (Steps 80-102) [P]

**Goal**: Upgrade Go to 1.22+, remove dead code, fix remaining P2/P3 items.
**Prerequisite**: Phase 0 exit gate passed
**Time**: ~2 hours
**Agent**: Agent C (Cleanup Focus)

### Go Version Upgrade

- [ ] **Step 80** [B]: Check current Go version and available upgrades
  ```bash
  go version && go env GOROOT
  ```

- [ ] **Step 81** [B] ~5m: Upgrade Go module to 1.22
  ```bash
  cd /home/muck/unheaded && go mod edit -go=1.22 && go mod tidy
  ```

- [ ] **Step 82** [B] ~2m: Build with new Go version
  ```bash
  cd /home/muck/unheaded && go build ./... 2>&1 | tail -20
  ```

- [ ] **Step 83** [V]: Build passes with Go 1.22.

- [ ] **Step 84** [B] ~3m: Run full test suite
  ```bash
  cd /home/muck/unheaded && go test ./... 2>&1 | grep -E "FAIL|ok" | tail -40
  ```

- [ ] **Step 85** [V]: All tests pass with Go 1.22.

- [ ] **Step 86** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add go.mod go.sum && git commit -m "chore: upgrade Go module to 1.22

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Dead Code Removal

- [ ] **Step 87** [B]: Find dead BroadcastJSON method
  ```bash
  cd /home/muck/unheaded && grep -rn "BroadcastJSON" --include="*.go" | grep -v test
  ```

- [ ] **Step 88** [W] ~5m: Remove dead `BroadcastJSON` method (P2 #37)

- [ ] **Step 89** [B]: Find duplicate service directories (P3 #42)
  ```bash
  cd /home/muck/unheaded && ls -d */ services/*/ | sort
  ```

- [ ] **Step 90** [W] ~15m: Consolidate duplicate directories — all services under `services/`

- [ ] **Step 91** [B]: Remove stale dev artifacts (P3 #40, #41)
  ```bash
  cd /home/muck/unheaded && ls *.txt PROJECT_TREE.txt 2>/dev/null
  ```

- [ ] **Step 92** [W] ~5m: Gitignore or remove stale files

- [ ] **Step 93** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "refactor: remove dead code + consolidate directory structure

- Remove dead BroadcastJSON method
- Consolidate duplicate service directories
- Clean stale dev artifacts

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Fix Remaining P1 Quick Wins

- [ ] **Step 94** [R]: Verify Captain data dir is not /tmp (P0 #14)
  ```bash
  cd /home/muck/unheaded && grep -rn "tmp\|/tmp" services/captain/ --include="*.go" | grep -v test
  ```

- [ ] **Step 95** [W] ~10m: Fix Captain data dir if still /tmp → configurable path

- [ ] **Step 96** [W] ~10m: Fix silent Wotan failures (P1 #19) — add fallback queue for missed messages

- [ ] **Step 97** [W] ~10m: Fix Connection header check (P1 #27) — case-insensitive comparison

- [ ] **Step 98** [W] ~10m: Fix double-check locking race (P1 #25) — use sync.Once pattern

- [ ] **Step 99** [B] ~3m: Run full test suite
  ```bash
  cd /home/muck/unheaded && go test ./... 2>&1 | grep -E "FAIL|ok" | tail -40
  ```

- [ ] **Step 100** [V]: All tests pass.

- [ ] **Step 101** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "fix: P1 quick wins — Captain data dir, Wotan fallback, connection header, locking race

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 102** [V]: **PHASE 3 EXIT GATE** — Go 1.22, dead code removed, P1 quick wins fixed, all tests pass.
  - If pass → unlocks Phase 6+ (contributes to clean codebase for eBPF work)
  - If fail → debug within this phase.

---

## PHASE 4: mTLS SERVICE MESH (Steps 103-132)

**Goal**: Enable mutual TLS between all services. Zero plaintext on the wire.
**Prerequisite**: Phase 1 exit gate passed (auth exists)
**Time**: ~4 hours
**Agent**: Agent A (Security Focus)

### Certificate Infrastructure

- [ ] **Step 103** [W] ~30m: Implement `pkg/tls/ca.go` — internal CA for service certificates
  - Self-signed root CA (development) / external CA (production)
  - Per-service certificate generation with SANs
  - Certificate rotation support (24-hour TTL for service certs)
  - Key material stored as mounted secrets

- [ ] **Step 104** [W] ~20m: Implement `pkg/tls/ca_test.go`
  - CA creation
  - Certificate generation for service name
  - Certificate validation (SANs match)
  - Expired cert rejection
  - Revoked cert handling

- [ ] **Step 105** [B] ~1m: Run TLS tests
  ```bash
  cd /home/muck/unheaded && go test ./pkg/tls/... -v -count=1 -race 2>&1 | tail -20
  ```

- [ ] **Step 106** [V]: TLS cert tests pass.

- [ ] **Step 107** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add pkg/tls/ && git commit -m "[PLAN S34] Steps 103-106: Internal CA + cert generation for mTLS"
  ```

### mTLS Server Configuration

- [ ] **Step 108** [W] ~20m: Implement `pkg/tls/server.go` — TLS server config factory
  - TLS 1.3 minimum
  - Client cert verification (mTLS)
  - Cipher suite restriction (TLS_AES_256_GCM_SHA384, TLS_CHACHA20_POLY1305_SHA256)
  - OCSP stapling support

- [ ] **Step 109** [W] ~20m: Implement `pkg/tls/client.go` — TLS client config factory
  - Load service client cert + key
  - Verify server cert against internal CA
  - Connection timeout configuration

- [ ] **Step 110** [W] ~15m: Wire mTLS into HTTP servers — all services
  - Update all `http.Server{}` instances to use TLS config from `pkg/tls/server.go`
  - Update all `http.Client{}` instances to use TLS config from `pkg/tls/client.go`

- [ ] **Step 111** [W] ~15m: Wire mTLS into gRPC connections
  - Wotan gRPC server: require client certs
  - Wotan gRPC clients: present service certs
  - `pkg/wotan-client/client.go`: use TLS transport credentials

- [ ] **Step 112** [B] ~2m: Build with mTLS
  ```bash
  cd /home/muck/unheaded && go build ./... 2>&1 | tail -10
  ```

- [ ] **Step 113** [V]: Build passes with mTLS.

- [ ] **Step 114** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "[PLAN S34] Steps 108-113: mTLS server + client config, wired into all services and gRPC"
  ```

### mTLS Integration Testing

- [ ] **Step 115** [W] ~20m: Write `tests/integration/mtls_test.go`
  - Service A → Service B with valid certs → 200
  - Service A → Service B with expired cert → connection refused
  - Service A → Service B with wrong CA → connection refused
  - Service A → Service B without cert → connection refused
  - gRPC connection with valid certs → stream success
  - gRPC connection without certs → stream failure

- [ ] **Step 116** [B] ~2m: Run mTLS integration tests
  ```bash
  cd /home/muck/unheaded && go test ./tests/integration/ -run "MTLS" -v -count=1 2>&1 | tail -30
  ```

- [ ] **Step 117** [V]: mTLS integration tests pass.

### NixOS Container TLS Configuration

- [ ] **Step 118** [W] ~20m: Update `nix/modules/tls.nix` — certificate distribution to containers
  - Mount CA cert to `/etc/unheaded/ca.pem` in each container
  - Mount service cert+key to `/etc/unheaded/tls/` in each container
  - Certificate file permissions: 0400 (owner read only)

- [ ] **Step 119** [W] ~15m: Update `nix/containers/*.nix` — enable TLS environment variables
  ```nix
  environment.variables = {
    UNHEADED_TLS_CERT = "/etc/unheaded/tls/cert.pem";
    UNHEADED_TLS_KEY = "/etc/unheaded/tls/key.pem";
    UNHEADED_TLS_CA = "/etc/unheaded/ca.pem";
  };
  ```

- [ ] **Step 120** [W] ~15m: Update Docker Compose with TLS volume mounts
  ```yaml
  volumes:
    - ./certs/ca.pem:/etc/unheaded/ca.pem:ro
    - ./certs/${SERVICE_NAME}.pem:/etc/unheaded/tls/cert.pem:ro
    - ./certs/${SERVICE_NAME}-key.pem:/etc/unheaded/tls/key.pem:ro
  ```

- [ ] **Step 121** [W] ~15m: Create `scripts/gen-dev-certs.sh` — generate dev certificates
  ```bash
  #!/bin/bash
  # Generate CA + per-service certs for development
  # Uses Go's crypto/x509 via a helper binary
  ```

- [ ] **Step 122** [B] ~1m: Generate dev certificates
  ```bash
  cd /home/muck/unheaded && bash scripts/gen-dev-certs.sh
  ```

- [ ] **Step 123** [V]: Dev certs generated. All services can start with TLS.

- [ ] **Step 124** [B] ~3m: Run full test suite
  ```bash
  cd /home/muck/unheaded && go test ./... 2>&1 | grep -E "FAIL|ok" | tail -40
  ```

- [ ] **Step 125** [V]: All tests pass.

- [ ] **Step 126** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(tls): mTLS service mesh — zero plaintext between services

- Internal CA with per-service certificate generation
- TLS 1.3 minimum, client cert verification
- All HTTP and gRPC connections encrypted
- NixOS, Docker Compose, and dev cert configs
- Integration tests verify cert validation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Gateway TLS Termination

- [ ] **Step 127** [W] ~15m: Update nginx gateway config for HTTP/3 + TLS 1.3
  - QUIC/HTTP3 listener
  - Client-facing TLS termination (Let's Encrypt or self-signed)
  - Backend connections via mTLS to services
  - HSTS header (already enabled)

- [ ] **Step 128** [W] ~10m: Update `nix/containers/gateway.nix` with TLS config

- [ ] **Step 129** [B] ~1m: Validate nginx config
  ```bash
  cd /home/muck/unheaded && nginx -t -c services/gateway/config/nginx.conf 2>&1 || echo "nginx not installed — config validated syntactically"
  ```

- [ ] **Step 130** [V]: Gateway config valid.

- [ ] **Step 131** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(gateway): HTTP/3 + QUIC + TLS 1.3 termination with mTLS backend

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 132** [V]: **PHASE 4 EXIT GATE** — All service-to-service communication encrypted via mTLS. Gateway terminates external TLS. No plaintext on any wire. All tests pass.
  - If pass → proceed
  - If fail → DO NOT PROCEED. Fix within this phase.

---

## PHASE 5: LICH FINDINGS REMEDIATION (Steps 133-148)

**Goal**: Fix all CRITICAL and HIGH findings from Lich D1-D6 campaigns.
**Prerequisite**: Phase 2 exit gate passed (campaigns complete, findings documented)
**Time**: ~3 hours
**Agent**: Agent B (BlackMage → Developer handoff)

### Triage Findings

- [ ] **Step 133** [R]: Read Lich campaign results
  ```bash
  cat /home/muck/unheaded/docs/security/lich-campaigns-s34.md
  ```

- [ ] **Step 134** [V]: All CRITICAL and HIGH findings identified. Fix plan for each.

### Fix Critical Findings

- [ ] **Step 135** [W] ~30m: Fix D4 finding — implement 128-bit trace ID generation for production
  - `pkg/trace/traceid.go` — UUIDv7 generation (time-ordered, k-sortable)
  - Flow label used only as BPF map key hint, NOT as trace identity
  - Production traces use full 128-bit ID in Wotan messages

- [ ] **Step 136** [W] ~15m: `pkg/trace/traceid_test.go` — uniqueness, ordering, collision resistance

- [ ] **Step 137** [B]: Run trace ID tests
  ```bash
  cd /home/muck/unheaded && go test ./pkg/trace/... -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 138** [V]: Trace ID tests pass. Zero collision in 1M generation test.

- [ ] **Step 139** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "[PLAN S34] Steps 135-138: 128-bit UUIDv7 trace IDs for production"
  ```

### Fix High Findings

- [ ] **Step 140** [W] ~20m: Fix D1/D6 findings — BPF map permission hardening
  - ROM_MAP: read-only after load (BPF_F_RDONLY_PROG flag)
  - Add map freeze after ROM loading completes
  - Verify: non-root process cannot write to frozen maps

- [ ] **Step 141** [W] ~20m: Fix D3 findings — SYSCALL topic input validation
  - Validate SYSCALL message source against authenticated service list
  - Rate limit SYSCALL messages per source
  - Reject invalid syscall numbers at Wotan topic filter level

- [ ] **Step 142** [W] ~20m: Fix D5 findings — SYSCALL handler defensive coding
  - Bounds check all syscall arguments
  - Default-deny for unknown syscall numbers
  - Error counters for rejected syscalls (Prometheus metric)

- [ ] **Step 143** [B] ~3m: Run security tests
  ```bash
  cd /home/muck/unheaded && go test ./tests/security/... -v -count=1 2>&1 | tail -30
  ```

- [ ] **Step 144** [V]: All security tests pass with fixes applied.

- [ ] **Step 145** [B] ~3m: Run full test suite
  ```bash
  cd /home/muck/unheaded && go test ./... 2>&1 | grep -E "FAIL|ok" | tail -40
  ```

- [ ] **Step 146** [V]: All tests pass. Zero regressions.

- [ ] **Step 147** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "fix(security): remediate Lich D1-D6 findings — map permissions, syscall validation, trace IDs

- BPF map freeze after ROM load (D1/D6 TOCTOU fix)
- SYSCALL topic input validation + rate limiting (D3)
- SYSCALL handler bounds checking + default-deny (D5)
- 128-bit UUIDv7 trace IDs replace 20-bit flow labels (D4)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 148** [V]: **PHASE 5 EXIT GATE** — All CRITICAL and HIGH Lich findings fixed. Security tests verify fixes. All tests pass.
  - If pass → proceed to Phase 6
  - If fail → DO NOT PROCEED. Fix within this phase.

---

## PHASE 6: PRODUCTION eBPF — packet_marker XDP (Steps 149-192)

**Goal**: Implement the REAL packet_marker XDP program. Stamp trace_id into IPv6 flow label at wire speed. Port Doom-proven patterns to production.
**Prerequisite**: Phases 0, 3 exit gates passed. BPF toolchain verified.
**Time**: ~8 hours
**Agent**: Coordinator + Agent D (eBPF Focus)

### Port Doom XDP Patterns

- [ ] **Step 149** [R]: Study Doom XDP attachment pattern
  ```bash
  cat /home/muck/unheaded/crates/monad-cpu-ebpf/src/main.rs | head -100
  ```

- [ ] **Step 150** [R]: Study Doom map pinning pattern
  ```bash
  ls /home/muck/unheaded/crates/monad-cpu-ebpf/src/ && cat /home/muck/unheaded/scripts/doom/doom-ring.sh | head -50
  ```

- [ ] **Step 151** [V]: Doom XDP patterns understood: program structure, map definitions, XDP return codes, aya attachment flow.

### Implement packet_marker BPF Program

- [ ] **Step 152** [W] ~60m: Implement `ebpf/packet-marker/src/main.rs` — production XDP packet marker
  ```rust
  // REAL packet_marker XDP program (not the stub)
  // Attaches at XDP hook on ingress interfaces
  // For every IPv6 packet:
  //   1. Generate or extract trace_id (20-bit flow label as BPF key)
  //   2. Stamp trace metadata into BPF map (TRACE_MAP)
  //   3. Record timestamp (ktime_get_ns)
  //   4. Record source/dest IP, port, protocol
  //   5. Return XDP_PASS (let packet continue to stack)
  //
  // Map layout:
  //   TRACE_MAP: HashMap<flow_label, TraceEntry>
  //   STATS_MAP: Array<StatsEntry> (per-CPU counters)
  ```

- [ ] **Step 153** [W] ~30m: Define `ebpf/packet-marker/src/common.rs` — shared types
  ```rust
  #[repr(C)]
  pub struct TraceEntry {
      pub trace_id: u128,        // UUIDv7 (set by userspace, read by BPF)
      pub timestamp_ns: u64,     // ktime_get_ns()
      pub src_ip: [u8; 16],      // IPv6 source
      pub dst_ip: [u8; 16],      // IPv6 destination
      pub src_port: u16,
      pub dst_port: u16,
      pub protocol: u8,          // TCP/UDP/ICMP
      pub flags: u8,             // TCP flags
      pub packet_len: u16,
      pub hop_count: u8,         // Number of hops observed
      pub _pad: [u8; 3],
  }

  #[repr(C)]
  pub struct StatsEntry {
      pub packets_total: u64,
      pub bytes_total: u64,
      pub traces_created: u64,
      pub traces_updated: u64,
      pub errors: u64,
  }
  ```

- [ ] **Step 154** [W] ~20m: Update `ebpf/packet-marker/Cargo.toml` — Aya dependencies
  ```toml
  [dependencies]
  aya-ebpf = "0.1"
  aya-log-ebpf = "0.1"

  [[bin]]
  name = "packet-marker"
  path = "src/main.rs"
  ```

- [ ] **Step 155** [B] ~3m: Compile packet_marker BPF program
  ```bash
  cd /home/muck/unheaded && cargo build --manifest-path ebpf/packet-marker/Cargo.toml --target bpfel-unknown-none -Z build-std=core --release 2>&1 | tail -20
  ```

- [ ] **Step 156** [V]: BPF program compiles. Binary at `target/bpfel-unknown-none/release/packet-marker`.
  - If fail → Step 157 [D]

- [ ] **Step 157** [D]: Debug BPF compilation errors
  ```bash
  cd /home/muck/unheaded && cargo build --manifest-path ebpf/packet-marker/Cargo.toml --target bpfel-unknown-none -Z build-std=core --release 2>&1
  ```
  - Common issues: no_std violations, BPF verifier-unfriendly patterns (loops, stack size)
  - If stuck after 2 attempts → [STUCK], move to Phase 7

- [ ] **Step 158** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add ebpf/packet-marker/ && git commit -m "[PLAN S34] Steps 152-156: packet_marker BPF program compiles"
  ```

### Implement Userspace Loader

- [ ] **Step 159** [W] ~40m: Implement `cmd/trace-collector/loader.go` — BPF program loader
  ```go
  // Uses cilium/ebpf (already in tree) to:
  // 1. Load compiled BPF program
  // 2. Create/pin TRACE_MAP and STATS_MAP
  // 3. Attach XDP program to specified interface
  // 4. Start userspace reader goroutine
  // 5. Publish trace events to Wotan
  ```

- [ ] **Step 160** [W] ~20m: Implement `cmd/trace-collector/reader.go` — map reader
  ```go
  // Periodically reads TRACE_MAP entries
  // Converts TraceEntry to protobuf
  // Publishes to Wotan topic "traces.packet"
  // Clears processed entries
  ```

- [ ] **Step 161** [W] ~20m: Implement `cmd/trace-collector/loader_test.go`
  - Mock BPF map operations
  - Test trace event conversion
  - Test Wotan publishing
  - Test map cleanup

- [ ] **Step 162** [B] ~2m: Build trace-collector
  ```bash
  cd /home/muck/unheaded && go build ./cmd/trace-collector/... 2>&1 | tail -10
  ```

- [ ] **Step 163** [V]: trace-collector builds.

- [ ] **Step 164** [B] ~1m: Run trace-collector tests
  ```bash
  cd /home/muck/unheaded && go test ./cmd/trace-collector/... -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 165** [V]: trace-collector tests pass.

- [ ] **Step 166** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add cmd/trace-collector/ && git commit -m "[PLAN S34] Steps 159-165: trace-collector userspace loader + reader"
  ```

### BPF Verifier Trial

- [ ] **Step 167** [S][B] ~5m: Load packet_marker into kernel
  ```bash
  cd /home/muck/unheaded && sudo ./cmd/trace-collector/trace-collector --interface lo --program ebpf/packet-marker/target/bpfel-unknown-none/release/packet-marker --dry-run 2>&1 | tail -20
  ```

- [ ] **Step 168** [V]: BPF verifier accepts the program. prog_id assigned.
  - If fail → Step 169 [D]

- [ ] **Step 169** [D]: Debug BPF verifier rejection
  ```bash
  sudo bpftool prog load /path/to/packet-marker /sys/fs/bpf/test_pm type xdp 2>&1
  ```
  - Common issues: unbounded loops, stack overflow (>512 bytes), invalid map access
  - Fix pattern: add bounds checks, use BPF helper calls, reduce stack usage
  - If stuck after 2 attempts → [STUCK], document verifier error

- [ ] **Step 170** [S][B] ~2m: Attach to loopback and generate test traffic
  ```bash
  cd /home/muck/unheaded && sudo ./cmd/trace-collector/trace-collector --interface lo &
  sleep 2 && curl -6 http://[::1]:8080/health 2>/dev/null; sleep 1
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/trace_map 2>&1 | head -20
  ```

- [ ] **Step 171** [V]: **FIRST PRODUCTION TRACE** — TRACE_MAP contains entry for the curl request. Timestamp, src/dst IP, port, protocol all populated.
  - If pass → **CELEBRATE.** This is the product.
  - If fail → debug map contents, check XDP return codes

- [ ] **Step 172** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(ebpf): FIRST PRODUCTION PACKET TRACE — packet_marker XDP stamps trace metadata

The packet_marker XDP program:
- Attaches at XDP hook on network interfaces
- Stamps trace_id, timestamp, src/dst IP, ports, protocol
- Stores in pinned TRACE_MAP (BPF HashMap)
- Per-CPU STATS_MAP counters
- Passes through BPF verifier
- trace-collector reads maps and publishes to Wotan

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Wotan Integration

- [ ] **Step 173** [W] ~20m: Wire trace-collector → Wotan publishing
  - Topic: `traces.packet`
  - Message format: protobuf TraceEvent
  - Include trace_id, timestamp, source, destination, latency
  - Batch publishing: collect N events, publish as batch

- [ ] **Step 174** [W] ~15m: Write `tests/integration/trace_wotan_test.go`
  - Start Wotan (mock or real)
  - Start trace-collector
  - Generate traffic
  - Subscribe to `traces.packet`
  - Verify trace event received with correct fields

- [ ] **Step 175** [B] ~2m: Run trace-Wotan integration test
  ```bash
  cd /home/muck/unheaded && go test ./tests/integration/ -run "TraceWotan" -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 176** [V]: Trace events flow from BPF → trace-collector → Wotan.

- [ ] **Step 177** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "[PLAN S34] Steps 173-176: trace-collector → Wotan publishing pipeline"
  ```

### Performance Baseline

- [ ] **Step 178** [S][B] ~5m: Benchmark packet_marker XDP performance
  ```bash
  cd /home/muck/unheaded && sudo ./cmd/trace-collector/trace-collector --interface lo --benchmark 2>&1 | tail -20
  ```

- [ ] **Step 179** [V]: Record baseline: packets/sec, traces/sec, CPU overhead, map operations/sec

- [ ] **Step 180** [B] ~5m: Run iperf3 through XDP-attached interface
  ```bash
  iperf3 -s -D && iperf3 -c ::1 -t 10 -V 2>&1 | tail -10
  ```

- [ ] **Step 181** [V]: iperf3 throughput with packet_marker attached vs without. Record overhead percentage.
  - Target: <5% throughput reduction
  - If >10% → optimize BPF program (reduce map operations per packet)

- [ ] **Step 182** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "[PLAN S34] Steps 178-181: packet_marker performance baseline recorded"
  ```

### packet_marker Hardening

- [ ] **Step 183** [W] ~20m: Add BPF map freeze after initialization
  - TRACE_MAP schema: frozen after first write (structure immutable, values mutable)
  - STATS_MAP: per-CPU array, no freeze needed

- [ ] **Step 184** [W] ~15m: Add per-hop authentication stub in XDP
  - Check source interface index against allowed list (BPF array map)
  - Drop packets from unknown interfaces
  - Counter: `packets_dropped_auth`

- [ ] **Step 185** [W] ~20m: Implement packet_marker fuzz targets
  ```go
  // tests/fuzz/packet_marker_fuzz_test.go
  // Fuzz the trace event parser
  // Fuzz the map entry converter
  // Fuzz the protobuf serializer
  ```

- [ ] **Step 186** [B] ~1m: Run fuzz targets (30s each)
  ```bash
  cd /home/muck/unheaded && go test ./tests/fuzz/ -run "FuzzPacketMarker" -fuzztime 30s 2>&1 | tail -10
  ```

- [ ] **Step 187** [V]: Fuzz targets run. Zero crashes.

- [ ] **Step 188** [B] ~3m: Full test suite
  ```bash
  cd /home/muck/unheaded && go test ./... 2>&1 | grep -E "FAIL|ok" | tail -40
  ```

- [ ] **Step 189** [V]: All tests pass.

- [ ] **Step 190** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(ebpf): packet_marker hardening — map freeze, per-hop auth, fuzz targets

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 191** [V]: **PHASE 6 EXIT GATE** — packet_marker XDP program:
  1. Compiles to BPF bytecode ✓
  2. Passes BPF verifier ✓
  3. Attaches to network interface ✓
  4. Stamps trace metadata for every IPv6 packet ✓
  5. trace-collector reads maps and publishes to Wotan ✓
  6. Performance overhead <5% ✓
  7. Fuzz targets clean ✓
  8. All tests pass ✓
  - If ALL pass → Phases 7, 8, 9 unlocked (parallel)
  - If ANY fail → DO NOT PROCEED. Debug within this phase.

- [ ] **Step 192** [C]: **MAJOR COMMIT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(ebpf): production packet_marker complete — first real packet trace

Production-grade XDP packet marker:
- Stamps trace_id, timestamp, src/dst IP, ports, protocol at wire speed
- trace-collector reads BPF maps, publishes to Wotan
- <5% throughput overhead
- Map freeze, per-hop auth, fuzz-tested
- This is the product.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

---

## PHASE 7: flow_tracker — Connection Tracking (Steps 193-216) [P]

**Goal**: Implement flow_tracker BPF program. Track connection state (new/established/closing) via TC or kprobe.
**Prerequisite**: Phase 6 exit gate passed
**Time**: ~5 hours
**Agent**: Agent D (eBPF Focus)

- [ ] **Step 193** [W] ~60m: Implement `ebpf/flow-tracker/src/main.rs`
  ```rust
  // TC (Traffic Control) or kprobe program
  // Tracks TCP connection state transitions
  // Maps: FLOW_MAP: HashMap<5-tuple, FlowState>
  // FlowState: { state: u8, start_ns: u64, last_seen_ns: u64, packets: u64, bytes: u64 }
  // States: NEW(0), ESTABLISHED(1), FIN_WAIT(2), CLOSED(3)
  ```

- [ ] **Step 194** [W] ~20m: Shared types in `ebpf/flow-tracker/src/common.rs`

- [ ] **Step 195** [B] ~3m: Compile flow_tracker
  ```bash
  cd /home/muck/unheaded && cargo build --manifest-path ebpf/flow-tracker/Cargo.toml --target bpfel-unknown-none -Z build-std=core --release 2>&1 | tail -20
  ```

- [ ] **Step 196** [V]: Compiles to BPF. If fail → debug, if stuck → [STUCK].

- [ ] **Step 197** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 198** [W] ~30m: Implement userspace reader in `cmd/trace-collector/flow_reader.go`
  - Read FLOW_MAP periodically
  - Publish flow events to Wotan topic `traces.flow`
  - Expire stale flows (no packets in 60s)

- [ ] **Step 199** [W] ~20m: `cmd/trace-collector/flow_reader_test.go`

- [ ] **Step 200** [B] ~2m: Run flow reader tests
  ```bash
  cd /home/muck/unheaded && go test ./cmd/trace-collector/... -run "Flow" -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 201** [V]: Flow reader tests pass.

- [ ] **Step 202** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 203** [S][B] ~5m: Load flow_tracker and generate TCP traffic
  ```bash
  sudo ./cmd/trace-collector/trace-collector --interface lo --enable-flow-tracker &
  sleep 2 && curl -6 http://[::1]:8080/health 2>/dev/null; sleep 1
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/flow_map 2>&1 | head -20
  ```

- [ ] **Step 204** [V]: FLOW_MAP shows TCP connection with state=ESTABLISHED.

- [ ] **Step 205** [W] ~15m: Integration test — flow events in Wotan

- [ ] **Step 206** [B] ~2m: Run integration test
  ```bash
  cd /home/muck/unheaded && go test ./tests/integration/ -run "FlowTracker" -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 207** [V]: Flow events published to Wotan.

- [ ] **Step 208** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 209** [W] ~20m: flow_tracker fuzz targets

- [ ] **Step 210** [B] ~1m: Run fuzz (30s)
  ```bash
  cd /home/muck/unheaded && go test ./tests/fuzz/ -run "FuzzFlowTracker" -fuzztime 30s 2>&1 | tail -10
  ```

- [ ] **Step 211** [V]: Zero crashes.

- [ ] **Step 212** [B] ~3m: Full test suite
  ```bash
  cd /home/muck/unheaded && go test ./... 2>&1 | grep -E "FAIL|ok" | tail -40
  ```

- [ ] **Step 213** [V]: All pass.

- [ ] **Step 214** [C]: **COMMIT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(ebpf): flow_tracker TC program — connection state tracking at wire speed

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 215** [V]: **PHASE 7 EXIT GATE** — flow_tracker compiles, loads, tracks TCP states, publishes to Wotan, fuzz clean.

- [ ] **Step 216**: Reserved for debug overflow.

---

## PHASE 8: latency_probe — RTT Measurement (Steps 217-240) [P]

**Goal**: Implement latency_probe BPF program. Measure RTT via tracepoints on TCP events.
**Prerequisite**: Phase 6 exit gate passed
**Time**: ~5 hours
**Agent**: Agent E (eBPF Focus)

- [ ] **Step 217** [W] ~60m: Implement `ebpf/latency-probe/src/main.rs`
  ```rust
  // Tracepoint program on tcp_probe / tcp_rcv_established
  // Records SYN-sent timestamp → ACK-received timestamp = RTT
  // Maps: LATENCY_MAP: HashMap<5-tuple, LatencyEntry>
  // LatencyEntry: { rtt_ns: u64, min_rtt_ns: u64, max_rtt_ns: u64, samples: u64 }
  ```

- [ ] **Step 218** [W] ~20m: Shared types

- [ ] **Step 219** [B] ~3m: Compile latency_probe
  ```bash
  cd /home/muck/unheaded && cargo build --manifest-path ebpf/latency-probe/Cargo.toml --target bpfel-unknown-none -Z build-std=core --release 2>&1 | tail -20
  ```

- [ ] **Step 220** [V]: Compiles. If fail → debug, if stuck → [STUCK].

- [ ] **Step 221** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 222** [W] ~30m: Implement userspace reader `cmd/trace-collector/latency_reader.go`
  - Read LATENCY_MAP
  - Publish to Wotan topic `traces.latency`
  - Expose as Prometheus histogram: `unheaded_packet_rtt_seconds`

- [ ] **Step 223** [W] ~20m: Tests

- [ ] **Step 224** [B] ~2m: Run tests
  ```bash
  cd /home/muck/unheaded && go test ./cmd/trace-collector/... -run "Latency" -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 225** [V]: Latency reader tests pass.

- [ ] **Step 226** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 227** [S][B] ~5m: Load and test
  ```bash
  sudo ./cmd/trace-collector/trace-collector --interface lo --enable-latency-probe &
  sleep 2 && curl -6 http://[::1]:8080/health 2>/dev/null; sleep 1
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/latency_map 2>&1 | head -20
  ```

- [ ] **Step 228** [V]: LATENCY_MAP shows RTT measurement.

- [ ] **Step 229** [W] ~15m: Integration test — latency events in Wotan

- [ ] **Step 230** [B] ~2m: Run integration test

- [ ] **Step 231** [V]: Latency events published to Wotan.

- [ ] **Step 232** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 233** [W] ~15m: Fuzz targets

- [ ] **Step 234** [B] ~1m: Run fuzz (30s)

- [ ] **Step 235** [V]: Zero crashes.

- [ ] **Step 236** [B] ~3m: Full test suite

- [ ] **Step 237** [V]: All pass.

- [ ] **Step 238** [C]: **COMMIT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(ebpf): latency_probe tracepoint program — RTT measurement at kernel level

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 239** [V]: **PHASE 8 EXIT GATE** — latency_probe compiles, loads, measures RTT, publishes to Wotan, fuzz clean.

- [ ] **Step 240**: Reserved for debug overflow.

---

## PHASE 9: trace-collector Unification (Steps 241-260) [P]

**Goal**: Unify all three BPF programs under one trace-collector binary. Single control surface.
**Prerequisite**: Phase 6 exit gate passed
**Time**: ~3 hours
**Agent**: Agent F

- [ ] **Step 241** [W] ~40m: Refactor `cmd/trace-collector/main.go` — unified loader
  ```go
  // Flags:
  // --interface eth0           (required)
  // --enable-packet-marker     (default: true)
  // --enable-flow-tracker      (default: true)
  // --enable-latency-probe     (default: true)
  // --wotan-addr 10.10.10.10:9090
  // --metrics-addr :9100
  // Graceful shutdown: detach all BPF programs, unpin maps
  ```

- [ ] **Step 242** [W] ~20m: Health endpoints — `/health`, `/ready`, `/metrics`
  - `/health`: all enabled BPF programs loaded
  - `/ready`: at least one trace event processed
  - `/metrics`: Prometheus — packets traced, flows tracked, latencies measured

- [ ] **Step 243** [W] ~20m: Wotan topic router — publishes to correct topics
  - `traces.packet` — from packet_marker
  - `traces.flow` — from flow_tracker
  - `traces.latency` — from latency_probe
  - `traces.correlated` — cross-correlated events (packet + flow + latency for same 5-tuple)

- [ ] **Step 244** [W] ~20m: Cross-correlation engine
  - Match packet trace → flow state → latency measurement by 5-tuple
  - Emit correlated trace event with full context
  - This is the "aha" moment: one event tells the whole story

- [ ] **Step 245** [W] ~20m: Tests for unified loader + correlator

- [ ] **Step 246** [B] ~2m: Build and test
  ```bash
  cd /home/muck/unheaded && go build ./cmd/trace-collector/... && go test ./cmd/trace-collector/... -v -count=1 2>&1 | tail -30
  ```

- [ ] **Step 247** [V]: Unified trace-collector builds and tests pass.

- [ ] **Step 248** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 249** [S][B] ~5m: Integration test — all three programs loaded
  ```bash
  sudo ./cmd/trace-collector/trace-collector --interface lo &
  sleep 3 && curl -6 http://[::1]:8080/health; sleep 2
  curl http://localhost:9100/metrics 2>/dev/null | grep unheaded_
  ```

- [ ] **Step 250** [V]: All three BPF programs loaded. Prometheus metrics flowing. Correlated trace events in Wotan.

- [ ] **Step 251** [C]: **COMMIT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(trace-collector): unified eBPF loader — packet_marker + flow_tracker + latency_probe

- Single binary loads all three BPF programs
- Cross-correlation engine matches events by 5-tuple
- Health/ready/metrics endpoints
- Wotan topic routing: traces.{packet,flow,latency,correlated}

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 252** [V]: **PHASE 9 EXIT GATE** — trace-collector binary loads all 3 BPF programs, correlates events, publishes to Wotan, exposes metrics.

- [ ] **Steps 253-260**: Reserved for debug overflow.

---

## PHASE 10: OBSERVABILITY PIPELINE INTEGRATION (Steps 261-280)

**Goal**: Wire the full pipeline: eBPF → trace-collector → Wotan → dashboard-backend → browser.
**Prerequisite**: Phases 6, 7, 8, 9 exit gates passed
**Time**: ~4 hours
**Agent**: Coordinator

- [ ] **Step 261** [W] ~30m: Update `cmd/dashboard-backend/` — subscribe to trace topics
  - Subscribe to `traces.correlated` (primary)
  - Subscribe to `traces.packet`, `traces.flow`, `traces.latency` (raw)
  - Maintain in-memory trace buffer (last 1000 events)
  - Push new events via WebSocket to connected dashboards

- [ ] **Step 262** [W] ~20m: WebSocket message format for traces
  ```json
  {
    "type": "trace",
    "data": {
      "trace_id": "01945a3b-...",
      "timestamp": 1708900000000,
      "src": "10.10.10.20:8000",
      "dst": "10.10.10.10:9090",
      "protocol": "TCP",
      "rtt_ms": 0.45,
      "flow_state": "ESTABLISHED",
      "hop_count": 1,
      "packet_len": 128
    }
  }
  ```

- [ ] **Step 263** [W] ~20m: Tests for dashboard trace subscription

- [ ] **Step 264** [B] ~2m: Build and test dashboard-backend
  ```bash
  cd /home/muck/unheaded && go build ./cmd/dashboard-backend/... && go test ./cmd/dashboard-backend/... -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 265** [V]: Dashboard backend receives and forwards trace events.

- [ ] **Step 266** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 267** [W] ~30m: E2E pipeline test — `tests/e2e/trace_pipeline_test.go`
  - Start Wotan
  - Start trace-collector (mock BPF or loopback)
  - Start dashboard-backend
  - Connect WebSocket client
  - Generate traffic
  - Verify: WebSocket client receives trace event within 5s

- [ ] **Step 268** [B] ~5m: Run E2E pipeline test
  ```bash
  cd /home/muck/unheaded && go test ./tests/e2e/ -run "TracePipeline" -v -count=1 -timeout 30s 2>&1 | tail -30
  ```

- [ ] **Step 269** [V]: **FIRST FULL-PIPELINE TRACE** — packet → BPF → map → collector → Wotan → dashboard → WebSocket → browser.
  - This is the product working end-to-end.

- [ ] **Step 270** [C]: **COMMIT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(pipeline): end-to-end trace pipeline — eBPF → Wotan → dashboard → browser

The full observability pipeline is live:
1. packet_marker stamps every IPv6 packet at XDP
2. flow_tracker tracks TCP connection state at TC
3. latency_probe measures RTT at kernel tracepoints
4. trace-collector reads BPF maps, correlates, publishes to Wotan
5. dashboard-backend subscribes, pushes via WebSocket
6. Browser receives real-time trace events

E2E test verifies the full chain.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 271** [V]: **PHASE 10 EXIT GATE** — Full pipeline operational. E2E test passes. Trace events reach the browser.

- [ ] **Steps 272-280**: Reserved for debug overflow.

---

## PHASE 11: DASHBOARD TRACE VISUALIZATION (Steps 281-296) [P]

**Goal**: Build real-time trace visualization in the dashboard. Packet flow diagrams, latency charts, connection state.
**Prerequisite**: Phase 10 exit gate passed
**Time**: ~4 hours
**Agent**: Agent G (Frontend Focus)

- [ ] **Step 281** [W] ~40m: Update `dashboard/js/trace-viewer.js` — real-time trace table
  - Live-updating table of recent trace events
  - Columns: Timestamp, Source, Destination, Protocol, RTT, Flow State, Trace ID
  - Click to expand: full trace detail
  - Filter: by source, destination, protocol, RTT threshold

- [ ] **Step 282** [W] ~40m: Implement `dashboard/js/packet-flow-diagram.js` — network topology view
  - SVG-based node graph showing services
  - Animated packets flowing between nodes
  - Color-coded by latency (green <1ms, yellow <10ms, red >10ms)
  - Click node for service health detail

- [ ] **Step 283** [W] ~30m: Implement `dashboard/js/latency-chart.js` — real-time latency histogram
  - Time-series chart of RTT distribution
  - Percentiles: p50, p90, p99
  - Anomaly highlighting (>3σ from mean)

- [ ] **Step 284** [W] ~20m: Update `dashboard/index.html` — integrate trace views
  - Tab: Packet Flow (default)
  - Tab: Trace Table
  - Tab: Latency
  - Tab: Doom (existing)

- [ ] **Step 285** [W] ~15m: WebSocket reconnection with backoff in dashboard JS

- [ ] **Step 286** [V]: Dashboard displays trace data visually. Latency chart updates in real-time.

- [ ] **Step 287** [C]: **COMMIT**
  ```bash
  cd /home/muck/unheaded && git add dashboard/ && git commit -m "feat(dashboard): real-time trace visualization — packet flow, latency charts, trace table

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 288** [V]: **PHASE 11 EXIT GATE** — Dashboard shows live packet traces, network topology, and latency distribution. All three views update in real-time.

- [ ] **Steps 289-296**: Reserved for additional UI polish / debug overflow.

---

## PHASE 12: SERVICE DISCOVERY (Steps 297-308) [P]

**Goal**: Replace hardcoded IPs with dynamic service discovery via Wotan announcements.
**Prerequisite**: Phase 10 exit gate passed
**Time**: ~2 hours
**Agent**: Agent H

- [ ] **Step 297** [W] ~30m: Implement `pkg/discovery/discovery.go`
  - Services announce to Wotan topic `system.discovery`
  - Announcement: service name, address, port, health status
  - TTL: 30s (re-announce every 15s)
  - Resolution: lookup service name → address:port

- [ ] **Step 298** [W] ~20m: Tests

- [ ] **Step 299** [B] ~2m: Run tests
  ```bash
  cd /home/muck/unheaded && go test ./pkg/discovery/... -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 300** [V]: Discovery tests pass.

- [ ] **Step 301** [W] ~30m: Wire discovery into all services — replace hardcoded addresses

- [ ] **Step 302** [B] ~2m: Build all
  ```bash
  cd /home/muck/unheaded && go build ./... 2>&1 | tail -10
  ```

- [ ] **Step 303** [V]: Build passes.

- [ ] **Step 304** [B] ~3m: Full test suite
  ```bash
  cd /home/muck/unheaded && go test ./... 2>&1 | grep -E "FAIL|ok" | tail -40
  ```

- [ ] **Step 305** [V]: All pass.

- [ ] **Step 306** [C]: **COMMIT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(discovery): Wotan-based service discovery — replace hardcoded IPs

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 307** [V]: **PHASE 12 EXIT GATE** — No hardcoded service addresses. All services use discovery.

- [ ] **Step 308**: Reserved.

---

## PHASE 13: WOTAN HARDENING (Steps 309-320) [P]

**Goal**: Fix Wotan reliability issues — silent failures, message loss detection, reconnection hardening.
**Prerequisite**: Phase 10 exit gate passed
**Time**: ~2 hours
**Agent**: Agent A

- [ ] **Step 309** [W] ~20m: Fix silent Wotan failures (P1 #19) — implement fallback message queue
  - When Wotan is unreachable, buffer messages locally (ring buffer, 1000 messages max)
  - On reconnect, flush buffer
  - Metric: `wotan_buffered_messages` gauge

- [ ] **Step 310** [W] ~20m: Add message delivery confirmation
  - Publisher receives ack/nack from Wotan
  - Retry on nack (3 attempts, exponential backoff)
  - Dead letter topic for permanently failed messages

- [ ] **Step 311** [W] ~15m: Tests for fallback queue + delivery confirmation

- [ ] **Step 312** [B] ~2m: Run Wotan tests
  ```bash
  cd /home/muck/unheaded && go test ./pkg/wotan-client/... -v -count=1 -race 2>&1 | tail -20
  ```

- [ ] **Step 313** [V]: Wotan client tests pass with race detector.

- [ ] **Step 314** [C]: **COMMIT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "fix(wotan): message delivery reliability — fallback queue, ack/nack, dead letters

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 315** [V]: **PHASE 13 EXIT GATE** — Wotan messages never silently lost. Fallback queue operational.

- [ ] **Steps 316-320**: Reserved.

---

## PHASE 14: E2E INTEGRATION TEST SUITE (Steps 321-336)

**Goal**: Comprehensive E2E test: browser → gateway → auth → services → eBPF → Wotan → dashboard.
**Prerequisite**: Phases 10, 11, 12, 13 exit gates passed
**Time**: ~3 hours
**Agent**: Coordinator

- [ ] **Step 321** [W] ~40m: Implement `tests/e2e/full_pipeline_test.go`
  - Start all services via Docker Compose or direct
  - Generate traffic through gateway
  - Verify: auth enforcement (401 without token)
  - Verify: authenticated request succeeds (200)
  - Verify: trace appears in Wotan within 5s
  - Verify: trace appears on WebSocket within 10s
  - Verify: /health and /ready on all services
  - Verify: /metrics on all services expose unheaded_* metrics
  - Verify: service discovery resolves all services

- [ ] **Step 322** [W] ~20m: Implement `tests/e2e/security_e2e_test.go`
  - mTLS required between services
  - Unauthenticated request → 401
  - Wrong cert → connection refused
  - Rate limiting works (429 after threshold)

- [ ] **Step 323** [W] ~20m: Implement `tests/e2e/performance_e2e_test.go`
  - 1000 requests through gateway
  - Verify: p99 latency <50ms
  - Verify: zero trace events lost (count matches)
  - Verify: eBPF overhead <5%

- [ ] **Step 324** [B] ~10m: Run E2E suite
  ```bash
  cd /home/muck/unheaded && go test ./tests/e2e/... -v -count=1 -timeout 120s 2>&1 | tail -50
  ```

- [ ] **Step 325** [V]: All E2E tests pass.

- [ ] **Step 326** [C]: **COMMIT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "test(e2e): comprehensive end-to-end test suite — full pipeline verified

- Full pipeline: gateway → auth → services → eBPF → Wotan → dashboard
- Security: mTLS, auth enforcement, rate limiting
- Performance: p99 <50ms, zero trace loss, <5% eBPF overhead

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 327** [V]: **PHASE 14 EXIT GATE** — E2E tests prove the entire system works end-to-end. Auth, tracing, dashboard, performance — all verified.

- [ ] **Steps 328-336**: Reserved.

---

## PHASE 15: DEPLOYMENT PIPELINE (Steps 337-352)

**Goal**: `make deploy` actually works. Single command, full kingdom.
**Prerequisite**: Phase 14 exit gate passed
**Time**: ~3 hours
**Agent**: Agent C

- [ ] **Step 337** [W] ~30m: Update `Makefile` — real deploy target
  ```makefile
  deploy: build test
  	@echo "Deploying Unheaded Kingdom..."
  	docker compose up -d --build
  	@echo "Waiting for services..."
  	./scripts/wait-for-healthy.sh
  	@echo "Kingdom deployed."
  ```

- [ ] **Step 338** [W] ~20m: Implement `scripts/wait-for-healthy.sh`
  - Poll /health on all services (retry 30 times, 2s interval)
  - Report status table
  - Exit 1 if any service unhealthy after 60s

- [ ] **Step 339** [W] ~30m: Update `docker-compose.yml` — all services with health checks, TLS volumes, proper networking

- [ ] **Step 340** [W] ~20m: Update `scripts/setup-host.sh` — prerequisites for deployment
  - Check kernel version
  - Install BPF tools
  - Configure network bridges
  - Generate initial certificates

- [ ] **Step 341** [B] ~1m: Validate docker compose config
  ```bash
  cd /home/muck/unheaded && docker compose config --quiet 2>&1
  ```

- [ ] **Step 342** [V]: Docker Compose config valid.

- [ ] **Step 343** [C]: **COMMIT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "feat(deploy): make deploy actually works — single command, full kingdom

- Docker Compose with health checks, TLS, networking
- wait-for-healthy.sh polls all services
- setup-host.sh prepares the environment

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 344** [V]: **PHASE 15 EXIT GATE** — `make deploy` starts all services. Health checks pass within 60s.

- [ ] **Steps 345-352**: Reserved.

---

## PHASE 16: ALPHA SHIP GATE (Steps 353-370)

**Goal**: Final verification, compliance snapshot, tag v0.1.0-alpha. The Alpha Ascension is COMPLETE.
**Prerequisite**: All previous phase exit gates passed
**Time**: ~2 hours
**Agent**: Coordinator + All

### Final Verification

- [ ] **Step 353** [B] ~5m: Full build
  ```bash
  cd /home/muck/unheaded && go build ./... && cargo test --manifest-path crates/monad-mbc/Cargo.toml 2>&1 | tail -20
  ```

- [ ] **Step 354** [V]: Build passes.

- [ ] **Step 355** [B] ~5m: Full test suite
  ```bash
  cd /home/muck/unheaded && go test ./... 2>&1 | grep -E "FAIL|ok" | wc -l && go test ./... 2>&1 | grep "FAIL" | head -5
  ```

- [ ] **Step 356** [V]: All tests pass. Zero failures.

- [ ] **Step 357** [B] ~2m: Run gosec security scan
  ```bash
  cd /home/muck/unheaded && gosec ./... 2>&1 | tail -20
  ```

- [ ] **Step 358** [V]: No critical security findings.

### Compliance Snapshot

- [ ] **Step 359** [B] ~2m: Generate SBOM
  ```bash
  cd /home/muck/unheaded && go list -m all > sbom-go-modules.txt && wc -l sbom-go-modules.txt
  ```

- [ ] **Step 360** [W] ~20m: Write `docs/ALPHA_RELEASE_NOTES.md`
  - Feature summary
  - Known limitations
  - Security posture
  - Next steps (Age 2: Beta)

### Tag Release

- [ ] **Step 361** [B]: Create annotated tag
  ```bash
  cd /home/muck/unheaded && git tag -a v0.1.0-alpha -m "Alpha Release — The Alpha Ascension

Unheaded v0.1.0-alpha — Production-ready infrastructure in hours, not months.

Highlights:
- eBPF packet tracing (XDP packet_marker, TC flow_tracker, tracepoint latency_probe)
- Real-time trace correlation and dashboard visualization
- 23 microservices with health/ready/metrics
- mTLS service mesh, JWT + API key authentication
- Wotan message bus with reliability guarantees
- Cross-platform container support (LXD, containerd, NixOS, Docker)
- Interchangeable IaC backends (Ansible, Terraform, Puppet, K8s, Chef, Salt)
- Doom-over-IPv6 computational completeness proof

465K+ LOC | 33 days from first commit | One engineer. One AI. One Kingdom."
  ```

- [ ] **Step 362** [V]: Tag created.

- [ ] **Step 363** [C]: **FINAL COMMIT**
  ```bash
  cd /home/muck/unheaded && git add -A && git commit -m "chore: Alpha release preparation — v0.1.0-alpha

- SBOM generated
- Release notes written
- All tests pass
- All security gates pass

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 364** [V]: **PHASE 16 EXIT GATE — ALPHA SHIP GATE**:
  1. All services build ✓
  2. All tests pass (Go + Rust) ✓
  3. gosec clean ✓
  4. eBPF programs load and trace ✓
  5. Full pipeline works end-to-end ✓
  6. Auth on all endpoints ✓
  7. mTLS between all services ✓
  8. Dashboard shows live traces ✓
  9. `make deploy` works ✓
  10. SBOM generated ✓
  11. Release notes written ✓
  12. Tag v0.1.0-alpha created ✓

**THE ALPHA ASCENSION IS COMPLETE.**

- [ ] **Steps 365-370**: Reserved for ceremony.

---

## APPENDIX A: EMERGENCY PROCEDURES

### EP-1: BPF Verifier Rejects Program

**Symptom**: `cargo build` succeeds but `bpftool prog load` fails with verifier error.

1. Read verifier output carefully — it tells you which instruction failed
2. Common fixes:
   - "unbounded loop" → add explicit loop bound (`for i in 0..MAX`)
   - "stack too large" → reduce local variables, use maps instead
   - "invalid map access" → add bounds check before map lookup
   - "unreachable code" → remove dead branches
3. Use `bpftool prog dump xlated` to see the translated program
4. If stuck: simplify program to minimal reproducer, then add features back one by one

### EP-2: Map Pinning Fails

**Symptom**: `bpftool map create` or pin operation fails.

1. Check `/sys/fs/bpf/` exists and is mounted: `mount | grep bpf`
2. If not mounted: `sudo mount -t bpf none /sys/fs/bpf`
3. Check permissions: `ls -la /sys/fs/bpf/unheaded/`
4. Remove stale pins: `sudo rm -rf /sys/fs/bpf/unheaded/`
5. Retry load

### EP-3: mTLS Handshake Failure

**Symptom**: "tls: certificate required" or "x509: certificate signed by unknown authority"

1. Check cert files exist: `ls -la /etc/unheaded/tls/`
2. Verify cert is signed by correct CA: `openssl verify -CAfile ca.pem cert.pem`
3. Check cert expiry: `openssl x509 -in cert.pem -noout -dates`
4. Check SAN matches service name: `openssl x509 -in cert.pem -noout -ext subjectAltName`
5. Regenerate certs: `./scripts/gen-dev-certs.sh`

### EP-4: Wotan Connection Refused

**Symptom**: "connection refused" to Wotan address

1. Check Wotan is running: `docker compose ps wotan`
2. Check port binding: `ss -tlnp | grep 9090`
3. Check network: `ping -c1 10.10.10.10`
4. Restart Wotan: `docker compose restart wotan`
5. Check logs: `docker compose logs wotan --tail 20`

### EP-5: Dashboard WebSocket Disconnects

**Symptom**: Dashboard shows "Disconnected" or trace events stop updating

1. Check dashboard-backend is running: `curl http://localhost:8081/health`
2. Check WebSocket upgrade: browser dev tools → Network → WS tab
3. Check CORS: Origin must be in allowed list
4. Check auth: WebSocket upgrade requires valid JWT
5. Check message size: events >1KB may be rejected (P1 #22 fix)

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Type | Parallel? | Dependencies | Est. Time |
|-------|-------|------|-----------|-------------|-----------|
| 0 | Coordinator | Sequential | No | None | 20 min |
| 1 | Agent A | Auth specialist | Yes (with 2,3) | Phase 0 | 4 hours |
| 2 | Agent B | BlackMage | Yes (with 1,3) | Phase 0 | 6 hours |
| 3 | Agent C | Cleanup | Yes (with 1,2) | Phase 0 | 2 hours |
| 4 | Agent A | Security | No | Phase 1 | 4 hours |
| 5 | Agent B | Fix findings | No | Phase 2 | 3 hours |
| 6 | Coord + Agent D | eBPF core | No | Phases 0,3 | 8 hours |
| 7 | Agent D | eBPF | Yes (with 8,9) | Phase 6 | 5 hours |
| 8 | Agent E | eBPF | Yes (with 7,9) | Phase 6 | 5 hours |
| 9 | Agent F | Integration | Yes (with 7,8) | Phase 6 | 3 hours |
| 10 | Coordinator | Pipeline | No | Phases 7,8,9 | 4 hours |
| 11 | Agent G | Frontend | Yes (with 12,13) | Phase 10 | 4 hours |
| 12 | Agent H | Discovery | Yes (with 11,13) | Phase 10 | 2 hours |
| 13 | Agent A | Wotan | Yes (with 11,12) | Phase 10 | 2 hours |
| 14 | Coordinator | E2E test | No | Phases 11,12,13 | 3 hours |
| 15 | Agent C | Deploy | No | Phase 14 | 3 hours |
| 16 | Coordinator | Ship gate | No | Phase 15 | 2 hours |

### Critical Path

```
Phase 0 (20m) → Phase 6 (8h) → Phase 10 (4h) → Phase 14 (3h) → Phase 15 (3h) → Phase 16 (2h)
= ~20.5 hours on critical path (minimum calendar time: ~4 working days)
```

### Parallelization Savings

Without parallelization: ~60 hours
With parallelization: ~25 hours (including slack)
**Savings: ~35 hours (~58%)**

---

## APPENDIX C: QUICK REFERENCE

### BPF Map Types Used

| Map | Type | Key | Value | Program |
|-----|------|-----|-------|---------|
| TRACE_MAP | HashMap | flow_label (u32) | TraceEntry (64B) | packet_marker |
| STATS_MAP | PerCPU Array | index (u32) | StatsEntry (40B) | packet_marker |
| FLOW_MAP | HashMap | 5-tuple (24B) | FlowState (32B) | flow_tracker |
| LATENCY_MAP | HashMap | 5-tuple (24B) | LatencyEntry (32B) | latency_probe |

### Wotan Topics

| Topic | Publisher | Subscribers |
|-------|-----------|-------------|
| traces.packet | trace-collector | dashboard-backend |
| traces.flow | trace-collector | dashboard-backend |
| traces.latency | trace-collector | dashboard-backend |
| traces.correlated | trace-collector | dashboard-backend, alerting |
| system.discovery | all services | all services |
| system.outage.reports | all services | dashboard-backend, captain |
| alerts.critical | all services | all services |

### Service Port Map

| Service | Port | Protocol |
|---------|------|----------|
| Wotan | 9090 | gRPC + REST |
| Gateway | 443 | HTTPS/HTTP3 |
| Timeguru | 8000 | HTTP |
| Captain | 8001 | HTTP |
| Architect | 8002 | HTTP |
| Micromanager | 8003 | HTTP |
| Monad | 8004 | HTTP |
| Sophia | 8005 | HTTP |
| Dashboard Backend | 8081 | HTTP + WS |
| Kanban App | 8080 | HTTP |
| trace-collector | 9100 | HTTP (metrics) |
| Fenrir's Eye | 8090 | HTTP + WS |

### CLI Cheat Sheet

```bash
# Build everything
make build

# Test everything
make test

# Deploy
make deploy

# Check all service health
for port in 8000 8001 8002 8003 8004 8005 8080 8081 8090 9090; do
  echo -n "Port $port: "; curl -s http://localhost:$port/health | head -1
done

# Check BPF programs
sudo bpftool prog list | grep unheaded

# Check BPF maps
sudo bpftool map list | grep unheaded

# Read trace map
sudo bpftool map dump pinned /sys/fs/bpf/unheaded/trace_map

# Wotan topic list
curl http://localhost:9090/api/v1/topics
```

---

*S34 Battle Plan — Forged 2026-02-23*
*16 Phases. 370 Steps. From spectacle to substance. From Doom to product.*
*The packets will speak. The traces will flow. The Kingdom will see ALL.*
