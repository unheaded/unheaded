# Doom Ring BPF Maps Reference

## Overview

The Doom-over-IPv6 project uses pinned BPF maps to communicate between the XDP program
(running on the packet circulation ring) and userspace services. The doom-bridge service
reads screen data and CPU state from these maps, and writes keyboard input back.

All maps are pinned under: `/sys/fs/bpf/unheaded/doom-ring/maps/`

---

## Screen Framebuffer (SCREEN_MAP)

- **Path:** `/sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP`
- **Type:** BPF_MAP_TYPE_ARRAY
- **Key:** uint32 (pixel offset, 0-63999)
- **Value:** uint8 (palette index, 0-255)
- **Size:** 64000 entries (320 width x 200 height)
- **Access:** Read-only for doom-bridge (XDP program writes)
- **Format:** Row-major, scan top-to-bottom left-to-right

### Screen Buffer Layout

The SCREEN_MAP array is laid out in **row-major** order:
- Width: 320 pixels
- Height: 200 lines
- Total: 64000 bytes (320 x 200)

**Addressing:**
```
offset = y * 320 + x
```

Where:
- y in [0, 199] (row index, top to bottom)
- x in [0, 319] (column index, left to right)

**Example offsets:**
- Top-left corner (0, 0) -> offset 0
- Top-right corner (319, 0) -> offset 319
- Bottom-left corner (0, 199) -> offset 63680
- Bottom-right corner (319, 199) -> offset 63999

**Pixel Format:**
- Each byte is a palette index (0-255)
- Palette index -> RGB via DoomPaletteRGB lookup table
- Palette indices 0-15: Standard VGA 16 colors
- Indices 16-47: Gray shades
- Indices 48-79: Red/orange shades (fire, blood)
- Indices 80-255: Synthetic gradient (MVP approximation)

---

## CPU Map (CPU_MAP)

- **Path:** `/sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP`
- **Type:** BPF_MAP_TYPE_ARRAY
- **Key:** uint32 (instance ID, typically 0xDE)
- **Value:** CpuState struct (104 bytes)
- **Access:** Read-only for doom-bridge

### CpuState Layout (104 bytes)

| Offset | Size | Field | Description |
|--------|------|-------|-------------|
| 0 | 64 | Regs[16] | 16 x uint32 general purpose registers |
| 64 | 4 | PC | Program counter (uint32) |
| 68 | 1 | Flags | CPU flags byte |
| 69 | 1 | Halted | 1 if CPU is halted |
| 70 | 1 | Stalled | 1 if CPU is stalled |
| 71 | 1 | Pad | Padding byte |
| 72 | 8 | SleepUntil | Sleep timer (uint64) |
| 80 | 8 | InsnCount | Total instructions executed (uint64) |
| 88 | 8 | CacheHits | L1 cache hit counter (uint64) |
| 96 | 8 | CacheMisses | L1 cache miss counter (uint64) |

---

## Keyboard Input (KBD_MAP)

- **Path:** `/sys/fs/bpf/unheaded/doom-ring/maps/KBD_MAP`
- **Type:** BPF_MAP_TYPE_ARRAY
- **Key:** uint32 (input slot, always 0)
- **Value:** uint32 (encoded key event)
- **Direction:** doom-bridge writes, XDP program reads and clears

### Key Event Encoding

```
value = (scancode << 1) | pressed_flag
```

- `scancode`: 16-bit keyboard scancode
- `pressed_flag`: 1 for key down, 0 for key up

---

## Statistics (STATS)

- **Path:** `/sys/fs/bpf/unheaded/doom-ring/maps/STATS`
- **Type:** BPF_MAP_TYPE_ARRAY
- **Key:** uint32 (stat_id)
- **Value:** uint64 (counter)

### Stat Keys

| Key | Name | Description |
|-----|------|-------------|
| 0 | XDP_PASS | Packets passed through XDP |
| 1 | PROCESSED | Packets processed by CPU |
| 2 | INSNS | Total instructions executed |
| 3 | HALTED | Halt events |
| 4 | SLEEPING | Sleep events |
| 5 | NO_STATE | Missing CPU state events |
| 6 | MEM_FAULTS | Memory fault events |
| 7 | SYSCALLS | Syscall events |
| 8 | ROM_FAULT | ROM access faults |
| 9 | CACHE_HITS | L1 cache hits |
| 10 | CACHE_MISSES | L1 cache misses |
