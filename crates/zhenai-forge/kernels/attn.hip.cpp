// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 Attention kernels (GQA-aware, unfused composition).
//
// Forward decomposition:
//   (1) attn_scores_fwd:  scores[h][sq][sk] = scale * sum_d Q[sq,h,d] * K[sk, h_kv, d]
//       where h_kv = h / (n_heads / n_kv_heads) — GQA broadcast.
//   (2) softmax_fwd_masked  (Phase 5, reused)
//   (3) attn_output_fwd:  out[sq][h][d] = sum_k probs[h][sq][k] * V[k, h_kv, d]
//
// Backward (ships in a later phase if needed):
//   grad_v    = probs^T @ grad_out
//   grad_probs = grad_out @ V^T
//   grad_scores = softmax_bwd(grad_probs, probs)   (Phase 5, reused)
//   grad_q     = grad_scores @ K * scale
//   grad_k     = grad_scores^T @ Q * scale
//   GQA collapse on grad_k, grad_v.

#include "common.hip.hpp"

// ============================================================================
// attn_scores_fwd: compute pre-softmax attention scores with GQA broadcast.
//
// Launch: grid = (n_heads, seq_q), block = threads covering seq_k with
// strided iteration (blockDim.x ≤ 256). Each thread computes ONE score
// for (head h, query pos sq, key pos sk).
//
// Indexing:
//   Q [seq, n_heads,    head_dim]  — layout (sq * n_heads + h) * head_dim + d
//   K [seq, n_kv_heads, head_dim]  — layout (sk * n_kv_heads + h_kv) * head_dim + d
//   scores [n_heads, seq, seq]     — layout (h * seq + sq) * seq + sk
// ============================================================================

__global__ void attn_scores_fwd_f32_kernel(
    float* __restrict__ scores,
    const float* __restrict__ q,
    const float* __restrict__ k,
    int n_heads, int n_kv_heads, int seq, int head_dim, float scale)
{
    const int h  = blockIdx.x;
    const int sq = blockIdx.y;
    const int group_size = n_heads / n_kv_heads;
    const int h_kv = h / group_size;

    // Each thread handles one or more sk positions via strided loop.
    const int tid = threadIdx.x;
    const int stride = blockDim.x;

    const int q_off = (sq * n_heads + h) * head_dim;
    const int scores_row_off = (h * seq + sq) * seq;

    for (int sk = tid; sk < seq; sk += stride) {
        const int k_off = (sk * n_kv_heads + h_kv) * head_dim;
        float dot = 0.0f;
        for (int d = 0; d < head_dim; ++d) {
            dot += q[q_off + d] * k[k_off + d];
        }
        scores[scores_row_off + sk] = dot * scale;
    }
}

// ============================================================================
// attn_output_fwd: combine probs and V into the per-position attention output.
//
// Launch: grid = (seq_q, n_heads), block = head_dim (up to 256). Each thread
// computes one output dim d by summing probs[h][sq][k] * V[k, h_kv, d] over k.
// ============================================================================

__global__ void attn_output_fwd_f32_kernel(
    float* __restrict__ out,
    const float* __restrict__ probs,
    const float* __restrict__ v,
    int n_heads, int n_kv_heads, int seq, int head_dim)
{
    const int sq = blockIdx.x;
    const int h  = blockIdx.y;
    const int d  = threadIdx.x;
    if (d >= head_dim) return;

    const int group_size = n_heads / n_kv_heads;
    const int h_kv = h / group_size;

    const int probs_row_off = (h * seq + sq) * seq;
    const int out_off = (sq * n_heads + h) * head_dim + d;

    float sum = 0.0f;
    for (int k = 0; k < seq; ++k) {
        float p = probs[probs_row_off + k];
        float vv = v[(k * n_kv_heads + h_kv) * head_dim + d];
        sum += p * vv;
    }
    out[out_off] = sum;
}

