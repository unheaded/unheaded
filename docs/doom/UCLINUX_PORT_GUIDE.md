# uClinux (nommu) Port Guide -- UPC Level 6a

*SPDX-License-Identifier: GPL-3.0-or-later*

**Date:** 2026-03-15
**Status:** Research complete, implementation not started
**Prerequisite:** Levels 0-5d (all complete)
**Target:** Linux 6.x (CONFIG_MMU=n) booting on UPC/MBC

---

## Table of Contents

1. uClinux Architecture Requirements
2. MBC ISA as a Linux Architecture
3. Syscall ABI Mapping
4. Toolchain Strategy
5. Kernel Config (defconfig)
6. Estimated Effort
7. Alternative: Emulate an Existing nommu Target
8. Decision Matrix
9. Blockers and Risks

---

## 1. uClinux Architecture Requirements

uClinux is mainline Linux compiled with `CONFIG_MMU=n`. It has been in the
kernel tree since Linux 2.6 and is actively maintained. The nommu path removes
virtual memory, page tables, and demand paging -- replacing them with a flat
address model where all processes share one physical address space.

### Hardware Requirements

| Requirement | Minimum | UPC Status |
|------------|---------|------------|
| Address space | 32-bit flat | 32-bit, 64MB addressable via Wotan RAM_MAP |
| RAM | 2MB (practical minimum for kernel + initramfs) | 64MB available |
| Timer interrupt | Periodic, programmable rate | BPF timer at 12-100Hz (configurable) |
| Serial/console | Byte-level I/O | TTY via Wotan 0xC001 + Busboy pub/sub |
| Block device | Optional (initramfs can be built-in) | 4MB ramdisk at 0x800000 |
| Atomics | At least test-and-set or compare-and-swap | NOT IMPLEMENTED -- blocker |
| Memory barriers | At least compiler barriers | NOT IMPLEMENTED -- blocker |
| Byte-addressable memory | Load/store at byte granularity | LDB/STB/LDH/STH opcodes exist |

### What nommu Linux Does NOT Need

- **MMU hardware** -- no page tables, no TLB, no virtual addresses
- **Multiple privilege levels** -- can run entirely in "supervisor" mode
- **Floating point** -- kernel is integer-only (FPU optional for userspace)
- **Cache coherency hardware** -- single-core, no coherency needed
- **DMA controller** -- ramdisk access is synchronous

### Supported CPU Architectures (nommu mode)

The following architectures have `CONFIG_MMU=n` support in mainline Linux:

| Architecture | Status | Relevance to MBC |
|-------------|--------|-------------------|
| ARM (Cortex-M) | Active, well-maintained | Different ISA, but good reference for nommu patterns |
| m68k (ColdFire) | Active | 32-bit, simple, closest conceptual match |
| RISC-V | Experimental nommu support | ISA we already transpile from |
| Xtensa | Active | Used in ESP32, similar constrained-CPU pattern |
| C-SKY | Active | Chinese RISC, 32-bit, nommu native |
| SuperH (sh) | Maintenance mode | 32-bit, simple |
| Microblaze | Active | Soft-core FPGA CPU, very similar "invented ISA" situation |

**Best reference architectures for an MBC port: Microblaze and m68k (ColdFire).**
Both are 32-bit, relatively simple, and have clean nommu implementations.
Microblaze is particularly relevant because it is also a soft-core CPU with a
custom ISA, meaning its Linux port deals with similar "this ISA doesn't exist
in the wild" problems.

### Memory Model Under nommu

With `CONFIG_MMU=n`, Linux uses:

- **Flat memory** -- all addresses are physical
- **No fork()** -- `fork()` returns `-EINVAL`; only `vfork()` + `execve()` works
- **bFLT or ELF-FDPIC binaries** -- position-independent executables with relocation
- **No `mmap()` with `MAP_PRIVATE` copy-on-write** -- shared mappings only
- **`brk()`/`sbrk()` still works** -- heap grows within pre-allocated region
- **Stack is fixed at exec time** -- no dynamic stack growth

This is a fundamental constraint: **Linux userspace programs that rely on
`fork()` semantics will not work.** Busybox and most embedded tools handle
this correctly (they use `vfork()` + `exec()`).

---

## 2. MBC ISA as a Linux Architecture Port

### Required Directory Structure

To add MBC as a Linux architecture, we need `arch/mbc/` in the kernel tree.
Below is the minimum file set, mapped to UPC equivalents.

