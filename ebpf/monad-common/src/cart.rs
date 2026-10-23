// SPDX-License-Identifier: GPL-3.0-or-later
//! MBC cartridge interpreter step (ADR-108, Cartridge ABI v0).
//!
//! One `no_std` source compiled into both the BPF object
//! (`ebpf/cartridge-ebpf`) and the host tools (`crates/upc-cartridge`), so a
//! cartridge run replays bit-for-bit on the host (ADR-108 determinism
//! contract).
//!
//! Semantics are the shipping engine's (`monad-cpu-ebpf`): PC advances before
//! execution, branches add a 24-bit signed offset to `pc + 1`, `MOVI`
//! zero-extends, flags follow `set_flags`. The opcode set is the ADR-108
//! allowlist; memory is two bounds-checked windows; inputs come only from
//! [`CartIo`].
//!
//! Verifier shape (this code is inlined into BPF): no recursion, no
//! allocation, every array index masked, every window access range-checked,
//! loops bounded by a const.

use crate::{mbc_flags as mf, mbc_opcodes as op, MbcInsn};

/// Stage of a cartridge (image header byte 5).
pub const STAGE_XDP: u8 = 0;
/// Tick stage: run by the host pump, resumable.
pub const STAGE_TICK: u8 = 1;

/// Instruction cap per XDP-stage run (`N_PKT`).
pub const N_PKT: usize = 64;
/// Instruction cap per tick-stage call (`N_TICK`).
pub const N_TICK: usize = 256;
/// ROM words per slot.
pub const ROM_WORDS_MAX: u16 = 1024;
/// DATA words per slot (4096 bytes).
pub const DATA_WORDS_MAX: u16 = 1024;
/// Words in the flow window (`mem`).
pub const MEM_WORDS: usize = 64;
/// First byte address of the read-only DATA window.
pub const DATA_BASE: u32 = 0x1000;
/// Words in a ScoreJob feature snapshot.
pub const FEAT_WORDS: usize = 8;
/// Number of FIELD ids (0..=12).
pub const FIELD_COUNT: u32 = 13;

/// FIELD ids (ADR-108). Metadata only: no id returns payload bytes.
pub mod field {
    /// 4, 6, or 0 (not IP).
    pub const L3_PROTO: u32 = 0;
    /// IP protocol / IPv6 next header.
    pub const L4_PROTO: u32 = 1;
    /// TCP/UDP source port, else 0.
    pub const SRC_PORT: u32 = 2;
    /// TCP/UDP destination port, else 0.
    pub const DST_PORT: u32 = 3;
    /// Frame length in bytes.
    pub const PKT_LEN: u32 = 4;
    /// IPv4 TTL / IPv6 hop limit.
    pub const IP_TTL: u32 = 5;
    /// TCP flags byte, else 0.
    pub const TCP_FLAGS: u32 = 6;
    /// floor(log2(inter-arrival µs + 1)), capped 31; 0 on a flow's first packet.
    pub const IAT_BUCKET: u32 = 7;
    /// Packets in the flow, including this one.
    pub const FLOW_PKT_COUNT: u32 = 8;
    /// Bytes in the flow, including this one (saturating).
    pub const FLOW_BYTE_COUNT: u32 = 9;
    /// 1 if the Monad HbH option is present.
    pub const MONAD_PRESENT: u32 = 10;
    /// Monad qos_class byte, else 0.
    pub const MONAD_QOS: u32 = 11;
    /// Monad flags byte, else 0.
    pub const MONAD_FLAGS: u32 = 12;
}

/// Syscall numbers (`SYSCALL imm16`).
pub mod sys {
    /// `r0 = field[r1]` (xdp).
    pub const FIELD: u16 = 0;
    /// `r0` = low 32 bits of the run's clock (both).
    pub const NOW_NS_LO: u16 = 1;
    /// `r1` = 0 PASS / 1 DROP; ends the run (xdp).
    pub const VERDICT: u16 = 2;
    /// Push `{kind=r1, a=r2, b=r3}` to the event ringbuf (both).
    pub const EMIT: u16 = 3;
    /// Push a ScoreJob of `mem[r1>>2 .. +8]` (xdp).
    pub const QUEUE: u16 = 4;
    /// Suspend; the next tick call resumes (tick).
    pub const YIELD: u16 = 5;
}

/// Verdict codes.
pub mod verdict {
    /// Let the packet through.
    pub const PASS: u8 = 0;
    /// Drop the packet.
    pub const DROP: u8 = 1;
}

/// `CartState.status` values.
pub mod status {
    /// Not started / between XDP runs.
    pub const IDLE: u8 = 0;
    /// Tick job suspended, resumes on the next call.
    pub const YIELDED: u8 = 1;
    /// Run or job finished (HALT, VERDICT, or fault).
    pub const DONE: u8 = 2;
}

/// Why a run stopped abnormally. Every fault is a value, never a panic.
#[repr(u8)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum FaultKind {
    /// Opcode outside the ADR-108 allowlist.
    BadOpcode = 1,
    /// Address outside both windows, a store to DATA, or a misaligned word access.
    Mem = 2,
    /// PC or branch target outside the ROM.
    Pc = 3,
    /// Unknown syscall, or one the stage does not allow.
    Syscall = 4,
    /// FIELD with an unknown id.
    Field = 5,
    /// VERDICT with an unknown code.
    Verdict = 6,
    /// XDP stage ran out of instructions.
    Budget = 7,
    /// More taken back-edges than `loop_bound` (any, in the XDP stage).
    LoopBound = 8,
}

/// Result of one [`step`] (or of a whole [`run`]).
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Step {
    /// Keep going.
    Continue,
    /// XDP verdict; the run ends.
    Verdict(u8),
    /// Tick job suspended.
    Yield,
    /// HALT: PASS in the XDP stage, job done in the tick stage.
    Halt,
    /// The run ends; the packet passes (unless the slot is fail_closed).
    Fault(FaultKind),
}

/// Machine state (ADR-108, 336 bytes, no implicit padding).
#[repr(C)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct CartState {
    /// General registers r0..r15.
    pub regs: [u32; 16],
    /// Flow window, byte addresses 0x0000..0x00FF.
    pub mem: [u32; MEM_WORDS],
    /// Word index into ROM.
    pub pc: u16,
    /// Z/N/C flags (`mbc_flags`).
    pub flags: u8,
    /// [`status`] value.
    pub status: u8,
    /// Instructions executed in this run (XDP) or call (tick).
    pub insns: u32,
    /// Taken back-edges in this job.
    pub back_edges: u32,
    /// Slot generation the state belongs to.
    pub gen: u32,
}

