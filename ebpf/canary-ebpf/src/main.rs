// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//! Unheaded Protocol — Canary Deployment Routing (App 05)
//!
//! This XDP program implements canary traffic splitting and metrics collection:
//!
//! 1. Parse IPv6 + HBH headers to locate the Monad option
//! 2. Read dst_service_id to identify the target service
//! 3. Look up canary state for the service (PROBING/RAMP/STABLE/ROLLBACK)
//! 4. Random number (bpf_get_prandom_u32) < traffic_split_pct → route to canary
//! 5. Rewrite L2 headers to route to canary or stable endpoint
//! 6. Set CANARY flag (0x40) in Monad flags for canary traffic
//! 7. Collect per-version metrics (error rate, latency)
//! 8. Circuit breaker: if canary error rate exceeds SLO → set state to ROLLBACK
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
    helpers::{bpf_get_prandom_u32, bpf_ktime_get_ns},
    macros::{map, xdp},
    maps::{HashMap, RingBuf},
    programs::XdpContext,
};
use monad_common::{
    flags, AnamnesisEvent, EventType, Monad, HBH_TOTAL_LEN, IPV6_FIXED_HDR_LEN,
    IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN, MONAD_OPT_TYPE, MONAD_SIZE,
};

// ── Canary state machine values ─────────────────────────────────────────────

const CANARY_PROBING: u8 = 0x01;
const CANARY_RAMP: u8 = 0x02;
const CANARY_STABLE: u8 = 0x03;
const CANARY_ROLLBACK: u8 = 0x04;

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// Canary state per service.
/// Key: service_id (u32)
/// Value: CanaryState { canary_version(u8), stable_version(u8), traffic_split_pct(u8),
///        state(u8), probe_start_ms(u32) } = 8 bytes
#[map]
static CANARY_STATE: HashMap<u32, [u8; 8]> = HashMap::with_max_entries(256, 0);

/// Per-version metrics.
/// Key: (service_id: u16, version: u16) packed as u32
/// Value: Metrics { errors(u32), req_count(u32), latency_sum(u64), lat_p99(u32),
///        last_update(u32) } = 24 bytes
#[map]
static CANARY_METRICS: HashMap<u32, [u8; 24]> = HashMap::with_max_entries(512, 0);

/// Version → endpoint mapping.
/// Key: (service_id: u16, version: u16) packed as u32
/// Value: { dst_ip[16], dst_port(u16), mac_dst[6] } = 24 bytes
#[map]
static VERSION_ENDPOINTS: HashMap<u32, [u8; 24]> = HashMap::with_max_entries(512, 0);

/// SLO thresholds per service.
/// Key: service_id (u32)
/// Value: { max_error_rate_ppm(u32), max_latency_p99_ms(u32), min_throughput_pps(u32) }
///        = 12 bytes
#[map]
static SLO_THRESHOLDS: HashMap<u32, [u8; 12]> = HashMap::with_max_entries(256, 0);

/// Runtime configuration — tunable from userspace without reloading.
#[map]
static CONFIG: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

/// Statistics counters — observable from userspace.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

/// Anamnesis ring buffer — 4 MiB for canary events.
#[map]
static ANAMNESIS: RingBuf = RingBuf::with_byte_size(4 * 1024 * 1024, 0);

// ── Stats keys ────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_CANARY_ROUTED: u32 = 1;
const STAT_STABLE_ROUTED: u32 = 2;
const STAT_ROLLBACKS: u32 = 3;
const STAT_SLO_BREACHES: u32 = 4;
const STAT_EVENTS_SENT: u32 = 5;
const STAT_EVENTS_DROPPED: u32 = 6;

// ── Config keys ──────────────────────────────────────────────────────────────
const CFG_HOP_ID: u32 = 0;

// ── Wire-format helpers ──────────────────────────────────────────────────────

#[repr(C, packed)]
struct EthHdr {
    dst: [u8; 6],
    src: [u8; 6],
    proto: u16,
}

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

// ── XDP entry point ──────────────────────────────────────────────────────────

#[xdp]
pub fn canary_xdp(ctx: XdpContext) -> u32 {
    match try_canary_xdp(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS, // fail-open: never silently drop
    }
}

