// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 Softmax GPU kernels — fwd, masked-fwd, bwd.

use crate::hip::GpuBuffer;

extern "C" {
    fn wave11_launch_softmax_fwd_f32(
        probs: *mut f32, scores: *const f32, rows: i32, cols: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_softmax_fwd_masked_f32(
        probs: *mut f32, scores: *const f32, mask: *const f32,
        rows: i32, cols: i32, stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_softmax_bwd_f32(
        grad_scores: *mut f32, grad_probs: *const f32, probs: *const f32,
        rows: i32, cols: i32, stream: *mut std::ffi::c_void,
    ) -> i32;
}

pub fn softmax_fwd(
    probs: &GpuBuffer, scores: &GpuBuffer, rows: usize, cols: usize,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_softmax_fwd_f32(
            probs.as_ptr() as *mut f32,
            scores.as_ptr() as *const f32,
            rows as i32, cols as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "softmax_fwd")
}

/// Masked softmax: `mask` is additive (0.0 = keep, very-negative = suppress).
/// Shape `[rows*cols]`.
pub fn softmax_fwd_masked(
    probs: &GpuBuffer, scores: &GpuBuffer, mask: &GpuBuffer,
    rows: usize, cols: usize,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_softmax_fwd_masked_f32(
            probs.as_ptr() as *mut f32,
            scores.as_ptr() as *const f32,
            mask.as_ptr() as *const f32,
            rows as i32, cols as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "softmax_fwd_masked")
}

pub fn softmax_bwd(
    grad_scores: &GpuBuffer, grad_probs: &GpuBuffer, probs: &GpuBuffer,
    rows: usize, cols: usize,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_softmax_bwd_f32(
            grad_scores.as_ptr() as *mut f32,
            grad_probs.as_ptr() as *const f32,
            probs.as_ptr() as *const f32,
            rows as i32, cols as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "softmax_bwd")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::hip::GpuBuffer;

    fn cosine(a: &[f32], b: &[f32]) -> f32 {
        let dot: f64 = a.iter().zip(b).map(|(x, y)| *x as f64 * *y as f64).sum();
        let na: f64 = a.iter().map(|x| (*x as f64).powi(2)).sum::<f64>().sqrt();
        let nb: f64 = b.iter().map(|x| (*x as f64).powi(2)).sum::<f64>().sqrt();
        (dot / (na * nb)) as f32
    }
    fn det_vec(n: usize, seed: u64, scale: f32) -> Vec<f32> {
        let mut s = seed.wrapping_add(0x9E3779B97F4A7C15);
        let mut v = Vec::with_capacity(n);
        for _ in 0..n {
            s ^= s >> 12; s ^= s << 25; s ^= s >> 27;
            let x = s.wrapping_mul(0x2545F4914F6CDD1D);
            let u = ((x >> 40) as f32) / (1u32 << 24) as f32 * 2.0 - 1.0;
            v.push(u * scale);
        }
        v
    }

    fn cpu_softmax(scores: &[f32], rows: usize, cols: usize) -> Vec<f32> {
        let mut out = vec![0.0f32; rows * cols];
        for r in 0..rows {
            let row = &scores[r * cols..(r + 1) * cols];
            let max = row.iter().cloned().fold(f32::NEG_INFINITY, f32::max);
            let mut s = 0.0f32;
            for (o, &v) in out[r * cols..(r + 1) * cols].iter_mut().zip(row) {
                *o = (v - max).exp();
                s += *o;
            }
            for o in out[r * cols..(r + 1) * cols].iter_mut() { *o /= s; }
        }
        out
    }

    #[test]
    fn test_softmax_fwd_matches_cpu() {
        let rows = 8usize; // attention rows (heads × q_positions mini)
        let cols = 384usize; // attention cols at seq=384
        let scores = det_vec(rows * cols, 0x501, 4.0); // wide range for stability test

        let cpu = cpu_softmax(&scores, rows, cols);

        let s_buf = GpuBuffer::alloc(rows * cols * 4).unwrap();
        let p_buf = GpuBuffer::alloc(rows * cols * 4).unwrap();
        let sb = unsafe { std::slice::from_raw_parts(scores.as_ptr() as *const u8, scores.len() * 4) };
        s_buf.copy_from_host(sb).unwrap();
        softmax_fwd(&p_buf, &s_buf, rows, cols).unwrap();
        let mut gpu = vec![0.0f32; rows * cols];
        p_buf.download_f32(&mut gpu).unwrap();

        let c = cosine(&cpu, &gpu);
        println!("softmax_fwd cosine={:.6}", c);
        assert!(c >= 0.9999);
        // Row sums should be 1.0 within noise.
        for r in 0..rows {
            let s: f32 = gpu[r * cols..(r + 1) * cols].iter().sum();
            assert!((s - 1.0).abs() < 1e-4, "row {} sum={}", r, s);
        }
    }

    #[test]
    fn test_softmax_fwd_masked_matches_cpu() {
        let rows = 4usize;
        let cols = 384usize;
        let scores = det_vec(rows * cols, 0x502, 3.0);
        // Causal mask: row r, col c → 0 if c <= r, else -1e30.
        let mut mask = vec![0.0f32; rows * cols];
        for r in 0..rows {
            for c in 0..cols {
                if c > r { mask[r * cols + c] = -1e30; }
            }
        }
        // CPU: mask-add then softmax.
        let mut masked_scores = scores.clone();
        for i in 0..masked_scores.len() { masked_scores[i] += mask[i]; }
        let cpu = cpu_softmax(&masked_scores, rows, cols);

        let s_buf = GpuBuffer::alloc(rows * cols * 4).unwrap();
        let m_buf = GpuBuffer::alloc(rows * cols * 4).unwrap();
        let p_buf = GpuBuffer::alloc(rows * cols * 4).unwrap();
        let sb = unsafe { std::slice::from_raw_parts(scores.as_ptr() as *const u8, scores.len() * 4) };
        let mb = unsafe { std::slice::from_raw_parts(mask.as_ptr() as *const u8, mask.len() * 4) };
        s_buf.copy_from_host(sb).unwrap();
        m_buf.copy_from_host(mb).unwrap();
        softmax_fwd_masked(&p_buf, &s_buf, &m_buf, rows, cols).unwrap();
        let mut gpu = vec![0.0f32; rows * cols];
        p_buf.download_f32(&mut gpu).unwrap();

        let c = cosine(&cpu, &gpu);
        println!("softmax_fwd_masked cosine={:.6}", c);
        assert!(c >= 0.9999);
        // Masked positions should be exactly 0 (within f32 epsilon).
        for r in 0..rows {
            for col in (r + 1)..cols {
                assert!(gpu[r * cols + col].abs() < 1e-6,
                    "row {} col {} should be masked but got {}", r, col, gpu[r * cols + col]);
            }
        }
    }

    #[test]
    fn test_softmax_bwd_matches_cpu() {
        let rows = 8usize;
        let cols = 384usize;
        let scores = det_vec(rows * cols, 0x503, 3.0);
        let grad_probs = det_vec(rows * cols, 0x504, 0.5);

        let probs = cpu_softmax(&scores, rows, cols);
        // CPU grad_scores.
        let mut cpu_gs = vec![0.0f32; rows * cols];
        for r in 0..rows {
            let p_row = &probs[r * cols..(r + 1) * cols];
            let gp_row = &grad_probs[r * cols..(r + 1) * cols];
            let s: f32 = p_row.iter().zip(gp_row).map(|(p, g)| p * g).sum();
            for c in 0..cols {
                cpu_gs[r * cols + c] = p_row[c] * (gp_row[c] - s);
            }
        }

        let p_buf = GpuBuffer::alloc(rows * cols * 4).unwrap();
        let gp_buf = GpuBuffer::alloc(rows * cols * 4).unwrap();
        let gs_buf = GpuBuffer::alloc(rows * cols * 4).unwrap();
        let pb = unsafe { std::slice::from_raw_parts(probs.as_ptr() as *const u8, probs.len() * 4) };
        let gpb = unsafe { std::slice::from_raw_parts(grad_probs.as_ptr() as *const u8, grad_probs.len() * 4) };
        p_buf.copy_from_host(pb).unwrap();
        gp_buf.copy_from_host(gpb).unwrap();
        softmax_bwd(&gs_buf, &gp_buf, &p_buf, rows, cols).unwrap();
        let mut gpu_gs = vec![0.0f32; rows * cols];
        gs_buf.download_f32(&mut gpu_gs).unwrap();

        let c = cosine(&cpu_gs, &gpu_gs);
        println!("softmax_bwd cosine={:.6}", c);
        assert!(c >= 0.9999);
    }
}
