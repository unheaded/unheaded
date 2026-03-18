# S34 Multi-Agent Infrastructure Sprint — Battle Plan
## "The Four Pillars" — Port Authority, gRPC-First, Log Aggregation, Service Discovery
## Forged: February 24, 2026 | Sprint Owner: Warmonger | Review: All Skills

---

## Agent Matrix

| Agent | Domain | Pillar | Est. Hours | Dependencies |
|-------|--------|--------|-----------|--------------|
| A | Port Migration | Pillar 1 | 4-6h | Port registry (Step 1) |
| B | gRPC-First Transport | Pillar 2 | 4-6h | Pillar 1 complete |
| C | Log Aggregation | Pillar 3 | 4-6h | Pillar 2 complete (gRPC transport) |
| D | Service Discovery | Pillar 4 | 4-6h | Pillar 1 complete |
| E | Documentation | All | 2-3h | Parallel with all |

**Critical Path**: Registry → Pillar 1 → Pillar 2 → Pillar 3
**Parallel Path**: Pillar 4 can run alongside Pillars 2-3

---

## PHASE 0: FOUNDATION (Steps 1-15) — All Agents

### Step 1: Create Port Registry Source of Truth
```bash
cat > /opt/unheaded/port-registry.yaml << 'EOF'
# Unheaded Port Registry — "The Doom Range"
# Source of truth for all service port allocations
# Range: 16666-26666
# Updated: 2026-02-24

infrastructure:
  doom-bridge:
    port: 16666
    proto: websocket
    description: "Doom frame streaming (Fenrir's Eye)"
  doom-go-injector:
    port: 16667
    proto: tcp
    description: "Packet injection service"
  trace-collector-http:
    port: 16670
    proto: http
    description: "Trace collector health/readiness"
  trace-collector-metrics:
    port: 16671
    proto: http
    description: "Trace collector Prometheus metrics"

control-plane:
  unheaded-daemon-http:
    port: 17000
    proto: http
    description: "Control plane REST API"
  unheaded-daemon-grpc:
    port: 17001
    proto: grpc
    description: "Control plane streaming"
  unheaded-cli:
    port: 17010
    proto: http
    description: "CLI server mode"

wotan:
  wotan-http:
    port: 18000
    proto: http
    description: "Wotan REST API / control plane"
  wotan-grpc:
    port: 18001
    proto: grpc
    description: "Wotan gRPC streaming data plane"

services:
  timeguru:
    port: 19000
    proto: http
    description: "Timeline tracking service"
  architect:
    port: 19001
    proto: http
    description: "Design service"
  captain:
    port: 19002
    proto: http
    description: "Strategy service"
  micromanager:
    port: 19003
    proto: http
    description: "Execution service"
  monad:
    port: 19004
    proto: http
    description: "Unified state management"
  sophia:
    port: 19005
    proto: http
    description: "Knowledge graph service"

applications:
  dashboard-backend:
    port: 20000
    proto: http
    description: "Dashboard metrics + WebSocket"
  kanban-app:
    port: 20001
    proto: http
    description: "Kanban board (meta moment)"
  wiki-server:
    port: 20002
    proto: http
    description: "Documentation server"

gateway:
  gateway-http:
    port: 21000
    proto: http
    description: "Gateway HTTP"
  gateway-https:
    port: 21443
    proto: https
    description: "Gateway HTTPS (TLS termination)"

utilities:
  cert-gen:
    port: 22000
    proto: http
    description: "Certificate generation"

customer:
  reserved:
    range: "26000-26666"
    description: "Customer application ports"
EOF
```
**Verify**: File exists, YAML parses, no duplicate ports
**Exit Gate**: `python3 -c "import yaml; d=yaml.safe_load(open('/opt/unheaded/port-registry.yaml')); print('OK')"` — or validate manually

