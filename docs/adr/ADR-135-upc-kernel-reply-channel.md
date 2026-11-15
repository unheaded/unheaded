<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-135: The owned kernel's reply channel to BPF

**Status**: Proposed (2026-11-04; implements DP-213, DP-501, DP-502). Independent read-only review done the same day, no blocker against D1: see Review. Acceptance is Stevie's. D10-D14 added 2026-11-05 for the fd rung; their review is plan Step 221. D15 added 2026-11-05 for the chdir rung; no independent review ran on it (plan Phase 19 asks for none). D7's a0 rule amended 2026-11-05 for the pipe rung; no review ran on that either (plan Phase 20 asks for none).
**Context sprint**: S-STAGE-2B (`docs/battle-plans/S-STAGE-2B-battle-plan.md`), Phases 10-13 and 17-20
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
arm when `spp == 0`, clears it, and acts. Flags: bit 0 SET_BRK, bit 1 YIELD, bit 2 KILL, bit 3 EXIT (D13). One reply per trap, with
at most one flag set: `reply_arg` has one meaning at a time. That is a rule on the kernel's one writer, `upc_reply`. BPF
does not enforce it: the block tests each flag on its own, so a forged word with two flags gets both on one `reply_arg`.
Every value stays bounded, so that is harmless today. I7 holds: the same process, with U-mode restored first.

`upc_slot_enter()` zeroes `reply` and `reply_arg` where it zeroes `cur_slot` and `trap_a7`, before the tag check, so
no word written in U-mode survives a trap that reaches `upc_slot_enter()`. Only a kernel handler writes `reply`, and no
U-mode code runs between that write and the read. Both statements cover the normal trap path. A child that can reach
the table has ways around them: see Security notes.

A write that runs after the handler must not replace the handler's reply. The plan's D14 (Step 220) has `prepare_return`
store a bare `UPC_TAG_REPLY`, which would wipe SET_BRK, YIELD or KILL, and Step 124's `upc_slot_leave()` clears
`upc_active` before `prepare_return`, so that code cannot tell a tagged trap. D14 below therefore puts the mark in
`upc_slot_leave()`, before it clears `upc_active`, and stores the bare tag only if `reply` is 0.

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
- It keeps a0. While `upc_retry()`'s YIELD reply is pending `syscall()` does not store the return value, so fd, buffer
  and count survive for the re-run. `piperead` and `pipewrite` return `UPC_RETRY_RC`; `upc_pause` still returns n.

A pipe follows the first rule this way: `piperead` retries on an empty pipe with a live writer, and `pipewrite` retries
only while `i` is 0. Past that it returns short, because a re-run would send the moved bytes again, and the caller loops.

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

### D10. Kernel fd numbers

Kernel fds are `16 + ofile index`, so they run 16-31. BPF's table is `NOFILE = 16`, so the spaces are disjoint and
neither allocator sees the other. BPF keeps fds below 16: the console and files opened with no masked flag (D11).
Translation happens where a number crosses to the user, when `upc_slot_active()`. `argfd` takes the user's number and
hands back the index, which is what `sys_close` clears. `fdalloc` keeps returning the index, because `sys_pipe` indexes
`ofile` with it, and `fduser()` turns an index into the user's number in `sys_open`, `sys_dup` and `sys_pipe`'s
`copyout`. Outside a tagged trap all three keep today's behaviour.

Limits:
- `dup` of a kernel fd stays at or above 16, so shell redirection onto fds 0-2 waits for the full hand-off (rung 6).
- Only kernel fds share an offset across fork. Fork copies BPF fd kinds, not `FD_INODE_MAP`, so a child reading an
  inherited BPF file fd gets its slot's stale `{inum, offset}`. Pre-existing.
- `open(path, O_RDWR)` on a regular file stays in BPF (D11), which ignores the flags and hands back a fd below 16.
  BPF `write` never looks at the fd, so a write there prints to the TTY, returns n and leaves the file alone. Pre-existing,
  and it is the one way left to lose a file write silently: a caller that wants one passes `O_WRONLY` or `O_CREATE`.

### D11. Routing

`kernel_routed_alias` takes `cpu` and gains these rows, each also behind `cfg!(feature = "ascend-linux")`:

```
5 | 8 | 10 | 16 | 21 if UPC_RUNG_FD && cpu.regs[8] >= 16 => 11
15 if UPC_RUNG_FD && cpu.regs[9] & (O_WRONLY | O_CREATE | O_TRUNC) != 0 => 11
2 if UPC_RUNG_FD => 11     (D13)
24 if UPC_RUNG_FD => 11    (D12)
```

