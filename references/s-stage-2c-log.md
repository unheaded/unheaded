# S-STAGE-2C log

Started 2026-11-15 on 18ec23a8 (branch sprint/s-stage-2c).
Plan: docs/battle-plans/S-STAGE-2C-battle-plan.md

## Ledger (verified insns, ascend)

| Change | OFF | ON | delta | net vs 921,036 | gate |
|---|---|---|---|---|---|
| priv3 + D6 | 889941 | 893507 | +3566 | 27529 | d6-after |

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
