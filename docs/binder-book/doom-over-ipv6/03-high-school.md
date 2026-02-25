# Doom Over IPv6: The High School Version

## Overview

This document explains how the classic 1993 FPS game DOOM runs inside Linux eBPF XDP programs, using IPv6 packet circulation as the execution clock. Prior familiarity with basic programming concepts (variables, loops, functions) and networking (IP addresses, packets, headers) is assumed.

## The Architecture in One Diagram

```
                        ┌──────────────────────────────────┐
                        │     Kernel BPF Maps (Shared)     │
                        │                                  │
                        │  ROM_MAP   - game bytecode       │
                        │  RAM_MAP   - game memory (64MB)  │
                        │  CPU_MAP   - registers, PC       │
                        │  SCREEN_MAP - framebuffer         │
                        │  KBD_MAP   - keyboard input      │
                        │  RV2MBC_MAP - addr translation   │
                        └──────────┬───────────────────────┘
                                   │ read/write on every packet
                                   │
    ┌──────┐  pkt  ┌──────┐  pkt  ┌──────┐
    │monad0│──────→│monad1│──────→│monad2│
    │ XDP  │       │ XDP  │       │ XDP  │
    └──┬───┘       └──────┘       └──┬───┘
       ↑                              │
    ┌──┴───┐       ┌──────┐       ┌──┴───┐
    │monad5│←──────│monad4│←──────│monad3│
    │ XDP  │       │ XDP  │       │ XDP  │
    └──────┘       └──────┘       └──────┘

    ┌─────────────┐         ┌──────────────┐
    │  Injector   │         │  doom-bridge  │
    │ (Go binary) │         │  (Go binary)  │
    │ sends pkts  │         │  reads SCREEN │
    │ into monad0 │         │  → WebSocket  │
    └─────────────┘         └──────┬───────┘
                                   │
                            ┌──────┴───────┐
                            │   Browser    │
                            │ HTML5 Canvas │
                            │  + keyboard  │
                            └──────────────┘
```

## Part 1: The Virtual CPU

### What Is MBC?

Doom was originally written in C for x86 processors. To run it in BPF, the code goes through a multi-step compilation:

```
C source code
    ↓  gcc (cross-compiler targeting RISC-V 32-bit)
RISC-V machine code (.elf)
    ↓  rv32i-to-mbc translator
MBC bytecode (.mbc) + address map (.rv2mbc)
    ↓  doom-loader
BPF kernel maps
```

**MBC (Monad Bytecode)** is a custom 32-bit instruction set designed to be executable inside BPF's constraints. Each instruction is exactly 32 bits:

```
┌────────┬────┬────┬──────────────────┐
│ opcode │ dst│ src│    immediate     │
│ 8 bits │ 4b │ 4b │    16 bits      │
└────────┴────┴────┴──────────────────┘
```

The virtual CPU has:
- **16 registers** (r0-r15), each 32-bit. r15 is the stack pointer.
- **A program counter** (PC) pointing to the current instruction in ROM_MAP
- **Flags**: Zero, Negative, Carry — set by arithmetic/comparison operations
- **An instruction counter** tracking total instructions executed

The instruction set includes everything a real CPU needs: arithmetic (ADD, SUB, MUL, DIV), logic (AND, OR, XOR, shifts), memory access (LD, ST with byte/half/word variants), branches (JMP, JZ, JNZ, etc.), function calls (CALL, RET), stack operations (PUSH, POP), and system calls (SYSCALL for I/O).

### How One "Tick" Works

When a packet hits an XDP program, the BPF program:

```
1. Parse packet headers → extract instance ID from IPv6 Flow Label
2. Load CPU state from CPU_MAP[instance_id]
3. Check: is CPU halted or sleeping? If so, drop packet
4. Execute loop (up to 256 iterations):
   a. Fetch instruction: word = ROM_MAP[cpu.pc]
   b. Decode: opcode, dst, src, immediate
   c. Execute: arithmetic, memory access, branch, or syscall
   d. Advance PC (unless branch taken)
   e. Increment instruction counter
5. Save CPU state back to CPU_MAP[instance_id]
6. Increment hop counter in packet
7. If hop counter < 255: return XDP_TX (bounce packet)
   If hop counter >= 255: return XDP_DROP (packet exhausted)
```

**XDP_TX** is the key mechanism. Instead of forwarding the packet through the kernel's IP stack (slow), XDP_TX reflects it back out the same interface — directly back to the veth peer. The packet bounces between the two ends of the veth pair, triggering the XDP program on each bounce, keeping the data hot in CPU cache.

## Part 2: The Network Ring

### Why Six Namespaces?

Linux network namespaces provide isolated network stacks. Each namespace has its own interfaces, routing tables, and iptables rules. By creating six namespaces and connecting them in a ring with veth pairs, we get six points where XDP programs can intercept packets.

The BPF program is loaded **once** into the kernel (on hop 0) and then the same program instance is attached to all six interfaces. All six attachments share the same BPF maps — this is critical. Every hop reads and writes the same ROM, RAM, CPU state, and screen buffer.

### IPv6 Specifics

The injected packet is a minimal 78-byte IPv6 packet:

```
Ethernet (14 bytes): dst MAC, src MAC, EtherType 0x86DD
IPv6 (40 bytes):
  - Version: 6
  - Flow Label: 0x000DE (low 8 bits = instance ID 0xDE)
  - Hop Limit: 255 (decremented at each bounce)
  - Next Header: Hop-by-Hop Options
  - Src: fd00:3f:75::1
  - Dst: fd00:dead::1 (unreachable — forces default route forwarding)
Hop-by-Hop Options (4 bytes):
  - Option Type 0x3E (Monad register)
  - 20 bytes of control data
Monad Register (20 bytes):
  - Version, flags, hop counter, reserved
```

The destination address `fd00:dead::1` is deliberately outside any connected subnet. This forces the kernel to use the default route at each namespace, forwarding the packet to the next hop in the ring — where XDP intercepts it again.

### The Bus Metaphor

In a traditional CPU, the **front-side bus** connects the processor to memory. Clock signals on the bus trigger read/write cycles. The bus speed directly determines how fast data moves between CPU and RAM.

In this architecture:
- **The veth ring IS the bus** — the physical medium connecting compute to memory
- **Each packet IS a clock cycle** — it triggers one burst of computation
- **BPF maps ARE the memory hierarchy** — ROM_MAP is instruction memory, RAM_MAP is data memory, SCREEN_MAP is memory-mapped I/O
- **The injector IS the clock crystal** — it determines the frequency by controlling packet rate

The analogy is structurally precise. A faster injector (more packets/sec) directly increases the "clock speed" of the virtual CPU, just as a faster crystal oscillator speeds up a real processor.

## Part 3: The Display Pipeline

### Framebuffer Architecture

Doom renders to a 320x200 pixel framebuffer using 8-bit palette indices (0-255). Each index maps to an RGB color in Doom's PLAYPAL palette.

In BPF, this framebuffer lives in SCREEN_MAP — a 64,000-entry byte array. When the MBC CPU executes a memory write to the framebuffer region (addresses 0xC000-0xF8BF), the XDP program writes both to RAM_MAP and to SCREEN_MAP simultaneously.

### The Bridge

The **doom-bridge** is a Go program that:

1. Opens the pinned SCREEN_MAP via `bpf()` syscall
2. Uses `BPF_MAP_LOOKUP_BATCH` to read all 64,000 pixels efficiently
3. Sends raw palette indices over WebSocket as binary frames
4. Polls at 60 Hz (~16ms interval)

The batch lookup is important — reading 64,000 individual map entries would be far too slow. The batch syscall reads the entire array in one kernel call.

### The Viewer

The HTML viewer (`demos/doom/index.html`) contains Doom's original 256-color VGA palette (768 bytes — 256 entries x 3 bytes RGB). On each WebSocket message:

```javascript
for (let i = 0; i < 64000; i++) {
    const palIdx = data[1 + i];          // palette index from BPF
    pixelBuf[i*4 + 0] = VGA_PALETTE[palIdx * 3 + 0]; // R
    pixelBuf[i*4 + 1] = VGA_PALETTE[palIdx * 3 + 1]; // G
    pixelBuf[i*4 + 2] = VGA_PALETTE[palIdx * 3 + 2]; // B
    pixelBuf[i*4 + 3] = 255;             // A (opaque)
}
ctx.putImageData(imageData, 0, 0);        // draw to canvas
```

Rendering at native 320x200 then scaling to display size via `drawImage()`.

## Part 4: Keyboard Input

Input flows in the reverse direction:

```
Browser keypress → WebSocket binary message → doom-bridge → KBD_MAP write
```

The browser maps standard key codes to Doom scancodes and sends 4-byte binary messages: `[0x02, scancode_lo, scancode_hi, pressed]`.

The doom-bridge writes `(scancode << 1) | pressed` to `KBD_MAP[0]`.

When Doom's game loop calls `DG_GetKey()`, the MBC SYSCALL instruction triggers `SYS_GET_KEY`, which reads and clears KBD_MAP[0]. This is single-shot — each keypress is consumed once.

## Part 5: Performance

### Throughput Math

| Parameter | Value |
|-----------|-------|
| Instructions per tick | 256 (MAX_INSN_PER_TICK) |
| Bounces per packet | ~255 (hop limit) |
| Instructions per packet | ~65,280 |
| Injection rate (fast mode) | ~460,000 pkt/s |
| Theoretical throughput | ~30 billion insns/s |
| Instructions per Doom frame | ~1,470,000 |
| Theoretical FPS (full screen) | ~20 fps |

### Why Smaller Screen = Smoother

Doom's renderer draws columns and spans. Fewer pixels = fewer draw calls = fewer instructions per frame. At the smallest screen size, Doom might need only ~500K instructions per frame instead of 1.47M — tripling the effective frame rate.

### BPF Verifier Constraints

The BPF verifier statically analyzes every program before loading. It enforces:
- **No infinite loops**: All loops must have a provable bound
- **No out-of-bounds memory access**: All map accesses must be checked
- **Jump complexity limit**: 8,192 total explored states
- **Instruction limit**: 1,000,000 verified instructions

MAX_INSN_PER_TICK = 256 is the highest value that passes the verifier with the current instruction decoder's branch complexity. The verifier explores every possible opcode path at each loop iteration, so complexity grows as O(opcodes x iterations).

---

*Previous: [← Middle School Explanation](02-middle-school.md) | Next: [PhD/Staff-Level Explanation →](04-staff-phd.md)*
