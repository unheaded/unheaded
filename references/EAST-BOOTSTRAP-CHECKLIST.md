# EAST Bare Metal Bootstrap Checklist

**Hardware Target:** 4-core CPU, 8GB DDR3 RAM, 1x NIC
**Network:** fd00:dead:beef::2/48 (EAST outpost)
**Duration:** ~30 minutes (experienced), ~60 minutes (first time)

---

## PRE-FLIGHT (Preparation - ~5 minutes)

- [ ] Hardware specs verified: 4 cores, 8GB RAM, gigabit NIC
  - `lsinfo` on EAST hardware
  - Note: If RAM < 8GB, adjust service memory limits in `east-flake.nix`

- [ ] USB installer prepared with NixOS minimal ISO (24.05+)
  - `dd if=nixos-minimal.iso of=/dev/sdX bs=4M status=progress`
  - Verify: Insert USB, see boot menu

- [ ] WEST endpoint known
  - WEST public IP: `_______________`
  - WEST WireGuard port: `_____ (default 51820)`
  - WEST IPv6 address: `fd00:dead:beef::1`

- [ ] Network cable connected and switch port active
  - Verify: LED indicators on NIC and switch
  - Test with laptop: `ping -c 1 <WEST-IP>`

---

## PHASE 1: OS INSTALL (Bare Metal → NixOS)

**Duration:** ~10 minutes
**Goal:** Boot from USB, install NixOS, enable SSH

- [ ] Boot from USB (press F2/F12 during startup)
  - Verify: NixOS boot prompt appears

- [ ] Load terminal (Ctrl+Alt+F2 if graphical)
  - `# zsh` (or bash)

- [ ] Partition disk (512MB EFI, 2GB swap, rest root)
  ```bash
  # Replace sdX with your target disk (e.g., sda, nvme0n1)
  parted /dev/sdX mklabel gpt
  parted /dev/sdX mkpart ESP fat32 1MB 512MB
  parted /dev/sdX mkpart swap linux-swap 512MB 2.5GB
  parted /dev/sdX mkpart nixos ext4 2.5GB 100%
  parted /dev/sdX set 1 boot on

  # Format
  mkfs.fat -F 32 /dev/sdX1
  mkswap /dev/sdX2
  mkfs.ext4 -L nixos /dev/sdX3

  # Verify
  lsblk /dev/sdX
  ```

- [ ] Mount filesystems
  ```bash
  mount /dev/sdX3 /mnt
  mkdir -p /mnt/boot
  mount /dev/sdX1 /mnt/boot
  ```

- [ ] Generate NixOS configuration
  ```bash
  nixos-generate-config --root /mnt
  ```

- [ ] Download EAST flake
  ```bash
  # Option 1: Copy from WEST (if WEST is network-reachable)
  ssh root@<WEST-IP> "cat /path/to/east-flake.nix" > /mnt/etc/nixos/east-flake.nix

  # Option 2: Manual copy via USB
  cp east-flake.nix /mnt/etc/nixos/
  ```

- [ ] Install NixOS
  ```bash
  nixos-install -I nixos-config=/mnt/etc/nixos/east-flake.nix
  ```
  - Verify: Build completes, no fatal errors

- [ ] Reboot into NixOS
  ```bash
  umount -R /mnt
  reboot
  ```

- [ ] Login and verify SSH
  ```bash
  # From WEST (or another machine):
  ssh-keyscan fd00:dead:beef::2 >> ~/.ssh/known_hosts
  ssh root@fd00:dead:beef::2 "hostname"
  # Expected output: east-outpost
  ```

---

## PHASE 2: WIREGUARD TUNNEL TO WEST

**Duration:** ~5 minutes
**Goal:** Establish encrypted tunnel, EAST ↔ WEST
**Network:** fd00:dead:beef:wg::2/64 (EAST), fd00:dead:beef:wg::1/64 (WEST)

- [ ] Generate WireGuard keypair on EAST
  ```bash
  # On EAST:
  ssh root@fd00:dead:beef::2
  mkdir -p /etc/unheaded/wg
  umask 077
  wg genkey > /etc/unheaded/wg/private.key
  wg pubkey < /etc/unheaded/wg/private.key > /etc/unheaded/wg/public.key
  cat /etc/unheaded/wg/public.key
  # Note public key: ___________________________________
  ```

