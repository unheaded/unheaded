# Unheaded Kingdom Ingress/Egress Ports and Firewall Rules

## Overview

This document provides a complete port reference for all Unheaded services, including WAN-exposed vs. internal-only classifications, firewall zone definitions, protocol requirements, and per-service firewall rules.

The **Kingdom Doom Port Range** is **16666–26666**, with sub-allocations:
- **50051–50067**: gRPC service ports (internal only)
- **8080**: Dashboard backend (internal only)
- **8443**: Dashboard HTTPS (internal only)
- **443**: Gateway HTTPS (WAN-exposed)
- **80**: HTTP redirect (WAN-exposed)
- **51820**: WireGuard (WAN-exposed)

---

## 1. Master Port Registry

### All Services at a Glance

| Service | Port | Protocol | Direction | WAN Exposed | IPv4 | IPv6 | FirewallZone | Notes |
|---------|------|----------|-----------|-------------|------|------|--------------|-------|
| **HTTP Redirect** | 80 | TCP | Inbound | YES | Yes | Yes | WAN | Redirect to HTTPS (301/308) |
| **HTTPS Gateway** | 443 | TCP | Inbound | YES | Yes | Yes | WAN | TLS termination (HAProxy/squid) |
| **WireGuard Tunnel** | 51820 | UDP | Bidirectional | YES | Yes | Yes | WAN/VPN | Host-to-host east-west VPN |
| **gRPC Svc 1** | 50051 | TCP | Bidirectional | NO | Yes | Yes | Internal | Accounting service |
| **gRPC Svc 2** | 50052 | TCP | Bidirectional | NO | Yes | Yes | Internal | Treasury service |
| **gRPC Svc 3** | 50053 | TCP | Bidirectional | NO | Yes | Yes | Internal | Ledger service |
| **gRPC Svc 4** | 50054 | TCP | Bidirectional | NO | Yes | Yes | Internal | Guardian service |
| **gRPC Svc 5** | 50055 | TCP | Bidirectional | NO | Yes | Yes | Internal | Sentinel service |
| **gRPC Svc 6** | 50056 | TCP | Bidirectional | NO | Yes | Yes | Internal | Herald service |
| **gRPC Svc 7** | 50057 | TCP | Bidirectional | NO | Yes | Yes | Internal | Archivist service |
| **gRPC Svc 8** | 50058 | TCP | Bidirectional | NO | Yes | Yes | Internal | Chronicler service |
| **gRPC Svc 9** | 50059 | TCP | Bidirectional | NO | Yes | Yes | Internal | Philosopher service |
| **gRPC Svc 10** | 50060 | TCP | Bidirectional | NO | Yes | Yes | Internal | Scribe service |
| **gRPC Svc 11** | 50061 | TCP | Bidirectional | NO | Yes | Yes | Internal | Keeper service |
| **gRPC Svc 12** | 50062 | TCP | Bidirectional | NO | Yes | Yes | Internal | Sage service |
| **gRPC Svc 13** | 50063 | TCP | Bidirectional | NO | Yes | Yes | Internal | Oracle service |
| **gRPC Svc 14** | 50064 | TCP | Bidirectional | NO | Yes | Yes | Internal | Mystic service |
| **gRPC Svc 15** | 50065 | TCP | Bidirectional | NO | Yes | Yes | Internal | Harbinger service |
| **gRPC Svc 16** | 50066 | TCP | Bidirectional | NO | Yes | Yes | Internal | Emissary service |
| **gRPC Svc 17** | 50067 | TCP | Bidirectional | NO | Yes | Yes | Internal | Witness service |
| **Dashboard BE** | 8080 | TCP | Bidirectional | NO | Yes | Yes | Internal | Admin dashboard backend |
| **Dashboard HTTPS** | 8443 | TCP | Bidirectional | NO | Yes | Yes | Internal | Admin dashboard frontend (HTTPS) |
| **DNS** | 53 | TCP/UDP | Bidirectional | NO | Yes | Yes | Internal | OPNsense/IPFire unbound DNS |
| **mDNS Discovery** | 5353 | UDP | Bidirectional | NO | Yes | Yes | LAN | Link-local service discovery |
| **VXLAN Overlay** | 4789 | UDP | Bidirectional | NO | Yes | Yes | Internal | Container overlay networking |

---

## 2. Detailed Port Descriptions

### 2.1 WAN-Exposed Ports

#### Port 80/TCP — HTTP Redirect

