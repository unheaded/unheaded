# Monad Protocol IPv6 Hop-by-Hop Firewall Configuration

## Overview

This document provides detailed, platform-specific firewall rules to ensure that **Monad Protocol IPv6 Hop-by-Hop (HbH) extension headers are passed through without stripping, rewriting, or filtering**.

The Monad protocol embeds a **20-byte register** (MONAD_METRIC_V1, option type 0x1E) in IPv6 HbH headers. If firewalls drop or modify HbH headers, the entire Monad observability and consensus system fails.

---

## 1. Quick Reference Table

| Firewall | IPv6 Proto | Rule Name | Action | Priority |
|----------|-----------|-----------|--------|----------|
| **OPNsense (pf)** | 0x00 (HOPOPT) | `pass inet6 exthdrs hbh` | PASS | High |
| **IPFire (nftables)** | 0 (nexthdr 0) | `ip6 nexthdr 0 accept` | ACCEPT | High |
| **Linux iptables** | 0 (ipv6-opts) | `-p ipv6-opts` | ACCEPT | High |
| **Windows Defender FW** | IPv6 + Extension Header | Allow custom rule | Allow | High |
| **pfSense (pf-compatible)** | 0x00 (HOPOPT) | `pass inet6 exthdrs hbh` | PASS | High |

---

## 2. OPNsense (FreeBSD/pf) Configuration

### 2.1 Complete pf.conf Rules

Add the following to `/etc/pf.conf` on the OPNsense VM:

```pf
# ============================================================================
# MONAD PROTOCOL IPv6 HOP-BY-HOP EXTENSION HEADERS
# ============================================================================
# Option Type: 0x1E (Unheaded MONAD_METRIC_V1)
# IPv6 Next-Header: 0x00 (HOPOPT)
# Behavior: PASS through without modification
# ============================================================================

# CRITICAL: Disable IPv6 scrubbing that rewrites headers
set skip on lo0
set optimization aggressive
set reassemble no
set ipopts deny

# Allow IPv6 HbH on all interfaces (WAN and LAN)
pass in quick inet6 proto ipv6-opts from any to any
pass out quick inet6 proto ipv6-opts from any to any

# Explicit HbH extension headers (redundant but emphatic)
pass in quick inet6 exthdrs hbh from any to any \
    comment "Monad: IPv6 HbH extension headers ingress"
pass out quick inet6 exthdrs hbh from any to any \
    comment "Monad: IPv6 HbH extension headers egress"

# Allow IPv6 fragmentation (required for HbH + large payloads)
pass inet6 proto ipv6-frag from any to any \
    comment "Allow IPv6 fragmentation"

# ============================================================================
# REST OF FIREWALL RULES (order matters!)
# ============================================================================

# Connection tracking (established/related)
pass in quick on $wan_if inet proto tcp from any to any \
    flags S/SA modulate state comment "Allow TCP established"
pass in quick on $wan_if inet proto udp from any to any \
    keep state comment "Allow UDP established"
pass in quick on $wan_if inet proto icmp \
    keep state comment "Allow ICMP"

# ICMP and ICMPv6 (required for path MTU, neighbor discovery)
pass quick inet proto icmp all \
    comment "Allow ICMP echo/unreachable"
pass quick inet6 proto icmp6 all \
    comment "Allow ICMPv6 (neighbor discovery, path MTU)"

# Exposed ports from Kingdom Doom range
pass in on $wan_if inet proto tcp to ($wan_ip) port 80 \
    rdr-to 10.20.0.1 port 80 comment "HTTP redirect"
pass in on $wan_if inet proto tcp to ($wan_ip) port 443 \
    rdr-to 10.20.0.1 port 443 comment "HTTPS gateway"
pass in on $wan_if inet proto udp to ($wan_ip) port 51820 \
    comment "WireGuard east-west VPN"

# Allow DNS from containers to firewall
pass in on $lan_if inet proto tcp from 10.20.0.0/16 to 10.20.0.1 port 53 \
    comment "DNS TCP from containers"
pass in on $lan_if inet proto udp from 10.20.0.0/16 to 10.20.0.1 port 53 \
    comment "DNS UDP from containers"

# Allow DNS from containers IPv6
pass in on $lan_if inet6 proto tcp from fd00:dead:beef:1::/64 \
    to fd00:dead:beef:1::1 port 53 comment "DNS TCP IPv6"
pass in on $lan_if inet6 proto udp from fd00:dead:beef:1::/64 \
    to fd00:dead:beef:1::1 port 53 comment "DNS UDP IPv6"

# Allow WireGuard tunnel endpoint (IPv6)
pass in quick inet6 proto udp to fd00:dead:beef:1::1 port 51820 \
    comment "WireGuard IPv6 endpoint"

# Block internal gRPC ports from WAN
block in quick on $wan_if inet proto tcp to ($wan_ip) port 50051:50067 \
    comment "Block internal gRPC from WAN"
block in quick on $wan_if inet6 proto tcp to ($wan_ipv6) port 50051:50067 \
    comment "Block internal gRPC from WAN (IPv6)"

# Block dashboard from WAN
block in quick on $wan_if inet proto tcp to ($wan_ip) port 8080 \
    comment "Block dashboard from WAN"
block in quick on $wan_if inet proto tcp to ($wan_ip) port 8443 \
    comment "Block dashboard HTTPS from WAN"

# Antispoofing (block RFC1918 from WAN)
block in quick on $wan_if inet from 10.0.0.0/8 to any \
    comment "Block RFC1918 spoofing (10.0.0.0/8)"
block in quick on $wan_if inet from 172.16.0.0/12 to any \
    comment "Block RFC1918 spoofing (172.16.0.0/12)"
block in quick on $wan_if inet from 192.168.0.0/16 to any \
    comment "Block RFC1918 spoofing (192.168.0.0/16)"

# Block bogons
block in quick on $wan_if inet from 0.0.0.0/8 to any \
    comment "Block bogon 0.0.0.0/8"
block in quick on $wan_if inet from 127.0.0.0/8 to any \
    comment "Block bogon 127.0.0.0/8"
block in quick on $wan_if inet from 224.0.0.0/4 to any \
    comment "Block bogon multicast"
block in quick on $wan_if inet from 240.0.0.0/4 to any \
    comment "Block bogon 240.0.0.0/4"

# Default deny all else (implicit)
# block in on $wan_if all
```

