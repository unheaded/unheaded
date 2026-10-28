// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Stevie Bellis.
//! Unheaded Kingdom - Packet Marker XDP Program
//!
//! THE WHISPERING VOID marks packets at the speed of the kernel.
//! This XDP program reads and writes trace IDs in packet headers
//! for distributed tracing propagation.
//!
//! Features:
//! - Extracts trace IDs from incoming packets (IP options or TCP options)
//! - Marks outgoing packets with trace IDs from flow state
//! - Sends packet events to userspace via ring buffer
//! - Ultra-minimal for XDP performance

#![no_std]
#![no_main]

mod common;

use aya_ebpf::{
    bindings::xdp_action,
    macros::{map, xdp},
    maps::{HashMap, RingBuf, XskMap},
    programs::XdpContext,
};
use unheaded_common::{
    Direction, FlowKey, FlowState, PacketAction, PacketEvent, TraceId, ETH_HLEN, ETH_P_IP,
    IPPROTO_TCP, IPPROTO_UDP, IPV4_MIN_HLEN, MAX_FLOWS, RING_BUFFER_SIZE, TCP_MIN_HLEN,
    TRACE_OPTION_TYPE,
};

/// Flow state map: 5-tuple -> flow state with trace ID.
#[map]
static FLOW_STATE: HashMap<FlowKey, FlowState> = HashMap::with_max_entries(MAX_FLOWS, 0);

/// Trace ID injection map: 5-tuple -> trace ID to mark on egress.
/// Set by userspace when a trace should be propagated.
#[map]
static TRACE_INJECT: HashMap<FlowKey, TraceId> = HashMap::with_max_entries(MAX_FLOWS, 0);

/// Ring buffer for packet events to userspace.
#[map]
static PACKET_EVENTS: RingBuf = RingBuf::with_byte_size(RING_BUFFER_SIZE, 0);

/// Statistics counters.
#[map]
static STATS: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

// ── AF_XDP Maps ─────────────────────────────────────────────────────────────

/// XSKMAP — maps queue IDs to AF_XDP socket file descriptors.
/// Trace-marked packets are redirected here for zero-copy delivery.
#[map]
static MARKER_XSKS: XskMap = XskMap::with_max_entries(64, 0);

/// Packet-marker AF_XDP configuration.  Key 0 holds the runtime toggle.
/// Bit 1: AF_XDP redirect enabled for trace-marked packets.
#[map]
static MARKER_CONFIG: HashMap<u32, u32> = HashMap::with_max_entries(4, 0);

// Stats keys
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_PACKETS_IPV4: u32 = 1;
const STAT_PACKETS_TCP: u32 = 2;
const STAT_PACKETS_UDP: u32 = 3;
#[allow(dead_code)]
const STAT_TRACE_EXTRACTED: u32 = 4;
const STAT_TRACE_INJECTED: u32 = 5;
const STAT_EVENTS_SENT: u32 = 6;
const STAT_EVENTS_DROPPED: u32 = 7;
const STAT_AFXDP_REDIRECT: u32 = 8;
const STAT_AFXDP_FALLBACK: u32 = 9;

/// MARKER_CONFIG key for AF_XDP enable toggle.
const PACKET_MARKER_AF_XDP_ENABLE: u32 = 0;

/// Ethernet header structure.
#[repr(C, packed)]
struct EthHdr {
    dst: [u8; 6],
    src: [u8; 6],
    proto: u16,
}

/// IPv4 header structure.
#[repr(C, packed)]
struct Ipv4Hdr {
    version_ihl: u8,
    tos: u8,
    tot_len: u16,
    id: u16,
    frag_off: u16,
    ttl: u8,
    protocol: u8,
    check: u16,
    saddr: u32,
    daddr: u32,
}

/// TCP header structure.
#[repr(C, packed)]
struct TcpHdr {
    source: u16,
    dest: u16,
    seq: u32,
    ack_seq: u32,
    doff_flags: u16, // data offset (4 bits) + reserved + flags
    window: u16,
    check: u16,
    urg_ptr: u16,
}

/// UDP header structure.
#[repr(C, packed)]
struct UdpHdr {
    source: u16,
    dest: u16,
    len: u16,
    check: u16,
}

#[xdp]
pub fn packet_marker(ctx: XdpContext) -> u32 {
    match try_packet_marker(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS,
    }
}

