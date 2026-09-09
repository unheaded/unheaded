// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE12 Phase 4: precision-conversion kernel FFI.
//
// f32 → bf16 on-device so activations can stay GPU-resident across
// rmsnorm (f32 out) → matmul (bf16 in) without a CPU round-trip.

use crate::hip::GpuBuffer;

extern "C" {
    fn wave12_launch_f32_to_bf16_f32(
        out_bf16: *mut u16,
        in_f32: *const f32,
        n: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave12_launch_add_f32(
        out: *mut f32,
        a: *const f32,
        b: *const f32,
        n: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
}

/// On-device elementwise add: `out[i] = a[i] + b[i]`. All buffers f32.
/// Used for residual-stream adds between rmsnorm outputs and the running
/// hidden vector across the 35-layer forward chain.
pub fn add_f32(out: &GpuBuffer, a: &GpuBuffer, b: &GpuBuffer, n: usize) -> Result<(), String> {
    let required = n * 4;
    if out.len() < required || a.len() < required || b.len() < required {
        return Err(format!("add_f32: buf too small (need {} bytes)", required));
    }
    let err = unsafe {
        wave12_launch_add_f32(
            out.as_ptr() as *mut f32,
            a.as_ptr() as *const f32,
            b.as_ptr() as *const f32,
            n as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "add_f32")
}

/// On-device f32 → bf16 conversion. `out_bf16` must have ≥ n*2 bytes;
/// `in_f32` must have ≥ n*4 bytes.
pub fn f32_to_bf16(out_bf16: &GpuBuffer, in_f32: &GpuBuffer, n: usize) -> Result<(), String> {
    let required_out = n * 2;
    let required_in = n * 4;
    if out_bf16.len() < required_out {
        return Err(format!(
            "f32_to_bf16: out buf too small ({} < {})",
            out_bf16.len(),
            required_out
        ));
    }
    if in_f32.len() < required_in {
        return Err(format!(
            "f32_to_bf16: in buf too small ({} < {})",
            in_f32.len(),
            required_in
        ));
    }
    let err = unsafe {
        wave12_launch_f32_to_bf16_f32(
            out_bf16.as_ptr() as *mut u16,
            in_f32.as_ptr() as *const f32,
            n as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "f32_to_bf16")
}

#[cfg(test)]
mod tests {
    use super::*;
    use half::bf16;

    fn det_vec(n: usize, seed: u64) -> Vec<f32> {
        let mut s = seed.wrapping_add(0x9E3779B97F4A7C15);
        let mut v = Vec::with_capacity(n);
        for _ in 0..n {
            s ^= s >> 12;
            s ^= s << 25;
            s ^= s >> 27;
            let x = s.wrapping_mul(0x2545F4914F6CDD1D);
            let u = ((x >> 40) as f32) / (1u32 << 24) as f32 * 2.0 - 1.0;
            v.push(u * 10.0);
        }
        v
    }

    fn cosine(a: &[f32], b: &[f32]) -> f32 {
        let dot: f64 = a.iter().zip(b).map(|(x, y)| *x as f64 * *y as f64).sum();
        let na: f64 = a.iter().map(|x| (*x as f64).powi(2)).sum::<f64>().sqrt();
        let nb: f64 = b.iter().map(|x| (*x as f64).powi(2)).sum::<f64>().sqrt();
        (dot / (na * nb)) as f32
    }

    #[test]
    fn test_f32_to_bf16_matches_cpu() {
        let n = 10_000;
        let x = det_vec(n, 0xC0);

        // CPU reference: half::bf16::from_f32 round-nearest-even.
        let cpu_bf16: Vec<bf16> = x.iter().map(|f| bf16::from_f32(*f)).collect();
        let cpu_roundtrip: Vec<f32> = cpu_bf16.iter().map(|b| b.to_f32()).collect();

        // GPU path
        let in_buf = GpuBuffer::alloc(n * 4).expect("in alloc");
        let out_buf = GpuBuffer::alloc(n * 2).expect("out alloc");
        in_buf.upload_f32(&x).expect("in upload");

        f32_to_bf16(&out_buf, &in_buf, n).expect("f32_to_bf16");

        // Download the bf16 bits as u16, convert to f32 on host for compare.
        let mut out_u16 = vec![0u16; n];
        let out_bytes =
            unsafe { std::slice::from_raw_parts_mut(out_u16.as_mut_ptr() as *mut u8, n * 2) };
        out_buf.copy_to_host(out_bytes).expect("download");
        let gpu_roundtrip: Vec<f32> = out_u16
            .iter()
            .map(|b| bf16::from_bits(*b).to_f32())
            .collect();

        let cos = cosine(&cpu_roundtrip, &gpu_roundtrip);
        println!("f32_to_bf16 cosine vs CPU: {:.6}", cos);
        assert!(cos >= 0.9999, "cosine {} < 0.9999", cos);

        // Stricter: bit-identical on all values.
        let mut mismatches = 0usize;
        for (i, (a, b)) in cpu_roundtrip.iter().zip(&gpu_roundtrip).enumerate() {
            if a.to_bits() != b.to_bits() {
                mismatches += 1;
                if mismatches <= 5 {
                    eprintln!(
                        "  [{}] cpu=0x{:08x} ({}) gpu=0x{:08x} ({})",
                        i,
                        a.to_bits(),
                        a,
                        b.to_bits(),
                        b
                    );
                }
            }
        }
        assert_eq!(
            mismatches, 0,
            "{} of {} bf16 bit patterns disagree",
            mismatches, n
        );
    }
}
