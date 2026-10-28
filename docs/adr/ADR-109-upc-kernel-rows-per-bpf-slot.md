<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-109: The owned kernel gets a process row per BPF slot

**Status**: Proposed (2026-10-28; revised after the four-lens review the same day)
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

BPF stays the authority over scheduling and process lifetime. The kernel keeps one row per BPF slot, `proc[s]` for
slot s, permanently. It refreshes a row on that slot's first trap after a fork.

### D1. Slot tables live in kernel `.bss`; BootParams carries their address

The kernel allocates `struct upc_slot_tables` (below) in its own `.bss`. After `userinit()` (D2), `upc_slots_init()`
writes the table's physical address into BootParams `reserved[0]`: RAM byte `0x160`, the first word after v3's
`rv2mbc_sha256` (bytes 64..96 of BootParams at `0x100`). The loader writes 0 there. **0 means "not published", and
BPF does nothing new.** `docs/doom/UPC_BOOT_PROTOCOL_V2.md` §v3 records the field. Using a reserved word is the
compatible extension that document allows, and this is the first field the kernel writes rather than the loader.

```c
struct upc_tf_slot { struct trapframe tf; uint8 pad[256 - sizeof(struct trapframe)]; };  // 144 + 112

struct upc_slot_tables {          // 2,064 bytes, 256-byte aligned
  struct upc_tf_slot tf[8];       // +0x000  slot s's trapframe at +s*256 (kernel prefills, D2)
  uint32 cur_slot;                // +0x800  UPC_TAG_SLOT | s, BPF writes at trap injection
  uint32 trap_a7;                 // +0x804  UPC_TAG_A7 | nr, BPF writes at trap injection
  uint32 fresh_mask;              // +0x808  BPF ORs 1 << child at fork; kernel clears bit s on refresh
  uint32 pad;                     // +0x80C
};
```

`UPC_TAG_SLOT` is `0x5107_0000` and `UPC_TAG_A7` is `0xA700_0000`; the low 16 bits carry the value.

A fixed low address was considered and rejected. The low page is not provably free: the CSR window reaches past
`0xF3FF` (`mscratch` is at `0xFD00`), the TTY MMIO is at `0xC001`, and the layout documents disagree. A fixed
trapframe stride means BPF computes `base + s*256` and never reads a table to find a trapframe.

### D2. One trapframe per slot, inside the table, prefilled

`umain.c` calls `upc_slots_init()` after `userinit()` and before the scheduler starts. For each slot s it fills the
four fields `uservec` reads before any C runs:
- `kernel_sp = proc[s].kstack + PGSIZE` (`procinit` gives every row a kernel stack);
- `kernel_trap = usertrap`;
- `kernel_hartid = 0`, a literal (tp is r2, which x16 shares);
- `kernel_satp`, as `prepare_return` writes it (BPF never reads it).

It copies init's trapframe into `tf[0]`, points `proc[0].trapframe` there, and frees the page `allocproc` gave it.
Only then does it publish the base. Prefill covers a child's first trap; from then on `prepare_return` refreshes the
four fields as it does today.

### D3. BPF writes at trap injection, behind the rung const

