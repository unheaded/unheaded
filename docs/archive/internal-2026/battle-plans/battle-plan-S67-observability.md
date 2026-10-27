# WARMONGER BATTLE PLAN: S67 Observability Stack + Log Aggregation + unheaded-daemon metrics pipeline

## HEADER BLOCK

**Date**: 2026-02-26  
**Sprint**: S67  
**Prerequisite**: Hosts host-a and host-b online, WireGuard tunnel fd00:dead:beef::/48 active, base NixOS config functional  
**Target**: Full observability of bare metal, LXD, Docker, FRR, BIRD, OPNsense, IPFire, Anamnesis, unheaded-daemon with 30d log retention  
**Duration**: 8-12 hours (phase-by-phase execution, cumulative verification)  
**Agent Strategy**: Sequential phase deployment with checkpoint commits after each phase gate  
**Commit Cadence**: [C] checkpoint every 4-5 steps, full PR per phase

---

## LEGEND

- **[B]** = Bash command execution (exact, verified)
- **[V]** = Verification step (curl, check output, assert state)
- **[D]** = Debug branch (conditional, failure path)
- **[W]** = Wait/poll (systemd start, port open, service health)
- **[R]** = Rollback/recovery action
- **[S]** = Schema/config write (file creation, JSON, YAML)
- **[P]** = Prometheus/Grafana provisioning
- **[C]** = Commit checkpoint (git add, commit with message)
- **[STUCK]** = Manual intervention required
- **[BLOCKED]** = Awaiting external condition

---

# PHASE 1: ENVIRONMENT VERIFICATION

**Goal**: Confirm kernel features, cgroup v2, binary availability, port availability on both hosts  
**Prerequisite**: SSH access to host-a (127.0.0.1:22, user=root) and host-b (fd00:dead:beef::2, user=root)  
**Time**: 30-45 min  
**Agent**: Bash (remote via SSH)

---

## Step 1: Check kernel cgroup v2 support on host-a

[B] Verify cgroup v2 unified hierarchy:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'grep "cgroup2" /proc/filesystems'
```
**Expected output**: `nodev  cgroup2`

[D] If cgroup v2 missing:
- Check `/etc/default/grub` for `systemd.unified_cgroup_hierarchy=0` disable
- Rebuild kernel with cgroup v2 enabled
- Reboot host-a
- Retry step 1

---

## Step 2: Check cgroup v2 mounted on host-a

[B] Verify mount:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'mount | grep "cgroup2 on /sys/fs/cgroup"'
```
**Expected output**: Contains `cgroup2 on /sys/fs/cgroup type cgroup2`

[D] If not mounted:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'mount -t cgroup2 none /sys/fs/cgroup'
```

---

## Step 3: Check kernel cgroup v2 support on host-b

[B] Repeat step 1 for host-b:
```bash
ssh -o StrictHostKeyChecking=no root@fd00:dead:beef::2 'grep "cgroup2" /proc/filesystems'
```

---

## Step 4: Check cgroup v2 mounted on host-b

[B] Repeat step 2 for host-b:
```bash
ssh -o StrictHostKeyChecking=no root@fd00:dead:beef::2 'mount | grep "cgroup2 on /sys/fs/cgroup"'
```

---

## Step 5: Verify Prometheus binary on host-a

[B] Check binary presence:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'which prometheus'
```
**Expected output**: `/nix/store/*/bin/prometheus` or similar

[D] If not found:
- Install via NixOS module in phase 6
- For now, defer to phase 6

---

## Step 6: Verify Grafana binary on host-a

[B] Check binary:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'which grafana-server'
```

[D] If not found:
- Install via NixOS module in phase 6

---

## Step 7: Verify Loki binary on host-a

[B] Check binary:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'which loki'
```

[D] If not found:
- Install via NixOS module in phase 6

---

## Step 8: Verify Promtail binary on host-a

[B] Check binary:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'which promtail'
```

---

## Step 9: Check port 9090 availability on host-a (Prometheus)

[B] Verify no service running:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'lsof -i :9090 2>/dev/null || echo "Port 9090 free"'
```

