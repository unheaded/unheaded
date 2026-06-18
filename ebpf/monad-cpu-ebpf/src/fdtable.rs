// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Phase 1.7 per-process file-descriptor table (ASCEND-LINUX, Gate A).
//!
//! xv6's `init` opens the console (`open("console", O_RDWR)` → fd 0) then
//! `dup`s it onto fds 1 and 2 (stdout/stderr); every later `sh` and command
//! inherits those three across `fork`/`exec`. Before Phase 1.7 the xv6 ecall
//! path had no `open`/`dup`/`close`, so those syscalls fell through the
//! "unknown syscall → silently ignore" arm (`main.rs`) and returned whatever
//! happened to be in a0 — init "succeeded" only by accident (a non-negative
//! path pointer looked like a valid fd). That accident cannot carry a real
//! `read(0, …)` from `sh`, which needs fd 0 to actually route to console-in.
//!
//! This module owns the *pure* index math and free-slot policy so it is unit
//! testable off-target; the BPF side (`FD_TABLE` map scan, console routing)
//! lives in `main.rs` and calls [`fd_slot`] / mirrors [`lowest_free`].
//!
//! Storage model: a flat `Array<u32>` of `FD_TABLE_LEN` entries, one row of
//! `NOFILE` descriptors per pid. Each entry holds a *kind* tag, not a real
//! file object — Gate A only needs to distinguish "console" from "closed".
//! Real file descriptors (inode-backed, with an offset) arrive with the
//! Phase 1.7 filesystem reader; the kind tag leaves room for an `FD_INODE`
//! base above [`FD_CONSOLE`] without changing the table shape.

/// Open files per process (matches xv6 `kernel/param.h` `NOFILE`).
pub const NOFILE: u32 = 16;

/// Process slots (matches `PROC_TABLE` `max_entries` and `phase12::MAX_PROCESSES`).
pub const MAX_PROCESSES: u32 = 8;

/// `FD_TABLE` map length: one `NOFILE`-wide row per pid.
pub const FD_TABLE_LEN: u32 = NOFILE * MAX_PROCESSES; // 128

// ── Descriptor kinds (values stored in FD_TABLE) ───────────────────────────
/// Free (closed) descriptor slot. Zero so a fresh (zeroed) map starts empty.
pub const FD_FREE: u32 = 0;
/// Console device: read → KBD_MAP/stdin queue, write → TTY MMIO 0xC001.
pub const FD_CONSOLE: u32 = 1;

/// Flat `FD_TABLE` index for (`pid`, `fd`). The caller guarantees
/// `fd < NOFILE` and `pid < MAX_PROCESSES`; out-of-range inputs would index a
/// neighbouring row, so the BPF side bounds-checks before calling.
#[inline(always)]
#[allow(dead_code)]
pub fn fd_slot(pid: u8, fd: u32) -> u32 {
    (pid as u32) * NOFILE + fd
}

/// Lowest free descriptor in `row`, or `None` if all `NOFILE` are in use.
///
/// xv6 semantics: `open`/`dup` return the *lowest-numbered* unused fd, so
/// init's first `open("console")` yields fd 0, then `dup(0)`→1, `dup(0)`→2.
/// The BPF side mirrors this scan over the live `FD_TABLE` map; this pure
/// form exists so the policy is unit tested without a kernel.
#[inline(always)]
#[allow(dead_code)]
pub fn lowest_free(row: &[u32; NOFILE as usize]) -> Option<u8> {
    let mut fd = 0u32;
    while fd < NOFILE {
        if row[fd as usize] == FD_FREE {
            return Some(fd as u8);
        }
        fd += 1;
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn fd_table_len_is_128() {
        assert_eq!(FD_TABLE_LEN, 128);
        // Must hold init + sh + 5 cmds + headroom, NOFILE each.
        assert_eq!(FD_TABLE_LEN, NOFILE * MAX_PROCESSES);
    }

    #[test]
    fn slot_rows_are_disjoint_and_contiguous() {
        // Adjacent pids' rows must not overlap and must tile the table.
        for pid in 0..(MAX_PROCESSES as u8) {
            let base = fd_slot(pid, 0);
            assert_eq!(base, (pid as u32) * NOFILE);
            let top = fd_slot(pid, NOFILE - 1);
            assert_eq!(top, base + NOFILE - 1);
            assert!(top < FD_TABLE_LEN, "pid {pid} row overruns the table");
            if pid > 0 {
                // No overlap with the previous row.
                assert_eq!(base, fd_slot(pid - 1, NOFILE - 1) + 1);
            }
        }
    }

    #[test]
    fn lowest_free_picks_fd0_on_empty_row() {
        let row = [FD_FREE; NOFILE as usize];
        assert_eq!(lowest_free(&row), Some(0));
    }

    #[test]
    fn lowest_free_matches_xv6_init_sequence() {
        // open→0, dup(0)→1, dup(0)→2, mirroring init's console setup.
        let mut row = [FD_FREE; NOFILE as usize];
        let a = lowest_free(&row).unwrap();
        row[a as usize] = FD_CONSOLE;
        assert_eq!(a, 0);
        let b = lowest_free(&row).unwrap();
        row[b as usize] = FD_CONSOLE;
        assert_eq!(b, 1);
        let c = lowest_free(&row).unwrap();
        assert_eq!(c, 2);
    }

    #[test]
    fn lowest_free_reuses_a_closed_hole() {
        // Closing fd 1 must let the next open reclaim it (not jump to 3).
        let mut row = [FD_CONSOLE; NOFILE as usize];
        row[1] = FD_FREE;
        assert_eq!(lowest_free(&row), Some(1));
    }

    #[test]
    fn lowest_free_none_when_full() {
        let row = [FD_CONSOLE; NOFILE as usize];
        assert_eq!(lowest_free(&row), None);
    }
}
