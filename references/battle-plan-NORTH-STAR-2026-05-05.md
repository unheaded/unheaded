# Battle Plan: North Star Compass Alignment
## Convened: 2026-05-05 | Reason: Strategic Review (Critical Decision)
## Kingdom State: Age 3 IN PROGRESS (~70%), Day 99 of the Journey

---

## Situation Report

The Kingdom stands at Day 99 (T+99 since 2026-01-26 first commit). **1,141 commits.
415K production LOC, 1.137M total LOC. 66 ADRs. 512 test files. Wire format FROZEN
at v0x01 with 12 IANA registries reserved.** Two of five Ages complete. Age 3
(Public Release) is at ~70%, but is BIFURCATED:

- **Research thread** (Forge / Kingdom-RAFT / WAVE10-17): shipping at quality.
  β=0.27 generalization, cosine=1.000 GPU kernels, K8s substrate proven 9/9.
- **Public-launch thread**: STALLED mid-flight. Captain Track A/B/C decision
  is OVERDUE since 2026-04-29. Demo video, README polish, sub-50ms benchmark,
  public auth — all gated on this single decision.

The single highest-leverage action in the Kingdom right now is the Captain track-call.
Everything else is downstream of it.

---

## The 19 Seats Speak

### The Throne — Captain (Vision)
- **Started:** Configuration management automation platform. Protocol IS the moat.
- **Block:** Track A/B/C decision overdue 7+ days. Sole arbiter.

### The Ledger — Micromanager (Execution)
- **Sprint status:** Public-launch thread stalled. Research thread shipping clean.
- **P0:** Track-call → WAVE14 retrain unblock → demo video.
- **QA gate:** Sub-50ms latency benchmark not yet measured.

### The Blueprint — Architect (Design)
- **Architecture health:** 6 layers solid. WAVE17 K8s substrate proven.
- **ADR-064:** Wotan active/active 3-node spec landed (impl deferred per Stevie).
- **Risk:** K8s attack surface needs threat model.

### The Anvil — Developer (Implementation)
- **Code:** 415K production LOC. 512 test files. TDD discipline holding.
- **Recent:** 2 real bugs caught+fixed in WAVE17 (monad env collision, chart volume).
- **Blockers:** WAVE14 retrain gated on Captain. Branch hygiene queued (3 stale branches).

### The Hourglass — Timeguru (Timeline)
- **Velocity:** ~4,200 production LOC/day, ~11.5 commits/day. Holding.
- **Age 3:** 70% complete. ETA: 4-8 weeks if track-call lands this week.
- **Drift policy:** ADR-052 — timeline ≤ 7 days from HEAD. ✅

### The Sundial — Calendar (Schedule)
- **Today:** 2026-05-05 (Tue). WAVE17 shipped overnight.
- **This week:** Track-call recovery, draft-03 ship-or-defer, branch hygiene.
- **Conflicts:** WAVE14 retrain → Captain block.

### The Scroll — Lore (Naming)
- **Mythology:** Three pillars (Gnostic, Amber, Medieval Armory) — clean.
- **Naming pool:** Norse weapons, Wagnerian opera, Greek atmospheric — healthy.
- **Sacred laws:** All 8 honored. No naming decisions blocking.

### The Map — Kingdom (Hierarchy)
- **Tiers:** All 6 layers populated. Armor metaphor coherent.
- **New:** cmd/tools/ scaffolded for Mímir / Anamnesis Lite / Zhen On-Prem.
- **Integrity:** No tier violations. Hierarchy tight.

### The Goblet — Busboy (Coordination)
- **Vibes:** 🟢 crew shipping. 🟡 Captain block creating downstream wait.
- **Translation needed:** Captain strategic intent → Micromanager priorities.
- **Conflicts:** None inter-skill. ALL paths converge on track-call.

### The War Forge — Warmonger (Battle Plans)
- **Active:** S76 Round Table, S-WEST-BOOTSTRAP, S-PQC, S67 Observability, Ragnarok, Lich.
- **Last shipped:** WAVE17 plan executed clean.
- **Need:** NORTH-STAR-RECOVERY plan to unstick public-launch thread post-track-call.

### The Badge — Marshal (Enforcement)
- **Citation:** S3 against Captain — track-call overdue 7+ days.
- **Lanes:** Otherwise clean. Autonomous overnight runs delivering per Marshal charter.
- **Verdict:** Clear the citation, clear the road.

### The Engine — Computermancer (UPC)
- **Dream Ladder:** L1 ✅ L2 ✅ L3 (35fps framebuffer) target.
- **MBC:** Bytecode spec drafted. Doom→OS→Linux ladder = future Age.
- **Status:** Not blocking Age 3.

