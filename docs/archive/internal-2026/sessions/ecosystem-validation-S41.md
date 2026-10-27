# Ecosystem Validation Report -- S41 Phase 6

**Date:** 2026-02-24
**Sprint:** S41 Kingdom Hardening
**Phase:** 6 -- Validate Ecosystem Comparison Topics
**Scope:** Cross-reference all technologies from the 4 comparison analyses (Flink, K8s/Cilium/Flannel, Coroot, full eBPF ecosystem) against codebase presence.

---

## Methodology

Searched `docs/`, `pkg/`, `cmd/`, `services/`, `ebpf/`, `configs/`, `containers/`, `nix/`, and `wiki/` for references to each technology. Status determined by:

- **PRESENT** -- Code, config, or integration exists in the codebase
- **PLANNED** -- Referenced in timeline, roadmap, or battle plans but no implementation
- **DOCUMENTED** -- Mentioned in docs/architecture but no code or adapter config
- **NOT YET** -- Not found in codebase at all
- **NOT APPLICABLE** -- Technology is a competitor/alternative, not an integration target

---

## 1. Networking

| Technology | Status | Evidence | Notes |
|------------|--------|----------|-------|
| **Cilium** | DOCUMENTED | `LICENSES/THIRD_PARTY.md`, `docs/adr/ADR-003`, S41 battle plan | cilium/ebpf Go library referenced for userspace; actual cilium CNI not used (own XDP stack) |
| **Flannel** | DOCUMENTED | S41 battle plan references only | Comparison target, not integration |
| **Calico** | NOT YET | Only in S41 battle plan category list | No references in code or docs |
| **Katran (Meta XDP LB)** | NOT YET | Only in S41 battle plan category list | Own load balancer in `pkg/loadbalancer/`, `pkg/mesh/loadbalancer/`, `services/pauldrons/` |
| **Envoy** | DOCUMENTED | `cmd/trace-collector/tests/`, `docs/protocol/PROTOCOL_MATH_AND_MAPS.md`, `docs/conference/TALK-OUTLINE.md` | Used as comparison benchmark; explicitly replaced by Wotan mesh. Test fixtures use "envoy" as mock service name. |
| **Istio** | DOCUMENTED | `docs/protocol/PROTOCOL_FOUNDATION.md` | Mentioned as replaced by wire-level mesh |
| **Linkerd** | NOT YET | No references found | Not mentioned anywhere |
| **Service Mesh (own)** | PRESENT | `pkg/mesh/` (discovery, load balancing, circuit breaking, mTLS, routing) | Full implementation: Hauberk service mesh |
| **Load Balancing (own)** | PRESENT | `pkg/loadbalancer/`, `pkg/mesh/loadbalancer/`, `services/pauldrons/`, `services/gateway/proxy/loadbalancer.go` | L4/L7, round-robin, least-conn, weighted, random, adaptive, P2C, zone-aware, Maglev |
| **Network Policy (own)** | PRESENT | `pkg/network/policy_controller.go`, `nix/modules/`, container configs | Firewall rules, seccomp, namespace isolation |
| **DNS/Discovery (own)** | PRESENT | `pkg/dns/`, `pkg/discovery/` | Custom DNS + 4-layer service discovery |

## 2. Observability

