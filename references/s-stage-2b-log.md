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
