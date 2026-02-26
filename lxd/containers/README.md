# Unheaded Kingdom LXD Container Definitions

Container YAML files for the Unheaded Kingdom microservices architecture. Each file defines a containerized service with CPU/memory limits, network configuration, persistent storage, and autostart behavior.

## Overview

- **Base image**: `ubuntu:24.04`
- **LXD version**: 5.x
- **Network**: "unheaded" bridge (10.20.0.0/16 + fd00:dead:beef:1::/64)
- **Storage pool**: "unheaded-ssd"
- **Container count**: 30 (25 application services + 5 telemetry services)

## Container YAML Files

Each container definition is a standalone YAML file that can be used with:
```bash
# Launch from YAML
lxc init ubuntu:24.04 unheaded-service-name < containers/service-name.yaml

# Or via shell script
./launch.sh service-name
```

### Protocol Services (9 containers)
High-priority coordination and protocol services that form the core of Unheaded Kingdom.

| Service | Port | Priority | CPU | RAM | File |
|---------|------|----------|-----|-----|------|
| Wotan | 50053, 4222 | 100 | 2 | 512MB | `wotan.yaml` |
| Monad | 50051 | 90 | 2 | 512MB | `monad.yaml` |
| Sophia | 50052 | 89 | 2 | 512MB | `sophia.yaml` |
| Anamnesis | 50054 | 88 | 2 | 512MB | `anamnesis.yaml` |
| Protocol API | 50056 | 70 | 2 | 512MB | `protocol-api.yaml` |

### Security & Control (3 containers)
Enforce security policies and manage control plane operations.

| Service | Port | Priority | CPU | RAM | File |
|---------|------|----------|-----|-----|------|
| Shield | 50055 | 85 | 4 | 2GB | `shield.yaml` |
| Unheaded Daemon | 9100 | 84 | 4 | 1GB | `unheaded-daemon.yaml` |
| BlackMage | 50066 | 30 | 2 | 512MB | `blackmage.yaml` |

### Infrastructure (2 containers)
Network routing, service discovery, and API gateway.

| Service | Port | Priority | CPU | RAM | File |
|---------|------|----------|-----|-----|------|
| Service Discovery | 50057, 8500 | 80 | 2 | 512MB | `service-discovery.yaml` |
| Gateway | 8443 | 65 | 2 | 512MB | `gateway.yaml` |

### Observability (2 containers)
Tracing and metrics collection.

| Service | Port | Priority | CPU | RAM | File |
|---------|------|----------|-----|-----|------|
| Trace Collector | 9411 | 75 | 2 | 512MB | `trace-collector.yaml` |
| eBPF Exporter | 9435 | 75 | 2 | 512MB | `ebpf-exporter.yaml` |

### Presentation (3 containers)
User-facing dashboards and interfaces.

| Service | Port | Priority | CPU | RAM | File |
|---------|------|----------|-----|-----|------|
| Dashboard Backend | 8080 | 60 | 2 | 512MB | `dashboard-backend.yaml` |
| Kanban | 3002 | 45 | 2 | 512MB | `kanban.yaml` |
| Dashboard Frontend | 3001 | 55 | 1 | 256MB | `dashboard-frontend.yaml` |

### Compute (1 container)
Heavy computation workload.

| Service | Port | Priority | CPU | RAM | File |
|---------|------|----------|-----|-----|------|
| Doom | 16680 | 50 | 4 | 1GB | `doom.yaml` |

### Royal Court (5 containers)
Administrative and command services.

| Service | Port | Priority | CPU | RAM | File |
|---------|------|----------|-----|-----|------|
| Captain | 50058 | 40 | 1 | 512MB | `captain.yaml` |
| Micromanager | 50059 | 40 | 1 | 512MB | `micromanager.yaml` |
| TimeGuru | 50060, 8600 | 40 | 1 | 512MB | `timeguru.yaml` |
| Architect | 50061 | 40 | 1 | 512MB | `architect.yaml` |
| Developer | 50062 | 40 | 1 | 512MB | `developer.yaml` |

### Coordination & Knowledge (3 containers)
Event routing, knowledge base, and realm coordination.

| Service | Port | Priority | CPU | RAM | File |
|---------|------|----------|-----|-----|------|
| Busboy | 50064 | 35 | 1 | 512MB | `busboy.yaml` |
| Kingdom | 50065 | 35 | 1 | 512MB | `kingdom.yaml` |
| Lore | 50063 | 35 | 1 | 512MB | `lore.yaml` |

### Compliance (1 container)
Audit and compliance operations.

| Service | Port | Priority | CPU | RAM | File |
|---------|------|----------|-----|-----|------|
| MoatGhost | 50067 | 30 | 2 | 512MB | `moatghost.yaml` |

### Telemetry (5 containers)
Metrics, logs, and visualization stack.