```
arch/mbc/
├── Kconfig                          # Architecture config options
├── Makefile                         # Build rules
├── include/
│   ├── asm/
│   │   ├── unistd.h                 # Syscall numbers (use Linux generic)
│   │   ├── ptrace.h                 # Register layout for /proc, signals
│   │   ├── page.h                   # PAGE_SIZE = 4096
│   │   ├── io.h                     # readb/writeb/readl/writel macros
│   │   ├── irq.h                    # local_irq_disable/enable
│   │   ├── thread_info.h            # Per-thread kernel stack + flags
│   │   ├── processor.h              # struct thread_struct (saved regs)
│   │   ├── elf.h                    # ELF machine type, ABI
│   │   ├── string.h                 # memcpy/memset (can use generic)
│   │   ├── bitops.h                 # Atomic bit operations -- HARD
│   │   ├── atomic.h                 # atomic_t operations -- HARD
│   │   ├── barrier.h                # mb()/rmb()/wmb()
│   │   ├── cmpxchg.h               # cmpxchg() -- HARD
│   │   ├── switch_to.h             # Context switch macro
│   │   ├── current.h               # "current" task pointer
│   │   └── setup.h                 # Machine-specific setup
│   └── uapi/asm/
│       ├── unistd.h                 # User-visible syscall numbers
│       └── signal.h                 # Signal definitions
├── kernel/
│   ├── entry.S                      # Syscall entry, IRQ entry, ret_from_fork
│   ├── head.S                       # Kernel entry point (first code to run)
│   ├── setup.c                      # Platform init, memory detection
│   ├── process.c                    # copy_thread, context switch
│   ├── signal.c                     # Signal delivery and handling
│   ├── time.c                       # Timer interrupt, jiffies
│   ├── traps.c                      # Exception handlers
│   ├── irq.c                        # IRQ management
│   └── sys_mbc.c                    # Architecture-specific syscalls
├── mm/
│   └── init.c                       # Memory zones, bootmem
├── lib/
│   ├── memcpy.S                     # Optimized memory operations
│   └── string.c                     # String functions
└── boot/
    └── head.S                       # Same as kernel/head.S (or symlink)
```

### Mapping to UPC Primitives

| Linux Concept | arch/mbc Implementation | UPC Primitive |
|--------------|------------------------|---------------|
| Interrupt entry/exit | `entry.S`: push all regs, call C handler, pop, IRET | IVT at 0x0000 + IRET (opcode 0x18) |
| Syscall entry | `entry.S`: INT 0x80 handler, read r0=nr, dispatch | INT 0x80 → IVT[0x80] → handler |
| Context switch | `switch_to.h`: save r0-r15+PC+flags, restore next | Process table in BPF map (Level 4c) |
| Timer tick | `time.c`: INT 0x20 handler, increment jiffies | BPF timer → vector 0x20 |
| Console output | `serial_mbc.c`: write byte to Wotan 0xC001 | TTY data output (Level 4f) |
| Console input | `serial_mbc.c`: read byte from Wotan 0xFFFF | Input register (Level 4f) |
| Block I/O | `ramdisk_mbc.c`: SYS_READ_BLOCK/SYS_WRITE_BLOCK | Ramdisk at 0x800000 (Level 4e) |
| Interrupt enable/disable | `irq.h`: CLI/STI MBC instructions | `interrupts_enabled` CPU flag |
| Boot | `head.S`: read BootParams at 0x0100, init BSS, call start_kernel | Boot protocol v1 |

### The Atomic Operations Problem

Linux requires atomic operations throughout the kernel: spinlocks, atomic
counters, RCU, wait queues. The MBC ISA currently has NO atomic instructions.

**Required additions to MBC ISA:**

| Instruction | Opcode | Semantics | Why |
|------------|--------|-----------|-----|
| `CMPXCHG` | 0x3A | `if (mem[r_dst] == r_src) { mem[r_dst] = r_arg; r0=1 } else { r0=0 }` | Spinlocks, atomic_cmpxchg |
| `XCHG` | 0x3B | `tmp = mem[r_dst]; mem[r_dst] = r_src; r_src = tmp` | test_and_set_bit |
| `FENCE` | 0x3C | Memory barrier (no-op on single-core, but kernel checks) | mb()/rmb()/wmb() |

On single-core (which UPC is), atomics can be implemented by disabling
interrupts around the operation. This is how many nommu architectures
handle it. The kernel's `local_irq_save()`/`local_irq_restore()` sequence
provides atomicity without true atomic instructions.

**Practical approach:** Implement `CMPXCHG` as `CLI; compare; swap; STI` in
the BPF execution loop. Since UPC is single-core, this is correct. Add a
`FENCE` instruction that is a no-op (it satisfies the kernel's barrier
requirements because single-core has no reordering).

---

## 3. Syscall ABI Mapping

### Convention

Linux requires a defined syscall ABI: how userspace passes the syscall number
and arguments, and how the kernel returns results.

**Proposed MBC syscall ABI (matching existing INT 0x80 handler):**

| Role | Register | Notes |
|------|----------|-------|
| Syscall number | r8 (RV32I a0) | Linux convention: first arg register |
| Argument 1 | r9 (RV32I a1) | |
| Argument 2 | r10 (RV32I a2) | |
| Argument 3 | r11 (RV32I a3) | |
| Argument 4 | r12 (RV32I a4) | |
| Argument 5 | r13 (RV32I a5) | |
| Return value | r8 (RV32I a0) | Negative = -errno |

