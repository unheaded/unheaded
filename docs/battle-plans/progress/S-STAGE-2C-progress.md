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
- ascend baseline row now 885529 (net vs 921036: 35507)
- Stop C reached 2026-11-15, after rung 3: gate mark-after same=12 diff=0 rc=0 doom=1 ascend=885529 (headroom 14471). Order reached: Stop A, Stop B, rung 3 (Phase 9, the plan's Stop E), then Phase 7 and UPC_FORK_MARK on. Phase 8 (Stop D) and Phase 10 (rung 4) are next; neither is started.
- Atomics, fourth option checked: the shared-arm translation (+16,855) still does not fit after rung 3 (headroom 14471), short by 2384. The linear-map form (+3,263) and the U-mode refusal fit now. Still yours to pick.
- Shift 4 picked up after the stalled shift. Its uncommitted Phase 8 edits (rmprobe, lnprobe, sleepprobe, hostile_ptr, bigprobe, the asserter's a7=16 line) all match Steps 140-144 and were kept. `gate resume` on the committed tree first: same=12 diff=0 rc=0 doom=1 ascend=885529.
- Phase 8: no BPF change (885,529 before and after). rmprobe RM 11, lnprobe LN 12, sleepprobe 5 (pause(50) woke 323 ticks late on this boot: a note), bigprobe BG 3, hostile_ptr HU 9, the rung 0 asserter passes on 141 tagged traps of a probe kernel and fails a changed pid. Gate `probes-after`: same=12 diff=0 rc=0 doom=1 ascend=885529, ALL GREEN.
- Finding R-P1 (Low, open; ADR-135, Probes booted 2026-11-15): BPF's file read copies a fixed 16-byte chunk whatever count it returns, so the last short read of a file writes up to 15 bytes past what it reports, inside the caller's slice, with bytes from the block past the file's size. rmprobe measured 12 and prints it as a note (`RM: read-past-end`). Not fixed in BPF; rungs 5+6 move file reads to the kernel, where it must read 0. Fix it in BPF before then, or leave it to the rung?
- The asserter needed two things the plan did not foresee: a decoder that separates kernel printf (a word per character) from user bytes (one byte), because rung 3 interleaves them character by character on a probe kernel (`scripts/upc-tty-split.py`, new); and the two programs the plan names counted as forks (`GF: fork-ret`, and `FL: child` toward the total, since forkloop prints no pid).
- Stop D: every A-62 probe gated; 2 stuck lines. Hardening done: D6, priv-3 returns, bounded paths, slot 0, the opcode scan (atomics translation stuck), .bss, sidecars, fork mark and I9, T-F3 interim, the block-syscall guard. Order reached: Stop A, Stop B, rung 3 (the plan's Stop E), Stop C, Stop D.
- ascend baseline row now 883522 (net vs 921036: 37514)
- Rung 4 on at 883522 (delta -2007); console read is kernel-side (ADR-135 D20). Gate rung4-after: same=12 diff=0 rc=0 doom=1; crprobe 3 checks, tty_r=12; console --stdin prints `$ hi`.
- Rung 4 departures from the plan's D20 text, all in ADR-135 D20: (1) the byte travels in a new slot-table word (`kbd_in`, table size 0x824) offered at injection, and the ring advances only on the kernel's reply flag `UPC_REPLY_KBD`, because the kernel's uartgetc reads the Doom-era keyboard word, not the ring; (2) the alias tests "is the console", not "not a file"; (3) tty_r is bumped where the ring advances (per byte taken), not at routing: at routing, the read left blocked at the prompt would count one more than the goldens; (4) no echo; a read returns a line; control characters (^H ^U ^D ^P) are now the kernel's line discipline, where BPF passed them through as bytes.
- Rung 4 cost, your call: a read blocked on an empty ring now retries through the kernel every tick. A boot that ends at the prompt runs about ten times the MBC instructions (ls 6.27 M → 63.3 M) and about a fifth longer on the wall (9.5 s → 11.4 s); an idle console session does about sixteen times the BPF work per tick. The corpus still fits its triggers. A cheap wait (the kernel marks the slot-table word when it retries with nothing buffered; BPF holds the read in its own arm while the mark is there and the ring is empty) is designed in the log and not built. Build it, or accept the cost?
- Stop F reached 2026-11-15 (rung 4). Order reached: Stop A, B, rung 3 (Stop E), C, D, rung 4 (Stop F).
- Rungs 5+6 design is written (ADR-135 D21, 2026-11-15), review pending. [DECIDE] Step 209.3 taken by the plan's recommendation: the console is matched by name in the kernel, no on-disk device node; mknod and T_DEVICE inodes are a later parked ADR if you want them.
- D21 departs from the plan in one choice that is yours to overrule: the kernel gives the first image its console (fds 0-2 from upc_slots_init, as Linux does for pid 1). Reasons: init's "console" literal sits in the first page the kernel's copy-in refuses for slot 0 (the slot 0 literal limit above), and 33 of the 36 gate programs open nothing and print on fd 1. The alternative on record: the name in .data in init.c and uinit.c, and an open in every gate program. With D21 as written, rungs 5+6 do not wait on the slot 0 literal decision.
- Rungs 5+6 make every open, read, fstat and close a kernel trap. Whether the 12 corpus boots still finish inside 4,000,000 triggers is measured in Phase 12 (Steps 223-226) and may need a decision of Step 179's kind.
- Rungs 5+6 design reviewed independently (ADR-135 D21, review table): five findings, all accepted and written into the design before any code. The one that would have bitten: a console read from the first image would have slept with nothing runnable.
- Rungs 5+6 (Phase 12): built, measured, and OFF. Wired with the const off (cf11267b); turned on it verifies at 529,305 (delta -354,217 against 883,522; headroom 370,695), every probe passes (rwprobe 5, fdprobe 51, chdirprobe 24, pipeprobe 20, bigprobe 3) and all 12 corpus boots finish inside 4,000,000 triggers. It is not on because the goldens move: the Phase 1.4 PC tracer prints the first 100 PCs after init starts, kernel PCs included, and that trace is in every golden. Under the rung init's first open (its 21st instruction) traps to the kernel, and it answers -1 (the slot 0 literal limit), so the trace differs from the 21st PC in all twelve. Eleven boots are byte-identical outside the trace. A moved golden is a stop: flipped off (A7), off object 883,522 again, Stop F stands.
- Rungs 5+6, your decision (both ways measured, ADR-135 D21 'Built and measured'): (1) NO RECAPTURE: let the kernel's copyinstr read slot 0's first page (reads only; this is the slot 0 literal question above, answered yes) and have the tracer print U-mode PCs only under the rung. Booted uncommitted: 12 of 12 goldens SAME with init.c and uinit.c untouched, 532,152 verified instructions. (2) ONE NAMED RECAPTURE of the twelve goldens' trace segment, with uinit's console name moved to .data. Either turns the rung on in two lines (const UPC_RUNG56 in main.rs, #define UPC_RUNG56 in upc_slots.h) plus the chosen change.
- Found in Phase 12 and fixed: the kernel Makefile generates no dependency files, so a changed upc_slots.h rebuilt nothing; a rung-on object booted against a rung-off kernel. Makefile.mbc now names the header for the ten objects that include it. A stale kernel build is still invisible to parity_slot_tables (it reads sources, not built files).
- D21 as designed said rungs 5+6 do not wait on the slot 0 literal decision. For the goldens they do (the tracer). The design's uinit edit (console name in .data) was tried and taken back: it moves uinit_echo with the const off.
- Shift 6 ends at Stop F with rungs 5+6 wired and off: gate rung56-off same=12 diff=0 rc=0 doom=1 ascend=883522 at a0c25e62, harness ALL GREEN. Steps 229-244 (mutants, usertests bodies, the after-gate, Stop G) wait on your answer to the rungs 5+6 question above; Phase 13 (rung 7 design) does not depend on it.
- Stop I reached 2026-11-15 (cd in ush). [DECIDE] Step 281 taken by the plan's recommendation: one recapture of ls.tty and ls.full, because the only differing bytes are sh's size field (7496 -> 7652); reason and HEAD in ~/tmp/golden/README.md and the ledger's Golden policy table; rollback copies ls.tty.pre-cd and ls.full.pre-cd. cd is stock sh's form: the line must start 'cd '; a bare 'cd' is still 'exec cd failed'.
- Phase 16 (security pass) is half done: the attack-surface table is in the ledger and every row names a harness line; two rows had none and got one (the opcode scan's PUSH/POP/INT/IRET, the .data sidecar refusal). The independent review of the sprint's diff (plan Step 293) is run by the coordinator and has not reported: ADR-135 has a table marked PENDING for it. [DECIDE] Step 296 (fuzz): SKIP, cargo-fuzz is not installed (target crates/monad-mbc/fuzz, fuzz_mbc_execute, exists). A SKIP is not a pass; installing it is yours to decide.
- Security pass review (Step 293, independent, at 99ae2fa9): nine findings, dispositions in ADR-135 'Security pass review of S-STAGE-2C'. Fixed: sidecar records bounded and the kernel and bootstub scanned (b0526886), consolewrite clamped at 4096 a call, three vacuous probe lines made real (each shown to fail with its protection removed). No fix added an eBPF instruction; ascend stays 883522.
- Open after the security pass, yours: (1) SP-1 residual: a resident program exec'd by slot 0 itself lands at raw physical addresses up to 0x800000 (slot 0 is init, trusted by design); close it with the 0xC000 window for resident programs too, or a slot test in exec? (2) SP-3: a sidecar format marker (a flag in the TLV count word, set by --with-bss) so bootctl can refuse a sidecar with no .bss in it; the Makefile now regenerates stale ones, the marker is not built. (3) SP-4d: a harness boot with D6 off, so UPC_FORK_MARK is the deciding control in hostile_fork (a second eBPF object per run). (4) SP-5b: JMPR/CALLR diagnostics store user-chosen words at physical 0xE0000 and 0xE1000 from U-mode; a privilege test costs verifier instructions. (5) SP-2 residual: the interpreter's atomic, PUSH, POP, INT and IRET arms still have no privilege test of their own; the scan keeps them out of every image.
- ADR-135 (amended D16-D21, three review tables) is Proposed: acceptance is yours. No rung 7 ADR exists (Phases 13-14 not run: Stop G waits on your rungs 5+6 choice). Gate commits: 539cf5b0 9d750030 b0526886 067ab7f1 070654cc 99ae2fa9 a0c25e62 cf11267b 0c66f515 ae44692b d980e84b 11f28a83 7fca9618 25d39563 

## Close

Closed 2026-11-15 from Stop F plus Stop I, by the plan's rule for a rung that will not gate. Branch `sprint/s-stage-2c`, develop merged in (`de471513`). Close gate `close2`: `same=12 diff=0 rc=0 doom=1 ascend=883522`, harness ALL GREEN, `check-ebpf-insns.sh` PASS, `BUDGET-OK 883522 (headroom 16478, ladder net 37514)`. Every commit of the branch is signed (39 before this one, all `G`); none needs `scripts/resign-unpushed.py`. Merging into develop (Steps 315-318) is the coordinator's.

**Stops reached, in order:** A (`f27cc727`), B (`10078582`), rung 3 (the plan's Stop E, before C under Step 30's override), C (`6941f239`), D (`7fca9618`), F (`d980e84b`), I (`070654cc`). Not reached: G (rungs 5+6 on), H (rung 7).

**Consts landed (on):** `UPC_D6_MISS_FAULT`, `UPC_FORK_MARK`, `UPC_RUNG3`, `UPC_RUNG4`. **Wired, off:** `UPC_RUNG56`. Ascend 889,941 at the start, 883,522 at the close; Doom 758,199 throughout.

**Ledger, last row:** `develop merged in; close | 883522 | 883522 | +0 | 37514 | close2`.

**Progress:** Steps 1-228 and 279-314, less the stuck and waiting ones below: about 262 of 318.

### Stuck Report (highest downstream impact first)

| What | Steps | Why | Tried | Downstream | Needs |
|---|---|---|---|---|---|
| Rungs 5+6 on (Stop G) | 229-244 | With `UPC_RUNG56` on every golden moves: each carries the first 100 PCs after init starts, kernel PCs included, and init's first `open` traps at its 21st instruction. Goldens are not recaptured to pass a rung. | Built, reviewed (five findings fixed in the design), measured on: 529,305, every probe passes, all 12 boots inside 4,000,000 triggers. A way on with no recapture was booted and reverted: 12 of 12 SAME at 532,152. | Rung 7 (Steps 245-278, Stop H) is blocked on it. The freed budget (about 350,000) is what the two items below wait for. | Stevie's choice of three (below). |
| Rung 7 design and build (Stop H) | 245-278 | `[BLOCKED]` by Stop G: its design needs the kernel's fd table. | Not started. | The MRET/SRET kill-or-NOP question (Step 250) stays where Step 26 left it: NOP. | Stop G first. |
| Atomics through `translate_address` | 78, 81 | Verifier budget: four inlined translations +20,993, one shared arm +16,855; headroom 16,478 at the close, 14,471 when last checked against the shared arm (short by 2,384). | Both forms measured; `main.rs` restored. The load-time scan refuses any image holding an atomic instead, the kernel and bootstub included. | None while the scan holds: no image can carry one. | A pick: the linear-map form (+3,263), a U-mode refusal, or the shared arm once rungs 5+6 are on. |
| Fork slot scan skipping another parent's marked row; clearing a mark when a parent dies mid-fork | 116 (second half) | +11,789 against the headroom of the day. | Measured; not landed. | None without preemption, which the I9 assertion forbids at build time. | Preemption work, or the budget rungs 5+6 free. |

### Open calls for Stevie (oldest first, newest last)

1. The host engine `crates/monad-mbc/src/execute.rs` has neither the priv-3 refusal nor D6 (review RV-3): bring it in line, with a parity test?
2. The atomics: which form (Stuck Report).
3. The scan's probe hatch `UPC_ALLOW_USER_PRIVRET=1` (admits MRET and SRET only, for `hostile_reply`): keep it, or build the probe another way?
4. A user `ebreak` halts the machine, with no privilege test: refuse HALT at U-mode?
5. Fork's first entry is keyed on BPF's own mark, not a tag in a3 as D18 was first written: accept the departure.
6. Slot 0's literal limit: with rung 3 on, init cannot `write` a string literal from its first page (the kernel's copy-in refuses it). Probes keep their tokens in `.data`.
7. R-P1 (low): BPF's file read writes up to 15 bytes past a short read, inside the caller's slice. Gone when rungs 5+6 are on.
8. Rung 4's cost: a read blocked at the prompt retries through the kernel every tick (`ls`: 6.27 M to 63.3 M instructions, about a fifth longer on the wall). A cheap wait is designed in the ledger, not built. Its behaviour changes: no echo, a cooked line, control characters to the line discipline.
9. D21's choice that the kernel gives the first image its console (fds 0-2), yours to overrule; and no on-disk device node (mknod and T_DEVICE inodes are a later parked ADR if you want them).
10. **Rungs 5+6, the choice that turns them on:** (1) let the kernel's `copyinstr` read slot 0's first page and have the tracer print U-mode PCs only: no recapture, booted 12 of 12 SAME at 532,152; (2) one named recapture of the goldens' trace segment; (3) leave `UPC_RUNG56` off. Rung 7 follows the same answer.
11. Fuzzing was a SKIP (cargo-fuzz is not installed): the MBC executor was not fuzzed in this sprint. Installing it is your call.
12. After the security pass (ADR-135, SP table): SP-1's residual (a resident program exec'd by slot 0 lands at raw physical addresses up to 0x800000); SP-3's sidecar format marker; SP-4d, a harness boot with D6 off so `UPC_FORK_MARK` is the deciding control; SP-5b, JMPR/CALLR diagnostics storing at 0xE0000 and 0xE1000 from U-mode; SP-2's residual, the interpreter's own arms for the eight opcodes have no privilege test.
13. `cd` is stock sh's form: a bare `cd` still answers `exec cd failed`.
14. ADR-135 (D16 to D21, three review tables) is Proposed; ADR-145 and ADR-146 are parked. Acceptance is yours.
15. For the next UPC plan: Step 310's rebuild order ends with the Doom build, which overwrites the ascend object before the sweep; the first close gate read `same=0 diff=12` for that reason alone.

## 2026-11-21: rungs 5+6 on (lane `upc-rung56`, branch `sprint/upc-rung56`)

Open call 10 was ruled on 2026-11-18 (`docs/battle-plans/DECISION-PACK-2026-11-18.md`, row 25): "On; the goldens compare the user-mode trace only (the option that booted 12 of 12 SAME). Kernel-side trace changes stop being a regression signal." This section records the switch. Evidence: `references/s-stage-2c-log.md`, "Rungs 5+6 on (2026-11-21 ...)". Unattended, on west, in the lab only (`upc0`).

**Stop G reached 2026-11-21.** `UPC_RUNG56` is on: `const UPC_RUNG56: bool = true` and `#define UPC_RUNG56 1`. With it, as the ruled option needs: the tracer prints and counts U-mode PCs only, and the kernel's `copyinstr` reads a path string from slot 0's first page from 0x200 up.

| | Before | After |
|---|---|---|
| Ascend, verified instructions | 883,522 | 532,152 (-351,370; headroom 367,848 under 900,000) |
| Doom, verified instructions | 758,199 | 758,199 |
| Goldens (`~/tmp/golden`, not written) | 12 of 12 SAME | 12 of 12 SAME |
| Harness | ALL GREEN, 62 PASS, `rwprobe` skipped | ALL GREEN, 63 PASS, `rwprobe: 7 checks` |

Gate `rung56-after`: `same=12 diff=0 rc=0 doom=1 ascend=532152`. `check-ebpf-loads.sh`: 41 programs pass the verifier. `check-ebpf-insns.sh`: PASS; the baseline was rewritten by `--update`. `cargo test --release --locked --offline` in `cmd/upc-bootctl`: all ok (`parity_slot_tables` among them). Five mutants and the rollback (switch off: 883,522, 12 of 12) are in the ledger.

Plan steps: 223-230 and 235-243 done (the flip, the mutants of Step 229, the gate, the records). **Not done: Steps 231-234** (seven usertests bodies as probes: `ut_writetest`, `ut_createtest`, `ut_truncate1`, `ut_writebig`, `ut_bigwrite`, `ut_opentest`, `ut_copyinstr1`): outside this lane's task. Step 241's note: `dup` onto fds 0 to 2 works now (`RW: dup-to-zero`), which is what ush pipelines and redirection waited on; the ush grammar for `|` and `>` is in no plan.

### For Stevie (2026-11-21), each with the option taken

1. **The ruled option carries the slot 0 first-page read (open call 6).** The option that booted 12 of 12 SAME is two changes: the U-mode-only tracer, and `copyinstr` reading slot 0's first page. Row 25 names the option and ADR-135's ruling note calls it the "No recapture" way, so both landed. Taken narrowly: path strings only, slot 0 only, from 0x200 (past BootParams); `copyin` and `copyout` still refuse the page, and a probe line holds that. Init reads those bytes itself in U-mode, so nothing new is exposed. If you meant the tracer change alone: `upc_slot0_str_ok()` answering 0 undoes it, and then the twelve goldens need the one named recapture (measured: 0 of 12 SAME without it).
2. **The baseline file.** `check-ebpf-insns.sh --update` rewrites every row. Besides the ascend row it recorded six rows that had drifted on develop before this lane: `flow-tracker` egress and ingress 16,096 and 16,097 → 9,638 and 9,639, `latency-probe` `tcp_connect_exit` 515 → 520, `tcp_recvmsg_exit` 516 → 521, `tcp_sendmsg_exit` 400 → 406, `packet-marker` 5,497 → 5,605. All inside the gate's 10 %. They are in the commit because the update command writes them; nobody reviewed those six as growth.
3. **The namespace.** `upc0` belongs to `unheaded-upc-netns.service` on west (active, enabled), so `upc-netns.sh down` refuses and the lane left it up, as found.
4. Open calls 6 (slot 0 literal, for `write`), 7 (R-P1: closed by the flip, `RM: read-past-end=0`), 9 and 11 to 14 stand as written above, except where a later entry of this section says otherwise.
5. **Open call 2, the atomics: which form.** Row 25 says the flip unblocks the atomics fix, and your A-61 answer decided the design (through `translate_address`). Taken: that design as the plan's Step 78 wrote it, one translation per opcode, +21,382. The single shared arm would be about 4,500 instructions cheaper (+16,855 when measured on 2026-11-15) and is not built; say if you want it instead. `UPC_ATOMIC_TRANSLATE` off restores the earlier object. ADR-135's D17 part 2 is marked landed, not accepted: acceptance is yours.
6. **The fork reservation is dead code today, and incomplete for the day it is not.** Landed as the task and D18 describe it (scan skip, death clear). It cannot be exercised by a stock boot; the proof is on mutants. It also showed that preemption needs a third piece nobody has designed: the scheduler must not run a reserved row. Taken: landed with the gap written down and I9's assertion kept. `UPC_FORK_RESERVE` off restores the earlier object exactly (553,534). If you would sooner not carry unreachable code in the object, turn it off.

### Atomics through `translate_address` (Steps 78 and 81, no longer stuck)

Landed 2026-11-21 on the rungs 5+6 object: 532,152 → 553,534 (+21,382), headroom 346,466. A patched `XCHG` and a patched `LR_W` at U-mode read the ramdisk before the change and read 0 after it; with the translation taken out of one arm that arm reads the ramdisk again. The patched images boot only on a throwaway bootctl with the scan removed (never committed): the stock bootctl refuses them, which is the first control and is unchanged. `CAS` and `SC_W` were read, not booted. Gate `atomics-after`: `same=12 diff=0 rc=0 doom=1 ascend=553534`, harness ALL GREEN.

### Fork slot scan skip and death clear (Step 116's second half, no longer stuck)

Landed 2026-11-21 behind `UPC_FORK_RESERVE`: 553,534 → 549,509 (-4,025; the count fell, the skip alone is -4,480 and the clear +455). Neither path can run in a stock boot (no timer switch, I9), so each was shown on a mutant that plants a mark, with a throwaway probe: the skip moves the next children one slot up, the const off puts them back, and a dead slot's mark is gone when the slot exits. Gate `reserve-after`: `same=12 diff=0 rc=0 doom=1 ascend=549509`, harness ALL GREEN.

**Found by the same experiment:** the reservation is not enough for preemption. A skipped slot that was never built sits below `num_processes` once a later fork passes it, and the scheduler runs it: the mutant without the death clear halted at PC 0 in that slot. A timer switch under ascend needs a scheduler that leaves a reserved row alone as well. Written into ADR-135 and into the I9 assertion's message.

### Rung 7: grounding note (2026-11-21; no design, no code)

A plan freeze is in force (ADR-112) and this lane was told to ground rung 7, not to start it. What follows is read from the tree and from the plan.

- **What the plan holds.** S-STAGE-2C's plan has two phases for it: Phase 13 (Steps 245-252: a new Proposed ADR, "UPC rung 7: process authority in the owned kernel", an independent review, and two [DECIDE]s) and Phase 14 (Steps 253-278: the build, behind `UPC_RUNG7`). Neither was run. The skeleton (`EVOLUTION-1-STAGE-2-skeleton.md`, rung 7) names the gate and the usertests subset.
- **What it needs.** The kernel's `proc[]` decides pid, parent, `ZOMBIE`, exit status, reparenting to init and wait's answer. `wait` (3) becomes a kernel-routed number that copies the status out and retries (D7) while a child lives. BPF keeps the slice copy at fork, the page directories, the run queue, and frees a slot only when the kernel says reaped: a new reply flag, `REAP`, bounded like `KILL`. Exec stays BPF's.
- **What is already there.** Its prerequisite, the kernel's `ofile` as the only fd table, is met since today. The kernel keeps a row per BPF slot (ADR-109); the fork note (D12) dups the parent's files into the child's row; exit and kill already pass through the kernel and close the row (D13, D8); the retry hand-off (D7) and the reply word exist (flags 1, 2, 4, 8 and 16 are in use, rung 4 having taken 16 for `KBD` after the plan's Appendix C was written, so `REAP` would be 32). Probes `ut_exitwait`, `ut_twochildren`, `ut_forktest`, `ut_reparent2`, `forkloop`, `gate_nway`, `gate2` are in the harness. Verifier headroom was 367,848 at the flip (350,491 after the atomics translation and the fork reservation).
- **What is not there.** `kexit`, `freeproc`, `sched` and `sleep` panic on a slot row by design (ADR-109 I4, I7), and `kwait` frees and sleeps through them, so the kernel's own exit and wait cannot be called as they are. BPF's wait arm writes status 0 and reaps by its own `PARENT_OF` and `SCHED_STATE` words; exit status is carried nowhere. `NPROC` is 64 against 8 slots.
- **Open choices, all Stevie's or the design review's.** (1) Step 248: rung 7 inside this plan, or its own plan (the plan's own override sends it to a new plan if it needs scheduler rework, preemption or an ABI change; a new plan needs the freeze lifted). (2) Step 250: a U-mode MRET or SRET kills the process with status -1, or stays a NOP. (3) What init's exit does: every gate program runs as slot 0 and exits there, while stock xv6 panics "init exiting". (4) Whether `forks` and `waitpid`, which the goldens compare, are bumped where they are today (a retried wait must not count twice). (5) Whether preemption comes with it: it would need the I9 assertion lifted, and the fork reservation finished (the scan skip and the death clear landed today; the scheduler side is missing, see above).

