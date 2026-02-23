# Welcome to the Unheaded Kingdom

**"Production-ready infrastructure in hours, not months."**

---

## The Origin Story

On January 20, 2026, the first commit landed. One engineer. One AI. A vision: deliver complete infrastructure for modern SaaS applications so customers could focus on their product -- "the head" -- while Unheaded provided everything else.

Over the next 33 days, what began as a configuration management platform evolved into something unprecedented. The team built a 6-layer architecture spanning eBPF kernel programs, a Go microservice mesh, a custom message bus (Wotan), declarative NixOS containers, and a vanilla JS dashboard -- all orchestrated by an AI-driven development workflow with 15 specialized skill personas.

Then came the proof of computational completeness: **Doom running inside eBPF**.

The Monad protocol -- a 20-byte register file carried in IPv6 Hop-by-Hop extension headers -- was designed for packet-level observability. To prove it could carry arbitrary computation at wire speed, the team cross-compiled Doom (the 1993 id Software classic) from C to RISC-V, translated RISC-V to a custom MBC bytecode ISA, loaded the bytecode into BPF maps, and executed it instruction-by-instruction as packets bounced through an XDP circulation ring across 6 Linux network namespaces.

**The result:**

| Metric | Value |
|--------|-------|
| Lines of code | 465,000+ |
| End-to-end tests | 23 passing |
| Doom frames rendered | 559+ |
| Instructions executed | 819,000,000+ |
| ROM faults | 0 |
| CPU halts | 0 |
| Time from first commit to Doom | 33 days |

If a game engine can run in the data plane, packet tracing can too. That is the proof.

---

## Wiki Pages

### Core Documentation

- **[Architecture](architecture.md)** -- The 6-layer system architecture, Monad wire format, and eBPF execution model
- **[Protocol Specifications](protocol-specs.md)** -- Monad protocol, Wotan message bus, and Sophia knowledge graph
- **[Security](security.md)** -- Security posture, Lich campaign results, and zero customer data access

### Doom-over-IPv6

- **[Doom over IPv6](doom-over-ipv6.md)** -- Full technical narrative of running Doom inside eBPF
- **[Bug Kill Chain](bug-kill-chain.md)** -- Every major bug found and fixed during Doom development
- **[Performance](performance.md)** -- Performance analysis, injection rates, and Netflix PPS comparison

### Project

- **[Roadmap](roadmap.md)** -- Current state, sprint progress, and future plans

---

## Key Numbers

- **8 microservices** (timeguru, captain, architect, micromanager, monad, sophia, dashboard-backend, kanban-app) all communicating via Wotan
- **6 eBPF programs** (packet_marker, flow_tracker, latency_probe, monad_cpu, screen writer, stats collector)
- **293 Rust tests + 135 Go test packages** -- all passing, zero failures
- **NixOS container definitions** for every service with seccomp, capability restrictions, and read-only filesystems
- **Interchangeable backends** -- IaC (Ansible, Terraform, Puppet, Kubernetes, Chef, Salt), observability (Prometheus, Grafana, ELK, Jaeger, Nagios), and container runtimes (LXD, containerd, NixOS, Docker)

---

## Quick Links

| Resource | Location |
|----------|----------|
| Source code | [github.com/unheaded/unheaded](https://github.com/unheaded/unheaded) |
| Dashboard | `/dashboard` (live packet flow visualization) |
| Kanban board | `/kanban` (the Meta Moment -- Unheaded building Unheaded) |
| Doom viewer | `/doom/` (live framebuffer from eBPF) |
| API health | `/api/v1/health` |

---

*Last updated: February 23, 2026 -- Sprint S33*
