# Doom-over-IPv6 -- Architecture

**Last updated:** 2026-03-30
**Source of truth:** `crates/doom-runner/src/memory.rs`

## Overview

Doom runs as MBC bytecode inside eBPF XDP programs. Each IPv6 packet hop through a
ring of network namespaces executes 256 MBC instructions. The computation IS the
network traversal. A Rust-based bridge (doom-runner) streams the framebuffer to a
browser over WebSocket, and relays keyboard input back.

## Component Diagram

```
+---------------------------+     +----------------------------+
|  id linuxdoom-1.10 (C)   |     |  demos/doom/ (platform)    |
|  62 .c files, GPL-2.0    |     |  i_video_mbc.c  (screen)   |
|  ~/tmp/projects/DOOM/    |     |  i_sound_mbc.c  (no-op)    |
|  linuxdoom-1.10/          |     |  i_net_mbc.c    (no-op)    |
+---------------------------+     |  i_system_mbc.c (syscalls)  |
            |                      |  libc_stubs.c   (111+ fns) |
            +----------+-----------+  gcc_runtime.c  (div/mul)  |
                       |           |  crt0.S         (startup)  |
                       v           +----------------------------+
              riscv64-unknown-elf-gcc
              -march=rv32i -mabi=ilp32
              -ffixed-x16..x31 (16 regs only)
                       |
                       v
                  doom.elf (RV32I bare-metal)
                       |
                       v
                 rv32i-to-mbc (translator)
                       |
                       v
              doom.mbc + doom.rv2mbc
                       |
                       v
+------------------------------------------------------+
|  doom-runner (crates/doom-runner/, Rust/Aya)         |
|                                                       |
|  1. Validates memory layout                          |
|  2. Sets up network ring (monad0, monad1 namespaces) |
|  3. Loads eBPF XDP program (monad_cpu) via Aya       |
|  4. Writes doom.mbc -> ROM_MAP                       |
|  5. Writes doom.elf sections -> RAM_MAP              |
|  6. Writes DOOM.WAD -> RAM_MAP at WAD_BASE           |
|  7. Writes doom.rv2mbc -> RV2MBC_MAP                 |
|  8. Initializes CPU_MAP (pc=0, sp=STACK_TOP)         |
|  9. Configures tail call chain (TAIL_CALL_PROGS)     |
| 10. Starts WebSocket bridge on port 16666            |
+------------------------------------------------------+
            |                        |
     +------v------+         +------v------+
     | Frame Poller |         | KBD Writer  |
     | 60 Hz timer  |         | 8-slot circ |
     | reads RAM_MAP |        | queue write |
     | at 0x60000   |         | to KBD_MAP  |
     | (palette) +  |         +------^------+
     | 0x70000      |                |
     | (pixels)     |         +------+------+
     +------+-------+         | Browser JS  |
            |                 | keydown/up  |
            v                 | e.repeat    |
     +------+-------+        | suppressed  |
     | WebSocket    |         +------^------+
     | Binary frame |                |
     | 768+64000 B  +------->+------+------+
     +------+-------+        | Canvas      |
            |                | 320x200     |
            v                | bilinear    |
     +--------------+        | CSS upscale |
     | Firefox      |        +--------------+
     +--------------+

+-----------------------------------------------------------+
|  eBPF XDP Execution Ring                                  |
|                                                            |
|  +-----------+  IPv6/HbH  +-----------+                   |
|  |  monad0   |----------->|  monad1   |                   |
|  | veth01    |            | veth01p   |                   |
|  | XDP: 256  |            | XDP: 256  |                   |
|  | insns/hop |<-----------| insns/hop |                   |
|  +-----------+  IPv6/HbH  +-----------+                   |
|                                                            |
|  doom-go-injector sends packets via sendmmsg               |
|  Each packet triggers XDP -> 16 tail calls x 16 insns     |
+-----------------------------------------------------------+
```

## Memory Map (source of truth: memory.rs)

