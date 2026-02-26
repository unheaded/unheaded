# Unheaded Kingdom LXD Host Architecture

## System Overview

The Unheaded Kingdom is deployed across two LXD hosts with distinct roles:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Unheaded Kingdom Deployment                  │
└─────────────────────────────────────────────────────────────────┘
              │                                      │
              ▼                                      ▼
    ┌──────────────────┐              ┌──────────────────┐
    │   host-a         │              │   host-b         │
    │   (The Forge)    │◄─ WireGuard ─┤  (The Outpost)   │
    │                  │              │                  │
    │ 16+ cores        │              │ Consumer-grade   │
    │ 64GB RAM         │              │ 8GB RAM          │
    │ RX 7700 XT       │              │ No GPU           │
    │ 25 services      │              │ 6 core + telemetry
    └──────────────────┘              └──────────────────┘
            │                                  │
            │ 10.20.0.0/16                    │ 10.21.0.0/16
            │ 200GB ZFS                       │ 50GB ZFS
            │                                 │
            └─────────────┬────────────────────┘
                          │
                      ┌───────────┐
                      │ Cluster   │
                      │ Overlay   │
                      │ Network   │
                      └───────────┘
```

## host-a: The Forge (Full Stack)

### Purpose
Complete production deployment hosting all 25 services, compute workloads, and observability infrastructure.

### Hardware Requirements
- CPU: 16+ cores (deployment uses 51 CPU allocations)
- RAM: 64GB (deployment uses 23.5GB across containers)
- GPU: RX 7700 XT (available for compute services like doom, vllm)
- Storage: 200GB+ ZFS pool

### Network Configuration
```
Interface: unheaded (LXD bridge)
IPv4: 10.20.0.0/16
      Gateway: 10.20.0.1
      DHCP Pool: 10.20.1.1-10.20.1.254
IPv6: fd00:dead:beef:1::/64
DNS Domain: unheaded.internal
DNS Mode: Managed by LXD
```

### Service Breakdown (25 services)

#### Phase 1: Core Infrastructure (1)
- **wotan** (10.20.1.1): Message bus - NATS/gRPC server
  - CPU: 4, Memory: 1GB
  - Ports: 50053 (gRPC), 4222 (NATS)

#### Phase 2: Protocol Layer (3)
- **monad** (10.20.1.2): Protocol logic
  - CPU: 2, Memory: 512MB, Port: 50051
- **sophia** (10.20.1.3): Protocol state management
  - CPU: 2, Memory: 512MB, Port: 50052
- **anamnesis** (10.20.1.4): Protocol history
  - CPU: 2, Memory: 512MB, Port: 50054

#### Phase 3: eBPF Control (2)
- **shield** (10.20.1.5): eBPF firewall/security
  - CPU: 4, Memory: 2GB, Port: 50055
  - Profile: unheaded-ebpf (kernel access)
- **unheaded-daemon** (10.20.1.6): System control daemon
  - CPU: 4, Memory: 1GB, Port: 9100
  - Profile: unheaded-ebpf (kernel access)

#### Phase 4: Routing (2)
- **gateway** (10.20.1.11): API gateway/load balancer
  - CPU: 2, Memory: 512MB, Port: 8443
- **service-discovery** (10.20.1.12): Service registry
  - CPU: 1, Memory: 256MB, Ports: 50057 (gRPC), 8500 (HTTP)

#### Phase 5: Presentation (3)
- **dashboard-backend** (10.20.1.7): Dashboard API
  - CPU: 2, Memory: 512MB, Port: 8080
- **dashboard-frontend** (10.20.1.8): Web UI
  - CPU: 1, Memory: 256MB, Port: 3001
- **protocol-api** (10.20.1.9): Protocol API
  - CPU: 2, Memory: 512MB, Port: 50056

#### Phase 6: Observability & Compute (2)
- **trace-collector** (10.20.1.10): Distributed tracing (Jaeger)
  - CPU: 2, Memory: 512MB, Port: 9411
- **doom** (10.20.1.13): GPU compute service
  - CPU: 4, Memory: 1GB, Port: 16680
  - Profile: unheaded-gpu (GPU access)

#### Phase 7-8: Business Services (11)
- **kanban** (10.20.1.20): Task management
  - CPU: 2, Memory: 512MB, Port: 3002
- **captain** (10.20.1.15): Orchestration
  - CPU: 1, Memory: 256MB, Port: 50058
- **micromanager** (10.20.1.16): Resource management
  - CPU: 1, Memory: 256MB, Port: 50059
- **timeguru** (10.20.1.17): Time/scheduling service
  - CPU: 1, Memory: 256MB, Ports: 50060 (gRPC), 8600 (HTTP)
- **architect** (10.20.1.18): Planning service
  - CPU: 1, Memory: 256MB, Port: 50061
- **developer** (10.20.1.19): Development tools
  - CPU: 1, Memory: 256MB, Port: 50062
- **lore** (10.20.1.21): Knowledge base
  - CPU: 1, Memory: 256MB, Port: 50063
- **busboy** (10.20.1.22): Transport service
  - CPU: 1, Memory: 256MB, Port: 50064
- **kingdom** (10.20.1.23): Kingdom state
  - CPU: 1, Memory: 256MB, Port: 50065
- **blackmage** (10.20.1.24): Magic system
  - CPU: 1, Memory: 256MB, Port: 50066
- **moatghost** (10.20.1.25): Perimeter security
  - CPU: 1, Memory: 256MB, Port: 50067

#### Phase 9: Adversary (1)
- **lich** (10.20.1.14): Adversarial testing
  - CPU: 4, Memory: 2GB, Port: 9999
  - Runs after all services are ready

#### Phase 10: Telemetry Stack (5)
- **prometheus** (10.20.1.50): Metrics collection
  - CPU: 2, Memory: 512MB, Port: 9090
- **victoriametrics** (10.20.1.51): Time-series DB
  - CPU: 2, Memory: 512MB, Port: 8428
- **loki** (10.20.1.52): Log aggregation
  - CPU: 2, Memory: 512MB, Port: 3100
- **grafana** (10.20.1.53): Visualization
  - CPU: 1, Memory: 256MB, Port: 3000
- **ebpf-exporter** (10.20.1.26): eBPF metrics
  - CPU: 1, Memory: 256MB, Port: 9435

### Storage Configuration
```
Pool Name: unheaded-ssd
Driver: ZFS
Size: 200GB
Container Root Allocations: 8GB per container
Total Containers: 30 (25 services + 5 telemetry)
```

### Initialization Flow
```
1. preseed.yaml
   └─ LXD auto-init with networks, storage, profiles
   