### 2.2 OPNsense Web UI Step-by-Step

#### Step 1: Add WAN Rule for HbH

1. **Navigate**: **Firewall → Rules → WAN**
2. Click **"Add"** (top-right, upward arrow)
3. Fill in the form:
   - **Interface**: WAN
   - **Direction**: In
   - **Action**: Pass
   - **Quick**: Checked
   - **Protocol**: IPv6
   - **Source**: Single host or Network → **any**
   - **Destination**: Single host or Network → **any**
   - **Description**: "Allow IPv6 HbH extension headers (Monad)"
   - **Advanced Options → Next Header**: **0** (HOPOPT)
4. Click **"Save"**
5. Click **"Apply"** at the top of the rules list

#### Step 2: Add LAN Rule for HbH

Repeat Step 1 but:
- **Navigate**: **Firewall → Rules → LAN**
- **Direction**: In or Out (recommend both)

#### Step 3: Disable IPv6 Scrubbing

1. **Navigate**: **System → Advanced → Firewall**
2. Uncheck all of:
   - "Enable IPv6 Reassembly"
   - "IPv6 Reassembly"
   - "IPv4 mapped IPv6 reassembly"
3. Click **"Save"**
4. System will reload firewall rules

#### Step 4: Verify HbH Rules Loaded

1. **Navigate**: **Diagnostics → Shell**
2. Run:
   ```bash
   pfctl -s rules | grep -i "hopopt\|exthdrs hbh\|proto 0"
   ```