| Service | Port | Priority | CPU | RAM | File |
|---------|------|----------|-----|-----|------|
| Prometheus | 9090 | 20 | 4 | 4GB | `prometheus.yaml` |
| VictoriaMetrics | 8428 | 20 | 4 | 8GB | `victoriametrics.yaml` |
| Loki | 3100 | 20 | 2 | 4GB | `loki.yaml` |
| Grafana | 3000 | 15 | 2 | 2GB | `grafana.yaml` |

### Testing (1 container)
Security testing (starts last, internal only).

| Service | Port | Priority | CPU | RAM | File |
|---------|------|----------|-----|-----|------|
| Lich | none | 10 | 4 | 2GB | `lich.yaml` |

## Launch Order and Dependencies

The `boot.autostart.priority` field controls startup order:

```
Priority 100 ← Starts first
    Wotan (message broker - critical dependency)
    ↓
Priority 90-70
    Protocol services (monad, sophia, anamnesis, protocol-api)
    Security & Control (shield, daemon)
    Infrastructure (service-discovery, gateway)
    Observability (trace-collector, ebpf-exporter)
    ↓
Priority 60-35
    Presentation (dashboard, kanban)
    Compute (doom)
    Coordination (busboy, kingdom, lore)
    ↓
Priority 30-10
    Compliance (moatghost, blackmage)
    Telemetry (prometheus, grafana, loki)
    Testing (lich - starts last)
Priority 10 ← Starts last
```

**Rationale:**
1. **Wotan first** (priority 100): All other services depend on this message broker
2. **Protocol services** (90-70): Form the core application logic
3. **Infrastructure** (80-65): Enable service discovery and routing
4. **Presentation/Compute** (60-45): Can use protocol services
5. **Coordination** (35): Use protocol and infrastructure
6. **Compliance/Telemetry** (30-15): Optional, can start later
7. **Lich last** (10): Security testing, doesn't serve external traffic

## Launching a Single Container

To manually launch a container without using `launch.sh`:

```bash
# 1. Initialize from YAML file
lxc init ubuntu:24.04 unheaded-monad < containers/monad.yaml

# 2. Alternatively, assign profile manually
lxc init ubuntu:24.04 unheaded-monad
lxc profile assign unheaded-monad unheaded-base
lxc profile assign unheaded-monad unheaded-ebpf  # if eBPF is needed

# 3. Verify configuration
lxc config show unheaded-monad
lxc device list unheaded-monad

# 4. Start the container
lxc start unheaded-monad

# 5. Check status
lxc list unheaded-monad
lxc console unheaded-monad
```

## Environment Variables

Each container receives environment variables that configure the service. These are set in the `config.environment.*` fields and passed to the service binary at startup.

Common variables:
- `SERVICE_PORT`: Port the service listens on
- `WOTAN_ADDR`: Message broker address (10.20.1.1:50053)
- `LOG_LEVEL`: Logging verbosity (debug, info, warn, error)
- `BACKEND_ADDR`: Address of dependent service
- Service-specific vars (MONAD_PORT, SHIELD_ROLE, etc.)

Example from monad.yaml:
```yaml
config:
  environment.MONAD_PORT: "50051"
  environment.WOTAN_ADDR: "10.20.1.1:50053"
  environment.LOG_LEVEL: "info"
```

These are available inside the container as standard env vars when the service binary starts.

## Bind-Mount Pattern

Each container uses a **three-tier bind-mount strategy** for isolation and persistence:

### 1. Service Binary (Read-Only)
**Host:** `/opt/unheaded/bin/<service-name>`
**Container:** `/opt/unheaded/bin/<service-name>`
**Purpose:** Allow host-based updates without container rebuild
**Protection:** Read-only mount

```yaml
devices:
  bin-monad:
    type: disk
    source: /opt/unheaded/bin/monad
    path: /opt/unheaded/bin/monad
    readonly: "true"
```

### 2. Service State (Read-Write)
**Host:** `/var/lib/unheaded/<service-name>`
**Container:** `/var/lib/unheaded/<service-name>`
**Purpose:** Persistent state across restarts
**Characteristics:** 
- Survives container stop/start
- Survives container rebuild
- Shared with host filesystem

```yaml
devices:
  state:
    type: disk
    source: /var/lib/unheaded/monad
    path: /var/lib/unheaded/monad
    readonly: "false"
```

### 3. Shared Logs (Read-Write)
**Host:** `/var/log/unheaded`
**Container:** `/var/log/unheaded`
**Purpose:** Centralized log collection for all services
**Characteristics:**
- All 30 containers write to same host directory
- Enables centralized log rotation/archival
- Log aggregators (Loki) can scrape from host

```yaml
devices:
  logs:
    type: disk
    source: /var/log/unheaded
    path: /var/log/unheaded
```

### Workflow Example: Update Monad Binary

1. Build new binary on host: `go build -o /opt/unheaded/bin/monad`
2. Container automatically sees update (read-only bind-mount)
3. Restart container: `lxc restart unheaded-monad`
4. New binary is used immediately

## Profiles

