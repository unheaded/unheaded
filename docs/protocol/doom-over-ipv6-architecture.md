# Doom-over-IPv6 Architecture

**Version:** S30 (February 2026)
**Status:** Implementation in progress
**Authors:** Stevie Bellis, Claude AI agents

---

## Overview

Doom-over-IPv6 is a proof-of-concept that runs the original Doom engine on a
custom CPU implemented entirely in eBPF/XDP. IPv6 packets circulate through a
ring of network namespaces, and each XDP program invocation executes a fixed
number of MBC (Monad Bytecode) instructions. The packet itself serves as the
system clock: every hop through the ring is one "tick" of computation.

This is the ultimate demonstration of Unheaded's eBPF compute capabilities:
if we can run Doom in XDP, we can observe and control any workload.

---

## Memory Hierarchy (L0-L4)

The system uses a five-level memory hierarchy, from fastest (BPF registers) to
slowest (disk-backed WAD data).

```
L0: BPF Registers        ~0 ns     16 x 32-bit MBC regs in BPF map value
L1: BPF Map (per-CPU)    ~50 ns    CPU_MAP, ROM_MAP (hot path)
L2: BPF Map (shared)     ~100 ns   RAM_MAP, SCREEN_MAP, KBD_MAP
L3: Userspace Cache       ~1 us    Loader pre-populates large data
L4: Disk / WAD file       ~1 ms    doom1.wad (4 MiB), read once at load time
```

### BPF Map Layout

All BPF maps are pinned under `/sys/fs/bpf/unheaded/doom-ring/maps/`:

| Map Name       | Type       | Key Size | Value Size | Max Entries | Description |
|---------------|------------|----------|------------|-------------|-------------|
| `CPU_MAP`     | ARRAY      | 4        | 104        | 1           | MbcCpuState (regs, PC, flags, counters) |
| `ROM_MAP`     | ARRAY      | 4        | 4          | 262144      | MBC instruction words (1 MiB ROM) |
| `RAM_MAP`     | ARRAY      | 4        | 4          | 1048576     | General RAM + WAD data (4 MiB) |
| `SCREEN_MAP`  | ARRAY      | 4        | 1          | 64000       | 320x200 framebuffer (indexed color) |
| `KBD_MAP`     | ARRAY      | 4        | 16         | 1           | KeyboardState (key, pressed, sequence) |
| `RV2MBC_MAP`  | HASH       | 4        | 4          | 262144      | RISC-V address -> MBC PC translation |

### CpuState Layout (104 bytes)

```
Offset  Size   Field           Description
0       64     regs[16]        General-purpose registers (r0-r15, r15=SP)
64      4      pc              Program counter (index into ROM_MAP)
68      1      flags           Z(bit0), N(bit1), C(bit2)
69      1      halted          1 if HALT executed
70      1      stalled         1 if waiting for cache miss
71      1      _pad            Alignment padding
72      8      sleep_until_ns  bpf_ktime_get_ns() threshold for sleep
80      8      insn_count      Total instructions executed
88      8      cache_hits      L1 cache hit counter
96      8      cache_misses    L1 cache miss counter
```

---

## Packet-as-Clock-Tick Model

### The Circulation Ring

Six network namespaces (`monad0` through `monad5`) are connected in a ring via
veth pairs. Each veth pair uses a unique /64 IPv6 prefix to avoid NDP ambiguity:

```
monad0 --veth--> monad1 --veth--> monad2 --veth--> monad3 --veth--> monad4 --veth--> monad5
  ^                                                                                    |
  |______________________________________________________________________________________|
         default route fd00:dead::1 -> next hop in ring
```

Per-link addressing:
```
monad{i} <-> monad{i+1}: fd00:3f:75:{i}::1/64 <-> fd00:3f:75:{i}::2/64
```

### Tick Execution

1. **Tick injector** (`scripts/doom-tick.py`) sends an IPv6 UDP packet to `monad0`
   with destination `fd00:dead::1` (not in any connected /64).
2. The XDP program `monad_cpu` attached to `monad0`'s ingress executes
   `MAX_INSN_PER_TICK` (16) MBC instructions from ROM.
3. After execution, XDP returns `XDP_TX` to forward the packet to `monad1`.
4. Each subsequent hop executes another 16 instructions.
5. After 6 hops (monad5 -> monad0), one full circulation is complete.

### Throughput

```
Instructions per packet:  16 instructions/hop x 6 hops = 96 instructions/circulation
Max packet rate:          ~8,600 packets/second
Effective throughput:     96 x 8,600 = ~825,600 instructions/second
                          With 255 ticks/packet: 16 x 255 = 4,080 insn/pkt
                          4,080 x 8,600 = ~35 million instructions/second
```

