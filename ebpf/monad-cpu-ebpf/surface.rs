// SPDX-License-Identifier: GPL-3.0-or-later
//
// The ecall surface each build of this object compiles in: every syscall
// number with a top-level arm in the `op::SYSCALL` dispatch of src/main.rs
// (the chain that starts at `} else if opc == op::SYSCALL {`) under the
// object's `cfg(feature)` set. A number with no arm falls through to
// "Unknown syscall: silently ignore".
//
// build.rs hashes the list for the active feature set into UPC_SURFACE_ID
// (src/upc_facts.rs); the loaders compare it to their Workload's declared
// surface before any map write (ADR-080 §"I/O-time gates").
// cmd/upc-bootctl/tests/parity_surface.rs parses the dispatch chain and fails
// when an arm is added or removed without updating this file.
//
// Out of scope: the INT 0x80 Linux-compatible path (`vector ==
// intr::VECTOR_SYSCALL`, non-ascend builds only). It is a second entry vector
// that no shipped guest uses; it is not part of either guest's surface.
//
// Plain items only: this file is `include!`d by build.rs and by host tests.

/// Default build (no `ascend-linux`; the Doom object): DRAW_FRAME 1,
/// GET_KEY 2, GET_TICKS 3, SLEEP 4 (`monad_common::mbc_syscalls`), write 16
/// (both builds), READ_BLOCK 200 and WRITE_BLOCK 201
/// (`monad_common::mbc_linux_syscalls`, both builds).
pub const SURFACE_DEFAULT: &[u32] = &[1, 2, 3, 4, 16, 200, 201];

/// `ascend-linux` build (the xv6 object): xv6 fork 1, exit 2, wait 3, read 5,
/// exec 7, fstat 8, dup 10, getpid 11, pause 13, uptime 14 (ADR-110), open 15, write 16, mknod 17,
/// close 21, plus READ_BLOCK 200 and WRITE_BLOCK 201. xv6's pipe 4, kill 6,
/// chdir 9, sbrk 12, unlink 18, link 19 and mkdir 20 have no arm.
pub const SURFACE_ASCEND: &[u32] = &[1, 2, 3, 5, 7, 8, 10, 11, 13, 14, 15, 16, 17, 21, 200, 201];
