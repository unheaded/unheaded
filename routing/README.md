# Unheaded Kingdom — Two-Host BGP EVPN-VXLAN Routing Fabric

SPDX-License-Identifier: MIT

## Executive Summary

The Unheaded Kingdom fabric implements a **two-tier routing architecture** across two hosts:

- **host-a (Forge)**: OPNsense + FRR — EVPN VTEP, IS-IS underlay, full routing suite (AS65001)
- **host-b (Outpost)**: IPFire + BIRD — lightweight BGP peer, BFD fast-failover (AS65002)

Both hosts are peered over **WireGuard IPv6 iBGP** (fd00:dead:beef::/48) with BFD-accelerated convergence (300ms). The overlay **VXLAN** fabric (VNI 10000-19999) provides service isolation and multi-tenancy.

**Critical feature**: IPv6 extension headers (Monad HbH 20-byte registers) pass through FRR and BIRD transparently at the L3 forwarding layer. No HbH stripping occurs.

---

## Architecture Overview

### Physical Topology

```
┌─────────────────────────────────────────────────────────────────┐
│  WAN (Upstream routing, DHCP, IPv6 RA)                          │
│  eno1 (host-a) / wan0 (host-b)                                  │
└────────────────────┬────────────────────────────────────────────┘
                     │
        ┌────────────┴────────────┐
        │                         │
   ┌────▼─────┐           ┌──────▼────┐
   │  host-a  │           │  host-b   │
   │  Forge   │           │  Outpost  │
   │ OPNsense │           │  IPFire   │
   └────┬─────┘           └──────┬────┘
        │                         │
   FRR: 65001               BIRD: 65002
   iBGP EVPN                BGP peer
   IS-IS underlay           BFD monitor
        │                         │
        └────────────WireGuard────┘
            fd00:dead:beef::/48
            iBGP peer link (MTU 1380)
            BFD 300ms detection

     ┌─────────────────────────────────┐
     │  Service Bridge & Containers    │
     │  host-a: fd00:dead:beef:1::/64  │
     │  host-b: fd00:dead:beef:2::/64  │
     │  IPv4: 10.20.0.0/16 (dual-stack)│
     │                                 │
     │  VXLAN VTEP VNIs:              │
     │  - 10001: Service VNI-1         │
     │  - 10002: Service VNI-2         │
     │  - 10100: Telemetry            │
     └─────────────────────────────────┘
```

### Routing Stack (Three Layers)

```
┌──────────────────────────────────────────────────────────┐
│ LAYER 3: BGP EVPN Overlay (iBGP between hosts)           │
│          Address-families: ipv4, ipv6, l2vpn evpn        │
│          Router-IDs: 10.20.255.1 (Forge), .2 (Outpost)   │
│          Fast convergence: BFD 300ms + graceful-restart  │
└──────────────────────────────────────────────────────────┘
            │
            └─ FRR (host-a): advertises EVPN routes + VRF RDs
            └─ BIRD (host-b): imports EVPN, exports local nets
                
┌──────────────────────────────────────────────────────────┐
│ LAYER 2: VXLAN Data Plane (MAC/IP learning)              │
│          VTEPs: 10.20.255.1 (Forge), 10.20.255.2 (Outpost)│
│          UDP port 4789, MTU 1380 over WireGuard           │
│          Per-VNI bridges: br-vxlan10001, br-vxlan10002    │
└──────────────────────────────────────────────────────────┘
            │
            └─ Fabric-wide bridge learning + flood suppression
            └─ EVPN provides MAC/IP mobility across hosts

┌──────────────────────────────────────────────────────────┐
│ LAYER 1: IS-IS Underlay (FRR host-a only)                │
│          Protocol: IS-IS L2 over IPv6 (RFC 5308)          │
│          Segment Routing: SID block 16000-23999           │
│          Future: SRv6 or MPLS L3VPN                       │
└──────────────────────────────────────────────────────────┘
            │
            └─ FRR interfaces: wg0 (point-to-point)
```

---

## Addressing Plan

### IPv4

