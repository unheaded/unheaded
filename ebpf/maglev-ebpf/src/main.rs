// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//! Unheaded Protocol — Maglev eBPF Program (App 04)
//!
//! Maglev consistent hash load balancer at the XDP layer.
//!
//! 1. Parse IPv6 + HBH headers to locate the Monad option
//! 2. Verify CRC-16/CCITT — emit ANOMALY on failure, pass without mutation
//! 3. Extract flow 5-tuple from IPv6 + TCP/UDP headers
//! 4. Hash flow key and lookup Maglev table for backend selection
//! 5. Check connection table for sticky routing (existing flows)
//! 6. Health-aware: skip unhealthy backends
//! 7. Rewrite L2 dst MAC for backend forwarding (XDP_TX)
//! 8. Emit forwarding events to ANAMNESIS ring buffer
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
    maps::{Array, HashMap, RingBuf},
    programs::XdpContext,
};
use monad_common::{
    flags, AnamnesisEvent, EventType, Monad, HBH_TOTAL_LEN, IPV6_FIXED_HDR_LEN,
    IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN, MONAD_OPT_TYPE, MONAD_SIZE,
};

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// Maglev lookup table — 65537 entries (prime for consistent hashing).
/// Index: hash % 65537, Value: backend_id (u32).
#[map]
static MAGLEV_TABLE: Array<u32> = Array::with_max_entries(65_537, 0);

/// Backend information — 256 max backends.
/// Key: backend_id (u32), Value: BackendInfo.
#[map]
static BACKENDS: HashMap<u32, BackendInfo> = HashMap::with_max_entries(256, 0);

/// Connection affinity table — 1M entries for sticky sessions.
/// Key: flow_hash (u32), Value: backend_id (u32).
#[map]
static CONN_TABLE: HashMap<u32, u32> = HashMap::with_max_entries(1_000_000, 0);

/// Runtime configuration — tunable from userspace without reloading.
///
/// | Key | Meaning                            | Default |
/// |-----|------------------------------------|---------|
/// | 0   | hop_id (this node's identifier)    | 0       |
/// | 1   | sticky_sessions (0=off, 1=on)      | 1       |
/// | 2   | health_check_interval_ns           | 0 = off |
/// | 3   | maglev_table_size (prime)          | 65537   |
#[map]
static CONFIG: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

/// Statistics counters — observable from userspace.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

/// Anamnesis ring buffer — 4 MiB, shared with userspace.
#[map]
static ANAMNESIS: RingBuf = RingBuf::with_byte_size(4 * 1024 * 1024, 0);

// ── Backend info type ───────────────────────────────────────────────────────

/// Backend information stored in BACKENDS map.
#[repr(C)]
#[derive(Clone, Copy, Default)]
struct BackendInfo {
    /// Backend MAC address for L2 rewrite.
    mac_addr: [u8; 6],
    /// Weight for weighted load balancing (0 = equal weight).
    weight: u8,
    /// Health status: 0 = unhealthy, 1 = healthy.
    health: u8,
    /// Backend IPv6 address (last 4 bytes for display/logging).
    addr_lo: u32,
    /// Backend port (for logging/stats only — L2 forwarding does not rewrite ports).
    port: u16,
    /// Padding for alignment.
    _pad: [u8; 2],
}

// ── Stats keys ────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_FORWARDED: u32 = 1;
const STAT_BACKEND_HITS: u32 = 2;
const STAT_BACKEND_MISSES: u32 = 3;
const STAT_REBALANCES: u32 = 4;
const STAT_HEALTH_CHECKS: u32 = 5;
const STAT_STICKY_HITS: u32 = 6;
const STAT_STICKY_MISSES: u32 = 7;
const STAT_CRC_ERRORS: u32 = 8;
const STAT_EVENTS_SENT: u32 = 9;
const STAT_EVENTS_DROPPED: u32 = 10;
const STAT_UNHEALTHY_SKIP: u32 = 11;

// ── Config keys ───────────────────────────────────────────────────────────────
const CFG_HOP_ID: u32 = 0;
const CFG_STICKY_SESSIONS: u32 = 1;
const CFG_HEALTH_CHECK_INTERVAL: u32 = 2;
const CFG_MAGLEV_TABLE_SIZE: u32 = 3;

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

/// TCP header (20 bytes minimum, packed).
#[repr(C, packed)]
struct TcpHdr {
    src_port: u16,
    dst_port: u16,
    seq: u32,
    ack: u32,
    off_flags: u16,
    window: u16,
    checksum: u16,
    urgent: u16,
}

