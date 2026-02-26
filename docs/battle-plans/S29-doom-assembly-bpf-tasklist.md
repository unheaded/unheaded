# S29: DOOM ASSEMBLY / BPF — Detailed Task List

**Sprint**: S29 (Target: post-Alpha gate)
**Scope**: monad-cpu-ebpf fetch-decode-execute implementation (THE Doom blocker)
**Effort**: XL (2-3 days focused dev time)
**Owner**: Developer (implementation) + BlackMage (adversarial validation)
**Dependencies**: S27 P0-P3 complete (Alpha gate closed), LICH-007 wired

---

## EXECUTIVE SUMMARY

The monad-cpu-ebpf program is the beating heart of Doom-over-IPv6. It runs inside XDP, processing packets as CPU clock ticks. Each packet triggers one fetch-decode-execute cycle of the MBC (Monad ByteCode) instruction set. Wotan provides RAM (L1 cache BPF maps). Sophia provides the dictionary mapping. The framebuffer writes to a Wotan topic. Keyboard reads from a Wotan topic. The flow label identifies the Doom CPU instance.

**Current state**: STUB. The fetch-decode-execute loop is scaffolded but not implemented.
**Target state**: Full MBC instruction dispatch running Doom at ~35 FPS in XDP.

---

## ARCHITECTURE (What We're Building)

```
                    ┌─────────────────────────────────────┐
                    │         INCOMING IPv6 PACKET         │
                    │  (Flow Label = Doom CPU instance ID) │
                    └───────────────┬─────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────┐
│  monad-cpu-ebpf (XDP program)                                    │
│                                                                   │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────┐      │
│  │ FETCH       │→ │ DECODE       │→ │ EXECUTE            │      │
│  │ PC → ROM_MAP│  │ opcode match │  │ ALU / MEM / BRANCH │      │
│  │ read instr  │  │ extract args │  │ SYSCALL dispatch   │      │
│  └─────────────┘  └──────────────┘  └────────────────────┘      │
│       ↕                                      ↕                    │
│  ┌─────────────┐                    ┌────────────────────┐      │
│  │ ROM_MAP     │                    │ CPU_STATE_MAP      │      │
│  │ (BPF_ARRAY) │                    │ (BPF_HASH)         │      │
│  │ Game ROM    │                    │ regs, PC, SP, flags│      │
│  └─────────────┘                    └────────────────────┘      │
│                                              ↕                    │
│                                     ┌────────────────────┐      │
│                                     │ RAM_MAP            │      │
│                                     │ (BPF_ARRAY)        │      │
│                                     │ 64KB address space │      │
│                                     └────────────────────┘      │
│                                              ↕                    │
│                              ┌───────────────────────────┐      │
│                              │ SYSCALL DISPATCH          │      │
│                              │ 0x01: SCREEN_WRITE        │      │
│                              │ 0x02: KBD_READ            │      │
│                              │ 0x03: TIMER_READ          │      │
│                              │ 0x04: EXIT                │      │
│                              └───────────────────────────┘      │
│                                              ↕                    │
│                              ┌───────────────────────────┐      │
│                              │ RING BUFFER → Anamnesis   │      │
│                              │ (framebuffer, events)     │      │
│                              └───────────────────────────┘      │
└──────────────────────────────────────────────────────────────────┘
```

---

## PHASE 1: MBC INSTRUCTION SET DEFINITION (Day 1, Morning)

### Task 1.1: Define MBC Instruction Encoding

**File**: `crates/monad-mbc/src/instruction.rs` (new or expand stub)
**Effort**: M (2 hours)

```
MBC INSTRUCTION FORMAT (32-bit fixed width)
┌──────────┬──────────┬──────────┬──────────┐
│ Opcode   │ Dst Reg  │ Src Reg  │ Immediate│
│ (8 bits) │ (4 bits) │ (4 bits) │ (16 bits)│
└──────────┴──────────┴──────────┴──────────┘
```

**Implementation checklist:**

- [ ] Define `Opcode` enum (Rust `#[repr(u8)]`):
  ```
  0x00  NOP         No operation
  0x01  LOAD_IMM    Rdst = imm16 (sign-extended to 32-bit)
  0x02  LOAD_MEM    Rdst = RAM[Rsrc + imm16]
  0x03  STORE_MEM   RAM[Rdst + imm16] = Rsrc
  0x04  MOV         Rdst = Rsrc
  0x05  ADD         Rdst = Rdst + Rsrc
  0x06  SUB         Rdst = Rdst - Rsrc
  0x07  MUL         Rdst = Rdst * Rsrc
  0x08  DIV         Rdst = Rdst / Rsrc (trap on /0)
  0x09  MOD         Rdst = Rdst % Rsrc (trap on /0)
  0x0A  AND         Rdst = Rdst & Rsrc
  0x0B  OR          Rdst = Rdst | Rsrc
  0x0C  XOR         Rdst = Rdst ^ Rsrc
  0x0D  SHL         Rdst = Rdst << (Rsrc & 0x1F)
  0x0E  SHR         Rdst = Rdst >> (Rsrc & 0x1F) (logical)
  0x0F  SRA         Rdst = Rdst >> (Rsrc & 0x1F) (arithmetic)
  0x10  CMP         flags = compare(Rdst, Rsrc)
  0x11  JMP         PC = PC + imm16 (relative)
  0x12  JEQ         if flags.EQ: PC = PC + imm16
  0x13  JNE         if !flags.EQ: PC = PC + imm16
  0x14  JLT         if flags.LT: PC = PC + imm16
  0x15  JGT         if flags.GT: PC = PC + imm16
  0x16  JLE         if flags.LT || flags.EQ: PC = PC + imm16
  0x17  JGE         if flags.GT || flags.EQ: PC = PC + imm16
  0x18  CALL        push PC+1, PC = imm16 (absolute)
  0x19  RET         PC = pop
  0x1A  PUSH        SP -= 4, RAM[SP] = Rsrc
  0x1B  POP         Rdst = RAM[SP], SP += 4
  0x1C  LOAD_IMM32  Rdst[31:16] = imm16 (load upper, pair with LOAD_IMM)
  0x1D  ADD_IMM     Rdst = Rdst + imm16 (sign-extended)
  0x1E  SYSCALL     syscall(imm16) — dispatch to I/O handlers
  0x1F  HALT        Stop execution, emit CPU state to ring buffer
  ```
