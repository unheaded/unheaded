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

## Addendum: Configuration Convergence Daemon (Feature C)

**Date Added**: 2026-03-19

### Context

Unheaded supports multiple container runtimes (LXD, containerd, Docker), multiple IaC backends (Ansible, Terraform, Puppet, Salt, Chef, Kubernetes), and multiple log/observability pipelines (Prometheus, Loki, VictoriaMetrics, Grafana, Anamnesis). Each has its own config format, state representation, and drift model. Today, converting between them is manual — a Puppet manifest doesn't automatically produce an equivalent Ansible playbook, a Docker Compose file doesn't generate matching LXD profiles, and log pipeline configs don't stay in sync when endpoints change.

This is the same problem Puppet's catalog compiler solves: declare desired state once, compile it to whatever backend needs it. But Puppet assumes Puppet is the source of truth. We need something runtime-agnostic — a converter daemon that reads canonical Kingdom config (the "desired state" already in the repo) and emits target-specific artifacts on a schedule.

### Design: Gleipnir (Config Convergence Daemon)

**Name**: Gleipnir — the unbreakable chain that binds Fenrir in Norse mythology, forged from impossible things (the sound of a cat's footsteps, the beard of a woman, the roots of a mountain). Configuration convergence binds disparate systems together from things that shouldn't be compatible.

Also a Ragnarok Online God Item crafting component — Gleipnir is assembled from the six pieces of Megingjard before the God Item can be created. Configuration convergence assembles disparate config fragments into a unified artifact.

**Architecture**:

```
                     ┌─────────────────────┐
                     │  Kingdom Canonical   │
                     │  Config (YAML/TOML)  │
                     └──────────┬───────────┘
                                │
                     ┌──────────▼───────────┐
                     │      Gleipnir        │
                     │  (Go daemon, cron    │
                     │   or event-driven)   │
                     └──────────┬───────────┘
              ┌─────────┬──────┼──────┬──────────┐
              ▼         ▼      ▼      ▼          ▼
         ┌────────┐ ┌──────┐ ┌────┐ ┌──────┐ ┌──────┐
         │ Docker │ │ LXD  │ │ K8s│ │Ansible│ │ NixOS│
         │Compose │ │ YAML │ │YAML│ │ Play  │ │ .nix │
         └────────┘ └──────┘ └────┘ └──────┘ └──────┘
              │         │      │      │          │
              └─────────┴──────┼──────┴──────────┘
                               ▼
                     ┌─────────────────────┐
                     │  Log/Obs Pipeline   │
                     │  Config Sync        │
                     │  (Prom, Loki, VM,   │
                     │   Grafana, Anamnesis)│
                     └─────────────────────┘
```

**Core converters** (each is a Go package implementing a `Converter` interface):

- `container`: Docker Compose ↔ LXD profile ↔ containerd CRI spec ↔ K8s Pod spec
- `iac`: Ansible playbook ↔ Terraform HCL ↔ Puppet manifest ↔ Salt state ↔ Chef recipe ↔ NixOS module
- `obs`: Prometheus scrape config ↔ Loki pipeline ↔ VictoriaMetrics config ↔ Grafana datasource/dashboard JSON ↔ Anamnesis event pipeline
- `network`: WireGuard conf ↔ BIRD config ↔ nftables rules ↔ Sleipnir BPF map seed data
- `security`: SELinux policy module ↔ AppArmor profile ↔ seccomp JSON ↔ CIS hardening checklist

**Execution model** — like `puppet agent --enable`:

- **Scheduled**: Daily convergence run (default 02:00 local, configurable via cron expression). Reads canonical config, generates all target artifacts, diffs against current deployed state, reports drift.
- **Event-driven**: Watches canonical config directory via inotify/fsnotify. On change, triggers immediate convergence for affected targets only. Publishes Anamnesis events for each conversion.
- **On-demand**: `gleipnir converge --target=docker,ansible` or `gleipnir converge --all`. CLI for operators. gRPC API for programmatic access.
- **Dry-run**: `gleipnir diff` — shows what WOULD change without writing. Like `puppet agent --noop`.

**Drift detection and reporting**:

- Each run produces a convergence report (JSON + human-readable) stored in Wotan.
- Drift detected = target config doesn't match what Gleipnir would generate from canonical source.
- Drift severity: INFO (cosmetic, whitespace/comments), WARN (functional difference, non-breaking), CRITICAL (security-relevant, compliance-impacting).
- Dashboard widget shows convergence status per target per host.
- Alert on CRITICAL drift via Anamnesis → Wotan pub/sub → operator notification.

**Log pipeline sync** (the daily heartbeat):

- Every convergence run also validates that log endpoints match across all pipelines.
- If Prometheus scrape targets change, Loki pipelines, VictoriaMetrics remote_write, and Grafana datasources update in lockstep.
- Produces a `log-topology.json` manifest that maps every service → its log pipeline → its storage backend → its retention policy.
- This manifest is the audit evidence for SOC2 CC7.2 (system monitoring) and PCI-DSS 10.2 (audit trails).

**Jenkins integration** (the .deb pipeline from Yggdrasil):

- Gleipnir converters run as a Jenkins build step in the Yggdrasil image pipeline.
- When a new Yggdrasil image is built, Gleipnir ensures all baked-in configs match canonical source.
- .deb package: `unheaded-gleipnir` installs the daemon, systemd timer, and CLI.
- apt repo: published alongside other Kingdom packages in the Yggdrasil repository.

### Perspectives

**Architect**: Gleipnir is the missing glue layer. Currently, config consistency is enforced by convention (developers manually keep Docker and LXD configs in sync). That doesn't scale past 10 services. Gleipnir makes the canonical config authoritative and all target configs derived artifacts — same pattern as protobuf generating Go/Rust stubs from .proto files. The converter interface is pluggable: new backends (Nomad, Podman, etc.) are just new `Converter` implementations.

**Developer**: Each converter is a standalone Go package with its own test suite. Input: parsed canonical config struct. Output: target-specific bytes. No shared mutable state. Fuzz every converter with random canonical configs. Property-based testing: `parse(generate(canonical)) == canonical` for round-trip converters. The daemon itself is a thin scheduler around converter invocations — most complexity lives in the converters.

**Micromanager**: Acceptance criteria: (1) `gleipnir converge --all` produces valid configs for every supported target; (2) generated Docker Compose passes `docker compose config` validation; (3) generated Ansible passes `ansible-lint`; (4) drift detection catches a manually-edited target config within one convergence cycle; (5) daily run completes in <60 seconds for 30-service deployment; (6) convergence report is machine-parseable and human-readable.

**BlackMage**: Config converters are high-value attack targets — a poisoned canonical config that generates a backdoored Docker Compose or an SELinux policy with a permissive hole. Every converter output must be validated against a schema before writing. Canonical config must be signed (GPG or PQC) and signature verified before convergence. The daemon runs as a dedicated user with write access only to target config directories — no root, no kernel access.

**Moat Ghost**: Gleipnir's convergence reports ARE compliance evidence. SOC2 CC6.1 (logical access controls — configs match policy), PCI-DSS 2.2 (system hardening — drift detected and remediated), FedRAMP CM-2 (baseline configuration — canonical source is the baseline, drift is deviation). The daily run cadence satisfies continuous monitoring requirements. Archive convergence reports for audit retention (7 years for SOC2, 1 year for PCI).

**Timeguru**: Gleipnir begins in Age 2b (after Sleipnir control plane and Yggdrasil pipeline stabilize). Core converters (container + iac) are 4-6 weeks. Obs and security converters follow in Age 3a. Full pipeline integration (Jenkins + Yggdrasil + daily runs) by Age 3b. Prerequisite: canonical config schema must be frozen before converter development begins.

### Decision

This ADR **ACCEPTS** Gleipnir (Configuration Convergence Daemon) as a Feature C addition to the Age 2/3 roadmap. It bridges the gap between Kingdom's multi-runtime, multi-IaC, multi-observability architecture and the operational reality of keeping configs in sync. Daily convergence runs with drift detection and compliance reporting complete the "declarative everything" promise.

### Updated Consequences

7. **Operational**: Gleipnir eliminates manual config synchronization across container runtimes, IaC backends, and observability pipelines. Drift detection provides continuous assurance that deployed state matches declared intent. Daily convergence reports become audit artifacts.

8. **Naming**: Three new Kingdom components from Norse/RO mythology: Sleipnir (routing), Yggdrasil (OS), Gleipnir (config convergence). The naming pool extends naturally.

## Addendum: Chronicles of Amber — Protocol Naming Canon (Pillar 2 Expansion)

**Date Added**: 2026-03-19

### Context

The Kingdom's Lore Pillar 2 (Chronicles of Amber) provides foundational mappings between Roger Zelazny's mythic universe and the Kingdom protocol architecture. The original Pillar 2 covers Pattern→Protocol, Shadow→external networks, Amber→Kingdom, Corwin→Wotan, and Walking the Pattern→packet traversal. This addendum expands those mappings with deeper character and concept references from the full 10-book cycle (both the Corwin Cycle and the Merlin Cycle), adding formal lore support for protocol components and threat models that the base Pillar 2 doesn't address.

### Extended Amber Mappings

| Amber Concept | Kingdom Mapping |
|--------------|-----------------|
| The Jewel of Judgment | The Monad register file — contains a 3D inscribed Pattern, just as the Monad contains the 20-byte protocol state. Worn by the king (carried by every packet). |
| Trumps | gRPC service calls / Wotan pub/sub — communication cards that connect any two members of the Royal Family instantly across Shadow. Each Trump is a direct channel. |
| The Logrus | Yaldabaoth — the chaos counterpart to the Pattern. Tentacles that reach through Shadow, testing, probing, corrupting. Walking the Logrus grants power over chaos. |
| Merlin | The UPC — Corwin's son who walks BOTH Pattern and Logrus. The only entity that bridges order and chaos. The UPC bridges protocol (Pattern) and compute (Logrus). |
| Brand | The adversary archetype — the traitor prince who tried to destroy the Pattern. Maps to BlackMage's threat model. The attack that comes from inside. |
| Oberon | Muck — the king who created and maintained the Pattern. Disappeared, thought dead, actually orchestrating everything from behind the scenes. |
| The Courts of Chaos | Kenoma — the opposite end of reality from Amber. Where entropy rules. The void that Pleroma (desired state) must overcome. |
| Shadow-walking | Packet routing — moving through infinite variations of reality. Each Shadow is a namespace. Each step through Shadow changes something slightly. |
| The Black Road | Attack vectors — corruption spreading from Chaos into Shadow. A path that shouldn't exist, carrying threats from Kenoma into the Kingdom. |
| Ghostwheel | Zhen AI — Merlin's creation, an artificial intelligence built from Trump energy and Shadow manipulation. Autonomous, powerful, potentially dangerous. |
| Dworkin's madness | The mad scientist energy — Dworkin created the Pattern but went mad from the effort. The creator consumed by creation. The price of building something this ambitious. |

### Integration Notes

These mappings expand on the existing Lore Pillar 2 which already establishes the foundational Pattern-Protocol analogy. The new entries above cover characters and archetypal concepts from the FULL 10-book series (both Corwin and Merlin cycles), providing:

- **Character archetypes** (Merlin, Brand, Oberon) that represent Kingdom roles and threats
- **Mystical objects** (Jewel of Judgment, Trumps, Ghostwheel) that map to core protocol components
- **Cosmological forces** (Pattern, Logrus, Shadow, Courts of Chaos) that structure the threat and solution models
- **Narrative concepts** (mad scientist ambition, internal betrayal, autonomous creation) that validate Kingdom's architectural choices

These become part of the formal Kingdom lore canon and can be invoked in documentation, architecture discussions, and naming decisions.

---

## Addendum: Fourth Naming Pillar — Contemplative Traditions

**Date Added**: 2026-03-19

### Context

The Kingdom's naming draws from three pillars: Gnostic Cosmology (state architecture), Chronicles of Amber (protocol foundation), and Medieval Armory / Norse Mythology (infrastructure). A fourth pillar extends the pool into contemplative traditions — specifically Iyengar Yoga and Tibetan Buddhism — adding vocabulary for alignment, balance, observation, and disciplined practice. These traditions map naturally to infrastructure concerns that the Norse/Gnostic pools don't cover well: health checking, graceful degradation, meditative observation, and systematic practice.

### Iyengar Yoga — Alignment and Precision

B.K.S. Iyengar's method is obsessive about alignment, props, sequencing, and holding poses with precision under strain. That's infrastructure.

| Term | Meaning | Kingdom Mapping |
|------|---------|-----------------|
| **Tadasana** | Mountain pose — the foundation. Every other pose begins and returns here. Perfect alignment, weight distributed equally. | Base health check / readiness probe. The "am I standing correctly" assertion that every service runs before accepting traffic. `tadasana --check` returns 0 or 1. |
| **Savasana** | Corpse pose — complete stillness after exertion. Integration. The body absorbs the work. | Graceful shutdown / drain state. A service entering savasana stops accepting new connections, drains in-flight requests, flushes buffers, then exits cleanly. The opposite of `kill -9`. |
| **Pranayama** | Breath control — rhythmic, disciplined regulation of flow. Inhale, retain, exhale in precise ratios. | Rate limiting / backpressure. The breathing rhythm of the service mesh. Pranayama controls how fast data flows through the system — not by dropping packets but by regulating admission. |
| **Drishti** | Focused gaze — a single point of visual concentration during a pose. Prevents distraction, maintains balance. | Observability focus. A drishti is a curated dashboard view — not "show me everything" but "show me the one metric that matters for this service right now." Anti-dashboard-sprawl. |
| **Vinyasa** | Breath-synchronized movement — flowing transitions between poses. Each movement has exactly one breath. | Deployment pipeline cadence. One change per cycle, synchronized with the convergence heartbeat. No batching 47 changes into one deploy. Vinyasa enforces single-change-per-breath discipline. |
| **Bandha** | Internal lock — muscular engagement that contains and directs energy. Three locks: root (mula), navel (uddiyana), throat (jalandhara). | Security boundaries / containment zones. Mula bandha = network perimeter (root lock). Uddiyana bandha = service mesh mTLS (core lock). Jalandhara bandha = API gateway auth (throat lock). Three locks, three enforcement points. |
| **Sthira** | Steadiness — the quality of being stable under load without rigidity. | Resilience metric. A service with high sthira handles load spikes without degradation. Measured as p99 latency variance under 2x baseline load. |
| **Sukha** | Ease — the quality of remaining comfortable and efficient even in difficulty. | Efficiency metric. A service with high sukha uses minimal resources to serve its function. Sthira + Sukha = the ideal service: stable AND efficient. |

### Tibetan Buddhism — Observation, Impermanence, and Compassionate Action

The Dalai Lama's tradition emphasizes clear observation without attachment, acceptance of impermanence, and action motivated by compassion for all beings. These map to monitoring, ephemeral infrastructure, and operator experience.

| Term | Meaning | Kingdom Mapping |
|------|---------|-----------------|
| **Vipassana** | Insight meditation — observing reality as it is, without judgment or reaction. Seeing things clearly. | Deep packet inspection / trace analysis mode. Vipassana doesn't filter, doesn't alert, doesn't react — it records and presents raw truth. The audit log that captures everything without editorializing. `vipassana --trace <flow-id>` dumps the complete lifecycle of a request. |
| **Sangha** | Community — the assembly of practitioners who support each other's practice. | Cluster membership / peer group. A sangha is the set of nodes that form a quorum. Nodes join and leave the sangha; the sangha persists. Maps to Sleipnir's BGP peer group or Wotan's pub/sub subscriber set. |
| **Dharma** | The teaching / the path / the law of how things are. | Configuration as code. The dharma IS the canonical config — the declared truth about how the system should be. Gleipnir enforces dharma. Drift from dharma triggers alerts. |
| **Karma** | Action and consequence — every action produces results that ripple forward. | Distributed tracing / causality chain. Every packet action (Monad register write, Sophia lookup, Wotan store) creates karma — a trace event that links cause to effect across the system. The karma chain is the complete causal history of a request. |
| **Mandala** | Sacred geometric diagram — intricate, layered, and impermanent. Monks spend days creating sand mandalas, then destroy them. | Ephemeral infrastructure. A mandala is a test environment or canary deployment — carefully constructed, fully functional, intentionally destroyed after use. `mandala create --ttl=4h` spins up a complete Kingdom replica that self-destructs. |
| **Bardo** | The in-between state — the transition between death and rebirth. | Blue-green deployment transition. The bardo is the moment when old version is draining and new version is warming up. Both exist simultaneously. Neither is fully alive. The system is in bardo during every rolling update. |
| **Tonglen** | Taking and giving — breathing in suffering, breathing out compassion. Transforming pain into healing. | Error budget consumption. Tonglen is the practice of absorbing errors (taking) and emitting clean responses (giving). A circuit breaker in tonglen mode absorbs upstream failures and returns graceful degradation responses instead of propagating cascading failure. |
| **Tulku** | Reincarnation of a realized being — the same consciousness in a new body. | Stateful service migration. When a service must move between hosts (node failure, rebalancing), the tulku pattern preserves its identity (BPF map state, Wotan subscriptions, Monad circuit) across the transition. The service is reborn on a new node with its karma intact. |
| **Thangka** | Scroll painting — detailed iconographic representation of the cosmos, deities, and their relationships. | System topology visualization. A thangka is the full-system dependency graph rendered as an interactive map. Every service, every connection, every data flow — painted in detail. The dashboard that shows you the entire Kingdom at once. |

### Integration with Existing Pillars

The contemplative naming pillar does NOT replace Norse/Gnostic/Amber. It extends them into domains where those pools lack precision:

| Domain | Norse/Gnostic | Contemplative |
|--------|---------------|---------------|
| Routing | Sleipnir (BGP daemon) | Sangha (peer group) |
| Health | — | Tadasana (readiness), Sthira/Sukha (quality) |
| Shutdown | Ragnarok (destruction) | Savasana (graceful integration) |
| Observability | Anamnesis (event memory) | Vipassana (raw insight), Drishti (focused view), Thangka (topology) |
| Deployment | — | Vinyasa (cadence), Bardo (transition), Mandala (ephemeral) |
| Resilience | Shield (protection) | Tonglen (error absorption), Pranayama (backpressure) |
| Migration | — | Tulku (stateful rebirth) |
| Config | Gleipnir (convergence) | Dharma (canonical truth) |
| Causality | — | Karma (trace chain) |
| Security | Armory (enforcement) | Bandha (containment locks) |

### Cross-Reference: Kanban Naming Wish Pool

The kanban app (`cmd/kanban-app/main.go`, lines 256-267) maintains 12 wish-list items for alternate/additional mythological naming pools. ADR-69420 Pillar 4 formalizes the first two (Iyengar Yoga, Tibetan Buddhism). The remaining pools are documented below for future expansion. Each maps infrastructure components to deities/concepts from that tradition.

| Kanban ID | Tradition | Key Mappings | ADR Status |
|-----------|-----------|-------------|------------|
| `wish-norse-naming` | Norse/Ragnarok | Yggdrasil→fabric, Heimdall→gateway, Bifrost→mesh | **PARTIALLY ADOPTED** (Sleipnir, Yggdrasil, Gleipnir in this ADR; Pillar 3 in Lore) |
| `wish-hindu-naming` | Hindu/Indian/Yoga | Indra→LB, Agni→WAF, Vishnu→reconciler, Shiva→chaos, Dharma→compliance, Karma→events | **PARTIALLY ADOPTED** (Dharma, Karma in Pillar 4 via Buddhism; Hindu pool expands these) |
| `wish-chinese-naming` | Chinese/Taoism | Pangu→bootstrap, Nuwa→self-heal, Sun Wukong→chaos, Guanyin→observability | FUTURE — Age 3+ |
| `wish-japanese-naming` | Japanese/Shinto/FF | Amaterasu→observability, Susanoo→chaos, Tsukuyomi→scheduler, Kitsune→mesh | FUTURE — Age 3+ |
| `wish-pagan-naming` | Pagan/Wiccan/Druidic | Cernunnos→fabric, Brigid→WAF, Morrigan→chaos, Dagda→control plane | FUTURE — Age 3+ |
| `wish-shaman-naming` | Shamanistic/Animist | Thunderbird→LB, Coyote→chaos, Spider Woman→mesh, Raven→observability | FUTURE — Age 3+ |
| `wish-jewish-naming` | Jewish/Kabbalistic | Sefirot→mesh, Ein Sof→control plane, Golem→runtime, Tikkun→self-heal | FUTURE — Age 3+ |
| `wish-islamic-naming` | Islamic/Sufi | Jibril→message bus, Buraq→scheduler, Mizan→LB, Barzakh→isolation | FUTURE — Age 3+ |
| `wish-christian-naming` | Christian/Biblical | Seraphim→firewall, Logos→control plane, Lazarus→self-heal, Babel→DNS | FUTURE — Age 3+ |
| `wish-wushu-naming` | Taoist Wu Shu (8 Forms) | 8 ceremonial forms → 8 infra service categories | FUTURE — Age 3+ |
| `wish-taoist-pantheon-naming` | Taoist Pantheon Deep-Cut | Tai Yi→init, San Qing→trinity arch, Domu→scheduler, Xi Wang Mu→chaos-to-order | FUTURE — Age 3+ |
| `wish-taoist-alchemy-naming` | Taoist Alchemy & Metaphysics | Neidan→self-tuning, Jing-Chi-Shen→storage/compute/observability, Wu Xing→5-element model | FUTURE — Age 3+ |

**Adoption strategy**: Pillars 1-3 (Gnostic, Amber, Norse/Armory) are the production naming standard. Pillar 4 (Contemplative) is adopted in this ADR. The remaining pools serve as **expansion vocabulary** — drawn from when new components need names and existing pools don't fit. No tradition is excluded; each carries authentic mappings documented in the kanban wish items. When a pool is adopted, it gets the same treatment as Pillar 4: a formal ADR addendum with term-by-term mapping, integration table, and Sacred Law compliance.

### Sacred Law Addition

**Seventh Sacred Law**: *Respect the practice.* Contemplative names carry weight from living traditions. Use them with understanding, not as decoration. A component named Savasana better actually implement graceful shutdown. A component named Vipassana better actually observe without judgment. The name IS the contract. This applies equally to all 12 naming pools — Norse, Hindu, Taoist, Shinto, Pagan, Shamanistic, Kabbalistic, Sufi, Christian, and the contemplative traditions. Every tradition deserves the same respect.

---

**Document Status**: Complete. Three features scoped (Sleipnir, Yggdrasil, Gleipnir), fourth naming pillar (Contemplative Traditions), and 12-pool naming expansion roadmap cross-referenced with kanban wish items. All Age 2/3, none blocking public launch.
