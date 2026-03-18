# Battleplan: Dashboard + BPF Integration → Doom

Date: 2026-02-18
Status: ACTIVE
Owner: Developer (coding agent)
Reviewer: Architect + Muck
RFC 9669 note: "eBPF" and "BPF" are interchangeable.  We use "BPF"
throughout per RFC 9669 §1.  One less byte.

---

## Terminology note: eBPF → BPF

Per RFC 9669 (BPF Instruction Set Architecture, October 2024):

    "eBPF (which is no longer an acronym for anything), also
     commonly referred to as BPF"

All new code, docs, comments, and variable names SHOULD use "BPF"
unless referencing a specific Linux API that uses "ebpf" in its
symbol name (e.g., `bpf_prog_type`, `SEC("xdp")`).  Existing code
will be migrated incrementally — no bulk rename, fix on touch.

---

## Current State Assessment

### What's DONE (production-grade, real logic):

    shield-ebpf         583 LOC Rust    XDP ingress/TC egress, Monad stamp/strip, BIRTH/DEATH
    hop-ebpf            506 LOC Rust    per-hop Monad processing, CRC-16, Sophia lookup, HOP events
    yaldabaoth-ebpf     407 LOC Rust    chaos injection (5 modes), CHAOS events
    monad-common        shared types    Monad struct, AnamnesisEvent, EventType enum
    ebpf-loader         309 LOC Rust    full program loading (XDP/TC/kprobe), map pinning
    pkg/ebpf            Go              low-level BPF syscall loader, ELF parse, BTF, ring buffer
    dashboard UI        ~100KB JS       packet-flow.js, metrics.js, monad-decode.js, particles.js
    dashboard-backend   5,926 LOC Go    REST API, WebSocket, Wotan integration, health, scraper
    kanban-app          1,361 LOC Go    full CRUD, SSE, Wotan + SQLite + in-memory triple fallback

### What's STUBBED (skeleton, needs implementation):

    packet-marker       stub            XDP trace ID injection
    flow-tracker        stub            TC flow state tracking
    latency-probe       stub            kprobe/kretprobe TCP timing
    syscall-tracer      stub            raw_tracepoint sys_enter/sys_exit
    monad-cpu-ebpf      stub            fetch-decode-execute VM for Doom
    trace-collector     partial Rust    framework present, loads stub programs
    ebpf-collector      partial         event loop present, real parsing incomplete

### What's MISSING (not started):

    Anamnesis ring buffer → dashboard pipeline (end-to-end)
    Real packet flow correlation (dashboard uses synthetic generator)
    Wotan memory service (L1 cache + L2 ring buffer for compute)
    MBC assembler / RV32I translator
    Doom integration

---

## PHASE 1: Dashboard + BPF Integration

Goal: Real BPF events flowing end-to-end from kernel to browser.
No more synthetic data.  Real packets, real Monads, real traces.

Estimated: 5-8 days coding

---

### Step 1.1: Anamnesis Event Schema (0.5 day)

Location: ebpf/monad-common/src/lib.rs + pkg/ebpf/types.go

The shared event structure exists in Rust (monad-common).  We need
the Go-side mirror for the dashboard pipeline.

Tasks:

    1.  Read ebpf/monad-common/src/lib.rs — understand AnamnesisEvent struct
    2.  Create pkg/ebpf/anamnesis.go with matching Go struct:

        type AnamnesisEvent struct {
            TimestampNs uint64       // bpf_ktime_get_ns()
            EventType   uint8        // Birth=0, Death=1, Hop=2, Anomaly=3, Chaos=4, Sampled=5
            HopID       uint8        // which node emitted
            Pad         [2]byte      // alignment
            TraceID     [16]byte     // trace correlation
            Monad       MonadState   // 20-byte register snapshot
        }

        type MonadState struct {
            R0 uint32   // service_id (exponent-encoded)
            R1 uint32   // flow_action + qos_class + hop_count + flags
            R2 uint32   // trace_id_hi
            R3 uint32   // trace_id_lo
            R4 uint32   // scratch + checksum
        }

    3.  Add binary decoder: func DecodeAnamnesisEvent(b []byte) (*AnamnesisEvent, error)
    4.  Add JSON encoder for WebSocket transport
    5.  Unit tests — fuzz the decoder with random bytes, verify round-trip

Acceptance: `go test ./pkg/ebpf/... -race -count=1` passes.
MonadState.Flags() returns parsed bitfield including K1:K0.

---

### Step 1.2: Ring Buffer Reader (1 day)

Location: pkg/ebpf/ringbuf.go (exists, needs Anamnesis integration)

