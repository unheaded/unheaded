# S27 EXECUTION RUNBOOK — Multi-Tied with Failure Cases

**Date**: 2026-02-21
**Session**: S27 (Dev Machine Sprint — Alpha Gate)
**Scope**: P0-P3 (Nomenclature, Lint, LICH Fuzz, Docker Compose)
**Environment**: Dev machine with Go 1.26.0, Rust nightly, golangci-lint
**Prepared by**: S26 Round Table + S27 Cowork sprint

---

## PRE-FLIGHT (Do This First, Every Time)

```bash
cd ~/tmp/unheaded

# Step 1: Verify clean state from S25
go build ./...
# EXPECTED: exit 0, no output
# FAILURE CASE:
#   If "go: module not found" → run: go mod tidy
#   If "package X imported but not used" → S25 left dirty state, check: git status
#   If build errors → DO NOT PROCEED. Run: git log --oneline -5 to verify we're on right branch
#   If "go: cannot find GOROOT" → export GOROOT=$(go env GOROOT)

# Step 2: Verify tests
go test -race -count=1 ./...
# EXPECTED: 134/134 PASS (or more if new tests added)
# FAILURE CASE:
#   If < 134 pass → regression detected. Run: go test -v -race ./... 2>&1 | grep FAIL
#   If race detector fires → STOP. This is a real bug. Fix before any other work.
#   If timeout → increase: go test -race -timeout 600s ./...
#   If "signal: killed" → OOM. Run tests per-package: go test -race ./internal/...

# Step 3: Set PATH
export PATH=$PATH:$(go env GOPATH)/bin
# VERIFY: which golangci-lint → should return a path
# FAILURE CASE:
#   If "not found" → go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
#   If still not found → echo $GOPATH and verify bin/ exists

# Step 4: Check git state
git status
git log --oneline -5
# EXPECTED: clean working tree, S25 commits at HEAD
# FAILURE CASE:
#   If dirty → git stash (save work) then proceed
#   If wrong branch → git checkout main (or whatever the primary branch is)
#   If "detached HEAD" → git checkout main && git pull
```

---

## P0: MAD-MARIA NOMENCLATURE FIX

**Effort**: XS (5 min) | **Risk**: LOW | **Blocks**: Nothing | **Owner**: Lore

### Happy Path

```bash
# Copy the script from Cowork outputs to repo
# OR run these commands directly:

# Fix moatghost skill file
sed -i 's/║    👑 THE MATRIARCH\/PATRIARCH 👑     ║/║         👑 MAD-MARIA 👑              ║/g' \
  .skills/skills/unheaded-moatghost/SKILL.md

# Fix kingdom skill file
sed -i 's/║    👑 THE MATRIARCH\/PATRIARCH 👑     ║/║         👑 MAD-MARIA 👑              ║/g' \
  .skills/skills/unheaded-kingdom/SKILL.md

# Verify
grep -rn "MATRIARCH\|PATRIARCH" .skills/skills/
# EXPECTED: No output (all fixed)

grep -rn "MAD-MARIA" .skills/skills/
# EXPECTED: 2 occurrences (one per file)

# Commit
git add .skills/skills/unheaded-moatghost/SKILL.md .skills/skills/unheaded-kingdom/SKILL.md
git commit -m "fix(lore): replace MATRIARCH/PATRIARCH with Mad-Maria in skill files

S26 Round Table Decision #2: Mad-Maria is the canonical name.
MATRIARCH/PATRIARCH was a placeholder. Cultural debt resolved.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

### Failure Cases

| Symptom | Cause | Recovery |
|---------|-------|----------|
| `sed: can't read file` | File moved or renamed | `find . -name "SKILL.md" -path "*moatghost*"` to locate |
| `Permission denied` | Git or NixOS protecting file | `chmod u+w <file>` then retry; if NixOS, edit nix config |
| Pattern not found (sed exits 0 but no change) | Pattern already fixed OR changed | `grep -n "MATRIARCH\|PATRIARCH\|MAD-MARIA" <file>` to inspect |
| Encoding issues (emoji garbled) | UTF-8 mismatch | `file <file>` to check encoding; `iconv -f utf-8 -t utf-8 <file> > /dev/null` to verify |
| Git won't commit (pre-commit hook) | Linter/formatter hook | Read hook output, fix issue, `git add` again, new commit |

