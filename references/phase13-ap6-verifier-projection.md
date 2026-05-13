# Phase 1.3 AP-6 — BPF Verifier Insn-Budget Projection

**Status:** Projection recorded 2026-05-13. Pre-work for Phase 1.3 (process model).
**Parent:** `references/battle-plan-ascend-linux-2026-05-08.md` Phase 1.3.

## Baseline (post Phase 1.2, post AP-2)

| Metric | Value |
|---|---|
| Latest gate insn count | 76,125 |
| Verifier budget | 900,000 |
| Current utilization | 8.46% |
| Phase 1.3 hard gate | 108,000 (12% of 900K) |

## Per-helper estimates for Phase 1.3 syscall handlers

These are conservative per-helper estimates based on (a) existing handler
shapes in `ebpf/monad-cpu-ebpf/src/main.rs` and (b) the AP-1/AP-3/AP-5
decisions that hold most policy outside BPF.

| Helper | Δ insns (est.) | Notes |
|---|---|---|
| `sys_execve_handler` | +800 | Reads program header, sets up new pgd entries, resets r0..r15. Assumes pre-translated MBC image (no in-BPF ELF parser). If we instead lower ELF in BPF the cost roughly doubles — avoid. |
| `sys_waitpid_handler` | +400 | Per-pid wait queue scan (bounded by MAX_PROCESSES = 8). Marks parent BLOCKED, schedules. |
| `sys_exit_handler` | +300 | PROC_TABLE slot teardown, signal parent (set parent state to RUNNABLE if waiting), pgd zero-fill via existing TLB-flush idiom. |
| `sys_sched_yield_handler` | +100 | Reuses the existing `scheduler_context_switch` block from Phase 1.2; mostly a thin wrapper. |
| `uvmcopy` (host-side, plain RV32) | **0** | Per AP-5: emitted as MBC LD/ST loops by the translator. No new BPF handler. |
| **Phase 1.3 handler subtotal** | **~+1,600** | |

## Combined projection (Phase 1.3 IMPL + AP-1 slot widening)

Per `references/phase13-ap1-slot-count-budget.md`, AP-1 widens `MAX_PROCESSES`
from 4 to 8. Loop amplification at three sites costs **~+300 insns**.

| Stage | Insn count | % of 900K |
|---|---|---|
| Pre-Phase-1.3 baseline | 76,125 | 8.46% |
| + AP-1 slot widening | +300 → 76,425 | 8.49% |
| + Phase 1.3 syscall handlers | +1,600 → 78,025 | **8.67%** |
| Phase 1.3 hard gate (12%) | 108,000 | 12.00% |
| Headroom remaining | 29,975 | 3.33% |

**Verdict:** Phase 1.3 ships well under the hard gate, with substantial
headroom (~30K insns ≈ 3.3 budget percentage points) carried into Phase 1.4
(filesystem) and Phase 1.5 (shell + commands).

## Risk surface

1. **`sys_execve_handler` blows budget if we add an in-BPF ELF parser.** The
   +800 estimate assumes the user image is already in MBC form (translated at
   load time by the same `rv32i_to_mbc` pipeline that produced xv6's image).
   If we slip a parser into BPF, expect +1500-2000 insns. **Mitigation:**
   Phase 1.3 IMPL must commit to the pre-translated image path; document any
   deviation in a Phase 1.3 ADR.
2. **Verifier path amplification on loops.** The slot-iteration sites are
   constant-tripped, so amplification stays linear. Adding any non-constant
   loop in a new handler would risk a non-linear blowup; treat as a code-review
   gate.
3. **MbcCpuState growth.** If Phase 1.3 needs another slot field (beyond the
   index-20 `page_dir_base` from Phase 1.2), each new field costs ~50-100
   verifier insns from extra load/store path work. No new field is currently
   planned.

## Gate procedure (for Phase 1.3 IMPL completion)

1. Run `scripts/bpf-verifier-check.sh` after each handler lands; record delta.
2. Compare cumulative delta to this projection; deviations > +500 trigger an
   ADR-worthy review.
3. Final SHIP gate: total ≤ 108,000 insns (12% of 900K).

## References

- `references/phase13-ap1-slot-count-budget.md` — slot widening cost.
- `references/phase13-ap5-fork-bridge.md` — uvmcopy-stays-in-C decision.
- `scripts/bpf-verifier-check.sh` — instruction count harness.
- `references/phase12-impl-complete-2026-05-13.md` — latest baseline measurement.

---

Free to use. Free to share.
