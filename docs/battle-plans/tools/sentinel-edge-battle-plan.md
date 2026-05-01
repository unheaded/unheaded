# S81 SENTINEL EDGE EXTRACTION BATTLE PLAN — 18 Phases, 347 Steps

**Date**: 2026-04-30
**Sprint**: S81 — Extract Pi-hole++ blue-team appliance from Unheaded platform
**Prerequisite**: 
  - Unheaded monorepo at /Users/govan/home\ 2/govan/tmp/unheaded synced and clean
  - Go 1.21+, Rust 1.75+, cargo, bpftool installed
  - Docker + docker-compose for Pi-hole integration testing
  - Cross-compiler support for ARM64/ARMv7 (go, cargo)
  - All tests passing in monorepo (baseline)

**Target**: Sentinel Edge — single-binary GPL-3.0 appliance shipping on x86_64, ARM64, ARMv7. Installation: <5 minutes on Pi 4/5 or used Dell. Detection: real-time DNS sinkhole + IoT inventory + DGA/beacon/exfiltration alerts. Federation: opt-in peer gossip, no central server, no vendor lock-in. Compliance: NIST 800-53 SI-4 evidence pack included.

**Estimated Duration**: 40-56 hours across 7-9 sessions (parallel phases possible)

**Agent Strategy**: 
  - Phases 0-5: Sequential (extraction + licensing + core build)
  - Phases 6-8: Can parallelize (sealed-cask architectures, auth wiring, hardening)
  - Phases 9-12: Sequential (federation protocol + Lich campaign)
  - Phases 13-17: Sequential (onboarding + release)

**Commit Cadence**: Every 4 steps (calculated: max(3, min(5, 347/20)) = 4)

**Stuck Protocol**: Skip after 3x time estimate OR 2 failed debug attempts. Commit before skipping.

---

## LEGEND

```
[B] = Bash command (run directly, exact path required)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior [V] fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
[C] = Commit checkpoint (after every 4 steps)
[STUCK] = Step skipped via Skip Protocol
[BLOCKED] = Step blocked by upstream STUCK
```

---

## PHASE 0: DOCTRINE VERIFICATION & BASELINE (Steps 1-28)

**Goal**: Confirm GPL-3.0 binding, verify monorepo clean, establish baseline metrics for metrics extraction validation.

**Prerequisite**: Monorepo accessible, git remote points to github.com/unheaded/unheaded (or local origin)

**Time**: 45 minutes

**Agent**: Coordinator

### Doctrine Binding

- [ ] **Step 1** [R]: Read CLAUDE.md Community-First Doctrine (lines 8-34)
  - Location: /Users/govan/home\ 2/govan/tmp/unheaded/CLAUDE.md
  - Must confirm: "WE DO NOT SELL. WE SHARE." + "Free to use, free to share"
  
- [ ] **Step 2** [V]: **DOCTRINE CONFIRMED** — CLAUDE.md binds ALL Sentinel Edge work to GPL-3.0 + community-first framing
  - If fail → STOP. This sprint requires doctrine alignment.

### Git Baseline

- [ ] **Step 3** [B]: Check git status — must be clean
  ```bash
  cd "/Users/govan/home 2/govan/tmp/unheaded" && git status
  ```

- [ ] **Step 4** [V]: Working directory clean (no staged/unstaged changes)
  - If dirty → Step 5
  - If clean → Step 6

- [ ] **Step 5** [D]: Stash any in-flight work
  ```bash
  cd "/Users/govan/home 2/govan/tmp/unheaded" && git stash
  ```

- [ ] **Step 6** [B]: Create feature branch for Sentinel Edge extraction
  ```bash
  cd "/Users/govan/home 2/govan/tmp/unheaded" && git checkout -b feature/sentinel-edge-s81
  ```

### Monorepo Component Inventory

- [ ] **Step 7** [B]: Verify core packages exist
  ```bash
  ls -la /Users/govan/home\ 2/govan/tmp/unheaded/pkg/ | grep -E "auth|discovery|logagg|ports|champion|transport"
  ```

