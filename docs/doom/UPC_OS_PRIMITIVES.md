# UPC OS Primitives -- Level 4 Implementation Summary

*SPDX-License-Identifier: GPL-3.0-or-later*

**Date:** 2026-03-15
**Status:** All 6 primitives implemented and tested
**Spec:** `docs/doom/ROAD_TO_LINUX.md`

---

## Overview

The UPC (Unheaded Protocol Computer) OS primitives are the Level 4 building
blocks required to run an operating system on the Monad Bytecode CPU (MBC).
All 6 primitives were implemented on 2026-03-15 and validated with a 37-test
integration suite (1,170 lines of Rust tests).

```
Level 4a: Timer Interrupts     ← 5479b27
Level 4b: Syscall Dispatch     ← c333236
Level 4c: Basic Scheduler      ← 7e0ded6
Level 4d: MMU/Paging           ← dc0e826
Level 4e: Block Device         ← 0d5a172
Level 4f: Console I/O          ← c333236
```

---

## Level 4a: Timer Interrupts

**Commit:** `5479b27` feat(mbc-cpu): implement Level 4a timer interrupt emulation

**Design:** BPF timer callbacks fire at ~100Hz, setting `interrupt_pending` and
`interrupt_vector` in the CPU state. The execution loop checks after each
instruction and vectors through the IVT (Interrupt Vector Table).

**CPU State Fields:**
- `interrupt_pending: u8` (offset 104) -- flag: interrupt waiting
- `interrupt_vector: u8` (offset 105) -- vector number to dispatch
- `interrupts_enabled: u8` (offset 106) -- CLI/STI flag
- `tick_counter: u32` (offset 108) -- monotonic tick count

**IVT Layout** (at RAM address 0x0000):

| Vector | Offset | Source |
|--------|--------|--------|
| 0x00-0x1F | 0x000-0x07C | CPU exceptions (div-by-zero, invalid opcode) |
| 0x20 | 0x080 | Timer interrupt (BPF timer, ~100Hz) |
| 0x21 | 0x084 | Keyboard/input (Busboy message arrival) |
| 0x22 | 0x088 | Disk complete (Wotan DMA) |
| 0x80 | 0x200 | Syscall (INT 0x80 instruction) |

**Interrupt flow:** save flags -> push PC -> push flags -> disable interrupts -> jump to IVT[vector]

**IRET instruction** (opcode 0x18): pop flags -> pop PC -> re-enable interrupts

---

## Level 4b: Syscall Dispatch

**Commit:** `c333236` feat(mbc): implement Level 4b syscall dispatch and Level 4f console I/O

**Design:** `INT 0x80` triggers the syscall handler. Register r0 holds the syscall
number, r1-r3 hold arguments. Return value goes back into r0 (negative = errno).

**Syscall Numbers** (Linux-compatible, defined in `ebpf/monad-common/src/lib.rs`):

| Number | Name | Signature | Level |
|--------|------|-----------|-------|
| 1 | SYS_EXIT | `exit(code)` | 4b |
| 2 | SYS_FORK | `fork()` | 4c |
| 3 | SYS_READ | `read(fd, buf, len)` | 5b |
| 4 | SYS_WRITE | `write(fd, buf, len)` | 4b/4f |
| 5 | SYS_OPEN | `open(path, flags)` | 5b |
| 6 | SYS_CLOSE | `close(fd)` | 5b |
| 7 | SYS_WAITPID | `waitpid(pid, status)` | 5b |
| 11 | SYS_EXECVE | `execve(path, argv, envp)` | 5 |
| 20 | SYS_GETPID | `getpid()` | 4b |
| 45 | SYS_BRK | `brk(addr)` | 4b |
| 54 | SYS_IOCTL | `ioctl(fd, cmd)` | 5 |
| 90 | SYS_MMAP | `mmap(addr, len, ...)` | 4d |
| 91 | SYS_MUNMAP | `munmap(addr, len)` | 4d |
| 120 | SYS_CLONE | `clone(flags, stack)` | 5 |
| 122 | SYS_UNAME | `uname(buf)` | 5 |
| 146 | SYS_WRITEV | `writev(fd, iov, iovcnt)` | 5 |
| 158 | SYS_SCHED_YIELD | `sched_yield()` | 4c |
| 162 | SYS_NANOSLEEP | `nanosleep(req, rem)` | 5 |
| 200 | SYS_READ_BLOCK | `read_block(blk, buf)` | 4e (custom) |
| 201 | SYS_WRITE_BLOCK | `write_block(blk, buf)` | 4e (custom) |
| 250 | SYS_SET_PAGE_DIR | `set_page_dir(addr)` | 4d (custom) |
| 251 | SYS_ENABLE_MMU | `enable_mmu()` | 4d (custom) |
| 252 | SYS_FLUSH_TLB | `flush_tlb()` | 4d (custom) |
| 265 | SYS_CLOCK_GETTIME | `clock_gettime(clk, tp)` | 4b |

