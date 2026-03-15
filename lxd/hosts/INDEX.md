# Unheaded Kingdom LXD Host Configuration - Complete Index

Generated: February 26, 2026
Base Directory: `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/lxd/hosts/`

## Quick Navigation

### For First-Time Users
1. Start with **README.md** - Overview and quick start guide
2. Review **ARCHITECTURE.md** - Understand the system design
3. Follow **DEPLOYMENT_CHECKLIST.md** - Deploy step-by-step

### For Experienced Operators
1. Review **FILE_MANIFEST.md** - See all files and their purposes
2. Copy and run **host-a/init.sh** and **host-a/launch-all.sh**
3. Copy and run **host-b/init.sh** and **host-b/launch-minimal.sh**

### For Reference During Deployment
- **DEPLOYMENT_CHECKLIST.md** - Verification steps
- **ARCHITECTURE.md** - Service breakdown and dependencies
- **host-a/static-ips.yaml** - Container IP assignments
- **host-b/wireguard.conf.example** - Network tunnel template

---

## File Organization

### Documentation (4 files)
```
├── README.md
│   Quick reference, troubleshooting, common operations
│   Start here if unsure where to begin
│
├── ARCHITECTURE.md
│   Complete technical design and system overview
│   Service breakdown, dependencies, scaling considerations
│
├── DEPLOYMENT_CHECKLIST.md
│   Step-by-step deployment verification guide
│   Use during actual deployment to verify success
│
└── FILE_MANIFEST.md
    Complete file listing with detailed descriptions
    Reference for specific file information
```

### host-a Configuration (4 files)
```
host-a/
├── preseed.yaml
│   LXD automated initialization preseed
│   Usage: sudo lxd init --preseed < preseed.yaml
│
├── init.sh
│   Bootstrap script for host-a
│   Usage: sudo ./init.sh
│   Creates: LXD infrastructure, binary directories
│
├── launch-all.sh
│   Deploy all 25 services in 10 phases
│   Usage: ./launch-all.sh
│   Launches: 25 services + 5 telemetry containers
│
└── static-ips.yaml
    Reference document with deterministic IP assignments
    Contains: All container IPs, ports, and addresses
```

### host-b Configuration (4 files)
```
host-b/
├── preseed.yaml
│   Minimal LXD initialization preseed
│   Usage: sudo lxd init --preseed < preseed.yaml
│
├── init.sh
│   Bootstrap script with WireGuard tunnel setup
│   Usage: sudo ./init.sh
│   Creates: LXD infrastructure, WireGuard tunnel
│
├── launch-minimal.sh
│   Deploy 6 core services + 3 telemetry agents
│   Usage: ./launch-minimal.sh
│   Launches: 9 containers with remote telemetry
│
└── wireguard.conf.example
    WireGuard tunnel configuration template
    Usage: Copy to wireguard.conf, replace PLACEHOLDER values
```

---

## Deployment Summary

### host-a: The Forge (Full Stack)
- **Deployment Size**: 30 containers (25 services + 5 telemetry)
- **Resource Allocation**: 51 CPU cores, 23.5GB RAM, 200GB storage
- **Network**: 10.20.0.0/16 (unheaded bridge)
- **Hardware Requirement**: 16+ cores, 64GB RAM, 200GB+ SSD
- **Services Included**: All 25 Unheaded Kingdom services
- **Special Features**: GPU support, eBPF security, full observability stack

**Deployment Steps**:
1. Copy `host-a/` to target: `scp -r host-a/ user@host-a:/tmp/`
2. Initialize: `ssh user@host-a "cd /tmp/host-a && sudo ./init.sh"`
3. Deploy: `ssh user@host-a "cd /tmp/host-a && ./launch-all.sh"`
4. Verify: `ssh user@host-a "lxc list | grep unheaded | wc -l"` (should be 30)

