# SPRINT ORCHESTRATOR — S45 through S50
# Master coordination document for unattended Claude Code execution

**Date**: 2026-02-24
**Scope**: 6 sequential sprints, ~700+ total steps
**Critical Path**: S45 → S46 → S47 → S48 → S49 → S50
**Estimated Total Duration**: 50-70 hours across 6 sprints

---

## HOW TO USE THIS DOCUMENT

Each sprint is launched as a separate Claude Code session. Copy the kickoff prompt
for the current sprint, paste it into Claude Code, and let it execute autonomously.
When it finishes, verify the handoff gate before launching the next sprint.

---

## SPRINT CHAIN & DEPENDENCY MAP

```
S45 (Docs/RFC)
  │
  ├──→ S46 (Dashboard UI)     ← needs design system from S45 decisions
  │       │
  │       └──→ S47 (Service Mgmt) ← needs design system + dashboard tabs
  │
  └──→ S48 (DOOM Validation)   ← needs wire format fix from S45
          │
          └──→ S49 (Protocol API)  ← needs validated wire format + DOOM proven
                  │
                  └──→ S50 (AI Model)  ← needs Protocol API for integration
```

**Parallelizable**: S46+S48 CAN run in parallel (independent subsystems) if you
have two Claude Code sessions. S47 depends on S46. S49 depends on S48. S50 depends on S49.

---

## SPRINT 1: S45 — DOCS & RFC ALIGNMENT

### Battle Plan
`~/tmp/unheaded/S45-DOCS-RFC-ALIGNMENT-BATTLE-PLAN.md`

### Kickoff Prompt (copy-paste into Claude Code)

```
You are executing an unattended code sprint for the Unheaded project.

IMPORTANT: Read CLAUDE.md first for project context: ~/tmp/unheaded/CLAUDE.md
Then read the battle plan: ~/tmp/unheaded/S45-DOCS-RFC-ALIGNMENT-BATTLE-PLAN.md

EXECUTE every step sequentially. Follow these rules:

1. Each step has a checkbox, tag ([B], [V], [D], [W], [R], [S], [P], [C]), time estimate (~Xm), and command
2. [V] steps are verification gates — if they FAIL, take the [D] debug branch
3. [C] steps are git commit checkpoints — commit with format:
   [PLAN S45] Steps N-M: Summary
4. STUCK PROTOCOL: if you spend 3x the estimated time OR fail debug twice:
   - Log what you tried
   - Mark step [STUCK] with reason
   - Mark downstream dependent steps [BLOCKED]
   - Skip to next independent step
   - Continue executing all non-blocked steps
5. NEVER skip verification gates — if a gate fails and debug fails, mark [BLOCKED]
6. Commit every 5 steps minimum, even if no [C] tag
7. After every phase exit gate, commit immediately
8. NEVER commit broken state — only commit after last [V] passed

DECISIONS PRE-SEEDED:
- CancelFlowValue wire size: Check RFC Section 5 — RFC is canonical, code must match
- Design system: FUSION of unheaded.org (dark #0a0a0a soul) + bellis.tech (CSS engineering)
- Repo location: ~/tmp/unheaded/

Branch: create s45-docs-rfc-alignment from current HEAD

START AT STEP 1. Execute every step. Report progress at each phase exit gate.

When S45 is COMPLETE, output a HANDOFF SUMMARY:
---
## S45 HANDOFF SUMMARY
**Status**: COMPLETE / INCOMPLETE
**Steps**: X completed / Y skipped / Z blocked out of TOTAL
**Commits**: (list hash + message for each)
**STUCK items**: (list with reason + suggested fix)
**BLOCKED items**: (list with upstream dependency)
**Wire format decision**: 20B or 24B — what was chosen?
**Tests**: go test ./... -race result (pass/fail count)
**Ready for S46**: YES / NO (if NO, explain what blocks it)
**Ready for S48**: YES / NO (if NO, explain what blocks it)
---
```

### Handoff Gate (verify before launching S46)

