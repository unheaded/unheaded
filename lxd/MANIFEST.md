# Unheaded Kingdom Container Manifest

**25 Microservices Deployment Guide**  
**LXD 5.x Target | Ubuntu 24.04 Base Image**

---

## Service Categories & Deployment

### API & Web Services (10 containers)

| Service | Profiles | Cloud-Init | CPU | RAM | Storage | Notes |
|---------|----------|-----------|-----|-----|---------|-------|
| api-gateway | base, service | base.yaml | 2 | 768MB | 8GB | Entry point, high throughput |
| auth-service | base, service | base.yaml | 2 | 768MB | 8GB | OIDC/JWT, rate-limiting |
| user-service | base, service | base.yaml | 2 | 768MB | 8GB | User profiles, preferences |
| catalog-service | base, service | base.yaml | 2 | 768MB | 8GB | Product catalog, search |
| order-service | base, service | base.yaml | 2 | 768MB | 8GB | Order management, workflows |
| payment-processor | base, service | base.yaml | 2 | 1GB | 8GB | Payment integrations, PCI |
| notification-service | base, service | base.yaml | 2 | 768MB | 8GB | Email, SMS, push notifications |
| search-indexer | base, service | base.yaml | 2 | 1GB | 10GB | Elasticsearch client, indexing |
| recommendation-engine | base, service | base.yaml | 3 | 1.5GB | 12GB | ML-based suggestions |
| analytics-pipeline | base, service | base.yaml | 2 | 1GB | 10GB | Event stream processing |

**Launch Example (api-gateway):**
```bash
lxc launch images:ubuntu/24.04/cloud api-gateway \
  --profile unheaded-base \
  --profile unheaded-service \
  --config user.user-data="$(cat cloud-init/base.yaml)" \
  --devices eth0.network=unheaded
```

---

### eBPF & Network Observability (5 containers)

| Service | Profiles | Cloud-Init | CPU | RAM | Storage | Notes |
|---------|----------|-----------|-----|-----|---------|-------|
| shield | base, ebpf | base.yaml + ebpf.yaml | 4 | 2GB | 8GB | DDoS mitigation, XDP |
| unheaded-daemon | base, ebpf | base.yaml + ebpf.yaml | 4 | 2GB | 8GB | Service mesh sidecar |
| ebpf-exporter | base, ebpf | base.yaml + ebpf.yaml | 2 | 1.5GB | 8GB | BPF to Prometheus metrics |
| network-tracer | base, ebpf | base.yaml + ebpf.yaml | 2 | 1.5GB | 8GB | Packet inspection, PCAP |
| syscall-monitor | base, ebpf | base.yaml + ebpf.yaml | 2 | 1.5GB | 8GB | Syscall auditing, seccomp |

**Launch Example (shield):**
```bash
lxc launch images:ubuntu/24.04/cloud shield \
  --profile unheaded-base \
  --profile unheaded-ebpf \
  --config user.user-data="$(cat cloud-init/base.yaml && cat cloud-init/ebpf.yaml)" \
  --devices eth0.network=unheaded
```

---

### Data & Persistence (6 containers)

| Service | Profiles | Cloud-Init | CPU | RAM | Storage | Notes |
|---------|----------|-----------|-----|-----|---------|-------|
| postgres | base, service | base.yaml | 4 | 2GB | 50GB | Primary RDBMS |
| redis-primary | base, service | base.yaml | 2 | 1GB | 20GB | Cache, sessions |
| redis-replica | base, service | base.yaml | 2 | 1GB | 20GB | Replica, failover |
| elasticsearch | base, service | base.yaml | 4 | 3GB | 50GB | Full-text search, logging |
| minio | base, service | base.yaml | 2 | 1GB | 100GB | S3-compatible object storage |
| mongodb | base, service | base.yaml | 4 | 2GB | 50GB | Document store, backups |

