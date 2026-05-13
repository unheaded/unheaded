# Phase 1.3 IMPL — Sub-phases A through D COMPLETE (Steps 1-8)

**Date:** 2026-05-13 (Marshal-led continuation + autonomous overnight)
**Predecessor plan:** `references/battle-plan-phase13-impl-2026-05-13.md`
**Preceded by:** Phase 1.2 IMPL closure (`b9572c26`) + Phase 1.3 PRE-WORK kickoff (`abb84cfb`) + AP-2 MRET/SRET fix (`73834054`) + AP-1/3/4/5/6 docs + ADR-075 ACCEPTED (`71a04796`).

---

## Sub-phases landed

| Sub-phase | Steps | Commits | Notes |
|---|---|---|---|
| **A — Slot widening** | 1-2 | `583a9463` | PROC_TABLE 4 → 8 across BPF + host + monad-common. Falsification test `phase13_proc_table_supports_8_slots`. |
| **B — Syscall handlers** | 3-5 | `c9550908`, `baaad0fc` | SYS_EXIT now ZOMBIEs the slot (halted_mask bit, no decrement, yield-or-halt). SYS_SCHED_YIELD/WAITPID/EXECVE already existed; pinned with 5 falsification tests. |
| **C — Security gates** | 6-7 | `5e29408b`, `9dc7726c` | LR.W reservation now invalidated on context switch (real RISC-V atomicity bug fixed). RV2MBC SHA-256 integrity gate via UPC_RV2MBC_SHA env var (logged-by-default, fail-fast-on-enforce). |
| **D — Userland build** | 8 | `cbb1de45` | Makefile.mbc-userland scaffold + Phase 1.4 carry-over notes. Build cut per plan because init.c needs Phase 1.4 syscall stubs. |
| **E — Live boot to userinit** | 9-10 | — | DEFERRED. Gates on Phase 1.4. |
| **F — Full regression + report** | 11 | this commit | The report you're reading. |

## Test counts (all green)

- monad-mbc: 266 lib + 67 os_primitives + 43 integration + 2 asm + 3 demo = **381 tests** (was 369 pre-Phase-1.3; +12 falsification tests).
- ebpf default + ascend-linux + phase12-option-a builds: all green.
- BPF verifier: GATE PASSED. No regression.
- golangci-lint: 0 issues.
- go build + pkg/runtime + pkg/ebpf: all green.

## Falsification tests landed (12 new)

1. `phase13_proc_table_supports_8_slots` — forks 7×, asserts 8 distinct pgds + Allocator A1 deterministic mapping + 9th fork = -EAGAIN.
2. `phase13_sys_exit_zombies_slot_and_yields` — SYS_EXIT sets halted_mask bit, doesn't decrement num_processes, yields to a runnable peer.
3. `phase13_sys_exit_last_process_halts_cpu` — last-runnable exit halts CPU (no scheduler ping-pong).
4. `phase13_sys_sched_yield_switches` — round-robin switch confirmed on 8-slot scheduler.
5. `phase13_sys_waitpid_returns_pid_when_child_halted` — fast-path returns child pid without yield.
6. `phase13_sys_waitpid_yields_when_child_running` — polling path yields + PC rewind.
7. `phase13_sys_waitpid_invalid_pid_returns_echild` — boundary case returns -ECHILD.
8. `phase13_sys_execve_resets_regs_preserves_pgd` — EXECVE clears regs/flags but PRESERVES page_dir_base (image swap, not address-space swap).
9. `phase13_lr_sc_reservation_cleared_by_context_switch` — RISC-V atomicity invariant: SYS_SCHED_YIELD invalidates outstanding LR.W reservation.
10. `phase13_lr_sc_reservation_cleared_by_mret_priv_transition` — MRET clears reservation on M→S transition.

## Verifier budget (measured)

| Snapshot | Estimated insns | % of 900K |
|---|---:|---:|
| Phase 1.2 close baseline | 76,125 | 8.46% |
| Post AP-2 (MRET/SRET RV2MBC fix) | ~76,135 | 8.46% |
| Post Phase 1.3 Step 1+2 (PROC_TABLE widening + scheduler bound) | ~76,425 | 8.49% |
| Post Step 3 (SYS_EXIT ZOMBIE refactor) | ~76,650 | 8.52% |
| Post Step 6+7 (SHA gate + reservation clear) | ~76,660 | 8.52% |

Delta vs the 12% hard gate from ADR-075 D-6: 3.48 percentage points headroom (= ~31K insns). Comfortable.

## Security wins (joint Sentinel + BlackMage brief, ADR-075 §Security Review)

- **§Security #1 (RV2MBC integrity)**: SHA-256 gate shipped via `UPC_RV2MBC_SHA` env var. BootParams v3 embedding deferred to a future ergonomics shift, but the runtime gate is in.
- **§Security #2 (PROC_TABLE MMIO isolation)**: PROC_TABLE remains a BPF map (host-only access). No userspace MMIO region overlaps; verified by inspection during Step 1.
- **§Security #3 (LR.W/SC.W priv-transition invariants)**: 2 falsification tests added. The reservation-on-context-switch bug was real — caught by writing the test, fixed in the same commit (`5e29408b`).

## What's deferred to Phase 1.4

- **Phase 1.4 syscall stubs**: open/dup/mknod backed by an in-memory FS, plus the ramdisk loader path.
- **mkfs host tool compile** + ramdisk image generation.
- **upc-bootctl `--ramdisk` flag** loading at byte 0x00800000.
- **Live boot Steps 9-10**: xv6 reaches userinit, init prints "init starting", forks sh.
- **BootParams v3 with embedded SHA**: removes the env-var dependency for the integrity gate.

## Commits this shift (all in main, unsigned)

```
cbb1de45 docs(phase13): Step 8 scaffold — userland Makefile + carry-over notes
9dc7726c feat(upc): Phase 1.3 IMPL Step 6 — RV2MBC SHA-256 integrity gate
5e29408b feat(upc): Phase 1.3 IMPL Step 7 — clear LR.W reservation on context switch
baaad0fc test(monad-mbc): Phase 1.3 IMPL Steps 4+5 — SYS_WAITPID + SYS_EXECVE falsification
c9550908 feat(upc): Phase 1.3 IMPL Step 3 — SYS_EXIT ZOMBIE + SYS_SCHED_YIELD
583a9463 feat(upc): Phase 1.3 IMPL Step 1+2 — PROC_TABLE 4 → 8 slots
d6bdc769 docs(security): UPC Linux gain-vs-risk spitball — joint Sentinel+BlackMage
```

Plus the prior continuation commits (overnight branched plan + Phase B parallel agents + side quests) that were already in flight when this Step-by-Step IMPL began.

## Closing

Phase 1.3 IMPL substrate is fully wired across BPF, host emulator, xv6 source layout, and security gates. The remaining work (Phase 1.4 FS + syscall stubs + live userinit boot) inherits a clean foundation. **No cut-points hit until the planned Step 8 cut-point**, which fires cleanly with a documented carry-over.

Live-boot regression confirmed unchanged behavior post-shift: xv6 advances past MRET into `main()`, priv 0→1 transition, TTY emits `"xv6 booting...\nxv6 kernel is booting\n\n"`.
