# S-STAGE-2B log

Started 2026-11-04 on 42d11e2f (branch sprint/s-stage-2b).
Plan: docs/battle-plans/S-STAGE-2B-battle-plan.md

## Run cost

Step 9, one `TRIG=3000 ub getpidfork` boot, verified 868290 insns:

- before: 10442 MiB
- min MemAvailable during boot: 10201 MiB
- eBPF build Maximum resident set size (kbytes): 180172 (wall 0:00.75; `touch` on main.rs first, because Step 7's own build was a no-op at 35200 kB after the interrupted run)

This section sits above the Ledger because `ledger` appends to the end of the file.

## Ledger (verified insns, ascend)

| Rung | OFF | ON | delta | net vs 921,036 | gate |
|---|---|---|---|---|---|
| rung 2 uptime (DP-717) | 868290 | 872276 | +3986 | 48760 | rung2-after |
net since rung 0: 48760

Step 52, probes built (7 .mbc present). sh.mbc and ls.mbc sha256 prefixes: 1c161377c8b23196 d2ade4a5942a3cdb

Step 57, gate probes: same=12 diff=0 rc=0, ascend 872276 (equal to Stop A), Doom 758199, 7 rung gates skipped behind their consts.
| SYS_yield 23 (DP-413) | 872276 | 870122 | -2154 | 50914 | p413-after |

Step 66-68, SYS_yield form: OFF 872276. Alias row plus empty arm (Step 61 as written): 876146, +3870, over the ±500 bar. Arm for 13 as `syscall_nr == 13 || syscall_nr == 23`, no alias row, no empty arm (Step 68): 870122, -2154. Kept the second. Why the count fell below OFF was not investigated; behaviour held: gate p413-after same=12 diff=0 rc=0, Doom 758199, 7 rung gates skipped.

Step 69, boots: gate_nway 3 NWAY-ok, getpidmulti 13 distinct GM lines. Order c1-pid, c2-pid, c3-pid, then c1-again, c2-again, c3-again: 23 really switches, since a silently ignored ecall would run each child straight through.

Step 88, DP-503 object: OFF 870122, ON 870702, +580 (bar +1,500), `BUDGET-OK`. Honest non-ascend object (`--no-default-features --target-dir target/honest`) compiles; compile-checked only, nothing ships it.

Step 90-91, the sbrk rung's RED: `SB: init-refused=no`, `SB: base=no`, `SB: grow=yes`, `SB: moved=no`. The first two would read the same under the old fall-through (sbrk(n) returned n). `grow=yes` is the line that tells them apart: sbrk(0) and sbrk(0x19000) gave the same answer, -38, where the fall-through gave 0 and 0x19000.
| honest ENOSYS (DP-503) | 870122 | 870702 | +580 | 50334 | p503-after |

Step 106-107, DP-414 mutation: `XV6_TEXT_RV_WORD_BASE` 0x8001 boots to `BOOT FAIL: rv text base` (1 hit) and `parity_bootparams` fails; restored, the file's hash matches the pre-mutant one.

Step 108, sidecar refusal: with `xv6-mbc.data` gone, `Error: no kernel .data sidecar at ...`, exit before attach. A 2-byte `.data` is refused too (`failed to load: data image too short`). `getpidfork` boots again after, `F: parent-pid=1`; both sidecars hash as before.

Step 110, frozen-ABI check: `parity_bootparams` 4 passed; `BOOT_VERSION` 3; 0 `pub` field lines changed in `bootparams.rs`, so offsets 0..100 stand; `upc-bootstub/src/start.c` names `reserved` only in the struct; 0 diff lines under `crates/doom-runner`.
| BootParams base, sidecar (DP-414) | 870702 | 870702 | +0 | 50334 | p414-after |

## Phase 10, reply channel ADR review (ADR-135)

Step 119 did not run as written. The plan wants a separate read-only review agent; this session had no way to start one (no agent tool, and a nested `claude` from the shell was not something any rule sanctioned, so it was not tried). The same agent that wrote the ADR ran the brief read-only: no sudo, no build. That is a self-review, and a separate one is still owed. The brief is at plan Step 119.

Answers to the brief's four questions:

- Can a child forge the reply word? Yes while D6 is off: its pgd is writable through a page-walk miss, so it can map kernel `.bss`. The forged word is wiped at trap entry; moving the base at `0x160` gets past that, but it buys only what `kill(2)` already gives. With D6 on, no. See F2.
- Does a retry double-apply anything? Not in BPF or `usertrap`: the reply clears on read, `epc` is undone, and a7 survives through gp. It would in a handler that changes state before replying, so D7 now says it may not (F3). The visible effect is the `S` and tracer bytes on each SRET (F4).
- Can a heap byte leak between processes? Between live processes yes, through the D6 hole (F1). From a dead occupant's heap to the next, no: fork overwrites the band. D5 is not a confidentiality control (F8).
- What breaks if a kill lands while the target's fork copy is mid-flight? Nothing today, because it cannot land: no timer switch under `ascend-linux`, so only the forking parent runs during the copy (new I9, F5). With preemption, killing the parent mid-copy leaves pde 0 present, and the next fork on that slot skips its cursor reset.

| Id | Severity | Against | Finding | Disposition |
|---|---|---|---|---|
| F1 | High, pre-existing | D2, D5 | With D6 off a child reads and writes another child's slice at VA >= 8 MiB, heap included | Accepted: stays with D6 (Stevie: not yet). ADR Security notes say so; For Stevie line |
| F2 | Medium | D1 | The forged text never said when the kernel clears `reply`, or what BPF bounds | Fixed: D1 clears `reply` and `reply_arg` before the tag check, exactly one flag per reply; Security notes list the bounds. Step 124 must clear before the tag check |
| F3 | Medium | D7 | No rule stopped a handler changing state before a retry | Fixed: D7 gains four handler rules |
| F4 | Medium | D7 | Each retry's SRET writes `S` and arms the tracer, about 500 TTY bytes on a 4 KiB ring, and D14 lands one rung after sleep | Accepted with a check: D7 states it; Phase 17's gate must show its probe lines survive. For Stevie line |
| F5 | Medium | D8 | Liveness was unspecified; plan Step 156's "`num_processes` moves at finalize" misses a reused slot, which only the halted bit covers | Fixed: D8 defines live; I9 added. Zombie gets -1, unlike upstream's 0, recorded |
| F6 | Low | D3 | "The kernel reads `brk_in` at each trap" was wrong (BPF writes each trap, `sys_sbrk` reads); top inclusive unstated; ADR-109 left `sz` to this rung | Fixed: D3 reworded; `sz` stays 0 |
| F7 | Low | D2 | The window assumes user images end below 0x10000; nothing checks it. Measured on the seven `.elf` in `target/`, largest `sh` 0x2084; the probes keep no `.elf` | Accepted: D2 states it; a link-time check is new scope, so a For Stevie line |
| F8 | Low, pre-existing | D5 | "Zero on growth" read as hygiene, but a child reads its whole slice without `sbrk`; nothing scrubs at reap | Accepted: D5 and Security notes say it is correctness only; For Stevie line |
| F9 | Low | D5 | The zeroing is a byte loop in one trap, and no slot runs meanwhile | Accepted: D5 states it; Step 144 records the MBC cost |
| F10 | Low | D6 | Slot 0, slots 6-7 and the ignored `pagetable` argument were unstated | Fixed: D6 states all three |
| F11 | Low | D9 | The row keeps its cwd across slot reuse, and D12's fork note `idup`s over it, which leaks a reference | Fixed in D9 as a rule on the fd rung; for Step 221's review |

Checked and sound, no change: `UPC_ST_SIZE` and offsets against `upc_slots.h`; `pc_map` holds every RV word, so `epc - 4` resolves (Step 208's check, read from the translator, not booted); r1 keeps a7 through `sw gp`/`lw gp`; `SWITCH_YIELD` after SRET meets the I7 guard, since SRET sets priv first; no fixed user word sits in the window (spill is at 0x64000); `program_break` under `ascend-linux` is read only by fork and the context switch. All of this is from reading the tree; nothing was built or booted in Phase 10.

No blocker against D1, so Step 120's [DECIDE] does not arise.

## Phase 10, independent review of ADR-135 (Step 119 closed)

Step 119 is now satisfied. The workflow ran the independent read-only review at `4cc7577e`, nothing built or booted, and handed the findings to Steps 120-121. Its ids carry `R-` below because its F numbers overlap the self-review's F1-F11 above. Each finding was re-read in the tree before its disposition, and this phase built and booted nothing.

The self-review's answers change on three of the four questions; only the retry answer stands:

- Can a child forge the reply word? Yes with D6 off, by three routes, not the one recorded above (R-F2). With D6 on, the log's flat "no" above is withdrawn: no store reaches `.bss`, but `MRET` and `SRET` are unchecked, so a child can leave U-mode (R-F1). No route is proven to place a chosen reply and keep control, so that is a boundary that does not hold, not a demonstrated forgery.
- Does a retry double-apply anything? No. Unchanged. D7's four rules are convention, so each later handler has to be read on its own (R-F8 is one that could lose a reply).
- Can a heap byte leak between processes? Yes, in more cases than recorded above: dead to next with a hostile live child (R-F3), parent to exec'd program (R-F4), and by the unbounded path reads, D6 on or off (R-F5).
- What breaks if a kill lands during a fork copy? Nothing today: it cannot land. I9 holds, and the preemption failure is wider than recorded above (R-F6).

| Id | Severity | Against | Finding | Disposition | Re-read in |
|---|---|---|---|---|---|
| R-F1 | High, pre-existing | Security notes | `MRET` and `SRET` have no privilege check, so ADR-109 D6 does not confine a child | Accepted; claim restated. Fix (reject at priv 3) open, For Stevie; it must pair with any D6 flip | `main.rs` MRET/SRET arms, `rv2mbc_branch_base`, RET tag path; `start_mbc.c` |
| R-F2 | Medium | D1, Security notes | The wipe has three bypasses, and the note understated what a forger gets; "exactly one flag" is not enforced | Fixed in text: D1 and Security notes; "at most one flag", stated as unenforced. No code: the `sret` route closes with R-F1 | Step 128 block; `trampoline_mbc.S`, `utrap.c` |
| R-F3 | Low | Security notes | "Fork overwrites the whole low band" is false with D6 off | Fixed in text. Tagged-cursor fix open, For Stevie | fork arm; `total_copy` = 0x14000 words |
| R-F4 | Low, pre-existing | Security notes | Exec does not scrub; the ADR's list omitted it | Fixed in text: fourth bullet | exec arm copy loop |
| R-F5 | Low to Medium, pre-existing | Security notes | `open` and `exec` read user paths with no bound, D6 on or off | Fixed in text. `user_phys` routing open, For Stevie; verifier cost unmeasured | `path_phys` in both arms, `user_phys` |
| R-F6 | Low, forward | D8 I9 | I9's failure description is incomplete and nothing enforces it | Fixed in text, Step 156's stale note included. Tripwire open, For Stevie | scan comment, timer `cfg`; line 739 is the one setter |
| R-F7 | Low | D3, D6 | `vmfault` is reached; slot 0 is refused for its whole first page | Fixed in text | `uvm.c` copyin, copyout, copyinstr, vmfault |
| R-F8 | Low to Medium, plan conflict | D1 | D14's bare tag in `prepare_return` can wipe a reply, and Step 124 clears `upc_active` first | Fixed as a D1 rule: the mark goes in `upc_slot_leave()`, only if `reply` is 0. Plan text unchanged, For Stevie | Steps 124, 220 |
| R-F9 | Low | Security notes | The hostile probes never reach the real table | Recorded for Phase 21. Correction to the review: the plan has such a step, 258, which says stores land in the child's own slice, so it has this gap | `upc_st` at 0x114C00 in `kernel.elf`; `hostile_slot.c`, `hostile_tf.c`, `hostile_pgd.c` |
| R-S1 | Suspected | Security notes | `main`'s prologue after `mret` gives a chosen-address write | Open, not booted. Rides with R-F1 | not provable by reading |
| R-S2 | Suspected | Security notes | R-F5 as a byte oracle | Open, leak rate unmeasured. Rides with R-F5 | unbounded read confirmed, rate not |
| R-S3 | Suspected | D6 | First-page refusal only matters for a kernel-routed pointer from slot 0 | Checked: real, unreached today. `init.elf` `.rodata` ends 0xF59, but init passes none, and `mkprobe`, `rmprobe`, `lnprobe`, `hostile_ptr` make those calls from a forked child. Stated in D6 | section tables; probe sources |
| R-S4 | Suspected | Security notes | `LR_W`, `SC_W`, `XCHG`, `CAS` skip `translate_address` | Translator emits none, confirmed by grep. Stated in Security notes. Opcode scan open, For Stevie | `main.rs` arms; `translator.rs`; `rv2mbc.rs` |
| R-NC | Not checked by the review | D5, Security notes | Whether exec zeroes a new image's `.bss` | Checked by reading: it does not (sidecar is `ALLOC PROGBITS` only, exec arm has no zeroing pass, `crt0_mbc.S` calls `main`). D5 narrowed to a forked child. For Stevie | `rv32i_to_mbc.rs`, exec arm, `crt0_mbc.S`, `umalloc.c` |

Step 120 [DECIDE]: no finding is a blocker against D1. R-F1 and R-F2 name D1's channel, but neither stops it: forging needs a hole that is already total with ADR-109 D6 off, which stays off, and every value BPF takes is bounded. Nothing else blocks a rung: every fix left open is pre-existing or forward, and the design's rungs do not depend on it. So the rungs proceed as designed.

Step 141 says to extend `sbrkprobe` only if the review found more. R-F3 and R-F4 are more, but neither gets a fix in this plan and D5 is not a confidentiality control, so a probe would only document an open hole. No extension.

## Phase 11, sbrk rung

Step 122: `$offv` 870,702, no before gate needed (no code diff against `last-green`).

Step 130 [DECIDE], where the reply is read. In the SRET arm: 880,266, +9,564, over the +3,000 bar. After the loop, beside `DEFERRED_SWITCH`, with a `reply_req` local and a `break` in the arm: 873,001, +2,299. The post-loop form stands, and `$rv` is 873,001. It costs one short tick per return to U-mode: the `break` skips the arm's `i += 1`, so an SRET to U-mode is not counted in `insn_count`.

Step 134, off-equal. The arm as Step 132 words it (`cfg!(...) && syscall_nr == 12`) failed this check: 869,372, -1,330. The `||` form on the arm for 14 measured the same, and the alias row alone 870,702, so the live compare is what moves the count. The landed arm is `cfg!(...) && UPC_RUNG_SBRK && syscall_nr == 12`: the const folds it away when off, 12 reaches the chain's tail and answers the same -ENOSYS, and the off object is 870,702 again. `parity_surface` reads this form unchanged. Later rungs will meet the same compare.
| reply channel | 870702 | 873001 | +2299 | 48035 | sbrk-after |
| sbrk | 873001 | 891427 | +18426 | 29609 | sbrk-after |

Steps 136-139: both consts on, 891,427, +18,426 over the reply channel alone; `BUDGET-OK`, headroom 8,573. `sbrkprobe`: 13 yes, 0 no, `SB: done`, on the first boot. The arm is not the cost, nor is the alias row's form: with the arm folded out, and again with one `12 | 14` row, the count stays 891,427. Why one more alias value costs five times what uptime's did is unexplained.

Step 141: mutant with the `memset` removed printed `SB: zeroed=no` once and every other line unchanged; restored, no mutation in the diff. No probe extension, as recorded under Phase 10.

Steps 142-143: `gate sbrk-after`: same=12 diff=0 rc=0 doom=1 ascend=891427, `sbrkprobe` PASS in the harness.

Step 144, cost of one call, measured as getpid's +1,348 was (`references/s5-upc-wire-log.md` Step 101: insn_count at halt, with and without the call). Three throwaway programs that fork, make the call in the child, wait and exit; insn_count repeated exactly across boots. None 5,083,200; `sbrk(0)` 5,085,491; `sbrk(4096)` 5,106,000. So `sbrk(4096)` = +22,800 MBC instructions: +2,291 for the round trip, +20,509 for the zeroing, about 5 per byte. The window's worst case, one 192 KiB grow, is near 1M instructions in one trap. insn_count misses the SRET to U-mode, since the arm's `break` skips the increment.

## Phase 12, kill rung

| shared exit block, kill const off | 891427 | 892692 | +1265 | 28344 | kill-after |
| kill | 892692 | 889682 | -3010 | 31354 | kill-after |

Step 146: `$offv` 891,427, no before gate needed (no code diff against `last-green`).

Step 148: the empty arm for 6 carries the const in its test, as Step 134 taught for 12, so it folds away when off and 6 answers -ENOSYS from the chain's tail.

Step 149 [DECIDE]: the loop is shared, and the goldens' order did not move (gate below). The reparent scan and halted write are `slot_exited()`, called once after the reply block, before the deferred switch. The exit arm sets `exit_slot`. With `DEFERRED_SWITCH` off the arm calls `slot_exited()` itself, so that rollback still marks the zombie before its inline switch. It did not pay as Step 149 hoped: const off, the object grew from 891,427 to 892,692, +1,265. With the const on it fell to 889,682, -3,010 against the off object and -1,745 against `$offv`. So turning the rung on makes the object smaller, and these counts are not additive per edit.

Step 153: `a` differs from `$offv`, so the refactor has its own ledger row, as this step says. Step 159's single `ledger 'kill' "$offv" "$nv"` is therefore split in two: the rows above sum to the same -1,745. Appendix A7's "rung's OFF number" for kill is 892,692, not 891,427: the shared block stays when the const goes off. The 892,692 object was measured, not gated on its own.

Steps 154-155: `killprobe`, first boot: 12 yes, 0 no, 0 `self-survived`, `KL: done`. Step 156 did not run.

Steps 157-158: `gate kill-after`: same=12 diff=0 rc=0 doom=1 ascend=889682, killprobe, gate_nway and forkloop PASS. `head=8ef6e117` on that line is the const-off commit: the gate ran on the tree with the const flipped, before Step 159 committed it. `check-ebpf-insns.sh`: 35 programs within 10 % of the baseline, 0 allowlisted over 900000. `cargo test --release` in bootctl: 0 failed on every line.

Not covered by a probe: `killprobe` tests a dead target only after the reap, so a kill on an unreaped zombie (-1, D8) takes the same halted-bit branch but is not booted on its own.

## Phase 13, user pointers and the row's cwd

Step 161: last-green was HEAD, no before gate needed; `offv` 889,682.

Steps 162-165: the parity test went red first (`UPC_USER_VA_CEILING not in upc_slots.h`), then 3 passed. `USER_VA_CEILING` is private in `main.rs`, so the test has its own line match, and it also asserts the ceiling is at most `SLICE_STRIDE`. Object unchanged, 889,682, so no ledger row. The kernel's `.mbc` fell 12 bytes (48,700 to 48,688) with code added; `.text` stays 31,488 in both builds, and `nm` shows `upc_user_pa` in the ELF. The fall was checked by rebuilding HEAD's three files and restoring them, not explained further.

Steps 166-167: `gate uptr`: same=12 diff=0 rc=0 doom=1 ascend=889682, ALL GREEN, no panic in the harness output. `head=fff34c62` is the commit before this phase's: the gate ran on the uncommitted tree. Every forked child's first trap now runs `namei("/")`, and fsinit has run by then. `killprobe` and `sbrkprobe` were booted once more by hand on the final kernel: all checks reported.

Not covered: no boot takes the slot-row branch of `walkaddr`, `copyout` or `vmfault` yet, because no pointer-taking number routes to the kernel. Phase 14's `mkprobe` is the first run of that code.

## Phase 14, mkdir rung

| Rung | OFF | ON | delta | net vs 921,036 | gate |
|---|---|---|---|---|---|
| mkdir | 889682 | 891248 | +1566 | 29788 | mkdir-after |

Steps 169-172: `offv` 889,682. The empty arm for 20 carries the const in its test, as Phases 11 and 12 taught. Step 172 commits only on `off-equal`, and fa7b03a3 is that commit; the off count itself was not written to a file.

Steps 173-174: `flip`, 891,248, delta +1,566, `BUDGET-OK`, headroom 8,752.

Steps 175-176: `mkprobe`: 7 yes, 0 no, `MK: done`, first boot. Step 177 did not run. `mkdir` reaches the slot-row branch of `walkaddr` through `argstr` and `copyinstr`, so that branch has now run.

Steps 178-179: `hostile_ptr`: unterminated, ceiling, kernel and wrap paths all refused, `alive=yes`, `HU: done`, no panic.

Steps 180-181: `gate mkdir-after`: same=12 diff=0 rc=0 doom=1 ascend=891248, ALL GREEN, `mkprobe` and `hostile_ptr` PASS (2). `head=fa7b03a3` is the const-off commit: the gate ran on the tree with the const flipped. The gate took about an hour on a box that was thrashing, so its timing means nothing.

Step 182: `check-ebpf-insns.sh`: 35 programs within 10 % of the baseline, 0 allowlisted over 900000.

Resume after west's unclean reboot, 2026-11-05. The first agent stopped with Step 182's edits uncommitted; its cmt never ran. The tree was confirmed as the gated one, so the gate was not re-run. `main.rs` was last written at 19:07:27, before the gate began; the only other tracked changes since `fa7b03a3` are the ADR row, the baseline row, the progress file and this log, none of them build inputs. No kernel or user source is newer than `xv6-mbc.mbc`. The working object's sha256 equals the one `check-ebpf-insns.sh` built on its own from the same source. After the reboot `vcount` read 891,248 twice, and `mkprobe` and `hostile_ptr` booted once more: `mkprobe` lines identical to the gated run, `hostile_ptr` the same five yes and `HU: done`.

Not covered: `mkdir` takes no `copyout`, so that slot-row branch and `vmfault`'s are still unbooted. `create` runs against a fresh `fs.img` with an empty log, so log pressure is untested.

## Phase 15, unlink rung

| Rung | OFF | ON | delta | net vs 921,036 | gate |
|---|---|---|---|---|---|
| unlink | 891248 | 891248 | +0 | 29788 | unlink-after |

Steps 184-187: before-gate diff 0 (`last-green` was HEAD), `offv` 891,248. `parity_surface` green, off object 891,248, `off-equal`. The arm carries the const in its test, per Phase 11's note.

Step 188: `flip`, 891,248, delta 0, `BUDGET-OK`, headroom 8,752. A zero was checked before it was believed, since it could mean a stale object. The on object served `unlink` (Step 189 needs a real 0 and -1, not -ENOSYS), and vcount printed the same on a second load. `.text` is 81,816 bytes in both builds: LLVM lowers `kernel_routed_alias` to one bit test, and the on object's mask is 0x145840 where the off object's is 0x105840, bit 18 added. Ignoring jump offsets, the two `.text` sections hold the same 10,227 instructions apart from that one immediate: LLVM laid the blocks out in a different order, and the arm for 18 is dead code it removed once the alias sends 18 away. The object file grew 976 bytes, in debug sections.

Steps 189-190: `rmprobe`: 8 yes, 0 no, `RM: done`, first boot. `fs_walk`'s dirent scan already skips inum 0 (`if inum != 0 && inum < ninodes`), so `inner-gone=yes` needed no read-path fix.

Steps 191-192: `gate unlink-after`: same=12 diff=0 rc=0 doom=1 ascend=891248, ALL GREEN, `rmprobe` PASS (1). `head=05c2c1de` is the const-off commit: the gate ran on the flipped tree, as Step 193 commits after it. Object sha256 unchanged across the gate (e2789733), so the harness's Doom build left the ascend object in place.

Step 193: `check-ebpf-insns.sh`: 35 programs within 10 % of the baseline, 0 allowlisted over 900000. The baseline row's count is unchanged; only its date moved.

Not covered: `rmprobe` unlinks directories only, so `unlink` on a regular file was not booted. The two directories' own data blocks do go through `iput` and `itrunc`, with no panic.

## Phase 16, link rung

| Rung | OFF | ON | delta | net vs 921,036 | gate |
|---|---|---|---|---|---|
| link | 891248 | 890064 | -1184 | 30972 | link-after |

Steps 195-198: before-gate diff 0 (`last-green` was HEAD), `offv` 891,248. `parity_surface` green, off object 891,248, `off-equal`. The arm carries the const in its test, per Phase 11's note.

Step 199: `flip`, 890,064, delta -1,184, `BUDGET-OK`, headroom 9,936. The fall was checked before it was believed. The off object was rebuilt from the same source for comparison and the on object then restored (sha256 478dc65f before and after). `.text` is 81,816 bytes off and 81,808 on: one instruction fewer. The mask goes from 0x145840 to 0x1c5840, bit 19 added, as Phase 15 predicted. The rest of the diff is LLVM's: blocks in a different order and one stack spill moved. So the fall comes from layout, not from anything link saves, and the verifier walks that layout. Read it as noise of the kind Phase 12 recorded, not as a rule.

Steps 200-201: `lnprobe`: 9 yes, 0 no, `LN: done`, first boot. `same-bytes` reads the first 4 bytes of README back through the new name in BPF `read`, so the second dirent reaches the same inode.

Steps 202-203: `gate link-after`: same=12 diff=0 rc=0 doom=1 ascend=890064, ALL GREEN, `lnprobe` PASS (1). `head=974106b0` is the const-off commit: the gate ran on the flipped tree, as Step 204 commits after it. Object sha256 unchanged across the gate (478dc65f), and `pgrep -c doom-runner` read 0 after it.

Step 204: `check-ebpf-insns.sh`: 35 programs within 10 % of the baseline, 0 allowlisted over 900000.

Not covered: `original-kept` opens README after the unlink of `lnk` and reads no bytes, so it does not by itself show the data survived. `nlink` is not read back: BPF `fstat` was not asked for it. `link` runs against a fresh `fs.img` with an empty log, so log pressure is untested, as for `mkdir`.

## Phase 17, sleep rung

| Rung | OFF | ON | delta | net vs 921,036 | gate |
|---|---|---|---|---|---|
| pause(n), retry hand-off | 890064 | 891808 | +1744 | 29228 | sleep-after |

Steps 206-210: before-gate diff 0 (`last-green` was HEAD), `offv` 890,064. The off object after the kernel and BPF edits also measured 890,064, so the const covers the alias and the reply branch. `parity_surface` green.

Step 208 as landed differs from the plan's text in two small ways. The reply block has sat after the loop since Phase 11, so there is nothing to `break` from: it only sets `switch_req`. The test carries `UPC_RUNG_SLEEP`, as kill's does, or the off object would not equal the previous count (Appendix A1). `epc - 4` resolved: the boots show `rom_fault=0`, no `0x48`, and the alias went in between 12 and 14, as no row for 23 exists since Phase 6. 23 stays on the shared arm; with 13 aliased away that arm serves yield only, and `gate_nway` (which calls `yield`) still shows 3 pids.

Steps 211-212: `flip`, 891,808, delta +1,744, `BUDGET-OK`, headroom 8,192.

Steps 213-214: first boot. `sleepprobe`: four `=yes`, `SL: parent-ran` before `SL: child-woke`, `SL: done`; `uptimeprobe` `moved=yes` 2; `gate_nway` 3. No `TRIG` change. ADR-135 D7's check is met: at 3,000,000 triggers all seven lines survived the `S` and the 100-PC tracer each retry writes. Stats from that boot: `syscalls=5546 ctx_sw=32 forks=1 waitpid=49 rom_fault=0`, at about 16 instructions per tick, so a retry costs about 84 ticks and `pause(2000)` polled about 24 times.

Steps 215-216: `gate sleep-after`: same=12 diff=0 rc=0 doom=1 ascend=891808, ALL GREEN, `sleepprobe` PASS (1), `pgrep -c doom-runner` 0. `head=5ae498fe` is the const-off commit: the gate ran on the flipped tree, as Step 217 commits after it.

Not covered: `sleepprobe` checks the 300-tick wait against `uptime`; the child's 2,000-tick wait is shown only by order against the parent, not by a tick count. Deadline wrap (`t + n` past 2^32) and a `kill` landing on a slot in `pause` are not booted: a killed slot's stale deadline is cleared at row refresh, read from the code.

## Phase 18, fd table

Step 219: before-gate diff 0 (`last-green` was HEAD, 118656a4), so no gate ran. `vcount` under the shared lock: `offv` 891,808, the same as Phase 17's on figure.

Step 220: ADR-135 gains D10-D14, uncommitted until Step 222. Nothing built or booted beyond Step 219's `vcount`. Read from the tree first: all 12 goldens hold one `S` and one tracer run, from init's first return; `kexit` panics on slot 0 as well as on slot rows; `slot_exited` has two callers, exit and the reply block's kill; `ush` and `init` open only `console`, with O_RDWR.

D14 as written differs from the plan's text twice. The mark goes in `upc_slot_leave()`, as ADR-135 D1 already ruled (R-F8). `S` and the tracer move behind the reply read, after the loop, where the plan moves the read ahead of them into the loop: that read measured +9,564 and the rung starts with 8,192 of headroom. D13 adds two things the plan leaves open: the test is `upc_slot_active()`, so slot 0 never reaches `kexit`, and the status comes back in a0, so the reply block sets `exit_code` as the arm does and reads no extra word.

Step 221: the workflow ran the independent read-only review at `118656a4` plus the uncommitted ADR. No blocker on its three questions: 24 cannot forge a child's row through the syscall, a parent cannot be killed between finalize and the note, and the re-run leaves a2's cursor alone. Nothing built or booted.

Step 222 dispositions, each re-read in the tree first. Ids are `T-` in ADR-135 Review, because the numbers overlap the earlier two sets.

| Id | Severity | Disposition |
|---|---|---|
| T-F1 | Medium | Fixed in D12: finalize records that a note is due and chooses at the a0 write, so `num_processes`, `PARENT_OF` and `STAT_FORKS` still run |
| T-F2 | Medium, forward | Fixed in D10: `fdalloc` keeps the index, `fduser()` translates in `sys_open`, `sys_dup` and `sys_pipe` |
| T-F3 | Low to Medium, pre-existing | Stated in D10 and D11; no code, For Stevie |
| T-F4 | Low to Medium | `fdprobe` gains the tokens and the `fresh_mask` check a mutant. The cwd drop boots but cannot be observed until chdir |
| T-F5 | Low, D6 off | Fixed: the note closes what it finds; D12 and D13 restated |
| T-F6 | Low | ADR-109 amended in I3, D5 and Security notes; `upc-rung0-assert.awk` updated and checked on four synthetic inputs (old pass, note pass, old fail, note fail). Not run against a probe kernel: `upc-regression.sh` does not run it |
| T-F7 | Low | ADR rules over the plan text; arm for 24 lands with Step 225; `parity_slot_tables.rs` joins Step 227's commit; For Stevie |
| T-F8 | Low, pre-existing | Stated in D10 |
| T-S1, T-S2, T-S3 | Suspected | Answered by Steps 228-232 below |
| fd table | 891808 | 889100 | -2708 | 31936 | fd-after |

Steps 223-227: `fdprobe`, the kernel and BPF landed const off. The off object is 891,808, equal to Phase 17's, and the sweep is 8/8. `parity_surface` and `parity_slot_tables` pass with 24 and `UPC_REPLY_EXIT` in. Step 227's commit gained `parity_slot_tables.rs` (T-F7). `utrap.c` did not change: D14's mark is in `upc_slot_leave()`, so that name on the step's list was a no-op. `fdprobe` has 49 checks, against the 13 the plan's Step 223 lists: the rest are T-F4's and the bad-pointer refusals.

Steps 228-229: first on figure 888,434, BUDGET-OK. T-S3 answered: D11-D14 fit with room, and the object fell.

Step 230, first boot: 48 of 49. `FD: read-ptr-refused=no` was the probe's bug, not the kernel's: `fd` sat at EOF, and `readi` clamps n to 0 there, so the bad pointer was never touched (`ufs.c`). The check moved to a fresh offset, which the kernel refuses; the bar did not change. That boot also carries the first copyout from a slot row (`read`, `fstat`), which Phase 14's note said was unbooted, and the refused pointers run the slot-row branch of `vmfault`.

Step 231 failed: 0 of 8 SAME. Step 232: the TTY bytes equal the golden (1,393 bytes, no diff), so D14's guard order is right. Only the counters moved: `ls` `tty_r=9 waitpid=7` against 3 and 1, with `forks=2`.

The plan's guess, stats drift from extra trap ticks, is wrong. `waitpid` counts only a successful reap, and 7 reaps cannot come from 2 forks. The excess was equal in both counters on two boots (`ls` +6 and +6, `fdprobe` +200 and +200), which pointed at one shared source: `increment_stat(20)` and `increment_stat(21)` in the `CALLR` arm, from 7521af79 (2026-03-31), reuse the keys of `STAT_TTY_READS` and `STAT_WAITPIDS`. `syscall()` calls its handler through a pointer, so each kernel trap counts. The corpus had taken no kernel trap, so no golden ever showed it.

Debug attempt 1, predicted first: guard the two bumps under `UPC_RUNG_FD`, and `ls` reads exactly 3 and 1 with no golden touched. Result: `forks=2 tty_r=3 waitpid=1`, 8/8 SAME, and `fdprobe` shows `waitpid=10` for 10 forks and `tty_r=0`. `~/tmp/golden` is unchanged (directory mtime 2026-10-28). The guard folds away with the const off (891,808 re-measured after it) and in Doom (758,199 in the gate). The same arm's log at physical 0xE1000 was checked too: it sits between `.rodata` and `.bss` and touches no kernel data.

The guard moved the on figure to 889,100, +666 for removing code: another case of verifier counts not being additive.

Mutants (T-F4), each on the probe kernel, each reverted (`git checkout` of `upc_slots.c`, tree checked clean, rebuilt, 49 again):

| Mutant | Result |
|---|---|
| `fresh_mask` test deleted from `upc_forknote` | 4 `=no`: note-own, note-live, sibling-intact, sibling-file |
| `upc_exit` does not close | 2 `=no`: exit-closed, kill-closed. The second is the next note closing the leaked file (T-F5), which frees a lower inum |
| `upc_kill` does not close | 1 `=no`: kill-closed |

T-F4's cwd drop boots (`FD: reuse-cwd`) but cannot be observed: every row's cwd is root until chdir, so a leaked reference shows nothing.

Steps 233-234: gate fd-after, same=12 diff=0 rc=0 doom=1 ascend=889100. `forkloop`, `gate_nway`, `gate2`, the four `ut_*` and `fdprobe: 49 checks` PASS, ALL GREEN. T-S1 and T-S2 answered: exec'd images take injected fork and exit traps in all 12, and none was cut off at its trigger count.

Not booted: a file past one block (`fdprobe` writes 300 bytes, so `filewrite`'s chunking, `bmap`'s indirect block and BPF reading them back are unshown); a forged 24 with ADR-109 D6 off (Phase 21's); `upc-rung0-assert.awk` against a probe kernel.

## Phase 19, chdir

Step 237: last-green is HEAD, so no before gate; `offv` 889100.

Step 238: ADR-135 D15. `cwd_mask` takes the pad at +0x81C; `UPC_ST_SIZE` stays. The kernel writes it at `sys_chdir` and in the fork note.

Steps 239-240: `chdirprobe`, 24 checks where the step lists 8. Added: `root-hidden` (a relative `README` from `cdd` must miss, which BPF's root walk would find), `absolute`, `cwd-kept` after a refused chdir, a file written and read back relative, then through BPF from the root, `parent-stayed`, `reuse-root`, and T-F4's cwd drop.

Steps 241-242: off 889100, `off-equal`; on 890496, delta +1,396, `BUDGET-OK`, headroom 9,504. Off, the probe answered `chdir=no`. On, first boot: 24 `=yes`, 0 `=no`, `CD: done`. Unlike Step 129's reply read, the in-loop table read did not cost +9,564.

| Mutant | Result |
|---|---|
| fork note does not copy the bit | 2 `=no`: child-inherits, reuse-root |
| `upc_cwd_mark` never clears | 3 `=no`: bpf-again, bpf-read, reuse-root |
| fork note does not drop the old `cwd` | 1 `=no`: cwd-dropped |
| no alias row for open off the root | 7 `=no`: open-rel, root-hidden, absolute, cwd-kept, read-rel, child-inherits, parent-stayed |

All four reverted; the restored kernel's `.mbc` hash and the restored object's count matched before the gate. T-F4's open item is closed: `cwd-held` and `cwd-dropped` show the note drops the row's reference.

Steps 243-244: gate chdir-after, same=12 diff=0 rc=0 doom=1 ascend=890496. `chdirprobe: 24 checks` PASS, ALL GREEN. `check-ebpf-insns.sh` PASS.

| Rung | Off | On | Delta | Net vs 921036 | Gate |
|---|---|---|---|---|---|
| chdir | 889100 | 890496 | +1396 | 30540 | chdir-after |

## Phase 20, pipe

Step 246: last-green is HEAD, so no before gate; `offv` 890496.

Step 247: `pipeprobe`, 20 checks. Beyond the step's list: EOF stays 0 on a second read, a writer already blocked on a full pipe gets -1 when the reader closes, a kill landing on a slot parked on a retry closes its read end and it never returns (Phase 17's unbooted case), and a pointer that straddles the ceiling, so the first word lands and the second is refused. Forty refusals against 16 `ofile` rows, then the same two fd numbers, show no leak. The child never closes its write end by hand, and its marker directory appears after a `pause(200)`, so EOF can only have come from exit's close.

Steps 248-249: `UPC_RETRY_RC` is what a retrying pipe end returns, but `syscall()` does not test that value: `upc_retry_pending()` reads the YIELD reply, so no handler's real return can be mistaken for a retry. ADR-135 D7's rule that a handler returns the a0 it was given becomes "keeps a0". The arm for 4 carries the const in its test, as for 12.

Steps 250-252: off 890496, `off-equal`; on 889941, delta -555, `BUDGET-OK`, headroom 10,059. First boot: 20 `=yes`, 0 `=no`, `PI: done`. At 8,000,000 triggers a boot takes about 19 s. Step 253 did not run. As with link, a fall is compiler layout, not a saving.

| Mutant | Result |
|---|---|
| `syscall()` stores the return over a0 | 9 `=no`, first all-bytes; the re-run reads a wrong fd |
| `pipewrite` retries after partial progress | stops after `PI: fds`, no `PI: done` |
| `upc_exit` does not close `ofile` | no EOF, stops after `short-writes`, no `PI: done` |
| `upc_kill` does not close `ofile` | 1 `=no`: kill-closed |
| `piperead` returns 0 on an empty pipe | 2 `=no`, first all-bytes, no `PI: done` |
| `sys_pipe` keeps the fds when copyout fails | 1 `=no`: no-leak |
| fork note shares the file without `filedup` | 2 `=no`, first all-bytes, no `PI: done` |

All seven reverted from saved copies, not git, since the edits were not yet committed; the restored kernel's `.mbc` hash matched before the gate. bootctl's tests passed with the new surface.

Steps 254-255: gate pipe-after, same=12 diff=0 rc=0 doom=1 ascend=889941. `pipeprobe: 20 checks` PASS, ALL GREEN.

| Rung | Off | On | Delta | Net vs 921036 | Gate |
|---|---|---|---|---|---|
| pipe | 890496 | 889941 | -555 | 31095 | pipe-after |

## Attack surface

Step 257. One row per landed rung. Kernel C now parses user pointers where the verifier used to bound every access, so each row names the check that bounds a read and the line that proves it. "Open" means a line that shows the hole, not a check that closes it. Every user pointer ends at `upc_user_pa`: below `UPC_USER_VA_CEILING`, plus the slot's slice base. Words a child stores ahead of a trap are covered by the `hostile_reply` rows; "BPF" means BPF reads the word.

| Rung | New read | Who writes it | What bounds it | Proof |
|---|---|---|---|---|
| uptime | `ticks` in `upc_slot_enter` | BPF, each injection | Only moves forward; a forged word is overwritten at the next injection | `UP: child-mono=yes` |
| sbrk | `brk_in` in `upc_sbrk` | BPF, each injection | Outside [0x10000, 0x40000] normalizes to the base; `n` checked against both ends and INT_MIN; memset stays in the window | `SB: over-cap-refused`, `SB: under-base-refused`, `SB: intmin-refused` |
| sbrk | `reply` and `reply_arg`, SET_BRK (BPF) | Kernel | Tag check, wipe at trap entry; the value is stored unchecked and bounded at its next use by the `brk_in` row | `HR: heap-intact=yes`; open: `HR: sret-forged=yes` |
| kill | `pid` argument in `upc_kill` | User | 2 to 8 | `KL: zero-refused`, `KL: neg-refused`, `KL: huge-refused`, `KL: init-refused` |
| kill | `reply_arg`, KILL (BPF) | Kernel | Masked to 7, `t != 0`, below `num_processes`, not halted | `KL: dead-refused`; forged at slot 0: `HR: kill-init-refused=yes` |
| mkdir | path, a0 | User | `copyinstr` through `upc_user_pa`; no NUL before the ceiling refuses | `HU: unterminated-refused`, `HU: ceiling-refused`, `HU: kernel-refused`, `HU: wrap-refused`, `MK: noparent-refused` |
| unlink | path, a0 | User | Same `argstr` as mkdir; `hostile_ptr` boots mkdir only, so the pointer bound here is read from the code | `RM: missing-refused`, `RM: dot-refused`, `RM: dotdot-refused` |
| link | two paths, a0 and a1 | User | As unlink | `LN: missing-refused`, `LN: dup-refused`, `LN: dir-refused` |
| sleep | `n` in `upc_pause`; `reply`, YIELD (BPF) | User; kernel | `n <= 0` returns at once; YIELD takes no argument and only reschedules, so forging it buys a slot its own turn. Not booted forged | `SL: zero`, `SL: neg` |
| fd table | read and write buffers, stat pointer, open path | User | `copyin`, `copyout`, `copyinstr` through `upc_user_pa` | `FD: read-ptr-refused`, `FD: write-ptr-refused`, `FD: fstat-ptr-refused` |
| fd table | fd number at or above 16 | User | `argfd` takes off 16, then bounds the index against `ofile` | `FD: read-bad`, `FD: write-neg`, `FD: close-unopened`, `FD: fstat-bad`, `FD: dup-bad` |
| fd table | `fresh_mask` and slot in `upc_forknote` | BPF; a0 | Slot 1 to 7 and the fresh bit, set only from finalize to the note | `FD: note-zero`, `FD: note-neg`, `FD: note-huge`, `FD: note-own`, `FD: note-live`, `HR: note-refused=yes`; open: `HR: note-forged=yes` |
| fd table | `reply`, EXIT (BPF) | Kernel | Ends the current slot only. Not booted forged | `FD: exit-closed`, for the honest path |
| chdir | path | User | `copyinstr` through `upc_user_pa` | `CD: nope-refused`, `CD: file-refused`, `CD: rel-file-refused` |
| chdir | `cwd_mask` (BPF) | Kernel | One bit, and it only picks which side serves open. Not booted forged | `CD: bpf-again` |
| pipe | fd array pointer, a0 | User | `copyout` of two words through `upc_user_pa`; a refusal closes both ends | `PI: bad-refused`, `PI: no-leak` |
| pipe | read and write buffers | User | `copyin`, `copyout` through `upc_user_pa`, in `piperead` and `pipewrite`. A bad write buffer returns 0, not -1, as stock xv6 does: read from the code | `PI: bad-refused` covers the fd array only |

One carried limit, in no single row: slot 0's base is 0, so init's only bound is the 8 MiB ceiling, and a kernel-routed `copyout` from init can write kernel memory below it (Phase 13's note). Init is identity-mapped and can already store there, so that buys nothing new.

## Phase 21, security pass

Step 258: `user/hostile_reply.c`, tokens `HR:`, gated when `UPC_REPLY` is on. The step's shape alone cannot test anything (R-F9): the table links below 8 MiB, so that store lands in the child's slice. It is kept as the first check, `HR: own-slice-intact`. The probe then adds a PDE to its own pgd, a superpage at physical 0 on free PDE 2, and forges through that alias by three routes:

| Route | Expected, D6 off | Booted |
|---|---|---|
| Word stored ahead of an ecall | wiped at entry | `HR: heap-intact=yes` |
| Forged `fresh_mask`, then 24 on a free slot | lands | `HR: note-refused=yes`, then `HR: note-forged=yes` |
| U-mode `sret`, real SEPC set through the alias, forged SET_BRK | lands | `HR: sret-forged=yes`: `sbrk(0)` answers 0x20000 |
| U-mode `sret`, forged KILL of slot 0 | refused | `HR: kill-init-refused=yes`, then `HR: done` from init |

So a U-mode `sret` does place a chosen reply and keep control, which ADR-135 had as expected and now has as booted. The forged break was chosen inside the heap window; a value outside it is normalized at its next use by `upc_sbrk`, read from the code and not booted. The KILL bound held.

The first draft trapped with `sbrk(0)`, and that could not fail: `upc_sbrk` calls `upc_reply`, which overwrites the forged word with or without the wipe. The ecall route now traps with `getpid`, whose handler sends no reply. Two mutants, each restored from a saved copy, not git:

| Mutant | Result |
|---|---|
| `upc_slot_enter` does not wipe `reply` and `reply_arg` | `HR: heap-intact=no`, every other line unchanged |
| BPF's KILL block drops `t != 0` | `HR: kill-init-refused=no`, init dead, no `HR: done` |

The first mutant also showed a draft line, `word-wiped`, staying `=yes`, because BPF clears any tagged word it consumes. It could not tell a wipe from a consumed forgery, so it was removed. After both mutants the kernel `.mbc` hash matched and `vcount` read 889941.

Not booted: the harness's D6-on branch (`HR: table-reached=no`, `HR: route=closed`), since D6 stays off; a forged note against a live slot, which by `upc_forknote` closes that row's fds and hands it the forger's; forged YIELD, EXIT and `cwd_mask`.

Step 259: `UPC_D6_MISS_FAULT` false, count 1.

Steps 260-261: `cargo-fuzz` is not installed (`error: no such command: fuzz`), so `fuzz_mbc_execute` is a SKIP, as Stage 2 Step 140 was. A SKIP is not a pass.
