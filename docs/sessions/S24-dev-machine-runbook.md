# S24 DEV MACHINE RUNBOOK — VERIFICATION & EXECUTION

**Date**: 2026-02-20
**Context**: S24 wrote ~6K lines across 8 phases using multi-agent swarm in Cowork VM.
**Problem**: Cowork VM has NO Go toolchain, NO Rust toolchain, NO network access.
**Solution**: This runbook provides step-by-step verification for a machine with toolchains.

---

## PREREQUISITES

```bash
# Verify toolchains
go version        # Need 1.21+ (project uses 1.24.0)
rustup show       # Need nightly for cargo-fuzz
cargo --version   # Need for eBPF compilation
```

If missing:
```bash
# Go
curl -sSL https://go.dev/dl/go1.24.0.linux-amd64.tar.gz | sudo tar -C /usr/local -xzf -
export PATH=$PATH:/usr/local/go/bin

# Rust nightly (for fuzzing)
rustup install nightly
rustup component add rust-src --toolchain nightly
cargo install cargo-fuzz
```

---

## PHASE 1: CRITICAL COMPILATION VERIFICATION

**Priority: P0 — MUST PASS before anything else**

These tests verify that all S23 refactoring (encoding unification, registry refactor, bpfschema parity fixes) AND all S24 additions (Flow Tracker maps, Hop XDP integration, MapLoader integration tests) compile and pass.

### Step 1.1: Full Go Build

```bash
cd ~/unheaded  # or wherever your repo lives
git pull origin main

# Verify S24 commits landed
git log --oneline -8
# Should show:
# 2a6aa08 docs(session): add S24 handoff
# c309c5f docs(timeline): update with S24 sprint metrics
# 272dc0a ci(hardening): production hardening
# 832b704 security(fuzz): generate LICH seed corpora
# cca26b7 test(ebpf): add MapLoader integration tests
# 3b1ec3a feat(protocol): close RFC compliance gaps
# 71c27f9 feat(ebpf): complete Hop XDP map integration
# 75ae2f3 feat(ebpf): wire Flow Tracker BPF maps

# Build all Go packages
go build ./...
echo "Exit code: $?"
# EXPECTED: 0 (clean build)
# If non-zero: fix compilation errors BEFORE proceeding
```

### Step 1.2: Protocol Package Tests (THE CORE)

```bash
# S23 refactored encoding — verify it didn't break anything
go test -v -race ./pkg/protocol/encoding/...
# EXPECT: CRC-16 test vector 0x29B1, varint roundtrips, TLV encode/decode

go test -v -race ./pkg/protocol/registry/...
# EXPECT: Generic registry, freeze semantics, range policy

go test -v -race ./pkg/protocol/errors/...
# EXPECT: Registry refactor, duplicate prevention, freeze enforcement

go test -v -race ./pkg/protocol/settings/...
# EXPECT: Settings encoding uses shared encoding package

go test -v -race ./pkg/protocol/tlv/...
# EXPECT: TLV encoding uses shared encoding package

go test -v -race ./pkg/protocol/integrity/...
# EXPECT: HMAC encoding uses shared encoding package
```

### Step 1.3: BPF Schema Parity Tests

```bash
# S23 fixed 3 critical bugs (FlowKey 40B→16B, FlowState +TraceID, MbcCpuState 80B)
# S24 added FlowMigrationTokenValue (48B) and FlowCancelValue (24B)
go test -v -race ./pkg/protocol/bpfschema/...

# EXPECT ALL PASS:
# TestMonadRegister (20B)
# TestAnamnesisEvent (32B)
# TestFlowKey (16B — was 40B before S23 fix)
# TestFlowState (56B — has TraceID now)
# TestMbcCpuState (80B)
# TestFlowMigrationTokenValue (48B — NEW in S24)
# TestFlowCancelValue (24B — NEW in S24)
```

### Step 1.4: MapLoader Tests

