# Unheaded Kingdom Docker Architecture

## Overview

The Unheaded Kingdom is deployed across two hosts with complementary architectures:
- **Host-A (Forge)**: Full-stack deployment, 16+ cores, 64GB RAM, RX 7700 XT GPU
- **Host-B (Outpost)**: Minimal deployment, consumer grade, 8GB RAM, no GPU

## Host-A: Forge (Full Stack)

Complete deployment of all 25 services plus comprehensive telemetry.

### Services (25 total)

#### Foundation (1)
1. **wotan** - NATS message bus (4.0 cpus, 1G mem)

#### Core Protocol (3)
2. **monad** - Protocol registry (2.0 cpus, 512M mem)
3. **sophia** - BPF dictionaries (2.0 cpus, 512M mem)
4. **anamnesis** - Event log (2.0 cpus, 512M mem)

#### Daemons (2)
5. **shield** - Policy enforcement eBPF (4.0 cpus, 2G mem)
6. **unheaded-daemon** - Main daemon (4.0 cpus, 1G mem)

#### API & Dashboard (6)
7. **dashboard-backend** - Metrics API (2.0 cpus, 512M mem)
8. **dashboard-frontend** - Web UI (1.0 cpus, 256M mem)
9. **protocol-api** - Protocol interface (2.0 cpus, 512M mem)
10. **trace-collector** - Distributed tracing (2.0 cpus, 512M mem)
11. **gateway** - API ingress (2.0 cpus, 512M mem)
12. **service-discovery** - Service registry (1.0 cpus, 256M mem)

#### High-Performance (2)
13. **doom** - Packet processor (4.0 cpus, 1G mem)
14. **lich** - Packet manipulation (4.0 cpus, 2G mem)

#### Orchestration (5)
15. **captain** - Orchestration (1.0 cpus, 256M mem)
16. **micromanager** - Resource mgmt (1.0 cpus, 256M mem)
17. **timeguru** - Scheduling (1.0 cpus, 256M mem)
18. **architect** - Infrastructure (1.0 cpus, 256M mem)
19. **developer** - Dev environment (1.0 cpus, 256M mem)

#### Data & Knowledge (6)
20. **kanban** - Project management (2.0 cpus, 512M mem)
21. **lore** - Knowledge base (1.0 cpus, 256M mem)
22. **busboy** - Service mesh sidecar (1.0 cpus, 256M mem)
23. **kingdom** - State management (1.0 cpus, 256M mem)
24. **blackmage** - Custom logic executor (2.0 cpus, 512M mem)
25. **moatghost** - Perimeter security (2.0 cpus, 512M mem)

#### Telemetry (5)
- **prometheus** - Metrics collection (2.0 cpus, 512M mem)
- **victoriametrics** - Long-term storage (2.0 cpus, 1G mem)
- **loki** - Log aggregation (2.0 cpus, 512M mem)
- **grafana** - Visualization (1.0 cpus, 512M mem)
- **ebpf-exporter** - eBPF metrics (1.0 cpus, 256M mem)

### Resource Summary (Host-A)
- **Total CPU Allocation**: ~40 cores (conservative on 16-core system)
- **Total Memory Allocation**: ~18GB (out of 64GB available)
- **GPU**: RX 7700 XT (optional, for vLLM services)

### Networking (Host-A)
- **Docker Network**: 172.20.0.0/16 (IPv4) + fd00:dead:beef:1::/64 (IPv6)
- **Exposed Ports**: 60+ service ports + telemetry ports
- **WireGuard**: fd00:dead:beef::1/48 (IPv6 endpoint for host-b)

## Host-B: Outpost (Minimal)

Lightweight deployment for resource-constrained environments.

### Services (10 total)

#### Foundation (1)
1. **wotan** - NATS message bus (2.0 cpus, 512M mem)

#### Core Protocol (3)
2. **monad** - Protocol registry (1.0 cpus, 256M mem)
3. **sophia** - BPF dictionaries (1.0 cpus, 256M mem)
4. **anamnesis** - Event log (1.0 cpus, 256M mem)

#### API & Dashboard (2)
5. **dashboard-backend** - Metrics API (1.0 cpus, 256M mem)
6. **gateway** - API ingress (1.0 cpus, 256M mem)

#### Telemetry Agents (4)
7. **prometheus** - Scrape agent, remote_write (1.0 cpus, 256M mem)
8. **promtail** - Log forwarder (0.5 cpus, 128M mem)
9. **node-exporter** - Hardware metrics (0.5 cpus, 128M mem)
10. **wireguard** - VPN bridge (1.0 cpus, 256M mem)

