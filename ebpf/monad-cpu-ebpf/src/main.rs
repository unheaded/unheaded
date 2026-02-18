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
    helpers::{bpf_ktime_get_ns, bpf_get_prandom_u32},
    macros::{map, xdp},
    maps::{Array, HashMap},
    programs::XdpContext,
};
use monad_common::{
    Monad,
    MbcCpuState, MbcInsn,
    mbc_opcodes as op, mbc_flags as mf, mbc_syscalls as sys, mbc_mmap as mmap,
    IPV6_FIXED_HDR_LEN, IPV6_NEXTHDR_HBH, MONAD_OPT_TYPE, MONAD_OPT_DATA_LEN, MONAD_SIZE,
    flags, MBC_REG_COUNT,
};

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// ROM: MBC program instructions, u32 per slot.
/// Index = PC value.  Loaded by Wotan trace-collector from .mbc binary.
/// 65 536 entries = 256 KiB of instructions.
#[map]
static ROM_MAP: Array<u32> = Array::with_max_entries(65_536, 0);

/// RAM: sparse word-addressable memory.
/// Key = word address (byte_addr >> 2).  Value = 32-bit word.
/// Covers [0x0000_0000 .. 0x0041_0000) word-addressed (1M word entries).
/// Zero-initialized: missing key → value 0.
#[map]
static RAM_MAP: HashMap<u32, u32> = HashMap::with_max_entries(262_144, 0);

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

// ── Stat keys ─────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL:   u32 = 0;
const STAT_CPU_TICKS:       u32 = 1;
const STAT_INSNS_EXECUTED:  u32 = 2;
const STAT_HALTED:          u32 = 3;
const STAT_SLEEPING:        u32 = 4;
const STAT_NO_STATE:        u32 = 5;
const STAT_MEM_FAULTS:      u32 = 6;
const STAT_SYSCALLS:        u32 = 7;
const STAT_ROM_FAULT:       u32 = 8;

// ── Tuning constants ──────────────────────────────────────────────────────────

/// Maximum MBC instructions to execute per tick packet.
/// At 35 Hz, this is the sustained instruction throughput per instance.
/// 512 instructions/tick × 35 ticks/s = ~17 920 instructions/s.
const MAX_INSN_PER_TICK: usize = 512;

// ── Wire-format helpers ───────────────────────────────────────────────────────

#[repr(C, packed)]
struct EthHdr {
    _dst:   [u8; 6],
    _src:   [u8; 6],
    proto:  u16,
}

#[repr(C, packed)]
struct Ipv6Hdr {
    vtf:         u32,
    payload_len: u16,
    next_header: u8,
    hop_limit:   u8,
    _src:        [u8; 16],
    _dst:        [u8; 16],
}

const ETH_HLEN:   usize = 14;
const ETH_P_IPV6: u16   = 0x86DD;

// ── XDP entry point ───────────────────────────────────────────────────────────

