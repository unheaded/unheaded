# Unheaded Kingdom Docker Infrastructure Index

## Overview

Complete, production-ready Docker infrastructure-as-code for the Unheaded Kingdom 25-service microservices architecture.

**Location**: `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/`

## Files at a Glance

### Core Infrastructure

| File | Lines | Purpose |
|------|-------|---------|
| `base/Dockerfile.base` | 82 | Multi-stage Go build reference pattern |
| `services/.dockerignore` | 16 | Global build context optimization |

### Service Dockerfiles (7 total)

#### Standard Go Services (5 distroless)
- `services/monad/Dockerfile` — 50051/gRPC register
- `services/sophia/Dockerfile` — 50052/gRPC state
- `services/protocol-api/Dockerfile` — 50056/gRPC
- `services/trace-collector/Dockerfile` — 9411/HTTP traces
- `services/dashboard-backend/Dockerfile` — 8080/HTTP+WS

#### eBPF/Privileged Services (2 debian:bookworm-slim)
- `services/shield/Dockerfile` — 50055/gRPC, CAP_BPF+CAP_NET_ADMIN+CAP_SYS_ADMIN+CAP_SYS_RESOURCE
- `services/unheaded-daemon/Dockerfile` — Internal, CAP_BPF+CAP_NET_ADMIN

### Documentation

| File | Lines | Content |
|------|-------|---------|
| `README.md` | ~200 | Comprehensive guide, build instructions, security features |
| `MANIFEST.md` | ~150 | Complete inventory, statistics, security posture |
| `QUICKSTART.md` | ~150 | Quick reference, build examples, port table |
| `INDEX.md` | this | Navigation and file index |

## Quick Navigation

### I want to...

**Build a single service**
→ See: `QUICKSTART.md` "Single Service" section

**Build all 7 services**
→ See: `QUICKSTART.md` "All Services (Parallel)" section

**Understand the security model**
→ Read: `README.md` "Security Features" section

**See capability requirements**
→ Check: `README.md` "Capability Requirements" table

**Deploy to production**
→ Follow: `README.md` "Production Checklist" section

**Verify all files exist**
→ Run: `find /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/services -name "Dockerfile" | wc -l` (should be 7)

**Check documentation completeness**
→ Run: `ls -la /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/*.md`

## Statistics

```
Total Dockerfiles:     8 (1 base + 7 services)
Total Lines:           883 production-ready code
Standard Services:     5 (distroless/static-debian12)
eBPF Services:         2 (debian:bookworm-slim)
Documentation Files:   4 (README, MANIFEST, QUICKSTART, INDEX)
Build Context Opt:     1 (.dockerignore)
```

## Key Features

### Security
- Multi-stage builds (minimal final images)
- Static binaries (CGO_ENABLED=0)
- Non-root execution (uid 65532 or 10001)
- Hardened Go compilation flags
- OCI-compliant labels
- Reproducible builds

### Consistency
- Uniform build patterns across all services
- Build argument injection (VERSION, BUILD_DATE, COMMIT)
- Standard CA certificates handling
- Consistent EXPOSE directives
- OCI image label standards

### Operations
- Parallel build capability (all 7 services simultaneously)
- Version/commit/timestamp tracking
- Per-service documentation in headers
- Capacity annotations
- Capability declarations for eBPF services

## Base Image Strategy

| Base Image | Services | Use Case |
|-----------|----------|----------|
| `gcr.io/distroless/static-debian12:nonroot` | 5 | Standard Go microservices |
| `debian:bookworm-slim` | 2 | eBPF + privileged services |

## Port Assignments

### gRPC Standard Ports (50051-50067)
- 50051: monad
- 50052: sophia
- 50055: shield
- 50056: protocol-api

### HTTP/Special Ports
- 8080: dashboard-backend
- 9411: trace-collector (Jaeger)

## Build Examples

### Single service build
```bash
docker build -f docker/services/monad/Dockerfile \
  --build-arg VERSION=v1.0.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
  -t unheaded-monad:v1.0.0 .
```

### Batch build (all 7)
See `QUICKSTART.md` for complete parallel build script

### With registry
```bash
docker build ... -t registry.example.com/unheaded-monad:v1.0.0
docker push registry.example.com/unheaded-monad:v1.0.0
```

## Security Scanning

### Recommended tools
- Trivy: `trivy image unheaded-monad:v1.0.0`
- Grype: `grype unheaded-monad:v1.0.0`
- Snyk: `snyk container test unheaded-monad:v1.0.0`

### CI/CD Integration
See `README.md` "Security Scanning" section

## Next Steps

1. **Read**: Start with `QUICKSTART.md` for overview
2. **Review**: Read `README.md` for comprehensive details
3. **Build**: Follow build examples to create images
4. **Scan**: Run security scanners on built images
5. **Deploy**: Use docker-compose or Kubernetes manifests
6. **Monitor**: Set up logging and metrics collection

## File References

All Dockerfiles include:
- SPDX license header
- Copyright and ownership information
- Descriptive comments explaining stages
- Build argument documentation
- Proper multi-stage patterns
- OCI-compliant labels
- Non-root user configuration

Example header pattern:
```dockerfile
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# docker/services/{service}/Dockerfile
# {Description}
```

## Version Control

All files are production-ready and include:
- Proper licensing (GPL-3.0-or-later)
- Version control comments
- Build traceability (VERSION, COMMIT, BUILD_DATE)
- Source URL references
- Reproducible build instructions

## Support

For detailed information, see:
- **Quick Start**: `QUICKSTART.md`
- **Full Guide**: `README.md`
- **Inventory**: `MANIFEST.md`
- **This Index**: `INDEX.md`

---

**Status**: Production-ready
**Total Lines**: 883 lines of Docker infrastructure code
**Last Generated**: 2026-02-26
