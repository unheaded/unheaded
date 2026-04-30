# CI/CD Strategy - S77 Phase 3: SBOM + CI/CD Fortress

**Project:** Unheaded
**Date:** 2026-03-05
**Sprint:** S77

---

## CI/CD Pipeline Architecture

Unheaded uses a defense-in-depth CI/CD strategy with three execution environments:

1. **GitHub Actions (GHA)** - Primary CI for pull requests and main branch
2. **Jenkins** - Multi-branch pipeline for enterprise/self-hosted environments
3. **Local CI** - Developer workstation validation before push

All pipelines enforce the same quality gates: build, test, security scan, SBOM generation, and license compliance.

---

## GitHub Actions Workflows

| Workflow | File | Trigger | Purpose |
|----------|------|---------|---------|
| CI | `ci.yml` | Push to main/develop, PRs to main | Go build, test, lint, vet |
| Security | `security.yml` | Push to main, PRs, daily schedule | gosec, govulncheck, cargo-audit, Trivy |
| SBOM Audit | `sbom-audit.yml` | Weekly (Monday 9AM UTC), manual | SBOM generation (Syft), Grype vulnerability scan, license compliance |
| GPL Boundary | `gpl-boundary.yml` | Push to main, PRs to main | SPDX header scan, go.mod/Cargo.toml GPL/AGPL detection |
| Docker | `docker.yml` | Push/PR | Docker image builds |
| eBPF | `ebpf.yml` | Push/PR | eBPF Rust program builds |
| Helm | `helm.yml` | Push/PR | Helm chart validation |
| Release | `release.yml` | Tags | Release artifact creation |
| CI Protocol | `ci-protocol.yml` | Push/PR | Protocol-specific validation |

---

## Jenkins Pipeline Stages

The `Jenkinsfile` implements a declarative multi-branch pipeline:

| Stage | What It Does | Failure Impact |
|-------|-------------|----------------|
| Build | `go build ./...` | Blocks all downstream |
| Test | `go test ./... -race` | Blocks deploy |
| Security | gosec + govulncheck (parallel) | Blocks deploy |
| SBOM | `make sbom` | Advisory |
| License Check | `make license-check` + GPL boundary script | Blocks deploy |
| Deploy | `make deploy` (main branch only) | N/A on feature branches |

---

## Makefile Targets Reference

### Build
| Target | Description |
|--------|-------------|
| `build` | Build all Go binaries |
| `ebpf` | Build eBPF programs (Rust) |
| `all` | Build everything |

### Testing
| Target | Description |
|--------|-------------|
| `test` | Run all tests (Go + Rust) |
| `test-go` | Run Go tests with race detector |
| `test-rust` | Run Rust tests |
| `test-e2e` | Run E2E integration tests |

### Security & Compliance
| Target | Description |
|--------|-------------|
| `security` | Run gosec security audit |
| `sbom` | Generate SBOM (CycloneDX, SPDX, module list) |
| `license-check` | Check dependency licenses (MIT, Apache-2.0, BSD, ISC) |
| `sbom-verify` | Run GPL boundary verification |
| `ci-security` | Run all security scans (gosec + govulncheck + GPL boundary) |

### CI
| Target | Description |
|--------|-------------|
| `ci-local` | Run full CI pipeline locally (calls `scripts/ci-local.sh`) |

### Deployment
| Target | Description |
|--------|-------------|
| `deploy` | Build, test, and deploy via Docker Compose |
| `deploy-health` | Verify all services are healthy |

### Utilities
| Target | Description |
|--------|-------------|
| `fmt` | Format Go and Rust code |
| `lint` | Lint Go and Rust code |
| `clean` | Remove build artifacts |

---

## How to Run CI Locally

### Quick Start

```bash
# Run the full local CI pipeline
make ci-local

# Run security scans only
make ci-security

# Run GPL boundary check only
make sbom-verify
```

### Manual Steps

```bash
# Build
go build ./...

# Test with race detector
go test ./... -race

# Static analysis
go vet ./...

# Security (install tools first if needed)
go install github.com/securego/gosec/v2/cmd/gosec@v2.21.0
gosec ./...

go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...

# SBOM
make sbom

# License compliance
make license-check
scripts/verify-gpl-boundary.sh
```

### Prerequisites

- Go 1.24+
- Rust toolchain (for eBPF builds)
- Optional: gosec, govulncheck, go-licenses, cargo-license, Syft

---

## License Boundary Policy

Unheaded is MIT-licensed. All dependencies must use permissive licenses:

**Allowed:** MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC

**Blocked:** GPL-2.0, GPL-3.0, AGPL-3.0, LGPL (any version in core)

The `scripts/verify-gpl-boundary.sh` script enforces this boundary by scanning:
1. SPDX headers in all Go source files
2. go.mod dependencies for GPL/AGPL patterns
3. Cargo.toml files for GPL/AGPL license declarations

Reports are written to `sbom-results/gpl-boundary-report.txt`.