[D] If port in use:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'lsof -i :9090'
```
Kill service if safe, or reassign Prometheus to different port (modify phase 3 config)

---

## Step 10: Check port 3100 availability on host-a (Loki)

[B] Verify no service running:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'lsof -i :3100 2>/dev/null || echo "Port 3100 free"'
```

---

## Step 11: Check port 3000 availability on host-a (Grafana)

[B] Verify no service running:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'lsof -i :3000 2>/dev/null || echo "Port 3000 free"'
```

---

## Step 12: Check port 8428 availability on host-a (VictoriaMetrics)

[B] Verify no service running:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'lsof -i :8428 2>/dev/null || echo "Port 8428 free"'
```

---

## Step 13: Verify WireGuard tunnel fd00:dead:beef::/48 is active on host-a

[B] Check interface:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'ip addr show wg0 2>/dev/null | grep "fd00:dead:beef"'
```
**Expected output**: Contains `inet6 fd00:dead:beef::1/48`

---

## Step 14: Verify WireGuard tunnel is active on host-b

[B] Check interface:
```bash
ssh -o StrictHostKeyChecking=no root@fd00:dead:beef::2 'ip addr show wg0 2>/dev/null | grep "fd00:dead:beef"'
```
**Expected output**: Contains `inet6 fd00:dead:beef::2/48`

---

## Step 15: Test connectivity host-a to host-b via WireGuard

[B] Ping host-b from host-a:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'ping -c 1 fd00:dead:beef::2'
```
**Expected output**: `1 packets transmitted, 1 received`

[D] If ping fails:
- Check WireGuard peers: `wg show wg0`
- Check allowed IPs configuration
- Verify firewall rules (ufw, nftables, pf)
- Debug with `ping -c 1 127.0.0.1` on host-a first (loopback sanity check)

---

## Step 16: Verify /proc/meminfo accessible on host-a

[B] Check file:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'head -3 /proc/meminfo'
```

---

## Step 17: Verify /proc/stat accessible on host-a

[B] Check file:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'head -1 /proc/stat'
```

---

## Step 18: Verify /sys/class/net accessible on host-a

[B] Check directory:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'ls /sys/class/net/'
```

---

## Step 19: Verify lm-sensors available on host-a

[B] Check binary:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'which sensors'
```

[D] If not found:
- Install via NixOS: add `lm_sensors` to environment.systemPackages in phase 6

---

## Step 20: Check LXD availability on host-a

[B] Check binary:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'which lxc'
```

---

## Step 21: Check Docker availability on host-a

[B] Check binary:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'which docker'
```

---

## Step 22: List LXD containers on host-a

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'lxc list --format=json | jq length'
```
**Expected output**: Numeric count >= 0

---

## Step 23: Check systemctl availability on host-a

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'systemctl list-units --output=json 2>/dev/null | jq . | length'
```
**Expected output**: Numeric count > 0

---

## Step 24: Verify journald is running on host-a

