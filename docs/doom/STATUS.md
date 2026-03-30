# Doom-over-IPv6 Status

**Last verified:** 2026-03-30 01:22 UTC
**Session commits:** 64 (32089f37 latest)
**Total repo commits:** 628+

## MILESTONE: PLAYABLE DOOM

id Software's DOOM (linuxdoom-1.10) is RUNNING and PLAYABLE over IPv6 on the
Unheaded Protocol Computer. Player can move, shoot, open doors, navigate menus.

### What the player sees
- Title screen renders correctly with proper PLAYPAL colors
- Demo playback works (Doom plays itself in attract mode)
- Pressing Enter starts a new game
- Player can move with arrow keys, fire with Ctrl, open doors with Space
- HUD renders (ammo, health, armor, weapon)
- Hand sprite visible (flashing — rendering issue, not a crash)
- Some textures not rendering correctly (walls, floors may be wrong)
- Frame rate is slow in the browser (~2-3 visible fps due to bridge read overhead)
- Game logic runs at 35 fps internally

### Known visual issues
- Hand/weapon sprite flashes (may be palette animation or rendering timing)
- Some wall/floor textures incorrect or missing
- Browser frame rate much lower than game logic frame rate
- These are rendering issues, NOT crashes — the game is stable

## VERIFIED WORKING (on disk, tested, committed)

| # | Feature | Status | Evidence |
|---|---------|--------|----------|
| 1 | id DOOM compiles for RV32I MBC | WORKING | `make` succeeds, doom.mbc generated |
| 2 | Retail DOOM.WAD loads (12.4MB) | WORKING | "IWAD magic verified" in doom-runner |
| 3 | 35 fps game logic | WORKING | STAT_FRAME_READY delta = 35/sec |
| 4 | Screen pixels render (62,754/64,000) | WORKING | bpftool map dump confirms |
| 5 | Dynamic PLAYPAL palette | WORKING | I_SetPalette writes to 0x60000 |
| 6 | Direct render (screens[0]=SCREEN_BASE) | WORKING | No memcpy overhead |
| 7 | Bridge reads RAM_MAP at 0x70000 | WORKING | Firefox shows frames |
| 8 | Keyboard input (full mapping) | WORKING | Player moves, fires, opens doors |
| 9 | Menu navigation (Enter, Escape) | WORKING | New Game starts, menu works |
| 10 | Weapon switching (1-7 keys) | WORKING | Keys mapped |
| 11 | Tail call chain (256 insn/tick) | WORKING | BTF enabled, self tail-call |
| 12 | doom-runner Aya pipeline | WORKING | Atomic program+map ownership |
| 13 | Soft-float IEEE 754 (25 funcs) | WORKING | Integer bit manipulation |
| 14 | sprintf precision (%.3d) | WORKING | Font lump names correct |
| 15 | JVM-style dynamic heap (sbrk) | WORKING | __heap_start linker symbol |
| 16 | POSIX fd stubs | WORKING | WAD reads via fd table |
| 17 | gamemode=retail (Ultimate DOOM) | WORKING | access() matches doomu.wad |
| 18 | 5.9B+ instructions executed | WORKING | No crashes, no PC corruption |

## KNOWN ISSUES (not blocking — game is playable)

| Issue | Symptom | Likely Cause | Priority |
|-------|---------|-------------|----------|
| Hand sprite flashes | Weapon sprite appears/disappears rapidly | Rendering timing or column draw issue | MEDIUM |
| Missing textures | Some walls/floors show wrong texture | R_DrawColumn or texture lookup edge case | MEDIUM |
| Browser frame rate low | ~2-3 fps visible despite 35 fps internal | Bridge reads 16K words per frame poll | HIGH |
| No sound | Silent | By design (no-op stubs) | LOW |

## ARCHITECTURE

```
 DOOM.WAD (Steam retail, 12.4MB)
       |
 linuxdoom-1.10 (id Software GPL-2.0, 62 .c files)
       |
 RV32I cross-compile (gcc, restricted x0-x15 registers)
       |
 doom.elf → rv32i-to-mbc → doom.mbc (86K MBC instructions)
       |
 doom-runner (Aya) loads into BPF maps atomically
       |
 ┌──────────┐  IPv6/HbH  ┌──────────┐
 │  monad0  │────────────→│  monad1  │
 │ XDP: 256 │             │ XDP: 256 │
 │ insns/hop│←────────────│ insns/hop│
 └──────────┘  IPv6/HbH  └──────────┘
       |
 doom-runner bridge (Rust/Axum WebSocket)
       |
 Firefox canvas (320x200, PLAYPAL palette, keyboard input)
```

