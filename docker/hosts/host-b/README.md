# Unheaded Kingdom - Host-B (Outpost)

Minimal deployment of 6 core services designed for resource-constrained environments with remote telemetry aggregation.

## Hardware Requirements

- **CPU**: 4-8 cores (consumer grade)
- **RAM**: 8GB
- **GPU**: None
- **Storage**: 50GB+ SSD (mainly for local metrics cache and logs)

## Quick Start

### 1. Ensure WireGuard Keys from Host-A

Before starting host-b, you need the WireGuard public key from host-a. On host-a, run:

```bash
# On host-a
cat wireguard.public.key
```

And generate host-b's keys:

```bash
# On host-b
wg genkey > wireguard.private.key
cat wireguard.private.key | wg pubkey > wireguard.public.key

# Share host-b's public key with host-a
cat wireguard.public.key
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
- `WG_PEER_PUBLIC_KEY`: Host-A's public key
- `WG_ENDPOINT_HOST`: Host-A IP address or hostname

### 3. Start Services

```bash
# Start all services
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f

# Check WireGuard connection
docker-compose exec wireguard wg show
```

### 4. Verify Connectivity to Host-A

```bash
# Test IPv6 connectivity to host-a
docker-compose exec prometheus ping6 fd00:dead:beef::1

# Check if host-a telemetry is reachable
docker-compose exec prometheus curl -s http://[fd00:dead:beef::1]:8428/api/v1/labels | head -20
```

### 5. Access Services

**Local APIs:**
- Gateway: http://localhost:8080
- Dashboard Backend: http://localhost:7000
- Prometheus (scrape-only): http://localhost:9090

**Remote Telemetry (on host-a):**
- Grafana: http://{host-a}:3001
- VictoriaMetrics: http://{host-a}:8428
- Loki: http://{host-a}:3100

## Service Architecture

### NATS Message Bus (Foundation)
- **wotan**: NATS server (local instance for host-b services)

### Core Protocol Services
- **monad**: Protocol registry (minimal footprint)
- **sophia**: BPF dictionary (shared with host-a over network)
- **anamnesis**: Event log (local cache, syncs to host-a)

### API & Dashboard
- **dashboard-backend**: Metrics API (limited history)
- **gateway**: API gateway (forwarding to host-a when needed)

### Telemetry Agents
- **prometheus**: Scrape-only agent (7-day local retention)
- **promtail**: Log collector (forwards to host-a Loki)
- **node-exporter**: Hardware metrics
- **wireguard**: VPN bridge to host-a (IPv6 tunneling)

## Resource Limits

Per-service CPU and memory allocation (total ~3GB, 3.5 cores):
- wotan: 2.0 cpus, 512M
- monad: 1.0 cpus, 256M
- sophia: 1.0 cpus, 256M
- anamnesis: 1.0 cpus, 256M
- dashboard-backend: 1.0 cpus, 256M
- gateway: 1.0 cpus, 256M
- prometheus: 1.0 cpus, 256M
- promtail: 0.5 cpus, 128M
- node-exporter: 0.5 cpus, 128M
- wireguard: 1.0 cpus, 256M

Total: ~10.5 cpus, ~2.5GB (includes headroom)

## Networking

### Docker Networks
- **unheaded bridge network**: 172.21.0.0/16 (IPv4) + fd00:dead:beef:2::/64 (IPv6)
- Services communicate via DNS within network
- External access via exposed ports on host-b

### WireGuard VPN
- Encrypted tunnel to Host-A (Forge)
- IPv6 addresses: host-a=::1, host-b=::2 within fd00:dead:beef::/48
- Port 51820/UDP (must be forwarded on router)
- Provides secure inter-host communication

### Port Forwarding

If host-b is behind NAT, configure port forwarding:
- External: 51820 UDP → Internal: 51820 UDP (WireGuard)
- External: 8080 TCP → Internal: 8080 TCP (Gateway, optional)

## Security

All services run with:
- `no-new-privileges: true` (except WireGuard)
- `read-only: true` root filesystem
- `/tmp` mounted as tmpfs with size limits
- Minimal capabilities (only what's needed)

WireGuard service:
- Requires `NET_ADMIN` and `SYS_MODULE` capabilities
- Read-write access to `/config` volume (VPN config)

## Persistent Storage

Named volumes (minimal footprint):
- Protocol services: `wotan-data`, `monad-data`, `sophia-data`, `anamnesis-data`
- APIs: `dashboard-backend-data`, `gateway-data`
- Telemetry: `prometheus-data`, `wireguard-config`

## Monitoring & Logs

### Local Metrics
Prometheus on host-b collects local metrics and forwards to host-a:
```bash
# View scrape targets
curl http://localhost:9090/api/v1/targets

