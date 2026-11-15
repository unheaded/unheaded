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
  **Interim 2026-11-15 (S-STAGE-2C Phase 7, D19):** with `UPC_RUNG3` on, BPF refuses a write to a file fd (-1); the
  write lands in the kernel at rung 6 (Phase 12).

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

**On again 2026-11-15, after rung 3 (D19).** Rung 3 took the console byte loop out of the object (-14,552), so the
mark fits: `UPC_FORK_MARK` is true, 885,529 verified, gate `mark-after` green with `hostile_fork`.

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

### D20. Rung 4: console read is the kernel's (2026-11-15, S-STAGE-2C Phase 10)

Under `UPC_RUNG4`, `read` on a fd below 16 goes to the owned kernel when BPF's table says the fd is the console.

- **Routing.** `kernel_routed_alias`: `5 if ascend && UPC_RUNG4 && fd_kind(pid, a0) == FD_CONSOLE => 11`. Unlike D19
  the test is "is the console": a read on a free fd below 16 answered -1 in BPF before the rung and still does.
- **How the byte gets there.** The keyboard ring is BPF's (`KBD_MAP`, `KBD_HEAD`, `KBD_TAIL`); the kernel's
  `uartgetc` reads the Doom-era keyboard word, not the ring, and a ring pop inside the load path would cost every
  load. So the hand-over rides the trap. At the injection of a console read (`a7 == 5`, fd below 16), `inject_strap`
  writes the next ring byte, tagged `UPC_TAG_KBD`, into a new slot-table word `kbd_in` (0 when the ring is empty) and
  does **not** advance the ring. The kernel takes the byte only when it needs one (`upc_kbd_take`); taking sets
  `UPC_REPLY_KBD` in the reply at the return, and BPF advances `KBD_TAIL` when it reads that flag, nowhere else. A
  byte that is not taken is offered again at the next console read. The table grows by one word (0x820, size 0x824);
  the parity test covers the offset, the tag and the flag.
- **Kernel.** `sys_read` on a slot row with a fd in 0-15 calls `upc_consoleread(addr, n)`. The pointer is judged
  first (`upc_user_pa`), so a bad one answers -1 with or without input. With no complete line in the console buffer,
  the trap's byte goes through `consoleintr` (the stock line discipline: `^H`, `^U`, `^D`, a full buffer) with echo
  off; if there is still no line the handler takes the retry hand-off (D7) and never sleeps. With a line buffered it
  copies out as `consoleread` does: up to n bytes, stopping after a newline, 0 at `^D`. One byte per trap, so a line
  of k bytes costs k traps before its first byte is returned.
- **Polling, not an interrupt.** The skeleton had the KBD ring as an interrupt source. There is no UART interrupt on
  the UPC; the read path polls on each (re)run of the ecall. Recorded as a departure.
- **No echo.** BPF never echoed typed input and the goldens hold what is printed, so the polled path sets a flag
  that keeps `consoleintr` from printing the character or a backspace. The interrupt path (`uartintr`), dead on the
  UPC, keeps stock echo.
- **`tty_r`.** `STAT_TTY_READS` is bumped where the ring advances, once per byte the kernel took. Before the rung it
  was bumped once per byte BPF delivered. A reader that consumes its input reads the same number either way; a read
  blocked on an empty ring bumps nothing, as before. (The plan put the bump at routing; a read left blocked at the
  prompt at the end of every boot would then count one more than the goldens.)
- **What changes for a reader.** A read returns a line (or n bytes of one), not one byte: stock xv6 behaviour.
  `sh`'s `gets` asks for one byte at a time and sees no difference. Control characters are now the line
  discipline's: `^H` and `^U` edit the pending line, `^D` is end-of-file, `^P` runs `procdump`. BPF passed them
  through as bytes. No corpus input holds one.
- **BPF.** The console branch of the read arm is the `!UPC_RUNG4` branch, so the on object does not hold it.

