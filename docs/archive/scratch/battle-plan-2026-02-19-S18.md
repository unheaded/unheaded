# Battle Plan: Protocol Doc Sync + Doom Integration Sprint
## Convened: 2026-02-19 (Session 18 — Cowork Round Table) | Reason: Sprint Planning + File Sync
## Kingdom State: Age 1.5 (Protocol Awakening), ~99% Alpha, ~260K production LOC (~464K w/ tests), Doom-over-IPv6 ACTIVE

---

### Situation Report

The Kingdom stands at a pivotal moment. 17 sessions of Claude Code CLI work have produced ~260K production LOC (~464K w/ tests) with a fully implemented Doom-over-IPv6 compute layer in software — BPF CPU core (40 opcodes), Wotan RAM (cache miss + dirty writeback), wotan-ctl CLI, MBC assembler with SYSCALL, dashboard CPU trace overlay, and a two-pass RV32I→MBC translator with 52 tests. The unheaded-protocol repo has massive uncommitted changes (busboy→wotan rename, Wotan service, eBPF workspace). Protocol specs have been iterating outside the repo — 8 uploaded docs represent the latest canonical versions with terminology standardization (BPF→eBPF, "atomically replaceable"→"hot-swappable", "kernel datapath speed"→"wire speed") and structural updates (event struct 64→40 bytes, MATH_AND_MAPS field layout redesign).

**Critical blocker remains unchanged:** All remaining Doom tasks (D-010 through D-020, Waves 3-6) require a real Linux kernel with BPF/XDP support and network namespaces — **not available in Cowork VM or macOS**. The tart Ubuntu VM is the execution target.

**This session's wins:** Protocol docs synced across both repos. 12 files updated in `unheaded`, 11 files updated/added in `unheaded-protocol`. All 3 IETF-style drafts (foundation-03, sophia-00, wotan-00) now canonical in both repos.

---

### The Throne Speaks (Captain — Vision & Strategy)

**Strategic Position**: We've proven computational completeness in theory (draft-03) and in software (59.8M MBC instructions, Doom reaches main()). The gap is hardware integration — real eBPF on real Linux.

**North Star**: Doom rendering in a browser via IPv6 packet circulation through eBPF programs at each hop. That's the demo that makes investors and IETF reviewers sit up.

**Key Decision**: This Cowork session should maximize doc/spec work and planning that DOESN'T require Linux kernel access, then hand off a tight battle plan to Claude Code CLI on the tart VM.

**Risk to Vision**: Protocol spec drift — docs iterating outside the repo creates divergence. MITIGATED this session by syncing all 8 files.

### The Ledger Records (Micromanager — Execution & QA)

**Sprint Status**: Session 17 completed D-001 through D-009 + D-013, D-014, D-015 (12/20 tasks). 8 tasks remain, all Linux-blocked.

**Priority Stack** (ordered):
1. **P0** — Git commits for both repos (doc sync + S17 work) — READY NOW
2. **P0** — D-010: Packet circulation ring test harness (THE FIRST PACKET) — Linux-blocked
3. **P1** — D-011/D-012: Dashboard↔BPF wiring (framebuffer + keyboard) — Linux-blocked
4. **P1** — D-016/D-017: gradient.mbc + checkerboard.mbc full pipeline — Linux-blocked
5. **P2** — D-018/D-019/D-020: Doom cross-compile + bare-metal port — Linux-blocked

**QA Gates**: `cargo test -p monad-mbc` (54 tests), `go test ./... -race` (all pass), git clean state

**Acceptance Criteria**: All protocol docs committed. Handoff doc for next CLI session includes exact commands.

### The Blueprint Reveals (Architect — Infrastructure & Design)

**Architecture Health**: SOLID. The compute loop design (packet→XDP→fetch/decode/execute→XDP_PASS) is clean. Memory hierarchy (L0 Monad → L1 BPF LRU → L2 Wotan ring → L3 WAL) is well-layered. SYSCALL bridge provides I/O without leaving the packet path.

**Technical Risks**:
- BPF verifier rejection of the 40-opcode CPU program (complexity limit) — HIGH impact, MEDIUM probability
- Cache miss latency under load (Wotan restaging speed) — MEDIUM impact
- RV32I→MBC translator edge cases with real Doom binaries (signed ops, x16-x31) — KNOWN, mitigated with `-ffixed-` flags

**Protocol Alignment**: All 3 specs updated and synced:
- **Monad (draft-03)**: 20-byte register, field layout redesigned, exponent encoding canonical
- **Sophia (draft-00)**: Hot-swappable dictionaries, simplified prose
- **Wotan (draft-00)**: Wire speed terminology, eBPF standardized, SHOULD→MAY for access control

**Recommended ADR**: Document the BPF verifier risk mitigation strategy (instruction budgeting, map access patterns)

