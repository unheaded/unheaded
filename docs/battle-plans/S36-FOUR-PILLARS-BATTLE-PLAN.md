# S36 FOUR PILLARS BATTLE PLAN — 7 Phases, 280+ Steps — Multi-Agent Execution

**Date**: 2026-02-24
**Sprint**: S36 — The Four Pillars: Port Authority, gRPC-First, Log Aggregation, Service Discovery
**Prerequisite**: S35 complete. Build passes. Tests pass. Working tree committed.
**Target**: All 20+ services on Doom Range high ports, gRPC-first transport, centralized log aggregation visible in dashboard, three-layer service discovery. Zero hardcoded IPs.
**Estimated Duration**: 16-24 hours across 2-3 agent sessions
**Agent Strategy**: 5 agents (A-E) + 1 Coordinator. Phases 0-1 sequential, then Phases 2+4 parallel, Phase 3 after 2, Phase 5 parallel throughout, Phase 6 sequential final.
**Commit Cadence**: Every 4 steps (calculated: max(3, min(5, 280/20)) = 5, reduced to 4 for safety)
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts. Commit before skip. Log everything.
**Go Version**: 1.24.0 (confirmed)
**Repo Root**: ~/tmp/unheaded (or wherever cloned — use `git rev-parse --show-toplevel`)

---

## LEGEND

```
[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
[C] = Commit checkpoint
[STUCK] = Step skipped via Skip Protocol
[BLOCKED] = Step blocked by upstream STUCK step
~Xm = Estimated wall-clock time
```

---

## MULTI-AGENT ASSIGNMENT MATRIX

| Agent | Phase | Domain | Est. Hours | Dependencies | Can Start When |
|-------|-------|--------|-----------|--------------|----------------|
| **Coordinator** | 0 | Foundation + git hygiene | 0.5h | None | Immediately |
| **Agent A** | 1 | Port Authority — migrate all services to Doom Range | 4-6h | Phase 0 complete | After Phase 0 |
| **Agent B** | 2 | gRPC-First Transport — flip defaults, cascade fallback | 4-6h | Phase 1 complete | After Phase 1 |
| **Agent C** | 3 | Log Aggregation — zerolog→Wotan→dashboard | 4-6h | Phase 2 complete | After Phase 2 |
| **Agent D** | 4 | Service Discovery — three-layer, kill hardcoded IPs | 4-6h | Phase 1 complete | After Phase 1 (parallel with B) |
| **Agent E** | 5 | Documentation — all 8 doc layers | 2-3h | Parallel with all | After Phase 1 |
| **Coordinator** | 6 | Integration verification + final commit | 1-2h | All phases complete | Last |

```
DEPENDENCY GRAPH:

  Phase 0 (Foundation)
       │
       ▼
  Phase 1 (Port Authority) ─────────────────────┐
       │                                          │
       ├──────────────┐                           │
       ▼              ▼                           ▼
  Phase 2         Phase 4                    Phase 5
  (gRPC-First)    (Discovery)               (Documentation)
       │          [PARALLEL with 2,3]        [PARALLEL with all]
       ▼
  Phase 3
  (Log Aggregation)
       │
       ▼
  Phase 6 (Integration — ALL must complete)
```

**Critical Path**: 0 → 1 → 2 → 3 → 6
**Parallel Paths**: Phase 4 after Phase 1 (parallel with 2-3). Phase 5 after Phase 1 (parallel with all).

---

## HOW TO USE THIS PLAN WITH CLAUDE CODE CLI

### Option 1: Single Agent (Sequential)
Feed the entire plan to one `claude` session. It will execute phases 0-6 in order.

### Option 2: Multi-Agent (Recommended)
1. **Terminal 1 — Coordinator**: Run Phase 0. When complete, signal Agents A and E.
2. **Terminal 2 — Agent A**: Run Phase 1 (Port Authority). When complete, signal Agents B, D.
3. **Terminal 3 — Agent B**: Run Phase 2 (gRPC-First). When complete, signal Agent C.
4. **Terminal 4 — Agent C**: Run Phase 3 (Log Aggregation).
5. **Terminal 5 — Agent D**: Run Phase 4 (Service Discovery) — starts after Phase 1, parallel with B/C.
6. **Terminal 6 — Agent E**: Run Phase 5 (Documentation) — starts after Phase 1, parallel with all.
7. **Terminal 1 — Coordinator**: Run Phase 6 (Integration) after all others complete.

### Agent Prompt Template
For each agent, paste into `claude`:
```
You are Agent [X] executing Phase [N] of the S36 Four Pillars Battle Plan.
Read these files FIRST:
1. CLAUDE.md
2. docs/battle-plans/S36-FOUR-PILLARS-BATTLE-PLAN.md (find Phase [N])
3. battle-plan.md (for context)

Execute ONLY Phase [N], steps [X] through [Y].
Commit every 4 steps. Use conventional commits: type(scope): description
TDD: tests first. Race detection: -race on all Go tests. All inputs hostile.
When done, run the Phase [N] EXIT GATE and report results.
```

---

## PHASE 0: FOUNDATION (Steps 1-20) — Coordinator

**Goal**: Verify environment, commit outstanding work, establish clean baseline.
**Prerequisite**: S35 complete.
**Time**: ~20 minutes
**Agent**: Coordinator

### Git Hygiene

- [ ] **Step 1** [B] ~30s: Verify repo root and branch
  ```bash
  cd ~/tmp/unheaded && git rev-parse --show-toplevel && git branch --show-current
  ```

- [ ] **Step 2** [V] ~15s: Must be on `main` branch. If not → `git checkout main`.

- [ ] **Step 3** [B] ~30s: Check working tree status
  ```bash
  git status --short
  ```

- [ ] **Step 4** [B] ~1m: Stage and commit any outstanding S35 files
  ```bash
  git add docs/sessions/S35-HANDOFF-MULTI-AGENT-SPRINT.md docs/sessions/S36-CLAUDE-CODE-PROMPT.md docs/battle-plans/S36-FOUR-PILLARS-BATTLE-PLAN.md && git commit -m "docs(s36): add battle plan and handoff for Four Pillars sprint"
  ```

- [ ] **Step 5** [V] ~15s: Working tree clean after commit
  ```bash
  git status --short
  ```
  - If clean → Step 6
  - If dirty → commit remaining files or stash

### Build Baseline

- [ ] **Step 6** [B] ~2m: Full build verification
  ```bash
  cd ~/tmp/unheaded && go build ./...
  ```

- [ ] **Step 7** [V] ~15s: Build must pass with zero errors. If fail → STOP, fix build first.

- [ ] **Step 8** [B] ~5m: Full test verification
  ```bash
  cd ~/tmp/unheaded && go test -race -count=1 ./... 2>&1 | tail -30
  ```

- [ ] **Step 9** [V] ~15s: Tests must pass. Record any known-flaky tests for later. If widespread failure → STOP.

- [ ] **Step 10** [C] ~30s: **COMMIT CHECKPOINT** (if any fixes were needed)

### Toolchain Verification

- [ ] **Step 11** [B] ~15s: Verify Go version
  ```bash
  go version
  ```

- [ ] **Step 12** [V] ~15s: Go >= 1.21 required (we have 1.24.0 — confirmed).

- [ ] **Step 13** [B] ~15s: Verify protoc/gRPC tools available
  ```bash
  which protoc 2>/dev/null && protoc --version || echo "protoc: NOT FOUND (ok — we generate manually)"
  ```

- [ ] **Step 14** [R] ~30s: Read current pkg/config/config.go to understand existing config patterns
  ```bash
  head -50 pkg/config/config.go
  ```

### Pre-Flight Port Audit

- [ ] **Step 15** [B] ~1m: Count all hardcoded old ports in production code (excluding tests)
  ```bash
  grep -rn "\":8080\"\|\":9090\"\|\":9080\"\|\":8000\"\|\":8001\"\|\":8002\"\|\":8003\"\|\":8004\"\|\":8005\"\|\":8006\"\|\":8081\"\|\":6660\"\|\":5555\"" --include="*.go" ~/tmp/unheaded/ | grep -v _test.go | grep -v vendor | grep -v docs/ | wc -l
  ```

- [ ] **Step 16** [B] ~1m: Count all hardcoded 10.10.10 IPs in production code
  ```bash
  grep -rn "10\.10\.10\." --include="*.go" ~/tmp/unheaded/ | grep -v _test.go | grep -v vendor | grep -v docs/ | wc -l
  ```

- [ ] **Step 17** [W] ~30s: Record baseline counts in a scratch file for Phase 6 verification
  ```bash
  echo "PRE-MIGRATION PORT AUDIT $(date)" > ~/tmp/unheaded/.port-audit-baseline.txt
  echo "Old ports in production .go: $(grep -rn '\":8080\"\|\":9090\"\|\":9080\"\|\":8000\"\|\":8001\"\|\":8002\"\|\":8003\"\|\":8004\"\|\":8005\"\|\":8006\"\|\":8081\"\|\":6660\"\|\":5555\"' --include='*.go' ~/tmp/unheaded/ | grep -v _test.go | grep -v vendor | grep -v docs/ | wc -l)" >> ~/tmp/unheaded/.port-audit-baseline.txt
  echo "10.10.10 IPs in production .go: $(grep -rn '10\.10\.10\.' --include='*.go' ~/tmp/unheaded/ | grep -v _test.go | grep -v vendor | grep -v docs/ | wc -l)" >> ~/tmp/unheaded/.port-audit-baseline.txt
  cat ~/tmp/unheaded/.port-audit-baseline.txt
  ```

- [ ] **Step 18** [B] ~30s: List all cmd/ entry points for reference
  ```bash
  ls -1 ~/tmp/unheaded/cmd/
  ```

- [ ] **Step 19** [B] ~30s: List all services/ for reference
  ```bash
  ls -1 ~/tmp/unheaded/services/
  ```

- [ ] **Step 20** [V] ~15s: **PHASE 0 EXIT GATE** — Build passes. Tests pass. Working tree clean. Port audit baseline recorded.
  - If all pass → Signal Agents A and E to begin
  - If fail → Fix before proceeding

---

## PHASE 1: PORT AUTHORITY (Steps 21-105) — Agent A

**Goal**: Migrate ALL services from legacy ports (8000-9100) to Doom Range (16666-26666). Create pkg/ports as single source of truth. Zero port conflicts.
**Prerequisite**: Phase 0 complete. Build passing. Clean tree.
**Time**: 4-6 hours
**Agent**: Agent A

### 1.1 — Create Port Registry Source of Truth (Steps 21-30)

- [ ] **Step 21** [W] ~3m: Create `pkg/ports/ports.go` — the canonical Go port constants
  ```go
  // File: pkg/ports/ports.go
  package ports

  // Doom Range: 16666-26666
  // All Unheaded services use high ports to avoid conflicts with dev tools.
  // This file is the SINGLE SOURCE OF TRUTH for port assignments.

  const (
      // Infrastructure Tier (16666-16999)
      DoomBridge             = 16666
      DoomGoInjector         = 16667
      TraceCollectorHTTP     = 16670
      TraceCollectorMetrics  = 16671

      // Control Plane (17000-17999)
      DaemonHTTP             = 17000
      DaemonGRPC             = 17001
      CLIServer              = 17010

      // Wotan — Message Bus (18000-18099)
      WotanHTTP              = 18000
      WotanGRPC              = 18001

      // Core Services (19000-19999)
      Timeguru               = 19000
      Architect              = 19001
      Captain                = 19002
      Micromanager           = 19003
      Monad                  = 19004
      Sophia                 = 19005
      Cuirass                = 19006

      // Applications (20000-20999)
      DashboardBackend       = 20000
      KanbanApp              = 20001
      WikiServer             = 20002

      // Gateway (21000-21443)
      GatewayHTTP            = 21000
      GatewayHTTPS           = 21443

      // Utilities (22000-22099)
      CertGen                = 22000

      // Customer Apps (26000-26666)
      CustomerStart          = 26000
      CustomerEnd            = 26666
  )

  // DefaultAddr returns ":<port>" string for net.Listen.
  func DefaultAddr(port int) string {
      return fmt.Sprintf(":%d", port)
  }

  // DefaultWotanGRPC returns the default Wotan gRPC dial target.
  func DefaultWotanGRPC() string {
      return fmt.Sprintf("localhost:%d", WotanGRPC)
  }

  // DefaultWotanHTTP returns the default Wotan HTTP base URL.
  func DefaultWotanHTTP() string {
      return fmt.Sprintf("http://localhost:%d", WotanHTTP)
  }
  ```
  **NOTE**: Add `import "fmt"` at top. Write the full file with proper package declaration.