- **Service**: HTTP ingress gateway
- **Visibility**: Public (WAN-accessible)
- **Firewall Action**: PASS inbound to OPNsense/IPFire on port 80
- **Routing**: Redirect (HTTP 301/308) to HTTPS port 443
- **TLS**: Not used (redirect before TLS negotiation)
- **Example Request**:
  ```
  GET / HTTP/1.1
  Host: example.com
  
  HTTP/1.1 301 Moved Permanently
  Location: https://example.com/
  ```
- **OPNsense Rule**:
  ```pf
  pass in on $wan_if inet proto tcp to ($wan_ip) port 80 \
      rdr-to 10.20.0.1 port 80 comment "HTTP redirect to gateway"
  ```
- **IPFire Rule**:
  ```nftables
  iif "eth0" tcp dport 80 accept comment "HTTP redirect"
  ```

#### Port 443/TCP — HTTPS Gateway

- **Service**: HTTPS ingress gateway (TLS termination)
- **Visibility**: Public (WAN-accessible)
- **Firewall Action**: PASS inbound to OPNsense/IPFire on port 443
- **TLS**: Required (minimum TLS 1.2, recommend TLS 1.3)
- **Certificates**: Let's Encrypt wildcard or self-signed per deployment
- **Backend**: OPNsense HAProxy or IPFire squid proxy
- **Example Request**:
  ```
  CONNECT example.com:443 HTTP/1.1
  TLS: Certificate, ClientHello, ServerHello, ChangeCipherSpec, Finished
  ```
- **OPNsense Rule**:
  ```pf
  pass in on $wan_if inet proto tcp to ($wan_ip) port 443 \
      rdr-to 10.20.0.1 port 443 comment "HTTPS gateway"
  ```
- **IPFire Rule**:
  ```nftables
  iif "eth0" tcp dport 443 accept comment "HTTPS gateway"
  ```
- **Cipher Suite** (recommended):
  - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
  - TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
  - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256

#### Port 51820/UDP — WireGuard VPN

- **Service**: WireGuard east-west tunnel (HOST-A ↔ HOST-B)
- **Visibility**: Public (WAN-accessible from both hosts)
- **Firewall Action**: PASS bidirectional UDP on port 51820
- **Encryption**: Curve25519, ChaCha20-Poly1305, BLAKE2s
- **MTU**: 1380 bytes (IPv6 + WireGuard overhead)
- **Tunnel Endpoints**:
  - HOST-A: fd00:dead:beef::1/64
  - HOST-B: fd00:dead:beef::2/64
- **Endpoint Discovery**: Via OPNsense/IPFire listening on port 51820
- **OPNsense Rule**:
  ```pf
  pass in on $wan_if inet proto udp to ($wan_ip) port 51820 \
      comment "WireGuard VPN east-west"
  pass out on $wan_if inet proto udp from ($wan_ip) port 51820 \
      comment "WireGuard VPN egress"
  ```
- **IPFire Rule**:
  ```nftables
  iif "eth0" udp dport 51820 accept comment "WireGuard inbound"
  oif "eth0" udp sport 51820 accept comment "WireGuard outbound"
  ```
- **Key Rotation**: Every 30 days (Anamnesis-managed)
- **Replay Protection**: Enabled (256-entry table, 64 second window)

---

### 2.2 Internal-Only Ports (Blocked at WAN)

#### Ports 50051–50067/TCP — gRPC Service Ports

- **Service**: Inter-service gRPC communication (17 services)
- **Visibility**: Internal only (blocked at WAN firewall)
- **Protocol**: gRPC over HTTP/2
- **TLS**: Mutual TLS (mTLS) between services
- **Authentication**: Service-to-service mutual certificates
- **Port Allocation**:

| Port | Service | Role |
|------|---------|------|
| 50051 | Accounting | Transaction ledger |
| 50052 | Treasury | Fund management |
| 50053 | Ledger | State database |
| 50054 | Guardian | Authorization |
| 50055 | Sentinel | Monitoring/metrics |
| 50056 | Herald | Event publishing |
| 50057 | Archivist | Archive management |
| 50058 | Chronicler | Event logging |
| 50059 | Philosopher | Config/consensus |
| 50060 | Scribe | Data serialization |
| 50061 | Keeper | Secret management |
| 50062 | Sage | Machine learning models |
| 50063 | Oracle | Data oracle service |
| 50064 | Mystic | Cryptographic operations |
| 50065 | Harbinger | Predictive analytics |
| 50066 | Emissary | External API gateway |
| 50067 | Witness | Observability/tracing |

