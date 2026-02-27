# PHASE 11: HOST-A (THE FORGE) BOOT SEQUENCE RUNBOOK

**Estimated Total Time: ~3 hours**
**Last Updated: 2026-02-27**
**Status: PRODUCTION-READY**

---

## SECTION I: HARDWARE & PREREQUISITES

### Hardware Requirements

- **CPU**: AMD64 with BTF support (Intel/AMD x86_64 with eBPF support)
- **Kernel**: Linux 5.17+ (per patch M3, provides BTF, XDP, BPF_PROG_TYPE_SK_LOOKUP)
- **RAM**: 32GB minimum (16GB for base, 16GB headroom for services + eBPF)
- **Storage**: 500GB+ NVMe SSD
  - 512MB EFI partition
  - 480GB+ btrfs for root + containers
- **Network**:
  - **NIC 1 (eno1)**: WAN upstream (dhcp + ipv6)
  - **NIC 2 (eno2)**: Reserved for future (currently unused)

### Tooling Prerequisites (on admin workstation)

```bash
# Required tools for ISO creation and SSH
sudo apt-get install -y nixos-generators sshpass openssh-client

# Optional but recommended: qemu-kvm for testing boot sequence
sudo apt-get install -y qemu-system-x86-64 virtio-win virt-install

# Verify kernel BTF support (on target hardware, later)
grep -o "^BTF" /proc/version_signature
# OR
ls -la /sys/kernel/btf/vmlinux
```

---

## SECTION II: PHASE 1 — NIXOS BASE INSTALL (~30 min)

### Step 1.1: Create NixOS ISO with Flake

```bash
# From your workstation with nixos-generators
cd /path/to/unheaded
nix run nixpkgs#nixos-generators -- \
  --format iso \
  --flake ".#host-a" \
  -o nixos-host-a-live.iso
```

**Expected Output:**
```
/nix/store/...-nixos-host-a-live.iso
```

### Step 1.2: Boot ISO on Target Hardware

```bash
# On KVM/hypervisor or physical:
# 1. Mount nixos-host-a-live.iso as boot ISO
# 2. Power on target hardware
# 3. Enter BIOS/UEFI: Verify
#    - Secure Boot: DISABLED (Nix signing not yet configured)
#    - TPM: ENABLED (optional but recommended)
#    - IOMMU/VT-x: ENABLED (for future VM nesting)
# 4. Boot from ISO
# 5. Wait for NixOS live environment prompt (bash shell)
```

### Step 1.3: Verify Hardware & BTF

```bash
# Logged in to NixOS live ISO:
uname -r
# Expected: 5.17.0 or later

# Check BTF support
ls -la /sys/kernel/btf/vmlinux
# Expected: -r--r--r-- 1 root root [SIZE] ... vmlinux

# Check CPU flags
grep flags /proc/cpuinfo | head -1
# Expected: flags: ... bmi1 bmi2 ... (Intel/AMD markers)

# Check available network interfaces
ip link show
# Expected: eno1 (WAN), eno2 (reserved), lo (loopback)

# Test WAN connectivity
ip link set eno1 up
dhclient eno1
ping -c 3 8.8.8.8
# Expected: 3 packets transmitted, 3 received, 0% packet loss
```

### Step 1.4: Partition Target Disk

**WARNING: This is DESTRUCTIVE. Verify target device is correct.**

```bash
TARGET_DISK="/dev/nvme0n1"  # Verify this is the correct disk!

# Wipe partition table (BE CAREFUL)
sudo wipefs -a "$TARGET_DISK"

# Partition: EFI (512MB) + root (remaining)
sudo parted -s "$TARGET_DISK" \
  mklabel gpt \
  mkpart primary fat32 1MiB 513MiB \
  mkpart primary btrfs 513MiB 100% \
  set 1 esp on

# Format EFI
sudo mkfs.fat -F 32 "${TARGET_DISK}p1"

# Format root (btrfs with compression)
sudo mkfs.btrfs -f -L nixos "${TARGET_DISK}p2"

# Mount root
sudo mkdir -p /mnt
sudo mount -o compress=zstd "${TARGET_DISK}p2" /mnt

# Create EFI mountpoint
sudo mkdir -p /mnt/boot/efi
sudo mount "${TARGET_DISK}p1" /mnt/boot/efi

# Verify structure
lsblk "$TARGET_DISK"
mount | grep /mnt
```

**Expected Output:**
```
nvme0n1      259:0    0 500G  0 disk
├─nvme0n1p1  259:1    0 512M  0 part /mnt/boot/efi
└─nvme0n1p2  259:2    0 499.5G 0 part /mnt
```

### Step 1.5: Generate NixOS Configuration

```bash
# Generate initial hardware-specific config
sudo nixos-generate-config --root /mnt

# Verify files were created
ls -la /mnt/etc/nixos/
# Expected: hardware-configuration.nix, configuration.nix
```

