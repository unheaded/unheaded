# LICH HARDENING CAMPAIGN BATTLE PLAN — 6 Phases, 85+ Steps

**Date**: 2026-03-25
**Sprint**: S79 — Lich Hardening Campaign (Post-Ragnarok)
**Prerequisite**: All 35 existing Lich targets PASS, bare metal WEST+EAST online, WireGuard + BGP operational
**Target**: Zero unfuzzed attack surfaces. 24h+ campaigns on all critical targets. New harnesses for HTTP API, Sophia hot-swap, PQC, and cross-service chains.
**Estimated Duration**: 3-5 sessions (campaigns run unattended overnight)
**Agent Strategy**: Phase 1 sequential (fixes + harness writing), Phases 2-5 parallelizable (campaigns run concurrently), Phase 6 sequential (triage)
**Commit Cadence**: Every 4 steps
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

---

## LEGEND

[B] = Bash command
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo/elevated privileges required
[P] = Parallelizable with other [P] steps
[C] = Commit checkpoint
[CODE] = Code implementation
[TEST] = Test execution
[SECURITY] = Security review step
[BARE-METAL] = Requires real hardware/kernel/BPF

---

## PHASE 0: PREFLIGHT — Environment & Baseline (Steps 1-12)

**Goal**: Verify Lich framework operational, record baseline, confirm bare metal readiness.
**Time**: 15 minutes
**Agent**: Coordinator

- [ ] **Step 1** [B] ~30s: Set project root
  ```bash
  export PROJECT_ROOT=$(cd ~/tmp/unheaded && pwd) && echo $PROJECT_ROOT
  ```

- [ ] **Step 2** [B] ~30s: Verify existing 35 targets still pass
  ```bash
  cd $PROJECT_ROOT && bash tomb/lich/lich-runner.sh -d 5 2>&1 | tail -10
  ```

- [ ] **Step 3** [V]: **BASELINE GATE** — 35 passed, 0 failed, 0 skipped
  - If fail → STOP. Do not proceed with broken baseline.

- [ ] **Step 4** [B] ~30s: Check kernel version and BPF support
  ```bash
  uname -r && bpftool version 2>/dev/null && echo "BPF tools OK" || echo "MISSING bpftool"
  ```

- [ ] **Step 5** [B] ~30s: Verify services can start (needed for HTTP API fuzzing)
  ```bash
  cd $PROJECT_ROOT && make build 2>&1 | tail -3
  ```

- [ ] **Step 6** [V]: Build succeeds — `All binaries built`

- [ ] **Step 7** [B] ~30s: Count API endpoints to fuzz
  ```bash
  grep -rn "HandleFunc.*api/v1" $PROJECT_ROOT/cmd/ $PROJECT_ROOT/services/ 2>/dev/null | grep -v test | wc -l
  ```

- [ ] **Step 8** [V]: API endpoint count recorded (expect ~128)

- [ ] **Step 9** [B] ~30s: Verify WireGuard tunnel for cross-host tests
  ```bash
  ping6 -c 2 -W 1 fd00:dead:beef::1 2>&1 | tail -3
  ```

- [ ] **Step 10** [V]: Cross-host connectivity confirmed (0% loss)

- [ ] **Step 11** [B] ~30s: Check race detector works
  ```bash
  cd $PROJECT_ROOT && go test -race -count=1 -run TestNONE ./tomb/lich/harnesses/ 2>&1 | tail -3
  ```

- [ ] **Step 12** [C]: **PREFLIGHT COMMIT**
  ```bash
  echo "Preflight passed $(date)" >> $PROJECT_ROOT/tomb/lich/CAMPAIGN_LOG.md
  ```

- [ ] **Step 12** [V]: **PHASE 0 EXIT GATE** — Baseline green, build clean, cross-host live, race detector functional

---

## PHASE 1: NEW HARNESSES + FIXES (Steps 13-45)

**Goal**: Write LICH-011 (Sophia hot-swap), LICH-012 (PQC), LICH-013 (HTTP API), LICH-014 (cross-service chains). Fix doom injector cast.
**Time**: 2-3 hours
**Agent**: Coordinator or Agent (code writing)

### Fix: Doom Injector uint64→int Cast