The compare is unsigned, so a negative fd reaches the kernel too, and `argfd` refuses it with -1. For `read`, `fstat`,
`dup` and `close` that matches BPF today. BPF `write` prints whatever the fd, so `write(-1, ...)` goes from printing to
-1. `open("console", O_RDWR)` stays in BPF: O_RDWR alone is not in the mask, and read-only BPF `open` is unchanged. Nothing
in the corpus passes a masked flag: `init` and `ush` open `console` with O_RDWR. The arms for 5, 8, 10, 15, 16 and 21
stay, because BPF still serves fds below 16.

BPF reads the ramdisk and the kernel writes through its log, so a finished `write` is visible to a later BPF `open` and
`read` the way `mkdir`'s entry is: `end_op` has committed it. A `write` runs inside one trap, and no other slot runs until
it ends, as with D5's zeroing.

### D12. Fork note

Lazy refresh is wrong for inheritance: a parent may close a fd or exit before its child's first trap.

When BPF finalizes a fork under `UPC_RUNG_FD`, the `base != 0` test that sets the fresh bit also records that a note is
due. Finalize then runs to its end as it does today (`num_processes`, `PARENT_OF`, `STAT_FORKS`), and where it writes the
parent's a0 it chooses: with a note due, `regs[1] = 24`, `regs[8] = child slot`, `pc -= 1`; otherwise
`regs[8] = child_pid + 1`. The note setup must come last, or that write turns the argument into slot + 1, the note
refuses, and `fork()` answers -1 with the child alive. The parent's ecall runs again as number 24, aliased into the
kernel. A fork that fails with -1 finalizes nothing and takes no note.

`sys_upc_forknote` (`syscalls[24]`, `SYS_upc_forknote 24`) refuses unless its argument is a slot in 1..7 whose
`fresh_mask` bit is set, so a user who issues 24 gets -1. Otherwise it clears the bit, refreshes the child's row at once
through the code `upc_slot_enter()` uses, closes anything left in the row's `ofile`, `filedup`s the parent's, `idup`s
`cwd`, drops the row's old `cwd` inside a log transaction (D9), and returns the child's pid, which is the parent's
`fork()` result. On the normal path the row's `ofile` is empty there, because exit and kill close it (D13). A forged EXIT
or KILL through a U-mode `sret` ends a slot without either, so the note closes what it finds rather than overwrite it.

`upc_slot_enter()` then finds the bit clear and `pid` set and leaves the filled row alone, so with the rung on the note
is what meets ADR-109 I3, not the child's first trap. With the const off nothing changes: the bit waits for that first
trap, and 24 is not routed and answers `-ENOSYS`.

The bit is set only from finalize to the note, and the parent runs nothing in U-mode between them, so for a process
that cannot reach the table it is never set where 24 could find it. With ADR-109 D6 off a child can reach the table
(Security notes), set a bit itself and overwrite a live sibling's row, and the sibling's fd 16 then names the attacker's
file. That is one more use of the same hole. The refusal is what `fdprobe` tests, and its mutant shows the test can fail.

The re-run never reaches the fork arm, so ADR-078 §3's fork row is untouched: a2 passes through the trapframe like any
other register. `child_state` is stored first, so the child's r1, a0 and pc stay the fork's. Finalize and note are
consecutive in one process, so I9's argument covers the gap, and a rung that adds preemption owns this as well: a parent
preempted there leaves the child to the lazy refresh with no fds, and a sibling's 24 could fill that row (suspected, not
shown). D12 raises the cost of I9 having no tripwire.

### D13. Exit

`2 if UPC_RUNG_FD => 11`. On a tagged trap `sys_exit` calls `upc_exit(n)`, which closes `ofile`, replies EXIT (bit 3,
`UPC_REPLY_EXIT 8`) and returns n, so a0 holds the status. The test is `upc_slot_active()`, as in `sys_kill`, not
`upc_is_slot_row()`, which leaves out slot 0: a gate booted as `--userland` runs in slot 0 and exits, and `kexit` must
not see it. BPF then runs the shared exit path from the reply block, as the arm for 2 does: `exit_code` from a0,
`exit_slot` the caller, `SWITCH_EXIT` if other processes exist, halt if not. `cwd` stays on the row (D9).

`upc_kill` closes the target's `ofile` before replying KILL. BPF can still answer -1, but only for a target that is
already dead, whose `ofile` is empty, so the close changes nothing there. A target is never inside a syscall when it is
killed, because a tagged trap runs to its SRET (I7) and a retry holds nothing (D7).

