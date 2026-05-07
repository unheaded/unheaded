// SPDX-License-Identifier: GPL-3.0-only
//! ELF-aware section loader for Doom-over-IPv6.
//!
//! Parses `doom.elf` with `goblin`, extracts sections, and loads them into the
//! appropriate BPF maps via Aya:
//!
//! - `.text`    -> ROM_MAP (as MBC instructions, via rv2mbc translation)
//! - `.data`    -> RAM_MAP at section's link address
//! - `.sdata`   -> RAM_MAP at section's link address
//! - `.rodata`  -> RAM_MAP at section's link address (also in ROM for code, but
//!                 Doom's rodata is data-referenced, so it needs to be in RAM too)
//! - `.srodata` -> RAM_MAP at section's link address
//! - `.bss`     -> RAM_MAP zeroed (BPF Array is zero-init, but we verify the range)
//! - WAD file   -> RAM_MAP at WAD_BASE
//!
//! The CPU state is initialized in CPU_MAP with PC=0, SP=STACK_TOP, registers zeroed.

use std::path::Path;

use anyhow::{bail, Context, Result};
use byteorder::{LittleEndian, ReadBytesExt};
use goblin::elf::Elf;
use tracing::{debug, info};

use crate::memory;

/// Parsed ELF sections ready for loading into BPF maps.
#[derive(Debug)]
pub struct DoomElf {
    /// MBC ROM instructions (translated from .text via rv2mbc).
    /// Each entry is one u32 MBC instruction word.
    pub rom: Vec<u32>,

    /// RAM regions to load: (byte_address, data).
    /// Each entry is a contiguous region at a specific address.
    pub ram_regions: Vec<RamRegion>,

    /// BSS range to zero: (start_byte_addr, length_bytes).
    /// Not strictly necessary (BPF Array is zero-init) but validates the range.
    pub bss_start: u32,
    pub bss_len: u32,

    /// RV32I-to-MBC address translation table.
    /// Index = RV32I word index (byte_addr >> 2), Value = MBC PC index.
    pub rv2mbc: Vec<u32>,

    /// Entry point PC (MBC index, not byte address).
    pub entry_pc: u32,
}

/// A contiguous region of data to load into RAM_MAP.
#[derive(Debug)]
pub struct RamRegion {
    /// Name of the ELF section (for logging).
    pub name: String,
    /// Byte address in the MBC address space where this region starts.
    pub addr: u32,
    /// Raw bytes to write (will be word-aligned and packed into u32s).
    pub data: Vec<u8>,
}

/// Parse a doom.elf file and extract all sections for loading.
///
/// The ELF must be a RISC-V 32-bit executable linked with our linker.ld.
/// The .text section contains RV32I instructions that have already been
/// translated to MBC via the monad-mbc crate — here we just load the
/// pre-translated .mbc binary for ROM.
pub fn parse_elf(elf_path: &Path) -> Result<ParsedSections> {
    let elf_bytes = std::fs::read(elf_path)
        .with_context(|| format!("failed to read ELF: {}", elf_path.display()))?;
    let elf = Elf::parse(&elf_bytes).context("failed to parse ELF")?;

    info!(
        "parsed ELF: {} sections, entry=0x{:08x}, machine={:#x}",
        elf.section_headers.len(),
        elf.entry,
        elf.header.e_machine,
    );

    let mut data_sections = Vec::new();
    let mut bss_start: Option<u32> = None;
    let mut bss_len: u32 = 0;

    for sh in &elf.section_headers {
        let name = elf.shdr_strtab.get_at(sh.sh_name).unwrap_or("<unknown>");
        let addr = sh.sh_addr as u32;
        let size = sh.sh_size as u32;
        let file_offset = sh.sh_offset as usize;
        let file_size = sh.sh_size as usize;

        match name {
            n if matches!(n, ".data" | ".sdata" | ".rodata" | ".srodata")
                || n.starts_with(".srodata.") =>
            {
                if file_size == 0 {
                    debug!("skipping empty section: {name}");
                    continue;
                }
                let end = file_offset + file_size;
                if end > elf_bytes.len() {
                    bail!(
                        "section {name} extends past EOF: offset={file_offset:#x} \
                         size={file_size:#x} file_len={:#x}",
                        elf_bytes.len()
                    );
                }
                let data = elf_bytes[file_offset..end].to_vec();
                info!("section {name}: addr=0x{addr:08x} size={size} ({size:#x}) bytes",);
                data_sections.push(RamRegion {
                    name: name.to_string(),
                    addr,
                    data,
                });
            }
            ".bss" | ".sbss" => {
                info!(
                    "section {name}: addr=0x{addr:08x} size={size} ({size:#x}) bytes (zero-init)"
                );
                match bss_start {
                    None => {
                        bss_start = Some(addr);
                        bss_len = size;
                    }
                    Some(start) => {
                        // Extend BSS to cover this section too
                        let new_end = addr + size;
                        let old_end = start + bss_len;
                        let combined_end = new_end.max(old_end);
                        let combined_start = start.min(addr);
                        bss_len = combined_end - combined_start;
                        bss_start = Some(combined_start);
                    }
                }
            }
            ".text" => {
                info!(
                    "section .text: addr=0x{addr:08x} size={size} bytes \
                     (will be loaded from .mbc binary, not from ELF .text)"
                );
            }
            _ => {
                debug!("ignoring section: {name} (addr=0x{addr:08x} size={size})");
            }
        }
    }

    Ok(ParsedSections {
        data_sections,
        bss_start: bss_start.unwrap_or(0),
        bss_len,
        entry_rv_addr: elf.entry as u32,
    })
}