- [ ] Define `Instruction` struct: `{ opcode: Opcode, dst: u4, src: u4, imm: i16 }`
- [ ] Implement `Instruction::decode(raw: u32) -> Result<Instruction, DecodeError>`
- [ ] Implement `Instruction::encode(&self) -> u32`
- [ ] Round-trip property test: `decode(encode(instr)) == instr` for all valid instructions

**🗡️ BLACKMAGE ATTACK VECTORS:**

| Vector | Test | Severity |
|--------|------|----------|
| Invalid opcode (0x20-0xFF) | Decode must return `Err`, not panic | HIGH |
| Register index > 15 | Impossible in 4-bit field, but verify decode masks correctly | MEDIUM |
| Immediate sign extension | `0xFFFF` as imm16 → must become `-1` as i32, not `65535` | HIGH |
| Truncated instruction (<4 bytes) | Decode from short slice must return `Err` | HIGH |
| NOP slide | 64K NOPs followed by HALT — must terminate, not loop forever | MEDIUM |

**Verification gate:**
```bash
cargo +nightly test -p monad-mbc -- --test-threads=1
# All decode/encode round-trips pass
# All invalid opcode inputs return Err
# All boundary immediate values sign-extend correctly
```

---

### Task 1.2: Define CPU State Structure

**File**: `crates/monad-mbc/src/cpu.rs` (new)
**Effort**: S (1 hour)

**Implementation checklist:**

- [ ] Define `CpuState` struct (must be `#[repr(C)]` for BPF map compatibility):
  ```rust
  #[repr(C)]
  pub struct CpuState {
      pub regs: [u32; 16],      // R0-R15 general purpose
      pub pc: u32,              // Program counter
      pub sp: u32,              // Stack pointer (starts at RAM top)
      pub flags: u8,            // EQ=0x01, LT=0x02, GT=0x04
      pub halted: u8,           // 0=running, 1=halted
      pub cycle_count: u64,     // Total instructions executed
      pub _padding: [u8; 2],    // Align to 8-byte boundary
  }
  // Total size: 64 + 4 + 4 + 1 + 1 + 8 + 2 = 84 bytes
  ```
- [ ] Implement `CpuState::new() -> Self` (zero-init, SP = RAM_SIZE - 4)
- [ ] Implement `CpuState::reset(&mut self)` (same as new)
- [ ] `const_assert!(std::mem::size_of::<CpuState>() == 84)` — BPF struct parity!
- [ ] Verify `#[repr(C)]` layout matches Go-side `encoding/binary.Read` (ADR-016)

**🗡️ BLACKMAGE ATTACK VECTORS:**

| Vector | Test | Severity |
|--------|------|----------|
| BPF struct parity | Go sizeof must == Rust sizeof (84 bytes) | CRITICAL |
| Alignment padding | Verify no hidden padding bytes between fields | HIGH |
| Endianness | Both sides must agree on byte order (native LE on arm64) | CRITICAL |
| Max cycle_count | Overflow at u64::MAX — wraps to 0, must not corrupt state | MEDIUM |
| Flags bit isolation | Setting EQ must not affect LT/GT bits | HIGH |

**Verification gate:**
```bash
# Rust side
cargo +nightly test -p monad-mbc -- cpu_state
# Go side (must match)
go test -run TestCpuStateSize ./internal/monad/mbc/
```

---

### Task 1.3: Define BPF Map Schemas

**File**: `crates/monad-cpu-ebpf/src/maps.rs` (new or expand)
**Effort**: M (1.5 hours)

**Implementation checklist:**

- [ ] **ROM_MAP** — `BPF_ARRAY`, key=u32 (instruction index), value=u32 (raw instruction)
  ```rust
  #[map]
  static ROM_MAP: Array<u32> = Array::with_max_entries(65536, 0);
  // 65536 instructions × 4 bytes = 256KB max ROM
  ```
- [ ] **CPU_STATE_MAP** — `BPF_HASH`, key=u32 (flow_label), value=CpuState
  ```rust
  #[map]
  static CPU_STATE_MAP: HashMap<u32, CpuState> = HashMap::with_max_entries(64, 0);
  // 64 concurrent Doom instances max
  ```
- [ ] **RAM_MAP** — `BPF_ARRAY`, key=u32 (address >> 2), value=u32 (word)
  ```rust
  #[map]
  static RAM_MAP: Array<u32> = Array::with_max_entries(16384, 0);
  // 16384 words × 4 bytes = 64KB RAM per instance
  // NOTE: Per-instance RAM requires composite key or per-flow maps
  ```
- [ ] **SCREEN_BUF** — `BPF_RINGBUF`, for framebuffer writes to userspace
  ```rust
  #[map]
  static SCREEN_BUF: RingBuf = RingBuf::with_byte_size(1048576, 0);
  // 1MB ring buffer for screen frames (320×200×1 = 64KB per frame)
  ```
- [ ] **INPUT_MAP** — `BPF_ARRAY`, key=u32 (0), value=u32 (keyboard state bitmap)
  ```rust
  #[map]
  static INPUT_MAP: Array<u32> = Array::with_max_entries(1, 0);
  ```
- [ ] **DOOM_STATS** — `BPF_PERCPU_ARRAY`, key=u32, value=u64
  ```rust
  #[map]
  static DOOM_STATS: PerCpuArray<u64> = PerCpuArray::with_max_entries(16, 0);
  // 0=cycles, 1=instructions, 2=syscalls, 3=frames, 4=errors
  ```