```bash
# Run these manually after S45 completes:
cd ~/tmp/unheaded && git log --oneline -10    # Verify S45 commits exist
go test ./... -race 2>&1 | tail -5            # Verify tests pass
git diff --stat s45-docs-rfc-alignment~10..s45-docs-rfc-alignment  # Review changes
```

**S45 → S46 Gate Criteria**:
- [ ] Wire format mismatch RESOLVED (CancelFlowValue consistent across Go/Rust/RFC)
- [ ] Hardcoded ports replaced with pkg/ports constants
- [ ] go test ./... -race passes with 0 failures
- [ ] Design system CSS variables file exists
- [ ] VISION.md rewritten

---

## SPRINT 2: S46 — DASHBOARD OVERHAUL

### Battle Plan
`~/tmp/unheaded/S46-DASHBOARD-OVERHAUL-BATTLE-PLAN.md`

### Kickoff Prompt

```
You are executing an unattended code sprint for the Unheaded project.

IMPORTANT: Read CLAUDE.md first: ~/tmp/unheaded/CLAUDE.md
Read the S45 handoff (if exists): check git log for [PLAN S45] commits
Then read the battle plan: ~/tmp/unheaded/S46-DASHBOARD-OVERHAUL-BATTLE-PLAN.md

CONTEXT FROM PRIOR SPRINT (S45):
- Wire format is now aligned across Go/Rust/RFC
- Design system CSS variables are defined
- VISION.md has been rewritten
- All tests passing

EXECUTE every step sequentially. Follow these rules:

1. Each step has checkbox, tag, time estimate, and command
2. [V] = verification gate. FAIL → take [D] debug branch
3. [C] = git commit checkpoint. Format: [PLAN S46] Steps N-M: Summary
4. STUCK PROTOCOL: 3x time OR 2 failed debugs → skip with [STUCK], continue
5. Commit every 5 steps minimum
6. NEVER commit broken state

DESIGN SYSTEM (MANDATORY — this is the soul of the sprint):
- Background: #0a0a0a (near-black)
- Text primary: #c9c9c9 (muted silver)
- Text heading: #fff (pure white, rare)
- Text dim: #666 (subtle labels)
- Text ghost: #3a3a3a (infrastructure poetry, reveals on hover to #888)
- Border: #222 (subtle borders)
- ZERO saturation. No bright colors. No gold. No blue.
- Typography: JetBrains Mono (code/headers) + Space Grotesk (body)
- From bellis.tech: frosted glass nav, CSS variables, card components, spacing scale
- From unheaded.org: dark soul, monospace, medieval, poetic, minimal

Branch: create s46-dashboard-overhaul from s45-docs-rfc-alignment

START AT STEP 1. Execute every step.

When S46 is COMPLETE, output:
---
## S46 HANDOFF SUMMARY
**Status**: COMPLETE / INCOMPLETE
**Steps**: X/Y/Z out of TOTAL
**Commits**: (list)
**Dashboard tabs working**: Packet Flow / Trace / Latency / Doom / Logs
**Kanban drag-drop**: WORKING / NOT WORKING
**Kanban review column**: WORKING / NOT WORKING
**Console errors**: 0 / N (list if any)
**Mobile responsive**: YES / NO
**Design system applied to**: dashboard / kanban / doom viewer / (list)
**Ready for S47**: YES / NO
---
```

### Handoff Gate

```bash
cd ~/tmp/unheaded
git log --oneline -15 | grep "PLAN S46"
# Open dashboard/index.html in browser — visual check
# Open kanban/index.html — test drag-drop
# Check Chrome DevTools console — 0 errors
```

**S46 → S47 Gate Criteria**:
- [ ] Design system CSS applied to ALL dashboard pages
- [ ] All 5 original tabs render without errors
- [ ] Kanban drag-and-drop between columns works
- [ ] Kanban Review column with approve/reject/request-changes works
- [ ] JetBrains Mono + Space Grotesk self-hosted and loading
- [ ] 0 console errors in Chrome DevTools

