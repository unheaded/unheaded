# Battle Plan: S51-S60 — The Alpha Gate Campaign
## Convened: 2026-02-25 | Reason: Full Round Table + Warmonger 10-Session Sprint Forge
## Kingdom State: Age 1 (Alpha), Epoch 5 (Convergence → Gate), ~99% Structural, S50 COMPLETE

---

## The Origin Myth — Continued

**February 25, 2026** — Fifty sessions deep. 260K production LOC. 464K with tests. 32 commits across S45-S50 merged to main. DOOM validated with canonical CpuState, binary keyboard protocol, and KeyStateBitmap. Protocol API scaffolded with 7 cross-service integration tests. AI model stack entries allocated (ports 20100-20106). Dashboard running 8 tabs. All four infrastructure pillars standing since S36.

**The Kingdom has built the castle. Now the castle must prove it can defend.**

Ten sessions remain before the Alpha Gate opens. Each session is a named battle in a campaign that transforms an engineering prototype into something a skeptical staff engineer says "holy shit" to. The campaign name: **The Alpha Gate**.

**50 sessions from first commit. One engineer. One AI. One Kingdom. The gate is in sight.**

---

## LEGEND

```
[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
[C] = Commit checkpoint
[STUCK] = Step skipped via Skip Protocol
[BLOCKED] = Step blocked by upstream STUCK step
```

**Commit Cadence**: Every 4 steps (calculated: max(3, min(5, ~250/20)))
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts.
**Exit Gate Convention**: Every session ends with a gate. If the gate fails, DO NOT proceed to next session.

---

## ROUND TABLE — ALL 9 SEATS REPORT

### The Throne Speaks (Captain — Vision & Strategy)

**Strategic Position**: Pre-public. Private repo. PoC proven internally. S45-S50 closed the UI + protocol + AI scaffolding gaps. The castle looks like a castle now.

**North Star**: Ship an alpha demo where: (1) eBPF traces real packets on bare metal, (2) DOOM plays in-browser over IPv6, (3) AI model runs as first "head" inside armor, (4) dashboard shows it all — professional, dark, medieval, polished. A skeptical staff engineer watches for 5 minutes and says "holy shit."

**Key Decision**: The Alpha Gate Campaign is a 10-session sequential dependency chain. Each session feeds the next. No parallelization between sessions — within each session, phases can parallelize.

**Risk to Vision**: Hardware dependencies (bare metal server, GPU) could slip. Mitigation: sessions that don't require hardware come first (S51-S52). Hardware sessions (S53-S58) have fallback plans. Polish sessions (S59-S60) can proceed with mock data if hardware isn't ready.

### The Ledger Records (Micromanager — Execution & QA)

**Sprint Status**: S50 COMPLETE. 32 commits across S45-S50. All merged to main.

**Priority Stack** (the 10-session campaign):

1. **P0 — S51: Security Hardening** — Close 6 remaining P0s. SBOM generation. Pre-public gate.
2. **P0 — S52: Legal & Licensing** — LICENSE file, IP audit, THIRD_PARTY.md completion, contributor framework.
3. **P0 — S53: Bare Metal Foundation** — NixOS install on 4-core DDR3. Unheaded daemon deployment.
4. **P0 — S54: eBPF Awakening** — Compile + verify eBPF programs on bare metal. First real XDP attachment.
5. **P0 — S55: First Real Trace** — Packet marker → flow tracker → trace collector → Wotan → dashboard. The product moment.
6. **P1 — S56: DOOM on Bare Metal** — 6-namespace packet ring, doom.mbc loaded, frames in browser via WebSocket.
7. **P1 — S57: Sophia's Eye Awakens** — vLLM + DeepSeek-R1 on gaming desktop. First "head" in armor. eBPF-traced inference.
8. **P1 — S58: The Bridge** — EVPN-VXLAN tunnel between gaming desktop and bare metal. Wotan bridges both. Split-brain architecture live.
9. **P0 — S59: The Polish** — Dashboard overhaul, demo data generators, Kanban drag-drop, all 8 tabs perfect.
10. **P0 — S60: The Gate Opens** — Alpha demo video, public landing page, conference talk outline, README for humans.

**QA Gates per session**: Defined in each session's EXIT GATE below.

**Acceptance Criteria (Campaign-Level)**: A human who has never seen Unheaded navigates to www.unheaded.org, clicks through to the dashboard, sees real packet traces, watches DOOM frames, queries the AI model, and thinks "this is a real product built by a real team."

### The Blueprint Reveals (Architect — Infrastructure & Design)

**Architecture Health**: 25 services across 6 layers. All wired to Wotan. gRPC-first transport. Port Authority enforced. Log aggregation operational. Service discovery active. No architectural debt blocking the campaign.

**Hardware Requirements**:
- **Bare metal server** (4-core DDR3): NixOS, Unheaded daemon, Wotan, all core services, eBPF programs
- **Gaming desktop** (Ryzen 5 7600X, RX 7700 XT 12GB, 16GB DDR5): vLLM, AI models, GPU inference
- **Network**: EVPN-VXLAN tunnel between both hosts over existing LAN

**Technical Risks**:
- eBPF verifier rejection on production programs (mitigated: Doom eBPF already verified)
- ROCm driver compatibility with vLLM on RX 7700 XT (mitigated: research + fallback to llama.cpp)
- EVPN-VXLAN requires FRRouting or similar (mitigated: can use simple VXLAN + static routes as fallback)
- 6 security P0s must close before any public exposure

### The Anvil Reports (Developer — Implementation & Testing)

**Code Health**: 158+ packages pass, 0 race conditions, 161 test functions, 7 integration tests. Build GREEN.

**Pre-Existing Issues to Fix in S51**:
- 6 security P0s: Nix deps audit, gosec findings, SBOM generation, MaxHeaderBytes on all HTTP servers, release signing, auth framework
- TestIsOpen in internal/bpfmap (pre-existing, unrelated)
- Sophia struct size discrepancy (74 vs 80 bytes — needs investigation)

**Implementation Effort (Campaign Total)**:
| Session | Est. LOC | Est. Duration |
|---------|----------|---------------|
| S51 Security | 800 | 4-6 hours |
| S52 Legal | 400 | 3-4 hours |
| S53 Bare Metal | 600 | 6-8 hours |
| S54 eBPF | 1,200 | 6-8 hours |
| S55 First Trace | 1,500 | 8-10 hours |
| S56 DOOM Bare Metal | 800 | 6-8 hours |
| S57 AI Stack | 1,000 | 6-8 hours |
| S58 EVPN-VXLAN | 800 | 6-8 hours |
| S59 Polish | 2,000 | 8-10 hours |
| S60 Gate Opens | 1,200 | 6-8 hours |
| **TOTAL** | **~10,300** | **~60-78 hours** |

### The Hourglass Measures (Timeguru — Timeline & Milestones)

**Current Age**: Age 1 — Alpha, ~99% structural
**Velocity**: S45-S50 averaged ~1,500 LOC/session. S41-S44 hit 9,800 LOC in one overnight. Realistic: 1,500-3,000 LOC/session.
**ETA to Alpha Gate**: 10 sessions = 2-3 weeks at 1 session/day, 3-4 weeks at 3-4 sessions/week.

**Campaign Timeline**:
```
Week 1 (Feb 25 - Mar 1):  S51 (Security) + S52 (Legal) + S53 (Bare Metal)
Week 2 (Mar 2 - Mar 8):   S54 (eBPF) + S55 (First Trace) + S56 (DOOM)
Week 3 (Mar 9 - Mar 15):  S57 (AI) + S58 (Bridge)
Week 4 (Mar 16 - Mar 22): S59 (Polish) + S60 (Gate Opens)

Alpha Gate Target: March 22, 2026
Confidence: Medium-High (hardware availability is the variable)
```

### The Sundial Tracks (Calendar — Schedule & Deadlines)

**This Week (Feb 25 - Mar 1)**: S51-S53. Security + Legal + Bare Metal Foundation.
**Next Week (Mar 2 - Mar 8)**: S54-S56. eBPF + Traces + DOOM on real hardware.
**Week 3 (Mar 9 - Mar 15)**: S57-S58. AI deployment + EVPN-VXLAN bridge.
**Week 4 (Mar 16 - Mar 22)**: S59-S60. Polish + Alpha Gate opening.

**Protocol Deadlines**: IANA HbH option type registration prep should begin S52 (Barrister + RFC Editor).
**Conference Deadline**: TBD — Captain to decide by S60.

### The Scroll Validates (Lore — Naming & Mythology)

**Campaign Name**: **"The Alpha Gate"** — the first passage through the castle wall. Once the gate opens, the Kingdom is visible to the world.

**Session Names** (each a named battle):
- S51: **"The Moat Ghost's Vigil"** — security hardening before the gate opens
- S52: **"The Barrister's Seal"** — legal protections stamped
- S53: **"The Foundation Stone Returns"** — bare metal, where the Kingdom was always meant to stand
- S54: **"The Whispering Void Awakens"** — eBPF programs come alive on real hardware
- S55: **"First Light"** — the first real packet trace. The product moment.
- S56: **"The Turing Forge Burns"** — DOOM on bare metal. The computational completeness proof runs for real.
- S57: **"Sophia's Eye Opens"** — AI model awakens inside the armor. The first "head."
- S58: **"The Bridge of Sighs"** — EVPN-VXLAN connects two hosts. The Kingdom spans hardware.
- S59: **"The Armorer's Final Touch"** — dashboard polish. Every pixel matters.
- S60: **"The Gate Opens"** — alpha demo. The world sees the Kingdom.

**Mythology Consistency**: All names follow the three pillars. No sacred law violations.

### The Map Confirms (Kingdom — Hierarchy & Placement)

**New Components Across Campaign**:
- `pkg/security/` → Layer 3 (Infrastructure Services) — security hardening utilities
- SBOM artifacts → Layer 0 (Infrastructure) — build-time outputs
- Bare metal NixOS config → Layer 0 (Infrastructure)
- eBPF production programs → Layer 1 (Data Plane)
- EVPN-VXLAN tunnel config → Layer 0 (Infrastructure)
- AI model service containers → Layer 4 (Application Services), ports 20100-20106

**Hierarchy Health**: Solid. All new components slot cleanly.

### The Goblet Toasts (Busboy — Alignment & Coordination)

**Cross-Skill Conflicts**: NONE. All seats aligned.
**Coordination Needs**:
- Moat Ghost + Developer must sync on security P0 closure (S51)
- Barrister + RFC Editor must sync on LICENSE + IANA (S52)
- Architect + Developer must sync on bare metal NixOS (S53)
- BlackMage should review eBPF attack surface after S54
- Captain + Librarian must sync on public-facing content (S60)

**Team Vibes**: MAXIMUM ENERGY. The gate is in sight. 50 sessions of building. 10 sessions to ship. LET'S GO.

---

## THE 10-SESSION CAMPAIGN

---

# S51: THE MOAT GHOST'S VIGIL — Security Hardening Sprint
## 3 Phases, ~45 Steps

**Date**: 2026-02-25
**Sprint**: S51 — Close 6 security P0s, generate SBOM, harden HTTP servers
**Prerequisite**: S50 merged to main. Build GREEN.
**Target**: Zero security P0s remaining. SBOM generated. All HTTP servers have MaxHeaderBytes.
**Estimated Duration**: 4-6 hours
**Agent Strategy**: Phases 1-2 sequential, Phase 3 parallelizable with Phase 2

---