**🗡️ BLACKMAGE ATTACK VECTORS:**

| Vector | Test | Severity |
|--------|------|----------|
| ROM_MAP unsigned replacement (D1) | No HMAC → anyone with map FD can rewrite ROM | CRITICAL |
| CPU_STATE_MAP flow collision (D4) | 20-bit flow label birthday attack | HIGH |
| RAM_MAP shared across instances | If not per-instance, flow A reads flow B's memory | CRITICAL |
| SCREEN_BUF exhaustion | Emit frames faster than userspace reads → ring full → drops | MEDIUM |
| INPUT_MAP injection (D3) | Userspace can write to INPUT_MAP → control game | HIGH |
| Map FD leak to unprivileged | Check that map pinning has restrictive permissions | HIGH |

**Verification gate:**
```bash
cargo +nightly check -p monad-cpu-ebpf
# Maps compile with correct types and sizes
# Verify map sizes don't exceed kernel limits
```

---

## PHASE 2: FETCH-DECODE-EXECUTE CORE (Day 1, Afternoon → Day 2, Morning)

### Task 2.1: Implement FETCH Stage

**File**: `crates/monad-cpu-ebpf/src/main.rs` (expand XDP handler)
**Effort**: M (1.5 hours)

**Implementation checklist:**

- [ ] Extract flow label from IPv6 header (already done in packet-marker, reuse pattern)
- [ ] Lookup CPU state from `CPU_STATE_MAP` using flow label as key
- [ ] If no state: initialize new `CpuState`, insert into map (first packet for this flow)
- [ ] If halted: increment stats, return `XDP_PASS` (CPU is done)
- [ ] Read instruction from `ROM_MAP[cpu.pc]`
- [ ] Bounds check: `if cpu.pc >= ROM_SIZE { halt with error }`
- [ ] Increment `cpu.cycle_count`

```rust
#[inline(always)]
fn fetch(flow_label: u32) -> Result<(CpuState, u32), FetchError> {
    // Get or create CPU state
    let state = unsafe {
        CPU_STATE_MAP.get_ptr_mut(&flow_label)
            .ok_or(FetchError::NoState)?
    };

    if state.halted != 0 {
        return Err(FetchError::Halted);
    }

    // Bounds check PC
    if state.pc >= ROM_MAX_ENTRIES {
        state.halted = 1;
        return Err(FetchError::PcOutOfBounds);
    }

    // Read instruction at PC
    let raw_instr = unsafe {
        ROM_MAP.get(state.pc)
            .ok_or(FetchError::RomReadFailed)?
    };

    state.cycle_count += 1;
    Ok((*state, *raw_instr))
}
```

**🗡️ BLACKMAGE ATTACK VECTORS:**

| Vector | Test | Severity |
|--------|------|----------|
| PC at ROM_MAP boundary | PC = max_entries - 1 (last valid), max_entries (OOB) | HIGH |
| PC overflow | PC = u32::MAX, increment wraps to 0 | HIGH |
| get_ptr_mut TOCTOU (S20 race) | Two packets same flow label → concurrent state mutation | CRITICAL |
| Missing flow label | Packet has flow_label=0 → valid? sentinel? | MEDIUM |
| New state initialization race | Two packets arrive simultaneously for new flow → double init | HIGH |

**Verification gate:**
```bash
# Unit test: fetch with valid PC returns instruction
# Unit test: fetch with PC out of bounds returns Err
# Unit test: fetch with halted state returns Err
# Concurrency test: 100 packets same flow, verify state consistency
```

---

### Task 2.2: Implement DECODE Stage

**File**: `crates/monad-cpu-ebpf/src/decode.rs` (new) or inline in main
**Effort**: S (1 hour)

**Implementation checklist:**

- [ ] Decode u32 → `(opcode, dst_reg, src_reg, imm16)`:
  ```rust
  #[inline(always)]
  fn decode(raw: u32) -> (u8, u8, u8, i16) {
      let opcode = (raw >> 24) as u8;
      let dst    = ((raw >> 20) & 0xF) as u8;
      let src    = ((raw >> 16) & 0xF) as u8;
      let imm    = (raw & 0xFFFF) as i16;
      (opcode, dst, src, imm)
  }
  ```
- [ ] Validate opcode range: `0x00..=0x1F` valid, else trap
- [ ] **BPF verifier constraint**: decode must be `#[inline(always)]` — no function calls in XDP hot path that the verifier can't trace

**🗡️ BLACKMAGE ATTACK VECTORS:**

| Vector | Test | Severity |
|--------|------|----------|
| Opcode 0x20-0xFF | Must trap, not fall through to random handler | HIGH |
| Bit extraction correctness | Verify each field extracted from correct bit positions | CRITICAL |
| Sign extension of imm16 | `0x8000` as u16 → must become `-32768` as i16 | HIGH |
| Verifier rejection | If decode is not inline, BPF verifier may reject program | CRITICAL |

---

### Task 2.3: Implement EXECUTE Stage — ALU Operations

**File**: `crates/monad-cpu-ebpf/src/execute.rs` (new)
**Effort**: L (3 hours)

**Implementation checklist:**

