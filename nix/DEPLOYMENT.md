# Unheaded Container Deployment Guide

Complete guide to deploying the NixOS container stack for Unheaded Alpha.

## Prerequisites

### System Requirements
- **OS**: Debian 12+ or Ubuntu 22.04+ (bare metal or VM)
- **CPU**: 8+ cores (recommended: 16+)
- **RAM**: 16GB+ (recommended: 32GB+)
- **Disk**: 100GB+ SSD
- **Network**: Static IP or DHCP reservation

### Software Requirements
```bash
# Install Nix with flakes
curl -L https://nixos.org/nix/install | sh -s -- --daemon
echo "experimental-features = nix-command flakes" >> ~/.config/nix/nix.conf

# Install LXD
sudo snap install lxd
sudo lxd init --auto

# Install Go (for tests)
sudo apt install -y golang-1.21
```

## Quick Start

### 1. Clone Repository
```bash
git clone https://github.com/unheaded/unheaded.git
cd unheaded
```

### 2. Build All Containers
```bash
# Build container images
nix build \
  .#nixosConfigurations.busboy.config.system.build.toplevel \
  .#nixosConfigurations.timeguru.config.system.build.toplevel \
  .#nixosConfigurations.captain.config.system.build.toplevel \
  .#nixosConfigurations.micromanager.config.system.build.toplevel \
  .#nixosConfigurations.architect.config.system.build.toplevel \
  .#nixosConfigurations.developer.config.system.build.toplevel \
  .#nixosConfigurations.kanban.config.system.build.toplevel \
  .#nixosConfigurations.dashboard.config.system.build.toplevel
```

### 3. Deploy Containers
```bash
# Run deployment script
nix run .#deploy
```

### 4. Verify Deployment
```bash
# Check container status
lxc list

# Check health endpoints
curl http://10.10.10.10:8080/health  # busboy
curl http://10.10.10.20:8000/health  # timeguru
curl http://10.10.10.200:8080/health # kanban
```

## Manual Deployment

### Step 1: Network Setup
```bash
# Create bridge network
sudo ip link add lxdbr0 type bridge
sudo ip addr add 10.10.10.1/24 dev lxdbr0
sudo ip link set lxdbr0 up

# Enable IP forwarding
sudo sysctl -w net.ipv4.ip_forward=1
```

### Step 2: Build Container Images
```bash
# Build each container
for container in busboy timeguru captain micromanager architect developer kanban dashboard; do
  echo "Building $container..."
  nix build .#nixosConfigurations.$container.config.system.build.toplevel
done
```

### Step 3: Import to LXD
```bash
# Create LXD profile
lxc profile create unheaded

# Configure profile
lxc profile set unheaded \
  limits.cpu=4 \
  limits.memory=1GB \
  security.nesting=false \
  security.privileged=false

# Import container images
# TODO: Generate LXD-compatible images from Nix builds
```

### Step 4: Launch Containers (Dependency Order)
```bash
# 1. Busboy (no dependencies)
lxc launch unheaded-busboy busboy \
  --profile=unheaded \
  --config=boot.autostart=true

# Wait for busboy
sleep 10
curl http://10.10.10.10:8080/health

# 2. Services (depend on busboy)
for service in timeguru captain micromanager architect developer; do
  lxc launch unheaded-$service $service --profile=unheaded
done

# 3. Apps (depend on services)
lxc launch unheaded-kanban kanban --profile=unheaded
lxc launch unheaded-dashboard dashboard --profile=unheaded
```

### Step 5: Verify All Services
```bash
# Run health checks
./scripts/health-check-all.sh
```

## Container Management

### Start/Stop/Restart
```bash
# Start all
lxc start --all

# Stop all
lxc stop --all

# Restart specific container
lxc restart unheaded-busboy

# Stop in dependency-safe order
lxc stop kanban dashboard
lxc stop timeguru captain micromanager architect developer
lxc stop busboy
```

### Logs
```bash
# View container logs
lxc exec unheaded-busboy -- journalctl -u busboy -f

# View all logs
lxc exec unheaded-busboy -- tail -f /var/log/unheaded/busboy.json

# Aggregate logs from all containers
for c in busboy timeguru captain micromanager architect developer kanban dashboard; do
  echo "=== $c ==="
  lxc exec unheaded-$c -- tail -n 20 /var/log/unheaded/$c.json
done
```

### Shell Access
```bash
# Interactive shell
lxc exec unheaded-busboy -- bash

# Run command
lxc exec unheaded-timeguru -- curl http://10.10.10.10:8080/health

# Copy files
lxc file push local.txt unheaded-busboy/tmp/
lxc file pull unheaded-busboy/var/log/busboy.log ./
```

### Resource Monitoring
```bash
# CPU/Memory usage
lxc info unheaded-busboy

# Real-time stats
lxc monitor --type=logging

# Prometheus metrics
curl http://10.10.10.10:9100/metrics
```

## Network Configuration

### Container IP Allocation
| Container | IP | Port(s) | Access |
|-----------|-----|---------|--------|
| busboy | 10.10.10.10 | 9090, 8080, 9100 | Internal + Gateway |
| timeguru | 10.10.10.20 | 8000, 9100 | Internal + Gateway |
| captain | 10.10.10.21 | 8001, 9100 | Internal + Gateway |
| micromanager | 10.10.10.22 | 8002, 9100 | Internal + Gateway |
| architect | 10.10.10.23 | 8003, 9100 | Internal + Gateway |
| developer | 10.10.10.24 | 8004, 9100 | Internal + Gateway |
| kanban | 10.10.10.200 | 8080, 9100 | Public via Gateway |
| dashboard | 10.10.10.201 | 8081, 9100 | Public via Gateway |

