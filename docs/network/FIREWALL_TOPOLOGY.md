# Network Topology and Firewall Integration Architecture

## 1. Overview

The Unheaded Kingdom project employs a dual-firewall architecture where each host maintains its own stateful firewall appliance at the ingress/egress boundary. This design ensures that:

- **OPNsense (BSD 2-Clause)** guards **HOST-A (The Forge)** with FreeBSD's pf firewall, providing enterprise-grade packet filtering, NAT, and intrusion detection via Suricata IDS.
- **IPFire (GPL v3)** guards **HOST-B (The Outpost)** with Linux nftables, providing similar capabilities via Snort IDS and squid proxy services.

Both firewalls:
- Act as the **first and last hop** for all traffic entering/leaving each node's container network
- Run as VMs (libvirt/QEMU on NixOS, privileged containers on Docker, or LXD VMs)
- Bridge the physical WAN interface via macvlan to maintain complete network isolation
- Provide SIEM-compatible logging for SOC2/PCI-DSS/HIPAA compliance
- **Pass Monad Protocol Hop-by-Hop (HbH) extension headers** without stripping or rewriting them
- Implement default-deny security posture with explicit allow rules per service

---

## 2. Full ASCII Topology Diagram

```
HOST-A (FORGE)                              HOST-B (OUTPOST)
─────────────────────────────────           ──────────────────────────────
 Physical NIC (eno1/eth0)                    Physical NIC (eno1/eth0)
        │ WAN                                       │ WAN
        ▼                                           ▼
 ┌─────────────────┐                        ┌─────────────────┐
 │   OPNsense VM   │  BSD 2-Clause          │   IPFire VM     │  GPL v3
 │  WAN: macvlan   │                        │  WAN: macvlan   │
 │  LAN: 10.20.0.1 │                        │  LAN: 10.20.0.1 │
 │                 │                        │                 │
 │ • pf firewall   │                        │ • nftables      │
 │ • Suricata IDS  │                        │ • Snort IDS     │
 │ • HAProxy SSL   │                        │ • squid proxy   │
 │ • HbH passthru  │                        │ • HbH passthru  │
 └────────┬────────┘                        └────────┬────────┘
          │ LAN (10.20.0.1/16)                       │ LAN (10.20.0.1/16)
          ▼                                           ▼
 ┌──────────────────────┐              ┌──────────────────────┐
 │  unheaded bridge     │              │  unheaded bridge     │
 │  10.20.0.0/16        │              │  10.20.0.0/16        │
 │  fd00:dead:beef:1::/64│             │  fd00:dead:beef:2::/64│
 └──────────────────────┘              └──────────────────────┘
          │                                           │
  ┌───────┴────────┐                         ┌───────┴───────┐
  │ 25 services    │                         │ 6 core svcs   │
  │ + telemetry    │                         │ + telemetry   │
  └────────────────┘                         └───────────────┘
          │                                           │
          │                                           │
          └─────────── WireGuard VPN ─────────────────┘
              fd00:dead:beef::1 ↔ fd00:dead:beef::2
              UDP 51820, MTU=1380
              (kernel interface, bypasses firewall NAT)
```

### Firewall VM Placement

Each firewall sits **between the physical NIC and container bridge**:

```
Physical WAN NIC
      │
      ├─ macvlan interface → Firewall VM WAN port
      │
      └─ Direct access to container bridge (LAN port of firewall)
              │
              ├─ 10.20.0.0/16 bridge network
              │
              └─ All service containers on this bridge
```

This topology ensures:
1. All traffic is inspected by the firewall (no bypass)
2. Firewall can perform NAT, TLS termination, and packet filtering
3. East-west traffic between containers on the same bridge is NOT re-inspected
4. WireGuard tunnels use kernel-mode interfaces, bypassing container-level NAT

---

## 3. Critical: Monad Protocol HbH Passthrough

### The Monad Register in IPv6 HbH Headers

The **Monad protocol** embeds a **20-byte register** in IPv6 Hop-by-Hop extension headers to transport:
- `MONAD_METRIC_V1` (option type `0x1E`)
- Per-packet eBPF metrics: latency, throughput, CPU load
- Cryptographic proof-of-work for Byzantine fault tolerance
- Path trace information for network debugging

**CRITICAL**: If the firewall strips or rewrites HbH headers, the Monad protocol ceases to function:
- Monad checksums become invalid
- eBPF metrics are lost
- Byzantine consensus breaks
- All observability and proof-of-work mechanisms fail