### D21. Rungs 5+6: the kernel's fd table is the only one (2026-11-15, S-STAGE-2C Phase 11; reviewed the same day, dispositions below)

One const, `UPC_RUNG56`, one gate (the skeleton's first kickoff option). Written against the tree at `d980e84b`:
rungs 3 and 4 are on, the block calls 200/201 refuse a U-mode caller (RV-1), and `kbd_in` exists. Where the plan's
eight points no longer match that tree, the point is restated here and the difference is listed under "Departures
from the plan's wording". The text was written before anything was built; the independent review (Step 210) read
it at `ae44692b`, and where it was right the points below are amended in place, each amendment marked "(review
F*n*)". The review table follows the design.

`UPC_RUNG56` needs `UPC_RUNG_FD`, `UPC_RUNG_CHDIR`, `UPC_RUNG_PIPE`, `UPC_RUNG3` and `UPC_RUNG4`. With rung 3 or 4
off, a console write or read would fall into a BPF arm that asks `FD_TABLE` what the fd is, and the table is gone. A
`const` assertion in `main.rs` refuses the combination at build time.

1. **Routing.** With the const on, `open` 15, `read` 5, `fstat` 8, `dup` 10, `close` 21 and `write` 16 alias to 11
   for every fd and every flag. The rows that ask `fd_kind()` (rung 3's "not a file", rung 4's "is the console"),
   the `>= 16` row and both `open` rows (masked flag, off the root) fold into one unconditional row each, so
   `kernel_routed_alias` reads no map. Guarded `!UPC_RUNG56 &&`, so the on object carries none of them:
   - the `open` arm (console name, superblock check, re-entrant dirent scan, `fd_alloc`, `fd_set_inode`) and its
     helpers `name_match_phys` and `name_is`, which then have no caller (review F3: the names);
   - the whole `read` arm, not only its inode branch: its first statement is `fd_kind(pid, fd)`, outside that
     branch, and a guard on the branch alone would have left one `FD_TABLE` read in the on object (review F3);
   - the `fstat`, `dup` and `close` arms;
   - what is left of the `write` arm (rung 3's -1 for a file fd, Phase 7's T-F3 interim);
   - fork's copy of the parent's `FD_TABLE` row (16 map reads and 16 map writes per fork);
   - the accessors `fd_kind`, `fd_set_kind`, `fd_alloc`, `fd_inode`, `fd_set_inode`, `fd_set_offset`,
     `fd_clear_inode`, which then have no caller, and with them the maps `FD_TABLE` and `FD_INODE_MAP`.

   Not guarded, because they read no fd state: the D16 path bound ahead of `open` and `exec`, `mknod` 17 (a success-returning no-op, see point 3), `exec` 7
   (point 7). **D16's 128-byte rule becomes exec-only under the const (review F2).** `open` is aliased to 11 before
   the bound's arm, so the bound never sees it; the kernel's `argstr` reads the path to its NUL through the
   slot's own page check and refuses a path that runs off the slice. A short, terminated path in the slice's last
   bytes, which D16 refused because 128 bytes from its start cross the ceiling, opens. That is safe (the next
   page is refused, `upc_user_pa`), and it is a change a probe asserts the old way: `hostile_path`'s
   `open-ceiling-refused` is replaced in Step 216 by `open-unterminated-refused` (a path with no NUL before the
   ceiling answers -1 with the const off or on). `exit` 2 has had no BPF fd work since D13. `cwd_off_root()` loses its only caller (the `open` row) and
   goes with it; `chdir` 9 still routes under `UPC_RUNG_CHDIR` and the kernel keeps `cwd_mask` for the fork note.
