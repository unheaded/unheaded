<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-145: UPC user images are checked against the heap window's floor

**Status:** Proposed (parked 2026-11-12, Stevie's answer A-61). Not built; no plan.
**Date:** 2026-11-15
**Related:** ADR-135 D2 (heap window) and its first review, finding F7 (`references/s-stage-2b-log.md`); ADR-109 (kernel rows per BPF slot); ADR-077 (exec of loader-resident images); `crates/xv6-mbc/adapters/Makefile.mbc-userland`; `cmd/upc-bootctl`.

## Context

ADR-135 D2 puts every child's heap at VA [0x10000, 0x40000). The window is bookkeeping, not an MMU boundary: a child's whole VA [0, 8 MiB) is mapped. D2 therefore assumes that a user image (text, `.data` and `.bss`) ends below 0x10000.

Nothing checks the assumption. The largest image in the corpus ends at 0x2084 (`sh`, measured for F7 on the seven `.elf` files in `target/`; the probes keep no `.elf`). An image that grows past 0x10000 would link and boot, and its upper bytes would share addresses with the heap: the first `sbrk` zeroes them (D5), and `malloc` hands them out.

## Decision (proposed)

Two checks, at the two places an image's extent is known:

1. **At link time.** `Makefile.mbc-userland` fails a link whose `_end` reaches 0x10000. The message names the program and its `_end`.
2. **At load time.** bootctl refuses an image whose `.data` sidecar records an address at or above 0x10000, for `--userland` and for every resident program.

The floor is `UPC_HEAP_BASE` (`ebpf/monad-cpu-ebpf/src/phase12.rs`), read by both checks, so moving the window moves them.

## Consequences

- A program that outgrows the space below the heap fails at build with a named cause, where today it would corrupt its own heap at run time.
- The build check sees `.bss` (through `_end`); the load check sees only what the sidecar carries (`ALLOC PROGBITS`), so it is the weaker of the two and exists for images built elsewhere.
- No verifier cost: neither check is in the BPF object.
- Until this is opened, D2's sentence stands as written: an assumption, measured once, enforced nowhere.

## Not decided

Whether the window should move instead (a larger image area costs heap, and a third fork copy band costs verifier instructions). That is ADR-135's to answer if a real program approaches the floor.