- **Firewall Rule** (OPNsense):
  ```pf
  # Block gRPC ports from WAN (explicit deny)
  block in quick on $wan_if inet proto tcp to ($wan_ip) port 50051:50067 \
      comment "Block internal gRPC from WAN"
  block in quick on $wan_if inet6 proto tcp to ($wan_ipv6) port 50051:50067 \
      comment "Block internal gRPC from WAN (IPv6)"
  ```
- **Firewall Rule** (IPFire):
  ```nftables
  # Block gRPC from WAN
  iif "eth0" tcp dport 50051:50067 drop comment "Block internal gRPC from WAN"
  ```
- **Internal Firewall Rule** (allow LAN ↔ LAN):
  ```pf
  # OPNsense: Allow gRPC between containers
  pass in on $lan_if inet proto tcp from 10.20.0.0/16 to 10.20.0.0/16 \
      port 50051:50067 comment "Allow internal gRPC"
  ```

#### Port 8080/TCP — Dashboard Backend

- **Service**: Admin dashboard API server
- **Visibility**: Internal only (blocked at WAN firewall)
- **Protocol**: HTTP/REST
- **Authentication**: JWT tokens, OAuth2
- **Firewall Rule** (OPNsense):
  ```pf
  block in quick on $wan_if inet proto tcp to ($wan_ip) port 8080 \
      comment "Block dashboard backend from WAN"
  ```
- **Firewall Rule** (IPFire):
  ```nftables
  iif "eth0" tcp dport 8080 drop comment "Block dashboard from WAN"
  ```
- **Allowed From**: LAN containers only
  ```pf
  # OPNsense
  pass in on $lan_if inet proto tcp from 10.20.0.0/16 to 10.20.0.1 \
      port 8080 comment "Dashboard from LAN"
  ```

#### Port 8443/TCP — Dashboard HTTPS

- **Service**: Admin dashboard frontend (web UI)
- **Visibility**: Internal only (blocked at WAN firewall)
- **Protocol**: HTTPS
- **TLS**: Optional (may use self-signed certificates)
- **Firewall Rule** (OPNsense):
  ```pf
  block in quick on $wan_if inet proto tcp to ($wan_ip) port 8443 \
      comment "Block dashboard HTTPS from WAN"
  ```
- **Firewall Rule** (IPFire):
  ```nftables
  iif "eth0" tcp dport 8443 drop comment "Block dashboard HTTPS from WAN"
  ```

---

### 2.3 Infrastructure Ports (Internal, Non-gRPC)

#### Port 53/TCP/UDP — DNS

- **Service**: OPNsense unbound / IPFire DNS resolver
- **Visibility**: Internal only
- **Protocol**: DNS over TCP/UDP (standard RFC 1035)
- **DoT Support**: DNS over TLS (optional, port 853)
- **DoH Support**: DNS over HTTPS (optional, via gateway port 443)
- **Firewall Rule** (OPNsense):
  ```pf
  # Allow DNS from containers to firewall
  pass in on $lan_if inet proto tcp from 10.20.0.0/16 to 10.20.0.1 port 53 \
      comment "DNS TCP from containers"
  pass in on $lan_if inet proto udp from 10.20.0.0/16 to 10.20.0.1 port 53 \
      comment "DNS UDP from containers"
  pass in on $lan_if inet6 proto tcp from fd00:dead:beef:1::/64 \
      to fd00:dead:beef:1::1 port 53 comment "DNS TCP IPv6"
  pass in on $lan_if inet6 proto udp from fd00:dead:beef:1::/64 \
      to fd00:dead:beef:1::1 port 53 comment "DNS UDP IPv6"
  ```
- **Default Resolver**: All containers use 10.20.0.1 (firewall gateway) as primary DNS

#### Port 5353/UDP — mDNS Discovery

- **Service**: Multicast DNS for service discovery
- **Visibility**: Link-local only (not routed)
- **Protocol**: mDNS (RFC 6762) on 224.0.0.251:5353
- **Use Case**: Service container discovery, health checks
- **Firewall Rule** (typically no rule needed, link-local multicast):
  ```pf
  # OPNsense: Allow mDNS (optional, usually allowed by default)
  pass in on $lan_if inet proto udp to 224.0.0.251 port 5353 \
      comment "mDNS discovery"
  ```
- **IPv6 mDNS**: ff02::fb:5353 (link-local)

#### Port 4789/UDP — VXLAN Overlay