### OPNsense Configuration (FreeBSD/pf)

#### pf.conf rules (add to /etc/pf.conf):

```pf
# IPv6 Hop-by-Hop extension headers (Monad MONAD_METRIC_V1)
# Option type 0x1E, carried in IPv6 next-header field = 0x00 (HOPOPT)

# Allow HbH on WAN ingress/egress
pass in quick on $wan_if inet6 proto ipv6-opts from any to any
pass out quick on $wan_if inet6 proto ipv6-opts from any to any

# Allow HbH on LAN ingress/egress
pass in quick on $lan_if inet6 proto ipv6-opts from any to any
pass out quick on $lan_if inet6 proto ipv6-opts from any to any

# Explicitly permit IPv6 extension headers (do not scrub)
set skip on lo0
pass in inet6 exthdrs all
pass out inet6 exthdrs all

# Disable IPv6 scrubbing (prevents header rewriting)
# Navigate to System → Advanced → Firewall and uncheck:
# - "Reassemble IPv6 fragments" 
# - "IPv6 stateless IPv4 mapping"
```

#### OPNsense GUI Steps:

1. **Navigate to Firewall → Rules → WAN**
   - Click "Add" (top right)
   - Fill in:
     - **Protocol**: IPv6
     - **Source**: any
     - **Destination**: any
     - **Next Header**: 0 (HOPOPT / IPv6 Extension Header)
     - **Action**: Pass
     - **Description**: "Allow IPv6 HbH extension headers (Monad)"
   - Click "Save"

2. **Repeat for LAN rules**: Firewall → Rules → LAN

3. **Disable IPv6 scrubbing**:
   - Go to System → Advanced → Firewall
   - Uncheck "Enable IPv6 Reassembly"
   - Uncheck "Disable IPv4 mapped IPv6 reassembly"
   - Click "Save"

4. **Verify in System → Logs → Firewall**:
   - Should NOT see dropped IPv6 packets with protocol=0
   - If you do, the HbH rules didn't apply correctly

### IPFire Configuration (Linux/nftables)

#### nftables ruleset (add to /etc/nftables.conf or via Web UI):

```nftables
# IPv6 Hop-by-Hop extension headers (Monad MONAD_METRIC_V1)
# Carried as IPv6 next-header = 0 (HOPOPT)

table ip6 filter {
  chain forward {
    # Allow IPv6 HbH extension headers (Monad protocol)
    ip6 nexthdr 0 accept comment "IPv6 Hop-by-Hop (Monad MONAD_METRIC_V1)"
    
    # Continue with other rules...
  }
  
  chain input {
    # Allow IPv6 HbH on input
    ip6 nexthdr 0 accept comment "IPv6 Hop-by-Hop (Monad MONAD_METRIC_V1)"
  }
  
  chain output {
    # Allow IPv6 HbH on output
    ip6 nexthdr 0 accept comment "IPv6 Hop-by-Hop (Monad MONAD_METRIC_V1)"
  }
}
```

#### IPFire Web UI Steps:

1. **Log in to IPFire Web UI** (port 444)
   - Credentials: admin / password (set at install)

2. **Navigate to Firewall → Firewall Rules**
   - Click "Add" under "Incoming traffic" (or "Forward" for east-west)
   - Fill in:
     - **Source**: Green (LAN) or Blue (WAN)
     - **Destination**: Green (LAN) or Blue (WAN)
     - **Protocol**: IPv6 Extension Headers (may be labeled as "IPv6 Hop-by-Hop")
     - **Action**: ACCEPT
   - Click "Add"

3. **Verify via CLI** (SSH into IPFire):
   ```bash
   nft list ruleset | grep -i "hopto\|0x00\|nexthdr 0"
   ```
   Should show rules allowing nexthdr 0.

---

## 4. Network Addressing Plan

### HOST-A (The Forge / OPNsense)

| Layer | IPv4 | IPv6 |
|-------|------|------|
| **WAN** | DHCP or static (upstream router) | SLAAC or static |
| **LAN Gateway** | 10.20.0.1/16 | fd00:dead:beef:1::1/64 |
| **Container Bridge** | 10.20.0.0/16 | fd00:dead:beef:1::/64 |
| **DNS** | Provided by OPNsense unbound | Provided by OPNsense unbound |

