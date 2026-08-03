// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! mbc-emulate: Run MBC programs in the userspace emulator.
//!
//! Usage:
//!   mbc-emulate <input.mbc> [--max-insn N] [--dump-screen FILE] [--stats] [--trace]
//!
//! Reads MBC assembly text, assembles to bytecode, then runs the CPU emulator.
//! Reports instruction count, register state, and optionally dumps the screen buffer.

use monad_mbc::{assemble, ExecCpu, ExecError};
use std::env;
use std::fs;
use std::io::Write;
use std::process;

fn main() {
    let args: Vec<String> = env::args().collect();

    if args.len() < 2 || args[1] == "-h" || args[1] == "--help" {
        eprintln!("Usage: mbc-emulate <input.mbc> [options]");
        eprintln!();
        eprintln!("Run an MBC assembly program in the userspace emulator.");
        eprintln!();
        eprintln!("Options:");
        eprintln!("  --max-insn N       Maximum instructions to execute (default: 1000000)");
        eprintln!("  --dump-screen FILE Dump screen buffer to PPM file");
        eprintln!("  --stats            Print execution statistics");
        eprintln!("  --trace            Print each instruction as it executes");
        process::exit(if args.len() < 2 { 1 } else { 0 });
    }

    let input_path = &args[1];
    let mut max_insn: u64 = 1_000_000;
    let mut dump_screen: Option<String> = None;
    let mut show_stats = false;
    let mut trace = false;

    let mut i = 2;
    while i < args.len() {
        match args[i].as_str() {
            "--max-insn" => {
                if i + 1 >= args.len() {
                    eprintln!("Error: --max-insn requires an argument");
                    process::exit(1);
                }
                max_insn = args[i + 1].parse().unwrap_or_else(|_| {
                    eprintln!("Error: invalid --max-insn value: {}", args[i + 1]);
                    process::exit(1);
                });
                i += 2;
            }
            "--dump-screen" => {
                if i + 1 >= args.len() {
                    eprintln!("Error: --dump-screen requires a filename");
                    process::exit(1);
                }
                dump_screen = Some(args[i + 1].clone());
                i += 2;
            }
            "--stats" => {
                show_stats = true;
                i += 1;
            }
            "--trace" => {
                trace = true;
                i += 1;
            }
            other => {
                eprintln!("Error: unknown option '{other}'");
                process::exit(1);
            }
        }
    }

    // Read input — try as text assembly first, fall back to raw binary.
    let rom = match fs::read_to_string(input_path) {
        Ok(source) => {
            // Text file — assemble it.
            match assemble(&source) {
                Ok(words) => {
                    eprintln!(
                        "Assembled: {} instructions ({} bytes)",
                        words.len(),
                        words.len() * 4,
                    );
                    words
                }
                Err(e) => {
                    eprintln!("Assembly failed: {e}");
                    process::exit(1);
                }
            }
        }
        Err(_) => {
            // Not valid UTF-8 — treat as raw binary (little-endian u32 words).
            let bytes = match fs::read(input_path) {
                Ok(b) => b,
                Err(e) => {
                    eprintln!("Error reading '{input_path}': {e}");
                    process::exit(1);
                }
            };
            if bytes.len() % 4 != 0 {
                eprintln!(
                    "Warning: binary file size {} is not a multiple of 4",
                    bytes.len()
                );
            }
            let words: Vec<u32> = bytes
                .chunks(4)
                .map(|chunk| {
                    let mut buf = [0u8; 4];
                    buf[..chunk.len()].copy_from_slice(chunk);
                    u32::from_le_bytes(buf)
                })
                .collect();
            eprintln!(
                "Loaded binary: {} instructions ({} bytes)",
                words.len(),
                bytes.len(),
            );
            words
        }
    };

    // Create CPU and load ROM.
    let mut cpu = ExecCpu::new();
    cpu.load_rom(&rom);

    // Execute.
    let mut insn_count: u64 = 0;
    let mut syscall_count: u64 = 0;
    let mut halt_reason = "max instructions reached";

    while insn_count < max_insn {
        if trace && (cpu.state.pc as usize) < rom.len() {
            let pc = cpu.state.pc;
            let word = rom[pc as usize];
            eprintln!("  PC={pc:5}  insn=0x{word:08X}  r0={}", cpu.state.regs[0]);
        }

        match cpu.step() {
            Ok(()) => {
                insn_count += 1;
            }
            Err(ExecError::Halted) => {
                halt_reason = "HALT instruction";
                break;
            }
            Err(ExecError::RomFault) => {
                halt_reason = "ROM fault (PC out of bounds)";
                break;
            }
            Err(ExecError::InvalidSyscall(n)) => {
                syscall_count += 1;
                eprintln!("SYSCALL {n:#X} at PC={}", cpu.state.pc.wrapping_sub(1));
                insn_count += 1;
                // Non-fatal: continue execution
            }
            Err(ExecError::DivisionByZero) => {
                halt_reason = "division by zero";
                break;
            }
            Err(ExecError::CycleBudgetExhausted) => {
                halt_reason = "cycle budget exhausted";
                break;
            }
        }
    }

    // Print results.
    eprintln!();
    eprintln!("=== Execution Complete ===");
    eprintln!("Halt reason:    {halt_reason}");
    eprintln!("Instructions:   {insn_count}");
    eprintln!("Syscalls:       {syscall_count}");
    eprintln!("Final PC:       {}", cpu.state.pc);
    eprintln!("Registers:");
    for r in 0..16 {
        let val = cpu.state.regs[r];
        if val != 0 || r == 0 || r == 15 {
            eprintln!("  r{r:2} = {val:#10X} ({val})");
        }
    }

    if show_stats {
        eprintln!();
        eprintln!("Cache hits:     {}", cpu.state.cache_hits);
        eprintln!("Cache misses:   {}", cpu.state.cache_misses);
        eprintln!("Halted flag:    {}", cpu.state.halted);
        eprintln!("Stalled flag:   {}", cpu.state.stalled);

        // Count non-zero screen pixels.
        let nonzero_pixels = cpu.screen.iter().filter(|&&b| b != 0).count();
        eprintln!(
            "Screen pixels:  {nonzero_pixels}/64000 non-zero ({:.1}%)",
            nonzero_pixels as f64 / 64000.0 * 100.0
        );
    }

    // Dump screen buffer if requested.
    if let Some(path) = dump_screen {
        match dump_screen_ppm(&cpu.screen, &path) {
            Ok(()) => eprintln!("Screen dumped to {path}"),
            Err(e) => eprintln!("Error dumping screen: {e}"),
        }
    }

    // Exit with status based on halt reason.
    // Both a real HALT and hitting the instruction cap are successful ends of
    // a run; anything else (trap, decode failure) is a nonzero exit.
    if halt_reason == "HALT instruction" || halt_reason == "max instructions reached" {
        process::exit(0);
    } else {
        process::exit(1);
    }
}

/// Dump screen buffer as PPM (P6) image.
/// Screen is 320x200 pixels, each byte is a palette index.
/// Uses a basic grayscale palette for simplicity.
fn dump_screen_ppm(screen: &[u8], path: &str) -> std::io::Result<()> {
    let width = 320;
    let height = 200;
    let mut file = fs::File::create(path)?;
    write!(file, "P6\n{width} {height}\n255\n")?;

    // Simple palette: index maps to (index, index, index) grayscale.
    // For Doom, you'd use the real 256-color palette.
    for &pixel in screen.iter().take(width * height) {
        let r = pixel;
        let g = pixel;
        let b = pixel;
        file.write_all(&[r, g, b])?;
    }
    Ok(())
}
