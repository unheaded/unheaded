# Unheaded Kingdom Docker Deployment

This directory contains complete Docker Compose configurations for deploying the Unheaded Kingdom project across two host architectures.

## Overview

- **Host-A (Forge)**: Full-stack deployment with all 25 services + telemetry
- **Host-B (Outpost)**: Minimal deployment with 6 core services + telemetry agents
- **Inter-host Communication**: WireGuard VPN with IPv6 addressing

## Quick Links

1. **New to this deployment?** Start with [QUICKSTART.md](QUICKSTART.md)
2. **Want system design details?** See [ARCHITECTURE.md](ARCHITECTURE.md)
3. **Deploying Host-A?** Read [host-a/README.md](host-a/README.md)
4. **Deploying Host-B?** Read [host-b/README.md](host-b/README.md)

## Directory Structure

```
hosts/
├── README.md                    # This file
├── QUICKSTART.md                # 5-minute setup guide
├── ARCHITECTURE.md              # System design & overview
├── host-a/                      # Forge (full stack)
│   ├── docker-compose.yml       # 25 services + telemetry (1252 lines)
│   ├── .env.example             # Environment variables template
│   ├── prometheus.yml           # Metrics scraping config
│   ├── loki-config.yaml         # Log aggregation config
│   ├── README.md                # Full deployment guide
│   └── grafana-provisioning/    # Dashboard & datasource configs
└── host-b/                      # Outpost (minimal)
    ├── docker-compose.yml       # 6 services + agents (430 lines)
    ├── .env.example             # Environment variables template
    ├── prometheus.yml           # Metrics agent + remote write
    ├── promtail-config.yaml     # Log forwarding config
    └── README.md                # Minimal deployment guide
```

## What's Included

### Host-A (Forge) - Full Stack

**25 Unheaded Kingdom Services:**
- 1 foundational: wotan (NATS message bus)
- 3 protocol services: monad, sophia, anamnesis
- 2 core daemons: shield, unheaded-daemon
- 6 API/Dashboard: dashboard-backend, dashboard-frontend, protocol-api, trace-collector, gateway, service-discovery
- 2 high-performance: doom, lich
- 5 orchestration: captain, micromanager, timeguru, architect, developer
- 6 data/knowledge: kanban, lore, busboy, kingdom, blackmage, moatghost

**5 Telemetry Services:**
- Prometheus (metrics collection, 30-day retention)
- VictoriaMetrics (long-term storage, 90-day retention)
- Loki (log aggregation)
- Grafana (visualization with dark theme)
- eBPF Exporter (kernel-level metrics)

**Features:**
- 30+ named volumes for persistent state
- Resource limits configured (40 cores, 18GB memory)
- Health checks for all services
- Security hardening (no-new-privileges, read-only, capability restrictions)
- IPv4 + IPv6 networking
- GPU support for RX 7700 XT

### Host-B (Outpost) - Minimal

**6 Core Services:**
- 1 foundational: wotan
- 3 protocol services: monad, sophia, anamnesis
- 2 API services: dashboard-backend, gateway

**4 Telemetry Agents:**
- Prometheus (local scraping, 7-day retention, remote write to Host-A)
- Promtail (log forwarding to Host-A)
- Node Exporter (hardware metrics)
- WireGuard (VPN bridge to Host-A)

**Features:**
- 8 named volumes for service state
- Resource limits configured (10.5 cores, 2.5GB memory)
- Lightweight footprint for consumer-grade hardware
- Encrypted tunnel to Host-A
- Log/metric forwarding to central Host-A

## Hardware Requirements

### Host-A (Forge)
- **CPU**: 16+ cores
- **RAM**: 64GB
- **GPU**: RX 7700 XT (optional)
- **Storage**: SSD recommended (for telemetry data)

### Host-B (Outpost)
- **CPU**: 4-8 cores (consumer grade)
- **RAM**: 8GB
- **GPU**: None
- **Storage**: 50GB+ SSD

