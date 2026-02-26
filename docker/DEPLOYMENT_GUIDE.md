# Unheaded Kingdom — Docker Deployment Guide

**Status:** Production Ready
**Date:** 2026-02-26
**Base Directory:** `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/`

## Quick Navigation

- **New to Docker deployment?** Start with [QUICKSTART.md](./QUICKSTART.md)
- **Need technical reference?** See [README.md](./README.md)
- **Want to understand the architecture?** Check [DOCKER_SETUP_SUMMARY.md](./DOCKER_SETUP_SUMMARY.md)
- **Looking for specific files?** See [MANIFEST.md](./MANIFEST.md)

## Core Components

### 1. WireGuard East-West Bridge

**Files:**
- `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/wireguard/docker-compose.wireguard.yml`
- `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/wireguard/wg0-server.conf.template`
- `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/wireguard/wg0-client.conf.template`

**Purpose:** Secure IPv6 tunnel between Host-A (Forge) and Host-B (Outpost)

**Network:**
```
Host-A Server: fd00:dead:beef::1/48
Host-B Client: fd00:dead:beef::2/48
Port: 51820/UDP
MTU: 1380 bytes (IPv6-safe)
Keepalive: 25 seconds
```

**Deploy:**
```bash
# Generate keys on Host-A
cd wireguard/config
wg genkey | tee server_private.key | wg pubkey > server_public.key
wg genkey | tee client_private.key | wg pubkey > client_public.key

# Populate templates with generated keys
# Then deploy via docker-compose
cd ..
docker compose -f docker-compose.wireguard.yml up -d
```

### 2. vLLM/ROCm GPU Inference

**Files:**
- `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/vllm-rocm/Dockerfile`
- `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/vllm-rocm/docker-compose.vllm.yml`

**Purpose:** AMD RX 7700 XT GPU inference with vLLM

**Specifications:**
- GPU: AMD RX 7700 XT (gfx1101, RDNA2)
- vLLM: v0.4.3
- ROCm: v6.1.3
- Model: Deepseek-R1 7B (Q4 quantized)
- Port: 20100
- Memory: 14GB limit, 4GB shared memory
- CPU: 8 cores

**Build & Deploy:**
```bash
# Build vLLM image
make build-vllm

# Or manual build
cd vllm-rocm
docker build -t unheaded-vllm-rocm:dev .

# Deploy
docker compose -f docker-compose.vllm.yml up -d

# Verify GPU access
docker exec unheaded-vllm-deepseek python3 -c "import torch; print(torch.cuda.is_available())"
```

### 3. Service Orchestration

**File:** `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/Makefile`

**Key Targets:**

**Build All Services:**
```bash
make build              # Build all 25 services
make build-svc SERVICE=monad  # Build single service
make build-vllm         # Build vLLM/ROCm only
```

**Deploy Services:**
```bash
make up-forge           # Start Host-A (full stack, 25 services)
make up-outpost         # Start Host-B (minimal suite + vLLM)
make down               # Stop all services
make status             # Show container status
```

**Registry Operations:**
```bash
make push               # Push to ghcr.io/stevenrbellis/unheaded
make pull               # Pull from registry
REGISTRY=myregistry make push  # Use custom registry
```

**Maintenance:**
```bash
make preflight          # Pre-deployment checks
make logs SERVICE=monad # Follow service logs
make clean              # Remove all images
make help               # Show all targets
```

### 4. Environment Configuration

**Shared Configuration:**
```env
# File: /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker/.env.shared
COMPOSE_PROJECT_NAME=unheaded
UNHEADED_LOG_LEVEL=info
UNHEADED_TRACE_ENABLED=true
UNHEADED_METRICS_ENABLED=true
VERSION=dev
UNHEADED_IPV6_PREFIX=fd00:dead:beef::/48
UNHEADED_NETWORK_MTU=1380
VLLM_MODEL=/models/deepseek-r1-7b-q4
ROCM_VERSION=6.1.3
VLLM_VERSION=0.4.3
HSA_OVERRIDE_GFX_VERSION=11.0.1
PYTORCH_ROCM_ARCH=gfx1101
HIP_VISIBLE_DEVICES=0
```

**Usage:**
```bash
# Copy to both hosts
cp .env.shared hosts/host-a/
cp .env.shared hosts/host-b/

# Create host-specific overrides in .env files
# (see .env.example for all options)
```

## Deployment Steps

### Phase 1: Prerequisites

```bash
cd /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/docker

# Verify system requirements
make preflight

# Expected output:
# - Docker 24.0+
# - Docker Compose v2.20+
# - Kernel 6.0+
# - Cgroups v2 enabled
# - IPv6 support enabled
# - /dev/kfd available (for GPU)
```

### Phase 2: Build Images

```bash
# Build all 25 services (30-45 minutes)
make build

# Build vLLM/ROCm (5-10 minutes)
make build-vllm

# Verify builds
docker images | grep unheaded-
```

### Phase 3: Configure WireGuard (Host-A)

```bash
cd wireguard

# Generate keypairs
mkdir -p config
cd config
wg genkey | tee server_private.key | wg pubkey > server_public.key
wg genkey | tee client_private.key | wg pubkey > client_public.key

# Display keys for Host-B
cat server_public.key   # Send to Host-B
cat client_public.key   # For server config
cat client_private.key  # Send to Host-B securely

cd ../..
```

### Phase 4: Deploy Host-A (Forge)

```bash
# Deploy full stack
make up-forge

# Monitor startup
make status

# Check service health
docker ps -f "name=unheaded-"

# View logs for specific service
make logs SERVICE=monad
```

### Phase 5: Configure & Deploy Host-B (Outpost)

On Host-B system:

```bash
cd /path/to/docker

# Configure WireGuard client
mkdir -p wireguard/config
# Populate wg0-client.conf with received keys and Host-A endpoint IP

# Set environment variables
export VLLM_ENABLED=true
export WG_SERVERURL=<host-a-ip-or-fqdn>

# Deploy
make up-outpost

# Monitor startup
make status

# Check WireGuard connection
docker exec unheaded-wireguard wg show all
```

### Phase 6: Verify Connectivity

**From Host-A:**
```bash
docker exec unheaded-wireguard ping -6 -c 3 fd00:dead:beef::2
# Expected: 3 replies from fd00:dead:beef::2
```

**From Host-B:**
```bash
docker exec unheaded-wireguard ping -6 -c 3 fd00:dead:beef::1
# Expected: 3 replies from fd00:dead:beef::1
```

**Verify GPU on Host-B:**
```bash
docker exec unheaded-vllm-deepseek python3 -c \
  "import torch; print(f'GPU: {torch.cuda.is_available()}, Device: {torch.cuda.device_count()}')"
# Expected: GPU: True, Device: 1
```

## Troubleshooting

### Docker Issues

**Container won't start:**
```bash
docker logs unheaded-monad
docker inspect unheaded-monad | jq '.State'
docker stats unheaded-monad
```

**Image build failed:**
```bash
docker build --progress=plain -t unheaded-monad:dev .
# Shows detailed build output
```

### Network Issues

**WireGuard not connecting:**
```bash
docker exec unheaded-wireguard wg show all
docker exec unheaded-wireguard ip -6 addr show
docker logs unheaded-wireguard | tail -20
```

**IPv6 not working:**
```bash
sysctl net.ipv6.conf.all.forwarding
# Should return: 1

# Enable if needed:
sudo sysctl -w net.ipv6.conf.all.forwarding=1
```

### GPU Issues

**GPU not detected:**
```bash
rocminfo | grep gfx
# Should show: gfx1101 for RX 7700 XT

ls -la /dev/kfd /dev/dri/
# Should be readable
```

**vLLM model loading fails:**
```bash
docker logs unheaded-vllm-deepseek | grep -i model
docker exec unheaded-vllm-deepseek ls -la /models/
# Model directory must exist and contain weights
```

## Production Deployment

### Pre-Deployment Checklist

- [ ] All prerequisites verified (make preflight)
- [ ] Images built successfully (docker images | grep unheaded-)
- [ ] WireGuard keys generated and configured
- [ ] Environment variables set correctly
- [ ] Model weights downloaded to /mnt/models
- [ ] Disk space available (min 100GB for models)
- [ ] Backup strategy in place
- [ ] Monitoring configured (Prometheus, Grafana)
- [ ] Security audit completed
- [ ] Network policies applied

### Backup & Recovery

**Backup persistent volumes:**
```bash
docker run --rm -v unheaded_data:/data -v $(pwd):/backup \
  alpine tar czf /backup/unheaded-backup-$(date +%Y%m%d).tar.gz -C /data .
```

**Export docker-compose configuration:**
```bash
docker compose config > backup-compose.yml
```

**Restore from backup:**
```bash
docker run --rm -v unheaded_data:/data -v $(pwd):/backup \
  alpine tar xzf /backup/unheaded-backup-YYYYMMDD.tar.gz -C /data
```

### Upgrade Procedure

```bash
# 1. Stop services
make down

# 2. Pull latest code
git pull origin main

# 3. Rebuild images
make build VERSION=v2
make build-vllm VERSION=v2

# 4. Update .env files with new version
VERSION=v2 docker compose up -d

# 5. Verify health
make status
```

### Monitoring

**Check service health:**
```bash
docker ps --filter "name=unheaded-" \
  --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
```

**View metrics:**
```bash
# Prometheus scrape config
curl http://localhost:9090/api/v1/targets

# Query metrics
curl http://localhost:9090/api/v1/query?query=container_memory_usage_bytes
```

**Centralized logging:**
```bash
docker logs unheaded-monad --since 1h
docker logs unheaded-vllm-deepseek --tail 100 -f
```

## Security Hardening

### Host Security

```bash
# Enable UFW firewall
sudo ufw enable
sudo ufw allow 51820/udp   # WireGuard
sudo ufw allow 20100/tcp   # vLLM API

# Restrict WireGuard to specific IPs
sudo ufw allow from 10.0.0.0/8 to any port 51820/udp
```

### Container Security

- All services run as non-root users
- No new privileges: `security_opt: [no-new-privileges:true]`
- Drop all capabilities, only add required ones
- Read-only root filesystem where applicable
- Resource limits enforced (memory, CPU)

### Network Security

- WireGuard tunnel encrypted by default
- IPv6 only for east-west traffic
- iptables rules for forwarding
- No direct internet exposure

## Getting Help

**Documentation:**
- Overview: [README.md](./README.md)
- Quick Start: [QUICKSTART.md](./QUICKSTART.md)
- Architecture: [DOCKER_SETUP_SUMMARY.md](./DOCKER_SETUP_SUMMARY.md)
- File Manifest: [MANIFEST.md](./MANIFEST.md)

**Commands:**
```bash
make help               # Show all Makefile targets
docker compose help     # Docker Compose documentation
make preflight          # Run pre-deployment checks
make logs SERVICE=monad # Follow service logs
```

## Support

For issues or questions:

1. Check logs: `docker logs <container_name>`
2. Run diagnostics: `make preflight`
3. Review documentation: See files listed above
4. Check GitHub issues: https://github.com/stevenrbellis/unheaded/issues

## License

SPDX-License-Identifier: MIT

Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

---

**Last Updated:** 2026-02-26
**Status:** Production Ready
**Docker Compose Version:** v2.20+
**Kernel Requirement:** 6.0+
