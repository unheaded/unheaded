# Unheaded Wiki

**Unheaded** — configuration management automation platform built around the Unheaded Protocol: a mapped data bus over IPv6 Hop-by-Hop Options with eBPF-powered observability. Provides control plane, service mesh, observability, and security baseline. You bring the application.

**State:** Age 3 in progress. Wire format frozen at v0x01 (12 IANA registries, S67). 6 Internet-Drafts shipped. K8s substrate proven (WAVE17, 2026-05-05). **ASCEND-LINUX L5: xv6 enters user mode on the UPC** (2026-05-14, [Linux on UPC](Linux-on-UPC)). Track-call (A/B/C) pending Captain.

---

## Getting Started

- [[Quick Start Guide|Quick-Start]]
- [[Vision]]
- [[The Meta Moment|The-Meta-Moment]]

## Architecture

- [[Architecture Overview|Architecture]]
- [[System Diagram|System-Diagram]]
- [[Kingdom Architecture|Kingdom-Architecture]]
- [[Project Structure|Project-Structure]]
- [[Microservices]]

## The Protocol Computer (UPC)

The UPC is a virtual CPU implemented in the eBPF runtime. Monad packets are the clock; XDP is the dispatcher; BPF maps are ROM/RAM/page-tables.

- [[UPC Overview|UPC-Overview]] — what the UPC is, BPF maps, MBC ISA summary, Boot Protocol v2
- [[Unheaded Protocol|Unheaded-Protocol]] — Monad + Sophia + Wotan + MBC + Shim + PQC tied together
- [[Dream Ladder|UPC-Dream-Ladder]] — six-level ascent (L1→L6), per-level status
- [[Linux on UPC|Linux-on-UPC]] — ASCEND-LINUX, current frontier (L5 spike, 2026-05-14)
- [[Doom on UPC|Doom-on-UPC]] — L3 computational-completeness proof

## The Protocol (specifications)

- [[Protocol Foundation|Protocol-Foundation]] — Monad 20-byte wire format (FROZEN v0x01)
- [[Protocol Technical Summary|Protocol-Technical-Summary]]
- [[Sophia Dictionaries|Sophia-Dictionaries]]
- [[Wotan Memory Model|Wotan-Memory-Model]]
- [[The First Packet|The-First-Packet]]
- [[MBC ISA Reference|MBC-ISA-Reference]]
- [[Error Registry|Error-Registry]]

### Internet-Drafts

- [[Foundation-06|Draft-Protocol-Foundation-06]]
- [[Sophia-03|Draft-Sophia-Dictionary-03]]
- [[Wotan-03|Draft-Wotan-Memory-03]]
- [[MBC-ISA-00|Draft-MBC-ISA-00]]
- [[Shim-00|Draft-Shim-00]]
- [[PQC-Authentication-00|Draft-PQC-Authentication-00]]

### RFC References

- [[IANA Guide|IANA-Guide]]
- [[RFC Cross-Reference|RFC-Cross-Reference]]
- [[Wire Format Patterns|Wire-Format-Patterns]]

## Architecture Decision Records

Canonical mirror: [[ADR Index|ADR-Index]] (65 ADRs, last synced 2026-05-05).

Recent:
- ADR-064 — Wotan Active/Active Cluster, K8s-native (Proposed, 2026-05-05; supersedes ADR-035)
- ADR-062 — Fuzz / Red-Team / Pentest Framework (Lich codified, 2026-05-04)
- ADR-060 — Zhenai Multi-Model Selector (In Progress, 2026-05-04)
- ADR-052 — Timeline & Battle-Plan Source-of-Truth Policy (Accepted, CI gate live)
- ADR-051 — WAVE13 generate-gemma4 path (Accepted, verdict: RETRAIN)
- ADR-043 — Mímir's Law: UPC-Controlled Baseline (PoC complete 2026-04-11)

## Security & Compliance

- [[Security Overview|Security]]
- [[Security Audit|Security-Audit]]
- [[Security TODOs|Security-TODOs]]
- [[LICH Fuzzing Campaigns|LICH-Campaigns]]
- [[Dark Grimoire|Dark-Grimoire]]

## Services

- [[Wotan|Service-Wotan]] — message bus, ring buffer, protocol RAM
- [[Timeguru|Service-Timeguru]]
- [[Captain|Service-Captain]]
- [[Architect|Service-Architect]]
- [[Micromanager|Service-Micromanager]]
- [[Dashboard Backend|Service-Dashboard-Backend]]
- [[Kanban App|Service-Kanban-App]]

## S36 Four Pillars

- [[Port Registry|Port-Registry]]
- [[Transport Cascade|Transport-Cascade]] — gRPC primary, HTTP fallback
- [[Log Aggregation|Log-Aggregation]]
- [[Service Discovery|Service-Discovery]]