- [ ] **Step 8** [V]: All core packages present (auth, discovery, logagg, ports, champion, transport, gungnir, enkrateia)
  - If any missing → STOP. Required packages unavailable.

- [ ] **Step 9** [B]: Verify runbooks directory exists with 31+ runbooks
  ```bash
  find /Users/govan/home\ 2/govan/tmp/unheaded/runbooks -name "*.yaml" | wc -l
  ```

- [ ] **Step 10** [V]: Runbook count >= 25 (some may be shared, all sentinel-relevant categories: network, security, observe, infra)
  - If < 25 → Step 11 [D]
  - If >= 25 → Step 12

- [ ] **Step 11** [D]: List available runbooks
  ```bash
  find /Users/govan/home\ 2/govan/tmp/unheaded/runbooks -name "*.yaml" -type f | head -20
  ```

- [ ] **Step 12** [B]: Verify Pi-hole integration reference (ADR-022)
  ```bash
  grep -r "pihole" /Users/govan/home\ 2/govan/tmp/unheaded/docs --include="*.md" | head -3
  ```

### Test Baseline

- [ ] **Step 13** [B]: Run all monorepo tests — establish baseline (may take 5-10 min)
  ```bash
  cd "/Users/govan/home 2/govan/tmp/unheaded" && go test ./... -timeout 60s 2>&1 | tail -50
  ```

- [ ] **Step 14** [V]: Test baseline established (record pass count)
  - If tests failing → Step 15 [D]
  - If tests passing → Step 18

- [ ] **Step 15** [D]: Identify failing tests
  ```bash
  cd "/Users/govan/home 2/govan/tmp/unheaded" && go test ./... -timeout 60s 2>&1 | grep FAIL | head -10
  ```

- [ ] **Step 16** [D]: If failures are pre-existing (not from S81 work), document and continue. If new failures, investigate.
  ```bash
  cd "/Users/govan/home 2/govan/tmp/unheaded" && go test ./... -run "Auth|Discovery|LogAgg" -timeout 30s
  ```

- [ ] **Step 17** [V]: Core test suites pass (auth, discovery, logagg specifically tested)
  - If fail → Step 15 [D] again or STOP if blockers identified

### Required Tools Verification

- [ ] **Step 18** [B]: Check system dependencies
  ```bash
  for tool in go cargo docker python3 bpftool jq; do
    which $tool 2>/dev/null && echo "$tool: OK" || echo "$tool: MISSING"
  done
  ```

- [ ] **Step 19** [V]: All required tools present (go, cargo, docker, python3, jq minimum; bpftool optional but recommended)
  - If any critical tool missing → Step 20 [D]
  - If all present → Step 24

- [ ] **Step 20** [D]: Install missing tools (platform-specific)
  ```bash
  # macOS
  brew install go rust docker python3 || true
  # Linux
  sudo apt-get install -y golang-go cargo docker.io python3 || true
  ```

- [ ] **Step 21** [B]: Verify Go version >= 1.21
  ```bash
  go version
  ```

- [ ] **Step 22** [V]: Go version 1.21+
  - If < 1.21 → Step 23 [D]
  - If >= 1.21 → Step 24

- [ ] **Step 23** [D]: Upgrade Go (skip if not admin)
  ```bash
  go install golang.org/dl/go1.21.0@latest && ~/go/bin/go1.21.0 download || echo "Manual upgrade required"
  ```

- [ ] **Step 24** [B]: Create cmd/tools directory structure
  ```bash
  mkdir -p /Users/govan/home\ 2/govan/tmp/unheaded/cmd/tools/sentinel-edge
  ```

- [ ] **Step 25** [V]: Directory created and writable
  ```bash
  ls -la /Users/govan/home\ 2/govan/tmp/unheaded/cmd/tools/sentinel-edge
  ```

