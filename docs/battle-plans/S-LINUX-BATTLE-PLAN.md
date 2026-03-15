<!--
SPDX-License-Identifier: GPL-3.0-or-later
-->

# S-LINUX BATTLE PLAN — 12 Weeks to Linux on UPC

**Date**: March 15, 2026
**Sprint**: S-LINUX — Port uClinux/nommu to the Unheaded Protocol Computer
**Prerequisite**: All 4 hard blockers resolved (assembler, atomics, bFLT, vfork)
**Target**: Linux kernel panic on UPC (Week 8), shell prompt (Week 12)
**Hardware**: AMD RX 7700 XT, 16GB DDR5, 1TB NVMe, 2TB HDD (WEST)
**Agent Strategy**: Weekly sprints, parallelizable where noted
**Commit Cadence**: Daily commits minimum
**Spec Reference**: `docs/doom/UCLINUX_PORT_GUIDE.md`, `docs/doom/ROAD_TO_LINUX.md`
**Reference Architecture**: `arch/microblaze/` (Xilinx soft-core, closest match to MBC)
**Toolchain**: RV32I cross-compile + `rv32i-to-mbc` transpiler (Option B from port guide)
**Syscall ABI**: r0-based (Option 2 — keep existing convention, define in arch headers)
**Binary Format**: bFLT (binary flat, standard for nommu)
**Kernel Base**: torvalds/linux v6.x, `CONFIG_MMU=n`

---

## LEGEND

```
[B] = Bash command           | [V] = Verification          | [D] = Debug
[W] = Write/create file      | [R] = Read/inspect          | [S] = Sudo required
[P] = Parallelizable         | [C] = Commit checkpoint     | [STUCK] = Skipped
[BLOCKED] = Blocked by upstream
```

---

## HARD BLOCKER PREREQUISITES (Must Be Complete Before Week 1)

These are the 4 hard blockers from `UCLINUX_PORT_GUIDE.md` Section 9. All must
be resolved and merged before the 12-week clock starts.

| # | Blocker | Resolution | Validation |
|---|---------|-----------|------------|
| HB-1 | **No MBC assembler** | Python-based assembler (~500 LOC) that reads `.S` files and emits 32-bit MBC words. Must support labels, `.word`, `.byte`, `.section`, `#include`. Located at `tools/mbc-as/mbc_as.py`. | `echo "NOP\nHLT" \| mbc-as -o test.bin && xxd test.bin` produces correct opcodes |
| HB-2 | **No atomic instructions** | Implement `CMPXCHG` (0x3A), `XCHG` (0x3B), `FENCE` (0x3C) in BPF execution loop. Single-core: `CLI; op; STI` sequence is correct. Add to `crates/monad-mbc/src/execute.rs`. | `test_cmpxchg`, `test_xchg`, `test_fence` pass in integration suite |
| HB-3 | **No bFLT loader** | Implement `binfmt_flat` loader for UPCFlat binaries. Parse bFLT header, load text+data, apply relocations, set entry point. ~500 LOC C or Rust. | Load a trivial bFLT binary (prints "hello"), verify output |
| HB-4 | **No vfork()** | Implement `vfork()` semantics: parent suspends, child runs in parent's address space until `execve()` or `_exit()`. Extend Level 4c scheduler. | `vfork_test`: parent blocks, child execs, parent resumes |

**Exit gate for starting Week 1**: All 4 blockers resolved, 37 existing OS primitive tests still pass, new blocker tests pass.

---

## PHASE 1: FOUNDATION (Weeks 1-3)

### Week 1: arch/mbc Kernel Skeleton

**Goal**: Linux kernel source tree compiles for MBC target (even if it does nothing)
**Effort**: ~40 hours
**Risk**: Low — mechanical boilerplate, copy from `arch/microblaze/`
**Reference**: `arch/microblaze/` for directory structure, `arch/m68k/` for nommu patterns

#### Deliverables

- [ ] **Step 1.1** [B] (~5m): **Shallow-clone Linux kernel**

```bash
git clone --depth=1 --branch v6.8 https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git ~/src/linux-mbc
cd ~/src/linux-mbc
git checkout -b mbc-port
```

**Verification**: `ls arch/ | head` shows existing architectures.

---

- [ ] **Step 1.2** [B][W] (~15m): **Create arch/mbc directory skeleton**

```bash
cd ~/src/linux-mbc
mkdir -p arch/mbc/{boot,configs,include/asm,include/uapi/asm,kernel,mm,lib}
```

Create the following empty/stub files:

```
arch/mbc/
├── Kconfig
├── Makefile
├── boot/
│   └── Makefile
├── configs/
│   └── mbc_defconfig
├── include/
│   ├── asm/
│   │   ├── Kbuild
│   │   ├── atomic.h
│   │   ├── barrier.h
│   │   ├── bitops.h
│   │   ├── bug.h
│   │   ├── cmpxchg.h
│   │   ├── current.h
│   │   ├── elf.h
│   │   ├── io.h
│   │   ├── irq.h
│   │   ├── irqflags.h
│   │   ├── page.h
│   │   ├── pgalloc.h
│   │   ├── pgtable.h
│   │   ├── processor.h
│   │   ├── ptrace.h
│   │   ├── setup.h
│   │   ├── string.h
│   │   ├── switch_to.h
│   │   ├── thread_info.h
│   │   ├── timex.h
│   │   ├── tlb.h
│   │   ├── unistd.h
│   │   └── vmalloc.h
│   └── uapi/asm/
│       ├── Kbuild
│       ├── signal.h
│       └── unistd.h
├── kernel/
│   ├── Makefile
│   ├── entry.S
│   ├── head.S
│   ├── irq.c
│   ├── process.c
│   ├── setup.c
│   ├── signal.c
│   ├── sys_mbc.c
│   ├── time.c
│   ├── traps.c
│   └── vmlinux.lds.S
├── mm/
│   ├── Makefile
│   └── init.c
└── lib/
    ├── Makefile
    └── string.c
```

**Verification**: `find arch/mbc -type f | wc -l` shows ~40 files.

---

- [ ] **Step 1.3** [W] (~30m): **Write arch/mbc/Kconfig**

Define the MBC architecture in Kconfig. Key options:

```kconfig
config MBC
    bool
    select ARCH_NO_SWAP
    select ARCH_HAS_SYNC_DMA_FOR_CPU
    select GENERIC_ATOMIC64
    select GENERIC_CLOCKEVENTS
    select GENERIC_CPU_DEVICES
    select GENERIC_IRQ_SHOW
    select GENERIC_STRNCPY_FROM_USER
    select GENERIC_STRNLEN_USER
    select HAVE_ARCH_KGDB if !SMP
    select HAVE_MEMBLOCK
    select IRQ_DOMAIN
    select MODULES_USE_ELF_RELA
    select NO_DMA
    select OF if !MMU
    select SET_FS

config CPU_MBC
    bool
    default y

config MMU
    bool
    default n

config 32BIT
    bool
    default y

config ZONE_DMA
    bool
    default y

config DRAM_BASE
    hex "DRAM base address"
    default 0x00000000

config DRAM_SIZE
    hex "DRAM size"
    default 0x04000000
    help
      Total RAM in bytes. Default 64MB (0x04000000) matching
      Wotan RAM_MAP capacity.
```

Model after `arch/microblaze/Kconfig` with all MMU-dependent options removed.

**Verification**: `grep CONFIG_MBC arch/mbc/Kconfig` returns matches.

---

- [ ] **Step 1.4** [W] (~30m): **Write arch/mbc/Makefile**

```makefile
# SPDX-License-Identifier: GPL-3.0-or-later
#
# arch/mbc/Makefile — Top-level Makefile for Monad Bytecode CPU

KBUILD_DEFCONFIG := mbc_defconfig

# Cross-compiler: compile C to RV32I, then translate to MBC
# The actual translation happens in a post-link step
CROSS_COMPILE ?= riscv64-unknown-elf-
KBUILD_CFLAGS += -march=rv32i -mabi=ilp32 -mno-relax
KBUILD_CFLAGS += -fno-pic -fno-pie
KBUILD_CFLAGS += -DCONFIG_MBC

# No floating point
KBUILD_CFLAGS += -msoft-float

KBUILD_AFLAGS += -march=rv32i -mabi=ilp32

head-y := arch/mbc/kernel/head.o

core-y += arch/mbc/kernel/ arch/mbc/mm/ arch/mbc/lib/

libs-y += arch/mbc/lib/

boot := arch/mbc/boot

all: vmlinux

archclean:
	$(Q)$(MAKE) $(clean)=$(boot)

PHONY += mbc_defconfig
mbc_defconfig:
	$(Q)$(MAKE) -f $(srctree)/scripts/kconfig/Makefile defconfig \
		KBUILD_DEFCONFIG=$(KBUILD_DEFCONFIG)

define archhelp
  echo '  vmlinux    - Flat binary kernel image'
endef
```

**Verification**: File exists and has no syntax errors (`make -n ARCH=mbc` does not crash immediately).

---

- [ ] **Step 1.5** [W] (~45m): **Write include/asm/ptrace.h — register layout**

This defines the MBC register set as seen by Linux. Must match `MbcCpuState` from
`ebpf/monad-common/src/lib.rs`.

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_PTRACE_H
#define _ASM_MBC_PTRACE_H

#include <linux/types.h>

/*
 * MBC register layout — 16 general-purpose 32-bit registers.
 *
 * r0-r7:   General purpose (r0 also syscall number/return)
 * r8-r13:  General purpose (r8-r13 map to RV32I a0-a5)
 * r14:     Frame pointer (FP)
 * r15:     Stack pointer (SP)
 *
 * Special registers (not in GPR file):
 *   PC     — Program counter
 *   FLAGS  — CPU flags (zero, carry, sign, overflow, interrupt_enable)
 */

struct pt_regs {
    unsigned long regs[16];     /* r0-r15 */
    unsigned long pc;           /* program counter */
    unsigned long flags;        /* CPU flags word */
    unsigned long orig_r0;      /* original r0 (syscall number, saved before dispatch) */
};

#define PT_R0       0
#define PT_R1       1
#define PT_R2       2
#define PT_R3       3
#define PT_R4       4
#define PT_R5       5
#define PT_R6       6
#define PT_R7       7
#define PT_R8       8
#define PT_R9       9
#define PT_R10      10
#define PT_R11      11
#define PT_R12      12
#define PT_R13      13
#define PT_FP       14
#define PT_SP       15
#define PT_PC       16
#define PT_FLAGS    17
#define PT_ORIG_R0  18

/* Syscall ABI: r0 = syscall number, r1-r5 = args, r0 = return */
#define REG_SYSCALL_NR(regs)    ((regs)->regs[0])
#define REG_SYSCALL_RET(regs)   ((regs)->regs[0])
#define REG_SYSCALL_ARG0(regs)  ((regs)->regs[1])
#define REG_SYSCALL_ARG1(regs)  ((regs)->regs[2])
#define REG_SYSCALL_ARG2(regs)  ((regs)->regs[3])
#define REG_SYSCALL_ARG3(regs)  ((regs)->regs[4])
#define REG_SYSCALL_ARG4(regs)  ((regs)->regs[5])

#define user_mode(regs) (0)  /* UPC has no privilege levels; always supervisor */

#define instruction_pointer(regs)   ((regs)->pc)
#define user_stack_pointer(regs)    ((regs)->regs[15])
#define profile_pc(regs)            instruction_pointer(regs)

#endif /* _ASM_MBC_PTRACE_H */
```

**Verification**: Header compiles without errors when included from a test `.c` file.

---

- [ ] **Step 1.6** [W] (~45m): **Write include/asm/unistd.h — syscall numbers**

Use the Linux generic syscall table (`include/uapi/asm-generic/unistd.h`) as the
base. Override only what MBC needs.

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_UNISTD_H
#define _ASM_MBC_UNISTD_H

/*
 * MBC uses the generic syscall table with the r0-based ABI.
 * Syscall number in r0, args in r1-r5, return in r0.
 *
 * We supplement with 7 UPC-specific syscalls (200-265).
 */
#define __ARCH_WANT_SYS_CLONE

#include <asm-generic/unistd.h>

/* UPC-specific syscalls (not in generic table) */
#define __NR_read_block     200
#define __NR_write_block    201
#define __NR_set_page_dir   250
#define __NR_enable_mmu     251
#define __NR_flush_tlb      252
#define __NR_draw_frame     260
#define __NR_get_key        261

#endif /* _ASM_MBC_UNISTD_H */
```

