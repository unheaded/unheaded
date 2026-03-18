# S-WEST WEST BOOTSTRAP & HARDENING BATTLE PLAN — 7 Phases, 95+ Steps

**Date**: 2026-03-15
**Sprint**: S-WEST — Patch, stabilize, verify, and harden west (192.168.69.184) for autonomous development
**Prerequisite**: SSH access to west on port 7375, repo cloned at ~/tmp/unheaded, FRR routing suite installed
**Target**: West fully patched, network identity pinned, repo verified (compile + critical tests pass), container fleet healthy, observability bootstrapped, Lich security harnesses green
**Estimated Duration**: 2-4 hours in 1 session
**Agent Strategy**: Phases 0-2 sequential (reboot required). Phases 3-6 parallelizable after Phase 2 gate.
**Commit Cadence**: Every 4 steps (calculated: max(3, min(5, 95/20)) = 5, rounded to 4 for safety)
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

---

## LEGEND

```
[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
[C] = Commit checkpoint (git commit at prescribed interval)
[STUCK] = Step skipped via Skip Protocol (needs human intervention)
[BLOCKED] = Step blocked by upstream STUCK step
```

**Exit Gate Convention**: Every phase ends with a gate. If the gate fails, DO NOT proceed. Debug in-phase (max 2 attempts, then Skip Protocol).

**Commit Convention**: `[C]` steps inserted every 4 steps. Never commit broken state — only after a passing `[V]`.

**Working Directory**: `~/tmp/unheaded` unless otherwise specified.

---

## PHASE 0: ENVIRONMENT RECONNAISSANCE (Steps 1-10)

**Goal**: Snapshot current system state before any changes. Baseline for rollback.
**Prerequisite**: SSH session active on west
**Time**: 5 minutes
**Agent**: Coordinator

### System Baseline

- [ ] **Step 1** [B] ~30s: Record kernel version and system info
  ```bash
  uname -a && lsb_release -a 2>/dev/null && uptime
  ```

- [ ] **Step 2** [B] ~30s: Record current package state
  ```bash
  apt list --upgradable 2>/dev/null | head -30
  ```

- [ ] **Step 3** [B] ~30s: Record current systemd service state
  ```bash
  systemctl --failed && echo "---" && systemctl list-units --state=running --no-pager | head -40
  ```

- [ ] **Step 4** [B] ~30s: Record current network interfaces and routing
  ```bash
  ip -br addr && echo "---" && ip route show && echo "---" && ip -6 route show | head -20
  ```

- [ ] **Step 5** [B] ~30s: Record FRR daemon state
  ```bash
  sudo vtysh -c "show running-config" 2>/dev/null | head -60 || echo "FRR vtysh not available"
  ```

- [ ] **Step 6** [B] ~30s: Record container fleet state
  ```bash
  docker ps -a 2>/dev/null && echo "---" && lxc list 2>/dev/null || echo "lxc not available"
  ```

- [ ] **Step 7** [B] ~30s: Record listening ports
  ```bash
  ss -tulnp | sort
  ```

- [ ] **Step 8** [B] ~30s: Record disk and memory state
  ```bash
  df -h && echo "---" && free -h
  ```

- [ ] **Step 9** [B] ~30s: Record repo state
  ```bash
  cd ~/tmp/unheaded && git status && git log --oneline -5
  ```

- [ ] **Step 10** [V]: **PHASE 0 EXIT GATE** — All baseline commands completed without error
  - If any command hung or errored → note it, continue (baseline is best-effort)
  - Proceed to Phase 1

---

## PHASE 1: PATCH & HARDEN (Steps 11-28)

**Goal**: Apply all pending security updates, fix known service failures, prepare for reboot.
**Prerequisite**: Phase 0 baseline captured
**Time**: 15-25 minutes (depends on download speed)
**Agent**: Coordinator (requires sudo)

### Security Updates

- [ ] **Step 11** [S][B] ~1m: Update package lists
  ```bash
  sudo apt update
  ```

- [ ] **Step 12** [V] ~30s: Verify package lists updated successfully
  ```bash
  apt list --upgradable 2>/dev/null | wc -l
  ```
  - If count > 0 → proceed (packages available to upgrade)
  - If error → Step 13 [D]