### Step 2: Create Go Port Constants
```bash
# Create pkg/config/ports.go with all port constants
```
File: `pkg/config/ports.go`
```go
package config

// Port Registry — "The Doom Range" (16666-26666)
// Source of truth: /opt/unheaded/port-registry.yaml
// All Unheaded services use high ports to avoid conflicts.

const (
    // Infrastructure Tier (16666-16999)
    PortDoomBridge          = 16666
    PortDoomGoInjector      = 16667
    PortTraceCollectorHTTP  = 16670
    PortTraceCollectorMetrics = 16671

    // Control Plane (17000-17999)
    PortDaemonHTTP          = 17000
    PortDaemonGRPC          = 17001
    PortCLIServer           = 17010

    // Wotan (18000-18099)
    PortWotanHTTP           = 18000
    PortWotanGRPC           = 18001

    // Core Services (19000-19999)
    PortTimeguru            = 19000
    PortArchitect           = 19001
    PortCaptain             = 19002
    PortMicromanager        = 19003
    PortMonad               = 19004
    PortSophia              = 19005

    // Applications (20000-20999)
    PortDashboard           = 20000
    PortKanban              = 20001
    PortWikiServer          = 20002

    // Gateway (21000-21443)
    PortGatewayHTTP         = 21000
    PortGatewayHTTPS        = 21443

    // Utilities (22000-22099)
    PortCertGen             = 22000

    // Customer Apps (26000-26666)
    PortCustomerStart       = 26000
    PortCustomerEnd         = 26666
)

// DefaultWotanGRPCAddr returns the default Wotan gRPC address.
func DefaultWotanGRPCAddr() string {
    return "localhost:18001"
}

// DefaultWotanHTTPAddr returns the default Wotan HTTP address.
func DefaultWotanHTTPAddr() string {
    return "localhost:18000"
}
```
**Verify**: `go build ./pkg/config/`

### Steps 3-15: Verification
- [ ] Step 3: Verify port registry has no duplicates (script)
- [ ] Step 4: Verify Go constants match YAML registry
- [ ] Step 5: Run `go build ./...` baseline — must pass before changes
- [ ] Step 6: Run `go test ./...` baseline — must pass before changes
- [ ] Step 7: Git stash any uncommitted work
- [ ] Step 8: Create branch `feat/s34-infrastructure-pillars`
- [ ] Step 9: Commit port registry + Go constants
- [ ] Step 10-15: Reserve for foundation verification

---

## PHASE 1: PORT MIGRATION — Agent A (Steps 16-80)

### Core Service Port Updates (Steps 16-33)

Each step follows the pattern:
1. Update default port in source
2. Update all references in same file
3. Update test expectations
4. Verify: `go build ./cmd/<service>/...` + `go test ./cmd/<service>/...`

| Step | Service | File | Old Port | New Port |
|------|---------|------|----------|----------|
| 16-17 | timeguru | services/timeguru/cmd/timeguru/main.go | 8000 | 19000 |
| 18-19 | architect | services/architect/cmd/architect/main.go | 8001 | 19001 |
| 20-21 | captain | services/captain/cmd/captain/main.go | 8002 | 19002 |
| 22-23 | micromanager | services/micromanager/cmd/micromanager/main.go | 8003 | 19003 |
| 24-25 | monad | cmd/monad/main.go | 8004 | 19004 |
| 26-27 | sophia | cmd/sophia/main.go | 8005 | 19005 |
| 28-29 | wiki-server | cmd/wiki-server/main.go | 8007 | 20002 |
| 30-31 | dashboard | cmd/dashboard-backend/main.go | 8080 | 20000 |
| 32-33 | kanban | cmd/kanban-app/main.go | 8081 | 20001 |

### Infrastructure Port Updates (Steps 34-45)

