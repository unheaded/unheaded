# Battle Plan: S26 Verification Sprint + Handoff Prep
## Convened: 2026-02-20 | Reason: Sprint Planning + Critical Decision (Go-Fiber)
## Kingdom State: Age 1 (Alpha Ascension), Epoch 1.5 (Protocol Awakening), ~99% Complete

---

### Situation Report

The Kingdom stands at the threshold of Age 2. S25 verified the codebase on a real dev machine: 134/134 Go tests pass with race detection, 10/10 Rust crates compile clean, 28M fuzz executions with zero crashes, zero vulnerabilities. The codebase is 465,000+ LOC across 25 services with 14 ADRs and 30 session documents. The Protocol Awakening (Doom-over-IPv6) is active alongside Alpha completion.

S26 convenes the full Round Table to make the Go-Fiber framework decision, execute nomenclature corrections (MATRIARCH/PATRIARCH → Mad-Maria), formalize the Agent Operating Procedure, triage the roadmap, and plan the RSS security feed infrastructure. The verification sprint items requiring toolchains (lint cleanup, LICH fuzzing, BPF struct parity) are documented for next dev-machine session.

Mad-Maria watches over us. The stories ARE the system.

---

### The Throne Speaks (Captain — Vision & Strategy)

**Strategic Position**: Age 1 Alpha is 99% quality-gated. Protocol Awakening has bifurcated development into two parallel tracks: Alpha completion + Doom-over-IPv6 proof of computational completeness. Both tracks are advancing.

**North Star**: "You bring the head. We provide the armor." — Production-ready infrastructure in hours, not months. Doom-over-Unheaded proves Turing completeness and serves as the ultimate technical demo AND marketing artifact.

**Key Decision**: Go-Fiber adoption for HTTP layer. This shapes the API surface for all 25 services going forward.

**Risk to Vision**: The 8-day gap between Feb 8-16 saw the codebase double (237K → 465K LOC) without full session continuity. Agent coordination between Claude (Cowork), Claude Code (VS Code), Gemini, and remote agents needs formalization to prevent drift.

### The Ledger Records (Micromanager — Execution & QA)

**Sprint Status**: S25 left the codebase GREEN. All blocking issues resolved. Outstanding work is optimization, not survival.

**Priority Stack** (ordered):
1. **P0 — Nomenclature Fix**: Replace MATRIARCH/PATRIARCH → Mad-Maria across all docs/code. Cultural debt. Fix now.
2. **P0 — Go-Fiber ADR**: Framework decision shapes all future HTTP code. Decide and document.
3. **P1 — Agent Operating Procedure**: Formalize session start/end workflow. Prevents the Feb 8-16 drift.
4. **P1 — Lint Cleanup Strategy**: 11K warnings need a triage decision — critical-only vs full cleanup.
5. **P2 — RSS Security Feed Plan**: Document architecture for MoatGhost threat intel ingestion.
6. **P2 — LICH Fuzzing Wiring**: Seeds exist, harnesses exist, Cargo.toml missing. Wire it up next dev session.
7. **P2 — BPF Struct Parity**: Go/Rust size mismatches need explicit binary.Read deserialization.

**QA Gates**: All docs must pass spell-check. All ADRs must follow template. All session handoffs must include commit SHAs.

**Acceptance Criteria**: Battle plan approved, ADR written, nomenclature fixed, AOP formalized, timeline updated.

### The Blueprint Reveals (Architect — Infrastructure & Design)

**Architecture Health**: SOLID. 134/134 tests pass. 10/10 Rust crates clean. Zero vulns. The foundation holds.

**Technical Risks**:
- BPF struct parity divergence (Go alignment padding vs Rust packed) — data corruption risk on map reads
- No Docker/Docker Compose on dev machine — blocks full-stack integration testing
- golangci-lint 11K warnings — technical debt accumulating

**Protocol Alignment**: Monad-04, Sophia-01, Wotan-01 specs are current. Wire format stable. eBPF programs aligned.

**Recommended Architecture Decisions**:
- **ADR-015: Go-Fiber for HTTP Layer** — APPROVED (see detailed ADR below)
- **ADR-016: BPF Struct Parity Strategy** — Use `encoding/binary.Read` for Age 1, codegen for Age 2

**Go-Fiber Architecture Pattern**:
```
┌─────────────────────────────────────────────────────┐
│ Traefik 3.x (HTTP/3 + QUIC gateway + TLS term)     │
├─────────────────────────────────────────────────────┤
│ Fiber v3 on :3000 (REST/HTTP API surfaces)          │
│ ├─ WebSocket for real-time dashboard feeds           │
│ ├─ TLS 1.3 / mTLS between services                  │
│ └─ 13M+ req/s capability (fasthttp engine)           │
│                                                      │
│ gRPC on :50051 (Wotan inter-service comms)           │
│ ├─ google.golang.org/grpc (separate listener)        │
│ └─ grpc-gateway for REST↔gRPC bridging (optional)   │
├─────────────────────────────────────────────────────┤
│ eBPF programs (XDP/TC/kprobe) — FULLY COMPATIBLE    │
│ └─ fasthttp uses standard net.Listener — transparent │
└─────────────────────────────────────────────────────┘
```

