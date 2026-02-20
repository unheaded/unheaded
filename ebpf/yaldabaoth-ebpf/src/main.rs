//! Unheaded Protocol — Yaldabaoth eBPF Program
//!
//! **The Demiurge of Chaos** — TC egress classifier that deliberately corrupts,
//! delays, duplicates, or truncates packets to test Kingdom resilience.
//!
//! Yaldabaoth is the chaos engineering subsystem of the Unheaded Protocol.
//! When attached to a TC egress hook, it inspects every outgoing Kingdom packet
//! (those carrying a Hop-by-Hop Monad) and applies the configured chaos mode
//! for flows registered in the CHAOS_TARGETS map.
//!
//! # Chaos Modes (from `monad_common::ChaosMode`)
//!
//! | Mode         | Action                                           | CRC after |
//! |--------------|--------------------------------------------------|-----------|
//! | BIT_FLIP     | XOR a random byte in Monad[1..17] with rand mask | Not recomputed (intentional corruption) |
//! | DELAY        | Mark packet for netem qdisc delay               | Recomputed |
//! | DUPLICATE    | Clone packet via bpf_clone_redirect              | Recomputed |
//! | TRUNCATE     | Zero Monad bytes 0x08..0x13                      | Not recomputed (intentional corruption) |
//! | CHAOS_MARKER | Set CHAOS flag only, no data corruption          | Recomputed |
//!
//! # Design
//!
//! All chaos actions:
//! 1. Set the CHAOS flag in Monad.flags
//! 2. Emit a CHAOS Anamnesis event (always, even for BIT_FLIP)
//! 3. Pass TC_ACT_OK — chaos injection is advisory for the tracing system
//!
//! The CHAOS_TARGETS map is the control plane: userspace writes entries to
//! select which flow labels receive chaos and at what intensity.
//! Key = low 20 bits of IPv6 Flow Label (u32, masked).
//! Value = (mode as u64) | ((param as u64) << 32)
//!   - For DELAY:        param = delay_us (microseconds)
//!   - For BIT_FLIP:     param = 0 (random byte + mask chosen by program)
//!   - For DUPLICATE:    param = 0 (ifindex from CONFIG[4])
//!   - For TRUNCATE:     param = 0
//!   - For CHAOS_MARKER: param = 0
//!
//! # Verifier Notes
//!
//! All packet reads/writes use TcContext.load/store.  Loop bounds are constant.
//! bpf_clone_redirect is only called if CONFIG[4] (egress_ifindex) is nonzero.

#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::TC_ACT_OK,
    helpers::{bpf_get_prandom_u32, bpf_ktime_get_ns},
    macros::{classifier, map},
    maps::{HashMap, RingBuf},
    programs::TcContext,
};
use monad_common::{
    AnamnesisEvent, ChaosMode, EventType, Monad,
    MONAD_OPT_TYPE, MONAD_OPT_DATA_LEN, MONAD_SIZE,
    IPV6_FIXED_HDR_LEN, IPV6_NEXTHDR_HBH,
    flags,
};

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// Anamnesis ring buffer — 8 MiB, shared with trace-collector.
#[map]
static ANAMNESIS: RingBuf = RingBuf::with_byte_size(8 * 1024 * 1024, 0);

/// Chaos target flows.
///
/// Key:   IPv6 Flow Label (u32, bits 19:0 used; masked on insert by userspace).
/// Value: `(mode as u64) | ((param as u64) << 32)`
///
/// Userspace inserts a target to enroll a flow in chaos injection.
/// Userspace removes the target to restore normal forwarding.
#[map]
static CHAOS_TARGETS: HashMap<u32, u64> = HashMap::with_max_entries(4_096, 0);

/// Runtime configuration.
///
/// | Key | Meaning                                     | Default |
/// |-----|---------------------------------------------|---------|
/// | 0   | hop_id                                      | 0       |
/// | 1   | sample_divisor                              | 0 = all |
/// | 4   | egress_ifindex (for DUPLICATE bpf_clone_redirect) | 0 = off |
#[map]
static CONFIG: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

/// Statistics.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

// ── Stat keys ─────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL:   u32 = 0;
const STAT_PACKETS_IPV6:    u32 = 1;
const STAT_PACKETS_HBH:     u32 = 2;
const STAT_TARGETS_HIT:     u32 = 3;
const STAT_MODE_BIT_FLIP:   u32 = 4;
const STAT_MODE_DELAY:      u32 = 5;
const STAT_MODE_DUPLICATE:  u32 = 6;
const STAT_MODE_TRUNCATE:   u32 = 7;
const STAT_MODE_MARKER:     u32 = 8;
const STAT_EVENTS_SENT:     u32 = 9;
const STAT_EVENTS_DROPPED:  u32 = 10;
const STAT_CLONE_FAILED:    u32 = 11;