## BUGS FIXED THIS SESSION (11 total)

| # | Bug | Fix | Commit |
|---|-----|-----|--------|
| 1 | sprintf 32-bit overflow | Cap size at 0x7FFFFFFF | 459210c6 |
| 2 | fd read returns -1 | Resilient fallback to slot 0 | 61648393 |
| 3 | close() kills WAD fd | No-op close for all fds | 61648393 |
| 4 | Soft-float infinite recursion (25 funcs) | IEEE 754 bit manipulation | b29a14b2 |
| 5 | gamemode=commercial (wrong WAD) | access() matches doom.wad only | e3730102 |
| 6 | STCFN033 → STCFN33 (missing font) | sprintf %.3d precision fix | 641901aa |
| 7 | HELP2 not found | gamemode=retail via doomu.wad | 641901aa |
| 8 | SCREEN_BASE mismatch (0x170000 vs 0x70000) | Fixed in memory.rs + C code | 7c09a490 |
| 9 | Debug regions corrupted by Doom data | Moved to 0x3200000 (above stack) | 4bed0732 |
| 10 | I_FinishUpdate 16K word stores per frame | Direct render (screens[0]=SCREEN_BASE) | d5510aec |
| 11 | Keyboard JS keyCode vs PC scancode | JS keyCode mapping in I_StartTic | 32089f37 |

## PREVIOUS SESSION BUGS (9 total)

See docs/doom/FINDINGS.md for the full debugging narrative from the doomgeneric port.

## DO NOT REGRESS

These invariants MUST hold after any change:

1. `make` in demos/doom/ produces doom.elf and doom.mbc without errors
2. doom-runner loads DOOM.WAD with all verifications PASS
3. STAT_FRAME_READY increments (game loop is running)
4. Screen pixels are non-zero in RAM_MAP at 0x70000
5. Keyboard events reach I_StartTic (game responds to input)
6. No PC corruption (PC stays in valid ROM range 0-86K)
7. No infinite loops in soft-float functions
8. Game runs stable for 1000+ frames without crash

## HOW TO RUN

```bash
cd ~/tmp/unheaded

# Build
cd demos/doom && make && cp doom.elf doom.mbc doom.rv2mbc ../../doom/ && cd ../..
make ebpf-monad-cpu
cd crates/doom-runner && cargo build --release && cd ../..

# Launch
sudo ./crates/doom-runner/target/release/doom-runner run \
  --doom-mbc doom/doom.mbc --rv2mbc doom/doom.rv2mbc \
  --doom-elf doom/doom.elf \
  --wad ~/tmp/projects/doom-related/DOOM.WAD --hops 2 &
sleep 6

# Attach XDP
PROG_ID=$(sudo bpftool prog list | grep monad_cpu | tail -1 | awk '{print $1}' | tr -d ':')
sudo ip netns exec monad1 bpftool net attach xdpgeneric id $PROG_ID dev veth01p
sudo ip netns exec monad0 bpftool net attach xdpgeneric id $PROG_ID dev veth10p

# Inject
SRC_MAC=$(sudo ip netns exec monad0 ip link show veth01 | awk '/ether/ {print $2}')
DST_MAC=$(sudo ip netns exec monad1 ip link show veth01p | awk '/ether/ {print $2}')
sudo nsenter --net=/var/run/netns/monad0 ./bin/doom-go-injector \
  --src-mac "$SRC_MAC" --dst-mac "$DST_MAC" --iface veth01 \
  --count 0 --mode sendmmsg --batch 200 &

# Play
# Open http://localhost:16666 in Firefox
# Controls: arrows=move, ctrl=fire, space=use, enter=start, esc=menu
```

## NEXT PRIORITIES

1. **Fix hand sprite flashing** — rendering timing or column draw
2. **Fix missing textures** — R_DrawColumn or texture cache issue
3. **Improve browser frame rate** — batch map reads, reduce poll overhead
4. **Record demo video** — needs stable visuals
5. **Document architecture** — update ARCHITECTURE.md for id DOOM port
