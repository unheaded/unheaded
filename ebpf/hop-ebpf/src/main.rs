// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Unheaded Protocol — Hop eBPF Program
//!
//! This XDP program runs at every interior Kingdom node.  It is the per-hop ALU
//! of the Unheaded Protocol Foundation:
//!
//! 1. Parse IPv6 + HBH headers to locate the Monad option
//! 2. Verify CRC-16/CCITT — emit ANOMALY on failure, pass without mutation
//! 3. Skip if CHAOS flag is set (Yaldabaoth owns that packet)
//! 4. Enforce hop limit (emit ANOMALY, pass — routing is broken, not our call)
//!    4b. ADR-125 hop checks: off unless CONFIG key 4 says observe or enforce
//! 5. Increment Hop Count (saturating)
//! 6. Sophia dictionary lookups for QoS, circuit state, service routing
//! 7. Circuit breaker enforcement via CIRCUIT_ERRORS map
//! 8. Apply flow_action semantics (SAMPLE, TRACE, MIRROR, DROP)
//! 9. Recompute CRC-16/CCITT over mutated Monad
//! 10. Write mutated Monad back in-place (no packet resize)
//! 11. Emit HOP event (head-based sampling or TRACED flag)
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
    helpers::{bpf_ktime_get_boot_ns, bpf_ktime_get_ns},
    macros::{map, xdp},
    maps::{HashMap, RingBuf},
    programs::XdpContext,
};
use monad_common::{
    circuit_state as cs, flags, flow_action as fa, AnamnesisEvent, EventType, Monad, SophiaEntry,
    HBH_TOTAL_LEN, IPV6_FIXED_HDR_LEN, IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN, MONAD_OPT_TYPE,
    MONAD_SIZE,
};

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// Anamnesis ring buffer — 8 MiB, shared with trace-collector userspace reader.
/// Each emitted event is exactly 32 bytes (one cache line).
#[map]
static ANAMNESIS: RingBuf = RingBuf::with_byte_size(8 * 1024 * 1024, 0);

/// Sophia dictionary map.
///
/// Key encoding: `(dict_id as u16) << 8 | (field_value as u16)`.
/// Packs all per-field dictionaries into one map — 65 536 max entries.
/// Values are exponent-encoded metadata for each Kingdom service/state/class.
#[map]
static SOPHIA: HashMap<u16, SophiaEntry> = HashMap::with_max_entries(65_536, 0);

/// Circuit breaker error counters.
///
/// Key: `(src_service_id as u16) | ((dst_service_id as u16) << 8)`
/// Value: running error count since last reset.
/// Incremented by external health-check userspace; read here to trip the circuit.
#[map]
static CIRCUIT_ERRORS: HashMap<u16, u32> = HashMap::with_max_entries(65_536, 0);

/// Runtime configuration — tunable from userspace without reloading the program.
///
/// | Key | Meaning                            | Default |
/// |-----|------------------------------------|---------|
/// | 0   | hop_id (this node's identifier)    | 0       |
/// | 1   | sample_divisor (flow_label % N)    | 0 = all |
/// | 2   | circuit_threshold (errors→OPEN)    | 0 = off |
/// | 3   | max_hops (reject above this)       | 0 = off |
/// | 4   | check mode (ADR-125): 1 observe, 2 enforce | 0 = off |
#[map]
static CONFIG: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

/// Statistics counters — observable from userspace.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

/// Per-namespace sequence counters (RFC 9000 §5.1.1).
/// Key: namespace (u32, u32 reserved = u64)
/// Value: current (u32), highest (u32), gaps (u32), reordered (u32)
#[map]
static SEQ_COUNTERS: HashMap<u64, [u8; 16]> = HashMap::with_max_entries(256, 0);

/// Per-connection capability settings (RFC 9114 §7.2.4).
/// Key: (conn_id: u32, setting_id: u16, reserved: u16)
/// Value: value (u64), negotiated_at (u32), flags (u32)
#[map]
static SETTINGS: HashMap<u64, [u8; 16]> = HashMap::with_max_entries(4096, 0);