Services use shared **profiles** for common configuration:

### unheaded-base (all services)
- Network interface (unheaded bridge)
- DNS resolution
- Log rotation policy
- Common sysctls (security, performance)

### unheaded-ebpf (security services)
- Debugfs mount (required for eBPF)
- Kernel module loading capability
- Services: shield, daemon, lich, ebpf-exporter

### unheaded-telemetry (observability stack)
- Time synchronization settings
- High file descriptor limits
- Services: prometheus, victoriametrics, loki, grafana

## Resource Allocation

Containers are allocated based on workload:

**Small (1 CPU, 512MB RAM):**
- Captain, Micromanager, Architect, Developer, Lore, Busboy, Kingdom

**Medium (2 CPU, 512MB RAM - 1GB RAM):**
- Monad, Sophia, Anamnesis, Protocol API, Gateway, Trace Collector, Dashboard Backend/Frontend, Kanban, BlackMage, MoatGhost

**Large (4 CPU, 1GB RAM - 2GB RAM):**
- Shield, Unheaded Daemon, Doom, Lich

**Extra-Large (4 CPU, 4GB - 8GB RAM):**
- Prometheus (4GB), VictoriaMetrics (8GB), Loki (4GB)

**Medium-Large (2 CPU, 2GB RAM):**
- Grafana

## Port Reference

All 30 services and their exposed ports:

**Protocol Layer (50051-50067):**
- 50051 = Monad
- 50052 = Sophia
- 50053 = Wotan (gRPC)
- 50054 = Anamnesis
- 50055 = Shield
- 50056 = Protocol API
- 50057 = Service Discovery (gRPC)
- 50058 = Captain
- 50059 = Micromanager
- 50060 = TimeGuru (gRPC)
- 50061 = Architect
- 50062 = Developer
- 50063 = Lore
- 50064 = Busboy
- 50065 = Kingdom
- 50066 = BlackMage
- 50067 = MoatGhost

**Additional Ports:**
- 4222 = Wotan (NATS)
- 3000 = Grafana
- 3001 = Dashboard Frontend
- 3002 = Kanban
- 3100 = Loki
- 8080 = Dashboard Backend
- 8428 = VictoriaMetrics
- 8443 = Gateway
- 8500 = Service Discovery (HTTP)
- 8600 = TimeGuru (HTTP)
- 9090 = Prometheus
- 9100 = Unheaded Daemon (metrics)
- 9411 = Trace Collector
- 9435 = eBPF Exporter
- 16680 = Doom

## Profiles Reference

### unheaded-base
Applied to ALL containers. Provides:
- Network bridge attachment
- DNS configuration
- Storage pool defaults
- Log rotation

```bash
lxc profile show unheaded-base
```

### unheaded-ebpf
Applied to security and observability containers:
- Debugfs mount for kernel instrumentation
- BPF capability
- Services: shield, daemon, lich, ebpf-exporter

### unheaded-telemetry
Applied to telemetry containers:
- Time sync settings
- High file descriptor limits
- Services: prometheus, victoriametrics, loki, grafana

## Customization

To modify a container definition:

1. Edit the YAML file
2. Delete and re-create the container:
   ```bash
   lxc delete -f unheaded-service
   lxc init ubuntu:24.04 unheaded-service < containers/service.yaml
   ```

Or modify an existing container:
```bash
lxc config set unheaded-service limits.cpu 4
lxc config set unheaded-service limits.memory 2GB
lxc restart unheaded-service
```

## Disk Usage

Container root filesystems are stored in the "unheaded-ssd" pool. Typical sizes:

- 8GB = Standard service (most containers)
- 10GB = eBPF/security containers (need more kernel/debug space)
- 15GB-30GB = Telemetry containers (need storage for metrics/logs)

Total footprint: ~200GB for all 30 containers + 100GB for persistent state.

## Networking

All containers attach to the "unheaded" bridge:

**IPv4:** 10.20.0.0/16
**IPv6:** fd00:dead:beef:1::/64

Container IPs are assigned sequentially:
- 10.20.1.1 = Gateway container (or host alias for message broker)
- 10.20.1.2+ = Service containers

To find a container's IP:
```bash
lxc list unheaded-monad
```

## Troubleshooting

**Container won't start:**
```bash
lxc start unheaded-service -c user.debug-log=true
lxc console unheaded-service
```

**Check container config:**
```bash
lxc config show unheaded-service
lxc device list unheaded-service
```

**Rebuild container:**
```bash
lxc delete -f unheaded-service
lxc init ubuntu:24.04 unheaded-service < containers/service.yaml
lxc start unheaded-service
```

**Check service logs:**
```bash
lxc file pull unheaded-service/var/log/unheaded/service.log -
```

## References

- LXD documentation: https://documentation.lxd.io/
- LXD networking: https://documentation.lxd.io/lxd/latest/networking/
- Container profiles: https://documentation.lxd.io/lxd/latest/profiles/
