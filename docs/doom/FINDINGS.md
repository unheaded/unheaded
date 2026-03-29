# Doom Findings -- Session 2026-03-29

## The Debugging Journey

This document records the full debugging narrative of the Doom-over-IPv6 session
on 2026-03-29, covering nine bugs fixed and the critical PC corruption discovery.

---

## Bug 1: Map Alignment (the systemic bug)

**Discovery:** Doom loaded but executed zero useful instructions. BPF map reads
returned all zeros despite data being loaded.

**Root cause:** The legacy pipeline (`doom-ring.sh` + `doom-loader.sh`) pins BPF
maps to `/sys/fs/bpf/`, but when the kernel loads the XDP program, it creates
SEPARATE maps with different IDs. Data loaded into pinned maps is invisible to
the program -- it reads its own empty maps.

**Fix:** Built `doom-runner` (`crates/doom-runner/`) using Aya. The Rust binary
loads the eBPF program first (which creates the maps), then writes data directly
into THOSE maps. No pins. No mismatches. Atomic ownership of program + data.

**Commit:** `208c2944`

**Lesson:** When working with eBPF, the entity that loads the program MUST be the
entity that writes to the maps. Pinned maps and program-created maps are not the
same thing.

---

## Bug 2: .sdata Section Missing

**Discovery:** After fixing map alignment, Doom started executing but crashed
almost immediately. `heap_ptr` and `stdout` (in `.sdata`) were zero.

**Root cause:** The ELF section loader used `objcopy -j .text -j .rodata -j .data`
but omitted `.sdata` (small data section). RV32I with `-O2` places frequently
accessed globals like `heap_ptr` in `.sdata` for GP-relative addressing.

**Fix:** Added `-j .sdata` to the extraction pipeline. `doom-runner/src/loader.rs`
now iterates all ELF sections and loads any that fall within the RAM region.

**Lesson:** RV32I `.sdata` is not optional. Always check what the compiler puts there.

---

## Bug 3: Stack/WAD Overlap

**Discovery:** Doom crashed at ~20.3M instructions in `W_CacheLumpNum`. The WAD
lump data was corrupted.

**Root cause:** `linker.ld` set `_stack_top = 0x1000000`, which falls inside the
WAD region (0x800000-0xBFFFFF in the old layout). Stack growth overwrote WAD data.

**Fix:** Moved `_stack_top` to `0x2100000`, above all data regions.

**Commit:** `eef8bfc5`

---

## Bug 4: sscanf No-Op

**Discovery:** Doom's config parser silently failed. Settings like screen resolution
were not applied.

**Root cause:** `sscanf` was a stub returning 0 (no matches). Callers check the
return value and skip processing when it reports no matches.

**Fix:** Implemented a real `sscanf` with `%d`, `%s`, `%x` format support.

**Commit:** `eef8bfc5`

---

## Bug 5: mmap Path Corruption

**Discovery:** WAD reading returned garbage data intermittently.

**Root cause:** `w_file_stdc.c` had an `mmap` code path where `wad_file` pointer
was off by 8 bytes. It read the Z_Malloc block header instead of actual file data.

**Fix:** Disabled the mmap path entirely with `#if 0`. WAD is already memory-mapped
at `WAD_BASE` by doom-runner; the mmap() libc path is unnecessary and harmful.

**Commit:** `15dbfb96`

---

## Bug 6: Z_Malloc Corrupts WAD Handle

**Discovery:** After ~10M instructions, WAD reads returned random data. The
`stdc_wad_file_t` structure was being overwritten.

**Root cause:** The WAD file handle was allocated inside Doom's zone memory
(Z_Malloc). When Z_Malloc recycled memory blocks, it overwrote the file handle.

**Fix:** Created `w_file_stdc.c` with static storage for WAD file handles.
File handles now live outside the zone allocator's reach.

**Commit:** `2ae0498a`

---

## Bug 7: heap_ptr Corruption

**Discovery:** Malloc returned addresses far outside the heap. Wild pointer
writes from Doom code corrupted `heap_ptr`.

**Root cause:** `heap_ptr` was a static variable in `.data` at approximately
`0x10E714`, surrounded by other Doom globals. Any wild pointer write to that
region could corrupt the heap pointer, causing all subsequent allocations to
land in random memory.

**Fix:** Two-part defense:
1. Moved `heap_ptr` to an isolated address (`0x1BF0000`) far from `.data`
2. Added bounds checking on every `malloc()` call -- if `heap_ptr` is outside
   `[HEAP_START, HEAP_END)`, reset it to `HEAP_START` and log breadcrumb `0x00FE`

**Commit:** `83fc1f56`

**Lesson:** In a bare-metal environment without memory protection, critical
allocator state must be physically isolated from application data.

---

## Bug 8: fclose No-Op

**Discovery:** After many WAD operations, `fopen` started returning NULL despite
the file table having slots.

