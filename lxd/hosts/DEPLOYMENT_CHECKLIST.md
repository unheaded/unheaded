# Unheaded Kingdom LXD Deployment Checklist

## Pre-Deployment

### Environment Preparation
- [ ] Identify host-a and host-b target machines
- [ ] Verify hardware meets minimum requirements:
  - host-a: 16+ cores, 64GB RAM, 200GB+ storage, GPU (optional)
  - host-b: 4+ cores, 8GB RAM, 50GB+ storage
- [ ] Install fresh Ubuntu 24.04 LTS or Debian 12 on both hosts
- [ ] Ensure internet connectivity on both hosts
- [ ] Configure SSH key-based access to both hosts

### Repository Preparation
- [ ] Clone/fork Unheaded Kingdom repository
- [ ] Verify directory structure exists:
  ```
  repo/
  ├── lxd/
  │   ├── hosts/
  │   │   ├── host-a/
  │   │   └── host-b/
  │   └── profiles/
  ├── bin/  (will contain compiled service binaries)
  └── cloud-init/
  ```
- [ ] Build all service binaries (if not pre-compiled):
  ```bash
  make build-all
  # Binaries should be copied to repo/bin/
  ```

## host-a (The Forge) Deployment

### Pre-Deployment Checks
- [ ] SSH access to host-a confirmed
- [ ] Sufficient disk space verified:
  ```bash
  df -h | grep -E "/$|/var"  # Need 200GB+ free
  ```
- [ ] Snap daemon is running:
  ```bash
  systemctl status snapd
  ```
- [ ] No existing LXD installation (or plan for migration)

### Step 1: Copy Configuration Files
```bash
scp -r lxd/hosts/host-a/ user@host-a:/tmp/
scp -r lxd/profiles/ user@host-a:/tmp/host-a/
scp -r cloud-init/ user@host-a:/tmp/host-a/
```

Checklist:
- [ ] preseed.yaml copied
- [ ] init.sh copied
- [ ] launch-all.sh copied
- [ ] static-ips.yaml copied
- [ ] profiles/ directory copied
- [ ] cloud-init/ directory copied

### Step 2: Copy Service Binaries
```bash
scp -r bin/ user@host-a:/tmp/host-a/binaries/
```

Checklist:
- [ ] All 25 service binaries present
- [ ] Binaries are executable (check permissions)

### Step 3: Initialize LXD
```bash
ssh user@host-a
cd /tmp/host-a
sudo ./init.sh
```

Expected output:
- [ ] "LXD installed" message
- [ ] "LXD initialization complete!"
- [ ] No error messages about preseed failure
- [ ] "Host binary directory created"

### Step 4: Verify LXD Setup
```bash
lxc network ls        # Should show 'unheaded' bridge
lxc storage ls        # Should show 'unheaded-ssd' pool
lxc profile ls        # Should show default + custom profiles
```

Checklist:
- [ ] Network 'unheaded' exists with 10.20.0.0/16
- [ ] Storage pool 'unheaded-ssd' exists (200GB)
- [ ] Profile 'default' exists
- [ ] Custom profiles loaded (unheaded-base, unheaded-ebpf, etc.)

### Step 5: Verify Host Binary Directory
```bash
ls -lh /opt/unheaded/bin/
# Should show all 25 service binaries
```

Checklist:
- [ ] /opt/unheaded/bin/ directory exists
- [ ] All 25 service binaries present
- [ ] Binaries are executable

### Step 6: Launch All Services
```bash
cd /tmp/host-a
./launch-all.sh
```

Monitor progress:
- [ ] Phase 1: wotan launched and ready
- [ ] Phase 2: monad, sophia, anamnesis launched
- [ ] Phase 3: shield, unheaded-daemon launched
- [ ] Phase 4: gateway, service-discovery launched
- [ ] Phase 5: dashboard-backend, etc. launched
- [ ] Phase 6: trace-collector, doom launched
- [ ] Phase 7-8: All business services launched
- [ ] Phase 9: lich (adversary) launched
- [ ] Phase 10: Telemetry stack launched
- [ ] No containers failed to launch

