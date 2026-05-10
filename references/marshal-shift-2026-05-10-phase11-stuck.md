# Marshal Shift Report — Phase 1.1 SHIP attempt — 2026-05-10

**Plan**: `references/battle-plan-phase11-ship-2026-05-10.md` (commit `dedda39e`)
**Owner**: Marshal (lead), Developer (implementation)
**Result**: SHIP GATE NOT MET. Phase 4 BLOCKED on pre-existing BPF verifier rejection. Phases 0-3 + Phase 5 PARTIAL completed clean.
**Skip Protocol**: Activated at Step 65. Forward progress continued through Phase 5 partial; Phases 6-7 unreachable until verifier-reject is resolved.

---

## STUCK REPORT

**Progress**: 49/104 steps completed (47%); 7 steps STUCK or BLOCKED downstream of STUCK.

**Stuck step:**

### Step 65 — FIRST BOOT ATTEMPT
- **Symptom**: kernel 6.17.0-22-generic BPF verifier rejected `monad-cpu-ebpf` at `Ebpf::load_file()`:
  ```
  infinite loop detected at insn 344
  verification time 162401 usec
  processed 41001 insns (limit 1000000) max_states_per_insn 16
  total_states 1263 peak_states 964 mark_read 140
  Invalid argument (os error 22)
  ```
