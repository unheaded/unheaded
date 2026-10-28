# S-ASCEND-2 sprint log

Started 2026-10-28 on 31346cbf (based on sprint/true-wire, not develop: the M-184 verifier ratchet lives there; if E2 reverts the true-wire renumber, rebase this branch)

## Baselines

## Hypotheses (Steps 15-24, 2026-10-28)

| H | Claim | Verdict |
|---|---|---|
| H1 | `GETPID_INJECT = false`; `user/getpidprobe.c` exists | match (main.rs:413) |
| H2 | the two loader gates are "named in the docs but not performed" | match (workload.rs:182) |
| H3 | `NewLoadSequence` has no non-test caller | match (the one hit is its definition, mapfreeze.go:122) |
| H4 | the Doom HMAC secret literal is in `cmd/` | match (2 hits) |
| H5 | the RV2MBC SHA gate is env-only (`UPC_RV2MBC_SHA`) | match (main.rs:634); `BOOT_VERSION = 2` |
| H6 | bootctl creates the netns itself (needs root) | match (netns.rs `ip netns`/`ip link`) |
| H7 | no seccomp / demo-mode gate for UPC | match: 0 in `cmd/upc-bootctl`; the grep's 62 hits are other services' units (32 files carry SystemCallFilter) |
| H8 | the instance id is `flow_label & 0xFF`, unvalidated | match |
| H9 | no upc sysuser, unit or nft rule | match (0/0/0) |
| H10 | T_DEVICE never handled in the open path | **amended**: `adapters/usysfile.c` (in the kernel build) handles T_DEVICE in `sys_open` the xv6 way (devsw major check, :354/:368); the BPF open path (`fs_walk`, `main.rs`) does not. Which one serves `open()` at runtime is Step 183's question |
| H11 | riscv.h CSR helpers return `uint64` (4 bytes under ilp32e) | match (21 helpers; `typedef unsigned long uint64`) |
| H12 | ADR-043 claims "13 phases shipped" | match (1) |
| H13 | "Unheaded Linux" names in docs | 14 files (13±2) |
| H14 | `crates/upc-bootstub` exists | match |
| H15 | cartridge live-leg surfaces: `cmd/anomaly -wotan`, `upc-cartridgectl -sophia`, scorer files, `cmd/steel-echo/load.go` | match |

**Live state (Step 21, read-only):** east up (2 weeks 1 day), `enp2s0` UP. West: 0 UPC programs attached, 0 `upc0` netns, 0 nft rules naming `veth-upc0p`. GPG signing works (Step 6). Toolchain: all ten present (`nft` in /usr/sbin); the bpfel target is build-std, not a rustup target (the eBPF builds work).

## Baselines (Steps 7-14, 2026-10-28, HEAD 31346cbf)

- go build OK; `go mod tidy -diff` empty; monad-mbc, upc-bootctl, doom-runner release builds OK.
- go test: **0** failure lines (`p0-go-fail.txt` empty).
- Golden sweep: **8/8 SAME** (ls, cat README, cat BIGFILE, cat sub/NOTE, ls sub, echo hello, echo peace and love, wc
  README). The other four of the 12 golden sets (blank, multi, nosuch, uinit_echo) are checked by `upc-regression.sh`,
  which passed: 12/12 covered.
- `upc-regression.sh`: ALL GREEN; ascend **926542**, Doom **761580**, cart_xdp **42743**, cart_tick **146897** (same as
  S-TRUE-WIRE Step 16 and `docs/policy/ebpf-insns-baseline.tsv`).

## Known failures (KF)

| KF | What | At start |
|---|---|---|
| KF1 | `gate_nway` (3 processes, isolation) | **0 `NWAY-ok`, 231 `NWAY-FAIL pid2` markers** at 1.5M triggers: pid 2 fails, pids 0/1 print nothing ok (`p0-nway.txt`). **Retired 2026-10-28 (f2bc1a56)**: not an isolation failure; init's exit resumed a reaped zombie (Phase 3) |

Phase 0 exit gate: Steps 10 and 12 green; KF1 reproduced; H1-H15 match (H10 amended). → Phase 1.

## Phase 1: the child-getpid probe (Steps 27-40, 2026-10-28)

`user/getpidfork.c`, run as the userland (`--userland target/getpidfork.mbc`), 800,000 triggers.