2. **Kernel fd numbers.** A kernel fd is the `ofile` index: `fduser()` is the identity and `argfd()` drops its
   `fd < 16` refusal and its subtraction. D10's +16 ends under the const. The switch is one kernel `#define`
   (`UPC_KFD_BASE` 0 or 16) and mirrored to BPF's const by `parity_slot_tables`, so a kernel and
   an object from different settings fail the parity test, not a boot. How it follows the flip (review F5):
   `upc_slots.h` carries `#define UPC_RUNG56 0` or `1`, `UPC_KFD_BASE` is derived from it, and everything the
   kernel does only under the rung (the console name match, the kernel-given console of point 4, the device
   branches of `fileread` and `filewrite`) is `#if UPC_RUNG56`. A header, not a compiler flag; `Makefile.mbc`
   names it as a prerequisite of the ten objects that include it (found in Phase 12: the kernel build generates
   no dependency files, so without that line a flip of the define rebuilt nothing, and an object with the rung
   on booted against a kernel with it off). Turning the rung on or off is two lines, the const in `main.rs` and the define in the
   header, then the kernel, the userland and the object rebuilt; `parity_slot_tables` reads both lines and fails
   when they differ. `NOFILE` is 16 on both sides, so a process
   has 16 fds where it had up to 32 (16 of BPF's and 16 of the kernel's). Probe lines that say "at or above 16"
   change in Step 216 to "lowest free fd".
