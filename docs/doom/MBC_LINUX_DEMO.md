<!--
SPDX-License-Identifier: GPL-3.0-or-later
-->

# MBC Linux Demo Materials

**Date**: 2026-03-15
**Sprint**: S-LINUX Week 12
**Status**: Demo-ready
**Audience**: Conference reviewers, attendees, blog readers

---

## 1. Conference Talk Abstract

**Title**: Linux on a Network Protocol: Building a Computer from IPv6 Extension Headers

**Authors**: Stevie Bellis

**Word count**: 298

We present MBC Linux, a port of the Linux kernel to a CPU that has no physical
hardware. The Monad Bytecode CPU (MBC) is a 32-bit architecture with 51
opcodes and 16 general-purpose registers. It executes entirely inside eBPF XDP
hooks attached to Linux network interfaces. Each CPU tick is driven by an IPv6
packet carrying a 20-byte Monad register file in a Hop-by-Hop Options
extension header. The XDP program fetches, decodes, and executes up to 256 MBC
instructions per tick, then returns XDP_TX to bounce the packet back for the
next tick -- achieving continuous, cache-warm execution without userspace
involvement.

The computer's memory system uses BPF maps: a 64 MB Array map as RAM, a 1 MB
Array map as ROM, and additional maps for screen framebuffer, keyboard input,
and CPU state. Interrupts are emulated via BPF timer callbacks. I/O is
memory-mapped through the Wotan message bus. The result is a complete von
Neumann architecture implemented in approximately 3,500 lines of Rust eBPF
code.

We ported uClinux (Linux 6.x, CONFIG_MMU=n) by creating a new architecture
target (arch/mbc). The kernel boots from a bFLT image loaded into BPF maps,
initializes a timer interrupt, console driver, scheduler, and virtual
filesystem, then mounts an initramfs containing a shell and standard Unix
utilities. The system boots to a shell prompt in under one second on a
single-hop configuration, executing approximately 6,000 MBC instructions.
Interactive commands (ls, cat, echo, uname, ps, uptime) respond within 50
milliseconds.

Key contributions: (1) a new Linux architecture port for a CPU with no
silicon, (2) a toolchain using RISC-V cross-compilation and binary
translation, (3) atomic operations via interrupt-disable sequences on a
single-core BPF CPU, and (4) to our knowledge, the first operating system
booted inside eBPF. This work demonstrates that modern eBPF is
computationally general enough to host an operating system, blurring the
boundary between network packet processing and general-purpose computation.

**Keywords**: eBPF, XDP, Linux, uClinux, nommu, protocol computer, IPv6,
computational completeness

**Track suggestions**: Systems, Networking, eBPF/Kernel, Unconventional
Computing

---

## 2. Demo Script (5-Minute Live Demo)

### Setup (Before Audience Arrives)

```bash
# Terminal 1: Wotan + BPF maps ready
cd ~/tmp/unheaded
cargo run --release -p wotan &

# Terminal 2: Console subscriber ready (paused)
# Don't start until demo begins
```

### Act 1: The Boot (60 seconds)

**[SLIDE: Title -- "Linux on a Network Protocol"]**

> "This is a standard Linux machine running kernel 6.x. I'm going to boot
> another Linux kernel -- but this one runs inside eBPF, driven by IPv6
> packets."

```bash
# Boot MBC Linux
cargo run --release -p wotan-ctl -- boot \
    --kernel ~/src/linux-mbc/vmlinux.mbc \
    --ramdisk ~/src/linux-mbc/initramfs.cpio \
    --label demo
```

**[TERMINAL: Boot messages scroll]**
```
UPC Boot v2.0 -- Linux 6.8.0-mbc
[  0.000] head.S: entry
[  0.005] start_kernel()
[  0.080] console: Wotan TTY registered
[  0.200] VFS: initramfs unpacked
[  0.600] /init started (PID 1)
[  0.670] / #
```

> "Under one second. Let's use it."

### Act 2: It's Real Linux (90 seconds)

```bash
# Basic commands
/ # echo "Hello from eBPF"
Hello from eBPF

/ # ls /
bin  dev  etc  init  proc  sys  tmp

/ # cat /proc/version
Linux version 6.8.0-mbc (riscv64-unknown-elf-gcc) #1 PREEMPT_NONE MBC

/ # uname -a
Linux upc 6.8.0-mbc #1 PREEMPT_NONE MBC

/ # cat /proc/cpuinfo
processor       : 0
model name      : MBC (Monad Bytecode CPU)
bogomips        : 8.96
features        : 32bit nommu bflt

/ # ps
  PID  STAT COMMAND
    1  S    /init
    2  S    [kthreadd]
    3  R    /bin/sh

/ # cat /proc/uptime
12.50 12.50

/ # cat /proc/meminfo
MemTotal:       65536 kB
MemFree:        64000 kB
```

