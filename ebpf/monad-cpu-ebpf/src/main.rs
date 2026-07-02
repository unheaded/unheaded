// SPDX-License-Identifier: GPL-3.0-or-later
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

mod fdtable;
mod phase12;

use aya_ebpf::{
    bindings::xdp_action,
    helpers::bpf_ktime_get_ns,
    macros::{map, xdp},
    maps::{Array, HashMap, LruHashMap, ProgramArray, RingBuf},
    programs::XdpContext,
};
use monad_common::{
    flags, fs_walk, mbc_block as blk, mbc_flags as mf, mbc_interrupts as intr,
    mbc_linux_syscalls as lsys, mbc_mmap as mmap, mbc_mmu as mmu, mbc_opcodes as op,
    mbc_syscalls as sys, ComputeHopEvent, MbcCpuState, MbcInsn, Monad, DEFAULT_PROGRAM_BREAK,
    EVENT_CACHE_MISS, EVENT_COMPUTE_HALT, EVENT_CONTEXT_SWITCH, EVENT_SCREEN_WRITE,
    EVENT_TTY_WRITE, IPV6_FIXED_HDR_LEN, IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN, MONAD_OPT_TYPE,
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
/// Userspace polls this at 60 Hz to render the current frame.
#[map]
static SCREEN_MAP: Array<u8> = Array::with_max_entries(64_000, 0);

/// Keyboard state: 8 entries, one u32 per active key slot.
/// Encoding: `(scancode << 1) | pressed_flag`
/// Userspace writes key events here.  CPU reads via SYS_GET_KEY.
#[map]
static KBD_MAP: Array<u32> = Array::with_max_entries(8, 0);

/// KBD ring producer/consumer indices (Gate C Stage 2). The host (upc-bootctl
/// `write_kbd`) fills KBD_MAP[0..n] and sets KBD_HEAD = n; the guest's console
/// read(5) drain consumes ONE byte at KBD_TAL per call and advances it. O(1) per
/// read — no 8-slot scan (verifier budget, hardening H2). Monotonic indices;
/// the slot is `idx & 7`.
#[map]
static KBD_HEAD: Array<u32> = Array::with_max_entries(1, 0);
#[map]
static KBD_TAIL: Array<u32> = Array::with_max_entries(1, 0);

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

/// Per-process RV2MBC base offset (ADR-077, Phase 1.7 Gate B). Under
/// ASCEND-LINUX each resident program (init, sh, …) occupies a disjoint
/// `RV2MBC_MAP` region, selected per-process by `cpu.rv2mbc_base`, so their
/// colliding RV-word-0 text ranges coexist. Every indirect-branch lookup
/// adds this base: `RV2MBC_MAP[rv2mbc_base + (rv_addr >> 2)]`.
///
/// In the non-ascend (Doom) build the field does not exist and each program
/// is the sole `RV2MBC_MAP` occupant, so the base is a compile-time 0 and the
/// lookup is byte-identical to before. This helper exists so the shared
/// JMPR/CALLR/RET sites compile in both feature configs.
#[cfg(feature = "ascend-linux")]
#[inline(always)]
fn rv2mbc_base_of(cpu: &MbcCpuState) -> u32 {
    cpu.rv2mbc_base
}
#[cfg(not(feature = "ascend-linux"))]
#[inline(always)]
fn rv2mbc_base_of(_cpu: &MbcCpuState) -> u32 {
    0
}

/// Set the per-process RV2MBC base (ADR-077). cfg-gated so the `exec(7)`
/// handler — whose body compiles in both feature configs behind a runtime
/// `cfg!` guard — can write the field without referencing it in the Doom
/// build (where it does not exist). No-op in the non-ascend build.
#[cfg(feature = "ascend-linux")]
#[inline(always)]
fn set_rv2mbc_base(cpu: &mut MbcCpuState, base: u32) {
    cpu.rv2mbc_base = base;
}
#[cfg(not(feature = "ascend-linux"))]
#[inline(always)]
fn set_rv2mbc_base(_cpu: &mut MbcCpuState, _base: u32) {}

/// Privilege-aware RV2MBC base for indirect-branch translation (ADR-077 Gate C
/// fix). The per-process base (`cpu.rv2mbc_base`) selects a *user* program's
/// disjoint RV2MBC region, but kernel (supervisor/machine) code lives in the
/// base-0 region shared by all processes. After `exec` set a user base, a
/// machine-mode trap (e.g. timer) whose MRET returns into supervisor code would
/// add the user base to a kernel return target and fault on an unmapped MBC
/// index (the Gate C `halt_reason=0x47`, mepc=0x27304 @ base 0x1000). Mirror the
/// MMU's own rule (`translate_address`: per-process only when `priv_level == 3`).
/// So: user mode → per-process base; kernel mode → 0. Doom build = compile-time 0.
#[cfg(feature = "ascend-linux")]
#[inline(always)]
fn rv2mbc_branch_base(cpu: &MbcCpuState) -> u32 {
    if cpu.priv_level == 3 {
        cpu.rv2mbc_base
    } else {
        0
    }
}
#[cfg(not(feature = "ascend-linux"))]
#[inline(always)]
fn rv2mbc_branch_base(_cpu: &MbcCpuState) -> u32 {
    0
}

/// 8 MiB per-pid user VA window (== `phase12::SLICE_STRIDE`). A user VA at or
/// above this escapes the pid's slice.
const USER_VA_CEILING: u32 = 0x0080_0000;

/// Bounded user-VA → physical translation for syscall buffer access (hardening
/// H1). The per-pid pgd maps user VA[0,8 MiB) linearly onto the pid's physical
/// slice, so `phys = pid_phys_offset(pid) + va` — NO Sv32 walk (the verifier
/// budget shortcut exec(7)/read(5) rely on). But `va` is USER-controlled: an
/// unbounded `phys` could hit another pid's slice, the ramdisk, staged `.data`,
/// or the guest page tables — and `wrapping_add` lets a huge `va` wrap into low
/// kernel memory. Reject overflow and any `[va, va+len)` that leaves the slice;
/// `None` ⇒ the caller returns -1/EFAULT and writes nothing.
#[inline(always)]
fn user_phys(pid: u8, va: u32, len: u32) -> Option<u32> {
    let end = va.checked_add(len)?;
    if end > USER_VA_CEILING {
        return None;
    }
    Some(phase12::pid_phys_offset(pid).wrapping_add(va))
}

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

/// Phase 1.4 forkret-bisect PC tracer. Slot 0 holds a countdown; when > 0,
/// the dispatch loop emits the current PC as 4 hex chars to TTY and decrements.
/// Set by the RET fix path on first large-r14 (cf. RET handler).
#[map]
static TRACE_HEAD: Array<u32> = Array::with_max_entries(1, 0);

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

/// Process table: 8 slots (Phase 1.3 ADR-075 D-1; was 4 in Phase 1.2).
/// Key = process_id (0-7), Value = saved register set (21 u32s = 84 bytes).
/// Layout per slot: [r0..r15, PC, flags, SP_copy, program_break, page_dir_base]
/// slot[20] = page_dir_base widened in Phase 1.2 (ADR-074 Option A); see
/// `phase12::PROC_TABLE_PGD_SLOT`. 8 slots support init + sh + 5 cmds with
/// one slot headroom for tests/forkbomb.
#[map]
static PROC_TABLE: Array<[u32; 22]> = Array::with_max_entries(8, 0);

/// Scheduler state: [0]=current_pid, [1]=num_processes, [2]=suspended_mask,
/// [3]=halted_mask, [4]=reaped_mask.
/// halted_mask: bit i set means process i has exited (ZOMBIE) — the scheduler
///   skips it but it stays in PROC_TABLE until reaped.
/// reaped_mask: bit i set means a parent wait() has already collected zombie i,
///   so a later wait() won't return the same pid twice (Phase 1.6).
#[map]
static SCHED_STATE: Array<u32> = Array::with_max_entries(5, 0);

/// Parent-of table (Gate C Stage 3): PARENT_OF[child_pid] = parent's pid. Set on
/// every fork, so wait() reaps only a process's DIRECT children — real xv6
/// semantics. Without it the wait scan used "any halted pid > me", so init (pid
/// 0) would reap sh's child (the grandchild-reap bug → sh's wait hangs). Default
/// 0 is safe: every live child is written on fork (pid reuse refreshes it), and
/// the wait scan skips p == self so init never reaps itself.
#[map]
static PARENT_OF: Array<u32> = Array::with_max_entries(8, 0);

/// Software TLB: 64 entries, direct-mapped by virtual page number (Level 4d).
/// Key = index (vpn & 63), Value = [vpn, pfn, flags] (3 u32s = 12 bytes).
/// Used by translate_address() for fast virtual-to-physical translation.
#[map]
static TLB_MAP: Array<[u32; 3]> = Array::with_max_entries(64, 0);

/// Per-process file-descriptor table (Phase 1.7 Gate A): one `NOFILE`-wide
/// row of descriptor *kind* tags per pid. Index = `pid*NOFILE + fd`
/// (`fdtable::fd_slot`); value ∈ {`FD_FREE`, `FD_CONSOLE`, …}. xv6's
/// `open`/`dup`/`close`/`read`/`fstat` consult it so init's console fds 0/1/2
/// route to the TTY MMIO and a forked child inherits its parent's open files.
#[map]
static FD_TABLE: Array<u32> = Array::with_max_entries(fdtable::FD_TABLE_LEN, 0);

/// Per-fd inode state (Phase 1.7 Gate D): `{inum, offset}` for every
/// [`fdtable::FD_INODE`] descriptor, one `NOFILE`-wide row per pid, indexed by
/// the same `fdtable::fd_slot(pid, fd)` as `FD_TABLE`. Value `[inum, offset]`:
/// `open` binds `[inum, 0]`, `read` advances `offset`, `close` clears it back
/// to `[0, 0]` so a recycled descriptor never carries a previous file's state.
/// The `FD_TABLE` kind tag is authoritative for *which* store applies to an fd.
#[map]
static FD_INODE_MAP: Array<[u32; 2]> = Array::with_max_entries(fdtable::FD_TABLE_LEN, 0);

/// Resident-program table (Phase 1.7 Gate B, ADR-077). The host loader
/// pre-translates each userland program (`sh`, `ls`, …) into a disjoint
/// `ROM_MAP` region + a disjoint `RV2MBC_MAP` base and records the result
/// here; `exec(7)` resolves a path basename to a slot and adopts it. Each
/// 8-word row is `[name_hash, entry_mbc, rv2mbc_base, data_dst_va,
/// data_src_va, data_len_words, valid, _resv]` (see `pt::*`). Indexed by
/// program slot, not pid.
#[map]
static PROGRAM_TABLE: Array<[u32; 8]> = Array::with_max_entries(16, 0);

/// PROGRAM_TABLE row field offsets — the host/BPF contract (ADR-077).
#[allow(dead_code)] // some fields are consumed only by the exec(7) handler
mod pt {
    /// Number of program slots (must match PROGRAM_TABLE max_entries).
    pub const SLOTS: u32 = 16;
    /// FNV-1a-32 hash of the program basename (e.g. "sh").
    pub const NAME_HASH: usize = 0;
    /// MBC ROM slot of the program entry point (`cpu.pc` adopts this).
    pub const ENTRY_MBC: usize = 1;
    /// Per-process RV2MBC_MAP index offset (→ `cpu.rv2mbc_base`).
    pub const RV2MBC_BASE: usize = 2;
    /// User byte VA where the flat .data/.rodata blob is copied.
    pub const DATA_DST_VA: usize = 3;
    /// RAM byte VA of the pristine host-staged flat blob (copy source).
    pub const DATA_SRC_VA: usize = 4;
    /// Number of 32-bit words to copy from src→dst at exec time.
    pub const DATA_LEN_WORDS: usize = 5;
    /// 1 = slot populated, 0 = empty.
    pub const VALID: usize = 6;
    /// FNV-1a-32 of a basename. Mirrors the host's `fnv1a` in upc-bootctl.
    pub fn name_hash(name: &[u8]) -> u32 {
        let mut h: u32 = 0x811c_9dc5;
        let mut i = 0usize;
        while i < name.len() {
            h ^= name[i] as u32;
            h = h.wrapping_mul(0x0100_0193);
            i += 1;
        }
        h
    }
}

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

// ── Phase 1.7 file-descriptor helpers (Gate A) ─────────────────────────────
// Thin BPF-side accessors over FD_TABLE; the index math + free-slot policy
// live (and are unit tested) in `fdtable`. All bound-check fd against NOFILE
// so a hostile/garbage fd can never index a neighbouring pid's row.

/// Kind tag for (`pid`, `fd`), or `FD_FREE` if out of range / closed.
#[inline(always)]
fn fd_kind(pid: u8, fd: u32) -> u32 {
    if fd >= fdtable::NOFILE {
        return fdtable::FD_FREE;
    }
    match FD_TABLE.get(fdtable::fd_slot(pid, fd)) {
        Some(v) => *v,
        None => fdtable::FD_FREE,
    }
}

/// Set the kind tag for (`pid`, `fd`). No-op if `fd >= NOFILE`.
#[inline(always)]
fn fd_set_kind(pid: u8, fd: u32, kind: u32) {
    if fd >= fdtable::NOFILE {
        return;
    }
    if let Some(p) = FD_TABLE.get_ptr_mut(fdtable::fd_slot(pid, fd)) {
        unsafe {
            *p = kind;
        }
    }
}

/// Lowest free fd for `pid`, or `NOFILE` (= "table full") if none — the caller
/// maps that to xv6's -1. Mirrors `fdtable::lowest_free` over the live map.
#[inline(always)]
fn fd_alloc(pid: u8) -> u32 {
    let mut fd = 0u32;
    while fd < fdtable::NOFILE {
        if fd_kind(pid, fd) == fdtable::FD_FREE {
            return fd;
        }
        fd += 1;
    }
    fdtable::NOFILE
}

// ── Phase 1.7 Gate D per-fd inode state (FD_INODE_MAP) ─────────────────────
// Thin BPF accessors over FD_INODE_MAP, indexed by the same `fdtable::fd_slot`
// as FD_TABLE. The pure storage model + invariants (disjoint rows, bind /
// advance / recycle-clear) are unit-tested in `monad_common::fdtable`. All
// bound-check `fd < NOFILE` so a hostile fd can never reach a neighbour's row.

/// `(inum, offset)` for (`pid`, `fd`), or `(0, 0)` if out of range / unbound.
#[inline(always)]
#[allow(dead_code)] // wired by Gate D read(5)/fstat(8) (Phases 4–5)
fn fd_inode(pid: u8, fd: u32) -> (u32, u32) {
    if fd >= fdtable::NOFILE {
        return (0, 0);
    }
    match FD_INODE_MAP.get(fdtable::fd_slot(pid, fd)) {
        Some(v) => (v[0], v[1]),
        None => (0, 0),
    }
}

/// Bind (`pid`, `fd`) to inode `inum` at offset 0 and tag it `FD_INODE`. The
/// open-time state for an inode-backed descriptor. No-op if `fd >= NOFILE`.
#[inline(always)]
#[allow(dead_code)] // wired by Gate D open(15) (Phase 3)
fn fd_set_inode(pid: u8, fd: u32, inum: u32) {
    if fd >= fdtable::NOFILE {
        return;
    }
    if let Some(p) = FD_INODE_MAP.get_ptr_mut(fdtable::fd_slot(pid, fd)) {
        unsafe {
            *p = [inum, 0];
        }
    }
    fd_set_kind(pid, fd, fdtable::FD_INODE);
}

/// Advance the stored read offset for (`pid`, `fd`), keeping `inum`. No-op if
/// `fd >= NOFILE`.
#[inline(always)]
#[allow(dead_code)] // wired by Gate D read(5) (Phase 5)
fn fd_set_offset(pid: u8, fd: u32, off: u32) {
    if fd >= fdtable::NOFILE {
        return;
    }
    if let Some(p) = FD_INODE_MAP.get_ptr_mut(fdtable::fd_slot(pid, fd)) {
        unsafe {
            (*p)[1] = off;
        }
    }
}

/// Clear (`pid`, `fd`)'s inode state on close/recycle so a reused descriptor
/// never inherits a previous file's inum or offset. No-op if `fd >= NOFILE`.
#[inline(always)]
fn fd_clear_inode(pid: u8, fd: u32) {
    if fd >= fdtable::NOFILE {
        return;
    }
    if let Some(p) = FD_INODE_MAP.get_ptr_mut(fdtable::fd_slot(pid, fd)) {
        unsafe {
            *p = [0, 0];
        }
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

    // ── Timer interrupt trigger ─────────────────────────────────────────────
    // Increment tick counter each XDP invocation. Every TIMER_TICK_DIVISOR
    // ticks (~12 Hz at 35 Hz XDP rate), fire a timer interrupt — but only
    // if interrupts are enabled and no interrupt is already pending.
    cpu.tick_counter = cpu.tick_counter.wrapping_add(1);
    // ASCEND-LINUX: xv6 manages its own interrupts via STVEC/sstatus and has
    // not programmed our flat IVT. Firing a BPF-level timer here causes
    // mem_read_word(ivt_addr) = 0 → PC reset to 0 → silent reboot. Gate the
    // timer behind feature so xv6 builds skip it. Doom path keeps firing.
    if cfg!(not(feature = "ascend-linux"))
        && cpu.tick_counter >= intr::TIMER_TICK_DIVISOR
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
    let mut prev_pc: u32 = cpu.pc; // Track last valid PC for post-mortem
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

        // Phase 1.4 PC tracer (forkret-bisect). When armed, emit current PC
        // as 4 hex chars to TTY and decrement countdown. Armed by RET-fix path.
        if let Some(p) = TRACE_HEAD.get_ptr_mut(0) {
            let n = unsafe { *p };
            if n > 0 {
                let hex = b"0123456789ABCDEF";
                let v = cpu.pc;
                mem_write_byte(0xC001, hex[((v >> 12) & 0xF) as usize]);
                mem_write_byte(0xC001, hex[((v >> 8) & 0xF) as usize]);
                mem_write_byte(0xC001, hex[((v >> 4) & 0xF) as usize]);
                mem_write_byte(0xC001, hex[(v & 0xF) as usize]);
                mem_write_byte(0xC001, b' ');
                unsafe {
                    *p = n - 1;
                }
            }
        }

        // Fetch — with explicit PC bounds check
        // BPF Array::get may not return None for large indices on all kernels.
        // Belt-and-suspenders: check explicitly, then check Array result.
        if cpu.pc >= 262_144_u32 {
            // PC out of ROM range — record bad PC AND last valid PC
            halt_diag(
                cpu.pc,
                rv2mbc_base_of(cpu),
                cpu.current_pid as u32,
                prev_pc,
                0x66,
            );
            cpu.halted = 1;
            break;
        }
        // Gate C diagnostic: an unexpected branch to PC=0 after boot re-runs
        // start_mbc → main() (the reboot loop seen after the rv2mbc_branch_base
        // fix). Catch it generically here — `prev_pc` still holds the culprit
        // branch's PC (set on the prior iteration). 1M-insn threshold skips the
        // legitimate PC=0 at cold boot.
        if cpu.pc == 0 && cpu.insn_count > 1_000_000 {
            halt_diag(
                cpu.pc,
                rv2mbc_branch_base(cpu),
                cpu.current_pid as u32,
                prev_pc,
                0x2E5E7,
            );
            cpu.halted = 1;
            break;
        }
        // SP monitor moved to HALT handler (verifier limit prevents per-insn checks).
        prev_pc = cpu.pc;
        let insn_word = match ROM_MAP.get(cpu.pc) {
            Some(w) => *w,
            None => {
                // ROM-fetch-miss: an indirect branch resolved to an unmapped
                // MBC index. Capture fault context (slots 25-29) for triage.
                halt_diag(
                    cpu.pc,
                    rv2mbc_base_of(cpu),
                    cpu.current_pid as u32,
                    prev_pc,
                    0x60,
                );
                cpu.halted = 1;
                increment_stat(STAT_ROM_FAULT);
                break;
            }
        };
        let insn = MbcInsn(insn_word);
        let opc = insn.opcode();
        let d = (insn.dst() as usize) & 0x0F;
        let s = (insn.src() as usize) & 0x0F;

        // DIAGNOSTIC: Check if PC enters key Doom render chain functions.
        // Gated behind `doom-debug` feature 2026-05-10 — kernel 6.17 BPF
        // verifier rejects the program with "infinite loop detected" when
        // all 14 PC checks compile in. xv6/uClinux/Linux paths build
        // without the feature; Doom debugging uses
        // `cargo build --features doom-debug`.
        #[cfg(feature = "doom-debug")]
        {
            let m = 0xE3000u32 >> 2;
            if cpu.pc == 13351 {
                mem_write_word(m, 0x0001);
            } // D_Display
            if cpu.pc == 12746 {
                mem_write_word(m + 11, 0x000C);
            } // D_Display.part.0
            if cpu.pc == 13034 {
                mem_write_word(m + 12, 0x000D);
            } // CALL to R_RenderPlayerView
            if cpu.pc == 13035 {
                mem_write_word(m + 13, 0x000E);
            } // After CALL returns
            if cpu.pc == 13678 {
                mem_write_word(m + 8, 0x0009);
            } // D_DoomLoop
            if cpu.pc == 14568 {
                mem_write_word(m + 9, 0x000A);
            } // D_DoomMain
            if cpu.pc == 20 {
                mem_write_word(m + 10, 0x000B);
            } // main
            if cpu.pc == 67345 {
                mem_write_word(m + 1, 0x0002);
            } // R_RenderPlayerView
            if cpu.pc == 62099 {
                mem_write_word(m + 2, 0x0003);
            } // R_RenderBSPNode
            if cpu.pc == 61968 {
                mem_write_word(m + 3, 0x0004);
            } // R_Subsector
            if cpu.pc == 61628 {
                mem_write_word(m + 4, 0x0005);
            } // R_AddLine
            if cpu.pc == 68648 {
                mem_write_word(m + 5, 0x0006);
            } // R_RenderSegLoop
            if cpu.pc == 67979 {
                mem_write_word(m + 6, 0x0007);
            } // R_DrawPlanes
            if cpu.pc == 69122 {
                mem_write_word(m + 7, 0x0008);
            } // R_StoreWallRange
        }
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
            // Guard against default-zero slots (BPF Array::get returns Some(&0)
            // for unset entries) — treat as unmapped to avoid silent PC=0 reset.
            let old_pc = cpu.pc.wrapping_sub(1); // PC of this JMPR instruction
            let rv_addr = cpu.regs[d];
            let rv_word = rv_addr >> 2;
            cpu.pc = match RV2MBC_MAP.get(rv_word.wrapping_add(rv2mbc_branch_base(cpu))) {
                Some(mbc_idx) if *mbc_idx != 0 => *mbc_idx,
                _ => {
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
        } else if opc == op::CALLR || (insn_word >> 24) == 0x2A {
            // GLOBAL CALLR COUNTER at STAT slot 20
            // Also log which path matched
            if opc == op::CALLR {
                increment_stat(20);
            }
            if (insn_word >> 24) == 0x2A {
                increment_stat(21);
            }

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

            cpu.pc = match RV2MBC_MAP.get(rv_word.wrapping_add(rv2mbc_branch_base(cpu))) {
                Some(mbc_idx) if *mbc_idx != 0 => {
                    // Log successful lookup
                    mem_write_word(callr_log_base + 4, *mbc_idx);
                    mem_write_word(callr_log_base + 5, 0xCA110001); // success marker
                    *mbc_idx
                }
                _ => {
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
            //
            // r14 holds one of two semantically-distinct things:
            //   (a) MBC PC saved by a prior CALL  — small value (< 0x10000)
            //   (b) RISC-V byte address loaded from a C struct field
            //       initialised with `(uint64)&function`  — large value
            //       (>= 0x10000 per kernel-mbc.ld layout: kernel .text
            //       at 0x20000, stage-1 stub at 0x10000, MBC PCs at
            //       ROM_MAP slots [0, 0x10000)).
            //
            // Case (a) is the compiled-RV path: CALL stores cpu.pc into
            // r14, callee saves/restores ra to/from stack via LW/SW, RET
            // reads it back. The stored value never escapes MBC space.
            //
            // Case (b) is the C-function-pointer pattern used by xv6's
            // scheduler / fork plumbing: `p->context.ra = (uint64)forkret;`
            // stores forkret's RV linker address. swtch_mbc.S loads it
            // into ra via `lw ra, 0(a1)` and rets. Without translation,
            // PC gets set to the RV byte address, walks through zero-NOPs
            // in ROM_MAP, and hits the bounds halt at 0x40000. See
            // references/phase14-session-2026-05-14-marshal-shift.md.
            //
            // Disambiguate by value: r14 >= 0x10000 means "RV byte address",
            // do rv2mbc lookup; otherwise treat as MBC PC directly.
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
            if ret >= 0x10000 {
                let rv_word = ret >> 2;
                cpu.pc = match RV2MBC_MAP.get(rv_word.wrapping_add(rv2mbc_branch_base(cpu))) {
                    Some(mbc_idx) if *mbc_idx != 0 => {
                        // Diagnostic: dump translated mbc_idx as 4 hex chars + arm tracer.
                        let v = *mbc_idx;
                        let hex = b"0123456789ABCDEF";
                        mem_write_byte(0xC001, b'<');
                        mem_write_byte(0xC001, hex[((v >> 12) & 0xF) as usize]);
                        mem_write_byte(0xC001, hex[((v >> 8) & 0xF) as usize]);
                        mem_write_byte(0xC001, hex[((v >> 4) & 0xF) as usize]);
                        mem_write_byte(0xC001, hex[(v & 0xF) as usize]);
                        mem_write_byte(0xC001, b'>');
                        // Tracer arming disabled — diagnostics moved to F1..F5 markers.
                        *mbc_idx
                    }
                    _ => {
                        mem_write_byte(0xC001, b'?');
                        ret
                    }
                };
            } else {
                cpu.pc = ret;
            }

        // ── Memory ────────────────────────────────────────────────────────────
        // All memory accesses go through translate_address() for MMU support
        // (Level 4d). When mmu_enabled == 0, translate_address returns the
        // address unchanged — ONE branch per access in Doom mode (acceptable).
        } else if opc == op::LD {
            // dst = RAM[src + simm16]  (32-bit word load)
            let raw_addr = cpu.regs[s].wrapping_add(simm as u32);
            let addr = translate_address(cpu, raw_addr);
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
            let addr = translate_address(cpu, raw_addr);
            let val = cpu.regs[s];
            mem_write_word(addr >> 2, val);
            cpu.cache_hits += 1; // mem_stores counter (legacy field name)
            increment_stat(STAT_MEM_STORES);
        } else if opc == op::LDB {
            // dst = zero_extend(RAM[src + simm16])  (byte load)
            let raw_addr = cpu.regs[s].wrapping_add(simm as u32);
            let addr = translate_address(cpu, raw_addr);
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
            let addr = translate_address(cpu, raw_addr);
            let val = (cpu.regs[s] & 0xFF) as u8;
            mem_write_byte(addr, val);
            cpu.cache_hits += 1;
            increment_stat(STAT_MEM_STORES);
        } else if opc == op::LDH {
            // dst = zero_extend(RAM[src + simm16])  (16-bit halfword load)
            let raw_addr = cpu.regs[s].wrapping_add(simm as u32);
            let addr = translate_address(cpu, raw_addr);
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
            let addr = translate_address(cpu, raw_addr);
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

        // ── ASCEND-LINUX (ADR-067) ────────────────────────────────────────────
        // The 5 new opcodes (FENCE/MRET/SRET/LR.W/SC.W) added in Phase 0 push
        // the kernel 6.17 BPF verifier past its state-pruning budget when
        // compiled together. Gate them behind `ascend-linux` so:
        //   - Doom build (default features) loads cleanly on 6.17.
        //   - xv6/uClinux/Linux build with `--features ascend-linux` to get
        //     the priv-level + atomic + barrier semantics they need.
        //   - When disabled, the if-arms short-circuit on cfg!() = false
        //     and the LLVM optimizer eliminates the dead branches, keeping
        //     the BPF program small enough for the verifier.
        // Phase 1.1 SHIP unblock 2026-05-10.
        } else if cfg!(feature = "ascend-linux") && opc == op::FENCE {
            // Full memory barrier. No-op on single-CPU MBC.
            // ADR-067 Decision 5.
        } else if cfg!(feature = "ascend-linux") && opc == op::MRET {
            // Machine-mode return. Pop MEPC + restore priv from MSTATUS.MPP.
            // MEPC holds an RV32 byte address (xv6 stores `&main` directly);
            // translate through RV2MBC_MAP to land on the correct MBC PC.
            // Phase 1.3 AP-2 fix — same translation pattern as JMPR/CALLR.
            //
            // Guard added 2026-05-14: BPF Array::get returns Some(&0) for
            // unset slots (not None). If MEPC points at a VA that wasn't
            // populated in rv2mbc (e.g. user VA 0 for an init proc whose
            // user binary isn't wired into the kernel image), the default
            // 0 would silently send PC to 0, resetting the CPU to start_mbc.c.
            // Treat zero result as unmapped and halt instead.
            let mepc = mem_read_word((0xF000 + 0x341 * 4) >> 2);
            let mstatus = mem_read_word((0xF000 + 0x300 * 4) >> 2);
            let mpp = ((mstatus >> 11) & 0b11) as u8;
            cpu.priv_level = mpp;
            let rv_word = (mepc >> 2) & 0xFFFF;
            cpu.pc = match RV2MBC_MAP.get(rv_word.wrapping_add(rv2mbc_branch_base(cpu))) {
                Some(mbc_idx) if *mbc_idx != 0 => *mbc_idx & 0xFFFF,
                _ => {
                    mem_write_word(0xE0050 >> 2, 0xDEAD0047); // MRET unmapped sentinel
                    mem_write_word(0xE0054 >> 2, mepc);
                    halt_diag(
                        mepc,
                        rv2mbc_base_of(cpu),
                        cpu.current_pid as u32,
                        prev_pc,
                        0x47,
                    );
                    cpu.halted = 1;
                    increment_stat(STAT_ROM_FAULT);
                    cpu.pc
                }
            };
            cpu.reservation_address = 0xFFFF_FFFF;
        } else if cfg!(feature = "ascend-linux") && opc == op::SRET {
            // Supervisor-mode return. SEPC + SSTATUS.SPP. Same RV2MBC translation
            // as MRET — SEPC holds an RV byte address.
            //
            // Guard added 2026-05-14 (same as MRET): default-zero slots in
            // RV2MBC_MAP must not silently route PC to 0.
            let sepc = mem_read_word((0xF000 + 0x141 * 4) >> 2);
            let sstatus = mem_read_word((0xF000 + 0x100 * 4) >> 2);
            let spp = ((sstatus >> 8) & 0b1) as u8;
            cpu.priv_level = if spp == 0 { 3 } else { 1 };
            let rv_word = (sepc >> 2) & 0xFFFF;
            cpu.pc = match RV2MBC_MAP.get(rv_word.wrapping_add(rv2mbc_branch_base(cpu))) {
                Some(mbc_idx) if *mbc_idx != 0 => {
                    // Diagnostic: emit 'S' on every SRET + arm PC tracer for
                    // first 100 user-mode instructions.
                    mem_write_byte(0xC001, b'S');
                    if let Some(p) = TRACE_HEAD.get_ptr_mut(0) {
                        unsafe {
                            *p = 100;
                        }
                    }
                    *mbc_idx & 0xFFFF
                }
                _ => {
                    mem_write_word(0xE0060 >> 2, 0xDEAD0048); // SRET unmapped sentinel
                    mem_write_word(0xE0064 >> 2, sepc);
                    halt_diag(
                        sepc,
                        rv2mbc_base_of(cpu),
                        cpu.current_pid as u32,
                        prev_pc,
                        0x48,
                    );
                    cpu.halted = 1;
                    increment_stat(STAT_ROM_FAULT);
                    cpu.pc
                }
            };
            cpu.reservation_address = 0xFFFF_FFFF;
        } else if cfg!(feature = "ascend-linux") && opc == op::LR_W {
            // Load-Reserved Word: rd = RAM[rs1]; reserve rs1.
            let addr = cpu.regs[s];
            let word_addr = addr >> 2;
            cpu.regs[d] = mem_read_word(word_addr);
            cpu.reservation_address = addr;
        } else if cfg!(feature = "ascend-linux") && opc == op::SC_W {
            // Store-Conditional Word: if reservation valid on rs1,
            // RAM[rs1] = rs2 (value reg = imm[3:0]); rd = 0 success / 1 failure.
            let addr = cpu.regs[s];
            let value_reg = (imm & 0xF) as usize;
            let word_addr = addr >> 2;
            if cpu.reservation_address == addr {
                mem_write_word(word_addr, cpu.regs[value_reg]);
                cpu.regs[d] = 0;
            } else {
                cpu.regs[d] = 1;
            }
            cpu.reservation_address = 0xFFFF_FFFF;

        // ── Interrupts ────────────────────────────────────────────────────────
        } else if opc == op::INT {
            let vector = imm as u8;
            // The INT-0x80 Linux/FUZIX dispatch below is dead weight under
            // ASCEND-LINUX: xv6 issues syscalls via `ecall` (op::SYSCALL), never
            // INT 0x80. Gating it with `!cfg!(ascend-linux)` lets rustc
            // dead-eliminate the whole ~560-line block before the BPF verifier
            // sees it, freeing the complexity budget the exec(7) handler needs
            // (the program sits right at the 1M processed-insn ceiling — adding
            // exec without this frees no room and the object fails to load).
            // Doom (non-ascend) keeps the dispatch exactly as before.
            if !cfg!(feature = "ascend-linux") && vector == intr::VECTOR_SYSCALL {
                // ── INT 0x80: Linux-compatible syscall dispatch (Level 4b) ────
                // Convention: r0 = syscall number, r1-r3 = args.
                // PC is NOT pushed (this is a synchronous call, not an interrupt).
                increment_stat(STAT_LINUX_SYSCALLS);
                let syscall_nr = cpu.regs[0];
                if syscall_nr == lsys::SYS_EXIT {
                    // SYS_EXIT(1): exiting process is marked ZOMBIE (halted_mask
                    // bit set); slot stays allocated until parent SYS_WAITPID
                    // reaps it. Phase 1.3 ADR-075 D-5 (xv6 kfork()-authoritative
                    // model — eBPF just owns the CPU/scheduler bookkeeping).
                    cpu.exit_code = cpu.regs[1];
                    let exiting_pid = cpu.current_pid as u32;
                    // Set halted_mask bit for current_pid in SCHED_STATE[3].
                    if let Some(p) = SCHED_STATE.get_ptr_mut(3) {
                        unsafe {
                            *p |= 1u32 << exiting_pid;
                        }
                    }
                    // Clear suspended_mask entirely — any parent vfork-waiting
                    // for any child wakes up. Phase 1.3 Step 4 will refine to
                    // per-pid wait queues; for now keep the broad-wake semantics.
                    if let Some(p) = SCHED_STATE.get_ptr_mut(2) {
                        unsafe {
                            *p = 0;
                        }
                    }
                    // Yield if any other process can run; else halt the CPU.
                    if cpu.num_processes > 1 {
                        scheduler_context_switch(cpu, flow_label, hop_id);
                        // If scheduler_context_switch stayed on same pid (all
                        // others halted), set CPU halted so we don't spin.
                        if cpu.current_pid as u32 == exiting_pid {
                            cpu.halted = 1;
                            increment_stat(STAT_HALTED);
                            break;
                        }
                        // Otherwise CPU keeps running as the next process.
                    } else {
                        cpu.halted = 1;
                        increment_stat(STAT_HALTED);
                        break;
                    }
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
                                unsafe {
                                    *p = byte_val;
                                }
                            }
                            h = h.wrapping_add(1);
                            written += 1;
                            b += 1;
                        }
                        // Update TTY head
                        if let Some(p) = TTY_HEAD.get_ptr_mut(0) {
                            unsafe {
                                *p = h;
                            }
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
                } else if !cfg!(feature = "ascend-linux") && syscall_nr == lsys::SYS_FORK {
                    // ── SYS_FORK(2): Create child process (Level 4c scheduler) ──
                    // Gated off under ascend-linux: xv6 manages processes via the
                    // RV32I ecall path, and this INT-0x80 handler was inflating
                    // cpu.num_processes during kernel boot (forkret/userinit).
                    // Copy current registers to next PROC_TABLE slot.
                    // Parent gets child_pid in r0, child gets 0 in r0.
                    if cpu.num_processes >= intr::MAX_PROCESSES {
                        // No room — return -EAGAIN (11)
                        cpu.regs[0] = (-11i32) as u32;
                    } else {
                        let child_pid = cpu.num_processes as u32;
                        // Save child state: copy parent's regs + PC + flags + SP + brk + pgd
                        let mut child_state = [0u32; 22];
                        let mut r = 0u32;
                        while r < 16 {
                            child_state[r as usize] = cpu.regs[r as usize];
                            r += 1;
                        }
                        child_state[16] = cpu.pc; // child resumes at same PC
                        child_state[17] = cpu.flags as u32;
                        child_state[18] = cpu.regs[15]; // SP copy
                        child_state[19] = cpu.program_break;
                        // Phase 1.2 (ADR-074 Option A + Allocator A1): each child gets
                        // its own pgd at the per-pid fixed region. Child inherits the
                        // address space identity but not the parent's pgd contents —
                        // xv6 kernel/proc.c::fork() COWs / shares mappings as needed.
                        child_state[phase12::PROC_TABLE_PGD_SLOT] =
                            phase12::pgd_base_for_pid(child_pid as u8);
                        // Child gets 0 in r0 (fork return value)
                        child_state[0] = 0;
                        // Write child state to PROC_TABLE
                        if let Some(p) = PROC_TABLE.get_ptr_mut(child_pid) {
                            unsafe {
                                *p = child_state;
                            }
                        }
                        cpu.num_processes += 1;
                        // Update SCHED_STATE[1] = num_processes
                        if let Some(p) = SCHED_STATE.get_ptr_mut(1) {
                            unsafe {
                                *p = cpu.num_processes as u32;
                            }
                        }
                        increment_stat(STAT_FORKS);
                        // Parent gets child_pid in r0
                        cpu.regs[0] = child_pid;
                    }
                } else if !cfg!(feature = "ascend-linux") && syscall_nr == lsys::SYS_VFORK {
                    // ── SYS_VFORK(190): Fork + suspend parent (Level 6) ──
                    // Like SYS_FORK but parent is suspended until child exits/execve.
                    if cpu.num_processes >= intr::MAX_PROCESSES {
                        cpu.regs[0] = (-11i32) as u32; // EAGAIN
                    } else {
                        let parent_pid = cpu.current_pid as u32;
                        let child_pid = cpu.num_processes as u32;
                        let mut child_state = [0u32; 22];
                        let mut r = 0u32;
                        while r < 16 {
                            child_state[r as usize] = cpu.regs[r as usize];
                            r += 1;
                        }
                        child_state[16] = cpu.pc;
                        child_state[17] = cpu.flags as u32;
                        child_state[18] = cpu.regs[15]; // SP copy
                        child_state[19] = cpu.program_break;
                        // Phase 1.2 (ADR-074 Option A): assign child its per-pid pgd.
                        child_state[phase12::PROC_TABLE_PGD_SLOT] =
                            phase12::pgd_base_for_pid(child_pid as u8);
                        child_state[0] = 0; // child gets 0
                        if let Some(p) = PROC_TABLE.get_ptr_mut(child_pid) {
                            unsafe {
                                *p = child_state;
                            }
                        }
                        cpu.num_processes += 1;
                        if let Some(p) = SCHED_STATE.get_ptr_mut(1) {
                            unsafe {
                                *p = cpu.num_processes as u32;
                            }
                        }
                        // Suspend parent: set bit in SCHED_STATE[2] (suspended_mask)
                        let suspended = match SCHED_STATE.get(2) {
                            Some(v) => *v,
                            None => 0,
                        };
                        if let Some(p) = SCHED_STATE.get_ptr_mut(2) {
                            unsafe {
                                *p = suspended | (1 << parent_pid);
                            }
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
                        let src_base =
                            blk::RAMDISK_BASE_WORD + block_num * blk::WORDS_PER_BLOCK + progress;
                        let dst_base = (buf_addr >> 2) + progress;
                        let mut w: u32 = 0;
                        while w < 16 {
                            let val = match RAM_MAP.get(src_base + w) {
                                Some(v) => *v,
                                None => 0,
                            };
                            if let Some(ptr) = RAM_MAP.get_ptr_mut(dst_base + w) {
                                unsafe {
                                    *ptr = val;
                                }
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
                        let dst_base =
                            blk::RAMDISK_BASE_WORD + block_num * blk::WORDS_PER_BLOCK + progress;
                        let src_base = (buf_addr >> 2) + progress;
                        let mut w: u32 = 0;
                        while w < 16 {
                            let val = match RAM_MAP.get(src_base + w) {
                                Some(v) => *v,
                                None => 0,
                            };
                            if let Some(ptr) = RAM_MAP.get_ptr_mut(dst_base + w) {
                                unsafe {
                                    *ptr = val;
                                }
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
                                    let dst_byte_off =
                                        (buf_addr.wrapping_add(read_count) & 3) as u32;
                                    let existing = mem_read_word(dst_word);
                                    let mask = !(0xFFu32 << (dst_byte_off * 8));
                                    let new_val =
                                        (existing & mask) | ((ch as u32) << (dst_byte_off * 8));
                                    mem_write_word(dst_word, new_val);
                                    read_count += 1;
                                }
                                // Consume the event (clear slot).
                                if let Some(p) = KBD_MAP.get_ptr_mut(slot) {
                                    unsafe {
                                        *p = 0;
                                    }
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
                        unsafe {
                            *p = 0;
                        } // clear all suspended bits
                    }

                    increment_stat(STAT_SYSCALLS);

                // ── Trivial FUZIX syscall stubs (Level 5c) ───────────────
                // UID/GID family: always root (0)
                } else if syscall_nr == lsys::SYS_GETUID
                    || syscall_nr == lsys::SYS_GETGID
                    || syscall_nr == lsys::SYS_GETEUID
                    || syscall_nr == lsys::SYS_GETEGID
                    || syscall_nr == lsys::SYS_SETUID
                    || syscall_nr == lsys::SYS_SETGID
                {
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
                    mem_write_word(buf_word, 0o100644); // st_mode (regular file)
                    mem_write_word(buf_word + 1, 4096); // st_size
                    mem_write_word(buf_word + 2, 512); // st_blksize
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
                    mem_write_word(pipefd_addr >> 2, 10); // read end
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
            // NOTE: the Doom syscalls DRAW_FRAME(1)/GET_KEY(2)/GET_TICKS(3)/SLEEP(4)
            // collide numerically with xv6 fork(1)/exit(2)/wait(3). They are
            // mutually-exclusive workloads, so under `ascend-linux` the Doom
            // numbers are disabled and 1/2/3 route to the xv6 handlers below.
            if !cfg!(feature = "ascend-linux") && syscall_nr == sys::SYS_DRAW_FRAME {
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
            } else if !cfg!(feature = "ascend-linux") && syscall_nr == sys::SYS_GET_KEY {
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
            } else if !cfg!(feature = "ascend-linux") && syscall_nr == sys::SYS_GET_TICKS {
                // DG_GetTicksMs: r8 (a0) = milliseconds since boot.
                cpu.regs[8] = (now / 1_000_000) as u32;
            } else if !cfg!(feature = "ascend-linux") && syscall_nr == sys::SYS_SLEEP {
                // DG_SleepMs: sleep for r8 (a0) milliseconds.
                let ms = cpu.regs[8] as u64;
                cpu.sleep_until_ns = now + ms * 1_000_000;
                increment_stat(STAT_SLEEPING);
                break;
            } else if syscall_nr == lsys::SYS_READ_BLOCK {
                // RV32I ecall convention: a7 (→ r1) = syscall_nr,
                //                          a0 (→ r8) = block_num,
                //                          a1 (→ r9) = dest_byte_addr.
                // 128-word block copied 16 words/tick (verifier-friendly);
                // r10 (= a2 = third "arg" slot) tracks progress.
                // Phase 1.4 fsinit unblock 2026-05-14.
                let block_num = cpu.regs[8];
                let buf_addr = cpu.regs[9];
                let progress = cpu.regs[10];
                if block_num < blk::TOTAL_BLOCKS {
                    let src_base =
                        blk::RAMDISK_BASE_WORD + block_num * blk::WORDS_PER_BLOCK + progress;
                    let dst_base = (buf_addr >> 2) + progress;
                    let mut w: u32 = 0;
                    while w < 16 {
                        let val = match RAM_MAP.get(src_base + w) {
                            Some(v) => *v,
                            None => 0,
                        };
                        if let Some(ptr) = RAM_MAP.get_ptr_mut(dst_base + w) {
                            unsafe {
                                *ptr = val;
                            }
                        }
                        w += 1;
                    }
                    let next_progress = progress + 16;
                    if next_progress >= 128 {
                        cpu.regs[8] = blk::BLOCK_SIZE;
                        cpu.regs[10] = 0;
                        increment_stat(STAT_BLOCK_OPS);
                    } else {
                        cpu.regs[10] = next_progress;
                        cpu.pc = cpu.pc.wrapping_sub(1);
                        break;
                    }
                } else {
                    cpu.regs[8] = (-(lsys::EIO as i32)) as u32;
                }
            } else if syscall_nr == 16 {
                // xv6 SYS_write (16) — 1-byte-per-call (printf's putc does
                // write(fd, &c, 1)). Larger writes blow the kernel-6.17 BPF
                // verifier budget. Phase 1.6 unblock: the trampoline_mbc.S
                // sp-load fix gave us a real RAM_MAP-backed buf_addr.
                // Gate 2: buf_addr is a USER virtual address — resolve it through
                // the calling process's page table (walkaddr equivalent) before
                // the physical read. Identity for pid 0; offset slice for children.
                let buf_addr = cpu.regs[9];
                let byte_val = mem_read_byte(translate_address(cpu, buf_addr));
                mem_write_byte(0xC001, byte_val);
                cpu.regs[8] = 1;
            } else if syscall_nr == 1 {
                // ── xv6 SYS_fork (1) — RV32I ecall path (Phase 1.6) ──────────
                // Real fork: snapshot the parent's full MBC register file into
                // the next PROC_TABLE slot so the scheduler (driven later by
                // SYS_wait → scheduler_context_switch) can run the child. This
                // mirrors the proven INT 0x80 SYS_FORK (Level 4c) but uses the
                // RISC-V ABI: a0 (→ r8) carries the return value. Parent gets
                // child_pid; the child resumes from the saved state with a0 = 0
                // and takes init.c's `if (pid == 0)` exec branch.
                //
                // cpu.pc was already advanced past the ecall at fetch (see the
                // `cpu.pc.wrapping_add(1)` before opcode dispatch), so storing
                // child_state[16] = cpu.pc resumes the child *after* the syscall.
                // Phase 1.2 slot recycling: allocate the lowest FREE pid rather
                // than a monotonic high-water counter. A slot is free if it was
                // never used (pid >= num_processes) OR it is fully collected —
                // its child exited (halted) AND the parent reaped it (reaped).
                // Without this, init's `fork→exec-fail→exit→wait` loop burns one
                // permanent slot per iteration and wedges at MAX after 7 forks.
                // The scan reads only masks (not mutated until finalize), so it
                // returns the SAME pid on every copy-tick re-entry — stable.
                let halted_m = match SCHED_STATE.get(3) {
                    Some(v) => *v,
                    None => 0,
                };
                let reaped_m = match SCHED_STATE.get(4) {
                    Some(v) => *v,
                    None => 0,
                };
                let nprocs = cpu.num_processes as u32;
                let mut child_pid = intr::MAX_PROCESSES as u32; // sentinel: none free
                let mut cand = 1u32;
                while cand < intr::MAX_PROCESSES as u32 {
                    let used_live = cand < nprocs
                        && !(((halted_m >> cand) & 1) != 0 && ((reaped_m >> cand) & 1) != 0);
                    if !used_live {
                        child_pid = cand;
                        break;
                    }
                    cand += 1;
                }
                // Gate 4 capacity guard (ADR-074 scoped N-way, 2026-05-31): a
                // child's 8 MiB slice must fit under RAM_ISOLATION_CEILING (56
                // MiB) inside the 64 MiB RAM_MAP. A forkbomb that exhausts the
                // physical budget bails (-1) here — BEFORE any pgd/leaf is built
                // — so we never map an out-of-bounds or overlapping slice.
                let slice_top =
                    phase12::pid_phys_offset(child_pid as u8).wrapping_add(phase12::SLICE_STRIDE);
                if child_pid >= intr::MAX_PROCESSES as u32
                    || slice_top > phase12::RAM_ISOLATION_CEILING
                {
                    // No free slot, OR the next slice would exceed the ceiling —
                    // xv6 fork() returns -1 (graceful physical exhaustion).
                    cpu.regs[8] = (-1i32) as u32;
                } else {
                    let child_pgd = phase12::pgd_base_for_pid(child_pid as u8);
                    let child_off = phase12::pid_phys_offset(child_pid as u8);
                    // ── Gate 2: give the child its own address space (ADR-074) ──
                    // First entry for this pid (child pgd pde[0] not yet present):
                    // build a per-pid page directory mapping VA[0,8MiB) onto the
                    // child's disjoint physical slice via two 4 MiB superpage
                    // leaves, and reset the copy cursor (a2/regs[10] — caller-saved,
                    // fork has no args so it's free scratch across the re-entries).
                    let pde0 = mem_read_word(child_pgd >> 2);
                    if (pde0 & mmu::PTE_PRESENT) == 0 {
                        let flags =
                            mmu::PTE_PRESENT | mmu::PTE_LEAF | mmu::PTE_WRITE | mmu::PTE_USER;
                        mem_write_word(child_pgd >> 2, (child_off & mmu::SUPERPAGE_MASK) | flags);
                        mem_write_word(
                            (child_pgd >> 2) + 1,
                            ((child_off + 0x0040_0000) & mmu::SUPERPAGE_MASK) | flags,
                        );
                        cpu.regs[10] = 0; // copy cursor (words copied so far)
                    }
                    // Copy the parent's WORKING SET into the child's slice, a
                    // chunk per tick (re-enters via pc-=1 until done). The child
                    // is not yet in PROC_TABLE, so the scheduler can't switch to
                    // it mid-copy. Parent slice is identity for pid 0 (src_off=0).
                    // Two bands skip the multi-MiB zero gap between the low image
                    // and the high stack (see phase12::FORK_COPY_* for the model):
                    // a linear cursor [0,total_copy) maps to the low band first,
                    // then the stack window around the (page-aligned) SP.
                    let cursor = cpu.regs[10];
                    let total = phase12::USER_SLICE_BYTES >> 2;
                    let low_words = phase12::FORK_COPY_LOW_BYTES >> 2;
                    let stack_words = phase12::FORK_COPY_STACK_BYTES >> 2;
                    let total_copy = low_words + stack_words;
                    let stack_base = (cpu.regs[15] & !0xFFFu32) >> 2; // page-aligned SP, words
                    let src_off = phase12::pid_phys_offset(cpu.current_pid) >> 2;
                    let dst_off = child_off >> 2;
                    let mut w = 0u32;
                    while w < 16 {
                        let c = cursor + w;
                        if c < total_copy {
                            let slice_word = if c < low_words {
                                c
                            } else {
                                stack_base + (c - low_words)
                            };
                            if slice_word < total {
                                let val = match RAM_MAP.get(src_off + slice_word) {
                                    Some(v) => *v,
                                    None => 0,
                                };
                                if let Some(p) = RAM_MAP.get_ptr_mut(dst_off + slice_word) {
                                    unsafe {
                                        *p = val;
                                    }
                                }
                            }
                        }
                        w += 1;
                    }
                    let next = cursor + 16;
                    if next < total_copy {
                        cpu.regs[10] = next;
                        cpu.pc = cpu.pc.wrapping_sub(1); // re-enter to continue copy
                        break;
                    }
                    // Copy complete — finalize the fork.
                    let mut child_state = [0u32; 22];
                    let mut r = 0u32;
                    while r < 16 {
                        child_state[r as usize] = cpu.regs[r as usize];
                        r += 1;
                    }
                    child_state[16] = cpu.pc; // child resumes after the ecall
                    child_state[17] = cpu.flags as u32;
                    child_state[18] = cpu.regs[15];
                    child_state[19] = cpu.program_break;
                    // Child runs out of its own pgd (per-pid slice).
                    child_state[phase12::PROC_TABLE_PGD_SLOT] = child_pgd;
                    // Phase 1.7 Gate B (ADR-077): the child inherits the parent's
                    // RV2MBC base — it runs the same image until it exec's. init
                    // (base 0) → its children resolve into init's region until
                    // exec("sh") swaps the base. Compile-time 0 in Doom.
                    child_state[phase12::PROC_TABLE_RV2MBC_SLOT] = rv2mbc_base_of(cpu);
                    child_state[8] = 0; // child's fork() returns 0
                    child_state[10] = 0; // clear copy-cursor scratch
                    if let Some(p) = PROC_TABLE.get_ptr_mut(child_pid) {
                        unsafe {
                            *p = child_state;
                        }
                    }
                    // Phase 1.7 Gate A: the child inherits the parent's open
                    // files (xv6 fork() dups every fd). Copy the parent's
                    // FD_TABLE row into the child's so init's console fds 0/1/2
                    // survive into the forked child — and thence into any exec'd
                    // sh, which writes its prompt to fd 2 and reads from fd 0.
                    // Bounded by NOFILE for the verifier.
                    let mut ifd = 0u32;
                    while ifd < fdtable::NOFILE {
                        let k = fd_kind(cpu.current_pid, ifd);
                        fd_set_kind(child_pid as u8, ifd, k);
                        ifd += 1;
                    }
                    // Revive the (possibly reused) slot: clear its halted/reaped
                    // bits so the scheduler runs it and a later wait() can reap it
                    // again. Extend the high-water only if this pid is new.
                    if let Some(p) = SCHED_STATE.get_ptr_mut(3) {
                        unsafe {
                            *p &= !(1u32 << child_pid);
                        }
                    }
                    if let Some(p) = SCHED_STATE.get_ptr_mut(4) {
                        unsafe {
                            *p &= !(1u32 << child_pid);
                        }
                    }
                    if child_pid + 1 > cpu.num_processes as u32 {
                        cpu.num_processes = (child_pid + 1) as u8;
                    }
                    if let Some(p) = SCHED_STATE.get_ptr_mut(1) {
                        unsafe {
                            *p = cpu.num_processes as u32;
                        }
                    }
                    // Stage 3: record parentage so wait() reaps only direct
                    // children (refreshes the slot on pid reuse — M2).
                    if let Some(p) = PARENT_OF.get_ptr_mut(child_pid) {
                        unsafe {
                            *p = cpu.current_pid as u32;
                        }
                    }
                    increment_stat(STAT_FORKS);
                    // Parent's fork() return value: a0 = child_pid.
                    cpu.regs[8] = child_pid;
                }
            } else if syscall_nr == 2 {
                // ── xv6 SYS_exit (2) — RV32I ecall path (Phase 1.6) ──────────
                // Mark the exiting process a ZOMBIE (set its halted_mask bit) so
                // its parent's wait() can reap it, then context-switch to a
                // runnable process. Only halt the whole CPU when nothing else
                // can run (the last process exited). Mirrors INT 0x80 SYS_EXIT
                // (Phase 1.3 ADR-075 D-5). exit code in a0 (→ r8).
                cpu.exit_code = cpu.regs[8];
                let exiting_pid = cpu.current_pid as u32;
                // Stage 3 / M1: reparent this pid's children to init (pid 0)
                // BEFORE marking it a zombie. With parent-filtered wait(), a
                // child whose parent died first would otherwise be reapable by
                // NO ONE → slot leak → fork-fails-forever (the phase16 bug
                // class). One bounded ≤8 scan.
                let mut q = 0u32;
                while q < intr::MAX_PROCESSES as u32 {
                    if let Some(pp) = PARENT_OF.get_ptr_mut(q) {
                        unsafe {
                            if *pp == exiting_pid {
                                *pp = 0;
                            }
                        }
                    }
                    q += 1;
                }
                if let Some(p) = SCHED_STATE.get_ptr_mut(3) {
                    unsafe {
                        *p |= 1u32 << exiting_pid;
                    }
                }
                if cpu.num_processes > 1 {
                    scheduler_context_switch(cpu, flow_label, hop_id);
                    // If the scheduler couldn't move off the exiting pid (all
                    // others are zombies/suspended), there's nothing left to
                    // run — halt.
                    if cpu.current_pid as u32 == exiting_pid {
                        cpu.halted = 1;
                        increment_stat(STAT_HALTED);
                    }
                    break;
                } else {
                    cpu.halted = 1;
                    increment_stat(STAT_HALTED);
                    break;
                }
            } else if syscall_nr == 3 {
                // ── xv6 SYS_wait (3) — RV32I ecall path (Phase 1.6) ──────────
                // wait(int *status): block until ANY child exits, return its
                // pid; -1 if the caller has no children. a0 (→ r8) is BOTH the
                // status pointer (0 = ignore) and the return value.
                //
                // Cooperative model: children are pids greater than the parent
                // (init=0 forks 1,2,…). If a not-yet-reaped zombie child exists,
                // reap it (mark reaped_mask, keep halted_mask so the scheduler
                // never reschedules it) and return its pid. Otherwise switch to
                // a runnable child and re-execute this ecall when we resume —
                // cpu.pc is decremented BEFORE the switch so the *parent's*
                // saved PC lands on the wait ecall (pc was pre-incremented at
                // fetch), while the child resumes at its own saved PC.
                let parent_pid = cpu.current_pid as u32;
                let num = cpu.num_processes as u32;
                let halted = match SCHED_STATE.get(3) {
                    Some(v) => *v,
                    None => 0,
                };
                let reaped = match SCHED_STATE.get(4) {
                    Some(v) => *v,
                    None => 0,
                };
                let mut found: i32 = -1;
                let mut any_child = false;
                // Stage 3: scan ALL pids and reap only DIRECT children
                // (PARENT_OF[p] == me), skipping self — so init never reaps sh's
                // child (grandchild-reap bug) and never reaps itself.
                let mut p = 0u32;
                // Bounded by MAX_PROCESSES for the BPF verifier.
                while p < num && p < intr::MAX_PROCESSES as u32 {
                    let parent_of_p = match PARENT_OF.get(p) {
                        Some(v) => *v,
                        None => 0,
                    };
                    if p != parent_pid && parent_of_p == parent_pid {
                        any_child = true;
                        if (halted >> p) & 1 != 0 && (reaped >> p) & 1 == 0 {
                            found = p as i32;
                            break;
                        }
                    }
                    p += 1;
                }
                if found >= 0 {
                    let child = found as u32;
                    // Reap: record in reaped_mask so a later wait() skips it.
                    if let Some(ptr) = SCHED_STATE.get_ptr_mut(4) {
                        unsafe {
                            *ptr = reaped | (1u32 << child);
                        }
                    }
                    // Phase 1.2 slot recycling: free the reaped child's address
                    // space by clearing pde[0] PRESENT. A future fork() that
                    // reuses this pid then sees an absent pgd and re-runs its
                    // first-entry path (rebuild superpages + reset copy cursor).
                    let freed_pgd = phase12::pgd_base_for_pid(child as u8);
                    let freed_pde0 = mem_read_word(freed_pgd >> 2);
                    mem_write_word(freed_pgd >> 2, freed_pde0 & !mmu::PTE_PRESENT);
                    // If a status pointer was supplied, write 0 (we don't track
                    // per-process exit status yet).
                    let status_ptr = cpu.regs[8];
                    if status_ptr != 0 {
                        // Gate 2: status_ptr is a user VA — translate to physical.
                        mem_write_word(translate_address(cpu, status_ptr) >> 2, 0);
                    }
                    increment_stat(STAT_WAITPIDS);
                    cpu.regs[8] = child; // wait() returns the reaped child's pid
                } else if any_child {
                    // Children exist but none have exited yet — run one.
                    cpu.pc = cpu.pc.wrapping_sub(1); // re-execute wait on resume
                    scheduler_context_switch(cpu, flow_label, hop_id);
                    break;
                } else {
                    // No children — xv6 wait() returns -1.
                    cpu.regs[8] = (-1i32) as u32;
                }
            } else if syscall_nr == 11 {
                // ── xv6 SYS_getpid (11) — RV32I ecall path (Gate 4) ──────────
                // Return the running pid (a0) so a userland process can
                // self-verify it is executing AS the pid whose per-pid physical
                // slice it expects — part of proving "per-pid" isolation, not
                // just "per-fork". 0 = init; children are 1.. . Not handled in
                // the original Phase 1.6 path (init/sh/gate2 don't call it).
                cpu.regs[8] = cpu.current_pid as u32;
            } else if syscall_nr == 13 {
                // ── xv6 SYS_pause (13) → cooperative YIELD (Gate 4, 2026-05-31) ─
                // ascend-linux has NO timer preemption (the round-robin timer
                // switch near the top of this program is cfg-gated off for xv6
                // builds), so a live process can only relinquish the CPU at
                // exit/wait. Proving per-pid isolation of CONCURRENT live
                // processes (gate_nway.c) requires children to pause-and-resume
                // while staying live — exactly what a cooperative yield gives.
                // We reuse SYS_pause (sleep's defining act IS yielding the CPU;
                // the duration arg is ignored in the cooperative model) instead
                // of minting a new syscall number — zero userland plumbing.
                // Phase-3 TODO: split a dedicated SYS_yield from
                // SYS_pause(sleep-with-duration) once real timer preemption lands.
                //
                // pc is already advanced past the ecall (fetch pre-increment) and
                // — unlike SYS_wait — we do NOT decrement it: the yielder resumes
                // AFTER pause() returns, not by re-running it. Set the return
                // value (0) BEFORE the switch so it is saved into the yielder's
                // PROC_TABLE slot; after the switch `cpu` holds the NEXT
                // process's registers. break only if a switch actually happened
                // (matches exit/wait); if the caller is the only runnable
                // process, scheduler_context_switch is a no-op and pause()
                // simply returns 0.
                cpu.regs[8] = 0;
                let prev_pid = cpu.current_pid;
                if cpu.num_processes > 1 {
                    scheduler_context_switch(cpu, flow_label, hop_id);
                }
                if cpu.current_pid != prev_pid {
                    break;
                }
            } else if syscall_nr == 15 {
                // ── xv6 SYS_open (15) — Gate D real inode resolve (Phase 3) ──
                // open(path=a0, flags=a1). `open("console")` keeps the pre-FS
                // console-device fd (init's stdio: open→0, dup×2→1,2). Any other
                // path is resolved against the root directory's dirents (inum 1)
                // → an FD_INODE descriptor carrying {inum, 0}, or -1 on a miss /
                // a bad superblock (H-FS3). All `fs.img` address arithmetic goes
                // through the off-target-tested `monad_common::fs_walk`.
                //
                // Verifier budget: the dirent scan is RE-ENTRANT ≤8 entries/tick
                // (a2=regs[10] is the cursor, a3=regs[11] an OPEN_MAGIC marker so
                // the cursor is zeroed once at entry). a0/a1 are never written
                // until the syscall returns, so the per-tick basename re-resolve
                // is stable. No `translate_address()` — direct phys throughout.
                let pid = cpu.current_pid;
                let path_va = cpu.regs[8];
                let path_phys = phase12::pid_phys_offset(pid).wrapping_add(path_va);
                // Basename phys byte addr: strip leading '/' and "./" (≤8 bounded).
                // Strip a single leading "./" or '/' (covers ls/stat's "./name"
                // and an absolute "/name"). A fixed two-way test, not a break-loop:
                // the loop's early exit added verifier state for no real benefit —
                // our paths never nest deeper than one separator.
                let mut base = path_phys;
                let c0 = ram_byte(base);
                if c0 == b'.' && ram_byte(base.wrapping_add(1)) == b'/' {
                    base = base.wrapping_add(2);
                } else if c0 == b'/' {
                    base = base.wrapping_add(1);
                }
                if name_is(base, b"console") {
                    // Console device is implicit (not in fs.img) — keep FD_CONSOLE.
                    let fd = fd_alloc(pid);
                    if fd >= fdtable::NOFILE {
                        cpu.regs[8] = (-1i32) as u32; // EMFILE
                    } else {
                        fd_set_kind(pid, fd, fdtable::FD_CONSOLE);
                        cpu.regs[8] = fd;
                    }
                } else {
                    // Superblock @ block 1 (8 words); H-FS3 gates every open.
                    let mut sb = [0u32; 8];
                    let sb_ok = match fs_walk::word_of_block(fs_walk::SUPERBLOCK_BLOCK) {
                        Some(w0) => {
                            let mut i = 0u32;
                            while i < 8 {
                                sb[i as usize] = ram_word(w0 + i);
                                i += 1;
                            }
                            fs_walk::superblock_valid(&sb)
                        }
                        None => false,
                    };
                    if !sb_ok {
                        cpu.regs[8] = (-1i32) as u32; // no valid FS → all opens fail
                    } else {
                        let ninodes = fs_walk::superblock_ninodes(&sb);
                        let inodestart = fs_walk::superblock_inodestart(&sb);
                        // Root dinode (inum 1). Read its fields DIRECTLY from fs.img
                        // (a BPF map) rather than decoding into a stack struct: the
                        // dirent scan indexes addrs by a runtime `fbn`, and a
                        // variable-offset read from the *stack* is rejected by the
                        // verifier — a variable index into a map is allowed.
                        let iblock = fs_walk::inode_block(fs_walk::ROOTINO, inodestart);
                        match fs_walk::word_of_block(iblock) {
                            None => cpu.regs[8] = (-1i32) as u32, // inode block oob (H-FS1)
                            Some(ib_word) => {
                                let dino_base =
                                    ib_word + fs_walk::dinode_word_offset(fs_walk::ROOTINO);
                                let rtype = (ram_word(dino_base) & 0xFFFF) as u16;
                                let rsize = ram_word(dino_base + 2);
                                if rtype != fs_walk::T_DIR {
                                    // Root inode must be a directory to walk dirents.
                                    cpu.regs[8] = (-1i32) as u32;
                                } else {
                                    // Clamp the entry count to a constant ceiling: a
                                    // hostile dinode.size can't drive an unbounded
                                    // scan (bounds total re-entries).
                                    const OPEN_MAX_DIRENTS: u32 = 256;
                                    const OPEN_MAGIC: u32 = 0x09E0_0DE0;
                                    let ndir_raw = rsize / fs_walk::DIRENT_BYTES;
                                    let ndir = if ndir_raw > OPEN_MAX_DIRENTS {
                                        OPEN_MAX_DIRENTS
                                    } else {
                                        ndir_raw
                                    };
                                    if cpu.regs[11] != OPEN_MAGIC {
                                        cpu.regs[10] = 0;
                                        cpu.regs[11] = OPEN_MAGIC;
                                    }
                                    let idx = cpu.regs[10];
                                    let mut found: u32 = 0; // 0 = none (never a real inum)
                                                            // ONE dirent per tick. No outer loop: re-entry
                                                            // (pc-=1) advances the cursor, so the only loop
                                                            // the verifier explores here is name_eq_phys's
                                                            // 14-iter compare. exec(7) already sits near the
                                                            // shared 1M ceiling; >1 dirent/tick tipped the
                                                            // whole program over (BPF program too large).
                                    if idx < ndir {
                                        let fbn = idx / fs_walk::DIRENTS_PER_BLOCK;
                                        if (fbn as usize) < fs_walk::NDIRECT {
                                            // addrs[fbn] = dinode word 3+fbn, read from
                                            // fs.img (map index, not stack).
                                            let dblk = ram_word(dino_base + 3 + fbn);
                                            if dblk != 0 {
                                                if let Some(dbw) = fs_walk::word_of_block(dblk) {
                                                    let within = idx % fs_walk::DIRENTS_PER_BLOCK;
                                                    let dword = dbw + within * 4; // 4 w/dirent
                                                    let inum = ram_word(dword) & 0xFFFF;
                                                    if inum != 0
                                                        && inum < ninodes
                                                        && name_eq_phys(dword * 4 + 2, base)
                                                    {
                                                        found = inum;
                                                    }
                                                }
                                            }
                                        }
                                    }
                                    let next = idx + 1;
                                    if found != 0 {
                                        // Retire the scan: a stale OPEN_MAGIC left in
                                        // a3 would make the NEXT open seed its cursor
                                        // from whatever the user last put in a2 (e.g.
                                        // read's n=16), silently skipping dirents.
                                        cpu.regs[11] = 0;
                                        let fd = fd_alloc(pid);
                                        if fd >= fdtable::NOFILE {
                                            cpu.regs[8] = (-1i32) as u32;
                                        } else {
                                            fd_set_inode(pid, fd, found); // FD_INODE + [inum,0]
                                            cpu.regs[8] = fd;
                                        }
                                    } else if next < ndir {
                                        // More dirents remain — re-enter next tick.
                                        cpu.regs[10] = next;
                                        cpu.pc = cpu.pc.wrapping_sub(1);
                                        break;
                                    } else {
                                        cpu.regs[11] = 0; // retire the scan (see above)
                                        cpu.regs[8] = (-1i32) as u32; // exhausted, no match
                                    }
                                }
                            }
                        }
                    }
                }
            } else if cfg!(feature = "ascend-linux") && syscall_nr == 8 {
                // ── xv6 SYS_fstat (8) — Gate D dinode stat (Phase 4) ─────────
                // fstat(fd=a0, st=a1): fill `struct stat` for an FD_INODE fd
                // from its dinode so `ls` can classify + size each entry.
                // THE ILP32E TRAP: xv6 types.h does `typedef unsigned long
                // uint64` — 4 BYTES under -mabi=ilp32e — so `struct stat` =
                // { int dev; uint ino; short type; short nlink; uint64 size }
                // is 16 bytes with size at offset 12, NOT the RV64 24-byte /
                // offset-16 layout. Writing 24 bytes smashed the 8 bytes after
                // the caller's st — ls's buf[0..3] — blanking every path `ls`
                // built (the Gate D "blank names + size 0" bug). No loops here
                // — straight reads + 4 word writes — so the verifier cost
                // stays tiny on top of the open walk.
                let pid = cpu.current_pid;
                let fd = cpu.regs[8];
                let st_va = cpu.regs[9];
                let mut done = false;
                if fd_kind(pid, fd) == fdtable::FD_INODE {
                    let (inum, _off) = fd_inode(pid, fd);
                    if let Some(sw) = fs_walk::word_of_block(fs_walk::SUPERBLOCK_BLOCK) {
                        let magic = ram_word(sw);
                        let ninodes = ram_word(sw + 3);
                        let inodestart = ram_word(sw + 6);
                        if magic == fs_walk::FSMAGIC && fs_walk::inum_valid(inum, ninodes) {
                            let iblock = fs_walk::inode_block(inum, inodestart);
                            if let Some(ib) = fs_walk::word_of_block(iblock) {
                                let db = ib + fs_walk::dinode_word_offset(inum);
                                let type_ = ram_word(db) & 0xFFFF;
                                let nlink = ram_word(db + 1) >> 16;
                                let size = ram_word(db + 2);
                                // H-FS6: copy-out only through the per-pid bound.
                                if let Some(phys) = user_phys(pid, st_va, 16) {
                                    let w = phys >> 2;
                                    ram_write_word(w, 1); // dev
                                    ram_write_word(w + 1, inum); // ino
                                    ram_write_word(w + 2, type_ | (nlink << 16)); // type|nlink
                                    ram_write_word(w + 3, size); // size (ulong = 4 B on ilp32e)
                                    cpu.regs[8] = 0;
                                    done = true;
                                }
                            }
                        }
                    }
                }
                if !done {
                    cpu.regs[8] = (-1i32) as u32;
                }
            } else if syscall_nr == 17 {
                // ── xv6 SYS_mknod (17) — RV32I ecall path (Phase 1.7 Gate A) ──
                // mknod(path, major, minor): register a device node. The console
                // device is implicit in this model (no device-number table at
                // L5), so this is a success-returning near-noop that only has to
                // satisfy init's `if(open("console")<0){ mknod("console",…); … }`
                // fallback. Real device nodes await the FS reader.
                cpu.regs[8] = 0;
            } else if syscall_nr == 10 {
                // ── xv6 SYS_dup (10) — RV32I ecall path (Phase 1.7 Gate A) ──
                // dup(oldfd=a0): copy oldfd's kind onto the lowest-free fd. init
                // does dup(0); dup(0) to put the console on stdout(1)/stderr(2).
                // -1 if oldfd isn't open or the table is full. oldfd itself is in
                // use, so fd_alloc never returns it.
                let pid = cpu.current_pid;
                let oldfd = cpu.regs[8];
                let kind = fd_kind(pid, oldfd);
                if kind == fdtable::FD_FREE {
                    cpu.regs[8] = (-1i32) as u32; // EBADF: oldfd not open
                } else {
                    let newfd = fd_alloc(pid);
                    if newfd >= fdtable::NOFILE {
                        cpu.regs[8] = (-1i32) as u32;
                    } else {
                        fd_set_kind(pid, newfd, kind);
                        cpu.regs[8] = newfd;
                    }
                }
            } else if syscall_nr == 21 {
                // ── xv6 SYS_close (21) — RV32I ecall path (Phase 1.7 Gate A) ──
                // close(fd=a0): free the descriptor; -1 if it wasn't open. No
                // refcount at L5 — each fd is an independent kind tag, so closing
                // one dup'd console fd doesn't disturb the others.
                let pid = cpu.current_pid;
                let fd = cpu.regs[8];
                if fd_kind(pid, fd) == fdtable::FD_FREE {
                    cpu.regs[8] = (-1i32) as u32;
                } else {
                    fd_set_kind(pid, fd, fdtable::FD_FREE);
                    // Gate D: drop any inode binding so a recycled fd can't read
                    // a stale {inum, offset} (harmless no-op for console fds).
                    fd_clear_inode(pid, fd);
                    cpu.regs[8] = 0;
                }
            } else if syscall_nr == 5 {
                // ── xv6 SYS_read (5) — RV32I ecall path (Gate C console input) ──
                // read(fd=a0, buf=a1, n=a2). sh's gets() always asks for n=1, so
                // we deliver exactly ONE byte per call from the KBD ring (host
                // pre-fills it via upc-bootctl --input). Verifier-friendly shape
                // (the deferral note from Gate A): NO per-byte translate loop —
                // one O(1) ring pop + one bounded user_phys() + one mem_write_byte.
                let pid = cpu.current_pid;
                let fd = cpu.regs[8];
                let kind = fd_kind(pid, fd);
                if kind == fdtable::FD_INODE {
                    // ── Gate D file read (Phase 5): copy ≤READ_CHUNK bytes from
                    // the inode's data at the stored offset into the user buffer,
                    // advance the offset, return the count (0 at EOF). Capped per
                    // call (a SHORT read) so NO re-entrancy is needed — ls reads
                    // 16 B exact, cat tolerates short reads — keeping verifier
                    // state well under the budget-tight 1M ceiling.
                    const READ_CHUNK: u32 = 16; // bytes per read() call (1 dirent / short read)
                    let n = cpu.regs[10]; // a2 = requested count
                    let buf_va = cpu.regs[9]; // a1
                    let (inum, offset) = fd_inode(pid, fd);
                    let mut ret = (-1i32) as u32;
                    if let Some(sw) = fs_walk::word_of_block(fs_walk::SUPERBLOCK_BLOCK) {
                        let magic = ram_word(sw);
                        let ninodes = ram_word(sw + 3);
                        let inodestart = ram_word(sw + 6);
                        if magic == fs_walk::FSMAGIC && fs_walk::inum_valid(inum, ninodes) {
                            let iblock = fs_walk::inode_block(inum, inodestart);
                            if let Some(ib) = fs_walk::word_of_block(iblock) {
                                let db = ib + fs_walk::dinode_word_offset(inum);
                                let size = ram_word(db + 2);
                                if offset >= size {
                                    ret = 0; // EOF
                                } else {
                                    // H-FS7: clamp to n, to one chunk, and to the
                                    // current data block's remaining bytes; no wrap.
                                    let avail = size - offset;
                                    let mut cap = if n < avail { n } else { avail };
                                    if cap > READ_CHUNK {
                                        cap = READ_CHUNK;
                                    }
                                    let within = offset % fs_walk::BSIZE;
                                    let block_left = fs_walk::BSIZE - within;
                                    if cap > block_left {
                                        cap = block_left;
                                    }
                                    let fbn = offset / fs_walk::BSIZE;
                                    if (fbn as usize) < fs_walk::NDIRECT {
                                        let dblk = ram_word(db + 3 + fbn);
                                        if dblk != 0 {
                                            if let Some(dbw) = fs_walk::word_of_block(dblk) {
                                                let src_byte = dbw * 4 + within;
                                                // H-FS6: copy-out via the per-pid bound.
                                                // Bound the dest to the full chunk; our
                                                // inode readers (ls=16, cat=512) always
                                                // pass buffers ≥ READ_CHUNK.
                                                if let Some(dst) =
                                                    user_phys(pid, buf_va, READ_CHUNK)
                                                {
                                                    // Fixed READ_CHUNK-byte copy with NO
                                                    // inner guard — the guarded form
                                                    // doubled verifier states and blew
                                                    // the 1M ceiling. The caller honors
                                                    // the returned `cap`, so tail bytes
                                                    // copied past it are ignored; every
                                                    // src byte stays inside fs.img.
                                                    let mut c = 0u32;
                                                    while c < READ_CHUNK {
                                                        ram_write_byte(
                                                            dst + c,
                                                            ram_byte(src_byte + c),
                                                        );
                                                        c += 1;
                                                    }
                                                    fd_set_offset(pid, fd, offset + cap);
                                                    ret = cap;
                                                }
                                            }
                                        }
                                    }
                                    // fbn ≥ NDIRECT (file > 12 KiB single-indirect)
                                    // is not needed for the Gate-D corpus.
                                }
                            }
                        }
                    }
                    cpu.regs[8] = ret;
                } else if kind == fdtable::FD_CONSOLE {
                    let tail = match KBD_TAIL.get(0) {
                        Some(v) => *v,
                        None => 0,
                    };
                    let head = match KBD_HEAD.get(0) {
                        Some(v) => *v,
                        None => 0,
                    };
                    if tail < head {
                        // One queued byte. Decode (scancode<<1)|pressed → ASCII.
                        let raw = match KBD_MAP.get(tail & 7) {
                            Some(v) => *v,
                            None => 0,
                        };
                        let ch = ((raw >> 1) & 0xFF) as u8;
                        let buf_va = cpu.regs[9];
                        match user_phys(pid, buf_va, 1) {
                            Some(phys) => {
                                mem_write_byte(phys, ch);
                                if let Some(p) = KBD_TAIL.get_ptr_mut(0) {
                                    unsafe {
                                        *p = tail.wrapping_add(1);
                                    }
                                }
                                increment_stat(STAT_TTY_READS);
                                cpu.regs[8] = 1; // 1 byte delivered
                            }
                            None => {
                                // H1: buffer VA escapes the pid slice → EFAULT,
                                // do not advance the ring or write memory.
                                cpu.regs[8] = (-1i32) as u32;
                            }
                        }
                    } else {
                        // Empty ring: BLOCK (Gate C Stage 1). A console is never
                        // truly at EOF — rewind PC to the ecall and re-poll next
                        // tick rather than let sh's gets() treat 0 as EOF + exit.
                        cpu.pc = cpu.pc.wrapping_sub(1);
                        break;
                    }
                } else {
                    cpu.regs[8] = (-1i32) as u32; // not a readable fd
                }
            } else if cfg!(feature = "ascend-linux") && syscall_nr == 7 {
                // ── xv6 SYS_exec (7) — Phase 1.7 Gate B headline (ADR-077) ──
                // exec(path=a0, argv=a1): replace this pid's image with a
                // resident program from PROGRAM_TABLE. Resolve the path
                // basename → slot via FNV-1a (matching the host loader's
                // fnv1a), copy the program's host-staged .data/.rodata into
                // this pid's physical slice, adopt its per-process rv2mbc_base
                // + entry PC, then reset the user context. On a miss → -1 so
                // init still prints `exec sh failed` rather than crashing.
                //
                // Verifier shape (per the read(5) note above): translate the
                // path base ONCE, then read consecutive bytes; the .data copy
                // is a single bounded ≤512-word loop (sh's data ≈ 252 words),
                // NOT a per-byte translate loop.
                let path_va = cpu.regs[8];
                // Resolve the path's physical address WITHOUT a Sv32 walk: the
                // per-pid pgd maps user VA[0,8MiB) onto the slice linearly via
                // two 4 MiB superpages, so phys = slice_base + va. This is the
                // verifier-budget-critical shortcut — a full translate_address()
                // here pushes the (already near-1M) program past the complexity
                // ceiling, exactly the Gate A read(5) failure mode.
                let path_phys = phase12::pid_phys_offset(cpu.current_pid).wrapping_add(path_va);
                let mut h: u32 = 0x811c_9dc5;
                let mut j = 0u32;
                while j < 16 {
                    let b = mem_read_byte(path_phys + j);
                    if b == 0 {
                        break;
                    }
                    if b == b'/' {
                        h = 0x811c_9dc5; // restart at each separator → basename
                    } else {
                        h ^= b as u32;
                        h = h.wrapping_mul(0x0100_0193);
                    }
                    j += 1;
                }
                // Find a populated PROGRAM_TABLE slot whose basename matches.
                let mut slot = pt::SLOTS;
                let mut s = 0u32;
                while s < pt::SLOTS {
                    if let Some(row) = PROGRAM_TABLE.get(s) {
                        if row[pt::VALID] == 1 && row[pt::NAME_HASH] == h {
                            slot = s;
                            break;
                        }
                    }
                    s += 1;
                }
                if slot >= pt::SLOTS {
                    cpu.regs[8] = (-1i32) as u32; // ENOENT → `exec sh failed`
                } else {
                    let row = match PROGRAM_TABLE.get(slot) {
                        Some(r) => *r,
                        None => [0u32; 8],
                    };
                    let entry_mbc = row[pt::ENTRY_MBC];
                    let new_base = row[pt::RV2MBC_BASE];
                    let dst_va = row[pt::DATA_DST_VA];
                    let src_va = row[pt::DATA_SRC_VA];
                    let len_words = row[pt::DATA_LEN_WORDS];
                    // Copy the host-staged flat .data/.rodata blob into THIS
                    // pid's slice at the program's linked VA — fork-copy left
                    // init's image here; exec overwrites it. RE-ENTRANT 16
                    // words/tick (the proven fork-copy shape): a single 512-word
                    // tick blew the verifier's 1M complexity ceiling, so the
                    // copy spans ticks via pc-=1, bounding the loop to 16.
                    //
                    // a2 (regs[10]) is the word cursor; a3 (regs[11]) is an
                    // exec-in-progress marker so the cursor is zeroed exactly
                    // once at entry (fork uses pde0-presence for the same job).
                    // path/argv (a0/a1) are untouched across re-entries, so the
                    // per-tick re-resolve above is stable.
                    const EXEC_MAGIC: u32 = 0xE0EC_B10C;
                    if cpu.regs[11] != EXEC_MAGIC {
                        cpu.regs[10] = 0;
                        cpu.regs[11] = EXEC_MAGIC;
                    }
                    let cursor = cpu.regs[10];
                    let src_word = (src_va >> 2).wrapping_add(cursor);
                    let dst_word = (phase12::pid_phys_offset(cpu.current_pid).wrapping_add(dst_va)
                        >> 2)
                        .wrapping_add(cursor);
                    let mut w = 0u32;
                    while w < 16 {
                        if cursor + w < len_words {
                            let val = match RAM_MAP.get(src_word + w) {
                                Some(v) => *v,
                                None => 0,
                            };
                            if let Some(p) = RAM_MAP.get_ptr_mut(dst_word + w) {
                                unsafe {
                                    *p = val;
                                }
                            }
                        }
                        w += 1;
                    }
                    let next = cursor + 16;
                    if next < len_words {
                        cpu.regs[10] = next;
                        cpu.pc = cpu.pc.wrapping_sub(1); // re-enter to continue
                        break;
                    }
                    // Copy complete — adopt the new image. priv stays U (3); the
                    // per-process RV2MBC base routes the program's indirect
                    // branches to its disjoint map region; PC jumps to its entry.
                    set_rv2mbc_base(cpu, new_base);
                    let mut r = 0u32;
                    while r < 16 {
                        cpu.regs[r as usize] = 0;
                        r += 1;
                    }
                    cpu.regs[15] = 0x0050_0000; // fresh user stack top (< 8 MiB)
                    cpu.pc = entry_mbc;
                    cpu.flags = 0;
                    cpu.program_break = DEFAULT_PROGRAM_BREAK;
                    cpu.reservation_address = 0xFFFF_FFFF;
                }
            } else if syscall_nr == lsys::SYS_WRITE_BLOCK {
                let block_num = cpu.regs[8];
                let buf_addr = cpu.regs[9];
                let progress = cpu.regs[10];
                if block_num < blk::TOTAL_BLOCKS {
                    let src_base = (buf_addr >> 2) + progress;
                    let dst_base =
                        blk::RAMDISK_BASE_WORD + block_num * blk::WORDS_PER_BLOCK + progress;
                    let mut w: u32 = 0;
                    while w < 16 {
                        let val = match RAM_MAP.get(src_base + w) {
                            Some(v) => *v,
                            None => 0,
                        };
                        if let Some(ptr) = RAM_MAP.get_ptr_mut(dst_base + w) {
                            unsafe {
                                *ptr = val;
                            }
                        }
                        w += 1;
                    }
                    let next_progress = progress + 16;
                    if next_progress >= 128 {
                        cpu.regs[8] = blk::BLOCK_SIZE;
                        cpu.regs[10] = 0;
                        increment_stat(STAT_BLOCK_OPS);
                    } else {
                        cpu.regs[10] = next_progress;
                        cpu.pc = cpu.pc.wrapping_sub(1);
                        break;
                    }
                } else {
                    cpu.regs[8] = (-(lsys::EIO as i32)) as u32;
                }
            }
            // Unknown syscall: silently ignore (fail-safe).
        } else if opc == op::HALT {
            halt_diag(
                cpu.pc,
                rv2mbc_base_of(cpu),
                cpu.current_pid as u32,
                prev_pc,
                0x80,
            );
            cpu.halted = 1;
            increment_stat(STAT_HALTED);
            // SP diagnostic: record r15 and PC at halt time
            let sp_key: u32 = 24;
            if let Some(ptr) = STATS.get_ptr_mut(&sp_key) {
                // Pack: high32 = r15 (SP), low32 = PC at halt
                unsafe {
                    *ptr = ((cpu.regs[15] as u64) << 32) | (cpu.pc as u64);
                }
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

/// Emit a TTY_WRITE event to the ring buffer (Level 4f console I/O).
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
/// All loops bounded to MAX_PROCESSES (8 since Phase 1.3 ADR-075 D-1) for BPF verifier.
#[inline(always)]
fn scheduler_context_switch(cpu: &mut MbcCpuState, flow_label: u32, hop_id: u8) {
    let old_pid = cpu.current_pid as u32;
    let num = cpu.num_processes as u32;

    // Guard: need at least 2 processes
    if num <= 1 {
        return;
    }

    // 1. Save current process state to PROC_TABLE[current_pid]
    let mut save_state = [0u32; 22];
    let mut r = 0u32;
    while r < 16 {
        save_state[r as usize] = cpu.regs[r as usize];
        r += 1;
    }
    save_state[16] = cpu.pc;
    save_state[17] = cpu.flags as u32;
    save_state[18] = cpu.regs[15]; // SP copy
    save_state[19] = cpu.program_break;
    // Phase 1.2 (ADR-074 Option A + Allocator A1): save current pgd phys addr.
    save_state[phase12::PROC_TABLE_PGD_SLOT] = cpu.page_dir_base;
    // Phase 1.7 Gate B (ADR-077): save the per-process RV2MBC base so a
    // descheduled program keeps resolving indirect branches into its own
    // region. `rv2mbc_base_of` is a compile-time 0 in the non-ascend build.
    save_state[phase12::PROC_TABLE_RV2MBC_SLOT] = rv2mbc_base_of(cpu);

    if let Some(p) = PROC_TABLE.get_ptr_mut(old_pid) {
        unsafe {
            *p = save_state;
        }
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
    // Bounded search: try up to MAX_PROCESSES (8) candidates per Phase 1.3 ADR-075 D-1.
    let mut attempts = 0u32;
    while attempts < 8 {
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
    // Phase 1.3 ADR-075 §Security #3: invalidate the LR.W reservation on every
    // context switch. RISC-V spec says reservations are cleared by exceptions
    // and interrupts; SYS_SCHED_YIELD (the syscall trap) qualifies. Without
    // this, a process could LR.W → yield → another process SC.W to the same
    // address → original process's SC.W incorrectly succeeds.
    cpu.reservation_address = 0xFFFF_FFFF;
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
    // Phase 1.2 (ADR-074 Option A): load incoming process's pgd phys addr.
    cpu.page_dir_base = load_state[phase12::PROC_TABLE_PGD_SLOT];
    // Phase 1.7 Gate B (ADR-077): restore the incoming process's RV2MBC base.
    set_rv2mbc_base(cpu, load_state[phase12::PROC_TABLE_RV2MBC_SLOT]);

    // Gate 2 (ADR-074 Option A): flush the TLB on every pgd switch. The 64-entry
    // direct-mapped TLB is keyed on vpn alone (no ASID), so without this a stale
    // entry from the outgoing process would resolve the incoming process's
    // identical VA to the WRONG physical slice — a cross-process leak. Invalidate
    // all 64 entries (sentinel vpn, present bit cleared). Bounded loop, ~+0.5%
    // verifier budget per ADR-074 §Issue 3.
    let mut t = 0u32;
    while t < 64 {
        if let Some(p) = TLB_MAP.get_ptr_mut(t) {
            unsafe {
                (*p)[0] = 0xFFFF_FFFF;
                (*p)[2] = 0;
            }
        }
        t += 1;
    }

    // Phase 1.2 (ADR-074 Decision 2026-05-12): invoke the chosen-option
    // context-switch hook. With `--features phase12-option-a` this calls
    // `phase12_option_a_on_context_switch`; default build calls the no-op.
    // Hook receives the *new* pid and may further mutate cpu (e.g. ASID
    // for Option B). The save+load above already established the basic
    // Option-A state; the hook is the extension point.
    #[cfg(feature = "phase12-option-a")]
    phase12::phase12_option_a_on_context_switch(cpu, next_pid as u8);
    #[cfg(feature = "phase12-option-b")]
    phase12::phase12_option_b_on_context_switch(cpu, next_pid as u8);
    #[cfg(feature = "phase12-option-c")]
    phase12::phase12_option_c_on_context_switch(cpu, next_pid as u8);
    #[cfg(not(any(
        feature = "phase12-option-a",
        feature = "phase12-option-b",
        feature = "phase12-option-c"
    )))]
    phase12::phase12_noop_on_context_switch(cpu, next_pid as u8);

    // 4. Update current_pid
    cpu.current_pid = next_pid as u8;

    // Update SCHED_STATE[0] = new current_pid
    if let Some(p) = SCHED_STATE.get_ptr_mut(0) {
        unsafe {
            *p = next_pid;
        }
    }

    increment_stat(STAT_CONTEXT_SWITCHES);
    emit_context_switch(flow_label, old_pid, next_pid, hop_id);
}

/// Emit a CONTEXT_SWITCH event to the ring buffer (Level 4c scheduler).
#[inline(always)]
fn emit_context_switch(flow_label: u32, from_pid: u32, to_pid: u32, hop_id: u8) {
    if let Some(mut entry) = COMPUTE_EVENTS.reserve::<ComputeHopEvent>(0) {
        let event = ComputeHopEvent {
            timestamp_ns: unsafe { bpf_ktime_get_ns() },
            event_type: EVENT_CONTEXT_SWITCH,
            hop_id,
            _pad: [0; 2],
            flow_label,
            pc: from_pid,        // reuse pc field for source pid
            instruction: to_pid, // reuse instruction field for dest pid
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
fn translate_address(cpu: &MbcCpuState, vaddr: u32) -> u32 {
    // Translate only for user mode (priv_level==3). The kernel runs at flat
    // physical addresses (ROM code + RAM data; kvminit is skipped on UPC), so
    // M/S-mode accesses bypass the walk. This lets us leave mmu_enabled=1
    // kingdom-wide while only user pages get the per-pid page table (ADR-074
    // Option A, 2026-05-30).
    if cpu.mmu_enabled == 0 || cpu.priv_level != 3 {
        return vaddr; // Flat mode — no translation (kernel, or Doom back-compat)
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

    // 4 MiB superpage leaf (ADR-074 large-page path): the PDE maps a whole
    // 4 MiB region directly — no second-level table. Resolve to physical and
    // install a 4 KiB-granular TLB entry so repeat hits skip the re-walk.
    if (pde & mmu::PTE_LEAF) != 0 {
        let paddr = (pde & mmu::SUPERPAGE_MASK) | (vaddr & mmu::SUPERPAGE_OFFSET_MASK);
        if let Some(p) = TLB_MAP.get_ptr_mut(tlb_idx) {
            unsafe {
                (*p)[0] = vpn;
                (*p)[1] = paddr >> 12;
                (*p)[2] = mmu::PTE_PRESENT;
            }
        }
        return paddr;
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

    // MMIO TTY intercept (UPC L4f convention, byte addr 0xC000-0xC003).
    // start_mbc.c::mmio_putc writes via word-store; the low byte of `value`
    // is the character. Push it to the TTY ring so userspace bridges see it.
    // Phase 1.1 SHIP enabler 2026-05-10.
    if word_addr == 0x3000 {
        tty_push_byte(value as u8);
    }

    // Write to RAM_MAP only.
    if let Some(ptr) = RAM_MAP.get_ptr_mut(word_addr) {
        unsafe {
            *ptr = value;
        }
    }
}

/// Append a single byte to the TTY ring buffer (4 KB circular).
/// Called from mem_write_word/mem_write_byte on writes to MMIO 0xC000-0xC003.
#[inline(always)]
fn tty_push_byte(b: u8) {
    let head = match TTY_HEAD.get(0) {
        Some(v) => *v,
        None => 0,
    };
    let head_bounded = head & 0xFFF; // 0..4095, helps verifier prove the index bound
    if let Some(slot) = TTY_MAP.get_ptr_mut(head_bounded) {
        unsafe {
            *slot = b;
        }
    }
    let new_head = (head_bounded + 1) & 0xFFF;
    if let Some(h) = TTY_HEAD.get_ptr_mut(0) {
        unsafe {
            *h = new_head;
        }
    }
}

/// Gate D: raw word read straight from `RAM_MAP` — NO MMIO/screen/KBD range
/// checks (those branches are pure verifier-complexity tax in the fs.img walk,
/// which never touches device memory). Use only for fs.img / user-slice RAM.
#[inline(always)]
fn ram_word(word_addr: u32) -> u32 {
    match RAM_MAP.get(word_addr) {
        Some(v) => *v,
        None => 0,
    }
}

/// Gate D: raw byte read via [`ram_word`] (one map lookup + shift, no MMIO
/// branches) — keeps the hot name-compare loop cheap for the BPF verifier.
#[inline(always)]
fn ram_byte(byte_addr: u32) -> u8 {
    let w = ram_word(byte_addr >> 2);
    ((w >> ((byte_addr & 3) * 8)) & 0xFF) as u8
}

/// Gate D: raw word write straight to `RAM_MAP`, skipping `mem_write_word`'s TTY
/// MMIO intercept. Used to fill a user `struct stat` buffer (plain RAM in the
/// pid slice, never device memory). Caller bounds the dest via [`user_phys`].
#[inline(always)]
fn ram_write_word(word_addr: u32, value: u32) {
    if let Some(p) = RAM_MAP.get_ptr_mut(word_addr) {
        unsafe {
            *p = value;
        }
    }
}

/// Gate D: raw byte write (read-modify-write of the enclosing word) straight to
/// `RAM_MAP`, skipping MMIO intercepts. Used by `read(5)` to copy file bytes
/// into a user buffer. Caller bounds the dest via [`user_phys`].
#[inline(always)]
fn ram_write_byte(byte_addr: u32, value: u8) {
    let shift = (byte_addr & 3) * 8;
    if let Some(p) = RAM_MAP.get_ptr_mut(byte_addr >> 2) {
        unsafe {
            *p = (*p & !(0xFFu32 << shift)) | ((value as u32) << shift);
        }
    }
}

/// Gate D: true if the NUL-or-`/`-terminated path at phys byte `base` equals the
/// literal `lit` (e.g. `b"console"`). Bounded to `lit.len()+1` byte reads.
#[inline(always)]
fn name_is(base: u32, lit: &[u8]) -> bool {
    let mut i = 0usize;
    while i < lit.len() {
        if ram_byte(base.wrapping_add(i as u32)) != lit[i] {
            return false;
        }
        i += 1;
    }
    let n = ram_byte(base.wrapping_add(lit.len() as u32));
    n == 0 || n == b'/'
}

/// Gate D: `strncmp(name, basename, DIRSIZ)` over phys memory. `dname` is the
/// phys byte address of a NUL-padded 14-byte dirent name; `base` is the
/// phys byte address of the NUL-or-`/`-terminated path basename. **H-FS8**:
/// reads at most [`fs_walk::DIRSIZ`] bytes of the dirent field, never past it.
#[inline(always)]
fn name_eq_phys(dname: u32, base: u32) -> bool {
    // Early-return form: a `return` lets the verifier PRUNE the rest of this
    // path immediately (cheaper than a flag-accumulating full 14-iter scan, which
    // forces the verifier to explore every iteration). H-FS8: ≤ DIRSIZ bytes.
    let mut k = 0u32;
    while k < fs_walk::DIRSIZ as u32 {
        let dn = ram_byte(dname.wrapping_add(k));
        let praw = ram_byte(base.wrapping_add(k));
        // Path basename terminates at NUL or '/'; treat both as the 0 pad the
        // dirent name uses for short names so strncmp semantics line up.
        let pn = if praw == b'/' { 0 } else { praw };
        if dn != pn {
            return false;
        }
        if pn == 0 {
            return true; // both end here
        }
        k += 1;
    }
    // Consumed all DIRSIZ bytes with no mismatch: equal iff the basename also
    // ends exactly at DIRSIZ (no 15th significant byte).
    let nxt = ram_byte(base.wrapping_add(fs_walk::DIRSIZ as u32));
    nxt == 0 || nxt == b'/'
}

/// Read a single byte from the MBC address space.
#[allow(dead_code)]
#[inline(always)]
fn mem_read_byte(byte_addr: u32) -> u8 {
    // Screen region: direct SCREEN_MAP read.
    if (mmap::SCREEN_BASE..mmap::SCREEN_BASE + mmap::SCREEN_SIZE).contains(&byte_addr) {
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
    // MMIO TTY intercept (UPC L4f convention, byte addr 0xC000-0xC003).
    // Some compilers emit STB for char writes — handle the byte-store
    // path too. Phase 1.1 SHIP enabler 2026-05-10.
    if byte_addr >= 0xC000 && byte_addr <= 0xC003 {
        tty_push_byte(value);
    }
    // Screen region: direct SCREEN_MAP write.
    if (mmap::SCREEN_BASE..mmap::SCREEN_BASE + mmap::SCREEN_SIZE).contains(&byte_addr) {
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

/// Set a STATS slot to an explicit value (insert-or-update). `STATS` is a
/// HashMap, so `get_ptr_mut(&k)` returns None for a never-written key — a plain
/// store would silently no-op (the slot-25 capture footgun, ~/tmp/next.md). This
/// mirrors `increment_stat`'s insert path so diagnostic slots always land.
#[inline(always)]
fn set_stat(key: u32, val: u64) {
    if let Some(v) = STATS.get_ptr_mut(&key) {
        unsafe {
            *v = val;
        }
    } else {
        let _ = STATS.insert(&key, &val, 0);
    }
}

/// Capture the halt context into diag slots 25-29 (printed by upc-bootctl as
/// `ROM-FAULT CTX:` + `halt_reason`). Non-inline so every halt site shares ONE
/// verified copy — cheap on the verifier's 1M-insn budget. `reason` identifies
/// which `cpu.halted = 1` site fired (Gate C post-`$` halt triage). `target` is
/// the most diagnostic address for the site (fault PC, or the untranslated
/// mepc/sepc for the indirect-branch arms).
fn halt_diag(target: u32, rv2mbc_base: u32, pid: u32, prev_pc: u32, reason: u32) {
    set_stat(25, target as u64);
    set_stat(26, rv2mbc_base as u64);
    set_stat(27, pid as u64);
    set_stat(28, prev_pc as u64);
    set_stat(29, reason as u64);
}

// ── Panic handler ─────────────────────────────────────────────────────────────

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
