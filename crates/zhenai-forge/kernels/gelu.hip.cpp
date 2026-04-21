// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 GELU (tanh approximation) kernels.
//
// gelu_tanh_approx(x) = 0.5 * x * (1 + tanh(sqrt(2/pi) * (x + 0.044715 * x^3)))
//
// Derivative (matching gemma4::gelu_tanh_approx_prime):
//   inner    = k * (x + α*x^3)   where k = sqrt(2/pi), α = 0.044715
//   th       = tanh(inner)
//   d_inner  = k * (1 + 3*α*x^2)
//   d/dx     = 0.5 * (1 + th) + 0.5 * x * (1 - th^2) * d_inner
//
// Fused gelu*up forward: out = gelu(gate_pre) * up_pre
// Fused gelu*up backward: given grad_out,
//   gelu_val     = gelu(gate_pre)
//   d_gelu       = gelu'(gate_pre)
//   grad_gate_pre = grad_out * up_pre * d_gelu
//   grad_up_pre   = grad_out * gelu_val

#include "common.hip.hpp"

__device__ __forceinline__ float gelu_tanh_approx(float x) {
    const float SQRT_2_OVER_PI = 0.7978845608028654f;
    const float ALPHA = 0.044715f;
    float inner = SQRT_2_OVER_PI * (x + ALPHA * x * x * x);
    return 0.5f * x * (1.0f + tanhf(inner));
}

__device__ __forceinline__ float gelu_tanh_approx_prime(float x) {
    const float SQRT_2_OVER_PI = 0.7978845608028654f;
    const float ALPHA = 0.044715f;
    float inner = SQRT_2_OVER_PI * (x + ALPHA * x * x * x);
    float th = tanhf(inner);
    float d_inner = SQRT_2_OVER_PI * (1.0f + 3.0f * ALPHA * x * x);
    return 0.5f * (1.0f + th) + 0.5f * x * (1.0f - th * th) * d_inner;
}

__global__ void gelu_fwd_f32_kernel(float* out, const float* in, int n) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < n) out[i] = gelu_tanh_approx(in[i]);
}

__global__ void gelu_bwd_f32_kernel(float* grad_in, const float* grad_out,
                                    const float* in, int n) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < n) grad_in[i] = grad_out[i] * gelu_tanh_approx_prime(in[i]);
}

// Fused: out[i] = gelu(gate_pre[i]) * up_pre[i]
__global__ void gelu_mul_fwd_f32_kernel(float* out,
                                         const float* gate_pre,
                                         const float* up_pre, int n) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < n) out[i] = gelu_tanh_approx(gate_pre[i]) * up_pre[i];
}

// Fused backward: given grad_out, produce grad_gate_pre + grad_up_pre.
__global__ void gelu_mul_bwd_f32_kernel(float* grad_gate_pre,
                                         float* grad_up_pre,
                                         const float* grad_out,
                                         const float* gate_pre,
                                         const float* up_pre,
                                         int n) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < n) {
        float g = gelu_tanh_approx(gate_pre[i]);
        float dg = gelu_tanh_approx_prime(gate_pre[i]);
        grad_gate_pre[i] = grad_out[i] * up_pre[i] * dg;
        grad_up_pre[i]   = grad_out[i] * g;
    }
}

extern "C" hipError_t wave11_launch_gelu_fwd_f32(
    float* out, const float* in, int n, hipStream_t stream)
{
    if (n <= 0) return hipSuccess;
    constexpr int threads = 256;
    int blocks = (n + threads - 1) / threads;
    hipLaunchKernelGGL(gelu_fwd_f32_kernel,
                       dim3(blocks), dim3(threads), 0, stream, out, in, n);
    return hipGetLastError();
}

extern "C" hipError_t wave11_launch_gelu_bwd_f32(
    float* grad_in, const float* grad_out, const float* in, int n,
    hipStream_t stream)
{
    if (n <= 0) return hipSuccess;
    constexpr int threads = 256;
    int blocks = (n + threads - 1) / threads;
    hipLaunchKernelGGL(gelu_bwd_f32_kernel,
                       dim3(blocks), dim3(threads), 0, stream,
                       grad_in, grad_out, in, n);
    return hipGetLastError();
}

extern "C" hipError_t wave11_launch_gelu_mul_fwd_f32(
    float* out, const float* gate_pre, const float* up_pre, int n,
    hipStream_t stream)
{
    if (n <= 0) return hipSuccess;
    constexpr int threads = 256;
    int blocks = (n + threads - 1) / threads;
    hipLaunchKernelGGL(gelu_mul_fwd_f32_kernel,
                       dim3(blocks), dim3(threads), 0, stream,
                       out, gate_pre, up_pre, n);
    return hipGetLastError();
}

extern "C" hipError_t wave11_launch_gelu_mul_bwd_f32(
    float* grad_gate_pre, float* grad_up_pre, const float* grad_out,
    const float* gate_pre, const float* up_pre, int n, hipStream_t stream)
{
    if (n <= 0) return hipSuccess;
    constexpr int threads = 256;
    int blocks = (n + threads - 1) / threads;
    hipLaunchKernelGGL(gelu_mul_bwd_f32_kernel,
                       dim3(blocks), dim3(threads), 0, stream,
                       grad_gate_pre, grad_up_pre, grad_out,
                       gate_pre, up_pre, n);
    return hipGetLastError();
}
