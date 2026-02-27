# PHASE 12A: HOST-B (THE OUTPOST) BOOT SEQUENCE RUNBOOK

**Estimated Total Time: ~3 hours**
**Last Updated: 2026-02-27**
**Status: PRODUCTION-READY**
**Dependencies: Host-A (The Forge) must be online first**

---

## OVERVIEW: HOST-B vs HOST-A

| Component | Host-A (Forge) | Host-B (Outpost) | Notes |
|-----------|---|---|---|
| Firewall VM | OPNsense 26.1.2 | IPFire 2.29.x | Different UI, same API concept |
| Routing Daemon | FRR 10.0 | BIRD 2.x | BGP/IS-IS capable, BIRD simpler config |
| BGP AS Number | AS 65001 | AS 65002 | iBGP EVPN over WireGuard |
| Storage | 500GB+ NVMe | 500GB+ NVMe | Same hardware tier |
| NIC Count | 2 (eno1 WAN, eno2 reserved) | 2 (eno1 WAN, eno2 reserved) | Same architecture |
| WireGuard Role | Server (listen :51820) | Client (dials host-a:51820) | But both listen for fallback |
| Knowledge Graph | Sophia (via Monad) | Sophia (via Monad) | Distributed state via WireGuard |

---

## SECTION I: PHASE 1 — NIXOS BASE INSTALL (IDENTICAL TO HOST-A)

Follow **PHASE 11, SECTION II** with ONE change:

### Configuration Difference

When creating the ISO, use `host-b` instead of `host-a`:

```bash
cd /path/to/unheaded
nix run nixpkgs#nixos-generators -- \
  --format iso \
  --flake ".#host-b"  # <-- host-b instead of host-a
  -o nixos-host-b-live.iso
```

### Steps are identical:
- Boot ISO (same BIOS settings)
- Verify kernel 5.17+ and BTF
- Partition disk (/dev/nvme0n1 → EFI + btrfs)
- Generate NixOS config
- Copy Unheaded NixOS tree
- Run `nixos-install --flake ".#host-b"`  # <-- host-b
- Reboot and SSH

**Expected result:** `hostname` returns "outpost" (not "forge")

---

## SECTION II: PHASE 2 — IPFIRE VM SETUP (~45 min)

### Difference from Host-A

Host-B uses **IPFire** instead of OPNsense. IPFire is:
- Lightweight (smaller VM)
- CLI-first with Perl backend
- Different firewall rule syntax
- Still supports IPv6 HbH passthrough

### Step 2.1: Create IPFire VM

```bash
# On host-b (via SSH):
ssh root@host-b

# Run the IPFire setup script
cd /tmp
scp root@host-a:/root/unheaded/scripts/firewall/setup-ipfire.sh .
./setup-ipfire.sh

# Expected output:
# [setup] Creating IPFire 2.29.x VM...
# [virt-install] Fetching IPFire ISO...
# [virt-install] Starting VM installation...
# [wait] Waiting for IPFire setup wizard...
# [setup] VM created, WebUI should be at https://192.168.2.1:8443
```

### Step 2.2: Verify IPFire WebUI

```bash
# On host-b, check VM status
virsh list
# Expected: ipfire-outpost RUNNING

# Get IPFire VM IP (usually 192.168.2.1)
virsh domifaddr ipfire-outpost
# Expected: eth1 192.168.2.1

# Test WebUI from host-b (IPFire uses 8443, not 443)
curl -sk https://192.168.2.1:8443/ -u admin:admin 2>/dev/null | head -20
# Expected: HTML response (200 OK) or 302 redirect
```

### Step 2.3: Configure IPFire (via Perl API)

```bash
# IPFire configuration is similar to OPNsense but uses different endpoints
# IPFire API is Perl-based; some operations may require shell access

# SSH into IPFire VM (if possible) OR use WebUI at https://192.168.2.1:8443
# Default creds: admin:admin

# Via WebUI:
# 1. Network → Interfaces
#    - Green (LAN): Set to 10.30.0.1/16 (different subnet than host-a!)
#    - Red (WAN): DHCP
# 2. Firewall → Rules
#    - Add rule: Allow IPv6 HbH (extension header 0)
#    - Add rule: Allow WireGuard (UDP 51820)
# 3. Services → Enable SSH (for remote config)

# Configuration snippet for IPFire firewall rules (via shell):
ssh root@192.168.2.1 <<EOF
# Allow IPv6 HbH extension headers
echo "rule family ipv6 { }" >> /etc/firewall.rules

# Allow WireGuard
echo "rule destination port 51820 protocol udp { }" >> /etc/firewall.rules

# Apply
/etc/init.d/firewall restart
EOF
```

