# Doom Findings -- Session 2026-03-29

> ## ✅ 2026-07-29 — Retail DOOM.WAD restored; IWAD flavour now detected, not hardcoded
> **Symptom:** shareware `doom1.wad` ran fine, retail `DOOM.WAD` did not.
>
> **Root cause:** the `access()` stub in `demos/doom/libc_stubs.c` hardcoded a single
> filename. id DOOM's `IdentifyVersion()` (d_main.c) selects `gamemode` by `access()`-probing
> `doom2f.wad → doom2.wad → plutonia.wad → tnt.wad → doomu.wad → doom.wad → doom1.wad` and
> taking the first hit, so whichever name the stub reports as present **pins the game mode for
> the whole build**. Commit `625c0b9b` changed that name from `doomu.wad`/`doom.wad` to
> `doom1.wad` to fix a shareware DEMO4 crash — a correct fix for shareware that silently made
> retail unrunnable. The two IWADs cannot share one hardcoded name:
>
> | IWAD | Needs mode | Because |
> |------|-----------|---------|
> | `doom1.wad` (4,196,020 B) | shareware | demo loop is `%6`; the WAD has no `DEMO4` — `retail` mode `I_Error`s ~5s in |
> | `DOOM.WAD` (12,408,292 B) | retail | Ultimate DOOM has `E4M1` + `DEMO4` but **no `HELP2`**, which `registered` mode requires |
>
> Note this was never a WAD *size* problem — `WAD_MAX_SIZE` was already 16 MiB and the real
> size was already published via `WAD_SIZE_ADDR`. The old RUNBOOK "known issue #3" blaming a
> size check was a red herring and has been corrected.
>
> **Fix:** detect the flavour from the lump directory at load time instead of hardcoding it.
> `loader::detect_iwad_flavour()` scans for `MAP01` → commercial, `E4M1` → retail, `E2M1` →
> registered, else shareware, and doom-runner writes the matching probe name to a new
> `WAD_IWAD_NAME_ADDR` (0x01BF_FFC0, 16 B) in the scratch page just below `WAD_SIZE_ADDR`.
> `access()` compares the basename against that string. Detection is content-based on purpose:
> the retail IWAD ships *named* `DOOM.WAD`, which maps to the wrong (`registered`) mode.
>
> **Verified 2026-07-29** on WEST, full pipeline (ring + XDP + injector, ~89k pkt/s):
> - `DOOM.WAD` → `detected IWAD flavour: Retail (2306 lumps) — probe name 'doomu.wad'`;
>   renders **THE ULTIMATE DOOM** title screen + the retail credits page ("SPECIAL THANKS TO
>   EPISODE FOUR"); ran >10 min / 13.4M packets / ~878B instructions with no `I_Error`.
> - `doom1.wad` → `Shareware (1264 lumps) — probe name 'doom1.wad'`; renders the shareware
>   title screen ("$9.00" banner) and survives well past the old ~5s DEMO4 crash. No regression.
>
> **Testing gotcha:** `pkill -f doom-runner` did not reliably kill the sudo-launched runner. A
> stale instance kept port 16666 and the "shareware" run died with `Address already in use`
> while the bridge happily served *retail* frames — briefly looked like a shareware bug.
> Check `ss -lntp | grep 16666` is empty between runs, and kill by PID.
>
> Also fixed in passing: `deploy/systemd/unheaded-doom-runner.service` was missing the `run`
> subcommand and the required `--doom-mbc`/`--doom-elf` args, and pointed `--wad` at a
> nonexistent path — systemd startup could not have worked as written.

> ## ✅ 2026-07-03 UPDATE — Doom RAN with ADR-079 live; PC valid, no corruption
> Brought Doom online through the full doom-runner pipeline (commit `7eba83e0`). **Doom loads,
> executes, and renders**: PC stays in valid ROM range (sampled 0x130bb ≪ 86463 — NO corruption
> into the billions), the ADR-079 tagged return address is live (`regs[14]=0x800130fa`, bit-31
> tag → untags to a valid ROM PC), FRAME_READY climbs past 14K, and the RAM framebuffer at
> 0x70000 is full of real DOOM palette pixels. So the ADR-079 tag scheme is **verified compatible
> with Doom** (whatever the historical PC-corruption was, tagging did not reintroduce it).
>
> **Two things that block/unblock Doom, learned the hard way:**
> 1. **The non-ascend (Doom) object must be LOAD-tested, not just compile-tested.** Ten xv6-only
>    syscall arms had accumulated in the shared object and DCE was not gating them out of the
>    Doom build, pushing it just over the 1M verifier ceiling — doom-runner could not load it.
>    Fixed by gating them `cfg!(feature = "ascend-linux")`. Non-ascend now sits right at the
>    edge of 1M; **add a real load-check to CI** (bpf-verifier-check.sh only compile/analyzes).
> 2. The framebuffer the bridge serves is **RAM_MAP @ 0x70000**, not SCREEN_MAP (word stores
>    only land in RAM_MAP). SCREEN_MAP reading ~0 is normal; check RAM_MAP for pixels.
>
> _(Original 2026-07-02 hypothesis below — now confirmed Doom-safe.)_

> ## ⚡ 2026-07-02 UPDATE — test ADR-079 FIRST next Doom shift (strong PC-corruption fix candidate)
> The ASCEND-LINUX work found and fixed a return-address disambiguation bug in the
> **shared** eBPF interpreter that is a strong candidate for (part of) the PC-corruption
> blocker below. The RET handler distinguished a saved MBC PC from a raw RV byte address
> by a **magnitude floor** (originally `0x10000`). Doom's `doom.rv2mbc` shows Doom's MBC
> PCs span **`[0x2, 0x151BF]`** — so **every Doom RET whose return address ≥ 0x10000 was
> misparsed as an RV byte address**, sent through the `RV2MBC_MAP` lookup, and jumped to a
> wrong/garbage target early in init. That matches this doc's signature exactly ("jumped
> to an invalid address early in init via an indirect path," then PC climbs through NOPs
> into the billions).
>
> **ADR-079 (commit `687c9182`) makes this misparse impossible by construction**: CALL/CALLR
> now tag MBC return addresses with bit 31, and RET tests the tag instead of magnitude.
> Verified regression-green on the whole xv6 corpus; the non-ascend (Doom) config compiles.
>
> **Caveat — Doom is currently "PLAYABLE" per `NEXT-STEPS.md` (2026-03-30) and E1M1 is
> completable**, so the 2026-03-29 PC corruption below was evidently worked around after
> this doc was written. That means ADR-079 is EITHER redundant with a later Doom-side fix,
> OR still relevant to residual issues (E1M2 texture corruption). Either way it should not
> regress a playable Doom (the tag is transparent to pure-MBC call/return). **Not yet run
> against Doom** — needs a Marshal-supervised doom-runner pipeline (netns + packet
> injection). **Next Doom session: rebuild the non-ascend object, run doom-runner, confirm
> E1M1 still plays and CPU_MAP PC stays in `[0, 0x3FFFF]`; then check whether E1M2 textures
> improve.** See `docs/adr/ADR-079-ret-address-tagging.md` and
> `references/phase17-gateD-shipped-2026-07-02.md`.

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

