# Doom over IPv6: Running a Game Engine Inside eBPF

## Summary

Doom (1993, id Software) runs inside Linux eBPF as a proof of computational completeness for the Unheaded Monad protocol. The game executes as MBC (Monad Bytecode) instructions inside an XDP program, with BPF maps serving as RAM, ROM, framebuffer, and keyboard input. Packets circulate through 6 Linux network namespaces in a ring topology, executing 128 instructions per hop, 255 hops per packet -- delivering approximately 32,640 instructions per injected packet.

**Key results:**
- 559+ frames rendered (title screen, credits, demo cycle)
- 819,000,000+ instructions executed
- Zero halts, zero ROM faults
- ~1.47 million instructions per frame
- ~6 fps at baseline (333 packets/second injection rate)

---

## Architecture

### The Packet Circulation Ring

Six Linux network namespaces (`monad0` through `monad5`) are connected by veth pairs in a ring topology. Each namespace has the same XDP program (`monad_cpu`) attached to its ingress interface. When a packet enters a namespace, the XDP program executes 128 MBC instructions, then returns `XDP_TX` to bounce the packet back out to the next namespace.

```
   monad0 ──→ monad1 ──→ monad2
     ^                       |
     |                       v
   monad5 ←── monad4 ←── monad3

Each hop: 128 MBC instructions (MAX_INSN_PER_TICK)
Full ring (6 hops): 768 instructions
Full packet (255 bounces): 128 x 255 = 32,640 instructions
```

A critical networking requirement: each veth pair must use a different /64 IPv6 prefix (`fd00:3f:75:${i}::1/64`). Using the same /64 across links causes kernel routing ambiguity, NDP failure, and packet drops.

### Monad Wire Format

Each packet carries a 78-byte header:

| Component | Size | Purpose |
|-----------|------|---------|
| Ethernet header | 14 bytes | L2 framing |
| IPv6 header | 40 bytes | L3 addressing, flow label identifies Doom instance |
| Hop-by-Hop extension | 8 bytes | Option type 0x1E, length, padding |
| Monad register file | 16 bytes | CPU instance ID, bounce counter, metadata |

The bounce counter in the Hop-by-Hop header tracks how many times the packet has circulated. When it reaches 255, the XDP program returns `XDP_PASS` instead of `XDP_TX`, allowing the packet to exit the ring.

### BPF Maps as Computer Memory

All state lives in pinned BPF maps under `/sys/fs/bpf/unheaded/doom-ring/maps/`:

| Map | Type | Size | Purpose |
|-----|------|------|---------|
| `ROM_MAP` | Array | ~360,000 entries | MBC bytecode (translated from RISC-V ELF) |
| `RAM_MAP` | Array | 16,000,000 entries | Heap, stack, BSS, globals (128 MB) |
| `CPU_MAP` | Array | 1 entry (104 bytes) | Registers (r0-r15), PC, flags, counters |
| `SCREEN_MAP` | Array | 64,000 entries | Framebuffer (320x200, palette indices) |
| `KBD_MAP` | Array | 256 entries | Keyboard state (scancode -> pressed) |
| `RV2MBC_MAP` | Array | variable | RISC-V address -> MBC PC translation table |
| `STATS` | Array | 16 entries | Packet count, instruction count, fault counters |

The CPU state struct (`MbcCpuState`) is 104 bytes:
- 16 general-purpose 32-bit registers (r0-r15, where r14=return address, r15=stack pointer)
- Program counter (u32)
- Flags (u8: zero, negative, carry)
- Halted flag (u8)
- Stalled flag (u8)
- Padding (u8)
- Sleep counter (u64)
- Instruction count (u64)
- Cache hits (u64)
- Cache misses (u64)

### The MBC Instruction Set

MBC (Monad Bytecode) is a 32-bit fixed-width ISA designed for BPF verifier compatibility:

```
[31..24]  opcode   (8 bits)
[23..20]  dst      (4 bits)
[19..16]  src      (4 bits)
[15..0]   imm16    (16 bits)
```

