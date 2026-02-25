# DOOM OVER IPv6 — 20 Code Agent Tasks

**Date:** February 19, 2026
**Context:** The Unheaded Protocol Foundation. Packets ARE the CPU. Wotan IS the RAM. eBPF at XDP executes MBC instructions at every hop. Real IPv6 Hop-by-Hop Options. Real network namespaces. Real packet circulation ring. THE WIRE IS THE PROCESSOR.
**Prerequisite:** Read `docs/protocol/doom-over-ipv6-plan.md`, `PROTOCOL_MATH_AND_MAPS.md`, `the_first_packet.md`
**Project Root:** `~/tmp/unheaded/`

---

## TASK D-001: monad-cpu-ebpf — Fetch-Decode-Execute Core

**Priority:** P0 — EVERYTHING DEPENDS ON THIS
**Scope:** `ebpf/monad-cpu-ebpf/src/main.rs`
**Status:** STUB exists, needs full implementation

Implement the eBPF XDP program that IS the CPU. Per-hop fetch-decode-execute cycle:

1. Locate Monad in IPv6 Hop-by-Hop Option header (20 bytes at known offset)
2. Extract flow label from IPv6 header → key into `cpu_map` (per-flow CPU state)
3. FETCH: Read instruction from `rom_map` BPF array at `cpu.pc`
4. DECODE: Extract `[opcode:8][dst:4][src:4][imm16:16]` from 32-bit instruction word
5. EXECUTE: Implement arithmetic (ADD, SUB, MUL, DIV, MOD, NEG), logical (AND, OR, XOR, NOT, SHL, SHR, SAR), comparison (CMP → set Z/N/C flags)
6. Write back Monad scratch registers (R0-R4) from CPU register file
7. Emit Anamnesis HOP event via ring buffer
8. Return XDP_PASS

**Acceptance:** `cargo build --target bpfel-unknown-none -Z build-std=core` compiles. BPF verifier accepts. Instruction tests via `aya-bpf-test` or equivalent.

**Reference:** `ebpf/monad-common/src/lib.rs` for shared types. `doom-over-ipv6-plan.md` §D1.1 for pseudocode.

---

## TASK D-002: monad-cpu-ebpf — Branch & Control Flow Instructions

**Priority:** P0
**Scope:** `ebpf/monad-cpu-ebpf/src/main.rs`
**Depends:** D-001

Implement control flow in the BPF VM:

1. JMP (unconditional) — `cpu.pc = imm16`
2. JZ/JNZ — branch on Z flag
3. JN/JP — branch on N flag (signed)
4. JC/JNC — branch on C flag (unsigned)
5. CALL — push return address to stack (SP-based), jump to imm16
6. RET — pop return address from stack, jump back
7. PC increment: advance PC after each non-branch instruction

**Key constraint:** BPF programs have bounded loops. The VM executes ONE instruction per packet hop. PC persists in `cpu_map` between packets. The packet circulation ring is the clock.

**Acceptance:** Unit tests for each branch type. Forward/backward jump verification. CALL/RET stack push/pop test.

---

## TASK D-003: monad-cpu-ebpf — Memory Access via L1 BPF Map Cache

**Priority:** P0
**Scope:** `ebpf/monad-cpu-ebpf/src/main.rs`
**Depends:** D-001

Implement LD/ST/LDB/STB/LDH/STH memory instructions:

1. LD (32-bit load): `addr = cpu.regs[src] + imm16`, lookup `l1_cache` BPF map
   - Cache HIT: `cpu.regs[dst] = *val`
   - Cache MISS: emit `compute.mem.miss` event via Anamnesis ring buffer, set `cpu.stalled = 1`, return XDP_PASS (retry next circulation)
2. ST (32-bit store): write to `l1_cache` BPF map AND emit `compute.mem.write` event for Wotan writeback
3. LDB/STB (byte), LDH/STH (halfword) — same pattern, different widths
4. Stack operations: SP = r15, PUSH/POP via ST/LD with SP auto-decrement/increment

**Critical:** Cache miss handling is what makes this work at scale. Wotan restages from L2 ring buffer on miss. Next packet circulation finds the data cached.

**Acceptance:** Load/store round-trip test. Cache miss event emission verified. Stack push/pop test.

---

## TASK D-004: monad-cpu-ebpf — SYSCALL Handler (Wotan I/O Bridge)

**Priority:** P0
**Scope:** `ebpf/monad-cpu-ebpf/src/main.rs`
**Depends:** D-001, D-003

