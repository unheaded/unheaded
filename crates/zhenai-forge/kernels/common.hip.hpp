// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// WAVE11 common HIP kernel helpers.

#ifndef WAVE11_COMMON_HIP_HPP
#define WAVE11_COMMON_HIP_HPP

#include <hip/hip_runtime.h>

// Warp size on AMD RDNA/CDNA is 32. Used for block reductions.
#ifndef WAVE11_WARP_SIZE
#define WAVE11_WARP_SIZE 32
#endif

// Launch-bound hint helper. Most kernels use <= 256 threads / block.
#define WAVE11_MAX_THREADS_PER_BLOCK 256

// bf16 (2-byte brain-float) host/device conversion. We don't use __bfloat16
// natively because it isn't fully supported on RDNA 3; instead we treat bf16
// as a uint16 bit-pattern and reconstruct f32 via bit-pun.
//
// bf16 → f32: shift the bits left by 16 into the upper half of a float.
// f32 → bf16: round-nearest-even then truncate.

__device__ __forceinline__ float bf16_bits_to_f32(unsigned short b) {
    unsigned int x = ((unsigned int)b) << 16;
    float f;
    __builtin_memcpy(&f, &x, sizeof(float));
    return f;
}

__device__ __forceinline__ unsigned short f32_to_bf16_bits(float f) {
    unsigned int x;
    __builtin_memcpy(&x, &f, sizeof(unsigned int));
    // Round-to-nearest-even: add 0x7FFF + ((x >> 16) & 1), then truncate.
    unsigned int rounding = 0x00007FFFu + ((x >> 16) & 1u);
    x += rounding;
    return (unsigned short)(x >> 16);
}

// Block-level reduce sum (assumes blockDim.x <= 1024). Uses shared memory.
// Caller provides the shared-memory buffer sized `blockDim.x * sizeof(float)`.
__device__ __forceinline__ float block_reduce_sum(float val, float* shm) {
    const int tid = threadIdx.x;
    shm[tid] = val;
    __syncthreads();
    // Tree reduction; assumes blockDim.x is a power of two.
    for (int stride = blockDim.x / 2; stride > 0; stride >>= 1) {
        if (tid < stride) shm[tid] += shm[tid + stride];
        __syncthreads();
    }
    return shm[0];
}

// Block-level reduce max (same contract as sum).
__device__ __forceinline__ float block_reduce_max(float val, float* shm) {
    const int tid = threadIdx.x;
    shm[tid] = val;
    __syncthreads();
    for (int stride = blockDim.x / 2; stride > 0; stride >>= 1) {
        if (tid < stride) {
            float other = shm[tid + stride];
            if (other > shm[tid]) shm[tid] = other;
        }
        __syncthreads();
    }
    return shm[0];
}

#endif  // WAVE11_COMMON_HIP_HPP
