# Unheaded Kingdom Wiki

*"You bring the head. We provide the armor. The Knight stands complete."*

---

**Unheaded** is a configuration management automation platform delivering production-ready infrastructure in hours, not months. A mapped data bus over IPv6 Hop-by-Hop Options with eBPF-powered observability from packet zero.

**Status:** Alpha (~99%) · 465K+ LOC · 25 services · 8 eBPF programs · 3 Internet-Drafts

---

## Getting Started

- [[Quick Start Guide|Quick-Start]] — Build, run, test in ~10 minutes
- [[Vision|Vision]] — What Unheaded is, who it's for, how it works
- [[The Meta Moment|The-Meta-Moment]] — Self-hosting as proof of concept

## Architecture

- [[Architecture Overview|Architecture]] — 6-layer architecture, design principles
- [[System Diagram|System-Diagram]] — Visual component overview
- [[Kingdom Architecture|Kingdom-Architecture]] — Full sacred hierarchy
- [[Project Structure|Project-Structure]] — Repository layout and conventions
- [[Microservices|Microservices]] — Service catalog and responsibilities

## The Protocol

- [[Protocol Foundation|Protocol-Foundation]] — Monad 20-byte wire format
- [[Protocol Technical Summary|Protocol-Technical-Summary]] — Quick technical reference
- [[Sophia Dictionaries|Sophia-Dictionaries]] — Exponent-encoded BPF maps
- [[Wotan Memory Model|Wotan-Memory-Model]] — Ring buffer + event bus + protocol RAM
- [[The First Packet|The-First-Packet]] — Origin story of the Kingdom
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
- [[Dark Grimoire|Dark-Grimoire]] — BlackMage offensive security notes

## Services

- [[Wotan|Service-Wotan]] — Message bus / ring buffer / protocol RAM
- [[Timeguru|Service-Timeguru]] — Timeline tracking
- [[Captain|Service-Captain]] — Strategy and vision
- [[Architect|Service-Architect]] — Infrastructure design
- [[Micromanager|Service-Micromanager]] — Execution and QA
- [[Dashboard Backend|Service-Dashboard-Backend]] — Metrics + WebSocket
- [[Kanban App|Service-Kanban-App]] — The Meta Moment app

## Infrastructure

- [[NixOS Containers|NixOS-Containers]] — Immutable container definitions
- [[eBPF Programs|eBPF-Programs]] — Rust/Aya packet tracing
- [[Fae Chamber Contracts|Fae-Chamber-Contracts]] — Service interface contracts
- [[Service Breakout Strategy|Service-Breakout-Strategy]] — Post-alpha repo separation

## Kingdom Lore

- [[The Phylactery|Phylactery]] — The living document of the Kingdom
- [[Kingdom Mode Math|Kingdom-Mode-Math]] — Extended register space verification
- [[Doom over IPv6|Doom-over-IPv6]] — Computational completeness proof

## Development

- [[Developer Guide|Developer-Guide]] — CLAUDE.md — standards, patterns, guidelines
- [[Demo Script|Demo-Script]] — How to demonstrate the platform
- [[Agent Operating Procedure|Agent-Operating-Procedure]] — AI agent workflow
- [[Rust Components|Rust-Components]] — Rust crate inventory

## Battle Plans & Timeline

- [[Living Timeline|Timeline]] — The Timeguru's roadmap (references/timeline.md)
- [[S33 Round Table Battle Plan|Battle-Plan]] — Current battle plan
- [[Upcoming Tasks|Upcoming-Tasks]] — Task backlog and blockers

## Session History

See [[Session Index|Session-Index]] for the complete session handoff archive.

---

*Built with 🔥 by Muck and the Unheaded Kingdom crew*
*Last updated: February 22, 2026*