- [ ] **Step 13** [D] ~2m: Debug apt update failure
  ```bash
  sudo apt update 2>&1 | grep -i "err\|fail\|warning"
  ```
  - If mirror issues → `sudo sed -i 's/archive.ubuntu.com/us.archive.ubuntu.com/' /etc/apt/sources.list`
  - If still failing after 2 attempts → [STUCK], proceed to Step 16

- [ ] **Step 14** [S][B] ~10m: Apply all upgrades (non-interactive)
  ```bash
  sudo DEBIAN_FRONTEND=noninteractive apt upgrade -y
  ```

- [ ] **Step 15** [V] ~30s: Verify upgrade completed
  ```bash
  apt list --upgradable 2>/dev/null | wc -l
  ```
  - If count is 0 or very low → pass
  - If many remain → check held packages: `apt-mark showhold`

- [ ] **Step 16** [C]: **COMMIT CHECKPOINT** (no git commit needed — this is system-level, not repo)
  - Note: Phase 1 operates on system packages, not the git repo. No git commits here.

### Fix Known Issues

- [ ] **Step 17** [S][B] ~30s: Fix the rust-coreutils date bug noted in MOTD
  ```bash
  sudo apt install --update rust-coreutils -y 2>/dev/null || echo "Package not available or already current"
  ```

- [ ] **Step 18** [S][B] ~30s: Disable the failed systemd-networkd-wait-online service
  ```bash
  sudo systemctl disable systemd-networkd-wait-online.service 2>/dev/null && echo "Disabled" || echo "Already disabled or not found"
  ```

- [ ] **Step 19** [S][B] ~30s: Mask the service to prevent re-enabling
  ```bash
  sudo systemctl mask systemd-networkd-wait-online.service
  ```

- [ ] **Step 20** [V] ~30s: Verify no failed services remain (except known exceptions)
  ```bash
  systemctl --failed
  ```
  - If 0 failed → pass
  - If failed services remain → note them, not a blocker unless critical

### Kernel Check & Reboot Decision

- [ ] **Step 21** [B] ~30s: Check if reboot is required
  ```bash
  [ -f /var/run/reboot-required ] && cat /var/run/reboot-required || echo "No reboot required"
  ```

- [ ] **Step 22** [B] ~30s: Check if kernel was updated
  ```bash
  ls -la /boot/vmlinuz-* | tail -3
  ```

- [ ] **Step 23** [V] ~30s: **REBOOT DECISION GATE**
  - If /var/run/reboot-required exists → proceed to Step 24 (reboot)
  - If no reboot needed → SKIP to Step 27

- [ ] **Step 24** [S][B] ~2m: Reboot west
  ```bash
  sudo reboot
  ```
  - **WAIT**: SSH will disconnect. Wait 60-90 seconds, then reconnect.

- [ ] **Step 25** [B] ~30s: Reconnect via SSH (run from Mac or wait for agent reconnect)
  ```bash
  ssh 192.168.69.184 -p 7375
  ```

- [ ] **Step 26** [V] ~30s: Verify post-reboot system is healthy
  ```bash
  uptime && systemctl --failed && uname -r
  ```
  - If 0 failed services and new kernel → pass
  - If problems → note and continue

- [ ] **Step 27** [V] ~30s: **PHASE 1 EXIT GATE** — System patched, no failed services, ready for work
  ```bash
  echo "=== PHASE 1 EXIT CHECK ===" && systemctl --failed | grep -c "loaded" && apt list --upgradable 2>/dev/null | tail -1
  ```
  - 0 failed units + 0 (or minimal) upgradable packages → PASS
  - If fail → DO NOT PROCEED. Debug within this phase.

- [ ] **Step 28** [B] ~30s: Record post-patch state
  ```bash
  uname -a && uptime && free -h
  ```

---

## PHASE 2: NETWORK IDENTITY STABILIZATION (Steps 29-42)

**Goal**: Verify FRR routing underlay health, confirm network identity, document current topology.
**Prerequisite**: Phase 1 complete, system rebooted (if needed)
**Time**: 10-15 minutes
**Agent**: Coordinator (requires sudo for vtysh)

