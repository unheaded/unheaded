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

**D4 correction (same day):** `uservec` keeps its a7 save. That save only reads the spill word, and the no-tag
fallback (an old object with the flag on) depends on `tf->a7`. Only the a6 save and the a6/a7 restores in `userret`
go. I8 is about writes, so it still holds.

## Outlines for the plan's Phases 3-4 (S26 Steps 31, 35)

**Phase 3: rung 0 kernel side, test first (plan Steps 59-84).**
- *Preflight.* Record the OFF verified count (921,036) and the kernel hash.
- *Probes:*
  - `getpidloop.c`: three sequential fork → getpid → exit → wait. Slot 1 is reused, and each child must print
    `GL: child-pid=2`.
  - `getpidmulti.c`: three live children, each calling getpid, then `pause`, then getpid again, so traps from slots
    1-3 interleave. The parent waits for all three.
- Build the probes and record the flag-OFF lines (BPF answers) and the flag-ON lines with today's kernel (RED).
- *Harness gates* behind `UPC_GATE_RUNG0=1`, skipped by default. Commit.
- *Kernel:*
  - `upc_slots.c`: tables in `.bss`, `upc_slots_init()`, `upc_slot_enter()`, added to `Makefile.mbc`.
  - `umain.c` calls `upc_slots_init()` after `userinit()`.
  - `usertrap` calls `upc_slot_enter()` first.
  - `uproc.c`: `allocproc` skips rows 1-7; panic guards in `kexit`, `sched`, `sleep`, `freeproc`.
  - `trampoline_mbc.S`: drop the a6 save and the a6/a7 restores.
- *Probe-build prints* (`UPC_STAGE2_PROBE`):
  - `UT:s;pid;ss;a7;rf` from `upc_slot_enter`;
  - the spill words before and after each tagged trap (I8);
  - a count of non-zero words in `0xF01000` at `upc_slots_init` (the init-pgd lead).
- *Build and test:* `kbuild` (new hash recorded); parity test. With the flag OFF: sweep 8/8, harness green, the object
  unchanged.
- *Compat check (finding 6):* `GETPID_INJECT` on, `UPC_RUNG0` off, new kernel. No tag reaches the kernel, so the
  Phase 1 RED lines must repeat exactly.
- Restore the flags and commit. **Exit gate:** flag OFF green; compat check identical; probes RED with the flag on.

**Phase 4: rung 0 BPF side and gate (plan Steps 85-110).**
- *Read* the injection arm, the fork arm and `translate_address` at the current line numbers.
- *Emulator tests first,* if the host emulator models the injection arm. Otherwise, the probe-build lines are the
  tests.
- *Consts,* both off by default:
  - `UPC_RUNG0`: guards `inject_strap()` (base at `0x160`, skip if 0; `sscratch = base + (current_pid << 8)`; tagged
    `cur_slot` and `trap_a7`), the fork arm's `fresh_mask |= 1 << child`, and the I7 early return.
  - `UPC_D6_MISS_FAULT`: guards D6.
- *Three verified counts:*
  - both off: equal to 921,036 (I6);
  - `UPC_RUNG0` on: the delta, bar ≤ +2,000, Override above +5,000;
  - plus D6: its own delta.

  The total must stay ≤ 950,000. Commit.
- *Gate with the flag and `UPC_RUNG0` on:*
  - `getpidfork` child-pid = fork-ret;
  - `getpidloop` and `getpidmulti`;
  - probe-build UT lines checked by an awk asserter;
  - the probe build injects 22 as well, and the kernel's "unknown sys call 22" line carries pid s+1;
  - I8 spill words unchanged.
- *D6:* `hostile_pgd.c` (the child stores to VA `0xF00000` and to the table address; init checks both). RED with D6
  off, GREEN with it on. If a golden changes with D6 on, keep D6 off and escalate D6 alone.
- *Mutation check:* four `-D` kernel variants (shared trapframe, `trap_a7` ignored, no refresh, a6/a7 restored). The
  gate must fail on each. Commit.
- *Corpus:* flags OFF: sweep 8/8, harness green. Flags ON: `gate2`, `gate_nway`, forkloop, devprobe and the `ut_*`
  probes. The harness gains a rung 0 stage (build the ON object, run the gates, rebuild the OFF object).
- Commit. **Exit gate:** the skeleton's rung 0 gate as amended.

**12-golden corpus helper (S26, forging Phase 7).** The harness checks no goldens, and the S5 `sweep` covers 8 of
the 12. The plan's Phase 3 helpers now define `sweep12`: the 8, plus `nosuch`, `multi` and `blank` through `boot`,
plus `uinit_echo` through `uinit.mbc`, all with the inputs `PHASE22-SH-REPLACE.md` captured. Run 2026-10-28 on
79ec42ef+, flag off: 12/12 SAME.