### Step 1.6: Copy Unheaded NixOS Tree

```bash
# From live ISO, mount the ISO source or prepare ahead:
# Option A: If you have network access from ISO:
cd /tmp
git clone https://github.com/unheaded/unheaded.git
cd unheaded

# Option B: Pre-copy to USB/external storage before booting ISO
# Then:
cp -r /path/to/unheaded/nixos/* /mnt/etc/nixos/
```

**Critical Files:**
```bash
ls -la /mnt/etc/nixos/
# Should contain:
#   flake.nix
#   flake.lock
#   hosts/host-a/configuration.nix
#   modules/...
```

### Step 1.7: NixOS Install with Flake

```bash
# From live ISO, in /mnt/etc/nixos:
sudo nixos-install --flake ".#host-a"

# This will:
# 1. Download dependencies (~10-15 min on good network)
# 2. Build NixOS system (~10 min)
# 3. Copy to /mnt
# 4. Install bootloader (GRUB + EFI)

# Watch for progress:
# - "[systemd-boot] Reading files..."
# - "building /nix/store/..."
# - "Copying files to /mnt..."
# - "Installing boot loader..."
```

**Expected Final Output:**
```
installation finished!
```

### Step 1.8: Reboot & Verify SSH

```bash
# Reboot from ISO
sudo systemctl reboot

# After hardware reboot:
# 1. Boot from NVMe (BIOS will auto-select after GRUB install)
# 2. Wait for kernel to load and systemd to settle (~30-45 sec)
# 3. Verify you get a login prompt or systemd-resolved starts

# From your admin workstation, test SSH:
ssh root@<TARGET_IP_ADDRESS>
# Expected: You should get a bash prompt on the target
# (IP obtained from DHCP on eno1, check your DHCP lease list)

# Once logged in on host-a:
uname -a
# Expected: Linux forge 5.17+ ... (kernel version confirmed)

hostnamectl
# Expected: hostname: forge

systemctl status systemd-resolved
# Expected: active (running)
```

---

## SECTION III: PHASE 2 — FIREWALL VM SETUP (~45 min)

### Step 2.1: Verify Host-A Network & Virtualization

```bash
# On host-a (via SSH):
ip link show
# Expected: eno1 (UP), possibly eno2

# Verify IOMMU/nested virt support
grep -E "vmx|svm" /proc/cpuinfo | head -1
# Expected: flags: ... vmx ... (Intel) OR svm ... (AMD)

# Verify libvirtd available
which virt-install
# Expected: /run/current-system/sw/bin/virt-install

systemctl status libvirtd
# Expected: active (running)
```

### Step 2.2: Prepare OPNsense VM via setup-opnsense.sh

```bash
# Copy script to host-a
scp scripts/firewall/setup-opnsense.sh root@host-a:/tmp/

# SSH to host-a
ssh root@host-a

# Run the script (this uses virt-install internally)
cd /tmp
./setup-opnsense.sh

# Expected output:
# [setup] Creating OPNsense 26.1.2 VM...
# [virt-install] Fetching OPNsense ISO...
# [virt-install] Starting VM installation...
# [wait] Waiting for OPNsense setup wizard...
# [setup] VM created, WebUI should be at https://192.168.1.1:443
```

### Step 2.3: Verify OPNsense WebUI

```bash
# On host-a, check VM status
virsh list
# Expected: opnsense-forge RUNNING

# Get OPNsense VM IP (usually 192.168.1.1)
virsh domifaddr opnsense-forge
# Expected: eth0 192.168.1.1

# Test WebUI from host-a
curl -sk https://192.168.1.1:443/ -u root:opnsense | head -20
# Expected: HTML response (200 OK), or 302 redirect to login

# Or from admin workstation (if accessible):
# Open browser: https://192.168.1.1/
# Expected: OPNsense login form
```

### Step 2.4: Configure OPNsense (WAN/LAN) via REST API

```bash
# On host-a, run the post-boot configuration
ssh -i /tmp/opnsense_vm_key root@192.168.1.1 <<EOF

# Set WAN interface (DHCP)
curl -sk -u root:opnsense -X POST \
  -H "Content-Type: application/json" \
  -d '{"interface": {"enable": "1", "ipaddr": "dhcp", "ipaddrv6": "dhcp6"}}' \
  https://192.168.1.1:443/api/interfaces/overview/setInterface/wan

# Set LAN interface (10.20.0.1/16)
curl -sk -u root:opnsense -X POST \
  -H "Content-Type: application/json" \
  -d '{"interface": {"enable": "1", "ipaddr": "10.20.0.1", "subnet": "16"}}' \
  https://192.168.1.1:443/api/interfaces/overview/setInterface/lan

# Apply firewall rules (see scripts/firewall/setup-opnsense.sh for full rules)
curl -sk -u root:opnsense -X POST \
  https://192.168.1.1:443/api/firewall/filter/apply

echo "✓ OPNsense configured"
EOF

# OR, if network isolation:
# Use the GUI at https://192.168.1.1, login with root:opnsense
# - Interfaces → WAN: Enable DHCP, set hostname "forge-opnsense"
# - Interfaces → LAN: Set to 10.20.0.1/16
# - Firewall → Rules: Add rules for Monad HbH (see scripts/firewall/setup-opnsense.sh)
# - Services → FRR: Enable (after plugin install)
```

