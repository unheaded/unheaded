# Unheaded Kingdom LXD Configuration - File Manifest

Complete listing of all configuration files created for the Unheaded Kingdom project.

## Directory Structure

```
lxd/hosts/
├── README.md                           # Main documentation and quick start
├── ARCHITECTURE.md                     # Detailed system architecture
├── DEPLOYMENT_CHECKLIST.md             # Step-by-step deployment guide
├── FILE_MANIFEST.md                    # This file
│
├── host-a/                             # The Forge (full stack)
│   ├── preseed.yaml                    # LXD initialization preseed (1.5KB)
│   ├── init.sh                         # Bootstrap script (4.4KB)
│   ├── launch-all.sh                   # Deploy all 25 services (7.9KB)
│   └── static-ips.yaml                 # IP address reference (2.9KB)
│
└── host-b/                             # The Outpost (minimal)
    ├── preseed.yaml                    # Minimal LXD preseed (1.6KB)
    ├── init.sh                         # Bootstrap with WireGuard setup (5.7KB)
    └── launch-minimal.sh                # Deploy 6 core + telemetry (6.1KB)
```

## File Details

### Root Level Documentation

#### README.md (15KB)
**Purpose**: Main entry point and quick reference

**Contents**:
- Project overview and quick start guides
- File descriptions and usage instructions
- Resource allocation tables
- Networking configuration
- Container lifecycle management
- Monitoring setup information
- Troubleshooting commands
- Advanced topics (scaling, storage, etc.)

**Key Sections**:
- Quick Start (for both host-a and host-b)
- File Descriptions (what each config file does)
- Container Resource Allocation (tables with CPU/memory)
- Networking (bridge configuration and WireGuard)
- Container Lifecycle (startup, management, cleanup)
- Monitoring and Observability
- Troubleshooting

#### ARCHITECTURE.md (12KB)
**Purpose**: Technical architecture and design overview

**Contents**:
- System overview diagram
- host-a detailed specification (25 services)
- host-b detailed specification (6 core + telemetry)
- Service breakdown by phase (10 phases)
- Service dependency chains
- Inter-host communication
- Performance metrics and scaling considerations
- High availability considerations

**Key Sections**:
- System Overview (ASCII diagram)
- host-a: The Forge (full specifications)
- host-b: The Outpost (minimal specifications)
- Service Dependencies (tree diagrams)
- Inter-Host Communication (WireGuard details)
- Performance Metrics (resource usage tables)
- Scaling Considerations

#### DEPLOYMENT_CHECKLIST.md (18KB)
**Purpose**: Step-by-step deployment verification guide

**Contents**:
- Pre-deployment preparation checklist
- 11-step deployment procedure for host-a
- 12-step deployment procedure for host-b
- Post-deployment verification
- Security hardening checklist
- Performance optimization checks
- Backup and recovery planning
- Troubleshooting commands
- Success criteria

**Key Sections**:
- Pre-Deployment (environment and repository setup)
- host-a Deployment (11 steps with verification)
- host-b Deployment (12 steps with verification)
- Post-Deployment (verification and hardening)
- Troubleshooting Commands
- Success Criteria

#### FILE_MANIFEST.md
**Purpose**: This file - complete listing and descriptions

---

### host-a Configuration (The Forge)

#### preseed.yaml (1.5KB)
**Purpose**: LXD automated initialization preseed for host-a

**Usage**: `sudo lxd init --preseed < preseed.yaml`

**Configures**:
- Core LXD settings (HTTPS address, image caching)
- Network: "unheaded" bridge (10.20.0.0/16, fd00:dead:beef:1::/64)
- Storage: "unheaded-ssd" ZFS pool (200GB)
- Default profile: 2 CPU, 512MB memory, 8GB disk per container
- 30 total containers (25 services + 5 telemetry)

**Key Settings**:
```yaml
core.https_address: "[::]:8443"
networks:
  - name: unheaded
    ipv4.address: 10.20.0.1/16
    ipv6.address: fd00:dead:beef:1::1/64
storage_pools:
  - name: unheaded-ssd
    driver: zfs
    size: 200GB
```

#### init.sh (4.4KB)
**Purpose**: Idempotent bootstrap script for host-a

**Usage**: `sudo ./init.sh`

