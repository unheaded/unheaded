// SPDX-License-Identifier: GPL-3.0-or-later
//! Cartridge differential oracle (S5 Phase 17, the Lich hook surface).
//!
//! Three engines run MBC cartridges:
//!
//! | Engine | Code | Driven by |
//! |---|---|---|
//! | shared step | `monad_common::cart` | [`lockstep`] (host), `cart_xdp` (BPF) |
//! | host emulator | `monad-mbc` `execute.rs` (`ExecCpu`) | [`lockstep`] |
//! | BPF interpreter | `ebpf/cartridge-ebpf` `cart_xdp` | `pkg/cartridge` via `BPF_PROG_TEST_RUN` |
//!
//! [`lockstep`] compares the first two after every instruction: the 16
//! registers, the Z/N/C flags, the PC and the 64-word flow window. The BPF
//! leg (`cart diff --out`, `TestDifferential_ThreeEngines`) compares the
//! third against the final state hash, verdict and EMIT records the first
//! two agreed on. Together: three engines, one answer.
//!
//! Syscalls are the cartridge's I/O boundary, not ISA semantics, so on the
//! host-emulator side the harness services them from the same input the
//! shared step reads (FIELD, NOW_NS_LO, EMIT, VERDICT). A cartridge fault
//! ends the comparison: the fault is the sandbox refusing (the host emulator
//! has no windows, it would read its full RAM), and the shared step's state
//! at the fault is what BPF stores.
//!
//! [`gen_xdp_cartridge`] draws from an [`Entropy`] source: [`XorShift`] for
//! seeded campaigns, [`Bytes`] for libFuzzer input (`fuzz_cart_differential`).

use std::fmt;

use monad_common::cart::{
    begin_run, field, run_one, run_out, sys, verdict, CartIo, CartState, FaultKind, Step,
    FIELD_COUNT, MEM_WORDS, N_PKT, STAGE_XDP,
};
use monad_common::{mbc_opcodes as op, MbcInsn};
use monad_mbc::ExecCpu;

use crate::reference::PacketInput;
use crate::Image;

/// A source of random 64-bit words.
pub trait Entropy {
    /// Next word.
    fn next_u64(&mut self) -> u64;
    /// Uniform-ish in `0..n` (`n > 0`).
    fn below(&mut self, n: u64) -> u64 {
        self.next_u64() % n
    }
}

/// xorshift64*: the same generator as `tests/differential.rs` and LICH-015.
#[derive(Clone, Debug)]
pub struct XorShift(u64);

impl XorShift {
    /// Seeded generator (a zero seed is replaced, xorshift has no zero state).
    pub fn new(seed: u64) -> Self {
        XorShift(if seed == 0 {
            0x9E37_79B9_7F4A_7C15
        } else {
            seed
        })
    }
}

impl Entropy for XorShift {
    fn next_u64(&mut self) -> u64 {
        let mut x = self.0;
        x ^= x >> 12;
        x ^= x << 25;
        x ^= x >> 27;
        self.0 = x;
        x.wrapping_mul(0x2545_F491_4F6C_DD1D)
    }
}

/// Raw bytes as entropy (libFuzzer input): 8 bytes per word, little-endian,
/// zero-padded; an exhausted stream reads as zeros, so generation always ends.
#[derive(Clone, Debug)]
pub struct Bytes<'a> {
    b: &'a [u8],
    pos: usize,
}

impl<'a> Bytes<'a> {
    /// Wrap a byte slice.
    pub fn new(b: &'a [u8]) -> Self {
        Bytes { b, pos: 0 }
    }
}

impl Entropy for Bytes<'_> {
    fn next_u64(&mut self) -> u64 {
        let mut w = [0u8; 8];
        let end = (self.pos + 8).min(self.b.len());
        if self.pos < end {
            w[..end - self.pos].copy_from_slice(&self.b[self.pos..end]);
        }
        self.pos = self.pos.saturating_add(8);
        u64::from_le_bytes(w)
    }
}

/// Longest generated ROM (words, HALT included). Paths are forward-only, so
/// no run can reach the 64-instruction XDP budget.
pub const GEN_ROM_MAX: usize = 48;

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

fn enc(o: u8, d: u8, s: u8, imm: u16) -> u32 {
    MbcInsn::encode(o, d & 0x0F, s & 0x0F, imm).0
}

fn interesting(e: &mut impl Entropy) -> u32 {
    match e.below(6) {
        0 => 0,
        1 => 0xFFFF_FFFF,
        2 => 0x8000_0000,
        3 => e.below(64) as u32,
        _ => e.next_u64() as u32,
    }
}