### PHASE 1: SECURITY P0 AUDIT & TRIAGE (Steps 1-12)

**Goal**: Identify all 6 remaining P0s with exact file locations and fix strategies.
**Prerequisite**: Access to codebase on main branch
**Time**: 45 minutes
**Agent**: Coordinator

- [ ] **Step 1** [R]: Read docs/planning/TODO.md for full security findings list
- [ ] **Step 2** [B]: Grep for MaxHeaderBytes across all HTTP servers
  ```bash
  grep -rn "MaxHeaderBytes" cmd/ services/ --include="*.go" | head -30
  ```
- [ ] **Step 3** [V]: Identify which HTTP servers LACK MaxHeaderBytes
  - Expected: Several cmd/ services missing this protection
- [ ] **Step 4** [C]: Document findings in scratch notes

- [ ] **Step 5** [B]: Check gosec status
  ```bash
  which gosec && gosec --version || echo "gosec not installed"
  ```
- [ ] **Step 6** [B]: Run gosec against entire codebase
  ```bash
  gosec -fmt json -out /tmp/gosec-results.json ./... 2>&1 | tail -20
  ```
- [ ] **Step 7** [V]: Triage gosec findings — CRITICAL and HIGH only

- [ ] **Step 8** [B]: Check Nix dependency audit capability
  ```bash
  ls nix/ && grep -rn "flake.lock" nix/ | head -5
  ```
- [ ] **Step 9** [R]: Read nix/flake.lock for pinned dependency versions
- [ ] **Step 10** [V]: Identify any known CVEs in pinned Nix inputs

- [ ] **Step 11** [B]: Check for hardcoded secrets
  ```bash
  grep -rn "password\|secret\|api.key\|token" --include="*.go" --include="*.yaml" --include="*.toml" | grep -vi "test\|example\|TODO\|placeholder" | head -20
  ```
- [ ] **Step 12** [V]: **PHASE 1 EXIT GATE** — All 6 P0s identified with exact locations and fix plans
  - If any P0 unclear → investigate before proceeding

---

### PHASE 2: SECURITY FIXES (Steps 13-32)

**Goal**: Fix all 6 P0s and verify test suite still passes.
**Prerequisite**: Phase 1 triage complete
**Time**: 2-3 hours
**Agent**: Coordinator