- **Service**: Container overlay networking (Docker/Kubernetes)
- **Visibility**: Internal only
- **Protocol**: VXLAN (RFC 7348)
- **Firewall Rule** (OPNsense):
  ```pf
  # Allow VXLAN between containers
  pass in on $lan_if inet proto udp to 10.20.0.0/16 port 4789 \
      comment "VXLAN overlay"
  ```
- **Used By**: Swarm mode, Kubernetes CNI plugins
- **MTU Impact**: VXLAN adds 50-byte overhead; containers use 1450-byte MTU

---

## 3. Firewall Zone Definitions

### Zone Architecture

```
┌─────────────────────────────────────────────────────────┐
│ WAN (Public Internet)                                   │
├─────────────────────────────────────────────────────────┤
│  Port 80/TCP    ← HTTP redirect                         │
│  Port 443/TCP   ← HTTPS gateway                         │
│  Port 51820/UDP ← WireGuard VPN endpoint               │
├─────────────────────────────────────────────────────────┤
│ Firewall Gateway (OPNsense / IPFire)                   │
├─────────────────────────────────────────────────────────┤
│ Internal LAN (10.20.0.0/16 / fd00:dead:beef:X::/64)   │
├─────────────────────────────────────────────────────────┤
│  Port 50051–50067/TCP ← gRPC (inter-service)          │
│  Port 8080/TCP        ← Dashboard backend              │
│  Port 8443/TCP        ← Dashboard HTTPS                │
│  Port 53/TCP/UDP      ← DNS resolver                   │
│  Port 5353/UDP        ← mDNS discovery (link-local)   │
│  Port 4789/UDP        ← VXLAN overlay                  │
├─────────────────────────────────────────────────────────┤
│ Service Containers (25 services on HOST-A, 6 on HOST-B)│
└─────────────────────────────────────────────────────────┘
```

### Zone Properties

| Zone | Interface | CIDR | IPv6 | Routing | Firewall Policy |
|------|-----------|------|------|---------|-----------------|
| **WAN** | eno1/eth0 | ISP-assigned | ISP-assigned | Dynamic (DHCP/RA) | Default DENY inbound, ALLOW outbound |
| **LAN** | docker0/bridge | 10.20.0.0/16 | fd00:dead:beef:X::/64 | Static (firewall GW) | Default ALLOW inbound from containers, Default ALLOW outbound to WAN |
| **VPN** | wg0 (WireGuard) | fd00:dead:beef::1/2 | (IPv6 only) | Static (kernel routes) | Default ALLOW (tunnel endpoint) |
| **DMZ** | (optional) | 10.21.0.0/16 | fd00:dead:beef:3::/64 | Static | Default DENY, ALLOW only HTTP/HTTPS |
| **Management** | (optional) | 10.22.0.0/16 | fd00:dead:beef:4::/64 | Static | Default DENY, ALLOW only SSH/SNMP from admin network |

---

## 4. Per-Service Firewall Rules (Complete Ruleset)

### 4.1 OPNsense (pf.conf) Complete Rules

