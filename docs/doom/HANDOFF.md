# Doom Banding Fix — Session Handoff

**Date:** 2026-03-31 19:30 UTC
**Status:** PU_CACHE HYPOTHESIS DISPROVEN — all PU_STATIC fixes cause zone exhaustion or crash
**Priority:** P0 — this is THE remaining visual bug

## The Bug

Wall textures show horizontal multicolor banding: alternating bands of black, dark red, white, brown where stone/brick textures should be. Affects pillars, some walls, worse in later rooms.

## Root Cause (CONFIRMED)

**Texture composite data is zeroed.** Raw pixel dump during GS_LEVEL gameplay shows:

```
y=32: idx=8, y=33: idx=2, y=34: idx=239, y=35: idx=5, y=36: idx=1...
```

These are palette indices 1-8 (near-black, first few palette entries) with occasional 239 (bright orange). Normal stone textures should have indices 64-95 (brown/grey gradients). The data in `dc_source` is **garbage from zeroed memory**.

**`texturecomposite[0..9]` are ALL NULL** (confirmed via bpftool RAM_MAP read at 0x203AF4). This means `R_GenerateComposite` either never runs or fails to populate the composite texture data.

## What We Proved Works

| Component | Status | Evidence |
|-----------|--------|---------|
| R_DrawColumn | ✅ Executes | 1.2M CALLR calls via STAT counter |
| BSP pipeline | ✅ Full chain | All markers hit: D_Display→R_RenderPV→BSP→Subsector→AddLine→SegLoop |
| CALLR opcode | ✅ Correct | MBC encoding verified (opcode in HIGH byte, bits 24-31) |
| SRA/SRL | ✅ Correct | Separate opcodes, sign extension verified |
| FixedMul/FixedDiv | ✅ Correct | 64-bit multiply preserves precision |
| Palette | ✅ Correct | Dynamic palette from WAD PLAYPAL |
| WAD reads | ✅ Correct | No missing file accesses |
| gamestate | ✅ GS_LEVEL | Transitions correctly during demo/gameplay |

## What's Broken

**`texturecomposite` array entries are NULL.** In `R_GetColumn` (r_data.c), when `texturecolumnlump[tex][col] <= 0`, it uses `texturecomposite[tex]`. If that's NULL, it calls `R_GenerateComposite(tex)`. But the composites are STILL null after init, meaning:

1. `R_GenerateComposite` may not be called (code path skipped)
2. OR it's called but `Z_Malloc` returns memory that gets zeroed/purged
3. OR the composite is built but the pointer is overwritten

## Next Steps (in order)

### Step 1: Check if R_GenerateComposite ever executes
Add `debug_breadcrumb(0xGC01)` at the start of R_GenerateComposite (r_data.c). If it never fires, the lump path (texturecolumnlump > 0) is always taken, and the bug is in W_CacheLumpNum returning zeroed data.

### Step 2: Check W_CacheLumpNum return values
The lump path calls `W_CacheLumpNum(lump, PU_CACHE)`. PU_CACHE means the zone allocator can PURGE this memory when it needs space. If zone memory is tight (12MB), cached lumps get purged and the pointers become dangling. When R_GetColumn returns `lump_data + offset`, the lump_data may have been purged.

**This is the most likely cause: PU_CACHE zone memory purging.**

### Step 3: Fix — increase PU_CACHE retention or use PU_STATIC
In r_data.c, change `W_CacheLumpNum(lump, PU_CACHE)` to `W_CacheLumpNum(lump, PU_STATIC)` for texture lumps. This prevents purging at the cost of more memory. With 12MB zone this should be feasible for E1M1.

OR increase zone memory from 12MB to 16-20MB (previous attempts at 14-20MB had stability issues, but those may have been from stale processes not the zone size itself).

## Key Files

