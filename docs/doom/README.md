# Doom-over-IPv6 -- Technical Documentation

**Status:** PC corruption discovered. 24B+ instructions executed, all NOPs after early corruption. First frame never rendered.
**Last updated:** 2026-03-29

## Architecture

Doom runs as RV32I->MBC bytecode inside eBPF XDP programs. Packets carry Monad
headers through a ring of network namespaces. Each hop executes MBC instructions.
The computation IS the network traversal.

```
 doom.c (C) -> doom.elf (RV32I) -> doom.mbc (MBC bytecode)
                                       |
              doom-runner (Aya) loads into BPF maps
                                       |
     +----------+  IPv6/HbH  +----------+
     |  monad0  |----------->|  monad1  |
     | XDP: 256 |             | XDP: 256 |
     | insns/hop|<------------|insns/hop |
     +----------+  IPv6/HbH  +----------+
                                       |
              doom-runner bridge -> WebSocket -> browser canvas
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
| `demos/doom/Makefile` | Build pipeline: C->RV32I->MBC |
| `ebpf/monad-cpu-ebpf/src/main.rs` | XDP MBC executor + tail calls |
| `ebpf/monad-common/src/lib.rs` | Shared types (MbcCpuState, Monad, mbc_mmap) |
| `ebpf/.cargo/config.toml` | BPF build config (BTF enabled for tail calls) |

## Memory Layout (doom-runner/src/memory.rs)

This is the authoritative layout. All other components must agree with `memory.rs`.

```
0x0000_0000 +--- ROM (1 MiB) --- .text + .rodata
            |   MBC instructions (Array<u32>, index = PC)
0x0010_0000 +--- RAM_BASE
            |   .data + .sdata   (from ELF, loaded at link addresses)
            |   .bss  + .sbss   (zeroed by crt0)
0x0016_8000 |   KBD_ADDR        (keyboard I/O word)
0x0017_0000 |   SCREEN_BASE     (320x200, 8-bit palette)
0x0017_FA00 |   (screen end)
0x001C_0000 +--- HEAP_START     (26 MiB bump allocator)
0x01BC_0000 +--- HEAP_END
0x01BF_0000 |   heap_ptr        (isolated from .data, immune to wild writes)
0x01C0_0000 +--- WAD_BASE       (doom1.wad, up to 4 MiB + 4K)
0x0200_1000 |   (WAD end)
0x0210_0000 +--- STACK_TOP      (grows down)
```

**BPF Map Sizing:**
- ROM_MAP: 262,144 entries (1 MiB instruction space)
- RAM_MAP: 16,777,216 entries x 4 bytes = 64 MiB kernel memory
- SCREEN_MAP: 64,000 entries (320x200 pixels)
- CPU_MAP: 256 instances (HashMap)
- KBD_MAP: 8 entries
- RV2MBC_MAP: 65,536 entries (RV32I-to-MBC address translation)

## Bugs Fixed (Session 2026-03-29)

Nine bugs discovered and fixed, plus the critical PC corruption finding:

| # | Bug | Root Cause | Fix | Commit |
|---|-----|-----------|-----|--------|
| 1 | Map alignment | doom-ring.sh pins maps, XDP program creates separate maps. Data in pinned maps invisible to program. | doom-runner Aya pipeline owns both program and maps atomically | `208c2944` |
| 2 | .sdata missing | objcopy omitted .sdata section containing heap_ptr, stdout, and critical globals | Added `-j .sdata` to objcopy | (in doom agent) |
| 3 | Stack/WAD overlap | `_stack_top` at 0x1000000 inside WAD region | Moved stack to 0x2100000 | `eef8bfc5` |
| 4 | sscanf returns 0 | No-op stub returned 0, callers assumed success | Real sscanf implementation | `eef8bfc5` |
| 5 | mmap path corruption | `wad_file` pointer off by 8 bytes, reads Z_Malloc header instead of file data | Disabled mmap path entirely (`#if 0`) | `15dbfb96` |
| 6 | Z_Malloc corrupts WAD handle | `stdc_wad_file_t` allocated in zone memory, freed by Z_Malloc recycling | Static storage in `w_file_stdc.c` | `2ae0498a` |
| 7 | heap_ptr corruption | Wild pointer writes to .data region where heap_ptr lived (~0x10E714) | Isolated heap_ptr at 0x1BF0000, far from .data; bounds check on every malloc | `83fc1f56` |
| 8 | fclose no-op | File table slots never freed, eventually exhausted | Proper slot freeing in fclose | `2ae0498a` |
| 9 | Heap/WAD overlap | 6 MiB heap grew into WAD at 0x800000 | Heap before WAD with 26 MiB, WAD at 0x1C00000 | `62734de1` |
| **10** | **PC corruption (CURRENT BLOCKER)** | Function pointer call with corrupted register jumps PC to garbage. ROM_MAP returns 0 (NOP) for out-of-bounds, so Doom executes infinite NOPs. | **NOT YET FIXED** -- need PC bounds check in eBPF executor | -- |