### host-b: The Outpost (Minimal)
- **Deployment Size**: 9 containers (6 core + 3 telemetry agents)
- **Resource Allocation**: 10 CPU cores, 2.5GB RAM, 50GB storage
- **Network**: 10.21.0.0/16 (unheaded-outpost bridge) + WireGuard tunnel
- **Hardware Requirement**: 4+ cores, 8GB RAM, 50GB+ SSD
- **Services Included**: 6 core services (wotan, monad, sophia, anamnesis, gateway, dashboard)
- **Special Features**: Remote telemetry, WireGuard cluster connectivity

**Deployment Steps**:
1. Prepare WireGuard config (see `host-b/wireguard.conf.example`)
2. Copy `host-b/` to target: `scp -r host-b/ user@host-b:/tmp/`
3. Initialize: `ssh user@host-b "cd /tmp/host-b && sudo ./init.sh"`
4. Deploy: `ssh user@host-b "cd /tmp/host-b && ./launch-minimal.sh"`
5. Verify: `ssh user@host-b "lxc list | grep unheaded | wc -l"` (should be 9)

---

## Configuration Highlights

### Network Architecture
```
host-a (Forge)
├─ Network: 10.20.0.0/16
├─ Containers: 10.20.1.1-10.20.1.53
└─ Services: 25 + 5 telemetry

host-b (Outpost)
├─ Network: 10.21.0.0/16
├─ Containers: 10.21.1.1-10.21.1.9
├─ Services: 6 core + 3 telemetry
└─ Tunnel: WireGuard to host-a
```

### Service Dependency Chain (host-a)
```
wotan (message bus)
  ├─ monad, sophia, anamnesis (protocol)
  │   └─ shield, unheaded-daemon (eBPF)
  │       └─ gateway, service-discovery (routing)
  │           ├─ 11 business services
  │           └─ presentation layer (dashboard, api)
  │               └─ lich (adversary - last)
  │
  └─ prometheus, grafana, loki, victoriametrics (telemetry)
```

### Resource Allocation (host-a)
| Category | CPU | Memory | Storage |
|----------|-----|--------|---------|
| Message Bus (wotan) | 4 | 1GB | 8GB |
| Protocol Layer | 6 | 1.5GB | 24GB |
| eBPF Control | 8 | 3GB | 16GB |
| Routing & Presentation | 7 | 1.5GB | 56GB |
| Services & Compute | 20 | 12GB | 160GB |
| Telemetry Stack | 6 | 3.5GB | 40GB |
| **Total** | **51** | **23.5GB** | **240GB** |

---

## Key Features

### Automation & Reliability
- Idempotent initialization scripts (safe to run multiple times)
- Dependency-aware service launching (respects service order)
- Graceful error handling (continues on failures, reports at end)
- Health checks with cloud-init wait (60-second timeout)
- Binary push and systemd service startup (integrated)

### Observability & Monitoring
- Prometheus metrics collection
- Grafana dashboard visualization
- Loki log aggregation
- eBPF kernel-level metrics
- VictoriaMetrics time-series database

### Network & Connectivity
- Managed DNS (unheaded.internal domain)
- Static IP allocation (deterministic, documented)
- IPv4 and IPv6 support
- WireGuard secure tunneling (host-b → host-a)
- Inter-host cluster connectivity

### Resource Management
- Per-service CPU limits
- Per-service memory limits
- Per-service disk allocation (8GB/4GB)
- ZFS storage backend (snapshotting, compression)
- Overcommit suitable for cloud environments

### Security & Isolation
- Unprivileged LXD containers (default)
- Custom security profiles (eBPF, GPU)
- Firewall rules per network
- Trust password disabled in LXD config
- Service isolation via containers

---

## Typical Workflows

### Deploy Complete Stack
```bash
# Prepare binaries
make build-all
mkdir -p repo/bin/

# Deploy host-a
./lxd/hosts/host-a/init.sh
./lxd/hosts/host-a/launch-all.sh

# Deploy host-b (if multi-host)
./lxd/hosts/host-b/init.sh
./lxd/hosts/host-b/launch-minimal.sh

# Verify
lxc list | grep unheaded
```

### Monitor Deployment
```bash
# Watch container launches
watch -n 2 'lxc list | grep unheaded'

# Monitor logs
lxc logs unheaded-wotan --follow
lxc logs unheaded-shield --follow

# Check metrics
lxc exec unheaded-prometheus -- curl -s http://localhost:9090/api/v1/targets
```