| Network | Purpose | Host-A | Host-B |
|---------|---------|--------|--------|
| 10.20.0.0/16 | Service bridge (containers) | 10.20.0.254/16 | 10.20.0.254/16 |
| 10.20.255.0/24 | Loopback/VTEP anchors | 10.20.255.1 | 10.20.255.2 |
| 10.20.255.1/32 | VTEP IP (host-a) | BGP RID | — |
| 10.20.255.2/32 | VTEP IP (host-b) | — | BGP RID |

### IPv6

| Network | Purpose | Host-A | Host-B |
|---------|---------|--------|--------|
| fd00:dead:beef::/32 | Unheaded Kingdom supernet | — | — |
| fd00:dead:beef::/48 | WireGuard iBGP tunnel | 1/48 | 2/48 |
| fd00:dead:beef:ff::1/128 | Loopback (host-a) | BGP source | — |
| fd00:dead:beef:ff::2/128 | Loopback (host-b) | — | BGP RID |
| fd00:dead:beef:1::/64 | Service bridge (host-a) | fe/64 | (reached via EVPN) |
| fd00:dead:beef:2::/64 | Service bridge (host-b) | (reached via EVPN) | fe/64 |

**Monad HbH consideration**: All IPv6 addresses support 20-byte HbH extension headers injected in container traffic. These headers are NOT stripped by FRR or BIRD — they pass through the routing table transparently.

---

## Protocol Stack Detail

### IS-IS Underlay (FRR host-a)

**File**: `routing/frr/frr.conf` (router isis UNHEADED block)

- **NET**: 49.0001.1020.0255.0001.00 (L2 routing)
- **Metric style**: Wide (32-bit, RFC 5305)
- **Type**: L2-only (area-less backbone, suitable for 2-host fabric)
- **Segment Routing**:
  - Global Block (SRGB): 16000-23999
  - Node-MSB: 8 (reserve for node-local SIDs)
  - Future: SRv6 path computation + MPLS steering

**Interfaces**:
- `lo`: Passive (loopback, does not flood IS-IS hellos)
- `wg0`: Point-to-point (WireGuard to host-b)
  - Metric: 10 (low cost, preferred over WAN)
  - Network type: P2P (suppresses LAN hello)

**Redistribution**:
- Connected IPv4 → IS-IS L2
- Connected IPv6 → IS-IS L2
- Allows loopback (10.20.255.0/24) and WireGuard (fd00:dead:beef::/48) to flood as underlay routes

---

### BGP EVPN Overlay (FRR host-a ↔ BIRD host-b)

#### FRR Configuration (host-a Forge)

**Peer details**:
- Neighbor: fd00:dead:beef::2 (host-b wg0 address)
- Remote AS: 65002 (host-b)
- Local AS: 65001 (host-a)
- **iBGP**: same AS path, full transparency of routes

**BGP Features**:
- Update-source: loopback (10.20.255.1) — ensures consistency if WireGuard address changes
- BFD: 300ms detect-multiplier 3 (failover ~900ms)
- Graceful restart: enabled (suppresses route flaps on peer reboot)
- Multipath: relax AS-path validation (host-a can prefer multiple paths)

**Address families**:

1. **IPv4 Unicast**:
   - Networks advertised by host-a:
     - 10.20.0.0/16 (service bridge)
   - Imported from host-b:
     - Host-b's local subnets (received and re-exported)
   - Maximum paths: 4 (ECMP for multi-path load balancing)

2. **IPv6 Unicast**:
   - Networks advertised by host-a:
     - fd00:dead:beef:1::/64 (service bridge IPv6)
   - Imported from host-b:
     - fd00:dead:beef:2::/64 (received via EVPN)

3. **L2VPN EVPN**:
   - **advertise-all-vni**: Host-a signals all locally-configured VNIs to host-b
   - **advertise-default-gw**: Sends EVPN type-2 (IP+MAC) routes for default gateway
   - **advertise-svi-ip**: Advertises SVI (switched virtual interface) IPs for inter-VNI routing
   - **Attribute-unchanged next-hop**: Prevents path hunting; VTEP IP remains constant

**VXLAN VNI-to-RD mapping** (host-a):

