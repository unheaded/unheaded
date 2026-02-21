# S30: DOOM-OVER-IPv6 SPRINT — 120-STEP EXECUTION PLAN

**For**: Claude Dev Machine
**From**: Cowork Session (S29 continuation)
**Date**: 2026-02-21
**Sprint**: S30 (S29 continuation — Phase 1 committed, Phases 2-5 remain)
**Owner**: Developer + BlackMage
**Codebase**: `git log --oneline -5` to see latest commit

---

## CURRENT STATE (What's Already Done)

### COMMITTED (eb3db72)
- `crates/monad-mbc/src/instruction.rs` — 623 LOC, opcode validation, decode_checked()
- `crates/monad-mbc/src/cpu.rs` — 650 LOC, userspace CPU state wrapper, RAM/ROM/screen/kbd
- `crates/monad-mbc/src/execute.rs` — 946 LOC, full fetch-decode-execute, 40+ opcodes, 30+ tests
- `crates/monad-mbc/src/lib.rs` — updated with module declarations and re-exports

### ALREADY EXISTED (DO NOT REWRITE)
- `ebpf/monad-cpu-ebpf/src/main.rs` — 1005 LOC, COMPLETE BPF XDP handler, ALL opcodes, 11 maps
- `ebpf/monad-common/src/lib.rs` — 1817 LOC, MbcCpuState (80 bytes), MbcInsn, Monad, 86+ tests
- `crates/monad-mbc/src/assembler.rs` — 697 LOC, COMPLETE two-pass assembler
- `crates/monad-mbc/src/disasm.rs` — 222 LOC, COMPLETE disassembler
- `crates/monad-mbc/src/translator.rs` — 1474 LOC, COMPLETE RV32I→MBC translator
- `crates/monad-mbc/tests/demo_programs.rs` — gradient + checkerboard demo tests
- `doom/doomgeneric/doom.mbc` — translated Doom ROM binary EXISTS
- `demos/mbc/` — add42.mbc, sum99.mbc, gradient.mbc, checkerboard.mbc

