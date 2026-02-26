# Unheaded Kingdom — Docker Infrastructure

SPDX-License-Identifier: MIT
Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

## Overview

This directory contains the Docker-based deployment infrastructure for the Unheaded Kingdom project. Docker serves as an alternative to NixOS for containerized workload management across Host-A (Forge) and Host-B (Outpost).

**Both deployment approaches are available:**
- **Docker Compose** (this directory): Container-based, cgroup v2 resource management
- **NixOS** (separate): Declarative system configuration with systemd resource slices

Both approaches provide equivalent security posture, observability, and resource isolation—they differ only in the management layer.

## Architecture

### Network Topology

```
Host-A (Forge)                          Host-B (Outpost)
┌─────────────────────────────┐       ┌──────────────────────────┐
│                             │       │                          │
│  Docker Bridge Network      │       │  Docker Bridge Network   │
│  172.20.0.0/16              │       │  172.21.0.0/16           │
│  fd00:dead:cafe::/64        │       │  fd00:dead:cafe::/64     │
│                             │       │                          │
│  ┌────────────────────────┐ │       │ ┌──────────────────────┐ │
│  │ unheaded-wireguard     │ │       │ │ unheaded-wireguard   │ │
│  │ (Server)               │ │       │ │ (Client)             │ │
│  │ fd00:dead:beef::1/48   │◄├──────►├┤ fd00:dead:beef::2/48 │ │
│  │ 51820/udp              │ │       │ │ PersistentKeepalive  │ │
│  └────────────────────────┘ │       │ └──────────────────────┘ │
│           ▲                 │       │           ▲              │
│           │ (East-West)     │       │           │              │
│  ┌────────┴─────────────┐   │       │ ┌─────────┴──────────┐  │
│  │ 25 Services          │   │       │ │ Minimal Services   │  │
│  │ (Full Stack)         │   │       │ │ (Edge Compute)     │  │
│  └──────────────────────┘   │       │ └────────────────────┘  │
│                             │       │                          │
└─────────────────────────────┘       └──────────────────────────┘
```

- **East-West Bridge**: WireGuard tunnel over IPv6 (fd00:dead:beef::/48)
- **MTU**: 1380 bytes (IPv6-safe: 1500 - 20 IPv4 - 8 UDP - 32 WireGuard - 40 IPv6 inner)
- **Keepalive**: 25 seconds (client side only, prevents connection drops)

### Services

#### Host-A (Forge) — 25 Services
Complete deployment of the Unheaded Kingdom system:

**Core Infrastructure:**
- `monad` — Central coordination
- `sophia` — Knowledge graph
- `wotan` — Decision engine
- `anamnesis` — Memory/persistence
- `shield` — Network security (eBPF)
- `unheaded-daemon` — Core daemon (eBPF)

**API Layer:**
- `protocol-api` — gRPC/REST interface
- `gateway` — API gateway
- `service-discovery` — Service registry (Consul-compatible)

**Observability:**
- `trace-collector` — Distributed trace collection
- `dashboard-backend` — Metrics API
- `dashboard-frontend` — Web UI

**Application Services:**
- `doom`, `lich`, `captain`, `micromanager`, `timeguru`
- `architect`, `developer`, `kanban`, `lore`, `busboy`
- `kingdom`, `blackmage`, `moatghost`

#### Host-B (Outpost) — Minimal Suite
Edge compute deployment with critical services only:
- WireGuard (east-west bridge)
- vLLM/ROCm (LLM inference)
- Selected application services

## Prerequisites

### System Requirements