#[xdp]
pub fn monad_cpu(ctx: XdpContext) -> u32 {
    match try_monad_cpu(&ctx) {
        Ok(action) => action,
        Err(_)     => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_monad_cpu(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_PACKETS_TOTAL);

    let data     = ctx.data();
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

    let opt_type: u8     = unsafe { core::ptr::read_volatile((hbh_start + 2) as *const u8) };
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
    let vtf        = u32::from_be(unsafe { core::ptr::read_volatile(&ip.vtf) });
    let flow_label = vtf & 0x000F_FFFF;
    let instance   = flow_label & 0xFF; // low 8 bits → 256 possible instances

    // ── Load CPU state ────────────────────────────────────────────────────────
    let cpu_ptr = match CPU_MAP.get_ptr_mut(&instance) {
        Some(p) => p,
        None    => {
            increment_stat(STAT_NO_STATE);
            return Ok(xdp_action::XDP_DROP); // tick consumed, no instance
        }
    };
    let cpu = unsafe { &mut *cpu_ptr };

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
            None    => {
                // PC out of bounds — halt the CPU.
                cpu.halted = 1;
                increment_stat(STAT_ROM_FAULT);
                break;
            }
        };
        let insn = MbcInsn(insn_word);
        let opc  = insn.opcode();
        let d    = (insn.dst() as usize) & 0x0F;
        let s    = (insn.src() as usize) & 0x0F;
        let imm  = insn.imm16() as u32;
        let simm = insn.imm16_signed() as i32;

        // Advance PC before execution (branches will overwrite if taken).
        cpu.pc = cpu.pc.wrapping_add(1);

        // ── Arithmetic ────────────────────────────────────────────────────────
        if opc == op::ADD {
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

        // ── Register moves ────────────────────────────────────────────────────
        } else if opc == op::MOV {
            cpu.regs[d] = cpu.regs[s];
        } else if opc == op::MOVI {
            cpu.regs[d] = imm;

        // ── Compare ───────────────────────────────────────────────────────────
        } else if opc == op::CMP {
            let rd  = cpu.regs[d];
            let rs  = cpu.regs[s];
            let (diff, borrow) = rd.overflowing_sub(rs);
            // Z: equal, N: signed less-than (MSB of diff set), C: unsigned borrow
            cpu.flags = 0;
            if diff == 0 { cpu.flags |= mf::Z; }
            if diff & 0x8000_0000 != 0 { cpu.flags |= mf::N; }
            if borrow { cpu.flags |= mf::C; }

        // ── Branches ─────────────────────────────────────────────────────────
        } else if opc == op::JMP {
            // Unconditional relative branch.  PC already incremented; simm
            // is relative to the instruction after the branch.
            cpu.pc = cpu.pc.wrapping_add(simm as u32);
        } else if opc == op::JZ {
            if cpu.flags & mf::Z != 0 {
                cpu.pc = cpu.pc.wrapping_add(simm as u32);
            }
        } else if opc == op::JNZ {
            if cpu.flags & mf::Z == 0 {
                cpu.pc = cpu.pc.wrapping_add(simm as u32);
            }
        } else if opc == op::JN {
            if cpu.flags & mf::N != 0 {
                cpu.pc = cpu.pc.wrapping_add(simm as u32);
            }
        } else if opc == op::JP {
            if cpu.flags & mf::N == 0 {
                cpu.pc = cpu.pc.wrapping_add(simm as u32);
            }
        } else if opc == op::JC {
            if cpu.flags & mf::C != 0 {
                cpu.pc = cpu.pc.wrapping_add(simm as u32);
            }
        } else if opc == op::JNC {
            if cpu.flags & mf::C == 0 {
                cpu.pc = cpu.pc.wrapping_add(simm as u32);
            }
        } else if opc == op::CALL {
            // Push PC (already incremented = return address) to stack.
            // r15 = SP, full-descending stack: SP-- then [SP] = return addr.
            cpu.regs[15] = cpu.regs[15].wrapping_sub(1);
            let sp = cpu.regs[15];
            mem_write_word(sp, cpu.pc);
            // Jump to absolute address in imm16.
            cpu.pc = imm;
        } else if opc == op::RET {
            let sp  = cpu.regs[15];
            let ret = mem_read_word(sp);
            cpu.regs[15] = sp.wrapping_add(1);
            cpu.pc = ret;

        // ── Memory ────────────────────────────────────────────────────────────
        } else if opc == op::LD {
            // dst = ram_map[src + imm16]  (32-bit load)
            let ea = cpu.regs[s].wrapping_add(imm);
            cpu.regs[d] = mem_read_word(ea >> 2);
        } else if opc == op::ST {
            // ram_map[dst + imm16] = src  (32-bit store)
            let ea = cpu.regs[d].wrapping_add(imm);
            mem_write_word(ea >> 2, cpu.regs[s]);
        } else if opc == op::LDB {
            // dst = zero_extend(ram_map[src + imm16])  (byte load)
            let ea    = cpu.regs[s].wrapping_add(imm);
            let byte  = mem_read_byte(ea);
            cpu.regs[d] = byte as u32;
        } else if opc == op::STB {
            // ram_map[dst + imm16] = src & 0xFF  (byte store)
            let ea = cpu.regs[d].wrapping_add(imm);
            mem_write_byte(ea, (cpu.regs[s] & 0xFF) as u8);
        } else if opc == op::LDH {
            // dst = zero_extend(halfword at [src + imm16])
            let ea = cpu.regs[s].wrapping_add(imm);
            let hw = mem_read_half(ea);
            cpu.regs[d] = hw as u32;
        } else if opc == op::STH {
            // [dst + imm16] = src & 0xFFFF
            let ea = cpu.regs[d].wrapping_add(imm);
            mem_write_half(ea, (cpu.regs[s] & 0xFFFF) as u16);

        // ── System ────────────────────────────────────────────────────────────
        } else if opc == op::SYSCALL {
            increment_stat(STAT_SYSCALLS);
            let syscall_nr = cpu.regs[0];
            if syscall_nr == sys::SYS_DRAW_FRAME {
                // DG_DrawFrame: r1 = pixel buffer pointer in RAM.
                // We copy 64000 bytes from RAM (starting at r1) to SCREEN_MAP.
                // Bounded loop: exactly 64000 byte writes.
                let fb_ptr = cpu.regs[1];
                copy_fb_to_screen(fb_ptr);
            } else if syscall_nr == sys::SYS_GET_KEY {
                // DG_GetKey: r0 = scancode, r1 = pressed.
                if let Some(kv) = KBD_MAP.get(0) {
                    cpu.regs[0] = (*kv >> 1) & 0x7FFF_FFFF; // scancode
                    cpu.regs[1] = *kv & 1;                   // pressed flag
                } else {
                    cpu.regs[0] = 0;
                    cpu.regs[1] = 0;
                }
            } else if syscall_nr == sys::SYS_GET_TICKS {
                // DG_GetTicksMs: r0 = milliseconds since boot.
                cpu.regs[0] = (now / 1_000_000) as u32;
            } else if syscall_nr == sys::SYS_SLEEP {
                // DG_SleepMs: sleep for r1 milliseconds.
                let ms = cpu.regs[1] as u64;
                cpu.sleep_until_ns = now + ms * 1_000_000;
                // Break the execute loop — we're asleep.
                increment_stat(STAT_SLEEPING);
                break;
            }
            // Unknown syscall: silently ignore (fail-safe).

        } else if opc == op::HALT {
            cpu.halted = 1;
            increment_stat(STAT_HALTED);
            break;
        }
        // Unknown opcode: treat as NOP (fail-open for forward compat).

        i += 1;
        increment_stat(STAT_INSNS_EXECUTED);
    }

    Ok(xdp_action::XDP_DROP) // tick packet consumed — do not forward
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
            None    => 0,
        };
    }
    match unsafe { RAM_MAP.get(&word_addr) } {
        Some(v) => *v,
        None    => 0,
    }
}

