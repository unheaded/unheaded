# Battle Plan: State, Direction and Drift
## Convened: 2026-10-30 (night) | Reason: General alignment check (Stevie: "assess state of kingdom, direction, and drift")
## Kingdom State: Age 3 (Public Release) not reached; NEXT-7 Sprints 1-4 code complete, 5-6 mostly done, 7 not started

---

### Situation Report

**The code is ahead of everything else.** Sprints 1-4 of NEXT-7 are code complete. S-TRUE-WIRE has 353 of 411 steps ticked and S-ASCEND-2 has 270 of 314. S-LOW-FRUIT merged on 10-29, and its ten new CI gates have each been shown to catch failures. Every commit since 10-27 is GPG-signed. `go test` passes on the branch tip and the UPC harness is ALL GREEN.

**Nothing runs live, and nothing has been reviewed or pushed.**

| Signal | Value (2026-10-30 22:30 UTC) |
|---|---|
| Compose stack on west | 0 of 36 containers running. Stopped since the 10-28 reboot (haproxy since 10-27). Blocked on the "BEFORE ANY compose up" list (E1). |
| Monitoring / alerting | Alerts reach nobody (FND-001, due today) |
| Well backups | `unheaded-well-backup` failed on west; `unheaded-well-offsite-check` failed on east |
| Steel-echo canaries | 3 running in LXD project `unheaded` |
| Not on origin | 58 commits (develop ahead 33 + `churn/2026-10-30`). develop is also 4 behind origin. |
| develop → staging | 1,498 commits not yet promoted (1,342 two days ago). staging last moved 09-24; main 07-29. |
| CI on origin/develop | Daily security scan red; Lich nightly `fuzz_mbc_execute` red every night since 10-28; static analysis red. Fixes for gosec and shellcheck are on the unpushed branch. |
| Decision markers in next.md | 115 lines asking Stevie to decide, confirm or approve |
| Lab perimeter | No host firewall on west or east. LAN and east have bypassed Pi-hole since 08-22. Pi-hole is behind on published advisories. |

**The three findings:**
- **Throughput is not the constraint.** Stevie's decision and push bandwidth is.
- **The live lab is dark.** The product is not running anywhere, so code-complete sprints have not been proven in operation.
- **New plans keep starting while old ones stay open.** S-LOG-HARVEST added 214 more open steps today.

---

### The Throne Speaks (Captain: Vision & Strategy)
**Strategic Position**: The direction still holds:
- The product: a configuration management automation platform with drop-in compliance (ADR-097/098).
- The scaling substrate: Yggdrasil.
- The protocol (Monad, Sophia, Wotan) as the moat.

Nothing done this week contradicts that direction.

**North Star**: Age 3, public release, through the ADR-089 promotion path (develop → staging → main).

**Key Decision**: Stop forging new plans until the lab runs again and develop has been promoted once. The bottleneck is no longer new code.

**Risk to Vision**: The promotion path exists only on paper. Every day the develop→staging gap grows by about 80 commits, and Age 3 needs a reviewed `main`.

### The Ledger Records (Micromanager: Execution & QA)
**Sprint Status**: Sprints 1-4 are code complete, with open steps attended or waiting on Stevie: Iron Gate 72, Well Spring 70, Champion Hardening 22. True Wire has 58 open, Ascend 2 has 44. Doom Last (106) and Log Harvest (214) have not started.

**Priority Stack**:
1. P0: Push and merge (`churn/2026-10-30` → develop). This turns origin CI green on gosec and shellcheck.
2. P0: Bring the stack up (E1). This one action unblocks FND-001, Well backups, Grafana, the S-LOG-HARVEST pipeline and every "code complete but not live" sprint.
3. P0: Apply the host firewall on west and east, with Stevie present (`firewall-inbound-deny`, due today).
4. P1: Fix the Lich nightly crash. Download the artifact, triage it, and add a corpus regression.
5. P1: First staging promotion batch, one sprint per batch, at phase-merge review grain (Round Table 10-28).
6. P1: One sitting for the third-party discussion. It clears seven held items in one go: the inventory verdicts, trivy, yaml.v3, Grimoire with its Dependabot criticals, llama.cpp, artifacts and ruff.
7. P2: Finish S-TRUE-WIRE (58) and S-ASCEND-2 (44) before starting S-LOG-HARVEST.
8. P3: S-DOOM-LAST stays last.

**QA Gates**: A sprint counts as done when it is running on west and its alerts fire into a monitored stack, not when it is "code complete".

**Acceptance Criteria**:
- origin/develop CI green;
- the stack up, with Grafana showing every service;
- a Well backup succeeding on west and the offsite check passing on east;
- staging moved past 09-24.

