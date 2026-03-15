# Unheaded Kingdom LXD Container Definitions - Complete Index

**Location:** `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/lxd/containers/`

**Generated:** 2026-02-26

**Statistics:**
- 30 service container definitions (YAML files)
- 1 template documentation (default.yaml)
- 1 comprehensive README
- 1,651 total lines of YAML
- 3 tier profiles (base, eBPF, telemetry)

## Quick Reference Tables

### All 30 Services by Startup Priority

| Priority | Service | Port | File | Tier |
|----------|---------|------|------|------|
| 100 | Wotan | 50053, 4222 | wotan.yaml | protocol |
| 90 | Monad | 50051 | monad.yaml | protocol |
| 89 | Sophia | 50052 | sophia.yaml | protocol |
| 88 | Anamnesis | 50054 | anamnesis.yaml | protocol |
| 85 | Shield | 50055 | shield.yaml | security |
| 84 | Unheaded Daemon | 9100 | unheaded-daemon.yaml | control |
| 80 | Service Discovery | 50057, 8500 | service-discovery.yaml | infrastructure |
| 75 | Trace Collector | 9411 | trace-collector.yaml | observability |
| 75 | eBPF Exporter | 9435 | ebpf-exporter.yaml | observability |
| 70 | Protocol API | 50056 | protocol-api.yaml | protocol |
| 65 | Gateway | 8443 | gateway.yaml | infrastructure |
| 60 | Dashboard Backend | 8080 | dashboard-backend.yaml | presentation |
| 55 | Dashboard Frontend | 3001 | dashboard-frontend.yaml | presentation |
| 50 | Doom | 16680 | doom.yaml | compute |
| 45 | Kanban | 3002 | kanban.yaml | presentation |
| 40 | Captain | 50058 | captain.yaml | royal-court |
| 40 | Micromanager | 50059 | micromanager.yaml | royal-court |
| 40 | TimeGuru | 50060, 8600 | timeguru.yaml | royal-court |
| 40 | Architect | 50061 | architect.yaml | royal-court |
| 40 | Developer | 50062 | developer.yaml | royal-court |
| 35 | Busboy | 50064 | busboy.yaml | coordination |
| 35 | Kingdom | 50065 | kingdom.yaml | coordination |
| 35 | Lore | 50063 | lore.yaml | knowledge |
| 30 | BlackMage | 50066 | blackmage.yaml | security |
| 30 | MoatGhost | 50067 | moatghost.yaml | compliance |
| 20 | Prometheus | 9090 | prometheus.yaml | telemetry |
| 20 | VictoriaMetrics | 8428 | victoriametrics.yaml | telemetry |
| 20 | Loki | 3100 | loki.yaml | telemetry |
| 15 | Grafana | 3000 | grafana.yaml | telemetry |
| 10 | Lich | none | lich.yaml | security-testing |

### Services by Resource Allocation

**Small (1 vCPU, 512MB):**
- captain.yaml, micromanager.yaml, architect.yaml, developer.yaml, lore.yaml, busboy.yaml, kingdom.yaml

**Medium (2 vCPU, 512MB-1GB):**
- monad.yaml, sophia.yaml, anamnesis.yaml, protocol-api.yaml, gateway.yaml, trace-collector.yaml, dashboard-backend.yaml, dashboard-frontend.yaml, kanban.yaml, blackmage.yaml, moatghost.yaml, wotan.yaml, service-discovery.yaml

**Large (4 vCPU, 1-2GB):**
- shield.yaml, unheaded-daemon.yaml, doom.yaml, lich.yaml, ebpf-exporter.yaml

**Extra-Large (2-4 vCPU, 2-8GB):**
- prometheus.yaml (4 vCPU, 4GB)
- victoriametrics.yaml (4 vCPU, 8GB)
- loki.yaml (2 vCPU, 4GB)
- grafana.yaml (2 vCPU, 2GB)

### Services by Profile Requirements

