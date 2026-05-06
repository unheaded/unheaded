# THE RAGNARÖK SPRINT — The Biggest Battle Plan in Kingdom History

## Convened: 2026-03-25 | Sprint: S78+ | Status: APPROVED

## Context

The Unheaded Kingdom has been on pause during Google interviews, Amazon interviews, job fairs, and RFC submissions. We're back. The project stands at Age 2 ~65%, Age 3 just starting. 415K production LOC, 1.137M total, 34 services, 23 eBPF programs. WEST + EAST bare metal online with BPF flow graph cross-host. Zhen AI operational. UPC Level 6 built. The Well (PostgreSQL) operational. Wire format FROZEN. All P1 bugs fixed. RFC blockers cleared.

**Goal:** Complete Age 2, push deep into Age 3, prepare for public launch, and write new fiction chapters of The First Packet that map 1:1 to technical epics.

---

## THE CHRONICLES: 15 Fiction Chapters → Technical Epics

Each chapter extends "The First Packet" and maps to a concrete engineering epic.

### Wave 1 (No blockers — start immediately)

| Ch | Title | Epic | Size |
|----|-------|------|------|
| 1 | "The Grand Curve" | Doom Renderer Completion | XL |
| 2 | "The Tunnel Under the Mountain" | WireGuard IPv6 Overlay | M |
| 9 | "The Runesmith's Forge" | RFC/Spec Finalization | L |
| 10 | "The Mirror of Kenoma" | Performance + VC Readiness | M |

### Wave 2 (Depends on Wave 1)

| Ch | Title | Epic | Size | Dep |
|----|-------|------|------|-----|
| 3 | "Ghostwheel Learns to Speak" | Zhen RAFT + Champion Agent (ADR-018/019) | XL | — |
| 5 | "The Forging of Sleipnir" | BGP Peering (ADR-69420 Pt 1) | XL | Ch 2 |
| 6 | "The Roots of Yggdrasil" | Hardened OS (ADR-69420 Pt 2) | L | — |
| 7 | "The Witnesses Awaken" | Anamnesis Production Hardening | L | — |

### Wave 3 (Depends on Wave 2)

| Ch | Title | Epic | Size | Dep |
|----|-------|------|------|-----|
| 4 | "The Sanctum Door" | Phylactery Implementation | XL | Ch 7 |
| 8 | "Brand's Gambit" | Adversarial Security Testing | L | Stable services |
| 13 | "The Zhen Engine" | Custom Rust Inference (ADR-017) | XL | Ch 3 |
| 15 | "Nagan Stirs" | Wotan Cluster Replication | XL | Ch 2 |

### Wave 4 (Final push)

| Ch | Title | Epic | Size | Dep |
|----|-------|------|------|-----|
| 11 | "The Severing of the Cord" | Service Breakout | L | All stable |
| 12 | "Gotterdammerung Rehearsal" | Disaster Recovery Testing | L | Ch 4, 7 |
| 14 | "The Opening of the Gates" | Public Launch | M | Ch 8, 10 |

---

## EXECUTION PLAN: 5 Phases

### Phase 1: BIFROST — The Bridge (Days 1-2)

WireGuard IPv6 overlay, fix broken tests, clear legal blockers.

| # | Task | Pri | Owner |
|---|------|-----|-------|
| 1.1 | WireGuard IPv6 tunnel WEST↔EAST | P0 | Stevie |
| 1.2 | fd00:dead:beef::/48 addressing | P0 | Stevie |
| 1.3-1.5 | Fix afxdp/runtime/wotan-client tests | P0 | Agent A |
| 1.6-1.7 | Fix kenoma/dashboard-server tests | P0 | Agent B |
| 1.8-1.10 | CLA, GPL isolation, IETF Note Well | P0 | Agent C |

**Gate:** `go test ./...` zero failures + WireGuard up + legal docs committed

### Phase 2A: MIMIR — The Knowledge (Days 3-4, parallel with 2B)

Finalize all 6 protocol specs (Foundation-06, Sophia-03, Wotan-03, MBC-ISA-00, Shim-00, PQC-00).

**Gate:** All 6 specs zero CRITICAL findings + SPEC-VS-CODE-AUDIT.md PASS

### Phase 2B: HEPHAESTUS — The Forge (Days 3-5, parallel with 2A)

Performance benchmarking (<50ms E2E latency, 1000 req/s) and Doom progress.

**Gate:** Sub-50ms latency demonstrated + BENCHMARKS.md committed

### Phase 3: HEIMDALL — The Watchman (Days 5-6)

README overhaul, demo video, SBOM, Amber IP audit, GitHub public flip.

**Gate:** Repo public, video live, SBOM clean, AMBER-IP-AUDIT.md PASS

### Phase 4: YGGDRASIL — The World Tree (Days 7-9)

BGP peering (FRR/BIRD), NixOS hardening, Phylactery, Lich adversarial.

**Gate:** BGP Established + NixOS hardened + Phylactery functional + Lich clean

### Phase 5: NORNS — The Fate Weavers (Days 10-13)

Zhen RAFT training (3K+ QA pairs, QLoRA, validation), Champion Agent schema, IETF submission, service breakout plan.

**Gate:** Zhen measurably better than base model + Foundation-06 on IETF datatracker

---

## ADR TODO MAPPING

- **ADR-017** (Zhen Hybrid): Context window DONE. Claude API deferred (budget).
- **ADR-018** (Zhen RAFT): Phases 1-6 in NORNS. Phase 7 (continuous loop) post-sprint.
- **ADR-019** (Zhen Champion): Phases 1-2 in NORNS. Phases 3-4 post-sprint.
- **ADR-69420 Sleipnir**: FRR/BIRD peering in YGGDRASIL. Custom Go daemon post-sprint.
- **ADR-69420 Yggdrasil**: NixOS hardening in YGGDRASIL. Debian pipeline post-sprint.
- **ADR-69420 Gleipnir**: Entirely post-sprint (Age 2b).
- **Amber IP Audit**: PRE-PUBLIC BLOCKER in HEIMDALL.
- **Sentinel**: Daily Adversarial Loop in YGGDRASIL. CVE Poller post-sprint.
- **Distribution** (apt/snap): Post-sprint (Age 3).
- **7th Overview RFC**: Post-sprint (Age 3).

---

## CRITICAL PATH TO PUBLIC LAUNCH: 6 DAYS (Phases 1-3 only)

1. WireGuard tunnel (1 session)
2. Fix 5 test failures (1 session, parallelized)
3. Legal docs (1 session, parallelized)
4. Specs final review (2 sessions, parallelized)
5. Performance benchmark (1 session)
6. README + demo video + GitHub flip (2 sessions)

---

_Forged at the Round Table by all minds. The Kingdom marches as one._
_"The circle always closes. You just have to be willing to stay up later than the darkness." — Mad Maria_
_Ragnarok Sprint — March 25, 2026_