## Getting Started

### Prerequisites
- Docker 20.10+ and Docker Compose 2.0+
- WireGuard tools (`wg` and `wg-quick` for key generation)
- 50GB+ free disk space per host
- Network connectivity between hosts (or internet access with port forwarding)

### For Host-A (Forge)

```bash
cd host-a
cp .env.example .env
# Edit .env with WireGuard keys and passwords
docker-compose up -d
# Access Grafana: http://localhost:3001
```

See [host-a/README.md](host-a/README.md) for detailed instructions.

### For Host-B (Outpost)

```bash
cd host-b
cp .env.example .env
# Edit .env with Host-A IP and WireGuard keys
docker-compose up -d
# Verify connection to Host-A
docker-compose exec prometheus ping6 fd00:dead:beef::1
```

See [host-b/README.md](host-b/README.md) for detailed instructions.

## Service Communication

```
Host-B Services
    ↓ (Docker network)
Local NATS (wotan)
    ↓ (WireGuard VPN, fd00:dead:beef::2)
Host-A Services
    ↓ (Docker network)
Central monitoring (prometheus, grafana, loki)
```

### Metrics Flow
- Host-B Prometheus scrapes local services (7-day retention)
- Remote writes to Host-A VictoriaMetrics (90-day retention)
- Host-A Prometheus scrapes all services
- Grafana queries VictoriaMetrics for long-term analysis

### Logs Flow
- Host-B Promtail collects container logs
- Sends to Host-A Loki over encrypted WireGuard tunnel
- Grafana searches/visualizes logs via Loki

## Networking

### Docker Networks
- **Host-A**: 172.20.0.0/16 (IPv4) + fd00:dead:beef:1::/64 (IPv6)
- **Host-B**: 172.21.0.0/16 (IPv4) + fd00:dead:beef:2::/64 (IPv6)

### WireGuard VPN
- **Network**: fd00:dead:beef::/48
- **Host-A Address**: fd00:dead:beef::1
- **Host-B Address**: fd00:dead:beef::2
- **Port**: 51820/UDP
- **Encryption**: WireGuard modern cryptography

## Security

### Default Hardening
- All services: `no-new-privileges:true`
- All services: `read_only:true` (except WireGuard)
- `/tmp` isolated per service (64MB limit)
- Network isolation via Docker networks
- Custom capability restrictions

### Exceptions for Performance
- **shield**: BPF + NET_ADMIN + SYS_ADMIN (eBPF/XDP operations)
- **daemon**: BPF + NET_ADMIN (network operations)
- **lich**: NET_RAW + NET_ADMIN (packet manipulation)
- **blackmage**: NET_RAW (raw sockets)
- **WireGuard**: NET_ADMIN + SYS_MODULE (VPN operations)

## Monitoring & Observability

### Metrics
- **Prometheus**: Real-time collection (5s interval)
- **VictoriaMetrics**: 90-day retention with compression
- **eBPF Exporter**: Kernel-level metrics

### Logs
- **Loki**: Searchable container logs
- **Promtail**: Log collection and forwarding

### Visualizations
- **Grafana**: Unified dashboards (dark theme)
- Pre-provisioned datasources (Prometheus, Loki, VictoriaMetrics)
- Ready for custom dashboard creation

## Resource Allocation

### Host-A Total
- **CPU**: ~40 cores allocated (conservative on 16+ core system)
- **Memory**: ~18GB allocated (conservative on 64GB system)

### Host-B Total
- **CPU**: ~10.5 cores allocated (conservative on 8 core system)
- **Memory**: ~2.5GB allocated (conservative on 8GB system)

Per-service limits match NixOS cgroup v2 settings.

## Common Commands

### View status
```bash
docker-compose ps
docker-compose logs -f
docker stats
```

### Health checks
```bash
docker-compose exec unheaded-monad curl http://localhost:5000/health
```