---

## SPRINT 3: S47 — SERVICE MANAGEMENT + INFRASTRUCTURE UI

### Battle Plan
`~/tmp/unheaded/S47-SERVICE-MANAGEMENT-BATTLE-PLAN.md`

### Kickoff Prompt

```
You are executing an unattended code sprint for the Unheaded project.

IMPORTANT: Read CLAUDE.md first: ~/tmp/unheaded/CLAUDE.md
Then read the battle plan: ~/tmp/unheaded/S47-SERVICE-MANAGEMENT-BATTLE-PLAN.md

CONTEXT FROM PRIOR SPRINTS:
- S45: Wire format aligned, docs updated, tests passing
- S46: Dashboard overhauled with unified design system (#0a0a0a dark theme),
  kanban has drag-drop + review column, all original tabs working

EXECUTE every step sequentially. Follow these rules:

1. Each step has checkbox, tag, time estimate, and command
2. [V] = verification gate. FAIL → take [D] debug branch
3. [C] = git commit checkpoint. Format: [PLAN S47] Steps N-M: Summary
4. STUCK PROTOCOL: 3x time OR 2 failed debugs → skip, continue
5. Commit every 5 steps minimum
6. NEVER commit broken state

KEY ARCHITECTURE:
- Service configs: YAML at /opt/unheaded/<service>/config.yaml (or bundled fallback)
- Dashboard backend: Go handlers at cmd/dashboard-backend/
- New tabs: "Services" (The Armory) + "Infrastructure" (The Forge)
- Port allocation: services in 18000-18999, infra in 19000-19999
- ALL new UI MUST use the design system from S46 (design-system.css)

Branch: create s47-service-management from s46-dashboard-overhaul

START AT STEP 1. Execute every step.

When S47 is COMPLETE, output:
---
## S47 HANDOFF SUMMARY
**Status**: COMPLETE / INCOMPLETE
**Steps**: X/Y/Z out of TOTAL
**Commits**: (list)
**All 7 dashboard tabs working**: Packet Flow / Trace / Latency / Doom / Logs / Services / Infrastructure
**Service YAML schema**: DEFINED / NOT DEFINED
**API endpoints**: GET /api/v1/services / GET /api/v1/infrastructure / (list working)
**go test ./... -race**: PASS / FAIL
**Ready for S48**: YES / NO
---
```

### Handoff Gate

```bash
cd ~/tmp/unheaded
go test ./... -race 2>&1 | tail -5
git log --oneline -15 | grep "PLAN S47"
# Open dashboard — verify 7 tabs all render
```

**S47 → S48 Gate Criteria**:
- [ ] Services tab renders with card grid (even if mock data)
- [ ] Infrastructure tab renders with container status
- [ ] All 7 tabs navigate correctly
- [ ] go test ./... -race passes
- [ ] YAML service config schema defined and tested

---

## SPRINT 4: S48 — DOOM VALIDATION + GO REWRITE

### Battle Plan
`~/tmp/unheaded/S48-DOOM-VALIDATION-BATTLE-PLAN.md`

### Kickoff Prompt