| Technology | Status | Evidence | Notes |
|------------|--------|----------|-------|
| **Coroot** | DOCUMENTED | S41 battle plan strategic position statement | Comparison target for dashboard UI; no integration |
| **Hubble (Cilium)** | DOCUMENTED | S41 battle plan | Comparison target for flow visualization |
| **Pixie** | NOT YET | Only in S41 battle plan category list | No references in code or docs |
| **Prometheus** | PRESENT | `cmd/dashboard-backend/internal/scraper/`, all services expose `/metrics`, `containers/containerd/config.toml` | Prometheus-format metrics scraping implemented; all 10 services emit Prometheus metrics |
| **Grafana** | PLANNED | `references/timeline.md`, `wiki/Observability-Backends.md`, `containers/README.md` | Docker Compose includes Grafana; adapter config DEFERRED |
| **Jaeger** | PLANNED | `references/timeline.md`, `wiki/Observability-Backends.md` | Listed in timeline as `observability/jaeger/`; no adapter config yet |
| **Zipkin** | PLANNED | `wiki/Observability-Backends.md`, `docs/sessions/HANDOFF_2026-02-09_S8.md` | Listed as supported tracing backend; no adapter config |
| **Tempo** | PLANNED | `wiki/Observability-Backends.md` | Listed as supported tracing backend; no adapter config |
| **ELK (Elasticsearch/Logstash/Kibana)** | PLANNED | `wiki/Observability-Backends.md`, `references/timeline.md` | Listed in timeline; no adapter config directory exists |
| **Fluentd / Fluent Bit** | PLANNED | `wiki/Observability-Backends.md` | Listed as logging backend; no adapter config |
| **Loki** | PLANNED | `references/timeline.md` (`observability/loki/`), `wiki/Observability-Backends.md` | In timeline roadmap; no implementation |
| **Graylog** | PLANNED | `wiki/Observability-Backends.md` | Listed as logging backend; no adapter config |
| **Splunk** | PRESENT | `pkg/audit/export/splunk.go`, `pkg/audit/export/export_test.go` | Splunk exporter implemented with HEC format, tests passing |
| **Nagios** | PLANNED | `references/timeline.md` (`observability/nagios/`), `wiki/Observability-Backends.md` | In timeline roadmap; no implementation |
| **Datadog** | DOCUMENTED | `wiki/Observability-Backends.md` | Listed as metrics/tracing/dashboard backend |
| **InfluxDB** | DOCUMENTED | `wiki/Observability-Backends.md` | Listed as metrics backend |
| **VictoriaMetrics** | PLANNED | `containers/README.md`, `containers/docker/README.md`, Docker Compose configs | Docker Compose includes it; no direct code integration |
| **ClickHouse** | PLANNED | `containers/docker/README.md`, Docker Compose configs, kanban task | Vector-to-ClickHouse pipeline planned; no code yet |
| **SigNoz** | DOCUMENTED | `docs/sessions/HANDOFF_2026-02-09_S8.md` | Listed as target integration |
| **OpenTelemetry** | PRESENT | `pkg/tracing/collector.go` (OTEL compatibility layer), `CLAUDE.md`, `wiki/Architecture.md` | OTLPSpan bridge, 128-bit trace IDs, W3C Trace Context. Core signals are OTel-compatible. |
| **PagerDuty** | PRESENT | `pkg/deploy/pipeline/notification.go`, `pkg/audit/export/splunk.go` | Notification channel implemented with routing key config |
| **OpsGenie** | PRESENT | `pkg/deploy/pipeline/notification.go` | Notification channel implemented with API key config |
| **SLO Management** | PRESENT | `services/vambraces/vambraces.go` | SLO struct, CreateSLO in vambraces observability service |
| **Log Aggregation (own)** | PRESENT | `pkg/logagg/` (ring buffer, publisher, subscriber, query, setup) | S36 Four Pillars: ring buffer, zerolog hook, SSE live tail |
| **Tracing (own)** | PRESENT | `pkg/tracing/collector.go`, `pkg/telemetry/` | Custom collector with span storage, OTEL bridge |
| **Dashboard (own)** | PRESENT | `dashboard/`, `cmd/dashboard-backend/` | Vanilla JS dashboard with packet-flow, metrics, live tail |

## 3. Security