### Verification Gate

```bash
# MUST ALL PASS before moving to P1:
[ $(grep -rc "MATRIARCH\|PATRIARCH" .skills/skills/ 2>/dev/null | awk -F: '{s+=$2}END{print s}') -eq 0 ] && echo "P0 PASS ✅" || echo "P0 FAIL ❌"
```

---

## P1: LINT CLEANUP (Critical-Only)

**Effort**: M (2-4 hours) | **Risk**: MEDIUM | **Blocks**: Alpha gate | **Owner**: Developer

### Happy Path

```bash
# Step 1: Deploy the .golangci.yml config
cp /path/to/cowork/outputs/.golangci.yml ~/tmp/unheaded/.golangci.yml
# OR copy the file content from S27 artifacts

# Step 2: Run critical-only lint (DRY RUN — see what we're dealing with)
golangci-lint run --config .golangci.yml ./... 2>&1 | tee /tmp/lint-output.txt
# EXPECTED: Errors from errcheck, govet, staticcheck, unused, gosec, bodyclose
# MEASURE: wc -l /tmp/lint-output.txt → count total issues

# Step 3: Triage by severity
# Count per linter:
grep -c "errcheck" /tmp/lint-output.txt || echo "0 errcheck"
grep -c "govet" /tmp/lint-output.txt || echo "0 govet"
grep -c "staticcheck" /tmp/lint-output.txt || echo "0 staticcheck"
grep -c "unused" /tmp/lint-output.txt || echo "0 unused"
grep -c "gosec" /tmp/lint-output.txt || echo "0 gosec"
grep -c "bodyclose" /tmp/lint-output.txt || echo "0 bodyclose"

# Step 4: Fix in priority order
# TIER 1 (govet + staticcheck) — These are compiler-level bugs
golangci-lint run --config .golangci.yml --enable-only govet,staticcheck ./... 2>&1 | head -50
# Fix each one. These are REAL bugs.

# TIER 2 (errcheck) — Unchecked errors
golangci-lint run --config .golangci.yml --enable-only errcheck ./... 2>&1 | head -50
# Add error checks. Pattern: if err != nil { return fmt.Errorf("context: %w", err) }

# TIER 3 (unused) — Dead code
golangci-lint run --config .golangci.yml --enable-only unused ./... 2>&1 | head -50
# Delete dead code. Less code = less attack surface.

# TIER 4 (gosec + bodyclose) — Security + resource leaks
golangci-lint run --config .golangci.yml --enable-only gosec,bodyclose ./... 2>&1 | head -50
# Fix security issues and unclosed response bodies.

# Step 5: Verify clean
golangci-lint run --config .golangci.yml ./...
# EXPECTED: exit 0, no output

# Step 6: Re-run tests (CRITICAL — lint fixes can break things)
go test -race -count=1 ./...
# EXPECTED: 134/134 PASS (or more)
# FAILURE CASE: If tests break, you introduced a regression. Fix it.

# Step 7: Commit
git add .golangci.yml
git add -p  # Interactive add — review each lint fix
git commit -m "chore(lint): add critical-only golangci-lint config and fix findings

S26 Decision #4: Critical-only enforcement for Age 1.
Enabled: errcheck, govet, staticcheck, unused, gosec, bodyclose.
Disabled: varnamelen, mnd, lll, revive, err113 (deferred to Age 2).
All tests pass with race detection.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

### Failure Cases

| Symptom | Cause | Recovery |
|---------|-------|----------|
| `golangci-lint: command not found` | Not installed or not in PATH | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && export PATH=$PATH:$(go env GOPATH)/bin` |
| Timeout on large codebase | Large codebase is heavy | Increase: `golangci-lint run --timeout 15m ./...` |
| `could not load packages` | Go module issue | `go mod tidy && go mod verify` |
| Thousands of errcheck findings | Many unchecked errors | Batch fix: focus on `internal/` first, then `cmd/`, then `pkg/`. Use `//nolint:errcheck` SPARINGLY for intentional ignores with comment |
| `staticcheck: unsupported Go version` | Tool version mismatch | `go install honnef.co/go/tools/cmd/staticcheck@latest` |
| Tests break after lint fixes | Regression from code changes | `git diff` to review changes, `git stash` to isolate, fix the specific change that broke |
| OOM during analysis | Large codebase memory pressure | Run per-directory: `golangci-lint run ./internal/monad/...` |