### Timing

At ~35M insns/sec effective clock:
- BSS clear (~60M insns): ~1.7 seconds
- Doom startup to main menu: ~5-10 seconds
- Frame rendering: ~2-5 seconds per frame at this clock rate

---

## Screen Rendering Pipeline

```
Doom game code
    |
    v
SYSCALL 0x01 (SYS_DRAW_FRAME)
    |  r1 = framebuffer address in RAM
    v
BPF monad_cpu: copy RAM[r1..r1+64000] -> SCREEN_MAP[0..64000]
    |
    v
Dashboard endpoint: GET /doom/screen
    |  Reads SCREEN_MAP via BPF map accessor
    v
HTTP response: 64000 bytes (raw) or base64 JSON
    |
    v
doom.html: Canvas 2D rendering
    |  8-bit indexed -> RGBA via palette lookup
    v
Browser canvas (320x200, scaled 2x/3x)
```

### Frame Format

The framebuffer uses 8-bit indexed color (VGA palette, 256 entries).
Each byte in the 64000-byte buffer is a palette index. The viewer maps
palette indices to RGB values using the Doom PLAYPAL lump from the WAD file.
A grayscale fallback is used when no palette is loaded.

---

## Keyboard Input Pipeline

```
Browser keydown/keyup event
    |
    v
doom.html: JavaScript key mapping
    |  Maps browser key codes to Doom scancodes
    v
POST /doom/input { key: scancode, pressed: bool }
    |
    v
Dashboard handler: InjectKey(kbdMap, scancode, pressed)
    |  Writes KeyboardState to KBD_MAP[0] with monotonic sequence number
    v
BPF monad_cpu: SYSCALL 0x02 (SYS_GET_KEY)
    |  Reads KBD_MAP[0], returns key/pressed in r0/r1
    v
Doom game code: D_ProcessEvents()
```

### KeyboardState Layout (16 bytes)

```
Offset  Size   Field      Description
0       4      key        Scancode
4       4      pressed    1=pressed, 0=released
8       8      sequence   Monotonically increasing (BPF detects changes)
```

### Doom Scancode Mapping

| Browser Key   | Scancode | Doom Action |
|--------------|----------|-------------|
| ArrowUp / W  | 0x48     | Move forward |
| ArrowDown / S| 0x50     | Move backward |
| ArrowLeft / A| 0x4B     | Turn left |
| ArrowRight / D| 0x4D    | Turn right |
| Ctrl         | 0x1D     | Fire |
| Space        | 0x39     | Use/Open |
| Shift        | 0x36     | Run |
| Escape       | 0x01     | Menu |
| Enter        | 0x1C     | Confirm |
| 1-9          | 0x02-0x0A| Weapon select |

---

## ROM Toolchain

### Compilation Pipeline

```
Doom C source (doomgeneric)
    |
    v
Cross-compiler: riscv32-unknown-elf-gcc
    |  Doom libc stubs: libc_monad.c (access, fopen, etc.)
    |  CRT0: crt0_monad.S (BSS clear, stack setup)
    |  Platform: doomgeneric_monad.c (framebuffer, input)
    v
ELF binary: doom_monad.elf (RISC-V 32-bit, no OS)
    |
    v
Translator: rv32i-to-mbc (crates/monad-mbc)
    |  Reads ELF .text section
    |  Translates each RV32I instruction to 1-3 MBC instructions
    |  Builds RV2MBC address map for indirect jumps
    v
MBC binary: doom.mbc (sequence of 32-bit instruction words)
    |
    v
Loader: doom-loader.sh + doom-loader-core.py
    |  Writes ROM_MAP entries via bpftool
    |  Writes RV2MBC_MAP entries
    |  Writes WAD data to RAM_MAP
    |  Initializes CPU_MAP with default state
    v
BPF maps pinned at /sys/fs/bpf/unheaded/doom-ring/maps/
```

### MBC Assembler

For testing and small programs, the MBC assembler (`crates/monad-mbc/src/assembler.rs`)
provides direct assembly:

```asm
; Example: draw a pattern to screen
.org 0x10                    ; Start code at PC 16
MOVI r0, 0xC000             ; r0 = screen base address
MOVI r1, 0                  ; r1 = pixel value
MOVI r2, 64000              ; r2 = screen size

loop:
    STB [r0+0], r1          ; Write pixel
    ADDI r0, 1              ; Next pixel
    ADDI r1, 1              ; Next color
    SUB r2, r1              ; Decrement counter
    JNZ loop                ; Loop until done
    DRAW_FRAME              ; SYSCALL 0x01
    HALT
```