### HOST-B (The Outpost / IPFire)

| Layer | IPv4 | IPv6 |
|-------|------|------|
| **WAN** | DHCP or static (upstream router) | SLAAC or static |
| **LAN Gateway** | 10.20.0.1/16 | fd00:dead:beef:2::1/64 |
| **Container Bridge** | 10.20.0.0/16 | fd00:dead:beef:2::/64 |
| **DNS** | Provided by IPFire unbound | Provided by IPFire unbound |

### All Service Containers

- **Default GW**: 10.20.0.1 (local OPNsense/IPFire gateway)
- **DNS**: Queried from firewall (port 53, TCP/UDP)
- **IPv6 Link-local**: Auto-configured (fe80::*/10)
- **IPv6 Global Unicast**: Assigned from fd00:dead:beef:X::/64 (via SLAAC or DHCPv6)

### WireGuard Tunnel (East-West)

- **Endpoint A**: HOST-A OPNsense VM, UDP 51820
- **Endpoint B**: HOST-B IPFire VM, UDP 51820
- **IPv6 Tunnel**: fd00:dead:beef::1 ↔ fd00:dead:beef::2
- **MTU**: 1380 (1500 physical - 60 IPv6 overhead - 60 WireGuard overhead)
- **Routing**: Kernel-level interface, bypasses container-level NAT
- **Firewall**: Allow UDP 51820 on both WAN rules

---

## 5. Firewall Rule Philosophy

The Unheaded firewall architecture follows a **default-deny** security posture:

### Core Principles

1. **Inbound WAN Traffic**: DENY all, ALLOW only explicitly configured services
2. **Outbound WAN Traffic**: ALLOW all (containers may initiate outbound), DENY known malicious destinations (botnet C2, ransomware pools, etc.)
3. **Established/Related Flows**: ALLOW (connection tracking)
4. **ICMP**: ALLOW ICMPv4 echo-request/reply (ping) and all ICMPv6 (required for neighbor discovery, path MTU)
5. **Monad HbH**: ALLOW IPv6 next-header 0x00 (HOPOPT) without rewriting
6. **WireGuard**: ALLOW UDP 51820 bidirectional on WAN
7. **Logging**: All denied inbound packets logged to Suricata (OPNsense) or Snort (IPFire)
8. **Spoofing**: Block RFC1918 (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16) on WAN ingress
9. **Bogons**: Block known invalid networks (0.0.0.0/8, 224.0.0.0/4, 127.0.0.0/8, etc.)

### Rule Precedence

```
1. Connection tracking (allow established/related)
2. Deny spoofed RFC1918 from WAN
3. Deny bogon networks
4. Deny known malicious IPs (blocklists)
5. ALLOW Monad HbH (IPv6 proto 0)
6. ALLOW ICMP/ICMPv6
7. ALLOW WireGuard (UDP 51820)
8. ALLOW exposed service ports (443, 80, etc.)
9. ALLOW internal LAN traffic
10. DENY all else (default)
```

---

## 6. Exposed Ports (WAN-Accessible)

From the Unheaded **Kingdom Doom Port Range (16666–26666)**:

| Port | Protocol | Service | Direction | WAN Exposed? | Notes |
|------|----------|---------|-----------|--------------|-------|
| **443** | TCP | Gateway HTTPS | Inbound | YES | TLS termination at OPNsense/IPFire HAProxy/squid |
| **80** | TCP | HTTP Redirect | Inbound | YES | Redirect to HTTPS (301/308) |
| **51820** | UDP | WireGuard East-West | Bidirectional | YES | Firewall-to-firewall tunnel (across WAN) |
| **50051–50067** | TCP | gRPC Service Ports | Bidirectional | NO | Internal only, blocked at WAN ingress |
| **8080** | TCP | Dashboard Backend | Bidirectional | NO | Internal only, blocked at WAN ingress |
| **8443** | TCP | Dashboard HTTPS | Bidirectional | NO | Internal only, blocked at WAN ingress |
| **5353** | UDP | mDNS Discovery | Bidirectional | NO | Local network only (link-local multicast) |
| **4789** | UDP | VXLAN Overlay | Bidirectional | NO | Internal container networking |
| **53** | TCP/UDP | DNS | Bidirectional | NO | Internal only (containers → firewall) |

### Firewall Rules for Exposed Ports

#### OPNsense (pf.conf)

