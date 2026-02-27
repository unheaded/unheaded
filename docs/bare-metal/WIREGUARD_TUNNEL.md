# PHASE 12B: WIREGUARD TUNNEL CONFIGURATION RUNBOOK

**Estimated Total Time: ~15-20 min**
**Last Updated: 2026-02-27**
**Status: PRODUCTION-READY**
**Prerequisites: Both Host-A and Host-B must be at end of PHASE 6 (services running, before cross-host validation)**

---

## OVERVIEW: WireGuard Role in Unheaded

WireGuard provides:

| Aspect | Details |
|--------|---------|
| **Purpose** | East-west overlay tunnel between Host-A and Host-B |
| **Encryption** | Curve25519 (elliptic curve) + ChaCha20-Poly1305 |
| **MTU** | 1420 bytes (account for 80-byte IPv6 HbH + VXLAN overhead) |
| **Key Exchange** | Manual pre-shared keys (no IKE complexity) |
| **Routing** | iBGP EVPN over fd00:dead:beef::/48 ULA |
| **Keepalive** | BFD with 300ms intervals (FRR on host-a, BIRD will integrate) |
| **Security** | Pre-shared keys + endpoint IP restrictions |

---

## SECTION I: KEY GENERATION & MANAGEMENT

### Step 1.1: Generate Keys on Admin Workstation

```bash
# Install WireGuard tools (on admin workstation, if not present)
sudo apt-get install -y wireguard-tools

# Generate Host-A keys
mkdir -p /tmp/wireguard-keys
cd /tmp/wireguard-keys

# Host-A (The Forge)
wg genkey | tee host-a-private.key | wg pubkey > host-a-public.key

# Host-B (The Outpost)
wg genkey | tee host-b-private.key | wg pubkey > host-b-public.key

# Pre-shared key (for additional security, optional but recommended)
wg genpsk > preshared.key

# Verify key files
ls -la /tmp/wireguard-keys/
# Expected:
# -rw-r--r-- host-a-private.key
# -rw-r--r-- host-a-public.key
# -rw-r--r-- host-b-private.key
# -rw-r--r-- host-b-public.key
# -rw-r--r-- preshared.key

# Display keys for copying
echo "=== HOST-A PRIVATE KEY ==="
cat /tmp/wireguard-keys/host-a-private.key
echo ""
echo "=== HOST-B PRIVATE KEY ==="
cat /tmp/wireguard-keys/host-b-private.key
echo ""
echo "=== HOST-A PUBLIC KEY ==="
cat /tmp/wireguard-keys/host-a-public.key
echo ""
echo "=== HOST-B PUBLIC KEY ==="
cat /tmp/wireguard-keys/host-b-public.key
echo ""
echo "=== PRE-SHARED KEY ==="
cat /tmp/wireguard-keys/preshared.key
```

### Step 1.2: Secure Key Distribution

**CRITICAL: Protect these keys. Never commit to git without encryption.**

```bash
# Option 1: Secure copy to both hosts (via SSH)
scp /tmp/wireguard-keys/host-a-private.key root@host-a:/tmp/wg-private.key
scp /tmp/wireguard-keys/host-b-private.key root@host-b:/tmp/wg-private.key
scp /tmp/wireguard-keys/preshared.key root@host-a:/tmp/preshared.key
scp /tmp/wireguard-keys/preshared.key root@host-b:/tmp/preshared.key

# Option 2: Encrypted transmission via gpg
# (encrypt keys, send to admins, decrypt on each host)

# After copying, verify permissions and delete from admin workstation
chmod 600 /tmp/wireguard-keys/*
rm -rf /tmp/wireguard-keys/
```

---

## SECTION II: INTERFACE CONFIGURATION

### Host-A (The Forge) — WireGuard Server/Listen

