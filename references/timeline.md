# Unheaded Timeline

**THE LIVING ROADMAP**

This document tracks Unheaded's journey from conception to production-ready platform.

**Status:** Alpha Development
**Last Updated:** January 26, 2026
**Next Milestone:** Alpha Demo Complete

---

## Vision

**"Production-ready infrastructure in hours, not months."**

Unheaded delivers the complete "suit of armor" for modern SaaS applications:
- eBPF-based observability (L2-L7)
- Immutable NixOS infrastructure
- Zero customer data access
- Compliance templates (FEDRAMP, SOC2, etc)
- Drop-in deployment

---

## Phases

### Phase 0: Foundation (COMPLETE ✓)
**Timeline:** Dec 2025 - Jan 2026
**Status:** SHIPPED

#### Milestone: Busboy Message Bus
- [x] Ring buffer implementation
- [x] Pub/sub engine
- [x] gRPC streaming
- [x] REST control plane
- [x] Prometheus metrics
- [x] Rate limiting & circuit breakers
- [x] CLI client
- [x] Documentation

**Outcome:** github.com/unheaded/busboy Phase 1 complete and battle-tested.

---

### Phase 1: Alpha (IN PROGRESS 🚀)
**Timeline:** Jan 26 - Feb 8, 2026 (REVISED - realistic parallel execution)
**Status:** 25% COMPLETE (honest assessment)

**Goal:** Demonstrate core platform capabilities with eBPF tracing, microservices, and self-hosting proof.

**CRITICAL UPDATE:** Complete reality check conducted Jan 27. Previous estimates were optimistic (34k LOC claimed vs 1.2k actual). With honest assessment and aggressive parallel execution, alpha delivery revised to **Feb 8, 2026** (realistic) instead of Feb 15 (optimistic).

---

#### Milestone 1.1: eBPF Foundation (RESTARTED - Honest Reset)
**Original ETA:** Jan 31, 2026
**Revised ETA:** Feb 3-4, 2026
**Status:** 0% functional (was incorrectly marked 10%)

- [ ] Write packet_marker.bpf (Rust + Aya XDP) - 1.5d
- [ ] Write flow_tracker.bpf (kprobes) - 1.5d
- [ ] Write latency_probe.bpf (tracepoints) - 1d
- [ ] Implement trace-collector (Rust bridge) - 1d
- [ ] Kernel verifier testing - 0.5d
- [ ] Test eBPF programs on bare metal - 0.5d
- [ ] Integrate with Busboy - 0.5d

**Dependencies:** None (can parallelize with 1.2-1.3)
**Owner:** Agent 5 (Architect - Rust focus)
**Risk:** Medium (kernel verifier issues likely)

#### Milestone 1.2: Control Plane (RESTARTED - Honest Reset)
**Original ETA:** Feb 3, 2026
**Revised ETA:** Feb 4-5, 2026
**Status:** 0% functional

- [ ] Implement unheaded-daemon skeleton (Go) - 2d
- [ ] LXD orchestration client - 1d
- [ ] State machine (desired vs actual) - 1d
- [ ] eBPF loader integration - 0.5d
- [ ] Drift detection (poll every 30s) - 0.5d
- [ ] Health monitoring endpoints - 0.5d

**Dependencies:** None (parallelize with eBPF + services)
**Owner:** Agent 7 (Architect - Go focus)
**Risk:** Medium

#### Milestone 1.3: Microservices (RESTARTED - Honest Reset)
**Original ETA:** Feb 7, 2026
**Revised ETA:** Feb 4-5, 2026
**Status:** 0% functional (structure only)

**Parallel agent execution (4 agents, 1 service each):**

- [ ] **Agent 1: timeguru service** (timeline REST API) - 1.5d
  - Read timeline.md, serve JSON/YAML
  - /api/v1/timeline endpoint
  - Busboy integration

- [ ] **Agent 2: captain service** (strategy stub) - 1d
  - Basic REST API
  - Busboy subscription to alerts
  - Metrics export

- [ ] **Agent 3: micromanager service** (exec stub) - 1d
  - Task execution API
  - Progress tracking
  - Busboy event publishing

- [ ] **Agent 4: architect service** (design stub) - 1d
  - Design review API
  - Technical decision tracking
  - Busboy message consumption

**Dependencies:** None (all parallel)
**Owner:** 4 agents (Micromanager coordinates)
**Risk:** Low

#### Milestone 1.4: Container Stack (RESTARTED - Honest Reset)
**Original ETA:** Feb 10, 2026
**Revised ETA:** Feb 5-6, 2026
**Status:** 0% (structure only, no executable configs)

- [ ] Create NixOS flake structure - 1d
- [ ] Container definition templates (per service) - 1d
- [ ] Hardening modules (seccomp, capabilities, read-only FS) - 0.5d
- [ ] Network policies (default deny + explicit allow) - 0.5d
- [ ] Gateway configuration (nginx HTTP/3) - 1d
- [ ] Test container build pipeline - 0.5d

**Dependencies:** Services (1.3) must have APIs defined
**Owner:** Agent (Architect - NixOS focus)
**Risk:** Low (declarative work)