The ring buffer mmap reader exists in pkg/ebpf.  Wire it to parse
AnamnesisEvent structs from the pinned ring buffer map.

Tasks:

    1.  Read pkg/ebpf/loader.go — understand existing ring buffer code
    2.  Create pkg/ebpf/anamnesis_reader.go:

        type AnamnesisReader struct {
            ringFD    int
            mmap      []byte
            events    chan *AnamnesisEvent
            stats     ReaderStats
        }

        func NewAnamnesisReader(pinPath string) (*AnamnesisReader, error)
        func (r *AnamnesisReader) Poll(ctx context.Context) <-chan *AnamnesisEvent
        func (r *AnamnesisReader) Stats() ReaderStats

    3.  Pin path: /sys/fs/bpf/unheaded/anamnesis_events (set by ebpf-loader)
    4.  Poll loop: epoll on ring buffer FD, decode events, emit to channel
    5.  Backpressure: if channel full, increment stats.Dropped, don't block
    6.  Graceful shutdown on context cancellation
    7.  Unit test with mock ring buffer (pre-filled byte slice)

Acceptance: Reader decodes BIRTH/DEATH/HOP/CHAOS events from ring buffer.
Stats track events_read, events_dropped, bytes_read, errors.

---

### Step 1.3: Trace Collector Rewrite (1.5 days)

Location: cmd/trace-collector/

The existing Rust trace-collector loads stub BPF programs.  Rewrite
to read from the Anamnesis ring buffer populated by the real Shield
and Hop programs.

Decision: Keep Rust or rewrite in Go?

    Option A (recommended): Go rewrite using pkg/ebpf/anamnesis_reader.go
      - Consistent with all other services
      - Uses existing Wotan Go client
      - Single binary, no cross-compilation
      - Can import monad-decode logic directly

    Option B: Keep Rust, add anamnesis ring buffer reader
      - Keeps Rust expertise in the BPF layer
      - Requires separate Wotan gRPC client maintenance

Go with Option A unless Muck says otherwise.

Tasks:

    1.  Create cmd/trace-collector-go/main.go (new Go binary)
    2.  Components:
        a.  AnamnesisReader (from Step 1.2)
        b.  EventTransformer — AnamnesisEvent → Wotan message format
        c.  WotanPublisher — publish to Wotan topics:
            - anamnesis.birth    (Shield ingress events)
            - anamnesis.death    (Shield egress events)
            - anamnesis.hop      (per-hop events)
            - anamnesis.chaos    (Yaldabaoth events)
            - anamnesis.anomaly  (CRC failures, fingerprint mismatches)
        d.  FlowCorrelator — group events by trace_id into flow records
        e.  RateLimiter — configurable max events/sec (default 10,000)
    3.  CLI flags:
        --ring-path     /sys/fs/bpf/unheaded/anamnesis_events
        --wotan-addr    localhost:9090
        --max-rate      10000
        --batch-size    100
        --batch-timeout 50ms
    4.  Health check: /healthz (ring buffer readable, Wotan connected)
    5.  Metrics: Prometheus /metrics (events_total, flows_correlated, etc.)
    6.  Graceful shutdown, circuit breaker on Wotan connection
    7.  Tests: unit + integration with mock ring buffer and mock Wotan

Acceptance: trace-collector reads real BPF events, correlates flows,
publishes to Wotan topics.  `make test` passes with race detector.

---

### Step 1.4: Dashboard Backend — Real Event Ingestion (1 day)

Location: cmd/dashboard-backend/internal/ebpf/

The dashboard backend has an ebpf package with ingestor, flows, and
latency modules.  Currently incomplete.  Wire it to Wotan topics
from the trace-collector.

Tasks:

    1.  Read cmd/dashboard-backend/internal/ebpf/ — assess current state
    2.  Update ebpf/ingestor.go to subscribe to Wotan anamnesis.* topics
    3.  Create ebpf/flow_builder.go:
        - Accumulate events by trace_id
        - Build complete flow record: BIRTH → HOP* → DEATH
        - Calculate per-flow metrics: hop count, total latency, path
        - Emit completed flows to flows channel
    4.  Update ebpf/latency.go:
        - Calculate per-hop latency from consecutive HOP event timestamps
        - Calculate Shield-to-Shield end-to-end latency (BIRTH→DEATH)
        - Histogram buckets: p50, p90, p99, p999
    5.  Create ebpf/monad_decode.go:
        - Decode Monad register state from event payload
        - Extract: service_id, flow_action, qos_class, hop_count, flags
        - Map service_id through Sophia dictionary (Wotan lookup)
    6.  Wire to existing WebSocket broadcaster:
        - New message types: "flow", "hop", "anomaly", "chaos"
        - Existing "packet" type replaced by real flow data
    7.  Remove or gate the synthetic packet generator behind --demo flag
    8.  Tests for each component