`fileclose` takes a log transaction. One process is in the kernel at a time, so `begin_op` never waits and `sleep` stays
unreached on a slot row (I4). A forged EXIT ends only the process that forged it, which `exit` already allows. With
ADR-109 D6 off, a forged EXIT or KILL through a U-mode `sret` also skips this close, so the inodes stay held and a pipe's
EOF does not arrive until the slot's next note closes them (D12).

### D14. Quiet trap returns

The SRET arm writes `S` to the TTY and arms the 100-PC tracer, and those bytes are in the goldens: each of the 12 has
one `S` and one tracer run, from init's first return, which is not a tagged trap. Corpus boots take no kernel trap
today. With D12 and D13 every fork and exit does, so all 12 goldens would move.

A tagged trap marks its return. `upc_slot_leave()`, before it clears `upc_active`, stores a bare `UPC_TAG_REPLY` if
`upc_active` is set and `reply` is 0 (D1); a handler's reply already carries the tag. The return to U-mode skips `S` and
the tracer when it sees the tag. Every tagged trap goes quiet, not only fork and exit, so with the rung on the earlier
rungs' traps lose their `S` and PCs too, and D7's retry noise ends.

The tag has to be read before the tracer is armed. The plan's D14 moves the reply read ahead of the tracer, into the
loop. That read measured +9,564 (Consequences), and this rung starts with 8,192 of headroom, so the order is reached the
other way round: `S` and the tracer move behind the read, into the block after the loop. The arm breaks right after them
when `spp == 0`, so no instruction runs between the two places and the bytes land in the same order. Under `UPC_RUNG_FD`
the arm keeps them for `spp != 0` only. The block writes them when `reply_req` is set, the tag was not seen (`base == 0`
counts as not seen), and the arm's SEPC lookup did not halt. With the const off the arm stays as it is and the block's
new branch folds away. Step 228 measured the rung whole, this move included: 891,808 off, 889,100 on.

The goldens also hold the `tty_r` and `waitpid` counters, and those moved where the TTY bytes did not: `ls` read 9 and 7
against 3 and 1. A 2026-03 diagnostic in the `CALLR` arm bumps stat keys 20 and 21, which are also `STAT_TTY_READS` and
`STAT_WAITPIDS`, and `syscall()` calls its handler through a pointer. No corpus boot had reached that arm before D12 and
D13, so the goldens never showed it. Under `UPC_RUNG_FD` the arm drops the two bumps; the Doom object keeps them. The
goldens were not recaptured. With the const off the object is unchanged, so a boot with kernel traps still overcounts
both, as it did before this rung.

The mark does not depend on the const. With `UPC_RUNG_FD` off, BPF reads a bare tag, clears it and acts on no flag. A
forged bare tag hides one diagnostic and nothing else.

### D15. A process off the root

BPF `open` walks from the root inode and never sees a row's `cwd`, so once `chdir` is real a relative read-only open
would answer for the wrong directory. A process off the root therefore opens through the kernel.

The table's last word, the pad at +0x81C, becomes `cwd_mask`. Bit s is set when row s's `cwd` is not the root inode.
`UPC_ST_SIZE` does not move. Only the kernel writes it, in two places: `sys_chdir` after it installs the new `cwd`, by
`ip->inum != ROOTINO`, so `chdir("..")` back to the root and `chdir("/")` both clear the bit; and the fork note (D12),
which copies the parent's bit to the child's, setting or clearing it, so a reused slot never keeps a dead process's bit.
A dead slot's bit stays with its `cwd` (D9, D13) until that note, and nothing reads it: only the running slot's bit is read.

`kernel_routed_alias` gains two rows, behind `cfg!(feature = "ascend-linux")`:

```
9 if UPC_RUNG_CHDIR => 11
15 if UPC_RUNG_CHDIR && cwd bit set => 11
```

The second reads the table word, and only when n is 15. `base == 0` counts as not set. Unlike the reply read, this one sits
inside the execute loop; Step 241 measured the rung whole at +1,396, against that read's +9,564. `sys_open` then answers a fd at or
above 16, which `read`, `fstat`, `dup` and `close` already route (D11). `sys_chdir` is stock over a row with a real `cwd`
(D9), and `mkdir`, `unlink` and `link` already resolve through `namex` from the row's `cwd`.

Limits:
- `exec` stays in BPF and takes the basename of its path, looked up in `PROGRAM_TABLE`, so it ignores `cwd`, as it
  ignores every directory in the path. `exec` keeps the row, so the process keeps its `cwd` and its bit.
- `console` is implicit in BPF, not in `fs.img`, so `open("console")` off the root reaches the kernel and answers -1.
  `init` and `ush` open it from the root.
