# Welcome to the Unheaded Kingdom

**Production-ready infrastructure in hours, not months.**

User brings their app ("the head"), we provide everything else ("unheaded").

## Running Services

| Service | Port | Role |
|---------|------|------|
| Wotan | 18000 | Message bus (Fae Chamber) |
| Timeguru | 19000 | Timeline tracking (Oracle's Antre) |
| Architect | 19001 | ADR service (Sage's Lair) |
| Captain | 19002 | Strategy service (Commander's Quarters) |
| Micromanager | 19003 | Task execution (War Room) |
| Dashboard | 16667 | System overview |
| Kanban | 16668 | Meta moment (self-hosting proof) |
| Wiki | 20002 | You are here |
| Grafana | 3001 | Metrics dashboards |
| VictoriaMetrics | 8428 | Time-series storage |
| Traefik | 80/443 | Gateway |

## Architecture

```
Layer 5: User Interface (Dashboard, Kanban, Wiki)
Layer 4: Application Services (timeguru, captain, micromanager, architect)
Layer 3: Infrastructure Services (wotan, trace-collector, gateway)
Layer 2: Control Plane (unheaded-daemon)
Layer 1: Data Plane (eBPF programs)
Layer 0: Infrastructure (LXD, Docker, NixOS)
```

## Core Principles

- **eBPF-based observability** from packet zero
- **Zero user data access** by architectural design
- **Interchangeable backends** for IaC, observability, and containers
- **Declarative everything** in version control
- **Self-hosting proof** (The Meta Moment)

## Quick Links

- [Architecture](ARCHITECTURE)
- [Phylactery](PHYLACTERY)
- [Gemini](GEMINI)
