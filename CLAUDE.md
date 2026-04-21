# CLAUDE.md - Unheaded Alpha Development Guide

**For:** Claude AI agents working on Unheaded
**Updated:** March 19, 2026
**Project:** github.com/unheaded/unheaded

---

## 🎯 Project Vision

**"Production-ready infrastructure in hours, not months."**

Unheaded is a configuration management automation platform delivering complete infrastructure for modern SaaS applications. User brings their app ("the head"), we provide everything else ("unheaded").

**Core Capabilities:**
- ✅ eBPF-based observability (L2-L7 tracing)
- ✅ Immutable infrastructure (LXD, containerd, NixOS, Docker)
- ✅ Interchangeable IaC backends (Ansible, Terraform, Puppet, Kubernetes, Chef, Salt)
- ✅ Zero user data access (architectural isolation)
- ✅ Service mesh built on Wotan message bus
- ✅ Declarative everything (version-controlled configs)
- ✅ Self-hosting proof (The Meta Moment)

---

## 🏗️ Architecture Overview

### The 6 Layers

```
Layer 5: User Interface (Dashboard, Kanban)
Layer 4: Application Services (timeguru, captain, micromanager, architect)
Layer 3: Infrastructure Services (wotan, trace-collector, gateway)
Layer 2: Control Plane (unheaded-daemon)
Layer 1: Data Plane (eBPF programs)
Layer 0: Infrastructure (LXD, host OS)
```

### Technology Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| eBPF programs | **Rust** (Aya) + **Go** (cilium/ebpf) | Aya for kernel programs, cilium/ebpf for Go userspace |
| Services | **Go 1.21+** | Simplicity, concurrency, tooling |
| Containers | **LXD / containerd / NixOS / Docker** | Interchangeable drop-in runtimes, same hardening baseline |
| Message Bus | **Wotan** (Go + gRPC) | Triple-role: ring buffer + event bus + protocol RAM |
| Load Balancers | **HAProxy 2.8+** (edge + internal) + **Nginx 1.25+** (per-app sidecars) | Three-layer LB: TLS term → service routing → app proxying. Full Prometheus + Loki telemetry |
| Gateway | **nginx** | Battle-tested, HTTP/3 support |
| Frontend | **Vanilla JS** | No framework overhead, full control |
| Orchestration | **LXD** (primary), containerd, Docker | Runtime-agnostic control plane |
| Config Management | **Ansible / Terraform / Puppet / Kubernetes / Chef / Salt** | Interchangeable IaC backends, same desired-state model |
| Observability | **Prometheus, Grafana, ELK, Fluentd, Jaeger, Nagios** + more | Interchangeable backends; custom Wotan-native defaults long-term |

**S67 Protocol Foundation:** Monad wire format frozen at v0x01 (20 bytes). Foundation spec draft-06 includes 12 IANA registries: Monad Protocol Version Numbers, Monad Flags Bitfield (C|Y|T|E|S|M|CUST|R), Monad Flow Actions (13 entries), Kingdom Mode Values, plus 8 others for extensibility and interoperability. IPR clearance: RFC 8928/9927 CLEAR. Plus 5 companion specs: Sophia-03 (dictionaries), Wotan-03 (distributed memory), MBC-ISA-00 (instruction set), Shim-00 (execution pipeline), PQC-00 (post-quantum auth).

### Network Design

- **Bridge:** lxdbr0 (10.10.10.0/24)
- **Gateway:** 10.10.10.100 (TLS termination, HTTP/3)
- **Wotan:** 10.10.10.10 (message hub)
- **Services:** 10.10.10.20-30 (agent services)
- **Apps:** 10.10.10.200+ (kanban, demo)

**Bare Metal Status (S76):**
- **WEST:** Online and running (test cluster, full feature validation)
- **EAST:** Online and running (4-core, 8GB DDR3 — staging environment, live since ~Feb 25, 2026)
- **Cross-host:** BPF flow graph operational between WEST ↔ EAST. Dashboard has host selector dropdown.

**Operational Notes:**
- **sudo required:** Docker commands (`docker compose`) require `sudo` on the dev machine.
- **P2P link:** EAST is reachable as `govan@east` via direct point-to-point connection (192.168.13.1 ↔ 192.168.13.2). SSH: `ssh govan@east`.

### Port Allocation — "The Doom Range" (16666-26666)

All Unheaded services use high ports to avoid conflicts with common dev tools.

| Tier | Range | Services |
|------|-------|----------|
| Infrastructure | 16666-16999 | doom-bridge (16666), doom-injector (16667), trace-collector (16670/16671) |
| Control Plane | 17000-17999 | unheaded-daemon HTTP (17000), gRPC (17001) |
| Wotan | 18000-18099 | wotan HTTP (18000), gRPC (18001) |
| Core Services | 19000-19999 | timeguru (19000), architect (19001), captain (19002), micromanager (19003), monad (19004), sophia (19005) |
| Applications | 20000-20999 | dashboard (20000), kanban (20001), wiki (20002) |
| Gateway | 21000-21443 | HTTP (21000), HTTPS (21443) |
| User Apps | 26000-26666 | Reserved for user applications |

### Transport Priority — gRPC-First

1. **Primary:** Wotan gRPC streaming (port 18001)
2. **Fallback:** HTTP/3 → HTTP/2 → HTTP/1.1
3. **Health:** Both gRPC health check AND HTTP /health endpoint required

**Implementation:** `pkg/transport/` implements the gRPC-first cascade with automatic HTTP fallback. All 10 services are wired to the transport package for unified connection management.

### Service Discovery

Four-layer approach implemented in `pkg/discovery/`:
1. **Wotan registration:** Services register/deregister via system.discovery topic (TTL-based with automatic reaping)
2. **Port scan:** Verify expected ports are listening on the host
3. **Convention scan:** /opt/unheaded/<service>/config.yaml discovery
4. **Static fallback:** configs/services.yaml for known-good defaults

All 10 services call `discovery.SetupServiceDiscovery()` on startup for automatic registration and peer discovery.

### Log Aggregation — "The Chronicler's Well"

**Implementation:** `pkg/logagg/` package with ring buffer (10,000 entry default capacity).