```bash
# S23 created maploader (33 tests)
# S24 added integration tests (7 tests + 1 benchmark)
go test -v -race ./pkg/ebpf/maploader/...
# EXPECT: 33+ tests pass, 100% coverage on Load* functions

# Integration tests (build-tagged, requires explicit opt-in)
go test -v -race -tags integration ./pkg/ebpf/maploader/...
# EXPECT: 40+ tests pass (33 unit + 7 integration)

# Benchmark (optional but recommended)
go test -tags integration -bench BenchmarkIntegration -benchmem ./pkg/ebpf/maploader/...
# EXPECT: batch mode significantly faster than individual
```

### Step 1.5: Full Protocol Suite

```bash
# The big one — ALL protocol packages at once
go test -v -race ./pkg/protocol/...
# EXPECT: 200+ tests pass, 0 failures, 0 timeouts

# Count tests
go test -v ./pkg/protocol/... 2>&1 | grep -c "^--- PASS"
# EXPECT: >= 200
```

### Step 1.6: Full Repository Test

```bash
# Everything
go test -v -race ./...
# Record output for handoff
go test -v -race ./... 2>&1 | tee /tmp/s24-test-results.txt

# Count results
grep -c "^--- PASS" /tmp/s24-test-results.txt
grep -c "^--- FAIL" /tmp/s24-test-results.txt
# EXPECT: 0 FAIL
```

**IF ANY TEST FAILS**: Stop. Fix it. Commit with message:
```
fix(protocol): resolve [description] from S24 code sprint

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

---

## PHASE 2: RUST eBPF COMPILATION VERIFICATION

### Step 2.1: Shield eBPF (S24 added Destination Options parsing)

```bash
cd ebpf/shield-ebpf
cargo check 2>&1
# EXPECT: no errors
# S24 CHANGES: Added process_destination_options() TLV parsing per RFC 8200 §4.6
# Verify: new option parsing loop, unknown option skip logic, stat counters

cargo clippy -- -D warnings 2>&1
# Fix any warnings
```

### Step 2.2: Hop eBPF (S24 completed 4 map integrations)

```bash
cd ebpf/hop-ebpf
cargo check 2>&1
# EXPECT: no errors
# S24 CHANGES: Enhanced seq_counters (gap/reorder detection), settings enforcement
#              (capability-required drop), DoS backpressure (ECN/shutdown), flow
#              type dispatch (control/data/bulk with QoS adjustment)
# Verify: 7 new STAT_* counters, RFC comments above each map lookup

cargo clippy -- -D warnings 2>&1
```

### Step 2.3: Flow Tracker (S24 added 2 new maps)

```bash
cd ebpf/flow-tracker
cargo check 2>&1
# EXPECT: no errors
# S24 CHANGES: Added MIGRATION_TOKENS HashMap and CANCEL_FLOWS HashMap
#              Migration: token lookup after flow creation, expiry check via ktime_get_ns()
#              Cancel: flag check before state mutation, ANOMALY event on cancel,
#              TCP RST for TCP flows, stat counters for both

cargo clippy -- -D warnings 2>&1
```

### Step 2.4: All Other eBPF Programs

```bash
# Verify nothing else broke
for dir in ebpf/*/; do
    if [ -f "$dir/Cargo.toml" ]; then
        echo "=== Checking $dir ==="
        (cd "$dir" && cargo check 2>&1)
        echo "Exit: $?"
    fi
done
# EXPECT: All exit 0
```

### Step 2.5: Full Cargo Test (if tests exist)

```bash
for dir in ebpf/*/; do
    if [ -f "$dir/Cargo.toml" ]; then
        echo "=== Testing $dir ==="
        (cd "$dir" && cargo test 2>&1)
    fi
