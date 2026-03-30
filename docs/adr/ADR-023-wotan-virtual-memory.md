# ADR-023: Virtual Memory in Wotan

**Status:** PROPOSED
**Date:** 2026-03-30
**Context:** Doom-over-IPv6 UPC, Dream Ladder L5 (MiniKernel)

## Context

The UPC (Unheaded Protocol Computer) runs MBC bytecode inside eBPF XDP programs.
Memory is backed by BPF maps:
- **RAM_MAP** (Array<u32>, 16M entries = 64MB kernel memory) — flat address space
- **L1_CACHE** — hot path cache (not currently used for Doom)

Doom's retail WAD (12.4MB) requires 12MB+ zone memory for texture cache in later
levels. With 26MB total heap, this works but leaves limited headroom. More
importantly, future workloads (xv6, FUZIX, Linux) will need >64MB address space
with memory protection.

## Problem

1. BPF maps are physically allocated in kernel memory — 16M entries = 64MB is the
   practical limit before kernel OOM.
2. No memory protection — any MBC instruction can write anywhere in RAM_MAP.
3. No swap — when zone memory runs out, Doom crashes (texture corruption).
4. No demand paging — all data must be loaded upfront.

## Proposal: Wotan Virtual Memory

Extend Wotan's memory hierarchy with virtual memory semantics:

```
┌─────────────────────────────────────────────┐
│ MBC Address Space (32-bit, 4GB virtual)     │
├──────────┬──────────┬───────────────────────┤
│  L1 TLB  │  L2 Page │  L3 Backing Store     │
│ (BPF map)│  Table   │  (userspace mmap)     │
│  ~1MB    │ (BPF map)│  ~unlimited           │
│  <100ns  │  ~200ns  │  ~1ms (ring buf evt)  │
└──────────┴──────────┴───────────────────────┘
```

### Page Table Design

- **Page size:** 4KB (matches Linux, 1024 pages per 4MB)
- **TLB:** BPF HashMap (virtual page → physical frame), ~256 entries
- **Page table:** BPF Array (virtual page → physical frame + flags)
- **Backing store:** Userspace file or mmap region

### Page Fault Handling

1. MBC executor reads/writes a virtual address
2. TLB miss → check page table
3. Page table miss → emit ring buffer event (PAGE_FAULT)
4. Userspace handler loads page from backing store into RAM_MAP
5. Update page table + TLB
6. MBC executor retries

### Dirty Page Writeback

- MBC writes set a dirty bit in the page table entry
- Periodic sweep (every N ticks) writes dirty pages to backing store
- Or: write-through for critical regions (screen, kbd)

## Trade-offs

| Aspect | Pro | Con |
|--------|-----|-----|
| Address space | 4GB virtual on 64MB physical | TLB miss adds ~200ns/access |
| Memory protection | Page-level R/W/X flags | Verifier complexity for permission checks |
| Swap | Unlimited via userspace backing | Page fault latency (~1ms) |
| Demand paging | Only load what's needed | Cold start slower for first access |

## Applicability

| Dream Ladder Level | Needs VM? | Why |
|-------------------|-----------|-----|
| L3 (Doom) | Optional | 26MB heap is sufficient with 12MB zone |
| L4 (OS Primitives) | Helpful | Process isolation needs memory protection |
| L5 (MiniKernel) | Required | xv6/FUZIX expect paged memory |
| L6 (Linux Boot) | Required | Linux requires MMU |

## Decision

PROPOSED — implement when Dream Ladder reaches L5 (MiniKernel). For L3 (Doom),
the current flat 26MB heap with 12MB zone is sufficient. The TLB_MAP already
exists in the eBPF program (currently unused) as a placeholder for this feature.

## References

- `ebpf/monad-cpu-ebpf/src/main.rs` — TLB_MAP already defined
- `crates/doom-runner/src/memory.rs` — current flat memory layout
- Dream Ladder: `docs/doom/ARCHITECTURE.md`
- Wotan spec: draft-03 (memory hierarchy)
