//! BPF Map operations.
//!
//! Provides utilities for interacting with various BPF map types
//! including hash maps, arrays, and stack traces.

use std::os::unix::io::RawFd;
use std::path::Path;

use super::{bpf_obj_get, BpfError};
use tracing::debug;

/// Generic BPF map handle
pub struct BpfMap {
    fd: RawFd,
    key_size: u32,
    value_size: u32,
    #[allow(dead_code)]
    max_entries: u32,
    map_type: u32,
    name: String,
}

impl BpfMap {
    /// Open a pinned BPF map
    pub fn open(path: &Path) -> Result<Self, BpfError> {
        let fd = bpf_obj_get(path)?;
        let info = super::bpf_map_info(fd)?;

        debug!(
            name = info.name_str(),
            map_type = info.map_type,
            key_size = info.key_size,
            value_size = info.value_size,
            max_entries = info.max_entries,
            "Opened BPF map"
        );

        Ok(Self {
            fd,
            key_size: info.key_size,
            value_size: info.value_size,
            max_entries: info.max_entries,
            map_type: info.map_type,
            name: info.name_str(),
        })
    }

    /// Get the map file descriptor
    pub fn fd(&self) -> RawFd {
        self.fd
    }

    /// Get the map name
    pub fn name(&self) -> &str {
        &self.name
    }

    /// Lookup a value by key
    pub fn lookup<K, V>(&self, key: &K) -> Result<Option<V>, BpfError>
    where
        K: Sized,
        V: Sized + Default,
    {
        if std::mem::size_of::<K>() != self.key_size as usize {
            return Err(BpfError::Syscall(format!(
                "Key size mismatch: expected {}, got {}",
                self.key_size,
                std::mem::size_of::<K>()
            )));
        }

        if std::mem::size_of::<V>() != self.value_size as usize {
            return Err(BpfError::Syscall(format!(
                "Value size mismatch: expected {}, got {}",
                self.value_size,
                std::mem::size_of::<V>()
            )));
        }

        #[repr(C)]
        struct BpfMapLookupAttr {
            map_fd: u32,
            _pad: u32,
            key: u64,
            value_or_next_key: u64,
            flags: u64,
        }

        let mut value: V = V::default();
        let attr = BpfMapLookupAttr {
            map_fd: self.fd as u32,
            _pad: 0,
            key: key as *const K as u64,
            value_or_next_key: &mut value as *mut V as u64,
            flags: 0,
        };

        let ret = unsafe {
            libc::syscall(
                libc::SYS_bpf,
                1u32, // BPF_MAP_LOOKUP_ELEM
                &attr as *const _,
                std::mem::size_of::<BpfMapLookupAttr>(),
            )
        };

        if ret < 0 {
            let err = std::io::Error::last_os_error();
            if err.raw_os_error() == Some(libc::ENOENT) {
                return Ok(None);
            }
            return Err(BpfError::Syscall(format!("Map lookup failed: {}", err)));
        }

        Ok(Some(value))
    }

    /// Update a map entry
    pub fn update<K, V>(&self, key: &K, value: &V, flags: u64) -> Result<(), BpfError>
    where
        K: Sized,
        V: Sized,
    {
        #[repr(C)]
        struct BpfMapUpdateAttr {
            map_fd: u32,
            _pad: u32,
            key: u64,
            value_or_next_key: u64,
            flags: u64,
        }

        let attr = BpfMapUpdateAttr {
            map_fd: self.fd as u32,
            _pad: 0,
            key: key as *const K as u64,
            value_or_next_key: value as *const V as u64,
            flags,
        };

        let ret = unsafe {
            libc::syscall(
                libc::SYS_bpf,
                2u32, // BPF_MAP_UPDATE_ELEM
                &attr as *const _,
                std::mem::size_of::<BpfMapUpdateAttr>(),
            )
        };

        if ret < 0 {
            return Err(BpfError::Syscall(format!(
                "Map update failed: {}",
                std::io::Error::last_os_error()
            )));
        }

        Ok(())
    }

    /// Delete a map entry
    pub fn delete<K>(&self, key: &K) -> Result<(), BpfError>
    where
        K: Sized,
    {
        #[repr(C)]
        struct BpfMapDeleteAttr {
            map_fd: u32,
            _pad: u32,
            key: u64,
            value_or_next_key: u64,
            flags: u64,
        }

        let attr = BpfMapDeleteAttr {
            map_fd: self.fd as u32,
            _pad: 0,
            key: key as *const K as u64,
            value_or_next_key: 0,
            flags: 0,
        };

        let ret = unsafe {
            libc::syscall(
                libc::SYS_bpf,
                3u32, // BPF_MAP_DELETE_ELEM
                &attr as *const _,
                std::mem::size_of::<BpfMapDeleteAttr>(),
            )
        };

        if ret < 0 {
            return Err(BpfError::Syscall(format!(
                "Map delete failed: {}",
                std::io::Error::last_os_error()
            )));
        }

        Ok(())
    }