impl CartState {
    /// All-zero state.
    pub const fn zeroed() -> Self {
        Self {
            regs: [0; 16],
            mem: [0; MEM_WORDS],
            pc: 0,
            flags: 0,
            status: 0,
            insns: 0,
            back_edges: 0,
            gen: 0,
        }
    }
}

/// Byte size of [`CartState`] (ABI).
pub const CART_STATE_SIZE: usize = 336;
const _: () = assert!(core::mem::size_of::<CartState>() == CART_STATE_SIZE);

/// Number of cartridge slots.
pub const SLOTS: usize = 4;
/// `FlowKey.slot` value for the per-flow metadata entry (not a slot).
pub const META_SLOT: u8 = 0xFF;

/// Flow identity (ADR-108, 40 bytes). IPv4 addresses sit in the first 4
/// bytes of `src`/`dst`, the rest zero.
#[repr(C)]
#[derive(Clone, Copy, Debug, PartialEq, Eq, Default)]
pub struct FlowKey {
    /// Source address.
    pub src: [u8; 16],
    /// Destination address.
    pub dst: [u8; 16],
    /// Source port (host order), 0 if not TCP/UDP.
    pub sport: u16,
    /// Destination port (host order), 0 if not TCP/UDP.
    pub dport: u16,
    /// L4 protocol.
    pub proto: u8,
    /// 4 or 6.
    pub l3: u8,
    /// Slot this state belongs to, or [`META_SLOT`].
    pub slot: u8,
    /// Zero.
    pub rsv: u8,
}
const _: () = assert!(core::mem::size_of::<FlowKey>() == 40);

/// Per-flow metadata behind FIELD ids 7–9 (FLOW_META map value, 24 bytes).
#[repr(C)]
#[derive(Clone, Copy, Debug, PartialEq, Eq, Default)]
pub struct FlowMeta {
    /// Clock of the previous packet.
    pub last_ns: u64,
    /// Packets so far.
    pub pkts: u32,
    /// Bytes so far (saturating).
    pub bytes: u32,
    /// Clock of the first packet.
    pub first_ns: u64,
}
const _: () = assert!(core::mem::size_of::<FlowMeta>() == 24);

/// One queued flow for the tick stage (SCORE_QUEUE value, 80 bytes).
#[repr(C)]
#[derive(Clone, Copy, Debug, PartialEq, Eq, Default)]
pub struct ScoreJob {
    /// Flow the features describe.
    pub flow: FlowKey,
    /// Feature snapshot (`mem[r1>>2 .. +8]` of the queuing cartridge).
    pub feat: [u32; FEAT_WORDS],
    /// Slot that queued it.
    pub src_slot: u8,
    /// Zero.
    pub rsv: u8,
    /// Zero.
    pub rsv2: u16,
    /// Low 32 bits of the clock at enqueue.
    pub enq_ns_lo: u32,
}
const _: () = assert!(core::mem::size_of::<ScoreJob>() == 80);

/// Slot configuration (CART_SLOT value, 20 bytes).
#[repr(C)]
#[derive(Clone, Copy, Debug, PartialEq, Eq, Default)]
pub struct SlotCfg {
    /// 1 = run this slot.
    pub active: u8,
    /// [`STAGE_XDP`] / [`STAGE_TICK`].
    pub stage: u8,
    /// Image header flags (bit 0 fail_closed).
    pub flags: u8,
    /// Zero.
    pub rsv: u8,
    /// Instructions per run / call.
    pub max_insn: u16,
    /// ROM length.
    pub rom_words: u16,
    /// DATA length.
    pub data_words: u16,
    /// Entry PC.
    pub entry: u16,
    /// Max taken back-edges per tick job.
    pub loop_bound: u16,
    /// Zero.
    pub rsv2: u16,
    /// Generation; bumped on every load so stale flow state resets.
    pub gen: u32,
}
const _: () = assert!(core::mem::size_of::<SlotCfg>() == 20);

impl SlotCfg {
    /// Step-function parameters for this slot.
    pub const fn cfg(&self) -> CartCfg {
        CartCfg {
            stage: self.stage,
            max_insn: self.max_insn,
            rom_words: self.rom_words,
            data_words: self.data_words,
            entry: self.entry,
            loop_bound: self.loop_bound,
        }
    }
}

/// One EMIT record (CART_EVENTS ringbuf, 64 bytes).
#[repr(C)]
#[derive(Clone, Copy, Debug, PartialEq, Eq, Default)]
pub struct CartEvent {
    /// Clock of the run.
    pub ts_ns: u64,
    /// Flow the run was for.
    pub flow: FlowKey,
    /// `r1` at EMIT.
    pub kind: u32,
    /// `r2` at EMIT.
    pub a: u32,
    /// `r3` at EMIT.
    pub b: u32,
    /// Slot.
    pub slot: u8,
    /// Stage.
    pub stage: u8,
    /// Zero.
    pub rsv: u16,
}
const _: () = assert!(core::mem::size_of::<CartEvent>() == 64);

/// `CART_STATS` per-slot counter offsets (slot `s` at `s * 8 + off`).
pub mod stat {
    /// Runs (XDP: packets seen by the slot; tick: calls).
    pub const RUNS: u32 = 0;
    /// Runs that ended PASS.
    pub const PASS: u32 = 1;
    /// Runs that ended DROP.
    pub const DROP: u32 = 2;
    /// Faults.
    pub const FAULTS: u32 = 3;
    /// Instructions executed.
    pub const INSNS: u32 = 4;
    /// EMIT records written.
    pub const EMITS: u32 = 5;
    /// QUEUE pushes that fit.
    pub const QUEUE_OK: u32 = 6;
    /// QUEUE pushes dropped (queue full).
    pub const QUEUE_DROPS: u32 = 7;
    /// Global: packets through `cart_xdp`.
    pub const XDP_PACKETS: u32 = 32;
    /// Global: `cart_tick` calls.
    pub const TICK_CALLS: u32 = 33;
    /// Global: tick jobs finished.
    pub const TICK_JOBS_DONE: u32 = 34;
    /// Global: EMIT records dropped (ringbuf full).
    pub const EVENTS_DROPPED: u32 = 35;
    /// Global: `cart_tick` calls with no job.
    pub const TICK_IDLE: u32 = 36;
    /// Global: slots the fault breaker disabled.
    pub const BREAKER_TRIPS: u32 = 37;
    /// Global: tick jobs abandoned because the slot was reloaded.
    pub const TICK_STALE: u32 = 38;
    /// Number of counters.
    pub const COUNT: u32 = 40;
}