/// One generated item before branch targets are fixed.
enum Item {
    Word(u32),
    /// Forward branch: opcode and a fraction used to place the target.
    Branch(u8, u64),
}

/// A random XDP-stage cartridge that the validator accepts: forward-only
/// branches, at most [`GEN_ROM_MAX`] words, ending in HALT, with 0–16 DATA
/// words. About one memory access in sixteen and one FIELD id in sixteen are
/// deliberately out of range, so the fault paths are exercised too.
pub fn gen_xdp_cartridge(e: &mut impl Entropy) -> Image {
    let data: Vec<u32> = (0..e.below(17)).map(|_| interesting(e)).collect();
    let target_len = 4 + e.below((GEN_ROM_MAX - 4) as u64) as usize; // excl. HALT
    let mut items: Vec<Item> = Vec::with_capacity(GEN_ROM_MAX);
    let push = |items: &mut Vec<Item>, w: u32| items.push(Item::Word(w));

    // Prologue: seed a few registers (an XDP run starts with all zero).
    for _ in 0..e.below(6) {
        let r = e.below(16) as u8;
        let v = interesting(e);
        push(&mut items, enc(op::MOVI, r, 0, v as u16));
        if e.below(2) == 0 {
            push(&mut items, enc(op::LOAD_IMM32, r, 0, (v >> 16) as u16));
        }
    }
    while items.len() + 2 <= target_len {
        let rb = e.below(16) as u8;
        let rx = e.below(16) as u8;
        match e.below(100) {
            0..=44 => {
                let o = ALU[e.below(ALU.len() as u64) as usize];
                let imm = match e.below(4) {
                    0 => e.below(40) as u16, // shifts, including >= 32
                    1 => 0xFFFF,
                    _ => e.next_u64() as u16,
                };
                push(&mut items, enc(o, rx, rb, imm));
            }
            45..=56 => {
                let o = BR[e.below(BR.len() as u64) as usize];
                items.push(Item::Branch(o, e.next_u64()));
            }
            57..=78 => {
                // Memory: MOVI base, then LD/ST/LDB [base + off].
                let kind = e.below(3);
                let off: i16 = [0, 4, 8, -4][e.below(4) as usize];
                let addr: u32 = if e.below(16) == 0 {
                    0x100 + e.below(0xFE00) as u32 // outside both windows (or misaligned)
                } else if kind != 1 && !data.is_empty() && e.below(3) == 0 {
                    0x1000 + 4 * e.below(data.len() as u64) as u32 // DATA (read-only)
                } else {
                    4 * e.below(MEM_WORDS as u64) as u32 // flow window
                };
                let addr = if kind == 2 {
                    addr + e.below(4) as u32
                } else {
                    addr
                };
                let base = (addr as i32 - off as i32).clamp(0, 0xFFFF) as u16;
                push(&mut items, enc(op::MOVI, rb, 0, base));
                let w = match kind {
                    0 => enc(op::LD, rx, rb, off as u16),
                    1 => enc(op::ST, rb, rx, off as u16),
                    _ => enc(op::LDB, rx, rb, off as u16),
                };
                push(&mut items, w);
            }
            79..=88 => {
                // FIELD(id): r0 = field[r1].
                let id = if e.below(16) == 0 {
                    FIELD_COUNT + e.below(8) as u32
                } else {
                    e.below(FIELD_COUNT as u64) as u32
                };
                push(&mut items, enc(op::MOVI, 1, 0, id as u16));
                push(&mut items, enc(op::SYSCALL, 0, 0, sys::FIELD));
            }
            89..=92 => push(&mut items, enc(op::SYSCALL, 0, 0, sys::NOW_NS_LO)),
            93..=96 => push(&mut items, enc(op::SYSCALL, 0, 0, sys::EMIT)),
            _ => {
                // VERDICT: PASS / DROP, rarely an unknown code (fault).
                let code = match e.below(8) {
                    0 => 2 + e.below(4) as u16,
                    1..=3 => verdict::DROP as u16,
                    _ => verdict::PASS as u16,
                };
                push(&mut items, enc(op::MOVI, 1, 0, code));
                push(&mut items, enc(op::SYSCALL, 0, 0, sys::VERDICT));
            }
        }
    }
    let n = items.len(); // HALT goes at index n
    let rom: Vec<u32> = items
        .iter()
        .enumerate()
        .map(|(i, it)| match *it {
            Item::Word(w) => w,
            Item::Branch(o, frac) => {
                // Target in (i, n]: forward only, HALT reachable.
                let span = (n - i) as u64;
                let off = (frac % span) as u32; // target = i + 1 + off
                ((o as u32) << 24) | off
            }
        })
        .chain(core::iter::once(enc(op::HALT, 0, 0, 0)))
        .collect();
    Image {
        stage: STAGE_XDP,
        max_insn: N_PKT as u16,
        entry: 0,
        loop_bound: 0,
        flags: 0,
        rom,
        data,
    }
}

