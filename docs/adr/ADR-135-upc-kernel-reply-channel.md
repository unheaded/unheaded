<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-135: The owned kernel's reply channel to BPF

**Status**: Proposed (2026-11-04; implements DP-213, DP-501, DP-502). Reviewed the same day by the agent that wrote it, not by a separate one: see Review.
**Context sprint**: S-STAGE-2B (`docs/battle-plans/S-STAGE-2B-battle-plan.md`), Phases 10-13 and 17
**Relates to**: ADR-109 (kernel rows per BPF slot), ADR-110 (kernel time from BPF ticks), ADR-078 §5 (syscall number
registry), ADR-074 (per-pid page directories), ADR-075 (process model)

## Context

ADR-109 gave the kernel a row per BPF slot and a one-way path: BPF injects a trap and the kernel answers in a0. That
covers getpid and uptime. Every rung past them needs more:

- `sbrk` must move a break that BPF carries across fork and context switch.
- `kill` must end a process, and BPF owns process lifetime (I4).
- A call that cannot finish must block, and `sleep` and `sched` panic on a slot row (I4, I7).
- `mkdir`, `unlink`, `link` and later calls take user pointers, and `copyin`, `copyout` and `copyinstr` walk a
  pagetable that is BPF's pgd, opaque to the kernel (ADR-109 D5).
- FS calls need a cwd, and a refreshed row has `cwd = 0`.

ADR-109 D5 says the first rung that needs more than rows writes its own ADR. This is that ADR. It covers what the rungs
share; each rung still lands behind its own const with a measured verifier delta.

## Decision

### D1. Reply channel

Three new words in `struct upc_slot_tables`, after `ticks`:

| Offset | Word | Writer |
|---|---|---|
| +0x810 | `brk_in` | BPF, at injection |
| +0x814 | `reply` | kernel |
| +0x818 | `reply_arg` | kernel |
| +0x81C | pad | |

`UPC_ST_SIZE` becomes 0x820. The kernel writes `UPC_TAG_REPLY (0x5250_0000) | flags`. BPF reads `reply` in the SRET
arm when `spp == 0`, clears it, and acts. Flags: bit 0 SET_BRK, bit 1 YIELD, bit 2 KILL. One reply per trap, with
exactly one flag set: `reply_arg` has one meaning at a time. I7 holds: the same process, with U-mode restored first.

`upc_slot_enter()` zeroes `reply` and `reply_arg` where it zeroes `cur_slot` and `trap_a7`, before the tag check, so
no word written in U-mode survives into a trap's SRET. Only a kernel handler writes `reply`, and no U-mode code runs
between that write and the read.

### D2. Heap window

VA [0x10000, 0x40000) in every child, 192 KiB: above the CSR window (the last word is at 0xFFFC) and inside fork's low
copy band (`FORK_COPY_LOW_BYTES`, `phase12.rs`), so fork needs no change. A bigger heap needs a third copy band and its
verifier cost.

The window is bookkeeping, not an MMU boundary: a child's whole VA [0, 8 MiB) is mapped. It assumes a user image ends
below 0x10000. The largest in the corpus ends at 0x2084 (`sh`), and nothing checks this at link time.

### D3. Break authority

BPF keeps `cpu.program_break`, which fork and the context switch already carry, and publishes it as `brk_in` at each
injection. `sys_sbrk` reads `brk_in` and normalizes anything outside [base, top], top included, to the base. That
covers exec's `DEFAULT_PROGRAM_BREAK` (0x400000) with no change to the exec arm. Growth goes back through SET_BRK.

`p->sz` stays 0 on a slot row, which settles ADR-109 D5's "`sz` (sbrk rung)": the kernel does not mirror the break.
`growproc` and `vmfault` are never reached, and `fetchaddr` refuses, which only `sys_exec` would meet, and BPF serves
exec. Under `ascend-linux` the only readers of `program_break` are fork, the context switch and this word, so BPF
stores SET_BRK's argument unchecked.

### D4. Init has no heap

Slot 0 is identity-mapped, and physical 0x10000-0x40000 holds kernel `.rodata`. `sbrk` in slot 0 returns -1.

### D5. Zero on growth

The slice holds whatever fork copied, kernel `.rodata` included, or a dead process's bytes. The kernel zeroes the grown
range through the flat physical address. Lazy sbrk is treated as eager: no fault path exists.

This makes `malloc` correct. It is not a confidentiality control: see Security notes. The zeroing is a byte loop inside
one trap, and no other slot runs until it ends; a grow of the whole window is the worst case.

### D6. User pointers

ADR-109 D5 leaves this to the first rung that dereferences one. `walkaddr` and copyout's `walk` get a slot-row branch:
`pa = slice_base(s) + va` when `va < 0x800000`, else 0. The BPF pgd stays unwalked. The branch is valid only inside a
tagged trap (`upc_slot_active()`), so boot-time xv6 paths keep today's behaviour.

Inside a tagged trap the branch ignores its `pagetable` argument and uses `myproc()`. Every caller on a slot row passes
the caller's own table; kernel `kexec` would not, and BPF serves exec. Slot 0 takes the branch too, with base 0, so
`pa == 0` for VA 0 reads as refused. Slots 6 and 7 have slice bases at or above 56 MiB and never run:
`RAM_ISOLATION_CEILING` stops fork at slot 5.

### D7. Retry hand-off

The rule ADR-109 I7 sketches. A syscall that cannot finish leaves `epc` on the ecall, keeps a0, and replies YIELD. BPF
yields at SRET, and the ecall runs again when the slot is next scheduled. This is polling, about 1,350 MBC instructions
per retry. No `sleep()` on a slot row.

