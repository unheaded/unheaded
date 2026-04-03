// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Unheaded Protocol — QoS eBPF Program (App 07)
//!
//! Adaptive QoS with token bucket rate limiting and CoDel active queue management.
//!
//! 1. Parse IPv6 + HBH headers to locate the Monad option
//! 2. Verify CRC-16/CCITT — emit ANOMALY on failure, pass without mutation
//! 3. Read dst_service_id and qos_class from Monad
//! 4. Sophia lookup: get QoS policy for destination service
//! 5. Token bucket: refill tokens based on elapsed time, check if packet fits
//! 6. CoDel AQM: track sojourn time, drop if above target AND in congestion
//! 7. Increment stats per service class
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
    maps::{HashMap, LruHashMap, RingBuf},
    programs::XdpContext,
};
use monad_common::{
    flags, AnamnesisEvent, EventType, Monad, HBH_TOTAL_LEN, IPV6_FIXED_HDR_LEN,
    IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN, MONAD_OPT_TYPE, MONAD_SIZE,
};

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// QoS policy table — 256 entries.
/// Key: dst_service_id (u32), Value: QosConfig.
#[map]
static QOS_POLICY: HashMap<u32, QosConfig> = HashMap::with_max_entries(256, 0);

/// Token bucket state — 1M LRU entries.
/// Key: flow_id (u32, derived from flow hash), Value: TokenBucket.
#[map]
static TOKEN_BUCKETS: LruHashMap<u32, TokenBucket> = LruHashMap::with_max_entries(1_000_000, 0);

/// Per-service queue statistics — 256 entries.
/// Key: dst_service_id (u32), Value: QueueStats.
#[map]
static QUEUE_STATS: HashMap<u32, QueueStats> = HashMap::with_max_entries(256, 0);

/// Runtime configuration — tunable from userspace without reloading.
///
/// | Key | Meaning                                | Default |
/// |-----|----------------------------------------|---------|
/// | 0   | hop_id (this node's identifier)        | 0       |
/// | 1   | default_rate_limit_mbps                | 0 = off |
/// | 2   | default_burst_bytes                    | 0 = off |
/// | 3   | codel_target_us (target latency, us)   | 5000    |
/// | 4   | codel_interval_ns (measurement window) | 100ms   |
#[map]
static CONFIG: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

/// Statistics counters — observable from userspace.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

/// Anamnesis ring buffer — 4 MiB, shared with userspace.
#[map]
static ANAMNESIS: RingBuf = RingBuf::with_byte_size(4 * 1024 * 1024, 0);

// ── QoS types ────────────────────────────────────────────────────────────────

/// QoS configuration for a destination service.
#[repr(C)]
#[derive(Clone, Copy, Default)]
struct QosConfig {
    /// QoS scheduling class (maps to Monad qos_class).
    class: u8,
    /// Scheduling weight (higher = more bandwidth).
    weight: u8,
    /// Padding.
    _pad: [u8; 2],
    /// Rate limit in megabits per second (0 = unlimited).
    rate_limit_mbps: u32,
    /// Burst size in bytes (token bucket capacity).
    burst_bytes: u32,
    /// Target latency in microseconds (CoDel target).
    target_latency_us: u32,
}

/// Token bucket state per flow.
#[repr(C)]
#[derive(Clone, Copy, Default)]
struct TokenBucket {
    /// Current number of tokens (bytes).
    tokens: u64,
    /// Last refill timestamp (bpf_ktime_get_ns).
    last_refill_ns: u64,
}

/// Per-service queue statistics.
#[repr(C)]
#[derive(Clone, Copy, Default)]
struct QueueStats {
    /// Total packets seen for this service.
    packets: u64,
    /// Packets passed through.
    passed: u64,
    /// Packets dropped by rate limiter.
    dropped_rate: u64,
    /// Packets dropped by CoDel.
    dropped_codel: u64,
    /// Total tokens consumed.
    tokens_consumed: u64,
    /// First timestamp above CoDel target (for interval tracking).
    codel_first_above_ns: u64,
    /// CoDel drop-next timestamp.
    codel_drop_next_ns: u64,
    /// CoDel consecutive drop count (for interval scaling).
    codel_count: u32,
    /// Whether CoDel is currently in dropping state.
    codel_dropping: u8,
    /// Padding.
    _pad: [u8; 3],
}