```pf
# ============================================================================
# UNHEADED KINGDOM — COMPLETE FIREWALL RULESET (OPNsense/pf)
# ============================================================================
# Version: 1.0
# Last Updated: 2026-02-26
# Generated for: HOST-A (The Forge)
# ============================================================================

# Variables
wan_if = "em0"
lan_if = "em1"
wan_ip = "203.0.113.100"  # Example WAN IP (replace with actual)
wan_ipv6 = "2001:db8::1"  # Example WAN IPv6 (replace with actual)

# ============================================================================
# SECTION 1: CRITICAL MONAD PROTOCOL SUPPORT
# ============================================================================

# IPv6 Hop-by-Hop extension headers (Monad MONAD_METRIC_V1, option 0x1E)
pass in quick inet6 proto ipv6-opts from any to any
pass out quick inet6 proto ipv6-opts from any to any
pass in quick inet6 exthdrs hbh from any to any \
    comment "Monad: IPv6 HbH extension headers ingress"
pass out quick inet6 exthdrs hbh from any to any \
    comment "Monad: IPv6 HbH extension headers egress"

# IPv6 fragmentation (required for HbH + large payloads)
pass inet6 proto ipv6-frag from any to any \
    comment "Allow IPv6 fragmentation"

# ============================================================================
# SECTION 2: CONNECTION TRACKING
# ============================================================================

pass in quick proto tcp flags S/SA modulate state \
    comment "TCP established connections"
pass in quick proto udp keep state \
    comment "UDP established connections"
pass in quick proto icmp keep state \
    comment "ICMP established"
pass in quick proto icmpv6 keep state \
    comment "ICMPv6 established"

# ============================================================================
# SECTION 3: EXPOSED PORTS (WAN-ACCESSIBLE)
# ============================================================================

# Port 80: HTTP redirect to HTTPS
pass in on $wan_if inet proto tcp to ($wan_ip) port 80 \
    rdr-to 10.20.0.1 port 80 comment "HTTP redirect"
pass in on $wan_if inet6 proto tcp to ($wan_ipv6) port 80 \
    rdr-to fd00:dead:beef:1::1 port 80 comment "HTTP redirect IPv6"

# Port 443: HTTPS gateway
pass in on $wan_if inet proto tcp to ($wan_ip) port 443 \
    rdr-to 10.20.0.1 port 443 comment "HTTPS gateway"
pass in on $wan_if inet6 proto tcp to ($wan_ipv6) port 443 \
    rdr-to fd00:dead:beef:1::1 port 443 comment "HTTPS gateway IPv6"

# Port 51820: WireGuard tunnel
pass in on $wan_if inet proto udp to ($wan_ip) port 51820 \
    comment "WireGuard east-west VPN"
pass in on $wan_if inet6 proto udp to ($wan_ipv6) port 51820 \
    comment "WireGuard IPv6 endpoint"
pass out on $wan_if inet proto udp from ($wan_ip) port 51820 \
    comment "WireGuard egress"

# ============================================================================
# SECTION 4: INTERNAL SERVICES (gRPC, Dashboard)
# ============================================================================

# Allow gRPC ports within LAN (container-to-container)
pass in on $lan_if inet proto tcp from 10.20.0.0/16 to 10.20.0.0/16 \
    port 50051:50067 comment "Allow internal gRPC IPv4"
pass in on $lan_if inet6 proto tcp from fd00:dead:beef:1::/64 \
    to fd00:dead:beef:1::/64 port 50051:50067 \
    comment "Allow internal gRPC IPv6"

# Allow Dashboard backend from LAN
pass in on $lan_if inet proto tcp from 10.20.0.0/16 to 10.20.0.1 port 8080 \
    comment "Dashboard backend from LAN IPv4"
pass in on $lan_if inet6 proto tcp from fd00:dead:beef:1::/64 \
    to fd00:dead:beef:1::1 port 8080 \
    comment "Dashboard backend from LAN IPv6"

# Allow Dashboard HTTPS from LAN
pass in on $lan_if inet proto tcp from 10.20.0.0/16 to 10.20.0.1 port 8443 \
    comment "Dashboard HTTPS from LAN IPv4"
pass in on $lan_if inet6 proto tcp from fd00:dead:beef:1::/64 \
    to fd00:dead:beef:1::1 port 8443 \
    comment "Dashboard HTTPS from LAN IPv6"

# ============================================================================
# SECTION 5: DNS, mDNS, VXLAN (Infrastructure)
# ============================================================================

# DNS from containers to firewall (port 53)
pass in on $lan_if inet proto tcp from 10.20.0.0/16 to 10.20.0.1 port 53 \
    comment "DNS TCP from containers"
pass in on $lan_if inet proto udp from 10.20.0.0/16 to 10.20.0.1 port 53 \
    comment "DNS UDP from containers"
pass in on $lan_if inet6 proto tcp from fd00:dead:beef:1::/64 \
    to fd00:dead:beef:1::1 port 53 comment "DNS TCP IPv6"
pass in on $lan_if inet6 proto udp from fd00:dead:beef:1::/64 \
    to fd00:dead:beef:1::1 port 53 comment "DNS UDP IPv6"

# mDNS discovery (link-local multicast, port 5353)
pass in on $lan_if inet proto udp to 224.0.0.251 port 5353 \
    comment "mDNS IPv4"
pass in on $lan_if inet6 proto udp to ff02::fb port 5353 \
    comment "mDNS IPv6"

# VXLAN overlay (port 4789)
pass in on $lan_if inet proto udp to 10.20.0.0/16 port 4789 \
    comment "VXLAN overlay"
pass in on $lan_if inet6 proto udp to fd00:dead:beef:1::/64 port 4789 \
    comment "VXLAN overlay IPv6"

# ============================================================================
# SECTION 6: ICMP AND ICMPv6 (Required for diagnostics, path MTU)
# ============================================================================

# ICMP echo-request/reply (ping)
pass quick inet proto icmp all comment "ICMP echo"

# ICMPv6 (neighbor discovery, unreachable, time exceeded)
pass quick inet6 proto icmpv6 all comment "ICMPv6 all"

# ============================================================================
# SECTION 7: BLOCK INTERNAL PORTS FROM WAN (Explicit deny)
# ============================================================================

# Block gRPC ports 50051–50067 from WAN
block in quick on $wan_if inet proto tcp to ($wan_ip) port 50051:50067 \
    comment "Block internal gRPC from WAN IPv4"
block in quick on $wan_if inet6 proto tcp to ($wan_ipv6) port 50051:50067 \
    comment "Block internal gRPC from WAN IPv6"

# Block dashboard from WAN
block in quick on $wan_if inet proto tcp to ($wan_ip) port 8080 \
    comment "Block dashboard from WAN IPv4"
block in quick on $wan_if inet proto tcp to ($wan_ip) port 8443 \
    comment "Block dashboard HTTPS from WAN IPv4"
block in quick on $wan_if inet6 proto tcp to ($wan_ipv6) port 8080 \
    comment "Block dashboard from WAN IPv6"
block in quick on $wan_if inet6 proto tcp to ($wan_ipv6) port 8443 \
    comment "Block dashboard HTTPS from WAN IPv6"

# ============================================================================
# SECTION 8: ANTISPOOFING (RFC1918 from WAN)
# ============================================================================

block in quick on $wan_if inet from 10.0.0.0/8 to any \
    comment "Block RFC1918 spoofing (10.0.0.0/8)"
block in quick on $wan_if inet from 172.16.0.0/12 to any \
    comment "Block RFC1918 spoofing (172.16.0.0/12)"
block in quick on $wan_if inet from 192.168.0.0/16 to any \
    comment "Block RFC1918 spoofing (192.168.0.0/16)"

# ============================================================================
# SECTION 9: BOGON BLOCKING
# ============================================================================

block in quick on $wan_if inet from 0.0.0.0/8 to any \
    comment "Block bogon 0.0.0.0/8"
block in quick on $wan_if inet from 127.0.0.0/8 to any \
    comment "Block bogon loopback"
block in quick on $wan_if inet from 224.0.0.0/4 to any \
    comment "Block bogon multicast"
block in quick on $wan_if inet from 240.0.0.0/4 to any \
    comment "Block bogon 240.0.0.0/4"
block in quick on $wan_if inet from 255.255.255.255/32 to any \
    comment "Block bogon broadcast"

# ============================================================================
# SECTION 10: DEFAULT DENY (implicit)
# ============================================================================

# All other inbound traffic is implicitly denied
# All outbound traffic from LAN is allowed by default
```

