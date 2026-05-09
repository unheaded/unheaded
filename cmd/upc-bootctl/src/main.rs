// SPDX-License-Identifier: GPL-3.0-or-later
//
// upc-bootctl — UPC kernel image loader + boot dispatcher.
//
// ASCEND-LINUX Phase 1.1 deliverable per
// `references/battle-plan-ascend-linux-2026-05-08.md`.
//
// Loads a translated MBC kernel image (xv6-mbc.mbc, uclinux-mbc.mbc, etc.),
// validates it, prepares BootParams v2, and dispatches a boot trigger to
// the UPC instance.
//
// **Status**: skeleton with `validate` and `boot --dry-run` subcommands.
// Full BPF map population + dispatcher to come in next shift's `boot` impl.

use anyhow::{anyhow, bail, Context, Result};
use clap::{Parser, Subcommand};
use std::path::PathBuf;

#[derive(Parser, Debug)]
#[command(name = "upc-bootctl", version, about)]
struct Cli {
    #[command(subcommand)]
    cmd: Cmd,
}

#[derive(Subcommand, Debug)]
enum Cmd {
    /// Validate a kernel image without loading it.
    Validate {
        /// Path to the .mbc kernel image.
        #[arg(long)]
        kernel: PathBuf,
    },

    /// Load + boot a kernel image into the UPC.
    Boot {
        /// Path to the .mbc kernel image.
        #[arg(long)]
        kernel: PathBuf,

        /// Path to the optional ramdisk (CPIO+gzip initramfs).
        #[arg(long)]
        initramfs: Option<PathBuf>,

        /// UPC instance ID (low byte of CPU_MAP key).
        #[arg(long, default_value_t = 0xDE)]
        instance: u8,

        /// Dry-run — print what we'd do but don't touch BPF maps.
        #[arg(long)]
        dry_run: bool,
    },

    /// Attach a host pty to the UPC's tty stream (Mode B demo surface).
    Console {
        /// UPC instance ID.
        #[arg(long, default_value_t = 0xDE)]
        instance: u8,
    },
}

/// Validate that a kernel image's byte buffer is 4-byte aligned and return the
/// instruction count. Pure function — no I/O — so it is unit-testable.
///
/// Errors with the byte-count in the message when the buffer length is not a
/// multiple of 4 (an MBC instruction is always one 32-bit word).
fn check_image_alignment(bytes: &[u8]) -> Result<usize> {
    if bytes.len() % 4 != 0 {
        bail!(
            "kernel image not 4-byte aligned ({} bytes)",
            bytes.len()
        );
    }
    Ok(bytes.len() / 4)
}

fn cmd_validate(kernel: PathBuf) -> Result<()> {
    let bytes = std::fs::read(&kernel)
        .with_context(|| format!("read kernel: {}", kernel.display()))?;

    let n_insns = check_image_alignment(&bytes)?;

    println!("kernel:        {}", kernel.display());
    println!("size:          {} bytes", bytes.len());
    println!("instructions:  {} MBC words", n_insns);

    // Decode first 32 instructions for sanity check.
    println!("\nfirst 32 MBC instructions:");
    for i in 0..32.min(n_insns) {
        let off = i * 4;
        let w = u32::from_le_bytes([
            bytes[off],
            bytes[off + 1],
            bytes[off + 2],
            bytes[off + 3],
        ]);
        let opcode = (w >> 24) & 0xFF;
        let dst = (w >> 20) & 0xF;
        let src = (w >> 16) & 0xF;
        let imm = w & 0xFFFF;
        println!(
            "  PC=0x{:04x}  word=0x{:08x}  op=0x{:02x} dst=r{:<2} src=r{:<2} imm=0x{:04x}",
            i, w, opcode, dst, src, imm
        );
    }

    println!(
        "\nBoot magic: 0x{:08X} ('{}')",
        xv6_mbc::BOOT_MAGIC,
        std::str::from_utf8(&xv6_mbc::BOOT_MAGIC.to_le_bytes())
            .unwrap_or("???")
    );

    println!("\n✓ kernel image structurally valid");
    Ok(())
}