3. Expected output (should show your HbH rules):
   ```
   pass in quick on em0 inet6 proto ipv6-opts from any to any
   pass in quick on em1 inet6 proto ipv6-opts from any to any
   pass out quick on em1 inet6 proto ipv6-opts from any to any
   ```

#### Step 5: Monitor Firewall Logs

1. **Navigate**: **System → Logs → Firewall**
2. **Filter**: Search for packets with IPv6 and `proto 0` or `hopopt`
3. **Expected**: Should NOT see these packets being dropped
4. **If blocked**: Check "Next Header: 0" rule is above any deny rules

### 2.3 Troubleshooting OPNsense HbH

| Problem | Diagnosis | Solution |
|---------|-----------|----------|
| HbH packets dropped | `pfctl -s rules` doesn't show HbH rule | Re-add via GUI, ensure "Quick" is checked |
| Monad checksums fail | `tcpdump -i em0 'ip6 proto 0'` shows packets but mangled | Disable IPv6 scrubbing in System → Advanced |
| "proto 0" in logs | Firewall logs show dropped IPv6 proto 0 | Move HbH rule above deny rules (use Quick flag) |
| pf reload fails | Running `pfctl -f /etc/pf.conf` errors | Check pf.conf syntax: `pfctl -n -f /etc/pf.conf` |

---

## 3. IPFire (Linux/nftables) Configuration

### 3.1 Complete nftables Ruleset

Create or edit `/etc/nftables.conf` on the IPFire VM:

```nftables
#!/usr/sbin/nft -f

# ============================================================================
# MONAD PROTOCOL IPv6 HOP-BY-HOP EXTENSION HEADERS (IPFire / nftables)
# ============================================================================
# Option Type: 0x1E (Unheaded MONAD_METRIC_V1)
# IPv6 Next-Header: 0 (HOPOPT)
# Behavior: ACCEPT (pass through without modification)
# ============================================================================

flush ruleset

table inet filter {
  chain input {
    type filter hook input priority 0; policy drop;

    # Allow loopback
    iif lo accept

    # Connection tracking
    ct state established,related accept

    # CRITICAL: Allow IPv6 HbH (Monad)
    # This MUST come before any deny rules
    ip6 nexthdr 0 accept comment "IPv6 Hop-by-Hop (Monad MONAD_METRIC_V1)"

    # Allow ICMPv4 (ping)
    ip protocol icmp accept comment "ICMPv4"

    # Allow ICMPv6 (neighbor discovery, path MTU)
    ip6 nexthdr icmpv6 accept comment "ICMPv6"

    # Allow from LAN (green interface)
    iif "green0" accept comment "Allow all from LAN"

    # Allow WireGuard
    iif "wlan0" udp dport 51820 accept comment "WireGuard VPN"
    iif "eth0" udp dport 51820 accept comment "WireGuard from WAN"

    # Allow WAN HTTPS and HTTP
    iif "eth0" tcp dport { 80, 443 } accept comment "HTTP/HTTPS"

    # Allow DNS from containers
    iif "docker0" tcp dport 53 accept comment "DNS TCP from containers"
    iif "docker0" udp dport 53 accept comment "DNS UDP from containers"

    # Block internal services from WAN
    iif "eth0" tcp dport 50051:50067 drop comment "Block internal gRPC"
    iif "eth0" tcp dport 8080 drop comment "Block dashboard"
    iif "eth0" tcp dport 8443 drop comment "Block dashboard HTTPS"

    # Deny spoofed RFC1918 from WAN
    iif "eth0" ip saddr 10.0.0.0/8 drop comment "Block RFC1918 spoofing (10.0.0.0/8)"
    iif "eth0" ip saddr 172.16.0.0/12 drop comment "Block RFC1918 spoofing (172.16.0.0/12)"
    iif "eth0" ip saddr 192.168.0.0/16 drop comment "Block RFC1918 spoofing (192.168.0.0/16)"

    # Deny bogons from WAN
    iif "eth0" ip saddr 0.0.0.0/8 drop comment "Block bogon 0.0.0.0/8"
    iif "eth0" ip saddr 127.0.0.0/8 drop comment "Block bogon 127.0.0.0/8"
    iif "eth0" ip saddr 224.0.0.0/4 drop comment "Block bogon multicast"
    iif "eth0" ip saddr 240.0.0.0/4 drop comment "Block bogon 240.0.0.0/4"

    # Log and drop all else
    limit rate 1/minute counter log prefix "INPUT DROP: " drop
  }

  chain forward {
    type filter hook forward priority 0; policy drop;

    # CRITICAL: Allow IPv6 HbH (Monad)
    # This MUST come before any deny rules
    ip6 nexthdr 0 accept comment "IPv6 Hop-by-Hop (Monad) forward"

    # Connection tracking
    ct state established,related accept

    # Allow from LAN to any
    iif "green0" accept comment "Allow all from LAN forward"

    # Allow ICMPv6
    ip6 nexthdr icmpv6 accept comment "ICMPv6 forward"

    # Deny spoofed RFC1918 from WAN
    iif "eth0" ip saddr 10.0.0.0/8 drop comment "Block RFC1918 spoofing forward"
    iif "eth0" ip saddr 172.16.0.0/12 drop comment "Block RFC1918 spoofing forward"
    iif "eth0" ip saddr 192.168.0.0/16 drop comment "Block RFC1918 spoofing forward"

    # Log and drop
    limit rate 1/minute counter log prefix "FORWARD DROP: " drop
  }

  chain output {
    type filter hook output priority 0; policy accept;

    # CRITICAL: Allow IPv6 HbH (Monad)
    # Usually output is accept by default, but be explicit
    ip6 nexthdr 0 accept comment "IPv6 Hop-by-Hop (Monad) output"
  }
}

# ============================================================================
# NAT RULES (optional, for SNAT/DNAT)
# ============================================================================

table nat {
  chain postrouting {
    type nat hook postrouting priority 100; policy accept;

    # SNAT outbound from containers
    oif "eth0" ip saddr 10.20.0.0/16 snat to 192.168.1.100 \
        comment "SNAT containers to WAN IP"
  }
}
```

