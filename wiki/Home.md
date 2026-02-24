# Unheaded Wiki

**Unheaded** is a configuration management automation platform built around the Unheaded Protocol, a mapped data bus over IPv6 Hop-by-Hop Options with eBPF-powered observability from packet zero. You bring the application. Unheaded provides the control plane, service mesh, observability, and security baseline.

**Status:** Alpha (~99%) · ~260K production LOC (~464K w/ tests) · 25 services · 8 eBPF programs · 3 Internet-Drafts (IETF Experimental)

---

## Getting Started

- [[Quick Start Guide|Quick-Start]] — Build, run, test in ~10 minutes
- [[Vision|Vision]] — What Unheaded is, who it's for, how it works
- [[The Meta Moment|The-Meta-Moment]] — Self-hosting as proof of concept

## Architecture

- [[Architecture Overview|Architecture]] — 6-layer architecture, design principles
- [[System Diagram|System-Diagram]] — Visual component overview
- [[Kingdom Architecture|Kingdom-Architecture]] — Component hierarchy and naming conventions
- [[Project Structure|Project-Structure]] — Repository layout and conventions
- [[Microservices|Microservices]] — Service catalog and responsibilities

## The Protocol

- [[Protocol Foundation|Protocol-Foundation]] — Monad 20-byte wire format
- [[Protocol Technical Summary|Protocol-Technical-Summary]] — Quick technical reference
- [[Sophia Dictionaries|Sophia-Dictionaries]] — Exponent-encoded BPF maps
- [[Wotan Memory Model|Wotan-Memory-Model]] — Ring buffer + event bus + protocol RAM
- [[The First Packet|The-First-Packet]] — Protocol design origin and rationale
- [[MBC ISA Reference|MBC-ISA-Reference]] — Monad Bytecode instruction set
- [[Error Registry|Error-Registry]] — Protocol error codes

### Internet-Drafts

- [[draft-bellis-unheaded-protocol-foundation-04|Draft-Protocol-Foundation-04]]
- [[draft-bellis-unheaded-sophia-dictionary-01|Draft-Sophia-Dictionary-01]]
- [[draft-bellis-unheaded-wotan-memory-01|Draft-Wotan-Memory-01]]

### RFC References

- [[IANA Guide|IANA-Guide]] — IANA considerations for Unheaded
- [[RFC Cross-Reference|RFC-Cross-Reference]] — Standards we build on
- [[Wire Format Patterns|Wire-Format-Patterns]] — Common wire format idioms

## Architecture Decision Records

- [[ADR-001|ADR-001-Gnostic-State-Management]] — Gnostic state management
- [[ADR-002|ADR-002-Kingdom-Naming-Convention]] — Kingdom naming convention
- [[ADR-003|ADR-003-eBPF-Rust-Aya-Framework]] — eBPF with Rust + Aya
- [[ADR-004|ADR-004-No-External-Deps-Policy]] — No external dependencies policy
- [[ADR-005|ADR-005-Wotan-Message-Backbone]] — Wotan as message backbone
- [[ADR-006|ADR-006-Vanilla-JS-Frontend]] — Vanilla JS frontend (no framework)
- [[ADR-007|ADR-007-Container-Hardening-Strategy]] — Container hardening strategy
- [[ADR-008|ADR-008-Security-Hardening-Baseline]] — Security hardening baseline
- [[ADR-009|ADR-009-Parish-Boundaries]] — Parish boundaries
- [[ADR-010|ADR-010-Sealed-Cask-Deployment]] — Sealed cask deployment
- [[ADR-011|ADR-011-Storage-Layer-Planning]] — Storage layer planning
- [[ADR-012|ADR-012-BPF-Verifier-Risk-Mitigation]] — BPF verifier risk mitigation
- [[ADR-013|ADR-013-Routing-Header-Support]] — Routing header support
- [[ADR-014|ADR-014-IPv6-Fragmentation-Support]] — IPv6 fragmentation support
- [[ADR-015|ADR-015-Go-Fiber-HTTP-Layer]] — Go Fiber HTTP layer