A single helper, `inject_strap()`, sits in the injection arm, just before the jump to STVEC. It runs only when the
rung const is on and the base word is non-zero:
- `sscratch = base + 256 * current_pid` (the CSR word at `0xF000 + 4*0x140`);
- `cur_slot = UPC_TAG_SLOT | current_pid`;
- `trap_a7 = UPC_TAG_A7 | regs[1]` (x17's shadow, the value BPF already dispatches on).

The fork arm ORs `1 << child` into `fresh_mask`, behind the same const. `scheduler_context_switch` is not touched. It
is inlined at in-loop call sites, so a variable RAM offset there is the pattern that once cost +119K verified
instructions. With the const off, the object is the pre-ADR object, and its verified count is equal.

BPF relies on `RAM_MAP` bounds for a hostile base: init can rewrite `0x160` (D6), and a wild base then reads or
writes inside RAM but nowhere else.

### D4. The kernel takes a7 from `trap_a7`; userret stops touching x16/x17

`upc_slot_enter()` (D5) sets `p->trapframe->a7` from `trap_a7` when the tag is present. `uservec` drops its a6/a7
save, and `userret` drops its a6/a7 restore. Under RV32E the compiler never allocates x16 or x17. The `usys` stubs
load a7 before each `ecall`, and nothing after the call reads it. BPF's wait retry re-dispatches from `regs[1]`, not
the spill. So a trap no longer reads or writes the spill words at physical `0x64000`/`0x64004` (init's copies), and r1
and r2 keep their pre-trap values. No handler reads a6 (`argraw` stops at a5), so nothing carries it.

### D5. Rows are refreshed at trap entry; the kernel never schedules or frees them

`usertrap` calls `upc_slot_enter()` first. It reads `cur_slot` and `trap_a7`, and writes both back as 0 at once.
- **No `UPC_TAG_SLOT` tag** (the const is off, or the object is old): it changes nothing. Today's path, `mycpu()->proc`
  and `tf->a7`, runs.
- **Tag present:** `s` = the low 16 bits. If `s >= 8`, it panics `upc: cur_slot`. It checks `r_sscratch() == &tf[s]`
  and panics `upc: sscratch` otherwise (I1 checked on every trap). If bit s of `fresh_mask` is set, or `proc[s]` was
  never initialised, it refreshes the row and clears the bit:
  - `pid = s + 1` (option K);
  - `trapframe = &tf[s].tf`;
  - `pagetable = 0x00F00000 + s*4096`, assigned directly, never through `uvmcreate`, `proc_pagetable`, `mappages` or
    `freeproc`. The pointer is opaque: BPF's two-level format (present at bit 11) is not xv6's, so kernel code must not
    walk it. The first rung that dereferences a user pointer (`copyin`/`copyout`, likely rung 3 or 6) owns the
    translation decision.
  - `state = RUNNING`, `killed = 0`, `parent = 0`, `sz = 0`.
- **Then, in every tagged case:** it sets `mycpu()->proc = &proc[s]`, and a7 from `trap_a7` if that tag is present.
  Without it, `tf->a7` stays as saved.

**Init.** Init stays `proc[0]`, created by `userinit`. Its `pagetable` is `uvmcreate(1)` = `0xF01000`, which is slot
1's BPF page directory; `uvmfirst` may have written xv6-format entries there, and BPF fork only rewrites PDE 0 and 1.
This is pre-existing. Rung 0 checks it (a word dump of `0xF01000` after boot) and leaves it as it is.

**Rows 0-7 belong to the slots.** `allocproc` skips rows 1-7. Before rung 7, a slot row never reaches `kexit`,
`sched`, `sleep` or `freeproc`; each panics `upc: slot row` if it does. `usertrap`'s `killed` exits panic the same
way. Rung 0 never sets `killed` on a slot row. How a kill reaches BPF is the kill rung's decision (skeleton rung 8).

**Fields left stale, and their owners:** `sz` (sbrk rung), `ofile`/`cwd` (rung 6), `xstate`/`parent` (rung 7), and
`chan` (the first rung that sleeps in the kernel).

**Rung 7 forward rule:** row identity is slot s ↔ `proc[s]`. When the kernel takes over fork, it uses the slot BPF
chose, because the page directory and memory slice are slot-indexed. `allocpid` gives way to slot + 1.

### D6. A user-mode page-walk miss stops returning the virtual address

Today `translate_address` returns the virtual address unchanged when the PDE or PTE is not present (`main.rs`,
"Page fault — return unmapped"). User code at priv 3 then reaches physical memory directly. A child that stores to VA
`0xF0xxxx` writes the per-pid page directories, and from there it can map anything: kernel `.bss`, these tables,
BootParams, the CSR window. With priv 3 and a non-zero `page_dir_base`, a miss returns `0xFFFF_FFFC`, which is
outside `RAM_MAP`, so loads read 0 and stores drop. S-mode and the Doom path are unchanged. This is a pre-existing
hole, not one this ADR opens, but the tables need it closed. Rung 0 measures the cost, adds a hostile-child probe, and
keeps the 12 goldens SAME. If a golden changes, the change is recorded and D6 escalates to Stevie on its own.

## Invariants (each checked by the rung 0 gate)

- **I1**: a trap from slot s saves into `tf[s]` (kernel assert on every trap, D5).
- **I2**: inside any trap from slot s, `myproc()->pid == s + 1`.
- **I3**: after a fork reuses slot s, the first trap from s refreshes the row; later traps from the same child do not.
- **I4**: BPF is the scheduler and lifetime authority; a slot row never reaches `kexit`, `sched`, `sleep` or `freeproc`
  (panic guards).
- **I5**: init's flow is unchanged: the 12 goldens stay SAME.
- **I6**: with the rung const off, the object is unchanged (equal verified count) and the kernel takes today's path
  (no tag).
- **I7**: BPF never context-switches while `priv_level != 3`. Today this holds because the timer switch is compiled out
  under `ascend-linux`, and the exit, wait and pause arms run on a user `ecall`. A guard makes it explicit:
  `scheduler_context_switch` returns early in S-mode. Kernel-side blocking, when a rung needs it, hands back to BPF
  by re-injecting on resume, never through `sched()`.
- **I8**: a trap does not change physical `0x64000`/`0x64004` (D4).

## Consequences

- Rung 0's gate. The plan (`EVOLUTION-1-STAGE-2-battle-plan.md` Phases 3-4) has the commands. With the const on:
  - `getpidfork`: the child's pid equals `fork()`'s return.
  - `getpidloop`: three sequential forks reuse a slot, each child refreshes once (I3).
  - `getpidmulti`: three concurrent children interleave traps through `pause` (I1 across slots).
  - The probe build prints `UT:s;pid;ss;a7;rf` per trap and the gate asserts every field: `ss` distinct per slot and
    equal to `&tf[s]`, `pid = s + 1`, `rf = 1` exactly on the first trap after each fork, `a7` equal to the user's
    number.
  - A second injected number in the probe build (22) shows a7 reaching the kernel from a child whose last syscall was
    different.
  - **Mutation check:** the gate must fail on each broken variant: one shared trapframe; `trap_a7` ignored; rows never
    refreshed; userret still restoring a6/a7.
  - Plus `gate2`, `gate_nway`, forkloop, and the 12 goldens SAME.

  ON and OFF printing the same user lines is necessary, not sufficient: both derive from BPF's `current_pid`.
- **BPF cost:** measured, not estimated. The bar is ≤ +2,000 verified instructions over the const-off count with D3,
  plus whatever D6 costs; D6 is measured separately. Above +5,000 for D3, the fallback is `cur_slot` only, with the
  generation derived in the kernel from a BPF exit count, which needs its own amendment.
- `uservec` and `userret` lose four instructions (D4). They are adapter files (`trampoline_mbc.S`), not upstream xv6.
- **Rollback:** the rung const off. The tables then have no writer, and the kernel takes today's path.
- **Skeleton amendment:** rung 0's "exit and wait free the row" becomes "a reused slot refreshes its row on first
  trap". The kernel frees nothing before rung 7.

## Security notes

- **Who can reach the tables:** before D6, any child. A user-mode page-walk miss falls through to physical memory (D6).
  After D6, a child's VAs reach only its own slice at 16 MiB and up (`USER_CHILD_SLICE`), and the tables are in the
  kernel image's `.bss` in low memory. Rung 0's hostile-child probe stores to VA `0xF00000` and to the table's address,
  then checks from init that neither changed.
- **Pre-existing, not changed here:** init (slot 0) runs with an identity page directory over the low 64 MiB
  (`populate_identity_pgd`, bootctl). Init's user code can read and write all physical RAM, including kernel memory,
  the CSR window, these tables and BootParams `0x160`. That is today's trust model for init, the image the loader
  supplies. Narrowing it is a separate change.
- **Every value the kernel takes from BPF-written memory is bounded:**
  - `cur_slot`: tag check, `< 8`, and `sscratch == &tf[s]`.
  - `fresh_mask`: only bit s is read, so any value is safe.
  - `trap_a7`: tag check; the 16-bit value goes through `handler_for`, so an unknown number yields -1.
  - Before BPF's first write, `.bss` zero means no tag, so the kernel takes today's path.
  - Both tagged words are cleared on read, so a stale value cannot be reused by the next trap.
- **Concurrency:** one CPU and one XDP invocation per instance at a time (one veth queue). BPF writes the tables only
  in user mode, and the kernel reads them only in S-mode (I7). Neither can interleave with the other. The unexplained
  half-tick loss (`references/evolution1-stage2-log.md`, side findings) does not change this: lost ticks are lost
  whole.

## Alternatives considered

- **Kernel-side fork (BPF fork injects a trap and the kernel's own `fork()` runs).** This is rung 7's direction, not
  rung 0's. It moves lifetime authority, and `ofile` and the page-table copy have to move with it.
- **One shared trapframe with a slot→row table.** Rejected. Two slots in the kernel at once (a future preemptive or
  blocking case) or a nested trap would overwrite each other's registers, and nothing would catch it.
- **BPF writes in `scheduler_context_switch` (the first draft of D3).** Rejected after review. It is inlined at
  in-loop call sites and runs on every switch, not only on a trap. It also writes when the const is off.
- **A per-slot generation array.** Replaced by `fresh_mask`. There is no spare word in a `PROC_TABLE` row (22 words,
  slots 20-21 in use), an array index is a variable offset, and the kernel only needs "first trap since fork".
- **Fixing x17 by remapping `0x64004` per slot in S-mode.** S-mode is flat by design (H5), and translation there would
  change every kernel memory access.