- [ ] **Step 13** [R] ~1m: Read the fragile cast
  ```bash
  sed -n '340,350p' $PROJECT_ROOT/cmd/doom-go-injector/main.go
  ```

- [ ] **Step 14** [CODE] ~5m: Fix with saturating arithmetic
  Replace `remaining := int(uint64(count) - sent)` with bounds-checked version:
  ```go
  raw := uint64(count) - sent
  remaining := int(raw)
  if raw > uint64(math.MaxInt) {
      remaining = math.MaxInt
  }
  ```

- [ ] **Step 15** [V] ~30s: Build passes
  ```bash
  cd $PROJECT_ROOT && go build ./cmd/doom-go-injector/... 2>&1
  ```

- [ ] **Step 16** [C]: Commit doom injector fix

### LICH-011: Sophia Dictionary Hot-Swap Race

- [ ] **Step 17** [R] ~2m: Read Sophia dictionary update code
  ```bash
  grep -rn "swap\|update\|replace\|rotate" $PROJECT_ROOT/services/sophia/ --include="*.go" | head -20
  ```

- [ ] **Step 18** [R] ~2m: Read existing Sophia service for map access patterns
  ```bash
  cat $PROJECT_ROOT/services/sophia/sophia.go | head -100
  ```

- [ ] **Step 19** [CODE][W] ~20m: Write LICH-011 harness
  Create `$PROJECT_ROOT/tomb/lich/harnesses/lich_011_sophia_hotswap_test.go`:
  - `FuzzSophiaDictionarySwap`: Concurrent goroutines doing lookups while another goroutine replaces the dictionary map. Uses sync.WaitGroup + atomic pointer swap.
  - `FuzzSophiaEpochRollover`: Epoch counter overflow during dictionary rotation (uint8 → 0 wrap).
  - `FuzzSophiaConcurrentLookup`: Multiple goroutines reading different keys from same dictionary under write pressure.

- [ ] **Step 20** [V] ~1m: Harness compiles
  ```bash
  cd $PROJECT_ROOT && go test -c -o /dev/null ./tomb/lich/harnesses/ 2>&1
  ```

- [ ] **Step 21** [TEST] ~30s: Quick smoke run
  ```bash
  cd $PROJECT_ROOT && go test -fuzz=FuzzSophiaDictionarySwap -fuzztime=5s ./tomb/lich/harnesses/ 2>&1 | tail -5
  ```

- [ ] **Step 22** [C]: Commit LICH-011

### LICH-012: PQC Signature Verification Fuzzing

- [ ] **Step 23** [R] ~2m: Read PQC implementation
  ```bash
  find $PROJECT_ROOT/pkg -name "*pqc*" -o -name "*crypto*" | head -10
  grep -rn "Verify\|Sign\|MLKEM\|MLDSA\|Dilithium" $PROJECT_ROOT/pkg/ --include="*.go" | head -20
  ```

- [ ] **Step 24** [CODE][W] ~20m: Write LICH-012 harness
  Create `$PROJECT_ROOT/tomb/lich/harnesses/lich_012_pqc_fuzz_test.go`:
  - `FuzzPQCSignatureValidation`: Fuzz ML-DSA-65 signature verification with corrupted signatures, truncated keys, wrong algorithm IDs.
  - `FuzzPQCKeyDerivation`: Fuzz ML-KEM-768 key encapsulation with malformed public keys.
  - `FuzzPQCAlgorithmIDMismatch`: Send signature with algo_id=0x03 (ML-DSA-65) but actual bytes from algo_id=0x05 (SLH-DSA).

- [ ] **Step 25** [V] ~1m: Harness compiles
  ```bash
  cd $PROJECT_ROOT && go test -c -o /dev/null ./tomb/lich/harnesses/ 2>&1
  ```

- [ ] **Step 26** [TEST] ~30s: Quick smoke
  ```bash
  cd $PROJECT_ROOT && go test -fuzz=FuzzPQCSignatureValidation -fuzztime=5s ./tomb/lich/harnesses/ 2>&1 | tail -5
  ```

- [ ] **Step 27** [C]: Commit LICH-012

### LICH-013: HTTP API Endpoint Fuzzing