#[inline(always)]
fn try_canary_xdp(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_PACKETS_TOTAL);

    let data = ctx.data();
    let data_end = ctx.data_end();

    // ── Ethernet ─────────────────────────────────────────────────────────────
    if data + ETH_HLEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let eth = unsafe { &*(data as *const EthHdr) };
    if u16::from_be(eth.proto) != ETH_P_IPV6 {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── IPv6 Fixed Header ────────────────────────────────────────────────────
    let ip_start = data + ETH_HLEN;
    if ip_start + IPV6_FIXED_HDR_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let ip = unsafe { &*(ip_start as *const Ipv6Hdr) };

    if ip.next_header != IPV6_NEXTHDR_HBH {
        return Ok(xdp_action::XDP_PASS);
    }

    // Extract Flow Label from the packed vtf field (low 20 bits).
    let vtf = u32::from_be(unsafe { core::ptr::read_unaligned(core::ptr::addr_of!(ip.vtf)) });
    let flow_label = vtf & 0x000F_FFFF;

    // ── Hop-by-Hop Header ────────────────────────────────────────────────────
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

    // ── Find Monad option ────────────────────────────────────────────────────
    let monad_opt_off = find_monad_option(opts_start, opts_end, data_end)?;

    let monad_data_off = monad_opt_off + 2;
    if monad_data_off + MONAD_SIZE > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Read Monad ───────────────────────────────────────────────────────────
    let monad = read_monad_from_pkt(monad_data_off, data_end)?;

    // ── Canary routing decision ──────────────────────────────────────────────
    let service_id = monad.dst_service_id as u32;
    let mut m = monad;

    if let Some(state) = unsafe { CANARY_STATE.get(&service_id) } {
        let canary_version = unsafe { core::ptr::read_volatile(&state[0]) };
        let stable_version = unsafe { core::ptr::read_volatile(&state[1]) };
        let traffic_split_pct = unsafe { core::ptr::read_volatile(&state[2]) };
        let canary_state = unsafe { core::ptr::read_volatile(&state[3]) };

        // On ROLLBACK state, send everything to stable
        if canary_state == CANARY_ROLLBACK {
            increment_stat(STAT_ROLLBACKS);
            route_to_version(ctx, data, data_end, service_id, stable_version);
            increment_stat(STAT_STABLE_ROUTED);
        } else {
            // Random split: bpf_get_prandom_u32() mod 100 < traffic_split_pct → canary
            let rand = unsafe { bpf_get_prandom_u32() } % 100;
            let is_canary = rand < traffic_split_pct as u32;

            if is_canary {
                // Route to canary endpoint
                route_to_version(ctx, data, data_end, service_id, canary_version);
                // Set CANARY flag (0x40) in Monad flags
                m.set_flag(flags::CANARY);
                increment_stat(STAT_CANARY_ROUTED);
            } else {
                // Route to stable endpoint
                route_to_version(ctx, data, data_end, service_id, stable_version);
                increment_stat(STAT_STABLE_ROUTED);
            }

            // ── Per-version metrics update ───────────────────────────────────
            let version = if is_canary {
                canary_version
            } else {
                stable_version
            };
            update_metrics(service_id, version, &m);

            // ── SLO circuit breaker ──────────────────────────────────────────
            // Check canary error rate against SLO thresholds
            if is_canary {
                check_slo_breach(service_id, canary_version);
            }
        }
    } else {
        // No canary state for this service — pass through as stable
        increment_stat(STAT_STABLE_ROUTED);
    }

    // ── Recompute checksum and write back ────────────────────────────────────
    if !m.has_flag(flags::CUSTOM) {
        m.recompute_checksum();
    }
    write_monad_to_pkt(monad_data_off, data_end, &m)?;

    // ── Emit canary event ────────────────────────────────────────────────────
    let hop_id = cfg(CFG_HOP_ID) as u8;
    if m.has_flag(flags::TRACED) || m.has_flag(flags::CANARY) {
        emit_event(EventType::Hop as u8, hop_id, flow_label, &m);
    }

    Ok(xdp_action::XDP_TX)
}

// ── Canary routing helpers ────────────────────────────────────────────────────

