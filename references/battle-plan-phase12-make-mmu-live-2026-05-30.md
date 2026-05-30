# Battle plan — Phase 1.2 "make the MMU live" (Option A / A1)

**Date:** 2026-05-30
**Owner:** Computermancer (+ Developer, Architect, BlackMage on the isolation gate)
**Status:** IN PROGRESS — strategy (i) large-page loader-side chosen. **Gate 1 PASS (2026-05-30):**
MMU live, pid-0 identity pgd (16×4MiB superpages) at 0x00F00000, priv-gated translation; boot
still reaches `init: starting sh`. Next: Gate 2 (per-pid physical offset → isolation).
**Decided already (ADR-074, 2026-05-12):** Option A per-task pgd; Allocator A1 fixed per-pid
pgd at `RAM_MAP[0x00F00000 + pid*0x1000]`; `PROC_TABLE[20] = page_dir_base`.

## Why now

Phase 1.6 landed real `fork`/`wait`/`exit` (RV32I ecall path), but they can't produce a
shell because **all processes share one flat `RAM_MAP`** (`mmu_enabled==0`,
`translate_address` is a passthrough). The child clobbers the parent's stack. Per-process
isolation = the decided Option A MMU, turned on. See
`references/phase16-session-2026-05-30-fork-wait-exit.md`.

## What already exists (inert substrate)

- `translate_address` (`ebpf/monad-cpu-ebpf/src/main.rs:2239`) — Sv32 2-level walk + 64-entry
  TLB; early-returns while `mmu_enabled==0`. Wired into all 6 LOAD/STORE sites.
- `phase12.rs` — `pgd_base_for_pid`, `PER_PID_PGD_BASE=0x00F00000`, `PROC_TABLE_PGD_SLOT=20`.
- `SYS_FLUSH_TLB` — chunked 8/tick flush (verifier-safe). `SYS_SET_PAGE_DIR`, `SYS_ENABLE_MMU`.
- `scheduler_context_switch` already saves/loads `page_dir_base` per pid (slot 20).

## What's missing (the work)

1. **Populate page tables.** Build a Sv32 page table at each pid's pgd region mapping that
   process's VA pages to a DISJOINT physical RAM slice. (Construction strategy = §Decision.)
2. **Enable translation for userland** — set `mmu_enabled=1` + `page_dir_base=pgd_base_for_pid(pid)`
   at user entry (kexec/SRET-to-user).
3. **fork → real address space** — child gets `pgd_base_for_pid(child_pid)`, its own physical
   slice, and a COPY of the parent's slice. (Register copy already done; add memory copy +
   pgd build.)
4. **Context switch** — already loads slot-20 pgd; ensure TLB flush fires (fold the
   `phase12_option_a` hook into `scheduler_context_switch` or enable the feature).
5. **Kernel→user pointer translation** — syscall handlers that read/write user pointers
   (`SYS_write` buf, `wait` status, exec path) must resolve through the current pgd
   (walkaddr equivalent), not raw `mem_read_byte`.

## Verification (strong-inference, per ADR-074)

- **Gate 1 (single process):** enable MMU for init alone with an identity slice; boot still
  reaches `init: starting sh`. Proves the live walk path.
- **Gate 2 (isolation — the hard safety property):** PID 1 writes a sentinel to a VA; switch
  to PID 2; PID 2 reads the same VA and must NOT see the sentinel (disjoint physical slices).
- **Gate 3 (lifecycle):** init forks child, child runs in its own slice, prints
  `init: exec sh failed` (Stage-2 signal) WITHOUT corrupting init; init's wait reaps and loops
  cleanly (no chaotic `0x4024` divergence).
- **Budget:** verifier stays < 12% hard gate (predicted delta +0.2–0.8% per ADR-074 §Issue 3).

## §Decision — page-table construction strategy (needs Stevie)

Where do the Sv32 tables get built? Three tractable shapes:

- **(i) Loader/BPF-side, 4 MiB large pages** — add large-page support to `translate_address`
  (pde "leaf" flag short-circuits the 2nd-level read), build per-pid pgds with a handful of
  4 MiB mappings (per-pid physical offset). Fewest entries, smallest blast radius, keeps Sv32
  fidelity. Needs a small `translate_address` change.
- **(ii) Loader/BPF-side, 4 KiB pages, minimal footprint** — map only the pages init/sh
  actually touch (stack + data). No walker change, but must enumerate the working set and
  grow it on faults (more bookkeeping).
- **(iii) Revive xv6 `uvm`/`exec`** — un-skip `kvminit` (fix the PHYSTOP/etext math) and route
  userland through xv6's real page-table construction. Most faithful to upstream, biggest
  surface (fights the MBC-loader/kexec bypass that exists today).

**Computermancer recommendation: (i).** Large-page identity-with-offset is the least code to
a working isolated shell, preserves the Sv32 path for Linux, and the `translate_address`
large-page tweak is small and verifier-cheap. (iii) is the "right for Linux" endgame but is a
separate, larger effort better tackled once isolation is proven with (i).

## First step if (i) is chosen

Add large-page (leaf-pde) support to `translate_address`, build pid-0 pgd with 4 MiB
identity mappings over the userland RAM region, set `mmu_enabled=1` at init entry → **Gate 1**.
Check in at Gate 1 before touching fork-copy.