All addresses are flat 32-bit physical. RAM_MAP is a BPF Array<u32> with 16M entries
(word-addressed). Every byte address `A` maps to RAM_MAP word index `A / 4`.

```
Address         Region          Size        Notes
-----------     ----------      --------    ----------------------------------
0x0000_0000     ROM_BASE        1 MiB       .text + .rodata (MBC instructions)
                ROM_MAP: 262,144 entries, one u32 per MBC instruction

                (PALETTE and SCREEN are below RAM_BASE but accessed via RAM_MAP)

0x0006_0000     PALETTE_ADDR    768 B       Written by I_SetPalette (256 x RGB)
                                            Bridge reads for dynamic palette

0x0007_0000     SCREEN_BASE     64,000 B    320x200 x 8-bit palette indices
                                            I_FinishUpdate word-copies here
                                            Bridge reads 16,000 words per frame

0x0010_0000     RAM_BASE        768 KiB     .data + .sdata + .bss + .sbss
                                            (from ELF, loaded by doom-runner)

0x0016_8000     KBD_ADDR        4 B         Keyboard I/O word (scancode<<1|pressed)
                                            Read by SYS_GET_KEY syscall

0x001C_0000     HEAP_START      26 MiB      JVM-style sbrk bump allocator
                                            Doom Z_Init zone + pre-init allocs
0x01BC_0000     HEAP_END

0x01BF_0000     HEAP_PTR_ADDR   4 B         Isolated heap pointer (legacy)
                                            Program uses __heap_start linker sym

0x01C0_0000     WAD_BASE        16 MiB max  Retail DOOM.WAD (12.4 MiB actual)
                                            Memory-mapped by doom-runner

0x00F0_0000     DEBUG_BASE      -           Debug scratch region (breadcrumbs)

0x0310_0000     STACK_TOP       grows down  Must be above WAD_BASE + WAD_MAX_SIZE
```

**Consistency constraints** (checked by `memory::validate_layout()`):
1. HEAP_START >= SCREEN_BASE + SCREEN_SIZE
2. WAD_BASE >= HEAP_END
3. WAD_BASE + WAD_MAX_SIZE <= STACK_TOP
4. RAM_MAP_ENTRIES >= STACK_TOP / 4

## BPF Maps

| Map | Type | Entries | Key | Value | Purpose |
|-----|------|---------|-----|-------|---------|
| ROM_MAP | Array<u32> | 262,144 | MBC PC index | MBC instruction | Code storage |
| RAM_MAP | Array<u32> | 16,777,216 | word index | 4 bytes | All memory (data, screen, heap, WAD) |
| SCREEN_MAP | Array<u8> | 64,000 | pixel index | palette index | Legacy screen (unused, RAM_MAP preferred) |
| CPU_MAP | HashMap<u32, MbcCpuState> | 256 | instance ID (0xDE) | 128-byte struct | CPU state (regs, PC, flags) |
| KBD_MAP | Array<u32> | 8 | slot index | scancode<<1 \| pressed | Circular keyboard queue |
| RV2MBC_MAP | Array<u32> | 65,536 | RV32I word index | MBC PC | Address translation for indirect jumps |
| STATS | HashMap<u32, u64> | - | stat ID | counter | Instruction count, frame ready, etc. |
| TAIL_CALL_PROGS | ProgramArray | - | 0 | monad_cpu FD | Self tail-call for 256 insns/hop |

## Data Flow: Frame Rendering

```
1. Doom renders to screens[0] (malloc'd back buffer, NOT SCREEN_BASE)
2. I_FinishUpdate() word-copies screens[0] -> SCREEN_BASE (0x70000)
   - 16,000 word stores (SW instructions) -> RAM_MAP
   - Copy window ~0.7ms vs 28ms render = 40x less tearing
3. I_FinishUpdate() calls mbc_syscall(SYS_DRAW_FRAME) to signal frame complete
4. Bridge frame_poller (60 Hz tokio timer) reads RAM_MAP:
   a. 192 words at PALETTE_ADDR/4 = 768 bytes palette (from I_SetPalette)
   b. 16,000 words at SCREEN_BASE/4 = 64,000 bytes pixels
5. Bridge broadcasts 64,768-byte binary frame via WebSocket
6. Browser JS decodes: palette[0..768] + pixels[768..64768]
7. Canvas putImageData renders 320x200 RGBA at native resolution
8. CSS scales to 960x600 with bilinear interpolation (smooth upscale)
```

