# Architecture Overview

## 6-Layer Architecture

```
Layer 5: User Interface (Dashboard, Kanban)
Layer 4: Application Services (timeguru, captain, micromanager, architect)
Layer 3: Infrastructure Services (wotan, trace-collector, gateway)
Layer 2: Control Plane (unheaded-daemon)
Layer 1: Data Plane (eBPF programs)
Layer 0: Infrastructure (LXD, host OS)
```

## Design Principles

1. **Security First** — eBPF traceability, immutable infra, zero customer data access
2. **Observable by Default** — Packet-level visibility L2–L7
3. **Declarative Everything** — NixOS containers, version-controlled configs
4. **Self-Hosting** — The Meta Moment proves it works
5. **Modern Stack** — HTTP/3, QUIC, gRPC, eBPF, Rust, Go

## Technology Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| eBPF programs | Rust (Aya) | Memory safety + performance for kernel |
| Services | Go 1.24+ | Simplicity, concurrency, tooling |
| Containers | NixOS | Declarative, immutable, reproducible |
| Message Bus | Wotan (Go + gRPC) | Triple-role: ring buffer + event bus + protocol RAM |
| Gateway | nginx | Battle-tested, HTTP/3 support |
| Frontend | Vanilla JS | No framework overhead, full control |
| Orchestration | LXD | Lightweight system containers |

---

> **Source:** [docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md)
