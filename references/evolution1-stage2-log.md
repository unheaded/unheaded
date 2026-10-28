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

## Phase 2: ADR-109 (Steps 43-50)

`docs/adr/ADR-109-upc-kernel-rows-per-bpf-slot.md` (Proposed). Decisions: Step 43 per-slot trapframes, BPF points
`sscratch` at the running slot's (no Override). Step 44 BPF publishes `cur_slot` and `gen[8]`, rows refreshed lazily at
trap entry (Override threshold +5,000 recorded). Step 45 rows are `proc[0..7]`. **Amendment from Phase 1:** a fourth
decision, D4, carries the trapping slot's x17/x16 at injection (`trap_a7`/`trap_a6`) because `uservec` reads them
from physical spill addresses. **Step 51 address choice changed:** no fixed low-page address is provably free (the CSR
window reaches `0xFD00`, TTY MMIO at `0xC001`, the layout docs disagree), so the tables live in kernel `.bss` and the
kernel publishes their address in BootParams `reserved[0]` (byte `0x160`); 0 = not published, BPF does nothing.
Invariants I1-I6 and the cost estimate are in the ADR. Security: children cannot reach low memory; init (slot 0) has a
pre-existing identity map of the low 64 MiB and can reach everything, which the ADR records as today's trust model.

### ADR-109 four-lens review and dispositions (Steps 54-56)

A read-only review agent covered the Architect, Developer, Robustness and Scientist lenses and returned 15 findings.
I checked findings 1, 3, 4 and 14 against the code before applying any of them:
- 1: `uvmcreate` clears the page and is keyed by pid with a stale `MBC_MAX_PROCESSES 4`.
- 3: `translate_address` returns the VA on a miss.
- 4: `scheduler_context_switch` is `#[inline(always)]` at in-loop call sites.
- 14: v3 uses bytes 64..96 for the hash, so `0x160` is free.

The ADR was revised the same day.

| # | Lens, severity | Finding | Disposition |
|---|---|---|---|
| 1 | Arch/Rob, blocker | D5 "pagetable per ADR-074" invites `uvmcreate`, which clears BPF's live pgd; pid/slot are off by one there; xv6 and BPF PTE formats differ | Applied. D5 assigns `0x00F00000 + s*4096` directly, as an opaque pointer that is never walked; the first rung that touches a user pointer owns translation. The init-row mismatch (`0xF01000` is slot 1's pgd) is recorded; rung 0 dumps it |
| 2 | Arch, blocker | I4 is unenforced: `killed` → `kexit` → `sched` spins in S-mode; `freeproc` frees BPF's pgd | Applied. `allocproc` skips rows 1-7; `kexit`/`sched`/`sleep`/`freeproc` panic on a slot row; rung 0 never sets `killed`; how a kill reaches BPF belongs to rung 8 |
| 3 | Rob/Sec, blocker | "Children cannot reach the tables" is false: a user page-walk miss falls through to physical memory | Applied as D6. A user-mode miss returns `0xFFFF_FFFC` (outside `RAM_MAP`). It needs its own measurement and a hostile-child probe, and escalates if a golden changes. The security note is qualified |
| 4 | Arch/Dev, should-fix | D3's writes sit in the inlined context switch (in-loop variable offset) | Applied. One `inject_strap()` in the injection arm. Fixed 256-byte trapframe stride, so no table read; `cur_slot` is written at injection |
| 5 | Dev, should-fix | D3 runs without the rung const, which breaks I6 and the OFF − ON count | Applied. D3 and the fork bit are behind the const; the OFF object equals the pre-ADR object |
| 6 | Rob, should-fix | New kernel + old object: `trap_a7 = 0` breaks today's getpid | Applied. Tags `0x5107_0000`/`0xA700_0000`, cleared on read; no tag → today's path |
| 7 | Rob, should-fix | Bound each BPF-written value; `trap_a6` is unread; a zero `sscratch` writes the IVT | Applied. Per-value bounds are in Security notes; `trap_a6` dropped; with the fixed stride, `sscratch` is `base + 256*s` and never 0 while published |
| 8 | Dev, should-fix | D2 ordering: after `userinit`; `kernel_hartid` literal 0; init's trapframe | Applied. `upc_slots_init()` after `userinit()`; init's trapframe is copied into `tf[0]` |
| 9 | Dev, should-fix | Do the a7 copy in C, and drop the a6/a7 save/restore in the trampoline | Applied (D4), plus invariant I8: the spill words are unchanged across a trap |
| 10 | Arch, should-fix | ADR vs skeleton ("free the row"), no generation field, no rung 7 rule, stale fields unowned | Applied. `fresh_mask` replaces `gen[8]` (no spare `PROC_TABLE` word; an array index is a variable offset). Skeleton rung 0 amended; rung 7 forward rule and stale-field owners added |
| 11 | Arch, should-fix | No invariant that BPF never switches in S-mode | Applied as I7. The timer switch is compiled out under `ascend-linux` today (`main.rs:708-727`); an early-return guard is added |
| 12 | Sci, should-fix | The gate proves I2 only; shared trapframe, stale a7 and no refresh would all pass | Applied. Kernel probe asserts per trap, a second injected number (22), interleaved traps in `getpidmulti`, and a mutation check over four broken variants |
| 13 | Dev, note | Test order | Adopted for Phases 3-4 of the plan |
| 14 | Arch, note | D1 writes a frozen BootParams field, reversing its direction | Applied. `UPC_BOOT_PROTOCOL_V2.md` §v3 now has a row for `reserved[0]`; the version stays 3 (the spec allows reserved-word extensions) |
| 15 | Rob, note | The no-race claim assumes one XDP invocation per instance | Stated in Security notes |

**New lead (from 1):** BPF fork rewrites only PDE 0 and 1 of the child's page directory (`main.rs:2259-2266`). If
`uvmfirst` wrote xv6-format entries into `0xF01000`, slot 1's children inherit those entries for VAs at 8 MiB and up.
This is pre-existing. Rung 0 dumps the page after boot.

Constants, header and parity test now match the revised layout:
- `upc_slots.h`: `_Static_assert`s on stride, size and offsets. It compiles under ilp32e, and a wrong offset fails.
- `phase12.rs`: the same constants.
- Parity test: 2 pass. It also checks `struct trapframe` is 144 bytes and the stride is a power of two.