**Verification**: Grep for `__NR_exit` in the generated `asm-generic/unistd.h` to confirm inclusion.

---

- [ ] **Step 1.7** [W] (~30m): **Write include/asm/processor.h — thread struct**

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_PROCESSOR_H
#define _ASM_MBC_PROCESSOR_H

#include <asm/ptrace.h>

/*
 * Saved kernel-mode register state for context switch.
 * This is the subset of registers the kernel must save/restore
 * across switch_to(). User-mode registers are in pt_regs on
 * the kernel stack.
 */
struct thread_struct {
    unsigned long sp;           /* kernel stack pointer */
    unsigned long pc;           /* saved PC (return address) */
    unsigned long regs[16];     /* callee-saved registers */
    unsigned long flags;        /* saved flags */
};

#define INIT_THREAD { \
    .sp = 0, \
    .pc = 0, \
    .flags = 0, \
}

#define task_pt_regs(tsk) \
    ((struct pt_regs *)(task_stack_page(tsk) + THREAD_SIZE) - 1)

/* MBC has no I/O port instructions; all I/O is memory-mapped */
#define TASK_UNMAPPED_BASE      0

/* Thread/process size limits */
#define TASK_SIZE               0x04000000  /* 64MB, matching DRAM_SIZE */

/* Start of user code (kernel is at 0x10000, user after kernel) */
#define STACK_TOP               TASK_SIZE
#define STACK_TOP_MAX           TASK_SIZE

/* No alignment constraints beyond word alignment */
unsigned long get_wchan(struct task_struct *p);

#define cpu_relax()     barrier()

#endif /* _ASM_MBC_PROCESSOR_H */
```

---

- [ ] **Step 1.8** [W] (~30m): **Write include/asm/atomic.h — atomic operations via CLI/STI**

Single-core UPC: atomics are implemented by disabling interrupts.

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_ATOMIC_H
#define _ASM_MBC_ATOMIC_H

#include <asm/irqflags.h>
#include <asm/cmpxchg.h>
#include <asm-generic/atomic.h>

/*
 * MBC is single-core. All atomic operations are implemented by
 * disabling interrupts (CLI), performing the operation, and
 * re-enabling interrupts (STI). This is correct because there
 * is no other execution context that could interleave.
 *
 * When CMPXCHG/XCHG opcodes (0x3A/0x3B) are available, these
 * can be optimized to use hardware atomics.
 */

#endif /* _ASM_MBC_ATOMIC_H */
```

---

- [ ] **Step 1.9** [W] (~30m): **Write include/asm/irqflags.h — CLI/STI wrappers**

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_IRQFLAGS_H
#define _ASM_MBC_IRQFLAGS_H

/*
 * Interrupt enable/disable for MBC.
 *
 * MBC has CLI (opcode 0x17) and STI (opcode 0x16) instructions.
 * These set/clear the interrupts_enabled flag in MbcCpuState.
 *
 * Since we compile C via RV32I translation, these must be
 * implemented as inline functions that emit the correct MBC
 * opcodes via the assembler or as special markers that the
 * translator recognizes.
 *
 * For initial bringup: use volatile memory-mapped writes to
 * a special address that the BPF execution loop intercepts.
 */

#include <linux/types.h>

/* Memory-mapped interrupt control register */
#define MBC_IRQ_CONTROL_ADDR    0xD000

static inline unsigned long arch_local_save_flags(void)
{
    /* Read interrupt state from memory-mapped register */
    return *(volatile unsigned long *)MBC_IRQ_CONTROL_ADDR;
}

static inline void arch_local_irq_disable(void)
{
    *(volatile unsigned long *)MBC_IRQ_CONTROL_ADDR = 0;
}

static inline void arch_local_irq_enable(void)
{
    *(volatile unsigned long *)MBC_IRQ_CONTROL_ADDR = 1;
}

static inline unsigned long arch_local_irq_save(void)
{
    unsigned long flags = arch_local_save_flags();
    arch_local_irq_disable();
    return flags;
}

static inline void arch_local_irq_restore(unsigned long flags)
{
    *(volatile unsigned long *)MBC_IRQ_CONTROL_ADDR = flags;
}

static inline bool arch_irqs_disabled_flags(unsigned long flags)
{
    return flags == 0;
}

static inline bool arch_irqs_disabled(void)
{
    return arch_irqs_disabled_flags(arch_local_save_flags());
}

#endif /* _ASM_MBC_IRQFLAGS_H */
```

---

- [ ] **Step 1.10** [W] (~30m): **Write include/asm/page.h — page size and memory model**

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_PAGE_H
#define _ASM_MBC_PAGE_H

#define PAGE_SHIFT      12
#define PAGE_SIZE       (1UL << PAGE_SHIFT)     /* 4096 bytes */
#define PAGE_MASK       (~(PAGE_SIZE - 1))

/* Physical memory starts at 0 on MBC/UPC */
#define PAGE_OFFSET     0x00000000UL
#define PHYS_OFFSET     0x00000000UL

#define __pa(x)         ((unsigned long)(x) - PAGE_OFFSET)
#define __va(x)         ((void *)((unsigned long)(x) + PAGE_OFFSET))

#define virt_to_page(addr)  (mem_map + (((unsigned long)(addr) - PAGE_OFFSET) >> PAGE_SHIFT))
#define page_to_virt(page)  ((void *)(((page) - mem_map) << PAGE_SHIFT) + PAGE_OFFSET)

/* nommu: virtual == physical */
#define virt_addr_valid(kaddr) ((unsigned long)(kaddr) < (unsigned long)high_memory)

#include <asm-generic/memory_model.h>
#include <asm-generic/getorder.h>

#endif /* _ASM_MBC_PAGE_H */
```

---

- [ ] **Step 1.11** [W] (~20m): **Write include/asm/cmpxchg.h**

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_CMPXCHG_H
#define _ASM_MBC_CMPXCHG_H

#include <linux/irqflags.h>

/*
 * cmpxchg via interrupt disable (single-core safe).
 * When CMPXCHG opcode (0x3A) is available, replace with
 * inline asm.
 */

static inline unsigned long __cmpxchg(volatile void *ptr,
                                       unsigned long old,
                                       unsigned long new,
                                       int size)
{
    unsigned long flags, prev;

    local_irq_save(flags);
    switch (size) {
    case 1:
        prev = *(volatile u8 *)ptr;
        if (prev == (u8)old)
            *(volatile u8 *)ptr = (u8)new;
        break;
    case 2:
        prev = *(volatile u16 *)ptr;
        if (prev == (u16)old)
            *(volatile u16 *)ptr = (u16)new;
        break;
    case 4:
        prev = *(volatile u32 *)ptr;
        if (prev == (u32)old)
            *(volatile u32 *)ptr = (u32)new;
        break;
    default:
        prev = 0;
        BUILD_BUG();
    }
    local_irq_restore(flags);
    return prev;
}

#define arch_cmpxchg(ptr, o, n) \
    (__cmpxchg((ptr), (unsigned long)(o), (unsigned long)(n), sizeof(*(ptr))))

static inline unsigned long __xchg(volatile void *ptr,
                                    unsigned long val,
                                    int size)
{
    unsigned long flags, prev;

    local_irq_save(flags);
    switch (size) {
    case 1:
        prev = *(volatile u8 *)ptr;
        *(volatile u8 *)ptr = (u8)val;
        break;
    case 2:
        prev = *(volatile u16 *)ptr;
        *(volatile u16 *)ptr = (u16)val;
        break;
    case 4:
        prev = *(volatile u32 *)ptr;
        *(volatile u32 *)ptr = (u32)val;
        break;
    default:
        prev = 0;
        BUILD_BUG();
    }
    local_irq_restore(flags);
    return prev;
}

#define arch_xchg(ptr, v) \
    (__xchg((ptr), (unsigned long)(v), sizeof(*(ptr))))

#endif /* _ASM_MBC_CMPXCHG_H */
```

---

- [ ] **Step 1.12** [W] (~20m): **Write include/asm/barrier.h**

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_BARRIER_H
#define _ASM_MBC_BARRIER_H

/*
 * MBC is single-core with in-order execution.
 * Memory barriers are compiler barriers only.
 * FENCE opcode (0x3C) exists but is a no-op.
 */

#define mb()    barrier()
#define rmb()   barrier()
#define wmb()   barrier()

#include <asm-generic/barrier.h>

#endif /* _ASM_MBC_BARRIER_H */
```

---

- [ ] **Step 1.13** [W] (~20m): **Write include/asm/thread_info.h**

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_THREAD_INFO_H
#define _ASM_MBC_THREAD_INFO_H

#include <linux/compiler.h>
#include <asm/page.h>

#define THREAD_SIZE_ORDER   1
#define THREAD_SIZE         (PAGE_SIZE << THREAD_SIZE_ORDER)  /* 8KB */

#ifndef __ASSEMBLY__

struct thread_info {
    struct task_struct  *task;
    unsigned long       flags;
    int                 preempt_count;
    unsigned long       tp_value;       /* thread pointer */
};

#define INIT_THREAD_INFO(tsk) \
{ \
    .task           = &tsk, \
    .flags          = 0, \
    .preempt_count  = INIT_PREEMPT_COUNT, \
}

#endif /* !__ASSEMBLY__ */

/* Thread info flags */
#define TIF_SYSCALL_TRACE   0
#define TIF_NOTIFY_RESUME   1
#define TIF_SIGPENDING      2
#define TIF_NEED_RESCHED    3
#define TIF_MEMDIE          4
#define TIF_RESTORE_SIGMASK 9

#define _TIF_SYSCALL_TRACE  (1 << TIF_SYSCALL_TRACE)
#define _TIF_NOTIFY_RESUME  (1 << TIF_NOTIFY_RESUME)
#define _TIF_SIGPENDING     (1 << TIF_SIGPENDING)
#define _TIF_NEED_RESCHED   (1 << TIF_NEED_RESCHED)

#define _TIF_WORK_MASK      (_TIF_NEED_RESCHED | _TIF_SIGPENDING | _TIF_NOTIFY_RESUME)

#endif /* _ASM_MBC_THREAD_INFO_H */
```

---

- [ ] **Step 1.14** [W] (~20m): **Write include/asm/elf.h — ELF machine type**

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_ELF_H
#define _ASM_MBC_ELF_H

/*
 * ELF definitions for MBC.
 * We use a private EM_ number (0xBEEF) since MBC is not
 * registered with the ELF specification. In practice,
 * bFLT binaries are used, not ELF.
 */

#define EM_MBC          0xBEEF

#define ELF_ARCH        EM_MBC
#define ELF_CLASS       ELFCLASS32
#define ELF_DATA        ELFDATA2LSB

/* MBC has 16 GPRs + PC + FLAGS */
typedef unsigned long   elf_greg_t;
#define ELF_NGREG       (sizeof(struct pt_regs) / sizeof(elf_greg_t))
typedef elf_greg_t      elf_gregset_t[ELF_NGREG];

/* No FPU */
typedef unsigned long   elf_fpregset_t;

#define elf_check_arch(x) ((x)->e_machine == EM_MBC)

#define ELF_ET_DYN_BASE (TASK_SIZE / 3 * 2)
#define ELF_EXEC_PAGESIZE PAGE_SIZE
#define ELF_PLATFORM    "mbc"

#endif /* _ASM_MBC_ELF_H */
```

---

- [ ] **Step 1.15** [W] (~20m): **Write include/asm/io.h — memory-mapped I/O**

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_IO_H
#define _ASM_MBC_IO_H

#include <asm/page.h>

/*
 * MBC I/O is entirely memory-mapped through Wotan RAM_MAP.
 * No port I/O instructions exist.
 *
 * I/O address map (from UPC_OS_PRIMITIVES.md):
 *   0xC001  TTY data output
 *   0xC002  TTY status
 *   0xC003  TTY control
 *   0xD000  IRQ control
 *   0xFFFF  Input register
 */

#define readb(addr)     (*(volatile u8  __force *)(addr))
#define readw(addr)     (*(volatile u16 __force *)(addr))
#define readl(addr)     (*(volatile u32 __force *)(addr))

#define writeb(val, addr) (*(volatile u8  __force *)(addr) = (val))
#define writew(val, addr) (*(volatile u16 __force *)(addr) = (val))
#define writel(val, addr) (*(volatile u32 __force *)(addr) = (val))

#define inb(port)       readb(port)
#define inw(port)       readw(port)
#define inl(port)       readl(port)
#define outb(val, port) writeb(val, port)
#define outw(val, port) writew(val, port)
#define outl(val, port) writel(val, port)

