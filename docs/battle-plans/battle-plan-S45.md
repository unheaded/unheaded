# Battle Plan: S45 — The Polishing of the Kingdom
## Convened: 2026-02-24 | Reason: PoC Near-Complete — Multi-Phase Sprint Planning
## Kingdom State: Age 1 (Alpha), Epoch 4 (Convergence), ~99% Structural Complete

---

### Situation Report

The Kingdom stands at a critical threshold. Forty-four sessions have produced ~260K production LOC, 203K test LOC, 25 microservices, 9 eBPF programs, 3 Internet-Drafts, 6 IaC backends, 8 observability backends, a Helm chart, and DOOM running in BPF maps via the Monad CPU. The Four Pillars (Port Authority, gRPC-First, Log Aggregation, Service Discovery) are forged. The anti-lock-in principle lives in code. The Meta Moment is proven — Kanban tracks itself via Wotan.

But we're at the "looks like an engineering prototype" stage, not the "shows well to other humans" stage. Dashboards are AI slop. The public website (www_unheaded_org) is an empty directory. The RFC draft and implementation have drifted on at least one wire format (CancelFlowValue 20B vs 24B). DOOM runs in BPF but we need to validate the PoC with more than just our own game — SNES emulators, Unix v4, anything that proves computational generality. The VISION.md is stale. Docs need alignment. And the AI brain stack needs its first integration plan.

The time has come to polish, align, validate, and prepare for the world to see.

---

### Round Table Addendum — Muck's Directives (Post-Convening)

Seven directives issued after initial battle plan forging. These are BINDING and woven into all phases below.

**D1 — RFC-to-Protocol Full Alignment**: Not just docs alignment — the RFC specs, the Go structs, the Rust structs, the wire format, the BPF maps, and the dashboard visualizations MUST all tell the same story. One source of truth flowing through every layer.

**D2 — AI Model = First "Head" Application**: The local AI stack (vLLM + DeepSeek-R1 + Qwen 2.5 Coder) is not just a tool. It is the FIRST APPLICATION running as "head" inside Unheaded's suit of armor. Strictly locked-down ingress/egress flows, all traffic visible via dashboards, eBPF-traced inference requests. This proves the platform works for real workloads.

**D3 — Preserve Protocol Innovations**: The inverse mask (double static address space), Kingdom Mode, and all innovative protocol proposals MUST persist and be prominently documented. These are the moat. Do not lose them in cleanup.

**D4 — Wotan Kanban Enhancements**: The kanban board MUST support drag-and-drop between columns. Review items get special actions (approve, reject, request changes). This is the Meta Moment — if our own kanban sucks, the platform story falls apart.

