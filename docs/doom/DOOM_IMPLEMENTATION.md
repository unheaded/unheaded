# DOOM Engine Integration in Unheaded (S37)

## Overview

The Unheaded project integrates the DOOM engine as a computational completeness
proof for the Monad spec (Section 12). DOOM runs inside eBPF BPF maps via MBC
bytecode — it does NOT link against or modify the Unheaded main codebase.

## Source Lineage

- **Original:** id Software DOOM engine (1993, GPL 2.0 released 1997)
- **Base:** doomgeneric (portable DOOM library)
- **Current:** S37 fork with SDL2 audio stubs (2026-02)

## GPL 2.0 Boundary

### GPL 2.0 Licensed (doom/ directory only)

- `/doom/doomgeneric/` (base library and source)
- `/doom/*.wad` (WAD game data)
- `/doom/doomgeneric/doomgeneric/audio_init.{c,h}` (audio stubs)
- `/doom/doom.mbc`, `/doom/doom.rv2mbc` (compiled MBC bytecode)

### NOT GPL 2.0 Licensed

- `/cmd/` — Go command-line tools (MIT)
- `/pkg/` — Go packages (MIT)
- `/services/` — Service implementations (MIT)
- `/ebpf/` — eBPF programs (MIT)
- `/crates/` — Rust crates (MIT)
- `/docs/protocol/` — Protocol specs (MIT/Apache 2.0)

### Separation Architecture

1. **Separate Directory:** DOOM code lives only in `/doom/`
2. **IPC Boundary:** Main code communicates with DOOM via sockets/RPC
3. **Build Isolation:** DOOM is built separately from main codebase
4. **No Direct Linking:** Main Go/Rust code never links DOOM C code

## S37 Modifications

### Audio Support (NEW)

- `doom/doomgeneric/doomgeneric/audio_init.c` — SDL2_mixer initialization (stub)
- `doom/doomgeneric/doomgeneric/audio_init.h` — Audio API declarations
- Conditional compilation: `HAVE_SDL_MIXER` flag controls SDL2_mixer usage
- Graceful fallback: no-op stubs when SDL2_mixer is unavailable

### License Compliance

- [x] GPL 2.0 license text referenced in `/doom/LICENSE`
- [x] GPL boundary clearly marked (subdirectory containment)
- [x] No GPL code in main codebase
- [x] Audio subsystem (SDL2_mixer) compatible with GPL 2.0

## References

- DOOM Engine Source: https://github.com/id-Software/DOOM/
- doomgeneric: https://github.com/ozkl/doomgeneric
- GPL 2.0: https://www.gnu.org/licenses/old-licenses/gpl-2.0.txt
- SDL2_mixer: https://www.libsdl.org/projects/SDL_mixer/