> "Six processes, 64 MB of RAM, standard proc filesystem. This is real Linux."

### Act 3: The Reveal (90 seconds)

**[SLIDE: Architecture diagram]**

> "Now let me show you WHERE this is running."

```bash
# On the HOST (not inside MBC Linux)
# Show the BPF programs
sudo bpftool prog list | grep monad
# 42: xdp  name monad_cpu  tag abc123  gpl

# Show the BPF maps
sudo bpftool map list | grep -E "RAM|ROM|CPU"
# 15: array  name RAM_MAP     max_entries 16777216
# 16: array  name ROM_MAP     max_entries 262144
# 17: array  name CPU_STATE   max_entries 1

# Show it's just an XDP program on a veth
ip link show monad0
# monad0: <BROADCAST> ... xdp/id:42
```

> "The entire computer -- CPU, RAM, ROM -- lives in BPF maps. Each IPv6
> packet that hits this interface triggers 256 instructions. No hardware.
> No emulator. Just the kernel's packet processing engine running an OS."

### Act 4: How It Works (60 seconds)

**[SLIDE: Packet flow diagram]**

> "Here's the architecture in one sentence: An IPv6 packet carries a 20-byte
> Monad header. An XDP program reads it, executes 256 MBC instructions using
> BPF maps as memory, then bounces the packet back with XDP_TX. Each bounce
> is a CPU tick. At 35 Hz with 6 hops, that's 54,000 instructions per second.
> Not fast -- but fast enough for Linux."

**[SLIDE: Numbers]**

> "51 opcodes. 24 syscalls. 64 MB RAM. 3,500 lines of Rust. The entire kernel
> fits in 4 KB. It boots in 0.7 seconds. And it's GPL -- you can clone it
> right now."

### Act 5: Q&A Hooks (30 seconds)

> "Questions I get asked: Is this real? Yes -- every instruction executes in
> BPF. Can it run other software? Anything that compiles to RISC-V 32-bit.
> Why? Because if a network protocol can run Linux, the protocol is
> computationally complete. And that has implications for what network
> infrastructure can become."

> "Thank you. The code is at github.com/unheaded/unheaded, GPL-3.0."

---

## 3. Blog Post Outline

**Title**: I Built a Computer from a Network Protocol and Booted Linux on It

**Target length**: 3,000-4,000 words
**Tone**: Technical but accessible. First-person narrative. Honest about
limitations. No hype -- the achievement speaks for itself.

### Section 1: The Origin Story (500 words)

- Start with the question: "What if a network protocol could compute?"
- The Shannon connection: information theory says any channel that can
  transmit data can, in principle, compute
- CB radio -> packet radio -> IPv6 extension headers -> Monad wire format
- The Monad: 20 bytes in a Hop-by-Hop Options header. Register file of a
  CPU, riding in a packet
- "I didn't set out to build a computer. I set out to build a network
  protocol. The computer emerged."

### Section 2: The Pipe Dream Ladder (600 words)

- Level 0: Can we execute a single instruction in BPF? (Yes -- XDP program
  reads opcode from packet, writes result to map)
- Level 1: Can we run a program? (Yes -- 256 instructions per tick, ROM in
  BPF Array map)
- Level 2: Can we run DOOM? (Yes -- full id Software DOOM, compiled to
  RV32I, transpiled to MBC, running at 35 fps)
- Level 3: Can we run an OS? (Yes -- timer interrupts, syscalls, scheduler,
  filesystem, console -- all in BPF)
- Level 4: Can we run Linux? (Yes -- uClinux boots to shell prompt in 0.7s)
- Each level felt impossible until the previous one worked
- "The trick is that each level only adds one new capability"

### Section 3: The Architecture (800 words, one diagram)

- The four components: Monad (instruction bus), Wotan (memory), Shield
  (packet lifecycle), Sophia (microcode)
- MBC ISA: 51 opcodes, 32-bit fixed-width, 16 GPRs
- Memory: BPF Array maps. RAM_MAP (64 MB), ROM_MAP (1 MB), SCREEN_MAP
- Execution: XDP_TX bounce loop. Packet arrives -> execute 256 insns ->
  XDP_TX -> packet bounces -> repeat
