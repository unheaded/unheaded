# Unheaded Project Software Bill of Materials (SBOM)

**Generated:** 2026-02-24
**Repository:** unheaded (Go 1.24 + Rust + eBPF)
**Scope:** Production code + test code (~260K + ~204K LOC)

## Summary

This document consolidates SBOM findings. External SBOM tools (ScanCode,
FOSSology, ORT) were not available at scan time; manual inspection and
Go module analysis were used as fallback.

## Key Findings

### Main Codebase Licensing (MIT)

- Primary license: MIT License
- Change Date: 2029-12-31
- Change License: Apache 2.0
- Status: All key source files have MIT headers

### Protocol Specifications (MIT/Apache-2.0)

- License: MIT (primary) or Apache-2.0 (alternative)
- Scope: /docs/protocol/ directory only
- Status: Separate LICENSE-PROTOCOLS file created

### DOOM Engine (GPL 2.0)

- License: GPL 2.0 or later (id Software original)
- Scope: /doom/ subdirectory only
- Status: GPL boundary established, separate LICENSE file

### Go Module Dependencies (91 modules)

All Go dependencies use permissive licenses (MIT, Apache 2.0, BSD).
Full list: sbom-results/go-modules-list.txt and LICENSES/sbom-go-modules.txt

### Rust Crate Dependencies

Rust crates (aya, tokio, tonic, etc.) use MIT/Apache 2.0 dual licensing.
See: LICENSES/THIRD_PARTY.md for full attribution.

## Third-Party License Compliance

All third-party licenses are documented in:
- /LICENSES/THIRD_PARTY.md (overview with full attribution)
- /LICENSES/sbom-go-modules.txt (Go modules list)
- sbom-results/ (scan outputs)

### License Categories

| Category | Status | Notes |
|----------|--------|-------|
| MIT | Compatible | Most common permissive license |
| Apache-2.0 | Compatible | Second most common |
| BSD-2/BSD-3 | Compatible | Standard permissive |
| ISC | Compatible | Permissive variant |
| GPL-2.0 | Isolated | Only in /doom/ subdirectory |
| AGPL-3.0 | None found | No AGPL dependencies |
| GPL-3.0 | None found | No GPL-3.0 dependencies in main code |

## Scanning Methodology

1. Manual file scan: `find . -name "LICENSE*"` across repository
2. Go module analysis: `go list -m all` for dependency enumeration
3. Rust crate review: Cargo.toml inspection for license fields
4. SPDX template: sbom-results/ort-sbom.spdx.json (manual)

## Next Steps

1. Install and run ScanCode, FOSSology, ORT for automated verification
2. Set up CI/CD SBOM generation pipeline
3. Monitor for license compliance issues in new dependencies
