// SPDX-License-Identifier: GPL-3.0-or-later
//! xv6-mbc — adapter shim for the xv6-riscv kernel ported to MBC ISA.
//!
//! ASCEND-LINUX Phase 1 (`references/battle-plan-ascend-linux-2026-05-08.md`).
//!
//! This crate's primary deliverable is the xv6 kernel image at
//! `target/xv6-mbc.mbc`, built from the C source under `upstream/` (vendored
//! from MIT-PDOS xv6-riscv) plus the MBC adapters under `adapters/`. The Rust
//! shim here exists to:
//!
//! - Hold the workspace declaration for cargo (so the C build hooks into
//!   `cargo build`).
//! - Re-export the boot-protocol constants from `monad-common` and the
//!   instruction-set helpers from `monad-mbc` for the build script to consume.
//! - Provide a thin `image_path()` accessor that doom-runner (and future
//!   `cmd/upc-bootctl`) can use to locate the built kernel image.
//!
//! See `crates/xv6-mbc/README.md` for the full Phase 1 build flow.

#![deny(missing_docs)]

use std::path::PathBuf;

/// Returns the absolute path to the built xv6-mbc kernel image.
///
/// Returns `None` if the image hasn't been built yet (run `make` in
/// `crates/xv6-mbc/upstream/` first).
pub fn image_path() -> Option<PathBuf> {
    let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let img = manifest_dir.join("target").join("xv6-mbc.mbc");
    if img.exists() {
        Some(img)
    } else {
        None
    }
}

/// Re-export the UPC Boot Protocol v2 magic for callers that need to verify
/// the built image is well-formed before booting.
///
/// **Canonical form:** `0x554E4844` — the u32 hex constant, brand-spelled
/// `'UNHD'` when read MSB-first (the form used in spec text, equality
/// comparisons, and human conversation).
///
/// **Wire/memory form:** stored as a single little-endian u32, so the four
/// bytes at increasing memory addresses are `D, H, N, U` (`0x44, 0x48, 0x4E,
/// 0x55`). A debugger or hex-dump of the BootParams region shows `DHNU`;
/// kernel code reading the field as a u32 (RV32 LW) sees `0x554E4844` and
/// matches against the canonical hex.
///
/// Both representations describe the same 32-bit constant. This is the same
/// pattern as ELF's `\x7fELF` magic, which is `0x464C457F` as a host-endian
/// u32 on LE systems.
///
/// Resolved 2026-05-10 per ADR-072 ("BOOT_MAGIC byte-ordering convention");
/// see `docs/doom/UPC_BOOT_PROTOCOL.md` §"Magic Value" and
/// `docs/doom/UPC_BOOT_PROTOCOL_V2.md` line 113 for the spec-side framing.
pub const BOOT_MAGIC: u32 = 0x554E4844;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn boot_magic_pinned_to_spec_value() {
        // Per docs/doom/UPC_BOOT_PROTOCOL.md line 73 and
        // docs/doom/UPC_BOOT_PROTOCOL_V2.md line 113.
        // If this assertion fails, ensure the spec was updated in lockstep.
        assert_eq!(BOOT_MAGIC, 0x554E4844);
    }

    #[test]
    fn boot_magic_le_bytes_visual_is_dhnu() {
        // Intentional pin per ADR-072: the canonical hex 0x554E4844, when
        // serialized as little-endian bytes for the BootParams.magic field,
        // produces D,H,N,U at increasing memory addresses. Any future change
        // to the BOOT_MAGIC constant must be an intentional, reviewed edit
        // (ADR amendment + spec doc update).
        let bytes = BOOT_MAGIC.to_le_bytes();
        assert_eq!(&bytes, b"DHNU");
    }

    #[test]
    fn image_path_contract_holds() {
        // Logical contract: Some(p) => p actually exists and is named
        // xv6-mbc.mbc; None => acceptable (image not built in this clone).
        match image_path() {
            Some(p) => {
                assert!(p.exists(), "Some(p) returned but {:?} does not exist", p);
                assert_eq!(p.file_name().unwrap(), "xv6-mbc.mbc");
            }
            None => {
                // No image built — acceptable in CI / fresh clone.
            }
        }
    }
}