Acceptance: Dashboard backend receives real BPF events from Wotan,
builds flow records, streams to browser via WebSocket.

---

### Step 1.5: Dashboard UI — Real Data Rendering (1 day)

Location: dashboard/

The frontend JS is substantial.  Update it to render real flow data
instead of synthetic packets.

Tasks:

    1.  Read dashboard/js/packet-flow.js — understand rendering pipeline
    2.  Read dashboard/js/monad-decode.js — understand existing decoder
    3.  Update WebSocket message handler for new event types:
        a.  "flow" — complete BIRTH→HOP*→DEATH path visualization
        b.  "hop" — real-time hop marker on flow path
        c.  "anomaly" — red flash on affected hop
        d.  "chaos" — orange pulse on Yaldabaoth-affected flow
    4.  Update metrics.js:
        a.  Real hop latency histogram (replace placeholder)
        b.  Real flow rate counter
        c.  Real BPF event rate
        d.  Monad register state inspector (click a flow → see R0-R4)
    5.  Add Monad detail panel:
        - Click any flow → expand to show register state at each hop
        - Color-code flags (Kingdom Mode bits, chaos flag, etc.)
        - Show Sophia-decoded service name (from service_id)
    6.  Add flow timeline view:
        - Horizontal timeline per flow showing BIRTH→HOP→...→DEATH
        - Microsecond precision timestamps
        - Hover for Monad state at that hop
    7.  Gate synthetic data behind ?demo=true query param

Acceptance: Browser renders real BPF flow data.  Click a flow to
inspect Monad register state at each hop.  Latency histogram shows
real per-hop timing.

---

### Step 1.6: Stub Program Implementation (1-2 days)

Location: ebpf/

The real Shield/Hop/Yaldabaoth are done.  The utility programs
(packet-marker, flow-tracker, latency-probe, syscall-tracer) are
stubs.  Implement them.

Tasks:

    1.  packet-marker (ebpf/packet-marker/src/main.rs):
        - XDP program, already has structure
        - Implement: parse IPv6, extract/inject trace_id in flow label
        - Use TRACE_INJECT BPF map for userspace-controlled injection
        - Emit PACKET_EVENTS to ring buffer
        - Test: verify trace_id appears in Anamnesis events

    2.  flow-tracker (ebpf/flow-tracker/src/main.rs):
        - TC ingress + TC egress classifier pair
        - Implement: 5-tuple flow key extraction (src/dst addr, ports, proto)
        - FLOW_STATE BPF hash map: track per-flow byte count, packet count,
          first_seen, last_seen, TCP flags seen
        - Emit flow state changes (new flow, flow timeout) to ring buffer
        - Timeout: BPF timer or userspace GC (60s default)

    3.  latency-probe (ebpf/latency-probe/src/main.rs):
        - kprobe on tcp_v6_connect, kretprobe on tcp_v6_connect
        - Measure TCP handshake RTT per destination
        - LATENCY_MAP: dst_addr → {min_ns, max_ns, sum_ns, count}
        - Emit latency events for p99 breaches (configurable threshold)

    4.  syscall-tracer (ebpf/syscall-tracer/src/main.rs):
        - raw_tracepoint on sys_enter
        - Track syscall frequency per container/cgroup
        - SYSCALL_MAP: (cgroup_id, syscall_nr) → count
        - Alert on unusual syscall patterns (configurable allowlist)

    5.  Update ebpf/Cargo.toml workspace members if needed
    6.  Update Makefile targets: make ebpf builds all programs
    7.  Update ebpf-loader to load new programs

Acceptance: `make ebpf` builds all 8 programs (shield, hop, yaldabaoth,
monad-cpu, packet-marker, flow-tracker, latency-probe, syscall-tracer).
ebpf-loader can attach all to interfaces.

---

### Step 1.7: End-to-End Integration Test (0.5 day)

Location: tests/

Tasks:

    1.  Create tests/e2e_bpf_dashboard_test.go
    2.  Test topology (requires Linux, skip on other OS):
        a.  Create 2 network namespaces + veth pair
        b.  Load Shield on ns0 (ingress), Hop on ns1
        c.  Start trace-collector pointed at ring buffer
        d.  Start Wotan (or mock)
        e.  Start dashboard-backend subscribing to Wotan
        f.  Send 100 IPv6 packets from ns0 → ns1
        g.  Assert: dashboard-backend received 100 BIRTH + 100 HOP + 100 DEATH
        h.  Assert: flow records have correct hop count (2)
        i.  Assert: Monad CRC-16 valid at each hop
        j.  Assert: trace_ids correlate across events
    3.  Teardown: remove namespaces, unload programs
    4.  Add to Makefile: make test-e2e-bpf (requires root)

