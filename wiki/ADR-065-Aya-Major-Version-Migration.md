# ADR-065 — aya 0.1.x → 0.13.x Major-Version Migration Plan

**See full ADR**: `docs/adr/ADR-065-aya-major-version-migration.md`
**Status**: Superseded 2026-05-08 — **no migration needed**

## Summary

Originally proposed migrating `ebpf/Cargo.toml`'s `aya-ebpf = "0.1"` (kernel-side) to match the userspace `aya = "0.13"`. Phase-A discovery (2026-05-08 Marshal drain shift) reversed the migration: **aya splits userspace and kernel into independent crate families** (`aya` 0.13 ↔ `aya-ebpf` 0.1). They are designed to evolve at different cadences. The kernel half does NOT need to chase the userspace major.

## Resolution

- `ebpf/Cargo.toml` keeps `aya-ebpf = "0.1"` indefinitely until upstream signals a kernel-side major bump
- Userspace continues independent updates via `cmd/ebpf-loader`'s `aya = "0.13"`
- Verifier-budget gate (`scripts/bpf-verifier-check.sh`) stays at 7% / 900K instructions
- 23 eBPF programs unaffected

## Lesson

Always sample the upstream design intent before assuming "same crate name = same major release cadence." This ADR's two-day life cycle (proposed 2026-05-07, superseded 2026-05-08) was the right outcome — better to be wrong and corrected fast than to ship a multi-day migration that wasn't needed.

## See also

- `docs/adr/ADR-065-aya-major-version-migration.md` — full ADR with addendum
- `references/marshal-shift-2026-05-08-drain.md` — Phase A discovery
- ADR-003 — eBPF Rust + Aya framework choice (the original adoption decision)