3. **[DECIDE] the console, taken by the plan's RECOMMENDATION.** The kernel matches the console by name and makes
   no disk inode for it; `fs.img` and the `ls` golden stay as they are (the golden's one "console" is the kernel's
   `after consoleinit` boot line, not a directory entry). `sys_open` answers a console file (`FD_DEVICE`, major
   `CONSOLE`, `ip` set to 0 explicitly, readable and writable by `omode` as stock derives them) when the path is
   the console's name. The match runs straight after `argstr` and before `begin_op`, so a console open takes no
   log transaction, never reaches `namei`, and never looks at `O_CREATE` (review F5). The name rule copies what BPF's arm does today: strip one leading `./` or `/`, then the
   whole remainder is `console`. A relative `console` or `./console` matches only when the row's cwd is the root, tested
   as `myproc()->cwd->inum == ROOTINO` (the kernel's own inode, the test `sys_chdir` already makes for D15's
   mask; the mask itself is BPF's copy and is not read back) (review F5); `sub/console` is a disk lookup, as now.
   **Shadowing, stated (review Q2).** The device wins, one way, from the root: a disk entry named `console`
   (made by `mkdir`, by `link`, or by `open("//console", O_CREATE)`) is not reachable as `console`, `./console`
   or `/console`. It stays reachable as `//console` or `sub/../console`, and `chdir`, `unlink`, `link` and `ls`
   see the disk entry, because only `open` matches the name. `../console` and a relative `console` from off the
   root are disk lookups. No case gives a program access it lacked: the device is the console every process
   already holds. Three places need a guard for `ip == 0`, because stock code assumes a
   device file has an inode: `fileclose` (no `iput`), `filestat` (answers -1, which is what BPF's `fstat` answers
   for a console fd today), and `sys_open`'s own `T_DEVICE` major check (not reached for the name match). The
   guard is on `ip == 0`, not on the major, and both places that make a console file store 0 there themselves
   (review F4): `filealloc` zeroes nothing and `fileclose` clears only `ref` and `type`, so a recycled
   `struct file` carries the last file's `ip`, and a console file that inherited one would `iput` an inode
   another slot may hold. The major is not the test because an on-disk console node (the parked ADR) would have
   the same major and a real inode to release. A read on that file made inside a tagged trap
   (`upc_slot_active()`, which is true for slot 0 too) goes to `upc_consoleread` (the polled, never-sleeping
   read of D20), not to `devsw[CONSOLE].read`: `consoleread` sleeps, and slot 0 is the one row the kernel's
   `sleep()` does not refuse (`upc_is_slot_row()` leaves it out), so a first image's `read(0, ...)` would have
   slept with nothing runnable (review F1). The test lives in `fileread`'s device branch; `sys_read`'s own
   shortcut (D20) is dead under the const and goes with the +16. A write goes to `consolewrite` from
   `filewrite`'s device branch, which keeps D19's rule there (a copy failing before the first byte answers -1,
   where stock passes `consolewrite`'s 0 through); `sys_write`'s shortcut is dead the same way (review F5). `mknod` stays BPF's no-op: an on-disk device node, `mknod` and
   `T_DEVICE` inodes are a later ADR, parked in the progress file.
4. **Slot 0 and the first image's console (restated; see Departures).** The plan has init open the console through
   the kernel like any process. Two facts of the tree stand in the way:
   - `user.ld` puts `.rodata` straight after `.text` from 0, so the literal `"console"` in `init.c` and `uinit.c`
     sits in init's first page, and the kernel's copy-in refuses slot 0's first page (D6, D16). A kernel `open`
     from init with that literal answers -1. `init.c` shrugs and dups a closed fd; `uinit.c` exits 1. This is the
     "slot 0 literal limit" already filed for Stevie, and this rung must not depend on its answer.
   - Thirty-three of the thirty-six gate programs boot as the first image (`--userland gate2.mbc`,
     `getpidfork.mbc`, nearly every probe) and print on fd 1 without opening anything; only `uinit`, `devprobe` and
     `crprobe` open the console. BPF's write never looked at the fd, and rung 3 kept that by testing "not a file".
     With the kernel's table the only one, fd 1 of such an image is an empty slot.
   - A read-only `open` with a literal path made by the first image itself was BPF's (BPF reads the bytes with
     no copy-in) and becomes the kernel's, so it meets the same first-page refusal. Nine gate programs call
     `open` with a literal (`uinit`, `devprobe`, `mkprobe`, `rmprobe`, `lnprobe`, `hostile_ptr`, `fdprobe`,
     `chdirprobe`, `bigprobe`, counted by `grep`). D6 records that the rung probes make their kernel-routed calls
     from a forked child; `devprobe`'s `open("console")` is made from slot 0. Step 216 audits each: a call made from slot 0 takes its path from
     `.data`, as `crprobe` already does.

   Design: **the kernel gives the first image its console.** `upc_slots_init()` (after `userinit()`, before the
   scheduler first runs init; `fileinit` and `consoleinit` have run) allocates one console file (opened read and write, `ip` 0) and puts it in
   `proc[0].ofile[0..2]` with three references, under `#if UPC_RUNG56` only. This is what Linux does for pid 1
   (`console_on_rootfs`). Then: every first image has stdio; `init.c` is not edited (its `open` answers -1 for the
   literal, `mknod` answers 0, the second `open` -1, and its two `dup(0)` answer 3 and 4); `uinit.c`, which is
   ours and exits when both opens fail, keeps the console's name in `.data` (one line, as the probes' tokens
   were moved), so its `open` answers 3 and its dups 4 and 5. Children inherit by the fork note as now. The cost
   is two or three spare console fds in every process of a boot, out of 16 (the corpus is expected to hold at most
   a directory and one entry open at a time, in `ls`; the gate shows it). The alternative, kept on record: no kernel-given console, the name in `.data` in `init.c` and
   `uinit.c`, and a three-line open in each of the thirty gate programs. It is the purer xv6 shape and it is thirty
   probe edits plus an edit to `init.c`; it also leaves every future first image one forgotten
   preamble away from printing nothing.
5. **Fork, exit, kill.** Nothing moves. The fork note (24) already `filedup`s the parent's `ofile` into the child's
   row and closes what a forged reply left there; exit closes the row (D13); kill closes it (D8). The BPF copy of
   `FD_TABLE` at fork goes (point 1).
6. **`open(O_RDWR)` on a regular file is a kernel file.** T-F3 closes: read and write go through `fileread` and
   `filewrite` on one `struct file`, and a child shares the offset with its parent (D10's stale-offset limit
   ends). Phase 7's BPF refusal goes with the compiled-out arm. R-P1 (BPF's read writes a full 16-byte chunk on a
   short read) ends the same way: `readi` copies what it returns. A read is no longer capped at 16 bytes a call:
   `cat`'s 512-byte reads come back whole, so it makes fewer calls; `ls` asks for 16 and gets 16.
