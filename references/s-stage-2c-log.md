# S-STAGE-2C log

Started 2026-11-15 on 18ec23a8 (branch sprint/s-stage-2c).
Plan: docs/battle-plans/S-STAGE-2C-battle-plan.md

## Ledger (verified insns, ascend)

| Change | OFF | ON | delta | net vs 921,036 | gate |
|---|---|---|---|---|---|
| priv3 + D6 | 889941 | 893507 | +3566 | 27529 | d6-after |
| bounded paths | 893507 | 895768 | +2261 | 25268 | paths-after |
| atomics: scan only (translate stuck) | 895768 | 895768 | +0 | 25268 | atomics-after |

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