#### Milestone 1.5: Dashboard & Backend (RESTARTED - Honest Reset)
**Original ETA:** Feb 12, 2026
**Revised ETA:** Feb 6-7, 2026
**Status:** 10% (kanban-app 60% done, dashboard-backend 0%)

- [ ] dashboard-backend (Go + WebSocket) - 1.5d
  - Metrics aggregation from services
  - Trace correlation
  - WebSocket for live updates

- [ ] Dashboard UI (packet flow viz) - 1d
  - Real-time metrics display
  - Trace timeline viewer
  - bellis.tech-inspired design

- [ ] Kanban app integration (already 60% done) - 0.5d
  - Wire to timeguru service
  - Interactive board rendering
  - Live indicators

**Dependencies:** Microservices (1.3) publishing metrics
**Owner:** Agent 8 (dashboard) + kanban owner
**Risk:** Low (kanban already mostly done)

#### Milestone 1.6: The Meta Moment - Alpha Demo (FINAL)
**Original ETA:** Feb 15, 2026
**Revised ETA:** Feb 8, 2026
**Status:** 0% integration (components exist, not connected)

- [ ] Wire Kanban app to timeguru service - 0.5d
- [ ] Integrate dashboard with metrics - 0.5d
- [ ] E2E testing (browser → gateway → services → Busboy) - 1d
- [ ] Security validation (zero customer data access) - 0.5d
- [ ] Public accessibility (firewall rules, DNS) - 0.5d
- [ ] Demo video & documentation - 1d

**Dependencies:** Milestones 1.1-1.5 (all required)
**Owner:** Captain + Timeguru
**Risk:** Low (integration phase)

**ALPHA COMPLETE (REALISTIC):** Feb 8, 2026

---

### Phase 2: Beta (PLANNED)
**Timeline:** Feb 16 - Mar 31, 2026
**Status:** Not started

**Goal:** Production hardening, testing, and first customer pilot.

#### Milestones:
- [ ] Comprehensive test coverage (unit, integration, load)
- [ ] Message persistence (WAL)
- [ ] Multi-node clustering
- [ ] Advanced monitoring (distributed tracing)
- [ ] Security audit
- [ ] Performance benchmarking
- [ ] First customer pilot

**ETA:** Mar 31, 2026

---

### Phase 3: MVP (PLANNED)
**Timeline:** Apr 1 - Jun 30, 2026
**Status:** Not started

**Goal:** Minimum viable product ready for paying customers.

#### Milestones:
- [ ] Compliance templates (SOC2, NIST)
- [ ] Multi-cloud support (AWS, Azure, GCP)
- [ ] API gateway features
- [ ] Self-healing infrastructure
- [ ] Customer dashboard
- [ ] Billing integration
- [ ] Documentation overhaul

**ETA:** Jun 30, 2026

---

### Phase 4: Scale (PLANNED)
**Timeline:** Jul 1 - Dec 31, 2026
**Status:** Not started

**Goal:** Scale to 100 customers, full compliance suite.

#### Milestones:
- [ ] FEDRAMP baseline
- [ ] HIPAA compliance
- [ ] PCI-DSS compliance
- [ ] Geographic distribution
- [ ] Advanced analytics
- [ ] ML-based anomaly detection
- [ ] Partner ecosystem

**ETA:** Dec 31, 2026

---

## Current Sprint (Week of Jan 27, 2026) - AGGRESSIVE PARALLEL EXECUTION

### REALITY CHECK COMPLETE (Jan 27)
- [x] Ground truth audit: actual codebase (1,234 LOC, not 34k)
- [x] Honest milestone assessment
- [x] Critical path analysis
- [x] Resource planning for parallel agents
- [x] Risk identification and mitigation

### STARTING TODAY - PARALLEL AGENT DEPLOYMENT
**Maximum parallelization model:**
- [ ] Agent 1-4: Microservices (timeguru, captain, micromanager, architect)
- [ ] Agent 5: eBPF programs (packet_marker, flow_tracker, latency_probe)
- [ ] Agent 6: trace-collector implementation
- [ ] Agent 7: unheaded-daemon (LXD orchestration, state mgmt)
- [ ] Agent 8: dashboard-backend + dashboard UI
- [ ] Kanban owner: Kanban app integration (60% → 100%)

### BLOCKED
None - all components have clear interfaces

### CRITICAL PATH (must complete by dates to stay on track)
- [ ] Feb 3: timeguru + eBPF packet_marker working
- [ ] Feb 5: All services wired to Busboy, daemon basic, containers building
- [ ] Feb 8: Full integration complete, demo ready

---

## Metrics - HONEST ASSESSMENT (Jan 27, 2026)

### Alpha Progress (REVISED TO REALITY)
| Component | Previous | Honest | Target (Feb 8) |
|-----------|----------|--------|----------------|
| **eBPF** | 10% | 0% functional | 90% |
| **Control Plane** | 0% | 0% | 80% |
| **Microservices** | 0% | <5% | 85% |
| **Containers** | 0% | 0% | 80% |
| **Dashboard** | 0% | 10% | 85% |
| **Meta Moment** | 0% | 60% (kanban) | 100% |
| **Overall** | 25% claimed | ~8% actual | **95%** |

