# System Architecture

## Overview

Unheaded is a configuration management automation platform built on 6 layers, from bare metal infrastructure up through a user-facing dashboard. The platform delivers production-ready infrastructure for SaaS applications, with eBPF-based observability at every hop and NIST 800-207 Zero Trust Architecture by architectural design.

---

## The 6 Layers

```
Layer 5: User Interface
  Dashboard (packet flow visualization, system metrics)
  Kanban App (the Meta Moment -- Unheaded building Unheaded)
  Wiki Server (documentation, rendered markdown)

Layer 4: Application Services
  timeguru    -- Timeline tracking, triple-format sync (MD/JSON/YAML)
  captain     -- Strategy and vision service
  architect   -- Infrastructure design service
  micromanager -- Execution and QA service
  monad       -- Unified state management
  sophia      -- Knowledge graph and dictionary service
  dashboard-backend -- Metrics aggregation, WebSocket streaming
  kanban-app  -- Kanban board with Wotan integration

Layer 3: Infrastructure Services
  wotan       -- Message bus (gRPC + ring buffer + pub/sub)
  trace-collector -- eBPF ring buffer reader, trace correlation
  gateway     -- nginx, TLS termination, HTTP/3, gRPC-Web proxy
  doom-bridge -- Framebuffer reader, WebSocket frame streaming

Layer 2: Control Plane
  unheaded-daemon -- Container orchestration, eBPF program management,
                     state enforcement, drift detection (30s poll),
                     auto-remediation, Wotan telemetry

Layer 1: Data Plane (eBPF)
  packet_marker  -- Trace ID injection at XDP layer
  flow_tracker   -- Connection tracking per-flow
  latency_probe  -- RTT measurement
  monad_cpu      -- MBC bytecode interpreter (Doom execution engine)

Layer 0: Infrastructure
  LXD / containerd / NixOS / Docker (interchangeable runtimes)
  Host OS (Linux, kernel 6.x+)
  Network: lxdbr0 bridge (10.10.10.0/24)
```

---

## Monad Wire Format

The Monad protocol extends IPv6 with a Hop-by-Hop extension header carrying a 20-byte proprietary register file. Every packet inside the Kingdom carries this metadata. Shield nodes at the boundary strip it on egress -- the outside world never sees it.

### Packet Structure (78 bytes total for Doom ring)

```
 0                   14                  54               62         78
 |  Ethernet (14B)  |   IPv6 (40B)     | HbH (8B)      | Monad (16B)
 |                  |                   |                |
 | dst_mac, src_mac | version, flow_label| opt_type=0x1E | instance_id
 | ethertype=0x86DD | payload_len       | opt_len        | bounce_cnt
 |                  | next_hdr=0 (HbH)  | pad            | metadata
 |                  | hop_limit=64      |                |
 |                  | src_addr (fd00::) |                |
 |                  | dst_addr (fd00::) |                |
```

### IPv6 Flow Label

The 20-bit IPv6 flow label identifies the compute instance. For Doom, flow label `0xDE` marks all packets belonging to the Doom game instance. In production, flow labels will correlate traces across the packet path.

### Protocol Containment

The Monad protocol exists only inside the Kingdom network boundary:

1. **Ingress (Shield):** Clean IPv4/IPv6 arrives. Shield's XDP program stamps the 20-byte Monad register ON. Packet becomes a Kingdom citizen.
2. **Transit:** Every hop reads, computes on, and updates the Monad bytes. The Wotan ring buffer records every touch.
3. **Egress (Shield):** Shield strips the 20 bytes. Clean packet exits. The next-hop router sees standard, RFC-compliant IPv4/IPv6.

The protocol cannot leak. It is born at the gate and dies at the gate.

---

## eBPF Execution Model

### XDP Processing

All eBPF programs attach at the XDP (eXpress Data Path) hook -- the earliest point in the Linux network stack, before `sk_buff` allocation. This gives sub-microsecond per-packet processing.