| File | What | Where |
|------|------|-------|
| `linuxdoom-1.10/r_data.c` | R_GetColumn, R_GenerateComposite, texture loading | Source of truth for texture pipeline |
| `linuxdoom-1.10/r_draw.c` | R_DrawColumn inner loop | Uses dc_source (fed by R_GetColumn) |
| `linuxdoom-1.10/r_segs.c` | R_RenderSegLoop | Calls R_GetColumn then colfunc() |
| `linuxdoom-1.10/w_wad.c` | W_CacheLumpNum | Zone-cached WAD lump reading |
| `linuxdoom-1.10/z_zone.c` | Z_Malloc, Z_FreeTags | Zone memory allocator |
| `demos/doom/i_system_mbc.c` | `mb_used = 12` | Zone memory size (line 67) |
| `demos/doom/libc_stubs.c` | sbrk, malloc, WAD read stubs | Memory allocation |
| `crates/doom-runner/src/memory.rs` | Memory layout constants | HEAP_START/END, WAD_BASE |

## Current Build State

- Doom: palette+indices path (no XRGB), back buffer, screenblocks=7
- Bridge: reads palette (768 bytes) + pixels (64000 bytes) from RAM_MAP
- eBPF: has diagnostic counters (STAT 20/21 for CALLR, render chain markers at 0xE3000)
- Zone: 12MB (`mb_used = 12`)
- WAD: dynamic size from doom-runner

## How to Run

```bash
cd ~/tmp/unheaded
# Build everything
cd demos/doom && make clean && make && cp doom.elf doom.mbc doom.rv2mbc ../../doom/ && cd ../..
cd ebpf && cargo build --target=bpfel-unknown-none -Z build-std=core -p monad-cpu-ebpf --release && cd ..
cd crates/doom-runner && cargo build --release && cd ../..

# Launch
sudo ./crates/doom-runner/target/release/doom-runner run \
  --doom-mbc doom/doom.mbc --rv2mbc doom/doom.rv2mbc --doom-elf doom/doom.elf \
  --wad ~/tmp/projects/doom-related/DOOM.WAD --hops 2 &
sleep 8

# Attach XDP + inject
PROG_ID=$(sudo bpftool prog list | grep monad_cpu | tail -1 | awk '{print $1}' | tr -d ':')
sudo ip netns exec monad1 bpftool net attach xdpgeneric id $PROG_ID dev veth01p
sudo ip netns exec monad0 bpftool net attach xdpgeneric id $PROG_ID dev veth10p
SRC_MAC=$(sudo ip netns exec monad0 ip link show veth01 | awk '/ether/ {print $2}')
DST_MAC=$(sudo ip netns exec monad1 ip link show veth01p | awk '/ether/ {print $2}')
sudo nsenter --net=/var/run/netns/monad0 ./bin/doom-go-injector \
  --src-mac "$SRC_MAC" --dst-mac "$DST_MAC" --iface veth01 \
  --count 0 --mode sendmmsg --batch 200 &

# Play at http://192.168.69.184:16666
```

## PU_CACHE Hypothesis — DISPROVEN (2026-03-31 19:30 UTC)

All PU_CACHE→PU_STATIC fixes were tested systematically (one at a time):

| Fix | Location | Result |
|-----|----------|--------|
| A (R_GetColumn line 395) | PU_CACHE→PU_STATIC | PC corruption at ~800B insns (zone exhaustion) |
| B (R_GenerateComposite line 288) | Disable Z_ChangeTag | Banding persists (tested with A) |
| C (zone 12→13-16MB) | i_system_mbc.c | Game won't load — init loop/HALT |
| D (R_GenerateLookup line 331) | PU_CACHE→PU_STATIC | I_Error at 8.7M insns — zone exhaustion |
| A+B together | Both runtime fixes | Banding persists |

**Conclusion:** 12MB zone CANNOT hold all textures as PU_STATIC. Zone CANNOT be increased past 12MB (causes init failure). PU_CACHE purging is NOT the root cause — it's a red herring.

## New Hypotheses (ordered by likelihood)

