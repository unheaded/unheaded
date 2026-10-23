// SPDX-License-Identifier: GPL-3.0-or-later
//! Heimdall execve tracepoint — observes program executions for drift
//! detection (ADR-043 K1 "all hooks", S5 Phase 5).
//!
//! A raw tracepoint on `sched_process_exec`, which fires once per successful
//! exec, after the new image is in place. Raw, not a classic tracepoint: a
//! raw tracepoint attaches by name through BPF_RAW_TRACEPOINT_OPEN and needs
//! no tracefs, so the hook also loads where tracefs is not mounted (a fresh
//! mount namespace, `ip netns exec`, most containers). Emits the same 24-byte record as
//! the vfs_write kprobe with `event_type = 1`, so heimdall-daemon decodes both
//! ringbufs with one decoder.
#![no_std]
#![no_main]

use aya_ebpf::{
    helpers::{bpf_get_current_pid_tgid, bpf_ktime_get_ns},
    macros::{map, raw_tracepoint},
    maps::RingBuf,
    programs::RawTracePointContext,
};

/// Same memory layout as `DriftEvent` in vfs_write.rs (pid @0, ts_ns @8,
/// event_type @16, size 24), with its padding named so every byte is
/// written explicitly.
#[repr(C)]
pub struct DriftEvent {
    pub pid: u32,
    pub pad0: u32,
    pub ts_ns: u64,
    pub event_type: u32, // 0=vfs_write, 1=execve, 2=mmap
    pub pad1: u32,
}

#[map]
static EVENTS: RingBuf = RingBuf::with_byte_size(64 * 1024, 0);

#[raw_tracepoint(tracepoint = "sched_process_exec")]
pub fn heimdall_execve(_ctx: RawTracePointContext) -> i32 {
    if let Some(mut entry) = EVENTS.reserve::<DriftEvent>(0) {
        let ev = entry.as_mut_ptr();
        unsafe {
            (*ev).pid = (bpf_get_current_pid_tgid() >> 32) as u32;
            (*ev).pad0 = 0;
            (*ev).ts_ns = bpf_ktime_get_ns();
            (*ev).event_type = 1;
            (*ev).pad1 = 0;
        }
        entry.submit(0);
    }
    0
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