- `mknod` stays the BPF stub, which takes no path walk at all.
- A dead slot still holds its `cwd` reference until the slot's next note (D9), so a directory removed in the meantime keeps
  its inode until then. The probe shows both halves. At most one inode per slot.

With ADR-109 D6 off a child can write this word as it can the rest of the table. Setting a bit sends that slot's open to the
kernel, which answers correctly from the row's `cwd`. Clearing one sends an off-root open to BPF, which walks from the
root: read-only, and the slot could have opened that path anyway. One more use of the same hole, and no new reach.

### D16. Bounded user paths and slot 0's window (2026-11-15, S-STAGE-2C Phase 3)

Three places took a user pointer further than its owner's memory. Each now refuses.

- **BPF `open` and `exec` are reached only when `user_phys(pid, path_va, 128)` answers** (128 is `MAXPATH`,
  `param.h`); otherwise a0 is -1 and no byte of the path is read. Before, both added `path_va` to the slice base with
  no bound, so a chosen a0 read any physical byte (R-F5). The test is one arm ahead of both syscalls, run on every
  re-entry. Written inside the arms it cost +48,914 verified instructions (the refused state was walked through the
  path hash and the dirent scan as well); ahead of them it costs +2,261. A path that starts within 128 bytes of the 8 MiB ceiling is refused as well; no corpus path sits there (the
  argv frame is at 0x600000, the stack top at 0x500000).
- **`upc_user_pa()` answers slot 0 only inside init's own windows.** Slot 0's base is 0, so its VAs are physical
  addresses and the 8 MiB ceiling alone let a kernel-routed pointer from init name kernel `.rodata`, `.data`, `.bss`
  (the slot tables), the CSR window and the TTY registers. The windows are the image pages [0x1000, 0xC000) and the
  stack [0x400000, 0x500000); everything else is 0, refused. The first page stays refused as before (D6). The image
  window stops at the TTY registers; the largest image booted as init ends at 0x3010 (`fdprobe`).
- **`pipewrite` returns -1 when its first `copyin` fails**, and the short count when a later one does, as Linux does.
  It returned its byte count, so a write from a bad buffer answered 0.

Init's trust model is otherwise unchanged: its own loads and stores still reach all of low memory through the identity
directory. This narrows what the kernel will do on init's behalf, not what init can do by itself.

### D17. User images carry no privileged or untranslated opcode; atomics translate (2026-11-15, S-STAGE-2C Phase 4)

`XCHG`, `CAS`, `LR_W` and `SC_W` take their address with no translation, so at U-mode they reach physical memory
(R-S4). The translator emits none of them and the kernel uses libcalls, so no compiled image has one. Booted on
2026-11-15 with a patched image (one `LD` rewritten to `XCHG` by `scripts/upc-mbc-patch-opcode.py`): a child's `XCHG` at
VA 0x800400 read the ramdisk's superblock word and stored over it. The `.rv2mbc` hash gate does not see this: it covers
the address map, not the image's words.

