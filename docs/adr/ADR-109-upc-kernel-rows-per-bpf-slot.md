<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-109: The owned kernel gets a process row per BPF slot

**Status**: Proposed (2026-10-28)
**Context sprint**: EVOLUTION-1 Stage 2, rung 0 (`docs/battle-plans/EVOLUTION-1-STAGE-2-battle-plan.md`)
**Relates to**: ADR-067 (MBC ISA v2, UPC ABI v1), ADR-074 (per-pid page directories), ADR-075 (process model),
ADR-080 (programmatic API), EVOLUTION-1 (`docs/battle-plans/EVOLUTION-1-TRAP-FLIP.md`)

## Context

User `fork`, `exit` and `wait` are serviced in BPF. BPF numbers processes by slot (8 slots, `PROC_TABLE`) and reports
pid = slot + 1 (S-ASCEND-2 E1, option K). The owned xv6 kernel has one process row, init's: only `userinit` calls
`allocproc`. With `GETPID_INJECT` on, a syscall from a BPF-forked child traps into the kernel through the stock
`uservec` / `usertrap` path, and it gets init's identity. The Stage 2 Phase 1 probe (`references/evolution1-stage2-log.md`,
"-1 explained") showed the exact path:

```
UT:idx=0;pid=1;tf=8118272;ss=8118272;a7=11;sc=8;      parent getpid
SC:num=11;fn;ret=01;
UT:idx=0;pid=1;tf=8118272;ss=8118272;a7=3;sc=8;       child getpid
SC:num=03;fn;ret=-01;
```

1. `uservec` saves the child's registers at `sscratch`, a single CSR in the shared CSR window. The kernel last wrote
   it in `usertrapret` with init's trapframe.
2. `usertrap` resolves `myproc()` to `mycpu()->proc`, which is init's row.
3. The syscall number is wrong. x17 (a7) does not exist under ilp32e; the translator spills it to RAM byte `0x64004`
   (x16 to `0x64000`). User code reaches that address through its own per-pid slice. `uservec` runs in flat S-mode, so
   its `sw a7` reads physical `0x64004`. That is init's copy, which still holds 3 from the parent's `wait()`. The
   kernel runs `wait` for init, which has no kernel children, and returns -1.

Every rung of Stage 2 past getpid, and every arrival (`sbrk`, `pipe`, `kill`, ...), runs in a BPF-forked child. So
the kernel needs a correct identity and correct syscall arguments for a trap from any slot.

## Decision

BPF stays the authority over scheduling and process lifetime. The kernel keeps one row per BPF slot, which it
creates or refreshes when that slot traps.

### D1. Slot tables live in kernel memory; BootParams carries their address

The kernel allocates the tables in its own `.bss` (`struct upc_slot_tables`, below) and, in `upc_slots_init()` at
boot, writes their physical address into BootParams `reserved[0]` (RAM byte `0x160`; BootParams v3 is at `0x100` and
`reserved` starts at byte offset 96). BPF reads that word; **0 means "not published" and BPF does nothing new**. The
loader writes 0 there, so an object and kernel without this change behave as before.

```c
struct upc_slot_tables {          // 76 bytes, word-aligned
  uint32 tf[8];                   //  +0   physical address of slot s's trapframe (kernel writes once)
  uint32 cur_slot;                //  +32  slot now running in user mode (BPF writes on every switch)
  uint32 gen[8];                  //  +36  per-slot generation, BPF increments on every successful fork
  uint32 trap_a7;                 //  +68  the trapping slot's x17 (BPF writes at trap injection)
  uint32 trap_a6;                 //  +72  the trapping slot's x16 (BPF writes at trap injection)
};
```

A fixed address was considered and rejected. The low page is not provably free: the CSR window reaches past
`0xF3FF` (`mscratch` is at `0xFD00`), the TTY MMIO is at `0xC001`, and the layout documents disagree. Kernel `.bss` is
memory the kernel already owns, and BootParams is already the agreed hand-off.

### D2. One trapframe per slot, preallocated and prefilled

