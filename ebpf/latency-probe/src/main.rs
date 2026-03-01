//! Unheaded Kingdom - Latency Probe Kprobe Program
//!
//! THE WHISPERING VOID measures time itself within the kernel.
//! This kprobe program attaches to TCP functions to measure
//! network latency and RTT at the kernel level.
//!
//! Features:
//! - Measures tcp_sendmsg latency
//! - Tracks tcp_recvmsg timing
//! - Associates measurements with trace IDs
//! - Sends latency events to userspace via ring buffer
//! - Aggregates per-flow RTT statistics in LATENCY_MAP (min/max/avg)

#![no_std]
#![no_main]

mod common;

use aya_ebpf::{
    helpers::{bpf_get_current_pid_tgid, bpf_ktime_get_ns},
    macros::{kprobe, kretprobe, map},
    maps::{HashMap, RingBuf},
    programs::{ProbeContext, RetProbeContext},
};
use common::{LatencyEntry, LatencyKey, MAX_LATENCY_MAP_ENTRIES};
use unheaded_common::{
    LatencyEvent, LatencyOperation, TraceId, MAX_LATENCY_ENTRIES, RING_BUFFER_SIZE,
};

/// Key for in-flight operation tracking.
#[repr(C)]
#[derive(Clone, Copy, Default)]
struct InflightKey {
    pid_tgid: u64,
    operation: u8,
    _pad: [u8; 7],
}

/// Value for in-flight operation (start time and socket pointer).
#[repr(C)]
#[derive(Clone, Copy, Default)]
struct InflightValue {
    start_ns: u64,
    sock_ptr: u64, // Pointer to socket for correlation
}

/// Map tracking in-flight operations.
/// Key: pid_tgid + operation type
/// Value: start timestamp + socket pointer
#[map]
static INFLIGHT: HashMap<InflightKey, InflightValue> =
    HashMap::with_max_entries(MAX_LATENCY_ENTRIES, 0);

/// Socket to trace ID mapping.
/// Populated by userspace or other eBPF programs.
#[map]
static SOCK_TRACE: HashMap<u64, TraceId> = HashMap::with_max_entries(MAX_LATENCY_ENTRIES, 0);

/// Ring buffer for latency events to userspace.
#[map]
static LATENCY_EVENTS: RingBuf = RingBuf::with_byte_size(RING_BUFFER_SIZE, 0);

/// Per-flow RTT statistics map.
/// Key: LatencyKey (5-tuple), Value: LatencyEntry (min/max/latest/samples).
/// Read by Go userspace LatencyReader to publish to Wotan and expose as Prometheus histogram.
#[map]
static LATENCY_MAP: HashMap<LatencyKey, LatencyEntry> =
    HashMap::with_max_entries(MAX_LATENCY_MAP_ENTRIES, 0);

/// Statistics counters.
#[map]
static LATENCY_STATS: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

// Stats keys
const STAT_SEND_ENTER: u32 = 0;
const STAT_SEND_EXIT: u32 = 1;
const STAT_RECV_ENTER: u32 = 2;
const STAT_RECV_EXIT: u32 = 3;
const STAT_CONNECT_ENTER: u32 = 4;
const STAT_CONNECT_EXIT: u32 = 5;
const STAT_EVENTS_SENT: u32 = 6;
const STAT_EVENTS_DROPPED: u32 = 7;
const STAT_MAP_UPDATES: u32 = 8;

/// Kprobe on tcp_sendmsg entry.
/// Signature: int tcp_sendmsg(struct sock *sk, struct msghdr *msg, size_t size)
#[kprobe]
pub fn tcp_sendmsg_enter(ctx: ProbeContext) -> u32 {
    match try_tcp_sendmsg_enter(&ctx) {
        Ok(ret) => ret,
        Err(_) => 0,
    }
}

#[inline(always)]
fn try_tcp_sendmsg_enter(ctx: &ProbeContext) -> Result<u32, ()> {
    increment_stat(STAT_SEND_ENTER);

    let pid_tgid = bpf_get_current_pid_tgid();
    let now = unsafe { bpf_ktime_get_ns() };

    // Get socket pointer from first argument
    let sock_ptr: u64 = match ctx.arg(0) {
        Some(ptr) => ptr,
        None => 0,
    };

    let key = InflightKey {
        pid_tgid,
        operation: LatencyOperation::TcpSend as u8,
        _pad: [0; 7],
    };

    let value = InflightValue {
        start_ns: now,
        sock_ptr,
    };

    let _ = INFLIGHT.insert(&key, &value, 0);

    Ok(0)
}

