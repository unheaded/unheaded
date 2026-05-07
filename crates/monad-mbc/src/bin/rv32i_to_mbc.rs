//! rv32i-to-mbc: Translate an RV32I ELF binary to MBC bytecode.
//!
//! Usage:
//!   rv32i-to-mbc <input.elf> [-o output.mbc] [--stats] [--disasm]
//!
//! Reads an RV32I ELF file, extracts the .text section (or all executable
//! PROGBITS sections), translates to MBC bytecode, and writes the output
//! as a flat binary of little-endian u32 words.

use goblin::elf::Elf;
use monad_mbc::Translator;
use std::env;
use std::fs;
use std::io::{self, Write};
use std::process;

fn main() {
    let args: Vec<String> = env::args().collect();

    if args.len() < 2 || args[1] == "-h" || args[1] == "--help" {
        eprintln!("Usage: rv32i-to-mbc <input.elf> [-o output.mbc] [--stats] [--disasm]");
        eprintln!();
        eprintln!("Translate an RV32I ELF binary to MBC (Monad Bytecode).");
        eprintln!();
        eprintln!("Options:");
        eprintln!("  -o <file>    Write output to file (default: stdout)");
        eprintln!("  --stats      Print translation statistics to stderr");
        eprintln!("  --disasm     Print MBC disassembly to stderr");
        eprintln!("  --sections   List ELF sections and exit");
        process::exit(if args.len() < 2 { 1 } else { 0 });
    }

    let input_path = &args[1];
    let mut output_path: Option<&str> = None;
    let mut show_stats = false;
    let mut show_disasm = false;
    let mut list_sections = false;

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
            "--stats" => {
                show_stats = true;
                i += 1;
            }
            "--disasm" => {
                show_disasm = true;
                i += 1;
            }
            "--sections" => {
                list_sections = true;
                i += 1;
            }
            other => {
                eprintln!("Error: unknown option '{other}'");
                process::exit(1);
            }
        }
    }

    // Read ELF file.
    let elf_data = match fs::read(input_path) {
        Ok(d) => d,
        Err(e) => {
            eprintln!("Error reading '{input_path}': {e}");
            process::exit(1);
        }
    };

    // Parse ELF.
    let elf = match Elf::parse(&elf_data) {
        Ok(e) => e,
        Err(e) => {
            eprintln!("Error parsing ELF: {e}");
            process::exit(1);
        }
    };

    // Validate: must be ELF32, little-endian, RISC-V.
    if elf.header.e_machine != goblin::elf::header::EM_RISCV {
        eprintln!(
            "Error: ELF machine type is {} (expected RISC-V/243)",
            elf.header.e_machine
        );
        process::exit(1);
    }
    if !elf.is_lib && !elf.header.e_ident[goblin::elf::header::EI_CLASS] == 1 {
        // Class 1 = ELF32
        eprintln!("Warning: expected ELF32 (class 1)");
    }

    if list_sections {
        eprintln!("ELF sections:");
        for (i, sh) in elf.section_headers.iter().enumerate() {
            let name = elf.shdr_strtab.get_at(sh.sh_name).unwrap_or("?");
            eprintln!(
                "  [{:2}] {:<20} type={:2} flags=0x{:x} offset=0x{:x} size=0x{:x}",
                i, name, sh.sh_type, sh.sh_flags, sh.sh_offset, sh.sh_size
            );
        }
        process::exit(0);
    }

    // Find .text section (or first executable PROGBITS).
    let text_section = elf
        .section_headers
        .iter()
        .find(|sh| {
            let name = elf.shdr_strtab.get_at(sh.sh_name).unwrap_or("");
            name == ".text"
        })
        .or_else(|| {
            // Fallback: first PROGBITS section with EXECINSTR flag.
            elf.section_headers.iter().find(|sh| {
                sh.sh_type == goblin::elf::section_header::SHT_PROGBITS
                    && (sh.sh_flags & goblin::elf::section_header::SHF_EXECINSTR as u64) != 0
            })
        });

    let text_sh = match text_section {
        Some(sh) => sh,
        None => {
            eprintln!("Error: no .text section found in ELF. Use --sections to list.");
            process::exit(1);
        }
    };

    let text_name = elf.shdr_strtab.get_at(text_sh.sh_name).unwrap_or(".text");
    let text_offset = text_sh.sh_offset as usize;
    let text_size = text_sh.sh_size as usize;

    if text_offset + text_size > elf_data.len() {
        eprintln!("Error: .text section extends beyond file bounds");
        process::exit(1);
    }

    if text_size % 4 != 0 {
        eprintln!("Warning: .text size ({text_size}) not word-aligned, truncating");
    }

    let text_bytes = &elf_data[text_offset..text_offset + text_size];
    let num_words = text_size / 4;

    eprintln!(
        "Section '{text_name}': {num_words} RV32I instructions ({text_size} bytes) at offset 0x{text_offset:x}"
    );

    // Convert bytes to u32 words (little-endian).
    let rv32i_words: Vec<u32> = text_bytes
        .chunks_exact(4)
        .map(|chunk| u32::from_le_bytes([chunk[0], chunk[1], chunk[2], chunk[3]]))
        .collect();

    // Translate (with RV-to-MBC address map for indirect jumps).
    match Translator::translate_program_with_map(&rv32i_words) {
        Ok((mbc_words, rv2mbc_map)) => {
            eprintln!(
                "Translation successful: {} RV32I → {} MBC instructions (expansion ratio: {:.1}x)",
                rv32i_words.len(),
                mbc_words.len(),
                mbc_words.len() as f64 / rv32i_words.len().max(1) as f64,
            );

            if show_stats {
                eprintln!(
                    "  ROM usage: {} / 65536 words ({:.1}%)",
                    mbc_words.len(),
                    mbc_words.len() as f64 / 65536.0 * 100.0,
                );
                eprintln!("  ROM size: {} bytes", mbc_words.len() * 4);
                eprintln!(
                    "  RV2MBC map: {} entries ({} bytes)",
                    rv2mbc_map.len(),
                    rv2mbc_map.len() * 4,
                );
            }

            if show_disasm {
                eprintln!("\nMBC Disassembly:");
                let listing = monad_mbc::disasm_listing(&mbc_words);
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

                // Write RV2MBC address translation map alongside the MBC output.
                // e.g., "doom.mbc" → "doom.rv2mbc"
                let rv2mbc_path = if path.ends_with(".mbc") {
                    path.replace(".mbc", ".rv2mbc")
                } else {
                    format!("{path}.rv2mbc")
                };
                let rv2mbc_bytes: Vec<u8> =
                    rv2mbc_map.iter().flat_map(|w| w.to_le_bytes()).collect();
                if let Err(e) = fs::write(&rv2mbc_path, &rv2mbc_bytes) {
                    eprintln!("Error writing rv2mbc map: {e}");
                    process::exit(1);
                }
                eprintln!(
                    "Wrote {} entries ({} bytes) to {rv2mbc_path}",
                    rv2mbc_map.len(),
                    rv2mbc_bytes.len(),
                );
            } else {
                let stdout = io::stdout();
                let mut handle = stdout.lock();
                if let Err(e) = handle.write_all(&mbc_bytes) {
                    eprintln!("Error writing to stdout: {e}");
                    process::exit(1);
                }
                // When writing to stdout, rv2mbc map is not written (no file path).
                eprintln!("Note: rv2mbc map not written (use -o to specify output file)");
            }
        }
        Err(errors) => {
            eprintln!("Translation failed with {} error(s):", errors.len());
            for (i, e) in errors.iter().enumerate().take(50) {
                eprintln!("  [{i}] {e}");
            }
            if errors.len() > 50 {
                eprintln!("  ... and {} more", errors.len() - 50);
            }

            // Print summary of error types.
            let mut unsupported_regs = 0;
            let mut unsupported_insns = 0;
            let mut unsupported_features = 0;
            let mut other = 0;
            for e in &errors {
                match e {
                    monad_mbc::TranslateError::UnsupportedRegister { .. } => unsupported_regs += 1,
                    monad_mbc::TranslateError::UnsupportedInstruction { .. } => {
                        unsupported_insns += 1
                    }
                    monad_mbc::TranslateError::UnsupportedFeature { .. } => {
                        unsupported_features += 1
                    }
                    _ => other += 1,
                }
            }
            eprintln!("\nError summary:");
            if unsupported_regs > 0 {
                eprintln!("  Unsupported registers (x16+): {unsupported_regs}");
            }
            if unsupported_insns > 0 {
                eprintln!("  Unsupported instructions: {unsupported_insns}");
            }
            if unsupported_features > 0 {
                eprintln!("  Unsupported features: {unsupported_features}");
            }
            if other > 0 {
                eprintln!("  Other errors: {other}");
            }
            process::exit(1);
        }
    }
}