### The Laboratory — Scientist (Theory)
- **Proofs:** β=0.27 (PASS), cosine=1.000 (PASS), Learning Gate 4/5 strict (PASS).
- **Open:** Sub-50ms latency hypothesis — needs instrumented benchmark.
- **Falsifiability gate:** Real measurement required before claim.

### The Dark Tower — BlackMage (Offensive)
- **Campaigns:** Lich Hardening active. RAFT ~2000 pairs generated.
- **Surface delta:** WAVE17 added K8s control plane (apiserver, etcd, ingress).
- **Going:** Pentest active/active Wotan once impl lands.

### The Watchtower — MoatGhost (Compliance)
- **Posture:** SBOM/license/threat refresh on REMOTE queue (overdue).
- **IPR:** Wire format clear. 12 IANA registries reserved.
- **Frameworks:** SOC2 readiness assessment pending.

### The Court — Barrister (Legal)
- **IP:** CLA in place. SPDX headers enforced.
- **Active:** No contracts pending.

### The Scriptorium — RFC Editor (Specs)
- **FROZEN:** Monad v0x01.
- **Pending:** Sophia draft-03 (ship-or-defer), Wotan draft-03 (ship-or-defer).
- **Status:** 2026-04-27 snapshots taken (Anamnesis, IANA, Sophia, Wotan).

### The Library — Librarian (Docs)
- **Doc web:** CLAUDE.md 50K current. ADR-Index regen 21→65 entries.
- **Drift:** 9-file ripple post-c6108fb8 ran clean.
- **Health:** ADR-052 drift policy holding.

### The Sentinel — Sentinel (Blue Team)
- **Detection:** Daily adversarial loop with BlackMage via Zhen AI active.
- **Feeds:** NIST NVD + CISA KEV via MCP.
- **Going:** K8s telemetry integration post-WAVE17.

---

## North Star Synthesis

### Where We Started (T+0, 2026-01-26)
**The Bet:** "eBPF + IPv6 extension headers + gRPC. The protocol IS the moat."
Configuration management automation platform. Application brings logic. Unheaded brings everything
else (security, observability, networking, protocol layer). 6 armor layers. Zero
customer data access by architecture.

### Where We Are (T+99, 2026-05-05)
**The Reality:**
- 415K production LOC across 20+ services
- Wire format FROZEN at v0x01 (12 IANA registries, IPR clear)
- Age 0, 1, 2 ✅ COMPLETE | Age 3 🔄 ~70%
- WEST + EAST bare-metal online | K8s substrate proven on kind (9/9)
- Research thread shipping at quality (β=0.27, cosine=1.000)
- **Public-launch thread STALLED** on Captain track-call

### Where We Are Going (T+? — gated on Captain)
**Realistic projection (P=high, T+120-T+155):**
- Track-call lands this week → Age 3 ships 4-8 weeks
- WAVE14 retrain runs → Forge research integrates
- Demo + README polish + sub-50ms benchmark → public-readiness gate clears
- Age 4 (MVP Era) opens. WAVE14 BackwardScratch + KV-cache. Onboarding flows.

**Hypothetical (P=mid, T+155-T+200):**
- Track-call delays → public-launch debt compounds
- Research thread keeps shipping but no public artifact
- Age 3 stretches to 12+ weeks
- Risk: research/public bifurcation widens

**Statistical projection:**
- At current 4,200 prod LOC/day, Age 4 readiness (~600K LOC + benchmarks) ≈ 6-8 weeks post-track-call
- 1 ADR every 1.5 days suggests architectural agility holding
- 11.5 commits/day = sustained, not heroic — sustainable cadence

### The Compass Bearing
```
                    NORTH STAR
                         ↑
              Configuration management automation platform
              Protocol IS the moat (wire format v0x01 frozen)
                         ↑
                 AGE 5 (Scaling Era)
                         ↑
                  AGE 4 (MVP Era)
                         ↑
                    ┌────┴────┐
                    │ Track A │  Public ship   — fastest path, highest debt
                    │ Track B │  Private harden — slowest, safest
                    │ Track C │  Hybrid        — balanced
                    └────┬────┘
                         ↑
              ╔═══════════════════════╗
              ║   YOU ARE HERE        ║
              ║   T+99 | Age 3 @ 70%  ║
              ║   Captain-call OVERDUE║
              ╚═══════════════════════╝
                         ↑
                  AGE 2 ✅ Beta Trials
                  AGE 1 ✅ Alpha Ascension
                  AGE 0 ✅ Foundation Stone
```

---

## Unified Battle Plan