```bash
# SSH to host-a
ssh root@host-a

# Create WireGuard interface
sudo ip link add wg0 type wireguard

# Set private key
sudo ip link set wg0 type wireguard private-key </tmp/wg-private.key

# Assign IPv6 ULA address (server)
sudo ip addr add fd00:dead:beef::1/48 dev wg0

# Set listening port (can be static)
sudo ip link set wg0 type wireguard listen-port 51820

# Add peer (Host-B)
HOST_B_PUBLIC_KEY=$(cat /tmp/wg-private.key | wg pubkey)  # This is HOST-A's public key
HOST_B_PUBLIC_KEY=$(ssh root@host-b "cat /tmp/wg-private.key | wg pubkey")  # Get HOST-B's actual public key

sudo wg set wg0 peer "$HOST_B_PUBLIC_KEY" \
  allowed-ips fd00:dead:beef::2/128 \
  preshared-key </tmp/preshared.key \
  persistent-keepalive 25

# Bring up interface
sudo ip link set wg0 up

# Verify
ip addr show wg0
# Expected:
# wg0: <POINTOPOINT,NOARP,UP,...>
#     inet6 fd00:dead:beef::1/48 scope global

# Check WireGuard status
sudo wg show
# Expected:
# interface: wg0
#   public key: [KEY]
#   private key: (hidden)
#   listen port: 51820
#   peer: [HOST-B-PUBLIC-KEY]
#     endpoint: <host-b-wg-endpoint>:51820
#     allowed ips: fd00:dead:beef::2/128
#     latest handshake: [timestamp]
#     transfer: [bytes]
```

### Host-B (The Outpost) — WireGuard Client

```bash
# SSH to host-b
ssh root@host-b

# Create WireGuard interface
sudo ip link add wg0 type wireguard

# Set private key
sudo ip link set wg0 type wireguard private-key </tmp/wg-private.key

# Assign IPv6 ULA address (client)
sudo ip addr add fd00:dead:beef::2/48 dev wg0

# Set listening port (for fallback/redundancy)
sudo ip link set wg0 type wireguard listen-port 51820

# Get Host-A public IP (WAN IP from DHCP)
HOST_A_ENDPOINT=$(host host-a.example.com | grep -oE '\b([0-9]{1,3}\.){3}[0-9]{1,3}\b' | head -1)
# OR find it from host-a
HOST_A_ENDPOINT=$(ssh root@host-a "ip route show | grep 'via' | awk '{print \$3}' | head -1")

# Get Host-A's public key
HOST_A_PUBLIC_KEY=$(scp root@host-a:/tmp/wg-private.key /tmp/host-a-private.key >/dev/null 2>&1 && cat /tmp/host-a-private.key | wg pubkey)
# OR retrieve it another way
HOST_A_PUBLIC_KEY=$(ssh root@host-a "sudo wg show | grep 'public key' | awk '{print \$3}'")

# Add peer (Host-A)
sudo wg set wg0 peer "$HOST_A_PUBLIC_KEY" \
  endpoint "$HOST_A_ENDPOINT:51820" \
  allowed-ips fd00:dead:beef::1/128 \
  preshared-key </tmp/preshared.key \
  persistent-keepalive 25

# Bring up interface
sudo ip link set wg0 up

# Verify
ip addr show wg0
# Expected: wg0 with fd00:dead:beef::2/48

# Test tunnel connectivity
ping6 -c 3 fd00:dead:beef::1
# Expected: 3 packets transmitted, 3 received
# (If fails, check firewall rules on host-a OPNsense: UDP 51820 must be allowed)
```

---

## SECTION III: INTERFACE CONFIGURATION (PERSISTENT VIA NIXOS)

For production, configure WireGuard in NixOS so it persists across reboots.

### Host-A NixOS Configuration

Edit `/etc/nixos/configuration.nix` (or flake module):

```nix
# nixos/hosts/host-a/configuration.nix
{ config, pkgs, lib, ... }:

{
  # ... other config ...

  # WireGuard interface (persistent)
  networking.wireguard.interfaces.wg0 = {
    ips = [ "fd00:dead:beef::1/48" ];
    listenPort = 51820;
    peers = [
      {
        # Host-B peer
        publicKey = "HOST_B_PUBLIC_KEY_HERE";
        allowedIPs = [ "fd00:dead:beef::2/128" ];
        presharedKey = "PRE_SHARED_KEY_HERE";
        persistentKeepalive = 25;
      }
    ];
    privateKeyFile = "/etc/wireguard/private.key";  # Should be readable only by root
  };

  # Ensure private key exists
  system.activationScripts = {
    wireguardKey = {
      text = ''
        mkdir -p /etc/wireguard
        chmod 700 /etc/wireguard
        if [ ! -f /etc/wireguard/private.key ]; then
          ${pkgs.wireguard-tools}/bin/wg genkey > /etc/wireguard/private.key
          chmod 600 /etc/wireguard/private.key
        fi
      '';
      deps = [];
    };
  };

  # IPv6 forwarding (for VXLAN traffic)
  boot.kernel.sysctl."net.ipv6.conf.all.forwarding" = 1;
  boot.kernel.sysctl."net.ipv6.conf.wg0.forwarding" = 1;

  # MTU considerations for VXLAN-over-WireGuard
  networking.interfaces.wg0.mtu = 1420;  # 1500 - 80 (IPv6 HbH header) = 1420

  # Firewall (allow WireGuard)
  networking.firewall.allowedUDPPorts = [ 51820 ];
  networking.firewall.allowedUDPPortRanges = [];
}
```

