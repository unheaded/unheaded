# CATCH-UP BATTLE PLAN — 13 Phases, 280+ Steps

# "THE RECKONING" — Everything Cowork Couldn't Touch

**Date**: 2026-02-26
**Sprint**: S71+ — Bare Metal Reckoning
**Prerequisite**: Repo cloned at ~/tmp/unheaded, Go 1.24+, NixOS host, sudo access
**Target**: All deferred bare-metal work from S40-S70 validated, deployed, or explicitly killed
**Estimated Duration**: 40-60 hours across 8-12 sessions
**Agent Strategy**: Phases 0-3 SEQUENTIAL (foundation). Phases 4-7 PARALLELIZABLE. Phases 8-12 SEQUENTIAL (integration).
**Commit Cadence**: Every 4 steps (max(3, min(5, 280/20)) = 5, conservative = 4)
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts. Log STUCK marker. Move forward.

---

## GAP ANALYSIS — What Cowork Wrote But Never Validated

| Category | Files Written | Validated on Bare Metal | Status |
|----------|--------------|------------------------|--------|
| NixOS modules | 53 .nix files | 0 | ❌ ZERO validation |
| Docker IaC | 67 files | 0 (no docker host) | ❌ ZERO validation |
| LXD IaC | 78 files | 0 (no LXD socket) | ❌ ZERO validation |
| Go code (S67-S69) | pkg/metrics, pkg/anamnesis, cmd/routing-health | 0 (never compiled) | ❌ NEVER BUILT |
| Monitoring stack | Prometheus/Grafana/Loki/VM/Alertmanager configs | 0 | ❌ ZERO deployment |
| Firewall configs | OPNsense + IPFire + nftables | 0 | ❌ ZERO deployment |
| Routing configs | FRR + BIRD + OSPF + IS-IS + MPLS | 0 | ❌ ZERO deployment |
| Suricata IDS | rules, configs, NixOS module | 0 | ❌ ZERO deployment |
| eBPF programs | Skeleton loaders only | 0 | ❌ BLOCKED since S1 |
| WireGuard tunnel | Config written | 0 | ❌ No key exchange |
| vLLM/ROCm AI | Service configs | 0 | ❌ No GPU test |
| S70 handoff items | 9 Part-A + 7 Part-B tasks | 0 | ❌ NOT STARTED |

**Bottom line**: ~300 files of infrastructure config exist in git. Zero have touched real hardware.

---

## LEGEND

