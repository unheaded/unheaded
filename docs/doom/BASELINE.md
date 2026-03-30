# Doom-over-IPv6 -- ACCEPTED BASELINE

**Date:** 2026-03-30
**Commits:** 42bbc34d (8-slot KBD queue) + 46f36f77 (browser auto-repeat suppression)
**Status:** PLAYABLE -- accepted as minimum baseline
**Source:** id Software linuxdoom-1.10 (GPL-2.0)
**WAD:** ~/tmp/projects/doom-related/DOOM.WAD (12,408,292 bytes, retail Steam)

## What Works at Baseline

- Player can run around levels with arrow keys + WASD
- Ctrl / L fires weapon
- Space opens doors
- Enter selects menu items, Escape opens menu
- 1-7 switches weapons
- Shift = run, Alt = strafe modifier, < > = strafe left/right
- Game logic runs at 35 fps internally
- Title screen, menus, demo playback all work
- Levels load and are navigable
- HUD renders correctly (ammo, health, armor, weapons)
- Sprites solid, movement responsive
- 5.9B+ instructions executed stably

## Known Issues at Baseline (accepted, not blocking)

- Some texture banding close to walls -- confirmed as authentic Doom 320x200 nearest-neighbor magnification, NOT a rendering bug
- Browser frame rate lower than internal game rate (~2-3 visible fps)
- No sound (by design, no audio hardware)

## Architecture at Baseline

- **Back buffer rendering**: screens[0] = malloc'd (private), word-copy to SCREEN_BASE in I_FinishUpdate
- screens[1-4] = malloc'd (wipe effects, status bar, temp buffers)
- I_FinishUpdate: 16K word stores (0.7ms copy window vs 28ms render = 40x less tearing)
- **8-slot KBD_MAP circular queue**: bridge scans for empty slot, writes scancode<<1|pressed
  - I_StartTic polls up to 8 times per tic to drain queue
  - Prevents rapid events from overwriting each other
- **Browser auto-repeat suppressed**: `e.repeat` check prevents held keys from flooding queue
- **Bilinear CSS upscale**: canvas scales 320x200 -> 960x600 with browser default (smooth)
- **Dynamic PLAYPAL palette**: bridge reads 768 bytes from RAM_MAP at 0x60000
  - Falls back to hardcoded palette if dynamic read is unavailable
- **Word-aligned memcpy**: fast path (4x fewer MBC stores), critical for I_FinishUpdate
- Tail call chain: 16 rounds x 16 insns = 256 insns/tick
- doom-runner Aya pipeline: atomic program+map ownership, no pins

## Key Addresses (from memory.rs)

| Address | Name | Notes |
|---------|------|-------|
| 0x0006_0000 | PALETTE_ADDR | 768 bytes, dynamic PLAYPAL |
| 0x0007_0000 | SCREEN_BASE | 64,000 bytes, front buffer (bridge reads) |
| 0x0010_0000 | RAM_BASE | .data + .bss |
| 0x0016_8000 | KBD_ADDR | Keyboard I/O word |
| 0x001C_0000 | HEAP_START | 26 MiB JVM-style sbrk heap |
| 0x01BC_0000 | HEAP_END | |
| 0x01C0_0000 | WAD_BASE | Retail DOOM.WAD (up to 16 MiB) |
| 0x0310_0000 | STACK_TOP | Grows down |

## DO NOT REGRESS BELOW THIS BASELINE

Any change must maintain:
1. Game runs and is navigable (movement works with arrows/WASD)
2. Keyboard input responsive (Ctrl/L fires, Space uses, menus navigate)
3. 8-slot KBD_MAP circular queue (not single-slot)
4. Back buffer rendering with word-copy (not direct render)
5. Game logic at 35 fps (STAT_FRAME_READY delta)
6. No crashes during normal gameplay
7. No PC corruption (PC in range 0 to ~85454)
8. Performance at least as fast as this commit

If a change breaks any of these, REVERT immediately.