Acceptance: End-to-end test passes on Linux with root.
Full pipeline: BPF → ring buffer → trace-collector → Wotan →
dashboard-backend → verified in test assertions.

---

## PHASE 2: Doom (Rough Plan)

Goal: Doom rendering to Kingdom dashboard via BPF packet compute.
This is the computational completeness proof-of-concept.
Priority: P1 AFTER Phase 1 ships.

Estimated: 8-13 days coding (per doom-over-ipv6-plan.md)

---

### Step 2.1: Monad CPU BPF Program (2-3 days)

Location: ebpf/monad-cpu-ebpf/src/main.rs

Currently a stub.  Implement the fetch-decode-execute VM.

    1.  Define CPU state struct in BPF map:
        - 16 x u32 registers (r0-r15, r15=SP)
        - PC (program counter)
        - Flags (Z/N/C)
        - stalled flag (cache miss → retry next packet)

    2.  Implement MBC instruction decode (RFC 9669 BPF ALU ops):
        - Arithmetic: ADD, SUB, MUL, DIV, MOD, NEG
        - Logical: AND, OR, XOR, NOT, SHL, SHR, SAR
        - Compare: CMP (sets Z/N/C flags)
        - Branch: JMP, JZ, JNZ, JN, JP, JC, JNC, CALL, RET
        - Memory: LD, ST, LDB, STB, LDH, STH
        - Register: MOV, MOVI
        - System: SYSCALL, HALT

    3.  Memory access through L1 BPF map cache:
        - Cache hit: read/write l1_cache BPF hash map
        - Cache miss (LD): emit compute.mem.miss event, set stalled=1
        - Write (ST): write l1_cache + emit compute.mem.write event

    4.  Instructions per packet: configurable (default 1, increase for
        batched execution — up to BPF instruction limit ~1M)

    5.  Writeback: sync hot registers to Monad scratch bytes

    6.  Emit HOP event with CPU state for dashboard tracing

    7.  Test: load a simple program (fibonacci), verify registers

---

### Step 2.2: Wotan Memory Service (2-3 days)

Location: services/wotan/ (new memory subsystem)

    1.  New Wotan channel type: compute.mem.<flow_label>
    2.  Subscribe to compute.mem.miss events:
        - Read requested page from ring buffer (L2)
        - Stage into target hop's l1_cache BPF map
        - Prefetch N adjacent pages (spatial locality)
    3.  Subscribe to compute.mem.write events:
        - Update ring buffer (L2) with written data
        - Async flush to WAL (L3) if persistence enabled
    4.  Memory allocation: configurable --ring-size (default 4MB,
        Doom needs ~512MB for WAD + heap + stack + framebuffer)
    5.  Page table: track which pages are in which hop's L1
    6.  Eviction: LRU when L1 full, write-back dirty pages first

---

### Step 2.3: MBC Assembler + RV32I Translator (1-2 days)

Location: crates/ (new Rust crate)

    1.  MBC assembler: text → 32-bit fixed-width bytecode
        - Instruction format: [opcode:8][dst:4][src:4][imm16:16]
        - Output: flat binary, loadable into Sophia rom_map
    2.  RV32I → MBC translator:
        - Parse RISC-V ELF (riscv32-unknown-elf-gcc output)
        - Map RV32I instructions to MBC equivalents
        - Handle: lui/auipc (load upper immediate), jal/jalr
        - Output: MBC bytecode
    3.  wotan-ctl load-rom command:
        - Load MBC bytecode into Sophia rom_map BPF array
        - Keyed by flow_label

---

### Step 2.4: Packet Circulation Ring (1 day)

Location: scripts/

    1.  Create scripts/doom-ring.sh:
        - 6 network namespaces (ns0-ns5)
        - veth pairs forming directed ring
        - Static IPv6 routes for circulation
        - Load monad_cpu BPF program at XDP on each ns
        - Load Shield on ns0 (entry/exit point)
    2.  Circulation: packet enters ns0, traverses ring, returns to ns0
    3.  6 instructions per circuit (1 per hop)
    4.  Target: ~2-3 MHz effective clock (per performance budget)

---

