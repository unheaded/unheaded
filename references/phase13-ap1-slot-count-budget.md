# Phase 1.3 AP-1 — Slot Count Budget

**Status:** Decision recorded 2026-05-13. Pre-work for Phase 1.3 (process model).
**Parent:** `references/battle-plan-ascend-linux-2026-05-08.md` Phase 1.3.

## Current state

`PROC_TABLE` is `Array<[u32; 21]>` with `max_entries = 4` — defined at
`ebpf/monad-cpu-ebpf/src/main.rs:160-168` (widened from `[u32; 20]` in Phase 1.2
to carry `page_dir_base` at slot index 20). The cap `MAX_PROCESSES = 4` lives at
`ebpf/monad-cpu-ebpf/src/phase12.rs:29-31` and is referenced by every iteration
site.

Three iteration sites in `main.rs` walk the full slot range, each performing 16
register copies (r0..r15) per slot per pass:

- `main.rs:1037-1050` — `SYS_FORK` slot allocation scan.
- `main.rs:1737-1740` — context-save register flush.
- `main.rs:1790-1793` — context-restore register fill.

## Verifier baseline (post Phase 1.2, pre Phase 1.3)

| Metric | Value |
|---|---|
| Insn count (latest gate) | 76,125 |
| Budget ceiling | 900,000 |
| Utilization | 8.46% |
| Hard gate (Phase 1.3 ship) | 108,000 (12% of 900K) |

## Slot count options

| Slots | Δ insns vs baseline (est.) | New util % | Headroom for Phase 1.3 |
|---|---|---|---|
| 4 (status quo) | 0 | 8.46% | full |
| **8 (recommended)** | **~+300** | **~8.49%** | **full — Phase 1.3 fits comfortably** |
| 16 (max) | ~+650 | ~8.53% | full — Phase 1.4+ headroom |

The 300/650-insn deltas come from the three iteration sites unrolling against a
larger fixed bound. Each additional slot adds roughly 16 × 3 = 48 reg-copy
checks across the three loops, plus per-slot bookkeeping. Verifier path
amplification stays bounded because the loops are constant-tripped.

## Recommendation

**Adopt 8 slots.** It is the minimum that unblocks Phase 1.3's expected workload
(init + shell + 1-2 user processes + reserved kernel pids) without forcing a
second widening mid-phase. Going straight to 16 is tempting for headroom but
costs +350 insns we do not need to spend now, and slot count is cheap to widen
later because all three loop sites are bounded by `MAX_PROCESSES`.

## Action items (for Phase 1.3 IMPL)

1. `MAX_PROCESSES = 4` → `MAX_PROCESSES = 8` in
   `ebpf/monad-cpu-ebpf/src/phase12.rs`.
2. `PROC_TABLE` `max_entries = 4` → `max_entries = 8` at
   `ebpf/monad-cpu-ebpf/src/main.rs:160-168`.
3. Re-run BPF verifier gate; confirm ≤ 12% of 900K.
4. Confirm `pgd_base_for_pid` math still fits within the 16 KiB Phase-1.2
   region — at 8 slots it grows to `0x00F00000..0x00F07FFF` (32 KiB), still
   inside the reserved `0x00F00000..0x00F0FFFF` window.

---

Free to use. Free to share.