### Resource Summary (Host-B)
- **Total CPU Allocation**: ~10.5 cores (conservative on 8-core system)
- **Total Memory Allocation**: ~2.5GB (out of 8GB available)
- **GPU**: None

### Networking (Host-B)
- **Docker Network**: 172.21.0.0/16 (IPv4) + fd00:dead:beef:2::/64 (IPv6)
- **Exposed Ports**: 6 service ports + telemetry ports
- **WireGuard**: fd00:dead:beef::2/48 (IPv6 endpoint for host-a)

## Inter-Host Communication

### WireGuard VPN Tunnel
- **Network**: fd00:dead:beef::/48 (IPv6)
- **Host-A Address**: fd00:dead:beef::1
- **Host-B Address**: fd00:dead:beef::2
- **Port**: 51820 UDP
- **Encryption**: WireGuard (modern, fast)
- **Use Cases**:
  - Service-to-service RPC calls
  - Metrics forwarding (Prometheus remote_write)
  - Log shipping (Promtail to Loki)

### Service Communication Patterns

```
Host-B Services → WireGuard VPN → Host-A Services
     ↓
Prometheus metrics → VictoriaMetrics remote_write
Promtail logs → Loki
Service RPC calls → Dashboard-Backend, Protocol-API, etc.
```

### Data Flow

1. **Metrics Flow**:
   ```
   Host-B Prometheus (7-day retention)
       ↓ remote_write
   Host-A VictoriaMetrics (90-day retention)
       ↓
   Prometheus (queries from Grafana)
       ↓
   Grafana (visualization)
   ```

2. **Logs Flow**:
   ```
   Host-B Docker containers
       ↓ Promtail
   Host-A Loki
       ↓
   Grafana (log queries)
   ```

3. **Service Communication**:
   ```
   Host-B Services
       ↓ gRPC/HTTP over WireGuard IPv6
   Host-A Services (monad, sophia, anamnesis, etc.)
   ```

## Security Architecture

### Network Security

1. **WireGuard Encryption**:
   - All inter-host communication encrypted
   - IPv6-based, modern cryptography
   - No exposed plaintext communication

2. **Docker Network Isolation**:
   - Separate bridge networks per host
   - Internal service names resolution only
   - No external access to internal services (except exposed ports)

3. **Capability Restrictions**:
   ```
   Most services: no-new-privileges + minimal caps
   
   shield: BPF, NET_ADMIN, SYS_ADMIN, SYS_RESOURCE (eBPF/XDP)
   daemon: BPF, NET_ADMIN (network operations)
   lich: NET_RAW, NET_ADMIN (packet manipulation)
   blackmage: NET_RAW (raw socket operations)
   ```

4. **Filesystem Security**:
   - All services: `read_only: true` except WireGuard
   - `/tmp` isolated per service (tmpfs, 64MB limit)
   - No access to host filesystem except volumes

### Data Security

1. **Persistent Volumes**:
   - Named volumes per service (isolated)
   - Host path isolation via Docker volume drivers
   - Encryption at rest can be enabled at host level

2. **Secrets Management**:
   - `.env` file (not committed to repo)
   - Environment variables passed to containers
   - Grafana secrets stored in volume
   - WireGuard keys in dedicated config volume

3. **Telemetry Data**:
   - Metrics stored in named volumes
   - 90-day retention on host-a
   - 7-day retention on host-b (minimal footprint)
   - Logs shipped encrypted via WireGuard

## Service Dependencies

### Host-A Service Startup Order

```
1. wotan (NATS)
   ↓
2. monad, sophia, anamnesis (depend on wotan)
   ↓
3. shield, unheaded-daemon (depend on monad+sophia)
   ↓
4. All other services (depend on unheaded-daemon)
   ↓
5. prometheus, victoriametrics (scrape services)
   ↓
6. loki, grafana, ebpf-exporter
```

### Host-B Service Startup Order

```
1. wotan (NATS)
   ↓
2. monad, sophia, anamnesis
   ↓
3. dashboard-backend, gateway
   ↓
4. prometheus, node-exporter, promtail
   ↓
5. wireguard (VPN bridge)
```

## Storage Architecture

### Host-A Storage
- **Total Named Volumes**: 30+
- **Typical Size Per Service**: 10-100MB
- **Total Footprint**: ~2-5GB (excluding telemetry)
- **Telemetry Storage**:
  - Prometheus: ~10GB/month (5s interval, 30 services)
  - VictoriaMetrics: ~20GB/3 months (with compression)
  - Loki: ~50GB/month (all container logs)

### Host-B Storage
- **Total Named Volumes**: 8
- **Typical Size Per Service**: 5-50MB
- **Total Footprint**: ~500MB
- **Telemetry Storage**:
  - Prometheus: ~200MB/week (7-day retention)
  - Local config: <100MB

