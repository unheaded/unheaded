# Unheaded Alpha - System Diagram

## Complete System Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           EXTERNAL WORLD                                 │
│                                                                          │
│  👤 User Browser                          🌐 Public Internet            │
│      │                                                                   │
│      │ HTTPS/HTTP3                                                      │
│      ↓                                                                   │
└─────────────────────────────────────────────────────────────────────────┘
                               │
                               │ Port 443
                               ↓
┌─────────────────────────────────────────────────────────────────────────┐
│                         BARE METAL / VM HOST                             │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │ eBPF LAYER (XDP + kprobe)                                          │ │
│  │                                                                    │ │
│  │  [packet_marker.bpf] ──→ Inject trace_id at XDP layer            │ │
│  │  [flow_tracker.bpf]  ──→ Track connections                        │ │
│  │  [latency_probe.bpf] ──→ Measure RTT                              │ │
│  │          │                                                         │ │
│  │          ↓ Ring Buffer                                            │ │
│  └──────────┼──────────────────────────────────────────────────────────┘ │
│             │                                                            │
│  ┌──────────┼──────────────────────────────────────────────────────────┐ │
│  │ unheaded-daemon (systemd)                                          │ │
│  │          │                                                         │ │
│  │    • Reads eBPF ring buffer                                       │ │
│  │    • Orchestrates LXD containers                                  │ │
│  │    • Enforces immutable state                                     │ │
│  │    • Reports to Wotan                                            │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │ LXD HYPERVISOR (lxdbr0: 10.10.10.0/24)                            │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
                               │
                               │ LXD networking
                               ↓
