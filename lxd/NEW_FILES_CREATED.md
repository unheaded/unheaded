# Unheaded Kingdom LXD Infrastructure — Newly Created Files

## Overview

This document summarizes the infrastructure files created for the Unheaded Kingdom LXD deployment, focusing on the newly added telemetry, WireGuard, vLLM, and operational files.

**Total New Files Created:** 14
**Date:** 2025-02-26
**Base Directory:** `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/lxd/`

---

## Telemetry Stack (4 containers + setup)

### 1. `telemetry/prometheus-lxd.yaml`
**Purpose:** Prometheus metrics scraper for all 25 service containers
- **Static IP:** 10.20.1.50, Port: 9090
- **Resources:** 2 CPUs, 4GB RAM, 20GB storage
- **Features:**
  - Scrapes all 25 services every 5 seconds
  - Scrapes host node-exporter at 10.20.0.1:9100
  - Remote-writes to VictoriaMetrics at 10.20.1.51:8428
  - Bind-mounts prometheus.yml from host at `/etc/unheaded/prometheus/`

**prometheus.yml Configuration (documented in YAML):**
- `scrape_interval: 5s`
- `evaluation_interval: 5s`
- External labels: `cluster: 'unheaded-kingdom'`
- Scrapes all 25 services by static IP (10.20.1.10-34)
- Remote write to VictoriaMetrics with queue configuration

### 2. `telemetry/victoriametrics-lxd.yaml`
**Purpose:** Long-term metrics storage and compression
- **Static IP:** 10.20.1.51, Port: 8428
- **Resources:** 4 CPUs, 8GB RAM, 50GB storage
- **Features:**
  - 90-day retention period (2160 hours)
  - Installed via cloud-init
  - Systemd service for auto-start
  - High-performance time-series database

### 3. `telemetry/loki-lxd.yaml`
**Purpose:** Log aggregation and analysis
- **Static IP:** 10.20.1.52, Port: 3100
- **Resources:** 2 CPUs, 4GB RAM, 20GB storage
- **Features:**
  - TSDB schema v13 (modern Loki)
  - 90-day retention period
  - Auth disabled for internal use
  - 16MB/s ingestion rate limit
  - Pre-configured loki.yml with filesystem storage

### 4. `telemetry/grafana-lxd.yaml`
**Purpose:** Visualization and dashboarding
- **Static IP:** 10.20.1.53, Port: 3000
- **Resources:** 2 CPUs, 2GB RAM, 15GB storage
- **Features:**
  - Dark theme by default
  - Pre-configured admin: `admin:unheaded-kingdom`
  - Bind-mounts provisioning directory from host
  - Datasources: Prometheus, VictoriaMetrics, Loki
  - Auto-discovery of dashboards

### 5. `telemetry/setup-telemetry.sh`
**Purpose:** Automated deployment script for telemetry stack
- **Execution:** Run on host-a to launch all 4 containers
- **Features:**
  - Creates host directories for bind-mounted configs
  - Writes prometheus.yml with all 25 service targets
  - Writes Grafana datasources configuration
  - Launches containers in correct order
  - Waits for services to initialize
  - Provides access point URLs

**Usage:**
```bash
cd lxd/telemetry
bash setup-telemetry.sh
```

---

## WireGuard East-West Bridge (1 container + templates)

### 6. `wireguard/wireguard-lxd.yaml`
**Purpose:** IPv6 tunnel bridging host-a and host-b
- **Dual-role:**
  - Host-a: Server (listens on 51820)
  - Host-b: Client (connects to host-a IP)
- **Static IPs:**
  - Host-a: 10.20.1.100/24
  - Host-b: 10.20.2.100/24
- **IPv6 Tunnel:** fd00:dead:beef::/48
- **Resources:** 2 CPUs, 1GB RAM, 10GB storage
- **Features:**
  - Cloud-init dynamically selects server/client config
  - TUN device passthrough
  - Raw LXC capabilities for net_admin
  - IPv6 NAT and forwarding rules
  - PersistentKeepalive: 25 seconds

**Environment Variables:**
- `WG_ROLE`: "server" (host-a) or "client" (host-b)
- `WG_PRIVATE_KEY`: Base64-encoded private key
- `WG_PEER_PUBLIC_KEY`: Peer public key
- `WG_ENDPOINT_ADDR`: Public IP of host-a (for client)
- `WG_LISTEN_PORT`: 51820 (standard)

**Security Model:**
- Not privileged, but has `cap_net_admin + cap_sys_module`
- Raw LXC config for TUN device mounting
- `/dev/net/tun` device passthrough

### 7. `wireguard/wg0-server.conf.template`
**Purpose:** WireGuard server config template (host-a)
- **Interface Address:** fd00:dead:beef::1/48
- **Listen Port:** 51820
- **Features:**
  - IPv6 NAT and forwarding rules
  - PostUp/PostDown iptables rules
  - MTU: 1380 (optimized for tunnel overhead)
  - Peer configuration with AllowedIPs