| Technology | Status | Evidence | Notes |
|------------|--------|----------|-------|
| **Falco** | DOCUMENTED | `docs/adr/ADR-003`, `docs/security/S24-security-roadmap.md` | Referenced as ecosystem comparison; S24 roadmap mentions Falco for runtime security monitoring |
| **Tetragon** | NOT YET | Only in S41 battle plan category list | No references in code or docs |
| **Tracee (Aqua)** | NOT YET | Only in S41 battle plan category list | No references in code or docs |
| **ebpfkit** | NOT YET | Only in S41 battle plan grep instruction | No references found -- offensive tool, N/A |
| **TripleCross** | NOT YET | Only in S41 battle plan grep instruction | No references found -- offensive tool, N/A |
| **Bad-BPF** | NOT YET | Only in S41 battle plan grep instruction | No references found -- offensive tool, N/A |
| **Bombini (Rust+Aya+LSM)** | NOT YET | Only in S41 battle plan category list | No references in code |
| **BPF LSM** | PRESENT | `pkg/ebpf/loader.go` (BPF_PROG_TYPE_LSM=29, BPF_LSM_MAC=27, BPF_LSM_CGROUP=43, TypeLSM), `docs/sessions/S21-blackmage-assessment.md` | LSM program type supported in eBPF loader; security roadmap mentions BPF LSM rules |
| **Wazuh** | DOCUMENTED | `wiki/Observability-Backends.md` | Listed as SIEM backend |
| **WAF (own)** | PRESENT | `pkg/waf/` (SQLi, XSS, SSRF, path traversal, bot detection), `cmd/waf/` (Rust) | Full WAF: Go detection engine + Rust proxy (Shield) |
| **mTLS** | PRESENT | `pkg/tls/`, `pkg/certs/` (CA, CSR, rotation, store, manager) | TLS 1.3, per-service certs, automatic rotation |
| **Seccomp** | PRESENT | `containers/docker/seccomp-default.json`, all containerd service configs, NixOS modules | Minimal syscall whitelist for Go static binaries |
| **Container Hardening** | PRESENT | `nix/modules/hardening.nix`, all NixOS container defs, containerd OCI configs | Namespaces, cgroups, capabilities, read-only FS |
| **RBAC** | PRESENT | `pkg/auth/` (rbac, jwt, apikey, service_token) | Role-based access control with JWT and API key auth |
| **Secrets Management** | PRESENT | `pkg/secrets/encryption/age.go` | SOPS + age encryption for secrets |

## 4. Toolchain (eBPF)

| Technology | Status | Evidence | Notes |
|------------|--------|----------|-------|
| **Aya (Rust)** | PRESENT | `ebpf/*/`, `cmd/ebpf-loader/`, `cmd/ebpf-collector/`, Cargo.toml dependencies | Primary eBPF framework: 8 BPF programs (packet-marker, flow-tracker, latency-probe, hop-ebpf, monad-cpu, shield, yaldabaoth, syscall-tracer) |
| **cilium/ebpf (Go)** | DOCUMENTED | Referenced in ADRs, battle plans; `cmd/unheaded-daemon/internal/ebpf/loader.go` has TODO | Own BPF syscalls in `pkg/ebpf/loader.go` preferred over cilium/ebpf dependency (ADR-004) |
| **libbpf** | DOCUMENTED | `cmd/ebpf-loader/src/main.rs`, `wiki/eBPF-Programs.md` | Acknowledged as system-level BPF support; aya used instead due to libbpf v1.0+ incompatibility with aya-ebpf 0.1.x legacy maps |
| **bpftool** | PRESENT | `scripts/doom-ring.sh`, multiple session docs | Used for program attachment and map inspection at runtime |
| **BPF CO-RE (BTF)** | DOCUMENTED | `pkg/ebpf/loader.go` references BTF types | Loader supports BTF; not fully CO-RE yet |
| **XDP** | PRESENT | `ebpf/packet-marker/`, `ebpf/hop-ebpf/`, `ebpf/shield-ebpf/`, `pkg/ebpf/loader.go` | XDP programs for packet marking, Doom ring, Shield WAF |
| **TC (Traffic Control)** | PRESENT | `ebpf/flow-tracker/`, `pkg/ebpf/loader.go`, `pkg/netlink/` | TC classifier programs for flow tracking |
| **kprobe/tracepoint** | PRESENT | `ebpf/latency-probe/`, `ebpf/syscall-tracer/`, `pkg/ebpf/loader.go` | Tracepoint programs for latency and syscall tracing |
| **Ring Buffer** | PRESENT | `pkg/logagg/ringbuffer.go`, `pkg/ebpf/anamnesis_reader.go` | Both userspace and BPF ring buffer implementations |

## 5. Stream Processing (Flink Patterns)

