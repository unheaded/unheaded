<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# EVOLUTION-1 Stage 2 — Migration Ladder (skeleton)

**Date**: 2026-10-28
**Status**: SKELETON. **E2 decided 2026-10-28: (a)**, the full ladder, rung by rung with the stop rule. **E1 decided: option K** (init = 1; BPF reports slot + 1), so rung 0 gives the kernel a process per BPF fork under that numbering. Phases, gates and prerequisites only.
**E2 answer**: *pending* (S-ASCEND-2 Step 56). Slot for Step 63: "DECIDED <date>: (a) push | (c) minimal".
**Cites**: M-221 (decision 45); [EVOLUTION-1-TRAP-FLIP.md](EVOLUTION-1-TRAP-FLIP.md) §D1 reach re-read, §Stage 2, Appendix B (state-ownership matrix); [S-ASCEND-2-battle-plan.md](S-ASCEND-2-battle-plan.md) Phases 1-2; sprint log `references/s-ascend-2-log.md`.
**Prerequisite**: S-ASCEND-2 Phase 1 (pid numbering decided by E1), Phase 3 (`gate_nway` 3x `NWAY-ok` and the forkloop gate, both in `scripts/upc-regression.sh`) and Phase 7 (device inodes; FS write route decided at Step 182) done. Pid reconciliation recorded. `gate_nway` green in the harness.
**Forge**: its own Warmonger session at kickoff, with a four-lens panel (Architect, Developer, BlackMage, Scientist) reviewing this skeleton before any step is written.

> **NO NUMBERED STEPS HERE — forge at kickoff.**

---

## Why a rung 0

With `GETPID_INJECT` ON, a BPF-forked child's getpid returns −1 (S-ASCEND-2 Phase 1). The
owned kernel's `proc[]` holds only init: user `fork()` is serviced in BPF and only
`userinit` calls `allocproc`. sh and every program it runs are BPF-forked, so any syscall
routed to the kernel from them has no kernel process behind it. Every rung past getpid, and
every arrival, needs a kernel row per BPF fork first. This holds under either E2 answer.

## Budget and measurement rules (every rung)

- Working limit 950,000 verified insns for the ascend object; ceiling 1,000,000. Today OFF
  926,542, getpid ON 911,319 (October toolchain, host kernel 6.17.0-41).
- One guard const per rung, the `GETPID_INJECT` shape. Rollback is that const.
- Freed insns = OFF − ON for the rung's guard, measured as a pair on one toolchain and one
  host kernel in one session. The July → October drift moved the getpid delta by 20,632 with
  no logic change; numbers from different builds are not compared.
- Cycle cost per call recorded (MBC `insn_count` delta for a fixed probe). Getpid's round
  trip costs +1,348 MBC instructions.
- Doom (non-ascend) 761,580 unchanged; the 12-boot corpus and its trigger budgets unchanged
  unless the rung's gate says otherwise.

**Rung gate (template; each rung below names its specifics):** corpus 12/12 SAME (or a
policy recapture recorded with reason and HEAD), harness ALL GREEN, freed insns recorded
and ≥ 0 (arrivals: added insns recorded, total ≤ 950,000), cycle cost recorded, the rung's
usertests subset green, rollback const verified by one OFF build.

**Stop rule (E2 = (a)):** a migration rung with freed insns < 0 stops the push at that
rung. It and the later migration rungs stay in BPF unless authenticity requires them, which
is (c) for the remainder. Arrivals continue.

---

## The ladder

EVOLUTION-1's order (cheapest state first), with rung 0 added in front.

### Rung 0 — Kernel row per BPF fork (shadow `proc[]`)

On each BPF `SYS_FORK` the kernel gets a `struct proc` for the child: pid per E1, trapframe
and pagetable matching the PROC_TABLE slot (ADR-074 per-pid pgd). The injected trap
resolves `myproc()` to the trapping slot's row. Exit and wait free the row. BPF stays
authoritative over scheduling and lifetime.

- **Prerequisite**: E1 answered; the source of the −1 isolated (S-ASCEND-2 Step 47 dump).
- **Gate**: `getpidfork` with the flag ON prints a child pid equal to `fork()`'s return in
  the parent (numbering per E1); ON and OFF print the same `GF:` lines; `gate2`, `gate_nway`
  and forkloop still pass; forkloop's three iterations leave no leaked row.
