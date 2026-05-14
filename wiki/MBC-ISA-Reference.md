# MBC ISA Reference

The Monad Bytecode (MBC) instruction set — a custom 32-bit fixed-width ISA designed for fast eBPF dispatch and easy translation from RV32I. This is the bytecode the [UPC](UPC-Overview) executes.

## Encoding

```
 31     24 23 20 19 16 15            0
+---------+-----+-----+----------------+
| opcode  | dst | src |     imm16      |
+---------+-----+-----+----------------+
```

- 8-bit opcode (256 slots)
- 4-bit dst register (r0–r15)
- 4-bit src register
- 16-bit immediate (signed for branches and ADDI, unsigned for MOVI)

Two extended forms: 24-bit absolute (`CALL`, `JMP` family — uses bits 23..0 instead of dst+src+imm16) and 32-bit immediate (`LOAD_IMM32` — pairs with prior `MOVI` to set upper half).

## Implemented opcodes (summary)

| Class | Opcodes |
|---|---|
| Arithmetic | ADD, SUB, MUL, DIV, MOD, NEG, MULH, MULHU |
| Logic | AND, OR, XOR, NOT, SHL, SHR, SAR, SHLR, SHRR, SARR |
| Immediate | MOVI, ADDI, LOAD_IMM32 |
| Move / compare | MOV, CMP |
| Memory | LD, ST, LDB, STB, LDH, STH (all through `translate_address`) |
| Control flow (relative) | JMP, JZ, JNZ, JN, JP — 24-bit signed PC-relative |
| Control flow (absolute) | CALL (24-bit MBC PC), RET (jumps to r14, or rv2mbc[r14] if r14 ≥ 0x10000) |
| Control flow (indirect) | JMPR, CALLR — register-based with RV2MBC_MAP lookup |
| Privilege (gated on `ascend-linux`) | MRET (0x47), SRET (0x48) — reads MEPC/SEPC, translates via RV2MBC_MAP, sets `cpu.priv_level` |
| Atomics (gated on `ascend-linux`) | LR.W (0x49), SC.W (0x4A) |
| Sync (gated on `ascend-linux`) | FENCE (0x3F) |
| System | SYSCALL (0x40), INT 0x80 (Linux compat), HALT (0xFF), CLI (0x..), STI |
| Stack | PUSH, POP |

The five `ascend-linux`-gated opcodes (FENCE, MRET, SRET, LR.W, SC.W) short-circuit to no-ops when the feature flag is off, so the Doom build stays under the kernel 6.17 BPF verifier budget.

## Register file

16 general-purpose registers (r0–r15). Convention:

| MBC reg | RV alias | Notes |
|---|---|---|
| r0 | x0 (zero) | Conventionally zero, but **not pinned** — `MOVI r0, 0` is emitted before any use that assumes zero |
| r1 | x3 (gp), x17 (a7 spill) | Spill-shadowed: x17 backs the syscall_nr at ecall time |
| r2 | x4 (tp), x16 (a6 spill) | Spill-shadowed |
| r3–r5 | x5–x7 (t0–t2) | |
| r6–r7 | x8–x9 (s0–s1) | Callee-saved |
| r8–r13 | x10–x15 (a0–a5) | Argument registers |
| r14 | x1 (ra) | Link register |
| r15 | x2 (sp) | Stack pointer |

The `-ffixed-x18 ... -ffixed-x31` GCC pin keeps the compiler off the upper RV registers. The x16/x17 spill-shadow uses fixed RAM addresses byte 0x64000 / 0x64004 — see [UPC Overview](UPC-Overview) for the layout and [Linux on UPC](Linux-on-UPC) for the Phase 1.6 spill-shadow bug.

## Translator

The rv32i-to-mbc translator (`crates/monad-mbc/src/bin/rv32i_to_mbc.rs`) consumes RV32I ELF and emits three sidecars:

| Sidecar | Contents | Consumed by |
|---|---|---|
| `.mbc` | Flat u32 LE MBC instructions | Loaded into ROM_MAP by `upc-bootctl` |
| `.rv2mbc` | RV word offset → MBC PC table | Loaded into RV2MBC_MAP; consumed at runtime by JMPR / CALLR / MRET / SRET |
| `.data` | TLV-format dump of `.rodata` + `.data` sections at their link-time byte addresses | Loaded into RAM_MAP by `upc-bootctl` |

## Stats

Doom-on-Monad: 559 frames rendered, 819 M+ instructions executed, zero halts, zero ROM faults at the L3 milestone.

ASCEND-LINUX Phase 1.5: ~5.35 M instructions to reach the user-mode SRET transition (xv6 init's main runs through open / dup×2 / printf / vprintf to the write ecall stub). 41 CALL targets patched at load time for the userland binary.

## Cross-references

- [UPC Overview](UPC-Overview) — substrate context
- [Linux on UPC](Linux-on-UPC) — current frontier program in MBC
- [Doom on UPC](Doom-on-UPC) — L3 proof in MBC
- ADR-067 — ISA v2 + UPC ABI v1
- [`docs/protocol/draft-bellis-unheaded-mbc-isa-00.md`](../docs/protocol/draft-bellis-unheaded-mbc-isa-00.md) — full Internet-Draft

---

> **Source:** [docs/protocol/draft-bellis-unheaded-mbc-isa-00.md](../docs/protocol/draft-bellis-unheaded-mbc-isa-00.md) · [ebpf/monad-cpu-ebpf/src/main.rs](../ebpf/monad-cpu-ebpf/src/main.rs) · [crates/monad-mbc/](../crates/monad-mbc/)