### Failure Recovery — Multi-Tiered

```bash
# TIER 1: If lint finds > 500 critical issues
# Don't try to fix all at once. Prioritize:
#   1. govet findings (compiler bugs) — fix ALL
#   2. gosec findings with severity HIGH — fix ALL
#   3. errcheck in protocol-critical paths (monad/, sophia/, wotan/) — fix ALL
#   4. Everything else → create tracking issue, commit .golangci.yml, move on

# TIER 2: If lint tool itself crashes
golangci-lint cache clean
golangci-lint run --config .golangci.yml --timeout 15m --concurrency 1 ./...
# Single-threaded avoids race conditions in the linter itself

# TIER 3: If .golangci.yml has syntax errors
golangci-lint linters --config .golangci.yml
# This validates config without running analysis
# Fix YAML syntax, re-run

# TIER 4: Nuclear option — minimal config
# If the full config causes issues, fall back to:
golangci-lint run --enable-only errcheck,govet,staticcheck,unused --timeout 15m ./...
# No config file needed, just CLI flags
```

### Verification Gate

```bash
# MUST ALL PASS before moving to P2:
golangci-lint run --config .golangci.yml ./... && echo "LINT PASS ✅" || echo "LINT FAIL ❌"
go test -race -count=1 ./... && echo "TESTS PASS ✅" || echo "TESTS FAIL ❌"
```

---

## P2: LICH FUZZ WIRING

**Effort**: S (1 hour wire + campaigns run unattended) | **Risk**: LOW | **Blocks**: Nothing directly | **Owner**: Developer + BlackMage

### Happy Path

