<!--
SPDX-License-Identifier: GPL-3.0-or-later
-->

# MBC Linux -- Frequently Asked Questions

**Date**: 2026-03-15
**Sprint**: S-LINUX Week 12

---

## The Big Questions

### "Is this real?"

Yes. Every MBC instruction executes inside a real eBPF XDP program attached to
a real Linux network interface. The RAM is a real BPF Array map (64 MB). The
ROM is a real BPF Array map (1 MB). The timer interrupt is a real BPF timer
callback. The console output goes through the Wotan message bus. There is no
emulator running in userspace -- the kernel's packet processing engine IS the
CPU.

You can verify this yourself:

```bash
# Show the XDP program
sudo bpftool prog list | grep monad_cpu

# Show the BPF maps (RAM, ROM, CPU state)
sudo bpftool map list | grep -E "RAM|ROM|CPU"

# Show it's attached to a network interface
ip link show monad0  # xdp/id:XX
```

The one thing that IS emulated: interrupts. Real CPUs have hardware interrupt
lines. MBC uses BPF timer callbacks to set a flag that the execution loop
checks after each instruction. This is functionally identical to how software
interrupts work on real hardware -- it is just triggered by a timer instead
of an electrical signal.

---

### "How fast is it?"

Honest numbers:

| Configuration | Instructions/sec | Comparison |
|--------------|-----------------|------------|
| Single-hop, 35 Hz | ~8,960 | ~1/10th of Intel 4004 (1971) |
| 6-hop turbo, 35 Hz | ~53,760 | Approaching Intel 4004 |
| 6-hop turbo, 100 Hz | ~153,600 | Between 4004 and 6502 |

Boot to shell prompt: ~0.7 seconds (single-hop), ~0.1 seconds (turbo).

Command response times: all under 50 milliseconds. `echo hello` takes about
50 MBC instructions. At 8,960 insns/sec, that is 5.6 ms. Imperceptible.

It is not fast. It was never meant to be fast. The goal is computational
completeness, not competitive performance. That said, it is fast enough to
be interactive -- you can type commands and get responses without noticeable
delay.

---

### "Can it run X?"

If X compiles to RV32I (RISC-V 32-bit integer), then yes -- via the
`rv32i-to-mbc` binary translator.

**Already demonstrated:**
- DOOM (id Software, 1993) -- full game at 35 fps
- Linux kernel (uClinux 6.x) -- boots to shell
- Standard Unix utilities (ls, cat, echo, ps, uname, uptime)

**Theoretically possible:**
- Any C program compiled with `riscv64-unknown-elf-gcc -march=rv32i`
- SNES emulators (SNES9X-2002, ~50K LOC C)
- Interpreters (Lua, Forth, BASIC)
- Compilers (small C compilers like tcc)

**Limitations:**
- No floating point hardware (soft-float only)
- No MMU (flat address space, no fork(), only vfork()+execve())
- 64 MB RAM maximum (BPF map size limit)
- ~9K-54K instructions/sec (CPU-bound applications will be slow)
- No networking stack (yet -- would be IP-in-Monad-in-IPv6, which is absurd
  but technically feasible)

---

### "Why?"

Three reasons, in order of intellectual honesty:

1. **Computational completeness proof.** The Unheaded Protocol (Monad wire
   format) was designed as a network observability protocol. Showing that it
   can execute arbitrary computation -- up to and including booting Linux --
   proves the protocol is computationally general. This has implications for
   what programmable network infrastructure can become.

2. **Resume showcase.** Building a working computer from a network protocol
   and porting Linux to it demonstrates systems programming ability at every
   level: ISA design, compiler toolchains, kernel development, eBPF
   programming, and network protocol design. It is a conversation starter
   that leads to real technical discussions.

3. **Because it is there.** After DOOM ran on the protocol computer, the
   question "can it run Linux?" became irresistible. Each level of the pipe
   dream ladder -- single instruction, program, DOOM, OS primitives, Linux --
   felt impossible until the previous level worked. The whole project is a
   testament to incremental ambition.

---

### "Is it useful?"

As a replacement for real hardware: no. A Raspberry Pi Zero costs $5 and
runs billions of instructions per second.

As a platform and proof of concept: yes.

**What it proves:**
- eBPF is computationally general (not just a packet filter)
- Network protocols can carry computation, not just data
- BPF maps are viable as a memory system (64 MB, O(1) access)
- The boundary between "network processing" and "general-purpose computation"
  is artificial