For the Doom proof of concept, the `monad_cpu` XDP program:

1. Reads the Monad header from the incoming packet
2. Loads CPU state from `CPU_MAP`
3. Executes `MAX_INSN_PER_TICK` (128) MBC instructions from `ROM_MAP`
4. Writes updated CPU state back to `CPU_MAP`
5. Writes screen pixels to `SCREEN_MAP` (on SYSCALL for screen write)
6. Increments bounce counter in the Monad header
7. Returns `XDP_TX` (bounce to next namespace) or `XDP_PASS` (exit ring at 255 bounces)

### Instruction Budget

BPF programs have a verifier instruction limit (historically 1M, now larger). The `monad_cpu` program processes 128 MBC instructions per XDP invocation, keeping well within verifier bounds while maximizing throughput.

| Parameter | Value |
|-----------|-------|
| Instructions per XDP call | 128 |
| Bounces per packet | 255 |
| Instructions per packet | 32,640 |
| XDP calls per packet | 255 x 6 namespaces = 1,530 |
| BPF program size | ~21 KB |

### Shared BPF Maps

The BPF program is loaded once on hop0 using the Aya eBPF loader (Rust), then attached to hops 1-5 via `bpftool net attach xdpgeneric`. All 6 hops share the same `prog_id` and the same pinned maps. This means a write to `RAM_MAP` on hop 3 is visible to the program executing on hop 4 -- shared memory across the ring.

Maps are pinned at: `/sys/fs/bpf/unheaded/doom-ring/maps/`

---

## Service Mesh (Wotan)

### Overview

Wotan is the central message bus and coordination layer. All services communicate exclusively through Wotan -- no direct service-to-service calls. Wotan serves three roles:

1. **Ring Buffer:** High-throughput event stream (eBPF traces, metrics)
2. **Event Bus:** Pub/sub topic system (service state changes, alerts)
3. **Protocol RAM:** Shared state accessible to all services (Sophia dictionaries, config)

### Communication Pattern

```
Service A                    Wotan                    Service B
    |                          |                          |
    |-- Publish("topic.X") -->|                          |
    |                          |-- Deliver("topic.X") -->|
    |                          |                          |
    |                          |<-- Subscribe("topic.Y")-|
    |<-- Deliver("topic.Y") --|                          |
```

### Required Topics

| Topic | Publisher | Subscribers | Purpose |
|-------|-----------|-------------|---------|
| `alerts.critical` | Any service | All services | Critical alert broadcast |
| `timeline.updates` | timeguru | kanban-app, dashboard | Timeline changes |
| `system.outage.reports` | Any service | dashboard, unheaded-daemon | Health degradation |
| `metrics.service.*` | Each service | dashboard-backend | Per-service metrics |

### Health Monitoring Protocol

Every service health-checks all services it depends on. Failures are reported to `system.outage.reports`. Severity is determined by percentage-based consensus:

| % Reporting | Severity | Action |
|-------------|----------|--------|
| 0% - 12.49% | OK | Healthy |
| 12.50% - 37.49% | WARN | Log, email |
| 37.50% - 62.49% | ERROR | Log, second email |
| 62.50% - 87.49% | CRITICAL | Auto-remediate |
| 87.50% - 100% | PANIC | All hands, PagerDuty |

Formula: `(unique_reporters / total_dependent_services) x 100`

---

## Network Design

### Internal Network

| Address | Service | Port |
|---------|---------|------|
| 10.10.10.10 | Wotan (message hub) | 9090 (gRPC) |
| 10.10.10.20 | timeguru | 8000 |
| 10.10.10.21 | captain | 8001 |
| 10.10.10.22 | architect | 8002 |
| 10.10.10.23 | micromanager | 8003 |
| 10.10.10.24 | monad | 8004 |
| 10.10.10.25 | sophia | 8005 |
| 10.10.10.100 | Gateway (nginx) | 443 (HTTPS), 80 (HTTP) |
| 10.10.10.200+ | User apps | variable |

