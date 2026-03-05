# S41 KINGDOM HARDENING BATTLE PLAN — 12 Phases, 210+ Steps

**Date**: 2026-02-24
**Sprint**: S41 — Dashboard Resurrection, Protocol Audit, Binary Lore, Binder Book, Ecosystem Validation
**Prerequisite**: S40 complete (6/6 known limitations fixed, 31K+ flows through pipeline, all tests pass)
**Target**: Production-quality dashboard displaying all eBPF data streams, all binaries named per Kingdom lore, full codebase protocol audit verified, binder book scaffolded at 4 comprehension levels, comparison research validated
**Estimated Duration**: 12-18 hours across 3-4 sessions
**Agent Strategy**: Phases 0-4 sequential (dashboard critical path), Phases 5-7 parallelizable, Phases 8-11 sequential
**Commit Cadence**: Every 5 steps (calculated: max(3, min(5, 210/20)))
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

---

## ROUND TABLE SITUATION REPORT

### Kingdom State: Age 1 → Age 2 Transition (~99.5% Alpha)

The Kingdom stands at the threshold. 40 sessions. 440K+ LOC Go. 24K LOC Rust eBPF. 25+ services. 4 eBPF programs compiled and loading. Full kernel-to-browser pipeline verified in demo mode with 31K+ flows. 25 commits ahead of origin, all pushed.

**What's working**: packet_marker (XDP), flow_tracker (TC), latency_probe (kprobe) → trace-collector-go → Wotan:18001 (gRPC) → dashboard-backend:20000 → browser. The pipeline EXISTS. Data FLOWS.

**What's broken/ugly**: The dashboard is a barely-functioning skeleton. The ingestor only processes topics containing "packet", "flow", "latency", or "syscall" — but demo mode generates `compute.*` and `anamnesis.*` topics that don't match. The UI needs heavy scaffolding to actually display all the data we're collecting. The advanced visualizations in `dashboard/` (doom.html, latency-chart.js, packet-flow-diagram.js) aren't served by the backend. It works but it's ugly.

### The Throne (Captain)
**Strategic Position**: We have the deepest eBPF data pipeline in the ecosystem (confirmed by 4 comparison analyses: Flink, K8s/Cilium/Flannel, Coroot, full eBPF ecosystem). Our wire-level depth is unbridgeable. But our insight extraction and UI are behind Coroot/Hubble. Dashboard resurrection is P0.

### The Ledger (Micromanager)
**Priority Stack**:
- P0: Dashboard resurrection — topic routing fix, UI scaffolding, serve advanced visualizations
- P0: Codebase protocol audit — verify all BPF tooling correctly uses Unheaded protocol
- P1: Binary naming per Kingdom lore — all compiled binaries get lore names, documented
- P1: Binder book scaffolding — ELI5/middle school/high school/PhD preamble to RFC
- P2: Ecosystem comparison validation — verify topics from uploaded comparison docs are accounted for

### The Blueprint (Architect)
**Architecture Health**: Pipeline sound. Dashboard ingestor has a routing gap (compute.*/anamnesis.* topics dropped). Advanced dashboard assets orphaned in `dashboard/` not served by `cmd/dashboard-backend`. Fix: expand topic dispatch, serve `dashboard/` assets, wire up WebSocket live streaming.

### The Anvil (Developer)
**Code Health**: All tests pass (race-clean). Build passes. Known gap: `dispatch()` in `cmd/dashboard-backend/internal/ebpf/ingestor.go` only handles 4 topic patterns. Needs expansion for compute.*, anamnesis.*, and any future topic prefixes.

### The Hourglass (Timeguru)
**Velocity**: S36-S40 averaged ~60 steps/hour with agent execution. 210 steps ≈ 3.5 hours pure execution + 2 hours debug/iteration.

### The Scroll (Lore)
**Naming Decisions Pending**: All 14+ compiled binaries need lore-appropriate names. Current names are functional (dashboard-backend, trace-collector-go, wotan-ctl). Kingdom convention: binaries should carry names from the Medieval Armory, Gnostic Cosmology, or Chronicles of Amber pillars.

### The Map (Kingdom)
**Hierarchy Health**: Stable. Dashboard components need proper placement in Crest (frontend) / Visor (backend) tier.

### The Goblet (Busboy)
**Vibes**: HIGH ENERGY. S40 proved the pipeline works. Now we polish it until it SHINES. The comparison docs show we have the deepest data source in the eBPF ecosystem — time to build the insight extraction that matches.

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

## PHASE 0: ENVIRONMENT VERIFICATION & BASELINE (Steps 1-15)

**Goal**: Verify dev environment, confirm S40 state, establish baseline.
**Prerequisite**: S40 handoff doc exists, all services buildable.
**Time**: 15 minutes
**Agent**: Coordinator

- [ ] **Step 1** [R] ~30s: Read S40 handoff doc for current state
  ```bash
  cat docs/sessions/S40-known-limitations-handoff.md | head -50
  ```

- [ ] **Step 2** [B] ~30s: Check Go version and toolchain
  ```bash
  go version && go env GOPATH && which go
  ```

- [ ] **Step 3** [V] ~30s: Go >= 1.21 required
  - If pass → Step 4
  - If fail → STOP, upgrade Go

- [ ] **Step 4** [B] ~2m: Build all services from root
  ```bash
  cd ~/tmp/unheaded && go build ./... 2>&1 | tail -20
  ```

- [ ] **Step 5** [V] ~30s: Zero build errors
  - If pass → Step 6
  - If fail → Step 5a [D]

- [ ] **Step 5a** [D] ~3m: Check for missing dependencies
  ```bash
  go mod tidy && go build ./... 2>&1 | head -30
  ```

- [ ] **Step 6** [B] ~2m: Run tests for dashboard-backend
  ```bash
  cd ~/tmp/unheaded && go test -race -count=1 ./cmd/dashboard-backend/... 2>&1 | tail -20
  ```

- [ ] **Step 7** [B] ~2m: Run tests for trace-collector-go
  ```bash
  cd ~/tmp/unheaded && go test -race -count=1 ./cmd/trace-collector-go/... 2>&1 | tail -20
  ```

- [ ] **Step 8** [V] ~30s: All tests pass
  - If pass → Step 9
  - If fail → Step 8a [D]

- [ ] **Step 8a** [D] ~3m: Identify failing tests, check if pre-existing
  ```bash
  go test -race -count=1 -v ./cmd/dashboard-backend/... 2>&1 | grep -E "FAIL|PASS" | tail -20
  ```

- [ ] **Step 9** [B] ~30s: Check current git status
  ```bash
  git status && git log --oneline -5
  ```

- [ ] **Step 10** [R] ~1m: Read dashboard ingestor to understand current dispatch
  ```bash
  cat cmd/dashboard-backend/internal/ebpf/ingestor.go | head -80
  ```

- [ ] **Step 11** [R] ~1m: Read dashboard frontend files
  ```bash
  ls -la cmd/dashboard-backend/static/ && ls -la dashboard/ && cat cmd/dashboard-backend/static/index.html | head -40
  ```

- [ ] **Step 12** [R] ~1m: Check what topics trace-collector publishes
  ```bash
  grep -rn "topic\|Topic\|publish\|Publish" cmd/trace-collector-go/main.go | head -30
  ```

- [ ] **Step 13** [R] ~1m: Check what topics dashboard subscribes to
  ```bash
  grep -rn "subscribe\|Subscribe\|topic\|Topic" cmd/dashboard-backend/internal/ | head -30
  ```