### The Blueprint Reveals (Architect: Infrastructure & Design)
**Architecture Health**: Sound on paper, but unproven in operation since the Iron Gate TLS and service-identity changes. The installed units now want `/etc/unheaded/tls/ca.pem`, the chaos-controller key and `trust.d`, and none of them exist on west.

**Technical Risks**:
- When the stack comes up, Iron Gate (TLS on every internal link), Well Spring and Champion Hardening all meet real traffic for the first time.
- Expect a burst of integration faults, and budget an attended session for them.

**Protocol Alignment**:
- S-TRUE-WIRE E2 is decided (spec wins on the renumber, merged `a57c6d3c`).
- WAL replication verifies ML-DSA-65 signatures (FND-020 resolved).
- The ADR-098 evidence regeneration is held, because it reads local evidence.

**Recommended ADRs**: None new. Close out ADR-022: DNSSEC is now on, and LAN use of Pi-hole is a router decision.

### The Anvil Reports (Developer: Implementation & Testing)
**Code Health**: On the branch tip: go test 240 ok / 0 FAIL, UPC harness ALL GREEN, and every static-analysis job reproduced locally passes.

**Implementation Blockers**:
- Lich nightly `fuzz_mbc_execute`: the crash artifact needs a GitHub login.
- ruff bump held by the third-party pause.
- FND-030 yaml.v3 swap held by the same pause.

**TDD Status**: Healthy. New gates are registered in `check-gates-can-fail.sh`.

**Estimated Effort**:

| Item | Size |
|---|---|
| Push + merge | XS (Stevie) |
| E1 stack up + integration fallout | L, attended |
| Firewall | S, attended |
| Lich crash | S-M |
| Staging batch 1 | M |

### The Hourglass Measures (Timeguru: Timeline & Milestones)
**Current Age**: Age 2 (Beta) done; Age 3 (Public Release) blocked on promotion and pre-public blockers (`docs/security/PRE-PUBLIC-BLOCKERS.md`).

**Velocity**: Engineering output is high. Delivery output (things running and promoted) has been flat since 09-24.

**ETA to Next Milestone**: First staging promotion within a week, if E1 and the push happen this weekend. Confidence: medium, because it depends on Stevie's time.

**Historical Pattern**: Every unattended churn has ended with a longer escalation list than it started with.

### The Sundial Tracks (Calendar: Schedule & Deadlines)
**This Week**:
- Fri/Sat: push + merge; E1 stack up; firewall (attended).
- Sun: Lich crash; third-party sitting.
- Mon onward: staging batch 1.

**Deadlines**:
- FND-001 and `firewall-inbound-deny` are due today (10-30) and will be overdue tomorrow.
- Pi-hole update to clear the advisories.

**Schedule Conflicts**: All attended items compete for the same evening hours. Order them by how much each unblocks (above).

**Calendar Health**: Needs attention. Two items will be overdue tomorrow.

### The Scroll Validates (Lore: Naming & Mythology)
Nothing to report.

**Naming Decisions Pending**: Q5 naming ceremony (kernel) is still parked. Nothing new tonight. Huginn, Muninn, Heimdall, Gungnir and Steel-echo are all used consistently.

### The Map Confirms (Kingdom: Hierarchy & Placement)
**Hierarchy Health**: Unchanged.

**New Components**: S-LOG-HARVEST proposes `cmd/syslog-relay`, `cmd/logcov` and `pkg/logsources`. They belong in the Anamnesis/Muninn tier (ADR-086/092); nothing is built yet.

**Tier Integrity**: Pi-hole sits in LXD project `default`, outside Cuirass's `unheaded` project (per ADR-022). That is fine, but it stays outside Unheaded's own inventory.

### The Goblet Toasts (Busboy: Alignment & Coordination)
**Cross-Skill Conflicts**: Warmonger keeps forging plans while Marshal's backlog grows. Captain and Micromanager agree: consolidate before expanding.

**Coordination Needs**: E1 needs Architect, Erda (Well) and MoatGhost (FND-001) in one attended session.

**Team Vibes**: Good momentum. The real risk is Stevie drowning in the decision queue.

**Translation Needed**: Shrink the 115 decision markers in next.md into one ordered list of about 10, with one line each.

### Extended Seats
- **Marshal (drift)**: Three lanes have drifted.
  - Plan proliferation: 8 plans open, 1 forged today.
  - Meta-work over delivery: the last three days were mostly gates, man pages and hardening, with nothing deployed.
  - Lab defense: the IOC sweep found the lab without firewalls while the product sells defense.
  - No citations against scope creep inside sprints.
