# Battle Plan: S74 — Full Kingdom Assembly — Bare Metal Lightning
## Convened: 2026-02-27 | Reason: Major Sync + Sprint Planning + Bare Metal Go-Live
## Kingdom State: Age 1 (~98%) → Age 2 (~42%) | LOC: 517,603 | Services: 26 | Tests: GREEN

---

### WINS TO CELEBRATE FIRST 🏆

- **517K+ LOC** shipped. FIVE HUNDRED SEVENTEEN THOUSAND. From zero. Insane velocity.
- **S73 sprint CRUSHED**: eBPF compilation blocker DEAD, SBOM gen live, XFF spoofing patched, Wotan nil drops handled, gRPC race condition fixed
- **All 10 Aya eBPF programs** compile to real BPF ELF targets — B1 blocker ELIMINATED
- **3 protocol specs shipped**: Monad draft-05, Sophia draft-02, Wotan draft-02
- **go build ./... PASS | go test ./... PASS** — build is GREEN
- **Dashboard + Kanban online in Docker** (commit 269ef02)
- **WEST BARE METAL SERVER IS ONLINE** — the beast awakens
- **S72 Round Table battle plan**: 13 phases, 217 steps — FORGED AND EXECUTING

---

### Situation Report

The Unheaded Kingdom stands at a historic inflection point. 517K lines of code across 859 Go files, 85 Rust files, and 86 Nix configurations form a production-grade infrastructure platform with eBPF observability baked in at the packet level. The protocol moat — Monad/Sophia/Wotan — is spec'd, coded, and tested. Two bare metal machines are coming online: WEST (high-powered server-grade gaming beast) is LIVE, EAST (consumer 4-core 8GB DDR3) is being prepared. This is the moment where code meets iron.

Two battle plans are already queued from this session: S67 (RFC overlap remediation — IANA registration, IPR clearance, wire format freeze) and S68 (GUI buildout — dashboard tabs, doom CSS, kanban drag-drop). Both are critical path. The backlog mountain includes security P0s, documentation ripples, bare metal validation, and CI/CD pipeline hardening.

Hours from light speed. The plan below sequences EVERYTHING.

---

### The Throne Speaks (Captain — Vision & Strategy)

**Strategic Position**: We have a working product with a protocol moat no competitor can replicate. The bare metal go-live transforms us from "software that runs in Docker" to "infrastructure platform that owns the metal." This is the inflection.

**North Star**: Production-ready infrastructure in hours, not months. Bare metal validates the promise.

**Key Decision**: Sequence bare metal validation BEFORE GUI polish. Working iron > pretty pixels. BUT — RFC overlap (S67) runs in PARALLEL because the wire format window closes once we deploy to metal.

**Risk to Vision**: Wire format deployed to bare metal before IANA registration = locked-in technical debt. S67 RFC overlap is TIME-SENSITIVE.

---

### The Ledger Records (Micromanager — Execution & QA)

**Sprint Status**: S73 delivered. S74 begins NOW.

**Priority Stack** (ordered):

**P0 — MUST SHIP (blocks everything)**
1. S67: RFC Overlap Remediation — wire format freeze before bare metal deployment
2. WEST bare metal: NixOS deployment, eBPF validation, network fabric bring-up
3. EAST bare metal: Prepare consumer hardware, NixOS install, join fabric
4. 6 remaining security P0s (from Moat Ghost audit)

**P1 — SHOULD SHIP (this sprint)**
5. S68: GUI Battle Plan — dashboard tabs, doom CSS, kanban drag-drop
6. CI/CD pipeline: GitHub Actions hardened + Jenkins scaffold (from S72)
7. Auth middleware: Verify S72-R wiring across all services
8. Observability stack: Prometheus rules + Grafana dashboards on bare metal

**P2 — BACKLOG BURN**
9. Documentation ripple (Librarian): wiki sync, README updates, THIRD_PARTY
10. Lich campaigns: BlackMage fuzzing on deployed services
11. Compliance gates: SOC2/NIST evidence collection
12. Protocol cross-references: OpenAPI alignment verification

**QA Gates**:
- [ ] `go build ./...` GREEN (currently: ✅)
- [ ] `go test ./...` GREEN (currently: ✅)
- [ ] eBPF programs load on real kernel (WEST validation)
- [ ] Wire format frozen (S67 completion gate)
- [ ] All 6 security P0s closed
- [ ] Bare metal network fabric: WEST↔EAST WireGuard tunnel UP