### The Anvil Reports (Developer — Implementation & Testing)

**Code Health**: BUILD CLEAN. 54 Rust tests pass (monad-mbc). All Go tests pass with `-race`. Zero races detected.

**Implementation Blockers**: All remaining work is Linux kernel integration — cannot be done in Cowork or macOS.

**TDD Status**: Green. monad-cpu-ebpf is written but untested (needs BPF verifier). wotan-ctl has dry-run tests. anamnesis_compute has round-trip fuzz tests.

**Estimated Effort**:
- D-010 (packet ring): M — shell script exists, needs real namespaces
- D-011/D-012 (dashboard wiring): M each
- D-016/D-017 (demo programs): S each — programs already written
- D-018/D-019/D-020 (Doom port): XL — cross-compiler + bare-metal stubs + integration

### The Hourglass Measures (Timeguru — Timeline & Milestones)

**Current Age**: Age 1.5 — Protocol Awakening (Alpha 99% + Doom sprint active)

**Velocity**: Sessions 16-17 delivered 12 Doom tasks in 2 sessions. At this rate, remaining 8 tasks = ~2-3 CLI sessions on Linux.

**ETA to Next Milestone**: D-016 (gradient.mbc full pipeline, "THE MOMENT OF TRUTH") — estimated 1-2 CLI sessions after D-010 passes.

**ETA to D-020 (DOOM WALKS THE PATTERN)**: 3-5 CLI sessions, heavily dependent on BPF verifier acceptance and cross-compiler stability.

### The Sundial Tracks (Calendar — Schedule & Deadlines)

**Today (2026-02-19)**: Protocol doc sync (DONE in this session). Battle plan forged. Handoff prepared for next CLI session.

**This Week Remaining**:
- Push git commits for both repos
- Boot tart VM, confirm `cargo build` and `go build` clean
- Execute D-010 (packet circulation ring)
- If D-010 passes: D-016 (gradient.mbc) same session

**Protocol Deadlines**: No external deadlines. Self-imposed: Doom demo by end of February 2026.

### The Scroll Validates (Lore — Naming & Mythology)

**Naming Decisions Pending**: None. All terminology standardized in this sync:
- "BPF" → "eBPF" consistently across all specs
- "atomically replaceable" → "hot-swappable" (Sophia, Wotan)
- "kernel datapath speed" / "per-hop latency ~320 ns" → "wire speed"
- Leon Torres acknowledgment added to draft-02

**Mythology Consistency**: CLEAN. Monad/Sophia/Wotan trinity intact. Ascension of Busboy→Wotan complete (codebase rename done in sessions 10-11).

**Sacred Law Compliance**: All 8 laws honored. No anti-patterns detected.

### The Map Confirms (Kingdom — Hierarchy & Placement)

**Hierarchy Health**: GOOD. All protocol docs now live in `docs/protocol/protocol/` (canonical) with copies in `docs/protocol/` (convenience). Both repos aligned.

**New Components**: wotan-ctl correctly placed in `cmd/wotan-ctl/`. doom-viewport in `dashboard/js/`. Compute memory handler in `services/wotan/internal/compute/`.

**Tier Integrity**: Protocol specs in the right Hollow. Implementation in the right Armory tier. No misplacements.

### The Goblet Toasts (Busboy — Alignment & Coordination)

**Cross-Skill Conflicts**: NONE. All seats aligned on priorities.

**Coordination Needs**:
- Cowork session (this) → CLI session handoff must be clean
- Two repos need separate commits with separate messages
- unheaded-protocol has MASSIVE uncommitted changes (busboy→wotan rename + Wotan service + eBPF workspace) — needs careful staging

**Team Vibes**: ELECTRIC. 12 tasks done in 2 sessions. Protocol specs polished. The momentum is real.

**Translation**: "Wire speed" is the key phrase — Captain uses it for the pitch deck, Architect uses it for the design doc, Developer uses it in the code comments. One language, one Kingdom.

---

### Unified Battle Plan

#### Immediate Actions (This Cowork Session — Next 30 Minutes)

- [x] Sync all 8 protocol docs to both repos — Owner: Round Table — DONE
- [ ] Write git commit messages for both repos — Owner: Micromanager
- [ ] Assess what Cowork CAN do (docs, specs, skill updates, planning) — Owner: Busboy

#### Git Commits To Push (from CLI or local terminal)

