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
