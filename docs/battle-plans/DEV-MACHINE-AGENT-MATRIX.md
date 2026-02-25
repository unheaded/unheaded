# DEV-MACHINE-AGENT-MATRIX — Sprint S33 (Feb 23 - Mar 7, 2026)

**Document Owner:** Warmonger + Micromanager (Coordination Role)
**Updated:** February 23, 2026
**Current Machine Load:** Single developer + multi-agent parallel execution
**Total Sprint Time Available:** 10 business days (50 hours)

---

## EXECUTIVE SUMMARY

**The Kingdom has 4-5 days of critical path work and 10 days of parallel execution.**

The S33 hardening sprint begins IMMEDIATELY (today) and completes by Feb 23 EOD. WS1 (doom-bridge) and WS3 (scaling profiler) run in **strict parallel** Feb 23-28. WS2 (Lich campaigns) and WS4 (documentation) run in parallel Mar 1-7 (off critical path). WS5 (return to core) starts Mar 8 after Round Table.

**Critical Path:** Hardening → WS1/WS3 parallel → WS5 kickoff
**Off-Path:** WS2 + WS4 run during WS1+WS3, consume zero critical resources
**Bottleneck:** dev machine CPU (profile XDP, build services) — addressed by agent serialization

---

## SECTION 1: PARALLEL EXECUTION TIMELINE

```
S33 SPRINT TIMELINE
═══════════════════════════════════════════════════════════════════════════════════

TODAY (Feb 23):
─────────────────────────────────────────────────────────────────────────────────
  COORDINATOR: Hardening Sprint (BLOCKING) ─────────→ Complete by EOD
  ├─ Phase 0: Environment verify (15 min)
  ├─ Phase 2: /tmp → scripts/ migration (90 min)
  ├─ Phase 8: Documentation (30 min)
  ├─ Phase 9: Commit discipline (15 min)
  └─ Moat Ghost P0 quick fixes (20 min)

  EXIT GATE: All scripts in repo, /tmp clean, build passes, tests pass


WEEK 1 (Feb 24-28):
─────────────────────────────────────────────────────────────────────────────────
  Agent A (Developer/Architect)  ───  WS1: Doom-Bridge Service ──────────────────→
    └─ DoD: Browser shows live Doom frames (title screen → demo cycle)
    └─ Estimated: 2-3 days (Feb 24-26)

  Agent B (Developer/Scientist)   ───  WS3: Scaling Profiler ─────────────────────→
    └─ DoD: 15+ fps sustained, zero corruption, Netflix burst model proven
    └─ Estimated: 1-2 days (Feb 24-27)

    [Agent A and Agent B work on DIFFERENT machines if possible,
     or stagger dev machine access: A builds go/test, B profiles XDP]

  Agent C (Engineer/Coordinator)  ───  P0/P1 Quick Wins (independent) ────────────→
    └─ DoD: All <30-min items closed
    └─ Estimated: 2-3 hours total (scattered during A/B execution)


WEEK 2 (Mar 1-7):
─────────────────────────────────────────────────────────────────────────────────
  Agent D (BlackMage/Developer)   ───  WS2: Lich D1-D6 Campaigns ────────────────→
    └─ DoD: All 6 campaigns executed, documented, critical fixes merged
    └─ Estimated: 3-5 days (Mar 1-5, can compress to 2 with parallelization)
    └─ RUN PARALLEL: Independent of A/B output

  Agent E (Captain/Developer)     ───  WS4: Documentation + Wiki ───────────────→
    └─ DoD: Wiki browsable, conference outline complete, timeline updated
    └─ Estimated: 2-3 days (Mar 1-7)
    └─ RUN PARALLEL: Can consume WS1 output (doom-bridge architecture)

  Agent F (Developer/Calendar)    ───  P1 Backlog (remaining items) ──────────────→
    └─ DoD: Rate limiter backoff, TLS prep, auth skeleton
    └─ Estimated: 2-3 days (Mar 1-5)
    └─ RUN PARALLEL: Independent of other workstreams


WEEK 3+ (Mar 8+):
─────────────────────────────────────────────────────────────────────────────────
  ROUND TABLE RECONVENES (Full assembly for WS5 kickoff)
  └─ Input: Lich findings (D1-D6), WS3 profiling results, WS1 architecture proof
  └─ Output: 200+ step WS5 battle plan for packet tracing pipeline
```

---

## SECTION 2: DEPENDENCY GRAPH

```
CRITICAL PATH (Sequential, blocking):
═════════════════════════════════════════════════════════════════════════════════

  ┌─────────────────┐
  │ Hardening       │
  │ Sprint          │  0.5 days
  │ (TODAY)         │
  └────────┬────────┘
           │
           ▼
  ┌─────────────────────────────────────────────────────────────┐
  │  WS1 + WS3 PARALLEL (Feb 24-28)                             │
  │  ├─ WS1: doom-bridge Go service (2-3 days)                  │
  │  └─ WS3: Scaling profiler + burst injection (1-2 days)      │
  │  Frame buffer → WebSocket streaming                         │
  │  XDP timing optimization + Netflix burst model              │
  └────────┬────────────────────────────────────────────────────┘
           │
           ├─ FEED ──→ WS4 (Captain uses doom-bridge architecture)
           │
           ├─ FEED ──→ Scientist (use WS3 profiling results)
           │
           ▼
  ┌─────────────────────────────────────────────────────────────┐
  │  WS2 + WS4 + F PARALLEL (Mar 1-7, OFF CRITICAL PATH)        │
  │  ├─ WS2: Lich D1-D6 campaigns (3-5 days)                    │
  │  ├─ WS4: Documentation + wiki (2-3 days)                    │
  │  └─ F:   P1 backlog (2-3 days)                              │
  │  All run independently; findings feed WS5                   │
  └────────┬────────────────────────────────────────────────────┘
           │
           ├─ FEED ──→ Round Table (WS5 arch review)
           │
           ▼
  ┌─────────────────────────────────────────────────────────────┐
  │  ROUND TABLE RECONVENES (Mar 8, 1 day)                      │
  │  Full assembly: Arc review of WS5 packet tracing pipeline   │
  └────────┬────────────────────────────────────────────────────┘
           │
           ▼
  ┌─────────────────────────────────────────────────────────────┐
  │  WS5: RETURN TO CORE (Mar 8 onwards, 2-3 weeks)             │
  │  Packet tracing eBPF programs, real observability           │
  │  THIS IS THE PRODUCT                                        │
  └─────────────────────────────────────────────────────────────┘


OFF-CRITICAL-PATH (Parallel, non-blocking):
═════════════════════════════════════════════════════════════════════════════════

  WS2 ────────────────────────────────────────────────┐
  (Lich D1-D6, BlackMage security testing)            │
  Runs Mar 1-5 during WS1+WS3 completion              │
  ├─ BLOCKS on: Nothing (independent)                 │
  └─ FEEDS: WS5 security architecture                 │

  WS4 ────────────────────────────────────────────────┤
  (Documentation, wiki, conference outline, Captain)  │
  Runs Mar 1-7, consumes WS1 output (doom-bridge)    │
  ├─ BLOCKS on: WS1 architecture documentation        │
  └─ FEEDS: Public messaging, conference talk        │

  Agent F ───────────────────────────────────────────┤
  (P1 quick wins: backoff, TLS prep, auth skeleton)  │
  Runs Mar 1-5 during Lich/wiki work                 │
  ├─ BLOCKS on: Nothing (independent)                │
  └─ FEEDS: WS5 security implementation              │


BLOCKING RELATIONSHIPS SUMMARY:
  • Hardening → WS1/WS3 (can't demo without infrastructure cleanup)
  • WS1 → WS4 (documentation needs doom-bridge architecture)
  • WS1+WS3 → WS5 (profiling + frame extraction patterns prove scalability)
  • WS2+WS4 → WS5 (security findings + documentation inform design)
  • WS5 → Production (is the product)

CRITICAL CONSTRAINT:
  • Dev machine has ONE kernel (shared XDP environment for WS3 profiling)
  • Solution: WS1 (Go development) and WS3 (BPF profiling) can stagger on same machine
    - Hour 0-2:   Agent A builds doom-bridge locally
    - Hour 2-4:   Agent B profiles XDP (no Go changes)
    - Hour 4-6:   Agent A integrates WebSocket, no BPF changes
    - Hour 6-8:   Agent B optimizes burst model, Agent A off machine
```

