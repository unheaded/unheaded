# Unheaded Kingdom Routing Configuration — Quick Index

**Base directory**: `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/routing/`

## Start Here

1. **README.md** (24 KB, 731 lines) — Full architecture documentation
   - Complete protocol stack explanation
   - Monad HbH passthrough guarantees
   - Troubleshooting guide

2. **DEPLOYMENT.md** (7.2 KB, 358 lines) — Quick deployment steps
   - FRR installation (OPNsense)
   - BIRD installation (IPFire)
   - Integration tests
   - Troubleshooting quick-fix

3. **MANIFEST.md** (15 KB, 524 lines) — File reference & matrix
   - Each file's purpose and content
   - Responsibility assignments
   - Version compatibility

## Host-A Configuration (Forge — OPNsense + FRR)

```
frr/
├── frr.conf ..................... Main routing config (201 lines)
│                               - BGP EVPN (VNI 10001, 10002, 10100)
│                               - IS-IS underlay (SR-enabled)
│                               - BFD 300ms detection
│
├── daemons ....................... Daemon startup control (24 lines)
│                               - bgpd, isisd, bfdd, zebra, staticd, mgmtd
│
├── vtysh.conf .................... Vtysh shell config (6 lines)
│                               - Integrated config, root access
│
├── setup-vtep.sh ................. VXLAN setup script (41 lines, executable)
│                               - Create vxlan10001-10100 interfaces
│                               - Run before FRR starts
│
└── Dockerfile .................... Build FRR from source (71 lines)
                                - Multi-stage Ubuntu 24.04 build
                                - Ports: 2601-2610
```

## Host-B Configuration (Outpost — IPFire + BIRD)

```
bird/
├── bird.conf ..................... Main routing config (184 lines)
│                               - BGP to host-a (AS65002 ← AS65001)
│                               - BFD 300ms detection
│                               - OSPF v3 local routing
│                               - Router Advertisement for IPv6 SLAAC
│
├── bird-env ....................... Environment variables (16 lines)
│                               - Log levels, PID/log file paths
│
├── bird-check.sh .................. Verification script (15 lines, executable)
│                               - Check BGP session, EVPN routes, BFD
│
└── Dockerfile ..................... Build BIRD from source (38 lines)
                                - Multi-stage Alpine 3.19 build
                                - Port: 179 (BGP)
```

## Documentation Files

```
├── README.md ...................... Architecture & protocols (731 lines)
├── DEPLOYMENT.md .................. Setup & testing guide (358 lines)
├── MANIFEST.md .................... File reference matrix (524 lines)
└── INDEX.md ....................... This file
```

---

## File Count Summary

| Category | Count | Total Size |
|----------|-------|-----------|
| FRR configs | 4 | 5.3 KB |
| FRR scripts | 1 | 1.1 KB |
| FRR Dockerfile | 1 | 2.3 KB |
| BIRD configs | 2 | 4.4 KB |
| BIRD scripts | 1 | 0.6 KB |
| BIRD Dockerfile | 1 | 950 B |
| Documentation | 4 | 55 KB |
| **TOTAL** | **14** | **71 KB** |

---

## Execution Permissions

Scripts with `chmod +x` (executable):
- `frr/setup-vtep.sh` — Must run before FRR starts
- `bird/bird-check.sh` — Run anytime to verify connectivity

---

## Key Characteristics

| Feature | Value |
|---------|-------|
| **Architecture** | Two-tier: IS-IS underlay + BGP EVPN overlay |
| **Hosts** | 2 (host-a Forge, host-b Outpost) |
| **BGP ASNs** | 65001 (host-a), 65002 (host-b) |
| **VXLAN VNIs** | 10001, 10002, 10100 |
| **BFD Detection** | 900ms (300ms × 3) |
| **IPv6 Supernet** | fd00:dead:beef::/32 |
| **WireGuard MTU** | 1380 bytes (VXLAN-safe) |
| **Protocol Lines** | ~200 (FRR) + 184 (BIRD) = 384 |
| **Monad HbH** | Supported (transparent passthrough) |