#include <asm-generic/io.h>

#endif /* _ASM_MBC_IO_H */
```

---

- [ ] **Step 1.16** [W] (~20m): **Write include/asm/Kbuild — generic header fallbacks**

```makefile
# SPDX-License-Identifier: GPL-3.0-or-later
# Headers that use asm-generic versions

generic-y += bug.h
generic-y += bitops.h
generic-y += cacheflush.h
generic-y += checksum.h
generic-y += compat.h
generic-y += delay.h
generic-y += device.h
generic-y += div64.h
generic-y += dma.h
generic-y += emergency-restart.h
generic-y += exec.h
generic-y += futex.h
generic-y += hardirq.h
generic-y += hw_irq.h
generic-y += kdebug.h
generic-y += kmap_types.h
generic-y += kprobes.h
generic-y += linkage.h
generic-y += local.h
generic-y += mcs_spinlock.h
generic-y += mm-arch-hooks.h
generic-y += mmu.h
generic-y += mmu_context.h
generic-y += module.h
generic-y += param.h
generic-y += pci.h
generic-y += percpu.h
generic-y += preempt.h
generic-y += sections.h
generic-y += serial.h
generic-y += shmparam.h
generic-y += signal.h
generic-y += spinlock.h
generic-y += syscall.h
generic-y += syscalls.h
generic-y += topology.h
generic-y += trace_clock.h
generic-y += uaccess.h
generic-y += vga.h
generic-y += word-at-a-time.h
```

---

- [ ] **Step 1.17** [W] (~15m): **Write include/asm/switch_to.h**

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_SWITCH_TO_H
#define _ASM_MBC_SWITCH_TO_H

struct task_struct;
struct thread_struct;

extern struct task_struct *__switch_to(struct task_struct *prev,
                                       struct task_struct *next);

#define switch_to(prev, next, last) \
do { \
    (last) = __switch_to((prev), (next)); \
} while (0)

#endif /* _ASM_MBC_SWITCH_TO_H */
```

---

- [ ] **Step 1.18** [W] (~15m): **Write include/asm/current.h**

```c
/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _ASM_MBC_CURRENT_H
#define _ASM_MBC_CURRENT_H

/*
 * "current" task pointer.
 * On MBC, stored as a global variable (single-core, no per-cpu).
 */

#include <linux/thread_info.h>

struct task_struct;

static inline struct task_struct *get_current(void)
{
    return current_thread_info()->task;
}

#define current get_current()

#endif /* _ASM_MBC_CURRENT_H */
```

---

- [ ] **Step 1.19** [W] (~30m): **Write kernel stub files (setup.c, process.c, traps.c, irq.c, time.c, signal.c, sys_mbc.c)**

Minimal C stubs with function signatures that satisfy the linker. Every function
body is `/* TODO: implement */` or a one-line return for this week. Real implementations
come in Weeks 2-4.

Key stubs:
- `setup_arch(char **cmdline_p)` — `*cmdline_p = boot_command_line;`
- `calibrate_delay()` — `loops_per_jiffy = 1000;`
- `copy_thread()` — return 0
- `start_thread()` — set regs->pc and regs->regs[15]
- `show_regs()` — printk all 16 registers
- `flush_thread()` — empty
- `exit_thread()` — empty
- `trap_init()` — empty
- `init_IRQ()` — empty
- `time_init()` — empty
- `do_signal()` — empty

**Verification**: `make ARCH=mbc defconfig` does not crash on missing symbol errors.

---

- [ ] **Step 1.20** [W] (~30m): **Write vmlinux.lds.S — linker script**

```ld
/* SPDX-License-Identifier: GPL-3.0-or-later */
/*
 * Linker script for MBC kernel.
 * Produces a flat binary loadable at 0x10000.
 */

#include <asm-generic/vmlinux.lds.h>

OUTPUT_FORMAT("elf32-littleriscv")
OUTPUT_ARCH(riscv)
ENTRY(_start)

SECTIONS
{
    . = 0x00010000;     /* Kernel load address */

    .text : {
        _stext = .;
        HEAD_TEXT
        TEXT_TEXT
        *(.text.*)
        _etext = .;
    }

    . = ALIGN(4096);

    .rodata : {
        RODATA
    }

    . = ALIGN(4096);

    .data : {
        DATA_DATA
        CONSTRUCTORS
    }

    . = ALIGN(4);

    .init.data : {
        INIT_DATA
    }

    . = ALIGN(4);

    .bss : {
        __bss_start = .;
        BSS_SECTION(4, 4, 4)
        __bss_stop = .;
    }

    _end = .;

    DISCARDS
}
```

---

- [ ] **Step 1.21** [W] (~15m): **Write mbc_defconfig**

```
# arch/mbc/configs/mbc_defconfig
CONFIG_MBC=y
CONFIG_CPU_MBC=y
CONFIG_MMU=n
CONFIG_32BIT=y
CONFIG_FLAT_MEM=y
CONFIG_DRAM_BASE=0x00000000
CONFIG_DRAM_SIZE=0x04000000
CONFIG_PAGE_OFFSET=0x00000000
CONFIG_BINFMT_FLAT=y
CONFIG_SERIAL_CORE=y
CONFIG_SERIAL_CORE_CONSOLE=y
CONFIG_TMPFS=y
CONFIG_PROC_FS=y
CONFIG_SYSFS=y
CONFIG_DEVTMPFS=y
CONFIG_DEVTMPFS_MOUNT=y
CONFIG_BLK_DEV_RAM=y
CONFIG_BLK_DEV_RAM_SIZE=4096
CONFIG_BLK_DEV_INITRD=y
CONFIG_PREEMPT_NONE=y
CONFIG_HZ=12
CONFIG_PRINTK=y
CONFIG_EARLY_PRINTK=y
CONFIG_DEBUG_INFO=y
CONFIG_DEBUG_KERNEL=y
CONFIG_PANIC_ON_OOPS=y
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

---

- [ ] **Step 1.22** [W] (~15m): **Write kernel/Makefile and mm/Makefile and lib/Makefile**

```makefile
# arch/mbc/kernel/Makefile
obj-y := setup.o process.o traps.o irq.o time.o signal.o sys_mbc.o entry.o head.o

# arch/mbc/mm/Makefile
obj-y := init.o

# arch/mbc/lib/Makefile
obj-y := string.o
```

---

- [ ] **Step 1.23** [W] (~30m): **Write stub entry.S and head.S (minimal assembly)**

For Week 1 these are minimal stubs that define the expected symbols but contain
only NOPs. Real assembly comes in Week 2.

```asm
/* arch/mbc/kernel/head.S — kernel entry point (stub) */
/* SPDX-License-Identifier: GPL-3.0-or-later */

.section .head.text, "ax"
.globl _start
_start:
    /* Week 2: clear BSS, set up stack, call start_kernel */
    j       start_kernel

.globl __bss_start
.globl __bss_stop
```

```asm
/* arch/mbc/kernel/entry.S — syscall/IRQ entry (stub) */
/* SPDX-License-Identifier: GPL-3.0-or-later */

.section .text
.globl ret_from_fork
ret_from_fork:
    j       ret_from_fork

.globl system_call
system_call:
    j       system_call

.globl ret_from_exception
ret_from_exception:
    j       ret_from_exception
```

---

- [ ] **Step 1.24** [B][V][C] (~60m): **First build attempt — fix all compile errors**

```bash
cd ~/src/linux-mbc
make ARCH=mbc defconfig
make ARCH=mbc CROSS_COMPILE=riscv64-unknown-elf- -j$(nproc) 2>&1 | tee build-w1.log
```

This WILL fail. Iterate:
1. Read error messages
2. Add missing `#include <asm-generic/...>` entries to Kbuild
3. Add missing stub functions
4. Repeat until `make` completes (warnings OK, errors not OK)

Expect 20-30 iterations. Common missing pieces:
- `asm/syscall.h`
- `asm/pgalloc.h` / `asm/pgtable.h` (nommu stubs)
- `asm/sections.h`
- `asm/uaccess.h` (nommu: direct pointer access)
- Various generic fallback headers

**Exit Gate**: `make ARCH=mbc defconfig && make ARCH=mbc` completes with zero errors.

---

- [ ] **Step 1.25** [C] (~5m): **Commit: "feat(arch/mbc): kernel skeleton — compiles for MBC target"**

```bash
cd ~/src/linux-mbc
git add arch/mbc/
git commit -m "feat(arch/mbc): kernel skeleton — compiles for MBC target

Complete arch/mbc/ directory with Kconfig, Makefiles, ~40 headers,
stub kernel functions, and defconfig. Based on arch/microblaze/
with all MMU code removed.

make ARCH=mbc defconfig && make ARCH=mbc completes."
```

---

### Week 2: Boot Code + Syscall Entry

**Goal**: Kernel can be loaded and jumps to `start_kernel()`
**Effort**: ~50 hours
**Risk**: Medium — assembly writing is the hardest single task
**Dependency**: Week 1 complete, HB-1 (MBC assembler) resolved

#### Deliverables

- [ ] **Step 2.1** [W] (~120m): **Write arch/mbc/boot/head.S — boot entry point**

Full MBC assembly via our assembler. This is the first code that runs when
the kernel is loaded at 0x10000.

```
Boot sequence (matching UPC_BOOT_PROTOCOL.md):
1. Computermancer loads kernel at 0x10000, sets PC=0x10000, SP=0x03F00000
2. head.S runs:
   a. Read BootParams at 0x0100 (word addr 0x40)
   b. Verify magic 0x554E4844 ("UNHD")
   c. Save memory size, ramdisk addr/size to globals
   d. Zero BSS section (__bss_start to __bss_stop)
   e. Set up initial kernel stack (SP = 0x03F00000)
   f. Install IVT entries:
      - Vector 0x20 (timer) -> timer_interrupt_handler
      - Vector 0x80 (syscall) -> system_call_handler
   g. Enable interrupts (STI)
   h. Call start_kernel() (C function, never returns)
```

Approximately 150-200 MBC assembly instructions. Use the assembler from HB-1.

**Verification**: `mbc-as head.S -o head.o && objdump -d head.o` shows valid instructions.

---

- [ ] **Step 2.2** [W] (~180m): **Write arch/mbc/kernel/entry.S — syscall entry/exit**

The most critical assembly file. Handles:

**Syscall path (INT 0x80):**
```
1. Save all 16 GPRs + PC + FLAGS to kernel stack (pt_regs)
2. Save original r0 (syscall number) to pt_regs.orig_r0
3. Load syscall table address
4. Bounds-check r0 against NR_syscalls
5. Call sys_call_table[r0] with args from r1-r5
6. Store return value to pt_regs.r0
7. Check TIF_NEED_RESCHED -> call schedule() if set
8. Check TIF_SIGPENDING -> call do_signal() if set
9. Restore all registers from pt_regs
10. IRET
```

**Timer interrupt path (INT 0x20):**
```
1. Save all registers to kernel stack
2. Call timer_interrupt() (C function)
3. Check TIF_NEED_RESCHED -> call schedule() if set
4. Restore all registers
5. IRET
```

**ret_from_fork path:**
```
1. Schedule tail cleanup
2. Check pending signals
3. Return to userspace (or new kernel thread)
```

Approximately 200-300 MBC assembly instructions.

**Verification**: Symbols `system_call`, `ret_from_fork`, `timer_interrupt_entry` are defined and reachable.

---

- [ ] **Step 2.3** [W] (~90m): **Write arch/mbc/kernel/setup.c — platform setup**

```c
void __init setup_arch(char **cmdline_p)
{
    /* 1. Read BootParams from RAM 0x0100 */
    struct upc_bootparams *bp = (void *)0x0100;
    if (bp->magic != 0x554E4844)
        panic("UPC: Invalid boot magic (expected UNHD)");

    /* 2. Set up memory info */
    memory_size = bp->memory_size;  /* 64MB */

    /* 3. Parse kernel command line */
    strlcpy(boot_command_line, (char *)bp->boot_args_addr, COMMAND_LINE_SIZE);
    *cmdline_p = boot_command_line;

    /* 4. Set up initrd location */
    initrd_start = bp->ramdisk_addr;
    initrd_end = bp->ramdisk_addr + bp->ramdisk_size;

    /* 5. Register memory regions with bootmem */
    memblock_add(0, memory_size);
    memblock_reserve(0, 0x10000);              /* IVT + BootParams + stack */
    memblock_reserve(0x10000, bp->kernel_size); /* kernel image */
    if (bp->ramdisk_size > 0)
        memblock_reserve(bp->ramdisk_addr, bp->ramdisk_size);

    /* 6. Console setup (earlycon) */
    setup_early_printk();

    printk(KERN_INFO "UPC: Monad Bytecode CPU, %dMB RAM\n",
           memory_size >> 20);
    printk(KERN_INFO "UPC: Kernel at 0x%08x (%d bytes)\n",
           bp->kernel_addr, bp->kernel_size);
    printk(KERN_INFO "UPC: Ramdisk at 0x%08x (%d bytes)\n",
           bp->ramdisk_addr, bp->ramdisk_size);
    printk(KERN_INFO "UPC: Timer at %d Hz\n", bp->tick_rate_hz);
}
```