## Phase 3: rung 0 kernel side (Steps 59-84)

**Steps 59-60 (preflight).** 0 changed files; kernel `4e39c82f9ba56079`; ascend 921,036 verified. That count is the I6
reference.

**Steps 61-67 (probes, flag OFF).** BPF answers both probes as expected:

```
getpidloop:  GL: fork-ret=2 GL: child-pid=2 GL: wait-ret=2  (x3)  GL: done
getpidmulti: GM: fork-ret=2 fork-ret=3 fork-ret=4 c1-pid=2 c2-pid=3 c3-pid=4 c1-again=2 c2-again=3 c3-again=4
             wait-ret=2 wait-ret=3 wait-ret=4 done
```

All three `-pid` lines print before any `-again` line, so traps from slots 1-3 do interleave across `pause`.

**Steps 68-69 (RED, flag ON, today's kernel).**

```
getpidloop:  GL: fork-ret=2 GL: child-pid=other GL: wait-ret=2  (x3)  GL: done
getpidmulti: GM: wait-ret=2 GM: wait-ret=3 GM: wait-ret=4 GM: done
```

In getpidmulti, the parent's three `fork-ret` lines are missing as well as every child line. Step 69 expected the
parent's lines to survive, and in getpidmulti they don't. **Corrected in Phase 4:** the lines were never lost by the program. `TTY_MAP` is a 4 KiB ring, and bootctl drained it
once, at the end. With the flag on, every trap ends in an SRET, and the PC tracer prints after each SRET. That pushed
getpidmulti's output past 4,096 bytes (4,193 once the drain was fixed), so only the tail survived. The earlier
"misdirected syscall disturbs the parent" reading was wrong.

**`ugl` fixed.** The TTY dump joins lines with a middle dot (U+00B7), so `ugl` splits on it first. The plan's
`ugl` was corrected before use.

**Step 70: gates always on, not behind `UPC_GATE_RUNG0` (deviation).** The harness boots the object in the tree,
which ships with the flag off, and both gates pass there. They now cover BPF slot reuse and three live slots. With
the flag on, they are the rung 0 gate. Each gate checks token counts and the total line count, not order. Proven both
ways: PASS with the flag off, FAIL (both) with the flag on and today's kernel.

**Steps 72-78 (kernel side).**
- `crates/xv6-mbc/adapters/upc_slots.c` is new:
  - `upc_st` sits in `.bss`, 256-byte aligned (`.bss` links near `0x108000`, physical).
  - `upc_slots_init()` copies init's trapframe into `tf[0]`, `kfree`s the old page, prefills the four kernel fields for
    all 8 slots, and only then publishes `&upc_st` at `0x160`.
  - `upc_slot_enter()` and `upc_is_slot_row()` are as ADR-109 D5 states them.
  - It also holds the probe-build prints and the three C mutants (`UPC_MUT_SHARED_TF`, `UPC_MUT_NO_A7`,
    `UPC_MUT_NO_REFRESH`).
- `upc_slots.o` goes at the end of `OBJS`, so no existing object moves.
- The prototypes are in `upc_slots.h`; upstream `defs.h` is untouched.
- Wiring:
  - `umain.c` calls `upc_slots_init()` after `userinit()`;
  - `usertrap` calls `upc_slot_enter()` before `myproc()`;
  - `uproc.c`: `allocproc` skips rows 1-7, and `kexit`, `sched`, `sleep` and `freeproc` panic on a slot row;
  - `trampoline_mbc.S`: the a6 save and the a6/a7 restores are gone; the a7 save stays.
- The new kernel is `3c63d4865a4b5dcf` (47,568 bytes). The parity test passes.
- The only warning is `riscv.h:241` (`SATP_SV39` shift), from the existing `MAKE_SATP` in `utrap.c`. It showed up only
  because that file recompiled.

**I8 probe redesigned.** An `SP:post` print from C would run before `userret`, which is the code that writes the
spill word, so it would miss the write I8 is about. Instead, on each child trap the probe build prints physical
`0x64004` (init's x17 spill word) and then plants a sentinel (24151) there. A later child trap must see the sentinel
or a number from one of init's own stubs; 11 means `userret` wrote a child's a7 back.

**Step 79 (flag OFF).**
- `sweep12`: 12/12 SAME with the new kernel. Neither the PC tracer nor the stats moved: the trampoline lost three
  instructions and nothing shifted that the goldens see.
- getpidfork: the Phase 0 lines.
- Harness: ALL GREEN, including the new getpidloop and getpidmulti gates. Ascend 921,036 (unchanged; the object did
  not change); Doom 758,199.

**Steps 81-82 (compat, finding 6).** Flag ON with today's BPF (no `UPC_RUNG0` yet) and the new kernel: no tag
arrives. The getpidloop and getpidmulti lines are byte-identical to the Phase 3 RED record, and init's own getpid
still answers 1 with the flag on (`GF: parent-pid=1`). So the no-tag path is today's path, even with D4's a6/a7
changes.

**Phase 3 exit gate: PASS.** Flag OFF: 12/12 SAME, harness ALL GREEN, count unchanged. The compat check repeats the
RED lines. The RED record is `$S7/p3-red.txt`. `GETPID_INJECT` is `false` in the tree.

## Phase 4: rung 0 BPF side and the gate (Steps 85-110)

**Step 85 (read).** Current line numbers:
- injection arm: `if GETPID_INJECT` at about `main.rs:2525`;
- fork arm: `syscall_nr == 1` at `:2193`; the child slot is final at `:2226-2232`, and the slot revive (`SCHED_STATE`
  3/4) is at about `:2355`;
- `translate_address` miss sites: PDE at about `:3712`, PTE at about `:3736`;
- `scheduler_context_switch`: about `:3500`.

`mem_write_word` drops a store outside `RAM_MAP`. No host emulator models the injection arm.

**Steps 86-87 (asserter).** `scripts/upc-rung0-assert.awk`. Init never traps in getpidloop/getpidmulti, so there is no
`s=0` line; the base is derived as `ss - 256*s` and must be equal on every line. As planned, the asserter could not
catch `UPC_MUT_SHARED_TF`: BPF still sets `sscratch = tf[s]`, so `ss` looks right while the row's trapframe is `tf[0]`.
So the UT line now prints `tf=` (the row's trapframe), and the asserter requires `tf == ss`.

It is proven able to fail on every hand-made violation, each with its own reason:
- no UT lines;
- wrong pid;
- no refresh;
- shared trapframe;
- stale a7;
- 11 in the I8 word.

**Steps 88-92 (BPF).**
- Consts `UPC_RUNG0` and `UPC_D6_MISS_FAULT`, both off.
- `inject_strap()` is called after the sstatus write in the injection arm.
- The fork arm sets bit `child & 7` in `fresh_mask` after the slot revive.
- I7: the first line of `scheduler_context_switch` returns early when `priv_level != 3`.
- `d6_user_miss()` is checked at both miss sites; it is ascend-only and U-mode with a pgd only.
- A placement slip (the helpers went between `translate_address`'s `#[inline(always)]` and its `fn`) was a compile
  error, fixed by moving the block above that doc comment.

**Steps 93-96 (verified counts, one session, rustc and host kernel as in Phase 0).**

| `GETPID_INJECT` | `UPC_RUNG0` | D6 | verified |
|---|---|---|---|
| off | off | off | **921,036** (= the Step 60 reference: I6 holds) |
| off | on | off | 922,754 (+1,718: the fork bit and the I7 guard; the inject arm is compiled out) |
| off | on | on | 925,342 (+2,588 for D6) |
| on | off | off | 909,353 |
| on | on | off | 913,408 (**+4,055** over inject alone) |
| on | on | on | 916,504 (+3,096 for D6) |

- The plan's Step 93 counted only flag-off objects. With the flag off, the injection arm is compiled out, so the
  rung's real cost is the flag-on pair; that pair is recorded here as well.
- `UPC_RUNG0` costs +4,055, over the +2,000 bar and under the +5,000 Override. Per Step 94 it is recorded and the
  plan continues.
- The shipping configuration (all on) is 916,504, which is 4,532 under the pre-ADR flag-off object: the ladder still
  pays for itself.
- `check-ebpf-insns.sh`: PASS.

**Step 98 (flag and `UPC_RUNG0` on, normal kernel `3c63d486`, unchanged).**
- getpidfork: `parent-pid=1 fork-ret=2 child-pid=2 wait-ret=2`; the child's getpid is now answered by the kernel.
- getpidloop: child-pid=2 three times.
- getpidmulti showed only its last lines. That was the TTY ring wrapping, not a fault (correction above).

**bootctl TTY drain (fix).** `upc-bootctl boot` now sends triggers in chunks of 1,000 (`TTY_DRAIN_CHUNK`) and drains
`TTY_MAP` between chunks into one buffer. A chunk can still lose bytes if it prints more than about 4 bytes per
trigger; this is recorded in the code comment. A boot that never wraps gets exactly the bytes the single end-of-run
drain gave. A getpidmulti boot with the flag on takes 23.4 s.

**Step 99:** getpidmulti with the flags on prints all 13 lines: children 2, 3, 4, stable across `pause`; fork-ret and
wait-ret name the same set. PASS.

**`ugl` decodes the hex line.** Kernel `printf` sends each byte as the char plus two NULs, and the `ascii:` rendering
shows NULs as the same middle dot used for newlines. So `ugl` now decodes the `hex:` line, drops the NULs, strips the
tracer and matches. The plan's helper was updated.

**Steps 100-101 (probe kernel, `-DUPC_STAGE2_PROBE`).** `RUNG0-ASSERT pass 9` over getpidloop + getpidmulti:

```
UT:s=1;pid=2;ss=1133824;tf=1133824;a7=11;rf=1;   x4 (getpidloop 3, getpidmulti 1), rf=0 x1
UT:s=2;pid=3;ss=1134080;tf=1134080;a7=11;rf=1;   then rf=0
UT:s=3;pid=4;ss=1134336;tf=1134336;a7=11;rf=1;   then rf=0
SP:x17=24151 x5 (sentinel intact), SP:x17=3 x4 (init's own wait stub)
PG:f01000-nonzero=1;first=1;val=2028545;
```

- The table base is 0x114C00 (in `.bss`), and the trapframes sit 256 bytes apart.
- **The init-pgd lead is closed.** The one stale word in slot 1's pgd is PDE 1 (0x1EF401, an xv6-format entry from
  `uvmfirst`), and BPF fork rewrites PDE 0 and 1 before a child runs.

**Injected 22.** `getpidx.c` and a probe-only const `UPC_PROBE_INJECT22`:
- `a7=22` reaches the kernel from slots 1 and 2, right after their getpid (11);
- the kernel prints `2 slot: unknown sys call 22` and `3 slot: unknown sys call 22`;
- each child gets -1 and its next getpid is unchanged;
- `RUNG0-ASSERT pass 6`.

The asserter now counts `GX` fork lines as well.

**Steps 102-103 (D6): deferred, not run.**
- D6 is implemented behind `UPC_D6_MISS_FAULT` (off) and measured at +3,096 verified instructions with the flag on.
- Its hostile-child validation probe was not built this session.
- **D6 stays off in the tree and does not flip with rung 1.** It goes to Stevie as a decision: the fall-through hole
  ADR-109's security notes describe is still open.
- ADR-109's status will read "Accepted except D6".

**Steps 104-105 (mutation check).** The four mutants each ran in their own probe kernel, with the flag and `UPC_RUNG0`
on, over getpidloop and getpidmulti. Every mutant fails, and each reason names the property it broke:

```
UPC_MUT_SHARED_TF:   fail slot 1 row trapframe 1133568 is not sscratch 1133824
UPC_MUT_NO_A7:       fail a7 3 on slot 1            (the stale spill value, Phase 1's mechanism)
UPC_MUT_NO_REFRESH:  fail pid 0 for slot 1
UPC_MUT_RESTORE_A67: fail init's x17 spill word holds a child's a7 (I8)
```

`UPC_MUT_RESTORE_A67` is an `#ifdef` in `trampoline_mbc.S` (`.S` files are built with `$(CC) $(CFLAGS)`). After the
runs, the probe `-D` was reverted; the normal kernel hash is `3c63d4865a4b5dcf` (unchanged since Phase 3).

**Step 107 (flags on: `GETPID_INJECT` + `UPC_RUNG0`, D6 off).** The harness was ALL GREEN (29 PASS: gate2, gate_nway,
forkloop, devprobe, the `ut_*` probes, getpidfork/loop/multi). It also surfaced three problems:

1. **Doom count moved to 759,416 (from 758,199).** The I7 guard in `scheduler_context_switch` was not gated to
   `ascend-linux`. With `UPC_RUNG0` on, Doom would have stopped context-switching, since Doom never runs at priv 3.
   Fixed: `cfg!(feature = "ascend-linux") && UPC_RUNG0 && ...`. The other two `UPC_RUNG0` sites are inside
   ascend-only arms. Phase 5's harness run re-measures Doom with `UPC_RUNG0` on.
2. **`cat_BIGFILE` golden: DIFF.** This came from the bootctl drain fix, not the flags. The golden was itself a
   post-wrap tail:
   - the full capture is 15,596 bytes;
   - the old golden is exactly its last 3,308 bytes (15,596 mod 4,096), with the stats equal;
   - all 14,570 bytes of `BIGFILE` appear verbatim in the new capture.

   So the old golden checked only the file's last 3.3 KB. **It was re-captured** from a flags-off boot into
   `~/tmp/golden/cat_BIGFILE.{tty,full}`; the old files are kept as `*.pre-drain`, and the stats are unchanged.
   `sweep12` is then 12/12 SAME with the flags off.
3. **The drain made a 4M-trigger boot 58.5 s instead of 9.8 s.** Each 1,000-trigger chunk opened and closed an
   AF_PACKET socket, and the close waits for an RCU grace period (about 12 ms). Fixed with
   `netns::trigger_with(count, instance, every, on_every)`: one socket, and a callback every 1,000 frames that
   drains `TTY_MAP`. `send_trigger_native` keeps its signature. A boot is back to 9.75 s with the full BIGFILE
   capture. The `--create-netns` script path drains only at the end, as before.

**`parity_surface.rs` caught the probe arm.** Its parser wants one `syscall_nr ==` per dispatch arm. The getpid arm is
back to `syscall_nr == 11`; `UPC_PROBE_INJECT22` now remaps 22 to 11 right after `syscall_nr` is read. The kernel
reads the number from `trap_a7` (`regs[1]`), so it still gets 22. Re-verified: `RUNG0-ASSERT pass 6`, and
"2 slot: unknown sys call 22" and "3 slot: unknown sys call 22" again. bootctl tests all pass.

**Step 108 (after the fixes; one session).**

| | flags OFF | `GETPID_INJECT` + `UPC_RUNG0` on |
|---|---|---|
| `sweep12` | 12/12 SAME | 12/12 SAME |
| harness | ALL GREEN | ALL GREEN (getpidfork, getpidloop, getpidmulti PASS) |
| ascend | 921,036 | 913,408 |
| Doom | 758,199 | 758,199 (the I7 fix holds) |

The consts were restored to `false` and the object rebuilt.

**Phase 4 exit gate: PASS, except D6.**
- The child's pid equals `fork()`'s return.
- ON and OFF lines agree.
- `gate2`, `gate_nway`, forkloop, devprobe and the `ut_*` probes pass with the flags on.
- Every reused slot refreshes exactly once.
- The asserter passes, and all four mutants fail.
- D6 is deferred (Steps 102-103): it stays off, as a decision for Stevie.

## Phase 5: rung 1, the getpid flip (Steps 111-130)

**Measured pair (one session; rustc 1.101.0-nightly 5c543b0b8 2026-09-29; host kernel 6.17.0-41-generic).**
- Flag OFF 921,036; `GETPID_INJECT` + `UPC_RUNG0` ON 913,408. **Freed 7,628.**
- For comparison, getpid alone frees 11,683 (921,036 − 909,353); rung 0 costs 4,055 of that.
- D6 stays off.

**Flipped in the tree.** `GETPID_INJECT = true` and `UPC_RUNG0 = true`; the const comment is updated.
- getpidfork, getpidloop and getpidmulti print their flag-OFF lines.
- `sweep12`: 12/12 SAME.
- Harness: ALL GREEN; ascend 913,408, Doom 758,199.
- Ratchet: `docs/policy/ebpf-insns-baseline.tsv` `monad_cpu` 921,036 → 913,408; the check passes. `monad_cpu` stays
  on the over-900,000 allowlist.

The harness has no expected-count assertion and no `UPC_GATE_RUNG0` variable (Steps 119-120 are moot: the rung 0
gates are always on since Phase 3).

**Docs (Steps 124-129).**
- `EVOLUTION-1-TRAP-FLIP.md`: "Stage 1 CLOSED 2026-10-28".
- Master plan 1.3.5 `[x]`.
- `wiki/UPC-Dream-Ladder.md`: the L6 row's Stage 1 clause now says the kernel serves getpid for every process. The
  plan named the L5 row, but the clause lives in L6.
- `CLAUDE.md`: one sentence after the S-ASCEND-2 "stays false" sentence.

Dated session blocks are left as written.

## Phase 6: security pass on rungs 0-1 (Steps 131-146)

**Step 131: attack surface of the values ADR-109 added.**

| Value | Writer | Reader | Hostile value | Bound | Test |
|---|---|---|---|---|---|
| BootParams `0x160` (table base) | kernel `upc_slots_init` | BPF `inject_strap`, fork arm | init (identity map) can rewrite it | BPF: `RAM_MAP` bounds every access; 0 skips | parity test (address); a wild base only redirects BPF's three writes inside RAM, which init could already write directly |
| `cur_slot` | BPF at injection | kernel `upc_slot_enter` | any word | tag check, `< 8`, `sscratch == &tf[s]`, then cleared | mutants SHARED_TF/NO_REFRESH fail the asserter; compat check (no tag → today's path) |
| `trap_a7` | BPF at injection | kernel | any word | tag check; 16-bit value through `handler_for` (unknown → -1); cleared on read | injected 22 → "unknown sys call 22", returns -1; mutant NO_A7 fails |
| `fresh_mask` | BPF at fork; kernel clears bit | kernel | any word | only bit s is read; a spurious bit only refreshes a row | asserter: refresh count = fork count |
| `sscratch` (BPF-set) | BPF at injection | `uservec` | none from user code (CSR window is not user-mapped for children) | always `base + 256*s` when published | asserter `ss == tf`; `upc: sscratch` panic guard |
| per-slot trapframes | `uservec`, kernel | kernel, `userret` | a child at VA ≥ 15 MiB reaches physical memory while D6 is off | D6 (off) | **deferred with D6** |
| init's x17/x16 spill words | init's stubs | BPF (via shadow) | n/a: no longer written by a trap | D4 | I8 sentinel probe; mutant RESTORE_A67 fails |
| `0xFFFF_FFFC` D6 sentinel | BPF (D6 on) | n/a | n/a | outside `RAM_MAP` | not enabled |

**Step 132 (reachability, read-only).**
- A child's pgd has BPF-written PDE 0 and 1 (its slice). Every other PDE is either not present or the one stale
  xv6-format word the `PG:` probe found (PDE 1 of slot 1's pgd at boot), and fork overwrites that one.
- So with D6 off, a child's VAs from 8 MiB up resolve to themselves physically. That is the pre-existing fall-through
  ADR-109 records. It reaches the per-pid pgds at 15 MiB and, through them, everything else.
- With D6 on, those VAs resolve to the sentinel.
- Init's identity map over the low 64 MiB is unchanged: it is today's trust model.

**Step 133.** Every row has a test except the trapframe/fall-through row. That row's hostile probes (Steps 134-138)
are deferred with D6: without D6 they would only confirm the open hole the ADR already states.

**Steps 140-143 (fuzz and repo gates).**
- `fuzz_mbc_execute`: **SKIP, `cargo-fuzz` not installed** (`cargo +nightly fuzz` → "no such command"). The target
  covers host monad-mbc, which this sprint did not change. Not a pass.
- `check-ebpf-insns`: PASS. `check-gitleaks`: PASS.
- `check-clippy` failed (5 warnings against a 0 baseline). All five are fixed; it is now 0/0:
  - `boot_prepare` lost its `#[allow(clippy::too_many_arguments)]`, because `TTY_DRAIN_CHUNK` was inserted between
    the attribute and the fn (the same slip as in `main.rs` earlier). The const is moved above.
  - `UPC_ST_SIZE` was unused. It is now used by two compile-time layout asserts in `phase12.rs`.
  - Two needless-range loops in `os_primitives_test.rs` and an unused binding in `fuzz_slice_geometry.rs`, both from
    S-ASCEND-2 commits. Fixed; os_primitives 69 pass.
- `check-gates-can-fail` exits 2 on a dirty tree; it is re-run after the commit.
- `check-gates-can-fail` (full run, 45 min limit): 53 gates "bite when provoked". **`verify-gpl-boundary` reported
  PASSED WHILE VIOLATED** in this worktree, while a throwaway `develop` worktree in the scratchpad bit.
  - Root cause: its `find` calls excluded `*/.claude/*` matched against absolute paths, so any checkout under a
    `.claude/` directory (this worktree is in `.claude/worktrees/`) scanned nothing and passed.
  - Fixed: the exclusion is `"${REPO_ROOT}/.claude/*"`. Plain run PASS; `--only verify-gpl-boundary` bites.
  - The full run then stalled in `well/test-zhen-captures` (docker-backed) and hit the limit. That gate and the ones
    after it were not checked this session.
- **Step 144:** `go test ./...`: 0 FAIL. upc-bootctl cargo tests: all ok.

**Phase 6 exit gate: PASS, with these not run:**
- the D6 hostile probes (deferred with D6);
- `fuzz_mbc_execute` (cargo-fuzz not installed);
- the `check-gates-can-fail` tail after `well/test-zhen-captures`.

## Close (Phase 7, Steps 147-160; S26 Phase 7)

**Final regression on the final tree.** Flags shipped: `GETPID_INJECT` and `UPC_RUNG0` on; D6 and
`UPC_PROBE_INJECT22` off.
- Harness: ALL GREEN.
- `sweep12`: 12/12 SAME.
- Ratchet: PASS.
- `go test ./...`: 0 FAIL (Phase 6).
- **Ascend 913,408; Doom 758,199.**

**Verified counts across the session (ascend object).**

| Point | Count |
|---|---|
| Phase 0 / Step 60 (flag off, pre-ADR) | 921,036 |
| Flag on, getpid alone (Phase 4) | 909,353 |
| Flag + `UPC_RUNG0` (shipped) | 913,408 |
| + D6 (not shipped) | 916,504 |

**Docs.**
- ADR-109 is Accepted except D6; the index and the status summary are updated.
- Timeline entry `phase-3-item-18a106dd`, mirrored to JSON, YAML and TOML (all three agree; FRESH).
- OUTSTANDING: M-220 DONE, M-221 PARTIAL (rung 2 next), M-247 (D6, Stevie) and M-248 (gates-can-fail stall) added.

**Steps not done.**
- 102-103 and 134-138 DEFERRED: the D6 hostile probes, with D6 off.
- 140 SKIP: cargo-fuzz is not installed.
- The `check-gates-can-fail` tail after `well/test-zhen-captures` was not exercised.

**Open leads.**
- Trigger halving (side findings).
- Bootstub retirement (Stevie: Boot Protocol v2 is frozen).
- forktest's fork limit is 5, not 7 (slice ceiling).
- The init-pgd lead is closed (PDE 1 only, which fork rewrites).

**Commits on this branch since `sprint/ascend-2`:** 29 (before this one).

## Rung 2 research (uptime / pause), 2026-10-28 night

**What `uptime` returns today** (`user/uptimeprobe.c`, shipped flags):

```
UP: init-before=small UP: init-after=zero UP: init-moved=yes UP: child-before=zero UP: child-after=zero UP: child-moved=no
```

BPF has no arm for 14, so it falls into "Unknown syscall: silently ignore", and `uptime()` returns whatever a0 held.
init's "moved" is an artifact: `pause` had set a0 to 0. The syscall is effectively unimplemented; it arrives.

**The kernel's `ticks` never moves.** With 14 routed to the kernel (temporary remap, not committed), every value is
`zero`. `ticks` advances only in `clockintr`, through `devintr`, on a timer trap, and the BPF timer is compiled out
under `ascend-linux`. **The skeleton's rung 2 prerequisite ("the timer interrupt reaches the kernel") fails today.**

**The BPF `pause` arm costs 45,502 verified insns** (913,408 → 867,906 with the arm compiled out). Most of that is
probably `scheduler_context_switch`, which is `#[inline(always)]` at seven call sites (tick start, three Doom arms,
exit, wait, pause). Moving `pause` into the kernel frees that only if the context switch is not re-incurred
elsewhere. Under I7, the kernel cannot switch BPF processes; BPF still has to yield, so a naive move puts the switch
in another arm and frees about nothing. E2's stop rule (stop at the first rung whose freed instructions are below
zero) makes this the deciding cost.

**Experiments on the verifier budget.**
- `scheduler_context_switch` as `#[inline(never)]` (a BPF static subprogram): **949,679, +36,271.** The verifier
  re-walks a static subprogram at every call site, plus the call overhead. Reverted.
- **Deferred switch (`DEFERRED_SWITCH`, on):** the ascend exit, wait and pause arms set `switch_req` and break, and
  one `scheduler_context_switch` after the execute loop does the work. An exit that leaves the old pid running halts,
  as the exit arm did.
  - **868,290: −45,118** (913,408 before).
  - Doom is unchanged at 758,199 (its arms keep their own calls).
  - Behaviour: a `pause` with nothing else runnable now ends its tick early. Before, it carried on in the same tick.
  - `sweep12`: 12/12 SAME. Harness: ALL GREEN (29 PASS).
- `monad_cpu` is under 900,000 for the first time since ASCEND-LINUX Gate D, so it comes off
  `docs/policy/ebpf-insns-over-budget.txt`. The removed entry's history:
  - 926,542 from Gate D (2026-07);
  - +181 instance-owner check;
  - +1,124 pid = slot + 1;
  - −3,914 wait ignores reaped children;
  - then 921,036 → 913,408 (rung 1) → 868,290 (this change).

  The baseline is updated, and the ratchet passes with 0 entries allowlisted.

**Rung 2, built behind `UPC_RUNG2` (off), ADR-110 Proposed.**
- **T1 (clock).** The slot table's spare word at +0x80C becomes `ticks`. `inject_strap()` writes `cpu.tick_counter`
  there when `UPC_RUNG2` is on, and `upc_slot_enter()` raises the kernel's `ticks` to it under `tickslock`, never
  lowering it. Kernel hash `8fb1319f8c000228`. With the const off, BPF never writes the word, and `ticks` stays 0 as
  before.
- **T2 (uptime arrives).**
  - The new ascend arm `syscall_nr == 14` is a no-op with the const off.
  - With it on, `kernel_routed_alias()` maps 14 to the getpid inject arm, and the kernel reads 14 from `trap_a7`.
  - 14 is added to `SURFACE_ASCEND` (surface.rs) and to bootctl's declared `XV6_SYSCALLS`, so the surface id
    changes.
  - `parity_surface`'s parser now treats only the first `if` as the chain head.
  - A first version (one shared post-chain injection, `inject_req`) cost +3,948 even with the const off. The alias
    keeps the off object at exactly 868,290.
- **T3:** `pause` stays a BPF yield.

**Measured.**

| `UPC_RUNG2` | verified | uptimeprobe |
|---|---|---|
| off | 868,290 (= before) | stale a0: `init-before=small init-after=zero init-moved=no ...` |
| on | 872,276 (**+3,986**) | `init-before=large init-after=large init-moved=yes child-moved=yes child-mono=yes` |

Both sides: 12/12 SAME, harness ALL GREEN, Doom 758,199. The new harness gate "uptime" runs when the const is on and
is skipped, with a note, when it is off. Its expectation rejects the OFF output (`init-moved=no`).

**Rung 2 frees −3,986 on its own.** By E2's literal rule (stop at the first rung whose freed instructions are below
zero), the ladder stops here. Read cumulatively, it has freed 49,746 since rung 0 (921,036 → 872,276 with rung 2
on). **Escalated to Stevie: which reading holds.** The const stays off until then.

**Trigger halving: found and fixed (the side-findings lead).** A new bootctl STATS line,
`TICKS: triggers= pkts= cpu_ticks= tick_counter=`, shows every frame reaching `monad_cpu` and exactly half passing the
Monad checks (5,000 of 10,000; 25,000 of 50,000).

The cause is the trigger frame layout:
- bootctl's native sender (copied from `scripts/doom-tick.py`) wrote the trace id at register bytes 0x04-0x07, over
  `qos_class`, `flow_action`, `circuit_state` and `flags`.
- It put CUSTOM at 0x0B, which is `mesh_flags` in the frozen v0x01 layout (`Monad::from_bytes`, flags at 0x07).
- So a frame ticked only when its trace id had bit 1 set, which is half of consecutive ids. Some ticks also carried
  CHAOS/CANARY/TRACED/... (trace id 0xFFFFFFFF sets every flag).
- Fixed in bootctl (`build_trigger_frame`), `scripts/doom-tick.py` and its byte-identical twin `scripts/doom/tick.py`:
  flags = CUSTOM at 0x07, and the trace id moves to `scratch` (0x0E-0x11).
- The Go Doom injector already had flags at 0x07.
- The pinned-bytes test `trigger_frame_matches_doom_tick_py` takes its new expected bytes from the fixed script's
  `--dry-run`.

Now `cpu_ticks = triggers` (10,000/10,000; 50,000/50,000): twice the CPU work per trigger. `sweep12`: 12/12 SAME.
Harness: ALL GREEN. Trigger budgets in the harness and plans are now twice as generous as they were measured; they
are left as they are.

**`check-gates-can-fail` coverage completed (M-248).** It was not a hang. In isolation `well/test-zhen-captures`
bites in under 10 minutes; the full run simply takes longer than 45 minutes. The eight gates the full run never
reached were each run with `--only`:
- `wireguard/test-rotate-psk`: OK, it bites.
- Five gates were INCONCLUSIVE because they already failed on a clean tree. All are fixed (`326a6ae0`) and now bite:
  - `check-crypto-inventory`: the CBOM was stale since S-ASCEND-2.
  - `check-unit-binaries`: the upc-bootctl unit runs a Rust binary; `build-binaries.sh` gains a `CARGO` list.
  - `test-deb-maintscripts`: cargo-built binaries are not packaged.
  - `well/test-zhen-confirm`: the integration test confirmed as `"stevie@test"`, where RT-03 requires
    `champion.QueuePrincipal`.
  - `well/test-zhen-captures` (above).
- Still INCONCLUSIVE on west, for Stevie:
  - `check-ruff` and `check-python-audit`: ruff and pip-audit are not installed here (CI pins them).
  - `check-tool-freshness`: `aquasecurity/trivy-action@master` is pinned at `6abccf398f78`, and `8f28faec2651`
    (2026-10-21) has been on master past the 7-day cooldown. Re-pinning (docker.yml, security.yml) is a dependency
    approval.
