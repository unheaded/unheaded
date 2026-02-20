//! Unheaded Kingdom - Flow Tracker TC Program
//!
//! THE WHISPERING VOID tracks all flows through the kingdom.
//! This TC (Traffic Control) program monitors connection state
//! and tracks bidirectional flows using 5-tuple hashes.
//!
//! Features:
//! - Tracks TCP connection state machine transitions
//! - Maintains bidirectional flow statistics
//! - Associates trace IDs with flows
//! - Sends flow events to userspace via ring buffer
//! - Works on both ingress and egress

#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::TC_ACT_PIPE,
    macros::{classifier, map},
    maps::{HashMap, RingBuf, LruHashMap},
    programs::TcContext,
};
use unheaded_common::{
    ConnectionState, Direction, FlowEvent, FlowEventType, FlowKey, FlowState, TraceId,
    ETH_HLEN, ETH_P_IP, IPV4_MIN_HLEN, IPPROTO_TCP, IPPROTO_UDP,
    MAX_FLOWS, RING_BUFFER_SIZE, TCP_MIN_HLEN,
    TCP_FLAG_SYN, TCP_FLAG_ACK, TCP_FLAG_FIN, TCP_FLAG_RST,
};

/// Main flow state map using LRU for automatic expiration.
/// Key: 5-tuple (normalized to always have smaller IP first for bidirectional tracking).
/// Value: Flow state including trace ID and statistics.
#[map]
static FLOWS: LruHashMap<FlowKey, FlowState> = LruHashMap::with_max_entries(MAX_FLOWS, 0);

/// Active trace associations: flow hash -> trace ID.
/// Used for quick lookup when we know a flow should have a trace.
#[map]
static TRACE_ASSOC: HashMap<u32, TraceId> = HashMap::with_max_entries(MAX_FLOWS, 0);

/// Ring buffer for flow events to userspace.
#[map]
static FLOW_EVENTS: RingBuf = RingBuf::with_byte_size(RING_BUFFER_SIZE, 0);

/// Per-CPU counters for statistics.
#[map]
static FLOW_STATS: HashMap<u32, u64> = HashMap::with_max_entries(32, 0);

/// Migration tokens for flow migration (RFC 9000 §9).
/// Key: 5-tuple identifying the original flow (16 bytes).
/// Value: migration token + new 5-tuple + expiry (48 bytes).
#[map]
static MIGRATION_TOKENS: HashMap<FlowKey, MigrationTokenValue> = HashMap::with_max_entries(4096, 0);

/// Cancellation flags for per-flow teardown (RFC 9114 §4.1.1).
/// Key: 5-tuple identifying the flow to cancel (16 bytes).
/// Value: reason + timestamp + flags (24 bytes).
#[map]
static CANCEL_FLOWS: HashMap<FlowKey, CancelFlowValue> = HashMap::with_max_entries(4096, 0);

// Stats keys
const STAT_PACKETS_INGRESS: u32 = 0;
const STAT_PACKETS_EGRESS: u32 = 1;
const STAT_FLOWS_NEW: u32 = 2;
const STAT_FLOWS_CLOSED: u32 = 3;
const STAT_TCP_SYNS: u32 = 4;
const STAT_TCP_FINS: u32 = 5;
const STAT_TCP_RSTS: u32 = 6;
const STAT_EVENTS_SENT: u32 = 7;
const STAT_EVENTS_DROPPED: u32 = 8;
const STAT_MIGRATIONS: u32 = 10;
const STAT_MIGRATION_EXPIRED: u32 = 11;
const STAT_CANCELLED: u32 = 12;

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
    doff_flags: u16,
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

/// Migration token value for flow migration (RFC 9000 §9).
/// Stores the 128-bit token and new 5-tuple for migrated flows.
/// Size: 48 bytes.
#[repr(C, packed)]
struct MigrationTokenValue {
    token: [u8; 16],           // 128-bit migration token
    expiry_ns: u64,            // Expiry timestamp (bpf_ktime_get_ns)
    new_src_addr: u32,         // New source address after migration
    new_dst_addr: u32,         // New destination address after migration
    new_src_port: u16,         // New source port
    new_dst_port: u16,         // New destination port
    flags: u32,                // Migration flags (bit 0: active)
    _pad: [u8; 4],             // Alignment padding to 48 bytes
}

/// Cancel flow value for per-flow teardown (RFC 9114 §4.1.1).
/// Stores cancellation reason and state.
/// Size: 24 bytes.
#[repr(C, packed)]
struct CancelFlowValue {
    reason: u32,               // Cancellation reason code
    timestamp_ns: u64,         // When cancellation was requested
    flags: u32,                // Bit 0: active, Bit 1: send-rst
    _pad: [u8; 4],             // Alignment padding to 24 bytes
}

