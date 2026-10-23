// SPDX-License-Identifier: GPL-3.0-or-later
#![no_main]
//! S5 Step 269: the shared cartridge step must never panic. Every bad input
//! (opcode, address, branch, syscall, budget) has to come back as a
//! `Step::Fault` value.
//!
//! Input layout:
//!   [0]       stage (bit 0)
//!   [1..3]    max_insn (LE, masked to 0..=256)
//!   [3..5]    loop_bound (LE)
//!   [5]       rom_words (1..=64); the rest of the ROM-sized bytes follow
//!   [6..70]   initial registers (16 x u32 LE)
//!   [70..]    ROM words, then DATA words (LE u32), then FIELD values reuse the
//!             same bytes cyclically

use libfuzzer_sys::fuzz_target;
use monad_common::cart::{
    begin_run, run, state_hash, CartCfg, CartIo, CartState, Step, N_PKT, N_TICK, STAGE_TICK,
    STAGE_XDP,
};

struct FuzzIo<'a> {
    bytes: &'a [u8],
    pos: usize,
}

impl FuzzIo<'_> {
    fn next(&mut self) -> u32 {
        if self.bytes.is_empty() {
            return 0;
        }
        let b = self.bytes[self.pos % self.bytes.len()];
        self.pos = self.pos.wrapping_add(1);
        b as u32
    }
}

impl CartIo for FuzzIo<'_> {
    fn field(&mut self, id: u32) -> Option<u32> {
        if id < 13 {
            Some(self.next())
        } else {
            None
        }
    }
    fn now_ns_lo(&mut self) -> u32 {
        self.next()
    }
    fn emit(&mut self, _: u32, _: u32, _: u32) {}
    fn queue(&mut self, _: &[u32; 8]) {}
}

fuzz_target!(|data: &[u8]| {
    if data.len() < 70 + 4 {
        return;
    }
    let stage = if data[0] & 1 == 0 {
        STAGE_XDP
    } else {
        STAGE_TICK
    };
    let max_insn = u16::from_le_bytes([data[1], data[2]]) % 257;
    let loop_bound = u16::from_le_bytes([data[3], data[4]]);
    let rom_cap = 1 + (data[5] as usize % 64);
    let words: Vec<u32> = data[70..]
        .as_chunks::<4>()
        .0
        .iter()
        .map(|c| u32::from_le_bytes(*c))
        .collect();
    if words.is_empty() {
        return;
    }
    let rom_n = rom_cap.min(words.len());
    let (rom, dat) = words.split_at(rom_n);
    let mut st = CartState::zeroed();
    begin_run(&mut st, 0);
    for (r, c) in st.regs.iter_mut().zip(data[6..70].as_chunks::<4>().0) {
        *r = u32::from_le_bytes(*c);
    }
    let cfg = CartCfg {
        stage,
        max_insn,
        rom_words: rom.len() as u16,
        data_words: dat.len().min(1024) as u16,
        entry: 0,
        loop_bound,
    };
    let mut io = FuzzIo {
        bytes: data,
        pos: 0,
    };
    // A few calls, so a yielded tick job resumes.
    for _ in 0..4 {
        let r = if stage == STAGE_XDP {
            run::<N_PKT, _, _, _>(&mut st, &cfg, rom, dat, &mut io)
        } else {
            run::<N_TICK, _, _, _>(&mut st, &cfg, rom, dat, &mut io)
        };
        assert!(st.insns <= max_insn as u32);
        assert!((st.pc as usize) <= rom.len());
        let _ = state_hash(&st);
        if r != Step::Yield {
            break;
        }
    }
});