- **BlackMage**: Lich nightly red is the one live signal of a possible bug in the MBC executor. Treat it as P1, not noise. The IOC sweep was clean.
- **MoatGhost**: Compliance evidence can't be produced while the stack is down. ADR-098 regeneration is held. Expect FND-001 and the firewall finding to be overdue tomorrow.
- **Computermancer**: EVOLUTION-1 Stage 2 hostile-child probes are built and gated (D6). Mode A works from netns upc0, and LICH-015 converged (550k programs, 0 divergences). The privileged full-engine arm waits on Stevie's go-ahead on east.
- **Librarian**: next.md is over 1,800 lines with stacked START HERE sections. It needs one live START HERE and an archive.
- **Barrister**: The third-party pause is the right call. It needs its discussion sitting so it doesn't become a permanent freeze.
- **RFC Editor**: E2 is settled. The RFC desk inherits the deferred Enkrateia restore topic (S-ASCEND-2 272-276).
- **Scientist**: No open proofs are blocking. The DNS data showed one clear causal break (08-22, router side).

---

### Unified Battle Plan

#### Immediate Actions (Next 24-48 Hours)
- [ ] Push `develop` (force-with-lease, re-signed laptop commits) and merge `churn/2026-10-30`. Owner: Stevie.
- [ ] E1: work the "BEFORE ANY compose up" list and bring the stack up. Attended; Owner: Architect + Erda + Stevie.
- [ ] Apply `deploy/nftables/{west,east}.nft` attended, with a timed rollback. Owner: Sentinel + Stevie.
- [ ] `pihole -up`, and point the router's DHCP DNS at Pi-hole. Owner: Stevie (router).
- [ ] Freeze new battle plans until the three items above are done. Owner: Marshal.

#### This Sprint (to 2026-11-06)
- [ ] Lich nightly crash: download the artifact, reproduce, fix, add a corpus regression. Owner: BlackMage/Developer. Due 11-02.
- [ ] Third-party discussion sitting, clearing 7 held items. Owner: Barrister + Stevie. Due 11-02.
- [ ] Staging promotion batch 1 (Sprint 1, Clean Slate) at phase-merge review grain. Owner: Micromanager. Due 11-04.
- [ ] Finish S-TRUE-WIRE open steps. Owner: Developer. Due 11-06.
- [ ] Rewrite next.md as one START HERE plus an archive, with decisions shrunk to an ordered list of about 10. Owner: Librarian. Due 11-01.
- [ ] Once the stack is up: decide S-LOG-HARVEST D1-D7, then execute. Owner: Warmonger. Due 11-06.

#### Protocol Milestones
- [ ] Monad: S-TRUE-WIRE wire rulings done. Shield data-plane truth steps finished on the branch. Owner: Developer. Due 11-06.
- [ ] Sophia: no change this cycle.
- [ ] Wotan: signed WAL replication goes live when the stack comes up. Verify it on west. Owner: Architect. Due 11-03.

#### UPC Compute Milestones
- [ ] Dream Ladder: S-ASCEND-2 open steps (44), including the privileged LICH-015 arm on east. Owner: Computermancer, with Stevie's go-ahead. Due 11-06.
- [ ] MBC: Lich nightly crash (see above).
- [ ] Shim: no change.

#### Decisions Proposed at This Round Table (Stevie decides)
1. **DECIDED 2026-10-30 (Stevie): plan freeze, ADR-112.** No new plans; every pitch becomes a parked `Proposed` ADR. Rationale: delivery is flat while code output is high.
2. "Done" means running live with alerts, not code complete.
3. Unattended churn may only work items that don't add escalations.

#### Open Questions (carry forward)
1. What changed on the router on 08-20/22, and should the LAN go back to Pi-hole? (Stevie)
2. Is 192.168.69.132 your laptop, and did you sideload east's core22/snapd snaps on 03-19? (Stevie)
3. `userdel lxd` on east? (Stevie)

#### Wins to Celebrate
- S-LOW-FRUIT merged: 10 gates that provably catch failures, 40 man pages, version and memory limits everywhere.
- Heimdall's silent ringbuf drops are now counted, alerted on and covered by a runbook (LICH-012 L12d).
- Gungnir seed-file race closed (L12a).
- `reqwest` removed from upc-bootctl, and Mode A works from netns upc0.
- LICH-015: 550,000 programs, 0 divergences.
- IOC sweep: no IOCs on Pi-hole, west or east. DNSSEC is validating on Pi-hole.
- Every commit is signed.

---

### Next Round Table
**Scheduled**: When the stack is up and staging has moved, or 2026-11-06, whichever comes first.
**Reason**: Check whether consolidation turned code-complete into running and promoted.

---
_Forged at the Round Table by the full council. The Round Table advises; Stevie decides._