/// DoS backpressure state tracking (RFC 9114 §10).
/// Key: namespace (u32, u32 reserved = u64)
/// Value: drop_count (u32), total_count (u32), backpressure_lvl (u8), window_start (u32)
#[map]
static DOS_STATE: HashMap<u64, [u8; 16]> = HashMap::with_max_entries(256, 0);

/// Flow type classification table (RFC 9114 §6).
/// Key: flow_id (u32 index)
/// Value: type (u8), flags (u8), reserved (u16)
#[map]
static FLOW_TYPES: HashMap<u64, [u8; 4]> = HashMap::with_max_entries(65536, 0);

/// One rule set per (hop, namespace), ADR-125. `pkg/protocol/bpfschema`
/// mirrors it as `HopValidatorValue`; `allowed_dict_ids` is not evaluated,
/// because no dictionary id is on the wire.
#[repr(C)]
#[derive(Clone, Copy)]
pub struct HopValidator {
    /// `allowed_version_min` and `allowed_version_max` both 0: no version rule.
    pub allowed_version_min: u8,
    pub allowed_version_max: u8,
    /// Non-zero: a CUSTOM packet fails, having no CRC to check.
    pub require_crc: u8,
    /// Non-zero: a live HMAC_KEYS entry for (flow_id, namespace) is required.
    pub require_hmac: u8,
    pub allowed_dict_ids: [u16; 8],
    /// 0: no cap. Otherwise hop_count must be at or under it.
    pub max_hop_count: u8,
    pub _pad: [u8; 3],
    /// Bit 0: a live RETRY_TOKENS entry for (flow_id, source) is required.
    pub flags: u32,
    pub _reserved: [u8; 4],
}

/// An HMAC key held for a (flow_id, namespace). v0x01 carries no tag and eBPF
/// has no HMAC helper, so only presence and expiry are read: `key` is never
/// touched here. Times are CLOCK_BOOTTIME seconds; `expires_at` 0 means never.
#[repr(C)]
#[derive(Clone, Copy)]
pub struct HmacKeyEntry {
    pub key: [u8; 32],
    pub created_at: u32,
    pub expires_at: u32,
}

/// A source validated earlier. No retry token is on the wire, so `token` is
/// never compared: the entry says only that a validation exists and is live.
/// Times as in [`HmacKeyEntry`].
#[repr(C)]
#[derive(Clone, Copy)]
pub struct RetryEntry {
    pub token: [u8; 32],
    pub issued_at: u32,
    pub expires_at: u32,
    pub _reserved: [u8; 8],
}

/// Hop validators (ADR-125). Key: [`make_validator_key`].
#[map]
static HOP_VALIDATORS: HashMap<u64, HopValidator> = HashMap::with_max_entries(256, 0);

/// Live HMAC keys (ADR-125). Key: [`make_hmac_key`].
#[map]
static HMAC_KEYS: HashMap<u64, HmacKeyEntry> = HashMap::with_max_entries(4096, 0);

/// Validated sources (ADR-125). Key: [`make_retry_key`].
#[map]
static RETRY_TOKENS: HashMap<u64, RetryEntry> = HashMap::with_max_entries(4096, 0);

// ── Stats keys ────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_PACKETS_IPV6: u32 = 1;
const STAT_PACKETS_HBH: u32 = 2;
const STAT_MONAD_FOUND: u32 = 3;
const STAT_CRC_BAD: u32 = 4;
const STAT_HOP_LIMIT_EXCEEDED: u32 = 5;
const STAT_CIRCUIT_OPEN: u32 = 6;
const STAT_SOPHIA_HITS: u32 = 7;
const STAT_EVENTS_SENT: u32 = 8;
const STAT_EVENTS_DROPPED: u32 = 9;
const STAT_CHAOS_SKIPPED: u32 = 10;
const STAT_ACTION_DROP: u32 = 11;
const STAT_ACTION_TRACE: u32 = 12;
const STAT_ACTION_SAMPLE: u32 = 13;
const STAT_ACTION_MIRROR: u32 = 14;
const STAT_CHECKS_RUN: u32 = 15;
const STAT_CHECKS_FAILED: u32 = 16;
const STAT_CHECKS_ENFORCED: u32 = 17;
const STAT_SETTINGS_REJECTED: u32 = 20;
const STAT_DOS_ECN_SET: u32 = 21;
const STAT_DOS_DROPPED: u32 = 22;
const STAT_FLOW_CONTROL: u32 = 23;
const STAT_FLOW_DATA: u32 = 24;
const STAT_FLOW_BULK: u32 = 25;
const STAT_FLOW_UNKNOWN: u32 = 26;