7. **Exec stays a BPF resident-program lookup** (departure, GROUND TRUTH). It reads no fd and no table; its path is
   still bounded by D16's arm. The exec'd image keeps the row's `ofile`, as it kept BPF's row.
8. **After this rung `dup` can land on 0-2**, which is what ush pipelines and redirection wait on. The shell
   grammar is not this plan's (Step 241).

**Rung 4's offer, restated for this rung.** BPF offers the next keyboard byte in `kbd_in` at the injection of a
read "on a fd below 16", which meant "the console" because only console reads below 16 were routed. With every
read routed and every fd below 16, the test becomes `a7 == 5` alone: a byte is offered on every routed read and
taken only inside `upc_consoleread`. The ring still moves only on `UPC_REPLY_KBD`, so a file read with input
pending takes nothing and loses nothing. Rung 3 needs no such change (it carries no state across the trap).

**What the user sees change.** fd numbers for files and pipes start at the lowest free fd, not at 16. A read
returns up to n bytes. `fstat` on a file is the kernel's `stati` (16 bytes under ilp32e, size at offset 12: Gate
D's lesson, to be checked on the first `ls`). A write on a closed fd 0-2 answers -1; BPF printed it before rung 3
and the kernel printed it after ("not a file"). A relative `console` opened off the root is a disk lookup (today:
the same, by D15); `/console` from off the root is the device (today -1, because D15 sends it to `namei`).

**Departures from the plan's wording.**

| Plan point | Tree | D21 |
|---|---|---|
| 1: "the write inode refusal" is guarded | It is what is left of the write arm since rung 3 | Same guard, named as the arm's remainder |
| 1: exit's close in BPF | None since D13 | Nothing to guard |
| 1: only the arms are listed | `kernel_routed_alias` itself reads `FD_TABLE` twice (rungs 3 and 4) | Those rows fold into the unconditional ones |
| 3: the name match | BPF matches after stripping one `./` or `/`, at the root only by way of D15 | The same rule, written out, with `/console` off the root as the one change |
| 4: "its row is the real `userinit` process, so `ofile` is its own" | True, and empty; init's literal is unreadable to the kernel; thirty first images never open a console | The kernel gives the first image its console |
| Rung 4 | `kbd_in` is offered for `fd < 16` | Offered on every routed read |
| not in the plan | `upc_consoleread` is the slot row's console read; `consoleread` sleeps | `fileread` on the console from a slot row calls the polled read |

**Budget and run cost, expected, to be measured at Step 223.** The open walk and the file read are the two
largest ascend-only arms (the comments in `main.rs` call them that; D16 measured +48,914 when a refused state was
walked through the open and exec arms, against +2,261 ahead of them), so the delta is expected well below zero, tens of
thousands of verified instructions, not a few thousand as rungs 3 and 4 gave (-14,552 and -2,007). Against that,
every `open`, `read`, `fstat` and `close` becomes a kernel trap. Rung 3 measured about 3,000 MBC instructions for a
`write` call; a kernel `open` walks the path through `namei`, `ilock` and the buffer cache, and `ls` makes an
`open`, an `fstat` and a `close` per entry. The twelve corpus boots must still finish inside 4,000,000 triggers
(Step 179's kind of question); this is the measurement most likely to need a decision.

**Questions the review (Step 210) is asked to press.** Does any BPF path still read `FD_TABLE` with the const on
(the routing rows, the TTY drain, exec, `cwd_off_root`)? Can a disk entry named `console` shadow or confuse the
name match (`mkdir console`, `link` to that name, `O_CREATE`)? What does slot 0 hold in `ofile` before its first
open, and is `upc_slots_init()` late enough for `filealloc`? Is the kernel-given console sound when slot 0 exits
or is killed, and when a slot is reused? Does D7's retry keep a0 for a read or write on a file (it must not
re-run a write that landed)? Does a `struct file` with `ip == 0` reach any `ilock` or `iput` besides the three
guarded places (`filedup`, the fork note, `upc_close_row`, `kexit`)? Which goldens can move, and why?

