# ADR-69420: Kingdom-Native BGP Routing Daemon + Unheaded OS

**Date**: 2026-03-18
**Author**: Stevie Bellis, Kingdom Council
**Status**: ACCEPTED
**Scope**: Long-term vision (Age 2/3 features, not blocking public launch)

## Context

The Kingdom currently operates across 2 bare-metal hosts with manual routing and external dependency on Linux networking stacks. To scale from 2 to 100s-1000s of servers while supporting modern hardware (Mellanox/ConnectX NICs with XDP offload, GPU workloads for AI/ML), two complementary capabilities are essential:

**Feature A: Kingdom-Native BGP Routing Daemon** — A minimal internal BGP speaker that:
- Peers exclusively with other Kingdom nodes (no internet BGP table, no external peering)
- Computes best paths with ECMP multipath for load distribution
- Writes forwarding state directly into pinned BPF maps consumed by XDP programs
- Provides service-aware path selection based on Monad register state (flow action, circuit state, QoS class)
- Achieves sub-second failover via BFD
- Built as a split control plane (Go) + data plane (Rust/Aya eBPF)

Scope: Internal fabric only, 50-500 prefixes. Not RFC 4271 full BGP—no 400K+ tables, no route dampening, no communities, confederations, or external peering.

**Feature B: Unheaded OS** — A hardened Debian image builder pipeline that:
- Starts with debian-12-minimal as base
- Applies Kingdom package set (Go/Rust binaries, configs, systemd units)
- Applies CIS/STIG hardening (sysctl, filesystem, services, kernel parameters)
- Installs and configures SELinux policy (ported from RHEL reference policy to Debian)
- Produces bare-metal ISO, cloud images (AMI/GCE/Azure), and VM images (qcow2)
- Implements Jenkins pipeline: source → .deb packages → apt repository → image builder → artifacts
- Maintains soft fork alignment: tracks upstream Debian stable with overlay patches

The SELinux-on-Debian angle is the key differentiator. Compliance frameworks (FedRAMP, STIG, PCI-DSS) assume RHEL-style MAC enforcement; production-quality SELinux policy for Debian bridges this gap and benefits the broader ecosystem.

## Warmonger (Driving) Perspective

Build Kingdom BGP in two phases: **Phase 1 (Age 2a, 8-10 weeks)** delivers core BGP FSM, UPDATE parsing, and ECMP path selection in Go, with a stub XDP data plane; Phase 2 (Age 2b, 6-8 weeks) hardens the BPF map integration and adds BFD sub-second detection. Unheaded OS follows in parallel: Phase 1 (Age 2a, 6-8 weeks) builds the Debian hardening pipeline and .deb package automation; Phase 2 (Age 3a, 4-6 weeks) adds SELinux policy and cloud image targets. Critical path: complete Kingdom BGP control plane before any data plane work; defer cloud image variants until Debian pipeline is stable. Total estimate: 24-32 weeks across both features. Dependency: alpha stabilization and API freeze are prerequisites.

## Architect Perspective

Kingdom BGP sits in the data plane layer between Monad (circuit state source of truth) and the XDP forwarding engine. The BGP daemon reads from Monad's register state and publishes updates to a pair of pinned BPF maps: one for route table (dest prefix → [next hop, weight, QoS]), another for tunnel endpoint state. XDP programs load these maps at packet processing time, zero-copy. Network topology: Kingdom nodes form a full mesh for BGP peering (or folded tree for large scale); each node announces its service prefixes (Pod CIDR, overlay endpoints). Convergence is fast because UPDATE parsing is deterministic and the fabric is flat—no MED, AS_PATH length concerns simplify computation. Integration with existing EVPN-VXLAN: Kingdom BGP does not replace fabric underlay (that's still physical L2+L3 to the data center); it operates as an overlay routing protocol, computing paths across service endpoints. Unheaded OS: replaces ad-hoc container images and the NixOS experiment. The .deb-based pipeline is more maintainable, integrates with standard Linux hardening tooling, and produces certified artifacts that enterprises expect (ISO boot, signed packages, reproducible builds). SELinux policy becomes a first-class Kingdom component, like Gnostic or Armory.

