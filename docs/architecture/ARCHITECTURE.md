# Unheaded Alpha Architecture

**Version:** Alpha
**Last Updated:** January 26, 2026

## Overview

Unheaded is a configuration management automation platform that delivers production-ready infrastructure in hours, not months. This document describes the alpha architecture demonstrating core capabilities.

## Design Principles

1. **Security First** - eBPF traceability, immutable infrastructure, zero user data access
2. **Observable by Default** - Packet-level visibility from L2-L7
3. **Declarative Everything** - NixOS containers, version-controlled configs
4. **Self-Hosting** - Self-hosting validation (Kanban app proves it)
5. **Modern Stack** - HTTP/3, QUIC, gRPC, eBPF, Rust, Go

## Architecture Layers

```
┌────────────────────────────────────────────────────────────────┐
│ Layer 5: USER INTERFACE                                         │
│ ┌────────────────┐ ┌────────────────┐                          │
│ │   Dashboard    │ │   Kanban App   │                          │
│ │  (Packet viz,  │ │  (Meta moment) │                          │
│ │   metrics)     │ │                │                          │
│ └────────────────┘ └────────────────┘                          │
└────────────────────────────────────────────────────────────────┘
                             ↓
┌────────────────────────────────────────────────────────────────┐
│ Layer 4: APPLICATION SERVICES                                   │
│ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│ │ timeguru │ │ captain  │ │microman. │ │architect │           │
│ │          │ │          │ │          │ │          │           │
│ │ Timeline │ │ Strategy │ │ Tasks &  │ │ Infra    │           │
│ │ tracking │ │ & vision │ │ QA       │ │ design   │           │
│ └──────────┘ └──────────┘ └──────────┘ └──────────┘           │
│                                                                 │
│ ┌──────────────────────┐ ┌──────────────────────┐             │
│ │  dashboard-backend   │ │    demo-app          │             │
│ │  (Metrics + WS)      │ │    (User sim)        │             │
│ └──────────────────────┘ └──────────────────────┘             │
└────────────────────────────────────────────────────────────────┘
                             ↓
┌────────────────────────────────────────────────────────────────┐
│ Layer 3: INFRASTRUCTURE SERVICES                                │
│ ┌──────────────────────┐ ┌──────────────────────┐             │
│ │       wotan         │ │   trace-collector    │             │
│ │   (Message bus)      │ │   (eBPF → Wotan)    │             │
│ │                      │ │                      │             │
│ │ • gRPC streaming     │ │ • Rust performance   │             │
│ │ • Pub/sub topics     │ │ • Ring buffer read   │             │
│ │ • Ring buffer        │ │ • Trace correlation  │             │
│ └──────────────────────┘ └──────────────────────┘             │
│                                                                 │
│ ┌──────────────────────┐                                       │
│ │       gateway        │                                       │
│ │   (nginx HTTP/3)     │                                       │
│ │                      │                                       │
│ │ • TLS termination    │                                       │
│ │ • gRPC-Web proxy     │                                       │
│ │ • WebSocket proxy    │                                       │
│ └──────────────────────┘                                       │
└────────────────────────────────────────────────────────────────┘
                             ↓
┌────────────────────────────────────────────────────────────────┐
│ Layer 2: CONTROL PLANE                                          │
│ ┌────────────────────────────────────────────────────────────┐ │
│ │ unheaded-daemon (systemd service on host)                  │ │
│ │                                                            │ │
│ │ Responsibilities:                                          │ │
│ │ • LXD container orchestration                              │ │
│ │ • eBPF program loading/unloading                           │ │
│ │ • State enforcement (immutability)                         │ │
│ │ • Drift detection and remediation                          │ │
│ │ • Telemetry reporting to Wotan                            │ │
│ │ • Health monitoring                                        │ │
│ └────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────┘
                             ↓
┌────────────────────────────────────────────────────────────────┐
│ Layer 1: DATA PLANE (eBPF)                                      │
│ ┌────────────────────────────────────────────────────────────┐ │
│ │ eBPF Programs (XDP + kprobe)                               │ │
│ │                                                            │ │
│ │ • packet_marker.bpf    - Trace ID injection at XDP        │ │
│ │ • flow_tracker.bpf     - Connection tracking              │ │
│ │ • latency_probe.bpf    - RTT measurement                  │ │
│ │                                                            │ │
│ │ All written in Rust for safety + performance              │ │
│ └────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────┘
                             ↓
┌────────────────────────────────────────────────────────────────┐
│ Layer 0: INFRASTRUCTURE                                         │
│ ┌────────────────────────────────────────────────────────────┐ │
│ │ LXD Hypervisor                                             │ │
│ │ • NixOS containers (immutable, declarative)                │ │
│ │ • Network: lxdbr0 (10.10.10.0/24)                          │ │
│ │ • Storage: ZFS or dir backend                              │ │
│ └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ ┌────────────────────────────────────────────────────────────┐ │
│ │ Host OS (any Linux distro)                                 │ │
│ │ • Kernel 5.8+ (eBPF support)                               │ │
│ │ • Supports: Bare metal, major cloud providers, VMware      │ │
│ └────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────┘
```

