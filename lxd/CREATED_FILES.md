# Unheaded Kingdom LXD IaC — Created Files Summary

**Creation Date:** 2026-02-26  
**LXD Target:** 5.x (Incus-compatible)  
**Base Image:** ubuntu:24.04 / debian:bookworm  
**Network Bridge:** unheaded (10.20.0.0/16 IPv4, fd00:dead:beef:1::/64 IPv6)

---

## Files Created (This Session)

### Profiles (5 files)
All profiles include SPDX MIT license headers and comprehensive comments.

1. **`profiles/unheaded-base.yaml`**
   - Applied to ALL 25 containers
   - Security: unprivileged, isolated idmap, capability drops
   - Defaults: 2 CPU, 512MB RAM, ZFS storage (8GB)
   - Environment: UNHEADED_ENV=production, LOG_FORMAT=json
   - Boot: autostart with 2s delay, priority 10

2. **`profiles/unheaded-service.yaml`**
   - Standard microservice profile (inherits base)
   - Resources: 2 CPU, 768MB RAM, priority 6
   - Observability: OTEL_ENABLED, JSON logging
   - Tier: "standard"

3. **`profiles/unheaded-ebpf.yaml`**
   - eBPF-capable containers (shield, unheaded-daemon, ebpf-exporter, etc.)
   - Capabilities: CAP_BPF, CAP_NET_ADMIN, CAP_NET_RAW, CAP_SYS_ADMIN, CAP_PERFMON
   - Mounts: /sys/fs/bpf, /sys/kernel/debug (optional)
   - Resources: 4 CPU, 2GB RAM, priority 8
   - **Unprivileged** (selective capability grant)

4. **`profiles/unheaded-gpu.yaml`**
   - AMD ROCm GPU passthrough (vLLM, host-a only)
   - Hardware: RX 7700 XT (gfx1101)
   - Passthrough: /dev/kfd, /dev/dri/renderD128
   - Resources: 8 CPU, 16GB RAM, priority 9
   - Environment: HSA_OVERRIDE_GFX_VERSION, PYTORCH_ROCM_ARCH

5. **`profiles/unheaded-telemetry.yaml`**
   - Observability stack (Prometheus, Grafana, Loki, VictoriaMetrics)
   - Resources: 4 CPU, 4GB RAM, 50GB persistent storage
   - Retention: 30 days
   - Tier: "critical"

### Cloud-Init Templates (3 files)
All cloud-init templates include SPDX MIT license headers.

1. **`cloud-init/base.yaml`**
   - Applied to ALL containers
   - User: unheaded (uid 10001, nologin)
   - Group: unheaded (gid 10001)
   - Packages: curl, ca-certificates, tini
   - Directories: /opt/unheaded/bin, /var/lib/unheaded, /var/log/unheaded
   - systemd: NOFILE=65536, NPROC=4096
   - Environment file: /etc/unheaded/env

2. **`cloud-init/ebpf.yaml`**
   - eBPF container initialization (use with base.yaml)
   - Packages: libbpf-dev, bpftool, clang, llvm, libelf-dev
   - Mounts: systemd bpffs.mount service
   - Environment: BPF_PATH, CLANG_INCLUDE_PATH
   - Creates: /var/lib/bpf directory

3. **`cloud-init/gpu.yaml`**
   - GPU/ROCm initialization (use with base.yaml)
   - Packages: rocm-core, rocm-libs, rocm-dev, rocm-smi, rocminfo
   - Group membership: video, render, kvm (for unheaded user)
   - Environment: ROCM_HOME, ROCM_PATH, HIP_VISIBLE_DEVICES
   - Health check: /usr/local/bin/rocm-check.sh

### Network Configuration

1. **`networks/unheaded-bridge.yaml`**
   - Type: LXD managed bridge
   - IPv4: 10.20.0.1/16, DHCP 10.20.1.0-10.20.1.250, NAT enabled
   - IPv6: fd00:dead:beef:1::1/64, stateful DHCPv6, NAT disabled
   - DNS: unheaded.internal (LXD managed)
   - Metadata: purpose, host, IP ranges

### Storage Configuration

1. **`storage/unheaded-ssd.yaml`**
   - Type: ZFS storage pool
   - Pool name: unheaded
   - Volume size: 8GB (default per container)
   - Filesystem: ext4
   - Features: refquota enabled, clone_copy optimized, snapshots retained

### Documentation

1. **`README.md`**
   - Comprehensive guide to all profiles and cloud-init templates
   - Deployment workflow (initialize → create profiles → launch containers)
   - Service categories and profile assignments
   - Networking details (IPv4/IPv6, DNS, WireGuard bridge)
   - Security model overview
   - Logging and observability configuration

2. **`MANIFEST.md`**
   - 25 microservices deployment guide
   - Service categories: API/Web (10), eBPF (5), Data (6), AI/ML (2), Observability (2)
   - Detailed table for each service: CPU, RAM, storage, profiles
   - Deployment checklist (6 phases)
   - Health checks for containers, eBPF, GPU
   - Resource limits summary
   - Production considerations and emergency procedures

3. **`CREATED_FILES.md`** (this file)
   - Summary of all created files and structure

### Deployment Helper

1. **`deploy.sh`**
   - Executable bash script (chmod +x)
   - Commands: check, storage, network, profiles, init, list
   - Full initialization: `./deploy.sh init`
   - Color-coded output (green/yellow/red)
   - Safety checks: LXD daemon verification, idempotent operations