```bash
# Step 1: Navigate to fuzz directory
cd ~/tmp/unheaded/ebpf/fuzz
# FAILURE CASE: Directory doesn't exist
#   mkdir -p ~/tmp/unheaded/ebpf/fuzz/fuzz_targets
#   mkdir -p ~/tmp/unheaded/ebpf/fuzz/seeds

# Step 2: Verify existing seeds and harnesses
ls -la seeds/
# EXPECTED: Seed files from S25 fuzzing campaigns
ls -la fuzz_targets/ 2>/dev/null || ls -la *.rs 2>/dev/null
# EXPECTED: Harness source files

# Step 3: Deploy Cargo.toml
cp /path/to/cowork/outputs/lich-fuzz-Cargo.toml ./Cargo.toml
# THEN: Edit Cargo.toml to uncomment workspace dependencies
# based on actual crate paths found in ebpf/

# Step 4: Verify crate structure
ls ../  # See sibling crates
# Adjust [dependencies] paths in Cargo.toml to match actual layout
# Example: if monad-common is at ../common, update the path

# Step 5: Verify harness locations
# Harnesses should be in fuzz_targets/
# If they're in the root, move them:
mkdir -p fuzz_targets
mv fuzz_monad_wire.rs fuzz_targets/ 2>/dev/null || true
mv fuzz_sophia_dict.rs fuzz_targets/ 2>/dev/null || true
mv fuzz_crc16.rs fuzz_targets/ 2>/dev/null || true
mv fuzz_exponent_encoding.rs fuzz_targets/ 2>/dev/null || true
mv fuzz_packet_parse.rs fuzz_targets/ 2>/dev/null || true

# Step 6: Build check (don't run campaigns yet)
cargo +nightly check
# EXPECTED: Compiles clean
# This validates Cargo.toml + dependencies without fuzzing

# Step 7: Quick smoke test (10 seconds per target)
for target in fuzz_monad_wire fuzz_crc16 fuzz_exponent_encoding; do
  echo "=== Smoke: $target ==="
  timeout 10 cargo +nightly fuzz run "$target" -- -max_total_time=10 2>&1 | tail -3
done
# EXPECTED: No crashes in 10s smoke test

# Step 8: Launch long campaigns (background, unattended)
for target in fuzz_monad_wire fuzz_sophia_dict fuzz_crc16 fuzz_exponent_encoding fuzz_packet_parse; do
  echo "=== Starting campaign: $target ==="
  nohup cargo +nightly fuzz run "$target" -- \
    -max_total_time=1800 \
    -jobs=2 \
    -workers=2 \
    > "/tmp/lich-${target}.log" 2>&1 &
  echo "  PID: $! → log: /tmp/lich-${target}.log"
done
echo "All LICH campaigns running. Check back in 30 min."

# Step 9: Commit the wiring (don't wait for campaigns)
cd ~/tmp/unheaded
git add ebpf/fuzz/Cargo.toml ebpf/fuzz/fuzz_targets/
git commit -m "feat(lich): wire LICH fuzz crate with Cargo.toml and target structure

The Lich awakens. Automated adversary fuzzing for:
- Monad wire format parsing
- Sophia dictionary operations
- CRC-16/CCITT validation
- Exponent encoding edge cases
- Packet parsing pipeline

Seeds from S25 (28M executions, 0 crashes). Now with a proper crate.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

### Failure Cases

| Symptom | Cause | Recovery |
|---------|-------|----------|
| `cargo +nightly: not found` | Nightly not installed | `rustup toolchain install nightly` |
| `cargo fuzz: command not found` | cargo-fuzz not installed | `cargo install cargo-fuzz` |
| `error[E0463]: can't find crate` | Dependency path wrong | `ls ../` to find actual crate names, fix paths in Cargo.toml |
| `error: linking with cc failed` | Missing system libs | `sudo apt-get install build-essential pkg-config libssl-dev` |
| Seeds directory empty | Seeds lost or moved | Check `find ~/tmp/unheaded -name "*.seed" -o -name "corpus"` |
| Harness compile error | API changed since harness written | Update harness to match current API: `cargo doc --open` on parent crate |
| OOM during fuzzing | Large inputs + small memory | Add `-rss_limit_mb=2048` to fuzz flags |
| Crash found! | **THIS IS A WIN** — LICH found a bug | `cargo +nightly fuzz fmt <target> <crash_file>` to get minimal repro |

### Failure Recovery — Multi-Tiered

```bash
# TIER 1: Cargo.toml dependency resolution fails
# Start minimal — comment out ALL workspace deps, get a building crate,
# then add deps back one at a time
[dependencies]
libfuzzer-sys = "0.4"
arbitrary = { version = "1", features = ["derive"] }
# Everything else commented out

# TIER 2: Harnesses reference non-existent types
# Write a minimal stub harness to prove the crate works:
cat > fuzz_targets/fuzz_smoke.rs << 'EOF'
#![no_main]
use libfuzzer_sys::fuzz_target;
fuzz_target!(|data: &[u8]| {
    // Minimal smoke test — just proves fuzz infrastructure works
    let _ = data.len();
});
EOF
cargo +nightly fuzz run fuzz_smoke -- -max_total_time=5

# TIER 3: Everything is broken, nothing compiles
# Skip the Cargo.toml, use cargo-fuzz init:
cd ~/tmp/unheaded/ebpf
cargo +nightly fuzz init --fuzz-dir fuzz
# This creates a minimal working fuzz crate, then migrate harnesses in
```