Key opcodes: ADD, SUB, MUL, DIV, MOD, NEG, AND, OR, XOR, NOT, SHL, SHR, SAR, MOV, MOVI, CMP, JMP, JZ, JNZ, CALL, RET, JMPR, CALLR, LD, ST, LDB, STB, LDH, STH, PUSH, POP, LOAD_IMM32, ADDI, SYSCALL, HALT, NOP.

LOAD_IMM32 (0x1C) sets the upper 16 bits of a register: `r[d] = (imm16 << 16) | (r[d] & 0xFFFF)`. Combined with MOVI (which sets the lower 16 bits), this allows loading arbitrary 32-bit constants in two instructions.

### Compilation Pipeline

```
Doom C source (doomgeneric)
    |
    v  riscv32-unknown-elf-gcc (cross-compile)
RISC-V 32-bit ELF
    |
    v  rv32i_to_mbc translator (Rust)
MBC bytecode (ROM_MAP entries)
    |
    v  doom-loader.sh + load_rom_fast.py
BPF map entries (pinned in /sys/fs/bpf/)
    |
    v  aya-ebpf loader (Rust)
XDP program attached to veth interfaces
    |
    v  bulk_inject.py (packet injection)
Doom executes instruction-by-instruction
```

The Doom binary includes a custom platform layer (`doomgeneric_monad.c`), libc stubs (`libc_monad.c`), a WAD file reader (`w_file_monad.c`), and a CRT0 startup routine (`crt0_monad.S`). The WAD file (doom1.wad, 4,196,020 bytes) is loaded into RAM at address 0x110000.

---

## The Journey

### Phase 1: BSS Clearing (Instructions 0 - 60,000,000)

The CRT0 startup routine clears Doom's ~6 MB BSS section byte-by-byte using a store-byte loop (~6 instructions per byte). At 4,080 instructions per packet (pre-turbo), this required approximately 14,700 packets just for BSS initialization -- roughly 60 million instructions total.

This was the first sign that the system worked: the instruction counter climbed steadily, the CPU never halted, and the BSS region in RAM_MAP filled with zeros.

### Phase 2: WAD Loading and Discovery

After BSS clearing, Doom calls `access()` to check for WAD files. The initial libc stub returned -1 for all `access()` calls, causing IWAD discovery to fail. Doom printed "Error: Game mode indeterminate" via `I_Error()` and called `exit()`, which translated to an `ebreak` instruction and then HALT at ROM PC 89788 after 59.8 million instructions.

**Fix:** Modified `access()` in `libc_monad.c` to return 0 for paths ending in ".wad", allowing IWAD discovery to succeed.

### Phase 3: The HashMap Catastrophe (Instruction ~99,400,000)

With WAD loading working, Doom progressed further but crashed with a CALLR fault at approximately 99.4 million instructions. The root cause: `RAM_MAP` was implemented as a BPF HashMap with 8 million entries (671 MB). When the map filled up, writes were silently dropped -- no error, no indication. This corrupted memory (function pointers, jump tables) because reads returned stale or zero values for addresses that should have been written.

**Fix:** Replaced HashMap with Array type (16 million entries, 128 MB). BPF Array maps never "fill up" -- every index is pre-allocated. This was committed as 3a1bbe7 in sprint S31.

### Phase 4: Protective Hardening

Even with the Array fix, Doom encountered numerous edge cases in the shareware WAD (doom1.wad) that would have crashed a native build. Each was addressed with a defensive patch:

- `I_Error()` made non-fatal (prints to debug buffer instead of calling `exit()`)
- `W_CacheLumpNum` returns NULL for invalid lump numbers
- `Z_Free` and `Z_ChangeTag` return early on ZONEID mismatch (prevents zone corruption cascade)
- `R_DrawColumn`, `R_DrawSpan`, `R_DrawFuzz` all bounds-check and return early
- `V_DrawPatch`, `V_CopyRect`, `V_DrawBlock` all bounds-check and return early
- `R_MapPlane`, `R_FindPlane`, `R_DrawPlanes` overflow-protected
- `R_InstallSpriteLump` warns instead of erroring on sprite duplicates
- `G_DoPlayDemo` returns early when demo lump is missing (shareware WAD)
- `Z_Malloc` returns NULL on allocation failure instead of entering infinite retry loop

