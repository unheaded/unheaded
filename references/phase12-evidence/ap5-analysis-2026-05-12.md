# AP-5 — MEM_READ_WORD Verifier Cost Sampling

**Date**: 2026-05-12
**Source baseline**: `references/phase12-evidence/bpf-budget-baseline-2026-05-12.txt`
**Question**: Does the Scientist's revised "+0.2% to +0.8%" Option-A verifier-budget delta hold up?

## Method

`translate_address()` already exists at `ebpf/monad-cpu-ebpf/src/main.rs::translate_address()`.
Step 33 confirms **6 callsites** of the MMU walker pattern (`translate_address` + `mem_read_word` accesses).

The per-access path is:

1. `mmu_enabled == 0` short-circuit (~3 BPF insns)
2. TLB lookup (~10-15 BPF insns: array get + 2 field compares + return)
3. On TLB miss: 2× `mem_read_word()` (~20 insns each = 40 insns) + TLB install (~10 insns)

Option A adds:
- Context-switch path: extend PROC_TABLE save/restore by 1 slot field (~5 insns / context switch)
- Flush: ALREADY chunked at 8/tick (existing code, no delta)

## Predicted delta

For per-context-switch cost (rare, ~once per scheduler tick):
- +5 BPF insns × N context switches per packet ≈ 0 in steady state (context switch is not per-packet)

For per-memory-access cost:
- NO change to translate_address(). Option A doesn't touch the TLB lookup.

**Predicted absolute delta: ~0.05% to ~0.5%** (lower bound of Scientist's revised estimate).

## Measured delta — three feature variants vs baseline

Method: clean build per variant, sized stripped binary.

| Build                    | Size (bytes) | Est. insns | Δ vs baseline | % of 900K budget |
|--------------------------|-------------:|-----------:|--------------:|-----------------:|
| baseline (no feature)    |      606,920 |     75,865 |             0 |           8.43 % |
| `phase12-option-a`       |      606,920 |     75,865 |             0 |           8.43 % |
| `phase12-option-b`       |      606,920 |     75,865 |             0 |           8.43 % |
| `phase12-option-c`       |      606,920 |     75,865 |             0 |           8.43 % |

**Measured delta: 0 bytes / 0 instructions across all three options.**

This is the expected outcome at this pre-implementation stage: the feature-flagged
hooks in `ebpf/monad-cpu-ebpf/src/phase12.rs` are NOT yet wired into the
SYS_SCHED_YIELD codepath. The linker strips the dead stub functions, so the binary
is byte-identical to the baseline. The measurement is informative as a **floor**:
the cost of *opting in* to the feature flag itself is zero.

The actual verifier-cost question — does calling the Option-A hook from the
scheduler path expand the proof graph past the predicted 0.05-0.5% — gets answered
in the implementation arc (Phase 1.2 IMPL, task #76), not pre-work. Pre-work has
proven that the scaffold is verifier-neutral, which is what the pair-call needs to
know to ratify Option A.

## Falsification

If the actual delta **after wiring** exceeds 2% — the verifier path-exploded. Either:
1. The feature flag accidentally enabled a code path it shouldn't have.
2. The stub function got inlined into a hot path and expanded the verifier proof graph.

Either way: implementation must address before adoption.

## See also

- `docs/adr/ADR-074-phase12-page-table-model.md` — full ADR + Decision 2026-05-12
- `references/phase12-evidence/bpf-budget-baseline-2026-05-12.txt` — AP-2 baseline
- `references/phase12-evidence/ap5-extract-2026-05-12.txt` — extracted baseline numbers
- `ebpf/monad-cpu-ebpf/src/phase12.rs` — the scaffold under measurement