/// Sections extracted from the ELF, ready for map loading.
#[derive(Debug)]
pub struct ParsedSections {
    /// Data sections to load into RAM_MAP.
    pub data_sections: Vec<RamRegion>,
    /// BSS start address.
    pub bss_start: u32,
    /// BSS length in bytes.
    pub bss_len: u32,
    /// ELF entry point (RV32I byte address — needs rv2mbc translation for PC).
    pub entry_rv_addr: u32,
}

/// Load the pre-translated MBC binary into a ROM vector.
///
/// The .mbc file is a flat binary of little-endian u32 MBC instructions,
/// produced by `monad-mbc translate`.
pub fn load_mbc_rom(mbc_path: &Path) -> Result<Vec<u32>> {
    let mbc_bytes = std::fs::read(mbc_path)
        .with_context(|| format!("failed to read MBC binary: {}", mbc_path.display()))?;

    if mbc_bytes.len() % 4 != 0 {
        bail!(
            "MBC binary size ({}) is not a multiple of 4",
            mbc_bytes.len()
        );
    }

    let num_insns = mbc_bytes.len() / 4;
    if num_insns > memory::ROM_MAP_ENTRIES as usize {
        bail!(
            "MBC binary has {} instructions but ROM_MAP only holds {}",
            num_insns,
            memory::ROM_MAP_ENTRIES,
        );
    }

    let mut rom = Vec::with_capacity(num_insns);
    let mut cursor = std::io::Cursor::new(&mbc_bytes);
    for _ in 0..num_insns {
        rom.push(cursor.read_u32::<LittleEndian>()?);
    }

    info!(
        "loaded {num_insns} MBC instructions from {}",
        mbc_path.display()
    );
    Ok(rom)
}

/// Load the RV32I-to-MBC address translation table.
///
/// The .rv2mbc file is a flat binary of little-endian u32 values:
/// index = RV32I word index (byte_addr >> 2), value = MBC PC index.
pub fn load_rv2mbc(rv2mbc_path: &Path) -> Result<Vec<u32>> {
    let bytes = std::fs::read(rv2mbc_path)
        .with_context(|| format!("failed to read rv2mbc: {}", rv2mbc_path.display()))?;

    if bytes.len() % 4 != 0 {
        bail!("rv2mbc file size ({}) is not a multiple of 4", bytes.len());
    }

    let count = bytes.len() / 4;
    if count > memory::RV2MBC_MAP_ENTRIES as usize {
        bail!(
            "rv2mbc has {} entries but RV2MBC_MAP only holds {}",
            count,
            memory::RV2MBC_MAP_ENTRIES,
        );
    }

    let mut table = Vec::with_capacity(count);
    let mut cursor = std::io::Cursor::new(&bytes);
    for _ in 0..count {
        table.push(cursor.read_u32::<LittleEndian>()?);
    }

    info!(
        "loaded {count} rv2mbc entries from {}",
        rv2mbc_path.display()
    );
    Ok(table)
}

/// Load the WAD file into a byte vector, validating size.
pub fn load_wad(wad_path: &Path) -> Result<Vec<u8>> {
    let wad_bytes = std::fs::read(wad_path)
        .with_context(|| format!("failed to read WAD: {}", wad_path.display()))?;

    if wad_bytes.len() > memory::WAD_MAX_SIZE as usize {
        bail!(
            "WAD file is {} bytes but max is {} (WAD_MAX_SIZE)",
            wad_bytes.len(),
            memory::WAD_MAX_SIZE,
        );
    }

    info!(
        "loaded WAD: {} bytes ({:.1} MiB) from {}",
        wad_bytes.len(),
        wad_bytes.len() as f64 / (1024.0 * 1024.0),
        wad_path.display(),
    );
    Ok(wad_bytes)
}

/// Pack a byte slice into word-aligned u32 values (little-endian).
/// Pads the final word with zeros if the data length is not a multiple of 4.
pub fn bytes_to_words(data: &[u8]) -> Vec<u32> {
    let mut words = Vec::with_capacity((data.len() + 3) / 4);
    for chunk in data.chunks(4) {
        let mut word_bytes = [0u8; 4];
        word_bytes[..chunk.len()].copy_from_slice(chunk);
        words.push(u32::from_le_bytes(word_bytes));
    }
    words
}