/// `CART_CFG` keys.
pub mod cfgkey {
    /// Reserved for the Phase 16 trace switch.
    pub const TRACE: u32 = 0;
    /// Fixed clock in ns (0 = the real monotonic clock); makes test runs reproducible.
    pub const CLOCK_NS: u32 = 2;
    /// Number of keys.
    pub const COUNT: u32 = 8;
}

/// Faults per one-second window that disable a slot (ADR-108 D-C3).
pub const BREAKER_LIMIT: u32 = 64;

/// floor(log2(iat_us + 1)) capped at 31, for FIELD 7 (branch-only, no loop).
#[inline(always)]
pub fn iat_bucket(iat_ns: u64) -> u32 {
    let mut v = iat_ns / 1000 + 1;
    let mut r = 0u32;
    if v >= 1 << 32 {
        return 31;
    }
    if v >= 1 << 16 {
        v >>= 16;
        r += 16;
    }
    if v >= 1 << 8 {
        v >>= 8;
        r += 8;
    }
    if v >= 1 << 4 {
        v >>= 4;
        r += 4;
    }
    if v >= 1 << 2 {
        v >>= 2;
        r += 2;
    }
    if v >= 1 << 1 {
        r += 1;
    }
    if r > 31 {
        31
    } else {
        r
    }
}

/// Run parameters for one slot (from `SlotCfg` / the image header).
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct CartCfg {
    /// [`STAGE_XDP`] or [`STAGE_TICK`].
    pub stage: u8,
    /// Instructions per run (XDP) or per call (tick).
    pub max_insn: u16,
    /// ROM length in words.
    pub rom_words: u16,
    /// DATA length in words.
    pub data_words: u16,
    /// Entry PC.
    pub entry: u16,
    /// Max taken back-edges per job (0 in the XDP stage).
    pub loop_bound: u16,
}

/// ROM access. `word` is called only with `pc < rom_words`.
pub trait CartRom {
    /// Instruction word at `pc`, or `None` if the backing store has none.
    fn word(&self, pc: u32) -> Option<u32>;
}

/// DATA access. `word` is called only with `idx < data_words`.
pub trait CartData {
    /// Data word `idx`, or `None` if the backing store has none.
    fn word(&self, idx: u32) -> Option<u32>;
}

/// The cartridge's only inputs and outputs.
pub trait CartIo {
    /// FIELD value, or `None` for an unknown id.
    fn field(&mut self, id: u32) -> Option<u32>;
    /// Low 32 bits of the run's clock.
    fn now_ns_lo(&mut self) -> u32;
    /// Event to `CART_EVENTS` (a full ringbuf is the engine's counted drop).
    fn emit(&mut self, kind: u32, a: u32, b: u32);
    /// ScoreJob to `SCORE_QUEUE` (a full queue is the engine's counted drop).
    fn queue(&mut self, feat: &[u32; FEAT_WORDS]);
}

impl CartRom for [u32] {
    fn word(&self, pc: u32) -> Option<u32> {
        self.get(pc as usize).copied()
    }
}

impl CartData for [u32] {
    fn word(&self, idx: u32) -> Option<u32> {
        self.get(idx as usize).copied()
    }
}

#[inline(always)]
fn set_flags(st: &mut CartState, result: u32, carry: bool) {
    let mut f = 0u8;
    if result == 0 {
        f |= mf::Z;
    }
    if result & 0x8000_0000 != 0 {
        f |= mf::N;
    }
    if carry {
        f |= mf::C;
    }
    st.flags = f;
}

/// Word index into `mem` for a byte address already range-checked by the
/// caller. The mask is applied to a laundered value: LLVM otherwise folds
/// "addr < 0x100 && aligned" into one mask test on a register copy and
/// indexes with the unmasked original, which the BPF verifier rejects as an
/// unbounded map-value access (S5 Phase 11).
#[inline(always)]
fn mem_index(addr: u32) -> usize {
    (core::hint::black_box(addr >> 2) as usize) & (MEM_WORDS - 1)
}