### Verification Gate

```bash
# MUST PASS before moving to P3:
cd ~/tmp/unheaded/ebpf/fuzz
cargo +nightly check && echo "LICH BUILD PASS ✅" || echo "LICH BUILD FAIL ❌"
# Campaigns can run in background — don't block on them
```

---

## P3: DOCKER COMPOSE SETUP

**Effort**: S (30 min install + configure) | **Risk**: MEDIUM | **Blocks**: E2E testing | **Owner**: Architect

### Happy Path

```bash
# Step 1: Install Docker (if not present)
which docker && echo "Docker found" || echo "Docker NOT found — installing"

# Option A: Official Docker install (recommended)
curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
sudo sh /tmp/get-docker.sh
sudo usermod -aG docker $USER
# NOTE: Log out and back in for group to take effect
# OR: newgrp docker (for current session)

# Option B: If on NixOS
# Add to configuration.nix:
# virtualisation.docker.enable = true;
# users.users.<username>.extraGroups = [ "docker" ];
# sudo nixos-rebuild switch

# Step 2: Verify Docker works
docker --version
# EXPECTED: Docker version 27.x or newer
docker run --rm hello-world
# EXPECTED: "Hello from Docker!"

# Step 3: Install Docker Compose v2 (built into Docker now)
docker compose version
# EXPECTED: Docker Compose version v2.x
# FAILURE: See failure cases below

# Step 4: Deploy docker-compose.yml
cp /path/to/cowork/outputs/docker-compose.yml ~/tmp/unheaded/docker-compose.yml

# Step 5: Create config directories for mounted volumes
mkdir -p ~/tmp/unheaded/config/vector
mkdir -p ~/tmp/unheaded/config/coredns

# Step 6: Create Vector config
cat > ~/tmp/unheaded/config/vector/vector.yaml << 'VECEOF'
# Vector pipeline: collect → transform → sink to ClickHouse
sources:
  unheaded_logs:
    type: file
    include:
      - /var/log/unheaded/*.log
    read_from: beginning

  internal_metrics:
    type: internal_metrics

transforms:
  parse_json:
    type: remap
    inputs: ["unheaded_logs"]
    source: |
      . = parse_json!(.message)
      .timestamp = now()
      .host = get_hostname!()

sinks:
  clickhouse:
    type: clickhouse
    inputs: ["parse_json"]
    endpoint: "http://clickhouse:8123"
    database: "unheaded_logs"
    table: "events"
    auth:
      strategy: basic
      user: unheaded
      password: dev-only-change-in-prod
    batch:
      max_bytes: 10485760
      timeout_secs: 5
    healthcheck:
      enabled: true

  console_debug:
    type: console
    inputs: ["parse_json"]
    encoding:
      codec: json
VECEOF

# Step 7: Create CoreDNS config
cat > ~/tmp/unheaded/config/coredns/Corefile << 'DNSEOF'
# CoreDNS config for Unheaded dev environment
unheaded.local:53 {
    log
    errors
    health {
        lameduck 5s
    }
    ready
    hosts {
        172.28.0.10 traefik.unheaded.local gateway.unheaded.local
        172.28.2.10 victoria.unheaded.local metrics.unheaded.local
        172.28.2.11 clickhouse.unheaded.local logs.unheaded.local
        172.28.2.12 grafana.unheaded.local dashboard.unheaded.local
        fallthrough
    }
    cache 30
    loop
    reload
}

.:53 {
    forward . 8.8.8.8 8.8.4.4 {
        tls_servername dns.google
        health_check 5s
    }
    cache 30
    log
    errors
}
DNSEOF

# Step 8: Start the stack
cd ~/tmp/unheaded
docker compose up -d
# EXPECTED: All services start without error

# Step 9: Verify all services are healthy
sleep 30  # Give services time to initialize
docker compose ps
# EXPECTED: All services show "healthy" or "running"

# Step 10: Smoke test each service
echo "--- Traefik Dashboard ---"
curl -sf http://localhost:8080/api/overview | jq .
echo "--- VictoriaMetrics ---"
curl -sf http://localhost:8428/-/healthy
echo "--- ClickHouse ---"
curl -sf "http://localhost:8123/?query=SELECT%201"
echo "--- Grafana ---"
curl -sf http://localhost:3001/api/health | jq .
echo "--- CoreDNS ---"
dig @localhost -p 5353 health.unheaded.local +short

# Step 11: Commit
git add docker-compose.yml config/
git commit -m "feat(infra): add Docker Compose dev stack with observability

Services: Traefik 3.x (HTTP/3+QUIC), VictoriaMetrics, ClickHouse,
Vector, Grafana, CoreDNS. IPv6 dual-stack networks. Health checks
on all services. Labeled with Kingdom armor mapping.

S26 Decision: Docker for dev, LXD for prod (reconcile in Age 2).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

### Failure Cases

| Symptom | Cause | Recovery |
|---------|-------|----------|
| `docker: command not found` | Not installed | Follow Step 1 install |
| `permission denied` on docker socket | User not in docker group | `sudo usermod -aG docker $USER && newgrp docker` |
| `docker compose: command not found` | Docker Compose v2 not bundled | `sudo apt-get install docker-compose-plugin` OR standalone: `sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose && sudo chmod +x /usr/local/bin/docker-compose` |
| Port 80/443 already in use | Other web server running | `sudo lsof -i :80` → stop conflicting service, OR change ports in docker-compose.yml |
| Port 3000 conflict (Fiber pilot) | Grafana default port clash | Already handled — Grafana on 3001 |
| ClickHouse OOM | Not enough RAM | Reduce memory limit in deploy.resources or skip ClickHouse for now |
| IPv6 network creation fails | Docker IPv6 not enabled | Add to `/etc/docker/daemon.json`: `{"ipv6": true, "fixed-cidr-v6": "fd00::/80"}` then `sudo systemctl restart docker` |
| `no space left on device` | Disk full from images | `docker system prune -a` (WARNING: removes all unused images) |
| Service stuck in "starting" | Dependency not healthy | Check: `docker compose logs <service>` → fix underlying issue |
| ARM64 image not available | Missing multi-arch image | Add `platform: linux/arm64` to service, or find ARM64-compatible image tag |

### Failure Recovery — Multi-Tiered

```bash
# TIER 1: Single service won't start
docker compose logs <service> 2>&1 | tail -20
# Read the error. Fix config. Restart just that service:
docker compose restart <service>

