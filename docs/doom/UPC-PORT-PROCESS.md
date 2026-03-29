# UPC Port Process — How to Port Any C Program to the Unheaded Protocol Computer

**Applicable to:** Any C program targeting the MBC virtual machine running on eBPF XDP over IPv6.

## Overview

The Unheaded Protocol Computer (UPC) executes MBC bytecode inside eBPF XDP programs.
Packets carry Monad headers through a ring of network namespaces. Each hop executes
MBC instructions. The computation IS the network traversal.

The port process turns any C program into MBC bytecode that runs over IPv6.

## Pipeline

```
source.c (C) → source.elf (RV32I bare-metal) → source.mbc (MBC bytecode) → BPF maps → XDP execution
```

## Step-by-Step Process

### Phase 1: Inventory the Source

1. **Count files**: How many .c and .h files?
2. **Identify platform deps**: Which files use OS-specific APIs? (X11, sockets, audio, files)
3. **Separate portable code from platform code**: Game logic, math, data structures = portable. I/O, graphics, sound, network = needs stubs.
4. **Check for float/double**: RV32I without F extension has no hardware float. Either stub, use lookup tables, or add soft-float runtime.

### Phase 2: Create the Build System

1. **Cross-compiler**: `riscv64-unknown-elf-gcc` with `-march=rv32i -mabi=ilp32`
2. **Bare-metal flags**: `-nostdlib -nostdinc -ffreestanding -fno-builtin`
3. **Register restriction**: `-ffixed-x16` through `-ffixed-x31` (MBC uses 16 registers, RV32I has 32)
4. **Include path**: Custom minimal libc headers in `include/` directory, then GCC builtins
5. **Makefile**: Compile all portable source files from original location + platform stubs from port directory

### Phase 3: Provide libc Stubs

Create `libc_stubs.c` with bare-metal implementations of C standard library functions:

**Essential (almost every C program needs these):**
- `malloc`, `calloc`, `realloc`, `free` — bump allocator with bounds checking
- `memcpy`, `memset`, `memmove`, `memcmp` — byte-by-byte implementations
- `strlen`, `strcmp`, `strncmp`, `strcpy`, `strncpy`, `strcat` — string ops
- `printf`, `sprintf`, `vsnprintf` — format string output (can be simplified)
- `exit`, `abort` — write debug message, then `ebreak` (MBC halt)

**File I/O (if program reads files):**
- POSIX: `open`, `read`, `lseek`, `close`, `fstat` — fd table mapping to memory-mapped data
- stdio: `fopen`, `fread`, `fseek`, `ftell`, `fclose` — FILE table wrapper

**Math (if program uses math):**
- `gcc_runtime.c` — software division and multiplication for RV32I without M extension
- Lookup tables for trig functions if needed

### Phase 4: Create Platform Stubs

Replace every OS-specific file with a MBC-compatible stub:

**Pattern for each platform file:**
1. Read the original (e.g., `i_video.c` with X11 code)
2. Extract function signatures from the header (e.g., `i_video.h`)
3. Write a new file (e.g., `i_video_mbc.c`) implementing each function
4. Graphics: write pixels to SCREEN_BASE memory address
5. Input: read from KBD_MAP via MBC syscall
6. Timing: MBC syscall SYS_GET_TICKS
7. Sound: no-op (unless UPC has audio hardware)
8. Network: no-op for single-player, or use Monad transport for multiplayer

### Phase 5: Create Startup Code

`crt0.S` — minimal RV32I assembly:
```asm
_start:
    lui   sp, %hi(_stack_top)      # Set stack pointer
    addi  sp, sp, %lo(_stack_top)
    # Zero BSS
    lui   a0, %hi(__bss_start)
    addi  a0, a0, %lo(__bss_start)
    lui   a1, %hi(__bss_end)
    addi  a1, a1, %lo(__bss_end)
.Lbss_loop:
    bge   a0, a1, .Lbss_done
    sw    zero, 0(a0)
    addi  a0, a0, 4
    j     .Lbss_loop
.Lbss_done:
    jal   ra, main
    ebreak                         # Halt after main returns
```

### Phase 6: Define Memory Layout

`linker.ld` — defines where code, data, heap, screen, and data files go:

```ld
MEMORY {
    ROM    (rx)  : ORIGIN = 0x00000000, LENGTH = 1M      /* Code */
    RAM    (rw)  : ORIGIN = 0x00100000, LENGTH = 768K    /* Data + BSS */
    SCREEN (rw)  : ORIGIN = 0x00070000, LENGTH = 64000   /* Framebuffer */
    HEAP   (rw)  : ORIGIN = 0x001C0000, LENGTH = 26M     /* Heap */
    DATA   (r)   : ORIGIN = 0x01C00000, LENGTH = 16M     /* Data files */
}
```

**Key constraints:**
- Regions MUST NOT overlap
- Stack grows down from above the last region
- doom-runner/src/memory.rs is the authoritative source — linker.ld and libc_stubs.c must match

### Phase 7: Compile, Link, Translate

```bash
make                                    # Compile all .o files
# Produces: program.elf (RV32I)
rv32i-to-mbc program.elf -o program.mbc --stats
# Produces: program.mbc (MBC bytecode) + program.rv2mbc (address map)
```

### Phase 8: Load and Run

```bash
# doom-runner (or a generic mbc-runner) loads MBC into BPF maps
sudo ./doom-runner run --doom-mbc program.mbc --doom-elf program.elf \
  --rv2mbc program.rv2mbc --wad /path/to/data.file --hops 2

# Attach XDP to namespace interfaces
PROG_ID=$(sudo bpftool prog list | grep monad_cpu | tail -1 | awk '{print $1}' | tr -d ':')
sudo ip netns exec monad1 bpftool net attach xdpgeneric id $PROG_ID dev veth01p
sudo ip netns exec monad0 bpftool net attach xdpgeneric id $PROG_ID dev veth10p

# Inject packets to start execution
sudo nsenter --net=/var/run/netns/monad0 ./bin/doom-go-injector \
  --src-mac $SRC --dst-mac $DST --iface veth01 --count 0 --mode sendmmsg
```

### Phase 9: Debug Iteratively

**Instrumentation pattern:**
1. Add `debug_breadcrumb(milestone_id)` calls at key init points
2. Write error messages to a fixed debug address (e.g., 0x7BF000)
3. Read BPF maps post-mortem via `bpftool map lookup`
4. Check PC is in valid ROM range (critical — invalid PC = infinite NOPs)
5. Check STATS counters: HALTED, INSNS_EXECUTED, FRAME_READY

**Common failure modes:**
- PC corruption → add bounds check in eBPF executor
- Heap overflow → increase HEAP_END or reduce allocations
- WAD/data file not found → fix open()/access() stubs
- Missing libc function → compiler error or runtime crash, add stub
- Stack overflow → increase _stack_top

### Phase 10: Optimize

Once the program runs correctly:
1. **Tail calls**: Increase insns/hop for more throughput
2. **sendmmsg injection**: Batch packet sends for higher pps
3. **More hops**: Each hop processes, so more hops = more processing per circuit
4. **Profile hot paths**: Which functions consume the most instructions?

## Lessons Learned (from DOOM port)

1. **"More instructions" ≠ "more progress"** — always verify PC is in valid ROM range
2. **Map alignment is critical** — the entity that loads the BPF program MUST write to its maps
3. **Isolate allocator state** — heap_ptr at a fixed address far from .data
4. **One change at a time** — verify after each change, then next
5. **Document everything** — findings, dead ends, what works, what doesn't
6. **The Marshal prevents drift** — always have oversight to catch hallucinations

## Reusable Components

These can be used for ANY UPC port, not just DOOM:

| Component | Location | Purpose |
|-----------|----------|---------|
| crt0.S | demos/doom/crt0.S | RV32I bare-metal startup |
| libc_stubs.c | demos/doom/libc_stubs.c | 111+ libc functions |
| gcc_runtime.c | demos/doom/gcc_runtime.c | Software div/mul |
| include/ | demos/doom/include/ | Minimal libc headers |
| doom-runner | crates/doom-runner/ | Aya-based BPF loader (rename to mbc-runner) |
| monad-cpu-ebpf | ebpf/monad-cpu-ebpf/ | XDP MBC executor |
| doom-go-injector | cmd/doom-go-injector/ | Packet injector |