### Step 2.5: Add Monad HbH Passthrough Rules

```bash
# On OPNsense WebUI (or via API):
# Firewall → Rules → WAN/LAN

# Rule 1: Allow IPv6 HbH (next-header 0x00) inbound on WAN
# - Action: Pass
# - Interface: WAN
# - Protocol: IPv6
# - IPv6 Protocol: Hop-by-Hop Options (0x00)
# - Direction: In
# - Log: NO (for performance)

# Rule 2: Allow IPv6 HbH inbound on LAN
# - (Same as above but Interface: LAN)

# Rule 3: Allow WireGuard (UDP 51820) on WAN
# - Action: Pass
# - Interface: WAN
# - Protocol: UDP
# - Destination Port: 51820
# - Direction: In

# Rule 4: Allow established/related
# - Action: Pass
# - Direction: In
# - Statetype: Keep state

# Rule 5: Default deny WAN inbound
# - Action: Block
# - Interface: WAN
# - Direction: In
# - Log: YES

# Apply and reload
```

**Verification:**
```bash
# From host-a, test OPNsense routing to service subnet
ping -c 3 10.20.0.254
# Expected: 3 packets transmitted, 3 received

# Check firewall rules applied
curl -sk -u root:opnsense \
  https://192.168.1.1:443/api/firewall/filter/searchRule | \
  python3 -c "import sys, json; rules = json.load(sys.stdin)['rows']; print(f'Total rules: {len(rules)}')"
# Expected: Total rules: 5+ (HbH, WireGuard, established, default deny, etc.)
```

---

## SECTION IV: PHASE 3 — FRR ROUTING (~30 min)

### Step 3.1: Verify FRR Installation on Host-A

```bash
# SSH to host-a
ssh root@host-a

# Verify FRR binaries
which vtysh zebra bgpd isisd bfdd
# Expected: /run/current-system/sw/bin/vtysh (etc.)

# Verify FRR service started
systemctl status frr
# Expected: active (running)

# Check FRR version
frr --version
# Expected: FRR 10.0 or later
```

### Step 3.2: Copy FRR Configuration

```bash
# From admin workstation:
scp routing/frr/frr.conf root@host-a:/etc/frr/frr.conf

# SSH to host-a and restart FRR
ssh root@host-a

# Verify config syntax
vtysh -f /etc/frr/frr.conf -c "exit"
# No output = syntax OK

# Restart FRR service
sudo systemctl restart frr

# Verify it's running
systemctl status frr
# Expected: active (running)
```

### Step 3.3: Verify IS-IS Adjacency (IPv6 ULA)

```bash
# SSH to host-a, enter vtysh
ssh root@host-a
vtysh

# Check IS-IS interface status
show isis interface
# Expected output:
# Interface wg0
#  Type: pointopoint
#  Circuit Type: level-2
#  Metric: 10
#  Level-2/Adjacency: UP

# Check IS-IS neighbors
show isis neighbor
# Expected (once host-b is online):
# Neighbor System ID    Interface   L  State  Holdtime  Hello Multiplier  Area Level
# host-b-outpost        wg0         2  Up     28        3

exit
```

### Step 3.4: Verify BGP Summary

```bash
# In vtysh:
show bgp summary
# Expected output:
# IPv4 Unicast Summary (VRF default):
# BGP router identifier 10.20.255.1, local AS number 65001
# Neighbor         V    AS   MsgRcvd MsgSent   TblVer  InQ OutQ  Up/Down State/PfxRcd
# fd00:dead:beef::2 4 65002     0       0        0      0    0    00:00:00 Active
#
# (Active = no peer yet. Once host-b online, should be "Established" with route counts)

show bgp ipv4 summary
show bgp ipv6 summary
show bgp l2vpn evpn summary

exit
```

### Step 3.5: Verify BFD Peers

```bash
# In vtysh:
show bfd peers
# Expected output:
# Peer           Local    Remote  Status
# fd00:dead:beef::2 wg0     wg0     Down (until host-b online)
#
# Once host-b is up and WireGuard tunnel is live:
# fd00:dead:beef::2 wg0     wg0     Up

exit
```

---

## SECTION V: PHASE 4 — VXLAN/EVPN SETUP (~20 min)

### Step 4.1: Create VXLAN Interfaces