- [ ] **Step 28** [R] ~3m: Catalog all API endpoints by service
  ```bash
  grep -rn "HandleFunc.*api/v1" $PROJECT_ROOT/cmd/ $PROJECT_ROOT/services/ 2>/dev/null | grep -v test | \
    awk -F: '{print $1}' | sort -u
  ```

- [ ] **Step 29** [CODE][W] ~30m: Write LICH-013 harness
  Create `$PROJECT_ROOT/tomb/lich/harnesses/lich_013_http_api_test.go`:
  - `FuzzHTTPAPIKanbanTasks`: Fuzz POST /api/v1/tasks with malformed JSON, oversized bodies, invalid UTF-8, SQLi payloads, XSS payloads.
  - `FuzzHTTPAPIMonadEncode`: Fuzz POST /api/v1/monad/encode with corrupted Monad registers, oversized values, invalid version fields.
  - `FuzzHTTPAPIWotanWrite`: Fuzz POST /api/v1/wotan/write with crafted topic names (path traversal), oversized messages, null bytes.
  - `FuzzHTTPAPIHeaderInjection`: Fuzz all endpoints with malformed HTTP headers (CRLF injection, oversized headers, duplicate Host).
  Uses httptest.NewServer with real handlers from each service.

- [ ] **Step 30** [V] ~1m: Harness compiles
  ```bash
  cd $PROJECT_ROOT && go test -c -o /dev/null ./tomb/lich/harnesses/ 2>&1
  ```

- [ ] **Step 31** [TEST] ~30s: Quick smoke
  ```bash
  cd $PROJECT_ROOT && go test -fuzz=FuzzHTTPAPIKanbanTasks -fuzztime=5s ./tomb/lich/harnesses/ 2>&1 | tail -5
  ```

- [ ] **Step 32** [C]: Commit LICH-013

### LICH-014: Cross-Service Attack Chains

- [ ] **Step 33** [R] ~3m: Map Wotan → Sophia → Monad data flow
  ```bash
  grep -rn "Publish\|Subscribe\|wotan\." $PROJECT_ROOT/services/sophia/ --include="*.go" | head -10
  grep -rn "sophia\.\|dictionary\|lookup" $PROJECT_ROOT/pkg/protocol/ --include="*.go" | head -10
  ```

- [ ] **Step 34** [CODE][W] ~25m: Write LICH-014 harness
  Create `$PROJECT_ROOT/tomb/lich/harnesses/lich_014_cross_service_test.go`:
  - `FuzzCrossServiceWotanToSophia`: Inject malicious Wotan message on dictionary update topic → verify Sophia rejects corrupted dictionary payloads.
  - `FuzzCrossServiceSophiaToMonad`: Craft Sophia dictionary entry with exponent values that overflow when Monad decodes them → verify Monad's exponent decoder bounds-checks.
  - `FuzzCrossServiceChainedCorruption`: Full chain: forge Wotan message → trigger Sophia dictionary load → Sophia returns corrupted field → Monad processes corrupted register. Test that corruption is detected at EACH boundary.

- [ ] **Step 35** [V] ~1m: Harness compiles
  ```bash
  cd $PROJECT_ROOT && go test -c -o /dev/null ./tomb/lich/harnesses/ 2>&1
  ```

- [ ] **Step 36** [TEST] ~30s: Quick smoke
  ```bash
  cd $PROJECT_ROOT && go test -fuzz=FuzzCrossServiceWotanToSophia -fuzztime=5s ./tomb/lich/harnesses/ 2>&1 | tail -5
  ```

- [ ] **Step 37** [C]: Commit LICH-014

### Update lich-runner.sh

- [ ] **Step 38** [CODE] ~5m: Add LICH-011 through LICH-014 to lich-runner.sh target list
  Append new harness names to the TARGETS array in `tomb/lich/lich-runner.sh`.

- [ ] **Step 39** [V] ~1m: Full suite compiles and dry-run passes
  ```bash
  cd $PROJECT_ROOT && go test -c -o /dev/null ./tomb/lich/harnesses/ 2>&1
  ```

- [ ] **Step 40** [B] ~3m: Run full suite (10s each, smoke test)
  ```bash
  cd $PROJECT_ROOT && bash tomb/lich/lich-runner.sh -d 10 2>&1 | tail -15
  ```