[B] Check:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'systemctl status systemd-journald --no-pager | grep "active (running)"'
```

---

## Step 25: PHASE 1 EXIT GATE

[V] Confirm all steps 1-24 passed:
- [ ] cgroup v2 enabled and mounted on both hosts
- [ ] All binaries verified (prometheus, grafana, loki, promtail, lm_sensors, lxc, docker, systemctl)
- [ ] All required ports free (9090, 3100, 3000, 8428)
- [ ] WireGuard tunnel active and connectivity confirmed
- [ ] /proc/, /sys/, systemd accessible

**If any step failed**: Return to step 1 debug branch for that component before proceeding.

[C] Commit checkpoint — Phase 1 baseline verification complete (no code changes yet):
```bash
git add monitoring/
git commit -m "S67 Phase 1: Environment verification baseline — cgroup v2, binaries, ports, WireGuard confirmed"
```

---

# PHASE 2: unheaded-daemon metrics collector extension

**Goal**: Extend unheaded-daemon binary to expose Prometheus /metrics endpoint with bare metal, LXD, Docker, NixOS systemd metrics  
**Prerequisite**: Phase 1 passed, Go 1.21+, unheaded-daemon source code repo cloned  
**Time**: 2-3 hours  
**Agent**: Bash (local), Go test runner

---

## Step 26: Create pkg/metrics directory structure

[B] Create directory:
```bash
mkdir -p pkg/metrics
```

---

## Step 27-50: METRICS IMPLEMENTATION DETAILS

Write these files:

**pkg/metrics/collector_test.go** - Unit tests for all collectors
**pkg/metrics/types.go** - Interface definitions
**pkg/metrics/baremetal.go** - /proc/meminfo, /proc/stat, network stats collection
**pkg/metrics/lxd.go** - LXD container metrics via Unix socket
**pkg/metrics/docker.go** - Docker stats collection
**pkg/metrics/nixos.go** - systemd unit and journald metrics
**pkg/metrics/server.go** - HTTP server exposing /metrics and /health endpoints

All must follow standard Prometheus exposition format with HELP and TYPE headers.

---

## Step 51: Build unheaded-daemon with metrics endpoint

[B] Build binary:
```bash
go build -o unheaded-daemon ./cmd/daemon
```

[D] If build fails:
- Check Go version: `go version`
- Run `go mod download`
- Fix import issues with `go get -u ./...`

---

## Step 52: Test metrics endpoint locally

[B] Start daemon and test:
```bash
./unheaded-daemon &
sleep 2
curl -s http://localhost:9090/metrics | head -20
```
**Expected output**: Prometheus format metric lines (# HELP, # TYPE, metric_name{labels} value)

[D] If curl fails:
- Check daemon started: `lsof -i :9090`
- Check daemon logs: `ps aux | grep unheaded`
- Increase sleep to 3 seconds
- Try `netstat -tlnp | grep 9090`

---

## Step 53: Run metrics unit tests

[B] Execute test suite:
```bash
go test ./pkg/metrics/... -race -count=1 -v
```
**Expected output**: All tests pass (or skip if sockets unavailable)

[D] If tests fail:
- Check error message for missing imports
- Run `go mod tidy`
- Verify prometheus/client_model is imported
- Re-run tests

---

## Step 54: PHASE 2 EXIT GATE

[V] Confirm:
- [ ] All metrics unit tests passing
- [ ] Binary builds successfully
- [ ] /metrics endpoint returns Prometheus format
- [ ] Bare metal, LXD, Docker, NixOS collectors implemented
- [ ] /health endpoint functional
- [ ] No hardcoded PII/customer data in metrics

[C] Commit checkpoint — Phase 2 metrics collector:
```bash
git add pkg/metrics/
git commit -m "S67 Phase 2: unheaded-daemon metrics collectors — bare metal, LXD, Docker, NixOS systemd"
```

---

# PHASE 3: Prometheus scrape configuration

**Goal**: Configure Prometheus to scrape all targets and remote-write to VictoriaMetrics  
**Prerequisite**: Phase 2 complete, unheaded-daemon binary with /metrics endpoint  
**Time**: 1-1.5 hours  
**Agent**: Bash (config write), promtool verification

---

## Step 55: Create monitoring directory structure

[B] Create directories:
```bash
mkdir -p monitoring/prometheus monitoring/grafana/dashboards monitoring/loki monitoring/promtail
```

---

## Step 56: Write Prometheus configuration file

[S] Write `monitoring/prometheus/prometheus.yml` with:
- global scrape_interval: 15s
- scrape_configs for: prometheus, unheaded-daemon (host-a:9090), unheaded-daemon (host-b:9090 via WireGuard), node_exporter (both hosts), FRR BGPd (:9342), BIRD (:9324), OPNsense (:9273), IPFire (:9274), 25 Doom Range service containers
- remote_write to VictoriaMetrics localhost:8428
- Queue config: capacity 10000, max_shards 200, min_shards 1

---

## Step 57: Validate Prometheus configuration

[B] Run promtool check:
```bash
promtool check config monitoring/prometheus/prometheus.yml
```
**Expected output**: `The config is valid` or similar success message

[D] If validation fails:
- Check YAML syntax: `yamllint monitoring/prometheus/prometheus.yml`
- Fix indentation and quotation
- Re-run step 57

---

## Step 58: Check Prometheus active targets

[V] Execute:
```bash
curl -s http://127.0.0.1:9090/api/v1/targets | jq '.data.activeTargets[] | {job:.labels.job, health:.health}' | head -20
```
**Expected output**: JSON array with targets and health status

[D] If targets missing:
- Check endpoint URLs in prometheus.yml (verify WireGuard IPs)
- Verify services running on target hosts
- Check firewall rules allowing scrape traffic
- Re-run prometheus reload

---

## Step 59: Verify VictoriaMetrics remote write receiving data

[W] Wait 30 seconds for samples:
```bash
sleep 30
curl -s 'http://127.0.0.1:8428/api/v1/query?query=up' | jq '.data.result | length'
```
**Expected output**: Numeric count > 0

[D] If no data:
- Check VictoriaMetrics logs
- Verify Prometheus remote_write config in prometheus.yml
- Check network connectivity from Prometheus to localhost:8428
- Reload Prometheus

---

## Step 60: PHASE 3 EXIT GATE

[V] Confirm:
- [ ] prometheus.yml validates with promtool
- [ ] All target jobs defined (prometheus, unheaded-daemon, node_exporter, FRR, BIRD, OPNsense, IPFire)
- [ ] Prometheus /api/v1/targets shows targets
- [ ] Remote write to VictoriaMetrics configured
- [ ] Alerts rules file created

[C] Commit checkpoint — Phase 3 Prometheus scrape config:
```bash
git add monitoring/prometheus/
git commit -m "S67 Phase 3: Prometheus scrape config + VictoriaMetrics remote write"
```

---

# PHASE 4: Grafana dashboards

**Goal**: Create and provision 5 new Grafana dashboards  
**Prerequisite**: Phase 3 complete, Prometheus active targets scraped  
**Time**: 1.5-2 hours  
**Agent**: Bash (dashboard JSON write), Grafana provisioning

---

## Step 61-65: Create 5 Grafana dashboards

Create JSON files:
- `monitoring/grafana/dashboards/infrastructure.json` - CPU, memory, network, disk across hosts
- `monitoring/grafana/dashboards/container-fleet.json` - LXD + Docker combined metrics
- `monitoring/grafana/dashboards/routing-bgp.json` - FRR BGP, BIRD, BFD, VXLAN VNI
- `monitoring/grafana/dashboards/firewall.json` - OPNsense pf, IPFire conntrack, WAN stats
- `monitoring/grafana/dashboards/ebpf.json` - Monad HbH packets, kernel events

Each dashboard must have 4+ panels with Prometheus queries.

---

## Step 66: Copy dashboards to Grafana provisioning directory on host-a

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'mkdir -p /etc/grafana/provisioning/dashboards'
scp monitoring/grafana/dashboards/*.json root@127.0.0.1:/etc/grafana/provisioning/dashboards/
```