### Step 2.4: Monad HbH Passthrough on IPFire

```bash
# IPFire may filter IPv6 extension headers aggressively
# To allow HbH, modify IPFire packet filter rules:

ssh root@192.168.2.1 <<EOF
# Check current IPv6 filter behavior
cat /proc/sys/net/ipv6/conf/*/disable_ipv6

# Allow HbH specifically in firewall rules
# IPFire uses nftables; add rule for extension header 0
nft add rule inet filter input ip6 nexthdr 0 accept

# Verify
nft list table inet filter | grep "nexthdr 0"

# Make persistent (add to IPFire config)
echo "nft add rule inet filter input ip6 nexthdr 0 accept" >> /etc/init.d/firewall

/etc/init.d/firewall restart
EOF
```

---

## SECTION III: PHASE 3 — BIRD ROUTING (~30 min)

### Key Difference from Host-A (FRR)

| Aspect | FRR (Host-A) | BIRD (Host-B) |
|--------|---|---|
| Config file | /etc/frr/frr.conf | /etc/bird/bird.conf |
| Protocol language | FRR proprietary | BIRD config language |
| IS-IS | ✓ (isisd) | ✗ (BIRD 2 doesn't support IS-IS natively; use BGP only) |
| BGP | ✓ | ✓ |
| BFD | ✓ | ✓ |
| EVPN | ✓ | ✓ |

**For Host-B, we use BGP-only (no IS-IS underlay). This is acceptable for a 2-node fabric.**

### Step 3.1: Verify BIRD Installation on Host-B

```bash
# SSH to host-b
ssh root@host-b

# Verify BIRD binaries
which bird birdc
# Expected: /run/current-system/sw/bin/bird (etc.)

# Verify BIRD service started
systemctl status bird
# Expected: active (running)

# Check BIRD version
bird -v
# Expected: BIRD v2.13+ or later
```

### Step 3.2: Create BIRD Configuration

```bash
# BIRD configuration for Host-B (AS 65002)
# Create /etc/bird/bird.conf on host-b (or copy from repository)

cat > /etc/bird/bird.conf <<'BIRD_CONFIG'
# SPDX-License-Identifier: MIT
# BIRD Configuration for host-b (Outpost)
# BGP EVPN overlay, no IS-IS underlay (simplified 2-node fabric)

log syslog all;
log "/var/log/bird.log" all;
router id 10.30.255.1;

# Kernel routing table integration
protocol kernel {
    scan time 10;
    export all;
}

# Device interface monitor
protocol device {
    scan time 10;
}

# Direct interface routes
protocol direct {
    interface "wg0";
    interface "lo";
}

# eBGP peering over WireGuard IPv6 ULA
protocol bgp FORGE {
    local as 65002;
    neighbor fd00:dead:beef::1 as 65001;

    ipv4 {
        import all;
        export where proto = "direct";
    };

    ipv6 {
        import all;
        export where proto = "direct";
    };
}

# BGP EVPN address family for VXLAN
protocol bgp EVPN {
    local as 65002;
    neighbor fd00:dead:beef::1 as 65001;

    ipv4 multicast {
        import all;
        export all;
    };

    ipv6 multicast {
        import all;
        export all;
    };
}

# Loopback address (BGP anchor)
protocol static {
    route 10.30.255.1/32 via "lo";
}

BIRD_CONFIG

# Restart BIRD
sudo systemctl restart bird

# Verify config syntax
sudo birdc -c "show"
# Expected: BIRD output (no errors)
```

### Step 3.3: Verify BGP Peering (FRR Host-A to BIRD Host-B)

Once WireGuard tunnel is up (Phase 12B):

```bash
# On host-b, check BIRD status
ssh root@host-b
sudo birdc

# Inside BIRD CLI:
show protocols all
# Expected output:
# name proto table state since info
# FORGE bgp main up <timestamp> Established

show bgp summary
# Expected:
# IPv4 Unicast Summary
#   Neighbor Address         AS      State
#   fd00:dead:beef::1        65001   Established

show bgp neighbors
# Expected: Shows BGP peer details (timers, stats, counters)

exit
```

---

## SECTION IV: PHASE 4 — VXLAN/EVPN SETUP (~20 min)

### Step 4.1: Create VXLAN Interface (same as Host-A)

```bash
# SSH to host-b
ssh root@host-b

# Create VXLAN interface for VNI 10001
sudo ip link add vxlan10001 type vxlan \
  id 10001 \
  local 10.30.255.1 \
  dstport 4789 \
  nolearning

# Create bridge
sudo ip link add name br-vxlan10001 type bridge
sudo ip link set vxlan10001 master br-vxlan10001

# Bring up
sudo ip link set vxlan10001 up
sudo ip link set br-vxlan10001 up

# Verify
ip link show | grep vxlan10001
# Expected: vxlan10001: <BROADCAST,MULTICAST,UP,...>
```

### Step 4.2: Verify VXLAN in BIRD

```bash
# On host-b:
sudo birdc

# BIRD automatically learns VXLAN endpoints from BGP EVPN routes
show route protocol EVPN
# Expected (once host-a is peering): Routes for VNI 10001

# Check route table
show route
# Expected: Routes including VXLAN VTEP entries

exit
```

---

## SECTION V: PHASE 5 — eBPF LOADING (IDENTICAL TO HOST-A)

Follow **PHASE 11, SECTION VI** — steps are identical for host-b.

```bash
# On host-b:
ssh root@host-b

# Build eBPF (if not pre-built)
cd /root/unheaded
make ebpf

# Create BPF pinning directory
sudo mkdir -p /sys/fs/bpf/unheaded
sudo mount -t bpf bpf /sys/fs/bpf/unheaded

# Load programs
sudo bpftool prog load /root/unheaded/ebpf/target/bpfel-unknown-none/release/monad.o \
  /sys/fs/bpf/unheaded/monad type sk_lookup
sudo bpftool prog load /root/unheaded/ebpf/target/bpfel-unknown-none/release/shield.o \
  /sys/fs/bpf/unheaded/shield type xdp
# (etc.)

# Attach Shield XDP to eno1
sudo ip link set dev eno1 xdp obj \
  /root/unheaded/ebpf/target/bpfel-unknown-none/release/shield.o

# Verify
sudo bpftool prog list
sudo bpftool net list
```

---

## SECTION VI: PHASE 6 — SERVICE FLEET BOOT (IDENTICAL TO HOST-A)

```bash
# On host-b:
ssh root@host-b

# Build binaries
cd /root/unheaded
make build

# Start services
docker compose up -d

# Verify health (services on 10.30.10.0/24 for host-b)
docker compose ps
docker compose logs --tail=50 | grep -i error
```

---

## SECTION VII: PHASE 7 — END-TO-END VALIDATION (LOCAL + CROSS-HOST)

### Step 7.1: Host-B Local Validation

```bash
# Same as Host-A (Section VIII of BOOT_SEQUENCE_HOST_A.md)
# Ping services on 10.30.10.0/24
# Health check all services
# Verify eBPF loaded
# Check logs for errors
```

### Step 7.2: Cross-Host Validation (WireGuard + BGP)

This is covered in the next section (PHASE 12B: WIREGUARD_TUNNEL.md).

Once WireGuard is up:

```bash
# Test BGP peering
ssh root@host-a "vtysh -c 'show bgp summary'"
ssh root@host-b "sudo birdc -c 'show bgp summary'"
# Both should show "Established" neighbor state

# Test EVPN route exchange
ssh root@host-a "vtysh -c 'show bgp l2vpn evpn route'"
ssh root@host-b "sudo birdc -c 'show route protocol EVPN'"
# Routes should propagate between hosts

# Test Wotan pub/sub across WireGuard
docker compose exec -T wotan curl -s http://localhost:18001/topics | jq .
# Should show cross-host subscriptions
```

---

## SECTION VIII: SMOKE TEST CHECKLIST (HOST-B)

```
HOST-B BOOT SEQUENCE SMOKE TEST
================================

[ ] Phase 1: NixOS Base Install
  [ ] Kernel version >= 5.17
  [ ] BTF support verified
  [ ] SSH accessible (root@host-b)
  [ ] Hostname is "outpost"
  [ ] WAN interface (eno1) has DHCP IP
  [ ] Loopback has 10.30.255.1/32
  [ ] Different subnet than host-a (10.30.0.0/16, not 10.20.0.0/16)

[ ] Phase 2: IPFire Firewall VM
  [ ] VM running (virsh list shows ipfire-outpost)
  [ ] WebUI accessible (https://192.168.2.1:8443/)
  [ ] WAN interface: DHCP
  [ ] LAN interface: 10.30.0.1/16
  [ ] Monad HbH rules present (nft shows "ip6 nexthdr 0")
  [ ] WireGuard UDP 51820 rule present

[ ] Phase 3: BIRD Routing
  [ ] BIRD service running (systemctl status bird)
  [ ] BIRD config valid (birdc 'show' works)
  [ ] BGP configured (AS 65002)
  [ ] BGP neighbor configured (fd00:dead:beef::1)
  [ ] Loopback advertised: 10.30.255.1/32

[ ] Phase 4: VXLAN/EVPN
  [ ] VXLAN interface created (ip vxlan10001)
  [ ] Bridge created (br-vxlan10001)
  [ ] BIRD capable of EVPN routes

[ ] Phase 5: eBPF Loading
  [ ] BPF filesystem mounted
  [ ] 4+ eBPF programs loaded
  [ ] Shield attached to eno1 (xdp)
  [ ] 8+ BPF maps available

[ ] Phase 6: Service Fleet Boot
  [ ] All binaries built
  [ ] docker compose ps: All containers "Up"
  [ ] All services healthy (:18000, :19000, :16666, :16667, :21000)
  [ ] No FATAL errors in logs

[ ] Phase 7: Local End-to-End
  [ ] Ping all service IPs (10.30.10.0/24)
  [ ] All services respond to health checks
  [ ] eBPF maps populated
  [ ] Service logs normal

[ ] Cross-Host (after WireGuard in Phase 12B)
  [ ] BGP peering established (Established state)
  [ ] EVPN routes exchanged
  [ ] WireGuard tunnel up (ping6 fd00:dead:beef::1)
  [ ] Wotan pub/sub cross-host (topics replicated)
  [ ] Sophia state synchronized

FINAL VERDICT: [ ] PASS  [ ] FAIL
  If FAIL, see TROUBLESHOOTING in BOOT_SEQUENCE_HOST_A.md (issues are identical).
```

---

## SECTION IX: HOST-B SPECIFIC NOTES

### Subnet Allocation

Host-B uses **different subnets** to avoid conflicts:

```
10.30.0.0/16        Service containers (host-b, NOT 10.20.0.0/16)
10.30.0.1/24        IPFire LAN interface
10.30.255.1/32      BIRD loopback
fd00:dead:beef::/48 WireGuard (shared with host-a)
  fd00:dead:beef::2/128   Host-B (BIRD source, client to host-a)
```

### BGP AS Number

Host-B uses **AS 65002** (Host-A is AS 65001). This is important for:
- BGP peering across WireGuard
- EVPN route distinguisher (65002:10001)
- AS path filtering (if needed)

### BIRD vs FRR

BIRD is simpler than FRR but lacks:
- IS-IS support (we use BGP only for underlay)
- Some advanced eBGP features
- Packet sniffer integration

This is acceptable for a 2-node fabric. For 3+ nodes, consider FRR on all.

---

## SUMMARY

Host-B boot sequence is complete when:

1. **NixOS** boots with kernel 5.17+, hostname "outpost", subnet 10.30.0.0/16
2. **IPFire** VM is running with Monad HbH rules
3. **BIRD** BGP is configured (AS 65002)
4. **VXLAN/EVPN** infrastructure is in place
5. **eBPF** programs are loaded
6. **Service fleet** is healthy
7. **Cross-host validation** passes (BGP peering, EVPN routes, Wotan sync)

Proceed to PHASE 12B (WireGuard Tunnel) to bring the fabric fully online.