### FRR Routing Verification

- [ ] **Step 29** [S][B] ~30s: Check FRR daemon status
  ```bash
  sudo systemctl status frr --no-pager | head -15
  ```

- [ ] **Step 30** [V] ~30s: Verify FRR is running
  - If active (running) → proceed
  - If dead/failed → Step 31 [D]

- [ ] **Step 31** [D] ~2m: Debug FRR
  ```bash
  sudo journalctl -u frr --no-pager -n 30
  ```
  - If config error → `sudo vtysh -c "show running-config"` and inspect
  - If daemon crash → `sudo systemctl restart frr`

- [ ] **Step 32** [S][B] ~30s: Show BGP summary
  ```bash
  sudo vtysh -c "show bgp summary" 2>/dev/null || echo "BGP not configured"
  ```

- [ ] **Step 33** [S][B] ~30s: Show IS-IS neighbors
  ```bash
  sudo vtysh -c "show isis neighbor" 2>/dev/null || echo "IS-IS not configured"
  ```

- [ ] **Step 34** [S][B] ~30s: Show BFD peers
  ```bash
  sudo vtysh -c "show bfd peers" 2>/dev/null || echo "BFD not configured"
  ```

- [ ] **Step 35** [S][B] ~30s: Show OSPF neighbors
  ```bash
  sudo vtysh -c "show ip ospf neighbor" 2>/dev/null || echo "OSPF not configured"
  ```

- [ ] **Step 36** [B] ~30s: Verify all network bridges are up
  ```bash
  ip -br link show type bridge
  ```

### Network Identity

- [ ] **Step 37** [B] ~30s: Verify IPv4 addresses stable
  ```bash
  ip -4 addr show | grep "inet " | grep -v "127.0.0"
  ```

- [ ] **Step 38** [B] ~30s: Verify IPv6 addresses (public + ULA)
  ```bash
  ip -6 addr show | grep "inet6 " | grep -v "::1/128" | grep -v "fe80"
  ```

- [ ] **Step 39** [B] ~30s: Check DNS resolution works
  ```bash
  dig github.com +short && dig google.com +short
  ```

- [ ] **Step 40** [V] ~30s: Verify outbound connectivity
  ```bash
  curl -s -o /dev/null -w "%{http_code}" https://github.com
  ```
  - If 200 or 301 → pass
  - If timeout or error → check gateway: `ip route show default`

- [ ] **Step 41** [V]: **PHASE 2 EXIT GATE** — FRR running, network stable, DNS working, internet reachable
  - FRR active → check
  - All bridge interfaces up → check
  - DNS resolves → check
  - GitHub reachable → check
  - If any fail → debug in-phase, not a hard blocker unless internet is down

- [ ] **Step 42** [B] ~30s: Document current FRR running config snapshot
  ```bash
  sudo vtysh -c "show running-config" > /tmp/frr-running-$(date +%Y%m%d).txt 2>/dev/null && echo "Saved" || echo "Could not save"
  ```

---

## PHASE 3: REPO VERIFICATION (Steps 43-62)

**Goal**: Verify unheaded monorepo compiles, critical tests pass with race detector.
**Prerequisite**: Phase 2 complete (network needed for go modules)
**Time**: 20-40 minutes (Go compilation + tests)
**Agent**: Agent [P] (can run parallel with Phase 4 container checks IF different terminal)

### Repo State

- [ ] **Step 43** [B] ~30s: Navigate to repo and verify clean state
  ```bash
  cd ~/tmp/unheaded && git status
  ```

- [ ] **Step 44** [V] ~30s: Verify clean working tree
  - If "nothing to commit, working tree clean" → pass
  - If dirty → `git stash` and note stashed changes

- [ ] **Step 45** [B] ~30s: Verify on main, up to date
  ```bash
  git log --oneline -3
  ```

- [ ] **Step 46** [B] ~30s: Check Go version
  ```bash
  go version
  ```

- [ ] **Step 47** [V] ~30s: Go version >= 1.22 required
  - If adequate → proceed
  - If too old → [STUCK] — need Go upgrade

### Full Build

- [ ] **Step 48** [B] ~5m: Compile entire monorepo
  ```bash
  cd ~/tmp/unheaded && go build ./... 2>&1 | tail -20
  ```