/// Execute one instruction. Never panics; every bad input is a [`Step::Fault`].
#[inline(always)]
pub fn step<R: CartRom + ?Sized, D: CartData + ?Sized, I: CartIo>(
    st: &mut CartState,
    cfg: &CartCfg,
    rom: &R,
    data: &D,
    io: &mut I,
) -> Step {
    let pc = st.pc as u32;
    if pc >= cfg.rom_words as u32 {
        st.status = status::DONE;
        return Step::Fault(FaultKind::Pc);
    }
    let w = match rom.word(pc) {
        Some(w) => w,
        None => {
            st.status = status::DONE;
            return Step::Fault(FaultKind::Pc);
        }
    };
    let insn = MbcInsn(w);
    let opc = insn.opcode();
    let d = (insn.dst() & 0x0F) as usize;
    let s = (insn.src() & 0x0F) as usize;
    let imm = insn.imm16() as u32;
    let simm = insn.imm16_signed() as i32 as u32;
    let next = pc + 1;
    st.pc = next as u16; // rom_words <= 1024, so this fits
    st.insns = st.insns.wrapping_add(1);

    let res = if opc == op::NOP {
        Step::Continue
    } else if opc == op::ADD {
        let (r, c) = st.regs[d].overflowing_add(st.regs[s]);
        st.regs[d] = r;
        set_flags(st, r, c);
        Step::Continue
    } else if opc == op::SUB {
        let (r, b) = st.regs[d].overflowing_sub(st.regs[s]);
        st.regs[d] = r;
        set_flags(st, r, b);
        Step::Continue
    } else if opc == op::MUL {
        let r = st.regs[d].wrapping_mul(st.regs[s]);
        st.regs[d] = r;
        set_flags(st, r, false);
        Step::Continue
    } else if opc == op::AND || opc == op::OR || opc == op::XOR {
        let (a, b) = (st.regs[d], st.regs[s]);
        let r = if opc == op::AND {
            a & b
        } else if opc == op::OR {
            a | b
        } else {
            a ^ b
        };
        st.regs[d] = r;
        set_flags(st, r, false);
        Step::Continue
    } else if opc == op::NOT {
        let r = !st.regs[d];
        st.regs[d] = r;
        set_flags(st, r, false);
        Step::Continue
    } else if opc == op::SHL || opc == op::SHR || opc == op::SAR {
        let sh = imm & 0x1F;
        let a = st.regs[d];
        let r = if opc == op::SHL {
            a << sh
        } else if opc == op::SHR {
            a >> sh
        } else {
            ((a as i32) >> sh) as u32
        };
        st.regs[d] = r;
        set_flags(st, r, false);
        Step::Continue
    } else if opc == op::SHLR || opc == op::SHRR || opc == op::SARR {
        let sh = st.regs[s] & 0x1F;
        let a = st.regs[d];
        let r = if opc == op::SHLR {
            a << sh
        } else if opc == op::SHRR {
            a >> sh
        } else {
            ((a as i32) >> sh) as u32
        };
        st.regs[d] = r;
        set_flags(st, r, false);
        Step::Continue
    } else if opc == op::MULH {
        let a = st.regs[d] as i32 as i64;
        let b = st.regs[s] as i32 as i64;
        let r = ((a * b) >> 32) as u32;
        st.regs[d] = r;
        set_flags(st, r, false);
        Step::Continue
    } else if opc == op::MOV {
        st.regs[d] = st.regs[s];
        Step::Continue
    } else if opc == op::MOVI {
        st.regs[d] = imm;
        Step::Continue
    } else if opc == op::LOAD_IMM32 {
        st.regs[d] = (imm << 16) | (st.regs[d] & 0xFFFF);
        Step::Continue
    } else if opc == op::ADDI {
        let (r, c) = st.regs[d].overflowing_add(simm);
        st.regs[d] = r;
        set_flags(st, r, c);
        Step::Continue
    } else if opc == op::CMP {
        let (diff, borrow) = st.regs[d].overflowing_sub(st.regs[s]);
        set_flags(st, diff, borrow);
        Step::Continue
    } else if (op::JMP..=op::JNC).contains(&opc) {
        let f = st.flags;
        let taken = if opc == op::JMP {
            true
        } else if opc == op::JZ {
            f & mf::Z != 0
        } else if opc == op::JNZ {
            f & mf::Z == 0
        } else if opc == op::JN {
            f & mf::N != 0
        } else if opc == op::JP {
            f & mf::N == 0
        } else if opc == op::JC {
            f & mf::C != 0
        } else {
            f & mf::C == 0
        };
        if taken {
            // 24-bit signed offset from pc + 1 (engine semantics).
            let raw = w & 0x00FF_FFFF;
            let off = if raw & 0x0080_0000 != 0 {
                raw | 0xFF00_0000
            } else {
                raw
            };
            let target = next.wrapping_add(off);
            if target >= cfg.rom_words as u32 {
                st.status = status::DONE;
                return Step::Fault(FaultKind::Pc);
            }
            if target <= pc {
                st.back_edges = st.back_edges.wrapping_add(1);
                if st.back_edges > cfg.loop_bound as u32 || cfg.stage == STAGE_XDP {
                    st.status = status::DONE;
                    return Step::Fault(FaultKind::LoopBound);
                }
            }
            st.pc = target as u16;
        }
        Step::Continue
    } else if opc == op::LD || opc == op::LDB {
        let addr = st.regs[s].wrapping_add(simm);
        let word = if addr < (MEM_WORDS as u32) * 4 {
            Some(st.mem[mem_index(addr)])
        } else if addr >= DATA_BASE && addr - DATA_BASE < (cfg.data_words as u32) * 4 {
            data.word((addr - DATA_BASE) >> 2)
        } else {
            None
        };
        match word {
            Some(v) if opc == op::LDB => {
                st.regs[d] = (v >> ((addr & 3) * 8)) & 0xFF;
                Step::Continue
            }
            Some(v) if addr & 3 == 0 => {
                st.regs[d] = v;
                Step::Continue
            }
            _ => Step::Fault(FaultKind::Mem),
        }
    } else if opc == op::ST {
        let addr = st.regs[d].wrapping_add(simm);
        if addr < (MEM_WORDS as u32) * 4 && addr & 3 == 0 {
            st.mem[mem_index(addr)] = st.regs[s];
            Step::Continue
        } else {
            Step::Fault(FaultKind::Mem)
        }
    } else if opc == op::SYSCALL {
        syscall(st, cfg, insn.imm16(), io)
    } else if opc == op::HALT {
        Step::Halt
    } else {
        Step::Fault(FaultKind::BadOpcode)
    };
    if res != Step::Continue && res != Step::Yield {
        st.status = status::DONE;
    }
    res
}

#[inline(always)]
fn syscall<I: CartIo>(st: &mut CartState, cfg: &CartCfg, n: u16, io: &mut I) -> Step {
    let xdp = cfg.stage == STAGE_XDP;
    if n == sys::FIELD && xdp {
        match io.field(st.regs[1]) {
            Some(v) => {
                st.regs[0] = v;
                Step::Continue
            }
            None => Step::Fault(FaultKind::Field),
        }
    } else if n == sys::NOW_NS_LO {
        st.regs[0] = io.now_ns_lo();
        Step::Continue
    } else if n == sys::VERDICT && xdp {
        let code = st.regs[1];
        if code == verdict::PASS as u32 || code == verdict::DROP as u32 {
            Step::Verdict(code as u8)
        } else {
            Step::Fault(FaultKind::Verdict)
        }
    } else if n == sys::EMIT {
        io.emit(st.regs[1], st.regs[2], st.regs[3]);
        Step::Continue
    } else if n == sys::QUEUE && xdp {
        let a = st.regs[1];
        if a & 3 != 0 || a > (MEM_WORDS as u32) * 4 - (FEAT_WORDS as u32) * 4 {
            return Step::Fault(FaultKind::Mem);
        }
        let base = (a >> 2) as usize;
        let mut feat = [0u32; FEAT_WORDS];
        let mut i = 0;
        while i < FEAT_WORDS {
            feat[i] = st.mem[(base + i) & (MEM_WORDS - 1)];
            i += 1;
        }
        io.queue(&feat);
        Step::Continue
    } else if n == sys::YIELD && !xdp {
        st.status = status::YIELDED;
        Step::Yield
    } else {
        Step::Fault(FaultKind::Syscall)
    }
}

