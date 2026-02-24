# Roadmap

## Current State: Alpha (~99% Complete)

**As of February 23, 2026 (Sprint S33)**

The Unheaded Kingdom has reached alpha feature completeness with all core systems operational:

| Component | Status |
|-----------|--------|
| 8 microservices (timeguru, captain, architect, micromanager, monad, sophia, dashboard-backend, kanban-app) | Operational, communicating via Wotan |
| eBPF Doom proof of concept | 559+ frames, 819M+ instructions, zero halts |
| NixOS container definitions | Complete for all services |
| Docker Compose full stack | Complete |
| Gateway routing | Complete for all services |
| Dashboard (packet flow + metrics) | Operational |
| Kanban app (Meta Moment) | Operational |
| Control plane (unheaded-daemon) | Operational with Wotan + reconciliation |
| Test suite | 293 Rust tests + 135 Go packages, zero failures |
| Codebase | ~260K production LOC (~464K w/ tests) |
| End-to-end tests | 23/23 passing |

---

## Sprint S33: Hardening Sprint (Feb 23 - Mar 7, 2026)

### Workstream Status

| Workstream | Agent | Timeline | Status |
|------------|-------|----------|--------|
| **WS1:** Doom-Bridge Service | Agent A (Developer/Architect) | Feb 24-26 | In progress |
| **WS3:** Scaling Profiler (15+ fps) | Agent B (Developer/Scientist) | Feb 24-27 | In progress |
| **WS2:** Lich Security Campaigns D1-D6 | Agent D (BlackMage) | Mar 1-5 | Scheduled |
| **WS4:** Documentation + Wiki + Conference | Agent E (Captain) | Mar 1-7 | In progress |
| **P0/P1 Quick Wins** | Agent C (Coordinator) | Feb 23-25 | In progress |
| **P1 Security Backlog** | Agent F (Developer) | Mar 1-5 | Scheduled |

### WS1: Doom-Bridge Service

Build `cmd/doom-bridge/` -- a Go service that reads the Doom framebuffer from BPF SCREEN_MAP, applies the Doom palette, and streams frames via WebSocket to browser clients. The dashboard will show live Doom gameplay at the current ~6 fps (improving to 15+ fps with WS3).

**Definition of Done:** Browser shows live Doom frames (title screen through demo cycle).

### WS3: Scaling Profiler

Profile and optimize the packet injection pipeline to achieve 15+ fps sustained with zero corruption. Test three hypotheses: (H1) timing bounds allow faster injection, (H2) Netflix-model burst injection improves throughput, (H3) Go injector outperforms Python.

**Definition of Done:** 15+ fps sustained for 60 seconds, zero corruption, Netflix burst model proven.

### WS2: Lich Security Campaigns

Execute 6 targeted security campaigns (D1-D6) against the live Doom PoC: ROM injection, framebuffer exfiltration, keyboard injection, flow label collision, SYSCALL fuzzing, and ROM TOCTOU. Document findings and merge critical fixes.

**Definition of Done:** All 6 campaigns executed, critical findings fixed, documented.

### WS4: Documentation + Wiki + Conference

Create public-facing wiki, wiki HTTP server, and conference talk outline. Document the Doom-over-IPv6 narrative, architecture, bug kill chain, performance, and security posture.

**Definition of Done:** Wiki browsable at `/wiki/`, conference outline complete, timeline updated.

---

## Upcoming Milestones

### Round Table Reconvenes (Mar 8, 2026)

All workstreams (WS1-WS4) complete. Full assembly reviews:
- Lich D1-D6 security findings
- WS3 performance profiling results
- Wiki and conference documentation
- Doom-bridge architecture proof

**Output:** 200+ step WS5 battle plan for production packet tracing pipeline.

### WS5: Return to Core (Mar 8 onwards, 2-3 weeks)

The main event. WS5 builds the production packet tracing pipeline using the same eBPF infrastructure proven by Doom:

- **packet_marker:** Inject trace IDs at XDP layer (replaces Doom ROM execution)
- **flow_tracker:** Per-flow connection tracking (replaces Doom RAM)
- **latency_probe:** RTT measurement (replaces Doom timing)
- **trace-collector:** eBPF ring buffer to Wotan pipeline
- **dashboard integration:** Real packet traces in the dashboard (replaces Doom framebuffer)

WS5 is the product. Everything before WS5 was proof that the architecture works.

### Service Breakout (Post-Alpha, Target: Mar 15)

After stable alpha, the monorepo splits into individual service repositories. Each service becomes an independent Go module imported by `github.com/unheaded/unheaded`. See `docs/SERVICE_BREAKOUT_STRATEGY.md` for the full plan.

---

## Future Phases

### Beta (Target: Q2 2026)

- Production packet tracing pipeline operational
- Real customer infrastructure (not Doom)
- Multi-tenant isolation validated
- Performance: sub-50ms latency (packet to browser)
- Container start time: < 10 seconds
- Load tested: 1000 req/s sustained

### Release (Target: Q3 2026)

- Public launch
- Compliance certifications (SOC 2, FedRAMP)
- IaC output backends validated (Ansible, Terraform, Puppet, Kubernetes, Chef, Salt)
- Observability backend integration (Prometheus, Grafana, ELK, Jaeger)
- Self-service onboarding

### Scale (Target: Q4 2026+)

- Multi-region deployment
- 800+ service mesh support
- Custom Wotan-native observability backends (replace third-party defaults)
- Full compliance portfolio (HIPAA, PCI-DSS, ITAR, GDPR)
- Enterprise features

---

## Key Dates

| Date | Event |
|------|-------|
| Jan 20, 2026 | First commit |
| Feb 3, 2026 | Alpha feature complete (all 8 services) |
| Feb 22, 2026 | Doom proven (559 frames, 819M instructions) |
| Feb 23, 2026 | S33 hardening sprint begins |
| Feb 28, 2026 | WS1 + WS3 complete (target) |
| Mar 5, 2026 | WS2 + WS4 complete (target) |
| Mar 8, 2026 | Round Table reconvenes, WS5 kickoff |
| Mar 15, 2026 | Service breakout complete (target) |
| Mar 31, 2026 | WS5 complete, production tracing operational (target) |

---

*See also: [Architecture](architecture.md) | [Security](security.md) | [Doom over IPv6](doom-over-ipv6.md)*