**Step 32 (two attempts).** The first draft ended the parent with `exit(0)`. It runs as pid 0 / init, and init's
`exit(0)` does not halt here: control comes back into user code and the parent's last line repeats for ever (one
`fork`, one `wait`, 182 copies of the tail at 900k triggers; `forks=1 waitpid=1` in STATS). That repetition first
looked like the child running the parent's post-`wait` code. It is not: with the parent spinning after its last line
(`for(;;);`, as real xv6 init never exits), fork, the child's line, `wait` and the parent's tail each happen exactly
once (three minimal variants, with and without a printf or getpid before fork, all agree). The probe now spins.

Two observations carried forward, not fixed here:
- **Init's `exit` returns to user code** instead of halting or panicking (xv6 panics "init exiting"). Any probe or
  gate that lets init exit sees its tail repeat. gate2's harness only greps `ISOLATION-PASS`, so it cannot see this.
- **A user PC trace** (`S4000 4005 …`, one hex PC per tick) is written to the TTY from `kexec` on, interleaving with
  the first user write (`G<trace>F: parent-pid=0`). Grep for `F: …`, not `GF: …`, until it is removed.

**Steps 30-37: dual books.**

| Line | flag OFF (BPF answers) | flag ON (owned kernel answers) |
|---|---|---|
| parent getpid (init) | `parent-pid=0` | `parent-pid=1` |
| child getpid | `child-pid=1` | `child-pid=other`: bit dump = **0xFFFFFFFF (-1)** |
| fork() return in parent | 1 (BPF slot) | 1 (bit dump) |
| parent after wait | yes | yes |
| STATS | `forks=1 waitpid=1 tty_r=0` | `forks=1 waitpid=5 tty_r=4` |
| ascend verified insns | **926542** | **911319** |

Delta ON - OFF = **-15223** verified instructions (ON is smaller). The plan expected the child to see init's pid (1)
through `cpu->proc`; it sees -1. The owned kernel has no process for the BPF-forked child, so its `sys_getpid`
fails: the two books do not merely number differently, the kernel's book has no entry for the child at all. fork's
return value in the parent stays the BPF slot (1) in both states, so with the flag ON a parent's `fork()` result and
its child's `getpid()` disagree (1 vs -1).

Evidence: `$S6/p1-fork-off.txt`, `$S6/p1-fork-on.txt`, `$S6/p1-fork-on-bits.txt` (temporary bit-dump probe, not
committed), `$S6/p1-fork-off-v1-exiting.txt` (the exiting first draft).

Plan deviations: 800k triggers, not 2.5M (past ~1M the spin's PC trace scrolls the probe lines out of the 2208-byte
ring). `UPC_VERIFIER_STATS=1 sudo …` as the plan writes it never prints a count, because sudo drops the variable;
`sudo UPC_VERIFIER_STATS=1 …` does.

**Step 38.** Flag restored OFF; the ascend object is byte-identical to the Phase 0 build
(sha256 `30b4ceb3…7fb2`); 0 modified files under `ebpf/`.

## Pid reconciliation design (Steps 41-42)

**The two books (Step 41).**
- *BPF.* `SYS_FORK` takes the next free `PROC_TABLE` slot; the slot index IS the pid (init = 0, first child = 1).
  `PROC_TABLE` is `Array<[u32; 22]>`, 8 entries: r0-r15, PC (16), flags (17), SP copy (18), program break (19),
  pgd (20, ADR-074), rv2mbc base (21). **No free u32**: storing a pid means growing the value to 23 words (every
  PROC_TABLE reader and the Go/host mirrors) or reusing slot 18 if the SP copy proves redundant (not checked here).
  `PARENT_OF`, `SCHED_STATE` halted/reaped masks and `wait()`'s return value are all slot-indexed.
- *Kernel.* `adapters/uproc.c` is stock xv6: `nextpid = 1`, `allocproc` sets `p->pid = allocpid()` (a counter), and
  `sys_getpid` returns `myproc()->pid`. Only `userinit` ever calls `allocproc` at runtime: user `fork()` is serviced
  in BPF, so the kernel's `proc[]` holds init and nothing else.

That is why the child sees -1 with the flag ON: the kernel book has no row for it. Where exactly the -1 comes from
(which trapframe `uservec` saved into, what `myproc()` returned) was not isolated: no kernel message was printed and
no halt occurred. Step 47's dump applies once E1 picks a direction.

