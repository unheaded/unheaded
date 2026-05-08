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
pub const BOOT_MAGIC: u32 = 0x554E4844; // 'UNHD'