### Step 7: Verify Containers Running
```bash
lxc list | grep unheaded
# Should show 30 containers (25 services + 5 telemetry)
```

Checklist:
- [ ] All 30 containers show "RUNNING" status
- [ ] Container count matches expected (30)
- [ ] All containers are in "unheaded" network

### Step 8: Verify Service Connectivity
```bash
# Test wotan connectivity
lxc exec unheaded-wotan -- nc -zv 127.0.0.1 50053
lxc exec unheaded-wotan -- nc -zv 127.0.0.1 4222

# Test gateway connectivity
lxc exec unheaded-gateway -- nc -zv 10.20.1.1 50053

# Test inter-container communication
lxc exec unheaded-monad -- ping -c 1 10.20.1.1
```

Checklist:
- [ ] wotan gRPC port (50053) responding
- [ ] wotan NATS port (4222) responding
- [ ] Containers can reach wotan
- [ ] Inter-container ping successful

### Step 9: Verify Observability Stack
```bash
# Check Prometheus
lxc exec unheaded-prometheus -- curl -s http://localhost:9090/api/v1/targets | head

# Check Grafana
lxc exec unheaded-grafana -- curl -s http://localhost:3000/api/v1/health

# Check Loki
lxc exec unheaded-loki -- curl -s http://localhost:3100/ready
```

Checklist:
- [ ] Prometheus API responding
- [ ] Grafana API responding
- [ ] Loki ready endpoint responding

### Step 10: Access Web UIs (from external host)
Get host-a IP address:
```bash
ssh user@host-a ip addr show | grep 'inet '
```

Then access from your local machine:
- [ ] Grafana: http://host-a-ip:3000 (admin/admin)
- [ ] Prometheus: http://host-a-ip:9090
- [ ] Dashboard Frontend: http://host-a-ip:3001

### Step 11: Configure Static IPs (Optional but Recommended)
```bash
# Use static-ips.yaml to set DHCP reservations
# This ensures consistent IPs for Prometheus scraping
```

Checklist:
- [ ] Reviewed static-ips.yaml
- [ ] Understand deterministic IP assignments
- [ ] (Optional) Configured DHCP reservations on host

## host-b (The Outpost) Deployment

### Pre-Deployment Checks
- [ ] SSH access to host-b confirmed
- [ ] Sufficient disk space verified:
  ```bash
  df -h | grep -E "/$|/var"  # Need 50GB+ free
  ```
- [ ] Snap daemon is running:
  ```bash
  systemctl status snapd
  ```
- [ ] Network connectivity to host-a planned

### Step 1: Copy Configuration Files
```bash
scp -r lxd/hosts/host-b/ user@host-b:/tmp/
scp -r lxd/profiles/ user@host-b:/tmp/host-b/
scp -r cloud-init/ user@host-b:/tmp/host-b/
```

Checklist:
- [ ] preseed.yaml copied
- [ ] init.sh copied
- [ ] launch-minimal.sh copied
- [ ] profiles/ directory copied
- [ ] cloud-init/ directory copied

### Step 2: Copy Service Binaries
```bash
scp -r bin/ user@host-b:/tmp/host-b/binaries/
# Only need 6 core services + 3 telemetry agents
```

Checklist:
- [ ] wotan binary present
- [ ] monad, sophia, anamnesis binaries present
- [ ] gateway, dashboard-backend binaries present
- [ ] prometheus-agent, promtail, node-exporter present

### Step 3: Create WireGuard Configuration (host-b → host-a)
Before init.sh, configure WireGuard:

```bash
# On host-b, create wireguard.conf
# Get host-a public key and endpoint

# Transfer config to host-b
scp wireguard.conf user@host-b:/tmp/host-b/wireguard.conf
```

Checklist:
- [ ] WireGuard configuration file created
- [ ] Host-a endpoint and key documented
- [ ] Host-b private key generated
- [ ] Config copied to host-b

### Step 4: Initialize LXD
```bash
ssh user@host-b
cd /tmp/host-b
sudo ./init.sh
```