- [ ] **Step 41** [V]: **PHASE 1 EXIT GATE** — All targets (35 old + new) compile and pass 10s smoke
  - Expected: ~47 targets total (35 + 3 + 3 + 4 + 3 = 48, minus any that share files)

- [ ] **Step 42** [C]: **PHASE 1 COMMIT**
  ```bash
  git add tomb/lich/ cmd/doom-go-injector/ && git commit -m "feat(lich): LICH-011/012/013/014 harnesses + doom injector fix"
  ```

---

## PHASE 2: EXTENDED CRC CAMPAIGN (Steps 43-50)

**Goal**: Run LICH-003 CRC collision hunting for 24+ hours. This is the campaign BlackMage said needs real time.
**Time**: 24 hours (runs overnight, unattended)
**Agent**: Background process [P]

- [ ] **Step 43** [B] ~1m: Create results directory
  ```bash
  mkdir -p $PROJECT_ROOT/tomb/lich/results/crc-24h-$(date +%Y%m%d)
  ```

- [ ] **Step 44** [B][P] ~24h: Launch CRC collision campaign (background)
  ```bash
  cd $PROJECT_ROOT && nohup bash -c '
    for target in FuzzCRC16CollisionPair FuzzCRC16StealthBitFlip FuzzCRC16MonadCollision FuzzCRC16LengthExtension FuzzCRC32MPEG2Collision; do
      echo "=== Starting $target at $(date) ==="
      go test -fuzz=$target -fuzztime=5h ./tomb/lich/harnesses/ 2>&1 | tail -5
      echo "=== Finished $target at $(date) ==="
    done
  ' > tomb/lich/results/crc-24h-$(date +%Y%m%d)/campaign.log 2>&1 &
  echo "CRC campaign PID: $!"
  ```

- [ ] **Step 45** [V] ~1m: Verify campaign is running
  ```bash
  ps aux | grep "go test.*Fuzz" | grep -v grep | head -5
  ```

- [ ] **Step 46** [C]: Log campaign start

**Exit Gate**: Campaign running. Results checked in Phase 6 (triage).

---

## PHASE 3: RACE DETECTION CAMPAIGNS (Steps 47-56)

**Goal**: Run LICH-007 (MBC VM) and LICH-008 (ring buffer) with -race flag. Run LICH-011 (Sophia hot-swap) with -race.
**Time**: 2-4 hours (runs in parallel with Phase 2)
**Agent**: Background process [P]

- [ ] **Step 47** [B] ~1m: Create results directory
  ```bash
  mkdir -p $PROJECT_ROOT/tomb/lich/results/race-$(date +%Y%m%d)
  ```

- [ ] **Step 48** [B][P] ~2h: Launch MBC VM race campaign
  ```bash
  cd $PROJECT_ROOT && nohup bash -c '
    for target in FuzzMBCExecution FuzzMBCArithmeticEdgeCases; do
      echo "=== $target -race at $(date) ==="
      go test -race -fuzz=$target -fuzztime=1h ./tomb/lich/harnesses/ 2>&1 | tail -10
    done
  ' > tomb/lich/results/race-$(date +%Y%m%d)/mbc-race.log 2>&1 &
  echo "MBC race PID: $!"
  ```

- [ ] **Step 49** [B][P] ~2h: Launch ring buffer race campaign
  ```bash
  cd $PROJECT_ROOT && nohup bash -c '
    for target in FuzzCacheRingBufferPush FuzzCacheRingBufferQuery FuzzCacheRingBufferInterleaved FuzzCacheRingBufferCapacityBoundary; do
      echo "=== $target -race at $(date) ==="
      go test -race -fuzz=$target -fuzztime=30m ./tomb/lich/harnesses/ 2>&1 | tail -10
    done
  ' > tomb/lich/results/race-$(date +%Y%m%d)/ringbuf-race.log 2>&1 &
  echo "Ring buffer race PID: $!"
  ```

- [ ] **Step 50** [B][P] ~1h: Launch Sophia hot-swap race campaign
  ```bash
  cd $PROJECT_ROOT && nohup bash -c '
    for target in FuzzSophiaDictionarySwap FuzzSophiaConcurrentLookup FuzzSophiaEpochRollover; do
      echo "=== $target -race at $(date) ==="
      go test -race -fuzz=$target -fuzztime=20m ./tomb/lich/harnesses/ 2>&1 | tail -10
    done
  ' > tomb/lich/results/race-$(date +%Y%m%d)/sophia-race.log 2>&1 &
  echo "Sophia race PID: $!"
  ```

