<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-STAGE-2B progress

Plan: `docs/battle-plans/S-STAGE-2B-battle-plan.md`. Evidence: `references/s-stage-2b-log.md`.

Skip Protocol acknowledged: skip after 3x the estimate or 2 failed debug attempts, mark [STUCK], commit known-good state, continue with the next independent step.

DP-206 (approved, already run once under guard): the attended list is Appendix D of the plan, for S-E1-LIVE.

## For Stevie
- Step 6 departs from the plan text. As written, root writes the pid to a user-owned mktemp file, and fs.protected_regular=2 refuses that with EACCES in sticky /tmp (tested on west), so doom_stop would silently do nothing and doom-runner would leak. A private mktemp -d directory was tried next, but a Claude Code safety check refused the rm -rf in that command and nothing ran; that removal was not retried. Landed instead: the launcher prints "doom-runner-pid N" into the user-owned $LOG, doom_stop reads it, kills by that pid and truncates $LOG. No new temp path, so nothing new to delete. The plan text still shows the PIDF form.
- Step 6 proven on the first real gate (gate p0, 2026-11-04): Doom object loaded, 'pipeline complete', ALL GREEN, pgrep -c doom-runner 0 after the harness. doom-runner run stays up until signalled, so the by-pid kill did the stop.
- Phase 0 resumed after the first agent was cut off during Step 7: DEADLINE reset to 13 h from the resume, Step 7 re-run from the top. Its eBPF build was a no-op (35 MB RSS), so main.rs was touched and rebuilt for the real figure, 180 MB in 0.75 s, against the plan's ~12 min estimate for the whole step. Run cost sits above the Ledger in the log because ledger appends to end of file.

1. (plan-time) Rung order differs from the pack: sbrk, kill, mkdir, unlink, link, sleep, fd table, chdir, pipe. pipe and chdir cannot be honest before the kernel fd table. Veto if you want otherwise.
2. (plan-time) Heap: 192 KiB per process in VA [0x10000, 0x40000), and init (slot 0) gets none. A bigger heap needs a third fork-copy band.
3. (plan-time) `cargo-fuzz` is not installed on west, so `fuzz_mbc_execute` records SKIP. Installing it is a third-party question.
- ascend baseline row now 872276 (net vs 921036: 48760)
- Phase 2 departures, both small: Step 23 printed 2, not 0, because Phase 0's harness edit landed after the p0 gate recorded HEAD, so gate rung2-before ran as the step says (12/12, rc=0, 868290, Doom 758199). Step 34's cmt list omits $PROG, but insrow writes to it, so $PROG was added to that commit or Step 35's clean-tree check could not pass. Phases that run insrow later need the same addition.
- ascend baseline row now 870122 (net vs 921036: 50914)
- DP-413 moved the ascend baseline row by -2154 (872276 to 870122): not a rung, so DP-717 does not name it; net vs 921036 is 50914.
- Phase 6 departures. Step 61's alias row measured +3870 (876146), so Step 68 ran: yield shares the arm for 13 as `syscall_nr == 13 || syscall_nr == 23`, measured 870122 (-2154), and there is no alias row or empty arm for 23. Step 67's ±500 was read as a growth bar: the fall is outside it on the low side, and the gate is green. parity_surface.rs now accepts one `||` group per arm (red first; a mutant dropping 23 fails it), so Step 73's cmt list gained that file and $PROG.
- For later phases: Step 87 says the empty arms for 14 and 23 stay, and Step 208 says the 13 alias goes before the row for 23. Neither exists for 23 now. Step 208 still holds: alias 13 to 11, and 23 stays on the shared arm in BPF, which is what keeps yield a yield. When that rung lands, check the arm still reads right with 13 aliased away.
- ascend baseline row now 870702 (net vs 921036: 50334)
- DP-503 grew the ascend baseline row by 580: not a rung, so DP-717 does not name it; net vs 921036 is 50334.
- Phase 8 notes. Step 87: there is no empty arm for 23 since Phase 6, so only the arm for 14 got the -ENOSYS answer. parity_surface skips only an arm that never reads syscall_nr, narrower than the step's 'no syscall_nr ==', so an odd comparison such as >= still fails the parse. Step 91's two lines would read the same under the old fall-through; SB: grow=yes is the line that shows -38, and the log says so.
- Until the sbrk rung lands, sbrk returns -38 while xv6 userland tests SBRK_ERROR (-1): the probe shows SB: malloc=yes on a pointer that is not real. No corpus binary mallocs (ush is malloc-free), and the rung ends it, but a tree parked at Stop B carries this.
- docs/policy/ebpf-size-pins.txt still pins the ascend object at 918840; it now builds to 936280 and was already 935448 before Phase 8. check-ebpf-size-pin.sh is the staging-entry gate for changes that claim to be inert, not part of this plan, so the pin was left alone: re-pin deliberately before staging.