**Launch Example (postgres):**
```bash
lxc launch images:ubuntu/24.04/cloud postgres \
  --profile unheaded-base \
  --profile unheaded-service \
  --config user.user-data="$(cat cloud-init/base.yaml)" \
  --devices eth0.network=unheaded
```

---

### AI/ML & LLM Inference (2 containers)

| Service | Profiles | Cloud-Init | CPU | RAM | GPU | Storage | Notes |
|---------|----------|-----------|-----|-----|-----|---------|-------|
| vllm-deepseek | base, gpu | base.yaml + gpu.yaml | 8 | 16GB | RX 7700 XT | 30GB | LLM inference, batching |
| embeddings-service | base, gpu | base.yaml + gpu.yaml | 8 | 16GB | RX 7700 XT | 20GB | Semantic embeddings, FAISS |

**Launch Example (vllm-deepseek, host-a only):**
```bash
lxc launch images:ubuntu/24.04/cloud vllm-deepseek \
  --profile unheaded-base \
  --profile unheaded-gpu \
  --config user.user-data="$(cat cloud-init/base.yaml && cat cloud-init/gpu.yaml)" \
  --devices eth0.network=unheaded
```

---

### Observability & Telemetry (2 containers)

| Service | Profiles | Cloud-Init | CPU | RAM | Storage | Retention | Notes |
|---------|----------|-----------|-----|-----|---------|-----------|-------|
| prometheus | base, telemetry | base.yaml | 4 | 4GB | 50GB | 30d | Metrics scraping, alerting |
| grafana | base, service | base.yaml | 2 | 1GB | 10GB | — | Visualization, dashboards |
| loki | base, telemetry | base.yaml | 4 | 4GB | 50GB | 30d | Log aggregation |
| victoria-metrics | base, telemetry | base.yaml | 4 | 4GB | 50GB | 30d | Long-term metric retention |

**Launch Example (prometheus):**
```bash
lxc launch images:ubuntu/24.04/cloud prometheus \
  --profile unheaded-base \
  --profile unheaded-telemetry \
  --config user.user-data="$(cat cloud-init/base.yaml)" \
  --devices eth0.network=unheaded
```

---

## Deployment Checklist

### Phase 1: Infrastructure Setup
- [ ] Verify LXD 5.x installed on host-a and host-b
- [ ] ZFS pool ready: `zfs list unheaded`
- [ ] WireGuard tunnel operational between hosts
- [ ] Run: `./deploy.sh init`

### Phase 2: Base Configuration
- [ ] Storage pool created: `lxc storage list`
- [ ] Network bridge created: `lxc network list`
- [ ] All profiles imported: `lxc profile list`

### Phase 3: Critical Services (Parallel)
1. **Database Tier** (postgres, redis-primary, elasticsearch)
   ```bash
   for svc in postgres redis-primary elasticsearch; do
     lxc launch images:ubuntu/24.04/cloud $svc \
       --profile unheaded-base --profile unheaded-service \
       --config user.user-data="$(cat cloud-init/base.yaml)"
   done
   ```

2. **Observability Tier** (prometheus, loki, victoria-metrics)
   ```bash
   for svc in prometheus loki victoria-metrics; do
     lxc launch images:ubuntu/24.04/cloud $svc \
       --profile unheaded-base --profile unheaded-telemetry \
       --config user.user-data="$(cat cloud-init/base.yaml)"
   done
   ```

### Phase 4: Network Layer
1. **eBPF Services** (shield, unheaded-daemon, network-tracer)
   ```bash
   for svc in shield unheaded-daemon network-tracer; do
     lxc launch images:ubuntu/24.04/cloud $svc \
       --profile unheaded-base --profile unheaded-ebpf \
       --config user.user-data="$(cat cloud-init/base.yaml && cat cloud-init/ebpf.yaml)"
   done
   ```

### Phase 5: API Services (Sequential, health checks)
```bash
api_services=(api-gateway auth-service user-service catalog-service order-service)
for svc in "${api_services[@]}"; do
  lxc launch images:ubuntu/24.04/cloud "$svc" \
    --profile unheaded-base --profile unheaded-service \
    --config user.user-data="$(cat cloud-init/base.yaml)"
  sleep 10
  lxc exec "$svc" -- systemctl status # or health check endpoint
done
```

