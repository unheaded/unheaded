# Doom-over-IPv6 — Technical Documentation

**Status:** Init executing at 23.7M MBC insns/sec. No crashes. Awaiting first frame.
**Last updated:** 2026-03-29

## Architecture

Doom runs as RV32I→MBC bytecode inside eBPF XDP programs. Packets carry Monad
headers through a ring of network namespaces. Each hop executes MBC instructions.
The computation IS the network traversal.

```
 doom.c (C) → doom.elf (RV32I) → doom.mbc (MBC bytecode)
                                       ↓
              doom-runner (Aya) loads into BPF maps
                                       ↓
     ┌──────────┐  IPv6/HbH  ┌──────────┐
     │  monad0  │────────────→│  monad1  │
     │ XDP: 256 │             │ XDP: 256 │
     │ insns/hop│←────────────│ insns/hop│
     └──────────┘  IPv6/HbH  └──────────┘
                                       ↓
              doom-runner bridge → WebSocket → browser canvas
```

## Key Files

| File | Purpose |
|------|---------|
| `crates/doom-runner/` | Aya-based unified runtime (program + maps + bridge) |
| `crates/doom-runner/src/memory.rs` | **THE** memory layout source of truth |
| `crates/doom-runner/src/bridge.rs` | Integrated WebSocket screen bridge |
| `crates/doom-runner/src/tail_calls.rs` | Tail call chain configuration |
| `demos/doom/libc_stubs.c` | C runtime stubs (malloc, fopen, fread, etc.) |
| `demos/doom/w_file_stdc.c` | Static WAD file handles (avoids Z_Malloc corruption) |
| `demos/doom/linker.ld` | RV32I linker memory layout |
| `demos/doom/doomgeneric_monad.c` | Platform layer (screen, input) |
| `demos/doom/Makefile` | Build pipeline: C→RV32I→MBC |
| `ebpf/monad-cpu-ebpf/src/main.rs` | XDP MBC executor + tail calls |
| `ebpf/monad-common/src/lib.rs` | Shared types (MbcCpuState, Monad, mbc_mmap) |
| `ebpf/.cargo/config.toml` | BPF build config (BTF enabled for tail calls) |

## Memory Layout (doom-runner/src/memory.rs)

```
0x000000-0x056430  ROM (.text + .rodata)     → ROM_MAP (262K entries)
0x070000-0x07FA00  SCREEN (320×200)          → SCREEN_MAP (64K entries)
0x100000-0x10E978  RAM (.data + .sdata)      → RAM_MAP
0x10E978-0x197BB0  BSS                       → RAM_MAP (zeroed by crt0)
0x1C0000-0x1BC0000 HEAP (26MB)              → RAM_MAP (bump allocator)
0x1BF0000          heap_ptr (isolated addr)  → RAM_MAP
0x1C00000-0x2001000 WAD (4MB)               → RAM_MAP
0x2100000          STACK_TOP (grows down)    → RAM_MAP
RAM_MAP: 16M entries × 4 bytes = 64MB addressable
```

## Bugs Fixed (Session 2026-03-29)

