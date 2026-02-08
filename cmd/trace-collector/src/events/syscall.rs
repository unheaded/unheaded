//! Syscall event parsing.
//!
//! Handles system call traces from eBPF programs attached to
//! tracepoints or kprobes.

use serde::{Deserialize, Serialize};

use super::EventError;

/// Maximum command name length
const TASK_COMM_LEN: usize = 16;

/// Maximum number of syscall arguments we track
const MAX_ARGS: usize = 6;

/// Syscall category for grouping
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum SyscallCategory {
    /// File operations (open, read, write, close, etc.)
    File,
    /// Process operations (fork, exec, exit, etc.)
    Process,
    /// Network operations (socket, connect, bind, etc.)
    Network,
    /// Memory operations (mmap, brk, etc.)
    Memory,
    /// IPC operations (pipe, shmget, etc.)
    Ipc,
    /// Signal operations
    Signal,
    /// Time operations
    Time,
    /// Other/unknown
    Other,
}

/// Raw syscall event structure from BPF
#[repr(C, packed)]
struct RawSyscallEvent {
    /// Process ID
    pid: u32,
    /// Thread ID
    tid: u32,
    /// Syscall number
    syscall_nr: u32,
    /// Entry or exit (0=entry, 1=exit)
    is_exit: u8,
    /// Padding
    _pad: [u8; 3],
    /// Return value (only valid on exit)
    ret: i64,
    /// Syscall arguments
    args: [u64; MAX_ARGS],
    /// Duration in nanoseconds (only valid on exit)
    duration_ns: u64,
    /// Command name
    comm: [u8; TASK_COMM_LEN],
}

/// Parsed syscall event
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SyscallEvent {
    /// Process ID
    pub pid: u32,
    /// Thread ID
    pub tid: u32,
    /// Command name
    pub comm: String,
    /// Syscall number
    pub syscall_nr: u32,
    /// Syscall name (resolved)
    pub syscall_name: String,
    /// Syscall category
    pub category: SyscallCategory,
    /// Whether this is an exit event
    pub is_exit: bool,
    /// Return value (for exit events)
    pub ret: Option<i64>,
    /// Syscall arguments
    pub args: Vec<u64>,
    /// Duration in nanoseconds (for exit events)
    pub duration_ns: Option<u64>,
    /// Error name (if ret < 0)
    pub error: Option<String>,
}

impl SyscallEvent {
    /// Parse syscall event from raw bytes
    pub fn from_bytes(data: &[u8]) -> Result<Self, EventError> {
        let size = std::mem::size_of::<RawSyscallEvent>();
        if data.len() < size {
            return Err(EventError::InsufficientData {
                expected: size,
                actual: data.len(),
            });
        }

        // SAFETY: We've verified the length
        let raw: RawSyscallEvent =
            unsafe { std::ptr::read_unaligned(data.as_ptr() as *const RawSyscallEvent) };

        // Copy fields from packed struct to avoid unaligned references
        let pid = raw.pid;
        let tid = raw.tid;
        let syscall_nr = raw.syscall_nr;
        let is_exit_byte = raw.is_exit;
        let ret_val = raw.ret;
        let duration = raw.duration_ns;
        let args = raw.args;
        let comm_bytes = raw.comm;

        let comm = parse_comm(&comm_bytes);
        let syscall_name = syscall_name(syscall_nr);
        let category = syscall_category(syscall_nr);

        let is_exit = is_exit_byte != 0;
        let (ret, duration_ns, error) = if is_exit {
            let error = if ret_val < 0 {
                Some(errno_name(-ret_val as i32))
            } else {
                None
            };
            (Some(ret_val), Some(duration), error)
        } else {
            (None, None, None)
        };

        Ok(Self {
            pid,
            tid,
            comm,
            syscall_nr,
            syscall_name,
            category,
            is_exit,
            ret,
            args: args.to_vec(),
            duration_ns,
            error,
        })
    }

    /// Check if the syscall failed
    pub fn failed(&self) -> bool {
        self.ret.map(|r| r < 0).unwrap_or(false)
    }

    /// Get latency in microseconds
    pub fn latency_us(&self) -> Option<f64> {
        self.duration_ns.map(|ns| ns as f64 / 1000.0)
    }
}

/// Parse null-terminated command name
fn parse_comm(comm: &[u8; TASK_COMM_LEN]) -> String {
    let end = comm.iter().position(|&c| c == 0).unwrap_or(TASK_COMM_LEN);
    String::from_utf8_lossy(&comm[..end]).to_string()
}