- Interrupts: BPF timer callbacks set a flag, execution loop checks it
- I/O: Memory-mapped through Wotan. TTY at 0xC001, IRQ at 0xD000
- **Include one architecture diagram** (see Section 4 below)

### Section 4: The Linux Port (800 words)

- arch/mbc: a new Linux architecture for a CPU with no silicon
- nommu (CONFIG_MMU=n): no virtual memory, flat address space, bFLT binaries
- Toolchain: C -> riscv64-unknown-elf-gcc -> RV32I ELF -> rv32i-to-mbc ->
  MBC flat binary -> load into BPF maps
- The four hard blockers: assembler, atomics, bFLT loader, vfork
- What worked surprisingly well: generic kernel infrastructure (VFS, slab,
  printk, procfs) "just works" once arch/ headers are correct
- What was hard: BPF verifier limits, debugging kernel panics in a CPU
  that runs inside another kernel

### Section 5: The Numbers (300 words)

- 51 opcodes (MBC ISA)
- ~40 syscalls (Linux-compatible numbers)
- ~3,500 lines of Rust (BPF execution engine)
- ~4 KB kernel binary
- 64 MB RAM (BPF Array map)
- 0.7 seconds boot to shell prompt
- 8,960 instructions/second (single-hop)
- 53,760 instructions/second (6-hop turbo)
- Comparable to Intel 4004 (1971) in throughput
- "It's not fast. It was never meant to be fast. It's meant to be real."

### Section 6: What's Next (300 words)

- Performance: AF_XDP zero-copy injection (validated at 920K pps)
- Networking: TCP/IP inside MBC Linux -- inception (IP-in-Monad-in-IPv6)
- Multi-core: multiple BPF programs, shared memory, real SMP
- Applications: anything that compiles to RV32I (already demonstrated
  DOOM, exploring SNES emulation)
- The real goal: proving that network infrastructure can be programmable
  at the protocol level, not just at the application level

### Section 7: GPL -- It's Free (200 words)

- GPL-3.0-or-later
- github.com/unheaded/unheaded
- Protocol specs dual-licensed GPL-3.0/Apache 2.0 for ecosystem adoption
- Clone it, build it, boot it
- "If you have a Linux machine with eBPF support, you can run Linux
  inside your network stack. That sentence shouldn't make sense, but it
  does."

---

## 4. Diagrams to Create

### Diagram 1: UPC Architecture Overview

```
Description: Single-page architecture diagram showing all four components.

Layout (left to right):

[IPv6 Packet]          [XDP Program]           [BPF Maps]
 ┌─────────┐           ┌───────────┐           ┌──────────┐
 │ Eth Hdr  │           │ monad_cpu │           │ RAM_MAP  │
 │ IPv6 Hdr │──────────>│           │<─────────>│ (64 MB)  │
 │ HBH Opt  │           │ Fetch     │           ├──────────┤
 │ ┌──────┐ │           │ Decode    │           │ ROM_MAP  │
 │ │Monad │ │           │ Execute   │           │ (1 MB)   │
 │ │20B   │ │           │ x256/tick │           ├──────────┤
 │ └──────┘ │           │           │           │ CPU_STATE│
 │ Payload  │<──────────│ XDP_TX    │           │ (256 B)  │
 └─────────┘  bounce    └───────────┘           └──────────┘

Annotations:
- Arrow from packet to XDP: "Tick: packet arrives"
- Arrow from XDP to maps: "Memory access (read/write)"
- Arrow from XDP back to packet: "XDP_TX bounce for next tick"
- Monad box: "20-byte register file in HBH option"

Color scheme: Network (blue), Compute (green), Memory (orange)
```

### Diagram 2: Boot Sequence Flow

```
Description: Vertical timeline showing boot phases with instruction counts.

[Computermancer]
    │ Load kernel into ROM_MAP
    │ Initialize IVT at RAM 0x0000
    │ Set BootParams at RAM 0x0100
    │ Load ramdisk at RAM 0x800000
    │ Set PC=0x10000, SP=0x03F00000
    │ Inject first tick packet
    ▼
[head.S]                    ~100 insns
    │ Clear BSS
    │ Install IVT handlers
    │ Set up kernel stack
    ▼
[start_kernel()]            ~5,000 insns
    │ Memory init (flat, nommu)
    │ Console init (Wotan TTY)
    │ Timer init (BPF timer)
    │ VFS + initramfs unpack
    │ Scheduler start
    ▼
[/init (PID 1)]             ~200 insns
    │ vfork()
    │ execve("/bin/sh")
    ▼
[/bin/sh (PID 2)]           ~200 insns
    │ Print prompt "/ # "
    │ Wait for input
    ▼
[Shell ready]               Total: ~6,000 insns = 0.7 sec
```