#### MaxHeaderBytes Hardening
- [ ] **Step 13** [W]: Add MaxHeaderBytes to every HTTP server missing it
  - Pattern: `&http.Server{MaxHeaderBytes: 1 << 20}` (1MB)
  - Target files: all cmd/*/main.go with http.ListenAndServe
- [ ] **Step 14** [V]: Grep confirms zero HTTP servers without MaxHeaderBytes
- [ ] **Step 15** [C]: Commit: `fix(security): add MaxHeaderBytes to all HTTP servers`

#### gosec Findings
- [ ] **Step 16** [W]: Fix CRITICAL gosec findings (typically: unhandled errors, SQL injection vectors, crypto weaknesses)
- [ ] **Step 17** [W]: Fix HIGH gosec findings
- [ ] **Step 18** [B]: Re-run gosec, verify zero CRITICAL/HIGH
  ```bash
  gosec -severity high ./... 2>&1 | tail -10
  ```
- [ ] **Step 19** [V]: gosec clean on CRITICAL+HIGH
- [ ] **Step 20** [C]: Commit: `fix(security): resolve gosec CRITICAL and HIGH findings`

#### Rate Limiter Hardening
- [ ] **Step 21** [R]: Review rate limiter implementation (X-Forwarded-For spoofing concern)
- [ ] **Step 22** [W]: Add configurable trust-proxy setting; default to direct connection IP
- [ ] **Step 23** [V]: Rate limiter uses peer IP by default, X-Forwarded-For only when trust-proxy enabled
- [ ] **Step 24** [C]: Commit: `fix(security): rate limiter uses peer IP by default`

#### Auth Framework Scaffold
- [ ] **Step 25** [W]: Create `pkg/auth/` with interface: `Authenticator`, `Claims`, `Middleware`
- [ ] **Step 26** [W]: Implement `NoopAuthenticator` (alpha mode — logs warning, allows all)
- [ ] **Step 27** [W]: Implement `APIKeyAuthenticator` (reads from config, checks Authorization header)
- [ ] **Step 28** [W]: Wire auth middleware into gateway
- [ ] **Step 29** [B]: Run tests
  ```bash
  go test ./pkg/auth/... -race -v
  ```
- [ ] **Step 30** [V]: Auth package tests pass
- [ ] **Step 31** [C]: Commit: `feat(security): auth framework scaffold with API key authenticator`

- [ ] **Step 32** [V]: **PHASE 2 EXIT GATE** — `go test ./... -race` passes. Zero CRITICAL/HIGH in gosec.

---

### PHASE 3: SBOM GENERATION (Steps 33-45)

**Goal**: Generate Software Bill of Materials using ScanCode + Go module analysis.
**Prerequisite**: Phase 2 complete (clean codebase)
**Time**: 1-2 hours
**Agent**: Agent [P] (can run parallel to late Phase 2)

- [ ] **Step 33** [B]: Install scancode-toolkit if not present
  ```bash
  pip install scancode-toolkit --break-system-packages 2>&1 | tail -5
  ```
- [ ] **Step 34** [B]: Generate Go module SBOM
  ```bash
  go list -m -json all > /tmp/go-modules-sbom.json 2>&1
  ```
- [ ] **Step 35** [B]: Run ScanCode against codebase
  ```bash
  scancode --license --copyright --package --json /tmp/scancode-results.json . 2>&1 | tail -10
  ```
- [ ] **Step 36** [V]: ScanCode completes without errors
  - If timeout → run on specific directories: cmd/, pkg/, services/
- [ ] **Step 37** [W]: Create `docs/compliance/SBOM.md` summarizing findings
- [ ] **Step 38** [W]: Create `docs/compliance/LICENSE-AUDIT.md` listing all detected licenses
- [ ] **Step 39** [V]: No GPL code in main codebase (DOOM is isolated in separate repo)
- [ ] **Step 40** [V]: No unknown/proprietary licenses detected in dependencies
- [ ] **Step 41** [C]: Commit: `docs(compliance): SBOM generation and license audit`

#### Final Verification
- [ ] **Step 42** [B]: Full test suite
  ```bash
  go test ./... -race -count=1 2>&1 | tail -20
  ```
- [ ] **Step 43** [V]: ALL PASS
- [ ] **Step 44** [B]: Build all binaries
  ```bash
  go build ./... 2>&1
  ```
- [ ] **Step 45** [V]: **S51 EXIT GATE** — Zero security P0s. SBOM generated. Auth framework scaffolded. All tests pass.
  - If fail → DO NOT proceed to S52

---

*S51 Battle Plan — Forged 2026-02-25*
*3 Phases. 45 Steps. The Moat Ghost sees all. The cracks are sealed.*

---

# S52: THE BARRISTER'S SEAL — Legal & Licensing Sprint
## 3 Phases, ~38 Steps

**Date**: Target 2026-02-26
**Sprint**: S52 — LICENSE file, IP audit, IANA prep, contributor framework
**Prerequisite**: S51 EXIT GATE passed. Zero security P0s.
**Target**: BSL 1.1 LICENSE file committed. THIRD_PARTY.md complete. Contributor guide drafted.
**Estimated Duration**: 3-4 hours
**Agent Strategy**: Phases 1-3 sequential (legal decisions cascade)

---

### PHASE 1: LICENSE FILE CREATION (Steps 1-14)

**Goal**: Draft and commit BSL 1.1 license for the main codebase.
**Time**: 1 hour
**Agent**: Coordinator (legal decisions need Muck approval)

- [ ] **Step 1** [R]: Read current THIRD_PARTY.md for existing license landscape
- [ ] **Step 2** [R]: Read MariaDB BSL 1.1 template for reference
- [ ] **Step 3** [W]: Create `LICENSE` file — BSL 1.1 with:
  - Licensor: Steven R. Bellis / Unheaded
  - Licensed Work: Unheaded (all versions)
  - Change Date: 4 years from first public release (or K8s-scale adoption trigger)
  - Change License: Apache 2.0 OR MIT (Muck decides)
  - Additional Use Grant: Protocol implementations exempt (specs are permissive)
- [ ] **Step 4** [V]: LICENSE file valid BSL 1.1 format
- [ ] **Step 5** [C]: Commit: `legal: add BSL 1.1 LICENSE file`

- [ ] **Step 6** [W]: Create `LICENSES/` directory structure:
  - `LICENSES/BSL-1.1.txt` — main codebase
  - `LICENSES/GPL-2.0.txt` — DOOM fork (reference)
  - `LICENSES/PROTOCOL-LICENSE.txt` — permissive license for protocol specs
- [ ] **Step 7** [V]: All license files present and correctly formatted
- [ ] **Step 8** [C]: Commit: `legal: structured LICENSES/ directory`

#### License Headers
- [ ] **Step 9** [B]: Count Go files missing license headers
  ```bash
  find . -name "*.go" -not -path "./vendor/*" | xargs grep -L "SPDX-License-Identifier" | wc -l
  ```
- [ ] **Step 10** [W]: Create header template script that adds BSL 1.1 SPDX header to all .go files
- [ ] **Step 11** [B]: Run header script
- [ ] **Step 12** [V]: All .go files have SPDX-License-Identifier header
- [ ] **Step 13** [B]: `go build ./...` still passes after header additions
- [ ] **Step 14** [C]: Commit: `legal: add SPDX license headers to all Go source files`

---

### PHASE 2: IP AUDIT & THIRD_PARTY.md (Steps 15-26)

**Goal**: Complete IP inventory and third-party dependency documentation.
**Time**: 1.5 hours
**Agent**: Agent

- [ ] **Step 15** [R]: Read existing THIRD_PARTY.md
- [ ] **Step 16** [B]: List all Go module dependencies
  ```bash
  go list -m all | wc -l
  ```
- [ ] **Step 17** [W]: Update THIRD_PARTY.md with complete dependency list + licenses
- [ ] **Step 18** [W]: Document GPL boundary clearly:
  - DOOM code is in SEPARATE REPOSITORY (github.com/unheaded/DOOM)
  - Main repo has ZERO GPL code
  - MBC bytecode is compiled output, not derived work (cross-compilation boundary)
  - doom-bridge reads BPF maps, doesn't link GPL code
- [ ] **Step 19** [V]: GPL isolation boundary is airtight in documentation
- [ ] **Step 20** [C]: Commit: `legal: complete THIRD_PARTY.md and GPL boundary docs`

- [ ] **Step 21** [W]: Create `docs/legal/IP-INVENTORY.md`:
  - Patent candidates: Monad wire format, exponent encoding, eBPF packet marking
  - Trade secrets: Sophia dictionary internals, Wotan memory model, service discovery architecture
  - Copyrights: All source code, documentation, specs
  - Trademarks: "Unheaded" name and logo (register intent)
- [ ] **Step 22** [W]: Create `docs/legal/CONTRIBUTOR-GUIDE.md`:
  - DCO (Developer Certificate of Origin) sign-off required
  - No CLA initially (too heavy for early contributors)
  - Code of conduct reference
  - IP assignment: contributors retain copyright, grant license
- [ ] **Step 23** [C]: Commit: `legal: IP inventory and contributor guide`

#### IANA Registration Prep
- [ ] **Step 24** [W]: Create `docs/legal/IANA-REGISTRATION.md`:
  - IPv6 Hop-by-Hop option type for Monad
  - Registration requirements and timeline
  - Contact information for IANA submission
- [ ] **Step 25** [V]: IANA doc references correct RFC sections
- [ ] **Step 26** [C]: Commit: `legal: IANA registration preparation`

---

### PHASE 3: VERIFICATION (Steps 27-38)

**Goal**: Verify all legal artifacts are complete and consistent.
**Time**: 30 minutes
**Agent**: Agent

- [ ] **Step 27** [V]: LICENSE file in repo root
- [ ] **Step 28** [V]: LICENSES/ directory with all 3 license texts
- [ ] **Step 29** [V]: All .go files have SPDX headers
- [ ] **Step 30** [V]: THIRD_PARTY.md lists all dependencies with licenses
- [ ] **Step 31** [V]: GPL boundary documented and airtight
- [ ] **Step 32** [V]: IP-INVENTORY.md lists all protectable assets
- [ ] **Step 33** [V]: CONTRIBUTOR-GUIDE.md ready for external contributors
- [ ] **Step 34** [V]: IANA-REGISTRATION.md references correct specs
- [ ] **Step 35** [B]: Full test suite passes
  ```bash
  go test ./... -race 2>&1 | tail -10
  ```
- [ ] **Step 36** [V]: ALL PASS
- [ ] **Step 37** [B]: Verify no secrets in committed files
  ```bash
  grep -rn "BEGIN.*PRIVATE\|password.*=\|secret.*=" --include="*.go" --include="*.yaml" | grep -vi "test\|example\|TODO" | head -5
  ```
- [ ] **Step 38** [V]: **S52 EXIT GATE** — All legal artifacts committed. LICENSE, SPDX headers, THIRD_PARTY, IP inventory, contributor guide, IANA prep. Tests pass.

---

*S52 Battle Plan — Forged 2026-02-25*
*3 Phases. 38 Steps. The Barrister's seal protects the Kingdom.*

---

# S53: THE FOUNDATION STONE RETURNS — Bare Metal NixOS Setup
## 4 Phases, ~52 Steps

**Date**: Target 2026-02-27
**Sprint**: S53 — NixOS on bare metal, Unheaded daemon deployed, network configured
**Prerequisite**: S52 EXIT GATE passed. Bare metal server physically available.
**Target**: NixOS running on bare metal with Unheaded daemon + Wotan + core services operational.
**Estimated Duration**: 6-8 hours
**Agent Strategy**: Phases 1-2 sequential, Phases 3-4 parallel

---

### PHASE 1: BARE METAL PREPARATION (Steps 1-15)

**Goal**: NixOS installed and base system configured on 4-core DDR3 server.
**Time**: 2 hours
**Agent**: Coordinator [S]

- [ ] **Step 1** [S][B]: Verify hardware specs
  ```bash
  lscpu && free -h && lsblk
  ```
- [ ] **Step 2** [V]: Confirm 4+ cores, adequate RAM (≥4GB), available disk
- [ ] **Step 3** [S][B]: Check kernel version
  ```bash
  uname -r
  ```
- [ ] **Step 4** [V]: Kernel ≥ 5.15 (required for eBPF features in S54)
  - If < 5.15 → need NixOS with newer kernel package
- [ ] **Step 5** [S][B]: Install NixOS or verify existing installation
  ```bash
  nixos-version 2>/dev/null || echo "NixOS not installed"
  ```
- [ ] **Step 6** [D]: If NixOS not installed, generate config from nix/ directory
- [ ] **Step 7** [S][W]: Create `/etc/nixos/unheaded.nix` importing our nix/ modules
- [ ] **Step 8** [S][B]: Apply NixOS configuration
  ```bash
  sudo nixos-rebuild switch
  ```
- [ ] **Step 9** [V]: NixOS rebuilt successfully
- [ ] **Step 10** [C]: Commit NixOS config if new

#### Network Foundation
- [ ] **Step 11** [S][B]: Configure static IP on management interface
- [ ] **Step 12** [S][B]: Enable IPv6 forwarding
  ```bash
  sysctl -w net.ipv6.conf.all.forwarding=1
  ```
- [ ] **Step 13** [S][B]: Verify BPF filesystem mounted
  ```bash
  mount | grep bpf || sudo mount -t bpf bpf /sys/fs/bpf
  ```
- [ ] **Step 14** [V]: BPF filesystem at /sys/fs/bpf
- [ ] **Step 15** [V]: **PHASE 1 EXIT GATE** — NixOS running, IPv6 forwarding enabled, BPF filesystem mounted

---

### PHASE 2: UNHEADED DEPLOYMENT (Steps 16-32)

**Goal**: Deploy Unheaded daemon + Wotan + core services on bare metal.
**Time**: 2 hours
**Agent**: Coordinator

- [ ] **Step 16** [B]: Clone/sync Unheaded repo to bare metal
  ```bash
  git clone <repo> /opt/unheaded/src && cd /opt/unheaded/src
  ```
- [ ] **Step 17** [B]: Build all binaries
  ```bash
  go build ./...
  ```
- [ ] **Step 18** [V]: All binaries compile
- [ ] **Step 19** [B]: Run test suite
  ```bash
  go test ./... -race -count=1 2>&1 | tail -20
  ```
- [ ] **Step 20** [V]: Tests pass on bare metal
- [ ] **Step 21** [C]: Record build hash

#### Service Deployment
- [ ] **Step 22** [W]: Create systemd unit files for core services:
  - unheaded-daemon (17000/17001)
  - wotan (18000/18001)
  - dashboard-backend (20000)
  - trace-collector (16670/16671)
  - gateway (21000/21443)
- [ ] **Step 23** [S][B]: Install systemd units
  ```bash
  sudo cp systemd/*.service /etc/systemd/system/ && sudo systemctl daemon-reload
  ```
- [ ] **Step 24** [S][B]: Start Wotan first (message bus must be up)
  ```bash
  sudo systemctl start wotan && sleep 2 && curl -s http://localhost:18000/health
  ```
- [ ] **Step 25** [V]: Wotan healthy on 18000/18001
- [ ] **Step 26** [S][B]: Start remaining services
  ```bash
  for svc in unheaded-daemon dashboard-backend trace-collector gateway; do
    sudo systemctl start $svc && echo "$svc started"
  done
  ```
- [ ] **Step 27** [V]: All services healthy
  ```bash
  for port in 17000 18000 20000 16670 21000; do
    curl -sf http://localhost:$port/health && echo "port $port: OK" || echo "port $port: FAIL"
  done
  ```
- [ ] **Step 28** [C]: Commit: systemd units

#### Connectivity Verification
- [ ] **Step 29** [B]: Dashboard accessible from browser
  ```bash
  curl -sf http://localhost:20000/ | head -5
  ```
- [ ] **Step 30** [V]: Dashboard HTML returned
- [ ] **Step 31** [B]: Check service discovery registration
  ```bash
  curl -sf http://localhost:18000/api/v1/services | python3 -m json.tool | head -20
  ```
- [ ] **Step 32** [V]: **PHASE 2 EXIT GATE** — All core services running on bare metal, health checks passing, dashboard accessible

---

### PHASE 3: KERNEL TUNING (Steps 33-42)

**Goal**: Tune kernel for eBPF workloads.
**Time**: 1 hour
**Agent**: Coordinator [S]

- [ ] **Step 33** [S][B]: Check eBPF capabilities
  ```bash
  bpftool feature probe kernel 2>&1 | head -30
  ```
- [ ] **Step 34** [V]: BPF JIT enabled, BPF bounded loops supported
  - If missing → need kernel config changes
- [ ] **Step 35** [S][B]: Tune kernel parameters for eBPF
  ```bash
  sudo sysctl -w kernel.bpf_stats_enabled=1
  sudo sysctl -w net.core.bpf_jit_enable=1
  sudo sysctl -w net.core.bpf_jit_harden=0  # alpha mode, harden in production
  ```
- [ ] **Step 36** [S][B]: Increase BPF memory limits
  ```bash
  sudo sysctl -w kernel.unprivileged_bpf_disabled=1  # only root loads BPF
  ulimit -l unlimited
  ```
- [ ] **Step 37** [V]: BPF settings applied
- [ ] **Step 38** [C]: Document kernel tuning in docs/operations/KERNEL_TUNING.md

#### Required Tools
- [ ] **Step 39** [S][B]: Install eBPF toolchain
  ```bash
  which bpftool ip tc nsenter || sudo apt-get install -y linux-tools-$(uname -r) iproute2
  ```
- [ ] **Step 40** [V]: All tools available
- [ ] **Step 41** [S][B]: Install Rust toolchain for eBPF compilation
  ```bash
  rustup target add bpfel-unknown-none && cargo install bpf-linker
  ```
- [ ] **Step 42** [V]: **PHASE 3 EXIT GATE** — Kernel tuned for eBPF, all tools installed

---

### PHASE 4: SMOKE TEST (Steps 43-52)

**Goal**: End-to-end verification of bare metal deployment.
**Time**: 30 minutes
**Agent**: Agent

- [ ] **Step 43** [B]: Test Wotan gRPC streaming
  ```bash
  grpcurl -plaintext localhost:18001 grpc.health.v1.Health/Check
  ```
- [ ] **Step 44** [V]: Wotan gRPC healthy
- [ ] **Step 45** [B]: Test log aggregation
  ```bash
  curl -sf "http://localhost:20000/api/v1/logs?lines=10" | python3 -m json.tool | head -20
  ```
- [ ] **Step 46** [V]: Logs flowing from services to dashboard
- [ ] **Step 47** [B]: Test dashboard WebSocket
  ```bash
  curl -sf -H "Upgrade: websocket" -H "Connection: Upgrade" http://localhost:20000/ws/metrics
  ```
- [ ] **Step 48** [B]: Open dashboard in browser, verify all 8 tabs render
- [ ] **Step 49** [V]: Dashboard functional on bare metal
- [ ] **Step 50** [C]: Commit: `ops: bare metal deployment verified`
- [ ] **Step 51** [W]: Write handoff: `docs/sessions/S53-handoff.md`
- [ ] **Step 52** [V]: **S53 EXIT GATE** — NixOS bare metal running all core services, kernel tuned for eBPF, dashboard accessible, logs flowing, gRPC healthy.

---

*S53 Battle Plan — Forged 2026-02-25*
*4 Phases. 52 Steps. The Foundation Stone returns to bare metal where it belongs.*

---

# S54: THE WHISPERING VOID AWAKENS — eBPF Production Programs
## 4 Phases, ~55 Steps

**Date**: Target 2026-03-02
**Sprint**: S54 — Compile, verify, and attach production eBPF programs on bare metal
**Prerequisite**: S53 EXIT GATE passed. Bare metal with kernel ≥ 5.15 and BPF tools.
**Target**: packet_marker, flow_tracker, and latency_probe eBPF programs loaded and verified by kernel.
**Estimated Duration**: 6-8 hours
**Agent Strategy**: Phases 1-2 sequential, Phases 3-4 parallel

---

### PHASE 1: eBPF BUILD ENVIRONMENT (Steps 1-14)

**Goal**: Compile all eBPF programs from Rust source.
**Time**: 1.5 hours
**Agent**: Coordinator [S]

- [ ] **Step 1** [B]: Verify Rust eBPF toolchain
  ```bash
  rustup show && cargo install --list | grep bpf
  ```
- [ ] **Step 2** [R]: Review ebpf/ directory structure for production programs
- [ ] **Step 3** [B]: Build monad-cpu-ebpf (already verified — baseline)
  ```bash
  cd crates/monad-cpu-ebpf && cargo build --release --target bpfel-unknown-none 2>&1 | tail -10
  ```
- [ ] **Step 4** [V]: monad-cpu-ebpf compiles
- [ ] **Step 5** [C]: Baseline confirmed

- [ ] **Step 6** [B]: Build packet_marker XDP program
  ```bash
  cd ebpf/packet_marker && cargo build --release --target bpfel-unknown-none 2>&1 | tail -20
  ```
- [ ] **Step 7** [V]: packet_marker compiles
  - If fail → check BPF helper usage, map definitions
- [ ] **Step 8** [D]: Debug verifier rejections
  ```bash
  sudo bpftool prog load packet_marker.o /sys/fs/bpf/unheaded/test 2>&1
  ```

- [ ] **Step 9** [B]: Build flow_tracker TC program
  ```bash
  cd ebpf/flow_tracker && cargo build --release --target bpfel-unknown-none 2>&1 | tail -20
  ```
- [ ] **Step 10** [V]: flow_tracker compiles

- [ ] **Step 11** [B]: Build latency_probe kprobe program
  ```bash
  cd ebpf/latency_probe && cargo build --release --target bpfel-unknown-none 2>&1 | tail -20
  ```
- [ ] **Step 12** [V]: latency_probe compiles
- [ ] **Step 13** [C]: All 3 production eBPF programs compiled
- [ ] **Step 14** [V]: **PHASE 1 EXIT GATE** — All eBPF programs compile for BPF target

---

### PHASE 2: KERNEL VERIFICATION (Steps 15-30)

**Goal**: Load each program into the kernel and pass BPF verifier.
**Time**: 2 hours
**Agent**: Coordinator [S]

#### packet_marker (XDP)
- [ ] **Step 15** [S][B]: Create test namespace for verification
  ```bash
  sudo ip netns add ebpf-test && sudo ip link add veth-test type veth peer name veth-test-peer
  sudo ip link set veth-test-peer netns ebpf-test
  ```
- [ ] **Step 16** [S][B]: Load packet_marker to XDP on test interface
  ```bash
  sudo bpftool prog load packet_marker.o /sys/fs/bpf/unheaded/packet_marker type xdp
  ```
- [ ] **Step 17** [V]: Verifier accepts
  - If reject → Step 18
- [ ] **Step 18** [D]: Read verifier log
  ```bash
  sudo bpftool prog load packet_marker.o /sys/fs/bpf/unheaded/packet_marker type xdp 2>&1 | head -50
  ```
- [ ] **Step 19** [S][B]: Attach to test interface
  ```bash
  sudo bpftool net attach xdpgeneric id $(bpftool prog show name packet_marker -j | jq '.[0].id') dev veth-test
  ```
- [ ] **Step 20** [V]: XDP attached — `bpftool net show dev veth-test`
- [ ] **Step 21** [C]: packet_marker verified

#### flow_tracker (TC)
- [ ] **Step 22** [S][B]: Load flow_tracker
  ```bash
  sudo bpftool prog load flow_tracker.o /sys/fs/bpf/unheaded/flow_tracker type tc
  ```
- [ ] **Step 23** [V]: Verifier accepts
- [ ] **Step 24** [S][B]: Attach to tc clsact qdisc
  ```bash
  sudo tc qdisc add dev veth-test clsact
  sudo tc filter add dev veth-test ingress bpf da fd /sys/fs/bpf/unheaded/flow_tracker
  ```
- [ ] **Step 25** [V]: TC filter attached
- [ ] **Step 26** [C]: flow_tracker verified

#### latency_probe (kprobe)
- [ ] **Step 27** [S][B]: Load latency_probe
  ```bash
  sudo bpftool prog load latency_probe.o /sys/fs/bpf/unheaded/latency_probe type kprobe
  ```
- [ ] **Step 28** [V]: Verifier accepts
- [ ] **Step 29** [S][B]: Attach to target kernel function (e.g., tcp_v6_rcv)

- [ ] **Step 30** [V]: **PHASE 2 EXIT GATE** — All 3 eBPF programs pass kernel verifier and attach to interfaces

---

### PHASE 3: MAP PINNING & GO INTEGRATION (Steps 31-44)

**Goal**: Pin BPF maps and wire Go userspace to read them.
**Time**: 1.5 hours
**Agent**: Coordinator

- [ ] **Step 31** [S][B]: Create pin directory
  ```bash
  sudo mkdir -p /sys/fs/bpf/unheaded/maps
  ```
- [ ] **Step 32** [S][B]: Pin packet_marker maps
  ```bash
  sudo bpftool map pin name trace_events /sys/fs/bpf/unheaded/maps/trace_events
  sudo bpftool map pin name flow_state /sys/fs/bpf/unheaded/maps/flow_state
  ```
- [ ] **Step 33** [V]: Maps visible: `ls /sys/fs/bpf/unheaded/maps/`

- [ ] **Step 34** [W]: Wire cmd/trace-collector/ to read pinned maps via cilium/ebpf
- [ ] **Step 35** [B]: Test trace-collector reads from real maps
  ```bash
  ./trace-collector --bpf-maps-dir=/sys/fs/bpf/unheaded/maps --mode=bpf 2>&1 | head -20
  ```
- [ ] **Step 36** [V]: trace-collector reads map data without errors
- [ ] **Step 37** [C]: Commit: `feat(ebpf): wire trace-collector to real BPF maps`

- [ ] **Step 38** [B]: Generate test traffic
  ```bash
  sudo ip netns exec ebpf-test ping6 -c 10 fd00:test::1
  ```
- [ ] **Step 39** [V]: trace-collector picks up ping packets in trace events
- [ ] **Step 40** [C]: First real eBPF trace captured!

#### Dashboard Integration
- [ ] **Step 41** [B]: Start dashboard-backend with real trace data
- [ ] **Step 42** [V]: Dashboard Packet Flow tab shows real trace events
- [ ] **Step 43** [C]: Commit: `feat(dashboard): real eBPF trace data in Packet Flow tab`

- [ ] **Step 44** [V]: **PHASE 3 EXIT GATE** — Maps pinned, trace-collector reading real data, dashboard showing traces

---

### PHASE 4: CLEANUP & HANDOFF (Steps 45-55)

**Goal**: Clean test namespace, document findings, prepare for S55.
**Time**: 30 minutes
**Agent**: Agent

- [ ] **Step 45** [S][B]: Clean test namespace
  ```bash
  sudo ip netns del ebpf-test
  ```
- [ ] **Step 46** [W]: Write `docs/sessions/S54-handoff.md`
- [ ] **Step 47** [W]: Update `references/timeline.md` — Whispering Void progress → 85%+
- [ ] **Step 48** [W]: Update `CLAUDE.md` with eBPF program status
- [ ] **Step 49** [B]: Full test suite
  ```bash
  go test ./... -race 2>&1 | tail -10
  ```
- [ ] **Step 50** [V]: ALL PASS
- [ ] **Step 51** [C]: Commit: `docs: S54 handoff and timeline update`
- [ ] **Step 52** [B]: Git log
  ```bash
  git log --oneline -10
  ```
- [ ] **Step 53** [V]: Clean git history
- [ ] **Step 54** [W]: Summary of eBPF programs: which maps, which attach points, which metrics
- [ ] **Step 55** [V]: **S54 EXIT GATE** — All 3 eBPF programs verified by kernel, maps pinned, trace-collector integrated, dashboard showing real traces, all tests pass.

---

*S54 Battle Plan — Forged 2026-02-25*
*4 Phases. 55 Steps. The Whispering Void speaks. The kernel listens.*

---

# S55: FIRST LIGHT — The First Real Packet Trace
## 3 Phases, ~42 Steps

**Date**: Target 2026-03-03
**Sprint**: S55 — End-to-end packet tracing: XDP → flow tracker → trace collector → Wotan → dashboard
**Prerequisite**: S54 EXIT GATE passed. eBPF programs verified.
**Target**: HTTP request traced from browser → gateway → service → response, entire path visible in dashboard.
**Estimated Duration**: 8-10 hours
**Agent Strategy**: All phases sequential (pipeline must be built in order)

---

### PHASE 1: TRACING PIPELINE ASSEMBLY (Steps 1-18)

**Goal**: Wire the full observability pipeline from eBPF to dashboard.
**Time**: 3 hours
**Agent**: Coordinator [S]

- [ ] **Step 1** [S][B]: Attach packet_marker to gateway interface (XDP)
- [ ] **Step 2** [S][B]: Attach flow_tracker to gateway interface (TC ingress/egress)
- [ ] **Step 3** [S][B]: Attach latency_probe to kernel TCP stack (kprobe)
- [ ] **Step 4** [V]: All 3 programs attached — `bpftool net show`
- [ ] **Step 5** [C]: Programs attached

- [ ] **Step 6** [B]: Start trace-collector in BPF mode
  ```bash
  ./trace-collector --mode=bpf --wotan-addr=localhost:18001 &
  ```
- [ ] **Step 7** [V]: trace-collector connecting to Wotan gRPC
- [ ] **Step 8** [B]: Verify trace-collector publishing to Wotan topic `traces.raw`
- [ ] **Step 9** [V]: Wotan receiving trace events

- [ ] **Step 10** [B]: Configure dashboard-backend to subscribe to `traces.raw`
- [ ] **Step 11** [V]: Dashboard-backend receiving traces via Wotan
- [ ] **Step 12** [B]: Generate test HTTP traffic through gateway
  ```bash
  for i in $(seq 1 100); do curl -sf http://localhost:21000/health; done
  ```
- [ ] **Step 13** [V]: Traces appear in dashboard Packet Flow tab
- [ ] **Step 14** [C]: Pipeline assembled

#### Trace Correlation
- [ ] **Step 15** [W]: Implement trace_id correlation in flow_tracker
  - packet_marker stamps flow label → flow_tracker reads flow label → correlates request/response
- [ ] **Step 16** [B]: Test correlation with paired request/response
- [ ] **Step 17** [V]: Request and response traces share same trace_id
- [ ] **Step 18** [V]: **PHASE 1 EXIT GATE** — Full pipeline: XDP → TC → trace-collector → Wotan → dashboard. Traces correlated.

---

### PHASE 2: DASHBOARD TRACE VISUALIZATION (Steps 19-32)

**Goal**: Make the trace data beautiful and informative in the dashboard.
**Time**: 3 hours
**Agent**: Agent

- [ ] **Step 19** [W]: Enhance Packet Flow tab with real-time trace stream
- [ ] **Step 20** [W]: Add flow label → service name resolution
- [ ] **Step 21** [W]: Add latency overlay (color-coded by RTT)
- [ ] **Step 22** [W]: Add trace detail panel (click trace → see full packet headers)
- [ ] **Step 23** [V]: Packet Flow tab shows live traces with service names and latency
- [ ] **Step 24** [C]: Commit: `feat(dashboard): real-time packet trace visualization`

- [ ] **Step 25** [W]: Enhance Trace Table tab with sortable columns
- [ ] **Step 26** [W]: Add filter by service, latency threshold, time range
- [ ] **Step 27** [V]: Trace Table functional with real data
- [ ] **Step 28** [C]: Commit: `feat(dashboard): trace table with filtering`

- [ ] **Step 29** [W]: Wire Latency Chart to real latency_probe data
- [ ] **Step 30** [V]: Latency chart showing real-time P50/P95/P99
- [ ] **Step 31** [C]: Commit: `feat(dashboard): real-time latency chart from eBPF`
- [ ] **Step 32** [V]: **PHASE 2 EXIT GATE** — All trace-related dashboard tabs showing real eBPF data

---

### PHASE 3: E2E VERIFICATION & "FIRST LIGHT" MOMENT (Steps 33-42)

**Goal**: The product moment. Real HTTP request traced end-to-end in the dashboard.
**Time**: 1 hour
**Agent**: Coordinator

- [ ] **Step 33** [B]: Open dashboard in browser on separate machine
- [ ] **Step 34** [B]: Generate real HTTP traffic: browse dashboard itself (self-tracing!)
- [ ] **Step 35** [V]: Dashboard shows its own HTTP requests being traced — **THE META MOMENT 2.0**
- [ ] **Step 36** [B]: Measure end-to-end latency: packet hits XDP → appears in dashboard
  - Target: < 100ms
- [ ] **Step 37** [V]: Trace latency < 100ms
- [ ] **Step 38** [C]: Commit: `milestone: FIRST LIGHT — real packet trace end-to-end`

- [ ] **Step 39** [W]: Write `docs/sessions/S55-handoff.md` — "First Light achieved"
- [ ] **Step 40** [W]: Update `references/timeline.md` — Whispering Void → 95%
- [ ] **Step 41** [B]: `go test ./... -race` — ALL PASS
- [ ] **Step 42** [V]: **S55 EXIT GATE** — Real HTTP request traced from XDP to dashboard in < 100ms. The product works. FIRST LIGHT.

---

*S55 Battle Plan — Forged 2026-02-25*
*3 Phases. 42 Steps. First Light. The product is real. The packets speak.*

---

# S56: THE TURING FORGE BURNS — DOOM on Bare Metal
## 3 Phases, ~40 Steps

**Date**: Target 2026-03-04
**Sprint**: S56 — 6-namespace packet ring, DOOM loaded, frames in browser
**Prerequisite**: S55 EXIT GATE passed. eBPF pipeline proven.
**Target**: DOOM playing in browser over IPv6 packet ring on bare metal.
**Estimated Duration**: 6-8 hours
**Agent Strategy**: Phases 1-3 sequential (each builds on prior)

---

### PHASE 1: PACKET RING ASSEMBLY (Steps 1-16)

**Goal**: Set up 6-namespace packet circulation ring.
**Time**: 2 hours
**Agent**: Coordinator [S]

- [ ] **Step 1** [S][B]: Create 6 namespaces (monad0-5)
  ```bash
  for i in $(seq 0 5); do sudo ip netns add monad$i; done
  ```
- [ ] **Step 2** [V]: All 6 exist: `ip netns list | grep monad`
- [ ] **Step 3** [S][B]: Create veth pairs linking namespaces in ring
- [ ] **Step 4** [S][B]: Configure per-link /64 IPv6 prefixes
  ```bash
  for i in $(seq 0 5); do
    sudo ip netns exec monad$i ip -6 addr add fd00:3f:75:$i::1/64 dev veth${i}-ns
  done
  ```
- [ ] **Step 5** [S][B]: Enable IPv6 forwarding in all namespaces
- [ ] **Step 6** [S][B]: Configure default routes for ring circulation
- [ ] **Step 7** [B]: Test ping through full ring
  ```bash
  sudo ip netns exec monad0 ping6 -c 100 fd00:dead::1
  ```
- [ ] **Step 8** [V]: 100/100 ping success through all 6 hops
- [ ] **Step 9** [C]: Packet ring assembled

#### BPF Loading
- [ ] **Step 10** [S][B]: Load monad-cpu-ebpf and pin maps
  ```bash
  sudo bpftool prog load monad-cpu-ebpf.o /sys/fs/bpf/unheaded/doom/cpu pinmaps /sys/fs/bpf/unheaded/doom/maps
  ```
- [ ] **Step 11** [V]: Maps pinned (ROM_MAP, CPU_MAP, RAM_MAP, SCREEN_MAP, KBD_MAP, etc.)
- [ ] **Step 12** [S][B]: Attach XDP to all 6 hop interfaces
- [ ] **Step 13** [V]: All 6 hops share same prog_id
- [ ] **Step 14** [B]: Initialize CPU state via doom CLI
  ```bash
  ./doom reset && ./doom status
  ```
- [ ] **Step 15** [V]: CPU state initialized (PC=0, halted=0)
- [ ] **Step 16** [V]: **PHASE 1 EXIT GATE** — Ring operational, BPF loaded, maps pinned, CPU initialized

---

### PHASE 2: DOOM ROM LOADING & EXECUTION (Steps 17-30)

**Goal**: Load doom.mbc, inject packets, see frames rendered.
**Time**: 3 hours
**Agent**: Coordinator [S]

- [ ] **Step 17** [B]: Load DOOM ROM into BPF maps
  ```bash
  ./doom load doom.mbc --stats
  ```
- [ ] **Step 18** [V]: ROM loaded, size reported
- [ ] **Step 19** [B]: Begin packet injection at safe rate (3ms delay)
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 50000 --delay 3000
  ```
- [ ] **Step 20** [V]: CPU insn_count increasing — `./doom status`
- [ ] **Step 21** [B]: Monitor for BSS clearing (~60M instructions)
- [ ] **Step 22** [V]: BSS clear complete (insn_count > 60M)
- [ ] **Step 23** [V]: No HALT flag set
  - If HALT → Step 24
- [ ] **Step 24** [D]: Check halt reason, patch libc stubs if needed
- [ ] **Step 25** [B]: Wait for first frame write to SCREEN_MAP
  ```bash
  ./doom screen-check  # returns non-zero pixel count
  ```
- [ ] **Step 26** [V]: First frame rendered (non-zero pixels in screen buffer)
- [ ] **Step 27** [C]: DOOM is RUNNING on bare metal!

#### Performance Tuning
- [ ] **Step 28** [B]: Profile injection rate — find minimum safe delay
  ```bash
  for delay in 2000 1500 1000 750 500; do
    echo "Testing delay=$delay"
    sudo python3 scripts/doom-tick.py --delay $delay --count 1000 2>&1 | tail -1
  done
  ```
- [ ] **Step 29** [V]: Minimum safe delay identified
- [ ] **Step 30** [V]: **PHASE 2 EXIT GATE** — DOOM running, frames rendering, optimal delay found

---

### PHASE 3: BROWSER INTEGRATION (Steps 31-40)

**Goal**: DOOM frames visible in browser via doom-bridge WebSocket.
**Time**: 1.5 hours
**Agent**: Agent

- [ ] **Step 31** [B]: Start doom-bridge service
  ```bash
  ./doom-bridge --bpf-maps=/sys/fs/bpf/unheaded/doom/maps --port=16666 &
  ```
- [ ] **Step 32** [V]: doom-bridge reading SCREEN_MAP
- [ ] **Step 33** [B]: Open dashboard doom.html in browser
- [ ] **Step 34** [V]: DOOM frames visible in browser canvas
- [ ] **Step 35** [V]: Keyboard input works (WASD, arrows, Ctrl, Space)
- [ ] **Step 36** [C]: DOOM PLAYABLE IN BROWSER OVER IPv6!

- [ ] **Step 37** [B]: Measure FPS
- [ ] **Step 38** [W]: Document performance: `docs/doom/BARE_METAL_PERFORMANCE.md`
- [ ] **Step 39** [W]: Write `docs/sessions/S56-handoff.md`
- [ ] **Step 40** [V]: **S56 EXIT GATE** — DOOM playing in browser via 6-namespace packet ring on bare metal. Keyboard works. FPS documented.

---

*S56 Battle Plan — Forged 2026-02-25*
*3 Phases. 40 Steps. The Turing Forge burns. Packets become computation.*

---

# S57: SOPHIA'S EYE OPENS — AI Model Stack Deployment
## 3 Phases, ~38 Steps

**Date**: Target 2026-03-09
**Sprint**: S57 — vLLM + DeepSeek-R1 + Qwen 2.5 on gaming desktop, eBPF-traced inference
**Prerequisite**: S56 EXIT GATE passed. Gaming desktop physically available with RX 7700 XT.
**Target**: AI model serving inference requests, registered in Sophia, traced by eBPF, visible in dashboard.
**Estimated Duration**: 6-8 hours
**Agent Strategy**: Phases 1-2 sequential, Phase 3 parallel

---

### PHASE 1: GPU & VLLM SETUP (Steps 1-16)

**Goal**: ROCm drivers installed, vLLM running, model loaded.
**Time**: 3 hours
**Agent**: Coordinator [S]

- [ ] **Step 1** [S][B]: Verify AMD GPU
  ```bash
  lspci | grep -i amd && rocminfo 2>/dev/null | head -20
  ```
- [ ] **Step 2** [V]: RX 7700 XT detected
  - If ROCm not installed → Step 3
- [ ] **Step 3** [S][D]: Install ROCm
  ```bash
  sudo apt-get install -y rocm-dev rocm-libs
  ```
- [ ] **Step 4** [V]: ROCm operational: `rocm-smi`
- [ ] **Step 5** [C]: GPU ready

- [ ] **Step 6** [B]: Install vLLM with ROCm backend
  ```bash
  pip install vllm --break-system-packages
  ```
- [ ] **Step 7** [D]: If vLLM ROCm fails, fallback to llama.cpp with ROCm
- [ ] **Step 8** [V]: vLLM installed

- [ ] **Step 9** [B]: Download DeepSeek-R1 7B distilled (fits 12GB VRAM)
  ```bash
  huggingface-cli download deepseek-ai/DeepSeek-R1-Distill-Qwen-7B --local-dir /models/deepseek-r1-7b
  ```
- [ ] **Step 10** [V]: Model downloaded
- [ ] **Step 11** [B]: Start vLLM server
  ```bash
  python -m vllm.entrypoints.openai.api_server \
    --model /models/deepseek-r1-7b \
    --port 20100 \
    --max-model-len 4096 &
  ```
- [ ] **Step 12** [V]: vLLM serving on port 20100
  ```bash
  curl -sf http://localhost:20100/v1/models | python3 -m json.tool
  ```
- [ ] **Step 13** [B]: Test inference
  ```bash
  curl http://localhost:20100/v1/completions -H "Content-Type: application/json" \
    -d '{"model":"deepseek-r1-7b","prompt":"Hello, I am","max_tokens":20}'
  ```
- [ ] **Step 14** [V]: Inference response received
- [ ] **Step 15** [C]: AI model serving!
- [ ] **Step 16** [V]: **PHASE 1 EXIT GATE** — vLLM serving DeepSeek-R1 7B on RX 7700 XT

---

### PHASE 2: UNHEADED INTEGRATION (Steps 17-30)

**Goal**: AI registered as service, traced by eBPF, visible in dashboard.
**Time**: 2 hours
**Agent**: Agent

- [ ] **Step 17** [B]: Register vLLM in service discovery
  ```bash
  curl -X POST http://bare-metal:18000/api/v1/services/register \
    -d '{"name":"vllm-deepseek","port":20100,"proto":"http","health":"/health"}'
  ```
- [ ] **Step 18** [V]: vLLM registered in Wotan service registry
- [ ] **Step 19** [W]: Create Sophia dictionary entry for AI model capabilities
- [ ] **Step 20** [V]: Sophia knows about vLLM-deepseek

#### eBPF Tracing of Inference
- [ ] **Step 21** [S][B]: Attach packet_marker to gaming desktop network interface
- [ ] **Step 22** [B]: Send inference request through traced path
- [ ] **Step 23** [V]: Inference request visible in trace-collector
- [ ] **Step 24** [V]: Dashboard shows AI inference traffic in Packet Flow tab
- [ ] **Step 25** [C]: Commit: `feat(ai): eBPF-traced AI inference`

#### Dashboard AI Tab
- [ ] **Step 26** [B]: Wire dashboard AI Inference tab to real vLLM metrics
  ```bash
  curl -sf http://gaming-desktop:20100/metrics | head -20
  ```
- [ ] **Step 27** [W]: Update dashboard AI tab to pull real inference latency, throughput, error rates
- [ ] **Step 28** [V]: AI tab showing real metrics
- [ ] **Step 29** [C]: Commit: `feat(dashboard): real AI inference metrics`
- [ ] **Step 30** [V]: **PHASE 2 EXIT GATE** — AI model registered, traced, dashboard showing real AI metrics

---

### PHASE 3: DOCUMENTATION & HANDOFF (Steps 31-38)

**Goal**: Document AI stack architecture and prepare for EVPN-VXLAN.
**Time**: 1 hour
**Agent**: Agent [P]

- [ ] **Step 31** [W]: Write `docs/architecture/AI_STACK.md`
- [ ] **Step 32** [W]: Update `references/timeline.md` — AI milestone
- [ ] **Step 33** [W]: Write `docs/sessions/S57-handoff.md`
- [ ] **Step 34** [B]: Full test suite: `go test ./... -race`
- [ ] **Step 35** [V]: ALL PASS
- [ ] **Step 36** [C]: Final commit
- [ ] **Step 37** [V]: Git clean
- [ ] **Step 38** [V]: **S57 EXIT GATE** — Sophia's Eye open. AI model serving, eBPF-traced, dashboard-visible, documented.

---

*S57 Battle Plan — Forged 2026-02-25*
*3 Phases. 38 Steps. Sophia's Eye opens. The first "head" sees through the armor.*

---

# S58: THE BRIDGE OF SIGHS — EVPN-VXLAN Two-Host Integration
## 3 Phases, ~40 Steps

**Date**: Target 2026-03-10
**Sprint**: S58 — EVPN-VXLAN tunnel between gaming desktop and bare metal, Wotan bridges both
**Prerequisite**: S57 EXIT GATE passed. Both hosts operational.
**Target**: Services on bare metal communicate with AI on gaming desktop over VXLAN overlay.
**Estimated Duration**: 6-8 hours
**Agent Strategy**: All phases sequential (network builds on prior)

---

### PHASE 1: VXLAN TUNNEL (Steps 1-16)

**Goal**: Layer 2 VXLAN tunnel between the two hosts.
**Time**: 2 hours
**Agent**: Coordinator [S]

- [ ] **Step 1** [B]: Verify LAN connectivity between hosts
  ```bash
  ping -c 5 <gaming-desktop-ip> && ping -c 5 <bare-metal-ip>
  ```
- [ ] **Step 2** [V]: Both hosts reachable
- [ ] **Step 3** [S][B]: Create VXLAN interface on bare metal
  ```bash
  sudo ip link add vxlan100 type vxlan id 100 local <bare-metal-ip> remote <gaming-ip> dstport 4789 dev eth0
  sudo ip link set vxlan100 up
  sudo ip addr add 10.100.0.1/24 dev vxlan100
  ```
- [ ] **Step 4** [S][B]: Create VXLAN interface on gaming desktop
  ```bash
  sudo ip link add vxlan100 type vxlan id 100 local <gaming-ip> remote <bare-metal-ip> dstport 4789 dev eth0
  sudo ip link set vxlan100 up
  sudo ip addr add 10.100.0.2/24 dev vxlan100
  ```
- [ ] **Step 5** [V]: Ping over VXLAN: `ping 10.100.0.2` from bare metal
- [ ] **Step 6** [C]: VXLAN tunnel operational

#### IPv6 over VXLAN
- [ ] **Step 7** [S][B]: Add IPv6 addresses on VXLAN
  ```bash
  # Bare metal
  sudo ip -6 addr add fd00:unheaded::1/64 dev vxlan100
  # Gaming desktop
  sudo ip -6 addr add fd00:unheaded::2/64 dev vxlan100
  ```
- [ ] **Step 8** [V]: IPv6 ping over VXLAN: `ping6 fd00:unheaded::2`
- [ ] **Step 9** [C]: IPv6 over VXLAN operational

#### FRRouting (Optional — BGP underlay)
- [ ] **Step 10** [S][B]: Install FRRouting if IS-IS/BGP underlay desired
  ```bash
  sudo apt-get install -y frr || echo "FRR not available — static routing fallback"
  ```
- [ ] **Step 11** [D]: If FRR not available, use static routes (sufficient for 2-host)
- [ ] **Step 12** [S][B]: Configure static routes for service discovery across hosts
- [ ] **Step 13** [V]: Services on bare metal reachable from gaming desktop via VXLAN
- [ ] **Step 14** [V]: AI on gaming desktop reachable from bare metal via VXLAN
- [ ] **Step 15** [C]: Cross-host routing operational
- [ ] **Step 16** [V]: **PHASE 1 EXIT GATE** — VXLAN tunnel up, IPv6 operational, cross-host routing working

---

### PHASE 2: WOTAN BRIDGE (Steps 17-30)

**Goal**: Wotan on bare metal communicates with services on gaming desktop.
**Time**: 2 hours
**Agent**: Coordinator

- [ ] **Step 17** [B]: Configure Wotan to listen on VXLAN interface
  ```bash
  wotan --grpc-addr=0.0.0.0:18001 --http-addr=0.0.0.0:18000
  ```
- [ ] **Step 18** [V]: Wotan reachable from gaming desktop: `curl http://fd00:unheaded::1:18000/health`
- [ ] **Step 19** [B]: Configure gaming desktop services to connect to Wotan over VXLAN
- [ ] **Step 20** [V]: vLLM service registered in Wotan from gaming desktop
- [ ] **Step 21** [C]: Wotan bridging both hosts

#### eBPF Cross-Host Tracing
- [ ] **Step 22** [S][B]: Attach packet_marker to VXLAN interface on bare metal
- [ ] **Step 23** [B]: Send inference request from bare metal to gaming desktop AI
  ```bash
  curl http://fd00:unheaded::2:20100/v1/completions -d '{"prompt":"test","max_tokens":5}'
  ```
- [ ] **Step 24** [V]: Inference request traced across VXLAN tunnel in dashboard
- [ ] **Step 25** [C]: Cross-host eBPF tracing working!

#### Split-Brain Architecture
- [ ] **Step 26** [V]: Bare metal runs: daemon, Wotan, dashboard, gateway, trace-collector, core services
- [ ] **Step 27** [V]: Gaming desktop runs: vLLM, AI models, DOOM (when ring active)
- [ ] **Step 28** [V]: Both connected via Wotan over EVPN-VXLAN
- [ ] **Step 29** [V]: Dashboard shows unified view of all services across both hosts
- [ ] **Step 30** [V]: **PHASE 2 EXIT GATE** — Wotan bridges both hosts, eBPF traces cross-VXLAN traffic

---

### PHASE 3: DOCUMENTATION (Steps 31-40)

**Goal**: Document two-host architecture.
**Time**: 1 hour
**Agent**: Agent [P]

- [ ] **Step 31** [W]: Write `docs/architecture/NETWORK_OVERLAY.md`
- [ ] **Step 32** [W]: Write `docs/architecture/SPLIT_BRAIN.md`
- [ ] **Step 33** [W]: Update `references/timeline.md`
- [ ] **Step 34** [W]: Write `docs/sessions/S58-handoff.md`
- [ ] **Step 35** [B]: Full test suite
- [ ] **Step 36** [V]: ALL PASS
- [ ] **Step 37** [C]: Final commit
- [ ] **Step 38** [W]: Network diagram: bare metal ↔ VXLAN ↔ gaming desktop
- [ ] **Step 39** [V]: All docs committed
- [ ] **Step 40** [V]: **S58 EXIT GATE** — EVPN-VXLAN operational, Wotan bridging, cross-host tracing, split-brain architecture documented.

---

*S58 Battle Plan — Forged 2026-02-25*
*3 Phases. 40 Steps. The Bridge of Sighs connects two worlds.*

---

# S59: THE ARMORER'S FINAL TOUCH — Dashboard Polish Sprint
## 4 Phases, ~50 Steps

**Date**: Target 2026-03-16
**Sprint**: S59 — Design system unification, Kanban drag-drop, all 8 tabs perfect
**Prerequisite**: S58 EXIT GATE passed (or S55 minimum — can polish with mock data if hardware blocked).
**Target**: Dashboard that makes a human say "this looks professional."
**Estimated Duration**: 8-10 hours
**Agent Strategy**: Phases 1-4 parallelizable (independent UI components)

---

### PHASE 1: DESIGN SYSTEM UNIFICATION (Steps 1-14)

**Goal**: Unified CSS design system across all dashboard pages.
**Time**: 2 hours
**Agent**: Agent [P]

- [ ] **Step 1** [W]: Create `dashboard/css/design-system.css` with unheaded.org aesthetic:
  - `--bg-primary: #0a0a0a`, `--text-primary: #c9c9c9`, zero saturation
  - JetBrains Mono + Space Grotesk typography
  - Spacing scale, border/radius/shadow tokens
- [ ] **Step 2** [W]: Self-host fonts in `dashboard/assets/fonts/`
- [ ] **Step 3** [W]: Refactor `dashboard/css/style.css` to import design-system.css
- [ ] **Step 4** [W]: Update nav bar — frosted glass, clean links, no emoji
- [ ] **Step 5** [W]: Update tab navigation — clean buttons, proper active state
- [ ] **Step 6** [W]: Update metric cards — bg-card, border, radius, hover
- [ ] **Step 7** [V]: Dashboard renders with unified design system
- [ ] **Step 8** [C]: Commit: `feat(ui): unified design system — unheaded.org aesthetic`

#### Apply to All Pages
- [ ] **Step 9** [W]: Apply design system to kanban/index.html
- [ ] **Step 10** [W]: Apply design system to doom.html
- [ ] **Step 11** [W]: Apply design system to logs.html (if standalone)
- [ ] **Step 12** [V]: All pages use same design tokens
- [ ] **Step 13** [C]: Commit: `feat(ui): design system applied to all pages`
- [ ] **Step 14** [V]: **PHASE 1 EXIT GATE** — Unified design system, all pages consistent

---

### PHASE 2: KANBAN POLISH (Steps 15-28)

**Goal**: Kanban with drag-drop, review actions, professional appearance.
**Time**: 2.5 hours
**Agent**: Agent [P]

- [ ] **Step 15** [R]: Identify canonical kanban implementation (unify if two exist)
- [ ] **Step 16** [W]: Implement HTML5 Drag API for column-to-column card movement
- [ ] **Step 17** [W]: On drop: update task status via API, publish Wotan event
- [ ] **Step 18** [V]: Drag-drop moves cards between columns
- [ ] **Step 19** [C]: Commit: `feat(kanban): drag-and-drop between columns`

- [ ] **Step 20** [W]: Add "Review" column with special actions:
  - Approve → moves to Done + publishes approval event
  - Reject → moves to In Progress + publishes rejection event
  - Request Changes → stays in Review + publishes changes event
- [ ] **Step 21** [V]: Review actions trigger correct status transitions
- [ ] **Step 22** [C]: Commit: `feat(kanban): review column with approve/reject/changes actions`

- [ ] **Step 23** [W]: Add proper empty states for all columns
- [ ] **Step 24** [W]: Add card creation form (inline, not modal)
- [ ] **Step 25** [W]: Fix WebSocket reconnection if broken
- [ ] **Step 26** [V]: Full CRUD: create, read, drag, review, delete
- [ ] **Step 27** [C]: Commit: `feat(kanban): full CRUD with real-time updates`
- [ ] **Step 28** [V]: **PHASE 2 EXIT GATE** — Kanban with drag-drop, review actions, real-time, Meta Moment verified

---

### PHASE 3: TAB PERFECTION (Steps 29-42)

**Goal**: All 8 dashboard tabs functional with real or convincing demo data.
**Time**: 2 hours
**Agent**: Agent [P]

- [ ] **Step 29** [W]: Build demo data generator (`js/demo-data.js`) for when no backend running
- [ ] **Step 30** [W]: Wire demo data to Packet Flow tab (if no real traces available)
- [ ] **Step 31** [W]: Wire demo data to Trace Table tab
- [ ] **Step 32** [W]: Wire demo data to Latency Chart tab
- [ ] **Step 33** [W]: Verify DOOM tab works (gradient/checkerboard + real if available)
- [ ] **Step 34** [W]: Verify Services tab reads from configs/services.yaml
- [ ] **Step 35** [W]: Verify Infrastructure tab shows container/runtime status
- [ ] **Step 36** [W]: Integrate logs.html content into tab navigation if not done
- [ ] **Step 37** [W]: Verify AI Inference tab shows real or demo metrics
- [ ] **Step 38** [V]: ALL 8 TABS: render, have data, no console errors, proper empty states
- [ ] **Step 39** [C]: Commit: `feat(dashboard): all 8 tabs polished with demo data fallback`

#### Responsive Design
- [ ] **Step 40** [W]: Mobile responsive pass — hamburger menu, stacked cards
- [ ] **Step 41** [V]: Dashboard usable at 375px, 768px, 1024px, 1440px
- [ ] **Step 42** [V]: **PHASE 3 EXIT GATE** — All tabs perfect, responsive, zero console errors

---

### PHASE 4: FINAL VERIFICATION (Steps 43-50)

**Goal**: End-to-end UI verification.
**Time**: 1 hour
**Agent**: Coordinator

- [ ] **Step 43** [B]: Full test suite
  ```bash
  go test ./... -race 2>&1 | tail -10
  ```
- [ ] **Step 44** [V]: ALL PASS
- [ ] **Step 45** [B]: Open dashboard in Chrome — check DevTools console
- [ ] **Step 46** [V]: ZERO JavaScript errors
- [ ] **Step 47** [B]: Open dashboard in Firefox — check console
- [ ] **Step 48** [V]: ZERO JavaScript errors
- [ ] **Step 49** [C]: Commit: `milestone: dashboard polish complete`
- [ ] **Step 50** [V]: **S59 EXIT GATE** — Dashboard professional, all 8 tabs perfect, Kanban with drag-drop, responsive, zero console errors, unified design system.

---

*S59 Battle Plan — Forged 2026-02-25*
*4 Phases. 50 Steps. The Armorer's final touch. Every pixel matters.*

---

# S60: THE GATE OPENS — Alpha Demo & Public Preparation
## 4 Phases, ~48 Steps

**Date**: Target 2026-03-22
**Sprint**: S60 — Demo video, public landing page, README for humans, conference outline
**Prerequisite**: S59 EXIT GATE passed. Dashboard polished. Traces working. DOOM running. AI serving.
**Target**: Alpha demo recorded. www.unheaded.org enhanced. README rewritten. Gate OPEN.
**Estimated Duration**: 6-8 hours
**Agent Strategy**: Phases 1-3 parallelizable, Phase 4 sequential

---

### PHASE 1: DEMO VIDEO SCRIPT & RECORDING (Steps 1-16)

**Goal**: 5-minute demo video showing the full Unheaded experience.
**Time**: 3 hours
**Agent**: Coordinator

- [ ] **Step 1** [W]: Write demo script — `docs/demos/ALPHA_DEMO_SCRIPT.md`:
  ```
  0:00-0:30  — Open www.unheaded.org. "War makes states. States make war."
  0:30-1:00  — Navigate to dashboard. Overview of the platform.
  1:00-2:00  — Packet Flow tab: real eBPF traces of live HTTP traffic.
  2:00-2:30  — Latency Chart: P50/P95/P99 from kernel-level measurement.
  2:30-3:00  — Services tab: 25 services, all healthy, all on Doom Range ports.
  3:00-3:30  — AI tab: DeepSeek-R1 serving inference, eBPF-traced.
  3:30-4:00  — DOOM tab: live gameplay over IPv6 packet ring. "Packets as CPU."
  4:00-4:30  — Kanban tab: "This board tracks the project that built the platform
               that runs this board." The Meta Moment.
  4:30-5:00  — Terminal: `unheaded status` — all systems green. "You bring the head.
               We provide everything else."
  ```
- [ ] **Step 2** [V]: Script reviewed by Captain for narrative flow
- [ ] **Step 3** [B]: Verify all demo components running
- [ ] **Step 4** [B]: Record demo (screen capture + voice if desired)
- [ ] **Step 5** [V]: Demo video captured
- [ ] **Step 6** [W]: Export to `/opt/unheaded/demos/alpha-demo.mp4`
- [ ] **Step 7** [C]: Demo script committed

#### Alternate: GIF/Screenshot Series (if video tools unavailable)
- [ ] **Step 8** [B]: Screenshot each dashboard tab
- [ ] **Step 9** [B]: Screenshot DOOM gameplay
- [ ] **Step 10** [B]: Screenshot terminal output
- [ ] **Step 11** [W]: Create `docs/demos/SCREENSHOTS.md` with annotated images
- [ ] **Step 12** [C]: Screenshots committed

#### Conference Talk Outline
- [ ] **Step 13** [W]: Write `docs/conference/TALK_OUTLINE.md`:
  - Title: "Doom in the Data Plane: Running a Game Engine Inside eBPF"
  - Abstract: 200 words
  - Outline: 12-15 slides
  - Target: IETF hackathon / BSides / local meetup
- [ ] **Step 14** [V]: Talk outline covers technical depth + narrative arc
- [ ] **Step 15** [C]: Talk outline committed
- [ ] **Step 16** [V]: **PHASE 1 EXIT GATE** — Demo captured, talk outlined

---

### PHASE 2: PUBLIC LANDING PAGE (Steps 17-28)

**Goal**: www.unheaded.org enhanced with feature highlights and dashboard links.
**Time**: 2 hours
**Agent**: Agent [P]

- [ ] **Step 17** [R]: Review existing www_unheaded_org source
- [ ] **Step 18** [W]: Add "What is Unheaded?" section:
  - "You bring the application. We provide the infrastructure."
  - Key capabilities: eBPF tracing, 25 services, IaC backends, zero data access
- [ ] **Step 19** [W]: Add feature highlights section:
  - Packet tracing (eBPF + dashboard)
  - Computational completeness (DOOM on the UPC)
  - AI integration (first "head" in armor)
  - Self-hosting proof (Meta Moment)
- [ ] **Step 20** [W]: Add architecture diagram (SVG or ASCII)
- [ ] **Step 21** [W]: Add links to dashboard, protocol API docs, wiki
- [ ] **Step 22** [V]: Page responsive, fast-loading, no broken links
- [ ] **Step 23** [C]: Commit: `feat(www): enhanced public landing page`

- [ ] **Step 24** [W]: Add protocol overview section linking to 3 Internet-Drafts
- [ ] **Step 25** [W]: Add "The Doom Range" port allocation visual
- [ ] **Step 26** [V]: All content accurate, no stale claims
- [ ] **Step 27** [C]: Commit: `feat(www): protocol overview and Doom Range visual`
- [ ] **Step 28** [V]: **PHASE 2 EXIT GATE** — www.unheaded.org enhanced, professional, accurate

---

### PHASE 3: README & DOCS FOR HUMANS (Steps 29-40)

**Goal**: README.md rewritten for external humans. Docs cleaned and accurate.
**Time**: 2 hours
**Agent**: Agent [P]

- [ ] **Step 29** [W]: Rewrite `README.md` for external audience:
  - Quick explanation (3 sentences)
  - Feature list (not internal jargon)
  - Quick start guide
  - Architecture diagram
  - LOC counts and test status badges
  - Contributing link
  - License badge (BSL 1.1)
- [ ] **Step 30** [V]: README readable by someone who's never heard of Unheaded
- [ ] **Step 31** [C]: Commit: `docs: README rewrite for external audience`

- [ ] **Step 32** [W]: Rewrite `docs/VISION.md` — reflect current state, not Jan 2026 hopes
- [ ] **Step 33** [W]: Update `CLAUDE.md` with S51-S60 changes
- [ ] **Step 34** [W]: Update `references/timeline.md` — mark Alpha Gate milestones
- [ ] **Step 35** [C]: Commit: `docs: VISION rewrite and timeline update`

#### Wiki Sync
- [ ] **Step 36** [B]: Verify all wiki pages current
  ```bash
  ls docs/wiki/ | wc -l
  ```
- [ ] **Step 37** [W]: Update stale wiki pages (any referencing pre-S36 architecture)
- [ ] **Step 38** [V]: Wiki internally consistent
- [ ] **Step 39** [C]: Commit: `docs: wiki sync for Alpha Gate`
- [ ] **Step 40** [V]: **PHASE 3 EXIT GATE** — README, VISION, CLAUDE.md, timeline, wiki all current

---

### PHASE 4: ALPHA GATE FINAL VERIFICATION (Steps 41-48)

**Goal**: The Alpha Gate checklist. Everything verified. Everything green.
**Time**: 1 hour
**Agent**: Coordinator

- [ ] **Step 41** [V]: **SECURITY**: Zero P0s. SBOM generated. Auth framework scaffolded.
- [ ] **Step 42** [V]: **LEGAL**: LICENSE file. SPDX headers. THIRD_PARTY. IP inventory. Contributor guide.
- [ ] **Step 43** [V]: **INFRASTRUCTURE**: Bare metal running. eBPF verified. Kernel tuned.
- [ ] **Step 44** [V]: **PRODUCT**: First real packet trace. Dashboard shows live eBPF data.
- [ ] **Step 45** [V]: **PROOF**: DOOM playing in browser over IPv6 on bare metal.
- [ ] **Step 46** [V]: **AI**: Sophia's Eye serving. eBPF-traced. Dashboard-visible.
- [ ] **Step 47** [V]: **NETWORK**: EVPN-VXLAN bridging two hosts. Wotan spanning both.
- [ ] **Step 48** [V]: **S60 EXIT GATE — THE ALPHA GATE OPENS**:
  - All tests pass
  - All services healthy
  - Dashboard professional
  - Demo captured
  - README for humans
  - www.unheaded.org enhanced
  - Conference talk outlined
  - Legal protections in place
  - Security hardened
  - **THE KINGDOM IS READY FOR THE WORLD TO SEE.**

---

*S60 Battle Plan — Forged 2026-02-25*
*4 Phases. 48 Steps. The Gate Opens. The Kingdom is revealed.*
*"You bring the head. We provide everything else."*

---

## APPENDIX A: EMERGENCY PROCEDURES

### Symptom: eBPF verifier rejects production program
```bash
# 1. Get verbose verifier log
sudo bpftool prog load program.o /sys/fs/bpf/test type xdp 2>&1 | tee /tmp/verifier.log
# 2. Check for unbounded loops (add explicit bounds)
# 3. Check for stack overflow (reduce local variables)
# 4. Check for uninitialized map access (add null checks)
# 5. If stuck: reduce program complexity, split into two programs
```

### Symptom: VXLAN tunnel not passing traffic
```bash
# 1. Check underlay connectivity
ping <remote-host-ip>
# 2. Check VXLAN interface state
ip link show vxlan100
# 3. Check FDB entries
bridge fdb show dev vxlan100
# 4. Check firewall (UDP 4789 must be open)
sudo iptables -L -n | grep 4789
# 5. tcpdump on underlay: sudo tcpdump -i eth0 port 4789
```

### Symptom: vLLM OOM on RX 7700 XT
```bash
# 1. Check VRAM usage
rocm-smi --showmeminfo vram
# 2. Reduce max_model_len: --max-model-len 2048
# 3. Use quantized model: GPTQ or AWQ 4-bit
# 4. Fallback: llama.cpp with --n-gpu-layers partial offload
```

### Symptom: Service won't register with Wotan over VXLAN
```bash
# 1. Check Wotan reachable from gaming desktop
curl http://fd00:unheaded::1:18000/health
# 2. Check firewall rules on bare metal
sudo iptables -L -n | grep 18000
# 3. Check Wotan listening on all interfaces (not just localhost)
ss -tlnp | grep 18000
# 4. Check routing: ip -6 route show
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Session | Primary Agent | Phase Count | Steps | Est. Hours | Dependencies |
|---------|--------------|-------------|-------|------------|--------------|
| S51 Security | Moat Ghost + Developer | 3 | 45 | 4-6 | S50 |
| S52 Legal | Barrister + Librarian | 3 | 38 | 3-4 | S51 |
| S53 Bare Metal | Architect + Developer | 4 | 52 | 6-8 | S52 + hardware |
| S54 eBPF | Developer + Architect | 4 | 55 | 6-8 | S53 |
| S55 First Trace | Developer + Architect | 3 | 42 | 8-10 | S54 |
| S56 DOOM | Developer | 3 | 40 | 6-8 | S55 |
| S57 AI Stack | Architect + Developer | 3 | 38 | 6-8 | S56 + GPU hardware |
| S58 VXLAN | Architect + Developer | 3 | 40 | 6-8 | S57 + 2 hosts |
| S59 Polish | Developer | 4 | 50 | 8-10 | S55 minimum |
| S60 Gate Opens | Captain + Librarian | 4 | 48 | 6-8 | S59 |
| **TOTAL** | | **34** | **448** | **60-78** | |

**Critical Path**: S51 → S52 → S53 → S54 → S55 → S56 → S57 → S58 → S59 → S60

**Parallelization Note**: S59 (Polish) can begin after S55 if hardware isn't ready for S56-S58. Use mock/demo data for dashboard polish.

**Fallback Path** (if hardware delayed):
```
S51 → S52 → S59 (polish with demo data) → S53 → S54 → S55 → S56 → S57 → S58 → S60
```

---

## APPENDIX C: QUICK REFERENCE

### Port Registry — The Doom Range
```
16666  doom-bridge         18000  wotan HTTP          20000  dashboard
16667  doom-go-injector    18001  wotan gRPC          20001  kanban
16670  trace-collector     19000  timeguru            20002  wiki
16671  trace-metrics       19001  architect           20100  vllm-deepseek
17000  daemon HTTP         19002  captain             20101  vllm-qwen
17001  daemon gRPC         19003  micromanager        20102  qdrant
17010  cli-server          19004  monad               20104  bge-m3
                           19005  sophia              20105  sophia-eye
21000  gateway HTTP                                   20106  ai-webui
21443  gateway HTTPS
```

### eBPF Program Types
```
packet_marker  → XDP (fastest, pre-routing)
flow_tracker   → TC (traffic control, post-routing)
latency_probe  → kprobe (kernel function tracing)
monad-cpu-ebpf → XDP (Doom computational engine)
```

### VXLAN Addressing
```
Bare metal:     10.100.0.1  / fd00:unheaded::1
Gaming desktop: 10.100.0.2  / fd00:unheaded::2
VNI: 100, UDP port: 4789
```

### Key File Locations
```
/opt/unheaded/src/          — source code
/sys/fs/bpf/unheaded/       — pinned BPF programs and maps
/opt/unheaded/configs/       — service YAML configs
/etc/systemd/system/         — systemd unit files
```

---

## Decisions Made at This Round Table

1. **10-session campaign "The Alpha Gate"** — sequential sessions from security → legal → bare metal → eBPF → traces → DOOM → AI → network → polish → demo. **APPROVED.**
2. **Session naming**: Mythological names following three-pillar conventions. **APPROVED.**
3. **Fallback path**: If hardware delays, polish sprint (S59) moves up. **APPROVED.**
4. **Alpha Gate target**: March 22, 2026. Confidence: Medium-High. **ACKNOWLEDGED.**
5. **Design system**: unheaded.org dark aesthetic (#0a0a0a, zero saturation, monospace). **CONFIRMED (D5).**
6. **AI model**: DeepSeek-R1 7B distilled as first model (fits 12GB VRAM). **APPROVED.**
7. **Network overlay**: Simple VXLAN first, FRRouting/BGP if needed later. **APPROVED.**
8. **Auth framework**: Scaffold in S51, full implementation post-Alpha. **APPROVED.**

## Post-Campaign Directives — Muck's Far-Future Mandates (Binding)

**D8 — SPAN/ERSPAN Collector/Aggregator Sink Architecture**: Port mirror/SPAN on virtual Linux switches to mirror eBPF metrics and headers to a collector/aggregator sink cluster. The sink cluster round-robins incoming time series data to clients that buffer and write to long-term storage. SPAN can be applied or popped at container/node/host ingress or egress. Implementation uses **native Linux kernel port mirroring** (available since ~kernel 4.14) — NO proprietary vendor tools (no Cumulus NVUE, no NVIDIA anything). Pure kernel `tc` + `mirred` action for local SPAN, and kernel-native ERSPAN tunnel interfaces via `iproute2` for remote mirroring (IPv4 ERSPAN kernel 4.14+, IPv6 ERSPAN kernel 4.16+):
```bash
# Local SPAN — mirror veth0 ingress to collector interface
tc qdisc add dev veth0 ingress
tc filter add dev veth0 ingress matchall action mirred egress mirror dev collector0

# ERSPAN — remote mirror across EVPN-VXLAN fabric
ip link add erspan1 type erspan local fd00:unheaded::1 remote fd00:unheaded::2 \
  seq key 100 erspan_ver 2 erspan_dir ingress erspan_hwid 4
ip link set erspan1 up

# Per-container — apply at veth pair egress
tc qdisc add dev veth-container0 clsact
tc filter add dev veth-container0 egress matchall action mirred egress mirror dev erspan1
```
This creates a **passive observability plane** that operates independently of the eBPF active tracing — defense in depth for telemetry. Can be selectively applied per-container, per-node, or per-host at ingress/egress with `tc` filter matching. **Target: Age 2+ (Beta Trials).**

**D9 — Scientist Stress Test Tuning Plans**: The Scientist MUST formulate comprehensive stress test and tuning plans for ALL services through the Kingdom. Methodology: identify bottlenecks, find poor code paths, measure under load, tune kernel parameters, optimize hot paths, and validate with statistical rigor. Each service gets a lab notebook entry with: (1) baseline performance profile, (2) load test at 10x/100x/1000x expected traffic, (3) bottleneck identification via flame graphs + eBPF tracing, (4) tuning recommendations, (5) regression test suite. The Scientist works with Developer (fixes), Architect (infrastructure tuning), and BlackMage (adversarial load patterns). **Target: Post-Alpha Gate, Pre-Beta.**

**D10 — IPv6 Header Metric Stuffing Research**: Determine how much gRPC-inspired Unheaded metrics can be packed into IPv6 headers. **SCIENTIST HAS COMPLETED INITIAL ANALYSIS (see `docs/research/IPV6_METRIC_CAPACITY.md`).** Key findings:
- **Theoretical maximum**: 6,124 bytes across Flow Label + HbH + 2x Destination Options
- **Practical internet-viable**: 382 bytes (conservative HbH)
- **Recommended sweet spot**: 102.5 bytes (Monad + Metrics option + Flow Label encoding)
- **Proposed wire format**: UNHEADED_METRIC_V1 (Type 0x2A) — 52-64 byte HbH option carrying Trace ID, Span ID, Timestamp, Latency, Service ID, Status, Hop Count, Custom Metrics
- **Flow Label encoding**: 20-bit packed format — Service ID (4b) + Status (4b) + Latency Bucket (8b) + Flags (4b)
- **RFC compliance**: FULL across RFC 8200, 6437, 3168, 2474, 7045
- **Next step**: RFC Editor drafts `draft-bellis-unheaded-metric-header-00` defining Type 0x2A
**Target: S54+ (after eBPF verified, before Beta).**

---

## Open Questions (Carry Forward)

1. **Conference target**: Which conference? When? — Captain to decide by S60.
2. **VC outreach**: Austin VCs — Captain + Barrister to research during campaign.
3. **Public launch trigger**: "Alpha Gate passed" = public? Or additional gate? — Muck decides.
4. **IANA HbH registration**: When to submit? Post-Alpha or after more implementation? — RFC Editor advises.
5. **Multi-host discovery**: DNS or BGP advertisement for service discovery? — Architect to design post-S58.

## Wins to Celebrate

- **50 SESSIONS FROM FIRST COMMIT** — 260K production LOC. 464K with tests. One engineer. One AI. One Kingdom.
- **32 COMMITS IN S45-S50** — Autonomous sprint model proven again and again.
- **8-TAB DASHBOARD** — Metrics, Packet Flow, Trace Table, Latency, DOOM, Services, Infrastructure, AI Inference.
- **PROTOCOL API SCAFFOLDED** — 7 cross-service integration tests. MockBackend + BPFBackend.
- **AI STACK DESIGNED** — Ports allocated, services registered, dashboard tab built. Sophia's Eye awaits hardware.
- **DOOM KEYBOARD FIXED** — Binary wire protocol, KeyStateBitmap, multi-key support. THE GAME IS PLAYABLE.
- **158+ PACKAGES PASS. ZERO RACE CONDITIONS.** — The codebase is HEALTHY.
- **THE PROTOCOL IS THE MOAT. THE GATE IS IN SIGHT. LET'S GO.**

---

### Next Round Table

**Scheduled**: After S55 (First Light) OR after S60 (Gate Opens)
**Reason**: Mid-campaign review at First Light milestone, or full review at Alpha Gate.
**Trigger**: Also convene if any EXIT GATE fails 3 consecutive attempts.

---

_Forged at the Round Table by all 9 minds plus Warmonger, BlackMage, Moat Ghost, Scientist, RFC Editor, Barrister, and Librarian._
_51 sessions from first commit. The Alpha Gate Campaign begins._
_10 sessions. 448 steps. 34 phases. 60-78 hours. One Kingdom._
_"You bring the head. We provide everything else."_
_THE KINGDOM MARCHES AS ONE. THE GATE IS IN SIGHT. LET'S GOOOO._
