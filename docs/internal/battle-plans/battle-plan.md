# Battle Plan: The S34 Convergence — Service Discovery, Port Authority, gRPC-First, Aggregated Logging
## Convened: February 24, 2026 | Reason: Full Round Table — Barrister Presiding (Inaugural Session)
## Kingdom State: Age 1 (Alpha), ~99% Complete | ~260K Production LOC (~464K w/ tests) | 20 Service Binaries | S33 COMPLETE
## All 17 Skills Consulted: Captain, Architect, Micromanager, Developer, Timeguru, Calendar, Lore, Kingdom, Busboy, Warmonger, Scientist, BlackMage, Moat Ghost, RFC Editor, Round Table, Barrister (NEW), Librarian

---

## The Origin Myth — Continued

**February 23, 2026** — S33 Hardening Sprint. Doom becomes playable with keyboard input. LH/LB sign-extension fix breaks the frame 565 blockmap barrier. WebSocket frame streaming at 30 FPS. 6 unpushed commits stacked. The execution driver, fault detection, and keyboard pipeline all land. L1 cache dead code cleaned.

**February 24, 2026** — The Barrister takes the bench for the first time. The inaugural legal counsel session of the Unheaded Kingdom. Not a trial — a grand jury. Every skill summoned. The agenda: service discovery architecture, port authority (high ports, unique allocation, no conflicts), gRPC-first communication, aggregated logging visible from dashboards. Four infrastructure pillars forged in a single sitting.

**34 days from first commit. ~260K production LOC (~464K with tests). One engineer. One AI. One Kingdom. Now with legal counsel.**

---

### Situation Report

The Kingdom stands at ~260K production LOC (~464K with tests). Doom is playable — keyboard input works, LH/LB sign-extension fixed, 30 FPS WebSocket streaming operational. But the growth has exposed four critical infrastructure gaps that must be resolved before WS5 (Return to Core) can begin:

**1. Service Discovery is Hardcoded.** IP addresses scattered across 20 service binaries. `10.10.10.20:8000`, `10.10.10.21:8001` — baked into code like it's 2005. Since we own the infrastructure top-to-bottom, we can do better: scan open ports on containers, use uniform install conventions (`/opt/unheaded/<service>/`), and let services announce themselves to Wotan.

**2. Port Chaos.** Two confirmed conflicts: dashboard-backend and unheaded-daemon both default to `:8080`, and unheaded-daemon gRPC and wotan gRPC both default to `:9090`. Every service lives in the dinky 8000-9100 range that conflicts with local dev tooling. We need a dedicated high-port range (16666-26666, Doom-themed) with unique allocations per service.

**3. HTTP-First is Wrong.** Every service defaults to HTTP REST with Wotan as an optional addon. For a platform where Wotan IS the nervous system, every service should default to Wotan gRPC message bus. HTTP becomes the fallback (preferably HTTP/3→2→1). Health checks must monitor both gRPC AND HTTP.

**4. Logging is Local.** Structured JSON logs via zerolog everywhere — but they stay on the container. No aggregation, no dashboard visibility, no searchability. Test phase needs at minimum: last 10K lines per service version, viewable from the kanban/eBPF dashboard.

**Ground truth from git:** 30+ commits in S33. Build passes. All tests pass. 6 unpushed Doom commits from yesterday's session. The codebase is healthy but architecturally messy on the networking/communication layer.

**The Barrister's First Ruling:** These four decisions have licensing and IP implications. The service discovery mechanism, the port allocation scheme, the gRPC-first protocol, and the logging architecture are all differentiating infrastructure innovations. They must be documented in specs, tested thoroughly, and the IP protected. The Barrister recommends these be added to the protocol spec family as implementation guidance appendices.

---

### The Throne Speaks (Captain — Vision & Strategy)

**Strategic Position**: We're past proof-of-concept and into platform engineering. Doom proved the eBPF pipeline works. Now we need the platform to act like a platform — services that discover each other, communicate on a real bus, log to a real aggregator, and listen on ports that don't fight with `python -m http.server`.

**North Star**: Unchanged. "Production-ready infrastructure in hours, not months." But production-ready means services don't hardcode IPs, don't conflict on ports, don't default to the wrong transport, and don't hide their logs.

**Key Decision**: The four pillars discussed today are prerequisites for WS5. Without them, "return to core" means returning to the same mess. Fix the foundation. Then build the house.

**Risk to Vision**: Scope creep. These four items could each be a multi-week project. We need MINIMUM VIABLE implementations — enough to unblock WS5, not gold-plated. The Micromanager owns the scope gates.

### The Ledger Records (Micromanager — Execution & QA)

**Sprint Status**: S33 COMPLETE (Doom playable, 6 commits stacked). S34 begins now.

**Priority Stack** (ordered):

