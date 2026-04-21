// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 RoPE partial-rotary GPU kernels.

use crate::hip::GpuBuffer;

extern "C" {
    fn wave11_launch_rope_partial_fwd_f32(
        out: *mut f32, input: *const f32,
        cos_tab: *const f32, sin_tab: *const f32,
        seq: i32, n_head: i32, head_dim: i32, rope_dim: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_rope_partial_bwd_f32(
        grad_in: *mut f32, grad_out: *const f32,
        cos_tab: *const f32, sin_tab: *const f32,
        seq: i32, n_head: i32, head_dim: i32, rope_dim: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
}

pub fn rope_partial_fwd(
    out: &GpuBuffer, input: &GpuBuffer,
    cos_tab: &GpuBuffer, sin_tab: &GpuBuffer,
    seq: usize, n_head: usize, head_dim: usize, rope_dim: usize,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_rope_partial_fwd_f32(
            out.as_ptr() as *mut f32,
            input.as_ptr() as *const f32,
            cos_tab.as_ptr() as *const f32,
            sin_tab.as_ptr() as *const f32,
            seq as i32, n_head as i32, head_dim as i32, rope_dim as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "rope_partial_fwd")
}

pub fn rope_partial_bwd(
    grad_in: &GpuBuffer, grad_out: &GpuBuffer,
    cos_tab: &GpuBuffer, sin_tab: &GpuBuffer,
    seq: usize, n_head: usize, head_dim: usize, rope_dim: usize,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_rope_partial_bwd_f32(
            grad_in.as_ptr() as *mut f32,
            grad_out.as_ptr() as *const f32,
            cos_tab.as_ptr() as *const f32,
            sin_tab.as_ptr() as *const f32,
            seq as i32, n_head as i32, head_dim as i32, rope_dim as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "rope_partial_bwd")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::forward::rope_freqs;
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
            s ^= s >> 12; s ^= s << 25; s ^= s >> 27;
            let x = s.wrapping_mul(0x2545F4914F6CDD1D);
            let u = ((x >> 40) as f32) / (1u32 << 24) as f32 * 2.0 - 1.0;
            v.push(u * 0.1);
        }
        v
    }

    /// CPU reference matching gemma4::rope_apply_partial.
    fn cpu_rope_fwd(x: &[f32], cos: &[f32], sin: &[f32],
                    seq: usize, n_head: usize, head_dim: usize, rope_dim: usize) -> Vec<f32> {
        let mut out = x.to_vec();
        let half = rope_dim / 2;
        for s in 0..seq {
            for h in 0..n_head {
                let off = (s * n_head + h) * head_dim;
                for d in 0..half {
                    let c = cos[s * half + d];
                    let si = sin[s * half + d];
                    let xe = x[off + 2 * d];
                    let xo = x[off + 2 * d + 1];
                    out[off + 2 * d]     = xe * c - xo * si;
                    out[off + 2 * d + 1] = xe * si + xo * c;
                }
            }
        }
        out
    }

    /// CPU reference matching gemma4::rope_backward_partial.
    fn cpu_rope_bwd(grad_rot: &[f32], cos: &[f32], sin: &[f32],
                    seq: usize, n_head: usize, head_dim: usize, rope_dim: usize) -> Vec<f32> {
        let mut out = grad_rot.to_vec();
        let half = rope_dim / 2;
        for s in 0..seq {
            for h in 0..n_head {
                let off = (s * n_head + h) * head_dim;
                for d in 0..half {
                    let c = cos[s * half + d];
                    let si = sin[s * half + d];
                    let ge = grad_rot[off + 2 * d];
                    let go = grad_rot[off + 2 * d + 1];
                    out[off + 2 * d]     = ge * c + go * si;
                    out[off + 2 * d + 1] = -ge * si + go * c;
                }
            }
        }
        out
    }

    #[test]
    fn test_rope_partial_fwd_matches_cpu() {
        let seq = 4usize;
        let n_head = 8usize;
        let head_dim = 256usize;
        let rope_dim = 256usize;  // full-rotary for layer-0 test (no passthrough)
        let freq_base = 10000.0f32;

        let x = det_vec(seq * n_head * head_dim, 0xF1);
        let (cos, sin) = rope_freqs(seq, rope_dim, freq_base);

        let cpu = cpu_rope_fwd(&x, &cos, &sin, seq, n_head, head_dim, rope_dim);

        let x_buf = GpuBuffer::alloc(x.len() * 4).unwrap();
        let o_buf = GpuBuffer::alloc(x.len() * 4).unwrap();
        let c_buf = GpuBuffer::alloc(cos.len() * 4).unwrap();
        let s_buf = GpuBuffer::alloc(sin.len() * 4).unwrap();
        let xb = unsafe { std::slice::from_raw_parts(x.as_ptr() as *const u8, x.len()*4) };
        let cb = unsafe { std::slice::from_raw_parts(cos.as_ptr() as *const u8, cos.len()*4) };
        let sb = unsafe { std::slice::from_raw_parts(sin.as_ptr() as *const u8, sin.len()*4) };
        x_buf.copy_from_host(xb).unwrap();
        c_buf.copy_from_host(cb).unwrap();
        s_buf.copy_from_host(sb).unwrap();

        rope_partial_fwd(&o_buf, &x_buf, &c_buf, &s_buf,
            seq, n_head, head_dim, rope_dim).unwrap();
        let mut gpu = vec![0.0f32; x.len()];
        o_buf.download_f32(&mut gpu).unwrap();

        let co = cosine(&cpu, &gpu);
        println!("rope_partial_fwd cosine={:.6}", co);
        assert!(co >= 0.9999);
    }

    #[test]
    fn test_rope_partial_bwd_matches_cpu() {
        let seq = 4usize;
        let n_head = 8usize;
        let head_dim = 256usize;
        let rope_dim = 256usize;
        let freq_base = 10000.0f32;

        let gr = det_vec(seq * n_head * head_dim, 0xF2);
        let (cos, sin) = rope_freqs(seq, rope_dim, freq_base);

        let cpu = cpu_rope_bwd(&gr, &cos, &sin, seq, n_head, head_dim, rope_dim);

        let gr_buf = GpuBuffer::alloc(gr.len() * 4).unwrap();
        let gi_buf = GpuBuffer::alloc(gr.len() * 4).unwrap();
        let c_buf = GpuBuffer::alloc(cos.len() * 4).unwrap();
        let s_buf = GpuBuffer::alloc(sin.len() * 4).unwrap();
        let grb = unsafe { std::slice::from_raw_parts(gr.as_ptr() as *const u8, gr.len()*4) };
        let cb = unsafe { std::slice::from_raw_parts(cos.as_ptr() as *const u8, cos.len()*4) };
        let sb = unsafe { std::slice::from_raw_parts(sin.as_ptr() as *const u8, sin.len()*4) };
        gr_buf.copy_from_host(grb).unwrap();
        c_buf.copy_from_host(cb).unwrap();
        s_buf.copy_from_host(sb).unwrap();

        rope_partial_bwd(&gi_buf, &gr_buf, &c_buf, &s_buf,
            seq, n_head, head_dim, rope_dim).unwrap();
        let mut gpu = vec![0.0f32; gr.len()];
        gi_buf.download_f32(&mut gpu).unwrap();

        let co = cosine(&cpu, &gpu);
        println!("rope_partial_bwd cosine={:.6}", co);
        assert!(co >= 0.9999);
    }

    #[test]
    fn test_rope_partial_fwd_passthrough() {
        // Partial-rotary: head_dim=256, rope_dim=128 — only first 128
        // dims rotate; remaining 128 should be copied through unchanged.
        let seq = 4usize;
        let n_head = 4usize;
        let head_dim = 256usize;
        let rope_dim = 128usize;
        let freq_base = 10000.0f32;

        let x = det_vec(seq * n_head * head_dim, 0xF3);
        let (cos, sin) = rope_freqs(seq, rope_dim, freq_base);

        let x_buf = GpuBuffer::alloc(x.len() * 4).unwrap();
        let o_buf = GpuBuffer::alloc(x.len() * 4).unwrap();
        let c_buf = GpuBuffer::alloc(cos.len() * 4).unwrap();
        let s_buf = GpuBuffer::alloc(sin.len() * 4).unwrap();
        let xb = unsafe { std::slice::from_raw_parts(x.as_ptr() as *const u8, x.len()*4) };
        let cb = unsafe { std::slice::from_raw_parts(cos.as_ptr() as *const u8, cos.len()*4) };
        let sb = unsafe { std::slice::from_raw_parts(sin.as_ptr() as *const u8, sin.len()*4) };
        x_buf.copy_from_host(xb).unwrap();
        c_buf.copy_from_host(cb).unwrap();
        s_buf.copy_from_host(sb).unwrap();

        rope_partial_fwd(&o_buf, &x_buf, &c_buf, &s_buf,
            seq, n_head, head_dim, rope_dim).unwrap();
        let mut gpu = vec![0.0f32; x.len()];
        o_buf.download_f32(&mut gpu).unwrap();

        // Check passthrough dims [rope_dim..head_dim] are unchanged.
        for s in 0..seq {
            for h in 0..n_head {
                let off = (s * n_head + h) * head_dim;
                for d in rope_dim..head_dim {
                    assert_eq!(gpu[off + d], x[off + d],
                        "passthrough failed at s={} h={} d={}", s, h, d);
                }
            }
        }
        println!("rope_partial_fwd passthrough ({} → {}) clean", rope_dim, head_dim);
    }
}
