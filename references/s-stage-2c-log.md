# S-STAGE-2C log

Started 2026-11-15 on 18ec23a8 (branch sprint/s-stage-2c).
Plan: docs/battle-plans/S-STAGE-2C-battle-plan.md

## Ledger (verified insns, ascend)

| Change | OFF | ON | delta | net vs 921,036 | gate |
|---|---|---|---|---|---|
| priv3 + D6 | 889941 | 893507 | +3566 | 27529 | d6-after |
| bounded paths | 893507 | 895768 | +2261 | 25268 | paths-after |
| atomics: scan only (translate stuck) | 895768 | 895768 | +0 | 25268 | atomics-after |
| bss + sidecar (SRET mask) | 895768 | 895767 | -1 | 25269 | bss-after |
| fork key + I9 tripwire | 895767 | 895936 | +169 | 25100 | fork-after |
| RV-1 block guard (fork mark off) | 895936 | 899906 | +3970 | 21130 | review-after |
| rung 3 console write | 899906 | 885354 | -14552 | 35682 | rung3-after2 |
| T-F3 interim (by rung 3's arm) | 885354 | 885354 | +0 | 35682 | rdwr-after |
| fork mark on (`UPC_FORK_MARK`) | 885354 | 885529 | +175 | 35507 | mark-after |
| probes | 885529 | 885529 | +0 | 35507 | probes-after |
| rung 4 console read | 885529 | 883522 | -2007 | 37514 | rung4-after |
| rungs 5+6 wired, const off (on: 529,305, measured, not landed) | 883522 | 883522 | +0 | 37514 | rung56-off |
| `cd` in ush (no BPF change) | 883522 | 883522 | +0 | 37514 | cd-after |
| security pass fixes (bootctl, kernel `consolewrite`, probes) | 883522 | 883522 | +0 | 37514 | review3-after |
| develop merged in; close | 883522 | 883522 | +0 | 37514 | close2 |
| 2026-11-21: rungs 5+6 on (const, define, U-mode tracer, slot 0 path read) | 883522 | 532152 | -351370 | 388884 | rung56-after |
| 2026-11-21: atomics through `translate_address` (`UPC_ATOMIC_TRANSLATE`) | 532152 | 553534 | +21382 | 367502 | atomics-after |
| 2026-11-21: fork scan skip and death clear (`UPC_FORK_RESERVE`) | 553534 | 549509 | -4025 | 371527 | reserve-after |

## Before (Phase 0, 2026-11-15)

- `gate p0: same=12 diff=0 rc=0 doom=1 ascend=889941 head=18ec23a8`
- `check-ebpf-insns.sh`: PASS, 41 programs within 10 % of the baseline; 0 allowlisted over 900,000. `pgrep -c doom-runner` after the gate: 0.
- `p3` (lines with `priv_level != 3` in `main.rs`): 2. `adr_last`: 144.
- Harness lines for the probes this plan extends:
  - PASS  rmprobe: 8 checks (UPC_RUNG_UNLINK)
  - PASS  lnprobe: 9 checks (UPC_RUNG_LINK)
  - PASS  fdprobe: 49 checks (UPC_RUNG_FD)
  - PASS  chdirprobe: 24 checks (UPC_RUNG_CHDIR)
  - PASS  pipeprobe: 20 checks (UPC_RUNG_PIPE)
  - PASS  hostile_ptr: 5 checks (UPC_RUNG_MKDIR)
  - PASS  sleepprobe: pause(n) waits, a sibling runs meanwhile (UPC_RUNG_SLEEP)
  - hostile_pgd: D6 is off, a child wrote slot 0's pgd (M-247 open)
  - PASS  hostile_reply: a word stored ahead of a trap is wiped, init stays out of reach
  - hostile_reply: D6 is off, a child forged the reply by sret and a fork note (ADR-135 open)

## Run cost

- One boot with `UPC_VERIFIER_STATS=1`: `verified 889941 insns` (the baseline row).
- eBPF ascend build: Maximum resident set size (kbytes): 975276.
- Gate p0: sweep12 about 2 min, harness about 7 min (other lanes' Go tests were running).

## Phase 2 review (Step 29, 2026-11-15)

Read-only pass by the lane itself: a fork cannot start a review agent, so this is not the independent read the plan asks for. It is filed under For Stevie as owed.

| Question | Disposition |
|---|---|
| Does a legitimate path run `MRET` or `SRET` at priv 3? | No. Privilege is set in three places: `MRET` (from `MPP`), `SRET` (from `SPP`) and trap injection (`priv_level = 1`, `main.rs`). The image's only `mret` is `start_mbc.c`'s, at priv 0 (bootctl seeds M-mode); the `sret` sites are `trampoline_mbc.S` and `kernelvec_mbc.S`, both entered at priv 1. U-mode is reached only by an `sret` with `SPP = 0`. |
| Does D6 drop a store the corpus makes? | Not by reading: a child maps PDEs 0 and 1 (8 MiB); the heap window, the stack top (0x500000) and the argv frame (0x600000) are below it; init has an identity directory over the low 64 MiB. D6 on was booted on 2026-10-30 with 12 SAME, before the S-STAGE-2B rungs. Step 39's sweep is the test. |
| Do `hostile_slot` and `hostile_tf` change meaning with D6 on? | No. Both store below 8 MiB, in the child's own slice; the harness comment already says "Both hold with D6 off or on". |
| Found by the pass | The host-side executor (`crates/monad-mbc/src/execute.rs`) still takes `MRET` and `SRET` at any privilege. It is not in a boot and no parity test compares the two on this; noted For Stevie, not changed (outside Step 31's file). |
| Found by the pass | With the opcode test in, the harness's D6-off branch could no longer want `sret-forged=yes` and `kill-init-refused=yes`: the alias `sret` is refused too. The branch now wants `sret-forged=no` and pins only the fork-note forgery; the probe prints the KILL line only when the `sret` route forged. Needed for A7 (a D6 rollback must leave a green harness with the priv-3 test on). |

Red run (Step 27, new probe on the object before the fix, D6 off): no `HR:` line at all. The first child's `mret` was taken: the boot ended at `priv=1`, `num_proc=6`, `cur_pid=5`, `forks=5`, `rom_fault=805166`. R-F1 is a demonstrated loss of control, not only a claimed one.

## Phase 2 measurements (2026-11-15)

- Priv-3 test on both arms, D6 off: 890279 (delta +338 against 889941). Under the +3,000 line of Step 33, so Step 34's hoist was not needed.
- Step 35, D6 off: `mret-refused=yes`, `sret-refused=yes`, `own-slice-intact=yes`, `table-reached=yes`, `heap-intact=yes`, `note-refused=yes`, `note-forged=yes`, `sret-forged=no`, `done`.
- Step 36, mutant (no test on `SRET` only): `sret-refused=no`, `sret-forged=yes`, `kill-init-refused=yes`; `mret-refused=yes` holds. Restored: count back at 890279.
- D6 on: 893507 (delta +3228 on the opcode test; +3566 against 889941); `BUDGET-OK`, headroom 6493.
- Step 39, D6 on: `hostile_reply`: `mret-refused=yes`, `sret-refused=yes`, `own-slice-intact=yes`, `table-reached=no`, `route=closed`, `heap-intact=yes`, `done`; no `forged=yes`. `hostile_pgd`: `child=stored`, `pgd=same`, `tables=same`, `done`. Corpus sweep 8 of 8 SAME.
- Step 43, mutant (`d6_user_miss` forced false): `hostile_pgd` reads `pgd=changed`. Restored: 893507.
- Step 40: the commit of the opcode test alone has D6 off and counts 890279.
- Gate `d6-after`: `same=12 diff=0 rc=0 doom=1 ascend=893507`; harness lines `hostile_reply: D6 closes the route to the reply word`, `hostile_pgd: a child's stores outside its slice are dropped (D6)`, `hostile_slot`, `hostile_tf` all PASS.

## Slot 0 (Step 54, 2026-11-15)

Slot 0's base is 0, so init's VAs are physical addresses.

| Range | What |
|---|---|
| [0x0, 0x1000) | BootParams (0x160), init's `.text` and `.rodata` (`init.elf`: `.text` 0x0-0xEC4, `.rodata` to 0xF59). Refused for kernel-routed pointers already (`pa == 0`, ADR-135 D6) |
| [0x1000, 0x1020) | `init.elf` `.data` and `.bss` |
| up to 0x3010 | the largest image booted as init: `fdprobe` 0x3010, `pipeprobe` 0x2840 (`sh` ends at 0x2084, `ls` at 0x201F; neither runs in slot 0). Measured by linking every `.mbc`'s `.elf` once and reading `ALLOC` sections |
| 0xC000-0xC003 | TTY registers |
| [0xF000, 0x10000) | CSR window (SEPC at 0xF504, MEPC at 0xFD04), keyboard byte at 0xFFFF |
| [0x20000, 0x28100) | kernel `.text` (the code runs from ROM) |
| [0x28100, 0x289EC) | kernel `.rodata` |
| [0x100000, 0x100008) | kernel `.data` |
| [0x100100, 0x115520) | kernel `.bss`, the slot tables in it |
| (0x400000, 0x500000) | init's stack: `uexec.c` sets `sp = 0x500000 - 16`, "1 MiB of stack room" |

Init's stack is clear of every kernel range, so Step 55's override does not apply.

## Phase 3 measurements (2026-11-15)

- Red run (Step 56, probe on the tree before the fixes): `open-wild-refused=no` (the unbounded read found "README" in the next slot's slice), then exec of the planted "echo" succeeded and the child became `echo`; `slot0-kernel-name-refused=no` (the kernel made a directory from a name in its own `.rodata`), `slot0-copyout-refused=no` (`pipe` stored two fds in kernel `.data`).
- Step 57 as written (`user_phys` inside the two arms): 942,421, +48,914: BUDGET-FAIL. The exec arm alone was +41,496 (935,003): the refused state runs the 16-byte hash loop and the table scan before it reaches -1. Appendix A1's remedy, hoisted: one guard arm ahead of open and exec. Count 895768, +2261 against 893507: BUDGET-OK, headroom 4232.
- Step 61: six `=yes`, no `=no`, `done`; corpus sweep 8 of 8 SAME.
- Step 63 mutants, each rebuilt, booted and restored: guard without open → `open-wild-refused=no` and `open-ceiling-refused=no`; slot 0 window removed → `slot0-kernel-name-refused=no` and `slot0-copyout-refused=no`; `pipewrite`'s -1 removed → `pipe-badbuf-refused=no`. After restoring: 895768, six `=yes`.
- Gate `paths-after`: `same=12 diff=0 rc=0 doom=1 ascend=895768`; `hostile_path: 6 checks (UPC_D6_MISS_FAULT)` PASS, `hostile_ptr: 5 checks` PASS.

## Phase 4 measurements (2026-11-15)

- Step 74: the translator emits none of `XCHG`, `CAS`, `LR_W`, `SC_W` (no hit in `translator.rs`). Opcodes: LD 0x30, XCHG 0x3D, CAS 0x3E, MRET 0x47, SRET 0x48, LR_W 0x49, SC_W 0x4A.
- Step 76: `scripts/upc-mbc-patch-opcode.py` and its test were written together (seven cases pass); the red-first order was not kept. The marker is an instruction, not a literal: the compiler builds 0x00800400 as `MOVI`, `SHL`, `MOV`, `ADDI r13, 0x0400`, then `LD r13, [r13]`, so the tool takes `--marker-mask` (`--word-of-marker 0x1D000400 --marker-mask 0xFF00FFFF`). Patched word 16: `0x30DD0000` to `0x3DDD0000`.
- Step 77: `hostile_atomic` is built and sidecarred with the gate programs, not put in fs.img (the plan's "userland program" would have moved the `ls` golden).
- Step 78 as written (four `translate_address` calls): 916,761, +20,993 against 895768: BUDGET-FAIL. One ascend-only arm with a single translation: 912,623, +16,855: BUDGET-FAIL. Two shapes, so Steps 78 and 81 are `[STUCK]` on the verifier budget; `main.rs` is back at 895768. For information, not landed: the bounded linear map (`user_phys`, a miss to `D6_MISS_ADDR`) measured 899,031, +3,263, headroom 969.
- Step 80 on the object without translation (the red state, kept as the proof): the unpatched probe prints `xchg-translated=yes`, `done`; the patched copy prints `xchg-translated=no`, `done` (`xxd -s 0x400 -l 4 fs.img`: `40 30 20 10`, the superblock magic). The `.rv2mbc` hash gate passed the patched image: it hashes the map, not the words. Once the scan lands this cannot be booted from a stock tree.
- Steps 83-85: `cmd/upc-bootctl/tests/opcode_scan.rs` red first (3 of 4 failed with no scan), then green; all bootctl test results ok; fmt and clippy clean. The built bootctl on the patched image: `UserImageOpcode: hostile_atomic_x: word 16 (0x3DDD0000) is XCHG (opcode 0x3D)`.
- Found while building the scan: `hostile_reply.mbc` itself holds SRET at words 22 and 120 and MRET at word 62, so the scan as planned would have refused the probe that proves Phase 2. Hence the hatch `UPC_ALLOW_USER_PRIVRET=1` (returns only). No other built image holds any of the six opcodes except the kernel (MRET at 156, SRET at 11165 and 12781), which is not scanned.
- Gate `atomics-after`: `same=12 diff=0 rc=0 doom=1 ascend=895768` (unchanged: no BPF change landed); harness lines `opcode scan refuses a user image with XCHG (hatch or not) and one with MRET/SRET` and `hostile_reply: D6 closes the route to the reply word` PASS. No corpus image is refused.

## Phase 5 measurements (2026-11-15)

- Step 92: 37 `.mbc` and 37 `.data` in `target/` (38 with `bssprobe`): every program has a sidecar. fs.img packs extensionless copies of `init sh ls cat echo wc` plus `README`, `BIGFILE`, `sub/NOTE`. `.bss`: sh 0x74 at 0x2010 (after `.data` 0x2000-0x2010), ls 0x1F at 0x2000, cat and wc 0x210 at 0x1000, echo 0x10 at 0x1000, init 0x10 at 0x1010. All contiguous with the data region; the staged blobs are under 2 KiB against a 256 KiB stride.
- Step 93 [DECIDE]: option A (the sidecar carries `.bss` as zeros; no BPF change for it). Neither override holds.
- Step 94 red (before `--with-bss`): `bss-zero=no`, `malloc=yes` (malloc ran on the parent's inherited free list), `done`.
- Step 95: the translator test was red on the bss record (2 of 3 passed), then 3 of 3. It is at function level (section descriptors), not an ELF fixture.
- Steps 97-98: every `.mbc` and fs.img unchanged, 38 of 38 hashes OK. The sidecars had to be regenerated by removing the user `.mbc` files: the `.data` is a side product of the `.mbc` rule, so make does not rebuild it by itself.
- Step 99 first run: `bss-zero=yes`, `done`, and no `malloc` line. Cause, found in three instrumented boots: `SRET` masked the return PC to 16 bits, and `bssprobe` sits in PROGRAM_TABLE slot 5 (ROM 0x15000). The first trap the kernel answered for it (getpid, uptime or sbrk alike) resumed it in another image; with zeroed `.bss` malloc has to call sbrk, where before it did not. Pre-existing for `wc` (ROM 0x12000), which never makes such a call. Fix: the mask removed in the SRET arm. Count 895767 (-1 against 895768).
- Step 99 after the fix: `bss-zero=yes`, `malloc=yes`, `done`.
- Step 100 mutants, each restored: no `--with-bss` → `bss-zero=no`; the SRET mask back → no `malloc` line.
- Steps 102-104: `cmd/upc-bootctl/tests/sidecar_fatal.rs` red first (4 of 5 failed), then green. The check runs before the eBPF object opens (`UserSidecarMissing`, `UserSidecarBad`), and the two live-path sites are fatal as well. Two older fixtures needed a valid empty sidecar (`tests/opcode_scan.rs`, `tests/rv2mbc_sha.rs`). All bootctl test results ok; fmt and clippy clean.
- Gate `bss-after`: `same=12 diff=0 rc=0 doom=1 ascend=895767`; `bssprobe: 2 checks (UPC_RUNG_SBRK)` PASS; no corpus boot refused for a sidecar.

## Phase 6 measurements (2026-11-15)

- Step 114, departure: D18's first design (a `FORK_TAG` in a3) is not built. a3 is the caller's register, so a process can set the tag itself and skip the copy with no page-directory write at all, D6 on or off. The first entry is keyed on a mark in the child slot's PROC_TABLE row instead, which is the reservation mark of point (2) put to use.
- Probe on the tree before the change, D6 on: `copy-complete=yes`, `pde-plant-dropped=yes`, `a2-ignored=yes`, `done` (the plant is dropped by D6).
- The hole, old fork code with D6 off for the experiment: `copy-complete=no`, `pde-plant-dropped=no`.
- Key and tripwire: 895936, +169 against 895767: BUDGET-OK, headroom 4064. Doom object builds.
- The slot scan skipping rows marked by another parent: 907,725, +11,958 in all: BUDGET-FAIL. `[STUCK]` with that number (Step 118's own ruling: preemption, which needs it, is out of scope); the death clear with it.
- Step 119, D6 on: three `=yes`, `done`.
- Step 120, new fork code with D6 off for the experiment: `copy-complete=yes`, `pde-plant-dropped=no` (the plant landed and did not matter), `a2-ignored=yes`. D6 back on: 895936.
- Step 121: with `TIMER_SWITCH` forced true the ascend build fails, the message naming ADR-135 I9 (2 hits); restored, 895936.
- Gate `fork-after`: `same=12 diff=0 rc=0 doom=1 ascend=895936`; `forkloop`, `gate_nway`, `gate2`, the four `ut_*` probes and `hostile_fork: 3 checks` PASS.

## Independent review of Phase 2, and its findings (2026-11-15)

The independent read of Step 29 (read-only, at `10078582`) came back through the coordinator. Its answers to the plan's questions:

| Question | The review's answer |
|---|---|
| A legitimate `MRET` or `SRET` at priv 3? | None. Sites: `start_mbc.c:180`, `upc-bootstub start.c:129`, `trampoline_mbc.S:160`, `kernelvec_mbc.S:56`, all reached at priv 0 or 1; `uentry.S` has none; the scheduler never switches while priv is not 3 (`main.rs:3724`) |
| Is the NOP safe? | Yes: PC already advanced, priv stays 3, `reply_req` is set only inside the `SRET` arm |
| Does D6 break a legitimate access? | No: children map VA 0-8 MiB by two leaf PDEs; fork's copy and exec use physical slice arithmetic; slot 0 is identity-mapped |
| Did anything rely on `SRET`'s 16-bit mask as a bound? | No: PC indexes only ROM_MAP behind the range check at `main.rs:852`. `MRET` and the STVEC injection still mask, harmless while the kernel stays under 0x10000 words |

Findings, each verified here before it was acted on (ADR-135, section of this date):

- **RV-1 (High).** Red run of `hostile_block` on the tree at Phase 6's close, D6 on: `calls-refused=no`, `read-outside-refused=no`, `write-outside-refused=no`: a child wrote init's memory through 200 and read init's through 201 then 200. Guard in the two arms' conditions: 900,333 (+4,397). Guard as a number alias ahead of every arm: 900,081 (+4,145): both BUDGET-FAIL against 895936. With D18's mark behind `UPC_FORK_MARK` (off): 899906: BUDGET-OK, headroom 94. With the mark off and the guard removed the count is 895,767, the pre-Phase-6 object exactly. Probe with the guard: three `=yes`, `done`. Mutant (guard removed): three `=no`. `hostile_fork` still three `=yes` (D6 on).
- **RV-2 (Medium).** `PUSH` 0x1A, `POP` 0x1B, `INT` 0x17, `IRET` 0x18 added to bootctl's scan; test red first (1 of 5 failed), then green; all bootctl results ok. No built user image holds one (every `.mbc` dry-run-scanned).
- **RV-3 (Low).** Recorded For Stevie.
- Seen while verifying RV-2, not acted on: the translator turns `ebreak` into `HALT`, and `HALT` has no privilege test, so a user program that executes `ebreak` halts the machine. No built user image holds a `HALT`. For Stevie.
- Gate `review-after`: `same=12 diff=0 rc=0 doom=1 ascend=899906`; `hostile_block: 3 checks`, `hostile_fork: 3 checks`, the opcode scan line and `hostile_reply` PASS. The kernel's own block calls are unaffected: all 12 corpus boots read the ramdisk.

## Phase 7, measured and not landed (2026-11-15)

- Step 132's refusal (an inode-kind fd in the BPF write arm answers -1): 900,528, +622 against 899,906: BUDGET-FAIL with 94 of headroom. `main.rs` restored, count 899,906.
- Reorder under Step 30's override: Phase 9 (rung 3, which compiles the BPF console write loop out) runs next; Phase 7 and `UPC_FORK_MARK` (+169) follow it. Phase 9 was not started in this session.

## Phase 9, rung 3 (2026-11-15)

Run under Step 30's override, ahead of Phase 7 and with the fork mark off: the object had 94 instructions of headroom.

- Gate `rung3-before`: `same=12 diff=0 rc=0 doom=1 ascend=899906` at `b2a8f49c`.
- Run cost before (4,000,000 triggers each, the boot ends with `sh` waiting for a key): `ls` insn_count 5,488,183; `cat BIGFILE` 5,129,411; `echo peace and love` 5,096,265. Write calls: `ls` prints through `printf`, one `write` per byte; `cat` writes up to 512 bytes a call; `echo` one `write` per word and one per separator.
- Design: ADR-135 D19. The alias tests "not a file", not "is the console": the gate programs run as init and print on fd 1 without opening `console`, so BPF's table calls their fd 1 free.
- Const off (Step 167): `vcount` 899,906 equals `offv`; `sweep` 8 of 8 SAME with the kernel's `consolewrite` and `sys_write` changed.
- Const on (Step 170): 885,354, delta -14,552 against 899,906 (the byte loop, its two translations and the re-entry are gone; the alias adds one `FD_TABLE` read). BUDGET-OK, headroom 14,646.
- `conprobe`: 4 of 4, `CW: done`; `abc` on the TTY once; the 600-byte write arrives as 599 `x` in one run, no NULs. `sweep` 8 of 8 SAME.
- Mutants (Step 176): `consolewrite` back on the word store: `sweep` 0 SAME, 8 DIFF. The alias row disabled with the const on: `conprobe` prints nothing (every write answers -1, `printf` included). Both restored; `vcount` 885,354.
- Gate `rung3-after`: `same=12 diff=0 rc=1`: `devprobe` failed. It runs as init and wrote two string literals; `user.ld` puts `.rodata` in init's first page, which the kernel's `copyin` refuses for slot 0 (D6, D16). Before the rung BPF read the bytes itself. The tokens moved to `.data`; the limit is For Stevie. No corpus program is touched: init and `uinit` print through `printf` (a stack byte), everything else runs in a child slot.
- Gate `rung3-after2`: `same=12 diff=0 rc=0 doom=1 ascend=885354`; `conprobe: 4 checks`, `devprobe`, `fdprobe: 49 checks` PASS, ALL GREEN.
- Run cost after (same three boots, 4,000,000 triggers): `ls` insn_count 6,271,580 (+783,397); `cat BIGFILE` 8,157,219 (+3,027,808); `echo peace and love` 5,168,583 (+72,318). A tick that waits counts one instruction and a working tick up to 16, so the work added is about 16/15 of each delta. That gives about 3,000 MBC instructions per `write` call (trap, `sys_write`, `copyin`, return; from `echo`: 26 calls) and about 220 per byte inside a call (from `cat BIGFILE`: 14,570 bytes in 512-byte calls). Before the rung a byte cost one tick.
- Step 179 [DECIDE]: all 12 corpus boots finish inside the harness's 4,000,000 triggers (12 SAME), so the recommendation holds: proceed. No budget changed.

## Phase 7 after rung 3 (2026-11-15)

- The refusal is rung 3's own arm: with `UPC_RUNG3` on, the alias has taken every `write` whose fd is not a file, so what reaches BPF's arm is a file fd below 16 and the arm answers -1. No kind test in the arm, no instruction added: `vcount` 885,354 before and after. Measured alone on the pre-rung object it was +622 (900,528, BUDGET-FAIL).
- `fdprobe` 49 → 51: `rdwr-file-write-refused=yes` (plain `O_RDWR` on README gives a fd below 16; `write` answers -1), `readme-intact=yes` (the first 8 bytes read back the same).
- Mutant (Step 134): the arm answering n instead of -1: `rdwr-file-write-refused=no`, 50 yes, 1 no. Restored; `vcount` 885,354.
- Gate `rdwr-after`: `same=12 diff=0 rc=0 doom=1 ascend=885354`; `fdprobe: 51 checks` PASS, ALL GREEN.
- With `UPC_RUNG3` off the old behaviour is back (the bytes print, n is answered): the off object has no room for a refusal of its own.

## Fork mark back on; Stop C (2026-11-15)

- `flip UPC_FORK_MARK false true`: 885,529, +175 against 885,354 (it measured +169 on the Phase 6 object). BUDGET-OK, headroom 14,471.
- `hostile_fork`: `copy-complete=yes`, `pde-plant-dropped=yes`, `a2-ignored=yes`, `HF: done`. `sweep` 8 of 8 SAME.
- Gate `mark-after`: `same=12 diff=0 rc=0 doom=1 ascend=885529`; `hostile_fork: 3 checks`, `fdprobe: 51 checks`, `conprobe: 4 checks` PASS, ALL GREEN.
- Order reached: Stop A, Stop B, rung 3 (the plan's Stop E, Phase 9), then Stop C (Phase 7 and the fork mark). Phase 8 (Stop D) and Phase 10 are not started.
- Atomics, for the record: the shared-arm translation measured +16,855 and the headroom is 14,471, so it still does not fit, by 2,384. The linear-map form (+3,263) and a U-mode refusal (a few hundred) do.

## Phase 8, the A-62 probes (2026-11-15)

Inherited from the stalled shift and kept: the edits to `rmprobe`, `lnprobe`, `sleepprobe`, `hostile_ptr`, the new `bigprobe`, and the asserter's `a7=16` line. Each matches Steps 140-144. The resume gate ran on the committed tree first (`gate resume`: `same=12 diff=0 rc=0 doom=1 ascend=885529`), with the edits set aside as a patch.

No BPF change: `vcount` 885,529 before and after (`bpf-unchanged`).

| Probe | Lines | Result |
|---|---|---|
| `rmprobe` | RM 8 → 11 | `file-with-data-unlinked`, `gone`, `blocks-reused-clean` all yes |
| `lnprobe` | LN 9 → 12 | `nlink-two`, `nlink-one`, `bytes-after-unlink` all yes |
| `sleepprobe` | SL 4 → 5 | `pause-ticks=yes`; note `late-by=323` (a pause of 50 woke 323 ticks late on this boot) |
| `bigprobe` (new) | BG 3 | `size`, `read-back`, `trunc` yes: 14,000 bytes, past the 12 direct blocks into the single-indirect block |
| `hostile_ptr` | HU 5 → 9 | `unlink-bad`, `link-old-bad`, `link-new-bad`, `chdir-bad` all yes |

**Finding, from `rmprobe`'s first run (`blocks-reused-clean=no`).** The read-back was right; the probe's own source buffer had been overwritten. BPF's file read (`main.rs`, the `FD_INODE` branch of `read`, H-FS6) copies a fixed 16-byte chunk into the user buffer whatever count it returns, so the last short read of a file writes up to 15 bytes past the end of what it reports. Here: file of 2,500 bytes, last read asked for 4, got 4, and 12 more bytes landed after the buffer, in the next array. The bytes come from the data block past the file's size (zeros in this run; block residue in general). The write is bounded to the caller's own slice (`user_phys(pid, buf_va, READ_CHUNK)`), so it does not cross a slot. The comment in the code calls it harmless because "our inode readers always pass buffers ≥ READ_CHUNK"; a program that reads into an exact-size buffer is not such a reader. Not fixed in BPF: the guarded copy is what "doubled verifier states" when it was tried (the comment says so), and rungs 5+6 move file reads to the kernel, where `readi` copies exactly n. The probe now reads into a buffer with a 32-byte guard after it and prints the count as a note, `RM: read-past-end=12`; it reads 0 once the kernel serves the read. `lnprobe`'s 8-byte read buffer was on the stack for the same reason and is 32 bytes now. Filed For Stevie.

**The asserter and rung 3.** The S-STAGE-2B decoder (`ugl`) dropped NULs and matched tokens. With rung 3 on, every character a user program prints is a tagged trap, and the probe kernel prints a `UT:` line on each, so the two writers interleave character by character and the old decode shreds both (first try: `RUNG0-ASSERT fail 1 fork notes for 0 forks`). Kernel `printf` stores a word per character (the character and two NULs on the ring); a user write is one byte. `scripts/upc-tty-split.py` splits on that and gives each writer its text back. The awk also learned the two programs the plan names: `GF: fork-ret=N` (getpidfork) counts as a fork return, and `FL: child` (forkloop, which prints no pid) counts toward the fork total, which is what the fork-note mode checks. By hand: getpidfork `pass 89`, forkloop `pass 52`, both `pass 141`; one `pid=2` changed to `pid=5` gives `fail pid 5 for slot 1`. The probe kernel is built from a copy in a temp dir (the make rules write objects beside the sources), so the tree's kernel and `$K/xv6-mbc.mbc` are untouched.

### Attack surface, Phase 8 probes

| Probe | What it attacks | What bounds it | Line |
|---|---|---|---|
| `rmprobe` | unlink of a file that holds blocks, then reuse of those blocks by a new file | `ufs.c` `itrunc` and `bfree`; the new file reads back its own bytes | `RM: file-with-data-unlinked`, `RM: gone`, `RM: blocks-reused-clean` |
| `rmprobe` (note) | a short file read into an exact-size buffer | nothing in BPF today: the fixed 16-byte copy writes past the count, inside the caller's slice | `RM: read-past-end=12` (a note; 0 when the kernel serves reads) |
| `lnprobe` | link's count as the kernel keeps it, and the data under the second name once the first is gone | `sys_link` `nlink++`, `sys_unlink` `nlink--`, read back through `fstat` | `LN: nlink-two`, `LN: nlink-one`, `LN: bytes-after-unlink` |
| `sleepprobe` | `pause(n)` returning early | the tick deadline in `upc_pause` | `SL: pause-ticks`; note `SL: late-by=N` |
| `bigprobe` | a file past the 12 direct blocks (14,000 bytes) written by the kernel and read by BPF's `fs_walk` | `bmap`'s single-indirect block on the write; the indirect lookup in BPF's read | `BG: size`, `BG: read-back`, `BG: trunc` |
| `hostile_ptr` | a path pointer at the user ceiling (0x00800000) given to unlink, link (either argument) and chdir | `argstr` through `upc_user_pa` (ADR-135 D6) | `HU: unlink-bad`, `HU: link-old-bad`, `HU: link-new-bad`, `HU: chdir-bad` |
| rung 0 asserter | ADR-109 I1-I8 on every tagged trap of two programs, on a probe kernel | `scripts/upc-rung0-assert.awk`; a mutant with one pid changed must fail | harness line "rung 0 asserter" |

Gate `probes-after`: `same=12 diff=0 rc=0 doom=1 ascend=885529` at `25d39563`; `rmprobe: 11 checks`, `lnprobe: 12 checks`, `hostile_ptr: 9 checks`, `bigprobe: 3 checks`, sleepprobe and the asserter PASS, ALL GREEN.

## Phase 10, rung 4 (2026-11-15)

Before gate: `probes-after` stands in for `rung4-before` (same code; only docs and the ledger changed between `25d39563` and Stop D `7fca9618`): `same=12 diff=0 rc=0 doom=1 ascend=885529`. `offv` 885,529.

Step 188, what was read: BPF's console read pops one ring byte per call, decodes `(raw >> 1) & 0xFF`, bumps `STAT_TTY_READS` per byte delivered, and on an empty ring rewinds the PC and ends the tick. It does not echo: the `echo hello` golden prints `$ hello`, not the typed line. The goldens' `tty_r` equals the input's byte count (`ls` 3, `echo hello` 11, `multi` 22, `blank` 15). The kernel's `uartgetc` reads the status word in RAM and `KBD_MAP[0]` (the Doom-era keyboard word), not the ring, so it cannot feed the read; D20's hand-over rides the trap instead (ADR-135 D20).

Const off (`UPC_RUNG4 = false`): `kbuild; gbuild; ebuild; bbuild`, `vcount` 885,529 = `offv` (`off-equal`), `sweep` 8 of 8 SAME, `parity_slot_tables` and `parity_surface` pass. The slot table is one word longer (`kbd_in` at 0x820, size 0x824); the kernel's `.bss` grew by it and the goldens did not move.

Rung on (`flip UPC_RUNG4 false true`): 883,522, delta -2,007 against 885,529 (the console branch of the read arm is out; the offer at injection and the ring advance at the reply are in). BUDGET-OK, headroom 16,478.

- `crprobe` with `--input $'hello\nagain\n'`: `line=yes`, `second-line=yes`, `badptr=yes`, `CR: done`; `forks=1 tty_r=12 waitpid=1`.
- `sweep12`: 12 of 12 SAME (the multi-line and blank-line inputs among them).
- Live check (Step 200): `printf 'echo hi\n' | ... upc-bootctl console --stdin ...` prints `$ hi`. The subcommand has no `--triggers`; it ran as written otherwise.
- Mutant (Step 201): the retry on an empty ring replaced by `return 0`: `CR: line=no`, `CR: second-line=no`, and the `echo hello` golden DIFF. Restored; `vcount` 883,522.
- Gate `rung4-after`: `same=12 diff=0 rc=0 doom=1 ascend=883522`; `crprobe: 3 checks, two lines read whole, tty_r=12`, `conprobe`, `devprobe`, the asserter PASS, ALL GREEN. `check-ebpf-insns.sh`: 41 programs within 10 % of the baseline.

Run cost after rung 4 (4,000,000 triggers; the boot ends with `sh` waiting for a key): `ls` insn_count 63,260,801 (it was 6,271,580 after rung 3); `echo peace and love` 63,254,989 (5,168,583). The work of the commands did not grow: the prompt did. A read blocked on an empty ring used to rewind the PC and end the tick (one instruction); it now traps into the kernel, finds no line, takes the retry hand-off and runs again on the next tick, so every waiting tick does a full tick's work (up to 16 instructions). 4,000,000 triggers allow 64,000,000. One `ls` boot took 11.4 s on the wall; the eight-boot sweep with the const off ran at about 9.5 s a boot. The same holds for an idle `console` session: the host does about sixteen times the BPF work per tick while the shell waits. A cheap wait is possible and not built: the kernel marks the slot-table word when it retries with nothing buffered, and BPF holds the read in its own arm (PC rewind, tick ends) while that mark is there and the ring is empty. Filed For Stevie.

## Phase 11, rungs 5+6 design (2026-11-15)

State read first: `d980e84b`, tree clean, baseline row 883,522 (ascend) and 758,199 (Doom), consts `UPC_RUNG3`, `UPC_RUNG4`, `UPC_FORK_MARK` on. No build, no boot in this phase's first half.

**Step 207, BPF sites** (`main.rs` at `d980e84b`). The plan's `grep` for `fd_inode_|FD_INODE_MAP|fs_walk|FD_TABLE` counts 58 lines; the accessors are named `fd_kind`, `fd_set_kind`, `fd_alloc`, `fd_inode`, `fd_set_inode`, `fd_set_offset`, `fd_clear_inode` here, so the callers were listed by those names too.

| Where | Lines | What it reads or writes |
|---|---|---|
| Map definitions | 374, 383 | `FD_TABLE` (kind per fd, 16 x 8 slots), `FD_INODE_MAP` (`[inum, offset]`) |
| Accessors | 610-704 | the seven functions above |
| `write` 16 | 2288-2347 | no table read in the arm; under rung 3 what is left answers -1 (T-F3 interim) |
| fork finalize | 2523-2531 | copies the parent's 16 kinds into the child's row (16 reads, 16 writes) |
| `open` 15 | 2862-3084 | console name → `fd_alloc`, `fd_set_kind` (2911-2915); superblock check and re-entrant dirent scan through `fs_walk` (2921-3015); bind → `fd_alloc`, `fd_set_inode` (3062-3066) |
| `fstat` 8 | 3085-3131 | `fd_kind`, `fd_inode`, dinode through `fs_walk` |
| `dup` 10 | 3141-3161 | `fd_kind`, `fd_alloc`, `fd_set_kind` (kind only: the inode binding is not copied) |
| `close` 21 | 3162-3178 | `fd_kind`, `fd_set_kind`, `fd_clear_inode` |
| `read` 5 | 3179-3325 | inode branch: `fd_inode`, block resolve through `fs_walk`, `fd_set_offset` (3192-3281); console branch is already `!UPC_RUNG4` |
| routing | 4013, 4019, 4025, 4034-4039 | `>= 16` row; rung 3 asks `fd_kind != FD_INODE`; rung 4 asks `fd_kind == FD_CONSOLE`; `open` by masked flag and by `cwd_off_root()` |
| trap injection | 4114 | rung 4's keyboard offer tests `a7 == 5 && fd < 16`; no table read |
| `component_match` | 4392-4427 | `fs_walk` constants for the dirent name compare |

Not sites: `exit` 2 (kernel-routed since D13, no BPF fd work), `exec` 7 (resident-program lookup, no fd), `mknod` 17 (returns 0, touches nothing), the D16 path bound (reads the path's address range, not a table). No host tool opens either map by name (`grep` over `cmd/upc-bootctl` and the crates finds only `monad-common`'s model and its tests).

**Step 208, kernel side.**

| File | Function | What matters for the rung |
|---|---|---|
| `usysfile.c` | `fduser` (45), `argfd` (54) | D10's +16 and the `fd < 16` refusal for slot rows |
| | `fdalloc` (78) | returns the `ofile` index; `sys_pipe` indexes with it |
| | `sys_dup` (93), `sys_close` (156), `sys_fstat` (169) | stock bodies over `argfd` |
| | `sys_read` (107) | D20: a slot row's fd below 16 goes straight to `upc_consoleread`, no `ofile` lookup |
| | `sys_write` (129) | D19: a slot row's fd below 16 goes straight to `consolewrite`; a copy that fails before the first byte is -1 |
| | `sys_open` (364) | `argstr`, `create` or `namei`, the `T_DEVICE` major check, `filealloc` + `fdalloc`, `fduser` at the return |
| `ufile.c` | `fileclose` (74) | `iput(ff.ip)` for `FD_INODE` and `FD_DEVICE` alike |
| | `filestat` (102) | `ilock(f->ip)` for both |
| | `fileread` (121), `filewrite` (149) | a device goes through `devsw[major]`; `consoleinit` wires `consoleread` (sleeps) and `consolewrite` |
| `uconsole.c` | `upc_consoleread` (214) | the polled read for a slot row (D20) |
| `upc_slots.c` | `upc_slots_init` (207) | runs after `userinit()` and after `fileinit` and `consoleinit` (`umain.c` 38, 63, 66, 67) |
| | `upc_forknote` (285) | `filedup`s the parent's `ofile` into the child's row; closes what was left there |
| | `upc_close_row` (83), `upc_kill` (99) | D13 and D8: the row's files are closed |
| `uproc.c` | `userinit` (274) | sets `cwd`; `ofile` is empty |

Slot 0: `proc[0]` is the `userinit` process and its `ofile` is empty until something opens. `init.c` and `uinit.c` do `open("console", O_RDWR)`, then `dup(0)` twice; the literal is in `.rodata`, which `user.ld` places in the first page (`init.elf` `.rodata` ends at 0xF59), and the kernel's copy-in refuses that page for slot 0 (D6, D16). `fs.img` has no `T_DEVICE` inode: the `ls` golden's only "console" is the kernel's `after consoleinit` line. Of the 36 gate programs, 33 never open a console and print on fd 1; nine call `open` with a literal path.

**Step 209.** ADR-135 D21 written (design only). Departures from the plan's eight points are tabled there. The two that change the shape: the kernel gives the first image its console (`upc_slots_init`, three references to one console file), because init's literal is unreadable to the kernel and 33 first images open nothing; and rung 4's keyboard offer is made on every routed read, because "fd below 16" no longer means "the console". Step 209.3's [DECIDE] taken by the recommendation (name match, no disk inode). Review (Step 210) pending.

**Steps 210-211, the independent review and its dispositions.** A separate read-only agent read D21 at `ae44692b` against the code (nothing built or booted). The lane re-read each finding in the tree before writing its disposition; the table is in ADR-135 under D21.

| Id | What the reviewer found | Checked in the tree | Disposition |
|---|---|---|---|
| F1 | A console read gated on `upc_is_slot_row()` would send slot 0 to `consoleread`, which sleeps | `upc_is_slot_row` is `p > &proc[0]`; `sleep()` and `sched()` panic only for it; `upc_slot_active()` is set for every tagged trap, slot 0 too; `crprobe` reads only in a child | Accepted: `upc_slot_active()`. `crprobe` gains a slot 0 read (Step 216) |
| F2 | `hostile_path`'s `open-ceiling-refused` flips: the kernel finds the NUL inside the slice | The probe plants `README` at 0x007FFFF8; BPF refuses it by `user_phys(pid, a0, 128)` ahead of the arm; `open` is aliased to 11 before that arm under the const | Accepted: D16's rule is exec-only under the const; the line becomes `open-unterminated-refused` |
| F3 | `fd_kind` heads the `read` arm outside the guarded branch; helper names | `let kind = fd_kind(pid, fd)` is the arm's third statement; `name_match_phys` and `name_is` are the helpers | Accepted: the whole arm is guarded |
| F4 | A recycled `struct file` keeps its `ip` | `fileclose` stores `ref = 0` and `type = FD_NONE` only; `filealloc` sets `ref = 1` only | Accepted: both console-file makers store `ip = 0`; guards on `ip == 0` |
| F5 | Five things left to guess | read against `usysfile.c`, `ufile.c`, `upc_slots.h` | Settled in D21: mode by `omode`; `#define UPC_RUNG56` in `upc_slots.h` with a parity check against the const; D19's -1 in `filewrite`'s device branch; the match before `begin_op`; `cwd->inum == ROOTINO` |
| Q2, Q3, Q4, Q6, Q8 | No problem found | the reviewer's evidence re-read | Recorded, a line each, in ADR-135 |
| Q7 | Which goldens could move | `fs.img`'s contents by `Makefile.mbc-userland` | A moved golden is a stop, not a recapture |

## Phase 12, rungs 5+6 (2026-11-15)

Before gate (Step 215), on the committed tree at `0c66f515`: `gate rung56-before: same=12 diff=0 rc=0 doom=1 ascend=883522`, harness ALL GREEN. `offv` 883,522.

**Step 216, probe changes** (each a change with a reason, not a recapture). The probes learn which build they run on from a pipe, whose fds were always the kernel's (`p[0] >= 16` before the rung, a low number after), so every changed line holds with the const off or on.

| Probe | Old text | New text | Why |
|---|---|---|---|
| `fdprobe` | `created`, `trunc-fd`, `rw-kernel`, `dup`, `reuse-cwd`: `fd >= 16` | `KFD(fd, want)`: 16 or above before the rung; after it, exactly the lowest free fd (`want` is what `dup(1)` answered just before) | D21 point 2: a kernel fd is the `ofile` index |
| `fdprobe` | `bpf-fd`: `fd >= 0 && fd < 16` | `BFD(fd, want)`: below 16 before; the lowest free fd after | every open is the kernel's |
| `fdprobe` | `rdwr-file-write-refused`: plain `O_RDWR` on README, `write` is -1 | `rdwr-file-write`: -1 before the rung; after it the write answers 2 and the two bytes are written back | T-F3 closes under the rung (D21 point 6) |
| `chdirprobe` | `kfd()` `fd >= 16`, `bfd()` `fd < 16`, `read-rel`, `bpf-read` | the same two forms | as `fdprobe` |
| `pipeprobe` | `fds`: both ends at 16 or above | at 16 or above before; after, the two lowest free fds | as `fdprobe` |
| `crprobe` | two lines, both read by a child; input `hello\nagain\n`; `tty_r=12` | a first line read by init itself (`slot0-line`), then the two; input `init\nhello\nagain\n`; `tty_r=17`; 4 checks | review F1: slot 0 is the row the kernel's `sleep()` does not refuse |
| `hostile_path` | `open-ceiling-refused`: `README` planted in the slice's last 8 bytes is -1 | `open-unterminated-refused`: 8 bytes with no NUL before the ceiling is -1 | review F2: D16's 128-byte rule is exec-only once open is the kernel's; the new line is refused by BPF's bound and by the kernel's copy-in alike |
| `devprobe` | `open("console", O_RDWR)` with a literal | the name from `.data` | the open is init's own, and under the rung the kernel copies the path in |
| `rwprobe` (new) | | `write-lands`, `read-back`, `restored`, `console-fstat` (-1: no inode, and the kernel goes on), `dup-to-zero`; `rung_gate UPC_RUNG56 rwprobe 6000000 RW 5` | Step 216 |

`uinit.c` is **not** edited. D21 put its console name in `.data`; doing that moved the `uinit_echo` golden with the const off (see "What stands in the way" below), so the edit was taken back and the golden is SAME again.

**Steps 217-219, wired with the const off.** Kernel: `#define UPC_RUNG56 0` in `upc_slots.h` with `UPC_KFD_BASE` derived from it; `sys_open`'s console name match before `begin_op` (`upc_console_name`: one leading `./` or `/` stripped, the remainder is `console`, a relative name only from `cwd->inum == ROOTINO`), mode by `omode`, `ip = 0` stored; `fileclose` and `filestat` guard `ip == 0`; `fileread` sends a console read inside a tagged trap (`upc_slot_active()`) to `upc_consoleread`; `filewrite` keeps D19's -1 there; `upc_slots_init` gives slot 0 one console file on fds 0-2; `sys_read` and `sys_write` lose their fd-below-16 shortcuts under the define. BPF: `const UPC_RUNG56: bool = false` with a build-time assertion that it needs the fd, chdir and pipe rungs and rungs 3 and 4; one unconditional routing row for 5, 8, 10, 15, 16, 21 and `!UPC_RUNG56 &&` on every row that asks `FD_TABLE` or the cwd mask; `!UPC_RUNG56 &&` on the open, fstat, dup, close and (whole) read arms and on fork's table copy; rung 4's keyboard offer on every read. `surface.rs` and `XV6_SYSCALLS` did not need a change: the arms stay in the source, so the surface list is the same (`parity_surface` passes). `parity_slot_tables` gained `kernel_rung56_is_bpfs` (the header's define equals the const; `UPC_KFD_BASE` is 0 under it, else 16).

Step 220: `kbuild; gbuild; ebuild; bbuild`: `vcount` 883,522 = `offv` (`off-equal`); `sweep` 8 of 8 SAME. With the const off the changed probes still pass: `fdprobe` 51, `chdirprobe` 24, `pipeprobe` 20, `hostile_path` 6, `crprobe` 4 with `tty_r=17` (a slot 0 console read works today through `sys_read`'s shortcut), `devprobe` both tokens; `uinit_echo` SAME.

**Step 223, the rung on** (`flip UPC_RUNG56 false true` and `#define UPC_RUNG56 1`): **529,305, delta -354,217** against 883,522. BUDGET-OK, headroom 370,695. Object file 897,784 → 729,216 bytes. `readelf -r` on the on object: the only relocations against `FD_TABLE` and `FD_INODE_MAP` are in `.rel.debug_info` and `.rel.BTF`; no instruction refers to either (review F3). The map definitions themselves stay in the object (a `#[map]` static is always emitted).

**Found on the way: a header change rebuilt nothing.** `Makefile.mbc` has `%.o: %.c` and generates no dependency files, so the first `kbuild` after the define changed relinked the old objects. The rung-on object then ran against a kernel still numbering fds from 16: `dup(1)` answered -1, opens answered 16, 17, 18 (a throwaway `rwprobe` printed the numbers), and six probe lines read `no`. `Makefile.mbc` now names `upc_slots.h` as a prerequisite of the ten objects that include it; after that the same throwaway read `dup(1)=3`, opens 3, 4, 5, `close(0)=0`, `dup=0`. `parity_slot_tables` reads the two source lines, so it cannot see a stale kernel build; the Makefile line is the control.

**Step 225, with the kernel really on.** `rwprobe` 5 of 5 (`write-lands`, `read-back`, `restored`, `console-fstat`, `dup-to-zero`), `fdprobe` 51, `chdirprobe` 24, `pipeprobe` 20, `bigprobe` 3: no `=no` line, every `done`.

**Step 226 fails: `sweep12` is 0 of 12 SAME.** Each of the twelve boots was kept (`$S/r56on/`) and compared with its golden byte by byte:

| Boot | Outside the 100-PC trace | `forks`, `waitpid`, `tty_r` | First trace PC that differs |
|---|---|---|---|
| `ls`, `cat README`, `cat BIGFILE`, `cat sub/NOTE`, `ls sub`, `echo hello`, `echo peace and love`, `wc README`, `nosuch`, `multi`, `blank` | identical | identical | the 21st |
| `uinit_echo` | differs (1,093 bytes against 1,016) | differ | the 20th |

Every boot finished inside 4,000,000 triggers and ended at the prompt; `insn_count` for the eight single-command boots is 63.54 M to 63.57 M (rung 4: `ls` 63.26 M), so the fd calls going to the kernel cost about 0.3 M instructions a boot and no trigger budget changes.

What moves is the Phase 1.4 PC tracer's output. `TRACE_HEAD` is armed with 100 at the first return to U-mode and the dispatch loop prints every PC while it counts down, whatever the privilege; the bytes are in the TTY and so in every golden (the `ls` golden's trace is exactly 100 PCs, all init's). Until this rung nothing inside that window trapped to the kernel: init's `open` and two `dup`s were BPF's. Under the rung the `open` is the 21st instruction: the golden reads `... 41E5 41E6 4011 4012 ...`, the boot reads `... 41E5 3435 3436 ...` (kernel PCs) to the end of the window. The open also answers -1, because init's `"console"` literal is in the first page the kernel's copy-in refuses for slot 0, so init goes through `mknod` and a second `open`; its output is the same, its path through the code is not. `uinit` exits when both opens fail, which is the larger difference in its row. (Its console name was moved to `.data` in Step 216 and taken back: that edit alone shifted its code and moved `uinit_echo` with the const off.)

**Steps 227-228.** This is not a kernel `stat` layout or a trigger problem, and no second attempt at the code changes it: a golden moves, which the rules make a stop. A7: the const and the define back to off, `kbuild; gbuild; ebuild; bbuild`: `vcount` 883,522 = `offv`. Stop F stands. The wiring stays in the tree, off.

**Measured for the decision, not committed.** With two further changes the rung is on and nothing moves: (a) `copyinstr` accepts slot 0's first page (reads only; the "slot 0 literal" question already filed), so init's and uinit's own `open("console")` succeed and their user-mode path is the golden's; (b) under the rung the tracer prints U-mode PCs only. Booted: `sweep12` **12 of 12 SAME** (`uinit_echo` among them, `init.c` and `uinit.c` untouched), `vcount` 532,152 (+2,847 for the tracer test over 529,305). Both changes were reverted before any commit: (a) is Stevie's to decide and was not to be landed by this lane.

**Gate on the committed tree, rung off** (`a0c25e62`): `gate rung56-off: same=12 diff=0 rc=0 doom=1 ascend=883522`, harness ALL GREEN (55 passes): `fdprobe` 51, `chdirprobe` 24, `pipeprobe` 20, `hostile_path` 6 with the new line, `crprobe` 4 checks with `tty_r=17` (init's own read among them); `rwprobe gate skipped: UPC_RUNG56 is off`. The baseline row stays 883,522. Steps 229-244 are not run: they need the rung on.

## Phase 15, cd in ush (2026-11-15)

Resume gate first, on the committed tree at `99ae2fa9`: `gate resume7: same=12 diff=0 rc=0 doom=1 ascend=883522`, harness ALL GREEN. `offv` 883,522.

- Step 280: `ls.tty` reads `sh             2 3 7496`. ush's loop is fork, exec, wait; `chdir` is already in `user.h`.
- Step 281 [DECIDE]: the recommendation, one recapture of `ls.tty` and `ls.full`, since Step 284 printed `only-sh-size`. The override does not hold.
- Step 282: `cd <dir>` in the shell itself, stock sh's form: the line starts `cd `, the newline is chopped, `chdir`, and `cannot cd <dir>` on failure. The two header comments say so. `cd` with no argument is still a command (`exec cd failed`), as in stock sh, whose test is `cd ` too.
- Step 283: `gbuild`, `sweep12`: 11 SAME, exactly `DIFF ls`.
- Step 284: `only-sh-size`. The one differing line is `sh             2 3 7496` → `sh             2 3 7652`; `ls.stats` is equal (`forks`, `waitpid`, `tty_r` do not move).
- Step 286: harness block "ush cd", one boot with `cd sub`, `ls`, `cd ..`, `ls`, `cd nosuch`, `echo after`: three lines. Tried on a real capture (three pass) and on the pre-cd `ls` capture, which has no builtin (three fail). `bash -n` syntax-ok. The boot printed: sub's listing (`.` inode 10, `..`, `NOTE 2 11 223`), then the root's, then `cannot cd nosuch`, then `after`; `forks=4 waitpid=3`.
- Step 287: `gate cd-after: same=12 diff=0 rc=0 doom=1 ascend=883522`, the three "ush cd" lines PASS, ALL GREEN. The eBPF object is untouched: the count is `offv`.

### Golden policy

| Date | Golden | What moved | Why | HEAD | Rollback |
|---|---|---|---|---|---|
| 2026-11-15 | `ls.tty`, `ls.full` (not `ls.stats`) | sh's size field, 7496 → 7652, and nothing else in `ls.tty`; `ls.full` is the whole boot log of that run | ush gains cd (Stevie 2026-11-12, round 3; plan Step 281) | `99ae2fa9` plus this phase's `ush.c` (the commit of Steps 279-289) | `~/tmp/golden/ls.tty.pre-cd`, `ls.full.pre-cd` |

This is the plan's one named recapture. No other golden was written in S-STAGE-2C.

## Phase 16, security pass (2026-11-15)

The plan closes from Stop F (rungs 5+6 are wired and off, rung 7 is not built), so the table holds what is on in the tree at `070654cc`: no `REAP` flag, no wait status, no fd call routed by `UPC_RUNG56`.

### Attack surface

One row per landed change. "Harness line" is the line of `scripts/upc-regression.sh` that runs the probe; Step 292 counted each (`grep -c`, last column).

| Landed change | What a hostile caller can try | What bounds it | Probe line that proves it | Harness line | Count |
|---|---|---|---|---|---|
| MRET and SRET refused at priv 3 (Phase 2) | execute a privileged return from U-mode | `opc == op::MRET && cpu.priv_level != 3`, the same for SRET: at priv 3 the opcode has no arm and is a NOP | `HR: mret-refused=yes`, `HR: sret-refused=yes` | `hostile_reply: D6 closes the route` | 1 |
| D6 on (`UPC_D6_MISS_FAULT`) | store through a PDE the child planted, to reach the slot table, the reply word or another slice | `d6_user_miss()`: a U-mode page-walk miss goes nowhere | `HR: own-slice-intact=yes`, `HR: table-reached=no`, `HR: route=closed`, `HR: heap-intact=yes`; `hostile_pgd` | `hostile_reply: D6 closes the route`, `hostile_pgd: a child` | 1, 1 |
| Bounded open and exec paths (D16) | a path pointer outside the slice, or with no NUL before the ceiling | `user_phys(pid, va, len)` ahead of the open and exec arms | `HQ: open-wild-refused`, `HQ: exec-wild-refused`, `HQ: open-unterminated-refused` | `rung_gate UPC_D6_MISS_FAULT hostile_path` | 1 |
| Slot 0's window (D16) | make the kernel copy to or from init's first page or kernel memory | `upc_user_pa()` for slot 0 | `HQ: slot0-copyout-refused`, `HQ: slot0-kernel-name-refused` | the same | 1 |
| Pipe write with a bad buffer | a bad first byte answered 0, not -1 | `pipewrite` returns -1 when nothing was copied | `HQ: pipe-badbuf-refused`, `PI: bad-refused` | the same; `rung_gate UPC_RUNG_PIPE pipeprobe` | 1, 1 |
| Opcode scan at load (D17): XCHG, CAS, LR_W, SC_W; MRET, SRET unless the probe hatch | a hand-made user image with an untranslated atomic or a privileged return | `scan_user_image` in `upc-bootctl`, before the eBPF object opens | dry runs of a patched image | `opcode scan refuses a user image with XCHG` | 1 |
| Opcode scan widened (review RV-2): PUSH, POP, INT, IRET | the same, with a stack pointer as a physical address | the same match arm; the hatch does not admit them | dry runs, one per opcode | **added in Step 292:** `opcode scan refuses a user image with PUSH` | 1 |
| `.bss` zeroed at exec through the sidecar; a bad or missing `.data` sidecar refuses the boot | run with whatever is in memory where strings, globals and `.bss` should be | `check_user_sidecars` (`UserSidecarMissing`, `UserSidecarBad`); the staged blob's zero tail | `BS: bss-zero`, `BS: malloc`; three dry runs | `rung_gate UPC_RUNG_SBRK bssprobe`; **added in Step 292:** `sidecar check refuses` | 1, 1 |
| Fork's first entry keyed on BPF's own mark (`UPC_FORK_MARK`, D18) | forge the first-entry test from a register or a planted PDE | the mark in BPF's PROC_TABLE row | `HF: a2-ignored`, `HF: copy-complete`, `HF: pde-plant-dropped` | `rung_gate UPC_D6_MISS_FAULT hostile_fork` | 1 |
| I9 tripwire (no timer switch under ascend) | none at run time | `const _: () = assert!(...)`: the object does not build | none: a build failure | `cargo_build_ok ebpf_cargo_build -p monad-cpu-ebpf --features ascend-linux` (the "ascend build" line) | 1 |
| Block syscalls 200/201 refuse a U-mode caller (review RV-1) | read or write any physical address through the ramdisk | `cpu.priv_level != 3` on both arms, -ENOSYS | `HB: calls-refused`, `HB: read-outside-refused`, `HB: write-outside-refused` | `rung_gate UPC_D6_MISS_FAULT hostile_block` | 1 |
| Rung 3: console write is the kernel's (D19) | a bad buffer pointer, a length past the slice, a zero length | `consolewrite` copies in through `either_copyin`, so through `upc_user_pa()`; `sys_write` answers -1 when the copy fails before the first byte | `CW: badptr`, `CW: long-write`, `CW: zero`, `CW: ret-four` | `rung_gate UPC_RUNG3 conprobe` | 1 |
| A write to a file fd in BPF is refused (T-F3 interim) | `O_RDWR` on a regular file, then write: bytes went to the console | rung 3's leftover arm answers -1 | `FD: rdwr-file-write` | `rung_gate UPC_RUNG_FD fdprobe` | 1 |
| Rung 4: console read is the kernel's (D20) | a bad buffer pointer; a read from slot 0 | `upc_consoleread`: `n < 0` or `upc_user_pa(dst) == 0` is -1 | `CR: badptr`, `CR: slot0-line`, `CR: line`, `CR: second-line` | `crprobe: 4 checks` | 1 |
| Rung 4's keyboard offer (`kbd_in`, slot table 0x824) | forge the offered byte, or the reply that says it was taken | only BPF writes the slot table; D6 keeps a child off it and off the reply word | `HR: table-reached=no`, `HR: route=closed` | `hostile_reply: D6 closes the route` | 1 |
| Kernel fd calls on files (fstat, read, write; in since S-STAGE-2B, numbered from 16) | a bad `struct stat` or buffer pointer | `upc_user_pa()` in the kernel's copy-out and copy-in | `FD: fstat-ptr-refused`, `FD: read-ptr-refused`, `FD: write-ptr-refused` | `rung_gate UPC_RUNG_FD fdprobe` | 1 |
| Phase 8 probes (unlink with data, nlink, pause, a file past one block, path pointers at the ceiling) | see "Attack surface, Phase 8 probes" above | | `RM:`, `LN:`, `SL:`, `BG:`, `HU:` lines | `rung_gate UPC_RUNG_UNLINK rmprobe`, `rung_gate UPC_RUNG_LINK lnprobe`, `sleepprobe`, `rung_gate UPC_RUNG_FD bigprobe`, `rung_gate UPC_RUNG_MKDIR hostile_ptr` | 1, 1, 3, 1, 1 |
| Rung 0 asserter in the harness | a kernel row that disagrees with BPF's slot | `scripts/upc-rung0-assert.awk`; a changed pid must fail | harness line | `rung 0 asserter:` | 1 |
| `cd` in ush (Phase 15) | a path the shell passes to `chdir`: the shell's own line buffer | `argstr` through `upc_user_pa` in the kernel, as for every chdir | `HU: chdir-bad`; the three "ush cd" lines | `rung_gate UPC_RUNG_MKDIR hostile_ptr`; `ush cd:` | 1; 6 (three pass, three fail texts) |
| Rungs 5+6, wired and **off** | nothing while off: the rung's kernel paths are inside `#if UPC_RUNG56` (`usysfile.c`, `ufile.c`, `upc_slots.c`); what is outside it is `fileclose` and `filestat` testing `ip == 0`, which no file has while off; the BPF arms are the ones on before | the const and the define, held equal by `parity_slot_tables` | `RW:` lines run only with the const on | `rung_gate UPC_RUNG56 rwprobe` (skipped while off) | 1 |

**Step 292.** Two rows had no harness line, only `cargo test` in `cmd/upc-bootctl` (`tests/opcode_scan.rs`, `tests/sidecar_fatal.rs`), which the harness does not run. Both got one, as dry runs (a loader refusal has no pointer to make bad, and nothing is booted):

- `opcode scan refuses a user image with PUSH, POP, INT or IRET (review RV-2)`: the image the XCHG line already patches, the same word rewritten to 0x1A, 0x1B, 0x17, 0x18 in turn, each refused by name with the hatch set.
- `sidecar check refuses a missing and a truncated .data sidecar, and passes the image's own`: a clean image with no sidecar (`UserSidecarMissing`), with its sidecar cut to seven bytes (`UserSidecarBad: truncated at record 0`), and with its own (`.data sidecars: 1 user image`).

Run alone, the block prints three PASS. Against a loader stand-in that refuses nothing, all three lines FAIL (the XCHG line too), so the lines are not vacuous. `bash -n` and shellcheck clean.

Not in the table because they did not land: the atomics through `translate_address` (stuck on budget; the scan refuses them at load instead), the fork slot-scan skip, `UPC_RUNG56` on, rung 7.

Known and open, already filed For Stevie: a user `ebreak` halts the machine with no privilege test; R-P1, BPF's file read writes up to 15 bytes past a short read inside the caller's slice (gone when rungs 5+6 are on); the host engine `crates/monad-mbc` has neither the priv-3 refusal nor D6; refusals and D6 misses count nothing.

### Steps 293-294: independent review and dispositions (2026-11-15)

The coordinator's read-only reviewer read `git diff develop...HEAD -- crates/xv6-mbc ebpf/monad-cpu-ebpf cmd/upc-bootctl scripts/upc-regression.sh` at `99ae2fa9` (before Phase 15's `ush.c` and Step 292's two harness lines). The table with each finding's full text is ADR-135, "Security pass review of S-STAGE-2C". Each finding was checked against the code first; `gate resume8` on the committed tree before any change: `same=12 diff=0 rc=0 doom=1 ascend=883522`.

| # | Finding | Severity | Disposition | Proof |
|---|---|---|---|---|
| SP-1 | sidecar addresses unbounded | Medium | fixed, `b0526886` | cargo `sidecar_fatal`: 5 new tests red, then 11 pass; harness line "sidecar check refuses a record in kernel memory, one that wraps and a resident span over the staging stride"; dry run of all 43 shipped user images: 0 refused |
| SP-2 | the scan skips the kernel and the bootstub | Low, latent | fixed, `b0526886` | cargo `opcode_scan`: 2 new tests red, then 7 pass; harness line "opcode scan refuses a kernel image with PUSH and passes the kernel"; the kernel holds MRET 1, SRET 2, the bootstub MRET 1, neither any of the eight |
| SP-3 | a sidecar cannot say whether `.bss` is in it | Low | Makefile prerequisite added; the format marker is filed | touched translator: all 43 sidecars regenerated, `fs.img`, `sh.mbc`, `init.mbc` sha256 unchanged |
| SP-4a | `open-unterminated-refused` vacuous | Harness | fixed | mutant (open's bound 4 bytes): `open-unterminated-refused=no`, the other five `yes` |
| SP-4b | `unlink-bad`, `link-old-bad`, `link-new-bad` vacuous (and `chdir-bad`, `ceiling-refused`) | Harness | fixed | kernel mutant (`upc_user_pa` without the ceiling): six lines `no` |
| SP-4c | `a2-ignored` vacuous | Harness | fixed | mutant (fork keeps the probe's a2): `a2-ignored=no` twice |
| SP-4d | no boot makes `UPC_FORK_MARK` the deciding control | Harness | recorded (D18), filed | |
| SP-5a | `consolewrite` has no count clamp | Low | fixed | `CW: clamp=no` on the kernel before the clamp, `=yes` after; sweep 8 of 8 SAME |
| SP-5b | JMPR/CALLR diagnostics store at 0xE0000/0xE1000 from U-mode | Low | recorded, filed | |

Every mutant was rebuilt, booted and restored; after the last restore the ascend object verifies at 883,522. No fix added an eBPF instruction: the changes are in `cmd/upc-bootctl`, the kernel's `consolewrite`, three probes, the harness and the userland Makefile.

Attack-surface rows the fixes add (same columns as the table above):

| Landed change | What a hostile caller can try | What bounds it | Probe line that proves it | Harness line | Count |
|---|---|---|---|---|---|
| Sidecar records bounded (SP-1) | a `.data` record in kernel memory, in the next slot, wrapping, or a span over the staging stride | `opscan::check_user_regions`, by path before the object opens and again on the bytes loaded | three dry runs | `sidecar check refuses a record in kernel memory` | 1 |
| Kernel and bootstub scanned (SP-2) | reach an atomic, PUSH, POP, INT or IRET in privileged ROM by a tagged RET | `opscan::scan_privileged_image`: the boot is refused | a dry run of a kernel copy with one PUSH word | `opcode scan refuses a kernel image with PUSH` | 1 |
| `consolewrite` takes at most 4096 bytes a call (SP-5a) | print the whole slice in one trap | the clamp; a short write | `CW: clamp` | `rung_gate UPC_RUNG3 conprobe` (5 checks) | 1 |

Gate `review3-after`, with every fix and the two new harness lines: `same=12 diff=0 rc=0 doom=1 ascend=883522`; `hostile_ptr: 9 checks`, `hostile_path: 6 checks`, `hostile_fork: 3 checks`, `conprobe: 5 checks` PASS, the five scan and sidecar lines PASS, ALL GREEN.

### Steps 295-296

- Step 295: `grep -c '^const UPC_D6_MISS_FAULT: bool = true;'` → 1. `grep -cE 'op::(MRET|SRET) && cpu.priv_level != 3|!user_ret'` → 2.
- Step 296 [DECIDE], fuzz: `command -v cargo-fuzz` finds nothing and `cargo +nightly fuzz --version` answers "no such command: `fuzz`". The target exists (`crates/monad-mbc/fuzz`, `fuzz_mbc_execute`). **SKIP**, by the plan's recommendation. A SKIP is not a pass: the MBC executor was not fuzzed in this sprint. Installing cargo-fuzz is Stevie's call.

Gate `sec-after`, with the two new harness lines, on the tree of this commit: `same=12 diff=0 rc=0 doom=1 ascend=883522`; `opcode scan refuses a user image with PUSH, POP, INT or IRET (review RV-2)` PASS, `sidecar check refuses a missing and a truncated .data sidecar, and passes the image's own` PASS, the three "ush cd" lines PASS, ALL GREEN. The eBPF object and the kernel are untouched by this phase.

### Steps 298-300 (2026-11-15)

Repo gates on `539cf5b0`, each read on its own line: no `.go` file in the branch's diff; `check-ebpf-insns.sh` PASS (41 programs within 10 % of the baseline; 0 allowlisted over 900000); `check-spdx.sh` OK (1,936 files); `check-gitleaks.sh` PASS (5,946 tracked files); `check-gates-can-fail.sh --only check-ebpf-insns` PASS (1 gate proven to fail when violated); `cargo clippy --release -- -D warnings` and `cargo fmt --check` in `cmd/upc-bootctl` clean; `cargo test --release` there green; ruff clean on the three UPC scripts and `check-ruff.sh` PASS (it was red on this branch: `scripts/upc-tty-split.py` had a shebang and no mode bit, fixed in `539cf5b0`); `tests/scripts` 114 passed; `bash -n` and shellcheck clean on the harness.

ADR-135 stays **Proposed**. No rung 7 ADR was written (Phases 13 and 14 were not run).

## Phase 18, close (2026-11-15)

- Step 309: develop (`46d3246f`) merged in as `de471513`. Conflicts, by Appendix A9: `docs/adr/ADR-INDEX.md` and `wiki/ADR-Index.md` (both sides' rows in number order: develop's 139 to 144, this branch's 145 and 146; counts recomputed: 143 entries, Accepted 66, Proposed 40, the status sum equals the row count). Everything else merged by itself: the baseline file keeps develop's `latency-probe` rows and this branch's ascend row; the timeline has both entries and the mirrors check clean (77 milestones). The merge brought S-EVIDENCE-RUNS, S-ZHEN-FRONT, S-WIKI-VOR, the trace collector work, the Go 1.27.4 pins and the parked truth fixes; none of it touches `ebpf/monad-cpu-ebpf`, `crates/xv6-mbc` or `cmd/upc-bootctl`.
- Step 310: kernel, userland and gates, the ascend object, bootctl and the Doom object rebuilt; `cargo test --release` in `cmd/upc-bootctl` green.
- Step 311, first run (`close`): `same=0 diff=12 rc=0 doom=1 ascend=883522`, harness ALL GREEN. Cause: Step 310's order ends with the Doom build, which writes the same object path, so the twelve sweep boots ran on the Doom object; the harness builds each object before it uses it and was unaffected. Not a regression: with the ascend object rebuilt, `echo hello` is SAME. The gate was run again with the ascend object in place.
- Step 311, second run (`close2`): `same=12 diff=0 rc=0 doom=1 ascend=883522 head=de471513`, harness ALL GREEN (62 PASS lines, no FAIL); `check-ebpf-insns.sh` PASS (41 programs within 10 % of the baseline; 0 allowlisted over 900000); `BUDGET-OK 883522 (headroom 16478, ladder net 37514)`. For the next plan: put `ebuild` after `dbuild` in the rebuild line, or the sweep runs on the Doom object.

## Rungs 5+6 on (2026-11-21, lane `upc-rung56`, branch `sprint/upc-rung56`)

Ruling: `docs/battle-plans/DECISION-PACK-2026-11-18.md` row 25, open call 10: "On; the goldens compare the user-mode trace only (the option that booted 12 of 12 SAME). Kernel-side trace changes stop being a regression signal." That is the first of the three options recorded on 2026-11-15 (no recapture; one named recapture of the trace; leave the rung off, or retire the tracer, also a recapture). Worktree `~/tmp/wt/upc-rung56` from develop `c3fa967e`; everything rebuilt there (translator, kernel, userland and gates, the ascend object, bootctl). The goldens in `~/tmp/golden` were read and not written.

**Before (rung off, the tree as branched).** `vcount` 883,522; `sweep12` 12 of 12 SAME; harness ALL GREEN (62 PASS lines, `rwprobe gate skipped: UPC_RUNG56 is off`), Doom `verified 758199 insns`.

**What changed.**

| Where | Change |
|---|---|
| `ebpf/monad-cpu-ebpf/src/main.rs` | `const UPC_RUNG56: bool = true`. The tracer's test is `n > 0 && !(cfg!(feature = "ascend-linux") && UPC_RUNG56 && cpu.priv_level != 3)`: under the const a PC outside U-mode is neither printed nor counted |
| `crates/xv6-mbc/adapters/upc_slots.h` | `#define UPC_RUNG56 1`; `UPC_INIT_STR_LO` 0x200 (BootParams is 256 bytes at 0x100) |
| `crates/xv6-mbc/adapters/upc_slots.c` | `upc_slot0_str_ok(va)`: inside a tagged trap, slot 0, `va` in [0x200, 0x1000); 0 with the define off |
| `crates/xv6-mbc/adapters/uvm.c` | `copyinstr` gives up on a page only when `walkaddr` answers 0 and `upc_slot0_str_ok(srcva)` is 0. `copyin` and `copyout` are untouched |
| `crates/xv6-mbc/upstream/user/rwprobe.c`, `scripts/upc-regression.sh` | two lines run by init itself, `RW 5` → `RW 7` (below) |

Nothing else was needed: no Makefile carries the switch (`Makefile.mbc` already names `upc_slots.h` for the ten objects that include it), and bootctl holds no expectation of it beyond `parity_slot_tables`, which reads the two source lines.

**Measured.**

- Rung on: **532,152**, delta **-351,370** against 883,522. `BUDGET-OK 532152 (headroom 367848, ladder net 388884)`. Object file 897,784 → 729,256 bytes. It is the number recorded on 2026-11-15 for this way (529,305 for the const and the define alone, +2,847 for the tracer's test).
- `sweep12`: **12 of 12 SAME**, `uinit_echo` among them; `init.c` and `uinit.c` untouched; no golden written. Each boot was kept and its `forks`, `waitpid` and `tty_r` are the golden's. `insn_count`: `ls` 63,571,865, `cat BIGFILE` 63,548,330, `echo hello` 63,538,879 (4,000,000 triggers; the prompt's wait is what it counts, as after rung 4).
- `rwprobe`, first run with the new lines: `slot0-literal-open=no`. The probe's `.text` is 0x10B4 bytes, so its `.rodata` starts past the first page and the literal was not where the line said (`nm`: `lit` at 0x10C0). The address test in the line caught it. The probe now stores the path at 0xF00 itself (RAM this image does not use: the code runs from ROM; init reaches it through its identity map) and reads the store back before it opens. Then: seven `=yes`, `RW: done`.

**Mutants, each rebuilt, booted and restored.**

| Mutant | Result |
|---|---|
| `upc_slot0_str_ok` answers 0 (the first page refused to `copyinstr` again) | `RW: slot0-literal-open=no`; `sweep12` 0 SAME, 12 DIFF |
| the tracer's test back to `n > 0` | `vcount` 529,305; `sweep12` 0 SAME, 12 DIFF |
| `copyin` takes the first page too | `RW: slot0-first-page-closed=no`, the other six `yes` |
| `UPC_KFD_BASE` 16 under the define (plan Step 229) | `rwprobe` prints nothing: the first image's fds 0 to 2 are out of the caller's reach |
| no console name match in `sys_open` (plan Step 229) | `sweep12` 0 SAME, 12 DIFF |

After the last restore: `vcount` 532,152, `rwprobe` seven `=yes`.

**Rollback checked (A7).** The const false and the define 0, kernel and object rebuilt: `vcount` 883,522 (the off object exactly), `sweep12` 12 of 12 SAME. The tracer's test and `upc_slot0_str_ok` are dead with the switch off. Switched back on: 532,152.

**Gate `rung56-after`:** `same=12 diff=0 rc=0 doom=1 ascend=532152` (worktree at `c3fa967e` plus this commit's changes). Harness ALL GREEN, 63 PASS lines, no FAIL: `rwprobe: 7 checks (UPC_RUNG56)`, `fdprobe: 51 checks`, `chdirprobe: 24 checks`, `pipeprobe: 20 checks`, `bigprobe: 3 checks`, `conprobe: 5 checks`, `crprobe: 4 checks ... tty_r=17`, `hostile_path: 6 checks`, `devprobe`, `gate2 ISOLATION-PASS`, `gate_nway 3x NWAY-ok`, `forkloop`, the four `ut_*`, the two `uinit` lines; `Doom verifier: verified 758199 insns`.

### What the goldens compare from here

The goldens are the files they were (`~/tmp/golden`, last written 2026-11-15 for `ls`). What changed is what the object prints into a boot's TTY: the 100-PC window after init first reaches U-mode now holds 100 U-mode PCs, where it held the first 100 PCs of any privilege. Before this rung nothing in that window trapped to the kernel, so the two were the same bytes, which is why no recapture was needed. From here a kernel-side change inside that window (a different path through `sys_open`, a longer `copyinstr`) leaves the goldens SAME. The kernel side's regression signals are the probes (`rwprobe`, `fdprobe`, `chdirprobe`, `pipeprobe`, `bigprobe`, `crprobe`, `conprobe`, the hostile set) and the corpus output itself, which is still compared byte for byte.

### Attack surface, rungs 5+6 on

| Landed change | What a hostile caller can try | What bounds it | Probe line that proves it | Harness line |
|---|---|---|---|---|
| Every fd call is the kernel's (`UPC_RUNG56`) | a bad buffer or `struct stat` pointer; a fd outside the caller's row; a write to a read-only fd | `argfd` on the caller's own `ofile`; `upc_user_pa()` in copy-in and copy-out | `FD: fstat-ptr-refused`, `FD: read-ptr-refused`, `FD: write-ptr-refused`, the `RW:` lines | `rung_gate UPC_RUNG_FD fdprobe`, `rung_gate UPC_RUNG56 rwprobe` |
| `copyinstr` reads slot 0's first page from 0x200 up | init (slot 0, trusted by design) names a path whose bytes lie in its first page; a child cannot (its VA 0 is its own slice, never physical page 0) | `upc_slot0_str_ok()`: tagged trap, `myproc() == &proc[0]`, start in [0x200, 0x1000); BootParams and the words below it stay refused; `copyin` and `copyout` still refuse the page | `RW: slot0-literal-open`, `RW: slot0-first-page-closed` | `rung_gate UPC_RUNG56 rwprobe` |
| The tracer skips PCs outside U-mode | nothing: it is output only | | the twelve goldens | `sweep12` |

The first-page read hands the kernel nothing init could not pass it from its stack: init reads those bytes in U-mode through its identity map. Slot 0's literal limit (open call 6) is answered for path strings only; a `write` of a first-page literal by init still answers -1, so probes keep their tokens in `.data`.

Closed by the flip, as D21 said: T-F3 (a file opened `O_RDWR` is written by the kernel: `RW: write-lands`), R-P1 (BPF's 16-byte chunk past a short read: the BPF file read is out of the object, and `rmprobe`'s note reads `RM: read-past-end=0`, where it read 12 on 2026-11-15), D10's stale offset between parent and child (by the design: one `struct file` shared through `filedup`; no probe line of its own).

## Atomics through translate_address (2026-11-21, lane `upc-rung56`)

Plan Steps 78 to 81, `[STUCK]` on 2026-11-15 on the verifier budget, run on the rungs 5+6 object (`offv` 532,152). ADR-135 D17 part 2, the decided design as written: one translation per opcode.

- **How a patched image is booted now.** The opcode scan refuses it, at the path check and again on the loaded words. A throwaway bootctl was built from the tree with `opscan::scan_user_image` returning at once, into `~/.cache/unheaded-lanes/upc-rung56/bc-noscan`; the two source files were restored before anything else ran (`git status` clean). For each boot the throwaway binary stood at the worktree's bootctl path and the stock one was copied back after (`cmp` equal). Nothing of it is committed.
- **Images.** `scripts/upc-mbc-patch-opcode.py` on `hostile_atomic.mbc`, marker `0x1D000400` mask `0xFF00FFFF`: word 16 `0x30DD0000` → `0x3DDD0000` (`XCHG`), and a second copy → `0x49DD0000` (`LR_W`, which has `LD`'s operand shape). Each with the stock `.rv2mbc`, its sha256 and `.data`.
- **Red, on the 532,152 object:** both copies print `HA: xchg-translated=no`, `HA: done`. A child's atomic at VA 0x800400 read the ramdisk's superblock word.
- **The change:** `atomic_addr()` (`translate_address` under `cfg!(feature = "ascend-linux") && UPC_ATOMIC_TRANSLATE`) in the `XCHG`, `CAS`, `LR_W` and `SC_W` arms; the reservation keeps the virtual address. Count **553,534**, delta **+21,382** (it measured +20,993 on the 895,768 object). `BUDGET-OK 553534 (headroom 346466, ladder net 367502)`.
- **Green:** both copies print `HA: xchg-translated=yes`, `HA: done`.
- **Mutant** (the translation taken out of the `XCHG` arm only): 548,430; the `XCHG` copy prints `=no`, the `LR_W` copy `=yes`. Restored: 553,534.
- **Const off** (`UPC_ATOMIC_TRANSLATE` false): 532,152, the earlier object exactly. Back on: 553,534.
- **Not booted:** `CAS` (fixed registers r0, r1, r2) and `SC_W` (a store): no compiled probe has their shape, so no one-word patch makes one. They are the same one-line change; read, not run.
- The stock bootctl still refuses the patched copy: `UserImageOpcode: hostile_atomic_x: word 16 (0x3DDD0000) is XCHG (opcode 0x3D)`.
- Not built: the single shared arm (+16,855 on 2026-11-15) and the linear-map form (+3,263). The decided design fits as written with 346,466 to spare.
- Gate `atomics-after`: `same=12 diff=0 rc=0 doom=1 ascend=553534`; harness ALL GREEN, 63 PASS lines; the two opcode scan lines PASS; `Doom verifier: verified 758199 insns`. `check-ebpf-loads.sh` 41 programs pass; `check-ebpf-insns.sh` PASS, baseline rewritten by `--update` (the ascend row only moved).

| Landed change | What a hostile caller can try | What bounds it | Proof | Harness line |
|---|---|---|---|---|
| Atomics translate (D17 part 2) | an image that did not come from the translator, holding `XCHG`, `CAS`, `LR_W` or `SC_W` with an address outside its slice | first the load-time scan (the boot is refused); then `translate_address`: at U-mode the access stays in the caller's mapping, a miss reads 0 and drops the store | the two patched boots above, on a scan-less bootctl; not repeatable from a stock tree, by design | `opcode scan refuses a user image with XCHG (hatch or not) and one with MRET/SRET` (the scan; the translation has no harness line) |

## Fork slot reservation (2026-11-21, lane `upc-rung56`)

Plan Step 116's second half, `[STUCK]` on 2026-11-15 (+11,789 against 4,064 of headroom), run on the 553,534 object. ADR-135 D18: the scan skips a row another parent has marked; a slot that exits or is killed clears its own marks. Behind `UPC_FORK_RESERVE` (needs `UPC_FORK_MARK`, asserted).

**Counts.**

| Object | Verified | Against 553,534 |
|---|---|---|
| scan skip and death clear (landed) | 549,509 | -4,025 |
| scan skip only | 549,054 | -4,480 |
| death clear only | 553,989 | +455 |
| `UPC_FORK_RESERVE` false | 553,534 | 0: the earlier object exactly |

The count falls. The skip reads the candidate's PROC_TABLE row inside the scan, and the verifier then walks fewer states through the rest of the fork arm; on the 2026-11-15 object the same change measured +11,789. `BUDGET-OK 549509 (headroom 350491, ladder net 371527)`. A first form computed the parent's mark once, ahead of the scan, for both uses: with the const off that object read 553,518, not 553,534, so the first-entry code keeps its own `want` and the scan has `own_mark`.

**Proof.** No stock boot reaches either path: nothing runs between a fork's first tick and its finalize (no timer switch under ascend; `TIMER_SWITCH` forced true still fails the build, 2 hits of `ADR-135 I9`). So each path was run on a mutant object that plants a mark in the fork arm, with a throwaway probe, `forkres` (init forks A, A exits and is reaped; init forks B, which pauses; init forks C, which prints its pid). Neither the plants nor the probe are committed; `main.rs` was restored from a copy after each and the count read 549,509 again.

| Run | Object | `forkres` prints |
|---|---|---|
| stock | landed | `a-pid=2`, `c-pid=3`, `b-pid=2`, `done` |
| A1: a mark of slot 7's planted once on slot 1 after A is reaped | landed + plant | `a-pid=2`, `c-pid=4`, `b-pid=3`, `done`: B and C each skip the held slot |
| A2: the same plant | `UPC_FORK_RESERVE` false + plant | `a-pid=2`, `c-pid=3`, `b-pid=2`, `done`: no skip, the first entry overwrites the mark |
| B1: a mark of slot 1's planted on slot 2 at init's first fork | landed + plant | `a-pid=2`, `c-pid=3`, `b-pid=2`, `done`: A's exit cleared it, C lands in slot 2 |
| B2: the same plant | death clear removed + plant | `a-pid=2`, then nothing: `halted=1 num_proc=4 cur_pid=2`, `PC=0x00000000`, `ROM-FAULT CTX: target=0x0 ... cur_pid=2` |

**What B2 shows beyond the death clear.** C skipped the stale mark on slot 2 and took slot 3, so `num_processes` became 4. Slot 2 was never built, and it is below the high-water mark with neither its halted nor its suspended bit set, so the scheduler switched to it and the machine halted at PC 0. A live reservation under preemption has the same shape: A is copying into a never-used slot k, B skips k and finalizes into k+1, and slot k is runnable before A finalizes it. A recycled slot does not have the problem (its halted and reaped bits stay set until finalize revives it). The reservation as D18 describes it is therefore necessary and not sufficient: the scheduler must also leave a reserved row alone. Not designed, not built; I9's assertion message now names it.

- Gate `reserve-after`: `same=12 diff=0 rc=0 doom=1 ascend=549509`; harness ALL GREEN, 63 PASS lines: `hostile_fork: 3 checks`, `forkloop`, `gate_nway`, `gate2`, `getpidloop` (slot 1 reused three times), `getpidmulti` (three live children), the four `ut_*`; `Doom verifier: verified 758199 insns`. `check-ebpf-loads.sh` 41 programs pass; `check-ebpf-insns.sh` PASS, baseline rewritten by `--update` (the ascend row only moved). `cargo test --release --locked --offline` in `cmd/upc-bootctl`: all ok.

| Landed change | What a hostile caller can try | What bounds it | Proof | Harness line |
|---|---|---|---|---|
| Scan skip and death clear (`UPC_FORK_RESERVE`) | nothing from a process: the marks are in PROC_TABLE, BPF's own map, and no register or guest word feeds the test | the row's state word, written only by fork's first entry, finalize and `slot_exited` | mutant runs A1, A2, B1, B2 above; unreachable in a stock boot | `rung_gate UPC_D6_MISS_FAULT hostile_fork`, `forkloop`, `getpidloop`, `getpidmulti` (that nothing regressed; none reaches the new paths) |