### H1: MBC executor CPU state is corrupt / not syncing to CPU_MAP
CPU_MAP reads show: PC=51,379,584 (way beyond 86K ROM), halted=48 (should be 0/1), insn_count is astronomically wrong. Yet the game renders and is playable. The BPF executor likely uses a local copy of CPU state per-packet that doesn't sync back to CPU_MAP properly, or the CPU_MAP key 0xDE doesn't match the active instance.

**UPDATE (Phase 3 result): RAM_MAP writes SUCCEED — `get_ptr_mut` never returns None.**

STAT counters added to `mem_write_word`:
- STAT[22] (RAM_WRITE_FAIL) = 0 — **zero failures**
- STAT[23] (RAM_WRITE_OK) = 10,540,368 — all writes succeed

The writes go through. But bpftool reads of .bss addresses (0x490B3, 0x5A745) show zeros. Heap (0x70000) and stack (0xCFF40) show non-zero data. This means .bss values genuinely ARE zero at those addresses — the game operates with zeroed .bss globals.

**The banding is NOT caused by failed writes.** The texture data path (WAD reads, memcpy, column rendering) is the remaining suspect — possibly byte-level corruption during MBC execution of texture loading/rendering code.

**Native build test (2026-03-31 22:00 UTC):**
- Built clean id-Software/DOOM linuxdoom-1.10 natively (x86-64). Crashes at R_InitTextures with out-of-bounds patchlookup + missing patches (BROWN1, COMPUTE1, DOORBLU, GRAY5, REDWALL1, etc.).
- Stock linuxdoom-1.10 has a known compatibility issue with retail/Steam DOOM.WAD — patch indices exceed PNAMES array bounds.
- Applied bounds check + fallback (patch 0) to MBC fork r_data.c. Banding PERSISTS — the missing patches are a real bug but not the banding root cause.
- chocolate-doom (installed, `chocolate-doom -iwad DOOM.WAD`) renders the same WAD correctly — confirms the WAD itself is fine.
- **Conclusion:** Banding is DEFINITIVELY an MBC execution artifact. Both retail AND shareware WADs show banding on MBC. chocolate-doom renders retail WAD correctly natively. The Doom C source is not the issue.
- Verified maptexture_t struct layout matches WAD format (masked=4 bytes, width@12, height@14). No struct packing mismatch.
- chocolate-doom's PACKED_STRUCT is a safety measure, not a bug fix for this specific issue.
- alloca(256) max texture width — safe, no stack overflow.

**DEFINITIVE: Banding is an MBC executor bug.** Doom C source, WAD format, struct packing all verified correct. Focus on MBC byte-level operations:
- LBU opcode handler (main.rs:773): `(word >> byte_shift) & 0xFF` — verify byte extraction
- STB opcode handler (main.rs:786): `mem_write_byte` — verify byte insertion  
- memcpy word-copy path (libc_stubs.c:155): LD/ST on MBC for 4-byte copies
- R_DrawColumn inner loop compiles to LBU for `dc_source[(frac>>16)&127]` — trace actual byte values

## Screen Buffer Analysis (2026-03-31 ~23:00 UTC)

Vertical strip at x=100 shows clear banding pattern:
- y=20-27: idx 109-110 (tan/brown) — CORRECT texture pixels
- y=28-36: idx 5 (near-black) — BAND (post header data, not pixels)
- y=37-43: idx 111 (tan/brown) — CORRECT
- y=44-55: idx 239 (bright orange, 0xEF) — BAND
- y=56-79: idx 1,2,7,238,239 — BAND (post header/terminator bytes)

Pattern: ~7-12 correct rows, then bands of post header bytes (0, 1, 2, 5 = topdelta/length, 238/239 = near-terminator). The `+3` offset skips the first post header correctly, but the `& 127` mask wraps frac values into post data territory prematurely.

**But frac math shows this shouldn't happen.** For the captured snapshot: frac starts at ~2.14, increments by 3.69/row. At y=28 (8 rows in), frac ≈ 31 — well within the 72-pixel column. Yet we see garbage at that point.