Assembler directives:
- `.data` / `.text` -- switch between data and code sections
- `.org <addr>` -- set assembly origin (NOP-fills gaps)
- `.byte <val>, ...` -- embed raw bytes (packed into 32-bit words, LE)
- `.half <val>, ...` -- embed 16-bit half-words
- `.word <val>, ...` -- embed 32-bit words

---

## Ring Setup

The ring is created by `scripts/doom-ring.sh`:

1. Create 6 network namespaces (`monad0`-`monad5`)
2. Create veth pairs connecting adjacent namespaces
3. Assign unique /64 IPv6 prefixes to each link
4. Configure default routes to forward packets around the ring
5. Enable IPv6 forwarding in each namespace
6. Load the `monad_cpu` XDP program once on `monad0`
7. Attach the same program to all other namespaces via `bpftool net attach xdpgeneric`
8. All 6 hops share the same BPF program and maps (single load, shared maps)

### Critical Design Decisions

- **Per-link /64 prefixes required**: All veth pairs MUST use different /64
  prefixes. Using the same /64 causes kernel routing ambiguity and NDP failure.
- **Destination outside connected /64**: Test packets use `fd00:dead::1` which
  is not in any connected subnet, forcing default-route forwarding.
- **Single BPF load, shared maps**: The XDP program is loaded once on hop0.
  Other hops attach the same prog_id via bpftool. All 6 hops share the same
  pinned maps for consistent state.

---

## Dashboard Integration

The Doom viewer is served at `/doom.html` and communicates with three HTTP endpoints:

| Endpoint          | Method | Description |
|-------------------|--------|-------------|
| `/doom/screen`    | GET    | Read 64000-byte framebuffer (raw or base64 JSON) |
| `/doom/status`    | GET    | Read CPU state (PC, regs, flags, counters) as JSON |
| `/doom/input`     | POST   | Write keyboard event (scancode + pressed) to KBD map |

The viewer polls `/doom/screen` at ~30 FPS and `/doom/status` every 200ms.
Keyboard events are sent immediately on keydown/keyup.

---

## Key File Paths

| Component | Path |
|-----------|------|
| BPF CPU program | `ebpf/monad-cpu-ebpf/src/main.rs` |
| MBC ISA types | `ebpf/monad-common/src/lib.rs` |
| MBC assembler | `crates/monad-mbc/src/assembler.rs` |
| MBC translator | `crates/monad-mbc/src/translator.rs` |
| Translator CLI | `crates/monad-mbc/src/bin/rv32i_to_mbc.rs` |
| Doom libc stubs | `doom/doomgeneric/doomgeneric/libc_monad.c` |
| Doom platform layer | `doom/doomgeneric/doomgeneric/doomgeneric_monad.c` |
| CRT0 startup | `doom/doomgeneric/doomgeneric/crt0_monad.S` |
| Doom Makefile | `doom/doomgeneric/doomgeneric/Makefile.monad` |
| Ring setup | `scripts/doom-ring.sh` |
| ROM loader | `scripts/doom-loader.sh`, `scripts/doom-loader-core.py` |
| Tick injector | `scripts/doom-tick.py` |
| CPU dump tool | `scripts/doom-cpu-dump.py` |
| Go types | `internal/doom/types.go` |
| Go CPU state | `internal/doom/state.go` |
| Go input | `internal/doom/input.go` |
| Go HTTP handlers | `internal/doom/handlers.go` |
| Dashboard viewer | `dashboard/doom.html` |
| ISA reference | `docs/protocol/mbc-isa-reference.md` |

---

## Known Limitations

1. **BSS clearing is slow**: Doom's ~6MB BSS takes ~60M instructions to clear
   byte-by-byte in CRT0. This takes ~1.7 seconds at maximum throughput.

2. **access() stub**: The default libc access() stub returns -1, causing IWAD
   discovery to fail. The stub must return 0 for `.wad` paths.

3. **Effective clock rate**: At ~35M insns/sec, Doom runs far below its original
   35 MHz 486 target. Frame rates are ~0.5-1 FPS, sufficient for proof-of-concept
   but not playable in real-time.

4. **No sound**: The platform layer does not implement audio output.

5. **Single-buffered display**: SYSCALL 0x01 copies the entire framebuffer
   synchronously. Tearing may occur if the dashboard reads during a copy.
