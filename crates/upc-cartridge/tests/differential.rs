// SPDX-License-Identifier: GPL-3.0-or-later
//! H-C3 (S5 Step 262): the shared cartridge step and the host MBC emulator
//! (`monad-mbc` execute.rs) agree on registers and flags for random
//! allowlisted programs without syscalls or memory. The shipping engine
//! (`monad-cpu-ebpf`) is the reference both must match; the cartridge step
//! copies its semantics, so a divergence here is a host-emulator drift.
//!
//! Instructions run in lockstep, so a divergence names the first opcode that
//! differs.

use monad_common::cart::{begin_run, step, CartCfg, CartIo, CartState, Step, STAGE_TICK};
use monad_common::{mbc_opcodes as op, MbcInsn};
use monad_mbc::ExecCpu;

struct NoIo;
impl CartIo for NoIo {
    fn field(&mut self, _: u32) -> Option<u32> {
        None
    }
    fn now_ns_lo(&mut self) -> u32 {
        0
    }
    fn emit(&mut self, _: u32, _: u32, _: u32) {}
    fn queue(&mut self, _: &[u32; 8]) {}
}

struct Rng(u64);
impl Rng {
    fn next(&mut self) -> u64 {
        let mut x = self.0;
        x ^= x >> 12;
        x ^= x << 25;
        x ^= x >> 27;
        self.0 = x;
        x.wrapping_mul(0x2545_F491_4F6C_DD1D)
    }
    fn below(&mut self, n: u64) -> u64 {
        self.next() % n
    }
}

const ALU: &[u8] = &[
    op::ADD,
    op::SUB,
    op::MUL,
    op::AND,
    op::OR,
    op::XOR,
    op::NOT,
    op::SHL,
    op::SHR,
    op::SAR,
    op::MOV,
    op::MOVI,
    op::CMP,
    op::LOAD_IMM32,
    op::ADDI,
    op::SHLR,
    op::SHRR,
    op::SARR,
    op::MULH,
    op::NOP,
];
const BR: &[u8] = &[op::JMP, op::JZ, op::JNZ, op::JN, op::JP, op::JC, op::JNC];

fn random_program(rng: &mut Rng) -> Vec<u32> {
    let n = 1 + rng.below(30) as usize;
    let mut rom = Vec::with_capacity(n + 1);
    for i in 0..n {
        if rng.below(5) == 0 {
            // Forward branch to somewhere in (i, n] (n = the HALT).
            let o = BR[rng.below(BR.len() as u64) as usize];
            let target = i + 1 + rng.below((n - i) as u64) as usize;
            let off = (target - (i + 1)) as u32;
            rom.push(((o as u32) << 24) | off);
        } else {
            let o = ALU[rng.below(ALU.len() as u64) as usize];
            let imm = match rng.below(4) {
                0 => rng.below(40) as u16, // small shifts, incl. >= 32
                1 => 0xFFFF,
                _ => rng.next() as u16,
            };
            rom.push(MbcInsn::encode(o, rng.below(16) as u8, rng.below(16) as u8, imm).0);
        }
    }
    rom.push(MbcInsn::encode(op::HALT, 0, 0, 0).0);
    rom
}

fn interesting(rng: &mut Rng) -> u32 {
    match rng.below(6) {
        0 => 0,
        1 => 0xFFFF_FFFF,
        2 => 0x8000_0000,
        3 => rng.below(64) as u32,
        _ => rng.next() as u32,
    }
}

#[test]
fn h_c3_cart_step_matches_host_emulator() {
    let mut rng = Rng(0xC3C3_5EED);
    let mut diverged: std::collections::BTreeMap<u8, (u32, String)> = Default::default();
    for _ in 0..10_000 {
        let rom = random_program(&mut rng);
        let regs: [u32; 16] = core::array::from_fn(|_| interesting(&mut rng));
        let cfg = CartCfg {
            stage: STAGE_TICK,
            max_insn: 256,
            rom_words: rom.len() as u16,
            data_words: 0,
            entry: 0,
            loop_bound: 0,
        };
        let mut st = CartState::zeroed();
        begin_run(&mut st, 0);
        st.regs = regs;
        let mut host = ExecCpu::new();
        host.load_rom(&rom);
        host.state.regs = regs;
        host.state.flags = 0;
        for _ in 0..64 {
            let pc = st.pc as usize;
            let w = rom[pc];
            let r = step(&mut st, &cfg, &rom[..], &[][..], &mut NoIo);
            let h = host.step();
            let hflags = host.state.flags & 0x07;
            if st.regs != host.state.regs || st.flags != hflags || st.pc as u32 != host.state.pc {
                let o = MbcInsn(w).opcode();
                diverged.entry(o).or_insert_with(|| {
                    (w, format!("cart regs[d]={:#x} flags={:#x} pc={} | host regs[d]={:#x} flags={:#x} pc={}",
                        st.regs[MbcInsn(w).dst() as usize], st.flags, st.pc,
                        host.state.regs[MbcInsn(w).dst() as usize], hflags, host.state.pc))
                });
                break;
            }
            match r {
                Step::Continue => assert!(h.is_ok(), "host errored where cart continued"),
                Step::Halt => break,
                other => panic!("unexpected {other:?} for {w:#010x}"),
            }
        }
    }
    if !diverged.is_empty() {
        let mut msg =
            String::from("ISA semantics drift (cart::step == monad-cpu-ebpf; host differs):\n");
        for (o, (w, d)) in &diverged {
            msg.push_str(&format!("  opcode {o:#04x} word {w:#010x}: {d}\n"));
        }
        panic!("{msg}");
    }
}