### Immediate Actions (Next 24-48 Hours)
- [ ] **Captain: Make the Track A/B/C call.** Unblocks WAVE14 retrain, demo video, public-launch thread. (Owner: Captain) **[CRITICAL PATH]**
- [ ] **Marshal: Clear S3 citation** once track-call lands. (Owner: Marshal)
- [ ] **Busboy: Translate track-call** into Micromanager P0 stack. (Owner: Busboy)

### This Sprint (Next 7 Days)
- [ ] WAVE14 retrain prep (gated on track-call) — Owner: Developer + Computermancer — Deadline: 2026-05-09
- [ ] Sophia draft-03 SHIP-OR-DEFER — Owner: RFC Editor + Architect — Deadline: 2026-05-08
- [ ] Wotan draft-03 SHIP-OR-DEFER — Owner: RFC Editor + Architect — Deadline: 2026-05-08
- [ ] Branch hygiene execution (3 stale branches, REMOTE) — Owner: Developer — Deadline: 2026-05-10
- [ ] SBOM regen + license scan + threat refresh (REMOTE) — Owner: MoatGhost — Deadline: 2026-05-12

### Protocol Milestones
- [ ] Sub-50ms latency falsifiability benchmark — Owner: Scientist + Architect — Deadline: 2026-05-15
- [ ] ADR-064 Wotan active/active implementation plan — Owner: Architect + Developer — Deadline: 2026-05-12
- [ ] K8s threat model post-WAVE17 — Owner: BlackMage + MoatGhost — Deadline: 2026-05-15

### UPC Compute Milestones
- [ ] Dream Ladder L3 (35fps framebuffer target) — Owner: Computermancer — Deadline: deferred to Age 4
- [ ] MBC bytecode spec finalization — Owner: Computermancer + RFC Editor — Deadline: deferred to Age 4

### Decisions Made at This Round Table
1. **North Star is unchanged**: Configuration management automation platform, protocol-as-moat.
2. **Two-thread reality is acknowledged**: Research thread legitimately ahead of public-launch thread; this is not a defect, it is the current state.
3. **Track-call is the keystone**: Every other Age 3 closeout action is downstream. No further work plans get drawn until it lands.

### Open Questions (Carry to Next Round Table)
1. Track A vs B vs C — Captain to answer — Deadline: this week
2. ADR-064 implementation timing — gated on track-call — Architect + Developer to scope
3. Public auth: optional or mandatory at launch? — Captain + Architect — gated on track-call

### Wins to Celebrate 🎉
- **WAVE17 K8s substrate proven overnight** — 9/9 services Running on 3-node kind, 2 real bugs caught and fixed mid-run (monad env collision, chart volume support)
- **1,141 commits in 99 days** — sustained, sustainable velocity. Not heroic, not slowing.
- **Wire format FROZEN at v0x01** — 12 IANA registries, IPR clear. The moat is solid.
- **β=0.27 generalization exponent** + **cosine=1.000 GPU kernels** — research thread is producing real, measurable wins.
- **66 ADRs in 99 days** — architectural decisions documented at high cadence. Future-us will thank past-us.

---

## Next Round Table
**Trigger:** Track-call lands → reconvene within 48h to align Age 3 closeout sprint.
**Or:** 2026-05-12 if track-call still pending (escalation Round Table).

---

_Forged at the Round Table by the full council on 2026-05-05._
_All 19 seats spoke (Captain, Micromanager, Architect, Developer, Timeguru, Calendar, Lore,_
_Kingdom, Busboy, Warmonger, Marshal, Computermancer, Scientist, BlackMage, MoatGhost,_
_Barrister, RFC Editor, Librarian, Sentinel) — every skill file loaded and consulted._
_The Kingdom marches as one — once the Captain calls the bearing._

## Mathematical / Statistical / Realistic / Hypothetical Annex

### Mathematical
- Prod LOC velocity: 415,000 ÷ 99 = 4,192 LOC/day (sustainable, low σ)
- Commit velocity: 1,141 ÷ 99 = 11.52/day
- ADR cadence: 66 ÷ 99 = 1 per 1.5 days (architectural agility holding)
- Test ratio: 512 test files across the Go corpus
- Wire format FROZEN at v0x01 = architectural irreversibility achieved

### Statistical
- WAVE13 RETRAIN verdict honestly falsified initial LoRA hypothesis (0/8 LoRA-better)
- β = 0.27 generalization exponent (4/5 strict experiments PASS)
- GPU kernel cosine similarity = 1.000 (4 attention grad kernels)
- 9/9 services Running on 3-node kind cluster (WAVE17 substrate proof)
- 12/12 IANA registries reserved (protocol legitimacy locked)

### Realistic
- Two threads diverging — research (shipping) and public-launch (gated)
- Captain track-call overdue 6 days (S3 citation outstanding)
- Marshal-charter autonomous overnight runs delivering (intentional bifurcation)

