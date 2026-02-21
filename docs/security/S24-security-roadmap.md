# Security Roadmap (S24 - Phase 7 - Production Hardening)

## Overview

This document outlines the security hardening completed in Phase 7 and the remaining priorities for Unheaded infrastructure. Security is a continuous process, and this roadmap tracks our progress from initial assessment (S22) through production deployment and beyond.

**Status**: In Progress - Phase 7 (Production Hardening)

---

## Completed in Phase 7 (P0 - RESOLVED)

The following critical security items from S22 have been resolved:

### 1. Static Linting Configuration (golangci-lint)
- **Status**: COMPLETED
- **Implementation**: `.golangci.yml` at repository root
- **Details**:
  - Enables 50+ linters covering code quality and security
  - Custom rules forbid `fmt.Println` in production code (allowed in `_test.go`)
  - Enforces error wrapping with `%w` format via `errorlint`
  - Configured for Go 1.24
  - Excludes test files from strict rules
  - Reasonable timeouts (10 minutes for full project)

### 2. Vulnerability Disclosure Policy
- **Status**: COMPLETED
- **Implementation**: `SECURITY.md` at repository root
- **Details**:
  - Supported versions table (current: 1.0.x)
  - Security contact: stevie@bellis.tech
  - 90-day disclosure timeline
  - Clear reporting format requirements
  - Distinction between in-scope (platform) and out-of-scope (customer apps)
  - Security best practices for deployment and configuration

### 3. Build Script with Version Metadata
- **Status**: COMPLETED
- **Implementation**: `scripts/build.sh`
- **Details**:
  - Injects version, commit hash, build date via `-ldflags`
  - Builds all Go cmd/ binaries with metadata
  - Optional Rust/eBPF builds (trace-collector, eBPF programs)
  - Color-coded logging and error reporting
  - Reproducible builds with `-trimpath` flag
  - Binary validation (executable check)

### 4. HTTP Server Security (MaxHeaderBytes)
- **Status**: COMPLETED
- **Implementation**: `pkg/http/server.go` and `cmd/dashboard-backend`
- **Details**:
  - HTTP servers configured with MaxHeaderBytes = 1MB (1 << 20 bytes)
  - Prevents header-based DoS attacks
  - ReadHeaderTimeout set to prevent slow-read attacks
  - Dashboard backend enforces 1MB body size limit on POST endpoints (line 659)
  - Rate limiting recommendations documented

### 5. Development Environment Flake (Nix)
- **Status**: COMPLETED
- **Implementation**: Root `flake.nix`
- **Details**:
  - Pinned dependencies for Go 1.24, Rust (stable), LLVM 18
  - Reproducible development environment
  - Includes eBPF build tools (bpf, clang, llvm)
  - Security tools: golangci-lint, gosec, gpg, git-crypt
  - Documentation and dev shell with helpful messages

### 6. HTTP Server Hardening Configuration
- **Status**: COMPLETED
- **Implementation**: `pkg/http/server.go`
- **Details**:
  - Read/Write/Idle timeout configurations
  - ReadHeaderTimeout: 10s (prevents Slowloris attacks)
  - Graceful shutdown with signal handling
  - Health check endpoints (/health, /health/live, /health/ready)
  - Metrics collection and monitoring
  - HTTP/2 support with TLS

### 7. Go Dependency Management
- **Status**: COMPLETED
- **Implementation**: `go.mod` at project root
- **Details**:
  - Pinned to Go 1.24.0
  - Dependencies frozen to specific versions
  - No external CVE-affected versions in current build
  - Local module replacements for code organization

### 8. eBPF Security Considerations
- **Status**: COMPLETED (Phase 6)
- **Implementation**: Capability restrictions, seccomp policies in NixOS containers
- **Details**:
  - eBPF programs compiled to known-safe targets
  - Loaded and pinned with elevated privileges only when needed
  - Network policies restrict access between containers

---

## Remaining Priority 1 (P1) Items

High-priority items that should be completed before production release:

### P1.1: Advanced Static Analysis (gosec)
- **Priority**: HIGH
- **Status**: PENDING
- **Description**: Integrate gosec (Go Security Scanner) into CI/CD pipeline
- **Action Items**:
  - Enable gosec in `.golangci.yml` (currently integrated)
  - Run: `gosec ./...` in CI/CD
  - Audit and fix flagged security issues
  - Document exceptions if needed
  - Add security advisories to documentation

### P1.2: Software Bill of Materials (SBOM)
- **Priority**: HIGH
- **Status**: PENDING
- **Description**: Generate and track SBOM for all releases
- **Action Items**:
  - Generate SBOM from go.mod and Cargo.lock
  - Format: SPDX (recommended) or CycloneDX
  - Include both Go and Rust dependencies
  - Automate generation in release process
  - Store SBOM with release artifacts
  - Set up CVE tracking for dependencies

### P1.3: HTTP Server Security Headers
- **Priority**: HIGH
- **Status**: PENDING
- **Description**: Enforce security headers on all HTTP responses
- **Action Items**:
  - Implement middleware in `pkg/http/server.go`:
    - Strict-Transport-Security (HSTS)
    - X-Content-Type-Options: nosniff
    - X-Frame-Options: DENY
    - Content-Security-Policy
    - X-XSS-Protection
  - Configure CORS properly (whitelist only trusted origins)
  - Add tests for header presence

### P1.4: Dependency Audit and Updates
- **Priority**: HIGH
- **Status**: PENDING
- **Description**: Establish dependency management and update procedures
- **Action Items**:
  - Run `go mod why -m github.com/...` for each major dependency
  - Create DEPENDENCIES.md documenting all external packages
  - Set up Dependabot or similar for automatic PR on updates
  - Review and test dependency updates monthly
  - Document rationale for held/pinned versions

