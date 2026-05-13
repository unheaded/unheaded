# UPC Page Table Layout — Phase 1.2 (ADR-074)

**Status**: Pair-call landed 2026-05-12 — Option A + Allocator A1 chosen.
**See**: `docs/adr/ADR-074-phase12-page-table-model.md` for full design rationale.

## Address Space (Option A: per-task pgd, fixed per-pid region)

The 64 MiB `RAM_MAP` (16,777,216 × u32) carves out a fixed 16 KiB region for the
4-slot process page directories. The choice of 0x00F00000 is deliberate: above the
default kernel image region (`0x00010000+` for xv6.mbc) and the .data/.bss/heap
range (default program break = `0x00400000`), below the userspace stack region
(top = `0x03F00000`).

```
RAM_MAP byte address       Contents
─────────────────────       ──────────────────────────────────────────────────
0x00000000  -  0x000003FF   IVT (256 vectors × 4 B)
0x00000100  -  0x000001FF   BootParams v2  (256 B)
0x00000200  -  0x000003FF   cmdline        (≤ 512 B)
0x0000F000  -  0x0000F3FF   Memory-mapped CSR region (zero-filled by bootstub)
0x00010000  -  ~~~~~~~~~~   kernel image (xv6 entry = start_mbc.c)
0x00400000  -  0x008FFFFF   default heap window (program_break grows up)
0x00900000  -  0x00EFFFFF   reserved for L4d MMU page-table area (current usage)
0x00F00000  -  0x00F00FFF   pgd[0]  pid 0 page directory  (1024 entries × 4 B)
0x00F01000  -  0x00F01FFF   pgd[1]  pid 1 page directory
0x00F02000  -  0x00F02FFF   pgd[2]  pid 2 page directory
0x00F03000  -  0x00F03FFF   pgd[3]  pid 3 page directory
0x00F04000  -  ~~~~~~~~~~   future: per-pid PT pool (Phase 1.2 may not need)
0x00800000  -  ~~~~~~~~~~   ramdisk image (Phase 1.4)
0x03F00000  -  0x03FFFFFF   userspace stack region (SP top, grows down)
0x04000000+                 unmapped — RAM_MAP top is 64 MiB
```

Notes:
- Per-pid region is 16 KiB total. RAM_MAP total = 64 MiB; this is < 0.025% of
  RAM_MAP. Negligible.
- The 4-pid cap is the existing scheduler limit (`PROC_TABLE` is `Array<[u32; 20]>`
  with max_entries = 4). Phase 1.2 inherits this cap. Phase 3 will widen.
- Each pgd is a 1024-entry first-level page directory (4 KiB). Second-level
  page tables live elsewhere — see "Second-level allocation" below.

## PROC_TABLE slot — proposed widening for Option A

Current layout: `Array<[u32; 20]>` with slot fields:

```
slot[0..15]   r0..r15
slot[16]      PC
slot[17]      flags
slot[18]      SP_copy
slot[19]      program_break
```

Proposed Option-A widening: `Array<[u32; 21]>` adds:

```
slot[20]      page_dir_base   (physical address of this pid's pgd, e.g. 0x00F00000)
```

Memory delta: 4 bytes × 4 slots = 16 bytes BPF map growth. Verifier-neutral.

## Second-level page table allocation

Phase 1.2 has two structural options for second-level PTs:

- **(A1) Fixed per-pid PT pool**: reserve `0x00F04000 - 0x00F0FFFF` (48 KiB) for
  4-pid × 3 PTs each. xv6 maps text + data + stack — 3 PTs suffice. Architect's
  pick for Phase 1.2 simplicity. **CHOSEN per ADR-074 Decision 2026-05-12.**
- **(A2) Kernel freelist**: add `SYS_ALLOC_PAGES` syscall; xv6 manages a freelist
  in `RAM_MAP`. More general; Phase 3 candidate.

This doc records A1 as the Phase-1.2 default; A2 is documented for Phase 3
forward awareness.

## Context-switch sequence (Option A reference)

```
SYS_SCHED_YIELD (or trap exit) triggers:
  1. Save: PROC_TABLE[current_pid][20] ← cpu.page_dir_base
  2. Load: cpu.page_dir_base ← PROC_TABLE[next_pid][20]
  3. Flush: zero TLB_MAP (already chunked 8/tick in monad-cpu-ebpf:1294)
  4. Update: cpu.current_pid ← next_pid
  5. Resume: PC ← PROC_TABLE[next_pid][16]
```

The flush in step 3 takes 8 ticks (8 × 8 entries). The CPU stalls for those
ticks; xv6's scheduler tick (~10 ms) easily absorbs them.

## Option B / Option C deltas (sketch — full design in ADR-074)

- **Option B (shared+ASID)**: drop the per-pid pgd region. Add
  `cpu.current_asid: u8`. TLB widens from `[u32; 3]` to `[u32; 4]`. Context
  switch is a 1-byte write; no flush.
- **Option C (per-task pgd + pid-tagged TLB)**: same per-pid region as Option A.
  TLB widens like Option B. Context switch updates BOTH `page_dir_base` and
  `current_pid`. Optional flush.

## Phase 3 follow-on flag

In Wotan DISTRIBUTED mode, `page_dir_base` as a physical address breaks the
model: another node's CPU cannot resolve `0x00F00000` unless RAM_MAP layout is
identical kingdom-wide. Phase 3 will need a logical `(node_id, pgd_id)` handle
that Wotan resolves at access time. **Phase 1.2 does not need to solve this**;
flagged here so Phase 3 doesn't re-derive it.

## References

- `docs/adr/ADR-074-phase12-page-table-model.md` — full ADR + Architect addendum + Decision 2026-05-12
- `ebpf/monad-cpu-ebpf/src/phase12.rs` — Phase 1.2 helpers (`pgd_base_for_pid`, `PROC_TABLE_PGD_SLOT`)
- `ebpf/monad-cpu-ebpf/src/main.rs::translate_address()` — existing MMU walker
- `ebpf/monad-common/src/lib.rs::MbcCpuState` — CPU state struct
- `docs/doom/UPC_BOOT_PROTOCOL_V2.md` — memory map authoritative source for the
  0x00000000-0x000003FF, 0x0000F000-0x0000F3FF, and other reserved regions