// ── Config keys ───────────────────────────────────────────────────────────────
const CFG_HOP_ID:         u32 = 0;
const CFG_EGRESS_IFINDEX: u32 = 4;

// ── Packet layout constants ───────────────────────────────────────────────────
const ETH_HLEN:   usize = 14;
const ETH_P_IPV6: u16   = 0x86DD;

/// Byte offset of the Monad data within a fully formed Kingdom packet.
///
/// Ethernet(14) + IPv6(40) + HBH(next_header=1 + hdr_ext_len=1 + opt_type=1 + opt_data_len=1)
///   = 14 + 40 + 4 = 58
const MONAD_PKT_OFFSET: usize = ETH_HLEN + IPV6_FIXED_HDR_LEN + 4;

/// Byte offset within the Monad register file where the protected mutable region
/// begins for BIT_FLIP (skip version byte at offset 0).
const MONAD_FLIP_FIRST_BYTE: usize = 1;

/// Last byte index (inclusive) in the Monad that BIT_FLIP may target.
/// We include scratch registers (0x0E-0x11) but intentionally skip the
/// checksum bytes (0x12-0x13) — flipping the checksum is less interesting
/// than flipping a real field and letting CRC mismatch detection fire.
const MONAD_FLIP_LAST_BYTE: usize = MONAD_SIZE - 3; // = 17

// ── TC entry point ────────────────────────────────────────────────────────────

#[classifier]
pub fn yaldabaoth_tc(mut ctx: TcContext) -> i32 {
    match try_yaldabaoth(&mut ctx) {
        Ok(action) => action,
        Err(_)     => TC_ACT_OK as i32,
    }
}

#[inline(always)]
fn try_yaldabaoth(ctx: &mut TcContext) -> Result<i32, ()> {
    increment_stat(STAT_PACKETS_TOTAL);

    // ── Check EtherType ───────────────────────────────────────────────────────
    let eth_proto: u16 = ctx.load(12).map_err(|_| ())?;
    if u16::from_be(eth_proto) != ETH_P_IPV6 {
        return Ok(TC_ACT_OK as i32);
    }
    increment_stat(STAT_PACKETS_IPV6);

    // ── Check IPv6 Next Header (offset 14 + 6 = 20) ───────────────────────────
    let next_hdr: u8 = ctx.load(ETH_HLEN + 6).map_err(|_| ())?;
    if next_hdr != IPV6_NEXTHDR_HBH {
        return Ok(TC_ACT_OK as i32);
    }
    increment_stat(STAT_PACKETS_HBH);

    // ── Verify Monad option type / data length ────────────────────────────────
    let hbh_offset  = ETH_HLEN + IPV6_FIXED_HDR_LEN;
    let opt_type: u8     = ctx.load(hbh_offset + 2).map_err(|_| ())?;
    let opt_data_len: u8 = ctx.load(hbh_offset + 3).map_err(|_| ())?;

    if opt_type != MONAD_OPT_TYPE || opt_data_len != MONAD_OPT_DATA_LEN {
        // Not our packet format — pass without chaos.
        return Ok(TC_ACT_OK as i32);
    }

    // ── Extract Flow Label from IPv6 header ───────────────────────────────────
    // IPv6 vtf field is at offset ETH_HLEN (byte 14 in the packet).
    // Load as big-endian u32: version(4) + traffic_class(8) + flow_label(20).
    let vtf: u32     = ctx.load(ETH_HLEN).map_err(|_| ())?;
    let flow_label   = u32::from_be(vtf) & 0x000F_FFFF;

    // ── Check CHAOS_TARGETS map ───────────────────────────────────────────────
    let target = match unsafe { CHAOS_TARGETS.get(&flow_label) } {
        Some(t) => *t,
        None    => return Ok(TC_ACT_OK as i32), // flow not enrolled for chaos
    };
    increment_stat(STAT_TARGETS_HIT);

    let mode  = (target & 0xFF) as u8;
    let param = (target >> 32) as u32;

    // ── Load current Monad ────────────────────────────────────────────────────
    let mut monad = load_monad_tc(ctx, MONAD_PKT_OFFSET)?;

    // ── Apply chaos mode ──────────────────────────────────────────────────────
    apply_chaos(ctx, &mut monad, mode, param, flow_label)?;

    // Emit CHAOS event (always — even if we corrupted the checksum, we still
    // log the pre-corruption Monad so the dashboard knows what was valid).
    let hop_id = cfg(CFG_HOP_ID) as u8;
    emit_event(EventType::Chaos as u8, hop_id, flow_label, &monad);

    Ok(TC_ACT_OK as i32)
}

