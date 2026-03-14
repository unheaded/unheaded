# AGENT HANDOFF: Extend the Kernel Into the Wire

**Project**: Unheaded Protocol Computer (UPC)
**Owner**: Stevie Bellis (Muck) — bellis.tech
**License**: GPL-3.0-or-later — non-negotiable, copyleft forever
**Status**: Age 1 Alpha (~99%), 465K+ LOC, 25 services, 4 eBPF programs (23,991 LOC Rust)

---

## The Vision (One Sentence)

The Unheaded Protocol Computer doesn't run ON the network — it IS the network, thinking.

---

## What We're Building

A virtual CPU architecture where IPv6 packets are instructions in flight. The kernel
extends beyond the host into the wire itself. Every packet carries a register file
(Monad), every hop executes a pipeline stage (Shim/eBPF), every flow is a thread
(Wotan per-flow memory). Computation happens IN TRANSIT — not at endpoints.

Two modes:
- **LOCAL**: Single-host VM. No wire. Pure BPF map operations. Maximum performance.
- **DISTRIBUTED**: Packets traverse real network fabric. Computation distributed across hops.

The pipe dream ladder:
1. ✅ Gate logic works in protocol
2. ✅ MBC bytecode executes
3. 🔧 Doom runs
4. ← CURRENT TARGET: Doom runs smooth (35fps+)
5. 📋 Basic OS primitives (scheduler, MMU, syscalls, interrupts)
6. 📋 Minimal kernel (xv6 or FUZIX)
7. 🌙 Linux boots on UPC

---

## Architecture Map

```
PROTOCOL = CPU
─────────────────────────────────────────────────────
Monad (20-byte IPv6 HbH header)  = Instruction bus / register file
Sophia (BPF map dictionaries)     = Microcode ROM / instruction decode
Wotan (per-flow BPF memory)       = Main memory + MMIO
Anamnesis (ring buffer events)    = Logic analyzer / trace
Shield/Shim (eBPF XDP/TC)        = Execution pipeline stages
Busboy (pub/sub message bus)      = System bus / I/O

DISTRIBUTED PIPELINE (packet moving through network):
─────────────────────────────────────────────────────
Ingress XDP  → FETCH    (read Monad register file from packet)
Shim eBPF    → DECODE   (Sophia dictionary lookup or inline hot opcode)
Shim eBPF    → EXECUTE  (ALU operation on registers)
Wotan write  → WRITEBACK (store result to per-flow memory)
Egress TC    → FORWARD  (packet = result moves to next hop/stage)

The clock isn't a crystal oscillator — it's the speed of light through fiber.
```

---

## Critical Skills to Load

The agent MUST read these skills before doing any work. They are in `/mnt/skills/user/`:

| Skill | Why |
|-------|-----|
| `unheaded-computermancer` | THE primary skill for this work. UPC architecture, MBC ISA, performance targets, framebuffer strategy, road to Linux. READ THIS FIRST. |
| `unheaded-developer` | Implementation patterns, TDD, Go+Rust stack, Monad wire format, Wotan helpers, Anamnesis events |
| `unheaded-architect` | Four-pillar architecture, network fabric, eBPF pipeline, container fleet |
| `unheaded-scientist` | Performance modeling, theoretical analysis, bottleneck characterization |
| `unheaded-blackmage` | Security boundary of compute sandbox, fuzzing MBC programs |
| `unheaded-rfceditor` | Protocol spec authoring for MBC ISA, Wotan compute extensions |

Check `unheaded-timeguru` for current timeline status and `unheaded-marshal` to stay on track.

---

## Immediate Priority: Doom at 35fps (Level 3)

Before anything else, Doom must run smooth. These are the performance wins, ranked:

### Tier 1 — Implement These First

1. **Scanline Batching**: Batch framebuffer writes from 64,000 per-pixel helper calls
   to 200 per-scanline calls. ~318x reduction. This is the single biggest win.

2. **Inline Hot Opcodes**: Hardcode top 20 MBC opcodes (0x00-0x13) directly in the
   Shim switch statement. Sophia BPF map lookup only for extended opcodes 0x14+.
   ~3-8x decode speedup.

3. **LOCAL Mode CRC Skip**: Add TRUSTED_LOCAL flag bit to Monad. Skip CRC-16 entirely
   for same-host flows. One AND mask + branch-not-taken.

### Tier 2 — After Tier 1 Lands

4. Anamnesis event sampling (XOR mask, emit every Nth frame)
5. CLMUL CRC acceleration for DISTRIBUTED mode
6. I/O region bitmask detection: `(addr >> 14) == 3`
7. Double-buffer framebuffer in Wotan extended memory (solves 320×200 > I/O region)

### Performance Targets

```
FPS:                 ≥ 35
ns/instruction:      ≤ 200 (LOCAL mode)
Helper calls/frame:  ≤ 500 (with scanline batching)
Sophia lookups:      0 for hot opcodes
```