```bash
# SSH to host-a
ssh root@host-a

# Create VXLAN interface for VNI 10001 (service subnet)
sudo ip link add vxlan10001 type vxlan \
  id 10001 \
  local 10.20.255.1 \
  dstport 4789 \
  nolearning

# Create VXLAN bridge
sudo ip link add name br-vxlan10001 type bridge
sudo ip link set vxlan10001 master br-vxlan10001

# Bring up interfaces
sudo ip link set vxlan10001 up
sudo ip link set br-vxlan10001 up

# Add service containers bridge as VLAN member (future)
sudo ip link set br-unheaded master br-vxlan10001

# Verify structure
ip link show | grep -A 2 "vxlan10001"
# Expected:
# vxlan10001: <BROADCAST,MULTICAST,UP,LOWER_UP> ...
```

### Step 4.2: Verify VTEPs in FRR

```bash
# Check if FRR detected VXLAN and is advertising EVPN
ssh root@host-a
vtysh

show evpn vni
# Expected (once host-b is peering):
# VNI    Tenantv4 Tenantv6 Iface/Tunnel
# 10001  10.20.0.0/16      vxlan10001

show bgp l2vpn evpn route
# Expected (once host-b is peering):
# Route Distinguisher: 65001:10001
#   [2]:[0]:[0]:[48]:[<MAC>]:[32]:[10.20.0.0/32] <- VXLAN endpoint
#   [5]:[0]:[0]:[0]:[0]:[0]:[0]:[24]:[fd00:dead:beef:1::/64] <- EVPN prefix

exit
```

### Step 4.3: Verify VNI-to-VRF Mappings

```bash
# Check NixOS/FRR VNI configuration (already in routing/frr/frr.conf)
# VNI 10001: service subnet (10.20.0.0/16, rd 65001:10001)
# VNI 10002: reserved (rd 65001:10002)
# VNI 10100: telemetry (rd 65001:10100)

# In vtysh:
show bgp l2vpn evpn vni 10001 arp
show bgp l2vpn evpn vni 10001 routes

# Expected: Routes will appear once host-b is online
```

---

## SECTION VI: PHASE 5 — eBPF LOADING (~30 min)

### Step 5.1: Verify eBPF Build Artifacts

```bash
# On host-a, verify eBPF binaries exist
ls -la /root/unheaded/ebpf/target/bpfel-unknown-none/release/
# Expected:
# -rw-r--r-- 1 root root [SIZE] ... monad.o
# -rw-r--r-- 1 root root [SIZE] ... shield.o
# -rw-r--r-- 1 root root [SIZE] ... hop.o
# -rw-r--r-- 1 root root [SIZE] ... yaldabaoth.o

# If not present, build them:
cd /root/unheaded
make ebpf
# This runs: cargo build --release --target=bpfel-unknown-none
```

### Step 5.2: Create BPF Filesystem & Mount

```bash
# SSH to host-a
ssh root@host-a

# Create BPF pinning directory
sudo mkdir -p /sys/fs/bpf/unheaded

# Mount BPF filesystem
sudo mount -t bpf bpf /sys/fs/bpf/unheaded

# Verify
mount | grep bpf
# Expected:
# bpf on /sys/fs/bpf/unheaded type bpf (rw,relatime)

# Make permanent (add to NixOS configuration or /etc/fstab)
echo "bpf /sys/fs/bpf/unheaded bpf defaults 0 0" | sudo tee -a /etc/fstab
```

### Step 5.3: Load eBPF Programs

```bash
# Verify bpftool is available
which bpftool
# Expected: /run/current-system/sw/bin/bpftool

# Load Monad program
sudo bpftool prog load /root/unheaded/ebpf/target/bpfel-unknown-none/release/monad.o \
  /sys/fs/bpf/unheaded/monad type sk_lookup

# Load Shield program (XDP on eno1)
sudo bpftool prog load /root/unheaded/ebpf/target/bpfel-unknown-none/release/shield.o \
  /sys/fs/bpf/unheaded/shield type xdp

# Load Hop program
sudo bpftool prog load /root/unheaded/ebpf/target/bpfel-unknown-none/release/hop.o \
  /sys/fs/bpf/unheaded/hop type tracepoint

# Load Yaldabaoth program
sudo bpftool prog load /root/unheaded/ebpf/target/bpfel-unknown-none/release/yaldabaoth.o \
  /sys/fs/bpf/unheaded/yaldabaoth type raw_tracepoint

# Verify all loaded
sudo bpftool prog list
# Expected: 4+ programs (monad, shield, hop, yaldabaoth, ...)
```

### Step 5.4: Attach XDP (Shield) to WAN Interface

```bash
# On host-a:
IFACE="eno1"

# Attach Shield program to eno1 (WAN)
sudo bpftool net attach xdp \
  /sys/fs/bpf/unheaded/shield \
  $IFACE

# Verify attachment
sudo bpftool net list
# Expected:
# xdp:
# eno1(2) -> shield(...)

# OR use newer syntax:
sudo ip link set dev $IFACE xdp obj \
  /root/unheaded/ebpf/target/bpfel-unknown-none/release/shield.o

# Check with ethtool
ethtool -S $IFACE | grep xdp
```

