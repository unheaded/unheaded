# Unheaded Kingdom LXD Host Configuration

This directory contains LXD initialization and deployment scripts for the Unheaded Kingdom project across two distinct hosts.

## Overview

### host-a: The Forge
- **Purpose**: Full-stack deployment with all 25 services
- **Hardware**: 16+ cores, 64GB RAM, RX 7700 XT GPU
- **Services**: Complete protocol stack, all business logic, compute services, observability stack
- **Storage**: 200GB ZFS pool (unheaded-ssd)
- **Network**: 10.20.0.0/16 (IPv4) + fd00:dead:beef:1::/64 (IPv6)

### host-b: The Outpost
- **Purpose**: Minimal deployment for distributed deployments
- **Hardware**: Consumer-grade (8GB RAM typical, no GPU)
- **Services**: 6 core services (wotan, monad, sophia, anamnesis, gateway, dashboard-backend) + telemetry agents
- **Storage**: 50GB ZFS pool (unheaded-minimal)
- **Network**: 10.21.0.0/16 (IPv4) + fd00:dead:beef:2::/64 (IPv6)
- **Connectivity**: WireGuard tunnel to host-a for cluster operations

## Quick Start

### For host-a (Forge)

```bash
# 1. Copy preseed and init script to target host
scp -r host-a/ user@forge-host:/tmp/

# 2. Run bootstrap (requires sudo)
ssh user@forge-host
cd /tmp/host-a
sudo ./init.sh

# 3. Verify LXD setup
lxc network ls
lxc storage ls
lxc profile ls

# 4. Launch all services
./launch-all.sh

# 5. Monitor deployment
lxc list | grep unheaded
lxc logs unheaded-wotan
```

### For host-b (Outpost)

```bash
# 1. Copy preseed and init script to target host
scp -r host-b/ user@outpost-host:/tmp/

# 2. Run bootstrap (requires sudo)
ssh user@outpost-host
cd /tmp/host-b
sudo ./init.sh

# 3. Verify LXD setup
lxc network ls
lxc storage ls

# 4. Launch core services
./launch-minimal.sh

# 5. Monitor deployment
lxc list | grep unheaded
lxc logs unheaded-wotan
```

## File Descriptions

### host-a/

#### preseed.yaml
LXD initialization preseed for automated configuration. Defines:
- Core LXD settings (HTTPS address, image caching)
- Network bridge "unheaded" (10.20.0.0/16, fd00:dead:beef:1::/64)
- ZFS storage pool "unheaded-ssd" (200GB)
- Default container profile with 2 CPU / 512MB memory

**Usage**: `sudo lxd init --preseed < preseed.yaml`

#### init.sh
Idempotent bootstrap script that:
1. Checks if running as root
2. Installs LXD 5.21 via snap (if needed)
3. Adds current user to lxd group
4. Applies LXD preseed configuration
5. Creates /opt/unheaded/bin directory on host
6. Copies service binaries from repo (if available)
7. Loads additional LXD profiles from ../../profiles/

**Usage**: `sudo ./init.sh`

#### launch-all.sh
Complete service deployment script that:
1. Launches 25 service containers in dependency order
2. Configures per-service CPU/memory limits
3. Applies specialized profiles (ebpf, gpu)
4. Pushes binaries and starts systemd services
5. Handles failures gracefully with reporting

**Launch order**:
- Phase 1: wotan (message bus)
- Phase 2: monad, sophia, anamnesis (protocol layer)
- Phase 3: shield, unheaded-daemon (eBPF control)
- Phase 4: gateway, service-discovery (routing)
- Phase 5: dashboard-backend, dashboard-frontend, protocol-api (presentation)
- Phase 6: trace-collector, doom (observability/compute)
- Phase 7-8: 11 service containers (kanban, captain, etc.)
- Phase 9: lich (adversary, last)
- Phase 10: telemetry stack (prometheus, grafana, loki, etc.)

**Usage**: `./launch-all.sh`

#### static-ips.yaml
Reference file containing deterministic IP assignments for all containers:
- 26 service containers: 10.20.1.1-10.20.1.25
- 4 telemetry containers: 10.20.1.50-10.20.1.53
- 1 exporter: 10.20.1.26