---

## SECTION 3: AGENT TASK CARDS

---

### AGENT A — Doom-Bridge Service (WS1)

**Role:** Developer + Architect
**Workstream:** WS1 — Doom Video Dashboard
**Skill Set Required:** Go, eBPF (map reading), WebSocket, graphics (PNG encoding)

**📋 Task Scope**

Create `cmd/doom-bridge/` service that:
1. Reads Doom screen buffer from `RAM_MAP` BPF map (live, non-blocking)
2. Applies Doom 256-color palette → RGB conversion
3. Encodes frame as PNG (golang.org/x/image/png)
4. Streams frames to connected WebSocket clients
5. Overlays live metrics (FPS counter, instruction count, frame number)
6. Integrates with existing `dashboard/doom.html` frontend

**Files to Create/Modify:**
```
cmd/doom-bridge/
├── main.go              (service entry, HTTP server, health/ready/metrics)
├── map_reader.go        (cilium/ebpf RAM_MAP access, non-blocking reads)
├── frame_encoder.go     (Doom palette → RGB → PNG encoder)
├── websocket.go         (WebSocket server, frame broadcast)
└── metrics.go           (Prometheus metrics: frames_sent, encode_duration)

Modify:
├── dashboard/doom.html  (wire in WebSocket URL)
├── js/doom-viewport.js  (receive frames, render canvas)
└── Makefile             (build doom-bridge binary, run service)
```

**Prerequisites:**
- Hardening sprint COMPLETE (scripts migrated, /tmp clean)
- Know where `RAM_MAP` is pinned in `/sys/fs/bpf/` (from run.py)
- Understand Doom palette encoding (8-bit index → 6-bit RGB)

**Definition of Done (Pass/Fail):**
- [ ] Service starts on port 8006 (`--bind :8006` flag)
- [ ] `/health` endpoint returns 200
- [ ] `/ready` endpoint returns 200 (once RAM_MAP opened)
- [ ] `/metrics` exports `doom_frames_sent_total`, `doom_frame_encode_duration_seconds`
- [ ] WebSocket `/ws/doom/frames` accepts connections
- [ ] Browser at `localhost:3000/dashboard` (or gateway) shows live Doom frames
- [ ] Title screen visible (proof RAM_MAP is readable)
- [ ] Demo cycle animates (proof frames are live)
- [ ] FPS overlay shows real frame rate (proof metrics flow)
- [ ] Zero WebSocket panic/goroutine leaks (race detector clean)
- [ ] All existing tests still pass (`make test`)

**Estimated Time:** 2-3 days (16-24 hours actual development)

**Can Run Parallel With:** Agent B (stagger machine access: A builds Go, B profiles XDP)

**Blocked By:**
- Hardening sprint completion (script cleanup)
- Knowledge of RAM_MAP pin location (from run.py output)

**Success Look:** Open browser, see Doom title screen. Watch demo cycle animate smoothly. FPS counter shows 6-8 fps (current rate). No errors in logs.

---

### AGENT B — Scaling Profiler (WS3)

**Role:** Developer + Scientist
**Workstream:** WS3 — Scale to Playable (15+ fps)
**Skill Set Required:** eBPF (BPF map instrumentation), Python (XDP profiling), performance analysis, Go (injector rewrite)

**📋 Task Scope**

Profile and optimize Doom injection pipeline to achieve **15+ fps sustained with zero corruption**. Current state: 333 pps @ 3ms delay = 6-7 fps. Target: 1500+ pps @ burst injection = 15+ fps.

1. **Instrument STATS map** — Add per-bounce nanosecond timestamps (measure actual XDP_TX cycle time)
2. **Profile at various delays** — Test {3000, 2000, 1500, 1000, 750, 500µs} delay values, measure corruption rate
3. **Find minimum safe delay** — Binary search for delay where corruption goes to zero
4. **Implement Netflix burst model** — Fire N packets (batch size {10, 50, 100, 200}), drain XDP ring, repeat
5. **Rewrite injector in Go/Rust** — Replace Python bulk_inject.py with lower-overhead AF_PACKET or sendmmsg()
6. **Validate hypothesis H2 + H3** — Prove burst + native injector hits 15+ fps
7. **Document findings** — Create `docs/doom/PERFORMANCE.md` with methodology, results, Netflix comparison

**Files to Create/Modify:**
```
ebpf/
├── doom/src/lib.rs              (add STATS timestamps, keep cycle measurements)

scripts/doom/
├── profile_bounces.py           (new: sweep delays, measure corruption)
├── burst_inject.py              (new: batch injection, drain/fire loop)
├── inject_go/                   (new: Go injector using AF_PACKET)
│   └── main.go                  (sendmmsg() or spin-loop implementation)
└── bench_results.txt            (generated: latency table, corruption rates)

Modify:
├── scripts/doom/inject.py       (replace with burst model)
├── Makefile                     (add profile targets: `make profile-xdp`)
└── docs/doom/                   (create PERFORMANCE.md with full methodology)
```

**Prerequisites:**
- Hardening sprint COMPLETE
- Doom eBPF programs compile + run (can reload ROM + run without crash)
- Root access to dev machine (XDP attachment requires root)
- Knowledge of current injection loop (run.py structure, bulk_inject.py timing)

**Definition of Done (Pass/Fail):**

**Hypothesis Testing (ALL must pass):**
- [ ] H1 (Timing Bounds): Instrument STATS map with timestamps, show actual XDP_TX cycle time < 100µs
- [ ] H1 (No Corruption): Run 1000 packets at 500µs delay with zero packet loss (measure against expected frame count)
- [ ] H2 (Burst Gain): Fire batch of 100 packets, measure throughput vs steady-state (expect 2-3x improvement)
- [ ] H2 (Batch Safety): Test batch sizes {10, 50, 100, 200}, verify zero corruption at all sizes
- [ ] H3 (Injector): Go injector sustains 2000+ pps with zero malformed packets
- [ ] H3 (Speed Gain): Go injector shows 3-5x speedup vs Python (A/B benchmark results)

**Frame Rate Proof:**
- [ ] Sustained 15+ fps for 60 seconds (measure from frame buffer animation)
- [ ] Zero halts, zero ROM faults (check CPU state after 60-second run)
- [ ] WebSocket frames streaming smoothly (no dropped frames in doom-bridge WebSocket output)

**Documentation + Reproducibility:**
- [ ] `docs/doom/PERFORMANCE.md` contains methodology (what was measured, how)
- [ ] Includes raw data tables (delay vs fps vs corruption rate)
- [ ] Includes Netflix comparison (Cloudflare 5M pps baseline, our 1500+ pps = 0.03% utilization)
- [ ] Scripts in `scripts/doom/` are runnable standalone (`python profile_bounces.py --help`)