- [ ] Exchange keys with WEST
  ```bash
  # On WEST:
  cat /etc/unheaded/wg/public.key
  # WEST public key: ___________________________________

  # On WEST, add EAST to peers:
  sudo wg set wg0 peer <EAST-PUBLIC-KEY> allowed-ips fd00:dead:beef:wg::2/128
  sudo wg show wg0
  ```

- [ ] Update EAST flake with WEST keys
  ```bash
  # On EAST (edit /etc/nixos/east-flake.nix):
  services.unheaded.wireguard.serverPublicKey = "<WEST-PUBLIC-KEY>";
  services.unheaded.wireguard.serverEndpoint = "<WEST-PUBLIC-IP>:51820";
  services.unheaded.wireguard.outpostPublicKey = "<EAST-PUBLIC-KEY>";
  ```

- [ ] Rebuild and activate NixOS
  ```bash
  # On EAST:
  nixos-rebuild switch -I nixos-config=/etc/nixos/east-flake.nix
  ```

- [ ] Verify WireGuard tunnel
  ```bash
  # On EAST:
  ip addr show wg0
  # Expected: inet6 fd00:dead:beef:wg::2/64

  ping6 -c 3 fd00:dead:beef:wg::1
  # Expected: 3 packets, 0% loss, RTT ~1-5ms

  # Full mesh reachability:
  ping6 -c 3 fd00:dead:beef::1
  # Expected: works (via wg0)
  ```

- [ ] Enable persistent tunnel (systemd)
  ```bash
  # On EAST:
  sudo systemctl enable wireguard-wg0
  sudo systemctl status wireguard-wg0
  ```

---

## PHASE 3: BGP ROUTING (BIRD)

**Duration:** ~5 minutes
**Goal:** Establish BGP peering, EAST (AS 65002) ↔ WEST (AS 65001)
**Verify:** Bidirectional route exchange

- [ ] Start BIRD daemon on EAST
  ```bash
  # On EAST:
  sudo systemctl enable bird
  sudo systemctl start bird
  sudo systemctl status bird
  # Expected: "active (running)"
  ```

- [ ] Check BGP status
  ```bash
  # On EAST:
  birdc show protocols
  # Expected output:
  #   name     proto  table  state  since       info
  #   forge_ebgp  bgp   master  up    HH:MM:SS    Established
  ```

- [ ] Verify routes received from WEST
  ```bash
  # On EAST:
  birdc show route
  # Expected: IPv6 routes from WEST, e.g., fd00:dead:beef:1::/64

  # Check specific peer:
  birdc show route protocol forge_ebgp
  ```

- [ ] Verify WEST sees EAST routes
  ```bash
  # On WEST (FRR):
  vtysh -c "show ip bgp ipv6 unicast neighbors fd00:dead:beef::2 routes"
  # Expected: Routes with next-hop fd00:dead:beef:wg::2
  ```

- [ ] Test BGP path
  ```bash
  # On EAST, trace route to WEST subnet:
  traceroute6 fd00:dead:beef:1::10
  # Expected: 2 hops (wg0 → WEST)
  ```

---

## PHASE 4: SERVICE DEPLOYMENT

**Duration:** ~5 minutes (copy), ~5 minutes (startup)
**Goal:** Deploy 9 core services, verify health

**The 9 EAST Services:**
1. wotan (message bus)
2. monad (state)
3. sophia (knowledge)
4. anamnesis (memory)
5. gateway (public access)
6. dashboard-backend (UI backend)
7. prometheus-agent (metrics)
8. promtail (log shipper)
9. node-exporter (system metrics)

- [ ] Copy service binaries from WEST
  ```bash
  # On WEST, create distribution tarball:
  cd /opt/unheaded
  tar czf /tmp/east-services.tar.gz \
    bin/wotan bin/monad bin/sophia bin/anamnesis \
    bin/gateway bin/dashboard-backend

  # Copy to EAST:
  scp /tmp/east-services.tar.gz root@fd00:dead:beef::2:/opt/east/
  ```