### 8. `wireguard/wg0-client.conf.template`
**Purpose:** WireGuard client config template (host-b)
- **Interface Address:** fd00:dead:beef::2/48
- **Features:**
  - DNS: 8.8.8.8, 8.8.4.4
  - PersistentKeepalive for NAT traversal
  - AllowedIPs: Server subnet + host-a server IPs
  - Dynamic endpoint configuration

---

## vLLM/ROCm GPU Inference (1 container + setup)

### 9. `vllm-rocm/vllm-lxd.yaml`
**Purpose:** DeepSeek-R1-7B inference on RX 7700 XT
- **Container Name:** unheaded-vllm-deepseek
- **Image:** Ubuntu 22.04 LTS (ROCm preference)
- **Static IP:** 10.20.1.99, Port: 20100
- **Resources:** 8 CPUs, 16GB RAM, 80GB storage + GPU
- **Auto-start:** Yes (delay: 30s, priority: 5)

**ROCm Configuration:**
- `HSA_OVERRIDE_GFX_VERSION: "11.0.1"` (RX 7700 XT)
- `PYTORCH_ROCM_ARCH: "gfx1101"`
- `HIP_VISIBLE_DEVICES: "0"`
- `ROCM_VERSION: "6.1.3"`

**Device Passthrough:**
- GPU device (AMD Radeon)
- `/dev/kfd` (Kernel Fusion Driver)
- `/dev/dri/renderD128` (DRI render device)
- `/dev/shm` (4GB tmpfs for inference)
- `/data/unheaded/models` (bind-mount for model weights)

**Model Configuration:**
- Model: `deepseek-ai/DeepSeek-R1-Distill-Llama-7B`
- Quantization: AWQ (post-training)
- Max Model Length: 8192 tokens
- GPU Memory Utilization: 85%
- API Port: 20100 (OpenAI-compatible)

**Cloud-Init Features:**
- Installs ROCm 6.1.3
- Installs PyTorch with ROCm support
- Installs vLLM with ROCm backend
- Creates systemd service for auto-start
- Sets up vllm user with proper permissions

### 10. `vllm-rocm/setup-vllm.sh`
**Purpose:** Automated vLLM deployment script
- **Execution:** Run on host-a to launch vLLM container
- **Steps:**
  1. Creates `/data/unheaded/models` on host
  2. Sets permissions for container access
  3. Launches vLLM container
  4. Waits for cloud-init completion (2-10 minutes)
  5. Checks vLLM service status
  6. Verifies GPU access (rocm-smi)
  7. Provides testing instructions

**Usage:**
```bash
cd lxd/vllm-rocm
bash setup-vllm.sh
```

**Access:**
- API Endpoint: `http://10.20.1.99:20100`
- OpenAI-compatible: `/v1/models`, `/v1/chat/completions`, etc.

---

## Operations & Documentation (3 files)

### 11. `Makefile`
**Purpose:** Centralized operations automation for all containers
- **Targets:** 25+ make targets for setup, launch, operations, and cleanup

**Setup Targets:**
- `init-host-a` — Initialize host-a (Forge) full setup
- `init-host-b` — Initialize host-b (Outpost) minimal setup
- `profiles` — Verify LXD profiles
- `networks` — Verify LXD network

**Launch Targets:**
- `launch-all` — Launch 30 containers (25 services + 4 telemetry + 1 WireGuard)
- `launch-minimal` — Launch 6 core containers (host-b)
- `launch-telemetry` — Launch telemetry stack only
- `launch-vllm` — Launch vLLM/ROCm container

**Operations Targets:**
- `status` — Show all containers (with CPU/memory/IP)
- `stop` — Stop all containers
- `restart` — Restart all containers
- `logs SVC=name` — Tail logs for a service
- `exec SVC=name` — Enter container shell
- `snapshot` — Snapshot all containers with timestamp

**Cleanup Targets:**
- `destroy` — Delete all containers (with confirmation)
- `clean` — Delete containers + profiles + network

**Features:**
- Color-coded output (RED/GREEN/YELLOW)
- Uses `lxc` commands directly
- Bash-compatible shell scripts
- Error checking and user confirmation

### 12. `README.md`
**Purpose:** Comprehensive LXD infrastructure documentation (4000+ words)

**Sections:**
1. **Prerequisites** — System requirements, Ubuntu 24.04, LXD 5.x, GPU
2. **Quick Start** — 5-minute setup for host-a and host-b
3. **Directory Structure** — File organization and purposes
4. **Deployment Options** — Comparison table (NixOS vs Docker vs LXD)
5. **LXD Concepts** — Profiles, cloud-init, bind-mounts, static IPs
6. **Network Topology** — ASCII diagram and network segments
7. **Resource Summary** — CPU/RAM/storage breakdown for all 35 containers
8. **Operations** — Common tasks and manual operations
9. **Troubleshooting** — Container startup, networking, GPU, storage issues
10. **Advanced Configuration** — Custom profiles, networks, persistent storage

**Notable Features:**
- Detailed network topology ASCII diagram
- Resource inventory table (35 containers)
- Side-by-side comparison with other deployment options
- 15+ troubleshooting scenarios with solutions
- Performance tuning recommendations