**Review (Step 210, 2026-11-15) and dispositions.** An independent read-only review read this design at
`ae44692b` against the code; nothing was built or booted by the reviewer. Each finding was re-read in the tree
by the lane before its disposition (file and line are the reviewer's, checked). Its ids carry `F` (findings) and
`Q` (the question it answers with no problem found).

| Id | Severity | Finding | Disposition |
|---|---|---|---|
| F1 | High if built as worded | Point 3 sent a read "from a slot row" to `upc_consoleread`. `upc_is_slot_row()` leaves slot 0 out (`upc_slots.c`), and `sleep()` and `sched()` panic only for that predicate (`uproc.c`). A device branch gated on it would send a first image's `read(0, ...)` to `consoleread`, which sleeps with nothing runnable. No gate would have caught it: `crprobe` reads in a forked child | **Accepted; design amended.** The test is `upc_slot_active()`, as `sys_read` uses today. A slot 0 read is added to the probes (`crprobe`: the first image reads a line itself before it forks) |
| F2 | Medium (a probe flips, safely) | `hostile_path` puts `README` at 0x007FFFF8 and expects -1: D16's 128-byte rule, enforced in BPF ahead of the `open` arm. With `open` routed, `copyinstr` finds the NUL inside the slice and `namei` opens README | **Accepted; design amended.** D16's rule is exec-only under the const; the probe line is replaced by one that proves a refusal either way (a path with no NUL before the ceiling) |
| F3 | Medium | The `read` arm keeps an `FD_TABLE` read: `fd_kind(pid, fd)` heads the arm, outside the inode branch that point 1 guarded, so the map would stay in the object. The helper is `name_match_phys`, not `component_match`; `name_is` loses its only caller too | **Accepted; design amended.** The whole arm is guarded. Step 240 checks the on object holds neither map |
| F4 | Medium (plausible) | A recycled `struct file` carries a stale `ip`: `fileclose` clears only `ref` and `type`, `filealloc` zeroes nothing. Close any file, then open the console: if the console path does not set `ip`, the `ip == 0` guards miss and close `iput`s an inode another slot may hold | **Accepted; design amended.** Both places that make a console file store `ip = 0`; the guards stay on `ip == 0`, not the major (an on-disk console node would share the major and have an inode) |
| F5 | Low (left to guess) | Console open mode; what gates the kernel-given console and how the kernel's define follows the const; where D19's -1 lives once `sys_write`'s shortcut is dead; whether the name match precedes `begin_op`; how "cwd is root" is tested | **Settled above.** Mode by `omode`, as stock. `#define UPC_RUNG56` in `upc_slots.h`, checked against the const by `parity_slot_tables`. D19's -1 moves to `filewrite`'s device branch. The match runs after `argstr`, before `begin_op`. `cwd->inum == ROOTINO` |
| Q2 | No problem | A disk entry named `console` | The device wins, one way, from the root; stated in point 3. No case gives access the program lacked |
| Q3 | No problem | Slot 0's `ofile`; exit, kill, slot reuse | Empty before (`userinit` opens nothing), one file with three references after; `consoleinit` and `fileinit` run first. Exit closes the row; kill of pid 1 is refused; slot 0 is never reused (fork scans from 1), and a reused row is closed, then filled |
| Q4 | No problem | D7's retry and a write that landed | Only pipes, the console read and pause retry. A file read or write finishes in one trap, so the offset moves once; `pipewrite` retries only before its first byte; the keyboard ring moves once; a0 is not stored on a retry |
| Q6 | No problem | Reference counts across fork, exec, kill | Balanced: fork's -1 precedes any note, exec is BPF-only and keeps the row, kill closes the target's row |
| Q7 | Noted | Which goldens can move | `fs.img` holds init, sh, ls, cat, echo and wc; the probes are outside it, so the claim that `fs.img` and the `ls` golden stay holds. What could move: `ls` and `cat BIGFILE` by trigger exhaustion; `ls` rows by `stati`'s layout; `uinit`'s `echo hello` if its `.data` edit is missed; all twelve without stdio. A moved golden is a stop, not a recapture |
| Q8 | No problem beyond F2 | Isolation with fds no longer offset and `dup` onto 0-2 | `argfd` is bounded to the caller's own row; copies are page-checked per slot |

**Built and measured (Phase 12, 2026-11-15): the rung works, and the goldens' PC trace stops it.** The wiring
is in the tree with the const off (`cf11267b`): the off object is 883,522, equal to before, and the eight corpus
boots are SAME. Turned on (the const and the define, nothing else; not committed):

- **Verifier:** 529,305 verified instructions, **-354,217** against 883,522 (headroom 370,695). The object file
  goes from 897,784 to 729,216 bytes. No instruction of the on object refers to `FD_TABLE` or `FD_INODE_MAP`
  (the only relocations against them are debug info and BTF); the two map definitions stay declared, because a
  `#[map]` static is always emitted, so the loader still creates two maps nothing reads.
- **Probes, all pass:** `rwprobe` 5 (a plain `O_RDWR` file is written, read back and restored; `dup` lands on
  0; `fstat` on the console answers -1 and the kernel goes on), `fdprobe` 51, `chdirprobe` 24, `pipeprobe` 20,
  `bigprobe` 3.
- **Run cost:** all twelve corpus boots finish inside the existing 4,000,000 triggers. A boot's instruction count
  is where rung 4 left it (`ls` 63.57 M against 63.26 M): the prompt's wait is what it counts, not the opens.
- **Goldens:** eleven of the twelve boots are byte for byte their golden **outside the 100-PC trace**, and their
  `forks`, `waitpid` and `tty_r` are the golden's. Inside the trace every one differs from the 21st PC on.
  `uinit_echo` differs outside it too: `uinit.c` exits when both its console opens fail.

Why the trace moves, which neither this design nor its review saw: the Phase 1.4 tracer (`TRACE_HEAD`) is armed
for 100 instructions at the first return to U-mode and prints every PC it executes, the kernel's included, and
those bytes are part of every golden's TTY. Until this rung no trap inside that window went to the kernel: init's
`open` and its two `dup`s were BPF's, and its first `write` traps after the window has closed. Under the rung init's
`open("console")` is the 21st instruction and traps into the kernel, so kernel PCs fill the rest of the window;
and the open answers -1 (the literal sits in the first page), so init takes its `mknod` path and its user-mode
PCs differ as well. Point 4's sentence that this rung does not wait on the slot 0 literal decision does not hold
for the goldens.

A moved golden is a stop, not a recapture. The const and the define went back to off (A7); the off object reads
883,522 again; Stop F stands, with the wiring left in the tree. Two ways on, both measured, for Stevie:

| Way | What it takes | Measured |
|---|---|---|
| No recapture | (a) The kernel's `copyinstr` may read slot 0's first page (reads only): the slot 0 literal question already filed, answered yes. (b) Under the rung the tracer prints U-mode PCs only | Booted, not committed: **12 of 12 SAME**, `uinit_echo` included, with `init.c` and `uinit.c` untouched. 532,152 verified instructions (the tracer test is +2,847) |
| One named recapture | The twelve goldens' trace segment recaptured once, by name and with its reason, as the plan allows for Phase 15's `ls`; `uinit.c` takes its console name from `.data` (which moves its own trace) | Eleven boots identical outside the trace; nothing else needed |

A third, larger change is to retire the tracer from the ascend build (it is a Phase 1.4 bisect aid): also a
recapture.

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
| T-F3 | Low to Medium, pre-existing | Plain `O_RDWR` on a regular file loses writes silently; `write` does not refuse a negative fd today | Stated in D10 and D11. No code: BPF `open` cannot tell a file from the console before the walk. For Stevie. **Interim 2026-11-15, S-STAGE-2C Phase 7 (D19): BPF refuses a write to a file fd below 16 while `UPC_RUNG3` is on (`fdprobe` `rdwr-file-write-refused`, `readme-intact`); the write lands in the kernel at rung 6 (Phase 12)** |
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

## Probes booted 2026-11-15 (S-STAGE-2C Phase 8, A-62)

Each of these was read from the code and not booted when this ADR's review was written. They are booted now and in the harness (`scripts/upc-regression.sh`); the numbers are in `references/s-stage-2c-log.md`, Phase 8.

- **Unlink of a file with data** (`rmprobe`, RM 11): the blocks are freed and the next file to take them reads back its own bytes.
- **Link's count read back** (`lnprobe`, LN 12): `fstat` gives nlink 2 after `link`, 1 after the first name is unlinked, and the bytes are still there under the second name.
- **Pause's tick count** (`sleepprobe`): `pause(50)` returns no earlier than 50 ticks later; how late it woke is printed as a note.
- **A file past one block** (`bigprobe`, BG 3): 14,000 bytes, past the direct blocks into the single-indirect block; written by the kernel, read back by BPF.
- **Bad pointers for unlink, link and chdir** (`hostile_ptr`, HU 9): a path at the user ceiling answers -1 and creates nothing.
- **The rung 0 asserter against a probe kernel**: ADR-109 I1-I8 hold on every tagged trap of `getpidfork` and `forkloop` (141 traps), and a changed pid fails. With rung 3 on, kernel `printf` and user bytes interleave on the TTY character by character; `scripts/upc-tty-split.py` separates them (kernel `printf` stores a word per character, a user write one byte).

**Found by `rmprobe` (R-P1, Low, open).** BPF's file read copies a fixed 16-byte chunk into the user buffer whatever count it returns, so the last short read of a file writes up to 15 bytes past the end of what it reports, with bytes from the data block past the file's size. The write stays inside the caller's slice (`user_phys(pid, buf_va, READ_CHUNK)`), so no other slot is touched; a program that reads into an exact-size buffer has the bytes after it overwritten. Not fixed in BPF (the guarded copy is what the code comment says doubled the verifier's states). It closes when file reads move to the kernel (rungs 5+6): `rmprobe` prints the count as a note and it must read 0 then.