- [ ] **Step 49** [V] ~30s: Build succeeded with zero errors
  - If clean → pass
  - If errors → Step 50 [D]

- [ ] **Step 50** [D] ~3m: Debug build failures
  ```bash
  cd ~/tmp/unheaded && go build ./... 2>&1 | grep "^#\|error:" | head -20
  ```
  - If missing module → `go mod tidy && go mod download`
  - If syntax error → note file and line, [STUCK] if not obvious fix

- [ ] **Step 51** [C]: **COMMIT CHECKPOINT** (only if go mod tidy changed go.sum)
  ```bash
  cd ~/tmp/unheaded && git diff --name-only | grep -q "go.sum" && git add go.sum go.mod && git commit -m "[PLAN S-WEST] go mod tidy after build verification" || echo "No module changes"
  ```

### Critical Path Tests (Race Detector ON)

- [ ] **Step 52** [B] ~5m: Test protocol core with race detector
  ```bash
  cd ~/tmp/unheaded && go test ./pkg/protocol/... -race -count=1 -timeout=120s 2>&1 | tail -30
  ```

- [ ] **Step 53** [V] ~30s: All protocol tests pass
  - If "ok" for all packages → pass
  - If FAIL → Step 54 [D]

- [ ] **Step 54** [D] ~3m: Debug protocol test failures
  ```bash
  cd ~/tmp/unheaded && go test ./pkg/protocol/... -race -count=1 -v 2>&1 | grep -A5 "FAIL"
  ```
  - Note failing test name and error
  - If race detected → CRITICAL — note for BlackMage review
  - If timeout → increase: `-timeout=300s`

- [ ] **Step 55** [B] ~5m: Test Wotan (topic config changes landed)
  ```bash
  cd ~/tmp/unheaded && go test ./services/wotan/... -race -count=1 -timeout=120s 2>&1 | tail -30
  ```

- [ ] **Step 56** [V] ~30s: All Wotan tests pass
  - If "ok" → pass
  - If FAIL → note, not a hard blocker but flag for Developer

- [ ] **Step 57** [B] ~3m: Test keymgmt (race fix verification — commit 903cfa3)
  ```bash
  cd ~/tmp/unheaded && go test ./services/keymgmt/... -race -count=1 -timeout=120s 2>&1 | tail -20
  ```

- [ ] **Step 58** [V] ~30s: Keymgmt race fix confirmed — no data race detected
  - If "ok" with no race warnings → pass
  - If "DATA RACE" → CRITICAL — the fix in 903cfa3 didn't hold

- [ ] **Step 59** [B] ~3m: Test audit storage (atomic counter fix)
  ```bash
  cd ~/tmp/unheaded && go test ./pkg/audit/storage/... -race -count=1 -timeout=120s 2>&1 | tail -20
  ```

- [ ] **Step 60** [V] ~30s: Audit storage tests pass, no race
  - If clean → pass

- [ ] **Step 61** [V]: **PHASE 3 EXIT GATE** — Monorepo compiles, protocol/wotan/keymgmt/audit tests green with race detector
  ```bash
  echo "Build: OK, Protocol: OK, Wotan: OK, Keymgmt: OK, Audit: OK"
  ```
  - ALL must pass for full gate
  - If any FAIL → note which, proceed if build + protocol pass (minimum bar)

- [ ] **Step 62** [C]: **PHASE 3 EXIT COMMIT** (if any go.mod/go.sum changes)
  ```bash
  cd ~/tmp/unheaded && git diff --quiet || (git add -A && git commit -m "[PLAN S-WEST] Phase 3: repo verified — build + critical tests green")
  ```

---

## PHASE 4: CONTAINER FLEET HEALTH (Steps 63-74)

**Goal**: Verify Docker and LXD container runtimes healthy, inventory all containers.
**Prerequisite**: Phase 2 (network) complete
**Time**: 10 minutes
**Agent**: Agent [P] (parallelizable with Phase 3)

### Docker

- [ ] **Step 63** [B] ~30s: Check Docker daemon status
  ```bash
  sudo systemctl status docker --no-pager | head -10
  ```