## Security

- [[Security Overview|Security]] — Security policy and reporting
- [[Security Audit|Security-Audit]] — Full audit findings
- [[Security TODOs|Security-TODOs]] — Current security work items
- [[LICH Fuzzing Campaigns|LICH-Campaigns]] — Automated adversary testing
- [[Dark Grimoire|Dark-Grimoire]] — Attack surface taxonomy and offensive security notes

## Services

- [[Wotan|Service-Wotan]] — Message bus / ring buffer / protocol RAM
- [[Timeguru|Service-Timeguru]] — Timeline tracking
- [[Captain|Service-Captain]] — Strategy and vision
- [[Architect|Service-Architect]] — Infrastructure design
- [[Micromanager|Service-Micromanager]] — Execution and QA
- [[Dashboard Backend|Service-Dashboard-Backend]] — Metrics + WebSocket
- [[Kanban App|Service-Kanban-App]] — The Meta Moment app

## S36 Four Pillars

- [[Port Registry|Port-Registry]] — The Doom Range (16666-26666) port allocation
- [[Transport Cascade|Transport-Cascade]] — gRPC-first transport with HTTP fallback
- [[Log Aggregation|Log-Aggregation]] — The Chronicler's Well: centralized structured logging
- [[Service Discovery|Service-Discovery]] — The Cartographer's Eye: four-layer resolution

## Infrastructure

- [[Containers|Containers]] — Immutable container definitions. LXD, containerd, NixOS, Docker
- [[IaC Backends|IaC-Backends]] — Interchangeable config management. Ansible, Terraform, Puppet, Kubernetes, Chef, Salt
- [[Observability Backends|Observability-Backends]] — Interchangeable logging/metrics/tracing. Prometheus, Grafana, ELK, Fluentd, Jaeger, Nagios + more
- [[eBPF Programs|eBPF-Programs]] — Rust/Aya + cilium/ebpf packet tracing (L2–L7)
- [[Fae Chamber Contracts|Fae-Chamber-Contracts]] — Service interface contracts
- [[Service Breakout Strategy|Service-Breakout-Strategy]] — Post-alpha repo separation

## Lore & Naming

- [[Lore Index|Lore-Index]] — All lore documents and naming conventions
- [[Naming Map|Naming-Map]] — Complete lore name → technical component reference
- [[Gnostic Architecture|Gnostic-Architecture]] — Gnostic cosmology → state management mapping
- [[Medieval Armory|Medieval-Armory]] — Armor pieces → infrastructure layers
- [[Norse Mythology|Norse-Mythology]] — Norse/Wagnerian names → protocol and messaging
- [[Sacred Hierarchy|Sacred-Hierarchy]] — Full component hierarchy
- [[Protocol Heritage|Protocol-Heritage]] — Lineage from ARINC 429 to Unheaded
- [[The Phylactery|Phylactery]] — Encrypted storage layer and state persistence
- [[Kingdom Mode Math|Kingdom-Mode-Math]] — Extended register space verification
- [[Doom over IPv6|Doom-over-IPv6]] — Computational completeness proof

## Skills

- [[Skills Index|Skills-Index]] — 16 AI agent specializations for development, ops, and docs

## Development

- [[Developer Guide|Developer-Guide]] — CLAUDE.md — standards, patterns, guidelines
- [[Demo Script|Demo-Script]] — How to demonstrate the platform
- [[Agent Operating Procedure|Agent-Operating-Procedure]] — AI agent workflow
- [[Rust Components|Rust-Components]] — Rust crate inventory

## Battle Plans & Timeline

- [[Timeline|Timeline]] — Project roadmap (references/timeline.md)
- [[Current Sprint Plan|Battle-Plan]] — Active sprint plan and deliverables
- [[Upcoming Tasks|Upcoming-Tasks]] — Task backlog and blockers

## Session History

See [[Session Index|Session-Index]] for the complete session handoff archive.

---

*Last updated: February 22, 2026*
