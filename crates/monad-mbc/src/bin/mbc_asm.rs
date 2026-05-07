//! mbc-asm: Assemble MBC assembly source files into bytecode.
//!
//! Usage:
//!   mbc-asm <input.mbc> [-o output.bin] [--disasm] [--stats]
//!
//! Reads MBC assembly text, assembles to bytecode, and writes output
//! as a flat binary of little-endian u32 words.

use monad_mbc::{assemble, disasm_listing};
use std::env;
use std::fs;
use std::io::{self, Write};
use std::process;

fn main() {
    let args: Vec<String> = env::args().collect();

    if args.len() < 2 || args[1] == "-h" || args[1] == "--help" {
        eprintln!("Usage: mbc-asm <input.mbc> [-o output.bin] [--disasm] [--stats]");
        eprintln!();
        eprintln!("Assemble MBC assembly source into bytecode.");
        eprintln!();
        eprintln!("Options:");
        eprintln!("  -o <file>    Write output to file (default: stdout)");
        eprintln!("  --disasm     Print MBC disassembly to stderr");
        eprintln!("  --stats      Print assembly statistics to stderr");
        process::exit(if args.len() < 2 { 1 } else { 0 });
    }

    let input_path = &args[1];
    let mut output_path: Option<&str> = None;
    let mut show_disasm = false;
    let mut show_stats = false;

    let mut i = 2;
    while i < args.len() {
        match args[i].as_str() {
            "-o" => {
                if i + 1 >= args.len() {
                    eprintln!("Error: -o requires an argument");
                    process::exit(1);
                }
                output_path = Some(&args[i + 1]);
                i += 2;
            }
            "--disasm" => {
                show_disasm = true;
                i += 1;
            }
            "--stats" => {
                show_stats = true;
                i += 1;
            }
            other => {
                eprintln!("Error: unknown option '{other}'");
                process::exit(1);
            }
        }
    }

    // Read source file.
    let source = match fs::read_to_string(input_path) {
        Ok(s) => s,
        Err(e) => {
            eprintln!("Error reading '{input_path}': {e}");
            process::exit(1);
        }
    };

    // Assemble.
    match assemble(&source) {
        Ok(mbc_words) => {
            eprintln!(
                "Assembly successful: {} MBC instructions ({} bytes)",
                mbc_words.len(),
                mbc_words.len() * 4,
            );

            if show_stats {
                eprintln!(
                    "  ROM usage: {} / 65536 words ({:.1}%)",
                    mbc_words.len(),
                    mbc_words.len() as f64 / 65536.0 * 100.0,
                );
            }

            if show_disasm {
                eprintln!("\nMBC Disassembly:");
                let listing = disasm_listing(&mbc_words);
                eprintln!("{listing}");
            }

            // Write output.
            let mbc_bytes: Vec<u8> = mbc_words.iter().flat_map(|w| w.to_le_bytes()).collect();

            if let Some(path) = output_path {
                if let Err(e) = fs::write(path, &mbc_bytes) {
                    eprintln!("Error writing output: {e}");
                    process::exit(1);
                }
                eprintln!("Wrote {} bytes to {path}", mbc_bytes.len());
            } else {
                let stdout = io::stdout();
                let mut handle = stdout.lock();
                if let Err(e) = handle.write_all(&mbc_bytes) {
                    eprintln!("Error writing to stdout: {e}");
                    process::exit(1);
                }
            }
        }
        Err(e) => {
            eprintln!("Assembly failed: {e}");
            process::exit(1);
        }
    }
}