## Network Architecture

### IP Addressing

```
lxdbr0: 10.10.10.1/24 (bridge)

Container IPs:
├── wotan:            10.10.10.10
├── trace-collector:   10.10.10.11
├── timeguru:          10.10.10.20
├── captain:           10.10.10.21
├── micromanager:      10.10.10.22
├── architect:         10.10.10.23
├── dashboard-backend: 10.10.10.30
├── gateway:           10.10.10.100
├── kanban-app:        10.10.10.200
└── demo-app:          10.10.10.254
```

### Port Assignments

```
wotan:
  - 8080: HTTP REST API
  - 9090: gRPC streaming

trace-collector:
  - 8081: HTTP metrics endpoint

gateway:
  - 443: HTTPS/HTTP3 (external)
  - 80: HTTP redirect to HTTPS

dashboard-backend:
  - 8082: HTTP API
  - 8083: WebSocket

timeguru/captain/micromanager/architect:
  - 8000: HTTP REST API

kanban-app:
  - 8001: HTTP

demo-app:
  - 8000: HTTP
```

### Traffic Flow

```
Browser (external)
    ↓ HTTPS/HTTP3
Gateway (10.10.10.100:443)
    ↓ HTTP/gRPC-Web/WebSocket
┌───────────────┬───────────────┬───────────────┐
│               │               │               │
Dashboard       Kanban         Services
Backend         App            (timeguru, etc)
│               │               │
└───────────────┴───────────────┴───────────────┘
                ↓
            Wotan (10.10.10.10)
                ↓
        trace-collector (10.10.10.11)
                ↓
         eBPF Ring Buffer
```

## Message Bus (Wotan) Topics

All services communicate via Wotan pub/sub:

| Topic | Publisher | Subscribers | Purpose |
|-------|-----------|-------------|---------|
| `network.traces` | trace-collector | dashboard-backend | eBPF packet traces |
| `system.metrics` | all containers | dashboard-backend | Container health |
| `timeline.updates` | timeguru | dashboard, kanban | Roadmap changes |
| `strategy.decisions` | captain | all services | Strategic guidance |
| `tasks.assignments` | micromanager | all services | Task tracking |
| `design.proposals` | architect | all services | Architecture decisions |
| `alerts.critical` | any | all | Critical alerts |
| `logs.aggregated` | all | dashboard-backend | Centralized logging |

## Data Flow: eBPF to Dashboard

```
1. Packet arrives at host network interface
       ↓
2. XDP hook (packet_marker.bpf) injects trace_id
       ↓
3. Packet continues to destination container
       ↓
4. eBPF program writes event to ring buffer
       ↓
5. trace-collector (Rust) reads ring buffer
       ↓
6. trace-collector publishes to Wotan topic "network.traces"
       ↓
7. dashboard-backend subscribes, receives event
       ↓
8. dashboard-backend aggregates + correlates traces
       ↓
9. WebSocket pushes update to browser
       ↓
10. Dashboard renders real-time packet flow visualization
```

**Latency target:** < 50ms end-to-end (packet → dashboard)

## The Meta Moment: Kanban App

### Purpose
Demonstrates that Unheaded can reliably host and manage its own development infrastructure.

### Architecture

```
Browser
    ↓ HTTP/3
Gateway
    ↓ HTTP
Kanban App (10.10.10.200)
    ↓ HTTP REST
Timeguru API (10.10.10.20:8000)
    ↓ File I/O
/opt/unheaded/references/timeline.md
    ↓ Auto-sync
timeline.json, timeline.yaml
```

### Flow

1. User visits `https://<host>/kanban`
2. Kanban app fetches `GET /api/v1/timeline` from timeguru
3. Timeguru reads `references/timeline.md`
4. Timeguru returns JSON representation
5. Kanban app renders interactive board
6. (Optional) User interaction writes to `/tmp/kanban-config.yaml`
7. eBPF traces entire request flow
8. Dashboard shows packet journey through full stack

