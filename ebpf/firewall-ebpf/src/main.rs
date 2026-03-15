// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Unheaded Protocol — Firewall eBPF Program (App 03)
//!
//! Stateful XDP firewall with connection tracking and Sophia policy enforcement.
//!
//! 1. Parse IPv6 + HBH headers to locate the Monad option
//! 2. Verify CRC-16/CCITT — emit ANOMALY on failure, pass without mutation
//! 3. Parse TCP/UDP transport headers for 5-tuple extraction
//! 4. Connection tracking: maintain state machine for each flow
//! 5. TCP state machine: SYN/SYN-ACK/ACK/FIN/RST transitions
//! 6. Sophia dictionary lookup for policy (src_svc, dst_svc -> allow/deny)
//! 7. Emit block events to ANAMNESIS ring buffer
//! 8. Increment per-flow packet/byte counters
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
    flags, AnamnesisEvent, EventType, Monad, SophiaEntry, HBH_TOTAL_LEN, IPV6_FIXED_HDR_LEN,
    IPV6_NEXTHDR_HBH, MONAD_OPT_DATA_LEN, MONAD_OPT_TYPE, MONAD_SIZE,
};

// ── BPF Maps ─────────────────────────────────────────────────────────────────

/// Connection tracking table — 1M entries.
/// Key: ConnTrackKey (5-tuple hash), Value: ConnState.
#[map]
static CONNTRACK: HashMap<ConnTrackKey, ConnState> = HashMap::with_max_entries(1_000_000, 0);

/// Policy cache — 100K entries.
/// Key: (src_service_id << 8 | dst_service_id), Value: PolicyEntry.
#[map]
static POLICY_CACHE: HashMap<u16, PolicyEntry> = HashMap::with_max_entries(100_000, 0);

/// Block event ring buffer — 4 MiB, shared with userspace.
#[map]
static ANAMNESIS: RingBuf = RingBuf::with_byte_size(4 * 1024 * 1024, 0);

/// Runtime configuration — tunable from userspace without reloading.
///
/// | Key | Meaning                            | Default |
/// |-----|------------------------------------|---------|
/// | 0   | hop_id (this node's identifier)    | 0       |
/// | 1   | default_policy (0=deny, 1=allow)   | 1       |
/// | 2   | conntrack_timeout_ns               | 0 = off |
/// | 3   | max_connections                    | 0 = off |
#[map]
static CONFIG: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

/// Statistics counters — observable from userspace.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

// ── Connection state types ──────────────────────────────────────────────────

/// TCP connection states for the stateful firewall.
#[repr(u8)]
#[derive(Clone, Copy, PartialEq, Eq)]
enum TcpConnState {
    New = 0,
    SynSent = 1,
    SynRcvd = 2,
    Established = 3,
    Closing = 4,
}

/// 5-tuple connection tracking key.
#[repr(C)]
#[derive(Clone, Copy, Default)]
struct ConnTrackKey {
    /// Source IPv6 address (last 4 bytes for map efficiency).
    src_addr: u32,
    /// Destination IPv6 address (last 4 bytes).
    dst_addr: u32,
    /// Source port.
    src_port: u16,
    /// Destination port.
    dst_port: u16,
    /// Protocol number (6=TCP, 17=UDP).
    protocol: u8,
    /// Padding for alignment.
    _pad: [u8; 3],
}

/// Connection state value stored in CONNTRACK map.
#[repr(C)]
#[derive(Clone, Copy, Default)]
struct ConnState {
    /// Current TCP state (TcpConnState value).
    state: u8,
    /// TCP flags seen on last packet.
    tcp_flags: u8,
    /// Padding.
    _pad: [u8; 2],
    /// Timestamp when this connection was created (bpf_ktime_get_ns).
    created_ts: u64,
    /// Timestamp of last packet seen on this connection.
    last_seen: u64,
    /// Total packet count for this connection.
    pkt_count: u32,
    /// Total byte count for this connection.
    byte_count: u32,
}

/// Policy entry stored in POLICY_CACHE map.
#[repr(C)]
#[derive(Clone, Copy, Default)]
struct PolicyEntry {
    /// Action: 0 = deny, 1 = allow.
    action: u8,
    /// Priority (higher = more specific rule).
    priority: u8,
    /// Padding.
    _pad: [u8; 2],
    /// Rate limit (packets per second, 0 = unlimited).
    rate_limit: u32,
}

// ── TCP flag constants ──────────────────────────────────────────────────────

const TCP_FIN: u8 = 0x01;
const TCP_SYN: u8 = 0x02;
const TCP_RST: u8 = 0x04;
const TCP_ACK: u8 = 0x10;

