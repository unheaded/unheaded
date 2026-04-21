// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 RMSNorm kernels.
//
// Forward formula (matching forward::rmsnorm in the CPU reference):
//   ss      = sum(x[r][d]^2 for d in 0..D) / D
//   rms     = sqrt(ss + eps)
//   out[r][d] = weight[d] * x[r][d] / rms
//
// Backward (matching backward::rmsnorm_backward):
//   sum_gwi = sum(grad_out[d] * weight[d] * x[d] for d in 0..D)
//   grad_in[d] = weight[d] * grad_out[d] / rms
//                - x[d] * sum_gwi / (D * rms^3)
//
// Launch:  one block per row, 256 threads per block. Rows loop over a grid
// of blocks. D ≤ 2048 supported directly (threads iterate if D > blockDim).

#include "common.hip.hpp"

constexpr int RMSNORM_THREADS = 256;

__global__ void rmsnorm_fwd_f32_kernel(float* __restrict__ out,
                                       const float* __restrict__ in,
                                       const float* __restrict__ weight,
                                       float eps,
                                       int D) {
    extern __shared__ float shm[];
    const int row = blockIdx.x;
    const int tid = threadIdx.x;
    const float* x_row = in  + row * D;
    float* out_row     = out + row * D;

    // Reduce sum(x^2) across D with thread-strided accumulate.
    float sum_sq = 0.0f;
    for (int i = tid; i < D; i += blockDim.x) {
        float v = x_row[i];
        sum_sq += v * v;
    }
    float ss = block_reduce_sum(sum_sq, shm) / (float)D;
    float rms = sqrtf(ss + eps);
    float rms_inv = 1.0f / rms;

    // Thread-strided write: out = weight * x * rms_inv.
    for (int i = tid; i < D; i += blockDim.x) {
        out_row[i] = weight[i] * x_row[i] * rms_inv;
    }
}

__global__ void rmsnorm_bwd_f32_kernel(float* __restrict__ grad_in,
                                       const float* __restrict__ grad_out,
                                       const float* __restrict__ in,
                                       const float* __restrict__ weight,
                                       float eps,
                                       int D) {
    extern __shared__ float shm[];
    const int row = blockIdx.x;
    const int tid = threadIdx.x;
    const float* x_row  = in       + row * D;
    const float* go_row = grad_out + row * D;
    float* gi_row       = grad_in  + row * D;

    // Pass 1: reduce sum(x^2) → rms.
    float sum_sq = 0.0f;
    for (int i = tid; i < D; i += blockDim.x) {
        float v = x_row[i];
        sum_sq += v * v;
    }
    float ss = block_reduce_sum(sum_sq, shm) / (float)D;
    float rms = sqrtf(ss + eps);
    float rms_inv = 1.0f / rms;
    float rms_inv3 = rms_inv * rms_inv * rms_inv;

    // Pass 2: reduce sum_gwi = sum(grad_out * weight * x).
    float sum_gwi = 0.0f;
    for (int i = tid; i < D; i += blockDim.x) {
        sum_gwi += go_row[i] * weight[i] * x_row[i];
    }
    sum_gwi = block_reduce_sum(sum_gwi, shm);

    // Pass 3: compute grad_in.
    float inv_D = 1.0f / (float)D;
    for (int i = tid; i < D; i += blockDim.x) {
        gi_row[i] = weight[i] * rms_inv * go_row[i]
                  - x_row[i] * sum_gwi * inv_D * rms_inv3;
    }
}

extern "C" hipError_t wave11_launch_rmsnorm_fwd_f32(
    float* out, const float* in, const float* weight,
    float eps, int rows, int D,
    hipStream_t stream)
{
    if (rows <= 0 || D <= 0) return hipSuccess;
    const int threads = RMSNORM_THREADS;
    const int blocks  = rows;
    const size_t shm_bytes = threads * sizeof(float);
    hipLaunchKernelGGL(rmsnorm_fwd_f32_kernel,
                       dim3(blocks), dim3(threads), shm_bytes, stream,
                       out, in, weight, eps, D);
    return hipGetLastError();
}

extern "C" hipError_t wave11_launch_rmsnorm_bwd_f32(
    float* grad_in, const float* grad_out, const float* in, const float* weight,
    float eps, int rows, int D,
    hipStream_t stream)
{
    if (rows <= 0 || D <= 0) return hipSuccess;
    const int threads = RMSNORM_THREADS;
    const int blocks  = rows;
    const size_t shm_bytes = threads * sizeof(float);
    hipLaunchKernelGGL(rmsnorm_bwd_f32_kernel,
                       dim3(blocks), dim3(threads), shm_bytes, stream,
                       grad_in, grad_out, in, weight, eps, D);
    return hipGetLastError();
}
