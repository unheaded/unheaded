# Docker Runtime Configuration

Docker configs for the Unheaded development stack.

## Location

The Docker configuration lives at the repository root:
- `Dockerfile` -- Multi-stage build for all Go services
- `docker-compose.yml` -- Full development stack with observability

## Quick Start

```bash
# Start the full dev stack
docker compose up -d

# Check status
docker compose ps

# Tail logs for a specific service
docker compose logs -f timeguru

# Stop all services
docker compose down

# Stop and remove volumes (DESTRUCTIVE)
docker compose down -v
```

## Architecture

### Multi-stage Dockerfile

The `Dockerfile` uses a multi-stage build pattern:

1. **Builder stage** (`golang:1.24-alpine`): Compiles all Go services
2. **Per-service stages** (`alpine:3.19`): Minimal runtime images per service
3. **All-in-one stage** (default): Supervisor-managed single container for dev

Each per-service stage can be targeted with `--target`:

```bash
docker build --target wotan -t unheaded/wotan .
docker build --target timeguru -t unheaded/timeguru .
docker build --target captain -t unheaded/captain .
docker build --target architect -t unheaded/architect .
docker build --target micromanager -t unheaded/micromanager .
docker build --target monad -t unheaded/monad .
docker build --target sophia -t unheaded/sophia .
docker build --target cuirass -t unheaded/cuirass .
```

### docker-compose.yml

The compose file defines:

- **Networks**: Control plane, data plane, observability (isolated bridges)
- **Gateway**: Traefik v3.3 with HTTP/3 and service discovery
- **Metrics**: VictoriaMetrics (Prometheus-compatible)
- **Logging**: ClickHouse + Vector pipeline
- **Dashboard**: Grafana for dev visualization
- **DNS**: CoreDNS for service discovery

## Security

All service containers:
- Run as non-root user `unheaded` (UID 1000)
- Use health checks (30s interval)
- Have resource limits (memory + CPU)
- Data directories owned by service user

## Service Ports (Docker)

The Docker stages expose slightly different internal ports than the
production network layout. The Traefik gateway handles routing.

| Service | Internal Port | Production Port |
|---------|---------------|-----------------|
| wotan | 8080/9090 | 8080/9090 |
| timeguru | 8082 | 8000 |
| captain | 8083 | 8001 |
| architect | 8084 | 8003 |
| micromanager | 8085 | 8002 |
| monad | 8086 | 8004 |
| sophia | 8087 | 8005 |
| cuirass | 8080 | 8005 |

Note: The internal Docker ports differ from the production layout due
to the multi-stage build sharing a builder. Production deployments
should use the NixOS, containerd, or LXD configs which use the
canonical ports from the network topology.
