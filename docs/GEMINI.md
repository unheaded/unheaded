# GEMINI.md - Unheaded Alpha Development Guide

**For:** Gemini AI agents working on Unheaded
**Updated:** January 26, 2026
**Project:** github.com/unheaded/unheaded

---

## 🎯 Project Vision

**"Production-ready infrastructure in hours, not months."**

Unheaded is a configuration management automation platform delivering complete infrastructure for modern SaaS applications. User brings their app ("the head"), we provide everything else ("unheaded").

**Core Capabilities:**
- ✅ eBPF-based observability (L2-L7 tracing)
- ✅ Immutable NixOS infrastructure
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
| eBPF programs | **Rust** (Aya framework) | Memory safety + performance for kernel |
| Services | **Go 1.21+** | Simplicity, concurrency, tooling |
| Containers | **NixOS** | Declarative, immutable, reproducible |
| Message Bus | **Wotan** (Go + gRPC) | Proven in Phase 1, custom |
| Gateway | **nginx** | Battle-tested, HTTP/3 support |
| Frontend | **Vanilla JS** | No framework overhead, full control |
| Orchestration | **LXD** | Lightweight system containers |

### Network Design

- **Bridge:** lxdbr0 (10.10.10.0/24)
- **Gateway:** 10.10.10.100 (TLS termination, HTTP/3)
- **Wotan:** 10.10.10.10 (message hub)
- **Services:** 10.10.10.20-30 (agent services)
- **Apps:** 10.10.10.200+ (kanban, demo)

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
- Publish metrics to Prometheus
- Log structured JSON (zerolog)
- Report to Wotan message bus
- Support distributed tracing
- Expose /health and /ready endpoints

**eBPF traces everything:**
- Packet markers at XDP layer
- Connection tracking
- Latency measurements
- All correlated by trace_id

### 3. Declarative Everything

**All config in version control:**
- NixOS container definitions (nix/containers/*.nix)
- Service configs (YAML/TOML)
- Network policies
- Security baselines
- Timeline and strategy (MD/JSON/YAML)

**State management:**
- Desired state: Git (Nix configs)
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
│   └── architect/        # Design service (Go)
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

Co-Authored-By: Gemini <noreply@google.com>
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

Co-Authored-By: Gemini <noreply@google.com>
```

---

## 🎯 Current Phase: Alpha

**Goal:** Demonstrate core platform capabilities

**Timeline:** Jan 26 - Feb 1, 2026 (6 days with max parallelization)

**Critical Path:**
1. Services (timeguru, captain, micromanager, architect) - Day 1
2. Kanban app + dashboard-backend - Day 2
3. unheaded-daemon + NixOS containers - Day 2-3
4. Dashboard UI + gateway - Day 3
5. eBPF programs + trace-collector - Day 4-5
6. Integration + polish - Day 6

**Success Criteria:**
- [ ] All services communicating via Wotan
- [ ] Kanban app displays timeline from timeguru
- [ ] eBPF traces end-to-end packet flow
- [ ] Dashboard visualizes traces in real-time
- [ ] Publicly accessible (optional auth)
- [ ] Sub-50ms latency (packet → browser)
- [ ] Containers start in <10s
- [ ] Zero user data access (validated)

---

## 🧠 Working with Gemini Agents

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
- Read GEMINI.md for standards
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

**Last Updated:** January 26, 2026
**Version:** Alpha Foundation
**Status:** Ready for parallel implementation 🚀
