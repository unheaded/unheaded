# ADR-066 — `cmd/trace-collector` tonic 0.10 → 0.12

**See full ADR**: `docs/adr/ADR-066-tonic-minor-bump-trace-collector.md`
**Status**: Accepted (2026-05-08)

## Summary

Bumped `cmd/trace-collector/Cargo.toml` from `tonic = "0.10"` to `tonic = "0.12"`, closing **four CVE-class advisories** in the rustls-webpki + protobuf transitive chain:

| Advisory | Crate | Severity |
|----------|-------|----------|
| RUSTSEC-2026-0104 | rustls-webpki 0.101.7 | High |
| RUSTSEC-2026-0099 | rustls-webpki 0.101.7 | Medium |
| RUSTSEC-2026-0098 | rustls-webpki 0.101.7 | Medium |
| RUSTSEC-2024-0437 | protobuf (older) | Medium |

This is **alignment with established kingdom convention** — `zhend` and `cmd/ebpf-loader` were already on tonic 0.12 — not a new direction.

## Verification

- `cargo audit` in `cmd/trace-collector/` shows zero advisories post-bump
- Build + tests green
- gRPC wire format unchanged

## See also

- `docs/adr/ADR-066-tonic-minor-bump-trace-collector.md` — full ADR
- `security/cargo-audit-2026-05-07.md` — Wave A advisory inventory
- ADR-049 — transport-layer security baseline
- ADR-052 — in-tree source-of-truth policy for Cargo edits