### Step 5.5: Verify eBPF Maps & Stats

```bash
# Check all maps
sudo bpftool map list
# Expected: 8+ maps
# - monad_register_stack (sk_lookup map)
# - monad_flow_table (per-flow state)
# - shield_stats (XDP statistics)
# - hop_events (tracepoint buffer)
# - yaldabaoth_metrics (aggregation map)

# Check individual map stats
sudo bpftool map show
sudo bpftool map dump name monad_register_stack
sudo bpftool map dump name monad_flow_table

# Check program stats
sudo bpftool prog stat
# Expected: loaded eBPF programs showing packet counts per attachment point
```

---

## SECTION VII: PHASE 6 — SERVICE FLEET BOOT (~30 min)

### Step 6.1: Build All Service Binaries

```bash
# On host-a (or admin workstation with Go/Rust):
cd /root/unheaded

# Full build (all services + eBPF)
make build

# Expected binaries created:
ls -la bin/
# - wotan (Fae Chamber message bus)
# - sophia (knowledge graph)
# - monad (state management)
# - shield (telemetry)
# - gateway (API gateway)
# - (etc.)

# Build should succeed with no errors
```

### Step 6.2: Docker Compose Up

```bash
# On host-a:
cd /root/unheaded

# Start all services (via docker-compose.yml)
docker compose up -d

# Expected output:
# [+] Running 12/12
#   ✓ Network unheaded-dev-control Created
#   ✓ Network unheaded-dev-data Created
#   ✓ Container wotan Started
#   ✓ Container sophia Started
#   ... (all services)

# Verify services are starting
docker compose ps
# Expected:
# CONTAINER ID  IMAGE            STATUS                PORTS
# abc123        unheaded-wotan   Up 15 seconds (starting)
# def456        unheaded-sophia  Up 14 seconds (starting)
# ... (all containers)

# Wait 30 seconds for services to fully initialize
sleep 30
```

### Step 6.3: Verify Service Health Checks

```bash
# Check all services are "Up" (not "starting" or "Exited")
docker compose ps
# Expected: All services in "Up X seconds" state

# Check logs for startup errors
docker compose logs --tail=50
# Expected: No FATAL or ERROR lines, services report "listening on :PORT"

# Test individual health endpoints (services on 10.10.10.0/24 subnet):
docker compose exec wotan curl -s http://localhost:18000/health | jq .
# Expected: {"status":"healthy",...}

docker compose exec sophia curl -s http://localhost:19000/health | jq .
# Expected: {"status":"healthy",...}

docker compose exec gateway curl -s http://localhost:21000/health | jq .
# Expected: {"status":"healthy",...}

# Check container logs for errors
docker compose logs monad | grep -i error
docker compose logs shield | grep -i error
# Expected: No error lines (or only expected errors)
```

---

## SECTION VIII: PHASE 7 — END-TO-END VALIDATION (~30 min)

### Step 7.1: Ping All Service IPs

```bash
# From host-a, ping service containers on 10.10.10.0/24
for i in {1..254}; do
  timeout 1 ping -c 1 10.10.10.$i &>/dev/null && echo "✓ 10.10.10.$i UP" || true
done

# Expected: Services on IPs like 10.10.10.10, 10.10.10.11, etc.
```

### Step 7.2: Health Check All Services

```bash
# Script to check all service endpoints
services=(
  "wotan:18000"
  "sophia:19000"
  "monad:16666"
  "shield:16667"
  "gateway:21000"
)

for svc in "${services[@]}"; do
  IFS=':' read -r name port <<< "$svc"
  status=$(docker compose exec $name curl -s http://localhost:$port/health 2>/dev/null | jq -r '.status' 2>/dev/null || echo "UNAVAILABLE")
  if [ "$status" = "healthy" ]; then
    echo "✓ $name:$port — HEALTHY"
  else
    echo "✗ $name:$port — $status"
  fi
done
```

### Step 7.3: Monad HbH Packet Injection (Scapy)

```bash
# Install Scapy on admin workstation
pip3 install scapy

# Create test script: /tmp/monad_test.py
cat > /tmp/monad_test.py <<'SCAPY_SCRIPT'
#!/usr/bin/env python3
from scapy.all import IPv6, IPv6ExtHdrHopByHop, Raw, send, conf
import time

conf.verb = 1

# Monad HbH header (20 bytes)
# TLV Option Type: 0x00 (Pad1), Length: 20
monad_hbh = IPv6ExtHdrHopByHop(
    options=[
        # 20-byte register stack (simplified as Raw payload)
        Raw(load=b'\x00' * 20)  # Monad 20-byte HbH register
    ]
)

# Craft packet: IPv6 -> HbH -> Echo Request to host-a
pkt = IPv6(dst="fd00:dead:beef:1::1") / monad_hbh / IPv6(src="fd00:dead:beef::2") / Raw(load=b"MONAD_TEST")

print("[*] Sending Monad HbH packet...")
print(pkt.show())
send(pkt)
print("[*] Packet sent")
time.sleep(1)

SCAPY_SCRIPT

# Run from admin workstation
python3 /tmp/monad_test.py

# OR, use tcpdump to verify HbH packets on host-a:
ssh root@host-a "sudo tcpdump -i eno1 'ip6 proto 0' -c 5"
# Expected: IPv6 packets with hopopt (HbH) headers
```