**Verification**: `setup_arch()` compiles. When loaded on UPC, printk output appears.

---

- [ ] **Step 2.4** [W] (~60m): **Write vmlinux.lds.S — production linker script**

Refine Week 1 stub into a complete linker script that:
- Places kernel at 0x10000
- Defines `__bss_start` / `__bss_stop` for BSS clearing
- Includes init sections for `__initcall` infrastructure
- Defines `_stext` / `_etext` / `_sdata` / `_edata` for memory info
- Produces a flat binary suitable for loading via `wotan-ctl boot`

---

- [ ] **Step 2.5** [W] (~30m): **Write post-link translation script**

```bash
#!/bin/bash
# tools/mbc-translate-kernel.sh
# Translates the RV32I vmlinux to MBC binary
#
# Usage: ./tools/mbc-translate-kernel.sh vmlinux vmlinux.mbc

set -euo pipefail

VMLINUX=$1
OUTPUT=$2

# 1. Extract text section as raw binary
riscv64-unknown-elf-objcopy -O binary $VMLINUX ${VMLINUX}.bin

# 2. Translate RV32I instructions to MBC
cargo run --release -p monad-mbc --bin rv32i-to-mbc -- \
    --input ${VMLINUX}.bin \
    --output ${OUTPUT} \
    --base-addr 0x10000 \
    --emit-flat

# 3. Verify output size
SIZE=$(stat -c%s ${OUTPUT})
echo "MBC kernel image: ${SIZE} bytes ($(( SIZE / 1024 ))KB)"

# 4. Merge hand-written MBC assembly (head.S, entry.S)
# These were assembled directly by mbc-as, not translated
cat arch/mbc/boot/head.mbc ${OUTPUT} > ${OUTPUT}.final
mv ${OUTPUT}.final ${OUTPUT}

echo "Final kernel with boot code: $(stat -c%s ${OUTPUT}) bytes"
```

---

- [ ] **Step 2.6** [B][V] (~120m): **Integration: load kernel on UPC, reach start_kernel**

```bash
# Build kernel
cd ~/src/linux-mbc
make ARCH=mbc CROSS_COMPILE=riscv64-unknown-elf- -j$(nproc)

# Translate to MBC
./tools/mbc-translate-kernel.sh vmlinux vmlinux.mbc

# Load on UPC
cd ~/tmp/unheaded
cargo run --release -p wotan-ctl -- boot \
    --kernel ~/src/linux-mbc/vmlinux.mbc \
    --label linux-test

# Watch console output
cargo run --release -p busboy -- subscribe compute.tty.linux-test
```

Debug cycle: this is where 70% of Week 2 time goes. Common issues:
- BSS not zeroed correctly (garbage in global variables)
- Stack pointer not aligned (crash on first function call)
- IVT entries wrong (interrupt fires, jumps to garbage)
- Translator bug on kernel code pattern (fix in `rv32i-to-mbc`)

**Exit Gate**: Kernel loaded via `wotan-ctl boot`, reaches `start_kernel()`, prints banner.
Kernel panic is expected and acceptable.

---

- [ ] **Step 2.7** [C] (~5m): **Commit: "feat(arch/mbc): boot code — kernel reaches start_kernel()"**

---

### Week 3: Timer + Console Drivers

**Goal**: Kernel can schedule and print to console
**Effort**: ~40 hours
**Risk**: Low-medium — standard driver patterns, well-documented

#### Deliverables

- [ ] **Step 3.1** [W] (~90m): **Write arch/mbc/kernel/time.c — Timer driver**

```c
/*
 * Timer driver for MBC/UPC.
 * BPF timer fires at BootParams.tick_rate_hz (default 12 Hz).
 * Each tick triggers INT 0x20, which calls timer_interrupt().
 */

#include <linux/clockchips.h>
#include <linux/clocksource.h>
#include <linux/interrupt.h>
#include <linux/time.h>

static unsigned long mbc_ticks;

/* Called from entry.S timer_interrupt_entry */
void timer_interrupt(struct pt_regs *regs)
{
    mbc_ticks++;

    /* Update jiffies and run scheduler tick */
    legacy_timer_tick(1);
}

/* Clocksource: read tick counter */
static u64 mbc_clocksource_read(struct clocksource *cs)
{
    return mbc_ticks;
}

static struct clocksource mbc_clocksource = {
    .name   = "mbc_timer",
    .rating = 200,
    .read   = mbc_clocksource_read,
    .mask   = CLOCKSOURCE_MASK(32),
    .flags  = CLOCK_SOURCE_IS_CONTINUOUS,
};

void __init time_init(void)
{
    /* HZ is set to 12 in defconfig, matching UPC timer rate */
    clocksource_register_hz(&mbc_clocksource, HZ);
    printk(KERN_INFO "UPC: Timer initialized at %d Hz\n", HZ);
}
```

**Verification**: `jiffies` increments. `printk` timestamps change.

---

- [ ] **Step 3.2** [W] (~120m): **Write drivers/tty/serial/mbc_console.c — Console driver**

```c
/*
 * MBC/UPC serial console driver.
 * Output: write bytes to TTY_MAP at 0xC001
 * Input: read bytes from input register at 0xFFFF
 *
 * Registers as both early console (earlycon) and normal serial port.
 */

#include <linux/console.h>
#include <linux/init.h>
#include <linux/serial_core.h>
#include <linux/tty.h>
#include <asm/io.h>

#define MBC_TTY_DATA    0xC001  /* Write: byte -> tty output */
#define MBC_TTY_STATUS  0xC002  /* Read: buffer status */
#define MBC_TTY_CONTROL 0xC003  /* Write: line discipline */
#define MBC_INPUT_REG   0xFFFF  /* Read: keyboard input byte */

static void mbc_console_putchar(char ch)
{
    /* Busy-wait until TX buffer is ready */
    while (readl(MBC_TTY_STATUS) & 0x01)
        cpu_relax();

    writeb(ch, MBC_TTY_DATA);
}

static void mbc_console_write(struct console *co,
                               const char *s, unsigned int count)
{
    unsigned int i;
    for (i = 0; i < count; i++) {
        if (s[i] == '\n')
            mbc_console_putchar('\r');
        mbc_console_putchar(s[i]);
    }
}

static int __init mbc_console_setup(struct console *co, char *options)
{
    return 0;
}

static struct console mbc_console = {
    .name   = "mbccon",
    .write  = mbc_console_write,
    .setup  = mbc_console_setup,
    .flags  = CON_PRINTBUFFER | CON_BOOT,
    .index  = -1,
};

/* Early console for boot messages (before full driver init) */
static void mbc_early_write(struct console *con,
                             const char *s, unsigned n)
{
    mbc_console_write(con, s, n);
}

static struct console mbc_earlycon = {
    .name   = "earlycon",
    .write  = mbc_early_write,
    .flags  = CON_PRINTBUFFER | CON_BOOT | CON_ANYTIME,
    .index  = -1,
};

void __init setup_early_printk(void)
{
    register_console(&mbc_earlycon);
}

static int __init mbc_console_init(void)
{
    register_console(&mbc_console);
    return 0;
}
console_initcall(mbc_console_init);
```

**Verification**: `printk("Linux version ...")` appears in Busboy tty topic.

---

- [ ] **Step 3.3** [W] (~30m): **Register console in setup.c**

Add `setup_early_printk()` call to `setup_arch()` so that printk output
is visible from the very first kernel message.

---

- [ ] **Step 3.4** [B][V] (~120m): **Integration: kernel boots, prints banner, panics**

```bash
# Build and translate
make ARCH=mbc CROSS_COMPILE=riscv64-unknown-elf- -j$(nproc)
./tools/mbc-translate-kernel.sh vmlinux vmlinux.mbc

# Boot on UPC
cargo run --release -p wotan-ctl -- boot \
    --kernel ~/src/linux-mbc/vmlinux.mbc \
    --label linux-w3

# Expected output:
# Linux version 6.8.0-mbc (mbc-linux-gcc) ...
# UPC: Monad Bytecode CPU, 64MB RAM
# ...
# Kernel panic - not syncing: No init found.
```

Debug cycle: timer interrupts must fire correctly, console output must be visible.

---

- [ ] **Step 3.5** [C] (~5m): **Commit: "feat(arch/mbc): timer + console — kernel prints and panics"**

**EXIT GATE FOR PHASE 1**: Kernel prints boot banner and panics with
"No init found." This is a MAJOR milestone — Linux is running on UPC.

---

## PHASE 2: PROCESS MODEL (Weeks 4-6)

### Week 4: Process Management

**Goal**: `fork()` and `exec()` work
**Effort**: ~50 hours
**Risk**: High — `execve()` is the single most complex syscall

#### Deliverables

- [ ] **Step 4.1** [W] (~90m): **Write arch/mbc/kernel/process.c — context switch**

Implement:
- `__switch_to(prev, next)` — Save prev's registers to `thread_struct`, restore next's.
  On UPC this also updates the BPF PROC_TABLE map via a special memory-mapped write
  to synchronize with the BPF-side scheduler.
- `copy_thread(clone_flags, usp, kthread_arg, p)` — Set up a new task's kernel stack
  with a `pt_regs` frame so that when it is scheduled, it returns to `ret_from_fork`.
- `start_thread(regs, pc, usp)` — Set up user-mode registers for a freshly exec'd process.
- `kernel_thread(fn, arg, flags)` — Create a kernel thread. Used for kthreadd (PID 2).

**Verification**: `kernel_thread()` creates kthreadd, which is visible in the boot log.

---

- [ ] **Step 4.2** [W] (~120m): **Implement execve() — bFLT binary loader**

This is the hardest single piece of the entire port. Implement `do_execve()` arch support:

1. Open file from initramfs (VFS layer provides this)
2. Read bFLT header (magic, entry, data_start, data_end, bss_end, stack_size, reloc_start, reloc_count)
3. Allocate memory for text+data+bss+stack (nommu: `do_mmap()` on flat memory)
4. Load text segment to allocated address
5. Load data segment after text
6. Zero BSS region
7. Apply relocations (add load-address offset to each reloc entry)
8. Set up user stack with argv, envp, auxv
9. Set entry point in pt_regs
10. Return to userspace via `ret_from_fork`

Most of this is in `fs/binfmt_flat.c` (kernel generic code). The arch part is:
- `start_thread()` — set PC and SP in pt_regs
- `ELF_PLAT_INIT()` — clear registers
- Stack setup (push argc, argv pointers, envp pointers, NULL terminators)

**Verification**: Load a trivial bFLT binary that writes "hello" to stdout. See output.

---

- [ ] **Step 4.3** [W] (~60m): **Implement vfork() — nommu fork**

On nommu, `fork()` returns `-EINVAL`. Only `vfork()` works:
1. Parent calls `vfork()`
2. Child is created sharing parent's address space
3. Parent is suspended (marked `TASK_UNINTERRUPTIBLE`)
4. Child runs until `execve()` or `_exit()`
5. On `execve()`: child gets new address space, parent is woken
6. On `_exit()`: parent is woken

Implement in `copy_thread()` by checking `CLONE_VFORK` flag. Set up
completion mechanism so parent blocks.

**Verification**: `vfork()` + `execve()` + `wait()` sequence works.

---

- [ ] **Step 4.4** [W] (~60m): **Write arch/mbc/kernel/signal.c — signal handling**

Minimal signal support for boot:
- `SIGCHLD` delivery to parent when child exits (required for `wait()`)
- `SIGKILL` / `SIGTERM` for process termination
- Signal frame setup on user stack
- `sigreturn()` to restore context after signal handler

Full signal support (arbitrary handlers, signal masks) deferred to Week 9.

**Verification**: Parent receives `SIGCHLD` when child exits. `wait()` returns.

---

- [ ] **Step 4.5** [B][V][C] (~60m): **Integration: kthreadd runs, fork+exec works**

