# MBC (Monad Bytecode) ISA Reference

**Source of truth:** `ebpf/monad-common/src/lib.rs` module `mbc_opcodes`
**Version:** S30 (February 2026)
**Encoding:** 32-bit fixed-width

---

## Instruction Encoding

```
[31..24]  opcode   (8 bits)
[23..20]  dst      (4 bits) — destination register r0-r15
[19..16]  src      (4 bits) — source register r0-r15
[15..0]   imm16    (16 bits) — immediate / address / branch offset
```

For branch instructions (JMP, JZ, JNZ, JN, JP, JC, JNC), the lower 24 bits
(dst + src + imm16) are treated as a single signed 24-bit PC-relative offset.

For CALL, the lower 24 bits are an absolute MBC PC target.

---

## Registers

| Register | Alias | Purpose |
|----------|-------|---------|
| r0       | zero  | General purpose (RV32I: holds zero / scratch) |
| r1-r13   |       | General purpose |
| r14      | ra    | Return address (RV32I x1 mapping) |
| r15      | sp    | Stack pointer (RV32I x2 mapping) |

**Flags register** (separate from r0-r15):
- Bit 0: **Z** (Zero) -- set when ALU result is zero
- Bit 1: **N** (Negative) -- set when MSB of ALU result is 1
- Bit 2: **C** (Carry) -- set on unsigned overflow/borrow

---

## Opcode Table

### No-op (0x00)

| Hex    | Mnemonic | Encoding         | Semantics |
|--------|----------|------------------|-----------|
| `0x00` | NOP      | `[00][--][--][----]` | No operation. PC advances, no side effects. |

### Arithmetic (0x01-0x06)