done
```

**IF ANY RUST FAILS**: Common issues:
- Missing `use` import → add it
- Type mismatch in new map declarations → check bpfschema parity
- BPF verifier limit → check loop bounds (MAX_EXT_HDRS_TO_STRIP=8)

---

## PHASE 3: LICH FUZZING CAMPAIGN EXECUTION

**Reference**: `docs/security/lich-execution.md` for full details

### Step 3.1: Go Native Fuzzing (Quick Win — 5 minutes)

```bash
cd pkg/protocol/fuzz

# Varint roundtrip (30 seconds)
go test -fuzz=FuzzVarintRoundtrip -fuzztime=30s
# EXPECT: no crashes, corpus growth

# Exponent roundtrip (30 seconds)
go test -fuzz=FuzzExponentRoundtrip -fuzztime=30s
# EXPECT: no crashes, overflow checks hold

# CRC invariant (30 seconds)
go test -fuzz=FuzzCRCInvariant -fuzztime=30s
# EXPECT: determinism confirmed, no collisions in test vectors

# TLV roundtrip (30 seconds)
go test -fuzz=FuzzTLVRoundtrip -fuzztime=30s
# EXPECT: malformed TLVs rejected, valid roundtrips preserved
```

### Step 3.2: Rust LICH Campaigns (Longer — hours)

```bash
# Verify seed corpus was generated
ls -la ebpf/fuzz/seeds/lich_007_mbc/    # 20 seeds
ls -la ebpf/fuzz/seeds/lich_008_wotan_cache/  # 20 seeds
ls -la ebpf/fuzz/seeds/lich_009_flow_collision/  # 50 seeds
ls -la ebpf/fuzz/seeds/lich_010_wal_integrity/  # 30 seeds

# If seeds are missing, regenerate:
python3 ebpf/fuzz/generate_seeds.py

# Run campaigns (adjust -runs and -max_total_time as needed)
cd ebpf/fuzz

# LICH-007: MBC Bytecode (target: 30 min)
cargo +nightly fuzz run lich_007_mbc -- \
    -max_total_time=1800 \
    -seed_inputs=seeds/lich_007_mbc/

# LICH-008: Wotan Cache Races (target: 30 min)
cargo +nightly fuzz run lich_008_wotan_cache -- \
    -max_total_time=1800 \
    -seed_inputs=seeds/lich_008_wotan_cache/

# LICH-009: Flow Collision (target: 1 hour)
cargo +nightly fuzz run lich_009_flow_collision -- \
    -max_total_time=3600 \
    -seed_inputs=seeds/lich_009_flow_collision/

# LICH-010: WAL Integrity (target: 30 min)
cargo +nightly fuzz run lich_010_wal_integrity -- \
    -max_total_time=1800 \
    -seed_inputs=seeds/lich_010_wal_integrity/
```

### Step 3.3: Record Results

Fill in `docs/security/lich-results-S24.md` with actual results:
- Corpus size after fuzzing
- Number of crashes (hopefully 0)
- Coverage metrics
- Any new edge cases discovered

---

## PHASE 4: PRODUCTION HARDENING VERIFICATION

### Step 4.1: Lint Check

```bash
# Install if needed
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run against full codebase
golangci-lint run ./...
# EXPECT: warnings only, no errors (S24 added .golangci.yml config)
# Fix any issues, commit: "fix(lint): resolve golangci-lint warnings"
```

### Step 4.2: Security Scanner

```bash
# Install gosec
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Scan
gosec ./...
# EXPECT: no HIGH severity issues
# Document findings in docs/security/S24-security-roadmap.md
```

### Step 4.3: Dependency Audit

```bash
# Clean up
go mod tidy
go mod verify
# EXPECT: "all modules verified"

# Vulnerability check
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
# Document any findings
```

### Step 4.4: SBOM Generation

```bash
# Install syft
curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin

# Generate SBOM
syft . -o json > SBOM.json
# Commit: "ci(sbom): generate software bill of materials"
```

### Step 4.5: Nix Flake Verification

```bash
# S24 added flake.nix — verify it works
nix flake check  # If nix available
nix flake lock    # Lock dependencies
# Commit lockfile if generated
```

### Step 4.6: Build Script Test

```bash
# S24 added scripts/build.sh
chmod +x scripts/build.sh
./scripts/build.sh
# EXPECT: binaries in bin/ with version metadata
# Verify: ./bin/unheaded-daemon --version (should show git hash + date)
```

---

## PHASE 5: DOCUMENTATION & README UPDATE

### Step 5.1: Update README.md

Update with current metrics:
- Total LOC: ~508,000+
- eBPF programs: 8 with full map integration (0 gaps)
- Protocol specs: Monad-04, Sophia-01, Wotan-01
- MapLoader: 3,044 lines, 40 tests (33 unit + 7 integration)
- Fuzzing: 8 harnesses + 120 seed corpus files
- ADRs: 14 (ADR-001 through ADR-014)
- Go version: 1.24.0
- Services: 25 active across 4 tiers

### Step 5.2: Verify ADR Count

```bash
ls docs/adr/ADR-*.md | wc -l
# EXPECT: 14
# New in S24: ADR-013 (Routing Header), ADR-014 (Fragment Header)
```

### Step 5.3: Verify Session Docs

```bash
ls docs/sessions/ | wc -l
# Should show S1 through S24 handoffs
```

---

## PHASE 6: AGE 2 PREPARATION (OPTIONAL — STRETCH GOAL)

If all verification passes, the next agent can begin Age 2: Beta Trials prep:

### 6.1: Multi-Tenant Isolation Design
- Document tenant boundary architecture
- Design namespace isolation for BPF maps
- Plan per-tenant Sophia dictionaries

### 6.2: Performance Baseline
```bash
# Run benchmarks and record baseline
go test -bench=. -benchmem ./pkg/protocol/... > benchmarks-baseline.txt
go test -bench=. -benchmem ./pkg/ebpf/maploader/... >> benchmarks-baseline.txt
```

### 6.3: CI/CD Pipeline
- GitHub Actions workflow for:
  - `go test -race ./...` on every PR
  - `cargo check` for all eBPF programs
  - `golangci-lint` enforcement
  - SBOM generation on release

---

## COMMIT CHECKLIST FOR NEXT AGENT

After running all verification steps, commit results:

```bash
# If fixes were needed:
git add -A
git commit -m "$(cat <<'EOF'
fix(s24-verification): resolve compilation issues found during dev machine testing

[describe what was fixed]

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"

# Test results:
git add /tmp/s24-test-results.txt docs/security/lich-results-S24.md
git commit -m "$(cat <<'EOF'
tests(s24): record dev machine verification results — all tests passing

Go: X/X tests pass, 0 failures
Rust: all eBPF programs compile clean
Fuzzing: X campaigns executed, 0 crashes
Lint: clean
Security: no HIGH severity issues

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## PRIORITY ORDER (if time-limited)

If you can only do some of these, this is the order:

1. **`go build ./...`** — does it compile? (2 minutes)
2. **`go test -v -race ./pkg/protocol/bpfschema/...`** — parity tests (1 minute)
3. **`go test -v -race ./pkg/protocol/...`** — protocol suite (5 minutes)
4. **`cargo check` in ebpf/flow-tracker + ebpf/hop-ebpf** — new Rust code (3 minutes)
5. **Go fuzz targets** — 30s each, 4 targets (2 minutes)
6. **`go test -v -race ./...`** — everything (10 minutes)
7. **golangci-lint + gosec** — production readiness (5 minutes)
8. **LICH campaigns** — security depth (2+ hours)

**Total minimum verification: ~30 minutes for steps 1-7**

---

**THE KINGDOM AWAITS VERIFICATION. THE CODE IS WRITTEN. THE TESTS NEED A MACHINE.**

*S24 Dev Machine Runbook — February 20, 2026*

