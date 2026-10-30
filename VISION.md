# Unheaded: Vision Document

> Every packet is a heartbeat. Every hop is a witness. The network IS the computer.

## The Problem We Solve

Modern infrastructure is broken. Applications are weighted down by sidecars, proxies, API gateways, observability agents, and security scanners—each adding latency, complexity, and surface area. Observability is bolted on after the fact. Security is scattered across network ACLs, service meshes, iptables rules, and application code. Infrastructure-as-Code is brittle: one change breaks three deployment targets.

The result: infrastructure teams spend months integrating tools that should be unified. DevOps becomes a career field. Security reviews take weeks. Observability data is sampled, delayed, and disconnected from the packets that generated it.

## The Vision

Unheaded delivers production-ready, fully observable, cryptographically secured infrastructure.

Applications bring their logic. Unheaded provides everything else: security, observability, networking, and a protocol layer that embeds computation directly into packets.

The outcome:
- **Unified infrastructure**: One platform, six IaC backends, eight observability backends, four container runtimes. Zero lock-in.
- **Observable by default**: Every packet carries its own trace. Full-fidelity event logging. No sampling, no sidecars, no tuning.
- **Secure by default**: eBPF-enforced policies at kernel speed. Post-quantum cryptography embedded in packet headers. Architectural isolation—applications never see infrastructure data.
- **Fast by default**: Observability with zero application overhead. Policy enforcement in microseconds. Sub-millisecond service discovery.

Infrastructure is armor. The application doesn't need to know it's there. But security teams can audit every packet. DevOps teams can swap backends without touching code.

## Who It Is For

Stevie, 2026-10-30: *"aimed at mom and pop shops and vibe coders to adhere to secure best practices so their application doesnt spread malware, etc or allow for RCE and jump box for threat actors."*

The adopter is not a security team. It is:
- **the small shop** running its own site;
- **the vibe coder** shipping an app an AI assistant wrote.

Neither will write authentication, input validation, egress control or audit logging correctly, and neither should have to.

Unheaded's job is to make sure their app cannot:
- serve malware to visitors;
- run an attacker's code (RCE);
- become a threat actor's jump box into everything else on the network.

That holds even when the app itself gets all of this wrong. The test for every capability, from the Captain's chair:
1. **Secure with zero configuration.** The safe setting is the default, never an option to find.
2. **No security code in the adopter's app.** Protection sits in front of and around the app (gate, firewall, egress allowlist, limits), not in a library they must wire in. ADR-114 applies this to login.
3. **Fails closed.** A misconfiguration blocks traffic; it never opens it.
4. **Plain language.** A shop owner can read what Unheaded did for them and why.
5. **Fits small hardware.** It runs on one modest machine, not a cluster.

The enterprise compliance work (ADR-097/098) still serves this adopter: a small shop that takes card payments faces PCI-DSS like anyone else. They get the evidence for free instead of paying an auditor to assemble it.

## The Technical Bet

eBPF is the inflection point. IPv6 extension headers are the medium. gRPC is the language. The protocol IS the moat.

**eBPF** changed everything. For the first time in networking history, we can attach arbitrary programs to packets without kernel modifications. Kernel-space observability. Kernel-space policy enforcement. No context switches. Sub-microsecond latency.

**IPv6 extension headers** are the standard mechanism for in-band signaling. No tunnels, no overlays, no "cloud-native" abstractions. Just the protocol. We embed a 20-byte register (Monad) in the Hop-by-Hop extension header. This register is the contract between application and armor. Every hop reads it, updates it, and passes it on.

**gRPC** is the canonical language for service-to-service communication. We built Monad-aware gRPC stubs that automatically encode/decode the register. Call `grpc.Dial()` with Unheaded transport; the register is populated automatically.

**The protocol is the moat.** Every other infrastructure platform is tied to cloud providers, container runtimes, orchestration frameworks. Unheaded is tied to one thing: IPv6. Ubiquitous, standardized, no vendor lock-in. Anyone running Linux 6.0+ with IPv6 can run Unheaded. Anyone who understands the protocol can implement it in their language, their runtime, their network.

## The Kingdom Metaphor

Infrastructure has layers, just like armor:

```
Layer 5: User Interface (Dashboard, Kanban) — The Face
Layer 4: Application Services (timeguru, captain, architect, micromanager) — The Mind
Layer 3: Infrastructure Services (wotan, trace-collector, gateway) — The Senses
Layer 2: Control Plane (unheaded-daemon) — The Will
Layer 1: Data Plane (eBPF programs) — The Reflexes
Layer 0: Infrastructure (Linux kernel, LXD/containerd/Docker) — The Body
```