- [ ] Extract and organize on EAST
  ```bash
  # On EAST:
  cd /opt/east
  tar xzf east-services.tar.gz

  # Verify:
  ls -la bin/
  # Expected: wotan, monad, sophia, anamnesis, gateway, dashboard-backend

  chmod +x bin/*
  ```

- [ ] Create systemd unit files (template)
  ```bash
  # On EAST, example for wotan:
  cat > /etc/systemd/system/east-wotan.service << 'EOF'
  [Unit]
  Description=EAST - Wotan Message Bus
  After=network-online.target wireguard-wg0.service
  Wants=network-online.target

  [Service]
  Type=simple
  User=unheaded
  ExecStart=/opt/east/bin/wotan --listen [::1]:8080
  Restart=on-failure
  RestartSec=5s
  MemoryLimit=800M

  [Install]
  WantedBy=multi-user.target
  EOF

  # Repeat for monad (8081), sophia (8082), etc.
  # Adjust ports, memory limits, and flags per service
  ```

- [ ] Enable and start services
  ```bash
  # On EAST:
  sudo systemctl daemon-reload

  for svc in east-wotan east-monad east-sophia east-anamnesis \
             east-gateway east-dashboard-backend; do
    sudo systemctl enable $svc
    sudo systemctl start $svc
  done

  # Verify all running:
  sudo systemctl status east-*
  # Expected: all "active (running)"
  ```

- [ ] Health check all services
  ```bash
  # On EAST, check ports are listening:
  ss -tlnp | grep -E ':(8080|8081|8082|8083|8084)'
  # Expected: 5 services on IPv6 loopback

  # Quick curl test (if services respond to health checks):
  curl -s http://[::1]:8080/health | jq .
  # (Adjust endpoint per service)
  ```

- [ ] Verify Wotan sees EAST services
  ```bash
  # On WEST, query Wotan:
  curl -s http://fd00:dead:beef::1:8080/registry | jq '.services[] | select(.host == "east-outpost")'
  # Expected: All 9 EAST services registered
  ```

---

## PHASE 5: OBSERVABILITY SETUP

**Duration:** ~3 minutes
**Goal:** Metrics and logs flowing to WEST

- [ ] Verify node-exporter
  ```bash
  # On EAST:
  sudo systemctl status prometheus-node-exporter

  # Test metrics endpoint:
  curl -s http://[::1]:9100/metrics | head -20
  # Expected: Prometheus format metrics (node_*)
  ```

- [ ] Verify promtail log shipper
  ```bash
  # On EAST:
  sudo systemctl status promtail

  # Check logs:
  journalctl -u promtail -n 20
  # Expected: No errors, "clients configured" messages
  ```

- [ ] Verify prometheus-agent locally
  ```bash
  # On EAST:
  curl -s http://[::1]:9090/api/v1/targets | jq '.data.activeTargets[].labels.instance'
  # Expected: [::1]:9100 (node-exporter)
  ```

- [ ] Check metrics on WEST Grafana
  ```bash
  # Open WEST Grafana:
  # http://fd00:dead:beef::1:3000/
  # Dashboard: "EAST Outpost" or "All Nodes"
  # Expected: Graphs for east-outpost showing CPU, memory, disk, network

  # Alternative: Query WEST Prometheus:
  curl 'http://fd00:dead:beef::1:9090/api/v1/query?query=node_cpu_seconds_total{instance=~"fd00:dead:beef::2.*"}'
  # Expected: JSON with east-outpost metrics
  ```

- [ ] Check logs on WEST Loki
  ```bash
  # Open WEST Grafana → Explore → Loki
  # Query: {hostname="east-outpost"}
  # Expected: Recent logs from EAST services
  ```

---

## PHASE 6: INTEGRATION VERIFICATION

**Duration:** ~5 minutes
**Goal:** Confirm all systems healthy and integrated

- [ ] All 9 services running
  ```bash
  # On EAST:
  sudo systemctl status east-* | grep -E '(wotan|monad|sophia|anamnesis|gateway|dashboard-backend)'
  # Expected: All "active (running)"
  ```

- [ ] WireGuard tunnel stable
  ```bash
  # On EAST:
  ip -s link show wg0
  # Expected: RX/TX packets, 0 errors, 0 drops

  # Verify no packet loss over 60 seconds:
  ping6 -c 60 fd00:dead:beef::1 | tail -1
  # Expected: 0% packet loss
  ```