### 3.2 IPFire Web UI Step-by-Step

#### Step 1: Log In to IPFire Web UI

1. Open browser: **https://192.168.1.1:444** (adjust IP to your IPFire)
2. Login: **admin** / **password** (or custom credentials set at install)
3. Click **"Firewall"** in left menu

#### Step 2: Add Firewall Rule for HbH (Incoming)

1. **Firewall → Firewall Rules**
2. Click **"Add"** under "Incoming Traffic"
3. Fill in:
   - **Source**: Green (LAN)
   - **Destination**: Any
   - **Protocol**: IPv6 (or "IPv6 Extension Headers" if available)
   - **Next Header** (if available): 0 (HOPOPT)
   - **Action**: ACCEPT
   - **Enable/Activate**: Checked
4. Click **"Add"**

#### Step 3: Add Firewall Rule for HbH (Forward)

1. **Firewall → Firewall Rules**
2. Click **"Add"** under "Forward Traffic"
3. Fill in:
   - **Source**: Green (LAN)
   - **Destination**: Any
   - **Protocol**: IPv6
   - **Next Header**: 0
   - **Action**: ACCEPT
4. Click **"Add"**

#### Step 4: Verify Rules via CLI

SSH into IPFire and run:

```bash
nft list ruleset | grep -E "nexthdr 0|HOPOPT|Monad"
```

Expected output:
```
ip6 nexthdr 0 accept comment "IPv6 Hop-by-Hop (Monad MONAD_METRIC_V1)"
ip6 nexthdr 0 accept comment "IPv6 Hop-by-Hop (Monad) forward"
```

#### Step 5: Monitor Logs

1. **Firewall → Firewall Logs**
2. Check for IPv6 packets with "nexthdr 0" being accepted
3. Should NOT see them being dropped

### 3.3 Troubleshooting IPFire HbH

