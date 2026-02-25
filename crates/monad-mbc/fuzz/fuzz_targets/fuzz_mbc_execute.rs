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
    for i in 0..16 {
        let offset = i * 4;
        let word = u32::from_le_bytes([
            reg_bytes[offset],
            reg_bytes[offset + 1],
            reg_bytes[offset + 2],
            reg_bytes[offset + 3],
        ]);
        cpu.state.regs[i] = word;
    }

    // Build ROM from remaining bytes
    let rom: Vec<u32> = rom_bytes
        .chunks_exact(4)
        .map(|c| u32::from_le_bytes([c[0], c[1], c[2], c[3]]))
        .collect();

    if rom.is_empty() {
        return;
    }

    cpu.load_rom(&rom);

    // Run up to 10,000 cycles -- must not panic
    let _ = cpu.run(10_000);
});
