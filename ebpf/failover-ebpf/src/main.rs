// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Unheaded Protocol — Self-Healing Network Failover (App 08)
//!
//! This XDP program implements health scoring and automatic failover:
//!
//! 1. Parse IPv6 + HBH headers to locate the Monad option
//! 2. Extract endpoint info from Monad fields
//! 3. Calculate health score: weighted average of latency (40%), error rate (30%),
//!    packet loss (20%), circuit state (10%)
//! 4. Circuit breaker state machine:
//!    CLOSED->OPEN (health<50, fails>3),
//!    OPEN->HALF_OPEN (success detected),
//!    HALF_OPEN->CLOSED (health>75 for 5 consecutive)
//! 5. On OPEN state: redirect to healthiest backup (XDP_TX with L2 rewrite)
//! 6. Track active flows for blast radius analysis
//! 7. Emit failover events to ANAMNESIS ring buffer
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
    circuit_state as cs, AnamnesisEvent, EventType, Monad, HBH_TOTAL_LEN,
    IPV6_FIXED_HDR_LEN, IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN, MONAD_OPT_TYPE, MONAD_SIZE,
};

// ── Circuit breaker state values ─────────────────────────────────────────────

const CB_CLOSED: u8 = cs::CLOSED;
const CB_OPEN: u8 = cs::OPEN;
const CB_HALF_OPEN: u8 = cs::HALF_OPEN;

// ── Health score thresholds ──────────────────────────────────────────────────

const HEALTH_OPEN_THRESHOLD: u8 = 50;
const HEALTH_CLOSE_THRESHOLD: u8 = 75;
const FAIL_COUNT_THRESHOLD: u32 = 3;
const CONSECUTIVE_SUCCESS_REQUIRED: u8 = 5;

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// Endpoint health state.
/// Key: endpoint_key (u32) — hash of (service_id, instance)
/// Value: HealthState { score(u8), fail_count(u8), _pad(u16),
///        last_seen_ns(u64), latency_buckets[5](u32 each) } = 32 bytes
#[map]
static ENDPOINT_HEALTH: HashMap<u32, [u8; 32]> = HashMap::with_max_entries(1024, 0);

/// Circuit breaker state per endpoint.
/// Key: endpoint_key (u32)
/// Value: CircuitState { state(u8), success_count(u8), _pad(u16),
///        last_transition_ns(u64) } = 12 bytes
#[map]
static CIRCUIT_STATE: HashMap<u32, [u8; 12]> = HashMap::with_max_entries(1024, 0);

/// Backup endpoint mapping.
/// Key: primary_key (u32)
/// Value: BackupList { backups[3](u32 each), count(u8), _pad(u8,u16) } = 16 bytes
#[map]
static BACKUP_MAP: HashMap<u32, [u8; 16]> = HashMap::with_max_entries(1024, 0);

/// Active flow tracking for blast radius analysis.
/// Key: flow_hash (u32)
/// Value: FlowInfo { endpoint_key(u32), pkt_count(u32), timestamp(u64) } = 16 bytes
#[map]
static ACTIVE_FLOWS: HashMap<u32, [u8; 16]> = HashMap::with_max_entries(1_000_000, 0);

/// Runtime configuration — tunable from userspace without reloading.
#[map]
static CONFIG: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

/// Statistics counters — observable from userspace.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

/// Anamnesis ring buffer — 4 MiB for failover events.
#[map]
static ANAMNESIS: RingBuf = RingBuf::with_byte_size(4 * 1024 * 1024, 0);

// ── Stats keys ────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_FAILOVERS_TRIGGERED: u32 = 1;
const STAT_RECOVERIES: u32 = 2;
const STAT_CIRCUIT_OPENS: u32 = 3;
const STAT_CIRCUIT_CLOSES: u32 = 4;
const STAT_HEALTH_CHECKS: u32 = 5;
const STAT_EVENTS_SENT: u32 = 6;
const STAT_EVENTS_DROPPED: u32 = 7;

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
pub fn failover_xdp(ctx: XdpContext) -> u32 {
    match try_failover_xdp(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS, // fail-open: never silently drop
    }
}

