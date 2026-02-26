# Unheaded Kingdom Routing Configuration — File Manifest

SPDX-License-Identifier: MIT

## Overview

Complete FRR (host-a Forge) and BIRD (host-b Outpost) routing configurations for a two-host BGP EVPN-VXLAN fabric with Monad IPv6 HbH header support.

**Total files**: 11  
**Total lines of code**: ~1200  
**Architecture**: IS-IS underlay + iBGP EVPN overlay + VXLAN data plane

---

## Directory Structure

```
routing/
├── README.md                    (Architecture & protocol documentation)
├── DEPLOYMENT.md                (Quick-start deployment guide)
├── MANIFEST.md                  (This file)
│
├── frr/                         (FRR daemon configuration for host-a Forge)
│   ├── frr.conf                 (Main FRR routing configuration)
│   ├── daemons                  (FRR daemon startup controls)
│   ├── vtysh.conf               (Vtysh shell configuration)
│   ├── setup-vtep.sh            (VXLAN VTEP creation script)
│   └── Dockerfile               (Docker image build for FRR from source)
│
└── bird/                        (BIRD daemon configuration for host-b Outpost)
    ├── bird.conf                (Main BIRD routing configuration)
    ├── bird-check.sh            (Connectivity verification script)
    ├── bird-env                 (BIRD environment variables)
    └── Dockerfile               (Docker image build for BIRD from source)
```

---

## File Details

### README.md

**Size**: ~8 KB  
**Purpose**: Comprehensive architecture documentation

**Covers**:
- Two-tier routing (IS-IS underlay + BGP EVPN overlay)
- Physical and logical topology diagrams
- IPv4/IPv6 addressing plan
- Protocol stack detail (IS-IS, BGP, BFD, VXLAN)
- VXLAN VTEP lifecycle management
- Monad HbH header passthrough guarantees
- Deployment steps for OPNsense and IPFire
- Troubleshooting commands
- Performance limits and references

**Read this first** for understanding the complete system.

---

### DEPLOYMENT.md

**Size**: ~3 KB  
**Purpose**: Quick-start deployment and integration testing

**Covers**:
- Pre-deployment checklist
- Step-by-step FRR installation (OPNsense)
- Step-by-step BIRD installation (IPFire)
- Systemd service configuration
- Integration test cases (4 tests)
- Troubleshooting quick-fix guide
- File locations reference
- Performance expectations

**Use this** for hands-on deployment after reading README.

---

### MANIFEST.md

**Size**: This file  
**Purpose**: Index of all files, line counts, and responsibilities

**Contains**:
- Directory tree
- File-by-file documentation
- Responsibility matrix (which component uses which file)
- Version information

---

## FRR Configuration (host-a Forge — OPNsense)

### frr.conf

**Size**: ~4.8 KB  
**Lines**: ~150  
**Language**: FRR configuration syntax

**Sections**:

1. **Preamble** (~5 lines)
   - Version declaration (FRR 10.0+)
   - Hostname: forge
   - Logging configuration

2. **Interface definitions** (~20 lines)
   - lo: Loopback (10.20.255.1/32, fd00:dead:beef:ff::1/128)
   - eno1: WAN (DHCP + IPv6 RA)
   - br-unheaded: LAN bridge (10.20.0.254/16, fd00:dead:beef:1::fe/64)
   - wg0: WireGuard (fd00:dead:beef::1/48)

3. **IS-IS routing** (~20 lines)
   - NET: 49.0001.1020.0255.0001.00
   - Metric style: wide (24-bit, RFC 5305)
   - Segment Routing: SRGB 16000-23999
   - Interface bindings (lo passive, wg0 p2p metric 10)

4. **BGP EVPN** (~60 lines)
   - ASN: 65001
   - Router-ID: 10.20.255.1
   - iBGP neighbor: fd00:dead:beef::2 (host-b, AS65002)
   - Address families:
     - ipv4 unicast (10.20.0.0/16)
     - ipv6 unicast (fd00:dead:beef:1::/64)
     - l2vpn evpn (VNI 10001, 10002, 10100)
   - BFD coupling
   - Graceful restart

5. **BFD configuration** (~10 lines)
   - Peer: fd00:dead:beef::2
   - Timers: 300ms detect, multiplier 3
   - Interface: wg0

6. **Route-maps and prefix lists** (~10 lines)
   - PERMIT-ALL (HbH-transparent)
   - IPv4/IPv6 prefix lists for service subnets