extern "C" hipError_t wave11_launch_attn_scores_fwd_f32(
    float* scores, const float* q, const float* k,
    int n_heads, int n_kv_heads, int seq, int head_dim, float scale,
    hipStream_t stream)
{
    if (seq <= 0 || n_heads <= 0 || n_kv_heads <= 0 || head_dim <= 0) return hipSuccess;
    int threads = seq;
    if (threads > 256) threads = 256;
    hipLaunchKernelGGL(attn_scores_fwd_f32_kernel,
                       dim3(n_heads, seq), dim3(threads), 0, stream,
                       scores, q, k, n_heads, n_kv_heads, seq, head_dim, scale);
    return hipGetLastError();
}

extern "C" hipError_t wave11_launch_attn_output_fwd_f32(
    float* out, const float* probs, const float* v,
    int n_heads, int n_kv_heads, int seq, int head_dim,
    hipStream_t stream)
{
    if (seq <= 0 || n_heads <= 0 || head_dim <= 0) return hipSuccess;
    int threads = head_dim;
    if (threads > 256) threads = 256;
    hipLaunchKernelGGL(attn_output_fwd_f32_kernel,
                       dim3(seq, n_heads), dim3(threads), 0, stream,
                       out, probs, v, n_heads, n_kv_heads, seq, head_dim);
    return hipGetLastError();
}

// ============================================================================
// BACKWARD KERNELS
//
// Forward recap:
//   scores[h][sq][sk] = scale * sum_d Q[sq,h,d] * K[sk,h_kv,d]
//   probs  = softmax_masked(scores)
//   out[sq][h][d] = sum_k probs[h][sq][k] * V[k,h_kv,d]
//
// Backward:
//   grad_V[sk, h_kv, d] = sum_{h in group(h_kv), sq} probs[h][sq][sk] * grad_out[sq][h][d]
//   grad_probs[h][sq][sk] = sum_d grad_out[sq][h][d] * V[sk, h_kv, d]
//   grad_scores = softmax_bwd(grad_probs, probs)  [Phase 5 reused]
//   grad_Q[sq, h, d] = scale * sum_sk grad_scores[h][sq][sk] * K[sk, h_kv, d]
//   grad_K[sk, h_kv, d] = scale * sum_{h in group(h_kv), sq} grad_scores[h][sq][sk] * Q[sq, h, d]
//
// grad_V and grad_K naturally sum over all heads in the KV-group, so GQA
// collapse is built into the kernel (one thread per (sk, h_kv, d), loops
// over both sq and group heads). No atomics, no temporary per-head buffers.
// ============================================================================

__global__ void attn_grad_v_kernel(
    float* __restrict__ grad_v,
    const float* __restrict__ probs,
    const float* __restrict__ grad_out,
    int n_heads, int n_kv_heads, int seq, int head_dim)
{
    const int sk = blockIdx.x;
    const int h_kv = blockIdx.y;
    const int d = threadIdx.x;
    if (d >= head_dim) return;

    const int group_size = n_heads / n_kv_heads;
    const int gv_off = (sk * n_kv_heads + h_kv) * head_dim + d;

    float sum = 0.0f;
    for (int gh = 0; gh < group_size; ++gh) {
        const int h = h_kv * group_size + gh;
        for (int sq = 0; sq < seq; ++sq) {
            float p  = probs[(h * seq + sq) * seq + sk];
            float go = grad_out[(sq * n_heads + h) * head_dim + d];
            sum += p * go;
        }
    }
    grad_v[gv_off] = sum;
}

__global__ void attn_grad_probs_kernel(
    float* __restrict__ grad_probs,
    const float* __restrict__ grad_out,
    const float* __restrict__ v,
    int n_heads, int n_kv_heads, int seq, int head_dim)
{
    const int h  = blockIdx.x;
    const int sq = blockIdx.y;
    const int sk = threadIdx.x;
    if (sk >= seq) return;

    const int group_size = n_heads / n_kv_heads;
    const int h_kv = h / group_size;

    float sum = 0.0f;
    for (int d = 0; d < head_dim; ++d) {
        float go = grad_out[(sq * n_heads + h) * head_dim + d];
        float vv = v[(sk * n_kv_heads + h_kv) * head_dim + d];
        sum += go * vv;
    }
    grad_probs[(h * seq + sq) * seq + sk] = sum;
}