**Problem:** The current UPC OS primitives use a DIFFERENT convention:

- Current: `r0` = syscall number, `r1-r3` = args, `r0` = return
- Linux standard: syscall number in first arg register, args in subsequent

This mismatch exists because the Level 4b implementation was designed for
FUZIX (which has its own translation layer), not for native Linux.

**Decision required:** Either:
1. **Change the MBC syscall ABI** to match standard Linux arg-register
   convention (r8-r13 for RISC-V mapping). This breaks FUZIX compatibility
   but is cleaner for Linux.
2. **Keep r0-based ABI** and adapt the Linux arch port to use it. Unusual but
   workable -- the arch port defines its own ABI.
3. **Support both** via different INT vectors (INT 0x80 for Linux ABI, INT 0x81
   for legacy). Adds complexity but preserves compatibility.

**Recommendation: Option 2.** Keep the existing r0-based ABI. The Linux arch
port defines `__SYSCALL_REG_NR` = r0, `__SYSCALL_REG_ARG0` = r1, etc. This is
unusual but perfectly valid -- every arch defines its own convention. It avoids
breaking the 37 existing tests and the FUZIX compatibility layer.

### Syscall Count Analysis

**Linux nommu minimum boot requires approximately 50-60 syscalls.** Here is the
breakdown from tracing a minimal ARM nommu boot with busybox init:

#### Critical Path (kernel boot to init exec): ~15 syscalls

| Syscall | UPC Status | Notes |
|---------|-----------|-------|
| `exit` / `exit_group` | IMPLEMENTED | Process termination |
| `read` | IMPLEMENTED | Console + file I/O |
| `write` | IMPLEMENTED | Console output |
| `open` / `openat` | IMPLEMENTED (stub) | Needs real file ops for initramfs |
| `close` | IMPLEMENTED (stub) | |
| `brk` | IMPLEMENTED | Heap management |
| `mmap2` | IMPLEMENTED (basic) | Anonymous mappings for heap |
| `munmap` | IMPLEMENTED | |
| `execve` | NOT IMPLEMENTED | Load ELF-FDPIC or bFLT binary |
| `vfork` | NOT IMPLEMENTED | nommu uses vfork, not fork |
| `waitpid` / `wait4` | IMPLEMENTED | |
| `getpid` | IMPLEMENTED | |
| `uname` | NOT IMPLEMENTED | Returns kernel version string |
| `ioctl` | NOT IMPLEMENTED | TTY control (TCGETS/TCSETS minimum) |
| `clone` | NOT IMPLEMENTED | Thread creation |

#### Shell prompt (busybox sh): ~30 additional syscalls

| Syscall | UPC Status | Notes |
|---------|-----------|-------|
| `stat` / `fstat` / `lstat` | NOT IMPLEMENTED | File metadata |
| `getdents` / `getdents64` | NOT IMPLEMENTED | Directory listing |
| `dup` / `dup2` / `dup3` | NOT IMPLEMENTED | fd manipulation |
| `pipe` / `pipe2` | NOT IMPLEMENTED | Shell pipelines |
| `fcntl` | NOT IMPLEMENTED | fd flags |
| `rt_sigaction` | NOT IMPLEMENTED | Signal handlers |
| `rt_sigprocmask` | NOT IMPLEMENTED | Signal blocking |
| `sigreturn` | NOT IMPLEMENTED | Return from signal handler |
| `access` / `faccessat` | NOT IMPLEMENTED | File permission check |
| `getcwd` | NOT IMPLEMENTED | Current directory |
| `chdir` | NOT IMPLEMENTED | Change directory |
| `getuid` / `getgid` / `geteuid` / `getegid` | NOT IMPLEMENTED | User/group IDs |
| `set_tid_address` | NOT IMPLEMENTED | Thread ID |
| `clock_gettime` | IMPLEMENTED | |
| `nanosleep` | IMPLEMENTED | |
| `writev` | NOT IMPLEMENTED | Scatter-gather write |
| `mprotect` | NOT IMPLEMENTED (no-op on nommu) | Returns 0 |
| `sched_yield` | IMPLEMENTED | |

#### Summary

| Category | Count | Implemented | Gap |
|----------|-------|-------------|-----|
| Currently implemented (UPC) | 24 | 24 | -- |
| Required for kernel boot | ~15 | 9 | 6 |
| Required for shell prompt | ~30 | 3 | ~27 |
| **Total for shell prompt** | **~45** | **12** | **~33** |

The 24 UPC syscalls include 7 custom syscalls (SYS_READ_BLOCK, SYS_WRITE_BLOCK,
SYS_SET_PAGE_DIR, SYS_ENABLE_MMU, SYS_FLUSH_TLB, SYS_DRAW_FRAME, SYS_GET_KEY)
that are NOT Linux syscalls. Of the remaining 17 Linux-compatible syscalls, 12
are needed for the boot+shell path.