---

## Directory Structure

```
lxd/
├── README.md                          # User guide
├── MANIFEST.md                        # Service deployment manifest
├── CREATED_FILES.md                   # This file
├── deploy.sh                          # Deployment helper script
├── profiles/
│   ├── unheaded-base.yaml            # Base profile (all containers)
│   ├── unheaded-service.yaml         # Standard microservices
│   ├── unheaded-ebpf.yaml            # eBPF/network services
│   ├── unheaded-gpu.yaml             # GPU/LLM services
│   └── unheaded-telemetry.yaml       # Observability stack
├── cloud-init/
│   ├── base.yaml                     # Base initialization
│   ├── ebpf.yaml                     # eBPF-specific setup
│   └── gpu.yaml                      # GPU/ROCm setup
├── networks/
│   └── unheaded-bridge.yaml          # LXD bridge definition
├── storage/
│   └── unheaded-ssd.yaml             # ZFS pool definition
├── containers/                        # (Pre-existing)
│   ├── shield.yaml
│   ├── unheaded-daemon.yaml
│   └── [other service definitions]
└── hosts/                             # (Pre-existing)
    ├── host-a/
    │   ├── preseed.yaml
    │   ├── init.sh
    │   └── launch-all.sh
    └── host-b/
        └── preseed.yaml
```

---

## Quick Start

### 1. Deploy Infrastructure
```bash
cd /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/lxd
./deploy.sh init
```

This will:
- Check LXD is running
- Create ZFS storage pool (unheaded-ssd)
- Create network bridge (unheaded)
- Import all 5 profiles

### 2. Launch a Microservice
```bash
# Standard API service
lxc launch images:ubuntu/24.04/cloud api-gateway \
  --profile unheaded-base \
  --profile unheaded-service \
  --config user.user-data="$(cat cloud-init/base.yaml)"

# eBPF service
lxc launch images:ubuntu/24.04/cloud shield \
  --profile unheaded-base \
  --profile unheaded-ebpf \
  --config user.user-data="$(cat cloud-init/base.yaml && cat cloud-init/ebpf.yaml)"

# GPU service (host-a only)
lxc launch images:ubuntu/24.04/cloud vllm-deepseek \
  --profile unheaded-base \
  --profile unheaded-gpu \
  --config user.user-data="$(cat cloud-init/base.yaml && cat cloud-init/gpu.yaml)"
```

### 3. Verify
```bash
lxc list                                      # Show all containers
lxc info <container-name>                     # Container details
lxc exec <container-name> -- systemctl status # Check services
```

---

## File Sizes & Statistics

```bash
wc -l lxd/*/*.yaml lxd/*.md lxd/deploy.sh

# Approximate line counts:
# profiles/unheaded-base.yaml       ~45 lines
# profiles/unheaded-service.yaml    ~25 lines
# profiles/unheaded-ebpf.yaml       ~40 lines
# profiles/unheaded-gpu.yaml        ~35 lines
# profiles/unheaded-telemetry.yaml  ~30 lines
# cloud-init/base.yaml              ~40 lines
# cloud-init/ebpf.yaml              ~50 lines
# cloud-init/gpu.yaml               ~50 lines
# networks/unheaded-bridge.yaml     ~30 lines
# storage/unheaded-ssd.yaml         ~20 lines
# deploy.sh                         ~150 lines
# README.md                         ~400 lines
# MANIFEST.md                       ~500 lines
# TOTAL: ~1,815 lines
```

---

## Key Design Decisions

### Security Model
- **All containers unprivileged** with isolated user namespace (uid 10001)
- eBPF containers: selective CAP_BPF/CAP_NET_ADMIN (not full privilege)
- Capability drops: sys_time, sys_module, mac_admin, mac_override, sys_rawio
- AppArmor confinement (unconfined only for eBPF when necessary)

### Resource Allocation
- **Service Priority Hierarchy:**
  - 9: GPU services (vLLM, embeddings)
  - 8: eBPF services (shield, daemon)
  - 7: Telemetry (prometheus, loki)
  - 6: Standard services (api-gateway, etc.)
  - 5: Default/fallback

### Network Topology
- IPv4: DHCP from 10.20.1.0-10.20.1.250 (gateway 10.20.0.1)
- IPv6: Stateful DHCPv6 in fd00:dead:beef:1::/64
- Internal DNS: unheaded.internal (LXD managed)
- Host-B bridges via WireGuard to host-A

### Storage Strategy
- ZFS with refquota (per-container quotas)
- Clone_copy optimized (fast container creation)
- Snapshots retained (manual snapshots, not auto-purged)
- Default size: 8GB per container (can override per-container)

---

## SPDX Licensing

All YAML files include:
```
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
```

---

## Notes for Integration

### With Existing Files
- `containers/*.yaml` — Service-specific overrides (pre-existing, not modified)
- `hosts/host-a/` — Host initialization scripts (pre-existing)
- `hosts/host-b/` — WireGuard bridging setup (pre-existing)

This session created **reusable, generic profiles and templates** that the service-specific files can reference.

### Next Steps
1. Review MANIFEST.md for 25-service deployment strategy
2. Run `./deploy.sh init` to set up infrastructure
3. Customize individual service cloud-init (combine base.yaml + service-specific configs)
4. Deploy using launch scripts in hosts/host-a/ and hosts/host-b/

---

## License

SPDX-License-Identifier: MIT  
Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