Boot kernel, verify `kthreadd` (PID 2) is created. If initramfs is present,
verify kernel attempts to exec `/sbin/init`.

**Exit Gate**: `kernel_thread()` creates kthreadd, `vfork()`+`execve()` works in test.

---

### Week 5: Memory Management (nommu)

**Goal**: Flat memory model works, `malloc()`/`brk()` functional
**Effort**: ~35 hours
**Risk**: Low-medium — nommu memory is simpler than full MMU

#### Deliverables

- [ ] **Step 5.1** [W] (~60m): **Write arch/mbc/mm/init.c — memory initialization**

```c
void __init mem_init(void)
{
    /* Set up free memory regions */
    /* Kernel is at 0x10000, ramdisk at 0x800000 */
    /* Everything between kernel end and ramdisk start is free */
    /* Everything after ramdisk end to DRAM_SIZE is free */

    unsigned long free_start = PAGE_ALIGN((unsigned long)_end);
    unsigned long free_end = memory_size;

    totalram_pages = free_pages = (free_end - free_start) >> PAGE_SHIFT;

    printk(KERN_INFO "Memory: %luK/%luK available (%dK kernel code, %dK data)\n",
           nr_free_pages() << (PAGE_SHIFT - 10),
           totalram_pages << (PAGE_SHIFT - 10),
           (_etext - _stext) >> 10,
           (_edata - _sdata) >> 10);
}
```

---

- [ ] **Step 5.2** [W] (~60m): **Implement nommu memory operations**

- `do_mmap()` — Allocate a contiguous region from the flat address space.
  On nommu, this uses `kobjsize()` and direct address assignment.
- `do_brk()` — Extend a process's data segment. Maps to the existing
  `SYS_BRK` implementation.
- `do_munmap()` — Free a previously mapped region.

These are mostly handled by the generic `mm/nommu.c` in the kernel tree.
The arch part is minimal: provide `__get_free_pages()` and related allocators.

**Verification**: `kmalloc()` / `kfree()` work. No OOM panics during boot.

---

- [ ] **Step 5.3** [W] (~30m): **Configure SLUB allocator for flat memory**

SLUB works on nommu. Ensure `CONFIG_SLUB=y` in defconfig. Verify that
slab caches are created during boot (visible in printk output).

---

- [ ] **Step 5.4** [B][V][C] (~30m): **Integration: memory init completes, no OOM**

**Exit Gate**: Kernel allocates memory without OOM panics. `/proc/meminfo`
(once procfs works) shows correct values.

---

### Week 6: Block Device + Filesystem

**Goal**: Mount root filesystem
**Effort**: ~50 hours
**Risk**: Medium — filesystem integration is fiddly

#### Deliverables

- [ ] **Step 6.1** [W] (~90m): **Write drivers/block/mbc_ramdisk.c — ramdisk driver**

```c
/*
 * MBC ramdisk block device.
 * Uses SYS_READ_BLOCK/SYS_WRITE_BLOCK to access the 4MB
 * ramdisk in Wotan extended memory at RAMDISK_BASE (0x100000).
 *
 * Registers as /dev/ram0 with 512-byte sectors.
 */

#define MBC_RAMDISK_SECTORS     8192    /* 4MB / 512 */
#define MBC_SECTOR_SIZE         512

static void mbc_ramdisk_request(struct request_queue *q)
{
    struct request *req;
    while ((req = blk_fetch_request(q)) != NULL) {
        unsigned long sector = blk_rq_pos(req);
        unsigned long nsect = blk_rq_cur_sectors(req);
        void *buffer = bio_data(req->bio);

        if (rq_data_dir(req) == READ) {
            /* Read sectors from Wotan ramdisk */
            for (unsigned long i = 0; i < nsect; i++) {
                mbc_read_block(sector + i, buffer + i * MBC_SECTOR_SIZE);
            }
        } else {
            /* Write sectors to Wotan ramdisk */
            for (unsigned long i = 0; i < nsect; i++) {
                mbc_write_block(sector + i, buffer + i * MBC_SECTOR_SIZE);
            }
        }
        __blk_end_request_cur(req, 0);
    }
}
```

**Verification**: `/dev/ram0` appears. `dd if=/dev/ram0 bs=512 count=1` reads a block.

---

- [ ] **Step 6.2** [W] (~120m): **Build initramfs**

Create a minimal root filesystem as initramfs (cpio archive):

```
initramfs/
├── bin/
│   └── sh            # Minimal shell (busybox or hand-written)
├── dev/
│   └── console       # c 5 1 (character device node)
├── etc/
│   └── inittab       # ::sysinit:/bin/sh
├── init              # /init script (PID 1)
├── proc/             # Mount point
├── sys/              # Mount point
└── tmp/              # Writable temp
```

The `/init` script:

```sh
#!/bin/sh
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
echo "UPC Linux: init running"
exec /bin/sh
```

For `/bin/sh`: either cross-compile busybox-static for RV32I and translate
to MBC, or write a minimal shell (50-100 lines of C) that handles `echo`,
`ls`, `cat`, `exit`.

Cross-compile initramfs:
```bash
riscv64-unknown-elf-gcc -march=rv32i -mabi=ilp32 -static \
    -o initramfs/bin/sh sh.c
# Then translate to MBC (bFLT format)
```

Package as cpio:
```bash
cd initramfs && find . | cpio -o -H newc > ../initramfs.cpio
```

**Verification**: `file initramfs.cpio` shows "ASCII cpio archive".

---

- [ ] **Step 6.3** [W] (~30m): **Enable initramfs in kernel config**

```
CONFIG_BLK_DEV_INITRD=y
CONFIG_INITRAMFS_SOURCE="initramfs.cpio"
```

Alternatively, use `CONFIG_INITRAMFS_SOURCE` to point at the directory
and let the kernel build system create the cpio.

---

- [ ] **Step 6.4** [W] (~60m): **Enable VFS + ramfs/romfs**

Ensure the following are enabled:
- `CONFIG_TMPFS=y` — tmpfs for `/tmp`
- `CONFIG_PROC_FS=y` — procfs for `/proc`
- `CONFIG_SYSFS=y` — sysfs for `/sys`
- `CONFIG_DEVTMPFS=y` — device nodes auto-created
- `CONFIG_ROMFS_FS=y` — romfs (optional, if using romfs instead of initramfs)
- `CONFIG_CRAMFS=y` — cramfs (optional alternative)

The kernel generic VFS code handles mount, open, read, write, close.
The arch part is providing `uaccess.h` functions (on nommu, these are
direct pointer operations — no copy_to_user/copy_from_user translation).

---

- [ ] **Step 6.5** [B][V][C] (~120m): **Integration: kernel mounts root, runs /init**

```bash
# Rebuild kernel with initramfs
make ARCH=mbc CROSS_COMPILE=riscv64-unknown-elf- -j$(nproc)
./tools/mbc-translate-kernel.sh vmlinux vmlinux.mbc

# Boot
cargo run --release -p wotan-ctl -- boot \
    --kernel ~/src/linux-mbc/vmlinux.mbc \
    --ramdisk ~/src/linux-mbc/initramfs.cpio \
    --label linux-w6

# Expected output:
# ...
# VFS: Mounted root (ramfs filesystem) readonly on device 0:1.
# Freeing unused kernel memory: ...
# UPC Linux: init running
```

**Exit Gate**: Kernel mounts root filesystem, runs `/init`.

---

## PHASE 3: USERSPACE (Weeks 7-9)

### Week 7: Init + Shell

**Goal**: `/init` runs, spawns shell
**Effort**: ~50 hours
**Risk**: High — busybox cross-compilation is the main unknown

#### Deliverables

- [ ] **Step 7.1** [W] (~60m): **Write /init program**

Minimal init (PID 1) in C, cross-compiled to MBC:

```c
/* init.c — UPC Linux init process (PID 1) */
#include <unistd.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <fcntl.h>

int main(void)
{
    int fd, pid, status;

    /* Open console */
    fd = open("/dev/console", O_RDWR);
    if (fd >= 0) {
        dup2(fd, 0);  /* stdin */
        dup2(fd, 1);  /* stdout */
        dup2(fd, 2);  /* stderr */
        if (fd > 2) close(fd);
    }

    write(1, "\nUPC Linux booted.\n", 19);

    while (1) {
        pid = vfork();
        if (pid == 0) {
            /* Child: exec shell */
            char *argv[] = { "/bin/sh", NULL };
            char *envp[] = { "HOME=/", "PATH=/bin", "TERM=vt100", NULL };
            execve("/bin/sh", argv, envp);
            _exit(1);
        }
        /* Parent: wait for shell to exit, then respawn */
        waitpid(pid, &status, 0);
        write(1, "\nShell exited. Respawning...\n", 28);
    }
    return 0;
}
```

Cross-compile:
```bash
riscv64-unknown-elf-gcc -march=rv32i -mabi=ilp32 -static \
    -nostdlib -o init init.c -lc
```

Convert to bFLT, translate to MBC.

---

- [ ] **Step 7.2** [W][P] (~180m): **Cross-compile busybox for MBC**

Option A — Full busybox (preferred):
```bash
# Configure busybox for minimal static build
make ARCH=riscv CROSS_COMPILE=riscv64-unknown-elf- defconfig
# Disable everything except: sh, echo, cat, ls, ps, mount, umount
make ARCH=riscv CROSS_COMPILE=riscv64-unknown-elf- \
    CFLAGS="-march=rv32i -mabi=ilp32 -static" \
    LDFLAGS="-march=rv32i -mabi=ilp32 -static" \
    -j$(nproc)
```

Then translate the resulting binary to MBC via `rv32i-to-mbc`.
Convert to bFLT format.

Expected busybox size: 800KB-1.5MB (RV32I static), ~1.3-2.4MB after MBC translation.

Option B — Hand-written minimal shell (fallback):
If busybox cross-compilation proves too difficult, write a minimal shell
in ~200 lines of C:
- Line reading from stdin
- Command parsing (split on spaces)
- Built-in: `echo`, `exit`, `cd`, `pwd`
- External: `vfork()` + `execve()` for running `/bin/ls`, `/bin/cat`
- Redirection: `>` and `<` via `dup2()`

---

- [ ] **Step 7.3** [W] (~30m): **Create /dev/console device node**

The console driver from Week 3 provides the backend. The device node
in initramfs connects `/dev/console` to major 5, minor 1 (standard Linux).

```bash
# In initramfs build:
mknod initramfs/dev/console c 5 1
```

---

- [ ] **Step 7.4** [B][V][C] (~120m): **Integration: shell prompt appears**

```bash
# Boot with initramfs containing init + shell
cargo run --release -p wotan-ctl -- boot \
    --kernel ~/src/linux-mbc/vmlinux.mbc \
    --ramdisk ~/src/linux-mbc/initramfs.cpio \
    --label linux-shell

# Expected:
# UPC Linux booted.
# / #
```

Debug cycle: if shell doesn't appear, check:
1. Does init run? (add debug prints)
2. Does vfork work? (check child created)
3. Does execve work? (check bFLT loading)
4. Does console input work? (check 0xFFFF reads)

**Exit Gate**: Shell prompt appears. Can type `echo hello` and see `hello`.

---

### Week 8: KERNEL PANIC IS SUCCESS

**Goal**: Linux kernel boots to panic or shell — either is a milestone
**Effort**: ~40 hours
**Risk**: Low (this is consolidation, not new features)

This is the checkpoint where we can say **"Linux boots on a protocol computer."**

#### Deliverables

- [ ] **Step 8.1** [V] (~60m): **End-to-end boot validation**

Full boot sequence from cold start:
1. `wotan-ctl boot --kernel vmlinux.mbc --ramdisk initramfs.cpio`
2. Kernel loads at 0x10000
3. `head.S` clears BSS, sets up stack, calls `start_kernel()`
4. `start_kernel()` initializes memory, console, timer, scheduler
5. Kernel mounts initramfs
6. Kernel execs `/init` (PID 1)
7. `/init` opens console, forks, execs `/bin/sh`
8. Shell prompt appears

Record the full console output. Measure time from boot command to shell prompt.

---

- [ ] **Step 8.2** [W] (~30m): **Document boot sequence**

Write `docs/doom/LINUX_BOOT_LOG.md` with:
- Full console output (copy-pasted from Busboy tty topic)
- Annotated timestamps for each boot phase
- Any error messages and their resolution
- Hardware/software versions used

---

- [ ] **Step 8.3** [V] (~30m): **Performance baseline**