/// Rewrite L2 headers to route to the endpoint for the given version.
#[inline(always)]
fn route_to_version(
    _ctx: &XdpContext,
    data: usize,
    data_end: usize,
    service_id: u32,
    version: u8,
) {
    let key = ((service_id as u32) & 0xFFFF) | ((version as u32) << 16);
    if let Some(endpoint) = unsafe { VERSION_ENDPOINTS.get(&key) } {
        // endpoint layout: dst_ip[16], dst_port[2], mac_dst[6]
        // Rewrite Ethernet destination MAC (bytes 18..24 of endpoint)
        if data + 6 <= data_end {
            let eth_dst = data as *mut u8;
            for i in 0..6usize {
                let mac_byte =
                    unsafe { core::ptr::read_volatile(&endpoint[18 + i]) };
                unsafe {
                    core::ptr::write_volatile(eth_dst.add(i), mac_byte);
                }
            }
        }
    }
}

/// Update per-version metrics (request count, latency, errors).
#[inline(always)]
fn update_metrics(service_id: u32, version: u8, monad: &Monad) {
    let key = ((service_id) & 0xFFFF) | ((version as u32) << 16);
    if let Some(metrics) = CANARY_METRICS.get_ptr_mut(&key) {
        let entry = metrics as *mut u8;
        // req_count at bytes 4..8 — increment
        let req_count = unsafe { core::ptr::read_volatile(entry.add(4) as *const u32) };
        unsafe {
            core::ptr::write_volatile(entry.add(4) as *mut u32, req_count.saturating_add(1));
        }

        // latency_sum at bytes 8..16 — add current latency hint
        let latency_sum = unsafe { core::ptr::read_volatile(entry.add(8) as *const u64) };
        let latency = monad.latency_hint_u16() as u64;
        unsafe {
            core::ptr::write_volatile(
                entry.add(8) as *mut u64,
                latency_sum.saturating_add(latency),
            );
        }

        // last_update at bytes 20..24 — set current time
        let now = unsafe { bpf_ktime_get_ns() };
        let now_ms = (now / 1_000_000) as u32;
        unsafe {
            core::ptr::write_volatile(entry.add(20) as *mut u32, now_ms);
        }
    }
}

/// Check if the canary error rate exceeds SLO thresholds.
/// If so, set CANARY_STATE to ROLLBACK.
#[inline(always)]
fn check_slo_breach(service_id: u32, canary_version: u8) {
    let metrics_key = ((service_id) & 0xFFFF) | ((canary_version as u32) << 16);
    if let Some(metrics) = unsafe { CANARY_METRICS.get(&metrics_key) } {
        // errors at bytes 0..4, req_count at bytes 4..8
        let errors = u32::from_ne_bytes([metrics[0], metrics[1], metrics[2], metrics[3]]);
        let req_count = u32::from_ne_bytes([metrics[4], metrics[5], metrics[6], metrics[7]]);

        if req_count == 0 {
            return;
        }

        // Error rate in PPM (parts per million)
        let error_rate_ppm = (errors as u64 * 1_000_000) / req_count as u64;

        if let Some(thresholds) = unsafe { SLO_THRESHOLDS.get(&service_id) } {
            let max_error_rate_ppm = u32::from_ne_bytes([
                thresholds[0],
                thresholds[1],
                thresholds[2],
                thresholds[3],
            ]);

            if error_rate_ppm > max_error_rate_ppm as u64 {
                // Set state to ROLLBACK
                if let Some(state) = CANARY_STATE.get_ptr_mut(&service_id) {
                    let entry = state as *mut u8;
                    // state field at byte 3
                    unsafe {
                        core::ptr::write_volatile(entry.add(3), CANARY_ROLLBACK);
                    }
                }
                increment_stat(STAT_SLO_BREACHES);
            }
        }
    }
}

// ── Packet memory helpers ─────────────────────────────────────────────────────

/// Find the byte offset of the MONAD_OPT_TYPE option within an HBH option area.
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
#[inline(always)]
fn emit_event(event_type: u8, hop_id: u8, flow_label: u32, monad: &Monad) {
    let now = unsafe { bpf_ktime_get_ns() };
    let event = AnamnesisEvent {
        timestamp_ns: now,
        event_type,
        hop_id,
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

// ── Panic handler (required for #![no_std]) ─────────────────────────────────

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
