# FUZIX Port Analysis — UPC Level 5b

**Date:** 2026-03-15
**Status:** Feasibility assessment complete
**Target:** FUZIX on UPC (Monad Bytecode CPU running in eBPF XDP)

---

## 1. Executive Summary

FUZIX is a minimal Unix OS targeting Z80/6502/68000/ARM. It runs in <128KB RAM
(we have 16MB+), uses a simple block device interface (matches our ramdisk),
and requires ~30 core syscalls to boot to a shell. It is the **fastest path to
"Unix on UPC"** — far simpler than xv6 or uClinux.

**Verdict: FEASIBLE.** The UPC already implements 15 of the ~30 required
syscalls. The remaining 15 are mostly stubs or trivial implementations. With
the additions in this sprint (SYS_READ, SYS_OPEN, SYS_CLOSE, SYS_WAITPID),
the minimum viable set for booting FUZIX to a shell prompt is nearly complete.

---

## 2. FUZIX Syscall Table (Complete — 80 entries)

FUZIX uses its own syscall numbering (0-79), NOT Linux i386 numbers. A thin
translation layer in the FUZIX platform port maps FUZIX syscall numbers to the
host's INT 0x80 dispatch. The UPC uses Linux i386 numbers internally.

| # | FUZIX Syscall | Linux Equiv | UPC Status | Effort | Notes |
|---|--------------|-------------|------------|--------|-------|
| 0 | `_exit` | exit(1) | **IMPLEMENTED** | - | SYS_EXIT halts CPU with exit code |
| 1 | `open` | open(5) | **IMPLEMENTED** | - | Stub: returns fd=3 (Level 5b) |
| 2 | `close` | close(6) | **IMPLEMENTED** | - | No-op, returns 0 (Level 5b) |
| 3 | `rename` | rename(38) | MISSING | trivial | Stub returning -ENOSYS until FS exists |
| 4 | `mknod` | mknod(14) | MISSING | trivial | Stub returning -ENOSYS until FS exists |
| 5 | `link` | link(9) | MISSING | trivial | Stub returning -ENOSYS |
| 6 | `unlink` | unlink(10) | MISSING | trivial | Stub returning -ENOSYS |
| 7 | `read` | read(3) | **IMPLEMENTED** | - | fd=0 reads from KBD_MAP (Level 5b) |
| 8 | `write` | write(4) | **IMPLEMENTED** | - | fd=1/2 writes to TTY_MAP |
| 9 | `_lseek` | lseek(19) | MISSING | moderate | Needs per-fd offset tracking |
| 10 | `chdir` | chdir(12) | MISSING | trivial | Stub (no FS yet) |
| 11 | `sync` | sync(36) | MISSING | trivial | No-op (ramdisk is always synced) |
| 12 | `access` | access(33) | MISSING | trivial | Always return 0 (success) |
| 13 | `chmod` | chmod(15) | MISSING | trivial | No-op stub |
| 14 | `chown` | chown(182) | MISSING | trivial | No-op stub |
| 15 | `_stat` | stat(106) | MISSING | moderate | Needs fake stat struct |
| 16 | `_fstat` | fstat(108) | MISSING | moderate | Needs fake stat struct for fd |
| 17 | `dup` | dup(41) | MISSING | trivial | Return next available fd |
| 18 | `getpid` | getpid(20) | **IMPLEMENTED** | - | Returns instance_id |
| 19 | `getppid` | getppid(64) | MISSING | trivial | Return 0 (init is parent of all) |
| 20 | `getuid` | getuid(24) | MISSING | trivial | Return 0 (root) |
| 21 | `umask` | umask(60) | MISSING | trivial | Store/return mask value |
| 22 | `_statfs` | statfs(99) | MISSING | moderate | Fake statfs for ramdisk |
| 23 | `execve` | execve(11) | MISSING | **complex** | Load binary from ramdisk, reset process |
| 24 | `_getdirent` | getdents(141) | MISSING | moderate | Needs directory iteration |
| 25 | `setuid` | setuid(23) | MISSING | trivial | No-op (single user) |
| 26 | `setgid` | setgid(46) | MISSING | trivial | No-op |
| 27 | `_time` | time(13) | **PARTIALLY** | trivial | Use CLOCK_GETTIME |
| 28 | `_stime` | stime(25) | MISSING | trivial | No-op |
| 29 | `ioctl` | ioctl(54) | MISSING | moderate | TTY ioctls needed for shell |
| 30 | `brk` | brk(45) | **IMPLEMENTED** | - | Set/query program break |
| 31 | `sbrk` | sbrk(n/a) | **PARTIALLY** | trivial | Derive from brk |
| 32 | `_fork` | fork(2) | **IMPLEMENTED** | - | Copy registers to PROC_TABLE slot |
| 33 | `mount` | mount(21) | MISSING | trivial | Stub |
| 34 | `_umount` | umount(22) | MISSING | trivial | Stub |
| 35 | `signal` | signal(48) | MISSING | moderate | Signal handler registration |
| 36 | `dup2` | dup2(63) | MISSING | trivial | fd aliasing |
| 37 | `_pause` | pause(29) | MISSING | trivial | Sleep until signal (yield loop) |
| 38 | `_alarm` | alarm(27) | MISSING | moderate | Timer-based signal delivery |
| 39 | `kill` | kill(37) | MISSING | moderate | Signal delivery to process |
| 40 | `pipe` | pipe(42) | MISSING | moderate | In-memory pipe buffer |
| 41 | `getgid` | getgid(47) | MISSING | trivial | Return 0 |
| 42 | `times` | times(43) | MISSING | trivial | Return tick counts |
| 43 | `utime` | utime(30) | MISSING | trivial | No-op |
| 44 | `geteuid` | geteuid(49) | MISSING | trivial | Return 0 |
| 45 | `getegid` | getegid(50) | MISSING | trivial | Return 0 |
| 46 | `chroot` | chroot(61) | MISSING | trivial | No-op |
| 47 | `fcntl` | fcntl(55) | MISSING | moderate | fd flags manipulation |
| 48 | `fchdir` | fchdir(133) | MISSING | trivial | No-op |
| 49 | `fchmod` | fchmod(94) | MISSING | trivial | No-op |
| 50 | `fchown` | fchown(95) | MISSING | trivial | No-op |
| 51 | `mkdir` | mkdir(39) | MISSING | trivial | Stub |
| 52 | `rmdir` | rmdir(40) | MISSING | trivial | Stub |
| 53 | `setpgrp` | setpgrp(n/a) | MISSING | trivial | Store value |
| 54 | `_uname` | uname(122) | MISSING | trivial | Return fixed "UPC" string |
| 55 | `waitpid` | waitpid(7) | **IMPLEMENTED** | - | Yield until target halted (Level 5b) |
| 56 | `_profil` | - | MISSING | trivial | No-op |
| 57 | `uadmin` | - | MISSING | trivial | No-op |
| 58 | `nice` | nice(34) | MISSING | trivial | No-op |
| 59 | `_sigdisp` | sigaction(67) | MISSING | moderate | Signal disposition |
| 60 | `flock` | flock(143) | MISSING | trivial | No-op |
| 61 | `getpgrp` | getpgrp(65) | MISSING | trivial | Return 0 |
| 62 | `yield` | sched_yield(158) | **IMPLEMENTED** | - | Context switch via scheduler |