### Access Dashboards
```bash
# Get host-a IP
lxc list | grep -E "^"

# Access Grafana (default admin/admin)
http://10.20.0.1:3000

# Access Prometheus
http://10.20.0.1:9090

# Access Loki
http://10.20.0.1:3100

# Access Dashboard Frontend
http://10.20.0.1:3001
```

### Troubleshoot Service
```bash
# Check service status
lxc exec unheaded-SERVICENAME -- systemctl status unheaded-SERVICENAME

# View logs
lxc exec unheaded-SERVICENAME -- journalctl -u unheaded-SERVICENAME -f

# Access container shell
lxc exec unheaded-SERVICENAME -- /bin/bash

# Check networking
lxc exec unheaded-SERVICENAME -- ip addr show
lxc exec unheaded-SERVICENAME -- ping -c 3 10.20.1.1
```

---

## Customization

### Adjusting Resource Limits
Edit `launch-all.sh` or `launch-minimal.sh` to change:
- `cpu_limit` parameter (default: 1-4 cores)
- `memory_limit` parameter (default: 256MB-2GB)
- `ebpf_profile` / `gpu_profile` flags (default: false)

### Adjusting Network Configuration
Edit `preseed.yaml` to change:
- `ipv4.address` (default: 10.20.0.0/16)
- `ipv4.dhcp.ranges` (default: 10.20.1.1-10.20.1.254)
- `ipv6.address` (default: fd00:dead:beef:1::/64)
- Storage pool size (default: 200GB/50GB)

### Adjusting WireGuard Tunnel
Edit `host-b/wireguard.conf` to change:
- Private/public keys
- Tunnel IP range (default: 10.50.0.0/24)
- Endpoint IP/port
- Keepalive interval

---

## Support & Help

### Documentation Files
| File | Purpose | Audience |
|------|---------|----------|
| README.md | Quick reference & troubleshooting | Everyone |
| ARCHITECTURE.md | Technical design overview | Architects, Devops |
| DEPLOYMENT_CHECKLIST.md | Verification steps | Operators |
| FILE_MANIFEST.md | Detailed file descriptions | Reference |

### Common Questions

**Q: How do I add a new service to host-a?**
- Add service container launch to `launch-all.sh`
- Add IP assignment to `static-ips.yaml`
- Update ARCHITECTURE.md documentation

**Q: How do I scale to more Outposts?**
- Duplicate `host-b/` configuration
- Adjust network subnet (10.22.x.x, 10.23.x.x, etc.)
- Configure WireGuard mesh networking

**Q: How do I backup containers?**
- Use ZFS snapshots: `zfs snapshot unheaded-ssd@backup-$(date +%s)`
- Use LXD snapshots: `lxc snapshot unheaded-SERVICENAME`

**Q: How do I update service binaries?**
- Update `/opt/unheaded/bin/SERVICENAME` on host
- Restart container service: `lxc exec unheaded-SERVICENAME -- systemctl restart unheaded-SERVICENAME`

---

## License & Attribution

All configuration files created for the Unheaded Kingdom project.

```
SPDX-License-Identifier: MIT
Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
```

---

## Version Information

- **LXD Version**: 5.21 (Incus-compatible)
- **Base Image**: ubuntu:24.04 LTS
- **OS Support**: Ubuntu 24.04 LTS, Debian 12+
- **Generated**: February 26, 2026
- **Configuration Type**: Production-ready

---

## Next Steps

1. **Read**: Start with README.md for overview
2. **Understand**: Review ARCHITECTURE.md for design
3. **Prepare**: Ensure hardware meets requirements
4. **Deploy**: Follow DEPLOYMENT_CHECKLIST.md
5. **Verify**: Use verification commands from README.md
6. **Monitor**: Access Grafana dashboards for metrics
7. **Document**: Record any customizations made

---

For questions or issues, refer to the inline documentation in each configuration file. All scripts include detailed comments and error messages to guide troubleshooting.

