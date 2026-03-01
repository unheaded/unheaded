// SPDX-License-Identifier: GPL-2.0-only
// Raw Linux syscall wrappers — zero libc dependency
// Sacred Law: Rust std only, inline asm for kernel calls

use std::arch::asm;

// =============================================================================
// Syscall Numbers (x86_64 Linux ABI)
// =============================================================================

const SYS_MMAP: u64 = 9;
const SYS_MUNMAP: u64 = 11;
const SYS_SOCKET: u64 = 41;
const SYS_BIND: u64 = 49;
const SYS_SETSOCKOPT: u64 = 54;
const SYS_GETSOCKOPT: u64 = 55;
const SYS_SENDTO: u64 = 44;
const SYS_CLOSE: u64 = 3;
const SYS_POLL: u64 = 7;

// =============================================================================
// Low-level syscall dispatch
// =============================================================================

/// Issue a 6-argument syscall. Returns raw kernel result (negative = -errno).
#[inline(always)]
unsafe fn syscall6(nr: u64, a1: u64, a2: u64, a3: u64, a4: u64, a5: u64, a6: u64) -> i64 {
    let ret: i64;
    asm!(
        "syscall",
        inlateout("rax") nr as i64 => ret,
        in("rdi") a1,
        in("rsi") a2,
        in("rdx") a3,
        in("r10") a4,
        in("r8") a5,
        in("r9") a6,
        lateout("rcx") _,
        lateout("r11") _,
        options(nostack),
    );
    ret
}

/// Issue a 3-argument syscall.
#[inline(always)]
unsafe fn syscall3(nr: u64, a1: u64, a2: u64, a3: u64) -> i64 {
    syscall6(nr, a1, a2, a3, 0, 0, 0)
}

/// Issue a 2-argument syscall.
#[inline(always)]
unsafe fn syscall2(nr: u64, a1: u64, a2: u64) -> i64 {
    syscall6(nr, a1, a2, 0, 0, 0, 0)
}

/// Issue a 1-argument syscall.
#[inline(always)]
unsafe fn syscall1(nr: u64, a1: u64) -> i64 {
    syscall6(nr, a1, 0, 0, 0, 0, 0)
}

// =============================================================================
// mmap / munmap
// =============================================================================

/// mmap protection flags
pub const PROT_READ: i32 = 0x1;
pub const PROT_WRITE: i32 = 0x2;

/// mmap flags
pub const MAP_SHARED: i32 = 0x01;
pub const MAP_PRIVATE: i32 = 0x02;
pub const MAP_ANONYMOUS: i32 = 0x20;
pub const MAP_POPULATE: i32 = 0x08000;
pub const MAP_FAILED: *mut u8 = usize::MAX as *mut u8;

/// mmap(addr, length, prot, flags, fd, offset)
pub unsafe fn mmap(
    addr: *mut u8,
    length: u64,
    prot: i32,
    flags: i32,
    fd: i32,
    offset: i64,
) -> Result<*mut u8, i32> {
    let ret = syscall6(
        SYS_MMAP,
        addr as u64,
        length,
        prot as u64,
        flags as u64,
        fd as u64,
        offset as u64,
    );
    if ret < 0 {
        Err((-ret) as i32)
    } else {
        Ok(ret as *mut u8)
    }
}

/// munmap(addr, length)
pub unsafe fn munmap(addr: *mut u8, length: u64) -> Result<(), i32> {
    let ret = syscall2(SYS_MUNMAP, addr as u64, length);
    if ret < 0 {
        Err((-ret) as i32)
    } else {
        Ok(())
    }
}

// =============================================================================
// socket / bind / close
// =============================================================================

/// Socket types
pub const SOCK_RAW: i32 = 3;

