# Intellectual Property Inventory — The Unheaded Kingdom

**Project:** Unheaded Kingdom  
**Last updated:** 2026-02-25  
**Contact:** stevie@bellis.tech

---

## Table of Contents

1. [Proprietary IP (BSL 1.1)](#proprietary-ip-bsl-11)
2. [Protocol Specifications (MIT / Apache 2.0)](#protocol-specifications-mit--apache-20)
3. [Open Source Components](#open-source-components)
4. [Trademarks](#trademarks)
5. [Trade Secrets](#trade-secrets)
6. [Patent Strategy](#patent-strategy)
7. [Attribution & Acknowledgments](#attribution--acknowledgments)

---

## Proprietary IP (BSL 1.1)

The following intellectual property is owned by Unheaded Kingdom and licensed under the **Business Source License 1.1 (BSL 1.1)**. See `/LICENSE` for full terms.

BSL 1.1 grants you the right to use this software for non-production purposes freely. Production use requires a license agreement. After 4 years from the release date, the license converts to Apache 2.0, allowing unlimited use.

### Core Protocol & Architecture

#### Monad Protocol

**Status:** Proprietary (BSL 1.1), planned public specification (draft-bellis-unheaded-monad-header)

The **Monad Protocol** is a packet-level protocol header format for in-band signaling of data plane control information across networks. It is the conceptual foundation of the Unheaded Kingdom's entire ecosystem.

**Key components:**
- **20-byte register format** — Fixed-size header with standardized field layout
- **TLV (Type-Length-Value) option encoding** — Extensible option space for future capability signaling
- **CRC-16 checksum scheme** — Polynomial-based integrity checking (x^16 + x^15 + x^2 + 1)
- **Payload type enumeration** — Classification system for encapsulated content

**Specification documents:**
- Monad spec Section 1–5: Core header format
- Monad spec Section 6–8: TLV option semantics
- Monad spec Section 12: Computational completeness proof (via DOOM)

**Where it lives in the codebase:**
- `/docs/protocol/monad-spec.md` — Core specification
- `/cmd/` — Protocol parsing, generation, and encapsulation code
- `/internal/monad/` — Protocol library (TLV parsing, CRC calculation, etc.)
- `/internal/bpf/` — eBPF program implementing Monad header handling

**IP Classification:** Monad Protocol is actively defended. However, the specification will be published (non-confidential) to:
1. Enable interoperability with downstream network operators
2. Prevent submarine patents by third parties
3. Establish a de facto standard

#### Sophia Dictionary System

**Status:** Proprietary (BSL 1.1)

The **Sophia Dictionary** is the Unheaded Kingdom's symbolic metadata system. It maps protocol state changes to descriptive records that can be rendered in multiple formats (JSON, YAML, Markdown).

**Key components:**
- **BPF map layout** — Kernel-space data structure design for efficient lookup
- **Symbolic encoding** — Fixed vocabulary for state machine transitions
- **Rendering pipelines** — Triple-format output (MD→JSON/YAML)

**Where it lives:**
- `/internal/sophia/` — Dictionary library
- `/pkg/sophia-cli/` — Command-line tools
- `/docs/lore/sophia.md` — Conceptual documentation

**IP Classification:** Proprietary. The encoding scheme is not published separately; it is embedded in the Unheaded applications and accessible only through public APIs (gRPC services).

#### Wotan Memory Model

**Status:** Proprietary (BSL 1.1)

The **Wotan** subsystem provides unified memory abstraction for distributed flow state. It features:

- **Ring buffer architecture** — Lock-free, multi-reader data structure for event streaming
- **Fae Chamber** — The in-kernel scratchpad memory region used for correlation and deduplication
- **Distributed trace model** — Time-synchronized state replicas across agents

**Key components:**
- Wotan server (coordination)
- Per-agent Wotan client (local state)
- BPF ring buffer attachment points
- gRPC streaming API (TopicStream)

**Where it lives:**
- `/cmd/wotan/` — Server binary
- `/pkg/wotan-client/` — Client library
- `/internal/wotan/` — Core implementation
- `/docs/protocol/wotan-spec.md` — Ring buffer and state model

**IP Classification:** Proprietary. The ring buffer layout and Fae Chamber semantics are core competitive differentiation.

#### Anamnesis Event System

**Status:** Proprietary (BSL 1.1)

**Anamnesis** is the event collection, correlation, and remediation framework. It consumes raw network telemetry and produces actionable events.

**Key components:**
- Event correlation rules (CEP-like)
- Distributed deduplication (via Wotan)
- Automatic remediation triggers
- State machine for operational insight

**Where it lives:**
- `/cmd/trace-collector/` — Event ingestion and correlation
- `/internal/events/` — Event definitions and routing
- `/internal/remediation/` — Auto-remediation logic
- `/configs/anamnesis.toml` — Configuration

**IP Classification:** Proprietary. The correlation rule semantics and remediation decision tree are differentiated IP.

#### Shield eBPF Pipeline

**Status:** Proprietary (BSL 1.1)

**Shield** is the packet processing pipeline in the kernel space. It inspects, marks, tracks, and samples packets for observability and enforcement.

**Key components:**
- **Packet marker (XDP)** — Marks packets with Monad headers
- **Flow tracker (TC)** — Maintains bidirectional flow state
- **Latency probe (kprobe)** — Measures packet processing latency
- **Syscall tracer (tracepoint)** — Observes system call activity
- **WAF (Web Application Firewall)** — L7 packet filtering

**Where it lives:**
- `/ebpf-programs/` — Source code for all eBPF programs
- `/cmd/ebpf-loader/` — User-space loader
- `/internal/bpf/` — BPF map abstractions
- `/docs/security/shield-architecture.md` — Threat model

**IP Classification:** Proprietary. The packet marking scheme, flow correlation, and filtering rules are core differentiation. However, the eBPF code is written in a modular, reusable style to encourage adoption by the broader eBPF community.

#### Monad Bytecode (MBC) ISA

**Status:** Proprietary (BSL 1.1), candidate for defensive publication

The **Monad Bytecode (MBC)** is a 43-opcode instruction set architecture designed for efficient execution in constrained kernel eBPF environments.

**Instruction set breakdown:**
- Arithmetic/logic: ADD, SUB, MUL, DIV, MOD, AND, OR, XOR, NOT, SHL, SHR
- Memory: LOAD, STORE, MLOAD, MSTORE, ALLOC, FREE
- Control: JMP, JZ, JNZ, CALL, RET, HLT
- I/O: READ, WRITE, SYSCALL
- Special: NOP, ENTER, LEAVE, FRAME_ALLOC, FRAME_FREE
- BPF integration: BPF_CALL, BPF_MAP_LOOKUP, BPF_MAP_UPDATE

**Where it lives:**
- `/specs/mbc-isa.md` — Complete opcode reference
- `/cmd/monad-disasm/` — Disassembler
- `/monad-mbc/` (Rust) — Reference interpreter and translator
- `/doom/doom.mbc` — Compiled DOOM executable (proof of computational completeness)

**IP Classification:** Proprietary. The instruction set and encoding scheme are core differentiation. However, we may defensively publish the MBC ISA spec to prevent third parties from claiming ownership.

#### unheaded-daemon Orchestration Engine

**Status:** Proprietary (BSL 1.1)

The **unheaded-daemon** is the core orchestrator that launches, manages, and coordinates all subsystems (Monad packet processing, Wotan state, Anamnesis events, Shield eBPF pipeline).

**Key responsibilities:**
- Startup/shutdown sequencing
- Inter-subsystem health checks
- Configuration management
- Metrics aggregation
- Policy enforcement

**Where it lives:**
- `/cmd/unheaded-daemon/` — Main binary
- `/internal/daemon/` — Orchestration logic
- `/internal/health/` — Health check framework
- `/configs/unheaded.toml` — Configuration schema

**IP Classification:** Proprietary. The orchestration logic and inter-subsystem communication protocols are core IP.

#### The Lich: Automated Adversary Framework

**Status:** Proprietary (BSL 1.1)

**The Lich** is an automated adversary simulation and attack injection framework. It can:
- Inject synthetic network events (with Monad headers)
- Simulate DDoS patterns
- Test Anamnesis correlation rules
- Measure Shield filtering latency
- Stress-test Wotan distributed state

**Key components:**
- Event generator (templates for common attack patterns)
- Injection engine (sends packets at line rate)
- Result validator (checks if Shield and Anamnesis respond correctly)
- Performance profiler

**Where it lives:**
- `/cmd/lich/` — Main CLI
- `/internal/lich/` — Core framework
- `/scripts/lich-scenarios/` — Pre-built attack templates

**IP Classification:** Proprietary. The attack templates and validation logic are differentiated.

#### Kingdom Mode (System Identity)

**Status:** Proprietary (BSL 1.1)

**Kingdom Mode** is a system identity and governance model. Each Unheaded installation can be deployed in one of several modes:

- **Hermit Mode** — Standalone, no replication or coordination
- **Vassal Mode** — Reports to a central Wotan server
- **Liege Mode** — Acts as a central coordinator for multiple Vassals
- **Wanderer Mode** — Mesh-connected, peer-to-peer state sharing

Kingdom Mode affects:
- Which subsystems are active
- Whether state is local or replicated
- Which APIs are exposed
- Which remediation actions are allowed

**Where it lives:**
- `/internal/kingdom-mode/` — Mode selection and enforcement
- `/configs/kingdom-mode.toml` — Configuration
- `/docs/architecture/kingdom-modes.md` — Policy documentation

**IP Classification:** Proprietary. The mode enumeration and state synchronization rules are core differentiation.

---

## Protocol Specifications (MIT / Apache 2.0)

The Unheaded Kingdom intentionally publishes some protocol specifications under **permissive licenses (MIT / Apache 2.0)**. This enables ecosystem adoption and prevents third-party submarine patents.

### Published (or Planned to be Published) Specifications

#### 1. draft-bellis-unheaded-monad-header

**Status:** Planned for publication (IETF Internet-Draft)

**License:** MIT / Apache-2.0 (to be selected)

The **Monad Protocol Header** specification describes the 20-byte fixed header format, TLV option encoding, and CRC checksum scheme. This will be submitted to the IETF as an Internet-Draft to establish the Monad Protocol as an open standard.

**Who can use it:** Anyone (permissive license)

**Why published:** To enable downstream operators, router vendors, and observability tools to interoperate with Unheaded deployments. Also to prevent submarine patents.

**Where it lives (draft):**
- `/docs/protocol/monad-spec.md` (technical reference)
- Will be published at: https://datatracker.ietf.org/doc/draft-bellis-unheaded-monad-header/

#### 2. draft-bellis-unheaded-metric-header-00

**Status:** Planned for publication (IETF Internet-Draft)

**License:** MIT / Apache-2.0 (to be selected)

The **UNHEADED_METRIC_V1** specification defines the optional Monad TLV option format for embedding observability metadata directly in packet headers. It includes:

- Type 0x2A: Latency measurement (microseconds)
- Type 0x2B: Custom classifier tag (e.g., "DDoS", "Anomaly")
- Type 0x2C: Remediation action taken (reserved for future)

**Who can use it:** Anyone (permissive license)

**Where it lives (draft):**
- `/docs/protocol/metric-header-spec.md`
- Will be published at: https://datatracker.ietf.org/doc/draft-bellis-unheaded-metric-header/

#### 3. License-Protocols Declaration

**Status:** Published in `/LICENSE-PROTOCOLS`

**License:** MIT

A foundational declaration that "protocol specifications published under LICENSE-PROTOCOLS are intentionally permissive to enable ecosystem adoption."

**Where it lives:**
- `/LICENSE-PROTOCOLS` — Canonical location
- Referenced by: All protocol drafts
- Referenced by: Any third-party implementation of Monad, UNHEADED_METRIC_V1, etc.

---

## Open Source Components

### DOOM Subsystem

**Status:** GPL v2.0 (isolated, not linked)

**Location:** `/doom/doomgeneric/` (git submodule)

**License:** GNU General Public License v2.0

The Unheaded Kingdom incorporates the **doomgeneric** portable DOOM source port by ozkl (based on the original DOOM source code by id Software, 1993). This serves as proof that the Monad Protocol achieves computational completeness (Section 12 of the specification).

**Key points:**
- GPL v2 code is **confined to the `doom/` directory only**
- The GPL code is **compiled to MBC bytecode** and runs inside the **eBPF VM sandbox**
- There is **no linking, compilation merging, or shared address space** with the BSL 1.1 codebase
- The boundary is enforced by the Linux kernel's eBPF VM sandbox
- The GPL v2 license **does not apply to any code outside the `doom/` directory**

**For more details:** See `/THIRD_PARTY.md` (GPL Boundary section) and `/doom/LICENSE`.

### Shareware Data

**Status:** id Software Shareware (freely redistributable)

**Location:** `/doom/doom1.wad` (.gitignore'd, not in repo)

**License:** id Software Shareware

The DOOM Shareware WAD file contains game data (levels, sprites, textures) and is governed by id Software's original shareware terms, which permit free redistribution.

---

## Trademarks

The following names and marks are claimed by the Unheaded Kingdom. They are **unregistered** but are actively used to identify our products and services.

| Trademark | Type | Status | Notes |
|-----------|------|--------|-------|
| **Unheaded** | Word mark | Claimed, unregistered | Name of the software project and product family |
| **The Unheaded Kingdom** | Phrase mark | Claimed, unregistered | Project identity and brand name |
| **Monad Protocol** | Phrase mark | Claimed, unregistered | Name of the core protocol |
| **Shield** | Word mark | Claimed, unregistered | eBPF-based packet processing subsystem |
| **Wotan** | Word mark | Claimed, unregistered | Distributed memory and state coordination system |
| **Anamnesis** | Word mark | Claimed, unregistered | Event collection and correlation framework |
| **The Lich** | Phrase mark | Claimed, unregistered | Automated adversary simulation framework |
| **Sophia Dictionary** | Phrase mark | Claimed, unregistered | Symbolic metadata system |
| **Kingdom Mode** | Phrase mark | Claimed, unregistered | System identity and governance model |

### Trademark Usage Policy

**Non-commercial use:** You may use "Unheaded", "The Unheaded Kingdom", and related marks in non-commercial contexts (documentation, community discussion, etc.) with attribution.

**Commercial use:** If you are commercializing a product or service, please contact **stevie@bellis.tech** for permission to use Unheaded trademarks.

**Misuse:** Do not use Unheaded trademarks in a way that suggests endorsement or official affiliation unless licensed to do so.

### Contact for Trademark Inquiries

**Email:** stevie@bellis.tech  
**Subject line:** "Trademark usage request — [your project name]"

Include:
1. Your name and organization
2. Description of intended use
3. Context (commercial, open-source fork, research, etc.)

---

## Trade Secrets

The following information is protected as trade secrets under the BSL 1.1 license and applicable law:

| Category | Description | Confidentiality Level |
|----------|-------------|----------------------|
| **Architecture decisions** | Internal design choices not reflected in public specifications | HIGH |
| **Performance tuning parameters** | Cache sizes, buffer depths, polling intervals optimized through testing | HIGH |
| **Security hardening configurations** | Specific BPF program constraints, rate limits, and anomaly thresholds | HIGH |
| **Optimization algorithms** | Packet deduplication heuristics, correlation rule weighting | HIGH |
| **Operational runbooks** | Deployment patterns, scaling configurations, failure modes | MEDIUM |

### Protection Measures

1. **Access control:** Trade secret information is available only to authorized Unheaded Kingdom maintainers and licensees under NDA.
2. **Documentation:** Kept in private repositories or encrypted documents. Public documentation abstracts away secret details.
3. **Source code:** Proprietary algorithms are implemented in closed-source binaries or obfuscated source (for BSL 1.1 code).

### Disclosure Risk

If you are interested in licensing Unheaded Kingdom with access to trade secrets (e.g., for a commercial deployment or academic research partnership), contact **stevie@bellis.tech**.

---

## Patent Strategy

### No Patents Filed (Intentional)

The Unheaded Kingdom has **not filed any patents** on the core technology. This is an intentional strategic choice.

**Rationale:**

1. **Trade Secrets are stronger than patents in this domain.** 
   - Patents require public disclosure of the invention (fighting innovation).
   - Trade secrets can be maintained indefinitely with reasonable security measures.
   - For protocol and infrastructure software, trade secrets offer better protection.

2. **BSL 1.1 provides business protection.**
   - The license restricts production use for 4 years, creating a moat.
   - After 4 years, conversion to Apache 2.0 is automatic (no perpetual control needed).

3. **Publishing protocol specs prevents submarine patents.**
   - By publishing Monad, UNHEADED_METRIC_V1, and other specs under permissive licenses, we establish prior art.
   - Third parties cannot patent these protocols later.
   - We are immune to patent suits on our own specs.

### Defensive Publication (Candidate)

We are **considering defensive publication** of the **MBC ISA** to prevent third-party patent claims. This would involve:

1. Publishing the full MBC specification (instructions, encoding, examples)
2. Placing it in the public domain or under a permissive license
3. Publishing to a defensive publication service (e.g., IP.com) to establish prior art

**Expected timeline:** Q3 2026

**Consultation:** Patent strategy may change if the Kingdom raises institutional investment or changes its business model. All stakeholders will be notified of changes.

### Submarine Patent Risk

**Scenario:** A third party (e.g., a router vendor) patents the Monad Protocol concept before we do, then demands licensing fees.

**Mitigation:** Our publication of draft-bellis-unheaded-monad-header and related specs establishes prior art and makes any patent claim vulnerable to invalidation.

### Liability

The Unheaded Kingdom makes **no representation or warranty** regarding patent infringement. See `/LICENSE` (BSL 1.1) for liability limitations.

---

## Attribution & Acknowledgments

The Unheaded Kingdom stands on the shoulders of giants. We are grateful to the maintainers and contributors of the following projects:

### Open Source Projects

- **Linux kernel & eBPF subsystem** — Without the kernel's BPF VM and ring buffer infrastructure, none of this would be possible.
- **LLVM & Rust** — The monad-mbc translator depends on LLVM IR and Rust's excellent type system.
- **Tokio** — Async runtime for the trace-collector and Wotan server.
- **gRPC & Protocol Buffers** — Foundation of Wotan's distributed state model.
- **doomgeneric** — ozkl's portable DOOM source port proved MBC computational completeness.
- **Gorilla WebSocket** — Dashboard real-time updates.
- **Prometheus** — Metrics and observability.
- **Cilium eBPF** — Documentation and examples of best practices (though we use aya-rs).

### Research & Inspiration

- **Brendan Gregg** — BPF and perf observability techniques
- **Luca Deri** — ntop and nProbe flow processing
- **Bill Lin** — FPGA packet processing (inspirational)
- **Van Jacobson** — Packet tracing and TCP congestion control

### Community

- The **eBPF community** (eBPF Foundation, Linux Foundation)
- The **IETF CBOR and structured data working groups**
- The **Rust embedded and systems programming community**

### Code Generation Attribution

Portions of the Unheaded Kingdom codebase were generated or improved with assistance from:

- **Claude Opus 4.6** (Anthropic) — Campaign 1 (TopicStream gRPC Sprint), Campaign 2.2 (Dashboard Backend eBPF Wiring), Campaign 3 (Security Hardening), Campaign 4 (Legal Sprint)
- **Gemini** (Google) — Selected security and performance optimizations

---

## Summary Table

| Asset | Type | License | Status | Location |
|-------|------|---------|--------|----------|
| Monad Protocol | Proprietary IP | BSL 1.1 | Core differentiator | /docs/protocol/ |
| Sophia Dictionary | Proprietary IP | BSL 1.1 | Core differentiator | /internal/sophia/ |
| Wotan Memory Model | Proprietary IP | BSL 1.1 | Core differentiator | /cmd/wotan/ |
| Anamnesis Event System | Proprietary IP | BSL 1.1 | Core differentiator | /cmd/trace-collector/ |
| Shield eBPF Pipeline | Proprietary IP | BSL 1.1 | Core differentiator | /ebpf-programs/ |
| MBC ISA | Proprietary IP | BSL 1.1 | Core differentiator | /specs/ |
| unheaded-daemon | Proprietary IP | BSL 1.1 | Core differentiator | /cmd/unheaded-daemon/ |
| The Lich | Proprietary IP | BSL 1.1 | Testing framework | /cmd/lich/ |
| Kingdom Mode | Proprietary IP | BSL 1.1 | Governance model | /internal/kingdom-mode/ |
| draft-bellis-unheaded-monad-header | Protocol Spec | MIT/Apache 2.0 | Public spec (planned) | /docs/protocol/ |
| draft-bellis-unheaded-metric-header-00 | Protocol Spec | MIT/Apache 2.0 | Public spec (planned) | /docs/protocol/ |
| LICENSE-PROTOCOLS | Permissive | MIT | Public declaration | /LICENSE-PROTOCOLS |
| DOOM subsystem | Open Source | GPL v2.0 | Isolated | /doom/doomgeneric/ |
| Doom1.wad | Shareware | id Software | Game data | /doom/doom1.wad (not in repo) |
| Unheaded, The Unheaded Kingdom, Monad Protocol, etc. | Trademarks | Unregistered | Claimed | All products |

---

## Revision History

| Date | Change | Author |
|------|--------|--------|
| 2026-02-25 | Initial creation during S52 Legal Sprint | Claude Opus 4.6 |
| TBD | Defensive publication decision on MBC ISA | TBD |
| TBD | IETF Internet-Draft submission (draft-bellis-unheaded-monad-header) | TBD |

---

## Document Control

**Classification:** PUBLIC (this document is intended for public distribution)  
**Version:** 1.0  
**Owner:** Unheaded Kingdom Legal / stevie@bellis.tech  
**Last reviewed:** 2026-02-25  
**Next review date:** 2026-12-31

---

**For questions about Unheaded Kingdom's intellectual property, contact stevie@bellis.tech**