### Additional UPC-specific syscalls (not in FUZIX table but available)

| Linux # | Syscall | Status | Notes |
|---------|---------|--------|-------|
| 162 | SYS_NANOSLEEP | IMPLEMENTED | High-res sleep |
| 265 | SYS_CLOCK_GETTIME | IMPLEMENTED | Clock time |
| 200 | SYS_READ_BLOCK | IMPLEMENTED | Ramdisk block read |
| 201 | SYS_WRITE_BLOCK | IMPLEMENTED | Ramdisk block write |
| 250 | SYS_SET_PAGE_DIR | IMPLEMENTED | MMU page directory |
| 251 | SYS_ENABLE_MMU | IMPLEMENTED | Enable paging |
| 252 | SYS_FLUSH_TLB | IMPLEMENTED | TLB invalidation |

---

## 3. Status Summary

| Status | Count | Details |
|--------|-------|---------|
| **IMPLEMENTED** | 15 | exit, open, close, read, write, getpid, brk, fork, waitpid, yield, nanosleep, clock_gettime, read_block, write_block, MMU (3) |
| **PARTIALLY** | 2 | time (via clock_gettime), sbrk (via brk) |
| **MISSING (trivial)** | 35 | Simple stubs, no-ops, or constant returns |
| **MISSING (moderate)** | 10 | lseek, stat, fstat, statfs, getdirent, ioctl, signal, alarm, kill, pipe, fcntl, sigdisp |
| **MISSING (complex)** | 1 | execve |

---

## 4. Minimum Viable Set to Boot FUZIX to Shell

FUZIX init process boots like this:
1. Kernel starts, mounts root filesystem
2. Calls `execve("/init")` which runs the init program
3. init opens `/dev/console` (open, ioctl for terminal settings)
4. init forks, child execs `/bin/sh`
5. Shell reads commands (read), forks, execs programs (execve), waits (waitpid)

### Critical path syscalls (must work):

