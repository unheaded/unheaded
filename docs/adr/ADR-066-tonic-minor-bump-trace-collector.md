# ADR-066 — `cmd/trace-collector` tonic 0.10 → 0.12 Minor-Version Bump

**Status:** Accepted
**Date:** 2026-05-08
**Deciders:** unheaded-developer (impl) + unheaded-architect (transport surface) + unheaded-moatghost (advisory closure) + unheaded-marshal (gate enforcement)
**Aligns with:** ADR-052 (in-tree source-of-truth policy for Cargo edits), ADR-049 (transport-layer security baseline), security/cargo-audit-2026-05-07.md (Wave A remediation)

---

## Context

The 2026-05-07 cargo audit sweep surfaced four CVE-class advisories pinned by `cmd/trace-collector/Cargo.toml`:

| Advisory | Crate | Severity | Path |
|----------|-------|----------|------|
| RUSTSEC-2026-0104 | rustls-webpki 0.101.7 | High | tonic 0.10.2 → rustls 0.21 → rustls-webpki 0.101 |
| RUSTSEC-2026-0099 | rustls-webpki 0.101.7 | Medium | same chain |
| RUSTSEC-2026-0098 | rustls-webpki 0.101.7 | Medium | same chain |
| RUSTSEC-2024-0437 | protobuf (older) | Medium | prost 0.12 transitive |

`zhend` already upgraded to `rustls-webpki 0.103.13` (commit ff24faa8). The trace-collector chain is locked behind the tonic 0.10 line — closing the four advisories requires a Cargo.toml edit, which under ADR-052 requires a record of decision before merge.

`zhend` and `cmd/ebpf-loader` are already on tonic 0.12, so this bump is **alignment with established kingdom convention**, not a new direction.

## Decision

Bump `cmd/trace-collector/Cargo.toml` from tonic 0.10 to tonic 0.12 in a single commit. Drag along the `prost` 0.12 → 0.13 and `tonic-build` 0.10 → 0.12 dependencies that tonic 0.12 requires. Run `cargo build`, `cargo test`, and `cargo audit` against the bumped lockfile to confirm: (a) zero compile errors, (b) tests still pass, (c) the four advisories drop out of the audit report.

The tonic API surface used by trace-collector is intentionally small — `tonic::transport::{Channel, Endpoint}`, `tonic::Status`, `tonic::Code`. These four symbols are stable across the 0.10 → 0.12 boundary; no API drift mitigation expected.

## Migration steps

1. **Cargo.toml edits:**
   - `tonic = { version = "0.12", features = ["gzip", "tls"] }`
   - `prost = "0.13"` and `prost-types = "0.13"`
   - `[build-dependencies] tonic-build = "0.12"`
2. **Cargo.lock regen:** `cargo update -w` from `cmd/trace-collector/`.
3. **Build:** `cargo build --release`.
4. **Test:** `cargo test`.
5. **Audit:** `cargo audit` should show 4 fewer advisories. Capture the diff in commit body.
6. **Single commit:** `feat(trace-collector): bump tonic 0.10 → 0.12 (closes ADR-066, 4 CVEs)`.

## Verification

```bash
cd ~/tmp/unheaded/cmd/trace-collector
cargo build --release 2>&1 | tail -10
cargo test 2>&1 | tail -10
cargo audit 2>&1 | grep -E "^(ID|warning|error)" | head -20
```

Pass criteria: `cargo build` and `cargo test` exit 0; `cargo audit` shows zero advisories matching the four IDs above.

## Consequences

**Positive:**
- 4 CVE-class advisories closed.
- Trace-collector aligns with zhend + ebpf-loader on tonic 0.12.
- Reduces tonic-version diversity across the repo from 2 to 1.

**Negative:**
- One more crate-graph rebuild on the next CI run.
- If tonic 0.12 ships any subtle behavioral change (timeout defaults, TLS handshake retries) we'd want to catch in fuzz testing before public launch.

**Neutral:**
- `prost 0.13` is a peer-dep bump only; trace-collector does not generate proto bindings dynamically.

## Alternatives considered

- **Keep tonic 0.10, accept the 4 CVEs.** Rejected. Trace-collector's TLS surface is the primary entry point for inter-host BPF event shipping; cert-validation CVEs are not academic.
- **Bump only rustls-webpki via `[patch.crates-io]`.** Rejected per ADR-052: ad-hoc patch overrides become invisible policy that future shifts cannot reason about. A deliberate tonic bump is cleaner.
- **Defer to launch-prep.** Rejected. The Wave A daytime gate explicitly schedules this work; deferring extends the unsigned-CVE window.

## Cross-reference

- `docs/security/cargo-audit-2026-05-07.md` — Wave A remediation plan.
- `references/marshal-parked-2026-05-07.md` — TONIC-BUMP-NEEDED entry, this ADR closes it.
- ADR-049 — transport security baseline.
- ADR-052 — in-tree source-of-truth policy.
