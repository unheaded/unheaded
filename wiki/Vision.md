# Vision

*Status: Early-stage — architecture subject to change*

Unheaded is a configuration management automation platform delivering production-ready infrastructure for web applications. You bring the application; Unheaded provides the control plane, observability, service mesh, and security baseline.

## Who It Is For

Small shops and vibe coders first: people shipping an app (often AI-written) who won't build security themselves. Security and infra teams remain users. Out of the box every compliance control is on and hardens the app's *hosting*; it cannot fix vulnerabilities in the app's own code. Unheaded is an aid, not a promise: it applies secure best practice around their app by default, so it is much harder for that app to serve malware, run an attacker's code, or become a jump box. Tests: secure with zero configuration, no security code in their app, fails closed (a badly configured app does not run), plain language (including what it does not cover), small hardware. See [VISION.md](../VISION.md#who-it-is-for).

## Core Capabilities

- eBPF-based observability with packet-level tracing from L2 to L7
- Immutable infrastructure via interchangeable container runtimes (LXD, containerd, NixOS, Docker)
- Interchangeable IaC backends (Ansible, Terraform, Puppet, Kubernetes, Chef, Salt) — your tools, our state model
- Interchangeable observability backends (Prometheus, Grafana, ELK, Fluentd, Jaeger, Nagios + more) — your dashboards, our data
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