### 4.2 IPFire (nftables) Complete Rules

```nftables
#!/usr/sbin/nft -f

# ============================================================================
# UNHEADED KINGDOM — COMPLETE FIREWALL RULESET (IPFire/nftables)
# ============================================================================
# Version: 1.0
# Last Updated: 2026-02-26
# Generated for: HOST-B (The Outpost)
# ============================================================================

flush ruleset

table inet filter {
  # ========================================================================
  # INPUT CHAIN (WAN traffic)
  # ========================================================================
  chain input {
    type filter hook input priority 0; policy drop;

    # Loopback
    iif lo accept

    # Connection tracking
    ct state established,related accept

    # CRITICAL: Monad IPv6 HbH (must come before deny rules)
    ip6 nexthdr 0 accept comment "IPv6 Hop-by-Hop (Monad)"

    # ICMPv4 and ICMPv6
    ip protocol icmp accept comment "ICMPv4"
    ip6 nexthdr icmpv6 accept comment "ICMPv6"

    # Allow from LAN (green0)
    iif "green0" accept comment "Allow all from LAN"

    # EXPOSED PORTS (WAN ingress)
    iif "eth0" tcp dport 80 accept comment "HTTP redirect"
    iif "eth0" tcp dport 443 accept comment "HTTPS gateway"
    iif "eth0" udp dport 51820 accept comment "WireGuard VPN"

    # DNS from containers (docker0 bridge)
    iif "docker0" tcp dport 53 accept comment "DNS TCP from containers"
    iif "docker0" udp dport 53 accept comment "DNS UDP from containers"

    # mDNS discovery (link-local)
    iif "green0" udp dport 5353 accept comment "mDNS discovery"

    # VXLAN overlay (container networking)
    iif "docker0" udp dport 4789 accept comment "VXLAN overlay"

    # BLOCK INTERNAL PORTS FROM WAN
    iif "eth0" tcp dport 50051:50067 drop comment "Block gRPC from WAN"
    iif "eth0" tcp dport 8080 drop comment "Block dashboard from WAN"
    iif "eth0" tcp dport 8443 drop comment "Block dashboard HTTPS from WAN"

    # ANTISPOOFING (RFC1918 from WAN)
    iif "eth0" ip saddr 10.0.0.0/8 drop comment "Block RFC1918 spoofing"
    iif "eth0" ip saddr 172.16.0.0/12 drop comment "Block RFC1918 spoofing"
    iif "eth0" ip saddr 192.168.0.0/16 drop comment "Block RFC1918 spoofing"

    # BOGON BLOCKING
    iif "eth0" ip saddr 0.0.0.0/8 drop comment "Block bogon 0.0.0.0/8"
    iif "eth0" ip saddr 127.0.0.0/8 drop comment "Block bogon loopback"
    iif "eth0" ip saddr 224.0.0.0/4 drop comment "Block bogon multicast"
    iif "eth0" ip saddr 240.0.0.0/4 drop comment "Block bogon 240.0.0.0/4"

    # Log and drop all else
    limit rate 1/minute counter log prefix "INPUT DROP: " drop
  }

  # ========================================================================
  # FORWARD CHAIN (container-to-container and container-to-WAN)
  # ========================================================================
  chain forward {
    type filter hook forward priority 0; policy drop;

    # CRITICAL: Monad IPv6 HbH
    ip6 nexthdr 0 accept comment "IPv6 Hop-by-Hop (Monad) forward"

    # Connection tracking
    ct state established,related accept

    # Allow from LAN to any
    iif "green0" accept comment "Allow all from LAN forward"
    iif "docker0" accept comment "Allow all from containers forward"

    # ICMPv6
    ip6 nexthdr icmpv6 accept comment "ICMPv6 forward"

    # INTERNAL SERVICES (gRPC, Dashboard)
    iif "docker0" tcp dport 50051:50067 accept comment "Allow gRPC"
    iif "docker0" tcp dport 8080 accept comment "Allow dashboard backend"
    iif "docker0" tcp dport 8443 accept comment "Allow dashboard HTTPS"
    iif "docker0" tcp dport 53 accept comment "Allow DNS"
    iif "docker0" udp dport 53 accept comment "Allow DNS UDP"
    iif "docker0" udp dport 5353 accept comment "Allow mDNS"
    iif "docker0" udp dport 4789 accept comment "Allow VXLAN"

    # ANTISPOOFING from WAN
    iif "eth0" ip saddr 10.0.0.0/8 drop comment "Block RFC1918 from WAN"
    iif "eth0" ip saddr 172.16.0.0/12 drop comment "Block RFC1918 from WAN"
    iif "eth0" ip saddr 192.168.0.0/16 drop comment "Block RFC1918 from WAN"

    # Log and drop
    limit rate 1/minute counter log prefix "FORWARD DROP: " drop
  }

  # ========================================================================
  # OUTPUT CHAIN (firewall host egress)
  # ========================================================================
  chain output {
    type filter hook output priority 0; policy accept;

    # Monad IPv6 HbH (usually output is accept by default)
    ip6 nexthdr 0 accept comment "IPv6 Hop-by-Hop (Monad) output"
  }
}

# ============================================================================
# NAT TABLE (optional SNAT for containers)
# ============================================================================

table nat {
  chain postrouting {
    type nat hook postrouting priority 100; policy accept;

    # SNAT outbound container traffic to WAN IP
    oif "eth0" ip saddr 10.20.0.0/16 snat to 203.0.113.100 \
        comment "SNAT containers to WAN"
  }
}
```