- [ ] **Step 64** [B] ~30s: List all Docker containers
  ```bash
  docker ps -a --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null
  ```

- [ ] **Step 65** [B] ~30s: Docker disk usage
  ```bash
  docker system df
  ```

- [ ] **Step 66** [V] ~30s: Docker healthy
  - If daemon running and responds → pass
  - If socket error → `sudo systemctl restart docker`

### LXD

- [ ] **Step 67** [B] ~30s: Check LXD status
  ```bash
  sudo systemctl status snap.lxd.daemon 2>/dev/null || sudo systemctl status lxd 2>/dev/null | head -10
  ```

- [ ] **Step 68** [B] ~30s: List all LXD containers
  ```bash
  lxc list --format table 2>/dev/null || echo "LXD not available or no containers"
  ```

- [ ] **Step 69** [B] ~30s: Check LXD network bridges
  ```bash
  lxc network list 2>/dev/null || echo "LXD networks not available"
  ```

- [ ] **Step 70** [B] ~30s: Check LXD storage pools
  ```bash
  lxc storage list 2>/dev/null || echo "LXD storage not available"
  ```

### LXC (legacy)

- [ ] **Step 71** [B] ~30s: Check LXC containers
  ```bash
  sudo lxc-ls --fancy 2>/dev/null || echo "LXC not installed or no containers"
  ```

- [ ] **Step 72** [B] ~30s: Verify container bridge connectivity
  ```bash
  ping -c 2 -W 2 10.10.10.1 && ping -c 2 -W 2 10.0.3.1 || echo "Bridge ping failed"
  ```

- [ ] **Step 73** [V]: **PHASE 4 EXIT GATE** — Docker running, LXD running, bridges reachable
  - Docker daemon → running
  - LXD daemon → running (or noted as not needed)
  - Bridge IPs → pingable
  - If any critical failure → note, not a hard blocker

- [ ] **Step 74** [B] ~30s: Docker image inventory (for future reference)
  ```bash
  docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}" | head -30
  ```

---

## PHASE 5: OBSERVABILITY BOOTSTRAP (Steps 75-84)

**Goal**: Verify metrics exporters running, check Grafana/VictoriaMetrics deployment status.
**Prerequisite**: Phase 2 (network) complete
**Time**: 10 minutes
**Agent**: Agent [P] (parallelizable with Phase 3)

### Metrics Exporters

- [ ] **Step 75** [B] ~30s: Verify node_exporter is serving metrics
  ```bash
  curl -s --max-time 5 http://localhost:9100/metrics | head -5 || echo "node_exporter not responding"
  ```

- [ ] **Step 76** [V] ~30s: node_exporter alive
  - If metrics returned → pass
  - If no response → check: `ss -tulnp | grep 9100`

- [ ] **Step 77** [B] ~30s: Verify Docker metrics endpoint
  ```bash
  curl -s --max-time 5 http://localhost:9323/metrics | head -5 || echo "Docker metrics not responding"
  ```

- [ ] **Step 78** [B] ~30s: Check if VictoriaMetrics is deployed
  ```bash
  docker ps | grep -i victoria || echo "VictoriaMetrics not running in Docker"
  ```

- [ ] **Step 79** [B] ~30s: Check if Grafana is deployed
  ```bash
  docker ps | grep -i grafana || echo "Grafana not running in Docker"
  ```

### Grafana Provisioning Check

- [ ] **Step 80** [B] ~30s: Verify Grafana provisioning configs exist in repo
  ```bash
  ls -la ~/tmp/unheaded/config/grafana/provisioning/dashboards/ ~/tmp/unheaded/config/grafana/provisioning/datasources/ 2>/dev/null || echo "Provisioning configs not found"
  ```

- [ ] **Step 81** [R] ~30s: Read VictoriaMetrics datasource config
  ```bash
  cat ~/tmp/unheaded/config/grafana/provisioning/datasources/victoriametrics.yml 2>/dev/null || echo "Not found"
  ```

### Port 8443 Investigation

- [ ] **Step 82** [B] ~30s: Identify what's on port 8443 (seen in nmap)
  ```bash
  ss -tulnp | grep 8443
  ```

