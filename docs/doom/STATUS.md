# Doom-over-IPv6 Status

**Last verified:** 2026-03-30
**Baseline commits:** 42bbc34d + 46f36f77
**Source:** id Software linuxdoom-1.10 (GPL-2.0)
**WAD:** Retail DOOM.WAD (12.4 MiB, Steam)

## MILESTONE: PLAYABLE DOOM

id Software's DOOM (linuxdoom-1.10) is RUNNING and PLAYABLE over IPv6 on the
Unheaded Protocol Computer. Player can move, shoot, open doors, navigate menus.

### What the player sees
- Title screen renders correctly with proper PLAYPAL colors
- Demo playback works (Doom plays itself in attract mode)
- Pressing Enter starts a new game
- Player can move with arrow keys/WASD, fire with Ctrl/L, open doors with Space
- HUD renders (ammo, health, armor, weapon)
- Sprites solid, weapon visible
- Menus stable, navigation responsive
- Game logic runs at 35 fps internally
- 5.9B+ instructions executed with zero crashes

### Key technical features at this milestone
- **Back buffer rendering**: screens[0] = malloc'd, word-copy to SCREEN_BASE in I_FinishUpdate
- **8-slot keyboard circular queue**: bridge writes to all 8 KBD_MAP slots with circular scan
- **Browser auto-repeat suppressed**: `e.repeat` check in JavaScript keydown handler
- **Bilinear CSS upscale**: `image-rendering: pixelated` removed for smoother scaling
- **Word-aligned memcpy**: fast path copies 4 bytes at a time (4x fewer MBC stores)
- **Dynamic PLAYPAL**: bridge reads palette from RAM_MAP at 0x60000

## VERIFIED WORKING

| # | Feature | Status | Evidence |
|---|---------|--------|----------|
| 1 | id DOOM compiles for RV32I MBC | WORKING | `make` succeeds, doom.mbc = 85,454 instructions |
| 2 | Retail DOOM.WAD loads (12.4 MiB) | WORKING | "IWAD magic verified" in doom-runner |
| 3 | 35 fps game logic | WORKING | STAT_FRAME_READY delta = 35/sec |
| 4 | Screen pixels render (62,754/64,000) | WORKING | bpftool map dump confirms |
| 5 | Dynamic PLAYPAL palette | WORKING | I_SetPalette writes to 0x60000 |
| 6 | Back buffer render + word-copy | WORKING | screens[0]=malloc, I_FinishUpdate copies |
| 7 | Bridge reads RAM_MAP at 0x70000 | WORKING | Firefox shows frames |
| 8 | Keyboard input (8-slot circular queue) | WORKING | Player moves, fires, opens doors |
| 9 | Menu navigation (Enter, Escape) | WORKING | New Game starts, menu works |
| 10 | Weapon switching (1-7 keys) | WORKING | Keys mapped via JS keyCode |
| 11 | Tail call chain (256 insn/tick) | WORKING | BTF enabled, self tail-call |
| 12 | doom-runner Aya pipeline | WORKING | Atomic program+map ownership |
| 13 | Soft-float IEEE 754 (25 funcs) | WORKING | Integer bit manipulation |
| 14 | sprintf precision (%.3d) | WORKING | Font lump names correct |
| 15 | JVM-style dynamic heap (sbrk) | WORKING | __heap_start linker symbol |
| 16 | POSIX fd stubs | WORKING | WAD reads via fd table |
| 17 | gamemode=retail (Ultimate DOOM) | WORKING | access() matches doomu.wad |
| 18 | 5.9B+ instructions executed | WORKING | No crashes, no PC corruption |
| 19 | Browser auto-repeat suppressed | WORKING | e.repeat check in JS |
| 20 | Bilinear CSS upscale | WORKING | Smooth texture rendering |

## KNOWN ISSUES (not blocking -- game is playable)

| Issue | Symptom | Cause | Priority |
|-------|---------|-------|----------|
| Texture banding | Horizontal color bands on walls close up | Authentic Doom 320x200 nearest-neighbor magnification | NOT A BUG |
| Browser frame rate | ~2-3 fps visible despite 35 fps internal | Bridge reads 16K words per frame poll | MEDIUM |
| No sound | Silent | By design (no-op stubs, no audio hardware) | LOW |