**The gap is ~33 syscalls.** Most are trivial stubs (return 0 or -ENOSYS), but
`execve`, `vfork`/`clone`, `rt_sigaction`, and the stat family require real
implementations.

---

## 4. Toolchain Strategy

Compiling the Linux kernel for MBC requires a C compiler that emits MBC
instructions (or something translatable to MBC).

### Option A: GCC Backend for MBC

**Approach:** Write a GCC machine description (`mbc.md`) that teaches GCC to
emit MBC assembly directly.

**Effort:** 3-6 months for a competent compiler engineer. GCC machine
descriptions are notoriously complex. The MBC ISA has only 43 opcodes and 16
registers, which simplifies things, but the GCC backend infrastructure
(register allocator integration, calling convention, ABI definition, ELF
support) is substantial.

**Pros:**
- Native code generation, no translation overhead
- Full optimization passes work correctly
- Kernel builds use standard `make ARCH=mbc CROSS_COMPILE=mbc-linux-`

**Cons:**
- Enormous upfront effort
- GCC backend maintenance burden
- Nobody else uses MBC, so no community help

### Option B: Compile to RISC-V, Transpile to MBC (Current Approach)

**Approach:** Use `riscv64-unknown-elf-gcc` with `-march=rv32i -mabi=ilp32`,
then run the `rv32i-to-mbc` translator (already exists in
`crates/monad-mbc/src/translator.rs`).

**Effort:** Already working for Doom. Kernel compilation needs additional work
to handle:
- Kernel-specific code patterns (inline assembly, special sections)
- Linux boot protocol expectations
- Kernel linker script adaptations

**Pros:**
- Translator already exists and is tested (65,917 RV32I -> 106,611 MBC for Doom)
- RISC-V is well-supported, mature toolchain
- No compiler engineering required
- Identical to current Doom pipeline

**Cons:**
- ~1.6x code size inflation (RV32I -> MBC is not 1:1)
- Linux kernel contains RISC-V inline assembly that must be replaced with MBC
  equivalents (context switch, barriers, atomic ops, boot code)
- The translator operates on ELF binaries; kernel build produces vmlinux which
  needs special handling
- RV2MBC_MAP translation for indirect jumps adds runtime overhead

**Specific kernel challenges with Option B:**
1. `arch/riscv/kernel/entry.S` -- ~800 lines of RISC-V assembly for syscall
   entry, IRQ handling, context switch. Cannot be auto-translated; must be
   rewritten in MBC assembly.
2. `arch/riscv/kernel/head.S` -- boot code, also assembly.
3. Inline `asm volatile` in atomics, barriers, CSR access -- all RISC-V
   specific. Must be replaced with MBC equivalents.
4. Linker script (`vmlinux.lds`) must produce a flat binary loadable at
   0x10000, not a standard ELF.

### Option C: LLVM Backend for MBC

**Approach:** Write an LLVM `MBCTargetMachine` that teaches LLVM to emit MBC.
LLVM is designed for adding new backends and has better documentation than GCC.

**Effort:** 2-4 months. LLVM backend infrastructure is cleaner than GCC.
TableGen descriptions are more straightforward than GCC `.md` files. The MBC
ISA is simple enough that the instruction selection patterns would be compact.

**Pros:**
- Cleaner than GCC backend
- Better documentation and modern tooling
- Clang can compile the Linux kernel (CONFIG_CC_IS_CLANG=y)
- Reusable for other MBC software (not just Linux)

**Cons:**
- Still significant effort
- LLVM backend for a 43-opcode ISA is overkill
- Linux kernel LLVM support has more restrictions than GCC

### Recommendation: Option B (RISC-V Transpilation) with Assembly Rewrite

**Rationale:**

1. The translator already works. Building a compiler backend is months of
   work that does not advance the "Linux boots" goal.
2. The kernel's C code (~99% of the kernel) compiles cleanly to RV32I and
   translates to MBC with no issues.
3. The ~1% that is architecture-specific assembly must be rewritten regardless
   of approach. Whether we rewrite it from RISC-V to MBC or write it from
   scratch for MBC is the same effort.
4. The approach is proven: Doom (106K MBC instructions) works today.

**Concrete plan:**
1. Fork `arch/riscv/` to `arch/mbc/`
2. Keep all C files, changing only `#include` paths and config
3. Rewrite `entry.S`, `head.S` in MBC assembly (~500 lines)
4. Replace inline asm in headers with MBC equivalents
5. Compile C files with `riscv64-unknown-elf-gcc -march=rv32i`
6. Translate the resulting object files with `rv32i-to-mbc`
7. Link into flat binary, load at 0x10000

This is a hybrid: native MBC assembly for the critical path, transpiled C
for everything else.

---

## 5. Kernel Configuration (defconfig)

Minimal `.config` for MBC/UPC nommu Linux:

```
# Architecture
CONFIG_MBC=y
CONFIG_CPU_MBC=y
CONFIG_MMU=n
CONFIG_32BIT=y

# Memory
CONFIG_FLAT_MEM=y
CONFIG_DRAM_BASE=0x00000000
CONFIG_DRAM_SIZE=0x04000000
CONFIG_PAGE_OFFSET=0x00000000

# No fork (nommu)
CONFIG_BINFMT_FLAT=y
CONFIG_BINFMT_ELF_FDPIC=y

# Console
CONFIG_SERIAL_CORE=y
CONFIG_SERIAL_CORE_CONSOLE=y
CONFIG_SERIAL_MBC=y
CONFIG_SERIAL_MBC_CONSOLE=y
CONFIG_CONSOLE_POLL=y

# Filesystem
CONFIG_TMPFS=y
CONFIG_PROC_FS=y
CONFIG_SYSFS=y
CONFIG_DEVTMPFS=y
CONFIG_DEVTMPFS_MOUNT=y

# Root filesystem
CONFIG_BLK_DEV_RAM=y
CONFIG_BLK_DEV_RAM_SIZE=4096
CONFIG_BLK_DEV_INITRD=y
CONFIG_INITRAMFS_SOURCE="initramfs.cpio"

# Scheduler
CONFIG_PREEMPT_NONE=y
CONFIG_HZ=100

# Debug (enable for bringup, disable later)
CONFIG_PRINTK=y
CONFIG_EARLY_PRINTK=y
CONFIG_DEBUG_INFO=y
CONFIG_DEBUG_KERNEL=y
CONFIG_PANIC_ON_OOPS=y

# Disable everything unnecessary
CONFIG_MODULES=n
CONFIG_NETWORKING=n
CONFIG_BLOCK=y
CONFIG_SOUND=n
CONFIG_USB=n
CONFIG_INPUT=n
CONFIG_FB=n
CONFIG_VGA_CONSOLE=n
CONFIG_SMP=n
CONFIG_NR_CPUS=1
CONFIG_EMBEDDED=y
CONFIG_EXPERT=y
CONFIG_KALLSYMS=y
CONFIG_FUTEX=n
CONFIG_EPOLL=n
CONFIG_SIGNALFD=n
CONFIG_TIMERFD=n
CONFIG_EVENTFD=n
CONFIG_AIO=n
CONFIG_IO_URING=n
```

**Expected kernel image size:** 500KB-1.5MB (fits easily in Wotan memory).
**Expected initramfs size:** 200KB-500KB (static busybox + init script).

---

## 6. Estimated Effort

### Phase Breakdown

All estimates assume a single developer with Linux kernel experience working
full-time. Add 50% for a developer new to kernel internals.

| Phase | Description | Effort | Deliverable | Success Criterion |
|-------|------------|--------|-------------|-------------------|
| **1** | arch/mbc skeleton | 1 week | Kconfig, Makefiles, headers that pass `make ARCH=mbc defconfig` | `make defconfig` succeeds |
| **2** | Boot code | 2 weeks | head.S, setup.c, linker script | Kernel entry point reached, BootParams read, `start_kernel()` called |
| **3** | Syscall entry + dispatch | 1 week | entry.S syscall path, generic syscall table | `INT 0x80` dispatches to `sys_write()` |
| **4** | Timer + scheduling | 1 week | time.c, IRQ entry in entry.S, jiffies incrementing | Timer interrupt fires, jiffies counts up, `schedule()` runs |
| **5** | Console driver | 1 week | serial_mbc.c (UART-like driver over Wotan I/O) | `printk()` output visible via Busboy TTY topic |
| **6** | Memory init | 1 week | mm/init.c, bootmem allocator setup | `kmalloc()` works, `/proc/meminfo` shows correct values |
| **7** | First boot attempt | 1 week | Integration, debug, iterate | Kernel boots far enough to print version string and panic ("No init found") |
| **8** | Root filesystem | 2 weeks | initramfs with static busybox, `/dev/console`, init script | Kernel mounts initramfs, execs `/sbin/init` |
| **9** | Shell prompt | 2 weeks | vfork+execve working, signal basics, fd operations | Busybox shell prints `#` prompt, `echo hello` works |

**Total estimated effort: 12-14 weeks (3-3.5 months)**

### Phase Details

#### Phase 1: arch/mbc Skeleton (1 week)

The bulk of this is boilerplate. Copy from `arch/microblaze/` (closest match)
and adapt. Key files:

- `arch/mbc/Kconfig` -- define `CONFIG_MBC`, `CONFIG_MMU=n` default, RAM base/size
- `arch/mbc/Makefile` -- CROSS_COMPILE, CFLAGS, subdirectories
- `arch/mbc/include/asm/*.h` -- ~40 header files, most can `#include <asm-generic/*.h>`
- Custom headers needed: `ptrace.h` (16 GPRs + PC + flags), `processor.h`, `elf.h`

**Risk:** Low. This is mechanical work.

#### Phase 2: Boot Code (2 weeks)

The hardest assembly work. `head.S` must:

1. Run from 0x10000 (kernel load address)
2. Read BootParams at 0x0100 (verify magic 0x554E4844)
3. Zero BSS section
4. Set up initial stack
5. Install IVT entries for timer (0x20) and syscall (0x80)
6. Call `start_kernel()` (C function)

This is ~200 lines of MBC assembly. The UPC MiniKernel test already does
steps 1-5, so this is an extension of existing work.

**Risk:** Medium. MBC assembly has no assembler tool -- code must be emitted
as raw 32-bit words. Consider writing a minimal assembler first.

**Blocker: MBC Assembler.** The kernel build needs `as` (assembler) that
understands MBC mnemonics. Options:
- Write a simple Python-based assembler (~500 lines) that reads `.S` files
  and emits 32-bit words
- Encode assembly by hand as `.word` directives in a C file (ugly but works
  for <500 instructions)

#### Phase 3: Syscall Entry (1 week)

The `entry.S` syscall path:
1. INT 0x80 fires
2. Save all registers to kernel stack (PUSH r0-r15, save flags)
3. Read syscall number from r0
4. Look up handler in `sys_call_table[]`
5. Call handler with args in r1-r5
6. Restore registers, put return value in r0
7. IRET

This is essentially the Level 4b handler, but more robust (save ALL registers,
not just the ones the handler clobbers).

**Risk:** Low. Direct extension of existing implementation.

#### Phase 4: Timer + Scheduling (1 week)

Timer interrupt handler:
1. INT 0x20 fires
2. Save registers (same as syscall entry)
3. Increment `jiffies`
4. Call `scheduler_tick()`
5. If `need_resched` is set, call `schedule()`
6. Restore registers, IRET

The Level 4c scheduler already does this. The Linux scheduler is more complex
but uses the same interrupt-driven model.

**Risk:** Low-medium. Linux scheduler has more state than our round-robin.

#### Phase 5: Console Driver (1 week)

Implement a `struct uart_port` driver that maps to Wotan I/O addresses:
- TX: write byte to 0xC001
- RX: read byte from 0xFFFF
- Status: read 0xC002 (TX empty, RX ready)

Register as early console for `printk()`. This is the first sign of life:
when `printk("Linux version ...")` appears on the Busboy TTY topic, we know
the kernel is alive.

**Risk:** Low. Standard UART driver pattern, well-documented.

#### Phase 6: Memory Init (1 week)

Tell Linux about available memory:
- Physical RAM: 0x00000000 to 0x03FFFFFF (64MB)
- Kernel image: 0x00010000 to 0x000XXXXX
- Initramfs: 0x00800000 to 0x008XXXXX
- Everything else: free pages

With nommu, this is simpler than with MMU -- no page tables to set up, just
register memory regions with the bootmem allocator.

**Risk:** Low-medium. Getting memory zones right is fiddly but well-documented.

#### Phase 7: First Boot Attempt (1 week)

Integrate phases 1-6, attempt boot, debug. Expected outcome:

```
Linux version 6.x.0-mbc (mbc-linux-gcc) ...
MBC CPU: Monad Bytecode CPU, 16 registers, 43 opcodes
Memory: 64MB available
Console: MBC serial on Wotan I/O
...
Kernel panic - not syncing: No working init found.
```

**"Kernel panic - not syncing: No working init found" IS SUCCESS at this
phase.** It means the kernel booted, initialized memory, set up the console,
and got far enough to look for `/sbin/init`.

**Risk:** High. Integration bugs are where most time is spent. Expect 70% of
this week to be debugging with `printk()` and analyzing register dumps.

#### Phase 8: Root Filesystem (2 weeks)

Build initramfs containing:
- `/sbin/init` -- busybox init (static, bFLT or ELF-FDPIC format)
- `/bin/busybox` -- static busybox binary with sh, echo, cat, ls
- `/dev/console` -- character device node
- `/etc/inittab` -- `::sysinit:/bin/sh`
- `/proc/` and `/sys/` -- mount points

The busybox binary must be cross-compiled for MBC. Using the transpilation
approach: compile busybox for RV32I with newlib/musl, then translate to MBC.
Busybox statically linked for RV32I is ~800KB-1.5MB.

**Critical dependency:** `execve()` must work. On nommu Linux, `execve()` loads
a bFLT (binary flat) or ELF-FDPIC executable. bFLT is simpler:
- Load flat binary at a free address
- Relocate (apply fixups from relocation table)
- Set up stack (arguments, environment)
- Jump to entry point

**Risk:** High. `execve()` is the single most complex syscall and the most
likely place to get stuck.

#### Phase 9: Shell Prompt (2 weeks)

Get busybox `sh` running:
- `vfork()` + `execve()` for running commands
- Signal handling (SIGCHLD at minimum)
- fd operations (dup2 for redirection)
- Basic stat() for command lookup in $PATH

**Success criterion:** Type `echo hello` at the shell prompt, see `hello`
in the output. Type `ls /`, see the initramfs contents.

