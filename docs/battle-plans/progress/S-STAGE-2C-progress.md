<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-STAGE-2C progress

Plan: `docs/battle-plans/S-STAGE-2C-battle-plan.md`. Evidence: `references/s-stage-2c-log.md`.

Skip Protocol acknowledged: skip after 3x the estimate or 2 failed debug attempts, mark [STUCK], commit known-good state, continue with the next independent step.

## For Stevie
- Answers this plan runs on (Stevie, 2026-11-12, from next.md rounds 2 and 3; recorded here, nothing to decide):
-   A-60, UPC D6: "enable D6 and reject MRET/SRET at priv 3 as the first phase of S-STAGE-2C".
-   A-61, exec .bss zeroing (R-NC) and the atomics skipping translate_address (R-S4): "fix both plus an opcode scan tripwire".
-   A-61, open(O_RDWR) on a regular file (T-F3) and the bad .data sidecar: "fix both (sidecar fatal)".
-   A-62, probes: "all of them".
-   ADR-135 F7/F8: "parked ADRs" (written as ADR-145 and ADR-146, Proposed, not built).
-   ush: "add a cd builtin".
- wiki/ADR-Index.md has no rows for ADR-139 to ADR-144 and its counts were 132 against the canonical 141 before this plan's two rows; not this plan's to fix (S-RESIDUE parked it).
- Step 29's review was done by the lane itself (a fork cannot start a review agent). An independent read of the priv-3 test and D6 is owed; dispositions are in references/s-stage-2c-log.md, Phase 2 review.
- Step 26 [DECIDE]: MRET/SRET at priv 3 are NOPs, not an illegal-instruction trap (the plan's recommendation: a trap needs a kill path from a slot row, which arrives at rung 7; Step 250 revisits).
- Step 30 [DECIDE]: D6 first, as asked (recommendation); the override (rung 3 first) applies only if the D6 count fails the budget.
- The host executor crates/monad-mbc/src/execute.rs still takes MRET/SRET at any privilege (not in a boot; no parity test on it). Bring it in line?
- ascend baseline row now 893507 (net vs 921036: 27529)
- D6 on and MRET/SRET at priv 3 are NOPs (not an illegal-instruction kill; Step 250 revisits at rung 7). Cost: 3566. hostile_reply closed on every route.
- Stop A reached 2026-11-15: gate d6-after same=12 diff=0 rc=0 doom=1 ascend=893507.
- ascend baseline row now 895768 (net vs 921036: 25268)
- [STUCK] Steps 78, 81 (atomics through translate_address): over the verifier budget. Four inlined translations +20,993, one shared arm +16,855, against 4,232 of headroom. The load-time scan lands instead and refuses any user image holding an atomic, MRET or SRET. Options for the BPF half: (1) the decided design after rung 3 frees instructions; (2) the slice's linear map (user_phys), measured +3,263; (3) refuse atomics at U-mode as MRET/SRET are, a few hundred. Which?
- The opcode scan has a probe hatch, UPC_ALLOW_USER_PRIVRET=1: hostile_reply holds an mret and two srets on purpose, so the scan as planned would have refused it. The hatch admits MRET/SRET only, prints a warning, and the harness sets it for that probe alone. Keep it, or build the probe some other way?
- Step 93 [DECIDE]: .bss gets its zeros through the .data sidecar (option A, the recommendation); no BPF change for it.
- Found by bssprobe, fixed: SRET masked the return PC to 16 bits, so a resident program at ROM 0x10000 or above (wc, bssprobe) was resumed in another image after any trap the kernel answered for it. wc never made such a call. One line in main.rs, verified count -1. Rungs 5+6 would have hit this with wc.
- ascend baseline row now 895767 (net vs 921036: 25269)
- R-NC closed: .bss arrives zeroed through the .data sidecar (option A); bad or missing userland sidecars now refuse the boot.
- Stop B reached 2026-11-15: gate bss-after same=12 diff=0 rc=0 doom=1 ascend=895767 (headroom 4233). One half is stuck inside it: atomics through translate_address (Steps 78, 81), see the [STUCK] line above. Next: Phase 6 (fork tag, reservation, I9 tripwire), which adds verified instructions against that headroom.
- Phase 6 departure: fork's first entry is keyed on a mark in the child slot's PROC_TABLE row, not on a tag in a3 as D18 was first written. a3 is the caller's register, so a forged tag would have skipped the copy with D6 on. Cost +169.
- [STUCK] the slot scan skipping rows another parent has marked, and clearing a mark when a parent dies mid-fork (Step 116, second half): the skip alone is +11,789 against 4,064 of headroom. Unreachable while ascend has no timer switch, which is now a compile-time assertion (I9). Needed only with preemption.
- ascend baseline row now 895936 (net vs 921036: 25100)
- Review finding RV-1 (High) fixed: syscalls 200/201 (block device) were callable from U-mode with a physical buffer address; a child wrote and read init's memory with D6 on. A U-mode caller now gets -ENOSYS. Cost +4,139.
- Budget reorder: the RV-1 guard did not fit (900,081). D18's fork mark (+169) is behind UPC_FORK_MARK and OFF to make room; headroom is now 94. Phase 7 and the mark need rung 3 (Phase 9) to free instructions first, as Step 30's override says.
- Review finding RV-2 fixed: the opcode scan also refuses PUSH, POP, INT and IRET in a user image. The interpreter itself still runs them at U-mode; the scan is the control, as for the atomics.
- Review finding RV-3 (Low, open): the host engine crates/monad-mbc/src/execute.rs has neither the priv-3 refusal nor D6; refusals and D6 misses count nothing. A counter needs instructions the object does not have.
- Seen, not acted on: a user program's ebreak becomes HALT, and HALT has no privilege test, so any user program can halt the machine. Refuse HALT at U-mode (cost unmeasured), or leave it?
- ascend baseline row now 899906 (net vs 921036: 21130)
- Reorder (Step 30's override): Phase 7's refusal measures +622 and does not fit (headroom 94). Order from here: Phase 9 (rung 3) first, then Phase 7, then UPC_FORK_MARK back on. Stop C is not reached; the last named Stop is B. Everything after it (Phase 6, the review fixes) is committed and gated green: gate review-after same=12 diff=0 rc=0 doom=1 ascend=899906.
- ascend baseline row now 885354 (net vs 921036: 35682)
- Step 179 [DECIDE]: rung 3 stays on. All 12 corpus boots finish inside the existing 4,000,000 triggers, so no budget changed (the recommendation's condition). Cost: about 3,000 MBC instructions per write call, about 220 per byte inside a call; ls +783,397, cat BIGFILE +3,027,808, echo +72,318 instructions per boot.
- Rung 3's alias tests 'not a file', not 'is the console' as D19 was first sketched: the gate programs run as init and print on fd 1 without opening console, so BPF's table calls that fd free and a console-kind test would have silenced every probe.
- Slot 0 and rung 3: init can no longer write a string literal. The kernel's copyin refuses init's first page (D6, D16), where user.ld puts .rodata; before the rung BPF read the bytes itself. devprobe failed on it and its two tokens moved to .data. init and uinit print through printf (a stack byte), so no corpus program is touched. Leave the limit, or let copyin (reads only) take slot 0's first page past BootParams?
- Rung 3 on at 885354 (delta -14552); console write is kernel-side; per-call cost in the log. Phase 9 ran before Phases 7 and 8 (Step 30's override), so rung 3 landed before Stop C: gate rung3-after2 same=12 diff=0 rc=0 doom=1 ascend=885354. Headroom is now 14646.
- T-F3 interim (Phase 7) is delivered by rung 3's arm, not by a test of its own: with UPC_RUNG3 on, a write to a file fd below 16 answers -1 (fdprobe 51 checks). With the rung off the old behaviour returns; the off object has no room for both (+622 measured against 94).

