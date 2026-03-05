// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//! Unheaded Protocol — Version eBPF Program (App 11: Protocol Version Translation)
//!
//! This program provides transparent protocol version translation for the
//! Unheaded Protocol.  Two entry points handle ingress and egress:
//!
//! 1. **XDP ingress**: Parse IPv6 + HbH + Monad, check the version byte against
//!    the destination service's expected version from SERVICE_VERSIONS.  On
//!    mismatch, look up translation rules from VERSION_MATRIX and apply field
//!    mappings in-place (move, default).  Recalculate CRC-16 if the rule requires.
//!
//! 2. **TC egress**: Similar but reverse direction — ensure outgoing packets
//!    match the destination service's expected version.
//!
//! # BPF Verifier Notes
//!
//! All packet reads use `read_volatile` with explicit bounds checks.  Every loop
//! is bounded by a compile-time constant.  `core::hint::black_box` prevents LLVM
//! from eliminating the per-iteration bounds checks the verifier requires.

#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::xdp_action,
    helpers::bpf_ktime_get_ns,
    macros::{map, xdp},
    maps::{HashMap, RingBuf},
    programs::XdpContext,
};
use monad_common::{
    flags, CrcCcitt, Monad, HBH_TOTAL_LEN, IPV6_FIXED_HDR_LEN, IPV6_NEXTHDR_HBH,
    MONAD_CRC_DATA_LEN, MONAD_OPT_DATA_LEN, MONAD_OPT_TYPE, MONAD_SIZE,
};

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// Version translation matrix: (src_ver, dst_ver) packed as u16 -> TranslationRule.
/// Defines how to translate between protocol versions.
#[map]
static VERSION_MATRIX: HashMap<u16, [u8; 48]> = HashMap::with_max_entries(256, 0);
// TranslationRule layout (48 bytes):
//   [0..4]:   field_mask (u32, bitmask of fields to translate)
//   [4..36]:  offset_map[8] (8 x u32 = 32 bytes: src_offset -> dst_offset pairs)
//   [36..44]: width_deltas[4] (4 x i16 = 8 bytes: field width adjustments)
//   [44..48]: default_values[4] (4 x u8 = 4 bytes: defaults for missing fields)
// Note: crc_recalc is encoded in field_mask bit 31

/// Service version registry: service_id -> VersionInfo.
#[map]
static SERVICE_VERSIONS: HashMap<u32, [u8; 12]> = HashMap::with_max_entries(256, 0);
// VersionInfo layout (12 bytes):
//   [0]:      current_version (u8)
//   [1..5]:   capabilities_mask (u32)
//   [5..9]:   last_updated (u32, epoch seconds)
//   [9..12]:  _pad (3 bytes)

/// Translation metric events emitted to ring buffer.
#[map]
static TRANSLATION_METRICS: RingBuf = RingBuf::with_byte_size(2 * 1024 * 1024, 0);

/// Runtime configuration — tunable from userspace without reloading.
///
/// | Key | Meaning                            | Default |
/// |-----|------------------------------------|---------|
/// | 0   | node_id (this node's identifier)   | 0       |
/// | 1   | default_version (fallback)         | 0x01    |
/// | 2   | strict_mode (drop on no rule)      | 0       |
#[map]
static CONFIG: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

/// Statistics counters — observable from userspace.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

// ── Stats keys ────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_TRANSLATIONS_APPLIED: u32 = 1;
const STAT_TRANSLATIONS_SKIPPED: u32 = 2;
const STAT_CRC_RECALCULATED: u32 = 3;
const STAT_ERRORS: u32 = 4;
const STAT_VERSION_MISMATCHES: u32 = 5;
const STAT_EVENTS_SENT: u32 = 6;
const STAT_EVENTS_DROPPED: u32 = 7;
const STAT_EGRESS_TOTAL: u32 = 8;
const STAT_EGRESS_TRANSLATIONS: u32 = 9;

