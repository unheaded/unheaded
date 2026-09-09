// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 Attention GPU kernels — unfused composition.
//
// Three-stage forward:
//   1. attn_scores_fwd   → pre-softmax scores  [n_heads, seq, seq]
//   2. softmax_fwd_masked (Phase 5, reused)    → post-softmax probs
//   3. attn_output_fwd   → [seq, n_heads*head_dim]
//
// Backward lands in a follow-up (once forward correctness is proven).

use crate::hip::GpuBuffer;

extern "C" {
    fn wave11_launch_attn_scores_fwd_f32(
        scores: *mut f32,
        q: *const f32,
        k: *const f32,
        n_heads: i32,
        n_kv_heads: i32,
        seq: i32,
        head_dim: i32,
        scale: f32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_attn_output_fwd_f32(
        out: *mut f32,
        probs: *const f32,
        v: *const f32,
        n_heads: i32,
        n_kv_heads: i32,
        seq: i32,
        head_dim: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_attn_grad_v_f32(
        grad_v: *mut f32,
        probs: *const f32,
        grad_out: *const f32,
        n_heads: i32,
        n_kv_heads: i32,
        seq: i32,
        head_dim: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_attn_grad_probs_f32(
        grad_probs: *mut f32,
        grad_out: *const f32,
        v: *const f32,
        n_heads: i32,
        n_kv_heads: i32,
        seq: i32,
        head_dim: i32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_attn_grad_q_f32(
        grad_q: *mut f32,
        grad_scores: *const f32,
        k: *const f32,
        n_heads: i32,
        n_kv_heads: i32,
        seq: i32,
        head_dim: i32,
        scale: f32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
    fn wave11_launch_attn_grad_k_f32(
        grad_k: *mut f32,
        grad_scores: *const f32,
        q: *const f32,
        n_heads: i32,
        n_kv_heads: i32,
        seq: i32,
        head_dim: i32,
        scale: f32,
        stream: *mut std::ffi::c_void,
    ) -> i32;
}

/// Compute pre-softmax attention scores with GQA broadcast.
/// Inputs: `q [seq, n_heads, head_dim]`, `k [seq, n_kv_heads, head_dim]`.
/// Output: `scores [n_heads, seq, seq]`. Scale is typically `1/sqrt(head_dim)`.
#[allow(clippy::too_many_arguments)] // numerical/GPU kernel signature — see crate note
pub fn attn_scores_fwd(
    scores: &GpuBuffer,
    q: &GpuBuffer,
    k: &GpuBuffer,
    n_heads: usize,
    n_kv_heads: usize,
    seq: usize,
    head_dim: usize,
    scale: f32,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_attn_scores_fwd_f32(
            scores.as_ptr() as *mut f32,
            q.as_ptr() as *const f32,
            k.as_ptr() as *const f32,
            n_heads as i32,
            n_kv_heads as i32,
            seq as i32,
            head_dim as i32,
            scale,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "attn_scores_fwd")
}

/// Combine softmax probs with V: `out[sq][h][d] = sum_k probs[h][sq][k] * V[k, h_kv, d]`.
/// Inputs: `probs [n_heads, seq, seq]`, `v [seq, n_kv_heads, head_dim]`.
/// Output: `out [seq, n_heads, head_dim]`.
pub fn attn_output_fwd(
    out: &GpuBuffer,
    probs: &GpuBuffer,
    v: &GpuBuffer,
    n_heads: usize,
    n_kv_heads: usize,
    seq: usize,
    head_dim: usize,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_attn_output_fwd_f32(
            out.as_ptr() as *mut f32,
            probs.as_ptr() as *const f32,
            v.as_ptr() as *const f32,
            n_heads as i32,
            n_kv_heads as i32,
            seq as i32,
            head_dim as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "attn_output_fwd")
}

/// grad_V[sk, h_kv, d] = sum_{h in group(h_kv), sq} probs[h][sq][sk] * grad_out[sq][h][d].
/// GQA collapse is built in — kernel writes grad_v directly in KV-head layout.
pub fn attn_grad_v(
    grad_v: &GpuBuffer,
    probs: &GpuBuffer,
    grad_out: &GpuBuffer,
    n_heads: usize,
    n_kv_heads: usize,
    seq: usize,
    head_dim: usize,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_attn_grad_v_f32(
            grad_v.as_ptr() as *mut f32,
            probs.as_ptr() as *const f32,
            grad_out.as_ptr() as *const f32,
            n_heads as i32,
            n_kv_heads as i32,
            seq as i32,
            head_dim as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "attn_grad_v")
}

/// grad_probs[h][sq][sk] = sum_d grad_out[sq][h][d] * V[sk, h_kv, d].
pub fn attn_grad_probs(
    grad_probs: &GpuBuffer,
    grad_out: &GpuBuffer,
    v: &GpuBuffer,
    n_heads: usize,
    n_kv_heads: usize,
    seq: usize,
    head_dim: usize,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_attn_grad_probs_f32(
            grad_probs.as_ptr() as *mut f32,
            grad_out.as_ptr() as *const f32,
            v.as_ptr() as *const f32,
            n_heads as i32,
            n_kv_heads as i32,
            seq as i32,
            head_dim as i32,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "attn_grad_probs")
}

/// grad_Q[sq, h, d] = scale * sum_sk grad_scores[h][sq][sk] * K[sk, h_kv, d].
#[allow(clippy::too_many_arguments)] // numerical/GPU kernel signature — see crate note
pub fn attn_grad_q(
    grad_q: &GpuBuffer,
    grad_scores: &GpuBuffer,
    k: &GpuBuffer,
    n_heads: usize,
    n_kv_heads: usize,
    seq: usize,
    head_dim: usize,
    scale: f32,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_attn_grad_q_f32(
            grad_q.as_ptr() as *mut f32,
            grad_scores.as_ptr() as *const f32,
            k.as_ptr() as *const f32,
            n_heads as i32,
            n_kv_heads as i32,
            seq as i32,
            head_dim as i32,
            scale,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "attn_grad_q")
}

/// grad_K[sk, h_kv, d] = scale * sum_{h in group, sq} grad_scores[h][sq][sk] * Q[sq, h, d].
/// GQA collapse built in.
#[allow(clippy::too_many_arguments)] // numerical/GPU kernel signature — see crate note
pub fn attn_grad_k(
    grad_k: &GpuBuffer,
    grad_scores: &GpuBuffer,
    q: &GpuBuffer,
    n_heads: usize,
    n_kv_heads: usize,
    seq: usize,
    head_dim: usize,
    scale: f32,
) -> Result<(), String> {
    let err = unsafe {
        wave11_launch_attn_grad_k_f32(
            grad_k.as_ptr() as *mut f32,
            grad_scores.as_ptr() as *const f32,
            q.as_ptr() as *const f32,
            n_heads as i32,
            n_kv_heads as i32,
            seq as i32,
            head_dim as i32,
            scale,
            std::ptr::null_mut(),
        )
    };
    super::check_hip(err, "attn_grad_k")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::forward::{attention_forward, AttnMask};
    use crate::hip::GpuBuffer;
    use crate::hip_kernels::softmax;

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
    fn test_attn_scores_fwd_matches_cpu() {
        // Layer-0 shape: seq=8, n_heads=8, n_kv_heads=2, head_dim=256.
        let seq = 8usize;
        let n_heads = 8usize;
        let n_kv_heads = 2usize;
        let head_dim = 256usize;
        let scale = 1.0 / (head_dim as f32).sqrt();

        let q = det_vec(seq * n_heads * head_dim, 0xA71);
        let k = det_vec(seq * n_kv_heads * head_dim, 0xA72);

        // CPU reference: compute scores matching attention_forward's internal
        // per-head, per-row pattern.
        let group_size = n_heads / n_kv_heads;
        let mut cpu_scores = vec![0.0f32; n_heads * seq * seq];
        for h in 0..n_heads {
            let h_kv = h / group_size;
            for sq in 0..seq {
                for sk in 0..seq {
                    let mut dot = 0.0f32;
                    for d in 0..head_dim {
                        let q_i = (sq * n_heads + h) * head_dim + d;
                        let k_i = (sk * n_kv_heads + h_kv) * head_dim + d;
                        dot += q[q_i] * k[k_i];
                    }
                    cpu_scores[(h * seq + sq) * seq + sk] = dot * scale;
                }
            }
        }

        // GPU.
        let q_buf = GpuBuffer::alloc(q.len() * 4).unwrap();
        let k_buf = GpuBuffer::alloc(k.len() * 4).unwrap();
        let s_buf = GpuBuffer::alloc(cpu_scores.len() * 4).unwrap();
        let qb = unsafe { std::slice::from_raw_parts(q.as_ptr() as *const u8, q.len() * 4) };
        let kb = unsafe { std::slice::from_raw_parts(k.as_ptr() as *const u8, k.len() * 4) };
        q_buf.copy_from_host(qb).unwrap();
        k_buf.copy_from_host(kb).unwrap();
        attn_scores_fwd(
            &s_buf, &q_buf, &k_buf, n_heads, n_kv_heads, seq, head_dim, scale,
        )
        .unwrap();
        let mut gpu_scores = vec![0.0f32; cpu_scores.len()];
        s_buf.download_f32(&mut gpu_scores).unwrap();

        let c = cosine(&cpu_scores, &gpu_scores);
        let max_abs = cpu_scores
            .iter()
            .zip(&gpu_scores)
            .map(|(a, b)| (a - b).abs())
            .fold(0.0f32, f32::max);
        println!(
            "attn_scores_fwd cosine={:.6} max_abs_err={:.4e}",
            c, max_abs
        );
        assert!(c >= 0.9999, "cosine {} below 0.9999", c);
    }

    #[test]
    fn test_attn_output_fwd_matches_cpu() {
        // Same shape. Use synthetic probs that already satisfy per-row=1.
        let seq = 8usize;
        let n_heads = 8usize;
        let n_kv_heads = 2usize;
        let head_dim = 256usize;

        // Build random probs then normalize each row to sum=1.
        let mut probs = det_vec(n_heads * seq * seq, 0xA73);
        for p in probs.iter_mut() {
            *p = p.abs() + 0.01;
        }
        for h in 0..n_heads {
            for sq in 0..seq {
                let start = (h * seq + sq) * seq;
                let s: f32 = probs[start..start + seq].iter().sum();
                for p in probs[start..start + seq].iter_mut() {
                    *p /= s;
                }
            }
        }
        let v = det_vec(seq * n_kv_heads * head_dim, 0xA74);

        // CPU reference: for each (h, sq, d), sum_k probs[h][sq][k] * V[k, h_kv, d].
        let group_size = n_heads / n_kv_heads;
        let mut cpu_out = vec![0.0f32; seq * n_heads * head_dim];
        for h in 0..n_heads {
            let h_kv = h / group_size;
            for sq in 0..seq {
                for d in 0..head_dim {
                    let mut sum = 0.0f32;
                    for k in 0..seq {
                        sum += probs[(h * seq + sq) * seq + k]
                            * v[(k * n_kv_heads + h_kv) * head_dim + d];
                    }
                    cpu_out[(sq * n_heads + h) * head_dim + d] = sum;
                }
            }
        }

        let p_buf = GpuBuffer::alloc(probs.len() * 4).unwrap();
        let v_buf = GpuBuffer::alloc(v.len() * 4).unwrap();
        let o_buf = GpuBuffer::alloc(cpu_out.len() * 4).unwrap();
        let pb =
            unsafe { std::slice::from_raw_parts(probs.as_ptr() as *const u8, probs.len() * 4) };
        let vb = unsafe { std::slice::from_raw_parts(v.as_ptr() as *const u8, v.len() * 4) };
        p_buf.copy_from_host(pb).unwrap();
        v_buf.copy_from_host(vb).unwrap();

        attn_output_fwd(&o_buf, &p_buf, &v_buf, n_heads, n_kv_heads, seq, head_dim).unwrap();
        let mut gpu_out = vec![0.0f32; cpu_out.len()];
        o_buf.download_f32(&mut gpu_out).unwrap();

        let c = cosine(&cpu_out, &gpu_out);
        let max_abs = cpu_out
            .iter()
            .zip(&gpu_out)
            .map(|(a, b)| (a - b).abs())
            .fold(0.0f32, f32::max);
        println!(
            "attn_output_fwd cosine={:.6} max_abs_err={:.4e}",
            c, max_abs
        );
        assert!(c >= 0.9999);
    }

    // CPU references for the four backward stages, matching
    // kernels/attn.hip.cpp documentation.
    fn cpu_grad_v(
        probs: &[f32],
        grad_out: &[f32],
        n_heads: usize,
        n_kv_heads: usize,
        seq: usize,
        head_dim: usize,
    ) -> Vec<f32> {
        let group_size = n_heads / n_kv_heads;
        let mut out = vec![0.0f32; seq * n_kv_heads * head_dim];
        for sk in 0..seq {
            for h_kv in 0..n_kv_heads {
                for d in 0..head_dim {
                    let mut sum = 0.0f32;
                    for gh in 0..group_size {
                        let h = h_kv * group_size + gh;
                        for sq in 0..seq {
                            let p = probs[(h * seq + sq) * seq + sk];
                            let go = grad_out[(sq * n_heads + h) * head_dim + d];
                            sum += p * go;
                        }
                    }
                    out[(sk * n_kv_heads + h_kv) * head_dim + d] = sum;
                }
            }
        }
        out
    }
    fn cpu_grad_probs(
        grad_out: &[f32],
        v: &[f32],
        n_heads: usize,
        n_kv_heads: usize,
        seq: usize,
        head_dim: usize,
    ) -> Vec<f32> {
        let group_size = n_heads / n_kv_heads;
        let mut out = vec![0.0f32; n_heads * seq * seq];
        for h in 0..n_heads {
            let h_kv = h / group_size;
            for sq in 0..seq {
                for sk in 0..seq {
                    let mut sum = 0.0f32;
                    for d in 0..head_dim {
                        let go = grad_out[(sq * n_heads + h) * head_dim + d];
                        let vv = v[(sk * n_kv_heads + h_kv) * head_dim + d];
                        sum += go * vv;
                    }
                    out[(h * seq + sq) * seq + sk] = sum;
                }
            }
        }
        out
    }
    fn cpu_grad_q(
        grad_scores: &[f32],
        k: &[f32],
        n_heads: usize,
        n_kv_heads: usize,
        seq: usize,
        head_dim: usize,
        scale: f32,
    ) -> Vec<f32> {
        let group_size = n_heads / n_kv_heads;
        let mut out = vec![0.0f32; seq * n_heads * head_dim];
        for sq in 0..seq {
            for h in 0..n_heads {
                let h_kv = h / group_size;
                for d in 0..head_dim {
                    let mut sum = 0.0f32;
                    for sk in 0..seq {
                        let gs = grad_scores[(h * seq + sq) * seq + sk];
                        let kv = k[(sk * n_kv_heads + h_kv) * head_dim + d];
                        sum += gs * kv;
                    }
                    out[(sq * n_heads + h) * head_dim + d] = sum * scale;
                }
            }
        }
        out
    }
    fn cpu_grad_k(
        grad_scores: &[f32],
        q: &[f32],
        n_heads: usize,
        n_kv_heads: usize,
        seq: usize,
        head_dim: usize,
        scale: f32,
    ) -> Vec<f32> {
        let group_size = n_heads / n_kv_heads;
        let mut out = vec![0.0f32; seq * n_kv_heads * head_dim];
        for sk in 0..seq {
            for h_kv in 0..n_kv_heads {
                for d in 0..head_dim {
                    let mut sum = 0.0f32;
                    for gh in 0..group_size {
                        let h = h_kv * group_size + gh;
                        for sq in 0..seq {
                            let gs = grad_scores[(h * seq + sq) * seq + sk];
                            let qv = q[(sq * n_heads + h) * head_dim + d];
                            sum += gs * qv;
                        }
                    }
                    out[(sk * n_kv_heads + h_kv) * head_dim + d] = sum * scale;
                }
            }
        }
        out
    }

    #[test]
    fn test_attn_grad_v_matches_cpu() {
        let seq = 8usize;
        let n_heads = 8usize;
        let n_kv_heads = 2usize;
        let head_dim = 256usize;
        let probs = {
            let mut v = det_vec(n_heads * seq * seq, 0xC71);
            for p in v.iter_mut() {
                *p = p.abs() + 0.01;
            }
            for h in 0..n_heads {
                for sq in 0..seq {
                    let start = (h * seq + sq) * seq;
                    let s: f32 = v[start..start + seq].iter().sum();
                    for p in v[start..start + seq].iter_mut() {
                        *p /= s;
                    }
                }
            }
            v
        };
        let grad_out = det_vec(seq * n_heads * head_dim, 0xC72);
        let cpu = cpu_grad_v(&probs, &grad_out, n_heads, n_kv_heads, seq, head_dim);

        let p_buf = GpuBuffer::alloc(probs.len() * 4).unwrap();
        let go_buf = GpuBuffer::alloc(grad_out.len() * 4).unwrap();
        let gv_buf = GpuBuffer::alloc(cpu.len() * 4).unwrap();
        let pb =
            unsafe { std::slice::from_raw_parts(probs.as_ptr() as *const u8, probs.len() * 4) };
        let gob = unsafe {
            std::slice::from_raw_parts(grad_out.as_ptr() as *const u8, grad_out.len() * 4)
        };
        p_buf.copy_from_host(pb).unwrap();
        go_buf.copy_from_host(gob).unwrap();
        attn_grad_v(&gv_buf, &p_buf, &go_buf, n_heads, n_kv_heads, seq, head_dim).unwrap();
        let mut gpu = vec![0.0f32; cpu.len()];
        gv_buf.download_f32(&mut gpu).unwrap();

        let c = cosine(&cpu, &gpu);
        println!("attn_grad_v cosine={:.6}", c);
        assert!(c >= 0.9999);
    }

    #[test]
    fn test_attn_grad_probs_matches_cpu() {
        let seq = 8usize;
        let n_heads = 8usize;
        let n_kv_heads = 2usize;
        let head_dim = 256usize;
        let grad_out = det_vec(seq * n_heads * head_dim, 0xC81);
        let v = det_vec(seq * n_kv_heads * head_dim, 0xC82);
        let cpu = cpu_grad_probs(&grad_out, &v, n_heads, n_kv_heads, seq, head_dim);

        let go_buf = GpuBuffer::alloc(grad_out.len() * 4).unwrap();
        let v_buf = GpuBuffer::alloc(v.len() * 4).unwrap();
        let gp_buf = GpuBuffer::alloc(cpu.len() * 4).unwrap();
        let gob = unsafe {
            std::slice::from_raw_parts(grad_out.as_ptr() as *const u8, grad_out.len() * 4)
        };
        let vb = unsafe { std::slice::from_raw_parts(v.as_ptr() as *const u8, v.len() * 4) };
        go_buf.copy_from_host(gob).unwrap();
        v_buf.copy_from_host(vb).unwrap();
        attn_grad_probs(&gp_buf, &go_buf, &v_buf, n_heads, n_kv_heads, seq, head_dim).unwrap();
        let mut gpu = vec![0.0f32; cpu.len()];
        gp_buf.download_f32(&mut gpu).unwrap();

        let c = cosine(&cpu, &gpu);
        println!("attn_grad_probs cosine={:.6}", c);
        assert!(c >= 0.9999);
    }

    #[test]
    fn test_attn_grad_q_matches_cpu() {
        let seq = 8usize;
        let n_heads = 8usize;
        let n_kv_heads = 2usize;
        let head_dim = 256usize;
        let scale = 1.0 / (head_dim as f32).sqrt();
        let grad_scores = det_vec(n_heads * seq * seq, 0xC91);
        let k = det_vec(seq * n_kv_heads * head_dim, 0xC92);
        let cpu = cpu_grad_q(&grad_scores, &k, n_heads, n_kv_heads, seq, head_dim, scale);

        let gs_buf = GpuBuffer::alloc(grad_scores.len() * 4).unwrap();
        let k_buf = GpuBuffer::alloc(k.len() * 4).unwrap();
        let gq_buf = GpuBuffer::alloc(cpu.len() * 4).unwrap();
        let gsb = unsafe {
            std::slice::from_raw_parts(grad_scores.as_ptr() as *const u8, grad_scores.len() * 4)
        };
        let kb = unsafe { std::slice::from_raw_parts(k.as_ptr() as *const u8, k.len() * 4) };
        gs_buf.copy_from_host(gsb).unwrap();
        k_buf.copy_from_host(kb).unwrap();
        attn_grad_q(
            &gq_buf, &gs_buf, &k_buf, n_heads, n_kv_heads, seq, head_dim, scale,
        )
        .unwrap();
        let mut gpu = vec![0.0f32; cpu.len()];
        gq_buf.download_f32(&mut gpu).unwrap();

        let c = cosine(&cpu, &gpu);
        println!("attn_grad_q cosine={:.6}", c);
        assert!(c >= 0.9999);
    }

    #[test]
    fn test_attn_grad_k_matches_cpu() {
        let seq = 8usize;
        let n_heads = 8usize;
        let n_kv_heads = 2usize;
        let head_dim = 256usize;
        let scale = 1.0 / (head_dim as f32).sqrt();
        let grad_scores = det_vec(n_heads * seq * seq, 0xCA1);
        let q = det_vec(seq * n_heads * head_dim, 0xCA2);
        let cpu = cpu_grad_k(&grad_scores, &q, n_heads, n_kv_heads, seq, head_dim, scale);

        let gs_buf = GpuBuffer::alloc(grad_scores.len() * 4).unwrap();
        let q_buf = GpuBuffer::alloc(q.len() * 4).unwrap();
        let gk_buf = GpuBuffer::alloc(cpu.len() * 4).unwrap();
        let gsb = unsafe {
            std::slice::from_raw_parts(grad_scores.as_ptr() as *const u8, grad_scores.len() * 4)
        };
        let qb = unsafe { std::slice::from_raw_parts(q.as_ptr() as *const u8, q.len() * 4) };
        gs_buf.copy_from_host(gsb).unwrap();
        q_buf.copy_from_host(qb).unwrap();
        attn_grad_k(
            &gk_buf, &gs_buf, &q_buf, n_heads, n_kv_heads, seq, head_dim, scale,
        )
        .unwrap();
        let mut gpu = vec![0.0f32; cpu.len()];
        gk_buf.download_f32(&mut gpu).unwrap();

        let c = cosine(&cpu, &gpu);
        println!("attn_grad_k cosine={:.6}", c);
        assert!(c >= 0.9999);
    }

    /// End-to-end attention forward: scores → masked softmax → output.
    /// Compared to `forward::attention_forward` on causal mask.
    #[test]
    fn test_attn_e2e_fwd_vs_cpu() {
        let seq = 8usize;
        let n_heads = 8usize;
        let n_kv_heads = 2usize;
        let head_dim = 256usize;
        let scale = 1.0 / (head_dim as f32).sqrt();

        let q = det_vec(seq * n_heads * head_dim, 0xB71);
        let k = det_vec(seq * n_kv_heads * head_dim, 0xB72);
        let v = det_vec(seq * n_kv_heads * head_dim, 0xB73);

        // CPU reference: attention_forward with causal mask.
        let (cpu_out, _cpu_cache) = attention_forward(
            &q,
            &k,
            &v,
            n_heads,
            n_kv_heads,
            head_dim,
            seq,
            AttnMask::Causal,
        );

        // GPU three-stage.
        // Buffers.
        let q_buf = GpuBuffer::alloc(q.len() * 4).unwrap();
        let k_buf = GpuBuffer::alloc(k.len() * 4).unwrap();
        let v_buf = GpuBuffer::alloc(v.len() * 4).unwrap();
        let s_buf = GpuBuffer::alloc(n_heads * seq * seq * 4).unwrap();
        let p_buf = GpuBuffer::alloc(n_heads * seq * seq * 4).unwrap();
        let m_buf = GpuBuffer::alloc(n_heads * seq * seq * 4).unwrap();
        let o_buf = GpuBuffer::alloc(cpu_out.len() * 4).unwrap();
        let qb = unsafe { std::slice::from_raw_parts(q.as_ptr() as *const u8, q.len() * 4) };
        let kb = unsafe { std::slice::from_raw_parts(k.as_ptr() as *const u8, k.len() * 4) };
        let vb = unsafe { std::slice::from_raw_parts(v.as_ptr() as *const u8, v.len() * 4) };
        q_buf.copy_from_host(qb).unwrap();
        k_buf.copy_from_host(kb).unwrap();
        v_buf.copy_from_host(vb).unwrap();

        // Build causal mask [n_heads, seq, seq]: 0 for sk ≤ sq, -1e30 otherwise.
        // Replicated per head for simplicity.
        let mut mask = vec![0.0f32; n_heads * seq * seq];
        for h in 0..n_heads {
            for sq in 0..seq {
                for sk in 0..seq {
                    if sk > sq {
                        mask[(h * seq + sq) * seq + sk] = -1e30;
                    }
                }
            }
        }
        let mb = unsafe { std::slice::from_raw_parts(mask.as_ptr() as *const u8, mask.len() * 4) };
        m_buf.copy_from_host(mb).unwrap();

        // Stage 1: scores = Q @ K^T * scale.
        attn_scores_fwd(
            &s_buf, &q_buf, &k_buf, n_heads, n_kv_heads, seq, head_dim, scale,
        )
        .unwrap();

        // Stage 2: masked softmax, row = (n_heads * seq), col = seq.
        softmax::softmax_fwd_masked(&p_buf, &s_buf, &m_buf, n_heads * seq, seq).unwrap();

        // Stage 3: output = probs @ V.
        attn_output_fwd(&o_buf, &p_buf, &v_buf, n_heads, n_kv_heads, seq, head_dim).unwrap();

        let mut gpu_out = vec![0.0f32; cpu_out.len()];
        o_buf.download_f32(&mut gpu_out).unwrap();

        let c = cosine(&cpu_out, &gpu_out);
        let max_abs = cpu_out
            .iter()
            .zip(&gpu_out)
            .map(|(a, b)| (a - b).abs())
            .fold(0.0f32, f32::max);
        println!("attn E2E cosine={:.6} max_abs_err={:.4e}", c, max_abs);
        assert!(
            c >= 0.999,
            "end-to-end attention cosine {} below 0.999 — mask/kernel mismatch?",
            c
        );
    }
}