#[inline(always)]
fn try_packet_marker(ctx: &XdpContext) -> Result<u32, ()> {
    // Increment total packet counter
    increment_stat(STAT_PACKETS_TOTAL);

    let data = ctx.data();
    let data_end = ctx.data_end();

    // Parse Ethernet header
    if data + ETH_HLEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    let eth = unsafe { &*(data as *const EthHdr) };
    let eth_proto = u16::from_be(eth.proto);

    // Only handle IPv4 for now
    if eth_proto != ETH_P_IP {
        return Ok(xdp_action::XDP_PASS);
    }

    increment_stat(STAT_PACKETS_IPV4);

    // Parse IPv4 header
    let ip_start = data + ETH_HLEN;
    if ip_start + IPV4_MIN_HLEN > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    let ip = unsafe { &*(ip_start as *const Ipv4Hdr) };
    let ip_hdr_len = ((ip.version_ihl & 0x0F) as usize) * 4;

    if ip_hdr_len < IPV4_MIN_HLEN || ip_start + ip_hdr_len > data_end {
        return Ok(xdp_action::XDP_PASS);
    }

    // Build flow key
    let mut flow_key = FlowKey {
        src_addr: ip.saddr,
        dst_addr: ip.daddr,
        src_port: 0,
        dst_port: 0,
        protocol: ip.protocol,
        _pad: [0; 3],
    };

    let mut trace_id = TraceId::zero();
    let transport_start = ip_start + ip_hdr_len;

    // Parse transport header based on protocol
    match ip.protocol {
        IPPROTO_TCP => {
            increment_stat(STAT_PACKETS_TCP);

            if transport_start + TCP_MIN_HLEN > data_end {
                return Ok(xdp_action::XDP_PASS);
            }

            let tcp = unsafe { &*(transport_start as *const TcpHdr) };
            flow_key.src_port = tcp.source;
            flow_key.dst_port = tcp.dest;

            // TCP option trace ID (kind 253), copied onto the stack and
            // parsed there (extract_trace_option; S-TRUE-WIRE Step 217, was
            // TODO(E1)).
            let tcp_hdr_len = (((u16::from_be(tcp.doff_flags) >> 12) & 0x0F) as usize) * 4;
            if tcp_hdr_len > TCP_MIN_HLEN {
                let off = transport_start - data + TCP_MIN_HLEN;
                if let Some(t) = extract_trace_option(
                    ctx,
                    off,
                    tcp_hdr_len - TCP_MIN_HLEN,
                    TCP_TRACE_OPTION_KIND,
                ) {
                    trace_id = t;
                    increment_stat(STAT_TRACE_EXTRACTED);
                }
            }
        }
        IPPROTO_UDP => {
            increment_stat(STAT_PACKETS_UDP);

            if transport_start + 8 > data_end {
                return Ok(xdp_action::XDP_PASS);
            }

            let udp = unsafe { &*(transport_start as *const UdpHdr) };
            flow_key.src_port = udp.source;
            flow_key.dst_port = udp.dest;
        }
        _ => {
            return Ok(xdp_action::XDP_PASS);
        }
    }

    // IP option trace ID, when the TCP options carried none (S-TRUE-WIRE
    // Step 217; was TODO(E1)).
    if trace_id.is_zero() && ip_hdr_len > IPV4_MIN_HLEN {
        let off = ETH_HLEN + IPV4_MIN_HLEN;
        if let Some(t) =
            extract_trace_option(ctx, off, ip_hdr_len - IPV4_MIN_HLEN, TRACE_OPTION_TYPE)
        {
            trace_id = t;
            increment_stat(STAT_TRACE_EXTRACTED);
        }
    }

    // Check if we have a trace ID to inject for this flow
    if let Some(inject_trace) = unsafe { TRACE_INJECT.get(&flow_key) } {
        if trace_id.is_zero() {
            trace_id = *inject_trace;
            increment_stat(STAT_TRACE_INJECTED);
        }
    }

    // Update flow state
    update_flow_state(&flow_key, &trace_id);

    // ── AF_XDP selective redirect ──────────────────────────────────────────
    // If AF_XDP is enabled and this packet has a trace ID, redirect to the
    // AF_XDP socket for zero-copy delivery to userspace trace-collector.
    // Unmarked packets fall through to the standard kernel stack.
    let af_xdp_enabled = match unsafe { MARKER_CONFIG.get(&PACKET_MARKER_AF_XDP_ENABLE) } {
        Some(val) => (*val & 1) != 0,
        None => false,
    };

    if af_xdp_enabled && !trace_id.is_zero() {
        let queue_id = unsafe { (*ctx.ctx).rx_queue_index };
        match MARKER_XSKS.redirect(queue_id, 0) {
            Ok(action) => {
                increment_stat(STAT_AFXDP_REDIRECT);
                let packet_len = (data_end - data) as u32;
                send_packet_event(
                    &flow_key,
                    &trace_id,
                    packet_len,
                    PacketAction::Redirect,
                    Direction::Ingress,
                );
                return Ok(action);
            }
            Err(_) => {
                // No socket bound — fall through to kernel stack
                increment_stat(STAT_AFXDP_FALLBACK);
            }
        }
    }

    // Send packet event to userspace (standard path)
    let packet_len = (data_end - data) as u32;
    send_packet_event(
        &flow_key,
        &trace_id,
        packet_len,
        PacketAction::Pass,
        Direction::Ingress,
    );

    Ok(xdp_action::XDP_PASS)
}