/// UDP header (8 bytes, packed).
#[repr(C, packed)]
struct UdpHdr {
    src_port: u16,
    dst_port: u16,
    length: u16,
    checksum: u16,
}

const ETH_HLEN: usize = 14;
const ETH_P_IPV6: u16 = 0x86DD;
const IPPROTO_TCP: u8 = 6;
const IPPROTO_UDP: u8 = 17;
const TCP_HDR_LEN: usize = 20;
const UDP_HDR_LEN: usize = 8;

/// Maglev table size — must match the Array map size.
const MAGLEV_SIZE: u32 = 65_537;

// ── XDP entry point ───────────────────────────────────────────────────────────

#[xdp]
pub fn maglev_xdp(ctx: XdpContext) -> u32 {
    match try_maglev_xdp(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS, // fail-open: never silently drop
    }
}

#[inline(always)]
fn try_maglev_xdp(ctx: &XdpContext) -> Result<u32, ()> {
    increment_stat(STAT_PACKETS_TOTAL);

    let data = ctx.data();
    let data_end = ctx.data_end();

    // ── Ethernet ──────────────────────────────────────────────────────────────
    if data + ETH_HLEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let eth_ptr = data as *mut EthHdr;
    let eth = unsafe { &*eth_ptr };
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

    // Extract Flow Label from the packed vtf field (low 20 bits).
    let vtf = u32::from_be(unsafe { core::ptr::read_unaligned(core::ptr::addr_of!(ip.vtf)) });
    let flow_label = vtf & 0x000F_FFFF;

    // Extract source and destination IPv6 addresses (last 4 bytes each).
    let src_addr = unsafe { core::ptr::read_volatile((ip_start + 24) as *const u32) };
    let dst_addr = unsafe { core::ptr::read_volatile((ip_start + 36) as *const u32) };

    // ── Hop-by-Hop Header ─────────────────────────────────────────────────────
    let hbh_start = ip_start + IPV6_FIXED_HDR_LEN;

    if hbh_start + HBH_TOTAL_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    let transport_proto = unsafe { core::ptr::read_volatile(hbh_start as *const u8) };

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

    // ── Parse Transport Header for 5-Tuple ──────────────────────────────────
    let transport_start = hbh_start + hbh_total;
    let mut src_port: u16 = 0;
    let mut dst_port: u16 = 0;

    if transport_proto == IPPROTO_TCP {
        if transport_start + TCP_HDR_LEN > data_end {
            return Ok(xdp_action::XDP_PASS);
        }
        let tcp = unsafe { &*(transport_start as *const TcpHdr) };
        src_port = u16::from_be(unsafe {
            core::ptr::read_unaligned(core::ptr::addr_of!(tcp.src_port))
        });
        dst_port = u16::from_be(unsafe {
            core::ptr::read_unaligned(core::ptr::addr_of!(tcp.dst_port))
        });
    } else if transport_proto == IPPROTO_UDP {
        if transport_start + UDP_HDR_LEN > data_end {
            return Ok(xdp_action::XDP_PASS);
        }
        let udp = unsafe { &*(transport_start as *const UdpHdr) };
        src_port = u16::from_be(unsafe {
            core::ptr::read_unaligned(core::ptr::addr_of!(udp.src_port))
        });
        dst_port = u16::from_be(unsafe {
            core::ptr::read_unaligned(core::ptr::addr_of!(udp.dst_port))
        });
    }

    // ── Compute Flow Hash ──────────────────────────────────────────────────
    let flow_hash = compute_flow_hash(src_addr, dst_addr, src_port, dst_port, transport_proto);

    // ── Sticky Session Lookup ─────────────────────────────────────────────────
    let sticky_enabled = cfg(CFG_STICKY_SESSIONS) != 0;
    let mut backend_id: Option<u32> = None;

    if sticky_enabled {
        if let Some(existing_backend) = unsafe { CONN_TABLE.get(&flow_hash) } {
            // Verify the backend is still healthy before using the sticky mapping.
            if let Some(info) = unsafe { BACKENDS.get(existing_backend) } {
                if info.health != 0 {
                    backend_id = Some(*existing_backend);
                    increment_stat(STAT_STICKY_HITS);
                } else {
                    // Backend unhealthy — need to rebalance.
                    increment_stat(STAT_UNHEALTHY_SKIP);
                    increment_stat(STAT_REBALANCES);
                }
            }
        }
        if backend_id.is_none() {
            increment_stat(STAT_STICKY_MISSES);
        }
    }

    // ── Maglev Table Lookup ──────────────────────────────────────────────────
    if backend_id.is_none() {
        let table_size = {
            let ts = cfg(CFG_MAGLEV_TABLE_SIZE) as u32;
            if ts == 0 { MAGLEV_SIZE } else { ts }
        };
        let idx = flow_hash % table_size;

        if let Some(bid) = MAGLEV_TABLE.get(idx) {
            // Verify backend health before using.
            if let Some(info) = unsafe { BACKENDS.get(bid) } {
                if info.health != 0 {
                    backend_id = Some(*bid);
                    increment_stat(STAT_BACKEND_HITS);
                } else {
                    // Primary backend unhealthy — probe next slots (bounded scan).
                    increment_stat(STAT_UNHEALTHY_SKIP);
                    let found = find_healthy_backend(idx, table_size);
                    if found.is_some() {
                        backend_id = found;
                        increment_stat(STAT_REBALANCES);
                    }
                }
            }
        }
    }

    // ── Forward to Backend ──────────────────────────────────────────────────
    let bid = match backend_id {
        Some(id) => id,
        None => {
            increment_stat(STAT_BACKEND_MISSES);
            return Ok(xdp_action::XDP_PASS);
        }
    };

    // Store sticky mapping for future packets in this flow.
    if sticky_enabled {
        let _ = CONN_TABLE.insert(&flow_hash, &bid, 0);
    }

    // Look up backend info for MAC rewrite.
    let backend = match unsafe { BACKENDS.get(&bid) } {
        Some(b) => b,
        None => {
            increment_stat(STAT_BACKEND_MISSES);
            return Ok(xdp_action::XDP_PASS);
        }
    };

    // ── Rewrite L2 Destination MAC ──────────────────────────────────────────
    // Bounds check already verified at Ethernet parse above.
    if data + ETH_HLEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }
    let eth_mut = unsafe { &mut *eth_ptr };
    // Save original src MAC, set dst to backend, set src to original dst (hairpin).
    let orig_src = eth_mut.src;
    eth_mut.dst = backend.mac_addr;
    eth_mut.src = orig_src;

    increment_stat(STAT_FORWARDED);

    // Emit forwarding event.
    let hop_id = cfg(CFG_HOP_ID) as u8;
    emit_event(EventType::Hop as u8, hop_id, flow_label, &monad);

    Ok(xdp_action::XDP_TX)
}