| VNI | RD (Route Distinguisher) | Route Target | Purpose |
|-----|-------------------------|--------------|---------|
| 10001 | 65001:10001 | 65001:10001 | Service VNI-1 |
| 10002 | 65001:10002 | 65001:10002 | Service VNI-2 |
| 10100 | 65001:10100 | 65001:10100 | Telemetry |

Each VNI can independently import/export routes based on RT filters.

#### BIRD Configuration (host-b Outpost)

**BGP session** (protocol bgp forge):
- Neighbor: fd00:dead:beef::1 (host-a wg0 address)
- Remote AS: 65001 (host-a)
- Local AS: 65002 (host-b)

**BFD integration**:
- Protocol: `bfd graceful` (converges faster than BGP hold timers)
- Timers: 300ms min-rx/tx, multiplier 3 (900ms total detection)

**Address families**:

1. **IPv4**:
   - Import: all routes from host-a
   - Export filter:
     ```
     if net ~ 10.20.0.0/16 then accept;       # Service bridge
     if net ~ 10.20.255.0/24 then accept;     # Loopbacks
     reject;
     ```
   - Next hop self: enabled (host-b rewrites next-hop to its own address)
   - Add paths rx: receive multiple paths for ECMP

2. **IPv6**:
   - Import: all routes from host-a
   - Export filter:
     ```
     if net ~ fd00:dead:beef::/32 then accept;
     reject;
     ```
   - Next hop self: enabled

3. **L2VPN EVPN** (host-b):
   - Import all: receives EVPN routes from host-a (type-2, type-5)
   - Export all: sends any locally-originated EVPN routes back to host-a
   - **Monad HbH**: BIRD does not inspect HbH headers in EVPN type-5 routes; transparent passthrough

---

### BFD Fast-Failover (Both hosts)

**FRR configuration** (routing/frr/frr.conf):
```
bfd
 peer fd00:dead:beef::2 interface wg0
  detect-multiplier 3
  receive-interval 300
  transmit-interval 300
```

**BIRD configuration** (routing/bird/bird.conf):
```
protocol bfd {
    interface "wg0" {
        min rx interval 300ms;
        min tx interval 300ms;
        multiplier 3;
    };
}
```

**Behavior**:
- Bidirectional detection: each host sends BFD packets every 300ms to peer
- Failure detection: if 3 consecutive BFD packets are missed (900ms total), session is down
- Coupling: BGP session is immediately reset (no 90-second hold timer wait)
- Use case: WireGuard tunnel flap, network congestion, or peer reboot

---

### Router Advertisement (BIRD host-b only)

**Protocol**: radv (IPv6 Router Advertisement)

**Configuration** (routing/bird/bird.conf):
```
protocol radv {
    interface "br-unheaded" {
        prefix fd00:dead:beef:2::/64 {
            autonomous yes;
            on link yes;
            valid lifetime 3600;
            preferred lifetime 1800;
        };
        rdnss fd00:dead:beef:2::1;    # Recursive DNS Server
        dnssl "unheaded.internal";     # DNS Search List
    };
}
```

**Purpose**:
- Container interfaces on host-b automatically receive `fd00:dead:beef:2::/64` via SLAAC
- Containers can form Monad HbH-enabled IPv6 addresses within the /64
- DNS: containers use host-b's DNS resolver (10.20.0.254 or fd00:dead:beef:2::1)

---

## VXLAN VTEP Setup

### Overview

VXLAN VTEPs are **data-plane L2 tunnels** that encapsulate Ethernet frames in UDP/IP. FRR (via zebra) manages the VTEP interfaces; EVPN BGP signals which MAC addresses live on which VNI.

### Setup Script (host-a)

**File**: `routing/frr/setup-vtep.sh`

**Called before FRR startup** (systemd ExecStartPre hook):
```
ExecStartPre=/opt/frr/setup-vtep.sh
ExecStart=/usr/lib/frr/frr start
```

**What it does**:
1. For each VNI (10001, 10002, 10100):
   - Creates VXLAN interface: `ip link add vxlan10001 type vxlan id 10001 local 10.20.255.1 dstport 4789`
   - Creates per-VNI bridge: `ip link add br-vxlan10001 type bridge`
   - Attaches VXLAN to bridge: `ip link set vxlan10001 master br-vxlan10001`

