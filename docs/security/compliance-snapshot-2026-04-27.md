# Compliance Snapshot — 2026-04-27

**Captured**: 2026-04-27 (Cowork-on-Macbook session, with Stevie KEV upload assist)
**Owners**: MoatGhost (compliance) + Sentinel (threat intel) + Barrister (legal)
**HEAD at capture**: `2e01fc09` (CHUNK 1 of Sprint 2026-04-27 LOCAL)

> Three sections complete locally; SBOM + license-scan rows pending Linux-box execution per `docs/security/COMPLIANCE-REMOTE-PACKET-2026-04-27.md`.

---

## SPDX coverage

| Language | Total | Missing SPDX | Coverage % | Status |
|---|---:|---:|---:|---|
| Go | 1,162 | 6 | 99.48% | 🟢 ACCEPTABLE — small backfill in remote packet Section D |
| Rust | 208 | 156 | 25.0% | 🟡 GAP — needs dedicated sprint (see `spdx-coverage-2026-04-27.md`) |

Go gap is mechanical (mostly recent files in `cmd/routing-health/` + `cmd/test_batch/` + one auto-gen `.pb.go`). Rust gap was deferred at S52 and now warrants its own sprint slot.

## CISA KEV (threat intel)

✅ **COMPLETE** (2026-04-27, Stevie-upload assist after sandbox proxy block)

- Total entries: **1,583** (range 2021-11-03 → 2026-04-24)
- Last 14 days: **24** entries
- **Relevant to Unheaded stack: 0** ← clean week
- Snapshot saved: `docs/security/feeds/cisa-kev-2026-04-27.json`
- Threat register updated: `docs/security/threat-register.md`

No patch actions required this sprint based on KEV.

## SBOM regeneration

🔴 **PENDING REMOTE EXECUTION** — `syft` not present in Cowork sandbox.

- Baseline (per CLAUDE.md S52): 553 packages
- Target: regenerate via `syft dir:. -o spdx-json > docs/legal/sbom-2026-04-27.spdx.json`
- Owner: next Linux dev box session
- Commands: `COMPLIANCE-REMOTE-PACKET-2026-04-27.md` Section A
- Expected wall-clock: 5 min

## License scan

🔴 **PENDING REMOTE EXECUTION** — `cargo-deny` and `go-licenses` not present in Cowork sandbox.

- Owner: next Linux dev box session
- Commands: `COMPLIANCE-REMOTE-PACKET-2026-04-27.md` Section B
- Expected wall-clock: 10 min combined
- Acceptance: zero `error[]` lines in cargo-deny output; zero unexpected GPL deps in go-licenses CSV

## SOC 2 Y1 trajectory

(Note from Round Table 2026-04-27 — not the central focus of this sprint, captured for continuity.)

- **Where we are**: pre-evidence-collection. Many controls map to existing infrastructure (auth framework, audit logging, RBAC) but formal evidence packs not yet assembled.
- **What's needed for Type 1**: control-by-control evidence binders, ~6 weeks focused work, Captain + Barrister + MoatGhost coordination.
- **Trigger to start**: VC conversations approach the term-sheet stage, OR a customer engagement explicitly requires SOC 2.
- **Recommendation**: defer to Sprint May-Q1 or later; not blocking public alpha (Track B/C).

## Cross-references

- ADR-052 (drift policy) — this snapshot is one of the doc-web layers tracked.
- ADR-053 (Hybrid Claude+Zhenai routing) — compliance work routes to HEAVY classifier (Claude), never local Zhenai.
- LICH-012 campaign — closed 2026-04-11, real-metal validated; no follow-on actions.
- `docs/security/COMPLIANCE-REMOTE-PACKET-2026-04-27.md` — packaged commands for SBOM + license scan + Go SPDX backfill.
- `docs/security/spdx-coverage-2026-04-27.md` — full SPDX gap analysis with Rust sprint plan.
- `docs/security/threat-register.md` — refreshed with KEV data this sprint.

## Posture summary

🟢 **Good**: KEV clean for our stack, Go SPDX 99.48%, ADR-052 CI gate live, Mímir's Law real-metal validated.
🟡 **Watch**: Rust SPDX gap, SBOM/license scan pending remote, no recent NIST NVD pull.
🔴 **Block**: pre-public scrub verification (separate from this snapshot — see branch audit summary).

---

*Compliance snapshot forged 2026-04-27 from Cowork-on-Macbook. SBOM + license rows finalize on next Linux session.*