### Doom Ring Network (separate namespace topology)

| Namespace | veth pair | IPv6 prefix |
|-----------|-----------|-------------|
| monad0 | veth01 | fd00:3f:75:0::/64 |
| monad1 | veth12 | fd00:3f:75:1::/64 |
| monad2 | veth23 | fd00:3f:75:2::/64 |
| monad3 | veth34 | fd00:3f:75:3::/64 |
| monad4 | veth45 | fd00:3f:75:4::/64 |
| monad5 | veth50 | fd00:3f:75:5::/64 |

---

## Container Hardening

Every service container runs with NixOS-enforced hardening:

- **Capabilities:** Minimum required only (`CAP_NET_BIND_SERVICE` for most services)
- **No privilege escalation:** `NoNewPrivileges = true`
- **Filesystem isolation:** `PrivateTmp`, `ProtectSystem = "strict"`, `ProtectHome`, read-only `/etc` and `/usr`
- **Seccomp filtering:** `@system-service` allowlist, `~@privileged` and `~@resources` denied
- **Process isolation:** `PrivateDevices`, `ProtectKernelTunables`, `ProtectControlGroups`
- **Network policies:** Default DENY, explicit allow for container network (10.10.10.0/24)

### Interchangeable Runtimes

The same hardening baseline applies regardless of container runtime:

| Runtime | Config Format | Use Case |
|---------|--------------|----------|
| NixOS | Nix expressions | Reference implementation (declarative, reproducible) |
| Docker | Dockerfile + compose | Development, CI/CD |
| containerd | CRI manifests | Kubernetes integration |
| LXD | Cloud-init + profiles | Primary orchestration target |

---

## IaC Output Strategy

Unheaded generates configuration artifacts for the user's preferred toolchain. The control plane maintains a single desired-state model; IaC backends are interchangeable output renderers:

| Backend | Output Format | Use Case |
|---------|--------------|----------|
| Ansible | Playbooks, roles, inventory | Agentless push-based config |
| Terraform | HCL modules, providers | Cloud infrastructure provisioning |
| Puppet | Manifests, Hiera data | Agent-based declarative config |
| Kubernetes | Manifests, Helm charts | Container orchestration at scale |
| Chef | Cookbooks, recipes | Ruby-based config management |
| Salt | States, pillars, grains | Event-driven, high-speed config |

---

## Observability Stack

### Internal Signals

All services emit OpenTelemetry-compatible signals:

- **Metrics:** Prometheus-native counters, histograms, gauges
- **Logs:** Structured JSON via zerolog (service, request_id, trace_id, level)
- **Traces:** Distributed tracing with trace_id propagation through Wotan

### Required Endpoints (Every Service)

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Health check (200 if healthy) |
| `GET /ready` | Readiness probe (200 if ready to serve) |
| `GET /metrics` | Prometheus metrics scrape |
| `GET /api/v1/*` | Service-specific REST API |

### Interchangeable Backends

| Category | Supported | Default (Future) |
|----------|-----------|-------------------|
| Metrics | Prometheus, Grafana, Datadog, InfluxDB, Nagios | Custom Wotan metrics store |
| Logging | ELK, Fluentd, Splunk, Loki, Graylog | Custom Wotan log aggregator |
| Tracing | Jaeger, Zipkin, Tempo, Datadog APM | Custom eBPF-native tracer |
| Alerting | Grafana Alerting, PagerDuty, OpsGenie, Nagios | Custom Wotan alert engine |
| Dashboards | Grafana, Kibana, Datadog, custom | Unheaded Dashboard (vanilla JS) |

---

*See also: [Protocol Specifications](protocol-specs.md) | [Doom over IPv6](doom-over-ipv6.md) | [Security](security.md)*
