# Unheaded

**Sloppy container orchestration platform wrote with claude AI**

Unheaded is a configuration management automation platform that delivers the complete "suit of armor" for modern SaaS applications. You bring your application ("the head"), we provide everything else.

## What is Unheaded?

A drop-in infrastructure platform providing:

- **eBPF-based observability** - Packet-level tracing from L2-L7
- **Immutable infrastructure** - NixOS containers on LXD
- **Service mesh** - Built on [Busboy](https://github.com/unheaded/busboy) message bus
- **Control plane** - Declarative config with drift detection
- **Security baseline** - FEDRAMP, NIST, SOC2, PCI-DSS, HIPAA, ITAR, GDPR
- **Zero customer data access** - Architectural isolation at every layer

## The Alpha

This alpha demonstrates the core platform capabilities:

1. **eBPF packet tracing** - Every packet tagged with trace_id at XDP layer
2. **Microservices architecture** - Services mirror our AI-augmented dev workflow
3. **Real-time dashboard** - Live packet flow visualization
4. **The Meta Moment** - Kanban app showing Unheaded building itself

## Architecture

```
BARE METAL HOST
├── unheaded-daemon (control plane)
├── eBPF programs (packet tracing)
└── LXD containers (NixOS)
    ├── busboy (message bus)
    ├── trace-collector (eBPF → Busboy)
    ├── timeguru (timeline tracking)
    ├── captain (strategy)
    ├── micromanager (execution)
    ├── architect (design)
    ├── dashboard-backend (metrics)
    ├── kanban-app (the meta moment)
    └── gateway (HTTP/3, gRPC, WebSocket)
```

## Quick Start

### Prerequisites

- Linux host (bare metal or VM: AWS, Azure, GCP, Oracle, VMware, QEMU)
- Root or sudo access
- Internet connection

### Deploy

```bash
# One-command deployment
curl -sSL https://raw.githubusercontent.com/unheaded/unheaded/main/scripts/deploy-alpha.sh | sudo bash
```

This will:
1. Install LXD and dependencies
2. Configure networking
3. Load eBPF programs
4. Launch all containers
5. Start the dashboard

Access: `https://localhost` (or your host IP)

### Manual Deployment

```bash
# Clone repo
git clone https://github.com/unheaded/unheaded.git
cd unheaded

# Setup host
sudo ./scripts/setup-host.sh

# Build services
make build

# Deploy containers
sudo ./scripts/deploy-alpha.sh

# Load eBPF
sudo ./scripts/load-ebpf.sh
```

## Project Structure

```
unheaded/
├── cmd/                      # Service binaries
│   ├── unheaded-daemon/     # Control plane agent
│   ├── trace-collector/     # eBPF → Busboy bridge (Rust)
│   ├── dashboard-backend/   # Metrics aggregator (Go)
│   └── kanban-app/          # The meta moment (Go + JS)
│
├── services/                 # Microservice integration
│   ├── busboy/              # Message bus (github.com/unheaded/busboy)
│   ├── timeguru/            # Timeline tracking (Go)
│   ├── captain/             # Strategy service (Go)
│   ├── micromanager/        # Execution service (Go)
│   └── architect/           # Design service (Go)
│
├── ebpf/                     # eBPF programs (Rust)
│   ├── packet_marker.rs     # Trace ID injection
│   ├── flow_tracker.rs      # Connection tracking
│   └── latency_probe.rs     # RTT measurement
│
├── nix/                      # NixOS container definitions
│   ├── flake.nix
│   ├── containers/          # Per-service configs
│   └── modules/             # Shared modules
│
├── dashboard/                # Main dashboard UI
│   ├── index.html
│   ├── css/
│   └── js/
│
├── kanban/                   # Kanban app (the meta moment)
│   ├── index.html
│   ├── css/
│   └── js/
│
├── pkg/                      # Shared Go packages
│   ├── lxd/                 # LXD client
│   ├── state/               # State management
│   ├── telemetry/           # Common telemetry
│   └── busboy-client/       # Busboy Go client
│
├── docs/                     # Documentation
│   ├── ARCHITECTURE.md
│   ├── MICROSERVICES.md
│   ├── SECURITY.md
│   └── THE_META_MOMENT.md
│
├── scripts/                  # Deployment automation
│   ├── setup-host.sh        # Host preparation
│   ├── deploy-alpha.sh      # Full deployment
│   ├── load-ebpf.sh         # eBPF loading
│   └── demo-kanban.sh       # Demo script
│
└── references/               # Source-of-truth files
    ├── timeline.md          # Roadmap (human-readable)
    ├── timeline.json        # API format
    └── timeline.yaml        # IaC format
```

## Technology Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| eBPF Programs | Rust | Packet tracing, low-latency |
| Services | Go | APIs, control plane, business logic |
| Containers | NixOS | Immutable, declarative |
| Message Bus | Busboy (Go + gRPC) | Service communication |
| Frontend | Vanilla JS | Dashboard, Kanban app |
| Gateway | Nginx | HTTP/3, QUIC, WebSocket |
| Observability | Prometheus + custom | Metrics, tracing |

## The Microservices

All services communicate via **Busboy** (the message bus):

### Core Services
- **busboy** - Message routing, pub/sub, gRPC streaming
- **trace-collector** - Reads eBPF, publishes to Busboy
- **dashboard-backend** - Aggregates metrics, serves WebSocket
- **gateway** - HTTP/3, gRPC-Web, WebSocket proxy

### Agent Services (Mirror Dev Workflow)
- **timeguru** - Timeline tracking, milestone management
- **captain** - Strategy decisions, vision alignment
- **micromanager** - Task breakdown, QA oversight
- **architect** - Infrastructure design, tech decisions

### The Meta Moment
- **kanban-app** - Reads timeline.md, displays Kanban board
- Demonstrates: "Unheaded hosting Unheaded's own development"
- Ultimate proof: If it can manage itself, it can manage anything

## Data Format Strategy

Each service maintains **triple format**:

```
references/timeline.md     → Source of truth (human, Git-friendly)
references/timeline.json   → REST API responses (machine)
references/timeline.yaml   → IaC config (tooling)
```

Auto-sync on write: MD → JSON/YAML

## Security

Security-first design from day one:

- eBPF programs verified by kernel
- NixOS immutable containers
- Zero customer data access (architectural isolation)
- mTLS between services
- Seccomp, capabilities restrictions
- Network policies enforced
- Headers-to-kernel hardening

See [SECURITY.md](docs/SECURITY.md) for details.

## Development

```bash
# Build all services
make build

# Run tests
make test

# Build eBPF programs
make ebpf

# Build containers
make containers

# Run locally (docker-compose)
make dev
```

## Roadmap

See [references/timeline.md](references/timeline.md) for the living roadmap.

**Current Phase:** Alpha - Proving the core concepts

**Next:**
1. Full test coverage
2. Production hardening
3. Multi-node clustering
4. Compliance templates

## Contributing

We're in alpha. Not accepting external contributions yet, but feel free to:
- Open issues for bugs
- Suggest features
- Star the repo if you're interested

## License

Proprietary (for now). Open source plans TBD.

## Contact

- **Web**: [unheaded.com](https://unheaded.com)
- **Email**: hello@unheaded.com
- **GitHub**: [github.com/unheaded](https://github.com/unheaded)

---

**"We drink our own champagne."** 🍾

Built by Unheaded, running on Unheaded.