---

## Step 67: Create Grafana datasource provisioning file

[S] Write `monitoring/grafana/provisioning/datasources/prometheus.yaml` with:
- Prometheus at http://localhost:9090 (default)
- Loki at http://localhost:3100
- VictoriaMetrics at http://localhost:8428

---

## Step 68: Copy datasource provisioning to host-a

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'mkdir -p /etc/grafana/provisioning/datasources'
scp monitoring/grafana/provisioning/datasources/prometheus.yaml root@127.0.0.1:/etc/grafana/provisioning/datasources/
```

---

## Step 69: Restart Grafana service on host-a

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'systemctl restart grafana-server'
```

[D] If restart fails:
- Check logs: `ssh root@127.0.0.1 'journalctl -u grafana-server -n 50'`

---

## Step 70: List Grafana dashboards via API

[V] Execute:
```bash
curl -s http://127.0.0.1:3000/api/search | jq '.[].title'
```
**Expected output**: List including Infrastructure, Container Fleet, Routing & BGP, Firewall, eBPF Observability

[D] If dashboards not showing:
- Verify provisioning files copied
- Check Grafana logs for provisioning errors
- Manually import JSON via UI

---

## Step 71: PHASE 4 EXIT GATE

[V] Confirm:
- [ ] All 5 dashboards created
- [ ] Grafana restarted successfully
- [ ] Dashboards visible via API (/api/search)
- [ ] Datasources provisioned

