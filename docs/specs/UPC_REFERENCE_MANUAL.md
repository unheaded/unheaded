# UPC Reference Manual

*SPDX-License-Identifier: GPL-3.0-or-later*
*Version 1.0 -- March 15, 2026*

The definitive technical specification for the Unheaded Protocol Computer (UPC),
a 32-bit virtual CPU implemented entirely inside Linux eBPF XDP hooks.

---

## 1. Overview

### 1.1 What is the UPC

The UPC (Unheaded Protocol Computer) is a complete 32-bit computer architecture
that executes programs inside the Linux kernel's eBPF subsystem. Each CPU tick
is driven by an IPv6 packet carrying a Monad (the 20-byte Unheaded Protocol
register file) in a Hop-by-Hop Options extension header. An XDP program
receives the tick packet, fetches/decodes/executes up to 256 MBC (Monad
Bytecode) instructions per tick, then returns `XDP_TX` to bounce the packet
for the next tick -- achieving cache-warm continuous execution.

The UPC was originally built as a computational completeness proof for the
Unheaded Protocol (running DOOM on the UPC), but has evolved into a full
operating system target with process scheduling, virtual memory, a block
device, a filesystem, and a Linux-compatible syscall interface.

### 1.2 Architecture Components

| Component | Role | Implementation |
|-----------|------|----------------|
| **Monad** | Instruction bus -- tick packets trigger CPU execution | IPv6 HBH option (20 bytes) |
| **Sophia** | Microcode / dictionary lookups for protocol field encoding | BPF hash maps (key u8 -> SophiaEntry) |
| **Wotan** | Memory system -- hosts RAM, ROM, screen, keyboard as BPF maps | BPF Array/HashMap maps |
| **Shield** | Packet lifecycle -- inserts/strips Monad at ingress/egress | XDP + TC eBPF programs |

### 1.3 Execution Modes

| Mode | Description | Tick Latency |
|------|-------------|-------------|
| **LOCAL** | XDP_TX turbo mode -- packet bounces on loopback, cache-warm | ~200 ns/insn |
| **DISTRIBUTED** | Packet traverses real network hops between Kingdom nodes | ~1 ms/insn |

In LOCAL mode, the Monad's `hop_count` field acts as a bounce counter
(saturates at 255, then the packet is dropped and a fresh one injected).

---

## 2. Instruction Set Architecture (ISA)

### 2.1 Instruction Format

All instructions are 32-bit fixed-width:

```
 31      24 23    20 19    16 15                 0
+---------+--------+--------+--------------------+
| opcode  |  dst   |  src   |      imm16         |
|  (8b)   |  (4b)  |  (4b)  |      (16b)         |
+---------+--------+--------+--------------------+
```

For branch instructions, the lower 24 bits (bits 23:0) are used as a signed
offset (sign-extended from bit 23), providing a +/- 8M instruction range.
For CALL, the lower 24 bits are an absolute target address.

### 2.2 Register File

16 general-purpose 32-bit registers:

| Register | Alias | Purpose |
|----------|-------|---------|
| r0 | -- | General purpose / syscall number (INT 0x80) / return value |
| r1 | -- | General purpose / syscall arg 1 (INT 0x80) / SYSCALL nr (Doom mode) |
| r2 | -- | General purpose / syscall arg 2 |
| r3 | -- | General purpose / syscall arg 3 |
| r4-r7 | -- | General purpose |
| r8 | -- | General purpose / a0 (RISC-V convention for SYSCALL opcode) |
| r9 | -- | General purpose / a1 (RISC-V convention for SYSCALL opcode) |
| r10-r12 | -- | General purpose |
| r13 | LR | Link register (convention, not hardware-enforced) |
| r14 | FP | Frame pointer (convention) |
| r15 | SP | Stack pointer (hardware: PUSH/POP/CALL/RET use this) |

Default SP value at boot: `0xFFFF_0000` (grows downward).

### 2.3 Flags Register

The flags register is an 8-bit value stored in `MbcCpuState.flags`:

| Bit | Name | Meaning |
|-----|------|---------|
| 0 | Z (Zero) | Set when the ALU result is zero |
| 1 | N (Negative) | Set when bit 31 of the ALU result is 1 |
| 2 | C (Carry) | Set on unsigned overflow/borrow |
| 3 | IF (Interrupt Enable) | Managed by STI/CLI/IRET (stored in `interrupts_enabled` field) |

Flags are set by: ADD, SUB, MUL, DIV, MOD, NEG, AND, OR, XOR, NOT, SHL, SHR,
SAR, SHLR, SHRR, SARR, MULH, MULHU, CMP, ADDI. The CMP instruction sets
flags without storing the result.

### 2.4 Complete Opcode Table

#### 2.4.1 No-op

| Opcode | Hex | Mnemonic | Operation | Flags |
|--------|-----|----------|-----------|-------|
| 0x00 | `00` | NOP | No operation | -- |

#### 2.4.2 Arithmetic

