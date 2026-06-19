// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! ADR-074 Phase 1.2 page-table mapping module.
//!
//! Three feature flags — `phase12-option-a`, `phase12-option-b`,
//! `phase12-option-c` — gate stub functions for the three options
//! enumerated in ADR-074. The pair-call (2026-05-12) chose **Option A**;
//! that's the implementation path. Options B and C remain as no-op
//! stubs both for verifier-budget measurement and as documentation of
//! the paths-not-taken.
//!
//! Per the Architect addendum:
//! - PROC_TABLE widens from `[u32; 20]` to `[u32; 21]` (slot[20] = `page_dir_base`)
//! - Allocator A1: fixed per-pid pgd region at `RAM_MAP[0x00F00000 + pid*0x1000]`
//! - Context switch: save/restore `page_dir_base` + chunked TLB flush

use monad_common::MbcCpuState;

/// Fixed per-pid page-directory base address (Allocator A1).
/// Each pid gets a 4 KiB pgd at this offset. Per `docs/doom/UPC_PAGE_TABLE_LAYOUT.md`.
#[allow(dead_code)] // Used by Phase 1.2 implementation when MMU goes live
pub const PER_PID_PGD_BASE: u32 = 0x00F0_0000;

/// Page-directory size (4 KiB = 1024 entries × 4 bytes).
#[allow(dead_code)]
pub const PGD_SIZE_BYTES: u32 = 4096;

/// Maximum supported processes (matches PROC_TABLE Array<[u32; 21]> max_entries=8).
/// Widened from 4 → 8 in Phase 1.3 IMPL per ADR-075 D-1 to support
/// init + sh + 5 user commands with one slot of test headroom.
#[allow(dead_code)]
pub const MAX_PROCESSES: u32 = 8;

/// Compute the per-pid pgd base address given a pid (Allocator A1).
#[inline(always)]
#[allow(dead_code)]
pub fn pgd_base_for_pid(pid: u8) -> u32 {
    PER_PID_PGD_BASE + (pid as u32) * PGD_SIZE_BYTES
}

/// fork()'s working-set COPY clamp (ADR-074 make-MMU-live Gate 2, 2026-05-30).
/// Covers init's data + stack footprint (stack sits near ~5 MiB, set by
/// exec.c → trapframe->sp = 0x500000-16). NOTE: this is the COPY bound, not the
/// mapped-slice size. The slice MAPPED by the two superpage leaves is 8 MiB
/// (= SLICE_STRIDE); the VA[6,8) MiB tail above this clamp is mapped but
/// un-copied — private to the pid within its own stride, NOT a cross-process
/// leak (Gate 4). 4 MiB-aligned.
pub const USER_SLICE_BYTES: u32 = 0x0060_0000; // 6 MiB

/// fork() working-set copy bounds (perf, 2026-05-31). The full 6 MiB slice is
/// almost all zeros: the user image (.text/.rodata/.data/.bss) sits in the low
/// few KiB (heap is untracked — program_break reads 0) and the live stack sits
/// near the top (SP ≈ 5 MiB). Copying the whole slice costs ~98K ticks/fork at
/// 16 words/tick; copying just these two bands costs ~5K. fork() copies
/// [0, FORK_COPY_LOW_BYTES) for the image+heap and a FORK_COPY_STACK_BYTES
/// window around the (page-aligned) stack pointer for the live stack.
/// Assumes user image+heap < 256 KiB and stack depth < 64 KiB at fork time —
/// true for init / sh / gate2; revisit (or fall back to full-slice) for a
/// malloc-heavy or deep-recursion userland.
pub const FORK_COPY_LOW_BYTES: u32 = 0x0004_0000; // 256 KiB: image + heap
pub const FORK_COPY_STACK_BYTES: u32 = 0x0001_0000; // 64 KiB: live stack window
/// 4 MiB-aligned physical base of the FIRST child slice (pid 1). Clear of the
/// kernel (<8 MiB), the ramdisk (8–10 MiB), and the per-pid pgd region (~15.7 MiB).
pub const USER_CHILD_SLICE: u32 = 0x0100_0000; // 16 MiB