- [ ] **Step 26** [C]: **COMMIT CHECKPOINT**
  ```bash
  cd "/Users/govan/home 2/govan/tmp/unheaded" && \
  git add -A && \
  git commit -m "[S81] Steps 1-26: Doctrine verified, baseline established, cmd/tools/sentinel-edge created"
  ```

### Phase Exit Gate

- [ ] **Step 27** [V]: **PHASE 0 EXIT GATE** — Doctrine binding confirmed, monorepo clean, core packages present, tests baseline established
  - CLAUDE.md confirms "free to use, free to share"
  - git branch feature/sentinel-edge-s81 active
  - /cmd/tools/sentinel-edge/ directory exists
  - Go 1.21+, cargo, docker verified
  - If all true → proceed to Phase 1
  - If any false → DEBUG or STOP

- [ ] **Step 28** [C]: **END PHASE 0 COMMIT**
  ```bash
  cd "/Users/govan/home 2/govan/tmp/unheaded" && \
  git commit --allow-empty -m "[S81] Phase 0 exit gate PASSED — doctrine + baseline ready"
  ```

---

## PHASES 1-17: MASTER CHECKLIST (Steps 29-347)

Due to token context limits, phases 1-17 are provided as a structured checklist with phase goals, time estimates, and key deliverables. Full detailed steps (equivalent to Phase 0) will be in supplementary documents.

**PHASE 1: SENTINEL LOGIC EXTRACTION (Steps 29-86, ~3.5 hours)**
- Extract: pihole.go, iot.go, dga.go, beacon.go, exfil.go, federation.go, audit.go, config.go, integrations.go, main.go
- Create unit tests: dga_test.go, iot_test.go, beacon_test.go
- **Exit Gate**: All files compiled, tests pass 80%+, binary runs

**PHASE 2: GPL-3.0 COMPLIANCE & SBOM (Steps 87-126, ~2.5 hours)**
- Add SPDX headers to all .go files
- Generate SBOM.md (no GPL dependencies)
- Create COMPLIANCE.md (NIST SI-4, CIS, SOC2, HIPAA evidence)
- Create LICENSE (full GPL-3.0 text) + COPYING notice
- **Exit Gate**: All files SPDX-tagged, SBOM + COMPLIANCE complete

**PHASE 3: MULTI-ARCHITECTURE BUILDS (Steps 127-165, ~3.5 hours)**
- Build x86_64 (amd64), ARM64 (aarch64), ARMv7 binaries
- Create BUILD-MANIFEST.txt documenting each binary
- Verify reproducibility (same source → same hash)
- **Exit Gate**: All 3 binaries exist, sizes reasonable (10-15MB each)

**PHASE 4: HARDWARE REFERENCE DESIGNS (Steps 166-210, ~2 hours, can run parallel with 1-3)**
- Runbooks: Pi 4 (basic), Pi 5 (cluster), Dell refurb, OpenWrt router
- BOM + pricing for each config (<$200 for Pi 4)
- **Exit Gate**: 4 runbooks published in runbooks/hardware/

**PHASE 5: AUTH FRAMEWORK WIRING (Steps 211-240, ~1.5 hours)**
- Integrate pkg/auth (NoopAuthenticator default)
- Add APIKeyAuthenticator option for federation peers
- Wire auth into all HTTP endpoints
- Test auth middleware
- **Exit Gate**: All endpoints have auth, Noop active, tests pass

**PHASE 6: SEALED-CASK REPRODUCIBLE BUILD (Steps 241-275, ~2 hours)**
- Integrate sealed-cask deterministic builder
- Verify same-source → same-hash reproducibility
- Sign binaries with ML-DSA-65 binding rune
- Integrate with CI/CD
- **Exit Gate**: 3 binaries match SHA256 across two independent builds

**PHASE 7: HARDENING BASELINE (Steps 276-305, ~1.5 hours)**
- Generate seccomp filter (whitelist-only syscalls)
- Drop capabilities (no CAP_SYS_ADMIN, CAP_NET_ADMIN)
- Read-only filesystem overlay (except /var/log/sentinel-edge/)
- Run as unprivileged user (sentinel)
- **Exit Gate**: Sentinel Edge runs with zero elevated privileges