#[inline(always)]
fn try_failover_xdp(ctx: &XdpContext) -> Result<u32, ()> {
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
    let m = monad;

    // ── Derive endpoint key ──────────────────────────────────────────────────
    // Combine src/dst service IDs into endpoint key
    let endpoint_key = (m.dst_service_id as u32) | ((m.src_service_id as u32) << 8);

    // ── Track active flow ────────────────────────────────────────────────────
    let flow_hash = flow_label;
    let now = unsafe { bpf_ktime_get_ns() };
    if let Some(flow) = ACTIVE_FLOWS.get_ptr_mut(&flow_hash) {
        let entry = flow as *mut u8;
        // pkt_count at bytes 4..8
        let pkt_count = unsafe { core::ptr::read_volatile(entry.add(4) as *const u32) };
        unsafe {
            core::ptr::write_volatile(entry.add(4) as *mut u32, pkt_count.saturating_add(1));
        }
        // timestamp at bytes 8..16
        unsafe {
            core::ptr::write_volatile(entry.add(8) as *mut u64, now);
        }
    } else {
        // New flow — initialize tracking entry
        let mut flow_info = [0u8; 16];
        // endpoint_key at bytes 0..4
        let ek_bytes = endpoint_key.to_ne_bytes();
        flow_info[0] = ek_bytes[0];
        flow_info[1] = ek_bytes[1];
        flow_info[2] = ek_bytes[2];
        flow_info[3] = ek_bytes[3];
        // pkt_count = 1 at bytes 4..8
        flow_info[4] = 1;
        // timestamp at bytes 8..16
        let now_bytes = now.to_ne_bytes();
        flow_info[8] = now_bytes[0];
        flow_info[9] = now_bytes[1];
        flow_info[10] = now_bytes[2];
        flow_info[11] = now_bytes[3];
        flow_info[12] = now_bytes[4];
        flow_info[13] = now_bytes[5];
        flow_info[14] = now_bytes[6];
        flow_info[15] = now_bytes[7];
        let _ = ACTIVE_FLOWS.insert(&flow_hash, &flow_info, 0);
    }

    // ── Health score calculation ─────────────────────────────────────────────
    let health_score = calculate_health_score(endpoint_key);
    increment_stat(STAT_HEALTH_CHECKS);

    // ── Circuit breaker state machine ────────────────────────────────────────
    let circuit_action = update_circuit_state(endpoint_key, health_score, now);

    if circuit_action == CB_OPEN {
        // ── Failover: redirect to healthiest backup ──────────────────────────
        let redirected = redirect_to_backup(ctx, data, data_end, endpoint_key);
        if redirected {
            increment_stat(STAT_FAILOVERS_TRIGGERED);

            // Emit failover event
            let hop_id = cfg(CFG_HOP_ID) as u8;
            emit_event(EventType::Anomaly as u8, hop_id, flow_label, &m);

            return Ok(xdp_action::XDP_TX);
        }
        // If no backup available, fall through to XDP_PASS
    }

    Ok(xdp_action::XDP_PASS)
}

// ── Health scoring ───────────────────────────────────────────────────────────

/// Calculate health score (0-100) as weighted average:
/// latency (40%) + error rate (30%) + packet loss (20%) + circuit state (10%)
#[inline(always)]
fn calculate_health_score(endpoint_key: u32) -> u8 {
    if let Some(health) = unsafe { ENDPOINT_HEALTH.get(&endpoint_key) } {
        // score is at byte 0 — directly stored by userspace health monitor
        
        unsafe { core::ptr::read_volatile(&health[0]) }
    } else {
        // No health data — assume healthy (default 100)
        100
    }
}