/// Per-pid physical slice stride (Gate 4 — N-way concurrent isolation, 2026-05-31).
/// Each live pid owns a DISJOINT physical slice this far apart. Must be ≥ two
/// 4 MiB superpages (= 8 MiB), because each pid's VA[0,8 MiB) is mapped by two
/// superpage leaves: a stride < 8 MiB makes pid N's upper leaf OVERLAP pid N+1's
/// lower leaf — the exact cross-process leak gate_nway.c falsifies (BlackMage
/// leak primitive #1). const-asserted below.
pub const SLICE_STRIDE: u32 = 0x0080_0000; // 8 MiB (two 4 MiB superpages)

/// Highest physical address a child slice may occupy inside the 64 MiB RAM_MAP
/// (ADR-074 scoped N-way, 2026-05-31). A fork whose slice top would exceed this
/// bails (-1) rather than mapping an out-of-bounds / overlapping slice — a
/// forkbomb then fails gracefully on physical exhaustion. 56 MiB admits pids
/// 1..=5 and leaves 8 MiB headroom below RAM_MAP's 64 MiB top.
pub const RAM_ISOLATION_CEILING: u32 = 0x0380_0000; // 56 MiB

// ── Gate 4 isolation invariants (compile-time) ─────────────────────────────
/// The stride must cover the two 4 MiB superpage leaves, or adjacent pids'
/// slices overlap physically (the leak gate_nway.c exists to catch).
const _: () = assert!(
    SLICE_STRIDE >= 2 * 0x0040_0000,
    "SLICE_STRIDE < two 4 MiB superpages → adjacent pid leaf OVERLAP"
);
/// At least 4 concurrent children (pid 1..=4) must fit under the ceiling so the
/// scoped 3–4-way gate has headroom. pid 4's slice top = base + 3*stride + stride.
const _: () = assert!(
    USER_CHILD_SLICE + 3 * SLICE_STRIDE + SLICE_STRIDE <= RAM_ISOLATION_CEILING,
    "≥4 concurrent child slices do not fit under RAM_ISOLATION_CEILING"
);

/// Physical base offset added to a user virtual address for `pid`.
///
/// pid 0 (init) keeps an identity slice (offset 0) exactly where the loader set
/// it up, so the kernel↔init boot is undisturbed and Gate 2/3 (which key pid 1
/// at 16 MiB) stay intact.
///
/// Children (pid ≥ 1) each map their VA[0,8 MiB) onto a DISJOINT physical slice
/// `USER_CHILD_SLICE + (pid-1)*SLICE_STRIDE`: pid1=16, pid2=24, pid3=32,
/// pid4=40, pid5=48 MiB. With SLICE_STRIDE = 8 MiB the slices never overlap, so
/// per-pid pgds isolate CONCURRENT live processes — the property gate_nway.c
/// proves (Gate 4, 2026-05-31). SYS_fork's capacity guard refuses a slice whose
/// top would exceed RAM_ISOLATION_CEILING, so a forkbomb fails gracefully.
#[inline(always)]
#[allow(dead_code)]
pub fn pid_phys_offset(pid: u8) -> u32 {
    if pid == 0 {
        0
    } else {
        USER_CHILD_SLICE + (pid as u32 - 1) * SLICE_STRIDE
    }
}

/// PROC_TABLE slot index for `page_dir_base` (Option A widening).
///
/// Slot layout per Architect addendum:
/// - slot[0..16]  → r0..r15
/// - slot[16]     → PC
/// - slot[17]     → flags
/// - slot[18]     → SP_copy
/// - slot[19]     → program_break
/// - slot[20]     → **page_dir_base** ← Option A addition
/// - slot[21]     → **rv2mbc_base** ← Phase 1.7 Gate B (ADR-077)
#[allow(dead_code)]
pub const PROC_TABLE_PGD_SLOT: usize = 20;

