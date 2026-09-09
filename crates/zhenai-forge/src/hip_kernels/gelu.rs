// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 GELU (tanh-approx) GPU kernels — fwd, bwd, fused-with-multiply.

use crate::hip::GpuBuffer;

extern "C" {
    fn wave11_launch_gelu_fwd_f32(
        out: *mut f32,
        input: *const f32,
        n: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_gelu_bwd_f32(
        grad_in: *mut f32,
        grad_out: *const f32,
        input: *const f32,
        n: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_gelu_mul_fwd_f32(
        out: *mut f32,
        gate_pre: *const f32,
        up_pre: *const f32,
        n: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_gelu_mul_bwd_f32(
        grad_gate_pre: *mut f32,
        grad_up_pre: *mut f32,
        grad_out: *const f32,
        gate_pre: *const f32,
        up_pre: *const f32,
        n: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
}

pub fn gelu_fwd(out: &GpuBuffer, input: &GpuBuffer, n: usize) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_gelu_fwd_f32(
            out.as_ptr() as *mut f32,
            input.as_ptr() as *const f32,
            n as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "gelu_fwd")
}

pub fn gelu_bwd(
    grad_in: &GpuBuffer,
    grad_out: &GpuBuffer,
    input: &GpuBuffer,
    n: usize,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_gelu_bwd_f32(
            grad_in.as_ptr() as *mut f32,
            grad_out.as_ptr() as *const f32,
            input.as_ptr() as *const f32,
            n as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "gelu_bwd")
}

/// Fused gelu*up forward: out = gelu(gate_pre) * up_pre. One kernel, one
/// memory pass vs two. Used in FFN forward.
pub fn gelu_mul_fwd(
    out: &GpuBuffer,
    gate_pre: &GpuBuffer,
    up_pre: &GpuBuffer,
    n: usize,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_gelu_mul_fwd_f32(
            out.as_ptr() as *mut f32,
            gate_pre.as_ptr() as *const f32,
            up_pre.as_ptr() as *const f32,
            n as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "gelu_mul_fwd")
}

/// Fused gelu*up backward.
pub fn gelu_mul_bwd(
    grad_gate_pre: &GpuBuffer,
    grad_up_pre: &GpuBuffer,
    grad_out: &GpuBuffer,
    gate_pre: &GpuBuffer,
    up_pre: &GpuBuffer,
    n: usize,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_gelu_mul_bwd_f32(
            grad_gate_pre.as_ptr() as *mut f32,
            grad_up_pre.as_ptr() as *mut f32,
            grad_out.as_ptr() as *const f32,
            gate_pre.as_ptr() as *const f32,
            up_pre.as_ptr() as *const f32,
            n as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "gelu_mul_bwd")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::forward::gelu_tanh_approx;
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
            v.push(u);
        }
        v
    }

    fn gelu_prime(x: f32) -> f32 {
        const K: f32 = 0.797_884_6;
        const ALPHA: f32 = 0.044715;
        let inner = K * (x + ALPHA * x * x * x);
        let th = inner.tanh();
        let d_inner = K * (1.0 + 3.0 * ALPHA * x * x);
        0.5 * (1.0 + th) + 0.5 * x * (1.0 - th * th) * d_inner
    }

    #[test]
    fn test_gelu_fwd_matches_cpu() {
        let n = 24576usize; // realistic FFN shape (seq=4, n_ff=6144)
        let x = det_vec(n, 0xC1);
        let cpu: Vec<f32> = x.iter().map(|&v| gelu_tanh_approx(v)).collect();

        let x_buf = GpuBuffer::alloc(n * 4).unwrap();
        let o_buf = GpuBuffer::alloc(n * 4).unwrap();
        let xb = unsafe { std::slice::from_raw_parts(x.as_ptr() as *const u8, n * 4) };
        x_buf.copy_from_host(xb).unwrap();
        gelu_fwd(&o_buf, &x_buf, n).unwrap();
        let mut gpu = vec![0.0f32; n];
        o_buf.download_f32(&mut gpu).unwrap();

        let c = cosine(&cpu, &gpu);
        println!("gelu_fwd cosine={:.6}", c);
        assert!(c >= 0.9999);
    }

    #[test]
    fn test_gelu_bwd_matches_cpu() {
        let n = 24576usize;
        let x = det_vec(n, 0xC2);
        let go = det_vec(n, 0xC3);
        let cpu: Vec<f32> = x
            .iter()
            .zip(&go)
            .map(|(&xi, &goi)| goi * gelu_prime(xi))
            .collect();

        let x_buf = GpuBuffer::alloc(n * 4).unwrap();
        let go_buf = GpuBuffer::alloc(n * 4).unwrap();
        let gi_buf = GpuBuffer::alloc(n * 4).unwrap();
        let xb = unsafe { std::slice::from_raw_parts(x.as_ptr() as *const u8, n * 4) };
        let gob = unsafe { std::slice::from_raw_parts(go.as_ptr() as *const u8, n * 4) };
        x_buf.copy_from_host(xb).unwrap();
        go_buf.copy_from_host(gob).unwrap();
        gelu_bwd(&gi_buf, &go_buf, &x_buf, n).unwrap();
        let mut gpu = vec![0.0f32; n];
        gi_buf.download_f32(&mut gpu).unwrap();

        let c = cosine(&cpu, &gpu);
        println!("gelu_bwd cosine={:.6}", c);
        assert!(c >= 0.9999);
    }

    #[test]
    fn test_gelu_mul_fwd_matches_cpu() {
        let n = 24576usize;
        let gate = det_vec(n, 0xD1);
        let up = det_vec(n, 0xD2);
        let cpu: Vec<f32> = gate
            .iter()
            .zip(&up)
            .map(|(&g, &u)| gelu_tanh_approx(g) * u)
            .collect();

        let g_buf = GpuBuffer::alloc(n * 4).unwrap();
        let u_buf = GpuBuffer::alloc(n * 4).unwrap();
        let o_buf = GpuBuffer::alloc(n * 4).unwrap();
        let gb = unsafe { std::slice::from_raw_parts(gate.as_ptr() as *const u8, n * 4) };
        let ub = unsafe { std::slice::from_raw_parts(up.as_ptr() as *const u8, n * 4) };
        g_buf.copy_from_host(gb).unwrap();
        u_buf.copy_from_host(ub).unwrap();
        gelu_mul_fwd(&o_buf, &g_buf, &u_buf, n).unwrap();
        let mut gpu = vec![0.0f32; n];
        o_buf.download_f32(&mut gpu).unwrap();

        let c = cosine(&cpu, &gpu);
        println!("gelu_mul_fwd cosine={:.6}", c);
        assert!(c >= 0.9999);
    }

    #[test]
    fn test_gelu_mul_bwd_matches_cpu() {
        let n = 24576usize;
        let gate = det_vec(n, 0xE1);
        let up = det_vec(n, 0xE2);
        let go = det_vec(n, 0xE3);
        let cpu_gate: Vec<f32> = gate
            .iter()
            .zip(&up)
            .zip(&go)
            .map(|((&g, &u), &goi)| goi * u * gelu_prime(g))
            .collect();
        let cpu_up: Vec<f32> = gate
            .iter()
            .zip(&go)
            .map(|(&g, &goi)| goi * gelu_tanh_approx(g))
            .collect();

        let g_buf = GpuBuffer::alloc(n * 4).unwrap();
        let u_buf = GpuBuffer::alloc(n * 4).unwrap();
        let go_buf = GpuBuffer::alloc(n * 4).unwrap();
        let gg_buf = GpuBuffer::alloc(n * 4).unwrap();
        let gu_buf = GpuBuffer::alloc(n * 4).unwrap();
        let gb = unsafe { std::slice::from_raw_parts(gate.as_ptr() as *const u8, n * 4) };
        let ub = unsafe { std::slice::from_raw_parts(up.as_ptr() as *const u8, n * 4) };
        let gob = unsafe { std::slice::from_raw_parts(go.as_ptr() as *const u8, n * 4) };
        g_buf.copy_from_host(gb).unwrap();
        u_buf.copy_from_host(ub).unwrap();
        go_buf.copy_from_host(gob).unwrap();
        gelu_mul_bwd(&gg_buf, &gu_buf, &go_buf, &g_buf, &u_buf, n).unwrap();
        let mut gpu_gate = vec![0.0f32; n];
        let mut gpu_up = vec![0.0f32; n];
        gg_buf.download_f32(&mut gpu_gate).unwrap();
        gu_buf.download_f32(&mut gpu_up).unwrap();

        let c_gate = cosine(&cpu_gate, &gpu_gate);
        let c_up = cosine(&cpu_up, &gpu_up);
        println!("gelu_mul_bwd cosine gate={:.6} up={:.6}", c_gate, c_up);
        assert!(c_gate >= 0.9999);
        assert!(c_up >= 0.9999);
    }
}