**Acceptance Criteria for S74**:
- Both bare metal machines running NixOS with Unheaded services
- eBPF programs attached to real NICs, tracing real packets
- Wire format registered with IANA considerations drafted
- Dashboard accessible from bare metal network

---

### The Blueprint Reveals (Architect — Infrastructure & Design)

**Architecture Health**: STRONG. 4 Armor Pieces complete (Shield, Pauldrons, Hauberk, Sword). Four Pillars locked (Port Authority, gRPC Transport, Log Aggregation, Service Discovery).

**Bare Metal Architecture**:
```
WEST (Forge) — High-Power Server         EAST (Outpost) — Consumer 4C/8GB
├── NixOS + flake.nix                     ├── NixOS + flake.nix
├── OPNsense/FRR — AS 65001              ├── IPFire/BIRD — AS 65002
├── All 26 services                       ├── Lightweight service subset
├── eBPF full stack                       ├── eBPF collectors only
├── Prometheus + Grafana                  ├── Node exporter + remote scrape
└── WireGuard fd00:dead:beef::/48 ────────└── WireGuard peer
    EVPN-VXLAN VNI 10001/10002/10100
```

**Technical Risks**:
1. eBPF programs compiled but NEVER tested on real kernel — WEST is first real validation
2. EAST has only 8GB DDR3 — memory budget is TIGHT, need service subset selection
3. WireGuard tunnel config not yet tested cross-host
4. EVPN-VXLAN fabric untested on real hardware

**Protocol Alignment**: Monad draft-05, Sophia draft-02, Wotan draft-02 — all spec'd. Wire format freeze (S67) MUST complete before first packet hits bare metal wire.

**Recommended ADRs**:
- ADR: EAST service subset selection (what runs on 8GB?)
- ADR: eBPF memory budget per program on constrained hardware
- ADR: Bare metal bootstrap sequence (NixOS → services → fabric → validation)

---

### The Anvil Reports (Developer — Implementation & Testing)

**Code Health**: GREEN. Build passes. Tests pass. 517K LOC.

**Implementation Blockers**:
1. eBPF real-kernel validation — compiled to ELF but never `bpf_prog_load()`'d on real hardware
2. WireGuard tunnel scripts exist but untested cross-host
3. Node_exporter version fix committed but unvalidated on metal

**TDD Status**: Red-green-refactor cycle healthy. 23/23 E2E tests green.

**Estimated Effort**:
| Item | T-Shirt | Hours |
|------|---------|-------|
| S67 RFC Overlap | M | 6-10 |
| WEST NixOS Deploy | L | 8-12 |
| EAST NixOS Deploy | M | 4-6 |
| S68 GUI Buildout | L | 8-14 |
| Security P0s (6) | M | 4-8 |
| WireGuard Tunnel | S | 2-3 |
| Observability Stack | M | 4-6 |

---

### The Hourglass Measures (Timeguru — Timeline & Milestones)

**Current Age**: Age 1: Alpha Ascension (~98%) → transitioning to Age 2: Beta Trials (~42%)

**Velocity**: 13.4K LOC/day (Feb 2-19). 40+ commits in last 7 days. BLAZING.

**ETA to Milestones**:
- Wire format freeze: 1-2 days (S67 critical path ~3.5hrs with parallelization)
- WEST fully operational: 2-3 days
- EAST joined to fabric: 3-5 days after WEST validated
- Age 1 → Age 2 transition: Upon bare metal validation (~1 week)

**Historical Pattern**: Sprint velocity is INCREASING. S73 crushed multiple P0s in single session. Bare metal will either maintain or briefly slow velocity (hardware debugging).

---

### The Sundial Tracks (Calendar — Schedule & Deadlines)

**Friday Feb 27 (TODAY)**:
- Round Table COMPLETE ← you are here
- S67 RFC Overlap: BEGIN (can run evening session)
- EAST hardware prep: Physical setup by Muck

**Saturday Feb 28**:
- S67 RFC Overlap: COMPLETE wire format freeze
- WEST: NixOS deployment BEGIN
- S68 GUI: Phase 0-2 (dashboard stubs → live viz)

**Sunday Mar 1**:
- WEST: eBPF validation, service bring-up
- WEST↔EAST: WireGuard tunnel bring-up
- S68 GUI: Phases 3-5 (doom CSS, kanban, wiki contrast)