- [ ] **NOP (0x00)**: No-op, advance PC
- [ ] **LOAD_IMM (0x01)**: `regs[dst] = sign_extend_16_to_32(imm)`
- [ ] **MOV (0x04)**: `regs[dst] = regs[src]`
- [ ] **ADD (0x05)**: `regs[dst] = regs[dst].wrapping_add(regs[src])` — WRAPPING, not panic
- [ ] **SUB (0x06)**: `regs[dst] = regs[dst].wrapping_sub(regs[src])`
- [ ] **MUL (0x07)**: `regs[dst] = regs[dst].wrapping_mul(regs[src])`
- [ ] **DIV (0x08)**: `if regs[src] == 0 { trap } else { regs[dst] = regs[dst] / regs[src] }`
- [ ] **MOD (0x09)**: Same div-by-zero guard
- [ ] **AND (0x0A)**: `regs[dst] = regs[dst] & regs[src]`
- [ ] **OR (0x0B)**: `regs[dst] = regs[dst] | regs[src]`
- [ ] **XOR (0x0C)**: `regs[dst] = regs[dst] ^ regs[src]`
- [ ] **SHL (0x0D)**: `regs[dst] = regs[dst] << (regs[src] & 0x1F)` — mask shift amount!
- [ ] **SHR (0x0E)**: `regs[dst] = regs[dst] >> (regs[src] & 0x1F)` (logical)
- [ ] **SRA (0x0F)**: `regs[dst] = (regs[dst] as i32 >> (regs[src] & 0x1F)) as u32` (arithmetic)
- [ ] **CMP (0x10)**: Set flags based on `regs[dst]` vs `regs[src]`
- [ ] **ADD_IMM (0x1D)**: `regs[dst] = regs[dst].wrapping_add(sign_extend(imm))`
- [ ] **LOAD_IMM32 (0x1C)**: `regs[dst] = (regs[dst] & 0x0000FFFF) | ((imm as u32) << 16)`

```rust
#[inline(always)]
fn execute_alu(state: &mut CpuState, opcode: u8, dst: u8, src: u8, imm: i16) -> Result<(), ExecError> {
    let d = dst as usize;
    let s = src as usize;
    // BPF verifier needs to see bounds check on array index
    if d >= 16 || s >= 16 {
        return Err(ExecError::InvalidRegister);
    }
    match opcode {
        0x00 => { /* NOP */ },
        0x01 => { state.regs[d] = imm as i32 as u32; },
        0x04 => { state.regs[d] = state.regs[s]; },
        0x05 => { state.regs[d] = state.regs[d].wrapping_add(state.regs[s]); },
        // ... etc
        _ => return Err(ExecError::InvalidOpcode),
    }
    state.pc += 1;
    Ok(())
}
```

**🗡️ BLACKMAGE ATTACK VECTORS:**

| Vector | Test | Severity |
|--------|------|----------|
| Division by zero | DIV/MOD with Rsrc=0 must trap gracefully, not panic kernel | CRITICAL |
| Shift by 32+ bits | Undefined behavior in many ISAs — mask to 0x1F | HIGH |
| Integer overflow | ADD u32::MAX + 1 must wrap to 0, not panic | HIGH |
| Register index OOB | Despite 4-bit field, verify bounds check present for verifier | CRITICAL |
| SRA sign propagation | Negative >> must propagate sign bit, not zero-fill | HIGH |
| All opcodes exercised | Every opcode path must have at least one test | HIGH |
| LOAD_IMM32 masking | Must preserve lower 16 bits of destination register | MEDIUM |

**Verification gate:**
```bash
# Test every ALU operation with:
# - Normal values
# - Boundary values (0, 1, MAX-1, MAX)
# - Division by zero
# - Shift by 0, 1, 31, 32
cargo +nightly test -p monad-mbc -- alu
```

---

### Task 2.4: Implement EXECUTE Stage — Memory Operations

**File**: same `execute.rs`
**Effort**: M (2 hours)

**Implementation checklist:**

- [ ] **LOAD_MEM (0x02)**: `regs[dst] = RAM_MAP[(regs[src] + imm) >> 2]`
  - Address must be word-aligned (& 0xFFFFFFFC) or trap
  - Address must be < RAM_SIZE (64KB = 16384 words)
  - BPF verifier needs to see explicit bounds check before map access
- [ ] **STORE_MEM (0x03)**: `RAM_MAP[(regs[dst] + imm) >> 2] = regs[src]`
  - Same alignment and bounds checks
- [ ] **PUSH (0x1A)**: `SP -= 4; RAM_MAP[SP >> 2] = regs[src]`
  - Check SP >= 4 before decrement (stack overflow)
- [ ] **POP (0x1B)**: `regs[dst] = RAM_MAP[SP >> 2]; SP += 4`
  - Check SP < RAM_SIZE before increment (stack underflow)

```rust
#[inline(always)]
fn execute_mem(
    state: &mut CpuState,
    opcode: u8,
    dst: u8,
    src: u8,
    imm: i16,
) -> Result<(), ExecError> {
    match opcode {
        0x02 => { // LOAD_MEM
            let addr = state.regs[src as usize].wrapping_add(imm as i32 as u32);
            let word_idx = addr >> 2;
            if word_idx >= RAM_MAX_WORDS {
                return Err(ExecError::MemoryOutOfBounds);
            }
            let val = unsafe {
                RAM_MAP.get(word_idx).ok_or(ExecError::RamReadFailed)?
            };
            state.regs[dst as usize] = *val;
        },
        0x03 => { // STORE_MEM
            let addr = state.regs[dst as usize].wrapping_add(imm as i32 as u32);
            let word_idx = addr >> 2;
            if word_idx >= RAM_MAX_WORDS {
                return Err(ExecError::MemoryOutOfBounds);
            }
            unsafe {
                // BPF map update in XDP context
                RAM_MAP.set(word_idx, &state.regs[src as usize], 0)
                    .map_err(|_| ExecError::RamWriteFailed)?;
            };
        },
        // PUSH, POP similar with SP checks
        _ => return Err(ExecError::InvalidOpcode),
    }
    state.pc += 1;
    Ok(())
}
```

**🗡️ BLACKMAGE ATTACK VECTORS:**

