# The Unheaded Chronicles

## A Living Grimoire of the Kingdom's Journey

**STATUS:** ALPHA
**LAST UPDATED:** February 24, 2026
**LOC:** ~260K production (~464K with tests) — 220K Go, 203K Go tests, 16K Rust, 13K JS, 5K Nix, 7K scripts

---

### Age 0: The Foundation Stone (✅ COMPLETED)

### Age 1: The Alpha Ascension (🔄 IN PROGRESS)

**Progress:** ~98% — all services operational, S36 four pillars complete (ports, gRPC-first, logging, discovery), eBPF blocked on bare metal

### Age 2: The Beta Trials (📋 PLANNED)

### Age 3: The MVP Era (📋 PLANNED)

### Age 4: The Scaling Era (📋 PLANNED)

## Milestones

### 🔄 The Whispering Void Awakens

**ETA:** Feb 3, 2026
**Owner:** The Architect's rust-touched agents
**Risk:** Medium — blocked on bare metal Linux environment
**Progress:** 55%
**Status:** in progress (code written, untested on hardware)

**Tasks:**
- [ ] `packet_marker.bpf` - Trace ID injection at XDP layer (Rust + Aya)
- [ ] `flow_tracker.bpf` - Connection tracking via kprobes
- [ ] `latency_probe.bpf` - RTT measurement via tracepoints
- [ ] `trace-collector` - Rust bridge from kernel to Fae Chamber
- [ ] Kernel verifier trials (the ancient tests)
- [ ] Bare metal communion (live testing)
- [ ] Fae Chamber integration (Wotan connection)

### 🔄 The Cuirass Takes Form

**ETA:** Feb 4, 2026
**Owner:** The Architect's Go-touched agents
**Risk:** Low
**Progress:** 75%
**Status:** in progress (core done, real LXD + eBPF integration pending)

**Tasks:**
- [ ] `unheaded-daemon` skeleton (Go) - COMPLETE (main.go + internal packages)
- [ ] LXD orchestration client - communion with container spirits (mock ready)
- [ ] State machine (desired vs actual) - the truth detector (state.go)
- [ ] eBPF loader interface - awakening the Whispering Void (mock ready)
- [ ] Drift detection - polling every 30 heartbeats (reconciliation loop)
- [ ] Health monitoring endpoints - the vital signs (/health, /ready, /metrics)
- [ ] Real LXD integration - awaiting live testing
- [ ] Real eBPF integration - awaiting Whispering Void programs

### 🔄 The Royal Court Assembles

**ETA:** Feb 4, 2026
**Owner:** Four Cavalry agents (Micromanager coordinates)
**Risk:** Low
**Progress:** 85%
**Status:** in progress (APIs scaffolded, Wotan wiring + prophecy engine pending)

**Tasks:**
- [ ] Timeline REST API - serves the living roadmap
- [ ] `/api/v1/timeline` - JSON/YAML/Markdown mirrors
- [ ] Markdown parser with regex extraction
- [ ] File watcher for auto-reload
- [ ] Prophecy engine connection (future)
- [ ] Strategy REST API - vision endpoints
- [ ] Health monitoring
- [ ] Scaffold complete with Kingdom branding
- [ ] Alert subscription - Wotan connection (wiring)
- [ ] Task execution API - WHAT & WHEN
- [ ] Progress tracking scaffold
- [ ] Health monitoring
- [ ] Event publishing - Fae Chamber dance (wiring)
- [ ] Design review API scaffold
- [ ] ADR management structure
- [ ] Health monitoring
- [ ] Wisdom storage connection (future)

### 🔄 The Citadels Rise

**ETA:** Feb 5, 2026
**Owner:** The Architect + The Developer
**Risk:** Low
**Progress:** 75%
**Status:** in progress (NixOS + Docker done, IaC renderers + observability adapters deferred not killed)

**Tasks:**
- [ ] NixOS flake structure - the master blueprint (reference implementation)
- [ ] Container definition templates (per service citadel, runtime-agnostic)
- [ ] Hardening modules (seccomp, capabilities, read-only FS — all runtimes)
- [ ] Network policies (default deny - The Closed Gate)
- [ ] Gateway configuration (nginx HTTP/3 - The Main Gate)
- [ ] Container build pipeline testing (all runtimes)
- [ ] Runtime abstraction layer in unheaded-daemon
- [ ] `IaCRenderer` interface in `pkg/iac/` — common output contract
- [ ] Ansible renderer — playbook + role generation from desired state
- [ ] Terraform renderer — HCL module generation from desired state
- [ ] Puppet renderer — manifest generation from desired state
- [ ] Kubernetes renderer — manifest + Helm chart generation
- [ ] Chef renderer — cookbook generation from desired state
- [ ] Salt renderer — state file generation from desired state
- [ ] Integration tests — each renderer produces valid, lintable output
- [ ] `unheaded generate --backend=ansible|terraform|puppet|k8s|chef|salt`
- [ ] `ObservabilityAdapter` interface in `pkg/observability/` — common signal contract
- [ ] `observability/prometheus/` — Prometheus scrape config + adapter
- [ ] `observability/grafana/` — Grafana dashboard JSON + datasource provisioning
- [ ] `observability/elk/` — Logstash pipeline + Kibana index patterns + ES templates
- [ ] `observability/fluentd/` — Fluent Bit / Fluentd config for log forwarding
- [ ] `observability/jaeger/` — Jaeger collector config + trace export adapter
- [ ] `observability/nagios/` — Nagios check configs + NRPE integration
- [ ] `observability/flume/` — Apache Flume agent + channel configs
- [ ] `observability/loki/` — Loki + Promtail config for log aggregation
- [ ] `observability/alertmanager/` — Alert rules + routing config
- [ ] Integration tests — each adapter ships valid, working config
- [ ] `unheaded observe --backend=prometheus|grafana|elk|jaeger|nagios|all`

