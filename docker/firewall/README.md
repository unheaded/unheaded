# Firewall Tier: OPNsense + IPFire with FRR + BIRD Routing

This directory contains Docker configurations for the network firewall and routing tier that sits at the WAN boundary, handling all ingress/egress traffic for the Unheaded cluster.

## Architecture Overview

### Host-A (OPNsense + FRR)
- **OPNsense 26.1.2**: FreeBSD-based firewall running in QEMU-in-Docker (via KVM)
- **FRR (Free Routing Suite)**: BGP routing daemon, handles dynamic route announcements

### Host-B (IPFire + BIRD)
- **IPFire 2.29**: Linux-based firewall running in QEMU-in-Docker (via KVM)
- **BIRD**: BGP routing daemon, handles dynamic route announcements

## Why QEMU-in-Docker?

Both OPNsense (FreeBSD) and IPFire (Linux) use full-featured OS images that require:
- Full kernel + drivers
- Dedicated hardware interfaces
- Complex network configuration via system tools

### Strategy: QEMU-in-Docker
- Runs the full OS image inside a KVM-accelerated QEMU emulator
- Container handles: image decompression, TAP interface setup, QEMU process management
- Firewall OS manages: network policies, routing, traffic shaping, VPN
- Separates host resources from guest OS — better isolation than raw containers
- Consistent approach for both FreeBSD and Linux firewalls
- Access guest serial console via `docker logs` or `docker attach`

## Image Paths

User must provide the firewall images at:
- `~/tmp/opnsense/OPNsense-26.1.2-serial-amd64.img.bz2` (bzip2 compressed)
- `~/tmp/ipfire/ipfire-2.29-core199-x86_64.img.xz` (xz compressed)

These are bind-mounted into the containers as read-only volumes. The entrypoint scripts automatically decompress them on first boot.

## Network Configuration

### WAN Network (macvlan)
- Parent interface: configurable via `WAN_INTERFACE` (default: `eno1`)
- Subnet: configurable via `WAN_SUBNET` (default: `192.168.1.0/24`)
- Gateway: upstream ISP router (default: `192.168.1.1`)

**macvlan Requirements:**
- Must have physical NIC available (not a VM/container NIC)
- Each container gets a VLAN-tagged MAC on the physical network
- Allows firewall direct WAN access without bridge overhead

### LAN Network (docker bridge)
- Default gateway for all service containers: `10.20.0.1` (OPNsense/IPFire LAN IP)
- All services behind firewall — no direct WAN access
- Traffic flow: Service → Firewall LAN (10.20.0.1) → Firewall WAN → Internet

## Environment Variables

See `.env.example` in the respective host directory:
```env
WAN_INTERFACE=eno1
WAN_SUBNET=192.168.1.0/24
WAN_GATEWAY=192.168.1.1
OPNSENSE_WAN_IP=192.168.1.2
OPNSENSE_MEM=4096
OPNSENSE_CPUS=4
```

## Running the Firewalls

### Start Services
```bash
cd docker/hosts/host-a
docker-compose up -d opnsense frr
```

### Monitor Boot
OPNsense/IPFire boot takes 1-2 minutes:
```bash
docker logs -f unheaded-opnsense
docker logs -f unheaded-ipfire
```

### Access Serial Console (OPNsense/IPFire)
```bash
docker attach unheaded-opnsense
# or
docker exec -it unheaded-opnsense socat - UNIX-CONNECT:/run/opnsense/serial.sock
```

### Access Web UI
- OPNsense: https://10.20.0.1:443 (or 192.168.1.2 from WAN)
- IPFire: https://10.20.0.1:444 (or 192.168.1.3 from WAN)

### Check FRR/BIRD Status
```bash
# FRR
docker exec unheaded-frr vtysh -c "show bgp summary"

# BIRD
docker exec unheaded-bird birdc "show route"
```

## Container Capabilities & Security

Both firewalls require:
- `--privileged` flag (for KVM access)
- `--device /dev/kvm` (for hardware acceleration)
- `NET_ADMIN`, `NET_RAW`, `SYS_ADMIN` capabilities
- macvlan network access for WAN