| Vector | Test | Severity |
|--------|------|----------|
| Address overflow | `regs[src] = 0xFFFFFFFF, imm = 1` → wrapping to 0, is that valid? | CRITICAL |
| Unaligned access | Address & 0x3 != 0 → must trap or align-down | HIGH |
| OOB read/write | Address >= 64KB (RAM_MAX_WORDS × 4) | CRITICAL |
| Stack overflow | PUSH when SP = 0 → underflow | HIGH |
| Stack underflow | POP when SP = RAM_SIZE → reading beyond stack | HIGH |
| RAM_MAP shared state (multi-flow) | Two Doom instances share RAM_MAP → CORRUPTION | CRITICAL |
| BPF verifier bounds | Verifier must see `word_idx < MAX` before map access | CRITICAL |

**⚠️ CRITICAL DESIGN DECISION: Per-Instance RAM**

RAM_MAP as a single `BPF_ARRAY` is SHARED across all flows. Two Doom instances writing to the same address corrupt each other. Options:

1. **Composite key**: `HashMap<(flow_label, address), u32>` — high overhead per access
2. **Map-of-maps**: Outer map keyed by flow_label, inner map is per-flow RAM — complex
3. **Single-instance**: Only support 1 Doom instance in XDP — simplest, ship first
4. **Partitioned array**: `RAM_MAP[flow_label * 16384 + address]` — fixed max instances

**BlackMage recommendation**: Option 3 (single instance) for S29. Multi-instance is Age 2. Document the shared-state risk as a known limitation.

---

### Task 2.5: Implement EXECUTE Stage — Branch Operations

**File**: same `execute.rs`
**Effort**: M (1.5 hours)

**Implementation checklist:**

- [ ] **JMP (0x11)**: `PC = PC + imm` (relative, signed)
  - After: verify PC is in bounds
- [ ] **JEQ (0x12)**: `if flags & EQ != 0 { PC = PC + imm } else { PC += 1 }`
- [ ] **JNE (0x13)**: `if flags & EQ == 0 { ... }`
- [ ] **JLT (0x14)**: `if flags & LT != 0 { ... }`
- [ ] **JGT (0x15)**: `if flags & GT != 0 { ... }`
- [ ] **JLE (0x16)**: `if flags & (LT | EQ) != 0 { ... }`
- [ ] **JGE (0x17)**: `if flags & (GT | EQ) != 0 { ... }`
- [ ] **CALL (0x18)**: `push(PC + 1); PC = imm` (absolute)
  - Call stack depth check (max 256 frames = 1024 bytes of stack)
- [ ] **RET (0x19)**: `PC = pop()`
  - Check stack not empty before pop
- [ ] **HALT (0x1F)**: `halted = 1; emit state to ring buffer`

**🗡️ BLACKMAGE ATTACK VECTORS:**

| Vector | Test | Severity |
|--------|------|----------|
| Backward jump to self | `JMP -0` → infinite loop → must have cycle budget | CRITICAL |
| Jump to negative PC | `PC + imm < 0` → wrapping → OOB ROM read | CRITICAL |
| CALL stack overflow | 257 nested CALLs → stack exceeds RAM | HIGH |
| RET with empty stack | RET on first instruction → SP at top of RAM | HIGH |
| Branch condition atomicity | CMP then conditional jump must be atomic (no interrupt between) | MEDIUM |
| HALT during SYSCALL | HALT inside a CALL frame → orphaned stack | LOW |

**⚠️ CRITICAL: CYCLE BUDGET**

BPF programs have an instruction limit (~1M verified instructions). More importantly, XDP must not block the packet path. We MUST implement a per-packet cycle budget:

```rust
const MAX_CYCLES_PER_PACKET: u32 = 1000;
// At 5600 packets/sec (35 FPS × 160 packets/frame):
// 1000 MBC instructions per packet × 5600 packets/sec = 5.6M instructions/sec
// Doom runs at ~70K instructions/frame → 160 packets/frame → 437 instructions/packet
// Budget of 1000 gives 2.3× headroom
```

- [ ] Add cycle counter per XDP invocation
- [ ] Break execute loop when `cycle_budget` exhausted
- [ ] Save CPU state back to map (resume on next packet)
- [ ] Return `XDP_PASS` to release packet

---

### Task 2.6: Implement SYSCALL Dispatch

**File**: `crates/monad-cpu-ebpf/src/syscall.rs` (new)
**Effort**: L (3 hours)

**Implementation checklist:**

- [ ] **SYSCALL 0x01: SCREEN_WRITE**
  - R0 = framebuffer address in RAM
  - R1 = width, R2 = height (validate ≤ 320×200)
  - Copy RAM region to `SCREEN_BUF` ring buffer
  - Format: `{ flow_label: u32, frame_no: u32, width: u16, height: u16, pixels: [u8; width*height] }`
  ```rust
  fn syscall_screen_write(state: &CpuState) -> Result<(), SyscallError> {
      let fb_addr = state.regs[0];
      let width = state.regs[1].min(320) as u16;
      let height = state.regs[2].min(200) as u16;
      let size = (width as u32) * (height as u32);

      // Bounds check framebuffer region in RAM
      if fb_addr + size > RAM_SIZE {
          return Err(SyscallError::FramebufferOob);
      }

      // Reserve ring buffer space
      let buf = unsafe {
          SCREEN_BUF.reserve::<ScreenFrame>(0)
              .ok_or(SyscallError::RingFull)?
      };

      // Copy pixels from RAM_MAP to ring buffer
      // ... (iterate RAM_MAP entries covering fb_addr..fb_addr+size)

      unsafe { buf.submit(0); }
      Ok(())
  }
  ```

- [ ] **SYSCALL 0x02: KBD_READ**
  - Read keyboard state bitmap from `INPUT_MAP[0]`
  - Store result in R0
  ```rust
  fn syscall_kbd_read(state: &mut CpuState) -> Result<(), SyscallError> {
      let kbd = unsafe {
          INPUT_MAP.get(0).ok_or(SyscallError::InputReadFailed)?
      };
      state.regs[0] = *kbd;
      Ok(())
  }
  ```

