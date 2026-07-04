# draft-bellis-unheaded-mbc-isa-00

**Status:** IETF Experimental · Independent Submission · March 2026

This document defines the MBC (Monad Bytecode) Instruction Set Architecture for the Unheaded Protocol Computer (UPC). MBC is a 48-opcode, 32-bit fixed-width instruction set designed for execution within eBPF XDP programs. It provides computational capabilities for distributed packet processing using the Monad wire format, enabling deterministic network packet classification and transformation at the network edge. MBC is optimized for execution within eBPF verifier constraints, including the 256-instruction limit per XDP invocation.

## Key Sections

- **Introduction** — UPC as a computational engine within eBPF XDP; primary use cases (packet classification, header rewriting, per-packet telemetry, policy enforcement); relationship to Foundation (Monad wire format), Wotan (message bus), and Sophia (dictionary service)
- **Instruction Encoding** — 32-bit fixed-width format: Opcode[31:24] | Destination[23:20] | Source[19:16] | Immediate[15:0]; little-endian storage; instruction encoding variants by family (arithmetic, extended immediate, memory, branch, atomic)
- **Register Model** — 16 general-purpose 32-bit registers (r0-r15); r0 = return value, r1-r5 = arguments/scratch, r6-r10 = callee-saved, r11-r14 = scratch, r15 = stack pointer; 8-bit flags register (IF, C, N, Z); 32-bit Program Counter; 128-byte CPU state structure
- **Instruction Set (48 opcodes by category):**
  - Arithmetic: ADD, SUB, MUL, DIV, MOD, NEG (0x01-0x06)
  - Logic/Bitwise: AND, OR, XOR, NOT, SHL, SHR, SAR (0x07-0x0D)
  - Data Movement: MOV, MOVI (0x0E-0x0F)
  - Comparison: CMP (0x10)
  - Extended Immediate: LOAD_IMM32, ADDI (0x1C-0x1D)
  - Interrupt: INT, IRET (0x17-0x18)
  - Stack: PUSH, POP (0x1A-0x1B)
  - Branch/Jump: JMP, JZ, JNZ, JN, JP, JC, JNC (0x20-0x26)
  - Call/Return: CALL, RET, JMPR, CALLR (0x27-0x2A)
  - Memory Load/Store: LD, ST, LDB, STB, LDH, STH (0x30-0x35)
  - Register Shifts and Extended Multiply: SHLR, SHRR, SARR, MULH, MULHU (0x36-0x3A)
  - Atomic: CLI, STI, XCHG, CAS (0x3B-0x3E)
  - System: SYSCALL (0x40), HALT (0xFF)
- **Memory Addressing** — Indexed mode only: effective address = base register + sign-extend(imm); no strict alignment requirement; address space via eBPF map keys validated by verifier
- **BPF Verifier Compliance** — 256-instruction limit per XDP invocation; bounded loop pattern requirement; map access null checks; memory safety guarantees inherited from eBPF execution model
- **Interrupt Model** — 256-entry IVT at addresses 0x000-0x3FF; interrupt handling sequence (save PC+1, clear IF, jump to IVT[vector]); standard vectors: 0x20 (Timer), 0x21 (Keyboard), 0x80 (Syscall), 0x00 (division by zero)
- **Safety Constraints** — Opcode whitelist (48 valid opcodes only); division-by-zero trap; stack bounds responsibility; flat privilege model (eBPF verifier is the trust boundary)
- **Security Considerations** — eBPF verifier as primary trust boundary; 256-instruction limit prevents infinite loops; no privilege separation; CAS atomicity limited to single-core; memory bounds enforced by eBPF maps; CRC validation required before MBC execution on Monad packets
- **IANA Considerations** — MBC Opcode Registry (0x00-0xFF, Standards Action); 48 initial assignments covering all defined opcodes

## Related

- [[Protocol Foundation|Protocol-Foundation]]
- [[Draft Protocol Foundation 06|Draft-Protocol-Foundation-06]]
- [[UPC Dream Ladder|UPC-Dream-Ladder]]
- [[Draft Shim 00|Draft-Shim-00]]
- [[Drafts Index|Drafts-Index]]

---

> **Source:** [docs/protocol/draft-bellis-unheaded-mbc-isa-00.md](../docs/protocol/draft-bellis-unheaded-mbc-isa-00.md)
