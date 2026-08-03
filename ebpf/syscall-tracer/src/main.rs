// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Unheaded Kingdom - Syscall Tracer Raw Tracepoint Program
//!
//! THE WHISPERING VOID audits all system calls for security.
//! This raw tracepoint program monitors syscall entry/exit
//! for security auditing and observability.
//!
//! Features:
//! - Traces syscall entry with arguments
//! - Traces syscall exit with return value
//! - Filters by process, syscall number, or user
//! - Sends audit events to userspace via ring buffer

#![no_std]
#![no_main]

use aya_ebpf::{
    cty::c_long,
    helpers::{
        bpf_get_current_comm, bpf_get_current_pid_tgid, bpf_get_current_uid_gid, bpf_ktime_get_ns,
        bpf_probe_read_kernel,
    },
    macros::{map, raw_tracepoint},
    maps::{Array, HashMap, RingBuf},
    programs::RawTracePointContext,
    EbpfContext,
};
use unheaded_common::{SyscallEvent, SyscallEventType, RING_BUFFER_SIZE};

/// Configuration for syscall filtering.
#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct FilterConfig {
    pub enabled: u32,    // Global enable flag
    pub filter_pid: u32, // Filter by specific PID (0 = all)
    pub filter_uid: u32, // Filter by specific UID (0 = all)
    pub log_all: u32,    // Log all syscalls (1) or only filtered (0)
    pub _pad: [u32; 4],
}

/// Configuration array (single element).
#[map]
static CONFIG: Array<FilterConfig> = Array::with_max_entries(1, 0);

/// Syscall filter bitmap: if bit N is set, syscall N is traced.
/// Each u64 covers 64 syscalls. 8 entries = 512 syscalls.
#[map]
static SYSCALL_FILTER: Array<u64> = Array::with_max_entries(8, 0);

/// PID filter map: if PID exists in map, it's being traced.
#[map]
static PID_FILTER: HashMap<u32, u8> = HashMap::with_max_entries(4096, 0);

/// UID filter map: if UID exists in map, it's being traced.
#[map]
static UID_FILTER: HashMap<u32, u8> = HashMap::with_max_entries(1024, 0);

/// In-flight syscalls: pid_tgid -> syscall_nr (for matching exit with entry).
#[map]
static INFLIGHT_SYSCALLS: HashMap<u64, i64> = HashMap::with_max_entries(65536, 0);

/// Ring buffer for syscall events to userspace.
#[map]
static SYSCALL_EVENTS: RingBuf = RingBuf::with_byte_size(RING_BUFFER_SIZE, 0);

/// Statistics counters.
#[map]
static SYSCALL_STATS: HashMap<u32, u64> = HashMap::with_max_entries(16, 0);

// Stats keys
const STAT_SYSCALL_ENTER: u32 = 0;
const STAT_SYSCALL_EXIT: u32 = 1;
const STAT_FILTERED_OUT: u32 = 2;
const STAT_EVENTS_SENT: u32 = 3;
const STAT_EVENTS_DROPPED: u32 = 4;

/// Raw tracepoint args for sys_enter.
/// struct trace_event_raw_sys_enter {
///     struct trace_entry ent;
///     long id;
///     unsigned long args[6];
/// }
#[repr(C)]
struct SysEnterArgs {
    _ent: [u8; 8], // trace_entry (8 bytes on most systems)
    id: c_long,
    args: [u64; 6],
}

/// Raw tracepoint args for sys_exit.
/// struct trace_event_raw_sys_exit {
///     struct trace_entry ent;
///     long id;
///     long ret;
/// }
#[repr(C)]
struct SysExitArgs {
    _ent: [u8; 8], // trace_entry
    id: c_long,
    ret: c_long,
}

/// Raw tracepoint for syscall entry.
#[raw_tracepoint]
pub fn sys_enter(ctx: RawTracePointContext) -> u32 {
    try_sys_enter(&ctx).unwrap_or_default()
}

#[inline(always)]
fn try_sys_enter(ctx: &RawTracePointContext) -> Result<u32, ()> {
    increment_stat(STAT_SYSCALL_ENTER);

    // Read tracepoint args
    let args_ptr = ctx.as_ptr() as *const SysEnterArgs;
    let args = unsafe { bpf_probe_read_kernel(args_ptr).map_err(|_| ())? };

    let syscall_nr = args.id;
    let syscall_args = args.args;

    // Get process info
    let pid_tgid = bpf_get_current_pid_tgid();
    let ids = TaskIds::current();

    // Check if we should trace this syscall
    if !should_trace(syscall_nr, ids.pid, ids.uid) {
        increment_stat(STAT_FILTERED_OUT);
        return Ok(0);
    }

    // Store for matching with exit
    let _ = INFLIGHT_SYSCALLS.insert(&pid_tgid, &syscall_nr, 0);

    // Get process name
    let comm = bpf_get_current_comm().unwrap_or_default();

    // Send event
    send_syscall_event(
        ids,
        syscall_nr,
        syscall_args,
        0, // no return value yet
        comm,
        SyscallEventType::Enter,
    );

    Ok(0)
}