## Scaling Considerations

### Adding More Hosts
1. Deploy Host-B-N (more Outposts)
2. Configure WireGuard mesh (each peer connects to all others)
3. Update Host-A Prometheus to scrape Host-B-N
4. Services use service discovery for load balancing

### Increasing Host-A Capacity
- Add CPU cores: increase resource limits
- Add RAM: increase service limits for memory-hungry services
- Add GPU: enable vLLM for lich service

### Reducing Host-B Resource Usage
- Disable eBPF-based services (shield, daemon)
- Increase prometheus scrape_interval (trade latency for resources)
- Reduce log verbosity
- Remove non-essential services

## Monitoring & Observability

### Metrics
- **Prometheus**: Service health, request counts, latencies
- **VictoriaMetrics**: Historical analysis, long-term trends
- **Node-Exporter**: CPU, memory, disk, network hardware
- **eBPF-Exporter**: Kernel-level metrics

### Logs
- **Loki**: All container logs, searchable by service/host
- **Promtail**: Log collection from Host-B

### Traces
- **Trace-Collector**: Distributed tracing (if enabled)
- **Tempo**: Trace backend (future integration)

### Dashboards
- **Grafana**: Unified visibility across all hosts
- Dark theme by default
- Auto-provisioned datasources and dashboards

## Deployment Options

### Option 1: Two Separate Hosts
```
Physical Host-A Machine (16+ cores, 64GB RAM)
    ↓ WireGuard VPN
Physical Host-B Machine (8 cores, 8GB RAM)
```

### Option 2: VMs on Single Server
```
Hypervisor (16+ cores, 128GB RAM)
    ├─ VM Host-A (16 cores, 64GB RAM)
    └─ VM Host-B (8 cores, 8GB RAM)
    WireGuard over network or bridge
```

### Option 3: Kubernetes Adaptation
```
Kubernetes Cluster
    ├─ Host-A Namespace (all 25 services)
    ├─ Host-B Namespace (6 core services)
    ├─ WireGuard pod
    └─ Telemetry namespace (central monitoring)
```

## File Structure

```
unheaded/docker/hosts/
├── ARCHITECTURE.md (this file)
├── host-a/
│   ├── docker-compose.yml (25 services + telemetry)
│   ├── .env.example (environment template)
│   ├── prometheus.yml (scrape config)
│   ├── loki-config.yaml (log aggregation)
│   ├── grafana-provisioning/
│   │   ├── datasources/unheaded.yml
│   │   └── dashboards/unheaded.yml
│   └── README.md (host-a specific docs)
├── host-b/
│   ├── docker-compose.yml (6 services + agents)
│   ├── .env.example (environment template)
│   ├── prometheus.yml (remote_write config)
│   ├── promtail-config.yaml (log forwarding)
│   └── README.md (host-b specific docs)
```

## Quick Start Summary

### Host-A Setup (5 minutes)
```bash
cd host-a
cp .env.example .env
# Edit .env with WireGuard keys and secrets
docker-compose up -d
# Access: Grafana at :3001, Prometheus at :9090
```

### Host-B Setup (5 minutes)
```bash
cd host-b
cp .env.example .env
# Edit .env with Host-A's IP and WireGuard keys
docker-compose up -d
# Verify: docker-compose exec prometheus curl http://[fd00:dead:beef::1]:8428
```

## Performance Benchmarks

### Service Startup Time
- Host-A: ~30-60 seconds (wait for NATS, then cascade start)
- Host-B: ~20-30 seconds (lighter workload)

### Metrics Ingestion
- Prometheus: 1K metrics/second capacity
- VictoriaMetrics: 10K metrics/second capacity
- Remote write: 100K metrics/minute (host-b to host-a)

### Log Processing
- Loki: 1GB/hour ingest capacity
- Promtail: 100MB/min forward capacity

### Network Throughput
- WireGuard: <10MB/s typical (metrics + logs)
- Required bandwidth: 1-10Mbps depending on service load

## Troubleshooting Resources

See individual host READMEs:
- `host-a/README.md` - Full-stack troubleshooting
- `host-b/README.md` - Outpost-specific issues
- Each has dedicated "Troubleshooting" section

## Future Enhancements

1. **Service Mesh Integration**: Istio/Linkerd for advanced networking
2. **Kubernetes Adaptation**: Helm charts for K8s deployment
3. **Multi-Region**: Additional WireGuard peers in different locations
4. **GPU Acceleration**: vLLM integration on host-a
5. **Advanced Telemetry**: OpenTelemetry collectors, Jaeger tracing
6. **Auto-Scaling**: Horizontal pod autoscaling for compute services