Two parts were decided (Stevie's answer A-61: fix it, plus an opcode scan):

1. **The load-time scan (landed).** bootctl reads the `--userland` image and every resident program word by word and
   refuses the boot on any of `XCHG` 0x3D, `CAS` 0x3E, `MRET` 0x47, `SRET` 0x48, `LR_W` 0x49, `SC_W` 0x4A
   (`UserImageOpcode`, naming the image, the word and the opcode). The kernel image is not scanned: it needs `MRET` and
   `SRET`. One probe holds privileged returns on purpose (`hostile_reply`: an `mret` and two `sret`s, which is how it
   shows they do nothing at U-mode), so `UPC_ALLOW_USER_PRIVRET=1` admits `MRET` and `SRET`, says so in the boot
   output, and never admits an atomic; the harness sets it for that probe alone. This closes the one way such an opcode reaches a child: an image that did not come from the translator.
2. **Translation in BPF (not landed: verifier budget).** The design is `translate_address(cpu, addr)` for each atomic
   under `cfg!(feature = "ascend-linux")`, the reservation keeping the virtual address. Measured against 895,768 with
   4,232 of headroom: four inlined translations +20,993; one ascend-only arm for all four with a single translation
   +16,855. Both pass 900,000. A bounded form, the slice's linear map through `user_phys` with a miss resolving where a
   D6 miss does, measured +3,263 and was not landed either: it is not the decided design, and it leaves 969 for the
   fork and file-fd fixes that follow. The choice waits on Stevie, or on rung 3 freeing instructions.

Until part 2 lands, a U-mode atomic is unreachable only because no loadable image can hold one.

### D18. Fork's first entry is BPF's to decide; no timer switch under ascend (2026-11-15, S-STAGE-2C Phase 6)

Fork copies the parent's working set over several ticks, re-entering the same `ecall` with its cursor in a2. It took
"the child's pde 0 is not present" to mean "this is the first entry: build the directory, zero the cursor". While
ADR-109 D6 was off a process could write that word, so a planted present bit skipped the first entry and the caller's
a2 became the cursor (R-F3). Booted with D6 off for the experiment: the child in the planted slot had an incomplete
copy.

- **First entry is read from the child slot's PROC_TABLE row.** While a fork copies into a slot, the row's state word
  (word 17) carries `PROC_FORK_RESERVED | parent << 8`; the first entry writes it, builds the directory and zeroes the
  cursor; finalize writes the whole row and the mark goes with it. PROC_TABLE is BPF's own map, not guest memory, so
  no store and no register decides it. With D6 off and the plant landed, the copy is now complete.
- **Not a tag in a3.** The first design here keyed the entry on a3 carrying `FORK_TAG | child_slot`, as exec and open
  key theirs. a3 is the caller's register: a process that sets the tag itself before the `ecall` is back where the
  planted bit put it. Exec and open can afford that, because a forged tag there hurts only the forger's own image; a
  fork that skips its copy hands the child the slot's last occupant. So the mark lives where the caller cannot write.
- **A slot is marked from the first copy tick (R-F6), and that is all the reservation there is.** The slot scan does
  not skip a row another parent has marked, and nothing clears a mark when a parent dies mid-fork: measured, the skip
  alone is +11,789 verified instructions against 4,064 of headroom. Neither case can happen while nothing else runs
  between a fork's first tick and its finalize, which is the next point.
- **I9 is a compile-time fact.** `TIMER_SWITCH` is `!cfg!(feature = "ascend-linux")`, the timer block tests it, and a
  `const` assertion fails any ascend build in which it is true. Preemption under ascend has to bring the scan skip and
  the death clear with it.

Cost: +169 verified instructions. `hostile_fork` holds the plant, and a hand-made fork with a2 past the end of the
copy (a3 clear, then all ones).

**Off since the same day.** The mark is behind `UPC_FORK_MARK`, and that const is false: the block-syscall guard
(RV-1, below) needed the instructions. Off, fork reads pde 0 as before, which is sound while D6 is on, and the object
is the earlier one exactly (895,767 without the guard). The tripwire and `hostile_fork` stay; the probe passes either
way with D6 on.

### D19. Rung 3: console write is the kernel's (2026-11-15, S-STAGE-2C Phase 9)

Under `UPC_RUNG3`, `write` on a fd below 16 goes to the owned kernel unless BPF's table says the fd is a file.

- **Routing.** `kernel_routed_alias`: `16 if ascend && UPC_RUNG3 && a0 < 16 && fd_kind(pid, a0) != FD_INODE => 11`.
  The test is "not a file", not "is the console": the gate programs run as init and print on fd 1 without ever
  opening `console`, so their fd 1 is `FD_FREE` in BPF's table, and BPF's loop printed for them all the same. Fds at
  or above 16, and negative ones, already go to the kernel (D11).
- **Kernel.** `sys_write` on a slot row with a fd in 0-15 calls `consolewrite(1, addr, n)`: `either_copyin` through
  the slot's page table, 32 bytes at a time, each byte stored by `uartputc_byte`, a one-byte store to 0xC001. Kernel
  `printf` keeps `uartputc_sync`'s word store, so the boot lines keep their char-plus-NUL bytes (they are in the
  goldens). A copy that fails before the first byte answers -1; one that fails later answers the bytes written, as
  stock xv6 does. A count of 0 answers 0. Only BPF routes such a fd there, so the kernel does not ask what kind it is.
- **BPF.** The console byte loop (the `n == 1` store and the 0xD0C0-tagged re-entrant cursor) is the `!UPC_RUNG3`
  branch, so the on object does not hold it. What is left of the arm answers -1.
- **T-F3 interim, by the same change.** With the rung on, the only `write` that still reaches BPF's arm is one on a
  file fd below 16 (plain `O_RDWR` on a regular file, D10), and it answers -1 instead of printing the bytes and
  returning n. That is Phase 7's refusal, with no kind test in the arm: the alias has already made it. The file write
  itself lands in the kernel at rung 6. With `UPC_RUNG3` off the old behaviour is back; the off object has no room
  for the refusal (Phase 7 measured +622 against 94 of headroom).
- **Cost shape.** Every `write` is now one trap, so the cost is per call, not per byte. xv6's user `printf` writes one
  byte per call.
