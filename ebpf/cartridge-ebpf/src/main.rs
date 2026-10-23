// SPDX-License-Identifier: GPL-3.0-or-later
//! cartridge-ebpf — the MBC cartridge interpreter (ADR-108, S5 Phase 11).
//!
//! Two XDP programs:
//! - `cart_xdp`: per packet. Parses headers into the ADR-108 FIELD table
//!   (metadata only, never payload), then runs every active XDP-stage slot
//!   for at most `N_PKT` instructions of `monad_common::cart::step` against
//!   that flow's `CartState`, and returns the most restrictive verdict.
//! - `cart_tick`: driven by the host through BPF_PROG_TEST_RUN (no live
//!   packets). Pops one `ScoreJob` (or resumes the yielded one) and runs the
//!   tick-stage slot for at most `N_TICK` instructions.
//!
//! The interpreter is the same `no_std` source the host reference runs
//! (`crates/upc-cartridge`), so a run replays bit-for-bit. This object is
//! separate from monad-cpu-ebpf and carries its own verifier budget
//! (ADR-108 D-C1); Doom and xv6 never load it.
//!
//! Verifier rules followed here (the UPC's hard-won list):
//! 1. Every loop is bounded by a `const` (`N_PKT`, `N_TICK`, `SLOTS`, 64).
//! 2. One variable offset into a map value per loop body: the step reads
//!    `CartState` through one pointer; ROM/DATA go through map lookups with
//!    masked indices, never through a second variable offset into a value.
//! 3. No address-translation-style helper inside the loop.
//! 4. Every map index is masked or range-checked first.
//! 5. No zero-init of padded structs (no implicit padding exists in any ABI
//!    record, and records are written field by field) — no memset/memcpy.
//! 6. Debug code is const-gated (there is none).
//! 7. Faults fail open: any fault is XDP_PASS unless the slot is fail_closed.
#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::xdp_action,
    helpers::bpf_ktime_get_ns,
    macros::{map, xdp},
    maps::{Array, LruHashMap, PerCpuArray, Queue, RingBuf},
    programs::XdpContext,
};
use monad_common::cart::{
    begin_job, begin_run, cfgkey, field, iat_bucket, run_one, run_out, stat, status, verdict,
    CartCfg, CartData, CartEvent, CartIo, CartRom, CartState, FlowKey, FlowMeta, ScoreJob, SlotCfg,
    Step, BREAKER_LIMIT, FEAT_WORDS, FIELD_COUNT, MEM_WORDS, META_SLOT, N_PKT, N_TICK, SLOTS,
    STAGE_TICK, STAGE_XDP,
};

const SLOT_WORDS: u32 = 1024;
const FLOWS: u32 = 65_536;

// ── Maps ────────────────────────────────────────────────────────────────────

/// ROM, 1024 words per slot.
#[map]
static CART_ROM: Array<u32> = Array::with_max_entries(SLOTS as u32 * SLOT_WORDS, 0);
/// Read-only DATA, 1024 words per slot.
#[map]
static CART_DATA: Array<u32> = Array::with_max_entries(SLOTS as u32 * SLOT_WORDS, 0);
/// Slot configuration; the loader writes `gen` last.
#[map]
static CART_SLOT: Array<SlotCfg> = Array::with_max_entries(SLOTS as u32, 0);
/// Per-flow, per-slot machine state.
#[map]
static FLOW_STATE: LruHashMap<FlowKey, CartState> = LruHashMap::with_max_entries(FLOWS, 0);
/// Per-flow metadata (FIELD 7–9), keyed with `slot = META_SLOT`.
#[map]
static FLOW_META: LruHashMap<FlowKey, FlowMeta> = LruHashMap::with_max_entries(FLOWS, 0);
/// Flows queued for the tick stage.
#[map]
static SCORE_QUEUE: Queue<ScoreJob> = Queue::with_max_entries(16_384, 0);
/// The tick job in progress (index 0).
#[map]
static TICK_JOB: Array<TickJob> = Array::with_max_entries(1, 0);
/// EMIT records.
#[map]
static CART_EVENTS: RingBuf = RingBuf::with_byte_size(256 * 1024, 0);
/// Counters (`monad_common::cart::stat`).
#[map]
static CART_STATS: PerCpuArray<u64> = PerCpuArray::with_max_entries(stat::COUNT, 0);
/// Runtime knobs (`monad_common::cart::cfgkey`).
#[map]
static CART_CFG: Array<u64> = Array::with_max_entries(cfgkey::COUNT, 0);
/// Fault breaker windows, one per slot.
#[map]
static BREAKER: Array<Breaker> = Array::with_max_entries(SLOTS as u32, 0);
/// An all-zero CartState: the insert source for a new flow. Never written.
#[map]
static ZERO_STATE: Array<CartState> = Array::with_max_entries(1, 0);