/// socket(domain, type, protocol)
pub unsafe fn socket(domain: i32, stype: i32, protocol: i32) -> Result<i32, i32> {
    let ret = syscall3(SYS_SOCKET, domain as u64, stype as u64, protocol as u64);
    if ret < 0 {
        Err((-ret) as i32)
    } else {
        Ok(ret as i32)
    }
}

/// bind(sockfd, addr, addrlen)
pub unsafe fn bind(sockfd: i32, addr: *const u8, addrlen: u32) -> Result<(), i32> {
    let ret = syscall3(SYS_BIND, sockfd as u64, addr as u64, addrlen as u64);
    if ret < 0 {
        Err((-ret) as i32)
    } else {
        Ok(())
    }
}

/// close(fd)
pub unsafe fn close(fd: i32) -> Result<(), i32> {
    let ret = syscall1(SYS_CLOSE, fd as u64);
    if ret < 0 {
        Err((-ret) as i32)
    } else {
        Ok(())
    }
}

// =============================================================================
// setsockopt / getsockopt
// =============================================================================

/// setsockopt(sockfd, level, optname, optval, optlen)
pub unsafe fn setsockopt(
    sockfd: i32,
    level: i32,
    optname: i32,
    optval: *const u8,
    optlen: u32,
) -> Result<(), i32> {
    let ret = syscall6(
        SYS_SETSOCKOPT,
        sockfd as u64,
        level as u64,
        optname as u64,
        optval as u64,
        optlen as u64,
        0,
    );
    if ret < 0 {
        Err((-ret) as i32)
    } else {
        Ok(())
    }
}

/// getsockopt(sockfd, level, optname, optval, optlen)
pub unsafe fn getsockopt(
    sockfd: i32,
    level: i32,
    optname: i32,
    optval: *mut u8,
    optlen: *mut u32,
) -> Result<(), i32> {
    let ret = syscall6(
        SYS_GETSOCKOPT,
        sockfd as u64,
        level as u64,
        optname as u64,
        optval as u64,
        optlen as u64,
        0,
    );
    if ret < 0 {
        Err((-ret) as i32)
    } else {
        Ok(())
    }
}

// =============================================================================
// sendto (for TX ring kick)
// =============================================================================

/// MSG_DONTWAIT flag
pub const MSG_DONTWAIT: i32 = 0x40;

/// sendto(sockfd, buf, len, flags, dest_addr, addrlen) — zero-length kick
pub unsafe fn sendto(
    sockfd: i32,
    buf: *const u8,
    len: u64,
    flags: i32,
    dest_addr: *const u8,
    addrlen: u32,
) -> Result<i64, i32> {
    let ret = syscall6(
        SYS_SENDTO,
        sockfd as u64,
        buf as u64,
        len,
        flags as u64,
        dest_addr as u64,
        addrlen as u64,
    );
    if ret < 0 {
        Err((-ret) as i32)
    } else {
        Ok(ret)
    }
}

// =============================================================================
// poll (for need-wakeup check)
// =============================================================================

/// Poll event flags
pub const POLLIN: i16 = 0x0001;
pub const POLLOUT: i16 = 0x0004;

/// pollfd structure
#[repr(C)]
#[derive(Copy, Clone, Debug, Default)]
pub struct PollFd {
    pub fd: i32,
    pub events: i16,
    pub revents: i16,
}

/// poll(fds, nfds, timeout_ms)
pub unsafe fn poll(fds: *mut PollFd, nfds: u64, timeout_ms: i32) -> Result<i32, i32> {
    let ret = syscall3(SYS_POLL, fds as u64, nfds, timeout_ms as u64);
    if ret < 0 {
        Err((-ret) as i32)
    } else {
        Ok(ret as i32)
    }
}

// =============================================================================
// if_nametoindex (via ioctl SIOCGIFINDEX)
// =============================================================================

const SYS_IOCTL: u64 = 16;
const SIOCGIFINDEX: u64 = 0x8933;