// ── Chaos mode dispatch ───────────────────────────────────────────────────────

#[inline(always)]
fn apply_chaos(
    ctx:        &mut TcContext,
    monad:      &mut Monad,
    mode:       u8,
    param:      u32,
    flow_label: u32,
) -> Result<(), ()> {
    if mode == ChaosMode::BitFlip as u8 {
        apply_bit_flip(ctx, monad)?;
        increment_stat(STAT_MODE_BIT_FLIP);

    } else if mode == ChaosMode::Delay as u8 {
        apply_delay(ctx, monad, param)?;
        increment_stat(STAT_MODE_DELAY);

    } else if mode == ChaosMode::Duplicate as u8 {
        apply_duplicate(ctx, monad)?;
        increment_stat(STAT_MODE_DUPLICATE);

    } else if mode == ChaosMode::Truncate as u8 {
        apply_truncate(ctx, monad)?;
        increment_stat(STAT_MODE_TRUNCATE);

    } else {
        // ChaosMode::ChaosMarker (0x05) or unknown — CHAOS flag only.
        apply_marker(ctx, monad)?;
        increment_stat(STAT_MODE_MARKER);
    }

    Ok(())
}

/// BIT_FLIP: XOR a random byte in Monad[1..=17] with a random non-zero mask.
/// Checksum is NOT recomputed — the verifier (hop-ebpf) must detect the error.
#[inline(always)]
fn apply_bit_flip(ctx: &mut TcContext, monad: &mut Monad) -> Result<(), ()> {
    let rand     = unsafe { bpf_get_prandom_u32() };
    let mask     = (rand & 0xFF) as u8;
    let mask     = if mask == 0 { 0x55 } else { mask }; // ensure non-zero flip

    // Target byte: pick from the 17-byte mutable region [1..=17].
    let range  = (MONAD_FLIP_LAST_BYTE - MONAD_FLIP_FIRST_BYTE + 1) as u32; // 17
    let rel    = ((rand >> 8) % range) as usize;
    let target = MONAD_FLIP_FIRST_BYTE + rel;

    let pkt_off = MONAD_PKT_OFFSET + target;
    let byte: u8 = ctx.load(pkt_off).map_err(|_| ())?;
    ctx.store(pkt_off, &(byte ^ mask), 0).map_err(|_| ())?;

    // Set CHAOS flag so downstream hops know this packet is under test.
    // We write CHAOS flag directly to packet — byte 7 of Monad (flags field).
    let flags_off = MONAD_PKT_OFFSET + 7; // OFF_FLAGS = 0x07
    let cur_flags: u8 = ctx.load(flags_off).map_err(|_| ())?;
    ctx.store(flags_off, &(cur_flags | flags::CHAOS), 0).map_err(|_| ())?;

    // Update our in-memory Monad copy for the event (reflects chaos state).
    let mut bytes = monad.to_bytes();
    bytes[target] ^= mask;
    bytes[7] |= flags::CHAOS;
    // Note: checksum bytes (18-19) are intentionally stale — that's the corruption.
    *monad = Monad::from_bytes(bytes);

    Ok(())
}

/// DELAY: Record requested delay in Monad scratch_r0, set CHAOS flag.
/// Actual delay is applied by a netem qdisc that this TC classifier feeds into.
/// The trace-collector reads the delay_us from the CHAOS event's Monad.scratch.
#[inline(always)]
fn apply_delay(ctx: &mut TcContext, monad: &mut Monad, delay_us: u32) -> Result<(), ()> {
    // Store delay_us in scratch_r0 (bytes 14-15 of Monad) for userspace read.
    monad.set_scratch_r0(delay_us.min(0xFFFF) as u16);
    monad.set_flag(flags::CHAOS);
    monad.recompute_checksum();
    write_monad_tc(ctx, MONAD_PKT_OFFSET, monad)
}

/// DUPLICATE: Clone the packet to the configured egress ifindex.
/// If CFG_EGRESS_IFINDEX is 0 (not configured), falls back to CHAOS_MARKER.
#[inline(always)]
fn apply_duplicate(ctx: &mut TcContext, monad: &mut Monad) -> Result<(), ()> {
    let ifindex = cfg(CFG_EGRESS_IFINDEX) as u32;
    if ifindex > 0 {
        // bpf_clone_redirect: send a copy of the packet to `ifindex`.
        // Returns 0 on success, negative on error.  We ignore the error —
        // failing to duplicate is non-fatal for the original packet.
        let rc = unsafe {
            aya_ebpf::helpers::bpf_clone_redirect(ctx.skb.skb, ifindex, 0)
        };
        if rc != 0 {
            increment_stat(STAT_CLONE_FAILED);
        }
    }
    // Mark both the original and (if successful) the clone with CHAOS.
    apply_marker(ctx, monad)
}