```pf
# Exposed ports
pass in on $wan_if inet proto tcp to ($wan_ip) port 80 \
    rdr-to 10.20.0.1 port 80 comment "HTTP redirect"
pass in on $wan_if inet proto tcp to ($wan_ip) port 443 \
    rdr-to 10.20.0.1 port 443 comment "HTTPS gateway"
pass in on $wan_if inet proto udp to ($wan_ip) port 51820 \
    comment "WireGuard east-west"

# Block all gRPC ports from WAN
block in quick on $wan_if inet proto tcp to ($wan_ip) port 50051:50067 \
    comment "Block internal gRPC from WAN"

# Block dashboard from WAN
block in quick on $wan_if inet proto tcp to ($wan_ip) port 8080 \
    comment "Block dashboard from WAN"
block in quick on $wan_if inet proto tcp to ($wan_ip) port 8443 \
    comment "Block dashboard HTTPS from WAN"
```

#### IPFire (nftables)

```nftables
table inet filter {
  chain input {
    # Allow WAN HTTPS
    iif $wan_if tcp dport 443 accept comment "HTTPS gateway"
    iif $wan_if tcp dport 80 accept comment "HTTP redirect"
    
    # Allow WireGuard
    iif $wan_if udp dport 51820 accept comment "WireGuard"
    
    # Block internal services from WAN
    iif $wan_if tcp dport 50051:50067 drop comment "Block gRPC from WAN"
    iif $wan_if tcp dport 8080 drop comment "Block dashboard from WAN"
    iif $wan_if tcp dport 8443 drop comment "Block dashboard from WAN"
  }
}
```

---

## 7. Platform-Specific Deployment

### NixOS (Recommended for development)

OPNsense and IPFire run as **libvirt QEMU VMs**:

```nix
# /etc/nixos/configuration.nix
virtualisation.libvirtd.enable = true;
virtualisation.libvirtd.onShutdown = "shutdown";

# Mount ISO for OPNsense/IPFire
environment.etc."libvirt/qemu/opnsense.xml".text = ''
  <domain type='kvm'>
    <name>opnsense-forge</name>
    <memory unit='KiB'>2097152</memory>
    <!-- Network interfaces -->
    <interface type='network'>
      <source network='default'/>
      <model type='rtl8139'/>
    </interface>
    <!-- macvlan bridge to physical NIC -->
    <interface type='direct'>
      <source dev='eno1' mode='macvlan'/>
      <model type='virtio'/>
    </interface>
  </domain>
'';
```

**Benefits**:
- Full kernel isolation
- Easy snapshotting and rollback
- Native NixOS declarative management
- Support for nested virtualization

### Docker (Multi-tenant deployments)

OPNsense/IPFire run as **privileged containers with macvlan networking**:

```dockerfile
# Dockerfile for OPNsense container
FROM freebsd:13.0
RUN pkg install -y opnsense opnsense-basesystem
COPY pf.conf /etc/pf.conf
EXPOSE 443/tcp 80/tcp 51820/udp
ENTRYPOINT ["/usr/sbin/pf", "-F", "all", "-f", "/etc/pf.conf"]
```

```bash
# Docker run command
docker run --privileged \
  --network host \
  --cap-add NET_ADMIN \
  -v /etc/pf.conf:/etc/pf.conf \
  unheaded/opnsense:latest
```

**Benefits**:
- Lightweight, fast startup
- Easy horizontal scaling
- Integrates with Docker Swarm/Kubernetes

**Drawbacks**:
- Shared kernel with host
- Limited isolation
- FreeBSD containers require FreeBSD host (for OPNsense)

### LXD (Production, best balance)

OPNsense/IPFire run as **LXD VMs with ISO import**:

```bash
# Import OPNsense ISO to LXD
lxc import opnsense-24.1.iso --vm opnsense-forge

# Configure network
lxc config device add opnsense-forge eth0 \
  nic nictype=macvlan parent=eno1

# Start
lxc start opnsense-forge
```

**Benefits**:
- Full kernel isolation (VM mode)
- LXD system container management
- Easy snapshot/restore
- Native IP filtering and resource limits

---

## 8. Compliance and Licensing

### OPNsense (BSD 2-Clause License)

**Compatibility**:
- Permissive license; can be integrated into commercial Unheaded deployments
- No requirement to open-source modifications to OPNsense itself
- Unheaded (MIT) remains MIT; OPNsense binaries retain BSD 2-Clause