### Step 7.4: Verify Full Pipeline

```bash
# On host-a, start packet sniffer (in background)
ssh root@host-a "sudo tcpdump -i any 'ip6 proto 0' -w /tmp/monad_flow.pcap &"

# Inject Monad packet (from admin workstation with Scapy)
python3 /tmp/monad_test.py

# Wait a moment
sleep 2

# Kill tcpdump
ssh root@host-a "pkill tcpdump"

# Check capture
ssh root@host-a "tcpdump -r /tmp/monad_flow.pcap"
# Expected: HbH packets captured, showing:
# IPv6(...) -> Hop-by-Hop-Options(...) -> ...

# Check Monad processing logs
docker compose logs monad --tail=50 | grep -i "hbh\|header\|packet"
# Expected: Monad logs showing packet reception and processing

# Check Shield logs for XDP drop/pass decisions
docker compose logs shield --tail=50 | grep -i "drop\|pass\|xdp"
# Expected: Shield telemetry on packet decisions

# Check Sophia knowledge graph for inferred relationships
docker compose exec sophia curl -s http://localhost:19000/graph | jq . | head -50
# Expected: JSON graph showing relationships between services/packets

# Check Wotan pub/sub for message flows
docker compose logs wotan --tail=30 | grep "message\|publish\|topic"
# Expected: Wotan logging message flows across services

# Check Gateway for API traversal
curl -s http://localhost:21000/api/services | jq .
# Expected: List of registered services + HbH state
```

### Step 7.5: Dashboard Live Flow Visualization

```bash
# Access dashboard (if available)
# Assuming dashboard-backend runs on port 21443
curl -s http://localhost:21443/api/flows | jq . | head -100

# OR access web UI (if implemented)
# http://<host-a-ip>:21443/
# Expected: Dashboard showing:
# - Live packet flows (HbH headers)
# - Service interconnections
# - eBPF program statistics
# - Monad state snapshots
```

### Step 7.6: Log Aggregation Verification

```bash
# Check if logs are being aggregated (Loki, ELK, or local)
# Assuming logs go to Loki on port 3100

curl -s "http://localhost:3100/loki/api/v1/query?query={job=~\"monad|shield|sophia|wotan\"}" | jq .

# Expected: JSON showing log entries from all services, sortable by time

# OR, check local log files
docker compose exec monad cat /var/log/monad.log | tail -20
docker compose exec shield cat /var/log/shield.log | tail -20

# Expected: Application logs showing normal operation
```

---

## SECTION IX: SMOKE TEST CHECKLIST

Use this checklist AFTER completing all 7 phases:

```
HOST-A BOOT SEQUENCE SMOKE TEST
===============================

[ ] Phase 1: NixOS Base Install
  [ ] Kernel version >= 5.17
  [ ] BTF support verified (/sys/kernel/btf/vmlinux exists)
  [ ] SSH accessible (root@host-a)
  [ ] Hostname is "forge"
  [ ] WAN interface (eno1) has DHCP IP
  [ ] Loopback has 10.20.255.1/32

[ ] Phase 2: OPNsense Firewall VM
  [ ] VM running (virsh list shows opnsense-forge)
  [ ] WebUI accessible (https://192.168.1.1/)
  [ ] WAN interface: DHCP (has upstream IP)
  [ ] LAN interface: 10.20.0.1/16
  [ ] Monad HbH rules present (5+ rules in firewall)
  [ ] Disabled IPv6 scrub (preserves HbH headers)
  [ ] WireGuard UDP 51820 rule present

[ ] Phase 3: FRR Routing
  [ ] FRR service running (systemctl status frr)
  [ ] IS-IS up on wg0 (show isis interface)
  [ ] BGP configured (router bgp 65001)
  [ ] BGP summary shows neighbor fd00:dead:beef::2
  [ ] BFD peer configuration present
  [ ] Loopback advertised: 10.20.255.1/32
  [ ] Service subnet advertised: 10.20.0.0/16

[ ] Phase 4: VXLAN/EVPN
  [ ] VXLAN interface created (ip vxlan10001)
  [ ] Bridge created (br-vxlan10001)
  [ ] FRR advertising EVPN routes
  [ ] VNI-to-VRF mappings configured (VNI 10001, 10002, 10100)
  [ ] advertise-all-vni enabled in BGP EVPN

[ ] Phase 5: eBPF Loading
  [ ] BPF filesystem mounted (/sys/fs/bpf/unheaded)
  [ ] 4+ eBPF programs loaded (monad, shield, hop, yaldabaoth)
  [ ] Shield attached to eno1 (xdp)
  [ ] 8+ BPF maps available
  [ ] bpftool prog list shows all programs
  [ ] bpftool map list shows maps

[ ] Phase 6: Service Fleet Boot
  [ ] All binaries built (bin/wotan, bin/sophia, etc.)
  [ ] docker compose ps: All containers "Up" (not "starting" or "Exited")
  [ ] wotan health: HTTP 200 on :18000/health
  [ ] sophia health: HTTP 200 on :19000/health
  [ ] monad health: HTTP 200 on :16666/health
  [ ] shield health: HTTP 200 on :16667/health
  [ ] gateway health: HTTP 200 on :21000/health
  [ ] No FATAL errors in logs (docker compose logs | grep FATAL)

[ ] Phase 7: End-to-End Validation
  [ ] Ping all service IPs (10.10.10.1..254)
  [ ] Monad HbH packet injection successful (tcpdump shows "ip6 proto 0")
  [ ] All services processed HbH packets (logs show traffic)
  [ ] Shield telemetry active (XDP stats visible)
  [ ] Monad flow state captured (maps populated)
  [ ] Sophia relationships inferred (graph updated)
  [ ] Wotan pub/sub active (message flows logged)
  [ ] Dashboard shows live flows (if implemented)
  [ ] Logs aggregated (Loki/ELK accessible)

FINAL VERDICT: [ ] PASS  [ ] FAIL
  If FAIL, see TROUBLESHOOTING section below.
```

