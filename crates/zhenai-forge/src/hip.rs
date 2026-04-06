// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! HIP (ROCm) GPU interface — raw FFI bindings for AMD GPU compute.
//!
//! Provides zero-dependency access to the RX 7700 XT via HIP runtime API.
//! No hip-sys crate (created 2022, fails age check). Pure extern "C" FFI.

use std::ffi::c_void;
use std::fmt;

/// HIP error codes (subset — full list in hip/hip_runtime_api.h)
#[repr(i32)]
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum HipError {
    Success = 0,
    InvalidValue = 1,
    OutOfMemory = 2,
    NotInitialized = 3,
    InvalidDevice = 100,
    InvalidMemcpyDirection = 21,
    Unknown = 999,
}

impl fmt::Display for HipError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            HipError::Success => write!(f, "Success"),
            HipError::OutOfMemory => write!(f, "Out of GPU memory"),
            HipError::InvalidDevice => write!(f, "Invalid device"),
            _ => write!(f, "HIP error {:?}", self),
        }
    }
}

impl From<i32> for HipError {
    fn from(code: i32) -> Self {
        match code {
            0 => HipError::Success,
            1 => HipError::InvalidValue,
            2 => HipError::OutOfMemory,
            3 => HipError::NotInitialized,
            100 => HipError::InvalidDevice,
            21 => HipError::InvalidMemcpyDirection,
            _ => HipError::Unknown,
        }
    }
}

/// HIP memcpy directions
#[repr(i32)]
pub enum HipMemcpyKind {
    HostToDevice = 1,
    DeviceToHost = 2,
    DeviceToDevice = 3,
}

/// Device properties (subset)
#[repr(C)]
pub struct HipDeviceProperties {
    pub name: [u8; 256],
    pub total_global_mem: usize,
    pub shared_mem_per_block: usize,
    pub warp_size: i32,
    pub max_threads_per_block: i32,
    pub clock_rate: i32,
    pub multi_processor_count: i32,
    pub major: i32,
    pub minor: i32,
    // ... more fields exist but we only need these
    _padding: [u8; 1024], // padding to match actual struct size
}

// Raw HIP runtime API bindings
#[link(name = "amdhip64", kind = "dylib")]
extern "C" {
    fn hipGetDeviceCount(count: *mut i32) -> i32;
    fn hipSetDevice(device_id: i32) -> i32;
    fn hipGetDeviceProperties(prop: *mut HipDeviceProperties, device_id: i32) -> i32;
    fn hipMalloc(ptr: *mut *mut c_void, size: usize) -> i32;
    fn hipFree(ptr: *mut c_void) -> i32;
    fn hipMemcpy(dst: *mut c_void, src: *const c_void, size: usize, kind: i32) -> i32;
    fn hipMemset(ptr: *mut c_void, value: i32, size: usize) -> i32;
    fn hipDeviceSynchronize() -> i32;
    fn hipGetLastError() -> i32;
}

// hipBLAS bindings for GPU matrix multiplication
// Used for LoRA forward/backward passes on RX 7700 XT
pub type HipblasHandle = *mut c_void;

#[repr(i32)]
pub enum HipblasOperation {
    None = 111,        // HIPBLAS_OP_N
    Transpose = 112,   // HIPBLAS_OP_T
}

#[link(name = "hipblas", kind = "dylib")]
extern "C" {
    pub fn hipblasCreate(handle: *mut HipblasHandle) -> i32;
    pub fn hipblasDestroy(handle: HipblasHandle) -> i32;

    /// C = alpha * A * B + beta * C
    /// Single-precision general matrix multiply
    pub fn hipblasSgemm(
        handle: HipblasHandle,
        transa: i32,    // HipblasOperation
        transb: i32,
        m: i32,         // rows of A (and C)
        n: i32,         // cols of B (and C)
        k: i32,         // cols of A / rows of B
        alpha: *const f32,
        a: *const c_void,  // device pointer
        lda: i32,
        b: *const c_void,
        ldb: i32,
        beta: *const f32,
        c: *mut c_void,
        ldc: i32,
    ) -> i32;
}

/// hipBLAS handle wrapper with RAII.
pub struct BlasHandle {
    handle: HipblasHandle,
}

impl BlasHandle {
    pub fn new() -> Result<Self, HipError> {
        let mut handle: HipblasHandle = std::ptr::null_mut();
        check(unsafe { hipblasCreate(&mut handle) })?;
        Ok(BlasHandle { handle })
    }

    /// GPU matrix multiply: C = alpha * A * B + beta * C
    /// A: (m × k), B: (k × n), C: (m × n), all in GPU memory
    pub fn sgemm(
        &self,
        m: i32, n: i32, k: i32,
        alpha: f32,
        a: &GpuBuffer, lda: i32,
        b: &GpuBuffer, ldb: i32,
        beta: f32,
        c: &GpuBuffer, ldc: i32,
    ) -> Result<(), HipError> {
        check(unsafe {
            hipblasSgemm(
                self.handle,
                HipblasOperation::None as i32,
                HipblasOperation::None as i32,
                m, n, k,
                &alpha,
                a.as_ptr() as *const c_void, lda,
                b.as_ptr() as *const c_void, ldb,
                &beta,
                c.as_ptr() as *mut c_void, ldc,
            )
        })
    }

