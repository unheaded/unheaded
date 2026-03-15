# Level 3 Optimizations — Doom-over-IPv6 Framebuffer Pipeline (35fps target)

**Date:** 2026-03-15
**Sprint:** Level 3 Performance

---

## Problem Statement

The Doom-over-IPv6 framebuffer pipeline had a critical bottleneck: the
`copy_fb_to_screen()` function in the eBPF program was a **no-op** because the
BPF verifier cannot handle the 16,000-iteration loop required to copy 64,000
bytes (320x200 pixels) from RAM_MAP to SCREEN_MAP.

Individual pixel writes via `mem_write_byte()` (STB opcode) did update
SCREEN_MAP one pixel at a time during rendering, but the bulk framebuffer copy
at `SYS_DRAW_FRAME` time never happened. This meant SCREEN_MAP only reflected
pixels that were individually written, not the full composed frame.

## Changes Implemented

### 1. Userspace Framebuffer Copy (RAM_MAP -> SCREEN_MAP)

**Files:** `ebpf/monad-cpu-ebpf/src/main.rs`, `cmd/doom-bridge/main.go`, `cmd/doom-bridge/bpf.go`

**Strategy:** Move the framebuffer copy from eBPF (where the verifier blocks it)
to userspace (doom-bridge), where there are no iteration limits.

**eBPF side:**
- Added `STAT_FRAME_READY` (key 11) counter to the STATS map
- `SYS_DRAW_FRAME` now increments this counter on every call
- Userspace can poll this counter to detect when a new frame is ready

**doom-bridge side:**
- Opens RAM_MAP in addition to existing maps
- On each screen poll (~60fps), checks STATS[11] for changes
- When the frame-ready counter changes, calls `copyRamToScreen()` which:
  - Reads 16,000 u32 words from RAM_MAP starting at SCREEN_BASE (0x70000)
  - Unpacks each word into 4 pixels (little-endian byte order)
  - Writes each pixel to SCREEN_MAP via BPF_MAP_UPDATE_ELEM
- After the bulk copy, the existing double-read tearing protection reads
  the now-updated SCREEN_MAP as before

**Why userspace:** The BPF verifier enforces a hard limit on loop iterations
(~8,192 jumps, ~1M processed instructions). A 16,000-iteration inner loop
(250 chunks x 64 words) exceeds this. Userspace has no such limits and can
perform the copy in a straightforward loop.

### 2. Ring Buffer Event Sampling (Anamnesis)

**File:** `ebpf/monad-cpu-ebpf/src/main.rs`

**Change:** SCREEN_WRITE events are now emitted to the COMPUTE_EVENTS ring
buffer only every 32nd frame (when `frame_count & 0x1F == 0`) instead of
every frame.

**Impact:** At 35fps, this reduces ring buffer events from 35/sec to ~1/sec,
significantly reducing ring buffer pressure and the associated overhead of
the `emit_screen_write()` function (which constructs and submits a
ComputeHopEvent struct).

The frame-ready counter (STAT_FRAME_READY) is still incremented every frame,
so userspace frame detection is unaffected.

### 3. Performance Impact Summary

| Metric | Before | After |
|--------|--------|-------|
| `copy_fb_to_screen()` | No-op (verifier blocked) | Userspace bulk copy |
| SCREEN_MAP updates per frame | Per-pixel STB only | Full 64K bulk + per-pixel |
| Ring buffer events per second | 35 (every frame) | ~1 (every 32nd frame) |
| Userspace overhead | Screen read only | Screen read + frame-ready poll |

## Architecture Notes

The frame-ready signaling uses the existing STATS HashMap (BPF_MAP_TYPE_HASH)
which is already shared between eBPF and userspace. No new maps or ring buffer
events are needed for the synchronization.

The bulk copy in userspace uses individual `BPF_MAP_LOOKUP_ELEM` calls for
RAM_MAP reads and `BPF_MAP_UPDATE_ELEM` calls for SCREEN_MAP writes. This is
O(16,000) syscalls per frame copy, which at 35fps would be ~560K syscalls/sec.
Future optimization could use `BPF_MAP_LOOKUP_BATCH` for RAM_MAP reads if
kernel batch support is available.
