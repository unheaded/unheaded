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