/// Random run input: every FIELD value and the clock.
pub fn gen_input(e: &mut impl Entropy) -> PacketInput {
    let mut fields = [0u32; FIELD_COUNT as usize];
    for f in fields.iter_mut() {
        *f = interesting(e);
    }
    PacketInput {
        fields,
        now_ns: e.next_u64(),
    }
}

/// The input `cart_xdp` computes for `pkg/cartridge`'s three-engine packet:
/// `IPv4Packet(17, 10.0.0.1, 10.0.0.2, sport, 53, 80)`, the first packet of
/// its flow, at clock `now_ns`.
pub fn bpf_case_input(sport: u16, now_ns: u64) -> PacketInput {
    let mut f = [0u32; FIELD_COUNT as usize];
    f[field::L3_PROTO as usize] = 4;
    f[field::L4_PROTO as usize] = 17;
    f[field::SRC_PORT as usize] = sport as u32;
    f[field::DST_PORT as usize] = 53;
    f[field::PKT_LEN as usize] = 80;
    f[field::IP_TTL as usize] = 64;
    f[field::IAT_BUCKET as usize] = 0;
    f[field::FLOW_PKT_COUNT as usize] = 1;
    f[field::FLOW_BYTE_COUNT as usize] = 80;
    PacketInput { fields: f, now_ns }
}

/// How an agreed run ended.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum End {
    /// HALT (XDP PASS).
    Halt,
    /// VERDICT with this code.
    Verdict(u8),
    /// The shared step refused the instruction (sandbox boundary).
    Fault(FaultKind),
}

impl End {
    /// The ending a run result maps to (`None` for `Continue` / `Yield`).
    pub fn from_step(s: Step) -> Option<End> {
        match s {
            Step::Halt => Some(End::Halt),
            Step::Verdict(v) => Some(End::Verdict(v)),
            Step::Fault(k) => Some(End::Fault(k)),
            Step::Continue | Step::Yield => None,
        }
    }

    /// XDP action code: PASS 0 / DROP 1 (ADR-108 D-C3; faults pass).
    pub fn xdp_verdict(self) -> u8 {
        match self {
            End::Verdict(v) => v,
            _ => verdict::PASS,
        }
    }
}

impl fmt::Display for End {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            End::Halt => write!(f, "halt"),
            End::Verdict(v) => write!(f, "verdict:{v}"),
            End::Fault(k) => write!(f, "fault:{k:?}"),
        }
    }
}

/// Both engines agreed on every instruction.
#[derive(Clone, Debug)]
pub struct Agreement {
    /// Instructions compared.
    pub steps: usize,
    /// How the run ended.
    pub end: End,
    /// Final shared-step state (what BPF stores in `FLOW_STATE`).
    pub state: CartState,
    /// EMIT records `(kind, a, b)` in order.
    pub emits: Vec<(u32, u32, u32)>,
    /// In-window LD/ST/LDB executed.
    pub mem_ops: usize,
    /// Syscalls serviced.
    pub syscalls: usize,
}

/// The first instruction where the engines disagree.
#[derive(Clone, Debug)]
pub struct Divergence {
    /// Instruction index in the run (0 = first).
    pub step: usize,
    /// PC of that instruction.
    pub pc: u16,
    /// Its word.
    pub word: u32,
    /// What differs.
    pub detail: String,
}

impl fmt::Display for Divergence {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "DIVERGED at step {} pc {} word {:#010x} (opcode {:#04x}): {}",
            self.step,
            self.pc,
            self.word,
            MbcInsn(self.word).opcode(),
            self.detail
        )
    }
}

/// Test hooks for the oracle itself.
#[derive(Clone, Copy, Debug, Default)]
pub struct Options {
    /// Flip bit 0 of host r1 after this instruction (self-test: the
    /// comparator must report that very step).
    pub flip_host_bit_after: Option<usize>,
}

struct RecIo<'a> {
    input: &'a PacketInput,
    emits: Vec<(u32, u32, u32)>,
}

impl CartIo for RecIo<'_> {
    fn field(&mut self, id: u32) -> Option<u32> {
        self.input.fields.get(id as usize).copied()
    }
    fn now_ns_lo(&mut self) -> u32 {
        self.input.now_ns as u32
    }
    fn emit(&mut self, kind: u32, a: u32, b: u32) {
        self.emits.push((kind, a, b));
    }
    fn queue(&mut self, _: &[u32; 8]) {}
}