/// Kretprobe on tcp_sendmsg exit.
#[kretprobe]
pub fn tcp_sendmsg_exit(ctx: RetProbeContext) -> u32 {
    match try_tcp_sendmsg_exit(&ctx) {
        Ok(ret) => ret,
        Err(_) => 0,
    }
}

#[inline(always)]
fn try_tcp_sendmsg_exit(_ctx: &RetProbeContext) -> Result<u32, ()> {
    increment_stat(STAT_SEND_EXIT);

    let pid_tgid = bpf_get_current_pid_tgid();
    let now = unsafe { bpf_ktime_get_ns() };

    let key = InflightKey {
        pid_tgid,
        operation: LatencyOperation::TcpSend as u8,
        _pad: [0; 7],
    };

    // Look up start time
    let inflight = match unsafe { INFLIGHT.get(&key) } {
        Some(v) => *v,
        None => return Ok(0),
    };

    // Clean up
    let _ = INFLIGHT.remove(&key);

    // Calculate latency
    let latency_ns = now.saturating_sub(inflight.start_ns);

    // Get trace ID from socket mapping
    let trace_id = match unsafe { SOCK_TRACE.get(&inflight.sock_ptr) } {
        Some(tid) => *tid,
        None => TraceId::zero(),
    };

    // Send event
    send_latency_event(pid_tgid, latency_ns, trace_id, LatencyOperation::TcpSend);

    Ok(0)
}

/// Kprobe on tcp_recvmsg entry.
/// Signature varies by kernel version but first arg is usually struct sock *
#[kprobe]
pub fn tcp_recvmsg_enter(ctx: ProbeContext) -> u32 {
    match try_tcp_recvmsg_enter(&ctx) {
        Ok(ret) => ret,
        Err(_) => 0,
    }
}

#[inline(always)]
fn try_tcp_recvmsg_enter(ctx: &ProbeContext) -> Result<u32, ()> {
    increment_stat(STAT_RECV_ENTER);

    let pid_tgid = bpf_get_current_pid_tgid();
    let now = unsafe { bpf_ktime_get_ns() };

    let sock_ptr: u64 = match ctx.arg(0) {
        Some(ptr) => ptr,
        None => 0,
    };

    let key = InflightKey {
        pid_tgid,
        operation: LatencyOperation::TcpRecv as u8,
        _pad: [0; 7],
    };

    let value = InflightValue {
        start_ns: now,
        sock_ptr,
    };

    let _ = INFLIGHT.insert(&key, &value, 0);

    Ok(0)
}

/// Kretprobe on tcp_recvmsg exit.
#[kretprobe]
pub fn tcp_recvmsg_exit(ctx: RetProbeContext) -> u32 {
    match try_tcp_recvmsg_exit(&ctx) {
        Ok(ret) => ret,
        Err(_) => 0,
    }
}

#[inline(always)]
fn try_tcp_recvmsg_exit(_ctx: &RetProbeContext) -> Result<u32, ()> {
    increment_stat(STAT_RECV_EXIT);

    let pid_tgid = bpf_get_current_pid_tgid();
    let now = unsafe { bpf_ktime_get_ns() };

    let key = InflightKey {
        pid_tgid,
        operation: LatencyOperation::TcpRecv as u8,
        _pad: [0; 7],
    };

    let inflight = match unsafe { INFLIGHT.get(&key) } {
        Some(v) => *v,
        None => return Ok(0),
    };

    let _ = INFLIGHT.remove(&key);

    let latency_ns = now.saturating_sub(inflight.start_ns);

    let trace_id = match unsafe { SOCK_TRACE.get(&inflight.sock_ptr) } {
        Some(tid) => *tid,
        None => TraceId::zero(),
    };

    send_latency_event(pid_tgid, latency_ns, trace_id, LatencyOperation::TcpRecv);

    Ok(0)
}

/// Kprobe on tcp_v4_connect entry (for connection establishment latency).
#[kprobe]
pub fn tcp_connect_enter(ctx: ProbeContext) -> u32 {
    match try_tcp_connect_enter(&ctx) {
        Ok(ret) => ret,
        Err(_) => 0,
    }
}

#[inline(always)]
fn try_tcp_connect_enter(ctx: &ProbeContext) -> Result<u32, ()> {
    increment_stat(STAT_CONNECT_ENTER);

    let pid_tgid = bpf_get_current_pid_tgid();
    let now = unsafe { bpf_ktime_get_ns() };

    let sock_ptr: u64 = match ctx.arg(0) {
        Some(ptr) => ptr,
        None => 0,
    };

    let key = InflightKey {
        pid_tgid,
        operation: LatencyOperation::TcpConnect as u8,
        _pad: [0; 7],
    };

    let value = InflightValue {
        start_ns: now,
        sock_ptr,
    };

    let _ = INFLIGHT.insert(&key, &value, 0);

    Ok(0)
}