- [ ] **Step 14** [C] ~30s: **COMMIT CHECKPOINT** — baseline verified
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 1-14: Environment verified, baseline established"
  ```

- [ ] **Step 15** [V]: **PHASE 0 EXIT GATE** — Build passes, tests pass, current state understood
  - Build: zero errors
  - Tests: all pass (or pre-existing failures documented)
  - Dashboard ingestor dispatch logic: READ and understood
  - Topic mismatch: IDENTIFIED
  - If pass → Phase 1
  - If fail → DO NOT PROCEED

---

## PHASE 1: DASHBOARD TOPIC ROUTING FIX (Steps 16-35)

**Goal**: Fix the ingestor to handle ALL topic types: ebpf.*, compute.*, anamnesis.*, and any future prefix.
**Prerequisite**: Phase 0 complete
**Time**: 30 minutes
**Agent**: Agent

- [ ] **Step 16** [R] ~1m: Read full ingestor dispatch logic
  ```bash
  cat cmd/dashboard-backend/internal/ebpf/ingestor.go
  ```

- [ ] **Step 17** [R] ~1m: Read ingestor test file
  ```bash
  cat cmd/dashboard-backend/internal/ebpf/ebpf_test.go
  ```

- [ ] **Step 18** [W] ~5m: Fix `dispatch()` / `dispatchSingle()` to route ALL topic types
  - Add routing for `compute.*` topics → map to appropriate internal event types
  - Add routing for `anamnesis.*` topics → convert to FlowEvent/PacketEvent format
  - Add catch-all for unknown topics → log and record in stats (don't silently drop)
  - Ensure `anamnesisToFlowJSON()` bridge in trace-collector covers all event types

- [ ] **Step 19** [V] ~30s: Verify dispatch handles all known topic prefixes
  ```bash
  grep -n "case\|switch\|contains\|HasPrefix" cmd/dashboard-backend/internal/ebpf/ingestor.go | head -20
  ```

- [ ] **Step 20** [W] ~5m: Add tests for new topic routing
  - Test: compute.hop dispatches correctly
  - Test: compute.miss dispatches correctly
  - Test: compute.halt dispatches correctly
  - Test: compute.syscall dispatches correctly
  - Test: anamnesis.flow dispatches correctly
  - Test: unknown.topic logs warning, doesn't crash

- [ ] **Step 21** [B] ~1m: Run ingestor tests
  ```bash
  go test -race -count=1 -v ./cmd/dashboard-backend/internal/ebpf/... 2>&1 | tail -30
  ```

- [ ] **Step 22** [V] ~30s: All ingestor tests pass
  - If pass → Step 23
  - If fail → Step 22a [D]

- [ ] **Step 22a** [D] ~3m: Debug failing tests — check event type mapping
  ```bash
  go test -race -count=1 -v -run "Test.*Dispatch\|Test.*Batch\|Test.*Topic" ./cmd/dashboard-backend/internal/ebpf/... 2>&1
  ```

- [ ] **Step 23** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 16-23: Dashboard topic routing expanded to all event types"
  ```

- [ ] **Step 24** [W] ~5m: Add event type definitions for compute.* events
  - Define `ComputeEvent` struct if not exists (hop, miss, halt, syscall)
  - Ensure JSON serialization matches what trace-collector publishes
  - Add to `/api/v1/ebpf/stats` endpoint

- [ ] **Step 25** [W] ~3m: Update stats tracking to include per-topic counters
  - `compute_events_ingested`, `anamnesis_events_ingested`, `unknown_topics_seen`
  - Expose in `/api/v1/ebpf/stats`

- [ ] **Step 26** [B] ~1m: Run full dashboard-backend tests
  ```bash
  go test -race -count=1 ./cmd/dashboard-backend/... 2>&1 | tail -20
  ```

- [ ] **Step 27** [V] ~30s: All dashboard tests pass
  - If pass → Step 28
  - If fail → Step 27a [D]

- [ ] **Step 27a** [D] ~3m: Isolate failing test
  ```bash
  go test -race -count=1 -v -run "FAIL" ./cmd/dashboard-backend/... 2>&1
  ```

- [ ] **Step 28** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 24-28: Per-topic stats and compute event types added"
  ```

- [ ] **Step 29** [W] ~5m: Ensure trace-collector's `anamnesisToFlowJSON()` covers all Anamnesis event subtypes
  - Check: does it bridge compute.hop → FlowEvent? compute.miss → FlowEvent?
  - If not, expand the bridge to convert ALL Anamnesis event types to dashboard-compatible formats
  - Dual-publish: original topic (anamnesis.*) AND converted topic (ebpf.flow.events)

- [ ] **Step 30** [B] ~1m: Run trace-collector tests
  ```bash
  go test -race -count=1 ./cmd/trace-collector-go/... 2>&1 | tail -20
  ```

- [ ] **Step 31** [V] ~30s: All trace-collector tests pass
  - If pass → Step 32
  - If fail → Step 31a [D]

- [ ] **Step 31a** [D] ~3m: Debug failing tests
  ```bash
  go test -race -count=1 -v ./cmd/trace-collector-go/... 2>&1
  ```

- [ ] **Step 32** [B] ~2m: Rebuild dashboard-backend and trace-collector binaries
  ```bash
  go build -o bin/dashboard-backend ./cmd/dashboard-backend/ && go build -o bin/trace-collector-go ./cmd/trace-collector-go/
  ```

- [ ] **Step 33** [V] ~30s: Both binaries exist and are fresh
  ```bash
  ls -la bin/dashboard-backend bin/trace-collector-go
  ```

- [ ] **Step 34** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 29-34: Full topic bridge and binary rebuild"
  ```

- [ ] **Step 35** [V]: **PHASE 1 EXIT GATE** — All topic types routed, tests pass, binaries fresh
  - Ingestor handles: ebpf.*, compute.*, anamnesis.*, unknown
  - Stats endpoint shows per-topic counters
  - All tests pass
  - If pass → Phase 2
  - If fail → DO NOT PROCEED

---

## PHASE 2: DASHBOARD UI/UX RESURRECTION (Steps 36-70)

**Goal**: Transform the dashboard from skeleton to functional data display. Serve advanced visualizations. Wire WebSocket.
**Prerequisite**: Phase 1 complete (all topics routed)
**Time**: 60-90 minutes
**Agent**: Coordinator

- [ ] **Step 36** [R] ~2m: Audit ALL existing dashboard assets
  ```bash
  find dashboard/ -type f | sort && echo "---" && find cmd/dashboard-backend/static/ -type f | sort
  ```

- [ ] **Step 37** [R] ~2m: Read the advanced dashboard files
  ```bash
  cat dashboard/js/latency-chart.js | head -40 && echo "---" && cat dashboard/js/packet-flow-diagram.js | head -40
  ```

- [ ] **Step 38** [R] ~1m: Check if dashboard/doom.html is the Doom visualization
  ```bash
  cat dashboard/doom.html | head -30
  ```

- [ ] **Step 39** [R] ~1m: Read current embedded static/index.html
  ```bash
  cat cmd/dashboard-backend/static/index.html
  ```

- [ ] **Step 40** [R] ~1m: Read current dashboard.js
  ```bash
  cat cmd/dashboard-backend/static/dashboard.js
  ```

