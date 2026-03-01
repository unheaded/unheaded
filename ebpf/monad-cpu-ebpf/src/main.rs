//! Unheaded Protocol — Monad CPU eBPF Program
//!
//! The **Monad CPU** is the Doom-over-IPv6 proof-of-concept: a complete
//! MBC (Monad Bytecode) virtual machine running entirely inside eBPF XDP.
//!
//! # How It Works
//!
//! A userspace timer injects "CPU tick" IPv6 packets at 35 Hz.  Each tick
//! packet carries a Hop-by-Hop Monad with the `CUSTOM` flag set and a flow
//! label identifying the CPU instance (low 8 bits = instance_id, 0-255).
//!
//! On receipt this XDP program:
//! 1. Identifies the tick packet (IPv6 + HBH + Monad.flags & CUSTOM)
//! 2. Loads the `MbcCpuState` for the instance from `CPU_MAP`
//! 3. If halted or sleeping: returns XDP_DROP immediately
//! 4. Executes up to `MAX_INSN_PER_TICK` MBC instructions:
//!    a. Fetch: `ROM_MAP[cpu.pc]` → `MbcInsn`
//!    b. Decode: opcode / dst / src / imm16
//!    c. Execute: mutate `cpu.regs`, `cpu.pc`, `cpu.flags`
//! 5. Writes mutated `MbcCpuState` back to `CPU_MAP`
//! 6. Returns XDP_DROP (tick packets are control, not data)
//!
//! # Memory Map
//!
//! | Address range                | Backing map     | Description         |
//! |------------------------------|-----------------|---------------------|
//! | 0x0000_0000 – 0x0000_BFFF   | `RAM_MAP`       | 48 KiB general RAM  |
//! | 0x0000_C000 – 0x0000_F8BF   | `SCREEN_MAP`    | 320×200 framebuffer |
//! | 0x0000_FFFF                  | `KBD_MAP[0]`   | Keyboard state word |
//! | 0x0001_0000 – 0x0040_FFFF   | `RAM_MAP`       | WAD data (4 MiB)    |
//!
//! # MBC ISA Summary
//!
//! 32-bit fixed-width instructions: `[opcode:8][dst:4][src:4][imm16:16]`.
//! 16 general-purpose 32-bit registers (r0–r15, r15=SP).
//! Status flags: Z (zero), N (negative), C (carry).
//!
//! # BPF Verifier Notes
//!
//! The execute loop is bounded by `MAX_INSN_PER_TICK` (constant).  All map
//! accesses use `get_ptr_mut` / `get` with NULL checks.  No unbounded loops.

#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::xdp_action,
    helpers::bpf_ktime_get_ns,
    macros::{map, xdp},
    maps::{Array, HashMap, LruHashMap, RingBuf},
    programs::XdpContext,
};
use monad_common::{
    flags, mbc_flags as mf, mbc_mmap as mmap, mbc_opcodes as op, mbc_syscalls as sys,
    ComputeHopEvent, MbcCpuState, MbcInsn, Monad, EVENT_CACHE_MISS, EVENT_COMPUTE_HALT,
    EVENT_SCREEN_WRITE, IPV6_FIXED_HDR_LEN, IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN, MONAD_OPT_TYPE,
    MONAD_SIZE,
};

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// ROM: MBC program instructions, u32 per slot.
/// Index = PC value.  Loaded by Wotan trace-collector from .mbc binary.
/// 262 144 entries = 1 MiB of instructions.
#[map]
static ROM_MAP: Array<u32> = Array::with_max_entries(262_144, 0);

/// RAM: word-addressable memory backed by a BPF Array.
/// Index = word address (byte_addr >> 2 for LD/ST, direct for CALL/RET stack).
/// 16M entries (64 MiB kernel memory) covers Doom's full address space:
///   - LD/ST: byte_addr >> 2, max ~4M (stack at 0x1000000 → word 0x400000)
///   - CALL/RET: SP used as word addr directly, max ~0x1000000 (16M)
/// Array never fills up (unlike HashMap) — in-range indices always succeed.
/// Zero-initialized by default (all entries start at 0).
#[map]
static RAM_MAP: Array<u32> = Array::with_max_entries(16_777_216, 0);

/// Screen framebuffer: 320×200 = 64 000 bytes, 8-bit palette indices.
/// Pixel (x, y) → SCREEN_MAP[y * 320 + x].
/// Userspace polls this at 35 Hz to render the current frame.
#[map]
static SCREEN_MAP: Array<u8> = Array::with_max_entries(64_000, 0);

/// Keyboard state: 8 entries, one u32 per active key slot.
/// Encoding: `(scancode << 1) | pressed_flag`
/// Userspace writes key events here.  CPU reads via SYS_GET_KEY.
#[map]
static KBD_MAP: Array<u32> = Array::with_max_entries(8, 0);

/// CPU instance state table: 256 instances.
/// Key = instance_id (u32, low 8 bits of Flow Label).
/// Value = MbcCpuState (80 bytes: 16×u32 regs + pc + flags + halted + pad + sleep_ns).
#[map]
static CPU_MAP: HashMap<u32, MbcCpuState> = HashMap::with_max_entries(256, 0);

/// Statistics counters.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

