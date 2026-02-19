
q# DOOM OVER IPv6 — 20 Code Agent Tasks

**Date:** February 19, 2026
**Project Root:** `~/tmp/unheaded/`
**Context:** The Unheaded Protocol Foundation. Packets ARE the CPU. Wotan IS the RAM. BPF at XDP executes MBC instructions at every hop. Real IPv6 Hop-by-Hop Options. Real network namespaces. Real packet circulation ring. THE WIRE IS THE PROCESSOR.
**Required Reading:** `docs/protocol/doom-over-ipv6-plan.md`, `docs/protocol/PROTOCOL_MATH_AND_MAPS.md`, `docs/protocol/the_first_packet.md`, `docs/protocol/draft-bellis-unheaded-protocol-foundation-02.md`
**RFC Note:** Per RFC 9669 §1, "eBPF" and "BPF" are interchangeable. We use "BPF" throughout. One less byte.

---

## TASK D-001: monad-cpu-ebpf — Fetch-Decode-Execute Core

**Priority:** P0 — EVERYTHING DEPENDS ON THIS
**Scope:** `ebpf/monad-cpu-ebpf/src/main.rs` (STUB exists — rewrite)
**Shared types:** `ebpf/monad-common/src/lib.rs`
**Depends:** D-005, D-014

Implement the BPF XDP program that IS the CPU. Per-hop fetch-decode-execute cycle:

1. Locate Monad in IPv6 Hop-by-Hop Option header (20 bytes — R0-R4, five u32 registers)
2. Extract flow label from IPv6 header (20 bits) → key into `cpu_map` (per-flow CPU state)
3. Check stall/halt/sleep: if `cpu.stalled == 1` or `cpu.halted == 1` or `bpf_ktime_get_ns() < cpu.sleep_until`, skip execution, return `XDP_PASS`
4. FETCH: Read 32-bit instruction from `rom_map` BPF array at `cpu.pc`
5. DECODE: Extract `[opcode:8][dst:4][src:4][imm16:16]` from instruction word
6. EXECUTE: Arithmetic opcodes — ADD, SUB, MUL, DIV, MOD, NEG. Logical — AND, OR, XOR, NOT, SHL, SHR, SAR. Comparison — CMP sets Z/N/C flags.
7. Increment `cpu.pc` (non-branch instructions)
8. Increment `cpu.insn_count`
9. Write back Monad R0-R4 from cpu register file (r0-r4 map to Monad R0-R4)
10. Emit Anamnesis COMPUTE_HOP event via BPF ring buffer
11. Return `XDP_PASS` — packet continues to next hop

**Key constraint:** BPF programs execute ONE instruction per packet hop. PC persists in `cpu_map` across packets. The packet circulation ring is the clock. Six namespaces = six instructions per lap.

**Acceptance:**
- `cargo build --target bpfel-unknown-none -Z build-std=core` compiles clean
- BPF verifier accepts program (no unbounded loops — we only run one insn)
- Unit tests for each arithmetic/logical opcode

---

## TASK D-002: monad-cpu-ebpf — Branch & Control Flow Instructions

**Priority:** P0
**Scope:** `ebpf/monad-cpu-ebpf/src/main.rs`
**Depends:** D-001

Implement control flow in the BPF VM:

1. JMP (unconditional): `cpu.pc = imm16`
2. JZ/JNZ: branch on Z flag
3. JN/JP: branch on N flag (signed)
4. JC/JNC: branch on C flag (unsigned)
5. CALL: `cpu.regs[SP] -= 4; mem_write(cpu.regs[SP], cpu.pc + 1); cpu.pc = imm16`
6. RET: `cpu.pc = mem_read(cpu.regs[SP]); cpu.regs[SP] += 4`
7. MOV: `cpu.regs[dst] = cpu.regs[src]`
8. MOVI: `cpu.regs[dst] = imm16`

**Important:** CALL/RET use the L1 cache for stack memory (same path as D-003). Stack lives in Wotan RAM.

**Acceptance:** Test each branch type. Forward + backward jump. CALL/RET with nested calls (2 deep).

---

## TASK D-003: monad-cpu-ebpf — Memory Access via L1 BPF Map Cache

**Priority:** P0
**Scope:** `ebpf/monad-cpu-ebpf/src/main.rs`
**Depends:** D-001

Implement LD/ST/LDB/STB/LDH/STH memory instructions:

