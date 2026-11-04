<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-135: The owned kernel's reply channel to BPF

**Status**: Proposed (2026-11-04; implements DP-213, DP-501, DP-502). Independent read-only review done the same day, no blocker against D1: see Review. Acceptance is Stevie's.
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
at most one flag set: `reply_arg` has one meaning at a time. That is a rule on the kernel's one writer, `upc_reply`. BPF
does not enforce it: the block tests each flag on its own, so a forged word with two flags gets both on one `reply_arg`.
Every value stays bounded, so that is harmless today. I7 holds: the same process, with U-mode restored first.

`upc_slot_enter()` zeroes `reply` and `reply_arg` where it zeroes `cur_slot` and `trap_a7`, before the tag check, so
no word written in U-mode survives a trap that reaches `upc_slot_enter()`. Only a kernel handler writes `reply`, and no
U-mode code runs between that write and the read. Both statements cover the normal trap path. A child that can reach
the table has ways around them: see Security notes.

A write that runs after the handler must not replace the handler's reply. The plan's D14 (Step 220) has `prepare_return`
store a bare `UPC_TAG_REPLY`, which would wipe SET_BRK, YIELD or KILL, and Step 124's `upc_slot_leave()` clears
`upc_active` before `prepare_return`, so that code cannot tell a tagged trap. When Step 220 writes D14 here, the mark
goes in `upc_slot_leave()`, before it clears `upc_active`, and stores the bare tag only if `reply` is 0.

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
`growproc` is never reached. `vmfault` is: `copyin` and `copyout` call it on every refused pointer (`uvm.c`), and it
refuses only because `va >= p->sz` holds with `sz` at 0. Plan Step 162 adds an explicit guard for slot rows. `fetchaddr`
refuses, which only `sys_exec` would meet, and BPF serves exec. Under `ascend-linux` the only readers of `program_break`
are fork, the context switch and this word, so BPF stores SET_BRK's argument unchecked.

### D4. Init has no heap

Slot 0 is identity-mapped, and physical 0x10000-0x40000 holds kernel `.rodata`. `sbrk` in slot 0 returns -1.

### D5. Zero on growth

The slice holds whatever fork copied, kernel `.rodata` included, or a dead process's bytes. The kernel zeroes the grown
range through the flat physical address. Lazy sbrk is treated as eager: no fault path exists.

This makes the grown range correct for `malloc`. It is not a confidentiality control: see Security notes. The zeroing is
a byte loop inside one trap, and no other slot runs until it ends; a grow of the whole window is the worst case.

`malloc` also needs its own `.bss` zero (`base`, `freep`), and exec does not zero `.bss`: see Security notes. The sbrk
rung's claim is `malloc` in a forked child, which takes `.bss` from its parent. It makes no claim for an exec'd image.

### D6. User pointers

ADR-109 D5 leaves this to the first rung that dereferences one. `walkaddr` and copyout's `walk` get a slot-row branch:
`pa = slice_base(s) + va` when `va < 0x800000`, else 0. The BPF pgd stays unwalked. The branch is valid only inside a
tagged trap (`upc_slot_active()`), so boot-time xv6 paths keep today's behaviour.

Inside a tagged trap the branch ignores its `pagetable` argument and uses `myproc()`. Every caller on a slot row passes
the caller's own table; kernel `kexec` would not, and BPF serves exec. Slot 0 takes the branch too, with base 0. Callers
pass `PGROUNDDOWN(va)`, so `pa == 0` refuses slot 0's whole first page, [0, 0x1000), not VA 0 alone. `user.ld` places
`.rodata` straight after `.text` from 0, so init's string literals sit in that page (`.rodata` ends at 0xF59 in
`init.elf`). No caller is hurt today: init passes no kernel-routed pointer, and the rung probes make those calls from a
forked child. A kernel-routed call from slot 0 with a literal path would get -1. Slots 6 and 7 have slice bases at or
above 56 MiB and never run: `RAM_ISOLATION_CEILING` stops fork at slot 5.

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
never enters S-mode in between. `interrupt_pending` has one setter, in the timer block that `cfg` compiles out, and a
copy tick leaves `switch_req` at `SWITCH_NONE`. Plan Step 156's "`num_processes` moves at finalize" does not cover a
reused slot: D8's halted bit does.

A rung that adds preemption owns this, and it has more to fix than a missed cursor reset:
- Nothing reserves a slot until finalize, so two parents can pick the same child slot and interleave copies into it.
- Killing a parent mid-copy leaves the child's pde 0 present. The next fork on that slot skips its first-entry path and
  takes that caller's a2 as the cursor, so the copy can be short or empty, not only misplaced.