**Steps Performed**:
1. Checks if running as root
2. Installs LXD 5.21 via snap (if not installed)
3. Adds current user to lxd group
4. Applies preseed.yaml configuration
5. Creates /opt/unheaded/bin/ directory
6. Copies service binaries from ../../../bin/
7. Loads profiles from ../../profiles/ directory

**Features**:
- Full color output (ANSI codes)
- Error handling with set -euo pipefail
- Idempotent (safe to run multiple times)
- Provides success summary with next steps

**Exit Codes**:
- 0: Success
- 1: Failed (not root, missing preseed, etc.)

#### launch-all.sh (7.9KB)
**Purpose**: Deploy all 25 services + telemetry containers on host-a

**Usage**: `./launch-all.sh`

**10 Launch Phases**:
1. **Phase 1**: Message bus (wotan)
2. **Phase 2**: Protocol layer (monad, sophia, anamnesis)
3. **Phase 3**: eBPF control (shield, unheaded-daemon)
4. **Phase 4**: Routing (gateway, service-discovery)
5. **Phase 5**: Presentation (dashboard-backend, frontend, protocol-api)
6. **Phase 6**: Observability (trace-collector, doom)
7. **Phase 7**: Services batch 1 (kanban, captain, micromanager, timeguru, architect, developer)
8. **Phase 8**: Services batch 2 (lore, busboy, kingdom, blackmage, moatghost)
9. **Phase 9**: Adversary (lich - last)
10. **Phase 10**: Telemetry stack (prometheus, grafana, loki, victoriametrics, ebpf-exporter)

**Features**:
- Dependency-aware launch order
- Per-service resource limits (CPU, memory)
- Profile application (ebpf, gpu)
- Cloud-init wait with 60-second timeout
- Binary push and systemd service start
- Graceful error handling (continues on failure)
- Color-coded output
- Final status table with results

**Exit Codes**:
- 0: All services launched successfully
- 1: Some services failed (details printed)

#### static-ips.yaml (2.9KB)
**Purpose**: Reference document with deterministic IP assignments

**Usage**: Reference for Prometheus scraping, service discovery, etc.

**Contents**: All 30 containers with:
- IPv4 address (10.20.1.x)
- IPv6 address (fd00:dead:beef:1::1xx)
- Primary ports
- Secondary ports (for multi-port services)

**Container Ranges**:
- Services: 10.20.1.1-10.20.1.25 (25 services)
- Exporter: 10.20.1.26 (ebpf-exporter)
- Telemetry: 10.20.1.50-10.20.1.53 (4 services)

---

### host-b Configuration (The Outpost)

#### preseed.yaml (1.6KB)
**Purpose**: Minimal LXD initialization preseed for host-b

**Usage**: `sudo lxd init --preseed < preseed.yaml`

**Configures**:
- Core LXD settings (optimized for minimal resources)
- Network: "unheaded-outpost" bridge (10.21.0.0/16, fd00:dead:beef:2::/64)
- Storage: "unheaded-minimal" ZFS pool (50GB)
- Default profile: 1 CPU, 256MB memory, 4GB disk per container
- 9 total containers (6 core + 3 telemetry agents)

**Key Differences from host-a**:
- Smaller resources per container
- Separate subnet (10.21.x.x vs 10.20.x.x)
- Smaller storage pool (50GB vs 200GB)
- Fewer image cache settings

#### init.sh (5.7KB)
**Purpose**: Minimal bootstrap script for host-b with WireGuard setup

**Usage**: `sudo ./init.sh`

**Steps Performed**:
1. Checks if running as root
2. Installs LXD 5.21 via snap (if not installed)
3. Applies preseed.yaml configuration
4. Creates /opt/unheaded/bin/ directory
5. Copies service binaries from ../../../bin/
6. **NEW**: Configures WireGuard tunnel to host-a
7. Loads profiles from ../../profiles/ directory

**WireGuard Setup**:
- Checks for wireguard.conf in same directory
- Installs wireguard-tools if needed
- Creates /etc/wireguard/ with proper permissions
- Brings up wg0 interface
- Logs connection status

**Features**:
- Full color output (ANSI codes)
- Error handling with set -euo pipefail
- Idempotent (safe to run multiple times)
- WireGuard optional (skipped if no config)

#### launch-minimal.sh (6.1KB)
**Purpose**: Deploy 6 core services + 3 telemetry agents on host-b