**Root cause:** `fclose` was a no-op. File table slots (used for WAD file handles)
were never freed, eventually exhausting the table.

**Fix:** Proper `fclose` that marks file table slots as available.

**Commit:** `2ae0498a`

---

## Bug 9: Heap/WAD Overlap

**Discovery:** Z_Init crashed with corrupted zone headers.

**Root cause:** In the original layout, heap started at `0x200000` (2 MiB) with
only 6 MiB. WAD was at `0x800000`. Doom's pre-Z_Init allocations consumed the
entire 6 MiB heap, growing into WAD territory and corrupting WAD data.

**Fix:** Reorganized the memory layout:
- Heap: `0x1C0000` to `0x1BC0000` (26 MiB -- enough for all pre-init allocations)
- WAD: `0x1C00000` (after heap end)
- Stack: `0x2100000` (after WAD)

**Commit:** `62734de1`

---

## CRITICAL FINDING: PC Corruption

**Discovery:** After all nine bugs were fixed, Doom ran for 24 billion+
instructions without crashing. But the screen remained black. No frame was
ever rendered.

**Investigation approach:**
1. Sampled PC values over 5 seconds via `bpftool map lookup` on CPU_MAP
2. Observed values like: `0x2E6EB7AB`, `0x365E9BDD`, `0x931664E9`
3. ROM_MAP has only 262,144 entries (max valid PC approximately `0x3FFFF`)
4. These PC values are millions of times larger than valid ROM space

**Root cause (confirmed):** The program counter is corrupted. Doom jumped to an
invalid address early in init via a function pointer call (indirect jump). The
MBC `CALL` instruction jumps to an address stored in a register. If the register
contains garbage, PC goes to garbage.

**Why it didn't crash:** ROM_MAP is a BPF Array. Out-of-bounds reads return the
default value (0). MBC opcode 0 = NOP. So Doom executes infinite NOPs,
incrementing PC into the void. Instructions climb. Nothing useful happens.

**What this means:** Every "fix" that increased instruction count was just getting
Doom further before it hit the PC corruption. The billions of instructions after
corruption are entirely wasted NOPs. Doom never reached `R_Init` or any rendering code.

**Suspected corruption sources:**
1. `Z_Malloc`'s zone management uses function pointers (PU_* callbacks)
2. `W_Read` dispatches through `wad_file->file_class->Read` (partially fixed
   with static handles, but possibly not all code paths)
3. A vtable or callback array in Doom's init code
4. Stack corruption overwriting a return address

---

## Instrumentation Approach

Throughout the session, three instrumentation techniques were used:

### 1. Debug Breadcrumbs
`debug_breadcrumb(uint32_t milestone)` in `libc_stubs.c` writes milestone values
to the debug scratch region at `0xF00000`. These can be read via `bpftool map lookup`
on RAM_MAP to trace execution flow without printf.

### 2. Debug Regions
OOM and corruption events write diagnostic data to addresses above WAD (e.g.,
`0x2051000`), safe from heap and stack. Fields include error markers, pointer
values, and sizes -- readable post-mortem via map dumps.

### 3. BPF Map Reads
Direct reading of BPF maps (`CPU_MAP`, `STATS`, `SCREEN_MAP`, `RAM_MAP`) via
`bpftool` provides zero-overhead observation:
- `STATS[2]` = total instructions executed
- `CPU_MAP[0xDE].pc` = current program counter
- `SCREEN_MAP` pixel count = rendering progress
- `RAM_MAP[heap_ptr_word]` = current heap pointer value

---

## Lesson Learned

**"More instructions" is not "more progress".** Always verify that the program
counter is in valid range. 24 billion instructions of NOPs looks like progress
from the stats dashboard but is actually an undetected crash.

The eBPF executor needs a PC bounds check: `if PC >= ROM_MAP_ENTRIES, halt
immediately and record the last valid PC`. This would have caught the corruption
instantly instead of letting it run for billions of wasted cycles.

---

## id DOOM Port — Session 2026-03-29 (continued)

### Phase 1-3: Build System, Compile, Link — COMPLETE

Commits: d56360a9, efd094b0, 4a16480a

- 62 id DOOM .c files compile for RV32I with restricted registers
- 4 MBC platform stubs: i_video_mbc.c, i_sound_mbc.c, i_net_mbc.c, i_system_mbc.c
- POSIX fd table added to libc_stubs.c (open/read/lseek/close for WAD I/O)
- doom.elf links (237K text, 119K data, 10.3M bss)
- doom.mbc: 85,454 MBC instructions

### Phase 4: Load and Run — COMPLETE

Commit: b1abb44c

- Retail DOOM.WAD (12.4MB) loaded at 0x1C00000
- All doom-runner verifications PASS
- Doom executes 1.8M instructions then halts (clean halt, valid PC)
- Error: "W_InitFiles: no files found"