| Hex    | Mnemonic | Encoding             | Semantics |
|--------|----------|----------------------|-----------|
| `0x01` | ADD      | `[01][dst][src][----]` | `regs[dst] = regs[dst] + regs[src]`. Sets Z, N, C. |
| `0x02` | SUB      | `[02][dst][src][----]` | `regs[dst] = regs[dst] - regs[src]`. Sets Z, N, C. |
| `0x03` | MUL      | `[03][dst][src][----]` | `regs[dst] = regs[dst] * regs[src]`. Sets Z, N, C. |
| `0x04` | DIV      | `[04][dst][src][----]` | `regs[dst] = regs[dst] / regs[src]` (unsigned). Div-by-zero: dst=0xFFFFFFFF, C=1. |
| `0x05` | MOD      | `[05][dst][src][----]` | `regs[dst] = regs[dst] % regs[src]` (unsigned). Mod-by-zero: dst=0. |
| `0x06` | NEG      | `[06][dst][--][----]`  | `regs[dst] = -regs[dst]` (two's complement). Sets Z, N, C. |

### Logical / Bitwise (0x07-0x0D)

| Hex    | Mnemonic | Encoding             | Semantics |
|--------|----------|----------------------|-----------|
| `0x07` | AND      | `[07][dst][src][----]` | `regs[dst] = regs[dst] & regs[src]`. Sets Z, N. |
| `0x08` | OR       | `[08][dst][src][----]` | `regs[dst] = regs[dst] \| regs[src]`. Sets Z, N. |
| `0x09` | XOR      | `[09][dst][src][----]` | `regs[dst] = regs[dst] ^ regs[src]`. Sets Z, N. |
| `0x0A` | NOT      | `[0A][dst][--][----]`  | `regs[dst] = ~regs[dst]`. Sets Z, N. |
| `0x0B` | SHL      | `[0B][dst][--][imm16]` | `regs[dst] = regs[dst] << (imm16 & 0xFF)`. Sets Z, N, C. |
| `0x0C` | SHR      | `[0C][dst][--][imm16]` | `regs[dst] = regs[dst] >> (imm16 & 0xFF)` (logical). Sets Z, N, C. |
| `0x0D` | SAR      | `[0D][dst][--][imm16]` | `regs[dst] = regs[dst] >> (imm16 & 0xFF)` (arithmetic, sign-extending). Sets Z, N, C. |

### Register Operations (0x0E-0x10)

| Hex    | Mnemonic | Encoding             | Semantics |
|--------|----------|----------------------|-----------|
| `0x0E` | MOV      | `[0E][dst][src][----]` | `regs[dst] = regs[src]`. |
| `0x0F` | MOVI     | `[0F][dst][--][imm16]` | `regs[dst] = zero_extend(imm16)`. |
| `0x10` | CMP      | `[10][dst][src][----]` | `flags = cmp(regs[dst], regs[src])`. Sets Z, N, C without storing result. |

### Stack Operations (0x1A-0x1B)

| Hex    | Mnemonic | Encoding             | Semantics |
|--------|----------|----------------------|-----------|
| `0x1A` | PUSH     | `[1A][dst][--][----]`  | `SP -= 1; ram[SP] = regs[dst]`. Full-descending stack. |
| `0x1B` | POP      | `[1B][dst][--][----]`  | `regs[dst] = ram[SP]; SP += 1`. Full-descending stack. |

### Extended Immediate (0x1C-0x1D)

| Hex    | Mnemonic   | Encoding             | Semantics |
|--------|------------|----------------------|-----------|
| `0x1C` | LOAD_IMM32 | `[1C][dst][--][imm16]` | `regs[dst][31:16] = imm16` (sets upper 16 bits, preserves lower 16). Use with MOVI for full 32-bit loads. |
| `0x1D` | ADDI       | `[1D][dst][--][imm16]` | `regs[dst] = regs[dst] + sign_extend(imm16)`. Sets Z, N, C. |

### Branch (0x20-0x26)

All branch instructions use 24-bit signed PC-relative offsets.
Offset is relative to PC *after* the fetch increment (i.e., `new_pc = pc + 1 + offset`).

| Hex    | Mnemonic | Encoding             | Semantics |
|--------|----------|----------------------|-----------|
| `0x20` | JMP      | `[20][off24]`         | `pc += offset` (unconditional). |
| `0x21` | JZ       | `[21][off24]`         | `if Z: pc += offset`. |
| `0x22` | JNZ      | `[22][off24]`         | `if !Z: pc += offset`. |
| `0x23` | JN       | `[23][off24]`         | `if N: pc += offset`. |
| `0x24` | JP       | `[24][off24]`         | `if !N && !Z: pc += offset` (positive, non-zero). |
| `0x25` | JC       | `[25][off24]`         | `if C: pc += offset`. |
| `0x26` | JNC      | `[26][off24]`         | `if !C: pc += offset`. |

### Call / Return (0x27-0x2A)

| Hex    | Mnemonic | Encoding             | Semantics |
|--------|----------|----------------------|-----------|
| `0x27` | CALL     | `[27][target24]`      | `push(PC); pc = target24` (24-bit absolute). |
| `0x28` | RET      | `[28][--][--][----]`   | `pc = pop()`. |
| `0x29` | JMPR     | `[29][dst][--][----]`  | `pc = regs[dst]` (indirect jump). In BPF: RV2MBC_MAP translation. |
| `0x2A` | CALLR    | `[2A][dst][--][----]`  | `push(PC); pc = regs[dst]` (indirect call). In BPF: RV2MBC_MAP translation. |

### Memory (0x30-0x35)

All memory addresses are byte addresses. `imm16` is sign-extended to 32 bits.

| Hex    | Mnemonic | Encoding             | Semantics |
|--------|----------|----------------------|-----------|
| `0x30` | LD       | `[30][dst][src][imm16]` | `regs[dst] = ram[regs[src] + sext(imm16)]` (32-bit word load). |
| `0x31` | ST       | `[31][dst][src][imm16]` | `ram[regs[dst] + sext(imm16)] = regs[src]` (32-bit word store). |
| `0x32` | LDB      | `[32][dst][src][imm16]` | `regs[dst] = zero_extend(ram_byte[regs[src] + sext(imm16)])` (byte load). |
| `0x33` | STB      | `[33][dst][src][imm16]` | `ram_byte[regs[dst] + sext(imm16)] = regs[src] & 0xFF` (byte store). |
| `0x34` | LDH      | `[34][dst][src][imm16]` | `regs[dst] = zero_extend(ram_half[regs[src] + sext(imm16)])` (16-bit load). |
| `0x35` | STH      | `[35][dst][src][imm16]` | `ram_half[regs[dst] + sext(imm16)] = regs[src] & 0xFFFF` (16-bit store). |

### Register-based Shifts (0x36-0x39)

| Hex    | Mnemonic | Encoding             | Semantics |
|--------|----------|----------------------|-----------|
| `0x36` | SHLR     | `[36][dst][src][----]` | `regs[dst] = regs[dst] << (regs[src] & 31)`. Sets Z, N, C. |
| `0x37` | SHRR     | `[37][dst][src][----]` | `regs[dst] = regs[dst] >> (regs[src] & 31)` (logical). Sets Z, N, C. |
| `0x38` | SARR     | `[38][dst][src][----]` | `regs[dst] = regs[dst] >> (regs[src] & 31)` (arithmetic). Sets Z, N, C. |
| `0x39` | MULH     | `[39][dst][src][----]` | `regs[dst] = (i64(regs[dst]) * i64(regs[src])) >> 32` (signed upper multiply). Sets Z, N. |

### System (0x40, 0xFF)

| Hex    | Mnemonic | Encoding             | Semantics |
|--------|----------|----------------------|-----------|
| `0x40` | SYSCALL  | `[40][--][--][imm16]` | Invoke syscall. `regs[0]` = syscall number (see below). |
| `0xFF` | HALT     | `[FF][--][--][----]`  | Set `halted = 1`. Execution stops. |

---

## Syscall Numbers

Syscall number is in `regs[0]` at time of SYSCALL execution.

| Number | Name           | Semantics |
|--------|----------------|-----------|
| 0x01   | SYS_DRAW_FRAME | Copy pixel buffer from RAM to screen. `r1` = framebuffer address. |
| 0x02   | SYS_GET_KEY    | Read keyboard state. Returns: `r0` = scancode, `r1` = pressed (1/0). |
| 0x03   | SYS_GET_TICKS  | `r0 = bpf_ktime_get_ns() / 1_000_000` (milliseconds since boot). |
| 0x04   | SYS_SLEEP      | `sleep_until = now + r1 * 1_000_000` (sleep for r1 milliseconds). |

---

## Memory Map (Doom-over-IPv6 PoC)

| Address Range              | Backing       | Description |
|---------------------------|---------------|-------------|
| 0x0000_0000 - 0x0000_BFFF | RAM_MAP       | 48 KiB general RAM |
| 0x0000_C000 - 0x0000_F8BF | SCREEN_MAP    | 320x200 framebuffer (64,000 bytes) |
| 0x0000_FFFF                | KBD_MAP[0]    | Keyboard state word |
| 0x0001_0000 - 0x0040_FFFF | RAM_MAP       | WAD data (4 MiB) |

---

## Opcode Map Summary

```
0x00       NOP
0x01-0x06  Arithmetic:     ADD SUB MUL DIV MOD NEG
0x07-0x0D  Bitwise:        AND OR XOR NOT SHL SHR SAR
0x0E-0x10  Register/CMP:   MOV MOVI CMP
0x11-0x19  (reserved)
0x1A-0x1B  Stack:          PUSH POP
0x1C-0x1D  Extended imm:   LOAD_IMM32 ADDI
0x1E-0x1F  (reserved)
0x20-0x26  Branch:         JMP JZ JNZ JN JP JC JNC
0x27-0x2A  Call/Ret:       CALL RET JMPR CALLR
0x2B-0x2F  (reserved)
0x30-0x35  Memory:         LD ST LDB STB LDH STH
0x36-0x39  Reg shifts:     SHLR SHRR SARR MULH
0x3A-0x3F  (reserved)
0x40       System:         SYSCALL
0x41-0xFE  (reserved)
0xFF       System:         HALT
```

Total defined opcodes: **43**

---

## Flag Effects Summary

| Category | Opcodes | Z | N | C |
|----------|---------|---|---|---|
| Arithmetic | ADD, SUB, MUL, NEG, ADDI | yes | yes | yes (overflow) |
| Division | DIV | yes | yes | yes (div-by-zero) |
| Modulo | MOD | yes | yes | no |
| Bitwise | AND, OR, XOR, NOT | yes | yes | no |
| Shift (imm) | SHL, SHR, SAR | yes | yes | yes (last shifted bit) |
| Shift (reg) | SHLR, SHRR, SARR | yes | yes | yes (last shifted bit) |
| Multiply high | MULH | yes | yes | no |
| Compare | CMP | yes | yes | yes (borrow) |
| All others | MOV, MOVI, LOAD_IMM32, branches, memory, PUSH, POP, NOP, SYSCALL, HALT | no | no | no |