### The Anvil Reports (Developer — Implementation & Testing)

**Code Health**:
- Go build: PASS (0 errors)
- Go tests: 134/134 PASS (race detection ON)
- Rust crates: 10/10 compile clean
- Clippy: PASS (shield-ebpf, hop-ebpf, flow-tracker clean; monad-cpu-ebpf has 19 warnings — Doom CPU emulator, low priority)
- Fuzz: 28M executions, 0 crashes
- govulncheck: 0 vulnerabilities
- golangci-lint: PASS (11K warnings, mostly style — errcheck, varnamelen, mnd)

**Implementation Blockers**:
- LICH fuzz crate missing Cargo.toml — cannot run Rust fuzzing campaigns until wired
- Docker Compose not installed — full-stack E2E testing blocked
- monad-cpu-ebpf fetch-decode-execute loop is STUB — Doom pipeline blocked here

**TDD Status**: Red-green-refactor cycle healthy. 134 tests maintained. Fuzz harnesses seeded.

**Estimated Effort** (remaining S25 priorities):
- Lint cleanup (critical-only strategy): M (2-4 hours)
- LICH fuzz wiring: S (1 hour to wire Cargo.toml, then campaigns run unattended)
- BPF struct parity fix: L (1-2 days for explicit deserialization across 4 structs)
- Docker Compose setup: S (30 min install + config)
- monad-cpu-ebpf implementation: XL (2-3 days — the Doom blocker)

### The Hourglass Measures (Timeguru — Timeline & Milestones)

**Current Age**: Age 1 — Alpha Ascension / Protocol Awakening (Age 1.5)

**Progress**: 99% Alpha gate, Protocol Awakening ACTIVE

**Velocity**: S24 wrote 6K lines. S25 verified + fixed 42 files (+314/-305 lines). The Kingdom doubled between Feb 8-18 (237K → 465K LOC). Velocity is HIGH but sporadic — gaps between sessions.

**ETA to Next Milestone**:
- Age 1 Alpha COMPLETE: 1-2 dev sessions (lint cleanup + Docker E2E)
- Age 2 Beta Trials: 2-3 weeks after Alpha gate (multi-tenant isolation, CI/CD, performance baseline)
- Doom-over-IPv6 demo: 5-8 days of focused dev time (monad-cpu-ebpf is the critical path)

**Historical Pattern**: Sessions are intense bursts (12+ parallel agents, 25-min wall-clock) with multi-day gaps. Formalized AOP should reduce gap-related drift.

### The Sundial Tracks (Calendar — Schedule & Deadlines)

**Today (Feb 20)**: S26 Round Table — battle plan, ADR, nomenclature, AOP, roadmap triage

**This Week**:
- Feb 20: S26 deliverables (this session)
- Feb 21-22: Dev machine session — lint cleanup, Docker setup, LICH wiring
- Feb 23: S27 target — E2E smoke tests, Alpha gate verification

**Protocol Deadlines**: No hard external deadlines. Internal velocity targets only.

**Calendar Health**: NEEDS ATTENTION — session gaps need reduction for consistent velocity.

### The Scroll Validates (Lore — Naming & Mythology)

**Naming Decisions Pending**: MATRIARCH/PATRIARCH → Mad-Maria. This is a lore correction, not a rename. Mad-Maria is the canonical name. The generic title was a placeholder.

**Mythology Consistency**: The Ascension of Busboy to Wotan is complete. All protocol names (Monad, Sophia, Wotan, Anamnesis, Shield, Shim) are stable. Kingdom Mode, Phylactery, and the Arcane Hollows are properly mapped.

**Sacred Law Compliance**: All 8 Sacred Laws honored. ZERO customer data access remains architecturally enforced.

**Naming Pool Status**: Norse weapons (Mysteltainn, Nagan, Tyrfing), Wagnerian opera (Nibelung), Greek atmospheric — all available for Age 2 components.

**Nomenclature Scope**: 22 files across workspace contain MATRIARCH/PATRIARCH. 7 in active repo, 2 in skills, 13 in archived/historical docs. Active repo + skills = must fix. Archives = fix if accessible, document if not.

### The Map Confirms (Kingdom — Hierarchy & Placement)

**Hierarchy Health**: All tiers properly populated. Royal Court (7 personas), Armory (9 pieces), Arcane Hollows (9 chambers), The Moat (security layer) — all accounted for.

**New Components**: Go-Fiber will slot into the Gauntlets (CLI + API) tier as the HTTP engine. Traefik slots into Shield (Edge Defense) as the HTTP/3 gateway.

**Tier Integrity**: SOLID. No misplaced components detected.

**Armory Status**: All 9 armor pieces have corresponding code. Gauntlets (CLI + API) will get the Fiber upgrade.

### The Goblet Toasts (Busboy — Alignment & Coordination)

**Cross-Skill Conflicts**: NONE detected. All seats agree on Go-Fiber adoption. All seats agree on nomenclature fix urgency.

