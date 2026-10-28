# EVOLUTION-1 Stage 2 log

Started 2026-10-28 on 2e060148 (branch sprint/evolution1-stage2, cut from sprint/ascend-2).
Plan: docs/battle-plans/EVOLUTION-1-STAGE-2-battle-plan.md (draft, Phases 0-2).

## Phase 0 (Steps 1-22)

- Baselines: harness ALL GREEN (ascend 924,950, Doom 758,199, cart 42,743 / 146,897), sweep 8/8 SAME, `go test ./...`
  0 failures. Known failures: none.
- `getpidfork`: flag OFF `parent-pid=1 fork-ret=2 child-pid=2 wait-ret=2 parent-after-wait`; flag ON the same except
  `child-pid=other`. As the plan assumes.
- Hypotheses: H1 `usertrapret` writes `sscratch` from `p->trapframe` (utrap.c:148): match. H2 `uservec` reads
  `sscratch` and saves 17 registers plus a0 (via mscratch) into it: match. H3 BPF never writes the `sscratch` word:
  match. H4 `scheduler_context_switch` writes nothing in the CSR window: match. H5 S-mode is flat (main.rs:3630):
  match. H6 BPF `wait` writes 0 to `*status` (no per-process exit status yet): amended, rung 7 must add it. H7 no
  timer interrupt reaches the ascend kernel (the timer path is `cfg!(not(feature = "ascend-linux"))`): match.

## Phase 1: paused

Not completed this session; the tree is back to the Phase 0 state (probe reverted, `GETPID_INJECT` false, kernel
`4e39c82f…`). Resume at Step 25.

## Side findings (2026-10-28)

- **Half the trigger frames become CPU ticks: localized.** One boot (200,000 triggers + a 2 x 20,000 hijack probe):
  `veth-upc0p` sent 240,000 frames and `veth-upc0` received 240,000, so nothing is lost on the link; the probe counted
  10,000 ticks per 20,000 frames. The loss is inside `monad_cpu` between receive and `STAT_CPU_TICKS`. Leads: the
  entry point's `XDP_TX` tail-call chain (`TAIL_ROUND`, main.rs ~480) and the fact that `veth-upc0`'s lifetime RX
  "otherhost" counter is exactly half its RX packets (90,673,000 of 181,346,055). Not debugged further.
- **Bootstub retirement: not done, needs Stevie.** `crates/upc-bootstub` is stage 1 of UPC Boot Protocol v2 (frozen,
  ADR-067), described in `docs/doom/UPC_BOOT_PROTOCOL_V2.md`, ADR-080's `BootProtocol` stub variant and bootctl's
  `--bootstub`. Removing it changes a frozen spec's two-stage boot, so it is a decision, not cleanup (S-ASCEND-2
  Steps 281-282 stay open).

## Phase 1 (S26, Steps 23-42)

**-1 hypotheses (Step 24).** M1 `syscall()` reads a wrong a7 and takes a handler other than getpid, or the
unknown-syscall branch (a0 = -1). M2 `myproc()` returns a row other than the child's (init's), so the child's
registers and result go through init's trapframe. M3 the result is written to a trapframe the child's return path
does not restore from. M4 `usertrap` takes a non-syscall branch. Probe output uses decimal values terminated by `;`:
the PC-tracer strip (`([0-9A-F]{4} )+`) ate digits of the first, unlogged probe's space-separated hex values, which is
why init's pid read as 0 there.

**-1 explained (Steps 28-31).** Probe (decimal, `;`-terminated) with the flag ON, `getpidfork`, 900,000 triggers:

```
UT:idx=0;pid=1;tf=8118272;ss=8118272;a7=11;sc=8;      parent getpid
SC:num=11;fn;ret=01;
UT:idx=0;pid=1;tf=8118272;ss=8118272;a7=3;sc=8;       child getpid
SC:num=03;fn;ret=-01;
```

Both traps resolve `myproc()` to `proc[0]` (init) and save into init's trapframe (`sscratch` = `p->trapframe` =
8118272). The child's trap reads `a7 = 3`, so the kernel runs `sys_wait` for init, which has no kernel children:
-1. Why a7 is 3: x17 does not exist under ilp32e; the translator spills it to RAM byte `0x64004`
(`crates/monad-mbc/src/translator.rs:23`). User code reaches that through its per-pid slice, but `uservec` runs in
flat S-mode and its `sw a7, 84(a0)` reads physical `0x64004`, init's identity-mapped copy, which still holds 3 from
the parent's preceding `wait()`. So M1 and M2 both hold, and the a7 read is the deciding detail. With the flag OFF
the kernel is never entered (0 `UT:` lines). Probe reverted: tree clean, kernel `4e39c82f…`, baseline lines back.

**What rung 0 must change (Step 39).** (1) `uservec` must save into the trapping slot's trapframe; (2) `myproc()`
must return the trapping slot's row; (3) the row must exist on, or be created at, the slot's first trap; (4) the
spilled registers (x16 at `0x64000`, x17 at `0x64004`) must be read from the trapping slot, not from physical
memory: either BPF writes the slot's a7 (its `regs[1]`) and x16 (`regs[2]`) into the trapframe at injection, or
`uservec` reads them through the slot's slice. Item 4 is new: ADR-109 must cover it.

## S26 Phase 5: usertests probes, and a BPF wait bug they found

`user/ut_exitwait.c`, `ut_twochildren.c`, `ut_forktest.c`, `ut_reparent2.c` (agent, merged `0432192f`), extracted
from xv6 usertests with fixed-string verdicts; adaptations in each file header (exitwait drops the exit-status check
because BPF wait writes 0; reparent2 runs its body in a child and reaps like init.c).

First boots, flag OFF: exitwait and twochildren `ok`; forktest printed `forks=5` and hung; reparent2 printed `ok`
and hung before `reaped-all`. Cause: BPF `wait` set "children exist" for every child whose `PARENT_OF` matched,
reaped ones included, so once a process had reaped all its children `wait()` rescheduled for ever instead of
returning -1 (xv6 init's reap-until-empty loop can never finish). Fix: collect a child mask in the scan and treat
"no unreaped child" as no children. A first version that put the reaped test in the loop condition verified at
963,613 (over 950,000); the mask form verifies at **921,036** (-3,914). All four probes pass and are harness gates;
harness ALL GREEN, sweep 8/8, Doom 758,199 unchanged. forktest's fork limit is 5, not 7: only pids 1-5 get memory
slices under fork's ceiling (S-ASCEND-2 Phase 4).

Also fixed: `kernel/types.h`'s uint64-width assert (S-ASCEND-2 Phase 7) broke the host `mkfs` build on x86-64; it now
applies only when `__riscv_xlen == 32` (`d8d5134f`); the kernel image is unchanged.
