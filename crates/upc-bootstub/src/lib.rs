// SPDX-License-Identifier: GPL-3.0-or-later
//
//! # upc-bootstub
//!
//! Stage-1 boot stub crate for uClinux + Linux on UPC (ASCEND-LINUX Phase 2).
//!
//! ## What this crate IS
//!
//! A Rust workspace shim that locates the pre-built `upc-bootstub.mbc`
//! artifact (compiled from `src/start.c` via the same RV32I → MBC translator
//! pipeline xv6 uses). cmd/upc-bootctl will consume this path the same way
//! it consumes `xv6-mbc::image_path()` today.
//!
//! ## What this crate IS NOT
//!
//! Not a Rust implementation of the boot stub. The boot stub itself is C
//! (start.c), translated to MBC bytecode at build time, run inside the
//! eBPF UPC interpreter at boot. This crate is the workspace-integration
//! layer; the actual logic lives in the C source under `src/`.
//!
//! ## Phase 2 contract
//!
//! When `cmd/upc-bootctl boot --kernel uclinux.mbc --initramfs rootfs.cpio.gz`
//! runs, the loader:
//!
//! 1. Writes BootParamsV2 at byte 0x0100 (per UPC_BOOT_PROTOCOL_V2.md).
//! 2. Loads `upc-bootstub.mbc` into ROM_MAP starting at byte 0x10000
//!    (same slot xv6 currently occupies — bootstub replaces start_mbc.c
//!    for non-xv6 kernels).
//! 3. Loads the kernel (uclinux/linux) into ROM_MAP at byte 0x20000.
//! 4. Loads the initramfs CPIO into RAM_MAP at byte 0x800000 (per
//!    BootParamsV2.ramdisk_addr).
//! 5. Sets CPU.PC = 0x4000 (word index of byte 0x10000).
//! 6. Dispatches the trigger packet.
//!
//! At PC=0x4000 the bootstub takes over:
//!
//! 1. Verify BootParams magic (`0x554E4844` per ADR-072) + version (`2`).
//! 2. Zero `[bss_start, bss_end)` byte range from BootParamsV2.
//! 3. Optionally CPIO-unpack initramfs into RAM_MAP at the addresses
//!    the kernel's `populate_rootfs()` will read from. (Phase 2.4
//!    refines this — Phase 2 day-1 may use a pre-unpacked layout.)
//! 4. Set MEPC = kernel entry, MSTATUS.MPP = S-mode (0b01).
//! 5. MRET — kernel takes over at supervisor privilege.
//!
//! If any verify step fails: print "BOOT FAIL: <reason>" via MMIO 0xC001
//! and HALT.
//!
//! ## Status: builds end-to-end (Phase 2.0 preflight, 2026-07-02)
//!
//! - `Cargo.toml` + `lib.rs` + `src/start.c` in place; steps 1–5 are
//!   IMPLEMENTED in `start.c` (not stubs — magic/version verify, BSS zero,
//!   MEPC/MSTATUS.MPP set, MRET, fail-fast diagnostics).
//! - `make -f adapters/Makefile.bootstub all` compiles start.c → links →
//!   `rv32i-to-mbc` → `target/upc-bootstub.mbc` (115 RV32I → 194 MBC insns).
//!   `upc-bootctl validate --kernel target/upc-bootstub.mbc` decodes it clean.
//! - NOT yet boot-integrated: the stub has no kernel to hand off to until
//!   Phase 2.1 vendors one, and per the Phase 2.0 capacity audit
//!   (`references/phase2-preflight-capacity-audit-2026-07-02.md`) the
//!   code-store substrate can't hold a Linux image yet. Boot-path wiring
//!   (`upc-bootctl --bootstub`) waits on that substrate decision.
//! - Loader note for Phase 2.4: the ilp32e link keeps `.rodata` in the BOOT
//!   text region AND the translator emits a `.data` sibling for it — decide
//!   which the loader stages so the "BOOT FAIL" strings resolve once.

#![deny(missing_docs)]

/// The byte address (in MBC virtual memory) where the bootstub is loaded
/// by `cmd/upc-bootctl`. Matches the kernel-mbc.ld `.stage1` region origin.
/// Word index in ROM_MAP = `BOOTSTUB_LOAD_ADDR / 4` = `0x4000`.
pub const BOOTSTUB_LOAD_ADDR: u32 = 0x0001_0000;

/// The MBC PC value that initial CPU state should be set to in order for
/// the bootstub to be the first code to execute.
pub const BOOTSTUB_ENTRY_PC: u32 = BOOTSTUB_LOAD_ADDR / 4;

/// Default kernel entry byte address — where the bootstub jumps via MRET
/// after verifying BootParams and zeroing BSS. Matches kernel-mbc.ld's
/// `.text` section origin (`0x00020000`).
pub const KERNEL_ENTRY_ADDR: u32 = 0x0002_0000;

/// Default initramfs load byte address. Matches BootParamsV2.ramdisk_addr.
/// Lives high in the RAM_MAP region so it doesn't collide with the
/// kernel's .data/.bss working set.
pub const INITRAMFS_LOAD_ADDR: u32 = 0x0080_0000;

/// MSTATUS.MPP field encoding for "Supervisor mode" (bits 12:11 = 0b01).
/// The bootstub sets this before MRET so the kernel begins at S-mode
/// privilege (xv6 pattern). M-mode-only kernels would set 0b11 instead.
pub const MSTATUS_MPP_S: u32 = 1 << 11;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn bootstub_load_addr_is_byte_0x10000() {
        // Per UPC_BOOT_PROTOCOL_V2.md memory layout § and kernel-mbc.ld
        // .stage1 region — must match for every kernel that uses the stub.
        assert_eq!(BOOTSTUB_LOAD_ADDR, 0x0001_0000);
    }

    #[test]
    fn bootstub_entry_pc_is_word_index_of_load_addr() {
        // CPU.PC is a word index into ROM_MAP, not a byte address.
        assert_eq!(BOOTSTUB_ENTRY_PC, BOOTSTUB_LOAD_ADDR / 4);
        assert_eq!(BOOTSTUB_ENTRY_PC, 0x4000);
    }

    // Constant-vs-constant by design: these assertions guard the UPC boot
    // memory map. If someone moves KERNEL_ENTRY_ADDR or BOOTSTUB_LOAD_ADDR
    // into an overlapping range, this test is what catches it. Being
    // compile-time decidable is the feature.
    #[allow(clippy::assertions_on_constants)]
    #[test]
    fn kernel_entry_is_above_bootstub_region() {
        // Bootstub occupies byte 0x10000..0x1FFFF (64 KB). Kernel image
        // must start at or above 0x20000 so the two never overlap.
        assert!(KERNEL_ENTRY_ADDR >= 0x0002_0000);
        assert!(KERNEL_ENTRY_ADDR > BOOTSTUB_LOAD_ADDR);
    }

    #[allow(clippy::assertions_on_constants)] // layout guard — see note above
    #[test]
    fn initramfs_addr_does_not_collide_with_rom() {
        // ROM_MAP is 64 KB max addressable as bytes (16384 word slots).
        // Initramfs must live in RAM_MAP, not ROM_MAP.
        assert!(INITRAMFS_LOAD_ADDR > 0x0010_0000); // RAM_MAP base per Wotan model
    }

    #[test]
    fn mstatus_mpp_s_bit_pattern_matches_riscv_priv_spec() {
        // MPP at bits 12:11 = 0b01 == 0x800.
        assert_eq!(MSTATUS_MPP_S, 0x800);
    }
}