# TIER 2: Network issues between containers
docker compose exec traefik ping -c 3 victoria
# If no connectivity, check network config:
docker network ls | grep unheaded
docker network inspect unheaded-dev_data

# TIER 3: Start with minimal stack (just infra, no apps)
docker compose up -d traefik victoria grafana
# Add services one at a time once base is stable

# TIER 4: Nuclear — blow it all away and rebuild
docker compose down -v --remove-orphans
docker system prune -f
docker compose up -d --build --force-recreate

# TIER 5: Docker itself is broken
sudo systemctl restart docker
# If still broken: sudo apt-get purge docker-ce && re-install
```

### Verification Gate

```bash
# MUST ALL PASS to close P3:
docker compose ps --format json | jq -r '.[] | "\(.Name): \(.Health // .State)"'
# ALL services should show "healthy" or "running"

curl -sf http://localhost:8080/api/overview > /dev/null && echo "TRAEFIK PASS ✅" || echo "TRAEFIK FAIL ❌"
curl -sf http://localhost:8428/-/healthy > /dev/null && echo "VICTORIA PASS ✅" || echo "VICTORIA FAIL ❌"
curl -sf "http://localhost:8123/?query=SELECT%201" > /dev/null && echo "CLICKHOUSE PASS ✅" || echo "CLICKHOUSE FAIL ❌"
curl -sf http://localhost:3001/api/health > /dev/null && echo "GRAFANA PASS ✅" || echo "GRAFANA FAIL ❌"
```

---

## POST-SPRINT VERIFICATION

After completing P0-P3, run the full verification suite:

```bash
cd ~/tmp/unheaded

