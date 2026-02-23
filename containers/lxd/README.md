# LXD Runtime Configuration

System container definitions for running Unheaded services under LXD.

## Overview

LXD provides VM-level isolation using system containers. Each service
runs inside a full Ubuntu 22.04 container with cloud-init provisioning,
static IP addressing on `lxdbr0`, and security hardening.

## Prerequisites

- LXD 5.0+ installed and initialized (`lxd init`)
- `lxdbr0` bridge configured with subnet 10.10.10.0/24
- Ubuntu 22.04 image available (`lxc image copy ubuntu:22.04 local:`)

## Network Setup

Configure the LXD bridge during `lxd init` or manually:

```bash
# Configure lxdbr0 with the Unheaded subnet
lxc network create lxdbr0 \
  ipv4.address=10.10.10.1/24 \
  ipv4.nat=true \
  ipv4.dhcp=false \
  ipv6.address=none \
  dns.domain=unheaded.local
```

## Profile Setup

Import the base and service profiles before launching instances:

```bash
# Create base security profile
lxc profile create unheaded-base
lxc profile edit unheaded-base < containers/lxd/profiles/unheaded-base.yaml

# Create service overlay profile
lxc profile create unheaded-service
lxc profile edit unheaded-service < containers/lxd/profiles/unheaded-service.yaml
```

## Launching Instances

### Single service

```bash
# Launch timeguru
lxc launch ubuntu:22.04 timeguru \
  --profile default \
  --profile unheaded-base \
  --profile unheaded-service \
  --config user.user-data="$(cat containers/lxd/instances/timeguru.yaml)"

# Check status
lxc list
lxc info timeguru
```

### Full stack

```bash
# Launch all services in dependency order
for svc in wotan timeguru captain architect micromanager monad sophia \
           doom-bridge wiki-server kanban dashboard gateway; do
  echo "Launching $svc..."
  lxc launch ubuntu:22.04 "$svc" \
    --profile default \
    --profile unheaded-base \
    --profile unheaded-service \
    --config user.user-data="$(cat containers/lxd/instances/$svc.yaml)"
  sleep 2
done

# Verify all running
lxc list
```

### Teardown

```bash
# Stop and delete all
for svc in gateway dashboard kanban wiki-server doom-bridge sophia monad \
           micromanager architect captain timeguru wotan; do
  lxc stop "$svc" --force 2>/dev/null
  lxc delete "$svc" --force 2>/dev/null
done
```

## Instance Configuration

Each instance YAML in `instances/` contains:
- Static IPv4 address on lxdbr0
- cloud-init user-data for service installation
- Resource limits (memory, CPU)
- Security settings (nesting disabled, unprivileged)
- Disk device for config/data mounts

## Profiles

### unheaded-base.yaml
Base security profile applied to all instances:
- Unprivileged container
- No nesting
- Kernel hardening sysctls
- Default-deny AppArmor profile
- Disk I/O limits

### unheaded-service.yaml
Service overlay profile:
- Shared data mount at /opt/unheaded
- Log directory mount
- Network device on lxdbr0

## File Listing

```
lxd/
├── README.md
├── profiles/
│   ├── unheaded-base.yaml       # Security baseline
│   └── unheaded-service.yaml    # Service-specific overrides
└── instances/
    ├── wotan.yaml               # Message bus
    ├── timeguru.yaml            # Timeline service
    ├── captain.yaml             # Strategy service
    ├── architect.yaml           # Design service
    ├── micromanager.yaml        # Execution service
    ├── monad.yaml               # State management
    ├── sophia.yaml              # Knowledge graph
    ├── dashboard.yaml           # Metrics + WebSocket
    ├── kanban.yaml              # Kanban app (meta moment)
    ├── doom-bridge.yaml         # Fenrir's Eye
    ├── wiki-server.yaml         # Documentation server
    └── gateway.yaml             # nginx reverse proxy
```
