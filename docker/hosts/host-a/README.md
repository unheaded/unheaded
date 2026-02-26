# Unheaded Kingdom - Host-A (Forge)

Full-stack deployment of all 25 Unheaded Kingdom services plus complete telemetry stack.

## Hardware Requirements

- **CPU**: 16+ cores
- **RAM**: 64GB
- **GPU**: RX 7700 XT (optional, for vLLM acceleration)
- **Storage**: SSD recommended for telemetry data

## Quick Start

### 1. Generate WireGuard Keys

```bash
# Generate private key
wg genkey > wireguard.private.key

# Derive public key
cat wireguard.private.key | wg pubkey > wireguard.public.key

# Display keys for configuration
echo "Private:" && cat wireguard.private.key
echo "Public:" && cat wireguard.public.key
```

### 2. Configure Environment

```bash
# Copy example configuration
cp .env.example .env

# Edit with your values
nano .env
```

Required configuration:
- `VERSION`: Docker image tag (default: `dev`)
- `LOG_LEVEL`: Logging level (default: `info`)
- `WG_PRIVATE_KEY`: Your WireGuard private key
- `WG_PUBLIC_KEY`: Your WireGuard public key
- `WG_PEER_PUBLIC_KEY`: Host-B's public key
- `WG_ENDPOINT_PEER`: Host-B IP address or hostname
- `GRAFANA_ADMIN_PASSWORD`: Grafana admin password
- `GRAFANA_SECRET_KEY`: 32+ character secret
- `VM_RETENTION_PERIOD`: VictoriaMetrics retention (default: `90d`)

### 3. Start Services

```bash
# Start all services
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f

# Check specific service
docker-compose logs unheaded-wotan
```

### 4. Access Services

**Dashboard & Monitoring:**
- Grafana: http://localhost:3001 (default: admin/changeme)
- Prometheus: http://localhost:9090
- VictoriaMetrics: http://localhost:8428
- Loki: http://localhost:3100

**APIs:**
- Gateway: http://localhost:8080
- Protocol API: http://localhost:7001
- Dashboard Backend: http://localhost:7000
- Trace Collector: http://localhost:7002

**Message Bus:**
- NATS: localhost:4222
- NATS HTTP: http://localhost:8222

## Service Architecture

### NATS Message Bus (Foundation)
- **wotan**: NATS server with HTTP monitoring

### Core Protocol Services
- **monad**: Protocol registry and coordination
- **sophia**: BPF dictionary and network definitions
- **anamnesis**: Event log and history

### Core Daemons
- **shield**: Network policy enforcement (eBPF-based)
- **unheaded-daemon**: Main daemon process

### API & Dashboard
- **dashboard-backend**: Metrics aggregation API
- **dashboard-frontend**: Web UI (via dashboard-backend)
- **protocol-api**: Protocol interface API
- **trace-collector**: Distributed trace collection
- **gateway**: API gateway and load balancer
- **service-discovery**: Service registration and lookup

### High-Performance Services
- **doom**: High-throughput packet processor
- **lich**: Advanced packet manipulation and inspection

### Orchestration
- **captain**: Service orchestration
- **micromanager**: Fine-grained resource management
- **timeguru**: Time synchronization and scheduling
- **architect**: Infrastructure design and planning
- **developer**: Development environment management

### Data & Knowledge
- **kanban**: Project management and workflow
- **lore**: Knowledge base and documentation
- **busboy**: Service mesh sidecar
- **kingdom**: Core state management
- **blackmage**: Magic/spell execution (custom logic)
- **moatghost**: Network perimeter security

### Telemetry Stack
- **prometheus**: Metrics collection (30-day retention)
- **victoriametrics**: Long-term metrics storage (90-day default)
- **loki**: Log aggregation and storage
- **grafana**: Visualization and dashboarding
- **ebpf-exporter**: eBPF program metrics

## Health Checks

All services include health checks. Monitor health status:

```bash
# Check all service health
docker-compose ps

# Check individual service
docker-compose exec unheaded-wotan curl http://localhost:8222/healthz

# View service health in logs
docker-compose logs --since 5m | grep health
```

## Resource Limits

Total resource allocation:
- **CPU**: ~40 cores (with safety margin on 16-core system)
- **Memory**: ~18GB (out of 64GB available)

Per-service limits are defined in docker-compose.yml and match NixOS cgroup v2 settings.

## Networking

### Docker Networks
- **unheaded bridge network**: 172.20.0.0/16 (IPv4) + fd00:dead:beef:1::/64 (IPv6)
- Services communicate via DNS within network
- External access via exposed ports