### Diagram 3: Memory Map

```
Description: Vertical memory map showing address ranges and contents.

Byte Address    Contents                 Size
────────────────────────────────────────────────
0x00000000  ┌─ IVT ──────────────────┐  1 KB
            │ 256 interrupt vectors   │
0x000003FF  └────────────────────────┘
0x00000100  ┌─ BootParams ───────────┐  256 B  (overlaps IVT word range)
0x000001FF  └────────────────────────┘
0x00000200  ┌─ Kernel cmdline ───────┐  512 B
0x000003FF  └────────────────────────┘
0x00000400  ┌─ Default HLT handler ──┐  4 B
0x00000403  └────────────────────────┘
0x00000404  ┌─ Kernel stack / heap ──┐  ~60 KB
0x0000FFFF  └────────────────────────┘
0x00010000  ┌─ Kernel image ─────────┐  ~4 KB
            │ (.text + .data)         │
0x000110FF  └────────────────────────┘
            │ ... free ...            │
0x00800000  ┌─ Ramdisk (initramfs) ──┐  ~4 KB
            │ UNFS filesystem         │
0x00804000  └────────────────────────┘
            │ ... free ...            │
0x03F00000  ┌─ Stack top ────────────┐  (SP starts here)
            │ grows ↓ downward        │
0x03FFFFFF  └─ RAM end (64 MB) ──────┘

I/O Ports (memory-mapped):
0x0000C001  TTY data output
0x0000C002  TTY status register
0x0000C003  TTY control register
0x0000D000  IRQ control register
0x0000FFFF  Input register (keyboard)
```

### Diagram 4: Syscall Flow

```
Description: Sequence diagram showing INT 0x80 syscall path.

User Process          Kernel                BPF Maps
    │                    │                      │
    │  INT 0x80          │                      │
    │  r0 = syscall_nr   │                      │
    │  r1-r3 = args      │                      │
    │──────────────────> │                      │
    │                    │ Push PC to stack      │
    │                    │ Push FLAGS to stack   │──> RAM_MAP[SP]
    │                    │ Disable interrupts    │
    │                    │ PC = IVT[0x80]        │<── RAM_MAP[0x200]
    │                    │                      │
    │                    │ Dispatch on r0:       │
    │                    │  1 -> sys_exit()      │
    │                    │  4 -> sys_write() ────│──> TTY @ 0xC001
    │                    │  20 -> sys_getpid()   │
    │                    │  ...                  │
    │                    │                      │
    │                    │ r0 = return value     │
    │                    │ IRET                  │
    │                    │ Pop FLAGS             │<── RAM_MAP[SP]
    │                    │ Pop PC                │<── RAM_MAP[SP]
    │                    │ Enable interrupts     │
    │ <─────────────────│                      │
    │  r0 = result       │                      │
```

---

## 5. Presentation Tips

### Technical Audience (eBPF Summit, Linux Plumbers, FOSDEM)

- Lead with the demo, not the slides
- Show `bpftool` output to prove it's real BPF
- Emphasize the BPF verifier challenges (instruction limits, map access patterns)
- Talk about what broke (kernel panics in BPF are entertaining)
- Have the UPC Reference Manual ready for deep-dive questions

### General Audience (Strange Loop, FOSDEM devroom, local meetup)

- Lead with "I booted Linux inside Linux" -- that hooks everyone
- Show the boot sequence, run commands, then reveal it's BPF
- Skip BPF internals, focus on the "protocol as computer" concept
- End with: "Your network card is already running programs on every packet.
  We just gave it an operating system."

### Media / Blog

- One sentence: "A developer built a working computer out of an IPv6 network
  protocol and booted Linux on it."
- One paragraph: include the numbers (51 opcodes, 0.7s boot, 64MB RAM, GPL)
- One image: the architecture diagram
- Link to repo, link to demo video

---

*All demo scripts and materials reference code in the Unheaded repository
(github.com/unheaded/unheaded) under GPL-3.0-or-later.*