Nothing ties I9 to that `cfg`. A compile-time tripwire would, and it is not in this ADR's rungs.

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

"D6" below means ADR-109 D6 (`UPC_D6_MISS_FAULT`), not D6 of this ADR.

- **A child can reach `reply` while ADR-109 D6 is off.** A store to VA `0xF0xxxx` writes its own pgd (`hostile_pgd`),
  and a PDE it adds maps kernel `.bss`. The wipe at trap entry (D1) stops a word stored ahead of an `ecall`. It is not
  the only gate, and there are three ways past it:
  - A U-mode `sret` runs the reply read with no trap entry, so no wipe.
  - Overwriting `tf[s].kernel_trap` sends `uservec` past `upc_slot_enter()`.
  - Moving the table base at BootParams `0x160`: BPF reads a reply the child wrote, and sscratch points at a child-owned
    frame, so `kernel_sp` and `kernel_trap` are child-chosen too.

  The reply word alone buys SET_BRK, YIELD and KILL of any live slot except init, and any child can already `kill` any
  other. A child-chosen `kernel_trap` buys far more than that. The channel adds no primitive because the hole with D6
  off is already total, not because the channel is safe against it. Init can write everything, as it can today.
- **`MRET` and `SRET` have no privilege check, so D6 alone does not confine a child.** With D6 on, no load or store
  reaches `.bss`, since they go through `translate_address`. D6 does not touch either opcode. `MPP=S` and `MEPC=main`
  are written once at boot (`start_mbc.c`), so a U-mode `mret` sets `priv_level = 1` and jumps to kernel `main()` with
  the child's registers and flat addressing. The image does not need the instruction: a tagged `ret` reaches any ROM
  word, so controlling `ra` is enough. At the least a child can re-enter kernel init or wedge the machine, since I7 then
  blocks all switching. No proven route places a chosen reply and keeps control, so this is a claimed boundary that does
  not hold, not a demonstrated forgery. Pre-existing. The fix is to reject both opcodes at priv 3; until it lands, D6 is
  not a confinement boundary. It is not in this ADR's rungs, and D6 stays off.
- **Every value BPF takes from the reply is bounded:** the tag is checked, KILL's target is masked and checked for
  liveness, and SET_BRK's value reaches only the kernel's own normalization (D3). `brk_in` is overwritten at each
  injection, so a forged one never reaches `sys_sbrk`.
- **Heap isolation between live processes needs more than D6.** With D6 off, a U-mode page-walk miss resolves to the
  physical address, so a child reads and writes another child's slice, heap included, at VA >= 8 MiB. With D6 on that
  route closes, but two remain: the `mret` route above, and BPF's `open` and `exec`, which read user paths at
  `pid_phys_offset + path_va` with neither `user_phys` nor any other bound. A chosen a0 reads any physical byte,
  including another child's heap, with D6 on or off. The read is confirmed; its leak rate as a match or no-match oracle
  is not. All of this is pre-existing and stays open with this ADR.
- **Bytes between processes:**
  - On the normal path fork overwrites the whole low band and reap clears pde 0, so a dead occupant's heap never
    reaches the next one, and the child gets only its parent's bytes. With D6 off, a live child can defeat this: it
    stores pde 0 for a free slot at `0xF00000 + slot*0x1000`, and the next fork into that slot skips first entry and
    takes the parent's a2 as the cursor. With a2 at or above 0x14000 it copies nothing, so the next occupant sees the
    dead one's heap. That child can read slices anyway, so this is no new capability. Keying first entry on a tagged
    cursor, as exec and open do, closes this and I9's cursor failure together.
  - A child can read its band without calling `sbrk`. A child of init sees init's physical low memory there: BootParams,
    CSR values and kernel `.rodata`.
  - Above the band, nothing scrubs a slice at reap, so a slot's next occupant can read the last one's stack and argv
    frame.
  - Exec does not scrub. It writes only the image's `len_words`, so a heap inherited through fork stays at VA 0x10000
    and up, and a newly exec'd program reads its parent's heap without `sbrk`. Nothing zeroes `.bss` either: the `.data`
    sidecar carries `ALLOC PROGBITS` sections only, the exec arm has no zeroing pass, and `crt0_mbc.S` calls `main` directly.
    An exec'd image's `.bss` holds whatever was at those VAs, which is a correctness gap as well as a leak (read from
    the tree, not booted).

  All four are pre-existing. D5 does not change them.
