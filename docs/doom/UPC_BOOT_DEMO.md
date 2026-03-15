# UPC Boot Demo

## Overview

The UPC Boot Demo exercises the full FUZIX boot sequence on the Unheaded
Protocol Computer. It validates that all the OS primitives — syscalls,
filesystem, scheduler, and boot protocol — work together to boot a
Unix-like environment.

## Components

### boot_demo.bin (Kernel Image)

A combined MBC binary containing:

- **init** — the first process (/init), entry point at ROM word 0
- **shell** — a minimal shell (/bin/sh), appended after init in the ROM

The init program follows the classic Unix System V init sequence:

1. Print `UPC Boot v1.0`
2. Call `SYS_FORK` to create a child process
3. **Child**: call `SYS_EXECVE` to replace itself with /bin/sh
4. **Parent**: call `SYS_WAITPID` to wait for the child (shell) to exit
5. When child exits: print `Shell exited`, then `Init complete — system halted.`
6. Call `SYS_EXIT(0)` to halt cleanly

### ramdisk.img (UNFS Filesystem)

A 4 MB UNFS filesystem image containing:

| Path            | Type   | Description                         |
|-----------------|--------|-------------------------------------|
| `/init`         | file   | Init program (MBC binary)           |
| `/bin/sh`       | file   | Shell program (MBC binary)          |
| `/dev/console`  | device | Console device node                 |
| `/etc/hostname` | file   | Contains `upc\n`                    |

The UNFS (Unheaded Nano Filesystem) is a minimal flat filesystem with:
- 512-byte blocks
- 504 inodes (8 per block, 63 inode blocks)
- Contiguous block allocation
- Superblock at block 0 with magic `0x554E4653` ("UNFS")

## Building

```bash
cd demos/mbc/boot_demo
go run build.go
```

This produces:
- `boot_demo.bin` — kernel image
- `ramdisk.img` — UNFS filesystem

## Running

```bash
wotan-ctl boot --kernel boot_demo.bin --ramdisk ramdisk.img
```

## Expected Output

```
UPC Boot v1.0
> Goodbye.
Shell exited
Init complete — system halted.
```

The shell reads from stdin. With no input available (EOF), it prints
`Goodbye.` and exits, causing the parent (init) to continue.

With interactive input, the shell echoes each character:

```
UPC Boot v1.0
> hello
Shell exited
Init complete — system halted.
```

## Testing

```bash
go test ./pkg/upc/... -run TestBoot -v
```

The boot test suite includes:

| Test                          | What it validates                                   |
|-------------------------------|-----------------------------------------------------|
| `TestBootSequence`            | /init prints "UNFS boot" and exits cleanly          |
| `TestBootWithShell`           | /bin/sh echoes input and shows prompt                |
| `TestFullBootWithForkAndExec` | Full init->fork->execve->waitpid->exit sequence     |
| `TestBootFilesystemRamdiskLayout` | Filesystem fits in ramdisk, no kernel overlap   |
| `TestBootMemoryWithRamdiskFilesystem` | Boot params contain correct addresses       |
| `TestBootInitProgramStructure` | /init MBC binary has valid instruction layout       |
| `TestBootShellProgramStructure` | /bin/sh MBC binary has read-eval-print loop        |
| `TestBootFilesystemRoundtrip` | All files round-trip through UNFS correctly          |

## How It Maps to a Real Unix Boot Sequence

| UPC Boot Demo          | Real Unix/FUZIX                              |
|------------------------|----------------------------------------------|
| `wotan-ctl boot`       | BIOS/bootloader loads kernel                 |
| Boot params at 0x0100  | Kernel command line, initrd address           |
| IVT at 0x0000          | Interrupt Descriptor Table (IDT)              |
| UNFS ramdisk           | initramfs / initrd                            |
| `/init` entry           | `/sbin/init` (PID 1)                          |
| `SYS_FORK`             | `fork(2)` — create child process              |
| `SYS_EXECVE /bin/sh`   | `execve(2)` — replace process image           |
| `SYS_WAITPID`          | `waitpid(2)` — parent waits for child         |
| `SYS_WRITE` to fd 1    | `write(2)` to stdout via console driver       |
| `SYS_READ` from fd 0   | `read(2)` from stdin via console driver       |
| `SYS_EXIT(0)`          | `exit(2)` — process termination               |

## Syscalls Used

The boot demo exercises the following syscall surface (all implemented in
the MBC CPU emulator at `crates/monad-mbc/src/execute.rs`):

| Nr | Name          | Usage                                |
|----|---------------|--------------------------------------|
| 1  | `SYS_EXIT`    | Clean process termination            |
| 2  | `SYS_FORK`    | Create child process                 |
| 3  | `SYS_READ`    | Read from stdin (fd 0)               |
| 4  | `SYS_WRITE`   | Write to stdout (fd 1)               |
| 7  | `SYS_WAITPID` | Parent waits for child               |
| 11 | `SYS_EXECVE`  | Replace process image with new binary |

## Architecture

```
  wotan-ctl boot
       |
       v
  BootConfig { kernel: boot_demo.bin, ramdisk: ramdisk.img }
       |
       v
  PrepareBootMemory()
       |
       +-- IVT at RAM[0x0000-0x03FF]
       +-- Boot params at RAM[0x0100-0x01FF]
       +-- Kernel at RAM[0x10000+]
       +-- Ramdisk at RAM[0x800000+]
       |
       v
  CPU starts at PC=0 (init entry point)
       |
       +-- SYS_WRITE "UPC Boot v1.0"
       +-- SYS_FORK
       |       |
       |       +-- Child: SYS_EXECVE -> shell entry
       |       |       +-- SYS_WRITE "> "
       |       |       +-- SYS_READ (stdin)
       |       |       +-- SYS_WRITE (echo)
       |       |       +-- SYS_EXIT(0)
       |       |
       |       +-- Parent: SYS_WAITPID
       |               +-- SYS_WRITE "Shell exited"
       |               +-- SYS_WRITE "Init complete"
       |               +-- SYS_EXIT(0)
       v
  System halted
```