/// L1 cache: reserved for future use.
/// Currently disabled because RAM_MAP is a BPF Array (O(1) direct indexing),
/// which is strictly faster than LruHashMap lookups (hash + LRU update).
/// The cache concept applies when the backing store is a HashMap or has
/// cross-CPU contention.  With a per-instance Array, every access is O(1).
#[map]
static L1_CACHE: LruHashMap<u32, [u8; 64]> = LruHashMap::with_max_entries(256, 0);

/// RV32I-to-MBC address translation map.
/// Index = RV32I word index (byte_addr >> 2), Value = MBC PC index.
/// Used by JMPR/CALLR to translate function pointers stored as RISC-V
/// byte addresses into the correct MBC ROM index.
/// 65,536 entries covers up to 262,144 bytes of .text (46,132 RV insns for Doom).
#[map]
static RV2MBC_MAP: Array<u32> = Array::with_max_entries(65_536, 0);

/// Compute events ring buffer: emits ComputeHopEvent on cache miss, screen write, halt.
#[map]
static COMPUTE_EVENTS: RingBuf = RingBuf::with_byte_size(262_144, 0);

// ── Stat keys ─────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_CPU_TICKS: u32 = 1;
const STAT_INSNS_EXECUTED: u32 = 2;
const STAT_HALTED: u32 = 3;
const STAT_SLEEPING: u32 = 4;
const STAT_NO_STATE: u32 = 5;
#[allow(dead_code)]
const STAT_MEM_FAULTS: u32 = 6;
const STAT_SYSCALLS: u32 = 7;
const STAT_ROM_FAULT: u32 = 8;
const STAT_MEM_STORES: u32 = 9; // was CACHE_HITS (misleading — cache is disabled)
const STAT_MEM_LOADS: u32 = 10; // was CACHE_MISSES (all loads go direct to Array)

// ── Tuning constants ──────────────────────────────────────────────────────────

/// Maximum MBC instructions to execute per XDP invocation.
///
/// Each XDP invocation executes up to N MBC instructions.
/// BPF verifier limits: 8192 jump complexity, 1M processed insns.
/// Verifier state-explores each opcode branch per iteration.
const MAX_INSN_PER_TICK: usize = 256;

// ── Wire-format helpers ───────────────────────────────────────────────────────

#[repr(C, packed)]
struct EthHdr {
    _dst: [u8; 6],
    _src: [u8; 6],
    proto: u16,
}

#[repr(C, packed)]
struct Ipv6Hdr {
    vtf: u32,
    payload_len: u16,
    next_header: u8,
    hop_limit: u8,
    _src: [u8; 16],
    _dst: [u8; 16],
}

const ETH_HLEN: usize = 14;
const ETH_P_IPV6: u16 = 0x86DD;

// ── XDP entry point ───────────────────────────────────────────────────────────