// ── Stats keys ────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_PASSED: u32 = 1;
const STAT_DROPPED_RATE_LIMIT: u32 = 2;
const STAT_DROPPED_CODEL: u32 = 3;
const STAT_TOKENS_CONSUMED: u32 = 4;
const STAT_CRC_ERRORS: u32 = 5;
const STAT_EVENTS_SENT: u32 = 6;
const STAT_EVENTS_DROPPED: u32 = 7;
const STAT_POLICY_HITS: u32 = 8;
const STAT_POLICY_MISSES: u32 = 9;
const STAT_BUCKET_CREATED: u32 = 10;
const STAT_BUCKET_REFILLED: u32 = 11;

// ── Config keys ───────────────────────────────────────────────────────────────
const CFG_HOP_ID: u32 = 0;
const CFG_DEFAULT_RATE_LIMIT: u32 = 1;
const CFG_DEFAULT_BURST: u32 = 2;
const CFG_CODEL_TARGET_US: u32 = 3;
const CFG_CODEL_INTERVAL_NS: u32 = 4;

/// Default CoDel target: 5ms in microseconds.
const DEFAULT_CODEL_TARGET_US: u64 = 5_000;
/// Default CoDel interval: 100ms in nanoseconds.
const DEFAULT_CODEL_INTERVAL_NS: u64 = 100_000_000;
/// Nanoseconds per second — for rate conversion.
const NS_PER_SEC: u64 = 1_000_000_000;
/// Bits per megabit.
const BITS_PER_MBIT: u64 = 1_000_000;
/// Bytes per bit (division factor).
const BITS_PER_BYTE: u64 = 8;

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
    vtf: u32,
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
pub fn qos_xdp(ctx: XdpContext) -> u32 {
    match try_qos_xdp(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS, // fail-open: never silently drop
    }
}