**unheaded-base only (22 services):**
monad, sophia, anamnesis, protocol-api, gateway, dashboard-backend, dashboard-frontend, kanban, captain, micromanager, timeguru, architect, developer, lore, busboy, kingdom, blackmage, moatghost, trace-collector, wotan, service-discovery

**unheaded-base + unheaded-ebpf (4 services):**
shield, unheaded-daemon, lich, ebpf-exporter

**unheaded-base + unheaded-telemetry (4 services):**
prometheus, victoriametrics, loki, grafana

## File Descriptions

### Application Services (25 files)

**Core Protocol Layer:**
- **wotan.yaml** - Message broker (gRPC + NATS), priority 100 (starts FIRST)
- **monad.yaml** - Register service for IPv6 HbH headers
- **sophia.yaml** - Consensus mechanism
- **anamnesis.yaml** - Memory and state service
- **protocol-api.yaml** - Protocol layer API

**Security & Control:**
- **shield.yaml** - eBPF-based security enforcement (4 vCPU, 2GB RAM)
- **unheaded-daemon.yaml** - Metrics and control daemon (4 vCPU, 1GB RAM)
- **blackmage.yaml** - Secret management and key derivation

**Infrastructure:**
- **service-discovery.yaml** - Service registry and health checking (gRPC + HTTP)
- **gateway.yaml** - API gateway with TLS termination

**Observability:**
- **trace-collector.yaml** - Zipkin-based distributed tracing
- **ebpf-exporter.yaml** - Kernel metrics exporter (eBPF-based)

**Presentation:**
- **dashboard-backend.yaml** - Dashboard API server
- **dashboard-frontend.yaml** - Web UI (1 vCPU, 256MB)
- **kanban.yaml** - Project management interface

**Compute:**
- **doom.yaml** - Heavy computation engine (4 vCPU, 1GB)

**Royal Court (Administrative):**
- **captain.yaml** - Royal court coordination service
- **micromanager.yaml** - Task management service
- **timeguru.yaml** - Temporal and scheduling service (gRPC + HTTP)
- **architect.yaml** - System design and configuration service
- **developer.yaml** - Development environment service

**Coordination & Knowledge:**
- **busboy.yaml** - Event coordination service
- **kingdom.yaml** - Realm coordination service
- **lore.yaml** - Knowledge management and documentation

**Compliance:**
- **moatghost.yaml** - Compliance auditing service

**Testing:**
- **lich.yaml** - Security testing (no external port, priority 10, starts LAST)

### Telemetry Services (5 files)

**Metrics:**
- **prometheus.yaml** - Metrics collection and storage (4 vCPU, 4GB)
- **victoriametrics.yaml** - Time-series database (4 vCPU, 8GB)

**Logs:**
- **loki.yaml** - Log aggregation (2 vCPU, 4GB)

**Visualization:**
- **grafana.yaml** - Dashboarding and visualization (2 vCPU, 2GB)

### Documentation (2 files)

- **default.yaml** - Template with all fields documented and explained
- **README.md** - Comprehensive reference (13 KB)

## Key Configuration Details

### Network Configuration
- Bridge: "unheaded"
- IPv4: 10.20.0.0/16
- IPv6: fd00:dead:beef:1::/64
- Assigned automatically via profile

### Storage Configuration
- Pool: "unheaded-ssd"
- Root sizes: 8-30GB depending on service
- Persistent state at: /var/lib/unheaded/<service>/
- Shared logs at: /var/log/unheaded/

### Service Launch Method
Each container has three bind-mounts:
1. **Binary** - Read-only from /opt/unheaded/bin/<service>
2. **State** - Read-write from /var/lib/unheaded/<service>
3. **Logs** - Read-write to /var/log/unheaded

This allows:
- Binary updates without container rebuild
- Persistent state across restarts
- Centralized log management

### Environment Variables
Each service receives configuration via environment variables:
- `<SERVICE>_PORT` - Port to listen on
- `WOTAN_ADDR` - Message broker address (10.20.1.1:50053)
- `LOG_LEVEL` - Logging verbosity (debug, info, warn, error)
- Service-specific variables (e.g., SHIELD_ROLE, LICH_INTERNAL_MODE)