**Option K: kernel numbering wins (init = 1).**
- BPF `SYS_FORK` writes `pid = slot + 1` into the child's PROC_TABLE row; BPF getpid/wait/`fork()` return answer
  from that field; the kernel's `allocpid` counter is replaced by a read of the same field (one source of truth).
- Not enough by itself: the kernel also needs a `struct proc` for every BPF-forked child (or `myproc()` must be
  derived from the current slot), otherwise the injected trap still lands on init's row. That is the larger half of
  the work and is the same direction as EVOLUTION-1 Stage 2 (kernel authority over process state).
- BPF cost: one store in fork, one load in getpid/wait, plus the PROC_TABLE row growth if slot 18 cannot be reused.
  Not measured; the flag-ON build is already 15,223 instructions *under* flag OFF, so the budget is not the
  constraint (ascend 926,542 of the 950,000 bar).
- Observable change: every pid a userland program prints shifts by one (init 1, children 2, 3, …).

**Option S: slot numbering wins (init = 0).**
- The kernel's `allocpid` returns the BPF slot through the PROC_TABLE MMIO window; `sys_getpid` answers the slot.
- No BPF change. It still needs the kernel to have a `struct proc` per BPF-forked child to answer for the child at
  all, so it does not avoid K's larger half; it only avoids the off-by-one.
- No observable pid change: the goldens and the gate comments that say "init (pid 0)" stay true.

**Who depends on init = 0 (Step 42 grep).** `user/gate2.c` and `user/gate_nway.c` say "runs as init (pid 0)" in
their headers. `gate_nway` labels its children 1, 2, 3 by fork order and says the property is pid-agnostic (it does
not assert `getpid()`). `user/uinit.c` uses only fork's return value. None of them compares a pid to a constant, so
either option leaves the gates' logic intact; option K changes their header comments and any golden line that
prints a pid.

**Caveat (Scientist).** getpid is one syscall and a biased estimator of "the kernel owns the process": it decides
which numbering is the identity, not what migrating fork/exit/wait into the kernel costs. The -1 result says the
real gap is the kernel having no process at all for a BPF-forked child, under either numbering.

**Found on the way, for Phase 3.** `gate2.c` and `gate_nway.c` both say init "exits, so the kernel restarts it".
The getpidfork runs show that is not what happens: init's `exit` returns to user code, with no new fork
(`forks=1` stays 1). `gate_nway`'s repeating verdict block, and possibly KF1's `NWAY-FAIL pid2` count, should be
read with that in mind before any isolation fix.

## E1 (Step 43): filed

Question to Stevie, unchanged from the plan, with this evidence: (1) kernel `p->pid` (init = 1) or BPF slot
(init = 0)? (2) if kernel, option K as above? Recommendation (Computermancer): kernel numbering, option K, because
EVOLUTION-1's direction is kernel authority and K removes the kernel's counter. New since the plan was written: the
kernel has no process for a forked child, so under either answer the next step is giving the kernel a row per BPF
fork. Until answered, Steps 44-51 are `[AFTER-E1]` skips and the flag stays OFF.

## Phase 2: D1 reach re-read (Steps 53-62, 2026-10-28)

**Step 54.** The plan's `grep 'ON=.*DELTA='` finds nothing: Step 37's numbers were written to this log as
the Phase 1 table, not as an `ON= OFF= DELTA=` line. Values used: July OFF 900,031, ON 905,440, delta +5,409
(`references/evolution1-stage1-2026-07-19.md`); today OFF 926,542, ON 911,319, delta -15,223. Headroom: OFF
73,458 below 1M and 23,458 below the 950,000 bar; ON 88,681 and 38,681.

**Attribution.** `monad-cpu-ebpf` logic is unchanged since `8de08368` except `233b6451` (clippy annotation),
per `references/s5-upc-wire-log.md` Phase 0. The delta moved 20,632 and the OFF build 26,511 with no logic
change: toolchain and host-kernel verifier. The retired getpid handler is one line, so the delta is a change
in verifier path exploration, not the handler's size. Per-rung freed insns must be measured as ON/OFF pairs on
one build.

**Step 55.** `EVOLUTION-1-TRAP-FLIP.md` §"D1 reach re-read (2026-10-28) — pending E2". Recommendation: push
toward (a) in ladder order, gated per rung on corpus 12/12 SAME, harness green, freed insns >= 0 and cycle
cost recorded; stop at the first rung with freed insns < 0. The child-getpid result is in the re-read: the
kernel needs a row per BPF fork before any stateful rung and before the arrivals, under either E2 answer.