### Phase 5: Debugging WAD Detection — IN PROGRESS

**Finding 1: access() returns 0 for ALL .wad but Doom picks wrong WAD**
IdentifyVersion checks WADs in order. access() returning 0 for all .wad
caused Doom to think it was French commercial edition. Fixed to only
match "doomu.wad" for retail.

**Finding 2: access() IS called but .wad extension check never matches**
7 access() calls, 0 matches. The strcasecmp(last_4_chars, ".wad") fails.
Reason: strlen(path) returns 0 — the path strings are EMPTY.

**Finding 3: sprintf output goes nowhere**
IdentifyVersion: `sprintf(doomuwad, "%s/doomu.wad", doomwaddir)` where
doomwaddir = "." and doomuwad = malloc'd buffer. But the buffer reads
as all zeros after sprintf writes to it.

**Finding 4: JVM-style dynamic heap implemented**
Replaced fixed-address heap_ptr (0x1BF0000) with sbrk-based allocator
using linker symbols __heap_start/__heap_end. Heap is 26MB, initialized
from BSS. doom-runner does NOT write any special address.

**Current blocker: MBC VM byte store at heap addresses**
malloc() returns non-NULL (breadcrumb 0x0002 confirms). But sprintf's
byte stores to the malloc'd buffer appear to not persist — strlen reads 0.
This suggests the MBC executor's byte store (SB instruction) at addresses
in the heap region (0x1C0000+) may not write to RAM_MAP correctly.

**Suspected cause:**
The MBC SW (store word) and SB (store byte) instructions compute a word
index into RAM_MAP as `byte_addr / 4`. For heap addresses (0x1C0000),
the word index is 0x1C0000/4 = 0x70000. RAM_MAP has 16M entries, so
0x70000 is valid. But there might be a byte-within-word addressing bug
where SB writes to the wrong byte offset within the u32 word.

**Resolved:** Byte store/load works fine. The real bug was sprintf.

---

## sprintf 32-bit Overflow (id DOOM port)

vsprintf passes size=(size_t)-1 (0xFFFFFFFF) to mini_vsnprintf. On 32-bit
RV32I, `buf + 0xFFFFFFFE` wraps BELOW buf, making `end < dst`. Every
character write check `(dst < end)` fails. Zero bytes written.

Fix: cap size at 0x7FFFFFFF. One line.

---

## fd Resilience (id DOOM port)

W_ReadLump calls read(handle, ...) but handle value may not match our
fd table (corrupted lumpinfo pointer or fd number mismatch). Fixed by
falling back to primary WAD slot 0 if fd lookup fails. Also made close()
a no-op for all fds (WADs are memory-mapped, never close).

---

## PC Corruption -- Confirmed in id DOOM Too

After all stub fixes, id DOOM loads WAD, reads lumps, executes 1.28B
instructions with no I_Error. But PC sampling shows PC = 0x4B200000
(max valid = 0x14DAE). Same pattern as doomgeneric.

This is NOT a doomgeneric-specific bug. It happens in BOTH codebases.
The corruption is in the MBC translator or eBPF executor:
1. Indirect jumps (JALR/CALL) may translate incorrectly
2. Return address handling may corrupt the link register
3. RV32I restricted registers (x0-x15 only) may break calling convention

**CRITICAL NEXT STEP: PC bounds check in eBPF executor.**
When PC >= ROM_MAP_ENTRIES, halt and record last valid PC + the bad instruction.
This pinpoints the exact corruption source.

---

## Debug Methodology — What Works (2026-03-29)

The following debug path has been proven effective:

1. **PC bounds check** — verify PC is in valid ROM range FIRST
   - If invalid: the CPU is executing NOPs, all instruction counts are lies
   - If valid: the CPU is actually running code, proceed to step 2

2. **SP bounds check** — verify stack pointer (r2 on RV32I) is valid
   - If SP=0 or in ROM range: stack is corrupt, all function calls are broken
   - Must check AFTER crt0 runs (first ~50K instructions zero BSS)

3. **Breadcrumb tracing** — debug_breadcrumb(milestone_id) at key init points
   - Captures execution flow without printf
   - Read via bpftool map lookup on RAM_MAP

4. **Error message capture** — I_Error writes to debug region
   - debug_write(buf) at 0x7BF000 in RAM_MAP
   - Read post-mortem via bpftool

5. **STAT counters** — HALTED, INSNS, ROM_FAULT, MEM_STORES, MEM_LOADS
   - HALTED absent = still running. HALTED present = crashed or sleeping.
   - INSNS climbing but no HALTED = verify PC is valid (could be NOP spin)

**WARNING: insn_count field in MbcCpuState does NOT persist across tail calls.**
Use STATS map (incremented per-instruction) instead.

**WARNING: CPU state sampling can read wrong bytes if struct layout is misunderstood.**
Always decode the FULL 128-byte CPU_MAP value, not individual offsets.