Measure and record:
- Time from `wotan-ctl boot` to first printk message
- Time from first printk to "No init found" or shell prompt
- Total MBC instructions executed (from Wotan tick counter)
- Estimated instructions per second
- Memory usage (from `/proc/meminfo` if available)

Expected: 10-50 seconds to shell prompt (10-50M MBC instructions at ~1M IPS).

---

- [ ] **Step 8.4** [W] (~120m): **Create demo recording**

```bash
# Record terminal session
asciinema rec linux-boot-demo.cast

# Run boot sequence
# Show shell prompt
# Run echo, ls, cat
# Show /proc/version
# Exit cleanly

asciinema upload linux-boot-demo.cast
```

Also capture video for conference submission if applicable.

---

- [ ] **Step 8.5** [W][C] (~60m): **Write blog post draft**

Title: "I Built a Computer from a Network Protocol and Booted Linux on It"

Outline:
1. What is the Unheaded Protocol Computer? (Monad packets as CPU instructions)
2. The MBC ISA (43 opcodes, 16 registers, runs inside eBPF)
3. The Doom milestone (proving computational completeness)
4. The OS primitives (interrupts, syscalls, scheduler, MMU)
5. The Linux port (arch/mbc, nommu, bFLT)
6. The boot (console output, timestamps)
7. What's next (networking — IP inside Monad, the inception)

**Exit Gate**: Reproducible Linux boot on UPC. Blog post material written.
This is a career-defining demo.

---

### Week 9: Shell Utilities

**Goal**: Basic commands work
**Effort**: ~40 hours
**Risk**: Low-medium — mostly missing syscall stubs

#### Deliverables

- [ ] **Step 9.1** [W] (~60m): **Implement missing syscalls for ls**

`ls` needs: `openat`, `getdents64`, `fstat`, `write`, `close`.

- `sys_getdents64` — Read directory entries from ramfs/initramfs
- `sys_fstat` / `sys_newfstatat` — Return file metadata (size, permissions, timestamps)

These can be minimal stubs that return hardcoded values for the initramfs
filesystem (all files owned by root, permissions 0755).

---

- [ ] **Step 9.2** [W] (~30m): **Implement missing syscalls for cat**

`cat` needs: `openat`, `read`, `write`, `close`. Most of these already work
from the console driver. Extend to handle file fds (not just stdin/stdout).

---

- [ ] **Step 9.3** [W] (~30m): **Implement missing syscalls for echo**

`echo` is trivial — `write(1, ...)`. Already works if console driver is functional.
Handle the edge case of `echo -n` (no newline).

---

- [ ] **Step 9.4** [W] (~60m): **Implement missing syscalls for ps**

`ps` needs `/proc` filesystem:
- `/proc/[pid]/stat` — process status
- `/proc/[pid]/cmdline` — process command line
- `/proc/self/` — symlink to current process

Ensure `CONFIG_PROC_FS=y` and the procfs infrastructure works.
Most of this is generic kernel code; the arch part is providing
`task_pt_regs()` and process state accessors.

---

- [ ] **Step 9.5** [W] (~60m): **Implement /proc filesystem basics**

```
/proc/version    — "Linux version 6.8.0-mbc ..."
/proc/meminfo    — Memory usage (total, free, buffers, cached)
/proc/uptime     — Seconds since boot (jiffies / HZ)
/proc/cpuinfo    — "MBC (Monad Bytecode CPU)\nbogomips: ..."
/proc/stat       — CPU statistics (time in user, system, idle)
```

Most of these are generic and work automatically with `CONFIG_PROC_FS=y`.
`/proc/cpuinfo` needs `arch/mbc/kernel/setup.c` to register a seq_file handler.

---

- [ ] **Step 9.6** [W] (~30m): **Implement dup/dup2 syscalls**

Required for shell redirection (`echo hello > file`). Simple fd table operations:
- `sys_dup(oldfd)` — duplicate fd to lowest available
- `sys_dup2(oldfd, newfd)` — duplicate fd to specific number

These are generic kernel functions; ensure they're enabled and work with
our file descriptor table.

---

- [ ] **Step 9.7** [W] (~30m): **Implement pipe syscall**

Required for shell pipelines (`ls | cat`). Create a kernel pipe:
- `sys_pipe2(fds, flags)` — create pipe, return read/write fds

Generic kernel infrastructure handles this. Verify it works with our
scheduler (reader blocks until writer produces data).

---

- [ ] **Step 9.8** [W] (~30m): **Implement getcwd/chdir syscalls**

Required for `cd` and `pwd` commands:
- `sys_getcwd(buf, size)` — return current working directory path
- `sys_chdir(path)` — change current directory

Generic VFS handles these. Verify with ramfs/initramfs.

---

- [ ] **Step 9.9** [W] (~30m): **Implement trivial identity syscalls**

Stub implementations that return hardcoded values:
- `sys_getuid()` → 0 (root)
- `sys_getgid()` → 0 (root)
- `sys_geteuid()` → 0 (root)
- `sys_getegid()` → 0 (root)
- `sys_set_tid_address()` → current PID
- `sys_access()` / `sys_faccessat()` → 0 (always accessible)
- `sys_mprotect()` → 0 (no-op on nommu)
- `sys_writev()` → call write() in a loop

---

- [ ] **Step 9.10** [B][V][C] (~60m): **Integration: 5+ commands working**

Test each command:
```
/ # echo hello
hello
/ # ls /
bin  dev  etc  init  proc  sys  tmp
/ # cat /proc/version
Linux version 6.8.0-mbc (...)
/ # cat /proc/uptime
42.50 42.50
/ # ps
  PID  STAT COMMAND
    1  S    /init
    2  S    [kthreadd]
    3  R    /bin/sh
```

**Exit Gate**: Interactive shell with 5+ working commands.

---

## PHASE 4: HARDENING (Weeks 10-12)

### Week 10: Stability

**Goal**: No crashes during normal use
**Effort**: ~40 hours
**Risk**: Medium — bug-hunting is unpredictable

#### Deliverables

- [ ] **Step 10.1** [D] (~120m): **Fix all known kernel panics**

Systematic testing:
1. Boot 10 times in a row — any boot failures?
2. Run each command 20 times — any crashes?
3. Fork-bomb test (limited): `for i in 1 2 3 4; do echo $i & done` — does scheduler handle it?
4. Memory stress: allocate and free repeatedly
5. Signal test: Ctrl+C kills foreground process

Triage and fix each panic. Common causes:
- NULL pointer dereference (missing initialization)
- Stack overflow (kernel stack too small — increase THREAD_SIZE)
- Use-after-free (process exited but task_struct still referenced)
- Scheduler bug (wrong process selected after exit)

---

- [ ] **Step 10.2** [D] (~60m): **Memory leak detection**

Boot kernel, run commands for 5 minutes, check `/proc/meminfo`:
- Is free memory decreasing over time?
- Are slab caches growing unbounded?
- Does `kfree()` actually return memory?

Add `printk` instrumentation to `kmalloc`/`kfree` if needed.

---

- [ ] **Step 10.3** [W] (~60m): **Signal handling hardening**

Full signal support:
- `rt_sigaction()` — register signal handlers
- `rt_sigprocmask()` — block/unblock signals
- `sigreturn()` — return from signal handler
- Ctrl+C → SIGINT to foreground process group
- SIGSEGV on invalid memory access (if detectable)

---

- [ ] **Step 10.4** [W] (~30m): **Multi-process stability**

Test pattern:
```sh
# Fork + exec + wait
for i in 1 2 3 4 5; do
    echo "process $i"
done

# Background processes
echo one &
echo two &
wait
```

Fix any race conditions in scheduler or process table.

---

- [ ] **Step 10.5** [V][C] (~60m): **1-hour stress test**

Run automated test script that continuously:
- Executes shell commands
- Forks processes
- Reads /proc entries
- Allocates memory
- Sends signals

For 1 hour straight. No crashes = pass.

**Exit Gate**: 1-hour stress test without crash.

---

### Week 11: Performance

**Goal**: Usable interactive performance
**Effort**: ~35 hours
**Risk**: Medium — performance work is iterative

#### Deliverables

- [ ] **Step 11.1** [V] (~30m): **Profile boot time**

Instrument each boot phase:
```
[  0.000] head.S: entry
[  0.010] head.S: BSS cleared
[  0.015] head.S: stack set up
[  0.020] start_kernel() entered
[  0.100] memory init complete
[  0.150] console registered
[  0.200] timer init
[  0.300] VFS init
[  0.500] initramfs unpacked
[  0.700] /init started
[  1.000] shell prompt
```

Target: < 30 seconds to shell prompt.

---

- [ ] **Step 11.2** [V] (~30m): **Profile command latency**

Time each command:
```
time echo hello         # Target: < 100ms
time ls /               # Target: < 500ms
time cat /proc/version  # Target: < 200ms
time ps                 # Target: < 1s
```

---

- [ ] **Step 11.3** [W] (~120m): **Optimize hot paths**

Based on profiling (instruction counts per operation), optimize:

1. **Syscall entry/exit** — Minimize register saves. Only save caller-saved
   registers on syscall entry (callee-saved are preserved by C ABI).
2. **Timer interrupt** — Fast path: if no reschedule needed, skip scheduler_tick()
   overhead. Just increment jiffies and IRET.
3. **Console output** — Batch character writes. Instead of one Wotan write per byte,
   buffer 64 bytes and write in one BPF map operation.
4. **Context switch** — Only save/restore registers that differ between processes.
   Keep a dirty-register bitmap.
5. **Memory allocation** — Pre-allocate slab caches for common sizes.

---

- [ ] **Step 11.4** [W] (~60m): **Wotan/BPF optimizations**

1. **Instruction batching** — Execute multiple MBC instructions per BPF timer tick.
   Current: 1 instruction per tick. Target: 100-1000 instructions per tick.
   This is the single biggest performance lever.
2. **BPF map caching** — Cache frequently accessed RAM locations in BPF per-CPU
   variables instead of reading from the map every time.
3. **Tail call elimination** — Reduce tail-call overhead in the BPF execution loop.

---

- [ ] **Step 11.5** [V][C] (~30m): **Performance report**

Document before/after measurements. Write `docs/doom/LINUX_PERFORMANCE.md`.

**Exit Gate**: Subjectively "usable" interactive experience. Commands respond
within 1 second.

---

### Week 12: Polish + Documentation

**Goal**: Ready for public demo
**Effort**: ~35 hours
**Risk**: Low — this is writing, not coding

#### Deliverables

- [ ] **Step 12.1** [W] (~60m): **README for the Linux port**

`~/src/linux-mbc/README.mbc.md`:
- What is this? (Linux 6.x ported to MBC, a CPU that runs inside eBPF)
- Architecture overview (arch/mbc, nommu, bFLT, 43 opcodes)
- How it works (Monad packets → Wotan → BPF → MBC instructions → Linux)
- Build instructions
- Boot instructions
- Known limitations

---

- [ ] **Step 12.2** [W] (~60m): **Build instructions (from scratch)**

Step-by-step guide for someone with no context:

```
1. Install prerequisites
   - Rust toolchain (for Wotan, rv32i-to-mbc)
   - RISC-V cross-compiler (riscv64-unknown-elf-gcc)
   - Python 3 (for mbc-as assembler)
   - Linux kernel build deps (flex, bison, bc, etc.)

2. Build the UPC
   cd ~/tmp/unheaded
   cargo build --release -p wotan
   cargo build --release -p monad-mbc
   cargo build --release -p busboy

3. Build the Linux kernel
   cd ~/src/linux-mbc
   make ARCH=mbc CROSS_COMPILE=riscv64-unknown-elf- defconfig
   make ARCH=mbc CROSS_COMPILE=riscv64-unknown-elf- -j$(nproc)
   ./tools/mbc-translate-kernel.sh vmlinux vmlinux.mbc

4. Build initramfs
   ./tools/build-initramfs.sh

5. Boot
   cargo run --release -p wotan-ctl -- boot \
       --kernel vmlinux.mbc \
       --ramdisk initramfs.cpio \
       --label linux

6. Connect to console
   cargo run --release -p busboy -- subscribe compute.tty.linux
```

---

- [ ] **Step 12.3** [W] (~30m): **Boot demo script**