A retrying handler obeys four rules, because the ecall runs from the top each time:
- It decides before it changes anything. Progress lives in per-slot state that survives the retry and clears on row
  refresh, or the call returns short.
- It holds no lock and no log transaction when it replies.
- It undoes `usertrap`'s `epc += 4`.
- It returns the a0 it was given, because `syscall()` stores the return value there.

The a7 is safe across a retry: x17 shares r1 with gp, and `uservec` and `userret` save and restore gp. The ecall has no
spill prologue, and `pc_map` records every RV word, so `epc - 4` resolves to the SYSCALL word itself.

Until the fd rung's quiet trap return, every retry's SRET writes `S` and arms the 100-PC tracer, about 500 TTY bytes
against a 4 KiB ring. The sleep rung lands first, and its gate has to show its probe lines survive that.

### D8. Kill

The kernel validates the pid range (2..8; init refused) and replies KILL with the slot. It never sets `p->killed` on a
slot row and never walks `proc[]` for one. BPF checks liveness and writes a0 = -1 if the target is dead. Otherwise it
does what exit does: reparent, mark halted, and, if the target is the caller, exit. A target dies at once, not at its
next kernel entry; with cooperative scheduling that is the stricter answer. Status stays 0 until rung 7.

Live means `t = arg & 7`, `t != 0`, `t < num_processes`, and halted bit clear. Both `num_processes` and the halted bit
are needed: a new slot is not yet counted, and a reused slot is counted but keeps its halted and reaped bits until
fork finalizes. A zombie counts as dead and gets -1, where upstream xv6 returns 0.

**I9.** A kill never meets a fork copy in progress. Fork re-enters across ticks through `pc - 1`, but `ascend-linux`
has no timer switch, so the forking parent is the only process that runs from its first copy tick to its last, and it
never enters S-mode in between. A rung that adds preemption owns this: killing a parent mid-copy leaves the child's
pde 0 present, and the next fork on that slot then skips its first-entry path, including the cursor reset.

### D9. Row cwd

A refreshed row gets `namei("/")` once and keeps it, across slot reuse. `cwd` is not inherited until the fd rung, whose
fork note must drop the row's old reference when it installs the parent's.

## Guards

One const per step, off by default: `UPC_REPLY` for the channel, `UPC_RUNG_SBRK`, `UPC_RUNG_KILL`, `UPC_RUNG_SLEEP` for
the rungs. With a const off the object equals the previous object (I6 style). Each alias row carries
`cfg!(feature = "ascend-linux")`, because `kernel_routed_alias` runs in the Doom build too.

## Consequences

- `phase12.rs`, `upc_slots.h` and `parity_slot_tables.rs` move together. The table stays kernel `.bss`, published
  through BootParams `reserved[0]`; no BootParams field changes.
- Every rung past uptime uses one read in the SRET arm. Its verifier cost is measured on its own (plan Step 129). If it
  is over +3,000 inside the execute loop, the read moves beside the `DEFERRED_SWITCH` block.
- Rollback: the rung const off. With `UPC_REPLY` off nothing reads `reply`, and no kernel path reaches a reply unless a
  rung routes it.

## Security notes

- **A child can reach `reply` while ADR-109 D6 is off.** A store to VA `0xF0xxxx` writes its own pgd (`hostile_pgd`),
  and a PDE it adds maps kernel `.bss`. A forged word is wiped at trap entry (D1). Moving the table base at BootParams
  `0x160` gets past that: BPF then reads a reply the child wrote. That buys SET_BRK, YIELD and KILL of any live slot
  except init, and any child can already `kill` any other, so the channel adds no primitive. With D6 on, a child
  reaches only its own slice. Init can write everything, as it can today.
- **Every value BPF takes from the reply is bounded:** the tag is checked, KILL's target is masked and checked for
  liveness, and SET_BRK's value reaches only the kernel's own normalization (D3). `brk_in` is overwritten at each
  injection, so a forged one never reaches `sys_sbrk`.
- **Heap isolation between live processes is only as strong as D6.** With D6 off, a U-mode page-walk miss resolves to
  the physical address, so a child reads and writes another child's slice, heap included, at VA >= 8 MiB. This hole is
  pre-existing and stays open with D6.
- **Bytes between processes, with D6 on or off:**
  - Fork overwrites the whole low band, so a dead occupant's heap never reaches the next one. The child gets only its
    parent's bytes.
  - A child can read its band without calling `sbrk`. A child of init sees init's physical low memory there: BootParams,
    CSR values and kernel `.rodata`.
  - Above the band, nothing scrubs a slice at reap, so a slot's next occupant can read the last one's stack and argv
    frame.

  All three are pre-existing. D5 does not change them.

## Review

Plan Step 119 asks for a separate read-only review agent. The session that wrote this ADR could not start one, so the
same agent ran the brief read-only, and that is not independent. The review outcome and every disposition are in
`references/s-stage-2b-log.md`. A separate review is still owed before this ADR is Accepted.

## Alternatives considered

- **BPF serves sbrk itself.** One arm and no reply word, but kill and blocking still need a way back, so the channel
  arrives anyway one rung later, and the arm costs more than an alias row.
- **Kernel `sleep()` with a real context switch.** `sched()` from S-mode breaks I7 and moves scheduling authority to
  the kernel. That is rung 7's direction, not this ADR's.
- **A heap above the low band.** It needs a third fork copy band, and fork's copy loop is verifier-sensitive.
- **Walking BPF's pgd in the kernel.** Two formats for one table; each child's map is two superpages, so an offset
  gives the same answer.