/// Tick job in progress (TICK_JOB value, 424 bytes, no implicit padding).
#[repr(C)]
pub struct TickJob {
    /// 1 while a job is running or yielded.
    pub active: u32,
    /// Slot running it.
    pub slot: u32,
    /// Slot generation at job start.
    pub gen: u32,
    /// Zero.
    pub rsv: u32,
    /// The job.
    pub job: ScoreJob,
    /// Its machine state.
    pub state: CartState,
}

/// Fault breaker window.
#[repr(C)]
pub struct Breaker {
    /// Window start.
    pub start_ns: u64,
    /// Faults in the window.
    pub count: u32,
    /// Zero.
    pub rsv: u32,
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// Zeroing and copying records word by word with volatile accesses: a plain
// `Default` / `*src` lowers to memset/memcpy calls, which --btf turns into
// global functions the verifier rejects ("Global function memset() doesn't
// return scalar"). Every ABI record is a multiple of 4 bytes.

/// Zero `*p` in place. Callers then use `&mut *p`: moving the value out of
/// a MaybeUninit lowers to a memset/memcpy of the whole record again.
#[inline(always)]
fn zero_at<T>(p: *mut T) {
    let w = p as *mut u32;
    let n = core::mem::size_of::<T>() / 4;
    let mut i = 0;
    while i < n {
        unsafe { core::ptr::write_volatile(w.add(i), 0) };
        i += 1;
    }
}

#[inline(always)]
fn copy_into<T>(dst: &mut T, src: &T) {
    let d = dst as *mut T as *mut u32;
    let s = src as *const T as *const u32;
    let n = core::mem::size_of::<T>() / 4;
    let mut i = 0;
    while i < n {
        unsafe { core::ptr::write_volatile(d.add(i), core::ptr::read_volatile(s.add(i))) };
        i += 1;
    }
}

#[inline(always)]
fn bump(idx: u32, by: u64) {
    if let Some(v) = CART_STATS.get_ptr_mut(idx) {
        unsafe { *v += by };
    }
}

#[inline(always)]
fn now_ns() -> u64 {
    match CART_CFG.get(cfgkey::CLOCK_NS) {
        Some(&fixed) if fixed != 0 => fixed,
        _ => unsafe { bpf_ktime_get_ns() },
    }
}

struct SlotRom {
    base: u32,
}
impl CartRom for SlotRom {
    #[inline(always)]
    fn word(&self, pc: u32) -> Option<u32> {
        CART_ROM.get(self.base + (pc & (SLOT_WORDS - 1))).copied()
    }
}

struct SlotData {
    base: u32,
}
impl CartData for SlotData {
    #[inline(always)]
    fn word(&self, idx: u32) -> Option<u32> {
        CART_DATA.get(self.base + (idx & (SLOT_WORDS - 1))).copied()
    }
}

struct Io<'a> {
    /// None in the tick stage (FIELD is an XDP-stage syscall).
    fields: Option<&'a [u32; FIELD_COUNT as usize]>,
    now: u64,
    flow: &'a FlowKey,
    slot: u8,
    stage: u8,
}