    pub fn raw_handle(&self) -> HipblasHandle {
        self.handle
    }
}

impl Drop for BlasHandle {
    fn drop(&mut self) {
        if !self.handle.is_null() {
            unsafe { hipblasDestroy(self.handle) };
        }
    }
}

/// Check a HIP call result.
fn check(code: i32) -> Result<(), HipError> {
    let err = HipError::from(code);
    if err == HipError::Success {
        Ok(())
    } else {
        Err(err)
    }
}

/// GPU device handle.
pub struct GpuDevice {
    pub device_id: i32,
    pub name: String,
    pub vram_bytes: usize,
    pub compute_units: i32,
}

impl GpuDevice {
    /// Initialize GPU and return device info.
    pub fn init(device_id: i32) -> Result<Self, HipError> {
        let mut count = 0i32;
        check(unsafe { hipGetDeviceCount(&mut count) })?;

        if device_id >= count {
            return Err(HipError::InvalidDevice);
        }

        check(unsafe { hipSetDevice(device_id) })?;

        let mut props = unsafe { std::mem::zeroed::<HipDeviceProperties>() };
        check(unsafe { hipGetDeviceProperties(&mut props, device_id) })?;

        let name = {
            let end = props.name.iter().position(|&b| b == 0).unwrap_or(255);
            String::from_utf8_lossy(&props.name[..end]).to_string()
        };

        Ok(GpuDevice {
            device_id,
            name,
            vram_bytes: props.total_global_mem,
            compute_units: props.multi_processor_count,
        })
    }
}

/// GPU memory buffer — RAII wrapper around hipMalloc/hipFree.
pub struct GpuBuffer {
    ptr: *mut c_void,
    size: usize,
}

impl GpuBuffer {
    /// Allocate GPU memory.
    pub fn alloc(size: usize) -> Result<Self, HipError> {
        let mut ptr: *mut c_void = std::ptr::null_mut();
        check(unsafe { hipMalloc(&mut ptr, size) })?;
        Ok(GpuBuffer { ptr, size })
    }

    /// Copy data from host (RAM) to this GPU buffer.
    pub fn copy_from_host(&self, data: &[u8]) -> Result<(), HipError> {
        assert!(data.len() <= self.size);
        check(unsafe {
            hipMemcpy(
                self.ptr,
                data.as_ptr() as *const c_void,
                data.len(),
                HipMemcpyKind::HostToDevice as i32,
            )
        })
    }

    /// Copy data from this GPU buffer to host (RAM).
    pub fn copy_to_host(&self, data: &mut [u8]) -> Result<(), HipError> {
        assert!(data.len() <= self.size);
        check(unsafe {
            hipMemcpy(
                data.as_mut_ptr() as *mut c_void,
                self.ptr as *const c_void,
                data.len(),
                HipMemcpyKind::DeviceToHost as i32,
            )
        })
    }

    /// Zero-fill GPU memory.
    pub fn zero(&self) -> Result<(), HipError> {
        check(unsafe { hipMemset(self.ptr, 0, self.size) })
    }

    /// Raw pointer for kernel arguments.
    pub fn as_ptr(&self) -> *mut c_void {
        self.ptr
    }

    /// Buffer size in bytes.
    pub fn len(&self) -> usize {
        self.size
    }
}

impl Drop for GpuBuffer {
    fn drop(&mut self) {
        if !self.ptr.is_null() {
            unsafe { hipFree(self.ptr) };
        }
    }
}

// Safe f32 cast helpers (pub for use in train.rs)
pub fn f32_as_bytes(data: &[f32]) -> &[u8] {
    unsafe { std::slice::from_raw_parts(data.as_ptr() as *const u8, data.len() * 4) }
}

pub fn f32_as_bytes_mut(data: &mut [f32]) -> &mut [u8] {
    unsafe { std::slice::from_raw_parts_mut(data.as_mut_ptr() as *mut u8, data.len() * 4) }
}

impl GpuBuffer {
    /// Upload f32 slice from CPU to GPU.
    pub fn upload_f32(&self, data: &[f32]) -> Result<(), HipError> {
        self.copy_from_host(f32_as_bytes(data))
    }

    /// Download f32 data from GPU to CPU.
    pub fn download_f32(&self, data: &mut [f32]) -> Result<(), HipError> {
        self.copy_to_host(f32_as_bytes_mut(data))
    }
}