```bash
#!/bin/bash
# tools/demo-linux-boot.sh
# One-command Linux boot demo for conferences
set -euo pipefail

echo "=== UPC Linux Boot Demo ==="
echo "Building kernel..."
make -C ~/src/linux-mbc ARCH=mbc CROSS_COMPILE=riscv64-unknown-elf- -j$(nproc) -s
echo "Translating to MBC..."
~/src/linux-mbc/tools/mbc-translate-kernel.sh \
    ~/src/linux-mbc/vmlinux ~/src/linux-mbc/vmlinux.mbc
echo "Booting..."
cargo run --release -p wotan-ctl -- boot \
    --kernel ~/src/linux-mbc/vmlinux.mbc \
    --ramdisk ~/src/linux-mbc/initramfs.cpio \
    --label linux-demo &
sleep 1
echo "=== Console output ==="
cargo run --release -p busboy -- subscribe compute.tty.linux-demo
```

---

- [ ] **Step 12.4** [W] (~120m): **Conference talk abstract**

Title: "Linux on a Protocol Computer: Booting an OS Inside eBPF"

Abstract (250 words):
We present the Unheaded Protocol Computer (UPC), a Turing-complete CPU that
executes inside eBPF programs attached to network interfaces. The UPC's
instruction set — the Monad Bytecode CPU (MBC) — has 43 opcodes and 16
general-purpose registers. Programs are delivered as network packets using
the Monad wire protocol.

We demonstrate a complete port of Linux (uClinux, CONFIG_MMU=n) to the MBC
architecture. The kernel boots from a flat binary loaded into eBPF maps,
initializes a timer (via BPF timer callbacks), console (via Wotan message
bus), and scheduler (via BPF process table). An initramfs containing busybox
provides a functional shell with standard Unix utilities.

Key contributions:
1. A new Linux architecture port (arch/mbc) for a CPU with no physical hardware
2. A toolchain that compiles C via RISC-V cross-compilation and binary translation
3. Atomic operations via interrupt-disable sequences on a single-core BPF CPU
4. The first (to our knowledge) operating system booted inside eBPF

The system boots to a shell prompt in approximately N seconds, executing
approximately M million MBC instructions. Interactive commands (ls, cat, echo)
respond within 1 second. The entire system — CPU, memory, I/O — runs as
eBPF programs on a standard Linux host.

This work demonstrates that modern eBPF is computationally complete enough to
host an operating system, blurring the line between network processing and
general-purpose computation.

---

- [ ] **Step 12.5** [W] (~60m): **Finalize blog post**

Complete the draft from Week 8. Add:
- Actual performance numbers
- Boot log
- Screenshots
- Architecture diagram
- Link to source code
- Link to demo video

---

- [ ] **Step 12.6** [B] (~60m): **Record video of boot sequence**

Use `asciinema` or screen recording:
1. Show the terminal
2. Run the boot command
3. Watch kernel messages scroll
4. Shell prompt appears
5. Run `echo hello`, `ls /`, `cat /proc/version`, `ps`
6. Show `uname -a` output
7. Exit cleanly

Post to YouTube/Vimeo for conference submission.

---

- [ ] **Step 12.7** [V][C] (~60m): **Reproducibility test**

Give the build instructions to someone else (or a fresh VM). Can they:
1. Clone the repos?
2. Install dependencies?
3. Build the kernel?
4. Boot Linux on UPC?
5. Get a shell prompt?

Fix any gaps in documentation.

**Exit Gate**: Someone else can reproduce the Linux boot from the documentation.

---

## Appendix A: Toolchain Setup

### Prerequisites

| Tool | Version | Purpose | Install |
|------|---------|---------|---------|
| `riscv64-unknown-elf-gcc` | 13.x+ | C cross-compiler (RV32I target) | `apt install gcc-riscv64-unknown-elf` or build from source |
| `riscv64-unknown-elf-ld` | 2.41+ | Linker | Comes with GCC |
| `riscv64-unknown-elf-objcopy` | 2.41+ | Binary extraction | Comes with GCC |
| `rustc` | 1.75+ | Wotan, rv32i-to-mbc, monad-mbc | `rustup install stable` |
| `cargo` | 1.75+ | Rust build system | Comes with rustc |
| `python3` | 3.10+ | MBC assembler (`mbc-as`) | System package |
| `flex` | 2.6+ | Kernel build (Kconfig lexer) | `apt install flex` |
| `bison` | 3.8+ | Kernel build (Kconfig parser) | `apt install bison` |
| `bc` | 1.07+ | Kernel build (calculations) | `apt install bc` |
| `libelf-dev` | 0.189+ | Kernel build (BTF, objtool) | `apt install libelf-dev` |
| `cpio` | 2.13+ | Initramfs packing | `apt install cpio` |

### RV32I Cross-Compiler Setup

```bash
# Option 1: System package (Ubuntu/Debian)
sudo apt install gcc-riscv64-unknown-elf binutils-riscv64-unknown-elf

# Option 2: Build from source (if system package is too old)
git clone https://github.com/riscv-collab/riscv-gnu-toolchain.git
cd riscv-gnu-toolchain
./configure --prefix=/opt/riscv --with-arch=rv32i --with-abi=ilp32
make -j$(nproc)
export PATH=/opt/riscv/bin:$PATH

# Verify
riscv64-unknown-elf-gcc -march=rv32i -mabi=ilp32 -v
```

### RV32I-to-MBC Translator

```bash
cd ~/tmp/unheaded
cargo build --release -p monad-mbc --bin rv32i-to-mbc

# Verify
cargo run --release -p monad-mbc --bin rv32i-to-mbc -- --help
```

### MBC Assembler (mbc-as)

```bash
# Located at tools/mbc-as/mbc_as.py
# Must be built as part of HB-1 resolution

python3 tools/mbc-as/mbc_as.py --help
echo "NOP" | python3 tools/mbc-as/mbc_as.py -o /dev/stdout | xxd
```

### Kernel Build Verification

```bash
cd ~/src/linux-mbc
make ARCH=mbc CROSS_COMPILE=riscv64-unknown-elf- defconfig
make ARCH=mbc CROSS_COMPILE=riscv64-unknown-elf- -j$(nproc)
file vmlinux
# Expected: ELF 32-bit LSB executable, UCB RISC-V, ...
```

### End-to-End Translation Pipeline

```
C source code
    │
    ▼ riscv64-unknown-elf-gcc -march=rv32i
RV32I ELF object (.o)
    │
    ▼ riscv64-unknown-elf-ld
RV32I ELF executable (vmlinux)
    │
    ▼ riscv64-unknown-elf-objcopy -O binary
RV32I flat binary (.bin)
    │
    ▼ rv32i-to-mbc translator
MBC flat binary (.mbc)
    │
    ▼ Merge with mbc-as output (head.S, entry.S)
Final MBC kernel image (vmlinux.mbc)
    │
    ▼ wotan-ctl boot --kernel
Loaded into Wotan RAM_MAP at 0x10000
    │
    ▼ BPF execution loop
Linux boots on UPC
```

---

## Appendix B: Kernel Configuration (Full defconfig)

```
#
# arch/mbc/configs/mbc_defconfig
# Minimal uClinux configuration for MBC/UPC
# Generated: 2026-03-15
#

# ── Architecture ──────────────────────────────────────────
CONFIG_MBC=y
CONFIG_CPU_MBC=y
CONFIG_MMU=n
CONFIG_32BIT=y
CONFIG_SMP=n
CONFIG_NR_CPUS=1
CONFIG_EMBEDDED=y
CONFIG_EXPERT=y

# ── Memory ────────────────────────────────────────────────
CONFIG_FLAT_MEM=y
CONFIG_DRAM_BASE=0x00000000
CONFIG_DRAM_SIZE=0x04000000
CONFIG_PAGE_OFFSET=0x00000000
CONFIG_FORCE_MAX_ZONEORDER=11
CONFIG_SELECT_MEMORY_MODEL=y

# ── Binary format ─────────────────────────────────────────
CONFIG_BINFMT_FLAT=y
CONFIG_BINFMT_FLAT_ARGVP_ENVP_ON_STACK=y
CONFIG_BINFMT_FLAT_OLD_ALWAYS_RAM=y
# CONFIG_BINFMT_ELF_FDPIC is not set
# CONFIG_BINFMT_MISC is not set
# CONFIG_BINFMT_SCRIPT is not set
# CONFIG_COREDUMP is not set

# ── Console/Serial ────────────────────────────────────────
CONFIG_SERIAL_CORE=y
CONFIG_SERIAL_CORE_CONSOLE=y
CONFIG_SERIAL_MBC=y
CONFIG_SERIAL_MBC_CONSOLE=y
CONFIG_CONSOLE_POLL=y
CONFIG_TTY=y
CONFIG_VT=n
CONFIG_HW_CONSOLE=n

# ── Filesystem ────────────────────────────────────────────
CONFIG_TMPFS=y
CONFIG_PROC_FS=y
CONFIG_PROC_SYSCTL=y
CONFIG_SYSFS=y
CONFIG_DEVTMPFS=y
CONFIG_DEVTMPFS_MOUNT=y
CONFIG_RAMFS=y

# ── Root filesystem (initramfs) ───────────────────────────
CONFIG_BLK_DEV_INITRD=y
CONFIG_INITRAMFS_SOURCE="initramfs.cpio"
CONFIG_INITRAMFS_COMPRESSION_NONE=y

# ── Block device ──────────────────────────────────────────
CONFIG_BLOCK=y
CONFIG_BLK_DEV=y
CONFIG_BLK_DEV_RAM=y
CONFIG_BLK_DEV_RAM_COUNT=1
CONFIG_BLK_DEV_RAM_SIZE=4096

# ── Scheduler ─────────────────────────────────────────────
CONFIG_PREEMPT_NONE=y
CONFIG_HZ_12=y
CONFIG_HZ=12
CONFIG_SCHED_DEBUG=n
CONFIG_SCHEDSTATS=n

# ── Debug (enable for bringup) ────────────────────────────
CONFIG_PRINTK=y
CONFIG_EARLY_PRINTK=y
CONFIG_DEBUG_INFO=y
CONFIG_DEBUG_KERNEL=y
CONFIG_PANIC_ON_OOPS=y
CONFIG_PANIC_TIMEOUT=0
CONFIG_KALLSYMS=y
CONFIG_KALLSYMS_ALL=y
CONFIG_STACKTRACE=y
CONFIG_DEBUG_BUGVERBOSE=y
CONFIG_MAGIC_SYSRQ=n

# ── Disable unnecessary features ─────────────────────────
CONFIG_MODULES=n
CONFIG_NETWORKING=n
CONFIG_SOUND=n
CONFIG_USB=n
CONFIG_INPUT=n
CONFIG_FB=n
CONFIG_VGA_CONSOLE=n
CONFIG_FUTEX=n
CONFIG_EPOLL=n
CONFIG_SIGNALFD=n
CONFIG_TIMERFD=n
CONFIG_EVENTFD=n
CONFIG_AIO=n
CONFIG_IO_URING=n
CONFIG_INOTIFY_USER=n
CONFIG_FHANDLE=n
CONFIG_SWAP=n
CONFIG_SYSVIPC=n
CONFIG_POSIX_MQUEUE=n
CONFIG_CROSS_MEMORY_ATTACH=n
CONFIG_USELIB=n
CONFIG_AUDIT=n
CONFIG_SECURITY=n
CONFIG_SECCOMP=n
CONFIG_MULTIUSER=y
CONFIG_PRINTK_TIME=y

# ── Crypto (minimal for kernel internals) ─────────────────
CONFIG_CRYPTO=n
```

**Expected image sizes:**
- `vmlinux` (RV32I ELF): ~800KB-1.5MB
- `vmlinux.mbc` (MBC flat binary): ~1.3-2.4MB (1.6x inflation from translation)
- `initramfs.cpio`: ~200KB-500KB (static busybox + init + dev nodes)
- Total memory footprint: ~3-5MB of 64MB available

---

## Appendix C: Risk Register