Used for:
- Prometheus scrape configuration
- Service discovery
- Networking documentation
- DHCP reservation planning

### host-b/

#### preseed.yaml
Minimal LXD initialization preseed. Differs from host-a:
- Smaller storage pool (50GB vs 200GB)
- Separate network "unheaded-outpost" (10.21.0.0/16)
- Smaller resource defaults (1 CPU / 256MB vs 2/512MB)
- Optimized for consumer-grade hardware

#### init.sh
Minimal bootstrap script that:
1. Installs LXD 5.21 via snap (if needed)
2. Applies minimal preseed configuration
3. Creates /opt/unheaded/bin directory
4. Sets up WireGuard tunnel to host-a (if config exists)
5. Loads LXD profiles

**Differences from host-a**:
- Includes WireGuard tunnel setup for cluster connectivity
- Simpler profile configuration
- Minimal storage allocation

**Usage**: `sudo ./init.sh`

#### launch-minimal.sh
Core service deployment for host-b:
1. Launches 6 essential services (wotan, monad, sophia, anamnesis, gateway, dashboard-backend)
2. Launches 3 telemetry agents:
   - prometheus-agent (remote_write to host-a)
   - promtail (log shipping to host-a)
   - node-exporter (host metrics)

**Resource allocation**:
- Core services: 1-2 CPU, 256-512MB memory
- Telemetry: 1 CPU, 256MB memory

**Usage**: `./launch-minimal.sh`

## Container Resource Allocation

### host-a (Forge) - Full Stack

| Service | CPU | Memory | Special Profile |
|---------|-----|--------|-----------------|
| wotan | 4 | 1GB | - |
| monad | 2 | 512MB | - |
| sophia | 2 | 512MB | - |
| anamnesis | 2 | 512MB | - |
| shield | 4 | 2GB | ebpf |
| unheaded-daemon | 4 | 1GB | ebpf |
| gateway | 2 | 512MB | - |
| service-discovery | 1 | 256MB | - |
| dashboard-backend | 2 | 512MB | - |
| dashboard-frontend | 1 | 256MB | - |
| protocol-api | 2 | 512MB | - |
| trace-collector | 2 | 512MB | - |
| doom | 4 | 1GB | - |
| lich | 4 | 2GB | - |
| captain | 1 | 256MB | - |
| micromanager | 1 | 256MB | - |
| timeguru | 1 | 256MB | - |
| architect | 1 | 256MB | - |
| developer | 1 | 256MB | - |
| kanban | 2 | 512MB | - |
| lore | 1 | 256MB | - |
| busboy | 1 | 256MB | - |
| kingdom | 1 | 256MB | - |
| blackmage | 1 | 256MB | - |
| moatghost | 1 | 256MB | - |
| **Total** | **51 CPU** | **23.5GB** | - |

### host-b (Outpost) - Minimal Stack

| Service | CPU | Memory |
|---------|-----|--------|
| wotan | 2 | 512MB |
| monad | 1 | 256MB |
| sophia | 1 | 256MB |
| anamnesis | 1 | 256MB |
| gateway | 1 | 256MB |
| dashboard-backend | 1 | 256MB |
| prometheus-agent | 1 | 256MB |
| promtail | 1 | 256MB |
| node-exporter | 1 | 256MB |
| **Total** | **10 CPU** | **2.5GB** |

## Networking

### host-a Internal Bridge (unheaded)
```
Network: 10.20.0.0/16
Gateway: 10.20.0.1
DHCP Pool: 10.20.1.1-10.20.1.254
IPv6: fd00:dead:beef:1::/64
DNS Domain: unheaded.internal
```

### host-b Internal Bridge (unheaded-outpost)
```
Network: 10.21.0.0/16
Gateway: 10.21.0.1
DHCP Pool: 10.21.1.1-10.21.1.100
IPv6: fd00:dead:beef:2::/64
DNS Domain: unheaded.internal
```

### WireGuard Tunnel (host-b to host-a)
- Connects host-b network to host-a for cluster operations
- Configuration: host-b/wireguard.conf (user-provided)
- Interface: wg0
- Purpose: Allows host-b containers to reach host-a services

