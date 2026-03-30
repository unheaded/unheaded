# Doom-over-IPv6 -- Technical Documentation

**Status:** PLAYABLE. Menus, movement, shooting, doors all work. Stable at 5.9B+ instructions.
**Last updated:** 2026-03-30
**Baseline commits:** 42bbc34d + 46f36f77

## Architecture

Doom runs as RV32I->MBC bytecode inside eBPF XDP programs. Packets carry Monad
headers through a ring of network namespaces. Each hop executes MBC instructions.
The computation IS the network traversal.

```
 linuxdoom-1.10 (C) -> doom.elf (RV32I) -> doom.mbc (MBC bytecode)
                                              |
             doom-runner (Aya) loads into BPF maps
                                              |
    +----------+  IPv6/HbH  +----------+
    |  monad0  |----------->|  monad1  |
    | XDP: 256 |            | XDP: 256 |
    | insns/hop|<-----------|insns/hop |
    +----------+  IPv6/HbH  +----------+
                                              |
             doom-runner bridge -> WebSocket -> browser canvas
```

## Source

- **Doom source:** `~/tmp/projects/DOOM/linuxdoom-1.10/` (pristine id Software GPL-2.0, 62 .c files)
- **WAD:** `~/tmp/projects/doom-related/DOOM.WAD` (12,408,292 bytes, retail Steam)
- **Platform stubs:** `demos/doom/` (MBC replacements for i_video, i_sound, i_net, i_system)
- **Runtime:** `crates/doom-runner/` (Aya-based loader, bridge, memory layout)

## Key Files

| File | Purpose |
|------|---------|
| `crates/doom-runner/src/memory.rs` | **THE** memory layout source of truth |
| `crates/doom-runner/src/bridge.rs` | WebSocket bridge (8-slot kbd queue, bilinear CSS, dynamic palette) |
| `crates/doom-runner/src/main.rs` | Launch pipeline (load, verify, serve) |
| `crates/doom-runner/src/loader.rs` | ELF parser, MBC/WAD/rv2mbc file loaders |
| `crates/doom-runner/src/tail_calls.rs` | Tail call chain configuration |
| `demos/doom/Makefile` | Build pipeline: id DOOM C -> RV32I -> MBC |
| `demos/doom/i_video_mbc.c` | Back buffer rendering, I_FinishUpdate word-copy, palette, keyboard |
| `demos/doom/libc_stubs.c` | 111+ libc functions, JVM-style heap, word-aligned memcpy |
| `demos/doom/gcc_runtime.c` | Software div/mul for RV32I without M extension |
| `demos/doom/crt0.S` | RV32I bare-metal startup (BSS zeroing, stack init) |
| `demos/doom/linker.ld` | Memory regions (ROM, RAM, SCREEN, HEAP, WAD) |
| `demos/doom/w_file_stdc.c` | Static WAD file handles (avoids Z_Malloc corruption) |
| `ebpf/monad-cpu-ebpf/src/main.rs` | XDP MBC executor + tail calls |
| `ebpf/monad-common/src/lib.rs` | Shared types (MbcCpuState, Monad, mbc_mmap) |

## Memory Layout (doom-runner/src/memory.rs)

This is the authoritative layout. All other components must agree with `memory.rs`.

```
0x0000_0000 +--- ROM (1 MiB) --- .text + .rodata via ROM_MAP
            |   MBC instructions (Array<u32>, index = PC)
            |
            |   (addresses below RAM_BASE are also accessible via RAM_MAP)
0x0006_0000 |   PALETTE_ADDR     (768 bytes, written by I_SetPalette via RAM_MAP)
0x0007_0000 |   SCREEN_BASE      (320x200 = 64,000 bytes, via RAM_MAP)
0x0008_FA00 |   (screen end)
            |
0x0010_0000 +--- RAM_BASE
            |   .data + .sdata   (from ELF, loaded at link addresses)
            |   .bss  + .sbss   (zeroed by crt0)
0x0016_8000 |   KBD_ADDR         (keyboard I/O word)
0x001C_0000 +--- HEAP_START      (26 MiB, JVM-style sbrk allocator)
0x01BC_0000 +--- HEAP_END
0x01BF_0000 |   HEAP_PTR_ADDR   (isolated from .data, immune to wild writes)
0x01C0_0000 +--- WAD_BASE        (retail DOOM.WAD, up to 16 MiB)
0x02C0_0000 |   (WAD end, max)
0x0310_0000 +--- STACK_TOP       (grows down)
```