**Step 57.** `docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md`: rungs 0-8 (rung 0 = kernel row per BPF fork,
added in front of EVOLUTION-1's ladder), one gate per rung, usertests subset plan, open questions. Found while
writing it: `pause` (13) is xv6's sleep(ticks) and BPF services it as a yield, so EVOLUTION-1's "sleep"
arrival and its "pause" rung are the same syscall number; `uptime` (14) is not BPF-implemented; rungs 3/4
(write, read) can only move console fds, because file fds live in `FD_INODE_MAP` until open migrates and D1
requires the FS-metadata bundle to move as one unit. `usertests.c` is not built today; its runner needs
`wait` exit status and `countfree()` needs `sbrk`.

**Step 60.** `0f31757c` (signed).

**Steps 61-62.** Link check on the re-read, the skeleton and the master plan: 0 BROKEN.

Phase 2 exit gate (Step 66): the pending-E2 branch holds. Steps 63-65 are `[AFTER-E2]` skips. -> Phase 3.

## E2 (Step 56): filed

Question to Stevie: given the negative getpid delta (-15,223 today vs +5,409 in July, toolchain-attributed)
and the child-getpid result (-1 with the flag ON: the kernel has no process for a BPF-forked child), do we
**(a)** push toward kernel-authoritative syscalls (Stage 2 runs the full ladder rung by rung, each rung gated
on freed insns >= 0, with a kernel row per BPF fork as rung 0), or **(c)** keep the reach minimal (migrate only
where authenticity demands: the `sbrk`, `pipe`, `kill` arrivals, nothing else)? Sub-question: if (c), does
"minimal" also drop the FS-metadata bundle and getpid/pause/yield that D1 (c) placed in the kernel on
2026-07-17? Recommendation (Computermancer + Scientist): (a), with the per-rung stop rule. New since the plan
was written: rung 0 (a kernel row per BPF fork) is needed under either answer, because every arrival also runs
in a forked child. Until answered, Steps 63-65 are `[AFTER-E2]` skips; the skeleton and the re-read say
"pending E2".

## Phase 3: N-way isolation and the looped-fork gate (Steps 67-94, 2026-10-28)

**Step 68.** Phase 0's gate_nway run: `forks=3 waitpid=3`, then `NWAY-FAIL pid2` 231 times with `cur_pid=0` and
`halted=0`. The real children's verdicts are not in the 2208-byte ring at all.

**Diagnosis (Hn0, not one of the plan's Hn1-Hn3).** The Phase 1 probe had already shown init's exit coming back to
user code. gate_nway's disassembly shows where it goes: main's `jal exit` at 0x174 is followed directly by the
`child(2, 0xB2B2B2B2)` block at 0x178, so a returning exit runs child 2's code as init, fails its canary checks in
init's slice, and its own `exit(1)` returns again: the 231 lines. One run of a copy whose parent spins instead of
exiting (`gate_nway_h0`, not committed) printed `NWAY-ok pid1/2/3` and `NWAY-PARENT-REAPED-ALL` once each. Isolation
was never broken; Hn1-Hn3 (Steps 70-72) were not needed.

**Root cause.** `scheduler_context_switch` (eBPF `main.rs` and the host emulator `crates/monad-mbc/src/execute.rs`,
same code) searches at most 8 slots for one not in `halted_mask | suspended_mask`. When none is runnable the loop
ends wherever its 8th step landed and only `next_pid == old_pid` was checked, so with 2 or 4 processes it stopped on
a reaped zombie (slot 1) and loaded it. The xv6 exit handler then saw `current_pid != exiting_pid` and never halted
(`halted=0` in every run). The resumed zombie continued after its own exit ecall: in getpidfork that was the
parent's post-`wait` code (the first probe's "child runs the parent's tail"), in gate_nway it was init falling into
child 2. Fix: the search records `found`; nothing found means stay put, and SYS_EXIT halts the CPU.

**Tests first.** `os_primitives_test.rs`: `scheduler_stays_put_when_nothing_is_runnable` (num 2-8, every slot a
zombie) and `sys_exit_with_only_zombie_children_halts_cpu` (num 2 and 4). Both FAILED on the old code, pass on the
new; monad-mbc 269 lib + 69 primitives + the rest, 0 failed.

**Results.**

| Check | Before | After |
|---|---|---|
| gate_nway (unmodified, init exits) | 0 ok, 231 `NWAY-FAIL pid2` | 3x `NWAY-ok`, `REAPED-ALL`, CPU halts |
| forkloop (new, 3 iterations of fork + wait) | 3 children + parent-done (parent spins) | same, in the harness |
| ascend verified insns | 926542 | **923645** (-2897) |
| Doom verified insns | 761580 | **758321** (-3259; Doom shares the scheduler) |
| golden sweep | 8/8 SAME | 8/8 SAME |
| upc-regression.sh | ALL GREEN | ALL GREEN, with gate_nway and forkloop |
| check-ebpf-loads.sh | 36 PASS | 36 PASS; `check-ebpf-insns.sh --update` recorded both decreases |

**Looped fork (Steps 81-87).** forkloop passes: 3 `FL: child`, 1 `FL: parent-done`. The "children re-enter the loop
and re-fork" behaviour in gate_nway's header was the same zombie resumption (a reaped child continuing after its
exit, inside the loop). Per Step 83 the fix steps 84-87 are skipped. gate_nway stays unrolled so its binary and
history stay comparable; its header now says why.