Routing daemons (FRR/BIRD):
- Run unprivileged (no `--privileged`)
- Have `NET_ADMIN`, `NET_RAW`, `SYS_ADMIN` capabilities
- Access firewall via docker bridge network

## Health Checks

All containers include health checks:
- **OPNsense**: Checks port 443 reachable on LAN (10.20.0.1)
- **IPFire**: Checks port 444 reachable on LAN (10.20.0.1)
- **FRR**: Checks BGP summary via vtysh
- **BIRD**: Checks route table via birdc

## Volumes

### OPNsense
- `/images`: Mounted firewall image (bind-mount, RO)
- `/var/lib/opnsense`: Persistent firewall config

### IPFire
- `/images`: Mounted firewall image (bind-mount, RO)
- `/var/lib/ipfire`: Persistent firewall config

### FRR
- `/etc/frr`: Config (bind-mount, RO)
- `/var/run/frr`: Runtime data

### BIRD
- `/etc/bird`: Config (bind-mount, RO)
- `/var/run`: Runtime data

## Known Limitations

1. **No GUI from Docker Exec**: QEMU runs headless (no X11/VNC)
   - Use serial console for initial setup
   - Use web UI (HTTPS) for admin panel

2. **Image Decompression**: First boot decompresses the image (~5-10GB)
   - Uses pbzip2 (parallel bzip2) and xz for speed
   - One-time cost, subsequent boots skip decompression

3. **TAP Interface Setup**: Requires root privileges on host
   - Docker entrypoint creates tap0/tap1 bridges
   - TAP interfaces cleaned up on container exit

4. **KVM Availability**: Requires `/dev/kvm` on host
   - Check: `ls -l /dev/kvm`
   - If not available, QEMU falls back to emulation (very slow)

## Troubleshooting

### Image Not Found
```
[opnsense] ERROR: No image found at /images/opnsense.img or /images/opnsense.img.bz2
```
**Solution**: Bind-mount the image from `~/tmp/opnsense/` in docker-compose.yml

### macvlan Network Error
```
error setting up network: initializing ipam driver: failed to retrieve network driver
```
**Solution**: Ensure `WAN_INTERFACE` is a physical NIC, not a virtual one

### OPNsense Stuck in Boot
**Solution**: Check QEMU logs: `docker logs unheaded-opnsense`
- May need more RAM: increase `OPNSENSE_MEM`
- May need more CPUs: increase `OPNSENSE_CPUS`

### FRR/BIRD Not Connecting
**Solution**: Verify firewall is healthy:
```bash
docker exec unheaded-opnsense ping 10.20.0.2  # should reach FRR
```

## Configuration Files

- `opnsense/Dockerfile`: Builds QEMU+OPNsense container
- `opnsense/entrypoint.sh`: Decompresses image, sets up TAP, boots QEMU
- `ipfire/Dockerfile`: Builds QEMU+IPFire container
- `ipfire/entrypoint.sh`: Decompresses image, sets up TAP, boots QEMU
- `frr/Dockerfile`: Builds FRR from ~/tmp/frr-master source
- `bird/Dockerfile`: Builds BIRD from ~/tmp/bird-master source

FRR and BIRD config files (bind-mounted):
- `routing/frr/frr.conf`: BGP/ISIS/BFD config
- `routing/frr/daemons`: Which FRR daemons to enable
- `routing/bird/bird.conf`: BIRD BGP/OSPF config

## Performance Tuning

### OPNsense Memory/CPU
Defaults: 4096 MB RAM, 4 CPUs
```yaml
environment:
  OPNSENSE_MEM: "8192"     # Increase for heavy traffic
  OPNSENSE_CPUS: "8"
```

### IPFire Memory/CPU
Defaults: 2048 MB RAM, 2 CPUs
```yaml
environment:
  IPFIRE_MEM: "4096"       # Increase for more concurrent connections
  IPFIRE_CPUS: "4"
```

### QEMU Optimization
- Enable KVM (`-enable-kvm`) for near-native performance
- Use virtio devices for network (not e1000)
- Use virtio disk (cache=writeback)

## Related Files

- `docker-compose.yml`: Service definitions in host-a/host-b
- `.env.example`: Environment variable templates
- `routing/frr/`: FRR routing daemon
- `routing/bird/`: BIRD routing daemon