- **Attempted**: 1) full upc-bootctl boot path → load fails. 2) doom-runner sanity check (binary works for `layout` subcommand; full `run` requires doom.mbc + doom.elf + WAD which aren't present on this host, so isolation incomplete).
- **Time burned**: ~5 min before Skip Protocol fired (well within 3× budget).
- **Class**: pre-existing — same family as parking-lot `P-LOT-3` (BPF verifier-budget revalidation) and decision query `Q3` from `references/session-summary-2026-05-10.md` (EBPF-CLIPPY-119 verifier-budget gate). Not introduced by this sprint. The Phase 0 static check `bpf-verifier-check.sh` passed but does NOT exercise per-instruction loop-bound inference.
- **Downstream impact**: Steps 66-72 (Phase 4 GATE), Steps 78-86 (Phase 5 GATE banner), all of Phases 6 + 7. Total blocked: ~55 steps.
- **Suggested fix**: Computermancer + BlackMage required. Three options:
  1. **Annotate the loop bound** in monad-cpu-ebpf so the verifier can prove termination (smallest scope; ~0.5 day if the loop is one of the documented dispatch loops).
  2. **Restructure the per-PC dispatch as a tail-call chain** (doom-runner already does this — `tail_calls::setup_tail_calls`). Lift that pattern into the load path (~1 day).
  3. **Split monad-cpu-ebpf into smaller XDP entries** so each entry has a smaller per-program instruction count (~2 days, larger refactor).
- **Estimate**: 0.5-2 days expert eBPF work + iterative load testing on kernel 6.17.

### Steps 66-72 — Phase 4 remainder
- **Status**: BLOCKED by Step 65.
- **Description**: trigger packet, observe PC advance, Phase 4 EXIT GATE.
- **Reachable when**: Step 65 resolved.

### Steps 78-86 — Phase 5 remainder + GATE
- **Status**: BLOCKED by Step 65.
- **Description**: Tokio TTY poller in upc-bootctl, browser banner moment, Phase 5 EXIT GATE.
- **Reachable when**: Step 65 resolved AND eBPF actually ticks the CPU through start_mbc.c::mmio_puts("xv6 booting...\n").

### Phases 6-7 (Steps 87-104)
- **Status**: BLOCKED by Step 65.
- **Description**: Halt cleanup, soak, GATE eval, doc propagation.
- **Reachable when**: Phases 4+5 GATE pass.

---

## Recommended intervention order

1. **Resolve Step 65 first** — unblocks 55 downstream steps (entire ship gate).
2. After resolution, re-run from Step 66 (trigger packet). Phases 0-3 + Phase 5 partial don't need redo.
3. Phase 6 + 7 should run cleanly once Phase 4+5 gates pass.

---

## What WAS completed (commits this session)

| Phase | Step Range | Status | Commit | Notes |
|-------|------------|--------|--------|-------|
| 0 PREFLIGHT | 1-12 | ✓ PASS | `9a9a4cdb` | Baseline pristine. H1 path patched (build/ → upstream/target/). |
| 1 STAGE-1 DECISION | 13-22 | ✓ PASS | `34b29872` | 3 [DECIDE] resolved + documented. Rationale corrected (xv6 DOES have .bss; start_mbc.c is the de-facto stage-1). |
| 2 LIFT AYA PATTERN | 23-42 | ✓ PASS | `d6cd5d2b` | BootRunner with 7 methods + xv6_initial_cpu_state(). 11 unit tests pass. |
| 3 BOOTPARAMS V2 | 43-58 | ✓ PASS | `7fc0134f` + `028556e9` | BootParamsV2 (256B exact) + 4 tests. cmd_boot live-path scaffolding. |
| 4 FIRST BOOT | 59-65 | ⚠ STUCK | `c730b74e` | netns.rs added, XDP attach wired, **Step 65 verifier-rejected**. |
| 5 TTY PIPELINE (PARTIAL) | 73-77 | ✓ PASS | `9197ec83` | /api/v1/tty/ingest endpoint + 5 tests. Steps 78-86 blocked. |
| 6 HALT CLEANUP | 87-94 | — BLOCKED | — | Unreachable. |
| 7 GATE EVAL | 95-104 | — BLOCKED | — | Unreachable. |

**Total commits this Marshal shift**: 7 (`9a9a4cdb` `34b29872` `d6cd5d2b` `7fc0134f` `028556e9` `c730b74e` `9197ec83`).

---

## Real bug surfaced

**Phase 0 verifier-budget gate is misleading.** `scripts/bpf-verifier-check.sh` PASSED in Step 8 (recorded "GATE: PASSED, 2 warnings, 0 failures") but the kernel's actual load-time verifier rejects the program ~5 seconds later. Anyone reading the static check would believe the program is loadable.

**Recommended action** (parking-lot P-LOT-3 promotion): Augment `bpf-verifier-check.sh` with an actual `Ebpf::load_file()` attempt against the running kernel. Or split the script into "static" (current behavior) and "load" (real attempt) and require both for Phase N gates.

---

## Plan amendments made in flight

1. `references/battle-plan-phase11-ship-2026-05-10.md` VARIABLES section: KERNEL_MBC path corrected from `crates/xv6-mbc/build/xv6-mbc.mbc` to `crates/xv6-mbc/upstream/target/xv6-mbc.mbc`.
2. Step 16 [DECIDE] rationale corrected: xv6 DOES have .bss; `start_mbc.c` IS the de-facto stage-1 because `kernel-mbc.ld` places it in the `.stage1` region. Banner is reachable without solving BSS zero-fill.
3. Steps 65-72 marked STUCK / BLOCKED in the plan with full debug context.

---

## Regression baseline

Plan steps 95-96 (full Go + Rust regression sweep) were Phase 7 and are BLOCKED. **Spot check**: 7 commits this session are all green-builds (each phase commit ran build + test). No new failures introduced.

---

## Marshal sign-off

Phase 1.1 SHIP GATE not reached. Phase 4 BPF verifier rejection is a pre-existing architectural blocker (parking-lot P-LOT-3, decision query Q3). Resolution requires Computermancer + BlackMage expert work outside the Marshal lane.

**Handoff**:
- Stevie: decide whether to invest 0.5-2 days on the verifier fix now (Q3 promotion to active sprint), OR defer and ship Phase 1.1 against a different kernel where the program loads. WEST/EAST host trial may unblock immediately if their kernels are older.
- Computermancer: own the loop-bound annotation OR tail-call-chain restructure decision.
- BlackMage: pen-test the chosen path (loop-bound annotations are a class of bug that can be exploited).
- Next Round Table: add an emergency item — "ship gate blocked on pre-existing verifier issue; need triage decision."

7/8 Phases attempted, 5/8 Phases completed (incl. partial), 0 regressions, 7 commits clean. The plan worked; the kernel didn't.

Marshal off-duty.