[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
[C] = Commit checkpoint
[STUCK] = Step skipped via Skip Protocol
[BLOCKED] = Step blocked by upstream STUCK

---

## PHASE 0: TRIAGE & ENVIRONMENT VERIFICATION (Steps 1-25)

**Goal**: Confirm what we have, what builds, what's broken. No fixes yet — pure assessment.
**Prerequisite**: Dev machine with Go 1.24+, git, sudo
**Time**: 45 minutes
**Agent**: Coordinator (needs interactive debugging)

### Hardware Inventory

- [ ] **Step 1** [B] ~30s: Identify hosts available
  ```bash
  hostname && uname -a && cat /etc/os-release | head -5
  ```

- [ ] **Step 2** [B] ~30s: Check CPU/RAM/disk
  ```bash
  nproc && free -h | head -2 && df -h / /home 2>/dev/null | head -5
  ```

- [ ] **Step 3** [B] ~30s: Check GPU (if host-a / gaming desktop)
  ```bash
  lspci | grep -i "vga\|3d\|display" && rocminfo 2>/dev/null | grep "Name:" | head -3 || echo "No ROCm"
  ```

- [ ] **Step 4** [B] ~30s: Check kernel eBPF support
  ```bash
  uname -r && cat /proc/config.gz 2>/dev/null | gunzip | grep -i "CONFIG_BPF" | head -10 || grep "CONFIG_BPF" /boot/config-$(uname -r) 2>/dev/null | head -10 || echo "Cannot read kernel config"
  ```

- [ ] **Step 5** [B] ~30s: Check eBPF toolchain
  ```bash
  for tool in bpftool clang llc ip tc nft; do
    which $tool 2>/dev/null && echo "$tool: $(command -v $tool)" || echo "$tool: MISSING"
  done
  ```

- [ ] **Step 6** [B] ~30s: Check container runtimes
  ```bash
  for rt in docker lxc lxd nix-build nixos-rebuild containerd; do
    which $rt 2>/dev/null && echo "$rt: $(command -v $rt)" || echo "$rt: MISSING"
  done
  ```

- [ ] **Step 7** [B] ~30s: Check network tools
  ```bash
  for tool in wg wireguard-go vtysh birdc frr; do
    which $tool 2>/dev/null && echo "$tool: $(command -v $tool)" || echo "$tool: MISSING"
  done
  ```

- [ ] **Step 8** [C]: **COMMIT CHECKPOINT** — Environment inventory captured
  ```bash
  # No git commit yet — just record output to docs/bare-metal/INVENTORY.md
  ```

### Go Toolchain Validation

- [ ] **Step 9** [B] ~10s: Verify Go version
  ```bash
  go version
  ```

- [ ] **Step 10** [V] ~5s: Go >= 1.24 required
  - If pass → Step 11
  - If fail → STOP. Install Go 1.24+.

- [ ] **Step 11** [B] ~60s: Full Go build
  ```bash
  cd ~/tmp/unheaded && go build ./... 2>&1 | tail -30
  ```

- [ ] **Step 12** [V] ~5s: Build must exit 0
  - If pass → Step 15
  - If fail → Step 13

- [ ] **Step 13** [D] ~5m: Capture all build errors
  ```bash
  cd ~/tmp/unheaded && go build ./... 2>&1 | grep "^#\|cannot\|undefined\|imported\|duplicate" | sort -u
  ```

- [ ] **Step 14** [W] ~2m: Log build errors to docs/bare-metal/BUILD-ERRORS.md
  - Do NOT fix yet. Triage only. Count errors. Categorize (import, type, duplicate, syntax).

- [ ] **Step 15** [B] ~120s: Full test suite (allow failures — we're triaging)
  ```bash
  cd ~/tmp/unheaded && go test ./... -race -count=1 -timeout 120s 2>&1 | tail -40
  ```

- [ ] **Step 16** [B] ~10s: Count test results
  ```bash
  cd ~/tmp/unheaded && go test ./... -race -count=1 -timeout 120s 2>&1 | grep -E "^(ok|FAIL|---)" | sort | uniq -c | sort -rn
  ```

- [ ] **Step 17** [C]: **COMMIT CHECKPOINT** — Go build/test triage complete

### NixOS Validation

- [ ] **Step 18** [B] ~30s: Check Nix availability
  ```bash
  nix --version 2>/dev/null && nixos-rebuild --help 2>/dev/null | head -3 || echo "NixOS tools not available"
  ```

- [ ] **Step 19** [B] ~60s: Parse all NixOS modules (syntax only)
  ```bash
  cd ~/tmp/unheaded/nixos
  for f in modules/*.nix modules/services/*.nix tests/*.nix flake.nix; do
    [ -f "$f" ] || continue
    nix-instantiate --parse "$f" > /dev/null 2>&1 && echo "OK: $f" || echo "FAIL: $f"
  done
  ```

- [ ] **Step 20** [B] ~10s: Count NixOS parse results
  ```bash
  cd ~/tmp/unheaded/nixos
  ok=0; fail=0
  for f in modules/*.nix modules/services/*.nix tests/*.nix flake.nix; do
    [ -f "$f" ] || continue
    nix-instantiate --parse "$f" > /dev/null 2>&1 && ok=$((ok+1)) || fail=$((fail+1))
  done
  echo "NixOS modules: $ok OK, $fail FAIL"
  ```

- [ ] **Step 21** [V] ~5s: Record failures. Do NOT fix yet.
  - If 0 failures → Step 22 (excellent)
  - If >0 failures → Log to docs/bare-metal/NIX-PARSE-ERRORS.md

### Config Lint

- [ ] **Step 22** [B] ~60s: YAML lint all monitoring/routing configs
  ```bash
  cd ~/tmp/unheaded
  for f in $(find monitoring routing -name "*.yml" -o -name "*.yaml" 2>/dev/null); do
    python3 -c "import yaml,sys; yaml.safe_load(open('$f'))" 2>&1 && echo "OK: $f" || echo "FAIL: $f"
  done
  ```

- [ ] **Step 23** [B] ~30s: Check Docker Compose validity
  ```bash
  cd ~/tmp/unheaded
  for f in $(find . -name "docker-compose*.yml" 2>/dev/null); do
    docker compose -f "$f" config --quiet 2>&1 && echo "OK: $f" || echo "FAIL: $f"
  done
  ```

- [ ] **Step 24** [W] ~5m: Write Phase 0 triage report → docs/bare-metal/TRIAGE-REPORT.md
  ```
  Contents:
  - Host inventory (CPU/RAM/disk/GPU)
  - Kernel features present/missing
  - Tool availability matrix
  - Go build: pass/fail + error count
  - Go test: pass/fail/skip counts
  - NixOS parse: pass/fail count
  - YAML lint: pass/fail count
  - Docker compose: pass/fail count
  ```

- [ ] **Step 25** [V][C]: **PHASE 0 EXIT GATE** — Triage report written. We know EXACTLY what's broken.
  ```bash
  test -f ~/tmp/unheaded/docs/bare-metal/TRIAGE-REPORT.md && echo "GATE PASS" || echo "GATE FAIL"
  ```
  - If pass → Phase 1
  - If fail → DO NOT PROCEED

---

## PHASE 1: GO BUILD FIXES (Steps 26-55)

**Goal**: `go build ./...` exits 0. `go test ./... -race` all pass. Zero regressions.
**Prerequisite**: Phase 0 triage report exists
**Time**: 2-3 hours
**Agent**: Coordinator

### Fix Build Errors (S67-S69 code never compiled on real Go)

- [ ] **Step 26** [B] ~60s: Full build with verbose errors
  ```bash
  cd ~/tmp/unheaded && go build ./... 2>&1 | tee /tmp/build-errors.txt
  ```

- [ ] **Step 27** [R] ~30s: Categorize errors
  ```bash
  grep "^#" /tmp/build-errors.txt | sort -u
  ```

- [ ] **Step 28** [B] ~5m: Fix each failing package in dependency order
  - Start with leaf packages (no internal deps)
  - Fix import paths, type mismatches, duplicate declarations
  - `pkg/metrics/` first (new in S67)
  - `pkg/anamnesis/` second (new in S68)
  - `cmd/routing-health/` third (new in S69)

- [ ] **Step 29** [V] ~60s: Rebuild after fixes
  ```bash
  cd ~/tmp/unheaded && go build ./... 2>&1 | tail -5
  ```
  - If exit 0 → Step 31
  - If fail → Step 30

- [ ] **Step 30** [D] ~5m: Iterate on remaining errors. Max 3 rounds.
  ```bash
  cd ~/tmp/unheaded && go build ./... 2>&1 | grep -c "^#"
  ```
  - If count decreasing → continue fixing
  - If stuck → STUCK protocol. Log remaining errors.

- [ ] **Step 31** [C]: **COMMIT CHECKPOINT** — go build clean
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "fix(s71): go build clean — fix S67-S69 compilation errors"
  ```

### Fix Test Failures

- [ ] **Step 32** [B] ~120s: Run full test suite
  ```bash
  cd ~/tmp/unheaded && go test ./... -race -count=1 -timeout 120s 2>&1 | tee /tmp/test-results.txt
  ```

- [ ] **Step 33** [B] ~10s: Extract failures
  ```bash
  grep "^FAIL\|--- FAIL" /tmp/test-results.txt | sort
  ```

- [ ] **Step 34** [B] ~5m: Fix pkg/metrics tests (most likely issues: socket missing, /proc paths)
  ```bash
  cd ~/tmp/unheaded && go test ./pkg/metrics/... -race -v -count=1 2>&1 | tail -30
  ```

- [ ] **Step 35** [B] ~5m: Fix pkg/anamnesis tests
  ```bash
  cd ~/tmp/unheaded && go test ./pkg/anamnesis/... -race -v -count=1 2>&1 | tail -30
  ```

- [ ] **Step 36** [B] ~3m: Fix cmd/routing-health tests
  ```bash
  cd ~/tmp/unheaded && go test ./cmd/routing-health/... -race -v -count=1 2>&1 | tail -20
  ```

- [ ] **Step 37** [C]: **COMMIT CHECKPOINT** — test fixes
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "fix(s71): test suite green — race detector clean"
  ```

### S70 Part B Code Fixes (Deferred from handoff)

- [ ] **Step 38** [B] ~3m: B1 — Captain /tmp fix
  ```bash
  grep -rn "os.TempDir\|/tmp\|ioutil.TempDir\|os.MkdirTemp" ~/tmp/unheaded/services/captain/ ~/tmp/unheaded/cmd/captain/ 2>/dev/null
  ```
  - Fix any unguarded /tmp writes → use `CAPTAIN_DATA_DIR` env var

- [ ] **Step 39** [B] ~5m: B2 — SBOM in CI
  ```bash
  # Add anchore/sbom-action + go-licenses check to .github/workflows/ci.yml
  ```

- [ ] **Step 40** [B] ~5m: B3 — gRPC client race fix (sync.Once pattern)
  ```bash
  # Fix pkg/wotan-client/client.go:615 — RLock/RUnlock/Lock window
  ```

- [ ] **Step 41** [B] ~5m: B3 verification — concurrent test
  ```bash
  cd ~/tmp/unheaded && go test ./pkg/wotan-client/... -race -run TestGetOrCreate -v -count=1
  ```

- [ ] **Step 42** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "fix(s71): P0/P1 code fixes — Captain tmp, SBOM CI, gRPC race"
  ```

- [ ] **Step 43** [B] ~5m: B4 — Wotan nil → degraded mode
  ```bash
  grep -rn "wotan.*nil\|wotanClient.*nil" --include="*.go" ~/tmp/unheaded/ | grep -v "_test\|.git\|vendor"
  ```
  - Replace silent drops with `log.Warn()` + metric increment

- [ ] **Step 44** [B] ~5m: B5 — gRPC TLS skeleton
  ```bash
  grep -rn "grpc.Dial\|grpc.NewClient\|WithInsecure" --include="*.go" ~/tmp/unheaded/ | grep -v ".git\|_test\|vendor"
  ```
  - Add `UNHEADED_GRPC_TLS=1` env flag with TLS 1.3 minimum

- [ ] **Step 45** [B] ~2m: B6 — Go module version
  ```bash
  head -5 ~/tmp/unheaded/go.mod
  ```
  - If not 1.24 → `go mod edit -go=1.24 && go mod tidy`

- [ ] **Step 46** [B] ~2m: B7 — Remove dead BroadcastJSON
  ```bash
  grep -rn "BroadcastJSON" --include="*.go" ~/tmp/unheaded/ | grep -v ".git\|_test"
  ```
  - Delete if found. Run tests. Commit.

- [ ] **Step 47** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "fix(s71): P1/P2 code fixes — Wotan degraded, gRPC TLS, go 1.24, dead code"
  ```

### Final Build Verification

- [ ] **Step 48** [B] ~120s: Full build + test
  ```bash
  cd ~/tmp/unheaded && go build ./... && go test ./... -race -count=1 -timeout 120s 2>&1 | tail -20
  ```

- [ ] **Step 49** [V] ~5s: ALL PASS required
  - If pass → Step 50
  - If fail → Debug. Max 2 rounds. Then STUCK.

- [ ] **Step 50** [B] ~30s: Count pass/fail/skip
  ```bash
  cd ~/tmp/unheaded && go test ./... -race -count=1 -timeout 120s 2>&1 | grep -cE "^ok" && echo "---" && go test ./... -race -count=1 -timeout 120s 2>&1 | grep -cE "^FAIL"
  ```

- [ ] **Step 51** [B] ~10s: Verify no race conditions
  ```bash
  cd ~/tmp/unheaded && go test ./... -race -count=1 -timeout 120s 2>&1 | grep -i "race\|DATA RACE" || echo "No races detected"
  ```

- [ ] **Step 52** [W] ~2m: Update docs/bare-metal/TRIAGE-REPORT.md with build results

- [ ] **Step 53** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "chore(s71): Phase 1 complete — go build/test clean, S70 code fixes applied"
  ```

- [ ] **Step 54** [V]: **PHASE 1 EXIT GATE** — `go build ./...` exits 0, `go test ./... -race` all pass
  ```bash
  cd ~/tmp/unheaded && go build ./... && go test ./... -race -count=1 -timeout 120s 2>&1 | grep "^FAIL" | wc -l
  ```
  - Output must be `0`
  - If pass → Phase 2
  - If fail → DO NOT PROCEED

- [ ] **Step 55** [C]: Phase gate commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "[PLAN CATCH-UP] Phase 1 EXIT GATE PASSED — Go toolchain verified"
  ```

---

## PHASE 2: NIXOS FOUNDATION (Steps 56-85)

**Goal**: NixOS flake builds. All modules parse. nixos-rebuild dry-run succeeds.
**Prerequisite**: Phase 1 passed (Go builds clean)
**Time**: 2-3 hours
**Agent**: Coordinator [S]

### Flake Lock Generation

- [ ] **Step 56** [B] ~30s: Check current flake.lock state
  ```bash
  cat ~/tmp/unheaded/nixos/flake.lock 2>/dev/null | head -5 || echo "No flake.lock"
  ```

- [ ] **Step 57** [V] ~5s: If stub/empty → must generate real one
  - Stub = any file under 100 bytes or containing "STUB"

- [ ] **Step 58** [B] ~120s: Generate real flake.lock
  ```bash
  cd ~/tmp/unheaded/nixos && nix flake update 2>&1 | tail -10
  ```

- [ ] **Step 59** [V] ~5s: flake.lock exists and is non-trivial
  ```bash
  wc -c ~/tmp/unheaded/nixos/flake.lock
  ```
  - Must be > 1000 bytes

- [ ] **Step 60** [D] ~3m: If flake update fails, check flake.nix inputs
  ```bash
  head -30 ~/tmp/unheaded/nixos/flake.nix
  ```

- [ ] **Step 61** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add nixos/flake.lock && git commit -m "chore(nix): generate real flake.lock from nix flake update"
  ```

### Module Syntax Fixes

- [ ] **Step 62** [B] ~60s: Parse every .nix file
  ```bash
  cd ~/tmp/unheaded/nixos
  for f in $(find . -name "*.nix" -not -path "./.git/*"); do
    nix-instantiate --parse "$f" > /dev/null 2>&1 && echo "OK: $f" || echo "FAIL: $f"
  done 2>&1 | tee /tmp/nix-parse.txt
  ```

- [ ] **Step 63** [B] ~5s: Count failures
  ```bash
  grep -c "^FAIL" /tmp/nix-parse.txt
  ```

- [ ] **Step 64** [B] ~10m: Fix each failing module
  - Common issues: missing semicolons, undefined variables, wrong import paths
  - Fix in dependency order: base modules → service modules → test modules

- [ ] **Step 65** [V] ~60s: Re-parse all modules — 0 failures required
  ```bash
  cd ~/tmp/unheaded/nixos
  fail=0
  for f in $(find . -name "*.nix" -not -path "./.git/*"); do
    nix-instantiate --parse "$f" > /dev/null 2>&1 || { echo "STILL FAIL: $f"; fail=$((fail+1)); }
  done
  echo "Remaining failures: $fail"
  ```
  - If 0 → Step 67
  - If >0 → Step 66

- [ ] **Step 66** [D] ~5m: Debug remaining parse failures. Max 2 rounds.
  ```bash
  # For each FAIL, get specific error:
  nix-instantiate --parse <file> 2>&1
  ```

- [ ] **Step 67** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "fix(nix): all NixOS modules parse clean"
  ```

### NixOS Rebuild Test

- [ ] **Step 68** [B] ~180s: Dry-run host-a configuration
  ```bash
  cd ~/tmp/unheaded/nixos && nixos-rebuild dry-run --flake .#host-a 2>&1 | tail -30
  ```

- [ ] **Step 69** [V] ~5s: Dry-run must succeed
  - If pass → Step 72
  - If fail → Step 70

- [ ] **Step 70** [D] ~10m: Debug NixOS build errors
  ```bash
  cd ~/tmp/unheaded/nixos && nixos-rebuild dry-run --flake .#host-a 2>&1 | grep -i "error\|fail\|undefined\|missing"
  ```
  - Common: missing package names, wrong option paths, circular imports

- [ ] **Step 71** [D] ~5m: If host-a config references hardware not present, create minimal test config
  ```bash
  # May need a host-dev.nix that strips GPU/WireGuard for local testing
  ```

- [ ] **Step 72** [B] ~180s: Dry-run host-b configuration
  ```bash
  cd ~/tmp/unheaded/nixos && nixos-rebuild dry-run --flake .#host-b 2>&1 | tail -30
  ```

- [ ] **Step 73** [V] ~5s: host-b dry-run must succeed
  - If fail → debug same as host-a

- [ ] **Step 74** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "fix(nix): nixos-rebuild dry-run passes for host-a and host-b"
  ```

### NixOS Test Modules

- [ ] **Step 75** [B] ~60s: Run NixOS test modules (syntax + basic eval)
  ```bash
  cd ~/tmp/unheaded/nixos
  for f in tests/*.nix; do
    echo "=== $f ===" && nix-instantiate --parse "$f" > /dev/null 2>&1 && echo "PARSE OK" || echo "PARSE FAIL"
  done
  ```

- [ ] **Step 76** [B] ~5m: Fix any test module failures

- [ ] **Step 77** [V] ~30s: All 5 test modules parse
  - firewall-bridge.nix
  - wireguard.nix
  - frr.nix
  - observability.nix
  - (5th if exists)

- [ ] **Step 78** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "fix(nix): all NixOS test modules parse clean"
  ```

### Service Module Audit

- [ ] **Step 79** [B] ~2m: Verify all 25 service modules exist
  ```bash
  cd ~/tmp/unheaded/nixos/modules/services
  ls -la *.nix | wc -l
  ```

- [ ] **Step 80** [B] ~5m: Spot-check 5 critical service modules for correctness
  ```bash
  for svc in wotan dashboard trace-collector protocol-api sophia-gateway; do
    echo "=== $svc ===" && head -20 ~/tmp/unheaded/nixos/modules/services/${svc}.nix 2>/dev/null || echo "MISSING: $svc"
  done
  ```

- [ ] **Step 81** [V] ~30s: All critical services have NixOS modules
  - If any MISSING → create from template

- [ ] **Step 82** [W] ~5m: Update docs/bare-metal/TRIAGE-REPORT.md — NixOS section
  ```
  - Flake lock: REAL (generated from nix flake update)
  - Module parse: X/Y pass
  - Rebuild dry-run host-a: PASS/FAIL
  - Rebuild dry-run host-b: PASS/FAIL
  - Test modules: X/5 pass
  - Service modules: X/25 present
  ```

- [ ] **Step 83** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 84** [V]: **PHASE 2 EXIT GATE** — NixOS flake builds, all modules parse, dry-run succeeds
  ```bash
  cd ~/tmp/unheaded/nixos && nix-instantiate --parse flake.nix > /dev/null 2>&1 && echo "FLAKE OK" || echo "FLAKE FAIL"
  ```
  - If pass → Phase 3
  - If fail → DO NOT PROCEED

- [ ] **Step 85** [C]: Phase gate commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "[PLAN CATCH-UP] Phase 2 EXIT GATE PASSED — NixOS foundation verified"
  ```

---

## PHASE 3: OBSERVABILITY STACK DEPLOYMENT (Steps 86-115)

**Goal**: Prometheus + Grafana + Loki + VictoriaMetrics running. Dashboards loaded. Alerts configured.
**Prerequisite**: Phase 2 (NixOS modules parse)
**Time**: 2-3 hours
**Agent**: Coordinator [S]

### Docker-Based Stack (fastest path to running)

- [ ] **Step 86** [B] ~30s: Check Docker availability
  ```bash
  docker --version && docker compose version
  ```

- [ ] **Step 87** [V] ~5s: Docker available
  - If yes → Step 88
  - If no → Step 87a: Install Docker or skip to NixOS-native deployment

- [ ] **Step 88** [B] ~60s: Lint monitoring docker-compose
  ```bash
  cd ~/tmp/unheaded && docker compose -f monitoring/docker-compose.yml config --quiet 2>&1
  ```

- [ ] **Step 89** [D] ~5m: Fix any compose errors
  - Common: wrong image tags, missing volumes, port conflicts

- [ ] **Step 90** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 91** [B] ~120s: Start monitoring stack
  ```bash
  cd ~/tmp/unheaded && docker compose -f monitoring/docker-compose.yml up -d 2>&1 | tail -20
  ```

- [ ] **Step 92** [B] ~15s: Wait for services
  ```bash
  sleep 15 && docker compose -f monitoring/docker-compose.yml ps
  ```

- [ ] **Step 93** [V] ~30s: Health check all endpoints
  ```bash
  curl -sf http://localhost:9090/-/healthy && echo "Prometheus: OK" || echo "Prometheus: FAIL"
  curl -sf http://localhost:3000/api/health && echo "Grafana: OK" || echo "Grafana: FAIL"
  curl -sf http://localhost:3100/ready && echo "Loki: OK" || echo "Loki: FAIL"
  curl -sf http://localhost:8428/health && echo "VictoriaMetrics: OK" || echo "VictoriaMetrics: FAIL"
  ```

- [ ] **Step 94** [D] ~5m: Debug failing services
  ```bash
  docker compose -f monitoring/docker-compose.yml logs --tail 30 <service-name>
  ```

- [ ] **Step 95** [C]: **COMMIT CHECKPOINT**

### Grafana Dashboard Provisioning

- [ ] **Step 96** [B] ~30s: Check dashboard JSON files exist
  ```bash
  find ~/tmp/unheaded/monitoring -name "*.json" -path "*dashboard*" | head -10
  ```

- [ ] **Step 97** [B] ~2m: Import dashboards to Grafana
  ```bash
  for dash in ~/tmp/unheaded/monitoring/grafana/dashboards/*.json; do
    [ -f "$dash" ] || continue
    curl -sf -X POST http://admin:admin@localhost:3000/api/dashboards/db \
      -H "Content-Type: application/json" \
      -d "{\"dashboard\": $(cat "$dash"), \"overwrite\": true}" 2>&1 | head -1
    echo " → $(basename "$dash")"
  done
  ```

- [ ] **Step 98** [V] ~30s: At least 3 dashboards loaded
  ```bash
  curl -sf http://admin:admin@localhost:3000/api/search?type=dash-db | python3 -c "import json,sys; d=json.load(sys.stdin); print(f'{len(d)} dashboards loaded')"
  ```

- [ ] **Step 99** [C]: **COMMIT CHECKPOINT**

### Alertmanager + Rules

- [ ] **Step 100** [B] ~60s: Verify alert rules load
  ```bash
  curl -sf http://localhost:9090/api/v1/rules | python3 -c "import json,sys; d=json.load(sys.stdin); groups=d['data']['groups']; print(f'{len(groups)} rule groups, {sum(len(g[\"rules\"]) for g in groups)} total rules')"
  ```

- [ ] **Step 101** [V] ~5s: MonadHbHDropRateHigh rule exists
  ```bash
  curl -sf http://localhost:9090/api/v1/rules | python3 -c "import json,sys; d=json.load(sys.stdin); names=[r['name'] for g in d['data']['groups'] for r in g['rules']]; print('MonadHbH rule PRESENT' if 'MonadHbHDropRateHigh' in names else 'MonadHbH rule MISSING')"
  ```

- [ ] **Step 102** [B] ~30s: Verify Alertmanager
  ```bash
  curl -sf http://localhost:9093/-/healthy && echo "Alertmanager: OK" || echo "Alertmanager: FAIL (may not be in compose)"
  ```

### Prometheus Target Verification

- [ ] **Step 103** [B] ~30s: Check configured targets
  ```bash
  curl -sf http://localhost:9090/api/v1/targets | python3 -c "
  import json,sys
  d=json.load(sys.stdin)
  targets=d['data']['activeTargets']
  up = sum(1 for t in targets if t['health']=='up')
  down = sum(1 for t in targets if t['health']=='down')
  print(f'Targets: {up} up, {down} down, {len(targets)} total')
  for t in targets:
    print(f'  {t[\"labels\"].get(\"job\",\"?\"): <30} {t[\"health\"]}')
  "
  ```

- [ ] **Step 104** [V] ~5s: Prometheus healthy, scraping configured targets
  - Expected: most targets DOWN (services not running yet) — that's OK at this stage
  - Critical: Prometheus itself must be UP

- [ ] **Step 105** [C]: **COMMIT CHECKPOINT**

### VictoriaMetrics Long-Term Storage

- [ ] **Step 106** [B] ~30s: Verify VM receives Prometheus remote_write
  ```bash
  curl -sf http://localhost:8428/api/v1/query?query=up | python3 -c "import json,sys; d=json.load(sys.stdin); print(f'{len(d[\"data\"][\"result\"])} series in VictoriaMetrics')" 2>/dev/null || echo "VM not receiving data yet"
  ```

- [ ] **Step 107** [V] ~5s: VM has data (even if 0 series — healthy endpoint is enough)

### Loki Log Ingestion

- [ ] **Step 108** [B] ~30s: Verify Loki ready
  ```bash
  curl -sf http://localhost:3100/loki/api/v1/labels && echo "Loki labels endpoint: OK" || echo "Loki: NOT READY"
  ```

- [ ] **Step 109** [B] ~30s: Push test log entry
  ```bash
  curl -sf -X POST http://localhost:3100/loki/api/v1/push \
    -H "Content-Type: application/json" \
    -d '{"streams":[{"stream":{"app":"unheaded-test","level":"info"},"values":[["'$(date +%s)000000000'","catch-up battle plan test log entry"]]}]}' && echo "Loki push: OK" || echo "Loki push: FAIL"
  ```

- [ ] **Step 110** [V] ~5s: Test entry visible
  ```bash
  curl -sf "http://localhost:3100/loki/api/v1/query?query={app=\"unheaded-test\"}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(f'{len(d[\"data\"][\"result\"])} streams found')"
  ```

- [ ] **Step 111** [W] ~2m: Update TRIAGE-REPORT.md — Observability section
  ```
  - Prometheus: UP/DOWN, X targets configured
  - Grafana: UP/DOWN, X dashboards loaded
  - Loki: UP/DOWN, log push verified
  - VictoriaMetrics: UP/DOWN
  - Alertmanager: UP/DOWN/NOT_DEPLOYED
  - MonadHbH alert rule: PRESENT/MISSING
  ```

- [ ] **Step 112** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 113** [V]: **PHASE 3 EXIT GATE** — 4 core services running (Prometheus, Grafana, Loki, VM)
  ```bash
  healthy=0
  curl -sf http://localhost:9090/-/healthy > /dev/null && healthy=$((healthy+1))
  curl -sf http://localhost:3000/api/health > /dev/null && healthy=$((healthy+1))
  curl -sf http://localhost:3100/ready > /dev/null && healthy=$((healthy+1))
  curl -sf http://localhost:8428/health > /dev/null && healthy=$((healthy+1))
  echo "Observability: $healthy/4 healthy"
  [ "$healthy" -ge 3 ] && echo "EXIT GATE PASS" || echo "EXIT GATE FAIL"
  ```
  - 3/4 minimum required (VM is nice-to-have)
  - If pass → Phase 4
  - If fail → DO NOT PROCEED

- [ ] **Step 114** [C]: Phase gate commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "[PLAN CATCH-UP] Phase 3 EXIT GATE PASSED — Observability stack live"
  ```

- [ ] **Step 115**: Log stack teardown instructions for later
  ```bash
  echo "To stop: cd ~/tmp/unheaded && docker compose -f monitoring/docker-compose.yml down"
  ```

---

## PHASE 4: eBPF KERNEL VERIFICATION (Steps 116-145) [P]

**Goal**: Verify eBPF programs compile. Load at least one XDP program. Read a BPF map.
**Prerequisite**: Phase 1 (Go builds). Linux kernel with BPF support. sudo access.
**Time**: 2-3 hours
**Agent**: Coordinator [S] — PARALLELIZABLE with Phase 5

### Kernel BPF Capabilities

- [ ] **Step 116** [S][B] ~30s: Verify BPF JIT
  ```bash
  sudo sysctl net.core.bpf_jit_enable
  ```
  - If 0 → `sudo sysctl -w net.core.bpf_jit_enable=1`

- [ ] **Step 117** [B] ~30s: Verify BTF support
  ```bash
  ls /sys/kernel/btf/vmlinux 2>/dev/null && echo "BTF: OK" || echo "BTF: MISSING — eBPF CO-RE won't work"
  ```

- [ ] **Step 118** [S][B] ~30s: Verify bpftool works
  ```bash
  sudo bpftool prog list 2>&1 | head -5
  sudo bpftool map list 2>&1 | head -5
  ```

- [ ] **Step 119** [V] ~5s: BPF subsystem accessible
  - If all OK → Step 121
  - If fail → Step 120

- [ ] **Step 120** [D] ~5m: Debug BPF access
  ```bash
  ls -la /sys/fs/bpf/ 2>/dev/null || echo "/sys/fs/bpf not mounted"
  sudo mount -t bpf bpf /sys/fs/bpf 2>/dev/null || true
  sudo ls /sys/fs/bpf/
  ```

- [ ] **Step 121** [C]: **COMMIT CHECKPOINT**

### eBPF Program Compilation

- [ ] **Step 122** [B] ~60s: Find all BPF C source files
  ```bash
  find ~/tmp/unheaded -name "*.bpf.c" -o -name "*_kern.c" -o -name "*.bpf.h" 2>/dev/null | head -20
  ```

- [ ] **Step 123** [B] ~30s: Find Rust Aya programs
  ```bash
  find ~/tmp/unheaded/crates -name "*.rs" -path "*ebpf*" -o -name "*.rs" -path "*xdp*" 2>/dev/null | head -20
  ```

- [ ] **Step 124** [B] ~120s: Attempt to compile eBPF programs (Rust/Aya)
  ```bash
  cd ~/tmp/unheaded && cargo build --manifest-path crates/monad-ebpf/Cargo.toml 2>&1 | tail -20 || echo "Rust eBPF build failed — check Aya deps"
  ```

- [ ] **Step 125** [V] ~5s: At least one eBPF program compiles
  - If pass → Step 127
  - If fail → Step 126

- [ ] **Step 126** [D] ~10m: Debug eBPF compilation
  ```bash
  # Check Aya/libbpf dependencies
  cargo tree --manifest-path crates/monad-ebpf/Cargo.toml 2>&1 | grep -i "aya\|libbpf\|bpf" | head -10
  # Check if we need specific Rust target
  rustup target list --installed | grep bpf
  ```

- [ ] **Step 127** [C]: **COMMIT CHECKPOINT**

### XDP Program Loading

- [ ] **Step 128** [S][B] ~60s: Load XDP program on loopback (safe test)
  ```bash
  # Use a minimal XDP pass program for testing
  sudo ip link set lo xdpgeneric off 2>/dev/null || true
  # Try loading our program
  cd ~/tmp/unheaded && sudo go run ./cmd/trace-collector/ --xdp-iface lo --mode test 2>&1 | head -20 || echo "XDP load failed"
  ```

- [ ] **Step 129** [V] ~30s: XDP program attached
  ```bash
  sudo ip link show lo | grep xdp && echo "XDP attached to lo" || echo "XDP not attached"
  ```

- [ ] **Step 130** [D] ~5m: If XDP fails, try generic/SKB mode
  ```bash
  # The S40 handoff noted driver→generic fallback was implemented
  # Check if the fallback code exists
  grep -rn "xdpgeneric\|XDP_FLAGS_SKB_MODE\|GenericMode" --include="*.go" ~/tmp/unheaded/pkg/ebpf/ 2>/dev/null | head -5
  ```

- [ ] **Step 131** [S][B] ~10s: Detach test XDP
  ```bash
  sudo ip link set lo xdpgeneric off 2>/dev/null || true
  ```

- [ ] **Step 132** [C]: **COMMIT CHECKPOINT**

### BPF Map Operations

- [ ] **Step 133** [S][B] ~60s: Create and read a BPF map
  ```bash
  sudo bpftool map create /sys/fs/bpf/test_map type hash key 4 value 8 entries 16 name unheaded_test 2>&1
  ```

- [ ] **Step 134** [S][B] ~30s: Write and read map entry
  ```bash
  sudo bpftool map update pinned /sys/fs/bpf/test_map key 0x01 0x00 0x00 0x00 value 0xDE 0xAD 0xBE 0xEF 0x00 0x00 0x00 0x00
  sudo bpftool map lookup pinned /sys/fs/bpf/test_map key 0x01 0x00 0x00 0x00
  ```

- [ ] **Step 135** [V] ~5s: Map read returns DEADBEEF
  - If pass → Step 137
  - If fail → Step 136

- [ ] **Step 136** [D] ~3m: Debug map operations
  ```bash
  sudo bpftool map list | grep unheaded
  ```

- [ ] **Step 137** [S][B] ~10s: Cleanup test map
  ```bash
  sudo rm /sys/fs/bpf/test_map 2>/dev/null
  ```

- [ ] **Step 138** [C]: **COMMIT CHECKPOINT**

### Go cilium/ebpf Integration Test

- [ ] **Step 139** [S][B] ~60s: Run eBPF-specific tests (if any exist)
  ```bash
  cd ~/tmp/unheaded && sudo go test ./pkg/ebpf/... -v -count=1 -timeout 60s 2>&1 | tail -20
  ```

- [ ] **Step 140** [V] ~5s: eBPF tests pass (or gracefully skip without sudo)

- [ ] **Step 141** [W] ~5m: Update TRIAGE-REPORT.md — eBPF section
  ```
  - Kernel BPF JIT: ON/OFF
  - BTF: PRESENT/MISSING
  - bpftool: AVAILABLE/MISSING
  - eBPF compilation: PASS/FAIL (Rust) / PASS/FAIL (Go gen)
  - XDP load test: PASS/FAIL (mode: driver/generic/skb)
  - BPF map ops: PASS/FAIL
  - cilium/ebpf tests: PASS/FAIL/SKIP
  ```

- [ ] **Step 142** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 143** [V]: **PHASE 4 EXIT GATE** — At least ONE eBPF program loads. BPF maps read/write.
  ```bash
  echo "eBPF Phase 4 checklist:"
  echo "  BPF JIT: $(sudo sysctl -n net.core.bpf_jit_enable)"
  echo "  BTF: $(ls /sys/kernel/btf/vmlinux 2>/dev/null && echo YES || echo NO)"
  echo "  bpftool: $(which bpftool 2>/dev/null && echo YES || echo NO)"
  ```
  - Minimum: BPF JIT ON + bpftool available
  - If pass → Phase 6 (or continue to Phase 5 in parallel)
  - If fail → STUCK. Log. Move to non-eBPF phases.

- [ ] **Step 144** [C]: Phase gate commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "[PLAN CATCH-UP] Phase 4 EXIT GATE — eBPF kernel verification"
  ```

- [ ] **Step 145**: Record eBPF capability level for downstream phases
  ```
  LEVEL 0: No BPF support → All eBPF phases BLOCKED
  LEVEL 1: BPF maps only → Map tests pass, XDP deferred
  LEVEL 2: XDP generic → Full pipeline testable on loopback
  LEVEL 3: XDP driver → Production-ready on physical NIC
  ```

---

## PHASE 5: WIREGUARD + NETWORK FOUNDATION (Steps 146-170) [P]

**Goal**: WireGuard tunnel established between hosts. IPv6 fd00:dead:beef::/48 routable.
**Prerequisite**: Phase 1 (Go builds). Two hosts available (or single-host loopback test).
**Time**: 1.5-2 hours
**Agent**: Coordinator [S] — PARALLELIZABLE with Phase 4

### WireGuard Key Generation

- [ ] **Step 146** [B] ~10s: Check WireGuard tools
  ```bash
  wg --version 2>/dev/null || echo "WireGuard tools MISSING"
  ```

- [ ] **Step 147** [D] ~2m: Install if missing
  ```bash
  # NixOS: nix-env -iA nixpkgs.wireguard-tools
  # Or: sudo apt install wireguard-tools
  ```

- [ ] **Step 148** [B] ~10s: Generate host-a keys (if not already done)
  ```bash
  mkdir -p ~/tmp/unheaded/.secrets/wireguard
  if [ ! -f ~/tmp/unheaded/.secrets/wireguard/host-a.key ]; then
    wg genkey > ~/tmp/unheaded/.secrets/wireguard/host-a.key
    cat ~/tmp/unheaded/.secrets/wireguard/host-a.key | wg pubkey > ~/tmp/unheaded/.secrets/wireguard/host-a.pub
    echo "host-a keys generated"
  else
    echo "host-a keys already exist"
  fi
  ```

- [ ] **Step 149** [B] ~10s: Generate host-b keys
  ```bash
  if [ ! -f ~/tmp/unheaded/.secrets/wireguard/host-b.key ]; then
    wg genkey > ~/tmp/unheaded/.secrets/wireguard/host-b.key
    cat ~/tmp/unheaded/.secrets/wireguard/host-b.key | wg pubkey > ~/tmp/unheaded/.secrets/wireguard/host-b.pub
    echo "host-b keys generated"
  else
    echo "host-b keys already exist"
  fi
  ```

- [ ] **Step 150** [V] ~5s: Both keypairs exist
  ```bash
  ls -la ~/tmp/unheaded/.secrets/wireguard/*.{key,pub} 2>/dev/null | wc -l
  ```
  - Must be 4 files

- [ ] **Step 151** [C]: **COMMIT CHECKPOINT** (DO NOT commit .secrets/ — .gitignore check)
  ```bash
  grep -q ".secrets" ~/tmp/unheaded/.gitignore 2>/dev/null && echo ".secrets in .gitignore: OK" || echo "WARNING: .secrets NOT in .gitignore — add it NOW"
  ```

### WireGuard Interface Setup

- [ ] **Step 152** [S][B] ~30s: Create wg0 interface (host-a)
  ```bash
  sudo ip link add wg0 type wireguard 2>/dev/null || echo "wg0 may already exist"
  sudo ip addr add fd00:dead:beef::1/48 dev wg0 2>/dev/null || echo "addr may already be set"
  sudo wg set wg0 listen-port 51820 private-key ~/tmp/unheaded/.secrets/wireguard/host-a.key
  sudo ip link set wg0 up
  sudo ip link set wg0 mtu 1380
  ```

- [ ] **Step 153** [V] ~10s: wg0 up with correct address
  ```bash
  ip addr show wg0 | grep "fd00:dead:beef::1" && echo "wg0 IPv6: OK" || echo "wg0 IPv6: MISSING"
  sudo wg show wg0 | head -5
  ```

- [ ] **Step 154** [B] ~30s: Add host-b peer (requires host-b pubkey + endpoint)
  ```bash
  HOST_B_PUB=$(cat ~/tmp/unheaded/.secrets/wireguard/host-b.pub)
  HOST_B_ENDPOINT="<host-b-ip>:51820"  # FILL IN REAL IP
  sudo wg set wg0 peer "$HOST_B_PUB" allowed-ips fd00:dead:beef::2/128 endpoint "$HOST_B_ENDPOINT" persistent-keepalive 25
  ```
  - NOTE: If single host, skip peer setup. Test with loopback.

- [ ] **Step 155** [V] ~10s: WireGuard handshake (if both hosts available)
  ```bash
  sudo wg show wg0 | grep "latest handshake" && echo "WG handshake: OK" || echo "WG handshake: PENDING (need both hosts)"
  ```

- [ ] **Step 156** [B] ~10s: Ping test (if tunnel established)
  ```bash
  ping6 -c 3 fd00:dead:beef::2 2>&1 | tail -3 || echo "Peer not reachable — expected if single host"
  ```

- [ ] **Step 157** [V] ~5s: **Tunnel status**
  - If both hosts: ping must succeed, RTT < 5ms (H4 verdict)
  - If single host: wg0 interface UP is sufficient

- [ ] **Step 158** [C]: **COMMIT CHECKPOINT**

### Bridge Network (br-unheaded)

- [ ] **Step 159** [S][B] ~30s: Create bridge
  ```bash
  sudo ip link add br-unheaded type bridge 2>/dev/null || echo "bridge may exist"
  sudo ip addr add 10.20.0.254/16 dev br-unheaded 2>/dev/null || true
  sudo ip link set br-unheaded up
  ```

- [ ] **Step 160** [V] ~10s: Bridge up
  ```bash
  ip addr show br-unheaded | head -5
  ```

### HbH Passthrough (CRITICAL)

- [ ] **Step 161** [S][B] ~30s: Verify nftables HbH passthrough
  ```bash
  sudo nft list ruleset 2>/dev/null | grep -i "hopopt\|hbh\|nexthdr 0" || echo "No HbH rules found — need to add"
  ```

- [ ] **Step 162** [S][B] ~60s: Add HbH passthrough if missing
  ```bash
  # The firewall-bridge.nix should handle this, but verify/add manually:
  sudo nft add table ip6 unheaded 2>/dev/null || true
  sudo nft add chain ip6 unheaded forward '{ type filter hook forward priority 0; policy accept; }' 2>/dev/null || true
  # Explicitly allow IPv6 HbH (nexthdr 0 = Hop-by-Hop Options)
  ```

- [ ] **Step 163** [V] ~10s: HbH not blocked
  ```bash
  sudo nft list ruleset 2>/dev/null | grep -c "drop.*nexthdr 0\|reject.*hopopt" || echo "0 blocking rules — OK"
  ```

- [ ] **Step 164** [C]: **COMMIT CHECKPOINT**

### Network Documentation

- [ ] **Step 165** [W] ~5m: Update TRIAGE-REPORT.md — Network section
  ```
  - WireGuard: INSTALLED/MISSING
  - wg0 interface: UP/DOWN
  - Tunnel status: ESTABLISHED/SINGLE_HOST/FAILED
  - IPv6 fd00:dead:beef::/48: ROUTABLE/LOCAL_ONLY
  - br-unheaded: UP/DOWN
  - HbH passthrough: VERIFIED/MISSING
  - Ping RTT (if tunnel): Xms
  ```

- [ ] **Step 166** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 167** [V]: **PHASE 5 EXIT GATE** — wg0 interface UP, br-unheaded UP
  ```bash
  wg0_up=$(ip link show wg0 2>/dev/null | grep -c "state UP\|state UNKNOWN")
  br_up=$(ip link show br-unheaded 2>/dev/null | grep -c "state UP\|state UNKNOWN")
  echo "wg0: $wg0_up, br-unheaded: $br_up"
  [ "$wg0_up" -ge 1 ] && [ "$br_up" -ge 1 ] && echo "EXIT GATE PASS" || echo "EXIT GATE FAIL"
  ```
  - wg0 state UNKNOWN is valid (WireGuard interfaces show UNKNOWN when up)
  - If pass → Phase 6
  - If fail → DO NOT PROCEED to routing phases

- [ ] **Step 168** [C]: Phase gate commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "[PLAN CATCH-UP] Phase 5 EXIT GATE — WireGuard + bridge network live"
  ```

- [ ] **Step 169**: Record network capability level
  ```
  LEVEL 0: No WireGuard → Cross-host phases BLOCKED
  LEVEL 1: wg0 local only → Single-host testing possible
  LEVEL 2: Tunnel established → Full cross-host testing
  LEVEL 3: br-unheaded + routing → Production topology
  ```

- [ ] **Step 170**: Reserved for network debug notes

---

## PHASE 6: ROUTING DAEMON DEPLOYMENT (Steps 171-200)

**Goal**: FRR or BIRD running. BGP/IS-IS session up. VXLAN VTEPs configured.
**Prerequisite**: Phase 5 (network foundation)
**Time**: 2-3 hours
**Agent**: Coordinator [S]

### FRR Installation

- [ ] **Step 171** [B] ~30s: Check FRR availability
  ```bash
  vtysh --version 2>/dev/null || frr --version 2>/dev/null || echo "FRR not installed"
  ```

- [ ] **Step 172** [D] ~10m: Install FRR if missing
  ```bash
  # NixOS: nix-env -iA nixpkgs.frr OR add to NixOS config
  # Ubuntu: sudo apt install frr frr-pythontools
  # From source: ~/tmp/frr-master (if present)
  ls ~/tmp/frr-master 2>/dev/null && echo "FRR source tree found" || echo "No FRR source"
  ```

- [ ] **Step 173** [S][B] ~60s: Enable FRR daemons
  ```bash
  # Enable bgpd, isisd, bfdd in /etc/frr/daemons
  sudo grep -q "bgpd=yes" /etc/frr/daemons 2>/dev/null || echo "Need to enable bgpd"
  sudo grep -q "isisd=yes" /etc/frr/daemons 2>/dev/null || echo "Need to enable isisd"
  ```

- [ ] **Step 174** [S][B] ~30s: Copy Unheaded FRR config
  ```bash
  sudo cp ~/tmp/unheaded/routing/bgp-evpn/frr-bgp-evpn.conf /etc/frr/frr.conf 2>/dev/null || echo "Config path may differ"
  ```

- [ ] **Step 175** [S][B] ~30s: Start FRR
  ```bash
  sudo systemctl restart frr 2>/dev/null || sudo /etc/init.d/frr restart 2>/dev/null || echo "Cannot start FRR"
  ```

- [ ] **Step 176** [V] ~10s: FRR running
  ```bash
  sudo systemctl is-active frr 2>/dev/null || pgrep -a bgpd || echo "FRR not running"
  ```

- [ ] **Step 177** [C]: **COMMIT CHECKPOINT**

### BGP Session

- [ ] **Step 178** [S][B] ~30s: Check BGP status
  ```bash
  sudo vtysh -c "show bgp summary" 2>&1 | head -15
  ```

- [ ] **Step 179** [V] ~5s: BGP session state
  - If Established → Excellent
  - If Active/Connect → Peer unreachable (expected on single host)
  - If error → Step 180

- [ ] **Step 180** [D] ~5m: Debug BGP
  ```bash
  sudo vtysh -c "show bgp neighbors" 2>&1 | head -30
  sudo vtysh -c "show running-config" 2>&1 | head -40
  ```

### IS-IS Underlay (if using IS-IS option)

- [ ] **Step 181** [S][B] ~30s: Check IS-IS status
  ```bash
  sudo vtysh -c "show isis summary" 2>&1 | head -15 || echo "IS-IS not configured"
  ```

- [ ] **Step 182** [V] ~5s: IS-IS adjacency state

### VXLAN VTEPs

- [ ] **Step 183** [S][B] ~30s: Create VXLAN interfaces
  ```bash
  sudo ip link add vxlan10001 type vxlan id 10001 local fd00:dead:beef::1 dstport 4789 2>/dev/null || echo "vxlan10001 may exist"
  sudo ip link set vxlan10001 up 2>/dev/null
  ```

- [ ] **Step 184** [V] ~10s: VXLAN interface exists
  ```bash
  ip link show vxlan10001 2>/dev/null | head -3 || echo "VXLAN not created"
  ```

- [ ] **Step 185** [C]: **COMMIT CHECKPOINT**

### BIRD (host-b)

- [ ] **Step 186** [B] ~30s: Check BIRD availability
  ```bash
  birdc show status 2>/dev/null | head -5 || echo "BIRD not installed or not on this host"
  ```

- [ ] **Step 187** [V] ~5s: If this is host-b, BIRD should be available
  - If host-a only → Skip BIRD steps, mark as DEFERRED

### Routing Selector Script

- [ ] **Step 188** [B] ~30s: Syntax check routing selector
  ```bash
  bash -n ~/tmp/unheaded/scripts/routing/select-routing.sh && echo "Script syntax: OK" || echo "Script syntax: FAIL"
  ```

- [ ] **Step 189** [B] ~30s: Verify all routing configs exist
  ```bash
  for opt in bgp-evpn ospf isis mpls; do
    ls ~/tmp/unheaded/routing/${opt}/ 2>/dev/null | head -3 && echo "=== $opt: EXISTS ===" || echo "=== $opt: MISSING ==="
  done
  ```

- [ ] **Step 190** [C]: **COMMIT CHECKPOINT**

### Routing Health Binary

- [ ] **Step 191** [B] ~30s: Build routing-health
  ```bash
  cd ~/tmp/unheaded && go build -o /tmp/routing-health ./cmd/routing-health/
  ```

- [ ] **Step 192** [B] ~10s: Start routing-health
  ```bash
  /tmp/routing-health &
  sleep 2
  ```

- [ ] **Step 193** [V] ~10s: Health endpoint responds
  ```bash
  curl -sf http://localhost:8080/health | python3 -m json.tool
  curl -sf http://localhost:8080/ready && echo "READY" || echo "NOT READY"
  curl -sf http://localhost:8080/metrics | grep routing | head -5
  ```

- [ ] **Step 194** [B] ~5s: Stop routing-health
  ```bash
  kill %1 2>/dev/null || true
  ```

- [ ] **Step 195** [C]: **COMMIT CHECKPOINT**

### Routing Documentation

- [ ] **Step 196** [W] ~5m: Update TRIAGE-REPORT.md — Routing section
  ```
  - FRR: INSTALLED/MISSING, RUNNING/STOPPED
  - BGP session: ESTABLISHED/ACTIVE/NOT_CONFIGURED
  - IS-IS: ADJACENCY_UP/DOWN/NOT_CONFIGURED
  - VXLAN VTEPs: CREATED/MISSING
  - BIRD: INSTALLED/MISSING (host-b only)
  - Routing selector script: SYNTAX_OK/FAIL
  - Routing configs present: bgp-evpn/ospf/isis/mpls
  - routing-health binary: BUILDS/FAILS, /health: OK/FAIL
  ```

- [ ] **Step 197** [V]: **PHASE 6 EXIT GATE** — At least one routing daemon running OR configs validated
  ```bash
  frr_ok=0; bird_ok=0
  pgrep -x bgpd > /dev/null && frr_ok=1
  pgrep -x bird > /dev/null && bird_ok=1
  configs=$(ls ~/tmp/unheaded/routing/bgp-evpn/*.conf 2>/dev/null | wc -l)
  echo "FRR: $frr_ok, BIRD: $bird_ok, Configs: $configs"
  [ "$frr_ok" -ge 1 ] || [ "$bird_ok" -ge 1 ] || [ "$configs" -ge 1 ] && echo "EXIT GATE PASS" || echo "EXIT GATE FAIL"
  ```
  - Minimum: configs exist and parse (daemon running is bonus)
  - If pass → Phase 7
  - If fail → STUCK. Skip to Phase 8.

- [ ] **Step 198** [C]: Phase gate commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "[PLAN CATCH-UP] Phase 6 EXIT GATE — Routing daemons assessed"
  ```

- [ ] **Step 199**: Reserved
- [ ] **Step 200**: Reserved

---

## PHASE 7: FIREWALL + IDS DEPLOYMENT (Steps 201-225)

**Goal**: Firewall rules active. Suricata IDS running (or validated ready-to-deploy).
**Prerequisite**: Phase 5 (network), Phase 6 (routing — at least configs validated)
**Time**: 2-3 hours
**Agent**: Coordinator [S]

### nftables Baseline

- [ ] **Step 201** [S][B] ~30s: Current ruleset
  ```bash
  sudo nft list ruleset 2>&1 | wc -l
  ```

- [ ] **Step 202** [S][B] ~60s: Apply Unheaded nftables rules
  ```bash
  # Check if NixOS firewall module exists
  cat ~/tmp/unheaded/nixos/modules/firewall-bridge.nix | head -30
  # Or apply directly from nft config if exists
  ls ~/tmp/unheaded/nixos/modules/firewall-bridge.nix 2>/dev/null && echo "Firewall module exists"
  ```

- [ ] **Step 203** [V] ~10s: HbH passthrough rule in place
  ```bash
  sudo nft list ruleset | grep -i "hopopt\|nexthdr 0\|hbh" || echo "HbH rules MISSING — CRITICAL"
  ```

- [ ] **Step 204** [C]: **COMMIT CHECKPOINT**

### OPNsense/IPFire (VM-based — needs /dev/kvm)

- [ ] **Step 205** [B] ~10s: Check KVM availability
  ```bash
  ls /dev/kvm 2>/dev/null && echo "KVM: OK" || echo "KVM: NOT AVAILABLE — skip VM-based firewalls"
  ```

- [ ] **Step 206** [V] ~5s: If no KVM → mark OPNsense/IPFire as DEFERRED
  - These are QEMU VMs — need /dev/kvm + libvirtd
  - If available → proceed to Step 207
  - If not → skip to Step 215

- [ ] **Step 207** [S][B] ~5m: Check libvirt
  ```bash
  virsh list --all 2>/dev/null || echo "libvirt not available"
  ```

- [ ] **Step 208-214**: OPNsense/IPFire VM boot sequence (CONDITIONAL on KVM)
  - Only execute if Step 205 passes
  - Otherwise mark as [BLOCKED]

- [ ] **Step 215** [C]: **COMMIT CHECKPOINT**

### Suricata IDS

- [ ] **Step 216** [B] ~30s: Check Suricata availability
  ```bash
  suricata --build-info 2>/dev/null | head -5 || echo "Suricata not installed"
  ```

- [ ] **Step 217** [D] ~5m: Install if missing
  ```bash
  # NixOS: nix-env -iA nixpkgs.suricata
  # Ubuntu: sudo add-apt-repository ppa:oisf/suricata-stable && sudo apt install suricata
  ```

- [ ] **Step 218** [B] ~30s: Validate Unheaded Monad rules
  ```bash
  cat ~/tmp/unheaded/routing/suricata/rules/unheaded-monad.rules | head -20
  ```

- [ ] **Step 219** [B] ~60s: Suricata config test (non-destructive)
  ```bash
  ls ~/tmp/unheaded/scripts/suricata/smoke-test.sh 2>/dev/null && bash -n ~/tmp/unheaded/scripts/suricata/smoke-test.sh && echo "Smoke test script: SYNTAX OK" || echo "No smoke test"
  ```

- [ ] **Step 220** [V] ~5s: Suricata validated (running or ready-to-deploy)

- [ ] **Step 221** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 222** [W] ~3m: Update TRIAGE-REPORT.md — Firewall/IDS section

- [ ] **Step 223** [V]: **PHASE 7 EXIT GATE** — nftables active with HbH passthrough
  ```bash
  hbh_ok=$(sudo nft list ruleset 2>/dev/null | grep -c "drop.*nexthdr 0")
  echo "HbH blocking rules: $hbh_ok (must be 0)"
  [ "$hbh_ok" -eq 0 ] && echo "EXIT GATE PASS" || echo "EXIT GATE FAIL — HbH IS BEING BLOCKED"
  ```
  - CRITICAL: If HbH is being dropped, Monad protocol is dead
  - If pass → Phase 8
  - If fail → FIX IMMEDIATELY. This is P0.

- [ ] **Step 224** [C]: Phase gate commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "[PLAN CATCH-UP] Phase 7 EXIT GATE — Firewall + IDS assessed"
  ```

- [ ] **Step 225**: Reserved

---

## PHASE 8: GPU + AI STACK (Steps 226-245)

**Goal**: vLLM running on RX 7700 XT. DeepSeek-R1 inference verified. OR explicitly deferred.
**Prerequisite**: Phase 1 (Go builds). Gaming desktop with ROCm.
**Time**: 2-3 hours (if GPU available) / 15 min (if not — just document)
**Agent**: Coordinator [S]

### GPU Detection

- [ ] **Step 226** [B] ~30s: Detect GPU
  ```bash
  lspci | grep -i "vga\|3d\|display"
  rocminfo 2>/dev/null | grep "Name:\|Marketing" | head -5 || echo "ROCm not available"
  ```

- [ ] **Step 227** [V] ~5s: GPU capability assessment
  - If RX 7700 XT + ROCm → Full AI stack deployment
  - If other AMD GPU + ROCm → Attempt with caveats
  - If no GPU / no ROCm → Mark Phase 8 as DEFERRED, skip to Phase 9

- [ ] **Step 228** [B] ~60s: Check Docker GPU passthrough
  ```bash
  docker run --rm --device=/dev/kfd --device=/dev/dri rocm/rocm-terminal rocminfo 2>&1 | head -10 || echo "Docker GPU passthrough failed"
  ```

- [ ] **Step 229** [C]: **COMMIT CHECKPOINT**

### vLLM Deployment (CONDITIONAL on GPU)

- [ ] **Step 230** [B] ~300s: Pull vLLM ROCm image
  ```bash
  docker pull vllm/vllm-openai:latest-rocm 2>&1 | tail -5 || echo "vLLM image pull failed"
  ```

- [ ] **Step 231** [B] ~600s: Download DeepSeek-R1 7B model (if not cached)
  ```bash
  # This is ~4.5GB — may take a while
  ls ~/models/deepseek-r1-7b-q4 2>/dev/null && echo "Model cached" || echo "Need to download model"
  ```

- [ ] **Step 232** [B] ~120s: Start vLLM
  ```bash
  docker run -d --name vllm-unheaded \
    --device=/dev/kfd --device=/dev/dri \
    -v ~/models:/models \
    -p 20100:8000 \
    vllm/vllm-openai:latest-rocm \
    --model /models/deepseek-r1-7b-q4 \
    --dtype half \
    --max-model-len 4096 2>&1 | tail -5
  ```

- [ ] **Step 233** [V] ~30s: vLLM health check
  ```bash
  sleep 30  # vLLM needs warmup
  curl -sf http://localhost:20100/health && echo "vLLM: OK" || echo "vLLM: STARTING..."
  ```

- [ ] **Step 234** [B] ~10s: Test inference
  ```bash
  curl -sf http://localhost:20100/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d '{"model":"deepseek-r1-7b-q4","messages":[{"role":"user","content":"What is 2+2?"}],"max_tokens":50}' | python3 -m json.tool | head -20
  ```

- [ ] **Step 235** [V] ~5s: Inference returns valid response
  - H5 verdict target: >50 tokens/sec at P95 <500ms

- [ ] **Step 236** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 237-243**: Open WebUI + Qdrant + BGE-M3 (CONDITIONAL — same GPU dependency)

- [ ] **Step 244** [V]: **PHASE 8 EXIT GATE** — vLLM responds OR explicitly deferred
  ```bash
  curl -sf http://localhost:20100/health > /dev/null 2>&1 && echo "EXIT GATE PASS — AI stack live" || echo "EXIT GATE PASS (DEFERRED) — No GPU available"
  ```
  - Both outcomes are valid — but must be DOCUMENTED

- [ ] **Step 245** [C]: Phase gate commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "[PLAN CATCH-UP] Phase 8 EXIT GATE — AI stack assessed"
  ```

---

## PHASE 9: UNHEADED SERVICE STACK (Steps 246-260)

**Goal**: Core Unheaded services start and respond to health checks.
**Prerequisite**: Phases 1-3 (Go builds, NixOS, observability)
**Time**: 1.5-2 hours
**Agent**: Coordinator

- [ ] **Step 246** [B] ~60s: Build all service binaries
  ```bash
  cd ~/tmp/unheaded
  for svc in wotan dashboard-backend trace-collector protocol-api sophia-gateway captain timeguru micromanager architect unheaded-daemon; do
    go build -o /tmp/unheaded-bins/$svc ./cmd/$svc/ 2>&1 && echo "BUILD OK: $svc" || echo "BUILD FAIL: $svc"
  done
  ```

- [ ] **Step 247** [V] ~5s: Count successful builds
  ```bash
  ls /tmp/unheaded-bins/ 2>/dev/null | wc -l
  ```

- [ ] **Step 248** [B] ~30s: Start Wotan (message bus — everything depends on this)
  ```bash
  /tmp/unheaded-bins/wotan 2>&1 &
  sleep 3
  curl -sf http://localhost:18000/health && echo "Wotan: OK" || echo "Wotan: FAIL"
  ```

- [ ] **Step 249** [B] ~30s: Start dashboard-backend
  ```bash
  /tmp/unheaded-bins/dashboard-backend 2>&1 &
  sleep 2
  curl -sf http://localhost:20000/health && echo "Dashboard: OK" || echo "Dashboard: FAIL"
  ```

- [ ] **Step 250** [B] ~60s: Start remaining services (best effort)
  ```bash
  for svc in protocol-api sophia-gateway trace-collector unheaded-daemon; do
    /tmp/unheaded-bins/$svc 2>&1 &
    sleep 1
  done
  ```

- [ ] **Step 251** [V] ~30s: Health check all running services
  ```bash
  for port in 17000 18000 19005 20000; do
    curl -sf http://localhost:$port/health 2>/dev/null && echo "Port $port: HEALTHY" || echo "Port $port: DOWN"
  done
  ```

- [ ] **Step 252** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 253** [B] ~10s: Stop all services
  ```bash
  pkill -f "/tmp/unheaded-bins/" 2>/dev/null || true
  ```

- [ ] **Step 254-258**: Reserved for service-specific debugging

- [ ] **Step 259** [V]: **PHASE 9 EXIT GATE** — At least 3 services respond to /health
  - If pass → Phase 10
  - If fewer than 3 → Log which ones fail and why. Continue.

- [ ] **Step 260** [C]: Phase gate commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "[PLAN CATCH-UP] Phase 9 EXIT GATE — Service stack assessed"
  ```

---

## PHASE 10: END-TO-END INTEGRATION TEST (Steps 261-270)

**Goal**: Packet enters XDP → flows through pipeline → appears in dashboard.
**Prerequisite**: Phases 4 (eBPF), 5 (network), 9 (services)
**Time**: 1-2 hours
**Agent**: Coordinator [S]

- [ ] **Step 261** [S][B] ~5m: Start full stack (Wotan + trace-collector + dashboard + XDP)
  ```bash
  # Start in order: Wotan → trace-collector → dashboard
  cd ~/tmp/unheaded
  /tmp/unheaded-bins/wotan &
  sleep 2
  sudo /tmp/unheaded-bins/trace-collector --xdp-iface lo &
  sleep 2
  /tmp/unheaded-bins/dashboard-backend &
  sleep 2
  ```

- [ ] **Step 262** [S][B] ~30s: Inject test packet with Monad HbH header
  ```bash
  # Use Scapy or raw socket to inject IPv6 + HbH packet
  python3 -c "
  from scapy.all import *
  from scapy.layers.inet6 import *
  p = IPv6(dst='::1')/IPv6ExtHdrHopByHop(options=[HBHOptUnknown(otype=0x3E, optdata=b'\x00'*18)])/UDP(dport=16666)/b'monad-e2e-test'
  send(p, iface='lo', verbose=0)
  print('Packet sent')
  " 2>&1 || echo "Scapy not available — install python3-scapy"
  ```

- [ ] **Step 263** [V] ~10s: Check dashboard for trace event
  ```bash
  curl -sf http://localhost:20000/api/v1/events?limit=5 2>/dev/null | python3 -m json.tool | head -20 || echo "No events endpoint or no data"
  ```

- [ ] **Step 264** [B] ~5s: Stop full stack
  ```bash
  pkill -f "/tmp/unheaded-bins/" 2>/dev/null || true
  ```

- [ ] **Step 265-268**: Reserved for E2E debugging

- [ ] **Step 269** [V]: **PHASE 10 EXIT GATE** — E2E pipeline demonstrated OR gap documented
  - Full success: Packet → XDP → trace-collector → Wotan → dashboard
  - Partial: Some stages work, gaps identified
  - Both are valid outcomes — DOCUMENT the result

- [ ] **Step 270** [C]: Phase gate commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "[PLAN CATCH-UP] Phase 10 EXIT GATE — E2E integration assessed"
  ```

---

## PHASE 11: DOOM VALIDATION (Steps 271-278)

**Goal**: Doom PoC runs on eBPF. Frame visible in browser. OR gap precisely documented.
**Prerequisite**: Phase 4 (eBPF verified), Phase 9 (services)
**Time**: 1-2 hours
**Agent**: Coordinator [S]

- [ ] **Step 271** [B] ~30s: Check Doom assets exist
  ```bash
  ls ~/tmp/unheaded/demos/doom/ 2>/dev/null | head -10
  ls ~/tmp/unheaded/cmd/doom-bridge/ 2>/dev/null | head -5
  ls ~/tmp/unheaded/cmd/doom-go-injector/ 2>/dev/null | head -5
  ```

- [ ] **Step 272** [B] ~60s: Build Doom binaries
  ```bash
  cd ~/tmp/unheaded
  go build -o /tmp/doom-bridge ./cmd/doom-bridge/ 2>&1 | tail -5
  go build -o /tmp/doom-injector ./cmd/doom-go-injector/ 2>&1 | tail -5
  ```

- [ ] **Step 273** [V] ~5s: Doom binaries build
  - If fail → STUCK. Doom requires eBPF ring buffer.

- [ ] **Step 274** [S][B] ~5m: Attempt Doom startup sequence
  ```bash
  # This requires: BPF maps loaded, ring buffer, XDP
  # Attempt and capture what happens
  cd ~/tmp/unheaded && sudo /tmp/doom-bridge --config demos/doom/doom.toml 2>&1 | head -20 &
  sleep 5
  curl -sf http://localhost:16666/health 2>/dev/null || echo "doom-bridge not responding"
  ```

- [ ] **Step 275** [V] ~5s: Doom status
  - If running → Check browser viewport
  - If not → Document exactly where it fails

- [ ] **Step 276** [B] ~5s: Stop Doom
  ```bash
  pkill -f "doom-bridge\|doom-injector" 2>/dev/null || true
  ```

- [ ] **Step 277** [V]: **PHASE 11 EXIT GATE** — Doom builds OR gap documented
  - D-020 (DOOM RUNS) is the ultimate goal but may not be achievable yet
  - Minimum: binaries compile, gap between current state and D-020 documented

- [ ] **Step 278** [C]: Phase gate commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "[PLAN CATCH-UP] Phase 11 EXIT GATE — Doom PoC assessed"
  ```

---

## PHASE 12: FINAL REPORT + VERDICT COLLECTION (Steps 279-290)

**Goal**: Comprehensive status report. H-verdicts collected (or NA'd). Next battle plan scoped.
**Prerequisite**: All prior phases attempted
**Time**: 1 hour
**Agent**: Coordinator

- [ ] **Step 279** [W] ~15m: Write comprehensive BARE-METAL-VERDICT.md
  ```markdown
  # Bare Metal Verdict Report — The Reckoning

  Date: YYYY-MM-DD
  Hosts tested: [list]
  Sessions covered: S40-S70

  ## H-Verdicts
  | ID | Metric | Target | Actual | Status |
  |----|--------|--------|--------|--------|
  | H1 | RAM utilization | <48GB | ? | MEASURED/DEFERRED |
  | H2 | eBPF CPU overhead | <5% | ? | MEASURED/DEFERRED |
  | H4 | WireGuard RTT | <5ms | ? | MEASURED/DEFERRED |
  | H5 | vLLM throughput | >50 tok/s | ? | MEASURED/DEFERRED |
  | H6 | Disk latency p95 | <50ms | ? | MEASURED/DEFERRED |
  | H7 | WireGuard CPU | <2% | ? | MEASURED/DEFERRED |
  | H8 | Process count | <500 | ? | MEASURED/DEFERRED |
  | H9 | 24h availability | 99.9% | ? | NOT_TESTED |

  ## Phase Results Summary
  [One line per phase: PASS / PARTIAL / FAIL / DEFERRED]

  ## Remaining Gaps → Next Battle Plan
  [Ordered by priority]
  ```

- [ ] **Step 280** [W] ~5m: Update timeline.md with catch-up results

- [ ] **Step 281** [W] ~5m: Update CLAUDE.md if any architecture facts changed

- [ ] **Step 282** [C]: **COMMIT CHECKPOINT**

- [ ] **Step 283** [B] ~30s: Final Go build + test verification
  ```bash
  cd ~/tmp/unheaded && go build ./... && echo "FINAL BUILD: CLEAN" || echo "FINAL BUILD: BROKEN"
  ```

- [ ] **Step 284** [B] ~120s: Final test suite
  ```bash
  cd ~/tmp/unheaded && go test ./... -race -count=1 -timeout 120s 2>&1 | grep -E "^(ok|FAIL)" | sort
  ```

- [ ] **Step 285** [V] ~5s: No regressions from catch-up work

- [ ] **Step 286** [C]: Final commit
  ```bash
  cd ~/tmp/unheaded && git add -A && git commit -m "docs(s71): bare metal catch-up complete — verdict report + triage"
  ```

- [ ] **Step 287** [V]: **PHASE 12 EXIT GATE** — Report written, no regressions, next steps clear
  ```bash
  test -f ~/tmp/unheaded/docs/bare-metal/BARE-METAL-VERDICT.md && echo "EXIT GATE PASS" || echo "EXIT GATE FAIL"
  ```

- [ ] **Step 288-290**: Reserved

---

## APPENDIX A: EMERGENCY PROCEDURES

### EP-1: Go Build Spiral (can't get `go build ./...` clean)
1. `go build ./cmd/wotan/` — build core service first
2. If import cycle → `go mod graph | head -20`
3. If undefined types → check if S67-S69 packages depend on each other incorrectly
4. Nuclear option: `go mod tidy && go build ./...`

### EP-2: NixOS Flake Won't Update
1. Check internet connectivity
2. `nix flake metadata` — verify input URLs
3. `nix flake lock --update-input nixpkgs` — update one input at a time
4. Check NixOS channel vs flake compatibility

### EP-3: eBPF Verifier Rejection
1. `sudo bpftool prog load <file> /sys/fs/bpf/test 2>&1` — get full verifier log
2. Common: loop bounds not proven, stack too deep, unreachable code
3. Check kernel version — some verifier features require 5.15+
4. Try with `bpf_loop()` helper instead of bounded loops (kernel 5.17+)

### EP-4: WireGuard No Handshake
1. Check firewall: `sudo nft list ruleset | grep 51820`
2. Check endpoint reachability: `nc -vzu <host-b-ip> 51820`
3. Check key mismatch: compare pubkeys on both sides
4. Check MTU: ensure 1380 on both ends

### EP-5: Docker Compose Stack Won't Start
1. `docker compose logs <service>` — check specific service
2. Common: port conflicts (`ss -tlnp | grep <port>`)
3. Volume permissions: `ls -la` on mounted paths
4. Image pull failures: check network / registry access

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel | Depends On | Est. Time |
|-------|-------|----------|------------|-----------|
| 0: Triage | Coordinator | No | — | 45m |
| 1: Go Build | Coordinator | No | Phase 0 | 2-3h |
| 2: NixOS | Coordinator [S] | No | Phase 1 | 2-3h |
| 3: Observability | Coordinator [S] | No | Phase 2 | 2-3h |
| 4: eBPF | Coordinator [S] | **Yes** (with 5) | Phase 1 | 2-3h |
| 5: WireGuard | Coordinator [S] | **Yes** (with 4) | Phase 1 | 1.5-2h |
| 6: Routing | Coordinator [S] | No | Phase 5 | 2-3h |
| 7: Firewall/IDS | Coordinator [S] | No | Phases 5,6 | 2-3h |
| 8: GPU/AI | Coordinator [S] | No | Phase 1 | 2-3h |
| 9: Services | Coordinator | No | Phases 1,3 | 1.5-2h |
| 10: E2E | Coordinator [S] | No | Phases 4,5,9 | 1-2h |
| 11: Doom | Coordinator [S] | No | Phase 4 | 1-2h |
| 12: Report | Coordinator | No | All prior | 1h |

### Critical Path
```
Phase 0 → Phase 1 → Phase 2 → Phase 3 → Phase 9 → Phase 10 → Phase 12
                  ↘ Phase 4 ↗                              ↗
                  ↘ Phase 5 → Phase 6 → Phase 7 ──────────↗
                                                            ↗
                  ↘ Phase 8 ──────────────────────────────↗
```

**Minimum sequential time**: ~15 hours (critical path)
**With parallelization**: ~12 hours (Phases 4+5 parallel)

---

## APPENDIX C: QUICK REFERENCE

### Architecture Constants
```
host-a  Forge    10.20.255.1  fd00:dead:beef::1  AS65001  FRR + OPNsense
host-b  Outpost  10.20.255.2  fd00:dead:beef::2  AS65002  BIRD + IPFire
WireGuard: fd00:dead:beef::/48  MTU=1380  Port 51820
Doom Range: 16666-26666
```

### Port Quick Reference
```
16666  doom-bridge          18000  wotan HTTP
16667  doom-injector        18001  wotan gRPC
16670  trace-collector      19000  timeguru
17000  unheaded-daemon      19005  sophia-gateway
20000  dashboard            20100  vLLM
20001  kanban               20102  qdrant
21443  gateway HTTPS        9090   prometheus
3000   grafana              3100   loki
8428   victoriametrics      9093   alertmanager
```

### Key File Locations
```
nixos/flake.nix              — NixOS entry point
nixos/modules/               — All NixOS modules
monitoring/                  — Prometheus/Grafana/Loki/VM configs
routing/bgp-evpn/            — Default routing config
routing/suricata/rules/      — Monad-specific IDS rules
pkg/metrics/                 — Bare metal/LXD/Docker/NixOS collectors
pkg/anamnesis/suricata.go    — EVE JSON → Wotan bridge
cmd/routing-health/          — Health check binary
scripts/routing/             — Routing selector
.secrets/wireguard/          — WG keys (NOT in git)
```

### Monad Register (20 bytes)
```
Offset  Size  Field
0       2     Version + Flags
2       4     FlowLabel
6       8     TimestampNs
14      2     TraceID
16      2     SpanID
18      2     CRC-16 (CCITT-FALSE)
```

---

*CATCH-UP Battle Plan — Forged 2026-02-26*
*13 Phases. 290 Steps. The Reckoning — validating everything Cowork wrote but never touched bare metal.*
*Three hundred files of infrastructure. Zero validated. Until now.*