/// Map syscall number to name (x86_64)
fn syscall_name(nr: u32) -> String {
    // Common syscalls on x86_64
    match nr {
        0 => "read".to_string(),
        1 => "write".to_string(),
        2 => "open".to_string(),
        3 => "close".to_string(),
        4 => "stat".to_string(),
        5 => "fstat".to_string(),
        6 => "lstat".to_string(),
        7 => "poll".to_string(),
        8 => "lseek".to_string(),
        9 => "mmap".to_string(),
        10 => "mprotect".to_string(),
        11 => "munmap".to_string(),
        12 => "brk".to_string(),
        13 => "rt_sigaction".to_string(),
        14 => "rt_sigprocmask".to_string(),
        15 => "rt_sigreturn".to_string(),
        16 => "ioctl".to_string(),
        17 => "pread64".to_string(),
        18 => "pwrite64".to_string(),
        19 => "readv".to_string(),
        20 => "writev".to_string(),
        21 => "access".to_string(),
        22 => "pipe".to_string(),
        23 => "select".to_string(),
        24 => "sched_yield".to_string(),
        25 => "mremap".to_string(),
        26 => "msync".to_string(),
        27 => "mincore".to_string(),
        28 => "madvise".to_string(),
        29 => "shmget".to_string(),
        30 => "shmat".to_string(),
        31 => "shmctl".to_string(),
        32 => "dup".to_string(),
        33 => "dup2".to_string(),
        34 => "pause".to_string(),
        35 => "nanosleep".to_string(),
        36 => "getitimer".to_string(),
        37 => "alarm".to_string(),
        38 => "setitimer".to_string(),
        39 => "getpid".to_string(),
        40 => "sendfile".to_string(),
        41 => "socket".to_string(),
        42 => "connect".to_string(),
        43 => "accept".to_string(),
        44 => "sendto".to_string(),
        45 => "recvfrom".to_string(),
        46 => "sendmsg".to_string(),
        47 => "recvmsg".to_string(),
        48 => "shutdown".to_string(),
        49 => "bind".to_string(),
        50 => "listen".to_string(),
        51 => "getsockname".to_string(),
        52 => "getpeername".to_string(),
        53 => "socketpair".to_string(),
        54 => "setsockopt".to_string(),
        55 => "getsockopt".to_string(),
        56 => "clone".to_string(),
        57 => "fork".to_string(),
        58 => "vfork".to_string(),
        59 => "execve".to_string(),
        60 => "exit".to_string(),
        61 => "wait4".to_string(),
        62 => "kill".to_string(),
        63 => "uname".to_string(),
        72 => "fcntl".to_string(),
        73 => "flock".to_string(),
        74 => "fsync".to_string(),
        75 => "fdatasync".to_string(),
        76 => "truncate".to_string(),
        77 => "ftruncate".to_string(),
        78 => "getdents".to_string(),
        79 => "getcwd".to_string(),
        80 => "chdir".to_string(),
        81 => "fchdir".to_string(),
        82 => "rename".to_string(),
        83 => "mkdir".to_string(),
        84 => "rmdir".to_string(),
        85 => "creat".to_string(),
        86 => "link".to_string(),
        87 => "unlink".to_string(),
        88 => "symlink".to_string(),
        89 => "readlink".to_string(),
        90 => "chmod".to_string(),
        91 => "fchmod".to_string(),
        92 => "chown".to_string(),
        93 => "fchown".to_string(),
        94 => "lchown".to_string(),
        95 => "umask".to_string(),
        96 => "gettimeofday".to_string(),
        97 => "getrlimit".to_string(),
        98 => "getrusage".to_string(),
        99 => "sysinfo".to_string(),
        100 => "times".to_string(),
        101 => "ptrace".to_string(),
        102 => "getuid".to_string(),
        103 => "syslog".to_string(),
        104 => "getgid".to_string(),
        105 => "setuid".to_string(),
        106 => "setgid".to_string(),
        107 => "geteuid".to_string(),
        108 => "getegid".to_string(),
        109 => "setpgid".to_string(),
        110 => "getppid".to_string(),
        111 => "getpgrp".to_string(),
        112 => "setsid".to_string(),
        113 => "setreuid".to_string(),
        114 => "setregid".to_string(),
        115 => "getgroups".to_string(),
        116 => "setgroups".to_string(),
        117 => "setresuid".to_string(),
        118 => "getresuid".to_string(),
        119 => "setresgid".to_string(),
        120 => "getresgid".to_string(),
        121 => "getpgid".to_string(),
        122 => "setfsuid".to_string(),
        123 => "setfsgid".to_string(),
        124 => "getsid".to_string(),
        125 => "capget".to_string(),
        126 => "capset".to_string(),
        186 => "gettid".to_string(),
        202 => "futex".to_string(),
        217 => "getdents64".to_string(),
        231 => "exit_group".to_string(),
        232 => "epoll_wait".to_string(),
        233 => "epoll_ctl".to_string(),
        257 => "openat".to_string(),
        262 => "newfstatat".to_string(),
        263 => "unlinkat".to_string(),
        265 => "linkat".to_string(),
        266 => "symlinkat".to_string(),
        267 => "readlinkat".to_string(),
        268 => "fchmodat".to_string(),
        269 => "faccessat".to_string(),
        270 => "pselect6".to_string(),
        271 => "ppoll".to_string(),
        280 => "utimensat".to_string(),
        281 => "epoll_pwait".to_string(),
        288 => "accept4".to_string(),
        290 => "eventfd2".to_string(),
        291 => "epoll_create1".to_string(),
        292 => "dup3".to_string(),
        293 => "pipe2".to_string(),
        318 => "getrandom".to_string(),
        319 => "memfd_create".to_string(),
        332 => "statx".to_string(),
        435 => "clone3".to_string(),
        _ => format!("syscall_{}", nr),
    }
}

