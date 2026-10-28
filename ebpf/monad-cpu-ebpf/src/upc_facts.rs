// SPDX-License-Identifier: GPL-3.0-or-later
//! What this object declares about itself to the host loaders (ADR-080
//! §"I/O-time gates"). Both statics are data in `.rodata.upc`: no BPF code
//! path reads them, so they add no instructions and leave the verified-insn
//! count unchanged. `upc_api::ObjectFacts::from_elf` reads them from the file
//! before the loader opens it.

use crate::phase12;
use monad_common::mbc_interrupts as intr;

include!(concat!(env!("OUT_DIR"), "/upc_surface_id.rs"));

/// Gate A: the surface id of the ecall numbers this build's `op::SYSCALL`
/// dispatch compiles in (`surface.rs`, hashed by `build.rs`).
#[no_mangle]
#[link_section = ".rodata.upc"]
pub static UPC_SURFACE_ID: [u8; 32] = UPC_SURFACE_ID_BYTES;

/// Gate B: the per-pid slice geometry fork() and `user_phys()` use, as
/// `[base, stride, slice_len, ceiling, max_pids]`: `USER_CHILD_SLICE`,
/// `SLICE_STRIDE`, `USER_VA_CEILING` (each pid's mapped VA window: two 4 MiB
/// superpage leaves), `RAM_ISOLATION_CEILING`, and `intr::MAX_PROCESSES`
/// (the bound of fork()'s free-slot scan).
#[no_mangle]
#[link_section = ".rodata.upc"]
pub static UPC_MMU_GEOMETRY: [u32; 5] = [
    phase12::USER_CHILD_SLICE,
    phase12::SLICE_STRIDE,
    crate::USER_VA_CEILING,
    phase12::RAM_ISOLATION_CEILING,
    intr::MAX_PROCESSES as u32,
];
