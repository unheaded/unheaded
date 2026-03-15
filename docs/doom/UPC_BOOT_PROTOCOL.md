# UPC Boot Protocol v1

*SPDX-License-Identifier: GPL-3.0-or-later*

## Overview

The UPC (Unheaded Protocol Computer) boot protocol defines how a kernel image
is loaded into RAM and how the execution environment is initialized before
the first instruction runs.

The boot process is managed by the "Computermancer" — the `wotan-ctl boot`
command — which prepares BPF maps with the initial memory state and CPU
configuration.

## Boot Sequence

```
1. Computermancer loads kernel image into RAM_MAP at 0x10000
2. Initializes IVT (interrupt vector table at 0x0000)
3. Writes default HLT handler at 0x0400
4. Writes BootParams structure at 0x0100
5. Optionally loads ramdisk at 0x800000
6. Optionally stores kernel command line at 0x0200
7. Sets stack pointer (r15) to 0x03F00000
8. Sets PC to 0x10000 (kernel entry point)
9. Starts execution (inject ticks)
```

## Memory Layout

```
Byte Address    Word Address    Size        Contents
─────────────────────────────────────────────────────────
0x0000-0x03FF   0x0000-0x00FF   1 KB        IVT (256 vectors x 4 bytes)
0x0100-0x01FF   0x0040-0x007F   256 B       BootParams structure
0x0200-0x03FF   0x0080-0x00FF   512 B       Kernel command line (boot args)
0x0400-0x0403   0x0100          4 B         Default HLT handler
0x0404-0xFFFF   0x0101-0x3FFF   ~60 KB      Kernel stack + early heap
0x10000+        0x4000+         variable    Kernel image
0x800000+       0x200000+       variable    Ramdisk (initrd/initramfs)
```

**Note:** The IVT and BootParams regions overlap in byte-address space
(0x0000-0x03FF for IVT, 0x0100-0x01FF for params). This is intentional:
the IVT uses word addresses 0x00-0xFF (vectors 0-255), while BootParams
uses word addresses 0x40-0x7F. The kernel must read BootParams from the
word-addressed RAM_MAP, not by treating IVT entries as params.

## BootParams Structure

Written at word address 0x40 (byte address 0x0100). The kernel locates
this structure by checking for the magic value.

```
Offset  Field           Type    Description
──────────────────────────────────────────────────
0x00    Magic           u32     0x554E4844 ("UNHD")
0x04    Version         u32     Boot protocol version (1)
0x08    MemorySize      u32     Total RAM in bytes (default: 64 MB)
0x0C    RamdiskAddr     u32     Ramdisk physical byte address
0x10    RamdiskSize     u32     Ramdisk size in bytes
0x14    KernelAddr      u32     Kernel load byte address (0x10000)
0x18    KernelSize      u32     Kernel size in bytes
0x1C    BootArgsAddr    u32     Command line byte address (0x0200)
0x20    BootArgsLen     u32     Command line length
0x24    NumCPUs         u32     Number of CPU instances (1)
0x28    TickRateHz      u32     Timer interrupt rate in Hz (12)
0x2C    Reserved[20]    u32[]   Reserved for future use (zero-filled)
```

### Magic Value

The magic value `0x554E4844` encodes "UNHD" (Unheaded) in ASCII:
- `U` = 0x55, `N` = 0x4E, `H` = 0x48, `D` = 0x44
- Stored as a single little-endian u32

A kernel should verify this magic before trusting the BootParams.

## Interrupt Vector Table (IVT)

The IVT occupies the first 256 words of RAM (word addresses 0x00-0xFF).
Each entry is a 4-byte handler address.

```
Vector   Byte Offset   Default Use
─────────────────────────────────────
0x00     0x000         Divide by zero
0x01     0x004         Debug/single-step
0x06     0x018         Invalid opcode
0x0E     0x038         Page fault
0x20     0x080         Timer interrupt
0x21     0x084         Keyboard/input interrupt
0x22     0x088         Disk complete interrupt
0x80     0x200         Syscall (INT 0x80)
```

All vectors default to 0x0400 (the HLT handler) until the kernel
installs its own interrupt service routines.

## CPU State at Boot

When the boot protocol completes:

| Register | Value | Description |
|----------|-------|-------------|
| PC | 0x10000 | Kernel entry point (word addr: 0x4000) |
| r15 (SP) | 0x03F00000 | Stack pointer (grows downward) |
| r0-r14 | 0 | General-purpose registers cleared |
| Flags | 0 | All flags cleared |
| Halted | 0 | CPU is running |
| NumProcesses | 1 | Single process |
| InterruptsEnabled | 0 | Interrupts disabled at boot |

The kernel is responsible for:
1. Reading BootParams to discover memory layout
2. Installing interrupt handlers in the IVT
3. Enabling interrupts when ready
4. Setting up its own page tables (if MMU is desired)

## Creating a Bootable Kernel

### Minimal Example

A bootable UPC kernel is a flat binary loaded at 0x10000. The simplest
kernel is two instructions:

```asm
; hello_kernel.mbc.s
MOVI r0, 42    ; 0x0F00002A
HLT            ; 0xFF000000
```

### Using wotan-ctl

```bash
# Boot a kernel image
wotan-ctl boot --kernel demos/mbc/hello_kernel.mbc

# Boot with ramdisk and command line
wotan-ctl boot --kernel kernel.mbc --ramdisk initrd.img --args "console=tty0"

# Dry-run (show plan without writing)
wotan-ctl boot --kernel kernel.mbc --dry-run
```

### Kernel Startup Checklist

1. Read BootParams at word address 0x40, verify magic = 0x554E4844
2. Note MemorySize, RamdiskAddr, RamdiskSize for memory management
3. Install IVT handlers (at minimum: timer 0x20, syscall 0x80)
4. Set up stack/heap based on MemorySize
5. Enable interrupts
6. Begin normal execution

## Go API

The `pkg/upc` package provides the boot infrastructure:

```go
import "unheaded/pkg/upc"

// Get default boot config
cfg := upc.DefaultBootConfig()
cfg.KernelPath = "kernel.mbc"
cfg.BootArgs = "console=tty0"

// Prepare memory map (word_address -> value)
mem, err := upc.PrepareBootMemory(cfg)

// Write to BPF maps
for addr, val := range mem {
    ramMap.Update(addr, val)
}
```

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1 | 2026-03-15 | Initial boot protocol specification |