**Audit Trail Requirements** (SOC2, PCI-DSS, HIPAA):
- OPNsense logs all firewall decisions to `/var/log/pf.log`
- Suricata IDS logs alert and event data
- HAProxy logs all TLS handshakes and HTTP requests
- Logs are exported to Anamnesis (Unheaded's SIEM-compatible telemetry system)

### IPFire (GNU General Public License v3)

**Compatibility**:
- Copyleft license; modifications to IPFire itself must remain open source
- Unheaded (MIT) is NOT linked to IPFire; remains MIT
- IPFire binary and kernel modules are separate from Unheaded application code

**Audit Trail Requirements**:
- IPFire logs all nftables decisions to syslog
- Snort IDS logs alerts to `/var/log/snort/`
- Squid proxy logs HTTP requests to `/var/log/squid/access.log`
- All logs exported to Anamnesis for SOC2/PCI-DSS/HIPAA compliance

### Joint Compliance Strategy

Both firewalls support:
- **NetFlow/sFlow** export for network telemetry (integration with Anamnesis)
- **Syslog/CEF** for centralized log aggregation (SIEM integration)
- **TLS certificate chain validation** for egress filtering
- **Role-based access control (RBAC)** in Web UIs
- **Audit logging** of all admin actions (rule changes, user logins, etc.)

---

## 9. Testing and Validation

### Health Checks

1. **Ping Firewall Gateway**:
   ```bash
   ping 10.20.0.1      # IPv4
   ping -6 fd00:dead:beef:1::1  # IPv6
   ```

2. **Verify HbH Passthrough**:
   ```bash
   tcpdump -i eth0 'ip6 proto 0'  # Should capture Monad packets
   ```

3. **Port Reachability**:
   ```bash
   curl -v https://10.20.0.1:443  # Should reach gateway HTTPS
   timeout 5 nc -zv 10.20.0.1 8080 2>&1  # Should fail (blocked)
   ```

4. **WireGuard Tunnel**:
   ```bash
   ping fd00:dead:beef::2  # Cross-host tunnel test
   ```

### Firewall Rule Validation

**OPNsense**:
```bash
pfctl -s rules | grep -i "hopopt\|50051\|51820"
```

**IPFire**:
```bash
nft list ruleset | grep -E "nexthdr 0|dport 50051|dport 51820"
```

---

## 10. Disaster Recovery and Failover

### Firewall Configuration Backup

**OPNsense**:
1. System → Backup & Restore
2. Download full backup (encrypted with optional password)
3. Store in Anamnesis backup vault
4. Restoration: Upload same backup file

**IPFire**:
1. Backup → Full Backup
2. Download `/var/ipfire/backup/` directory
3. Restoration: Upload and run restore script

### Failover Scenarios

**If OPNsense fails**:
1. WireGuard tunnel to HOST-B drops (WG endpoint unreachable)
2. HOST-A containers lose internet access (no WAN gateway)
3. Recovery: Restore latest OPNsense backup or redeploy from NixOS configuration
4. Failover mechanism: External load balancer (in cloud deployments) re-routes WAN traffic to IPFire

**If IPFire fails**:
1. HOST-B containers lose internet access
2. WireGuard tunnel to HOST-A drops
3. Recovery: Same as OPNsense (restore backup or redeploy)

### Redundancy for HA

In production deployments, implement:
- **VRRP (Virtual Router Redundancy Protocol)** between OPNsense instances
- **CARP (Common Address Redundancy Protocol)** for failover on BSD platforms
- **keepalived** on IPFire for LB-style failover
- **Anamnesis distributed consensus** to elect active firewall in case of split-brain

---

## 11. Additional Resources

- **OPNsense Docs**: https://docs.opnsense.org/
- **IPFire Docs**: https://wiki.ipfire.org/
- **Monad Protocol Spec**: See `docs/protocol/MONAD_SPEC.md` in this repository
- **Unheaded Port Registry**: See `docs/network/INGRESS_EGRESS_PORTS.md` in this repository
- **Firewall Rule Testing**: See `docs/network/MONAD_HBH_FIREWALL_RULES.md` in this repository

---

**Document Version**: 1.0  
**Last Updated**: 2026-02-26  
**Maintained By**: Unheaded Development Team  
**License**: MIT (Unheaded), BSD 2-Clause (OPNsense components), GPL v3 (IPFire components)