/// Raw tracepoint for syscall exit.
#[raw_tracepoint]
pub fn sys_exit(ctx: RawTracePointContext) -> u32 {
    try_sys_exit(&ctx).unwrap_or_default()
}

#[inline(always)]
fn try_sys_exit(ctx: &RawTracePointContext) -> Result<u32, ()> {
    increment_stat(STAT_SYSCALL_EXIT);

    // Read tracepoint args
    let args_ptr = ctx.as_ptr() as *const SysExitArgs;
    let args = unsafe { bpf_probe_read_kernel(args_ptr).map_err(|_| ())? };

    let syscall_nr = args.id;
    let ret = args.ret;

    let pid_tgid = bpf_get_current_pid_tgid();
    let ids = TaskIds::current();

    // Check if we tracked this syscall entry
    let tracked = unsafe { INFLIGHT_SYSCALLS.get(&pid_tgid) };
    if tracked.is_none() {
        return Ok(0);
    }

    // Clean up
    let _ = INFLIGHT_SYSCALLS.remove(&pid_tgid);

    // Get process name
    let comm = bpf_get_current_comm().unwrap_or_default();

    // Send exit event (args are not available at exit, use zeros)
    send_syscall_event(ids, syscall_nr, [0; 6], ret, comm, SyscallEventType::Exit);

    Ok(0)
}

/// Check if a syscall should be traced based on filters.
#[inline(always)]
fn should_trace(syscall_nr: i64, pid: u32, uid: u32) -> bool {
    // Get config
    let config = match CONFIG.get(0) {
        Some(c) => *c,
        None => {
            // No config means trace everything
            return true;
        }
    };

    // Check global enable
    if config.enabled == 0 {
        return false;
    }

    // If logging all, skip other filters
    if config.log_all != 0 {
        return true;
    }

    // Check PID filter
    if config.filter_pid != 0 && pid != config.filter_pid {
        // Check PID filter map
        if unsafe { PID_FILTER.get(&pid) }.is_none() {
            return false;
        }
    }

    // Check UID filter
    if config.filter_uid != 0 && uid != config.filter_uid {
        // Check UID filter map
        if unsafe { UID_FILTER.get(&uid) }.is_none() {
            return false;
        }
    }

    // Check syscall bitmap filter
    if (0..512).contains(&syscall_nr) {
        let idx = (syscall_nr / 64) as u32;
        let bit = syscall_nr % 64;

        if let Some(bitmap) = SYSCALL_FILTER.get(idx) {
            // If bitmap is 0, allow all (no filter)
            // If bitmap is non-zero, check specific bit
            if *bitmap != 0 && (*bitmap & (1u64 << bit)) == 0 {
                return false;
            }
        }
    }

    true
}

/// Identity of the task that made the syscall.
///
/// Grouped rather than passed as four adjacent `u32`s: `(pid, tid, uid, gid)`
/// is exactly the shape where a transposed call site still compiles, and both
/// call sites were deriving these with the same six lines of bit-shuffling.
#[derive(Clone, Copy)]
struct TaskIds {
    pid: u32,
    tid: u32,
    uid: u32,
    gid: u32,
}

impl TaskIds {
    #[inline(always)]
    fn current() -> Self {
        let pid_tgid = bpf_get_current_pid_tgid();
        let uid_gid = bpf_get_current_uid_gid();
        Self {
            pid: (pid_tgid >> 32) as u32,
            tid: pid_tgid as u32,
            uid: uid_gid as u32,
            gid: (uid_gid >> 32) as u32,
        }
    }
}

/// Send syscall event to userspace.
#[inline(always)]
fn send_syscall_event(
    ids: TaskIds,
    syscall_nr: i64,
    args: [u64; 6],
    ret: i64,
    comm: [u8; 16],
    event_type: SyscallEventType,
) {
    let TaskIds { pid, tid, uid, gid } = ids;
    let now = unsafe { bpf_ktime_get_ns() };

    let event = SyscallEvent {
        timestamp_ns: now,
        pid,
        tid,
        uid,
        gid,
        syscall_nr,
        args,
        ret,
        comm,
        event_type,
        _pad: [0; 7],
    };

    if let Some(mut buf) = SYSCALL_EVENTS.reserve::<SyscallEvent>(0) {
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
    if let Some(val) = SYSCALL_STATS.get_ptr_mut(&key) {
        unsafe {
            *val = (*val).saturating_add(1);
        }
    } else {
        let _ = SYSCALL_STATS.insert(&key, &1u64, 0);
    }
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
