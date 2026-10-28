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

**Item 5, the `.rv2mbc` SHA gate default-on and BootParams v3 (Steps 122-127, host side).** One resolver,
`upc_api::rv2mbc_sha` (SHA-256 from Phase 4's `surface_hash::sha256`, no new crate): explicit value → sibling
`<file>.rv2mbc.sha256` (first token, 64 lowercase hex) → refuse `Rv2mbcShaRequired`; a wrong hash is
`Rv2mbcShaMismatch` with both hashes. upc-bootctl checks the kernel's, the `--userland` program's and every resident
program's `.rv2mbc` (`src/rv2mbc.rs`, `check_all`) before the eBPF object is opened, and writes the bytes it hashed;
`--rv2mbc-sha` (else `UPC_RV2MBC_SHA`) applies to the kernel only. doom-runner checks `--rv2mbc` before the ring or any
map. `expected_rv2mbc_sha256` is `Some` on every verified path. BootParams v3: `BOOT_VERSION = 3`, raw kernel `.rv2mbc`
SHA-256 at bytes 64..96 (`reserved[48]` → `rv2mbc_sha256[32]` + `reserved[40]`, offset asserted at compile time).
`--allow-unverified-rv2mbc` (bootctl `boot`, doom-runner `run`) only turns a missing hash into a warning, only with
`--dry-run`, and never excuses a mismatch. Tests first: `cmd/upc-bootctl/tests/rv2mbc_sha.rs` (18; RED 16 failed, the
2 positive controls passed), `crates/upc-api/tests/rv2mbc_sha.rs` (10) + 2 unit, bootparams v3 unit test, doom-runner
9 (RED: did not compile). Green: upc-api 62, upc-bootctl 76, doom-runner 44, monad-mbc 393; clippy clean.
Kernel `xv6-mbc.mbc` `f5bf200353fa42d5…` (at `9f22d092`) → `4e39c82f9ba56079…`: `start_mbc.c` accepts version 2 or
3 (a source change, so byte identity breaks by design; it prints nothing new). The kernel `.rv2mbc` follows
(`c02e16c0…` → `dd370941…`). The Makefiles write `<name>.rv2mbc.sha256` in the translate recipe and by a standalone
rule (`kernel`, `userland`, `gates`, `ramdisk` depend on it), so an already-built tree gets sidecars without a
retranslation; `clean` removes them (the userland clean must not glob `$T/*.sha256`: the kernel's sidecar lives there).
`doom/doom.rv2mbc.sha256` = `e7ab7b67bd2ea306…`, equal to a fresh translation of `doom/doom.elf` (force-added: `/doom`
is gitignored). Dry runs with the real images pass for init, gate2, gate_nway, forkloop and uinit; a one-byte change
to the kernel `.rv2mbc` is refused `Rv2mbcShaMismatch`. Deviations: (a) the plan's Step 123 hatch rule ("refused when
`UPC_DEMO_MODE=1` with a network attach") and TTY version log are replaced by the dry-run-only hatch and no new TTY
output; (b) upc-api had no mismatch path to reuse; (c) a live boot without a kernel `.rv2mbc` is now refused
(`Rv2mbcMissing`; it used to warn), since v3 has no zeros on the live path; (d) `crates/upc-bootstub/src/start.c`
also accepts 2|3, because `--bootstub` boots write the same BootParams; (e) doom-runner gained `--dry-run` (the hatch
needs one). Steps 128-129 (harness, sweep) are Stevie's after the merge.
**Item 5 live (Steps 128-129), after merging it with Items 1-3.** Kernel rebuilt (`xv6-mbc.mbc` `4e39c82f…`, the
v2|v3 accept), 11 sidecars written by the Makefiles. Harness ALL GREEN (923645 / 758321), sweep 8/8 SAME. Live
negative: a kernel `.rv2mbc` with one byte changed is refused (`Rv2mbcShaMismatch`, expected `dd370941…`), 0 programs
attached. Evidence `p5-regression-sha.log`, `p5-sweep-sha.txt`, `p5-neg-rv2mbc.txt`.

**Items 1-3: namespace, unprivileged bootctl, seccomp (Steps 131-147).**

*Design (Step 131), changed from the plan's after reading `netns.rs`.* bootctl attached XDP to `veth-upc0` in the
root namespace and sent triggers from inside `upc0` with `ip netns exec upc0 python3 doom-tick.py`. Two plan
assumptions did not hold: (1) the plan's rule `iifname "veth-upc0p" drop` in the root namespace can never match,
because `veth-upc0p` is not in the root namespace (the host side is `veth-upc0`); (2) caps-only cannot work while
triggers go through `ip netns exec` (setns needs CAP_SYS_ADMIN, and it is an execve). New layout:
- `scripts/upc-netns.sh up|down|status` creates `upc0` with **both** veth ends inside it, the old addresses, and
  `table inet unheaded_upc` (forward chain, iif/oif either veth, counter drop). No link to the root namespace exists,
  so nothing the guest emits reaches the host; the rule is defence in depth. The root namespace is not touched.
- bootctl runs inside `upc0` and by default only verifies both veths are visible (`ensure_present`), attaches XDP to
  `veth-upc0` and sends triggers itself over AF_PACKET on `veth-upc0p` (`send_trigger_native`: byte-identical frames
  to doom-tick.py, unit-tested against its `--dry-run` hex; retries ENOBUFS). `--create-netns` keeps the old path.
- On EPERM/EACCES bootctl names the capabilities it needs.

*Evidence.*

| Check | Result |
|---|---|
| root boot inside upc0, native sender | `echo inside-ok` runs, 4,000,000 frames sent (first try hit ENOBUFS at 273,615: fixed with a retry) |
| non-root (`govan`), caps only, NoNewPrivileges, in upc0 | `caps-ok` with CAP_BPF CAP_NET_ADMIN CAP_NET_RAW **CAP_PERFMON**; without PERFMON the verifier refuses the load (EACCES after 19,404 insns); dropping any one of the four fails |
| forward-drop (forwarding enabled in upc0 for the test) | with the rule: counter 3, 3 packets out; without it: 192 transmissions (the 3 pings forwarded back and forth until hop limit). Forwarding restored to 0 |
| seccomp `@system-service bpf` + `~@privileged` | killed: bpf(2) is in @privileged, so the deny removes it again |
| seccomp `@system-service bpf` | `seccomp-ok` |
| `~execve execveat` in SystemCallFilter | the service never starts: it also blocks ExecStart |
| `NoExecPaths=/` + `ExecPaths=<bootctl> /usr/lib /lib /lib64` | `seccomp-ok`; `--create-netns` under it fails `ip netns add upc0: Permission denied` (exec blocked) |
| oneshot unit's properties (transient) | first draft failed: PrivateTmp/ProtectSystem etc. give the unit a private mount namespace, so `/run/netns/upc0` existed on the host with no namespace behind it ("Peer netns reference is invalid"); mount-namespace properties removed, then good |
| bootctl unit's full sandbox (transient; User=govan, ProtectHome=read-only to reach the dev images) | `unit-ok` |
| harness with every boot inside upc0 | ALL GREEN (all 23 checks), 923645 / 758321; harness brings the namespace up and down |
| sweep | 8/8 SAME |

Deviations: Step 137 ran as `govan`, not `nobody` (`/home/govan` is 0750); the plan's Step 140 addresses
(`fd00:upc::`) are not valid IPv6; uid **16791** (16790 is planned for unheaded-doom in S-DOOM-LAST).
Units: `deploy/systemd/unheaded-upc-netns.service`, `unheaded-upc-bootctl@.service` (+ `demo.conf.example`),
sysusers `unheaded-upc 16791`, `pkg/uids` `UPC`. Not installed (E6).

## E6 (Step 148): filed

West host window for UPC isolation, adjusted to what Phase 5 measured. Install on west (a) `deploy/sysusers.d/unheaded.conf`
→ `/etc/sysusers.d/` + `systemd-sysusers` (adds `unheaded-upc`, uid 16791); (b) `scripts/upc-netns.sh` →
`/usr/local/libexec/unheaded/` and `unheaded-upc-netns.service` enabled (creates netns `upc0` at boot; nothing in the
root namespace changes, unlike the plan's host nft rule); (c) `unheaded-upc-bootctl@.service` installed, not enabled,
with `/etc/unheaded/upc/xv6.env` and the images + sidecars + object in `/var/lib/unheaded/upc`. Recommendation: yes.
Undo: `systemctl disable --now unheaded-upc-netns; rm /etc/systemd/system/unheaded-upc-*; userdel unheaded-upc`.
Until answered Steps 149-154 are skipped; FND-028 stays open.

Phase 5 exit gate (Step 156): Items 4 and 5 run in the harness; Items 1-3 proven inside upc0 (Steps 138, 141, 144);
harness green, sweep 8/8 SAME, budgets unchanged (923645 / 758321). M-224 DONE in code; FND-028 open for the host.

## Phase 6

**Freeze (Steps 157-161, 166-168; host side on `sprint/ascend-2-freeze`).** Freeze sequence: `.rv2mbc` SHA gate →
object preflight → `validate` + `validate_against_object` → ROM_MAP / RV2MBC_MAP writes → `ImageSink::finalize`
(`bpf(BPF_MAP_FREEZE)` on both maps) → XDP attach. aya 0.13.1 has no public freeze (`MapData::finalize` is
`pub(crate)` and freezes `.rodata` maps only), so both loaders call bpf(2) through `libc::syscall` on the fd aya
holds (`cmd/upc-bootctl/src/freeze.rs`, `crates/doom-runner/src/freeze.rs`; libc was already in both lockfiles).
`upc_api::load` = gates, writes, finalize (doom-runner, one image). bootctl writes the kernel (and bootstub) image
and the kernel `.rv2mbc` outside `upc_api`, then init and the resident programs, so its two `load` calls became
`upc_api::write_image` and one `upc_api::finalize(&mut runner)` sits in `boot_prepare` after the last of them,
before `netns::prepare` and `attach_xdp`; a failed freeze is `LoadError::Finalize` and the boot stops before attach.
The program never writes either map (`ROM_MAP.get`, `RV2MBC_MAP.get` only) and declares both with flags 0: no
`BPF_F_RDONLY_PROG` (adding it is an eBPF change, not made here). Tests first: `crates/upc-api/tests/freeze.rs` (9;
RED: did not compile), `cmd/upc-bootctl/tests/freeze_order.rs` (2, source order write < freeze < attach; RED: 2
failed), `freeze_live.rs` (`#[ignore]`, `UPC_LIVE=1`). Green: upc-api 71, upc-bootctl 86 + 1 ignored, doom-runner 46;
clippy clean. Deviations: (a) the plan's `bpftool map update pinned` probe cannot work, since neither loader pins a
map and a dry boot touches none; `boot --freeze-check` instead writes slot 0 of each map back with its own value after
the freeze and fails the boot unless both return EPERM (`freeze check: <MAP> update refused (EPERM)`); `--keep-pins`
not added; (b) Step 166's override applied: `cmd/trace-collector-go` and `cmd/dashboard-backend` import `pkg/ebpf`, so
only `mapfreeze.go` + its test are deleted (it also claimed a freeze blocks program-side writes; it does not); no
unreached-list tool exists; (c) doom-runner loads the program but attaches no XDP itself; its freeze precedes
`verify_maps`. ADR-080 §7. Steps 162-165 (live EPERM, harness) are Stevie's after the merge.

**HMAC default (Steps 169-171).** `cmd/doom load --hmac` and `wotan-ctl doom load --hmac` defaulted the key to a
literal compiled into the binary. Now `internal/doom.ResolveHMACKey`: `--hmac-key`, else `UPC_HMAC_KEY`, else a loud
error. Only `--hmac` needs a key; no script or unit runs it. 0 literals left in Go (`b993efbf`).

**Instance ownership (Steps 172-178).** `INSTANCE_OWNER` (Array<u32>, 256): bit 31 BOUND, bits 0-11 the owner's
nonce; checked right after the instance is derived, before CPU_MAP. bootctl draws the nonce with getrandom, binds
before attach, puts it in flow-label bits 8-19 of every trigger, and prints `hijack_drop=` in its STATS line. Not an
ABI change: a new map, MbcCpuState untouched. Test hook `boot --hijack-probe N` (harness never sets it):

| Run (forkloop guest, spinning) | ticks seen | insns | hijack_drop |
|---|---|---|---|
| 20,000 sent, wrong nonce | 10,000 | 0 | 10,000 |
| 20,000 sent, owner nonce | 10,000 | 160,000 | 0 |

The first probe ran on a guest blocked in `read` and showed 0 instructions for owner ticks too: a blocked guest
executes nothing, so the probe needs a busy guest. **Observation, not new:** only half the frames sent reach XDP as
CPU ticks, on the native sender and on the old python path alike (same probe with `--create-netns`); each counted
tick runs 16 instructions. Not investigated further; it halves trigger throughput everywhere and predates this sprint.

Verified instructions: ascend 923,645 → **923,826** (+181), Doom 758,321 → **758,199** (−122); load gate 36 PASS,
baseline updated. Harness ALL GREEN, sweep 8/8 SAME. doom-runner does not bind (Doom unchecked, as before): the plan's
"doom-runner also writes the owner record" is not done, since Doom's injector would need the nonce too.
Security brief updated (Step 178).

**Freeze live (Steps 162-165), after merging it with the hijack work.** `boot --freeze-check`: `ROM_MAP + RV2MBC_MAP
frozen (BPF_MAP_FREEZE)`, then `freeze check: ROM_MAP update refused (EPERM)` and the same for RV2MBC_MAP, then
FIRST HEARTBEAT. `tests/freeze_live.rs` with `UPC_LIVE=1`: 1 passed. Harness ALL GREEN (923826 / 758199, unchanged by
the freeze), sweep 8/8 SAME, nothing left attached. Phase 6 exit gate passed.

## Phase 7: FS substrate (Steps 181-204, 2026-10-28)

**Route (Steps 181-182).** Ascend is at 923,826 (92.4 %); the subdir walk cost +41K, a second variable offset +119K.
FS writes + `>` and pipes go **kernel-side through EVOLUTION-1 Stage 2** (decision 48 default; E2 is open and only an
answer of "(c) minimal" plus wanting `>` first would reopen it). Recorded in the master plan 1.3.2/1.3.3, the
Stage 2 skeleton (rungs 6 and 8) and OUTSTANDING M-226.

**Device inodes (Steps 183-190), smaller than planned.** H10's open question answered: `open` is serviced in BPF
(syscall 15); the kernel's `sys_open` with its T_DEVICE branch never runs. The BPF handler already matches
`"console"` by name and returns the console fd. `mkfs` writes no device inode and `mknod` is a stub returning 0, so
fs.img holds no T_DEVICE inode at all: routing one in BPF would be code nothing can exercise, and adding one to
fs.img changes the `ls` golden. Done instead: `user/devprobe.c` (as init, with no fd open: `open("console", O_RDWR)`,
write a token through that fd, then through a dup of it) and a harness gate. Deferred to Stage 2 rung 6 (devsw) with
mknod: on-disk device inodes. No eBPF change, no golden change. Step 185's host-emulator test was not written: the
live harness gate runs the real BPF path. The Phase 1.4 PC tracer (100 PCs printed after every SRET) splits the bytes
of a write that follows an SRET, so the gate strips `([0-9A-F]{4} )+` before matching; removing the tracer would
change all 12 goldens, left for a recapture window.

**ilp32e (Steps 191-196).** 21 `static inline uint64` CSR readers and 18 `(uint64 x)` writers in `riscv.h` are now
`uint32`; `types.h` asserts `sizeof(uint64) == 4` with the reason (struct stat's layout, Phase 1.7), and `pde_t` /
`pte_t` carry a comment (Sv32: 4-byte entries are right). `unsigned long` and `unsigned int` are both 4 bytes here,
so a clean kernel rebuild (33 compiles) is **byte-identical** (`4e39c82f…`): the change is to what the names claim.

Results: harness ALL GREEN with the devprobe gate, sweep 8/8 SAME, monad-mbc 393 and xv6-mbc 3 tests pass, budgets
unchanged (923,826 / 758,199). M-226 done in part (writes, pipes, on-disk device inodes routed), M-227 done.

## Phase 8: Mímir and the distributed leg (Steps 205-242, 2026-10-28)

**E3 (Step 205): filed, unchanged from the plan.** (1) approve S5 Step 177 (heimdall-daemon as root from /tmp on east,
XDP generic on `enp2s0`, observe-only; scratch Wotan on west bound to 192.168.13.2)? (2) approve S5 Step 435 (scratch
Wotan reachable from east; `upc-cartridgectl` + `cartridge-ebpf` as root on east from /tmp, removed at phase end)?
(3) or drop Phase 18 for good? No recommendation: east's host actions are Stevie's. Until answered, the two-netns
fallback runs and Steps 233-238 are skipped.

**Topology deviation.** The plan's fallback bridge `br-upc` lives in the root namespace; this session changes nothing
there, so the bridge and the scratch Wotan live in a third namespace (`lab-hub`), with the two nodes in `lab-a` and
`lab-b` (`scripts/lab-two-node.sh`). `upc0` keeps its Phase 5 layout. Heimdall has no "await bootstrap / seed
operational" flow (Step 212's premise): a BootstrapBroadcast is classified and logged by the XDP observer's policy and a
ReverifyUnicast re-verifies the manifest and scans, so Phase 9 here measures horn delivery and classification.

**LICH-015 full-engine arm (Steps 226-232): not done this session.**

**Mímir Phases 9, 11, 13 re-run (Steps 206-219), topology "2 netns on one bridge, not across hosts".**
`scripts/lab-two-node.sh` (lab-a, lab-b, bridge + scratch Wotan in lab-hub). Lab keys and the lab Wotan config live in
`~/tmp/s-ascend-2/p8/` only; the lab manifest covers 20 scratch files. heimdall ran in lab-b with
`AUTH_KEY_FILE` pointed at a missing file, so west's real `/etc/unheaded/keys/heimdall.key` was never read (the lab
Wotan runs with auth off). Hooks loaded: vfs_write 20, execve 21, Gjallarhorn XDP 292 verified instructions.

| Check | Result |
|---|---|
| K2: BootstrapBroadcast multicast lab-a → lab-b, 7 runs, send to classified | 18, 11, 11, 10, 10, 11, 10 ms (p50 11 ms, max 18 ms; bar 10 s) |
| reverify, this cluster | `action=reverify`, "re-verified manifest and scanned" |
| reverify, cluster 2 | `action=other-cluster` |
| 100 reverifies at ~10/s | 13 reverify, 87 rate-limited (1/s, burst 3); daemon alive |
| tampered manifest at startup | refused: `gungnir: payload hash mismatch`, exit 1 |
| expired seal (1 s TTL) at startup | refused: `gungnir: seal expired`, exit 1 |
| K3: drift, f1-f10 changed together | alerts on exactly f1-f10, none on f11-f20 |
| K3: drift, f11-f20 one at a time | 30-264 ms each (all ≤ 1 s) |
| K4: alerts only | no restore; alerts repeat each scan while the drift stays |
| K4: signed drift events | 90 messages on `drift.detected.seed-netns-b`, all published over gRPC; an unsigned HTTP publish to the same topic gets 403 `SignatureRequired` |

The plan's negatives "unknown sha" and "older manifest" do not apply: a horn is unauthenticated by design and can
only ask a node to re-check a manifest it already trusts, so the negatives that exist are the cluster, the rate
limit and the seal checks above. Verdict: Phases 9/11/13 PASS in the netns topology; across hosts waits on E3.
Teardown: no heimdall program, no lab namespace, no process left.

**weights publish/fetch (Step 221, [CODE] branch, 2026-10-28).** `upc-cartridgectl` had no publish/fetch, so
`weights publish|fetch` was added (`cmd/upc-cartridgectl/weights.go`). Publish decodes the `.cart` with
`cartridge.DecodeImage` (embedded sha checked), takes the DATA section, and seals a JSON manifest
`{v:1, model, data_sha256, data_words, image_sha256, data}` with Gungnir ML-DSA-65 (`SignFor`, default ttl 7 d, the
same `GungnirSeal` JSON as a `.cart.seal`). The Wotan payload is `{type:"upc.cart.weights/v1", manifest:<base64 of
the signed bytes>, seal}`, one message on `upc.cart.weights.<model>` (under `upc.cart.*`, RFC-EDITOR-REQUESTS-S5
item 3). There was no DATA-only seal format to reuse: the loader's seal covers the whole image. Fetch walks the topic
to its newest message and refuses, before writing anything, an empty topic, a non-envelope, a seal that fails
(wrong key, tampered manifest, expired), a manifest for another model (a sealed envelope replayed onto another
topic), a data length that is not whole words or over one slot, and a data sha that differs from the manifest's;
then writes `<out>/<model>.data.bin` 0600 by rename and prints `seal verified` with the sha. The newest message is
refused, not skipped, if it does not verify. Model names are `[a-z0-9][a-z0-9_-]{0,62}` (one topic segment, one
file name). Wotan identity `upc-cartridgectl` (`InitServiceIdentity`; a Wotan must list it in
`topics.auto_approve`, not added to `configs/wotan.yaml`). 13 tests (envelope good / tampered / wrong key / wrong
model / sha mismatch / expired / garbage; httptest fake Wotan round trip, paging, refusals). Manual: scratch Wotan
(auth off, `auto_approve: [upc-cartridgectl]`) in `unshare -rn` on 26601/26602, real `scorer.cart` from `cart build`:
published data_sha256 `f175ee26…fa38b`, fetched bytes identical to `cartridges/scorer.data.bin` (`uniq | wc -l` = 1),
mode 600; wrong key, empty topic and a junk newest message each exit 1 with nothing written. Steps 222-225 (netns,
sudo) not run.

**DISTRIBUTED leg (Steps 222-225).** DISTRIBUTED scorer weights: cross-node demonstrated in netns, not across hosts.
Lab Wotan in lab-hub; `weights publish` from lab-a (`.cart` built with `cart build` from `cartridges/scorer.s` +
`scorer.data.bin`: the `.cart` is not in the repo), `weights fetch` in lab-b: `seal verified (ml-dsa-65, key
8c67a26796eac091)`, `data_sha256=f175ee26…`, equal to the repo's `scorer.data.bin`. A cart rebuilt from node B's DATA has
the same image sha (`ed8dd3b0…`); `cart tick --jobs` over the same 200 fixed-seed jobs gives identical output on both
(400 lines, 0 diff). Step 223's `cart score --data` does not exist; `cart tick` stands in. Caution: one byte changed at
DATA offset 40 changed none of the 200 scores, so equal scores alone are weak evidence; the equal DATA and image hashes
carry it. Seal-key reuse: the lab seal key signed both the manifest and the weights (lab only). Teardown clean.

## Phase 9: cartridge live legs and the scorer (Steps 243-266, 2026-10-28)

**Decisions 243-244: defaults taken.** Gates #1(a) and #2 accepted as the documented interpreter cost; no ISA sprint
(recorded in ADR-108 §Status at S5 close).

**Step 245: 8 steps per callback, measured and reverted** (`STEPS_PER_CB` in `ebpf/cartridge-ebpf`; CPU 2 pinned;
`p9-bench-{4,8}.txt`):

| | 4 (shipped) | 8 |
|---|---|---|
| rate limiter over b0_pass_v6 | +197 ns | +172 ns |
| sampler over b0_pass_tcp | +226 ns | +208 ns |
| scorer | 227,842 flows/s | 264,550 flows/s |
| cart_xdp / cart_tick verified insns | 42,743 / 146,897 | 85,049 / 318,333 |

Doubling the batch saves 12-14 % per packet and doubles the verified size; both gates still miss (bars 150 and
120 ns). Supports the decision. Reverted: `ebpf/` matches HEAD (Step 246).

**Live legs: the plan's compose path is not used.** Steps 247 and 251 start Wotan and Sophia from west's compose stack;
this session does not run `docker compose up` on west (the running containers are the old ones; next.md's BEFORE
warning), so the live legs run against scratch services in the lab namespaces instead.

**Live legs (Steps 247-253): not done.** `cmd/anomaly` reads scores from the pinned cartridge interpreter's ring
buffer, so `anomaly.scores` on a live Wotan needs the whole pipeline running (cartridge-ebpf pinned on bpffs, sampler +
scorer loaded, traffic through an XDP interface), and the Sophia leg needs a Sophia. The compose path is excluded (see
above); a scratch-service rig was not built this session. KF5 ("stack down") stays open.

**Scorer retrain (Steps 254-262): STUCK on data (the plan's Step 259 fallback).** No real flow capture exists: the
Real Steel game day kept one scenario pcap for flow 0x5EE1 (`docs/gamedays/2026-10-26-real-steel-1.md`), and no
dataset or trainer scripts exist under `scripts/cartridge/`. Decision 255's default stands in intent; v1 stays shipped
as a synthetic demonstrator. Recorded in ADR-108. Needs: a steel-echo window with the sampler's flow export on.

Phase 9 exit: decisions 243/244 recorded, 245 measured and reverted, live legs and retrain not done; no compose
service started; tree clean.

## Phase 12: security and compliance review (Steps 293-302, 2026-10-28)

**Attack-surface changes this sprint (Step 293).**

| Change | Surface | Control / evidence |
|---|---|---|
| Scheduler `found` flag (eBPF + emulator) | none new; removes a zombie-resume path | 2 emulator tests; gate_nway, forkloop gates |
| `UPC_DEMO_MODE` gate | refuses live boots | `tests/demo_mode.rs` (5) |
| `.rv2mbc` SHA resolver + sidecars, BootParams v3 | new file and hex parsing; 32 new bytes the kernel reads | `tests/rv2mbc_sha.rs` (18), upc-api tests; live tamper refused |
| `UPC_SURFACE_ID` / `UPC_MMU_GEOMETRY` ELF reader | new ELF parsing in the loader | `fuzz_object_facts` 180 s, 51.7M execs, no crash |
| Slice geometry gate | new arithmetic on declared values | `fuzz_slice_geometry` 180 s, 38.4M execs, no crash; asserts accepted slices are in RAM and disjoint |
| `scripts/upc-netns.sh`, `scripts/lab-two-node.sh` | root scripts; arguments `up|down|status` only | shellcheck clean; the root namespace is never changed |
| Native AF_PACKET trigger sender | raw socket (CAP_NET_RAW) on `veth-upc0p` only | frame unit-tested against doom-tick.py; inside upc0 |
| `unheaded-upc-bootctl@` / `-netns` units, uid 16791 | 4 capabilities; seccomp `@system-service bpf`; exec limited | measured minimal; check-systemd-units PASS; not installed (E6) |
| ROM/RV2MBC freeze | removes userspace writes after load | live EPERM; `freeze_live` test |
| HMAC key resolver | removes a shipped default key | 3 tests; 0 literals in Go |
| `INSTANCE_OWNER` + nonce | new map and compare per tick | live probe; residual (12-bit tripwire, replay) in the security brief |
| `weights publish|fetch` | new Wotan consumer and producer | seal, model, length and sha checked before any write; 13 tests; residual: replay of an older valid envelope |
| `devprobe`, riscv.h types | none new | harness gate; kernel byte-identical |

**Gates (Step 297).** gitleaks (no new secrets, 6,157 files), gosec ratchet, dependency baseline (596), systemd units
(46), runbooks (94), python syntax (162), `make license-check`: all PASS; `go mod tidy -diff` empty. The new fuzz crate
`crates/upc-api/fuzz` adds `libfuzzer-sys` only (as `crates/monad-mbc/fuzz` already does).

**Tests (Step 299).** `go test ./...`: 0 FAIL lines (Phase 0: 0). Rust: upc-api 71, upc-bootctl 88, monad-mbc 393,
xv6-mbc 3, upc-cartridge 40, doom-runner 46; 0 failed.

**Register (Step 301).** FND-028 (UPC isolation) open until E6; FND-029 (flow-label instance hijack) resolved.

## Close (2026-10-28)

| Budget | Phase 0 | Close |
|---|---|---|
| ascend monad_cpu | 926,542 | 923,826 |
| Doom monad_cpu | 761,580 | 758,199 |
| cart_xdp / cart_tick | 42,743 / 146,897 | 42,743 / 146,897 |

Final regression on the final tree: harness ALL GREEN (now with gate_nway, forkloop and devprobe), sweep 8/8 SAME plus
the harness's other golden sets, 0 programs attached, 0 network namespaces. West is as Step 22 found it: nothing
installed (E6 open). Commits since the merge base with develop: 82 (includes S-TRUE-WIRE, which this branch was cut
from).

| KF | At start | At close |
|---|---|---|
| KF1 gate_nway | 0 ok, 231 FAIL pid2 | retired (scheduler zombie-resume fix) |

**Escalations at close.** E1 pid numbering: open (flag off). E2 Stage 2 reach: open (skeleton marked pending). E3 east
as a node: open (two-netns fallback ran). E4 naming, E5 attended run: Phase 11 not run. E6 west install: open (units
committed). Carried from S-TRUE-WIRE: E1-E8 there, including its E2 merge gate, which this branch inherits.

### STUCK / not done

- **Phase 1 Steps 44-51**: wait on E1.
- **Phase 2 Steps 63-65**: wait on E2.
- **Phase 5 Steps 149-154**: wait on E6.
- **Phase 8 Steps 226-232 (LICH-015 full-engine arm)**: not done this session. Steps 233-238 wait on E3.
- **Phase 9 Steps 247-253 (live legs)**: need the live cartridge pipeline; west's compose stack is not started.
  **Steps 254-262 (scorer retrain)**: no real flow capture exists.
- **Phase 10 (LICH-012 L12a/L12d, Enkrateia v2, semantic drift) and Phase 11 (naming, statuses, attended run)**: not
  run this session.
- **Phase 13**: wiki ripple done (Linux-on-UPC, UPC-Dream-Ladder, Mimirs-Law, UPC-Cartridges, Systemd-Units); the
  RFC requests file notes the `upc.cart.weights.<model>` sub-topic; ADR-INDEX unchanged (no ADR changed status).

## Phase 11, doc parts (Steps 281-292, run after the close)

- **Step 281 (bootstub): recommendation stands, not applied unattended.** Nothing builds `crates/upc-bootstub` as a
  dependency, but `upc-bootctl --bootstub` and upc-api's stub boot protocol still name it, and Item 5 updated its
  `start.c` to accept BootParams v3 this sprint. Retiring it means removing the flag, the protocol variant and the
  crate together; left for an attended session (Steps 281-282 open).
- **Step 283:** ADR-023 already said "Superseded by ADR-074"; ADR-081's naming item points at E4.
- **Step 284 E4: filed** (is `unheadnix` the name? Lore and Stevie). Steps 285-286 wait on it.
- **Step 287:** the Dream Ladder's L6 text already followed ADR-081; three `docs/doom/MBC_LINUX_*` docs gained a
  "superseded by ADR-081" banner (the FUZIX, uClinux and road-to-Linux docs had one).
- **Step 289:** attended-run recipe in `wiki/Linux-on-UPC.md`. **Finding:** a default boot now runs inside `upc0`,
  which has no link to the host, so `console` cannot reach `upc-tty-bridge` on the host's loopback; Mode A uses
  `--create-netns` until the bridge gets a path into `upc0` (for example a Unix socket bind-mounted into the unit).
- **Step 290 E5: filed** (a ~20-minute attended window with the GPU); Step 291 waits on it.

## Answers (Stevie, 2026-10-28)

- **E1: option K** (kernel numbering, init = 1). Steps 44-51 proceed.
- **E2: (a)**, push toward kernel-authoritative syscalls, rung by rung with the freed-instructions stop rule.
- **E3: scaffold, observe, research and plan first; expand to east with caution, with an auto-remediation path if
  something goes wrong.** Read-only observation of east and a written plan come first; nothing is attached on east
  until the plan and its safety net are reviewed.
- **E4: no rename.** "Unheaded Linux", "unheadnix" (the shorter form) and "uhl" (abbreviation) all stand; the
  community can settle on one if the project gains traction. Steps 285-286 are closed as not needed.
- **E5: deferred** (no time now). The recipe stays in `wiki/Linux-on-UPC.md`.
- **E6: not answered yet.** Still open; the units stay committed and not installed.

## E6 installed on west (Steps 149-154, 2026-10-28)

- Account: `/etc/sysusers.d/unheaded-upc.conf` with only the `unheaded-upc` line (uid/gid 16791), applied with
  `systemd-sysusers` on that file. The repo's full `unheaded.conf` differs from the installed one by 7 more accounts
  (vector, heartbeat, ci-status, well-retention, auth, heimdall, buildkey), which E6 did not cover, so it was not
  copied over.
- `upc-netns.sh` → `/usr/local/libexec/unheaded/`; `unheaded-upc-netns.service` enabled and active: `upc0` with both
  veths and `table inet unheaded_upc`; the root namespace gained no interface.
- `/usr/local/bin/upc-bootctl`; `unheaded-upc-bootctl@.service` + `demo.conf` drop-in (not enabled);
  `/etc/unheaded/upc/xv6.env` (0640 root:unheaded-upc); `/var/lib/unheaded/upc` (0750 unheaded-upc): kernel, fs.img,
  init/sh/ls/cat/echo/wc with their `.rv2mbc`, `.data` and `.rv2mbc.sha256`, and the ascend object.
- `systemctl start unheaded-upc-bootctl@xv6`: process user `unheaded-upc`, result `success`; journal: `xv6 booting`,
  nonce bound, ROM/RV2MBC frozen, FIRST HEARTBEAT, no EPERM; 0 programs attached afterwards.
- Undo: `systemctl disable --now unheaded-upc-netns; rm /etc/systemd/system/unheaded-upc-*;
  rm -r /etc/systemd/system/unheaded-upc-bootctl@.service.d /etc/unheaded/upc /var/lib/unheaded/upc;
  rm /usr/local/bin/upc-bootctl /usr/local/libexec/unheaded/upc-netns.sh /etc/sysusers.d/unheaded-upc.conf;
  userdel unheaded-upc`.
- The images in `/var/lib/unheaded/upc` are copies: after a kernel or userland rebuild, re-copy them with their
  sidecars, or the SHA gate refuses the boot.

## E1 option K applied (Steps 44-52, 2026-10-28)

- **Numbering.** BPF reports pid = slot + 1 at the three places a pid reaches user code: `getpid` (flag off), the
  parent's `fork()` return and `wait()`'s return. No PROC_TABLE word was added: slot + 1 is a function, not a stored
  field, so the table stays 22 words (deviation from the plan's "pid field"; pids reuse with slots, unlike xv6's
  monotonic counter). The kernel already numbers init 1 (`nextpid = 1`), so the books agree on init.
- **Test first.** `getpidfork` now also prints `fork-ret` and `wait-ret` and is a harness gate expecting parent 1,
  child 2, fork 2, wait 2. Before: 0 / 1 / 1 / 1 (RED). After: 1 / 2 / 2 / 2.
- **Flag ON (Step 45).** parent 1 (kernel), child `other` (-1), fork 2, wait 2. Step 46's "ON == OFF on all lines"
  fails only on the child, for the known reason (the kernel has no process for a BPF-forked child). That is Stage 2
  rung 0, so Steps 47-52 do not flip the flag: it stays OFF (the plan's exit (b) for this part).
- **Goldens.** No golden prints a pid: sweep 8/8 SAME and the harness's other golden sets pass, so no recapture (the
  plan expected one).
- **Budget.** ascend 923,826 → **924,950** (+1,124); flag-ON build 910,133. Baseline and over-budget note updated.
- West's installed object (`/var/lib/unheaded/upc/monad-cpu-ebpf`) refreshed to this build.
- Comments that said "init (pid 0)" in gate2, gate_nway, devprobe, getpidfork and getpidprobe now say pid 1 (slot 0).

## E2 (a) recorded (Steps 63-65)

EVOLUTION-1 §D1 re-read and the Stage 2 skeleton say "DECIDED (a)"; master plan 1.6.4 notes it. The next UPC work
is rung 0 (master plan 1.6.3) and forging the Stage 2 battle plan (1.6.4).

## E3: observe, research, plan, safety net (2026-10-28)

Plan: `docs/battle-plans/S-ASCEND-2-EAST-NODE-plan.md` (stages S0-S4, each starts on Stevie's go). Done:
read-only observation of east (sole live link `enp2s0`, default route via west, `wg0` over it; no XDP; bpftool;
passwordless sudo; chaos-controller active); `scripts/node-guard.sh`, a dead-man switch run as a transient systemd unit
(peer-loss or lease expiry → kill lab processes, unpin, detach XDP by netlink and by link, verify), tested in the lab
(link cut: remediated after 3 misses, ~8 s; lease expiry; disarm leaves work alone) and self-tested on east with a
20 s lease and nothing attached (no misses, no-op remediation, removed after). The first lab run showed netlink cannot
detach aya's link-based XDP, so killing the owner comes first. Wotan gained `-bind` (default unchanged), so a scratch
Wotan can listen on 192.168.13.2 only (checked: both listeners on the given address). The Gjallarhorn XDP program
returns XDP_PASS on every path. Open question in the plan: out-of-band access to east.

## E3 across hosts (S0-S4, 2026-10-28)

Run after Stevie confirmed physical access to east and said to churn unattended. The guard stayed armed on east for
every stage (fail-after 5, 20 min leases, renewed); it never fired.

| Stage | Result |
|---|---|
| S0 stage | fresh lab keys (east-seal, east-drift, in `~/tmp/s-ascend-2/p8e` and `/tmp/s6` only), manifest over 20 east scratch files sealed 6 h; 29 files identical by sha256 on both ends |
| S1 heimdall, kprobes only | seal verified; f1-f10 changed together → alerts on exactly those; f11-f20 one at a time, 78-260 ms each; 66 signed drift events on the west scratch Wotan (`-bind 192.168.13.2`, listeners on that address only); unsigned publish 403; tampered manifest and 1 s expired seal refused at startup |
| S2 XDP generic on `enp2s0` | attached (prog id 31907); 2-minute watch: every ping, SSH session and guard check fine; 7/7 multicast bootstrap horns, 8.4-15.2 ms send-to-classified across two chrony clocks; reverify 14 (incl. 13 from the flood), other-cluster 1, rate-limited 87 of 100; heimdall stopped, no XDP left |
| S3 weights | published on west, fetched on east: seal verified, DATA sha `f175ee26…` equal to the repo; cart rebuilt on east has image sha `ed8dd3b0…` (same); 200-job `cart tick` output identical on both (0 diff lines) |
| S4 teardown | east: `/tmp/s6*` removed, 0 lab processes, 0 transient units, no XDP, none of our programs; chaos-controller still active. West: scratch Wotan stopped, its listeners gone |

**Findings on hardware the lab could not show.**
1. **Multicast needs the group joined.** The first 7 horns never reached XDP on east: a real NIC filters frames for
   multicast groups nobody on the host joined, and veth interfaces do not. Heimdall only observes through XDP and joins
   nothing, so the bootstrap only arrived once `gjallarhorn-listener -multicast` ran on east. A deployed seed must join
   `ff02::1:abba`. **Fixed the same day:** with `--xdp-iface`, heimdall joins the group on that interface itself
   (`cmd/heimdall-daemon/hornjoin.go`; the constants moved to `pkg/gjallarhorn`, shared with the sender). Lab: the
   group appears in `ip -6 maddr` while heimdall runs and is gone after it exits. East (guarded, no listener): 3/3
   multicast horns observed; east cleaned again.
2. **Unicast needed a zoned link-local target.** The observer parses IPv6 only and east's `enp2s0` has only a
   link-local IPv6 address; the sender fell back to IPv4. `gjallarhorn-sender -target fe80::…%iface` now works
   (new `unicastAddr`, test-first; a zone is refused on non-link-local addresses).
3. My first teardown missed the scratch Wotan (started as `./wotan`; the kill pattern used the full path); stopped by
   pid after checking its command line and working directory.