- **Slot 0.** Init's first page reads as physical 0 to `copyin`, which refuses it (D6), so a string literal in init's
  `.rodata` cannot be the buffer of a kernel-routed `write`. Init and `uinit` print through `printf`, whose byte is on
  the stack; every other corpus program runs in a child slot. `conprobe` keeps its buffers in `.bss`.

## Guards

One const per step, off by default: `UPC_REPLY` for the channel, `UPC_RUNG_SBRK`, `UPC_RUNG_KILL`, `UPC_RUNG_SLEEP`,
`UPC_RUNG_FD`, `UPC_RUNG_CHDIR` for the rungs. With a const off the object equals the previous object (I6 style). Each alias row carries
`cfg!(feature = "ascend-linux")`, because `kernel_routed_alias` runs in the Doom build too.

## Consequences

- `phase12.rs`, `upc_slots.h` and `parity_slot_tables.rs` move together. The table stays kernel `.bss`, published
  through BootParams `reserved[0]`; no BootParams field changes.
- Every rung past uptime uses one read on the return to U-mode. Inside the execute loop it measured +9,564 (plan Step
  129), over the +3,000 bar, so the SRET arm only sets `reply_req` and breaks, and the read sits beside the
  `DEFERRED_SWITCH` block, before the switch: +2,299. D1's "reads `reply` in the SRET arm" means that block. The price is
  one short tick per return to U-mode.
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
  other. This is ADR-109's M-247 route: a store past 8 MiB reaches any slot's pgd, and with this channel it now reaches the
  reply word too. It stays open while D6 is off. A child-chosen `kernel_trap` buys far more than that. The channel adds no primitive because the hole with D6
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
- **Booted by `hostile_reply` (Phase 21, D6 off).** It adds the PDE and tries each route. A word stored ahead of an
  `ecall` does not take effect: `HR: heap-intact=yes`, and with the wipe removed that line goes to `=no`. A U-mode `sret`
  forges: a chosen SET_BRK lands and the child keeps control, so that route is now demonstrated, not expected. The probe
  picks a value inside the heap window; D3 normalizes one outside it, read from the code and not booted. A forged KILL of
  slot 0 was refused (`HR: kill-init-refused=yes`). The same alias reaches
  `fresh_mask`, so a child can run the fork note on a slot it names (`HR: note-forged=yes`, on a free slot). On a live
  slot the note would close that row's fds and hand it the forger's: read from the code, not booted. The harness pins both
  forgeries, so a change to either shows up. All of this needs D6 off, and none of it is closed here.

- **2026-11-15, S-STAGE-2C Phase 2: `MRET` and `SRET` at priv 3 are NOPs** (ADR-109, dated section of that day).
  R-F1 is closed: a U-mode `mret` or `sret` does nothing, by an inline instruction or by a tagged `ret` to a ROM word.
  R-F2's `sret` route and R-S1 close with it. `hostile_reply` runs both opcodes in a child of their own before the PDE
  route and wants `mret-refused=yes` and `sret-refused=yes` in every build; with D6 off, `sret-forged` now says no and
  the forged KILL, which rode the same `sret`, is not tried. The fork-note forgery still needs D6 on to close.

- **Closed 2026-11-15 (S-STAGE-2C Phase 2): ADR-109 D6 is on, and `MRET`/`SRET` at priv 3 are NOPs.**
  `hostile_reply` closes on every route: the PDE store is dropped (`table-reached=no`, `route=closed`), and a U-mode
  `mret` or `sret` does nothing. R-F1, R-F2's `sret` route and R-S1 are closed; F1's live-to-live heap route is closed.
  R-F5 (unbounded `open` and `exec` paths) remains until S-STAGE-2C Phase 3.

- **Closed 2026-11-15 (S-STAGE-2C Phase 3, D16):** R-F5, and R-S2's byte oracle with it: BPF `open` and `exec` read no
  path outside the caller's slice. A kernel-routed pointer from slot 0 is answered only inside init's image and stack
  windows. A pipe write from a bad buffer is -1. `hostile_path` holds all six and each fix has a mutant that fails it.

## Review

Plan Step 119 asks for a separate read-only review. The agent that wrote this ADR reviewed it first, which was not
independent; those eleven findings, F1-F11, are in `references/s-stage-2b-log.md`. The independent review followed at
`4cc7577e`, nothing built or booted, and closes Step 119. Its ids carry `R-` here because its numbers overlap the
self-review's.

F7 and F8 of that first review are parked as ADR-145 and ADR-146 (2026-11-12, Stevie; written 2026-11-15, S-STAGE-2C).

