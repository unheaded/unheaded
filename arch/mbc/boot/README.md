# MBC Linux Boot — Unheaded Protocol Computer

## Overview

This directory contains everything needed to boot MBC Linux on the UPC:

| File | Purpose |
|------|---------|
| `init/init.S` | PID 1 init process (MBC assembly source) |
| `init/init.c` | PID 1 init process (C reference) |
| `init/build.go` | Builds init.upcf (UPCFlat binary) |
| `mkrootfs.go` | Creates rootfs.img (UNFS filesystem) |
| `rootfs.img` | Root filesystem image (generated) |
| `dts/upc.dts` | Device tree source |
| `Makefile` | Kernel boot image targets |

## Quick Start

From the project root (`~/tmp/unheaded/`):

```bash
./scripts/boot-linux.sh
```

This runs all four build steps automatically.

## Manual Build Steps

### 1. Build the init process

```bash
cd arch/mbc/boot/init
go run build.go
# -> init.upcf
```

### 2. Build the shell

```bash
cd demos/mbc/shell
go run build.go
# -> shell.upcf
```

### 3. Create the root filesystem

```bash
go run arch/mbc/boot/mkrootfs.go
# -> arch/mbc/boot/rootfs.img
```

### 4. Boot

```bash
wotan-ctl boot \
  --kernel arch/mbc/boot/init/init.upcf \
  --ramdisk arch/mbc/boot/rootfs.img \
  --args "console=ttyMBC0 root=/dev/ram0"
```

## Boot Sequence

1. UPC bootloader loads kernel at 0x00010000
2. `head.S:_start` runs: disables IRQs, sets up stack, clears BSS, calls `start_kernel()`
3. Kernel initializes: IRQs, timer, console, memory, process table
4. Kernel mounts initrd as root filesystem (UNFS on /dev/ram0)
5. Kernel executes `/init` (PID 1)
6. Init prints banner, forks child, child exec's `/bin/sh`
7. Shell prints `> ` prompt, reads commands, echos output
8. If shell exits, init respawns it

## Root Filesystem Layout (UNFS)

```
/
├── init            Executable (UPCFlat) — PID 1 init process
├── bin/
│   └── sh          Executable (UPCFlat) — interactive shell
├── dev/
│   └── console     Device node — console I/O
└── etc/
    └── hostname    Text file — "mbc-linux\n"
```

## Binary Format

All executables use **UPCFlat** format (simplified bFLT for nommu):

- 32-byte header: magic "UPCF", text/data sizes, entry point, stack size
- Text segment: MBC instructions (word-aligned)
- Data segment: strings, constants
- BSS: zero-filled at load time

See `pkg/upc/bflt.go` for the format specification.

## Kernel Configuration

The default config is at `arch/mbc/configs/upc_defconfig`. Key settings:

- `CONFIG_MMU` is not set (nommu)
- `CONFIG_BINFMT_FLAT=y` (UPCFlat binary loader)
- `CONFIG_BLK_DEV_INITRD=y` (ramdisk root)
- `CONFIG_SERIAL_MBC_CONSOLE=y` (MBC console driver)
