# Vision

*Status: Pipe Dream Riffing — Not Set in Stone*

Unheaded is a configuration management automation platform delivering a complete infrastructure "suit of armor" for web applications. You bring your application; it provides the control plane, observability, and supporting services.

## Core Capabilities

- eBPF-based observability with packet-level tracing from L2 to L7
- Immutable infrastructure via Kubernetes, Docker Swarm, or NixOS containers on LXD
- Dual-layered service mesh built on Wotan ring buffer and BGP
- Control plane with declarative config and drift detection
- Security baseline: FedRAMP, NIST, SOC2, PCI-DSS, HIPAA, ITAR, GDPR
- Zero application data access via architectural isolation
- GDPR/ePrivacy PII containment baked in from day one

## Network Architecture

| Option | Underlay | Overlay | Notes |
|--------|----------|---------|-------|
| Default | eBGP (RFC 7938) | EVPN-VXLAN | Clos fabric, route reflectors |
| Alternative | IS-IS | EVPN-VXLAN | Classic SP-style, link-state |

Both support: CLOS topology, BFD sub-second failover, ECMP, MP-BGP overlay, iBGP route reflectors, infinite horizontal scaling.

---

> **Source:** [docs/VISION.md](../docs/VISION.md)
