# Bug Kill Chain: Doom-over-IPv6 Development

Every major bug encountered during the Doom-over-IPv6 proof of concept, from first packet to D_DoomLoop alive. Each entry documents the symptom, root cause, fix, and lesson learned.

---

## Bug 1: Per-Link /64 Prefix Collision

**Symptom:** Packets injected into monad0 never reached monad1. The XDP program on monad0 executed correctly (instruction count advanced), but `XDP_TX` packets were silently dropped between namespaces.

**Root Cause:** All veth pairs shared the same /64 IPv6 prefix (`fd00:3f:75::${i}:1/64`). The Linux kernel treats all addresses in the same /64 as link-local neighbors. When monad0 tried to forward a packet to monad1, the kernel attempted NDP (Neighbor Discovery Protocol) resolution instead of routing, because both endpoints appeared to be on the same subnet. NDP resolution failed (no real neighbor existed), and packets were dropped.

**Fix:** Each veth pair was assigned a distinct /64 prefix: `fd00:3f:75:${i}::1/64`. This ensures the kernel treats cross-namespace forwarding as a routing decision, not a link-local neighbor lookup.

**Lesson:** In IPv6 networking, every link must have its own /64. Sharing a /64 across multiple links causes routing ambiguity that silently drops packets.

---

## Bug 2: Destination Address Inside Connected Subnet

**Symptom:** Test packets with destination addresses in the connected /64 range were not forwarded through the ring. They triggered NDP instead of default-route forwarding.

**Root Cause:** If the destination IPv6 address falls within a connected /64 subnet, the kernel assumes the destination is a direct neighbor and attempts NDP resolution. For the Doom ring, test packets needed a destination outside all connected subnets.

**Fix:** Test packets use `fd00:dead::1` as the destination address -- a /64 that no namespace claims. The kernel falls back to the default route, which points to the next namespace in the ring.

**Lesson:** When testing packet forwarding through custom topologies, always use a destination address outside all connected prefixes to avoid NDP interception.

---

## Bug 3: Aya-eBPF Legacy Map Reuse Failure

**Symptom:** After loading the XDP program on monad0, attaching the same program to monad1-monad5 resulted in each hop using independent, empty maps. Writes to ROM_MAP on hop 0 were not visible on hop 1.

**Root Cause:** `EbpfLoader::map_pin_path()` in aya-ebpf 0.1.x does not reuse existing pinned legacy maps. Each `load()` call creates new, independent map instances even when the pin path points to existing maps. The API appeared to support reuse but silently allocated fresh maps.

**Fix:** Load the BPF program once on hop 0 using the Aya loader. For hops 1-5, use `bpftool net attach xdpgeneric` to attach the already-loaded program (by `prog_id`) to additional interfaces. All hops share the same program instance and the same pinned maps.

**Lesson:** Never assume BPF loader libraries reuse pinned maps. Verify with `bpftool map dump` that writes on one hop are visible on another. The reliable pattern is: load once, attach everywhere.

---

## Bug 4: access() Returns -1, Doom Halts at exit()

**Symptom:** Doom halted at ROM PC 89788 after 59.8 million instructions. The CPU hit an `ebreak` (RISC-V) -> HALT. The debug output showed "Error: Game mode indeterminate."

**Root Cause:** Doom calls `access()` to check if WAD files exist on disk. The libc stub (`libc_monad.c`) returned -1 for all `access()` calls. IWAD discovery failed, Doom printed an error via `I_Error()`, called `exit()`, which compiled to `ebreak`, which the MBC translator mapped to HALT.

**Fix:** Modified `access()` in `libc_monad.c` to return 0 for paths ending in ".wad". This allows Doom's IWAD discovery to succeed without real filesystem access.

**Lesson:** Every libc stub must be evaluated for its impact on the application's startup sequence. A single "not implemented" return value can prevent the application from reaching its main loop.

---

## Bug 5: RAM_MAP HashMap Silent Write Drops (The HashMap Catastrophe)

**Symptom:** CALLR fault at approximately 99.4 million instructions. The CPU attempted an indirect function call (CALLR) to a bogus MBC address translated from a corrupted RISC-V function pointer. The RISC-V address was ASCII garbage ("LMNO" = 0x4F4E4D4C), indicating corrupted data in the jump table region of RAM.

**Root Cause:** `RAM_MAP` was implemented as a BPF HashMap with 8 million entries (671 MB). BPF HashMaps have a maximum entry limit. When the map filled up, `bpf_map_update_elem()` returned an error code, but the XDP program did not check the return value. Writes were silently dropped. Subsequent reads returned stale or zero values for addresses that should have been written, corrupting function pointers, jump tables, and other critical data structures.

**Fix:** Replaced `RAM_MAP` with BPF Array type (16 million entries, 128 MB). BPF Arrays pre-allocate all entries at creation time -- every index is valid, writes never fail. Committed as 3a1bbe7 in sprint S31.

**Lesson:** Never use BPF HashMap for memory that must be 100% reliable. HashMap has a capacity limit, and writes beyond that limit are silently dropped. BPF Array is the correct choice for virtual memory implementations.