**Monday Mar 2**:
- EAST: NixOS deploy + join fabric
- Security P0s: Burn remaining 6
- Observability: Prometheus + Grafana on metal

**Tuesday Mar 3**:
- Full fabric validation: Both machines, all services, eBPF tracing real packets
- Age 1 → Age 2 transition review
- Documentation ripple

---

### The Scroll Validates (Lore — Naming & Mythology)

**Naming Decisions Pending**: None critical. All protocol names RFC-canonical.

**WEST/EAST naming**:
- WEST = "The Forge" (where things are built, high-power)
- EAST = "The Outpost" (remote, lightweight, scouting)
- Already aligned with existing host-a/host-b architecture docs

**Mythology Consistency**: ✅ All 7 protocol names valid. Busboy → Wotan Ascension complete. No conflicts.

**Naming Pool Available**: Norse weapons, Wagnerian opera, Greek atmospheric — ready for new components as needed.

---

### The Map Confirms (Kingdom — Hierarchy & Placement)

**Hierarchy Health**: ALL tiers populated. 26 services across 4 tiers operational.

**New Components to Place**:
- Bare metal bootstrap scripts → Armory (infrastructure tier)
- EAST service subset config → Hollows (configuration tier)
- Cross-host WireGuard config → Moat (security perimeter)

**Tier Integrity**: ✅ No misplaced components detected.

**Armory Status**: Shield (eBPF), Pauldrons (firewall), Hauberk (routing), Sword (IDS) — all forged.

---

### The Goblet Toasts (Busboy — Alignment & Coordination)

**Cross-Skill Conflicts**: NONE. All 16 skills aligned on priorities.

**Coordination Needs**:
- Developer ↔ Architect: eBPF real-kernel validation requires both
- RFC Editor ↔ BlackMage: Wire format freeze must survive adversarial review
- Librarian ↔ All: Documentation ripple after bare metal validates

**Team Vibes**: 🔥 MAXIMUM. Bare metal coming online. 517K LOC shipped. Protocol moat dug. This is the moment.

**Translation**: "We have a working product. Now we prove it runs on real hardware. The wire format gets frozen first so we don't deploy something we have to break later."

---

### The Dark Mage Warns (BlackMage — Offensive Security)

**Attack Surface Change**: Bare metal DOUBLES the attack surface. Real NICs, real kernels, real network fabric.

**Priority Targets**:
1. eBPF programs on real kernel — verify verifier catches bad programs
2. WireGuard tunnel — key rotation, MTU handling, keepalive
3. Exposed services on Doom Range — real port scanning from EAST→WEST
4. XFF spoofing fix (S73) — validate on real reverse proxy

**Lich Status**: 9 campaigns planned, 2 active. Bare metal enables REAL adversarial testing.

---

### The Ghost Watches (Moat Ghost — Compliance & Threat Intel)

**Compliance Status**: HARDENING | THREAT LEVEL ELEVATED (bare metal exposure)

**6 Security P0s Remaining**: Must close BEFORE Age 2 transition.

**Bare Metal Risks**:
- Physical access = game over (encrypt at rest)
- Network exposure on real interfaces (firewall rules MUST deploy first)
- EAST consumer hardware = potential supply chain risk (verify firmware)

**Recommendation**: Deploy firewall config BEFORE services on both machines.

---

### The Scientist Reasons (Scientist — Theory & Analysis)

**Hypothesis**: EAST (4-core, 8GB DDR3) can run a meaningful subset of Unheaded services.

**Experiment Design**: Profile memory usage of each service in Docker on WEST first, then select EAST subset based on empirical data, not guesses.

**Protocol Reasoning**: Wire format freeze (S67) is mathematically correct — deployment creates path dependency. Once packets are on wire, breaking changes have non-zero migration cost. Freeze first.

---

### The Forgemaster Plans (Warmonger — Battle Plan Architecture)

**Existing Plans in Queue**:
- S67: 7 phases, 68 steps (RFC overlap)
- S68: 10 phases, 124 steps (GUI buildout)
- S72: 13 phases, 217 steps (partially executed)

**New Plan Needed**: S74 Bare Metal Bootstrap — estimated 80-120 steps covering NixOS deploy, service bring-up, fabric validation, eBPF real-kernel testing.