Verdict: no blocker against D1; sbrk, kill and pause land as designed. Each finding was re-read in the tree before its
disposition. Text fixes are in this ADR. A code fix named below is outside the rungs, so it is a For Stevie line in
`docs/battle-plans/progress/S-STAGE-2B-progress.md`, not landed work.

| Id | Severity | Finding | Disposition |
|---|---|---|---|
| R-F1 | High, pre-existing | `MRET` and `SRET` have no privilege check, so D6 does not confine a child | Accepted. Claim restated in Security notes. Fix (reject at priv 3) open, For Stevie: it must pair with any D6 flip. **Closed 2026-11-15, S-STAGE-2C Phase 2** |
| R-F2 | Medium | The wipe has three bypasses, and a forger gets more than the channel's three flags | Fixed in D1 and Security notes. "Exactly one flag" is now "at most one", stated as unenforced. No code: the `sret` route closes with R-F1 |
| R-F3 | Low | "Fork overwrites the whole low band" is false with D6 off | Fixed in Security notes. Tagged-cursor fix open, For Stevie. **Closed 2026-11-15, S-STAGE-2C Phase 6 (D18): the first entry is read from a BPF-owned mark, not from pde 0 and not from a register** |
| R-F4 | Low, pre-existing | Exec does not scrub | Fixed in Security notes as a fourth bullet |
| R-F5 | Low to Medium, pre-existing | `open` and `exec` read user paths with no bound, D6 on or off | Fixed in Security notes. Routing through `user_phys` open, For Stevie: verifier cost unmeasured. **Closed 2026-11-15, S-STAGE-2C Phase 3 (D16)** |
| R-F6 | Low, forward | I9's failure description is incomplete, and nothing enforces it | Fixed in I9, Step 156's stale note included. Tripwire open, For Stevie. **Closed 2026-11-15, S-STAGE-2C Phase 6 (D18): a compile-time assertion; the slot is marked from the first copy tick, the scan skip and death clear wait on preemption (over budget)** |
| R-F7 | Low | D3's `vmfault` and D6's first-page statements do not match `uvm.c` | Fixed in D3 and D6 |
| R-F8 | Low to Medium, plan conflict | D14's bare tag can lose a reply | Fixed as a D1 rule that Step 220 must follow. Plan text unchanged, For Stevie |
| R-F9 | Low | The hostile probes never reach the real table | Recorded in Security notes for Phase 21. Correction: the plan does have such a step, 258, and it has this gap |
| R-S1 | Suspected | `main`'s prologue after `mret` gives a chosen-address write | Open, not booted. Rides with R-F1. **Closed with R-F1, 2026-11-15** |
| R-S2 | Suspected | R-F5 as a byte oracle | Open, leak rate unmeasured. Rides with R-F5. **Closed with R-F5, 2026-11-15** |
| R-S3 | Suspected | First-page refusal only matters for a kernel-routed pointer from slot 0 | Checked: real but unreached today. Stated in D6 |
| R-S4 | Suspected | `LR_W`, `SC_W`, `XCHG`, `CAS` skip translation | Translator read, emits none. Stated in Security notes. Opcode scan open, For Stevie. **2026-11-15, S-STAGE-2C Phase 4 (D17): the load-time scan landed and closes the reachable route; translation in BPF did not (verifier budget) and stays open** |
| R-NC | Not checked by the review | Whether exec zeroes `.bss` | Checked: it does not. D5 narrowed, stated in Security notes, For Stevie. **Closed 2026-11-15, S-STAGE-2C Phase 5: the `.data` sidecar carries `.bss` as zeros (ADR-077, note of that day)** |

Plan Step 221 reviewed D10-D14 independently at `1dda0be8` plus the uncommitted text, nothing built or booted, and found
no blocker on its three questions: 24 cannot forge a child's row through the syscall, a parent cannot be killed between
finalize and the note, and the re-run does not disturb a2's copy cursor. Those findings are numbered `T-` here, because
their numbers overlap both sets above. Each was re-read in the tree before its disposition.