/// Run until the cartridge stops or the stage budget runs out, `N` being
/// the stage's const instruction cap ([`N_PKT`] or [`N_TICK`]). Resets
/// `insns` first. XDP budget-out is `Fault(Budget)`; tick budget-out is an
/// implicit `Yield`.
#[inline(always)]
pub fn run<const N: usize, R: CartRom + ?Sized, D: CartData + ?Sized, I: CartIo>(
    st: &mut CartState,
    cfg: &CartCfg,
    rom: &R,
    data: &D,
    io: &mut I,
) -> Step {
    st.insns = 0;
    for _ in 0..N {
        match run_one(st, cfg, rom, data, io) {
            Step::Continue => {}
            done => return done,
        }
    }
    run_out(st, cfg)
}

/// One iteration of [`run`]: `Continue` while the cartridge runs and has
/// budget left, otherwise how it stopped. BPF drives this from a `bpf_loop`
/// callback (one verified body for any N); [`run`] drives it from a plain
/// loop. Both then call [`run_out`] if every iteration returned `Continue`.
#[inline(always)]
pub fn run_one<R: CartRom + ?Sized, D: CartData + ?Sized, I: CartIo>(
    st: &mut CartState,
    cfg: &CartCfg,
    rom: &R,
    data: &D,
    io: &mut I,
) -> Step {
    if st.insns >= cfg.max_insn as u32 {
        return run_out(st, cfg);
    }
    step(st, cfg, rom, data, io)
}

/// How a run ends when its budget is spent: XDP `Fault(Budget)`, tick
/// implicit `Yield`.
#[inline(always)]
pub fn run_out(st: &mut CartState, cfg: &CartCfg) -> Step {
    if cfg.stage == STAGE_TICK {
        st.status = status::YIELDED;
        Step::Yield
    } else {
        st.status = status::DONE;
        Step::Fault(FaultKind::Budget)
    }
}

/// Reset `st` for a new XDP-stage run (`mem` and `gen` persist).
#[inline(always)]
pub fn begin_run(st: &mut CartState, entry: u16) {
    let mut i = 0;
    while i < 16 {
        // Volatile: a plain store loop becomes a memset call, which BPF with
        // --btf rejects (S5 Phase 11). Same result on the host.
        unsafe { core::ptr::write_volatile(&mut st.regs[i], 0) };
        i += 1;
    }
    // Volatile for the same reason: LLVM merges these adjacent zero stores
    // into a 10-byte memset otherwise.
    unsafe {
        core::ptr::write_volatile(&mut st.pc, entry);
        core::ptr::write_volatile(&mut st.flags, 0);
        core::ptr::write_volatile(&mut st.status, status::IDLE);
        core::ptr::write_volatile(&mut st.insns, 0);
        core::ptr::write_volatile(&mut st.back_edges, 0);
    }
}

/// Reset `st` for a new tick job seeded with `feat` in `mem[0..8]`.
#[inline(always)]
pub fn begin_job(st: &mut CartState, entry: u16, feat: &[u32; FEAT_WORDS]) {
    begin_run(st, entry);
    let mut i = 0;
    while i < MEM_WORDS {
        let v = if i < FEAT_WORDS { feat[i & 7] } else { 0 };
        unsafe { core::ptr::write_volatile(&mut st.mem[i], v) };
        i += 1;
    }
}

/// 32-bit FNV-1a over `regs` ‖ `pc` ‖ `flags` ‖ `mem` (little-endian), the
/// ADR-108 `state_hash`.
pub fn state_hash(st: &CartState) -> u32 {
    let mut h: u32 = 0x811c_9dc5;
    let mut eat = |b: u8| {
        h ^= b as u32;
        h = h.wrapping_mul(0x0100_0193);
    };
    for r in st.regs.iter() {
        for b in r.to_le_bytes() {
            eat(b);
        }
    }
    for b in st.pc.to_le_bytes() {
        eat(b);
    }
    eat(st.flags);
    for m in st.mem.iter() {
        for b in m.to_le_bytes() {
            eat(b);
        }
    }
    h
}

#[cfg(test)]
mod tests {
    extern crate std;
    use super::*;
    use std::vec::Vec;

    fn enc(o: u8, d: u8, s: u8, imm: u16) -> u32 {
        MbcInsn::encode(o, d, s, imm).0
    }
    /// Branch word with a 24-bit signed offset.
    fn br(o: u8, off: i32) -> u32 {
        ((o as u32) << 24) | ((off as u32) & 0x00FF_FFFF)
    }
    fn sc(n: u16) -> u32 {
        enc(op::SYSCALL, 0, 0, n)
    }

    #[derive(Default)]
    struct Io {
        fields: [u32; FIELD_COUNT as usize],
        now: u32,
        emits: Vec<(u32, u32, u32)>,
        queued: Vec<[u32; FEAT_WORDS]>,
    }
    impl CartIo for Io {
        fn field(&mut self, id: u32) -> Option<u32> {
            self.fields.get(id as usize).copied()
        }
        fn now_ns_lo(&mut self) -> u32 {
            self.now
        }
        fn emit(&mut self, kind: u32, a: u32, b: u32) {
            self.emits.push((kind, a, b));
        }
        fn queue(&mut self, feat: &[u32; FEAT_WORDS]) {
            self.queued.push(*feat);
        }
    }

    fn cfg(stage: u8, rom: &[u32], data: &[u32]) -> CartCfg {
        CartCfg {
            stage,
            max_insn: if stage == STAGE_XDP { 64 } else { 256 },
            rom_words: rom.len() as u16,
            data_words: data.len() as u16,
            entry: 0,
            loop_bound: if stage == STAGE_XDP { 0 } else { 100 },
        }
    }

    /// Run `rom` from a fresh state with r1..r3 preset; return (result, state).
    fn exec(
        stage: u8,
        rom: &[u32],
        data: &[u32],
        io: &mut Io,
        init: &[(usize, u32)],
    ) -> (Step, CartState) {
        let c = cfg(stage, rom, data);
        let mut st = CartState::zeroed();
        begin_run(&mut st, 0);
        for &(r, v) in init {
            st.regs[r] = v;
        }
        let res = if stage == STAGE_XDP {
            run::<N_PKT, _, _, _>(&mut st, &c, rom, data, io)
        } else {
            run::<N_TICK, _, _, _>(&mut st, &c, rom, data, io)
        };
        (res, st)
    }

