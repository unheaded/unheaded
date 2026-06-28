// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Phase 1.7 per-process file-descriptor table — pure index math & policy.
//!
//! xv6's `init` opens the console (`open("console", O_RDWR)` → fd 0) then
//! `dup`s it onto fds 1 and 2 (stdout/stderr); every later `sh` and command
//! inherits those three across `fork`/`exec`. The ASCEND ecall path
//! (`monad-cpu-ebpf::main`) owns the live BPF maps (`FD_TABLE`, `FD_INODE_MAP`);
//! this module owns the *pure* parts — descriptor-kind constants, the
//! `pid*NOFILE+fd` slot math, the lowest-free policy, and an off-target model
//! of the per-fd inode state — so they are unit-testable without a kernel.
//!
//! This lives in `monad-common` (not the ebpf bin crate) because the bin
//! crate's own `cargo test` is poisoned by `ebpf/.cargo/config.toml`'s
//! `build-std`; the working off-target harness is `cargo test -p monad-common`
//! (see the Gate D battle plan's Phase 1 test-home correction). The BPF side
//! re-exports these (`mod fdtable { pub use monad_common::fdtable::*; }`) so its
//! `fdtable::*` call sites are unchanged.
//!
//! ## Storage model
//!
//! Two flat per-pid tables, one `NOFILE`-wide row per pid, both indexed by
//! [`fd_slot`]:
//! - `FD_TABLE: Array<u32>` — a *kind* tag per descriptor
//!   ([`FD_FREE`] / [`FD_CONSOLE`] / [`FD_INODE`]).
//! - `FD_INODE_MAP: Array<[u32; 2]>` — `{inum, offset}` for [`FD_INODE`]
//!   descriptors. The kind tag in `FD_TABLE` says *which* of the two stores is
//!   authoritative for a given fd; closing an fd clears both (see
//!   [`inode_clear`]) so a recycled slot never carries stale inode state.

/// Open files per process (matches xv6 `kernel/param.h` `NOFILE`).
pub const NOFILE: u32 = 16;

/// Process slots (matches `PROC_TABLE` `max_entries` and `MAX_PROCESSES`).
pub const MAX_PROCESSES: u32 = 8;

/// `FD_TABLE` / `FD_INODE_MAP` length: one `NOFILE`-wide row per pid.
pub const FD_TABLE_LEN: u32 = NOFILE * MAX_PROCESSES; // 128

// ── Descriptor kinds (values stored in FD_TABLE) ───────────────────────────
/// Free (closed) descriptor slot. Zero so a fresh (zeroed) map starts empty.
pub const FD_FREE: u32 = 0;
/// Console device: read → KBD_MAP/stdin queue, write → TTY MMIO 0xC001.
pub const FD_CONSOLE: u32 = 1;
/// Inode-backed file (Gate D): read/fstat resolve through `FD_INODE_MAP`'s
/// `{inum, offset}` against `fs.img`. Distinct from [`FD_FREE`]/[`FD_CONSOLE`]
/// so the read/fstat handlers can tell a real file apart from the console.
pub const FD_INODE: u32 = 2;

/// Flat table index for (`pid`, `fd`). The caller guarantees `fd < NOFILE`
/// and `pid < MAX_PROCESSES`; out-of-range inputs would index a neighbouring
/// row, so the BPF side bounds-checks `fd < NOFILE` before calling.
#[inline(always)]
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

// ── Per-fd inode state (Gate D) ────────────────────────────────────────────
//
// Off-target model of the live `FD_INODE_MAP` (`Array<[u32; 2]>`). The BPF
// helpers in `main.rs` (`fd_inode`/`fd_set_inode`/`fd_set_offset`/
// `fd_clear_inode`) walk the map via the same [`fd_slot`] math; these pure fns
// operate on a caller-supplied `[[u32; 2]]` so the storage semantics (disjoint
// rows, bind/advance/clear) are exercised without a kernel. `table` is one
// `[inum, offset]` entry per fd, conventionally [`FD_TABLE_LEN`] long.

/// Read `{inum, offset}` for (`pid`, `fd`), or `(0, 0)` if `fd >= NOFILE` or
/// the slot is out of `table`'s range (mirrors the BPF NULL-check return).
#[inline(always)]
pub fn inode_get(table: &[[u32; 2]], pid: u8, fd: u32) -> (u32, u32) {
    if fd >= NOFILE {
        return (0, 0);
    }
    match table.get(fd_slot(pid, fd) as usize) {
        Some(e) => (e[0], e[1]),
        None => (0, 0),
    }
}

