//! BPF interaction layer.
//!
//! Provides direct access to BPF ring buffers, perf event arrays,
//! and BPF maps without using high-level libraries like aya.
//!
//! We use raw syscalls and mmap for maximum performance and
//! educational value.
//!
//! ## Modules
//!
//! - `ringbuf` - Ring buffer reading with zero-copy semantics
//! - `perf` - Perf event array reading (per-CPU)
//! - `maps` - BPF map operations
//! - `multi_source` - Unified reading from multiple eBPF programs

pub mod maps;
pub mod multi_source;
pub mod perf;
pub mod ringbuf;

pub use multi_source::{
    EventSource, GlobalStats, MultiSourceConfig, MultiSourceReader, MultiSourceReaderBuilder,
    SourceConfig, SourceStats, SourcedEvent,
};
pub use perf::PerfEventReader;
pub use ringbuf::RingBufReader;

use std::io;
use std::os::unix::io::RawFd;

use thiserror::Error;

/// BPF-related errors
#[derive(Error, Debug)]
pub enum BpfError {
    #[error("Failed to open BPF map: {0}")]
    MapOpen(#[from] io::Error),

    #[error("Failed to mmap BPF ring buffer: {0}")]
    Mmap(String),

    #[error("Invalid map type: expected {expected}, got {actual}")]
    InvalidMapType { expected: u32, actual: u32 },

    #[error("BPF syscall failed: {0}")]
    Syscall(String),

    #[error("Ring buffer overflow")]
    RingBufferOverflow,

    #[error("Perf event error: {0}")]
    PerfEvent(String),

    #[error("Event parsing error: {0}")]
    Parse(String),
}

/// BPF map types (from linux/bpf.h)
#[repr(u32)]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum BpfMapType {
    Unspec = 0,
    Hash = 1,
    Array = 2,
    ProgArray = 3,
    PerfEventArray = 4,
    PerCpuHash = 5,
    PerCpuArray = 6,
    StackTrace = 7,
    CgroupArray = 8,
    LruHash = 9,
    LruPerCpuHash = 10,
    LpmTrie = 11,
    ArrayOfMaps = 12,
    HashOfMaps = 13,
    DevMap = 14,
    SockMap = 15,
    CpuMap = 16,
    XskMap = 17,
    SockHash = 18,
    CgroupStorage = 19,
    ReusePortSockArray = 20,
    PerCpuCgroupStorage = 21,
    Queue = 22,
    Stack = 23,
    SkStorage = 24,
    DevMapHash = 25,
    StructOps = 26,
    RingBuf = 27,
    InodeStorage = 28,
    TaskStorage = 29,
    BloomFilter = 30,
}

/// BPF commands for bpf() syscall
#[repr(u32)]
#[allow(dead_code)]
pub enum BpfCmd {
    MapCreate = 0,
    MapLookupElem = 1,
    MapUpdateElem = 2,
    MapDeleteElem = 3,
    MapGetNextKey = 4,
    ProgLoad = 5,
    ObjPin = 6,
    ObjGet = 7,
    ProgAttach = 8,
    ProgDetach = 9,
    ProgTestRun = 10,
    ProgGetNextId = 11,
    MapGetNextId = 12,
    ProgGetFdById = 13,
    MapGetFdById = 14,
    ObjGetInfoByFd = 15,
    ProgQuery = 16,
    RawTracepointOpen = 17,
    BtfLoad = 18,
    BtfGetFdById = 19,
    TaskFdQuery = 20,
    MapLookupAndDeleteElem = 21,
    MapFreeze = 22,
    BtfGetNextId = 23,
    MapLookupBatch = 24,
    MapLookupAndDeleteBatch = 25,
    MapUpdateBatch = 26,
    MapDeleteBatch = 27,
    LinkCreate = 28,
    LinkUpdate = 29,
    LinkGetFdById = 30,
    LinkGetNextId = 31,
    EnableStats = 32,
    IterCreate = 33,
    LinkDetach = 34,
    ProgBindMap = 35,
}

/// bpf() syscall wrapper
#[inline]
pub fn bpf_syscall(cmd: BpfCmd, attr: *const libc::c_void, size: u32) -> Result<RawFd, BpfError> {
    let ret = unsafe { libc::syscall(libc::SYS_bpf, cmd as u32, attr, size) };

    if ret < 0 {
        Err(BpfError::Syscall(format!(
            "bpf syscall failed with errno: {}",
            io::Error::last_os_error()
        )))
    } else {
        Ok(ret as RawFd)
    }
}

/// Open a pinned BPF object (map or program)
pub fn bpf_obj_get(path: &std::path::Path) -> Result<RawFd, BpfError> {
    use std::ffi::CString;
    use std::os::unix::ffi::OsStrExt;

    let path_cstr = CString::new(path.as_os_str().as_bytes())
        .map_err(|e| BpfError::Syscall(format!("Invalid path: {}", e)))?;

    // BPF_OBJ_GET attr structure
    #[repr(C)]
    struct BpfObjGetAttr {
        pathname: u64,
        bpf_fd: u32,
        file_flags: u32,
    }

    let attr = BpfObjGetAttr {
        pathname: path_cstr.as_ptr() as u64,
        bpf_fd: 0,
        file_flags: 0,
    };

    bpf_syscall(
        BpfCmd::ObjGet,
        &attr as *const _ as *const libc::c_void,
        std::mem::size_of_val(&attr) as u32,
    )
}

/// Get information about a BPF map
pub fn bpf_map_info(fd: RawFd) -> Result<BpfMapInfo, BpfError> {
    #[repr(C)]
    struct BpfObjInfoAttr {
        bpf_fd: u32,
        info_len: u32,
        info: u64,
    }

    let mut info = BpfMapInfo::default();
    let attr = BpfObjInfoAttr {
        bpf_fd: fd as u32,
        info_len: std::mem::size_of::<BpfMapInfo>() as u32,
        info: &mut info as *mut _ as u64,
    };

    bpf_syscall(
        BpfCmd::ObjGetInfoByFd,
        &attr as *const _ as *const libc::c_void,
        std::mem::size_of_val(&attr) as u32,
    )?;

    Ok(info)
}

/// BPF map information structure (matches kernel struct bpf_map_info)
#[repr(C)]
#[derive(Debug, Clone, Default)]
pub struct BpfMapInfo {
    pub map_type: u32,
    pub id: u32,
    pub key_size: u32,
    pub value_size: u32,
    pub max_entries: u32,
    pub map_flags: u32,
    pub name: [u8; 16],
    pub ifindex: u32,
    pub btf_vmlinux_value_type_id: u32,
    pub netns_dev: u64,
    pub netns_ino: u64,
    pub btf_id: u32,
    pub btf_key_type_id: u32,
    pub btf_value_type_id: u32,
    _pad: u32,
    pub map_extra: u64,
}

impl BpfMapInfo {
    pub fn name_str(&self) -> String {
        let end = self.name.iter().position(|&c| c == 0).unwrap_or(16);
        String::from_utf8_lossy(&self.name[..end]).to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_map_type() {
        assert_eq!(BpfMapType::RingBuf as u32, 27);
        assert_eq!(BpfMapType::PerfEventArray as u32, 4);
    }
}