### Startup Priority Order

**Priority 100** (Wotan - starts first)
- All other services depend on message broker

**Priority 90-70** (Protocol & Infrastructure)
- Core application logic
- Service discovery and routing
- Enable other services to function

**Priority 60-45** (Presentation & Compute)
- Can use protocol services
- Heavy workloads

**Priority 35-30** (Coordination & Compliance)
- Use protocol and infrastructure
- Optional observability

**Priority 20-15** (Telemetry)
- Can start later
- Optional for operation

**Priority 10** (Lich - starts last)
- Security testing
- Internal use only
- No external dependencies

## Usage Examples

### Launch a Single Service
```bash
lxc init ubuntu:24.04 unheaded-monad < containers/monad.yaml
lxc start unheaded-monad
```

### Launch All Services in Order
```bash
./launch.sh                    # launches all containers with proper priority
```

### Check Service Status
```bash
lxc list | grep unheaded
lxc info unheaded-monad
```

### Access Service
```bash
lxc list unheaded-monad       # shows container IP
# Access at: <container_ip>:<port>
```

### View Service Logs
```bash
lxc file pull unheaded-monad/var/log/unheaded/monad.log -
```

### Update Service Binary
```bash
go build -o /opt/unheaded/bin/monad
lxc restart unheaded-monad    # container auto-loads new binary
```

## Design Rationale

### Why Priority-Based Startup?
- Wotan (message broker) must start first - all services depend on it
- Protocol services form the application core
- Infrastructure enables service discovery and communication
- Presentation and compute can start after protocol services
- Telemetry is optional and can start last
- Lich (testing) starts last to avoid interfering with operation

### Why Bind-Mount Pattern?
- **Binary** (read-only): Enables binary updates without container rebuild
- **State** (read-write): Survives container restart/rebuild
- **Logs** (shared): Centralized log collection for all services

### Why Profile-Based Configuration?
- **unheaded-base**: Common configuration (network, storage, DNS)
- **unheaded-ebpf**: Kernel debugging for security services
- **unheaded-telemetry**: High resource limits and time sync for metrics

This allows consistent configuration across 30 containers while customizing as needed.

### Resource Allocation Strategy
- Small services (1 vCPU, 512MB): Low-traffic administrative services
- Medium services (2 vCPU, 512MB-1GB): Core application logic
- Large services (4 vCPU, 1-2GB): Security/compute-intensive
- Extra-large services (2-4 vCPU, 2-8GB): Telemetry (metric/log storage)

## Total Resource Footprint

**CPU:**
- 5 × 4 vCPU = 20 vCPU (shield, daemon, doom, lich, ebpf-exporter)
- 11 × 2 vCPU = 22 vCPU (monad, sophia, etc.)
- 12 × 1 vCPU = 12 vCPU (administrative services)
- **Total: ~54 vCPU (typical allocation)**

**Memory:**
- Telemetry: 4 + 8 + 4 + 2 = 18 GB
- Large services: 2 + 1 + 1 + 2 = 6 GB
- Medium services: 11 × 1 = 11 GB
- Small services: 7 × 0.5 = 3.5 GB
- **Total: ~38.5 GB (typical allocation)**

**Storage:**
- Root filesystems: ~250 GB
- Persistent state: ~100 GB
- Logs (daily rotation): ~50 GB
- **Total: ~400 GB (typical footprint)**

## References

- **YAML Specification**: https://yaml.org/spec/
- **LXD Documentation**: https://documentation.lxd.io/
- **LXD Profiles**: https://documentation.lxd.io/lxd/latest/profiles/
- **LXD Networking**: https://documentation.lxd.io/lxd/latest/networking/
- **Container Devices**: https://documentation.lxd.io/lxd/latest/config-devices/

---

**Version:** 1.0
**Created:** 2026-02-26
**License:** MIT (Copyright 2024-2026 Stevie Bellis)