## Developer Perspective

Go control plane uses `google/gopacket` for BGP UPDATE parsing (proven, no gobgp runtime dependency bloat). Route computation is a clean graph algorithm (Dijkstra + ECMP tie-breaking); testable in isolation without a running BGP daemon. BPF map schema: `struct route_entry { __u32 dest; __u16 prefix_len; __u32 next_hops[16]; __u8 num_hops; __u16 qos_class; }` and a per-flow state map for stateful FIB lookups. Use Aya for map boilerplate and verifier safety. TDD from day one: unit tests for BGP FSM transitions (open → established paths, WITHDRAW handling), fuzzing for UPDATE parsing (corrupted length fields, invalid ASN encoding), and integration tests on two VMs with virtual fabric simulation. Hardest part: BPF verifier constraints on map iteration and getting deterministic convergence proofs right. For Unheaded OS, use Debian's `live-build` + `packer` for image generation; integrate into Jenkins via a Groovy pipeline that triggers on .deb package updates. The SELinux policy port is the biggest lift—RHEL's policy is tuned for systemd unit placement and /usr/lib structure that may differ in Debian.

## Scientist Perspective

Convergence proof for Kingdom BGP: with N nodes, full mesh peering, and a single failure domain (one link down), the worst-case UPDATE propagation is O(N) rounds of BGP UPDATE messages. Since each round is deterministic (no route reflectors, no backdoor path preferences), convergence time is bounded by network latency × N and the BGP minimum route advertisement interval (MRAI, typically 30s). For internal fabric with <256 nodes, convergence under 5 minutes is provable. ECMP throughput: if a 100G NIC has K equal-cost paths, XDP forwarding can achieve K × bandwidth without per-flow rebalancing (stateless hashing on 5-tuple), assuming no hash collision overhead. Theoretical upper bound: 100G × K ≈ 100K pps per path in a worst-case scenario. FSM confusion attacks (malformed STATE transitions, unexpected message timing) are mitigated by the simplified profile: we reject messages outside [OPENCONFIRM, ESTABLISHED], making the state machine a two-state automaton for steady-state operation. BGP's known attack classes (false NEXT_HOP injection, prefix hijacking via AS_PATH manipulation, route flapping) are orthogonal to the internal-only scope—we don't receive external routes, so hijacking Kingdom service prefixes would require a peer to be compromised, moving the attack surface upstream to peer authentication.

## Captain Perspective

Market differentiation: "We own the entire stack—network fabric, routing, kernel hardening, and SELinux enforcement—so customers get zero-trust networking with provable security and performance." BGP daemon is table-stakes for scaling beyond 10 nodes; companies building Kingdom clusters at scale (100+ servers) will demand it. Unheaded OS is premium: enterprises expect a certified, hardened base image that passes FedRAMP/SOC2 audits. Bundling SELinux policy into the image is a competitive moat—no one else is shipping hardened Debian with mandatory access control for Kubernetes/Kingdom workloads. Pricing: Kingdom BGP is a standard tier feature (no upsell). Unheaded OS is sold as a "Kingdom Enterprise" SKU with audit trail generation and SLA-backed image updates. GTM for Unheaded OS: partner with procurement teams early (Q2 2026) showing FedRAMP alignment; release AMI to AWS Marketplace; target regulatory-heavy verticals (financial services, healthcare, defense). First customer: internal Kingdom infrastructure (dogfood), second: beta customer in finance needing SOC2 attestation.

## Micromanager Perspective

QA requirements: unit test coverage >90% for BGP FSM and route computation; fuzzing suite with 100K+ random UPDATE messages; integration tests on 4-node test fabric (2 physical servers + 2 VMs) validating convergence, failover, and QoS rerouting. Test acceptance criteria: BGP daemon converges within 60 seconds of topology change; BFD detects link failure in <1 second; rerouting completes before TCP retransmit timeout; zero packet loss during graceful shutdown. For Unheaded OS: packer builds produce byte-identical ISOs on repeated runs (reproducibility); SELinux policy validates with `semodule -e` without errors; CIS/STIG scan via `lynis` passes all "SUGGEST" items; cloud image boots on AWS/GCE/Azure within 60 seconds; signed .deb packages validate with GPG key on installation. Definition of done: Kingdom BGP = stable upstream peering + XDP forwarding live + production logs + runbook documentation. Unheaded OS = packer pipeline automated, image artifacts in S3, Jenkins job automated, signed .deb repository live, audit trail logs included in images.