| Step | Service | File | Old Port | New Port |
|------|---------|------|----------|----------|
| 34-35 | doom-bridge | cmd/doom-bridge/main.go | 6660 | 16666 |
| 36-37 | trace-collector HTTP | cmd/trace-collector-go/main.go | 9092 | 16670 |
| 38-39 | trace-collector metrics | cmd/trace-collector-go/main.go | 9100 | 16671 |
| 40-41 | unheaded-daemon HTTP | cmd/unheaded-daemon/main.go | 8080 | 17000 |
| 42-43 | unheaded-daemon gRPC | cmd/unheaded-daemon/main.go | 9090 | 17001 |
| 44-45 | wotan HTTP | services/wotan/cmd/wotan/main.go | 9080 | 18000 |
| 46-47 | wotan gRPC | services/wotan/cmd/wotan/main.go | 9090 | 18001 |

### Wotan Address Updates (Steps 48-60)
Update all `localhost:9090` and `localhost:9080` references to new Wotan ports:
- [ ] Step 48: services/timeguru — wotan addr 9080→18000, grpc 9090→18001
- [ ] Step 49: services/architect — wotan addr →18000
- [ ] Step 50: services/captain — wotan addr 9090→18001
- [ ] Step 51: services/micromanager — wotan addr
- [ ] Step 52: cmd/monad — wotan 9090→18001
- [ ] Step 53: cmd/sophia — wotan 9090→18001
- [ ] Step 54: cmd/dashboard-backend — wotan 9090→18001
- [ ] Step 55: cmd/kanban-app — wotan 9080→18000, grpc 9090→18001
- [ ] Step 56: cmd/trace-collector-go — wotan 9090→18001
- [ ] Step 57: cmd/unheaded-daemon — wotan 5555→18001
- [ ] Step 58: cmd/unheaded-cli — all hardcoded endpoints
- [ ] Step 59: All test files referencing old ports
- [ ] Step 60: Verify `grep -rn "9090\|9080\|8080\|8000\|8001\|8002\|8003\|8004\|8005" --include="*.go" | grep -v test | grep -v vendor` returns only acceptable matches

