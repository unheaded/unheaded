# Unheaded Alpha: Agent Dispatch Plan
**Date:** January 27, 2026
**Status:** Ready for execution
**Coordination:** Timeguru + Micromanager

---

## Overview

**Objective:** Deliver Unheaded alpha by Feb 8, 2026 via aggressive parallel execution

**Model:** 7-8 independent Claude agents working simultaneously on non-blocking components

**Synchronization:** Busboy message bus as single source of truth for inter-service communication

---

## Agent Assignments

### Agent 1: timeguru Service (REST API)
**Owner:** Micromanager
**Timeline:** Jan 27-29 (2 days)
**Dependencies:** None

**Deliverables:**
- `/services/timeguru/main.go` - REST API server
- `/services/timeguru/*_test.go` - Unit tests (80%+ coverage)
- `/nix/containers/timeguru.nix` - NixOS container definition
- `docs/services/timeguru.md` - API documentation

**Scope:**
- Read `references/timeline.md` from disk
- Serve via REST API: `/api/v1/timeline` (GET/POST/PUT)
- Return JSON + YAML formats
- Publish state changes to Busboy topic: `timeline.updates`
- Health checks: `/health`, `/ready`
- Metrics: Prometheus scrape endpoint `/metrics`

**Busboy Integration:**
- Connect to Busboy on startup
- Subscribe to `timeline.updates` (listen for other changes)
- Publish to `timeline.updates` when modified
- Graceful shutdown on SIGTERM

**Interface Contract:**
```go
type Timeline struct {
    Title    string
    Phase    string
    Progress float64 // 0-100
    // ... full structure in CLAUDE.md
}
```

**Success Criteria:**
- [ ] Service starts and responds to requests
- [ ] Unit tests pass (80%+ coverage)
- [ ] Busboy connection working
- [ ] Container definition builds in NixOS
- [ ] API returns valid JSON/YAML

---

### Agent 2: captain Service (Strategy)
**Owner:** Micromanager
**Timeline:** Jan 27-28 (1.5 days)
**Dependencies:** None

**Deliverables:**
- `/services/captain/main.go` - REST API server
- `/services/captain/*_test.go` - Unit tests
- `/nix/containers/captain.nix` - NixOS container definition

**Scope:**
- REST API stub (`/api/v1/strategy`)
- Subscription to `timeline.updates` from Busboy
- Publish strategic decisions to `strategy.decisions` topic
- Health + metrics endpoints
- Simple in-memory state (no persistence needed for alpha)

**Success Criteria:**
- [ ] Service starts and responds
- [ ] Listens to timeline updates
- [ ] Can publish decisions to Busboy
- [ ] Unit tests passing

---

### Agent 3: micromanager Service (Execution)
**Owner:** Micromanager
**Timeline:** Jan 27-28 (1.5 days)
**Dependencies:** None

**Deliverables:**
- `/services/micromanager/main.go` - REST API server
- `/services/micromanager/*_test.go` - Unit tests
- `/nix/containers/micromanager.nix` - NixOS container definition

**Scope:**
- Task execution API
- Subscription to `strategy.decisions` from Busboy
- Publish task progress to `execution.progress` topic
- Health + metrics endpoints

**Success Criteria:**
- [ ] Service responds to requests
- [ ] Listens to strategy decisions
- [ ] Publishes execution progress
- [ ] Tests passing

---

### Agent 4: architect Service (Design)
**Owner:** Micromanager
**Timeline:** Jan 27-28 (1.5 days)
**Dependencies:** None

**Deliverables:**
- `/services/architect/main.go` - REST API server
- `/services/architect/*_test.go` - Unit tests
- `/nix/containers/architect.nix` - NixOS container definition

**Scope:**
- Design review API
- Subscription to `execution.progress` from Busboy
- Publish design updates to `design.updates` topic
- Health + metrics endpoints

**Success Criteria:**
- [ ] Service responds
- [ ] Listens to execution progress
- [ ] Publishes design updates
- [ ] Tests passing

---

### Agent 5: eBPF Programs (Kernel Space)
**Owner:** Architect
**Timeline:** Jan 27 - Feb 3 (6 days - CRITICAL PATH)
**Dependencies:** None (but blocks trace-collector)

**Deliverables:**
- `/ebpf/src/packet_marker.rs` - XDP program (trace ID injection)
- `/ebpf/src/flow_tracker.rs` - kprobe program (connection tracking)
- `/ebpf/src/latency_probe.rs` - tracepoint program (RTT measurement)
- `/ebpf/tests/*` - Kernel verifier tests
- `/ebpf/Cargo.toml` - Build configuration

