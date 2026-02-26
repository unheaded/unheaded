# Unheaded Kingdom LXD Infrastructure-as-Code — Complete Index

**Base Directory:** `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/lxd/`  
**Created:** 2026-02-26  
**LXD Version Target:** 5.x (Incus-compatible)  
**25 Microservices Deployment**

---

## Quick Navigation

### Documentation (Start Here)
1. **[README.md](README.md)** — User guide, profiles overview, deployment workflow
2. **[MANIFEST.md](MANIFEST.md)** — 25-service catalog, deployment phases, health checks
3. **[CREATED_FILES.md](CREATED_FILES.md)** — Summary of all files, design decisions
4. **[LAUNCH_EXAMPLES.sh](LAUNCH_EXAMPLES.sh)** — Launch commands for all services
5. **[INDEX.md](INDEX.md)** — This file

### Infrastructure Files
- **[deploy.sh](deploy.sh)** — Automated initialization script
- **profiles/** — LXD profiles (5 files)
- **cloud-init/** — Cloud-init templates (3 files)
- **networks/** — Network definitions (1 file)
- **storage/** — Storage pool definitions (1 file)

---

## File Structure

```
lxd/
├── README.md                    ← Start here for overview
├── MANIFEST.md                  ← Service catalog
├── CREATED_FILES.md             ← File summary
├── INDEX.md                     ← This file (navigation)
├── LAUNCH_EXAMPLES.sh           ← Launch commands (all 25 services)
├── deploy.sh                    ← Automated setup
│
├── profiles/
│   ├── unheaded-base.yaml       ← Base (all 25 containers)
│   ├── unheaded-service.yaml    ← Standard microservices
│   ├── unheaded-ebpf.yaml       ← eBPF/network services
│   ├── unheaded-gpu.yaml        ← GPU/ROCm (host-a only)
│   └── unheaded-telemetry.yaml  ← Observability stack
│
├── cloud-init/
│   ├── base.yaml                ← Base initialization (all)
│   ├── ebpf.yaml                ← eBPF-specific packages
│   └── gpu.yaml                 ← GPU/ROCm setup
│
├── networks/
│   └── unheaded-bridge.yaml     ← LXD bridge (10.20.0.0/16, IPv6)
│
├── storage/
│   └── unheaded-ssd.yaml        ← ZFS storage pool
│
├── containers/                  ← (Pre-existing)
├── hosts/                       ← (Pre-existing)
└── ...
```

---

## Typical Workflows

### Scenario 1: Fresh LXD Host Setup

```bash
cd /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/lxd

# 1. Full infrastructure initialization
./deploy.sh init

# 2. Launch critical services (DB tier)
bash LAUNCH_EXAMPLES.sh all-data

# 3. Launch observability stack
bash LAUNCH_EXAMPLES.sh all-telemetry

# 4. Launch eBPF/network layer
bash LAUNCH_EXAMPLES.sh all-ebpf

# 5. Launch API services
bash LAUNCH_EXAMPLES.sh all-api

# 6. Launch GPU services (host-a only)
# On host-a:
bash LAUNCH_EXAMPLES.sh all-gpu

# Verify
lxc list
```

### Scenario 2: Add Individual Service

```bash
cd /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/lxd

# Launch specific service
bash LAUNCH_EXAMPLES.sh api-gateway

# Verify
lxc info api-gateway
lxc exec api-gateway -- systemctl status
```

### Scenario 3: Update Profile or Cloud-Init Template

```bash
# Edit profile
vim profiles/unheaded-service.yaml

# Update in LXD
lxc profile edit unheaded-service < profiles/unheaded-service.yaml

# For new containers (existing ones don't auto-update)
lxc launch images:ubuntu/24.04/cloud new-service \
  --profile unheaded-base \
  --profile unheaded-service \
  --config user.user-data="$(cat cloud-init/base.yaml)"
```

---

## 25 Microservices Quick Reference

| # | Service | Category | Profiles | Cloud-Init |
|---|---------|----------|----------|-----------|
| 1 | api-gateway | API | base, service | base |
| 2 | auth-service | API | base, service | base |
| 3 | user-service | API | base, service | base |
| 4 | catalog-service | API | base, service | base |
| 5 | order-service | API | base, service | base |
| 6 | payment-processor | API | base, service | base |
| 7 | notification-service | API | base, service | base |
| 8 | search-indexer | API | base, service | base |
| 9 | recommendation-engine | API | base, service | base |
| 10 | analytics-pipeline | API | base, service | base |
| 11 | shield | eBPF | base, ebpf | base + ebpf |
| 12 | unheaded-daemon | eBPF | base, ebpf | base + ebpf |
| 13 | ebpf-exporter | eBPF | base, ebpf | base + ebpf |
| 14 | network-tracer | eBPF | base, ebpf | base + ebpf |
| 15 | syscall-monitor | eBPF | base, ebpf | base + ebpf |
| 16 | postgres | Data | base, service | base |
| 17 | redis-primary | Data | base, service | base |
| 18 | redis-replica | Data | base, service | base |
| 19 | elasticsearch | Data | base, service | base |
| 20 | minio | Data | base, service | base |
| 21 | mongodb | Data | base, service | base |
| 22 | vllm-deepseek | GPU | base, gpu | base + gpu |
| 23 | embeddings-service | GPU | base, gpu | base + gpu |
| 24 | prometheus | Telemetry | base, telemetry | base |
| 25 | grafana/loki/victoria-metrics | Telemetry | base, telemetry/service | base |

---

## Profile Quick Reference

### `unheaded-base.yaml`
Applied to: **ALL 25 containers**

Key configs:
- Security: unprivileged, isolated idmap, capability drops
- CPU: 2 (default)
- RAM: 512MB (default)
- Network: `unheaded` bridge
- Storage: `unheaded-ssd` ZFS pool
- Environment: UNHEADED_ENV=production, LOG_FORMAT=json

### `unheaded-service.yaml`
Applied to: **API/Web (10), Data (6), Telemetry (2) = 18 containers**

Key configs:
- CPU: 2
- RAM: 768MB
- Priority: 6
- Observability: OTEL_ENABLED=true

### `unheaded-ebpf.yaml`
Applied to: **eBPF/Network (5 containers)**

Key configs:
- CPU: 4
- RAM: 2GB
- Capabilities: CAP_BPF, CAP_NET_ADMIN, CAP_PERFMON
- Mounts: /sys/fs/bpf, /sys/kernel/debug
- Priority: 8

### `unheaded-gpu.yaml`
Applied to: **GPU/LLM (2 containers, host-a only)**

Key configs:
- CPU: 8
- RAM: 16GB
- GPU: AMD RX 7700 XT (gfx1101)
- Passthrough: /dev/kfd, /dev/dri/renderD128
- Priority: 9

### `unheaded-telemetry.yaml`
Applied to: **Telemetry (Prometheus, Loki, VictoriaMetrics)**

Key configs:
- CPU: 4
- RAM: 4GB
- Storage: 50GB persistent
- Retention: 30 days
- Priority: 7

---

## Cloud-Init Quick Reference

### `base.yaml`
Applied to: **ALL containers**

Configures:
- User: unheaded (uid 10001)
- Packages: curl, ca-certificates, tini
- Directories: /opt/unheaded/bin, /var/lib/unheaded
- systemd limits: NOFILE=65536

### `ebpf.yaml`
Applied to: **eBPF containers (in addition to base.yaml)**

Installs:
- Build tools: clang, llvm, linux-headers
- BPF tools: libbpf-dev, bpftool
- Configures: /sys/fs/bpf mount, BPF environment vars

### `gpu.yaml`
Applied to: **GPU containers (in addition to base.yaml)**

Installs:
- ROCm: rocm-core, rocm-libs, rocm-dev
- Tools: rocm-smi, rocminfo
- Configures: HIP environment, group membership

---

## Network Configuration

**Bridge Name:** `unheaded`

**IPv4:**
- Gateway: 10.20.0.1
- Network: 10.20.0.0/16
- DHCP Pool: 10.20.1.0-10.20.1.250
- NAT: enabled
- Firewall: enabled

**IPv6:**
- Gateway: fd00:dead:beef:1::1
- Network: fd00:dead:beef:1::/64
- DHCPv6: stateful
- NAT: disabled (routed)
- Firewall: enabled

**DNS:**
- Domain: unheaded.internal
- Managed by LXD
- Containers accessible by name (e.g., api-gateway.unheaded.internal)

**Multi-Host:**
- Host-A: Primary (runs unheaded bridge)
- Host-B: Secondary (bridges via WireGuard to Host-A)

---

## Storage Configuration

**Pool Name:** `unheaded-ssd`

**Driver:** ZFS

**Config:**
- Default volume size: 8GB per container
- Filesystem: ext4
- Refquota: enabled (per-container quotas)
- Clone copy: optimized (fast container creation)
- Snapshots: retained (manual snapshots, not auto-purged)

**Usage:**
```bash
# List pools
lxc storage list

# Check pool info
lxc storage show unheaded-ssd

# Check container volumes
lxc storage volume list unheaded-ssd
```

---

## Security Model Summary

### All Containers
- **Unprivileged:** All 25 containers unprivileged
- **User isolation:** uid 10001 (unheaded) isolated per container
- **Capability drops:** sys_time, sys_module, mac_admin, mac_override, sys_rawio

### eBPF Containers
- **Selective grants:** CAP_BPF, CAP_NET_ADMIN, CAP_NET_RAW, CAP_SYS_ADMIN, CAP_PERFMON
- **Not privileged:** `security.privileged: false` (selective caps, not full privilege)
- **BPF filesystem access:** /sys/fs/bpf, /sys/kernel/debug

### GPU Containers
- **Device passthrough:** /dev/kfd, /dev/dri/renderD128 (not privileged)
- **Group memberships:** video, render, kvm (uid 10001)

---

## Getting Help

### Common Tasks

**Launch a single service:**
```bash
bash LAUNCH_EXAMPLES.sh service-name
# e.g.: bash LAUNCH_EXAMPLES.sh api-gateway
```

**Launch all services in a category:**
```bash
bash LAUNCH_EXAMPLES.sh all-api
# Options: all-api, all-ebpf, all-data, all-gpu, all-telemetry
```

**Check container status:**
```bash
lxc list
lxc info <container-name>
lxc exec <container-name> -- systemctl status
```

**Update a profile:**
```bash
# Edit file
vim profiles/unheaded-service.yaml

# Update LXD
lxc profile edit unheaded-service < profiles/unheaded-service.yaml

# Note: Existing containers don't auto-update; new ones get new profile
```

**Verify infrastructure:**
```bash
./deploy.sh verify
# or
./deploy.sh list
```

### Troubleshooting

**Container won't start:**
```bash
lxc info <container-name>           # Check status
lxc logs <container-name>           # View logs
lxc exec <container-name> -- journalctl -xe  # System journal
```

**Network issues:**
```bash
lxc exec <container-name> -- ip addr         # Check IP
lxc exec <container-name> -- ping 8.8.8.8    # Test internet
lxc exec <container-name> -- nslookup google.com  # Test DNS
```

**eBPF verification:**
```bash
lxc exec <ebpf-service> -- ls -la /sys/fs/bpf
lxc exec <ebpf-service> -- bpftool prog list
```

**GPU verification:**
```bash
lxc exec vllm-deepseek -- rocm-smi
lxc exec vllm-deepseek -- rocminfo | grep gfx1101
```

---

## Next Steps

1. **Review documentation:**
   - Start with [README.md](README.md) for overview
   - Check [MANIFEST.md](MANIFEST.md) for 25-service details

2. **Initialize infrastructure:**
   ```bash
   ./deploy.sh init
   ```

3. **Launch services:**
   ```bash
   # Recommended order:
   bash LAUNCH_EXAMPLES.sh all-data        # Databases first
   bash LAUNCH_EXAMPLES.sh all-telemetry   # Observability
   bash LAUNCH_EXAMPLES.sh all-ebpf        # Network/security layer
   bash LAUNCH_EXAMPLES.sh all-api         # API services
   bash LAUNCH_EXAMPLES.sh all-gpu         # GPU (host-a only)
   ```

4. **Verify & monitor:**
   ```bash
   lxc list
   lxc exec prometheus -- curl localhost:9090/-/healthy
   ```

---

## Key Files by Task

| Task | File |
|------|------|
| Understand architecture | README.md |
| Deploy 25 services | MANIFEST.md + LAUNCH_EXAMPLES.sh |
| Quick infrastructure setup | deploy.sh |
| See all files created | CREATED_FILES.md |
| Navigate documentation | INDEX.md (this file) |
| View specific profile | profiles/*.yaml |
| See eBPF packages | cloud-init/ebpf.yaml |
| See GPU packages | cloud-init/gpu.yaml |
| Configure network | networks/unheaded-bridge.yaml |
| Configure storage | storage/unheaded-ssd.yaml |

---

## License & Attribution

**SPDX-License-Identifier:** MIT  
**Copyright:** (c) 2024-2026 Steven Bellis. All rights reserved.

All files in `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/lxd/` include proper SPDX headers and attribution.

---

## Version History

**v1.0** (2026-02-26) — Initial creation
- 5 LXD profiles
- 3 cloud-init templates
- 1 network definition
- 1 storage definition
- 4 documentation files
- 2 helper scripts (deploy.sh, LAUNCH_EXAMPLES.sh)
- Support for 25 microservices across 5 categories