All services publish structured logs to Wotan topic `logs.<service>.<level>`. Log forwarding is handled by a `zerolog.Hook` that captures log events and pushes them to the aggregation ring buffer.

**Dashboard endpoints:**
- `GET /api/v1/logs` — query/filter aggregated logs
- `SSE /ws/logs` — live tail via Server-Sent Events

---

## 🚀 Development Principles

### 1. Security First, Always

**Critical Rules:**
- eBPF traceability from packet zero
- Zero user data access - architectural isolation enforced
- Container hardening: seccomp, capabilities, read-only FS
- Network policies: explicit allow, default deny
- TLS 1.3 minimum for external traffic
- Secrets: never in code, environment, or logs

**Test every PR for:**
- Does this access user data? → BLOCK
- Does this weaken isolation? → BLOCK
- Does this skip hardening? → BLOCK

### 2. Observable by Default

**Every component must:**
- Publish metrics (Prometheus-native, exportable to any backend)
- Log structured JSON (zerolog-native, shippable to any backend)
- Report to Wotan message bus
- Support distributed tracing (OpenTelemetry-compatible)
- Expose /health and /ready endpoints

**eBPF traces everything:**
- Packet markers at XDP layer
- Connection tracking
- Latency measurements
- All correlated by trace_id

**Observability Backend Strategy:**
Unheaded's internal observability emits OpenTelemetry-compatible signals (metrics, logs, traces). Users plug in their preferred backends — or use Unheaded's tailored defaults (long-term roadmap). Backends are interchangeable output adapters:

| Category | Supported Backends | Unheaded Default (Future) |
|----------|-------------------|--------------------------|
| **Metrics** | Prometheus, Grafana, Datadog, InfluxDB, Nagios | Custom Wotan metrics store |
| **Logging** | ELK (Elasticsearch/Logstash/Kibana), Fluentd/Fluent Bit, Flume, Splunk, Loki, Graylog | Custom Wotan log aggregator |
| **Tracing** | Jaeger, Zipkin, Tempo, Datadog APM | Custom eBPF-native tracer |
| **Alerting** | Grafana Alerting, PagerDuty, OpsGenie, Nagios, Prometheus Alertmanager | Custom Wotan alert engine |
| **Dashboards** | Grafana, Kibana, Datadog, custom | Unheaded Dashboard (vanilla JS) |
| **SIEM** | Elastic SIEM, Splunk Enterprise Security, Wazuh | Custom Wotan SIEM integration |

Config mirrors in `observability/` provide drop-in adapter configs for each backend. Same pattern as containers and IaC — your tools, our data model.

### 3. Declarative Everything

**All config in version control:**
- Container definitions (NixOS reference, plus Docker/containerd/LXD equivalents)
- IaC output modules (Ansible, Terraform, Puppet, Kubernetes, Chef, Salt)
- Service configs (YAML/TOML)
- Network policies
- Security baselines
- Timeline and strategy (MD/JSON/YAML)

**IaC Output Strategy:**
Unheaded generates configuration artifacts for the user's preferred toolchain. The control plane maintains a single desired-state model; IaC backends are interchangeable output renderers:

| Backend | Output | Use Case |
|---------|--------|----------|
| **Ansible** | Playbooks, roles, inventory | Agentless push-based config |
| **Terraform** | HCL modules, providers, state | Cloud infrastructure provisioning |
| **Puppet** | Manifests, Hiera data, modules | Agent-based declarative config |
| **Kubernetes** | Manifests, Helm charts, operators | Container orchestration at scale |
| **Chef** | Cookbooks, recipes, data bags | Ruby-based config management |
| **Salt** | States, pillars, grains | Event-driven, high-speed config |

