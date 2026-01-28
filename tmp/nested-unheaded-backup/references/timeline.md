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
**Timeline:** Jan 26 - Feb 15, 2026
**Status:** 25% COMPLETE

**Goal:** Demonstrate core platform capabilities with eBPF tracing, microservices, and self-hosting proof.

#### Milestone 1.1: eBPF Foundation (IN PROGRESS)
**ETA:** Jan 31, 2026
**Status:** 10% complete

- [ ] Write packet_marker.bpf (Rust)
- [ ] Write flow_tracker.bpf (Rust)
- [ ] Write latency_probe.bpf (Rust)
- [ ] Implement trace-collector (Rust)
- [ ] Test eBPF programs on bare metal
- [ ] Integrate with Busboy

**Blockers:** None
**Owner:** Architect + Muck

#### Milestone 1.2: Control Plane (PENDING)
**ETA:** Feb 3, 2026
**Status:** Not started

- [ ] Implement unheaded-daemon (Go)
- [ ] LXD orchestration
- [ ] State management (desired vs actual)
- [ ] eBPF loader
- [ ] Drift detection
- [ ] Health monitoring

**Blockers:** eBPF foundation (1.1)
**Owner:** Architect

#### Milestone 1.3: Microservices (PENDING)
**ETA:** Feb 7, 2026
**Status:** Not started

- [ ] Create timeguru service (Go)
- [ ] Create captain service (Go)
- [ ] Create micromanager service (Go)
- [ ] Create architect service (Go)
- [ ] Wire all services to Busboy
- [ ] Implement triple format (MD/JSON/YAML)

**Blockers:** None (can parallel with 1.2)
**Owner:** Micromanager

#### Milestone 1.4: Container Stack (PENDING)
**ETA:** Feb 10, 2026
**Status:** Not started

- [ ] NixOS flake structure
- [ ] Container definitions (all services)
- [ ] Hardening modules (seccomp, capabilities)
- [ ] Network policies
- [ ] Gateway configuration (nginx HTTP/3)

**Blockers:** Microservices (1.3)
**Owner:** Architect

#### Milestone 1.5: Dashboard (PENDING)
**ETA:** Feb 12, 2026
**Status:** Not started

- [ ] dashboard-backend (Go + WebSocket)
- [ ] Dashboard UI (packet flow viz)
- [ ] Metrics aggregation
- [ ] Trace correlation viewer
- [ ] bellis.tech-inspired design

**Blockers:** Container stack (1.4)
**Owner:** Captain + Muck

#### Milestone 1.6: The Meta Moment (PENDING)
**ETA:** Feb 15, 2026
**Status:** Not started

- [ ] Kanban app (Go + vanilla JS)
- [ ] Read timeline.md from timeguru
- [ ] Render interactive board
- [ ] Write dummy config.yaml
- [ ] Public accessibility
- [ ] Record demo video

**Blockers:** Dashboard (1.5)
**Owner:** Captain + Timeguru

**ALPHA COMPLETE:** Feb 15, 2026

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

## Current Sprint (Week of Jan 26, 2026)

### IN PROGRESS
- [x] Project structure creation
- [x] Documentation (README, ARCHITECTURE, this timeline)
- [x] Setup script (setup-host.sh)
- [ ] eBPF programs (packet_marker, flow_tracker, latency_probe)

### BLOCKED
None

### NEXT UP
- trace-collector implementation
- unheaded-daemon skeleton
- timeguru service

---

## Metrics

### Alpha Progress
- **Overall:** 25%
- **eBPF:** 10%
- **Control Plane:** 0%
- **Microservices:** 0%
- **Containers:** 0%
- **Dashboard:** 0%
- **Meta Moment:** 0%

### Team Velocity
- **Sprint length:** 1 week
- **Story points completed:** 15 (last sprint was Busboy Phase 1)
- **Estimated velocity:** 20-25 points/sprint

### Risk Assessment
| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| eBPF complexity | Medium | High | Start early, Rust safety |
| NixOS learning curve | Medium | Medium | Parallel learning track |
| Timeline slip | Low | Medium | Buffer built into estimates |
| Scope creep | Medium | High | Strict milestone gates |

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

## Lessons Learned

### What's Working
- **Go for services:** Fast iteration, great tooling
- **Rust for eBPF:** Safety + performance critical
- **NixOS for containers:** Declarative wins
- **Busboy architecture:** Proven in Phase 1
- **AI pair programming:** Massive velocity boost

### What to Improve
- **Testing discipline:** Need TDD from day one
- **Documentation:** Keep it up to date
- **Sprint planning:** Better task breakdown

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

**Last Commit:** Initial timeline structure
**Next Review:** Feb 1, 2026
