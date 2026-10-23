# SBOM — Software Bill of Materials

Dependency tracking, license compliance and supply-chain scanning for Unheaded. Unheaded is GPL-3.0-or-later; the protocol specs are dual GPL-3.0 / Apache-2.0 (`LICENSE-PROTOCOLS`); the eBPF workspace is GPL-2.0-only, as the kernel requires.

## Dependency Breakdown

| Ecosystem | Manifest | Primary licenses |
|-----------|----------|------------------|
| Go modules | `go.mod` (list: `sbom/go-dependencies.json`, `LICENSES/sbom-go-modules.txt`) | MIT, Apache-2.0, BSD-3-Clause |
| Rust crates | `ebpf/`, `crates/*/Cargo.toml` (`sbom/rust-dependencies.json`) | MIT, Apache-2.0 |
| Python | `sbom/python-requirements.txt` (Zhen / RAFT tooling) | mixed permissive |
| System (libbpf, etc.) | — | LGPL-2.1, GPL-2.0 |

There is no npm in the repo: the front ends are vanilla JS (ADR-006) and JS lint is oxlint.

The March 2026 audit counted 553 dependencies; that number included npm packages since removed and is not regenerated.

## GPL Boundary

`scripts/verify-gpl-boundary.sh` (workflow `gpl-boundary.yml`) classifies every third-party Go module and Rust crate license as permissive, LGPL, GPL, GPL-2.0-only or AGPL:

- First-party manifests are skipped: they are GPL by project license.
- **GPL-2.0-only dependencies are flagged**: they cannot be combined with GPL-3.0-or-later code. GPL-2.0-or-later is fine (3.0 is elected). A bare `GPL-2.0` counts as -only.
- The classifier has its own test, `scripts/test-gpl-boundary-classifier.sh`.

`sbom-audit.yml` also runs `go-licenses check ./...` with an allow-list of MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause and ISC, ignoring this module, `github.com/unheaded/doomgeneric` (first-party, fetched by `scripts/ci-fetch-doomgeneric.sh` as the `go.mod` replace target) and `modernc.org/mathutil` (BSD-3-Clause the classifier misses).

## SPDX Headers

Source files carry `SPDX-License-Identifier` headers; `ci.yml` (`spdx-check`) fails if a Go file lacks one.

## Audit Process

1. **SBOM** — `sbom.yml` (Anchore SBOM action, SPDX JSON) and `sbom-audit.yml` (`syft dir:.` → `sbom-results/sbom-cyclonedx.json` + `sbom-spdx.json`; syft and grype pinned through `scripts/ci-install-tools.sh`).
2. **Vulnerabilities** — grype on the SBOM (SARIF upload), `govulncheck ./...` and `cargo audit` per Rust workspace (`security.yml`), `scripts/check-python-audit.sh` for Python.
3. **Licenses** — the GPL boundary check and `go-licenses` above.
4. **Manual review** — new dependencies need explicit approval (ADR-004).

## Output Formats

- **SPDX JSON** and **CycloneDX JSON** from syft (`sbom-results/`)
- **Human-readable** — `LICENSES/THIRD_PARTY.md`

---

> **Source:** `.github/workflows/sbom.yml`, `sbom-audit.yml`, `gpl-boundary.yml`, `security.yml` · `scripts/verify-gpl-boundary.sh`