[C] Commit checkpoint — Phase 4 Grafana dashboards:
```bash
git add monitoring/grafana/
git commit -m "S67 Phase 4: Grafana dashboards + datasource provisioning"
```

---

# PHASE 5: Log aggregation pipeline (Loki + Promtail)

**Goal**: Configure Loki on host-a and Promtail on both hosts  
**Prerequisite**: Phase 4 complete, Grafana running  
**Time**: 1.5-2 hours  
**Agent**: Bash (config write), logcli verification

---

## Step 72: Create Loki configuration file

[S] Write `monitoring/loki/loki.yaml` with:
- Server on port 3100
- Ingester: chunk_idle_period 3m, max_chunk_age 1h
- Storage: boltdb-shipper with filesystem backend at /loki
- Retention: 720h (30 days) default, 2160h (90 days) for security logs
- Auth disabled, metric_name not enforced

---

## Step 73: Copy Loki config to host-a

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'mkdir -p /etc/loki /loki/{index,cache,chunks}'
scp monitoring/loki/loki.yaml root@127.0.0.1:/etc/loki/loki.yaml
```

---

## Step 74: Create Promtail config for host-a

[S] Write `monitoring/promtail/promtail-host-a.yaml` with:
- Server on port 9080
- Push to http://127.0.0.1:3100/loki/api/v1/push
- Scrape configs: journald (labels: host=host-a), FRR logs, Docker, LXD
- Labels: job, host, service, level, container_runtime

---

## Step 75: Copy Promtail config to host-a

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'mkdir -p /etc/promtail'
scp monitoring/promtail/promtail-host-a.yaml root@127.0.0.1:/etc/promtail/promtail.yaml
```

---

## Step 76: Create Promtail config for host-b (remote write to host-a)

[S] Write `monitoring/promtail/promtail-host-b.yaml` with:
- Server on port 9080
- Push to http://fd00:dead:beef::1:3100/loki/api/v1/push (via WireGuard)
- Scrape configs: journald (labels: host=host-b), FRR, Docker

---

## Step 77: Copy Promtail config to host-b

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@fd00:dead:beef::2 'mkdir -p /etc/promtail'
scp monitoring/promtail/promtail-host-b.yaml root@fd00:dead:beef::2:/etc/promtail/promtail.yaml
```

---

## Step 78: Start Loki service on host-a

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'nohup loki --config.file=/etc/loki/loki.yaml > /var/log/loki.log 2>&1 &'
```

[D] If Loki fails:
- Check logs: `ssh root@127.0.0.1 'tail -20 /var/log/loki.log'`
- Verify config syntax: `ssh root@127.0.0.1 'loki --config.file=/etc/loki/loki.yaml --config.expand-env=true'`

---

## Step 79: Verify Loki is running

[W] Poll Loki API:
```bash
sleep 5
curl -s http://127.0.0.1:3100/ready
```
**Expected output**: `ready` or HTTP 200

---