| # | Risk | Probability | Impact | Mitigation | Contingency | Week |
|---|------|------------|--------|------------|-------------|------|
| R1 | **BPF verifier rejects kernel execution paths** — Linux interrupt entry/exit is deeper than Doom's loop. BPF verifier may reject due to complexity or instruction count. | Medium (40%) | High — blocks all progress | Split execution into tail-called BPF sub-programs (proven pattern from Doom). Pre-test each kernel path in isolation. | Restructure as 4-5 tail-called programs: boot, syscall, timer, scheduler, I/O. Add 1-2 weeks. | 2-3 |
| R2 | **RV32I-to-MBC translator chokes on kernel code patterns** — Kernel uses computed gotos (`switch` dispatch tables), per-CPU variables, section attributes, and inline asm not seen in Doom. | Medium (50%) | Medium — delays but doesn't block | Run translator on kernel objects early (Week 1). Fix translator bugs as found. Maintain a "kernel patterns" test suite. | Manually translate problematic functions. Budget 1 week of translator fixes. | 1-3 |
| R3 | **execve() bFLT loading fails** — bFLT relocation is subtle. Off-by-one in relocation application corrupts all pointers. | High (60%) | High — blocks shell | Start with a trivial test binary (no relocations). Add relocations incrementally. Test with `readelf`-equivalent for bFLT. | Fall back to raw flat binary loading (no relocations) for initial demo. | 4, 6 |
| R4 | **Busybox cross-compilation fails** — Busybox may use GCC extensions or libc features not available in our RV32I + newlib setup. | Medium (40%) | Medium — no shell utils | Use musl-libc instead of newlib (better POSIX compliance). Or write minimal hand-coded utilities (50-100 LOC each). | Hand-write a minimal shell (200 LOC C). Provide echo, ls, cat as separate binaries. | 7 |
| R5 | **Memory exhaustion** — Kernel + initramfs + busybox + process stacks exceed comfortable BPF map sizes. | Low (20%) | High — OOM panics | Monitor memory usage from Week 3. Trim kernel config aggressively (disable all optional features). Use XIP (execute-in-place) for busybox if possible. | Increase Wotan RAM_MAP size. Current 64MB is generous; could go to 128MB. | 5-7 |
| R6 | **Performance too slow for interactive use** — Each MBC instruction takes ~1us. Shell command may need 10M+ instructions = 10+ seconds. | Medium (50%) | Low — demo still works, just slow | Instruction batching: execute 100-1000 instructions per BPF tick. This is the single biggest performance lever. | Accept slow performance for initial demo. Optimize in Phase 4. Record demo at 4x speed. | 8-11 |
| R7 | **Console input not working** — TTY input (keyboard → Busboy → 0xFFFF register) may have buffering/timing issues. | Medium (30%) | High — no interactive shell | Test console input in isolation before kernel integration. Ensure Busboy-to-input-register path works with existing Doom keyboard input. | Pipe commands via boot args instead of interactive shell. | 3, 7 |
| R8 | **Timer interrupt race conditions** — Timer fires during syscall handling, corrupting kernel state. | Medium (40%) | High — random crashes | Disable interrupts during critical sections (CLI/STI around kernel data structure modifications). Verify interrupt nesting is handled correctly in entry.S. | Add interrupt-depth counter. Panic on nested interrupts during bringup. | 3-4 |
| R9 | **Linker script produces wrong layout** — Kernel sections at wrong addresses, BSS overlaps data, init sections missing. | Medium (30%) | Medium — boot fails | Use `objdump -h vmlinux` to verify section layout after every linker script change. Compare with working Microblaze vmlinux. | Simplify linker script to absolute minimum (text, data, bss only). | 1-2 |
| R10 | **vfork() semantics wrong** — Parent resumes too early (before child execs), corrupting shared address space. | Medium (40%) | High — fork+exec broken | Implement vfork() completion mechanism carefully. Test with debug prints showing exact sequence: parent blocks → child runs → child execs → parent resumes. | Use clone(CLONE_VM) instead of vfork(). Simpler but still correct for nommu. | 4 |
| R11 | **Signal delivery crashes** — Signal frame setup on user stack is wrong. sigreturn corrupts registers. | Medium (40%) | Medium — no Ctrl+C | Test signal delivery in isolation. Start with SIGCHLD only (simplest). Dump signal frame contents for debugging. | Disable all signals for initial demo. Handle SIGCHLD in kernel only (no user-space signal handlers). | 4, 10 |
| R12 | **MBC assembler bugs** — Assembler emits wrong opcodes or wrong encoding for edge cases (large immediates, branch offsets). | Medium (30%) | High — boot code broken | Comprehensive test suite for assembler. Compare output against hand-encoded known-good binaries. Disassemble output and verify. | Hand-encode critical sections as `.word` directives in C. Ugly but guaranteed correct. | 1-2 |

### Risk Summary

- **High probability (>50%)**: R3 (execve/bFLT)
- **High impact**: R1 (BPF verifier), R3 (execve), R5 (OOM), R7 (console input), R8 (timer races), R10 (vfork), R12 (assembler)
- **Critical path risks**: R1, R3, R10 — these block the shell prompt milestone
- **Mitigable risks**: R2, R4, R6, R9 — workarounds exist that add 1-2 weeks

**Overall schedule confidence**: 70% chance of completing in 12 weeks. 95% chance in 14 weeks (with 2-week buffer).

---

## Appendix D: Agent Assignment Matrix

### Parallelization Opportunities

```
Week 1  [SEQUENTIAL]  — Kernel skeleton (single agent, foundation)
Week 2  [SEQUENTIAL]  — Boot code (depends on Week 1)
Week 3  [SEMI-PARALLEL]
        Agent A: Timer driver (time.c)
        Agent B: Console driver (mbc_console.c)
        Integration: Both agents merge, test together

Week 4  [PARALLEL — 3 agents]
        Agent A: Process management (process.c, switch_to)
        Agent B: execve/bFLT loader (binfmt_flat support)
        Agent C: vfork implementation (scheduler extension)
        Integration: Merge all, test fork+exec+wait

Week 5  [SEQUENTIAL]  — Memory management (depends on Week 4)
Week 6  [PARALLEL — 2 agents]
        Agent A: Block device driver (mbc_ramdisk.c)
        Agent B: Initramfs build (cross-compile busybox, create cpio)
        Integration: Boot with initramfs

Week 7  [PARALLEL — 2 agents]
        Agent A: Init process + shell compilation
        Agent B: Missing syscall stubs (dup, pipe, getcwd, etc.)
        Integration: Shell prompt test

Week 8  [SEQUENTIAL]  — Integration, debug, demo (single agent focus)
Week 9  [PARALLEL — 3 agents]
        Agent A: File-related syscalls (stat, getdents, openat)
        Agent B: /proc filesystem entries
        Agent C: Identity syscalls (getuid, access, mprotect stubs)
        Integration: Test all commands

Week 10 [SEQUENTIAL]  — Stability (single agent, systematic debugging)
Week 11 [PARALLEL — 2 agents]
        Agent A: Kernel/arch optimization (entry.S, timer, console)
        Agent B: Wotan/BPF optimization (instruction batching, caching)
        Integration: Performance benchmarks

Week 12 [PARALLEL — 3 agents]
        Agent A: Documentation (README, build instructions)
        Agent B: Conference materials (abstract, blog post)
        Agent C: Demo recording + reproducibility test
        Integration: Final review
```

### Agent Capacity Planning

| Week | Agents | Parallel? | Notes |
|------|--------|-----------|-------|
| 1 | 1 | No | Foundation must be sequential |
| 2 | 1 | No | Assembly requires deep focus |
| 3 | 2 | Semi | Timer + console are independent |
| 4 | 3 | Yes | Process, execve, vfork are independent |
| 5 | 1 | No | Memory depends on process model |
| 6 | 2 | Yes | Block device and initramfs are independent |
| 7 | 2 | Yes | Init and syscall stubs are independent |
| 8 | 1 | No | Integration requires single focus |
| 9 | 3 | Yes | Syscall groups are independent |
| 10 | 1 | No | Debugging requires single focus |
| 11 | 2 | Yes | Kernel and BPF optimization are independent |
| 12 | 3 | Yes | Docs, marketing, testing are independent |

**Peak parallelism**: 3 agents (Weeks 4, 9, 12)
**Total agent-weeks**: ~18 (vs. 12 calendar weeks)
**Single-agent critical path**: Weeks 1, 2, 5, 8, 10 (7 weeks sequential)

### Agent Skill Requirements

| Agent Role | Skills | Weeks |
|-----------|--------|-------|
| **Kernel Architect** | Linux kernel internals, nommu, assembly | 1-5, 8, 10 |
| **Systems Programmer** | C, cross-compilation, binary formats | 4, 6-7, 9 |
| **BPF Engineer** | eBPF, Rust, Wotan internals | 3, 11 |
| **Documentation** | Technical writing, diagrams, video | 8, 12 |

---

## Appendix E: Hardware Upgrade Recommendations

### Current Setup (WEST)

| Component | Spec | Bottleneck? |
|-----------|------|-------------|
| CPU | AMD Ryzen (RX 7700 XT system) | No — kernel build is I/O bound |
| RAM | 16GB DDR5 | **Maybe** — kernel build + Wotan + BPF maps |
| Storage | 1TB NVMe + 2TB HDD | No — NVMe is fast enough |
| GPU | RX 7700 XT | Irrelevant for this workload |

### Recommended Upgrades

| Priority | Upgrade | Cost | Impact | Rationale |
|----------|---------|------|--------|-----------|
| **P0** | RAM: 16GB → 32GB DDR5 | ~$60 | High | Kernel build (make -j) + Wotan BPF maps + multiple agent terminals. 16GB is tight. |
| **P1** | EAST as build server | $0 (already owned) | Medium | Offload kernel builds to EAST via `ssh govan@east make -j4`. Frees WEST for Wotan/BPF. |
| **P2** | Second monitor | ~$150 | Medium | Console output on one screen, code on the other. Essential for boot debugging. |
| **P3** | RAM: 32GB → 64GB DDR5 | ~$120 | Low | Only needed if BPF map sizes become a bottleneck (unlikely at 64MB RAM_MAP). |

### Development Environment Optimization

```bash
# Kernel build: use ccache for faster rebuilds
sudo apt install ccache
export CC="ccache riscv64-unknown-elf-gcc"

# Kernel build: use EAST as distcc server
sudo apt install distcc
export DISTCC_HOSTS="localhost east"
make ARCH=mbc CC="distcc riscv64-unknown-elf-gcc" -j8

# BPF development: increase locked memory limit
sudo sysctl -w kernel.unprivileged_bpf_disabled=0
ulimit -l unlimited

# Wotan: pre-allocate BPF maps for faster boot
cargo run --release -p wotan-ctl -- preallocate --ram-size 67108864
```

### CI/CD Recommendations

| Phase | Tool | Purpose |
|-------|------|---------|
| Build | GitHub Actions | `make ARCH=mbc` on every push to mbc-port branch |
| Test | Custom runner (WEST) | Boot kernel on UPC, run test suite, collect console output |
| Benchmark | Custom runner (WEST) | Measure boot time, command latency, memory usage |
| Release | GitHub Releases | Tag + binary kernel image + initramfs |

---

## Timeline Summary

```
         PHASE 1: FOUNDATION          PHASE 2: PROCESS       PHASE 3: USERSPACE    PHASE 4: HARDENING
    ┌──────────────────────────┐  ┌────────────────────┐  ┌──────────────────┐  ┌──────────────────┐
    │ W1: Skeleton  ░░░░░░░░░ │  │ W4: Process ░░░░░░ │  │ W7: Shell  ░░░░░ │  │ W10: Stability   │
    │ W2: Boot code ░░░░░░░░░ │  │ W5: Memory  ░░░░░░ │  │ W8: BOOT!! ░░░░░ │  │ W11: Performance │
    │ W3: Timer+Con ░░░░░░░░░ │  │ W6: FS+Init ░░░░░░ │  │ W9: Utils  ░░░░░ │  │ W12: Polish+Docs │
    └──────────────────────────┘  └────────────────────┘  └──────────────────┘  └──────────────────┘
                │                           │                       │                      │
                ▼                           ▼                       ▼                      ▼
         "Kernel panic:              fork()+exec()           Shell prompt!         Public demo
          No init found"               works                  "/ # _"              ready
         (MILESTONE 1)             (MILESTONE 2)          (MILESTONE 3)        (MILESTONE 4)
```

**Calendar**: Weeks 1-12 from the date all hard blockers are resolved.
**Effort**: ~500 hours total (~40 hours/week average).
**Confidence**: 70% in 12 weeks, 95% in 14 weeks.

---

*This battle plan is a practical roadmap, not a guarantee. The estimates are
honest: 12 weeks of hard kernel work by someone who has never ported Linux to
a new architecture before. The UPC already has all the hardware primitives
(timer, syscalls, scheduler, block device, console) that Linux needs. The gap
is software: headers, assembly stubs, and syscall implementations. The hardest
single piece is `execve()`. Everything else is tedious but straightforward.*

*The payoff is extraordinary: **Linux running on a CPU made of network packets.**
That sentence alone is worth a conference talk, a blog post, and a career story.*

*SPDX-License-Identifier: GPL-3.0-or-later*
