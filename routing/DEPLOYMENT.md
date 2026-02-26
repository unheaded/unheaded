# Unheaded Kingdom — Quick Deployment Guide

SPDX-License-Identifier: MIT

## Pre-Deployment Checklist

- [ ] WireGuard tunnel established between host-a and host-b
- [ ] WireGuard IP addresses configured:
  - host-a: fd00:dead:beef::1/48
  - host-b: fd00:dead:beef::2/48
- [ ] Network connectivity test:
  ```bash
  ping6 fd00:dead:beef::2  # From host-a
  ping6 fd00:dead:beef::1  # From host-b
  ```
- [ ] Service bridges created:
  - br-unheaded on both hosts
  - IPv4: 10.20.0.254/16
  - IPv6: fd00:dead:beef:1::/64 (host-a), fd00:dead:beef:2::/64 (host-b)

---

## Host-A (Forge — OPNsense + FRR)

### Installation

```bash
# 1. SSH into host-a OPNsense
ssh root@[opnsense-ip]

# 2. Install FRR package via OPNsense
# GUI: System > Firmware > Plugins > search "os-frr" > Install

# 3. Copy configuration files
scp routing/frr/* root@[opnsense-ip]:/tmp/
ssh root@[opnsense-ip]
mkdir -p /etc/frr
cp /tmp/frr.conf /etc/frr/
cp /tmp/daemons /etc/frr/
cp /tmp/vtysh.conf /etc/frr/
mkdir -p /opt/frr
cp /tmp/setup-vtep.sh /opt/frr/
chmod +x /opt/frr/setup-vtep.sh
```

### Configure Systemd Service

Edit `/etc/systemd/system/frr.service` to add ExecStartPre:

```ini
[Unit]
Description=FRR - Free Range Routing Suite
After=network.target

[Service]
Type=notify
ExecStartPre=/opt/frr/setup-vtep.sh
ExecStart=/usr/lib/frr/frr start
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### Start FRR

```bash
systemctl daemon-reload
systemctl enable frr
systemctl start frr
systemctl status frr
```

### Verify

```bash
vtysh
# Inside vtysh:
show bgp summary
show bgp neighbors fd00:dead:beef::2
show isis neighbors
show bfd peers
exit
```

---

## Host-B (Outpost — IPFire + BIRD)

### Installation

```bash
# 1. SSH into host-b IPFire
ssh root@[ipfire-ip]

# 2. Build BIRD from source (if not pre-installed)
cd /tmp
git clone https://git.nic.cz/bird/bird.git bird-master
cd bird-master
autoreconf -fi
./configure \
  --prefix=/usr \
  --sysconfdir=/etc/bird \
  --localstatedir=/var/run \
  --enable-ipv6 \
  --with-protocols=bgp,ospf,bfd,static,direct,kernel,device,radv
make -j4 && make install

# 3. Create BIRD user and directories
adduser -system -group bird
mkdir -p /var/run/bird /var/log/bird
chown bird:bird /var/run/bird /var/log/bird
```

### Copy Configuration

```bash
scp routing/bird/bird.conf root@[ipfire-ip]:/tmp/
ssh root@[ipfire-ip]
cp /tmp/bird.conf /etc/bird/bird.conf
chown bird:bird /etc/bird/bird.conf
chmod 640 /etc/bird/bird.conf
```

### Install Systemd Service

```bash
cat > /etc/systemd/system/bird.service << 'EOL'
[Unit]
Description=BIRD Internet Routing Daemon
After=network.target

[Service]
Type=simple
User=bird
Group=bird
ExecStart=/usr/sbin/bird -f -c /etc/bird/bird.conf
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOL

systemctl daemon-reload
systemctl enable bird
systemctl start bird
systemctl status bird
```

### Verify

```bash
birdc show protocols
birdc show protocols forge
birdc show route protocol forge
```

---

## Integration Testing

### Test 1: BGP Session Establishment

**On host-a:**
```bash
vtysh
show bgp neighbors fd00:dead:beef::2
# Expect: State = Established, Hold time countdown
```

**On host-b:**
```bash
birdc show protocols forge
# Expect: BGP    forge    up
```

### Test 2: Route Advertisement

**On host-a:**
```bash
vtysh
show bgp ipv4 unicast summary
show bgp ipv6 unicast summary
# Expect: Routes to 10.20.0.0/16 and fd00:dead:beef:1::/64
```

**On host-b:**
```bash
birdc show route where net ~ 10.20.0.0/16
# Expect: 10.20.0.0/16 via fd00:dead:beef::1 [forge] 0.0.0.0
```

### Test 3: BFD Fast-Failover

**Start continuous ping from host-a container:**
```bash
docker exec [container] ping6 fd00:dead:beef:2::100
# Should see responses every ~100ms
```

**From host-b, bring down WireGuard:**
```bash
ip link set wg0 down
# Ping stops after ~900ms (BFD detect time)
# BGP session drops
```

**Restore WireGuard:**
```bash
ip link set wg0 up
# Ping resumes within ~300ms (BFD re-establish)
```

### Test 4: VXLAN VTEP Verification

**On host-a:**
```bash
ip -d link show vxlan10001
# Expect: type vxlan id 10001 local 10.20.255.1 dev br-unheaded nolearning
```

**Check FRR knows about VXLAN:**
```bash
vtysh
show interface vxlan10001
show evpn vni
```

---

## Troubleshooting

### BGP not establishing

**Symptoms**: `show bgp neighbors` shows state = Connect / Active

**Causes**:
1. WireGuard tunnel is down
   ```bash
   ip link show wg0  # Should be UP
   ping6 fd00:dead:beef::2
   ```

2. Firewall blocking BGP (port 179 or 16384+ for ephemeral ports)
   ```bash
   # Allow BGP on OPNsense firewall rules
   # Or check IPFire firewall
   ```

3. FRR daemon not running
   ```bash
   systemctl status frr
   tail -f /var/log/frr/bgpd.log
   ```

4. BGP router-id or update-source misconfigured
   ```bash
   vtysh
   show bgp config | grep router-id
   show bgp neighbors fd00:dead:beef::2 | grep "Source"
   ```

### VXLAN not forwarding

**Symptoms**: Containers can't reach across hosts

**Causes**:
1. VXLAN interfaces not created
   ```bash
   ip link show | grep vxlan
   # Run setup-vtep.sh manually if missing
   ```

2. EVPN not advertising routes
   ```bash
   vtysh
   show bgp l2vpn evpn route  # Should see type-2 and type-5 routes
   ```

3. Bridge membership wrong
   ```bash
   brctl show br-vxlan10001  # vxlan10001 should be listed
   ```

### BFD not detecting failures

**Symptoms**: WireGuard goes down, but BGP takes 90s to drop

**Causes**:
1. BFD not enabled on FRR neighbor
   ```bash
   vtysh
   show bfd peers  # Should show session to fd00:dead:beef::2
   ```

2. BFD not enabled on BIRD
   ```bash
   birdc show bfd sessions  # Should show session to fd00:dead:beef::1
   ```

3. BFD session in Down state
   ```bash
   # Check WireGuard link is up
   ip link show wg0
   # Ensure timers are reasonable (300ms)
   ```

---

## File Locations Reference

| Component | File | Host | Location |
|-----------|------|------|----------|
| FRR daemon config | frr.conf | host-a | /etc/frr/frr.conf |
| FRR daemons | daemons | host-a | /etc/frr/daemons |
| Vtysh shell | vtysh.conf | host-a | /etc/frr/vtysh.conf |
| VTEP setup | setup-vtep.sh | host-a | /opt/frr/setup-vtep.sh |
| FRR logs | bgpd.log, isisd.log | host-a | /var/log/frr/ |
| BIRD daemon config | bird.conf | host-b | /etc/bird/bird.conf |
| BIRD logs | bird.log | host-b | /var/log/bird/ |

---

## Performance Expectations

- **BGP convergence time**: < 1 second (with graceful-restart)
- **BFD detection time**: < 1 second (300ms intervals × 3)
- **EVPN MAC learning**: < 100ms (BGP advertisement overhead)
- **Throughput**: Line rate (limited by WireGuard MTU 1380 → VXLAN 50 byte overhead)

---

## Next Steps

1. **Enable per-VRF routing** (if multi-tenancy needed):
   - Add more VNIs in frr.conf
   - Configure VRF-to-VNI mapping in zebra

2. **Implement monitoring**:
   - Export BIRD/FRR metrics to Prometheus
   - Alert on BFD/BGP session loss

3. **Scale to N hosts**:
   - Replace point-to-point WireGuard with full-mesh
   - Use BGP route reflector if > 4 peers
   - Migrate IS-IS to multi-area topology

4. **Test failover scenarios**:
   - WireGuard link flap
   - Host reboot
   - BGP configuration reload (no restart)

---

For detailed architecture, see **README.md** in this directory.