## BUGS FIXED -- id DOOM SESSION (11 total)

| # | Bug | Fix | Commit |
|---|-----|-----|--------|
| 1 | sprintf 32-bit overflow | Cap size at 0x7FFFFFFF | 459210c6 |
| 2 | fd read returns -1 | Resilient fallback to slot 0 | 61648393 |
| 3 | close() kills WAD fd | No-op close for all fds | 61648393 |
| 4 | Soft-float infinite recursion (25 funcs) | IEEE 754 bit manipulation | b29a14b2 |
| 5 | gamemode=commercial (wrong WAD) | access() matches doom.wad only | e3730102 |
| 6 | STCFN033 -> STCFN33 (missing font) | sprintf %.3d precision fix | 641901aa |
| 7 | HELP2 not found | gamemode=retail via doomu.wad | 641901aa |
| 8 | SCREEN_BASE mismatch (0x170000 vs 0x70000) | Fixed in memory.rs + C code | 7c09a490 |
| 9 | Debug regions corrupted by Doom data | Moved to 0x3200000 (above stack) | 4bed0732 |
| 10 | I_FinishUpdate 16K word stores per frame | Back buffer (screens[0]=malloc, word-copy to SCREEN_BASE) | d5510aec |
| 11 | Keyboard JS keyCode vs PC scancode | JS keyCode mapping in I_StartTic | 32089f37 |

## BUGS FIXED -- Post-Baseline Polish (3 total)

| # | Bug | Fix | Commit |
|---|-----|-----|--------|
| 12 | Palette 203/256 entries wrong | Regenerated from actual WAD PLAYPAL | 823dde86 |
| 13 | Single-slot KBD overwrite on rapid input | 8-slot circular queue in bridge + poll loop in I_StartTic | 42bbc34d |
| 14 | Browser auto-repeat floods key events | e.repeat check in JavaScript | 46f36f77 |

## PREVIOUS SESSION BUGS (9 total -- doomgeneric era)

| # | Bug | Root Cause | Fix | Commit |
|---|-----|-----------|-----|--------|
| 1 | Map alignment | doom-ring.sh pins vs XDP-created maps | doom-runner Aya pipeline | 208c2944 |
| 2 | .sdata missing | objcopy omitted .sdata section | Added -j .sdata | - |
| 3 | Stack/WAD overlap | _stack_top inside WAD region | Moved stack to 0x2100000 | eef8bfc5 |
| 4 | sscanf returns 0 | No-op stub | Real sscanf implementation | eef8bfc5 |
| 5 | mmap path corruption | wad_file pointer off by 8 | Disabled mmap path | 15dbfb96 |
| 6 | Z_Malloc corrupts WAD handle | Handle in zone memory | Static storage in w_file_stdc.c | 2ae0498a |
| 7 | heap_ptr corruption | Wild write to .data | Isolated at 0x1BF0000 | 83fc1f56 |
| 8 | fclose no-op | Slots never freed | Proper slot freeing | 2ae0498a |
| 9 | Heap/WAD overlap | 6 MiB heap into WAD | Heap 26 MiB before WAD | 62734de1 |

## DO NOT REGRESS

These invariants MUST hold after any change:

1. `make` in demos/doom/ produces doom.elf and doom.mbc without errors
2. doom-runner loads DOOM.WAD with all verifications PASS
3. STAT_FRAME_READY increments (game loop is running)
4. Screen pixels are non-zero in RAM_MAP at 0x70000
5. Keyboard events reach I_StartTic (game responds to input)
6. No PC corruption (PC stays in valid ROM range 0-85454)
7. No infinite loops in soft-float functions
8. Game runs stable for 1000+ frames without crash
9. Back buffer word-copy in I_FinishUpdate (not direct render)
10. 8-slot KBD_MAP circular queue (not single-slot)

## NEXT PRIORITIES

1. **Improve browser frame rate** -- batch map reads, reduce poll overhead
2. **Record demo video** -- document the milestone
3. **Investigate weapon sprite flashing** -- rendering timing or column draw
4. **NixOS service module** -- doom-runner.nix with systemd service