---

## SECTION X: TROUBLESHOOTING

### Issue: BTF support not available

**Symptom:** `ls -la /sys/kernel/btf/vmlinux` returns "No such file"

**Root Cause:** Kernel < 5.17 or compiled without BTF support

**Resolution:**
```bash
# Option 1: Update kernel via NixOS
# In nixos/hosts/host-a/configuration.nix:
boot.kernelPackages = pkgs.linuxPackages_latest;
boot.kernelParams = ["CONFIG_DEBUG_INFO_BTF=y"];

# Then:
sudo nixos-rebuild switch

# Option 2: Switch to a pre-built kernel with BTF
# In NixOS:
boot.kernelPackages = pkgs.linuxPackages_6_7;

# Reboot and verify:
ls -la /sys/kernel/btf/vmlinux
```

### Issue: OPNsense VM fails to start

**Symptom:** `virsh list` shows opnsense-forge as "shut off", WebUI unreachable

**Root Cause:** VM resource constraints, ISO fetch timeout, or virt-install misconfiguration

**Resolution:**
```bash
# Check VM status and errors
virsh dominfo opnsense-forge
virsh start opnsense-forge
virsh console opnsense-forge
# (May need Ctrl-] to exit console)

# Check if virt-install is still running
ps aux | grep virt-install

# If resource constrained:
# - Close other VMs
# - Increase available RAM on host-a (requirements: 32GB)
# - Reduce other services (docker compose down)

# Check network bridge
ip link show | grep -E "virbr|vnet"

# Restart libvirtd
sudo systemctl restart libvirtd
```

### Issue: FRR BGP neighbor "Active" (not "Established")

**Symptom:** `show bgp summary` shows neighbor state as "Active" indefinitely

**Root Cause:** WireGuard tunnel not up, or WireGuard key exchange failed

**Resolution:**
```bash
# Verify WireGuard interface
ip link show wg0
# Expected: wg0: <POINTOPOINT,NOARP,UP,...>

# Check WireGuard status
sudo wg show
# Expected: interface: wg0
#           public key: [KEY]
#           private key: (hidden)
#           listen port: 51820
#           fwmark: 0x0
#           peer: [host-b-public-key]
#             endpoint: host-b-ip:51820
#             allowed ips: fd00:dead:beef::2/128
#             latest handshake: [timestamp]
#             transfer: [bytes]

# If no handshake:
# - Verify host-b is online and reachable
# - Check WireGuard key exchange (see PHASE 12)
# - Verify firewall rules allow UDP 51820

# Check connectivity to host-b WireGuard endpoint
ping -c 3 <host-b-ip>
# If fails, check WAN routing on OPNsense firewall

# Once tunnel is up, BGP neighbor will transition to "Established"
vtysh -c "show bgp summary"
```

### Issue: eBPF programs fail to load

**Symptom:** `bpftool prog load` returns error, or `bpftool prog list` shows 0 programs

**Root Cause:** BTF mismatch, kernel module missing, or permission issues