**Risk:** Medium. Most issues at this stage are missing syscalls that return
-ENOSYS. Each one is individually simple but there are many.

### Total Timeline

```
Week  1:    arch/mbc skeleton
Weeks 2-3:  Boot code (head.S, setup.c)
Week  4:    Syscall entry
Week  5:    Timer + scheduling
Week  6:    Console driver
Week  7:    Memory init
Week  8:    First boot (expect panic -- this is good)
Weeks 9-10: Root filesystem + execve
Weeks 11-12: Shell prompt
Weeks 13-14: Buffer for debugging, missed issues
```

**Calendar time with a single developer: 3-3.5 months.**
**With two developers (one on kernel, one on toolchain/userspace): 2-2.5 months.**

---

## 7. Alternative: Emulate an Existing nommu Target

Instead of porting Linux to MBC natively, we could run an existing nommu
Linux binary by emulating that architecture on MBC. This is the
"meta-emulation" approach already explored for Unix v4 (PDP-11 on RV32I on
MBC) in `COMPUTATIONAL_GENERALITY.md`.

### Option 7A: ARM Cortex-M Emulation

Run an ARM Cortex-M3/M4 uClinux binary inside an ARM emulator compiled to MBC.

**Approach:**
1. Take a working uClinux kernel image for STM32F4 (Cortex-M4)
2. Compile a Cortex-M emulator (e.g., `thumbulator`, ~3K LOC C) to RV32I
3. Translate to MBC with existing translator
4. Run the ARM kernel inside the emulator inside MBC

**Performance:**
- Each ARM instruction: ~50-100 MBC instructions (emulation overhead)
- ARM uClinux boot: ~10M ARM instructions to shell prompt
- Total: ~500M-1B MBC instructions
- At UPC's current ~100K instructions/sec: **5,000-10,000 seconds (1-3 hours)**
- This is impractical for interactive use

**Verdict: TOO SLOW for practical use, but interesting as a proof of concept.**

### Option 7B: RISC-V nommu Emulation

Run a RISC-V nommu Linux binary on MBC. Since we already translate RV32I to
MBC, this is conceptually simpler.

**Approach:**
1. Build uClinux for RISC-V with `CONFIG_MMU=n`
2. The kernel binary is already RV32I -- translate it directly to MBC
3. BUT: the kernel contains RISC-V CSR instructions, privilege levels,
   trap handling that don't translate

**Problem:** The RISC-V kernel uses CSR instructions (`csrr`, `csrw`,
`csrrw`) extensively for interrupt management, timer access, and privilege
control. These have no MBC equivalent and cannot be auto-translated. We would
need to:
- Trap CSR accesses and emulate them in the MBC execution loop
- Emulate RISC-V privilege levels (M/S/U mode)
- Emulate the RISC-V interrupt controller (PLIC/CLINT)

This is essentially writing a RISC-V system emulator, which is MORE work than
a native MBC port.

**Verdict: MORE WORK THAN NATIVE PORT. Not recommended.**

### Option 7C: m68k ColdFire Emulation

Run an m68k ColdFire uClinux binary inside a 68k emulator on MBC.

**Approach:**
1. Take a working uClinux kernel for ColdFire (MCF5272 or similar)
2. Compile Musashi (m68k emulator, ~8K LOC C) to RV32I
3. Translate to MBC
4. Run the m68k kernel inside the emulator

**Performance:**
- Similar to ARM: ~50-100x overhead
- Too slow for interactive use
- But m68k uClinux is very well-tested and many reference boards exist

**Verdict: Same speed problem as ARM. Not recommended for interactive use.**

### Emulation vs Native Port Decision

| Criterion | Emulation (7A/7C) | Native Port |
|-----------|-------------------|-------------|
| Time to first boot | 2-3 weeks | 8-10 weeks |
| Interactive performance | Unusable (~hours to boot) | Usable (~seconds to boot) |
| Maintenance burden | Low (emulator is frozen) | High (track kernel changes) |
| Educational value | High (meta-emulation is cool) | Higher (real arch port) |
| Resume value | Good ("ran Linux via emulation") | Excellent ("ported Linux to custom CPU") |
| Long-term viability | Dead end | Foundation for real system |

**Recommendation: Native port.** The emulation approach is a fun demo but
not a viable system. The native port is harder but produces something real.

**Exception:** If the goal is purely "demonstrate Linux boots on UPC as fast
as possible" for a conference talk or funding pitch, the ARM emulation approach
could produce a (very slow) boot in 2-3 weeks. Run it overnight, record the
output, present the recording.

---

## 8. Decision Matrix

Key decisions that must be made before implementation begins:

| # | Decision | Options | Recommendation | Blocking? |
|---|----------|---------|---------------|-----------|
| 1 | Toolchain | GCC backend / RV32I transpile / LLVM backend | RV32I transpile (Option B) | Yes |
| 2 | Syscall ABI | r0-based (current) / arg-register-based / both | r0-based (Option 2) | Yes |
| 3 | Atomic ops | New MBC instructions / CLI+STI wrapper / emulation | CLI+STI wrapper (single-core safe) | Yes |
| 4 | MBC assembler | Python tool / hand-encode / extend translator | Python tool (~500 lines) | Yes |
| 5 | Binary format | bFLT / ELF-FDPIC / flat binary | bFLT (simpler, standard for nommu) | No (Phase 8) |
| 6 | Reference arch | Microblaze / m68k / RISC-V | Microblaze (closest match) | No |
| 7 | Approach | Native port / Emulation | Native port | Yes |

---

## 9. Blockers and Risks

### Hard Blockers (must resolve before starting)

| Blocker | Description | Resolution |
|---------|------------|------------|
| **No atomic instructions** | Linux kernel requires `cmpxchg()`, `atomic_add()`, `test_and_set_bit()`. MBC has no atomic ops. | Implement as CLI + memory op + STI sequences. Single-core makes this correct. Add FENCE as no-op. |
| **No MBC assembler** | `head.S` and `entry.S` must be written in MBC assembly. No assembler exists. | Write minimal Python assembler or encode as `.word` directives. |
| **No `execve()`** | The most complex missing syscall. Required for loading init and shell. | Implement bFLT loader (binary flat format). ~500 lines of C. |
| **No `vfork()`** | nommu Linux uses `vfork()` instead of `fork()`. Semantics are different (parent blocks until child execs). | Implement `vfork()` as: parent suspends, child runs in parent's address space until `execve()` or `_exit()`. |

### Soft Blockers (can work around, but impact quality)

| Risk | Impact | Mitigation |
|------|--------|------------|
| BPF instruction limit | Kernel interrupt handler code path may exceed BPF verifier limits | Split into tail-called sub-programs (existing pattern from Doom) |
| Wotan latency | Memory access through Wotan adds latency vs. real RAM | Acceptable for boot demo; optimize later |
| RV2MBC translation for kernel | Kernel code patterns may stress translator edge cases | Run translator on kernel objects early; fix translator bugs as found |
| No debugging tools | No GDB, no JTAG, no serial debugger for MBC | Heavy use of `printk()`, register dump on panic, custom crash handler |
| Signal delivery | Linux signal infrastructure is complex | Start with SIGCHLD only; add others as needed |

### What Could Go Wrong

1. **BPF verifier rejects kernel code paths.** The Linux kernel's interrupt
   entry/exit is deeper than Doom's execution loop. If the BPF verifier rejects
   the complexity, we need to restructure as multiple tail-called programs.
   This is solvable but adds 1-2 weeks.

2. **Memory consumption.** Linux kernel + initramfs + busybox might exceed
   comfortable BPF map sizes. The kernel alone is 500KB-1.5MB; with initramfs,
   total could be 2-3MB of ROM + 4-8MB of working RAM. UPC has 64MB addressable,
   so this should be fine, but BPF map allocation is a separate constraint.

3. **Performance.** Linux does far more memory accesses than Doom. Each
   instruction takes ~1us on UPC (packet-driven execution). Kernel boot to
   shell involves ~10-50M instructions = 10-50 seconds. This is acceptable
   for a demo but not for interactive use. Optimization (instruction batching,
   larger tick quanta) would be needed for usability.

4. **Toolchain gaps.** The RV32I-to-MBC translator was tested on Doom
   (single-threaded game). Kernel code uses patterns not seen in Doom: inline
   asm, computed gotos, per-cpu variables, section attributes. Each may
   require translator fixes.

---

## References

- `docs/doom/ROAD_TO_LINUX.md` -- OS primitive design and the Dream Ladder
- `docs/doom/UPC_OS_PRIMITIVES.md` -- Level 4a-4f implementation summary
- `docs/doom/FUZIX_PORT_ANALYSIS.md` -- Level 5b feasibility (FUZIX syscall table)
- `docs/doom/UPC_BOOT_PROTOCOL.md` -- Boot sequence and BootParams structure
- `docs/protocol/mbc-isa-reference.md` -- MBC instruction set (43 opcodes)
- `docs/doom/DOOM-BUILD-GUIDE.md` -- RV32I-to-MBC translation pipeline
- Linux kernel `Documentation/admin-guide/README.rst` -- kernel build
- Linux kernel `arch/microblaze/` -- reference nommu architecture port
- Linux kernel `arch/m68k/` -- reference ColdFire nommu support
- Linux kernel `fs/binfmt_flat.c` -- bFLT binary format loader

---

*This document is a practical assessment, not a promise. The estimates are
honest: this is 3+ months of hard kernel work. But the UPC already has
all the hardware primitives (timer, syscalls, scheduler, block device, console)
that Linux needs. The gap is software: headers, assembly stubs, and syscall
implementations. The hardest single piece is `execve()`. Everything else is
tedious but straightforward.*

*SPDX-License-Identifier: GPL-3.0-or-later*