`upc_slots_init()` allocates 8 trapframe pages (`kalloc`) and fills the fields `uservec` reads before C code runs:
`kernel_sp` (the slot row's kernel stack), `kernel_satp`, `kernel_trap` (`usertrap`), `kernel_hartid`. It writes
their addresses to `tf[s]`. Slot 0 is init's existing trapframe.

### D3. BPF points `sscratch` at the running slot's trapframe

In `scheduler_context_switch`, after choosing `next`: if the tables are published,
`sscratch = tf[next]` (the CSR word at `0xF000 + 4*0x140`) and `cur_slot = next`. Both addresses are a constant word
plus the published base, outside any loop.

### D4. BPF carries the spilled syscall registers

At trap injection (the `GETPID_INJECT` arm and every later rung's arm), BPF writes the slot's x17 and x16 shadow
values (`regs[1]`, `regs[2]`) into `trap_a7` and `trap_a6`. `uservec` stores those into the trapframe's a7 and a6
slots, instead of reading x17 and x16 through their spill addresses. On the return path, `userret` does not restore
a6 or a7 into the spill addresses. Under the RISC-V calling convention both are caller-saved across `ecall`, and the
syscall stubs do not rely on them after the call. That also stops a child's return from writing init's spill slots.

### D5. Rows are `proc[0..7]`, created or refreshed at trap entry

`usertrap` calls `upc_slot_enter()` before anything else. It reads `cur_slot = s` and `gen[s]`. If `proc[s]` is not
initialised, or its recorded generation differs, it (re)initialises the row:
- `pid = s + 1` (option K);
- `trapframe = tf[s]`;
- `pagetable` = slot s's page directory (ADR-074);
- `state = RUNNING`, `killed = 0`;
- `parent = 0`, until rung 7 makes the kernel aware of parenthood.

Then it sets `mycpu()->proc = &proc[s]`. Init stays `proc[0]`, created by `userinit` as today. Rows 8-63 stay unused
until rung 7 decides the slot count (Stage 2 escalation S2-E2).

The kernel never runs its own scheduler for slot rows. `state` is RUNNING only while that slot is in a trap. On trap
exit the row is left marked so the kernel scheduler never picks it. The kernel changes a slot row's state only to
mark it killed.

## Invariants (each checked by the rung 0 gate)

- **I1**: a trap from slot s saves into `tf[s]`.
- **I2**: inside any trap from slot s, `myproc()->pid == s + 1`.
- **I3**: after a fork reuses slot s, its first trap sees a freshly initialised row (new generation), never the
  previous owner's state.
- **I4**: BPF remains the scheduler and lifetime authority; the kernel scheduler never runs a slot row.
- **I5**: init's flow is unchanged: the 12 goldens stay SAME.
- **I6**: with `GETPID_INJECT` off, nothing reads the tables, and behaviour is unchanged.

## Consequences

- With the flag on, `getpidfork`, `getpidloop` (sequential forks reusing one slot) and `getpidmulti` (three
  concurrent children) must print the same lines as with the flag off: rung 0's gate.
- BPF cost (estimate): one load of the published base plus two stores per context switch; one increment per fork;
  two stores per injected trap. Constant addresses, no new loop. Expected ≤ +2,000 verified instructions on 921,036.
  Above +5,000, fall back to publishing only `cur_slot` and deriving the generation in the kernel.
- `uservec` and `userret` change in a few instructions (D4). They are adapter files (`trampoline_mbc.S`), not
  upstream xv6.
- Rollback: `GETPID_INJECT` off. The tables then have no reader, and an object without D3/D4 ignores them.

## Security notes

- Children cannot reach the tables. A child's user VAs map into its own slice at 16 MiB and up
  (`USER_CHILD_SLICE`), and the tables are in the kernel image's `.bss` in low memory.
- **Pre-existing, not changed here:** init (slot 0) runs with an identity page directory over the low 64 MiB
  (`populate_identity_pgd`, bootctl). Init's user code can therefore read and write all physical RAM: kernel memory,
  the CSR window and these tables alike. That is today's trust model for init, the image the loader itself supplies.
  Narrowing it is a separate change.
- The kernel now reads words BPF writes (`cur_slot`, `gen`, `trap_a7`, `trap_a6`) and must bound them. `cur_slot`
  must be < 8, or the trap panics with a clear message rather than indexing past `proc[7]`. `trap_a7` goes through
  the existing `handler_for` lookup, so an unknown number still yields -1.

## Alternatives considered

- **Kernel-side fork (BPF fork injects a trap and the kernel's own `fork()` runs).** This is rung 7's direction, not
  rung 0's. It moves lifetime authority, and it needs `ofile` and the page-table copy to move with it.
- **A slot→row table but one shared trapframe.** Rejected: two slots in the kernel at once (a future preemptive
  case) or a nested trap would overwrite each other's registers, and nothing would catch it.
- **Fixing x17 by remapping 0x64004 per slot in S-mode.** S-mode is flat by design (H5); translation there would
  change every kernel memory access.