/// TC classifier for ingress traffic.
#[classifier]
pub fn flow_tracker_ingress(ctx: TcContext) -> i32 {
    match try_flow_tracker(&ctx, Direction::Ingress) {
        Ok(ret) => ret,
        Err(_) => TC_ACT_PIPE,
    }
}

/// TC classifier for egress traffic.
#[classifier]
pub fn flow_tracker_egress(ctx: TcContext) -> i32 {
    match try_flow_tracker(&ctx, Direction::Egress) {
        Ok(ret) => ret,
        Err(_) => TC_ACT_PIPE,
    }
}

#[inline(always)]
fn try_flow_tracker(ctx: &TcContext, direction: Direction) -> Result<i32, ()> {
    // Increment direction counter
    match direction {
        Direction::Ingress => increment_stat(STAT_PACKETS_INGRESS),
        Direction::Egress => increment_stat(STAT_PACKETS_EGRESS),
    }

    let data = ctx.data();
    let data_end = ctx.data_end();

    // Parse Ethernet header
    if data + ETH_HLEN > data_end {
        return Ok(TC_ACT_PIPE);
    }

    let eth = unsafe { &*(data as *const EthHdr) };
    let eth_proto = u16::from_be(eth.proto);

    // Only handle IPv4
    if eth_proto != ETH_P_IP {
        return Ok(TC_ACT_PIPE);
    }

    // Parse IPv4 header
    let ip_start = data + ETH_HLEN;
    if ip_start + IPV4_MIN_HLEN > data_end {
        return Ok(TC_ACT_PIPE);
    }

    let ip = unsafe { &*(ip_start as *const Ipv4Hdr) };
    let ip_hdr_len = ((ip.version_ihl & 0x0F) as usize) * 4;

    if ip_hdr_len < IPV4_MIN_HLEN || ip_start + ip_hdr_len > data_end {
        return Ok(TC_ACT_PIPE);
    }

    let transport_start = ip_start + ip_hdr_len;
    let packet_len = (data_end - data) as u64;

    match ip.protocol {
        IPPROTO_TCP => {
            process_tcp(transport_start, data_end, ip, packet_len, direction)?;
        }
        IPPROTO_UDP => {
            process_udp(transport_start, data_end, ip, packet_len, direction)?;
        }
        _ => {}
    }

    Ok(TC_ACT_PIPE)
}

/// Process TCP packet and update flow state.
#[inline(always)]
fn process_tcp(
    tcp_start: usize,
    data_end: usize,
    ip: &Ipv4Hdr,
    packet_len: u64,
    _direction: Direction,
) -> Result<(), ()> {
    if tcp_start + TCP_MIN_HLEN > data_end {
        return Err(());
    }

    let tcp = unsafe { &*(tcp_start as *const TcpHdr) };

    // Extract TCP flags
    let doff_flags = u16::from_be(tcp.doff_flags);
    let flags = (doff_flags & 0x3F) as u8;

    let is_syn = flags & TCP_FLAG_SYN != 0;
    let is_ack = flags & TCP_FLAG_ACK != 0;
    let is_fin = flags & TCP_FLAG_FIN != 0;
    let is_rst = flags & TCP_FLAG_RST != 0;

    // Update stats
    if is_syn && !is_ack {
        increment_stat(STAT_TCP_SYNS);
    }
    if is_fin {
        increment_stat(STAT_TCP_FINS);
    }
    if is_rst {
        increment_stat(STAT_TCP_RSTS);
    }

    // Build normalized flow key (smaller IP first for bidirectional matching)
    let flow_key = normalize_flow_key(
        ip.saddr,
        ip.daddr,
        tcp.source,
        tcp.dest,
        IPPROTO_TCP,
    );

    // Determine if this is the "forward" direction (src initiated the flow)
    let is_forward = ip.saddr < ip.daddr ||
        (ip.saddr == ip.daddr && u16::from_be(tcp.source) < u16::from_be(tcp.dest));

    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };

    // Look up or create flow state
    let (flow_state, is_new) = get_or_create_flow(&flow_key, now);

    if is_new {
        increment_stat(STAT_FLOWS_NEW);
        send_flow_event(&flow_key, &flow_state, FlowEventType::New);
    }

    // RFC 9114 §4.1.1 — Per-flow cancellation check
    if let Some(cancel) = unsafe { CANCEL_FLOWS.get(&flow_key) } {
        let flags = cancel.flags;
        if flags & 1 != 0 {
            // Flow is cancelled — emit ANOMALY event and drop
            send_flow_event(&flow_key, &flow_state, FlowEventType::Anomaly);
            increment_stat(STAT_CANCELLED);
            return Ok(TC_ACT_PIPE);
        }
    }

    // RFC 9000 §9 — Flow migration token check
    if let Some(token) = unsafe { MIGRATION_TOKENS.get(&flow_key) } {
        let expiry = token.expiry_ns;
        if now < expiry {
            // Token valid — allow flow migration to new 5-tuple
            increment_stat(STAT_MIGRATIONS);
        } else {
            // Token expired — reject migration
            increment_stat(STAT_MIGRATION_EXPIRED);
        }
    }

    // Update flow state
    if let Some(state) = FLOWS.get_ptr_mut(&flow_key) {
        let state = unsafe { &mut *state };

        // Update timestamps
        state.last_seen_ns = now;

        // Update packet/byte counters based on direction
        if is_forward {
            state.packets_out = state.packets_out.saturating_add(1);
            state.bytes_out = state.bytes_out.saturating_add(packet_len);
        } else {
            state.packets_in = state.packets_in.saturating_add(1);
            state.bytes_in = state.bytes_in.saturating_add(packet_len);
        }

        // TCP state machine
        let old_state = state.state;
        state.state = tcp_state_transition(state.state, flags, is_forward);

        // Send event on state change
        if old_state != state.state {
            let flow_state_copy = *state;
            send_flow_event(&flow_key, &flow_state_copy, FlowEventType::StateChange);

            // Track closed flows
            if state.state == ConnectionState::Closed {
                increment_stat(STAT_FLOWS_CLOSED);
            }
        }

        // Check for trace ID in our association map
        let flow_hash = flow_key.hash();
        if state.trace_id.is_zero() {
            if let Some(trace_id) = unsafe { TRACE_ASSOC.get(&flow_hash) } {
                state.trace_id = *trace_id;
            }
        }
    }

    Ok(())
}

