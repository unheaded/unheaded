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
  kanban-app  -- Kanban board with Wotan integration (cards from The Well)
  akira       -- Cross-service health checks + percentage consensus (ADR-029)
  cuirass     -- Control-plane service

Layer 3: Infrastructure Services
  wotan       -- Message bus (gRPC + ring buffer + pub/sub)
  trace-collector -- eBPF ring buffer reader, trace correlation
  gateway     -- Traefik in compose (edge 80/443, gRPC 50051); nginx sidecars in other substrates
  doom-bridge -- Framebuffer reader, WebSocket frame streaming
  The Well    -- PostgreSQL 16, three databases, TLS only ([[The Well|The-Well]])
  observe     -- VictoriaMetrics, ClickHouse, Vector, Grafana
  huginn      -- per-host metrics agent (systemd, not a container; ADR-084)

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
  Network: compose bridges control/data/observe (172.28.0-2.0/24); LXD lxdbr0 (10.10.10.0/24) in lxd/
```

---

## Monad Wire Format

The Monad protocol extends IPv6 with a Hop-by-Hop extension header carrying a 20-byte proprietary register file. Every packet inside the Kingdom carries this metadata. Shield nodes at the boundary strip it on egress -- the outside world never sees it.

### Packet Structure (78 bytes total for Doom ring)

```
 0                   14                  54                  58          78
 |  Ethernet (14B)  |   IPv6 (40B)      | HbH hdr + opt (4B) | Monad (20B)
 |                  |                   |                    |
 | dst_mac, src_mac | version, flow_label| next_hdr, hdr_len | register file
 | ethertype=0x86DD | payload_len       | opt_type=0x3E      | (instance, bounce
 |                  | next_hdr=0 (HbH)  | opt_data_len=20    |  count, metadata)
 |                  | hop_limit=64      |                    |
 |                  | src_addr (fd00::) |                    |
 |                  | dst_addr (fd00::) |                    |
```

Option type `0x3E` is action `00` (skip if unknown), change-en-route `1`, number `0x1E` (`ebpf/monad-common/src/lib.rs` `MONAD_OPT_TYPE`). The XDP program passes any packet whose option type or data length differs.

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
3. Executes up to `MAX_INSN_PER_TICK` (16) MBC instructions from `ROM_MAP` per round, and tail-calls itself for `--chain-depth` rounds (default 16, `crates/doom-runner`): up to 256 per packet
4. Writes updated CPU state back to `CPU_MAP`
5. Writes screen pixels to `SCREEN_MAP` (on SYSCALL for screen write)
6. Increments bounce counter in the Monad header
7. Returns `XDP_TX` (bounce to next namespace) or `XDP_PASS` (exit ring at 255 bounces)

### Instruction Budget

BPF programs have a verifier instruction limit of 1M. `MAX_INSN_PER_TICK` is 16 (`ebpf/monad-cpu-ebpf/src/main.rs`): each loop iteration costs thousands of verifier states, and 256 exceeds the limit. Each tail call gets a fresh verifier budget, so throughput comes from chaining (16 x 16 = 256 per packet) and from the packet rate of the injector. A packet leaves the ring after 255 bounces.

The ring has `DOOM_RING_HOPS` namespaces (`scripts/doom-ring.sh`, default 2; the script's diagram and the table below show 6).

### Shared BPF Maps

The BPF program is loaded once on hop0 using the Aya eBPF loader (Rust), then attached to the other hops via `bpftool net attach xdpgeneric` (`scripts/doom-xdp-attach.sh`). All 6 hops share the same `prog_id` and the same pinned maps. This means a write to `RAM_MAP` on hop 3 is visible to the program executing on hop 4 -- shared memory across the ring.

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
| `alerts.critical` | Any service | Services that wire it (timeguru, captain, micromanager, chaos-controller, …) | Critical alert broadcast |
| `timeline.updates` | timeguru | kanban-app | Timeline changes |
| `system.health.reports` | akira, every service with `discovery.Start` (`pkg/healthreport`) | akira (tally), dashboard-backend (into The Well) | Each reporter's checks, and each service's check of itself |
| `system.health.consensus` | akira | dashboard-backend | Health band changes |
| `system.health.remediate` | akira | none yet (advisory) | One restart suggestion per entry into CRITICAL or PANIC |
| `metrics.<service>` | dashboard-backend event helper | dashboard-backend (`metrics.*`) | Per-service metric events |

### Health Monitoring Protocol

Every service health-checks the services it depends on. Reports go to Wotan
topic `system.health.reports`; band changes go to `system.health.consensus`
(ADR-029). Akira (`cmd/akira`, port 19100) checks every service, tallies all
reporters' votes and publishes the band.

| % Reporting | Severity | Action |
|-------------|----------|--------|
| 0% - 12.49% | OK | Healthy |
| 12.50% - 37.49% | WARN | Log, email |
| 37.50% - 62.49% | ERROR | Log, second email |
| 62.50% - 87.49% | CRITICAL | Auto-remediate from 66.67% (two-thirds) |
| 87.50% - 100% | PANIC | All hands, PagerDuty |

Formula: `failing_reporters / reporters x 100`; each reporter's latest report
from the last three sweeps is one vote. Remediation (`systemctl restart`, at
most 3 per outage) is off unless Akira runs with `--remediate`; the compose
stack runs it for detection only.

---

## Network Design

### Internal Network (Docker Compose)

Three bridge networks; services publish their Doom Range port (see
[[Port Registry|Port-Registry]]):

| Network | Subnet | Members |
|---------|--------|---------|
| `control` | 172.28.0.0/24 | wotan, cuirass, coredns, traefik |
| `data` | 172.28.1.0/24 | every Kingdom service, The Well, akira |
| `observe` | 172.28.2.0/24 | VictoriaMetrics, ClickHouse, Vector, Grafana, dashboard-backend |

The Well publishes `127.0.0.1:5432` only; ClickHouse `127.0.0.1` only.
The older LXD design (`lxdbr0`, 10.10.10.0/24) remains in `lxd/`.

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

In compose every container runs as its own uid (`pkg/uids`) with `read_only`, `cap_drop: ALL` and `no-new-privileges` (`scripts/check-compose-hardening.sh`); systemd units use their own nologin accounts ([[Systemd Units|Systemd-Units]]). The NixOS definitions in `nix/containers/` carry the equivalent systemd settings:

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

*See also: [[Protocol Specifications|Protocol-Specs]] | [[Doom on UPC|Doom-on-UPC]] | [[Security]] | [[The Well|The-Well]]*