### WireGuard VPN
- Provides encrypted tunnel to Host-B (Outpost)
- IPv6 addresses: host-a=::1, host-b=::2 within fd00:dead:beef::/48
- Port 51820/UDP

## Security

All services run with:
- `no-new-privileges: true` (prevents privilege escalation)
- `read-only: true` root filesystem
- `/tmp` mounted as tmpfs with size limits
- Custom capabilities for privileged operations (shield, daemon, lich)

Exceptions:
- **shield**: Uses BPF + NET_ADMIN + SYS_ADMIN for eBPF/XDP operations
- **unheaded-daemon**: BPF + NET_ADMIN for network operations
- **lich**: NET_RAW + NET_ADMIN for packet manipulation

## Persistent Storage

Named volumes for stateful services:
- Protocol services: `wotan-data`, `monad-data`, `sophia-data`, `anamnesis-data`
- Daemons: `shield-data`, `daemon-data`
- APIs: `dashboard-backend-data`, `protocol-api-data`, `trace-collector-data`, `gateway-data`, `service-discovery-data`
- Performance: `doom-data`, `lich-data`
- Orchestration: `captain-data`, `micromanager-data`, `timeguru-data`, `architect-data`, `developer-data`
- Knowledge: `kanban-data`, `lore-data`, `busboy-data`, `kingdom-data`, `blackmage-data`, `moatghost-data`
- Telemetry: `prometheus-data`, `victoriametrics-data`, `loki-data`, `grafana-data`

List volumes:
```bash
docker volume ls | grep unheaded
```

Clean up volumes (WARNING: deletes data):
```bash
docker-compose down -v
```

## Troubleshooting

### Service won't start
```bash
# Check logs
docker-compose logs unheaded-{service-name}

# Verify image exists
docker images | grep unheaded-

# Check resource limits
docker stats --no-stream
```

### Health check failing
```bash
# Test service manually
docker-compose exec unheaded-{service} curl http://localhost:{port}/health

# Check service logs for errors
docker-compose logs unheaded-{service}

# Check network connectivity
docker-compose exec unheaded-{service} ping wotan
```

### High resource usage
```bash
# Monitor resource usage
docker stats

# Check specific service
docker-compose exec unheaded-{service} top
```

### Metrics not appearing in Prometheus
```bash
# Check Prometheus targets
curl http://localhost:9090/api/v1/targets

# Verify service is exposing metrics
curl http://localhost:{service-port}/metrics

# Check Prometheus configuration
cat prometheus.yml
```

## Maintenance

### Update services
```bash
# Update image tag
export VERSION=v1.0.0

# Restart services with new version
docker-compose up -d
```

### View logs
```bash
# All logs from past hour
docker-compose logs --since 1h

# Follow specific service
docker-compose logs -f unheaded-dashboard-backend

# Extract error logs
docker-compose logs | grep ERROR
```

### Backup data
```bash
# Backup all volumes
for volume in $(docker volume ls --filter name=unheaded -q); do
  docker run --rm -v $volume:/data -v $(pwd)/backup:/backup \
    alpine tar czf /backup/$volume.tar.gz -C /data .
done
```

### Cleanup
```bash
# Stop all services
docker-compose down

# Remove images
docker-compose down --rmi all

# Prune unused Docker resources
docker system prune -a
```

## Configuration Files

- **docker-compose.yml**: Service definitions and orchestration
- **.env**: Environment variables (create from .env.example)
- **prometheus.yml**: Prometheus scrape targets and configuration
- **loki-config.yaml**: Loki log aggregation configuration
- **grafana-provisioning/datasources/**: Grafana data source definitions
- **grafana-provisioning/dashboards/**: Grafana dashboard provisioning

## Integration with Host-B (Outpost)

Host-B acts as a minimal outpost with:
- 6 core services (wotan, monad, sophia, anamnesis, dashboard-backend, gateway)
- Local Prometheus for metrics scraping
- Promtail for log forwarding to host-a Loki
- WireGuard bridge to host-a

Services on host-b forward metrics to host-a's VictoriaMetrics via `remote_write`.

## Performance Tuning

### For high-throughput packet processing
- Increase doom/lich CPU allocation
- Check network card IRQ distribution
- Monitor shield eBPF program performance

### For large deployments
- Scale VictoriaMetrics retention based on storage
- Increase Loki chunk size for high-volume logs
- Consider external S3-compatible storage for long-term metrics

## Support and Documentation

See main Unheaded Kingdom documentation for:
- Service API specifications
- Network protocol details
- Development guidelines
- Advanced configuration options