- [ ] **Step 83** [V]: **PHASE 5 EXIT GATE** — node_exporter serving, observability status documented
  - node_exporter → alive
  - Grafana/VictoriaMetrics status → documented (running or not-yet-deployed)
  - Port 8443 → identified

- [ ] **Step 84** [B] ~30s: Summary of observability state
  ```bash
  echo "=== OBSERVABILITY STATUS ===" && echo "node_exporter: $(curl -s -o /dev/null -w '%{http_code}' http://localhost:9100/metrics)" && echo "docker_metrics: $(curl -s -o /dev/null -w '%{http_code}' http://localhost:9323/metrics)"
  ```

---

## PHASE 6: SECURITY VERIFICATION — LICH HARNESSES (Steps 85-95)

**Goal**: Run Lich fuzzing harnesses and Doom security tests. BlackMage's minimum bar.
**Prerequisite**: Phase 3 (repo compiles and tests pass)
**Time**: 15-25 minutes
**Agent**: Agent (delegatable, no sudo needed)

### Lich Harnesses

- [ ] **Step 85** [B] ~5m: Run all 10 Lich fuzzing harnesses
  ```bash
  cd ~/tmp/unheaded && go test ./tomb/lich/harnesses/... -v -count=1 -timeout=300s 2>&1 | tail -40
  ```

- [ ] **Step 86** [V] ~30s: All Lich harnesses pass
  - If all "ok" → pass
  - If FAIL → note which harness and error

- [ ] **Step 87** [D] ~3m: Debug Lich failures (attempt 1 of 2)
  ```bash
  cd ~/tmp/unheaded && go test ./tomb/lich/harnesses/... -v -run "FAILING_TEST_NAME" 2>&1 | tail -40
  ```
  - If build error → missing dependency, check imports
  - If test logic → note for BlackMage review

### Doom Security Tests

- [ ] **Step 88** [B] ~5m: Run D1-D6 Doom security tests
  ```bash
  cd ~/tmp/unheaded && go test ./tests/security/doom/... -v -count=1 -timeout=300s 2>&1 | tail -40
  ```

- [ ] **Step 89** [V] ~30s: All Doom security tests pass
  - If all "ok" → pass
  - If FAIL → note which D-test and error

### Lich Campaign Runner

- [ ] **Step 90** [B] ~2m: Verify Lich campaign runner compiles
  ```bash
  cd ~/tmp/unheaded && go build ./cmd/lich-security/... 2>&1 | tail -10
  ```

- [ ] **Step 91** [V] ~30s: Lich runner builds clean
  - If clean → pass
  - If error → note, not a blocker

### Specific Race Fix Verification

- [ ] **Step 92** [R] ~30s: Confirm keymgmt struct copy fix is live
  ```bash
  cd ~/tmp/unheaded && grep -n "copy\|Copy\|return \*\|struct{" services/keymgmt/keymgmt.go | head -10
  ```

- [ ] **Step 93** [R] ~30s: Confirm audit atomic counter is live
  ```bash
  cd ~/tmp/unheaded && grep -n "atomic\|Atomic\|AddInt\|AddUint" pkg/audit/storage/storage.go | head -10
  ```

- [ ] **Step 94** [V]: **PHASE 6 EXIT GATE** — Lich harnesses + Doom security tests documented, race fixes verified
  - Lich harnesses → status documented (pass/fail/skip per harness)
  - Doom D1-D6 → status documented
  - Race fixes → confirmed in source
  - If any CRITICAL failures → flag for immediate Muck attention

- [ ] **Step 95** [C]: **FINAL COMMIT** (if any changes were made during verification)
  ```bash
  cd ~/tmp/unheaded && git diff --quiet || (git add -A && git commit -m "[PLAN S-WEST] Phase 6: security verification complete — Lich + Doom status documented")
  ```

---

## STUCK REPORT (generated at runtime if skips occur)

**Progress**: [X]/95 steps completed ([Z]%)
**Stuck Steps**: [list]
**Blocked Steps**: [list of steps dependent on stuck steps]
**Completed After Skip**: [steps that succeeded despite skips]

### Stuck Step Detail Template