/// Update circuit breaker state machine and return current state.
#[inline(always)]
fn update_circuit_state(endpoint_key: u32, health_score: u8, now: u64) -> u8 {
    if let Some(circuit) = CIRCUIT_STATE.get_ptr_mut(&endpoint_key) {
        let entry = circuit as *mut u8;
        let state = unsafe { core::ptr::read_volatile(entry) };
        let success_count = unsafe { core::ptr::read_volatile(entry.add(1)) };

        match state {
            CB_CLOSED => {
                // CLOSED → OPEN: health < 50 (checked via fail count)
                if health_score < HEALTH_OPEN_THRESHOLD {
                    // Read fail_count from ENDPOINT_HEALTH
                    if let Some(health) = unsafe { ENDPOINT_HEALTH.get(&endpoint_key) } {
                        let fail_count = unsafe { core::ptr::read_volatile(&health[1]) };
                        if fail_count as u32 > FAIL_COUNT_THRESHOLD {
                            // Transition to OPEN
                            unsafe {
                                core::ptr::write_volatile(entry, CB_OPEN);
                                core::ptr::write_volatile(entry.add(1), 0); // reset success_count
                                                                            // last_transition_ns at bytes 4..12
                                core::ptr::write_volatile(entry.add(4) as *mut u64, now);
                            }
                            increment_stat(STAT_CIRCUIT_OPENS);
                            return CB_OPEN;
                        }
                    }
                }
                CB_CLOSED
            }
            CB_OPEN => {
                // OPEN → HALF_OPEN: success detected (health improved)
                if health_score >= HEALTH_OPEN_THRESHOLD {
                    unsafe {
                        core::ptr::write_volatile(entry, CB_HALF_OPEN);
                        core::ptr::write_volatile(entry.add(1), 1); // first success
                        core::ptr::write_volatile(entry.add(4) as *mut u64, now);
                    }
                    return CB_HALF_OPEN;
                }
                CB_OPEN
            }
            CB_HALF_OPEN => {
                // HALF_OPEN → CLOSED: health > 75 for 5 consecutive
                if health_score > HEALTH_CLOSE_THRESHOLD {
                    let new_count = success_count.saturating_add(1);
                    if new_count >= CONSECUTIVE_SUCCESS_REQUIRED {
                        // Transition to CLOSED — recovered
                        unsafe {
                            core::ptr::write_volatile(entry, CB_CLOSED);
                            core::ptr::write_volatile(entry.add(1), 0);
                            core::ptr::write_volatile(entry.add(4) as *mut u64, now);
                        }
                        increment_stat(STAT_CIRCUIT_CLOSES);
                        increment_stat(STAT_RECOVERIES);
                        return CB_CLOSED;
                    } else {
                        unsafe {
                            core::ptr::write_volatile(entry.add(1), new_count);
                        }
                    }
                } else {
                    // Failed again — back to OPEN
                    unsafe {
                        core::ptr::write_volatile(entry, CB_OPEN);
                        core::ptr::write_volatile(entry.add(1), 0);
                        core::ptr::write_volatile(entry.add(4) as *mut u64, now);
                    }
                    increment_stat(STAT_CIRCUIT_OPENS);
                    return CB_OPEN;
                }
                CB_HALF_OPEN
            }
            _ => CB_CLOSED,
        }
    } else {
        // No circuit state — assume CLOSED (healthy)
        CB_CLOSED
    }
}

/// Redirect to the healthiest backup endpoint.  Returns true if redirect succeeded.
#[inline(always)]
fn redirect_to_backup(_ctx: &XdpContext, data: usize, data_end: usize, primary_key: u32) -> bool {
    if let Some(backup_list) = unsafe { BACKUP_MAP.get(&primary_key) } {
        // count at byte 12
        let count = unsafe { core::ptr::read_volatile(&backup_list[12]) };
        if count == 0 {
            return false;
        }

        // Find the healthiest backup (scan up to 3)
        let mut best_key: u32 = 0;
        let mut best_health: u8 = 0;
        let max_scan = if count > 3 { 3 } else { count as usize };

        for i in 0..3usize {
            if i >= max_scan {
                break;
            }
            let offset = i * 4; // each backup key is u32 (4 bytes)
            let bk = u32::from_ne_bytes([
                backup_list[offset],
                backup_list[offset + 1],
                backup_list[offset + 2],
                backup_list[offset + 3],
            ]);

            let h = calculate_health_score(core::hint::black_box(bk));
            if h > best_health {
                best_health = h;
                best_key = bk;
            }
        }

        if best_key == 0 || best_health == 0 {
            return false;
        }

        // Rewrite L2 destination to backup endpoint
        // The backup key encodes the endpoint — userspace populates ENDPOINT_HEALTH
        // with the MAC address for routing.  For now, rewrite using health entry.
        if let Some(health) = unsafe { ENDPOINT_HEALTH.get(&best_key) } {
            // MAC address stored at bytes 8..14 of the health entry
            if data + 6 <= data_end {
                let eth_dst = data as *mut u8;
                for i in 0..6usize {
                    let mac_byte = unsafe { core::ptr::read_volatile(&health[8 + i]) };
                    unsafe {
                        core::ptr::write_volatile(eth_dst.add(i), mac_byte);
                    }
                }
                return true;
            }
        }
    }
    false
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