### Visual Design

- Inspired by bellis.tech (particle canvas background)
- Dark theme, clean typography
- Columns: TODO, IN PROGRESS, DONE
- Real-time live indicator
- Header: "Unheaded Alpha - Built by Unheaded 🔄"

## Security Architecture

### Container Isolation

```
demo-app (user simulation)
    ↑ Network policy: DENY all except gateway
    ↑ No access to Unheaded internals
    ↑ Separate network namespace
    ↑ Read-only root filesystem
```

### eBPF Safety

- All eBPF programs verified by kernel verifier
- Written in Rust (memory safety)
- Cannot crash kernel or leak data
- Bounded loops, no unbounded recursion

### Hardening

Applied to all containers:

```nix
{
  # Capabilities
  systemd.services.<service>.serviceConfig = {
    CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
    NoNewPrivileges = true;
  };

  # Seccomp
  systemd.services.<service>.serviceConfig.SystemCallFilter = [
    "@system-service"
    "~@privileged"
  ];

  # Filesystem
  systemd.services.<service>.serviceConfig = {
    ProtectSystem = "strict";
    ProtectHome = true;
    ReadOnlyPaths = [ "/etc" "/usr" ];
  };

  # Network
  networking.firewall.enable = true;
  networking.firewall.allowedTCPPorts = [ <service-port> ];
}
```

### TLS

- All external traffic: TLS 1.3 minimum
- Inter-service: mTLS (future)
- Certificates: Let's Encrypt (production) or self-signed (dev)

## State Management

### Desired State

Stored in Git:

```
nix/containers/*.nix      → Container definitions
references/timeline.md    → Roadmap (source of truth)
references/strategy.md    → Strategic decisions
```

### Actual State

Tracked by unheaded-daemon:

```
/var/lib/unheaded/state/
├── containers.json       → Running containers
├── ebpf-programs.json    → Loaded eBPF programs
└── metrics.json          → Current metrics snapshot
```

### Drift Detection

```
unheaded-daemon polls every 30 seconds:
1. Compare desired (Nix) vs actual (LXD)
2. If drift detected:
   - Log event to Wotan
   - Auto-remediate (restart container with correct config)
   - Alert dashboard
```

## Performance Targets

| Metric | Target | Notes |
|--------|--------|-------|
| eBPF overhead | < 5% CPU | Per-packet processing |
| Message latency | < 5ms | Wotan publish → subscribe |
| Dashboard latency | < 50ms | Packet → browser |
| Container startup | < 10s | NixOS container boot |
| API response | < 100ms | p99 for REST endpoints |

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Container crash | unheaded-daemon health check | Automatic restart |
| eBPF program crash | Kernel verifier prevents | N/A (cannot happen) |
| Wotan unavailable | Service health checks | Restart, queue locally |
| Network partition | Connection timeouts | Retry with backoff |
| Disk full | Metrics threshold | Alert, rotate logs |
| Memory exhaustion | cgroup limits | OOM kill container, restart |

## Technology Choices

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| eBPF programs | Rust (Aya framework) | Memory safety, performance |
| Services | Go | Simplicity, concurrency, tooling |
| Containers | NixOS | Declarative, immutable, reproducible |
| Message bus | Wotan (Go + gRPC) | Custom, high-perf, proven in Phase 1 |
| Gateway | nginx | Battle-tested, HTTP/3 support |
| Frontend | Vanilla JS | No framework overhead, full control |
| Orchestration | LXD | Lightweight, system containers |

## Future Enhancements

### Phase 2: Production Hardening
- Message persistence (WAL)
- Multi-node clustering
- Geographic distribution
- Advanced RBAC

### Phase 3: Compliance
- FEDRAMP baseline
- NIST controls
- SOC2 audit support
- Automated compliance reporting

### Phase 4: Advanced Observability
- Distributed tracing (OpenTelemetry)
- Log analytics (ClickHouse)
- Anomaly detection (ML)
- Predictive alerting

### Phase 5: Full IDP
- Service registry
- Circuit breakers
- Rate limiting per-service
- API gateway features

## References

- [MICROSERVICES.md](MICROSERVICES.md) - Service catalog and communication patterns
- [SECURITY.md](SECURITY.md) - Detailed security specifications
- [THE_META_MOMENT.md](THE_META_MOMENT.md) - Philosophy of self-hosting
- [../references/timeline.md](../references/timeline.md) - Living roadmap

---

**Self-hosting is proof, not marketing.**

Architecture designed and implemented by the Unheaded team (with significant assistance from our AI pair programmers).