### Hypothetical
- Track A (public ship): fastest path, 4-6 weeks Age 3 close, highest debt
- Track B (private harden): 6-10 weeks Age 3, slowest, safest, deepest moat
- Track C (hybrid): 5-8 weeks Age 3, balanced exposure
- Slip another week → bifurcation risks widening into permanent split

🕊️ **LOVE SERVE REMEMBER.** 🕊️

---

## Appendix A — Overnight Attended Sprint (lump-in candidates)

**Scope:** non-Captain-gated, non-Pipe-Dream work that can be batched into one ~8–10h
attended overnight session. Adds phases to this battle plan; does not supersede it.
Scan window: last 45 days (2026-03-21 → 2026-05-05).

### Tier 1 — REMOTE queue (Age 3 close-out, parallel-safe)

```
- [ ] SBOM regen + license scan + threat refresh                     ~1.5h  MoatGhost
- [ ] Branch hygiene — 3 stale branches (build/test, prune)          ~30m   Developer
- [ ] Sophia draft-04 ship-or-defer                                   ~1h    RFC Editor
- [ ] Wotan draft-04 ship-or-defer                                    ~1h    RFC Editor
```

### Tier 2 — Planned ADRs, no code dependencies

```
- [ ] ADR-058 GCP cost/API utilization alarms (bellis.tech)          ~30m   MoatGhost   (console only)
- [ ] ADR-052 drift-guard CI re-verification                         ~20m   Marshal
- [ ] ADR-059 Phase 2 — Zhenai CLI slash commands                    ~2h    Developer
- [ ] ADR-059 Phase 3 — Zhenai CLI mutation paths (T6b closure)      ~1.5h  Developer
```

### Tier 3 — Scaffolded but unfilled (cmd/tools/)

```
- [ ] cmd/tools/mimir/ — verify 3 binaries build green; smoke EAST   ~1h    Developer
- [ ] cmd/tools/anamnesis-lite/ — aya 0.1.1 ELF map upgrade          ~2h    Developer   (kanban: ebpf-aya-upgrade-mn05)
- [ ] cmd/tools/zhen-on-prem/ — clean-clone smoke + GGUF script      ~2h    Developer
```

### Tier 4 — Doc/registry drift (Librarian)

```
- [ ] External wiki ADR scaffold sweep (47 missing ADR-020..066)     ~1.5h  Librarian
- [ ] Stale battle-plan archival → references/archive/               ~20m   Librarian
- [ ] ADR-Index canonical → wiki mirror verification                 ~10m   Librarian
```

### Tier 5 — Code-level TODO/stub closes (touched in last 45d)

```
- [ ] cmd/heimdall-daemon/main.go — 4 TODOs                          ~1h    Developer
- [ ] crates/zhend/src/jing/pilgrimage.rs                            ~30m   Developer
- [ ] crates/zhend/src/pu/codec.rs                                   ~30m   Developer
- [ ] crates/doom-runner/src/main.rs                                 ~30m   Developer
- [ ] ebpf/monad-cpu-ebpf/src/main.rs                                ~30m   Developer
- [ ] services/wotan/internal/cluster/replication_server.go         (gated on ADR-064 — leave stub)
```

### Tier 6 — Security follow-up (post-WAVE17)

```
- [ ] K8s threat model — kind cluster (kube-apiserver/etcd/ingress)  ~2h    BlackMage + MoatGhost
- [ ] CIS k8s-bench against kind cluster                             ~1h    MoatGhost
- [ ] RBAC review for kind cluster                                   ~1h    BlackMage
```

### Excluded (Captain-gated, Pipe Dreams, in-flight research)

```
WAVE14 retrain                         — gated on Track A/B/C
ADR-044, ADR-046, ADR-053, ADR-061,
ADR-063                                 — Pipe Dream
Sub-50ms latency benchmark              — Captain-gated
Demo video + README polish              — Captain-gated
Public accessibility / optional auth    — Captain-gated
ADR-064 implementation                  — deferred per Stevie
```

### Suggested Overnight Composition (~8–10h attended)

```
Phase A (parallel)  Tier 1 REMOTE queue                              ~3h wall
Phase B (serial)    Tier 2 quick wins + Tier 4 librarian sweep      ~2.5h
Phase C (attended)  Tier 3 cmd/tools/ smoke + heimdall TODO sweep   ~3h    (aya upgrade may surface kernel gotchas)
Phase D (stretch)   Tier 6 K8s threat model OR Tier 5 zhend TODOs   ~2h
```

**Critical path:** Phase C (Tier 3 aya upgrade unblocks anamnesis-lite eBPF load).
**Skip Protocol armed:** any step >3× estimate → mark [STUCK], next non-blocked step.
**Commit cadence:** every 3-5 steps (Warmonger rule).