- [ ] **SYSCALL 0x03: TIMER_READ**
  - Read `bpf_ktime_get_ns()` → convert to milliseconds
  - Store result in R0 (lower 32 bits)
  ```rust
  fn syscall_timer_read(state: &mut CpuState) -> Result<(), SyscallError> {
      let ns = unsafe { bpf_ktime_get_ns() };
      state.regs[0] = (ns / 1_000_000) as u32; // milliseconds
      Ok(())
  }
  ```

- [ ] **SYSCALL 0x04: EXIT**
  - Set `halted = 1`
  - Emit final CPU state to ring buffer
  - Return exit code from R0

- [ ] **SYSCALL any other**: Trap with `SyscallError::InvalidSyscall`
  - WHITELIST only — never fall through to default

**🗡️ BLACKMAGE ATTACK VECTORS:**

| Vector | Test | Severity |
|--------|------|----------|
| SCREEN_WRITE OOB | `fb_addr + width*height > RAM_SIZE` → bounds check | CRITICAL |
| SCREEN_WRITE huge dimensions | `width=0xFFFF, height=0xFFFF` → overflow in size calc | CRITICAL |
| Ring buffer exhaustion | Emit faster than drain → SCREEN_BUF full → graceful drop | MEDIUM |
| KBD_READ injection (D3) | Userspace writes to INPUT_MAP → controls game | HIGH |
| TIMER_READ wraparound | After 49 days, ms counter wraps u32 | LOW |
| Invalid SYSCALL number (D5) | `SYSCALL 0xFF` → must return error, not dispatch wildly | HIGH |
| SYSCALL during interrupt | Nested SYSCALL (SYSCALL calls CALL calls SYSCALL) | MEDIUM |

---

## PHASE 3: XDP INTEGRATION (Day 2, Afternoon)

### Task 3.1: Wire Fetch-Decode-Execute into XDP Handler

**File**: `crates/monad-cpu-ebpf/src/main.rs`
**Effort**: L (2.5 hours)

**Implementation checklist:**

