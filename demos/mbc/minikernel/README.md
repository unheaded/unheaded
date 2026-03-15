# MiniKernel — UPC Level 5 Proof of Concept

A minimal kernel skeleton for the Unheaded Protocol Computer (UPC) that
demonstrates all Level 4 OS primitives working together in a single
bootable MBC binary.

## What it demonstrates

| Primitive | How it is exercised |
|-----------|-------------------|
| Boot protocol | Loaded by `wotan-ctl boot`, IVT already initialised |
| Interrupts (INT/IRET) | Installs a custom timer handler in IVT[0x20] |
| Syscalls (INT 0x80) | SYS_WRITE, SYS_FORK, SYS_GETPID, SYS_NANOSLEEP, SYS_SCHED_YIELD, SYS_CLOCK_GETTIME |
| Scheduler / Fork | Forks into parent + child, both run concurrently |
| Console I/O | Multiple SYS_WRITE calls to stdout (fd 1) |
| Block device | SYS_READ_BLOCK (block 0) and SYS_WRITE_BLOCK (block 1) |
| MMU | Implicit — page tables active during all memory access |

## Memory layout

```
0x0000-0x03FF  IVT (256 vectors, set by bootloader)
0x0080         IVT[0x20] — timer interrupt (patched by kernel)
0x0200         IVT[0x80] — syscall dispatch (set by bootloader)
0x0500+        Kernel code + data (loaded binary)
0x3000         Block read/write buffer (512 bytes)
0x3200         Clock time output buffer
```

## Building

```bash
make build
```

This runs `go run build.go`, which generates `minikernel.bin` — a flat
binary of MBC instructions followed by a data section.

## Booting

```bash
make boot
```

Or manually:

```bash
wotan-ctl boot --kernel minikernel.bin --args "console=tty0"
```

## Expected output

```
MiniKernel v0.1 — UPC Level 5
Parent process running
Block 0 read OK
Block 1 write OK
PID queried OK
Child process running
.
.
.
```

The parent process prints its messages, exercises the block device and
clock, then enters a yield loop. The child process prints a dot every
100 ms in an infinite loop. Both are scheduled by the timer-driven
round-robin scheduler.

## File inventory

| File | Purpose |
|------|---------|
| `minikernel.s` | Annotated MBC assembly source (reference / documentation) |
| `build.go` | Go program that encodes the assembly into `minikernel.bin` |
| `Makefile` | Build and boot targets |
| `README.md` | This file |

## Instruction encoding

Each MBC instruction is a 32-bit little-endian word:

```
[opcode:8][dst:4][src:4][imm16:16]
```

Opcodes and syscall numbers are sourced from
`ebpf/monad-common/src/lib.rs` (`mbc_opcodes` and `mbc_linux_syscalls`
modules).