**unheaded repo:**
```
cd ~/tmp/unheaded
git add docs/protocol/
git commit -m "docs(protocol): sync latest spec revisions — eBPF terminology, hot-swappable, wire speed

Updates all protocol support docs and IETF drafts to latest canonical versions:
- BPF → eBPF standardized across all specs
- 'atomically replaceable' → 'hot-swappable' (Sophia, Wotan)
- 'kernel datapath speed ~320ns' → 'wire speed'
- Anamnesis event struct: 64 → 40 bytes
- PROTOCOL_MATH_AND_MAPS: field layout redesigned for draft-03
- draft-02: Leon Torres acknowledgment added
- sophia-00: simplified prose, hot-swappable
- wotan-00: SHOULD→MAY access control, eBPF, wire speed"
```

**unheaded-protocol repo:**
```
cd ~/tmp/unheaded-protocol
git add docs/PROTOCOL_FOUNDATION.md docs/PROTOCOL_MATH_AND_MAPS.md docs/PROTOCOL_TECHNICAL_SUMMARY.md
git add docs/draft-bellis-unheaded-protocol-foundation-02.md
git add docs/draft-bellis-unheaded-protocol-foundation-03.md
git add docs/draft-bellis-unheaded-sophia-dictionary-00.md
git add docs/draft-bellis-unheaded-wotan-memory-00.md
git add docs/the_first_packet.md the_first_packet.md
git commit -m "docs(protocol): add draft-02/03 + sophia-00 + wotan-00 + sync support docs

Adds all three IETF-style Internet-Drafts to docs/:
- draft-bellis-unheaded-protocol-foundation-02 (Computational Completeness)
- draft-bellis-unheaded-protocol-foundation-03 (Latest, field layout redesign)
- draft-bellis-unheaded-sophia-dictionary-00 (Dictionary Subsystem)
- draft-bellis-unheaded-wotan-memory-00 (Memory Subsystem)

Updates support docs with eBPF terminology standardization and
event struct redesign (64→40 bytes per event)."
```

#### Multi-Agent Actions Available In This Cowork Session

These tasks do NOT require Linux kernel access and CAN be done now:

1. **Skill Updates** — Update unheaded-developer, unheaded-architect, unheaded-timeguru skills with S17 progress (Doom compute layer complete, protocol synced)
2. **ADR: BPF Verifier Risk** — Write Architecture Decision Record for BPF complexity budget strategy
3. **Protocol Review** — Cross-reference draft-03 field layout against monad-cpu-ebpf implementation for consistency
4. **Timeline Update** — Update timeline.md with S17+S18 entries
5. **DOOM_TASKS_20 Review** — Verify task statuses match actual git state

#### Linux CLI Session (Next Session — Tart VM)

```
WAVE 3 — THE FIRST PACKET:
  D-010: sudo ./scripts/doom-ring.sh start && inject test packet

WAVE 4 — Dashboard Wiring:
  D-011: Framebuffer WebSocket (SCREEN_WRITE → canvas)
  D-012: Keyboard → BPF map

WAVE 5 — MOMENT OF TRUTH:
  D-016: gradient.mbc full pipeline
  D-017: checkerboard.mbc full pipeline

WAVE 6 — DOOM:
  D-018: C → RV32I → MBC cross-compile
  D-019: doomgeneric stubs
  D-020: DOOM WALKS THE PATTERN
```

#### Decisions Made at This Round Table

1. **Uploads are canonical** — The 8 uploaded files represent the latest spec versions. Repos updated to match. — Owner: All
2. **Both repos get commits** — Separate commit messages prepared for each repo. — Owner: Muck (to push)
3. **Cowork session for docs/planning, CLI for integration** — Clean separation of what's possible where. — Owner: Busboy

#### Open Questions (Carry to Next Round Table)

1. **BPF verifier acceptance** — Will monad-cpu-ebpf pass the verifier's complexity limits? — Answer: D-010 — Deadline: Next CLI session
2. **unheaded-protocol mega-commit** — The busboy→wotan rename + all uncommitted changes should be staged carefully. Stage by feature or one big commit? — Answer: Muck — Deadline: Before next push
3. **Protocol spec publication** — When do drafts move from docs/ to a public IETF submission? — Answer: Captain — Deadline: Post-Doom-demo

#### Wins to Celebrate

- **12 Doom tasks completed in 2 sessions** — Developer + Architect — Massive velocity
- **Protocol specs unified** — 3 IETF drafts + 4 support docs now canonical in both repos — Round Table
- **Zero test failures** — 54 Rust + all Go tests pass with -race — Developer
- **59.8M MBC instructions executed** — Doom reaches main() in software simulation — Developer
- **~260K production LOC (~464K w/ tests)** — From zero to production-ready infra in 17 sessions — Everyone

---

### Next Round Table
**Scheduled**: After D-010 passes (THE FIRST PACKET circulates successfully)
**Reason**: Protocol Milestone Review — first real eBPF execution on the wire

---
_Forged at the Round Table by all 9 minds. The Kingdom marches as one._
_The wire is the processor. Wotan is the RAM. The packet walks the pattern. Doom follows._