### ✅ The Cape & Cloak Emerge

**ETA:** Jan 30, 2026
**Owner:** Developer + Micromanager QA*
**Risk:** Low
**Progress:** 90%
**Status:** completed

**Tasks:**
- [ ] `dashboard-backend` (Go + WebSocket) - metrics aggregation
- [ ] Dashboard UI - packet flow visualization
- [ ] **Task:** Create Kanban column components
- [ ] **Task:** Create task card components
- [ ] **Task:** Implement card drag-and-drop
- [ ] **Task:** Wire frontend to Timeguru service
- [ ] **Task:** Implement task management
- [ ] **Task:** Establish real-time connection
- [ ] **Task:** Subscribe to task events via Wotan
- [ ] **Task:** Handle update conflicts
- [ ] **Task:** Apply Kingdom aesthetic
- [ ] **Task:** Ensure accessibility compliance
- [ ] **Task:** Embed frontend in Go binary
- [ ] **Task:** Verify full flow works
- [ ] Code complete and compiles
- [ ] Unit tests pass (if applicable)
- [ ] Integration test pass (if applicable)
- [ ] Security review: **ZERO user data access** ✓
- [ ] No external dependencies (Kingdom code only)
- [ ] Documentation updated (if API change)
- [ ] Code reviewed (or self-reviewed with rationale)
- [ ] Merged to main
- [ ] Works in browser (Chrome, Firefox, Safari)
- [ ] All 5 features DONE
- [ ] E2E smoke test passes (manual verification pending)
- [ ] Kanban shows THIS timeline
- [ ] Real-time updates working
- [ ] Deployed on Kingdom infrastructure (pending)
- [ ] **THE META MOMENT ACHIEVED** (pending final verification) 🎉

### 📋 Alpha Demonstration

**ETA:** TBD (post bare metal eBPF)
**Owner:** Captain + Timeguru + the assembled Court
**Risk:** Medium — depends on eBPF bare metal verification
**Progress:** 10%
**Status:** planned (S36 four pillars COMPLETE, blocked on eBPF verification + public deployment)

**Tasks:**
- [ ] Kanban Frontend COMPLETE (P0 Epic above)
- [ ] Integrate dashboard with metrics - 0.5 days
- [ ] E2E testing (browser → gateway → services → Fae Chamber) - 1 day
- [ ] Security validation (The Sacred Law verified) - 0.5 days
- [ ] Public accessibility (firewall rules, DNS, the Grand Opening) - 0.5 days
- [ ] Demo video & documentation (The Chronicle Recording) - 1 day

---

### S35 Strategic Direction (Feb 24, 2026)

**Licensing:** MIT short-term → permissive at stable/K8s-scale. Protocol specs separately permissive.
**Doom:** Fork official id-Software/DOOM (with sound). Move out of repo before public.
**SBOM:** ScanCode + FOSSology + ORT — run tonight, fold results into repo.
**Backends:** DEFERRED not killed. Anti-lock-in core principle. Prometheus + zerolog ship first.
**Inverse Mask:** Deep exploration session (BlackMage + Developer + Architect + Scientist).
**VC:** Austin venture capital exploration while private. Protocol IS the moat.
**Priority:** ~~S34 pillars~~ (DONE — S36) → LICENSE file → SBOM → bare metal eBPF → inverse mask → IANA → VC → demo video.

---

### S36 Four Pillars — COMPLETE (Feb 24, 2026)

All four infrastructure pillars planned in S34, executed via multi-agent sprint in S36:

1. **Port Authority (Pillar 1):** All 20+ services migrated to Doom Range (16666-26666). `pkg/ports/ports.go` is single source of truth. `configs/port-registry.yaml` for container configs. Zero port conflicts.
2. **gRPC-First Transport (Pillar 2):** `pkg/transport/` implements gRPC-first cascade with HTTP fallback. All 10 services wired. Dual health checks (gRPC + HTTP). DEGRADED state detection.
3. **Log Aggregation (Pillar 3):** `pkg/logagg/` with zerolog hook publishing to Wotan `logs.<service>.<level>` topics. 10K-line ring buffer per service. Dashboard endpoints: `GET /api/v1/logs`, `WebSocket /ws/logs`.
4. **Service Discovery (Pillar 4):** `pkg/discovery/` with convention-based + port-scan + Wotan registration. `configs/services.yaml` static fallback. Zero hardcoded IPs in production code.

---

*Synced: 2026-02-24 UTC*