---

## Quick Start (5 minutes)

### Host-A (OPNsense)
```bash
# Install FRR package, copy files, start service
scp routing/frr/* root@[opnsense-ip]:/tmp/
ssh root@[opnsense-ip] "cp /tmp/*.conf /etc/frr/ && \
  cp /tmp/setup-vtep.sh /opt/frr/ && chmod +x /opt/frr/setup-vtep.sh && \
  systemctl start frr"

# Verify
vtysh
show bgp neighbors fd00:dead:beef::2
```

### Host-B (IPFire)
```bash
# Install BIRD, copy config, start service
scp routing/bird/bird.conf root@[ipfire-ip]:/etc/bird/
ssh root@[ipfire-ip] "systemctl start bird"

# Verify
birdc show protocols forge
```

---

## Architecture at a Glance

```
┌────────────────────────────────────────────────────────────┐
│                BGP EVPN Overlay (iBGP)                     │
│  AS65001 (host-a) ←→ AS65002 (host-b) over fd00:dead:beef::/48
│                                                             │
│  Advertises: VNI RD/RT, Type-2 MAC/IP, Type-5 IP routes   │
└────────────────────────────────────────────────────────────┘
                          ↓
┌────────────────────────────────────────────────────────────┐
│              VXLAN Data Plane (UDP 4789)                    │
│    VTEP 10.20.255.1 (host-a) ↔ 10.20.255.2 (host-b)       │
│    VNI: 10001, 10002, 10100 (service isolation)            │
└────────────────────────────────────────────────────────────┘
                          ↓
┌────────────────────────────────────────────────────────────┐
│           IS-IS Underlay (host-a only)                      │
│  RFC 5308 IPv6, Segment Routing SRGB 16000-23999           │
└────────────────────────────────────────────────────────────┘
```

---

## Deployment Path

```
1. Read README.md (understand architecture)
              ↓
2. Review DEPLOYMENT.md (follow step-by-step)
              ↓
3. Install FRR on host-a (use frr/* files)
              ↓
4. Install BIRD on host-b (use bird/* files)
              ↓
5. Test BGP session: vtysh / birdc
              ↓
6. Run bird-check.sh for full verification
              ↓
7. Test VXLAN with containers (east-west ping)
              ↓
8. Simulate BFD failover (ip link set wg0 down)
              ↓
9. Production ready!
```

---

## File Locations (After Deployment)

| Component | File | Deployed To |
|-----------|------|-------------|
| FRR config | frr.conf | `/etc/frr/frr.conf` |
| FRR daemons | daemons | `/etc/frr/daemons` |
| Vtysh | vtysh.conf | `/etc/frr/vtysh.conf` |
| VTEP setup | setup-vtep.sh | `/opt/frr/setup-vtep.sh` |
| BIRD config | bird.conf | `/etc/bird/bird.conf` |
| BIRD env | bird-env | `/etc/bird/bird-env` |

---

## Verification Commands

### Host-A (vtysh)
```
show bgp neighbors fd00:dead:beef::2
show bgp l2vpn evpn route
show isis neighbors
show bfd peers
show evpn vni
show vxlan vtep
```

### Host-B (birdc)
```
show protocols forge
show route protocol forge
show bfd sessions
show route where net ~ fd00:dead:beef::/32
```

---

## Support

- **Architecture questions**: See README.md
- **Deployment issues**: See DEPLOYMENT.md
- **File details**: See MANIFEST.md
- **Protocol specs**: RFC 5308 (IS-IS), RFC 7432 (EVPN), RFC 7348 (VXLAN)

---

**License**: MIT  
**Created**: 2026-02-26  
**Base directory**: `/sessions/great-dreamy-ptolemy/mnt/tmp/unheaded/routing/`