// ── Config keys ───────────────────────────────────────────────────────────────
const CFG_NODE_ID: u32 = 0;
const CFG_DEFAULT_VERSION: u32 = 1;
const CFG_STRICT_MODE: u32 = 2;

// ── Translation event types ───────────────────────────────────────────────────
const TRANS_EVENT_INGRESS: u8 = 0x01;
const TRANS_EVENT_EGRESS: u8 = 0x02;
const TRANS_EVENT_MISMATCH: u8 = 0x03;
const TRANS_EVENT_ERROR: u8 = 0x04;

// ── Wire-format helpers ─────────────────────────────────────────────────────

/// Ethernet header (14 bytes, packed).
#[repr(C, packed)]
struct EthHdr {
    dst: [u8; 6],
    src: [u8; 6],
    proto: u16,
}

/// IPv6 fixed header (40 bytes, packed).
#[repr(C, packed)]
struct Ipv6Hdr {
    vtf: u32,
    payload_len: u16,
    next_header: u8,
    hop_limit: u8,
    src: [u8; 16],
    dst: [u8; 16],
}

const ETH_HLEN: usize = 14;
const ETH_P_IPV6: u16 = 0x86DD;

// ── Translation metric event ──────────────────────────────────────────────────

/// Translation metric event (24 bytes).
#[repr(C)]
struct TranslationMetricEvent {
    timestamp: u64,
    event_type: u8,
    src_version: u8,
    dst_version: u8,
    service_id: u8,
    fields_translated: u32,
}

// ── XDP ingress entry point ──────────────────────────────────────────────────

#[xdp]
pub fn version_xdp_ingress(ctx: XdpContext) -> u32 {
    match try_version_xdp_ingress(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS, // fail-open: never silently drop
    }
}