echo "═══════════════════════════════════════════"
echo "  S27 POST-SPRINT VERIFICATION"
echo "═══════════════════════════════════════════"

# 1. Code still builds
echo "[1/6] Go build..."
go build ./... && echo "  BUILD PASS ✅" || echo "  BUILD FAIL ❌ — STOP AND FIX"

# 2. Tests still pass
echo "[2/6] Go tests (with race detection)..."
go test -race -count=1 ./... && echo "  TESTS PASS ✅" || echo "  TESTS FAIL ❌ — STOP AND FIX"

# 3. Lint is clean
echo "[3/6] Lint (critical-only)..."
golangci-lint run --config .golangci.yml ./... && echo "  LINT PASS ✅" || echo "  LINT FAIL ❌"

# 4. Nomenclature clean
echo "[4/6] Nomenclature..."
MATRIARCH_COUNT=$(grep -rc "MATRIARCH\|PATRIARCH" .skills/skills/ 2>/dev/null | awk -F: '{s+=$2}END{print s+0}')
[ "$MATRIARCH_COUNT" -eq 0 ] && echo "  NOMENCLATURE PASS ✅" || echo "  NOMENCLATURE FAIL ❌ ($MATRIARCH_COUNT remaining)"

# 5. LICH crate builds
echo "[5/6] LICH fuzz crate..."
(cd ebpf/fuzz && cargo +nightly check 2>/dev/null) && echo "  LICH PASS ✅" || echo "  LICH FAIL ❌ (non-blocking)"

# 6. Docker stack healthy
echo "[6/6] Docker stack..."
HEALTHY=$(docker compose ps --format json 2>/dev/null | jq -r '.[].Health' | grep -c "healthy" || echo "0")
TOTAL=$(docker compose ps --format json 2>/dev/null | jq -r '.[].Name' | wc -l || echo "0")
echo "  DOCKER: $HEALTHY/$TOTAL healthy"
[ "$HEALTHY" -ge 3 ] && echo "  DOCKER PASS ✅" || echo "  DOCKER WARN ⚠️ (some services unhealthy)"

echo ""
echo "═══════════════════════════════════════════"
echo "  S27 VERIFICATION COMPLETE"
echo "═══════════════════════════════════════════"
```

---

## SPRINT EXIT CHECKLIST

Before ending S27:

- [ ] All P0-P3 verification gates PASS
- [ ] Post-sprint verification all green (or documented exceptions)
- [ ] Git log shows clean commit history for this session
- [ ] No uncommitted changes (`git status` clean)
- [ ] LICH campaigns either complete or documented as running
- [ ] Docker stack either healthy or documented blockers
- [ ] Write S27 handoff with: commit SHAs, test counts, blockers, next priorities
- [ ] Update timeline.md: `ALPHA 99% READY` → `ALPHA 100% — GATE CLOSED` (if all pass)

---

## OPEN QUESTIONS (Carry Forward)

1. **Docker vs Podman vs LXD-native** — Docker for dev (this session), LXD for prod (Age 2)
2. **CI/CD provider** — Defer to S28
3. **Multi-tenant isolation** — Defer to Age 2 planning
4. **ARM64 compatibility** — Dev machine is arm64, verify all Docker images support it

---

*Prepared by S27 Cowork Sprint — February 21, 2026*
*The Round Table forged the plan. This runbook executes it.*
*Peace and Love — Mad-Maria watches over the Kingdom* 🏰
