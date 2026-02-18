//! TCP latency probe — measures tcp_sendmsg/tcp_recvmsg call duration.
//!
//! Attaches kprobe to tcp_sendmsg entry and kretprobe to return.
//! On entry: stores bpf_ktime_get_ns() keyed by pid_tgid.
//! On return: computes delta_ns, emits LatencyEvent to ring buffer.
//! Same pattern for tcp_recvmsg.

#![no_std]
#![no_main]

use aya_ebpf::{
    macros::{kprobe, kretprobe, map},
    maps::{HashMap, RingBuf},
    programs::{ProbeContext, RetProbeContext},
};
use aya_log_ebpf::info;
use ebpf_common::{LatencyEvent, OP_TCP_SEND, OP_TCP_RECV, RING_BUF_SIZE};

/// Inflight tracking: pid_tgid → start timestamp (ns)
#[map]
static INFLIGHT: HashMap<u64, u64> = HashMap::with_max_entries(8192, 0);

/// Ring buffer for latency events
#[map]
static LATENCY_EVENTS: RingBuf = RingBuf::with_byte_size(RING_BUF_SIZE, 0);

// --- tcp_sendmsg ---

#[kprobe]
pub fn tcp_sendmsg_enter(ctx: ProbeContext) -> u32 {
    match try_entry(&ctx) {
        Ok(ret) => ret,
        Err(_) => 0,
    }
}

#[kretprobe]
pub fn tcp_sendmsg_exit(ctx: RetProbeContext) -> u32 {
    match try_exit(OP_TCP_SEND) {
        Ok(ret) => ret,
        Err(_) => 0,
    }
}

// --- tcp_recvmsg ---

#[kprobe]
pub fn tcp_recvmsg_enter(ctx: ProbeContext) -> u32 {
    match try_entry(&ctx) {
        Ok(ret) => ret,
        Err(_) => 0,
    }
}

#[kretprobe]
pub fn tcp_recvmsg_exit(ctx: RetProbeContext) -> u32 {
    match try_exit(OP_TCP_RECV) {
        Ok(ret) => ret,
        Err(_) => 0,
    }
}

// --- Shared entry/exit logic ---

#[inline(always)]
fn try_entry(_ctx: &ProbeContext) -> Result<u32, ()> {
    let pid_tgid = unsafe { aya_ebpf::helpers::bpf_get_current_pid_tgid() };
    let ts = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };

    // Store start time keyed by pid_tgid
    INFLIGHT.insert(&pid_tgid, &ts, 0).map_err(|_| ())?;

    Ok(0)
}

#[inline(always)]
fn try_exit(_ctx: &ProbeContext, operation: u8) -> Result<u32, ()> {
    let pid_tgid = unsafe { aya_ebpf::helpers::bpf_get_current_pid_tgid() };
    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };

    // Look up start time
    let start = unsafe { INFLIGHT.get(&pid_tgid) }.ok_or(())?;
    let delta_ns = now.saturating_sub(*start);

    // Remove from inflight map
    let _ = INFLIGHT.remove(&pid_tgid);

    // Extract PID and TID from pid_tgid
    let pid = (pid_tgid >> 32) as u32;
    let tid = pid_tgid as u32;

    let event = LatencyEvent {
        timestamp_ns: now,
        pid,
        tid,
        latency_ns: delta_ns,
        operation,
        _pad: [0u8; 7],
    };

    // Emit to ring buffer
    if let Some(mut buf) = LATENCY_EVENTS.reserve::<LatencyEvent>(0) {
        unsafe {
            buf.write(event);
        }
        buf.submit(0);
    }

    Ok(0)
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
