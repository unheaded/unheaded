// SPDX-License-Identifier: GPL-3.0-or-later
//! `BPF_MAP_FREEZE` for ROM_MAP and RV2MBC_MAP (S-ASCEND-2 Steps 157-161,
//! ADR-080 §7). Same call as `cmd/upc-bootctl/src/freeze.rs`: aya 0.13 freezes
//! only the `.rodata` maps it creates itself, so this calls bpf(2) on the map fd
//! aya holds. The freeze blocks syscall-side updates only; the program reads
//! both maps and never writes them.

use std::io;
use std::os::fd::{AsRawFd, BorrowedFd};

/// `BPF_MAP_FREEZE` in `enum bpf_cmd`, include/uapi/linux/bpf.h (Linux 5.2+).
const BPF_MAP_FREEZE: libc::c_long = 22;

/// The `BPF_MAP_FREEZE` member of `union bpf_attr`: `map_fd` alone (libbpf
/// passes `offsetofend(union bpf_attr, map_fd)` = 4 bytes).
#[repr(C)]
struct FreezeAttr {
    map_fd: u32,
}

/// Freeze one map. Errors: EPERM without CAP_BPF, EBUSY when already frozen or
/// mapped writable, EINVAL on a kernel older than 5.2.
pub fn freeze_map(fd: BorrowedFd<'_>) -> io::Result<()> {
    let attr = FreezeAttr {
        map_fd: fd.as_raw_fd() as u32,
    };
    // SAFETY: bpf(2) reads `size_of::<FreezeAttr>()` bytes from `&attr`, which
    // outlives the call; the fd is borrowed, so it stays open for the call.
    let r = unsafe {
        libc::syscall(
            libc::SYS_bpf,
            BPF_MAP_FREEZE,
            &attr as *const FreezeAttr,
            std::mem::size_of::<FreezeAttr>() as libc::c_uint,
        )
    };
    if r < 0 {
        Err(io::Error::last_os_error())
    } else {
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn attr_is_map_fd_only() {
        assert_eq!(std::mem::size_of::<FreezeAttr>(), 4);
        assert_eq!(BPF_MAP_FREEZE, 22);
    }

    #[test]
    fn freeze_of_a_non_bpf_fd_is_refused() {
        let f = std::fs::File::open("/dev/null").unwrap();
        use std::os::fd::AsFd;
        assert!(freeze_map(f.as_fd()).is_err());
    }
}