### Firewall Rules
All containers have default DENY with explicit allows:
- Loopback: ALLOW
- Container network (10.10.10.0/24): ALLOW
- Established connections: ALLOW
- Everything else: DROP (logged)

### Testing Connectivity
```bash
# From host to busboy
curl http://10.10.10.10:8080/health

# From timeguru to busboy
lxc exec unheaded-timeguru -- curl http://10.10.10.10:8080/health

# From kanban to timeguru
lxc exec unheaded-kanban -- curl http://10.10.10.20:8000/api/v1/timeline
```

## Security

### Hardening Verification
```bash
# Check seccomp
lxc exec unheaded-busboy -- grep Seccomp /proc/1/status

# Check capabilities
lxc exec unheaded-busboy -- getpcaps 1

# Check filesystem
lxc exec unheaded-busboy -- mount | grep "on / "

# Check NoNewPrivileges
lxc exec unheaded-busboy -- grep NoNewPrivs /proc/1/status
```

### Security Audit
```bash
# Run automated security checks
nix run .#checks.${system}.security-audit

# Manual audit
for c in busboy timeguru captain micromanager architect developer kanban dashboard; do
  echo "=== Auditing $c ==="
  lxc exec unheaded-$c -- /etc/unheaded/security-audit.sh
done
```

## Troubleshooting

### Container Won't Start
```bash
# Check systemd status
lxc exec unheaded-busboy -- systemctl status busboy

# Check logs
lxc exec unheaded-busboy -- journalctl -u busboy -n 100

# Check dependencies
lxc exec unheaded-timeguru -- curl -v http://10.10.10.10:8080/health
```

### Network Issues
```bash
# Check bridge
ip addr show lxdbr0

# Check container networking
lxc exec unheaded-busboy -- ip addr
lxc exec unheaded-busboy -- ip route

# Check firewall
lxc exec unheaded-busboy -- iptables -L -v -n
```

### Performance Issues
```bash
# Check resource limits
systemctl show unheaded-busboy | grep -E '(Memory|CPU)'

# Check system load
lxc exec unheaded-busboy -- top

# Check disk I/O
lxc exec unheaded-busboy -- iostat -x 1
```

### Health Check Failures
```bash
# Test health endpoint
curl -v http://10.10.10.10:8080/health

# Check service status
lxc exec unheaded-busboy -- systemctl status busboy

# Check ports
lxc exec unheaded-busboy -- netstat -tlnp
```

## Backup and Recovery

### Backup
```bash
# Snapshot all containers
for c in busboy timeguru captain micromanager architect developer kanban dashboard; do
  lxc snapshot unheaded-$c backup-$(date +%Y%m%d-%H%M%S)
done

# Export snapshots
lxc export unheaded-busboy/backup-20260127-120000 busboy-backup.tar.gz
```

### Restore
```bash
# Restore from snapshot
lxc restore unheaded-busboy backup-20260127-120000

# Import backup
lxc import busboy-backup.tar.gz
```

## Updates and Maintenance

### Update Container Images
```bash
# Rebuild containers
nix build .#nixosConfigurations.busboy.config.system.build.toplevel

# Rolling update (zero downtime)
for c in timeguru captain micromanager architect developer; do
  echo "Updating $c..."
  lxc stop unheaded-$c
  # Re-deploy with new image
  lxc start unheaded-$c
  sleep 5
  curl http://10.10.10.20:8000/health # verify
done
```

### Rotate Logs
```bash
# Logs auto-rotate via journald, but manual cleanup:
lxc exec unheaded-busboy -- journalctl --vacuum-size=500M
```

### Clean Up
```bash
# Remove old snapshots
lxc delete unheaded-busboy/backup-20260101-000000

# Remove stopped containers
lxc delete --force $(lxc list -c n --format csv status=stopped)
```

## Production Checklist

Before going to production:

- [ ] All containers pass health checks
- [ ] Security audit passed (no findings)
- [ ] Network isolation verified
- [ ] Backup strategy in place
- [ ] Monitoring configured (Prometheus + Grafana)
- [ ] Log aggregation configured
- [ ] TLS certificates configured
- [ ] Resource limits tuned for load
- [ ] Load testing completed (1000+ req/s)
- [ ] Disaster recovery tested
- [ ] Runbook documented
- [ ] On-call rotation established

## Integration with Control Plane

Once containers are deployed, the `unheaded-daemon` control plane will:
1. Discover all containers via LXD API
2. Monitor health endpoints
3. Detect drift from desired state
4. Auto-remediate failures (restart, rebuild)
5. Collect metrics and logs
6. Report status to dashboard

See [cmd/unheaded-daemon/README.md](/cmd/unheaded-daemon/README.md) for control plane setup.

## Next Steps

- [ ] Deploy eBPF programs (see [ebpf/README.md](/ebpf/README.md))
- [ ] Configure gateway (see [gateway/README.md](/gateway/README.md))
- [ ] Set up dashboard (see [dashboard/README.md](/dashboard/README.md))
- [ ] Enable eBPF tracing (see [ARCHITECTURE.md](/docs/ARCHITECTURE.md))

## Resources

- [NixOS Manual](https://nixos.org/manual/)
- [LXD Documentation](https://linuxcontainers.org/lxd/docs/latest/)
- [CLAUDE.md](/CLAUDE.md) - Development standards
- [ARCHITECTURE.md](/docs/ARCHITECTURE.md) - System design
- [THE_META_MOMENT.md](/docs/THE_META_MOMENT.md) - Self-hosting philosophy

## Support

- **Issues**: https://github.com/unheaded/unheaded/issues
- **Discussions**: https://github.com/unheaded/unheaded/discussions
- **Email**: hello@unheaded.com
