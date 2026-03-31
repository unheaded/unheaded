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
//! | Address range                | Backing map     | Description              |
//! |------------------------------|-----------------|--------------------------|
//! | 0x0000_0000 – 0x0006_3330   | `RAM_MAP`       | .rodata + .data + .bss   |
//! | 0x0006_8000                  | `KBD_MAP[0]`    | Keyboard state word      |
//! | 0x0007_0000 – 0x0007_F9FF   | `SCREEN_MAP`    | 320×200 framebuffer      |
//! | 0x0011_0000 – 0x0051_0000   | `RAM_MAP`       | WAD data (4 MiB)         |
//! | 0x0052_0000 – 0x0152_0000   | `RAM_MAP`       | Heap (16 MiB)            |
//! | 0x03F0_0000                  | `RAM_MAP`       | Stack top (growing down) |
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
    maps::{Array, HashMap, LruHashMap, ProgramArray, RingBuf},
    programs::XdpContext,
};
use monad_common::{
    flags, mbc_block as blk, mbc_flags as mf, mbc_interrupts as intr,
    mbc_linux_syscalls as lsys, mbc_mmap as mmap, mbc_mmu as mmu,
    mbc_opcodes as op, mbc_syscalls as sys,
    ComputeHopEvent, MbcCpuState, MbcInsn, Monad, DEFAULT_PROGRAM_BREAK,
    EVENT_CACHE_MISS, EVENT_COMPUTE_HALT,
    EVENT_CONTEXT_SWITCH, EVENT_SCREEN_WRITE, EVENT_TTY_WRITE, IPV6_FIXED_HDR_LEN,
    IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN, MONAD_OPT_TYPE, MONAD_SIZE,
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
/// Userspace polls this at 60 Hz to render the current frame.
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

/// Tail call program array: self-referencing for multi-round execution.
/// Userspace (doom-runner) populates index 0 with monad_cpu's own FD.
/// Each tail call gives a fresh BPF verifier budget (16 insns per round).
/// At N=16 rounds: 16 × 16 = 256 MBC instructions per packet.
#[map]
static TAIL_CALL_PROGS: ProgramArray = ProgramArray::with_max_entries(1, 0);

/// Tail call round counter: tracks how many rounds have executed this packet.
/// Single entry at index 0.  Reset to 0 after the last round completes.
/// Max tail call depth in kernel 6.x is 33; we default to 15 additional
/// rounds (16 total including the initial invocation).
#[map]
static TAIL_ROUND: Array<u32> = Array::with_max_entries(1, 0);

/// Maximum additional tail call rounds per packet (0 = no tail calls).
/// Total rounds = 1 (initial) + MAX_TAIL_CALLS.
/// At 15: 16 total rounds × 16 insns = 256 insns/packet.
/// Kernel limit is 33 tail calls, so max value is 32.
const MAX_TAIL_CALLS: u32 = 15;

/// TTY output buffer: 4 KB circular buffer for console I/O (Level 4f).
/// SYS_WRITE to fd 1 (stdout) or fd 2 (stderr) writes bytes here.
/// Userspace (doom-bridge) polls TTY_HEAD to detect new output.
#[map]
static TTY_MAP: Array<u8> = Array::with_max_entries(4096, 0);

/// TTY write head position (circular, wraps at 4096).
/// Single entry: index 0 = current write position.
#[map]
static TTY_HEAD: Array<u32> = Array::with_max_entries(1, 0);

/// Process table: 4 slots, each stores saved CPU state for context switch (Level 4c).
/// Key = process_id (0-3), Value = saved register set (20 u32s = 80 bytes).
/// Layout per slot: [r0..r15, PC, flags, SP_copy, program_break]
#[map]
static PROC_TABLE: Array<[u32; 20]> = Array::with_max_entries(4, 0);

/// Scheduler state: [0]=current_pid, [1]=num_processes, [2]=scheduler_enabled, [3]=halted_mask.
/// halted_mask: bit i set means process i has exited/halted.
#[map]
static SCHED_STATE: Array<u32> = Array::with_max_entries(4, 0);

/// Software TLB: 64 entries, direct-mapped by virtual page number (Level 4d).
/// Key = index (vpn & 63), Value = [vpn, pfn, flags] (3 u32s = 12 bytes).
/// Used by translate_address() for fast virtual-to-physical translation.
#[map]
static TLB_MAP: Array<[u32; 3]> = Array::with_max_entries(64, 0);

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
const STAT_FRAME_READY: u32 = 11; // Incremented by SYS_DRAW_FRAME; userspace polls to trigger bulk FB copy
const STAT_TIMER_INTERRUPTS: u32 = 12; // Timer interrupt deliveries
const STAT_LINUX_SYSCALLS: u32 = 13; // Linux-compatible INT 0x80 syscalls
const STAT_TTY_WRITES: u32 = 14; // TTY write operations (SYS_WRITE to fd 1/2)
const STAT_CONTEXT_SWITCHES: u32 = 15; // Scheduler context switches (Level 4c)
const STAT_FORKS: u32 = 16; // SYS_FORK process creation (Level 4c)
const STAT_BLOCK_OPS: u32 = 17; // Block device read/write operations (Level 4e)
const STAT_TLB_HITS: u32 = 18; // TLB hits (Level 4d MMU)
const STAT_TLB_MISSES: u32 = 19; // TLB misses — page table walk required (Level 4d MMU)
const STAT_TTY_READS: u32 = 20; // TTY read operations (SYS_READ from fd 0) (Level 5b)
const STAT_WAITPIDS: u32 = 21; // SYS_WAITPID calls (Level 5b)
const STAT_TRIVIAL_SYSCALLS: u32 = 22; // Trivial FUZIX stub syscalls (Level 5c)
const STAT_MODERATE_SYSCALLS: u32 = 23; // Moderate FUZIX syscalls (Level 5d: ioctl, signal, stat, pipe, etc.)

// ── Tuning constants ──────────────────────────────────────────────────────────

/// Maximum MBC instructions to execute per XDP invocation.
///
/// Each XDP invocation executes up to N MBC instructions.
/// BPF verifier limits: 8192 jump complexity, 1M processed insns.
/// Verifier state-explores each opcode branch per iteration.
/// Each iteration costs ~3900 verifier states (many opcode branches + map ops).
/// 256 → exceeds 1M verifier insn limit (opcode branches × iterations × inner loops).
/// 64 → ~250K verifier insns (safe margin for complex opcode handlers).
/// Compensate with higher tick rate from injector (2000+ Hz).
/// Computermancer: throughput = MAX_INSN_PER_TICK × tick_rate. At 2000 Hz:
/// 64 × 2000 = 128K insns/sec (sufficient for Doom at ~100K insns/frame × 35 fps).
const MAX_INSN_PER_TICK: usize = 16;

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
    let action = match try_monad_cpu(&ctx) {
        Ok(a) => a,
        Err(_) => return xdp_action::XDP_PASS,
    };

    // ── Tail call chain (MUST be in entry point, not subprog) ──────────────
    // Kernel 6.17: "tail_call not allowed in subprogs without BTF"
    if action == xdp_action::XDP_TX {
        if let Some(round_ptr) = unsafe { TAIL_ROUND.get_ptr_mut(0) } {
            let round = unsafe { *round_ptr };
            if round < MAX_TAIL_CALLS {
                unsafe { *round_ptr = round + 1 };
                unsafe { TAIL_CALL_PROGS.tail_call(&ctx, 0).ok() };
            }
            unsafe { *round_ptr = 0 };
        }
    }

    action
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

    // ── Timer interrupt trigger ─────────────────────────────────────────────
    // Increment tick counter each XDP invocation. Every TIMER_TICK_DIVISOR
    // ticks (~12 Hz at 35 Hz XDP rate), fire a timer interrupt — but only
    // if interrupts are enabled and no interrupt is already pending.
    cpu.tick_counter = cpu.tick_counter.wrapping_add(1);
    if cpu.tick_counter >= intr::TIMER_TICK_DIVISOR
        && cpu.interrupts_enabled != 0
        && cpu.interrupt_pending == 0
    {
        cpu.interrupt_pending = 1;
        cpu.interrupt_vector = intr::VECTOR_TIMER;
        cpu.tick_counter = 0;
        increment_stat(STAT_TIMER_INTERRUPTS);

        // ── Level 4c: Scheduler context switch on timer ──────────────────
        // When num_processes > 1, the timer interrupt triggers a round-robin
        // context switch. Save current process state, load next runnable.
        if cpu.num_processes > 1 {
            scheduler_context_switch(cpu, flow_label, hop_id);
        }
    }

    // ── Execute loop ──────────────────────────────────────────────────────────
    let mut prev_pc: u32 = cpu.pc;  // Track last valid PC for post-mortem
    let mut i = 0usize;
    while i < MAX_INSN_PER_TICK {
        if cpu.halted != 0 {
            break;
        }

        // ── Interrupt dispatch (before fetch) ────────────────────────────────
        // If an interrupt is pending and interrupts are enabled, push flags
        // and PC onto the stack, disable interrupts, and jump to the IVT handler.
        if cpu.interrupt_pending != 0 && cpu.interrupts_enabled != 0 {
            // Push flags to stack
            cpu.regs[15] = cpu.regs[15].wrapping_sub(4);
            mem_write_word(cpu.regs[15] >> 2, cpu.flags as u32);
            // Push PC to stack
            cpu.regs[15] = cpu.regs[15].wrapping_sub(4);
            mem_write_word(cpu.regs[15] >> 2, cpu.pc);
            // Disable interrupts
            cpu.interrupts_enabled = 0;
            // Jump to IVT handler
            let ivt_addr = (cpu.interrupt_vector as u32) * 4;
            cpu.pc = mem_read_word(ivt_addr >> 2);
            // Clear pending
            cpu.interrupt_pending = 0;
        }

        // Fetch — with explicit PC bounds check
        // BPF Array::get may not return None for large indices on all kernels.
        // Belt-and-suspenders: check explicitly, then check Array result.
        if cpu.pc >= 262_144_u32 {
            // PC out of ROM range — record bad PC AND last valid PC
            if let Some(ptr) = STATS.get_ptr_mut(&STAT_ROM_FAULT) {
                unsafe { *ptr = ((cpu.pc as u64) << 32) | (prev_pc as u64); }
            }
            cpu.halted = 1;
            break;
        }
        // SP monitor moved to HALT handler (verifier limit prevents per-insn checks).
        prev_pc = cpu.pc;
        let insn_word = match ROM_MAP.get(cpu.pc) {
            Some(w) => *w,
            None => {
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
        } else if opc == op::MULHU {
            let a = cpu.regs[d] as u64;
            let b = cpu.regs[s] as u64;
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
            // Link register semantics (RV32I ABI compatible):
            // Store return address (current PC, already incremented) in r14 (LR).
            // The compiled code's prologue handles saving r14 to the stack.
            // No stack manipulation here — that's the compiler's job.
            //
            // This matches RV32I `jal x1, target`: x1=PC+4, PC=target.
            // x1(ra) maps to MBC r14.
            cpu.regs[14] = cpu.pc; // LR = return address
            let target = insn_word & 0x00FF_FFFF;
            cpu.pc = target;
            if cpu.pc == 0 {
                let old_pc = cpu.pc.wrapping_sub(1);
                mem_write_word(0xE0000 >> 2, 0xDEAD0001);
                mem_write_word(0xE0004 >> 2, 0x27);
                mem_write_word(0xE0008 >> 2, old_pc);
                mem_write_word(0xE000C >> 2, target);
                mem_write_word(0xE0010 >> 2, cpu.regs[15]);
                increment_stat(STAT_ROM_FAULT);
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
                    // Bug 20 fix: Unmapped JMPR — skip instead of halting.
                    // BSS corruption in DOOM produces garbage function pointers.
                    // Skipping the indirect jump lets execution fall through to
                    // the next MBC instruction (the return point), as if the
                    // called function returned immediately. Write diagnostics.
                    mem_write_word(0xE0000 >> 2, 0xDEAD0002); // sentinel (unmapped)
                    mem_write_word(0xE0004 >> 2, opc as u32); // 0x29 = JMPR
                    mem_write_word(0xE0008 >> 2, old_pc); // MBC PC of JMPR insn
                    mem_write_word(0xE000C >> 2, rv_addr); // RV byte addr
                    mem_write_word(0xE0010 >> 2, cpu.regs[15]); // SP
                    mem_write_word(0xE0014 >> 2, rv_word); // RV word index
                    increment_stat(STAT_ROM_FAULT);
                    cpu.pc // keep PC at next instruction (skip the JMPR)
                }
            };
            // Restart: log but don't halt
            if cpu.pc == 0 {
                mem_write_word(0xE0000 >> 2, 0xDEAD0001);
                mem_write_word(0xE0004 >> 2, opc as u32);
                mem_write_word(0xE0008 >> 2, old_pc);
                mem_write_word(0xE000C >> 2, rv_addr);
                mem_write_word(0xE0010 >> 2, cpu.regs[15]);
                increment_stat(STAT_ROM_FAULT);
            }
        } else if opc == op::CALLR {
            // Indirect call with RV32I→MBC address translation.
            let old_pc = cpu.pc.wrapping_sub(1);
            cpu.regs[14] = cpu.pc; // LR = return address
            let rv_addr = cpu.regs[d];
            let rv_word = rv_addr >> 2;

            // DIAGNOSTIC: log every CALLR to debug region 0xE1000
            let callr_log_base = 0xE1000u32 >> 2;
            mem_write_word(callr_log_base, rv_addr);
            mem_write_word(callr_log_base + 1, rv_word);
            mem_write_word(callr_log_base + 2, d as u32);
            mem_write_word(callr_log_base + 3, old_pc);

            cpu.pc = match RV2MBC_MAP.get(rv_word) {
                Some(mbc_idx) => {
                    // Log successful lookup
                    mem_write_word(callr_log_base + 4, *mbc_idx);
                    mem_write_word(callr_log_base + 5, 0xCA110001); // success marker
                    *mbc_idx
                },
                None => {
                    // Unmapped CALLR — skip
                    mem_write_word(callr_log_base + 4, 0xDEAD0003);
                    mem_write_word(callr_log_base + 5, rv_word);
                    mem_write_word(0xE0000 >> 2, 0xDEAD0003);
                    mem_write_word(0xE0004 >> 2, opc as u32);
                    mem_write_word(0xE0008 >> 2, old_pc);
                    mem_write_word(0xE000C >> 2, rv_addr);
                    mem_write_word(0xE0010 >> 2, cpu.regs[15]);
                    mem_write_word(0xE0014 >> 2, rv_word);
                    increment_stat(STAT_ROM_FAULT);
                    cpu.pc // skip
                }
            };
            if cpu.pc == 0 {
                mem_write_word(0xE0000 >> 2, 0xDEAD0001);
                mem_write_word(0xE0004 >> 2, opc as u32);
                mem_write_word(0xE0008 >> 2, old_pc);
                mem_write_word(0xE000C >> 2, rv_addr);
                mem_write_word(0xE0010 >> 2, cpu.regs[15]);
                increment_stat(STAT_ROM_FAULT);
            }
        } else if opc == op::RET {
            // Link register return: jump to address in r14 (LR).
            // The compiled code's epilogue restores r14 from the stack
            // before executing RET, so r14 always holds the correct
            // return address.
            //
            // This matches RV32I `jalr x0, 0(x1)`: PC = x1(ra).
            // x1(ra) maps to MBC r14.
            let ret = cpu.regs[14];
            if ret == 0 {
                mem_write_word(0xE0000 >> 2, 0xDEAD0001);
                mem_write_word(0xE0004 >> 2, 0x28);
                mem_write_word(0xE0008 >> 2, cpu.pc.wrapping_sub(1));
                mem_write_word(0xE000C >> 2, ret);
                mem_write_word(0xE0010 >> 2, cpu.regs[15]);
                mem_write_word(0xE0014 >> 2, cpu.regs[14]);
                increment_stat(STAT_ROM_FAULT);
            }
            cpu.pc = ret;

        // ── Memory ────────────────────────────────────────────────────────────
        // All memory accesses go through translate_address() for MMU support
        // (Level 4d). When mmu_enabled == 0, translate_address returns the
        // address unchanged — ONE branch per access in Doom mode (acceptable).
        } else if opc == op::LD {
            // dst = RAM[src + simm16]  (32-bit word load)
            let raw_addr = cpu.regs[s].wrapping_add(simm as u32);
            let addr = translate_address(&cpu, raw_addr);
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
            let raw_addr = cpu.regs[d].wrapping_add(simm as u32);
            let addr = translate_address(&cpu, raw_addr);
            let val = cpu.regs[s];
            mem_write_word(addr >> 2, val);
            cpu.cache_hits += 1; // mem_stores counter (legacy field name)
            increment_stat(STAT_MEM_STORES);
        } else if opc == op::LDB {
            // dst = zero_extend(RAM[src + simm16])  (byte load)
            let raw_addr = cpu.regs[s].wrapping_add(simm as u32);
            let addr = translate_address(&cpu, raw_addr);
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
            let raw_addr = cpu.regs[d].wrapping_add(simm as u32);
            let addr = translate_address(&cpu, raw_addr);
            let val = (cpu.regs[s] & 0xFF) as u8;
            mem_write_byte(addr, val);
            cpu.cache_hits += 1;
            increment_stat(STAT_MEM_STORES);
        } else if opc == op::LDH {
            // dst = zero_extend(RAM[src + simm16])  (16-bit halfword load)
            let raw_addr = cpu.regs[s].wrapping_add(simm as u32);
            let addr = translate_address(&cpu, raw_addr);
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
            let raw_addr = cpu.regs[d].wrapping_add(simm as u32);
            let addr = translate_address(&cpu, raw_addr);
            let val = (cpu.regs[s] & 0xFFFF) as u16;
            mem_write_half(addr, val);
            cpu.cache_hits += 1;
            increment_stat(STAT_MEM_STORES);

        // ── Atomic operations (Level 6 — single-core safe via CLI/STI) ────────
        } else if opc == op::CLI {
            cpu.interrupts_enabled = 0;
        } else if opc == op::STI {
            cpu.interrupts_enabled = 1;
        } else if opc == op::XCHG {
            // Atomic exchange: tmp=dst, dst=RAM[src+imm], RAM[src+imm]=tmp
            let addr = cpu.regs[s].wrapping_add(simm as u32);
            let word_addr = addr >> 2;
            let old = mem_read_word(word_addr);
            mem_write_word(word_addr, cpu.regs[d]);
            cpu.regs[d] = old;
        } else if opc == op::CAS {
            // Compare-and-swap: if RAM[r1]==r0 then RAM[r1]=r2, set Z; r0=old
            let addr = cpu.regs[1];
            let expected = cpu.regs[0];
            let desired = cpu.regs[2];
            let word_addr = addr >> 2;
            let old = mem_read_word(word_addr);
            if old == expected {
                mem_write_word(word_addr, desired);
                cpu.flags |= mf::Z;
            } else {
                cpu.flags &= !mf::Z;
            }
            cpu.regs[0] = old;

        // ── Interrupts ────────────────────────────────────────────────────────
        } else if opc == op::INT {
            let vector = imm as u8;
            if vector == intr::VECTOR_SYSCALL {
                // ── INT 0x80: Linux-compatible syscall dispatch (Level 4b) ────
                // Convention: r0 = syscall number, r1-r3 = args.
                // PC is NOT pushed (this is a synchronous call, not an interrupt).
                increment_stat(STAT_LINUX_SYSCALLS);
                let syscall_nr = cpu.regs[0];
                if syscall_nr == lsys::SYS_EXIT {
                    // SYS_EXIT(1): halt CPU with exit code from r1.
                    cpu.exit_code = cpu.regs[1];
                    // Unsuspend any parent waiting via vfork
                    if let Some(p) = SCHED_STATE.get_ptr_mut(2) {
                        unsafe { *p = 0; } // clear all suspended bits
                    }
                    cpu.halted = 1;
                    increment_stat(STAT_HALTED);
                    break;
                } else if syscall_nr == lsys::SYS_WRITE {
                    // SYS_WRITE(4): r1=fd, r2=buf_addr, r3=len.
                    // If fd==1 (stdout) or fd==2 (stderr), write to TTY_MAP.
                    let fd = cpu.regs[1];
                    let buf_addr = cpu.regs[2];
                    let len = cpu.regs[3];
                    if fd == 1 || fd == 2 {
                        // Get current TTY head position
                        let head = match TTY_HEAD.get(0) {
                            Some(v) => *v,
                            None => 0,
                        };
                        let mut written: u32 = 0;
                        let mut h = head;
                        // Bounded loop: write up to 16 bytes per syscall (BPF verifier friendly)
                        let max_write = if len < 16 { len } else { 16 };
                        let mut b: u32 = 0;
                        while b < max_write {
                            let byte_val = mem_read_byte(buf_addr.wrapping_add(b));
                            let tty_idx = h % 4096;
                            if let Some(p) = TTY_MAP.get_ptr_mut(tty_idx) {
                                unsafe { *p = byte_val; }
                            }
                            h = h.wrapping_add(1);
                            written += 1;
                            b += 1;
                        }
                        // Update TTY head
                        if let Some(p) = TTY_HEAD.get_ptr_mut(0) {
                            unsafe { *p = h; }
                        }
                        increment_stat(STAT_TTY_WRITES);
                        // Emit ring buffer event so userspace can pick up output
                        emit_tty_write(flow_label, written, hop_id);
                        // Return bytes written in r0
                        cpu.regs[0] = written;
                    } else {
                        // Unsupported fd: return -9 (EBADF)
                        cpu.regs[0] = (-9i32) as u32;
                    }
                } else if syscall_nr == lsys::SYS_BRK {
                    // SYS_BRK(45): r1=new_brk. If 0, return current break.
                    let new_brk = cpu.regs[1];
                    if new_brk == 0 {
                        cpu.regs[0] = cpu.program_break;
                    } else {
                        cpu.program_break = new_brk;
                        cpu.regs[0] = new_brk;
                    }
                } else if syscall_nr == lsys::SYS_GETPID {
                    // SYS_GETPID(20): return instance_id in r0.
                    cpu.regs[0] = instance;
                } else if syscall_nr == lsys::SYS_CLOCK_GETTIME {
                    // SYS_CLOCK_GETTIME(265): write ktime_ns to RAM at r1.
                    // Writes a simplified timespec: {u32 seconds, u32 nanoseconds}
                    let tp_addr = cpu.regs[1];
                    let secs = (now / 1_000_000_000) as u32;
                    let nsecs = (now % 1_000_000_000) as u32;
                    mem_write_word(tp_addr >> 2, secs);
                    mem_write_word((tp_addr.wrapping_add(4)) >> 2, nsecs);
                    cpu.regs[0] = 0; // success
                } else if syscall_nr == lsys::SYS_NANOSLEEP {
                    // SYS_NANOSLEEP(162): read timespec from RAM at r1, set sleep.
                    let req_addr = cpu.regs[1];
                    let secs = mem_read_word(req_addr >> 2) as u64;
                    let nsecs = mem_read_word((req_addr.wrapping_add(4)) >> 2) as u64;
                    let sleep_ns = secs * 1_000_000_000 + nsecs;
                    cpu.sleep_until_ns = now + sleep_ns;
                    cpu.regs[0] = 0; // success
                    increment_stat(STAT_SLEEPING);
                    break;
                } else if syscall_nr == lsys::SYS_FORK {
                    // ── SYS_FORK(2): Create child process (Level 4c scheduler) ──
                    // Copy current registers to next PROC_TABLE slot.
                    // Parent gets child_pid in r0, child gets 0 in r0.
                    if cpu.num_processes >= intr::MAX_PROCESSES {
                        // No room — return -EAGAIN (11)
                        cpu.regs[0] = (-11i32) as u32;
                    } else {
                        let child_pid = cpu.num_processes as u32;
                        // Save child state: copy parent's regs + PC + flags + SP + brk
                        let mut child_state = [0u32; 20];
                        let mut r = 0u32;
                        while r < 16 {
                            child_state[r as usize] = cpu.regs[r as usize];
                            r += 1;
                        }
                        child_state[16] = cpu.pc; // child resumes at same PC
                        child_state[17] = cpu.flags as u32;
                        child_state[18] = cpu.regs[15]; // SP copy
                        child_state[19] = cpu.program_break;
                        // Child gets 0 in r0 (fork return value)
                        child_state[0] = 0;
                        // Write child state to PROC_TABLE
                        if let Some(p) = PROC_TABLE.get_ptr_mut(child_pid) {
                            unsafe { *p = child_state; }
                        }
                        cpu.num_processes += 1;
                        // Update SCHED_STATE[1] = num_processes
                        if let Some(p) = SCHED_STATE.get_ptr_mut(1) {
                            unsafe { *p = cpu.num_processes as u32; }
                        }
                        increment_stat(STAT_FORKS);
                        // Parent gets child_pid in r0
                        cpu.regs[0] = child_pid;
                    }
                } else if syscall_nr == lsys::SYS_VFORK {
                    // ── SYS_VFORK(190): Fork + suspend parent (Level 6) ──
                    // Like SYS_FORK but parent is suspended until child exits/execve.
                    if cpu.num_processes >= intr::MAX_PROCESSES {
                        cpu.regs[0] = (-11i32) as u32; // EAGAIN
                    } else {
                        let parent_pid = cpu.current_pid as u32;
                        let child_pid = cpu.num_processes as u32;
                        let mut child_state = [0u32; 20];
                        let mut r = 0u32;
                        while r < 16 {
                            child_state[r as usize] = cpu.regs[r as usize];
                            r += 1;
                        }
                        child_state[16] = cpu.pc;
                        child_state[17] = cpu.flags as u32;
                        child_state[18] = cpu.regs[15]; // SP copy
                        child_state[19] = cpu.program_break;
                        child_state[0] = 0; // child gets 0
                        if let Some(p) = PROC_TABLE.get_ptr_mut(child_pid) {
                            unsafe { *p = child_state; }
                        }
                        cpu.num_processes += 1;
                        if let Some(p) = SCHED_STATE.get_ptr_mut(1) {
                            unsafe { *p = cpu.num_processes as u32; }
                        }
                        // Suspend parent: set bit in SCHED_STATE[2] (suspended_mask)
                        let suspended = match SCHED_STATE.get(2) {
                            Some(v) => *v,
                            None => 0,
                        };
                        if let Some(p) = SCHED_STATE.get_ptr_mut(2) {
                            unsafe { *p = suspended | (1 << parent_pid); }
                        }
                        increment_stat(STAT_FORKS);
                        cpu.regs[0] = child_pid;
                    }
                } else if syscall_nr == lsys::SYS_SCHED_YIELD {
                    // ── SYS_SCHED_YIELD(158): Voluntary context switch (Level 4c) ──
                    if cpu.num_processes > 1 {
                        scheduler_context_switch(cpu, flow_label, hop_id);
                    }
                    cpu.regs[0] = 0; // success
                } else if syscall_nr == lsys::SYS_READ_BLOCK {
                    // SYS_READ_BLOCK(200): r1=block_num, r2=buf_addr (dest in RAM).
                    // Copy 16 words per tick (verifier-friendly). Full 128-word block
                    // completes over 8 ticks using r3 as progress counter.
                    // Computermancer: DMA-style chunked transfer — 16 words/tick avoids
                    // verifier blowup from 128-iteration inner loop.
                    let block_num = cpu.regs[1];
                    let buf_addr = cpu.regs[2];
                    let progress = cpu.regs[3]; // word offset within block (0..128)
                    if block_num < blk::TOTAL_BLOCKS {
                        let src_base = blk::RAMDISK_BASE_WORD + block_num * blk::WORDS_PER_BLOCK + progress;
                        let dst_base = (buf_addr >> 2) + progress;
                        let mut w: u32 = 0;
                        while w < 16 {
                            let val = match RAM_MAP.get(src_base + w) {
                                Some(v) => *v,
                                None => 0,
                            };
                            if let Some(ptr) = RAM_MAP.get_ptr_mut(dst_base + w) {
                                unsafe { *ptr = val; }
                            }
                            w += 1;
                        }
                        let next_progress = progress + 16;
                        if next_progress >= 128 {
                            // Block complete
                            cpu.regs[0] = blk::BLOCK_SIZE; // 512 bytes read
                            cpu.regs[3] = 0;
                            increment_stat(STAT_BLOCK_OPS);
                        } else {
                            // More chunks needed — re-execute this syscall next tick
                            cpu.regs[3] = next_progress;
                            cpu.pc = cpu.pc.wrapping_sub(1); // back up PC to re-execute
                            break; // yield this tick
                        }
                    } else {
                        cpu.regs[0] = (-(lsys::EIO as i32)) as u32;
                    }
                } else if syscall_nr == lsys::SYS_WRITE_BLOCK {
                    // SYS_WRITE_BLOCK(201): r1=block_num, r2=buf_addr (src in RAM).
                    // Chunked: 16 words per tick, r3 = progress counter.
                    let block_num = cpu.regs[1];
                    let buf_addr = cpu.regs[2];
                    let progress = cpu.regs[3];
                    if block_num < blk::TOTAL_BLOCKS {
                        let dst_base = blk::RAMDISK_BASE_WORD + block_num * blk::WORDS_PER_BLOCK + progress;
                        let src_base = (buf_addr >> 2) + progress;
                        let mut w: u32 = 0;
                        while w < 16 {
                            let val = match RAM_MAP.get(src_base + w) {
                                Some(v) => *v,
                                None => 0,
                            };
                            if let Some(ptr) = RAM_MAP.get_ptr_mut(dst_base + w) {
                                unsafe { *ptr = val; }
                            }
                            w += 1;
                        }
                        let next_progress = progress + 16;
                        if next_progress >= 128 {
                            cpu.regs[0] = blk::BLOCK_SIZE; // 512 bytes written
                            cpu.regs[3] = 0;
                            increment_stat(STAT_BLOCK_OPS);
                        } else {
                            cpu.regs[3] = next_progress;
                            cpu.pc = cpu.pc.wrapping_sub(1);
                            break;
                        }
                    } else {
                        cpu.regs[0] = (-(lsys::EIO as i32)) as u32;
                    }
                // ── Level 5b FUZIX-critical syscalls ─────────────────
                } else if syscall_nr == lsys::SYS_READ {
                    // SYS_READ(3): r1=fd, r2=buf_addr, r3=len.
                    // If fd==0 (stdin), read from KBD_MAP circular queue.
                    let fd = cpu.regs[1];
                    let buf_addr = cpu.regs[2];
                    let len = cpu.regs[3];
                    if fd == 0 {
                        // Read keyboard events as bytes from KBD_MAP slots.
                        // Each non-zero KBD_MAP entry is consumed as one byte
                        // (low 8 bits of the key code, shifted right 1 to strip
                        // pressed flag — only deliver key-down events).
                        let max_read = if len < 8 { len } else { 8 };
                        let mut read_count: u32 = 0;
                        let mut slot: u32 = 0;
                        while slot < 8 && read_count < max_read {
                            let kv = match KBD_MAP.get(slot) {
                                Some(v) => *v,
                                None => 0,
                            };
                            if kv != 0 {
                                // Extract key code (bits 31:1), take low 8 bits as ASCII.
                                let pressed = kv & 1;
                                if pressed == 1 {
                                    let ch = ((kv >> 1) & 0xFF) as u8;
                                    // Write byte to destination buffer.
                                    let dst_word = buf_addr.wrapping_add(read_count) >> 2;
                                    let dst_byte_off = (buf_addr.wrapping_add(read_count) & 3) as u32;
                                    let existing = mem_read_word(dst_word);
                                    let mask = !(0xFFu32 << (dst_byte_off * 8));
                                    let new_val = (existing & mask) | ((ch as u32) << (dst_byte_off * 8));
                                    mem_write_word(dst_word, new_val);
                                    read_count += 1;
                                }
                                // Consume the event (clear slot).
                                if let Some(p) = KBD_MAP.get_ptr_mut(slot) {
                                    unsafe { *p = 0; }
                                }
                            }
                            slot += 1;
                        }
                        increment_stat(STAT_TTY_READS);
                        cpu.regs[0] = read_count;
                    } else {
                        // Unsupported fd for read: return -EBADF (9)
                        cpu.regs[0] = (-9i32) as u32;
                    }
                } else if syscall_nr == lsys::SYS_OPEN {
                    // SYS_OPEN(5): stub — return fd=3 (first available fd).
                    // No real filesystem yet; any open succeeds with fd 3.
                    cpu.regs[0] = 3;
                } else if syscall_nr == lsys::SYS_CLOSE {
                    // SYS_CLOSE(6): no-op, return 0 (success).
                    cpu.regs[0] = 0;
                } else if syscall_nr == lsys::SYS_WAITPID {
                    // SYS_WAITPID(7): r1=pid to wait for.
                    // Simplified: check if target process is halted. If not,
                    // yield and re-check next tick (set sleep to force re-entry).
                    let target_pid = cpu.regs[1];
                    if target_pid < cpu.num_processes as u32 {
                        // Check if the target process has exited by reading its
                        // state from PROC_TABLE. Convention: slot[17] bit 7 set
                        // means halted, or we check the halted_mask.
                        let halted = match SCHED_STATE.get(3) {
                            Some(v) => *v, // halted_mask (SCHED_STATE[3])
                            None => 0,
                        };
                        if (halted >> target_pid) & 1 != 0 {
                            // Child has exited — return the pid.
                            cpu.regs[0] = target_pid;
                        } else {
                            // Not exited yet — yield and retry.
                            // Set a short sleep so we re-check next tick.
                            increment_stat(STAT_WAITPIDS);
                            if cpu.num_processes > 1 {
                                scheduler_context_switch(cpu, flow_label, hop_id);
                            }
                            // Back up PC to re-execute this INT 0x80 on next tick.
                            cpu.pc = cpu.pc.wrapping_sub(1);
                            break;
                        }
                    } else {
                        // Invalid pid: return -ECHILD (10)
                        cpu.regs[0] = (-10i32) as u32;
                    }
                // ── MMU control syscalls (Level 4d) ──────────────────
                } else if syscall_nr == lsys::SYS_SET_PAGE_DIR {
                    // SYS_SET_PAGE_DIR(250): r1 = physical address of page directory.
                    cpu.page_dir_base = cpu.regs[1];
                    cpu.regs[0] = 0;
                } else if syscall_nr == lsys::SYS_ENABLE_MMU {
                    // SYS_ENABLE_MMU(251): enable paging.
                    cpu.mmu_enabled = 1;
                    cpu.regs[0] = 0;
                } else if syscall_nr == lsys::SYS_FLUSH_TLB {
                    // SYS_FLUSH_TLB(252): invalidate all TLB entries.
                    // Chunked: 8 entries per tick (verifier friendly). r3 = progress.
                    let progress = cpu.regs[3];
                    let mut idx: u32 = progress;
                    let end = if progress + 8 < 64 { progress + 8 } else { 64 };
                    while idx < end {
                        if let Some(p) = TLB_MAP.get_ptr_mut(idx) {
                            unsafe {
                                (*p)[0] = 0;
                                (*p)[1] = 0;
                                (*p)[2] = 0;
                            }
                        }
                        idx += 1;
                    }
                    if end < 64 {
                        cpu.regs[3] = end;
                        cpu.pc = cpu.pc.wrapping_sub(1);
                        break;
                    }
                    cpu.regs[3] = 0;
                    cpu.regs[0] = 0;
                } else if syscall_nr == lsys::SYS_EXECVE {
                    // ── SYS_EXECVE(11): Replace current process image ──
                    // r1 = entry_point (ROM word address to jump to).
                    // Binary is pre-loaded in ROM_MAP by boot loader / parent.
                    // We just reset CPU state and jump.
                    let entry_point = cpu.regs[1];

                    // Reset all general-purpose registers
                    let mut r: u32 = 0;
                    while r < 16 {
                        cpu.regs[r as usize] = 0;
                        r += 1;
                    }
                    // Reset stack pointer to top of memory
                    cpu.regs[15] = 0xFFFF_0000;

                    // Jump to entry point
                    cpu.pc = entry_point;
                    cpu.flags = 0;

                    // Reset interrupt state
                    cpu.interrupts_enabled = 0;
                    cpu.interrupt_pending = 0;

                    // Reset program break to default
                    cpu.program_break = DEFAULT_PROGRAM_BREAK;

                    // Unsuspend any parent waiting via vfork
                    if let Some(p) = SCHED_STATE.get_ptr_mut(2) {
                        unsafe { *p = 0; } // clear all suspended bits
                    }

                    increment_stat(STAT_SYSCALLS);

                // ── Trivial FUZIX syscall stubs (Level 5c) ───────────────
                // UID/GID family: always root (0)
                } else if syscall_nr == lsys::SYS_GETUID
                       || syscall_nr == lsys::SYS_GETGID
                       || syscall_nr == lsys::SYS_GETEUID
                       || syscall_nr == lsys::SYS_GETEGID
                       || syscall_nr == lsys::SYS_SETUID
                       || syscall_nr == lsys::SYS_SETGID {
                    cpu.regs[0] = 0;
                    increment_stat(STAT_TRIVIAL_SYSCALLS);
                } else if syscall_nr == lsys::SYS_DUP {
                    // SYS_DUP(41): r1=oldfd. Return oldfd (stub).
                    cpu.regs[0] = cpu.regs[1];
                    increment_stat(STAT_TRIVIAL_SYSCALLS);
                } else if syscall_nr == lsys::SYS_DUP2 {
                    // SYS_DUP2(63): r1=oldfd, r2=newfd. Return newfd (stub).
                    cpu.regs[0] = cpu.regs[2];
                    increment_stat(STAT_TRIVIAL_SYSCALLS);
                } else if syscall_nr == lsys::SYS_SIGNAL {
                    // SYS_SIGNAL(48): r1=signum, r2=handler_addr.
                    // Store handler in signal table, return old handler.
                    let signum = cpu.regs[1];
                    let handler = cpu.regs[2];
                    if signum < lsys::SIGNAL_MAX_SLOTS {
                        let slot_word = (lsys::SIGNAL_TABLE_BASE >> 2) + signum;
                        let old_handler = mem_read_word(slot_word);
                        mem_write_word(slot_word, handler);
                        cpu.regs[0] = old_handler;
                    } else {
                        cpu.regs[0] = 0;
                    }
                    increment_stat(STAT_MODERATE_SYSCALLS);
                } else if syscall_nr == lsys::SYS_KILL {
                    // SYS_KILL(37): r1=pid, r2=sig. No-op.
                    cpu.regs[0] = 0;
                    increment_stat(STAT_TRIVIAL_SYSCALLS);
                } else if syscall_nr == lsys::SYS_STAT || syscall_nr == lsys::SYS_FSTAT {
                    // SYS_STAT(106) / SYS_FSTAT(108): return minimal stat struct.
                    let buf = cpu.regs[2];
                    let buf_word = buf >> 2;
                    mem_write_word(buf_word, 0o100644);     // st_mode (regular file)
                    mem_write_word(buf_word + 1, 4096);     // st_size
                    mem_write_word(buf_word + 2, 512);      // st_blksize
                    cpu.regs[0] = 0;
                    increment_stat(STAT_MODERATE_SYSCALLS);
                } else if syscall_nr == lsys::SYS_LSEEK {
                    // SYS_LSEEK(19): stub — pretend seek succeeded.
                    cpu.regs[0] = 0;
                    increment_stat(STAT_TRIVIAL_SYSCALLS);
                } else if syscall_nr == lsys::SYS_GETPPID {
                    // SYS_GETPPID(64): init is always parent.
                    cpu.regs[0] = 0;
                    increment_stat(STAT_TRIVIAL_SYSCALLS);
                } else if syscall_nr == lsys::SYS_UMASK {
                    // SYS_UMASK(60): r1=mask. Return default 022, ignore the set.
                    cpu.regs[0] = 0o22;
                    increment_stat(STAT_TRIVIAL_SYSCALLS);
                } else if syscall_nr == lsys::SYS_SYNC {
                    // SYS_SYNC(36): no-op, no real filesystem.
                    cpu.regs[0] = 0;
                    increment_stat(STAT_TRIVIAL_SYSCALLS);
                } else if syscall_nr == lsys::SYS_TIMES {
                    // SYS_TIMES(43): no process accounting.
                    cpu.regs[0] = 0;
                    increment_stat(STAT_TRIVIAL_SYSCALLS);
                } else if syscall_nr == lsys::SYS_IOCTL {
                    // SYS_IOCTL(54): r1=fd, r2=request, r3=argp.
                    // TIOCGWINSZ (0x5413): write terminal size to argp.
                    // TCGETS (0x5401): return termios (success stub).
                    // Everything else: return 0 (success).
                    if cpu.regs[2] == 0x5413 {
                        let argp = cpu.regs[3];
                        // struct winsize: rows(u16) | cols(u16) packed into one word
                        mem_write_word(argp >> 2, 24 | (80 << 16)); // 24 rows, 80 cols
                    }
                    cpu.regs[0] = 0;
                    increment_stat(STAT_MODERATE_SYSCALLS);
                } else if syscall_nr == lsys::SYS_PIPE {
                    // SYS_PIPE(42): r1=pipefd[2] array address.
                    // Allocate stub fd numbers (10=read, 11=write). No actual pipe buffer.
                    let pipefd_addr = cpu.regs[1];
                    mem_write_word(pipefd_addr >> 2, 10);       // read end
                    mem_write_word((pipefd_addr >> 2) + 1, 11); // write end
                    cpu.regs[0] = 0;
                    increment_stat(STAT_MODERATE_SYSCALLS);
                } else if syscall_nr == lsys::SYS_FCNTL {
                    // SYS_FCNTL(55): no-op.
                    cpu.regs[0] = 0;
                    increment_stat(STAT_TRIVIAL_SYSCALLS);

                // ── Moderate FUZIX syscalls (Level 5d) ──────────────────
                } else if syscall_nr == lsys::SYS_ACCESS {
                    // SYS_ACCESS(33): r1=pathname, r2=mode.
                    // Always return 0 (file exists and is accessible).
                    cpu.regs[0] = 0;
                    increment_stat(STAT_MODERATE_SYSCALLS);
                } else if syscall_nr == lsys::SYS_CHDIR {
                    // SYS_CHDIR(12): stub — always succeed, no real filesystem.
                    cpu.regs[0] = 0;
                    increment_stat(STAT_MODERATE_SYSCALLS);
                } else if syscall_nr == lsys::SYS_TIME {
                    // SYS_TIME(13): return seconds since boot (ktime_ns / 1e9).
                    let now = unsafe { bpf_ktime_get_ns() };
                    cpu.regs[0] = (now / 1_000_000_000) as u32;
                    increment_stat(STAT_MODERATE_SYSCALLS);
                } else {
                    // Unknown syscall: return -ENOSYS (38) in r0.
                    cpu.regs[0] = (-(lsys::ENOSYS as i32)) as u32;
                }
            } else {
                // Non-0x80 software interrupt: standard IVT dispatch.
                // Push flags
                cpu.regs[15] = cpu.regs[15].wrapping_sub(4);
                mem_write_word(cpu.regs[15] >> 2, cpu.flags as u32);
                // Push PC (already advanced past INT instruction = return address)
                cpu.regs[15] = cpu.regs[15].wrapping_sub(4);
                mem_write_word(cpu.regs[15] >> 2, cpu.pc);
                // Disable interrupts
                cpu.interrupts_enabled = 0;
                // Jump to handler
                let ivt_addr = (vector as u32) * 4;
                cpu.pc = mem_read_word(ivt_addr >> 2);
            }
        } else if opc == op::IRET {
            // Return from interrupt: pop PC + flags, re-enable interrupts.
            // Pop PC from stack
            let word_addr = cpu.regs[15] >> 2;
            cpu.pc = mem_read_word(word_addr);
            cpu.regs[15] = cpu.regs[15].wrapping_add(4);
            // Pop flags from stack
            let word_addr2 = cpu.regs[15] >> 2;
            cpu.flags = mem_read_word(word_addr2) as u8;
            cpu.regs[15] = cpu.regs[15].wrapping_add(4);
            // Re-enable interrupts
            cpu.interrupts_enabled = 1;

        // ── System ────────────────────────────────────────────────────────────
        } else if opc == op::SYSCALL {
            increment_stat(STAT_SYSCALLS);
            // RV32I ecall convention: a7 (x17 → r1 via spill) = syscall number,
            // a0 (x10 → r8) = first arg / return value,
            // a1 (x11 → r9) = second arg / return value.
            let syscall_nr = cpu.regs[1];
            if syscall_nr == sys::SYS_DRAW_FRAME {
                // DG_DrawFrame: framebuffer at SCREEN_BASE.
                // Increment frame-ready counter so userspace (doom-bridge) can detect
                // new frames and bulk-copy RAM_MAP → SCREEN_MAP without BPF verifier limits.
                increment_stat(STAT_FRAME_READY);
                // Emit ring buffer event only every 32nd frame to reduce event pressure.
                let frame_count = match unsafe { STATS.get(&STAT_FRAME_READY) } {
                    Some(v) => *v,
                    None => 1,
                };
                if (frame_count & 0x1F) == 0 {
                    emit_screen_write(flow_label, mmap::SCREEN_BASE, hop_id);
                }
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
                        let key_code = (kv >> 1) & 0xFF;
                        let pressed = kv & 1;
                        // Pack for I_StartTic: bit 31 = pressed, bits 0-7 = scancode
                        cpu.regs[8] = key_code | (pressed << 31);
                        cpu.regs[9] = pressed;
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
                increment_stat(STAT_SLEEPING);
                break;
            }
            // Unknown syscall: silently ignore (fail-safe).
        } else if opc == op::HALT {
            cpu.halted = 1;
            increment_stat(STAT_HALTED);
            // SP diagnostic: record r15 and PC at halt time
            let sp_key: u32 = 24;
            if let Some(ptr) = STATS.get_ptr_mut(&sp_key) {
                // Pack: high32 = r15 (SP), low32 = PC at halt
                unsafe { *ptr = ((cpu.regs[15] as u64) << 32) | (cpu.pc as u64); }
            }
            emit_compute_halt(flow_label, cpu.insn_count, hop_id);
            break;
        }
        // Unknown opcode: treat as NOP (fail-open for forward compat).

        i += 1;
        cpu.insn_count += 1;
        increment_stat(STAT_INSNS_EXECUTED);
    }

    // Tail call chain moved to monad_cpu() entry point (required by kernel —
    // tail_call not allowed in subprogs without BTF).

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
#[inline(always)]
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

/// Emit a TTY_WRITE event to the ring buffer (Level 4f console I/O).
#[inline(always)]
#[inline(always)]
fn emit_tty_write(flow_label: u32, bytes_written: u32, hop_id: u8) {
    if let Some(mut entry) = COMPUTE_EVENTS.reserve::<ComputeHopEvent>(0) {
        let event = ComputeHopEvent {
            timestamp_ns: unsafe { bpf_ktime_get_ns() },
            event_type: EVENT_TTY_WRITE,
            hop_id,
            _pad: [0; 2],
            flow_label,
            pc: 0,
            instruction: bytes_written, // Reuse instruction field for byte count
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

// ── Level 4c: Round-Robin Scheduler ──────────────────────────────────────────

/// Perform a round-robin context switch.
///
/// 1. Save current process state (regs + PC + flags + SP + brk) to PROC_TABLE
/// 2. Advance to next runnable process (skip halted ones)
/// 3. Load that process's state from PROC_TABLE
/// 4. Update current_pid in cpu state and SCHED_STATE
///
/// All loops bounded to MAX_PROCESSES (4) for BPF verifier.
#[inline(always)]
#[inline(always)]
fn scheduler_context_switch(cpu: &mut MbcCpuState, flow_label: u32, hop_id: u8) {
    let old_pid = cpu.current_pid as u32;
    let num = cpu.num_processes as u32;

    // Guard: need at least 2 processes
    if num <= 1 {
        return;
    }

    // 1. Save current process state to PROC_TABLE[current_pid]
    let mut save_state = [0u32; 20];
    let mut r = 0u32;
    while r < 16 {
        save_state[r as usize] = cpu.regs[r as usize];
        r += 1;
    }
    save_state[16] = cpu.pc;
    save_state[17] = cpu.flags as u32;
    save_state[18] = cpu.regs[15]; // SP copy
    save_state[19] = cpu.program_break;

    if let Some(p) = PROC_TABLE.get_ptr_mut(old_pid) {
        unsafe { *p = save_state; }
    }

    // 2. Find next runnable process (round-robin, skip halted and suspended)
    // Read halted_mask from SCHED_STATE[3]
    let halted_mask = match SCHED_STATE.get(3) {
        Some(v) => *v,
        None => 0,
    };
    // Read suspended_mask from SCHED_STATE[2]
    let suspended_mask = match SCHED_STATE.get(2) {
        Some(v) => *v,
        None => 0,
    };
    let skip_mask = halted_mask | suspended_mask;

    let mut next_pid = (old_pid + 1) % num;
    // Bounded search: try up to 4 candidates
    let mut attempts = 0u32;
    while attempts < 4 {
        if next_pid < num && (skip_mask & (1 << next_pid)) == 0 {
            break; // found a runnable process
        }
        next_pid = (next_pid + 1) % num;
        attempts += 1;
    }

    // If all processes halted (or only self runnable), stay on current
    if next_pid == old_pid {
        return;
    }

    // 3. Load next process state from PROC_TABLE[next_pid]
    let load_state = match PROC_TABLE.get(next_pid) {
        Some(s) => *s,
        None => return, // safety: shouldn't happen
    };

    let mut r2 = 0u32;
    while r2 < 16 {
        cpu.regs[r2 as usize] = load_state[r2 as usize];
        r2 += 1;
    }
    cpu.pc = load_state[16];
    cpu.flags = load_state[17] as u8;
    // load_state[18] is SP copy — already in regs[15] from the load above
    cpu.program_break = load_state[19];

    // 4. Update current_pid
    cpu.current_pid = next_pid as u8;

    // Update SCHED_STATE[0] = new current_pid
    if let Some(p) = SCHED_STATE.get_ptr_mut(0) {
        unsafe { *p = next_pid; }
    }

    increment_stat(STAT_CONTEXT_SWITCHES);
    emit_context_switch(flow_label, old_pid, next_pid, hop_id);
}

/// Emit a CONTEXT_SWITCH event to the ring buffer (Level 4c scheduler).
#[inline(always)]
#[inline(always)]
fn emit_context_switch(flow_label: u32, from_pid: u32, to_pid: u32, hop_id: u8) {
    if let Some(mut entry) = COMPUTE_EVENTS.reserve::<ComputeHopEvent>(0) {
        let event = ComputeHopEvent {
            timestamp_ns: unsafe { bpf_ktime_get_ns() },
            event_type: EVENT_CONTEXT_SWITCH,
            hop_id,
            _pad: [0; 2],
            flow_label,
            pc: from_pid,              // reuse pc field for source pid
            instruction: to_pid,       // reuse instruction field for dest pid
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
/// Translate a virtual address to a physical address using two-level page tables
/// and a direct-mapped software TLB (Level 4d MMU).
///
/// When `cpu.mmu_enabled == 0`, returns `vaddr` unchanged (flat addressing, Doom mode).
/// When enabled, checks the 64-entry TLB first (O(1) array lookup), then walks
/// the two-level page table on miss (2 bounded RAM_MAP reads — verifier-safe).
///
/// BPF verifier: this function is #[inline(always)] to avoid call overhead.
/// The page table walk is bounded (2 reads), TLB access is 1 Array lookup.
#[inline(always)]
#[inline(always)]
fn translate_address(cpu: &MbcCpuState, vaddr: u32) -> u32 {
    if cpu.mmu_enabled == 0 {
        return vaddr; // Flat mode — no translation (Doom, backward compat)
    }

    let vpn = vaddr >> 12;
    let offset = vaddr & 0xFFF;
    let tlb_idx = vpn & 63; // Direct-mapped

    // Check TLB
    if let Some(entry) = TLB_MAP.get(tlb_idx) {
        if entry[0] == vpn && (entry[2] & mmu::PTE_PRESENT) != 0 {
            // TLB hit
            increment_stat(STAT_TLB_HITS);
            return (entry[1] << 12) | offset;
        }
    }

    // TLB miss — walk page tables
    increment_stat(STAT_TLB_MISSES);
    let pde_idx = vaddr >> 22;
    let pte_idx = (vaddr >> 12) & 0x3FF;

    // Read page directory entry
    let pd_word_addr = (cpu.page_dir_base >> 2) + pde_idx;
    let pde = mem_read_word(pd_word_addr);
    if (pde & mmu::PTE_PRESENT) == 0 {
        return vaddr; // Page fault — return unmapped (handle gracefully)
    }

    // Read page table entry
    let pt_base = pde & mmu::PTE_PFN_MASK;
    let pt_word_addr = (pt_base >> 2) + pte_idx;
    let pte = mem_read_word(pt_word_addr);
    if (pte & mmu::PTE_PRESENT) == 0 {
        return vaddr; // Page fault
    }

    let pfn = (pte & mmu::PTE_PFN_MASK) >> 12;

    // Update TLB
    if let Some(p) = TLB_MAP.get_ptr_mut(tlb_idx) {
        unsafe {
            (*p)[0] = vpn;
            (*p)[1] = pfn;
            (*p)[2] = pte;
        }
    }

    (pfn << 12) | offset
}

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
#[inline(always)]
fn mem_write_word(word_addr: u32, value: u32) {
    // Bug 24 fix: Do NOT write to SCREEN_MAP from word stores (ST).
    // Only byte stores (STB → mem_write_byte) update SCREEN_MAP.
    //
    // Rationale: DOOM's BSS corruption causes random code to write WAD data
    // structures (lump names like "COMPTALL", "BROWN96") through corrupted
    // pointers that land in the screen region (0x70000-0x73E80). Word stores
    // from these writes were overwriting SCREEN_MAP with garbage.
    //
    // The authoritative screen update path is copy_fb_to_screen() which uses
    // explicit byte stores (SB → mem_write_byte → SCREEN_MAP). By removing
    // SCREEN_MAP writes from word stores, only intentional pixel writes reach
    // the display.

    // Write to RAM_MAP only.
    if let Some(ptr) = RAM_MAP.get_ptr_mut(word_addr) {
        unsafe {
            *ptr = value;
        }
    }
}

/// Read a single byte from the MBC address space.
#[allow(dead_code)]
#[inline(always)]
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
#[inline(always)]
fn mem_read_half(byte_addr: u32) -> u16 {
    let word_addr = byte_addr >> 2;
    let half_shift = (byte_addr & 2) * 8; // 0 for low half, 16 for high half
    let word = mem_read_word(word_addr);
    ((word >> half_shift) & 0xFFFF) as u16
}

/// Write a 16-bit halfword to the MBC address space (little-endian).
#[inline(always)]
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
/// STUB: The loop (64 000 bytes at 320x200) would exceed the BPF verifier's
/// 8192-jump complexity limit.  Framebuffer copy is deferred to userspace:
/// the emit_screen_write() event signals the dashboard to read SCREEN_MAP
/// directly.  mem_write_word() already projects screen-region writes into
/// SCREEN_MAP, so individual pixel writes during rendering are still visible.
///
/// When full Doom rendering is needed, this can be implemented via:
///   (a) BPF tail call to a dedicated copy program, or
///   (b) Userspace poller that bulk-copies RAM_MAP → SCREEN_MAP on event.
#[inline(always)]
#[inline(always)]
fn copy_fb_to_screen(_fb_ptr: u32) {
    // No-op: verifier cannot handle 16K iterations.
    // Screen writes via mem_write_word/mem_write_byte still work individually.
}

// ── Monad XDP read ────────────────────────────────────────────────────────────

/// Read 20 Monad bytes from XDP packet memory.
#[inline(always)]
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
