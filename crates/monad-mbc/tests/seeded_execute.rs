// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//! M-186.11b (decision pack row 41): the body of the libFuzzer target
//! `fuzz/fuzz_targets/fuzz_mbc_execute.rs` (`execute::run_fuzz_input`), fed
//! from a seeded generator so it runs under plain `cargo test`, with no fuzzer
//! and no new dependency. No coverage guidance: weaker than libFuzzer, and a
//! test that runs on every `cargo test` instead of a SKIP.
//!
//! Each case is 64 bytes of registers then 1-256 ROM words. Registers are
//! drawn to land in RAM, near the top of the address space, or anywhere;
//! three ROM words in four carry a defined opcode. Any panic is a finding:
//! the message names the run seed, the case and the input bytes, and
//! `MBC_SEEDED_SEED=<seed> MBC_SEEDED_CASES=<case+1>` reproduces it.
//!
//! The committed seeds under `fuzz/seeds/fuzz_mbc_execute/` run first.
//!
//! Knobs (environment, test-only): `MBC_SEEDED_SEED` (decimal or 0x-hex,
//! default below), `MBC_SEEDED_CASES` (default 50,000).

use monad_common::{mbc_interrupts as intr, mbc_opcodes as op};
use monad_mbc::execute::run_fuzz_input;
use monad_mbc::is_valid_opcode;
use std::panic::{catch_unwind, AssertUnwindSafe};

const DEFAULT_SEED: u64 = 0x5EED_2026_1122_0186;
const DEFAULT_CASES: u64 = 50_000;

/// splitmix64: small, std-only, and every seed gives a distinct stream.
struct SplitMix(u64);

impl SplitMix {
    fn next(&mut self) -> u64 {
        self.0 = self.0.wrapping_add(0x9E37_79B9_7F4A_7C15);
        let mut z = self.0;
        z = (z ^ (z >> 30)).wrapping_mul(0xBF58_476D_1CE4_E5B9);
        z = (z ^ (z >> 27)).wrapping_mul(0x94D0_49BB_1331_11EB);
        z ^ (z >> 31)
    }
    fn below(&mut self, n: u64) -> u64 {
        self.next() % n
    }
}

fn env_u64(name: &str, default: u64) -> u64 {
    match std::env::var(name) {
        Ok(v) => {
            let v = v.trim();
            let parsed = match v.strip_prefix("0x").or_else(|| v.strip_prefix("0X")) {
                Some(h) => u64::from_str_radix(h, 16),
                None => v.parse(),
            };
            parsed.unwrap_or_else(|_| panic!("{name}={v:?} is not a number"))
        }
        Err(_) => default,
    }
}

fn gen_reg(r: &mut SplitMix) -> u32 {
    match r.below(5) {
        0 => r.below(512) as u32,                         // syscall numbers, counts
        1 => r.below(0x1_0000) as u32,                    // low RAM, the CSR window
        2 => r.below(0x0C0_0000) as u32 & !3,             // anywhere in RAM, aligned
        3 => 0u32.wrapping_sub(r.below(0x1_0000) as u32), // near the top
        _ => r.next() as u32,
    }
}

fn gen_word(r: &mut SplitMix) -> u32 {
    if r.below(4) == 0 {
        return r.next() as u32;
    }
    // dst and src registers, then imm16: small half the time (INT vectors,
    // short offsets), otherwise anything.
    let regs = (r.next() as u32 & 0xFF) << 16;
    let imm = if r.below(2) == 0 {
        r.below(256) as u32
    } else {
        r.next() as u32 & 0xFFFF
    };
    loop {
        let opc = r.below(256) as u8;
        if is_valid_opcode(opc) {
            return (u32::from(opc) << 24) | regs | imm;
        }
    }
}

const ST: u32 = op::ST as u32;
const INT: u32 = op::INT as u32;
const SYSCALL: u32 = op::SYSCALL as u32;
const VECTOR_SYSCALL: u32 = intr::VECTOR_SYSCALL as u32;

fn gen_case(r: &mut SplitMix) -> Vec<u8> {
    let words = 1 + r.below(256) as usize;
    let mut regs: Vec<u32> = (0..16).map(|_| gen_reg(r)).collect();
    let mut rom = Vec::with_capacity(words + 5);
    // One case in three is syscall-shaped: r0 a call number, r1-r3 aligned RAM
    // addresses, up to four stores of r4-r15 through them, then the trap. Pure
    // random words almost never assemble a trap with a filled argument block.
    if r.below(3) == 0 {
        regs[0] = r.below(300) as u32;
        for reg in regs.iter_mut().take(4).skip(1) {
            *reg = (0x1000 + r.below(0x0BF_0000) as u32) & !3;
        }
        for _ in 0..r.below(5) {
            let base = 1 + r.below(3) as u32;
            let val = 4 + r.below(12) as u32;
            let off = 4 * r.below(4) as u32;
            rom.push((ST << 24) | (base << 20) | (val << 16) | off);
        }
        rom.push(if r.below(2) == 0 {
            (INT << 24) | VECTOR_SYSCALL
        } else {
            SYSCALL << 24
        });
    }
    while rom.len() < words {
        rom.push(gen_word(r));
    }
    let mut data = Vec::with_capacity(64 + 4 * rom.len());
    for v in regs.iter().chain(rom.iter()) {
        data.extend_from_slice(&v.to_le_bytes());
    }
    data
}

fn run_one(label: &str, data: &[u8]) {
    if catch_unwind(AssertUnwindSafe(|| run_fuzz_input(data))).is_err() {
        panic!("{label}: run_fuzz_input panicked\ninput {data:02x?}");
    }
}

#[test]
fn committed_seeds_do_not_panic() {
    let dir = concat!(env!("CARGO_MANIFEST_DIR"), "/fuzz/seeds/fuzz_mbc_execute");
    let mut n = 0;
    for entry in std::fs::read_dir(dir).expect("seed directory") {
        let path = entry.expect("seed entry").path();
        let data = std::fs::read(&path).expect("seed file");
        run_one(&format!("seed {}", path.display()), &data);
        n += 1;
    }
    assert!(n > 0, "no committed seeds under {dir}");
}

#[test]
fn seeded_inputs_do_not_panic() {
    let seed = env_u64("MBC_SEEDED_SEED", DEFAULT_SEED);
    let cases = env_u64("MBC_SEEDED_CASES", DEFAULT_CASES);
    let mut r = SplitMix(seed);
    for case in 0..cases {
        let data = gen_case(&mut r);
        run_one(&format!("MBC_SEEDED_SEED={seed:#x} case {case}"), &data);
    }
}

#[test]
fn generator_reaches_every_defined_opcode() {
    // A generator that never emits an opcode never tests its arm.
    let mut r = SplitMix(DEFAULT_SEED);
    let mut seen = [false; 256];
    for _ in 0..3000 {
        let data = gen_case(&mut r);
        for w in data[64..].as_chunks::<4>().0 {
            seen[w[3] as usize] = true;
        }
    }
    let missing: Vec<u8> = (0..=255u8)
        .filter(|&o| is_valid_opcode(o) && !seen[o as usize])
        .collect();
    assert!(
        missing.is_empty(),
        "opcodes never generated: {missing:02x?}"
    );
}

#[test]
fn short_input_is_ignored() {
    // The body returns early below 16 registers and one word.
    run_one("short", &[0u8; 67]);
    run_one("empty", &[]);
}
