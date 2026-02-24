# Doom Over IPv6: The Middle School Version

## What Even Is This?

Someone got the classic 1993 video game DOOM running inside the Linux kernel's network system. Not as a normal program — as a *side effect of packets bouncing around a network*.

If that sounds insane, it is. Let's break it down.

## Computers Inside Computers

Your computer's network card can run tiny programs called **BPF programs**. These programs were originally designed to look at network packets and decide what to do with them — keep, drop, or forward. They're extremely fast because they run in the kernel, right next to the hardware.

The trick: nobody said these programs *only* have to look at packets. What if, every time a packet arrives, the program also does some math? Like, say, running a few instructions of a video game?

## The Packet Circulation Ring

Picture six rooms connected in a circle by hallways:

```
Room 0 → Room 1 → Room 2
  ↑                    ↓
Room 5 ← Room 4 ← Room 3
```

Each room is a Linux **network namespace** — basically a virtual mini-network inside your computer. They're connected by virtual Ethernet cables called **veth pairs**.

Each room has the same BPF program installed at its door. When a packet arrives at any room, the program:

1. Wakes up
2. Executes **256 game instructions**
3. Sends the packet to the next room
4. Goes back to sleep

The packet is an **IPv6 packet** — the next-generation internet protocol. It has a "hop limit" field (like a countdown timer) that starts at 255. Each room decrements it by 1. When it reaches 0, the packet is dropped.

So one packet triggers: **256 instructions x 255 hops = ~65,000 instructions**

## The Bus Analogy

Think of a real CPU. It has a clock crystal that sends electrical pulses millions of times per second. Each pulse tells the CPU "do your next instruction."

In our system, **the packet IS the clock pulse**. The network ring is the bus — the communication channel between the processor (BPF program) and its memory (BPF maps). Each time a packet arrives, it's like one tick of the clock.

## Where Does Doom Live?

Doom doesn't travel inside the packets. The packets are nearly empty — just 78 bytes with headers.

The actual game lives in **BPF maps** — shared memory regions inside the kernel:

| Map | What It Stores | Size |
|-----|---------------|------|
| **ROM_MAP** | The game's compiled code (76,128 instructions) | 1 MB |
| **RAM_MAP** | The game's working memory | 64 MB |
| **SCREEN_MAP** | The 320x200 pixel framebuffer | 64 KB |
| **CPU_MAP** | The virtual CPU's registers and program counter | 104 bytes |
| **KBD_MAP** | Keyboard input from the player | 32 bytes |
| **RV2MBC_MAP** | Address translation table | 256 KB |

All six rooms share the same maps. It doesn't matter which room processes the packet — they all read and write the same game state.

## How You See the Game

A separate program called the **doom-bridge** runs in normal userspace (outside the kernel). Every 16 milliseconds (~60 times per second), it:

1. Reads the SCREEN_MAP from the kernel (64,000 bytes of pixel data)
2. Sends it over a WebSocket to your browser
3. Your browser looks up each pixel's color in Doom's 256-color palette
4. Draws it on an HTML5 canvas

## How You Control the Game

When you press a key in the browser:

1. Browser sends the key code over WebSocket to doom-bridge
2. doom-bridge writes it to KBD_MAP in the kernel
3. Next time the BPF program runs Doom's input-checking code, it reads KBD_MAP
4. Doom processes the keypress (move forward, shoot, open door, etc.)

## The Injector: Keeping the Clock Ticking

Remember, no packets = no work. A **Go program** called the injector sits inside Room 0 and fires packets as fast as possible:

```
~460,000 packets per second
x 65,000 instructions per packet
= ~30 billion instructions per second
```

At roughly 1.5 million instructions per frame, that's about **20 frames per second** — playable!

## Why Is This Cool?

1. **It proves BPF is Turing-complete** — you can compute anything, not just filter packets
2. **It runs entirely in kernel space** — no userspace process is running the game logic
3. **The network IS the computer** — packets are clock signals, the wire is the bus, BPF maps are memory
4. **It's an actual playable game** — you can navigate menus, start levels, and move around

---

*Previous: [← ELI5 Explanation](01-eli5.md) | Next: [High School Explanation →](03-high-school.md)*