**Usage**: `./launch-minimal.sh`

**4 Launch Phases**:
1. **Phase 1**: Message bus (wotan - local)
2. **Phase 2**: Protocol layer (monad, sophia, anamnesis - local)
3. **Phase 3**: Gateway and dashboard (gateway, dashboard-backend)
4. **Phase 4**: Telemetry agents (prometheus-agent, promtail, node-exporter)

**Telemetry Agent Configuration**:
- **Prometheus Agent**: Agent mode with remote_write to host-a Prometheus (10.20.1.50:9090)
- **Promtail**: Configured to ship logs to host-a Loki (10.20.1.52:3100)
- **Node Exporter**: Exports host metrics for host-a Prometheus scraping

**Features**:
- Simplified dependency chain for minimal deployments
- Smaller resource allocations (1-2 CPU, 256MB-512MB memory)
- Graceful error handling
- Color-coded output
- Final status with configuration notes
- Clarifies remote telemetry setup in output

**Exit Codes**:
- 0: All services launched successfully
- 1: Some services failed (details printed)

---

## File Statistics

| File | Type | Size | Lines | Purpose |
|------|------|------|-------|---------|
| README.md | MD | ~15KB | ~450 | Main documentation |
| ARCHITECTURE.md | MD | ~12KB | ~380 | Technical architecture |
| DEPLOYMENT_CHECKLIST.md | MD | ~18KB | ~550 | Step-by-step guide |
| FILE_MANIFEST.md | MD | (this) | ~350 | File listing |
| host-a/preseed.yaml | YAML | 1.5KB | 46 | LXD init preseed |
| host-a/init.sh | Bash | 4.4KB | 155 | Bootstrap script |
| host-a/launch-all.sh | Bash | 7.9KB | 290 | Service deployment |
| host-a/static-ips.yaml | YAML | 2.9KB | 85 | IP reference |
| host-b/preseed.yaml | YAML | 1.6KB | 48 | Minimal preseed |
| host-b/init.sh | Bash | 5.7KB | 200 | Bootstrap + WireGuard |
| host-b/launch-minimal.sh | Bash | 6.1KB | 240 | Minimal deployment |
| **Total** | | **~75KB** | **~2,500** | Complete configuration set |

---

## Required Dependencies

### On deployment machine (where you copy files to):
- Ubuntu 24.04 LTS or Debian 12+
- Sufficient disk space (host-a: 200GB+, host-b: 50GB+)
- Snap daemon (for LXD installation)
- SSH access (to copy files)

### On host-a:
- 16+ CPU cores
- 64GB RAM
- 200GB storage
- Optional: RX 7700 XT GPU (for compute services)

### On host-b:
- 4+ CPU cores
- 8GB RAM
- 50GB storage
- Network connectivity to host-a

### Service binaries required:
- 25 service binaries for host-a
- 9 service binaries for host-b (6 core + 3 telemetry)
- All binaries must exist at repo/bin/ before deployment

---

## Usage Quick Reference

### Deploy host-a (full stack)
```bash
# 1. Copy files and binaries
scp -r lxd/hosts/host-a/ user@forge:/tmp/
scp -r lxd/profiles/ user@forge:/tmp/host-a/
scp -r bin/ user@forge:/tmp/host-a/

# 2. Initialize
ssh user@forge "cd /tmp/host-a && sudo ./init.sh"

# 3. Launch services
ssh user@forge "cd /tmp/host-a && ./launch-all.sh"

# 4. Verify
ssh user@forge "lxc list | grep unheaded"
```

### Deploy host-b (minimal)
```bash
# 1. Create and copy WireGuard config
# (Prepare host-b/wireguard.conf)

# 2. Copy files and binaries
scp -r lxd/hosts/host-b/ user@outpost:/tmp/
scp lxd/hosts/host-b/wireguard.conf user@outpost:/tmp/host-b/
scp -r bin/ user@outpost:/tmp/host-b/

# 3. Initialize
ssh user@outpost "cd /tmp/host-b && sudo ./init.sh"

# 4. Launch services
ssh user@outpost "cd /tmp/host-b && ./launch-minimal.sh"

# 5. Verify
ssh user@outpost "lxc list | grep unheaded"
ssh user@outpost "wg show wg0"
```

---

## License

All configuration files are licensed under MIT License:
```
SPDX-License-Identifier: MIT
Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
```