1. **P0 — Port Authority** (2-4 hours) — Allocate unique high ports (16666-26666), create port registry, update all 20 service binaries, verify zero conflicts. This is the fastest win with the most immediate impact — kills two confirmed conflicts plus prevents future ones.
2. **P0 — gRPC-First Default** (4-6 hours) — Flip every service to default to Wotan gRPC, HTTP as fallback. Add gRPC health checks alongside HTTP. All services should have CLI arg, env var, AND config file overrides for transport selection.
3. **P1 — Service Discovery** (6-8 hours) — Implement port-scanning + `/opt/unheaded/` convention discovery. Services register with Wotan on startup, deregister on shutdown. No more hardcoded IPs.
4. **P1 — Aggregated Logging** (4-6 hours) — Wotan topic `logs.*` for structured log forwarding. Last 10K lines per service per version stored in ring buffer. Dashboard endpoint for viewing/searching. Think kanban/eBPF dashboard but for logs.
5. **P0 — Push S33 commits + doc updates** — The 6 unpushed Doom commits need to land.

**QA Gates**: Every service must pass: (a) starts on its unique high port, (b) connects to Wotan gRPC by default, (c) falls back to HTTP gracefully, (d) publishes logs to `logs.<service>` topic, (e) is discoverable via the new mechanism.

**Acceptance Criteria**: `make test` passes with new ports. Zero port conflicts when all services run simultaneously. Logs visible from dashboard within 5 seconds of emission.

### The Blueprint Reveals (Architect — Infrastructure & Design)

**Architecture Health**: 20 service binaries across `cmd/` and `services/`. Strong service structure with health/ready/metrics endpoints. But the networking layer is ad-hoc — a mix of hardcoded addresses, inconsistent Wotan connection patterns, and two confirmed port conflicts.

**Port Authority Architecture (NEW)**:

The Unheaded Port Registry. Every service gets a unique port in the high range. The scheme:

```
PORT ALLOCATION REGISTRY — "The Doom Range" (16666-26666)
═══════════════════════════════════════════════════════════

INFRASTRUCTURE TIER (16666-16999)
  16666  doom-bridge          (WebSocket frame server — THE NUMBER OF THE BEAST)
  16667  doom-go-injector     (packet injection service)
  16670  trace-collector HTTP (health/metrics)
  16671  trace-collector metrics (Prometheus)

CONTROL PLANE (17000-17999)
  17000  unheaded-daemon HTTP (control plane REST)
  17001  unheaded-daemon gRPC (control plane streaming)
  17010  unheaded-cli         (when running as server)

WOTAN (18000-18099) — The Message Bus Gets Its Own Block
  18000  wotan HTTP           (REST API / control plane)
  18001  wotan gRPC           (streaming data plane)

CORE SERVICES (19000-19999)
  19000  timeguru             (timeline tracking)
  19001  architect            (design service)
  19002  captain              (strategy service)
  19003  micromanager         (execution service)
  19004  monad                (state management)
  19005  sophia               (knowledge graph)

APPLICATION TIER (20000-20999)
  20000  dashboard-backend    (metrics + WebSocket)
  20001  kanban-app           (meta moment)
  20002  wiki-server          (documentation)

GATEWAY (21000-21099)
  21000  gateway HTTP         (TLS termination)
  21443  gateway HTTPS        (TLS termination)

CERT UTILITIES (22000-22099)
  22000  cert-gen             (certificate generation)

FUTURE SERVICES (23000-25999)
  [reserved for expansion]

CUSTOMER APP TIER (26000-26666)
  26000+ customer applications ("the head")
```

**Why This Range**: 16666 starts with 666 — Doom's number, our proof of computational completeness. The range 16666-26666 gives us 10,000 ports — room for 10,000 services. No conflicts with standard HTTP (80/443/8080), database (3306/5432/6379/27017), or common dev tools (3000/5000/8000). The range is above the typical ephemeral port start (32768) concern on most systems and well below it, sitting in the "registered port" space (1024-49151) where IANA assigns specific services.

**Service Discovery Architecture (NEW)**:

Since we own infrastructure top-to-bottom, service discovery uses a three-layer approach:

```
Layer 1: CONVENTION-BASED (Static)
  Install path: /opt/unheaded/<service-name>/
  Config file:  /opt/unheaded/<service-name>/config.yaml
  Port defined: In config.yaml AND registry (source of truth)
  Binary name:  Matches directory name

Layer 2: PORT-SCAN (Dynamic)
  On container boot, scan /opt/unheaded/*/config.yaml
  Read declared ports, verify they're listening (TCP connect)
  Report results to Wotan topic: system.discovery.report
  Re-scan on SIGHUP for live reload

Layer 3: WOTAN REGISTRATION (Active)
  On startup: service publishes to system.discovery.register
    { "name": "timeguru", "port": 19000, "proto": ["grpc", "http"],
      "health": "/health", "ready": "/ready", "version": "0.1.0" }
  On shutdown: service publishes to system.discovery.deregister
  Wotan maintains authoritative service registry in BPF map
  Other services query Wotan for endpoint resolution
  Fallback: read /opt/unheaded/services.yaml (static registry)
```

**gRPC-First Architecture (NEW)**:

Every service MUST implement the following transport priority:

```
1. PRIMARY:  Wotan gRPC streaming (bi-directional, persistent connection)
2. FALLBACK: HTTP/3 (QUIC — zero RTT, multiplexed)
3. FALLBACK: HTTP/2 (TLS 1.3, multiplexed)
4. FALLBACK: HTTP/1.1 (TLS 1.3, keepalive)

Health checks:
  /health  — HTTP GET (always available, lightweight)
  /ready   — HTTP GET (service-specific readiness)
  gRPC:    — grpc.health.v1.Health/Check (standard gRPC health)

Monitor BOTH:
  - HTTP health check (200 OK)
  - gRPC health check (SERVING status)
  - If gRPC fails but HTTP works → DEGRADED (log, alert, attempt reconnect)
  - If both fail → DOWN (escalate per consensus severity table)
```

**Config Override Hierarchy** (every service, every setting):
```
1. Command-line flag:    --port=19000 --wotan-grpc=18001 --transport=grpc
2. Environment variable: PORT=19000 WOTAN_GRPC_ADDR=localhost:18001
3. Config file:          /opt/unheaded/<service>/config.yaml
4. Registry default:     /opt/unheaded/port-registry.yaml
5. Compiled default:     (hardcoded in source as ultimate fallback)
```

**Aggregated Logging Architecture (NEW)**:

```
SERVICE → zerolog (structured JSON) → stdout + Wotan topic logs.<service>
                                              ↓
                                    Wotan ring buffer
                                    (per-service, per-version)
                                    Last 10,000 lines retained
                                              ↓
                                    Dashboard /api/v1/logs endpoint
                                    - GET /api/v1/logs?service=timeguru&lines=100
                                    - GET /api/v1/logs?service=all&level=error
                                    - WebSocket /ws/logs (live tail)
                                              ↓
                                    Dashboard UI (kanban/eBPF style)
                                    - Service selector dropdown
                                    - Level filter (debug/info/warn/error)
                                    - Full-text search
                                    - Timestamp range
                                    - Auto-scroll (tail -f mode)

Storage:
  TEST PHASE:  In-memory ring buffer per service (10K lines × N services)
               Wotan topic: logs.<service>.<version>
               Retained until service restart or buffer overflow
  FUTURE:      Persistent log store (ELK/Loki/custom Wotan log aggregator)
               Configurable retention per observability backend strategy
```

**Technical Risks**:
- Port migration touches 20+ files — high regression surface. Need comprehensive grep + sed + test cycle.
- gRPC-first flip may break services that don't have Wotan connection handling yet.
- Log aggregation via Wotan adds load to the message bus. Ring buffer sizing matters.
- Service discovery port scanning could create startup ordering issues.

**Barrister Note on IP**: The port allocation scheme, service discovery convention, and gRPC-first transport cascade are all novel infrastructure design patterns. Document in ADR (Architecture Decision Record) format. These are trade-secret-protectable innovations in the aggregate — the specific combination of convention-based discovery + port scanning + Wotan registration is unique to Unheaded.

### The Anvil Reports (Developer — Implementation & Testing)

**Code Health**: BUILD PASS. ALL TESTS PASS. ~260K production LOC (~464K w/ tests). 20 service binaries. 6 unpushed S33 commits.

**Port Migration Effort Assessment**:
Files to change (confirmed by grep):
- `services/timeguru/cmd/timeguru/main.go` — port 8000 → 19000
- `services/architect/cmd/architect/main.go` — port 8001 → 19001
- `services/captain/cmd/captain/main.go` — port 8002 → 19002
- `services/micromanager/cmd/micromanager/main.go` — port 8003 → 19003
- `cmd/monad/main.go` — port 8004 → 19004
- `cmd/sophia/main.go` — port 8005 → 19005
- `cmd/wiki-server/main.go` — port 8007 → 20002
- `cmd/dashboard-backend/main.go` — port 8080 → 20000
- `cmd/kanban-app/main.go` — port 8081 → 20001
- `cmd/unheaded-daemon/main.go` — HTTP 8080 → 17000, gRPC 9090 → 17001
- `cmd/doom-bridge/main.go` — port 6660 → 16666
- `cmd/trace-collector-go/main.go` — HTTP 9092 → 16670, metrics 9100 → 16671
- `services/wotan/cmd/wotan/main.go` — HTTP 9080 → 18000, gRPC 9090 → 18001
- `cmd/unheaded-cli/cmd/service.go` — hardcoded endpoints → registry lookup
- `cmd/unheaded-cli/cmd/status.go` — hardcoded endpoints → registry lookup
- `cmd/unheaded-cli/cmd/network.go` — port references → registry lookup
- All test files referencing old ports
- Docker Compose configs
- NixOS container definitions (`nix/containers/*.nix`)
- Gateway nginx config
- CORS allowed origins in middleware

**Effort**: M-L (6-10 hours total across all four pillars). Port migration is the riskiest (widest blast radius). gRPC-first is the most architecturally important. Logging is the most visible win. Discovery is the most forward-looking.

**Implementation Priority Order**:
1. Create `/opt/unheaded/port-registry.yaml` — the source of truth
2. Port migration (all 20 binaries + tests + configs)
3. gRPC-first transport flip
4. Log aggregation via Wotan
5. Service discovery mechanism

**TDD Approach**: For each service port change:
- Update default in source
- Update test expectations
- Run `go test ./...` — must pass
- Run `go build ./...` — must pass
- Verify no duplicate port in registry

### The Hourglass Measures (Timeguru — Timeline & Milestones)