**State management:**
- Desired state: Git (declarative configs, IaC modules)
- Actual state: /var/lib/unheaded/state/*.json
- Drift detection: unheaded-daemon polls every 30s
- Auto-remediation: restart with correct config

### 4. The Meta Moment - Self-Hosting Validation

**Critical proof of concept:**
- Kanban app shows Unheaded building Unheaded
- Reads timeline.md from timeguru service
- Every request traced by eBPF
- Publicly accessible (optional auth)

**If Unheaded can't host itself reliably, it's not ready for users.**

### 5. Ship Fast, Test Thoroughly

**Speed priorities:**
- Services first (fastest to demo)
- Control plane parallel
- eBPF last (adds tracing to working system)

**Quality gates:**
- Unit tests: 80%+ coverage
- Integration tests: all services communicating
- E2E tests: browser → gateway → services → Wotan
- Load tests: 1000 req/s sustained
- Security audit: automated + manual

---

## 📂 Project Structure

```
unheaded/
├── cmd/                    # Service binaries
│   ├── unheaded-daemon/   # Control plane (Go)
│   ├── trace-collector/   # eBPF → Wotan bridge (Rust)
│   ├── dashboard-backend/ # Metrics + WebSocket (Go)
│   └── kanban-app/        # Meta moment (Go + JS)
│
├── services/              # Microservice integration
│   ├── wotan/           # github.com/unheaded/wotan (existing)
│   ├── timeguru/         # Timeline tracking (Go)
│   ├── captain/          # Strategy service (Go)
│   ├── micromanager/     # Execution service (Go)
│   ├── architect/        # Design service (Go)
│   ├── monad/            # Unified state management (Go)
│   └── sophia/           # Knowledge graph service (Go)
│
├── ebpf/                  # eBPF programs (Rust + Aya)
│   ├── packet_marker/    # Trace ID injection at XDP
│   ├── flow_tracker/     # Connection tracking
│   └── latency_probe/    # RTT measurement
│
├── nix/                   # NixOS container definitions
│   ├── flake.nix         # Top-level flake
│   ├── containers/       # Per-service configs
│   └── modules/          # Shared modules (hardening, networking)
│
├── dashboard/             # Main dashboard UI
├── kanban/               # Kanban app (meta moment)
├── pkg/                  # Shared Go packages
├── docs/                 # Documentation
├── scripts/              # Automation scripts
└── references/           # Source-of-truth files (MD/JSON/YAML)
```

---

## 🔧 Service Implementation Guidelines

### Go Services (timeguru, captain, micromanager, architect)

**Structure:**
```go
// cmd/service/main.go
package main

import (
    "github.com/unheaded/unheaded/pkg/wotan-client"
    "github.com/unheaded/unheaded/pkg/telemetry"
)

func main() {
    // 1. Load config
    // 2. Connect to Wotan
    // 3. Start HTTP server (REST API)
    // 4. Subscribe to relevant Wotan topics
    // 5. Publish service metrics
    // 6. Handle graceful shutdown
}
```

**Required endpoints:**
- `GET /health` - Health check (200 if healthy)
- `GET /ready` - Readiness probe (200 if ready to serve)
- `GET /metrics` - Prometheus metrics
- `GET /api/v1/*` - Service-specific REST API

**Wotan integration:**
- Connect on startup
- Subscribe to relevant topics
- Publish state changes
- Graceful disconnect on shutdown

**Logging:**
```go
log.Info().
    Str("service", "timeguru").
    Str("trace_id", traceID).
    Msg("processing request")
```

### Data Format - Triple Mirror Strategy

**Every service maintains three formats:**

```
references/timeline.md    → Source of truth (human, Git)
references/timeline.json  → API responses (machine)
references/timeline.yaml  → IaC configs (tooling)
```

**Auto-sync on write:** MD → JSON/YAML

**Example (timeguru):**
```go
func (s *Service) UpdateTimeline(md string) error {
    // 1. Parse markdown
    timeline := parseMarkdown(md)

    // 2. Validate
    if err := timeline.Validate(); err != nil {
        return err
    }

    // 3. Write source
    if err := os.WriteFile("timeline.md", []byte(md), 0644); err != nil {
        return err
    }

    // 4. Generate mirrors
    json, _ := json.Marshal(timeline)
    yaml, _ := yaml.Marshal(timeline)

    os.WriteFile("timeline.json", json, 0644)
    os.WriteFile("timeline.yaml", yaml, 0644)

    // 5. Publish to Wotan
    s.wotan.Publish("timeline.updates", json)

    return nil
}
```

### NixOS Container Definitions

**Template:**
```nix
# nix/containers/timeguru.nix
{ config, pkgs, ... }:

{
  # Service definition
  systemd.services.timeguru = {
    description = "Timeguru Timeline Tracking Service";
    wantedBy = [ "multi-user.target" ];

    serviceConfig = {
      ExecStart = "${pkgs.timeguru}/bin/timeguru";
      Restart = "always";
      RestartSec = "5s";

      # Security hardening
      CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      NoNewPrivileges = true;
      PrivateTmp = true;
      ProtectSystem = "strict";
      ProtectHome = true;
      ReadOnlyPaths = [ "/etc" "/usr" ];
      ReadWritePaths = [ "/opt/unheaded/references" ];

      # Seccomp
      SystemCallFilter = [ "@system-service" "~@privileged" ];
    };
  };

  # Networking
  networking.firewall.enable = true;
  networking.firewall.allowedTCPPorts = [ 8000 ];

  # Environment
  environment.systemPackages = [ pkgs.timeguru ];
}
```

---

## 🔒 Security Requirements

### Container Hardening

**Every container MUST have:**

```nix
serviceConfig = {
  # Capabilities - minimum required only
  CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
  AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];

  # No privilege escalation
  NoNewPrivileges = true;

  # Filesystem isolation
  PrivateTmp = true;
  ProtectSystem = "strict";
  ProtectHome = true;
  ReadOnlyPaths = [ "/etc" "/usr" ];

  # Seccomp - block dangerous syscalls
  SystemCallFilter = [
    "@system-service"
    "~@privileged"
    "~@resources"
  ];

  # Process isolation
  PrivateDevices = true;
  ProtectKernelTunables = true;
  ProtectControlGroups = true;
  RestrictRealtime = true;
  RestrictNamespaces = true;
};
```

### Network Policies

**Default: DENY ALL**

```nix
networking.firewall = {
  enable = true;
  allowedTCPPorts = [ ]; # Explicit allow only
  extraCommands = ''
    # Allow internal container network
    iptables -A INPUT -s 10.10.10.0/24 -j ACCEPT

    # Drop everything else
    iptables -A INPUT -j DROP
  '';
};
```

### Secrets Management

**NEVER:**
- Hard-code secrets
- Put secrets in environment variables visible to ps
- Log secrets
- Store secrets in Git

**ALWAYS:**
- Use SOPS + age for encrypted secrets
- Mount secrets as files (not env vars)
- Rotate regularly
- Audit access

---

## 📊 Observability Standards

### Metrics (Prometheus)

**Required metrics for all services:**

```go
var (
    httpRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "unheaded_http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"service", "method", "path", "status"},
    )

    httpRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "unheaded_http_request_duration_seconds",
            Help: "HTTP request latency",
            Buckets: prometheus.DefBuckets,
        },
        []string{"service", "method", "path"},
    )

    wotanMessagesPublished = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "unheaded_wotan_messages_published_total",
            Help: "Messages published to Wotan",
        },
        []string{"service", "topic"},
    )
)
```

### Logging (Structured JSON)

**Use zerolog:**

```go
log.Info().
    Str("service", "timeguru").
    Str("request_id", requestID).
    Str("trace_id", traceID).
    Str("method", "GET").
    Str("path", "/api/v1/timeline").
    Int("status", 200).
    Dur("duration", duration).
    Msg("request_complete")
```

**Log levels:**
- `DEBUG`: Development only, never in production
- `INFO`: Normal operations
- `WARN`: Degraded but functional
- `ERROR`: Errors that need attention
- `FATAL`: Service cannot continue

### Wotan Topics

**Required subscriptions:**
- `alerts.critical` - All services must subscribe for critical alerts

**Publishing guidelines:**
- Include `trace_id` in all messages
- Include timestamps (Unix milliseconds)
- Keep payloads < 1MB
- Use protobuf for large messages

---

## 🧪 Testing Standards

### Unit Tests

**Coverage: 80%+ required**

```go
func TestTimelineParser(t *testing.T) {
    md := `# Timeline\n## Phase 1\n- [ ] Task 1`

    timeline, err := parseMarkdown(md)
    assert.NoError(t, err)
    assert.Equal(t, "Timeline", timeline.Title)
    assert.Len(t, timeline.Phases, 1)
}
```

### Integration Tests

**Test service communication:**

```go
func TestTimelineAPI(t *testing.T) {
    // 1. Start service
    srv := startTimeguru(t)
    defer srv.Stop()

    // 2. Make request
    resp := httpGet(t, "http://localhost:8000/api/v1/timeline")

    // 3. Verify
    assert.Equal(t, 200, resp.StatusCode)

    var timeline Timeline
    json.Unmarshal(resp.Body, &timeline)
    assert.NotEmpty(t, timeline.Phases)
}
```

### E2E Tests

**Browser → Gateway → Services → Wotan:**

```bash
# Start all containers
make containers-up

# Run E2E tests
go test ./tests/e2e/... -tags=e2e

# Tests should verify:
# - HTTP/3 connection
# - Trace propagation
# - Wotan message delivery
# - WebSocket updates
```

---

## 🚀 Development Workflow

### Building

```bash
# Build all Go services
make build

# Build eBPF programs (Rust)
make ebpf

# Build NixOS containers
make containers

# Build everything
make all
```

### Testing

```bash
# Unit tests
make test

# Integration tests
make test-integration

# E2E tests
make test-e2e

# All tests
make test-all
```

### Local Development

```bash
# Start development environment
make dev

# This starts:
# - Wotan (docker)
# - PostgreSQL (docker)
# - Services (locally)
# - Dashboard (http://localhost:8080)
```

### Deployment

```bash
# Setup host
sudo ./scripts/setup-host.sh

# Deploy alpha
sudo make deploy

# Check status
make status
```

---

## 📝 Commit Guidelines

### Conventional Commits

```
<type>(<scope>): <subject>

[optional body]

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `style`: Formatting (no code change)
- `refactor`: Code restructuring
- `test`: Adding tests
- `chore`: Build process, dependencies

**Examples:**
```
feat(timeguru): add timeline auto-sync to JSON/YAML

Implements triple format strategy. On timeline.md write,
automatically generates timeline.json and timeline.yaml mirrors.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

---

## 🎯 Current Phase: Alpha (✅ COMPLETE) → Age 2 (~65%) | Wire Format FROZEN

**Alpha Goal:** Demonstrate core platform capabilities — **ACHIEVED**

**Age 2 Goal:** Production hardening, bare metal validation, multi-host operations, Zhen AI

**Wire Format Status:** FROZEN at version 0x01 (20 bytes) — monad protocol locked, 7 breaking change candidates analyzed and rejected, 12 IANA registries in foundation spec draft-06

**Services:** 10 core + Zhen AI (timeguru, captain, architect, micromanager, monad, sophia, dashboard-backend, kanban-app, wotan, unheaded-daemon, zhen-inference, zhen-web-ui)

**Build:** `go build ./...` passes, all tests pass (0 failures)

**Bare Metal:** WEST + EAST both online. BPF flow graph works cross-host. Dashboard has host selector dropdown.

**S36 Four Pillars — COMPLETE:**
- **Port Authority:** Doom Range migration (all services on 16666-26666)
- **gRPC-First Transport:** `pkg/transport/` — gRPC cascade with HTTP fallback, all 10 services wired
- **Log Aggregation:** `pkg/logagg/` — ring buffer, zerolog hook, dashboard live tail
- **Service Discovery:** `pkg/discovery/` — four-layer discovery (Wotan/port-scan/convention/static)

**Alpha Success Criteria (ALL MET):**
- [x] All services communicating via Wotan
- [x] Kanban app displays timeline from timeguru
- [x] eBPF traces end-to-end packet flow
- [x] Dashboard visualizes traces in real-time (packet-flow + metrics)
- [x] Zero user data access (validated)
- [x] Dual bare metal hosts online (WEST + EAST)
- [x] Cross-host BPF flow graph operational
- [x] Authentication framework deployed (64 test cases, 100% coverage)
- [ ] Publicly accessible (optional auth) — deferred to Age 2
- [ ] Sub-50ms latency (packet → browser) — needs benchmarking
- [ ] Containers start in <10s — needs benchmarking

**What's Done (cumulative through S76):**
- All 10 services have HTTP APIs with health/ready/metrics endpoints
- Monad (port 19004) and Sophia (port 19005) services created
- Kanban task detail modal with view/edit/delete
- Dashboard packet-flow visualization + system metrics + host selector dropdown
- Control plane (unheaded-daemon) with Wotan + reconciliation
- NixOS container definitions for all services
- Docker Compose configuration for full stack
- Gateway routing for all services
- All tests passing (0 failures, 0 timeouts)
- S36 Four Pillars: Port Authority, gRPC-First Transport, Log Aggregation, Service Discovery
- S51 Auth Framework: Noop/APIKey/JWT authenticators, RBAC, audit logger (64 tests)
- S52 Legal/Compliance: SPDX headers on 838 Go files, GPL boundary documented
- S59 Dashboard Polish: Design system, demo data, Kanban review column
- S67 Wire Format Freeze: v0x01 locked, 12 IANA registries, IPR clear
- S67-S69 Multi-agent swarm: Observability stack, Suricata IDS, alternate routing (81 files)
- eBPF AF_XDP pipeline: 920K pps validated, Rust FFI + Go bridge
- DOOM-on-Monad: Computational completeness proof
- doom-runner (crates/doom-runner/): Aya-based Rust runtime replacing shell/Go loader chain
- Zhen Layer 0 (crates/zhend/): Anti-fragile gossip knowledge substrate, PQ crypto, 161 tests green
- EAST bare metal: Online, P2P link live, BPF flow graph cross-host
- ADR Sweep (2026-04-03): All 27 ADRs resolved (19 accepted, 4 deferred, 1 in-progress, 1 pipe dream)
- Zhen Champion (pkg/champion/): Trust Level 1+2, sandboxed file R/W, Kanban CRUD, action logging, snapshots (18 tests)
- Zhen MCP Server (raft/zhen_mcp_server.py): 7 tools for Claude Code integration (corpus_search, file_read, file_write, file_patch, service_health, runbook_list, runbook_show)
- 31 Operational Runbooks (runbooks/): 7 categories (infra, network, security, data, observe, doom, deploy)
- Service Template (pkg/service/): Standardized scaffold with health/ready/auth/Wotan (7 tests)
- BPF CI Gate (scripts/bpf-verifier-check.sh): Instruction budget tracking, static analysis
- Sealed Cask (scripts/build-sealed-cask.sh + verify-binding-rune.sh): Deterministic image builder with SHA256 integrity
- ROCm GPU acceleration: PyTorch 2.5.1+rocm6.2 for embedding (862 chunks/s), llama.cpp for inference (60 tok/s)
- RAFT QA generation: ~2000 pairs from Mistral-7B for fine-tuning pipeline
- Kanban Bugs: All 8 Three Crowns bugs fixed (ADR-020)
- Dependency Policy: Established orgs OK with approval, Rust replacement long-term (ADR-004)
- Wave 10C Backprop (2026-04-11): 3 backprop bugs fixed in zhenai-forge (lora_backward wrong tensor, missing chain rule, rmsnorm_backward extra weight factor). 32-layer toy gradient descent test proves correctness (loss 2.24→1.62, monotonic). 46/46 tests pass. See `wiki/Wave-10C-Backprop.md`.
- Wotan Topic Signing (2026-04-11): ML-DSA-65 enforcement on `config.*` topics in services/wotan/internal/signing/ (6 tests). Proto fields added to TopicPublishRequest/TopicEvent. ADR-043 hard condition #2 satisfied. See `wiki/Wotan-Topic-Signing.md`.
- Mímir's Law / Gleipnir Phase 0 Spike (2026-04-11, ADR-043, branch `spike/mimirs-law`): UPC-controlled OS baseline delivery PoC. New components: `pkg/gungnir/` (ML-DSA-65 sealed payloads, 4 tests), `pkg/gjallarhorn/` (20-byte Monad trigger packets, 5 tests), `pkg/enkrateia/` (alerts-only drift aggregator, 3 tests, zero FS mutations enforced), `cmd/heimdall-daemon/` (Mjölnir scan + drift detection), `cmd/gjallarhorn-sender/` (UPC trigger CLI), `crates/heimdall-bpf/` (Aya kprobe scaffold), `tomb/lich/LICH-012-config-convergence/` (red team campaign). REAL-METAL VALIDATED on EAST: zero false positives, 100% drift detection accuracy, alerts-only confirmed. See `wiki/Mimirs-Law.md`. 11 of 13 phases complete; bare-metal Phases 9 (full bootstrap), 11 (stress), 13 (gate eval) pending.
- WAVE10F Forge Real-Attention Gemma 4 (2026-04-17, plan `docs/battle-plans/WAVE10F-FORGE-REAL-ATTENTION-GEMMA4.md`): zhenai-forge gained a complete Gemma 4 E2B training path in `crates/zhenai-forge/src/gemma4.rs` (3000+ LOC). Real attention forward+backward (Q/K/V/softmax/RoPE/GQA), hybrid sliding/global mask per layer, partial-rotary RoPE on full layers, per-head Q/K-norm + weightless V-norm, gelu_pytorch_tanh, final logit softcap=30, Per-Layer Embeddings (PLE chain), unified KV gradient routing for layers 20-34 → producer 18/19, LoRA injection on Q/K/V/O at all 110 active targets, Adam with NaN guard. Verified end-to-end on real `/var/zhen/models/gemma-4-E2B-it.gguf` (9.3GB GGUF converted from HF safetensors at `/home/govan/tmp/gemma-4-E2B-it/` via llama.cpp@30dce2c convert_hf_to_gguf.py): grad-norm probe shows healthy=35/35 zero=0 nan=0 across all layers, healthy=110/110 LoRA targets, training loss descends 21.28→12.02→6.78 over 3 steps. CLI subcommand `zhenai-forge train-gemma4 --model ... --data ...` wired. GPU port landed 2026-04-20 (Phase 7 Steps A-E per `notes/wave10f-step-c-battle-plan.md`) — `zhenai-forge train-gemma4` defaults to GPU, `--cpu` falls back. Warm-path per-step: **2.05s** (vs 58s pure CPU, 43s hybrid-fwd). 10-step descent on fixed tokens: 19.96 → 0.043 (but that was memorization on 1 example — see below). Full timing at `notes/wave10f-step-c-timing.md`.

**Learning Gate** (2026-04-20, plan at `notes/wave10f-learning-gate-plan.md`): 5-experiment protocol designed by unheaded-scientist + unheaded-developer to distinguish memorization from genuine learning. Four of five pass strict thresholds on real GGUF: (1) eval loss drops 42% on held-out prefixes (CI95 excludes 1.0), (3) LoRA-A=0 init reproduces base model bit-identical, (4) eval_final scales monotonically with |T|: |T|=1 → eval WORSENS (memorization failure), |T|≥64 saturates at 13.17, (5) generalization gap exponent β = 0.27 (≪ 1.0 memorization signature). Exp 2 (scrambled-labels) converted to diagnostic after four iterations — 20-step budget sits in pre-memorization regime for both real and scrambled, so memorization signature deferred to Phase 7.2. **Forge is learning, not just memorizing.**

**24h Consolidation Block** (2026-04-20, plan at `notes/wave10f-24h-battle-plan.md`, session log at `notes/wave10f-24h-session-2026-04-20.md`): 7-phase follow-on executed autonomously under unheaded-marshal. Outcomes: (1) Exp 2 iter-5 plateau-based training revealed the **log(vocab) = log(262144) ≈ 12.48 information-theoretic floor** — scrambled-train CE can't descend below it by construction, so the hard-gate synthetic-corpus target is physically impossible. (2) lr sweep at plateau 100 steps showed **lr=1e-3 is the stable operating point**; lr=3e-3 is a cliff edge (works for 20 steps, blows up at 100 — eval returns to baseline). (3) Multi-Y capacity probe: **both groups ratio ~0.60** at rank-16 LoRA on 2 disjoint Y maps — forge partitions capacity cleanly. (4) **Python tokenizer slice** landed at `scripts/tokenize-kingdom-for-gemma4.py`; Kingdom corpus pre-tokenized to `/tmp/24h-kingdom-{train,eval}.jsonl` (3568 + 397 sequences, MAX_TOKENS=384). (5) **Phase 5 real-RAFT training ABORTED**: forge's GPU forward offloads only matmuls, so at seq=384 single-threaded CPU attention (O(seq²)) dominates — RAFT smoke stalled 64 min with zero training output; GPU% = 0 during stall. (6) **All 6 Learning Gate + core tests regression-green** — zero regressions from 24h session. Next sprint: either GPU attention kernels (unlocks long-seq training) or aggressive prompt truncation to seq≤64 (unblocks real LoRA this week). Mistral training path retained as stale functions in `train.rs` per Stevie's "side stuff" rule. Architecture spec at `crates/zhenai-forge/notes/gemma4-arch-spec.md`, math derivations at `notes/phase1-attention-math.md`, session log at `notes/wave10f-session-2026-04-17.md`. Supersedes WAVE10E (banner in old battle plan).

### Doom-over-IPv6 — ALL DOCS IN `docs/doom/`

**MANDATORY: All Doom work MUST be documented in `docs/doom/`.** Before and after every
Doom session, read and update the docs. Do NOT keep Doom knowledge in CLAUDE.md — it
goes stale. The docs/ folder is the living record.

- `docs/doom/README.md` — Architecture, memory layout, bugs fixed, what works/doesn't
- `docs/doom/FINDINGS.md` — Session findings, debugging narrative, lessons learned
- `docs/doom/BATTLE-PLAN.md` — Choose-your-own-adventure paths to playable Doom
- `docs/doom/RUNBOOK.md` — Step-by-step operational runbook
- `docs/doom/ARCHITECTURE.md` — Deep technical architecture
- `docs/doom/NEXT-STEPS.md` — Prioritized roadmap

**Current blocker:** PC corruption. See `docs/doom/FINDINGS.md`.
**Source of truth for memory layout:** `crates/doom-runner/src/memory.rs`
**Source of truth for runtime:** `crates/doom-runner/`
- Kingdom startup/shutdown services, host metrics storage
- S-ZHEN: Zhen AI Champion (RAG pipeline, Flask web UI, Zhen-Claude bridge API, 1.52M vector corpus)
- S-PQC: SLH-DSA (FIPS 205) implemented via cloudflare/circl v1.6.3, 60 PQC tests passing
- UPC Dream Ladder: All Level 4 OS primitives + Level 5 boot + Level 6 arch/mbc (37 files)
- The Well: Multi-DB PostgreSQL (3 databases: unheaded_app/ops/config, 7 service-scoped users)
- SBOM: 553 dependencies audited, GPL boundary clean, SPDX checks integrated
- CI/CD: GHA + Jenkins hardened, SPDX checks, pre-commit hook installed
- S-WEST: Bootstrap complete (kernel 6.17.0-19, 0 failed services)
- All 4 P1 bugs fixed (#20 Nix TLS, #36 log forwarding, #29 Kanban logging, E2E smoke)

**Age 2 Remaining Epics:**
- WireGuard IPv6 overlay (fd00:dead:beef::/48)
- Foundation spec draft-06 (IANA integration)
- Sophia draft-03 (sub-dictionary types, QPACK compression)
- Wotan draft-03 (error code taxonomy)
- Demo video + README polish (VC readiness)
- Public accessibility (optional auth)
- Performance benchmarking (sub-50ms target)
- RAFT fine-tuning (QLoRA on Mistral-7B with 616 QA pairs)
- Zhen Engine (custom Rust inference + Go management plane)

**S35 Strategic Decisions (Feb 24, 2026):**
- **License**: GPL-3.0. Protocol specs dual-licensed GPL-3.0/Apache 2.0 for ecosystem adoption.
- **Doom**: Fork official id-Software/DOOM (with sound), replace doomgeneric. Move out of repo before going public.
- **SBOM**: Run ScanCode + FOSSology + ORT against codebase. Must complete before going public.
- **Backends**: All observability + IaC adapters DEFERRED not killed. Anti-lock-in is core principle. Ship Prometheus + zerolog first.
- **Inverse Mask**: Deep exploration session required (BlackMage + Developer + Architect + Scientist).
- **VC**: Explore Austin venture capital while repo is private. Protocol IS the moat.
- **LOC Audit (S78, March 17, 2026)**: 415K production, 753K with tests, 1,137K total. Breakdown: Go 267K prod + 337K test = 604K, Rust 53K (30K eBPF + 23K other), Python 15K, JS 21K, Shell 27K, Nix 11K, HTML/CSS 20K, Config 40K, Docs 345K, Proto+SQL 1K. 34 active services (zero stubs). 23 eBPF programs.
- **Timeline Audit**: Fix milestone statuses — no "completed" at 55% progress. Honesty > hype.

---

## 🧠 Working with Claude Agents

### When to Spawn Agents

**USE AGENTS FOR:**
- Independent services (timeguru, captain, etc)
- Parallel implementation (services + daemon)
- Large refactors
- Documentation updates
- Test coverage improvements

**DON'T USE AGENTS FOR:**
- Quick fixes (< 10 lines)
- Single file edits
- Simple bug fixes

### Agent Instructions Template

```
Build the [SERVICE] service for Unheaded.

Context:
- Read CLAUDE.md for standards
- Read docs/ARCHITECTURE.md for architecture
- Read references/timeline.md for current status

Requirements:
- Go 1.21+
- REST API with /health, /ready, /metrics
- Wotan integration
- Triple format (MD/JSON/YAML)
- Unit tests (80%+ coverage)
- NixOS container definition with hardening

Deliverables:
1. services/[SERVICE]/main.go
2. services/[SERVICE]/*_test.go
3. nix/containers/[SERVICE].nix
4. docs/services/[SERVICE].md
```

### Agent Coordination

**For parallel agents:**
1. Spawn all at once (not sequential)
2. Agents work independently
3. Review all output together
4. Integration test after merge

**Communication:**
- All agents use Wotan (no direct service-to-service)
- Shared types in `pkg/`
- Conflicts resolved by final integration test

---

## 🔗 Key References

**Essential reading:**
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - Complete architecture
- [THE_META_MOMENT.md](docs/THE_META_MOMENT.md) - Self-hosting philosophy
- [SYSTEM_DIAGRAM.md](docs/SYSTEM_DIAGRAM.md) - Visual overview
- [timeline.md](references/timeline.md) - Living roadmap

**External:**
- [Wotan](https://github.com/unheaded/wotan) - Message bus (Phase 1)
- [Aya](https://aya-rs.dev/) - eBPF framework for Rust
- [NixOS Manual](https://nixos.org/manual/) - Container definitions

---

## 💡 Design Philosophy

### 1. Self-Hosting Validation

Self-hosting is proof, not marketing. If Unheaded can't reliably host its own development, it's not ready for users.

### 2. Security is Not Optional

Every decision evaluated through security lens:
- Does this access user data? → NO
- Does this weaken isolation? → NO
- Does this skip hardening? → NO

### 3. Observable by Default

If you can't see it, you can't trust it. eBPF traces everything from packet zero.

### 4. Ship Fast, Validate Thoroughly

Move quickly, but every commit must pass:
- Unit tests
- Integration tests
- Security audit (automated)
- Manual review

### 5. Radical Transparency

Timeline is public. Progress is public. Kanban board is public (optional auth).

We're building in the open because we have nothing to hide.

---

## 🎓 Learning Resources

**Go:**
- Effective Go: https://go.dev/doc/effective_go
- Go Concurrency Patterns: https://go.dev/blog/pipelines

**Rust + eBPF:**
- Aya Book: https://aya-rs.dev/book/
- eBPF Guide: https://ebpf.io/what-is-ebpf/

**NixOS:**
- NixOS Manual: https://nixos.org/manual/
- Nix Pills: https://nixos.org/guides/nix-pills/

**Observability:**
- Prometheus Best Practices: https://prometheus.io/docs/practices/
- Kibana Guide: https://www.elastic.co/guide/en/kibana/index.html
- Nagios Documentation: https://library.nagios.com/docs
- Structured Logging: https://flume.apache.org/releases/content/1.11.0/FlumeDeveloperGuide.html
- Structured Logging: https://www.fluentd.org/architecture
- Structured Logging: https://www.elastic.co/docs/get-started
---

## 🚨 Common Pitfalls

### 1. Breaking Architectural Isolation

**WRONG:**
```go
// demo-app reading from Wotan topics
wotan.Subscribe("network.traces")  // NO! User can see infrastructure
```

**RIGHT:**
```go
// demo-app is isolated, only talks to gateway
http.Get("http://gateway/api/mydata")
```

### 2. Hardcoding Configuration

**WRONG:**
```go
const wotanAddr = "10.10.10.10:9090"
```

**RIGHT:**
```go
wotanAddr := os.Getenv("WOTAN_ADDR")
if wotanAddr == "" {
    wotanAddr = config.Get("wotan.address")
}
```

### 3. Skipping Error Handling

**WRONG:**
```go
data, _ := ioutil.ReadFile("timeline.md")
```

**RIGHT:**
```go
data, err := ioutil.ReadFile("timeline.md")
if err != nil {
    return fmt.Errorf("read timeline: %w", err)
}
```

### 4. Ignoring Context Cancellation

**WRONG:**
```go
for {
    processMessage()
}
```

**RIGHT:**
```go
for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        processMessage()
    }
}
```

---

## 📞 Questions?

**For Stevie:**
- Email: stevie@bellis.tech

**For Agents:**
- Read this doc first
- Check existing code for patterns
- Ask in context of specific implementation

---

## 📜 Kingdom Doctrines

### Cross-Service Health Monitoring

**Location:** `docs/SERVICE_BREAKOUT_STRATEGY.md`

Every microservice MUST health check all services it depends on. Failures reported to Wotan topic `system.outage.reports`. Severity by **percentage-based consensus**:

| % Reporting | Severity | UI Color | Hex | Actions |
|-------------|----------|----------|-----|---------|
| 0% - 12.49% | **OK** | Green | `#008000` | Healthy |
| 12.50% - 37.49% | **WARN** | Yellow-Brown | `#fdda61` | Log, email |
| 37.50% - 62.49% | **ERROR** | Bright Yellow | `#ffff00` | Log, 2nd email |
| 62.50% - 87.49% | **CRITICAL** | Neon Orange | `#ff5c00` | Auto-remediate |
| 87.50% - 100% | **PANIC** | Bright Red | `#ff0000` | All hands, PagerDuty |

**Formula:** `(unique_reporters / total_dependent_services) × 100`

Scales automatically from 8 to 800+ services.

### Post-Alpha Service Breakout

After stable alpha, monorepo splits into individual service repos. Each service becomes a Go module imported by `github.com/unheaded/unheaded`.

**Timeline:** Alpha (Feb 8) → Breakout complete (Mar 15)

See `docs/SERVICE_BREAKOUT_STRATEGY.md` for full plan.

---

**Last Updated:** February 24, 2026
**Version:** Alpha (~99% Complete) | S36 Four Pillars Complete
**Status:** All 10 services operational, all tests passing. S36 Four Pillars complete: Port Authority (Doom Range), gRPC-First Transport (pkg/transport/), Log Aggregation (pkg/logagg/), Service Discovery (pkg/discovery/).

## Recent Sprints (S51-S60: Security, Legal, Dashboard Polish, Docs)

### Wave 1 Complete: Security Framework (S51)
- **pkg/auth/**: Full authentication framework
  - Authenticator interface (pluggable auth backends)
  - NoopAuthenticator (development/testing)
  - APIKeyAuthenticator (production)
  - JWTAuthenticator (federated identity)
  - RBACAuthorizer (role-based access control)
  - AuditLogger (security event tracking)
- **Code**: 3,093 LOC across auth packages
- **Tests**: 64 test cases (100% coverage of auth layer)
- **Integration**: All 10 services now use auth.Middleware(authenticator) on startup
- **Hardening**: MaxHeaderBytes 1<<20 on protocol-api, doom-bridge, trace-collector-go. Rate limiter with TrustedProxies guard on X-Forwarded-For
- **Result**: Production-ready auth framework, zero security debt, ready for commercial licensing

### Wave 2 Complete: Legal & Compliance (S52)
- **SPDX Headers**: All Go files tagged with SPDX-License-Identifier: GPL-3.0-or-later
  - GPL-3.0 License
  - 100% coverage, automated CI checks
- **docs/legal/**:
  - LICENSE-MIT.md: GPL-3.0 License documentation
  - IP-INVENTORY.md: Complete IP ownership matrix (original code, forks, integrations)
  - IANA-REGISTRATION.md: Plan for registering UNHEADED_METRIC_V1 (Type 0x2A)
- **CONTRIBUTOR-GUIDE.md**:
  - CLA policy (copyright assignment for commercial licensing)
  - Branch strategy (main/staging/feature/)
  - Testing requirements (80%+ coverage, no skipped tests)
  - Commit message format (Conventional Commits)
- **THIRD_PARTY.md**: Complete dependency inventory
  - GPL boundary documentation (zero GPL dependencies in core)
  - SBOM generation (ScanCode + FOSSology)
  - License audit trail (all transitive dependencies verified)
- **Result**: Ready for public release, legal audit passing, IANA registration path clear

### Wave 3 Complete: Dashboard Polish (S59)
- **dashboard/css/design-system.css**: 76 CSS tokens
  - Color palette (frosted glass theme)
  - Typography (JetBrains Mono for code, system fonts for UI)
  - Spacing, shadows, borders (consistent Kingdom branding)
- **dashboard/js/demo-data.js**: 9 data generators
  - Packet flow graph (mock traces, latency buckets)
  - Metrics (CPU, memory, network, disk)
  - Service health (percentage-based consensus)
  - Timeline events
  - Auto demo-mode detection (no services running → enable mock)
- **Kanban improvements**:
  - Review column with action buttons (Approve, Reject, Request Changes)
  - Task detail modal with view/edit/delete
  - Phase progress visualization
  - Drag-and-drop between columns
- **Result**: Professional UI, ready for conference demo, self-hosting proof visible

### IPv6 Metric Research Complete (D10)
- **Location**: docs/research/IPV6_METRIC_CAPACITY.md
- **Scientist lab notebook** exploring Monad register design space
- **Key findings**:
  - Proposed UNHEADED_METRIC_V1 (Type 0x2A): 52 bytes in HbH extension header
  - Practical sweet spot: 103 bytes total (HbH + Flow Label + flags)
  - Flow Label fast-path: [SVC:4][STATUS:4][LATENCY_BUCKET:8][FLAGS:4]
  - RFC 8200 (IPv6), RFC 6437 (Flow Label), RFC 3168 (ECN) compliant
  - Inverse Mask formula: reclaimed = 2 × (128 - host_bits)
  - Kingdom Mode feasibility: /8, /12, /16 modes for different scale
- **Result**: Protocol is RFC-compliant, IANA registration path clear, deployment ready

---

## Port Registry (The Doom Range: 16666-26666)

**Single Source of Truth**: `pkg/ports/ports.go`

All Unheaded services use high ports to avoid conflicts with standard dev tools (SSH, HTTP, DNS, etc).

| Component | Port | Protocol | Layer |
|-----------|------|----------|-------|
| protocol-api | 16666 | gRPC+HTTP | Protocol Handlers |
| dashboard-backend | 16667 | HTTP/WS | Observability |
| kanban-app | 16668 | HTTP/WS | User Interface |
| trace-collector | 16670/16671 | gRPC/HTTP | eBPF Bridge |
| unheaded-daemon | 17000/17001 | HTTP/gRPC | Control Plane |
| wotan | 18000/18001 | HTTP/gRPC | Message Bus |
| timeguru | 19000 | HTTP | Timeline Service |
| architect | 19001 | HTTP | Design Service |
| captain | 19002 | HTTP | Strategy Service |
| micromanager | 19003 | HTTP | Execution Service |
| monad | 19004 | gRPC | State Management |
| sophia | 19005 | gRPC | Knowledge Graph |
| haproxy-edge | 21080/21443/21404 | HTTP/HTTPS/Stats | Edge LB: TLS term, rate limiting |
| haproxy-internal | 21081/21405 | HTTP/Stats | Internal LB: service routing |
| nginx-wotan | 18080 | HTTP | Sidecar LB for wotan |
| nginx-monad | 19080 | HTTP | Sidecar LB for monad |
| nginx-sophia | 19081 | HTTP | Sidecar LB for sophia |
| nginx-dashboard | 20080 | HTTP | Sidecar LB for dashboard |
| nginx-kanban | 20081 | HTTP | Sidecar LB for kanban |
| nginx-gateway | 21090 | HTTP | Sidecar LB for gateway |
| gateway | 21000/21443 | HTTP/HTTPS | TLS Termination |
| zhen-inference | 20100 | HTTP | Zhen inference (llama.cpp/Mistral-7B) |
| zhen-web-ui | 20103 | HTTP | Zhen web UI (Flask RAG) |
| AI Services | 20101-20106 | HTTP | Reserved AI services |
| User Apps | 26000-26666 | HTTP/HTTPS | Reserved |

---

## Authentication Framework

**Location**: `pkg/auth/`

All Unheaded services support pluggable authentication. Development default is **NoopAuthenticator**. Production uses **APIKeyAuthenticator** or **JWTAuthenticator**.

**Usage Pattern**:
```go
authenticator := auth.NewAPIKeyAuthenticator(secretsManager)
router.Use(auth.Middleware(authenticator))
```

**Available Authenticators**:
- **NoopAuthenticator**: Allows all requests (development/testing)
- **APIKeyAuthenticator**: Static API keys (staging/simple deployments)
- **JWTAuthenticator**: Federated JWT tokens (production, OAuth 2.0 compatible)
- **RBACAuthorizer**: Role-based access control on top of any authenticator

**Best Practice**: Swap authenticators via environment variable or config file. Don't hardcode auth backend selection.

---

**Last Updated**: March 19, 2026 (Age 3 Launch Sprint — RFC blockers fixed, 2 new Internet-Drafts, ADR-69420 Kingdom BGP/OS)
**Version**: Alpha (✅ COMPLETE) → Age 2 (✅ COMPLETE) → Age 3 IN PROGRESS — Wire Format FROZEN, Dual Bare Metal Online, Zhen AI Online, Public Launch Ready
**Status**: WEST + EAST bare metal online. Zhen AI operational (1.52M vectors, Mistral-7B inference). UPC Level 6 built. The Well (PostgreSQL multi-DB). SBOM audited. All P1 bugs fixed. 96 commits March 15-19. RFC blockers cleared across Foundation-06, Sophia-03, Wotan-03. Sleipnir (BGP) + Yggdrasil (Hardened OS) vision documented in ADR-69420.
**LOC**: ~385K production, ~941K total. 34 services, 23 eBPF programs.