**Scope:**
1. **packet_marker.rs** (XDP hook at L2)
   - Inject trace_id into packet metadata
   - Non-invasive, minimal performance impact
   - Test on bare metal kernel

2. **flow_tracker.rs** (kprobes at tcp_connect)
   - Track active connections
   - Report via ring buffer
   - Store trace_id correlation

3. **latency_probe.rs** (tracepoints at tcp_cleanup_rbuf)
   - Measure RTT per connection
   - Aggregate for dashboard

**Tech Stack:**
- Language: Rust (Aya framework)
- Build: `cargo build --target bpfel-unknown-none`
- Testing: Kernel verifier validation

**Critical Success Criteria:**
- [ ] Programs pass kernel verifier (non-negotiable)
- [ ] Compile to valid eBPF bytecode
- [ ] Load without kernel oops
- [ ] Report data to ring buffer
- [ ] Integration tests with trace-collector

**Risk Mitigation:**
- Use Aya stdlib helpers (proven patterns)
- Test on minimal kernel first
- Have fallback: if verifier fails, simplify probe logic
- Document kernel version requirements

---

### Agent 6: trace-collector Service (Rust)
**Owner:** Architect
**Timeline:** Feb 1-3 (2 days - depends on Agent 5)
**Dependencies:** eBPF programs (Agent 5)

**Deliverables:**
- `/cmd/trace-collector/src/main.rs` - Ring buffer reader
- `/cmd/trace-collector/Cargo.toml` - Build configuration
- Integration test with eBPF programs

**Scope:**
- Load eBPF programs at startup
- Read from ring buffer
- Parse trace data
- Publish to Busboy topic: `traces.raw`
- Handle backpressure

**Interface:**
```rust
pub struct Trace {
    trace_id: u64,
    timestamp: u64,
    connection: ConnectionInfo,
    latency_us: u32,
}
```

**Success Criteria:**
- [ ] Loads eBPF programs
- [ ] Reads from ring buffer
- [ ] Publishes to Busboy
- [ ] Handles disconnects gracefully

---

### Agent 7: unheaded-daemon (Control Plane)
**Owner:** Architect
**Timeline:** Jan 27 - Feb 5 (8 days)
**Dependencies:** None (but controls 1.2-1.4 integration)

**Deliverables:**
- `/cmd/unheaded-daemon/main.go` - Orchestration engine
- `/cmd/unheaded-daemon/*_test.go` - Unit tests
- `/pkg/lxd/*` - LXD client implementation
- `/pkg/state/*` - State management
- `/nix/containers/daemon.nix` - NixOS container definition

**Scope:**
1. **LXD Orchestration**
   - Launch containers via `lxc` CLI or SDK
   - Wait for container startup
   - Report readiness

2. **State Management**
   - Desired state: NixOS flake definitions
   - Actual state: `/var/lib/unheaded/state/containers.json`
   - Drift detection (poll every 30s)
   - Auto-remediate on drift

3. **eBPF Loader**
   - Load eBPF programs into kernel
   - Track loaded programs
   - Report to Busboy: `system.ebpf.loaded`

4. **Telemetry**
   - Publish container health to Busboy
   - Report startup times
   - Track resource usage

**Success Criteria:**
- [ ] Starts containers
- [ ] Detects drift
- [ ] Loads eBPF programs
- [ ] Reports health via Busboy
- [ ] Unit tests passing

---

### Agent 8: Dashboard (Backend + UI)
**Owner:** Captain
**Timeline:** Jan 28 - Feb 7 (9 days)
**Dependencies:** Services + eBPF/trace-collector (for metrics)

**Deliverables - Backend:**
- `/cmd/dashboard-backend/main.go` - Metrics aggregation + WebSocket
- `/cmd/dashboard-backend/*_test.go` - Unit tests
- `/nix/containers/dashboard.nix` - NixOS container definition

**Deliverables - UI:**
- `/dashboard/index.html` - HTML structure
- `/dashboard/js/app.js` - React-free vanilla JS
- `/dashboard/css/style.css` - bellis.tech-inspired design
- `/dashboard/js/particles.js` - Background animation

**Scope - Backend:**
- Aggregate metrics from all services
- Subscribe to Busboy topics (metrics, traces, alerts)
- WebSocket endpoint for live updates
- Correlation engine (match requests to traces)
- `/api/v1/dashboard` REST API

**Scope - UI:**
- Real-time metrics display
- Trace timeline viewer
- Packet flow visualization
- Live system health
- Connection to backend via WebSocket