    /// Get the next key in the map
    pub fn get_next_key<K>(&self, key: Option<&K>) -> Result<Option<K>, BpfError>
    where
        K: Sized + Default,
    {
        #[repr(C)]
        struct BpfMapGetNextKeyAttr {
            map_fd: u32,
            _pad: u32,
            key: u64,
            value_or_next_key: u64,
            flags: u64,
        }

        let mut next_key: K = K::default();
        let key_ptr = match key {
            Some(k) => k as *const K as u64,
            None => 0,
        };

        let attr = BpfMapGetNextKeyAttr {
            map_fd: self.fd as u32,
            _pad: 0,
            key: key_ptr,
            value_or_next_key: &mut next_key as *mut K as u64,
            flags: 0,
        };

        let ret = unsafe {
            libc::syscall(
                libc::SYS_bpf,
                4u32, // BPF_MAP_GET_NEXT_KEY
                &attr as *const _,
                std::mem::size_of::<BpfMapGetNextKeyAttr>(),
            )
        };

        if ret < 0 {
            let err = std::io::Error::last_os_error();
            if err.raw_os_error() == Some(libc::ENOENT) {
                return Ok(None);
            }
            return Err(BpfError::Syscall(format!("Get next key failed: {}", err)));
        }

        Ok(Some(next_key))
    }
}

impl Drop for BpfMap {
    fn drop(&mut self) {
        unsafe {
            libc::close(self.fd);
        }
    }
}

/// Stack trace map for reading kernel/user stack traces
pub struct StackTraceMap {
    map: BpfMap,
}

/// Maximum stack depth
const MAX_STACK_DEPTH: usize = 127;

/// Stack trace entry
#[repr(C)]
#[derive(Clone, Copy)]
pub struct StackTrace {
    pub ip: [u64; MAX_STACK_DEPTH],
}

impl Default for StackTrace {
    fn default() -> Self {
        Self {
            ip: [0; MAX_STACK_DEPTH],
        }
    }
}

impl StackTraceMap {
    /// Open a stack trace map
    pub fn open(path: &Path) -> Result<Self, BpfError> {
        let map = BpfMap::open(path)?;

        // Verify it's a stack trace map
        if map.map_type != super::BpfMapType::StackTrace as u32 {
            return Err(BpfError::InvalidMapType {
                expected: super::BpfMapType::StackTrace as u32,
                actual: map.map_type,
            });
        }

        Ok(Self { map })
    }

    /// Get a stack trace by ID
    pub fn get(&self, stack_id: i32) -> Result<Option<StackTrace>, BpfError> {
        if stack_id < 0 {
            return Ok(None);
        }

        self.map.lookup(&(stack_id as u32))
    }

    /// Get stack addresses as a vector
    pub fn get_addresses(&self, stack_id: i32) -> Result<Vec<u64>, BpfError> {
        match self.get(stack_id)? {
            Some(trace) => {
                let addrs: Vec<u64> = trace
                    .ip
                    .iter()
                    .take_while(|&&ip| ip != 0)
                    .copied()
                    .collect();
                Ok(addrs)
            }
            None => Ok(Vec::new()),
        }
    }
}

/// Per-CPU array helper
pub struct PerCpuArray<V> {
    map: BpfMap,
    num_cpus: usize,
    _marker: std::marker::PhantomData<V>,
}

impl<V: Sized + Default + Clone> PerCpuArray<V> {
    /// Open a per-CPU array map
    pub fn open(path: &Path) -> Result<Self, BpfError> {
        let map = BpfMap::open(path)?;

        // Verify it's a per-CPU array
        if map.map_type != super::BpfMapType::PerCpuArray as u32 {
            return Err(BpfError::InvalidMapType {
                expected: super::BpfMapType::PerCpuArray as u32,
                actual: map.map_type,
            });
        }

        let num_cpus = std::thread::available_parallelism()
            .map(|p| p.get())
            .unwrap_or(1);

        Ok(Self {
            map,
            num_cpus,
            _marker: std::marker::PhantomData,
        })
    }

    /// Get all per-CPU values for a key
    pub fn get_all(&self, key: u32) -> Result<Vec<V>, BpfError> {
        // For per-CPU maps, the value buffer must be large enough for all CPUs
        let value_size = std::mem::size_of::<V>() * self.num_cpus;
        let mut buffer = vec![0u8; value_size];

        #[repr(C)]
        struct BpfMapLookupAttr {
            map_fd: u32,
            _pad: u32,
            key: u64,
            value_or_next_key: u64,
            flags: u64,
        }

        let attr = BpfMapLookupAttr {
            map_fd: self.map.fd as u32,
            _pad: 0,
            key: &key as *const u32 as u64,
            value_or_next_key: buffer.as_mut_ptr() as u64,
            flags: 0,
        };

        let ret = unsafe {
            libc::syscall(
                libc::SYS_bpf,
                1u32,
                &attr as *const _,
                std::mem::size_of::<BpfMapLookupAttr>(),
            )
        };

        if ret < 0 {
            return Err(BpfError::Syscall(format!(
                "Per-CPU lookup failed: {}",
                std::io::Error::last_os_error()
            )));
        }

        // Parse the values
        let mut values = Vec::with_capacity(self.num_cpus);
        let value_size = std::mem::size_of::<V>();

        for i in 0..self.num_cpus {
            let ptr = unsafe { buffer.as_ptr().add(i * value_size) as *const V };
            let value = unsafe { std::ptr::read_unaligned(ptr) };
            values.push(value);
        }

        Ok(values)
    }

    /// Sum all per-CPU values (for counters)
    pub fn sum(&self, key: u32) -> Result<V, BpfError>
    where
        V: std::ops::Add<Output = V>,
    {
        let values = self.get_all(key)?;
        values
            .into_iter()
            .reduce(|a, b| a + b)
            .ok_or_else(|| BpfError::Syscall("No values".to_string()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_stack_trace_default() {
        let trace = StackTrace::default();
        assert_eq!(trace.ip[0], 0);
        assert_eq!(trace.ip.len(), MAX_STACK_DEPTH);
    }
}