impl BlasHandle {
    /// GPU matmul with transpose support: C = alpha * op(A) * op(B) + beta * C
    /// transa/transb: false = no-transpose, true = transpose
    pub fn sgemm_ex(
        &self,
        transa: bool, transb: bool,
        m: i32, n: i32, k: i32,
        alpha: f32,
        a: &GpuBuffer, lda: i32,
        b: &GpuBuffer, ldb: i32,
        beta: f32,
        c: &GpuBuffer, ldc: i32,
    ) -> Result<(), HipError> {
        let op_a = if transa { HipblasOperation::Transpose as i32 } else { HipblasOperation::None as i32 };
        let op_b = if transb { HipblasOperation::Transpose as i32 } else { HipblasOperation::None as i32 };
        check(unsafe {
            hipblasSgemm(
                self.handle, op_a, op_b,
                m, n, k,
                &alpha,
                a.as_ptr() as *const c_void, lda,
                b.as_ptr() as *const c_void, ldb,
                &beta,
                c.as_ptr() as *mut c_void, ldc,
            )
        })
    }
}

/// Synchronize GPU — wait for all operations to complete.
pub fn sync() -> Result<(), HipError> {
    check(unsafe { hipDeviceSynchronize() })
}

/// Get last HIP error (for debugging).
pub fn last_error() -> HipError {
    HipError::from(unsafe { hipGetLastError() })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_hip_device_init() {
        match GpuDevice::init(0) {
            Ok(dev) => {
                println!("GPU: {} ({:.1} GB VRAM, {} CUs)",
                    dev.name, dev.vram_bytes as f64 / 1e9, dev.compute_units);
                assert!(!dev.name.is_empty());
                assert!(dev.vram_bytes > 0);
            }
            Err(e) => {
                println!("No GPU available ({}), skipping GPU tests", e);
            }
        }
    }

    #[test]
    fn test_gpu_buffer_alloc_free() {
        if GpuDevice::init(0).is_err() {
            println!("No GPU, skipping");
            return;
        }

        let buf = GpuBuffer::alloc(1024).expect("alloc failed");
        assert_eq!(buf.len(), 1024);
        buf.zero().expect("zero failed");
        // buf drops here, hipFree called
    }

    #[test]
    fn test_gpu_memcpy_roundtrip() {
        if GpuDevice::init(0).is_err() {
            println!("No GPU, skipping");
            return;
        }

        let host_data: Vec<u8> = vec![42, 69, 255, 0, 1, 2, 3, 4];
        let buf = GpuBuffer::alloc(host_data.len()).expect("alloc");

        // Host → GPU
        buf.copy_from_host(&host_data).expect("H2D");
        sync().expect("sync");

        // GPU → Host
        let mut result = vec![0u8; host_data.len()];
        buf.copy_to_host(&mut result).expect("D2H");

        assert_eq!(host_data, result, "Round-trip data mismatch");
    }

    #[test]
    fn test_hipblas_sgemm() {
        if GpuDevice::init(0).is_err() {
            println!("No GPU, skipping");
            return;
        }

        let blas = BlasHandle::new().expect("hipblas create failed");

        // Simple 2x2 matrix multiply: C = A * B
        // A = [[1, 2], [3, 4]]  B = [[5, 6], [7, 8]]
        // C = [[1*5+2*7, 1*6+2*8], [3*5+4*7, 3*6+4*8]] = [[19, 22], [43, 50]]
        // Note: hipBLAS uses column-major, so we store transposed
        let a_data: Vec<f32> = vec![1.0, 3.0, 2.0, 4.0]; // col-major
        let b_data: Vec<f32> = vec![5.0, 7.0, 6.0, 8.0]; // col-major
        let c_data: Vec<f32> = vec![0.0; 4];

        let a_buf = GpuBuffer::alloc(16).expect("alloc A");
        let b_buf = GpuBuffer::alloc(16).expect("alloc B");
        let c_buf = GpuBuffer::alloc(16).expect("alloc C");

        a_buf.copy_from_host(bytemuck_cast(&a_data)).expect("H2D A");
        b_buf.copy_from_host(bytemuck_cast(&b_data)).expect("H2D B");
        c_buf.copy_from_host(bytemuck_cast(&c_data)).expect("H2D C");

        blas.sgemm(2, 2, 2, 1.0, &a_buf, 2, &b_buf, 2, 0.0, &c_buf, 2).expect("sgemm");
        sync().expect("sync");

        let mut result = vec![0.0f32; 4];
        c_buf.copy_to_host(bytemuck_cast_mut(&mut result)).expect("D2H C");

        // Column-major result: [19, 43, 22, 50]
        assert!((result[0] - 19.0).abs() < 0.01, "C[0,0] = {} (expected 19)", result[0]);
        assert!((result[1] - 43.0).abs() < 0.01, "C[1,0] = {} (expected 43)", result[1]);
        assert!((result[2] - 22.0).abs() < 0.01, "C[0,1] = {} (expected 22)", result[2]);
        assert!((result[3] - 50.0).abs() < 0.01, "C[1,1] = {} (expected 50)", result[3]);

        println!("hipBLAS SGEMM verified: [[19, 22], [43, 50]]");
    }

    fn bytemuck_cast(data: &[f32]) -> &[u8] {
        unsafe { std::slice::from_raw_parts(data.as_ptr() as *const u8, data.len() * 4) }
    }

    fn bytemuck_cast_mut(data: &mut [f32]) -> &mut [u8] {
        unsafe { std::slice::from_raw_parts_mut(data.as_mut_ptr() as *mut u8, data.len() * 4) }
    }
}