### Start/stop
```bash
docker-compose up -d      # Start all
docker-compose down       # Stop all
docker-compose restart    # Restart all
```

### Updates
```bash
docker-compose pull       # Pull new images
docker-compose up -d      # Restart with new version
```

### Logs
```bash
docker-compose logs -f                    # Follow all
docker-compose logs unheaded-monad        # Specific service
docker-compose logs --since 10m           # Recent
docker-compose logs --tail 100            # Last 100 lines
```

## Troubleshooting

### Services won't start
1. Check image availability: `docker images | grep unheaded-`
2. Check logs: `docker-compose logs`
3. Verify disk space: `df -h`
4. Check resource limits: `docker stats`

### Health checks failing
1. Review service logs: `docker-compose logs unheaded-{service}`
2. Manual test: `docker-compose exec unheaded-{service} curl http://localhost:{port}/health`
3. Check network: `docker-compose exec unheaded-{service} ping wotan`

### WireGuard not connecting
1. Check keys: `docker-compose exec wireguard wg show`
2. Verify endpoint: `cat .env | grep WG_ENDPOINT`
3. Test connectivity: `docker-compose exec prometheus ping6 fd00:dead:beef::1`
4. Check firewall: Allow 51820/UDP inbound/outbound

### Metrics not appearing
1. Check Prometheus targets: `curl http://localhost:9090/api/v1/targets`
2. Verify service metrics: `curl http://localhost:{port}/metrics`
3. Check remote write config: `cat prometheus.yml | grep remote_write`

For more details, see individual host READMEs.

## File Sizes & Scope

| File | Size | Description |
|------|------|-------------|
| host-a/docker-compose.yml | 1252 lines | 25 services + telemetry |
| host-b/docker-compose.yml | 430 lines | 6 services + agents |
| ARCHITECTURE.md | 397 lines | System design |
| host-a/README.md | 450+ lines | Full deployment guide |
| host-b/README.md | 400+ lines | Minimal deployment guide |
| QUICKSTART.md | 300+ lines | Quick start guide |

## Features Highlights

### Scalability
- Services discoverable via DNS
- Load balancing ready (gateway service)
- Remote write for distributed telemetry
- Horizontal expansion via more Host-B instances

### Reliability
- Health checks on all services
- Graceful shutdown (30s timeout)
- Restart policies (unless-stopped)
- Persistent volumes for state

### Performance
- Resource limits per service
- eBPF-based packet processing
- High-throughput services (doom, lich)
- Efficient metrics storage

### Observability
- Comprehensive logging
- Distributed tracing ready
- Prometheus metrics
- Grafana dashboards

### Security
- Network encryption (WireGuard)
- Capability restrictions
- Read-only filesystems
- Privilege isolation

## Next Steps

1. **Review docs**: Start with QUICKSTART.md or ARCHITECTURE.md
2. **Generate keys**: Create WireGuard keys for both hosts
3. **Configure environment**: Copy .env.example to .env on each host
4. **Deploy**: Run `docker-compose up -d` on Host-A first, then Host-B
5. **Verify**: Check logs and health status
6. **Access**: Open Grafana and explore dashboards
7. **Monitor**: Watch metrics and logs during initial operation
8. **Customize**: Adjust resource limits and monitoring based on usage

## Support

For issues or questions:
1. Check the Troubleshooting sections in individual READMEs
2. Review ARCHITECTURE.md for system design details
3. Check docker-compose.yml comments for service-specific info
4. Review service logs: `docker-compose logs {service}`
5. Verify configuration: `cat .env` and `cat prometheus.yml`

## License

These Docker Compose configurations are part of the Unheaded Kingdom project.

## Version Info

- Created: 2026-02-26
- Docker Compose: v3.8 format
- Docker: 20.10+ recommended
- Compose: 2.0+ recommended

---

**Happy deploying!** Start with [QUICKSTART.md](QUICKSTART.md) for a 5-minute setup.