**BPF Map Sizing:**
- ROM_MAP: 262,144 entries (1 MiB instruction space)
- RAM_MAP: 16,777,216 entries x 4 bytes = 64 MiB kernel memory
- SCREEN_MAP: 64,000 entries (320x200 pixels)
- CPU_MAP: 256 instances (HashMap)
- KBD_MAP: 8 entries (circular keyboard queue)
- RV2MBC_MAP: 65,536 entries (RV32I-to-MBC address translation)

## What Works

- id DOOM linuxdoom-1.10 compiles for RV32I, translates to MBC, runs on UPC
- Retail DOOM.WAD loads (12.4 MiB)
- Title screen, demo playback, menus all render correctly
- Player can move (arrows/WASD), fire (Ctrl/L), use doors (Space), switch weapons (1-7)
- HUD renders (ammo, health, armor, weapon sprites)
- Game logic at 35 fps internally, 5.9B+ instructions executed stably
- Back buffer rendering (screens[0] = malloc, word-copy to SCREEN_BASE in I_FinishUpdate)
- 8-slot circular keyboard queue (prevents event overwrite on rapid input)
- Browser auto-repeat suppressed (e.repeat check in JavaScript)
- Bilinear CSS upscale (image-rendering default, not pixelated)
- Dynamic PLAYPAL palette read from RAM (768 bytes at 0x60000)
- Word-aligned memcpy fast path (4x fewer MBC store instructions)
- doom-runner loads program + maps atomically via Aya (no stale pins)
- BTF-enabled tail calls: 16 rounds x 16 insns = 256 insns/hop

## Known Visual Issues (not bugs)

- Banding on some textures is authentic Doom 320x200 nearest-neighbor magnification
- Browser frame rate lower than 35 fps internal rate (bridge reads 16K words per frame poll)
- No sound (by design, no audio hardware)

## Running Doom

See `docs/doom/RUNBOOK.md` for the complete step-by-step operational runbook.

Quick version:

```bash
cd ~/tmp/unheaded

# Build MBC binary from id DOOM source
cd demos/doom && make clean && make && cd ../..

# Build doom-runner
cd crates/doom-runner && cargo build --release && cd ../..

# Build eBPF
make ebpf-monad-cpu

# Launch (sets up ring, loads XDP + data, starts bridge)
sudo ./crates/doom-runner/target/release/doom-runner run \
  --doom-mbc demos/doom/doom.mbc \
  --doom-elf demos/doom/doom.elf \
  --rv2mbc demos/doom/doom.rv2mbc \
  --wad ~/tmp/projects/doom-related/DOOM.WAD \
  --hops 2 &
sleep 6

# Attach XDP to namespaces
PROG_ID=$(sudo bpftool prog list | grep monad_cpu | tail -1 | awk '{print $1}' | tr -d ':')
sudo ip netns exec monad1 bpftool net attach xdpgeneric id $PROG_ID dev veth01p
sudo ip netns exec monad0 bpftool net attach xdpgeneric id $PROG_ID dev veth10p

# Inject packets
SRC_MAC=$(sudo ip netns exec monad0 ip link show veth01 | awk '/ether/ {print $2}')
DST_MAC=$(sudo ip netns exec monad1 ip link show veth01p | awk '/ether/ {print $2}')
sudo nsenter --net=/var/run/netns/monad0 ./bin/doom-go-injector \
  --src-mac "$SRC_MAC" --dst-mac "$DST_MAC" --iface veth01 \
  --count 0 --mode sendmmsg --batch 200 &

# Open browser: http://localhost:16666
# Controls: arrows/WASD=move, Ctrl/L=fire, Space=use, Enter=start, Esc=menu
```

## Related Documentation

| Document | Content |
|----------|---------|
| `docs/doom/ARCHITECTURE.md` | Deep technical architecture, memory map, data flow |
| `docs/doom/RUNBOOK.md` | Complete step-by-step operational runbook |
| `docs/doom/STATUS.md` | Current status, bugs fixed, regression invariants |
| `docs/doom/BASELINE.md` | Accepted baseline definition |
| `docs/doom/FINDINGS.md` | Full debugging narrative (20 bugs, PC corruption discovery) |
| `docs/doom/BATTLE-PLAN.md` | Choose-your-own-adventure execution plan |
| `docs/doom/UPC-PORT-PROCESS.md` | How to port any C program to the UPC |
| `docs/doom/PIVOT-TO-ID-DOOM.md` | Decision record: doomgeneric -> linuxdoom-1.10 |
| `docs/doom/NEXT-STEPS.md` | Prioritized roadmap |