| # | Bug | Root Cause | Fix | Commit |
|---|-----|-----------|-----|--------|
| 1 | Map alignment | doom-ring.sh pins ≠ program maps | doom-runner Aya pipeline | 208c2944 |
| 2 | .sdata missing | objcopy omitted .sdata section | Added -j .sdata | (in doom agent) |
| 3 | Stack/WAD overlap | _stack_top at 0x1000000 inside WAD | Moved to 0x2100000 | eef8bfc5 |
| 4 | sscanf returns 0 | No-op stub | Real implementation | eef8bfc5 |
| 5 | mmap path corruption | wad_file ptr off by 8 → reads Z_Malloc header | Disabled mmap (#if 0) | 15dbfb96 |
| 6 | Z_Malloc corrupts WAD handle | stdc_wad_file_t in zone memory | Static storage (w_file_stdc.c) | 2ae0498a |
| 7 | heap_ptr corruption | Wild pointer writes to .data | Isolated at 0x1BF0000 + reset in main() | 83fc1f56 |
| 8 | fclose no-op | File table slots never freed | Proper slot freeing | 2ae0498a |
| 9 | Heap/WAD overlap | 6MB heap grew into WAD at 0x800000 | Heap before WAD, 26MB | 62734de1 |

## Performance History

| Change | insns/sec | Notes |
|--------|-----------|-------|
| Baseline (16 insns/tick, burst) | ~1M | Doom crashed at 1.37M insns |
| After bug fixes | ~10M | 2 hops × 16 insns × 80K pps |
| Tail calls (256 insns/tick) | ~15M | 2 hops × 256 insns × 44K pps (tail overhead halves pps) |
| sendmmsg injection | ~23.7M | 2 hops × 256 insns × 93K pps |

## What Works

- doom-runner loads program + maps atomically via Aya (no stale pins)
- Integrated WebSocket bridge (no external Go binary)
- BTF-enabled tail calls: 16 rounds × 16 insns = 256 insns/hop
- sendmmsg injection at 93K pps
- Doom executes 18B+ instructions with zero crashes
- Firefox connects to bridge, canvas renders SCREEN_MAP contents
- Bridge auto-reconnects, shows FPS counter

## What Doesn't Work Yet

- **First frame not rendered** — Doom init takes 30-50B+ instructions on this architecture
- **6-hop ring** — caused issues in past sessions, needs careful testing
- **Playable fps** — need ~100K insns/frame × 35 fps = 3.5M insns/sec (achievable, but init must complete first)

## What Was Tried and Rejected

| Approach | Why Rejected |
|----------|-------------|
| bpf_loop(10000, callback) | Computes locally, packets don't traverse network. Defeats the purpose. |
| chain-depth 33 (528 insns/tick) | Same throughput as 16 — pps drops proportionally to tail call overhead |
| Shell script + Go loader | Systemic map alignment bug. Replaced by doom-runner. |
| Pinned maps for bridge | Fragile, requires manual pin management. Integrated bridge reads via Aya. |

## Next Steps (Prioritized)

1. **Profile init hot path** — where are the 30B+ instructions going? R_InitTextures? memcpy? BSS zeroing?
2. **Optimize hot path** — batch reads, word-aligned copies, skip unnecessary init
3. **6-hop ring** — test carefully (was problematic before), would give 3x throughput
4. **Higher pps** — explore AF_XDP, larger batch sizes, multiple injectors
5. **Tail call tuning** — find optimal chain depth (currently 16, more = diminishing returns)

## Running Doom

```bash
cd ~/tmp/unheaded

# Build (if needed)
demos/doom/make clean && demos/doom/make
cp demos/doom/doom.{elf,mbc,rv2mbc} doom/
make ebpf-monad-cpu
cd crates/doom-runner && cargo build --release && cd ../..

# Launch doom-runner (stays alive, holds BPF fds)
sudo ./crates/doom-runner/target/release/doom-runner run \
  --doom-mbc doom/doom.mbc --rv2mbc doom/doom.rv2mbc \
  --doom-elf doom/doom.elf \
  --wad /home/govan/tmp/projects/doom-related/doom1.wad \
  --hops 2 &

# Attach XDP (use latest prog ID)
PROG_ID=$(sudo bpftool prog list | grep monad_cpu | tail -1 | awk '{print $1}' | tr -d ':')
sudo ip netns exec monad1 bpftool net attach xdpgeneric id $PROG_ID dev veth01p
sudo ip netns exec monad0 bpftool net attach xdpgeneric id $PROG_ID dev veth10p

# Inject packets
SRC_MAC=$(sudo ip netns exec monad0 ip link show veth01 | awk '/ether/ {print $2}')
DST_MAC=$(sudo ip netns exec monad1 ip link show veth01p | awk '/ether/ {print $2}')
sudo nsenter --net=/var/run/netns/monad0 ./bin/doom-go-injector \
  --src-mac "$SRC_MAC" --dst-mac "$DST_MAC" --iface veth01 \
  --count 0 --mode sendmmsg --batch 200 &

# Open browser
# http://localhost:16666
```

## Monitoring

```bash
# Instruction count
sudo bpftool map lookup id $(sudo bpftool map list | grep STATS | awk '{print $1}' | tr -d ':') key hex 02 00 00 00

# Screen pixels (non-zero = rendering)
sudo bpftool map dump id $(sudo bpftool map list | grep SCREEN | awk '{print $1}' | tr -d ':') | grep -cv "value: 00$"

# Injector throughput
tail -1 /tmp/doom-*-inject.log
```