### Phase 5: Virtual Time

Doom's game loop uses `DG_GetTicksMs()` to track elapsed time. When this returned wall-clock time, the discrepancy between real time and virtual execution time caused a "catch-up spiral" -- the game tried to simulate hundreds of missed tics, consuming resources without advancing meaningfully.

**Fix:** `DG_GetTicksMs()` returns a static incrementing counter instead of wall-clock time. `DG_SleepMs()` is a no-op. This gives Doom a consistent virtual time that advances proportionally to instructions executed.

### Phase 6: D_DoomLoop Alive (Instruction 819,000,000+)

With all fixes in place, `D_DoomLoop` entered its main rendering cycle and stayed there. The title screen appeared (TITLEPIC, 170 tics), followed by demo playback attempts (skipped in shareware), credit screens (200 tics each), and the full demo cycle repeat.

The framebuffer at RAM address 0x100000 contained valid Doom palette indices -- 99.9% non-zero pixels. The SCREEN_MAP BPF map reflected this data, readable by the doom-bridge service for WebSocket streaming to browser clients.

---

## Metrics

### Execution Performance

| Metric | Value |
|--------|-------|
| Instructions per hop | 128 (MAX_INSN_PER_TICK) |
| Bounces per packet | 255 |
| Instructions per packet | 32,640 |
| Injection rate (baseline) | ~333 pps (3000 us delay) |
| Instructions per second | ~1,350,000 |
| Instructions per frame | ~1,470,000 |
| Frames per second (baseline) | ~6 fps |
| Total instructions executed | 819,000,000+ |
| Total frames rendered | 559+ |

### BPF Map Statistics (STATS keys)

| Key | Name | Meaning |
|-----|------|---------|
| 0 | XDP_PASS | Packets that completed 255 bounces |
| 1 | PROCESSED | Total packets processed |
| 2 | INSNS | Total instructions executed |
| 3 | HALTED | CPU halt events |
| 4 | SLEEPING | Sleep cycle count |
| 5 | NO_STATE | Packets with no CPU state |
| 6 | MEM_FAULTS | Memory access faults |
| 7 | SYSCALLS | System call invocations |
| 8 | ROM_FAULT | ROM read faults |
| 9 | CACHE_HITS | Instruction cache hits |
| 10 | CACHE_MISSES | Instruction cache misses |

### Resource Usage

| Resource | Size |
|----------|------|
| ROM_MAP | ~360K entries (MBC bytecode) |
| RAM_MAP | 16M entries, 128 MB (Array) |
| doom1.wad | 4,196,020 bytes (loaded at 0x110000) |
| MBC ELF | ~720 KB compiled bytecode |
| CPU state | 104 bytes per instance |
| Framebuffer | 64,000 bytes (320 x 200) |

---

## Implications

### Computational Completeness

If a game engine -- with its rendering pipeline, memory allocator, WAD parser, input handling, and game logic -- can execute inside eBPF via packet-carried computation, then packet-level observability tracing is trivially achievable. The Monad protocol is not limited to metadata tagging; it is a general-purpose computational substrate operating at kernel datapath speed.

### XDP Capacity Headroom

The baseline injection rate of 333 packets per second uses approximately 0.003% of estimated XDP hardware capacity (10,000,000 pps). There is room for over 30,000x improvement before hitting kernel limits. The bottleneck is always userspace injection, never the data plane.

### Path to Production

The same BPF map infrastructure that runs Doom will run production packet tracing:
- `ROM_MAP` becomes trace logic bytecode
- `RAM_MAP` becomes per-flow state tables
- `SCREEN_MAP` becomes metrics export buffers
- `KBD_MAP` becomes control plane command interface
- The XDP circulation ring becomes the real packet path with trace insertion

---

*See also: [Architecture](architecture.md) | [Bug Kill Chain](bug-kill-chain.md) | [Performance](performance.md)*