---

## 5. Traffic Flow Diagrams

### 5.1 Inbound Traffic (WAN → Container)

```
Internet (203.0.113.0/24)
        │
        │ TCP 443 (HTTPS)
        ▼
    Firewall WAN Interface (OPNsense/IPFire)
        │
        │ Match rule: "HTTPS gateway"
        │ Action: PASS / NAT to 10.20.0.1:443
        ▼
    Firewall LAN Interface (10.20.0.1/16)
        │
        │ HAProxy / squid proxy
        │ TLS termination
        ▼
    Container Bridge (10.20.0.0/16)
        │
        │ HTTP (plaintext) to backend service
        │ Port 8080 / 50051
        ▼
    Service Container (10.20.0.X)
        │
        ▼ Response (reverse path)
    HTTP response → HAProxy → TLS wrap → Internet
```

### 5.2 Container-to-Container Traffic (Internal)

```
Container A (10.20.0.10)
        │
        │ TCP 50052 (gRPC)
        ▼
    Container Bridge (10.20.0.0/16)
        │
        │ No firewall re-inspection (same bridge)
        ▼
    Container B (10.20.0.20)
        │
        ▼ Response (mTLS encrypted)
    Container A
```

### 5.3 East-West Traffic (HOST-A ↔ HOST-B via WireGuard)