### Host-B NixOS Configuration

Similar to Host-A, but with:
- IPv6: `fd00:dead:beef::2/48`
- Peer: Host-A (with endpoint)

```nix
networking.wireguard.interfaces.wg0 = {
  ips = [ "fd00:dead:beef::2/48" ];
  listenPort = 51820;
  peers = [
    {
      publicKey = "HOST_A_PUBLIC_KEY_HERE";
      endpoint = "HOST_A_WAN_IP:51820";  # e.g., 203.0.113.45:51820
      allowedIPs = [ "fd00:dead:beef::1/128" ];
      presharedKey = "PRE_SHARED_KEY_HERE";
      persistentKeepalive = 25;
    }
  ];
  privateKeyFile = "/etc/wireguard/private.key";
};
```

After updating NixOS config:

```bash
# On host-a or host-b
sudo nixos-rebuild switch

# Verify WireGuard is up
ip addr show wg0
ip link show wg0
sudo wg show
```

---

## SECTION IV: ROUTING & NEIGHBOR DISCOVERY

### Step 4.1: IPv6 Neighbor Discovery

```bash
# On host-a, verify you can reach host-b's WireGuard IP
ping6 -c 3 fd00:dead:beef::2
# Expected: 3 packets transmitted, 3 received

# On host-b, verify you can reach host-a's WireGuard IP
ssh root@host-b
ping6 -c 3 fd00:dead:beef::1
# Expected: 3 packets transmitted, 3 received
```

### Step 4.2: BGP Adjacency Over WireGuard

Once WireGuard is up, FRR and BIRD should establish BGP peering:

```bash
# On host-a, check FRR BGP neighbor status
ssh root@host-a
vtysh

show bgp summary
# Expected (was "Active" before, now "Established"):
# Neighbor         V    AS   MsgRcvd MsgSent   TblVer  InQ OutQ  Up/Down State/PfxRcd
# fd00:dead:beef::2 4 65002    15      15        5      0    0 00:03:42 Established
#   (time increases as BGP messages are exchanged)

# On host-b, check BIRD BGP peer
ssh root@host-b
sudo birdc

show bgp summary
# Expected:
# BGP Session Summary
# Peer           State      Up/Down   Inst. RX    TX State|Received
# fd00:dead:beef::1 Established <timestamp> <msgs> <msgs>

exit
```

### Step 4.3: EVPN Route Exchange

```bash
# On host-a, check EVPN routes learned from host-b
vtysh

show bgp l2vpn evpn route
# Expected: Routes from host-b (rd 65002:10001)

show bgp l2vpn evpn route summary
# Expected: Shows route counts

# Check if host-b's loopback is reachable
show bgp ipv4 summary
show route 10.30.255.1
# Expected: Route to host-b loopback

exit

# On host-b, verify learned routes from host-a
sudo birdc

show route
# Expected: Routes including host-a loopback (10.20.255.1)

exit
```

---

## SECTION V: MTU & PERFORMANCE CONSIDERATIONS

### MTU Calculation

```
Standard Ethernet MTU: 1500 bytes
- 40 bytes (IPv6 header)
- 20 bytes (Monad HbH header, worst case)
- 50 bytes (WireGuard overhead: 16B auth tag + 34B encap)
- 40 bytes (VXLAN header: 8B fixed + 8B metadata + 24B flags)
= Usable payload: ~1350 bytes

Conservative setting: 1420 bytes (1500 - 80 for HbH + overhead buffer)
```

Set MTU on both hosts:

```bash
# On host-a
sudo ip link set wg0 mtu 1420

# On host-b
sudo ip link set wg0 mtu 1420

# Verify
ip link show wg0 | grep mtu
# Expected: mtu 1420
```

### Bandwidth & Latency

Test tunnel performance:

```bash
# Install iperf3 (if not present)
sudo apt-get install -y iperf3

# On host-b, start iperf3 server
ssh root@host-b "iperf3 -s -D"

# On host-a, run iperf3 client (test IPv6 ULA tunnel)
iperf3 -c fd00:dead:beef::2 -t 10

# Expected output:
# Connecting to host fd00:dead:beef::2, port 5201
# [ 4] local fd00:dead:beef::1 port XXXXX connected to fd00:dead:beef::2 port 5201
# [ ID] Interval           Transfer     Bitrate         Retr  Cwnd
# [ 4]   0.00-1.00   sec  X Bytes      Y Mbps          Z   Mbytes

# Typical performance: 900-1000 Mbps on modern hardware (depends on CPU, encryption)
```

---

## SECTION VI: SECURITY CONSIDERATIONS

### Pre-Shared Key (PSK)

The PSK adds an additional layer of symmetric encryption:

```bash
# Both hosts should have the same PSK
# Store securely (not in version control)

# Rotate keys periodically (recommended: 90 days)
# - Generate new keys
# - Update on both hosts simultaneously (coordinated downtime)
# - Verify traffic flows before removing old keys
```

### Endpoint Restriction

WireGuard's allowed-ips field restricts which IPs can be sent through the peer:

```bash
# On host-a:
# peer Host-B is allowed to send only from fd00:dead:beef::2/128
# Any traffic claiming to be from other IPs will be dropped

# On host-b:
# peer Host-A is allowed to send only from fd00:dead:beef::1/128

# This prevents spoofing attacks within the tunnel
```

### Firewall Rules

OPNsense and IPFire should allow WireGuard (UDP 51820):

```bash
# Already configured in setup-opnsense.sh and setup-ipfire.sh
# But verify they're still in place

# On host-a OPNsense:
curl -sk -u root:opnsense https://192.168.1.1/api/firewall/filter/searchRule | \
  python3 -c "import sys, json; rules = json.load(sys.stdin)['rows']; print([r['description'] for r in rules if '51820' in str(r)])"
# Expected: Rule with "51820" (WireGuard)

# On host-b IPFire:
ssh root@192.168.2.1 "nft list table inet filter | grep 51820"
# Expected: Rule mentioning port 51820
```

---

## SECTION VII: TROUBLESHOOTING

### Issue: WireGuard Handshake Never Completes

**Symptom:** `sudo wg show` shows "latest handshake: never"

**Root Cause:**
1. Firewall blocking UDP 51820
2. Wrong public key
3. MTU too small
4. Time sync issues (WireGuard is sensitive to clock skew)

**Resolution:**

```bash
# Step 1: Verify firewall allows UDP 51820
# On host-a (check OPNsense rules)
curl -sk -u root:opnsense https://192.168.1.1/api/firewall/filter/searchRule | jq '.rows[] | select(.description | contains("51820"))'
# Should show: action "pass"

# Step 2: Verify keys are correct
on_host_a="$(ssh root@host-a 'sudo wg show | grep public' | awk '{print $3}')"
on_host_b="$(ssh root@host-b 'sudo wg show | grep public' | awk '{print $3}')"
echo "Host-A public: $on_host_a"
echo "Host-B public: $on_host_b"

# Verify they match what each host has as peer
ssh root@host-a "sudo wg show wg0 | grep peer"  # Should list host-b's public key
ssh root@host-b "sudo wg show wg0 | grep peer"  # Should list host-a's public key

# Step 3: Check MTU
ip link show wg0 | grep mtu
# If MTU is too small, increase to 1500 (or 1420 for safety)

# Step 4: Verify time sync
timedatectl status
# Expected: "synchronized: yes"
# If not synced, run: sudo timedatectl set-ntp true

# Step 5: Try manual handshake
ssh root@host-a "ping6 -c 1 fd00:dead:beef::2"
# Should trigger handshake (if host-b is reachable)

# Check logs
journalctl -u wireguard@wg0
# Look for "Handshake did not complete"
```

### Issue: BGP Neighbor State is "Idle" or "Connect"

**Symptom:** FRR/BIRD show neighbor in "Idle" or "Connect" state (not "Established")

**Root Cause:** BGP TCP connection not established (TCP 179)

**Resolution:**

```bash
# Verify BGP can reach the neighbor's loopback
# On host-a:
ssh root@host-a
ping -c 3 10.30.255.1  # Host-B loopback via VXLAN/routing
# If this fails, routing is broken

# Check FRR listen socket
netstat -tuln | grep ':179'
# Expected: LISTEN on 0.0.0.0:179 (IPv4) and [::]:179 (IPv6)

# Check neighbor configuration
vtysh -c "show bgp neighbor fd00:dead:beef::2"
# Expected: Connection State: established
#          Timers: Connect timer must be running

# Check routing to neighbor
vtysh -c "show route | grep fd00:dead:beef::2"
# Expected: Route to fd00:dead:beef::2 via wg0

# If stuck, try soft reset
vtysh -c "clear bgp fd00:dead:beef::2"
# This clears the session, forcing re-negotiation
```