/// Write a 32-bit word to the MBC address space.
/// Handles screen framebuffer and general RAM.
#[inline(always)]
fn mem_write_word(word_addr: u32, value: u32) {
    // Screen region: word_addr in [SCREEN_BASE/4 .. (SCREEN_BASE+SCREEN_SIZE)/4)
    let screen_word_start = mmap::SCREEN_BASE >> 2;
    let screen_word_end   = (mmap::SCREEN_BASE + mmap::SCREEN_SIZE + 3) >> 2;

    if word_addr >= screen_word_start && word_addr < screen_word_end {
        // Write four bytes to SCREEN_MAP.
        let pixel_base = ((word_addr - screen_word_start) * 4) as u32;
        let bytes = value.to_le_bytes();
        for k in 0..4u32 {
            let px = pixel_base + k;
            if px < mmap::SCREEN_SIZE {
                if let Some(p) = SCREEN_MAP.get_ptr_mut(px) {
                    unsafe { *p = bytes[k as usize]; }
                }
            }
        }
    }

    // Always write to RAM_MAP (SCREEN_MAP is a projection of RAM_MAP).
    let _ = unsafe { RAM_MAP.insert(&word_addr, &value, 0) };
}

/// Read a single byte from the MBC address space.
#[inline(always)]
fn mem_read_byte(byte_addr: u32) -> u8 {
    // Screen region: direct SCREEN_MAP read.
    if byte_addr >= mmap::SCREEN_BASE && byte_addr < mmap::SCREEN_BASE + mmap::SCREEN_SIZE {
        let px = byte_addr - mmap::SCREEN_BASE;
        return match SCREEN_MAP.get(px) {
            Some(v) => *v,
            None    => 0,
        };
    }
    // General RAM: extract byte from word.
    let word_addr  = byte_addr >> 2;
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
            unsafe { *p = value; }
        }
        // Fall through to also write the enclosing word in RAM_MAP so reads are consistent.
    }
    let word_addr  = byte_addr >> 2;
    let byte_shift = (byte_addr & 3) * 8;
    let old_word = mem_read_word(word_addr);
    let new_word = (old_word & !(0xFF << byte_shift)) | ((value as u32) << byte_shift);
    let _ = unsafe { RAM_MAP.insert(&word_addr, &new_word, 0) };
}