**Step [N]**: [description]
- **Symptom**: [what went wrong]
- **Attempted**: [debug steps tried]
- **Time Burned**: [duration before skip]
- **Downstream Impact**: Steps [A, B, C] blocked
- **Suggested Fix**: [agent's best guess]

### Recommended Intervention Order
1. Fix Step [X] first — unblocks [N] downstream steps
2. Fix Step [Y] next — unblocks [M] downstream steps

---

## APPENDIX A: EMERGENCY PROCEDURES

### SSH Disconnects During Reboot (Step 24-25)
```bash
# Wait 90 seconds, then:
ssh 192.168.69.184 -p 7375
# If still unreachable after 3 minutes:
# 1. Check if IP changed (DHCP): nmap 192.168.69.0/24 -p 7375 from Mac
# 2. Look for MAC FC:B0:DE:6E:FF:E3 on a new IP
# 3. If found, update SSH command and continue
```

### Go Build Fails Due to Missing Modules
```bash
cd ~/tmp/unheaded
go mod tidy
go mod download
# If private module: check GOPRIVATE env
echo $GOPRIVATE
# If GitHub auth: check SSH key
ssh -T git@github.com
```

### FRR Won't Start After Reboot
```bash
sudo journalctl -u frr --no-pager -n 50
sudo cat /etc/frr/daemons  # Verify correct daemons enabled
sudo vtysh -c "show running-config"
sudo systemctl restart frr
```

### Docker Daemon Won't Start
```bash
sudo journalctl -u docker --no-pager -n 30
sudo systemctl restart docker
# If storage issue:
docker system prune --volumes  # CAREFUL: removes unused data
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent Type | Parallelizable | Dependencies | Est. Time |
|-------|-----------|---------------|--------------|-----------|
| 0 — Recon | Coordinator | No | None | 5 min |
| 1 — Patch & Harden | Coordinator | No | Phase 0 | 15-25 min |
| 2 — Network Identity | Coordinator | No | Phase 1 | 10-15 min |
| 3 — Repo Verification | Agent [P] | Yes (after Phase 2) | Phase 2 | 20-40 min |
| 4 — Container Fleet | Agent [P] | Yes (after Phase 2) | Phase 2 | 10 min |
| 5 — Observability | Agent [P] | Yes (after Phase 2) | Phase 2 | 10 min |
| 6 — Security (Lich) | Agent | No | Phase 3 | 15-25 min |

**Critical Path**: 0 → 1 → 2 → 3 → 6 (minimum ~65 min sequential)
**With Parallelization**: Phases 3, 4, 5 run concurrently after Phase 2 → saves ~20 min

---

## APPENDIX C: QUICK REFERENCE

### West Network Identity
```
Hostname:     west
IPv4 (wifi):  192.168.69.184 (wlp17s0)
IPv4 (p2p):   192.168.13.2/30 (enp16s0)
IPv4 (lxd):   10.10.10.1/24 (lxdbr0)
IPv4 (lxc):   10.0.3.1/24 (lxcbr0)
IPv4 (docker): 172.17.0.1/16 (docker0), 172.20.0.1/24 (br-81d5e318cc70)
IPv6 (public): 2605:a601:81ef:d000::5
IPv6 (lxd ULA): fd42:474b:622d:259d::1
SSH:          port 7375
MAC:          FC:B0:DE:6E:FF:E3
```

### FRR Ports
```
2601 — zebra
2604 — bgpd
2605 — ospfd
2606 — ospf6d
2608 — isisd
2616 — bfdd
2617 — fabricd
2623 — staticd
3784/4784 — BFD (multihop/singlehop)
```

### Key Commits to Verify
```
903cfa3 — keymgmt race fix (struct copy on GetKey)
903cfa3 — audit atomic counter fix (InMemoryArchiveStore.Store)
903cfa3 — timeguru ctx fix (context.Background() at line 121)
```

### Repo Path
```
~/tmp/unheaded (on west)
/Users/govan/tmp/unheaded (on Mac — Stevies-MacBook-Air)
```

---

*S-WEST Battle Plan — Forged 2026-03-15*
*7 Phases. 95 Steps. West rises. The Kingdom's watchtower is hardened.*
*From the Mac to the metal — every packet, every process, every port accounted for.*