7. **VXLAN VTEP notes** (~5 lines)
   - Zebra netlink management
   - Kernel integration via setup-vtep.sh

**Key features**:
- Full VXLAN EVPN with per-VNI RD/RT
- Monad HbH passthrough (no HbH stripping)
- IS-IS segment routing ready
- BFD <1s detection
- Graceful restart for zero-loss reconfig

---

### daemons

**Size**: ~330 bytes  
**Lines**: ~20  
**Format**: FRR daemons control file

**Enabled daemons**:
- `zebra=yes` (kernel interface)
- `bgpd=yes` (BGP daemon)
- `isisd=yes` (IS-IS daemon)
- `bfdd=yes` (BFD daemon)
- `staticd=yes` (static routes)
- `mgmtd=yes` (management daemon)

**Disabled daemons**:
- OSPF (ospfd, ospf6d) — IS-IS is the underlay
- RIP (ripd, ripngd) — legacy, not needed
- PIM, LDP, NHRP, etc. — not in scope

---

### vtysh.conf

**Size**: ~178 bytes  
**Lines**: ~5  
**Format**: Vtysh shell configuration

**Purpose**:
- Enable integrated-vtysh-config (merge daemons into single config file)
- Allow root user passwordless access to vtysh

---

### setup-vtep.sh

**Size**: ~1.1 KB  
**Lines**: ~50  
**Language**: Bash

**Purpose**: Create VXLAN VTEP interfaces before FRR daemon starts

**Actions**:
1. Loop through VNIs: 10001, 10002, 10100
2. For each VNI:
   - Create VXLAN interface: `ip link add vxlan10001 type vxlan id 10001 ...`
   - Create per-VNI bridge: `ip link add br-vxlan10001 type bridge`
   - Attach VXLAN to bridge: `ip link set vxlan10001 master br-vxlan10001`

**Integration**:
- Run as ExecStartPre in frr.service systemd unit
- Must run with root privileges
- Idempotent (checks if interface exists before creating)

**Execution permission**: `chmod +x` (executable)

---

### Dockerfile (FRR)

**Size**: ~2.3 KB  
**Language**: Dockerfile

**Stages**:

1. **Builder stage** (ubuntu:24.04):
   - Install build tools, development headers
   - Copy FRR source (assumed at /src/frr or bind-mount)
   - ./bootstrap.sh && ./configure && make -j$(nproc) && make install

2. **Runtime stage** (ubuntu:24.04):
   - Install runtime libraries only
   - Create frr user and group
   - Copy binaries from builder
   - Copy configuration files (frr.conf, daemons, vtysh.conf)
   - Expose ports: 2601-2610 (FRR vtysh daemon ports)

**Build command**:
```bash
docker build -t frr:unheaded \
  --build-arg=FRR_REPO=https://github.com/FRRouting/frr.git \
  -f routing/frr/Dockerfile .
```

**Run command**:
```bash
docker run -d --name frr-forge \
  --net=host \
  -v /etc/frr:/etc/frr:ro \
  frr:unheaded
```

---

## BIRD Configuration (host-b Outpost — IPFire)

### bird.conf

**Size**: ~4.0 KB  
**Lines**: ~140  
**Language**: BIRD configuration syntax

**Sections**:

1. **Global settings** (~5 lines)
   - Log syslog + stderr
   - Router-ID: 10.20.255.2

2. **Device & Direct protocols** (~10 lines)
   - Scan interfaces every 10 seconds
   - Direct protocol for interface IPs
   - Interfaces: br-unheaded, wg0, lo

3. **Kernel protocol** (~20 lines)
   - IPv4 kernel sync (export all, import all)
   - IPv6 kernel sync
   - Learn from kernel (pick up added routes)
   - Merge paths on (ECMP support)

4. **BFD protocol** (~10 lines)
   - Interface: wg0
   - Timers: 300ms min-rx/tx, multiplier 3
   - Fast failover integration

5. **Static routes** (~10 lines)
   - IPv4: 10.20.0.0/16, 10.20.255.2/32
   - IPv6: fd00:dead:beef:2::/64, fd00:dead:beef:ff::2/128

6. **BGP session** (~50 lines)
   - Name: forge (iBGP to host-a)
   - Local: fd00:dead:beef::2 AS65002
   - Neighbor: fd00:dead:beef::1 AS65001
   - BFD graceful coupling
   - Hold time: 90s, keepalive: 30s
   - Address families:
     - ipv4 unicast (with export filter)
     - ipv6 unicast (with export filter)
     - l2vpn evpn (VXLAN overlay routes)
   - Add paths rx (ECMP)