__global__ void attn_grad_q_kernel(
    float* __restrict__ grad_q,
    const float* __restrict__ grad_scores,
    const float* __restrict__ k,
    int n_heads, int n_kv_heads, int seq, int head_dim, float scale)
{
    const int sq = blockIdx.x;
    const int h  = blockIdx.y;
    const int d  = threadIdx.x;
    if (d >= head_dim) return;

    const int group_size = n_heads / n_kv_heads;
    const int h_kv = h / group_size;

    float sum = 0.0f;
    for (int sk = 0; sk < seq; ++sk) {
        float gs = grad_scores[(h * seq + sq) * seq + sk];
        float kv = k[(sk * n_kv_heads + h_kv) * head_dim + d];
        sum += gs * kv;
    }
    grad_q[(sq * n_heads + h) * head_dim + d] = sum * scale;
}

__global__ void attn_grad_k_kernel(
    float* __restrict__ grad_k,
    const float* __restrict__ grad_scores,
    const float* __restrict__ q,
    int n_heads, int n_kv_heads, int seq, int head_dim, float scale)
{
    const int sk = blockIdx.x;
    const int h_kv = blockIdx.y;
    const int d = threadIdx.x;
    if (d >= head_dim) return;

    const int group_size = n_heads / n_kv_heads;
    const int gk_off = (sk * n_kv_heads + h_kv) * head_dim + d;

    float sum = 0.0f;
    for (int gh = 0; gh < group_size; ++gh) {
        const int h = h_kv * group_size + gh;
        for (int sq = 0; sq < seq; ++sq) {
            float gs = grad_scores[(h * seq + sq) * seq + sk];
            float qv = q[(sq * n_heads + h) * head_dim + d];
            sum += gs * qv;
        }
    }
    grad_k[gk_off] = sum * scale;
}

extern "C" hipError_t wave11_launch_attn_grad_v_f32(
    float* grad_v, const float* probs, const float* grad_out,
    int n_heads, int n_kv_heads, int seq, int head_dim,
    hipStream_t stream)
{
    if (seq <= 0) return hipSuccess;
    int threads = head_dim; if (threads > 256) threads = 256;
    hipLaunchKernelGGL(attn_grad_v_kernel,
                       dim3(seq, n_kv_heads), dim3(threads), 0, stream,
                       grad_v, probs, grad_out, n_heads, n_kv_heads, seq, head_dim);
    return hipGetLastError();
}

extern "C" hipError_t wave11_launch_attn_grad_probs_f32(
    float* grad_probs, const float* grad_out, const float* v,
    int n_heads, int n_kv_heads, int seq, int head_dim,
    hipStream_t stream)
{
    if (seq <= 0) return hipSuccess;
    int threads = seq; if (threads > 256) threads = 256;
    hipLaunchKernelGGL(attn_grad_probs_kernel,
                       dim3(n_heads, seq), dim3(threads), 0, stream,
                       grad_probs, grad_out, v, n_heads, n_kv_heads, seq, head_dim);
    return hipGetLastError();
}

extern "C" hipError_t wave11_launch_attn_grad_q_f32(
    float* grad_q, const float* grad_scores, const float* k,
    int n_heads, int n_kv_heads, int seq, int head_dim, float scale,
    hipStream_t stream)
{
    if (seq <= 0) return hipSuccess;
    int threads = head_dim; if (threads > 256) threads = 256;
    hipLaunchKernelGGL(attn_grad_q_kernel,
                       dim3(seq, n_heads), dim3(threads), 0, stream,
                       grad_q, grad_scores, k, n_heads, n_kv_heads, seq, head_dim, scale);
    return hipGetLastError();
}

extern "C" hipError_t wave11_launch_attn_grad_k_f32(
    float* grad_k, const float* grad_scores, const float* q,
    int n_heads, int n_kv_heads, int seq, int head_dim, float scale,
    hipStream_t stream)
{
    if (seq <= 0) return hipSuccess;
    int threads = head_dim; if (threads > 256) threads = 256;
    hipLaunchKernelGGL(attn_grad_k_kernel,
                       dim3(seq, n_kv_heads), dim3(threads), 0, stream,
                       grad_k, grad_scores, q, n_heads, n_kv_heads, seq, head_dim, scale);
    return hipGetLastError();
}