2. init.sh
   ├─ Installs LXD 5.21 (snap)
   ├─ Adds user to lxd group
   ├─ Applies preseed.yaml
   ├─ Creates /opt/unheaded/bin/
   ├─ Copies service binaries
   └─ Loads LXD profiles

3. launch-all.sh
   ├─ Phase 1-10 container launches
   ├─ Respects service dependencies
   ├─ Applies per-service resource limits
   ├─ Pushes binaries to containers
   └─ Starts systemd services

4. Monitoring (via Prometheus + Grafana)
   └─ Collects metrics from all services
```

## host-b: The Outpost (Minimal)

### Purpose
Minimal distributed deployment for edge locations, development environments, or testing. Maintains cluster connectivity via WireGuard to host-a.

### Hardware Requirements
- CPU: 4+ cores (deployment uses 10 CPU allocations)
- RAM: 8GB (deployment uses 2.5GB across containers)
- GPU: None required
- Storage: 50GB+ ZFS pool

### Network Configuration
```
Interface: unheaded-outpost (LXD bridge)
IPv4: 10.21.0.0/16
      Gateway: 10.21.0.1
      DHCP Pool: 10.21.1.1-10.21.1.100
IPv6: fd00:dead:beef:2::/64
DNS Domain: unheaded.internal

WireGuard Tunnel: wg0
  Purpose: Cluster connectivity to host-a
  Configuration: wireguard.conf (user-provided)
```

### Service Breakdown (6 core + telemetry)

#### Core Services (6)
- **wotan** (10.21.1.1): Local message bus
  - CPU: 2, Memory: 512MB
- **monad** (10.21.1.2): Protocol service
  - CPU: 1, Memory: 256MB
- **sophia** (10.21.1.3): State management
  - CPU: 1, Memory: 256MB
- **anamnesis** (10.21.1.4): History service
  - CPU: 1, Memory: 256MB
- **gateway** (10.21.1.5): API gateway
  - CPU: 1, Memory: 256MB
- **dashboard-backend** (10.21.1.6): Dashboard API
  - CPU: 1, Memory: 256MB

#### Telemetry Agents (3)
- **prometheus-agent**: Prometheus in agent mode
  - CPU: 1, Memory: 256MB
  - Scrapes local services, remote_write to host-a
- **promtail**: Log shipper
  - CPU: 1, Memory: 256MB
  - Ships logs to host-a Loki
- **node-exporter**: Host metrics exporter
  - CPU: 1, Memory: 256MB
  - Exports system metrics for host-a Prometheus

### Storage Configuration
```
Pool Name: unheaded-minimal
Driver: ZFS
Size: 50GB
Container Root Allocations: 4GB per container
Total Containers: 9 (6 core + 3 telemetry)
```

### Initialization Flow
```
1. preseed.yaml
   └─ LXD auto-init with networks, storage, profiles
   