| Technology / Pattern | Status | Evidence | Notes |
|---------------------|--------|----------|-------|
| **Apache Flink** | NOT APPLICABLE | S41 battle plan comparison only | Comparison target, not integration |
| **Watermark (perf)** | PRESENT | `cmd/trace-collector/src/collector/perf.rs`, `pkg/runtime/cgroups_v2.go` | Perf event watermarks in trace collector; memory watermarks in cgroup management. Not Flink-style event-time watermarks. |
| **Checkpoint (state)** | PRESENT | `pkg/storage/wal/wal.go` (WAL checkpoint), `pkg/telemetry/telemetry_test.go` | WAL checkpoint + truncation in storage layer. Not Flink-style Chandy-Lamport snapshots. |
| **Exactly-Once Semantics** | DOCUMENTED | `docs/sessions/S21-blackmage-assessment.md` | BlackMage assessment discusses exactly-once for GOAWAY flows; not a general guarantee |
| **Event-Time Processing** | NOT YET | No event-time vs processing-time distinction found | All timestamps are wall-clock; no out-of-order event handling |
| **Windowing (Tumbling/Sliding)** | NOT YET | No windowing abstractions found | Time-series aggregation exists in dashboard metrics but no formal windowing |
| **Backpressure** | PRESENT | `pkg/ebpf/anamnesis_reader.go`, `pkg/protocol/dos/dos.go`, `cmd/trace-collector/src/main.rs` | Backpressure handling in BPF reader, DoS protection, trace collector flow control |
| **State Management** | PRESENT | `pkg/state/reconciler.go`, `pkg/storage/wal/wal.go`, `pkg/storage/kv/badger.go` | Reconciler with desired-state model, WAL, BadgerDB key-value store |

## 6. IaC Backends

| Technology | Status | Evidence | Notes |
|------------|--------|----------|-------|
| **Ansible** | PLANNED | `references/timeline.md` ("Ansible renderer -- playbook + role generation") | In roadmap; DEFERRED per S35 strategic decision |
| **Terraform** | PLANNED | `references/timeline.md` ("Terraform renderer -- HCL module generation") | In roadmap; DEFERRED per S35 strategic decision |
| **Puppet** | PLANNED | `references/timeline.md` ("Puppet renderer -- manifest generation") | In roadmap; DEFERRED per S35 strategic decision |
| **Kubernetes** | PLANNED | `references/timeline.md`, `wiki/IaC-Backends.md` | In roadmap; DEFERRED per S35 strategic decision |
| **Chef** | PLANNED | `references/timeline.md` ("Chef renderer -- cookbook generation") | In roadmap; DEFERRED per S35 strategic decision |
| **Salt** | PLANNED | `references/timeline.md` ("Salt renderer -- state file generation") | In roadmap; DEFERRED per S35 strategic decision |
| **NixOS** | PRESENT | `nix/` (flake, containers, modules, hardening) | Primary container definition format; all services have NixOS defs |
| **Docker** | PRESENT | `containers/docker/`, Docker Compose, seccomp profiles | Full Docker Compose stack with all services |
| **LXD** | PRESENT | `containers/lxd/` (profiles, scripts) | LXD container profiles with hardening |
| **containerd** | PRESENT | `containers/containerd/` (config, OCI specs for all services) | OCI runtime specs with seccomp for every service |

## 7. Additional Capabilities (Cross-Cutting)

| Capability | Status | Evidence | Notes |
|------------|--------|----------|-------|
| **Chaos Engineering** | PRESENT | `services/yaldabaoth/`, `pkg/mesh/config.go`, dashboard chaos injection panel | Yaldabaoth chaos service; fault injection via mesh config |
| **Circuit Breaking** | PRESENT | `pkg/mesh/` (circuit breaker in mesh), `pkg/wotan-client/reliability_test.go` | Circuit breaker with configurable thresholds |
| **Rate Limiting** | PRESENT | `pkg/dns/`, `pkg/waf/`, `pkg/validation/` | Rate limiting in DNS, WAF, and syscall validation |
| **Drift Detection** | PRESENT | `services/kenoma/`, `services/pleroma/`, `pkg/state/reconciler.go` | Kenoma drift detection service, Pleroma desired-state, reconciler |
| **Service Map/Topology** | PRESENT | `services/cloak/cloak.go`, `services/architect/core.go`, `pkg/scheduler/algorithm.go` | Topology-aware scheduling, dependency graphs in architect |
| **Distributed Tracing** | PRESENT | `pkg/tracing/collector.go`, `pkg/telemetry/`, `ebpf/packet-marker/` | End-to-end: eBPF trace ID injection -> collector -> dashboard |
| **Apache httpd** | PRESENT | `pkg/waf/detection/path.go`, `pkg/waf/inspection/response.go`, `pkg/waf/detection/bot.go` | WAF detects Apache config exposure, version disclosure, and Apache HTTP client bot signatures |

---

## Gap Analysis

### Priority 1 -- Gaps That Impact Competitive Position