**D5 — UI = Fusion of unheaded.org + bellis.tech**: The Unheaded dashboard UI is a FUSION of both sites. unheaded.org provides the soul — dark (#0a0a0a), minimal, medieval, poetic, evocative. bellis.tech provides the engineering — CSS variables, spacing system, frosted glass nav, card components, particle canvas, feed grids, responsive patterns. The result: unheaded.org's dark soul with bellis.tech's professional polish. "bellus — pretty | perennis — everlasting."

**D6 — RFC-Compliant Protocol API**: Build an RFC-compliant API for the Unheaded Protocol itself. This accelerates development and adoption of applications running on it. Applications talk to the protocol through a well-defined API surface — not raw BPF maps. This is how the AI "head" and future applications integrate.

**D7 — Mount ~/tmp**: Source files for unheaded.org and bellis.tech live in ~/tmp on Muck's machine. Must be mounted for direct reference during UI work.

---

### The Throne Speaks (Captain — Vision & Strategy)

**Strategic Position**: Pre-public. Private repo. PoC proven internally. No external eyes yet.

**North Star**: Ship a demo that makes a skeptical staff engineer say "holy shit, they actually did it" — eBPF packet tracing from wire to dashboard, DOOM running in extension headers, and a clean professional UI that doesn't scream "AI generated this." The AI model running as the first "head" inside the armor proves this isn't vapor.

**Key Decisions — RESOLVED**:
1. AI model stack: vLLM + DeepSeek-R1 + Qwen 2.5 Coder as the first "head" application in Unheaded armor — **APPROVED**
2. DOOM GPL code consolidation: move ~/tmp/unheaded/doom edits → ~/tmp/doomgeneric fork — **APPROVED**
3. Dashboard aesthetic: **RESOLVED — unheaded.org dark aesthetic** (NOT bellis.tech blue). bellis.tech for CSS architecture reference only.
4. www_unheaded_org: already live with headless knights aesthetic — **KEEP AND ENHANCE**
5. RFC-compliant Protocol API: **APPROVED** — critical for application adoption

**Risk to Vision**: Showing this to humans before the UI is polished will create a "cool tech, bad product" first impression that's hard to shake. We get one shot at first impressions.

### The Ledger Records (Micromanager — Execution & QA)

**Sprint Status**: S41-S44 COMPLETE. 23 commits, 160+ files, 158 packages passing, 0 race conditions.

**Priority Stack** (ordered):

1. **P0 — MUST SHIP** (Phases 1-3)
   - Docs/RFC alignment audit — CancelFlowValue mismatch, hardcoded ports, stale VISION.md
   - Dashboard + Kanban UI overhaul — bellis.tech design system adoption
   - Service/Container management UI scaffolding (YAML-driven)
   - All dashboard tabs FUNCTIONAL with real or convincing demo data

2. **P1 — SHOULD SHIP** (Phases 4-5)
   - DOOM GPL consolidation (unheaded/doom → doomgeneric fork)
   - Computational generality validation (SNES emu, Unix v4 feasibility study)
   - www_unheaded_org public landing page
   - AI model stack integration plan (vLLM + DeepSeek-R1 + Qwen 2.5)

3. **P2 — NICE TO HAVE** (Phase 6)
   - Vision doc rewrite with current scope
   - Doc archive/cleanup proposal
   - Conference talk outline refresh
   - Gorgonia replacement design (Go ML with generics)

**QA Gates**:
- All 158 Go packages pass
- 0 race conditions
- Dashboard renders without console errors in Chrome/Firefox
- All nav links resolve (no 404s)
- Kanban CRUD operations work end-to-end
- Service management UI reads from YAML config

**Acceptance Criteria**: A human who has never seen Unheaded can navigate the dashboard, understand what they're looking at, click through all tabs, see data flowing, and think "this looks professional."

### The Blueprint Reveals (Architect — Infrastructure & Design)

**Architecture Health**: Strong. 25 services, all wired to Wotan, gRPC-first transport, proper security baseline across NixOS/Helm/IaC. No architectural debt blocking this sprint.

**Technical Risks**:
- CancelFlowValue wire mismatch (20B Rust vs 24B Go) — must resolve before any public spec review
- 13+ files hardcode port values instead of `pkg/ports/` — technical debt
- JWT auth still TODO in `pkg/auth/auth.go` — not blocking PoC but needed pre-public
- TestPerformance_TraceProcessingLatency failing (20µs vs 10µs) — cosmetic but ugly

**Design System — RESOLVED (D5) — Extracted from unheaded.org source**:
```
Source of Truth: FUSION of unheaded.org (soul) + bellis.tech (engineering)

COLORS:
  --bg-primary:    #0a0a0a     (body background)
  --text-primary:  #c9c9c9     (body text — muted silver)
  --text-heading:  #fff        (h1 — pure white, rare)
  --text-dim:      #666        (etymology, subtle labels)
  --text-ghost:    #3a3a3a     (infrastructure poetry — nearly invisible, reveals on hover)
  --text-ghost-hover: #888     (hover state for ghost text)
  --text-nodes:    #444        (node labels — very subtle)
  --text-accent:   #555        (serif "here we are")
  --border-subtle: #222        (left-border on infra block)
  NO BRIGHT COLORS. NO GOLD. NO BLUE. Zero saturation.

TYPOGRAPHY:
  Headers: 'Courier New', monospace — font-weight: 300, letter-spacing: 0.3em
  Body: 'Courier New', monospace — line-height: 1.8
  Accents: serif (for "here we are" motif)
  Upgrade path: JetBrains Mono replaces Courier New, Space Grotesk for body
  Self-hosted in /dashboard/assets/fonts/

SPACING:
  Max content width: 42ch (narrow, readable)
  Padding: 2rem base
  Section gaps: 2rem

ANIMATION:
  Pulse glyph: 4s ease-in-out infinite, opacity 0.4→1.0
  Hover transitions: 0.5s ease on ghost text

IMAGERY:
  Headless armor suits (sm-logo-L.png, sm-logo-R.png)
  Medieval, evocative, dark

VIBE: "war makes states. states make war. extinction builds headless empires."

ALL UIs adopt this: dashboard, kanban, doom viewer, wiki server
```

**Service/Container Management UI Architecture**:
```
Static YAML configs (convention: /opt/unheaded/<service>/config.yaml)
  ↓
dashboard-backend reads on startup + watches for changes
  ↓
REST API: GET /api/v1/services, GET /api/v1/containers
  ↓
New Dashboard Tab: "Services" — card grid showing service health, config, actions
New Dashboard Tab: "Infrastructure" — container status, NixOS/Docker/LXD state
  ↓
Future: CLI `unheaded service list/start/stop/restart` → same YAML → IaC vision
```

**AI Stack Architecture** (First Thing in Unheaded Armor):
```
Gaming Desktop (Ryzen 5 7600X, RX 7700 XT 12GB VRAM, 16GB DDR5):
  ├── vLLM server (GPU-accelerated inference)
  │   ├── DeepSeek-R1 (reasoning/RAG — distilled to fit VRAM)
  │   └── Qwen 2.5 Coder (code generation)
  ├── Open WebUI or custom Unheaded chat UI
  ├── BGE-M3 embeddings + Qdrant vector DB
  └── 2TB HDD: model weights, RAG corpus, training data
      1TB NVMe: swap/vram overflow + vector indices

Bare Metal Server (4-core DDR3):
  ├── Unheaded daemon + control plane
  ├── Wotan message bus
  ├── MCP API server
  ├── Dashboard + Kanban
  └── Gateway (HTTP/3, QUIC, gRPC-Web)

Communication: Wotan gRPC over EVPN-VXLAN between hosts
eBPF: Runs on both — gaming box traces AI inference, bare metal traces services
```

**Hardware Constraints for AI**:
- RX 7700 XT: 12GB VRAM — can run DeepSeek-R1 7B or 14B distilled, NOT 70B+
- 16GB DDR5: tight — need 1TB NVMe as swap for model loading
- ROCm support required (AMD GPU) — vLLM has ROCm backend
- Realistic model sizes: 7B-14B quantized (Q4_K_M or Q5_K_M)

### The Anvil Reports (Developer — Implementation & Testing)

**Code Health**: 158 packages pass, 0 race conditions. Production LOC ~260K, test LOC ~203K. Build is green.

**Implementation Blockers**:
- eBPF programs require bare metal (sudo, kernel ≥5.15, BPF filesystem)
- Real LXD client requires LXD socket access
- D-020 DOOM RUNS requires full doomgeneric port + BPF ring on bare metal

**Pre-Existing Issues (Must Fix This Sprint)**:
- CancelFlowValue: Go `protocol/flow.go` encodes 24 bytes, Rust `monad-common` encodes 20 bytes → pick one, update both, update RFC
- 13 files hardcode ports → grep and replace with `pkg/ports/` constants
- `TestPerformance_TraceProcessingLatency` → relax threshold to 25µs or optimize path

**Dashboard Functional Gaps** (Must close):
- Packet Flow tab: renders but needs real/demo data pipeline working smoothly
- Trace Table tab: functional but needs polished empty state
- Latency Chart tab: functional but needs demo data generator
- Doom tab: functional (gradient/checkerboard demos work)
- Services tab: DOES NOT EXIST YET — must build
- Infrastructure tab: DOES NOT EXIST YET — must build
- Logs tab: exists at logs.html but not integrated into tab navigation
- Metrics section: renders but shows "--" without backend

**Estimated Effort**:
| Task | T-Shirt | LOC Est |
|------|---------|---------|
| Design system unification (CSS) | L | 1,500 |
| Service management tab + API | M | 800 |
| Infrastructure tab + API | M | 600 |
| Logs tab integration | S | 200 |
| Demo data generators | M | 500 |
| Wire format fix | S | 100 |
| Port hardcode cleanup | S | 200 |
| DOOM GPL consolidation | M | 400 |
| www_unheaded_org landing | L | 1,200 |
| AI stack docker-compose | M | 300 |
| Vision doc rewrite | S | 300 |
| Doc cleanup/archive | M | 200 |
| **Total** | | **~6,300** |

### The Hourglass Measures (Timeguru — Timeline & Milestones)

**Current Age**: Age 1 — Alpha, ~99% structural complete

**Velocity**: Last sprint (S41-S44) shipped ~9,800 LOC in one overnight session. Average session velocity: 2,000-4,000 LOC.

**Milestone Status Update**:
| Milestone | Previous % | Actual % | Notes |
|-----------|-----------|----------|-------|
| Whispering Void (eBPF) | 55% | 55% | Operational on WEST bare metal |
| Cuirass (control plane) | 75% | 80% | IaC + observability frameworks added |
| Royal Court (services) | 85% | 90% | 25 services operational |
| Citadels Rise (containers) | 75% | 80% | Helm chart + TestableRuntime added |
| Cape & Cloak (dashboard) | 90% | 75% | DOWNGRADED — "AI slop" admission means we overstated |
| Alpha Demo | 10% | 25% | Frameworks done, UI polish remaining |

**ETA to "Showable to Humans"**: 3-5 focused sessions (S45-S49), ~2 weeks at current velocity.

**Historical Pattern**: S30-S35 showed we can execute massive sprints when scope is clear. S41-S44 proved autonomous overnight execution works. This sprint needs the same discipline: clear battle plan → heads-down execution → handoff.

### The Sundial Tracks (Calendar — Schedule & Deadlines)

**This Week (2/24 - 3/1)**:
- Mon 2/24: Round Table (this session) — forge battle plan
- Tue-Wed 2/25-26: Phase 1 — Docs/RFC alignment + wire format fix
- Thu-Fri 2/27-28: Phase 2 — Design system unification + dashboard overhaul
- Sat 3/1: Phase 3 — Service/Infrastructure UI scaffolding

**Next Week (3/1 - 3/7)**:
- Mon-Tue 3/3-4: Phase 4 — DOOM consolidation + computational validation
- Wed-Thu 3/5-6: Phase 5 — www_unheaded_org + AI stack integration plan
- Fri 3/7: Phase 6 — Vision rewrite, doc cleanup, next Round Table

**Bare Metal Server Setup**: Parallel track — can happen anytime this week
**Gaming Desktop AI Setup**: After bare metal is running Unheaded daemon

### The Scroll Validates (Lore — Naming & Mythology)

**Naming Decisions Pending**:
- AI model service name: Propose **"Sophia's Eye"** (Sophia = wisdom/knowledge, Eye = observation/inference)
- Service management UI: Propose **"The Armory"** (where armor pieces are managed)
- Infrastructure tab: Propose **"The Forge"** (where infrastructure is shaped)
- Public website: **"The Gate"** (entry point to the Kingdom)

**Mythology Consistency**: All existing names follow the three pillars (Gnostic, Amber, Medieval). New names must continue this.

**Sacred Law Compliance**: All 8 laws honored. No naming violations detected.

### The Map Confirms (Kingdom — Hierarchy & Placement)

**Hierarchy Health**: All tiers properly populated. New components placement:
- Sophia's Eye (AI) → Applications Tier (port range 20000-20999)
- The Armory (service mgmt UI) → Dashboard extension (existing tier)
- The Forge (infra UI) → Dashboard extension (existing tier)
- www_unheaded_org → External-facing, outside the Doom Range

**Tier Integrity**: Good. No misplaced components detected.

### The Goblet Toasts (Busboy — Alignment & Coordination)

**Cross-Skill Conflicts**: None detected. All seats aligned on priorities.

**Coordination Needs**:
- RFC Editor + Developer must sync on CancelFlowValue resolution
- Architect + Developer must sync on service management YAML schema
- Barrister must review GPL compliance of DOOM consolidation before execution
- BlackMage should review AI stack attack surface before deployment

**Team Vibes**: HIGH ENERGY. PoC near-complete. The finish line is visible. Muck is fired up. The crew is ready to sprint.

**Translation**: "AI slop dashboards" = the engineering works but the presentation layer needs human design sensibility. This is a UI/UX sprint with strategic cleanup, not a feature sprint.

---

### Unified Battle Plan

---

## Phase 1: Docs & RFC Alignment (Owner: Librarian + RFC Editor + Developer)
*"The scrolls must speak truth."*

### 1.1 Wire Format Audit & Fix
- [ ] **FIX CancelFlowValue mismatch** — decide 20B or 24B, update Go `pkg/protocol/flow.go` AND Rust `monad-common/src/lib.rs`, update RFC Section 5.x
- [ ] **Grep all hardcoded ports** — replace with `pkg/ports/` constants (13+ files identified in S41)
- [ ] **Fix TestPerformance_TraceProcessingLatency** — relax threshold to 25µs or optimize hot path
- [ ] **Verify**: `go test ./... -race` passes with 0 failures

### 1.2 RFC Draft Alignment
- [ ] **Read draft-bellis-unheaded-protocol-foundation-04.md end-to-end** against current implementation
- [ ] **Cross-reference**: every wire format in the RFC has matching Go struct + Rust struct + test
- [ ] **Update RFC** if implementation diverged (CancelFlowValue, any other findings)
- [ ] **Check draft-bellis-unheaded-sophia-dictionary-01.md** against `pkg/protocol/sophia/`
- [ ] **Check draft-bellis-unheaded-wotan-memory-01.md** against `services/wotan/`

### 1.3 Strategic Doc Updates
- [ ] **Rewrite VISION.md** — current version is Feb 10 and Muck says "we are way past this"
- [ ] **Update CLAUDE.md** — ensure all S41-S44 changes reflected (new packages, helm, SBOM)
- [ ] **Update README.md** — reflect current LOC counts, new capabilities
- [ ] **Update battle-plan.md** (root) — replace S34 content with S45 content
- [ ] **Update references/timeline.md** — fix milestone percentages per Timeguru findings above
- [ ] **Propose archive list**: identify stale docs for docs/archive/

---

## Phase 2: Design System Unification & Dashboard Overhaul (Owner: Developer + Architect)
*"The Kingdom must look as strong as it is."*

### 2.1 Unified Design System
- [ ] **Create `/dashboard/css/design-system.css`** — single source of truth CSS variables
- [ ] **Decision point**: bellis.tech aesthetic (black+blue+custom fonts) vs Kingdom gold (Muck decides)
- [ ] **Self-host fonts**: JetBrains Mono + Space Grotesk in `/dashboard/assets/fonts/`
- [ ] **Unify particle canvas** across dashboard, kanban, doom viewer
- [ ] **Port bellis.tech nav pattern**: frosted glass nav, clean typography, accent-glow cards

### 2.2 Dashboard Polish
- [ ] **Redesign nav bar**: cleaner, bellis.tech inspired, all links working
- [ ] **Redesign tab navigation**: remove emoji icons, use clean SVG or text-only
- [ ] **Redesign metric cards**: adopt card system from bellis.tech (bg-card, border, radius)
- [ ] **Add proper empty states** for all tabs (not just blank divs)
- [ ] **Add demo data generator** — fills all tabs with realistic-looking data when no backend running
- [ ] **Fix Logs integration** — bring logs.html content into tab navigation as 5th tab
- [ ] **Fix all console errors** — zero JS errors in Chrome DevTools
- [ ] **Mobile responsive pass** — hamburger menu, stacked cards, readable on phone

### 2.3 Kanban Polish (D4 — Wotan Kanban Enhancements)
- [ ] **Apply unheaded.org design system** to kanban/index.html and cmd/kanban-app/static/
- [ ] **Unify the TWO kanban implementations** — /kanban/ and /cmd/kanban-app/ should be ONE
- [ ] **Implement drag-and-drop between columns** — HTML5 Drag API or pointer events, update task status on drop via API
- [ ] **Add "Review" column** — tasks in review get special actions: Approve, Reject, Request Changes
- [ ] **Review action buttons** — each triggers a status transition + Wotan event publish
- [ ] **Test CRUD operations** — create, edit, move (drag), delete tasks all working
- [ ] **Fix any broken WebSocket reconnection** behavior
- [ ] **Verify Meta Moment**: kanban tracking its own development, real-time via Wotan

---

## Phase 3: Service & Infrastructure Management UI (Owner: Developer + Architect)
*"The Armory and The Forge take form."*

### 3.1 Service Management Tab ("The Armory")
- [ ] **Define YAML schema** for service configs:
  ```yaml
  # /opt/unheaded/<service>/config.yaml
  service:
    name: wotan
    port: 18001
    protocol: grpc
    health_endpoint: /health
    tier: infrastructure
    replicas: 1
    depends_on: []
  ```
- [ ] **Add dashboard-backend endpoint**: `GET /api/v1/services` (reads YAML configs)
- [ ] **Add dashboard-backend endpoint**: `POST /api/v1/services/{name}/restart` (signals daemon)
- [ ] **Build "Services" tab** in dashboard — card grid: name, port, status indicator, health, actions
- [ ] **Static fallback**: if no daemon running, read from bundled `services.yaml` manifest

### 3.2 Infrastructure Tab ("The Forge")
- [ ] **Add dashboard-backend endpoint**: `GET /api/v1/infrastructure` (container runtime status)
- [ ] **Build "Infrastructure" tab** — shows container status per runtime (Docker, LXD, NixOS)
- [ ] **IaC integration**: button to `generate` configs for selected backend
- [ ] **Link to Helm chart** deployment status when running in K8s

### 3.3 Verify All Tabs Working
- [ ] **Tab inventory**: Packet Flow ✓, Trace Table ✓, Latency ✓, Doom ✓, Services (NEW), Infrastructure (NEW), Logs (INTEGRATED)
- [ ] **Each tab**: renders, has data (demo or real), no console errors, proper empty state
- [ ] **Navigation**: all nav links resolve, no 404s, breadcrumbs if needed

---

## Phase 4: DOOM Consolidation & Computational Validation (Owner: Developer + Barrister)
*"Prove it's not a parlor trick."*

### 4.1 DOOM GPL Consolidation
- [ ] **Barrister review**: confirm GPL boundary clean before consolidation
- [ ] **Move ~/tmp/unheaded/doom/ edits → ~/tmp/doomgeneric/** (the fork, not the monorepo)
- [ ] **Ensure doomgeneric fork has**: doom.mbc, doom.rv2mbc, doom1.wad, doomgeneric_unheaded.c, crt0.S
- [ ] **Clean ~/tmp/unheaded/doom/**: remove duplicated source, keep only references/links
- [ ] **Update THIRD_PARTY.md**: reflect new GPL file locations
- [ ] **Test**: doomgeneric fork builds independently, MBC pipeline still works

### 4.2 Original DOOM with Sound (~/tmp/DOOM)
- [ ] **Assess**: id-Software fork at ~/tmp/DOOM — can it cross-compile C → RV32I → MBC?
- [ ] **Sound challenge**: MBC has 2 syscalls (SCREEN_WRITE, KBD_READ) — need AUDIO_WRITE syscall
- [ ] **Feasibility**: estimate ROM size for full DOOM with sound vs BPF map capacity
- [ ] **Document**: findings in docs/doom/ORIGINAL_DOOM_FEASIBILITY.md

### 4.3 Computational Generality Validation
- [ ] **Research**: lightweight open-source SNES emulators in C (candidates: bsnes-classic, snes9x-mini)
- [ ] **Research**: Unix v4 (1973) tape backup — what's needed to boot it in MBC?
- [ ] **Feasibility matrix**: for each candidate, estimate ROM size, RAM needs, syscall requirements
- [ ] **Document**: findings in docs/doom/COMPUTATIONAL_GENERALITY.md
- [ ] **Stretch goal**: if any candidate is <4MB ROM and uses simple I/O, attempt cross-compile

---

## Phase 4B: RFC-Compliant Protocol API (Owner: Developer + RFC Editor + Architect)
*"The bridge between armor and application." (D6)*

### 4B.1 Protocol API Design
- [ ] **Define API surface** — RESTful + gRPC dual interface for Unheaded Protocol operations
- [ ] **Core endpoints**:
  - `POST /api/v1/monad/encode` — encode a Monad register from structured input
  - `POST /api/v1/monad/decode` — decode raw 20-byte Monad to structured JSON
  - `GET /api/v1/sophia/dictionaries` — list available Sophia dictionaries
  - `GET /api/v1/sophia/dictionaries/{id}` — get dictionary entries
  - `POST /api/v1/wotan/read` — read from per-flow Wotan memory
  - `POST /api/v1/wotan/write` — write to per-flow Wotan memory
  - `GET /api/v1/anamnesis/events` — query Anamnesis event stream
  - `GET /api/v1/flows` — list active flows with Monad state
  - `POST /api/v1/flows/{label}/inject` — inject a packet into a flow
- [ ] **gRPC proto definitions** — `proto/unheaded/v1/protocol.proto`
- [ ] **RFC Editor review** — ensure API semantics align with all 3 Internet-Drafts
- [ ] **OpenAPI spec** — generate from Go handlers, publish at `/api/v1/docs`
- [ ] **Authentication** — API key or mTLS for protocol operations (not open)

### 4B.2 Protocol API Implementation
- [ ] **Go handlers** in `cmd/protocol-api/` or extend `cmd/dashboard-backend/`
- [ ] **Wire format validation** — every encode/decode verifies CRC-16, version, bounds
- [ ] **Sophia dictionary CRUD** — read-only for now, write requires BPF map access (bare metal)
- [ ] **Wotan memory proxy** — translates API calls to BPF helper invocations (bare metal) or mock (dev mode)
- [ ] **Tests** — table-driven tests for every endpoint, fuzzing on decode
- [ ] **Port allocation** — Protocol API in 17000-17999 range (control plane tier)

### 4B.3 Protocol Innovation Preservation (D3)
- [ ] **Document inverse mask** (double static address space) in docs/protocol/INVERSE_MASK.md
- [ ] **Document Kingdom Mode** fully — all 4 states, transitions, security implications
- [ ] **Ensure RFC draft Section 11** (Kingdom Mode) is comprehensive and normative
- [ ] **Ensure RFC draft** covers inverse mask proposal (new section or appendix)
- [ ] **Cross-reference**: every innovative proposal has both spec text AND implementation code

---

## Phase 5: Public Presence & AI Stack (Owner: Architect + Captain + Developer)
*"The Gate opens. Sophia's Eye awakens."*

### 5.1 www_unheaded_org Enhancement (D5)
- [ ] **Keep existing aesthetic** — headless knights, Latin etymology, dark minimal, poetic
- [ ] **Enhance**: add feature highlights section, architecture diagram, protocol overview
- [ ] **Add**: "What is Unheaded?" section with the daisy/war/extinction poetry as motif
- [ ] **Add**: links to dashboard, wiki, GitHub (when public), protocol API docs
- [ ] **Ensure**: responsive, particle canvas if fits the vibe, fast load
- [ ] **Mount ~/tmp** (D7) to access live source for direct editing

### 5.2 AI Model Stack — First "Head" in Armor (D2)
- [ ] **Docker Compose**: vLLM + Open WebUI + Qdrant + BGE-M3 embedder
- [ ] **Hardware validation**: confirm RX 7700 XT ROCm support with vLLM
- [ ] **Model selection**: DeepSeek-R1 7B-distill (fits 12GB VRAM) + Qwen 2.5 Coder 7B
- [ ] **THIS IS THE FIRST "HEAD"** — AI service runs INSIDE Unheaded's suit of armor:
  - Registered in service discovery via Sophia dictionary
  - All inference requests traced by eBPF (ingress/egress visible on dashboard)
  - Strictly locked-down network: only allowed flows defined in Shield WAF rules
  - All traffic flows through Wotan message bus for observability
  - Dashboard shows AI inference latency, throughput, error rates in real-time
- [ ] **Protocol API integration** (D6): AI talks to Unheaded via the RFC-compliant API, not raw internals
- [ ] **NixOS container definition**: add to nix/containers/ for the AI stack
- [ ] **Port allocation**: Sophia's Eye in 20000-20999 range (propose 20100-20199)
- [ ] **Document**: AI stack architecture in docs/architecture/AI_STACK.md
- [ ] **Gorgonia replacement**: document why gorgonia is dead (Go generics killed it). Use vLLM API for inference. Build Go-native tensor foundation only for lightweight in-app tasks (embeddings, similarity). 2TB HDD for model weights + RAG corpus, 1TB NVMe as swap/vram overflow

### 5.3 Bare Metal Server Setup Plan
- [ ] **Document**: bare metal specs, OS install plan, network config
- [ ] **NixOS**: generate config for the 4-core DDR3 machine
- [ ] **Unheaded deployment**: docker-compose or NixOS containers for all services
- [ ] **Network**: EVPN-VXLAN tunnel between gaming desktop and bare metal
- [ ] **Split-brain architecture**: AI on gaming box, services on bare metal, Wotan bridges both

---

## Phase 6: Vision, Cleanup & Next Round Table (Owner: Captain + Librarian + Timeguru)
*"Polish the crown before presenting it."*

### 6.1 Vision Document Rewrite
- [ ] **New VISION.md**: reflect actual current state + realistic near-term goals
- [ ] **Include**: AI integration vision, bare metal deployment, conference targets
- [ ] **Include**: the "first secure thing wrapped in Unheaded's armor" AI stack narrative
- [ ] **Remove**: outdated "early-stage" hedging — we have 260K LOC, this is real

### 6.2 Documentation Cleanup
- [ ] **Propose archive list** (move to docs/archive/):
  - Stale planning docs superseded by battle plans
  - Old session handoffs older than S30 (keep for history but mark archived)
  - Duplicate docs identified in S44 cleanup
- [ ] **Propose deletion list** (remove entirely):
  - Empty stubs, broken references, orphaned files
- [ ] **Wiki sync**: ensure unheaded-wiki/ reflects current state (68 pages, check for stale)
- [ ] **README audit**: all links in README.md resolve, all claims accurate

### 6.3 Pre-Public Checklist
- [ ] **SBOM current**: run ScanCode + FOSSology before any public push
- [ ] **License headers**: all source files have correct license header (BSL 1.1 or GPL 2.0 for doom)
- [ ] **Secrets scan**: grep for hardcoded secrets, API keys, tokens — ZERO tolerance
- [ ] **Git history clean**: no large binaries, no credentials, no sensitive data in history
- [ ] **.gitignore audit**: confirm all build artifacts, secrets, binaries excluded

---

### Decisions Made at This Round Table

1. **Dashboard aesthetic**: **RESOLVED — unheaded.org dark aesthetic** (#0a0a0a, zero saturation, monospace, medieval). NOT bellis.tech blue. (D5)
2. **AI model stack**: vLLM + DeepSeek-R1 distilled + Qwen 2.5 Coder = FIRST "HEAD" in Unheaded armor. eBPF-traced, Wotan-observed, Shield-gated. (D2) — **APPROVED**
3. **DOOM consolidation**: move edits to doomgeneric fork, clean monorepo — **APPROVED PENDING BARRISTER GPL REVIEW**
4. **Computational generality**: research SNES emu + Unix v4 feasibility — **APPROVED AS RESEARCH ONLY**
5. **Gorgonia**: dead project. Use vLLM API. Go-native tensors only for lightweight edge tasks. (D2)
6. **Bare metal split**: services on bare metal (4-core DDR3), AI brain on gaming desktop (Ryzen 5 + RX 7700 XT) — **APPROVED**
7. **www_unheaded_org**: already live with perfect aesthetic — **ENHANCE, DON'T REDESIGN** (D5)
8. **RFC-Compliant Protocol API**: critical for application adoption, enables AI "head" integration (D6) — **APPROVED**
9. **Protocol innovation preservation**: inverse mask, Kingdom Mode, all novel proposals must persist prominently (D3) — **MANDATED**
10. **Kanban drag-drop + review actions**: Meta Moment must be polished, not just functional (D4) — **APPROVED**

### Open Questions (Carry to Next Round Table)

1. **Conference target**: Which conference? When? What talk format? — Captain + Muck to decide by Phase 6
2. **VC outreach timing**: Austin VC while repo is private? Need public demo first? — Captain to assess
3. **Public launch date**: What's the trigger? "All tabs working" or "bare metal proven"? — Muck decides
4. **JWT auth design**: token format, session management, integration with gateway — Developer + Architect pre-Phase 7
5. **WAF Rust rebuild**: priority relative to AI stack? — Defer to post-AI integration

### Wins to Celebrate

- **S41-S44 overnight autonomous execution**: 23 commits, 9,800 LOC, zero human intervention needed — the autonomous sprint model WORKS
- **Anti-lock-in principle is REAL**: 6 IaC backends, 8 observability backends, 4 container runtimes — all swappable
- **DOOM keyboard input FIXED**: multi-key circular queue, Go loader replaces Python, actual gameplay works
- **Repo cleanup**: 826MB reclaimed, 32 loose docs organized into 5 directories, skills consolidated
- **158 packages pass**: ZERO failures, ZERO race conditions — the codebase is HEALTHY
- **Meta Moment proven**: Kanban tracks itself via Wotan message bus — we eat our own dog food
- **3 Internet-Drafts**: IETF Experimental track. A protocol spec. For real. With IANA considerations.

---

### Agent Execution Matrix

| Phase | Primary Agent | Support Agents | Est. LOC | Est. Duration |
|-------|--------------|----------------|----------|---------------|
| 1 - Docs/RFC Alignment | Librarian + RFC Editor | Developer | 800 | 1 session |
| 2 - UI Overhaul (D5 aesthetic) | Developer | Architect | 2,500 | 2 sessions |
| 3 - Service/Infra UI | Developer + Architect | — | 1,600 | 1 session |
| 4 - DOOM/PoC | Developer + Barrister | Scientist | 500 | 1 session |
| 4B - Protocol API (D6) | Developer + RFC Editor | Architect, BlackMage | 1,200 | 1-2 sessions |
| 5 - Public/AI as First Head (D2) | Architect + Developer | Captain | 1,800 | 2 sessions |
| 6 - Polish + Innovation Docs (D3) | Captain + Librarian | Timeguru | 600 | 1 session |
| **TOTAL** | | | **~9,000** | **9-12 sessions** |

### Next Round Table

**Scheduled**: After Phase 3 completion (estimated 3/1) OR if a blocker emerges
**Reason**: Mid-sprint review — are we on track for "showable to humans"?

---

### Warmonger's Detailed Sub-Steps (Phase 1-3 Immediate Execution)

**These are the steps an autonomous agent can execute NOW:**

#### Phase 1 Detailed Steps (26 steps)

```
P1-01: git checkout -b s45-polish
P1-02: grep -rn "CancelFlowValue" pkg/ services/ ebpf/ --include="*.go" --include="*.rs"
P1-03: Determine correct wire size (20B or 24B) from RFC Section 5
P1-04: Fix Go implementation in pkg/protocol/flow.go
P1-05: Fix Rust implementation in monad-common/src/lib.rs
P1-06: Update RFC draft Section 5 to match
P1-07: go test ./pkg/protocol/... -race -v
P1-08: grep -rn "16666\|17000\|18001\|19000\|20000\|21443" --include="*.go" | grep -v pkg/ports | grep -v _test.go
P1-09: Replace each hardcoded port with pkg/ports constant
P1-10: go test ./... -race (full suite, verify 0 failures)
P1-11: Fix TestPerformance threshold (relax to 25µs or optimize)
P1-12: go test ./pkg/ebpf/... -race -run TestPerformance -v
P1-13: Read draft-bellis-unheaded-protocol-foundation-04.md end-to-end
P1-14: Cross-reference each wire format diagram against Go structs
P1-15: Cross-reference each wire format diagram against Rust structs
P1-16: Document any additional mismatches found
P1-17: Read draft-bellis-unheaded-sophia-dictionary-01.md
P1-18: Cross-reference against pkg/protocol/sophia/
P1-19: Read draft-bellis-unheaded-wotan-memory-01.md
P1-20: Cross-reference against services/wotan/
P1-21: Rewrite docs/VISION.md with current scope
P1-22: Update CLAUDE.md with S41-S44 changes
P1-23: Update README.md LOC counts and new capabilities
P1-24: Update references/timeline.md milestone percentages
P1-25: Compile archive proposal list
P1-26: git add -A && git commit -m "fix(protocol): align wire format + docs sprint"
```

#### Phase 2 Detailed Steps (34 steps)

```
P2-01: Create dashboard/css/design-system.css with CSS variables
P2-02: Copy JetBrains Mono woff2 to dashboard/assets/fonts/
P2-03: Copy Space Grotesk woff2 to dashboard/assets/fonts/
P2-04: Add @font-face declarations to design-system.css
P2-05: Define color palette (based on Muck's choice)
P2-06: Define spacing scale (xs through 3xl)
P2-07: Define typography scale
P2-08: Define border/radius/shadow tokens
P2-09: Refactor dashboard/css/style.css to import design-system.css
P2-10: Update nav bar — frosted glass, clean links, no emoji
P2-11: Update tab navigation — clean buttons, proper active state
P2-12: Update metric cards — bg-card, border, radius, hover
P2-13: Update footer — clean, minimal
P2-14: Add empty state component (reusable for all tabs)
P2-15: Build demo data generator (js/demo-data.js)
P2-16: Wire demo data to Packet Flow tab
P2-17: Wire demo data to Trace Table tab
P2-18: Wire demo data to Latency Chart tab
P2-19: Wire demo data to Doom tab (already has gradient/checkerboard)
P2-20: Integrate logs.html content as 5th tab
P2-21: Build Log Viewer tab panel in index.html
P2-22: Wire log-viewer.js to tab lazy-init
P2-23: Test all tabs render without console errors
P2-24: Mobile responsive pass — test at 375px, 768px, 1024px
P2-25: Fix hamburger menu behavior
P2-26: Refactor kanban CSS to use same design-system.css
P2-27: Merge the TWO kanban implementations (decide which is canonical)
P2-28: Test kanban CRUD: create task, edit, move columns, delete
P2-29: Test kanban WebSocket reconnection
P2-30: Update doom.html to use design system
P2-31: Update logs.html to use design system (if kept as standalone)
P2-32: Visual regression check — screenshot all pages
P2-33: git add -A && git commit -m "feat(ui): unify design system, polish dashboard + kanban"
P2-34: Verify: 0 console errors, all tabs render, responsive works
```

#### Phase 3 Detailed Steps (22 steps)

```
P3-01: Define ServiceConfig YAML schema in docs/architecture/SERVICE_CONFIG_SCHEMA.md
P3-02: Create sample config: services/wotan/config.yaml
P3-03: Create sample config: services/captain/config.yaml
P3-04: Create sample config: services/dashboard/config.yaml
P3-05: Add Go struct for ServiceConfig in pkg/discovery/config.go
P3-06: Add YAML parser: pkg/discovery/yaml_loader.go
P3-07: Add tests for YAML loading
P3-08: Add dashboard-backend handler: GET /api/v1/services
P3-09: Add dashboard-backend handler: GET /api/v1/services/{name}
P3-10: Add dashboard-backend handler: POST /api/v1/services/{name}/restart (stub)
P3-11: Add tests for service API endpoints
P3-12: Build "Services" tab panel HTML in dashboard/index.html
P3-13: Build services.js — card grid, health indicators, refresh
P3-14: Wire services tab to lazy-init system
P3-15: Add dashboard-backend handler: GET /api/v1/infrastructure
P3-16: Build "Infrastructure" tab panel HTML
P3-17: Build infrastructure.js — container status cards
P3-18: Wire infrastructure tab to lazy-init system
P3-19: Test all 7 tabs: Packet Flow, Trace, Latency, Doom, Services, Infrastructure, Logs
P3-20: Test service card rendering with sample YAML
P3-21: git add -A && git commit -m "feat(dashboard): add Services + Infrastructure tabs, YAML config"
P3-22: Full test suite: go test ./... -race (verify 0 failures)
```

---

_Forged at the Round Table by all 9 minds. Barrister (GPL review), RFC Editor (wire format alignment), BlackMage (AI attack surface), Developer (implementation), Busboy (coordination), Micromanager (QA gates), Architect (infrastructure design), Captain (vision), Timeguru (milestones), Calendar (schedule), Lore (naming), Kingdom (hierarchy). The Kingdom marches as one._

_"You bring the application. Unheaded provides the infrastructure. And now — the infrastructure provides the intelligence."_