| Problem | Diagnosis | Solution |
|---------|-----------|----------|
| HbH packets dropped | `nft list ruleset` doesn't show `nexthdr 0` | Add rule via Web UI or edit `/etc/nftables.conf` |
| nftables won't reload | Command `nft -f /etc/nftables.conf` errors | Check syntax: `nft -f - <<< "$(cat /etc/nftables.conf)"` |
| Monad metrics missing | `tcpdump -i eth0 'ip6 proto 0'` shows packets but Monad metrics field empty | Verify rule order; HbH accept must come before any drop |
| Rule not persistent | Rules disappear after reboot | Edit `/etc/nftables.conf` and reload on boot |

---

## 4. Linux iptables/iptables-nft (Generic Linux)

For deployments using standard Linux firewall instead of OPNsense/IPFire:

### 4.1 iptables Rules

```bash
#!/bin/bash
# Add to /etc/iptables/rules.v6 or use iptables-restore

# Allow IPv6 HbH extension headers (Monad)
ip6tables -A INPUT -p ipv6-opts -j ACCEPT -m comment --comment "IPv6 HbH (Monad)"
ip6tables -A FORWARD -p ipv6-opts -j ACCEPT -m comment --comment "IPv6 HbH forward (Monad)"
ip6tables -A OUTPUT -p ipv6-opts -j ACCEPT -m comment --comment "IPv6 HbH output (Monad)"

# Allow ICMPv6 (neighbor discovery)
ip6tables -A INPUT -p icmpv6 -j ACCEPT
ip6tables -A FORWARD -p icmpv6 -j ACCEPT
ip6tables -A OUTPUT -p icmpv6 -j ACCEPT

# WireGuard
ip6tables -A INPUT -p udp --dport 51820 -j ACCEPT
ip6tables -A INPUT -p tcp --dport 443 -j ACCEPT
ip6tables -A INPUT -p tcp --dport 80 -j ACCEPT

# Block internal gRPC from WAN
ip6tables -A INPUT -p tcp --dport 50051:50067 -i eth0 -j DROP

# Default policy
ip6tables -P INPUT DROP
ip6tables -P FORWARD DROP
ip6tables -P OUTPUT ACCEPT
```

### 4.2 systemd-nftables

For systems using systemd-nftables (NFT wrapper):

```bash
# Load nftables rules on boot
systemctl enable nftables
systemctl start nftables

# Verify rules
systemctl status nftables
nftctl list ruleset  # or: nft list ruleset
```

---

## 5. Testing and Verification

### 5.1 tcpdump Test for HbH Passthrough

#### Capture Monad HbH Packets

On the firewall or a monitoring interface, run:

```bash
# Capture IPv6 packets with next-header 0 (HbH)
tcpdump -i eth0 -n 'ip6 proto 0' -v

# Expected output:
# 14:32:45.123456 IP6 fd00:dead:beef:1::10 > fd00:dead:beef:2::10: \
#     HBH hopopt len 24 (0x18): option pad1: \
#     IPv6 extension header dump...
```

#### Create Test Packet with HbH

Use scapy (Python) to craft and send a test Monad packet:

```python
#!/usr/bin/env python3
from scapy.all import IPv6, IPv6ExtHdrHopByHop, Raw, send, get_if_hwaddr
import time

# Simulate Monad MONAD_METRIC_V1 in HbH
payload = bytes.fromhex(
    "1e180000"  # Option type 0x1E (Monad), length 0x18
    + "0102030405060708090a0b0c0d0e0f1011121314"  # 20-byte register
)

pkt = IPv6(dst="fd00:dead:beef:2::10", src="fd00:dead:beef:1::10") / \
      IPv6ExtHdrHopByHop(options=[Raw(load=payload)]) / \
      Raw(load=b"Monad test payload")

print("[*] Sending Monad HbH test packet...")
send(pkt, verbose=True)
print("[+] Packet sent!")
```

Run from a test container:
```bash
python3 send_monad_test.py

# On firewall, capture with:
tcpdump -i eth0 -n 'ip6 proto 0' -v
```