/// Bind (`pid`, `fd`) to inode `inum` at offset 0 (the open-time state). No-op
/// if `fd >= NOFILE` or the slot is out of range. Caller tags the kind
/// [`FD_INODE`] in `FD_TABLE` separately (the BPF `fd_set_inode` does both).
#[inline(always)]
pub fn inode_bind(table: &mut [[u32; 2]], pid: u8, fd: u32, inum: u32) {
    if fd >= NOFILE {
        return;
    }
    if let Some(e) = table.get_mut(fd_slot(pid, fd) as usize) {
        *e = [inum, 0];
    }
}

/// Advance the stored read offset for (`pid`, `fd`), keeping `inum`. No-op if
/// `fd >= NOFILE` or the slot is out of range.
#[inline(always)]
pub fn inode_set_offset(table: &mut [[u32; 2]], pid: u8, fd: u32, off: u32) {
    if fd >= NOFILE {
        return;
    }
    if let Some(e) = table.get_mut(fd_slot(pid, fd) as usize) {
        e[1] = off;
    }
}

/// Clear (`pid`, `fd`)'s inode state to `{0, 0}` — called on `close`/recycle so
/// a reused descriptor never inherits a previous file's inum or offset. No-op
/// if `fd >= NOFILE` or the slot is out of range.
#[inline(always)]
pub fn inode_clear(table: &mut [[u32; 2]], pid: u8, fd: u32) {
    if fd >= NOFILE {
        return;
    }
    if let Some(e) = table.get_mut(fd_slot(pid, fd) as usize) {
        *e = [0, 0];
    }
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
    fn descriptor_kinds_are_distinct() {
        // read/fstat must be able to tell console, file, and closed apart.
        assert_ne!(FD_FREE, FD_CONSOLE);
        assert_ne!(FD_FREE, FD_INODE);
        assert_ne!(FD_CONSOLE, FD_INODE);
        // FD_FREE is zero so a freshly-zeroed map reads as "all closed".
        assert_eq!(FD_FREE, 0);
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

    // ── Per-fd inode state ─────────────────────────────────────────────────

    fn fresh_table() -> [[u32; 2]; FD_TABLE_LEN as usize] {
        [[0u32; 2]; FD_TABLE_LEN as usize]
    }

    #[test]
    fn inode_bind_then_get_is_inum_at_offset_zero() {
        let mut t = fresh_table();
        inode_bind(&mut t, 1, 3, 42);
        assert_eq!(inode_get(&t, 1, 3), (42, 0));
    }

    #[test]
    fn inode_set_offset_advances_and_keeps_inum() {
        let mut t = fresh_table();
        inode_bind(&mut t, 2, 5, 7);
        inode_set_offset(&mut t, 2, 5, 1024);
        assert_eq!(inode_get(&t, 2, 5), (7, 1024));
        // A second advance overwrites the offset, inum still intact.
        inode_set_offset(&mut t, 2, 5, 2048);
        assert_eq!(inode_get(&t, 2, 5), (7, 2048));
    }

    #[test]
    fn inode_rows_are_disjoint_across_pids() {
        // Binding pid 1's fd 3 must not disturb pid 2's fd 3 (same fd index,
        // different pid row) — the isolation invariant FD_INODE_MAP relies on.
        let mut t = fresh_table();
        inode_bind(&mut t, 1, 3, 11);
        inode_bind(&mut t, 2, 3, 22);
        inode_set_offset(&mut t, 1, 3, 100);
        assert_eq!(inode_get(&t, 1, 3), (11, 100));
        assert_eq!(inode_get(&t, 2, 3), (22, 0));
    }

    #[test]
    fn inode_clear_on_close_wipes_state() {
        // recycle-on-close: a closed fd, when reopened, must not see the old
        // file's inum or offset.
        let mut t = fresh_table();
        inode_bind(&mut t, 0, 4, 99);
        inode_set_offset(&mut t, 0, 4, 512);
        inode_clear(&mut t, 0, 4);
        assert_eq!(inode_get(&t, 0, 4), (0, 0));
    }

    #[test]
    fn inode_helpers_ignore_out_of_range_fd() {
        // fd >= NOFILE must never index a neighbouring pid's row.
        let mut t = fresh_table();
        inode_bind(&mut t, 0, NOFILE, 1234); // out of range → no-op
        assert_eq!(inode_get(&t, 0, NOFILE), (0, 0));
        // pid 1's fd 0 (the slot fd==NOFILE of pid 0 would alias) is untouched.
        assert_eq!(inode_get(&t, 1, 0), (0, 0));
    }
}
