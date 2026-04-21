// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 identity-kernel Rust wrapper — smoke test for the HIP build pipeline.
// If this doesn't launch correctly, nothing else in WAVE11 will.

use crate::hip::GpuBuffer;

// Declared in kernels/identity.hip.cpp. Launched on the default stream (null).
// Signature matches the C prototype:
//   extern "C" hipError_t wave11_launch_identity_f32(
//       float* out, const float* in, int n, hipStream_t stream);
extern "C" {
    fn wave11_launch_identity_f32(
        out: *mut f32,
        input: *const f32,
        n: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
}

/// Copy `in_buf` → `out_buf` via a GPU kernel. Same-length f32 buffers.
/// Returns Ok(()) on success. Primarily for testing the build pipeline.
pub fn launch_identity(
    out_buf: &GpuBuffer,
    in_buf: &GpuBuffer,
    n: usize,
) -> Result<(), String> {
    // GpuBuffer::len() returns size in bytes. At 4 bytes per f32 we need
    // n_bytes ≥ n*4.
    let required_bytes = n * 4;
    if out_buf.len() < required_bytes || in_buf.len() < required_bytes {
        return Err(format!(
            "identity: buffers too small (need {} bytes, have out={}, in={})",
            required_bytes, out_buf.len(), in_buf.len(),
        ));
    }
    let err = unsafe {
        wave11_launch_identity_f32(
            out_buf.as_ptr() as *mut f32,
            in_buf.as_ptr() as *const f32,
            n as i32,
            std::ptr::null_mut(), // default stream
        )
    };
    super::check_hip(err, "launch_identity_f32")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::hip::GpuBuffer;

    #[test]
    fn test_identity_kernel_heartbeat() {
        // First real kernel launch. If this passes, the WAVE11 pipeline
        // (build.rs → hipcc → .so → Rust FFI → launch) is operational.
        let input = vec![1.0f32, 2.0, 3.0, 4.0];
        let n = input.len();

        let in_buf = GpuBuffer::alloc(n * 4).expect("alloc in");
        let out_buf = GpuBuffer::alloc(n * 4).expect("alloc out");

        // Upload input bytes.
        let in_bytes = unsafe {
            std::slice::from_raw_parts(input.as_ptr() as *const u8, n * 4)
        };
        in_buf.copy_from_host(in_bytes).expect("upload");

        // Launch kernel.
        launch_identity(&out_buf, &in_buf, n).expect("launch");

        // Download output.
        let mut output = vec![0.0f32; n];
        out_buf.download_f32(&mut output).expect("download");

        assert_eq!(output, input, "WAVE11 identity kernel: output != input");
        println!("WAVE11 identity kernel heartbeat: {:?} -> {:?}", input, output);
    }

    #[test]
    fn test_identity_kernel_1m_elements() {
        // Stress the launch config with 1M elements. Catches grid-size /
        // bounds issues that the 4-element test misses.
        let n = 1_000_000usize;
        let mut input = Vec::with_capacity(n);
        for i in 0..n {
            input.push(i as f32 * 0.001);
        }

        let in_buf = GpuBuffer::alloc(n * 4).expect("alloc in");
        let out_buf = GpuBuffer::alloc(n * 4).expect("alloc out");

        let in_bytes = unsafe {
            std::slice::from_raw_parts(input.as_ptr() as *const u8, n * 4)
        };
        in_buf.copy_from_host(in_bytes).expect("upload");

        launch_identity(&out_buf, &in_buf, n).expect("launch");

        let mut output = vec![0.0f32; n];
        out_buf.download_f32(&mut output).expect("download");

        // Spot-check first, middle, last.
        assert_eq!(output[0], input[0]);
        assert_eq!(output[n / 2], input[n / 2]);
        assert_eq!(output[n - 1], input[n - 1]);
        // Full check.
        for i in 0..n {
            if output[i] != input[i] {
                panic!("mismatch at {}: out={} in={}", i, output[i], input[i]);
            }
        }
        println!("WAVE11 identity kernel @ 1M elements: all values match");
    }
}