    fn alu(o: u8, a: u32, b: u32) -> (u32, u8) {
        let rom = [enc(o, 2, 3, 0), enc(op::HALT, 0, 0, 0)];
        let (r, st) = exec(STAGE_XDP, &rom, &[], &mut Io::default(), &[(2, a), (3, b)]);
        assert_eq!(r, Step::Halt, "op {o:#x}");
        (st.regs[2], st.flags)
    }

    #[test]
    fn cart_iat_bucket() {
        assert_eq!(iat_bucket(0), 0); // log2(1)
        assert_eq!(iat_bucket(999), 0);
        assert_eq!(iat_bucket(1_000), 1); // 1 µs → log2(2)
        assert_eq!(iat_bucket(3_000), 2); // log2(4)
        assert_eq!(iat_bucket(1_000_000), 9); // 1 ms → log2(1001)
        assert_eq!(iat_bucket(1_000_000_000), 19); // 1 s
        assert_eq!(iat_bucket(u64::MAX), 31);
        for us in [1u64, 7, 100, 65_535, 1 << 20, (1 << 31) - 2] {
            let want = (63 - (us + 1).leading_zeros()).min(31);
            assert_eq!(iat_bucket(us * 1000), want, "{us} µs");
        }
    }

    /// Cross-language vector: pkg/cartridge's Go Hash() must give the same.
    #[test]
    fn cart_state_hash_vector() {
        let mut st = CartState::zeroed();
        for i in 0..16 {
            st.regs[i] = i as u32;
        }
        for i in 0..MEM_WORDS {
            st.mem[i] = 3 * i as u32;
        }
        st.pc = 7;
        st.flags = 5;
        assert_eq!(state_hash(&st), 0x6da0_d6f5);
    }

    #[test]
    fn cart_record_sizes() {
        assert_eq!(core::mem::size_of::<FlowKey>(), 40);
        assert_eq!(core::mem::size_of::<FlowMeta>(), 24);
        assert_eq!(core::mem::size_of::<ScoreJob>(), 80);
        assert_eq!(core::mem::size_of::<SlotCfg>(), 20);
        assert_eq!(core::mem::size_of::<CartEvent>(), 64);
    }

    #[test]
    fn cart_state_is_336_bytes() {
        assert_eq!(core::mem::size_of::<CartState>(), 336);
        assert_eq!(core::mem::align_of::<CartState>(), 4);
    }

    #[test]
    fn cart_alu_golden() {
        assert_eq!(alu(op::ADD, 2, 3), (5, 0));
        assert_eq!(alu(op::ADD, 0xFFFF_FFFF, 1), (0, mf::Z | mf::C));
        assert_eq!(alu(op::SUB, 3, 5), (0xFFFF_FFFE, mf::N | mf::C));
        assert_eq!(alu(op::SUB, 5, 5), (0, mf::Z));
        assert_eq!(alu(op::MUL, 0x10000, 0x10000), (0, mf::Z));
        assert_eq!(alu(op::MUL, 7, 6), (42, 0));
        assert_eq!(alu(op::AND, 0xF0F0, 0xFF00), (0xF000, 0));
        assert_eq!(alu(op::OR, 0xF0, 0x0F), (0xFF, 0));
        assert_eq!(alu(op::XOR, 0xFF, 0xFF), (0, mf::Z));
        assert_eq!(alu(op::NOT, 0, 0), (0xFFFF_FFFF, mf::N));
        assert_eq!(alu(op::SHLR, 1, 33), (2, 0)); // shift masked to 5 bits
        assert_eq!(alu(op::SHRR, 0x8000_0000, 31), (1, 0));
        assert_eq!(alu(op::SARR, 0x8000_0000, 31), (0xFFFF_FFFF, mf::N));
        assert_eq!(alu(op::MULH, (-2i32) as u32, 0x8000_0000), (1, 0));
        assert_eq!(alu(op::MOV, 1, 9), (9, 0));
    }

    #[test]
    fn cart_immediates_golden() {
        let rom = [
            enc(op::MOVI, 1, 0, 0xFFFF),       // zero-extend
            enc(op::LOAD_IMM32, 1, 0, 0x1234), // upper half
            enc(op::MOVI, 2, 0, 5),
            enc(op::ADDI, 2, 0, 0xFFFF), // -1
            enc(op::MOVI, 3, 0, 1),
            enc(op::SHL, 3, 0, 4),
            enc(op::MOVI, 4, 0, 0x80),
            enc(op::SHR, 4, 0, 3),
            enc(op::MOVI, 5, 0, 0),
            enc(op::NOT, 5, 0, 0),
            enc(op::SAR, 5, 0, 8),
            enc(op::HALT, 0, 0, 0),
        ];
        let (r, st) = exec(STAGE_XDP, &rom, &[], &mut Io::default(), &[]);
        assert_eq!(r, Step::Halt);
        assert_eq!(st.regs[1], 0x1234_FFFF);
        assert_eq!(st.regs[2], 4);
        assert_eq!(st.regs[3], 16);
        assert_eq!(st.regs[4], 0x10);
        assert_eq!(st.regs[5], 0xFFFF_FFFF);
    }

    #[test]
    fn cart_mov_and_loads_keep_flags() {
        // CMP sets Z; MOV/MOVI/LOAD_IMM32 leave it alone (engine semantics).
        let rom = [
            enc(op::CMP, 1, 2, 0),
            enc(op::MOV, 3, 1, 0),
            enc(op::MOVI, 4, 0, 7),
            enc(op::LOAD_IMM32, 4, 0, 1),
            enc(op::HALT, 0, 0, 0),
        ];
        let (_, st) = exec(STAGE_XDP, &rom, &[], &mut Io::default(), &[(1, 9), (2, 9)]);
        assert_eq!(st.flags, mf::Z);
    }