/// ifreq structure (simplified for SIOCGIFINDEX)
#[repr(C)]
pub struct Ifreq {
    pub ifr_name: [u8; 16],
    pub ifr_ifindex: i32,
    _pad: [u8; 20],
}

/// Get interface index by name (equivalent to if_nametoindex).
/// Uses ioctl(SIOCGIFINDEX) on a temporary socket.
pub unsafe fn if_nametoindex(name: &str) -> Result<u32, i32> {
    // Create a temporary UDP socket for the ioctl
    let sock = socket(2 /* AF_INET */, 2 /* SOCK_DGRAM */, 0)?;

    let mut req: Ifreq = std::mem::zeroed();
    let name_bytes = name.as_bytes();
    let copy_len = name_bytes.len().min(15);
    req.ifr_name[..copy_len].copy_from_slice(&name_bytes[..copy_len]);
    // Already null-terminated since zeroed

    let ret = syscall3(
        SYS_IOCTL,
        sock as u64,
        SIOCGIFINDEX,
        &mut req as *mut Ifreq as u64,
    );
    let _ = close(sock);

    if ret < 0 {
        Err((-ret) as i32)
    } else {
        Ok(req.ifr_ifindex as u32)
    }
}

// =============================================================================
// Memory barrier (compiler fence for ring buffer access)
// =============================================================================

/// Compiler fence — ensures memory ordering for ring buffer operations.
/// On x86_64, the hardware provides TSO (Total Store Order), so a compiler
/// fence is sufficient for producer/consumer ring synchronization.
#[inline(always)]
pub fn smp_mb() {
    std::sync::atomic::fence(std::sync::atomic::Ordering::SeqCst);
}

/// Load-acquire barrier for reading producer/consumer indices.
#[inline(always)]
pub fn smp_rmb() {
    std::sync::atomic::fence(std::sync::atomic::Ordering::Acquire);
}

/// Store-release barrier for writing producer/consumer indices.
#[inline(always)]
pub fn smp_wmb() {
    std::sync::atomic::fence(std::sync::atomic::Ordering::Release);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_pollfd_size() {
        assert_eq!(std::mem::size_of::<PollFd>(), 8);
    }

    #[test]
    fn test_ifreq_name_fits() {
        let mut req: Ifreq = unsafe { std::mem::zeroed() };
        let name = b"lo";
        req.ifr_name[..name.len()].copy_from_slice(name);
        assert_eq!(&req.ifr_name[..2], b"lo");
        assert_eq!(req.ifr_name[2], 0); // null terminated
    }

    #[test]
    fn test_mmap_anonymous() {
        // Test anonymous mmap (doesn't need privileges)
        let size = 4096u64;
        let result = unsafe {
            mmap(
                std::ptr::null_mut(),
                size,
                PROT_READ | PROT_WRITE,
                MAP_PRIVATE | MAP_ANONYMOUS,
                -1,
                0,
            )
        };
        assert!(result.is_ok(), "anonymous mmap should succeed");
        let ptr = result.unwrap();
        assert!(!ptr.is_null());

        // Write and read back
        unsafe {
            *ptr = 42;
            assert_eq!(*ptr, 42);
        }

        // Cleanup
        let unmap = unsafe { munmap(ptr, size) };
        assert!(unmap.is_ok(), "munmap should succeed");
    }

    #[test]
    fn test_if_nametoindex_loopback() {
        let result = unsafe { if_nametoindex("lo") };
        assert!(result.is_ok(), "loopback interface should exist");
        assert!(result.unwrap() > 0, "loopback ifindex should be > 0");
    }

    #[test]
    fn test_if_nametoindex_nonexistent() {
        let result = unsafe { if_nametoindex("nonexistent_if_xyz") };
        assert!(result.is_err(), "nonexistent interface should fail");
    }

    #[test]
    fn test_socket_invalid_domain() {
        let result = unsafe { socket(9999, SOCK_RAW, 0) };
        assert!(result.is_err(), "invalid domain should fail");
    }
}