- **usertests subset**: none (no stateful kernel syscall yet).
- **Also closes**: Stage 1. `GETPID_INJECT` can default ON only after this gate.

### Rung 1 — getpid ✓

Trap path proven (S5 Phase 3). Default flip waits on E1 and rung 0.

- **Gate**: `getpidprobe` and `getpidfork` print the same lines ON and OFF; the flag's
  default flips to ON; freed insns recorded on the build of the flip (−15,223 on the
  October toolchain).
- **usertests subset**: none.

### Rung 2 — uptime / pause

`pause` (13) is xv6's sleep(ticks); BPF services it today as a cooperative yield
(`main.rs` `syscall_nr == 13`). `uptime` (14) is not BPF-implemented, so it arrives.
The "sleep" arrival in EVOLUTION-1's list is the same syscall number as `pause`: reconcile
at kickoff.

- **Prerequisite**: rung 0; the timer interrupt reaches the kernel (`clockintr` advancing
  `ticks`). Unverified.
- **Gate**: template; a probe prints `uptime` increasing across a `pause(1)`; a yielding
  program still lets its sibling run (`gate_nway` co-scheduling unchanged).
- **usertests subset**: none.

### Rung 3 — write (console fds)

Console write is per byte and the hottest path; D1 (c) keeps it in BPF for that reason.
Under (a) it routes devsw → `uconsole` → console-mmio. File-backed writes do not move here:
the fd table lives in `FD_INODE_MAP` until rung 6, and D1 requires the FS-metadata bundle to
move as one unit. FS writes and `>` (master plan 1.3.2) take this route if Step 182 decides
"kernel-side via Stage 2".

- **Prerequisite**: rung 0.
- **Gate**: template; the corpus completes within the harness's existing trigger budgets,
  or the new budgets and the per-byte cycle cost are recorded and accepted.
- **usertests subset**: none until FS writes exist; then `writetest`, `createtest`,
  `truncate1-3`, `writebig`, `bigwrite`.

### Rung 4 — read (console fds)

Console read moves from the BPF KBD ring to `consoleintr`; the KBD ring becomes the
interrupt source (Appendix B, console-in row). File reads move with rung 6.

- **Prerequisite**: rung 3.
- **Gate**: template; `upc-bootctl console --stdin` input reaches sh; uinit's
  `echo hello` golden SAME.
- **usertests subset**: none.

### Rung 5 — fstat / dup / close

Lands as one unit with rung 6 (D1: splitting the FS-metadata bundle is an FD-table split
brain). Kickoff picks one of: one gate for rungs 5+6, or rung 5 staged dark behind its guard
until rung 6 lands.

- **Prerequisite**: rung 0; ilp32e `struct stat` layout (16 B, size at offset 12) kept.
- **Gate**: template, run jointly with rung 6.
- **usertests subset**: none alone.

### Rung 6 — open (fs_walk hand-off) and file read/write

`readi`/`namex` become the read path; in-BPF `fs_walk` and `FD_INODE_MAP` retire; the
kernel's `ofile` becomes the fd table. Device inodes open through devsw: S-ASCEND-2 Phase 7
found none on disk (the console is matched by name in BPF; mknod is a stub), so mknod and
on-disk T_DEVICE inodes arrive here. FS writes and `>` (master plan 1.3.2) take this rung,
per decision 48 (S-ASCEND-2 Step 182).

- **Prerequisite**: rungs 0 and 5; Phase 7 done.
- **Gate**: template; the four read-path goldens (README direct blocks, BIGFILE
  single-indirect, `sub/NOTE` path walk, `ls sub`) SAME; freed insns include `fs_walk`.
- **usertests subset**: `opentest`, `copyinstr1` (bad-address open; kernel `copyinstr`).

### Rung 7 — fork / exec / wait (PROC_TABLE hand-off)

`proc[]` becomes the authority; PROC_TABLE derives from it or retires; the ADR-074 pgd hook
re-anchors on the kernel's fork.