- [ ] Parse IPv6 header, extract flow label
- [ ] Check if flow label matches Doom CPU flow (sentinel value or map lookup)
- [ ] Enter fetch-decode-execute loop with cycle budget
- [ ] Save state back to `CPU_STATE_MAP` after budget exhausted or HALT
- [ ] Update `DOOM_STATS` counters
- [ ] Return `XDP_PASS` (don't drop the packet — it still carries regular traffic)

```rust
#[xdp]
pub fn monad_cpu(ctx: XdpContext) -> u32 {
    match try_monad_cpu(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_monad_cpu(ctx: &XdpContext) -> Result<u32, ()> {
    // Parse IPv6 header
    let flow_label = parse_ipv6_flow_label(ctx)?;

    // Check if this is a Doom CPU packet
    let state = match unsafe { CPU_STATE_MAP.get_ptr_mut(&flow_label) } {
        Some(s) => s,
        None => return Ok(xdp_action::XDP_PASS), // Not a Doom flow
    };

    if state.halted != 0 {
        increment_stat(STAT_HALTED_PACKETS);
        return Ok(xdp_action::XDP_PASS);
    }

    // Execute cycle budget
    let mut cycles = 0u32;
    while cycles < MAX_CYCLES_PER_PACKET {
        // FETCH
        let pc = state.pc;
        if pc >= ROM_MAX_ENTRIES {
            state.halted = 1;
            break;
        }
        let raw_instr = match unsafe { ROM_MAP.get(pc) } {
            Some(v) => *v,
            None => { state.halted = 1; break; }
        };

        // DECODE
        let (opcode, dst, src, imm) = decode(raw_instr);

        // EXECUTE
        match execute(state, opcode, dst, src, imm) {
            Ok(()) => {},
            Err(ExecError::Halted) => break,
            Err(ExecError::Syscall(n)) => {
                dispatch_syscall(state, n)?;
                state.pc += 1;
            },
            Err(_) => {
                state.halted = 1;
                increment_stat(STAT_EXEC_ERRORS);
                break;
            }
        }

        cycles += 1;
        state.cycle_count += 1;
    }

    increment_stat(STAT_CYCLES_TOTAL);
    Ok(xdp_action::XDP_PASS)
}
```

**🗡️ BLACKMAGE ATTACK VECTORS:**

| Vector | Test | Severity |
|--------|------|----------|
| BPF verifier rejection | Total instruction count must stay under 1M verified | CRITICAL |
| Infinite loop in execute | Backward jump + no cycle budget = kernel hang | CRITICAL |
| get_ptr_mut concurrent access (S20) | Two packets same flow → TOCTOU on CpuState | CRITICAL |
| XDP latency impact | CPU loop must not block packet processing > 100µs | HIGH |
| Stack depth | Nested function calls in XDP must stay under BPF stack limit (512 bytes) | CRITICAL |
| Tail call budget | If using tail calls, 33 max (kernel limit) | HIGH |

**⚠️ BPF VERIFIER STRATEGY:**

The verifier is the #1 risk. Strategies to keep the program verifiable:

1. **All functions `#[inline(always)]`** — no indirect calls
2. **Explicit bounds checks before EVERY map access** — verifier must see them
3. **Loop with bounded iteration** — `cycles < MAX_CYCLES_PER_PACKET` gives verifier a bound
4. **No dynamic dispatch** — match arms, not function pointers
5. **Minimal stack usage** — CpuState is 84 bytes, BPF stack is 512 bytes. Keep other locals small.
6. **Test with `bpf_linker`** — compile, verify, catch rejections early

---

### Task 3.2: Implement Userspace Loader (wotan-ctl integration)

**File**: `cmd/wotan-ctl/doom.go` (new subcommand) or `cmd/doom-loader/main.go`
**Effort**: M (2 hours)

**Implementation checklist:**

- [ ] `doom load <rom.bin>` subcommand:
  - Read ROM binary file
  - Validate: size ≤ 256KB (65536 × 4 bytes)
  - Compute HMAC-SHA256 of ROM contents (D6 mitigation)
  - Write instructions to ROM_MAP via `bpf_map_update_elem`
  - Initialize CPU_STATE_MAP entry for flow label
  - Initialize RAM_MAP to zeros
  - Print: "ROM loaded: {size} instructions, HMAC: {hash}"

- [ ] `doom status` subcommand:
  - Read CPU_STATE_MAP for active flows
  - Print: PC, cycle count, halted status, register dump

- [ ] `doom input <key_bitmap>` subcommand:
  - Write keyboard state to INPUT_MAP[0]
  - For integration: pipe from SDL2/ncurses keyboard handler

- [ ] `doom reset <flow_label>` subcommand:
  - Zero CPU state, reset PC to 0, clear halted flag

**🗡️ BLACKMAGE ATTACK VECTORS:**

| Vector | Test | Severity |
|--------|------|----------|
| ROM without signature (D6) | `doom load` must compute and store HMAC | HIGH |
| ROM TOCTOU (D6) | File read → HMAC check → map write. Verify no race window | MEDIUM |
| Oversized ROM | File > 256KB → must reject, not truncate silently | MEDIUM |
| Map permission check | `doom load` must verify it has CAP_BPF or equivalent | HIGH |
| Malformed ROM file | Non-multiple-of-4 bytes → reject | MEDIUM |

---

## PHASE 4: TESTING & ADVERSARIAL VALIDATION (Day 2, Evening → Day 3)

### Task 4.1: Unit Tests for MBC Instruction Set

**File**: `crates/monad-mbc/src/tests.rs` (new)
**Effort**: L (3 hours)

**Implementation checklist:**

- [ ] Test every opcode with normal values
- [ ] Test every opcode with boundary values (0, MAX, overflow)
- [ ] Test division by zero (DIV, MOD)
- [ ] Test shift amounts (0, 1, 31, 32)
- [ ] Test memory access OOB (address at boundary, beyond boundary)
- [ ] Test stack overflow (PUSH at SP=0)
- [ ] Test stack underflow (POP at SP=RAM_SIZE)
- [ ] Test backward jump to self (must not infinite loop with budget)
- [ ] Test CALL/RET nesting to max depth
- [ ] Test SYSCALL with every valid number
- [ ] Test SYSCALL with invalid number
- [ ] Test HALT sets halted flag
- [ ] Round-trip: encode→decode for every opcode
- [ ] Property test: `decode(encode(any_valid_instruction)) == instruction`

---

### Task 4.2: Wire LICH-007 Campaign (MBC Bytecode Fuzzing)

**File**: `ebpf/fuzz/fuzz_targets/fuzz_mbc_decode.rs` (new)
**Effort**: M (1.5 hours)

**Implementation checklist:**

- [ ] Fuzz target: `fuzz_mbc_decode` — random bytes → decode → must not panic
- [ ] Fuzz target: `fuzz_mbc_execute` — random instruction stream → execute with cycle budget → must not panic
- [ ] Fuzz target: `fuzz_mbc_roundtrip` — random valid instructions → encode → decode → assert equal
- [ ] Structured fuzzing with `Arbitrary` derive on `Instruction` — generates valid-ish instructions
- [ ] Seed corpus: minimal programs (NOP+HALT, ADD+HALT, LOAD+STORE+HALT)
- [ ] Launch 72-hour campaign per lich-operations.md template

```rust
// fuzz_targets/fuzz_mbc_execute.rs
#![no_main]
use libfuzzer_sys::fuzz_target;
use arbitrary::Arbitrary;

#[derive(Arbitrary, Debug)]
struct FuzzProgram {
    instructions: Vec<u32>,  // Raw instruction words
    initial_regs: [u32; 16],
    initial_sp: u32,
}

fuzz_target!(|prog: FuzzProgram| {
    let mut state = CpuState::new();
    state.regs = prog.initial_regs;
    state.sp = prog.initial_sp.min(RAM_SIZE - 4);

    // Load program into mock ROM
    let rom: Vec<u32> = prog.instructions.iter()
        .take(1024)  // Max 1K instructions for fuzz speed
        .copied()
        .collect();

    // Execute with strict budget
    let mut cycles = 0;
    while cycles < 10_000 && !state.halted() {
        if let Err(_) = state.step(&rom) {
            break; // Errors are fine, panics are not
        }
        cycles += 1;
    }
    // If we get here without panic, the input is safe
});
```

**🗡️ BLACKMAGE SUCCESS CRITERIA:**

| Metric | Target |
|--------|--------|
| Coverage | >85% of instruction dispatch paths |
| Runtime | 72 hours minimum |
| Crashes | 0 after all fixes applied |
| Exec/sec | >5000 (Rust libfuzzer) |

---

### Task 4.3: Integration Test — Minimal Doom-like Program

**File**: `crates/monad-mbc/tests/integration.rs` or Go test
**Effort**: M (2 hours)

**Implementation checklist:**

- [ ] Write a minimal MBC program that:
  1. Loads immediate values into registers
  2. Performs arithmetic
  3. Writes to RAM
  4. Reads from RAM
  5. Calls a subroutine
  6. Returns from subroutine
  7. Uses SYSCALL to write to screen buffer
  8. HALTs

- [ ] Verify:
  - Final register state matches expected values
  - RAM contents match expected values
  - Screen buffer received exactly 1 frame
  - Cycle count matches instruction count
  - CPU halted flag is set

- [ ] Write a "counter" program: increment R0 every tick, SYSCALL screen when R0 % 160 == 0
  - Verifies the packet-as-clock-tick model works
  - Should emit ~35 frames/sec at 5600 packets/sec

---

### Task 4.4: BPF Verifier Compliance Test

**File**: build script or CI step
**Effort**: M (1.5 hours)

**Implementation checklist:**

- [ ] Compile monad-cpu-ebpf with `cargo +nightly build --target bpfel-unknown-none -Z build-std=core`
- [ ] Run through `bpf_linker` to produce final .o file
- [ ] Load into kernel with `bpftool prog load` on test machine
- [ ] Verify: program loads WITHOUT verifier rejection
- [ ] If rejected: read verifier log, identify the failing path, add bounds checks

**⚠️ VERIFIER REJECTION IS THE #1 RISK**

The BPF verifier will reject programs that:
- Have unbounded loops (our cycle budget must be a compile-time visible bound)
- Access maps without visible bounds checks in the same basic block
- Exceed stack depth (512 bytes)
- Have too many instructions (1M verified)
- Use disallowed helpers

**Failure recovery tiers:**

| Tier | Symptom | Fix |
|------|---------|-----|
| 1 | "infinite loop detected" | Make cycle budget `const` and visible to verifier |
| 2 | "R0 invalid mem access" | Add explicit `if idx < MAX` before every map access |
| 3 | "stack depth exceeded" | Reduce CpuState to essentials, inline more aggressively |
| 4 | "program too large" | Split into multiple programs with tail calls |
| 5 | "func#N not found" | Ensure all functions are `#[inline(always)]` |

---

## PHASE 5: DOOM ROM TOOLCHAIN (Day 3, Stretch Goal)

### Task 5.1: MBC Assembler

**File**: `tools/mbc-asm/main.go` or `tools/mbc-asm/src/main.rs`
**Effort**: M (2 hours, stretch)

- [ ] Simple assembler: text → binary MBC
  ```
  ; counter.mbc — minimal test ROM
  LOAD_IMM R0, 0       ; counter = 0
  loop:
    ADD_IMM R0, 1       ; counter++
    MOV R1, R0
    LOAD_IMM R2, 160
    MOD R1, R2          ; R1 = counter % 160
    CMP R1, R0          ; is it zero? (hack: compare with self for now)
    JNE loop
    SYSCALL 0x01        ; screen write
    JMP loop
  halt:
    HALT
  ```
- [ ] Label resolution (forward/backward references)
- [ ] Output: raw binary (4 bytes per instruction, little-endian)
- [ ] HMAC computation on output

### Task 5.2: Doom WAD → MBC Transpiler (FUTURE — Age 2+)

**Status**: NOT in S29 scope. This is the long-term goal — transpile actual Doom WAD data + engine logic into MBC. For S29, a hand-written test ROM proves the infrastructure.

---

## SPRINT SUMMARY

### Task Count by Phase

| Phase | Tasks | Effort | Day |
|-------|-------|--------|-----|
| 1: MBC ISA Definition | 3 tasks | ~4.5 hours | Day 1 AM |
| 2: Fetch-Decode-Execute | 6 tasks | ~13.5 hours | Day 1 PM → Day 2 AM |
| 3: XDP Integration | 2 tasks | ~4.5 hours | Day 2 PM |
| 4: Testing & Adversarial | 4 tasks | ~8 hours | Day 2 PM → Day 3 |
| 5: ROM Toolchain (stretch) | 1 task (+1 future) | ~2 hours | Day 3 |
| **TOTAL** | **16 tasks** | **~32.5 hours** | **~3 days** |

### Critical Path

```
Task 1.1 (ISA) → Task 1.2 (CPU State) → Task 1.3 (Maps)
                                              ↓
Task 2.1 (Fetch) → Task 2.2 (Decode) → Task 2.3 (ALU) → Task 2.4 (Memory)
                                              ↓
                                        Task 2.5 (Branch) → Task 2.6 (Syscall)
                                                                    ↓
                                                             Task 3.1 (XDP Wire)
                                                                    ↓
                                                    Task 3.2 (Loader) ← can parallel
                                                                    ↓
                                                Tasks 4.1-4.4 (Testing) ← can parallel
                                                                    ↓
                                                Task 5.1 (Assembler) ← stretch
```

### BlackMage Severity Summary

| Severity | Count | Top Risks |
|----------|-------|-----------|
| CRITICAL | 14 | BPF verifier rejection, div-by-zero in kernel, RAM shared state, get_ptr_mut TOCTOU, ROM injection |
| HIGH | 18 | Shift UB, stack overflow, flow collision, struct parity, input injection |
| MEDIUM | 8 | Timer wraparound, ring exhaustion, nested SYSCALL, encoding edge cases |
| LOW | 2 | HALT during CALL, timer precision |

### LICH Campaigns to Launch

| Campaign | Target | Duration | Priority |
|----------|--------|----------|----------|
| LICH-007 | MBC bytecode decode/execute | 72 hours | P0 (launch Day 2) |
| LICH-009 | Flow label collision | 24 hours | P1 (launch Day 3) |
| LICH-008 | L1 cache races | 48 hours | P2 (after multi-instance, Age 2) |

---

### EXIT CRITERIA

S29 is DONE when:

- [ ] All 32 MBC opcodes decode without panic on any input
- [ ] All ALU operations handle overflow/underflow with wrapping semantics
- [ ] Division by zero traps gracefully (not kernel panic)
- [ ] Memory access OOB returns error (not segfault)
- [ ] Cycle budget prevents infinite loops in XDP
- [ ] BPF verifier ACCEPTS the monad-cpu-ebpf program
- [ ] Minimal test ROM executes correctly (counter program)
- [ ] SCREEN_WRITE emits frame to ring buffer
- [ ] KBD_READ reads from INPUT_MAP
- [ ] LICH-007 running for 72 hours with 0 crashes
- [ ] All unit tests pass with race detection
- [ ] Go/Rust struct parity verified (84 bytes CpuState)
- [ ] wotan-ctl doom load/status/input/reset commands work

---

*Forged by the BlackMage — the Adversary who builds what it breaks.*
*Every opcode has an attack vector. Every map has a race condition.*
*Break everything. Fix everything. Ship Doom over IPv6.*

*"The Lich stirs. 32 opcodes. 14 critical vectors. Let's go."* 🗡️🔥