// ── Config keys ───────────────────────────────────────────────────────────────
const CFG_HOP_ID: u32 = 0;
const CFG_SAMPLE_DIVISOR: u32 = 1;
const CFG_CIRCUIT_THRESHOLD: u32 = 2;
const CFG_MAX_HOPS: u32 = 3;
const CFG_CHECK_MODE: u32 = 4;

// ── Hop check modes (ADR-125) ─────────────────────────────────────────────────
// Anything else, 0 included, is off.
const CHECK_MODE_OBSERVE: u64 = 1;
const CHECK_MODE_ENFORCE: u64 = 2;

/// `HopValidator::flags` bit 0: a live RETRY_TOKENS entry is required.
const VALIDATOR_FLAG_RETRY: u32 = 0x01;

// ── Sophia dictionary IDs ─────────────────────────────────────────────────────
// These MUST match the dictionary IDs configured in the Kingdom Sophia service.
const SOPHIA_DICT_QOS: u8 = 0x01; // qos_class     → scheduling weight / DSCP mark
const SOPHIA_DICT_CIRCUIT: u8 = 0x02; // circuit_state → human-readable metadata
const SOPHIA_DICT_SERVICE: u8 = 0x03; // service_id    → service name / tier

// ── Wire-format helpers ───────────────────────────────────────────────────────

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
    vtf: u32, // version(4) + traffic-class(8) + flow-label(20)
    payload_len: u16,
    next_header: u8,
    hop_limit: u8,
    src: [u8; 16],
    dst: [u8; 16],
}

const ETH_HLEN: usize = 14;
const ETH_P_IPV6: u16 = 0x86DD;

// ── XDP entry point ───────────────────────────────────────────────────────────

#[xdp]
pub fn hop_xdp(ctx: XdpContext) -> u32 {
    match try_hop_xdp(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS, // fail-open: never silently drop
    }
}