#[inline(always)]
fn try_version_xdp_ingress(ctx: &XdpContext) -> Result<u32, ()> {
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

    // ── IPv6 Fixed Header ─────────────────────────────────────────────────────
    let ip_start = data + ETH_HLEN;
    if ip_start + IPV6_FIXED_HDR_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let ip = unsafe { &*(ip_start as *const Ipv6Hdr) };

    if ip.next_header != IPV6_NEXTHDR_HBH {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Hop-by-Hop Header ─────────────────────────────────────────────────────
    let hbh_start = ip_start + IPV6_FIXED_HDR_LEN;

    if hbh_start + HBH_TOTAL_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    let hbh_ext_len = unsafe { core::ptr::read_volatile((hbh_start + 1) as *const u8) };
    let hbh_total = (hbh_ext_len as usize + 1) * 8;

    if hbh_start + hbh_total > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    let opts_start = hbh_start + 2;
    let opts_end = hbh_start + hbh_total;

    // ── Find Monad option ─────────────────────────────────────────────────────
    let monad_opt_off = find_monad_option(opts_start, opts_end, data_end)?;

    let monad_data_off = monad_opt_off + 2;
    if monad_data_off + MONAD_SIZE > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Read Monad ────────────────────────────────────────────────────────────
    let monad = read_monad_from_pkt(monad_data_off, data_end)?;
    let src_version = monad.version;

    // ── Look up destination service version ───────────────────────────────────
    let dst_svc_id = monad.dst_service_id as u32;
    let dst_version = match unsafe { SERVICE_VERSIONS.get(&dst_svc_id) } {
        Some(vi) => unsafe { core::ptr::read_volatile(vi.as_ptr()) },
        None => {
            // No service version info — use default
            let default_ver = cfg(CFG_DEFAULT_VERSION) as u8;
            if default_ver == 0 { 0x01 } else { default_ver }
        }
    };

    // ── Version match check ───────────────────────────────────────────────────
    if src_version == dst_version {
        increment_stat(STAT_TRANSLATIONS_SKIPPED);
        return Ok(xdp_action::XDP_PASS);
    }

    increment_stat(STAT_VERSION_MISMATCHES);

    // ── Look up translation rule ──────────────────────────────────────────────
    let matrix_key = ((src_version as u16) << 8) | (dst_version as u16);
    let rule = match unsafe { VERSION_MATRIX.get(&matrix_key) } {
        Some(r) => r,
        None => {
            // No translation rule — strict mode drops, permissive passes
            if cfg(CFG_STRICT_MODE) != 0 {
                increment_stat(STAT_ERRORS);
                emit_translation_event(TRANS_EVENT_ERROR, src_version, dst_version, dst_svc_id as u8, 0);
                return Ok(xdp_action::XDP_DROP);
            }
            increment_stat(STAT_TRANSLATIONS_SKIPPED);
            return Ok(xdp_action::XDP_PASS);
        }
    };

    // ── Apply translation rule in-place ───────────────────────────────────────
    let mut m = monad;
    let fields_translated = apply_translation_rule(&mut m, rule, dst_version);

    // ── CRC recalculation ─────────────────────────────────────────────────────
    // Bit 31 of field_mask signals CRC recalculation is required
    let field_mask = unsafe { core::ptr::read_volatile(rule.as_ptr() as *const u32) };
    let crc_recalc = (field_mask & 0x8000_0000) != 0;

    if crc_recalc && !m.has_flag(flags::CUSTOM) {
        m.recompute_checksum();
        increment_stat(STAT_CRC_RECALCULATED);
    }

    // ── Write translated Monad back ───────────────────────────────────────────
    write_monad_to_pkt(monad_data_off, data_end, &m)?;

    increment_stat(STAT_TRANSLATIONS_APPLIED);
    emit_translation_event(TRANS_EVENT_INGRESS, src_version, dst_version, dst_svc_id as u8, fields_translated);

    Ok(xdp_action::XDP_PASS)
}

// ── TC egress entry point ─────────────────────────────────────────────────────
//
// TC programs use the same XdpContext-compatible pattern.  In a full deployment,
// this would be attached as a TC BPF program.  Here we implement the logic as
// a separate XDP entry point that can be used for egress-direction translation.

#[xdp]
pub fn version_tc_egress(ctx: XdpContext) -> u32 {
    match try_version_tc_egress(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_version_tc_egress(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_EGRESS_TOTAL);

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

    // ── IPv6 Fixed Header ─────────────────────────────────────────────────────
    let ip_start = data + ETH_HLEN;
    if ip_start + IPV6_FIXED_HDR_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let ip = unsafe { &*(ip_start as *const Ipv6Hdr) };

    if ip.next_header != IPV6_NEXTHDR_HBH {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Hop-by-Hop Header ─────────────────────────────────────────────────────
    let hbh_start = ip_start + IPV6_FIXED_HDR_LEN;

    if hbh_start + HBH_TOTAL_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    let hbh_ext_len = unsafe { core::ptr::read_volatile((hbh_start + 1) as *const u8) };
    let hbh_total = (hbh_ext_len as usize + 1) * 8;

    if hbh_start + hbh_total > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    let opts_start = hbh_start + 2;
    let opts_end = hbh_start + hbh_total;

    // ── Find Monad option ─────────────────────────────────────────────────────
    let monad_opt_off = find_monad_option(opts_start, opts_end, data_end)?;

    let monad_data_off = monad_opt_off + 2;
    if monad_data_off + MONAD_SIZE > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Read Monad ────────────────────────────────────────────────────────────
    let monad = read_monad_from_pkt(monad_data_off, data_end)?;
    let current_version = monad.version;

    // ── Egress: ensure packet matches destination's expected version ──────────
    // For egress, we use the dst_service_id to determine what version the
    // remote service expects.
    let dst_svc_id = monad.dst_service_id as u32;
    let expected_version = match unsafe { SERVICE_VERSIONS.get(&dst_svc_id) } {
        Some(vi) => unsafe { core::ptr::read_volatile(vi.as_ptr()) },
        None => return Ok(xdp_action::XDP_PASS), // no info, pass through
    };

    if current_version == expected_version {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Reverse translation ───────────────────────────────────────────────────
    let matrix_key = ((current_version as u16) << 8) | (expected_version as u16);
    let rule = match unsafe { VERSION_MATRIX.get(&matrix_key) } {
        Some(r) => r,
        None => return Ok(xdp_action::XDP_PASS),
    };

    let mut m = monad;
    let fields_translated = apply_translation_rule(&mut m, rule, expected_version);

    let field_mask = unsafe { core::ptr::read_volatile(rule.as_ptr() as *const u32) };
    let crc_recalc = (field_mask & 0x8000_0000) != 0;

    if crc_recalc && !m.has_flag(flags::CUSTOM) {
        m.recompute_checksum();
        increment_stat(STAT_CRC_RECALCULATED);
    }

    write_monad_to_pkt(monad_data_off, data_end, &m)?;

    increment_stat(STAT_EGRESS_TRANSLATIONS);
    emit_translation_event(TRANS_EVENT_EGRESS, current_version, expected_version, dst_svc_id as u8, fields_translated);

    Ok(xdp_action::XDP_PASS)
}

// ── Translation rule application ──────────────────────────────────────────────

/// Apply a translation rule to a Monad in-place.
/// Returns the number of fields translated.
#[inline(always)]
fn apply_translation_rule(m: &mut Monad, rule: &[u8; 48], dst_version: u8) -> u32 {
    let field_mask = unsafe { core::ptr::read_volatile(rule.as_ptr() as *const u32) };
    let mut fields_translated: u32 = 0;

    // Convert Monad to mutable bytes for field manipulation
    let mut bytes = m.to_bytes();

    // ── Apply offset mappings (8 pairs of src_offset -> dst_offset) ───────────
    // offset_map starts at byte 4, each entry is 4 bytes: (src_off u16, dst_off u16)
    for i in 0..8usize {
        let map_bit = 1u32 << i;
        if field_mask & map_bit == 0 {
            continue;
        }

        let entry_offset = 4 + i * 4;
        if entry_offset + 4 > 48 {
            break;
        }

        let src_off = unsafe {
            core::ptr::read_volatile(rule.as_ptr().add(entry_offset) as *const u16)
        } as usize;
        let dst_off = unsafe {
            core::ptr::read_volatile(rule.as_ptr().add(entry_offset + 2) as *const u16)
        } as usize;

        // Bounds check: both offsets must be within the 20-byte Monad
        if src_off < MONAD_SIZE && dst_off < MONAD_SIZE {
            bytes[dst_off] = bytes[src_off];
            fields_translated += 1;
        }
    }

    // ── Apply default values for fields not present in source version ─────────
    // default_values at bytes 44-47: 4 x u8 values for fields that need defaults
    // The corresponding field offset is derived from width_deltas at bytes 36-43
    for i in 0..4usize {
        let default_bit = 1u32 << (8 + i);
        if field_mask & default_bit == 0 {
            continue;
        }

        // Read the target offset from width_deltas (interpreted as offset for default)
        let delta_offset = 36 + i * 2;
        if delta_offset + 2 > 48 {
            break;
        }
        let target_off = unsafe {
            core::ptr::read_volatile(rule.as_ptr().add(delta_offset) as *const u16)
        } as usize;

        let default_val = unsafe {
            core::ptr::read_volatile(rule.as_ptr().add(44 + i))
        };

        if target_off < MONAD_SIZE {
            bytes[target_off] = default_val;
            fields_translated += 1;
        }
    }

    // ── Set the target version ────────────────────────────────────────────────
    bytes[0] = dst_version;

    // Reconstruct Monad from modified bytes
    *m = Monad::from_bytes(bytes);
    fields_translated
}

// ── Packet memory helpers ─────────────────────────────────────────────────────

/// Find the byte offset of the MONAD_OPT_TYPE option within an HBH option area.
///
/// Bounded to 16 iterations — verifier-safe.  `black_box` prevents LLVM from
/// eliminating the per-iteration `data_end` checks.
#[inline(always)]
fn find_monad_option(opts_start: usize, opts_end: usize, data_end: usize) -> Result<usize, ()> {
    let opts_end = core::hint::black_box(opts_end);
    let mut offset = opts_start;

    for _ in 0..16usize {
        if offset >= opts_end {
            break;
        }
        if offset + 1 > data_end {
            break;
        }

        let opt_type = unsafe { core::ptr::read_volatile(offset as *const u8) };

        if opt_type == 0 {
            break;
        }

        if opt_type == 1 {
            offset += 1;
            continue;
        }

        if offset + 2 > data_end {
            break;
        }
        let opt_data_len = unsafe { core::ptr::read_volatile((offset + 1) as *const u8) };
        let opt_total = 2 + opt_data_len as usize;

        if opt_total < 2 || offset + opt_total > data_end {
            break;
        }

        if opt_type == MONAD_OPT_TYPE
            && opt_data_len == MONAD_OPT_DATA_LEN
            && offset + 2 + MONAD_SIZE <= data_end
        {
            return Ok(offset);
        }

        offset += opt_total;
    }

    Err(())
}

/// Read 20 Monad bytes from packet memory into a [`Monad`] value.
#[inline(always)]
fn read_monad_from_pkt(start: usize, data_end: usize) -> Result<Monad, ()> {
    if start + MONAD_SIZE > data_end {
        return Err(());
    }
    let mut bytes = [0u8; 20];
    #[allow(clippy::needless_range_loop)]
    for i in 0..20usize {
        bytes[i] = unsafe { core::ptr::read_volatile((start + i) as *const u8) };
    }
    Ok(Monad::from_bytes(bytes))
}

/// Write a mutated [`Monad`] back into packet memory at the same offset.
#[inline(always)]
fn write_monad_to_pkt(start: usize, data_end: usize, m: &Monad) -> Result<(), ()> {
    if start + MONAD_SIZE > data_end {
        return Err(());
    }
    let bytes = m.to_bytes();
    #[allow(clippy::needless_range_loop)]
    for i in 0..20usize {
        unsafe { core::ptr::write_volatile((start + i) as *mut u8, bytes[i]) };
    }
    Ok(())
}

// ── Translation event emission ────────────────────────────────────────────────

/// Emit a translation metric event to the TRANSLATION_METRICS ring buffer.
#[inline(always)]
fn emit_translation_event(
    event_type: u8,
    src_version: u8,
    dst_version: u8,
    service_id: u8,
    fields_translated: u32,
) {
    let now = unsafe { bpf_ktime_get_ns() };
    let event = TranslationMetricEvent {
        timestamp: now,
        event_type,
        src_version,
        dst_version,
        service_id,
        fields_translated,
    };

    if let Some(mut buf) = TRANSLATION_METRICS.reserve::<TranslationMetricEvent>(0) {
        unsafe {
            core::ptr::write(buf.as_mut_ptr(), event);
            buf.submit(0);
        }
        increment_stat(STAT_EVENTS_SENT);
    } else {
        increment_stat(STAT_EVENTS_DROPPED);
    }
}

// ── Config / stats helpers ────────────────────────────────────────────────────

/// Read a CONFIG entry, returning 0 if not present.
#[inline(always)]
fn cfg(key: u32) -> u64 {
    match unsafe { CONFIG.get(&key) } {
        Some(v) => *v,
        None => 0,
    }
}

/// Saturating increment of a STATS counter.
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

// ── Panic handler (required for #![no_std]) ───────────────────────────────────

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