**Possible cause:** FixedMul (`__mulsi3`) in the initial frac computation returns wrong result, OR dc_iscale/dc_texturemid values are stale/wrong when captured by bpftool (timing issue — values change every column).

## ROOT CAUSE FOUND (2026-03-31 ~23:30 UTC)

**`texturecolumnlump[]` arrays contain GARBAGE lump numbers for some textures.**

Verified via bpftool RAM_MAP reads:
- STARTAN3 (texture 70): col[0..7] = lump 1911 (SW19_2) ← CORRECT
- BROWN96 (texture 16): col[0] = lump 24 (SSECTORS!), col[1] = 0 (PLAYPAL!), col[6] = 18961 (OOB!) ← GARBAGE
- BROWNGRN (texture 17): same garbage pattern as BROWN96

These garbage lump numbers cause R_GetColumn to load data from non-texture lumps (PLAYPAL, SSECTORS, COLORMAP, map lumps), which contain non-pixel data → multicolor horizontal bands.

**The corruption occurs during R_InitTextures → R_GenerateLookup.** The texturecolumnlump arrays are Z_Malloc'd to heap, then populated with lump numbers. Some arrays get overwritten by subsequent Z_Malloc allocations (zone memory overlap/corruption).

**CONFIRMED: Z_Malloc corruption is a KNOWN issue on MBC.** File `demos/doom/w_file_stdc.c` line 6 documents: "On MBC, Z_Malloc'd memory gets corrupted by zone management operations, destroying the file_class pointer and causing W_ReadLump to fail." Previous developers already worked around this for WAD file handles by using static storage.

Zone block header for texturecolumnlump[16] shows `size=24` instead of expected 280 — confirming the allocation was corrupted (either allocated wrong or header overwritten).

**Root cause is in z_zone.c running on MBC executor.** Likely: pointer arithmetic bug, block splitting error, or software multiply (__mulsi3) producing wrong results for zone size calculations. The zone allocator's linked list operations may have subtle bugs under MBC's 32-bit execution model.

## MAJOR FIX: malloc bypass for texture initialization (2026-03-31 ~24:00 UTC)

**Fix applied:** Replaced ALL `Z_Malloc(..., PU_STATIC, ...)` calls in R_InitTextures and R_GenerateComposite with `malloc()` (sbrk bump allocator). The bump allocator is immune to zone corruption because it only advances a pointer — no block headers, no linked lists, no free-list management.

**Result:** Significant banding reduction across E1M1-E1M3. User reports:
- "Big improvement" 
- Banding on pillars in first room remains (likely W_CacheLumpNum PU_CACHE path)
- Second room banding "reduced significantly"
- Third room banding also reduced

**Remaining banding source:** `W_CacheLumpNum(lump, PU_CACHE)` — the runtime lump cache still uses Z_Malloc. Every call to R_GetColumn → W_CacheLumpNum allocates zone memory for the cached lump copy. These PU_CACHE allocations can't simply be replaced with malloc because they need to be purgeable (memory management). 

**Possible next fix for remaining banding:**
1. Read texture data directly from WAD_BASE in RAM_MAP (skip W_CacheLumpNum entirely)
2. Implement a simple LRU cache with malloc instead of zone
3. Fix Z_Malloc itself (debug the zone block corruption root cause in MBC execution)

## Why Z_Malloc corrupts on MBC

Z_Malloc (z_zone.c) uses a linked list of `memblock_t` headers embedded in a contiguous memory region. Operations include:
- Block splitting (when a free block is larger than needed)
- Pointer chain updates (next/prev linked list)
- Tag-based purging (scan and free PU_CACHE blocks during allocation)

On MBC, these operations involve pointer arithmetic compiled to RV32I ADD/SUB/LD/ST instructions translated to MBC opcodes. The zone corruption manifests as:
- Wrong block sizes in headers (e.g., size=24 for a 256-byte allocation)
- Garbage pointer chain values
- Previously documented: WAD file handle corruption (w_file_stdc.c)