1. Compute effective address: `addr = cpu.regs[src] + sign_extend(imm16)`
2. LD (32-bit load):
   - Lookup `l1_cache` BPF hash map with `addr` key
   - Cache HIT: `cpu.regs[dst] = *val`, clear stall
   - Cache MISS: emit `CACHE_MISS` event via Anamnesis ring buffer (includes flow_label, addr, hop_id). Set `cpu.stalled = 1`. Return `XDP_PASS`. Wotan handles restaging. Next packet circulation retries.
3. ST (32-bit store): write to `l1_cache` BPF map AND emit `MEM_WRITE` event for Wotan async writeback to L2
4. LDB/STB (byte width), LDH/STH (halfword width) — same cache path, mask appropriately
5. PUSH: `cpu.regs[15] -= 4; ST cpu.regs[src] at cpu.regs[15]`
6. POP: `LD cpu.regs[dst] from cpu.regs[15]; cpu.regs[15] += 4`

**Critical:** Cache miss → stall → Wotan restage → retry is the core memory hierarchy. This is what makes Wotan the RAM.

**Acceptance:** Load/store round-trip. Cache miss event emission. Stack push/pop. Stall-and-retry sequence.

---

## TASK D-004: monad-cpu-ebpf — SYSCALL Handler (Wotan I/O Bridge)

**Priority:** P0
**Scope:** `ebpf/monad-cpu-ebpf/src/main.rs`
**Depends:** D-001, D-003

Implement SYSCALL opcode — the bridge between BPF compute and the Kingdom:

1. `SYSCALL 0x01` (DG_DrawFrame): `r1` = framebuffer base addr in Wotan RAM. Emit `SCREEN_WRITE` event with flow_label + addr. Wotan reads 64000 bytes (320×200) from L2 → dashboard WebSocket.
2. `SYSCALL 0x02` (DG_GetKey): Read from `kbd_map` BPF array (Wotan populates from dashboard input). `r0 = keycode, r1 = pressed_flag`.
3. `SYSCALL 0x03` (DG_GetTicksMs): `r0 = (bpf_ktime_get_ns() / 1_000_000) as u32`
4. `SYSCALL 0x04` (DG_SleepMs): `cpu.sleep_until = bpf_ktime_get_ns() + (r0 as u64 * 1_000_000)`. Packet passes through without executing until timer expires.
5. `SYSCALL 0xFF` (HALT): `cpu.halted = 1`. Emit `COMPUTE_HALT` event.

**Acceptance:** Each SYSCALL tested. Timer sleep verified. Screen write event carries correct flow_label and size.

---

## TASK D-005: CPU State BPF Maps — Schema & Initialization

**Priority:** P0 — DO THIS FIRST
**Scope:** `ebpf/monad-common/src/lib.rs` (shared types), `ebpf/monad-cpu-ebpf/src/main.rs` (map defs)

Define all BPF maps for the compute engine:

```
cpu_map:     BPF_MAP_TYPE_HASH    key=u32(flow_label)  val=CpuState    max=256
rom_map:     BPF_MAP_TYPE_ARRAY   key=u32(index)       val=u32(insn)   max=1048576 (1M instructions)
l1_cache:    BPF_MAP_TYPE_LRU_HASH  key=u32(addr)      val=[u8;4096]   max=4096 (16MB L1)
kbd_map:     BPF_MAP_TYPE_ARRAY   key=u32(0)           val=KeyboardState  max=1
```

CpuState struct in `ebpf/monad-common/src/lib.rs`:
```rust
#[repr(C)]
pub struct CpuState {
    pub regs: [u32; 16],     // r0-r15, r15 = SP
    pub pc: u32,
    pub flags: u8,           // bit 0=Z, bit 1=N, bit 2=C
    pub stalled: u8,
    pub halted: u8,
    pub _pad: u8,
    pub sleep_until: u64,
    pub insn_count: u64,
}
```

KeyboardState struct:
```rust
#[repr(C)]
pub struct KeyboardState {
    pub key: u32,
    pub pressed: u32,
    pub sequence: u64,       // monotonic, so BPF knows if new
}
```

**Acceptance:** Maps defined, types in monad-common. `cargo build` for both monad-common and monad-cpu-ebpf succeeds.

---

## TASK D-006: Wotan Memory Service — Cache Miss Handler

