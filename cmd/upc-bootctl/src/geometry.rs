// SPDX-License-Identifier: GPL-3.0-or-later
//! The xv6 guest's physical memory layout as the loader declares it to the
//! I/O-time gates (ADR-080 §"I/O-time gates", Gate B; S-ASCEND-2 Step 106).
//!
//! The child-slice geometry lives in the eBPF interpreter
//! (`ebpf/monad-cpu-ebpf/src/phase12.rs`, `src/main.rs`), which a host crate
//! cannot import, so the values are cited here and pinned by
//! `tests/parity_slice_constants.rs`. The gate then compares them with the
//! `UPC_MMU_GEOMETRY` the object carries, so a stale copy refuses to boot
//! rather than booting with a wrong idea of the layout. The owned kernel
//! (`crates/xv6-mbc/adapters/*.c`) does not compute child slices: it only
//! places per-pid page directories (`uvm.c` `MBC_PER_PID_PGD_BASE`).
//!
//! Also used by `tests/parity_slice_constants.rs` through `#[path]`, so it
//! depends only on `upc_api`, `monad_common` and `anyhow`.

use anyhow::{bail, Result};
use upc_api::{MemRegion, MmuLayout, SliceGeometry};

/// pid 1's physical slice base: `phase12::USER_CHILD_SLICE` (16 MiB).
pub const XV6_CHILD_SLICE_BASE: u32 = 0x0100_0000;
/// Per-pid slice stride: `phase12::SLICE_STRIDE` (8 MiB).
pub const XV6_SLICE_STRIDE: u32 = 0x0080_0000;
/// Bytes each child slice maps: `USER_VA_CEILING` in `monad-cpu-ebpf/src/main.rs`
/// (VA[0, 8 MiB) through two 4 MiB superpage leaves).
pub const XV6_SLICE_LEN: u32 = 0x0080_0000;
/// fork()'s capacity guard: `phase12::RAM_ISOLATION_CEILING` (56 MiB).
pub const XV6_ISOLATION_CEILING: u32 = 0x0380_0000;
/// Process-table size: `monad_common::mbc_interrupts::MAX_PROCESSES`, the bound
/// of fork()'s free-slot scan and `PROC_TABLE`'s `max_entries`.
pub const XV6_MAX_PIDS: u32 = monad_common::mbc_interrupts::MAX_PROCESSES as u32;

/// Top of the owned kernel's physical memory: `-DPHYSTOP=0x00800000UL` in
/// `crates/xv6-mbc/adapters/Makefile.mbc`. pid 0 (init) is identity-mapped over
/// the same VA[0, 8 MiB), and every MMIO window sits below it.
pub const XV6_PHYSTOP: u32 = 0x0080_0000;
/// Ramdisk (fs.img) base: `mbc_block::RAMDISK_BASE_WORD` × 4; `boot --ramdisk`
/// writes it here.
pub const XV6_RAMDISK_BASE: u32 = monad_common::mbc_block::RAMDISK_BASE_WORD * 4;
/// Ramdisk size: `mbc_block::RAMDISK_SIZE` (4 MiB).
pub const XV6_RAMDISK_LEN: u32 = monad_common::mbc_block::RAMDISK_SIZE;
/// Resident programs exec(7) can resolve, in PROGRAM_TABLE slot order.
pub const RESIDENT_PROGRAMS: &[&str] = &["sh", "echo", "ls", "cat", "wc"];
/// Pristine RAM where each resident program's `.data` is staged for exec(7).
pub const RESIDENT_DATA_STAGE_BASE: u32 = 0x00C0_0000;
/// Distance between consecutive staged `.data` regions.
pub const RESIDENT_DATA_STAGE_STRIDE: u32 = 0x0004_0000;
/// Per-pid page directories: `MBC_PER_PID_PGD_BASE` in `adapters/uvm.c`,
/// `phase12::PER_PID_PGD_BASE`; one 4 KiB pgd per pid.
pub const XV6_PGD_BASE: u32 = 0x00F0_0000;
/// One page directory.
pub const XV6_PGD_SIZE: u32 = 0x1000;
/// Initial kernel SP: `xv6_initial_cpu_state()` in `runner.rs` (grows down).
pub const XV6_BOOT_SP: u32 = 0x03F0_0000;

/// The geometry the loader expects the ascend object to ship.
pub fn xv6_slice_geometry(stride: u32) -> SliceGeometry {
    SliceGeometry {
        base: XV6_CHILD_SLICE_BASE,
        stride,
        slice_len: XV6_SLICE_LEN,
        ceiling: XV6_ISOLATION_CEILING,
        max_pids: XV6_MAX_PIDS,
    }
}

/// Everything the loader and the kernel place in physical RAM that a child
/// slice must not overlap.
pub fn xv6_reserved_regions() -> Vec<MemRegion> {
    vec![
        MemRegion {
            name: "kernel image, pid-0 identity slice and MMIO windows (below PHYSTOP)",
            start: 0,
            len: XV6_PHYSTOP,
        },
        MemRegion {
            name: "ramdisk (fs.img)",
            start: XV6_RAMDISK_BASE,
            len: XV6_RAMDISK_LEN,
        },
        MemRegion {
            name: "resident program .data staging",
            start: RESIDENT_DATA_STAGE_BASE,
            len: RESIDENT_PROGRAMS.len() as u32 * RESIDENT_DATA_STAGE_STRIDE,
        },
        MemRegion {
            name: "per-pid page directories",
            start: XV6_PGD_BASE,
            len: XV6_MAX_PIDS * XV6_PGD_SIZE,
        },
        MemRegion {
            name: "kernel boot stack (above RAM_ISOLATION_CEILING)",
            start: XV6_ISOLATION_CEILING,
            len: XV6_BOOT_SP - XV6_ISOLATION_CEILING,
        },
    ]
}

/// The xv6 MMU layout, with the shipped stride unless `stride` overrides it.
pub fn xv6_mmu_layout(stride: Option<u32>) -> MmuLayout {
    MmuLayout {
        slices: xv6_slice_geometry(stride.unwrap_or(XV6_SLICE_STRIDE)),
        reserved: xv6_reserved_regions(),
    }
}

/// The `UPC_SLICE_STRIDE` test override, honoured only with `--unsafe-geometry`.
///
/// It changes only what the loader *declares*; the object's stride is a
/// compile-time constant. A declared stride below the slice length is refused
/// with `SliceOverlap`, any other value that differs from the object's with
/// `SliceGeometryMismatch`, so the override can make the loader refuse but can
/// never make it boot a geometry the object does not ship. It exists for the
/// S-ASCEND-2 Step 111 negative gate.
pub fn stride_override(unsafe_geometry: bool, env: Option<&str>) -> Result<Option<u32>> {
    let Some(raw) = env else {
        return Ok(None);
    };
    if !unsafe_geometry {
        eprintln!("  ⚠ UPC_SLICE_STRIDE={raw} ignored: it is read only with --unsafe-geometry");
        return Ok(None);
    }
    let raw = raw.trim();
    let parsed = match raw.strip_prefix("0x").or_else(|| raw.strip_prefix("0X")) {
        Some(hex) => u32::from_str_radix(hex, 16),
        None => raw.parse::<u32>(),
    };
    let Ok(stride) = parsed else {
        bail!("UPC_SLICE_STRIDE={raw}: not a u32 (decimal or 0x-hex)");
    };
    eprintln!(
        "  ⚠ --unsafe-geometry: declaring slice stride 0x{stride:X} instead of the shipped \
         0x{XV6_SLICE_STRIDE:X}; the loader gate will refuse any stride the object does not ship"
    );
    Ok(Some(stride))
}