impl CartIo for Io<'_> {
    #[inline(always)]
    fn field(&mut self, id: u32) -> Option<u32> {
        match self.fields {
            Some(f) if id < FIELD_COUNT => Some(f[id as usize]),
            _ => None,
        }
    }
    #[inline(always)]
    fn now_ns_lo(&mut self) -> u32 {
        self.now as u32
    }
    #[inline(always)]
    fn emit(&mut self, kind: u32, a: u32, b: u32) {
        match CART_EVENTS.reserve::<CartEvent>(0) {
            Some(mut e) => {
                let p = e.as_mut_ptr();
                unsafe {
                    (*p).ts_ns = self.now;
                    copy_into(&mut (*p).flow, self.flow);
                    (*p).kind = kind;
                    (*p).a = a;
                    (*p).b = b;
                    (*p).slot = self.slot;
                    (*p).stage = self.stage;
                    (*p).rsv = 0;
                }
                e.submit(0);
                bump(self.slot as u32 * 8 + stat::EMITS, 1);
            }
            None => bump(stat::EVENTS_DROPPED, 1),
        }
    }
    #[inline(always)]
    fn queue(&mut self, feat: &[u32; FEAT_WORDS]) {
        let mut job_m = core::mem::MaybeUninit::<ScoreJob>::uninit();
        zero_at(job_m.as_mut_ptr());
        let job = unsafe { &mut *job_m.as_mut_ptr() };
        copy_into(&mut job.flow, self.flow);
        copy_into(&mut job.feat, feat);
        job.src_slot = self.slot;
        job.enq_ns_lo = self.now as u32;
        job.flow.slot = self.slot;
        if SCORE_QUEUE.push(job, 0).is_ok() {
            bump(self.slot as u32 * 8 + stat::QUEUE_OK, 1);
        } else {
            bump(self.slot as u32 * 8 + stat::QUEUE_DROPS, 1);
        }
    }
}

/// Count a fault against the slot's breaker; disable the slot at the limit.
#[inline(always)]
fn fault(slot: u32, now: u64) {
    bump(slot * 8 + stat::FAULTS, 1);
    if let Some(b) = BREAKER.get_ptr_mut(slot) {
        unsafe {
            if now.wrapping_sub((*b).start_ns) > 1_000_000_000 {
                (*b).start_ns = now;
                (*b).count = 0;
            }
            (*b).count += 1;
            if (*b).count >= BREAKER_LIMIT {
                if let Some(c) = CART_SLOT.get_ptr_mut(slot) {
                    if (*c).active != 0 {
                        (*c).active = 0;
                        bump(stat::BREAKER_TRIPS, 1);
                    }
                }
                (*b).count = 0;
            }
        }
    }
}

// ── The interpreter loop: bpf_loop over the shared step ─────────────────────
//
// An inlined `for _ in 0..N` around the step function does not converge in
// the verifier: every opcode path reaches the loop head with a different
// register/stack state, so states do not prune and 64 iterations x 4 slots
// exhausts the 1M budget (measured in S5 Phase 11). bpf_loop runs the step as
// a callback the verifier checks once per call site, whatever N is.
// `monad_common::cart::run` on the host runs the same `run_one` / `run_out`
// pair in a plain loop, so both engines share the loop semantics too.

/// The callback's context, on the caller's stack.
struct LoopCtx<'a> {
    st: *mut CartState,
    cfg: CartCfg,
    rom: SlotRom,
    data: SlotData,
    io: Io<'a>,
    /// `step_code` of how the run stopped; 0 = still running. A plain u32,
    /// not the `Step` enum: storing an enum writes its payload byte even for
    /// unit variants, and that byte is uninitialised (verifier: "!read_ok").
    result: u32,
}

#[inline(always)]
fn step_code(s: Step) -> u32 {
    match s {
        Step::Continue => 0,
        Step::Verdict(v) => 0x100 | v as u32,
        Step::Yield => 0x200,
        Step::Halt => 0x300,
        Step::Fault(k) => 0x400 | k as u32,
    }
}