**Success Criteria:**
- [ ] Backend aggregates metrics
- [ ] WebSocket connection working
- [ ] UI renders without errors
- [ ] Live updates flowing
- [ ] Tests passing
- [ ] Looks polished (CSS)

---

### Agent 9: Kanban App Integration (Final Polish)
**Owner:** Captain (parallel with Agent 8)
**Timeline:** Jan 28 - Feb 7 (9 days)
**Dependencies:** timeguru service (Agent 1)

**Current Status:** 60% complete (408 LOC)

**Deliverables:**
- `/cmd/kanban-app/main.go` - Integration with timeguru
- `/kanban/index.html` - Updated board UI
- `/kanban/js/app.js` - Board rendering + updates
- Integration tests with timeguru service

**Remaining Scope:**
- Wire HTTP requests to timeguru service
- Parse timeline.json response
- Render interactive Kanban board
- Live indicators (what's in progress)
- Update handlers (click to change status)

**Success Criteria:**
- [ ] Reads timeline from timeguru
- [ ] Renders board correctly
- [ ] Updates work
- [ ] Integration tests pass
- [ ] Looks good (matches design)

---

## Execution Schedule

### Day 1: Jan 27 (Today)
**Goal:** Spawn all agents, establish baselines

```
Agent 1-4: Service scaffolds complete (empty main.go with structure)
Agent 5: eBPF environment validated + packet_marker.rs started
Agent 6: Waiting on Agent 5
Agent 7: Daemon skeleton started
Agent 8: Dashboard structure created
Agent 9: Kanban app existing code reviewed
```

**Sync Point:** 9am standup - report readiness

### Day 2: Jan 28
**Goal:** Services responding, eBPF compiling, dashboard framework

```
Agent 1: timeguru API responding (might fail on Busboy integration)
Agent 2-4: captain/micromanager/architect scaffolds responding
Agent 5: packet_marker.rs compiles, verifier testing
Agent 7: LXD client started, state skeleton
Agent 8: Dashboard backend listening
Agent 9: Kanban app wired to timeguru (partial)
```

**Blocker Watch:** eBPF kernel version issues?

### Day 3: Jan 29
**Goal:** Services talking via Busboy, eBPF passing verifier

```
Agent 1: timeguru → Busboy publishing/subscribing
Agent 2-4: All services → Busboy working
Agent 5: All 3 eBPF programs compiling + verifying
Agent 7: LXD orchestration working
Agent 8: Dashboard subscribing to metrics
Agent 9: Kanban board rendering with real data
```

### Day 4: Jan 30
**Goal:** Integration layer complete, containers definition done

```
Agent 5: eBPF programs tested on bare metal
Agent 6: trace-collector started (reading eBPF data)
Agent 7: Daemon starting containers
Agent 8: Dashboard showing live traces
Agent 9: Kanban app fully functional
Architect: NixOS container definitions created
```

### Day 5: Jan 31
**Goal:** All components in containers, talking via Busboy

```
All agents: Build containers + verify
Network integration: Gateway → services working
E2E test: Browser → gateway → services → Busboy
```

### Day 6: Feb 1
**Goal:** Polish + security validation

```
Security audit: Zero customer data access confirmed
UI refinement: Dashboard + Kanban looking great
Demo prep: Record video, create summary
```

### Day 7: Feb 2
**Goal:** Optional buffer/fixes

```
Buffer day for any blockers discovered
Final integration testing
Demo rehearsal
```

### Day 8: Feb 8
**Goal:** SHIP ALPHA

```
Public demo goes live
Kanban board accessible
eBPF traces running
```

---

## Synchronization & Coordination

### Daily Standup (9am)
**Format:** 5 min per agent
- What did I ship yesterday?
- What's blocking me?
- What do I ship today?

**Owner:** Micromanager + Timeguru

### Blockers & Escalation
**Response time:** < 4 hours
- Agent blocked → message in Unheaded Slack/chat
- Escalate to relevant skill (Architect, Micromanager, etc)
- Resolve or find workaround same day

### Integration Testing
**Timing:** Continuous from Day 2
- Unit tests: Each agent responsible
- Integration tests: Timeguru ensures Busboy contracts
- E2E tests: Captain runs full flow nightly

### Code Review
**Model:** Asynchronous
- Each agent commits daily
- Architect reviews eBPF/NixOS
- Micromanager reviews Go code
- Captain reviews UI/UX

---

## Busboy Message Contracts

### Required Topics (Pre-defined)
All agents must use these Busboy topics for coordination:

```
timeline.updates          → timeguru publishes, others subscribe
strategy.decisions        → captain publishes, micromanager subscribes
execution.progress        → micromanager publishes, architect subscribes
design.updates            → architect publishes, dashboard subscribes
traces.raw                → trace-collector publishes, dashboard subscribes
system.metrics.*          → all services publish metrics
system.ebpf.loaded        → daemon publishes eBPF status
system.errors.critical    → all services publish critical errors
```

### Message Format (Required)
```json
{
  "trace_id": "uuid",
  "timestamp": 1234567890,
  "service": "service_name",
  "topic": "topic_name",
  "data": { /* service-specific */ }
}
```

---

## Resource Checklist

### Before Agents Start

- [ ] Linux kernel 5.8+ on build machine
- [ ] Rust 1.70+ with eBPF target
- [ ] Go 1.21+
- [ ] LXD 5.0+ (for daemon testing)
- [ ] NixOS with flakes enabled (or Nix 2.4+)
- [ ] Git repository ready
- [ ] Busboy running and accessible
- [ ] CI/CD pipeline set up (GitHub Actions)

### Per Agent

- [ ] Read /CLAUDE.md (standards + patterns)
- [ ] Read docs/ARCHITECTURE.md (system overview)
- [ ] Read references/timeline.md (current status)
- [ ] Understand their component's dependencies
- [ ] Know who to ask for blockers

---

## Success Criteria for Alpha

### Technical
- [ ] All services running in containers
- [ ] Services communicating via Busboy
- [ ] eBPF programs loading + collecting traces
- [ ] Dashboard showing live metrics + traces
- [ ] Kanban app reading timeline from timeguru
- [ ] E2E test: Click → Request → Busboy → Dashboard → Visible

### Quality
- [ ] 80%+ unit test coverage (all components)
- [ ] Integration tests passing
- [ ] E2E tests passing
- [ ] Zero customer data access (security audit)
- [ ] <50ms latency (packet zero → browser)

### Demo-Readiness
- [ ] Public URL accessible
- [ ] Demo script prepared
- [ ] Video recorded
- [ ] GitHub repo public-ready
- [ ] Documentation complete

---

## Contingency Plans

### If eBPF Stalls
**Risk:** Kernel verifier rejects programs

**Plan B:** Implement simplified trace collection
- Use kprobes only (skip XDP)
- Reduce latency measurement scope
- Still proves architecture

**Timeline impact:** +2 days (Feb 10 instead of Feb 8)

### If Container Networking Fails
**Risk:** LXD bridge misconfiguration

**Plan B:** Use host networking for demo
- Services run on localhost
- Gateway on different port
- Proves functionality, not isolation

**Timeline impact:** +1 day (Feb 9 instead of Feb 8)

### If Busboy Integration Stalls
**Risk:** Service-to-service communication breaks

**Plan B:** Direct HTTP between services
- No Busboy messaging
- Still proves platform works
- Add Busboy in Phase 2

**Timeline impact:** Minimal (rework only)

---

## Communication Channels

### Primary
- Slack: #unheaded-alpha channel
- GitHub: Issues + PRs for blocking items

### Escalation
- Architect: Infrastructure/eBPF issues
- Micromanager: Execution/coordination issues
- Captain: Strategic/priority decisions
- Timeguru: Timeline/blockers tracking

### Daily Sync
- Time: 9am daily (Jan 27 - Feb 8)
- Duration: 15-30 min (5 min per agent)
- Format: Standup (shipped/blocked/next)

---

## Definitions of Done

### Per-Agent Deliverable
- [ ] Code compiles and runs
- [ ] Unit tests pass (80%+ coverage)
- [ ] Documentation complete (README + code comments)
- [ ] Container definition working
- [ ] Busboy integration tested
- [ ] Code review approved

### Phase Completion
- [ ] All components delivering
- [ ] Integration tests passing
- [ ] E2E test successful
- [ ] Security audit passed
- [ ] Demo rehearsal complete

---

## The Meta Moment

**Why Feb 8?**

Because on Feb 8, Unheaded will self-host its own development dashboard.

Users will see:
- Kanban board showing project timeline
- Real-time traces from eBPF
- Services communicating via Busboy
- All traced end-to-end from packet zero

**That's not just a demo. That's a promise.**

"We drink our own champagne" isn't marketing. It's proof.

---

**Status:** Ready for execution
**Decision required:** GO/NO-GO from Muck
**Timeline:** 12 days to ship (Jan 27 - Feb 8)
**Team:** 7-8 parallel Claude agents + human oversight

**THE TIMEGURU STANDS READY.**

