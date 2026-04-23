// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE12 Phase 5: elementwise ops for GPU-resident residual streams.
//
// Currently just add: out[i] = a[i] + b[i]. Used to keep residual
// additions on the GPU between rmsnorm outputs and the running hidden
// stream across all 35 transformer layers, eliminating per-layer
// download/upload of n_embd*seq worth of f32.

#include "common.hip.hpp"

__global__ void add_f32_kernel(float* __restrict__ out,
                               const float* __restrict__ a,
                               const float* __restrict__ b,
                               int n) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    int stride = gridDim.x * blockDim.x;
    for (int i = tid; i < n; i += stride) {
        out[i] = a[i] + b[i];
    }
}

extern "C" hipError_t wave12_launch_add_f32(
    float* out, const float* a, const float* b, int n, hipStream_t stream)
{
    if (n <= 0) return hipSuccess;
    constexpr int threads = 256;
    int blocks = (n + threads - 1) / threads;
    if (blocks > 65535) blocks = 65535;
    hipLaunchKernelGGL(add_f32_kernel,
                       dim3(blocks), dim3(threads), 0, stream,
                       out, a, b, n);
    return hipGetLastError();
}