---

## Bug 6: I_Error Calls exit() -- Fatal Error Loop

**Symptom:** After fixing the HashMap issue, Doom still halted periodically when encountering non-fatal error conditions (missing lumps, sprite duplicates, zone ID mismatches).

**Root Cause:** Doom's `I_Error()` function calls `exit()`. In a normal environment, this terminates the process. In the MBC VM, `exit()` -> `ebreak` -> HALT, stopping all computation permanently. Many conditions that trigger `I_Error()` are recoverable (missing shareware content, duplicate sprite names).

**Fix:** Modified `I_Error()` in `libc_monad.c` to print the error message to the debug buffer and return, instead of calling `exit()`. This makes all error paths non-fatal.

**Lesson:** When porting software to constrained environments, exit points must be eliminated or converted to non-fatal paths. A halt is permanent in a VM that has no process restart mechanism.

---

## Bug 7: Z_Free / Z_ChangeTag Zone Corruption Cascade

**Symptom:** After extended execution, Doom's zone memory allocator would encounter blocks with invalid ZONEID markers. `Z_Free()` would call `I_Error()` (originally fatal), creating a cascade of errors.

**Root Cause:** The zone allocator uses magic numbers (ZONEID) to validate heap blocks. Under the MBC VM's constrained memory and the shareware WAD's edge cases, some zone blocks became corrupted (likely due to out-of-bounds writes from rendering routines). When `Z_Free()` or `Z_ChangeTag()` encountered these corrupted blocks, they triggered the error cascade.

**Fix:** Added early-return guards to `Z_Free()` and `Z_ChangeTag()` (in `z_zone.c`): if the ZONEID check fails, return immediately instead of triggering an error. Additionally, `Z_Malloc()` returns NULL on allocation failure instead of entering an infinite retry loop.

**Lesson:** Memory allocators in constrained VMs must be defensive. Assume corruption will happen and fail gracefully rather than cascading.

---

## Bug 8: Rendering Bounds Check Violations

**Symptom:** Doom rendered garbled output or wrote pixels outside the 320x200 framebuffer, corrupting adjacent memory.

**Root Cause:** Multiple rendering functions (`R_DrawColumn`, `R_DrawSpan`, `R_DrawFuzz`, `V_DrawPatch`, `V_CopyRect`, `V_DrawBlock`, `R_MapPlane`, `R_FindPlane`, `R_DrawPlanes`) performed bounds checks using `RANGECHECK` macros that called `I_Error()` on violation. In a normal environment, the check would terminate the program. In the MBC VM, these checks needed to prevent the write without halting.

**Fix:** Replaced `RANGECHECK` error calls with `#ifdef __MONAD__` early-return guards in all rendering functions. On bounds violation, the function returns immediately without writing pixels. This prevents both out-of-bounds writes and error cascades.

**Lesson:** Every bounds check in ported code must be converted from "detect and crash" to "detect and skip." The VM cannot afford to halt on rendering edge cases.

---

## Bug 9: R_InstallSpriteLump Duplicate Warnings

**Symptom:** Console output flooded with "R_InstallSpriteLump: sprite TROO duplicate lump" warnings, slowing execution.

**Root Cause:** The shareware WAD (doom1.wad) contains duplicate sprite frame entries for the TROO (imp) enemy. Doom's sprite installer treats duplicates as an error condition.

**Fix:** Changed `I_Error()` calls in `R_InstallSpriteLump()` (in `r_things.c`) to `printf()` for duplicate sprite warnings. Duplicates are logged but do not trigger the error path.

**Lesson:** Shareware WADs have known content issues. Port code must tolerate them without error escalation.

---

## Bug 10: Virtual Time Catch-Up Spiral

**Symptom:** After Doom's main loop started, the game attempted to simulate hundreds of game tics per frame, consuming instructions without producing visible output.

**Root Cause:** `DG_GetTicksMs()` returned wall-clock time. The MBC VM executes far slower than real time (~6 fps at 333 pps injection). When Doom checked elapsed time, it saw minutes had passed since the last frame, and attempted to simulate all missed tics in a burst. This consumed millions of instructions on game logic with no rendering.

**Fix:** `DG_GetTicksMs()` now returns a static incrementing counter (`static ticks++`). `DG_SleepMs()` is a no-op. Virtual time advances proportionally to instructions executed, not wall-clock time.

**Lesson:** Games assume real-time execution. When running in a VM with variable-speed execution, virtual time must replace wall-clock time.

---

## Bug 11: G_DoPlayDemo NULL Pointer

**Symptom:** Crash when Doom attempted to play back recorded demo lumps that do not exist in the shareware WAD.

**Root Cause:** The shareware WAD does not include all demo lumps referenced by the demo cycle code. `W_CacheLumpName()` returned NULL for missing lumps, and `G_DoPlayDemo()` dereferenced the NULL pointer.

**Fix:** Added NULL check to `G_DoPlayDemo()` (in `g_game.c`): if the demo lump is NULL, return early and skip to the next screen in the cycle.

**Lesson:** Demo playback is optional content. Always NULL-check lump cache results before dereferencing.

