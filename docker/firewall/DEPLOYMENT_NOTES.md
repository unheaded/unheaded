# Firewall Tier Deployment Notes

## Files Created

### Directory Structure
```
docker/firewall/
├── opnsense/
│   ├── Dockerfile         # QEMU-in-Docker with KVM
│   ├── entrypoint.sh      # Decompress image, setup TAP, boot QEMU
│   └── prepare-image.sh   # Image preparation helper
├── ipfire/
│   ├── Dockerfile         # QEMU-in-Docker with KVM
│   └── entrypoint.sh      # Decompress image, setup TAP, boot QEMU
├── frr/
│   └── Dockerfile         # Multi-stage build of FRR from ~/tmp/frr-master
├── bird/
│   └── Dockerfile         # Multi-stage build of BIRD from ~/tmp/bird-master
└── README.md              # Architecture, configuration, troubleshooting guide
```

### Updated Host Configurations

#### Host-A (Forge)
- `docker/hosts/host-a/docker-compose.yml`
  - Added `opnsense` service (QEMU 4096MB, 4 CPU)
  - Added `frr` service (BGP AS 65001)
  - Added `wan` macvlan network
  - Added firewall volumes: `opnsense-images`, `opnsense-data`, `frr-data`, `frr-run`
  - All existing services unchanged, inherit 10.20.0.1 gateway

- `docker/hosts/host-a/.env.example`
  - `WAN_INTERFACE`: Physical NIC (default: eno1)
  - `WAN_SUBNET`, `WAN_GATEWAY`: Upstream network
  - `OPNSENSE_WAN_IP`: macvlan IP (default: 192.168.1.2)
  - `OPNSENSE_MEM`, `OPNSENSE_CPUS`: Memory & CPU allocation
  - `OPNSENSE_IMG_PATH`: Path to OPNsense image
  - `FRR_CONF`, `FRR_BGP_AS`, `FRR_ROUTER_ID`: FRR configuration

#### Host-B (Outpost)
- `docker/hosts/host-b/docker-compose.yml`
  - Added `ipfire` service (QEMU 2048MB, 2 CPU)
  - Added `bird` service (BGP AS 65002)
  - Added `wan` macvlan network
  - Added firewall volumes: `ipfire-images`, `ipfire-data`, `bird-data`, `bird-run`
  - All existing services unchanged, inherit 10.20.0.1 gateway

- `docker/hosts/host-b/.env.example`
  - `WAN_INTERFACE`: Physical NIC (default: eno1)
  - `WAN_SUBNET`, `WAN_GATEWAY`: Upstream network
  - `IPFIRE_WAN_IP`: macvlan IP (default: 192.168.1.3)
  - `IPFIRE_MEM`, `IPFIRE_CPUS`: Memory & CPU allocation
  - `IPFIRE_IMG_PATH`: Path to IPFire image
  - `BIRD_CONF`, `BIRD_BGP_AS`, `BIRD_ROUTER_ID`: BIRD configuration

## Key Design Decisions

### 1. QEMU-in-Docker Pattern
Both firewalls (OPNsense & IPFire) are full OS images that require:
- Kernel + drivers
- Network interface management
- Hardware acceleration (KVM)

**Why QEMU-in-Docker?**
- Keeps container approach consistent across infrastructure
- Provides full OS isolation (better than raw container)
- Enables hardware acceleration via `--device /dev/kvm`
- Both FreeBSD (OPNsense) and Linux (IPFire) support this pattern
- Automatic image decompression on first boot

### 2. Network Architecture
```
┌─────────────────────────────────────────┐
│          Physical Host (eno1)           │
│         192.168.1.x/24 (WAN)            │
└───────────┬─────────────────────────────┘
            │
       macvlan bridge
            │
    ┌───────┴────────┐
    │                │
┌───▼────┐      ┌───▼────┐
│OPNsense│      │IPFire  │
│WAN:192.│      │WAN:192.│
│168.1.2 │      │168.1.3 │
└───┬────┘      └───┬────┘
    │                │
    │   docker bridge (172.20.0.0/16)
    │   
    ▼
┌─────────────────────────┐
│  All Service Containers │
│  Gateway: 10.20.0.1     │
│  (OPNsense/IPFire LAN)  │
└─────────────────────────┘
```

- **WAN Network**: macvlan, direct host interface access
  - OPNsense: 192.168.1.2
  - IPFire: 192.168.1.3
  - Upstream gateway: 192.168.1.1 (ISP router)

- **LAN Network**: docker bridge, isolated from WAN
  - OPNsense LAN: 10.20.0.1 (eth1 in QEMU)
  - IPFire LAN: 10.20.0.1 (eth1 in QEMU)
  - All services: 10.20.0.0/24 with gateway 10.20.0.1
  - No direct WAN access — must route through firewall

### 3. Routing Daemons
- **FRR (host-a)**: BGP AS 65001, announces 10.20.0.0/24 to upstream
- **BIRD (host-b)**: BGP AS 65002, backup route announcements
- Both run unprivileged with `NET_ADMIN`, `NET_RAW` capabilities
- Depend on firewall being healthy before starting

