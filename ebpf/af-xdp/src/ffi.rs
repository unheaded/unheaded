// SPDX-License-Identifier: GPL-2.0-only
// AF_XDP Bridge — Rust FFI module
// Part of: Unheaded Kingdom project
//
// C-compatible FFI bindings for Go/C interop.
// All functions use `extern "C"` ABI with opaque handles.
// Safety invariants enforced by caller (C/Go) — pointer validity is
// checked via null guards at each entry point.
#![allow(clippy::not_unsafe_ptr_arg_deref)]

use crate::engine::{XdpEngine, EngineStats};
use std::ffi::CStr;
use std::os::raw::{c_char, c_int, c_uint};
use std::ptr;

// ── Error codes ──────────────────────────────────────────────────────────────

pub const AFXDP_OK: c_int = 0;
pub const AFXDP_ERR_INVALID_ARGS: c_int = -1;
pub const AFXDP_ERR_NO_MEMORY: c_int = -2;
pub const AFXDP_ERR_SOCKET: c_int = -3;
pub const AFXDP_ERR_UMEM: c_int = -4;

// ── Opaque handle ────────────────────────────────────────────────────────────

/// Opaque handle wrapping the XDP engine.
/// Allocated on heap, returned as raw pointer to C/Go caller.
#[repr(C)]
pub struct AfxdpHandle {
    engine: *mut XdpEngine,
}

/// Statistics structure matching C header layout.
#[repr(C)]
pub struct AfxdpStats {
    pub rx_packets: u64,
    pub tx_packets: u64,
    pub rx_bytes: u64,
    pub tx_bytes: u64,
    pub rx_drops: u64,
    pub fill_empty: u64,
    pub comp_full: u64,
}

// ── FFI functions ────────────────────────────────────────────────────────────

/// Create AF_XDP engine on interface and queue.
/// Returns opaque handle or NULL on error.
///
/// # Safety
/// `iface` must be a valid null-terminated C string.
#[no_mangle]
pub extern "C" fn afxdp_create(
    iface: *const c_char,
    queue: c_uint,
    frame_count: c_uint,
) -> *mut AfxdpHandle {
    if iface.is_null() {
        return ptr::null_mut();
    }

    let iface_str = match unsafe { CStr::from_ptr(iface).to_str() } {
        Ok(s) => s,
        Err(_) => return ptr::null_mut(),
    };

    match XdpEngine::new(iface_str, queue as u32, frame_count as u32) {
        Ok(engine) => {
            Box::into_raw(Box::new(AfxdpHandle {
                engine: Box::into_raw(Box::new(engine)),
            }))
        }
        Err(_) => ptr::null_mut(),
    }
}

/// Receive packets into caller-provided buffer.
/// Returns number of bytes written, or negative error code.
///
/// # Safety
/// `handle` must be a valid handle from `afxdp_create`.
/// `buf` must point to a buffer of at least `buf_size` bytes.
#[no_mangle]
pub extern "C" fn afxdp_recv(
    handle: *mut AfxdpHandle,
    buf: *mut c_char,
    buf_size: c_uint,
    batch_size: c_uint,
) -> c_int {
    if handle.is_null() || buf.is_null() {
        return AFXDP_ERR_INVALID_ARGS;
    }

    let engine = unsafe { &mut *(*handle).engine };
    let packets = engine.rx_burst(batch_size);

    // Copy packet data into caller's buffer
    let mut offset: usize = 0;
    let buf_limit = buf_size as usize;

    for pkt in &packets {
        let pkt_len = pkt.len as usize;
        if offset + pkt_len > buf_limit {
            break;
        }

        let src = engine.frame_ptr(pkt.addr);
        unsafe {
            ptr::copy_nonoverlapping(
                src,
                buf.add(offset) as *mut u8,
                pkt_len,
            );
        }
        offset += pkt_len;

        // Free the frame back to UMEM after copying
        engine.free_frame(pkt.addr);
    }

    offset as c_int
}

/// Send packet data from buffer.
/// Returns number of packets sent, or negative error code.
///
/// # Safety
/// `handle` must be a valid handle from `afxdp_create`.
/// `buf` must point to valid data of `buf_size` bytes.
#[no_mangle]
pub extern "C" fn afxdp_send(
    handle: *mut AfxdpHandle,
    buf: *const c_char,
    buf_size: c_uint,
) -> c_int {
    if handle.is_null() || buf.is_null() {
        return AFXDP_ERR_INVALID_ARGS;
    }

    let engine = unsafe { &mut *(*handle).engine };

    // Allocate a UMEM frame and copy data into it
    let addr = match engine.alloc_frame() {
        Some(a) => a,
        None => return AFXDP_ERR_NO_MEMORY,
    };

    let dst = engine.frame_ptr(addr);
    let len = buf_size as usize;
    unsafe {
        ptr::copy_nonoverlapping(buf as *const u8, dst, len);
    }

    let pkt = crate::engine::PacketBuf {
        addr,
        len: buf_size as u32,
    };

    engine.tx_burst(&[pkt]) as c_int
}

/// Get statistics snapshot.
/// Returns 0 on success, negative error code on failure.
///
/// # Safety
/// `handle` and `stats` must be valid pointers.
#[no_mangle]
pub extern "C" fn afxdp_stats(
    handle: *mut AfxdpHandle,
    stats: *mut AfxdpStats,
) -> c_int {
    if handle.is_null() || stats.is_null() {
        return AFXDP_ERR_INVALID_ARGS;
    }

    let engine = unsafe { &*(*handle).engine };
    let s = engine.stats();

    unsafe {
        (*stats).rx_packets = s.rx_packets;
        (*stats).tx_packets = s.tx_packets;
        (*stats).rx_bytes = s.rx_bytes;
        (*stats).tx_bytes = s.tx_bytes;
        (*stats).rx_drops = s.rx_drops;
        (*stats).fill_empty = s.fill_empty;
        (*stats).comp_full = s.comp_full;
    }

    AFXDP_OK
}

/// Poll for RX readiness with timeout.
/// Returns 1 if ready, 0 if timeout, negative on error.
///
/// # Safety
/// `handle` must be a valid handle from `afxdp_create`.
#[no_mangle]
pub extern "C" fn afxdp_poll(
    handle: *mut AfxdpHandle,
    timeout_ms: c_int,
) -> c_int {
    if handle.is_null() {
        return AFXDP_ERR_INVALID_ARGS;
    }

    let engine = unsafe { &*(*handle).engine };

    match engine.poll(timeout_ms) {
        Ok(true) => 1,
        Ok(false) => 0,
        Err(_) => AFXDP_ERR_SOCKET,
    }
}

/// Destroy handle and free all resources.
/// Returns 0 on success, negative on error.
///
/// # Safety
/// `handle` must be a valid handle from `afxdp_create` (or NULL, which is a no-op returning error).
/// After this call, the handle is invalid and must not be reused.
#[no_mangle]
pub extern "C" fn afxdp_destroy(handle: *mut AfxdpHandle) -> c_int {
    if handle.is_null() {
        return AFXDP_ERR_INVALID_ARGS;
    }

    let handle_box = unsafe { Box::from_raw(handle) };
    let _ = unsafe { Box::from_raw(handle_box.engine) };

    AFXDP_OK
}