### Phase 6: AI/ML Services (Host-A only, post-GPU verification)
```bash
lxc launch images:ubuntu/24.04/cloud vllm-deepseek \
  --profile unheaded-base --profile unheaded-gpu \
  --config user.user-data="$(cat cloud-init/base.yaml && cat cloud-init/gpu.yaml)"

# Verify GPU passthrough
lxc exec vllm-deepseek -- rocm-smi
```

---

## Health Checks

### Container Startup Verification
```bash
# Check container is running
lxc info <container-name> | grep Status

# Check systemd services
lxc exec <container-name> -- systemctl status

# Check network connectivity
lxc exec <container-name> -- ping 8.8.8.8
lxc exec <container-name> -- nslookup unheaded.internal

# Check disk usage
lxc exec <container-name> -- df -h /
```

### eBPF Service Verification
```bash
lxc exec shield -- ls -la /sys/fs/bpf/
lxc exec shield -- bpftool prog list
lxc exec shield -- systemctl status shield
```

### GPU Service Verification
```bash
lxc exec vllm-deepseek -- rocm-smi
lxc exec vllm-deepseek -- rocminfo | grep gfx1101
lxc exec vllm-deepseek -- cat /proc/meminfo | grep HugePages
```

---

## Networking Details

### IPv4 Addressing (DHCP Pool)
- Bridge gateway: `10.20.0.1`
- DHCP range: `10.20.1.0 - 10.20.1.250`
- Containers get addresses from DHCP pool automatically

### IPv6 Addressing (Stateful DHCPv6)
- Bridge gateway: `fd00:dead:beef:1::1/64`
- Containers assigned unique addresses in range
- Prefix delegation to host-b via WireGuard

### DNS Resolution
- Internal domain: `unheaded.internal`
- Container names resolvable: `api-gateway.unheaded.internal`
- Managed by LXD embedded DNS (port 53 on 10.20.0.1)

---

## Resource Limits Summary

| Profile | CPU | RAM | Priority | Use Case |
|---------|-----|-----|----------|----------|
| base | 2 | 512MB | 5 | Default fallback |
| service | 2 | 768MB | 6 | Standard microservices |
| ebpf | 4 | 2GB | 8 | Network, BPF workloads |
| gpu | 8 | 16GB | 9 | LLM inference, GPU |
| telemetry | 4 | 4GB | 7 | Observability stack |

---

## Production Considerations

1. **Snapshots & Backups**
   - Snapshot database containers hourly
   - Off-site backup to MinIO S3 bucket
   - Test restore procedures monthly

2. **Security**
   - All containers unprivileged (uid 10001)
   - eBPF services: CAP_BPF only (no full privilege)
   - TLS certificates: `/etc/unheaded/certs/`
   - Secrets: Use LXD config encryption

3. **Monitoring**
   - Prometheus scrapes all containers on port 9090
   - Loki aggregates container logs
   - Alerts: CPU > 80%, Memory > 85%, Disk > 90%

4. **Scaling**
   - Redis replica can become primary failover
   - Elasticsearch can add data nodes
   - vLLM can be split across host-a and host-b (with cross-host access)

---

## Emergency Procedures

### Container Recovery
```bash
# Stop container cleanly
lxc stop <container-name>

# Restore from snapshot
lxc copy <container-name>/snapshot snapshot-name <container-name>
lxc start <container-name>

# Full reset (destructive)
lxc delete <container-name>
lxc launch images:ubuntu/24.04/cloud <container-name> --profile unheaded-base
```

### Network Troubleshooting
```bash
# Inspect bridge
lxc network show unheaded

# Restart bridge
lxc network stop unheaded && lxc network start unheaded

# Container network info
lxc info <container-name> | grep Address
```

---

## License

SPDX-License-Identifier: MIT  
Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