**Resolution:**
```bash
# Verify BTF availability
ls -la /sys/kernel/btf/vmlinux

# Check kernel eBPF module
sudo modprobe bpf

# Verify bpftool works
sudo bpftool version
# Expected: bpftool v7.x.x (matching kernel)

# Check BPF maps can be created
sudo bpftool map create name test_map type hash key 4 value 8 entries 10
# If this fails, there's a kernel/permission issue

# Ensure /sys/fs/bpf is writable
ls -la /sys/fs/bpf/
# Expected: drwxr-xr-x root root

# Retry loading eBPF programs
sudo bpftool prog load /root/unheaded/ebpf/target/bpfel-unknown-none/release/monad.o \
  /sys/fs/bpf/unheaded/monad type sk_lookup

# If still fails, check kernel logs
sudo dmesg | tail -50 | grep -i "ebpf\|bpf\|verif"
```

### Issue: Services stuck in "starting" state

**Symptom:** `docker compose ps` shows containers with status "Up X seconds (health: starting)" after 5+ minutes

**Root Cause:** Service process failing silently, or health check misconfigured

**Resolution:**
```bash
# Check container logs
docker compose logs monad --tail=100
docker compose logs shield --tail=100

# Look for FATAL, panic, or exit messages
docker compose logs | grep -i "fatal\|panic\|exit"

# Check if container is actually running
docker ps | grep unheaded

# Try manually starting one service
docker compose up -d --no-deps --build monad
docker compose logs monad

# If service binary is missing, rebuild
cd /root/unheaded
make clean
make build

# If dependencies are missing, check Dockerfile
docker compose logs --tail=50 | grep -i "not found\|missing"

# Restart all services
docker compose down
docker compose up -d
```

### Issue: tcpdump shows no HbH packets

**Symptom:** `tcpdump -i any 'ip6 proto 0'` returns no packets even after Monad test injection

**Root Cause:** Packets not reaching interface, or tcpdump filter is wrong

**Resolution:**
```bash
# Verify IPv6 is working
ping -6 -c 3 fe80::1%eno1
# Expected: 3 packets transmitted, 3 received

# Check all IPv6 traffic
sudo tcpdump -i eno1 'ip6' -c 5

# Verify HbH option type (should be 0x00)
# Correct filter syntax:
sudo tcpdump -i eno1 'ip6[40] == 0x00' -c 5
# (Byte 40 is first extension header)

# Check if packets are being filtered at eBPF layer
sudo bpftool prog stats
# Look for Shield program: check if packets are being dropped

# Verify FRR is advertising routes (HbH packets may be for BGP updates)
vtysh -c "show bgp ipv6 summary"
```

---

## SECTION XI: PORT REGISTRY & SERVICE REFERENCES

**Unheaded Kingdom Port Allocations:**

```
16666-16999   Infrastructure & eBPF
  16666       Monad state service
  16667       Shield telemetry
  16668       Hop packet processor
  16669       Yaldabaoth metrics

17000-17999   Control Plane
  17000       Captain vision store
  17001       Architect ADR service
  17002       Micromanager task runner
  17003       Timeguru timeline

18000-18999   Data/Message Plane
  18000       Wotan message bus
  18001       Wotan admin API
  18100-18200  (reserved for future)

19000-19999   Knowledge Layer
  19000       Sophia knowledge graph
  19001       Sophia SPARQL endpoint
  19100-19200  (reserved for future)

20000-20999   Application Layer
  20100-20200  (reserved for apps)

21000-21443   Gateway/API
  21000       Gateway API
  21443       Gateway HTTPS

51820         WireGuard (east-west to host-b)
```

**Service IPs (10.10.10.0/24 subnet):**

```
10.10.10.1     Wotan (message bus)
10.10.10.2     Sophia (knowledge graph)
10.10.10.3     Monad (state management)
10.10.10.4     Shield (telemetry)
10.10.10.5     Captain (vision)
10.10.10.6     Architect (ADRs)
10.10.10.7     Micromanager (tasks)
10.10.10.8     Timeguru (timeline)
10.10.10.9     Gateway (API)
10.10.10.254   Host-A services bridge (10.20.0.254 on OPNsense LAN)
```

**Routing & Control Plane (Infrastructure Subnets):**

```
10.20.0.0/16        Service containers (host-a)
10.20.255.1/32      FRR loopback (host-a BGP anchor)
fd00:dead:beef::/48 WireGuard east-west tunnel
  fd00:dead:beef::1/128   Host-A (FRR source)
  fd00:dead:beef::2/128   Host-B (BIRD source)
```

---

## SUMMARY

Host-A boot sequence is complete when:

1. **NixOS** boots with kernel 5.17+, BTF, and proper hostname
2. **OPNsense** VM is running with Monad HbH rules
3. **FRR** IS-IS and BGP are configured (awaiting host-b for full adjacency)
4. **VXLAN/EVPN** infrastructure is in place
5. **eBPF** programs are loaded and XDP is attached
6. **Service fleet** (Wotan, Sophia, Monad, Shield, etc.) is healthy
7. **End-to-end validation** passes (HbH packets flow through pipeline)

Proceed to PHASE 12 (Host-B + WireGuard) once all smoke tests pass.