- [ ] **Step 22** [B] ~30s: Verify it compiles
  ```bash
  cd ~/tmp/unheaded && go build ./pkg/ports/
  ```

- [ ] **Step 23** [V] ~15s: Must compile clean. If not → fix syntax.

- [ ] **Step 24** [W] ~3m: Create `pkg/ports/ports_test.go` — verify no duplicate ports
  ```go
  package ports

  import (
      "testing"
      "reflect"
  )

  func TestNoDuplicatePorts(t *testing.T) {
      seen := make(map[int]string)
      // Use reflection or manual list to check all port constants
      portMap := map[string]int{
          "DoomBridge":            DoomBridge,
          "DoomGoInjector":        DoomGoInjector,
          "TraceCollectorHTTP":    TraceCollectorHTTP,
          "TraceCollectorMetrics": TraceCollectorMetrics,
          "DaemonHTTP":            DaemonHTTP,
          "DaemonGRPC":            DaemonGRPC,
          "CLIServer":             CLIServer,
          "WotanHTTP":             WotanHTTP,
          "WotanGRPC":             WotanGRPC,
          "Timeguru":              Timeguru,
          "Architect":             Architect,
          "Captain":               Captain,
          "Micromanager":          Micromanager,
          "Monad":                 Monad,
          "Sophia":                Sophia,
          "Cuirass":               Cuirass,
          "DashboardBackend":      DashboardBackend,
          "KanbanApp":             KanbanApp,
          "WikiServer":            WikiServer,
          "GatewayHTTP":           GatewayHTTP,
          "GatewayHTTPS":          GatewayHTTPS,
          "CertGen":               CertGen,
      }
      for name, port := range portMap {
          if existing, ok := seen[port]; ok {
              t.Errorf("DUPLICATE PORT %d: %s and %s", port, existing, name)
          }
          seen[port] = name
      }
  }

  func TestAllPortsInDoomRange(t *testing.T) {
      ports := []int{
          DoomBridge, DoomGoInjector, TraceCollectorHTTP, TraceCollectorMetrics,
          DaemonHTTP, DaemonGRPC, CLIServer,
          WotanHTTP, WotanGRPC,
          Timeguru, Architect, Captain, Micromanager, Monad, Sophia, Cuirass,
          DashboardBackend, KanbanApp, WikiServer,
          GatewayHTTP, GatewayHTTPS,
          CertGen,
      }
      for _, p := range ports {
          if p < 16666 || p > 26666 {
              t.Errorf("Port %d outside Doom Range (16666-26666)", p)
          }
      }
  }
  ```

- [ ] **Step 25** [B] ~30s: Run port tests
  ```bash
  cd ~/tmp/unheaded && go test -v ./pkg/ports/
  ```

- [ ] **Step 26** [V] ~15s: All port tests pass. Zero duplicates. All in Doom Range.

- [ ] **Step 27** [W] ~2m: Create port registry YAML (for container configs and non-Go consumers)
  ```bash
  cat > ~/tmp/unheaded/configs/port-registry.yaml << 'PORTEOF'
  # Unheaded Port Registry — "The Doom Range" (16666-26666)
  # Source of truth: pkg/ports/ports.go
  # This YAML is for container configs and non-Go consumers.

  infrastructure:
    doom-bridge: { port: 16666, proto: websocket }
    doom-go-injector: { port: 16667, proto: tcp }
    trace-collector-http: { port: 16670, proto: http }
    trace-collector-metrics: { port: 16671, proto: http }
  control-plane:
    unheaded-daemon-http: { port: 17000, proto: http }
    unheaded-daemon-grpc: { port: 17001, proto: grpc }
    unheaded-cli: { port: 17010, proto: http }
  wotan:
    wotan-http: { port: 18000, proto: http }
    wotan-grpc: { port: 18001, proto: grpc }
  services:
    timeguru: { port: 19000, proto: http }
    architect: { port: 19001, proto: http }
    captain: { port: 19002, proto: http }
    micromanager: { port: 19003, proto: http }
    monad: { port: 19004, proto: http }
    sophia: { port: 19005, proto: http }
    cuirass: { port: 19006, proto: http }
  applications:
    dashboard-backend: { port: 20000, proto: http }
    kanban-app: { port: 20001, proto: http }
    wiki-server: { port: 20002, proto: http }
  gateway:
    gateway-http: { port: 21000, proto: http }
    gateway-https: { port: 21443, proto: https }
  utilities:
    cert-gen: { port: 22000, proto: http }
  customer:
    reserved: { range: "26000-26666" }
  PORTEOF
  ```
  **NOTE**: Create `configs/` dir first if it doesn't exist: `mkdir -p ~/tmp/unheaded/configs`

