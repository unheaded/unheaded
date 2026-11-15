// SPDX-License-Identifier: GPL-3.0-or-later
//! ADR-135 D17 (S-STAGE-2C Phase 4, R-S4): a user image holds no atomic and no
//! privileged return.
//!
//! The RV32I to MBC translator emits none of XCHG, CAS, LR_W, SC_W, PUSH, POP,
//! INT or IRET, and a user program has no use for MRET or SRET. The interpreter's atomics take their
//! address with no translation, and the stack and interrupt opcodes go through
//! r15 the same way, so at U-mode they would reach physical memory.
//! An image holding one did not come from the translator. [`scan_user_images`]
//! reads the `--userland` image and every resident program before the eBPF
//! object is opened and refuses the boot on the first such word.
//!
//! The kernel image is not scanned: it needs MRET and SRET.
//!
//! `UPC_ALLOW_USER_PRIVRET=1` admits MRET and SRET, for the one probe that runs
//! them at U-mode on purpose (`hostile_reply`); the interpreter refuses both at
//! priv 3 (ADR-109, 2026-11-15). Nothing admits an atomic.

use crate::rv2mbc::Rv2mbcSet;
use anyhow::{bail, Context, Result};
use monad_common::mbc_opcodes as op;
use std::path::Path;

/// The probe hatch. Read once per boot, in [`scan_user_images`].
pub const PRIVRET_HATCH_ENV: &str = "UPC_ALLOW_USER_PRIVRET";

/// What a forbidden opcode is, or `None`. The bool says whether the probe
/// hatch may admit it.
fn forbidden(opcode: u8) -> Option<(&'static str, bool)> {
    match opcode {
        op::XCHG => Some(("XCHG", false)),
        op::CAS => Some(("CAS", false)),
        op::LR_W => Some(("LR_W", false)),
        op::SC_W => Some(("SC_W", false)),
        // Step 29 review, RV-2 (2026-11-15): these go through r15 with no
        // translation, so at U-mode a chosen stack pointer is a physical
        // address. INT is refused whatever its vector.
        op::PUSH => Some(("PUSH", false)),
        op::POP => Some(("POP", false)),
        op::INT => Some(("INT", false)),
        op::IRET => Some(("IRET", false)),
        op::MRET => Some(("MRET", true)),
        op::SRET => Some(("SRET", true)),
        _ => None,
    }
}

/// Refuse `words` if any is an atomic, or a privileged return while
/// `allow_privret` is false. `name` is the image, for the message.
pub fn scan_user_image(name: &str, words: &[u32], allow_privret: bool) -> Result<()> {
    for (i, w) in words.iter().enumerate() {
        let opcode = (w >> 24) as u8;
        let Some((mnemonic, hatchable)) = forbidden(opcode) else {
            continue;
        };
        if hatchable && allow_privret {
            continue;
        }
        bail!(
            "UserImageOpcode: {name}: word {i} (0x{w:08X}) is {mnemonic} (opcode 0x{opcode:02X}); \
             a user image holds no atomic, no stack or interrupt opcode and no privileged \
             return (ADR-135 D17)"
        );
    }
    Ok(())
}

fn read_words(path: &Path) -> Result<Vec<u32>> {
    let bytes = std::fs::read(path).with_context(|| format!("read {}", path.display()))?;
    if !bytes.len().is_multiple_of(4) {
        bail!("{} not 4-byte aligned", path.display());
    }
    Ok(bytes
        .as_chunks::<4>()
        .0
        .iter()
        .map(|c| u32::from_le_bytes(*c))
        .collect())
}