**INT 0x80 dispatch:** IVT offset = 0x80 * 4 = 0x200. The handler at that address
reads r0 and dispatches through a match table in `crates/monad-mbc/src/execute.rs`.

---

## Level 4c: Basic Scheduler

**Commit:** `7e0ded6` feat(mbc): implement Level 4c basic round-robin scheduler

**Design:** Round-robin scheduler supporting up to 4 processes. Timer interrupt
(0x20) triggers context switch. Each process has a saved register set and PC.

**CPU State Fields:**
- `current_pid: u8` (offset 120) -- active process ID (0-3)
- `num_processes: u8` (offset 121) -- count of live processes

**Process Table:** Stored in BPF map. Each entry contains:
- Saved registers (r0-r15)
- Saved PC
- Saved flags
- Process state (running / ready / blocked / zombie)

**Scheduling algorithm:**
1. Timer fires -> INT 0x20
2. Save current process context to process table
3. Advance `current_pid` round-robin (skip non-ready processes)
4. Restore new process context
5. IRET to new process

**SYS_FORK:** Copies current process state into next free slot, returns child
PID to parent (r0), 0 to child.

**SYS_SCHED_YIELD:** Voluntarily triggers a context switch without waiting for timer.

---

## Level 4d: MMU/Paging Emulation

**Commit:** `dc0e826` feat(upc): implement Level 4d MMU/paging emulation with software TLB

**Design:** Software two-level page table walk with a 64-entry direct-mapped TLB
cache. Every LOAD/STORE goes through address translation when MMU is enabled.

**Virtual Address Layout (32-bit):**
```
[31:22] = Page Directory Index  (10 bits, 1024 entries)
[21:12] = Page Table Index      (10 bits, 1024 entries)
[11:0]  = Page Offset           (12 bits, 4KB pages)
```

**Page Table Entry format:**
```
[31:12] = Physical page frame number
[11]    = Present
[10]    = Read/Write
[9]     = User/Supervisor
[8]     = Accessed
[7]     = Dirty
[6:0]   = Reserved
```

**CPU State Fields:**
- `mmu_enabled: u8` (offset 122) -- 0 = physical addressing, 1 = virtual
- `page_dir_base: u32` (offset 124) -- physical address of page directory

**TLB (Software):**
- 64 entries, direct-mapped by `(virtual_page_number & 63)`
- Each entry: `{ virtual_page, physical_page, flags, valid }`
- TLB hit: ~100-200ns per memory access
- TLB miss (page table walk): ~400-600ns (2 BPF map lookups)
- Flushed via `SYS_FLUSH_TLB` (syscall 252) or on page directory change

**Custom syscalls:**
- `SYS_SET_PAGE_DIR (250)` -- set page directory base address
- `SYS_ENABLE_MMU (251)` -- enable virtual addressing
- `SYS_FLUSH_TLB (252)` -- invalidate all TLB entries

**Implementation:** `crates/monad-mbc/src/execute.rs` -- `translate_address()` and `flush_tlb()`

---

## Level 4e: Block Device

**Commit:** `0d5a172` feat(mbc): implement Level 4e block device emulation (ramdisk)

**Design:** 4MB ramdisk in Wotan extended memory with 512-byte blocks.
Accessed via custom syscalls (Linux block device interface is too complex
for BPF; these are simplified DMA-style calls).