**Harness (Steps 88-91).** Two gates after gate2. Deviations from the plan's block: (1) the plan counted
`grep -ac 'NWAY-ok'`, which counts lines, and the TTY dump is one `ascii:` line, so a passing run scored 1 and
failed; the gate counts distinct `NWAY-ok pid[123]` tokens. (2) Step 89 asked for both programs in the default
userland list; that list is copied into fs.img and the `ls` golden lists fs.img, so they are in a new `gates` make
target (`uinit gate2 gate_nway forkloop`) instead, and a missing binary fails the gate.

KF1 retired (evidence `p3-nway-fixed.txt`, `p3-regression-final.log`).

Also corrected: gate2.c and gate_nway.c said init "exits, so the kernel restarts it" (it never did), and the
over-budget note's 926,542.

## Phase 4: the loader's I/O-time gates (Steps 95-116, 2026-10-28)

Host side (Steps 95-106, 113, 115) on `sprint/ascend-2-ph4` (37b01b8d, da9a6473), merged as 50c7b41d. The object
now carries `UPC_SURFACE_ID` and `UPC_MMU_GEOMETRY` in `.rodata.upc`; `upc_api::load()` and an object check in both
loaders compare them with the workload. 44 new tests. Plan corrections found on the way: both surface descriptors
were wrong (Doom declared 0-3, the object handles 1-4, 16, 200, 201; upc-bootctl declared 21 xv6 syscalls, the object
handles 15), only pids 1-5 fit slices under the 56 MiB fork ceiling, and Step 111's `0x400000` is 4 MiB, not 6.

Live (Steps 107-112), after the Phase 3 merge:

| Check | Result |
|---|---|
| harness | ALL GREEN; ascend 923645, Doom 758321 (equal to the Phase 3 values: the constant is data) |
| Doom object under the xv6 kernel | `refusing to load …: SurfaceObjectMismatch`, 0 programs, 0 netns |
| `UPC_SLICE_STRIDE=0x600000 --unsafe-geometry` | `SliceOverlap: pid 1 slice [0x1000000, 0x1800000) overlaps slice of pid 2 [0x1600000, 0x1E00000)`, 0 programs |
| positive control (shipped object, no override) | boots, no refusal |

Master plan 1.2.5 GATE closed. Evidence: `p4-regression.log`, `p4-neg-surface.txt`, `p4-neg-geometry.txt`.

## Phase 5: required-before-prod (Steps 117-156, 2026-10-28)

**Item 4, the demo gate (Steps 117-121).** `upc-bootctl boot` and `console` refuse a live run unless
`UPC_DEMO_MODE=1`; `--dry-run` never needs it. The check runs before the kernel file is read, so nothing touches BPF
or a netns. Tests: `cmd/upc-bootctl/tests/demo_mode.rs` (5; 3 failed before the change) and a unit test. Deviation:
the plan's "export UPC_DEMO_MODE=1 near the top of the harness" does not work, because sudo resets the environment;
every caller passes it inside sudo (`sudo UPC_DEMO_MODE=1 "$BC" …`): the harness, the S5 helper block (`boot()`, also
fixing its `UPC_VERIFIER_STATS=1 sudo` ordering), CLAUDE.md's boot recipe and wiki/Linux-on-UPC.md. Harness ALL GREEN,
sweep 8/8 SAME; a boot without it prints the refusal and leaves 0 programs.