2. Flags:
   - `nolearning`: disables dynamic MAC learning on the VXLAN interface (EVPN controls learning)
   - `local 10.20.255.1`: source IP for VXLAN tunnel (the VTEP IP)
   - `dstport 4789`: standard VXLAN UDP port

**Persistence**: This script runs once at boot. If you restart FRR without rebooting, the VTEPs remain up and FRR reattaches.

### VTEP Lifecycle (FRR → Zebra → Kernel)

```
User modifies /etc/frr/frr.conf
                  │
                  ▼
    FRR zebra daemon reads vtysh updates
                  │
                  ▼
    Zebra netlink API sends VXLAN config to kernel
                  │
                  ▼
    Kernel creates VXLAN interface (if not exists)
                  │
                  ▼
    FRR EVPN module advertises VNI to peers (BGP)
                  │
                  ▼
    Peer learns MAC/IP routes for that VNI (EVPN type-2)
```

### Example: Adding a new VNI

1. **Add to FRR config** (routing/frr/frr.conf):
   ```
   vni 10003
    rd 65001:10003
    route-target import 65001:10003
    route-target export 65001:10003
   ```

2. **Add to setup-vtep.sh** (routing/frr/setup-vtep.sh):
   ```bash
   VNIS=(10001 10002 10003 10100)  # Add 10003
   ```

3. **Restart FRR**:
   ```bash
   systemctl restart frr
   ```

---

## Monad HbH Extension Header Passthrough

### What is Monad HbH?

Monad is a hypothetical IPv6 extension header (next-header 0) that carries a 20-byte register payload after the base IPv6 header. Containers may inject HbH headers into their traffic for metadata transport (e.g., service tracing, hardware tags).

### FRR Behavior

**FRR does NOT strip HbH headers**. Here's why:

1. **FRR routing is prefix-based**: BGP/IS-IS route lookups key on destination IP prefix, not on extension headers.
2. **HbH headers are transparent to L3 forwarding**: The kernel routing table handles routing based on the IPv6 destination address alone.
3. **No ACL/route-map filtering on next-header**: FRR's route-maps (`route-map PERMIT-ALL`) ignore L4 details. HbH passes through.

### BIRD Behavior

**BIRD also does NOT strip HbH headers**. Static routes and BGP-learned routes carry HbH transparently.

### Kernel Routing Table

When FRR or BIRD installs a route like:
```
fd00:dead:beef:2::/64 via fd00:dead:beef::2 dev wg0
```

The kernel routes **any IPv6 packet** with destination in `fd00:dead:beef:2::/64` to `wg0` — **including those with HbH headers**. The HbH header is not inspected or modified.

### Example: Container with Monad HbH

```
Container sends traffic:
  IPv6 src: fd00:dead:beef:2::100
  IPv6 dst: fd00:dead:beef:1::200
  Next-header: 0 (HbH)
  HbH payload: 20-byte Monad register
               │
               └─ [metadata: trace ID, service tag, etc.]

FRR BGP lookup:
  destination = fd00:dead:beef:1::200
  match route: fd00:dead:beef:1::/64 via fd00:dead:beef::1
  action: forward to fd00:dead:beef::1

Kernel xfrm / IPv6 output:
  Packet forwarded with HbH header INTACT
  No stripping, no modification
```

### ACL/Route-Map Guidelines for Monad

If you later add **prefix lists** or **access lists** to FRR:

```
ipv6 prefix-list MONAD-SAFE seq 5 permit fd00:dead:beef::/32 le 128
```

This list matches **prefixes only** — HbH next-header is irrelevant. Safe to use.

If you add route-maps with **community filters** or **as-path checks**:

```
route-map MONAD-AWARE permit 10
 match ip next-hop 10.20.255.1
 set community 65001:1000
```

This also ignores HbH. Safe.

**Never add**:
```
ipv6 access-list HBHBAN seq 10 deny ipv6 any any nexthdr 0
match ipv6 next-header 0
```

These would drop HbH traffic, which we don't want.

---

## Deployment & Integration

### Host-A (OPNsense + FRR)

#### OPNsense GUI Integration

1. **Access OPNsense Admin Console**:
   - Navigate to `https://opnsense-host-a:8443`
   - Login with admin credentials