**Priority:** P0
**Scope:** `services/wotan/internal/compute/memory.go` (EXISTS — extend)
**Uses:** `services/wotan/internal/compute/bpfmap.go` (EXISTS)
**Depends:** D-003, D-005

Wire Wotan to handle `CACHE_MISS` events from the BPF Anamnesis ring buffer:

1. Wotan subscribes to Anamnesis ring buffer (already has reader infrastructure in `pkg/ebpf/anamnesis_reader.go`)
2. Filter for `EVENT_CACHE_MISS` events
3. On miss: extract flow_label + addr from event
4. Read page from L2 ring buffer (Wotan's per-flow memory channel) at `addr & ~(PAGE_SIZE-1)`
5. Stage page into L1 BPF map cache via `bpfmap.go` → `bpf_map_update_elem`
6. Prefetch N adjacent pages (configurable `prefetch_n`, default 4 — spatial locality)
7. Emit `compute.mem.staged` structured event on Wotan pub/sub

**Existing code:** `memory.go` already has `PageTable`, `LRUEviction`, `DirtyWriteback`, `Prefetch`. Wire the BPF map write path from `bpfmap.go`.

**Acceptance:** Miss → stage → re-read test. Prefetch correctness. LRU eviction when L1 full. All tests with `-race`.

---

## TASK D-007: Wotan Memory Service — Dirty Writeback Handler

**Priority:** P0
**Scope:** `services/wotan/internal/compute/memory.go`
**Depends:** D-003, D-006

Wire Wotan to handle `MEM_WRITE` events (dirty pages from BPF ST instructions):

1. Subscribe to `EVENT_MEM_WRITE` Anamnesis events
2. Batch writes: accumulate dirty pages, flush every 1ms (configurable)
3. Write dirty pages to L2 ring buffer (Wotan per-flow channel)
4. Track dirty bitmap per flow — only flush actually-dirty pages
5. If WAL enabled (`wal_enabled` config): append dirty pages to WAL file for persistence
6. Emit `compute.mem.flushed` event with page count + flush latency

**Acceptance:** Write → batch → flush → read-back consistency. Batch interval config. WAL round-trip (if enabled). `-race` clean.

---

## TASK D-008: wotan-ctl — load-rom Command (NEW Binary)

**Priority:** P0
**Scope:** `cmd/wotan-ctl/main.go` (NEW — create)
**Depends:** D-005

Create the `wotan-ctl` CLI tool. First command: `load-rom`:

```bash
cd ~/tmp/unheaded
go run ./cmd/wotan-ctl load-rom --flow-label 0x0A3F7E --file program.mbc [--stats] [--disasm]
```

1. Parse MBC binary file (sequence of 32-bit LE instruction words)
2. Pin/open `rom_map` BPF map (via `pkg/ebpf` or direct bpf syscall)
3. Write each instruction to `rom_map[i] = insn_word`
4. Initialize `cpu_map` entry: PC=0, SP=0xFFFF0000, regs zeroed, flags=0, stalled=0, halted=0
5. `--stats`: print instruction count, program size, estimated runtime at 2MHz
6. `--disasm`: disassemble and print each instruction (opcode name + operands)
7. `--reset`: clear cpu_map entry (restart program)

**Scaffolding:** Use cobra or plain flag package. Follow existing cmd/ patterns (see `cmd/unheaded-cli/`).

**Acceptance:** `go build ./cmd/wotan-ctl/` succeeds. Load `demos/mbc/gradient.mbc` → verify map contents via `bpftool map dump`. `-race` clean.

---

## TASK D-009: wotan-ctl — load-mem Command

**Priority:** P0
**Scope:** `cmd/wotan-ctl/main.go`
**Depends:** D-006, D-008

Add `load-mem` command to wotan-ctl — loads data into Wotan L2 ring buffer as addressable memory:

```bash
go run ./cmd/wotan-ctl load-mem --flow-label 0x0A3F7E --base-addr 0x100000 --file doom1.wad
```

1. Read binary file
2. Split into PAGE_SIZE (4096 byte) chunks
3. Write each chunk to Wotan L2 ring buffer at `base_addr + (i * PAGE_SIZE)`
4. `--warm`: also pre-stage first N pages into L1 BPF map cache (skip cold start)
5. `--verify`: read back and checksum

**Acceptance:** Load test binary → `--verify` passes → byte-for-byte match. Load 1MB file → no OOM.

---

## TASK D-010: Packet Circulation Ring — End-to-End Test Harness

**Priority:** P0
**Scope:** `scripts/doom-ring.sh` (EXISTS — extend), new `scripts/doom-test.sh`
**Depends:** D-001, D-005, D-008

Create the test harness that proves packets execute MBC instructions:

1. `doom-ring.sh` already creates 6 namespaces with veth pairs and IPv6 directed ring
2. Extend: load `monad-cpu-ebpf` at XDP on each veth interface (use `ebpf-loader` or `ip link set dev ... xdp obj ...`)
3. Load gradient.mbc into rom_map via `wotan-ctl load-rom`
4. Craft and inject ONE IPv6 packet with:
   - Flow Label = test flow
   - Hop-by-Hop Option containing 20-byte Monad (R0-R4 zeroed)
   - Destination = next-hop in ring (packet will circulate)
5. Read Anamnesis ring buffer for COMPUTE_HOP events
6. Verify: 6 hops = 6 instructions, PC advances 0→1→2→3→4→5, register values match gradient.mbc expected output
7. Print instruction trace to stdout

**This is THE FIRST PACKET WALKING THE PATTERN as a compute engine.**

**Acceptance:** `sudo ./scripts/doom-test.sh gradient` exits 0, prints correct register trace for 6 instructions.

---

## TASK D-011: Dashboard — Framebuffer WebSocket from Wotan

**Priority:** P1
**Scope:** `dashboard/js/doom-viewport.js` (EXISTS — wire to real data), `cmd/dashboard-backend/`
**Depends:** D-004, D-010

Wire dashboard to receive REAL framebuffer data from Wotan:

1. Dashboard backend: subscribe to `compute.screen.<flow_label>` Wotan topic
2. On SCREEN_WRITE event: Wotan reads 64000 bytes (320×200 × 1 byte palette index) from L2 ring buffer
3. Forward raw framebuffer bytes to dashboard via existing WebSocket connection as binary message
4. `doom-viewport.js` (already has canvas renderer + 256-color VGA palette): decode binary → RGBA → `putImageData` → `requestAnimationFrame`

**Existing code:** `doom-viewport.js` already has `DoomViewport` class with canvas, palette, WebSocket handler. Currently uses synthetic data. Switch to real Wotan topic subscription.

**Acceptance:** gradient.mbc SYSCALL 0x01 → Wotan → WebSocket → browser canvas shows gradient pattern.

---

## TASK D-012: Dashboard — Keyboard Input → Wotan → BPF Map

**Priority:** P1
**Scope:** `dashboard/js/doom-viewport.js`, `cmd/dashboard-backend/`, Wotan
**Depends:** D-004

Wire keyboard input from browser → BPF compute engine:

1. `doom-viewport.js`: `addEventListener('keydown'/'keyup')` → pack `{key, pressed, sequence}` → send WebSocket binary
2. Dashboard backend: receive keyboard WebSocket message → publish to `compute.input.<flow_label>` Wotan topic
3. Wotan: on keyboard event → write `KeyboardState` struct to `kbd_map` BPF array (map index 0)
4. BPF SYSCALL 0x02: reads `kbd_map[0]`, returns key + pressed to MBC program

**Latency target:** keypress → BPF map update < 10ms. BPF reads on next packet circulation (~500ns per hop).

**Acceptance:** Press key in browser → `bpftool map dump id <kbd_map>` shows key within 10ms.

---

## TASK D-013: Dashboard — CPU Trace Overlay

**Priority:** P1
**Scope:** `dashboard/js/doom-viewport.js`, `cmd/dashboard-backend/`
**Depends:** D-010, D-014

Real-time CPU state overlay on the Doom viewport canvas:

1. Dashboard backend subscribes to COMPUTE_HOP Anamnesis events
2. Forward to dashboard via existing WebSocket
3. `doom-viewport.js` renders semi-transparent overlay showing:
   - PC (hex), current opcode name
   - Registers r0-r15 (hex, highlight changed)
   - Flags: Z/N/C (highlight set)
   - Instructions/sec counter (rolling 1s window)
   - L1 cache hit rate (hits / (hits + misses) over 1s window)
   - Wotan L2 ring buffer utilization %
4. Toggle overlay with F3 key
5. Minimal performance impact on the renderer (skip if frame budget exceeded)

**Acceptance:** Overlay visible during gradient.mbc. Toggle works. FPS stays >30 with overlay enabled.

---

## TASK D-014: Anamnesis Event Schema — Compute Event Types

**Priority:** P0 — DO THIS EARLY (D-001 depends on it)
**Scope:** `ebpf/monad-common/src/lib.rs` (Rust), `pkg/ebpf/anamnesis.go` (Go)

Extend Anamnesis event types for the compute engine. Both sides must match byte-for-byte.

Rust side (`ebpf/monad-common/src/lib.rs` — extend existing EventType enum):
```rust
pub const EVENT_COMPUTE_HOP: u8 = 0x10;
pub const EVENT_CACHE_MISS: u8 = 0x11;
pub const EVENT_MEM_WRITE: u8 = 0x12;
pub const EVENT_MEM_STAGED: u8 = 0x13;
pub const EVENT_SCREEN_WRITE: u8 = 0x14;
pub const EVENT_KEY_READ: u8 = 0x15;
pub const EVENT_COMPUTE_HALT: u8 = 0x16;
pub const EVENT_COMPUTE_STALL: u8 = 0x17;
```

ComputeHopEvent struct (emitted by monad-cpu-ebpf):
```rust
#[repr(C)]
pub struct ComputeHopEvent {
    pub timestamp_ns: u64,
    pub event_type: u8,
    pub hop_id: u8,
    pub _pad: [u8; 2],
    pub flow_label: u32,
    pub pc: u32,
    pub instruction: u32,
    pub regs: [u32; 16],    // full register snapshot
    pub flags: u8,
    pub cache_hit: u8,      // 1=hit, 0=miss (for stats)
    pub _pad2: [u8; 2],
}
```

Go side (`pkg/ebpf/anamnesis.go` — matching struct + decoder):
- `DecodeComputeHopEvent(b []byte) (*ComputeHopEvent, error)`
- JSON marshaling for WebSocket transport
- Fuzz test the decoder with random bytes

**Acceptance:** `sizeof(ComputeHopEvent)` matches Rust and Go. Fuzz test passes 10K iterations. Round-trip encode/decode.

---

## TASK D-015: MBC Assembler — SYSCALL + Named Aliases

**Priority:** P1
**Scope:** `crates/monad-mbc/src/assembler.rs`
**Depends:** D-004

Extend the assembler to support SYSCALL:

1. `SYSCALL imm16` — encodes as `[SYSCALL_OPCODE:8][0:4][0:4][imm16:16]`
2. Named aliases in assembler: `DRAW_FRAME` → `SYSCALL 0x01`, `GET_KEY` → `SYSCALL 0x02`, `GET_TICKS` → `SYSCALL 0x03`, `SLEEP` → `SYSCALL 0x04`, `HALT` → `SYSCALL 0xFF`
3. Update `demos/mbc/gradient.mbc` to use `SYSCALL DRAW_FRAME` instead of HALT for output
4. Update `demos/mbc/checkerboard.mbc` same

**Acceptance:** Assemble program with SYSCALL → disassemble → opcode matches. Demo programs updated. All existing assembler tests still pass.

---

## TASK D-016: Progressive Demo — gradient.mbc Full Pipeline

**Priority:** P0 — THE CHECKPOINT
**Scope:** Integration test across full stack
**Depends:** D-001 through D-011, D-014

First complete end-to-end proof:

1. Assemble `demos/mbc/gradient.mbc` (or use pre-assembled binary)
2. `wotan-ctl load-rom --flow-label 0xDEAD --file gradient.mbc`
3. Start Wotan with compute memory service
4. Start doom-ring.sh (6 namespaces, BPF loaded)
5. Start dashboard-backend with compute topic subscription
6. Open dashboard in browser
7. Inject IPv6 packet with flow label 0xDEAD into ns0
8. Packet circulates: 6 instructions per lap, ~5000 instructions to fill framebuffer
9. SYSCALL 0x01 fires → Wotan reads framebuffer → WebSocket → doom-viewport.js
10. **GRADIENT VISIBLE ON DASHBOARD**

**If this works, Doom is a matter of scale.**

**Acceptance:** Gradient pattern renders in browser. Anamnesis shows instruction trace. Wotan logs cache hit rate > 90%.

---

## TASK D-017: Progressive Demo — checkerboard.mbc Full Pipeline

**Priority:** P1
**Scope:** Integration test
**Depends:** D-016

Repeat D-016 pipeline with checkerboard.mbc:

1. More complex: nested loops, conditional branches, memory read/write patterns
2. Validates branch instructions end-to-end (JZ, JNZ, CMP)
3. Validates memory write → Wotan dirty writeback → re-read consistency
4. Higher instruction count (~10K+ instructions)
5. Multiple SYSCALL DRAW_FRAME calls (animated pattern)

**Acceptance:** Checkerboard pattern renders. No stalls after L1 warms. Cache hit rate logged and > 90%.

---

## TASK D-018: Cross-Compile Pipeline — C → RV32I → MBC

**Priority:** P1
**Scope:** `scripts/cross-compile.sh` (NEW), toolchain docs
**Depends:** D-002, D-008

Build and validate the full C-to-MBC compilation pipeline:

1. Get `riscv32-unknown-elf-gcc` (apt, nix, or Docker — document the method)
2. Write trivial test: `int result = 0; for (int i = 0; i < 100; i++) result += i; // expect 4950`
3. Compile:
   ```bash
   riscv32-unknown-elf-gcc -O2 -march=rv32im -mabi=ilp32 \
     -ffixed-x16 -ffixed-x17 -ffixed-x18 -ffixed-x19 \
     -ffixed-x20 -ffixed-x21 -ffixed-x22 -ffixed-x23 \
     -ffixed-x24 -ffixed-x25 -ffixed-x26 -ffixed-x27 \
     -ffixed-x28 -ffixed-x29 -ffixed-x30 -ffixed-x31 \
     -nostdlib -static -T linker.ld -o test.elf test.c
   ```
4. Translate: `cargo run -p monad-mbc --bin rv32i-to-mbc -- test.elf -o test.mbc --stats --disasm`
5. Load + run through doom-ring → verify r0 = 4950 after execution completes
6. Package as `scripts/cross-compile.sh` for reuse

**Known translator limitations (documented in HANDOFF_2026_02_18_SESSION2.md):**
- Register-based shifts unsupported (use `-O2` to get immediate shifts)
- Only x0-x15 (hence `-ffixed-x16..x31`)
- MULH/MULHSU/MULHU unsupported
- Signed loads zero-extend only

**Acceptance:** C → ELF → MBC → BPF map → execute → r0 == 4950. Script exits 0.

---

## TASK D-019: doomgeneric Bare-Metal Port Stubs

**Priority:** P1
**Scope:** `demos/doom/` (NEW directory)
**Depends:** D-018, D-004

Scaffold the Doom bare-metal port:

1. Clone/vendor doomgeneric source (MIT license) into `demos/doom/doomgeneric/`
2. Create `demos/doom/doomgeneric_unheaded.c`:
   ```c
   void DG_Init() { /* alloc 64000 bytes at 0x200000 in Wotan RAM for framebuffer */ }
   void DG_DrawFrame() { __asm__("SYSCALL 0x01"); /* r1 = 0x200000 */ }
   void DG_SleepMs(uint32_t ms) { __asm__("SYSCALL 0x04"); }
   uint32_t DG_GetTicksMs() { __asm__("SYSCALL 0x03"); return r0; }
   int DG_GetKey(int *pressed, unsigned char *key) { __asm__("SYSCALL 0x02"); }
   ```
   (Actual implementation will use inline asm or direct SYSCALL encoding)
3. Create `demos/doom/linker.ld`: `.text` at 0x0, `.rodata` after, `.data` after, `.bss` after, stack at 0xFFFF0000
4. Create `demos/doom/Makefile`: cross-compile target, translate target, load target
5. Create `demos/doom/crt0.s`: minimal startup (zero BSS, set SP, call main)

**Acceptance:** `make -C demos/doom doom.elf` produces ELF. `rv32i-to-mbc demos/doom/doom.elf -o doom.mbc --stats` produces MBC binary.

---

## TASK D-020: Doom End-to-End — THE MOMENT

**Priority:** P1 — THE REASON WE'RE HERE
**Scope:** Full stack integration
**Depends:** ALL above tasks

Wire everything for the full Doom demo:

```bash
cd ~/tmp/unheaded

# 1. Load Doom MBC into BPF rom_map
go run ./cmd/wotan-ctl load-rom --flow-label 0x0A3F7E --file demos/doom/doom.mbc --stats

# 2. Load doom1.wad into Wotan L2 ring buffer
go run ./cmd/wotan-ctl load-mem --flow-label 0x0A3F7E --base-addr 0x100000 --file demos/doom/doom1.wad

# 3. Start infrastructure
sudo ./scripts/doom-ring.sh start    # 6 namespaces, BPF loaded
go run ./cmd/wotan-ctl start         # Wotan with compute memory service
go run ./cmd/dashboard-backend       # Dashboard with compute topics

# 4. Open browser
open http://localhost:8080/doom

# 5. Inject first packet — THE FIRST STEP ON THE PATTERN
sudo ./scripts/doom-ring.sh inject --flow-label 0x0A3F7E

# 6. Watch Doom boot
```

**Success Criteria (from doom-over-ipv6-plan.md):**
- [ ] Doom title screen renders on Kingdom dashboard
- [ ] Frame rate >= 20 FPS
- [ ] Keyboard input latency < 50ms (browser → Wotan → BPF → game)
- [ ] L1 cache hit rate > 90%
- [ ] Wotan ring buffer scales via --ring-size (512MB for Doom)
- [ ] Anamnesis captures full execution trace (viewable in CPU overlay)
- [ ] Stable for >= 5 minutes gameplay
- [ ] Demo video recorded and publishable

**THE PACKET WALKS THE PATTERN. DOOM RUNS ON THE WIRE. WOTAN IS THE RAM.**

---

## EXECUTION ORDER

```
WAVE 1 — FOUNDATION (sequential, each builds on last):
  D-005 (maps/types) → D-014 (events) → D-001 (CPU core) → D-002 (branches) → D-003 (memory) → D-004 (syscall)

WAVE 2 — WOTAN (parallel with Wave 1 after D-005):
  D-006 (cache miss) ║ D-007 (dirty writeback) ║ D-008 (wotan-ctl load-rom) ║ D-009 (load-mem)

WAVE 3 — INTEGRATION (after Wave 1 + Wave 2):
  D-010 (test harness) ║ D-015 (assembler SYSCALL)

WAVE 4 — DASHBOARD (parallel, after D-010):
  D-011 (framebuffer WS) ║ D-012 (keyboard input) ║ D-013 (CPU overlay)

WAVE 5 — PROGRESSIVE DEMOS (sequential checkpoints):
  D-016 (gradient — THE CHECKPOINT) → D-017 (checkerboard)

WAVE 6 — DOOM PORT (sequential):
  D-018 (cross-compile pipeline) → D-019 (doomgeneric stubs) → D-020 (THE MOMENT)
```

---

## KEY FILE PATHS (all relative to ~/tmp/unheaded/)

```
ebpf/monad-cpu-ebpf/src/main.rs             ← THE CPU (D-001 through D-004)
ebpf/monad-common/src/lib.rs                ← Shared types (D-005, D-014)
services/wotan/internal/compute/memory.go    ← Wotan RAM (D-006, D-007) — EXISTS, 13K LOC
services/wotan/internal/compute/bpfmap.go    ← BPF map abstraction (D-006) — EXISTS, 2.7K LOC
services/wotan/internal/compute/memory_test.go ← Existing 19 tests
crates/monad-mbc/src/assembler.rs           ← MBC assembler (D-015) — EXISTS
crates/monad-mbc/src/translator.rs          ← RV32I→MBC translator — DONE, 52 tests
crates/monad-mbc/src/bin/rv32i_to_mbc.rs    ← ELF→MBC CLI — DONE
pkg/ebpf/anamnesis.go                       ← Go event types (D-014) — EXISTS
pkg/ebpf/anamnesis_reader.go                ← Ring buffer reader — EXISTS
dashboard/js/doom-viewport.js               ← Doom renderer (D-011, D-012, D-013) — EXISTS, 450 LOC
cmd/dashboard-backend/                       ← Dashboard API — EXISTS, 5.9K LOC
cmd/wotan-ctl/                              ← ROM/mem loader (D-008, D-009) — NEW
scripts/doom-ring.sh                        ← Packet ring — EXISTS
demos/mbc/gradient.mbc                      ← Test program — EXISTS
demos/mbc/checkerboard.mbc                  ← Test program — EXISTS
demos/doom/                                 ← Doom port (D-019) — NEW
```

---

**THE WIRE IS THE PROCESSOR.**
**WOTAN IS THE RAM.**
**PACKETS ARE THE CPU.**
**DOOM WALKS THE PATTERN.**

⚔️🔥🎮🛡️