┌─────────────────────────────────────────────────────────────────────────┐
│                         NIXOS CONTAINERS (LXD)                           │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │ GATEWAY (10.10.10.100)              [nginx]                      │  │
│  │                                                                   │  │
│  │  :443 ─→ TLS termination, HTTP/3, QUIC                           │  │
│  │  :80  ─→ Redirect to HTTPS                                       │  │
│  │                                                                   │  │
│  │  Routes:                                                          │  │
│  │    /           ─→ dashboard (10.10.10.30)                        │  │
│  │    /kanban     ─→ kanban-app (10.10.10.200)                      │  │
│  │    /api/*      ─→ services (timeguru, captain, etc)              │  │
│  │    /ws         ─→ dashboard-backend (WebSocket)                  │  │
│  │    /grpc       ─→ wotan (gRPC-Web)                              │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                          │                                              │
│                          ↓                                              │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    CORE INFRASTRUCTURE                           │   │
│  │                                                                  │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │   wotan     │  │trace-collect │  │  dashboard-  │          │   │
│  │  │ 10.10.10.10  │  │ 10.10.10.11  │  │   backend    │          │   │
│  │  │              │  │              │  │ 10.10.10.30  │          │   │
│  │  │ :8080 HTTP   │  │ • Reads eBPF │  │              │          │   │
│  │  │ :9090 gRPC   │  │ • Publishes  │  │ :8082 HTTP   │          │   │
│  │  │              │  │   to Wotan  │  │ :8083 WS     │          │   │
│  │  │ Message Bus  │  │              │  │              │          │   │
│  │  │ • Pub/sub    │  │ (Rust)       │  │ • Subscribe  │          │   │
│  │  │ • Topics     │  │              │  │   all topics │          │   │
│  │  │ • Ring buf   │  │              │  │ • Aggregate  │          │   │
│  │  │ • Streaming  │  │              │  │ • WebSocket  │          │   │
│  │  └──────┬───────┘  └──────────────┘  └──────────────┘          │   │
│  │         │                                                       │   │
│  └─────────┼───────────────────────────────────────────────────────┘   │
│            │ Topics:                                                   │
│            │ • network.traces                                          │
│            │ • system.metrics                                          │
│            │ • timeline.updates                                        │
│            │ • strategy.decisions                                      │
│            │ • tasks.assignments                                       │
│            │ • design.proposals                                        │
│            │ • alerts.critical                                         │
│            │                                                           │
│            ↓ Subscribe to topics                                       │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    AGENT SERVICES                                │   │
│  │              (Mirror our dev methodology)                        │   │
│  │                                                                  │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │  timeguru    │  │   captain    │  │ micromanager │          │   │
│  │  │ 10.10.10.20  │  │ 10.10.10.21  │  │ 10.10.10.22  │          │   │
│  │  │              │  │              │  │              │          │   │
│  │  │ :8000 HTTP   │  │ :8000 HTTP   │  │ :8000 HTTP   │          │   │
│  │  │              │  │              │  │              │          │   │
│  │  │ Timeline     │  │ Strategy     │  │ Tasks & QA   │          │   │
│  │  │ tracking     │  │ & vision     │  │              │          │   │
│  │  │              │  │              │  │              │          │   │
│  │  │ R/W:         │  │ R/W:         │  │ R/W:         │          │   │
│  │  │ timeline.md  │  │ strategy.md  │  │ tasks.yaml   │          │   │
│  │  │ timeline.json│  │ strategy.json│  │ tasks.json   │          │   │
│  │  │ timeline.yaml│  │ strategy.yaml│  │              │          │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘          │   │
│  │                                                                  │   │
│  │  ┌──────────────┐                                               │   │
│  │  │  architect   │                                               │   │
│  │  │ 10.10.10.23  │                                               │   │
│  │  │              │                                               │   │
│  │  │ :8000 HTTP   │                                               │   │
│  │  │              │                                               │   │
│  │  │ Infra design │                                               │   │
│  │  │ & patterns   │                                               │   │
│  │  │              │                                               │   │
│  │  │ R/W:         │                                               │   │
│  │  │ designs.md   │                                               │   │
│  │  │ designs.json │                                               │   │
│  │  └──────────────┘                                               │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    THE META MOMENT                               │   │
│  │                                                                  │   │
│  │  ┌──────────────┐                                               │   │
│  │  │  kanban-app  │                                               │   │
│  │  │ 10.10.10.200 │                                               │   │
│  │  │              │                                               │   │
│  │  │ :8001 HTTP   │                                               │   │
│  │  │              │                                               │   │
│  │  │ Serves:      │                                               │   │
│  │  │ • HTML/JS    │                                               │   │
│  │  │ • CSS        │                                               │   │
│  │  │ • Assets     │                                               │   │
│  │  │              │                                               │   │
│  │  │ Reads from:  │                                               │   │
│  │  │ timeguru API ├──────────────┐                               │   │
│  │  │              │              │                               │   │
│  │  │ Displays:    │              ↓                               │   │
│  │  │ Kanban board │   GET /api/v1/timeline                       │   │
│  │  │ showing      │   (reads timeline.md)                        │   │
│  │  │ Unheaded     │                                               │   │
│  │  │ building     │   "Unheaded Alpha - Built by Unheaded 🔄"    │   │
│  │  │ itself!      │                                               │   │
│  │  └──────────────┘                                               │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    CUSTOMER SIMULATION                           │   │
│  │                (Zero access to Unheaded)                        │   │
│  │                                                                  │   │
│  │  ┌──────────────┐                                               │   │
│  │  │  demo-app    │                                               │   │
│  │  │ 10.10.10.254 │                                               │   │
│  │  │              │                                               │   │
│  │  │ :8000 HTTP   │                                               │   │
│  │  │              │                                               │   │
│  │  │ Simulates    │                                               │   │
│  │  │ customer     │                                               │   │
│  │  │ workload     │                                               │   │
│  │  │              │                                               │   │
│  │  │ Network      │                                               │   │
│  │  │ isolated     │                                               │   │
│  │  │ from core    │                                               │   │
│  │  └──────────────┘                                               │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

## Data Flow: Browser to Timeline

```
User clicks "Kanban" in browser
    ↓
1. HTTPS/HTTP3 request to gateway:443
   [eBPF: packet_marker injects trace_id = abc123]
    ↓
2. gateway routes /kanban → 10.10.10.200:8001
   [eBPF: flow_tracker records connection]
    ↓
3. kanban-app serves HTML/JS
   [eBPF: latency_probe measures RTT]
    ↓
4. Browser renders, JS fetches /api/v1/timeline
   [eBPF: new trace_id = def456]
    ↓
5. gateway routes /api/v1/timeline → 10.10.10.20:8000
   [eBPF: correlation captured]
    ↓
6. timeguru reads /opt/unheaded/references/timeline.md
   [Kernel kprobe traces file I/O]
    ↓
7. timeguru returns JSON
   [eBPF: response traced back through gateway]
    ↓
8. Browser receives timeline data
   [Total time: ~50ms]
    ↓
9. Kanban board renders with TODO/IN PROGRESS/DONE columns
    ↓
10. trace-collector publishes eBPF events to Wotan topic "network.traces"
    ↓
11. dashboard-backend receives events via subscription
    ↓
12. WebSocket pushes to dashboard in browser
    ↓
13. Dashboard shows: Browser → gateway → kanban → gateway → timeguru
                      (6 hops, 47ms, trace_id: abc123 + def456)
```

**Every step traced, correlated, and visualized in real-time!**

## Message Flow Through Wotan

```
                         ┌──────────────┐
                         │    WOTAN    │
                         │ (Message Bus)│
                         └───────┬──────┘
                                 │
                 ┌───────────────┼───────────────┐
                 │               │               │
         ┌───────▼──────┐ ┌─────▼──────┐ ┌─────▼──────┐
         │ network.     │ │ timeline.  │ │ system.    │
         │ traces       │ │ updates    │ │ metrics    │
         └───────┬──────┘ └─────┬──────┘ └─────┬──────┘
                 │               │               │
         ┌───────▼──────────┐   │   ┌───────────▼──────┐
         │ trace-collector  │   │   │ All containers   │
         │ (Publisher)      │   │   │ (Publishers)     │
         └──────────────────┘   │   └──────────────────┘
                                │
                        ┌───────▼──────┐
                        │  timeguru    │
                        │ (Publisher)  │
                        └──────────────┘

         Subscribers:
         • dashboard-backend (all topics)
         • kanban-app (timeline.updates)
         • All services (alerts.critical)
```

## Security Boundaries

```
┌────────────────────────────────────────────────┐
│ EXTERNAL ZONE (untrusted)                      │
│   • User browsers                              │
│   • Public internet                            │
└────────────────┬───────────────────────────────┘
                 │ TLS 1.3 only
                 │ Port 443
                 ↓
┌────────────────────────────────────────────────┐
│ DMZ (gateway)                                  │
│   • nginx gateway (10.10.10.100)               │
│   • TLS termination                            │
│   • Rate limiting                              │
│   • WAF (future)                               │
└────────────────┬───────────────────────────────┘
                 │ Internal HTTP/gRPC
                 │ No encryption (yet - mTLS future)
                 ↓
┌────────────────────────────────────────────────┐
│ APPLICATION ZONE (Unheaded services)           │
│   • timeguru, captain, micromanager, architect │
│   • dashboard-backend, kanban-app              │
│   • Network policies: ALLOW within zone        │
└────────────────┬───────────────────────────────┘
                 │ Via Wotan
                 ↓
┌────────────────────────────────────────────────┐
│ INFRASTRUCTURE ZONE (core services)            │
│   • wotan, trace-collector                    │
│   • Network policies: restricted access        │
└────────────────────────────────────────────────┘

┌────────────────────────────────────────────────┐
│ CUSTOMER ZONE (isolated)                       │
│   • demo-app                                   │
│   • Network policies: DENY all except gateway  │
│   • Zero access to Unheaded internals          │
└────────────────────────────────────────────────┘
```

## Container Dependency Graph

```
unheaded-daemon
    │
    ├─→ Starts: wotan (first, must be ready before others)
    │
    ├─→ Starts: trace-collector (needs eBPF loaded)
    │       └─→ Publishes to: wotan
    │
    ├─→ Starts: gateway (needs to route traffic)
    │
    ├─→ Starts: timeguru, captain, micromanager, architect (parallel)
    │       └─→ All subscribe to: wotan
    │
    ├─→ Starts: dashboard-backend
    │       └─→ Subscribes to: all Wotan topics
    │
    ├─→ Starts: kanban-app
    │       └─→ Reads from: timeguru
    │
    └─→ Starts: demo-app (isolated, last)
```

## File System Layout

```
Host:
/opt/unheaded/
├── bin/
│   ├── unheaded-daemon
│   ├── trace-collector
│   ├── dashboard-backend
│   └── kanban-app
├── config/
│   ├── daemon.yaml
│   └── containers.yaml
├── data/
│   └── state/
│       ├── containers.json
│       └── ebpf-programs.json
└── logs/
    ├── daemon.log
    └── containers/

/var/lib/unheaded/
├── state/
└── ebpf/

Container (timeguru):
/opt/unheaded/
├── bin/
│   └── timeguru
├── config/
│   └── timeguru.yaml
└── references/
    ├── timeline.md       ← Source of truth
    ├── timeline.json     ← Auto-generated
    └── timeline.yaml     ← Auto-generated
```

## Port Map (Complete)

| Container | IP | Ports | Purpose |
|-----------|----|----|---------|
| gateway | 10.10.10.100 | 443 (HTTPS/HTTP3), 80 (redirect) | Entry point |
| wotan | 10.10.10.10 | 8080 (HTTP), 9090 (gRPC) | Message bus |
| trace-collector | 10.10.10.11 | 8081 (metrics) | eBPF → Wotan |
| timeguru | 10.10.10.20 | 8000 (HTTP) | Timeline API |
| captain | 10.10.10.21 | 8000 (HTTP) | Strategy API |
| micromanager | 10.10.10.22 | 8000 (HTTP) | Tasks API |
| architect | 10.10.10.23 | 8000 (HTTP) | Design API |
| dashboard-backend | 10.10.10.30 | 8082 (HTTP), 8083 (WS) | Metrics + WS |
| kanban-app | 10.10.10.200 | 8001 (HTTP) | Kanban UI |
| demo-app | 10.10.10.254 | 8000 (HTTP) | Customer sim |

## Technologies at a Glance

| Layer | Technology |
|-------|-----------|
| eBPF | Rust (Aya framework) |
| Services | Go 1.21+ |
| Containers | NixOS (declarative) |
| Orchestration | LXD |
| Message Bus | Wotan (gRPC + HTTP) |
| Gateway | nginx (HTTP/3) |
| Frontend | Vanilla JS |
| Observability | Prometheus, custom |
| Logging | zerolog (structured) |
| Config | YAML, TOML |

---

This diagram represents the **complete Unheaded alpha system** as designed.

All components traced, all services communicating, all proving: **Unheaded can host itself**. 🍾
