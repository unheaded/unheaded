// SPDX-License-Identifier: GPL-3.0-or-later
//! Heimdall vfs_write kprobe — observes file writes for drift detection.
//! ADR-043: Heimdall sees all changes; nothing escapes the watchman.
#![no_std]
#![no_main]

use aya_ebpf::{
    macros::{kprobe, map},
    maps::RingBuf,
    programs::ProbeContext,
};

#[repr(C)]
#[derive(Clone, Copy)]
pub struct DriftEvent {
    pub pid: u32,
    pub ts_ns: u64,
    pub event_type: u32, // 0=vfs_write, 1=execve, 2=mmap
}

#[map]
static EVENTS: RingBuf = RingBuf::with_byte_size(64 * 1024, 0);

#[kprobe]
pub fn heimdall_vfs_write(ctx: ProbeContext) -> u32 {
    let _ = try_vfs_write(ctx);
    0
}

fn try_vfs_write(_ctx: ProbeContext) -> Result<(), i64> {
    let pid = (aya_ebpf::helpers::bpf_get_current_pid_tgid() >> 32) as u32;
    let ts = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };
    if let Some(mut entry) = EVENTS.reserve::<DriftEvent>(0) {
        entry.write(DriftEvent {
            pid,
            ts_ns: ts,
            event_type: 0,
        });
        entry.submit(0);
    }
    Ok(())
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