### 13. `NEW_FILES_CREATED.md` (this file)
**Purpose:** Summary of newly created infrastructure files
- **Created:** 2025-02-26
- **Coverage:** Telemetry, WireGuard, vLLM, Makefile, README

---

## File Statistics

### By Type
- **YAML Container Definitions:** 5 files
- **Configuration Templates:** 2 files
- **Shell Scripts:** 2 files
- **Makefile:** 1 file
- **Documentation:** 2 files (README + this file)

### By Category
- **Telemetry Stack:** 4 containers + 1 setup script
- **WireGuard:** 1 container + 2 config templates
- **vLLM/ROCm:** 1 container + 1 setup script
- **Operations:** 1 Makefile + 2 docs

### By Size (approximate)
- `prometheus-lxd.yaml` — 100 lines
- `victoriametrics-lxd.yaml` — 75 lines
- `loki-lxd.yaml` — 110 lines
- `grafana-lxd.yaml` — 115 lines
- `setup-telemetry.sh` — 150 lines
- `wireguard-lxd.yaml` — 130 lines
- `wg0-server.conf.template` — 25 lines
- `wg0-client.conf.template` — 25 lines
- `vllm-lxd.yaml` — 145 lines
- `setup-vllm.sh` — 110 lines
- `Makefile` — 220 lines
- `README.md` — 1100+ lines
- **Total:** 2300+ lines of infrastructure code

---

## Integration Points

### Service Discovery
All 25 services are automatically discovered by Prometheus via:
- Static IP configuration in cloud-init
- `/etc/unheaded/prometheus/prometheus.yml` (bind-mounted)
- Service naming: `unheaded-{service-name}`

### Telemetry Collection Flow
```
Service Container (10.20.1.10-34)
        ↓ (metrics :9100)
   Prometheus (10.20.1.50)
        ↓ (remote_write)
  VictoriaMetrics (10.20.1.51)
        ↓
   Grafana (10.20.1.53) ← Visualization
                     ↓
                Loki (10.20.1.52) ← Logs
```

### Encryption Flow
```
Host-A Services (10.20.1.10-34)
        ↓ (IPv6 tunnel)
   WireGuard Server (10.20.1.100 ← fd00:dead:beef::1)
        ↓ (encrypted tunnel :51820)
   WireGuard Client (10.20.2.100 ← fd00:dead:beef::2)
        ↓
Host-B Services (10.20.2.10-15)
```

### GPU Inference Flow
```
Client Request (http://10.20.1.99:20100/v1/chat/completions)
        ↓
   vLLM Container (10.20.1.99)
        ↓
 ROCm Runtime (RX 7700 XT via /dev/kfd)
        ↓
DeepSeek-R1-7B Model (/models/deepseek-r1-7b-awq)
        ↓
OpenAI-Compatible Response
```

---

## Deployment Checklist

### Pre-Deployment
- [ ] Ubuntu 24.04 LTS on host-a and host-b
- [ ] LXD 5.x installed via snap
- [ ] LXD initialized with `lxd init`
- [ ] Storage pool created (SSD recommended)
- [ ] Network bridge configured
- [ ] Profiles created

### Host-a Deployment
- [ ] Run `make init-host-a`
- [ ] Run `make launch-all`
- [ ] Verify: `make status`
- [ ] Run `make launch-telemetry`
- [ ] Test: `curl http://10.20.1.50:9090` (Prometheus)
- [ ] Run `make launch-vllm` (optional, GPU required)
- [ ] Test: `curl http://10.20.1.99:20100/v1/models` (vLLM)

### Host-b Deployment
- [ ] Run `make init-host-b`
- [ ] Run `make launch-minimal`
- [ ] Verify: `make status`
- [ ] Configure WireGuard keys (in progress)

### Post-Deployment
- [ ] Verify all containers running
- [ ] Check container networking
- [ ] Verify telemetry collection
- [ ] Test inter-host communication (WireGuard)

---

## Next Steps

1. **Execute telemetry deployment:**
   ```bash
   cd lxd/telemetry
   bash setup-telemetry.sh
   ```

2. **Execute vLLM deployment (optional):**
   ```bash
   cd lxd/vllm-rocm
   bash setup-vllm.sh
   ```

3. **Monitor deployments:**
   ```bash
   make status
   make logs SVC=prometheus
   ```

4. **Configure WireGuard:**
   - Generate keys
   - Update wireguard-lxd.yaml with WG_PRIVATE_KEY and WG_PEER_PUBLIC_KEY
   - Deploy to both hosts

5. **Access dashboards:**
   - Grafana: http://10.20.1.53:3000 (admin:unheaded-kingdom)
   - Prometheus: http://10.20.1.50:9090
   - Loki: http://10.20.1.52:3100

---

## License

SPDX-License-Identifier: MIT

All Unheaded Kingdom LXD infrastructure files are released under the MIT License.

---

**Created:** 2025-02-26
**Base Path:** `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/lxd/`
**Total Files:** 14 new + existing infrastructure files