### 4. Image Handling
- OPNsense: `OPNsense-26.1.2-serial-amd64.img.bz2`
  - Decompressed with `pbzip2` (parallel bzip2)
  - ~5-10GB, one-time cost on first boot
  
- IPFire: `ipfire-2.29-core199-x86_64.img.xz`
  - Decompressed with `xz`
  - ~2-3GB, one-time cost on first boot

- Bind-mounted as read-only: `:/images/opnsense.img.bz2:ro`
- Persistent state in volumes: `/var/lib/opnsense`, `/var/lib/ipfire`

## Deployment Checklist

### Prerequisites
- [ ] `/dev/kvm` available on host (check: `ls -l /dev/kvm`)
- [ ] Physical NIC available for WAN (not virtualized)
- [ ] OPNsense image: `~/tmp/opnsense/OPNsense-26.1.2-serial-amd64.img.bz2`
- [ ] IPFire image: `~/tmp/ipfire/ipfire-2.29-core199-x86_64.img.xz`
- [ ] FRR source: `~/tmp/frr-master/` (for building)
- [ ] BIRD source: `~/tmp/bird-master/` (for building)

### Configuration
1. Copy `.env.example` to `.env` in each host directory:
   ```bash
   cd docker/hosts/host-a
   cp .env.example .env
   # Edit WAN_INTERFACE, WAN_SUBNET, etc.
   ```

2. Update `WAN_INTERFACE` to your physical NIC (e.g., eth0, eno1, enp0s25)

3. Verify upstream network:
   - WAN_SUBNET: must match your ISP network
   - WAN_GATEWAY: must be reachable from WAN_INTERFACE

### First Boot
1. **Build images**:
   ```bash
   cd docker/hosts/host-a
   docker-compose build opnsense frr
   cd ../host-b
   docker-compose build ipfire bird
   ```

2. **Start firewalls**:
   ```bash
   cd docker/hosts/host-a
   docker-compose up -d opnsense
   ```
   
   Monitor boot (1-2 minutes):
   ```bash
   docker logs -f unheaded-opnsense
   ```

3. **Initial OPNsense/IPFire Setup**:
   - Access serial console:
     ```bash
     docker attach unheaded-opnsense
     # (Ctrl-P Ctrl-Q to detach, not Ctrl-C)
     ```
   - Or via web UI (after system boots):
     - OPNsense: https://192.168.1.2 (from WAN), https://10.20.0.1 (internal)
     - IPFire: https://192.168.1.3 (from WAN), https://10.20.0.1:444 (internal)

4. **Configure FRR/BIRD**:
   - Ensure firewall is healthy:
     ```bash
     docker exec unheaded-frr vtysh -c "show bgp summary"
     docker exec unheaded-bird birdc "show route"
     ```

### Troubleshooting

**OPNsense stuck in boot**
```bash
docker logs unheaded-opnsense
# Check for KVM errors, increase OPNSENSE_MEM if needed
```

**Image decompression takes too long**
```bash
# Monitor progress
docker exec unheaded-opnsense ps aux | grep pbzip2
```

**macvlan network error**
```
error setting up network: initializing ipam driver
```
**Solution**: Ensure WAN_INTERFACE is a physical NIC, not docker0 or veth*

**FRR/BIRD not starting**
```bash
docker logs unheaded-frr
# Ensure opnsense is healthy first
docker ps | grep unheaded-opnsense
```

## Performance Tuning

### OPNsense
- Default: 4096 MB RAM, 4 CPUs (suitable for 50-100 Mbps traffic)
- For 1 Gbps+: increase to 8GB RAM, 8 CPUs
- Monitor in QEMU: `qemu-monitor` over socket

### IPFire
- Default: 2048 MB RAM, 2 CPUs (lightweight, 10-50 Mbps)
- For 100+ Mbps: increase to 4GB RAM, 4 CPUs

### KVM Optimization
- Enable nested KVM if available (check host CPU)
- Use virtio devices (enabled by default)
- Enable cache=writeback for disk (configured)

## Metrics & Monitoring

All firewall containers export Prometheus metrics:
- **OPNsense**: port 8080, `/metrics` (SNMP via OPNsense agent)
- **IPFire**: port 81, `/metrics` (if enabled)
- **FRR**: port 2601 (vtysh)
- **BIRD**: port 179 (BGP protocol)

Existing Prometheus instances should auto-scrape via service discovery.

## Future Enhancements

1. **High Availability**: Active/Standby failover between host-a and host-b
2. **IPsec Tunnel**: Encrypted WAN connection between firewalls
3. **VXLAN Tunneling**: For seamless multi-site deployments
4. **Anycast**: Same IP on both firewalls, clients route to nearest
5. **Policy Routing**: BGP-based traffic engineering

## Support

See `docker/firewall/README.md` for:
- Detailed architecture documentation
- Configuration examples
- Advanced troubleshooting
- Performance tuning guidelines