## Performance History

| Change | insns/sec | Notes |
|--------|-----------|-------|
| Baseline (16 insns/tick, burst) | ~1M | Doom crashed at 1.37M insns |
| After bug fixes | ~10M | 2 hops x 16 insns x 80K pps |
| Tail calls (256 insns/tick) | ~15M | 2 hops x 256 insns x 44K pps (tail overhead halves pps) |
| sendmmsg injection | ~23.7M | 2 hops x 256 insns x 93K pps |
| **Reality check** | **0 useful** | All instructions after PC corruption are wasted NOPs |

**Key insight:** "More instructions" does not equal "more progress". The 24B+ instruction
count was misleading -- nearly all of it was NOP execution after PC corruption.

## What Works

- doom-runner loads program + maps atomically via Aya (no stale pins)
- Integrated WebSocket bridge (no external Go binary)
- BTF-enabled tail calls: 16 rounds x 16 insns = 256 insns/hop
- sendmmsg injection at 93K pps
- Doom executes 18B+ instructions with zero crashes (but not usefully -- see PC corruption)
- Firefox connects to bridge, canvas renders SCREEN_MAP contents
- Bridge auto-reconnects, shows FPS counter
- Memory layout validated at startup (overlap detection)

## What Doesn't Work Yet

- **First frame not rendered** -- PC corruption means Doom never reaches rendering code
- **PC bounds checking** -- eBPF executor does not halt on out-of-range PC
- **6-hop ring** -- caused issues in past sessions, needs careful testing after PC fix
- **Playable fps** -- blocked on getting past init (first frame)

## What Was Tried and Rejected

| Approach | Why Rejected |
|----------|-------------|
| `bpf_loop(10000, callback)` | Computes locally, packets don't traverse network. Defeats the entire purpose of Doom-over-IPv6. |
| chain-depth 33 (528 insns/tick) | Same throughput as 16 -- pps drops proportionally to tail call overhead. Kernel limit for diminishing returns. |
| Shell script + Go loader | Systemic map alignment bug (#1). Replaced by doom-runner which owns the whole pipeline. |
| Pinned maps for bridge | Fragile, requires manual pin management. Integrated bridge reads maps directly via Aya Ebpf object. |

## Running Doom

See `docs/doom/RUNBOOK.md` for the complete operational runbook.

Quick version:

```bash
cd ~/tmp/unheaded

# Build MBC binary
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
  --wad /path/to/doom1.wad \
  --hops 2

# Attach XDP to namespaces (doom-runner loads the program but
# namespace attachment requires nsenter -- see RUNBOOK.md)
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
```

## Monitoring

```bash
# Instruction count
sudo bpftool map lookup id $(sudo bpftool map list | grep STATS | awk '{print $1}' | tr -d ':') key hex 02 00 00 00

# Screen pixels (non-zero count means rendering has started)
sudo bpftool map dump id $(sudo bpftool map list | grep SCREEN | awk '{print $1}' | tr -d ':') | grep -cv "value: 00$"

# Current PC (check for corruption -- valid range is 0x0 to ~0x19700)
sudo bpftool map lookup id $(sudo bpftool map list | grep CPU_MAP | head -1 | awk '{print $1}' | tr -d ':') key hex de 00 00 00

# Injector throughput
tail -1 /tmp/doom-*-inject.log
```

## Related Documentation

- `docs/doom/ARCHITECTURE.md` -- Deep technical architecture
- `docs/doom/FINDINGS.md` -- Debugging narrative and PC corruption discovery
- `docs/doom/RUNBOOK.md` -- Operational runbook
- `docs/doom/NEXT-STEPS.md` -- Prioritized roadmap