    #[test]
    fn cart_branches_golden() {
        // CMP r1,r2 ; JZ +1 ; MOVI r3,1 ; MOVI r4,2 ; HALT
        let rom = [
            enc(op::CMP, 1, 2, 0),
            br(op::JZ, 1),
            enc(op::MOVI, 3, 0, 1),
            enc(op::MOVI, 4, 0, 2),
            enc(op::HALT, 0, 0, 0),
        ];
        let (_, st) = exec(STAGE_XDP, &rom, &[], &mut Io::default(), &[(1, 4), (2, 4)]);
        assert_eq!((st.regs[3], st.regs[4]), (0, 2), "taken");
        let (_, st) = exec(STAGE_XDP, &rom, &[], &mut Io::default(), &[(1, 4), (2, 5)]);
        assert_eq!((st.regs[3], st.regs[4]), (1, 2), "not taken");
        // JN / JP / JC / JNC / JNZ / JMP
        for (o, a, b, taken) in [
            (op::JN, 1u32, 2u32, true),
            (op::JN, 2, 1, false),
            (op::JP, 2, 1, true),
            (op::JC, 1, 2, true),
            (op::JC, 2, 1, false),
            (op::JNC, 2, 1, true),
            (op::JNZ, 1, 2, true),
            (op::JNZ, 1, 1, false),
            (op::JMP, 1, 1, true),
        ] {
            let rom = [
                enc(op::CMP, 1, 2, 0),
                br(o, 1),
                enc(op::MOVI, 3, 0, 1),
                enc(op::HALT, 0, 0, 0),
            ];
            let (_, st) = exec(STAGE_XDP, &rom, &[], &mut Io::default(), &[(1, a), (2, b)]);
            assert_eq!(st.regs[3] == 0, taken, "op {o:#x} a={a} b={b}");
        }
    }

    #[test]
    fn cart_branch_out_of_rom_faults() {
        let rom = [br(op::JMP, 5), enc(op::HALT, 0, 0, 0)];
        let (r, _) = exec(STAGE_XDP, &rom, &[], &mut Io::default(), &[]);
        assert_eq!(r, Step::Fault(FaultKind::Pc));
        let rom = [enc(op::NOP, 0, 0, 0)]; // runs off the end
        let (r, _) = exec(STAGE_XDP, &rom, &[], &mut Io::default(), &[]);
        assert_eq!(r, Step::Fault(FaultKind::Pc));
    }

    #[test]
    fn cart_back_edges() {
        // r1 = 3; loop: ADDI r1,-1 ; JNZ -2 ; HALT
        let rom = [
            enc(op::MOVI, 1, 0, 3),
            enc(op::ADDI, 1, 0, 0xFFFF),
            br(op::JNZ, -2),
            enc(op::HALT, 0, 0, 0),
        ];
        // XDP: any back-edge faults.
        let (r, _) = exec(STAGE_XDP, &rom, &[], &mut Io::default(), &[]);
        assert_eq!(r, Step::Fault(FaultKind::LoopBound));
        // Tick: 2 taken back-edges, within loop_bound 100.
        let (r, st) = exec(STAGE_TICK, &rom, &[], &mut Io::default(), &[]);
        assert_eq!(r, Step::Halt);
        assert_eq!((st.regs[1], st.back_edges), (0, 2));
        // Tick with loop_bound 1: the second back-edge faults.
        let mut c = cfg(STAGE_TICK, &rom, &[]);
        c.loop_bound = 1;
        let mut st = CartState::zeroed();
        begin_run(&mut st, 0);
        let r = run::<N_TICK, _, _, _>(&mut st, &c, &rom[..], &[][..], &mut Io::default());
        assert_eq!(r, Step::Fault(FaultKind::LoopBound));
    }

    #[test]
    fn cart_budgets() {
        // Infinite forward NOPs are impossible (ROM is finite), so use a
        // long straight line: 70 NOPs + HALT exceeds the XDP cap of 64.
        let mut rom = std::vec![enc(op::NOP, 0, 0, 0); 70];
        rom.push(enc(op::HALT, 0, 0, 0));
        let (r, st) = exec(STAGE_XDP, &rom, &[], &mut Io::default(), &[]);
        assert_eq!(r, Step::Fault(FaultKind::Budget));
        assert_eq!(st.insns, 64);
        // Tick: budget-out is an implicit yield, and the job resumes.
        let mut c = cfg(STAGE_TICK, &rom, &[]);
        c.max_insn = 50;
        let mut st = CartState::zeroed();
        begin_run(&mut st, 0);
        let r = run::<N_TICK, _, _, _>(&mut st, &c, &rom[..], &[][..], &mut Io::default());
        assert_eq!((r, st.pc, st.status), (Step::Yield, 50, status::YIELDED));
        let r = run::<N_TICK, _, _, _>(&mut st, &c, &rom[..], &[][..], &mut Io::default());
        assert_eq!((r, st.status), (Step::Halt, status::DONE));
    }

    #[test]
    fn cart_memory_windows() {
        let data = [0x1122_3344u32, 0xAABB_CCDD];
        let rom = [
            enc(op::MOVI, 1, 0, 0x2A),
            enc(op::ST, 0, 1, 0x10),    // mem[4] = 0x2A   (base r0 = 0)
            enc(op::LD, 2, 0, 0x10),    // r2 = mem[4]
            enc(op::LD, 3, 0, 0x1004),  // r3 = DATA[1]
            enc(op::LDB, 4, 0, 0x1001), // r4 = byte 1 of DATA[0] = 0x33
            enc(op::LDB, 5, 0, 0x10),   // r5 = byte 0 of mem[4]
            enc(op::HALT, 0, 0, 0),
        ];
        let (r, st) = exec(STAGE_XDP, &rom, &data, &mut Io::default(), &[]);
        assert_eq!(r, Step::Halt);
        assert_eq!(st.mem[4], 0x2A);
        assert_eq!(
            (st.regs[2], st.regs[3], st.regs[4], st.regs[5]),
            (0x2A, 0xAABB_CCDD, 0x33, 0x2A)
        );
        let fault = |w: u32| {
            let rom = [w, enc(op::HALT, 0, 0, 0)];
            exec(STAGE_XDP, &rom, &data, &mut Io::default(), &[]).0
        };
        assert_eq!(
            fault(enc(op::LD, 1, 0, 0x100)),
            Step::Fault(FaultKind::Mem),
            "past mem"
        );
        assert_eq!(
            fault(enc(op::LD, 1, 0, 0x2000)),
            Step::Fault(FaultKind::Mem),
            "between windows"
        );
        assert_eq!(
            fault(enc(op::LD, 1, 0, 0x1008)),
            Step::Fault(FaultKind::Mem),
            "past data"
        );
        assert_eq!(
            fault(enc(op::ST, 0, 1, 0x1000)),
            Step::Fault(FaultKind::Mem),
            "store to DATA"
        );
        assert_eq!(
            fault(enc(op::LD, 1, 0, 0x2)),
            Step::Fault(FaultKind::Mem),
            "misaligned LD"
        );
        assert_eq!(
            fault(enc(op::ST, 0, 1, 0x3)),
            Step::Fault(FaultKind::Mem),
            "misaligned ST"
        );
        assert_eq!(
            fault(enc(op::LD, 1, 0, 0xFFFC)),
            Step::Fault(FaultKind::Mem),
            "negative offset wraps"
        );
    }