/// Scan the `--userland` image and the resident programs `check_all` found.
/// Runs before the eBPF object is opened, in a dry run too.
pub fn scan_user_images(userland: Option<&Path>, rv: &Rv2mbcSet) -> Result<()> {
    let Some(up) = userland else {
        return Ok(());
    };
    let allow_privret = std::env::var(PRIVRET_HATCH_ENV).ok().as_deref() == Some("1");
    if allow_privret {
        println!(
            "  ⚠ {PRIVRET_HATCH_ENV}=1: MRET and SRET are admitted in user images (probe hatch); \
             atomics are still refused"
        );
    }
    let mut scanned = 0usize;
    let name = up
        .file_stem()
        .and_then(|s| s.to_str())
        .unwrap_or("userland");
    scan_user_image(name, &read_words(up)?, allow_privret)?;
    scanned += 1;
    for (mbc, _) in rv.programs.iter().flatten() {
        let name = mbc
            .file_stem()
            .and_then(|s| s.to_str())
            .unwrap_or("program");
        scan_user_image(name, &read_words(mbc)?, allow_privret)?;
        scanned += 1;
    }
    println!("  ✓ opcode scan: {scanned} user image(s) hold no atomic and no privileged return");
    Ok(())
}

/// S-STAGE-2C Phase 5 (A-61): the `.data` sidecar of the `--userland` image
/// and of every resident program must be there and must parse. A program with
/// a bad or missing sidecar would run with whatever is in memory where its
/// strings, globals and `.bss` should be. An empty TLV (count 0) is a sidecar.
/// Runs before the eBPF object is opened, in a dry run too.
pub fn check_user_sidecars(userland: Option<&Path>, rv: &Rv2mbcSet) -> Result<()> {
    let Some(up) = userland else {
        return Ok(());
    };
    let mut checked = 0usize;
    for mbc in std::iter::once(up).chain(rv.programs.iter().flatten().map(|(m, _)| m.as_path())) {
        let data = mbc.with_extension("data");
        if !data.exists() {
            bail!(
                "UserSidecarMissing: no .data sidecar at {} for {}; rebuild it with the \
                 userland Makefile (the sidecar carries .rodata, .data and .bss)",
                data.display(),
                mbc.display()
            );
        }
        let blob = std::fs::read(&data).with_context(|| format!("read {}", data.display()))?;
        crate::runner::BootRunner::parse_data_regions(&blob, &data.display().to_string())
            .map_err(|e| anyhow::anyhow!("UserSidecarBad: {e}"))?;
        checked += 1;
    }
    println!("  ✓ .data sidecars: {checked} user image(s), each present and well-formed");
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn insn(opcode: u8) -> u32 {
        ((opcode as u32) << 24) | 0x0012_0004
    }

    #[test]
    fn clean_words_pass() {
        let words = [insn(op::LD), insn(op::ST), insn(op::ADD), 0];
        assert!(scan_user_image("init", &words, false).is_ok());
    }

    #[test]
    fn every_forbidden_opcode_is_refused_with_its_name_and_word() {
        for (o, n) in [
            (op::XCHG, "XCHG"),
            (op::CAS, "CAS"),
            (op::MRET, "MRET"),
            (op::SRET, "SRET"),
            (op::LR_W, "LR_W"),
            (op::SC_W, "SC_W"),
        ] {
            let words = [insn(op::LD), insn(op::LD), insn(o)];
            let e = scan_user_image("sh", &words, false)
                .unwrap_err()
                .to_string();
            assert!(e.contains("UserImageOpcode"), "{e}");
            assert!(
                e.contains(n) && e.contains("word 2") && e.contains("sh"),
                "{e}"
            );
        }
    }

    #[test]
    fn the_hatch_admits_returns_only() {
        assert!(scan_user_image("p", &[insn(op::MRET), insn(op::SRET)], true).is_ok());
        for o in [
            op::XCHG,
            op::CAS,
            op::LR_W,
            op::SC_W,
            op::PUSH,
            op::POP,
            op::INT,
            op::IRET,
        ] {
            assert!(scan_user_image("p", &[insn(o)], true).is_err());
            assert!(scan_user_image("p", &[insn(o)], false).is_err());
        }
    }
}
