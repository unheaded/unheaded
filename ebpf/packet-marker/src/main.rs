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

use aya_ebpf::{
    bindings::xdp_action,
    macros::{map, xdp},
    maps::{HashMap, RingBuf},
    programs::XdpContext,
};
use aya_log_ebpf::info;
use unheaded_common::{
    Direction, FlowKey, FlowState, PacketAction, PacketEvent, TraceId,
    ETH_HLEN, ETH_P_IP, IPV4_MIN_HLEN, IPPROTO_TCP, IPPROTO_UDP,
    MAX_FLOWS, RING_BUFFER_SIZE, TCP_MIN_HLEN, TRACE_OPTION_TYPE,
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

// Stats keys
const STAT_PACKETS_TOTAL: u32 = 0;
const STAT_PACKETS_IPV4: u32 = 1;
const STAT_PACKETS_TCP: u32 = 2;
const STAT_PACKETS_UDP: u32 = 3;
const STAT_TRACE_EXTRACTED: u32 = 4;
const STAT_TRACE_INJECTED: u32 = 5;
const STAT_EVENTS_SENT: u32 = 6;
const STAT_EVENTS_DROPPED: u32 = 7;

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

            // Try to extract trace ID from TCP options
            let tcp_hdr_len = (((u16::from_be(tcp.doff_flags) >> 12) & 0x0F) as usize) * 4;
            if tcp_hdr_len > TCP_MIN_HLEN && transport_start + tcp_hdr_len <= data_end {
                if let Some(tid) = extract_tcp_trace_id(transport_start, tcp_hdr_len, data_end) {
                    trace_id = tid;
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

    // Try to extract trace ID from IP options if not found in TCP
    if trace_id.is_zero() && ip_hdr_len > IPV4_MIN_HLEN {
        if let Some(tid) = extract_ip_trace_id(ip_start, ip_hdr_len, data_end) {
            trace_id = tid;
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

    // Send packet event to userspace
    let packet_len = (data_end - data) as u32;
    send_packet_event(&flow_key, &trace_id, packet_len, PacketAction::Pass, Direction::Ingress);

    Ok(xdp_action::XDP_PASS)
}

/// Extract trace ID from IP options.
#[inline(always)]
fn extract_ip_trace_id(ip_start: usize, ip_hdr_len: usize, data_end: usize) -> Option<TraceId> {
    let options_start = ip_start + IPV4_MIN_HLEN;
    let options_end = ip_start + ip_hdr_len;

    if options_end > data_end {
        return None;
    }

    let mut offset = options_start;

    // Parse IP options - bounded loop for BPF verifier
    for _ in 0..40 {
        if offset >= options_end {
            break;
        }

        if offset + 1 > data_end {
            break;
        }

        let opt_type = unsafe { *(offset as *const u8) };

        // End of options
        if opt_type == 0 {
            break;
        }

        // NOP option
        if opt_type == 1 {
            offset += 1;
            continue;
        }

        // Multi-byte option
        if offset + 2 > data_end {
            break;
        }

        let opt_len = unsafe { *((offset + 1) as *const u8) } as usize;
        if opt_len < 2 || offset + opt_len > data_end {
            break;
        }

        // Check for our trace option
        if opt_type == TRACE_OPTION_TYPE && opt_len >= 18 {
            if offset + 18 <= data_end {
                let trace_data = unsafe { core::slice::from_raw_parts((offset + 2) as *const u8, 16) };
                let mut bytes = [0u8; 16];
                bytes.copy_from_slice(trace_data);
                return Some(TraceId::from_bytes(bytes));
            }
        }

        offset += opt_len;
    }

    None
}

/// Extract trace ID from TCP options.
#[inline(always)]
fn extract_tcp_trace_id(tcp_start: usize, tcp_hdr_len: usize, data_end: usize) -> Option<TraceId> {
    let options_start = tcp_start + TCP_MIN_HLEN;
    let options_end = tcp_start + tcp_hdr_len;

    if options_end > data_end {
        return None;
    }

    let mut offset = options_start;

    // Parse TCP options - bounded loop for BPF verifier
    for _ in 0..40 {
        if offset >= options_end {
            break;
        }

        if offset + 1 > data_end {
            break;
        }

        let opt_kind = unsafe { *(offset as *const u8) };

        // End of options
        if opt_kind == 0 {
            break;
        }

        // NOP option
        if opt_kind == 1 {
            offset += 1;
            continue;
        }

        // Multi-byte option
        if offset + 2 > data_end {
            break;
        }

        let opt_len = unsafe { *((offset + 1) as *const u8) } as usize;
        if opt_len < 2 || offset + opt_len > data_end {
            break;
        }

        // Check for experimental trace option (kind 253)
        if opt_kind == 253 && opt_len >= 18 {
            if offset + 18 <= data_end {
                let trace_data = unsafe { core::slice::from_raw_parts((offset + 2) as *const u8, 16) };
                let mut bytes = [0u8; 16];
                bytes.copy_from_slice(trace_data);
                return Some(TraceId::from_bytes(bytes));
            }
        }

        offset += opt_len;
    }

    None
}

/// Update flow state with trace ID.
#[inline(always)]
fn update_flow_state(flow_key: &FlowKey, trace_id: &TraceId) {
    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };

    if let Some(state) = unsafe { FLOW_STATE.get_ptr_mut(flow_key) } {
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
    if let Some(val) = unsafe { STATS.get_ptr_mut(&key) } {
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