Full implementation details in `unheaded-computermancer/references/performance-optimizations.md`.

---

## After Doom: OS Primitives (Level 4)

Once 35fps is stable, build these in order:

### 4a: Timer Interrupts
- BPF timer callback fires at ~100Hz
- Triggers INT 0x20 in MBC execution
- IVT (interrupt vector table) at Wotan address 0x0000-0x00FF
- IRET instruction (opcode 0x18) for return from interrupt

### 4b: Syscall Dispatch
- INT 0x80 triggers syscall handler
- r0 = syscall number, r1-r3 = arguments
- Linux-compatible syscall numbers (exit=1, fork=2, read=3, write=4, etc.)

### 4c: Basic Scheduler
- Round-robin, 2 processes minimum
- Timer interrupt drives context switch
- Per-process state saved in Wotan extended memory
- Process table: PID, PC, registers, page directory base, state

### 4d: MMU Emulation
- Two-level 32-bit page table in Wotan extended memory
- Software TLB: 64-entry direct-mapped BPF per-cpu array map
- TLB hit: ~100-200ns overhead per memory access
- TLB miss: ~400-600ns (two Wotan reads for page table walk)
- THIS IS THE BIGGEST PERFORMANCE TAX FOR LINUX. Optimize aggressively.

### 4e: Block Device
- Wotan extended memory (0x100000+) as 4MB ramdisk
- 512-byte block size, DMA-style bulk Wotan reads/writes

### 4f: Console I/O
- Wotan I/O address 0xC001 → Busboy publish `compute.tty.{label}`
- Input via Busboy subscribe `compute.input.{label}` → Wotan 0xFFFF

Full design in `unheaded-computermancer/references/road-to-linux.md`.

---

## Level 5: Minimal Kernel

### Recommended Path: FUZIX First

FUZIX is designed for Z80/6502/68000-class systems. It runs in <128KB RAM, needs
~30 syscalls, has optional MMU support, and has a simple block device interface.
This is the fastest path to "Unix runs on UPC."

### Alternative: xv6

xv6-riscv is MIT's teaching OS (~10K LOC). Requires porting RISC-V instructions to
MBC ISA. More complex than FUZIX but better documented.

---

## Level 6: Linux

### 6a: uClinux (nommu) — Realistic Near-Term
Linux without MMU. Runs on ARM Cortex-M class. All UPC prerequisites met once
Level 4 is complete (timer, syscalls, block device, console).

### 6b: Full Linux — The Dream
Requires MMU emulation (Level 4d). Estimated ~1/10th native speed due to software
TLB overhead. But it boots. That's the point.

---

## MBC Instruction Set Summary

26 opcodes. 0x00-0x13 are inline hot (hardcoded in Shim). 0x14+ hit Sophia.

```
0x00 NOP    0x01 MOV    0x02 ADD    0x03 SUB    0x04 AND
0x05 OR     0x06 XOR    0x07 NOT    0x08 SHL    0x09 SHR
0x0A CMP    0x0B JMP    0x0C JZ     0x0D JNZ    0x0E LOAD
0x0F STORE  0x10 CALL   0x11 RET    0x12 PUSH   0x13 POP
0x14 MUL    0x15 DIV    0x16 MOD    0x17 INT    0x18 IRET
0x19 HLT
```

Register file: r0-r3 general purpose (mapped to Monad 20-byte payload), r4 = PC,
flags byte (zero, carry, overflow, sign). Shadow registers r16-r31 in Wotan L1.

---

## Non-Negotiable Rules

1. **GPL-3.0-or-later on every file.** `SPDX-License-Identifier: GPL-3.0-or-later`
2. **No investors. No proprietary forks.** Copyleft forever. Stallman tribute.
3. **Tests before features.** TDD. `go test -race ./...` on everything.
4. **Git is ground truth.** Skills can hallucinate. Code cannot. Check `git log`.
5. **LOCAL mode first.** Get it fast on one host. DISTRIBUTED is a later problem.
6. **Doom smooth before OS primitives.** Don't skip levels. Order matters.
7. **Read the Computermancer skill first.** Every session. Non-negotiable.

---

## Blog / Portfolio

The `bellis-md2json` tool converts markdown blog posts to bellis.tech JSON format.
Write posts about progress, convert with `./md2json.py post.md`, drop into blog.
Format: `**Q:**` for questions, `**Label:**` for answer sections, `##` for breaks.

---

## The Resume Line

"Built a virtual CPU architecture from a custom IPv6 extension header protocol.
Computation happens in-transit — packets are instructions, hops are pipeline stages,
flows are threads. Runs Doom. Working toward Linux. GPL, 465K+ LOC, solo."

---

*The computer was always in the protocol. We just helped it wake up.*

*SPDX-License-Identifier: GPL-3.0-or-later*