#[xdp]
pub fn monad_cpu(ctx: XdpContext) -> u32 {
    match try_monad_cpu(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_monad_cpu(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_PACKETS_TOTAL);

    let data = ctx.data();
    let data_end = ctx.data_end();

    // ── Ethernet ──────────────────────────────────────────────────────────────
    if data + ETH_HLEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let eth = unsafe { &*(data as *const EthHdr) };
    if u16::from_be(eth.proto) != ETH_P_IPV6 {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── IPv6 ──────────────────────────────────────────────────────────────────
    let ip_start = data + ETH_HLEN;
    if ip_start + IPV6_FIXED_HDR_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let ip = unsafe { &*(ip_start as *const Ipv6Hdr) };
    if ip.next_header != IPV6_NEXTHDR_HBH {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── HBH + Monad ───────────────────────────────────────────────────────────
    let hbh_start = ip_start + IPV6_FIXED_HDR_LEN;
    // Minimum 24 bytes: hbh(2) + opt_type(1) + opt_data_len(1) + monad(20)
    if hbh_start + 24 > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    let opt_type: u8 = unsafe { core::ptr::read_volatile((hbh_start + 2) as *const u8) };
    let opt_data_len: u8 = unsafe { core::ptr::read_volatile((hbh_start + 3) as *const u8) };
    if opt_type != MONAD_OPT_TYPE || opt_data_len != MONAD_OPT_DATA_LEN {
        return Ok(xdp_action::XDP_PASS);
    }

    let monad_start = hbh_start + 4;
    if monad_start + MONAD_SIZE > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let monad = read_monad_xdp(monad_start, data_end)?;

    // ── CPU tick identification ────────────────────────────────────────────────
    // CPU tick packets carry the CUSTOM flag.  All other Kingdom packets pass.
    if !monad.has_flag(flags::CUSTOM) {
        return Ok(xdp_action::XDP_PASS);
    }
    increment_stat(STAT_CPU_TICKS);

    // ── Extract instance_id from Flow Label ───────────────────────────────────
    let vtf = u32::from_be(unsafe { core::ptr::read_unaligned(core::ptr::addr_of!(ip.vtf)) });
    let flow_label = vtf & 0x000F_FFFF;
    let instance = flow_label & 0xFF; // low 8 bits → 256 possible instances

    // ── Load CPU state ────────────────────────────────────────────────────────
    let cpu_ptr = match CPU_MAP.get_ptr_mut(&instance) {
        Some(p) => p,
        None => {
            increment_stat(STAT_NO_STATE);
            return Ok(xdp_action::XDP_DROP); // tick consumed, no instance
        }
    };
    let cpu = unsafe { &mut *cpu_ptr };

    // Extract hop_id from the Monad for event emission
    let hop_id = monad.hop_count;

    // ── Halted check ──────────────────────────────────────────────────────────
    if cpu.halted != 0 {
        increment_stat(STAT_HALTED);
        return Ok(xdp_action::XDP_DROP);
    }

    // ── Sleep check ───────────────────────────────────────────────────────────
    let now = unsafe { bpf_ktime_get_ns() };
    if cpu.sleep_until_ns > 0 && now < cpu.sleep_until_ns {
        increment_stat(STAT_SLEEPING);
        return Ok(xdp_action::XDP_DROP);
    }
    if cpu.sleep_until_ns > 0 && now >= cpu.sleep_until_ns {
        cpu.sleep_until_ns = 0;
    }

    // ── Execute loop ──────────────────────────────────────────────────────────
    let mut i = 0usize;
    while i < MAX_INSN_PER_TICK {
        if cpu.halted != 0 {
            break;
        }

        // Fetch
        let insn_word = match ROM_MAP.get(cpu.pc) {
            Some(w) => *w,
            None => {
                // PC out of bounds — halt the CPU.
                cpu.halted = 1;
                increment_stat(STAT_ROM_FAULT);
                break;
            }
        };
        let insn = MbcInsn(insn_word);
        let opc = insn.opcode();
        let d = (insn.dst() as usize) & 0x0F;
        let s = (insn.src() as usize) & 0x0F;
        let imm = insn.imm16() as u32;
        let simm = insn.imm16_signed() as i32;

        // Advance PC before execution (branches will overwrite if taken).
        cpu.pc = cpu.pc.wrapping_add(1);

        // For branch instructions: 24-bit signed offset (bits 23:0, sign-extended from bit 23).
        // Matches CALL's 24-bit addressing for large programs.
        let branch_raw = insn_word & 0x00FF_FFFF;
        let branch_offset = if branch_raw & 0x0080_0000 != 0 {
            (branch_raw | 0xFF00_0000) as i32
        } else {
            branch_raw as i32
        };

        // ── No-op ─────────────────────────────────────────────────────────────
        if opc == op::NOP {
            // No operation — just advance PC (already done above).

            // ── Arithmetic ────────────────────────────────────────────────────────
        } else if opc == op::ADD {
            let (r, c) = cpu.regs[d].overflowing_add(cpu.regs[s]);
            cpu.regs[d] = r;
            set_flags(cpu, r, c);
        } else if opc == op::SUB {
            let (r, b) = cpu.regs[d].overflowing_sub(cpu.regs[s]);
            cpu.regs[d] = r;
            set_flags(cpu, r, b);
        } else if opc == op::MUL {
            let r = cpu.regs[d].wrapping_mul(cpu.regs[s]);
            cpu.regs[d] = r;
            set_flags(cpu, r, false);
        } else if opc == op::DIV {
            if cpu.regs[s] == 0 {
                cpu.regs[d] = 0xFFFF_FFFF; // division by zero → saturate
            } else {
                let r = cpu.regs[d] / cpu.regs[s];
                cpu.regs[d] = r;
                set_flags(cpu, r, false);
            }
        } else if opc == op::MOD {
            if cpu.regs[s] == 0 {
                cpu.regs[d] = 0;
            } else {
                let r = cpu.regs[d] % cpu.regs[s];
                cpu.regs[d] = r;
                set_flags(cpu, r, false);
            }
        } else if opc == op::NEG {
            let r = (cpu.regs[d] as i32).wrapping_neg() as u32;
            cpu.regs[d] = r;
            set_flags(cpu, r, false);

        // ── Logical / Bitwise ─────────────────────────────────────────────────
        } else if opc == op::AND {
            let r = cpu.regs[d] & cpu.regs[s];
            cpu.regs[d] = r;
            set_flags(cpu, r, false);
        } else if opc == op::OR {
            let r = cpu.regs[d] | cpu.regs[s];
            cpu.regs[d] = r;
            set_flags(cpu, r, false);
        } else if opc == op::XOR {
            let r = cpu.regs[d] ^ cpu.regs[s];
            cpu.regs[d] = r;
            set_flags(cpu, r, false);
        } else if opc == op::NOT {
            let r = !cpu.regs[d];
            cpu.regs[d] = r;
            set_flags(cpu, r, false);
        } else if opc == op::SHL {
            let shift = (imm & 0x1F) as u32;
            let r = cpu.regs[d] << shift;
            cpu.regs[d] = r;
            set_flags(cpu, r, false);
        } else if opc == op::SHR {
            let shift = (imm & 0x1F) as u32;
            let r = cpu.regs[d] >> shift;
            cpu.regs[d] = r;
            set_flags(cpu, r, false);
        } else if opc == op::SAR {
            let shift = (imm & 0x1F) as u32;
            let r = ((cpu.regs[d] as i32) >> shift) as u32;
            cpu.regs[d] = r;
            set_flags(cpu, r, false);
        } else if opc == op::SHLR {
            let shift = cpu.regs[s] & 0x1F;
            let r = cpu.regs[d] << shift;
            cpu.regs[d] = r;
            set_flags(cpu, r, false);
        } else if opc == op::SHRR {
            let shift = cpu.regs[s] & 0x1F;
            let r = cpu.regs[d] >> shift;
            cpu.regs[d] = r;
            set_flags(cpu, r, false);
        } else if opc == op::SARR {
            let shift = cpu.regs[s] & 0x1F;
            let r = ((cpu.regs[d] as i32) >> shift) as u32;
            cpu.regs[d] = r;
            set_flags(cpu, r, false);
        } else if opc == op::MULH {
            let a = cpu.regs[d] as i32 as i64;
            let b = cpu.regs[s] as i32 as i64;
            let r = ((a * b) >> 32) as u32;
            cpu.regs[d] = r;
            set_flags(cpu, r, false);

        // ── Stack operations ─────────────────────────────────────────────────
        // SP is a byte address (consistent with LD/ST). Each entry is 4 bytes.
        } else if opc == op::PUSH {
            cpu.regs[15] = cpu.regs[15].wrapping_sub(4);
            let word_addr = cpu.regs[15] >> 2;
            mem_write_word(word_addr, cpu.regs[d]);
        } else if opc == op::POP {
            let word_addr = cpu.regs[15] >> 2;
            cpu.regs[d] = mem_read_word(word_addr);
            cpu.regs[15] = cpu.regs[15].wrapping_add(4);

        // ── Extended immediate ───────────────────────────────────────────────
        } else if opc == op::LOAD_IMM32 {
            // regs[d][31:16] = imm16, preserve lower 16 bits.
            cpu.regs[d] = (imm << 16) | (cpu.regs[d] & 0xFFFF);
        } else if opc == op::ADDI {
            // dst = dst + sign_extend(imm16)
            let sext = imm as u16 as i16 as i32 as u32;
            let (r, c) = cpu.regs[d].overflowing_add(sext);
            cpu.regs[d] = r;
            set_flags(cpu, r, c);

        // ── Register moves ────────────────────────────────────────────────────
        } else if opc == op::MOV {
            cpu.regs[d] = cpu.regs[s];
        } else if opc == op::MOVI {
            cpu.regs[d] = imm;

        // ── Compare ───────────────────────────────────────────────────────────
        } else if opc == op::CMP {
            let rd = cpu.regs[d];
            let rs = cpu.regs[s];
            let (diff, borrow) = rd.overflowing_sub(rs);
            // Z: equal, N: signed less-than (MSB of diff set), C: unsigned borrow
            cpu.flags = 0;
            if diff == 0 {
                cpu.flags |= mf::Z;
            }
            if diff & 0x8000_0000 != 0 {
                cpu.flags |= mf::N;
            }
            if borrow {
                cpu.flags |= mf::C;
            }

        // ── Branches ─────────────────────────────────────────────────────────
        } else if opc == op::JMP {
            // Unconditional relative branch (24-bit offset).
            cpu.pc = cpu.pc.wrapping_add(branch_offset as u32);
        } else if opc == op::JZ {
            if cpu.flags & mf::Z != 0 {
                cpu.pc = cpu.pc.wrapping_add(branch_offset as u32);
            }
        } else if opc == op::JNZ {
            if cpu.flags & mf::Z == 0 {
                cpu.pc = cpu.pc.wrapping_add(branch_offset as u32);
            }
        } else if opc == op::JN {
            if cpu.flags & mf::N != 0 {
                cpu.pc = cpu.pc.wrapping_add(branch_offset as u32);
            }
        } else if opc == op::JP {
            if cpu.flags & mf::N == 0 {
                cpu.pc = cpu.pc.wrapping_add(branch_offset as u32);
            }
        } else if opc == op::JC {
            if cpu.flags & mf::C != 0 {
                cpu.pc = cpu.pc.wrapping_add(branch_offset as u32);
            }
        } else if opc == op::JNC {
            if cpu.flags & mf::C == 0 {
                cpu.pc = cpu.pc.wrapping_add(branch_offset as u32);
            }
        } else if opc == op::CALL {
            // Push PC (already incremented = return address) to stack.
            // SP is byte address; each stack entry is 4 bytes.
            let old_pc = cpu.pc.wrapping_sub(1); // PC of this CALL instruction
            cpu.regs[15] = cpu.regs[15].wrapping_sub(4);
            let word_addr = cpu.regs[15] >> 2;
            mem_write_word(word_addr, cpu.pc);
            // Extended addressing: use all 24 bits below opcode for target.
            let target = insn_word & 0x00FF_FFFF;
            cpu.pc = target;
            // Restart trap: catch direct call to PC 0 (translator bug)
            if cpu.pc == 0 {
                mem_write_word(0xE0000 >> 2, 0xDEAD0001); // sentinel
                mem_write_word(0xE0004 >> 2, 0x27); // CALL opcode
                mem_write_word(0xE0008 >> 2, old_pc); // MBC PC of CALL insn
                mem_write_word(0xE000C >> 2, target); // target was 0
                mem_write_word(0xE0010 >> 2, cpu.regs[15]); // SP
                cpu.halted = 1;
                increment_stat(STAT_ROM_FAULT);
                break;
            }
        } else if opc == op::JMPR {
            // Indirect jump with RV32I→MBC address translation.
            // regs[dst] holds a RISC-V byte address (e.g. function pointer from .data).
            // Convert to RV word index, look up the MBC PC in RV2MBC_MAP.
            let old_pc = cpu.pc.wrapping_sub(1); // PC of this JMPR instruction
            let rv_addr = cpu.regs[d];
            let rv_word = rv_addr >> 2;
            cpu.pc = match RV2MBC_MAP.get(rv_word) {
                Some(mbc_idx) => *mbc_idx,
                None => {
                    // Unmapped address — halt + write diagnostics.
                    mem_write_word(0xE0000 >> 2, 0xDEAD0002); // sentinel (unmapped)
                    mem_write_word(0xE0004 >> 2, opc as u32); // 0x29 = JMPR
                    mem_write_word(0xE0008 >> 2, old_pc); // MBC PC of JMPR insn
                    mem_write_word(0xE000C >> 2, rv_addr); // RV byte addr
                    mem_write_word(0xE0010 >> 2, cpu.regs[15]); // SP
                    mem_write_word(0xE0014 >> 2, rv_word); // RV word index
                    cpu.halted = 1;
                    increment_stat(STAT_ROM_FAULT);
                    break;
                }
            };
            // Restart trap: catch jump to PC 0 (_start) — indicates restart bug
            if cpu.pc == 0 {
                mem_write_word(0xE0000 >> 2, 0xDEAD0001); // sentinel
                mem_write_word(0xE0004 >> 2, opc as u32); // 0x29 = JMPR
                mem_write_word(0xE0008 >> 2, old_pc); // MBC PC before jump
                mem_write_word(0xE000C >> 2, rv_addr); // RV addr that mapped to 0
                mem_write_word(0xE0010 >> 2, cpu.regs[15]); // SP
                cpu.halted = 1;
                increment_stat(STAT_ROM_FAULT);
                break;
            }
        } else if opc == op::CALLR {
            // Indirect call with RV32I→MBC address translation.
            // SP is byte address; each stack entry is 4 bytes.
            let old_pc = cpu.pc.wrapping_sub(1); // PC of this CALLR instruction
            cpu.regs[15] = cpu.regs[15].wrapping_sub(4);
            let word_addr = cpu.regs[15] >> 2;
            mem_write_word(word_addr, cpu.pc);
            let rv_addr = cpu.regs[d];
            let rv_word = rv_addr >> 2;
            cpu.pc = match RV2MBC_MAP.get(rv_word) {
                Some(mbc_idx) => *mbc_idx,
                None => {
                    mem_write_word(0xE0000 >> 2, 0xDEAD0003); // sentinel (unmapped CALLR)
                    mem_write_word(0xE0004 >> 2, opc as u32); // 0x2A = CALLR
                    mem_write_word(0xE0008 >> 2, old_pc); // MBC PC of CALLR insn
                    mem_write_word(0xE000C >> 2, rv_addr); // RV byte addr
                    mem_write_word(0xE0010 >> 2, cpu.regs[15]); // SP
                    mem_write_word(0xE0014 >> 2, rv_word); // RV word index
                    cpu.halted = 1;
                    increment_stat(STAT_ROM_FAULT);
                    break;
                }
            };
            // Restart trap: catch indirect call to PC 0
            if cpu.pc == 0 {
                mem_write_word(0xE0000 >> 2, 0xDEAD0001); // sentinel
                mem_write_word(0xE0004 >> 2, opc as u32); // 0x2A = CALLR
                mem_write_word(0xE0008 >> 2, old_pc); // MBC PC before call
                mem_write_word(0xE000C >> 2, rv_addr); // RV addr that mapped to 0
                mem_write_word(0xE0010 >> 2, cpu.regs[15]); // SP
                cpu.halted = 1;
                increment_stat(STAT_ROM_FAULT);
                break;
            }
        } else if opc == op::RET {
            // SP is byte address; each stack entry is 4 bytes.
            let word_addr = cpu.regs[15] >> 2;
            let ret = mem_read_word(word_addr);
            cpu.regs[15] = cpu.regs[15].wrapping_add(4);
            // Restart trap: catch RET popping 0 (stack corruption)
            if ret == 0 {
                mem_write_word(0xE0000 >> 2, 0xDEAD0001); // sentinel
                mem_write_word(0xE0004 >> 2, 0x28); // RET opcode
                mem_write_word(0xE0008 >> 2, cpu.pc.wrapping_sub(1)); // PC of RET insn
                mem_write_word(0xE000C >> 2, ret); // popped value (0)
                mem_write_word(0xE0010 >> 2, cpu.regs[15]); // SP after pop
                mem_write_word(0xE0014 >> 2, word_addr); // stack word addr that was read
                cpu.halted = 1;
                increment_stat(STAT_ROM_FAULT);
                break;
            }
            cpu.pc = ret;

        // ── Memory ────────────────────────────────────────────────────────────
        // All memory accesses go directly to RAM_MAP (BPF Array, O(1)).
        // L1 cache is disabled — Array is faster than LruHashMap.
        } else if opc == op::LD {
            // dst = RAM[src + simm16]  (32-bit word load)
            let addr = cpu.regs[s].wrapping_add(simm as u32);
            let word_addr = addr >> 2;
            let val = match RAM_MAP.get(word_addr) {
                Some(v) => *v,
                None => 0,
            };
            cpu.regs[d] = val;
            cpu.cache_misses += 1; // mem_loads counter (legacy field name)
            increment_stat(STAT_MEM_LOADS);
        } else if opc == op::ST {
            // RAM[dst + simm16] = src  (32-bit word store)
            let addr = cpu.regs[d].wrapping_add(simm as u32);
            let val = cpu.regs[s];
            mem_write_word(addr >> 2, val);
            cpu.cache_hits += 1; // mem_stores counter (legacy field name)
            increment_stat(STAT_MEM_STORES);
        } else if opc == op::LDB {
            // dst = zero_extend(RAM[src + simm16])  (byte load)
            let addr = cpu.regs[s].wrapping_add(simm as u32);
            let word_addr = addr >> 2;
            let byte_shift = (addr & 3) * 8;
            let word = match RAM_MAP.get(word_addr) {
                Some(v) => *v,
                None => 0,
            };
            cpu.regs[d] = (word >> byte_shift) & 0xFF;
            cpu.cache_misses += 1;
            increment_stat(STAT_MEM_LOADS);
        } else if opc == op::STB {
            // RAM[dst + simm16] = src & 0xFF  (byte store)
            let addr = cpu.regs[d].wrapping_add(simm as u32);
            let val = (cpu.regs[s] & 0xFF) as u8;
            mem_write_byte(addr, val);
            cpu.cache_hits += 1;
            increment_stat(STAT_MEM_STORES);
        } else if opc == op::LDH {
            // dst = zero_extend(RAM[src + simm16])  (16-bit halfword load)
            let addr = cpu.regs[s].wrapping_add(simm as u32);
            let word_addr = addr >> 2;
            let half_shift = (addr & 2) * 8;
            let word = match RAM_MAP.get(word_addr) {
                Some(v) => *v,
                None => 0,
            };
            cpu.regs[d] = (word >> half_shift) & 0xFFFF;
            cpu.cache_misses += 1;
            increment_stat(STAT_MEM_LOADS);
        } else if opc == op::STH {
            // RAM[dst + simm16] = src & 0xFFFF  (16-bit halfword store)
            let addr = cpu.regs[d].wrapping_add(simm as u32);
            let val = (cpu.regs[s] & 0xFFFF) as u16;
            mem_write_half(addr, val);
            cpu.cache_hits += 1;
            increment_stat(STAT_MEM_STORES);

        // ── System ────────────────────────────────────────────────────────────
        } else if opc == op::SYSCALL {
            increment_stat(STAT_SYSCALLS);
            // RV32I ecall convention: a7 (x17 → r1 via spill) = syscall number,
            // a0 (x10 → r8) = first arg / return value,
            // a1 (x11 → r9) = second arg / return value.
            let syscall_nr = cpu.regs[1];
            if syscall_nr == sys::SYS_DRAW_FRAME {
                // DG_DrawFrame: framebuffer at SCREEN_BASE.
                emit_screen_write(flow_label, mmap::SCREEN_BASE, hop_id);
                copy_fb_to_screen(mmap::SCREEN_BASE);
            } else if syscall_nr == sys::SYS_GET_KEY {
                // DG_GetKey: r8 (a0) = key code, r9 (a1) = pressed.
                // Scan 8 KBD_MAP slots (circular queue) for the first non-zero event.
                // Encoding per slot: (key_code << 1) | pressed_flag
                // Clear consumed slot so the same event isn't returned twice.
                let mut found = false;
                let mut slot: u32 = 0;
                while slot < 8 {
                    let kv = match KBD_MAP.get(slot) {
                        Some(v) => *v,
                        None => 0,
                    };
                    if kv != 0 {
                        cpu.regs[8] = (kv >> 1) & 0x7FFF_FFFF; // key code
                        cpu.regs[9] = kv & 1; // pressed flag
                                              // Consume: clear slot
                        if let Some(p) = KBD_MAP.get_ptr_mut(slot) {
                            unsafe {
                                *p = 0;
                            }
                        }
                        found = true;
                        break;
                    }
                    slot += 1;
                }
                if !found {
                    cpu.regs[8] = 0;
                    cpu.regs[9] = 0;
                }
            } else if syscall_nr == sys::SYS_GET_TICKS {
                // DG_GetTicksMs: r8 (a0) = milliseconds since boot.
                cpu.regs[8] = (now / 1_000_000) as u32;
            } else if syscall_nr == sys::SYS_SLEEP {
                // DG_SleepMs: sleep for r8 (a0) milliseconds.
                let ms = cpu.regs[8] as u64;
                cpu.sleep_until_ns = now + ms * 1_000_000;
                // Break the execute loop — we're asleep.
                increment_stat(STAT_SLEEPING);
                break;
            }
            // Unknown syscall: silently ignore (fail-safe).
        } else if opc == op::HALT {
            cpu.halted = 1;
            increment_stat(STAT_HALTED);
            emit_compute_halt(flow_label, cpu.insn_count, hop_id);
            break;
        }
        // Unknown opcode: treat as NOP (fail-open for forward compat).

        i += 1;
        cpu.insn_count += 1;
        increment_stat(STAT_INSNS_EXECUTED);
    }

    // Turbo mode: bounce packet on same interface (XDP_TX) for cache-warm execution.
    // Manual hop counter since XDP_TX bypasses kernel IP stack (no hop_limit decrement).
    let hop_count_ptr = (monad_start + 3) as *mut u8;
    let current_hop = unsafe { core::ptr::read_volatile(hop_count_ptr) };
    if current_hop == 255 {
        return Ok(xdp_action::XDP_DROP); // exhausted — drop, inject fresh packet
    }
    unsafe { core::ptr::write_volatile(hop_count_ptr, current_hop.wrapping_add(1)) };

    Ok(xdp_action::XDP_TX) // turbo mode: packet bounces on same interface (cache-warm)
}

// ── L1 Cache helpers ──────────────────────────────────────────────────────────

/// Load a u32 from L1 cache. Returns Ok(val) on hit, Err(addr) on miss.
/// Cache DISABLED: concurrent XDP hops cause non-atomic read-modify-write
/// on cache lines, leading to silent data corruption. All loads now go
/// directly through RAM_MAP.
#[inline(always)]
// L1 cache functions removed — RAM_MAP (BPF Array) provides O(1) direct
// indexing which is faster than LruHashMap.  Memory operations now go
// directly to RAM_MAP in the execute loop above.

// ── Event emission helpers ─────────────────────────────────────────────────────

/// Emit a CACHE_MISS event to the ring buffer.
#[allow(dead_code)]
fn emit_cache_miss(flow_label: u32, miss_addr: u32, hop_id: u8) {
    if let Some(mut entry) = COMPUTE_EVENTS.reserve::<ComputeHopEvent>(0) {
        let event = ComputeHopEvent {
            timestamp_ns: unsafe { bpf_ktime_get_ns() },
            event_type: EVENT_CACHE_MISS,
            hop_id,
            _pad: [0; 2],
            flow_label,
            pc: 0,
            instruction: 0,
            regs: [0; 16],
            flags: 0,
            cache_hit: 0,
            miss_addr,
        };
        unsafe {
            core::ptr::write(entry.as_mut_ptr(), event);
            entry.submit(0);
        }
    }
}

/// Emit a SCREEN_WRITE event to the ring buffer.
#[inline(always)]
fn emit_screen_write(flow_label: u32, fb_addr: u32, hop_id: u8) {
    if let Some(mut entry) = COMPUTE_EVENTS.reserve::<ComputeHopEvent>(0) {
        let event = ComputeHopEvent {
            timestamp_ns: unsafe { bpf_ktime_get_ns() },
            event_type: EVENT_SCREEN_WRITE,
            hop_id,
            _pad: [0; 2],
            flow_label,
            pc: fb_addr, // Reuse pc field for framebuffer address
            instruction: 0,
            regs: [0; 16],
            flags: 0,
            cache_hit: 0,
            miss_addr: 0,
        };
        unsafe {
            core::ptr::write(entry.as_mut_ptr(), event);
            entry.submit(0);
        }
    }
}

/// Emit a COMPUTE_HALT event to the ring buffer.
#[inline(always)]
fn emit_compute_halt(flow_label: u32, insn_count: u64, hop_id: u8) {
    if let Some(mut entry) = COMPUTE_EVENTS.reserve::<ComputeHopEvent>(0) {
        let event = ComputeHopEvent {
            timestamp_ns: unsafe { bpf_ktime_get_ns() },
            event_type: EVENT_COMPUTE_HALT,
            hop_id,
            _pad: [0; 2],
            flow_label,
            pc: 0,
            instruction: (insn_count & 0xFFFF_FFFF) as u32,
            regs: [0; 16],
            flags: 0,
            cache_hit: 0,
            miss_addr: 0,
        };
        unsafe {
            core::ptr::write(entry.as_mut_ptr(), event);
            entry.submit(0);
        }
    }
}

// ── MBC CPU flag helper ───────────────────────────────────────────────────────

/// Update Z, N, C flags from an ALU result.
#[inline(always)]
fn set_flags(cpu: &mut MbcCpuState, result: u32, carry: bool) {
    cpu.flags = 0;
    if result == 0 {
        cpu.flags |= mf::Z;
    }
    if result & 0x8000_0000 != 0 {
        cpu.flags |= mf::N;
    }
    if carry {
        cpu.flags |= mf::C;
    }
}

// ── Memory access helpers ─────────────────────────────────────────────────────
//
// All helpers are #[inline(always)] — they are folded into the execute loop.
// The BPF verifier sees the inlined code; there are no BPF subprograms here.

/// Read a 32-bit word from the MBC address space.
/// `word_addr` = byte_addr >> 2 for word-aligned operations.
#[inline(always)]
fn mem_read_word(word_addr: u32) -> u32 {
    // KBD register: word address of 0xFFFF/4 = 0x3FFF (nearest word).
    let kbd_word = mmap::KBD_ADDR >> 2;
    if word_addr == kbd_word {
        return match KBD_MAP.get(0) {
            Some(v) => *v,
            None => 0,
        };
    }
    match RAM_MAP.get(word_addr) {
        Some(v) => *v,
        None => 0,
    }
}

/// Write a 32-bit word to the MBC address space.
/// Handles screen framebuffer and general RAM.
#[inline(always)]
fn mem_write_word(word_addr: u32, value: u32) {
    // Screen region: word_addr in [SCREEN_BASE/4 .. (SCREEN_BASE+SCREEN_SIZE)/4)
    let screen_word_start = mmap::SCREEN_BASE >> 2;
    let screen_word_end = (mmap::SCREEN_BASE + mmap::SCREEN_SIZE + 3) >> 2;

    if word_addr >= screen_word_start && word_addr < screen_word_end {
        // Write four bytes to SCREEN_MAP.
        let pixel_base = ((word_addr - screen_word_start) * 4) as u32;
        let bytes = value.to_le_bytes();
        for k in 0..4u32 {
            let px = pixel_base + k;
            if px < mmap::SCREEN_SIZE {
                if let Some(p) = SCREEN_MAP.get_ptr_mut(px) {
                    unsafe {
                        *p = bytes[k as usize];
                    }
                }
            }
        }
    }

    // Always write to RAM_MAP (SCREEN_MAP is a projection of RAM_MAP).
    if let Some(ptr) = RAM_MAP.get_ptr_mut(word_addr) {
        unsafe {
            *ptr = value;
        }
    }
}

/// Read a single byte from the MBC address space.
#[allow(dead_code)]
#[inline(always)]
fn mem_read_byte(byte_addr: u32) -> u8 {
    // Screen region: direct SCREEN_MAP read.
    if byte_addr >= mmap::SCREEN_BASE && byte_addr < mmap::SCREEN_BASE + mmap::SCREEN_SIZE {
        let px = byte_addr - mmap::SCREEN_BASE;
        return match SCREEN_MAP.get(px) {
            Some(v) => *v,
            None => 0,
        };
    }
    // General RAM: extract byte from word.
    let word_addr = byte_addr >> 2;
    let byte_shift = (byte_addr & 3) * 8;
    let word = mem_read_word(word_addr);
    ((word >> byte_shift) & 0xFF) as u8
}

/// Write a single byte to the MBC address space.
#[inline(always)]
fn mem_write_byte(byte_addr: u32, value: u8) {
    // Screen region: direct SCREEN_MAP write.
    if byte_addr >= mmap::SCREEN_BASE && byte_addr < mmap::SCREEN_BASE + mmap::SCREEN_SIZE {
        let px = byte_addr - mmap::SCREEN_BASE;
        if let Some(p) = SCREEN_MAP.get_ptr_mut(px) {
            unsafe {
                *p = value;
            }
        }
        // Fall through to also write the enclosing word in RAM_MAP so reads are consistent.
    }
    let word_addr = byte_addr >> 2;
    let byte_shift = (byte_addr & 3) * 8;
    let old_word = mem_read_word(word_addr);
    let new_word = (old_word & !(0xFF << byte_shift)) | ((value as u32) << byte_shift);
    if let Some(ptr) = RAM_MAP.get_ptr_mut(word_addr) {
        unsafe {
            *ptr = new_word;
        }
    }
}

/// Read a 16-bit halfword from the MBC address space (little-endian).
#[allow(dead_code)]
#[inline(always)]
fn mem_read_half(byte_addr: u32) -> u16 {
    let word_addr = byte_addr >> 2;
    let half_shift = (byte_addr & 2) * 8; // 0 for low half, 16 for high half
    let word = mem_read_word(word_addr);
    ((word >> half_shift) & 0xFFFF) as u16
}

/// Write a 16-bit halfword to the MBC address space (little-endian).
#[inline(always)]
fn mem_write_half(byte_addr: u32, value: u16) {
    let word_addr = byte_addr >> 2;
    let half_shift = (byte_addr & 2) * 8;
    let old_word = mem_read_word(word_addr);
    let new_word = (old_word & !(0xFFFF << half_shift)) | ((value as u32) << half_shift);
    mem_write_word(word_addr, new_word);
}

// ── Framebuffer copy ──────────────────────────────────────────────────────────

/// Copy framebuffer from RAM to SCREEN_MAP.
///
/// STUB: The 16K-iteration loop (64 000 bytes) exceeded the BPF verifier's
/// 8192-jump complexity limit.  Framebuffer copy is deferred to userspace:
/// the emit_screen_write() event signals the dashboard to read SCREEN_MAP
/// directly.  mem_write_word() already projects screen-region writes into
/// SCREEN_MAP, so individual pixel writes during rendering are still visible.
///
/// When full Doom rendering is needed, this can be implemented via:
///   (a) BPF tail call to a dedicated copy program, or
///   (b) Userspace poller that bulk-copies RAM_MAP → SCREEN_MAP on event.
#[inline(always)]
fn copy_fb_to_screen(_fb_ptr: u32) {
    // No-op: verifier cannot handle 16K iterations.
    // Screen writes via mem_write_word/mem_write_byte still work individually.
}

// ── Monad XDP read ────────────────────────────────────────────────────────────

/// Read 20 Monad bytes from XDP packet memory.
#[inline(always)]
fn read_monad_xdp(start: usize, data_end: usize) -> Result<Monad, ()> {
    if start + MONAD_SIZE > data_end {
        return Err(());
    }
    let mut bytes = [0u8; 20];
    for i in 0..20usize {
        bytes[i] = unsafe { core::ptr::read_volatile((start + i) as *const u8) };
    }
    Ok(Monad::from_bytes(bytes))
}

// ── Stats helper ──────────────────────────────────────────────────────────────

#[inline(always)]
fn increment_stat(key: u32) {
    if let Some(v) = STATS.get_ptr_mut(&key) {
        unsafe {
            *v = (*v).saturating_add(1);
        }
    } else {
        let _ = STATS.insert(&key, &1u64, 0);
    }
}

// ── Panic handler ─────────────────────────────────────────────────────────────

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