7. **OSPF v3** (~15 lines)
   - Area 0.0.0.0
   - Interface: br-unheaded (broadcast)
   - Loopback: stub (no hellos)
   - IPv6 export filter

8. **Router Advertisement (radv)** (~20 lines)
   - Advertises fd00:dead:beef:2::/64 to containers
   - Managed=no (SLAAC only, no DHCPv6)
   - Other config=yes (allows DHCPv6 for DNS if needed)
   - RDNSS: fd00:dead:beef:2::1
   - DNSSL: unheaded.internal
   - Lifetime: 3600s, preferred: 1800s

**Key features**:
- HbH-transparent BGP (routes pass HbH as-is)
- BFD <1s detection to host-a
- Automatic kernel route sync
- IPv6 RA for container SLAAC
- OSPF for intra-host routing

---

### bird-check.sh

**Size**: ~559 bytes  
**Lines**: ~12  
**Language**: Bash

**Purpose**: Quick connectivity verification for BIRD

**Commands**:
1. `birdc show protocols all` — all protocols status
2. `birdc show protocols forge` — BGP session detail
3. `birdc show route protocol forge` — routes learned from host-a
4. `birdc show route where net ~ fd00:dead:beef::/32` — IPv6 routes
5. `birdc show bfd sessions` — BFD session status
6. `ip -6 route show | grep dead:beef` — kernel routes

**Usage**:
```bash
/opt/bird/bird-check.sh
# or
bash routing/bird/bird-check.sh
```

**Output**: Real-time status of BGP session, routes, BFD state

**Execution permission**: `chmod +x` (executable)

---

### bird-env

**Size**: ~400 bytes  
**Lines**: ~12  
**Format**: Shell environment variables

**Variables**:
- `BIRD_OPTS="-c /etc/bird/bird.conf"` — daemon startup options
- `BIRD_LOG_LEVEL="info"` — logging level
- `BIRD_LOG_FILE="/var/log/bird/bird.log"` — log output
- `BIRD_PIDFILE="/var/run/bird/bird.pid"` — PID file location
- `ENABLE_IPV6="yes"` — IPv6 support

**Usage**: Source in systemd service or init.d script

---

### Dockerfile (BIRD)

**Size**: ~950 bytes  
**Language**: Dockerfile

**Stages**:

1. **Builder stage** (alpine:3.19):
   - Lightweight build environment
   - Install autoconf, flex, bison, readline-dev
   - Copy BIRD source (assumed at /src/bird)
   - autoreconf -fi && ./configure && make -j$(nproc)

2. **Runtime stage** (alpine:3.19):
   - Minimal base image (~5 MB)
   - Install libssh, readline runtime libs
   - Create bird user/group
   - Copy binaries from builder
   - Copy bird.conf
   - Expose port 179 (BGP)

**Build command**:
```bash
docker build -t bird:unheaded \
  -f routing/bird/Dockerfile .
```

**Run command**:
```bash
docker run -d --name bird-outpost \
  --net=host \
  -v /etc/bird:/etc/bird:ro \
  bird:unheaded
```

---

## Responsibility Matrix

| Component | File | Host | Responsibility |
|-----------|------|------|-----------------|
| BGP EVPN | frr.conf | host-a | iBGP peering, VNI advertisements, EVPN routes |
| IS-IS | frr.conf | host-a | Underlay routing, segment routing |
| BFD (FRR) | frr.conf | host-a | Fast failure detection, 300ms timers |
| VXLAN VTEP | setup-vtep.sh | host-a | Create VXLAN interfaces before FRR starts |
| FRR startup | daemons | host-a | Control which daemons (bgpd, isisd, bfdd, zebra) |
| BGP peer (BIRD) | bird.conf | host-b | iBGP to host-a, EVPN import, route export |
| BFD (BIRD) | bird.conf | host-b | Fast failure detection, 300ms timers |
| IPv6 RA | bird.conf | host-b | Advertise fd00:dead:beef:2::/64 to containers |
| OSPF | bird.conf | host-b | Optional intra-host routing |
| Verification | bird-check.sh | host-b | Test BGP/EVPN/BFD/kernel routes |
| Docker (FRR) | Dockerfile | — | Build FRR from source |
| Docker (BIRD) | Dockerfile | — | Build BIRD from source |

---

## Key Features Summary