2. **Install FRR Package**:
   - Go to **System > Firmware > Plugins**
   - Search for and install `os-frr`
   - This sets up `/etc/frr/` directory and systemd service

3. **Upload Configuration Files**:
   - Copy `routing/frr/frr.conf` → `/etc/frr/frr.conf`
   - Copy `routing/frr/daemons` → `/etc/frr/daemons`
   - Copy `routing/frr/vtysh.conf` → `/etc/frr/vtysh.conf`
   - Copy `routing/frr/setup-vtep.sh` → `/opt/frr/setup-vtep.sh`
   - `chmod +x /opt/frr/setup-vtep.sh`

4. **Enable FRR Service**:
   ```bash
   systemctl enable frr
   systemctl start frr
   ```

5. **Verify FRR is Running**:
   ```bash
   vtysh
   # Inside vtysh:
   show bgp summary
   show bgp neighbors fd00:dead:beef::2
   show isis neighbors
   ```

#### Network Configuration (OPNsense)

**Interfaces**:
- `eno1`: WAN (DHCP client + IPv6 RA)
- `br-unheaded`: LAN bridge (10.20.0.254/16, fd00:dead:beef:1::fe/64)
- `wg0`: WireGuard peer-to-peer (fd00:dead:beef::1/48)

**WireGuard Tunnel**:
```
Interface: wg0
Private key: [generated]
Listen port: 51820
Peer (host-b):
  Endpoint: [host-b-wan-ip]:51820
  Public key: [host-b-public-key]
  Allowed IPs: fd00:dead:beef::2/128
```

---

### Host-B (IPFire + BIRD)

#### Manual BIRD Installation (if not pre-packaged)

1. **Download & compile BIRD**:
   ```bash
   cd /tmp
   git clone https://git.nic.cz/bird/bird.git bird-master
   cd bird-master
   autoreconf -fi
   ./configure --prefix=/usr --sysconfdir=/etc/bird \
     --enable-ipv6 \
     --with-protocols=bgp,ospf,bfd,static,direct,kernel,device,radv
   make -j4 && sudo make install
   ```

2. **Create BIRD User**:
   ```bash
   sudo adduser -system -group bird
   sudo mkdir -p /var/run/bird
   sudo chown bird:bird /var/run/bird
   ```

3. **Install Configuration**:
   ```bash
   sudo cp routing/bird/bird.conf /etc/bird/bird.conf
   sudo chown bird:bird /etc/bird/bird.conf
   sudo chmod 640 /etc/bird/bird.conf
   ```

4. **Create Systemd Service** (IPFire):
   ```
   File: /etc/systemd/system/bird.service
   
   [Unit]
   Description=BIRD Internet Routing Daemon
   After=network.target
   
   [Service]
   Type=simple
   User=bird
   Group=bird
   ExecStartPre=/usr/sbin/bird -c /etc/bird/bird.conf -d
   ExecStart=/usr/sbin/bird -f -c /etc/bird/bird.conf
   ExecReload=/bin/kill -HUP $MAINPID
   Restart=on-failure
   RestartSec=5
   
   [Install]
   WantedBy=multi-user.target
   ```

5. **Enable & Start**:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable bird
   sudo systemctl start bird
   ```

#### Network Configuration (IPFire)

**Interfaces**:
- `wan0`: WAN (DHCP client + IPv6 RA)
- `br-unheaded`: LAN bridge (10.20.0.254/16, fd00:dead:beef:2::fe/64)
- `wg0`: WireGuard peer-to-peer (fd00:dead:beef::2/48)

---

## Troubleshooting & Verification

### Check FRR (host-a)

```bash
# Vtysh shell
vtysh

# See all protocols
show protocols

# BGP EVPN neighbors
show bgp neighbors fd00:dead:beef::2

# EVPN routes learned
show bgp l2vpn evpn summary
show bgp l2vpn evpn route

# IS-IS status
show isis neighbors

# BFD sessions
show bfd peers

# Check that WireGuard is in IS-IS
show isis interface wg0
```

### Check BIRD (host-b)

```bash
# BIRD client
birdc

# Protocols
show protocols

# BGP session to host-a
show protocols forge
show route protocol forge