## Data Flow: Keyboard Input

```
1. Browser keydown/keyup event fires
2. JavaScript checks e.repeat -> suppressed (prevents auto-repeat flood)
3. JS sends 3-byte binary WebSocket message: [scancode_lo, scancode_hi, pressed]
   - scancode is JS e.keyCode (NOT PC scancode)
4. bridge::kbd_writer receives KeyEvent via mpsc channel
5. Encodes as u32: (scancode << 1) | pressed
6. Circular write to KBD_MAP:
   a. Scans 8 slots starting from write_head
   b. Finds first empty slot (value == 0) -> writes there
   c. If all 8 full -> overwrites write_head (drops oldest)
7. MBC executor SYS_GET_KEY syscall:
   a. Scans all 8 KBD_MAP slots
   b. Returns first non-zero value, clears the slot
   c. Returns 0 if all empty
8. i_video_mbc.c I_StartTic() polls up to 8 times per tic (35 Hz):
   a. Decodes: bit 31 = keydown/keyup, bits 0-15 = JS keyCode
   b. Maps JS keyCode -> Doom KEY_* constants (switch statement)
   c. Posts event_t via D_PostEvent()
```

## Build Pipeline

```
demos/doom/Makefile:
  riscv64-unknown-elf-gcc
    -march=rv32i -mabi=ilp32
    -nostdlib -nostdinc -ffreestanding -fno-builtin -O2
    -ffixed-x16..x31 (MBC uses only x0-x15)
    -DNORMALUNIX -DLINUX
    -isystem include/ (custom libc headers)
    -I ~/tmp/projects/DOOM/linuxdoom-1.10/ (id DOOM headers)

  Sources: 57 id DOOM .c files + 4 MBC platform stubs + 3 support files
  Link:    linker.ld, --gc-sections, --allow-multiple-definition
  Output:  doom.elf (RV32I bare-metal)

  rv32i-to-mbc doom.elf -> doom.mbc + doom.rv2mbc
    85,454 MBC instructions
```

## Video Rendering Architecture

```
screens[0] = malloc(64000)     <- Back buffer (Doom renders here)
screens[1] = malloc(64000)     <- Status bar source
screens[2] = malloc(64000)     <- Wipe start frame
screens[3] = malloc(64000)     <- Wipe end frame
screens[4] = malloc(64000)     <- Temp buffer

SCREEN_BASE = 0x70000          <- Front buffer (bridge reads here)
PALETTE_ADDR = 0x60000         <- Dynamic palette (bridge reads here)

I_FinishUpdate:
  if screens[0] != SCREEN_BASE:
    word-copy screens[0] -> SCREEN_BASE (16K SW instructions)
  mbc_syscall(SYS_DRAW_FRAME)

I_SetPalette:
  byte-copy 768 bytes to PALETTE_ADDR (volatile writes)

Bridge reads both PALETTE_ADDR and SCREEN_BASE from RAM_MAP (not SCREEN_MAP)
because HUD/status bar uses word stores (SW) which only go to RAM_MAP.
```

## Tail Call Chain

The XDP program uses BPF tail calls for higher throughput:
- TAIL_CALL_PROGS[0] = monad_cpu (self-referencing)
- Each invocation: 16 MBC instructions + conditional tail call
- Max 16 tail calls per packet (BPF verifier limit ~33, we use 16)
- Total: 16 rounds x 16 instructions = 256 instructions per hop
- Two hops per circuit: 512 instructions per packet round-trip
