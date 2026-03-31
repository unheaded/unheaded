# Doom Banding Fix — Session Handoff

**Date:** 2026-03-31 05:30 UTC
**Status:** ROOT CAUSE IDENTIFIED — texture composite data is zeroed/corrupted
**Priority:** P0 — this is THE remaining visual bug

## The Bug

Wall textures show horizontal multicolor banding: alternating bands of black, dark red, white, brown where stone/brick textures should be. Affects pillars, some walls, worse in later rooms.

## Root Cause (CONFIRMED)

**Texture composite data is zeroed.** Raw pixel dump during GS_LEVEL gameplay shows:

```
y=32: idx=8, y=33: idx=2, y=34: idx=239, y=35: idx=5, y=36: idx=1...
```

These are palette indices 1-8 (near-black, first few palette entries) with occasional 239 (bright orange). Normal stone textures should have indices 64-95 (brown/grey gradients). The data in `dc_source` is **garbage from zeroed memory**.

**`texturecomposite[0..9]` are ALL NULL** (confirmed via bpftool RAM_MAP read at 0x203AF4). This means `R_GenerateComposite` either never runs or fails to populate the composite texture data.

## What We Proved Works

| Component | Status | Evidence |
|-----------|--------|---------|
| R_DrawColumn | ✅ Executes | 1.2M CALLR calls via STAT counter |
| BSP pipeline | ✅ Full chain | All markers hit: D_Display→R_RenderPV→BSP→Subsector→AddLine→SegLoop |
| CALLR opcode | ✅ Correct | MBC encoding verified (opcode in HIGH byte, bits 24-31) |
| SRA/SRL | ✅ Correct | Separate opcodes, sign extension verified |
| FixedMul/FixedDiv | ✅ Correct | 64-bit multiply preserves precision |
| Palette | ✅ Correct | Dynamic palette from WAD PLAYPAL |
| WAD reads | ✅ Correct | No missing file accesses |
| gamestate | ✅ GS_LEVEL | Transitions correctly during demo/gameplay |

## What's Broken

**`texturecomposite` array entries are NULL.** In `R_GetColumn` (r_data.c), when `texturecolumnlump[tex][col] <= 0`, it uses `texturecomposite[tex]`. If that's NULL, it calls `R_GenerateComposite(tex)`. But the composites are STILL null after init, meaning:

1. `R_GenerateComposite` may not be called (code path skipped)
2. OR it's called but `Z_Malloc` returns memory that gets zeroed/purged
3. OR the composite is built but the pointer is overwritten

## Next Steps (in order)

### Step 1: Check if R_GenerateComposite ever executes
Add `debug_breadcrumb(0xGC01)` at the start of R_GenerateComposite (r_data.c). If it never fires, the lump path (texturecolumnlump > 0) is always taken, and the bug is in W_CacheLumpNum returning zeroed data.

### Step 2: Check W_CacheLumpNum return values
The lump path calls `W_CacheLumpNum(lump, PU_CACHE)`. PU_CACHE means the zone allocator can PURGE this memory when it needs space. If zone memory is tight (12MB), cached lumps get purged and the pointers become dangling. When R_GetColumn returns `lump_data + offset`, the lump_data may have been purged.

**This is the most likely cause: PU_CACHE zone memory purging.**

### Step 3: Fix — increase PU_CACHE retention or use PU_STATIC
In r_data.c, change `W_CacheLumpNum(lump, PU_CACHE)` to `W_CacheLumpNum(lump, PU_STATIC)` for texture lumps. This prevents purging at the cost of more memory. With 12MB zone this should be feasible for E1M1.

OR increase zone memory from 12MB to 16-20MB (previous attempts at 14-20MB had stability issues, but those may have been from stale processes not the zone size itself).

## Key Files

| File | What | Where |
|------|------|-------|
| `linuxdoom-1.10/r_data.c` | R_GetColumn, R_GenerateComposite, texture loading | Source of truth for texture pipeline |
| `linuxdoom-1.10/r_draw.c` | R_DrawColumn inner loop | Uses dc_source (fed by R_GetColumn) |
| `linuxdoom-1.10/r_segs.c` | R_RenderSegLoop | Calls R_GetColumn then colfunc() |
| `linuxdoom-1.10/w_wad.c` | W_CacheLumpNum | Zone-cached WAD lump reading |
| `linuxdoom-1.10/z_zone.c` | Z_Malloc, Z_FreeTags | Zone memory allocator |
| `demos/doom/i_system_mbc.c` | `mb_used = 12` | Zone memory size (line 67) |
| `demos/doom/libc_stubs.c` | sbrk, malloc, WAD read stubs | Memory allocation |
| `crates/doom-runner/src/memory.rs` | Memory layout constants | HEAP_START/END, WAD_BASE |

## Current Build State

- Doom: palette+indices path (no XRGB), back buffer, screenblocks=7
- Bridge: reads palette (768 bytes) + pixels (64000 bytes) from RAM_MAP
- eBPF: has diagnostic counters (STAT 20/21 for CALLR, render chain markers at 0xE3000)
- Zone: 12MB (`mb_used = 12`)
- WAD: dynamic size from doom-runner

## How to Run

```bash
cd ~/tmp/unheaded
# Build everything
cd demos/doom && make clean && make && cp doom.elf doom.mbc doom.rv2mbc ../../doom/ && cd ../..
cd ebpf && cargo build --target=bpfel-unknown-none -Z build-std=core -p monad-cpu-ebpf --release && cd ..
cd crates/doom-runner && cargo build --release && cd ../..

# Launch
sudo ./crates/doom-runner/target/release/doom-runner run \
  --doom-mbc doom/doom.mbc --rv2mbc doom/doom.rv2mbc --doom-elf doom/doom.elf \
  --wad ~/tmp/projects/doom-related/DOOM.WAD --hops 2 &
sleep 8

# Attach XDP + inject
PROG_ID=$(sudo bpftool prog list | grep monad_cpu | tail -1 | awk '{print $1}' | tr -d ':')
sudo ip netns exec monad1 bpftool net attach xdpgeneric id $PROG_ID dev veth01p
sudo ip netns exec monad0 bpftool net attach xdpgeneric id $PROG_ID dev veth10p
SRC_MAC=$(sudo ip netns exec monad0 ip link show veth01 | awk '/ether/ {print $2}')
DST_MAC=$(sudo ip netns exec monad1 ip link show veth01p | awk '/ether/ {print $2}')
sudo nsenter --net=/var/run/netns/monad0 ./bin/doom-go-injector \
  --src-mac "$SRC_MAC" --dst-mac "$DST_MAC" --iface veth01 \
  --count 0 --mode sendmmsg --batch 200 &

# Play at http://192.168.69.184:16666
```

## Session Stats

- ~30 doom-runner restarts
- 100+ commits on main branch
- Investigations: SRA, FixedMul, FixedDiv, gamma, XRGB, resolution, dithering, file I/O, WAD_MAX_SIZE, zone memory, CALLR opcode, render pipeline markers, gamestate, texture composites
- Key breakthrough: raw pixel data shows garbage indices (1,2,5,6,8,239) not texture data
- All texturecomposite[0..9] are NULL — texture cache is failing
