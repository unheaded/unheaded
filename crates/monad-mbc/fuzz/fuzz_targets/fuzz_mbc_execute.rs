// SPDX-License-Identifier: GPL-3.0-or-later
#![no_main]

use libfuzzer_sys::fuzz_target;
use monad_mbc::execute::Cpu;

fuzz_target!(|data: &[u8]| {
    // Layout of fuzzer input:
    //   First 64 bytes: initial register values (16 x u32 LE)
    //   Remaining bytes: ROM instruction words (groups of 4 bytes, LE u32)
    //
    // The CPU must never panic regardless of input.

    if data.len() < 64 + 4 {
        // Need at least 16 registers + 1 instruction
        return;
    }

    let (reg_bytes, rom_bytes) = data.split_at(64);

    let mut cpu = Cpu::new();

    // Set initial registers from fuzz data
    for (r, c) in cpu.state.regs.iter_mut().zip(reg_bytes.as_chunks::<4>().0) {
        *r = u32::from_le_bytes(*c);
    }

    // Build ROM from remaining bytes
    let rom: Vec<u32> = rom_bytes
        .as_chunks::<4>()
        .0
        .iter()
        .map(|c| u32::from_le_bytes(*c))
        .collect();

    if rom.is_empty() {
        return;
    }

    cpu.load_rom(&rom);

    // Run up to 10,000 cycles -- must not panic
    let _ = cpu.run(10_000);
});
