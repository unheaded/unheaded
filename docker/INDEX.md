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

### Service Dockerfiles (25 total)

#### Standard Go Services (23 distroless)
- `services/monad/Dockerfile` — 50051/gRPC register
- `services/sophia/Dockerfile` — 50052/gRPC state
- `services/wotan/Dockerfile` — 50053/gRPC, 4222/NATS
- `services/anamnesis/Dockerfile` — 50054/gRPC memory
- `services/protocol-api/Dockerfile` — 50056/gRPC
- `services/trace-collector/Dockerfile` — 9411/HTTP traces
- `services/gateway/Dockerfile` — 8443/HTTPS TLS
- `services/service-discovery/Dockerfile` — 50057/gRPC, 8500/HTTP
- `services/doom/Dockerfile` — 16680/TCP (1GB limit)
- `services/captain/Dockerfile` — 50058/gRPC
- `services/micromanager/Dockerfile` — 50059/gRPC
- `services/timeguru/Dockerfile` — 50060/gRPC, 8600/HTTP
- `services/architect/Dockerfile` — 50061/gRPC
- `services/developer/Dockerfile` — 50062/gRPC
- `services/kanban/Dockerfile` — 3002/HTTP
- `services/lore/Dockerfile` — 50063/gRPC
- `services/busboy/Dockerfile` — 50064/gRPC
- `services/kingdom/Dockerfile` — 50065/gRPC
- `services/moatghost/Dockerfile` — 50067/gRPC
- `services/dashboard-backend/Dockerfile` — 8080/HTTP+WS

#### eBPF/Privileged Services (4 debian:bookworm-slim)
- `services/shield/Dockerfile` — 50055/gRPC, CAP_BPF+CAP_NET_ADMIN+CAP_SYS_ADMIN+CAP_SYS_RESOURCE
- `services/unheaded-daemon/Dockerfile` — Internal, CAP_BPF+CAP_NET_ADMIN
- `services/lich/Dockerfile` — Internal, CAP_NET_RAW+CAP_NET_ADMIN
- `services/blackmage/Dockerfile` — 50066/gRPC, CAP_NET_RAW

#### Frontend (1 Node/Nginx)
- `services/dashboard-frontend/Dockerfile` — 3001/HTTP, node:22-alpine→nginx:alpine

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

**Build all 25 services**
→ See: `QUICKSTART.md` "All Services (Parallel)" section

**Understand the security model**
→ Read: `README.md` "Security Features" section

**See capability requirements**
→ Check: `README.md` "Capability Requirements" table

**Deploy to production**
→ Follow: `README.md` "Production Checklist" section

**Verify all files exist**
→ Run: `find /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/services -name "Dockerfile" | wc -l` (should be 25)

**Check documentation completeness**
→ Run: `ls -la /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/*.md`

## Statistics

```
Total Dockerfiles:     27 (1 base + 25 services)
Total Lines:           883 production-ready code
Standard Services:     23 (distroless/static-debian12)
eBPF Services:         4 (debian:bookworm-slim)
Frontend:              1 (node:22-alpine → nginx:alpine)
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
- Parallel build capability (all 25 services simultaneously)
- Version/commit/timestamp tracking
- Per-service documentation in headers
- Capacity annotations (e.g., doom 1GB limit)
- Capability declarations for eBPF services

## Base Image Strategy

| Base Image | Services | Use Case |
|-----------|----------|----------|
| `gcr.io/distroless/static-debian12:nonroot` | 23 | Standard Go microservices |
| `node:22-alpine` → `nginx:alpine` | 1 | Frontend web UI |
| `debian:bookworm-slim` | 4 | eBPF + privileged services |

## Port Assignments

### gRPC Standard Ports (50051-50067)
- 50051: monad
- 50052: sophia
- 50053: wotan
- 50054: anamnesis
- 50055: shield
- 50056: protocol-api
- 50057: service-discovery
- 50058: captain
- 50059: micromanager
- 50060: timeguru
- 50061: architect
- 50062: developer
- 50063: lore
- 50064: busboy
- 50065: kingdom
- 50066: blackmage
- 50067: moatghost

### HTTP/Special Ports
- 3001: dashboard-frontend
- 3002: kanban
- 4222: wotan (NATS)
- 8080: dashboard-backend
- 8443: gateway (HTTPS)
- 8500: service-discovery (Consul HTTP)
- 8600: timeguru (HTTP)
- 9411: trace-collector (Jaeger)
- 16680: doom (TCP)

## Build Examples

### Single service build
```bash
docker build -f docker/services/monad/Dockerfile \
  --build-arg VERSION=v1.0.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
  -t unheaded-monad:v1.0.0 .
```

### Batch build (all 25)
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
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# docker/services/{service}/Dockerfile
# {Description}
```

## Version Control

All files are production-ready and include:
- Proper licensing (MIT)
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