### Issue: VXLAN Traffic Not Flowing Across Tunnel

**Symptom:** Services on host-a can't reach services on host-b (or vice versa)

**Root Cause:** VXLAN endpoint not discovered, or routing not configured

**Resolution:**

```bash
# Step 1: Verify VXLAN endpoints are configured
# On host-a:
ip link show vxlan10001
# Expected: vxlan10001: <BROADCAST,MULTICAST,UP,...> ... local 10.20.255.1 ... dstport 4789

# On host-b:
ip link show vxlan10001
# Expected: vxlan10001: <BROADCAST,MULTICAST,UP,...> ... local 10.30.255.1 ... dstport 4789

# Step 2: Verify EVPN is advertising VXLAN endpoints
# On host-a:
vtysh -c "show bgp l2vpn evpn route"
# Look for entries with remote VTEP IP (10.30.255.1)

# Step 3: Check FDB (forwarding database) entries
# On host-a:
bridge fdb show vxlan10001
# Expected: FDB entries showing VXLAN replication list

# Step 4: Test connectivity at service level
docker compose exec wotan ping -c 3 10.30.10.1
# If fails, check VXLAN bridge configuration

# Step 5: Verify no MTU issues in VXLAN path
# Run iperf3 over VXLAN tunnel
ssh root@host-b "docker compose exec sophia iperf3 -s -D"
docker compose exec sophia iperf3 -c 10.30.10.2 -t 10
# If throughput is low, increase MTU on VXLAN interfaces
```

---

## SECTION VIII: MONITORING & VALIDATION SCRIPT

Create a script to continuously monitor WireGuard tunnel health:

```bash
#!/usr/bin/env bash
# /usr/local/bin/monitor-wireguard.sh

set -euo pipefail

interval=30  # Check every 30 seconds
while true; do
  echo "=== $(date) ==="

  # Check interface status
  if ip link show wg0 >/dev/null 2>&1; then
    echo "✓ wg0 interface UP"
  else
    echo "✗ wg0 interface DOWN"
  fi

  # Check IPv6 connectivity
  if ping6 -c 1 -W 2 fd00:dead:beef::1 >/dev/null 2>&1 || ping6 -c 1 -W 2 fd00:dead:beef::2 >/dev/null 2>&1; then
    echo "✓ IPv6 tunnel connectivity OK"
  else
    echo "✗ IPv6 tunnel connectivity FAILED"
  fi

  # Check WireGuard peer handshake
  handshake=$(sudo wg show wg0 | grep "latest handshake" | awk '{print $NF}')
  if [ "$handshake" != "never" ]; then
    echo "✓ WireGuard handshake: $handshake"
  else
    echo "✗ WireGuard handshake: NEVER"
  fi

  # Check BGP peer status (if FRR installed)
  if command -v vtysh >/dev/null 2>&1; then
    bgp_state=$(vtysh -c "show bgp summary" | grep "fd00:dead:beef" | awk '{print $NF}' || echo "N/A")
    echo "  BGP neighbor state: $bgp_state"
  fi

  # Check transfer rates
  tx=$(sudo wg show wg0 | grep "transfer:" | awk '{print $NF}')
  echo "  Transfer: $tx"

  echo ""
  sleep $interval
done
```

Run it in background:

```bash
sudo nohup /usr/local/bin/monitor-wireguard.sh >> /var/log/wireguard-monitor.log 2>&1 &
```

---

## SUMMARY: WIREGUARD TUNNEL

WireGuard tunnel is ready when:

1. **Key Exchange**: Private/public keys generated and distributed
2. **Interface Configuration**: Both hosts have wg0 up with IPv6 ULA addresses
3. **Peer Configuration**: Each host knows the other's public key and endpoint
4. **Connectivity**: `ping6` succeeds across tunnel
5. **BGP Peering**: FRR (host-a) and BIRD (host-b) show "Established" neighbor state
6. **EVPN Routes**: Routes exchange successfully between hosts
7. **Traffic Flow**: Services can communicate across WireGuard tunnel

Proceed to Phase 12C (Cross-Host Validation) to complete the fabric bring-up.