/// PROC_TABLE slot holding the per-process RV2MBC base (ADR-077 Gate B). A
/// context switch save/restores it next to `page_dir_base` so a descheduled
/// program (e.g. sh) keeps resolving its indirect branches into its own
/// disjoint RV2MBC_MAP region. Grows the row from `[u32;21]` to `[u32;22]`.
#[allow(dead_code)]
pub const PROC_TABLE_RV2MBC_SLOT: usize = 21;

// ───────────────────────────────────────────────────────────────────────
// Option A — per-task pgd (CHOSEN)
// ───────────────────────────────────────────────────────────────────────

#[cfg(feature = "phase12-option-a")]
#[inline(always)]
pub fn phase12_option_a_on_context_switch(_cpu: &mut MbcCpuState, _new_pid: u8) {
    // Real implementation lands in the post-pre-work phase. The contract:
    //   1. Save current cpu.page_dir_base into PROC_TABLE[current_pid][20]
    //   2. Load next  cpu.page_dir_base from PROC_TABLE[new_pid][20]
    //      (or compute via pgd_base_for_pid(new_pid) for fixed-region allocator)
    //   3. Trigger SYS_FLUSH_TLB-equivalent (chunked 8/tick — already implemented)
    //   4. Update cpu.current_pid = new_pid
    //
    // Stub for verifier measurement; implementation lands when xv6 SYS_SCHED_YIELD
    // path is wired to call this hook (Phase 1.2 Step N+).
}

// ───────────────────────────────────────────────────────────────────────
// Option B — ASID-tagged shared pgd (NOT CHOSEN; stub for measurement)
// ───────────────────────────────────────────────────────────────────────

#[cfg(feature = "phase12-option-b")]
#[inline(always)]
pub fn phase12_option_b_on_context_switch(_cpu: &mut MbcCpuState, _new_asid: u8) {
    // Path-not-taken per ADR-074 pair-call. Documented for completeness.
    // Would have required:
    //   1. Add cpu.current_asid: u8 (1 new field on MbcCpuState — wire-format-compatible default 0)
    //   2. Widen TLB_MAP from Array<[u32; 3]> to Array<[u32; 4]> (ASID column)
    //   3. Update translate_address() TLB lookup to filter by ASID
    //   4. Context switch becomes a 1-byte write (no flush)
}

// ───────────────────────────────────────────────────────────────────────
// Option C — per-task pgd + pid-tagged TLB (NOT CHOSEN; future extension)
// ───────────────────────────────────────────────────────────────────────

#[cfg(feature = "phase12-option-c")]
#[inline(always)]
pub fn phase12_option_c_on_context_switch(_cpu: &mut MbcCpuState, _new_pid: u8) {
    // Path-not-taken per ADR-074 pair-call. Can be added LATER as a
    // non-breaking extension on top of Option A if defense-in-depth is
    // needed. Would have required:
    //   1. Option A's full path (pgd swap + flush)
    //   2. Plus widen TLB_MAP to include pid column
    //   3. Plus update translate_address() to filter by pid (belt-and-suspenders)
}

// ───────────────────────────────────────────────────────────────────────
// Default (no Phase 1.2 feature) — pre-pair-call state
// ───────────────────────────────────────────────────────────────────────

