# Unheaded Project Software Bill of Materials (SBOM)

**Generated:** 2026-02-27
**Repository:** unheaded (Go 1.24 + Rust + eBPF)
**Version:** v0.1.0-alpha-186-g540a552-dirty

## Summary

**Go Modules:** 96 direct + transitive dependencies

## License Compliance

### Main Codebase
- Primary license: **MIT**
- Change Date: 2029-12-31
- Change License: Apache-2.0

### Protocol Specifications
- License: **MIT** (primary) or **Apache-2.0** (alternative)
- Scope: /docs/protocol/ directory only

### DOOM Engine (GPL Boundary)
- License: **GPL v2.0** (id Software original)
- Scope: /doom/ subdirectory only
- Status: Architecturally isolated via eBPF VM sandbox

### Go Dependencies
- All use permissive licenses (MIT, Apache-2.0, BSD)
- License check: PASSED (see go-licenses-report.txt)

### Rust Dependencies
- All use MIT or Apache-2.0 dual licensing
- Audit report: cargo-audit-report.txt

## Generated Artifacts

- **sbom-cyclonedx.json** — CycloneDX format (if Syft available)
- **sbom-spdx.json** — SPDX format (if Syft available)
- **go-modules-list.txt** — Go module inventory with versions
- **go-licenses-report.txt** — Go license compliance check
- **cargo-audit-report.txt** — Rust security audit
- **SBOM-SUMMARY.md** — This file

## References

- [THIRD_PARTY.md](../../THIRD_PARTY.md) — Detailed dependency inventory
- [LICENSE](../../LICENSE) — MIT license for main codebase
- [LICENSE-PROTOCOLS](../../LICENSE-PROTOCOLS) — Protocol specifications licensing

**Generated:** 2026-02-27T04:47:27Z