/// Compute the initial CPU state for a Doom instance.
#[derive(Debug, Clone)]
pub struct InitialCpuState {
    /// General-purpose registers (r0-r15), all zeroed except SP.
    pub regs: [u32; 16],
    /// Program counter (MBC instruction index).
    pub pc: u32,
    /// CPU flags (Z, N, C) — zeroed.
    pub flags: u32,
    /// Halted flag — 0 (running).
    pub halted: u32,
}

impl InitialCpuState {
    /// Create initial state for a Doom instance.
    /// PC = entry_pc (from rv2mbc of ELF entry), SP = STACK_TOP, everything else zeroed.
    pub fn new(entry_pc: u32) -> Self {
        let mut regs = [0u32; 16];
        regs[15] = memory::STACK_TOP; // r15 = SP
                                      // r10 (a0) = heap start address (Doom's Z_Init argument)
        regs[10] = memory::HEAP_START;
        // r11 (a1) = heap size
        regs[11] = memory::HEAP_SIZE;

        Self {
            regs,
            pc: entry_pc,
            flags: 0,
            halted: 0,
        }
    }
}

/// Write data sections into a RAM word buffer.
///
/// This is the "staging" step — we build a complete RAM image in userspace,
/// then bulk-write it to the BPF map. This avoids thousands of individual
/// map update syscalls.
pub fn stage_ram_image(sections: &[RamRegion], wad_data: Option<&[u8]>) -> Result<Vec<(u32, u32)>> {
    let mut updates: Vec<(u32, u32)> = Vec::new();

    for region in sections {
        let words = bytes_to_words(&region.data);
        let base_word = region.addr / 4;
        info!(
            "staging {}: {} words at word index {base_word:#x} (byte addr {:#010x})",
            region.name,
            words.len(),
            region.addr,
        );

        // Validate the region fits in RAM_MAP
        let end_word = base_word + words.len() as u32;
        if end_word > memory::RAM_MAP_ENTRIES {
            bail!(
                "section {} at {:#010x} + {} bytes exceeds RAM_MAP capacity",
                region.name,
                region.addr,
                region.data.len(),
            );
        }

        for (i, &word) in words.iter().enumerate() {
            if word != 0 {
                // Skip zero words — BPF Array is zero-initialized
                updates.push((base_word + i as u32, word));
            }
        }
    }

    // Load WAD data into RAM at WAD_BASE
    if let Some(wad) = wad_data {
        let wad_words = bytes_to_words(wad);
        let wad_base_word = memory::WAD_BASE / 4;
        info!(
            "staging WAD: {} words at word index {wad_base_word:#x} (byte addr {:#010x})",
            wad_words.len(),
            memory::WAD_BASE,
        );

        let end_word = wad_base_word + wad_words.len() as u32;
        if end_word > memory::RAM_MAP_ENTRIES {
            bail!(
                "WAD at {:#010x} + {} bytes exceeds RAM_MAP capacity",
                memory::WAD_BASE,
                wad.len(),
            );
        }

        for (i, &word) in wad_words.iter().enumerate() {
            if word != 0 {
                updates.push((wad_base_word + i as u32, word));
            }
        }
    }

    info!("staged {} non-zero word updates for RAM_MAP", updates.len());
    Ok(updates)
}

/// Populate the program break value at the expected address.
///
/// Doom's sbrk() implementation reads the program break from a known location.
/// We set it to HEAP_START so the first sbrk(n) starts allocating from there.
pub fn heap_break_word() -> (u32, u32) {
    // DEFAULT_PROGRAM_BREAK in monad-common points to the initial brk value.
    // For doom-runner, we store it at a well-known address that the CPU reads.
    // The word index for HEAP_START:
    let break_addr = memory::HEAP_START;
    (break_addr / 4, break_addr)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn bytes_to_words_empty() {
        assert!(bytes_to_words(&[]).is_empty());
    }

    #[test]
    fn bytes_to_words_exact() {
        let data = [0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08];
        let words = bytes_to_words(&data);
        assert_eq!(words, vec![0x04030201, 0x08070605]);
    }

    #[test]
    fn bytes_to_words_partial() {
        let data = [0x01, 0x02, 0x03];
        let words = bytes_to_words(&data);
        assert_eq!(words, vec![0x00030201]);
    }

    #[test]
    fn initial_cpu_state_sp() {
        let state = InitialCpuState::new(0);
        assert_eq!(state.regs[15], memory::STACK_TOP);
        assert_eq!(state.regs[10], memory::HEAP_START);
        assert_eq!(state.regs[11], memory::HEAP_SIZE);
    }

    #[test]
    fn heap_break_is_at_heap_start() {
        let (_, value) = heap_break_word();
        assert_eq!(value, memory::HEAP_START);
    }
}
