# ADR-067: MBC ISA v2 + UPC ABI v1

**Status:** Accepted (Phase 0 complete 2026-05-08)
**Date:** 2026-05-08

> Wiki stub generated 2026-05-11. See the canonical ADR for full text + rationale.

## TL;DR

The MBC instruction set gains five new opcodes to support kernel-level workloads (xv6, uClinux, full Linux). MbcCpuState grows from 128 → 136 bytes to carry the privilege state these opcodes need.

## New opcodes (v2)

| Opcode | Mnemonic | Semantics |
|--------|----------|-----------|
| 0x3F | FENCE | Memory barrier (no-op on single-CPU MBC) |
| 0x47 | MRET | Machine-mode return — pop MEPC + restore priv from MSTATUS.MPP |
| 0x48 | SRET | Supervisor-mode return — pop SEPC + restore priv from SSTATUS.SPP |
| 0x49 | LR.W | Load-Reserved Word — `rd = RAM[rs1]; reserve rs1` |
| 0x4A | SC.W | Store-Conditional Word — atomic write iff reservation valid |

## CSR access (Decision 2)

CSRs are memory-mapped at byte address `0x000_F000 + csr_index * 4`. The translator emits CSR access (`csrrw`/`csrrs`/`csrrc`) as load/store against this region. Avoids growing the MBC ISA further.

## MbcCpuState (Decision 3)

| Old (128 B) | New (136 B) |
|-------------|-------------|
| regs[16] u32 + pc + flags + halted + ... | + `priv_level: u8` (M=0, S=1, U=3) + `reservation_address: u32` (LR/SC) + padding |

## Phase 0 verification gate

Closed at task #41. All 5 opcodes implemented in monad-mbc + monad-cpu-ebpf, tested via `os_primitives_test.rs`, BPF verifier still under budget. ABI v1 frozen.

## Canonical

[docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md](../docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md)

## Cross-references

- [ADR Index](ADR-Index.md)
- [ADR-072](ADR-072-boot-magic-byte-ordering.md) — BOOT_MAGIC byte-ordering
- [Architecture overview](Architecture.md)
- `docs/doom/UPC_BOOT_PROTOCOL_V2.md` — boot protocol that consumes these opcodes
- `references/battle-plan-ascend-linux-2026-05-08.md` §3 (Phase 0)