#[inline(always)]
fn try_qos_xdp(ctx: &XdpContext) -> Result<u32, ()> {
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

    // Extract Flow Label and payload length.
    let vtf = u32::from_be(unsafe { core::ptr::read_unaligned(core::ptr::addr_of!(ip.vtf)) });
    let flow_label = vtf & 0x000F_FFFF;
    let payload_len = u16::from_be(unsafe {
        core::ptr::read_unaligned(core::ptr::addr_of!(ip.payload_len))
    }) as u64;

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

    // ── CRC Verification ──────────────────────────────────────────────────────
    if !monad.has_flag(flags::CUSTOM) && !monad.verify_checksum() {
        increment_stat(STAT_CRC_ERRORS);
        let hop_id = cfg(CFG_HOP_ID) as u8;
        emit_event(EventType::Anomaly as u8, hop_id, flow_label, &monad);
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Read QoS fields from Monad ──────────────────────────────────────────
    let dst_svc_id = monad.dst_service_id as u32;
    let _qos_class = monad.qos_class;

    // ── Sophia Policy Lookup ────────────────────────────────────────────────
    let (rate_limit_mbps, burst_bytes, target_latency_us) =
        if let Some(policy) = unsafe { QOS_POLICY.get(&dst_svc_id) } {
            increment_stat(STAT_POLICY_HITS);
            (
                policy.rate_limit_mbps as u64,
                policy.burst_bytes as u64,
                policy.target_latency_us as u64,
            )
        } else {
            increment_stat(STAT_POLICY_MISSES);
            // Use config defaults.
            let rl = cfg(CFG_DEFAULT_RATE_LIMIT);
            let burst = cfg(CFG_DEFAULT_BURST);
            let target = {
                let t = cfg(CFG_CODEL_TARGET_US);
                if t == 0 { DEFAULT_CODEL_TARGET_US } else { t }
            };
            (rl, burst, target)
        };

    let now = unsafe { bpf_ktime_get_ns() };

    // Total packet size in bytes (IPv6 payload + fixed header + Ethernet).
    let pkt_bytes = payload_len + (IPV6_FIXED_HDR_LEN as u64) + (ETH_HLEN as u64);

    // ── Token Bucket Rate Limiting ──────────────────────────────────────────
    if rate_limit_mbps > 0 {
        let flow_id = flow_label;

        let bucket_capacity = if burst_bytes > 0 {
            burst_bytes
        } else {
            // Default burst: 1ms worth of traffic at the configured rate.
            (rate_limit_mbps * BITS_PER_MBIT) / (BITS_PER_BYTE * 1_000)
        };

        // Rate in bytes per nanosecond (avoid division by zero).
        // rate_bpns = (rate_limit_mbps * 1_000_000) / (8 * 1_000_000_000)
        //           = rate_limit_mbps / 8_000
        // To maintain precision, we compute: tokens_to_add = elapsed_ns * rate_limit_mbps / 8_000

        if let Some(bucket) = unsafe { TOKEN_BUCKETS.get_ptr_mut(&flow_id) } {
            let bucket_ref = unsafe { &mut *bucket };
            let elapsed = now.saturating_sub(bucket_ref.last_refill_ns);

            // Refill tokens: tokens += elapsed_ns * rate_mbps * 1_000_000 / (8 * 1_000_000_000)
            // Simplified: tokens += elapsed_ns * rate_mbps / 8_000
            let tokens_to_add = if rate_limit_mbps > 0 {
                // Use wrapping_mul — BPF doesn't support u128 (__multi3) from saturating_mul
                (elapsed / 8_000).wrapping_mul(rate_limit_mbps)
            } else {
                0
            };

            if tokens_to_add > 0 {
                bucket_ref.tokens = (bucket_ref.tokens + tokens_to_add).min(bucket_capacity);
                bucket_ref.last_refill_ns = now;
                increment_stat(STAT_BUCKET_REFILLED);
            }

            // Check if packet fits in the bucket.
            if bucket_ref.tokens < pkt_bytes {
                // Rate limit exceeded — drop.
                increment_stat(STAT_DROPPED_RATE_LIMIT);
                update_queue_stats_drop_rate(dst_svc_id);
                let hop_id = cfg(CFG_HOP_ID) as u8;
                emit_event(EventType::Anomaly as u8, hop_id, flow_label, &monad);
                return Ok(xdp_action::XDP_DROP);
            }

            // Consume tokens.
            bucket_ref.tokens -= pkt_bytes;
            increment_stat(STAT_TOKENS_CONSUMED);
        } else {
            // First packet for this flow — create bucket.
            let new_bucket = TokenBucket {
                tokens: bucket_capacity.saturating_sub(pkt_bytes),
                last_refill_ns: now,
            };
            let _ = TOKEN_BUCKETS.insert(&flow_id, &new_bucket, 0);
            increment_stat(STAT_BUCKET_CREATED);
            increment_stat(STAT_TOKENS_CONSUMED);
        }
    }

    // ── CoDel Active Queue Management ───────────────────────────────────────
    // CoDel tracks sojourn time (time spent in queue) and drops packets if
    // the sojourn time consistently exceeds the target.
    //
    // In XDP context, we approximate sojourn time using the Monad latency_hint
    // field, which carries upstream latency in microseconds.
    let sojourn_us = monad.latency_hint_u16() as u64;

    if sojourn_us > target_latency_us && target_latency_us > 0 {
        // Sojourn time exceeds target — check CoDel state.
        let codel_interval = {
            let ci = cfg(CFG_CODEL_INTERVAL_NS);
            if ci == 0 { DEFAULT_CODEL_INTERVAL_NS } else { ci }
        };

        if let Some(qs) = QUEUE_STATS.get_ptr_mut(&dst_svc_id) {
            let qs_ref = unsafe { &mut *qs };

            if qs_ref.codel_first_above_ns == 0 {
                // First time above target — record timestamp.
                qs_ref.codel_first_above_ns = now;
            } else if now.saturating_sub(qs_ref.codel_first_above_ns) > codel_interval {
                // Sojourn time has been above target for longer than one interval.
                // CoDel enters dropping state.
                if qs_ref.codel_dropping == 0 {
                    qs_ref.codel_dropping = 1;
                    qs_ref.codel_count = 1;
                    qs_ref.codel_drop_next_ns = now;
                }

                if now >= qs_ref.codel_drop_next_ns {
                    // Drop this packet.
                    qs_ref.codel_count = qs_ref.codel_count.saturating_add(1);
                    qs_ref.dropped_codel = qs_ref.dropped_codel.saturating_add(1);

                    // Scale interval: next_drop = now + interval / sqrt(count).
                    // Approximate sqrt with a bounded shift.
                    let sqrt_count = isqrt(qs_ref.codel_count);
                    let next_interval = if sqrt_count > 0 {
                        codel_interval / (sqrt_count as u64)
                    } else {
                        codel_interval
                    };
                    qs_ref.codel_drop_next_ns = now + next_interval;

                    increment_stat(STAT_DROPPED_CODEL);
                    let hop_id = cfg(CFG_HOP_ID) as u8;
                    emit_event(EventType::Anomaly as u8, hop_id, flow_label, &monad);
                    return Ok(xdp_action::XDP_DROP);
                }
            }
        } else {
            // No queue stats entry yet — create one with first-above timestamp.
            let new_qs = QueueStats {
                packets: 1,
                passed: 0,
                dropped_rate: 0,
                dropped_codel: 0,
                tokens_consumed: 0,
                codel_first_above_ns: now,
                codel_drop_next_ns: 0,
                codel_count: 0,
                codel_dropping: 0,
                _pad: [0; 3],
            };
            let _ = QUEUE_STATS.insert(&dst_svc_id, &new_qs, 0);
        }
    } else {
        // Sojourn time is below target — reset CoDel state.
        if let Some(qs) = QUEUE_STATS.get_ptr_mut(&dst_svc_id) {
            let qs_ref = unsafe { &mut *qs };
            qs_ref.codel_first_above_ns = 0;
            qs_ref.codel_dropping = 0;
            qs_ref.codel_count = 0;
        }
    }

    // ── Update Queue Stats ──────────────────────────────────────────────────
    if let Some(qs) = QUEUE_STATS.get_ptr_mut(&dst_svc_id) {
        let qs_ref = unsafe { &mut *qs };
        qs_ref.packets = qs_ref.packets.saturating_add(1);
        qs_ref.passed = qs_ref.passed.saturating_add(1);
        qs_ref.tokens_consumed = qs_ref.tokens_consumed.saturating_add(pkt_bytes);
    }

    increment_stat(STAT_PASSED);

    Ok(xdp_action::XDP_PASS)
}

// ── Queue stats helper ────────────────────────────────────────────────────────

/// Increment the rate-limit drop counter for a service.
#[inline(always)]
fn update_queue_stats_drop_rate(dst_svc_id: u32) {
    if let Some(qs) = QUEUE_STATS.get_ptr_mut(&dst_svc_id) {
        let qs_ref = unsafe { &mut *qs };
        qs_ref.packets = qs_ref.packets.saturating_add(1);
        qs_ref.dropped_rate = qs_ref.dropped_rate.saturating_add(1);
    }
}

// ── Integer square root ──────────────────────────────────────────────────────

/// Integer square root using Newton's method — bounded iterations, verifier-safe.
#[inline(always)]
fn isqrt(n: u32) -> u32 {
    if n <= 1 {
        return n;
    }
    let mut x = n;
    let mut y = (x + 1) / 2;
    // Bounded to 16 iterations — sufficient for u32.
    for _ in 0..16u32 {
        if y >= x {
            break;
        }
        x = y;
        // y = (x + n/x) / 2 — avoid division by zero.
        if x > 0 {
            y = (x + n / x) / 2;
        } else {
            break;
        }
    }
    x
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
///
/// Uses `read_volatile` to prevent the compiler from caching stale values and to
/// keep each per-byte bounds check visible to the BPF verifier.
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
