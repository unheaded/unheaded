# Pivot: doomgeneric → id Software linuxdoom-1.10

**Date:** 2026-03-29
**Decision:** Drop doomgeneric fork. Use id Software's GPL-2.0 source directly.
**WAD:** Retail DOOM.WAD (12.4MB, Steam licensed) replaces shareware doom1.wad.

## Why

1. **Trust:** doomgeneric is a third-party fork. id's source is the canonical release.
2. **PC corruption bug:** The doomgeneric abstraction layer adds indirection (DG_* callbacks)
   that complicates debugging. Direct i_* interface files are simpler.
3. **Endgame alignment:** The UPC goal is a 486-class computer. Real DOOM ran on a 386.
   Using the real source aligns with the vision.

## Source Locations

```
~/tmp/projects/DOOM/linuxdoom-1.10/   # 62 .c, 62 .h, ~45K lines, GPL-2.0
~/tmp/projects/doom-related/DOOM.WAD  # 12,408,292 bytes, retail (Steam)
~/tmp/unheaded/demos/doom/            # MBC platform stubs + build system
~/tmp/unheaded/crates/doom-runner/    # Aya runtime (UNCHANGED)
```

## What Maps from Current Port

| Component | Reusable? | Notes |
|-----------|-----------|-------|
| crt0.S | 100% | Standard RV32I startup |
| libc_stubs.c | 95% | Needs POSIX fd table (open/read/lseek) |
| gcc_runtime.c | 100% | Software div/mul |
| include/ | 95% | Needs alloca.h, malloc.h |
| linker.ld | 90% | WAD region 4M→16M, stack moves up |
| doom-runner | 100% | Just update memory.rs constants |
| eBPF executor | 100% | Unchanged |
| tail calls | 100% | Unchanged |
| doomgeneric_monad.c | 0% | DELETED — replaced by i_*_mbc.c files |
| w_file_stdc.c | 0% | DELETED — id DOOM uses POSIX I/O directly |

## Key Architecture Difference: File I/O

- **doomgeneric:** stdio (fopen/fread/fseek) — our stubs handle this
- **id DOOM:** POSIX (open/read/lseek/close) — must add fd table to libc_stubs.c

## Memory Layout Changes

```
WAD_MAX_SIZE:  4MB → 16MB  (retail DOOM.WAD is 12.4MB)
STACK_TOP:     0x2100000 → 0x3100000  (above WAD end)
RAM_MAP:       16M entries (64MB) — unchanged, covers everything
```

## New Platform Files (to create)

| File | Replaces | Purpose |
|------|----------|---------|
| i_video_mbc.c | X11 i_video.c | Copy screens[0] → SCREEN_BASE |
| i_sound_mbc.c | OSS i_sound.c | No-op stubs (no audio hardware) |
| i_net_mbc.c | UDP i_net.c | Single-player stubs |
| i_system_mbc.c | UNIX i_system.c | MBC syscalls, main(), I_Error |

## Build Command Comparison

```bash
# OLD (doomgeneric)
make  # compiles doomgeneric sources + stubs

# NEW (id DOOM)
make  # compiles linuxdoom-1.10 sources + MBC platform files
```

## Phase Plan

See BATTLE-PLAN.md for the choose-your-own-adventure execution plan.
The recommended path for the id DOOM pivot is straight through:
Phase 1 (build) → Phase 2 (compile) → Phase 3 (link) → Phase 4 (run) → Phase 5 (fix) → Phase 6 (frame)