- [ ] **Step 51** [V] ~30s: All 3 campaigns running
  ```bash
  ps aux | grep "go test.*-race.*Fuzz" | grep -v grep | wc -l
  ```

- [ ] **Step 52** [C]: Log race campaigns start

**Exit Gate**: 3 race campaigns running. Results checked in Phase 6.

---

## PHASE 4: HTTP API + CROSS-SERVICE CAMPAIGNS (Steps 53-62)

**Goal**: Run LICH-013 (HTTP API) and LICH-014 (cross-service) for extended periods. These need running services.
**Time**: 2-3 hours
**Agent**: Coordinator (needs to start services)

- [ ] **Step 53** [B] ~30s: Start minimum services for API fuzzing
  ```bash
  cd $PROJECT_ROOT
  ./bin/wotan &> /tmp/wotan-lich.log &
  ./bin/kanban-app &> /tmp/kanban-lich.log &
  sleep 2
  curl -s http://localhost:18000/health && curl -s http://localhost:16668/health && echo " services up"
  ```

- [ ] **Step 54** [V]: Services responding on health endpoints

- [ ] **Step 55** [B] ~1m: Create results directory
  ```bash
  mkdir -p $PROJECT_ROOT/tomb/lich/results/api-$(date +%Y%m%d)
  ```

- [ ] **Step 56** [B][P] ~2h: Launch HTTP API fuzzing campaign
  ```bash
  cd $PROJECT_ROOT && nohup bash -c '
    for target in FuzzHTTPAPIKanbanTasks FuzzHTTPAPIMonadEncode FuzzHTTPAPIWotanWrite FuzzHTTPAPIHeaderInjection; do
      echo "=== $target at $(date) ==="
      go test -fuzz=$target -fuzztime=30m ./tomb/lich/harnesses/ 2>&1 | tail -10
    done
  ' > tomb/lich/results/api-$(date +%Y%m%d)/api-fuzz.log 2>&1 &
  echo "API fuzz PID: $!"
  ```

- [ ] **Step 57** [B][P] ~1h: Launch cross-service chain campaign
  ```bash
  cd $PROJECT_ROOT && nohup bash -c '
    for target in FuzzCrossServiceWotanToSophia FuzzCrossServiceSophiaToMonad FuzzCrossServiceChainedCorruption; do
      echo "=== $target at $(date) ==="
      go test -fuzz=$target -fuzztime=20m ./tomb/lich/harnesses/ 2>&1 | tail -10
    done
  ' > tomb/lich/results/api-$(date +%Y%m%d)/cross-service.log 2>&1 &
  echo "Cross-service PID: $!"
  ```

- [ ] **Step 58** [V] ~30s: Both campaigns running
  ```bash
  ps aux | grep "go test.*Fuzz" | grep -v grep | wc -l
  ```

- [ ] **Step 59** [C]: Log API campaigns start

**Exit Gate**: API + cross-service campaigns running. Results in Phase 6.

---

## PHASE 5: BARE METAL BPF LOADING (Steps 60-72)

**Goal**: Actually load BPF programs into the kernel and fuzz the load/attach path. LICH-005 upgrade.
**Time**: 1-2 hours
**Agent**: Coordinator [S] (requires sudo for BPF)

- [ ] **Step 60** [S][B] ~1m: Check BPF filesystem mounted
  ```bash
  mount | grep bpf && echo "bpffs mounted" || sudo mount -t bpf bpf /sys/fs/bpf
  ```

- [ ] **Step 61** [B] ~2m: Build eBPF programs
  ```bash
  cd $PROJECT_ROOT && make ebpf 2>&1 | tail -10
  ```

- [ ] **Step 62** [V]: eBPF programs compiled (check for .o files)
  ```bash
  find $PROJECT_ROOT/ebpf -name "*.o" -newer $PROJECT_ROOT/Makefile | head -10
  ```

