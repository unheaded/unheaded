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

/// Size of a process's user RAM slice (ADR-074 make-MMU-live Gate 2, 2026-05-30).
/// Covers init's data + stack footprint (stack sits near ~5 MiB). 4 MiB-aligned.
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
/// 4 MiB-aligned physical base of the (shared) child slice. Clear of the kernel
/// (<8 MiB), the ramdisk (8–10 MiB), and the per-pid pgd region (~15.7 MiB).
pub const USER_CHILD_SLICE: u32 = 0x0100_0000; // 16 MiB

/// Physical base offset added to a user virtual address for `pid`. pid 0 (init)
/// keeps an identity slice (offset 0) exactly where the loader set it up, so the
/// kernel↔init boot is undisturbed. Children map their VA[0,SLICE) onto a
/// disjoint physical slice.
///
/// NOTE (Phase 3.1): children currently SHARE one physical slice — correct only
/// because they run serially (init forks → waits → child runs+exits → reap →
/// next fork). True N-way isolation (the forkbomb gate) needs a distinct slice
/// per live pid; deferred per ADR-074.
#[inline(always)]
#[allow(dead_code)]
pub fn pid_phys_offset(pid: u8) -> u32 {
    if pid == 0 {
        0
    } else {
        USER_CHILD_SLICE
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
#[allow(dead_code)]
pub const PROC_TABLE_PGD_SLOT: usize = 20;

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
}
