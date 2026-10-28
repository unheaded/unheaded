// SPDX-License-Identifier: GPL-3.0-or-later
//! `BPF_MAP_FREEZE` for ROM_MAP and RV2MBC_MAP (S-ASCEND-2 Steps 157-161,
//! ADR-080 §7; the Lich D1-001 / D6-001 remediation).
//!
//! aya 0.13 freezes only the `.rodata` maps it creates itself (`MapData::finalize`
//! is `pub(crate)`), so this calls bpf(2) directly on the map fd aya holds. The
//! freeze blocks updates from the syscall side only; the XDP program keeps
//! reading both maps, and it never writes them (`ROM_MAP.get` / `RV2MBC_MAP.get`
//! only, `ebpf/monad-cpu-ebpf/src/main.rs`).

use std::io;
use std::os::fd::{AsRawFd, BorrowedFd};
use std::sync::atomic::{AtomicBool, Ordering};

/// `BPF_MAP_FREEZE` in `enum bpf_cmd`, include/uapi/linux/bpf.h (Linux 5.2+).
/// aya-obj 0.2.1's generated binding carries the same value.
const BPF_MAP_FREEZE: libc::c_long = 22;

/// The `BPF_MAP_FREEZE` member of `union bpf_attr`: `map_fd` alone. libbpf passes
/// `offsetofend(union bpf_attr, map_fd)` = 4 bytes; the kernel zero-fills the rest.
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

static CHECK: AtomicBool = AtomicBool::new(false);

/// `boot --freeze-check`: after the freeze, try one userspace update of each
/// image map and require EPERM. Off unless the flag is given; the harness never
/// passes it.
pub fn set_check(on: bool) {
    CHECK.store(on, Ordering::SeqCst);
}

pub fn check_enabled() -> bool {
    CHECK.load(Ordering::SeqCst)
}

/// The line `--freeze-check` prints for `map`, or why the check failed.
/// `update` is the probe's result: `Ok(())` when the kernel accepted the write,
/// else the errno (None when the error carried none).
pub fn probe_verdict(map: &str, update: Result<(), Option<i32>>) -> Result<String, String> {
    match update {
        Err(Some(libc::EPERM)) => Ok(format!("freeze check: {map} update refused (EPERM)")),
        Ok(()) => Err(format!(
            "freeze check: {map} update ACCEPTED: the map is not frozen"
        )),
        Err(e) => Err(format!(
            "freeze check: {map} update failed with errno {e:?}, expected EPERM"
        )),
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
    fn eperm_is_the_pass_line() {
        assert_eq!(
            probe_verdict("ROM_MAP", Err(Some(libc::EPERM))),
            Ok("freeze check: ROM_MAP update refused (EPERM)".to_string())
        );
    }

    #[test]
    fn accepted_update_fails_the_check() {
        let e = probe_verdict("RV2MBC_MAP", Ok(())).unwrap_err();
        assert!(e.contains("not frozen"), "{e}");
    }

    #[test]
    fn other_errno_fails_the_check() {
        assert!(probe_verdict("ROM_MAP", Err(Some(libc::EINVAL))).is_err());
        assert!(probe_verdict("ROM_MAP", Err(None)).is_err());
    }

    #[test]
    fn freeze_of_a_non_bpf_fd_is_refused() {
        // /dev/null is not a map: bpf(2) refuses it (EBADF/EINVAL, or EPERM
        // when unprivileged), and the call returns an error, never Ok.
        let f = std::fs::File::open("/dev/null").unwrap();
        use std::os::fd::AsFd;
        assert!(freeze_map(f.as_fd()).is_err());
    }
}