- [ ] **Step 41** [W] ~10m: Redesign `static/index.html` — proper scaffold with:
  - Header: Kingdom branding, service status indicators
  - Nav: Flows | Packets | Latency | Compute | Doom | Logs
  - Main grid: Flow table (sortable), live counter cards, mini charts
  - Status bar: events/sec, total flows, parse errors, connected topics
  - Dark theme (we're infrastructure engineers, not accountants)
  - Pull in CSS from `dashboard/css/` or inline it
  - Reference `dashboard.js` for logic

- [ ] **Step 42** [W] ~10m: Redesign `static/dashboard.js` — functional data display:
  - WebSocket connection to `/ws` for live streaming
  - Fetch `/api/v1/flows` on load, populate flow table
  - Fetch `/api/v1/ebpf/stats` every 5s, update counter cards
  - Fetch `/api/v1/traces` every 5s, update recent traces panel
  - Auto-refresh with configurable interval
  - Search/filter by IP, port, protocol
  - Sort columns ascending/descending

- [ ] **Step 43** [W] ~5m: Update `static/styles.css` — dark theme, grid layout, responsive

- [ ] **Step 44** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 36-44: Dashboard UI redesigned with proper scaffold"
  ```

- [ ] **Step 45** [W] ~5m: Serve `dashboard/` advanced visualizations from backend
  - Add route: `/viz/doom` → serve `dashboard/doom.html`
  - Add route: `/viz/latency` → serve latency chart
  - Add route: `/viz/packets` → serve packet flow diagram
  - Add route: `/viz/logs` → serve `dashboard/logs.html`
  - Mount `dashboard/` directory assets under `/viz/` prefix

- [ ] **Step 46** [B] ~1m: Verify routes compile
  ```bash
  go build ./cmd/dashboard-backend/ 2>&1 | tail -10
  ```

- [ ] **Step 47** [V] ~30s: Build passes
  - If pass → Step 48
  - If fail → Step 47a [D]

- [ ] **Step 47a** [D] ~3m: Fix import/route issues
  ```bash
  go build -v ./cmd/dashboard-backend/ 2>&1
  ```

- [ ] **Step 48** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 45-48: Advanced visualizations served under /viz/"
  ```

- [ ] **Step 49** [W] ~5m: Verify WebSocket endpoint exists and works
  - Check `cmd/dashboard-backend/internal/events/events.go` for WS handler
  - Ensure it streams ALL topic events to connected clients
  - If missing: implement basic WS handler using gorilla/websocket or nhooyr.io/websocket

- [ ] **Step 50** [W] ~5m: Add flow table API improvements
  - `/api/v1/flows?sort=bytes&order=desc&limit=100` — query params
  - `/api/v1/flows?filter=src_ip:10.0.0.1` — basic filtering
  - Return JSON with total count, page info

- [ ] **Step 51** [W] ~5m: Add traces API improvements
  - `/api/v1/traces?limit=50&protocol=TCP` — filtering
  - Include 5-tuple in response

- [ ] **Step 52** [B] ~1m: Run tests
  ```bash
  go test -race -count=1 ./cmd/dashboard-backend/... 2>&1 | tail -20
  ```

- [ ] **Step 53** [V] ~30s: Tests pass
  - If pass → Step 54
  - If fail → Step 53a [D]

- [ ] **Step 53a** [D] ~3m: Debug
  ```bash
  go test -race -count=1 -v ./cmd/dashboard-backend/... 2>&1 | grep "FAIL"
  ```

- [ ] **Step 54** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 49-54: WebSocket + API improvements"
  ```

- [ ] **Step 55** [W] ~10m: Create dashboard status page component
  - Shows: all connected services, their ports, health status
  - Real-time: events/sec graph (simple SVG or canvas)
  - Topic subscriptions: list all active topics with message rates
  - BPF program status: loaded/attached/detached per program

- [ ] **Step 56** [W] ~5m: Create flow visualization component
  - Sankey or chord diagram showing top-N flow paths
  - Color by protocol (TCP=blue, UDP=green, ICMP=yellow)
  - Size by byte count

- [ ] **Step 57** [W] ~5m: Create latency display component
  - Histogram of RTT values from correlator
  - P50/P90/P99 latency cards
  - Per-destination latency breakdown

- [ ] **Step 58** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 55-58: Status, flow viz, and latency components"
  ```

- [ ] **Step 59** [W] ~5m: Add nav links from main dashboard to advanced viz pages
  - Doom visualization: `/viz/doom`
  - Full latency chart: `/viz/latency`
  - Packet flow diagram: `/viz/packets`
  - Log viewer: `/viz/logs`

- [ ] **Step 60** [B] ~2m: Full rebuild and basic smoke test
  ```bash
  go build -o bin/dashboard-backend ./cmd/dashboard-backend/ && echo "Build OK"
  ```

- [ ] **Step 61** [V] ~30s: Binary builds clean
  - If pass → Step 62
  - If fail → Step 61a [D]

- [ ] **Step 61a** [D] ~3m: Fix build issues
  ```bash
  go build -v ./cmd/dashboard-backend/ 2>&1
  ```

- [ ] **Step 62** [B] ~2m: Run full test suite
  ```bash
  go test -race -count=1 ./cmd/dashboard-backend/... 2>&1 | tail -20
  ```

- [ ] **Step 63** [V] ~30s: All tests pass
  - If pass → Step 64
  - If fail → Step 63a [D]

- [ ] **Step 63a** [D] ~3m: Debug
  ```bash
  go test -race -count=1 -v ./cmd/dashboard-backend/... 2>&1
  ```

- [ ] **Step 64** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 59-64: Dashboard UI complete with nav and visualizations"
  ```

- [ ] **Step 65** [R] ~2m: Compare our dashboard to what Coroot/Hubble offer (from comparison docs)
  - Do we show service maps? (Coroot has this → "Cartographer's View")
  - Do we show SLO tracking? (Coroot has this → "Oath Tracker")
  - Do we show predefined inspections? (Coroot has this → "Edicts Engine")
  - Document gaps for future sprints

- [ ] **Step 66** [W] ~3m: Create `docs/dashboard-gap-analysis.md`
  - What we have now vs what Coroot/Hubble have
  - Prioritized feature list for future dashboard sprints
  - Reference the 4 comparison battle plans

- [ ] **Step 67** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 65-67: Dashboard gap analysis documented"
  ```

- [ ] **Step 68** [V]: **PHASE 2 EXIT GATE** — Dashboard displays all data streams, advanced viz accessible
  - Main dashboard: flow table, stats cards, recent traces
  - WebSocket: live streaming functional
  - Advanced viz: `/viz/doom`, `/viz/latency`, `/viz/packets`, `/viz/logs` all serve
  - All tests pass
  - If pass → Phase 3
  - If fail → DO NOT PROCEED

---

## PHASE 3: CODEBASE PROTOCOL AUDIT (Steps 69-100)

**Goal**: Verify ALL BPF tooling correctly uses the Unheaded protocol (Monad wire format, Sophia maps, Wotan persistence).
**Prerequisite**: Phase 2 complete
**Time**: 45-60 minutes
**Agent**: Agent

- [ ] **Step 69** [R] ~2m: Identify all BPF-related Go packages
  ```bash
  find . -name "*.go" | xargs grep -l "ebpf\|bpf\|BPF\|xdp\|XDP" | sort -u | head -40
  ```

- [ ] **Step 70** [R] ~2m: Read the BPF loader interface
  ```bash
  cat pkg/ebpf/loader.go | head -100
  ```

- [ ] **Step 71** [R] ~2m: Read the Rust eBPF programs list
  ```bash
  find ebpf/ -name "*.rs" -path "*/src/*" | sort && find ebpf/ -name "Cargo.toml" | sort
  ```

- [ ] **Step 72** [B] ~2m: Verify Monad register file format in BPF programs matches Go types
  ```bash
  grep -rn "TraceEntry\|trace_entry\|TRACE_MAP\|trace_map" ebpf/ cmd/trace-collector-go/types.go | head -30
  ```

- [ ] **Step 73** [V] ~1m: TraceEntry struct (Go) matches BPF map value layout (Rust)
  - 16 bytes TraceID + 8 bytes timestamp + 16+16 bytes IPs + 2+2 ports + 1 proto + 1 flags + 2 pktlen + 1 hop + 3 pad = 68 bytes
  - If match → Step 74
  - If mismatch → STOP, fix alignment

- [ ] **Step 74** [B] ~2m: Verify Sophia map definitions match between Rust and Go
  ```bash
  grep -rn "MAP_TYPE\|map_type\|BPF_MAP_TYPE" ebpf/ pkg/ebpf/ | head -30
  ```

- [ ] **Step 75** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 69-75: Protocol audit — wire format alignment verified"
  ```

- [ ] **Step 76** [R] ~2m: Check all map names for consistency
  ```bash
  grep -rn "TRACE_MAP\|STATS\|FLOW_STATE\|LATENCY_MAP\|PACKET_EVENTS" ebpf/ cmd/trace-collector-go/ pkg/ebpf/ | head -40
  ```

- [ ] **Step 77** [R] ~2m: Check FlowStateEntry format
  ```bash
  grep -A 20 "FlowStateEntry\|FlowKey\|flow_state" cmd/trace-collector-go/flow_reader.go | head -40
  ```

- [ ] **Step 78** [R] ~2m: Check LatencyEntry format
  ```bash
  grep -A 20 "LatencyEntry\|latency_entry\|LATENCY" cmd/trace-collector-go/latency_reader.go | head -40
  ```

- [ ] **Step 79** [V] ~1m: All map entry formats consistent between Rust eBPF and Go consumer
  - If consistent → Step 80
  - If inconsistent → document mismatches, fix in subsequent steps

- [ ] **Step 80** [B] ~2m: Verify CRC-16 implementation matches protocol spec
  ```bash
  grep -rn "crc\|CRC\|checksum\|Checksum" pkg/ cmd/ | head -20
  ```

- [ ] **Step 81** [B] ~2m: Verify exponent encoding implementation
  ```bash
  grep -rn "exponent\|Exponent\|exp_encode\|ExpEncode" pkg/ cmd/ | head -20
  ```

- [ ] **Step 82** [R] ~2m: Check Anamnesis event format
  ```bash
  grep -A 30 "AnamnesisEvent\|anamnesis_event" pkg/ebpf/ cmd/trace-collector-go/ | head -50
  ```

- [ ] **Step 83** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 76-83: Protocol audit — map formats, CRC, exponent encoding verified"
  ```

- [ ] **Step 84** [B] ~3m: Search for any hardcoded values that should reference protocol constants
  ```bash
  grep -rn "16666\|18000\|18001\|20000\|16670" cmd/ pkg/ --include="*.go" | grep -v "_test.go\|vendor" | head -30
  ```

- [ ] **Step 85** [V] ~1m: All port references use `pkg/ports/` or `configs/port-registry.yaml`
  - If yes → Step 86
  - If hardcoded found → document for fix

- [ ] **Step 86** [B] ~2m: Verify Shield WAF integration with Monad protocol
  ```bash
  grep -rn "shield\|Shield\|WAF\|waf" pkg/ebpf/ cmd/ | head -20
  ```

- [ ] **Step 87** [B] ~2m: Verify Wotan persistence layer handles all Sophia map types
  ```bash
  grep -rn "PersistMap\|SaveMap\|LoadMap\|RestoreMap\|snapshot" services/wotan/ cmd/wotan-ctl/ | head -20
  ```

- [ ] **Step 88** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 84-88: Protocol audit — ports, Shield, Wotan persistence verified"
  ```

- [ ] **Step 89** [W] ~10m: Create `docs/protocol-audit-S41.md`
  - Summary of all protocol alignment checks
  - Any mismatches found and their severity
  - Recommendations for fixes
  - Cross-reference to RFC specs (Monad draft-03, Sophia-00, Wotan-00)

- [ ] **Step 90** [B] ~2m: Check Hop-by-Hop extension header handling
  ```bash
  grep -rn "HbH\|hop.by.hop\|hbh\|extension.header\|IPv6.*option" pkg/ ebpf/ cmd/ | head -20
  ```

- [ ] **Step 91** [B] ~2m: Check if RFC 791/792/793/768 references exist in code comments
  ```bash
  grep -rn "RFC.*79[123]\|RFC.*768\|RFC.*9669\|RFC.*8200" pkg/ cmd/ ebpf/ docs/ | head -20
  ```

- [ ] **Step 92** [V] ~1m: RFC references present where protocol operations happen
  - If yes → Step 93
  - If missing → add in next step

- [ ] **Step 93** [W] ~5m: Add missing RFC references to key protocol files
  - `pkg/ebpf/loader.go` — RFC 9669 (BPF), RFC 8200 (IPv6)
  - `cmd/trace-collector-go/types.go` — RFC 793 (TCP), RFC 768 (UDP)
  - `ebpf/` Rust sources — relevant RFCs in module docs

- [ ] **Step 94** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 89-94: Protocol audit doc and RFC references added"
  ```

- [ ] **Step 95** [V]: **PHASE 3 EXIT GATE** — Protocol audit complete
  - Wire format alignment: VERIFIED
  - Map entry formats: CONSISTENT
  - CRC/checksum: VERIFIED (or documented gap)
  - Port usage: AUDITED
  - RFC references: PRESENT
  - Audit doc: WRITTEN
  - If pass → Phase 4
  - If fail → DO NOT PROCEED

---

## PHASE 4: BINARY NAMING PER KINGDOM LORE (Steps 96-125)

**Goal**: Name all compiled binaries after lore from the Unheaded realm. Document the naming.
**Prerequisite**: Phase 3 complete
**Time**: 30-45 minutes
**Agent**: Agent

- [ ] **Step 96** [B] ~1m: List ALL compiled binary targets
  ```bash
  find cmd/ -name "main.go" -exec dirname {} \; | sort
  ```

- [ ] **Step 97** [R] ~2m: Read the Lore skill for naming conventions
  ```bash
  cat ~/.skills/skills/unheaded-lore/SKILL.md | head -100 || echo "Read lore from docs"
  ```

- [ ] **Step 98** [R] ~1m: Check existing binary names in bin/
  ```bash
  ls -la bin/ | head -20
  ```

- [ ] **Step 99** [W] ~10m: Create binary naming map document `docs/binary-lore-names.md`

  Proposed naming scheme (each binary gets a lore name + functional description):

  | Current Name | Lore Name | Pillar | Description |
  |-------------|-----------|--------|-------------|
  | dashboard-backend | **visor** | Medieval Armory | The visor that lets the knight see the battlefield |
  | trace-collector-go | **vambraces** | Medieval Armory | Arm guards that sense and record every blow |
  | wotan | **wotan** | Gnostic/Norse | Already named — the All-Father of memory |
  | wotan-ctl | **gungnir** | Norse | Wotan's spear — his command instrument |
  | unheaded-daemon | **aegis** | Greek/Medieval | The divine shield, the control plane daemon |
  | kanban-app | **herald** | Medieval | The herald who announces and tracks tasks |
  | doom-bridge | **bifrost** | Norse | The bridge between realms (Doom ↔ Kingdom) |
  | monad | **monad** | Gnostic | Already named — the divine spark |
  | sophia | **sophia** | Gnostic | Already named — divine wisdom |
  | architect | **mason** | Medieval | The master builder |
  | captain | **pennant** | Medieval/Naval | The flag of command |
  | micromanager | **quartermaster** | Medieval/Military | One who tracks every supply |
  | timeguru | **hourglass** | Medieval | The keeper of time |
  | gateway | **portcullis** | Medieval | The fortified gateway |

- [ ] **Step 100** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 96-100: Binary lore naming scheme documented"
  ```

- [ ] **Step 101** [W] ~5m: Update Makefile / build scripts to output lore-named binaries
  - Build target: `make binaries` produces both names (symlink or copy)
  - Example: `go build -o bin/visor ./cmd/dashboard-backend/`
  - Keep functional names as aliases for discoverability

- [ ] **Step 102** [B] ~3m: Rebuild all binaries with lore names
  ```bash
  cd ~/tmp/unheaded && for dir in cmd/*/; do name=$(basename "$dir"); go build -o "bin/$name" "./$dir" 2>&1; done | tail -20
  ```

- [ ] **Step 103** [V] ~1m: All binaries build
  ```bash
  ls -la bin/ | wc -l && ls bin/
  ```

- [ ] **Step 104** [W] ~5m: Create symlinks or wrapper scripts with lore names
  ```bash
  cd ~/tmp/unheaded/bin && ln -sf dashboard-backend visor && ln -sf trace-collector-go vambraces && ln -sf wotan-ctl gungnir && ln -sf unheaded-daemon aegis && ln -sf kanban-app herald && ln -sf doom-bridge bifrost
  ```

- [ ] **Step 105** [V] ~30s: Lore-named binaries work
  ```bash
  ls -la bin/visor bin/vambraces bin/gungnir bin/aegis bin/herald bin/bifrost 2>&1 | head -10
  ```

- [ ] **Step 106** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 101-106: Lore-named binary symlinks created"
  ```

- [ ] **Step 107** [W] ~5m: Update `configs/port-registry.yaml` with lore names
  - Add `lore_name` field to each service entry
  - Cross-reference with `docs/binary-lore-names.md`

- [ ] **Step 108** [W] ~3m: Update `configs/services.yaml` with lore names
  - Each service gets a `display_name` (lore name) alongside `binary_name` (functional name)

- [ ] **Step 109** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 107-109: Config files updated with lore names"
  ```

- [ ] **Step 110** [V]: **PHASE 4 EXIT GATE** — All binaries named, documented, configs updated
  - `docs/binary-lore-names.md`: EXISTS with full mapping
  - Symlinks: ALL lore names work
  - Configs: UPDATED
  - If pass → Phase 5
  - If fail → DO NOT PROCEED

---

## PHASE 5: BINDER BOOK SCAFFOLDING (Steps 111-145)

**Goal**: Create ELI5 / middle school / high school / PhD-level docs as preamble to RFC.
**Prerequisite**: Phase 3 or Phase 4 complete (parallelizable)
**Time**: 45-60 minutes
**Agent**: Agent [P]

- [ ] **Step 111** [R] ~2m: Check existing documentation structure
  ```bash
  ls docs/ && ls docs/protocol/ 2>/dev/null && ls docs/specs/ 2>/dev/null
  ```

- [ ] **Step 112** [W] ~1m: Create binder book directory
  ```bash
  mkdir -p docs/binder-book/{eli5,middle-school,high-school,phd}
  ```

- [ ] **Step 113** [W] ~10m: Write `docs/binder-book/eli5/what-is-unheaded.md`
  - Explain like I'm 5: "Imagine every letter you send has a special sticker..."
  - Use analogies: post office (routing), stamps (Monad), address book (Sophia), memory box (Wotan)
  - No technical jargon
  - Target: someone who has never used a computer professionally

- [ ] **Step 114** [W] ~10m: Write `docs/binder-book/eli5/how-packets-work.md`
  - What is a packet? (A digital envelope)
  - What's inside? (The message and the address)
  - How does it get there? (Routers are like postal sorting facilities)
  - What does Unheaded add? (A special tracking sticker on every envelope)

- [ ] **Step 115** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 111-115: ELI5 binder book section created"
  ```

- [ ] **Step 116** [W] ~10m: Write `docs/binder-book/middle-school/networking-basics.md`
  - IP addresses, ports, protocols (TCP/UDP)
  - What a server does, what a client does
  - Why we need firewalls and monitoring
  - Simple diagrams using ASCII art

- [ ] **Step 117** [W] ~10m: Write `docs/binder-book/middle-school/what-unheaded-does.md`
  - The 3 pillars: networking + observability + security
  - eBPF explained simply: "programs that run inside the kernel"
  - The wire protocol: "extra information attached to every packet"
  - Why this matters: faster, safer, more visible networks

- [ ] **Step 118** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 116-118: Middle school binder book section"
  ```

- [ ] **Step 119** [W] ~10m: Write `docs/binder-book/high-school/architecture-overview.md`
  - The full stack: XDP → TC → kprobes → userspace → dashboard
  - IPv6 Hop-by-Hop extension headers
  - BPF maps and ring buffers
  - 5-tuple flow correlation
  - Prometheus metrics
  - Diagrams with proper protocol stack layers

- [ ] **Step 120** [W] ~10m: Write `docs/binder-book/high-school/protocol-primer.md`
  - Monad: 20-byte register file in IPv6 HbH headers
  - Sophia: BPF map state management
  - Wotan: memory persistence and snapshots
  - Anamnesis: event streaming via ring buffers
  - Shield: XDP packet filtering
  - How they work together

- [ ] **Step 121** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 119-121: High school binder book section"
  ```

- [ ] **Step 122** [W] ~10m: Write `docs/binder-book/phd/formal-specification-intro.md`
  - Stateful stream transducer model: T: (State × Input) → (State' × Output)
  - Information-theoretic analysis of observability depth
  - Comparison to Flink's checkpoint model (Chandy-Lamport)
  - BPF verifier constraints and their implications
  - Wire format security analysis
  - References to academic papers from the eBPF ecosystem comparison

- [ ] **Step 123** [W] ~10m: Write `docs/binder-book/phd/competitive-landscape.md`
  - Summarize all 4 comparison battle plans (Flink, K8s/Cilium/Flannel, Coroot, eBPF ecosystem)
  - Formal capability matrices: C(X) ⊂ C(Unheaded)
  - 72 projects assessed, 16 competitors, unique position confirmed
  - Wire-level depth advantage is architecturally unbridgeable

- [ ] **Step 124** [W] ~5m: Write `docs/binder-book/README.md`
  - Table of contents linking all 4 levels
  - Reading guide: "Start with ELI5, graduate up"
  - Purpose: preamble to the formal RFC/protocol specifications

- [ ] **Step 125** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 122-125: PhD + binder book README complete"
  ```

- [ ] **Step 126** [V]: **PHASE 5 EXIT GATE** — Binder book scaffolded at all 4 levels
  - ELI5: 2 docs ✓
  - Middle school: 2 docs ✓
  - High school: 2 docs ✓
  - PhD: 2 docs ✓
  - README: 1 doc ✓
  - Total: 9 documents
  - If pass → Phase 6
  - If fail → DO NOT PROCEED

---

## PHASE 6: ECOSYSTEM COMPARISON VALIDATION (Steps 127-150)

**Goal**: Verify topics from the 4 comparison docs (Flink, K8s/Cilium/Flannel, Coroot, eBPF ecosystem) are accounted for in codebase.
**Prerequisite**: Phase 3 complete (protocol audit done)
**Time**: 30 minutes
**Agent**: Agent [P]

- [ ] **Step 127** [B] ~2m: Check if Apache Flink patterns are referenced/planned
  ```bash
  grep -rn "flink\|Flink\|watermark\|Watermark\|checkpoint\|Checkpoint\|tide.mark" docs/ pkg/ cmd/ | head -20
  ```

- [ ] **Step 128** [B] ~2m: Check if Cilium-inspired patterns exist
  ```bash
  grep -rn "cilium\|Cilium\|identity\|ClusterMesh\|hubble\|Hubble" docs/ pkg/ cmd/ | head -20
  ```

- [ ] **Step 129** [B] ~2m: Check if Coroot-inspired patterns are planned
  ```bash
  grep -rn "coroot\|Coroot\|inspection\|Inspection\|SLO\|slo\|edict\|Edict\|oath\|Oath" docs/ pkg/ cmd/ | head -20
  ```

- [ ] **Step 130** [B] ~2m: Check if chroot/seccomp/namespace hardening exists
  ```bash
  grep -rn "chroot\|seccomp\|namespace\|cgroup\|hardening" configs/ pkg/ cmd/ docs/ | head -20
  ```

- [ ] **Step 131** [B] ~2m: Check Apache (httpd) references — do we use/document it?
  ```bash
  grep -rn "apache\|Apache\|httpd" docs/ configs/ | head -10
  ```

- [ ] **Step 132** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 127-132: Ecosystem comparison validation sweep"
  ```

- [ ] **Step 133** [W] ~10m: Create `docs/ecosystem-validation-S41.md`
  - Checklist of all technologies from comparison docs
  - Status: PRESENT / PLANNED / NOT YET / NOT APPLICABLE
  - Categories: Networking (Cilium, Flannel, Calico, Katran), Observability (Coroot, Hubble, Pixie), Security (Falco, Tetragon, Tracee), Toolchain (Aya, bpftool, libbpf), Stream Processing (Flink patterns)
  - Gap list with priority and target Age

- [ ] **Step 134** [B] ~2m: Check if eBPF offensive tools from comparison are documented
  ```bash
  grep -rn "ebpfkit\|TripleCross\|Bad.BPF\|rootkit" docs/ | head -10
  ```

- [ ] **Step 135** [B] ~2m: Check if Bombini (Rust+Aya+LSM) patterns are referenced
  ```bash
  grep -rn "Bombini\|LSM\|lsm\|BPF_LSM" docs/ ebpf/ | head -10
  ```

- [ ] **Step 136** [B] ~2m: Check if Katran (Meta XDP LB) patterns are studied
  ```bash
  grep -rn "Katran\|katran\|XDP.*LB\|load.balanc" docs/ pkg/ | head -10
  ```

- [ ] **Step 137** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 133-137: Ecosystem validation doc created"
  ```

- [ ] **Step 138** [V]: **PHASE 6 EXIT GATE** — All comparison topics validated
  - Ecosystem validation doc: EXISTS
  - All 72+ eBPF ecosystem projects: CLASSIFIED (from comparison doc)
  - Gap list: DOCUMENTED
  - If pass → Phase 7
  - If fail → DO NOT PROCEED

---

## PHASE 7: DOCUMENTATION RENAMES (Steps 139-155)

**Goal**: Apply S36 mandatory renames: "product" → "application", "customer" → "user", remove champagne references.
**Prerequisite**: None (parallelizable)
**Time**: 20 minutes
**Agent**: Agent [P]

- [ ] **Step 139** [B] ~1m: Count "product" references that need changing
  ```bash
  grep -rn '"product"\|the product\|our product\|Product' docs/ --include="*.md" | grep -v "node_modules\|vendor\|battle-plan" | wc -l
  ```

- [ ] **Step 140** [B] ~1m: Count "customer" references that need changing
  ```bash
  grep -rn 'customer\|Customer' docs/ --include="*.md" | grep -v "node_modules\|vendor\|battle-plan" | wc -l
  ```

- [ ] **Step 141** [B] ~1m: Count champagne references
  ```bash
  grep -rn 'champagne\|Champagne\|drink our own' docs/ --include="*.md" | wc -l
  ```

- [ ] **Step 142** [B] ~5m: Replace "product" → "application" in docs (excluding battle plans and session notes)
  ```bash
  find docs/ -name "*.md" -not -path "*/battle-plans/*" -not -path "*/sessions/*" -exec sed -i 's/\bthe product\b/the application/g; s/\bour product\b/our application/g; s/\bthis product\b/this application/g' {} +
  ```

- [ ] **Step 143** [B] ~5m: Replace "customer" → "user" in docs
  ```bash
  find docs/ -name "*.md" -not -path "*/battle-plans/*" -not -path "*/sessions/*" -exec sed -i 's/\bcustomer\b/user/g; s/\bCustomer\b/User/g; s/\bcustomers\b/users/g; s/\bCustomers\b/Users/g' {} +
  ```

- [ ] **Step 144** [B] ~2m: Remove champagne references
  ```bash
  find docs/ -name "*.md" -exec sed -i '/drink our own champagne/d; /We drink our own/d; /champagne/Id' {} +
  ```

- [ ] **Step 145** [V] ~1m: Verify renames applied
  ```bash
  echo "Remaining 'the product':" && grep -rn 'the product' docs/ --include="*.md" | grep -v "battle-plans\|sessions" | wc -l
  echo "Remaining 'customer':" && grep -rn '\bcustomer\b' docs/ --include="*.md" | grep -v "battle-plans\|sessions" | wc -l
  echo "Remaining 'champagne':" && grep -rn 'champagne' docs/ --include="*.md" | wc -l
  ```

- [ ] **Step 146** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 139-146: Mandatory doc renames — product→application, customer→user, champagne removed"
  ```

- [ ] **Step 147** [V]: **PHASE 7 EXIT GATE** — All renames applied
  - "product" references: ZERO (in scope files)
  - "customer" references: ZERO (in scope files)
  - "champagne" references: ZERO
  - If pass → Phase 8
  - If fail → DO NOT PROCEED

---

## PHASE 8: PORT RANGE TUNING DOCUMENTATION (Steps 148-162)

**Goal**: Document and implement Linux ephemeral port range tuning to avoid Doom Range collision.
**Prerequisite**: Phase 0 complete
**Time**: 15 minutes
**Agent**: Agent [P]

- [ ] **Step 148** [R] ~1m: Check current port range config
  ```bash
  grep -rn "ip_local_port_range\|ephemeral\|Doom Range\|16666\|26666" configs/ docs/ | head -20
  ```

- [ ] **Step 149** [W] ~5m: Add port range tuning to NixOS/sysctl config
  - `configs/networking/sysctl-tuning.yaml` (or similar)
  - `net.ipv4.ip_local_port_range = 27000 65000`
  - Comment: "Avoids collision with Doom Range (16666-26666)"
  - Document in deployment docs

- [ ] **Step 150** [W] ~3m: Add port range check to health verification
  - `/health` endpoint or startup check should verify ephemeral port range doesn't overlap Doom Range
  - Log warning if overlap detected

- [ ] **Step 151** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 148-151: Ephemeral port range tuning documented"
  ```

- [ ] **Step 152** [V]: **PHASE 8 EXIT GATE** — Port range documented and configured
  - Sysctl config: EXISTS
  - Doom Range avoidance: DOCUMENTED
  - If pass → Phase 9
  - If fail → DO NOT PROCEED

---

## PHASE 9: E2E SMOKE TEST (Steps 153-175)

**Goal**: Full integration smoke test — start services, verify data flows through entire pipeline.
**Prerequisite**: Phases 1-4 complete
**Time**: 30 minutes
**Agent**: Coordinator

- [ ] **Step 153** [B] ~2m: Rebuild all binaries
  ```bash
  cd ~/tmp/unheaded && go build -o bin/dashboard-backend ./cmd/dashboard-backend/ && go build -o bin/trace-collector-go ./cmd/trace-collector-go/ && go build -o bin/wotan ./services/wotan/cmd/wotan/
  ```

- [ ] **Step 154** [V] ~30s: All binaries exist
  ```bash
  ls -la bin/dashboard-backend bin/trace-collector-go bin/wotan
  ```

- [ ] **Step 155** [B] ~1m: Start Wotan
  ```bash
  bin/wotan &
  sleep 2 && curl -s http://localhost:18000/health | head -5
  ```

- [ ] **Step 156** [V] ~30s: Wotan healthy on 18000/18001
  - If pass → Step 157
  - If fail → Step 156a [D]

- [ ] **Step 156a** [D] ~3m: Check Wotan logs
  ```bash
  journalctl -u wotan --no-pager -n 20 || cat /tmp/wotan.log 2>/dev/null | tail -20
  ```

- [ ] **Step 157** [B] ~1m: Start dashboard-backend
  ```bash
  bin/dashboard-backend &
  sleep 2 && curl -s http://localhost:20000/ | head -5
  ```

- [ ] **Step 158** [V] ~30s: Dashboard serves HTML at port 20000
  - If pass → Step 159
  - If fail → Step 158a [D]

- [ ] **Step 158a** [D] ~3m: Check dashboard logs
  ```bash
  curl -v http://localhost:20000/ 2>&1 | head -20
  ```

- [ ] **Step 159** [B] ~1m: Start trace-collector in demo mode
  ```bash
  bin/trace-collector-go -demo -wotan-addr localhost:18001 &
  sleep 3
  ```

- [ ] **Step 160** [B] ~1m: Check flow count
  ```bash
  curl -s http://localhost:20000/api/v1/flows | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Flows: {d.get(\"total_flows\", len(d.get(\"flows\", [])))}') " 2>/dev/null || curl -s http://localhost:20000/api/v1/flows | head -5
  ```

- [ ] **Step 161** [V] ~30s: Flows > 0
  - If pass → Step 162
  - If fail → Step 161a [D]

- [ ] **Step 161a** [D] ~3m: Check stats for parse errors
  ```bash
  curl -s http://localhost:20000/api/v1/ebpf/stats | python3 -m json.tool 2>/dev/null || curl -s http://localhost:20000/api/v1/ebpf/stats
  ```

- [ ] **Step 162** [B] ~1m: Check traces endpoint
  ```bash
  curl -s http://localhost:20000/api/v1/traces | head -5
  ```

- [ ] **Step 163** [B] ~1m: Check advanced viz routes
  ```bash
  curl -s -o /dev/null -w "%{http_code}" http://localhost:20000/viz/doom 2>/dev/null || echo "Route not yet available"
  curl -s -o /dev/null -w "%{http_code}" http://localhost:20000/viz/latency 2>/dev/null || echo "Route not yet available"
  ```

- [ ] **Step 164** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 153-164: E2E smoke test — pipeline verified"
  ```

- [ ] **Step 165** [B] ~1m: Stop all services
  ```bash
  pkill -f dashboard-backend; pkill -f trace-collector; pkill -f wotan; sleep 1 && echo "All stopped"
  ```

- [ ] **Step 166** [V]: **PHASE 9 EXIT GATE** — E2E pipeline verified
  - Wotan: healthy on 18000/18001
  - Dashboard: serves HTML on 20000
  - Flows: ingested and visible via API
  - Advanced viz routes: functional (or documented as TODO)
  - All services stopped cleanly
  - If pass → Phase 10
  - If fail → DO NOT PROCEED

---

## PHASE 10: TEST SUITE HARDENING (Steps 167-185)

**Goal**: Ensure all new code has tests. Run full suite with race detector.
**Prerequisite**: Phase 9 complete
**Time**: 20 minutes
**Agent**: Agent

- [ ] **Step 167** [B] ~5m: Run FULL test suite across entire repo
  ```bash
  cd ~/tmp/unheaded && go test -race -count=1 ./... 2>&1 | tail -40
  ```

- [ ] **Step 168** [V] ~1m: All tests pass (document any pre-existing failures)
  - If all pass → Step 169
  - If failures → Step 168a [D]

- [ ] **Step 168a** [D] ~5m: Identify and classify failures
  ```bash
  go test -race -count=1 ./... 2>&1 | grep "FAIL" | sort -u
  ```
  - Pre-existing (known): document and continue
  - New failures: FIX before proceeding

- [ ] **Step 169** [B] ~2m: Check test coverage for dashboard-backend
  ```bash
  go test -cover ./cmd/dashboard-backend/... 2>&1 | tail -10
  ```

- [ ] **Step 170** [B] ~2m: Check test coverage for trace-collector
  ```bash
  go test -cover ./cmd/trace-collector-go/... 2>&1 | tail -10
  ```

- [ ] **Step 171** [W] ~5m: Add any missing tests for new dispatch logic
  - Test: compute.hop topic → correct event type
  - Test: anamnesis.* topic → correct conversion
  - Test: unknown topic → warning logged, no crash

- [ ] **Step 172** [B] ~2m: Re-run dashboard tests
  ```bash
  go test -race -count=1 -v ./cmd/dashboard-backend/... 2>&1 | tail -30
  ```

- [ ] **Step 173** [V] ~30s: All pass
  - If pass → Step 174
  - If fail → FIX before proceeding

- [ ] **Step 174** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 167-174: Full test suite verified, coverage checked"
  ```

- [ ] **Step 175** [V]: **PHASE 10 EXIT GATE** — Test suite green
  - All tests: PASS (or pre-existing documented)
  - New code: COVERED
  - Race detector: CLEAN
  - If pass → Phase 11
  - If fail → DO NOT PROCEED

---

## PHASE 11: SESSION HANDOFF & DOCUMENTATION (Steps 176-200)

**Goal**: Write S41 handoff doc, update timeline, prepare for S42+.
**Prerequisite**: All prior phases complete
**Time**: 20 minutes
**Agent**: Agent

- [ ] **Step 176** [B] ~1m: Full git status and diff summary
  ```bash
  git diff --stat HEAD~10 | tail -20 && echo "---" && git log --oneline -10
  ```

- [ ] **Step 177** [W] ~10m: Write `docs/sessions/S41-kingdom-hardening-handoff.md`
  - What was done (all phases)
  - What's next (S42: Cloudflare Research or Centralized Config)
  - Current state (services, tests, dashboard, pipeline)
  - Known issues remaining
  - Key files changed

- [ ] **Step 178** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 176-178: S41 handoff doc written"
  ```

- [ ] **Step 179** [W] ~5m: Update CLAUDE.md / MEMORY.md with S41 findings
  - Dashboard topic routing expanded
  - Binary lore names documented
  - Protocol audit results
  - Binder book location
  - Ecosystem validation status

- [ ] **Step 180** [W] ~3m: Update timeline if it exists
  ```bash
  ls docs/timeline* 2>/dev/null && cat docs/timeline.md | head -20
  ```

- [ ] **Step 181** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S41] Steps 179-181: CLAUDE.md and timeline updated"
  ```

- [ ] **Step 182** [B] ~1m: Final full test
  ```bash
  go test -race -count=1 ./cmd/dashboard-backend/... ./cmd/trace-collector-go/... ./pkg/ebpf/... 2>&1 | tail -20
  ```

- [ ] **Step 183** [V] ~30s: All pass
  - If pass → Step 184
  - If fail → FIX

- [ ] **Step 184** [B] ~30s: Final git status
  ```bash
  git status
  ```

- [ ] **Step 185** [C] ~30s: **FINAL COMMIT**
  ```bash
  git add -A && git commit -m "feat(S41): Kingdom Hardening complete — dashboard, protocol audit, binary lore, binder book"
  ```

- [ ] **Step 186** [V]: **PHASE 11 EXIT GATE** — Sprint complete, handoff ready
  - Handoff doc: WRITTEN
  - CLAUDE.md: UPDATED
  - All tests: PASS
  - Git: CLEAN
  - If pass → SPRINT COMPLETE
  - If fail → Document remaining items

---

## PHASE 12: COPY COMPARISON DOCS TO REPO (Steps 187-195)

**Goal**: Archive the 4 comparison battle plans in the repo for permanent reference.
**Prerequisite**: None (parallelizable)
**Time**: 10 minutes
**Agent**: Agent [P]

- [ ] **Step 187** [B] ~1m: Copy comparison docs to battle-plans directory
  ```bash
  cp ~/uploads/battle-plan-ebpf-ecosystem.md docs/battle-plans/
  cp ~/uploads/battle-plan-coroot.md docs/battle-plans/
  cp ~/uploads/battle-plan-unheaded-vs-flink.md docs/battle-plans/
  cp ~/uploads/battle-plan-k8s-cilium-flannel.md docs/battle-plans/
  ```

- [ ] **Step 188** [V] ~30s: All 4 docs in battle-plans/
  ```bash
  ls docs/battle-plans/battle-plan-*.md
  ```

- [ ] **Step 189** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add docs/battle-plans/battle-plan-*.md && git commit -m "docs: archive ecosystem comparison battle plans (Flink, K8s/Cilium, Coroot, eBPF ecosystem)"
  ```

- [ ] **Step 190** [V]: **PHASE 12 EXIT GATE** — Comparison docs archived
  - 4 comparison docs in battle-plans/: VERIFIED
  - If pass → DONE
  - If fail → DO NOT PROCEED

---

## APPENDIX A: EMERGENCY PROCEDURES

### Dashboard Won't Build
```
1. Check go.mod for missing deps: go mod tidy
2. Check for circular imports: go build -v ./cmd/dashboard-backend/ 2>&1
3. If embed fails: verify static/ directory exists and has files
4. If io/fs import fails: verify Go >= 1.16
```

### Tests Fail After Topic Routing Changes
```
1. Run specific failing test: go test -v -run "TestName" ./cmd/dashboard-backend/...
2. Check if test expects old topic format
3. Verify dispatchSingle() handles all cases
4. Check for nil pointer in event type conversion
```

### Pipeline Shows 0 Flows After Changes
```
1. Check Wotan is running: curl localhost:18000/health
2. Check trace-collector publishes: curl localhost:16670/metrics | grep publish
3. Check dashboard ingests: curl localhost:20000/api/v1/ebpf/stats
4. Check topic names match between publisher and subscriber
5. Look for parse errors in stats
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent Type | Parallelizable | Dependencies | Est. Time |
|-------|-----------|---------------|-------------|-----------|
| 0: Environment | Coordinator | No | None | 15m |
| 1: Topic Routing | Agent | No | Phase 0 | 30m |
| 2: Dashboard UI | Coordinator | No | Phase 1 | 60-90m |
| 3: Protocol Audit | Agent | Yes (with Phase 4-8) | Phase 0 | 45-60m |
| 4: Binary Naming | Agent | Yes (with Phase 3) | Phase 0 | 30-45m |
| 5: Binder Book | Agent [P] | Yes (with Phase 3-4) | None | 45-60m |
| 6: Ecosystem Valid | Agent [P] | Yes (with Phase 5) | Phase 3 | 30m |
| 7: Doc Renames | Agent [P] | Yes (independent) | None | 20m |
| 8: Port Range | Agent [P] | Yes (independent) | None | 15m |
| 9: E2E Smoke | Coordinator | No | Phases 1-4 | 30m |
| 10: Test Hardening | Agent | No | Phase 9 | 20m |
| 11: Handoff | Agent | No | All | 20m |
| 12: Copy Docs | Agent [P] | Yes (independent) | None | 10m |

**Critical Path**: Phase 0 → Phase 1 → Phase 2 → Phase 9 → Phase 10 → Phase 11
**Minimum Time**: ~4 hours (critical path only, sequential)
**With Parallelization**: ~3 hours (Phases 3-8 run alongside Phase 2)

---

## APPENDIX C: QUICK REFERENCE

### Key File Paths
```
cmd/dashboard-backend/internal/ebpf/ingestor.go  — Topic dispatch logic (FIX HERE)
cmd/dashboard-backend/static/                      — Embedded frontend (REDESIGN HERE)
cmd/trace-collector-go/main.go                     — Publisher / anamnesisToFlowJSON bridge
dashboard/                                         — Advanced visualizations (SERVE THESE)
pkg/ebpf/loader.go                                — BPF loader (3,937 LOC)
cmd/trace-collector-go/types.go                    — Wire format Go types
ebpf/                                              — Rust eBPF programs
configs/port-registry.yaml                         — Port assignments
docs/battle-plans/                                 — Battle plan archive
docs/binder-book/                                  — Multi-level documentation (CREATE)
```

### Service Ports (Doom Range: 16666-26666)
```
Wotan HTTP:        18000
Wotan gRPC:        18001
Dashboard:         20000
Trace Collector:   16670 (metrics)
```

### Topic Name Patterns
```
ebpf.flow.events    — Flow events (dashboard native)
ebpf.packet.events  — Packet events (dashboard native)
ebpf.latency.events — Latency events (dashboard native)
compute.hop         — Compute hop events (NEEDS ROUTING)
compute.miss        — Compute miss events (NEEDS ROUTING)
compute.halt        — Compute halt events (NEEDS ROUTING)
compute.syscall     — Compute syscall events (partially routed)
anamnesis.*         — Anamnesis events (NEEDS BRIDGE)
```

---

---

## AUTO-CHAIN: When S41 is COMPLETE, proceed to S42

**CRITICAL INSTRUCTION**: When ALL phases of S41 are complete and the final EXIT GATE passes, DO NOT STOP. Instead:

1. Report: "S41 COMPLETE — KINGDOM HARDENED"
2. Read `docs/battle-plans/S42-DOOM-POC-BATTLE-PLAN.md`
3. Begin executing S42 Phase 0 immediately

If S42 has issues (bare metal Linux with kernel BPF support now available on WEST), execute all phases including BPF-dependent ones:

1. Report: "S42 partially blocked — executing non-BPF phases" (Note: BPF environment resolved)
2. Execute all phases that DON'T require `sudo` or network namespaces
3. Document skipped phases in handoff doc
4. Continue to next available battle plan

The sprint chain is: **S41 → S42 → STOP and report**

---

*S41 Battle Plan — Forged 2026-02-24*
*12 Phases. 190 Steps. The Kingdom hardens its armor and sharpens its blade.*
*From ugly prototype to polished steel — the dashboard rises, the protocol holds, the lore endures.*