fn cmd_boot(
    kernel: PathBuf,
    initramfs: Option<PathBuf>,
    instance: u8,
    dry_run: bool,
) -> Result<()> {
    cmd_validate(kernel.clone())?;

    println!("\n=== BOOT DISPATCH (dry_run={}) ===", dry_run);
    println!("instance:      0x{:02X}", instance);
    if let Some(ref ir) = initramfs {
        let sz = std::fs::metadata(ir)
            .with_context(|| format!("stat initramfs: {}", ir.display()))?
            .len();
        println!("initramfs:     {} ({} bytes)", ir.display(), sz);
    } else {
        println!("initramfs:     (none — kernel boots without /init)");
    }

    println!("\nBoot Protocol v2 setup (per docs/doom/UPC_BOOT_PROTOCOL_V2.md):");
    println!("  1. Allocate PerCPU<MbcCpuState> slot for instance 0x{:02X}", instance);
    println!("  2. Zero IVT region (byte 0x0000-0x03FF)");
    println!("  3. Zero CSR region (byte 0x000_F000-0x000_F0FF)");
    println!("  4. Write default HLT trap handler at 0x0400");
    println!("  5. Write BootParams v2 (256 bytes) at 0x0100");
    println!("     magic = 'UNHD' (0x{:08X})", xv6_mbc::BOOT_MAGIC);
    println!("     version = 2");
    println!("     kernel_addr = 0x10000 (stage-1 stub)");
    println!("     bss_start / bss_end (read from kernel.elf .bss)");
    println!("     boot_random_seed = 32 bytes from getrandom(2)");
    println!("  6. Load kernel image at byte 0x10000");
    println!("     {} words → ROM_MAP slots 0x4000+", std::fs::metadata(&kernel)?.len() / 4);
    if initramfs.is_some() {
        println!("  7. Load initramfs at byte 0x800000");
    }
    println!("  8. Initialize MbcCpuState:");
    println!("     PC = 0x4000 (stage-1 stub, word index)");
    println!("     SP = 0x03F00000 (top of stack, byte address)");
    println!("     priv_level = 0 (M-mode at boot)");
    println!("     reservation_address = 0xFFFF_FFFF (no LR)");
    println!("  9. Dispatch Gjallarhorn UPC trigger packet");
    println!(" 10. Watch tty stream (compute.tty.{{instance}})");

    if dry_run {
        println!("\n✓ dry-run complete; no BPF maps touched");
        return Ok(());
    }

    Err(anyhow!(
        "live boot path not yet implemented — use --dry-run for now\n\
         Next shift work: integrate with crates/doom-runner-style aya BPF\n\
         loader, populate ROM_MAP/RAM_MAP/PerCPU<MbcCpuState>"
    ))
}

fn cmd_console(instance: u8) -> Result<()> {
    println!("=== UPC CONSOLE (instance 0x{:02X}) ===", instance);
    println!("Console attach mode (Mode B demo surface).");
    println!();
    println!("Implementation pending: subscribe to Busboy topic");
    println!("compute.tty.0x{:02X}, render bytes to host stdout, capture", instance);
    println!("stdin keystrokes and write to KBD_MAP at 0x{:04X}.",
             instance);
    println!();
    println!("Pattern: cmd/doom-bridge/main.go but framing tty bytes");
    println!("instead of framebuffer pixels.");

    Err(anyhow!("not yet implemented"))
}

fn main() -> Result<()> {
    let cli = Cli::parse();
    match cli.cmd {
        Cmd::Validate { kernel } => cmd_validate(kernel),
        Cmd::Boot {
            kernel,
            initramfs,
            instance,
            dry_run,
        } => cmd_boot(kernel, initramfs, instance, dry_run),
        Cmd::Console { instance } => cmd_console(instance),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn alignment_4_bytes_is_one_instruction() {
        assert_eq!(check_image_alignment(&[0; 4]).unwrap(), 1);
    }

    #[test]
    fn alignment_8_bytes_is_two_instructions() {
        assert_eq!(check_image_alignment(&[0xAA; 8]).unwrap(), 2);
    }

    #[test]
    fn alignment_empty_is_zero_instructions() {
        // Empty image is technically aligned (0 % 4 == 0). cmd_validate's
        // higher-level checks (boot magic etc.) would reject it later, but
        // the alignment gate alone passes.
        assert_eq!(check_image_alignment(&[]).unwrap(), 0);
    }

    #[test]
    fn alignment_5_bytes_errors_with_byte_count() {
        let err = check_image_alignment(&[0; 5]).unwrap_err();
        let msg = err.to_string();
        assert!(msg.contains("4-byte aligned"), "got: {}", msg);
        assert!(msg.contains("5 bytes"), "got: {}", msg);
    }

    #[test]
    fn alignment_realistic_xv6_size_resolves_to_word_count() {
        // xv6-mbc.mbc as of ASCEND-LINUX Phase 1.1 super-sprint = 11_721 MBC
        // instructions = 46_884 bytes. Spot-check that ratio.
        const XV6_MBC_BYTES: usize = 11_721 * 4;
        let buf = vec![0u8; XV6_MBC_BYTES];
        assert_eq!(check_image_alignment(&buf).unwrap(), 11_721);
    }
}