/// Experimental TCP option kind carrying the trace ID (RFC 6994 range).
const TCP_TRACE_OPTION_KIND: u8 = 253;

/// IPv4 and TCP both cap their option area at 40 bytes.
const MAX_OPTIONS: usize = 40;

/// Find the trace-ID option (`kind`, 18 bytes: kind, len, 16-byte ID) in the
/// IPv4 or TCP option area of `len` bytes at packet offset `off`.
///
/// The option bytes are copied onto the stack with bpf_xdp_load_bytes and
/// parsed there with bounded indices. Walking them through packet pointers
/// (the previous code, disabled as TODO(E1)) cannot be verified: LLVM folds
/// away the per-iteration data_end checks the verifier needs ("offset is
/// outside of the packet", S-TRUE-WIRE Step 217), and a bpf_loop callback
/// cannot hold packet pointers either.
#[inline(always)]
fn extract_trace_option(ctx: &XdpContext, off: usize, len: usize, kind: u8) -> Option<TraceId> {
    if len < 2 || len > MAX_OPTIONS {
        return None;
    }
    let mut buf = [0u8; MAX_OPTIONS];
    let ret = unsafe {
        aya_ebpf::helpers::gen::bpf_xdp_load_bytes(
            ctx.ctx,
            off as u32,
            buf.as_mut_ptr() as *mut aya_ebpf::cty::c_void,
            len as u32,
        )
    };
    if ret != 0 {
        return None;
    }
    find_trace_option(&buf, len, kind)
}

/// Parse an option area copied onto the stack. At most 10 options are
/// examined (40 bytes, and every real stack sends far fewer).
#[inline(always)]
fn find_trace_option(buf: &[u8; MAX_OPTIONS], n: usize, kind: u8) -> Option<TraceId> {
    let mut i: usize = 0;
    for _ in 0..10 {
        if i >= n {
            break;
        }
        let k = *buf.get(i)?;
        if k == 0 {
            break; // end of options
        }
        if k == 1 {
            i += 1; // NOP
            continue;
        }
        let l = *buf.get(i + 1)? as usize;
        if l < 2 || i + l > n {
            break;
        }
        if k == kind && l >= 18 {
            let mut bytes = [0u8; 16];
            let mut j = 0;
            while j < 16 {
                bytes[j] = *buf.get(i + 2 + j)?;
                j += 1;
            }
            return Some(TraceId::from_bytes(bytes));
        }
        i += l;
    }
    None
}

/// Update flow state with trace ID.
#[inline(always)]
fn update_flow_state(flow_key: &FlowKey, trace_id: &TraceId) {
    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };

    if let Some(state) = FLOW_STATE.get_ptr_mut(flow_key) {
        let state = unsafe { &mut *state };
        state.last_seen_ns = now;
        state.packets_in = state.packets_in.saturating_add(1);

        // Update trace ID if we extracted a new one
        if !trace_id.is_zero() && state.trace_id.is_zero() {
            state.trace_id = *trace_id;
        }
    } else {
        // New flow
        let state = FlowState {
            trace_id: *trace_id,
            start_ns: now,
            last_seen_ns: now,
            packets_in: 1,
            packets_out: 0,
            bytes_in: 0,
            bytes_out: 0,
            state: unheaded_common::ConnectionState::Unknown,
            _pad: [0; 7],
        };
        let _ = FLOW_STATE.insert(flow_key, &state, 0);
    }
}

/// Send packet event to userspace via ring buffer.
#[inline(always)]
fn send_packet_event(
    flow_key: &FlowKey,
    trace_id: &TraceId,
    packet_len: u32,
    action: PacketAction,
    direction: Direction,
) {
    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };

    let event = PacketEvent {
        timestamp_ns: now,
        trace_id: *trace_id,
        flow_key: *flow_key,
        packet_len,
        action,
        direction,
        _pad: [0; 2],
    };

    if let Some(mut buf) = PACKET_EVENTS.reserve::<PacketEvent>(0) {
        unsafe {
            core::ptr::write(buf.as_mut_ptr(), event);
            buf.submit(0);
        }
        increment_stat(STAT_EVENTS_SENT);
    } else {
        increment_stat(STAT_EVENTS_DROPPED);
    }
}

/// Increment a statistics counter.
#[inline(always)]
fn increment_stat(key: u32) {
    if let Some(val) = STATS.get_ptr_mut(&key) {
        unsafe {
            *val = (*val).saturating_add(1);
        }
    } else {
        let _ = STATS.insert(&key, &1u64, 0);
    }
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