---

## Texture Banding Investigation (2026-03-29)

### Observation
Wall/floor textures showed "horizontal colored bands" — wide stripes of uniform
color instead of recognizable texture detail (bricks, metal, etc).

### Investigation
Exhaustive analysis of the complete texture rendering pipeline:
- WAD loading path (doom-runner `bytes_to_words` → RAM_MAP → `read()` → `memcpy` → heap)
- Byte order (LE encoding consistent across `from_le_bytes`, LDB/STB, LDH/STH, LD/ST)
- RV32I→MBC translation (LBU→LDB, SB→STB, SRAI→MOV+SAR, all correct)
- BPF executor memory ops (read-modify-write for byte stores, LE extraction)
- R_DrawColumn inner loop (compiler output verified: LBU for texel read, LBU for colormap, SB for pixel write)
- R_GetColumn texture column selection (columnofs correctly loaded, columns vary per X)
- Bridge screen reading (RAM_MAP word extraction matches Doom's byte store layout)
- Software division (__udivsi3 verified correct for dc_iscale computation)

### Empirical verification (live RAM_MAP reads via bpftool)
1. **Palette**: Dynamic PLAYPAL from RAM_MAP at 0x60000 matches DOOM.WAD byte-for-byte
2. **Screen pixels**: Varied values across columns (not uniform per row)
3. **Status bar / Doomguy face**: Correct skin tones and HUD layout
4. **Adjacent rows at wall region**: Identical pixels span ~33 columns per row

### Root cause: TWO issues found

**Issue 1: Incorrect fallback palette (FIXED, commit 823dde86)**
The hardcoded PALETTE in the bridge HTML (used as JS fallback) had 203 of 256
entries wrong compared to the actual retail DOOM.WAD PLAYPAL. Entries 48-111 were
shifted by entire color ramps — skin tones mapped to fire colors, grey ramp mapped
to red/fire, green mapped to brown/leather. While the dynamic palette path reads
correct values from RAM_MAP, any fallback scenario would display dramatically wrong
colors. Fixed by regenerating both Rust and JS palettes from the actual WAD file.

**Issue 2: Normal Doom nearest-neighbor magnification (NOT A BUG)**
When close to a wall, dc_iscale < 0x10000 causes multiple screen rows to map to
the same texel row (via `(frac >> FRACBITS) & 127`). This produces visible
horizontal bands where each texel row is stretched across 4-12 screen pixels.
This is identical to how original Doom looks on a CRT at 320x200 — the low texture
resolution becomes visible at close range. At appropriate viewing distances, texture
detail is correctly visible.

### Verification: texture rendering IS correct
The pixel data at wall regions shows:
- Different palette indices per column (dc_source varies correctly per texturecolumn)
- Gradual vertical transitions within columns (frac stepping works)
- Correct colormap application (dark lighting compresses palette range as expected)
- HUD and face sprites render correctly (V_DrawPatch path confirmed working)

The "banding" perception was amplified by the incorrect palette mapping 203 colors
to wrong destinations. With the correct palette, the natural Doom texture
magnification should look significantly more recognizable.

---

## Post-Baseline Fixes (2026-03-30)

### Fix 12: Palette 203/256 Entries Wrong (commit 823dde86)

The hardcoded PLAYPAL palette in both the Rust bridge and JavaScript fallback had
203 of 256 entries wrong compared to the actual retail DOOM.WAD PLAYPAL. Entries
48-111 were shifted by entire color ramps -- skin tones mapped to fire colors, grey
ramp mapped to red/fire, green mapped to brown/leather. Regenerated both palettes
from the actual WAD file.

### Fix 13: 8-Slot Keyboard Circular Queue (commit 42bbc34d)

**Problem:** Bridge wrote keyboard events to a single KBD_MAP slot. When rapid
input occurred (keydown + keyup in quick succession), the second event would
overwrite the first before the MBC executor read it. This caused keys to "stick"
(keydown consumed, keyup lost) or to be ignored entirely.

**Fix (bridge.rs):** Changed kbd_writer to use an 8-slot circular queue:
- Maintains a write_head index (0-7)
- On each event, scans from write_head for an empty slot (value == 0)
- If empty slot found, writes there and advances write_head
- If all 8 full, overwrites write_head (drops oldest unread event)

**Fix (i_video_mbc.c):** Changed I_StartTic to poll up to 8 times per tic:
- Loop calls SYS_GET_KEY up to 8 times
- Returns early when SYS_GET_KEY returns 0 (no more events)
- Drains the entire queue each tic instead of reading just one event

### Fix 14: Browser Auto-Repeat Suppression (commit 46f36f77)

**Problem:** Holding a key in the browser generates repeated keydown events
(browser auto-repeat). Each auto-repeat wrote another event to KBD_MAP, flooding
the 8-slot queue and causing erratic behavior (multiple keydown events with no
corresponding keyup, queue full of redundant events).

**Fix (bridge.rs VIEWER_HTML):** Added `if (e.repeat) return;` to the JavaScript
keydown handler. Only the initial press generates an event. The browser's built-in
auto-repeat is suppressed. Doom's own key repeat logic (in the game loop) handles
held keys correctly via the maintained keydown state.

---

## Summary: All Bugs Fixed (23 total)

### doomgeneric era (9 bugs)
1. Map alignment (Aya pipeline)
2. .sdata missing
3. Stack/WAD overlap
4. sscanf no-op
5. mmap path corruption
6. Z_Malloc corrupts WAD handle
7. heap_ptr corruption
8. fclose no-op
9. Heap/WAD overlap

### id DOOM port session (11 bugs)
10. sprintf 32-bit overflow
11. fd read returns -1
12. close() kills WAD fd
13. Soft-float infinite recursion
14. gamemode=commercial (wrong WAD)
15. STCFN033 -> STCFN33
16. HELP2 not found
17. SCREEN_BASE mismatch
18. Debug regions corrupted
19. I_FinishUpdate 16K stores
20. Keyboard JS keyCode mapping

### Post-baseline polish (3 bugs)
21. Palette 203/256 wrong
22. Single-slot KBD overwrite
23. Browser auto-repeat flood

---

## Banding Investigation — Complete Findings (2026-03-31)

### Key Discoveries

**1. R_DrawColumn IS executing — 1.2M CALLR indirect calls confirmed**
STAT counter monitoring confirmed 1.2 million CALLR (indirect call) opcode
executions during gameplay. R_DrawColumn, the inner loop of Doom's wall
renderer, is being called and executing correctly. The rendering pipeline
is not broken.

**2. Full BSP render pipeline runs — ALL markers hit**
The complete rendering call chain executes every frame:
```
D_Display.part.0
  → R_RenderPlayerView
    → R_RenderBSPNode
      → R_Subsector
        → R_AddLine
          → R_RenderSegLoop
            → R_DrawPlanes
            → R_StoreWallRange
```
Every function in the chain was confirmed via STAT markers or breadcrumb
instrumentation. The BSP traversal, wall clipping, plane rendering, and
segment storage all operate correctly.

**3. gamestate=0 (GS_LEVEL) during gameplay**
Earlier observations of `gamestate=3` (GS_DEMOSCREEN) were snapshots taken
during the title screen before a demo started. Once gameplay begins (either
player-initiated or demo playback), gamestate correctly transitions to 0
(GS_LEVEL). The game loop processes tics and calls the full render pipeline.

**4. CALLR opcode (0x2A) works correctly**
The CALLR (call-register) opcode is encoded in the HIGH byte of the MBC
instruction word (bits 24-31), not the low byte. This was initially
suspected as a bug but confirmed correct. The opcode is 0x2A and the
instruction encoding is:
```
[opcode:8][dst:4][src:4][imm16:16]
```
in the u32 value (big-endian format within the instruction word).

**5. Raw pixel indices show repeated texel rows — correct magnification**
Direct RAM_MAP reads of the screen buffer show patterns like 9 consecutive
rows containing palette index 0x4D. This is correct nearest-neighbor
magnification: when the player is close to a wall, `dc_iscale < 0x10000`
causes `(frac >> FRACBITS) & 127` to return the same texel row for
multiple screen rows. Each texel row stretches across 4-12 screen pixels
vertically, producing visible horizontal bands.

**6. XRGB conversion (doomgeneric-style cmap_to_fb) produced wrong colors**
An attempt to use doomgeneric-style colormap-to-framebuffer XRGB conversion
produced incorrect colors. The palette+indices path (where the bridge reads
8-bit palette indices from SCREEN_MAP and converts via the PLAYPAL lookup
in JavaScript/Rust) is the correct rendering path. Reverted.

**7. Vertical JS smoothing filter — blur without fixing banding**
A JavaScript post-processing filter was tested that smoothed vertical pixel
transitions. It produced visible blur without actually resolving the banding
appearance. The filter was reverted as it degraded image quality without
benefit.

**8. CSS bilinear upscale (committed 46f36f77)**
CSS `image-rendering: auto` (bilinear interpolation) applied to the canvas
upscale provides slight visual improvement at zero computational cost. This
was committed as a minor quality-of-life improvement.

### Ruled Out as Causes

The following were exhaustively investigated and confirmed NOT to be the
source of banding:

- **SRA/SRL instructions** — shift-right arithmetic/logical translate correctly
- **FixedMul / FixedDiv** — 32-bit fixed-point math verified correct
- **WAD file reads** — lump data matches retail DOOM.WAD byte-for-byte
- **Gamma correction** — gamma tables present and applied correctly
- **CALLR bugs** — opcode works, 1.2M calls confirmed
- **Translator bugs** — RV32I → MBC translation verified for all relevant opcodes
- **Missing file I/O** — all WAD reads succeed, fd table functional
- **Zone memory** — Z_Malloc operates correctly for rendering allocations

### Root Cause: Authentic Doom Rendering

The banding IS authentic Doom 320x200 nearest-neighbor texture magnification
with 128-pixel textures. The user describes it as "10px horizontal bands of
black/red/white/brown where stone textures should be." This is exactly how
Doom's 8-bit indexed palette renders close-up wall textures at low resolution.

When close to a wall:
- `dc_iscale` drops below `FRACUNIT` (0x10000)
- Multiple screen rows map to the same texel row via `(frac >> 16) & 127`
- Each unique texel value spans 4-12 vertical pixels
- 128-pixel source textures stretched across 200 vertical screen pixels
  produce ~1.5 pixels per texel — visible as distinct color bands

This is identical to original Doom on period hardware at 320x200.

### Additional Fixes and Diagnostics from This Session

**12MB zone memory fixes E1M1 completion**
Increasing zone memory from the default to 12MB allowed E1M1 to complete
without Z_Malloc failures. Doom's zone allocator needs sufficient headroom
for level geometry, texture caches, and rendering temporaries. The 12MB
size is set by doom-runner at load time.

**Dynamic WAD size from doom-runner**
doom-runner now reads the actual WAD file size rather than using a hardcoded
constant. This ensures the memory layout correctly accommodates different
WAD files (shareware vs retail vs Ultimate Doom) without manual adjustment.
The WAD size is passed to the memory layout calculator which positions heap,
WAD, and stack regions dynamically.

**screenblocks=7 default**
The default screen size is `screenblocks=7`, which renders the game view
with a visible status bar and border. This is standard Doom behavior. The
status bar (showing health, ammo, arms, face, armor) renders correctly via
V_DrawPatch, confirming the sprite rendering path works.

**Back buffer rendering**
Doom renders to a back buffer (screens[0] at SCREEN_BASE) and then
I_FinishUpdate copies the completed frame to SCREEN_MAP for the bridge to
read. This double-buffering prevents tearing. The 16K store operations per
frame (320x200 / 4 bytes per store) were confirmed as the I_FinishUpdate
copy loop.

**8-slot keyboard circular queue**
The keyboard input system uses an 8-slot circular queue (Fix 13, commit
42bbc34d). The bridge writes events to the next available slot; the MBC
executor's I_StartTic drains up to 8 events per tic. This prevents the
single-slot overwrite bug where rapid keydown+keyup pairs would lose the
keyup event.

**File open logging diagnostic**
A file-open logging diagnostic was added to trace WAD file access patterns.
This confirmed that all fopen/read/lseek operations correctly route through
the fd table to the memory-mapped WAD region. The diagnostic was used to
verify that W_ReadLump and W_CacheLumpNum read correct lump offsets and
sizes.

## 2026-07-23 — Black screen: stuck before video init (frames all-zero)

**Symptom:** DOOM rendered earlier the same session, then went to a black
screen and did not recover across restarts (including a user force-kill).

**Definitive diagnosis (do not re-litigate):** the bridge WebSocket delivers
frames fine — `64768 B` each (`768` palette + `64000` screen indices), the
exact size `bridge.rs` VIEWER_HTML decodes — but **every byte is zero**:
palette `0/768` non-zero, screen all pixel-index `0`. So the CPU is executing
(injector drove `13.7B+` instructions) yet never runs `I_SetPalette` (palette
would be non-zero) and never writes a pixel. **DOOM is stuck before video
init**, i.e. the compute-side PC-corruption / NOP-loop class of blocker — not a
display bug.

**How to reproduce the check (fast):**
```
python3 -c "import asyncio,websockets,collections;\
async def m():\
 import websockets;\
 ws=await websockets.connect('ws://localhost:16666/ws',max_size=None);\
 f=[await ws.recv() for _ in range(5)][-1];\
 print('pal',sum(1 for b in f[:768] if b),'scr',sum(1 for b in f[768:] if b));\
asyncio.run(m())"
```
Non-zero palette/screen ⇒ rendering. All-zero ⇒ stuck (current state).

**Ruled out this session:**
- NOT the viewer nav links (HTML-only; frame format verified correct).
- NOT stale processes/BPF state — killed all `doom-runner`/`doom-go-injector`,
  `ring teardown`, `rm -rf /sys/fs/bpf/unheaded`, deleted all `monad*` netns,
  no host XDP. Pipeline reloads clean (ROM/CPU/RAM maps PASS, `IWAD` verified).
- NOT the WS/render path (frames reach the browser; `client lagged` logged).
- Render code is byte-identical to when it worked (rebuilds only touched HTML),
  so this is an execution/runtime landing state, not a code regression in
  `bridge.rs`.

**Next steps for the debug session (Marshal-overseen):** PC-trace the executor
to find where it loops instead of reaching `I_InitGraphics`/`I_SetPalette`;
verify `ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf` is the
**non-ascend** DOOM build (an `--features ascend-linux` object overwriting that
path would change interpreter semantics); confirm `./doom/doom.{mbc,elf,rv2mbc}`
integrity. Pipeline invocation + injector are in `docs/doom/RUNBOOK.md` and
`~/tmp/next.md`.

## 2026-07-24 — Black screen root cause: XDP never attached (session startup regression)

**Root cause confirmed:** `doom-runner run` loads the Aya XDP program into the kernel
(monad_cpu, id=276 in this session) but **never attaches it to any interface**. Lines
429-436 of `crates/doom-runner/src/main.rs` explicitly defer attachment with a comment:
"For now, log and continue. The XDP program will be attached after we move to the
namespace." There is no code path that performs this attachment.

When DOOM was killed with `kill PID`, the running process died and with it, the Aya-held
BPF program + maps were released. On restart, a fresh eBPF object was loaded but never
attached → injector packets flowed through veth interfaces without hitting any XDP hook →
CPU_MAP never updated (PC=0, insn_count=0 across all readings) → frames all-zero.

**Fix (manual, each restart):**
```bash
# After doom-runner run is live, attach XDP program by its kernel ID:
PROG_ID=$(sudo bpftool prog list | grep monad_cpu | awk '{print $1}' | tr -d ':')
sudo ip netns exec monad1 bpftool net attach xdp id $PROG_ID dev veth01p
sudo ip netns exec monad0 bpftool net attach xdp id $PROG_ID dev veth10p
```

**Permanent fix needed:** In `crates/doom-runner/src/main.rs` Step 3, replace the
"log and continue" block with:
```
// After program.load(), get the prog FD and attach via nsenter + bpftool
// OR: use Aya's XDP link attach API which supports netns via open_fd
```

The ring teardown (`ring.rs:260`) correctly runs `ip link set veth{prev}{i}p xdp off`
inside each namespace, which proves the topology: XDP must run on `veth01p` (monad1)
and `veth10p` (monad0) — the ingress sides of each hop.

**Verification after attaching:**
- `sudo bpftool map lookup id <CPU_MAP_ID> key 0xde 0x00 0x00 0x00` → PC/insn_count advance
- python3 WS check → palette_nonzero > 0

**State at recovery (2026-07-24):** palette_nonzero=680, screen_nonzero=64000, 19.5K pkt/s,
534B insns/s. DOOM rendering confirmed.

---

## 2026-07-24 — Demo4 Crash: Root Cause + Fix

**Symptom:** DOOM rendered ~5s of frames then halted. PC=7972, halted=1, reason=0x80 (HALT opcode = exit() called).

**Diagnosis:**
- Read RAM_MAP at 0x03200000 (DEBUG_MSG_ADDR): `W_GetNumForName: demo4 not found!`
- DOOM's `D_DoAdvanceDemo()` cycles through demo sequences. For `gamemode=retail` it uses `%7` (sequences 0-6); sequence 6 calls `G_DeferedPlayDemo("demo4")`.
- `libc_stubs.c` `access()` stub was matching `doomu.wad` → `gamemode=retail` → demo loop included case 6.
- `doom1.wad` (shareware) has no DEMO4 lump → `W_GetNumForName` calls `I_Error` → `exit()` → HALT.

**Fix:** Changed `access()` stub to match `doom1.wad` → `gamemode=shareware`. Shareware uses `%6` (sequences 0-5), case 6 (demo4) never reached.

**File:** `demos/doom/libc_stubs.c` — `access()` function, WAD filename match.

**Rebuild required after this change:** `cd demos/doom && make`

**Verification:** After fix, DOOM ran past the 5s mark, insn_count advancing, no STAT_HALTED entry in STATS map.