- [ ] BGP session ESTABLISHED
  ```bash
  # On EAST:
  birdc show protocols | grep forge_ebgp
  # Expected: State column shows "up"

  # Check established time (should be recent):
  birdc show protocol forge_ebgp all
  # Expected: "State: ESTABLISHED, since YYYY-MM-DD HH:MM:SS"
  ```

- [ ] Logs flowing to WEST
  ```bash
  # On WEST, check recent EAST logs:
  tail -f /var/log/loki/index.log | grep east-outpost
  # Or query Loki:
  curl 'http://localhost:3100/loki/api/v1/query_range?query={hostname="east-outpost"}&start=NNN&end=NNN'
  # Expected: Recent entries with east-outpost label
  ```

- [ ] Metrics visible on WEST
  ```bash
  # On WEST Prometheus:
  curl 'http://localhost:9090/api/v1/query?query=count(node_uname_info{instance=~".*east.*"})'
  # Expected: Result value > 0

  # Or check WEST Grafana dashboard for EAST node
  # Dashboard: "Node Exporter Full" → Select "east-outpost" → See metrics
  ```

- [ ] End-to-end latency acceptable
  ```bash
  # On EAST:
  ping6 -c 10 fd00:dead:beef::1 | grep "min/avg/max"
  # Expected: avg < 5ms (local network)

  # Test service roundtrip:
  time curl -6 http://fd00:dead:beef::1:8080/health
  # Expected: < 100ms
  ```

- [ ] Configuration snapshot
  ```bash
  # On EAST, save current state:
  mkdir -p /opt/east/bootstrap-snapshot
  date > /opt/east/bootstrap-snapshot/timestamp.txt
  birdc show protocols > /opt/east/bootstrap-snapshot/bgp-status.txt
  ip addr > /opt/east/bootstrap-snapshot/network.txt
  systemctl status east-* > /opt/east/bootstrap-snapshot/services.txt

  # Backup to WEST:
  rsync -av /opt/east/bootstrap-snapshot/ root@fd00:dead:beef::1:/var/backups/east-snapshot/
  ```

---

## Troubleshooting Quick Reference

| Issue | Symptom | Command | Fix |
|-------|---------|---------|-----|
| **WireGuard down** | `ping6 fd00:dead:beef::1` fails | `sudo systemctl restart wireguard-wg0` | Check keys, endpoint IP |
| **BGP not peering** | `birdc show protocols` shows "down" | `birdc show protocol forge_ebgp all` | Check BGP AS, router ID, firewall |
| **Service won't start** | `systemctl status east-wotan` shows error | `journalctl -u east-wotan -n 50` | Check binary path, permissions, ports |
| **No metrics on WEST** | WEST Grafana shows "No data" for EAST | `curl http://[::1]:9090/metrics` on EAST | Verify prometheus-agent running, check firewall |
| **Logs not flowing** | WEST Loki has no EAST logs | `journalctl -u promtail -n 20` | Check Loki URL, network connectivity |
| **High packet loss** | `ping6` shows > 1% loss | `ip -s link show wg0` | Check MTU (1380), CPU load, network cable |

---

## Success Criteria

**EAST is ready for production when:**

- [x] All 9 services report "active (running)"
- [x] WireGuard tunnel stable (0% packet loss, < 5ms latency)
- [x] BGP session ESTABLISHED (routes exchanged)
- [x] WEST Grafana shows EAST node metrics (CPU, memory, disk, network)
- [x] WEST Loki shows recent logs from all EAST services
- [x] Wotan on WEST sees all 9 EAST services registered
- [x] End-to-end curl latency < 100ms (WEST → EAST service)

**Estimated Time:** 30-60 minutes (including troubleshooting)

---

## Post-Bootstrap (Optional)

Once EAST is stable, consider:

- [ ] Enable persistent storage backup (rsync to WEST daily)
- [ ] Configure alerting rules (CPU > 80%, memory > 90%)
- [ ] Add EAST to disaster recovery plan
- [ ] Document any custom service configuration
- [ ] Schedule next health check: `________________`

---

*Document revision: 2026-02-28*
*For issues or updates, contact: unheaded-infra@example.com*