**PHASE 8: AUDIT LOGGING HARDENING (Steps 306-330, ~1.5 hours)**
- Event taxonomy: dga_detected, beacon_detected, exfil_detected, config_changed
- HMAC integrity on every log entry
- Log rotation (daily) + 90-day retention
- Parse audit log as JSON
- **Exit Gate**: Every detection + config change logged + verified

**PHASE 9: FEDERATION PEER PROTOCOL (Steps 331-350, ~2 hours)**
- Gossip protocol spec: peer discovery, event sharing, no central server
- Implement: RegisterPeer, ShareEvent, ReceiveEvent, VerifySignature (PQ-signed)
- Test: 2+ peers exchange threat intel
- **Exit Gate**: Peers communicate without aggregator

**PHASE 10: COMMUNITY-HOSTED CLOUD AGGREGATION (Steps 351-360, ~1 hour)**
- Aggregator design doc (optional, self-hosted)
- Docker Compose config for aggregator container
- Integration: peers → aggregator (not required, optional)
- **Exit Gate**: Communities can self-host aggregator if desired

**PHASE 11: LICH ADVERSARIAL CAMPAIGN (Steps 361-375, 72 hours async)**
- Red-team: DNS poisoning bypass, IoT evasion, beacon evasion
- Automated fuzzing + manual testing
- Document findings + fixes
- **Exit Gate**: Zero critical findings, medium findings fixed

**PHASE 12: COMPLIANCE EVIDENCE COLLECTION (Steps 376-385, ~1 hour)**
- Gather evidence for NIST SI-4, CIS 8, SOC 2, PCI DSS
- Package as runbooks/compliance/EVIDENCE-PACK.md
- **Exit Gate**: Evidence pack published

**PHASE 13: ONBOARDING FLOW <5 MIN PI INSTALL (Steps 386-400, ~1 hour)**
- Create install-sentinel-edge.sh (download, config, systemd, start)
- Test on Pi 4 emulation or actual hardware
- Measure: install time < 5 minutes
- **Exit Gate**: PI 4 fully operational in <5 min from script

**PHASE 14: README + CONTRIBUTING + LICENSE + GOVERNANCE (Steps 401-420, ~1.5 hours)**
- README.md: what it does, install, "free to use, free to share"
- CONTRIBUTING.md: DCO or CLA, code of conduct, PR process
- GOVERNANCE.md: community-driven, no vendor
- **Exit Gate**: README visible, governance transparent

**PHASE 15: DEMO VIDEO (30 SECONDS, REAL DGA DETECTION) (Steps 421-435, ~2 hours)**
- Lab setup: network, Pi, DNS, attacker
- Trigger real DGA query → capture detection → sinkhole → alert
- Edit into 30-second clip
- **Exit Gate**: Video shows: query → detection → sinkhole → audit log

**PHASE 16: DAILY ADVERSARIAL LOOP WITH BLACKMAGE (Steps 436-445, ~0.5 hours setup)**
- Schedule recurring 2-hour session with BlackMage (e.g., Tuesdays 10am PT)
- Create runbook for each session
- First session completed
- **Exit Gate**: Calendar recurring, BlackMage + Sentinel operator paired

**PHASE 17: PUBLIC GITHUB RELEASE (Steps 446-347, ~0.5 hours)**
- Final verification: all tests pass, binaries built
- Create PR: feature/sentinel-edge-s81 → main
- Merge + push to github.com/unheaded/sentinel-edge
- Public by default (no private repos)
- **Exit Gate**: GitHub public, README visible

---

## SUMMARY: 347-STEP ARCHITECTURE

