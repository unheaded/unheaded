# LXD Firewall VMs & Routing Daemons

This directory contains configurations for deploying OPNsense and IPFire as LXD VMs, along with FRR and BIRD routing daemons as containers.

## Why LXD VMs for Firewalls?

LXD VMs are the best platform for enterprise-grade firewalls like OPNsense and IPFire because:

1. **Full Kernel Isolation**: VMs run their own complete kernel (FreeBSD for OPNsense, Linux for IPFire), providing true OS-level isolation from the host and other containers.
2. **Native Driver Support**: Access to real NIC drivers, not just virtio pass-through. OPNsense and IPFire expect full hardware control.
3. **Serial Console Access**: LXD VMs support serial consoles, which is essential for initial setup and troubleshooting of headless firewalls.
4. **Memory and CPU Isolation**: True resource isolation, not cgroups-based limits.
5. **QEMU/KVM Performance**: Hardware virtualization ensures firewall packet processing isn't limited by container overhead.
6. **Stateful Firewall Rules**: Both OPNsense and IPFire maintain complex stateful firewall tables that require full kernel support.

## Image Import Process

### OPNsense 26.1.2 (FreeBSD)

The OPNsense image is a **serial-optimized FreeBSD disk image** (520MB bz2):

```bash
bash lxd/firewall/import-opnsense.sh
```

This script:
1. Decompresses the bz2 image to raw format
2. Creates LXD metadata.yaml with FreeBSD properties
3. Imports into LXD as a VM image with alias `opnsense-26.1.2`
4. Verifies the import succeeded

### IPFire 2.29 (Linux)

The IPFire image is a **Linux disk image** (497MB xz):

```bash
bash lxd/firewall/import-ipfire.sh
```

This script:
1. Decompresses the xz image to raw format
2. Creates LXD metadata.yaml with Linux properties
3. Imports into LXD as a VM image with alias `ipfire-2.29-core199`
4. Verifies the import succeeded

## VM vs Container Distinction in LXD

| Feature | VM | Container |
|---------|----|-----------| 
| Kernel | Own kernel instance | Shared host kernel |
| Isolation | Hardware-level | cgroup/namespace |
| Boot time | 30-90 seconds | <1 second |
| Overhead | ~2-4% CPU/mem | <1% |
| Use case | Firewalls, legacy OS | Microservices, stateless |
| File system | Full ext4/UFS | Overlayfs |
| Network driver | Real virtio NIC | Virtual bridge |

**OPNsense and IPFire MUST run as VMs** — they need full OS control and can't share the host kernel.

## Two-NIC Setup

Both firewall VMs have two network interfaces:

### WAN Interface (vtnet0 / eth0)
- **Type**: macvlan (direct physical NIC pass-through)
- **Parent**: `eno1` (your physical WAN NIC)
- **Role**: Connects to upstream router / external network
- **Configuration**: DHCP (or static, depending on upstream)
- **NO host IP**: The firewall VM owns this interface entirely

### LAN Interface (vtnet1 / eth1)
- **Type**: bridged
- **Parent**: `unheaded` (the service container bridge)
- **Role**: Connects to internal service containers
- **IP**: 10.20.0.1/16 (configured inside firewall)
- **Purpose**: Provides DHCP/routing to internal containers

## Serial Console Access

OPNsense and IPFire both support serial console, which is essential for initial setup:

```bash
# Connect to OPNsense serial console
lxc console unheaded-opnsense

# Connect to IPFire serial console
lxc console unheaded-ipfire

# Type Ctrl+A then Q to exit
```

The serial console allows:
- Initial firewall setup wizard
- Network interface configuration
- Root password setup
- Debugging boot issues
- Emergency access if web UI fails

## Boot Priority Chain

The `boot.autostart.priority` configuration ensures services start in the correct order:

1. **Priority 200 (OPNsense/IPFire)**: Firewall VMs start FIRST
   - Wait 30 seconds for OPNsense/IPFire to fully initialize
   - These provide the critical egress/ingress path

2. **Priority 190 (FRR/BIRD)**: Routing daemons start SECOND
   - Wait 10 seconds for routing engines to initialize
   - They announce routes via BGP to the firewall

3. **Priority 100-150 (Services)**: All other containers start LAST
   - Depend on firewall for egress and routing for traffic optimization
   - Safe to start once firewall is operational

### Why This Order?

- **Firewall first**: Packet egress/ingress must be controlled from the start
- **Routing second**: BGP speakers need to advertise routes immediately after firewall is up
- **Services last**: Depend on both firewall and routing being ready

## OPNsense Initial Setup

After launching:

```bash
lxc console unheaded-opnsense
```

You'll see the FreeBSD boot messages and eventually the OPNsense setup wizard:

```
Choose an option from the menu:
1) Configure LAN interface
2) Configure WAN interface
3) Configure OPT interfaces
4) Set up encryption
5) Shell
6) Reboot
7) Halt
8) Reset factory defaults
```

**Critical steps**:

1. Set WAN interface to `vtnet0` → DHCP (or static IP depending on upstream)
2. Set LAN interface to `vtnet1` → Static IP 10.20.0.1/16
3. Configure root password
4. Enable sshd (for remote configuration)
5. Set hostname to `opnsense-host-a`

After initial setup, you can configure firewall rules via:
- **Web UI**: `https://10.20.0.1` (from a container on the `unheaded` bridge)
- **SSH**: `ssh root@10.20.0.1` (once firewall rules allow it)
- **OPNsense REST API**: `https://10.20.0.1/api` (with API key)

## IPFire Initial Setup

After launching:

```bash
lxc console unheaded-ipfire
```

IPFire will boot to a login prompt. Default credentials:

```
Username: root
Password: (set during installation, or 'password' if factory reset)
```

**Critical setup**:

1. Run `setup` to configure networking
2. Assign `eth0` to RED (WAN) → DHCP
3. Assign `eth1` to GREEN (LAN) → Static 10.20.0.1/255.255.0.0
4. Set hostname to `ipfire-host-b`
5. Change root password
6. Enable web UI (on port 10443 by default)

Access web UI:
- **URL**: `https://10.20.0.1:10443`
- **User**: admin
- **Password**: (your chosen password)

## FRR Installation & Configuration

FRR runs in a container on host-a:

```bash
lxc launch ubuntu:24.04 unheaded-frr --profile unheaded-base --profile unheaded-ebpf
```

The container's cloud-init automatically:
1. Installs build dependencies
2. Creates frr user/group
3. Sets up directories

To build and install FRR from source:

```bash
lxc exec unheaded-frr -- bash
cd /tmp
git clone https://github.com/FRRouting/frr.git
cd frr
./configure --prefix=/usr --sysconfdir=/etc/frr
make
make install
```

Then configure BGP in `/etc/frr/bgpd.conf`:

```
router bgp 65001
  bgp router-id 10.20.255.1
  neighbor 10.20.0.1 remote-as external
  neighbor 10.20.0.1 description "OPNsense WAN"
  
  address-family ipv4 unicast
    neighbor 10.20.0.1 activate
  exit-address-family
```

Start FRR:
```bash
systemctl start frr
systemctl enable frr
```

## BIRD Installation & Configuration

BIRD runs in a container on host-b:

```bash
lxc launch ubuntu:24.04 unheaded-bird --profile unheaded-base --profile unheaded-ebpf
```

Cloud-init installs build dependencies and creates bird user/group.

Build BIRD from source:

```bash
lxc exec unheaded-bird -- bash
cd /tmp
git clone https://github.com/BIRD/bird.git bird
cd bird
./configure
make
make install
```

Configure BGP in `/etc/bird/bird.conf`:

```
router id 10.20.255.2;

protocol bgp {
  local as 65002;
  neighbor 10.20.0.1 as external;
  export all;
  import all;
}

protocol device {
  scan time 10;
}
```

Start BIRD:
```bash
systemctl restart bird
systemctl enable bird
```

## OPNsense FRR Plugin

After initial OPNsense setup, you can install the FRR plugin for native BGP support:

1. Log into OPNsense web UI: `https://10.20.0.1`
2. Navigate to **System → Firmware → Plugins**
3. Install **os-frr** plugin
4. Configure BGP in **Routing → BGP**
5. Reload FRR daemon

This allows OPNsense to speak BGP natively without a separate container.

## Troubleshooting

### Firewall VM won't boot
- Check serial console: `lxc console unheaded-opnsense`
- Verify image import: `lxc image list | grep opnsense`
- Check QEMU errors: `lxc info unheaded-opnsense | grep -A5 Error`

### Can't reach firewall from containers
- Verify LAN bridge is connected: `lxc network list`
- Check firewall rules inside VM (may be blocking access initially)
- Verify container is on `unheaded` bridge: `lxc config show unheaded-SERVICE | grep parent`

### FRR/BIRD not connecting to firewall
- Verify firewall has routing enabled (not just filtering)
- Check BGP session status: `vtysh -c "show ip bgp summary"`
- Verify ASN/router-id configuration
- Check firewall's BGP config (if using native BGP, not FRR plugin)

### Serial console unresponsive
- Try `lxc console --type=vga` for alternative console
- Reboot VM: `lxc restart unheaded-opnsense --force`
- Check if SSH is available instead

## File Locations

- Image import scripts: `lxd/firewall/import-*.sh`
- Network config: `lxd/networks/wan-bridge.yaml`
- Firewall profile: `lxd/profiles/unheaded-firewall.yaml`
- VM definitions: `lxd/containers/opnsense.yaml`, `ipfire.yaml`
- Routing containers: `lxd/containers/frr.yaml`, `bird.yaml`
- Launch scripts: `lxd/hosts/host-a/launch-all.sh`, `host-b/launch-minimal.sh`

## License

- OPNsense: BSD 2-Clause
- IPFire: GPL 2.0
- FRR: GPL 2.0+
- BIRD: GPL 2.0+