## Step 80: Start Promtail services on both hosts

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'nohup promtail --config.file=/etc/promtail/promtail.yaml > /var/log/promtail.log 2>&1 &'
ssh -o StrictHostKeyChecking=no root@fd00:dead:beef::2 'nohup promtail --config.file=/etc/promtail/promtail.yaml > /var/log/promtail.log 2>&1 &'
```

---

## Step 81: Query Loki for host-a logs

[V] Execute logcli:
```bash
logcli query '{host="host-a"}' --limit=10 --addr=http://127.0.0.1:3100
```
**Expected output**: Log entries with timestamps

[D] If no logs:
- Check Promtail running: `ssh root@127.0.0.1 'ps aux | grep promtail'`
- Check Loki receiving: `ssh root@127.0.0.1 'tail -20 /var/log/loki.log'`
- Wait 5-10 seconds for scrape

---

## Step 82: Query Loki for host-b logs

[V] Execute:
```bash
logcli query '{host="host-b"}' --limit=10 --addr=http://127.0.0.1:3100
```

[D] If host-b logs missing:
- Verify Promtail on host-b running
- Check WireGuard connectivity: `ping fd00:dead:beef::2`
- Check Loki firewall: `ssh root@127.0.0.1 'lsof -i :3100'`

---

## Step 83: PHASE 5 EXIT GATE

[V] Confirm:
- [ ] Loki running on host-a (port 3100)
- [ ] Promtail running on both hosts
- [ ] logcli can query host-a logs
- [ ] logcli can query host-b logs
- [ ] WireGuard connectivity confirmed

[C] Commit checkpoint — Phase 5 log aggregation:
```bash
git add monitoring/loki/ monitoring/promtail/
git commit -m "S67 Phase 5: Loki + Promtail log aggregation — journald, FRR, container logs"
```

---

# PHASE 6: NixOS module updates

**Goal**: Create consolidated observability NixOS module  
**Prerequisite**: Phase 5 complete  
**Time**: 1 hour  
**Agent**: Bash (file write), nixos-rebuild

---

## Step 84-90: Create NixOS Modules

Create:
- `nixos/modules/observability.nix` - Consolidated module with:
  - services.prometheus (primary)
  - services.grafana (primary)
  - services.loki (primary)
  - services.promtail (all)
  - services.victoriametrics (primary)
  - Configurable role: primary | secondary
  - Firewall rules for all ports

- Update `nixos/hosts/host-a/configuration.nix` - Enable with role=primary
- Update `nixos/hosts/host-b/configuration.nix` - Enable with role=secondary

---

## Step 91: Rebuild NixOS on host-a

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'nixos-rebuild switch'
```

[D] If rebuild fails:
- Check syntax errors
- Verify observability.nix in /etc/nixos/modules/

---

## Step 92: Rebuild NixOS on host-b

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@fd00:dead:beef::2 'nixos-rebuild switch'
```

---

## Step 93: PHASE 6 EXIT GATE

[V] Confirm:
- [ ] NixOS rebuild successful on both hosts
- [ ] All services auto-start post-rebuild
- [ ] systemctl status shows running state

[C] Commit checkpoint — Phase 6 NixOS modules:
```bash
git add nixos/modules/ nixos/hosts/
git commit -m "S67 Phase 6: NixOS observability module + host configs"
```

---

# PHASE 7: Docker Compose stack updates

**Goal**: Add observability services to Docker Compose  
**Prerequisite**: Phase 6 complete  
**Time**: 45 min  
**Agent**: Bash (compose file write), docker-compose

---

## Step 94: Create Docker Compose for host-a

[S] Write `monitoring/docker-compose.yml` with services:
- prometheus (port 9090, volume prometheus.yml)
- grafana (port 3000, provisioning volumes)
- loki (port 3100, loki.yaml config)
- promtail (volumes: /var/log, /run/docker.sock, promtail.yaml)
- victoriametrics (port 8428)

---

## Step 95: Start Docker Compose on host-a

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'mkdir -p /opt/monitoring && cd /opt/monitoring && docker-compose up -d'
```

[D] If docker-compose up fails:
- Check Docker running: `ssh root@127.0.0.1 'systemctl status docker'`
- Pull images: `docker-compose pull`
- Check port availability

---

## Step 96: Create Docker Compose for host-b (promtail only)

[S] Write `monitoring/docker-compose-host-b.yml` with:
- promtail service only

---

## Step 97: Start Docker Compose on host-b

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@fd00:dead:beef::2 'mkdir -p /opt/monitoring && cd /opt/monitoring && docker-compose up -d'
```

---

## Step 98: PHASE 7 EXIT GATE

[V] Confirm:
- [ ] All containers running on host-a
- [ ] Promtail container running on host-b
- [ ] Port mappings working

[C] Commit checkpoint — Phase 7 Docker Compose:
```bash
git add monitoring/docker-compose*.yml
git commit -m "S67 Phase 7: Docker Compose observability stack"
```

---

# PHASE 8: LXD profile + cloud-init for observability containers

**Goal**: Create LXD profiles for observability-enabled containers  
**Prerequisite**: Phase 7 complete  
**Time**: 45 min  
**Agent**: Bash (profile/script write), lxc command

---

## Step 99: Create LXD observability profile

[B] Create profile:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'lxc profile create observability 2>/dev/null || true'
```