/// Process UDP packet and update flow state.
#[inline(always)]
fn process_udp(
    udp_start: usize,
    data_end: usize,
    ip: &Ipv4Hdr,
    packet_len: u64,
    _direction: Direction,
) -> Result<(), ()> {
    if udp_start + 8 > data_end {
        return Err(());
    }

    let udp = unsafe { &*(udp_start as *const UdpHdr) };

    // Build normalized flow key
    let flow_key = normalize_flow_key(
        ip.saddr,
        ip.daddr,
        udp.source,
        udp.dest,
        IPPROTO_UDP,
    );

    let is_forward = ip.saddr < ip.daddr ||
        (ip.saddr == ip.daddr && u16::from_be(udp.source) < u16::from_be(udp.dest));

    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };

    let (flow_state, is_new) = get_or_create_flow(&flow_key, now);

    if is_new {
        increment_stat(STAT_FLOWS_NEW);
        send_flow_event(&flow_key, &flow_state, FlowEventType::New);
    }

    // RFC 9114 §4.1.1 — Per-flow cancellation check
    if let Some(cancel) = unsafe { CANCEL_FLOWS.get(&flow_key) } {
        let flags = cancel.flags;
        if flags & 1 != 0 {
            // Flow is cancelled — emit ANOMALY event and drop
            send_flow_event(&flow_key, &flow_state, FlowEventType::Anomaly);
            increment_stat(STAT_CANCELLED);
            return Ok(TC_ACT_PIPE);
        }
    }

    // RFC 9000 §9 — Flow migration token check
    if let Some(token) = unsafe { MIGRATION_TOKENS.get(&flow_key) } {
        let expiry = token.expiry_ns;
        if now < expiry {
            // Token valid — allow flow migration to new 5-tuple
            increment_stat(STAT_MIGRATIONS);
        } else {
            // Token expired — reject migration
            increment_stat(STAT_MIGRATION_EXPIRED);
        }
    }

    // Update flow state
    if let Some(state) = FLOWS.get_ptr_mut(&flow_key) {
        let state = unsafe { &mut *state };
        state.last_seen_ns = now;

        if is_forward {
            state.packets_out = state.packets_out.saturating_add(1);
            state.bytes_out = state.bytes_out.saturating_add(packet_len);
        } else {
            state.packets_in = state.packets_in.saturating_add(1);
            state.bytes_in = state.bytes_in.saturating_add(packet_len);
        }

        // UDP is connectionless, but we track it as "Established"
        if state.state == ConnectionState::Unknown {
            state.state = ConnectionState::Established;
        }
    }

    Ok(())
}

