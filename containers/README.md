# Unheaded Container Runtimes

Four interchangeable container runtimes, one security baseline.

## Supported Runtimes

| Runtime | Location | Use Case | Maturity |
|---------|----------|----------|----------|
| **NixOS** | `nix/containers/` | Production reference, reproducible builds | Primary |
| **Docker** | `Dockerfile` + `docker-compose.yml` (repo root) | Local development, CI/CD | Stable |
| **containerd** | `containers/containerd/` | Kubernetes integration, CRI-compatible | Stub |
| **LXD** | `containers/lxd/` | Bare-metal VM-style isolation, system containers | Stub |

## When to Use Which

### NixOS (Production Reference)
- Reproducible, declarative builds with full hardening
- Immutable root filesystem by default
- Source of truth for security baseline
- Requires NixOS host or `nixos-container` tooling

### Docker (Development)
- Fastest to get started: `docker compose up -d`
- Multi-stage build for all services in one Dockerfile
- Traefik gateway with service discovery
- Observability stack included (VictoriaMetrics, ClickHouse, Grafana)

### containerd (Kubernetes / CRI)
- OCI runtime specs for each service
- Direct integration with Kubernetes CRI
- Minimal daemon overhead
- Use when running on k8s or with `ctr` / `nerdctl`

### LXD (Bare Metal / System Containers)
- Full OS containers (not app containers)
- Closest to VM-level isolation without hypervisor overhead
- Cloud-init provisioning for service installation
- Static IP on `lxdbr0` bridge (10.10.10.0/24)

## Network Topology (All Runtimes)

All four runtimes configure the same network layout:

```
Bridge: lxdbr0 / br-unheaded (10.10.10.0/24)

+-- Gateway --------- 10.10.10.100 (nginx, ports 80/443)
|
+-- Wotan ----------- 10.10.10.10  (message bus, gRPC 9090 + REST 8080)
+-- Cuirass --------- 10.10.10.5   (control plane, port 8005)
|
+-- Timeguru -------- 10.10.10.20  (timeline, port 8000)
+-- Captain --------- 10.10.10.21  (strategy, port 8001)
+-- Micromanager ---- 10.10.10.22  (execution, port 8002)
+-- Architect ------- 10.10.10.23  (design, port 8003)
+-- Monad ----------- 10.10.10.27  (state mgmt, port 8004)
+-- Sophia ---------- 10.10.10.26  (knowledge graph, port 8005)
+-- Doom Bridge ----- 10.10.10.28  (Fenrir's Eye, port 6660)
+-- Wiki Server ----- 10.10.10.29  (documentation, port 8007)
|
+-- Kanban App ------ 10.10.10.200 (meta moment, port 8080)
+-- Dashboard ------- 10.10.10.201 (metrics+WS, port 8081)
```

## Security Baseline (All Runtimes)

Every runtime enforces the same security properties:

- **No new privileges**: Processes cannot gain elevated permissions
- **Read-only root filesystem**: Only explicitly writable paths are mutable
- **Minimal capabilities**: Only `CAP_NET_BIND_SERVICE` where needed
- **Non-root user**: Services run as `unheaded` (UID 1000)
- **Resource limits**: Memory and CPU caps per service
- **Network isolation**: Default-deny firewall, explicit port allowlists
- **Health checks**: Every service exposes `/health` endpoint
- **Seccomp filtering**: Dangerous syscalls blocked (NixOS strict profile)

## Quick Start

### Docker (recommended for development)

```bash
# Full stack
docker compose up -d

# Check status
docker compose ps

# Tail logs
docker compose logs -f timeguru
```

### containerd

```bash
# Pull images
for svc in wotan timeguru captain architect micromanager monad sophia; do
  ctr images pull ghcr.io/unheaded/$svc:latest
done

# Create container from OCI spec
ctr run --detach \
  --net-host \
  ghcr.io/unheaded/timeguru:latest \
  timeguru

# Or use nerdctl with the spec files
nerdctl create --name timeguru \
  --label io.containerd.config="containers/containerd/services/timeguru.json" \
  ghcr.io/unheaded/timeguru:latest
```

### LXD

```bash
# Import profiles
lxc profile create unheaded-base < containers/lxd/profiles/unheaded-base.yaml
lxc profile create unheaded-service < containers/lxd/profiles/unheaded-service.yaml

# Launch an instance
lxc launch ubuntu:22.04 timeguru \
  --profile unheaded-base \
  --profile unheaded-service \
  --config=user.user-data="$(cat containers/lxd/instances/timeguru.yaml)"

# Or use the instance YAML directly
lxc init ubuntu:22.04 timeguru < containers/lxd/instances/timeguru.yaml
```

### NixOS

```bash
# Build all containers
nix build .#containers

# Start a container
sudo nixos-container start timeguru

# Check status
sudo nixos-container status timeguru
```

## Environment Variables (All Runtimes)

Every service receives these standard environment variables:

| Variable | Example | Description |
|----------|---------|-------------|
| `SERVICE_NAME` | `timeguru` | Service identifier |
| `WOTAN_ADDR` | `10.10.10.10:9090` | Wotan gRPC address |
| `WOTAN_HTTP_ADDR` | `http://10.10.10.10:8080` | Wotan REST address |
| `LOG_LEVEL` | `info` | Logging verbosity |
| `METRICS_ADDR` | `0.0.0.0:9100` | Prometheus metrics bind |
| `GOGC` | `100` | Go GC tuning |
| `GOMAXPROCS` | `2` | Go runtime parallelism |

## Adding a New Service

1. Add NixOS container definition in `nix/containers/`
2. Add Docker stage in `Dockerfile` and service in `docker-compose.yml`
3. Add containerd OCI spec in `containers/containerd/services/`
4. Add LXD instance YAML in `containers/lxd/instances/`
5. Assign static IP from the appropriate range
6. Update gateway routing in all four runtimes
