// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE12 Phase 4: precision-conversion kernels.
//
// Used to keep activations GPU-resident across the forward chain: rmsnorm
// outputs f32, matmul consumes bf16. This kernel does that conversion in
// place on the device instead of downloading to host, converting, uploading.

#include "common.hip.hpp"

__global__ void f32_to_bf16_f32_kernel(unsigned short* __restrict__ out_bf16,
                                       const float* __restrict__ in_f32,
                                       int n) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    int stride = gridDim.x * blockDim.x;
    for (int i = tid; i < n; i += stride) {
        out_bf16[i] = f32_to_bf16_bits(in_f32[i]);
    }
}

extern "C" hipError_t wave12_launch_f32_to_bf16_f32(
    unsigned short* out_bf16, const float* in_f32, int n, hipStream_t stream)
{
    if (n <= 0) return hipSuccess;
    constexpr int threads = 256;
    int blocks = (n + threads - 1) / threads;
    if (blocks > 65535) blocks = 65535; // grid-stride loop handles n > blocks*threads
    hipLaunchKernelGGL(f32_to_bf16_f32_kernel,
                       dim3(blocks), dim3(threads), 0, stream,
                       out_bf16, in_f32, n);
    return hipGetLastError();
}