| Gap | Category | Impact | Recommendation |
|-----|----------|--------|----------------|
| **No `observability/` adapter directory** | Observability | Cannot generate drop-in configs for any backend | Create `observability/` with Prometheus, Grafana, ELK, Jaeger adapter configs (S35 decision: ship Prometheus + zerolog first) |
| **Dashboard UI behind Coroot/Hubble** | Observability | S41 battle plan identifies this as P0 | Dashboard resurrection sprint: service map, flow visualization, SLO dashboards |
| **No event-time processing** | Stream Processing | Cannot handle out-of-order events in eBPF pipeline | Design event-time watermark model for trace collector (Flink-inspired but wire-native) |
| **No formal windowing abstractions** | Stream Processing | Metrics aggregation is ad-hoc | Add tumbling/sliding window support to `pkg/logagg/` or new `pkg/windowing/` |

### Priority 2 -- Gaps That Affect Completeness

| Gap | Category | Impact | Recommendation |
|-----|----------|--------|----------------|
| **Tetragon/Tracee patterns not studied** | Security | Missing runtime security enforcement patterns | Study Tetragon's process-level eBPF enforcement for hardening roadmap |
| **Katran XDP LB patterns not studied** | Networking | Own LB is userspace; Katran proves XDP-level LB is viable | Evaluate XDP-based load balancing for Pauldrons v2 |
| **Pixie not studied** | Observability | Missing auto-instrumentation patterns | Low priority; own eBPF pipeline covers same ground differently |
| **Calico not studied** | Networking | Missing eBPF-based network policy enforcement patterns | Evaluate for `pkg/network/policy_controller.go` enhancement |
| **Bombini LSM patterns not studied** | Security | Rust+Aya+LSM is directly relevant to our stack | Study for hardening BPF map access (BlackMage S21 recommendation) |
| **cilium/ebpf Go library not integrated** | Toolchain | Own BPF syscalls work but lack community testing | ADR-004 decided against; revisit if own impl hits edge cases |
| **IaC renderers not started** | IaC | All 6 backends DEFERRED | Correct per S35; ship after stable alpha |

### Priority 3 -- Gaps That Are Intentionally Deferred

| Gap | Category | Status | Timeline |
|-----|----------|--------|----------|
| All observability adapter configs | Observability | DEFERRED (S35) | Post-alpha: Prometheus + zerolog first |
| All IaC renderers (Ansible, Terraform, Puppet, K8s, Chef, Salt) | IaC | DEFERRED (S35) | Post-alpha: ship iteratively |
| Grafana dashboard JSON | Observability | DEFERRED | Post-alpha |
| ELK/Loki/Nagios adapters | Observability | DEFERRED | Post-alpha |
| Exactly-once semantics (general) | Stream Processing | DOCUMENTED only | Design needed for production |

---

## Summary Statistics

| Status | Count |
|--------|-------|
| **PRESENT** (code exists) | 38 |
| **PLANNED** (in roadmap, no code) | 16 |
| **DOCUMENTED** (mentioned in docs only) | 14 |
| **NOT YET** (no references) | 8 |
| **NOT APPLICABLE** (competitor, not integration) | 2 |

### Strengths Confirmed
1. **eBPF toolchain is deep**: 8 BPF programs across XDP/TC/kprobe/tracepoint, Aya framework, own BPF syscall loader, BPF LSM support
2. **Load balancing is comprehensive**: 10+ algorithms across 3 packages, L4/L7, Maglev consistent hashing
3. **Security hardening is thorough**: Seccomp, namespaces, cgroups, mTLS, WAF (Go + Rust), RBAC, secrets management
4. **Service mesh is fully custom**: Discovery, load balancing, circuit breaking, mTLS, routing -- no Envoy/Istio dependency
5. **Observability foundations solid**: Prometheus scraping, OTEL bridge, log aggregation ring buffer, custom tracing collector

### Weaknesses Confirmed
1. **No adapter config directory**: `observability/` does not exist yet despite being in timeline
2. **Stream processing primitives missing**: No formal windowing, event-time processing, or exactly-once guarantees
3. **Dashboard UI needs resurrection**: Behind Coroot/Hubble in insight extraction (S41 P0)
4. **Runtime security tools not studied**: Tetragon, Tracee, Bombini patterns not yet incorporated

---

**Generated by:** S41 Phase 6 ecosystem validation
**Next Phase:** Phase 7 -- Gap closure prioritization and sprint planning
