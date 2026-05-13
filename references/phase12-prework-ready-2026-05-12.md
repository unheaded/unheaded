# Phase 1.2 Pair-Call — LANDED + Pre-work CLOSED

**Date**: 2026-05-12
**Status**: ALL 6 AP-* items closed. Pair-call landed. Implementation arc cleared.

## Pair-call outcome (recorded in ADR-074 §"Decision — 2026-05-12 pair-call")

- **Option A — per-task pgd** (Scientist + Architect pick) — RATIFIED
- **Allocator A1 — Fixed per-pid region** at `RAM_MAP[0x00F00000 + pid*0x1000]` — RATIFIED
- **Marshal executes pre-work + implementation autonomously** per Track C "slow + safe" — RATIFIED
- No dissents. Decision unanimous on the call.

## Evidence on the table

| Item | Status | Artifact |
|------|--------|----------|
| AP-1 Architect addendum | ✅ | `docs/adr/ADR-074-phase12-page-table-model.md` (§"Architect review addendum" + "Decision — 2026-05-12 pair-call") |
| AP-2 Verifier-budget baseline | ✅ | `references/phase12-evidence/bpf-budget-baseline-2026-05-12.txt` |
| AP-3 Feature-flag scaffold | ✅ | `ebpf/monad-cpu-ebpf/src/phase12.rs` + Cargo.toml features (commit `4c966cfa`) |
| AP-4 Page-table layout doc | ✅ | `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` (commit `3c36225c`) |
| AP-5 Verifier cost analysis | ✅ | `references/phase12-evidence/ap5-analysis-2026-05-12.md` (commit `0e90b09c`) — measured 0-byte delta across all 3 stub variants |
| AP-6 DISTRIBUTED follow-on flag | ✅ | `references/battle-plan-ascend-linux-2026-05-08.md` Phase 3.1 §note (commit `01298258`) |

## Phase 7 readiness gate evidence (2026-05-13 03:22 UTC)

| Gate | Result |
|------|--------|
| `golangci-lint run ./...` | 0 issues (ADR-073 ratchet HOLDS) |
| `go build ./...` | OK |
| `go test -short ./...` | 243/243 packages PASS, 0 FAIL |
| `cargo build -p monad-cpu-ebpf --features phase12-option-{a,b,c}` | All 3 GREEN |
| `bash scripts/bpf-verifier-check.sh` | PASSED, monad-cpu-ebpf 75865 insns / 8% of 900K budget — no delta vs AP-2 baseline |

## Implementation handoff — Option A arc (task #76)

The Phase 1.2 IMPL arc cleared by this pre-work:

1. Widen `PROC_TABLE` from `Array<[u32; 20]>` to `Array<[u32; 21]>`; add slot[20] = `page_dir_base`.
2. Replace the `phase12_option_a_on_context_switch` stub in `ebpf/monad-cpu-ebpf/src/phase12.rs` with real save/restore logic.
3. Wire SYS_SCHED_YIELD path to call the hook.
4. Patch xv6 `kernel/proc.c::proc_pagetable()` to write into `PROC_TABLE[pid][20]`.
5. Patch xv6 `swtch_mbc.S` (or equivalent) to save+restore `page_dir_base`.
6. Patch xv6 `kernel/vm.c::kvmcreate()` to allocate from `pgd_base_for_pid(pid)`.
7. Add forkbomb test at `crates/xv6-mbc/tests/forkbomb_test.rs` — Phase 3.1 hard gate (run early as smoke).
8. Run the ADR-074 Option A falsification experiment: PID 1 maps page, switches to PID 2, verifies isolation.

## Phase 3 follow-on docket (forwarded from ADR-074 Decision)

- Wotan DISTRIBUTED handle to replace physical `page_dir_base` (flagged in ASCEND-LINUX battle plan Phase 3.1, AP-6).
- Option C as future hybrid extension if defense-in-depth need surfaces.
- Allocator A2/A3 if process count exceeds the current 4-slot cap.

## Closing

This closes the Phase 1.2 pre-work battle plan at `references/battle-plan-phase12-prework-2026-05-11.md`. Implementation arc tracked at task #76.