- **Opcodes with no translation.** `LR_W`, `SC_W`, `XCHG` and `CAS` skip `translate_address`, so they reach physical
  memory from U-mode. The translator emits none of them and the kernel uses libcalls, so no compiled image has one, and
  `exec` matches loader-resident images only. The boot check covers the `.rv2mbc` hash, not an opcode scan of the image,
  so a raw-MBC image would keep physical access. Suspected reachable only that way.
- **A forged-reply probe has to reach the real table.** The table links at `0x114C00`, below 8 MiB, so a store at that
  VA lands in the child's own slice, as `hostile_pgd.c` says. `hostile_slot` and `hostile_tf` do that, and plan Step
  258's `hostile_reply` in their shape would pass without testing anything. It must add a PDE first, then try both the
  `ecall` route and a U-mode `sret`. Step 258's `HR: heap-intact=yes` is the expected answer for the `ecall` route only:
  the `sret` route is expected to forge while D6 is off and `SRET` is unchecked.

## Review

Plan Step 119 asks for a separate read-only review. The agent that wrote this ADR reviewed it first, which was not
independent; those eleven findings, F1-F11, are in `references/s-stage-2b-log.md`. The independent review followed at
`4cc7577e`, nothing built or booted, and closes Step 119. Its ids carry `R-` here because its numbers overlap the
self-review's.

Verdict: no blocker against D1; sbrk, kill and pause land as designed. Each finding was re-read in the tree before its
disposition. Text fixes are in this ADR. A code fix named below is outside the rungs, so it is a For Stevie line in
`docs/battle-plans/progress/S-STAGE-2B-progress.md`, not landed work.

| Id | Severity | Finding | Disposition |
|---|---|---|---|
| R-F1 | High, pre-existing | `MRET` and `SRET` have no privilege check, so D6 does not confine a child | Accepted. Claim restated in Security notes. Fix (reject at priv 3) open, For Stevie: it must pair with any D6 flip |
| R-F2 | Medium | The wipe has three bypasses, and a forger gets more than the channel's three flags | Fixed in D1 and Security notes. "Exactly one flag" is now "at most one", stated as unenforced. No code: the `sret` route closes with R-F1 |
| R-F3 | Low | "Fork overwrites the whole low band" is false with D6 off | Fixed in Security notes. Tagged-cursor fix open, For Stevie |
| R-F4 | Low, pre-existing | Exec does not scrub | Fixed in Security notes as a fourth bullet |
| R-F5 | Low to Medium, pre-existing | `open` and `exec` read user paths with no bound, D6 on or off | Fixed in Security notes. Routing through `user_phys` open, For Stevie: verifier cost unmeasured |
| R-F6 | Low, forward | I9's failure description is incomplete, and nothing enforces it | Fixed in I9, Step 156's stale note included. Tripwire open, For Stevie |
| R-F7 | Low | D3's `vmfault` and D6's first-page statements do not match `uvm.c` | Fixed in D3 and D6 |
| R-F8 | Low to Medium, plan conflict | D14's bare tag can lose a reply | Fixed as a D1 rule that Step 220 must follow. Plan text unchanged, For Stevie |
| R-F9 | Low | The hostile probes never reach the real table | Recorded in Security notes for Phase 21. Correction: the plan does have such a step, 258, and it has this gap |
| R-S1 | Suspected | `main`'s prologue after `mret` gives a chosen-address write | Open, not booted. Rides with R-F1 |
| R-S2 | Suspected | R-F5 as a byte oracle | Open, leak rate unmeasured. Rides with R-F5 |
| R-S3 | Suspected | First-page refusal only matters for a kernel-routed pointer from slot 0 | Checked: real but unreached today. Stated in D6 |
| R-S4 | Suspected | `LR_W`, `SC_W`, `XCHG`, `CAS` skip translation | Translator read, emits none. Stated in Security notes. Opcode scan open, For Stevie |
| R-NC | Not checked by the review | Whether exec zeroes `.bss` | Checked: it does not. D5 narrowed, stated in Security notes, For Stevie |

## Alternatives considered

- **BPF serves sbrk itself.** One arm and no reply word, but kill and blocking still need a way back, so the channel
  arrives anyway one rung later, and the arm costs more than an alias row.
- **Kernel `sleep()` with a real context switch.** `sched()` from S-mode breaks I7 and moves scheduling authority to
  the kernel. That is rung 7's direction, not this ADR's.
- **A heap above the low band.** It needs a third fork copy band, and fork's copy loop is verifier-sensitive.
- **Walking BPF's pgd in the kernel.** Two formats for one table; each child's map is two superpages, so an offset
  gives the same answer.
