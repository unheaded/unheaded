# Phase 1.3 AP-3 — Trapframe / Context-Switch ABI Decision

**Status:** Decision recorded 2026-05-13. Pre-work for Phase 1.3 (process model).
**Parent:** `references/battle-plan-ascend-linux-2026-05-08.md` Phase 1.3.

## Substrate inventory

Three xv6 assembly adapters carry the M→S→U trap dance, already converted to
RV32 and stripped of s2-s11 callee-saves per task #60:

- **`crates/xv6-mbc/adapters/swtch_mbc.S`** — kernel-internal context switch.
  16-byte saved context (`ra`, `sp`, `s0`, `s1`), RV32 `sw`/`lw`. s2..s11
  stripped — the MBC translator does not register-allocate them.
- **`crates/xv6-mbc/adapters/trampoline_mbc.S`** — full user↔kernel trap entry
  and exit. Saves all general-purpose regs to the per-process trapframe,
  switches page tables via the memory-mapped CSR region, RV32-converted.
- **`crates/xv6-mbc/adapters/kernelvec_mbc.S`** — kernel-mode trap vector,
  128-byte stack frame for saved caller state during nested traps.

## UPC ABI v1 alignment

The behavior these adapters depend on is already a formal contract:

- **`docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md` Decision 1** specifies
  `SYSCALL`/`SRET`/`MRET` semantics — privilege transitions, EPC/STATUS
  manipulation, and reservation invalidation on context switch.
- **`docs/doom/UPC_BOOT_PROTOCOL_V2.md`** locks the memory-mapped CSR region
  (`0x000_F000+`) that the trampoline writes to drive `satp`, `sepc`,
  `sstatus`, `mstatus`, `mepc`, etc. The translator already lowers RV32 CSR
  reads/writes into MBC LD/ST against this region (per Phase 1.1 SHIP).

## Decision

**Keep xv6's existing trapframe pattern. No new ABI is introduced for
Phase 1.3.** Concretely:

1. The trampoline + kernelvec + swtch trio remains the authoritative
   user↔kernel boundary code. No replacement, no rewrite.
2. The translator already handles the CSR-region LD/ST sequences and the
   `MRET`/`SRET` opcodes (Phase 1.3 AP-2 shipped 2026-05-13, commit
   `73834054`); Phase 1.3 implementation does not need new opcodes or new
   translator branches.
3. The 16-byte `swtch` context and the per-pid trapframe layout (see AP-4
   amendment to `docs/doom/UPC_PAGE_TABLE_LAYOUT.md`) become the canonical
   reference for Phase 1.3 scheduler implementation.

## What this unblocks

Phase 1.3 IMPL can move straight to scheduler + syscall handler work
(`sys_execve`, `sys_waitpid`, `sys_exit`, `sys_sched_yield`) without touching
adapter assembly or the MBC translator. The ABI is frozen; the surface area is
the BPF-side syscall dispatch table and the xv6-side syscall stubs.

## What this defers

The "do we eventually replace xv6's trampoline with a UPC-native trapframe"
question is a Phase 3+ topic, gated on Wotan DISTRIBUTED-mode trap delivery
semantics and any future ISA v3 work. It is **not** Phase 1.3 scope.

---

Free to use. Free to share.