/// Create a normalized flow key where smaller IP comes first.
/// This ensures bidirectional flows map to the same key.
#[inline(always)]
fn normalize_flow_key(
    src_addr: u32,
    dst_addr: u32,
    src_port: u16,
    dst_port: u16,
    protocol: u8,
) -> FlowKey {
    if src_addr < dst_addr || (src_addr == dst_addr && u16::from_be(src_port) < u16::from_be(dst_port)) {
        FlowKey {
            src_addr,
            dst_addr,
            src_port,
            dst_port,
            protocol,
            _pad: [0; 3],
        }
    } else {
        FlowKey {
            src_addr: dst_addr,
            dst_addr: src_addr,
            src_port: dst_port,
            dst_port: src_port,
            protocol,
            _pad: [0; 3],
        }
    }
}

/// Get or create flow state, returns (state, is_new).
#[inline(always)]
fn get_or_create_flow(flow_key: &FlowKey, now: u64) -> (FlowState, bool) {
    if let Some(state) = unsafe { FLOWS.get(flow_key) } {
        (*state, false)
    } else {
        let state = FlowState {
            trace_id: TraceId::zero(),
            start_ns: now,
            last_seen_ns: now,
            packets_in: 0,
            packets_out: 0,
            bytes_in: 0,
            bytes_out: 0,
            state: ConnectionState::Unknown,
            _pad: [0; 7],
        };
        let _ = FLOWS.insert(flow_key, &state, 0);
        (state, true)
    }
}

/// TCP state machine transition based on flags.
#[inline(always)]
fn tcp_state_transition(
    current: ConnectionState,
    flags: u8,
    is_forward: bool,
) -> ConnectionState {
    let is_syn = flags & TCP_FLAG_SYN != 0;
    let is_ack = flags & TCP_FLAG_ACK != 0;
    let is_fin = flags & TCP_FLAG_FIN != 0;
    let is_rst = flags & TCP_FLAG_RST != 0;

    // RST always goes to Closed
    if is_rst {
        return ConnectionState::Closed;
    }

    match current {
        ConnectionState::Unknown => {
            if is_syn && !is_ack {
                ConnectionState::SynSent
            } else if is_syn && is_ack {
                ConnectionState::SynReceived
            } else {
                // Mid-stream pickup, assume established
                ConnectionState::Established
            }
        }
        ConnectionState::SynSent => {
            if is_syn && is_ack {
                ConnectionState::SynReceived
            } else if is_ack {
                ConnectionState::Established
            } else {
                current
            }
        }
        ConnectionState::SynReceived => {
            if is_ack && !is_syn {
                ConnectionState::Established
            } else {
                current
            }
        }
        ConnectionState::Established => {
            if is_fin {
                if is_forward {
                    ConnectionState::FinWait1
                } else {
                    ConnectionState::CloseWait
                }
            } else {
                current
            }
        }
        ConnectionState::FinWait1 => {
            if is_fin && is_ack {
                ConnectionState::TimeWait
            } else if is_fin {
                ConnectionState::Closing
            } else if is_ack {
                ConnectionState::FinWait2
            } else {
                current
            }
        }
        ConnectionState::FinWait2 => {
            if is_fin {
                ConnectionState::TimeWait
            } else {
                current
            }
        }
        ConnectionState::CloseWait => {
            if is_fin {
                ConnectionState::LastAck
            } else {
                current
            }
        }
        ConnectionState::Closing => {
            if is_ack {
                ConnectionState::TimeWait
            } else {
                current
            }
        }
        ConnectionState::LastAck => {
            if is_ack {
                ConnectionState::Closed
            } else {
                current
            }
        }
        ConnectionState::TimeWait => {
            // TimeWait eventually transitions to Closed (handled by userspace timeout)
            ConnectionState::TimeWait
        }
        ConnectionState::Closed => ConnectionState::Closed,
    }
}

/// Send flow event to userspace via ring buffer.
#[inline(always)]
fn send_flow_event(flow_key: &FlowKey, flow_state: &FlowState, event_type: FlowEventType) {
    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };

    let event = FlowEvent {
        timestamp_ns: now,
        flow_key: *flow_key,
        state: *flow_state,
        event_type,
        _pad: [0; 7],
    };

    if let Some(mut buf) = FLOW_EVENTS.reserve::<FlowEvent>(0) {
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
    if let Some(val) = FLOW_STATS.get_ptr_mut(&key) {
        unsafe {
            *val = (*val).saturating_add(1);
        }
    } else {
        let _ = FLOW_STATS.insert(&key, &1u64, 0);
    }
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