The application doesn't wear the armor. The application IS inside the armor. The armor is transparent. The armor is auditable. The armor is swappable. The armor is the platform.

**The Cuirass** (unheaded-daemon) is the central intelligence. It watches for drift (desired vs. actual state), loads eBPF programs, allocates resources, and reconciles policy.

**The Cape & Cloak** (dashboard-backend) are the senses. Every event, every packet, every state transition is recorded to the event stream and forwarded to observability backends.

**The Gauntlets** (protocol-api) are the articulation points. Wire format conversion, dictionary lookups, protocol validation.

**The Greaves** (service discovery) are the foundation. Every service knows where every other service lives. Failover is automatic.

Together, they form an invisible layer of infrastructure that applications never need to think about. But operators can see everything. Security teams can enforce policies. Developers can focus on logic.

## The Long Game

**Age 1 — Alpha: Protocol + Services in Mock Mode** (Feb 2026)

All 25 services operational. Monad, Sophia, Wotan fully specified. gRPC-first transport working. Message bus delivering events. Dashboard visualizing traces. Kanban self-hosting. eBPF programs written but not yet tested on bare metal.

Success criteria: Prove the architecture works. Every service talks to Wotan. Every trace flows end-to-end. Dashboard is responsive. Zero architectural flaws discovered.

**Age 2 — Beta: Bare Metal eBPF Live** (Apr 2026)

eBPF programs running on Linux 6.0+ kernel. Packet-level observability working. XDP programs at line rate. TC programs on egress. Full-fidelity event logging to Anamnesis. EVPN-VXLAN overlay (Kingdom Mode) working on two-host testbed.

Success criteria: Barebone deployment guide published. Trace propagation end-to-end. Sub-100µs latency to observability backend. Kernel verifier passing on all programs. Public demo on YouTube.

**Age 3 — Hardening: Production-Ready** (Jul 2026)

Security audit complete. Integration with major cloud provider consoles. Helm charts passing Artifact Hub validation. Zero security issues in third-party audits. Community deployments in the wild.

Success criteria: Deployment guide battle-tested. Security audit clean. Conference talks at KubeCon and eBPF Summit. Others are using it.

**Age 4 — Ecosystem: IANA-Registered Protocol, Standards-Track RFC** (Oct 2026)

IANA registration of protocol type (0x2A for UNHEADED_METRIC_V1). Standards-track RFC published. Interoperable implementations in Go, Rust, Python, Node.js. Open-source SBOM. Open-source security audit.

Success criteria: Anyone can implement the protocol independently. Protocol is part of the IPv6 ecosystem. Unheaded is synonymous with "observable infrastructure."

## What We Ship That Nobody Else Ships

**Metrics embedded IN packets, not alongside them.** Datadog, New Relic, Honeycomb—they all sidecar observability agents. Unheaded embeds metrics directly into packet headers. Zero additional wire bytes. Zero application overhead. Full-fidelity tracing as a side effect of packet processing.

**DOOM on the UPC as a compute proof.** We implemented a Towers of Hanoi solver in LISP, compiled it to BPF, and ran it inside the kernel. Not as a research project—as a real service on port 16680. Proves that Unheaded's BPF layer is a general-purpose compute substrate. Network-level machine learning, anomaly detection, cryptographic operations—all in kernel space.

**The Lich as a continuous adversary.** Every infrastructure has security bugs. The Lich is a simulated attacker that runs inside Unheaded, probing for vulnerabilities, testing policy enforcement, validating isolation. Red team automated. Every deploy, the Lich attacks. Every deploy, we verify the armor holds.

**Kingdom Mode.** A single /48 block of IPv6 addresses can be interpreted two ways: as 2^80 hosts OR as 2^160 distinct flows with extended register space. Inverse Mask reclaims deterministic address bits as computational state. Deterministic yet flexible. No special hardware. Just IPv6 arithmetic.

## Open Source

Unheaded is GPL-3.0 licensed. Take it. Use it. Build on it. Break it. The ideas matter more than the moat.

---

**Contact:** stevie@bellis.tech
**Source:** github.com/stevenrbellis/unheaded
**Specification:** docs/protocol/draft-bellis-unheaded-foundation-04.md
