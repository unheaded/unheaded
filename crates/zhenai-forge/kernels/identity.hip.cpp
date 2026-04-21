// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 identity kernel — proves the HIP → Rust → GPU launch pipeline works.
// First kernel we ever launch. If this fails, nothing else in WAVE11 works.

#include "common.hip.hpp"

__global__ void identity_f32_kernel(float* __restrict__ out,
                                    const float* __restrict__ in,
                                    int n) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < n) out[i] = in[i];
}

extern "C" hipError_t wave11_launch_identity_f32(float* out,
                                                  const float* in,
                                                  int n,
                                                  hipStream_t stream) {
    if (n <= 0) return hipSuccess;
    constexpr int threads = 256;
    int blocks = (n + threads - 1) / threads;
    hipLaunchKernelGGL(identity_f32_kernel,
                       dim3(blocks), dim3(threads), 0, stream,
                       out, in, n);
    return hipGetLastError();
}