| Syscall | Why | Status |
|---------|-----|--------|
| `_exit` | Process termination | DONE |
| `open` | Open files/devices | DONE (stub) |
| `close` | Close fds | DONE (stub) |
| `read` | Read from console/files | DONE |
| `write` | Write to console/files | DONE |
| `brk/sbrk` | Heap allocation | DONE |
| `fork` | Create processes | DONE |
| `waitpid` | Wait for children | DONE |
| `execve` | Load and run programs | **MISSING (complex)** |
| `getpid` | Process identification | DONE |
| `ioctl` | Terminal settings (TCGETS etc) | **MISSING (moderate)** |
| `signal/_sigdisp` | Signal handling (SIGCHLD etc) | **MISSING (moderate)** |
| `dup/dup2` | fd manipulation for shell | **MISSING (trivial)** |
| `pipe` | Shell pipelines | **MISSING (moderate)** |
| `_stat/_fstat` | File existence checks | **MISSING (moderate)** |

### Minimal boot without shell pipelines (13 syscalls):

With the 4 new syscalls added in this sprint (read, open, close, waitpid),
we need **5 more** for the absolute minimum:

1. **execve** (complex) — Load MBC binary from ramdisk block layout, reset
   process registers. This is the single hardest piece.
2. **ioctl** (moderate) — At minimum, return success for TCGETS/TCSETS so the
   shell thinks it has a terminal.
3. **signal** (moderate) — Register signal handlers. Can be simplified: just
   store handler address, deliver on waitpid return.
4. **dup2** (trivial) — Copy fd table entry.
5. **stat/fstat** (moderate) — Return fake stat struct with S_IFCHR for
   console, S_IFREG for files.

### Estimated effort to shell prompt:

| Task | Effort | Sprint estimate |
|------|--------|----------------|
| execve (load from ramdisk) | 2-3 days | Must design binary format |
| ioctl stubs (TTY) | 0.5 day | Return success for key ioctls |
| signal/sigdisp | 1 day | Simplified signal delivery |
| dup/dup2 | 0.5 day | fd table in proc state |
| stat/fstat stubs | 0.5 day | Fake struct population |
| FUZIX kernel adaptation | 3-5 days | Platform port layer |
| **Total** | **~8-10 days** | |

---

## 5. Architecture Notes for FUZIX Platform Port

### Syscall Number Translation

FUZIX uses its own syscall numbers (0-79). The UPC uses Linux i386 numbers.
The FUZIX platform port (`platform-upc/`) must include a translation table:

```c
// fuzix_syscall_nr → linux_i386_syscall_nr
static const uint8_t syscall_map[80] = {
    [0]  = 1,   // _exit → SYS_EXIT
    [1]  = 5,   // open  → SYS_OPEN
    [7]  = 3,   // read  → SYS_READ
    [8]  = 4,   // write → SYS_WRITE
    // ...
};
```

### Memory Layout for FUZIX on UPC

```
0x0000_0000 - 0x0000_03FF  IVT (256 vectors)
0x0000_0400 - 0x0000_7FFF  FUZIX kernel code + data (~30KB)
0x0000_8000 - 0x0000_FFFF  Kernel heap + buffers (32KB)
0x0001_0000 - 0x0001_FFFF  User process space (64KB)
0x0006_8000               KBD_MAP (keyboard input)
0x0007_0000 - 0x0007_F9FF  Screen framebuffer (not used by FUZIX)
0x0020_0000 - 0x005F_FFFF  Ramdisk (4MB, block device)
```

### Block Device Integration

FUZIX uses a simple block device interface: read_block(dev, blk, buf) and
write_block(dev, blk, buf). This maps directly to our SYS_READ_BLOCK /
SYS_WRITE_BLOCK syscalls. The ramdisk holds the FUZIX filesystem image.

### Console Integration

FUZIX console reads from fd 0 (stdin) via SYS_READ, which reads from KBD_MAP.
Console output via SYS_WRITE to fd 1/2 writes to TTY_MAP circular buffer.
This matches the existing UPC TTY infrastructure.

---

## 6. Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| execve complexity | HIGH | Start with flat binary format, no ELF |
| BPF verifier limits on new syscalls | MEDIUM | Keep loops bounded (<256 iterations) |
| FUZIX expects C library | MEDIUM | Cross-compile with FUZIX's own libc |
| Signal delivery timing | LOW | Simplified: deliver on next yield/waitpid |
| Console line editing | LOW | FUZIX has its own line discipline |

---

## 7. Next Steps

1. **This sprint (Level 5b):** Implement SYS_READ, SYS_OPEN, SYS_CLOSE,
   SYS_WAITPID in eBPF engine + userspace mirror. Create compatibility test.
2. **Next sprint (Level 5c):** Implement execve, ioctl stubs, signal stubs.
   Design flat binary format for ramdisk.
3. **Level 5d:** Build FUZIX platform port. Cross-compile FUZIX kernel.
   Create ramdisk filesystem image. Boot test.

---

*SPDX-License-Identifier: GPL-3.0-or-later*