/// TRUNCATE: Zero Monad bytes 0x08–0x13 (Latency Hint through Checksum).
/// The next hop's CRC check will detect the truncation.
#[inline(always)]
fn apply_truncate(ctx: &mut TcContext, monad: &mut Monad) -> Result<(), ()> {
    // Zero bytes 8 through 19 of the Monad (latency_hint through checksum).
    // Offset in packet: MONAD_PKT_OFFSET + 8 through MONAD_PKT_OFFSET + 19.
    for i in 8..MONAD_SIZE {
        let off = MONAD_PKT_OFFSET + i;
        ctx.store(off, &0u8, 0).map_err(|_| ())?;
    }

    // Set CHAOS flag (byte 7 of Monad) — but we've already zeroed 8+, so set it.
    let flags_off = MONAD_PKT_OFFSET + 7;
    let cur_flags: u8 = ctx.load(flags_off).map_err(|_| ())?;
    ctx.store(flags_off, &(cur_flags | flags::CHAOS), 0).map_err(|_| ())?;

    // Update in-memory copy for event.
    let mut bytes = monad.to_bytes();
    for b in bytes[8..].iter_mut() {
        *b = 0;
    }
    bytes[7] |= flags::CHAOS;
    *monad = Monad::from_bytes(bytes);

    Ok(())
}

/// CHAOS_MARKER: Set CHAOS flag, recompute CRC.  No data corruption.
/// Used to tag a packet for "chaos observation" without breaking it.
#[inline(always)]
fn apply_marker(ctx: &mut TcContext, monad: &mut Monad) -> Result<(), ()> {
    monad.set_flag(flags::CHAOS);
    monad.recompute_checksum();
    write_monad_tc(ctx, MONAD_PKT_OFFSET, monad)
}

// ── TC packet access helpers ──────────────────────────────────────────────────

/// Load 20 Monad bytes from a TC context at `pkt_offset`.
#[inline(always)]
fn load_monad_tc(ctx: &TcContext, pkt_offset: usize) -> Result<Monad, ()> {
    let mut bytes = [0u8; 20];
    // Exactly 20 iterations — verifier-trivial.
    for i in 0..20usize {
        bytes[i] = ctx.load::<u8>(pkt_offset + i).map_err(|_| ())?;
    }
    Ok(Monad::from_bytes(bytes))
}

/// Write 20 Monad bytes to a TC context at `pkt_offset`.
#[inline(always)]
fn write_monad_tc(ctx: &mut TcContext, pkt_offset: usize, m: &Monad) -> Result<(), ()> {
    let bytes = m.to_bytes();
    for i in 0..20usize {
        ctx.store(pkt_offset + i, &bytes[i], 0).map_err(|_| ())?;
    }
    Ok(())
}

// ── Config / stats helpers ────────────────────────────────────────────────────

#[inline(always)]
fn cfg(key: u32) -> u64 {
    match unsafe { CONFIG.get(&key) } {
        Some(v) => *v,
        None    => 0,
    }
}

#[inline(always)]
fn increment_stat(key: u32) {
    if let Some(v) = STATS.get_ptr_mut(&key) {
        unsafe { *v = (*v).saturating_add(1); }
    } else {
        let _ = STATS.insert(&key, &1u64, 0);
    }
}

// ── Anamnesis event emission ──────────────────────────────────────────────────

#[inline(always)]
fn emit_event(event_type: u8, hop_id: u8, flow_label: u32, monad: &Monad) {
    let now   = unsafe { bpf_ktime_get_ns() };
    let event = AnamnesisEvent {
        timestamp_ns:  now,
        event_type,
        hop_id,
        flow_label_lo: [
            ((flow_label >> 8) & 0xFF) as u8,
            ( flow_label       & 0xFF) as u8,
        ],
        monad: *monad,
    };

    if let Some(mut buf) = ANAMNESIS.reserve::<AnamnesisEvent>(0) {
        unsafe {
            core::ptr::write(buf.as_mut_ptr(), event);
            buf.submit(0);
        }
        increment_stat(STAT_EVENTS_SENT);
    } else {
        increment_stat(STAT_EVENTS_DROPPED);
    }
}

// ── Panic handler ─────────────────────────────────────────────────────────────

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
