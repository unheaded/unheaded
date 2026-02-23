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
3. **Declarative Everything** — Immutable containers (LXD/containerd/NixOS/Docker), version-controlled configs
4. **Self-Hosting** — The Meta Moment proves it works
5. **Modern Stack** — HTTP/3, QUIC, gRPC, eBPF, Rust, Go

## Technology Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| eBPF programs | Rust (Aya) | Memory safety + performance for kernel |
| Services | Go 1.24+ | Simplicity, concurrency, tooling |
| Containers | LXD / containerd / NixOS / Docker | Interchangeable drop-in runtimes, same hardening baseline |
| Message Bus | Wotan (Go + gRPC) | Triple-role: ring buffer + event bus + protocol RAM |
| Gateway | nginx | Battle-tested, HTTP/3 support |
| Frontend | Vanilla JS | No framework overhead, full control |
| Orchestration | LXD (primary), containerd, Docker | Runtime-agnostic control plane |
| Config Management | Ansible / Terraform / Puppet / K8s / Chef / Salt | Interchangeable IaC output backends |
| Observability | Prometheus, Grafana, ELK, Fluentd, Jaeger, Nagios + more | Interchangeable backends; custom Wotan-native long-term |

## IaC Output Strategy

Unheaded generates configuration artifacts for the customer's preferred toolchain. The control plane maintains a single desired-state model; IaC backends are interchangeable output renderers. Adding a new backend is an output renderer — the control plane and eBPF layer don't change.

See [[IaC Backends|IaC-Backends]] for details on each supported backend.

## Observability Backend Strategy

Unheaded emits OpenTelemetry-compatible signals. Customers plug in their preferred observability stack via interchangeable adapters. Long-term: custom Wotan-native implementations leveraging the eBPF data plane for wire-speed observability with zero serialization overhead.

See [[Observability Backends|Observability-Backends]] for supported tools and phased roadmap.

---

> **Source:** [docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md)
