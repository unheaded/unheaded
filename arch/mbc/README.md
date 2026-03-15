# arch/mbc — Linux kernel port for the MBC architecture

## Status: Week 1 Skeleton (S-LINUX)

This is the beginning of the Linux kernel architecture port for the
**MBC (Monad Bytecode)** ISA, the instruction set of the **Unheaded
Protocol Computer (UPC)**.

### What this is

A compilation skeleton — directory structure, Kconfig, headers, and
kernel stubs that follow Linux kernel conventions.  This is preparation
for transplanting into a real kernel source tree.

### What this is NOT

A working kernel.  The entry.S is pseudocode, the syscall table is
not wired to the kernel's generic syscall infrastructure, and there
is no interrupt controller driver.

### Architecture summary

| Property | Value |
|----------|-------|
| ISA | MBC (Monad Bytecode) |
| Word size | 32-bit |
| Registers | r0-r15 (r15 = SP) |
| Page size | 4 KB |
| MMU | Software (BPF TLB maps), nommu default |
| Syscall vector | INT 0x80 |
| Syscall convention | r0=nr, r1-r5=args, r0=return |
| Implemented syscalls | 47 (Linux-compatible numbers) |
| Endianness | Little-endian |

### File layout

```
arch/mbc/
  Kconfig              Architecture and CPU config
  Makefile             Top-level kbuild integration
  README.md            This file
  include/asm/
    unistd.h           Syscall numbers (47 calls)
    ptrace.h           Register layout (pt_regs)
    page.h             Page size definitions
    setup.h            Boot parameter structure
    io.h               Memory-mapped I/O macros
    irq.h              Interrupt definitions
  kernel/
    Makefile           Kernel object list
    setup.c            setup_arch() — boot initialization
    process.c          start_thread() — process startup
    entry.S            Syscall/IRQ entry (stub)
  boot/dts/
    upc.dts            Device tree for base UPC hardware
```

### Next steps (Week 2+)

- Wire syscall table to generic Linux syscall dispatch
- Implement context switch in entry.S (MBC assembly)
- Add thread_info and kernel stack layout
- Timer driver (clocksource/clockevent)
- Console driver (UPC TTY map)
- bFLT binary loader integration (nommu)
- Ramdisk block driver

### License

GPL-2.0 (Linux kernel license)

### References

- `docs/UPC_REFERENCE_MANUAL.md` — full MBC ISA specification
- `docs/doom/UCLINUX_PORT_GUIDE.md` — uClinux porting notes
- `crates/monad-mbc/` — reference MBC implementation (Rust)
- `ebpf/monad-common/src/lib.rs` — canonical syscall numbers