// ── Stats keys ────────────────────────────────────────────────────────────────
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_BLOCKED: u32 = 1;
const STAT_ALLOWED: u32 = 2;
const STAT_CONNTRACK_ENTRIES: u32 = 3;
const STAT_CONNTRACK_MISSES: u32 = 4;
const STAT_POLICY_HITS: u32 = 5;
const STAT_POLICY_MISSES: u32 = 6;
const STAT_CRC_ERRORS: u32 = 7;
const STAT_EVENTS_SENT: u32 = 8;
const STAT_EVENTS_DROPPED: u32 = 9;
const STAT_TCP_SYN: u32 = 10;
const STAT_TCP_SYN_ACK: u32 = 11;
const STAT_TCP_FIN: u32 = 12;
const STAT_TCP_RST: u32 = 13;

// ── Config keys ───────────────────────────────────────────────────────────────
const CFG_HOP_ID: u32 = 0;
const CFG_DEFAULT_POLICY: u32 = 1;
const CFG_CONNTRACK_TIMEOUT: u32 = 2;
const CFG_MAX_CONNECTIONS: u32 = 3;

// ── Sophia dictionary IDs ─────────────────────────────────────────────────────
const SOPHIA_DICT_POLICY: u8 = 0x10;

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

// ── XDP entry point ───────────────────────────────────────────────────────────

#[xdp]
pub fn firewall_xdp(ctx: XdpContext) -> u32 {
    match try_firewall_xdp(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS, // fail-open: never silently drop
    }
}

#[inline(always)]
fn try_firewall_xdp(ctx: &XdpContext) -> Result<u32, ()> {
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

    // Only process HBH-carrying packets (Monad protocol).
    if ip.next_header != IPV6_NEXTHDR_HBH {
        return Ok(xdp_action::XDP_PASS);
    }

    // Extract Flow Label from the packed vtf field (low 20 bits).
    let vtf = u32::from_be(unsafe { core::ptr::read_unaligned(core::ptr::addr_of!(ip.vtf)) });
    let flow_label = vtf & 0x000F_FFFF;

    // Extract source and destination IPv6 addresses (last 4 bytes each).
    let src_addr = unsafe {
        core::ptr::read_volatile((ip_start + 24) as *const u32)
    };
    let dst_addr = unsafe {
        core::ptr::read_volatile((ip_start + 36) as *const u32)
    };

    // ── Hop-by-Hop Header ─────────────────────────────────────────────────────
    let hbh_start = ip_start + IPV6_FIXED_HDR_LEN;

    if hbh_start + HBH_TOTAL_LEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    // Read HBH next_header to determine transport protocol.
    let transport_proto = unsafe { core::ptr::read_volatile(hbh_start as *const u8) };

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

    // ── Parse Transport Header ──────────────────────────────────────────────
    let transport_start = hbh_start + hbh_total;
    let mut src_port: u16 = 0;
    let mut dst_port: u16 = 0;
    let mut tcp_flags_byte: u8 = 0;
    let payload_len = if data_end > transport_start {
        (data_end - transport_start) as u32
    } else {
        0
    };

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
        // TCP flags are in the low 8 bits of the off_flags field.
        let off_flags = u16::from_be(unsafe {
            core::ptr::read_unaligned(core::ptr::addr_of!(tcp.off_flags))
        });
        tcp_flags_byte = (off_flags & 0x3F) as u8;
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
    } else {
        // Non-TCP/UDP: pass through without firewall processing.
        increment_stat(STAT_ALLOWED);
        return Ok(xdp_action::XDP_PASS);
    }

    // ── Build Connection Tracking Key ────────────────────────────────────────
    let ct_key = ConnTrackKey {
        src_addr,
        dst_addr,
        src_port,
        dst_port,
        protocol: transport_proto,
        _pad: [0; 3],
    };

    // ── Connection Tracking Lookup ──────────────────────────────────────────
    let now = unsafe { bpf_ktime_get_ns() };

    if let Some(conn) = unsafe { CONNTRACK.get_ptr_mut(&ct_key) } {
        // Existing connection — update state.
        let conn_ref = unsafe { &mut *conn };
        conn_ref.last_seen = now;
        conn_ref.pkt_count = conn_ref.pkt_count.saturating_add(1);
        conn_ref.byte_count = conn_ref.byte_count.saturating_add(payload_len);

        // ── TCP State Machine ──────────────────────────────────────────────
        if transport_proto == IPPROTO_TCP {
            conn_ref.tcp_flags = tcp_flags_byte;
            update_tcp_state(conn_ref, tcp_flags_byte);
        }

        // Check conntrack timeout.
        let timeout = cfg(CFG_CONNTRACK_TIMEOUT);
        if timeout > 0 && (now - conn_ref.last_seen) > timeout {
            // Connection timed out — treat as new.
            increment_stat(STAT_CONNTRACK_MISSES);
        } else {
            // Existing established connection — allow.
            if conn_ref.state == TcpConnState::Established as u8
                || transport_proto == IPPROTO_UDP
            {
                increment_stat(STAT_ALLOWED);
                return Ok(xdp_action::XDP_PASS);
            }
            // Connection in progress (SYN_SENT, SYN_RCVD) — allow to complete.
            if conn_ref.state == TcpConnState::SynSent as u8
                || conn_ref.state == TcpConnState::SynRcvd as u8
            {
                increment_stat(STAT_ALLOWED);
                return Ok(xdp_action::XDP_PASS);
            }
        }
    } else {
        increment_stat(STAT_CONNTRACK_MISSES);
    }

    // ── Sophia Policy Lookup ────────────────────────────────────────────────
    let policy_key = ((monad.src_service_id as u16) << 8) | (monad.dst_service_id as u16);
    let action = if let Some(policy) = unsafe { POLICY_CACHE.get(&policy_key) } {
        increment_stat(STAT_POLICY_HITS);
        policy.action
    } else {
        increment_stat(STAT_POLICY_MISSES);
        // Default policy from config (0 = deny, 1 = allow).
        cfg(CFG_DEFAULT_POLICY) as u8
    };

    // ── Enforce Policy ──────────────────────────────────────────────────────
    if action == 0 {
        // DENY — emit block event and drop.
        increment_stat(STAT_BLOCKED);
        let hop_id = cfg(CFG_HOP_ID) as u8;
        emit_event(EventType::Anomaly as u8, hop_id, flow_label, &monad);
        return Ok(xdp_action::XDP_DROP);
    }

    // ── Create / Update Connection Tracking Entry ───────────────────────────
    let initial_state = if transport_proto == IPPROTO_TCP {
        if tcp_flags_byte & TCP_SYN != 0 && tcp_flags_byte & TCP_ACK == 0 {
            increment_stat(STAT_TCP_SYN);
            TcpConnState::SynSent as u8
        } else if tcp_flags_byte & TCP_SYN != 0 && tcp_flags_byte & TCP_ACK != 0 {
            increment_stat(STAT_TCP_SYN_ACK);
            TcpConnState::SynRcvd as u8
        } else {
            TcpConnState::New as u8
        }
    } else {
        TcpConnState::Established as u8 // UDP is stateless — immediately "established"
    };

    let new_conn = ConnState {
        state: initial_state,
        tcp_flags: tcp_flags_byte,
        _pad: [0; 2],
        created_ts: now,
        last_seen: now,
        pkt_count: 1,
        byte_count: payload_len,
    };

    let _ = CONNTRACK.insert(&ct_key, &new_conn, 0);
    increment_stat(STAT_CONNTRACK_ENTRIES);
    increment_stat(STAT_ALLOWED);

    Ok(xdp_action::XDP_PASS)
}