**Current Age**: Age 1 (Alpha Ascension), ~99% complete
**Velocity**: SUSTAINED HIGH. ~260K production LOC. 34 sessions. ~7.6K LOC/session average.
**Sprint count**: S34 (session 34 since founding)

**Timeline**:
```
S34 (Feb 24):
  Port Authority + gRPC-First + Logging Architecture + Service Discovery
  Push S33 commits. Update all docs. Forge battle plan.

Week 5 (Feb 25-28): Implementation Sprint
  Mon-Tue: Port migration across all 20 binaries + tests
  Wed: gRPC-first transport flip + health monitoring
  Thu: Aggregated logging via Wotan + dashboard endpoint
  Fri: Service discovery + integration testing

Week 6 (Mar 1-7): WS5 Prep
  Port eBPF patterns to production tracing (WITH correct ports/transport)
  Lich campaigns D1-D6 (WITH correct infrastructure)

Week 7+ (Mar 8 onwards): WS5 — Return to Core
  Production packet tracing on the new infrastructure foundation
```

**ETA to Port Migration Complete**: Feb 25 (high confidence — mechanical grep/replace + test)
**ETA to gRPC-First**: Feb 26 (high confidence — patterns exist in timeguru already)
**ETA to Log Aggregation MVP**: Feb 27 (medium confidence — new Wotan topic + dashboard endpoint)
**ETA to Service Discovery MVP**: Feb 28 (medium confidence — new mechanism)

### The Sundial Tracks (Calendar — Schedule & Deadlines)

**Today (Feb 24)**: Round Table. Forge battle plan. Push S33 commits. Create port registry.
**This Week (Feb 25-28)**: Four-pillar implementation sprint.
**Protocol Deadlines**: None blocking. These four pillars are implementation architecture, not protocol spec.
**Schedule Conflicts**: None. Self-contained infrastructure work.
**Calendar Health**: HEALTHY. Momentum high. Context fresh from S33 Doom work.

### The Scroll Validates (Lore — Naming & Mythology)

**Naming Decisions Made**:
- Port range 16666-26666 → **"The Doom Range"** — named for the number of the beast that proved computational completeness.
- doom-bridge port 16666 → **THE NUMBER.** The service that bridges the Void to the browser gets the unholy port.
- Port registry → **"The Harbormaster's Ledger"** — the authoritative list of where every ship docks.
- Service discovery → **"The Cartographer's Eye"** — scans the Kingdom, maps what it finds, reports to Wotan.
- Log aggregation → **"The Chronicler's Well"** — where all stories flow, last 10,000 verses per bard, readable from the Great Hall (dashboard).
- gRPC-first transport → **"The King's Road"** — the primary path. HTTP is the merchant's trail (fallback).