```
You are executing an unattended code sprint for the Unheaded project.

IMPORTANT: Read CLAUDE.md first: ~/tmp/unheaded/CLAUDE.md
Then read the battle plan: ~/tmp/unheaded/S48-DOOM-VALIDATION-BATTLE-PLAN.md

CONTEXT FROM PRIOR SPRINTS:
- S45: Wire format aligned (CancelFlowValue resolved), all tests passing
- S46-S47: Dashboard fully overhauled with 7 tabs, design system unified

THIS SPRINT HAS 3 TRACKS:
Track A: DOOM keyboard fix (scancode mismatch + key state bitmap) — DO FIRST
Track B: Python → Go rewrite (doom-loader, doom-cpu-dump) — AFTER keyboard fix
Track C: Computational generality research (SNES emu, Unix v4 feasibility) — PARALLEL

CRITICAL KEYBOARD FIX (from Scientist's analysis):
- TranslateKey() in i_input.c is IDENTITY (passthrough)
- Ctrl sends 0x9D but Doom expects KEY_FIRE = 0xA3
- Space sends 0x20 but Doom expects KEY_USE = 0xA2
- WASD not bound in vanilla doomgeneric — map to arrow equivalents
- Single-slot KBD_MAP causes key stomping — need 256-bit key state bitmap (32 bytes)
- KBD_MAP value size: 16 → 40 bytes (32 state + 8 sequence)
- CPU_MAP/CpuState grows: 104 → 136 bytes (adds last_kbd_state[32])

BARE METAL REQUIREMENT: Tracks A+B need sudo + BPF filesystem + kernel ≥5.15.
Track C is research only — no bare metal needed.

Branch: create s48-doom-validation from s47-service-management

START AT STEP 1. Execute every step.

When S48 is COMPLETE, output:
---
## S48 HANDOFF SUMMARY
**Status**: COMPLETE / INCOMPLETE
**Steps**: X/Y/Z out of TOTAL
**Commits**: (list)
**Track A — Keyboard**: FIXED / STUCK (detail)
**Track B — Go Rewrite**: COMPLETE / PARTIAL (which tools rewritten)
**Track C — Generality**: Research complete / findings summary
**DOOM playable**: YES / NO (multi-key: W+Ctrl works?)
**Python scripts replaced**: (list deleted files)
**go test ./... -race**: PASS / FAIL
**GPL consolidation**: COMPLETE / PENDING BARRISTER REVIEW
**Ready for S49**: YES / NO
---
```

### Handoff Gate

```bash
cd ~/tmp/unheaded
go test ./... -race 2>&1 | tail -5
git log --oneline -20 | grep "PLAN S48"
ls cmd/doom-loader/ cmd/doom-cpu-dump/  # Go rewrites exist
ls scripts/*.py | wc -l                  # Python scripts removed/moved
```

**S48 → S49 Gate Criteria**:
- [ ] Wire format proven in live DOOM execution (or documented as bare-metal-blocked)
- [ ] Go loader + Go cpu-dump replace Python scripts
- [ ] Keyboard bitmap implemented (or documented as bare-metal-blocked)
- [ ] go test ./... -race passes
- [ ] Computational generality feasibility doc exists

---

## SPRINT 5: S49 — RFC-COMPLIANT PROTOCOL API

### Battle Plan
`~/tmp/unheaded/S49-PROTOCOL-API-BATTLE-PLAN.md`

### Kickoff Prompt

```
You are executing an unattended code sprint for the Unheaded project.

IMPORTANT: Read CLAUDE.md first: ~/tmp/unheaded/CLAUDE.md
Read all 3 RFC drafts for alignment:
  ~/tmp/unheaded/docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md
  ~/tmp/unheaded/docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md
  ~/tmp/unheaded/docs/protocol/draft-bellis-unheaded-wotan-memory-01.md
Then read the battle plan: ~/tmp/unheaded/S49-PROTOCOL-API-BATTLE-PLAN.md

CONTEXT:
- S45: Wire format aligned — RFC is source of truth, code matches
- S48: DOOM validates the wire format works end-to-end
- This API is how applications (starting with S50's AI model) talk to Unheaded

THIS IS DIRECTIVE D6: The bridge between armor and application.

DUAL INTERFACE: REST (JSON over HTTP/1.1) + gRPC (protobuf over HTTP/2)
Both interfaces MUST return identical results for identical operations.

CORE API SURFACE:
- Monad: encode/decode 20-byte registers with CRC-16 validation
- Sophia: dictionary CRUD (read-only without BPF, full with BPF)
- Wotan: per-flow memory read/write
- Anamnesis: event stream query + WebSocket real-time
- Flows: list active flows, inject packets

MOCK MODE: Every endpoint must work WITHOUT BPF maps (for dev/testing).
BPF MODE: Connect to real pinned maps when running on bare metal.

Branch: create s49-protocol-api from s48-doom-validation

START AT STEP 1. Execute every step.

When S49 is COMPLETE, output:
---
## S49 HANDOFF SUMMARY
**Status**: COMPLETE / INCOMPLETE
**Steps**: X/Y/Z out of TOTAL
**Commits**: (list)
**Proto definitions**: COMPLETE / PARTIAL (list missing)
**REST endpoints working**: (list each with status)
**gRPC services working**: (list each with status)
**OpenAPI spec generated**: YES / NO
**Mock mode**: ALL ENDPOINTS / PARTIAL
**BPF mode**: ALL ENDPOINTS / PARTIAL / NOT TESTED (bare metal required)
**Authentication**: API key / mTLS / NONE
**Port**: (what port was allocated)
**go test ./... -race**: PASS / FAIL
**Ready for S50**: YES / NO
---
```

