# Phase 1.2 IMPL — COMPLETE

**Date:** 2026-05-13
**ADR:** docs/adr/ADR-074-phase12-page-table-model.md
**Predecessor plan:** references/battle-plan-phase12-impl-2026-05-13.md
**Pre-work plan (closed):** references/battle-plan-phase12-prework-2026-05-11.md

## Steps landed (8 of 8 from the IMPL plan)

| Step | What | Commit |
|------|------|--------|
| 1 | PROC_TABLE [u32; 20] → [u32; 21] in BPF + host emulator | 1f4a357a |
| 2 | scheduler_context_switch saves slot[20] + loads on context switch | 1f4a357a |
| 3 | Feature-gated hook invocation in scheduler path | 1f4a357a |
| 4 | xv6 proc.c::proc_pagetable() passes p->pid to uvmcreate | 84fc38d1 |
| 5 | xv6 swtch_mbc.S — confirmed no Option A changes needed | (audit only) |
| 6 | xv6 vm.c::uvmcreate() allocates from per-pid fixed region | 84fc38d1 |
| 7 | Forkbomb-style smoke test on host emulator (2 tests pass) | 1f4a357a |
| 8 | ADR-074 falsification on host emulator (2nd of 2 tests) | 1f4a357a |

## Deferred to Phase 1.3+

- Live forkbomb on the actual booted xv6 kernel — Phase 1.3 must first
  advance xv6 past the banner into the scheduler.
- Multi-process xv6 demo — Phase 1.5 territory.
- Phase 3 DISTRIBUTED-mode replacement of physical-address page_dir_base
  with a logical (node_id, pgd_id) handle (already flagged in ASCEND-LINUX
  battle plan Phase 3.1 per AP-6).

## Verifier cost (final)

| Snapshot | Estimated insns | % of 900K |
|----------|----------------:|----------:|
| AP-2 baseline | 75,865 | 8.43 % |
| Post-IMPL Step 1 | 76,125 | 8.46 % |
| Post-IMPL Phase A+B (xv6 patches) | 76,125 | 8.46 % |

Phase A+B touched only vendored xv6 C source — zero BPF delta, as expected.
Total delta from AP-2 baseline: +260 insns (0.03% of budget), well under
the 2% falsification gate.

## Boot regression (Step 9 evidence)

```
=== TTY OUTPUT (44 bytes) ===
  ascii: "xv6 booting...·BOOT FAIL: MRET fall-through·"
  CPU_MAP[0xDE]: PC=0x00000081 SP=0x000E1030 priv=0 halted=1 insn_count=383
```

Banner emits cleanly. CPU halts at the same MRET fall-through as 3ac1f684
(Phase 1.1 SHIP) — no privilege transition yet, which is expected: Phase
1.2 Option A only wires the page-table allocator; M→S transition is
Phase 1.3 work (process model bring-up + scheduler advancement).

## Closing

Phase 1.2 IMPL is functionally complete. The Option A substrate is in
place across BPF, host emulator, AND the vendored xv6 C source. Live
runtime validation gates on Phase 1.3 advancing xv6 past the banner.

*Free to use. Free to share.*