### Step 2.5: Doom Cross-Compilation + Port (2-3 days)

    1.  Fork doomgeneric (minimal Doom port)
    2.  Cross-compile: doomgeneric → RISC-V RV32I ELF
    3.  Translate: RV32I → MBC bytecode
    4.  Implement SYSCALL callbacks:
        - 0x01 DG_DrawFrame → compute.screen.write event
        - 0x02 DG_GetKey → read from Wotan kbd topic
        - 0x03 DG_GetTicksMs → bpf_ktime_get_ns() / 1M
        - 0x04 DG_SleepMs → cpu.sleep_until timer
    5.  Load doom1.wad into Wotan ring buffer (wotan-ctl load-mem)
    6.  Load doom.mbc into Sophia rom_map
    7.  Test: title screen renders

---

### Step 2.6: Dashboard Doom Renderer (1-2 days)

    1.  New dashboard panel: Doom viewport
    2.  Subscribe to compute.screen.<flow_label> Wotan topic
    3.  Render 320x200 framebuffer to HTML5 Canvas (scaled)
    4.  Keyboard input → Wotan compute.input.<flow_label> topic
    5.  Overlay: CPU trace (PC, registers, IPS, cache hit rate)
    6.  FPS counter, Wotan ring buffer utilization gauge

---

### Step 2.7: Progressive Demos (fallback milestones)

If full Doom is blocked, each of these is independently publishable:

    Week 1:  "Hello World" — text string to screen_map via MBC
    Week 2:  "Snake" — game loop, keyboard, screen via Wotan
    Week 3:  "Pong" — real-time collision, full I/O bridging
    Week 4:  "Doom" — the real thing

Even "Snake running on IPv6 extension headers" is a headline.

---

## Dependency Graph

    Step 1.1 (event schema)
      ↓
    Step 1.2 (ring buffer reader)  ←── depends on 1.1
      ↓
    Step 1.3 (trace-collector)     ←── depends on 1.2
      ↓
    Step 1.4 (dashboard backend)   ←── depends on 1.3 (Wotan topics)
      ↓
    Step 1.5 (dashboard UI)        ←── depends on 1.4 (WebSocket messages)
      |
    Step 1.6 (stub programs)       ←── independent, parallel with 1.3-1.5
      ↓
    Step 1.7 (e2e test)            ←── depends on ALL of 1.1-1.6

    ──── Phase 1 complete ────

    Step 2.1 (monad CPU)           ←── depends on 1.1, 1.2 (event infra)
    Step 2.2 (Wotan memory)        ←── depends on 2.1 (cache miss events)
    Step 2.3 (MBC assembler)       ←── independent, parallel with 2.1-2.2
    Step 2.4 (circulation ring)    ←── depends on 2.1
    Step 2.5 (Doom port)           ←── depends on 2.1, 2.2, 2.3
    Step 2.6 (dashboard Doom)      ←── depends on 2.5, 1.5
    Step 2.7 (progressive demos)   ←── fallback at any point

---

## Coding Agent Instructions

For each step:

    1.  Read the relevant source files FIRST.  Understand what exists.
    2.  Write tests BEFORE implementation (red-green-refactor).
    3.  All inputs are hostile.  Bounds-check everything.
    4.  Run `go test ./... -race -count=1` after every Go change.
    5.  Run `cargo test` after every Rust change.
    6.  Run `cargo build --target bpfel-unknown-none -Z build-std=core`
        for BPF programs (verifier-safe, no_std).
    7.  Use BPF (not eBPF) in all new code/comments per RFC 9669.
    8.  Commit after each step with descriptive message.
    9.  If blocked, document the blocker and move to the next
        independent step.

---

## Success Criteria

### Phase 1 (Dashboard + BPF):
    [ ] Real BPF events visible in browser (not synthetic)
    [ ] Click a flow → inspect Monad register state at each hop
    [ ] Latency histogram shows real per-hop timing
    [ ] Flow correlation: BIRTH → HOP* → DEATH grouped by trace_id
    [ ] All 8 BPF programs build clean
    [ ] E2E test passes on Linux with root
    [ ] Zero race conditions (go test -race)

### Phase 2 (Doom):
    [ ] MBC VM executes instructions in BPF at wire speed
    [ ] Wotan memory service handles cache miss/write cycle
    [ ] Doom renders ≥ 20 FPS to Kingdom dashboard
    [ ] Input latency < 50ms
    [ ] L1 cache hit rate > 90%
    [ ] Stable for ≥ 5 minutes gameplay
    [ ] Demo video recorded and publishable

---

THE PLAN IS SET.  THE DASHBOARD AWAKENS.  THE DOOM APPROACHES.