### Team Velocity (Post-Busboy)
- **Previous:** 615 LOC/day (serial execution)
- **Current:** 3,075 LOC/week with Busboy (parallel possible)
- **Target:** 6-8 parallel agents × 500 LOC/day = 3,000-4,000 LOC in 5-6 days
- **Verdict:** Achievable if execution is parallel

### Risk Assessment (UPDATED)
| Risk | Likelihood | Impact | Mitigation | Owner |
|------|-----------|--------|------------|-------|
| eBPF kernel verifier failure | Medium | High | Start immediately, Rust safety | Agent 5 |
| LXD networking misconfiguration | Medium | High | Pre-test bridge + firewall | Agent 7 |
| Service integration failures | Low | High | Busboy as single source, clear interfaces | All |
| Container build failures | Low | Medium | NixOS templates, pre-test | Architect |
| Dashboard UI not ready | Low | Medium | Kanban already 60%, parallel work | Agent 8 |
| **Timeline slip risk** | **Medium** | **High** | **Aggressive parallel only** | Timeguru |

### Dependencies & Critical Blockers
```
eBPF (1.1) ─┐
            ├─→ trace-collector ─→ Dashboard → Meta Moment (1.6)
Services (1.3) ─→ Containers (1.4) ──↗
Daemon (1.2) ────↗
```

**Non-blocking path:** Services can develop independently if Busboy interfaces are defined early

---

## Wins 🎉

### January 2026
- ✅ Busboy Phase 1 shipped!
- ✅ Unheaded repo structure created
- ✅ Architecture document complete
- ✅ Fully automated setup script
- ✅ Clear timeline with milestones

### December 2025
- ✅ Busboy prototype validated
- ✅ gRPC streaming working
- ✅ Message bus pattern proven

---

## Lessons Learned (Post-Reality Check)

### What's Working
- **Go for services:** Fast iteration, great tooling
- **Rust for eBPF:** Safety + performance critical
- **NixOS for containers:** Declarative wins
- **Busboy architecture:** Proven in Phase 1 ✅
- **AI pair programming:** Massive velocity boost
- **Clear architecture BEFORE coding:** Prevents rework

### What Went Wrong (Honest Assessment)
- **LOC estimates were wildly off:** 34k claimed vs 1.2k actual (96% variance)
- **"10% complete" misleading:** Skeleton ≠ progress
- **Optimistic timelines:** Didn't account for eBPF complexity
- **Sequential thinking:** Should have planned parallel execution from start
- **Skeleton code as progress:** TODO stubs aren't 10% done

### What to Improve
- **Testing discipline:** Need TDD from day one (CRITICAL)
- **Realistic estimates:** Base on actual implementation, not just scaffolding
- **Parallel by default:** Don't sequence when components are independent
- **Daily honest updates:** Update Timeguru daily with actual progress %
- **Risk flags early:** eBPF dev environment validation on Day 1

---

## Team

### Core Contributors
- **Muck** - Lead SRE, infrastructure design
- **Architect** - Staff-level fusion (4 architects)
- **Micromanager** - Engineering leadership, QA
- **Captain** - Strategy, vision, leadership
- **Timeguru** - Timeline tracking, milestones

### AI Augmentation
- Claude (Sonnet 4.5) - Pair programming, design
- Skills framework - Specialized expertise

---

## Contact & Links

- **GitHub:** https://github.com/unheaded/unheaded
- **Website:** https://unheaded.com (coming soon)
- **Email:** hello@unheaded.com
- **Busboy:** https://github.com/unheaded/busboy

---

## Notes

This timeline is a **living document**. Updated continuously as we ship.

**Philosophy:**
- Ship early, iterate fast
- Security first, always
- We drink our own champagne
- Radical transparency

---

**Last Major Update:** Jan 27, 2026 - Reality check & aggressive parallelization plan
**Previous Naive Target:** Feb 15, 2026
**Revised Realistic Target:** Feb 8, 2026 (with 7-8 parallel agents)
**Next Review:** Jan 28, 2026 (daily during execution phase)

---

## 🚨 EXECUTION DECISION REQUIRED

**Status:** Timeguru has completed honest reality check. Awaiting GO/NO-GO from leadership.

**Two paths:**

### Path A: Aggressive Parallel (RECOMMENDED)
- **Start:** Today (Jan 27)
- **Execution:** 7-8 Claude agents in parallel
- **Delivery:** Feb 8, 2026 (realistic)
- **Risk:** Medium (eBPF complexity)
- **Reward:** Alpha with self-hosted demo 7 days early

### Path B: Conservative Sequential
- **Start:** Today (Jan 27)
- **Execution:** 2-3 agents sequentially
- **Delivery:** Late Feb / early Mar (safe)
- **Risk:** Low (predictable)
- **Reward:** Higher confidence, lower stress

**Muck's call.** Timeguru is ready either way.
