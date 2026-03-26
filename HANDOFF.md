# Agent Handoff — DOOM Performance + Boot Fix
**Date:** 2026-03-26
**HEAD:** 665bbbd8 (feat(lich): LICH-011/012/013/014)
**System state:** Doom ring is DOWN. No injector/bridge running. Zhen killed (was eating 1.1GB).

---

## Current Problem: DOOM Won't Boot After Teardown/Reload

### What Happened
User asked to make DOOM run faster (was 15K IPS, barely playable). We switched the injector from `burst` mode (60 pps) to `sendmmsg` mode (batch=64) and got **29M IPS** — a 1,824x speedup. But DOOM showed a blank screen.

### Root Cause Discovery
The blank screen is **NOT speed-related**. After full teardown + fresh setup + data reload, DOOM fails to boot even at the original 15K IPS:

- `doomgeneric_Create()` crashes via `longjmp` on every attempt
- `SENTINEL_CREATE_TRIES` reaches 5 (= `CREATE_MAX_RETRIES`) after ~3.5 min at 15K IPS
- After 5 failures, the code gives up and enters the game loop without proper init
- Game loop runs (GTIC increments, sentinels advance) but produces **zero screen output**
- SCREEN_MAP: 0% non-zero. Video buffer at 0x80000: all zeros. RAM screen at 0x70000: all zeros.

### Key Evidence
- `CREATE_MAX_RETRIES = 5` in `/tmp/doom-build/doomgeneric_monad_patched.c:51`
- After teardown/setup/reload, PC gets stuck cycling 0x0A-0x10 (BSS zeroing loop in `_start`)
- The game IS executing tics (GTIC increments ~67/sec at 29M IPS) but renders nothing
- BSS variables (DG_ScreenBuffer, I_VideoBuffer, viewwidth, viewheight, ylookup) are correctly repaired by `repair_bss()`
- `DG_ScreenBuffer = 0x70000` (SCREEN_BASE) — intentional per platform code
- `I_VideoBuffer = 0x80000` (FALLBACK_VIDEOBUFFER) — correct
- WAD loaded correctly (0x110000 shows "IWAD" magic)
- Heap is being used (bump allocator at 0x520000 advanced to 0x3E808)

### What Previously Worked
The session BEFORE this one had DOOM rendering at 94.7% non-zero pixels, 15K IPS. That session's doom-ring was set up ~Mar 2 and ran continuously until we tore it down today. The difference: that ring was never torn down and reloaded — it had been running continuously since initial setup.

### Hypothesis
`doomgeneric_Create()` → `D_DoomMain()` crashes during WAD/texture init via `I_Error()` → `longjmp(error_recovery, 1)`. This happens on EVERY fresh boot. The old working session likely had a warm ring that was never torn down.

Possible causes of Create crash:
1. **Stack corruption** during D_DoomMain's heavyweight init (WAD parsing, texture compositing)
2. **BSS variable overlap** — some init variable at a bad address gets clobbered
3. **Memory map issue** — heap bump allocator conflicts with BSS or .data regions
4. **RV32I→MBC translation bug** triggered by specific init code paths

### What Needs Investigation
1. **WHY does Create crash?** Need to catch the exact crash point. Ideas:
   - Add sentinel writes at key points in D_DoomMain init sequence
   - Check if `error_recovery` jmp_buf (BSS) is being corrupted
   - Check stack depth during init — SP might underflow/overflow
   - Single-step through init by running at very low IPS and sampling PC

2. **Is the old doom.elf correct?** Verify doom.elf → doom.mbc → doom_data.bin are all from the same build:
   - `doom/doom.elf` — 441K, RV32IM ELF
   - `doom/doom.mbc` — ~340K, ~85K MBC instructions
   - `doom/doom_data.bin` — ~162K, combined .rodata+.data
   - Source: `/tmp/doom-build/doom.elf` (the build output)
   - **CRITICAL**: doom_data.bin MUST match doom.elf. Stale data causes jump table corruption.

3. **BSS boundaries** — `_start` zeros from 0x3DF80 to 0x63938 (~155KB). Verify no overlap with .data or heap.

---

## Performance Plan (Paused — Blocked on Boot Fix)

Original 3-step plan to make DOOM playable:

| Step | Description | Status |
|------|-------------|--------|
| 1 | Restart injector with sendmmsg mode | DONE — 29M IPS achieved |
| 2 | Fix turbo mode (attach XDP to both veth sides) | Pending — not needed if Step 1 is enough |
| 3 | Increase MAX_INSN_PER_TICK from 256 to 300 | Pending — not needed if Step 1 is enough |

Step 1 alone gave 29M IPS (68 game tics/sec, 2x the 35fps target). Steps 2 and 3 are unnecessary for playability.

**The blocker is the boot crash, not performance.**

---

## DOOM Startup Runbook (Once Boot Fix Is In)

```bash
# Setup ring
sudo scripts/doom-ring.sh setup

# Load data
MAP=/sys/fs/bpf/unheaded/doom-ring/maps
sudo cmd/doom-loader/doom-loader rom $MAP/ROM_MAP doom/doom.mbc
DATA_VMA=$(riscv64-unknown-elf-objdump -h doom/doom.elf | awk '/\.rodata[[:space:]]/{print "0x"$4}')
sudo cmd/doom-loader/doom-loader ram $MAP/RAM_MAP doom/doom_data.bin $DATA_VMA
sudo cmd/doom-loader/doom-loader ram $MAP/RAM_MAP ~/tmp/DOOM_wads/doom1.wad 0x110000
sudo cmd/doom-loader/doom-loader rv2mbc $MAP/RV2MBC_MAP doom/doom.rv2mbc
sudo cmd/doom-loader/doom-loader cpu --instance DE --map $MAP/CPU_MAP

# Start injector (sendmmsg for max speed)
SRC=$(sudo ip netns exec monad0 cat /sys/class/net/veth01/address)
DST=$(sudo ip netns exec monad1 cat /sys/class/net/veth01p/address)
sudo ip netns exec monad0 cmd/doom-go-injector/doom-go-injector \
  -iface veth01 -src-mac $SRC -dst-mac $DST -mode sendmmsg -batch 64 -count 0 &

# Start bridge
go build -o /tmp/doom-bridge ./cmd/doom-bridge/
sudo /tmp/doom-bridge -port 16666 -map-path $MAP -static demos/doom &

# Browser: http://localhost:16666/
```

---

## Key Files

| File | Purpose |
|------|---------|
| `/tmp/doom-build/doomgeneric_monad_patched.c` | Platform layer (main, repair_bss, copy_fb_to_screen) |
| `ebpf/monad-cpu-ebpf/src/main.rs` | XDP program (MAX_INSN_PER_TICK=256 at line 145) |
| `scripts/doom-ring.sh` | Ring setup/teardown |
| `cmd/doom-go-injector/main.go` | Packet injector |
| `cmd/doom-bridge/main.go` | WebSocket bridge (screen → browser) |
| `cmd/doom-loader/main.go` | BPF map loader |
| `demos/doom/index.html` | Browser viewer |
| `doom/doom.elf` | DOOM ELF binary (RV32IM) |
| `doom/doom.mbc` | Translated MBC instructions |
| `doom/doom_data.bin` | .rodata + .data binary blob |
| `doom/doom.rv2mbc` | RV32I→MBC PC address mapping |

## Uncommitted Changes
- `references/timeline.*` — modified (json/md/toml/yaml)
- `tomb/lich/` — new fuzz corpus entries + results (staged)
- `raft/ADD_SOURCES.md`, `raft/scripts/add-source.sh` — untracked