**Commit Cadence**: Every 4 steps (per S68 standard).

---

### The Marshal Enforces (Marshal — Execution Discipline)

**Standing Orders for S74**:
1. S67 RFC Overlap runs FIRST — wire format freeze is time-sensitive
2. NO GUI work until wire format is frozen (S68 waits for S67)
3. Bare metal work can PARALLEL with S67 (physical setup doesn't touch code)
4. Security P0s weave into gaps between major phases
5. Documentation ripple is LAST — after things stabilize

**Violation Watch**: Scope creep risk HIGH when bare metal comes online. Every hardware problem will tempt yak-shaving. STAY IN LANE.

---

### The Editor Reviews (RFC Editor — Protocol Specs)

**Protocol Status**:
- Monad: draft-05 (foundation spec)
- Sophia: draft-02 (dictionary spec)
- Wotan: draft-02 (memory spec)

**S67 Deliverables** (RFC Editor owns):
- IANA Considerations sections for all 3 specs
- 5 registration documents (Flags, Kingdom Mode, HbH Option, Flow Actions, Anamnesis Events)
- IPR clearance for RFC 8928/9927

**Wire Format Window**: OPEN. Closes on first bare metal deployment. S67 is the last chance for breaking changes.

---

### The Librarian Catalogs (Librarian — Document Web)

**Document Drift**: MODERATE. Recent sprints shipped code faster than docs updated.

**Priority Updates After S74**:
1. CLAUDE.md: Update LOC count (517K), add bare metal sections
2. README.md: Update architecture with WEST/EAST topology
3. wiki/: Sync with latest sprint deliverables
4. timeline.md: Add S74 entries
5. THIRD_PARTY.md: Verify after SBOM generation

**Wiki Health**: 66 wiki pages. Need sync pass after bare metal stabilizes.

---

## UNIFIED BATTLE PLAN

### PHASE 1: Wire Format Freeze (S67) — CRITICAL PATH
**Duration**: 6-10 hours | **Owner**: RFC Editor + Developer + BlackMage
**Parallel with**: EAST physical hardware prep (Muck)

- [ ] Execute S67 Phase 0: Intelligence gathering (Monad wire format inventory)
- [ ] Execute S67 Phase 1: IETF IPR database search (RFC 8928/9927 clearance)
- [ ] Execute S67 Phase 2: Monad flags collision analysis
- [ ] Execute S67 Phase 3: Breaking changes window (score all candidates)
- [ ] Execute S67 Phase 4: IANA Considerations draft
- [ ] Execute S67 Phase 5: I-D integration
- [ ] Execute S67 Phase 6: Documentation ripple
- [ ] Execute S67 Phase 7: Final verification
- [ ] **GATE**: Wire format FROZEN. No more breaking changes.

### PHASE 2: WEST Bare Metal Deployment — THE FORGE AWAKENS
**Duration**: 8-12 hours | **Owner**: Architect + Developer
**Prerequisite**: Phase 1 GATE passed (wire format frozen)

- [ ] NixOS flake deployment to WEST
- [ ] Firewall rules deployed FIRST (Moat Ghost requirement)
- [ ] FRR/OPNsense routing fabric: AS 65001 BGP
- [ ] Service bring-up: All 26 services on Doom Range
- [ ] eBPF program loading: Real kernel, real NICs, real packets
- [ ] Prometheus + Grafana: Scraping real metrics
- [ ] **GATE**: WEST healthy, all services responding, eBPF attached

### PHASE 3: EAST Bare Metal Deployment — THE OUTPOST RISES
**Duration**: 4-6 hours | **Owner**: Architect + Muck
**Prerequisite**: Phase 2 GATE passed (WEST validated)

- [ ] NixOS install on consumer hardware
- [ ] Memory profiling: Select service subset for 8GB budget
- [ ] IPFire/BIRD routing: AS 65002 BGP
- [ ] WireGuard tunnel: fd00:dead:beef::/48 EAST↔WEST
- [ ] EVPN-VXLAN fabric: VNIs 10001/10002/10100
- [ ] Node exporter + remote Prometheus scrape from WEST
- [ ] **GATE**: EAST joined to fabric, tunnel UP, metrics flowing

### PHASE 4: Security Hardening Sprint
**Duration**: 4-8 hours | **Owner**: BlackMage + Moat Ghost + Developer
**Parallel with**: Phase 3 or after

- [ ] Close 6 remaining security P0s
- [ ] Real-network penetration test: EAST→WEST port scan
- [ ] eBPF verifier validation on real kernel
- [ ] WireGuard key rotation test
- [ ] XFF spoofing fix validated on real reverse proxy
- [ ] **GATE**: All security P0s CLOSED. Lich campaigns can begin.

### PHASE 5: GUI Buildout (S68)
**Duration**: 8-14 hours | **Owner**: Developer
**Prerequisite**: Phase 1 GATE (wire format frozen)

- [ ] Execute S68 Phases 0-2: Dashboard stubs → live visualization
- [ ] Execute S68 Phase 3: Doom CSS text overlap fix
- [ ] Execute S68 Phase 4: Kanban drag-drop with Marshal lanes
- [ ] Execute S68 Phases 5-7: Wiki contrast + integration
- [ ] Execute S68 Phases 8-10: Testing + final gate
- [ ] **GATE**: All 4 dashboard tabs live, doom readable, kanban functional

### PHASE 6: Observability + CI/CD
**Duration**: 4-6 hours | **Owner**: Architect + Developer
**After**: Phase 2-3 (needs bare metal)

- [ ] Prometheus rules deployed to WEST
- [ ] Grafana dashboards: eBPF packet flow, service health, network fabric
- [ ] GitHub Actions: Hardened pipeline from S72
- [ ] Jenkins: Scaffold from S72 battle plan
- [ ] **GATE**: Dashboards showing real data from real metal

### PHASE 7: Documentation Ripple + Age Transition Review
**Duration**: 2-4 hours | **Owner**: Librarian + Timeguru
**After**: All other phases

- [ ] CLAUDE.md: Update to 517K LOC, bare metal architecture
- [ ] README.md: WEST/EAST topology
- [ ] timeline.md: S74 entries, milestone dates
- [ ] wiki/: Full sync pass (66 pages)
- [ ] THIRD_PARTY.md: Verify against SBOM
- [ ] Age 1 → Age 2 transition review
- [ ] **GATE**: All docs current. Age transition GO/NO-GO.

---

### Parallelization Matrix

```
TIME →  Fri Eve   |  Sat AM    |  Sat PM    |  Sun AM    |  Sun PM    |  Mon
────────────────────────────────────────────────────────────────────────────────
S67     ████████████████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
WEST    ░░░░░░░░░░░░░░░░░░░░░░░░████████████████████████████░░░░░░░░░░░░░░░░
EAST    ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░████████████░░░░░
SEC     ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░██████████████████████████░░░░░░
S68     ░░░░░░░░░░░░░░░░░░░░░░░░████████████████████████████████████████████░
OBS     ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░████████████░
DOCS    ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░████████░
PHYS    ████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
        ↑ Muck preps EAST hardware while S67 runs in parallel
```

### Decisions Made at This Round Table

1. **Wire format freeze BEFORE bare metal** — Deploying unfrozen wire format creates irreversible technical debt. S67 is Phase 1. Rationale: RFC Editor + Scientist unanimous.

2. **WEST deploys first, EAST follows** — Validate on powerful hardware before constrained hardware. Rationale: Architect + Developer unanimous.

3. **Firewall deploys BEFORE services** — Moat Ghost requirement. No exposed services without perimeter defense.

4. **S68 GUI work can parallel with bare metal** — GUI doesn't touch infrastructure code. Developer can context-switch.

5. **EAST runs service SUBSET** — 8GB DDR3 cannot run all 26 services. Scientist will profile on WEST first.

### Open Questions (Carry Forward)

1. **EAST service subset**: Which services? Need WEST profiling data first. — Scientist + Architect — By end of WEST deployment.
2. **eBPF memory budget on EAST**: How much kernel memory per program on 4-core/8GB? — Developer — During EAST prep.
3. **Age 2 transition criteria**: What EXACTLY constitutes "bare metal validated"? — Micromanager — Before Tuesday review.

---

### Next Round Table
**Scheduled**: After Phase 3 GATE (EAST joined to fabric) OR on blocker
**Reason**: Age 1 → Age 2 transition GO/NO-GO decision

---

_Forged at the Round Table by all 16 minds. The Kingdom marches as one._
_WEST awakens. EAST rises. The wire freezes. The packets flow._
_LET'S. GO. NUTS._ 🔥
