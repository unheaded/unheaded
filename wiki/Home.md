# Unheaded Wiki

**Unheaded** — configuration management automation platform built around the Unheaded Protocol: a mapped data bus over IPv6 Hop-by-Hop Options with eBPF-powered observability. Provides control plane, service mesh, observability, and security baseline. You bring the application.

**State (2026-10-15):** Age 3 in progress. Wire format frozen at v0x01 (13 IANA registries in Foundation draft-06). ASCEND-LINUX Phase 1 complete: an interactive xv6 shell with `ls`/`cat`/`echo`/`wc` runs on the UPC, and the kernel is 100% Unheaded-authored ([[Linux on UPC|Linux-on-UPC]]). On west: Wotan, Akira, dashboard, kanban and The Well in compose, plus Huginn on the host. Work lands on `develop` → `staging` → `main` (ADR-089).

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
- [[Microservices]] — every binary under `cmd/`, by role

## The Protocol Computer (UPC)

The UPC is a virtual CPU implemented in the eBPF runtime. Monad packets are the clock; XDP is the dispatcher; BPF maps are ROM/RAM/page-tables.

- [[UPC Overview|UPC-Overview]] — what the UPC is, BPF maps, MBC ISA summary, Boot Protocol v2
- [[Unheaded Protocol|Unheaded-Protocol]] — Monad + Sophia + Wotan + MBC + Shim + PQC tied together
- [[Dream Ladder|UPC-Dream-Ladder]] — six-level ascent (L1→L6), per-level status
- [[Linux on UPC|Linux-on-UPC]] — ASCEND-LINUX: interactive shell + FS reader (Phase 1 complete 2026-07-02), owned kernel (2026-07-07), EVOLUTION-1
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
- [[Protocol Specs|Protocol-Specs]]

## Architecture Decision Records

Canonical mirror: [[ADR Index|ADR-Index]] (99 entries, synced 2026-10-15).

Recent:
- ADR-102 — Container images use exact version tags (Accepted, 2026-10-15)
- ADR-101 — The Well's migration ledger, `cmd/well-migrate` (Proposed; in use live since 2026-10-15)
- ADR-097 / 098 / 099 — Compliance crosswalk, findings register, remediation as code (Proposed)
- ADR-092 — `/var/log` discipline and rotation (Proposed; steps 1, 3, 6 done)
- ADR-089 — `develop` → `staging` → `main` promotion (Accepted)
- ADR-084 — Huginn host metrics agent (Accepted)
- ADR-016 amendment — The Well's RPO 24 h, nightly backups (Accepted 2026-10-15)

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
- [[Akira|Service-Akira]] — health checks + percentage consensus (ADR-029)
- [[Huginn|Service-Huginn]] — host metrics agent on each host (ADR-084)
- [[kev-poller|Service-KEV-Poller]] — CISA KEV feed, tagged against what we run (ADR-055)

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

## Recent Progress (May → Oct 2026)

Full record: [[Timeline]] (`references/timeline.md`).

- **2026-10-13 → 10-15** The Well brought onto a migration ledger (ADR-101): TLS 1.3 only, an owner role per database, every password rotated out of the repo, nightly backups on a second disk + offsite to east, restore drill weekly ([[The Well|The-Well]], [[Well Backups|Well-Backups]]).
- **2026-10-12** East back after an 11-day hang; ADR-092 log discipline; Huginn log-space alerts; `promtool` rule tests in CI.
- **2026-09-30 → 10-01** Compliance crosswalk (ADR-097) across 800-53, FedRAMP, SOC 2, ISO 27001, CSF 2.0, 800-171, SSDF, CIS; findings register (ADR-098); remediation as code (ADR-099). CI red → green, reproduced locally.
- **2026-09-28 → 09-29** Observability truth pass: `unheaded_http_*` on every service, wrong dashboard numbers fixed, Wotan seq paging and wildcard topics, single-copy live view.
- **2026-09-21** The Meta Moment works in compose for the first time (five defects); dashboard-backend OOM fixed (400 MB → 26 MB).
- **2026-09-09** Meta-gate: every CI gate must prove it can fail (`scripts/check-gates-can-fail.sh`).
- **2026-08-03 → 09-21** Security remediation program: 124 rungs promoted to `staging` in nine reviewed batches; `go vet` clean, `go test ./...` 0 failures.
- **2026-07-29** CI green sweep; per-service run-as UIDs (`pkg/uids`).
- **2026-07-19** EVOLUTION-1 Stage 1 measured (getpid flip deferred, +5409 verifier insns); Doom browser-verified live again (verifier 737,087).
- **2026-07-02 → 07-07** ASCEND-LINUX Phase 1 complete (shell + FS reader, ADR-078); Phases 2.2–2.4 replace every MIT kernel file: the UPC kernel is 100% Unheaded-authored.
- **2026-05-14** `init: starting sh` from xv6 user mode on the UPC.

## Infrastructure

- [[Load Balancers|Load-Balancers]] — HAProxy edge/internal + Nginx per-app sidecars
- [[Containers]] — LXD, containerd, NixOS, Docker
- [[IaC Backends|IaC-Backends]] — Ansible, Terraform, Puppet, Kubernetes, Chef, Salt
- [[Observability Backends|Observability-Backends]] — VictoriaMetrics + Grafana + ClickHouse/Vector live; ELK, Jaeger, Nagios adapters
- [[eBPF Programs|eBPF-Programs]] — Rust/Aya + cilium/ebpf, L2–L7
- [[Fae Chamber Contracts|Fae-Chamber-Contracts]]
- [[Service Breakout Strategy|Service-Breakout-Strategy]]
- [[OS Fork Discipline|OS-Fork-Discipline]] — Yggdrasil fork rules

## Operations

- [[Systemd Units|Systemd-Units]] — what is installed and enabled on west and east, timers
- [[Runbooks]] — `runbooks/` (Champion-executable YAML) + `docs/runbooks/`

## Storage

- [[The Well|The-Well]] — PostgreSQL: databases, roles, TLS, where everything lives on west
- [[Well Backups|Well-Backups]] — backup locations (west + east), schedule, alerts, restore

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
- [[Doom over IPv6|Doom-over-IPv6]] *(legacy stub — see [[Doom on UPC|Doom-on-UPC]])*

## Skills

- [[Skills Index|Skills-Index]]

## Development

- [[Developer Guide|Developer-Guide]]
- [[Agent Operating Procedure|Agent-Operating-Procedure]]
- [[Rust Components|Rust-Components]]
- [[Demo Script|Demo-Script]]
- [[Bug Kill Chain|Bug-Kill-Chain]]
- [[Performance]] — Doom engine performance (historical)

## Battle Plans & Timeline

- [[Timeline]]
- [[Roadmap]]
- [[Upcoming Tasks|Upcoming-Tasks]]
- [[Battle Plan|Battle-Plan]]
- [[Sessions|Session-Index]]

---

*Source of truth: `references/timeline.md` + `docs/adr/ADR-INDEX.md`. Drift policy: ADR-052 (≤7 days from HEAD).*