```
HOST-A Container A (10.20.0.10)
        │
        │ TCP 50051 (gRPC to Accounting)
        │ on HOST-B
        ▼
    HOST-A Firewall (OPNsense)
        │
        │ Match rule: "Allow gRPC within LAN"
        │ No action (same LAN)
        ▼
    WireGuard Tunnel (UDP 51820)
        │ Encrypted with Curve25519
        │ IPv6: fd00:dead:beef::1 ↔ fd00:dead:beef::2
        ▼
    HOST-B Firewall (IPFire)
        │
        │ Match rule: "Allow gRPC within LAN"
        ▼
    HOST-B Container B (10.20.0.30)
        │
        ▼ Response → reverse path
    HOST-A Container A
```

---

## 6. Monitoring and Logging

### 6.1 Firewall Log Parsing

**OPNsense (pf logs)**:
```bash
tcpdump -e -ttt -r /var/log/pf.log | \
  grep -E "50051|50067|8080|8443" | \
  grep -v "pass"  # Show only blocked traffic
```

**IPFire (syslog)**:
```bash
tail -f /var/log/messages | \
  grep -E "INPUT DROP|FORWARD DROP|50051|8080"
```

### 6.2 Real-Time Port Monitoring

```bash
# Monitor all connections on exposed ports
netstat -an | grep -E ":80|:443|:51820" | sort | uniq -c

# Or use ss (newer)
ss -an | grep -E ":80|:443|:51820"

# Watch for connection state changes
watch -n 1 'netstat -an | grep -E ":8080|:50051"'
```

### 6.3 Anamnesis Integration

All firewall logs are exported to Anamnesis SIEM for:
- Compliance audits (SOC2, PCI-DSS, HIPAA)
- Threat detection
- Performance analytics
- Historical retention (7 years)

---

## 7. Testing Checklist

- [ ] **Port 80 reachable**: `curl -v http://example.com:80`
- [ ] **Port 443 reachable**: `curl -v https://example.com:443`
- [ ] **WireGuard endpoint reachable**: `timeout 5 nc -zv <WAN_IP> 51820`
- [ ] **gRPC blocked from WAN**: `timeout 5 nc -zv <WAN_IP> 50051` should timeout/refuse
- [ ] **Dashboard blocked from WAN**: `timeout 5 nc -zv <WAN_IP> 8080` should timeout/refuse
- [ ] **Internal gRPC works**: Container A → Container B on port 50051 succeeds
- [ ] **DNS resolves**: `nslookup example.com 10.20.0.1` succeeds
- [ ] **mDNS discovery works**: `avahi-browse -a` shows services
- [ ] **WireGuard tunnel active**: `ip link show wg0` shows state UP
- [ ] **Monad HbH passing**: `tcpdump -i eth0 'ip6 proto 0'` captures packets

---

## 8. Port Allocation for Future Services

Reserved ranges for expansion:

| Range | Use | Status |
|-------|-----|--------|
| 50051–50067 | gRPC services | In Use (17/17 services) |
| 50068–50099 | Reserved gRPC | Available (32 ports) |
| 8000–8099 | HTTP services | Available |
| 5000–5099 | Database/cache | Available |
| 6000–6099 | Message queues | Available |

---

## 9. Regulatory Compliance Matrix

| Regulation | Requirement | Implementation | Verification |
|-----------|-------------|-----------------|---------------|
| **SOC2 Type II** | Firewall rule change log | Git + Anamnesis | `git log docs/network/` |
| **PCI-DSS 6.6** | Annual review of rules | Audit trail | Anamnesis historical report |
| **HIPAA Security Rule** | Encryption in transit | TLS 1.3 on 443 | `nmap --script ssl-enum-ciphers` |
| **ISO 27001** | Access control | mTLS on gRPC | Certificate verification |

---

**Document Version**: 1.0  
**Last Updated**: 2026-02-26  
**Maintained By**: Unheaded Development Team  
**License**: MIT
