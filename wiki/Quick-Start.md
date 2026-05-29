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
docker compose up -d             # Start the full stack
```

> If a host port is already taken (e.g. macOS Bonjour on 5353, or a host LB on 80/443),
> override it in a local `.env` — see Troubleshooting in [QUICKSTART.md](../QUICKSTART.md).

## Service ports (local defaults)

Open these in a browser:

| App | URL | Notes |
|-----|-----|-------|
| **Kanban board** | http://localhost:20001 | self-hosting cockpit |
| **Dashboard** | http://localhost:20000 | metrics + live packet-flow |
| **Grafana** | http://localhost:3001 | admin / unheaded |
| **Traefik dashboard** | http://localhost:21000 | edge proxy |

Kingdom service APIs (each exposes `/health`):

| Service | Port | Service | Port |
|---------|------|---------|------|
| Wotan | 18000 (HTTP) / 18001 (gRPC) | Monad | 19004 |
| Timeguru | 19000 | Sophia | 19005 |
| Architect | 19001 | Cuirass | 19006 |
| Captain | 19002 | Dashboard | 20000 |
| Micromanager | 19003 | Kanban | 20001 |

Infra: VictoriaMetrics `8428` · ClickHouse `8123`/`9000` · Postgres `5432` · CoreDNS `5353` · Traefik edge `80`/`443`/`50051`.

## Verify

```bash
# All Kingdom services should return 200 OK on /health
for p in 18000 19000 19001 19002 19003 19004 19005 19006 20000 20001; do
  printf "localhost:%-5s -> " "$p"; curl -sf "http://localhost:$p/health" && echo || echo DOWN
done
```

---

> **Source:** [QUICKSTART.md](../QUICKSTART.md)