/// Read a 16-bit halfword from the MBC address space (little-endian).
#[inline(always)]
fn mem_read_half(byte_addr: u32) -> u16 {
    let word_addr  = byte_addr >> 2;
    let half_shift = (byte_addr & 2) * 8; // 0 for low half, 16 for high half
    let word = mem_read_word(word_addr);
    ((word >> half_shift) & 0xFFFF) as u16
}

/// Write a 16-bit halfword to the MBC address space (little-endian).
#[inline(always)]
fn mem_write_half(byte_addr: u32, value: u16) {
    let word_addr  = byte_addr >> 2;
    let half_shift = (byte_addr & 2) * 8;
    let old_word = mem_read_word(word_addr);
    let new_word = (old_word & !(0xFFFF << half_shift)) | ((value as u32) << half_shift);
    mem_write_word(word_addr, new_word);
}

// ── Framebuffer copy ──────────────────────────────────────────────────────────

/// Copy 64 000 bytes from RAM (starting at byte address `fb_ptr`) to SCREEN_MAP.
///
/// This implements `DG_DrawFrame`: doomgeneric calls it with a pointer to the
/// current frame in RAM.  We iterate over the 64 000 pixels and copy each byte.
///
/// Bounded loop: exactly 64 000 iterations.  Safe for the BPF verifier.
#[inline(always)]
fn copy_fb_to_screen(fb_ptr: u32) {
    // Copy 16 000 words (64 000 bytes).
    for w in 0u32..16_000u32 {
        let src_addr = fb_ptr.wrapping_add(w * 4);
        let word_addr = src_addr >> 2;
        let word = match unsafe { RAM_MAP.get(&word_addr) } {
            Some(v) => *v,
            None    => 0,
        };
        // Write 4 pixels.
        let px_base = w * 4;
        let bytes = word.to_le_bytes();
        for k in 0u32..4u32 {
            let px = px_base + k;
            if px < mmap::SCREEN_SIZE {
                if let Some(p) = SCREEN_MAP.get_ptr_mut(px) {
                    unsafe { *p = bytes[k as usize]; }
                }
            }
        }
    }
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
        unsafe { *v = (*v).saturating_add(1); }
    } else {
        let _ = STATS.insert(&key, &1u64, 0);
    }
}

// ── Panic handler ─────────────────────────────────────────────────────────────

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