### Config + Container Updates (Steps 61-75)
- [ ] Step 61-63: Update Docker Compose with new ports
- [ ] Step 64-68: Update NixOS container definitions (nix/containers/*.nix)
- [ ] Step 69-71: Update gateway nginx config
- [ ] Step 72-73: Update CORS allowed origins in middleware.go
- [ ] Step 74: Update Makefile port references
- [ ] Step 75: Update scripts referencing old ports

### Config Override Verification (Steps 76-80)
- [ ] Step 76: Verify every service accepts --port flag
- [ ] Step 77: Verify every service reads PORT env var
- [ ] Step 78: Verify every service has config file support
- [ ] Step 79: Full build: `go build ./...`
- [ ] Step 80: Full test: `go test ./...`
- [ ] **EXIT GATE**: Zero port conflicts. All services build. All tests pass.

---

## PHASE 2: gRPC-FIRST TRANSPORT — Agent B (Steps 81-130)

### Transport Package (Steps 81-95)
- [ ] Step 81: Create `pkg/transport/transport.go` — transport interface
- [ ] Step 82: Create `pkg/transport/grpc.go` — gRPC transport implementation
- [ ] Step 83: Create `pkg/transport/http.go` — HTTP fallback implementation  
- [ ] Step 84: Create `pkg/transport/cascade.go` — try gRPC → HTTP/3 → HTTP/2 → HTTP/1.1
- [ ] Step 85: Create `pkg/transport/health.go` — dual health check (gRPC + HTTP)
- [ ] Step 86-90: Unit tests for all transport package files
- [ ] Step 91-95: Integration test: cascade fallback behavior

### Service Transport Flip (Steps 96-115)
For each service:
1. Import `pkg/transport`
2. Add `--transport` flag (grpc|http|auto, default: grpc)
3. Connect to Wotan gRPC (18001) by default
4. Fallback to Wotan HTTP (18000) on gRPC failure
5. Add gRPC health service registration

- [ ] Steps 96-97: timeguru (already has dual transport — verify defaults)
- [ ] Steps 98-99: architect
- [ ] Steps 100-101: captain
- [ ] Steps 102-103: micromanager
- [ ] Steps 104-105: monad
- [ ] Steps 106-107: sophia
- [ ] Steps 108-109: dashboard-backend
- [ ] Steps 110-111: kanban-app
- [ ] Steps 112-113: unheaded-daemon
- [ ] Steps 114-115: trace-collector

### Health Monitoring (Steps 116-130)
- [ ] Step 116: Add grpc.health.v1 to Wotan
- [ ] Step 117-120: Dual health check in dashboard (gRPC + HTTP)
- [ ] Step 121-125: Update cross-service health consensus to include gRPC status
- [ ] Step 126-128: DEGRADED state when gRPC down but HTTP up
- [ ] Step 129: Full build verification
- [ ] Step 130: Full test verification
- [ ] **EXIT GATE**: All services start with gRPC. Fallback works. Health checks dual-protocol.

---

## PHASE 3: LOG AGGREGATION — Agent C (Steps 131-180)

### Log Publisher Package (Steps 131-145)
- [ ] Step 131: Create `pkg/logagg/publisher.go` — zerolog hook for Wotan
- [ ] Step 132: Create `pkg/logagg/ringbuffer.go` — in-memory ring buffer (10K lines)
- [ ] Step 133: Create `pkg/logagg/query.go` — log query interface
- [ ] Step 134-140: Unit tests
- [ ] Step 141: Define Wotan topics: `logs.<service>.<level>`
- [ ] Step 142-145: Integration test with mock Wotan

### Dashboard Log API (Steps 146-160)
- [ ] Step 146: Add `GET /api/v1/logs` endpoint to dashboard-backend
- [ ] Step 147: Query params: service, lines, level, search, from, to
- [ ] Step 148: Add `ws://dashboard:20000/ws/logs` WebSocket endpoint
- [ ] Step 149: Live tail: subscribe to `logs.*` Wotan topics
- [ ] Step 150-155: API tests
- [ ] Step 156-160: WebSocket tests

### Dashboard Log UI (Steps 161-175)
- [ ] Step 161: Create `dashboard/js/log-viewer.js`
- [ ] Step 162: Service selector dropdown
- [ ] Step 163: Level filter (debug/info/warn/error/fatal)
- [ ] Step 164: Full-text search input
- [ ] Step 165: Timestamp range picker
- [ ] Step 166: Auto-scroll toggle (tail -f mode)
- [ ] Step 167: Color-coded log levels
- [ ] Step 168: JSON expand/collapse for structured fields
- [ ] Step 169-170: Create `dashboard/logs.html` page
- [ ] Step 171-175: Wire into dashboard navigation

### Service Integration (Steps 176-180)
- [ ] Step 176: Wire zerolog hook into all services (one-line per service)
- [ ] Step 177: Verify logs appear in dashboard within 5 seconds
- [ ] Step 178: Verify search works across services
- [ ] Step 179: Full build verification
- [ ] Step 180: Full test verification
- [ ] **EXIT GATE**: Logs visible in dashboard. Search works. Live tail works. 10K retention.

---

## PHASE 4: SERVICE DISCOVERY — Agent D (Steps 181-230)

### Discovery Package (Steps 181-200)
- [ ] Step 181: Create `pkg/discovery/scanner.go` — scan /opt/unheaded/*/config.yaml
- [ ] Step 182: Create `pkg/discovery/registrar.go` — Wotan register/deregister
- [ ] Step 183: Create `pkg/discovery/resolver.go` — resolve service name → endpoint
- [ ] Step 184: Create `pkg/discovery/static.go` — static fallback from services.yaml
- [ ] Step 185: Define Sophia dictionary entries for discovery messages
- [ ] Step 186-195: Unit tests for all discovery package files
- [ ] Step 196-200: Integration tests

### Service Integration (Steps 201-220)
- [ ] Step 201: Add startup registration to timeguru
- [ ] Step 202: Add startup registration to architect
- [ ] Step 203: Add startup registration to captain
- [ ] Step 204: Add startup registration to micromanager
- [ ] Step 205: Add startup registration to monad
- [ ] Step 206: Add startup registration to sophia
- [ ] Step 207: Add startup registration to dashboard-backend
- [ ] Step 208: Add startup registration to kanban-app
- [ ] Step 209: Add startup registration to wiki-server
- [ ] Step 210: Add shutdown deregistration to all services (graceful shutdown hook)
- [ ] Step 211-215: Replace hardcoded IPs in unheaded-cli with discovery lookups
- [ ] Step 216-220: Create static fallback /opt/unheaded/services.yaml

### Verification (Steps 221-230)
- [ ] Step 221: Start all services — verify registration messages appear in Wotan
- [ ] Step 222: Stop a service — verify deregistration message
- [ ] Step 223: Query discovery endpoint — verify all services listed
- [ ] Step 224: Kill Wotan — verify static fallback works
- [ ] Step 225: Restart Wotan — verify services re-register
- [ ] Step 226-228: Edge case tests (duplicate names, port conflicts, stale registrations)
- [ ] Step 229: Full build verification
- [ ] Step 230: Full test verification
- [ ] **EXIT GATE**: Services register/deregister. CLI uses discovery. Fallback works.

---

## PHASE 5: DOCUMENTATION — Agent E (Steps 231-260)

- [ ] Step 231-240: Update CLAUDE.md with all four pillars
- [ ] Step 241-245: Update wiki (Home, Architecture, _Sidebar, session index)
- [ ] Step 246-250: Create wiki pages (Port-Registry, Service-Discovery, Transport-Cascade, Log-Aggregation)
- [ ] Step 251-255: Update README.md, QUICKSTART.md with new ports
- [ ] Step 256-258: Update Docker Compose documentation
- [ ] Step 259-260: Verify all wiki links work
- [ ] **EXIT GATE**: All 8+ document layers updated. No stale port references.

---

## PHASE 6: INTEGRATION + VERIFICATION (Steps 261-280)

- [ ] Step 261: `go build ./...` — full build pass
- [ ] Step 262: `go test ./...` — full test pass
- [ ] Step 263: `go vet ./...` — static analysis pass
- [ ] Step 264: Verify zero port conflicts (grep for old ports)
- [ ] Step 265: Verify all services have 3-level config override
- [ ] Step 266: Verify gRPC health checks respond on all services
- [ ] Step 267: Verify HTTP health checks respond on all services
- [ ] Step 268: Verify logs appear in dashboard for each service
- [ ] Step 269: Verify service discovery registers all services
- [ ] Step 270: Verify service discovery handles Wotan restart
- [ ] Step 271-275: Security review of new endpoints (BlackMage)
- [ ] Step 276-278: Performance baseline (Scientist)
- [ ] Step 279: Final commit with conventional commit message
- [ ] Step 280: Push and tag v0.1.0-alpha.2
- [ ] **EXIT GATE**: ALL FOUR PILLARS OPERATIONAL. Build passes. Tests pass. Docs updated.

---

## Emergency Procedures

### If Port Migration Breaks Build
1. Check for stale port references: `grep -rn "8080\|9090\|8000" --include="*.go"`
2. Check test expectations match new ports
3. Check Docker Compose port mappings

### If gRPC Transport Fails
1. Services should automatically fall back to HTTP
2. Check Wotan is running on port 18001 (gRPC)
3. Check gRPC health service is registered

### If Log Aggregation Overloads Wotan
1. Reduce ring buffer from 10K to 1K lines
2. Rate-limit log publishing (1 msg/sec per service per level)
3. Disable debug-level log forwarding

### If Service Discovery Creates Startup Loop
1. Use static fallback: /opt/unheaded/services.yaml
2. Services should start even if registration fails
3. Registration is fire-and-forget with retry

---

_280 steps across 6 phases. 5 parallel agents. 4 pillars. 1 Kingdom._
_The Doom Range awaits. The King's Road opens. The Chronicler's Well fills._
_The Cartographer's Eye opens. LET'S BUILD._