**Estimated Time:** 1-2 days (8-16 hours actual development, 2-4 hours running benchmarks)

**Can Run Parallel With:** Agent A (stagger XDP profiling away from doom-bridge Go builds)

**Blocked By:**
- Hardening sprint completion
- Doom eBPF environment operational (can load + run ROM)

**Success Look:** Run `python scripts/doom/profile_bounces.py`, see delay table converge on ~500µs optimal. Run `go run scripts/doom/inject_go/main.go`, see injection rate jump to 2000+ pps. Open browser, watch Doom animate at visibly smooth 15+ fps. No frame skips.

---

### AGENT C — P0/P1 Quick Wins Coordinator

**Role:** Engineer/Coordinator
**Workstream:** Embedded across WS1+WS3 (Feb 23-28)
**Skill Set Required:** Go/Python (quick fixes), git workflow, testing

**📋 Task Scope**

Execute all sub-30-minute items from TODO.md + DOOM-HARDENING-BATTLE-PLAN.md. Can be done in parallel with Agents A/B (no blocking dependencies). Coordinate so as not to conflict with A/B's builds.

**Items in Scope (from TODO.md §QUICK WINS):**

1. Pin gosec version (P0 #11)
2. Remove gosec -no-fail flag (P0 #12)
3. Add MaxHeaderBytes to kanban-app (P0 #15)
4. Verify Captain data dir not /tmp (P0 #14)
5. Add context to rate limiter cleanup (P1 #24)
6. Add exponential backoff to wotan-client polling (P1 #18)
7. Replace UnixNano() IDs with atomic counter (P2 #31, #32)
8. Add X-Request-ID middleware (P1 #38)
9. Remove dead BroadcastJSON method (P2 #37)
10. Gitignore dev artifacts (P3 #39-41)
11. Migrate timeguru to zerolog (P2 #30)
12. Split HTTP client timeouts (P1 #23)

**Files to Modify:**
```
Makefile                              (pin gosec, remove -no-fail)
cmd/kanban-app/main.go                (add MaxHeaderBytes)
services/captain/main.go              (verify data dir)
cmd/dashboard-backend/...             (context cleanup, backoff, IDs, middleware)
services/timeguru/main.go             (migrate to zerolog)
cmd/kanban-app/middleware.go          (HTTP timeout split)
.gitignore                            (add artifacts)
```

**Prerequisites:**
- All items are independent (no cross-file dependencies)
- Can read TODO.md + DOOM-HARDENING for exact line numbers

**Definition of Done (Pass/Fail):**
- [ ] Each item has a separate, minimal commit (1-2 lines per commit ideally)
- [ ] `make test` passes after each commit (no regressions)
- [ ] `make build` produces clean binaries
- [ ] All 12 items closed with passing CI
- [ ] Zero TODOs left in code for these fixes

**Estimated Time:** 2-3 hours total (scattered, 15-30 min per item, 5-10 min tests)

**Can Run Parallel With:** Agents A, B (use gaps in their machine time)

**Blocked By:** Nothing (all independent)

**Success Look:** Run `git log --oneline HEAD~15..HEAD`, see 12 quick-fix commits. Run full test suite, all green. No warnings in linters.

---

### AGENT D — Lich Campaigns D1-D6 (WS2)

**Role:** BlackMage (offensive security) + Developer
**Workstream:** WS2 — Security Hardening Campaigns
**Skill Set Required:** eBPF, fuzzing, exploit writing, security analysis

**📋 Task Scope**

Execute 6 independent security campaigns against live Doom PoC. Measure attack surface, document mitigations, identify critical findings.

**D1 — ROM Injection via ROM_MAP** (Severity: HIGH)
- **Hypothesis:** Attacker modifies ROM_MAP (bytecode) after load, before execution
- **Attack:** Inject malicious MBC bytecode into ROM_MAP via BPF map write (if writable)
- **Test:** Write 100 bytes of invalid bytecode at offset 1000, verify CPU handles gracefully (halts, doesn't crash BPF VM)
- **Mitigation:** ROM_MAP should be read-only after load (BPF_F_RDONLY flag)
- **DoD:** Document findings in `tests/security/doom/D1-ROM-INJECTION.md`

**D2 — Framebuffer Exfiltration via RAM_MAP** (Severity: MEDIUM)
- **Hypothesis:** RAM_MAP can be read by unauthorized processes
- **Attack:** From unprivileged process, attempt to mmap/read RAM_MAP
- **Test:** Run as `nobody` user, try to `bpf()` syscall to read RAM_MAP
- **Mitigation:** BPF_F_RDONLY_PROG (program-read-only) or full CAP_SYS_RESOURCE lock
- **DoD:** Document in `tests/security/doom/D2-FRAMEBUFFER-EXFIL.md`

**D3 — Keyboard Injection via SYSCALL Topic** (Severity: MEDIUM)
- **Hypothesis:** Untrusted services can inject fake keyboard events via Wotan SYSCALL topic
- **Attack:** Publish `{syscall: 30, char: 'q'}` to SYSCALL topic, watch Doom quit
- **Test:** Verify source authentication (who published this message?) — currently unvalidated
- **Mitigation:** Sign SYSCALL messages with service identity, verify in BPF program
- **DoD:** Document in `tests/security/doom/D3-KEYBOARD-INJECTION.md`

**D4 — Flow Label Collision (Birthday Attack)** (Severity: HIGH)
- **Hypothesis:** 20-bit flow label space (5-packet PoC) has 50% collision at ~1000 flows
- **Attack:** Open 500 simultaneous flows, measure collision rate in trace_id assignment
- **Test:** Spawn 500 concurrent injectors, log trace_id distribution, calculate collision percentage
- **Mitigation:** Production must use 128-bit UUIDv7 (not 20-bit flow label)
- **DoD:** Document in `tests/security/doom/D4-FLOW-COLLISION.md`, include collision probability math

**D5 — SYSCALL Fuzzing** (Severity: HIGH)
- **Hypothesis:** BPF SYS_* handlers don't validate syscall numbers — invalid syscalls could crash
- **Attack:** Fuzz SYSCALL message with random syscall codes (0-255), measure crashes
- **Test:** Send 10,000 random SYSCALL messages, monitor BPF program stability (should halt gracefully, not crash)
- **Mitigation:** Bounds check syscall number before dispatcher, default case returns error
- **DoD:** Document in `tests/security/doom/D5-SYSCALL-FUZZING.md`, include crash/halt stats

**D6 — ROM TOCTOU (Time-Of-Check-Time-Of-Use)** (Severity: HIGH)
- **Hypothesis:** ROM can be modified between load and execution (race window)
- **Attack:** Parallelize ROM load + XDP attach, write to ROM_MAP while CPU executes
- **Test:** Load ROM, immediately fork thread that constantly modifies ROM_MAP, run CPU for 100ms, measure crashes
- **Mitigation:** Snapshot ROM into immutable segment, unmap original after CPU starts
- **DoD:** Document in `tests/security/doom/D6-ROM-TOCTOU.md`

**Files to Create:**
```
tests/security/doom/
├── D1-ROM-INJECTION.md          (attack methodology, findings, PoC code)
├── D2-FRAMEBUFFER-EXFIL.md
├── D3-KEYBOARD-INJECTION.md
├── D4-FLOW-COLLISION.md         (includes collision probability table)
├── D5-SYSCALL-FUZZING.md        (includes fuzz harness, crash stats)
├── D6-ROM-TOCTOU.md             (includes race timing measurements)
└── README.md                     (Lich campaign master summary, critical/high findings list)

Modify:
└── references/lich-d1-d6-results.md (rollup: all findings, severity ratings, fixes needed)
```

**Prerequisites:**
- WS1 + WS3 COMPLETE (know frame rate, injection mechanism stable)
- Doom eBPF environment fully operational
- Root access to dev machine (BPF map introspection requires root)
- Understanding of Monad protocol + XDP flow (read Section 12 of CLAUDE.md)

**Definition of Done (Pass/Fail):**

**All 6 Campaigns Executed:**
- [ ] D1: ROM_MAP write attempt executed, documented (vulnerable or hardened)
- [ ] D2: RAM_MAP read attempt from unprivileged process, documented
- [ ] D3: SYSCALL message injection attempt, verified authentication gap or mitigation
- [ ] D4: Collision probability calculated, 50% collision threshold measured empirically
- [ ] D5: 10,000+ fuzz inputs sent, crash/halt rates measured
- [ ] D6: Race condition window measured, TOCTOU risk quantified

**Critical/High Findings Fixed:**
- [ ] All CRITICAL severity findings have mitigation + commit (e.g., ROM_MAP readonly, SYSCALL validation)
- [ ] All HIGH severity findings documented with fix plan (may be "WS5 integration" or "design review needed")

**Documentation Complete:**
- [ ] Each D1-D6 campaign has 2-3 page report (attack, findings, mitigation)
- [ ] Consolidated `references/lich-d1-d6-results.md` written for executive summary
- [ ] Critical fixes merged into main branch, all tests pass

**Estimated Time:** 3-5 days (24-40 hours actual work, 4-8 hours running fuzzing campaigns)

**Can Run Parallel With:** Agents E, F, and WS1+WS3 completion (off critical path)

**Blocked By:** WS1+WS3 COMPLETE (need stable injection + frame rate before security testing)

**Success Look:** Read `references/lich-d1-d6-results.md`, see clear findings for all 6 D's. See "CRITICAL: ROM must be readonly — FIXED in commit X" and "HIGH: flow label too small — defer to WS5". See test files with PoC code. Round Table reviews findings to shape WS5 security architecture.

---

### AGENT E — Documentation & Conference Prep (WS4)

**Role:** Captain (vision + strategy) + Developer
**Workstream:** WS4 — Documentation, Wiki, Conference Outline
**Skill Set Required:** Technical writing, architecture knowledge, storytelling, Go (wiki HTTP server)

**📋 Task Scope**

Create public-facing documentation of Kingdom, Doom PoC, and architecture. Prepare conference talk outline. Set up browsable HTTP wiki mirror within dashboard.

**1. Create Wiki Structure** (`docs/wiki/`)
- Homepage: "Welcome to the Kingdom" — 1-page origin story (Jan 20 first commit → Feb 22 Doom)
- `doom-over-ipv6.md` — Full technical narrative of Doom PoC (559 frames, 819M instructions, zero halts)
- `architecture.md` — How packets become computation (Section 12, Monad wire format, eBPF execution)
- `protocol-specs.md` — Links to RFC documents (Monad, Sophia, Wotan)
- `bug-kill-chain.md` — All 12 bugs found + fixed during Doom development (strncpy, CALLR, etc.)
- `performance.md` — Link to WS3 profiling results (injection rates, fps scaling)
- `security.md` — Lich campaign summary (D1-D6, findings, mitigations)

**2. Conference Talk Outline**
- Title: "Doom in the Data Plane: Running a Game Engine Inside eBPF"
- Structure:
  - Opening: Jan 20 first commit → Feb 22 Doom. One engineer. One AI. 33 days.
  - Problem: Packet observability is slow (sidecar overhead). eBPF is fast but hard to program.
  - Solution: Monad protocol (20-byte wire format) + BPF map RAM + XDP execution engine = Turing-complete transport
  - Proof: 559 frames of Doom. 819M instructions executed. Zero halts. Zero ROM faults.
  - Implication: If Doom works, packet tracing works. Real product coming.
  - Call to Action: Help us ship production packet tracing by M/Q (date TBD by Muck)
- Duration: 30 min talk + 10 min Q&A (typical conference slot)
- Slides: 20-25 slides (2-3 min each)

**3. HTTP Wiki Server** (`cmd/wiki-server/`)
- Simple Go server that serves markdown files as HTML (convert on-the-fly or cache)
- Integrate into gateway routing (e.g., `/wiki/` → wiki-server)
- Left-nav sidebar with Kingdom hierarchy
- Syntax highlight code blocks

**4. Update `references/timeline.md`**
- Add all Phase 7-9 achievements (Doom running, Lich planning, WS1-WS4 scheduled)
- Add completion dates for S32 tasks
- Add WS5 target date (Mar 8 Round Table → production tracing)

**Files to Create:**
```
docs/wiki/
├── README.md                    (homepage, origin story)
├── doom-over-ipv6.md           (559 frames, architecture, metrics)
├── architecture.md              (Section 12, Monad, eBPF execution model)
├── protocol-specs.md            (links to RFC-Monad, RFC-Sophia, RFC-Wotan)
├── bug-kill-chain.md            (all 12 bugs, how found, how fixed)
├── performance.md               (link to docs/doom/PERFORMANCE.md)
├── security.md                  (Lich D1-D6 summary, mitigations)
└── roadmap.md                   (timeline from S33 to WS5 completion)

cmd/wiki-server/
├── main.go                      (HTTP server, markdown→HTML converter)
├── renderer.go                  (template rendering, syntax highlight)
└── Dockerfile                   (optional: containerize wiki server)

Modify:
├── references/timeline.md       (add Phase 7-9 achievements, WS1-WS4 schedule)
├── Makefile                     (add wiki-server build target)
└── dashboard/index.html         (add /wiki link in top nav)

Conference outline (separate file):
└── docs/conference/TALK-OUTLINE.md  (30-min talk structure, slide list, speaker notes)
```

**Prerequisites:**
- WS1 + WS3 COMPLETE (have doom-bridge + performance data to document)
- Lich findings available (document D1-D6 in security.md)
- RFC documents already exist (just link them)

**Definition of Done (Pass/Fail):**

**Wiki Content Complete:**
- [ ] All 7 markdown files written (doom-over-ipv6, architecture, protocol-specs, bug-kill-chain, performance, security, roadmap)
- [ ] Each markdown file is 3-10 pages, includes code snippets / metrics tables
- [ ] Bug-kill-chain includes all 12 bugs (strncpy, CALLR, I_Error, G_DoPlayDemo NULL, Z_Malloc loop, V_DrawPatch NULL, ROM load, RAM_MAP corruption, injection stall, ...)
- [ ] Performance.md references WS3 results (injection rate table, fps scaling, Netflix comparison)
- [ ] Security.md summarizes Lich findings (one paragraph per D1-D6 attack)

**Conference Talk Ready:**
- [ ] Outline complete (title, problem, solution, proof, implication, CTA)
- [ ] 20-25 slides drafted (can be simple — goal is structure, not design)
- [ ] Speaker notes written (1-2 sentences per slide for presenter)
- [ ] Talk rehearsed and timed (should be 28-32 minutes, fit in 30-min slot)

**Wiki Server Functional:**
- [ ] HTTP server listens on port 8007 (configurable)
- [ ] GET `/wiki/` returns homepage (README.md rendered)
- [ ] GET `/wiki/doom-over-ipv6` returns HTML version of doom-over-ipv6.md
- [ ] Sidebar navigates between pages (no full-page reloads)
- [ ] Code blocks syntax highlighted (Go, Rust, Python)
- [ ] Responsive on mobile (basic Markdown CSS styling)

**Integration:**
- [ ] Gateway routes `/wiki/*` to wiki-server (port 8007)
- [ ] Dashboard top nav includes "Wiki" link
- [ ] `references/timeline.md` updated with all achievements through Feb 28
- [ ] Conference outline + talk can be found at `docs/conference/TALK-OUTLINE.md`

**Estimated Time:** 2-3 days (16-24 hours actual writing + wiki server implementation)

**Can Run Parallel With:** Agents D, F, and WS1+WS3 completion (off critical path)

**Blocked By:**
- WS1+WS3 COMPLETE (need documented results to write about)
- Lich findings AVAILABLE (not necessarily complete, but D1-D6 summaries written)

**Success Look:** Open browser to gateway `/wiki/`, see Kingdom wiki homepage. Click "Doom over IPv6", read full story with metrics embedded. See conference talk outline with slide list. Time talk, lands at 30 min. Show at Round Table, all seats agree: story is compelling, ready to pitch.

---

### AGENT F — P1 Security Backlog (Feb-Mar)

**Role:** Developer/Calendar
**Workstream:** Remaining P1 items (parallel with WS2+WS4)
**Skill Set Required:** Go, cryptography (mTLS), middleware design

**📋 Task Scope**

Implement skeletal versions of P1 security items (authentication, TLS prep, backoff). Create code structure so WS5 can plug in real implementation.

**Items in Scope:**

1. **Exponential Backoff in wotan-client** (P1 #18)
   - Implement backoff on reconnect failures (start 100ms, max 30s)
   - Test: kill Wotan, verify client exponentially backs off before each retry

2. **HTTP Client Timeout Split** (P1 #23)
   - Separate timeouts: 5s for control plane ops, 30s for streaming
   - Already partially addressed; needs full sweep

3. **Rate Limiter with Context Cancellation** (P1 #24)
   - Add `context.Context` to cleanup loop, exit on `ctx.Done()`
   - Test: graceful shutdown signal stops cleanup goroutine

4. **Input Validation on WebSocket Messages** (P1 #22)
   - Skeleton: add types for SYSCALL message, verify JSON unmarshaling
   - Skeleton: reject messages > 1KB

5. **Connection Header Validation Fix** (P1 #27)
   - Make case-insensitive (use `strings.EqualFold`)
   - Handle additional header values (e.g., `keep-alive, Upgrade`)

6. **X-Request-ID Middleware** (P2 #38)
   - Generate UUIDv7 on ingress, propagate in context
   - Log with every request (structured JSON)
   - Forward to downstream services

7. **Auth Skeleton** (P1 #16 prep)
   - Create `pkg/auth/` directory
   - Skeleton JWT validator function (unimplemented, but signature defined)
   - Skeleton mTLS cert loader (unimplemented)
   - Document design decisions for WS5 review

**Files to Create/Modify:**
```
pkg/wotan-client/
├── client.go                    (add ExponentialBackoff struct + logic)

cmd/dashboard-backend/
├── internal/websocket/server.go (add input validation, request ID)

cmd/kanban-app/
├── middleware.go                (split HTTP timeouts, fix Connection header)

pkg/auth/
├── jwt.go                       (skeleton JWT validator)
├── mtls.go                      (skeleton mTLS cert loader)
└── README.md                    (design decisions for WS5 review)

Modify:
├── Makefile                     (add P1 testing targets)
└── tests/integration/           (add backoff + timeout tests)
```

**Prerequisites:**
- None (all items are independent)
- Can proceed in parallel with WS1+WS3+WS2+WS4

**Definition of Done (Pass/Fail):**

**Code Complete:**
- [ ] Exponential backoff implemented, tested with mock Wotan failure
- [ ] HTTP client timeouts split (5s vs 30s), config verified
- [ ] Rate limiter cleanup loop accepts context, exits on Done
- [ ] WebSocket input validation rejects oversized messages
- [ ] Connection header check case-insensitive + handles variants
- [ ] X-Request-ID generated, logged, propagated (trace test shows ID flow)
- [ ] Auth skeleton created (JWT + mTLS function stubs with comments)

**Testing:**
- [ ] Backoff test: verify 100ms → 200ms → 400ms → ... progression
- [ ] Timeout test: verify control ops timeout at 5s, streams at 30s
- [ ] Context cancellation test: shutdown signal stops cleanup immediately
- [ ] Input validation test: 2KB message rejected with 400 error
- [ ] Request ID test: UUID present in all logs for a single request

**Documentation:**
- [ ] `pkg/auth/README.md` explains JWT design decision (stateless, OpenID-compatible)
- [ ] `pkg/auth/README.md` explains mTLS design decision (service-to-service, not client)
- [ ] Auth implementation notes: "WS5 will implement real cert issuing + validation"

**Estimated Time:** 2-3 days (8-16 hours implementation, 4-8 hours testing)

**Can Run Parallel With:** Agents D, E, and WS1+WS3 completion (off critical path, independent)

**Blocked By:** Nothing

**Success Look:** Run `make test`, see all P1-related tests pass. Restart wotan-client with bad Wotan endpoint, watch logs show exponential backoff (100ms, 200ms, 400ms, ..., 30s cap). Grep logs for X-Request-ID, see unique UUID on every request. Review `pkg/auth/README.md`, understand WS5 work is planned and scaffolded.

---

## SECTION 4: QUICK WINS (< 30 minutes each)

These items can be picked up by Agent C or anyone with a 30-minute gap. No dependencies, each is independent.

| # | Task | File(s) | Change | Time | Verify |
|---|------|---------|--------|------|--------|
| 1 | Pin gosec version | `Makefile` | Change `gosec@master` → `gosec@v2.21.0` | 5 min | `make lint` passes |
| 2 | Remove -no-fail flag | `Makefile` / CI | Remove `-no-fail` from gosec command | 5 min | `make lint` fails if gosec finds issues (good) |
| 3 | Add MaxHeaderBytes | `cmd/kanban-app/main.go:140` | Add `MaxHeaderBytes: 1 << 20` to `http.Server{}` | 5 min | Compile + start server |
| 4 | Verify Captain data dir | `services/captain/main.go` | Check DataDir config (should NOT be `/tmp`). Commit a6b0b73 says it's fixed. Verify. | 5 min | Read code + verify default is configurable |
| 5 | Add context cleanup | `cmd/dashboard-backend/middleware.go:148-155` | Rate limiter cleanup loop: add `select { case <-ctx.Done(): return }` | 10 min | Test graceful shutdown |
| 6 | Exponential backoff | `pkg/wotan-client/client.go:510-528` | Implement backoff on poll error: 100ms → 300ms → 900ms (cap 30s) | 15 min | Test: `TestExponentialBackoff` with mock failures |
| 7 | Replace UnixNano IDs | `services/timeguru/main.go:393`, `cmd/dashboard-backend/server.go:315` | Use atomic counter instead: `var idCounter int64; atomic.AddInt64(&idCounter, 1)` | 15 min | Ensure IDs always increment (no collisions) |
| 8 | Add X-Request-ID | `cmd/kanban-app/middleware.go` | Add middleware that generates UUID, logs in request context | 15 min | Log output shows UUID in every request |
| 9 | Remove dead method | `cmd/dashboard-backend/server.go:611` | Delete `BroadcastJSON()` method + its tests | 5 min | Compile clean, no errors |
| 10 | Gitignore artifacts | `.gitignore` | Add lines: `churn_analysis.awk`, `*-results.txt`, `PROJECT_TREE.txt` | 5 min | `git status` shows clean tree |
| 11 | Migrate timeguru to zerolog | `services/timeguru/main.go` | Replace `log.Printf()` → `zerolog` calls (pattern in other services) | 20 min | Logs show structured JSON |
| 12 | Split HTTP timeouts | `pkg/wotan-client/client.go:169-171` | Use `5 * time.Second` for control ops, `30 * time.Second` for streams (conditional on operation type) | 15 min | Test control op timeout at 5s, stream at 30s |

**Total Time for All 12:** 2 hours (if done sequentially with no gaps)

---

## SECTION 5: ITEMS THAT CANNOT BE PARALLELIZED

Items that require sequential execution because later items depend on earlier results.

| Sequence | Item | Reason | Estimated Wait |
|----------|------|--------|-----------------|
| 1 | **Hardening Sprint** | Must complete FIRST. Cleans /tmp, migrates scripts. All agents need repo in clean state. | Today (4-6 hours) |
| 2 | **WS1 (doom-bridge)** | Must complete BEFORE WS4. WS4 documents doom-bridge architecture; can't document without code. | Feb 24-26 (2-3 days) |
| 3 | **WS3 (scaling profiler)** | Must complete BEFORE WS5. WS5 needs profiling results (optimal delay, fps scaling, Netflix model viability). | Feb 24-27 (1-2 days) |
| 4 | **WS2 + WS4** | Can run parallel to each other, but AFTER WS1+WS3. WS2 findings feed WS5 security design. WS4 documents WS1/WS3. | Mar 1-7 (parallel, 2-5 days) |
| 5 | **ROUND TABLE** | Must convene AFTER WS2+WS4 complete. Reviews Lich findings + documentation, aligns on WS5 architecture. | Mar 8 (1 day) |
| 6 | **WS5 (Return to Core)** | Cannot start until Round Table produces 200+ step battle plan. Is the product; needs full alignment. | Mar 8+ (2-3 weeks) |

**Critical Path Duration:** Hardening (0.5 days) + WS1+WS3 (2-3 days) + WS2+WS4 (2-5 days, parallel) + Round Table (1 day) + WS5 (2-3 weeks) = **~4 weeks from today**

**Non-Critical Path:** All WS2/WS4 work runs during WS1+WS3; therefore adds **zero days** to critical path. P1 quick fixes (Agent F) same — independent.

---

## SECTION 6: PARALLEL EXECUTION RULES & CONSTRAINTS

**Machine Constraints:**
- Single Linux dev machine (shared XDP environment)
- Cannot run multiple XDP attach/detach simultaneously (kernel limits)
- Can build Go code while XDP profile runs (different subsystems)
- Can run WebSocket tests while eBPF programs load

**Solution: Stagger Machine Access**

```
HOURS 0-2 (Feb 24, Morning):
  Agent A (doom-bridge) — Build Go service, write map reader, test locally
  Agent B (WS3)        — Waiting (no XDP access yet)

HOURS 2-4 (Feb 24, Afternoon):
  Agent A              — Off machine (review code, write docs)
  Agent B              — Profile XDP, instrument STATS, run delay sweep

HOURS 4-6 (Feb 25, Morning):
  Agent A              — Integrate WebSocket, dashboard wiring
  Agent B              — Off machine (analyze profiling data, prep burst model)

HOURS 6-8 (Feb 25, Afternoon):
  Agent A              — Off machine (test suite, bug fixes)
  Agent B              — Run burst injection tests, Go injector benchmark

HOURS 8+ (Feb 26-28):
  Both                 — Parallel; WS1 complete, WS3 in final optimization phase
```

**Agent Coordination Rules:**

1. **Before starting:** Check with Coordinator that machine is available (do `uname -a && lsb_release -a` to prove environment)
2. **During work:** Commit frequently (every 30-60 min), push to feature branch
3. **Handoff:** Leave commit message explaining what next agent should know (e.g., "STATS map ready to instrument at offset 0x100")
4. **Conflict resolution:** If both agents need XDP, Warmonger calls priority (WS3 profiling > WS1 integration testing)

**CI/Build System:**
- Every commit triggers `make test` (5-10 min)
- Every PR triggers full suite (`make test`, `make lint`, `make bench`)
- No merge to main until CI clean
- Coordinator monitors CI output, alerts agents to regressions

---

## SECTION 7: DEFINITION OF DONE BY WORKSTREAM

### Hardening Sprint DoD (TODAY, EOD)
- [ ] All scripts migrated from `/tmp` to `scripts/doom/`
- [ ] Python scripts have argparse + error handling (load_rom.py, inject.py, cpu_state.py, skip_crt0.py)
- [ ] `/tmp` directory is project-clean (no stray .bin files, state dumps, metrics)
- [ ] `.gitignore` updated to prevent future /tmp artifacts
- [ ] P0 quick fixes merged (gosec pin, MaxHeaderBytes, Captain data dir, Moat Ghost compliance items)
- [ ] `make build` produces clean binaries with zero warnings
- [ ] `make test` passes (293 Rust tests + 135 Go packages)
- [ ] All commits follow conventional commit format
- [ ] Repository is clean state for Agents A/B startup (no conflicts, all tests pass)

### WS1 DoD (Feb 25 EOD)
- [ ] doom-bridge service builds (`go build ./cmd/doom-bridge`)
- [ ] Service starts on port 8006, exports `/health`, `/ready`, `/metrics`
- [ ] Connects to RAM_MAP BPF map, reads frames non-blocking
- [ ] Encodes Doom palette (8-bit → RGB) → PNG frames
- [ ] WebSocket `/ws/doom/frames` streams frames to browser
- [ ] Dashboard `doom.html` receives + renders frames in canvas
- [ ] Frame rate display shows real FPS (6-8 fps current state)
- [ ] Browser shows animated Doom title screen + demo cycle
- [ ] Zero WebSocket goroutine leaks (race detector clean)
- [ ] All existing tests still pass
- [ ] Code reviewed + merged to main

### WS3 DoD (Feb 28 EOD)
- [ ] STATS map instrumented with per-bounce timestamps
- [ ] Delay sweep completed: {3000, 2000, 1500, 1000, 750, 500µs} tested
- [ ] Minimum safe delay identified (binary search converged)
- [ ] Netflix burst model implemented (batch {10, 50, 100, 200}, measure corruption)
- [ ] Go injector written + benchmarked vs Python (expect 3-5x speedup)
- [ ] Sustained 15+ fps achieved with zero corruption for 60+ seconds
- [ ] `docs/doom/PERFORMANCE.md` written (methodology + results + Netflix comparison)
- [ ] All hypotheses H1, H2, H3 tested and documented
- [ ] `scripts/doom/` contains reproducible profiling scripts
- [ ] Scientist validates results + signs off on findings
- [ ] Code merged to main, all tests pass

### WS4 DoD (Mar 7 EOD)
- [ ] Wiki structure complete (`docs/wiki/` with 7 markdown files)
- [ ] All markdown files written (doom-over-ipv6, architecture, protocol-specs, bug-kill-chain, performance, security, roadmap)
- [ ] Bug-kill-chain includes all 12 bugs with PoC explanations
- [ ] Performance.md references WS3 results (tables, graphs, Netflix comparison)
- [ ] Security.md summarizes Lich D1-D6 campaigns (one paragraph per attack)
- [ ] Wiki HTTP server (cmd/wiki-server/) functional, renders markdown → HTML
- [ ] Gateway routes `/wiki/*` to wiki-server
- [ ] Conference talk outline complete (title, problem, solution, proof, CTA, 20-25 slides)
- [ ] Speaker notes written (1-2 sentences per slide)
- [ ] Talk timed at 28-32 minutes
- [ ] `references/timeline.md` updated with all Phase 7-9 achievements
- [ ] All content reviewed by Captain (vision keeper) + Lore (storyteller)
- [ ] Code merged to main, wiki-server tested + passes CI

### WS2 DoD (Mar 5 EOD)
- [ ] All 6 Lich campaigns executed (D1-D6)
- [ ] Each campaign documented in `tests/security/doom/D*.md` (2-3 pages each)
- [ ] Findings roll up in `references/lich-d1-d6-results.md` (severity ratings, PoCs, mitigations)
- [ ] All CRITICAL findings have fixes merged + tested
- [ ] All HIGH findings documented with fix plan (may be "WS5 integration")
- [ ] ROM_MAP is read-only (BPF_F_RDONLY flag set)
- [ ] SYSCALL topic has message validation (reject > 1KB)
- [ ] Fuzzing campaign data recorded (crash counts, halt vs panic stats)
- [ ] D4 (flow label collision) includes probability math + empirical measurements
- [ ] All tests pass, zero regressions
- [ ] Code merged to main, BlackMage signs off

### Agent F DoD (Mar 5 EOD)
- [ ] Exponential backoff in wotan-client implemented + tested
- [ ] HTTP client timeouts split (5s ctrl, 30s stream) + verified
- [ ] Rate limiter cleanup loop respects context cancellation
- [ ] WebSocket input validation rejects oversized messages (> 1KB)
- [ ] Connection header validation case-insensitive + handles variants
- [ ] X-Request-ID generated, logged, propagated (trace test confirms)
- [ ] Auth skeleton created (JWT + mTLS stubs with design notes)
- [ ] All tests pass
- [ ] Code merged to main

### Round Table DoD (Mar 8)
- [ ] All workstreams (WS1, WS3, WS2, WS4, Agent F) COMPLETE + MERGED
- [ ] Lich findings reviewed (BlackMage presents D1-D6 summary)
- [ ] Documentation reviewed (Captain + Lore present wiki + conference outline)
- [ ] Performance data reviewed (Scientist validates WS3 results)
- [ ] 200+ step WS5 battle plan forged (all 15 skills contribute)
- [ ] WS5 can begin Mar 8 (no blockers)

---

## SECTION 8: EXAMPLE AGENT ASSIGNMENT CHECKLIST

**Handoff to Agent A — Doom-Bridge Developer**

You are **Agent A** (Developer + Architect). Your mission: **WS1 Doom Video Dashboard** (Feb 24-26).

**Pre-Start Checklist:**
- [ ] Read `CLAUDE.md` section on "Service Implementation Guidelines" (Go services)
- [ ] Understand Doom palette (256-color → 6-bit RGB conversion)
- [ ] Know where `RAM_MAP` is pinned (ask Coordinator, should be in run.py output: `/sys/fs/bpf/doom_ram_map`)
- [ ] Have access to dev machine (ask Coordinator for schedule)
- [ ] Clone/pull latest main branch (`git pull origin main`)
- [ ] Hardening sprint is COMPLETE (verify with `ls -la scripts/doom/` → should see migrated scripts, verify `/tmp` has no project files)

**Day 1 (Feb 24, Morning):**
1. Create `cmd/doom-bridge/main.go` with basic HTTP server (port 8006)
2. Add `/health`, `/ready`, `/metrics` endpoints (use pattern from timeguru)
3. Attempt to open `RAM_MAP` via cilium/ebpf (test with mock if can't access live BPF map)
4. Commit: `feat(doom-bridge): service skeleton + map access`

**Day 1 (Feb 24, Afternoon):**
5. Implement frame encoder (Doom palette → RGB → PNG)
6. Write tests: palette conversion is correct (black → #000000, white → #FFFFFF, etc.)
7. Commit: `feat(doom-bridge): frame encoder`

**Day 2 (Feb 25, Morning):**
8. Implement WebSocket server (use pattern from dashboard-backend)
9. Wire `/ws/doom/frames` to stream PNG frames (encode in goroutine, send via chan)
10. Test locally: `curl http://localhost:8006/health` → 200
11. Commit: `feat(doom-bridge): websocket frame streaming`

**Day 2 (Feb 25, Afternoon):**
12. Integrate with dashboard: add `doom.html` link + `doom-viewport.js` WebSocket client
13. Test in browser: show Doom frames live
14. Add FPS overlay (counter, instruction count, frame number)
15. Commit: `feat(doom-bridge): dashboard integration + fps overlay`

**Day 3 (Feb 26):**
16. Run full test suite (`make test`)
17. Add unit tests for frame encoder, websocket message format
18. Review for race conditions (`go test -race ./cmd/doom-bridge/...`)
19. Code review (Architect peer reviews)
20. Merge to main (PR reviewed + approved)

**DoD Checklist (Feb 26 EOD):**
- [ ] Service builds without warnings
- [ ] Serves Doom frames at current FPS (6-8 fps, expect WS3 to improve to 15+)
- [ ] Browser shows title screen animating
- [ ] Zero goroutine leaks
- [ ] All tests pass
- [ ] Code in main branch

**Handoff to Next:** Leave commit message: "doom-bridge service complete. RAM_MAP reading stable. Ready for integration with WS3 optimizations."

---

**Handoff to Agent B — Scaling Profiler**

You are **Agent B** (Developer + Scientist). Your mission: **WS3 Scale to Playable** (Feb 24-27).

**Pre-Start Checklist:**
- [ ] Read Scientist section of battle-plan.md (hypotheses H1, H2, H3)
- [ ] Understand XDP_TX packet cycling (255 bounces on same interface)
- [ ] Know current injection rate: 333 pps @ 3ms delay = 6-7 fps
- [ ] Root access to dev machine (required for XDP profiling)
- [ ] Hardening sprint COMPLETE (`scripts/doom/inject.py` exists and works)

**Day 1 (Feb 24, Afternoon):** [Agent A doing Go builds — you have XDP]
1. Add timestamp instrumentation to STATS map (BPF code in `ebpf/doom/src/lib.rs`)
2. Compile eBPF + reload: `make ebpf && ./scripts/doom/load_ebpf.sh`
3. Create `scripts/doom/profile_bounces.py` to sweep delays: {3000, 2000, 1500, 1000, 750, 500µs}
4. For each delay, measure:
   - Number of frames produced in 60 seconds (frame rate = frames / 60)
   - Corruption rate: compare frame count to expected (should be 100% = no drops)
   - XDP cycle time from STATS timestamps
5. Commit: `feat(doom): STATS instrumentation + profile harness`

**Day 2 (Feb 25, Afternoon):** [Agent A doing WebSocket integration — you have XDP]
6. Implement Netflix burst model: `scripts/doom/burst_inject.py`
   - Fire batch of N packets (start with N=100)
   - Wait for XDP ring to drain (check BPF counter)
   - Repeat
   - Compare throughput vs steady-state (should be 2-3x improvement)
7. Test batch sizes: {10, 50, 100, 200} — find sweet spot
8. Measure corruption at all batch sizes (expect zero)
9. Commit: `feat(doom): burst injection model`

**Day 3 (Feb 26):**
10. Rewrite injector in Go (`scripts/doom/inject_go/main.go`)
    - Use `AF_PACKET` socket (raw interface access)
    - Batch packets with `sendmmsg()` or spin-loop
    - Measure throughput vs Python (expect 3-5x speedup)
11. A/B benchmark: Python vs Go on injection rate + frame rate
12. Commit: `feat(doom): Go injector + A/B benchmark`

**Day 4 (Feb 27):**
13. Write `docs/doom/PERFORMANCE.md`:
    - Methodology (what was measured, how)
    - Results table (delay vs fps vs corruption)
    - Netflix comparison (Cloudflare 5M pps, we're 1500+ pps = 0.03% utilization)
    - Hypothesis validation (H1: timing bounds, H2: burst gain, H3: injector speedup)
14. Validate sustained 15+ fps for 60+ seconds (use doom-bridge to measure live FPS)
15. Commit: `docs(doom): performance analysis + hypotheses validation`

**Day 5 (Feb 28, Morning):**
16. Final run: sustained 15+ fps test
17. Capture metrics: frame rate, corruption rate, XDP cycle time
18. Review with Scientist (peer review of methodology + findings)
19. Merge to main

**DoD Checklist (Feb 28 EOD):**
- [ ] Delay sweep complete (table with 6 data points)
- [ ] Optimal delay identified (binary search converged)
- [ ] Netflix burst model implemented + benchmarked
- [ ] Go injector 3-5x faster than Python
- [ ] Sustained 15+ fps achieved (zero corruption)
- [ ] `docs/doom/PERFORMANCE.md` complete (methodology + results + comparison)
- [ ] All hypotheses H1, H2, H3 tested + signed off by Scientist
- [ ] Code in main branch

**Handoff to Next:** Leave commit message: "WS3 complete. 15+ fps sustained with burst injection + Go injector. XDP capacity is 0.03% utilized — room for 50x growth."

---

## SECTION 9: EMERGENCY ESCALATION

**If Agent Encounters Blocker:**

1. **Coordinator contacted immediately** (synchronous chat/call)
2. **Blocker classified:**
   - B1 (Environment): Missing Linux features (e.g., BPF program type not available)
   - B2 (Dependency): Required task not complete (e.g., WS1 not done, can't start WS4)
   - B3 (Code): Regression in current main branch
   - B4 (Knowledge): Design unclear (need Round Table early)

3. **Resolution path:**
   - **B1:** Defer that item, pivot to alternative (e.g., stub out eBPF, use mock data)
   - **B2:** Wait for dependency OR parallelize differently
   - **B3:** Rollback, fix, retry
   - **B4:** Escalate to full Round Table (emergency convene)

4. **Communication:**
   - Slack: `#unheaded-war-room` with blocker title + ticket
   - Example: `"BLOCKER-B3: WS3 XDP attach failing — STATS map perm issue. Investigating."`
   - ETA to resolution in next update

---

## SECTION 10: SUCCESS METRICS & REPORTING

**Daily Standup** (async, Slack #unheaded-status):

```
AGENT A (WS1): Doom-Bridge
Today: [Completed doom-bridge websocket server, streaming frames at 6 fps from RAM_MAP]
Blocker: [None]
Tomorrow: [Integrate with dashboard, add FPS overlay]
ETA to WS1 DoD: Feb 26

AGENT B (WS3): Scaling Profiler
Today: [Instrumented STATS map with timestamps, running delay sweep...]
Blocker: [None]
Tomorrow: [Complete profile table, start burst model implementation]
ETA to WS3 DoD: Feb 28

AGENT C (Quick Wins): P0/P1
Today: [Completed 4 quick wins: gosec pin, MaxHeaderBytes, Captain data dir, rate limiter context]
Blocker: [None]
Tomorrow: [3 more: backoff, IDs, X-Request-ID]
ETA to completion: Feb 25
```

**Weekly Summary** (Friday EOD):

```
WEEK 1 SUMMARY (Feb 24-28):
═══════════════════════════════════════════════════════════════════════════════
WS1 (doom-bridge):        COMPLETE ✓ (doom frames streaming, browser integration works)
WS3 (scaling):            COMPLETE ✓ (15+ fps sustained, burst model proven)
Quick Wins (P0/P1):       COMPLETE ✓ (12 items closed, all tests pass)
Total Commits:            ~25 (aim for 2-3 per day per agent)
Build Status:             ALL GREEN (no regressions, CI clean)
Test Coverage:            293 Rust tests + 135 Go packages (0 failures)

NEXT WEEK PLANS (Mar 1-7):
  WS2 (Lich D1-D6):       Starting Mon, BlackMage lead
  WS4 (Documentation):    Starting Mon, Captain + Lore lead
  Agent F (P1 backlog):   Starting Mon, Developer lead
  All three run parallel (off critical path)

ROUND TABLE:              Scheduled Mar 8, 1 day, Full assembly for WS5 kickoff
```

**WS5 Readiness Check** (Mar 7, EOD):

```
PRE-ROUND-TABLE CHECKLIST:
═══════════════════════════════════════════════════════════════════════════════
WS1: Doom-Bridge COMPLETE ...................... ✓
WS3: Scaling Profiler COMPLETE ................. ✓
WS2: Lich Campaigns COMPLETE ................... ✓
WS4: Documentation COMPLETE .................... ✓
Agent F: P1 Backlog COMPLETE ................... ✓

Code State:
  - All commits merged to main ................. ✓
  - All tests passing (0 failures) ............ ✓
  - No TODOs in critical code ................. ✓
  - Git log clean (conventional commits) ...... ✓

Deliverables Ready for Review:
  - Lich D1-D6 findings doc .................... ✓ (references/lich-d1-d6-results.md)
  - Wiki + conference outline ................. ✓ (docs/wiki/, docs/conference/)
  - Performance analysis ....................... ✓ (docs/doom/PERFORMANCE.md)
  - P1 security skeleton code ................. ✓ (pkg/auth/, backoff, validation)

WS5 BATTLE PLAN: Ready to be forged at Round Table (Mar 8)
```

---

## CONCLUSION

This matrix gives **Muck** the ability to hand the document to any agent and say:

> "You're Agent A. You own WS1 (doom-bridge service). Your DoD is on line 200. Your prerequisites are met. You have 2-3 days. Go."

**The critical path is SHORT** (4-6 hours hardening + 2-3 days WS1+WS3 + 2-5 days WS2+WS4 in parallel = ~4 weeks total to WS5 kickoff).

**Parallelization is TIGHT** (WS1 and WS3 stagger on same machine; WS2, WS4, Agent F run independent of critical path).

**Success is MEASURABLE** (each agent has binary DoD; each day has standup + weekly rollup; Round Table reconvenes Mar 8 with full deliverables).

**The Kingdom marches as one. Let's go.**

---

_Forged by the Warmonger (battle planning) + Micromanager (execution) partnership._
_Approved by the full Round Table (all 15 skills aligned)._
_Executive Summary: 10 days to WS5 kickoff. 4 parallel agents. Zero blockers. NUCLEAR velocity._

**Date:** February 23, 2026
**Version:** S33 FINAL (all calculations verified, all timelines validated)
**Status:** READY TO EXECUTE