/// Run `img` on one fresh flow with `input`, both engines in lockstep.
pub fn lockstep(img: &Image, input: &PacketInput) -> Result<Agreement, Divergence> {
    lockstep_with(img, input, Options::default())
}

/// [`lockstep`] with oracle test hooks.
pub fn lockstep_with(
    img: &Image,
    input: &PacketInput,
    opt: Options,
) -> Result<Agreement, Divergence> {
    let cfg = img.cfg();
    let mut st = CartState::zeroed();
    begin_run(&mut st, img.entry);
    let mut io = RecIo {
        input,
        emits: Vec::new(),
    };

    // Host emulator: same registers, PC, flags; flow window at byte 0x0000,
    // DATA at 0x1000 (the ADR-108 windows) in its flat RAM.
    let mut host = ExecCpu::new();
    host.load_rom(&img.rom);
    host.state.regs = st.regs;
    host.state.pc = img.entry as u32;
    host.state.flags = 0;
    host.ram[..MEM_WORDS].copy_from_slice(&st.mem);
    let dbase = (monad_common::cart::DATA_BASE / 4) as usize;
    host.ram[dbase..dbase + img.data.len()].copy_from_slice(&img.data);
    let mut host_emits: Vec<(u32, u32, u32)> = Vec::new();

    let (mut mem_ops, mut syscalls) = (0usize, 0usize);
    let mut steps = 0usize;
    let end = loop {
        if steps > N_PKT {
            // Unreachable for generated images (paths <= GEN_ROM_MAX), but a
            // hand-made image must not loop the oracle.
            break End::from_step(run_out(&mut st, &cfg)).unwrap_or(End::Halt);
        }
        let pc = st.pc;
        let word = img.rom.get(pc as usize).copied().unwrap_or(0);
        let insn = MbcInsn(word);
        let r = run_one(&mut st, &cfg, &img.rom[..], &img.data[..], &mut io);
        let div = |detail: String| Divergence {
            step: steps,
            pc,
            word,
            detail,
        };
        if let Step::Fault(k) = r {
            break End::Fault(k);
        }
        if insn.opcode() == op::SYSCALL {
            // Service the syscall for the host arm from the same input.
            syscalls += 1;
            let h = &mut host.state;
            match insn.imm16() {
                sys::FIELD => h.regs[0] = input.fields[h.regs[1] as usize],
                sys::NOW_NS_LO => h.regs[0] = input.now_ns as u32,
                sys::EMIT => host_emits.push((h.regs[1], h.regs[2], h.regs[3])),
                _ => {}
            }
            h.pc = h.pc.wrapping_add(1);
        } else {
            if matches!(insn.opcode(), op::LD | op::ST | op::LDB) {
                mem_ops += 1;
            }
            match (host.step(), r) {
                (Err(monad_mbc::ExecError::Halted), Step::Halt) => {}
                (Ok(()), Step::Continue) => {}
                (h, c) => return Err(div(format!("host {h:?}, shared step {c:?}"))),
            }
        }
        if opt.flip_host_bit_after == Some(steps) {
            host.state.regs[1] ^= 1;
        }
        let hflags = host.state.flags & 0x07;
        if host.state.regs != st.regs {
            let i = (0..16)
                .find(|&i| host.state.regs[i] != st.regs[i])
                .unwrap_or(0);
            return Err(div(format!(
                "r{i}: shared {:#010x}, host {:#010x}",
                st.regs[i], host.state.regs[i]
            )));
        }
        if hflags != st.flags {
            return Err(div(format!(
                "flags: shared {:#x}, host {hflags:#x}",
                st.flags
            )));
        }
        if host.state.pc != st.pc as u32 {
            return Err(div(format!("pc: shared {}, host {}", st.pc, host.state.pc)));
        }
        if host.ram[..MEM_WORDS] != st.mem[..] {
            let i = (0..MEM_WORDS)
                .find(|&i| host.ram[i] != st.mem[i])
                .unwrap_or(0);
            return Err(div(format!(
                "mem[{:#04x}]: shared {:#010x}, host {:#010x}",
                i * 4,
                st.mem[i],
                host.ram[i]
            )));
        }
        steps += 1;
        match r {
            Step::Continue => {}
            other => break End::from_step(other).unwrap_or(End::Halt),
        }
    };
    if host_emits != io.emits {
        return Err(Divergence {
            step: steps,
            pc: st.pc,
            word: 0,
            detail: format!("emits: shared {:?}, host {:?}", io.emits, host_emits),
        });
    }
    Ok(Agreement {
        steps,
        end,
        state: st,
        emits: io.emits,
        mem_ops,
        syscalls,
    })
}