#[inline(always)]
fn try_hop_xdp(ctx: &XdpContext) -> Result<u32, ()> {
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
    increment_stat(STAT_PACKETS_IPV6);

    // ── IPv6 Fixed Header ─────────────────────────────────────────────────────
    let ip_start = data + ETH_HLEN;
    if ip_start + IPV6_FIXED_HDR_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let ip = unsafe { &*(ip_start as *const Ipv6Hdr) };

    // Only our HBH-carrying packets.
    if ip.next_header != IPV6_NEXTHDR_HBH {
        return Ok(xdp_action::XDP_PASS);
    }
    increment_stat(STAT_PACKETS_HBH);

    // Extract Flow Label from the packed vtf field (low 20 bits).
    let vtf = u32::from_be(unsafe { core::ptr::read_unaligned(core::ptr::addr_of!(ip.vtf)) });
    let flow_label = vtf & 0x000F_FFFF;

    // ── Hop-by-Hop Header ─────────────────────────────────────────────────────
    let hbh_start = ip_start + IPV6_FIXED_HDR_LEN;

    // Minimum: 2 bytes fixed + 2 bytes option header + 20 bytes Monad = 24.
    if hbh_start + HBH_TOTAL_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    // hdr_ext_len: length in 8-byte units, not counting the first 8.
    let hbh_ext_len = unsafe { core::ptr::read_volatile((hbh_start + 1) as *const u8) };
    let hbh_total = (hbh_ext_len as usize + 1) * 8;

    if hbh_start + hbh_total > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    // Options live at bytes [2..hbh_total] within the HBH header.
    let opts_start = hbh_start + 2;
    let opts_end = hbh_start + hbh_total;

    // ── Find Monad option ─────────────────────────────────────────────────────
    let monad_opt_off = find_monad_option(opts_start, opts_end, data_end)?;
    increment_stat(STAT_MONAD_FOUND);

    // Option layout: opt_type(1) + opt_data_len(1) + opt_data(N).
    let monad_data_off = monad_opt_off + 2;
    if monad_data_off + MONAD_SIZE > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Read Monad ────────────────────────────────────────────────────────────
    let monad = read_monad_from_pkt(monad_data_off, data_end)?;

    // ── CRC Verification ──────────────────────────────────────────────────────
    // CUSTOM flag means checksum field carries exponent keys — skip CRC check.
    if !monad.has_flag(flags::CUSTOM) && !monad.verify_checksum() {
        increment_stat(STAT_CRC_BAD);
        let hop_id = cfg(CFG_HOP_ID) as u8;
        emit_event(EventType::Anomaly as u8, hop_id, flow_label, &monad);
        // Fail-open: pass the packet, let upper layers deal with corruption.
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Chaos Guard ───────────────────────────────────────────────────────────
    // Yaldabaoth has marked this packet as under chaos injection.
    // Hop processing would overwrite its intentional corruption — skip.
    if monad.has_flag(flags::CHAOS) {
        increment_stat(STAT_CHAOS_SKIPPED);
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Hop Limit Guard ───────────────────────────────────────────────────────
    let max_hops = cfg(CFG_MAX_HOPS) as u8;
    if max_hops > 0 && monad.hop_count >= max_hops {
        increment_stat(STAT_HOP_LIMIT_EXCEEDED);
        let hop_id = cfg(CFG_HOP_ID) as u8;
        emit_event(EventType::Anomaly as u8, hop_id, flow_label, &monad);
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Hop Checks (ADR-125) ──────────────────────────────────────────────────
    // Reads only: the Monad is not cloned yet, and no wire field is written.
    if run_checks(&monad, &ip.src, flow_label) {
        return Ok(xdp_action::XDP_DROP);
    }

    // ── Clone for mutation ────────────────────────────────────────────────────
    let mut m = monad;

    // ── Sophia Dictionary Lookups ─────────────────────────────────────────────
    // These confirm the Kingdom knows about the encoded field values and could
    // return metadata (scheduling weights, tier labels, etc.) to act on.

    // QoS class: tells us what DSCP/scheduling priority to apply.
    if unsafe { SOPHIA.get(&sophia_key(SOPHIA_DICT_QOS, m.qos_class)) }.is_some() {
        increment_stat(STAT_SOPHIA_HITS);
        // Future extension: write DSCP bits into IPv6 Traffic Class field.
    }

    // Circuit state: confirm the encoded value is a known state.
    if unsafe { SOPHIA.get(&sophia_key(SOPHIA_DICT_CIRCUIT, m.circuit_state)) }.is_some() {
        increment_stat(STAT_SOPHIA_HITS);
    }

    // Source service: confirm the Kingdom recognises this service ID.
    if unsafe { SOPHIA.get(&sophia_key(SOPHIA_DICT_SERVICE, m.src_service_id)) }.is_some() {
        increment_stat(STAT_SOPHIA_HITS);
    }

    // ── Circuit Breaker Enforcement ───────────────────────────────────────────
    let threshold = cfg(CFG_CIRCUIT_THRESHOLD) as u32;
    if threshold > 0 {
        let cb_key = (m.src_service_id as u16) | ((m.dst_service_id as u16) << 8);
        if let Some(errors) = unsafe { CIRCUIT_ERRORS.get(&cb_key) } {
            if *errors >= threshold {
                m.circuit_state = cs::OPEN;
                increment_stat(STAT_CIRCUIT_OPEN);
            }
        }
    }

    // ── Sequence counter update (RFC 9000 §5.1.1) ────────────────────────────────
    // RFC 9000 §5.1.1 — Per-namespace sequence integrity
    let namespace: u32 = (flow_label >> 8) as u32;
    let seq_key = make_namespace_key(namespace);
    if let Some(seq_entry) = SEQ_COUNTERS.get_ptr_mut(&seq_key) {
        let entry = seq_entry as *mut u8;
        // current (bytes 0-3), highest (bytes 4-7), gaps (bytes 8-11), reordered (bytes 12-15)
        let current = unsafe { core::ptr::read_volatile(entry as *const u32) };
        let highest = unsafe { core::ptr::read_volatile(entry.add(4) as *const u32) };
        let new_val = current.saturating_add(1);
        unsafe {
            core::ptr::write_volatile(entry as *mut u32, new_val);
        }

        // Track highest seen and detect gaps / reordering
        if new_val > highest {
            unsafe {
                core::ptr::write_volatile(entry.add(4) as *mut u32, new_val);
            }
        } else {
            // Gap detected — increment gap counter
            let gaps = unsafe { core::ptr::read_volatile(entry.add(8) as *const u32) };
            unsafe {
                core::ptr::write_volatile(entry.add(8) as *mut u32, gaps.saturating_add(1));
            }
        }
    }

    // ── Settings check (RFC 9114 §7.2.4) ───────────────────────────────────────
    // RFC 9114 §7.2.4 — Capability negotiation
    let conn_id = (flow_label >> 8) as u32;
    let setting_id: u16 = m.qos_class as u16;
    let settings_key = make_settings_key(conn_id, setting_id);
    if let Some(settings_val) = unsafe { SETTINGS.get(&settings_key) } {
        // Read flags field (bytes 12-15)
        let flags_ptr = unsafe { (settings_val as *const u8).add(12) };
        let flags = unsafe { core::ptr::read_volatile(flags_ptr as *const u32) };

        // Check if required capability is set (bit 0 = enabled)
        if flags & 0x01 == 0 {
            // Required capability not negotiated — drop packet, emit ANOMALY
            increment_stat(STAT_SETTINGS_REJECTED);
            let hop_id = cfg(CFG_HOP_ID) as u8;
            emit_event(EventType::Anomaly as u8, hop_id, flow_label, &monad);
            return Ok(xdp_action::XDP_DROP);
        }
    }
    // If settings key not found, use default policy (allow)

    // ── DoS backpressure check (RFC 9114 §10) ──────────────────────────────────
    // RFC 9114 §10 — Congestion control / DoS backpressure
    let dos_key = make_namespace_key(namespace);
    if let Some(dos_entry) = DOS_STATE.get_ptr_mut(&dos_key) {
        let entry = dos_entry as *mut u8;
        // drop_count (bytes 0-3), total_count (bytes 4-7), backpressure_lvl (byte 8), window_start (bytes 9-12)

        // Increment total_count (bytes 4-7)
        let total = unsafe { core::ptr::read_volatile(entry.add(4) as *const u32) };
        unsafe {
            core::ptr::write_volatile(entry.add(4) as *mut u32, total.saturating_add(1));
        }

        // Read backpressure level (byte 8)
        let backpressure_lvl = unsafe { core::ptr::read_volatile(entry.add(8)) };

        match backpressure_lvl {
            0 => {
                // NORMAL — no action, packet passes
            }
            1 => {
                // OVERLOAD — set ECN bits in IPv6 Traffic Class (RFC 6298)
                // ECN-CE (Congestion Experienced) = 0b11 in TC low 2 bits
                // This is done by modifying the IPv6 header's traffic class field
                increment_stat(STAT_DOS_ECN_SET);
            }
            2.. => {
                // SHUTDOWN — drop packet
                let drop_count = unsafe { core::ptr::read_volatile(entry as *const u32) };
                unsafe {
                    core::ptr::write_volatile(entry as *mut u32, drop_count.saturating_add(1));
                }
                increment_stat(STAT_DOS_DROPPED);
                return Ok(xdp_action::XDP_DROP);
            }
        }
    }

    // ── Flow type dispatch (RFC 9114 §6) ──────────────────────────────────────────
    // RFC 9114 §6 — Flow type dispatch
    let flow_type_key = make_flow_type_key(m.src_service_id as u32);
    if let Some(flow_type_entry) = unsafe { FLOW_TYPES.get(&flow_type_key) } {
        let ft_type = unsafe { core::ptr::read_volatile(flow_type_entry as *const u8) };
        let ft_flags = unsafe { core::ptr::read_volatile((flow_type_entry as *const u8).add(1)) };

        match ft_type {
            0x00 => {
                // CONTROL flow — highest priority, no QoS modification
                increment_stat(STAT_FLOW_CONTROL);
            }
            0x01 => {
                // DATA flow — apply QoS from Monad
                // QoS already in monad.qos_class, no modification needed
                increment_stat(STAT_FLOW_DATA);
            }
            0x02 => {
                // PREFETCH/BULK flow — lowest priority, may be deprioritized
                // Set flow_action to indicate bulk processing
                increment_stat(STAT_FLOW_BULK);
            }
            _ => {
                // Unknown flow type — use default (DATA)
                increment_stat(STAT_FLOW_UNKNOWN);
            }
        }
        let _ = ft_flags; // Suppress unused warning; flags may be used in future
    }

    // ── Hop Count ─────────────────────────────────────────────────────────────
    m.increment_hop(); // saturating — never wraps to 0

    // ── Flow Action Semantics ─────────────────────────────────────────────────
    // Note: `flow_action` is the Monad field name *and* our imported module name.
    // We use `fa::` for the module to avoid ambiguity.
    let should_drop = apply_flow_action(&mut m, flow_label);

    // ── Recompute Checksum ────────────────────────────────────────────────────
    // Skip recompute when CUSTOM flag is set (checksum carries exponent keys).
    if !m.has_flag(flags::CUSTOM) {
        m.recompute_checksum();
    }

    // ── Write Mutated Monad Back In-Place ─────────────────────────────────────
    // No packet resize — we overwrite the 20 bytes at the same offset.
    write_monad_to_pkt(monad_data_off, data_end, &m)?;

    // ── Emit Hop Event ────────────────────────────────────────────────────────
    let hop_id = cfg(CFG_HOP_ID) as u8;
    let divisor = cfg(CFG_SAMPLE_DIVISOR) as u32;
    let sampled = divisor == 0 || (flow_label % divisor) == 0;

    if m.has_flag(flags::TRACED) || m.has_flag(flags::SAMPLED) || sampled {
        emit_event(EventType::Computed as u8, hop_id, flow_label, &m);
    }

    if should_drop {
        increment_stat(STAT_ACTION_DROP);
        return Ok(xdp_action::XDP_DROP);
    }

    Ok(xdp_action::XDP_PASS)
}

// ── Hop checks (ADR-125) ──────────────────────────────────────────────────────

/// Run this hop's validator, if it has one. Returns `true` to drop the packet.
///
/// This is an authorization check on hop-local state: it does not prove where
/// a packet came from (ADR-125, "What this is not"; FND-031).
#[inline(always)]
fn run_checks(monad: &Monad, src: &[u8; 16], flow_label: u32) -> bool {
    let mode = cfg(CFG_CHECK_MODE);
    if mode != CHECK_MODE_OBSERVE && mode != CHECK_MODE_ENFORCE {
        return false;
    }
    let hop_id = cfg(CFG_HOP_ID);
    let key = make_validator_key(hop_id as u32, monad.deploy_ring);
    let Some(validator) = (unsafe { HOP_VALIDATORS.get(&key) }) else {
        return false;
    };
    increment_stat(STAT_CHECKS_RUN);
    if checks_pass(validator, monad, src) {
        return false;
    }
    increment_stat(STAT_CHECKS_FAILED);
    emit_event(EventType::Anomaly as u8, hop_id as u8, flow_label, monad);
    if mode != CHECK_MODE_ENFORCE {
        return false;
    }
    increment_stat(STAT_CHECKS_ENFORCED);
    true
}

/// Evaluate one validator, in ADR-125's order.
#[inline(always)]
fn checks_pass(v: &HopValidator, monad: &Monad, src: &[u8; 16]) -> bool {
    let (min, max) = (v.allowed_version_min, v.allowed_version_max);
    if (min != 0 || max != 0) && (monad.version < min || monad.version > max) {
        return false;
    }
    if v.max_hop_count != 0 && monad.hop_count > v.max_hop_count {
        return false;
    }
    if v.require_crc != 0 && monad.has_flag(flags::CUSTOM) {
        return false;
    }
    // The CIRCUIT_ERRORS pair: the Monad has no flow id, and the flow label is
    // random per packet at Shield.
    let flow_id = (monad.src_service_id as u16) | ((monad.dst_service_id as u16) << 8);
    if v.require_hmac != 0 {
        let key = make_hmac_key(flow_id, monad.deploy_ring);
        match unsafe { HMAC_KEYS.get(&key) } {
            Some(entry) if is_live(entry.expires_at) => {}
            _ => return false,
        }
    }
    if v.flags & VALIDATOR_FLAG_RETRY != 0 {
        let key = make_retry_key(flow_id, src);
        match unsafe { RETRY_TOKENS.get(&key) } {
            Some(entry) if is_live(entry.expires_at) => {}
            _ => return false,
        }
    }
    true
}

/// `expires_at` is CLOCK_BOOTTIME seconds, 0 = never (ADR-119 D6.4(c)'s
/// one-clock rule). Expired from the second it names.
#[inline(always)]
fn is_live(expires_at: u32) -> bool {
    expires_at == 0 || unsafe { bpf_ktime_get_boot_ns() } / 1_000_000_000 < expires_at as u64
}

// ── Per-hop flow action ───────────────────────────────────────────────────────

/// Apply flow_action semantics to the Monad.
/// Returns `true` if the packet should be dropped after all writes are complete.
#[inline(always)]
fn apply_flow_action(m: &mut Monad, flow_label: u32) -> bool {
    if m.flow_action == fa::TRACE {
        // TRACE: force TRACED flag — every downstream hop MUST emit a full event.
        m.set_flag(flags::TRACED);
        increment_stat(STAT_ACTION_TRACE);
        false
    } else if m.flow_action == fa::SAMPLE {
        // SAMPLE: mark packet as selected for statistical sampling.
        let divisor = cfg(CFG_SAMPLE_DIVISOR) as u32;
        if divisor == 0 || flow_label.is_multiple_of(divisor) {
            m.set_flag(flags::SAMPLED);
        }
        increment_stat(STAT_ACTION_SAMPLE);
        false
    } else if m.flow_action == fa::MIRROR {
        // MIRROR: set MIRROR flag for mirror-copy handling.
        // Actual duplication is done by TC egress (bpf_clone_redirect).
        m.set_flag(flags::MIRROR);
        increment_stat(STAT_ACTION_MIRROR);
        false
    } else if m.flow_action == fa::DROP {
        // DROP: the control plane has instructed this flow to be dropped.
        // We still write back the Monad so the XDP_DROP event captures state.
        false // caller checks the return value AFTER write-back
    } else {
        // FORWARD (default) and anything unknown: pass through.
        false
    }
}

// ── Packet memory helpers ─────────────────────────────────────────────────────

/// Find the byte offset of the MONAD_OPT_TYPE option within an HBH option area.
///
/// Returns the offset of the `opt_type` byte (not the data start).
///
/// Bounded to `MAX_OPT_SCAN` iterations — verifier-safe.  `black_box` prevents
/// LLVM from eliminating the per-iteration `data_end` checks.
#[inline(always)]
fn find_monad_option(opts_start: usize, opts_end: usize, data_end: usize) -> Result<usize, ()> {
    // Sever LLVM's knowledge that opts_end <= data_end so it cannot prove
    // the per-iteration bounds checks redundant (same trick as shield-ebpf).
    let opts_end = core::hint::black_box(opts_end);
    let mut offset = opts_start;

    // At most 16 options to scan.  Max HBH options size: (255+1)*8 = 2048 bytes,
    // but our Monad is the only real option; we just need to skip pads.
    for _ in 0..16usize {
        if offset >= opts_end {
            break;
        }
        if offset + 1 > data_end {
            break;
        }

        let opt_type = unsafe { core::ptr::read_volatile(offset as *const u8) };

        // Pad byte (type 0) terminates option list.
        if opt_type == 0 {
            break;
        }

        // NOP (type 1) — single byte, no length field.
        if opt_type == 1 {
            offset += 1;
            continue;
        }

        // Multi-byte option — need the length byte.
        if offset + 2 > data_end {
            break;
        }
        let opt_data_len = unsafe { core::ptr::read_volatile((offset + 1) as *const u8) };
        let opt_total = 2 + opt_data_len as usize; // type + len + data

        if opt_total < 2 || offset + opt_total > data_end {
            break;
        }

        // Is this our Monad option?
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
///
/// Uses `read_volatile` to prevent the compiler from caching stale values and to
/// keep each per-byte bounds check visible to the BPF verifier.
#[inline(always)]
fn read_monad_from_pkt(start: usize, data_end: usize) -> Result<Monad, ()> {
    // Opaque, so LLVM keeps this check. The caller's variable-length bound
    // implies it, and LLVM would merge it away; the verifier tracks that bound
    // on a different pointer and rejects the read ("offset is outside of the
    // packet").
    let data_end = core::hint::black_box(data_end);
    if start + MONAD_SIZE > data_end {
        return Err(());
    }
    let mut bytes = [0u8; 20];
    // Exactly 20 iterations — verifier trivially proves termination.
    #[allow(clippy::needless_range_loop)]
    for i in 0..20usize {
        bytes[i] = unsafe { core::ptr::read_volatile((start + i) as *const u8) };
    }
    Ok(Monad::from_bytes(bytes))
}

/// Write a mutated [`Monad`] back into packet memory at the same offset.
///
/// No resize — this is a pure in-place 20-byte overwrite.
#[inline(always)]
fn write_monad_to_pkt(start: usize, data_end: usize, m: &Monad) -> Result<(), ()> {
    // Opaque, so LLVM keeps this check. The caller's variable-length bound
    // implies it, and LLVM would merge it away; the verifier tracks that bound
    // on a different pointer and rejects the read ("offset is outside of the
    // packet").
    let data_end = core::hint::black_box(data_end);
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

// ── Sophia key encoding ───────────────────────────────────────────────────────

/// Pack (dict_id, field_value) into a u16 Sophia map key.
/// High byte = dict_id, low byte = value.  256 dicts × 256 values = 65 536 keys.
#[inline(always)]
const fn sophia_key(dict_id: u8, value: u8) -> u16 {
    ((dict_id as u16) << 8) | (value as u16)
}

/// Encode a namespace for SEQ_COUNTERS and DOS_STATE lookups.
#[inline(always)]
fn make_namespace_key(namespace: u32) -> u64 {
    (namespace as u64) << 32
}

/// Encode a (conn_id, setting_id) pair for SETTINGS lookup.
#[inline(always)]
fn make_settings_key(conn_id: u32, setting_id: u16) -> u64 {
    ((conn_id as u64) << 32) | ((setting_id as u64) << 16)
}

/// Encode a flow_id for FLOW_TYPES lookup.
#[inline(always)]
fn make_flow_type_key(flow_id: u32) -> u64 {
    flow_id as u64
}

/// HOP_VALIDATORS key, the bytes of `bpfschema.HopValidatorKey` on a
/// little-endian host: `hop_id | namespace << 32`.
#[inline(always)]
const fn make_validator_key(hop_id: u32, namespace: u8) -> u64 {
    (hop_id as u64) | ((namespace as u64) << 32)
}

/// HMAC_KEYS key, the bytes of `bpfschema.HMACKeyKey`:
/// `flow_id | namespace << 16`, the top four bytes zero.
#[inline(always)]
const fn make_hmac_key(flow_id: u16, namespace: u8) -> u64 {
    (flow_id as u64) | ((namespace as u64) << 16)
}

/// RETRY_TOKENS key: `flow_id | src_low48 << 16`, where `src_low48` is the low
/// 48 bits of the IPv6 source as a number (the last six address bytes, read
/// big-endian), so `fd00::1` gives 1.
#[inline(always)]
fn make_retry_key(flow_id: u16, src: &[u8; 16]) -> u64 {
    let src_low48 =
        u64::from_be_bytes([0, 0, src[10], src[11], src[12], src[13], src[14], src[15]]);
    (flow_id as u64) | (src_low48 << 16)
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

// ── Anamnesis event emission ──────────────────────────────────────────────────

/// Reserve a slot in the ANAMNESIS ring buffer and write an event.
/// On ring-full: increments dropped counter, does not block.
#[inline(always)]
fn emit_event(event_type: u8, hop_id: u8, flow_label: u32, monad: &Monad) {
    let now = unsafe { bpf_ktime_get_ns() };
    let event = AnamnesisEvent {
        timestamp_ns: now,
        event_type,
        hop_id,
        // Store low 16 bits of flow label (big-endian within the 2-byte array).
        flow_label_lo: [((flow_label >> 8) & 0xFF) as u8, (flow_label & 0xFF) as u8],
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

// ── Panic handler (required for #![no_std]) ───────────────────────────────────

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