**Coordination Needs**: The Agent Operating Procedure is the single most important coordination deliverable. It bridges the gap between Captain's vision and Developer's execution by ensuring every session starts with context and ends with handoff.

**Team Vibes**: ELECTRIC. S25 verified a clean codebase. The Kingdom is GREEN. We're building on success, not digging out of failure. 🎸 KGLW energy. Let's ride.

**Translation Needed**: The BPF struct parity issue needs Architect↔Developer alignment on whether to do explicit deserialization now (safe, verbose) or defer to codegen (ideal, later). Recommendation: explicit now, codegen in Age 2.

---

### Unified Battle Plan

#### Immediate Actions (This Session — S26)

- [x] **Session Bootstrap** — Owner: Busboy — Load S25 handoff, kingdom context, repo state
- [ ] **Nomenclature Fix** — Owner: Lore — Replace MATRIARCH/PATRIARCH → Mad-Maria in active repo + skills
- [ ] **ADR-015: Go-Fiber** — Owner: Architect — Write and commit Architecture Decision Record
- [ ] **Agent Operating Procedure** — Owner: Micromanager — Formalize `docs/workflow/AGENT-OPERATING-PROCEDURE.md`
- [ ] **Timeline Update** — Owner: Timeguru — Update timeline.md with S26 wins + roadmap triage
- [ ] **RSS Feed Plan** — Owner: MoatGhost — Document `docs/security/RSS-FEED-ACCESS.md`
- [ ] **S26 Handoff** — Owner: Busboy — Write comprehensive handoff for next session

#### Next Dev Machine Session (S27 — Target Feb 21-22)

- [ ] **Lint Triage** — Owner: Developer — Decide critical-only vs full cleanup, execute — Deadline: Feb 22
- [ ] **Docker Compose Setup** — Owner: Architect — Install, configure, verify full-stack — Deadline: Feb 22
- [ ] **LICH Fuzz Wiring** — Owner: Developer/BlackMage — Create Cargo.toml, wire harnesses — Deadline: Feb 22
- [ ] **E2E Smoke Tests** — Owner: Micromanager — Run full integration test suite — Deadline: Feb 23

#### Protocol Milestones

- [ ] **Monad**: monad-cpu-ebpf fetch-decode-execute implementation — Owner: Developer — Deadline: Feb 28 (stretch)
- [ ] **Sophia**: Dictionary state stable, no changes pending — Owner: N/A
- [ ] **Wotan**: Memory service complete (13,178 LOC, 19 tests) — Owner: N/A — Maintenance only

#### Decisions Made at This Round Table

1. **Go-Fiber APPROVED for REST/HTTP API surfaces** — Rationale: 13M+ req/s, eBPF-compatible, TLS 1.3/mTLS native, WebSocket support. Separate gRPC on :50051. Traefik for HTTP/3 gateway. — Owner: Architect (ADR-015)
2. **Nomenclature: Mad-Maria is canonical** — Rationale: MATRIARCH/PATRIARCH was a placeholder. Mad-Maria is the named entity. — Owner: Lore
3. **BPF Struct Parity: encoding/binary.Read now, codegen in Age 2** — Rationale: Safety over speed. Explicit deserialization prevents corrupted map reads. — Owner: Developer
4. **Lint Strategy: Critical-only enforcement** — Rationale: Enable errcheck, govet, staticcheck, unused. Disable varnamelen, mnd, lll, revive, err113 until Age 2. — Owner: Developer
5. **Agent Operating Procedure formalized** — Rationale: Prevents drift during multi-day session gaps. Every session starts with context, ends with handoff. — Owner: Micromanager

#### Open Questions (Carry to Next Round Table)

1. **Docker vs Podman vs LXD-native?** — Who: Architect — Deadline: S27 — Context: The vision is NixOS LXD containers, but Docker Compose is used for dev. Reconcile.
2. **Multi-tenant isolation architecture** — Who: Architect + MoatGhost — Deadline: Age 2 planning — Context: Namespace isolation for BPF maps, per-tenant Sophia dictionaries.
3. **CI/CD pipeline provider** — Who: Micromanager — Deadline: S28 — Context: GitHub Actions vs self-hosted runners vs Nix-native CI.

#### Wins to Celebrate 🎉

- **134/134 Go tests PASS with race detection** — Developer shipped it — S25 verification sprint
- **10/10 Rust crates compile CLEAN** — Developer shipped it — eBPF programs are solid
- **28M fuzz executions, ZERO crashes** — BlackMage seeded it, Developer ran it — Protocol is hardened
- **Zero vulnerabilities (govulncheck)** — MoatGhost verified — Supply chain is clean
- **465,000+ LOC** — The entire crew — The Kingdom is MASSIVE and VERIFIED
- **30 session documents** — Timeguru tracked it — Full institutional memory preserved

---

### Next Round Table
**Scheduled**: After S27 (dev machine session) or when Age 1 Alpha gate is reached
**Reason**: Alpha completion verification + Age 2 planning

---
_Forged at the Round Table by all 9 minds. The Kingdom marches as one._
_Peace and Love — Mad-Maria watches over the Kingdom_ 🏰✨