- [ ] **Step 28** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add pkg/ports/ configs/port-registry.yaml && git commit -m "feat(ports): add Doom Range port registry — single source of truth for all service ports"
  ```

### 1.2 — Migrate Wotan Ports (Steps 29-38) — THE NERVE CENTER FIRST

Wotan is the message bus. Every other service connects TO Wotan. Migrate it first so downstream services can reference the new address.

- [ ] **Step 29** [R] ~1m: Read current Wotan main.go to understand flag structure
  ```bash
  cat ~/tmp/unheaded/services/wotan/cmd/wotan/main.go
  ```

- [ ] **Step 30** [B] ~3m: Update Wotan default HTTP port 9080→18000 and gRPC port 9090→18001
  Update `services/wotan/cmd/wotan/main.go`:
  - Change `-http-port` default from `9080` to `18000`
  - Change `-grpc-port` default from `9090` to `18001`
  - Add import for `"github.com/unheaded/unheaded/pkg/ports"` and use `ports.WotanHTTP` / `ports.WotanGRPC`

- [ ] **Step 31** [B] ~30s: Build Wotan
  ```bash
  cd ~/tmp/unheaded && go build ./services/wotan/...
  ```

- [ ] **Step 32** [V] ~15s: Wotan builds clean.

- [ ] **Step 33** [B] ~1m: Run Wotan tests
  ```bash
  cd ~/tmp/unheaded && go test -race ./services/wotan/... 2>&1 | tail -20
  ```

- [ ] **Step 34** [V] ~15s: Wotan tests pass.
  - If fail → Step 35 [D]
  - If pass → Step 36

- [ ] **Step 35** [D] ~3m: Fix test expectations that reference old ports (9080/9090). Grep:
  ```bash
  grep -rn "9080\|9090" ~/tmp/unheaded/services/wotan/ --include="*.go"
  ```
  Update test expectations to match new ports.

- [ ] **Step 36** [B] ~1m: Update wotan-ctl default address (old: localhost:5555)
  ```bash
  grep -n "5555\|9080\|9090" ~/tmp/unheaded/cmd/wotan-ctl/*.go
  ```
  Update all references to use `ports.WotanHTTP` / `ports.WotanGRPC`.

- [ ] **Step 37** [B] ~30s: Build wotan-ctl
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/wotan-ctl/...
  ```

- [ ] **Step 38** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(ports): migrate Wotan to Doom Range — HTTP 18000, gRPC 18001"
  ```

### 1.3 — Migrate Core Services (Steps 39-62)

Each service follows the pattern: read → update port → update Wotan addr → build → test → next.

**Timeguru** (8000 → 19000)

- [ ] **Step 39** [R] ~30s: Read timeguru main.go
  ```bash
  cat ~/tmp/unheaded/services/timeguru/cmd/timeguru/main.go | head -60
  ```

- [ ] **Step 40** [B] ~2m: Update timeguru:
  - Default port `8000` → `19000` (use `ports.Timeguru`)
  - `defaultWotanAddr` `localhost:9080` → `localhost:18000` (use `ports.DefaultWotanHTTP()`)
  - `WOTAN_GRPC_ADDR` default `localhost:9090` → `localhost:18001` (use `ports.DefaultWotanGRPC()`)

- [ ] **Step 41** [B] ~1m: Build + test timeguru
  ```bash
  cd ~/tmp/unheaded && go build ./services/timeguru/... && go test -race ./services/timeguru/... 2>&1 | tail -10
  ```

- [ ] **Step 42** [V] ~15s: Build + tests pass. If fail → fix test port expectations.

**Captain** (8002 → 19002)

- [ ] **Step 43** [R] ~30s: Read captain main.go
  ```bash
  cat ~/tmp/unheaded/services/captain/cmd/captain/main.go | head -60
  ```

- [ ] **Step 44** [B] ~2m: Update captain:
  - `HTTP_ADDR` default `0.0.0.0:8002` → `0.0.0.0:19002`
  - `WOTAN_ADDR` default `localhost:9090` → `localhost:18001`

- [ ] **Step 45** [B] ~1m: Build + test captain
  ```bash
  cd ~/tmp/unheaded && go build ./services/captain/... && go test -race ./services/captain/... 2>&1 | tail -10
  ```

- [ ] **Step 46** [V] ~15s: Pass. If fail → fix.

- [ ] **Step 47** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(ports): migrate timeguru (19000) and captain (19002) to Doom Range"
  ```

**Architect** (8001 → 19001)

- [ ] **Step 48** [R] ~30s: Read architect main.go
  ```bash
  cat ~/tmp/unheaded/services/architect/cmd/architect/main.go | head -60
  ```

- [ ] **Step 49** [B] ~2m: Update architect:
  - `-addr` default `:8001` → `:19001`
  - `-wotan` default `localhost:8081` → `localhost:18000`

- [ ] **Step 50** [B] ~1m: Build + test architect
  ```bash
  cd ~/tmp/unheaded && go build ./services/architect/... && go test -race ./services/architect/... 2>&1 | tail -10
  ```

- [ ] **Step 51** [V] ~15s: Pass.

**Micromanager** (8003 → 19003)

- [ ] **Step 52** [R] ~30s: Read micromanager main.go
  ```bash
  cat ~/tmp/unheaded/services/micromanager/cmd/micromanager/main.go | head -60
  ```

- [ ] **Step 53** [B] ~2m: Update micromanager:
  - `-port` default `8003` → `19003`
  - `WOTAN_ADDR` → `localhost:18001`

- [ ] **Step 54** [B] ~1m: Build + test micromanager
  ```bash
  cd ~/tmp/unheaded && go build ./services/micromanager/... && go test -race ./services/micromanager/... 2>&1 | tail -10
  ```

- [ ] **Step 55** [V] ~15s: Pass.

- [ ] **Step 56** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(ports): migrate architect (19001) and micromanager (19003) to Doom Range"
  ```

**Monad** (8004 → 19004)

- [ ] **Step 57** [B] ~2m: Update cmd/monad/main.go:
  - `-listen` default `:8004` → `:19004`
  - `-wotan` default `localhost:9090` → `localhost:18001`
  - `MONAD_PORT` / `MONAD_LISTEN_ADDR` defaults updated

- [ ] **Step 58** [B] ~1m: Build + test monad
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/monad/... && go test -race ./cmd/monad/... 2>&1 | tail -10
  ```

- [ ] **Step 59** [V] ~15s: Pass.

**Sophia** (8005 → 19005)

- [ ] **Step 60** [B] ~2m: Update cmd/sophia/main.go:
  - `-listen` default `:8005` → `:19005`
  - `-wotan` default `localhost:9090` → `localhost:18001`
  - `SOPHIA_LISTEN_ADDR` default updated

- [ ] **Step 61** [B] ~1m: Build + test sophia
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/sophia/... && go test -race ./cmd/sophia/... 2>&1 | tail -10
  ```

- [ ] **Step 62** [V] ~15s: Pass.

- [ ] **Step 63** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(ports): migrate monad (19004) and sophia (19005) to Doom Range"
  ```

### 1.4 — Migrate Application Tier (Steps 64-80)

**Dashboard Backend** (8080 → 20000)

- [ ] **Step 64** [R] ~30s: Read dashboard-backend main.go
  ```bash
  cat ~/tmp/unheaded/cmd/dashboard-backend/main.go | head -80
  ```

- [ ] **Step 65** [B] ~3m: Update cmd/dashboard-backend/main.go:
  - `-listen` default `:8080` → `:20000`
  - `-wotan` default `localhost:9090` → `localhost:18001`
  - Update `internal/events/events.go` default (9090 → 18001)
  - Update `internal/server/server.go` default (9090 → 18001)

- [ ] **Step 66** [B] ~2m: Update dashboard-backend internal health check endpoints
  ```bash
  grep -rn "8080\|8000\|8001\|8002\|8003\|8004\|8005\|9090\|9080" ~/tmp/unheaded/cmd/dashboard-backend/internal/ --include="*.go" | grep -v _test.go
  ```
  Update ALL hardcoded ports in health.go and scraper.go to use new Doom Range ports.

- [ ] **Step 67** [B] ~1m: Build + test dashboard-backend
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/dashboard-backend/... && go test -race ./cmd/dashboard-backend/... 2>&1 | tail -20
  ```

- [ ] **Step 68** [V] ~15s: Pass. Dashboard tests are extensive — expect some test updates needed.
  - If fail → grep test files for old ports and update expectations

**Kanban App** (8081 → 20001)

- [ ] **Step 69** [R] ~30s: Read kanban-app main.go
  ```bash
  grep -n "PORT\|WOTAN\|TIMEGURU\|8081\|9090\|9080\|8000" ~/tmp/unheaded/cmd/kanban-app/main.go
  ```

- [ ] **Step 70** [B] ~3m: Update cmd/kanban-app/main.go:
  - `PORT` default `8081` → `20001`
  - `WOTAN_ADDR` default `localhost:9080` → `localhost:18000`
  - `WOTAN_GRPC_ADDR` default `localhost:9090` → `localhost:18001`
  - `TIMEGURU_ADDR` default `localhost:8000` → `localhost:19000`

- [ ] **Step 71** [B] ~2m: Update kanban-app CORS middleware
  ```bash
  grep -n "8080\|8081\|localhost" ~/tmp/unheaded/cmd/kanban-app/middleware.go
  ```
  Update CORS allowed origins to reference new ports (20000, 20001, 21000, 21443).

- [ ] **Step 72** [B] ~1m: Build + test kanban-app
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/kanban-app/... && go test -race ./cmd/kanban-app/... 2>&1 | tail -10
  ```

- [ ] **Step 73** [V] ~15s: Pass.

- [ ] **Step 74** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(ports): migrate dashboard (20000) and kanban (20001) to Doom Range"
  ```

### 1.5 — Migrate Infrastructure Tier (Steps 75-88)

**Doom Bridge** (6660 → 16666)

- [ ] **Step 75** [B] ~2m: Update cmd/doom-bridge/main.go:
  - Default port `6660` → `16666`

- [ ] **Step 76** [B] ~30s: Build doom-bridge
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/doom-bridge/...
  ```

**Trace Collector** (9092/9100 → 16670/16671)

- [ ] **Step 77** [B] ~2m: Update cmd/trace-collector-go/main.go:
  - HTTP port `9092` → `16670`
  - Metrics port `9100` → `16671`
  - Wotan gRPC addr `localhost:9090` → `localhost:18001`

- [ ] **Step 78** [B] ~1m: Build + test trace-collector
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/trace-collector-go/... && go test -race ./cmd/trace-collector-go/... 2>&1 | tail -10
  ```

- [ ] **Step 79** [V] ~15s: Pass.

**Unheaded Daemon** (8080/9090 → 17000/17001)

- [ ] **Step 80** [R] ~30s: Read daemon config
  ```bash
  grep -n "8080\|9090\|5555\|port" ~/tmp/unheaded/cmd/unheaded-daemon/internal/config/config.go
  ```

- [ ] **Step 81** [B] ~3m: Update unheaded-daemon:
  - HTTP default `8080` → `17000`
  - gRPC default `9090` → `17001`
  - Wotan addr `5555` → `18001`
  - Update cmd/unheaded-daemon/main.go with same changes

- [ ] **Step 82** [B] ~1m: Build + test daemon
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/unheaded-daemon/... && go test -race ./cmd/unheaded-daemon/... 2>&1 | tail -10
  ```

- [ ] **Step 83** [V] ~15s: Pass.

- [ ] **Step 84** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(ports): migrate doom-bridge (16666), trace-collector (16670), daemon (17000) to Doom Range"
  ```

### 1.6 — Migrate CLI, Gateway, and Remaining Services (Steps 85-92)

- [ ] **Step 85** [B] ~3m: Update unheaded-cli — ALL hardcoded endpoints
  ```bash
  grep -rn "8080\|9090\|9080\|8000\|8001\|8002\|8003\|8004\|8005\|8006\|10\.10\.10\." ~/tmp/unheaded/cmd/unheaded-cli/ --include="*.go" | grep -v _test.go
  ```
  Update every reference to use new Doom Range ports. Service endpoints should use `ports.*` constants.

- [ ] **Step 86** [B] ~3m: Update gateway config defaults
  ```bash
  grep -rn "8080\|8000\|8001\|8002\|8003\|8004\|8005\|8081" ~/tmp/unheaded/services/gateway/ --include="*.go" | grep -v _test.go
  ```
  Update `services/gateway/config/config.go` — all service endpoint defaults to new ports.

- [ ] **Step 87** [B] ~2m: Update cert-gen service IP/port map
  ```bash
  grep -rn "10\.10\.10\.\|8000\|8001\|8002\|8003\|8004\|8005\|6660" ~/tmp/unheaded/cmd/cert-gen/ --include="*.go"
  ```
  Update all service definitions with new ports.

- [ ] **Step 88** [B] ~2m: Build everything
  ```bash
  cd ~/tmp/unheaded && go build ./...
  ```

- [ ] **Step 89** [V] ~15s: Full build passes.

- [ ] **Step 90** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(ports): migrate CLI, gateway, and cert-gen to Doom Range"
  ```

### 1.7 — Migrate Container and Config Files (Steps 91-100)

- [ ] **Step 91** [B] ~5m: Update docker-compose.yml — ALL port mappings and environment variables
  ```bash
  grep -n "8080\|8081\|8000\|8001\|8002\|8003\|8004\|8005\|8006\|9090\|9080\|6660" ~/tmp/unheaded/docker-compose.yml
  ```
  Update every port mapping and env var to Doom Range equivalents.

- [ ] **Step 92** [B] ~3m: Update NixOS container configs
  ```bash
  grep -rn "8080\|8081\|8000\|8001\|8002\|8003\|8004\|8005\|8006\|9090\|9080\|6660\|5555" ~/tmp/unheaded/nix/containers/ --include="*.nix"
  ```
  Update all .nix files with new ports.

- [ ] **Step 93** [B] ~2m: Update NixOS modules
  ```bash
  grep -rn "8080\|9090\|10\.10\.10\." ~/tmp/unheaded/nix/modules/ --include="*.nix"
  ```
  Update networking.nix (Wotan address) and health-monitor.nix (health URLs).

- [ ] **Step 94** [B] ~2m: Update LXD container configs
  ```bash
  grep -rn "6660\|8080\|9090" ~/tmp/unheaded/containers/lxd/ --include="*.yaml" --include="*.yml"
  ```

- [ ] **Step 95** [B] ~2m: Update containerd service configs
  ```bash
  grep -rn "6660\|8080\|9090" ~/tmp/unheaded/containers/containerd/ --include="*.json"
  ```

- [ ] **Step 96** [B] ~1m: Update Makefile port references
  ```bash
  grep -n "8080\|9090\|8000\|8001\|8002\|8003\|8004\|8005\|6660" ~/tmp/unheaded/Makefile
  ```

- [ ] **Step 97** [B] ~1m: Update nix/Makefile health check ports
  ```bash
  grep -n "8080\|8000\|8001\|8002\|8003\|8004\|8005\|8006\|9090" ~/tmp/unheaded/nix/Makefile
  ```

- [ ] **Step 98** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(ports): migrate docker-compose, NixOS, LXD, containerd configs to Doom Range"
  ```

### 1.8 — Update Test Files (Steps 99-103)

- [ ] **Step 99** [B] ~5m: Update test port expectations — gateway config tests
  ```bash
  grep -rn "8080\|8081\|8000\|8001\|8002\|8003\|8004\|8005\|9090\|9080" ~/tmp/unheaded/services/gateway/config/config_test.go
  ```
  Update ALL test expectations to new Doom Range ports.

- [ ] **Step 100** [B] ~5m: Update dashboard backend test expectations
  ```bash
  grep -rn "8080\|9090\|8000\|8001\|8002\|8003\|8004\|8005" ~/tmp/unheaded/cmd/dashboard-backend/ --include="*_test.go"
  ```
  Update ALL test files.

- [ ] **Step 101** [B] ~5m: Full test suite
  ```bash
  cd ~/tmp/unheaded && go test -race -count=1 ./... 2>&1 | tail -40
  ```

- [ ] **Step 102** [V] ~15s: ALL tests pass.
  - If fail → grep failing test for old port, fix, rerun
  - If >5 failures → systematic grep: `grep -rn "\":8080\"\|\":9090\"" --include="*_test.go" ~/tmp/unheaded/`

- [ ] **Step 103** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "test(ports): update all test expectations for Doom Range ports"
  ```

### 1.9 — Port Migration Verification (Steps 104-105)

- [ ] **Step 104** [B] ~2m: **ZERO OLD PORTS REMAINING** — Verify no production code references old ports
  ```bash
  echo "=== Old ports remaining in production .go files ==="
  grep -rn "\":8080\"\|\":9090\"\|\":9080\"\|\":8000\"\|\":8001\"\|\":8002\"\|\":8003\"\|\":8004\"\|\":8005\"\|\":8006\"\|\":8081\"\|\":6660\"\|\":5555\"" --include="*.go" ~/tmp/unheaded/ | grep -v _test.go | grep -v vendor | grep -v docs/ || echo "CLEAN — zero old port references!"
  echo ""
  echo "=== Full build ==="
  cd ~/tmp/unheaded && go build ./...
  echo ""
  echo "=== Full test ==="
  cd ~/tmp/unheaded && go test -race -count=1 ./... 2>&1 | grep -E "^(ok|FAIL|---)" | tail -40
  ```

- [ ] **Step 105** [V] ~15s: **PHASE 1 EXIT GATE** — Port Authority Complete
  - ✅ Zero old port references in production Go code
  - ✅ `go build ./...` passes
  - ✅ `go test -race ./...` passes
  - ✅ pkg/ports/ exists with all constants and passes its own tests
  - ✅ docker-compose.yml updated
  - ✅ NixOS configs updated
  - ✅ Container configs updated
  - If ALL pass → Signal Agents B, D, and E to begin their phases
  - If fail → DO NOT PROCEED. Fix within this phase.

---

## PHASE 2: gRPC-FIRST TRANSPORT (Steps 106-175) — Agent B

**Goal**: Create transport abstraction package. Flip every service to default to Wotan gRPC (18001). HTTP becomes fallback. Every service gets dual health checks (gRPC + HTTP). Transport cascade: gRPC → HTTP/3 → HTTP/2 → HTTP/1.1.
**Prerequisite**: Phase 1 complete. All services on Doom Range. Build passing.
**Time**: 4-6 hours
**Agent**: Agent B

### 2.1 — Create Transport Package (Steps 106-130)

- [ ] **Step 106** [W] ~5m: Create `pkg/transport/transport.go` — transport interface and types
  ```go
  // File: pkg/transport/transport.go
  package transport

  import (
      "context"
      "time"
  )

  // Type represents a transport protocol.
  type Type string

  const (
      GRPC  Type = "grpc"
      HTTP3 Type = "http3"
      HTTP2 Type = "http2"
      HTTP1 Type = "http1"
      Auto  Type = "auto" // Try gRPC first, cascade to HTTP
  )

  // Config holds transport configuration for a service.
  type Config struct {
      // Preferred transport type (default: Auto)
      Type Type
      // WotanGRPCAddr is the Wotan gRPC endpoint (default: localhost:18001)
      WotanGRPCAddr string
      // WotanHTTPAddr is the Wotan HTTP endpoint (default: http://localhost:18000)
      WotanHTTPAddr string
      // ConnectTimeout for initial connection (default: 5s)
      ConnectTimeout time.Duration
      // FallbackEnabled allows falling back from gRPC to HTTP (default: true)
      FallbackEnabled bool
  }

  // DefaultConfig returns sensible defaults.
  func DefaultConfig() Config {
      return Config{
          Type:            Auto,
          WotanGRPCAddr:   "localhost:18001",
          WotanHTTPAddr:   "http://localhost:18000",
          ConnectTimeout:  5 * time.Second,
          FallbackEnabled: true,
      }
  }

  // Connection represents an active transport connection.
  type Connection interface {
      // Type returns the active transport type.
      Type() Type
      // Publish sends a message to a Wotan topic.
      Publish(ctx context.Context, topic string, data []byte) error
      // Subscribe listens to a Wotan topic.
      Subscribe(ctx context.Context, topic string) (<-chan []byte, error)
      // Close tears down the connection.
      Close() error
      // Healthy returns true if the transport is operational.
      Healthy() bool
  }
  ```

- [ ] **Step 107** [B] ~30s: Verify compiles
  ```bash
  cd ~/tmp/unheaded && go build ./pkg/transport/
  ```

- [ ] **Step 108** [W] ~5m: Create `pkg/transport/grpc.go` — gRPC transport implementation
  Implement a `grpcConnection` struct that satisfies the `Connection` interface.
  - Dial `WotanGRPCAddr` with `grpc.DialContext` and timeout
  - Use existing Wotan gRPC client from `pkg/wotan/` if available
  - Publish via Wotan's gRPC PublishMessage RPC
  - Subscribe via Wotan's gRPC SubscribeTopic RPC
  - Health check via `grpc.health.v1.Health/Check`

- [ ] **Step 109** [W] ~5m: Create `pkg/transport/http.go` — HTTP fallback implementation
  Implement an `httpConnection` struct that satisfies the `Connection` interface.
  - POST to `WotanHTTPAddr/api/v1/publish` for Publish
  - GET/WebSocket to `WotanHTTPAddr/api/v1/subscribe` for Subscribe
  - GET `WotanHTTPAddr/health` for health check
  - Support HTTP/3, HTTP/2, HTTP/1.1 (use `net/http` with ALPN)

- [ ] **Step 110** [W] ~5m: Create `pkg/transport/cascade.go` — auto-cascade logic
  ```go
  // cascade.go implements the transport cascade:
  // 1. Try gRPC connection to WotanGRPC
  // 2. If gRPC fails and FallbackEnabled, try HTTP
  // 3. Return the first successful connection
  // 4. If all fail, return error with all attempt details

  func Connect(ctx context.Context, cfg Config) (Connection, error) {
      // ...cascade logic...
  }
  ```

- [ ] **Step 111** [B] ~30s: Verify all transport files compile
  ```bash
  cd ~/tmp/unheaded && go build ./pkg/transport/
  ```

- [ ] **Step 112** [V] ~15s: Transport package compiles.
  - If fail → fix imports/interfaces

- [ ] **Step 113** [W] ~5m: Create `pkg/transport/health.go` — dual health check server
  ```go
  // health.go provides dual-protocol health check registration.
  // Every Unheaded service registers BOTH:
  //   1. gRPC health service (grpc.health.v1)
  //   2. HTTP /health endpoint
  // The HealthServer wraps both and reports service status.

  type HealthServer struct {
      ServiceName string
      // ...
  }

  func NewHealthServer(serviceName string) *HealthServer { ... }
  func (h *HealthServer) RegisterGRPC(s *grpc.Server) { ... }
  func (h *HealthServer) RegisterHTTP(mux *http.ServeMux) { ... }
  func (h *HealthServer) SetStatus(status HealthStatus) { ... }
  ```

- [ ] **Step 114** [W] ~3m: Create `pkg/transport/flags.go` — CLI flag helpers
  ```go
  // flags.go provides standard CLI flag registration for transport config.
  // Every service calls RegisterFlags() to get consistent --transport,
  // --wotan-grpc-addr, --wotan-http-addr flags.

  func RegisterFlags(fs *flag.FlagSet, cfg *Config) {
      fs.StringVar((*string)(&cfg.Type), "transport", string(Auto), "Transport type: grpc, http, auto (default: auto)")
      fs.StringVar(&cfg.WotanGRPCAddr, "wotan-grpc-addr", cfg.WotanGRPCAddr, "Wotan gRPC address")
      fs.StringVar(&cfg.WotanHTTPAddr, "wotan-http-addr", cfg.WotanHTTPAddr, "Wotan HTTP address")
      fs.DurationVar(&cfg.ConnectTimeout, "connect-timeout", cfg.ConnectTimeout, "Transport connect timeout")
      fs.BoolVar(&cfg.FallbackEnabled, "fallback", cfg.FallbackEnabled, "Enable transport fallback")
  }
  ```

- [ ] **Step 115** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add pkg/transport/ && git commit -m "feat(transport): create transport abstraction package — gRPC-first with HTTP cascade fallback"
  ```

### 2.2 — Transport Package Tests (Steps 116-125)

- [ ] **Step 116** [W] ~5m: Create `pkg/transport/transport_test.go` — unit tests
  ```go
  // Test DefaultConfig returns sane defaults
  // Test Config validation
  // Test Type constants
  ```

- [ ] **Step 117** [W] ~5m: Create `pkg/transport/cascade_test.go` — cascade logic tests
  ```go
  // Test: gRPC available → uses gRPC
  // Test: gRPC down + HTTP up → falls back to HTTP
  // Test: both down → returns error with both failure details
  // Test: fallback disabled + gRPC down → returns error (no fallback)
  // Test: timeout respected
  ```

- [ ] **Step 118** [W] ~5m: Create `pkg/transport/health_test.go` — dual health check tests
  ```go
  // Test: gRPC health check responds SERVING when healthy
  // Test: gRPC health check responds NOT_SERVING when unhealthy
  // Test: HTTP /health returns 200 when healthy
  // Test: HTTP /health returns 503 when unhealthy
  // Test: SetStatus propagates to both protocols
  ```

- [ ] **Step 119** [B] ~1m: Run transport tests
  ```bash
  cd ~/tmp/unheaded && go test -v -race ./pkg/transport/... 2>&1 | tail -30
  ```

- [ ] **Step 120** [V] ~15s: All transport tests pass.
  - If fail → fix implementations to match interface contracts

- [ ] **Step 121** [W] ~3m: Create `pkg/transport/flags_test.go`
  ```go
  // Test: RegisterFlags adds all expected flags
  // Test: Flag parsing populates Config correctly
  // Test: Environment variable override works (TRANSPORT_TYPE, WOTAN_GRPC_ADDR, etc.)
  ```

- [ ] **Step 122** [B] ~30s: Run all transport tests
  ```bash
  cd ~/tmp/unheaded && go test -v -race ./pkg/transport/...
  ```

- [ ] **Step 123** [V] ~15s: All pass.

- [ ] **Step 124** [B] ~30s: Check coverage
  ```bash
  cd ~/tmp/unheaded && go test -cover ./pkg/transport/...
  ```

- [ ] **Step 125** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "test(transport): comprehensive unit tests for transport package — cascade, health, flags"
  ```

### 2.3 — Register gRPC Health on Wotan (Steps 126-130)

- [ ] **Step 126** [R] ~1m: Read Wotan's gRPC server setup
  ```bash
  grep -rn "grpc.NewServer\|RegisterService\|health" ~/tmp/unheaded/services/wotan/ --include="*.go" | head -20
  ```

- [ ] **Step 127** [B] ~3m: Add `grpc.health.v1` service to Wotan's gRPC server
  - Import `google.golang.org/grpc/health` and `google.golang.org/grpc/health/grpc_health_v1`
  - Register health server on Wotan's gRPC server
  - Set initial status to SERVING

- [ ] **Step 128** [B] ~30s: Build Wotan with health service
  ```bash
  cd ~/tmp/unheaded && go build ./services/wotan/...
  ```

- [ ] **Step 129** [B] ~1m: Test Wotan with health
  ```bash
  cd ~/tmp/unheaded && go test -race ./services/wotan/... 2>&1 | tail -10
  ```

- [ ] **Step 130** [V] ~15s: Wotan builds and tests pass with gRPC health.

### 2.4 — Flip Services to gRPC-First (Steps 131-158)

Each service gets the same treatment:
1. Import `pkg/transport`
2. Add transport flags via `transport.RegisterFlags()`
3. Replace direct HTTP Wotan connection with `transport.Connect()`
4. Add dual health check via `transport.NewHealthServer()`
5. Build + test

**Timeguru** (already has some dual transport)

- [ ] **Step 131** [R] ~30s: Check timeguru's existing transport setup
  ```bash
  grep -n "wotan\|grpc\|transport\|health" ~/tmp/unheaded/services/timeguru/cmd/timeguru/main.go | head -20
  ```

- [ ] **Step 132** [B] ~5m: Wire timeguru to use `pkg/transport`:
  - Add `transport.RegisterFlags()` to flag setup
  - Replace manual Wotan connection with `transport.Connect()`
  - Add `transport.NewHealthServer("timeguru")` with dual registration
  - Keep existing HTTP endpoints alongside new gRPC default

- [ ] **Step 133** [B] ~1m: Build + test timeguru
  ```bash
  cd ~/tmp/unheaded && go build ./services/timeguru/... && go test -race ./services/timeguru/... 2>&1 | tail -10
  ```

- [ ] **Step 134** [V] ~15s: Pass.

- [ ] **Step 135** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(transport): wire timeguru to gRPC-first transport with dual health check"
  ```

**Captain, Architect, Micromanager** (batch — similar patterns)

- [ ] **Step 136** [B] ~5m: Wire captain to `pkg/transport`
- [ ] **Step 137** [B] ~1m: Build + test captain
  ```bash
  cd ~/tmp/unheaded && go build ./services/captain/... && go test -race ./services/captain/... 2>&1 | tail -10
  ```
- [ ] **Step 138** [V] ~15s: Pass.

- [ ] **Step 139** [B] ~5m: Wire architect to `pkg/transport`
- [ ] **Step 140** [B] ~1m: Build + test architect
  ```bash
  cd ~/tmp/unheaded && go build ./services/architect/... && go test -race ./services/architect/... 2>&1 | tail -10
  ```
- [ ] **Step 141** [V] ~15s: Pass.

- [ ] **Step 142** [B] ~5m: Wire micromanager to `pkg/transport`
- [ ] **Step 143** [B] ~1m: Build + test micromanager
  ```bash
  cd ~/tmp/unheaded && go build ./services/micromanager/... && go test -race ./services/micromanager/... 2>&1 | tail -10
  ```
- [ ] **Step 144** [V] ~15s: Pass.

- [ ] **Step 145** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(transport): wire captain, architect, micromanager to gRPC-first transport"
  ```

**Monad, Sophia** (cmd/ directory services)

- [ ] **Step 146** [B] ~5m: Wire monad to `pkg/transport`
- [ ] **Step 147** [B] ~1m: Build + test
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/monad/... && go test -race ./cmd/monad/... 2>&1 | tail -10
  ```
- [ ] **Step 148** [V] ~15s: Pass.

- [ ] **Step 149** [B] ~5m: Wire sophia to `pkg/transport`
- [ ] **Step 150** [B] ~1m: Build + test
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/sophia/... && go test -race ./cmd/sophia/... 2>&1 | tail -10
  ```
- [ ] **Step 151** [V] ~15s: Pass.

- [ ] **Step 152** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(transport): wire monad and sophia to gRPC-first transport"
  ```

**Dashboard, Kanban, Trace Collector, Daemon**

- [ ] **Step 153** [B] ~5m: Wire dashboard-backend to `pkg/transport`
- [ ] **Step 154** [B] ~5m: Wire kanban-app to `pkg/transport`
- [ ] **Step 155** [B] ~5m: Wire trace-collector-go to `pkg/transport`
- [ ] **Step 156** [B] ~5m: Wire unheaded-daemon to `pkg/transport`

- [ ] **Step 157** [B] ~2m: Full build + test
  ```bash
  cd ~/tmp/unheaded && go build ./... && go test -race ./... 2>&1 | grep -E "^(ok|FAIL)" | tail -30
  ```

- [ ] **Step 158** [V] ~15s: All pass.

- [ ] **Step 159** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(transport): wire dashboard, kanban, trace-collector, daemon to gRPC-first transport"
  ```

### 2.5 — DEGRADED State Handling (Steps 160-168)

- [ ] **Step 160** [W] ~3m: Add DEGRADED status to `pkg/transport/health.go`
  When gRPC is down but HTTP is up → service status = DEGRADED (not DOWN).
  Dashboard health panel should show yellow for DEGRADED, green for HEALTHY, red for DOWN.

- [ ] **Step 161** [B] ~3m: Update dashboard-backend health aggregation
  ```bash
  grep -n "health\|status\|DEGRADED\|DOWN\|HEALTHY" ~/tmp/unheaded/cmd/dashboard-backend/internal/health/health.go | head -20
  ```
  Add DEGRADED state to health consensus logic. Update cross-service health to check gRPC AND HTTP.

- [ ] **Step 162** [W] ~3m: Write tests for DEGRADED state
  ```go
  // Test: gRPC up + HTTP up → HEALTHY
  // Test: gRPC down + HTTP up → DEGRADED
  // Test: gRPC down + HTTP down → DOWN
  // Test: gRPC up + HTTP down → DEGRADED (unusual but possible)
  ```

- [ ] **Step 163** [B] ~1m: Run health tests
  ```bash
  cd ~/tmp/unheaded && go test -race ./pkg/transport/... ./cmd/dashboard-backend/internal/health/... 2>&1 | tail -15
  ```

- [ ] **Step 164** [V] ~15s: Pass.

- [ ] **Step 165** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(transport): add DEGRADED state — gRPC down + HTTP up = yellow not red"
  ```

### 2.6 — Environment Variable Override (Steps 166-172)

- [ ] **Step 166** [B] ~3m: Ensure every service reads these env vars (in addition to flags):
  - `TRANSPORT_TYPE` → overrides `--transport`
  - `WOTAN_GRPC_ADDR` → overrides `--wotan-grpc-addr`
  - `WOTAN_HTTP_ADDR` → overrides `--wotan-http-addr`
  - `CONNECT_TIMEOUT` → overrides `--connect-timeout`
  Add env var reading to `pkg/transport/flags.go` with `os.Getenv()` fallbacks.

- [ ] **Step 167** [W] ~2m: Test env var overrides
  ```go
  // Test: TRANSPORT_TYPE=grpc overrides default auto
  // Test: WOTAN_GRPC_ADDR overrides default
  // Test: flag takes precedence over env var
  ```

- [ ] **Step 168** [B] ~1m: Run tests
  ```bash
  cd ~/tmp/unheaded && go test -race ./pkg/transport/... 2>&1 | tail -10
  ```

- [ ] **Step 169** [V] ~15s: Pass.

### 2.7 — Phase 2 Verification (Steps 170-175)

- [ ] **Step 170** [B] ~2m: Full build
  ```bash
  cd ~/tmp/unheaded && go build ./...
  ```

- [ ] **Step 171** [B] ~5m: Full test suite
  ```bash
  cd ~/tmp/unheaded && go test -race -count=1 ./... 2>&1 | grep -E "^(ok|FAIL)" | tail -40
  ```

- [ ] **Step 172** [V] ~15s: All pass.

- [ ] **Step 173** [B] ~1m: Verify every service has dual health check
  ```bash
  grep -rn "NewHealthServer\|RegisterGRPC\|RegisterHTTP\|grpc_health" --include="*.go" ~/tmp/unheaded/cmd/ ~/tmp/unheaded/services/ | grep -v _test.go | grep -v vendor
  ```
  Every service in cmd/ and services/ should have at least one match.

- [ ] **Step 174** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(transport): complete gRPC-first transport migration — all services, dual health, DEGRADED state"
  ```

- [ ] **Step 175** [V] ~15s: **PHASE 2 EXIT GATE** — gRPC-First Transport Complete
  - ✅ `pkg/transport/` exists with transport.go, grpc.go, http.go, cascade.go, health.go, flags.go
  - ✅ All transport tests pass with -race
  - ✅ Every service uses `transport.Connect()` for Wotan communication
  - ✅ Every service has dual health checks (gRPC + HTTP)
  - ✅ DEGRADED state implemented and tested
  - ✅ Env var + flag + config file override all work
  - ✅ `go build ./...` passes
  - ✅ `go test -race ./...` passes
  - If ALL pass → Signal Agent C (Phase 3) to begin
  - If fail → DO NOT PROCEED.

---

## PHASE 3: LOG AGGREGATION (Steps 176-230) — Agent C

**Goal**: All services publish structured logs to Wotan. Dashboard has log viewer with search, filter, and live tail. Ring buffer retains 10K lines per service.
**Prerequisite**: Phase 2 complete. gRPC transport operational.
**Time**: 4-6 hours
**Agent**: Agent C

### 3.1 — Create Log Aggregation Package (Steps 176-195)

- [ ] **Step 176** [W] ~5m: Create `pkg/logagg/publisher.go` — zerolog hook that forwards to Wotan
  ```go
  // publisher.go implements a zerolog.Hook that publishes log entries
  // to Wotan topic: logs.<service>.<level>
  //
  // Usage:
  //   hook := logagg.NewPublisher("timeguru", transport)
  //   logger := zerolog.New(os.Stdout).Hook(hook)
  //
  // Each log entry is published as JSON to Wotan topic:
  //   logs.timeguru.info
  //   logs.timeguru.error
  //   etc.

  type Publisher struct {
      serviceName string
      transport   transport.Connection
      enabled     bool
  }

  func NewPublisher(serviceName string, conn transport.Connection) *Publisher { ... }
  func (p *Publisher) Run(e *zerolog.Event, level zerolog.Level, msg string) { ... }
  ```

- [ ] **Step 177** [W] ~5m: Create `pkg/logagg/ringbuffer.go` — in-memory log retention
  ```go
  // ringbuffer.go provides a thread-safe ring buffer for log entries.
  // Retains the last N entries per service (default: 10,000).
  // Supports query by: service, level, time range, text search.

  type RingBuffer struct {
      mu       sync.RWMutex
      entries  []LogEntry
      capacity int
      head     int
      count    int
  }

  type LogEntry struct {
      Timestamp time.Time
      Service   string
      Level     string
      Message   string
      Fields    map[string]interface{}
  }

  func NewRingBuffer(capacity int) *RingBuffer { ... }
  func (rb *RingBuffer) Push(entry LogEntry) { ... }
  func (rb *RingBuffer) Query(q LogQuery) []LogEntry { ... }
  ```

- [ ] **Step 178** [W] ~3m: Create `pkg/logagg/query.go` — log query types
  ```go
  type LogQuery struct {
      Service   string     // filter by service name (empty = all)
      Level     string     // filter by level (empty = all)
      Search    string     // full-text search in message + fields
      From      time.Time  // start time (zero = no lower bound)
      To        time.Time  // end time (zero = no upper bound)
      Limit     int        // max results (0 = default 100)
      Offset    int        // pagination offset
  }
  ```

- [ ] **Step 179** [W] ~3m: Create `pkg/logagg/subscriber.go` — Wotan log topic subscriber
  ```go
  // subscriber.go listens to logs.* Wotan topics and feeds entries
  // into the ring buffer. Used by dashboard-backend.

  type Subscriber struct {
      transport  transport.Connection
      buffer     *RingBuffer
      topics     []string // e.g., ["logs.>"] for wildcard
  }

  func NewSubscriber(conn transport.Connection, buf *RingBuffer) *Subscriber { ... }
  func (s *Subscriber) Start(ctx context.Context) error { ... }
  func (s *Subscriber) Stop() { ... }
  ```

- [ ] **Step 180** [B] ~30s: Verify logagg package compiles
  ```bash
  cd ~/tmp/unheaded && go build ./pkg/logagg/
  ```

- [ ] **Step 181** [V] ~15s: Compiles clean.

- [ ] **Step 182** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add pkg/logagg/ && git commit -m "feat(logagg): create log aggregation package — publisher, ring buffer, query, subscriber"
  ```

### 3.2 — Log Aggregation Tests (Steps 183-192)

- [ ] **Step 183** [W] ~5m: Create `pkg/logagg/publisher_test.go`
  ```go
  // Test: publisher sends to correct topic (logs.<service>.<level>)
  // Test: publisher handles nil transport gracefully (disabled mode)
  // Test: publisher doesn't block on slow transport
  // Test: publisher formats JSON correctly
  ```

- [ ] **Step 184** [W] ~5m: Create `pkg/logagg/ringbuffer_test.go`
  ```go
  // Test: push + query round-trip
  // Test: ring buffer wraps at capacity (10K entries)
  // Test: oldest entries evicted when full
  // Test: query by service filters correctly
  // Test: query by level filters correctly
  // Test: query by time range filters correctly
  // Test: full-text search matches message and field values
  // Test: concurrent push/query is safe (race detection)
  // Test: empty buffer returns empty results
  ```

- [ ] **Step 185** [W] ~3m: Create `pkg/logagg/subscriber_test.go`
  ```go
  // Test: subscriber receives messages from transport
  // Test: subscriber feeds entries into ring buffer
  // Test: subscriber handles transport disconnect gracefully
  // Test: subscriber reconnects on transport recovery
  ```

- [ ] **Step 186** [B] ~1m: Run logagg tests
  ```bash
  cd ~/tmp/unheaded && go test -v -race ./pkg/logagg/... 2>&1 | tail -30
  ```

- [ ] **Step 187** [V] ~15s: All logagg tests pass with -race.

- [ ] **Step 188** [B] ~30s: Check coverage
  ```bash
  cd ~/tmp/unheaded && go test -cover ./pkg/logagg/...
  ```

- [ ] **Step 189** [V] ~15s: Coverage > 70% on core files.

- [ ] **Step 190** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "test(logagg): comprehensive tests — ring buffer, publisher, subscriber, concurrent safety"
  ```

### 3.3 — Dashboard Log API (Steps 191-205)

- [ ] **Step 191** [R] ~1m: Read dashboard-backend server structure
  ```bash
  ls ~/tmp/unheaded/cmd/dashboard-backend/internal/
  grep -rn "func.*Handler\|http.Handle\|mux\." ~/tmp/unheaded/cmd/dashboard-backend/internal/server/ --include="*.go" | head -20
  ```

- [ ] **Step 192** [W] ~5m: Create `cmd/dashboard-backend/internal/logs/handler.go`
  ```go
  // GET /api/v1/logs — query log entries
  // Query params: service, level, search, from, to, limit, offset
  // Returns JSON array of LogEntry objects

  type LogHandler struct {
      buffer *logagg.RingBuffer
  }

  func NewLogHandler(buf *logagg.RingBuffer) *LogHandler { ... }
  func (h *LogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { ... }
  ```

- [ ] **Step 193** [W] ~5m: Create `cmd/dashboard-backend/internal/logs/websocket.go`
  ```go
  // WebSocket /ws/logs — live tail
  // Query params: service, level (optional filters)
  // Streams new log entries as they arrive in real-time
  // Uses Wotan subscription under the hood

  type LogStream struct {
      subscriber *logagg.Subscriber
      buffer     *logagg.RingBuffer
  }

  func NewLogStream(sub *logagg.Subscriber, buf *logagg.RingBuffer) *LogStream { ... }
  func (ls *LogStream) ServeHTTP(w http.ResponseWriter, r *http.Request) { ... }
  ```

- [ ] **Step 194** [B] ~3m: Wire log endpoints into dashboard-backend main.go
  - Initialize RingBuffer(10000)
  - Initialize Subscriber with transport connection
  - Start Subscriber in background goroutine
  - Register `GET /api/v1/logs` → LogHandler
  - Register `/ws/logs` → LogStream WebSocket
  - Graceful shutdown: stop subscriber

- [ ] **Step 195** [B] ~30s: Build dashboard-backend
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/dashboard-backend/...
  ```

- [ ] **Step 196** [V] ~15s: Builds clean.

- [ ] **Step 197** [W] ~5m: Create `cmd/dashboard-backend/internal/logs/handler_test.go`
  ```go
  // Test: GET /api/v1/logs returns JSON array
  // Test: service filter works
  // Test: level filter works
  // Test: search filter works
  // Test: time range filter works
  // Test: pagination (limit + offset) works
  // Test: empty results return empty array (not null)
  ```

- [ ] **Step 198** [W] ~3m: Create `cmd/dashboard-backend/internal/logs/websocket_test.go`
  ```go
  // Test: WebSocket connection established
  // Test: new log entries stream to connected clients
  // Test: service filter in WebSocket works
  // Test: client disconnect handled gracefully
  ```

- [ ] **Step 199** [B] ~1m: Run log handler tests
  ```bash
  cd ~/tmp/unheaded && go test -race ./cmd/dashboard-backend/internal/logs/... 2>&1 | tail -15
  ```

- [ ] **Step 200** [V] ~15s: Pass.

- [ ] **Step 201** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(logs): dashboard log API — GET /api/v1/logs + WebSocket /ws/logs live tail"
  ```

### 3.4 — Dashboard Log UI (Steps 202-215)

- [ ] **Step 202** [W] ~5m: Create `dashboard/js/log-viewer.js`
  ```javascript
  // Log viewer component
  // - Service selector dropdown (populated from discovery or hardcoded list)
  // - Level filter: all/debug/info/warn/error/fatal
  // - Full-text search input with debounce
  // - Timestamp range picker (last 5m/15m/1h/24h/custom)
  // - Auto-scroll toggle (tail -f mode)
  // - Color-coded log levels (debug=gray, info=blue, warn=yellow, error=red, fatal=magenta)
  // - JSON expand/collapse for structured fields
  // - Copy-to-clipboard on click
  ```

- [ ] **Step 203** [W] ~5m: Create `dashboard/logs.html`
  - Include log-viewer.js
  - Toolbar: service dropdown, level filter, search box, time range, auto-scroll toggle
  - Main area: scrollable log output with color coding
  - Status bar: connection status, entry count, buffer usage

- [ ] **Step 204** [B] ~2m: Wire logs.html into dashboard navigation
  - Add "Logs" link to dashboard sidebar/nav
  - Add route in dashboard-backend for serving logs.html

- [ ] **Step 205** [B] ~30s: Verify dashboard builds and static files are served
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/dashboard-backend/...
  ```

- [ ] **Step 206** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(logs): dashboard log viewer UI — service filter, level filter, search, live tail"
  ```

### 3.5 — Wire Services to Log Publisher (Steps 207-222)

- [ ] **Step 207** [W] ~2m: Create helper function for standard service logger setup
  ```go
  // In pkg/logagg/setup.go:
  // SetupServiceLogger creates a zerolog.Logger with the logagg publisher hook.
  // One-liner for services to adopt centralized logging.

  func SetupServiceLogger(serviceName string, conn transport.Connection) zerolog.Logger {
      publisher := NewPublisher(serviceName, conn)
      return zerolog.New(os.Stdout).
          With().Timestamp().Str("service", serviceName).Logger().
          Hook(publisher)
  }
  ```

Now wire into each service (one line change per service — add hook to existing logger):

- [ ] **Step 208** [B] ~2m: Wire timeguru
- [ ] **Step 209** [B] ~2m: Wire captain
- [ ] **Step 210** [B] ~2m: Wire architect
- [ ] **Step 211** [B] ~2m: Wire micromanager
- [ ] **Step 212** [B] ~2m: Wire monad
- [ ] **Step 213** [B] ~2m: Wire sophia
- [ ] **Step 214** [B] ~2m: Wire dashboard-backend
- [ ] **Step 215** [B] ~2m: Wire kanban-app
- [ ] **Step 216** [B] ~2m: Wire trace-collector-go
- [ ] **Step 217** [B] ~2m: Wire unheaded-daemon

- [ ] **Step 218** [B] ~2m: Full build
  ```bash
  cd ~/tmp/unheaded && go build ./...
  ```

- [ ] **Step 219** [B] ~5m: Full test
  ```bash
  cd ~/tmp/unheaded && go test -race ./... 2>&1 | grep -E "^(ok|FAIL)" | tail -40
  ```

- [ ] **Step 220** [V] ~15s: All pass.

- [ ] **Step 221** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(logs): wire all services to centralized log aggregation via Wotan"
  ```

### 3.6 — Phase 3 Verification (Steps 222-230)

- [ ] **Step 222** [B] ~1m: Verify log topics are defined
  ```bash
  grep -rn "logs\.\|logagg\.\|NewPublisher\|SetupServiceLogger" --include="*.go" ~/tmp/unheaded/cmd/ ~/tmp/unheaded/services/ | grep -v _test.go | grep -v vendor
  ```
  Every service should have a match.

- [ ] **Step 223** [B] ~1m: Verify dashboard log endpoints exist
  ```bash
  grep -rn "/api/v1/logs\|/ws/logs" --include="*.go" ~/tmp/unheaded/cmd/dashboard-backend/ | grep -v _test.go
  ```

- [ ] **Step 224** [B] ~30s: Verify log viewer HTML exists
  ```bash
  ls -la ~/tmp/unheaded/dashboard/logs.html ~/tmp/unheaded/dashboard/js/log-viewer.js
  ```

- [ ] **Step 225** [B] ~2m: Full build + test
  ```bash
  cd ~/tmp/unheaded && go build ./... && go test -race ./... 2>&1 | grep -E "^(ok|FAIL)" | tail -40
  ```

- [ ] **Step 226** [V] ~15s: All pass.

- [ ] **Step 227** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(logs): complete log aggregation — all services wired, dashboard UI, ring buffer"
  ```

- [ ] **Step 228** [V] ~15s: **PHASE 3 EXIT GATE** — Log Aggregation Complete
  - ✅ `pkg/logagg/` exists with publisher, ring buffer, query, subscriber
  - ✅ All logagg tests pass with -race
  - ✅ Every service has `logagg.SetupServiceLogger()` or equivalent hook
  - ✅ Dashboard has `GET /api/v1/logs` and `WebSocket /ws/logs`
  - ✅ `dashboard/logs.html` exists with viewer UI
  - ✅ Ring buffer retains 10K entries
  - ✅ `go build ./...` passes
  - ✅ `go test -race ./...` passes
  - If ALL pass → Phase 3 complete
  - If fail → DO NOT PROCEED.

---

## PHASE 4: SERVICE DISCOVERY (Steps 229-275) — Agent D

**Goal**: Three-layer service discovery. Kill ALL hardcoded IPs. Services register with Wotan on startup, deregister on shutdown. Static fallback when Wotan is down.
**Prerequisite**: Phase 1 complete (Doom Range ports assigned).
**Time**: 4-6 hours
**Agent**: Agent D (runs PARALLEL with Phases 2-3)

### 4.1 — Extend Existing Discovery Package (Steps 229-250)

- [ ] **Step 229** [R] ~1m: Read existing pkg/discovery/ to understand current state
  ```bash
  ls ~/tmp/unheaded/pkg/discovery/
  cat ~/tmp/unheaded/pkg/discovery/*.go | head -100
  ```

- [ ] **Step 230** [B] ~3m: Assess what exists vs what's needed:
  ```bash
  grep -rn "func\|type\|interface" ~/tmp/unheaded/pkg/discovery/*.go | grep -v _test.go
  ```
  Document: what's already implemented, what needs adding.

- [ ] **Step 231** [W] ~5m: Create/update `pkg/discovery/scanner.go` — filesystem convention scanner
  ```go
  // scanner.go implements Layer 1: Convention-based discovery.
  // Scans /opt/unheaded/<service>/config.yaml for declared service endpoints.
  // Returns []ServiceEndpoint for all discovered services.
  //
  // Convention:
  //   /opt/unheaded/<service>/config.yaml must contain:
  //     name: <service-name>
  //     port: <port>
  //     proto: <http|grpc>
  //     health: <health-endpoint-path>

  type Scanner struct {
      basePath string // default: /opt/unheaded
  }

  func NewScanner(basePath string) *Scanner { ... }
  func (s *Scanner) Scan() ([]ServiceEndpoint, error) { ... }
  ```

- [ ] **Step 232** [W] ~5m: Create/update `pkg/discovery/registrar.go` — Wotan-based registration
  ```go
  // registrar.go implements Layer 3: Wotan registration.
  // Services register on startup via system.discovery.register topic.
  // Services deregister on shutdown via system.discovery.deregister topic.
  // Registration message: { name, port, proto, health, pid, started_at }

  type Registrar struct {
      transport transport.Connection
      service   ServiceEndpoint
  }

  func NewRegistrar(conn transport.Connection, svc ServiceEndpoint) *Registrar { ... }
  func (r *Registrar) Register(ctx context.Context) error { ... }
  func (r *Registrar) Deregister(ctx context.Context) error { ... }
  ```

- [ ] **Step 233** [W] ~5m: Create/update `pkg/discovery/resolver.go` — service name resolution
  ```go
  // resolver.go implements the three-layer resolution cascade:
  // 1. Check Wotan registry (system.discovery.* topics)
  // 2. Check port scan (is the expected port open?)
  // 3. Check convention (/opt/unheaded/<service>/config.yaml)
  // 4. Fallback: static config from services.yaml
  //
  // Resolution priority: Wotan > Port scan > Convention > Static

  type Resolver struct {
      wotan    transport.Connection
      scanner  *Scanner
      static   *StaticConfig
  }

  func NewResolver(conn transport.Connection, scanner *Scanner, static *StaticConfig) *Resolver { ... }
  func (r *Resolver) Resolve(ctx context.Context, serviceName string) (ServiceEndpoint, error) { ... }
  func (r *Resolver) ResolveAll(ctx context.Context) ([]ServiceEndpoint, error) { ... }
  ```

- [ ] **Step 234** [W] ~3m: Create/update `pkg/discovery/static.go` — static fallback config
  ```go
  // static.go provides the last-resort static service map.
  // Used when Wotan is down and convention scan fails.
  // Loaded from configs/services.yaml

  type StaticConfig struct {
      Services map[string]ServiceEndpoint
  }

  func LoadStaticConfig(path string) (*StaticConfig, error) { ... }
  ```

- [ ] **Step 235** [W] ~2m: Create `configs/services.yaml` — static fallback file
  ```yaml
  # Static service discovery fallback
  # Used when Wotan is unavailable
  services:
    wotan: { host: localhost, port: 18001, proto: grpc }
    timeguru: { host: localhost, port: 19000, proto: http }
    captain: { host: localhost, port: 19002, proto: http }
    architect: { host: localhost, port: 19001, proto: http }
    micromanager: { host: localhost, port: 19003, proto: http }
    monad: { host: localhost, port: 19004, proto: http }
    sophia: { host: localhost, port: 19005, proto: http }
    dashboard: { host: localhost, port: 20000, proto: http }
    kanban: { host: localhost, port: 20001, proto: http }
  ```

- [ ] **Step 236** [B] ~30s: Verify discovery package compiles
  ```bash
  cd ~/tmp/unheaded && go build ./pkg/discovery/
  ```

- [ ] **Step 237** [V] ~15s: Compiles.

- [ ] **Step 238** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add pkg/discovery/ configs/services.yaml && git commit -m "feat(discovery): three-layer service discovery — Wotan, port-scan, convention, static fallback"
  ```

### 4.2 — Discovery Tests (Steps 239-248)

- [ ] **Step 239** [W] ~5m: Create/update `pkg/discovery/scanner_test.go`
  ```go
  // Test: scan empty directory returns empty
  // Test: scan directory with valid config.yaml returns service
  // Test: scan directory with invalid YAML returns error
  // Test: scan multiple services returns all
  ```

- [ ] **Step 240** [W] ~5m: Create/update `pkg/discovery/registrar_test.go`
  ```go
  // Test: register sends message to system.discovery.register topic
  // Test: deregister sends message to system.discovery.deregister topic
  // Test: register with nil transport returns error
  // Test: register message contains all required fields
  ```

- [ ] **Step 241** [W] ~5m: Create/update `pkg/discovery/resolver_test.go`
  ```go
  // Test: resolve known service via Wotan registry
  // Test: resolve falls back to port scan when Wotan down
  // Test: resolve falls back to convention scan
  // Test: resolve falls back to static config
  // Test: resolve unknown service returns error
  // Test: resolve with all layers down returns meaningful error
  ```

- [ ] **Step 242** [W] ~3m: Create `pkg/discovery/static_test.go`
  ```go
  // Test: load valid services.yaml
  // Test: load missing file returns error
  // Test: load invalid YAML returns error
  ```

- [ ] **Step 243** [B] ~1m: Run discovery tests
  ```bash
  cd ~/tmp/unheaded && go test -v -race ./pkg/discovery/... 2>&1 | tail -30
  ```

- [ ] **Step 244** [V] ~15s: All pass.

- [ ] **Step 245** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "test(discovery): comprehensive tests — scanner, registrar, resolver, static fallback"
  ```

### 4.3 — Wire Services to Discovery (Steps 246-265)

- [ ] **Step 246** [W] ~3m: Create discovery helper for services
  ```go
  // In pkg/discovery/setup.go:
  // SetupServiceDiscovery registers a service on startup and
  // deregisters on SIGTERM/SIGINT.
  // One-liner for services to adopt discovery.

  func SetupServiceDiscovery(ctx context.Context, conn transport.Connection, name string, port int) (*Registrar, error) {
      reg := NewRegistrar(conn, ServiceEndpoint{Name: name, Port: port, ...})
      if err := reg.Register(ctx); err != nil {
          // Log warning but don't fail startup — discovery is best-effort
          log.Warn().Err(err).Msg("discovery registration failed — service will operate without discovery")
          return reg, nil
      }
      // Register shutdown hook
      go func() {
          <-ctx.Done()
          reg.Deregister(context.Background())
      }()
      return reg, nil
  }
  ```

Now wire into each service:

- [ ] **Step 247** [B] ~2m: Wire timeguru — add `discovery.SetupServiceDiscovery()` to startup
- [ ] **Step 248** [B] ~2m: Wire captain
- [ ] **Step 249** [B] ~2m: Wire architect
- [ ] **Step 250** [B] ~2m: Wire micromanager
- [ ] **Step 251** [B] ~2m: Wire monad
- [ ] **Step 252** [B] ~2m: Wire sophia
- [ ] **Step 253** [B] ~2m: Wire dashboard-backend
- [ ] **Step 254** [B] ~2m: Wire kanban-app
- [ ] **Step 255** [B] ~2m: Wire trace-collector-go
- [ ] **Step 256** [B] ~2m: Wire unheaded-daemon

- [ ] **Step 257** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(discovery): wire all services to register/deregister with Wotan discovery"
  ```

### 4.4 — Kill Hardcoded IPs (Steps 258-265)

- [ ] **Step 258** [B] ~2m: Find ALL remaining hardcoded 10.10.10 IPs in production code
  ```bash
  grep -rn "10\.10\.10\." --include="*.go" ~/tmp/unheaded/ | grep -v _test.go | grep -v vendor | grep -v docs/
  ```

- [ ] **Step 259** [B] ~5m: Replace hardcoded IPs with discovery lookups or ports.* constants
  For each file found in Step 258:
  - Replace `10.10.10.X:port` with `resolver.Resolve(ctx, "service-name")`
  - Or use `ports.DefaultWotanGRPC()` / `ports.DefaultAddr(ports.Timeguru)` for direct references

- [ ] **Step 260** [B] ~3m: Update CLI status/service/network commands to use resolver
  ```bash
  grep -rn "10\.10\.10\." ~/tmp/unheaded/cmd/unheaded-cli/ --include="*.go" | grep -v _test.go
  ```
  Replace all hardcoded endpoints with discovery resolution.

- [ ] **Step 261** [B] ~3m: Update dashboard health checker to use resolver
  ```bash
  grep -rn "10\.10\.10\." ~/tmp/unheaded/cmd/dashboard-backend/internal/ --include="*.go" | grep -v _test.go
  ```

- [ ] **Step 262** [B] ~3m: Update cert-gen service map to use ports.* and resolver
  ```bash
  grep -rn "10\.10\.10\." ~/tmp/unheaded/cmd/cert-gen/ --include="*.go"
  ```

- [ ] **Step 263** [B] ~2m: Verify zero hardcoded IPs remain
  ```bash
  echo "=== Remaining 10.10.10 in production code ==="
  grep -rn "10\.10\.10\." --include="*.go" ~/tmp/unheaded/ | grep -v _test.go | grep -v vendor | grep -v docs/ | grep -v archive/ || echo "CLEAN — zero hardcoded IPs!"
  ```

- [ ] **Step 264** [V] ~15s: Zero hardcoded IPs in production code.

- [ ] **Step 265** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(discovery): kill all hardcoded 10.10.10 IPs — replaced with discovery resolution"
  ```

### 4.5 — Phase 4 Verification (Steps 266-275)

- [ ] **Step 266** [B] ~2m: Full build
  ```bash
  cd ~/tmp/unheaded && go build ./...
  ```

- [ ] **Step 267** [B] ~5m: Full test suite
  ```bash
  cd ~/tmp/unheaded && go test -race -count=1 ./... 2>&1 | grep -E "^(ok|FAIL)" | tail -40
  ```

- [ ] **Step 268** [V] ~15s: All pass.

- [ ] **Step 269** [B] ~1m: Verify all services have discovery registration
  ```bash
  grep -rn "SetupServiceDiscovery\|discovery.Register\|discovery.NewRegistrar" --include="*.go" ~/tmp/unheaded/cmd/ ~/tmp/unheaded/services/ | grep -v _test.go | grep -v vendor
  ```

- [ ] **Step 270** [B] ~1m: Verify static fallback exists
  ```bash
  cat ~/tmp/unheaded/configs/services.yaml
  ```

- [ ] **Step 271** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "feat(discovery): complete three-layer service discovery — all services wired, IPs killed"
  ```

- [ ] **Step 272** [V] ~15s: **PHASE 4 EXIT GATE** — Service Discovery Complete
  - ✅ `pkg/discovery/` has scanner, registrar, resolver, static fallback
  - ✅ All discovery tests pass with -race
  - ✅ Every service registers on startup, deregisters on shutdown
  - ✅ Zero hardcoded 10.10.10 IPs in production code
  - ✅ Static fallback `configs/services.yaml` exists
  - ✅ CLI uses resolver instead of hardcoded endpoints
  - ✅ `go build ./...` passes
  - ✅ `go test -race ./...` passes
  - If ALL pass → Phase 4 complete
  - If fail → DO NOT PROCEED.

---

## PHASE 5: DOCUMENTATION (Steps 273-290) — Agent E

**Goal**: Update all 8 documentation layers to reflect Four Pillars changes.
**Prerequisite**: Phase 1 complete (runs parallel with Phases 2-4).
**Time**: 2-3 hours
**Agent**: Agent E

- [ ] **Step 273** [B] ~5m: Update CLAUDE.md — Port registry section, transport priority, discovery, logging
- [ ] **Step 274** [B] ~5m: Update README.md — Architecture section with Doom Range, gRPC-first, log aggregation
- [ ] **Step 275** [B] ~3m: Update QUICKSTART.md — New port numbers, new endpoints
- [ ] **Step 276** [B] ~3m: Update battle-plan.md — Mark S34 Four Pillars as COMPLETE
- [ ] **Step 277** [B] ~3m: Update references/timeline.md — Mark pillars as completed

- [ ] **Step 278** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "docs(s36): update CLAUDE.md, README, QUICKSTART, battle-plan, timeline for Four Pillars"
  ```

- [ ] **Step 279** [B] ~5m: Create/update wiki pages:
  - `wiki/Port-Registry.md` — full Doom Range reference
  - `wiki/Transport-Cascade.md` — gRPC-first with fallback documentation
  - `wiki/Log-Aggregation.md` — log viewer, ring buffer, topics
  - `wiki/Service-Discovery.md` — three-layer approach

- [ ] **Step 280** [B] ~3m: Update `wiki/Home.md` — add links to new pages
- [ ] **Step 281** [B] ~3m: Update `wiki/_Sidebar.md` — add navigation entries
- [ ] **Step 282** [B] ~3m: Update `wiki/Architecture.md` — reference new subsystems

- [ ] **Step 283** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "docs(wiki): add Port Registry, Transport, Log Aggregation, Service Discovery wiki pages"
  ```

- [ ] **Step 284** [B] ~3m: Update docker-compose.yml comments with new port references
- [ ] **Step 285** [B] ~3m: Update nix/DEPLOYMENT.md with new ports and endpoints
- [ ] **Step 286** [B] ~3m: Update nix/README.md with new health check URLs
- [ ] **Step 287** [B] ~3m: Create `docs/sessions/S36-four-pillars-summary.md` — session summary

- [ ] **Step 288** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "docs(s36): update deployment docs, NixOS docs, session summary"
  ```

- [ ] **Step 289** [B] ~2m: Verify no stale port references in docs
  ```bash
  grep -rn "\":8080\"\|\":9090\"\|\":8000\"\|\":8001\"\|\":8002\"\|\":8003\"\|\":8004\"\|\":8005\"\|\":8006\"\|\":8081\"" ~/tmp/unheaded/docs/ ~/tmp/unheaded/wiki/ ~/tmp/unheaded/README.md ~/tmp/unheaded/QUICKSTART.md --include="*.md" || echo "CLEAN"
  ```

- [ ] **Step 290** [V] ~15s: **PHASE 5 EXIT GATE** — Documentation Complete
  - ✅ CLAUDE.md updated with all four pillars
  - ✅ README.md and QUICKSTART.md updated
  - ✅ Wiki has 4 new pages (Port Registry, Transport, Logs, Discovery)
  - ✅ Timeline and battle-plan updated
  - ✅ Zero stale port references in documentation
  - If ALL pass → Phase 5 complete

---

## PHASE 6: INTEGRATION VERIFICATION (Steps 291-310) — Coordinator

**Goal**: Full-stack verification that all four pillars work together. Final commit and tag.
**Prerequisite**: ALL phases (1-5) complete.
**Time**: 1-2 hours
**Agent**: Coordinator

- [ ] **Step 291** [B] ~2m: Full build — clean
  ```bash
  cd ~/tmp/unheaded && go clean ./... && go build ./...
  ```

- [ ] **Step 292** [V] ~15s: Build passes.

- [ ] **Step 293** [B] ~5m: Full test suite with race detection
  ```bash
  cd ~/tmp/unheaded && go test -race -count=1 ./... 2>&1 | tee /tmp/s36-test-results.txt | grep -E "^(ok|FAIL)" | tail -40
  ```

- [ ] **Step 294** [V] ~15s: All tests pass.

- [ ] **Step 295** [B] ~1m: Go vet — static analysis
  ```bash
  cd ~/tmp/unheaded && go vet ./...
  ```

- [ ] **Step 296** [V] ~15s: Zero vet issues.

- [ ] **Step 297** [B] ~2m: **ZERO OLD PORTS** — Final port audit
  ```bash
  echo "=== Old ports in ALL .go files (including tests) ==="
  grep -rn "\":8080\"\|\":9090\"\|\":9080\"\|\":8000\"\|\":8001\"\|\":8002\"\|\":8003\"\|\":8004\"\|\":8005\"\|\":8006\"\|\":8081\"\|\":6660\"\|\":5555\"" --include="*.go" ~/tmp/unheaded/ | grep -v vendor | grep -v docs/ | wc -l
  echo "=== Compare to baseline ==="
  cat ~/tmp/unheaded/.port-audit-baseline.txt
  ```

- [ ] **Step 298** [V] ~15s: Old port count should be dramatically reduced (ideally zero in production, minimal in tests).

- [ ] **Step 299** [B] ~1m: **ZERO HARDCODED IPs** — Final IP audit
  ```bash
  grep -rn "10\.10\.10\." --include="*.go" ~/tmp/unheaded/ | grep -v _test.go | grep -v vendor | grep -v docs/ | wc -l
  ```

- [ ] **Step 300** [V] ~15s: Zero hardcoded IPs in production code.

- [ ] **Step 301** [B] ~1m: Verify new packages exist and have tests
  ```bash
  echo "=== pkg/ports ==="
  ls ~/tmp/unheaded/pkg/ports/*.go
  echo "=== pkg/transport ==="
  ls ~/tmp/unheaded/pkg/transport/*.go
  echo "=== pkg/logagg ==="
  ls ~/tmp/unheaded/pkg/logagg/*.go
  echo "=== pkg/discovery ==="
  ls ~/tmp/unheaded/pkg/discovery/*.go
  echo "=== configs ==="
  ls ~/tmp/unheaded/configs/
  ```

- [ ] **Step 302** [B] ~1m: Verify dashboard log UI exists
  ```bash
  ls -la ~/tmp/unheaded/dashboard/logs.html ~/tmp/unheaded/dashboard/js/log-viewer.js
  ```

- [ ] **Step 303** [B] ~2m: Count total test coverage improvement
  ```bash
  cd ~/tmp/unheaded && go test -cover ./pkg/ports/ ./pkg/transport/ ./pkg/logagg/ ./pkg/discovery/ 2>&1
  ```

- [ ] **Step 304** [C] ~30s: **COMMIT CHECKPOINT** (final cleanup if any)
  ```bash
  cd ~/tmp/unheaded && git add -A && git status --short
  # Only commit if there are changes
  git diff --cached --quiet || git commit -m "chore(s36): final integration verification cleanup"
  ```

- [ ] **Step 305** [B] ~1m: Generate git log of all S36 commits
  ```bash
  cd ~/tmp/unheaded && git log --oneline --since="2026-02-24" | head -30
  ```

- [ ] **Step 306** [B] ~30s: Count files changed
  ```bash
  cd ~/tmp/unheaded && git diff --stat HEAD~20 HEAD | tail -5
  ```

- [ ] **Step 307** [W] ~3m: Update `.port-audit-baseline.txt` with final counts
  ```bash
  echo "POST-MIGRATION PORT AUDIT $(date)" >> ~/tmp/unheaded/.port-audit-baseline.txt
  echo "Old ports in production .go: $(grep -rn '\":8080\"\|\":9090\"\|\":9080\"\|\":8000\"\|\":8001\"\|\":8002\"\|\":8003\"\|\":8004\"\|\":8005\"\|\":8006\"\|\":8081\"\|\":6660\"\|\":5555\"' --include='*.go' ~/tmp/unheaded/ | grep -v _test.go | grep -v vendor | grep -v docs/ | wc -l)" >> ~/tmp/unheaded/.port-audit-baseline.txt
  echo "10.10.10 IPs in production .go: $(grep -rn '10\.10\.10\.' --include='*.go' ~/tmp/unheaded/ | grep -v _test.go | grep -v vendor | grep -v docs/ | wc -l)" >> ~/tmp/unheaded/.port-audit-baseline.txt
  cat ~/tmp/unheaded/.port-audit-baseline.txt
  ```

- [ ] **Step 308** [C] ~30s: Final commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "chore(s36): Four Pillars complete — Port Authority, gRPC-First, Log Aggregation, Service Discovery"
  ```

- [ ] **Step 309** [V] ~15s: Working tree clean.

- [ ] **Step 310** [V] ~15s: **PHASE 6 EXIT GATE — S36 COMPLETE**
  - ✅ `go build ./...` passes
  - ✅ `go test -race ./...` passes
  - ✅ `go vet ./...` passes
  - ✅ Zero old port references in production code
  - ✅ Zero hardcoded 10.10.10 IPs in production code
  - ✅ `pkg/ports/` — Doom Range port constants
  - ✅ `pkg/transport/` — gRPC-first with cascade fallback
  - ✅ `pkg/logagg/` — centralized log aggregation
  - ✅ `pkg/discovery/` — three-layer service discovery
  - ✅ Dashboard has log viewer UI
  - ✅ All 8 documentation layers updated
  - ✅ All commits use conventional commit format
  - **THE FOUR PILLARS STAND.**

---

## APPENDIX A: EMERGENCY PROCEDURES

### If Port Migration Breaks Build
1. `grep -rn "8080\|9090\|8000" --include="*.go" ~/tmp/unheaded/ | grep -v _test.go` — find stale references
2. Check for import cycles: `go build ./... 2>&1 | grep "import cycle"`
3. Check that `pkg/ports` doesn't import anything from cmd/ or services/

### If gRPC Transport Fails
1. Verify Wotan runs on port 18001: check `services/wotan/cmd/wotan/main.go`
2. Verify `google.golang.org/grpc` is in go.mod: `grep grpc go.mod`
3. If missing: `go get google.golang.org/grpc@latest`
4. Services should automatically fall back to HTTP — check cascade logic

### If Log Aggregation Overloads Wotan
1. Reduce ring buffer: `logagg.NewRingBuffer(1000)` instead of 10000
2. Add rate limiter to publisher: max 10 msgs/sec per service
3. Disable debug-level forwarding: only info/warn/error/fatal

### If Service Discovery Creates Startup Loop
1. Registration is fire-and-forget — services MUST start even if registration fails
2. Check `SetupServiceDiscovery` handles errors gracefully (log.Warn, not fatal)
3. Static fallback should work without Wotan: `configs/services.yaml`

### If Tests Fail After Port Migration
1. `grep -rn "8080\|9090\|8000\|8001\|8002\|8003" --include="*_test.go" ~/tmp/unheaded/` — find stale test expectations
2. Gateway config tests are the most likely to fail — check `services/gateway/config/config_test.go`
3. Dashboard backend tests reference many ports — check `cmd/dashboard-backend/`

---

## APPENDIX B: QUICK REFERENCE

### Port Registry (Doom Range)
```
INFRASTRUCTURE   16666-16999  doom-bridge(16666) trace-collector(16670/16671)
CONTROL PLANE    17000-17999  daemon(17000/17001) cli(17010)
WOTAN            18000-18099  http(18000) grpc(18001)
CORE SERVICES    19000-19999  timeguru(19000) architect(19001) captain(19002) micromanager(19003) monad(19004) sophia(19005) cuirass(19006)
APPLICATIONS     20000-20999  dashboard(20000) kanban(20001) wiki(20002)
GATEWAY          21000-21443  http(21000) https(21443)
UTILITIES        22000-22099  cert-gen(22000)
CUSTOMER         26000-26666  reserved
```

### Transport Cascade
```
1. gRPC → localhost:18001 (Wotan gRPC)
2. HTTP/3 → https://localhost:18000 (if available)
3. HTTP/2 → https://localhost:18000
4. HTTP/1.1 → http://localhost:18000
```

### Log Topics
```
logs.<service>.debug
logs.<service>.info
logs.<service>.warn
logs.<service>.error
logs.<service>.fatal
```

### Discovery Topics
```
system.discovery.register    — service startup announcement
system.discovery.deregister  — service shutdown announcement
system.discovery.query       — resolve service by name
system.discovery.list        — list all registered services
```

### New Package Inventory
```
pkg/ports/       — Port constants (Doom Range)
pkg/transport/   — gRPC-first transport with cascade fallback
pkg/logagg/      — Log aggregation (publisher, ring buffer, query)
pkg/discovery/   — Three-layer service discovery
configs/         — Port registry YAML, static services fallback
```

---

*S36 Four Pillars Battle Plan — Forged 2026-02-24*
*7 Phases. 310 Steps. 5 Agents + Coordinator.*
*The Doom Range opens. The nervous system awakens. The Chronicler's Well fills. The Cartographer's Eye sees all.*
*From chaos, infrastructure. From infrastructure, platform. From platform, Kingdom.*