**What it enables:**
- Research into in-network computing
- A test platform for protocol-level programmability
- Educational tool for teaching CPU architecture (MBC is simple enough to
  understand completely)
- A foundation spec for the Unheaded Protocol's computational capabilities

**What it does NOT replace:**
- Real CPUs (obviously)
- Real operating systems
- Hardware emulators (QEMU, Bochs)
- Cloud VMs

---

### "Can I try it?"

Yes. The code is GPL-3.0-or-later.

**Requirements:**
- Linux host (kernel 5.15+ for BPF features, 6.x recommended)
- eBPF support (CONFIG_BPF=y, CONFIG_BPF_SYSCALL=y, CONFIG_XDP=y)
- Rust toolchain (rustup stable)
- RISC-V cross-compiler (gcc-riscv64-unknown-elf)
- Python 3.10+ (for MBC assembler)
- ROCm not required (that is for the GPU, not the protocol computer)

**Quick start:**
```bash
git clone https://github.com/unheaded/unheaded.git
cd unheaded
# See docs/doom/UPC_BOOT_PROTOCOL.md for build instructions
# See docs/battle-plans/S-LINUX-BATTLE-PLAN.md for the full port guide
```

**Note:** The Linux kernel port requires a separate clone of the Linux source
tree with the `arch/mbc` directory added. Build instructions are in the
battle plan (Appendix A: Toolchain Setup).

---

## Technical Questions

### "How do interrupts work without interrupt hardware?"

BPF timer callbacks. The BPF subsystem provides `bpf_timer_set_callback()`
which fires a callback at a configurable rate. The callback sets
`interrupt_pending = 1` and `interrupt_vector = 0x20` (timer) in the CPU
state. The main execution loop checks these flags after each instruction.
If pending, it saves PC and FLAGS to the stack, disables interrupts, and
jumps to the handler address in the IVT (Interrupt Vector Table at RAM
address 0x0000).

This is functionally identical to edge-triggered hardware interrupts,
except the trigger source is a software timer instead of an electrical
signal. Since MBC is single-core, there are no race conditions between
the timer callback and the execution loop -- BPF programs run atomically
on a single CPU.

### "How do atomics work on a single-core BPF CPU?"

CLI/STI sequences. Since MBC is single-core with no concurrent memory
access, disabling interrupts (CLI) before an atomic operation and
re-enabling (STI) after is sufficient for correctness. The kernel's
`atomic_t` operations map to:

```
CLI           ; disable interrupts
LDW r1, [r0] ; load current value
ADD r1, r2   ; modify
STW r1, [r0] ; store
STI           ; re-enable interrupts
```

This is the same technique used by early x86 kernels on uniprocessor
systems (the `local_irq_disable()` / `local_irq_enable()` pattern).

### "What is the BPF verifier situation?"

The BPF verifier is the main constraint on MBC performance. Every XDP
program must pass static analysis before loading. Key limits:

| Constraint | Kernel 6.x Limit | MBC Usage |
|-----------|-----------------|-----------|
| Total BPF instructions | 1,000,000 | ~6,400 (256 MBC insns x ~25 BPF insns each) |
| Stack depth | 512 bytes | ~200 bytes (local variables + prefetch buffer) |
| Map lookups per program | Unlimited (but each costs ~10 BPF insns) | ~512 (2 per MBC insn: fetch + memory) |
| Tail calls | 33 max depth | 0 (single program) |
| Loop iterations | Must be bounded | 256 (MAX_INSN_PER_TICK constant) |

The execution loop is a `while i < MAX_INSN_PER_TICK` loop, which the
verifier can prove terminates. All map accesses use bounds-checked indices.
The verifier has never rejected the monad_cpu program on kernel 6.x.

### "Why not just use QEMU?"

QEMU emulates hardware in userspace. MBC runs in kernelspace (eBPF).
The distinction matters:

- QEMU: Userspace process -> syscall -> kernel -> hardware
- MBC: Packet arrives -> XDP hook -> BPF program -> BPF maps (all in kernel)

MBC has no userspace involvement in the execution path. Every instruction
executes at kernel privilege level inside the network stack. This means:

1. No context switches between user and kernel space
2. No scheduling overhead (XDP runs to completion)
3. Direct access to kernel data structures (BPF maps)
4. Cache-warm execution (XDP_TX keeps the packet hot in L1)