### KEY ARCHITECTURE FACTS
- **MbcCpuState is 80 bytes** (not 84 as S29 originally spec'd — S29 was written before monad-common existed)
- **MbcInsn encoding**: `[opcode:8][dst:4][src:4][imm16:16]` = 32-bit fixed width
- **MbcInsn API**: `MbcInsn(u32)`, `.opcode()`, `.dst()`, `.src()`, `.imm16()`, `.imm16_signed()`, `MbcInsn::encode(opcode, dst, src, imm)`
- **Branches use 24-bit offsets**: Lower 24 bits of instruction word (dst+src+imm16 combined)
- **halted field is u8** (0=running, 1=halted), NOT bool
- **REG_SP is usize = 15**, REG_SP_DEFAULT = 0xFFFF_0000
- **Memory map**: RAM_BASE=0, RAM_SIZE=0xC000 (48KB), SCREEN_BASE=0xC000, SCREEN_SIZE=64000, KBD_ADDR=0xFFFF
- **BPF maps**: ROM_MAP (262K), RAM_MAP (2M HashMap), SCREEN_MAP (64K), KBD_MAP, CPU_MAP (256), L1_CACHE (LRU 256), COMPUTE_EVENTS ring buffer
- **MAX_INSN_PER_TICK = 16** in BPF (tunable), MAX_CYCLES_PER_PACKET = 1000 in userspace

---

## RULES OF ENGAGEMENT

1. **TDD**: Write tests FIRST, then implementation (red-green-refactor)
2. **`#[inline(always)]`** on ALL functions in eBPF code paths
3. **Explicit bounds checks** before EVERY BPF map access
4. **`cargo +nightly test -p monad-mbc`** after every phase
5. **`go test -race ./...`** after every Go change
6. **Single-instance RAM_MAP** for S30 (multi-instance is Age 2)
7. **Auto-accept all decisions** — no design bikeshedding
8. **Commit after each phase** with descriptive message
9. **DO NOT rewrite existing working code** — extend, don't replace
10. **`#[repr(C)]`** on all BPF-shared structs

---

## PHASE 0: COMPILATION VERIFICATION (Steps 1-12)

> **Goal**: Ensure all committed code compiles and all tests pass.

### Step 1: Verify Rust toolchain
```bash
rustup show
cargo --version
# Need nightly for eBPF. If missing: rustup toolchain install nightly
```

### Step 2: Check workspace builds
```bash
cd /path/to/unheaded
cargo check -p monad-common 2>&1 | head -50
```

### Step 3: Build monad-mbc crate
```bash
cargo check -p monad-mbc 2>&1 | head -100
```

### Step 4: Fix any compilation errors in instruction.rs
- If `decode_checked` fails: verify `MbcInsn(u32)` constructor is pub
- If `is_valid_opcode` fails: verify all opcode constants imported from monad_common

### Step 5: Fix any compilation errors in cpu.rs
- If `MbcCpuState` field access fails: check field visibility in monad_common
- If `REG_SP_DEFAULT` not found: check if it's exported from monad_common
- The `ram_write_word` returns `bool`, NOT `Result` — tests use `assert!()` not `.unwrap()`

### Step 6: Fix any compilation errors in execute.rs
- `MbcInsn(insn_word)` NOT `MbcInsn::from_word(insn_word)`
- `insn.imm16() as u32` NOT `insn.imm()`
- `self.state.halted != 0` NOT `self.state.halted` (it's u8 not bool)
- `self.state.halted = 1` NOT `self.state.halted = true`
- Branch offsets: `insn.0 & 0x00FF_FFFF` for 24-bit extraction
- `wrapping_add(branch_offset as u32)` NOT `wrapping_add_signed(branch_offset)`

### Step 7: Fix any compilation errors in lib.rs
- Verify `pub use cpu::Cpu;` and `pub use execute::{Cpu as ExecCpu, ExecError};` don't conflict
- If assembler.rs has issues with new module imports, fix path references

### Step 8: Run monad-mbc unit tests
```bash
cargo test -p monad-mbc -- --test-threads=1 2>&1
```

### Step 9: Run instruction.rs tests specifically
```bash
cargo test -p monad-mbc -- instruction:: --test-threads=1
```

### Step 10: Run cpu.rs tests specifically
```bash
cargo test -p monad-mbc -- cpu:: --test-threads=1
```

### Step 11: Run execute.rs tests specifically
```bash
cargo test -p monad-mbc -- execute:: --test-threads=1
```

### Step 12: Run demo_programs integration tests
```bash
cargo test -p monad-mbc -- demo --test-threads=1
```

**EXIT GATE**: All tests pass. Zero compilation warnings on core modules.

---

## PHASE 1: RECONCILE S29 ISA WITH EXISTING CODEBASE (Steps 13-25)

> **Goal**: S29 spec'd opcodes 0x00-0x1F. Existing code has ~40 opcodes across 0x01-0xFF. Reconcile and document the ACTUAL ISA.

### Step 13: Document actual opcode table
Read `ebpf/monad-common/src/lib.rs` mod `mbc_opcodes`. Write a canonical table in `docs/protocol/mbc-isa-reference.md` listing every opcode, its value, encoding, and semantics.

### Step 14: Verify instruction.rs covers ALL opcodes
Compare `is_valid_opcode()` match arms against `mbc_opcodes` constants. Add any missing opcodes.

### Step 15: Verify execute.rs covers ALL opcodes
Compare `execute_insn()` match arms against BPF `main.rs` dispatch. Every opcode in BPF must have a userspace mirror.

### Step 16: Add ADDI opcode if missing
S29 spec'd `ADD_IMM (0x1D)`. Check if monad-common has ADDI. If not, add it:
```rust
pub const ADDI: u8 = 0x1D; // dst = dst + sign_extend(imm16)
```
Then add to execute.rs and instruction.rs.

### Step 17: Add PUSH/POP opcodes if missing
S29 spec'd PUSH (0x1A) and POP (0x1B). Check if they exist. If not:
```rust
pub const PUSH: u8 = 0x1A; // SP -= 4; RAM[SP] = regs[src]
pub const POP:  u8 = 0x1B; // regs[dst] = RAM[SP]; SP += 4
```

### Step 18: Add NOP opcode if missing
S29 spec'd NOP (0x00). Check if it exists. The current codebase may treat unknown opcodes as NOP.

### Step 19: Add LOAD_IMM32 opcode if missing
S29 spec'd `LOAD_IMM32 (0x1C)`: `regs[dst][31:16] = imm16`. Useful for loading full 32-bit values in two instructions.

### Step 20: Update assembler.rs for any new opcodes
If Steps 16-19 added opcodes, update the assembler mnemonic table.

### Step 21: Update disasm.rs for any new opcodes
Mirror assembler changes in the disassembler.

### Step 22: Update translator.rs for any new opcodes
If new opcodes enable better RV32I translation patterns, update the translator.

### Step 23: Update BPF main.rs for any new opcodes
If new opcodes were added to monad-common, add execution logic to the BPF dispatch.

### Step 24: Write tests for any new opcodes
TDD: test for each new opcode in execute.rs tests, instruction.rs tests.

### Step 25: Run full test suite
```bash
cargo test -p monad-mbc -- --test-threads=1
cargo test -p monad-common -- --test-threads=1
```

**EXIT GATE**: ISA fully documented. All opcodes have userspace mirrors. All tests pass.

---

## PHASE 2: FETCH-DECODE-EXECUTE HARDENING (Steps 26-50)

> **Goal**: Harden the userspace emulator and ensure it exactly mirrors BPF behavior.

### Step 26: Add ExecCpu::step() instruction counter
After each successful instruction, increment `self.state.insn_count`.

### Step 27: Add ExecCpu::step() cache miss/hit tracking
When accessing RAM, increment `self.state.cache_hits` or `self.state.cache_misses` based on whether the address was recently accessed.

### Step 28: Add screen region projection to LD/ST
When `addr >= SCREEN_BASE && addr < SCREEN_BASE + SCREEN_SIZE`, read/write from `self.screen` instead of `self.ram`. Verify this matches BPF behavior.

### Step 29: Add keyboard register projection
When `addr == KBD_ADDR (0xFFFF)`, read from `self.kbd`. Verify this matches BPF behavior.

### Step 30: Add memory-mapped I/O test
Write test: store byte to SCREEN_BASE, verify screen buffer updated. Read from KBD_ADDR, verify kbd value returned.

### Step 31: Add MOVI sign extension test
`MOVI r0, 0xFFFF` should set r0 to 0x0000FFFF (zero-extended u16), NOT -1. Verify against BPF behavior. If BPF sign-extends, match that.

### Step 32: Add CMP flag semantics verification
Write exhaustive test: CMP(10, 10) → Z set. CMP(10, 5) → result positive, no Z/N. CMP(5, 10) → result negative, N set. CMP(0xFFFFFFFF, 0) → borrow/carry. Verify against BPF.

### Step 33: Verify CALL pushes correct return address
BPF advances PC BEFORE dispatch, so CALL pushes PC+1 (already incremented). Our userspace does the same (line 71: `self.state.pc.wrapping_add(1)` before execute_insn). Write explicit test.

### Step 34: Verify RET pops correct address
Write test: CALL to addr 50, then RET. Verify PC returns to instruction AFTER the CALL.

### Step 35: Test nested CALL/RET
Write test: CALL → CALL → RET → RET. Verify SP and PC restoration at each level.

### Step 36: Test CALL stack overflow
CALL 256 times. Verify SP doesn't wrap below RAM bounds. If it does, implement stack depth guard.

### Step 37: Test division by zero behavior
DIV r0, r1 where r1=0: should set r0=0xFFFFFFFF and flags=C (carry). Verify matches BPF `main.rs` line ~308.

### Step 38: Test MOD by zero behavior
MOD r0, r1 where r1=0: should set r0=0. Verify matches BPF.

### Step 39: Test shift by 0, 1, 31, 32
SHL by 0 = no change. SHL by 32 = 0 (wrapping). Verify BPF behavior matches.

### Step 40: Test MULH correctness
0x80000000 × 2 = 0x1_00000000, high word = 1. Verify MULH returns 1.

### Step 41: Test backward branch boundary
JMP with offset -1 should jump to the JMP instruction itself (infinite loop). With cycle budget, should exhaust and return CycleBudgetExhausted.

### Step 42: Test forward branch out of ROM
JMP to PC > ROM length. Should trigger RomFault on next fetch.

### Step 43: Test SYSCALL dispatch
Write test for each syscall: SYS_DRAW_FRAME, SYS_GET_KEY, SYS_GET_TICKS, SYS_SLEEP. Verify register state after each.

### Step 44: Test invalid SYSCALL number
SYSCALL with r0=0xFF → should return ExecError::InvalidSyscall(0xFF).

### Step 45: Add property-based test for instruction roundtrip
For all valid opcodes, all register values 0-15, all imm16 values: encode → decode → verify fields match.

### Step 46: Add adversarial instruction stream test
Load ROM with 1000 random u32 values. Run with budget 1000. Must not panic.

### Step 47: Add sum_1_to_N program test for N=100
Verify result = 5050. This tests loop, compare, branch, arithmetic.

### Step 48: Add fibonacci program test
Compute fib(10) = 55. Tests: MOVI, ADD, MOV, CMP, JNZ.

### Step 49: Add memory copy program test
Store values to RAM, then copy them to another location. Verify correctness.

### Step 50: Run full test suite + clippy
```bash
cargo test -p monad-mbc -- --test-threads=1
cargo clippy -p monad-mbc -- -D warnings
```

**EXIT GATE**: Userspace emulator matches BPF instruction-for-instruction. All 40+ opcodes tested.

---

## PHASE 3: GO-SIDE DOOM LOADER (Steps 51-70)

> **Goal**: Build `doom` subcommand for wotan-ctl (or standalone binary) that loads ROM, manages CPU state, and handles I/O.

### Step 51: Create Go package structure
```
internal/doom/
├── loader.go       # ROM loading into BPF maps
├── loader_test.go
├── state.go        # CPU state read/write from BPF maps
├── state_test.go
├── input.go        # Keyboard input injection
├── input_test.go
└── types.go        # Go-side MBC types matching Rust #[repr(C)]
```

### Step 52: Define Go CpuState struct matching MbcCpuState
```go
type CpuState struct {
    Regs        [16]uint32
    PC          uint32
    Flags       uint8
    Halted      uint8
    Stalled     uint8
    Pad         uint8
    SleepUntilNs uint64  // Note: this is NOT split, it's u64 in MbcCpuState
    InsnCount   uint32
    CacheHits   uint32
    CacheMisses uint32
}
```

### Step 53: Write Go sizeof test
```go
func TestCpuStateSize(t *testing.T) {
    size := binary.Size(CpuState{})
    if size != 80 { t.Errorf("CpuState size = %d, want 80", size) }
}
```

### Step 54: Implement ROM loader
Read binary file, validate size (max 262144 × 4 bytes = 1 MiB), write to ROM_MAP via bpf map update. Use cilium/ebpf or custom bpf syscall wrapper.

### Step 55: Write ROM loader test
Load a small ROM (10 instructions), read back from map, verify byte-for-byte match.

### Step 56: Implement CPU state initializer
Write default CpuState to CPU_MAP keyed by flow_label. SP = REG_SP_DEFAULT. All else zero.

### Step 57: Write CPU state init test
Init CPU state for flow_label=0x12345, read back, verify SP and zeroed fields.

### Step 58: Implement CPU state reader
Read CpuState from CPU_MAP, pretty-print: PC, registers, flags, cycle count.

### Step 59: Write CPU state reader test
Init state, modify some fields, read back, verify all fields.

### Step 60: Implement keyboard input writer
Write u32 bitmap to KBD_MAP[0]. Define key mapping constants.

### Step 61: Write keyboard input test
Write 0xDEAD to KBD_MAP, read back, verify.

### Step 62: Implement CPU reset command
Zero CpuState, reset SP, clear halted flag, zero PC.

### Step 63: Write CPU reset test
Load state, set PC=100/halted=1, reset, verify PC=0/halted=0/SP=default.

### Step 64: Implement doom CLI subcommand: `doom load <rom.bin>`
Parse args, call loader, print summary (size, instruction count).

### Step 65: Implement doom CLI subcommand: `doom status [flow_label]`
Call state reader, format output as table.

### Step 66: Implement doom CLI subcommand: `doom input <key_bitmap>`
Parse hex bitmap, call keyboard writer.

### Step 67: Implement doom CLI subcommand: `doom reset [flow_label]`
Call CPU reset.

### Step 68: Implement doom CLI subcommand: `doom inject-tick [flow_label]`
Send a single packet with the Doom flow label to trigger one CPU tick. Uses raw socket or BPF_REDIRECT.

### Step 69: Implement ROM HMAC verification
Compute HMAC-SHA256 of ROM bytes. Store alongside ROM. On load, verify. On status, display.

### Step 70: Run Go tests with race detection
```bash
go test -v -race ./internal/doom/...
```

**EXIT GATE**: `doom load`, `doom status`, `doom input`, `doom reset` all work. Go struct matches Rust struct (80 bytes).

---

## PHASE 4: BPF VERIFIER COMPLIANCE (Steps 71-82)

> **Goal**: Ensure monad-cpu-ebpf passes BPF verifier and loads into kernel.

### Step 71: Build monad-cpu-ebpf for BPF target
```bash
cargo +nightly build -p monad-cpu-ebpf \
  --target bpfel-unknown-none \
  -Z build-std=core \
  --release 2>&1
```

### Step 72: If build fails — fix eBPF-specific issues
Common: missing `#[inline(always)]`, use of std types, dynamic dispatch. Fix each error.

### Step 73: Link with bpf-linker
```bash
# Use aya's bpf-linker
cargo install bpf-linker
bpf-linker target/bpfel-unknown-none/release/monad_cpu_ebpf \
  -o target/monad-cpu.bpf.o
```

### Step 74: Verify ELF sections
```bash
llvm-readelf -S target/monad-cpu.bpf.o
# Should see: xdp section, .maps section, .bss
```

### Step 75: Attempt kernel load
```bash
sudo bpftool prog load target/monad-cpu.bpf.o /sys/fs/bpf/monad_cpu \
  type xdp 2>&1
```

### Step 76: If verifier rejects — read verifier log
```bash
sudo bpftool prog load target/monad-cpu.bpf.o /sys/fs/bpf/monad_cpu \
  type xdp 2>&1 | tail -200
```

### Step 77: Fix verifier rejection tier 1 — unbounded loops
Make cycle budget a `const` visible to verifier. Use `#[allow(clippy::manual_range)]` if needed. BPF verifier needs to see `if cycles >= MAX { break }`.

### Step 78: Fix verifier rejection tier 2 — map access without bounds
Before every `ROM_MAP.get(idx)`, ensure `if idx < MAX_ENTRIES` is in same basic block.

### Step 79: Fix verifier rejection tier 3 — stack too deep
BPF stack = 512 bytes. CpuState = 80 bytes. Keep local variables minimal. Inline everything.

### Step 80: Fix verifier rejection tier 4 — program too large
If >1M verified instructions: split into tail-called programs. Each opcode group in separate program.

### Step 81: Fix verifier rejection tier 5 — missing inline
Ensure every function called from XDP handler is `#[inline(always)]`.

### Step 82: Verify successful load
```bash
sudo bpftool prog show name monad_cpu
# Should show: loaded program with XDP type
sudo bpftool prog detach name monad_cpu xdp
```

**EXIT GATE**: BPF program loads without verifier rejection.

---

## PHASE 5: FUZZ TESTING — LICH-007 CAMPAIGN (Steps 83-95)

> **Goal**: Launch MBC bytecode fuzzing. 72-hour campaign. Zero crashes.

### Step 83: Create fuzz directory structure
```
crates/monad-mbc/fuzz/
├── Cargo.toml
└── fuzz_targets/
    ├── fuzz_mbc_decode.rs
    ├── fuzz_mbc_execute.rs
    └── fuzz_mbc_roundtrip.rs
```

### Step 84: Write fuzz_mbc_decode target
Random u32 → `decode_checked()` — must not panic on any input.

### Step 85: Write fuzz_mbc_execute target
Random instruction stream + random initial registers → `Cpu::run(10_000)` — must not panic.

### Step 86: Write fuzz_mbc_roundtrip target
For valid opcodes: encode → decode → assert fields match.

### Step 87: Create seed corpus — minimal programs
- `seed_nop_halt`: `[NOP, HALT]`
- `seed_add_halt`: `[MOVI r0 1, MOVI r1 2, ADD r0 r1, HALT]`
- `seed_load_store_halt`: `[MOVI r0 42, MOVI r1 0x100, ST r1 r0 0, LD r0 r1 0, HALT]`
- `seed_loop`: `[MOVI r0 10, MOVI r1 1, SUB r0 r1, JNZ -2, HALT]`

### Step 88: Install cargo-fuzz
```bash
cargo install cargo-fuzz
```

### Step 89: Run fuzz_mbc_decode for 1 hour (initial smoke)
```bash
cd crates/monad-mbc && cargo +nightly fuzz run fuzz_mbc_decode -- -max_total_time=3600
```

### Step 90: Run fuzz_mbc_execute for 1 hour (initial smoke)
```bash
cargo +nightly fuzz run fuzz_mbc_execute -- -max_total_time=3600
```

### Step 91: Fix any crashes found
For each crash: reproduce, write regression test, fix code, verify test passes.

### Step 92: Run fuzz_mbc_roundtrip for 1 hour
```bash
cargo +nightly fuzz run fuzz_mbc_roundtrip -- -max_total_time=3600
```

### Step 93: Launch 72-hour LICH-007 campaign
```bash
# Run all 3 fuzz targets in parallel
cargo +nightly fuzz run fuzz_mbc_decode -- -max_total_time=259200 &
cargo +nightly fuzz run fuzz_mbc_execute -- -max_total_time=259200 &
cargo +nightly fuzz run fuzz_mbc_roundtrip -- -max_total_time=259200 &
```

### Step 94: Monitor coverage
```bash
cargo +nightly fuzz coverage fuzz_mbc_execute
# Target: >85% of instruction dispatch paths
```

### Step 95: Document LICH-007 results
Write `docs/sessions/S30-lich007-results.md` with: duration, exec/sec, coverage %, crashes found, fixes applied.

**EXIT GATE**: 72-hour fuzz campaign with 0 crashes. >85% coverage.

---

## PHASE 6: INTEGRATION TESTING (Steps 96-110)

> **Goal**: End-to-end tests proving the full pipeline works.

### Step 96: Write minimal MBC test ROM in assembly
```asm
; test_rom.mbc — exercises all instruction classes
    MOVI r0, 42          ; immediate load
    MOVI r1, 10          ; another immediate
    ADD r0, r1           ; arithmetic (r0 = 52)
    MOVI r2, 0x100       ; RAM address
    ST r2, r0, 0         ; store to RAM
    MOVI r0, 0           ; clear r0
    LD r0, r2, 0         ; load from RAM (r0 = 52)
    CALL subroutine      ; call/return
    SYSCALL              ; screen write (r0 = SYS_DRAW_FRAME)
    HALT
subroutine:
    MOVI r3, 99
    RET
```

### Step 97: Assemble test ROM
```bash
cargo run --bin mbc-asm -- test_rom.mbc -o test_rom.bin
```

### Step 98: Write integration test: assemble → load → execute → verify
Load assembled ROM into ExecCpu, run 1000 cycles, assert:
- r0 = 52 (after LD)
- r3 = 99 (from subroutine)
- RAM[0x100>>2] = 52
- CPU halted

### Step 99: Write integration test: disassemble → reassemble → compare
Assemble a program, disassemble it, reassemble the disassembly, compare bytecode.

### Step 100: Write integration test: translator roundtrip
If possible: simple C → RV32I (via cross-compiler) → MBC → execute → verify result.

### Step 101: Write screen rendering test
Program that writes gradient pattern to screen region. Verify screen buffer contents match expected pattern.

### Step 102: Write keyboard input test
Set kbd register, execute SYS_GET_KEY, verify r0 contains keyboard state.

### Step 103: Write timer test
Execute SYS_GET_TICKS, verify r0 contains reasonable value. Execute SYS_SLEEP with r1=100, verify ticks_ms advanced by 100.

### Step 104: Write cycle budget exhaustion test
Load infinite loop program (JMP -1), run with budget 500, verify returns CycleBudgetExhausted after exactly 500 cycles.

### Step 105: Write halt-in-subroutine test
CALL → ... → HALT. Verify CPU halts correctly even with stack frames.

### Step 106: Write doom.mbc smoke test (if ROM exists)
Load `doom/doomgeneric/doom.mbc`, execute 10000 cycles. Verify no panic, no RomFault. CPU should be running (not halted — Doom runs forever).

### Step 107: Write Go integration test
Go test that:
1. Compiles test ROM
2. Loads into mock BPF maps (or uses cilium/ebpf test framework)
3. Sends mock packet
4. Reads CPU state
5. Verifies execution happened

### Step 108: Write concurrent execution test
Launch 10 goroutines each sending packets for different flow labels. Verify no cross-contamination.

### Step 109: Write BPF map size stress test
Load maximum-size ROM (262144 instructions). Verify map handles it. Load into CPU, execute first 100 instructions.

### Step 110: Run full integration test suite
```bash
cargo test -p monad-mbc -- --test-threads=1
go test -v -race ./internal/doom/...
go test -v -race ./cmd/wotan-ctl/...
```

**EXIT GATE**: All integration tests pass. End-to-end pipeline proven.

---

## PHASE 7: ROM TOOLCHAIN & DASHBOARD (Steps 111-120)

> **Goal**: Polish the assembler, add dashboard screen rendering, document everything.

### Step 111: Enhance assembler with .data directive
Support `.data` section for embedding raw bytes into ROM. Useful for string tables and lookup data.

### Step 112: Enhance assembler with .org directive
Support `.org <address>` to set assembly origin. Useful for ROM layout control.

### Step 113: Add assembler macro support (stretch)
Simple text substitution macros: `.macro push_all` → expands to PUSH sequence.

### Step 114: Write dashboard screen renderer endpoint
Go HTTP handler that reads SCREEN_MAP (or ring buffer), converts to PNG/framebuffer, serves at `/doom/screen`.

### Step 115: Write dashboard CPU status endpoint
Go HTTP handler that reads CPU_MAP, returns JSON with PC, registers, flags, cycle count.

### Step 116: Write dashboard input endpoint
Go HTTP POST handler that accepts keyboard state, writes to KBD_MAP.

### Step 117: Write simple HTML Doom viewer
Minimal HTML page: canvas element, fetches `/doom/screen` on timer (30 FPS), sends key events to `/doom/input`.

### Step 118: Document the MBC ISA
Final `docs/protocol/mbc-isa-reference.md` with:
- Full opcode table
- Encoding diagrams
- Flag semantics
- Memory map
- Syscall reference
- Example programs

### Step 119: Document the Doom-over-IPv6 architecture
Final `docs/protocol/doom-over-ipv6-architecture.md` with:
- Memory hierarchy (L0-L4)
- Packet-as-clock-tick model
- BPF map layout
- Screen rendering pipeline
- Keyboard input pipeline

### Step 120: Final verification sprint
```bash
# Full Rust test suite
cargo test --workspace -- --test-threads=1

# Full Go test suite
go test -v -race ./...

# Clippy clean
cargo clippy --workspace -- -D warnings

# BPF build
cargo +nightly build -p monad-cpu-ebpf --target bpfel-unknown-none -Z build-std=core --release

# Fuzz results check
ls crates/monad-mbc/fuzz/artifacts/  # Should be empty (no crashes)
```

**EXIT GATE**: Ship it. Doom runs over IPv6.

---

## S29/S30 EXIT CRITERIA CHECKLIST

- [ ] All 40+ MBC opcodes decode without panic on any input
- [ ] All ALU operations handle overflow/underflow with wrapping semantics
- [ ] Division by zero saturates to 0xFFFFFFFF (not kernel panic)
- [ ] Memory access OOB returns 0/false (not segfault)
- [ ] Cycle budget prevents infinite loops in XDP
- [ ] BPF verifier ACCEPTS the monad-cpu-ebpf program
- [ ] Minimal test ROM executes correctly (counter + subroutine + syscall)
- [ ] SYS_DRAW_FRAME writes to screen buffer / ring buffer
- [ ] SYS_GET_KEY reads from KBD_MAP
- [ ] LICH-007 running for 72 hours with 0 crashes
- [ ] All unit tests pass with race detection
- [ ] Go/Rust struct parity verified (80 bytes CpuState)
- [ ] `doom load` / `doom status` / `doom input` / `doom reset` commands work
- [ ] doom.mbc ROM loads and executes without crash
- [ ] Userspace emulator matches BPF dispatch instruction-for-instruction
- [ ] ISA reference document complete
- [ ] Architecture document complete

---

## APPENDIX: FILE INVENTORY (Read Before Writing)

| File | LOC | Status | Notes |
|------|-----|--------|-------|
| `ebpf/monad-common/src/lib.rs` | 1817 | COMPLETE | DO NOT MODIFY unless adding opcodes |
| `ebpf/monad-cpu-ebpf/src/main.rs` | 1005 | COMPLETE | BPF XDP handler, 40+ opcodes |
| `crates/monad-mbc/src/instruction.rs` | 623 | COMPLETE | Opcode validation, decode_checked |
| `crates/monad-mbc/src/cpu.rs` | 650 | COMPLETE | Userspace CPU state wrapper |
| `crates/monad-mbc/src/execute.rs` | 946 | COMPLETE | Userspace fetch-decode-execute |
| `crates/monad-mbc/src/assembler.rs` | 697 | COMPLETE | Two-pass assembler |
| `crates/monad-mbc/src/disasm.rs` | 222 | COMPLETE | Full disassembler |
| `crates/monad-mbc/src/translator.rs` | 1474 | COMPLETE | RV32I → MBC |
| `crates/monad-mbc/src/lib.rs` | 35 | COMPLETE | Module declarations |
| `crates/monad-mbc/tests/demo_programs.rs` | 80+ | COMPLETE | Gradient, checkerboard demos |
| `crates/monad-mbc/Cargo.toml` | — | COMPLETE | deps: monad-common, thiserror, goblin |
| `internal/doom/` | 0 | MISSING | GO-SIDE LOADER — BUILD THIS |
| `crates/monad-mbc/fuzz/` | 0 | MISSING | FUZZ TARGETS — BUILD THIS |
| `docs/protocol/mbc-isa-reference.md` | 0 | MISSING | ISA DOCS — WRITE THIS |

---

*Forged by the BlackMage and the Developer.*
*120 steps. 7 phases. Zero tolerance for panics.*
*Break everything. Fix everything. Ship Doom over IPv6.* 🗡️🔥