#[cfg(not(any(
    feature = "phase12-option-a",
    feature = "phase12-option-b",
    feature = "phase12-option-c"
)))]
#[inline(always)]
#[allow(dead_code)]
pub fn phase12_noop_on_context_switch(_cpu: &mut MbcCpuState, _new_pid: u8) {
    // Default: no Phase 1.2 active. xv6 boots in flat-mode (mmu_enabled=0).
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn pgd_base_for_pid0_is_per_pid_base() {
        assert_eq!(pgd_base_for_pid(0), PER_PID_PGD_BASE);
    }

    #[test]
    fn pgd_base_for_pid_is_4kib_aligned() {
        for pid in 0..(MAX_PROCESSES as u8) {
            let base = pgd_base_for_pid(pid);
            assert_eq!(
                base & 0xFFF,
                0,
                "pgd base 0x{:08X} for pid {} is not 4 KiB aligned",
                base,
                pid
            );
        }
    }

    #[test]
    fn pgd_regions_dont_overlap() {
        for i in 0..(MAX_PROCESSES as u8) {
            for j in 0..(MAX_PROCESSES as u8) {
                if i == j {
                    continue;
                }
                let bi = pgd_base_for_pid(i);
                let bj = pgd_base_for_pid(j);
                let gap = if bi > bj { bi - bj } else { bj - bi };
                assert!(
                    gap >= PGD_SIZE_BYTES,
                    "pid {} (0x{:08X}) and pid {} (0x{:08X}) regions overlap or touch",
                    i,
                    bi,
                    j,
                    bj
                );
            }
        }
    }

    #[test]
    fn proc_table_pgd_slot_is_20() {
        // The Architect's addendum specifies slot[20] = page_dir_base.
        // This test guards against accidental renumbering during implementation.
        assert_eq!(PROC_TABLE_PGD_SLOT, 20);
    }

    #[test]
    fn fixed_region_fits_in_first_16kib_above_base() {
        // Allocator A1: 4 pids × 4 KiB = 16 KiB total reserved.
        let last_pid_top = pgd_base_for_pid((MAX_PROCESSES - 1) as u8) + PGD_SIZE_BYTES;
        let total_used = last_pid_top - PER_PID_PGD_BASE;
        assert_eq!(total_used, MAX_PROCESSES * PGD_SIZE_BYTES);
        assert_eq!(total_used, 16 * 1024);
    }

    // ── Gate 4 — N-way concurrent isolation (2026-05-31) ───────────────────

    #[test]
    fn pid0_phys_offset_is_identity() {
        assert_eq!(pid_phys_offset(0), 0);
    }

    #[test]
    fn pid1_base_unchanged_at_16mib() {
        // Gate 2/3 key pid 1 at 16 MiB; the N-way generalization must not move
        // it (init↔pid1 boot + gate2 regression depend on it).
        assert_eq!(pid_phys_offset(1), USER_CHILD_SLICE);
        assert_eq!(USER_CHILD_SLICE, 0x0100_0000);
    }

    #[test]
    fn child_slices_are_disjoint() {
        // Each live pid's 8 MiB-mapped slice [off, off+SLICE_STRIDE) must not
        // overlap any other's — the property gate_nway.c proves at runtime.
        for i in 1u8..=5 {
            for j in 1u8..=5 {
                if i == j {
                    continue;
                }
                let (a, b) = (pid_phys_offset(i), pid_phys_offset(j));
                let (lo, hi) = if a < b { (a, b) } else { (b, a) };
                assert!(
                    hi - lo >= SLICE_STRIDE,
                    "pid {} (0x{:08X}) and pid {} (0x{:08X}) slices overlap",
                    i,
                    a,
                    j,
                    b
                );
            }
        }
    }

    #[test]
    fn stride_covers_two_superpages() {
        // < two 4 MiB superpages → adjacent pid leaf overlap (leak primitive #1).
        assert!(SLICE_STRIDE >= 2 * 0x0040_0000);
        assert_eq!(SLICE_STRIDE, 0x0080_0000);
    }

    #[test]
    fn four_children_fit_under_ceiling() {
        // pids 1..=4 must fit; the SYS_fork capacity guard bails above this.
        for pid in 1u8..=4 {
            let top = pid_phys_offset(pid) + SLICE_STRIDE;
            assert!(
                top <= RAM_ISOLATION_CEILING,
                "pid {} slice top 0x{:08X} exceeds ceiling 0x{:08X}",
                pid,
                top,
                RAM_ISOLATION_CEILING
            );
        }
    }

    #[test]
    fn sixth_child_exhausts_the_budget() {
        // The forkbomb gate: a 6th concurrent child (pid 6) must NOT fit — its
        // slice top exceeds the ceiling, so SYS_fork returns -1 (graceful).
        let top = pid_phys_offset(6) + SLICE_STRIDE;
        assert!(top > RAM_ISOLATION_CEILING);
    }
}