## Security pass review of S-STAGE-2C (2026-11-15, plan Step 293): PENDING

The plan's close runs from Stop F plus `cd` in ush (Stop I): rungs 3 and 4 and the fork mark on, rungs 5+6 wired and off, rung 7 not built. The attack-surface table for what is on is in `references/s-stage-2c-log.md`, "Phase 16, security pass"; every row names a harness line, and two rows that had none got one (the opcode scan's PUSH, POP, INT and IRET; the `.data` sidecar refusal).

**The independent read-only review of the sprint's diff (kernel C parsing user pointers, BPF guards that might not fold, D6 and the priv-3 test, the opcode scan's coverage, the sidecar refusal) has not reported yet.** Its findings and their dispositions go in this table. Until they do, this ADR's security pass is not complete.

| # | Finding | Severity | Disposition |
|---|---|---|---|
| (pending) | | | |

## Alternatives considered

- **BPF serves sbrk itself.** One arm and no reply word, but kill and blocking still need a way back, so the channel
  arrives anyway one rung later, and the arm costs more than an alias row.
- **Kernel `sleep()` with a real context switch.** `sched()` from S-mode breaks I7 and moves scheduling authority to
  the kernel. That is rung 7's direction, not this ADR's.
- **A heap above the low band.** It needs a third fork copy band, and fork's copy loop is verifier-sensitive.
- **Walking BPF's pgd in the kernel.** Two formats for one table; each child's map is two superpages, so an offset
  gives the same answer.