#[inline(always)]
fn code_step(c: u32) -> Step {
    use monad_common::cart::FaultKind as F;
    match c >> 8 {
        1 => Step::Verdict(c as u8),
        2 => Step::Yield,
        3 => Step::Halt,
        4 => Step::Fault(match c & 0xFF {
            1 => F::BadOpcode,
            2 => F::Mem,
            3 => F::Pc,
            4 => F::Syscall,
            5 => F::Field,
            6 => F::Verdict,
            7 => F::Budget,
            _ => F::LoopBound,
        }),
        _ => Step::Continue,
    }
}

/// Instructions per bpf_loop iteration. The per-iteration callback cost was
/// the largest share of an interpreted instruction (S5 Phase 13 Step 340:
/// 8.1 ns for a NOP with one step per callback; 4.3 ns with 4; 3.7 ns with 8,
/// which doubles the verified callback for little), so each iteration runs up
/// to STEPS_PER_CB steps. N_PKT and N_TICK are multiples of it, and `run_one`
/// enforces `max_insn` itself, so the instruction budget is unchanged.
const STEPS_PER_CB: u32 = 4;
const _: () = assert!(N_PKT as u32 % STEPS_PER_CB == 0 && N_TICK as u32 % STEPS_PER_CB == 0);

#[inline(never)]
extern "C" fn cart_loop_cb(_index: u64, ctx: *mut core::ffi::c_void) -> i64 {
    let c = unsafe { &mut *(ctx as *mut LoopCtx) };
    let st = unsafe { &mut *c.st };
    let mut k = 0;
    while k < STEPS_PER_CB {
        match run_one(st, &c.cfg, &c.rom, &c.data, &mut c.io) {
            Step::Continue => {}
            done => {
                c.result = step_code(done);
                return 1;
            }
        }
        k += 1;
    }
    0
}

/// Run a slot for at most `n` instructions (`N_PKT` or `N_TICK`).
#[inline(always)]
fn drive(st: &mut CartState, cfg: &CartCfg, rom: SlotRom, data: SlotData, io: Io, n: u32) -> Step {
    st.insns = 0;
    let mut c = LoopCtx {
        st: st as *mut CartState,
        cfg: *cfg,
        rom,
        data,
        io,
        result: 0,
    };
    let rc = unsafe {
        aya_ebpf::helpers::gen::bpf_loop(
            n / STEPS_PER_CB,
            cart_loop_cb as *mut core::ffi::c_void,
            &mut c as *mut LoopCtx as *mut core::ffi::c_void,
            0,
        )
    };
    if rc < 0 {
        return Step::Fault(monad_common::cart::FaultKind::Budget);
    }
    match code_step(c.result) {
        Step::Continue => run_out(st, cfg),
        done => done,
    }
}

// ── Packet parsing (metadata only) ──────────────────────────────────────────

#[inline(always)]
fn byte(ctx: &XdpContext, off: usize) -> Option<u8> {
    let p = ctx.data() + off;
    if p + 1 > ctx.data_end() {
        return None;
    }
    Some(unsafe { *(p as *const u8) })
}

#[inline(always)]
fn be16(ctx: &XdpContext, off: usize) -> Option<u16> {
    Some(((byte(ctx, off)? as u16) << 8) | byte(ctx, off + 1)? as u16)
}