The exact MBC opcode or instruction sequence causing the corruption is not yet identified. Candidates: software multiply (__mulsi3) in size calculations, pointer arithmetic wrapping, or linked list traversal bugs in the MBC executor.

## Native Build & chocolate-doom Comparison (2026-03-31)

### Native id Software Build
- Cloned fresh from github.com/id-Software/DOOM (no MBC modifications)
- Applied 3 minimal compatibility fixes for modern GCC:
  - `errnos.h` → `errno.h` (i_video.c)
  - `extern int errno` → commented out, use errno.h (i_sound.c)
  - `int defaultvalue` → `intptr_t defaultvalue` in m_misc.c (pointer-to-int casts)
- Binary: `~/tmp/projects/DOOM-clean/linuxdoom-1.10/linux/linuxxdoom`

**Result:** Crashes at R_InitTextures with out-of-bounds `patchlookup[]` access, then (after bounds check fix) `I_Error: Missing patch in texture BROWN1`. Stock linuxdoom-1.10 has a known compatibility issue with retail/Steam DOOM.WAD — dozens of textures report missing patches: BROWN1, COMPUTE1, DOORBLU, DOORYEL, GRAY5, LITE4, REDWALL1, SW2STON2, etc.

**Root cause of native crash:** `patchlookup` is allocated via `alloca(nummappatches)` on the stack. `SHORT(mpatch->patch)` can exceed `nummappatches` for certain textures in the retail WAD, causing out-of-bounds array read → segfault. After adding bounds check + fallback (`patch->patch = 0`), the game progresses past R_Init but still shows "Missing patch" warnings for many textures.

**Applied bounds check + fallback to MBC fork** at `r_data.c:541` — banding PERSISTS, confirming missing patches are a separate issue from the banding bug.

### chocolate-doom Comparison
- Source cloned to `~/tmp/projects/chocolate-doom/`
- chocolate-doom is a vanilla-compatible Doom port that renders the same WAD correctly
- Key differences found in texture pipeline:

**1. Struct packing (r_data.c maptexture_t):**
- Stock id: `boolean masked` (enum, compiler-dependent size) + natural alignment
- chocolate-doom: `PACKED_STRUCT` with `int masked` + explicit `int obsolete` field
- **Verified NOT the banding cause:** WAD binary format has masked as 4 bytes. Our C struct layout (boolean=4 bytes on RV32I ILP32) matches the WAD format exactly. Offsets verified: width@12, height@14, patchcount@20 — all correct.

**2. alloca → Z_Malloc in R_GenerateLookup:**
- Stock id: `patchcount = (byte *)alloca(texture->width)` (stack allocation)
- chocolate-doom: `patchcount = Z_Malloc(texture->width, PU_STATIC, &patchcount)` (zone allocation)
- **Verified NOT the banding cause:** Max texture width is 256 bytes (texture COMP2). alloca(256) is safe on MBC stack.

**3. Translation table alignment hack (r_draw.c):**
- Stock id: `translationtables = (byte *)(((int)translationtables + 255) & ~255)` — manual 256-byte alignment with memory leak
- chocolate-doom: `translationtables = Z_Malloc(256*3, PU_STATIC, 0)` — no alignment hack
- Not yet tested as banding cause.

**4. Missing patch handling:**
- Stock id: `I_Error("Missing patch in texture %s")` — fatal crash
- chocolate-doom: graceful fallback, continues with available patches
- Applied bounds check to MBC fork — does not fix banding.

### WAD Data Verification
- WAD data in RAM_MAP matches WAD file byte-for-byte (verified DOOR2_1 header + column data)
- Patch pixel data correct: palette indices 0x6B-0x8B (brown/grey range) present in RAM_MAP
- maptexture_t struct layout verified against WAD binary: all field offsets correct
- PNAMES has 351 entries, all referenced patches (WALL02_2, DOOR2_1, etc.) exist in WAD directory