### 5.2 Monad-Specific Verification

#### Check if Monad Metrics are Captured

After running a Monad service through the firewall, verify metrics in Anamnesis:

```bash
# Query Anamnesis for Monad packets with HbH metrics
curl http://anamnesis:8080/api/metrics?filter=monad_hbh

# Expected response (JSON):
{
  "packets_with_hbh": 1024,
  "monad_metrics_v1_count": 1024,
  "monad_checksum_failures": 0,  # Should be 0 if HbH is passing
  "average_path_latency_us": 450
}
```

#### Verify in Firewall Logs

**OPNsense**:
```bash
tail -f /var/log/pf.log | grep -i "proto.*0\|hopopt"
```

**IPFire**:
```bash
tail -f /var/log/messages | grep -i "nexthdr 0\|hopopt"
```

Should see PASS/ACCEPT entries, not DROP.

### 5.3 Integration Test Script

Complete end-to-end verification:

```bash
#!/bin/bash
set -e

echo "[*] Testing Monad HbH passthrough..."

# Test 1: Check firewall rules
echo "[1] Checking firewall rules..."
if command -v pfctl &> /dev/null; then
    echo "    [OPNsense] Rules:"
    pfctl -s rules | grep -i "hopopt\|exthdrs hbh" && echo "    [OK] HbH rules loaded" || echo "    [FAIL] HbH rules missing"
elif command -v nft &> /dev/null; then
    echo "    [IPFire] Rules:"
    nft list ruleset | grep "nexthdr 0" && echo "    [OK] HbH rules loaded" || echo "    [FAIL] HbH rules missing"
fi

# Test 2: Verify IPv6 connectivity
echo "[2] Testing IPv6 connectivity..."
ping6 -c 1 fd00:dead:beef:2::1 && echo "    [OK] IPv6 reachable" || echo "    [FAIL] IPv6 unreachable"

# Test 3: Capture HbH packets
echo "[3] Capturing HbH packets (5 seconds)..."
timeout 5 tcpdump -i eth0 'ip6 proto 0' -c 10 -Q in 2>/dev/null && echo "    [OK] HbH packets captured" || echo "    [INFO] No HbH packets captured (may be normal if no Monad traffic)"

# Test 4: Verify port filtering
echo "[4] Checking port filtering..."
timeout 1 nc -zv 10.20.0.1 443 2>&1 | grep -q "succeeded" && echo "    [OK] HTTPS allowed" || echo "    [FAIL] HTTPS blocked"
timeout 1 nc -zv 10.20.0.1 8080 2>&1 | grep -q "refused\|timeout" && echo "    [OK] Dashboard blocked" || echo "    [FAIL] Dashboard allowed (should be blocked)"

echo "[+] Verification complete!"
```

Run:
```bash
bash verify_hbh.sh
```

---

## 6. Common Mistakes and Troubleshooting

### 6.1 Monad Checksums Fail

**Symptom**: Monad protocol reports checksum errors, metrics are dropped

**Root Cause**: HbH headers are being rewritten or stripped

**Fix**:
- **OPNsense**: Disable IPv6 scrubbing (System → Advanced → Firewall)
- **IPFire**: Ensure `nft` rule comes before any drop/reject rules
- **Linux**: Check `sysctl` settings:
  ```bash
  sysctl -a | grep ipv6
  sysctl -w net.ipv6.conf.all.disable_ipv6=0
  ```

### 6.2 eBPF Metrics Disappear

**Symptom**: Monad packets are transmitted but no metrics arrive

**Root Cause**: HbH extension headers are dropped silently

**Fix**:
1. Verify rule is in firewall: `tcpdump -i eth0 'ip6 proto 0'`
2. Check firewall logs for dropped IPv6 proto 0
3. Ensure HbH rule has HIGH priority (Quick flag in pf, comes before drops in nftables)

### 6.3 "Path MTU Discovery Broken"

**Symptom**: Large Monad packets fragmented, reassembly timeout

**Root Cause**: IPv6 fragmentation blocked or MTU miscalculated