| Id | Severity | Finding | Disposition |
|---|---|---|---|
| T-F1 | Medium | D12 sets the note's a0 where finalize then overwrites it, so the note reads slot + 1 and refuses | Fixed in D12: finalize records that a note is due and chooses at the a0 write |
| T-F2 | Medium, forward | `fdalloc` returning `16 + i` makes `sys_pipe` index past `ofile` | Fixed in D10: `fdalloc` keeps the index, `fduser()` translates, `sys_pipe` included, so Phase 20 inherits it right |
| T-F3 | Low to Medium, pre-existing | Plain `O_RDWR` on a regular file loses writes silently; `write` does not refuse a negative fd today | Stated in D10 and D11. No code: BPF `open` cannot tell a file from the console before the walk. For Stevie |
| T-F4 | Low to Medium | D12's claims have no test | `fdprobe` gains the tokens, and the `fresh_mask` check gets a mutant. The cwd drop can be booted, not observed, until chdir: the row's reference is always root |
| T-F5 | Low, D6 off only | D12 and D13 assume `ofile` is empty, which a forged reply breaks | Fixed: the note closes what it finds; D12 and D13 restated |
| T-F6 | Low | ADR-109's `fresh_mask` line goes stale, and `upc-rung0-assert.awk` would fail with the rung on | ADR-109 amended (I3, D5 note, Security notes) and the asserter updated together |
| T-F7 | Low | Plan Steps 224-225 still say `prepare_return` tag and name no arm for 24; `parity_slot_tables.rs` needs `UPC_REPLY_EXIT` | This ADR rules (D1, D14). The arm lands with Step 225 and the parity file joins Step 227's commit. Plan text unchanged, For Stevie |
| T-F8 | Low, pre-existing | Forked BPF file fds lose their state | Stated in D10 |
| T-S1 | Suspected | Corpus forks and exits now take injected traps from exec'd images | Answered: 12/12 SAME with them, so the path works |
| T-S2 | Suspected | Extra trap ticks could cut off a golden with a tight trigger count | Answered: no golden was cut off. The counters did move, for another reason: see D14 |
| T-S3 | Suspected | D11-D14 may not fit 8,192 of headroom | Answered: the on object fell to 889,100 |

## Independent review of S-STAGE-2C Phase 2 (2026-11-15)

S-STAGE-2C Step 29 asks for a separate read-only review of the priv-3 test and of D6 on. The lane read its own change
first; the independent read followed at `10078582`, nothing built or booted by the reviewer. Its ids carry `RV-`.

Verdict: the three diffs it was given are sound. No legitimate path runs `MRET` or `SRET` at priv 3; the NOP is safe
(PC already advanced, privilege stays 3, the reply read is set only inside the `SRET` arm); D6 breaks no legitimate
access; nothing relied on `SRET`'s 16-bit mask as a bound. It found a confinement hole outside the diffs, which D6
does not close. Each finding was re-read in the tree, and RV-1 was booted, before its disposition.

| Id | Severity | Finding | Disposition |
|---|---|---|---|
| RV-1 | High, pre-existing | The block device's syscalls, 200 and 201, had no privilege test and take a physical buffer address and the caller's cursor. A child's `ecall` copied a ramdisk block over any physical memory, and with 201 then 200 read any physical memory back | Booted: `hostile_block`'s child overwrote init's buffer and read init's secret, D6 on. **Fixed 2026-11-15:** a U-mode caller's 200 or 201 becomes a number with no arm and answers -ENOSYS; the kernel's calls (S-mode) are unchanged. +4,139 verified instructions |
| RV-2 | Medium, pre-existing | The load-time scan (D17) listed six opcodes and missed `PUSH`, `POP`, `INT` and `IRET`, which reach memory through r15 with no translation | **Fixed 2026-11-15:** all four are refused in a user image, with or without the probe hatch. No built image holds one. As with the atomics, the interpreter itself does not refuse them at U-mode: the scan is the control |
| RV-3 | Low | The host engine (`crates/monad-mbc/src/execute.rs`) has neither the priv-3 refusal nor D6, so differential runs diverge; refusals and D6 misses increment no counter, so hostile behaviour leaves no trace | Open, For Stevie. A counter costs verified instructions the object does not have today |

**What RV-1 cost.** The guard does not fit beside everything else: with it the object read 900,081 against the 900,000
line. It outranks D18's mark (+169, a second lock on a route D6 already closes), so the mark went behind
`UPC_FORK_MARK`, off, and returns when a rung frees instructions. With the guard on and the mark off the object is
899,906.

## Alternatives considered

- **BPF serves sbrk itself.** One arm and no reply word, but kill and blocking still need a way back, so the channel
  arrives anyway one rung later, and the arm costs more than an alias row.
- **Kernel `sleep()` with a real context switch.** `sched()` from S-mode breaks I7 and moves scheduling authority to
  the kernel. That is rung 7's direction, not this ADR's.
- **A heap above the low band.** It needs a third fork copy band, and fork's copy loop is verifier-sensitive.
- **Walking BPF's pgd in the kernel.** Two formats for one table; each child's map is two superpages, so an offset
  gives the same answer.
