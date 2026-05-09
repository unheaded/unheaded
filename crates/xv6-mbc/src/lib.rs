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
/// Per `docs/doom/UPC_BOOT_PROTOCOL.md` ("encodes UNHD in ASCII") and
/// `docs/doom/UPC_BOOT_PROTOCOL_V2.md:113` (`u32 magic; /* 'UNHD' = 0x554E4844 */`).
///
/// **Display caveat (Marshal note 2026-05-09):** when this u32 is rendered as
/// little-endian bytes (e.g. via `cmd/upc-bootctl validate`'s
/// `BOOT_MAGIC.to_le_bytes()` call) the visible string is `"DHNU"`, not
/// `"UNHD"` — the spec's "UNHD" framing reads the bytes in big-endian order.
/// Either the spec needs an endianness clarification or the in-memory layout
/// should be byte-swapped to `0x44484E55`. Surfaced for daytime resolution;
/// the constant value itself is left at the spec-pinned `0x554E4844`.
pub const BOOT_MAGIC: u32 = 0x554E4844; // 'UNHD' per spec, 'DHNU' if read le

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
        // Pin the *actual* observable byte order so any future endianness
        // change is an intentional, reviewed edit rather than silent drift.
        // See the BOOT_MAGIC docstring for the ongoing spec-clarity question.
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
