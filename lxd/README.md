# Unheaded Kingdom — LXD Infrastructure

Comprehensive Linux Container (LXD) deployment for the Unheaded Kingdom distributed system. Manages 30+ containerized services across two hosts (Forge and Outpost) with telemetry, east-west encryption, and GPU acceleration.

**Status:** Production-ready
**Updated:** 2025-02
**Architecture:** AMD64 + ROCm GPU
**Containers:** 30 services + 4 telemetry + 1 inference = 35 total

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Quick Start (5 minutes)](#quick-start-5-minutes)
3. [Directory Structure](#directory-structure)
4. [Deployment Options](#deployment-options)
5. [LXD Concepts](#lxd-concepts)
6. [Network Topology](#network-topology)
7. [Resource Summary](#resource-summary)
8. [Operations](#operations)
9. [Troubleshooting](#troubleshooting)

---

## Prerequisites

### System Requirements

- **OS:** Ubuntu 22.04 LTS or 24.04 LTS
- **CPU:** 16+ cores (recommended)
- **RAM:** 64GB+ for all 30 containers
- **Storage:** 500GB+ SSD (LXD pool)
- **GPU:** (Optional) AMD Radeon RX 7700 XT or compatible for vLLM

### Software

```bash
# Install LXD via snap (5.x or later)
sudo snap install lxd --channel=latest/stable
sudo snap refresh lxd

# Verify installation
lxc --version  # Should be 5.x or later

# Initialize LXD (choose defaults for most prompts)
lxd init --minimal
```

### Network Setup

- Host-a (Forge): 10.20.1.0/24 (25 services + telemetry)
- Host-b (Outpost): 10.20.2.0/24 (6 core services)
- WireGuard tunnel: fd00:dead:beef::/48 (IPv6)

---

## Quick Start (5 minutes)

### On Host-a (Forge)

```bash
# 1. Clone infrastructure files
cd /path/to/unheaded/lxd

# 2. Create LXD profiles and network
lxc profile create unheaded-base
lxc network create unheaded-br ipv4.address=10.20.1.1/24
lxc network attach unheaded-br unheaded-base eth0

# 3. Create storage pool
lxc storage create unheaded-ssd dir source=/var/lib/lxd/storage-pools/unheaded-ssd

# 4. Initialize LXD infrastructure
make init-host-a

# 5. Launch all 30 containers
make launch-all

# 6. Launch telemetry stack (optional)
make launch-telemetry

# 7. Launch vLLM/ROCm (optional, requires GPU)
make launch-vllm

# Check status
make status
```

### On Host-b (Outpost)

```bash
# 1-3. Same as host-a (profiles, network, storage)

# 4. Initialize for host-b
make init-host-b

# 5. Launch 6 core containers
make launch-minimal

# Check status
make status
```

---

## Directory Structure

```
lxd/
├── README.md                         # This file
├── Makefile                          # Operations automation
├── static-ips.yaml                   # Service IP assignments
│
├── telemetry/                        # Observability stack
│   ├── prometheus-lxd.yaml           # Metrics scraper (10.20.1.50:9090)
│   ├── victoriametrics-lxd.yaml      # Long-term storage (10.20.1.51:8428)
│   ├── loki-lxd.yaml                 # Log aggregation (10.20.1.52:3100)
│   ├── grafana-lxd.yaml              # Visualization (10.20.1.53:3000)
│   └── setup-telemetry.sh            # Launch script
│
├── wireguard/                        # East-west encryption
│   ├── wireguard-lxd.yaml            # Container definition
│   ├── wg0-server.conf.template      # Server config (host-a)
│   └── wg0-client.conf.template      # Client config (host-b)
│
├── vllm-rocm/                        # LLM inference
│   ├── vllm-lxd.yaml                 # DeepSeek-R1 container
│   └── setup-vllm.sh                 # Launch script
│
└── services/                         # Service container definitions
    ├── wotan/                        # DNS/proxy
    ├── ares/                         # API gateway
    ├── hermes/                       # Message broker
    ├── athena/                       # Configuration management
    └── ... (22 more services)
```

---

## Deployment Options

### Comparison: NixOS vs Docker vs LXD

| Aspect | NixOS | Docker | LXD |
|--------|-------|--------|-----|
| **Boot Time** | 30-60s | 5-10s | 3-5s |
| **Container Size** | 2-5GB | 500MB-2GB | 1-3GB |
| **Startup Overhead** | 512MB | 256MB | 128MB |
| **Networking** | Complex bridging | Built-in | Native bridge |
| **GPU Support** | Manual passthrough | via `--gpus` | Direct mapping |
| **Declarative** | Full system | Image-based | Config YAML |
| **Security Model** | AppArmor + namespaces | UID mapping | Full isolation |
| **Best For** | Reproducible builds | Microservices | System containers |

**Unheaded Kingdom uses LXD because:**
- Fast container startup (3-5s vs 30-60s NixOS)
- System container model (less overhead than Docker)
- Native IPv6 support for WireGuard
- GPU passthrough for ROCm/vLLM
- Minimal resource overhead (1GB base vs 2-5GB NixOS)

---

## LXD Concepts

### Profiles

LXD profiles define container base configuration. Unheaded Kingdom provides:

**`unheaded-base`**
- 2 CPU cores (configurable per container)
- 2GB RAM (configurable per container)
- Ubuntu 24.04 image
- Network bridge attachment (unheaded-br)
- Storage pool assignment (unheaded-ssd)

**`unheaded-gpu`**
- GPU device mapping (AMD Radeon)
- KFD (Kernel Fusion Driver) access
- `/dev/dri/renderD*` passthrough

### Cloud-Init

Each container includes `cloud-init.user-data` for boot-time provisioning:

```yaml
cloud-init.user-data: |
  #cloud-config
  package_update: true
  packages:
    - curl
    - wget
  runcmd:
    - systemctl enable my-service
    - systemctl start my-service
```

### Bind Mounts

Host directories mounted inside containers for config sharing:

```yaml
devices:
  prometheus-config:
    type: disk
    source: /etc/unheaded/prometheus      # Host path
    path: /etc/prometheus                 # Container path
    readonly: false
```

### Static IPs

Each container gets a static IPv4 address via cloud-init:

```yaml
config:
  cloud-init.network-config: |
    version: 2
    ethernets:
      eth0:
        dhcp4: false
        addresses:
          - 10.20.1.50/24
        gateway4: 10.20.1.1
```

---

## Network Topology

```
                    INTERNET
                        |
        ________________|________________
        |                               |
    HOST-A (Forge)                  HOST-B (Outpost)
    10.20.0.0/24                    10.20.2.0/24
        |                               |
        |-- unheaded-br (10.20.1.0/24)  |
        |                               |
        |-- 25 services                 |-- 6 services
        |   - wotan (10.20.1.10)        |   - wotan (10.20.2.10)
        |   - ares (10.20.1.11)         |   - ares (10.20.2.11)
        |   - ... (23 more)             |   - ... (4 more)
        |                               |
        |-- Telemetry (4 containers)    |
        |   - Prometheus (10.20.1.50)   |
        |   - VictoriaMetrics (10.20.1.51)
        |   - Loki (10.20.1.52)         |
        |   - Grafana (10.20.1.53)      |
        |                               |
        |-- WireGuard (10.20.1.100)     |-- WireGuard (10.20.2.100)
        |   IPv6: fd00:dead:beef::1     |   IPv6: fd00:dead:beef::2
        |                               |
        |-- vLLM/ROCm (10.20.1.99)      |
        |   DeepSeek-R1-7B              |
        |                               |
        +----------- IPv6 Tunnel -------+
            (fd00:dead:beef::/48)
            Port 51820 (WireGuard)
```

### Network Segments

**10.20.0.0/24** — Host management network
- 10.20.0.1 — Host-a gateway (node-exporter :9100)
- 10.20.0.2 — Host-b gateway (node-exporter :9100)

**10.20.1.0/24** — Host-a service network
- 10.20.1.1 — LXD bridge gateway
- 10.20.1.10-34 — 25 service containers
- 10.20.1.50-53 — Telemetry stack
- 10.20.1.99 — vLLM/ROCm
- 10.20.1.100 — WireGuard server

**10.20.2.0/24** — Host-b service network
- 10.20.2.1 — LXD bridge gateway
- 10.20.2.10-15 — 6 core services
- 10.20.2.100 — WireGuard client

**fd00:dead:beef::/48** — WireGuard tunnel (IPv6)
- fd00:dead:beef::1/48 — Host-a endpoint
- fd00:dead:beef::2/48 — Host-b endpoint

---

## Resource Summary

### Container Inventory (35 total)

| Category | Count | CPUs | RAM | Storage | Notes |
|----------|-------|------|-----|---------|-------|
| Services | 25 | 50 | 50GB | 250GB | Full deployment |
| Services (minimal) | 6 | 12 | 12GB | 60GB | Host-b only |
| Telemetry | 4 | 8 | 14GB | 95GB | Prometheus, VM, Loki, Grafana |
| Inference | 1 | 8 | 16GB | 80GB | vLLM/ROCm (GPU) |
| **TOTAL (host-a)** | **30** | **58** | **64GB** | **425GB** | With GPU container |
| **TOTAL (host-b)** | **6** | **12** | **12GB** | **60GB** | Core services only |

### CPU/Memory by Service Type

**Service Containers (25 total)**
```
- wotan, ares, hermes: 2 CPUs, 2GB RAM each (API tier)
- athena, hephaestus, demeter: 2 CPUs, 2GB RAM (Logic tier)
- poseidon, hades, apollo, artemis: 2 CPUs, 2GB RAM (Data tier)
- aphrodite, ares2, athena2, hermes2: 2 CPUs, 2GB RAM (Cache tier)
- monad, dyad, triad, tetrad, pentad: 1 CPU, 1GB RAM (Utility)
- hexad, heptad, ogdoad, ennead, dekad, hendecad: 1 CPU, 1GB RAM (Utility)
```

**Telemetry (4 containers)**
- Prometheus: 2 CPUs, 4GB RAM, 20GB storage
- VictoriaMetrics: 4 CPUs, 8GB RAM, 50GB storage
- Loki: 2 CPUs, 4GB RAM, 20GB storage
- Grafana: 2 CPUs, 2GB RAM, 15GB storage

**Inference (1 container)**
- vLLM/ROCm: 8 CPUs, 16GB RAM, 80GB storage, GPU

### Network Bandwidth

**Expected throughput:**
- Inter-container (LXD bridge): 10Gbps (native)
- Host-a to Host-b (WireGuard): 1-5Gbps (IPv6 tunnel)
- Service to Telemetry: 100Mbps aggregate
- vLLM requests: 10-100Mbps (model dependent)

---

## Operations

### Common Tasks

**View all containers**
```bash
make status
```

**Tail logs for a service**
```bash
make logs SVC=prometheus
```

**Enter container shell**
```bash
make exec SVC=athena
```

**Restart all containers**
```bash
make restart
```

**Stop all containers**
```bash
make stop
```

**Snapshot all containers**
```bash
make snapshot
```

**Destroy all containers**
```bash
make destroy
```

### Manual Operations

**Create a single container**
```bash
lxc launch -f telemetry/prometheus-lxd.yaml ubuntu:24.04 unheaded-prometheus
```

**Execute command in container**
```bash
lxc exec unheaded-wotan -- systemctl status unheaded-wotan
```

**Copy file to container**
```bash
lxc file push local-file.txt unheaded-wotan/root/
```

**View container config**
```bash
lxc config show unheaded-prometheus
```

**Modify container resources**
```bash
lxc config set unheaded-wotan limits.cpu 4
lxc config set unheaded-wotan limits.memory 4GB
```

---

## Troubleshooting

### Container Won't Start

**Symptoms:** `lxc launch` fails or container stays in "stopped" state

**Diagnosis:**
```bash
# Check LXD logs
journalctl -u snap.lxd.daemon -n 50

# Check container logs
lxc info unheaded-wotan

# View cloud-init output
lxc exec unheaded-wotan -- cat /var/log/cloud-init-output.log
```

**Solutions:**
- Ensure LXD storage pool exists: `lxc storage list`
- Check disk space: `df -h /var/lib/lxd`
- Verify network is created: `lxc network list`
- Reset container: `lxc delete -f unheaded-wotan && lxc launch ...`

### Networking Issues

**Container can't reach other containers**
```bash
# Check LXD bridge
lxc network show unheaded-br

# Ping container from host
ping -c 1 10.20.1.10

# Ping host from container
lxc exec unheaded-wotan -- ping 10.20.0.1

# Check container routing
lxc exec unheaded-wotan -- ip route
```

**WireGuard tunnel not working**
```bash
# Check WireGuard interface
lxc exec unheaded-wireguard -- wg show

# Check IPv6 connectivity
lxc exec unheaded-wireguard -- ping6 fd00:dead:beef::2

# View system logs
lxc exec unheaded-wireguard -- journalctl -u wireguard -n 50
```

### GPU Not Detected

**Symptoms:** vLLM container can't access GPU

**Diagnosis:**
```bash
# Check host GPU
lspci | grep -i amd
rocm-smi

# Check container GPU access
lxc exec unheaded-vllm-deepseek -- rocm-smi

# Check device permissions
lxc exec unheaded-vllm-deepseek -- ls -la /dev/kfd /dev/dri/
```

**Solutions:**
- Verify GPU profile is applied: `lxc profile show unheaded-gpu`
- Check host ROCm installation: `which rocm-smi`
- Verify /dev/kfd exists on host: `ls /dev/kfd`
- Check container logs: `lxc exec unheaded-vllm-deepseek -- journalctl -u vllm -n 50`

### Storage Issues

**Container out of space**
```bash
# Check container disk usage
lxc exec unheaded-wotan -- df -h /

# Check storage pool
lxc storage show unheaded-ssd

# Expand container root disk
lxc config device set unheaded-wotan root size 30GB
```

**High I/O latency**
```bash
# Check if using spinning disk
lxc storage show unheaded-ssd | grep source

# Monitor I/O
iotop -p $(pgrep -f "lxc")

# Ensure SSD pool: set `source: /dev/nvme0n1` or similar
```

### Telemetry Stack Issues

**Prometheus not scraping services**
```bash
# Check Prometheus config
lxc exec unheaded-prometheus -- cat /etc/prometheus/prometheus.yml

# Test service endpoint
curl http://10.20.1.10:9100/metrics

# Check Prometheus targets
curl http://10.20.1.50:9090/api/v1/targets
```

**Grafana can't connect to datasources**
```bash
# Check datasources
lxc exec unheaded-grafana -- cat /etc/grafana/provisioning/datasources/prometheus.yml

# Test connection from Grafana
lxc exec unheaded-grafana -- curl -v http://10.20.1.50:9090
```

**Loki logs not appearing**
```bash
# Check Loki is running
lxc exec unheaded-loki -- systemctl status loki

# Check ingestion
curl -s 'http://10.20.1.52:3100/loki/api/v1/query' --data-urlencode 'query={job="unheaded"}' | jq .
```

### Performance Tuning

**Optimize for throughput**
```bash
# Increase container memory limits
for svc in wotan ares hermes; do
  lxc config set unheaded-$svc limits.memory 4GB
done

# Increase VM memory
lxc config set unheaded-victoriametrics limits.memory 16GB
```

**Optimize for latency**
```bash
# Increase CPU allocation
lxc config set unheaded-wotan limits.cpu 4

# Reduce vLLM batch size
lxc config set unheaded-vllm-deepseek environment.VLLM_MAX_NUM_BATCHED_TOKENS 4096
```

**Monitor performance**
```bash
# Container stats
lxc list unheaded- --format=json | jq '.[] | {name, cpu, memory}'

# Detailed metrics
lxc info unheaded-wotan --show-log

# System metrics (from host)
make logs SVC=prometheus
curl http://10.20.1.53:3000  # Grafana dashboards
```

---

## Advanced Configuration

### Custom Service Profiles

Create additional profiles for specialized workloads:

```bash
# High-memory profile
lxc profile create unheaded-highmem
lxc profile set unheaded-highmem limits.cpu 8
lxc profile set unheaded-highmem limits.memory 16GB

# Launch with multiple profiles
lxc launch -f telemetry/victoriametrics-lxd.yaml \
  --profile unheaded-base \
  --profile unheaded-highmem \
  ubuntu:24.04 unheaded-victoriametrics
```

### Custom Network Topology

To use different network segments:

1. Create custom bridge:
   ```bash
   lxc network create custom-br ipv4.address=192.168.1.1/24
   lxc network attach custom-br unheaded-base eth1
   ```

2. Update container config:
   ```yaml
   cloud-init.network-config: |
     version: 2
     ethernets:
       eth1:
         addresses:
           - 192.168.1.10/24
   ```

### Persistent Storage

For containers requiring persistent state:

```bash
# Create storage volume
lxc storage volume create unheaded-ssd prometheus-data

# Attach to container
lxc config device add unheaded-prometheus data disk pool=unheaded-ssd \
  source=prometheus-data path=/var/lib/prometheus
```

### Backup and Restore

**Backup all containers**
```bash
for container in $(lxc list unheaded- -cn); do
  lxc snapshot $container backup-$(date +%Y%m%d)
done
```

**Restore from snapshot**
```bash
lxc restore unheaded-wotan backup-20250226
```

---

## Support and Troubleshooting

For issues:

1. Check container logs: `make logs SVC=SERVICE_NAME`
2. Verify networking: `lxc network list` and `lxc list`
3. Inspect cloud-init: `lxc exec CONTAINER -- cat /var/log/cloud-init-output.log`
4. Review LXD daemon: `journalctl -u snap.lxd.daemon`
5. Check disk/CPU: `lxc info CONTAINER`

Common fixes:
- Restart container: `lxc restart unheaded-SERVICE`
- Refresh profiles: `lxc profile refresh unheaded-base`
- Rebuild network: `lxc network delete unheaded-br && make networks`
- Full reset: `make clean && make init-host-a`

---

## License

SPDX-License-Identifier: MIT

All LXD infrastructure files, scripts, and configurations are released under the MIT License.

---

**Unheaded Kingdom LXD Infrastructure** | v1.0 | Feb 2025