Expected output:
- [ ] "LXD installed" message
- [ ] "LXD initialization complete!"
- [ ] "WireGuard configuration complete"
- [ ] "Host binary directory created"

### Step 5: Verify LXD Setup
```bash
lxc network ls        # Should show 'unheaded-outpost' bridge
lxc storage ls        # Should show 'unheaded-minimal' pool
```

Checklist:
- [ ] Network 'unheaded-outpost' exists with 10.21.0.0/16
- [ ] Storage pool 'unheaded-minimal' exists (50GB)
- [ ] Profile 'default' exists

### Step 6: Verify WireGuard Tunnel
```bash
wg show wg0
# Should show interface up with peer connection

# Test tunnel connectivity to host-a
ping -c 3 10.20.0.1
```

Checklist:
- [ ] WireGuard interface wg0 is up
- [ ] Peer (host-a) is showing in wg show
- [ ] Can ping host-a gateway through tunnel
- [ ] Handshake recent (< 2 minutes)

### Step 7: Verify Host Binary Directory
```bash
ls -lh /opt/unheaded/bin/
# Should show core service binaries
```

Checklist:
- [ ] /opt/unheaded/bin/ directory exists
- [ ] 6 core service binaries present
- [ ] 3 telemetry agent binaries present
- [ ] All binaries are executable

### Step 8: Launch Core Services
```bash
cd /tmp/host-b
./launch-minimal.sh
```

Monitor progress:
- [ ] Phase 1: wotan launched (local bus)
- [ ] Phase 2: monad, sophia, anamnesis launched
- [ ] Phase 3: gateway, dashboard-backend launched
- [ ] Phase 4: telemetry agents launched
- [ ] No containers failed to launch

### Step 9: Verify Containers Running
```bash
lxc list | grep unheaded
# Should show 9 containers (6 core + 3 telemetry)
```

Checklist:
- [ ] All 9 containers show "RUNNING" status
- [ ] Container count is 9
- [ ] All containers are in "unheaded-outpost" network

### Step 10: Verify Service Connectivity
```bash
# Test local wotan connectivity
lxc exec unheaded-wotan -- nc -zv 127.0.0.1 50053

# Test inter-container communication
lxc exec unheaded-monad -- ping -c 1 10.21.1.1

# Test tunnel connectivity to host-a
lxc exec unheaded-prometheus-agent -- ping -c 1 10.20.1.1
```

Checklist:
- [ ] Local wotan is responding
- [ ] Containers can reach local wotan
- [ ] Containers can reach host-a through tunnel
- [ ] No packet loss on inter-container ping

### Step 11: Verify Telemetry Agents Configuration
```bash
# Check Prometheus agent config
lxc exec unheaded-prometheus-agent -- cat /etc/prometheus/prometheus.yml | grep remote_write

# Check Promtail config
lxc exec unheaded-promtail -- cat /etc/promtail/config.yml | grep clients

# Check node-exporter
lxc exec unheaded-node-exporter -- systemctl status node-exporter
```

Checklist:
- [ ] Prometheus agent has remote_write to host-a (10.20.1.50:9090)
- [ ] Promtail is configured to ship to host-a Loki (10.20.1.52:3100)
- [ ] Node-exporter is running and exporting metrics

### Step 12: Verify Remote Telemetry Reception (on host-a)
```bash
ssh user@host-a

# Check if host-b metrics appear in Prometheus
lxc exec unheaded-prometheus -- curl -s 'http://localhost:9090/api/v1/targets' | grep host-b

# Check if host-b logs appear in Loki
lxc exec unheaded-loki -- curl -s 'http://localhost:3100/loki/api/v1/labels' | grep job_name
```

Checklist:
- [ ] host-b Prometheus agent shows as "UP" in host-a Prometheus
- [ ] host-b metrics are being scraped
- [ ] host-b logs are appearing in Loki
- [ ] Dashboard shows host-b metrics

## Post-Deployment