/// Fill the FIELD table and the flow key. Returns false for non-IP frames
/// that are too short to be anything (they pass without running slots).
#[inline(always)]
fn parse(ctx: &XdpContext, key: &mut FlowKey, f: &mut [u32; FIELD_COUNT as usize]) -> bool {
    let len = (ctx.data_end() - ctx.data()) as u32;
    f[field::PKT_LEN as usize] = len;
    let ethertype = match be16(ctx, 12) {
        Some(t) => t,
        None => return false,
    };
    let l4: usize;
    let proto: u8;
    if ethertype == 0x0800 {
        let vihl = match byte(ctx, 14) {
            Some(v) => v,
            None => return false,
        };
        let ihl = ((vihl & 0x0F) as usize) * 4;
        if ihl < 20 {
            return false;
        }
        key.l3 = 4;
        f[field::L3_PROTO as usize] = 4;
        f[field::IP_TTL as usize] = byte(ctx, 22).unwrap_or(0) as u32;
        proto = byte(ctx, 23).unwrap_or(0);
        let mut i = 0;
        while i < 4 {
            key.src[i] = byte(ctx, 26 + i).unwrap_or(0);
            key.dst[i] = byte(ctx, 30 + i).unwrap_or(0);
            i += 1;
        }
        l4 = 14 + (ihl & 0x3C);
    } else if ethertype == 0x86DD {
        key.l3 = 6;
        f[field::L3_PROTO as usize] = 6;
        f[field::IP_TTL as usize] = byte(ctx, 21).unwrap_or(0) as u32;
        let mut nh = match byte(ctx, 20) {
            Some(n) => n,
            None => return false,
        };
        let mut i = 0;
        while i < 16 {
            key.src[i] = byte(ctx, 22 + i).unwrap_or(0);
            key.dst[i] = byte(ctx, 38 + i).unwrap_or(0);
            i += 1;
        }
        let mut off = 54usize;
        if nh == 0 {
            // Hop-by-Hop: is its first option the Monad register?
            let hnh = byte(ctx, off).unwrap_or(59);
            let hlen = ((byte(ctx, off + 1).unwrap_or(0) as usize) + 1) * 8;
            if byte(ctx, off + 2) == Some(monad_common::MONAD_OPT_TYPE)
                && byte(ctx, off + 3) == Some(monad_common::MONAD_OPT_DATA_LEN)
            {
                f[field::MONAD_PRESENT as usize] = 1;
                f[field::MONAD_QOS as usize] = byte(ctx, off + 4 + 8).unwrap_or(0) as u32;
                f[field::MONAD_FLAGS as usize] = byte(ctx, off + 4 + 11).unwrap_or(0) as u32;
            }
            nh = hnh;
            off += hlen & 0x7F8;
        }
        proto = nh;
        l4 = off;
    } else {
        return false;
    }
    key.proto = proto;
    f[field::L4_PROTO as usize] = proto as u32;
    if proto == 6 || proto == 17 {
        // The L4 offset is variable (IPv4 IHL, IPv6 HbH length). One bounds
        // check on the very pointer the loads use, the offset laundered and
        // masked so LLVM cannot re-derive it from an unchecked copy.
        let off = core::hint::black_box(l4) & 0x7FF;
        let p = ctx.data() + off;
        let need = if proto == 6 { 14 } else { 4 };
        if p + need <= ctx.data_end() {
            let rd = |k: usize| unsafe { *((p + k) as *const u8) };
            let sp = ((rd(0) as u16) << 8) | rd(1) as u16;
            let dp = ((rd(2) as u16) << 8) | rd(3) as u16;
            key.sport = sp;
            key.dport = dp;
            f[field::SRC_PORT as usize] = sp as u32;
            f[field::DST_PORT as usize] = dp as u32;
            if proto == 6 {
                f[field::TCP_FLAGS as usize] = rd(13) as u32;
            }
        }
    }
    true
}

/// FIELD 7–9 from the flow's metadata entry, updated for this packet.
#[inline(always)]
fn flow_meta(key: &FlowKey, now: u64, len: u32, f: &mut [u32; FIELD_COUNT as usize]) {
    let mut mk_m = core::mem::MaybeUninit::<FlowKey>::uninit();
    zero_at(mk_m.as_mut_ptr());
    let mk = unsafe { &mut *mk_m.as_mut_ptr() };
    copy_into(mk, key);
    mk.slot = META_SLOT;
    match FLOW_META.get_ptr_mut(mk) {
        Some(m) => unsafe {
            f[field::IAT_BUCKET as usize] = iat_bucket(now.wrapping_sub((*m).last_ns));
            (*m).last_ns = now;
            (*m).pkts = (*m).pkts.saturating_add(1);
            (*m).bytes = (*m).bytes.saturating_add(len);
            f[field::FLOW_PKT_COUNT as usize] = (*m).pkts;
            f[field::FLOW_BYTE_COUNT as usize] = (*m).bytes;
        },
        None => {
            let m = FlowMeta {
                last_ns: now,
                pkts: 1,
                bytes: len,
                first_ns: now,
            };
            let _ = FLOW_META.insert(mk, &m, 0);
            f[field::IAT_BUCKET as usize] = 0;
            f[field::FLOW_PKT_COUNT as usize] = 1;
            f[field::FLOW_BYTE_COUNT as usize] = len;
        }
    }
}