Configure with:
- Network device eth0 on lxdbr0
- Prometheus node-exporter enabled
- Cloud-init with observability packages

---

## Step 100: Create cloud-init script for observability containers

[S] Write `monitoring/cloud-init-observability.yaml` with:
- Package: prometheus-node-exporter
- Service: enable prometheus-node-exporter
- Config file for monitoring

---

## Step 101: Test observability-enabled container

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'lxc launch ubuntu:22.04 obs-test --profile default --profile observability'
sleep 10
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'lxc exec obs-test -- systemctl status prometheus-node-exporter'
```
**Expected output**: active (running)

---

## Step 102: Delete test container

[B] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'lxc delete -f obs-test'
```

---

## Step 103: PHASE 8 EXIT GATE

[V] Confirm:
- [ ] LXD profile created
- [ ] Cloud-init script configured
- [ ] Test container had metrics
- [ ] Profile ready for deployment

[C] Commit checkpoint — Phase 8 LXD observability:
```bash
git add monitoring/cloud-init-observability.yaml
git commit -m "S67 Phase 8: LXD observability profile + cloud-init"
```

---

# PHASE 9: End-to-end verification + smoke test

**Goal**: Verify all observability components integrated  
**Prerequisite**: Phase 8 complete  
**Time**: 1-1.5 hours  
**Agent**: Bash (curl/logcli), manual verification

---

## Step 104: Check Prometheus targets health

[V] Execute:
```bash
curl -s http://127.0.0.1:9090/api/v1/targets | jq '.data.activeTargets[] | {job:.labels.job, health:.health}'
```
**Expected output**: Target list with health="up"

---

## Step 105: Verify Prometheus scraping self

[V] Execute:
```bash
curl -s 'http://127.0.0.1:9090/api/v1/query?query=up{job="prometheus"}'
```
**Expected output**: JSON with value 1

---

## Step 106: Query bare metal memory metric from Prometheus

[V] Execute:
```bash
curl -s 'http://127.0.0.1:9090/api/v1/query?query=node_memory_MemFree_bytes' | jq '.data.result[0]'
```
**Expected output**: JSON with metric value

---

## Step 107: Verify Grafana Infrastructure dashboard

[V] Execute:
```bash
curl -s http://127.0.0.1:3000/api/dashboards/db/infrastructure 2>/dev/null | jq '.dashboard.title'
```
**Expected output**: "Infrastructure"

---

## Step 108: Verify Loki receiving host-a logs

[V] Execute:
```bash
logcli query '{host="host-a"}' --limit=5 --addr=http://127.0.0.1:3100
```
**Expected output**: Recent log entries

---

## Step 109: Verify Loki receiving host-b logs

[V] Execute:
```bash
logcli query '{host="host-b"}' --limit=5 --addr=http://127.0.0.1:3100
```
**Expected output**: Recent log entries

---

## Step 110: Check VictoriaMetrics remote-write active

[V] Execute:
```bash
curl -s 'http://127.0.0.1:8428/api/v1/query?query=up' | jq '.data.result | length'
```
**Expected output**: Numeric count > 0

---

## Step 111: Verify all Grafana datasources healthy

[V] Execute:
```bash
curl -s http://127.0.0.1:3000/api/datasources | jq '.[] | {name, type, state}'
```
**Expected output**: All datasources with state="success"

---

## Step 112: Verify all 5 Grafana dashboards

[V] Execute:
```bash
curl -s http://127.0.0.1:3000/api/search | jq '.[].title'
```
**Expected output**: Infrastructure, Container Fleet, Routing & BGP, Firewall, eBPF Observability

---

## Step 113: Test Prometheus PromQL aggregation

[V] Execute:
```bash
curl -s 'http://127.0.0.1:9090/api/v1/query?query=rate(node_network_receive_bytes_total[5m])' | jq '.data.result | length'
```
**Expected output**: Numeric count > 0

---

## Step 114: Verify metric retention on Prometheus

[V] Execute (manual):
```bash
echo "Check Prometheus UI at http://127.0.0.1:9090/config — verify retention 30d configured"
```

---

## Step 115: Verify Loki retention config