### Handoff Gate

```bash
cd ~/tmp/unheaded
go test ./... -race 2>&1 | tail -5
git log --oneline -20 | grep "PLAN S49"
ls proto/unheaded/v1/                     # Proto files exist
ls cmd/protocol-api/ || ls cmd/dashboard-backend/api/  # API handlers exist
```

**S49 → S50 Gate Criteria**:
- [ ] All 5 gRPC services defined in proto
- [ ] REST endpoints for monad encode/decode working
- [ ] Mock mode works without BPF
- [ ] go test ./... -race passes
- [ ] OpenAPI spec generated and servable

---

## SPRINT 6: S50 — AI MODEL STACK (FIRST "HEAD" IN ARMOR)

### Battle Plan
`~/tmp/unheaded/S50-AI-MODEL-STACK-BATTLE-PLAN.md`

### Kickoff Prompt

```
You are executing an unattended code sprint for the Unheaded project.

IMPORTANT: Read CLAUDE.md first: ~/tmp/unheaded/CLAUDE.md
Then read the battle plan: ~/tmp/unheaded/S50-AI-MODEL-STACK-BATTLE-PLAN.md

CONTEXT FROM ALL PRIOR SPRINTS:
- S45: Docs aligned, wire format fixed
- S46: Dashboard overhauled (#0a0a0a dark theme, 7 tabs)
- S47: Service management + infrastructure UI
- S48: DOOM validated, Go tools replace Python
- S49: Protocol API live (REST+gRPC, mock mode works)

THIS IS DIRECTIVE D2: The AI model is the FIRST APPLICATION running as "head"
inside Unheaded's suit of armor.

HARDWARE TARGET:
- Gaming Desktop: AMD Ryzen 5 7600X, RX 7700 XT (12GB VRAM), 16GB DDR5, 2TB HDD, 1TB NVMe
- Bare Metal Server: 4-core DDR3 for Unheaded services
- Communication: Wotan gRPC over EVPN-VXLAN between hosts

AI STACK:
- vLLM (ROCm backend for AMD GPU) — inference server
- DeepSeek-R1 7B distilled — reasoning/RAG model (fits 12GB VRAM)
- Qwen 2.5 Coder 7B — code generation model
- BGE-M3 — embeddings model
- Qdrant — vector database
- Open WebUI or custom Unheaded chat UI

THE "HEAD" REQUIREMENTS:
1. AI service registered in Sophia dictionary (service discovery)
2. All inference requests traced by eBPF (visible on dashboard)
3. Strictly locked-down network: only allowed flows via Shield WAF rules
4. All traffic flows through Wotan for observability
5. Dashboard shows AI inference latency, throughput, error rates
6. AI talks to Unheaded via Protocol API (D6/S49), NOT raw BPF maps

GORGONIA IS DEAD (Go generics killed it). Use vLLM API for inference.
Go-native tensor ops only for lightweight in-app tasks (embeddings, similarity).

Branch: create s50-ai-model-stack from s49-protocol-api

START AT STEP 1. Execute every step.

When S50 is COMPLETE, output:
---
## S50 HANDOFF SUMMARY — FINAL SPRINT
**Status**: COMPLETE / INCOMPLETE
**Steps**: X/Y/Z out of TOTAL
**Commits**: (list)
**Docker Compose**: CREATED / NOT CREATED
**vLLM ROCm**: VERIFIED / UNTESTED (needs GPU)
**Model selection**: (which models, what quantization)
**Sophia registration**: AI service registered in dictionary
**Shield rules**: inference ingress/egress locked down
**Dashboard integration**: AI metrics visible / NOT YET
**Protocol API integration**: AI uses API / DIRECT
**NixOS container def**: CREATED / NOT CREATED
**Port allocation**: (what ports for AI services)
**Bare metal setup doc**: EXISTS / NOT EXISTS
**EVPN-VXLAN config**: DOCUMENTED / CONFIGURED / UNTESTED
**go test ./... -race**: PASS / FAIL

## FULL CAMPAIGN STATUS
**S45 Docs/RFC**: COMPLETE
**S46 Dashboard**: COMPLETE
**S47 Services**: COMPLETE
**S48 DOOM**: COMPLETE
**S49 Protocol API**: COMPLETE
**S50 AI Stack**: COMPLETE
**Overall**: READY FOR HUMAN REVIEW / NEEDS INTERVENTION
**Bare metal items**: (list anything that needs real hardware)
---
```

