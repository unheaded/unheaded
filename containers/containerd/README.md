# containerd Runtime Configuration

OCI runtime specs for running Unheaded services under containerd.

## Overview

Each service has a JSON spec file in `services/` that defines:
- Image reference (ghcr.io/unheaded/SERVICE:latest)
- Resource limits (memory, CPU)
- Security settings (no-new-privileges, read-only rootfs, capabilities)
- Network configuration (static IP on unheaded bridge)
- Environment variables
- Health check commands

## Prerequisites

- containerd 1.7+ installed
- CNI plugins for bridge networking
- `ctr` or `nerdctl` CLI

## Network Setup

Configure a CNI bridge network before launching containers:

```bash
# Create CNI config for unheaded bridge
cat > /etc/cni/net.d/10-unheaded.conflist << 'EOF'
{
  "cniVersion": "1.0.0",
  "name": "unheaded",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "br-unheaded",
      "isGateway": true,
      "ipMasq": true,
      "ipam": {
        "type": "static"
      }
    },
    {
      "type": "firewall"
    }
  ]
}
EOF
```

## Usage

### With ctr (low-level)

```bash
# Pull image
ctr images pull ghcr.io/unheaded/timeguru:latest

# Run with OCI spec
ctr run \
  --detach \
  --cni \
  ghcr.io/unheaded/timeguru:latest \
  timeguru

# Check status
ctr task ls
```

### With nerdctl (Docker-compatible)

```bash
# Create network
nerdctl network create unheaded --subnet 10.10.10.0/24

# Run service
nerdctl run -d \
  --name timeguru \
  --network unheaded \
  --ip 10.10.10.20 \
  --memory 256m \
  --cpus 1.0 \
  --read-only \
  --security-opt no-new-privileges \
  --health-cmd "wget -qO- http://localhost:8000/health || exit 1" \
  --health-interval 30s \
  -e WOTAN_ADDR=10.10.10.10:9090 \
  -e SERVICE_NAME=timeguru \
  -e LOG_LEVEL=info \
  ghcr.io/unheaded/timeguru:latest
```

### With Kubernetes (CRI)

The specs are compatible with Kubernetes RuntimeClass when using
containerd as the CRI. Map the resource limits and security context
from the JSON specs to your pod manifests.

## Daemon Configuration

The `config.toml` file provides a containerd daemon configuration
tuned for the Unheaded workload:

- OCI runtime (runc) with seccomp defaults
- Snapshot driver: overlayfs
- Metrics endpoint on port 9200
- CNI networking enabled

Install to `/etc/containerd/config.toml` and restart containerd.

## File Listing

```
containerd/
├── README.md              # This file
├── config.toml            # containerd daemon config
└── services/
    ├── wotan.json         # Message bus
    ├── timeguru.json      # Timeline service
    ├── captain.json       # Strategy service
    ├── architect.json     # Design service
    ├── micromanager.json  # Execution service
    ├── monad.json         # State management
    ├── sophia.json        # Knowledge graph
    ├── dashboard.json     # Metrics + WebSocket
    ├── kanban.json        # Kanban app (meta moment)
    ├── doom-bridge.json   # Fenrir's Eye
    ├── wiki-server.json   # Documentation server
    └── gateway.json       # nginx reverse proxy
```