The tradeoff: MBC is vastly slower than QEMU because BPF programs have
verification overhead and cannot use hardware acceleration. But the point
is not speed -- it is demonstrating that the network processing path is
computationally complete.

### "What about security? You're running an OS inside BPF!"

The BPF verifier ensures safety:

1. **Memory safety**: All map accesses are bounds-checked at load time
2. **Termination**: The execution loop is provably bounded (256 iterations)
3. **No kernel memory access**: BPF programs cannot read/write arbitrary
   kernel memory -- only BPF maps they own
4. **Sandboxed**: MBC Linux cannot affect the host kernel. It can only
   read/write its own BPF maps. A kernel panic inside MBC simply halts
   the MBC CPU (sets HLT flag) -- the host continues normally.
5. **No privilege escalation**: MBC processes run inside BPF, which is
   already sandboxed by the verifier. Even if MBC Linux is "root," it
   has no capabilities on the host.

The security model is: MBC Linux is a sandbox inside a sandbox. The BPF
verifier is the outer sandbox; the MBC kernel is the inner sandbox.

### "How does the toolchain work?"

```
C source (.c)
    |
    v  riscv64-unknown-elf-gcc -march=rv32i -mabi=ilp32
RV32I ELF object (.o)
    |
    v  riscv64-unknown-elf-ld
RV32I ELF executable (vmlinux)
    |
    v  riscv64-unknown-elf-objcopy -O binary
RV32I flat binary (.bin)
    |
    v  rv32i-to-mbc (Rust binary translator)
MBC flat binary (.mbc)
    |
    v  Merge with head.S (assembled by mbc-as)
Final kernel image (vmlinux.mbc)
    |
    v  wotan-ctl boot --kernel vmlinux.mbc
Loaded into BPF ROM_MAP, execution begins
```

The key insight: we do not write an MBC compiler. We reuse the mature
RISC-V GCC toolchain and translate the output. The `rv32i-to-mbc`
translator is a mechanical 1:1 instruction mapping because MBC was
deliberately designed to be a superset of RV32I's core integer operations.

### "What is the Monad wire format?"

The Monad is a 20-byte structure carried in an IPv6 Hop-by-Hop Options
extension header:

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|    Version    |     Flags     |          Flow Label           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         Trace ID                              |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   Hop Count   |    Action     |         Hop Latency           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                        Timestamp                              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

The wire format is frozen at version 0x01. For the UPC use case, the
Monad acts as the CPU's instruction bus: each packet arrival triggers
a tick, and the flow_label identifies which UPC instance to execute.

---

## Skepticism Section

### "This sounds like a toy."

It is a toy -- and that is the point. The 4004 was a toy by modern
standards. The PDP-7 that Ken Thompson wrote Unix on had 9 KB of RAM.
Toys become tools when you figure out why they work.

The value is not in the MBC CPU itself. The value is in demonstrating
that network protocols have untapped computational potential. If eBPF
can run Linux, what else can programmable network infrastructure do?

### "BPF was not designed for this."

Correct. BPF was designed for packet filtering. Then it became a
tracing tool. Then a security enforcement mechanism. Then a networking
datapath. Each new use case pushed BPF's capabilities further.

Running an OS inside BPF is the logical extreme of this trajectory.
It does not mean you should do it in production. It means the
abstraction is more powerful than its original designers intended --
which is a sign of good design.

### "The numbers are unimpressive."

By modern standards, yes. 9,000 instructions per second is glacial.
But consider:

- The CPU has no silicon. Zero transistors. Zero gates.
- It runs inside a packet processing hook designed for filtering
- It shares the host's network stack, scheduler, and memory management
- Despite all this, it boots Linux in under a second

The constraint is the BPF execution model (bounded loops, map access
overhead), not the architecture. With AF_XDP zero-copy injection
(validated at 920K packets/sec in separate testing), the theoretical
instruction throughput jumps to millions per second.

### "Has anyone else done this?"

To our knowledge, no. There are eBPF-based network functions (load
balancers, firewalls, DDoS mitigation), eBPF-based tracing tools, and
eBPF-based security monitors. But we are not aware of a prior attempt
to implement a general-purpose CPU in eBPF and boot an operating system
on it.

If someone has done this before, we would love to know. Email
stevie@bellis.tech.

---

*This FAQ is maintained alongside the MBC Linux port. Last updated
2026-03-15. For technical details, see `docs/UPC_REFERENCE_MANUAL.md`
and `docs/doom/MBC_LINUX_PERFORMANCE.md`.*