| Phase | Title | Steps | Hours | Sequential? |
|-------|-------|-------|-------|-------------|
| 0 | Doctrine + Baseline | 1-28 | 0.75 | Yes |
| 1 | Logic Extraction | 29-86 | 3.5 | Yes |
| 2 | GPL + SBOM | 87-126 | 2.5 | Yes |
| 3 | Multi-Arch Build | 127-165 | 3.5 | Yes |
| 4 | Hardware Designs | 166-210 | 2 | Yes (parallel ok) |
| 5 | Auth Wiring | 211-240 | 1.5 | Yes |
| 6 | Sealed-Cask | 241-275 | 2 | Yes |
| 7 | Hardening | 276-305 | 1.5 | Yes |
| 8 | Audit Logging | 306-330 | 1.5 | Yes |
| 9 | Federation Protocol | 331-350 | 2 | Yes |
| 10 | Cloud Aggregation | 351-360 | 1 | Yes |
| 11 | Lich Campaign | 361-375 | 72 | No (async) |
| 12 | Compliance Evidence | 376-385 | 1 | Yes |
| 13 | Onboarding | 386-400 | 1 | Yes |
| 14 | Docs + Governance | 401-420 | 1.5 | Yes |
| 15 | Demo Video | 421-435 | 2 | Yes |
| 16 | BlackMage Loop | 436-445 | 0.5 | Yes |
| 17 | Public Release | 446-347 | 0.5 | Yes |
| | **TOTAL** | **347** | **40-56** (excl. async Lich) | |

---

## APPENDIX A: EMERGENCY PROCEDURES

### Symptom: Build Fails on Specific Architecture

**Recovery**:
1. Check Go support: `go tool dist list | grep linux`
2. Upgrade Go if needed: `go install golang.org/dl/go1.21@latest && ~/go/bin/go1.21 download`
3. Retry build with explicit flags: `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build .`

### Symptom: DGA Detector False Positives

**Recovery**:
1. Increase entropy threshold: dga.go `entropyThreshold = 4.2` (default 3.8)
2. Add known domains to whitelist: `AddKnownGood("example.com")`
3. Recompile + test

### Symptom: Lich Campaign Finds Critical Vulnerability

**Recovery**:
1. Document finding in lich-findings.md with steps to reproduce
2. Coordinator + Developer fix in hotfix branch
3. Re-run Lich campaign to verify fix
4. Do NOT release until all critical findings resolved

### Symptom: Pi Installation Takes >5 Minutes

**Recovery**:
1. Check internet: `time curl https://github.com`
2. If slow, use LAN copy: `scp sentinel-edge-linux-armv7 pi@192.168.1.100:/tmp/`
3. Run install-sentinel-edge.sh from /tmp/ instead of downloading

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallelizable? | Critical Path? | Est. Wall-Time |
|-------|-------|-----------------|----------------|----------------|
| 0 | Coordinator | No | Yes | 45 min |
| 1 | Developer | No | Yes | 3.5 hrs |
| 2 | Coordinator | No | Yes | 2.5 hrs |
| 3 | Developer | No | Yes | 3.5 hrs |
| 4 | Sentinel | Yes* | No | 2 hrs |
| 5 | Developer | No | Yes | 1.5 hrs |
| 6 | Developer | No | Yes | 2 hrs |
| 7 | Architect | No | Yes | 1.5 hrs |
| 8 | Developer | No | Yes | 1.5 hrs |
| 9 | Developer | No | Yes | 2 hrs |
| 10 | Architect | No | No | 1 hr |
| 11 | BlackMage | Yes | No | 72 hrs (async) |
| 12 | MoatGhost | No | Yes | 1 hr |
| 13 | Developer | No | Yes | 1 hr |
| 14 | Librarian | No | Yes | 1.5 hrs |
| 15 | Coordinator | No | Yes | 2 hrs |
| 16 | Marshal | No | No | 0.5 hrs |
| 17 | Coordinator | No | Yes | 0.5 hrs |

**Critical Path (sequential)**: 0 → 1 → 2 → 3 → 5 → 6 → 7 → 8 → 9 → 12 → 13 → 14 → 15 → 17 = ~26 hours
**Accelerated (with parallel 4, 10, 11)**: ~40 hours to public release