2. init.sh
   ├─ Installs LXD 5.21 (snap)
   ├─ Applies preseed.yaml
   ├─ Creates /opt/unheaded/bin/
   ├─ Configures WireGuard tunnel to host-a
   └─ Loads LXD profiles

3. launch-minimal.sh
   ├─ Phase 1-4 container launches
   ├─ Simpler service dependency chain
   ├─ Applies per-service resource limits
   ├─ Configures remote telemetry
   └─ Starts systemd services

4. Remote Monitoring
   ├─ Prometheus Agent: remote_write to host-a:9090
   ├─ Promtail: remote write to host-a Loki:3100
   └─ Node Exporter: scraped by host-a Prometheus
```

## Service Dependencies

### host-a Full Chain
```
wotan (message bus)
  ├─ monad, sophia, anamnesis (protocol layer)
  │   └─ shield, unheaded-daemon (eBPF control)
  │       └─ gateway, service-discovery (routing)
  │           ├─ dashboard-backend, dashboard-frontend, protocol-api (presentation)
  │           │   └─ kanban, captain, micromanager, timeguru, architect, developer, lore, busboy, kingdom, blackmage, moatghost (services)
  │           │       └─ lich (adversary - runs after all others)
  │           └─ trace-collector, doom (observability)
  │
  └─ prometheus, victoriametrics, loki, grafana, ebpf-exporter (telemetry)
```

### host-b Minimal Chain
```
wotan (local message bus)
  └─ monad, sophia, anamnesis (protocol layer)
      └─ gateway, dashboard-backend (presentation)
          └─ prometheus-agent, promtail, node-exporter (telemetry agents)
```

## Inter-Host Communication

### WireGuard Tunnel (host-b → host-a)
```
host-b (10.21.0.0/16)
    │
    ├─ Local Network: unheaded-outpost bridge
    │   └─ Containers: 10.21.1.x
    │
    ├─ WireGuard Interface: wg0
    │   └─ Tunnel Endpoint: host-a via VPN
    │
    └─ Remote Cluster: host-a (10.20.0.0/16)
        └─ Services: 10.20.1.x
```

### Service-to-Service Communication
- Within host: Direct via internal bridge (unheaded / unheaded-outpost)
- Across hosts: Via WireGuard tunnel (host-b containers → host-a containers)
- Telemetry: host-b agents remote_write to host-a collectors

## Performance Metrics

### host-a (Forge) Total Resource Usage
```
Cores: 51 / 16+ available (overcommit 3.19x typical)
Memory: 23.5GB / 64GB available (36.7% utilization)
Storage: 240GB allocated (8GB × 30 containers) / 200GB pool
Network: 30 containers on unheaded bridge (10.20.1.1-10.20.1.53)
```

### host-b (Outpost) Total Resource Usage
```
Cores: 10 / 4+ available (overcommit 2.5x typical)
Memory: 2.5GB / 8GB available (31.3% utilization)
Storage: 36GB allocated (4GB × 9 containers) / 50GB pool
Network: 9 containers on unheaded-outpost bridge (10.21.1.1-10.21.1.9)
```

## Scaling Considerations

### Horizontal Scaling
To add more Outpost nodes:
1. Duplicate host-b configuration
2. Adjust network subnet (10.22.x.x, 10.23.x.x, etc.)
3. Configure WireGuard mesh networking
4. Set up Prometheus federation

### Vertical Scaling
To upgrade host-a:
1. Increase container CPU/memory allocations
2. Expand ZFS pool size
3. Scale resource limits in preseed.yaml
4. Adjust Grafana dashboards for new baselines

### High Availability
To implement HA:
1. Run two host-a instances with shared storage (HA ZFS)
2. Use floating VIP for LXD API
3. Configure Prometheus federation
4. Implement etcd for distributed state

## SPDX License

All configuration files are licensed under MIT License:
Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

