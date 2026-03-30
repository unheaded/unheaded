# Doom-over-IPv6 — ACCEPTED BASELINE

**Date:** 2026-03-30 02:00 UTC
**Commit:** a253f660
**Status:** PLAYABLE — accepted by Stevie as minimum baseline

## What Works at Baseline

- Player can run around levels with WASD + arrow keys
- L key fires weapon
- Space opens doors
- Enter selects menu items, Escape opens menu
- 1-7 switches weapons
- Game logic runs at 35 fps internally
- Title screen, menus, demo playback all work
- Levels load and are navigable
- Performance is acceptable for gameplay

## Known Issues at Baseline (accepted, not blocking)

- Hand/weapon sprite flashes occasionally (tearing from direct render)
- Wall/floor textures show horizontal colored bands (palette was 203/256 entries wrong — FIXED)
- Browser frame rate lower than internal game rate
- No sound

## DO NOT REGRESS BELOW THIS BASELINE

Any change must maintain:
1. Game runs and is navigable (WASD movement works)
2. Keyboard input responsive (L fires, Space uses, menus navigate)
3. Game logic at 35 fps (STAT_FRAME_READY delta)
4. No crashes during normal gameplay
5. Performance at least as fast as this commit

If a change breaks any of these, REVERT immediately.

## Architecture at Baseline

- screens[0] = SCREEN_BASE (0x70000) — direct render, zero copy
- screens[1-4] = malloc'd — wipe effects, status bar, temp buffers
- I_FinishUpdate = SYS_DRAW_FRAME signal only (no memcpy)
- Bridge polls RAM_MAP at SCREEN_BASE every 16ms
- Single-slot KBD_MAP write from bridge
- Dynamic PLAYPAL palette from RAM at 0x60000
- Tail call chain: 16 rounds × 16 insns = 256 insns/tick

## Next Steps (in priority order)

1. ~~Fix texture banding~~ FIXED: palette had 203/256 wrong entries; remaining banding is normal Doom magnification
2. Investigate tearing fix that doesn't sacrifice speed
3. Improve browser frame rate
4. Document for video recording