**Fix**:
- Allow IPv6 fragmentation (proto 44) in firewall
- Set correct WireGuard MTU: 1380 for IPv6 HbH overhead
- Test: `ping -s 1400 fd00:dead:beef:2::1` should fragment gracefully

### 6.4 Firewall Rule Reload Fails

**Symptom**: `pfctl` or `nft` commands error

**Fix**:
- **OPNsense**: Check pf.conf syntax:
  ```bash
  pfctl -n -f /etc/pf.conf
  ```
- **IPFire**: Validate nftables:
  ```bash
  nft -f - <<< "$(cat /etc/nftables.conf)"
  ```
- **Linux**: Dry-run iptables:
  ```bash
  iptables-restore -t < rules.v6
  ```

---

## 7. Performance and Logging Considerations

### 7.1 Logging HbH Packets

To debug Monad traffic, add logging rules:

**OPNsense (pf.conf)**:
```pf
pass in quick on $lan_if inet6 proto ipv6-opts all \
    log (all) comment "Log HbH packets"
```

Then view logs:
```bash
tcpdump -e -ttt -r /var/log/pf.log | grep -i "hopopt"
```

**IPFire (nftables)**:
```nftables
ip6 nexthdr 0 log prefix "MONAD_HBH: " accept
```

View logs:
```bash
tail -f /var/log/messages | grep MONAD_HBH
```

### 7.2 Performance Impact

HbH passthrough has **minimal overhead**:
- No deep packet inspection (DPI) needed
- Rule matches IPv6 header field (O(1) lookup)
- No reassembly required
- Estimated CPU impact: <0.1%

### 7.3 Audit Trail

For SOC2/HIPAA compliance, ensure logs include:
- Timestamp
- Source/destination IPv6 address
- HbH option type (0x1E for Monad)
- Action (PASS/ACCEPT)
- Firewall device

Example log format (CEF/syslog):
```
Feb 26 14:32:45 opnsense pf[1234]: pass in on em0: IPv6 > fd00:dead:beef:2::10 > fd00:dead:beef:1::10 proto 0 (hopopt) action=pass
```

---

## 8. Regulatory Compliance

### 8.1 SOC2 Type II

- Firewall rules must be version-controlled (Git)
- All rule changes logged to Anamnesis
- Monthly audit of rule effectiveness
- Incident response playbooks for dropped Monad packets

### 8.2 PCI-DSS 6.6

- Firewall configurations reviewed annually
- Change management process for rule modifications
- Segregation of duties (admin must approve rule changes)
- Encrypted backups of firewall configs

### 8.3 HIPAA Security Rule

- Audit trails of all firewall rule changes
- Encryption of logs in transit (TLS to Anamnesis)
- Access controls to firewall admin interfaces
- Disaster recovery testing quarterly

---

## 9. Migration Checklist

When deploying new firewall or replacing existing:

- [ ] Backup current firewall configuration
- [ ] Test HbH rules in staging environment
- [ ] Verify Monad metrics flow after deployment
- [ ] Check for packet loss (tcpdump on both sides)
- [ ] Validate all exposed ports still accessible (443, 80, 51820)
- [ ] Confirm internal services remain unreachable from WAN (50051:50067, 8080)
- [ ] Audit firewall logs for 24 hours
- [ ] Update documentation with new firewall IP/hostname
- [ ] Train ops team on new platform

---

## 10. Additional Resources

- **IPv6 HbH RFC**: [RFC 8200 - IPv6 Specification](https://tools.ietf.org/html/rfc8200)
- **Monad Protocol Spec**: See `docs/protocol/MONAD_SPEC.md`
- **OPNsense Docs**: https://docs.opnsense.org/
- **IPFire Docs**: https://wiki.ipfire.org/
- **Unheaded Network Architecture**: See `docs/network/FIREWALL_TOPOLOGY.md`

---

**Document Version**: 1.0  
**Last Updated**: 2026-02-26  
**Maintained By**: Unheaded Development Team  
**License**: MIT
