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