# EVPN routes (if supported by BIRD version)
show route where net ~ fd00:dead:beef::/32

# BFD status
show bfd sessions
```

### Verify VXLAN VTEPs (host-a)

```bash
# List VXLAN interfaces
ip -d link show vxlan10001

# Check bridge membership
brctl show br-vxlan10001

# Monitor VXLAN traffic (optional tcpdump)
tcpdump -i wg0 'udp port 4789'
```

### Test East-West Connectivity

```
# From host-a container to host-b container
ping6 fd00:dead:beef:2::100

# Check route
ip -6 route get fd00:dead:beef:2::100

# Should show: via fd00:dead:beef::2 (EVPN-learned route)
```

### BFD Failover Test

```bash
# On host-a
(ping fd00:dead:beef::2 in background)

# On host-b: bring down WireGuard
ip link set wg0 down

# Observe:
# - BFD detects loss within 900ms
# - BGP session drops
# - FRR withdraws routes to host-b
# - Ping stops after detection

# Bring WireGuard back up
ip link set wg0 up

# BFD re-establishes within 300ms
# BGP routes re-advertised
# Ping resumes
```

---

## Files Reference

| File | Host | Purpose |
|------|------|---------|
| `routing/frr/frr.conf` | host-a | Main FRR daemon config (BGP, IS-IS, BFD) |
| `routing/frr/daemons` | host-a | FRR startup controls (enables bgpd, isisd, bfdd, etc.) |
| `routing/frr/vtysh.conf` | host-a | Vtysh shell configuration |
| `routing/frr/setup-vtep.sh` | host-a | Creates VXLAN VTEPs pre-boot |
| `routing/frr/Dockerfile` | — | Docker image for FRR (build from source) |
| `routing/bird/bird.conf` | host-b | Main BIRD daemon config (BGP, BFD, radv) |
| `routing/bird/bird-check.sh` | host-b | Connectivity verification script |
| `routing/bird/Dockerfile` | — | Docker image for BIRD (build from source) |
| `routing/README.md` | — | This file |

---

## Performance & Limits

| Parameter | Value | Notes |
|-----------|-------|-------|
| BGP ECMP paths | 4 | `maximum-paths 4` (FRR) |
| BFD detect time | 900ms | 300ms interval × 3 multiplier |
| BGP hold time | 90s | `hold time 90` (BIRD) |
| IS-IS metric range | 0-16777215 | 24-bit wide metric |
| SRGB size | 8000 SIDs | Range 16000-23999 |
| Segment Routing node-MSB | 8 | Reserve for local SIDs |
| VXLAN VNI range | 10000-19999 | Configured RDs use 65001:VVVVV |
| MTU (WireGuard) | 1380 | VXLAN encap overhead ~50 bytes |

---

## Future Enhancements

1. **Segment Routing v6 (SRv6)**:
   - Deploy segment routing headers (next-header 43)
   - Use SID list for service chain steering
   - Compatible with Monad HbH (both pass through transparently)

2. **MPLS L3VPN**:
   - Extend IS-IS to advertise MPLS LSPs
   - Create VRF overlays for tenant isolation

3. **Multi-Area IS-IS**:
   - If fabric grows beyond 2 hosts, add L1-L2 area hierarchy

4. **BGP RR (Route Reflector)**:
   - If > 3 BGP peers, use a route reflector to reduce iBGP mesh

5. **Telemetry (gRPC + Protobuf)**:
   - Leverage FRR's gRPC interface for programmatic route control
   - BIRD's dynamic protocol module for automation

---

## License

All configuration files and scripts are licensed under the MIT License.
See individual file headers for details.

---

## References

- [FRR Documentation](https://docs.frrouting.org/)
- [BIRD Project](https://bird.network.cz/)
- [RFC 5308 — Routing IPv6 with IS-IS](https://tools.ietf.org/html/rfc5308)
- [RFC 7432 — BGP MPLS-Based Ethernet VPN](https://tools.ietf.org/html/rfc7432)
- [VXLAN RFC 7348](https://tools.ietf.org/html/rfc7348)
- [IPv6 Extension Headers (RFC 8200)](https://tools.ietf.org/html/rfc8200)