- **Prerequisite**: rung 0 (rows exist), rungs 5-6 (`ofile` copied on fork), E1; the
  `gate_nway` and forkloop gates green; wait passes the exit status (`usertests`' `run()`
  reads it; unverified today); the BPF PROC_TABLE (8 slots) vs the kernel's `NPROC` (64) bound
  decided.
- **Gate**: template; `gate2`, `gate_nway`, forkloop, uinit gates green. Init's `exit`
  now panics as stock xv6 does ("init exiting") instead of returning to user code
  (S-ASCEND-2 Phase 1 observation): gates that let init exit change in the same commit.
- **usertests subset**: `exitwait`, `twochildren`, `forktest`, `forkfork`,
  `forkforkfork`, `reparent2`; `exectest` once FS writes exist.

### Rung 8 — arrivals: sbrk, pipe, kill, mkdir, unlink, link, chdir, sleep

Kernel paths exist (Phase 2.4 T3/T4) and were never BPF-implemented: these arrive, there is
no handler to retire. Each arrival is its own gate. Pipes (master plan 1.3.3) arrive here,
per decision 48 (S-ASCEND-2 Step 182).

- **Prerequisite**: rung 0 (every arrival runs in a forked child). `sbrk`: memory authority
  (`growproc`, `vmfault`) against the per-pid pagetable. `mkdir`/`unlink`/`link`: FS writes
  (block allocation). `sleep`: see rung 2. `kill`: the kernel can mark a BPF-scheduled
  process killed and BPF honours it.
- **Gate**: template with added insns in place of freed insns.
- **usertests subset**: `sbrkbasic`, `sbrkmuch`, `sbrkarg`, `bsstest` (sbrk); `pipe1`
  (pipe); `killstatus` (kill); `dirtest`, `linktest`, `unlinkread`, `subdir`, `rmdot`
  (mkdir/unlink/link/chdir).

---

## usertests subset plan

`user/usertests.c` (3,248 lines) is not built today (not in `Makefile.mbc-userland`). Its
runner forks each test and reads the exit status through `wait`; `countfree()` calls
`sbrk`. So:

1. **Before sbrk and exit status**: each named test body is extracted into a gate2-style
   probe (`user/ut_<name>.c`, fixed-string verdict), one harness gate per probe. The rung
   that makes a test reachable adds its probe.
2. **After sbrk, pipe and exit status**: build `usertests`; gate `usertests <name>` per
   test.
3. **Last**: `usertests -q` as one gate; the full run after that is a measurement, not a
   gate.

| Rung | Tests that become gates |
|---|---|
| 0, 1, 2, 4, 5 | none (probes only) |
| 3 (with FS writes) | writetest, createtest, truncate1-3, writebig, bigwrite |
| 6 | opentest, copyinstr1 |
| 7 | exitwait, twochildren, forktest, forkfork, forkforkfork, reparent2, exectest |
| 8 | sbrkbasic, sbrkmuch, sbrkarg, bsstest, pipe1, killstatus, dirtest, linktest, unlinkread, subdir, rmdot |

---

## If E2 = (c) minimal

Step 63 trims the ladder: rungs 0, 1 and 8 stay; rungs 2-7 are marked "not planned". Open
for E2 to settle: D1 (c) as decided on 2026-07-17 already places getpid/pause/yield and the
FS-metadata bundle (rungs 2, 5, 6) in the kernel, which is more than arrivals only.

## Open questions for kickoff

- How the injected trap finds the slot's row: BPF writes `cpu->proc`, or the kernel looks up
  the slot.
- Where the −1 comes from (which trapframe `uservec` saved into, what `myproc()` returned).
- Whether BPF `wait` passes the exit status today.
- Whether the timer interrupt reaches the kernel (`sleep`, `preempt`).
- Per-byte write cycle cost against the corpus trigger budgets (rung 3).
- Toolchain pinning for the sprint so ON/OFF pairs are comparable across sessions.
- BlackMage lens: kernel C now parses user pointers (`copyin`, `copyinstr`, `argstr`)
  where the BPF verifier bounded every access before. Each rung adds a hostile-input probe
  for its syscall.

## Stage 2 exit (whole ladder)

Every planned rung gated green; freed and added insns tabulated with the build each was
measured on; the usertests gates in the harness; EVOLUTION-1 Stage 2 status written;
master plan Epic 1.6 closed.