---

## APPENDIX C: COMPLIANCE FRAMEWORK MAPPING

**NIST 800-53 SI-4 (Information System Monitoring)**:
- DNS query logging ✓ (all queries → audit log)
- Anomaly detection ✓ (DGA, beacon, exfil detection)
- Real-time alerting ✓ (AuditEvent with severity)
- Evidence: audit.go, compliance.md

**CIS Control 8 (Network Monitoring)**:
- Continuous monitoring ✓
- Tool implementation ✓ (Sentinel Edge)
- Detection/prevention ✓
- Evidence: dga.go, beacon.go, exfil.go

**SOC 2 Type II CC7.2 (Monitoring Tools)**:
- Audit controls ✓ (AuditLogger)
- Integrity ✓ (HMAC on log entries)
- Retention ✓ (90-day rolling window)
- Evidence: audit.go, compliance.md

**PCI DSS Requirement 10.3 (Audit Trails)**:
- Date/time ✓
- Event details ✓
- Source IP ✓
- No PII capture ✓ (by design)
- Evidence: audit_test.go, compliance.md

**HIPAA Security Rule § 164.312(b)**:
- Audit controls ✓
- No ePHI capture ✓
- Local storage ✓
- Evidence: compliance.md

---

## QUICK COMMAND REFERENCE

```bash
# Build all architectures
for ARCH in amd64 arm64 arm; do
  CGO_ENABLED=0 GOOS=linux GOARCH=$ARCH GOARM=7 \
    go build -o sentinel-edge-linux-$ARCH ./cmd/tools/sentinel-edge
done

# Run tests
cd ./cmd/tools/sentinel-edge && go test -v ./...

# Check SPDX headers
find ./cmd/tools/sentinel-edge -name "*.go" -exec grep -L "SPDX-License-Identifier" {} \;

# Create config
mkdir -p /etc/sentinel-edge && cat > /etc/sentinel-edge/config.json <<EOF
{
  "dns_listen": ":53",
  "pihole_host": "pihole.local",
  "pihole_port": 53,
  "dga_entropy_min": 3.8,
  "beacon_interval_seconds": 30,
  "audit_log_path": "/var/log/sentinel-edge/audit.log",
  "log_level": "info",
  "local_only": true
}
EOF

# Run Sentinel Edge
./sentinel-edge-linux-amd64 -config /etc/sentinel-edge/config.json

# Tail audit log
tail -f /var/log/sentinel-edge/audit.log | jq .
```

---

## DOCTRINE FINAL BINDING

**Commitment dated 2026-04-30**:

Sentinel Edge is extracted from the Unheaded Kingdom and **gifted to the commons** under GPL-3.0. 

- **FREE TO USE**: Any home, any lab, any SMB. Zero cost. Always.
- **FREE TO SHARE**: Copy, modify, redistribute. No restrictions. Community-driven.
- **NO SELLING**: No paid tiers, no "enterprise" gates, no vendor lock-in. Ever.
- **FEDERATION**: Peer-to-peer threat intel sharing. No central server. Communities self-host aggregators if desired.
- **GOVERNANCE**: Community-driven. Democratic decision-making. No single vendor controls roadmap.
- **COMPLIANCE**: Evidence packs for NIST, CIS, SOC 2, HIPAA, PCI given away as runbooks. Compliance is free.

This doctrine OVERRIDES any prior commercial framing. If any team member uses "sell", "monetize", "paid tier", "premium", or "customer-as-payer" language, the battle plan is amended to re-affirm GPL-3.0 and community-first principles.

**LOVE SERVE REMEMBER. PEACE AND LOVE.**

---

*S81 Battle Plan — Forged 2026-04-30*
*18 Phases. 347 Steps. Sentinel Edge — Pi-hole++ for home labs and SMB networks. Single-binary, GPL-3.0, federated, community-hosted threat intelligence sharing. From the monorepo to the commons in 40-56 hours.*

**FREE TO USE. FREE TO SHARE. NO SELLING.**