// ── TCP state machine ───────────────────────────────────────────────────────

/// Update TCP connection state based on observed flags.
#[inline(always)]
fn update_tcp_state(conn: &mut ConnState, tcp_flags: u8) {
    let current = conn.state;

    if tcp_flags & TCP_RST != 0 {
        // RST always terminates the connection.
        conn.state = TcpConnState::Closing as u8;
        increment_stat(STAT_TCP_RST);
        return;
    }

    if current == TcpConnState::New as u8 || current == TcpConnState::SynSent as u8 {
        if tcp_flags & TCP_SYN != 0 && tcp_flags & TCP_ACK != 0 {
            // SYN-ACK received — move to SYN_RCVD.
            conn.state = TcpConnState::SynRcvd as u8;
            increment_stat(STAT_TCP_SYN_ACK);
        } else if tcp_flags & TCP_SYN != 0 {
            conn.state = TcpConnState::SynSent as u8;
            increment_stat(STAT_TCP_SYN);
        }
    } else if current == TcpConnState::SynRcvd as u8 {
        if tcp_flags & TCP_ACK != 0 {
            // Final ACK of 3-way handshake — ESTABLISHED.
            conn.state = TcpConnState::Established as u8;
        }
    } else if current == TcpConnState::Established as u8 {
        if tcp_flags & TCP_FIN != 0 {
            // FIN received — begin closing.
            conn.state = TcpConnState::Closing as u8;
            increment_stat(STAT_TCP_FIN);
        }
    }
    // CLOSING state: stays closing until entry is reaped by timeout.
}

// ── Packet memory helpers ─────────────────────────────────────────────────────

/// Find the byte offset of the MONAD_OPT_TYPE option within an HBH option area.
///
/// Bounded to `MAX_OPT_SCAN` iterations — verifier-safe.  `black_box` prevents
/// LLVM from eliminating the per-iteration `data_end` checks.
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