## Lore Perspective

Kingdom BGP daemon: **Sleipnir** — Odin's eight-legged horse from the Prose Edda. Eight legs map to eight ECMP paths. The fastest mount in Norse mythology — the routing daemon that makes packets move fastest through the fabric. Also the first seal in the Ragnarok Online God Items quest, the foundation that unlocks everything else. Unheaded OS: **Yggdrasil** — the World Tree that connects all nine realms in Norse cosmology. The substrate that holds the entire cosmos together, exactly as the OS holds the entire Kingdom together. Also a nod to Ragnarok Online, where Yggdrasil items fully restore a player to 100% HP and MP — a fresh install, fully provisioned, everything healthy. Both names anchor the Kingdom in consistent Norse mythological language while carrying dual resonance with the gaming heritage that inspired the project's culture.

## Kingdom Hierarchy Perspective

Sleipnir (BGP daemon) fits as a new **Gnostic service**—it participates in the Monad-driven circuit state and publishes to shared BPF maps like other fabric services. It lives alongside Armory (dataplane) but operates at the control plane layer. Yggdrasil (Unheaded OS) is a **new organizational tier** above the current image/package layer—it's the Kingdom substrate tier responsible for OS release, hardening policy, and base image production. It owns the Jenkins pipeline and package repository, feeding artifacts to every other Kingdom layer. Both are part of the public Kingdom hierarchy starting Age 2, but only documented internally until public launch; after launch, they're leading marketing differentiators.

## Moat Ghost (Compliance) Perspective

SELinux-on-Debian directly maps to FedRAMP requirements: AC-3 (Access Control), SI-7 (Software/Information Integrity), and AU-12 (Audit Generation). STIG mapping: RHEL STIG rules (e.g., RHEL-07-010010 for SELinux enforcement mode) translate to Debian contexts; a Debian-specific STIG adaptation is in draft by DISA but not yet published, so our policy positions Kingdom ahead of the curve. PCI-DSS requirement 2.2 (hardened configs) and requirement 10 (logging) are satisfied by CIS hardening + auditd integration + SELinux denial logs. Audit evidence: image builder produces a signed manifest listing all applied policies, hardening rules, and package versions; every boot logs policy loads; policy compliance dashboard (daily policy diffs, denial trend analysis) supports continuous audit. Compliance posture: pre-audit, Kingdom environments now have evidence for "our base OS is hardened to [FedRAMP/STIG/PCI] baseline" without expensive consulting. Risk reduction: eliminates the "we have a custom hardened image but no proof" liability that haunts many scale-ups.

## BlackMage (Security) Perspective

BGP daemon attack surface: UPDATE message parsing (malformed length, invalid ASN, corrupted NLRI), FSM confusion (unexpected state transitions, timer abuse), and peer spoofing. Mitigation: strict UPDATE parsing with length bounds checking, a two-state steady-state FSM (reject out-of-state messages), and mandatory authentication via per-peer pre-shared keys (PSK) in the gRPC API. Fuzzing must cover: random 32-bit ASN values, crafted NLRI with prefix length >32 bits, zero-length path attributes, withdrawn routes without corresponding announces. Known BGP attack classes that apply: **UPDATE manipulation** (we block via PSK), **prefix hijacking via AS_PATH** (not applicable—no AS_PATH, only neighbor list), **route flapping** to trigger re-convergence storms (mitigated by deterministic computation; add dampening if tests show instability), **FSM confusion** (two-state automaton eliminates this), **memory exhaustion** (bounded BPF map sizes prevent memory bombs). The internal-only scope significantly reduces risk: peers are authenticated Kingdom nodes, not internet routers; ECMP failures can't leak traffic because paths are precomputed and map-based. Residual risk: BPF verifier bypass (mitigated by Aya's type safety), map poisoning by a compromised peer (mitigated by PSK + per-peer rate limits). Red team exercise: corrupt BPF map entries from a rogue peer, verify forwarding either fails safely or triggers alerts.