### Definitive Conclusion
- chocolate-doom renders retail DOOM.WAD correctly on native x86
- Both retail AND shareware WADs show banding on MBC
- Stock linuxdoom-1.10 source has compatibility issues (missing patches, patchlookup OOB) but these are separate from banding
- **Banding is definitively an MBC execution artifact** — the Doom C source, WAD format, struct packing, and loaded data are all correct

**Evidence:**
- `diag_buf` (global at 0x1242CC in .bss) was manually set to 0xDDCCBBAA via bpftool, then the game ran for minutes — value never changed. R_GetColumn's stores to diag_buf never reach RAM_MAP.
- Executor render markers at 0xE3000 (below RAM_BASE) also mostly empty.
- Screen buffer at 0x70000 DOES have valid pixel data — writes to SCREEN_MAP work.
- WAD magic at 0x1C00000 is correct ("IWAD") — doom-runner writes work.
- .data at 0x100000 is readable and correct.

### H2: MBC executor byte/word access bug
LD opcode (32-bit word load) at `ebpf/monad-cpu-ebpf/src/main.rs:753` truncates `addr & 3` — silently loads wrong word for misaligned addresses. If GCC emits misaligned LD for texture structs, data corruption follows.

### H3: memcpy word-copy on MBC
`demos/doom/libc_stubs.c:150-166` — word-aligned fast path compiles to LD/ST. If MBC LD/ST has byte-ordering issues, memcpy corrupts texture data.

### H4: WAD lump read corruption
W_ReadLump uses lseek+read. If fd position tracking in libc_stubs.c drifts, lumps contain wrong bytes.

## Diagnostic Approach for Next Session

Since .bss writes don't reach RAM_MAP, use one of these alternative approaches:
1. **Write diagnostics to SCREEN_MAP** — byte stores (STB) to screen addresses DO work. Write texture data bytes to a known screen location.
2. **Write diagnostics to 0xE3000+ region** via executor-side code (the markers work for hardcoded PCs).
3. **Modify the MBC executor** in Rust to log texture data when specific MBC PCs execute (like the existing render markers).
4. **Read texture data directly from WAD in RAM_MAP** — compare expected lump bytes against what the MBC code would read, to verify WAD data is correct in memory.

## Session Stats

- ~30 doom-runner restarts
- 100+ commits on main branch
- Investigations: SRA, FixedMul, FixedDiv, gamma, XRGB, resolution, dithering, file I/O, WAD_MAX_SIZE, zone memory, CALLR opcode, render pipeline markers, gamestate, texture composites, PU_CACHE (disproven)
- Key breakthrough: PU_CACHE hypothesis DISPROVEN — all PU_STATIC fixes cause zone exhaustion at 12MB
- Next: investigate MBC executor memory access (H1) or add diagnostic breadcrumbs to trace actual texture data values

## THE PARADOX (2026-04-01)

| Build | Banding | Stability |
|---|---|---|
| Baseline (original z_zone, no workarounds) | Heavy banding | Stable |
| Original z_zone + r_data.c malloc | **No banding** | Crashes room 2-3 |
| Simplified z_zone (pure malloc) + r_data.c malloc | **Banding returns** | Stable |
| Original z_zone + patch pre-cache | No banding | Crashes (heap pressure) |

**The original zone allocator is REQUIRED for correct rendering.** Replacing it with pure malloc reintroduces banding even with r_data.c workarounds. Something about the zone's memory layout (12MB contiguous block, block headers, address patterns) is needed for W_CacheLumpNum data integrity.

But the original zone CRASHES during gameplay from linked-list corruption.

**Best build so far:** Original z_zone + r_data.c malloc workarounds. No banding, crashes in rooms 2-3. The r_data.c workarounds reduce zone pressure, extending stability but not eliminating crashes.

**Next approach:** Keep original z_zone.c. Make Z_Malloc a bump-only allocator within the zone (allocate sequentially, never split/merge/purge blocks). This preserves the zone's memory layout while eliminating the linked-list operations that corrupt.