/// Look up (or create) the flow's state for `slot`; reset it if the slot was
/// reloaded since (generation mismatch).
#[inline(always)]
fn flow_state(key: &FlowKey, gen: u32) -> Option<*mut CartState> {
    let st = match FLOW_STATE.get_ptr_mut(key) {
        Some(p) => p,
        None => {
            let zero = ZERO_STATE.get(0)?;
            FLOW_STATE.insert(key, zero, 0).ok()?;
            FLOW_STATE.get_ptr_mut(key)?
        }
    };
    unsafe {
        if (*st).gen != gen {
            let mut i = 0;
            while i < MEM_WORDS {
                core::ptr::write_volatile(&mut (*st).mem[i], 0);
                i += 1;
            }
            (*st).gen = gen;
        }
    }
    Some(st)
}

// ── cart_xdp ────────────────────────────────────────────────────────────────

#[xdp]
pub fn cart_xdp(ctx: XdpContext) -> u32 {
    bump(stat::XDP_PACKETS, 1);
    let mut args_m = core::mem::MaybeUninit::<SlotArgs>::uninit();
    zero_at(args_m.as_mut_ptr());
    let args = unsafe { &mut *args_m.as_mut_ptr() };
    let key = &mut args.key;
    let f = &mut args.f;
    if !parse(&ctx, key, f) {
        return xdp_action::XDP_PASS;
    }
    let now = now_ns();
    let len = f[field::PKT_LEN as usize];
    flow_meta(key, now, len, f);
    args.now = now;

    let mut drop = false;
    let mut slot = 0u32;
    while slot < SLOTS as u32 {
        if cart_run_xdp_slot(slot, args as *mut SlotArgs) != 0 {
            drop = true;
        }
        slot += 1;
    }
    if drop {
        xdp_action::XDP_DROP
    } else {
        xdp_action::XDP_PASS
    }
}

/// Per-packet inputs handed to the per-slot global function.
#[repr(C)]
pub struct SlotArgs {
    /// Flow key (its `slot` byte is set per slot).
    pub key: FlowKey,
    /// FIELD table.
    pub f: [u32; FIELD_COUNT as usize],
    /// Zero.
    pub pad: u32,
    /// Clock of the run.
    pub now: u64,
}

/// One slot's run as a GLOBAL BPF function: the verifier checks a global
/// function once, on its own, instead of once per caller state. Without it
/// every distinct header-parse path re-verified all four slots and cart_xdp
/// sat at 498,904 of the 500,000 gate (S5 Phase 11 Step 283, option 4).
#[no_mangle]
#[inline(never)]
// A BPF global function cannot be `unsafe fn` in any way the verifier sees;
// the kernel checks `args` against the BTF type at load and it is
// null-checked below.
#[allow(clippy::not_unsafe_ptr_arg_deref)]
pub extern "C" fn cart_run_xdp_slot(slot: u32, args: *mut SlotArgs) -> u32 {
    if args.is_null() {
        return 0;
    }
    let a = unsafe { &mut *args };
    let now = a.now;
    run_xdp_slot(slot, &mut a.key, &a.f, now) as u32
}