### host-a Verification
- [ ] All 25 services responding to health checks
- [ ] All telemetry components are receiving data
- [ ] Grafana dashboards are populated with metrics
- [ ] Log aggregation is working (Loki)
- [ ] Service-to-service RPC communication verified

### host-b Verification
- [ ] All 6 core services operational
- [ ] WireGuard tunnel stable (no handshake timeouts)
- [ ] Telemetry agents sending data to host-a
- [ ] Remote metrics visible in host-a Prometheus
- [ ] Remote logs visible in host-a Loki

### Security Hardening
- [ ] LXD daemon is NOT listening on public interfaces
  ```bash
  netstat -tlnp | grep lxd  # Should only show [::]:8443 or 127.0.0.1
  ```
- [ ] Firewall rules configured for WireGuard (if applicable)
- [ ] SSH key-based auth only (no password auth)
- [ ] LXD trust password disabled (core.trust_password empty)

### Performance Optimization
- [ ] Verify CPU overcommit is acceptable:
  ```bash
  # host-a: max 51 CPU / 16 cores = 3.19x
  # host-b: max 10 CPU / 4 cores = 2.5x
  ```
- [ ] Memory utilization under 80%:
  ```bash
  free -h | grep Mem
  ```
- [ ] Disk I/O latency acceptable:
  ```bash
  iostat -x 1 2
  ```
- [ ] Network latency to remote host < 50ms:
  ```bash
  ping -c 5 host-b-ip  # Or vice versa
  ```

### Backup and Recovery
- [ ] Document LXD configuration backup procedure
- [ ] Test backup/restore of container snapshots
- [ ] Document ZFS snapshot strategy
- [ ] Plan disaster recovery (loss of host-a, host-b)

### Monitoring Setup
- [ ] Configure Grafana alert rules
- [ ] Set up log-based alerts in Loki
- [ ] Configure email/Slack notifications
- [ ] Create runbooks for common alerts

### Documentation
- [ ] Update team documentation with host IPs/credentials
- [ ] Document access procedures for dashboards
- [ ] Record WireGuard key exchange details (securely)
- [ ] Create runbook for scaling (adding more Outposts)

## Troubleshooting Commands

### LXD Status
```bash
lxd --version
lxc network show unheaded
lxc storage info unheaded-ssd
lxc profile show default
```

### Container Logs
```bash
lxc logs unheaded-SERVICENAME --follow
lxc exec unheaded-SERVICENAME -- journalctl -u unheaded-SERVICENAME -f
```

### Network Diagnostics
```bash
lxc exec unheaded-SERVICENAME -- ip addr show
lxc exec unheaded-SERVICENAME -- ping -c 3 10.20.1.1
lxc exec unheaded-SERVICENAME -- nslookup unheaded.internal
```

### Resource Usage
```bash
lxc exec unheaded-SERVICENAME -- free -h
lxc exec unheaded-SERVICENAME -- df -h
```

### Service Status
```bash
lxc exec unheaded-SERVICENAME -- systemctl status unheaded-SERVICENAME
lxc exec unheaded-SERVICENAME -- netstat -tlnp | grep SERVICENAME
```

## Success Criteria

### host-a Deployment Success
- [ ] 30 containers running (25 services + 5 telemetry)
- [ ] All services responding to health checks
- [ ] Prometheus collecting metrics from all targets
- [ ] Grafana dashboards populated and accessible
- [ ] Zero container restarts in first 5 minutes
- [ ] All inter-service communication functional

### host-b Deployment Success
- [ ] 9 containers running (6 core + 3 telemetry)
- [ ] WireGuard tunnel stable with < 50ms latency
- [ ] All local services responsive
- [ ] Telemetry agents successfully remote_write to host-a
- [ ] Host-a Prometheus shows host-b metrics as "UP"
- [ ] Host-a Loki receiving host-b logs

### Overall Deployment Success
- [ ] Cluster communication verified
- [ ] End-to-end service test passes
- [ ] All dashboards and UIs accessible
- [ ] Alerting configured and tested
- [ ] Backup/recovery procedures documented