---

## Bug 12: Stale doom_data.bin After ELF Changes

**Symptom:** After modifying Doom's C source (adding libc stubs, hardening patches) and recompiling, JMPR faults occurred at locations that previously worked. The faulting RISC-V address was ASCII garbage (e.g., "LMNO" = 0x4F4E4D4C).

**Root Cause:** `doom-loader.sh` only regenerates `doom_data.bin` if the file is missing (`if [[ ! -f ]]`). When the ELF changes, code additions shift `.rodata` VMA (virtual memory addresses). The stale `doom_data.bin` has `.rodata` data at the old offsets, so jump tables and string pointers resolve to garbage.

**Fix:** Always `rm -f doom_data.bin` before running `doom-loader.sh all`. This forces regeneration from the current ELF.

**Lesson:** Any build cache that depends on ELF layout must be invalidated when the ELF changes. Guard against stale artifacts by always regenerating from source.

---

## Bug 13: Keyboard Input Key Code Mismatch

**Symptom:** Keyboard input from the browser did not produce the expected actions in Doom. Arrow keys, Enter, and Escape were not recognized.

**Root Cause:** The doom-bridge service translated browser JavaScript key codes to Doom scan codes, but the mapping used incorrect Doom key constants. Doom uses its own internal key codes (defined in `doomkeys.h`), which differ from both JavaScript `event.keyCode` and standard PC scan codes.

**Fix:** Corrected the key code translation table in the doom-bridge service and the browser-side JavaScript. Added a clear-after-read pattern to KBD_MAP: after the BPF program reads a key event, it clears the entry, preventing stale key-down events from repeating indefinitely.

**Lesson:** Key code translation between browser, service, and game engine requires three separate mapping tables. Test each key individually and verify the Doom-side key constants match the source header.

---

## Bug 14: CALL/RET Stack Pointer Address Confusion

**Symptom:** CALL instructions corrupted the return address stack, and RET jumped to invalid PC values.

**Root Cause:** CALL/RET instructions treated the stack pointer (r15) as a word address directly: `cpu.regs[15].wrapping_sub(1)` then `mem_write_word(sp, pc)`. However, the CRT0 startup code set SP to 0x1000000 (a byte address from RISC-V conventions), and the MBC interpreter used it as a word index. This meant the stack pointer referenced word addresses up to ~16 million, requiring the Array map to have at least 16 million entries.

**Fix:** Ensured RAM_MAP Array has 16 million entries to accommodate the full stack pointer range. The MBC interpreter correctly handles SP as a word index into the array.

**Lesson:** When translating between RISC-V byte-addressed memory and MBC word-addressed memory, ensure the address space is large enough for the maximum stack pointer value.

---

## Bug 15: RV2MBC Address Translation for Indirect Jumps

**Symptom:** JMPR (indirect jump) and CALLR (indirect call) instructions halted the CPU because the target MBC PC was not found in `RV2MBC_MAP`.

**Root Cause:** Function pointers in Doom's `.data` section contain RISC-V byte addresses. The MBC translator outputs MBC instruction indices. JMPR/CALLR must look up `RV2MBC_MAP[rv_addr >> 2]` to translate the RISC-V byte address to the corresponding MBC PC index. Without this translation, indirect jumps landed in wrong ROM locations.

**Fix:** The MBC interpreter performs `RV2MBC_MAP` lookup for all indirect jump/call targets. If the lookup fails (key not found), the CPU halts with a ROM fault rather than executing at a garbage address.

**Lesson:** Cross-compilation to a custom ISA requires a bidirectional address mapping. Every indirect control flow transfer must go through the translation table.

---

## Summary

| # | Bug | Severity | Instructions to Manifest | Fix Complexity |
|---|-----|----------|-------------------------|----------------|
| 1 | /64 prefix collision | Critical | 0 (no packets flow) | Config change |
| 2 | Destination in connected subnet | High | 0 (no packets flow) | Config change |
| 3 | Aya legacy map reuse | Critical | 0 (maps not shared) | Architecture change |
| 4 | access() returns -1 | High | 59,800,000 | One-line libc stub |
| 5 | HashMap silent drops | Critical | 99,400,000 | Map type change (Array) |
| 6 | I_Error calls exit() | High | Variable | Non-fatal error path |
| 7 | Z_Free zone cascade | Medium | Variable | Early-return guards |
| 8 | Rendering bounds | Medium | Variable | Bounds-check returns |
| 9 | Sprite duplicate warnings | Low | Variable | Downgrade to printf |
| 10 | Virtual time spiral | High | Variable | Static tick counter |
| 11 | G_DoPlayDemo NULL | Medium | Variable | NULL check |
| 12 | Stale doom_data.bin | High | Variable | Force regeneration |
| 13 | Key code mismatch | Medium | N/A (input) | Correct mapping table |
| 14 | SP address confusion | High | Variable | Array size increase |
| 15 | RV2MBC translation | Critical | Variable | Translation table lookup |

---

*See also: [Doom over IPv6](doom-over-ipv6.md) | [Performance](performance.md) | [Architecture](architecture.md)*