/// Run one XDP-stage slot for this packet. Returns true for DROP.
#[inline(always)]
fn run_xdp_slot(slot: u32, key: &mut FlowKey, f: &[u32; FIELD_COUNT as usize], now: u64) -> bool {
    let cfgp = match CART_SLOT.get(slot) {
        Some(c) => c,
        None => return false,
    };
    if cfgp.active == 0 || cfgp.stage != STAGE_XDP {
        return false;
    }
    let sc = cfgp;
    let cfg = sc.cfg();
    key.slot = slot as u8;
    let stp = match flow_state(key, sc.gen) {
        Some(p) => p,
        None => return false,
    };
    let st = unsafe { &mut *stp };
    let rom = SlotRom {
        base: slot * SLOT_WORDS,
    };
    let data = SlotData {
        base: slot * SLOT_WORDS,
    };
    let io = Io {
        fields: Some(f),
        now,
        flow: key,
        slot: slot as u8,
        stage: STAGE_XDP,
    };
    begin_run(st, sc.entry);
    let r = drive(st, &cfg, rom, data, io, N_PKT as u32);
    let base = slot * 8;
    bump(base + stat::RUNS, 1);
    bump(base + stat::INSNS, st.insns as u64);
    let drop = match r {
        Step::Verdict(v) => v == verdict::DROP,
        Step::Fault(_) => {
            fault(slot, now);
            sc.flags & 1 != 0
        }
        _ => false,
    };
    bump(base + if drop { stat::DROP } else { stat::PASS }, 1);
    drop
}

// ── cart_tick ───────────────────────────────────────────────────────────────

#[xdp]
pub fn cart_tick(_ctx: XdpContext) -> u32 {
    bump(stat::TICK_CALLS, 1);
    let now = now_ns();
    let tj = match TICK_JOB.get_ptr_mut(0) {
        Some(p) => unsafe { &mut *p },
        None => return xdp_action::XDP_PASS,
    };

    // Find the tick-stage slot (the first active one).
    let mut slot = SLOTS as u32;
    let mut s = 0u32;
    while s < SLOTS as u32 {
        if let Some(c) = CART_SLOT.get(s) {
            if slot == SLOTS as u32 && c.active != 0 && c.stage == STAGE_TICK {
                slot = s;
            }
        }
        s += 1;
    }
    if slot >= SLOTS as u32 {
        bump(stat::TICK_IDLE, 1);
        return xdp_action::XDP_PASS;
    }
    let sc = match CART_SLOT.get(slot) {
        Some(c) => c,
        None => return xdp_action::XDP_PASS,
    };

    if tj.active != 0 && (tj.slot != slot || tj.gen != sc.gen) {
        // The slot was reloaded under a yielded job: drop it.
        tj.active = 0;
        bump(stat::TICK_STALE, 1);
    }
    if tj.active == 0 {
        let job = match SCORE_QUEUE.pop() {
            Some(j) => j,
            None => {
                bump(stat::TICK_IDLE, 1);
                return xdp_action::XDP_PASS;
            }
        };
        copy_into(&mut tj.job, &job);
        tj.slot = slot;
        tj.gen = sc.gen;
        tj.rsv = 0;
        tj.active = 1;
        begin_job(&mut tj.state, sc.entry, &tj.job.feat);
    }

    let cfg = sc.cfg();
    let rom = SlotRom {
        base: slot * SLOT_WORDS,
    };
    let data = SlotData {
        base: slot * SLOT_WORDS,
    };
    let io = Io {
        fields: None,
        now,
        flow: &tj.job.flow,
        slot: slot as u8,
        stage: STAGE_TICK,
    };
    let r = drive(&mut tj.state, &cfg, rom, data, io, N_TICK as u32);
    let base = slot * 8;
    bump(base + stat::RUNS, 1);
    bump(base + stat::INSNS, tj.state.insns as u64);
    match r {
        Step::Yield => {}
        Step::Fault(_) => {
            fault(slot, now);
            tj.active = 0;
            bump(stat::TICK_JOBS_DONE, 1);
        }
        _ => {
            tj.state.status = status::DONE;
            tj.active = 0;
            bump(stat::TICK_JOBS_DONE, 1);
        }
    }
    xdp_action::XDP_PASS
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