- [ ] **Step 63** [S][B] ~5m: Attempt to load shield-ebpf into kernel
  ```bash
  sudo bpftool prog load $PROJECT_ROOT/ebpf/shield-ebpf/target/bpfel-unknown-none/release/shield-ebpf /sys/fs/bpf/unheaded/test_shield 2>&1 || echo "Load failed — documenting"
  ```

- [ ] **Step 64** [V]: Document result (PASS if loaded, or document verifier rejection)
  ```bash
  sudo bpftool prog list | grep unheaded 2>&1 || echo "No programs loaded — expected if verifier rejected"
  ```

- [ ] **Step 65** [S][B] ~2m: Clean up test program
  ```bash
  sudo rm -f /sys/fs/bpf/unheaded/test_shield 2>/dev/null
  ```

- [ ] **Step 66** [B] ~5m: Fuzz BPF ELF loading path with malformed binaries
  ```bash
  cd $PROJECT_ROOT && go test -fuzz=FuzzELFSectionClassifier -fuzztime=5m ./tomb/lich/harnesses/ 2>&1 | tail -5
  ```

- [ ] **Step 67** [V]: No crashes from ELF fuzzing

- [ ] **Step 68** [SECURITY] ~10m: Document BPF loading results
  Write findings to `$PROJECT_ROOT/tomb/lich/results/bpf-load-$(date +%Y%m%d).md`:
  - Which programs loaded successfully
  - Which were rejected by verifier (and why)
  - Whether the test environment matches production kernel

- [ ] **Step 69** [C]: Commit BPF loading results

- [ ] **Step 70** [V]: **PHASE 5 EXIT GATE** — BPF loading attempted, results documented

---

## PHASE 6: TRIAGE & REPORTING (Steps 71-85)

**Goal**: Collect results from all overnight campaigns, triage crashes, produce final report.
**Time**: 1-2 hours (run AFTER overnight campaigns complete)
**Agent**: Coordinator

- [ ] **Step 71** [V] ~1m: Check all campaigns finished
  ```bash
  ps aux | grep "go test.*Fuzz" | grep -v grep | wc -l
  # Should be 0 if all campaigns completed
  ```

- [ ] **Step 72** [D]: If campaigns still running, check progress
  ```bash
  tail -5 $PROJECT_ROOT/tomb/lich/results/*/campaign.log $PROJECT_ROOT/tomb/lich/results/*/*.log 2>/dev/null
  ```

### Collect Results

- [ ] **Step 73** [B] ~2m: Scan for new crash corpus entries
  ```bash
  find $PROJECT_ROOT/tomb/lich/harnesses/testdata/fuzz/ -newer $PROJECT_ROOT/tomb/lich/results/ -type f 2>/dev/null
  ```

- [ ] **Step 74** [B] ~1m: Count crashes per campaign
  ```bash
  for dir in $PROJECT_ROOT/tomb/lich/results/*/; do
    crashes=$(find "$dir" -path "*/crashes/*" -type f 2>/dev/null | wc -l)
    echo "$(basename $dir): $crashes crashes"
  done
  ```

- [ ] **Step 75** [B] ~2m: Check race detector output
  ```bash
  grep -l "DATA RACE" $PROJECT_ROOT/tomb/lich/results/race-*/*.log 2>/dev/null && echo "RACES FOUND" || echo "No races detected"
  ```

- [ ] **Step 76** [V]: **RACE GATE** — If races found → P0, document and escalate

### Triage Crashes

- [ ] **Step 77** [SECURITY] ~15m: For each crash found, assess severity:
  - Read crash input
  - Determine if it causes panic (MEDIUM) or memory corruption (CRITICAL)
  - Check if the vulnerable code path is reachable from network input
  - Rate: CRITICAL / HIGH / MEDIUM / LOW / INFO

- [ ] **Step 78** [B] ~5m: Reproduce any crashes found
  ```bash
  # For each crash file, re-run with the specific input
  for crash in $(find $PROJECT_ROOT/tomb/lich/harnesses/testdata/fuzz/ -newer $PROJECT_ROOT/tomb/lich/results/ -type f 2>/dev/null); do
    target=$(basename $(dirname $crash))
    echo "Reproducing $target with $(basename $crash)..."
    go test -run="$target/$(basename $crash)" ./tomb/lich/harnesses/ 2>&1 | tail -3
  done
  ```

### Produce Report