| Opcode | Hex | Mnemonic | Operation | Flags |
|--------|-----|----------|-----------|-------|
| 0x01 | `01` | ADD | `dst = dst + src` | Z, N, C |
| 0x02 | `02` | SUB | `dst = dst - src` | Z, N, C |
| 0x03 | `03` | MUL | `dst = dst * src` (wrapping) | Z, N |
| 0x04 | `04` | DIV | `dst = dst / src` (unsigned; div-by-zero -> 0xFFFFFFFF) | Z, N |
| 0x05 | `05` | MOD | `dst = dst % src` (unsigned; mod-by-zero -> 0) | Z, N |
| 0x06 | `06` | NEG | `dst = -dst` (two's complement) | Z, N |

#### 2.4.3 Logic / Bitwise

| Opcode | Hex | Mnemonic | Operation | Flags |
|--------|-----|----------|-----------|-------|
| 0x07 | `07` | AND | `dst = dst & src` | Z, N |
| 0x08 | `08` | OR | `dst = dst \| src` | Z, N |
| 0x09 | `09` | XOR | `dst = dst ^ src` | Z, N |
| 0x0A | `0A` | NOT | `dst = ~dst` | Z, N |
| 0x0B | `0B` | SHL | `dst = dst << (imm16 & 31)` | Z, N |
| 0x0C | `0C` | SHR | `dst = dst >> (imm16 & 31)` (logical) | Z, N |
| 0x0D | `0D` | SAR | `dst = dst >> (imm16 & 31)` (arithmetic, sign-extending) | Z, N |

#### 2.4.4 Register Operations

| Opcode | Hex | Mnemonic | Operation | Flags |
|--------|-----|----------|-----------|-------|
| 0x0E | `0E` | MOV | `dst = src` | -- |
| 0x0F | `0F` | MOVI | `dst = zero_extend(imm16)` | -- |

#### 2.4.5 Comparison

| Opcode | Hex | Mnemonic | Operation | Flags |
|--------|-----|----------|-----------|-------|
| 0x10 | `10` | CMP | `temp = dst - src` (result discarded) | Z, N, C |

#### 2.4.6 Interrupts

| Opcode | Hex | Mnemonic | Operation | Flags |
|--------|-----|----------|-----------|-------|
| 0x17 | `17` | INT | Software interrupt: push flags+PC, jump to IVT[imm8] | -- |
| 0x18 | `18` | IRET | Return from interrupt: pop PC+flags, enable interrupts | restored |

#### 2.4.7 Stack Operations

| Opcode | Hex | Mnemonic | Operation | Flags |
|--------|-----|----------|-----------|-------|
| 0x1A | `1A` | PUSH | `SP -= 4; RAM[SP] = dst` | -- |
| 0x1B | `1B` | POP | `dst = RAM[SP]; SP += 4` | -- |

#### 2.4.8 Extended Immediate

| Opcode | Hex | Mnemonic | Operation | Flags |
|--------|-----|----------|-----------|-------|
| 0x1C | `1C` | LOAD_IMM32 | `dst[31:16] = imm16` (preserves lower 16 bits) | -- |
| 0x1D | `1D` | ADDI | `dst = dst + sign_extend(imm16)` | Z, N, C |

To load a full 32-bit constant: `MOVI r0, lo16` then `LOAD_IMM32 r0, hi16`.

#### 2.4.9 Branch

All branch offsets are 24-bit signed (bits 23:0, sign-extended from bit 23),
PC-relative (added to already-incremented PC).

| Opcode | Hex | Mnemonic | Operation | Condition |
|--------|-----|----------|-----------|-----------|
| 0x20 | `20` | JMP | `PC += offset24` | Unconditional |
| 0x21 | `21` | JZ | `if Z: PC += offset24` | Zero (equal) |
| 0x22 | `22` | JNZ | `if !Z: PC += offset24` | Not zero (not equal) |
| 0x23 | `23` | JN | `if N: PC += offset24` | Negative (signed less) |
| 0x24 | `24` | JP | `if !N: PC += offset24` | Positive (signed greater/equal) |
| 0x25 | `25` | JC | `if C: PC += offset24` | Carry (unsigned overflow) |
| 0x26 | `26` | JNC | `if !C: PC += offset24` | No carry |
| 0x27 | `27` | CALL | `PUSH(PC); PC = target24` | Unconditional |
| 0x28 | `28` | RET | `PC = POP()` | Unconditional |
| 0x29 | `29` | JMPR | `PC = RV2MBC_MAP[dst >> 2]` | Indirect (via address translation) |
| 0x2A | `2A` | CALLR | `PUSH(PC); PC = RV2MBC_MAP[dst >> 2]` | Indirect call |

CALL uses the lower 24 bits as an absolute target address (not relative).
JMPR and CALLR perform RV32I-to-MBC address translation through the RV2MBC_MAP
for RISC-V function pointer compatibility.

#### 2.4.10 Memory

All memory operations use `src + sign_extend(imm16)` as the effective address.
Addresses pass through the MMU's `translate_address()` when paging is enabled.

| Opcode | Hex | Mnemonic | Operation | Width |
|--------|-----|----------|-----------|-------|
| 0x30 | `30` | LD | `dst = RAM[src + simm16]` | 32-bit word |
| 0x31 | `31` | ST | `RAM[dst + simm16] = src` | 32-bit word |
| 0x32 | `32` | LDB | `dst = zero_extend(RAM_byte[src + simm16])` | 8-bit byte |
| 0x33 | `33` | STB | `RAM_byte[dst + simm16] = src & 0xFF` | 8-bit byte |
| 0x34 | `34` | LDH | `dst = zero_extend(RAM_half[src + simm16])` | 16-bit halfword |
| 0x35 | `35` | STH | `RAM_half[dst + simm16] = src & 0xFFFF` | 16-bit halfword |

Word loads/stores require 4-byte alignment. Halfword loads/stores use the
appropriate 16-bit slice within the containing word. Byte accesses are
fully unaligned.

#### 2.4.11 Register-Based Shifts and Extended Multiply

| Opcode | Hex | Mnemonic | Operation | Flags |
|--------|-----|----------|-----------|-------|
| 0x36 | `36` | SHLR | `dst = dst << (src & 31)` | Z, N |
| 0x37 | `37` | SHRR | `dst = dst >> (src & 31)` (logical) | Z, N |
| 0x38 | `38` | SARR | `dst = (i32)dst >> (src & 31)` (arithmetic) | Z, N |
| 0x39 | `39` | MULH | `dst = (i64(dst) * i64(src)) >> 32` (signed upper half) | Z, N |
| 0x3A | `3A` | MULHU | `dst = (u64(dst) * u64(src)) >> 32` (unsigned upper half) | Z, N |

#### 2.4.12 Atomic Operations

These provide mutual exclusion on the single-core UPC via interrupt
disable/enable:

| Opcode | Hex | Mnemonic | Operation | Flags |
|--------|-----|----------|-----------|-------|
| 0x3B | `3B` | CLI | Disable interrupts (`interrupts_enabled = 0`) | -- |
| 0x3C | `3C` | STI | Enable interrupts (`interrupts_enabled = 1`) | -- |
| 0x3D | `3D` | XCHG | `tmp = dst; dst = RAM[src+simm]; RAM[src+simm] = tmp` | -- |
| 0x3E | `3E` | CAS | `old = RAM[r1]; if old == r0: RAM[r1] = r2, set Z; r0 = old` | Z |

#### 2.4.13 System

| Opcode | Hex | Mnemonic | Operation | Flags |
|--------|-----|----------|-----------|-------|
| 0x40 | `40` | SYSCALL | Invoke Doom I/O callback (r1=syscall_nr for Doom mode) | -- |
| 0xFF | `FF` | HALT | Set `halted = 1`, stop execution | -- |

---

## 3. Memory Architecture

### 3.1 Physical Memory Map

The UPC uses BPF maps as its physical memory, providing a flat 32-bit
byte-addressable space:

```
Byte Address Range          Word Range          Size        Description
───────────────────────────────────────────────────────────────────────
0x0000_0000 - 0x0001_9080  0x000000 - 0x006420  ~100 KB    .rodata
0x0001_9080 - 0x0002_7B24  0x006420 - 0x009EC9  ~60 KB     .data
0x0002_7B24 - 0x0006_3330  0x009EC9 - 0x018CCC  ~240 KB    .bss
0x0006_8000                 0x01A000             4 B        Keyboard I/O word
0x0007_0000 - 0x0007_FA00  0x01C000 - 0x01FE80  ~64 KB     Screen framebuffer (320x200)
0x000F_0000 - 0x0010_FFFF  0x03C000 - 0x043FFF  128 KB     Debug/diagnostic region
0x0011_0000 - 0x0051_0000  0x044000 - 0x144000  4 MB       WAD data / application data
0x0052_0000 - 0x0152_0000  0x148000 - 0x548000  16 MB      Heap (bump allocator)
0x03F0_0000                 0x0FC0000            --         Stack top (grows downward)
0x0080_0000+               0x200000+            4 MB       Ramdisk (UNFS)
0x00F0_0000                 0x03C000             256 B      Signal handler table
```

### 3.2 BPF Map Backing

| BPF Map | Type | Entries | Entry Size | Description |
|---------|------|---------|------------|-------------|
| `ROM_MAP` | Array | 262,144 | u32 | Instruction memory (1 MiB, indexed by PC) |
| `RAM_MAP` | Array | 16,777,216 | u32 | Main memory (64 MiB, word-addressed) |
| `SCREEN_MAP` | Array | 64,000 | u8 | Framebuffer (320x200, 8-bit palette) |
| `KBD_MAP` | Array | 8 | u32 | Keyboard input queue (scancode << 1 \| pressed) |
| `CPU_MAP` | HashMap | 256 | MbcCpuState | CPU instance state (keyed by flow label) |
| `STATS` | HashMap | 32 | u64 | Performance counters |
| `L1_CACHE` | LruHashMap | 256 | [u8; 64] | Reserved (currently disabled) |
| `RV2MBC_MAP` | Array | 65,536 | u32 | RV32I-to-MBC address translation |
| `COMPUTE_EVENTS` | RingBuf | 262,144 B | -- | Event ring buffer to userspace |
| `TTY_MAP` | Array | 4,096 | u8 | Console output circular buffer |
| `TTY_HEAD` | Array | 1 | u32 | TTY write head position |
| `PROC_TABLE` | Array | 4 | [u32; 20] | Process state table (4 slots) |
| `SCHED_STATE` | Array | 4 | u32 | Scheduler state |
| `TLB_MAP` | Array | 64 | [u32; 3] | Software TLB (direct-mapped) |

### 3.3 MMIO Regions

| Address | Map | Access | Description |
|---------|-----|--------|-------------|
| 0x0006_8000 | KBD_MAP[0] | R | Keyboard state word |
| 0x0007_0000 - 0x0007_FA00 | SCREEN_MAP | R/W (byte only) | 320x200 framebuffer |
| 0x00F0_0000 - 0x00F0_00FF | RAM_MAP | R/W | Signal handler table (64 slots x 4 bytes) |

Screen writes via ST (word store) go only to RAM_MAP, not SCREEN_MAP.
Only STB (byte store) and the `copy_fb_to_screen()` helper update SCREEN_MAP.
This prevents BSS corruption from writing garbage to the display.

---

## 4. Interrupt System

### 4.1 Interrupt Vector Table (IVT)

The IVT occupies the first 256 words of RAM (word addresses 0x00 - 0xFF,
byte addresses 0x0000 - 0x03FF). Each entry is a 32-bit handler address
(an MBC ROM word address).

| Vector | Byte Offset | Default Use |
|--------|-------------|-------------|
| 0x00 | 0x000 | Divide by zero |
| 0x01 | 0x004 | Debug / single-step |
| 0x06 | 0x018 | Invalid opcode |
| 0x0E | 0x038 | Page fault |
| 0x20 | 0x080 | Timer interrupt |
| 0x21 | 0x084 | Keyboard interrupt |
| 0x22 | 0x088 | Disk complete |
| 0x80 | 0x200 | Syscall trap (INT 0x80) |

At boot, all vectors default to the HLT handler at word address 0x0100
(byte address 0x0400).

### 4.2 Timer Interrupt

- Vector: `0x20` (IVT offset 0x80)
- Rate: ~12 Hz (every 3rd XDP tick at 35 Hz tick rate)
- Controlled by `TIMER_TICK_DIVISOR = 3`
- Only fires when `interrupts_enabled != 0` and `interrupt_pending == 0`
- When `num_processes > 1`, the timer triggers a round-robin context switch

### 4.3 Keyboard Interrupt

- Vector: `0x21` (IVT offset 0x84)
- Triggered by userspace writing to KBD_MAP

### 4.4 Syscall Trap (INT 0x80)

- Vector: `0x80` (IVT offset 0x200)
- Convention: `r0 = syscall_nr`, `r1-r3 = arguments`, `r0 = return value`
- INT 0x80 is handled as a synchronous dispatch -- PC is NOT pushed to stack
  (unlike hardware interrupts), the syscall handler runs inline

### 4.5 Interrupt Sequence

When an interrupt fires (hardware or software via INT):

1. Push flags to stack: `SP -= 4; RAM[SP] = flags`
2. Push PC to stack: `SP -= 4; RAM[SP] = PC`
3. Disable interrupts: `interrupts_enabled = 0`
4. Jump to handler: `PC = RAM[vector * 4]`
5. Clear pending: `interrupt_pending = 0`

Exception: INT 0x80 (syscall) skips steps 1-3 and dispatches inline.
Non-0x80 INT instructions follow the full sequence.

### 4.6 IRET (Return from Interrupt)

1. Pop PC from stack: `PC = RAM[SP]; SP += 4`
2. Pop flags from stack: `flags = RAM[SP]; SP += 4`
3. Re-enable interrupts: `interrupts_enabled = 1`

---

## 5. System Call Interface

### 5.1 Doom Syscalls (SYSCALL opcode 0x40)

Used by the Doom engine via the `SYSCALL` instruction.
Convention: `r1 = syscall_nr`, `r8 = arg0/return0`, `r9 = arg1/return1`.

| Nr | Name | Hex | Arguments | Returns |
|----|------|-----|-----------|---------|
| 1 | SYS_DRAW_FRAME | 0x01 | r1=fb_addr | -- (increments STAT_FRAME_READY) |
| 2 | SYS_GET_KEY | 0x02 | -- | r8=keycode, r9=pressed (0/1) |
| 3 | SYS_GET_TICKS | 0x03 | -- | r8=milliseconds since boot |
| 4 | SYS_SLEEP | 0x04 | r8=milliseconds | -- (sets sleep_until_ns) |

### 5.2 Linux-Compatible Syscalls (INT 0x80)

Used by C programs compiled for the UPC. Follow the i386 ABI numbering.
Convention: `r0 = syscall_nr`, `r1-r5 = arguments`, `r0 = return value`.
Negative return values indicate `-errno`.

#### 5.2.1 Process Management

| Nr | Name | Args | Returns | Notes |
|----|------|------|---------|-------|
| 1 | SYS_EXIT | r1=status | -- (halts CPU) | Sets exit_code, unsuspends vfork parent |
| 2 | SYS_FORK | -- | r0=child_pid (parent), r0=0 (child) | Max 4 processes; -EAGAIN if full |
| 7 | SYS_WAITPID | r1=pid | r0=pid (if exited), blocks otherwise | Context switches while waiting |
| 11 | SYS_EXECVE | r1=entry_point | -- | Resets CPU state, jumps to entry |
| 20 | SYS_GETPID | -- | r0=instance_id | Returns flow label instance |
| 64 | SYS_GETPPID | -- | r0=0 | Always returns 0 (init) |
| 158 | SYS_SCHED_YIELD | -- | r0=0 | Voluntary context switch |
| 190 | SYS_VFORK | -- | r0=child_pid | Like fork + suspends parent |

#### 5.2.2 File I/O

| Nr | Name | Args | Returns | Notes |
|----|------|------|---------|-------|
| 3 | SYS_READ | r1=fd, r2=buf, r3=count | r0=bytes read | fd 0: reads KBD_MAP; others: -EBADF |
| 4 | SYS_WRITE | r1=fd, r2=buf, r3=count | r0=bytes written | fd 1/2: writes TTY_MAP (max 256/call) |
| 5 | SYS_OPEN | r1=path, r2=flags, r3=mode | r0=3 | Stub: always returns fd 3 |
| 6 | SYS_CLOSE | r1=fd | r0=0 | No-op stub |
| 19 | SYS_LSEEK | r1=fd, r2=offset, r3=whence | r0=0 | Stub |
| 33 | SYS_ACCESS | r1=path, r2=mode | r0=0 | Stub: always succeeds |
| 41 | SYS_DUP | r1=oldfd | r0=oldfd | Stub: returns same fd |
| 42 | SYS_PIPE | r1=pipefd_addr | r0=0 | Stub: writes fd 10 (read), 11 (write) |
| 55 | SYS_FCNTL | r1=fd, r2=cmd | r0=0 | No-op stub |
| 63 | SYS_DUP2 | r1=oldfd, r2=newfd | r0=newfd | Stub |
| 106 | SYS_STAT | r1=path, r2=buf | r0=0 | Writes minimal stat struct |
| 108 | SYS_FSTAT | r1=fd, r2=buf | r0=0 | Writes minimal stat struct |

#### 5.2.3 Memory Management

| Nr | Name | Args | Returns | Notes |
|----|------|------|---------|-------|
| 45 | SYS_BRK | r1=new_brk | r0=current_brk | If r1=0, returns current; else sets new |

Default program break: `0x0040_0000` (4 MiB).

#### 5.2.4 Signals

| Nr | Name | Args | Returns | Notes |
|----|------|------|---------|-------|
| 37 | SYS_KILL | r1=pid, r2=sig | r0=0 | No-op stub |
| 48 | SYS_SIGNAL | r1=signum, r2=handler | r0=old_handler | Stores in signal table at 0xF0000 |

Signal table: 64 slots at RAM byte address 0xF0000 (word 0x3C000), 4 bytes each.

#### 5.2.5 Time

| Nr | Name | Args | Returns | Notes |
|----|------|------|---------|-------|
| 13 | SYS_TIME | -- | r0=seconds since boot | ktime_ns / 1e9 |
| 162 | SYS_NANOSLEEP | r1=timespec_addr | r0=0 | Reads {secs, nsecs} from RAM |
| 265 | SYS_CLOCK_GETTIME | r1=tp_addr | r0=0 | Writes {secs, nsecs} to RAM |

#### 5.2.6 Device / Block I/O

| Nr | Name | Args | Returns | Notes |
|----|------|------|---------|-------|
| 54 | SYS_IOCTL | r1=fd, r2=request, r3=argp | r0=0 | TIOCGWINSZ: 24x80; TCGETS: stub |
| 200 | SYS_READ_BLOCK | r1=block_num, r2=buf_addr | r0=512 or -EIO | Ramdisk read (128 words) |
| 201 | SYS_WRITE_BLOCK | r1=block_num, r2=buf_addr | r0=512 or -EIO | Ramdisk write (128 words) |

#### 5.2.7 MMU Control

| Nr | Name | Args | Returns | Notes |
|----|------|------|---------|-------|
| 250 | SYS_SET_PAGE_DIR | r1=phys_addr | r0=0 | Sets page directory base |
| 251 | SYS_ENABLE_MMU | -- | r0=0 | Enables paging (mmu_enabled = 1) |
| 252 | SYS_FLUSH_TLB | -- | r0=0 | Invalidates all 64 TLB entries |

#### 5.2.8 Identity

| Nr | Name | Args | Returns | Notes |
|----|------|------|---------|-------|
| 23 | SYS_SETUID | r1=uid | r0=0 | No-op (always root) |
| 24 | SYS_GETUID | -- | r0=0 | Always root |
| 46 | SYS_SETGID | r1=gid | r0=0 | No-op |
| 47 | SYS_GETGID | -- | r0=0 | Always root |
| 49 | SYS_GETEUID | -- | r0=0 | Always root |
| 50 | SYS_GETEGID | -- | r0=0 | Always root |
| 60 | SYS_UMASK | r1=mask | r0=022 | Returns default, ignores set |

#### 5.2.9 Miscellaneous

| Nr | Name | Args | Returns | Notes |
|----|------|------|---------|-------|
| 12 | SYS_CHDIR | r1=path | r0=0 | Stub: always succeeds |
| 36 | SYS_SYNC | -- | r0=0 | No-op |
| 43 | SYS_TIMES | r1=buf | r0=0 | No process accounting |

Unknown syscall numbers return `-ENOSYS` (-38) in r0.

---

## 6. Scheduler

### 6.1 Design

Round-robin preemptive scheduler supporting up to 4 concurrent processes.
Context switches are triggered by:
- Timer interrupt (every ~12 Hz when `num_processes > 1`)
- SYS_SCHED_YIELD (voluntary)
- SYS_WAITPID (blocks calling process)

### 6.2 Process Table (PROC_TABLE)

BPF Array with 4 entries, each storing 20 u32 values:

| Offset | Field | Description |
|--------|-------|-------------|
| 0-15 | r0-r15 | Saved general-purpose registers |
| 16 | PC | Saved program counter |
| 17 | flags | Saved flags register (as u32) |
| 18 | SP_copy | Copy of r15 (redundant, for debugging) |
| 19 | program_break | Saved heap break address |

### 6.3 Scheduler State (SCHED_STATE)

BPF Array with 4 entries:

| Index | Field | Description |
|-------|-------|-------------|
| 0 | current_pid | Currently running process ID (0-3) |
| 1 | num_processes | Total active processes |
| 2 | suspended_mask | Bitmask: bit i set = process i suspended (vfork) |
| 3 | halted_mask | Bitmask: bit i set = process i has exited |

### 6.4 Context Switch Sequence

1. **Save** current process: copy `cpu.regs[0..15]`, `pc`, `flags`, `SP`,
   `program_break` into `PROC_TABLE[current_pid]`
2. **Select** next: `next_pid = (current_pid + 1) % num_processes`, skipping
   processes with bits set in `halted_mask | suspended_mask` (bounded to 4 attempts)
3. **Load** next process: restore all fields from `PROC_TABLE[next_pid]`
4. **Update**: set `cpu.current_pid = next_pid`, write to `SCHED_STATE[0]`
5. **Emit** `EVENT_CONTEXT_SWITCH` ring buffer event

If all other processes are halted/suspended, the switch is skipped (stays on
current process).

---

## 7. Virtual Memory (MMU)

### 7.1 Overview

Two-level 32-bit page table with a 64-entry direct-mapped software TLB.
Disabled by default (`mmu_enabled = 0`, flat addressing for Doom mode).
Enabled via `SYS_ENABLE_MMU` (syscall 251).

### 7.2 Virtual Address Translation

```
Virtual Address (32-bit):
  [31:22] = Page Directory Index  (10 bits, 1024 entries)
  [21:12] = Page Table Index      (10 bits, 1024 entries)
  [11:0]  = Page Offset           (12 bits, 4KB pages)
```

### 7.3 TLB (TLB_MAP)

64 entries, direct-mapped by `VPN & 63`. Each entry is 3 u32 values:

| Field | Description |
|-------|-------------|
| [0] vpn | Virtual page number |
| [1] pfn | Physical frame number |
| [2] flags | Page table entry flags (PTE_PRESENT, etc.) |

### 7.4 Page Table Entry Format

| Bits | Field | Description |
|------|-------|-------------|
| [31:12] | PFN | Physical Frame Number (`PTE_PFN_MASK = 0xFFFFF000`) |
| [11] | Present | Page is present in memory (`PTE_PRESENT = 0x800`) |
| [10] | Write | Page is writable (`PTE_WRITE = 0x400`) |
| [9] | User | Page is accessible from user mode (`PTE_USER = 0x200`) |
| [8] | Accessed | Page has been read (`PTE_ACCESSED = 0x100`) |
| [7] | Dirty | Page has been written (`PTE_DIRTY = 0x080`) |
| [6:0] | Reserved | Must be zero |

### 7.5 Translation Flow

1. If `mmu_enabled == 0`: return virtual address unchanged (flat mode)
2. Extract VPN = `vaddr >> 12`, offset = `vaddr & 0xFFF`
3. **TLB check**: `TLB_MAP[vpn & 63]` -- if `entry.vpn == vpn` and `PTE_PRESENT`, return
   `(entry.pfn << 12) | offset` (TLB hit, increments `STAT_TLB_HITS`)
4. **TLB miss**: walk page table (increments `STAT_TLB_MISSES`)
   - Read PDE from `RAM[page_dir_base + pde_idx]`
   - If not present: return vaddr (page fault, graceful)
   - Read PTE from `RAM[pde.pfn_addr + pte_idx]`
   - If not present: return vaddr (page fault, graceful)
   - Fill TLB entry, return `(pte.pfn << 12) | offset`

### 7.6 MMU Syscalls

- `SYS_SET_PAGE_DIR` (250): set `cpu.page_dir_base = r1`
- `SYS_ENABLE_MMU` (251): set `cpu.mmu_enabled = 1`
- `SYS_FLUSH_TLB` (252): zero all 64 TLB entries

---

## 8. Block Device

### 8.1 Ramdisk

A 4 MB region of RAM_MAP serves as a block device:

| Parameter | Value |
|-----------|-------|
| Base address | `RAMDISK_BASE_WORD = 0x200000` (word address) |
| Size | 4 MB (`RAMDISK_SIZE = 4,194,304` bytes) |
| Block size | 512 bytes (`BLOCK_SIZE`) |
| Total blocks | 8,192 (`TOTAL_BLOCKS`) |
| Words per block | 128 (`WORDS_PER_BLOCK`) |

### 8.2 Block I/O Syscalls

**SYS_READ_BLOCK** (200): Copy 128 words from ramdisk to destination buffer.
- `r1` = block number (0 to 8191)
- `r2` = destination byte address in RAM
- Returns 512 on success, `-EIO` on invalid block number

**SYS_WRITE_BLOCK** (201): Copy 128 words from source buffer to ramdisk.
- `r1` = block number (0 to 8191)
- `r2` = source byte address in RAM
- Returns 512 on success, `-EIO` on invalid block number

---

## 9. UNFS Filesystem

### 9.1 Overview

UNFS (Unheaded Nano Filesystem) is a minimal flat filesystem for the UPC
ramdisk. Stored in the ramdisk region starting at `RAMDISK_BASE_WORD`.

### 9.2 Layout

| Block Range | Count | Description |
|-------------|-------|-------------|
| Block 0 | 1 | Superblock |
| Blocks 1-63 | 63 | Inode table (504 inodes, 8 per block) |
| Blocks 64+ | 8,128 | Data blocks |

### 9.3 Superblock (Block 0)

24 bytes at the start of block 0:

| Offset | Field | Type | Description |
|--------|-------|------|-------------|
| 0x00 | Magic | u32 | `0x554E4653` ("UNFS" in LE) |
| 0x04 | Version | u32 | Format version (1) |
| 0x08 | TotalBlks | u32 | Total block count (8192) |
| 0x0C | FreeBlks | u32 | Free block count |
| 0x10 | RootInode | u32 | Root directory inode number |
| 0x14 | FirstData | u32 | First data block number (64) |

### 9.4 Inode Format (64 bytes)

8 inodes per block, for a total of 504 inodes.

| Offset | Field | Type | Description |
|--------|-------|------|-------------|
| 0x00 | Name | [28]byte | Filename (null-terminated, max 27 chars) |
| 0x1C | Size | u32 | File size in bytes |
| 0x20 | Mode | u32 | Unix permission bits (e.g. 0o755) |
| 0x24 | BlockStart | u32 | First data block number |
| 0x28 | BlockCount | u32 | Number of contiguous data blocks |
| 0x2C | Created | u32 | Creation timestamp (Unix epoch) |
| 0x30 | Modified | u32 | Modification timestamp |
| 0x34 | FileType | u32 | 1=file, 2=directory, 3=device |
| 0x38 | Reserved | [8]byte | Zero-filled |

### 9.5 Data Blocks

Starting at block 64, each data block is 512 bytes. Files occupy contiguous
blocks. Data is stored in little-endian format.

### 9.6 API (Go)

```go
fs := upc.FormatFilesystem()                           // Create empty UNFS
inodeNum, err := upc.AddFile(fs, "hello.txt", data, 0o644)  // Add a file
data, err := upc.ReadFile(fs, "hello.txt")              // Read a file
names := upc.ListFiles(fs)                              // List all files
inode := upc.LookupInode(fs, "hello.txt")                // Find inode by name
```

`CreateBootFilesystem()` builds a filesystem with `/init`, `/bin/sh`,
`/dev/console`, and `/etc/hostname` pre-populated.

---

## 10. UPCFlat Binary Format

### 10.1 Overview

UPCFlat (`.upcf`) is a simplified flat binary format for the UPC, inspired
by bFLT (uClinux) but stripped down to the minimum needed for a nommu
flat binary: a header, text segment, data segment, and BSS size.

### 10.2 Header (32 bytes)

| Offset | Field | Type | Description |
|--------|-------|------|-------------|
| 0x00 | Magic | [4]byte | "UPCF" |
| 0x04 | Version | u32 | Format version (1) |
| 0x08 | Entry | u32 | Entry point (word offset in text) |
| 0x0C | TextSize | u32 | Text segment size in words |
| 0x10 | DataStart | u32 | Data segment word offset from payload start |
| 0x14 | DataSize | u32 | Data segment size in words |
| 0x18 | BSSSize | u32 | BSS size in words (zero-filled after data) |
| 0x1C | StackSize | u32 | Requested stack size in bytes |

All fields are little-endian u32.

### 10.3 On-Disk Layout

```
Bytes 0-31:    UPCFlatHeader (8 x uint32)
Bytes 32...:   Text words (TextSize x 4 bytes)
               Data words (DataSize x 4 bytes)
```

BSS is not stored on disk -- it is zero-filled at load time.

### 10.4 Loading

Text is copied into ROM starting at word 0. Data is copied into RAM
at a caller-specified word address. BSS words are zeroed immediately
after data in RAM. The caller sets PC to `header.Entry` and configures
SP from `header.StackSize`.

### 10.5 Comparison to bFLT

| Feature | bFLT | UPCFlat |
|---------|------|---------|
| Relocations | Yes (reloc table) | No (position-independent or fixed-address) |
| Compression | Optional (gzip) | None |
| Shared libraries | Partial | None |
| Header size | 64 bytes | 32 bytes |
| Target | ARM/m68k/etc | MBC (UPC) |

---

## 11. Boot Protocol

### 11.1 Boot Sequence

The boot process is managed by the Computermancer (`wotan-ctl boot`):

1. Load kernel image into RAM_MAP at 0x10000 (word address 0x4000)
2. Initialize IVT (256 vectors at word addresses 0x00 - 0xFF, all pointing to HLT handler)
3. Write default HLT handler at word address 0x0100 (byte 0x0400)
4. Write BootParams structure at word address 0x40 (byte 0x0100)
5. Optionally load ramdisk at byte address 0x800000 (word 0x200000)
6. Optionally store kernel command line at byte address 0x0200
7. Set SP (r15) to 0x03F00000
8. Set PC to 0x10000 (kernel entry point, word address 0x4000)
9. Start execution by injecting tick packets

### 11.2 BootParams Structure

Located at word address 0x40 (byte address 0x0100):

| Offset | Field | Type | Description |
|--------|-------|------|-------------|
| 0x00 | Magic | u32 | `0x554E4844` ("UNHD") |
| 0x04 | Version | u32 | Boot protocol version (1) |
| 0x08 | MemorySize | u32 | Total RAM in bytes (default: 64 MB) |
| 0x0C | RamdiskAddr | u32 | Ramdisk physical byte address |
| 0x10 | RamdiskSize | u32 | Ramdisk size in bytes |
| 0x14 | KernelAddr | u32 | Kernel load byte address (0x10000) |
| 0x18 | KernelSize | u32 | Kernel size in bytes |
| 0x1C | BootArgsAddr | u32 | Command line byte address (0x0200) |
| 0x20 | BootArgsLen | u32 | Command line length |
| 0x24 | NumCPUs | u32 | Number of CPU instances (1) |
| 0x28 | TickRateHz | u32 | Timer interrupt rate in Hz (12) |
| 0x2C | Reserved[20] | u32[] | Reserved, zero-filled |

### 11.3 CPU State at Boot

| Register / Field | Value | Description |
|------------------|-------|-------------|
| PC | 0x10000 | Kernel entry point (word 0x4000) |
| r15 (SP) | 0x03F00000 | Stack pointer (grows downward) |
| r0-r14 | 0 | Cleared |
| flags | 0 | All flags cleared |
| halted | 0 | CPU is running |
| num_processes | 1 | Single process |
| interrupts_enabled | 0 | Interrupts disabled at boot |
| program_break | 0x00400000 | Default heap start (4 MiB) |
| mmu_enabled | 0 | Flat addressing |

### 11.4 wotan-ctl Commands

```bash
wotan-ctl boot --kernel kernel.mbc                      # Boot a kernel
wotan-ctl boot --kernel kernel.mbc --ramdisk initrd.img  # Boot with ramdisk
wotan-ctl boot --kernel kernel.mbc --args "console=tty0" # Boot with args
wotan-ctl mkfs --output rootfs.img                       # Create UNFS image
```

---

## 12. Console I/O

### 12.1 TTY Output

Console output uses a circular buffer backed by BPF maps:

| Component | Description |
|-----------|-------------|
| `TTY_MAP` | 4,096-entry BPF Array of u8 (circular buffer) |
| `TTY_HEAD` | 1-entry BPF Array of u32 (current write position, wraps at 4096) |

When SYS_WRITE is called with fd 1 (stdout) or fd 2 (stderr):
1. Read current head position from `TTY_HEAD[0]`
2. Copy up to 256 bytes from the source buffer to `TTY_MAP[head % 4096]`
3. Update `TTY_HEAD[0]` to new position
4. Emit `EVENT_TTY_WRITE` ring buffer event with byte count
5. Return bytes written in r0

### 12.2 TTY Input

Keyboard input is read via SYS_READ with fd 0 (stdin):
1. Scan 8 `KBD_MAP` slots for non-zero events
2. For each key-down event (pressed flag = 1): extract low 8 bits as ASCII
3. Write byte to destination buffer, clear the KBD_MAP slot
4. Return bytes read in r0 (max 8 per call)

### 12.3 Ring Buffer Events

Userspace (doom-bridge) monitors the `COMPUTE_EVENTS` ring buffer for:

| Event Type | Value | Description |
|------------|-------|-------------|
| EVENT_TTY_WRITE | 0x18 | Console output -- `instruction` field = byte count |
| EVENT_SCREEN_WRITE | 0x14 | Frame ready (every 32nd frame) |
| EVENT_COMPUTE_HALT | 0x16 | CPU halted |
| EVENT_CONTEXT_SWITCH | 0x19 | Process switch -- `pc`=from_pid, `instruction`=to_pid |
| EVENT_CACHE_MISS | 0x11 | L1 cache miss (currently unused) |

---

## 13. MBC Assembler

### 13.1 Overview

The MBC assembler (`tools/mbc-asm/mbc_asm.py`) is a two-pass assembler
that produces raw 32-bit little-endian binary images from MBC assembly source.

```bash
python3 mbc_asm.py input.asm --output out.bin
python3 mbc_asm.py input.asm --output out.bin --list
python3 mbc_asm.py input.asm --hex
```

### 13.2 Syntax

```asm
; Comments start with semicolon
label:                      ; Labels end with colon
    MOVI r0, 42             ; Instruction with register and immediate
    ADD r0, r1              ; Two-register instruction
    LD r0, [r1+8]           ; Memory load with offset
    ST [r0+4], r1           ; Memory store with offset
    JZ target_label         ; Branch to label
    INT 0x80                ; Software interrupt
```

### 13.3 Register Aliases

| Alias | Register |
|-------|----------|
| SP, sp | r15 |
| FP, fp | r14 |
| LR, lr | r13 |

### 13.4 Directives

| Directive | Syntax | Description |
|-----------|--------|-------------|
| `.text` | `.text` | Switch to text (code) section |
| `.data` | `.data` | Switch to data section |
| `.org` | `.org ADDR` | Set origin address (byte address, divided by 4 internally) |
| `.ascii` | `.ascii "string"` | Emit null-terminated string (only in .data) |
| `.word` | `.word VALUE` | Emit a 32-bit word |
| `.equ` | `.equ NAME, VALUE` | Define a constant |

Escape sequences in `.ascii`: `\n`, `\r`, `\t`, `\0`, `\\`, `\"`.

### 13.5 Instruction Categories

| Category | Mnemonics | Operands |
|----------|-----------|----------|
| No operand | NOP, RET, IRET, HLT, HALT, SYSCALL | -- |
| Dst only | NOT, NEG, PUSH, POP, JMPR, CALLR | rD |
| Dst, Src | ADD, SUB, MUL, DIV, MOD, AND, OR, XOR, MOV, CMP, SHLR, SHRR, SARR, MULH, MULHU | rD, rS |
| Dst, Imm | MOVI, SHL, SHR, SAR, ADDI, LOAD_IMM32 | rD, imm16 |
| Branch | JMP, JZ, JNZ, JN, JP, JC, JNC, CALL | label_or_offset |
| Interrupt | INT | imm8 |
| Memory load | LD, LDB, LDH | rD, [rS+offset] |
| Memory store | ST, STB, STH | [rD+offset], rS |

### 13.6 Branch Encoding

Branch targets can be labels or numeric offsets. For labels, the assembler
computes a PC-relative signed 16-bit offset: `offset = target - (PC + 1)`.
The offset must fit in the range -32768 to +32767.

CALL uses a 24-bit absolute target in the eBPF execution engine (the assembler
emits the offset in the imm16 field for label resolution).

---

## 14. Performance Characteristics

### 14.1 Instruction Timing

| Mode | Instructions/Tick | Tick Rate | Peak IPS |
|------|-------------------|-----------|----------|
| LOCAL (XDP_TX turbo) | 256 | ~35 Hz (software-injected, bounced via XDP_TX) | ~9,000 |
| DISTRIBUTED | 256 | depends on network RTT | varies |

Each XDP invocation executes up to `MAX_INSN_PER_TICK = 256` instructions.
The BPF verifier limits this to 256 (320+ exceeds the 1M processed
instruction limit on current kernels).

### 14.2 Memory Access Latency

| Operation | Backing | Cost |
|-----------|---------|------|
| ROM fetch | BPF Array lookup | O(1), ~10 ns |
| RAM read/write | BPF Array lookup | O(1), ~10 ns |
| Screen byte write | BPF Array lookup | O(1), ~10 ns |
| TLB hit | BPF Array lookup | O(1), ~10 ns |
| TLB miss (page walk) | 2x BPF Array lookups + TLB fill | ~40 ns |

All memory accesses go directly through BPF Array maps (O(1) direct indexing).
The L1_CACHE (LruHashMap) is disabled because Array lookups are faster than
LRU hash lookups.

### 14.3 Syscall Overhead

Each Linux-compatible syscall (INT 0x80) incurs:
- Stat counter increment: 1 BPF HashMap update
- Argument extraction from registers: negligible
- Implementation-specific work (e.g., TTY_MAP writes for SYS_WRITE)

SYS_DRAW_FRAME triggers a bulk framebuffer copy (up to 16,000 words from
RAM_MAP to SCREEN_MAP), bounded by a BPF verifier-safe loop.

### 14.4 BPF Verifier Limits

| Limit | Value | Impact |
|-------|-------|--------|
| Max processed instructions | 1,000,000 | Limits execute loop to 256 iterations |
| Jump complexity | 8,192 | Each opcode branch is explored per iteration |
| Map operations per program | No hard limit | All go through NULL-checked `get_ptr_mut` |

---

## 15. Appendices

### Appendix A: Complete Opcode Encoding Table

| Hex | Mnemonic | Category | Encoding |
|-----|----------|----------|----------|
| 0x00 | NOP | No-op | `00_0_0_0000` |
| 0x01 | ADD | Arithmetic | `01_d_s_0000` |
| 0x02 | SUB | Arithmetic | `02_d_s_0000` |
| 0x03 | MUL | Arithmetic | `03_d_s_0000` |
| 0x04 | DIV | Arithmetic | `04_d_s_0000` |
| 0x05 | MOD | Arithmetic | `05_d_s_0000` |
| 0x06 | NEG | Arithmetic | `06_d_0_0000` |
| 0x07 | AND | Logic | `07_d_s_0000` |
| 0x08 | OR | Logic | `08_d_s_0000` |
| 0x09 | XOR | Logic | `09_d_s_0000` |
| 0x0A | NOT | Logic | `0A_d_0_0000` |
| 0x0B | SHL | Logic | `0B_d_0_iiii` |
| 0x0C | SHR | Logic | `0C_d_0_iiii` |
| 0x0D | SAR | Logic | `0D_d_0_iiii` |
| 0x0E | MOV | Data | `0E_d_s_0000` |
| 0x0F | MOVI | Data | `0F_d_0_iiii` |
| 0x10 | CMP | Compare | `10_d_s_0000` |
| 0x17 | INT | Interrupt | `17_0_0_iiii` |
| 0x18 | IRET | Interrupt | `18_0_0_0000` |
| 0x1A | PUSH | Stack | `1A_d_0_0000` |
| 0x1B | POP | Stack | `1B_d_0_0000` |
| 0x1C | LOAD_IMM32 | Immediate | `1C_d_0_iiii` |
| 0x1D | ADDI | Immediate | `1D_d_0_iiii` |
| 0x20 | JMP | Branch | `20_oooooo` (24-bit offset) |
| 0x21 | JZ | Branch | `21_oooooo` |
| 0x22 | JNZ | Branch | `22_oooooo` |
| 0x23 | JN | Branch | `23_oooooo` |
| 0x24 | JP | Branch | `24_oooooo` |
| 0x25 | JC | Branch | `25_oooooo` |
| 0x26 | JNC | Branch | `26_oooooo` |
| 0x27 | CALL | Branch | `27_tttttt` (24-bit target) |
| 0x28 | RET | Branch | `28_0_0_0000` |
| 0x29 | JMPR | Branch | `29_d_0_0000` |
| 0x2A | CALLR | Branch | `2A_d_0_0000` |
| 0x30 | LD | Memory | `30_d_s_iiii` |
| 0x31 | ST | Memory | `31_d_s_iiii` |
| 0x32 | LDB | Memory | `32_d_s_iiii` |
| 0x33 | STB | Memory | `33_d_s_iiii` |
| 0x34 | LDH | Memory | `34_d_s_iiii` |
| 0x35 | STH | Memory | `35_d_s_iiii` |
| 0x36 | SHLR | Shift | `36_d_s_0000` |
| 0x37 | SHRR | Shift | `37_d_s_0000` |
| 0x38 | SARR | Shift | `38_d_s_0000` |
| 0x39 | MULH | Multiply | `39_d_s_0000` |
| 0x3A | MULHU | Multiply | `3A_d_s_0000` |
| 0x3B | CLI | Atomic | `3B_0_0_0000` |
| 0x3C | STI | Atomic | `3C_0_0_0000` |
| 0x3D | XCHG | Atomic | `3D_d_s_iiii` |
| 0x3E | CAS | Atomic | `3E_0_0_0000` |
| 0x40 | SYSCALL | System | `40_0_0_0000` |
| 0xFF | HALT | System | `FF_0_0_0000` |

Legend: `d`=dst register, `s`=src register, `i`=immediate, `o`=offset, `t`=target.

### Appendix B: Complete Syscall Number Table

#### Doom Syscalls (SYSCALL opcode)

| Nr | Name | Description |
|----|------|-------------|
| 0x01 | SYS_DRAW_FRAME | Copy framebuffer to screen |
| 0x02 | SYS_GET_KEY | Read keyboard event |
| 0x03 | SYS_GET_TICKS | Get milliseconds since boot |
| 0x04 | SYS_SLEEP | Sleep for N milliseconds |

#### Linux Syscalls (INT 0x80)

| Nr | Name | Category |
|----|------|----------|
| 1 | SYS_EXIT | Process |
| 2 | SYS_FORK | Process |
| 3 | SYS_READ | File I/O |
| 4 | SYS_WRITE | File I/O |
| 5 | SYS_OPEN | File I/O |
| 6 | SYS_CLOSE | File I/O |
| 7 | SYS_WAITPID | Process |
| 11 | SYS_EXECVE | Process |
| 12 | SYS_CHDIR | File I/O |
| 13 | SYS_TIME | Time |
| 19 | SYS_LSEEK | File I/O |
| 20 | SYS_GETPID | Process |
| 23 | SYS_SETUID | Identity |
| 24 | SYS_GETUID | Identity |
| 33 | SYS_ACCESS | File I/O |
| 36 | SYS_SYNC | File I/O |
| 37 | SYS_KILL | Signal |
| 41 | SYS_DUP | File I/O |
| 42 | SYS_PIPE | File I/O |
| 43 | SYS_TIMES | Time |
| 45 | SYS_BRK | Memory |
| 46 | SYS_SETGID | Identity |
| 47 | SYS_GETGID | Identity |
| 48 | SYS_SIGNAL | Signal |
| 49 | SYS_GETEUID | Identity |
| 50 | SYS_GETEGID | Identity |
| 54 | SYS_IOCTL | Device |
| 55 | SYS_FCNTL | File I/O |
| 60 | SYS_UMASK | Identity |
| 63 | SYS_DUP2 | File I/O |
| 64 | SYS_GETPPID | Process |
| 106 | SYS_STAT | File I/O |
| 108 | SYS_FSTAT | File I/O |
| 158 | SYS_SCHED_YIELD | Process |
| 162 | SYS_NANOSLEEP | Time |
| 190 | SYS_VFORK | Process |
| 200 | SYS_READ_BLOCK | Block Device |
| 201 | SYS_WRITE_BLOCK | Block Device |
| 250 | SYS_SET_PAGE_DIR | MMU |
| 251 | SYS_ENABLE_MMU | MMU |
| 252 | SYS_FLUSH_TLB | MMU |
| 265 | SYS_CLOCK_GETTIME | Time |

### Appendix C: BPF Map Inventory

| Map Name | Type | Max Entries | Value Type | Size | Purpose |
|----------|------|-------------|------------|------|---------|
| ROM_MAP | Array | 262,144 | u32 | 1 MiB | Instruction memory |
| RAM_MAP | Array | 16,777,216 | u32 | 64 MiB | Main memory |
| SCREEN_MAP | Array | 64,000 | u8 | 64 KB | Framebuffer |
| KBD_MAP | Array | 8 | u32 | 32 B | Keyboard queue |
| CPU_MAP | HashMap | 256 | MbcCpuState | ~16 KB | CPU instances |
| STATS | HashMap | 32 | u64 | 256 B | Performance counters |
| L1_CACHE | LruHashMap | 256 | [u8; 64] | 16 KB | Reserved (disabled) |
| RV2MBC_MAP | Array | 65,536 | u32 | 256 KB | Address translation |
| COMPUTE_EVENTS | RingBuf | -- | -- | 256 KB | Event ring buffer |
| TTY_MAP | Array | 4,096 | u8 | 4 KB | Console output buffer |
| TTY_HEAD | Array | 1 | u32 | 4 B | TTY write position |
| PROC_TABLE | Array | 4 | [u32; 20] | 320 B | Process state |
| SCHED_STATE | Array | 4 | u32 | 16 B | Scheduler state |
| TLB_MAP | Array | 64 | [u32; 3] | 768 B | Software TLB |

### Appendix D: STAT Counter Keys

| Key | Name | Description |
|-----|------|-------------|
| 0 | STAT_PACKETS_TOTAL | Total XDP packets received |
| 1 | STAT_CPU_TICKS | CPU tick packets processed |
| 2 | STAT_INSNS_EXECUTED | Total MBC instructions executed |
| 3 | STAT_HALTED | CPU halt events |
| 4 | STAT_SLEEPING | CPU sleep events |
| 5 | STAT_NO_STATE | Ticks with no CPU_MAP entry |
| 6 | STAT_MEM_FAULTS | Memory faults (reserved) |
| 7 | STAT_SYSCALLS | SYSCALL opcode invocations |
| 8 | STAT_ROM_FAULT | ROM access faults / diagnostic events |
| 9 | STAT_MEM_STORES | Memory store operations |
| 10 | STAT_MEM_LOADS | Memory load operations |
| 11 | STAT_FRAME_READY | Frames rendered (DG_DrawFrame calls) |
| 12 | STAT_TIMER_INTERRUPTS | Timer interrupt deliveries |
| 13 | STAT_LINUX_SYSCALLS | INT 0x80 syscall invocations |
| 14 | STAT_TTY_WRITES | TTY write operations |
| 15 | STAT_CONTEXT_SWITCHES | Scheduler context switches |
| 16 | STAT_FORKS | SYS_FORK process creations |
| 17 | STAT_BLOCK_OPS | Block device read/write operations |
| 18 | STAT_TLB_HITS | TLB hits |
| 19 | STAT_TLB_MISSES | TLB misses (page table walks) |
| 20 | STAT_TTY_READS | TTY read operations (fd 0) |
| 21 | STAT_WAITPIDS | SYS_WAITPID calls |
| 22 | STAT_TRIVIAL_SYSCALLS | Trivial FUZIX stub syscalls |
| 23 | STAT_MODERATE_SYSCALLS | Moderate FUZIX syscalls (ioctl, signal, stat, pipe) |

### Appendix E: Event Type Constants

| Value | Name | Description |
|-------|------|-------------|
| 0x01 | Birth | Shield ingress inserted Monad |
| 0x02 | Hop | Intermediate eBPF hop processing |
| 0x03 | Death | Shield egress stripped Monad |
| 0x04 | Anomaly | Checksum failure or decode error |
| 0x05 | Chaos | Yaldabaoth chaos injection |
| 0x10 | EVENT_COMPUTE_HOP | MBC instruction executed |
| 0x11 | EVENT_CACHE_MISS | L1 cache miss |
| 0x12 | EVENT_MEM_WRITE | Dirty page writeback |
| 0x13 | EVENT_MEM_STAGED | Wotan staged page into L1 |
| 0x14 | EVENT_SCREEN_WRITE | DG_DrawFrame emitted |
| 0x15 | EVENT_KEY_READ | DG_GetKey executed |
| 0x16 | EVENT_COMPUTE_HALT | HALT syscall |
| 0x17 | EVENT_COMPUTE_STALL | CPU stall (cache miss, sleep) |
| 0x18 | EVENT_TTY_WRITE | Console output |
| 0x19 | EVENT_CONTEXT_SWITCH | Scheduler context switch |

### Appendix F: MbcCpuState Struct Layout (128 bytes)

```
Offset  Size   Field               Description
──────────────────────────────────────────────────────────────
0x00    64     regs[16]            General-purpose registers r0-r15 (16 x u32)
0x40    4      pc                  Program counter
0x44    1      flags               Status flags (Z, N, C, IF)
0x45    1      halted              1 if HALT executed
0x46    1      stalled             1 if waiting for cache miss
0x47    1      _pad                Alignment padding
0x48    8      sleep_until_ns      bpf_ktime_get_ns sleep target
0x50    8      insn_count          Total instructions executed
0x58    8      cache_hits          Memory store count (legacy name)
0x60    8      cache_misses        Memory load count (legacy name)
0x68    1      interrupt_pending   Non-zero if interrupt waiting
0x69    1      interrupt_vector    Vector number of pending interrupt
0x6A    1      interrupts_enabled  Non-zero if CPU accepts interrupts
0x6B    1      _pad2               Alignment padding
0x6C    4      tick_counter        Timer interrupt tick counter
0x70    4      program_break       Heap end address (default: 0x00400000)
0x74    4      exit_code           Exit code from SYS_EXIT
0x78    1      current_pid         Current process ID (0-3)
0x79    1      num_processes       Number of active processes
0x7A    1      mmu_enabled         0=flat, 1=paging active
0x7B    1      _pad3               Alignment padding
0x7C    4      page_dir_base       Page directory physical address
──────────────────────────────────────────────────────────────
Total: 128 bytes (0x80)
```

Verified by compile-time assertion: `const _: [u8; 128] = [0u8; size_of::<MbcCpuState>()];`

---

*This document is the canonical reference for the UPC architecture. All opcode
values, syscall numbers, struct layouts, and constants are sourced directly from
`ebpf/monad-common/src/lib.rs` and verified against the eBPF execution engine
in `ebpf/monad-cpu-ebpf/src/main.rs`.*