# Query local metrics (limited 7-day history)
curl http://localhost:9090/api/v1/query?query=up
```

### Logs
Promtail forwards logs to host-a Loki:
```bash
# Check Promtail status
docker-compose logs promtail

# View locally forwarded logs (on host-a)
# Login to Grafana → Explore → Loki → select host-b
```

### Node Metrics
Hardware metrics available via node-exporter:
```bash
curl http://localhost:9100/metrics | grep node_
```

## Troubleshooting

### WireGuard not connecting
```bash
# Check WireGuard status
docker-compose exec wireguard wg show

# Check configuration
docker-compose exec wireguard cat /config/wg0.conf

# Test DNS resolution
docker-compose exec prometheus getent hosts host-a

# Ping host-a over IPv6
docker-compose exec prometheus ping6 fd00:dead:beef::1
```

### Services not starting
```bash
# Check logs
docker-compose logs unheaded-{service}

# Verify image exists locally
docker images | grep unheaded-

# Check disk space
df -h

# Check memory
free -m
```

### Metrics not reaching host-a
```bash
# Check Prometheus remote_write config
cat prometheus.yml | grep -A5 remote_write

# Test connectivity to host-a VictoriaMetrics
docker-compose exec prometheus curl -v http://[fd00:dead:beef::1]:8428/api/v1/write

# Check Prometheus target health
curl http://localhost:9090/api/v1/targets | jq '.data.activeTargets'
```

### High resource usage
```bash
# Monitor resource usage
docker stats

# Check what's consuming memory
docker-compose exec prometheus ps aux --sort=-%mem

# Reduce scrape frequency in prometheus.yml
# (increase scrape_interval and evaluation_interval)
```

## Maintenance

### Update services
```bash
# Update image tag
export VERSION=v1.0.0

# Restart services
docker-compose up -d
```

### Backup WireGuard configuration
```bash
# Backup VPN keys before changing anything
docker cp unheaded-wireguard:/config /backup/wireguard-config-$(date +%s)
```

### View logs
```bash
# All logs from past hour
docker-compose logs --since 1h

# Follow specific service
docker-compose logs -f promtail

# Extract WireGuard logs
docker-compose logs wireguard | grep -i error
```

### Cleanup
```bash
# Stop all services
docker-compose down

# Remove images (will re-pull on next up)
docker-compose down --rmi all

# Clean up volumes (WARNING: deletes local data)
docker-compose down -v
```

## Integration with Host-A (Forge)

Host-B connects to Host-A via:

1. **WireGuard VPN**: Encrypted IPv6 tunnel
   - Enables service-to-service communication
   - Metrics forwarding over IPv6
   - Log shipping over IPv6

2. **Prometheus Remote Write**: Forwards metrics to host-a VictoriaMetrics
   ```yaml
   remote_write:
     - url: http://[fd00:dead:beef::1]:8428/api/v1/write
   ```

3. **Promtail Remote Logging**: Ships logs to host-a Loki
   ```yaml
   clients:
     - url: http://[fd00:dead:beef::1]:3100/loki/api/v1/push
   ```

## Configuration Files

- **docker-compose.yml**: Service definitions
- **.env**: Environment variables (create from .env.example)
- **prometheus.yml**: Prometheus scrape targets + remote_write config
- **promtail-config.yaml**: Log forwarding configuration

## Performance Notes

### Network Bandwidth
- Metrics forwarding: ~10-50KB/min (depends on scrape frequency)
- Log forwarding: ~100KB-1MB/min (depends on application volume)
- WireGuard overhead: ~50 bytes per packet

### Storage
- Local Prometheus: ~200MB/day for 10 services at 5s interval
- Volume storage on host-b: ~5-10GB recommended

### CPU
- Typical idle: <0.5 cores
- During metric collection: ~1-2 cores
- WireGuard tunnel: <0.1 cores

## Advanced Configuration

### Increase Prometheus retention
Edit `prometheus.yml`:
```yaml
global:
  scrape_interval: 10s  # from 10s to reduce disk usage
```

### Add additional services
To add more services to host-b, update `docker-compose.yml`:
1. Add service definition
2. Add appropriate memory/CPU limits (total must stay under 8GB)
3. Add to prometheus.yml scrape_configs
4. Add to promtail-config.yaml if it generates logs

### Custom metrics collection
Modify `prometheus.yml` to scrape additional targets or change intervals:
```yaml
scrape_configs:
  - job_name: 'custom-service'
    static_configs:
      - targets: ['custom-host:9090']
```

## Support and Documentation

For more information:
- Check Host-A README for full architecture details
- See main Unheaded Kingdom documentation
- Review docker-compose.yml comments for service-specific info