**Docker & Runtime:**
- Docker 24.0+ ([installation](https://docs.docker.com/engine/install/))
- Docker Compose v2.20+ (included with Docker Desktop 4.12+)
- Linux kernel 6.0+ (for eBPF services in Host-A)

**Hardware (Host-B with GPU inference):**
- AMD RX 7700 XT (gfx1101) or compatible RDNA2+ GPU
- 16GB+ RAM (recommended 24GB for vLLM with 4GB shm)
- 8+ CPU cores

**Cgroups v2 (recommended for resource limits):**
```bash
# Check cgroups version
mount | grep cgroup2
# Expected: cgroup2 on /sys/fs/cgroup type cgroup2 ...
```

**IPv6 Support:**
```bash
# Verify IPv6 routing is enabled
sysctl net.ipv6.conf.all.forwarding
# Should return: 1
```

### Pre-Deployment Checklist

```bash
cd /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker
make preflight
```

Verifies:
- Docker installation and version
- Docker Compose version
- Kernel version (6.0+)
- Cgroups v2 support
- IPv6 configuration
- ROCm device availability (/dev/kfd)

## Quick Start

### 1. Build All Images

```bash
cd /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker

# Build all 25 service images
make build

# Or build single service
make build-svc SERVICE=monad

# Build vLLM/ROCm image (GPU inference)
make build-vllm
```

**Build times:**
- Full build (25 services): ~30-45 minutes (depends on layer caching)
- Single service: ~2-5 minutes
- vLLM/ROCm: ~5-10 minutes (includes PyTorch + ROCm stack)

### 2. Environment Configuration

Copy `.env.shared` to both host directories:

```bash
cp .env.shared hosts/host-a/.env.shared
cp .env.shared hosts/host-b/.env.shared
```

Edit host-specific overrides in:
- `hosts/host-a/.env` (Forge configuration)
- `hosts/host-b/.env` (Outpost configuration)

### 3. Deploy Host-A (Forge)

```bash
# Start all 25 services
make up-forge

# Verify health
make status
```

Expected output:
```
→ Unheaded service status:
unheaded-monad                  Up 2 minutes
unheaded-sophia                 Up 2 minutes
...
unheaded-wireguard              Up 2 minutes
```

### 4. Deploy Host-B (Outpost)

On Host-B system:

```bash
# Start edge compute services
make up-outpost

# Verify WireGuard connection
docker exec unheaded-wireguard wg show all peers
```

Expected: Client peer shows `fd00:dead:beef::1/128` with recent handshake.

### 5. Verify East-West Connectivity

From Host-A:
```bash
docker exec unheaded-wireguard ping -6 -c 3 fd00:dead:beef::2
# Expected: 3 replies from fd00:dead:beef::2
```

From Host-B:
```bash
docker exec unheaded-wireguard ping -6 -c 3 fd00:dead:beef::1
# Expected: 3 replies from fd00:dead:beef::1
```

## Environment Variables

### Shared Variables (.env.shared)

```env
# Project configuration
COMPOSE_PROJECT_NAME=unheaded
VERSION=dev

# Logging
UNHEADED_LOG_LEVEL=info|debug|warn|error
UNHEADED_TRACE_ENABLED=true|false
UNHEADED_METRICS_ENABLED=true|false

# Network
UNHEADED_IPV6_PREFIX=fd00:dead:beef::/48
UNHEADED_NETWORK_MTU=1380

# vLLM/ROCm
VLLM_MODEL=/models/deepseek-r1-7b-q4
VLLM_GPU_MEMORY_UTILIZATION=0.90  # 0.0-1.0
VLLM_MAX_MODEL_LEN=8192
```

### Host-Specific Variables

**Host-A (hosts/host-a/.env):**
```env
# Forge-specific overrides
UNHEADED_ROLE=primary
UNHEADED_LOG_LEVEL=info
VLLM_ENABLED=true
```

**Host-B (hosts/host-b/.env):**
```env
# Outpost-specific overrides
UNHEADED_ROLE=edge
UNHEADED_LOG_LEVEL=debug
VLLM_ENABLED=true
WG_SERVERURL=<host-a-ip-or-fqdn>  # For client configuration
```

## Build & Deployment

### Makefile Targets

**Building:**
```bash
make build                          # Build all 25 services
make build-svc SERVICE=monad        # Build single service
make build-vllm                     # Build vLLM/ROCm image
```

**Running:**
```bash
make up-forge                       # Start Host-A
make up-outpost                     # Start Host-B
make down                           # Stop all services
make status                         # Show container health
make logs SERVICE=monad             # Follow service logs
```

**Registry:**
```bash
make push                           # Push to ghcr.io
make pull                           # Pull from registry
REGISTRY=myregistry make push       # Use custom registry
```

**Maintenance:**
```bash
make preflight                      # Pre-deployment checks
make clean                          # Remove all images (careful!)
```

### Manual Image Build

```bash
# Build vLLM/ROCm
cd /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/vllm-rocm
docker build \
  --build-arg ROCM_VERSION=6.1.3 \
  --build-arg VLLM_VERSION=0.4.3 \
  -t unheaded-vllm-rocm:dev \
  .

# Build individual service
cd /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker
docker build \
  --build-arg VERSION=dev \
  --build-arg SERVICE=monad \
  -f services/monad/Dockerfile \
  -t unheaded-monad:dev \
  ../
```

## Network Configuration

### WireGuard Setup

WireGuard configurations are templated and require key generation:

**Generate keys on Host-A:**
```bash
cd docker/wireguard/config
wg genkey | tee server_private.key | wg pubkey > server_public.key
wg genkey | tee client_private.key | wg pubkey > client_public.key
```

**Populate templates:**

`wg0-server.conf`:
```ini
[Interface]
Address = fd00:dead:beef::1/48
ListenPort = 51820
PrivateKey = $(cat server_private.key)
MTU = 1380

[Peer]
PublicKey = $(cat client_public.key)
AllowedIPs = fd00:dead:beef::2/128
```

`wg0-client.conf`:
```ini
[Interface]
Address = fd00:dead:beef::2/48
PrivateKey = $(cat client_private.key)
MTU = 1380

[Peer]
PublicKey = $(cat server_public.key)
AllowedIPs = fd00:dead:beef::1/128, fd00:dead:beef::/48
Endpoint = <HOST_A_IP>:51820
PersistentKeepalive = 25
```

**Docker network:**
- Host-A bridge: `172.20.0.0/16` + `fd00:dead:cafe::/64`
- Host-B bridge: `172.21.0.0/16` + `fd00:dead:cafe::/64`
- WireGuard tunnel: `fd00:dead:beef::/48` (east-west only)

### IPv6 Configuration

Enable forwarding on host:
```bash
sudo sysctl -w net.ipv6.conf.all.forwarding=1
sudo sysctl -w net.ipv6.conf.eth0.forwarding=1  # or appropriate interface
# Persist:
echo "net.ipv6.conf.all.forwarding=1" | sudo tee -a /etc/sysctl.d/99-unheaded.conf
sudo sysctl -p /etc/sysctl.d/99-unheaded.conf
```

## ROCm GPU Passthrough

### Host Prerequisites

**Install ROCm (on host, not in container):**
```bash
# Ubuntu 22.04
wget -q -O - https://repo.radeon.com/rocm/rocm.gpg.key | sudo apt-key add -
echo 'deb [arch=amd64] https://repo.radeon.com/rocm/apt/debian jammy main' | sudo tee /etc/apt/sources.list.d/rocm.list
sudo apt-get update
sudo apt-get install -y rocm-core rocminfo
```

**Verify GPU:**
```bash
rocminfo | grep gfx
# Expected: gfx1101 for RX 7700 XT
```

**Device access:**
```bash
ls -la /dev/kfd /dev/dri/renderD*
# Should be readable by container user
sudo usermod -aG render,video $(whoami)
```

### Container Configuration

The `docker-compose.vllm.yml` already includes:

```yaml
devices:
  - /dev/kfd:/dev/kfd
  - /dev/dri:/dev/dri
group_add:
  - video
  - render
```

Environment variables for gfx1101 (RX 7700 XT):
```yaml
environment:
  HSA_OVERRIDE_GFX_VERSION: "11.0.1"
  PYTORCH_ROCM_ARCH: "gfx1101"
  HIP_VISIBLE_DEVICES: "0"
```

### Verify GPU Access

```bash
# Build vLLM image
make build-vllm

# Launch test container
docker run --rm \
  --device /dev/kfd:/dev/kfd \
  --device /dev/dri:/dev/dri \
  --group-add video \
  --group-add render \
  -e HSA_OVERRIDE_GFX_VERSION=11.0.1 \
  -e PYTORCH_ROCM_ARCH=gfx1101 \
  unheaded-vllm-rocm:dev \
  python3 -c "import torch; print(f'GPU Available: {torch.cuda.is_available()}'); print(f'Devices: {torch.cuda.device_count()}')"
```

Expected output:
```
GPU Available: True
Devices: 1
```

## Docker vs. NixOS: Resource Management

### Docker Approach (cgroup v2)

Resource limits via `deploy.resources`:
```yaml
deploy:
  resources:
    limits:
      memory: 14g
      cpus: '8.0'
```

Advantages:
- Port-compatible across distributions
- Familiar for container-native teams
- Fine-grained per-service limits
- Standard monitoring (cgroup metrics)

Disadvantages:
- Requires container runtime overhead
- Less integrated with system services

### NixOS Approach (systemd slices)

Resource limits via `systemd.slices`:
```nix
systemd.slices."unheaded-monad" = {
  sliceConfig = {
    MemoryAccounting = true;
    MemoryMax = "2G";
    CPUAccounting = true;
    CPUQuota = "50%";
  };
};
```

Advantages:
- Native system integration
- Declarative, immutable configuration
- Minimal overhead
- Better systemd integration

Disadvantages:
- Linux-only
- Steeper learning curve for traditional teams

### Security Posture

Both approaches provide equivalent security:

| Aspect | Docker | NixOS |
|--------|--------|-------|
| Process isolation | cgroups | systemd slices |
| Network isolation | bridge networking | nftables rules |
| Secrets management | Docker secrets / volumes | SOPS/agenix |
| eBPF services | Elevated caps + seccomp | systemd mount units |
| Audit trail | Container logs | journalctl |

**Recommendation:** Use Docker for portability, NixOS for long-term reproducibility.

## Security Considerations

### eBPF Services (Host-A)

Services `shield` and `unheaded-daemon` require elevated capabilities:

```yaml
cap_add:
  - SYS_ADMIN       # eBPF program load/attach
  - NET_ADMIN       # Network manipulation
  - SYS_RESOURCE    # Unlimited locked memory
  - SYS_PERFMON     # Performance monitoring
```

**Minimize blast radius:**
```yaml
security_opt:
  - no-new-privileges:true

cap_drop:
  - ALL

cap_add:
  - NET_ADMIN
  - SYS_ADMIN
```

### Secrets Management

**Never commit secrets to Git:**
```bash
# Use Docker secrets (Swarm mode)
echo "my-secret" | docker secret create db_password -

# Or use volumes with restricted permissions
docker run -v /secrets:/secrets:ro ...
chmod 600 /secrets/db_password
```

### Network Security

WireGuard tunnel is encrypted by default. Additional hardening:

```yaml
sysctls:
  # Drop invalid packets
  - net.ipv4.conf.all.log_martians=1
  # Enable reverse path filtering
  - net.ipv4.conf.all.rp_filter=1
  # Ignore ICMP redirects
  - net.ipv4.conf.all.send_redirects=0
  - net.ipv4.conf.all.accept_redirects=0
```

## Monitoring & Observability

### Health Checks

Each service includes `healthcheck`:
```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8000/health"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 30s
```

**View health status:**
```bash
docker inspect --format='{{json .State.Health}}' unheaded-monad | jq
```

### Logs

Centralized logging via `logging` driver:
```yaml
logging:
  driver: "json-file"
  options:
    max-size: "100m"    # Single log file size
    max-file: "3"       # Keep 3 rotated files
```

**View logs:**
```bash
make logs SERVICE=monad          # Follow live logs
docker logs unheaded-monad       # Show all logs
docker logs --tail 100 unheaded-monad  # Last 100 lines
```

### Metrics (Prometheus)

Services expose metrics on configured ports:

```yaml
labels:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8000"
  prometheus.io/path: "/metrics"
```

**Scrape configuration (in Prometheus):**
```yaml
scrape_configs:
  - job_name: 'docker'
    static_configs:
      - targets: ['localhost:9323']  # Docker daemon metrics
```

## Troubleshooting

### Container Won't Start

```bash
# Check logs
docker logs unheaded-monad

# Inspect container state
docker inspect unheaded-monad | jq '.State'

# Check resource constraints
docker stats unheaded-monad
```

### WireGuard Connection Issues

```bash
# Check status
docker exec unheaded-wireguard wg show all

# Verify interface
docker exec unheaded-wireguard ip -6 addr show

# Test connectivity
docker exec unheaded-wireguard ping -6 -c 3 fd00:dead:beef::2
```

### GPU Not Detected

```bash
# Verify host setup
rocminfo | grep gfx

# Check container has device access
docker exec unheaded-vllm-deepseek ls -la /dev/kfd /dev/dri/

# Test GPU access
docker exec unheaded-vllm-deepseek python3 -c "import torch; print(torch.cuda.is_available())"
```

### Memory Pressure

```bash
# Check memory usage
docker stats --no-stream | head -20

# Increase memory limit
docker update --memory 16g unheaded-monad

# Check for OOM kills
docker inspect unheaded-monad | jq '.State.OOMKilled'
```

### Network Connectivity

```bash
# Check bridge network
docker network inspect unheaded_unheaded

# Verify DNS resolution
docker exec unheaded-monad getent hosts unheaded-sophia

# Test port connectivity
docker exec unheaded-monad curl -v http://unheaded-sophia:8000/health
```

## Production Deployment

### Pre-Production Checklist

- [ ] All 25 services built and tested
- [ ] WireGuard tunnel tested (Host-A ↔ Host-B)
- [ ] GPU access verified on Host-B
- [ ] Logs aggregated to centralized store
- [ ] Backup strategy for persistent volumes
- [ ] Resource limits verified
- [ ] Health checks passing
- [ ] Security audit completed
- [ ] Network policies applied
- [ ] Monitoring and alerting configured

### Backup Strategy

```bash
# Backup persistent volumes
docker run --rm -v unheaded_data:/data -v $(pwd):/backup \
  alpine tar czf /backup/unheaded-data-$(date +%Y%m%d).tar.gz -C /data .

# Automated backup (cron)
0 2 * * * /usr/local/bin/unheaded-backup.sh
```

### Upgrades

```bash
# Rolling update: stop, rebuild, restart
docker compose down
git pull origin main
make build
docker compose up -d

# Blue-green: run new version alongside old
VERSION=v2 docker compose -f docker-compose.v2.yml up -d
# Test...
docker compose down  # stop v1
```

### Disaster Recovery

```bash
# Export configuration
docker compose config > unheaded-compose-backup.yml

# Rebuild from backup
docker compose down
docker compose -f unheaded-compose-backup.yml up -d
```

## Development

### Adding New Service

1. Create service directory:
   ```bash
   mkdir -p docker/services/mynewservice
   ```

2. Create Dockerfile:
   ```dockerfile
   FROM ubuntu:22.04
   # ...
   LABEL app.unheaded.service="mynewservice"
   ```

3. Add to Makefile SERVICES list

4. Add to host compose files (`hosts/host-a/docker-compose.yml`)

5. Build and test:
   ```bash
   make build-svc SERVICE=mynewservice
   docker compose up -d unheaded-mynewservice
   ```

### Local Testing

```bash
# Run single service locally
docker build -t unheaded-monad:test .
docker run -it --rm \
  --network unheaded_unheaded \
  unheaded-monad:test \
  /bin/bash

# Debug container
docker exec -it unheaded-monad /bin/bash
docker run -it --rm --entrypoint /bin/bash unheaded-monad:test
```

## References

- [Docker Documentation](https://docs.docker.com/)
- [Docker Compose Specification](https://github.com/compose-spec/compose-spec)
- [WireGuard Manual](https://www.wireguard.com/quickstart/)
- [ROCm Documentation](https://rocmdocs.amd.com/)
- [vLLM Documentation](https://docs.vllm.ai/)
- [Prometheus Monitoring](https://prometheus.io/docs/)

## License

SPDX-License-Identifier: MIT

Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

This Docker infrastructure is provided as-is under the MIT License. See LICENSE file in the root directory.
