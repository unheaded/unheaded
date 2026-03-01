// SPDX-License-Identifier: GPL-2.0-only
// AF_XDP socket (XskSocket) — RX/TX rings with kernel bypass
//
// Phase 3 will expand this stub into full implementation.

use af_xdp_common::XskDesc;

/// AF_XDP socket for zero-copy packet I/O (stub — Phase 3).
pub struct XskSocket {
    _private: (),
}

impl XskSocket {
    /// Create new AF_XDP socket (stub for Phase 3).
    pub fn new(_ifname: &str, _queue_id: u32) -> Result<Self, &'static str> {
        Err("not yet implemented — Phase 3")
    }
}