---

## QUICK REFERENCE: ALL BRANCHES

```
main
  └── s45-docs-rfc-alignment
       └── s46-dashboard-overhaul
            └── s47-service-management
                 └── s48-doom-validation
                      └── s49-protocol-api
                           └── s50-ai-model-stack
```

After S50 completes, merge chain back:
```bash
git checkout main
git merge s50-ai-model-stack --no-ff -m "Merge S45-S50: The Polishing of the Kingdom"
```

Or if you prefer squash-per-sprint:
```bash
git checkout main
for branch in s45-docs-rfc-alignment s46-dashboard-overhaul s47-service-management s48-doom-validation s49-protocol-api s50-ai-model-stack; do
  git merge $branch --no-ff -m "Merge $branch"
done
```

---

## PARALLEL EXECUTION STRATEGY

If you have TWO Claude Code sessions:

```
Session 1:  S45 ──→ S46 ──→ S47
Session 2:         S48 ──→ S49 ──→ S50

Timeline:
  Day 1: Session 1 starts S45, Session 2 waits (S48 needs S45 wire fix)
  Day 1: S45 completes → Session 2 starts S48 from S45 branch
  Day 2: Session 1 does S46, Session 2 does S48 (PARALLEL)
  Day 3: Session 1 does S47, Session 2 does S49
  Day 4: Session 2 does S50 (needs S49 API)
  Day 4: Merge both chains into main
```

Merge strategy for parallel:
```bash
git checkout main
git merge s47-service-management --no-ff
git merge s50-ai-model-stack --no-ff  # may need conflict resolution
```

---

## EMERGENCY: CONTEXT OVERFLOW

If a sprint is too large for one Claude Code context window:

1. The agent should commit everything done so far
2. Output a PARTIAL HANDOFF with the last completed step number
3. Launch a NEW session with this prompt prefix:

```
You are RESUMING an unattended code sprint that ran out of context.

Read the battle plan: ~/tmp/unheaded/S4X-...-BATTLE-PLAN.md
Check git log for [PLAN S4X] commits to see what's done.
RESUME FROM STEP [N] (the first uncompleted step).
Continue all rules from the original kickoff.
```

---

## POST-CAMPAIGN: NEXT ROUND TABLE

After S50, convene Round Table to assess:
- What shipped vs what was [STUCK] or [BLOCKED]
- Bare metal items that need real hardware
- GPU items that need the RX 7700 XT
- Next sprint priorities (S51+)
- Conference/demo readiness assessment
- Public launch criteria check

---

*Forged at the Round Table, 2026-02-24*
*6 sprints. 700+ steps. The Polishing of the Kingdom.*
*"You bring the application. Unheaded provides the infrastructure. And now — the infrastructure provides the intelligence."*