/// Categorize syscall by number
fn syscall_category(nr: u32) -> SyscallCategory {
    match nr {
        // Signal operations (checked first to avoid overlap with File)
        13..=15 | 34 => SyscallCategory::Signal,
        // Process operations (checked before File for 56-62 range)
        56..=61 | 101 | 186 | 231 | 435 => SyscallCategory::Process,
        // Network operations (checked before IPC for 41-55 range)
        41..=55 | 288 => SyscallCategory::Network,
        // IPC operations
        22 | 29..=31 | 290 | 293 => SyscallCategory::Ipc,
        // Memory operations
        9..=12 | 25..=28 | 202 => SyscallCategory::Memory,
        // Time operations
        35..=38 | 96 | 100 => SyscallCategory::Time,
        // File operations
        0..=8 | 17..=21 | 32..=33 | 40 | 62 | 72..=95 | 257..=269 | 332 => SyscallCategory::File,
        // Other
        _ => SyscallCategory::Other,
    }
}

/// Map errno to name
fn errno_name(errno: i32) -> String {
    match errno {
        1 => "EPERM".to_string(),
        2 => "ENOENT".to_string(),
        3 => "ESRCH".to_string(),
        4 => "EINTR".to_string(),
        5 => "EIO".to_string(),
        6 => "ENXIO".to_string(),
        7 => "E2BIG".to_string(),
        8 => "ENOEXEC".to_string(),
        9 => "EBADF".to_string(),
        10 => "ECHILD".to_string(),
        11 => "EAGAIN".to_string(),
        12 => "ENOMEM".to_string(),
        13 => "EACCES".to_string(),
        14 => "EFAULT".to_string(),
        15 => "ENOTBLK".to_string(),
        16 => "EBUSY".to_string(),
        17 => "EEXIST".to_string(),
        18 => "EXDEV".to_string(),
        19 => "ENODEV".to_string(),
        20 => "ENOTDIR".to_string(),
        21 => "EISDIR".to_string(),
        22 => "EINVAL".to_string(),
        23 => "ENFILE".to_string(),
        24 => "EMFILE".to_string(),
        25 => "ENOTTY".to_string(),
        26 => "ETXTBSY".to_string(),
        27 => "EFBIG".to_string(),
        28 => "ENOSPC".to_string(),
        29 => "ESPIPE".to_string(),
        30 => "EROFS".to_string(),
        31 => "EMLINK".to_string(),
        32 => "EPIPE".to_string(),
        33 => "EDOM".to_string(),
        34 => "ERANGE".to_string(),
        35 => "EDEADLK".to_string(),
        36 => "ENAMETOOLONG".to_string(),
        37 => "ENOLCK".to_string(),
        38 => "ENOSYS".to_string(),
        39 => "ENOTEMPTY".to_string(),
        40 => "ELOOP".to_string(),
        98 => "EADDRINUSE".to_string(),
        99 => "EADDRNOTAVAIL".to_string(),
        100 => "ENETDOWN".to_string(),
        101 => "ENETUNREACH".to_string(),
        102 => "ENETRESET".to_string(),
        103 => "ECONNABORTED".to_string(),
        104 => "ECONNRESET".to_string(),
        105 => "ENOBUFS".to_string(),
        106 => "EISCONN".to_string(),
        107 => "ENOTCONN".to_string(),
        110 => "ETIMEDOUT".to_string(),
        111 => "ECONNREFUSED".to_string(),
        112 => "EHOSTDOWN".to_string(),
        113 => "EHOSTUNREACH".to_string(),
        114 => "EALREADY".to_string(),
        115 => "EINPROGRESS".to_string(),
        _ => format!("ERRNO_{}", errno),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_syscall_name() {
        assert_eq!(syscall_name(0), "read");
        assert_eq!(syscall_name(1), "write");
        assert_eq!(syscall_name(59), "execve");
        assert_eq!(syscall_name(9999), "syscall_9999");
    }

    #[test]
    fn test_syscall_category() {
        assert_eq!(syscall_category(0), SyscallCategory::File);
        assert_eq!(syscall_category(41), SyscallCategory::Network);
        assert_eq!(syscall_category(57), SyscallCategory::Process);
    }

    #[test]
    fn test_errno_name() {
        assert_eq!(errno_name(2), "ENOENT");
        assert_eq!(errno_name(13), "EACCES");
        assert_eq!(errno_name(111), "ECONNREFUSED");
    }
}