    #[test]
    fn cart_disallowed_opcodes_fault() {
        for o in [
            op::DIV,
            op::MOD,
            op::NEG,
            op::PUSH,
            op::POP,
            op::CALL,
            op::RET,
            op::JMPR,
            op::CALLR,
            op::STB,
            op::LDH,
            op::STH,
            op::INT,
            op::IRET,
            op::CLI,
            op::STI,
            op::XCHG,
            op::CAS,
            op::FENCE,
            op::MRET,
            op::SRET,
            op::LR_W,
            op::SC_W,
            op::MULHU,
            0x11,
            0x50,
            0xFE,
        ] {
            let rom = [enc(o, 0, 0, 0), enc(op::HALT, 0, 0, 0)];
            let (r, _) = exec(STAGE_XDP, &rom, &[], &mut Io::default(), &[]);
            assert_eq!(r, Step::Fault(FaultKind::BadOpcode), "op {o:#x}");
        }
    }

    #[test]
    fn cart_syscalls_xdp() {
        let mut io = Io::default();
        io.fields[field::DST_PORT as usize] = 9;
        io.now = 77;
        let rom = [
            enc(op::MOVI, 1, 0, field::DST_PORT as u16),
            sc(sys::FIELD),
            enc(op::MOV, 5, 0, 0),
            sc(sys::NOW_NS_LO),
            enc(op::MOV, 6, 0, 0),
            enc(op::MOVI, 1, 0, 7),
            enc(op::MOVI, 2, 0, 8),
            enc(op::MOVI, 3, 0, 9),
            sc(sys::EMIT),
            enc(op::MOVI, 1, 0, 0x20),
            sc(sys::QUEUE),
            enc(op::MOVI, 1, 0, 1),
            sc(sys::VERDICT),
            enc(op::HALT, 0, 0, 0),
        ];
        let c = cfg(STAGE_XDP, &rom, &[]);
        let mut st = CartState::zeroed();
        st.mem[8] = 0xF00D; // feature word 0 of the queued snapshot
        begin_run(&mut st, 0);
        let r = run::<N_PKT, _, _, _>(&mut st, &c, &rom[..], &[][..], &mut io);
        assert_eq!(r, Step::Verdict(verdict::DROP));
        assert_eq!((st.regs[5], st.regs[6]), (9, 77));
        assert_eq!(io.emits, std::vec![(7, 8, 9)]);
        assert_eq!(io.queued.len(), 1);
        assert_eq!(io.queued[0][0], 0xF00D);
    }

    #[test]
    fn cart_syscall_faults() {
        let f = |stage: u8, pre: &[(usize, u32)], n: u16| {
            let rom = [sc(n), enc(op::HALT, 0, 0, 0)];
            exec(stage, &rom, &[], &mut Io::default(), pre).0
        };
        assert_eq!(
            f(STAGE_XDP, &[(1, 13)], sys::FIELD),
            Step::Fault(FaultKind::Field)
        );
        assert_eq!(
            f(STAGE_XDP, &[(1, 2)], sys::VERDICT),
            Step::Fault(FaultKind::Verdict)
        );
        assert_eq!(
            f(STAGE_XDP, &[], sys::YIELD),
            Step::Fault(FaultKind::Syscall)
        );
        assert_eq!(f(STAGE_XDP, &[], 6), Step::Fault(FaultKind::Syscall));
        assert_eq!(
            f(STAGE_TICK, &[], sys::FIELD),
            Step::Fault(FaultKind::Syscall)
        );
        assert_eq!(
            f(STAGE_TICK, &[], sys::VERDICT),
            Step::Fault(FaultKind::Syscall)
        );
        assert_eq!(
            f(STAGE_TICK, &[], sys::QUEUE),
            Step::Fault(FaultKind::Syscall)
        );
        assert_eq!(
            f(STAGE_XDP, &[(1, 0xE4)], sys::QUEUE),
            Step::Fault(FaultKind::Mem),
            "snapshot past the window"
        );
        assert_eq!(
            f(STAGE_XDP, &[(1, 0x22)], sys::QUEUE),
            Step::Fault(FaultKind::Mem),
            "misaligned snapshot"
        );
        assert_eq!(
            f(STAGE_XDP, &[(1, 0xE0)], sys::QUEUE),
            Step::Halt,
            "last legal snapshot"
        );
        assert_eq!(f(STAGE_TICK, &[], sys::YIELD), Step::Yield);
    }

    #[test]
    fn cart_tick_job_seeds_features() {
        let feat = [1, 2, 3, 4, 5, 6, 7, 8];
        let mut st = CartState::zeroed();
        st.mem[40] = 99;
        begin_job(&mut st, 3, &feat);
        assert_eq!(&st.mem[..8], &feat);
        assert_eq!(st.mem[40], 0);
        assert_eq!(st.pc, 3);
    }

    #[test]
    fn cart_state_hash_sees_every_bit() {
        let mut st = CartState::zeroed();
        let h0 = state_hash(&st);
        for bit in 0..32 {
            st.mem[63] ^= 1 << bit;
            assert_ne!(state_hash(&st), h0);
            st.mem[63] ^= 1 << bit;
        }
        st.pc = 1;
        assert_ne!(state_hash(&st), h0);
        st.pc = 0;
        st.flags = mf::C;
        assert_ne!(state_hash(&st), h0);
        st.flags = 0;
        st.insns = 5; // not part of the hash (ADR-108)
        assert_eq!(state_hash(&st), h0);
        assert_eq!(state_hash(&CartState::zeroed()), h0);
    }
}