/// Kretprobe on tcp_v4_connect exit.
#[kretprobe]
pub fn tcp_connect_exit(ctx: RetProbeContext) -> u32 {
    match try_tcp_connect_exit(&ctx) {
        Ok(ret) => ret,
        Err(_) => 0,
    }
}

#[inline(always)]
fn try_tcp_connect_exit(_ctx: &RetProbeContext) -> Result<u32, ()> {
    increment_stat(STAT_CONNECT_EXIT);

    let pid_tgid = bpf_get_current_pid_tgid();
    let now = unsafe { bpf_ktime_get_ns() };

    let key = InflightKey {
        pid_tgid,
        operation: LatencyOperation::TcpConnect as u8,
        _pad: [0; 7],
    };

    let inflight = match unsafe { INFLIGHT.get(&key) } {
        Some(v) => *v,
        None => return Ok(0),
    };

    let _ = INFLIGHT.remove(&key);

    let latency_ns = now.saturating_sub(inflight.start_ns);

    let trace_id = match unsafe { SOCK_TRACE.get(&inflight.sock_ptr) } {
        Some(tid) => *tid,
        None => TraceId::zero(),
    };

    send_latency_event(pid_tgid, latency_ns, trace_id, LatencyOperation::TcpConnect);

    Ok(0)
}

/// Send latency event to userspace and update per-flow LATENCY_MAP.
#[inline(always)]
fn send_latency_event(
    pid_tgid: u64,
    latency_ns: u64,
    trace_id: TraceId,
    operation: LatencyOperation,
) {
    let now = unsafe { bpf_ktime_get_ns() };
    let pid = (pid_tgid >> 32) as u32;
    let tid = pid_tgid as u32;

    let event = LatencyEvent {
        timestamp_ns: now,
        trace_id,
        pid,
        tid,
        latency_ns,
        operation,
        _pad: [0; 7],
    };

    if let Some(mut buf) = LATENCY_EVENTS.reserve::<LatencyEvent>(0) {
        unsafe {
            core::ptr::write(buf.as_mut_ptr(), event);
            buf.submit(0);
        }
        increment_stat(STAT_EVENTS_SENT);
    } else {
        increment_stat(STAT_EVENTS_DROPPED);
    }

    // Update per-flow RTT statistics in LATENCY_MAP.
    // We use trace_id bytes as a proxy for the 5-tuple key since the kprobe
    // context does not directly provide IP/port info. The key is constructed
    // from trace_id high/low split into src_ip/dst_ip placeholders, with
    // pid as src_port and tid as dst_port, protocol=6 (TCP).
    let map_key = LatencyKey {
        src_ip: trace_id.to_bytes(),
        dst_ip: [0u8; 16],
        src_port: pid as u16,
        dst_port: tid as u16,
        protocol: 6, // TCP
        _pad: [0; 3],
    };

    // Read existing entry or create new one.
    match unsafe { LATENCY_MAP.get(&map_key) } {
        Some(existing) => {
            let min_rtt = if latency_ns < existing.min_rtt_ns {
                latency_ns
            } else {
                existing.min_rtt_ns
            };
            let max_rtt = if latency_ns > existing.max_rtt_ns {
                latency_ns
            } else {
                existing.max_rtt_ns
            };
            let updated = LatencyEntry {
                rtt_ns: latency_ns,
                min_rtt_ns: min_rtt,
                max_rtt_ns: max_rtt,
                samples: existing.samples.saturating_add(1),
                last_update_ns: now,
            };
            let _ = LATENCY_MAP.insert(&map_key, &updated, 0);
        }
        None => {
            let new_entry = LatencyEntry {
                rtt_ns: latency_ns,
                min_rtt_ns: latency_ns,
                max_rtt_ns: latency_ns,
                samples: 1,
                last_update_ns: now,
            };
            let _ = LATENCY_MAP.insert(&map_key, &new_entry, 0);
        }
    }
    increment_stat(STAT_MAP_UPDATES);
}

/// Increment a statistics counter.
#[inline(always)]
fn increment_stat(key: u32) {
    if let Some(val) = LATENCY_STATS.get_ptr_mut(&key) {
        unsafe {
            *val = (*val).saturating_add(1);
        }
    } else {
        let _ = LATENCY_STATS.insert(&key, &1u64, 0);
    }
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