**Parameters** (defined in `ebpf/monad-common/src/lib.rs` as `mbc_block`):

| Constant | Value | Description |
|----------|-------|-------------|
| `RAMDISK_BASE` | 0x100000 | Start address in Wotan memory |
| `RAMDISK_SIZE` | 4,194,304 (4MB) | Total ramdisk capacity |
| `BLOCK_SIZE` | 512 | Bytes per block |
| `BLOCK_COUNT` | 8,192 | Total blocks (4MB / 512) |

**Syscalls:**
- `SYS_READ_BLOCK (200)` -- `read_block(block_num, buf_addr) -> bytes_read`
- `SYS_WRITE_BLOCK (201)` -- `write_block(block_num, buf_addr) -> bytes_written`

**Error handling:** Returns `-EIO` if block number is out of range.

---

## Level 4f: Console I/O

**Commit:** `c333236` feat(mbc): implement Level 4b syscall dispatch and Level 4f console I/O

**Design:** Maps `SYS_WRITE` (syscall 4) to fd 1 (stdout) and fd 2 (stderr)
onto a TTY output buffer. Characters written are published to the Busboy
topic `compute.tty.{label}`.

**I/O Addresses (memory-mapped):**

| Address | Register | Direction |
|---------|----------|-----------|
| 0xC001 | TTY data output | Write: byte -> publish to tty topic |
| 0xC002 | TTY status | Read: buffer empty/full flags |
| 0xC003 | TTY control | Write: line discipline config |
| 0xFFFF | Input register | Read: keyboard/input byte from Busboy |

**SYS_WRITE to stdout/stderr:**
1. Read `len` bytes from buffer at `buf_addr` in RAM
2. Append to `tty_output` ring buffer
3. Publish to Busboy `compute.tty.{label}` topic
4. Return bytes written in r0

---

## BPF Map References

The OS primitives interact with several BPF maps. Key maps from
`pkg/protocol/bpfschema/core_maps.go`:

| Map | Type | Key -> Value | Used By |
|-----|------|-------------|---------|
| `cpu_state_map` | Array | u32 -> MbcCpuState (128 bytes) | All primitives |
| `ram_map` | Hash | u32 -> u32 | Memory access, IVT, page tables |
| `rom_map` | Array | u32 -> u32 | Instruction fetch |
| `screen_map` | Array | u32 -> u8 | Framebuffer output |

**MbcCpuState struct size:** 128 bytes (matches Rust `#[repr(C)]` in
`ebpf/monad-common/src/lib.rs` and Go mirror in `internal/doom/types.go`).

---

## Test Coverage

**Test file:** `crates/monad-mbc/tests/os_primitives_test.rs` (1,170 lines, 37 tests)

Tests cover all 6 primitive areas:

| Area | Tests | What's Validated |
|------|-------|-----------------|
| Timer interrupts | IVT setup, INT dispatch, IRET, nested interrupts | Correct PC save/restore, flag handling |
| Syscall dispatch | SYS_EXIT, SYS_WRITE, SYS_BRK, SYS_GETPID, SYS_CLOCK_GETTIME | Return values, errno on error |
| Scheduler | SYS_FORK, context switch, round-robin, SYS_SCHED_YIELD | PID assignment, register isolation |
| MMU | Page table walk, TLB hit/miss, identity mapping, enable/disable | Address translation correctness |
| Block device | Read/write blocks, boundary checks, out-of-range errors | Data integrity, error codes |
| Console I/O | SYS_WRITE to fd 1/2, tty_output capture, multi-write | Output accumulation, byte counts |
| Boot integration | MiniKernel demo (hello_kernel) | Full boot -> init -> output -> exit |

**MiniKernel** (`776dcfa`): Level 5 proof-of-concept that boots through the
full stack -- IVT setup, syscall handler install, MMU identity map, ramdisk
init, console write "Hello from MiniKernel!", then SYS_EXIT.

---

*See also: `docs/doom/ROAD_TO_LINUX.md` (full design spec), `docs/doom/UPC_BOOT_PROTOCOL.md`
(boot sequence), `docs/doom/FUZIX_PORT_ANALYSIS.md` (Level 5b feasibility)*
