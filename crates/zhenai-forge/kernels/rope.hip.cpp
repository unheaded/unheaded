// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 RoPE partial-rotary kernels.
//
// Forward (matching gemma4::rope_apply_partial):
//   For each position s, head h, dim-pair d in 0..rope_dim/2:
//     c = cos[s][d], si = sin[s][d]
//     xe = x[s][h][2d],   xo = x[s][h][2d+1]
//     out[s][h][2d]   = xe * c - xo * si
//     out[s][h][2d+1] = xe * si + xo * c
//   Dims [rope_dim..head_dim] are copied through unchanged.
//
// Backward (matching gemma4::rope_backward_partial):
//   ge = grad_rot[s][h][2d], go = grad_rot[s][h][2d+1]
//   grad_x[s][h][2d]   = ge * c + go * si
//   grad_x[s][h][2d+1] = -ge * si + go * c
//
// Launch: grid = (seq, n_head), block = head_dim (up to 256). Each thread
// handles one dim index.

#include "common.hip.hpp"

__global__ void rope_partial_fwd_f32_kernel(
    float* __restrict__ out, const float* __restrict__ in,
    const float* __restrict__ cos_tab, const float* __restrict__ sin_tab,
    int n_head, int head_dim, int rope_dim)
{
    const int s = blockIdx.x;
    const int h = blockIdx.y;
    const int d = threadIdx.x;
    if (d >= head_dim) return;
    const int half = rope_dim / 2;
    const int off = (s * n_head + h) * head_dim;

    if (d < rope_dim) {
        // Rotated pair. Compute only on even d; odd d is written by the
        // even-d thread via thread-cooperation. But simpler: each thread
        // computes its own output independently using the counterpart.
        // For even d: pair index = d/2 → reads x[off+d], x[off+d+1].
        // For odd d:  pair index = (d-1)/2 → reads x[off+d-1], x[off+d].
        int pair_idx = d / 2;
        float c = cos_tab[s * half + pair_idx];
        float si = sin_tab[s * half + pair_idx];
        float xe = in[off + 2 * pair_idx];
        float xo = in[off + 2 * pair_idx + 1];
        if (d % 2 == 0) {
            out[off + d] = xe * c - xo * si;
        } else {
            out[off + d] = xe * si + xo * c;
        }
    } else {
        // Passthrough for dims [rope_dim..head_dim].
        out[off + d] = in[off + d];
    }
}

__global__ void rope_partial_bwd_f32_kernel(
    float* __restrict__ grad_in, const float* __restrict__ grad_out,
    const float* __restrict__ cos_tab, const float* __restrict__ sin_tab,
    int n_head, int head_dim, int rope_dim)
{
    const int s = blockIdx.x;
    const int h = blockIdx.y;
    const int d = threadIdx.x;
    if (d >= head_dim) return;
    const int half = rope_dim / 2;
    const int off = (s * n_head + h) * head_dim;

    if (d < rope_dim) {
        int pair_idx = d / 2;
        float c = cos_tab[s * half + pair_idx];
        float si = sin_tab[s * half + pair_idx];
        float ge = grad_out[off + 2 * pair_idx];
        float go = grad_out[off + 2 * pair_idx + 1];
        if (d % 2 == 0) {
            grad_in[off + d] = ge * c + go * si;
        } else {
            grad_in[off + d] = -ge * si + go * c;
        }
    } else {
        grad_in[off + d] = grad_out[off + d];
    }
}

extern "C" hipError_t wave11_launch_rope_partial_fwd_f32(
    float* out, const float* in,
    const float* cos_tab, const float* sin_tab,
    int seq, int n_head, int head_dim, int rope_dim,
    hipStream_t stream)
{
    if (seq <= 0 || n_head <= 0 || head_dim <= 0) return hipSuccess;
    // Round threads up to a power of 2 ≤ 256 that covers head_dim.
    int threads = head_dim;
    if (threads > 256) threads = 256;
    hipLaunchKernelGGL(rope_partial_fwd_f32_kernel,
                       dim3(seq, n_head), dim3(threads), 0, stream,
                       out, in, cos_tab, sin_tab, n_head, head_dim, rope_dim);
    return hipGetLastError();
}

extern "C" hipError_t wave11_launch_rope_partial_bwd_f32(
    float* grad_in, const float* grad_out,
    const float* cos_tab, const float* sin_tab,
    int seq, int n_head, int head_dim, int rope_dim,
    hipStream_t stream)
{
    if (seq <= 0 || n_head <= 0 || head_dim <= 0) return hipSuccess;
    int threads = head_dim;
    if (threads > 256) threads = 256;
    hipLaunchKernelGGL(rope_partial_bwd_f32_kernel,
                       dim3(seq, n_head), dim3(threads), 0, stream,
                       grad_in, grad_out, cos_tab, sin_tab,
                       n_head, head_dim, rope_dim);
    return hipGetLastError();
}