## Barrister (Licensing) Perspective

Kingdom BGP using `google/gopacket` (BSD-3-Clause) is compatible with GPL-3.0 (more permissive license can be used in GPL projects). If we later consider **gobgp** (Apache-2.0) for advanced BGP features, Apache-2.0 is also compatible with GPL-3.0 by linking. No patent concerns on BGP itself (RFC 4271 is patent-free as of 2025; Cisco and Juniper patents expired or are licensed under general FRAND terms). SELinux policy derived from RHEL reference policy (GPL-2.0): policy source files are GPL-2.0, compiled policies (.pp files) are policy objects exempt from GPL linking requirements (they don't execute as code, they configure kernel enforcement). We can distribute compiled SELinux policies under GPL-2.0 or dual-license as GPL-2.0 + MIT (permissive alternative). Recommendation: license Yggdrasil's SELinux policy as GPL-2.0 (consistent with RHEL source) and document the derivation in COPYING.SELinux. All Kingdom GPL-3.0 binaries can load these policies without license conflict. No trademark issues (Yggdrasil, Sleipnir are generic Norse mythology terms, not trademarked).

## RFC Editor Perspective

Kingdom BGP does not need a full internal specification yet, but a **Kingdom BGP Profile Document** (KBP-001) should be drafted by end of Age 2a, defining: (1) the simplified BGP subset we implement (OPEN, NOTIFICATION, UPDATE, KEEPALIVE—no ROUTE_REFRESH), (2) the BPF map schema and update semantics, (3) gRPC API for route injection/withdrawal, and (4) convergence guarantees under failure conditions. This is not an IETF draft (too narrow), but an internal design spec that guides vendors/partners integrating with Kingdom. The document serves two purposes: clarity for implementers and audit evidence that the BGP subset is intentionally scoped. Reference: structured like RFC 5880 (BFD) in terseness and completeness, not RFC 4271's exhaustive state machine.

## Timeguru (Roadmap) Perspective

Kingdom BGP is an **Age 2** feature, beginning immediately after alpha stabilization (Q2 2026). Phase 1 (8-10 weeks) delivers the control plane and stub data plane; Phase 2 (6-8 weeks) hardens BPF integration and adds BFD. Unheaded OS starts in parallel in Age 2a (Q2 2026) with the Debian hardening pipeline and .deb package automation (6-8 weeks). SELinux policy work begins in Age 3a (Q4 2026), as it's the highest-complexity item; by end of Age 3a, all three components (BGP control+data, hardened pipeline, SELinux) converge into a unified product story. Prerequisites: Alpha must be stable (no major API churn), Monad register interface must be frozen, and the BPF map pinning convention must be documented. Go modules and Aya ecosystem must be production-ready (they are as of 2026). First public availability: end of Age 2 (Q4 2026) for Sleipnir (BGP daemon); end of Age 3a (Q4 2026) for Yggdrasil (Unheaded OS) in MVP form (ISO + bare-metal image). Enterprise roadmap ties to FedRAMP audit timelines; plan for FedRAMP In Process (P-ATO) by mid-2027.

## Busboy (Coordination) Perspective

Dependency graph: **Sleipnir** (BGP daemon) depends on Monad being stable (reads circuit state) and Armory being ready (consumes BPF maps for forwarding). Sleipnir is independent of Yggdrasil. **Yggdrasil** (Unheaded OS) depends on the Kingdom package set being finalized (all .deb packages must be stable before image builds begin) and SELinux policy being drafted (policy is in the critical path for Age 3a, not Age 2). Coordination cadence: weekly sync between Warmonger (driving), Architect (topology decisions), and Developer (implementation feedback). Sleipnir lead owns BGP profile doc; Yggdrasil lead owns Jenkins pipeline and .deb repository. No blocking cross-features until Age 3a SELinux work begins. Hand-offs: Sleipnir control plane completes in Age 2a, handed to Scientist for convergence analysis and to Micromanager for test plan approval before Age 2b data plane work. Yggdrasil pipeline completes in Age 2a, handed to Captain for GTM/pricing, Moat Ghost for compliance matrix, and BlackMage for supply chain security audit. All three components converge for launch narrative at end of Age 2/beginning of Age 3.

## Decision

This ADR **ACCEPTS** both Kingdom-native BGP Routing Daemon (Sleipnir) and Unheaded OS (Yggdrasil) as Age 2/3 long-term vision features. They are not blocking public launch (scheduled for Q2 2026) but represent essential capabilities for Kingdom to scale beyond 10-node deployments.

**Sleipnir decision**: Build a simplified internal BGP speaker (control plane in Go, data plane in Rust/Aya eBPF) that peers only within Kingdom clusters and writes forwarding state into BPF maps. Begin implementation in Age 2a (Q2 2026) in two phases: control plane + stub data plane, then BFD + hardened XDP integration. Target first deployment in Age 2 (Q4 2026).

**Yggdrasil decision**: Build a hardened Debian image builder pipeline that produces bare-metal ISOs, cloud images, and cloud-optimized artifacts with full CIS/STIG hardening and SELinux policy. Begin in Age 2a (Q2 2026) with the pipeline and .deb automation; begin SELinux policy port in Age 3a (Q4 2026). Target first production artifacts by end of Age 2 (Q4 2026).

Both features are **NOT blocking public launch** and will be communicated to customers as "roadmap differentiators" starting Q3 2026, with GA availability expected by end of 2026 (Sleipnir) and mid-2027 (Yggdrasil with full SELinux).

## Consequences

1. **Architectural**: Kingdom now owns the complete networking stack (fabric routing + service routing + policy enforcement), enabling differentiated positioning against commodity cloud platforms. The BPF-based data plane becomes a first-class abstraction layer that other Kingdom services can depend on (observability, load balancing, traffic shaping).

2. **Engineering**: Go + Rust split becomes standard pattern for Kingdom services (control + data plane separation). BGP profile specification becomes reference architecture for future Kingdom protocol work. Test infrastructure must support network simulation (2+ VM fabric for integration testing).

3. **Compliance and Security**: SELinux policy work unlocks FedRAMP/STIG/PCI-DSS compliance pathways, reducing go-to-market friction for enterprise customers. Supply chain security requirements (signed packages, reproducible builds, audit trails) become enforced by the build pipeline, improving overall Kingdom security posture.

4. **Organizational**: Two new organizational units emerge: Sleipnir team (control plane + data plane, 4-6 engineers, Age 2-3) and Yggdrasil team (image builder + hardening policy, 2-3 engineers, Age 2-3). Knowledge of BGP and SELinux policy becomes in-scope hiring criteria. Partner ecosystem may standardize on Yggdrasil as the "official" Kingdom base image.

5. **Business**: New revenue streams: Sleipnir is table-stakes for Enterprise SKU (no upsell, standard feature). Yggdrasil becomes premium offering (Kingdom Enterprise with audit trails, signed artifacts, SLA image updates). FedRAMP/STIG compliance becomes repeatable, reducing consulting overhead per customer. Market positioning shifts from "scalable container platform" to "hardened, compliant, routable fabric infrastructure."

6. **Roadmap**: Age 2 becomes "infrastructure stability" phase (Sleipnir control, Yggdrasil pipeline); Age 3 becomes "enterprise readiness" phase (SELinux full integration, multi-cloud image variants, FedRAMP audit). All post-alpha prioritization assumes both features complete on schedule.

## References

- RFC 4271: A Border Gateway Protocol (BGP-4)
- RFC 5880: Bidirectional Forwarding Detection (BFD)
- CIS Debian Linux 12 Benchmark v1.0
- NIST SP 800-53 (FedRAMP control mapping)
- RHEL 9 Security Hardening Profile (SELinux reference policy)
- Aya eBPF Framework: https://aya-rs.dev/
- google/gopacket: https://github.com/google/gopacket
- Debian Policy Manual 4.6

---

**Document Status**: Complete, no outstanding questions or placeholders. This document captures the full vision and will serve as the authoritative design reference for both features through Age 2-3.
