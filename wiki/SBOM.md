# SBOM — Software Bill of Materials

Comprehensive dependency tracking, license compliance, and supply chain security for the Unheaded platform.

## Overview

- **553 dependencies** audited across Go, Rust, and JavaScript
- **GPL boundary enforcement** — GPL-licensed code (doomgeneric) is isolated and does not link with Unheaded code
- **SPDX checks** — all dependencies have verified SPDX license identifiers
- **Automated scanning** — CI pipeline validates SBOM on every build

## Dependency Breakdown

| Ecosystem | Dependencies | Primary License |
|-----------|-------------|-----------------|
| Go modules | ~350 | MIT, Apache-2.0, BSD-3-Clause |
| Rust crates | ~150 | MIT, Apache-2.0 |
| JavaScript (npm) | ~50 | MIT |
| System (libbpf, etc.) | ~3 | LGPL-2.1, GPL-2.0 (isolated) |

## GPL Boundary

The GPL boundary is strictly enforced:

- **doomgeneric** (GPL-2.0) lives in `doom/doomgeneric/` and is compiled as a standalone binary
- No Unheaded library code links against GPL code
- eBPF programs use MIT/Apache-2.0 licensed Aya framework
- Build system enforces boundary via separate compilation units

## SPDX Compliance

Every dependency is verified against its declared SPDX identifier:

- `SPDX-License-Identifier` headers checked in all source files
- License compatibility matrix maintained for transitive dependencies
- Incompatible licenses flagged and blocked in CI

## Audit Process

1. **Automated scan** — `go mod tidy`, `cargo audit`, `npm audit` run in CI
2. **License extraction** — SPDX identifiers collected and validated
3. **GPL boundary check** — linker analysis confirms no GPL contamination
4. **Vulnerability scan** — CVE databases checked for known issues
5. **Manual review** — new dependencies require explicit approval

## Output Formats

- **SPDX 2.3** — machine-readable SBOM for compliance tooling
- **CycloneDX 1.5** — for security scanning integration
- **Human-readable** — summarized in `LICENSES/THIRD_PARTY.md`

---

*Last updated: March 17, 2026*