[V] Execute:
```bash
ssh -o StrictHostKeyChecking=no root@127.0.0.1 'grep retention_period /etc/loki/loki.yaml'
```
**Expected output**: `retention_period: 720h`

---

## Step 116: Query journald logs via Loki

[V] Execute:
```bash
logcli query '{job="journald"}' --limit=10 --addr=http://127.0.0.1:3100
```
**Expected output**: Log entries

---

## Step 117: Test Prometheus config reload

[B] Reload config:
```bash
curl -X POST http://127.0.0.1:9090/-/reload
```

---

## Step 118: Verify no data loss on reload

[V] Execute:
```bash
curl -s 'http://127.0.0.1:9090/api/v1/query?query=up' | jq '.data.result | length'
```
**Expected output**: Numeric count > 0 (data persists)

---

## Step 119: PHASE 9 EXIT GATE

[V] Confirm all:
- [ ] Prometheus targets all healthy
- [ ] Prometheus self-scraping working
- [ ] Grafana dashboards rendering
- [ ] Loki receiving logs from both hosts
- [ ] VictoriaMetrics remote-write active
- [ ] All datasources healthy
- [ ] All metrics queryable

[C] Final commit — Phase 9 verification complete:
```bash
git add -A
git commit -m "S67 Phase 9: End-to-end observability verification complete"
```

---

# APPENDIX A: Emergency Procedures

## Rollback Prometheus config
```bash
ssh root@127.0.0.1 'cp /etc/prometheus/prometheus.yml.bak /etc/prometheus/prometheus.yml'
ssh root@127.0.0.1 'systemctl restart prometheus'
```

## Restart Loki
```bash
ssh root@127.0.0.1 'systemctl restart loki'
ssh root@127.0.0.1 'sleep 5 && logcli query "{host=\"host-a\"}" --limit=5 --addr=http://127.0.0.1:3100'
```

## Restart all observability on host-a
```bash
ssh root@127.0.0.1 'cd /opt/monitoring && docker-compose restart'
```

## Force Prometheus target refresh
```bash
curl -X POST http://127.0.0.1:9090/-/reload
```

## Clear Loki cache
```bash
ssh root@127.0.0.1 'rm -rf /loki/cache && systemctl restart loki'
```

---

# APPENDIX B: Agent Matrix

| Phase | Component | Tool | Verification | Timeout |
|-------|-----------|------|--------------|---------|
| 1 | Kernel/cgroup | ssh+grep | /proc/filesystems | 10s |
| 2 | Go tests | go test | All pass | 30s |
| 3 | Prometheus config | promtool | "valid" | 5s |
| 4 | Grafana dashboards | curl /api/search | Titles | 10s |
| 5 | Loki logs | logcli | Entries | 15s |
| 6 | NixOS rebuild | nixos-rebuild | Exit 0 | 300s |
| 7 | Docker Compose | docker-compose | All Up | 60s |
| 8 | LXD profile | lxc | Container running | 30s |
| 9 | E2E smoke test | curl+logcli | All metrics | 120s |

---

# APPENDIX C: Quick Reference

## Key Commands
```bash
curl -s localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {job:.labels.job, health:.health}'
promtool check config monitoring/prometheus/prometheus.yml
loki --config.file=loki.yaml --config.expand-env=true
logcli query '{host="forge"}' --limit=10 --addr=http://127.0.0.1:3100
go test ./pkg/metrics/... -race -count=1 -v
```

## Default Ports
- Prometheus: 9090
- Grafana: 3000
- Loki: 3100
- Promtail: 9080
- node_exporter: 9100
- VictoriaMetrics: 8428

---

## FORGE STAMP

**Battle Plan ID**: S67-Observability-Stack-Log-Aggregation  
**Generated**: 2026-02-26  
**Version**: 1.0  
**Status**: Ready for Deployment  
**Total Steps**: 120  
**Estimated Duration**: 8-12 hours  

**Signed Off**:
```
Warmonger v2.4.1
Commit Cadence: Every 4-5 steps + phase gates
Verification: curl, jq, logcli, git status
Bash Commands: Exact, quoted, with expected output
Debug Branches: First-step diagnostics for every failure
No blocked items — all prerequisites in place
```

**This battle plan is LIVE. Execute with caution.**