### P1.5: Nix Flake Pinning and Reproducibility
- **Priority**: HIGH
- **Status**: PENDING
- **Description**: Ensure reproducible builds and track nixpkgs revisions
- **Action Items**:
  - Add flake.lock to version control
  - Document flake.lock update procedure
  - Test full reproducibility (same output from same inputs)
  - Consider using a specific nixpkgs revision instead of nixos-unstable
  - Document rationale for input pin strategy

### P1.6: Container Security Scanning
- **Priority**: MEDIUM-HIGH
- **Status**: PENDING
- **Description**: Scan NixOS container images for vulnerabilities
- **Action Items**:
  - Integrate Trivy or similar container scanner
  - Run scans on built container images
  - Document findings and fix procedures
  - Block release if HIGH/CRITICAL vulnerabilities present
  - Track vulnerability lifecycle

---

## Remaining Priority 2 (P2) Items

Medium-priority items for future phases:

### P2.1: Runtime Security Monitoring
- **Description**: Deploy runtime security monitoring using eBPF and Falco
- **Details**:
  - Monitor system calls for suspicious activity
  - Detect privilege escalation attempts
  - Track unauthorized file access
  - Alert on anomalous network behavior
- **Target Phase**: Phase 8+

### P2.2: Post-Quantum Cryptography (PQC) Key Rotation
- **Description**: Evaluate and implement PQC algorithms for future resistance
- **Details**:
  - Research NIST-approved PQC algorithms
  - Implement hybrid key exchange (classical + PQC)
  - Plan for future key rotation procedures
  - Monitor standardization progress
- **Target Phase**: Phase 9+

### P2.3: Advanced Threat Modeling
- **Description**: Perform formal threat modeling and penetration testing
- **Details**:
  - STRIDE threat modeling for each service
  - Attack tree analysis
  - Engage security researchers for external audit
  - Remediate findings from audit
- **Target Phase**: Phase 8+ (pre-production)

### P2.4: Secret Management System
- **Description**: Implement centralized secrets management
- **Details**:
  - Integration with HashiCorp Vault or similar
  - Automatic secret rotation
  - Audit logging for secret access
  - Emergency secret revocation procedures
- **Target Phase**: Phase 8

### P2.5: Security Event Logging and SIEM Integration
- **Description**: Centralized logging with SIEM integration
- **Details**:
  - Collect security events from all services
  - Forward to SIEM system (Splunk, ELK, etc.)
  - Create alerts for suspicious patterns
  - Maintain audit trail for compliance
- **Target Phase**: Phase 8

---

## Current Implementation Status

### Completed Security Controls

| Control | Status | Implementation | Evidence |
|---------|--------|-----------------|----------|
| Static Analysis (golangci-lint) | ✅ COMPLETE | `.golangci.yml` | Line 277 of Makefile |
| Vulnerability Disclosure | ✅ COMPLETE | `SECURITY.md` | Root level |
| Build Metadata Injection | ✅ COMPLETE | `scripts/build.sh` | Uses `-ldflags` |
| HTTP Header Limits | ✅ COMPLETE | `pkg/http/server.go` line 86, 301 | MaxHeaderBytes = 1MB |
| Go Version Pinning | ✅ COMPLETE | `go.mod` line 3 | Go 1.24.0 |
| Development Shell | ✅ COMPLETE | Root `flake.nix` | Reproducible envs |
| eBPF Container Security | ✅ COMPLETE | `nix/containers/*.nix` | Capability restrictions |

### Pending Security Controls

| Control | Priority | Status | Timeline |
|---------|----------|--------|----------|
| gosec Integration | P1 | PENDING | Phase 7 (ongoing) |
| SBOM Generation | P1 | PENDING | Phase 7 (ongoing) |
| Security Headers | P1 | PENDING | Phase 7 (ongoing) |
| Dependency Audit | P1 | PENDING | Phase 7 (ongoing) |
| Container Scanning | P1 | PENDING | Phase 7 (ongoing) |
| PQC Research | P2 | PENDING | Phase 8+ |
| Runtime Monitoring | P2 | PENDING | Phase 8+ |

---

## Security Best Practices Summary

### For Developers

1. **Code Quality**: Run `make lint` before committing
2. **Error Handling**: Always use `fmt.Errorf(..., %w)` for error wrapping
3. **No Debug Prints**: Use logging instead of `fmt.Println`
4. **Testing**: Write security-focused tests for authentication, authorization
5. **Dependencies**: Keep dependencies up-to-date, review transitive deps

### For Deployment

1. **TLS Everywhere**: Use TLS 1.2+ for all network communication
2. **Minimal Privileges**: Run services with CAP_DROP and read-only filesystem
3. **Network Isolation**: Implement network policies between services
4. **Monitoring**: Enable audit logging and centralized log collection
5. **Secrets**: Use secure vaults, never commit secrets to git

### For Administrators

1. **Keep Updated**: Monitor security advisories and apply patches
2. **Audit Logs**: Enable and monitor audit logging
3. **Capacity Planning**: Monitor resource usage for DoS detection
4. **Incident Response**: Have a plan for security incidents
5. **Compliance**: Verify compliance with security policies periodically

---

## References

- **Go Security**: https://golang.org/doc/fuzz
- **OWASP Top 10**: https://owasp.org/www-project-top-ten/
- **CWE/CVSS**: https://cwe.mitre.org/
- **NixOS Security**: https://nixos.wiki/wiki/Security
- **eBPF Security**: https://ebpf.io/what-is-ebpf/

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-20 | Phase 7 | Initial security roadmap from S22/S24 |

---

**Status**: Updated 2026-02-20
**Next Review**: Phase 8 (Post-Production)
**Maintainer**: Security Team