- [ ] **Step 79** [W] ~20m: Write campaign report
  Create `$PROJECT_ROOT/tomb/lich/results/HARDENING-CAMPAIGN-REPORT.md`:
  - Campaign date and duration
  - Targets run (old 35 + new harnesses)
  - Total fuzz time per target
  - Crashes found (count, severity, location)
  - Race conditions found (count, location)
  - CRC collisions found (yes/no, if yes → CRITICAL)
  - API vulnerabilities found (count, type)
  - Cross-service chain weaknesses found
  - BPF loading results
  - Recommendations for next campaign

- [ ] **Step 80** [B] ~30s: Run full quick suite to confirm no regressions
  ```bash
  cd $PROJECT_ROOT && bash tomb/lich/lich-runner.sh -d 10 2>&1 | tail -10
  ```

- [ ] **Step 81** [V]: All targets still pass (no regressions from new harnesses)

- [ ] **Step 82** [C]: Commit full campaign results
  ```bash
  git add tomb/lich/ && git commit -m "security(lich): hardening campaign results — LICH-011/012/013/014 + 24h CRC + race detection"
  ```

- [ ] **Step 83** [V]: **PHASE 6 EXIT GATE** — Report written, all results committed, no P0 findings unaddressed

---

## APPENDIX A: Campaign Duration Matrix

| Campaign | Targets | Duration | Mode | Phase |
|----------|---------|----------|------|-------|
| CRC collision (LICH-003) | 5 targets | 5h each = 25h total | Sequential | 2 |
| MBC VM race (LICH-007) | 2 targets | 1h each = 2h | -race | 3 |
| Ring buffer race (LICH-008) | 4 targets | 30m each = 2h | -race | 3 |
| Sophia hot-swap (LICH-011) | 3 targets | 20m each = 1h | -race | 3 |
| PQC signature (LICH-012) | 3 targets | 20m each = 1h | Normal | 3 |
| HTTP API (LICH-013) | 4 targets | 30m each = 2h | Normal | 4 |
| Cross-service (LICH-014) | 3 targets | 20m each = 1h | Normal | 4 |
| BPF loading (LICH-005+) | 1 target | 5m | Bare metal | 5 |
| **Total fuzz time** | **~25 targets** | **~34 hours** | | |

## APPENDIX B: Attack Surface Coverage After Campaign

| Surface | Before | After | Gap |
|---------|--------|-------|-----|
| Monad wire format | HARDENED (10s) | HARDENED (10s) | Run 24h in next campaign |
| CRC collision | WARMUP (10s) | **HARDENED (25h)** | Closed |
| MBC VM execution | TESTED (10s) | **RACE-TESTED (2h)** | Closed |
| Ring buffer concurrency | TESTED (10s) | **RACE-TESTED (2h)** | Closed |
| Sophia hot-swap | **UNTESTED** | **RACE-TESTED (1h)** | Closed |
| PQC signatures | **UNTESTED** | **TESTED (1h)** | Run 24h next |
| HTTP API endpoints (128) | **UNTESTED** | **TESTED (2h)** | Run 24h next |
| Cross-service chains | **UNTESTED** | **TESTED (1h)** | Run 24h next |
| BPF kernel loading | **UNTESTED** | **DOCUMENTED** | Needs syzkaller for deep coverage |
| WAL integrity | HARDENED (10s) | HARDENED | Overflow fixed |

## APPENDIX C: Severity Response Matrix

| Finding Type | Severity | Response | SLA |
|-------------|----------|----------|-----|
| Race condition in production code | HIGH | Fix immediately, re-run -race | 24h |
| CRC-16 collision found | CRITICAL | Evaluate polynomial change | Immediate |
| HTTP API panic on fuzzed input | MEDIUM | Add input validation, add to regression | This sprint |
| Cross-service corruption propagates | HIGH | Add boundary validation at each service | 24h |
| BPF verifier rejects valid program | LOW | Document, adjust program | Next sprint |
| PQC signature bypass | CRITICAL | Fix immediately | Immediate |

---

*S79 Lich Hardening Campaign — Forged 2026-03-25*
*6 Phases. 85 Steps. The Lich never sleeps. Neither does the Kingdom's defense.*
*"You can't harden what you haven't broken." — BlackMage*
