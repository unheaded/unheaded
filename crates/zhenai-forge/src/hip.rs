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
}