## Mímir's Law (ADR-043)

UPC-controlled OS baseline, drift detection, alerts-only self-healing. Two-plane architecture: Wotan steady-state + Gjallarhorn discrete UPC triggers. PoC complete 2026-04-11 (real-metal validated on EAST).

- [[Overview|Mimirs-Law]]
- [[Wotan Topic Signing|Wotan-Topic-Signing]] — ML-DSA-65 enforcement on `config.*`
- [[Wave 10C Backprop|Wave-10C-Backprop]]
- [[So The Game Goes On|So-The-Game-Goes-On]]

## Recent Progress (Apr 11 → May 14, 2026)

- **2026-05-14** ASCEND-LINUX **Phase 1.4 milestone** (Linux on UPC L5 substrate clears all init() calls; clean halt at U-mode entry) **+ Phase 1.5 spike** (userland MBC loader; xv6 init enters `priv=3` and runs `open` / `dup×2` / `printf` / `vprintf` → `write` ecall). 13 commits. Phase 1.6 is the byte-path through `SYS_write`. See [Linux on UPC](Linux-on-UPC).
- **2026-05-05** WAVE17 — K8s substrate proven (9/9 services Running on 3-node kind). ADR-064 active/active spec landed (impl deferred). cmd/tools/ scaffold for Mímir / Anamnesis Lite / Zhen On-Prem. ADR-Index regenerated 21→65 entries.
- **2026-05-04** WAVE16 — Multi-model selector live in sidebar. 5 model keys. qwen-coder-14b benched. ADR-060 LIVE.
- **2026-05-03** ADR-059 Phase 1 shipped — Zhenai Interactive CLI (`cmd/zhen-cli`).
- **2026-05-02** WAVE15 rewire planning + security-posture review. ADR-056/057/058/059 added.
- **2026-04-28** WAVE13 Phase 2 verdict: **RETRAIN**. ADR-051 Accepted. LoRA underperformed on 6/8 prompts; 2/8 mode-collapse.
- **2026-04-27** Round Table verification audit (19 seats, 2 citations issued + cleared). ADR-052/053/055 added.
- **2026-04-25** WAVE13 Phase 1 — `generate-gemma4` subcommand.
- **2026-04-23** WAVE12 — Kingdom RAFT LoRA. ADR-050 GPU-resident activations.
- **2026-04-21** WAVE11 — 4 attention grad kernels (cosine 1.000). ADR-049.
- **2026-04-20** Learning Gate strict experiments. 24h Consolidation Block.
- **2026-04-17** WAVE10F — Forge Real-Attention Gemma-4 (3000+ LOC, end-to-end LoRA training).
- **2026-04-11** Mímir's Law PoC complete — real-metal validated on EAST.

## Infrastructure

- [[Load Balancers|Load-Balancers]] — HAProxy edge/internal + Nginx per-app sidecars
- [[Containers]] — LXD, containerd, NixOS, Docker
- [[IaC Backends|IaC-Backends]] — Ansible, Terraform, Puppet, Kubernetes, Chef, Salt
- [[Observability Backends|Observability-Backends]] — Prometheus, Grafana, ELK, Jaeger, Nagios
- [[eBPF Programs|eBPF-Programs]] — Rust/Aya + cilium/ebpf, L2–L7
- [[Fae Chamber Contracts|Fae-Chamber-Contracts]]
- [[Service Breakout Strategy|Service-Breakout-Strategy]]

## Lore & Naming

- [[Lore Index|Lore-Index]]
- [[Naming Map|Naming-Map]]
- [[Gnostic Architecture|Gnostic-Architecture]]
- [[Medieval Armory|Medieval-Armory]]
- [[Norse Mythology|Norse-Mythology]]
- [[Sacred Hierarchy|Sacred-Hierarchy]]
- [[Protocol Heritage|Protocol-Heritage]]
- [[Phylactery]]
- [[Kingdom Mode Math|Kingdom-Mode-Math]]
- [[Doom over IPv6|Doom-over-IPv6]] *(legacy stub — see [[Doom on UPC]])*

## Skills

- [[Skills Index|Skills-Index]]

## Development

- [[Developer Guide|Developer-Guide]]
- [[Agent Operating Procedure|Agent-Operating-Procedure]]
- [[Rust Components|Rust-Components]]

## Battle Plans & Timeline

- [[Timeline]]
- [[Battle Plan|Battle-Plan]]
- [[Sessions|Session-Index]]

---

*Source of truth: `references/timeline.md` + `docs/adr/ADR-INDEX.md`. Drift policy: ADR-052 (≤7 days from HEAD).*
