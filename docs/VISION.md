# Unheaded - Vision: The Future Reality

**Status: Pipe Dream Riffing - Not Set in Stone**
**Date: February 10, 2026**
**Author: Muck (The Matriarch/Patriarch)**

---

## What It Is

Unheaded is a configuration management automation platform that delivers a
complete infrastructure "suit of armor" for web applications. You bring
your application; it provides the control plane, observability, and supporting
services.

## Who It Is For

Currently aimed at users who want to build, run, and test their platform locally
in a mock infrastructure environment.

## What It Does

- eBPF-based observability with packet-level tracing from L2 to L7.
- Immutable infrastructure using either Kubernetes, Docker Swarm, or NixOS containers on LXD.
- Dual-layered service mesh built on the Busboy ephemeral ring buffer and BGP.
- Control plane with declarative config and drift detection.
- Security baseline aligned to FEDRAMP, NIST, SOC2, PCI-DSS, HIPAA, ITAR, and GDPR.
- Zero application data access via architectural isolation.
- GDPR/ePrivacy PII containment: *"When GDPR and ePrivacy are in effect, PII is radioactive. You need containment, handling and disposal procedures, you need to allow users to inspect it at any time and if you accidentally expose anyone to it that's a major emergency incident."* — [hnbad](https://news.ycombinator.com/user?id=hnbad). Configurable per-deployment (`pii_mode: eu`) with containment, right-of-access, right-of-erasure, and breach alerting baked in from day one.
- Real-time dashboard and the Kanban "Meta Moment" app running on the platform.

## How It Works

- Host/VM runs the unheaded-daemon control plane plus eBPF programs for packet tracing.
- Docker/LXD/K8s/NixOS hosts containers for Busboy, trace-collector, timeguru, captain, micromanager, architect, dashboard-backend, kanban-app, and gateway.
- Trace-collector bridges eBPF events into Busboy; services communicate over the bus and UIs are served by the dashboard and Kanban apps... more to come.
- Logs pipe to integrated SIEM.
- Ring buffer and BGP perform application and container health checks, alarm/playbook state changes depending on percentage of apps reporting.
- Minimum of 2 unheaded suits running in parallel utilizing CLOS, BFD, ECMP, EVPN/MP-BGP, eBGP, iBGP with route reflectors for VXLAN creating RFC 7938 full mesh capable of scaling infinitely.

### Network Underlay Options

> **NOTE**: For fun (and flexibility), we support IS-IS underlay as an alternative to
> the default eBGP underlay. Talk to the Architect about it.

| Option | Underlay | Overlay | Notes |
|--------|----------|---------|-------|
| Default | eBGP (RFC 7938) | EVPN-VXLAN | Clos fabric, route reflectors for scale |
| Alternative | IS-IS | EVPN-VXLAN | Classic SP-style, link-state underlay |

Both options support: CLOS topology, BFD sub-second failover, ECMP, MP-BGP for overlay, iBGP route reflectors, and infinite horizontal scaling.

## How to Run (Minimal)

- Install prerequisites: Go 1.24+, Docker with Compose, Git, and curl.
- Build: run `go build ./...` (or `make build`).
- Start core services: run `docker compose up -d`.
- Verify health: hit `/health` on all service ports.
- Load eBPF (Linux only): `sudo cargo run` from `cmd/ebpf-loader/`.

> *This section needs editing as the platform matures.*

---

*"You bring the head. We provide the armor. The Knight stands complete."*
