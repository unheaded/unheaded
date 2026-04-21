// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 Softmax kernels (forward + backward + masked variant).
//
// Forward (stable, max-subtract):
//   row-max = max(scores[row])
//   row-sum = sum(exp(scores[row] - row-max))
//   probs[row][col] = exp(scores[row][col] - row-max) / row-sum
//
// Backward (Jacobian-vector product):
//   s = sum_k probs[row][k] * grad_probs[row][k]
//   grad_scores[row][col] = probs[row][col] * (grad_probs[row][col] - s)
//
// Masked forward: caller supplies a `mask` buffer of shape [rows*cols] where
// each element is 0.0 (keep) or a large negative number (-1e30, "mask out").
// Added to scores before max-subtract.

#include "common.hip.hpp"

constexpr int SOFTMAX_THREADS = 256;

__global__ void softmax_fwd_f32_kernel(float* probs, const float* scores,
                                        int rows, int cols) {
    extern __shared__ float shm[];
    const int row = blockIdx.x;
    const int tid = threadIdx.x;
    const float* s_row = scores + row * cols;
    float* p_row       = probs  + row * cols;

    // Pass 1: max.
    float local_max = -INFINITY;
    for (int i = tid; i < cols; i += blockDim.x) {
        float v = s_row[i];
        if (v > local_max) local_max = v;
    }
    float row_max = block_reduce_max(local_max, shm);

    // Pass 2: sum(exp(v - max)).
    float local_sum = 0.0f;
    for (int i = tid; i < cols; i += blockDim.x) {
        local_sum += expf(s_row[i] - row_max);
    }
    float row_sum = block_reduce_sum(local_sum, shm);
    float inv_sum = 1.0f / row_sum;

    // Pass 3: normalize.
    for (int i = tid; i < cols; i += blockDim.x) {
        p_row[i] = expf(s_row[i] - row_max) * inv_sum;
    }
}

__global__ void softmax_fwd_masked_f32_kernel(
    float* probs, const float* scores, const float* mask,
    int rows, int cols)
{
    extern __shared__ float shm[];
    const int row = blockIdx.x;
    const int tid = threadIdx.x;
    const float* s_row = scores + row * cols;
    const float* m_row = mask   + row * cols;
    float* p_row       = probs  + row * cols;

    // Pass 1: max of (score + mask).
    float local_max = -INFINITY;
    for (int i = tid; i < cols; i += blockDim.x) {
        float v = s_row[i] + m_row[i];
        if (v > local_max) local_max = v;
    }
    float row_max = block_reduce_max(local_max, shm);

    // Pass 2: sum.
    float local_sum = 0.0f;
    for (int i = tid; i < cols; i += blockDim.x) {
        local_sum += expf(s_row[i] + m_row[i] - row_max);
    }
    float row_sum = block_reduce_sum(local_sum, shm);
    float inv_sum = 1.0f / row_sum;

    // Pass 3: normalize.
    for (int i = tid; i < cols; i += blockDim.x) {
        p_row[i] = expf(s_row[i] + m_row[i] - row_max) * inv_sum;
    }
}

__global__ void softmax_bwd_f32_kernel(
    float* grad_scores, const float* grad_probs, const float* probs,
    int rows, int cols)
{
    extern __shared__ float shm[];
    const int row = blockIdx.x;
    const int tid = threadIdx.x;
    const float* gp_row = grad_probs + row * cols;
    const float* p_row  = probs      + row * cols;
    float* gs_row       = grad_scores + row * cols;

    // Pass 1: reduce s = sum(probs * grad_probs).
    float local_s = 0.0f;
    for (int i = tid; i < cols; i += blockDim.x) {
        local_s += p_row[i] * gp_row[i];
    }
    float s = block_reduce_sum(local_s, shm);

    // Pass 2: grad_scores[i] = probs[i] * (grad_probs[i] - s).
    for (int i = tid; i < cols; i += blockDim.x) {
        gs_row[i] = p_row[i] * (gp_row[i] - s);
    }
}

extern "C" hipError_t wave11_launch_softmax_fwd_f32(
    float* probs, const float* scores, int rows, int cols, hipStream_t stream)
{
    if (rows <= 0 || cols <= 0) return hipSuccess;
    const size_t shm_bytes = SOFTMAX_THREADS * sizeof(float);
    hipLaunchKernelGGL(softmax_fwd_f32_kernel,
                       dim3(rows), dim3(SOFTMAX_THREADS), shm_bytes, stream,
                       probs, scores, rows, cols);
    return hipGetLastError();
}

extern "C" hipError_t wave11_launch_softmax_fwd_masked_f32(
    float* probs, const float* scores, const float* mask,
    int rows, int cols, hipStream_t stream)
{
    if (rows <= 0 || cols <= 0) return hipSuccess;
    const size_t shm_bytes = SOFTMAX_THREADS * sizeof(float);
    hipLaunchKernelGGL(softmax_fwd_masked_f32_kernel,
                       dim3(rows), dim3(SOFTMAX_THREADS), shm_bytes, stream,
                       probs, scores, mask, rows, cols);
    return hipGetLastError();
}

extern "C" hipError_t wave11_launch_softmax_bwd_f32(
    float* grad_scores, const float* grad_probs, const float* probs,
    int rows, int cols, hipStream_t stream)
{
    if (rows <= 0 || cols <= 0) return hipSuccess;
    const size_t shm_bytes = SOFTMAX_THREADS * sizeof(float);
    hipLaunchKernelGGL(softmax_bwd_f32_kernel,
                       dim3(rows), dim3(SOFTMAX_THREADS), shm_bytes, stream,
                       grad_scores, grad_probs, probs, rows, cols);
    return hipGetLastError();
}
