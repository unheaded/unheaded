// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 RMSNorm forward + backward GPU kernels. CPU reference:
//   forward.rs::rmsnorm
//   backward.rs::rmsnorm_backward

use crate::hip::GpuBuffer;

extern "C" {
    fn wave11_launch_rmsnorm_fwd_f32(
        out: *mut f32,
        input: *const f32,
        weight: *const f32,
        eps: f32,
        rows: i32,
        d: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_rmsnorm_bwd_f32(
        grad_in: *mut f32,
        grad_out: *const f32,
        input: *const f32,
        weight: *const f32,
        eps: f32,
        rows: i32,
        d: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
}

/// RMSNorm forward on GPU. `input`, `output` are `[rows, d]` f32 GpuBuffers;
/// `weight` is `[d]` f32. Returns Ok on success.
pub fn rmsnorm_fwd(
    output: &GpuBuffer,
    input: &GpuBuffer,
    weight: &GpuBuffer,
    eps: f32,
    rows: usize,
    d: usize,
) -> Result<(), String> {
    let required = rows * d * 4;
    if output.len() < required || input.len() < required {
        return Err(format!(
            "rmsnorm_fwd: buf too small (need {} bytes)",
            required
        ));
    }
    if weight.len() < d * 4 {
        return Err(format!(
            "rmsnorm_fwd: weight too small (need {} bytes)",
            d * 4
        ));
    }
    let err = unsafe {
        wave11_launch_rmsnorm_fwd_f32(
            output.as_ptr() as *mut f32,
            input.as_ptr() as *const f32,
            weight.as_ptr() as *const f32,
            eps,
            rows as i32,
            d as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "rmsnorm_fwd")
}

/// RMSNorm backward on GPU. `grad_in` and `grad_out` are `[rows, d]`.
pub fn rmsnorm_bwd(
    grad_in: &GpuBuffer,
    grad_out: &GpuBuffer,
    input: &GpuBuffer,
    weight: &GpuBuffer,
    eps: f32,
    rows: usize,
    d: usize,
) -> Result<(), String> {
    let required = rows * d * 4;
    if grad_in.len() < required || grad_out.len() < required || input.len() < required {
        return Err(format!(
            "rmsnorm_bwd: buf too small (need {} bytes)",
            required
        ));
    }
    if weight.len() < d * 4 {
        return Err(format!(
            "rmsnorm_bwd: weight too small (need {} bytes)",
            d * 4
        ));
    }
    let err = unsafe {
        wave11_launch_rmsnorm_bwd_f32(
            grad_in.as_ptr() as *mut f32,
            grad_out.as_ptr() as *const f32,
            input.as_ptr() as *const f32,
            weight.as_ptr() as *const f32,
            eps,
            rows as i32,
            d as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "rmsnorm_bwd")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::backward::rmsnorm_backward as cpu_rmsnorm_bwd;
    use crate::forward::rmsnorm as cpu_rmsnorm;
    use crate::hip::GpuBuffer;

    fn cosine(a: &[f32], b: &[f32]) -> f32 {
        let dot: f64 = a.iter().zip(b).map(|(x, y)| *x as f64 * *y as f64).sum();
        let na: f64 = a.iter().map(|x| (*x as f64).powi(2)).sum::<f64>().sqrt();
        let nb: f64 = b.iter().map(|x| (*x as f64).powi(2)).sum::<f64>().sqrt();
        (dot / (na * nb)) as f32
    }

    fn det_vec(n: usize, seed: u64) -> Vec<f32> {
        let mut s = seed.wrapping_add(0x9E3779B97F4A7C15);
        let mut v = Vec::with_capacity(n);
        for _ in 0..n {
            s ^= s >> 12;
            s ^= s << 25;
            s ^= s >> 27;
            let x = s.wrapping_mul(0x2545F4914F6CDD1D);
            let u = ((x >> 40) as f32) / (1u32 << 24) as f32 * 2.0 - 1.0;
            v.push(u * 0.1);
        }
        v
    }

    #[test]
    fn test_rmsnorm_fwd_matches_cpu() {
        let rows = 4usize;
        let d = 1536usize;
        let eps = 1e-6f32;
        let x = det_vec(rows * d, 0xA1);
        let w = det_vec(d, 0xA2);

        // CPU reference: apply per-row.
        let mut cpu_out = vec![0.0f32; rows * d];
        for r in 0..rows {
            let row = cpu_rmsnorm(&x[r * d..(r + 1) * d], &w, eps);
            cpu_out[r * d..(r + 1) * d].copy_from_slice(&row);
        }

        // GPU.
        let x_buf = GpuBuffer::alloc(rows * d * 4).unwrap();
        let w_buf = GpuBuffer::alloc(d * 4).unwrap();
        let o_buf = GpuBuffer::alloc(rows * d * 4).unwrap();
        let x_bytes = unsafe { std::slice::from_raw_parts(x.as_ptr() as *const u8, x.len() * 4) };
        let w_bytes = unsafe { std::slice::from_raw_parts(w.as_ptr() as *const u8, w.len() * 4) };
        x_buf.copy_from_host(x_bytes).unwrap();
        w_buf.copy_from_host(w_bytes).unwrap();

        rmsnorm_fwd(&o_buf, &x_buf, &w_buf, eps, rows, d).unwrap();

        let mut gpu_out = vec![0.0f32; rows * d];
        o_buf.download_f32(&mut gpu_out).unwrap();

        let c = cosine(&cpu_out, &gpu_out);
        let max_abs = cpu_out
            .iter()
            .zip(&gpu_out)
            .map(|(a, b)| (a - b).abs())
            .fold(0.0f32, f32::max);
        println!("rmsnorm_fwd cosine={:.6} max_abs_err={:.4e}", c, max_abs);
        assert!(c >= 0.9999, "cosine {} below 0.9999", c);
    }

    #[test]
    fn test_rmsnorm_bwd_matches_cpu() {
        let rows = 4usize;
        let d = 1536usize;
        let eps = 1e-6f32;
        let x = det_vec(rows * d, 0xB1);
        let w = det_vec(d, 0xB2);
        let go = det_vec(rows * d, 0xB3);

        let mut cpu_gi = vec![0.0f32; rows * d];
        for r in 0..rows {
            let row = cpu_rmsnorm_bwd(&x[r * d..(r + 1) * d], &w, &go[r * d..(r + 1) * d], eps);
            cpu_gi[r * d..(r + 1) * d].copy_from_slice(&row);
        }

        let x_buf = GpuBuffer::alloc(rows * d * 4).unwrap();
        let w_buf = GpuBuffer::alloc(d * 4).unwrap();
        let go_buf = GpuBuffer::alloc(rows * d * 4).unwrap();
        let gi_buf = GpuBuffer::alloc(rows * d * 4).unwrap();
        let x_bytes = unsafe { std::slice::from_raw_parts(x.as_ptr() as *const u8, x.len() * 4) };
        let w_bytes = unsafe { std::slice::from_raw_parts(w.as_ptr() as *const u8, w.len() * 4) };
        let go_bytes =
            unsafe { std::slice::from_raw_parts(go.as_ptr() as *const u8, go.len() * 4) };
        x_buf.copy_from_host(x_bytes).unwrap();
        w_buf.copy_from_host(w_bytes).unwrap();
        go_buf.copy_from_host(go_bytes).unwrap();

        rmsnorm_bwd(&gi_buf, &go_buf, &x_buf, &w_buf, eps, rows, d).unwrap();

        let mut gpu_gi = vec![0.0f32; rows * d];
        gi_buf.download_f32(&mut gpu_gi).unwrap();

        let c = cosine(&cpu_gi, &gpu_gi);
        let max_abs = cpu_gi
            .iter()
            .zip(&gpu_gi)
            .map(|(a, b)| (a - b).abs())
            .fold(0.0f32, f32::max);
        println!("rmsnorm_bwd cosine={:.6} max_abs_err={:.4e}", c, max_abs);
        assert!(c >= 0.9999, "cosine {} below 0.9999", c);
    }
}