**Mythology Consistency**: All four pillars map cleanly to the Medieval Armory:
- Port Authority → **Sabatons** (foundation, what the Kingdom stands on)
- Service Discovery → **Vambraces** (observability, seeing what's running)
- gRPC-First → **Hauberk** (chainmail, the primary communication layer)
- Log Aggregation → **Pauldrons** (shoulder armor, protecting by recording)

**Sacred Law Compliance**: All 8 laws honored. Zero customer data access in any new mechanism.

### The Map Confirms (Kingdom — Hierarchy & Placement)

**New Components This Sprint**:
- `/opt/unheaded/port-registry.yaml` → Layer 2 (Control Plane) — configuration artifact
- `pkg/discovery/` → Layer 3 (Infrastructure Services) — service discovery library
- `pkg/logagg/` → Layer 3 (Infrastructure Services) — log aggregation client
- Port registry validation in CI → Layer 0 (Infrastructure) — build-time check
- Dashboard log viewer → Layer 5 (UI) — user-facing log browsing

**Hierarchy Health**: Solid. New components slot cleanly into existing tiers.

### The War Table Thunders (Warmonger — Battle Planning)

**Sprint Plan Assessment**: S34 is a FOUR-PILLAR INFRASTRUCTURE SPRINT. Each pillar is independently valuable but they reinforce each other. Port migration is mechanical but wide-blast-radius. gRPC-first is architectural. Logging is visible. Discovery is forward-looking.

**Agent Matrix Recommendation**:
- **Agent A**: Port Migration — grep/replace across all 20 binaries, tests, configs. Widest blast radius.
- **Agent B**: gRPC-First Transport — flip default transport, add gRPC health checks. Deepest architectural change.
- **Agent C**: Log Aggregation — Wotan topic, ring buffer, dashboard endpoint. Most visible result.
- **Agent D**: Service Discovery — convention scanner, Wotan registration, deregistration. Most forward-looking.
- **Agent E**: Documentation — update CLAUDE.md, wiki, battle-plan, README with all four changes.

**Critical Path**: Port Registry (must exist first) → Port Migration → gRPC-First (needs correct ports) → Log Aggregation (needs gRPC transport) → Service Discovery (needs all of the above).

**Warmonger's Detailed Step Plan**: See `S34-INFRASTRUCTURE-BATTLE-PLAN.md` (to be forged after this round table).

### The Crucible Tests (Scientist — First-Principles Analysis)

**Port Range Analysis**:
- Range 16666-26666 = 10,001 ports. At current growth rate (20 services in 34 days), we could run 500 days before needing expansion assuming 20 ports/day. Practically infinite for our scale.
- The range is entirely within IANA "Registered Ports" (1024-49151). No conflict with ephemeral ports (49152-65535 on Linux by default, but configurable via `net.ipv4.ip_local_port_range`).
- Statistical collision probability: with 20 services in 10,001 ports, birthday problem gives P(collision) ≈ 0.02% — effectively zero. With manual allocation, P(collision) = 0%.

**gRPC vs HTTP Performance Hypothesis**:
- H4: gRPC persistent connections reduce connection overhead by 10-100x vs HTTP/1.1 per-request connections.
- H5: gRPC streaming for Wotan topics reduces latency by 5-50x vs HTTP polling (current implementation).
- H6: Dual health checking (gRPC + HTTP) catches transport-specific failures that single-protocol checks miss.

**Log Aggregation Sizing**:
- 10K lines × 20 services × ~500 bytes/line = 100MB in-memory ring buffer total. Trivial. Even 100K lines per service = 1GB. A single Wotan instance handles this easily.
- At 100 log lines/second across all services, the ring buffer turns over every ~33 minutes. Plenty of retention for debugging.

### The Dark Mirror Speaks (BlackMage — Offensive Security)

**New Attack Surface Assessment**:
- **Port scanning**: Attackers can enumerate all services by scanning 16666-26666. Mitigation: network policies (default deny), firewall rules restrict source IPs.
- **Service discovery poisoning**: If Wotan's `system.discovery.register` topic is unauthed, attackers can register fake services. Mitigation: WS5 authentication requirement.
- **Log injection**: If log messages aren't sanitized before Wotan publishing, attackers can inject false log entries. Mitigation: structured JSON only (zerolog prevents format string attacks), but content injection still possible.
- **gRPC reflection**: If gRPC reflection is enabled (default in many frameworks), attackers can enumerate all RPC methods. Mitigation: disable reflection in production.

**Recommendation**: All four pillars MUST have auth before public exposure (WS5). For alpha testing, network isolation is sufficient.

### The Ghost Materializes (Moat Ghost — Compliance & Audit)

**Compliance Impact of Four Pillars**:
- **Port Registry**: Excellent for SOC2 CM-* (configuration management). A versioned, auditable port allocation document.
- **Service Discovery**: Addresses NIST AC-* partially — services must identify themselves to the central authority.
- **gRPC-First**: TLS on gRPC (WS5) satisfies encryption-in-transit requirements. Current plaintext is acceptable for alpha.
- **Log Aggregation**: Addresses SOC2 AU-* (audit logging) and NIST AU-2/AU-3. Centralized, searchable, retained logs.

**Compliance Verdict**: These four pillars significantly improve audit posture. Port registry + log aggregation are direct evidence artifacts for SOC2 readiness.

### The Quill Speaks (RFC Editor — Protocol Documentation)

**Protocol Spec Impact**: These four pillars are implementation architecture, not wire format changes. However:
- Port allocation scheme should be documented as an **Informational Appendix** to the Wotan memory spec
- Service discovery protocol (register/deregister messages) should have a defined message format in Sophia dictionaries
- Log aggregation topic naming (`logs.<service>.<version>`) should follow Sophia naming conventions
- gRPC transport cascade should be referenced in the protocol foundation spec as the recommended implementation guidance

**No IANA implications.** No wire format changes. These are implementation decisions that make the protocol usable.

### The Barrister Rules (Barrister — Legal & IP — PRESIDING)

**IP Assessment of Four Pillars**:

1. **Port Allocation Scheme ("The Doom Range")**: The specific range choice (16666-26666) and the hierarchical allocation (infrastructure → control plane → Wotan → services → apps → gateway → customer) is a creative work protectable by copyright as a technical document. Not independently patentable but contributes to trade secret portfolio as part of Unheaded's overall infrastructure design.

2. **Service Discovery Architecture**: The three-layer approach (convention-based + port-scan + Wotan registration) is a novel combination. Individual elements are known (service registries, port scanning, message bus registration). The specific combination with the `/opt/unheaded/` convention, Wotan-as-registry, and fallback hierarchy is a trade secret worth protecting. Document in internal architecture specs, NOT in public RFCs.

3. **gRPC-First Transport Cascade**: The priority order (gRPC → HTTP/3 → HTTP/2 → HTTP/1.1) with dual health monitoring is standard industry practice. Not novel enough for IP protection independently but contributes to the overall platform design trade secret.

4. **Log Aggregation via Wotan Ring Buffer**: Using the Wotan ring buffer (which is also Protocol RAM for eBPF programs) as a log aggregation transport is novel — it unifies the logging plane with the data plane. This is a genuine innovation. The insight that "the same ring buffer that carries eBPF events can carry application logs" is worth documenting as a differentiator.

**Open Source Compliance**: No new dependencies introduced by these four pillars. All implementation in Go/Rust using existing libraries. No license compatibility concerns.

**Licensing Recommendation**: These four pillars should be covered by the same license as the rest of Unheaded (to be decided — see open question). The trade secret aspects (discovery architecture, Wotan-as-log-aggregator) should be documented in internal docs, not public specs.

**Contract Implications**: When licensing to customers, the port range 26000-26666 reserved for "customer apps" creates a clear boundary. Customer applications get their own port space. Unheaded infrastructure has its own. This architectural separation supports the "zero customer data access" principle at the network level — different port ranges, different firewall rules, different audit scopes.

### The Goblet Toasts (Busboy — Alignment & Coordination)

**Cross-Skill Conflicts**: NONE. All 17 skills aligned on the four-pillar approach.

**Coordination Needs**:
- Port migration touches ALL services → Developer owns, Architect reviews
- gRPC-first affects Wotan connection patterns → Developer + Architect co-own
- Log aggregation needs Wotan topic design → Architect owns, Developer implements
- Service discovery needs convention docs → Librarian owns, Architect designs
- All four need test updates → Developer + Micromanager co-own

**Team Vibes**: LOVE AND PEACE. The Barrister's first session brings a new voice to the table — legal clarity on IP and licensing. The four pillars are concrete, achievable, and directly unblock WS5. No philosophical debates. Just infrastructure. Just shipping. 🎵

---

### Unified Battle Plan

#### IMMEDIATE: Port Registry + S33 Push (Today — 2 hours) — ✅ COMPLETE

- [x] Push 6 unpushed S33 Doom commits — Owner: Developer
- [x] Create `/opt/unheaded/port-registry.yaml` — Owner: Architect
- [x] Create `pkg/config/ports.go` — Go constants for all port allocations — Owner: Developer
- [x] Add CI validation: no duplicate ports in registry — Owner: Developer
- [x] **EXIT GATE**: All S33 commits pushed. Port registry exists. `go build ./...` passes.

#### PILLAR 1: Port Migration (Feb 25 — 4-6 hours) — ✅ COMPLETE (S36)

- [x] Update timeguru: 8000 → 19000 (source + tests + nix + docker)
- [x] Update architect: 8001 → 19001
- [x] Update captain: 8002 → 19002
- [x] Update micromanager: 8003 → 19003
- [x] Update monad: 8004 → 19004
- [x] Update sophia: 8005 → 19005
- [x] Update wiki-server: 8007 → 20002
- [x] Update dashboard-backend: 8080 → 20000
- [x] Update kanban-app: 8081 → 20001
- [x] Update unheaded-daemon: HTTP 8080 → 17000, gRPC 9090 → 17001
- [x] Update doom-bridge: 6660 → 16666
- [x] Update trace-collector: HTTP 9092 → 16670, metrics 9100 → 16671
- [x] Update wotan: HTTP 9080 → 18000, gRPC 9090 → 18001
- [x] Update unheaded-cli: all hardcoded endpoints → registry lookup
- [x] Update CORS allowed origins (middleware.go)
- [x] Update Docker Compose configs
- [x] Update NixOS container definitions
- [x] Update gateway nginx config
- [x] Verify: every service has `--port` flag + `PORT` env var + config file override
- [x] **EXIT GATE**: `go test ./...` passes. `go build ./...` passes. Zero port conflicts. Every service has 3-level config override.

#### PILLAR 2: gRPC-First Transport (Feb 26 — 4-6 hours) — ✅ COMPLETE (S36)

- [x] Define standard transport initialization pattern in `pkg/transport/`
- [x] Pattern: try gRPC → fallback HTTP/3 → HTTP/2 → HTTP/1.1
- [x] Add `grpc.health.v1.Health` service to all gRPC-capable services
- [x] Update all services to connect to Wotan gRPC (18001) by default
- [x] HTTP fallback to Wotan HTTP (18000) when gRPC unavailable
- [x] Add `--transport` flag: "grpc" (default) | "http" | "auto"
- [x] Health monitoring: check both gRPC AND HTTP, report DEGRADED if gRPC-only failure
- [x] Update cross-service health checks to include gRPC probe
- [x] **EXIT GATE**: All services start with gRPC transport. HTTP fallback works when Wotan gRPC is down. Health checks report both protocols.

#### PILLAR 3: Aggregated Logging (Feb 27 — 4-6 hours) — ✅ COMPLETE (S36)

- [x] Create `pkg/logagg/publisher.go` — zerolog hook that publishes to Wotan
- [x] Define Wotan topics: `logs.<service>.<level>` (e.g., `logs.timeguru.info`)
- [x] Implement in-memory ring buffer in Wotan: 10K lines per service per version
- [x] Add dashboard API: `GET /api/v1/logs?service=X&lines=N&level=Y`
- [x] Add dashboard WebSocket: `ws://dashboard:20000/ws/logs` for live tail
- [x] Create dashboard UI: service selector, level filter, search, auto-scroll
- [x] Wire zerolog hook into all 20 services (one-line integration)
- [x] **EXIT GATE**: Logs from any service visible in dashboard within 5 seconds. Search works. Live tail works. 10K line retention per service.

#### PILLAR 4: Service Discovery (Feb 28 — 4-6 hours) — ✅ COMPLETE (S36)

- [x] Create `/opt/unheaded/` directory convention documentation
- [x] Implement `pkg/discovery/scanner.go` — scans `/opt/unheaded/*/config.yaml`
- [x] Implement `pkg/discovery/registrar.go` — Wotan register/deregister
- [x] Define Wotan topics: `system.discovery.register`, `system.discovery.deregister`, `system.discovery.report`
- [x] Define discovery message format (Sophia dictionary entry)
- [x] Add startup registration to all services (one-line integration)
- [x] Add shutdown deregistration via graceful shutdown hooks
- [x] Create static fallback: `/opt/unheaded/services.yaml`
- [x] Replace all hardcoded IPs in unheaded-cli with discovery lookups
- [x] **EXIT GATE**: Services register on startup. Deregister on shutdown. CLI uses discovery. Fallback works when Wotan is down.

#### Documentation Sprint (Parallel with all pillars) — ✅ COMPLETE (S36)

- [x] Update CLAUDE.md: new port ranges, gRPC-first policy, log aggregation, service discovery
- [x] Update wiki Home.md: add four-pillar architecture section
- [x] Update wiki Architecture.md: port registry, transport cascade, log flow
- [x] Update wiki Service-*.md pages: new ports for each service
- [x] Update wiki _Sidebar.md: add Port Registry, Service Discovery, Logging pages
- [x] Create wiki Port-Registry.md: the full Doom Range allocation table
- [x] Create wiki Service-Discovery.md: three-layer architecture
- [x] Create wiki Log-Aggregation.md: Chronicler's Well architecture
- [x] Create wiki Transport-Cascade.md: gRPC-first policy
- [x] Update README.md: new quick-start ports
- [x] **EXIT GATE**: All 8+ document layers updated. Wiki _Sidebar links work. No stale port references.

#### Decisions Made at This Round Table

1. **Port range 16666-26666 ("The Doom Range")** — All Unheaded services migrate to high ports. Rationale: Eliminates conflicts with common dev tools. Doom-themed for lore. Hierarchical allocation by tier. 10K ports = effectively infinite headroom.
2. **Two confirmed port conflicts resolved** — dashboard-backend and unheaded-daemon both at 8080; unheaded-daemon gRPC and wotan gRPC both at 9090. Rationale: Can't run the platform if services fight over ports.
3. **gRPC-first, HTTP fallback** — All services default to Wotan gRPC. HTTP/3→2→1 cascade for fallback. Rationale: Wotan IS the nervous system. The primary transport should use the bus, not bypass it.
4. **Every service gets 3-level config override** — CLI flag + env var + config file for ALL settings (port, transport, Wotan address, etc.). Rationale: Operators need flexibility. Hardcoded defaults are for developers.
5. **Log aggregation via Wotan ring buffer** — 10K lines per service in test phase. Dashboard-viewable. Rationale: Logs that stay on the container are invisible. Invisible logs = invisible bugs.
6. **Convention-based service discovery** — `/opt/unheaded/<service>/` install path + Wotan registration. Rationale: We own the infra. Use the conventions we control.
7. **Barrister's First Ruling: IP protection for discovery architecture** — The three-layer discovery approach is a trade secret. Document internally, not in public specs. Rationale: Novel combination worth protecting.
8. **Love and peace** — From Muck. Acknowledged by all 17 seats. The Kingdom vibes are strong.

#### Open Questions (Carry to Next Round Table)

1. ~~**License choice for Unheaded**~~ — **RESOLVED S35**: BSL 1.1 short-term (while finishing codebase), converting to permissive (MIT/Apache/GNU) at stable release or K8s-scale adoption. Protocol specs licensed separately (permissive) for free implementation. Barrister session still needed to draft LICENSE file.
2. **Port 16666 for doom-bridge vs keeping 6660** — Lore says 16666. Practicality says 6660 is already memorable. Round Table says 16666 for consistency.
3. **HTTP/3 support timeline** — gRPC doesn't natively support HTTP/3 (QUIC). The cascade HTTP/3→2→1 applies to the HTTP fallback path, not gRPC. When do we add QUIC to the HTTP fallback? WS5 or later?
4. **Log retention policy for production** — 10K lines in test phase. What for production? Configurable per observability backend? The Moat Ghost needs a compliance-driven answer.
5. **Service discovery for multi-host** — Current design assumes single host (all services on one machine). Multi-host discovery needs DNS or BGP-style advertisement. Age 2+?
6. **NEW — Inverse mask concept** — Explore, expand, and document. Requires BlackMage + Developer + Architect + Scientist session. Added S35.
7. **NEW — Austin VC exploration** — While repo is still private, investigate Austin-area venture capital. Captain + Barrister to research.

#### Wins to Celebrate

- **~260K PRODUCTION LOC (~464K WITH TESTS)** — Corrected from inflated wc -l counts. 220K Go + 16K Rust + 13K JS + 5K Nix + 7K scripts. 93% test-to-production ratio.
- **DOOM IS PLAYABLE** — Keyboard input works. LH/LB sign-extension fixed. 30 FPS WebSocket streaming.
- **17 SKILLS AT THE TABLE** — The most complete assembly yet. The Barrister joins the Kingdom.
- **ZERO PORT CONFLICTS AFTER TODAY** — Two confirmed conflicts identified AND a migration plan forged to prevent all future conflicts.
- **FOUR PILLARS FORGED AND STANDING (S36)** — Port Authority, gRPC-First, Log Aggregation, Service Discovery. Planned in S34, executed in S36. All EXIT GATES passed.
- **S36 COMPLETE — THE FOUR PILLARS STAND** — All services on Doom Range ports, gRPC-first transport wired, log aggregation via Wotan ring buffer, three-layer service discovery operational.
- **LOVE AND PEACE** — From Muck. The vibes are immaculate.

---

## S35 Strategic Review — Muck's Directives (February 24, 2026)

### Decisions Made

**1. Licensing Direction — BSL 1.1 Short-Term, Permissive Long-Term**
- BSL 1.1 with short conversion period while codebase is being polished toward stable release
- Goal: credit for novel ideas (protocol covers that via RFC), BSL protects product commercially
- Aspiration: if Unheaded reaches Kubernetes-scale adoption, convert to fully permissive (MIT/Apache/GNU)
- Protocol specs (Monad, Sophia, Wotan) licensed separately under permissive license for free implementation
- Started as a lab/portfolio project for GitHub — commercial viability is a bonus, not the origin

**2. Doom Fork — Official id Software Source**
- Replace doomgeneric fork with official https://github.com/id-Software/DOOM
- Running real id DOOM with sound over Unheaded Protocol is the pitch, not stripped-down generic
- Already have https://github.com/unheaded/doomgeneric — will become https://github.com/unheaded/DOOM
- Doom code MUST be moved out of main repo before switching from private to public
- GPL-2.0 isolation boundary maintained: submodule, compiled to MBC bytecode, no linking

**3. SBOM Scanning — Tonight**
- ScanCode, FOSSology, and ORT all downloaded to ~/tmp/
- Run all 3 against codebase, output findings to ~/tmp/
- Review results, fold into main ~/tmp/unheaded/ repo
- Must complete before accepting outside contributors or going public

**4. Observability & IaC Backends — DEFER, DO NOT KILL**
- All backend adapters (Prometheus/Grafana/ELK/Jaeger/Nagios/Flume/Loki/Alertmanager) stay in roadmap
- All IaC renderers (Ansible/Terraform/Puppet/K8s/Chef/Salt) stay in roadmap
- Anti-proprietary lock-in is a core principle — scaffolding for all backends drives adoption
- Priority: ship Prometheus + zerolog first (they work now), scaffold the rest iteratively
- This is a DEFER not a KILL — no scope reduction, just sequencing

**5. Inverse Mask Concept — Deep Exploration Required**
- Call BlackMage, Developer, Architect, and Scientist for dedicated session
- Explore, expand, and formally document the inverse mask concept
- Potential protocol-level innovation worth protecting

**6. Austin VC Exploration**
- While repo is still private, investigate Austin-area venture capital landscape
- "Doom-over-IPv6 proves computational completeness" is the pitch
- Protocol IS the moat — BSL the product, open the specs, let protocol adoption drive product interest
- Captain + Barrister to research firms, term sheet readiness, pitch deck prep

**7. Timeline Honesty Audit**
- timeline.md has milestones marked "completed" with 55-85% progress and unchecked subtasks
- Age 4 "Scaling" marked COMPLETED at 5% — impossible, must be fixed
- All milestone statuses must accurately reflect reality before external eyes see them
- Investors and partners will look at this — honesty > hype

### Strategic Direction Confirmed

**The Protocol IS the Moat.**
- BSL the product implementation
- Open the protocol specs (permissive license)
- Get IANA registration for HbH option type
- Let protocol adoption drive product interest
- "Doom-over-IPv6 proves computational completeness" = IETF hackathon pitch

**Priority Order:**
1. Execute S34 four pillars (port migration, gRPC-first, logging, discovery)
2. License decision — draft LICENSE file (Barrister session)
3. Run SBOM scanners (ScanCode + FOSSology + ORT)
4. eBPF on bare metal — THE core differentiator
5. Inverse mask deep dive (BlackMage + Developer + Architect + Scientist)
6. IANA registration prep (RFC Editor)
7. Austin VC exploration (Captain + Barrister)
8. 5-minute demo video (Doom over IPv6 with packet tracing)

---

### Next Round Table
**Scheduled**: March 1, 2026 (post-implementation review of four pillars)
**Reason**: Verify all four pillars are operational. Review WS5 readiness. Begin WS5 planning.
**Trigger**: Also convene if port migration breaks more than 3 tests OR if gRPC transport causes service startup failures.
**Additional Agenda**: License file draft review. SBOM scan results. Inverse mask session scheduling.

---

_Forged at the Round Table by all 17 minds — the full Royal Court assembled._
_Captain, Architect, Micromanager, Developer, Timeguru, Calendar, Lore, Kingdom, Busboy,_
_Warmonger, Scientist, BlackMage, Moat Ghost, RFC Editor, Barrister (presiding), Librarian, and the Round Table itself._
_35 sessions from first commit. ~260K production lines of code (~464K with tests). Doom is playable. Four pillars forged._
_The Doom Range: 16666-26666. The King's Road: gRPC-first. The Chronicler's Well: aggregated logs._
_The Cartographer's Eye: service discovery. The Protocol IS the Moat. Love and peace._
_THE KINGDOM MARCHES AS ONE. LET'S GO._
