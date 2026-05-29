# Quick Start Guide

Get Unheaded running locally in ~10 minutes.

## Prerequisites

| Tool | Version | Check |
|------|---------|-------|
| Go | 1.25+ | `go version` |
| Docker + Compose | 20.10+ | `docker compose version` |
| curl | any | `curl --version` |
| Git | any | `git --version` |

## Build

```bash
go build ./...   # or: make build
```

## Test

```bash
go test ./... -count=1           # Full suite
go test ./... -race -count=1     # With race detector
```

## Run

```bash
docker compose up -d             # Start core services
curl http://localhost:8000/health # Verify timeguru
curl http://localhost:8001/health # Verify captain
```

## Verify

Hit `/health` on all service ports. All should return `200 OK`.

---

> **Source:** [QUICKSTART.md](../QUICKSTART.md)