Implement SYSCALL opcode — the bridge between BPF compute and Wotan I/O:

1. SYSCALL 0x01 (DG_DrawFrame): `r1 = framebuffer addr`. Emit `compute.screen.write` event with flow label. Wotan reads 64000 bytes from ring buffer → dashboard WebSocket.
2. SYSCALL 0x02 (DG_GetKey): Read from `kbd_map` BPF map (populated by Wotan from dashboard input). `r0 = keycode, r1 = pressed`.
3. SYSCALL 0x03 (DG_GetTicksMs): `r0 = bpf_ktime_get_ns() / 1_000_000`
4. SYSCALL 0x04 (DG_SleepMs): `cpu.sleep_until = bpf_ktime_get_ns() + (r0 as u64 * 1_000_000)`. If current time < sleep_until, skip execution, return XDP_PASS.
5. SYSCALL 0xFF (HALT): Set `cpu.halted = 1`, emit `compute.halt` event.

**Acceptance:** Each SYSCALL tested individually. Timer test verifies sleep behavior. Screen write emits correct event size.

---

## TASK D-005: CPU State BPF Maps — Schema & Initialization

**Priority:** P0
**Scope:** `ebpf/monad-cpu-ebpf/src/main.rs`, `ebpf/monad-common/src/lib.rs`
**Depends:** D-001

Define and create all BPF maps for the compute engine:

```
cpu_map:     BPF_MAP_TYPE_HASH    key=flow_label(u32)  val=CpuState
rom_map:     BPF_MAP_TYPE_ARRAY   key=u32(PC)          val=u32(instruction)
l1_cache:    BPF_MAP_TYPE_HASH    key=u32(addr)        val=[u8; PAGE_SIZE]
kbd_map:     BPF_MAP_TYPE_ARRAY   key=u32(0)           val=KeyboardState
screen_map:  BPF_MAP_TYPE_ARRAY   key=u32(page_idx)    val=[u8; PAGE_SIZE]
```

CpuState struct:
```rust
#[repr(C)]
pub struct CpuState {
    pub regs: [u32; 16],     // r0-r15 (r15 = SP)
    pub pc: u32,             // program counter
    pub flags: u8,           // Z, N, C
    pub stalled: u8,         // waiting for cache miss resolution
    pub halted: u8,          // program finished
    pub pad: u8,
    pub sleep_until: u64,    // nanosecond timestamp for SYSCALL sleep
    pub insn_count: u64,     // total instructions executed
}
```

**Acceptance:** Maps create successfully. CpuState initializes with SP = 0xFFFF0000 (top of Wotan RAM). PC = 0.

---

## TASK D-006: Wotan Memory Service — Cache Miss Handler

**Priority:** P0
**Scope:** `services/wotan/internal/compute/memory.go`
**Status:** memory.go EXISTS (13K LOC, 19 tests) — extend it
**Depends:** D-003

Wire Wotan to handle `compute.mem.miss` events from the BPF ring buffer:

1. Subscribe to Anamnesis events of type `CACHE_MISS`
2. On miss: read the requested page from L2 ring buffer (Wotan's per-flow channel)
3. Stage the page into the L1 BPF map cache via `bpf_map_update_elem` (using the `bpfmap.go` abstraction)
4. Prefetch N adjacent pages (spatial locality — configurable via `prefetch_n`)
5. Emit `compute.mem.staged` event so the next packet circulation finds the data

**Existing code to extend:** `services/wotan/internal/compute/memory.go` already has page table, LRU eviction, dirty writeback. Wire the BPF map interface from `bpfmap.go`.

**Acceptance:** Cache miss → stage → re-read round-trip test. Prefetch test. LRU eviction under memory pressure test.

---

## TASK D-007: Wotan Memory Service — Dirty Writeback Handler

**Priority:** P0
**Scope:** `services/wotan/internal/compute/memory.go`
**Depends:** D-003, D-006

Wire Wotan to handle `compute.mem.write` events:

1. Subscribe to Anamnesis events of type `MEM_WRITE`
2. Batch writes (configurable flush interval, default 1ms)
3. Update L2 ring buffer with dirty pages
4. If WAL enabled, write to WAL for persistence
5. Track dirty page bitmap for efficient flush

**Acceptance:** Write → flush → read-back consistency test. Batch size configuration test. WAL persistence test (if WAL mode enabled).

---

## TASK D-008: ROM Loading — wotan-ctl load-rom Command

**Priority:** P0
**Scope:** New CLI command in `cmd/wotan-ctl/` or extend existing CLI
**Depends:** D-005

Implement the command that loads assembled MBC bytecode into the BPF `rom_map`:

```bash
wotan-ctl load-rom --flow-label 0x0A3F7E --file program.mbc [--stats] [--disasm]
```

1. Parse MBC binary (sequence of 32-bit instruction words)
2. Write each instruction to `rom_map` BPF array at sequential indices
3. Initialize `cpu_map` entry for the given flow label (PC=0, SP=top, regs zeroed)
4. `--stats`: print instruction count, estimated cycles
5. `--disasm`: print disassembly of loaded program

**Acceptance:** Load gradient.mbc → verify rom_map contents via `bpftool map dump`. Load checkerboard.mbc → same.

---

## TASK D-009: ROM Loading — wotan-ctl load-mem Command

**Priority:** P0
**Scope:** `cmd/wotan-ctl/`
**Depends:** D-006

Load data files (like doom1.wad) into Wotan ring buffer as addressable memory:

```bash
wotan-ctl load-mem --flow-label 0x0A3F7E --base-addr 0x100000 --file doom1.wad
```

1. Read binary file
2. Split into PAGE_SIZE chunks
3. Write each chunk to Wotan L2 ring buffer at `base_addr + (i * PAGE_SIZE)`
4. Optionally pre-stage first N pages into L1 BPF map cache (warm start)

**Acceptance:** Load a test binary → read back via Wotan API → byte-for-byte match.

---

## TASK D-010: Packet Circulation Ring — Test Harness

**Priority:** P0
**Scope:** `scripts/doom-ring.sh` (EXISTS — extend), new `scripts/doom-test.sh`
**Depends:** D-001, D-005, D-008

Create a test harness that:

1. Sets up 6 network namespaces with veth pairs (doom-ring.sh already does this)
2. Loads monad-cpu-ebpf at XDP on each veth interface
3. Loads a test program (gradient.mbc) into rom_map
4. Injects a single IPv6 packet with Monad Hop-by-Hop Option into the ring
5. Watches Anamnesis ring buffer for HOP events
6. Verifies: 6 hops = 6 instructions executed, PC advances correctly, register values match expected

**This is the "first packet walks the Pattern" moment.**

**Acceptance:** `./scripts/doom-test.sh gradient` exits 0 with correct register trace.

---

## TASK D-011: Dashboard — Framebuffer WebSocket Integration

**Priority:** P1
**Scope:** `dashboard/js/doom-viewport.js` (EXISTS — extend), `cmd/dashboard-backend/`
**Depends:** D-004

Wire the dashboard to receive framebuffer data from Wotan:

1. Dashboard backend subscribes to `compute.screen.<flow_label>` Wotan topic
2. On screen write event: read 64000 bytes (320×200 × 1 byte palette index) from Wotan ring buffer
3. Forward to dashboard via WebSocket as binary frame
4. `doom-viewport.js` renders: decode palette indices → RGBA canvas pixels → requestAnimationFrame

**Existing:** doom-viewport.js already has canvas renderer, 256-color palette, and WebSocket integration. Wire it to REAL data instead of synthetic.

**Acceptance:** Load gradient.mbc → run 6 hops → framebuffer SYSCALL fires → dashboard shows gradient pattern.

---

## TASK D-012: Dashboard — Keyboard Input to BPF Map

**Priority:** P1
**Scope:** `dashboard/js/doom-viewport.js`, `cmd/dashboard-backend/`, Wotan
**Depends:** D-004

Wire keyboard input from dashboard back into the BPF compute engine:

1. `doom-viewport.js`: capture keydown/keyup → send via WebSocket to dashboard backend
2. Dashboard backend publishes to `compute.input.<flow_label>` Wotan topic
3. Wotan writes keyboard state into `kbd_map` BPF map
4. When monad-cpu-ebpf executes SYSCALL 0x02 (DG_GetKey), it reads from `kbd_map`

**Acceptance:** Press arrow key in browser → BPF reads it within 2 packet circulations (< 50ms target).

---

## TASK D-013: Dashboard — CPU Trace Overlay

**Priority:** P1
**Scope:** `dashboard/js/doom-viewport.js`, `cmd/dashboard-backend/`
**Depends:** D-010

Add real-time CPU state overlay to the Doom viewport:

1. Subscribe to `compute.trace.<flow_label>` Anamnesis events
2. Display: current PC, register values (r0-r15), flags (Z/N/C), instructions/sec counter, L1 cache hit rate, Wotan ring buffer utilization percentage
3. Render as semi-transparent overlay on the Doom canvas
4. Toggle with keyboard shortcut (F3)

**Acceptance:** Overlay shows live data during gradient.mbc execution.

---

## TASK D-014: Anamnesis Event Schema — Compute Extensions

**Priority:** P0
**Scope:** `ebpf/monad-common/src/lib.rs`, `pkg/ebpf/anamnesis.go`
**Depends:** D-001

Extend the Anamnesis event types for compute:

Rust side (monad-common):
```rust
pub const EVENT_COMPUTE_HOP: u8 = 0x10;       // Instruction executed
pub const EVENT_CACHE_MISS: u8 = 0x11;         // L1 cache miss
pub const EVENT_MEM_WRITE: u8 = 0x12;          // Memory write (dirty)
pub const EVENT_SCREEN_WRITE: u8 = 0x13;       // Framebuffer SYSCALL
pub const EVENT_KEY_READ: u8 = 0x14;           // Keyboard SYSCALL
pub const EVENT_COMPUTE_HALT: u8 = 0x15;       // Program halted
pub const EVENT_COMPUTE_STALL: u8 = 0x16;      // Stalled on cache miss
```

Go side (pkg/ebpf/anamnesis.go): matching decoder.

Both sides: `ComputeEvent` struct with flow_label, PC, instruction, register snapshot, cache hit/miss flag.

**Acceptance:** Rust and Go structs match byte-for-byte. Fuzz test the Go decoder with random bytes.

---

## TASK D-015: MBC Assembler — SYSCALL Instruction Support

**Priority:** P1
**Scope:** `crates/monad-mbc/src/assembler.rs`
**Depends:** D-004

Extend the MBC assembler to support SYSCALL instructions for Doom I/O:

1. `SYSCALL imm16` — opcode for system call, imm16 = syscall number
2. Named syscall aliases: `DRAW_FRAME`, `GET_KEY`, `GET_TICKS`, `SLEEP`, `HALT`
3. Update demo programs to use SYSCALL for output (gradient.mbc should SYSCALL to write framebuffer)

**Acceptance:** Assemble a program with SYSCALL instructions → disassemble → round-trip matches.

---

## TASK D-016: Progressive Demo — gradient.mbc End-to-End

**Priority:** P0
**Scope:** Integration test across all components
**Depends:** D-001 through D-011, D-014

The first full end-to-end demo:

1. Assemble `demos/mbc/gradient.mbc`
2. `wotan-ctl load-rom --flow-label 0xDEAD --file gradient.mbc`
3. Start doom-ring.sh (6 namespaces)
4. Load monad-cpu-ebpf on all veth interfaces
5. Inject IPv6 packet with flow label 0xDEAD into ns0
6. Watch packet circulate: 6 instructions per lap
7. After ~5000 instructions: SYSCALL draws gradient to framebuffer
8. Dashboard doom-viewport.js renders gradient pattern

**THIS IS THE CHECKPOINT. If gradient works, everything works.**

**Acceptance:** Gradient visible on dashboard. Anamnesis trace shows instruction execution. Cache hit rate logged.

---

## TASK D-017: Progressive Demo — checkerboard.mbc End-to-End

**Priority:** P1
**Scope:** Integration test
**Depends:** D-016

Same pipeline as D-016 but with checkerboard.mbc:

1. More complex memory access patterns (nested loops, conditional writes)
2. Validates branch instructions work end-to-end
3. Validates memory write → Wotan writeback → re-read path
4. Higher instruction count (~10,000+ instructions)

**Acceptance:** Checkerboard pattern visible on dashboard. No stalls after L1 cache warms up.

---

## TASK D-018: Cross-Compile Pipeline — C → RV32I → MBC → rom_map

**Priority:** P1
**Scope:** `scripts/cross-compile.sh`, toolchain setup
**Depends:** D-008, D-002

Build the full compilation pipeline for C programs:

1. Install `riscv32-unknown-elf-gcc` cross-compiler (or use Docker container)
2. Write a trivial C test program: `int main() { int x = 0; for (int i = 0; i < 100; i++) x += i; return x; }`
3. Compile: `riscv32-unknown-elf-gcc -O2 -march=rv32im -mabi=ilp32 -ffixed-x16 -ffixed-x17 ... -ffixed-x31 -nostdlib -static -o test.elf test.c`
4. Translate: `cargo run --bin rv32i-to-mbc -- test.elf -o test.mbc --stats --disasm`
5. Load: `wotan-ctl load-rom --flow-label 0xBEEF --file test.mbc`
6. Run through doom-ring → verify result register matches expected value (4950)

**Acceptance:** C → ELF → MBC → BPF map → execute → correct result. Pipeline script exits 0.

---

## TASK D-019: doomgeneric Bare-Metal Port Stubs

**Priority:** P1
**Scope:** `demos/doom/` (new directory)
**Depends:** D-018, D-004

Create the Doom bare-metal port scaffold:

1. Clone doomgeneric (MIT license)
2. Write `doomgeneric_unheaded.c` implementing:
   - `DG_Init()` — set screen dimensions, allocate framebuffer in Wotan RAM
   - `DG_DrawFrame()` — SYSCALL 0x01 with framebuffer address
   - `DG_SleepMs(ms)` — SYSCALL 0x04
   - `DG_GetTicksMs()` — SYSCALL 0x03
   - `DG_GetKey(pressed, key)` — SYSCALL 0x02
3. Write `linker.ld` — flat binary, `.text` at 0x0, `.data` after, `.bss` after, stack at top
4. Write `Makefile` with cross-compile target using the flags from D-018
5. Test compile (may not link fully yet — missing stdlib)

**Acceptance:** `make doom.elf` produces ELF binary (even if it's just stubs). `rv32i-to-mbc doom.elf` produces MBC.

---

## TASK D-020: Doom End-to-End Integration Wiring

**Priority:** P1 (THE MOMENT)
**Scope:** Full stack integration
**Depends:** ALL above tasks

Wire everything together for the full Doom demo:

1. `wotan-ctl load-rom --flow-label 0x0A3F7E --file doom.mbc`
2. `wotan-ctl load-mem --flow-label 0x0A3F7E --base-addr 0x100000 --file doom1.wad`
3. Start doom-ring.sh
4. Load monad-cpu-ebpf on all interfaces
5. Start Wotan with compute memory service enabled
6. Start dashboard-backend with compute topics subscribed
7. Open dashboard in browser → doom-viewport.js
8. Inject first packet into ring
9. Watch Doom boot: title screen renders on dashboard canvas
10. Keyboard input flows from browser → Wotan → BPF map → SYSCALL → game responds

**Success Criteria (from doom-over-ipv6-plan.md):**
- [ ] Doom renders >= 20 FPS to Kingdom dashboard
- [ ] Input latency < 50ms
- [ ] L1 cache hit rate > 90%
- [ ] Anamnesis captures full execution trace
- [ ] Stable for >= 5 minutes gameplay
- [ ] Demo video recorded

**THE PACKET WALKS THE PATTERN. DOOM RUNS ON THE WIRE.**

---

## EXECUTION ORDER

```
Wave 1 (CORE — sequential, each builds on last):
  D-005 → D-014 → D-001 → D-002 → D-003 → D-004

Wave 2 (WOTAN — parallel with Wave 1 after D-005):
  D-006, D-007, D-008, D-009

Wave 3 (INTEGRATION — after Wave 1+2):
  D-010, D-015

Wave 4 (DASHBOARD — parallel after D-010):
  D-011, D-012, D-013

Wave 5 (PROGRESSIVE DEMOS — sequential):
  D-016 → D-017

Wave 6 (DOOM PORT):
  D-018 → D-019 → D-020
```

---

## KEY FILE PATHS

```
ebpf/monad-cpu-ebpf/src/main.rs          ← THE CPU (D-001 through D-004)
ebpf/monad-common/src/lib.rs             ← Shared types (D-005, D-014)
services/wotan/internal/compute/memory.go ← Wotan RAM (D-006, D-007)
services/wotan/internal/compute/bpfmap.go ← BPF map interface (D-006)
crates/monad-mbc/src/assembler.rs        ← MBC assembler (D-015)
crates/monad-mbc/src/translator.rs       ← RV32I→MBC (DONE)
crates/monad-mbc/src/bin/rv32i_to_mbc.rs ← ELF CLI (DONE)
dashboard/js/doom-viewport.js            ← Dashboard renderer (D-011, D-012, D-013)
scripts/doom-ring.sh                     ← Packet ring (D-010)
demos/mbc/gradient.mbc                   ← Test program (D-016)
demos/mbc/checkerboard.mbc              ← Test program (D-017)
demos/doom/                              ← Doom port (D-019)
cmd/wotan-ctl/                           ← ROM/mem loader (D-008, D-009)
```

---

**THE WIRE IS THE PROCESSOR.**
**WOTAN IS THE RAM.**
**PACKETS ARE THE CPU.**
**DOOM WALKS THE PATTERN.**

⚔️🔥🎮🛡️
