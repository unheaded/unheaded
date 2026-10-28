// SPDX-License-Identifier: GPL-3.0-or-later
//! The `.rv2mbc` SHA-256 gate for every image upc-bootctl loads (S-ASCEND-2
//! Item 5, ADR-075 §Security #1).
//!
//! Before anything opens the eBPF object, [`check_all`] reads every `.rv2mbc`
//! a boot will put into RV2MBC_MAP (the kernel's, the `--userland`
//! program's and each resident program's) and checks it with
//! `upc_api::rv2mbc_sha::read_verified`. The boot then writes those same bytes;
//! no `.rv2mbc` is read twice. The kernel's expected hash may be given
//! explicitly (`--rv2mbc-sha`, else `UPC_RV2MBC_SHA`); every other file needs
//! its `<file>.rv2mbc.sha256` sidecar.

use anyhow::{anyhow, bail, Context, Result};
use std::path::{Path, PathBuf};
use upc_api::rv2mbc_sha::{read_verified, Rv2mbcShaError};

use crate::geometry;

/// One `.rv2mbc` file, read once and checked.
pub struct CheckedRv2mbc {
    pub path: PathBuf,
    pub bytes: Vec<u8>,
    /// The verified SHA-256. `None` only under the dry-run hatch.
    pub sha256: Option<[u8; 32]>,
}

impl CheckedRv2mbc {
    pub fn sha_hex(&self) -> Option<String> {
        self.sha256.as_ref().map(upc_api::object::hex32)
    }
}

/// Every `.rv2mbc` one boot loads.
pub struct Rv2mbcSet {
    /// The kernel's. `None` only on a dry run (a live boot refuses).
    pub kernel: Option<CheckedRv2mbc>,
    /// The `--userland` program's; `None` without `--userland` or when that
    /// program has no `.rv2mbc` (nothing is written, as before).
    pub userland: Option<CheckedRv2mbc>,
    /// Indexed like `geometry::RESIDENT_PROGRAMS`: the program's `.mbc` and its
    /// checked `.rv2mbc`, or `None` when the `.mbc` is absent (skipped).
    pub programs: Vec<Option<(PathBuf, CheckedRv2mbc)>>,
}

/// `--allow-unverified-rv2mbc` is a lab-only hatch for inspecting images that
/// have no sidecar: accepted with `--dry-run`, refused on a live run.
pub fn hatch_gate(allow_unverified: bool, dry_run: bool) -> Result<()> {
    if allow_unverified && !dry_run {
        bail!(
            "--allow-unverified-rv2mbc is accepted only with --dry-run; a live boot \
             verifies every .rv2mbc (the Makefiles write the .sha256 sidecars)"
        );
    }
    Ok(())
}

/// Read and check one `.rv2mbc`. Under the hatch a missing hash is a warning;
/// a mismatch or a malformed hash is refused either way.
fn check_one(path: &Path, explicit: Option<&str>, allow_unverified: bool) -> Result<CheckedRv2mbc> {
    match read_verified(path, explicit) {
        Ok(v) => Ok(CheckedRv2mbc {
            path: path.to_path_buf(),
            bytes: v.bytes,
            sha256: Some(v.sha256),
        }),
        Err(e @ Rv2mbcShaError::Required { .. }) if allow_unverified => {
            eprintln!("warning: --allow-unverified-rv2mbc: {e}; continuing unverified (dry run)");
            let bytes = std::fs::read(path).with_context(|| format!("read {}", path.display()))?;
            Ok(CheckedRv2mbc {
                path: path.to_path_buf(),
                bytes,
                sha256: None,
            })
        }
        Err(e) => Err(anyhow!("refusing to boot: {e}")),
    }
}

/// Check every `.rv2mbc` the boot of `kernel` (+ `userland` and its resident
/// siblings) will load. Runs before the eBPF object is opened.
pub fn check_all(
    kernel: &Path,
    userland: Option<&Path>,
    explicit_kernel_sha: Option<&str>,
    allow_unverified: bool,
    dry_run: bool,
) -> Result<Rv2mbcSet> {
    let kernel_rv = kernel.with_extension("rv2mbc");
    let kernel_checked = if kernel_rv.exists() {
        Some(check_one(
            &kernel_rv,
            explicit_kernel_sha,
            allow_unverified,
        )?)
    } else if dry_run {
        println!(
            "  ⚠ no rv2mbc map at {}: a live boot refuses (Rv2mbcMissing)",
            kernel_rv.display()
        );
        None
    } else {
        bail!(
            "Rv2mbcMissing: no kernel rv2mbc map at {}; BootParams v3 carries its SHA-256 \
             and MRET/SRET/JALR/CALLR need it",
            kernel_rv.display()
        );
    };

    let mut userland_checked = None;
    let mut programs = Vec::with_capacity(geometry::RESIDENT_PROGRAMS.len());
    if let Some(up) = userland {
        let up_rv = up.with_extension("rv2mbc");
        if up_rv.exists() {
            userland_checked = Some(check_one(&up_rv, None, allow_unverified)?);
        }
        let dir = up.parent().unwrap_or_else(|| Path::new("."));
        for name in geometry::RESIDENT_PROGRAMS {
            let mbc = dir.join(format!("{name}.mbc"));
            if !mbc.exists() {
                programs.push(None);
                continue;
            }
            let checked = check_one(&mbc.with_extension("rv2mbc"), None, allow_unverified)?;
            programs.push(Some((mbc, checked)));
        }
    }

    for c in kernel_checked
        .iter()
        .chain(userland_checked.iter())
        .chain(programs.iter().flatten().map(|(_, c)| c))
    {
        match c.sha_hex() {
            Some(h) => println!("  ✓ rv2mbc sha256 {h} {}", c.path.display()),
            None => println!("  ⚠ rv2mbc UNVERIFIED {}", c.path.display()),
        }
    }

    Ok(Rv2mbcSet {
        kernel: kernel_checked,
        userland: userland_checked,
        programs,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn hatch_is_dry_run_only() {
        assert!(hatch_gate(false, false).is_ok());
        assert!(hatch_gate(false, true).is_ok());
        assert!(hatch_gate(true, true).is_ok());
        let e = hatch_gate(true, false).unwrap_err().to_string();
        assert!(e.contains("accepted only with --dry-run"), "{e}");
    }
}