| Feature | Supported | Implementation |
|---------|-----------|-----------------|
| **BGP EVPN** | Yes | Type-2 (MAC/IP), Type-5 (IP), VNI RD/RT isolation |
| **VXLAN** | Yes | 3 VNIs (10001, 10002, 10100), 4789/UDP |
| **IS-IS** | Yes (FRR) | L2-only, wide metrics, SR-enabled |
| **IPv6** | Yes | fd00:dead:beef::/32 supernet, dual-stack |
| **Monad HbH** | Yes (passthrough) | No stripping, transparent routing |
| **BFD** | Yes (both) | 300ms detect-multiplier 3 (~900ms detection) |
| **Multi-path** | Yes | ECMP, maximum-paths 4 (FRR), add paths rx (BIRD) |
| **Graceful restart** | Yes (FRR) | Suppresses route flaps on peer reboot |
| **Segment routing** | Yes (FRR) | SRGB 16000-23999, node-msd 8 |
| **Static routes** | Yes | Loopbacks, service subnets |
| **Router advertisement** | Yes (BIRD) | SLAAC for containers |

---

## Version Compatibility

| Software | Version | Notes |
|----------|---------|-------|
| FRR | 10.0+ | Tested with 10.x release cycle |
| BIRD | 2.x+ | Tested with 2.14+ for EVPN support |
| OPNsense | 24.x+ | Plugin: os-frr |
| IPFire | 2.27+ | Manual build or package install |
| Kernel | 5.x+ | VXLAN support, IPv6 extensions |
| WireGuard | 1.0+ | IPv6 tunnel, MTU 1380 recommended |

---

## Configuration Size & Complexity

| File | Type | Size | Lines | Complexity |
|------|------|------|-------|------------|
| frr.conf | config | 4.8 KB | ~150 | High (routing logic) |
| daemons | config | 330 B | ~20 | Low (list) |
| vtysh.conf | config | 178 B | ~5 | Trivial |
| setup-vtep.sh | script | 1.1 KB | ~50 | Medium (bash loops) |
| bird.conf | config | 4.0 KB | ~140 | High (protocols + filters) |
| bird-env | config | 400 B | ~12 | Low (vars) |
| README.md | docs | 8 KB | ~250 | High (detailed) |
| DEPLOYMENT.md | docs | 3 KB | ~100 | Medium (procedures) |
| Dockerfile (FRR) | build | 2.3 KB | ~80 | Medium (multi-stage) |
| Dockerfile (BIRD) | build | 950 B | ~45 | Low (simple build) |

**Total**: ~30 KB code, ~1200 lines (config + scripts + docs)

---

## Testing Procedures

### Minimal test (5 minutes)
1. Verify WireGuard tunnel up: `ping6 fd00:dead:beef::2`
2. Check BGP established: `vtysh` → `show bgp neighbors`
3. Verify VXLAN: `ip link show | grep vxlan`
4. Check BIRD: `birdc show protocols forge`

### Full test (20 minutes)
1. All above
2. Test east-west VXLAN: container on host-a ping container on host-b
3. Simulate BFD failure: `ip link set wg0 down`, watch BGP drop within 900ms
4. Restore and verify recovery: `ip link set wg0 up`, BGP back within 300ms
5. Check EVPN routes: `vtysh` → `show bgp l2vpn evpn route`

### Stress test (optional)
1. Generate traffic across VXLAN (iperf3)
2. Monitor CPU/memory: `top` on both hosts
3. Check packet loss with MTU 1380: `ping -s 1330 -c 1000`

---

## Maintenance Notes

### Regular checks
- Monthly: Review BGP peer statistics, check for neighbor flaps
- Quarterly: Audit ACLs/route-maps for unused rules
- Yearly: Review segment routing SID allocation

### Upgrade path
1. FRR: Minor versions are compatible, major versions may require vtysh migration
2. BIRD: Update bird.conf syntax to match release notes
3. Test in staging before production

### Backup
```bash
# On host-a
tar czf frr-backup-$(date +%Y%m%d).tar.gz /etc/frr/

# On host-b
tar czf bird-backup-$(date +%Y%m%d).tar.gz /etc/bird/
```

---

## Support & References

- **FRR docs**: https://docs.frrouting.org/
- **BIRD docs**: https://bird.network.cz/
- **RFC 5308**: IS-IS for IPv6
- **RFC 7432**: BGP MPLS Ethernet VPN (EVPN)
- **RFC 7348**: VXLAN

---

**Last Updated**: 2026-02-26  
**License**: MIT  
**Maintained by**: Unheaded Kingdom team
