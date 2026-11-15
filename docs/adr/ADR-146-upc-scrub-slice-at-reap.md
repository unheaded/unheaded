<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-146: A UPC slot's slice is scrubbed when its process is reaped

**Status:** Proposed (parked 2026-11-12, Stevie's answer A-61). Not built; no plan.
**Date:** 2026-11-15
**Related:** ADR-135 D5 (zero on growth), its Security notes ("Bytes between processes") and its first review, finding F8 (`references/s-stage-2b-log.md`); ADR-135 R-F4 (exec does not scrub); ADR-109 D6 (`UPC_D6_MISS_FAULT`); ADR-074 (per-pid slices).

## Context

Each BPF slot owns an 8 MiB slice of physical memory. When a process is reaped, the slot is freed and pde 0 is cleared; the slice's bytes stay. Fork overwrites the low copy band of the next occupant, so a dead process's image and heap do not reach it on the normal path. Above the band nothing is overwritten: the next occupant of the slot can read the last one's stack and its argv frame.

ADR-135 D5 zeroes a range when `sbrk` grows into it. That makes the range correct for `malloc`. It is not a confidentiality control: a child reads its whole slice without calling `sbrk` (F8).

With ADR-109 D6 on and `MRET`/`SRET` refused at U-mode (S-STAGE-2C Phase 2), a live process can no longer read another live process's slice. What remains is the dead-to-next route above.

## Decision (proposed)

The reap path zeroes what the dead slot's process could have written and fork will not overwrite:

1. the stack window,
2. the argv frame,
3. with R-F4, the part of the low band that exec does not overwrite (a heap inherited through fork and left under a newly exec'd image).

The zeroing is chunked across ticks, as `sbrk`'s is, so no single trap carries the whole cost. Measure the verifier cost and the per-reap time before choosing between a BPF pass and a kernel pass through the slice's flat address.

## Consequences

- Closes the dead-to-next leak inside a slot: the remaining route by which one process's bytes reach another once D6 is on.
- Every reap pays for the scrub. A fork-heavy workload (the shell's corpus) pays it per command; the measurement decides whether that is acceptable or whether the scrub runs lazily, before the slot's next fork.
- A slot must not be handed out again before its scrub ends, which adds a state to slot recycling.

## Not decided

Scrubbing at exec (R-F4 alone), which is a separate, smaller change: an exec'd program reads its parent's heap today, and that is a parent-to-child flow within one slot, not a flow between processes.
