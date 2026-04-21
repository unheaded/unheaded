// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 HIP kernel FFI — host-side wrappers for custom kernels compiled
// via hipcc (see `build.rs`). All kernels expose an `extern "C"` launch
// function; Rust calls those directly (no dlopen needed — see ADR-049).
//
// Error handling: every launcher returns `hipError_t` which we map to
// `Result<(), String>` to match the existing `hip.rs` style.

pub mod attn;
pub mod gelu;
pub mod identity;
pub mod rmsnorm;
pub mod rope;
pub mod softmax;

/// Map hipError_t (i32) to Result. 0 = hipSuccess.
pub fn check_hip(err: i32, op: &str) -> Result<(), String> {
    if err == 0 {
        Ok(())
    } else {
        Err(format!("wave11 {} failed: hipError={}", op, err))
    }
}