## Container Lifecycle

### Startup
1. Container launched with base ubuntu:24.04 image
2. Cloud-init runs (if configured)
3. Binary pushed to /opt/unheaded/bin/
4. Systemd service started (service-specific unit file required)

### Management

List all containers:
```bash
lxc list | grep unheaded
```

View container logs:
```bash
lxc logs unheaded-SERVICENAME --follow
```

Execute command in container:
```bash
lxc exec unheaded-SERVICENAME -- systemctl status unheaded-SERVICENAME
```

Access container shell:
```bash
lxc exec unheaded-SERVICENAME -- /bin/bash
```

### Cleanup

Stop a service:
```bash
lxc stop unheaded-SERVICENAME
```

Delete a container:
```bash
lxc delete unheaded-SERVICENAME
```

## Monitoring and Observability

### host-a Telemetry Stack
- **Prometheus** (10.20.1.50:9090): Metrics collection and storage
- **VictoriaMetrics** (10.20.1.51:8428): Time-series database (optional)
- **Loki** (10.20.1.52:3100): Log aggregation
- **Grafana** (10.20.1.53:3000): Visualization and dashboards
- **eBPF Exporter** (10.20.1.26:9435): Kernel-level metrics

### host-b Telemetry Agents
- **Prometheus Agent** (agent mode): Scrapes local services, remote_write to host-a
- **Promtail**: Ships logs to host-a Loki
- **Node Exporter**: Exports host machine metrics

### Static IP References
See `static-ips.yaml` for all container IPs and ports used in scrape configurations.

## Troubleshooting

### LXD won't initialize
```bash
# Check if snap installed correctly
snap list lxd

# Verify snap daemon
systemctl status snapd

# Try manual init (if preseed fails)
sudo lxd init --auto
```

### Containers won't start
```bash
# Check container status
lxc list

# View container logs
lxc logs CONTAINER_NAME

# Check resource availability
free -h
df -h /var/lib/lxd

# Verify storage pool
lxc storage ls
```

### Network connectivity issues
```bash
# Check bridge interface
ip addr show unheaded

# Test container networking
lxc exec CONTAINER_NAME -- ping -c 1 8.8.8.8

# Check DNS resolution
lxc exec CONTAINER_NAME -- nslookup unheaded.internal
```

### Service won't start
```bash
# Check if binary exists in container
lxc exec CONTAINER_NAME -- ls -la /opt/unheaded/bin/

# Check systemd status
lxc exec CONTAINER_NAME -- systemctl status unheaded-SERVICENAME

# View service logs
lxc exec CONTAINER_NAME -- journalctl -u unheaded-SERVICENAME -f
```

## Advanced Topics

### Custom Profiles
Additional profiles can be placed in the repo's `lxd/profiles/` directory and will be automatically loaded during init.sh.

Common profiles to implement:
- `unheaded-ebpf.yaml`: eBPF programs and seccomp rules
- `unheaded-gpu.yaml`: GPU device passthrough for RX 7700 XT

### Scaling Beyond Two Hosts
For deployments with more than two hosts:
1. Duplicate host-b configuration for additional minimal nodes
2. Adjust network subnets (10.21.x.x, 10.22.x.x, etc.)
3. Configure WireGuard mesh networking between all hosts
4. Set up Prometheus federation for centralized metrics

### Persistent Storage
Container root filesystems are stored in ZFS pools:
- host-a: /var/lib/lxd/storage-pools/unheaded-ssd/
- host-b: /var/lib/lxd/storage-pools/unheaded-minimal/

To add additional storage devices to containers:
```bash
lxc storage volume create POOL VOLUME_NAME
lxc storage volume attach POOL VOLUME_NAME CONTAINER /mount/point
```

## References

- LXD Documentation: https://documentation.ubuntu.com/lxd/
- LXD Preseed: https://documentation.ubuntu.com/lxd/latest/cli/lxd_init/
- ZFS: https://openzfs.org/wiki/Main_Page
- WireGuard: https://www.wireguard.com/