// ── Flow hash computation ─────────────────────────────────────────────────────

/// Compute a hash of the flow 5-tuple for Maglev table lookup.
///
/// Uses a simple but effective hash (FNV-1a inspired) suitable for BPF.
/// Distributes well across the 65537-entry Maglev table.
#[inline(always)]
fn compute_flow_hash(src_addr: u32, dst_addr: u32, src_port: u16, dst_port: u16, proto: u8) -> u32 {
    // FNV-1a-inspired hash for BPF (no division, no loops over variable bounds).
    let mut h: u32 = 0x811c_9dc5; // FNV offset basis
    const PRIME: u32 = 0x0100_0193; // FNV prime

    // Mix source address bytes.
    h ^= src_addr;
    h = h.wrapping_mul(PRIME);

    // Mix destination address bytes.
    h ^= dst_addr;
    h = h.wrapping_mul(PRIME);

    // Mix ports.
    h ^= (src_port as u32) << 16 | (dst_port as u32);
    h = h.wrapping_mul(PRIME);

    // Mix protocol.
    h ^= proto as u32;
    h = h.wrapping_mul(PRIME);

    h
}

// ── Healthy backend search ──────────────────────────────────────────────────

/// Probe up to 8 adjacent Maglev table slots to find a healthy backend.
/// Returns the first healthy backend_id found, or None.
#[inline(always)]
fn find_healthy_backend(start_idx: u32, table_size: u32) -> Option<u32> {
    // Bounded scan: at most 8 probes to avoid verifier complexity.
    for i in 1..8u32 {
        let probe_idx = (start_idx + i) % table_size;
        if let Some(bid) = MAGLEV_TABLE.get(probe_idx) {
            if let Some(info) = unsafe { BACKENDS.get(bid) } {
                if info.health != 0 {
                    return Some(*bid);
                }
            }
        }
    }
    None
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
