# LICH Campaign Execution Guide

**Phase 6 Deliverable: Campaign Execution Instructions**

This document provides complete instructions for executing the LICH (Long-Interval Computational Hazards) fuzzing campaigns, including setup, running procedures, resource requirements, and result interpretation.

## Campaign Overview

| Campaign | Name | Target | Harness Language | Corpus Size | Expected Duration | CPU/Memory |
|----------|------|--------|------------------|-------------|------------------|-----------|
| LICH-007 | MBC Bytecode | Doom substrate MBC decoder | Rust (libfuzzer) | 20 seeds | 48 hours | 4-8 cores, 4GB |
| LICH-008 | Wotan Cache | L1 cache race conditions | Rust (libfuzzer) | 20 seeds | 72 hours | 8-16 cores, 8GB |
| LICH-009 | Flow Collision | IPv4 5-tuple birthday attack | Rust (libfuzzer) | 50 seeds | 24 hours | 4-8 cores, 4GB |
| LICH-010 | WAL Integrity | Write-ahead log compaction races | Rust (libfuzzer) | 30 seeds | 48 hours | 4-8 cores, 4GB |

**Total Campaign Duration:** ~190 hours of continuous fuzzing (~8 days on a 4-core machine, 2 days on a 16-core machine)

---

## Prerequisites

### Rust Fuzzing (LICH-007 through LICH-010)

```bash
# Install nightly Rust toolchain
rustup toolchain install nightly

# Install cargo-fuzz
cargo +nightly install cargo-fuzz

# Verify installation
cargo +nightly fuzz --version
```

### Go Fuzzing (Protocol Encoding Tests)

```bash
# Verify Go 1.18+ installed
go version

# Go fuzzing is built-in to Go 1.18+
# No additional installation required
```

### System Requirements

- **LICH-007 (MBC):** 4-8 CPU cores, 4GB RAM
- **LICH-008 (Cache):** 8-16 CPU cores, 8GB RAM (ThreadSanitizer enabled)
- **LICH-009 (Flow Collision):** 4-8 CPU cores, 4GB RAM
- **LICH-010 (WAL):** 4-8 CPU cores, 4GB RAM

**Total parallel resources:** If running all campaigns simultaneously, allocate 32-48 cores and 20GB+ RAM.

---

## LICH-007: MBC Bytecode Instruction Fuzzer

### Objective
Identify bytecode interpretation errors, instruction encoding flaws, and correctness violations in the Doom substrate's MBC (Minimal Bytecode Compiler) implementation.

### Target
- MBC instruction decoder with random bytecode sequences
- Located at: `crates/monad-mbc/src/lib.rs` (MBC decoder)

### Running the Fuzzer

#### Single-threaded (baseline)
```bash
cd /path/to/unheaded
cargo +nightly fuzz run lich_007_mbc -- -max_len=1024
```

#### Multi-threaded with corpus
```bash
cargo +nightly fuzz run lich_007_mbc \
  -j 8 \
  ebpf/fuzz/seeds/lich_007_mbc \
  -max_len=1024 \
  -max_total_time=172800  # 48 hours in seconds
```

#### With coverage tracking
```bash
LLVM_PROFILE_FILE="coverage-%p.profraw" \
  cargo +nightly fuzz run lich_007_mbc \
  -j 4 \
  ebpf/fuzz/seeds/lich_007_mbc \
  -max_total_time=172800
```

#### With verbose output
```bash
cargo +nightly fuzz run lich_007_mbc \
  -j 4 \
  ebpf/fuzz/seeds/lich_007_mbc \
  -v \
  -max_total_time=172800
```

### Seed Corpus

Location: `ebpf/fuzz/seeds/lich_007_mbc/` (20 seeds, 160 bytes total)

Seed breakdown:
- `seed_000-005`: Single instruction types (NOP, LOAD_IMM, STORE, ADD, JMP, JEQ)
- `seed_006-012`: Multi-instruction sequences (2-3 instructions)
- `seed_013`: Complex 4-instruction sequence (LOAD + ADD + STORE + RET)
- `seed_014-016`: Boundary conditions (jump overflow, negative jumps, max register)
- `seed_017-019`: Stress patterns (all registers, read/write alternation, 16-instruction sequence)

### Oracle Verification

The fuzzer checks for violations:
1. **No panics or crashes** during instruction decoding
2. **Bounds checking** enforced on:
   - Jump offsets (must stay within code segment [0, len) )
   - Register numbers (must be 0-15)
   - Memory addresses (checked against segment bounds)
3. **Integer overflow** detected and handled gracefully (error codes, no silent wrapping)
4. **Instruction truncation** handled (incomplete opcodes detected)

### Expected Findings

- Instruction decoding off-by-one errors
- Register width mismatches
- Stack underflow/overflow
- Incorrect memory protection
- Branch offset calculation errors
- Integer overflow violations

### Success Criteria (S21 Assessment)

- ≥2 new coverage paths discovered per hour (decreasing trend)
- ≥1 crash or assertion failure per 8 hours
- All crashes reproducible and categorized by severity
- Root cause identified for crash triggers

### Monitoring

```bash
# Watch for crashes in real-time
ls -lh fuzz/artifacts/lich_007_mbc/ | tail -20

# Check coverage summary
cargo +nightly fuzz cov lich_007_mbc 2>/dev/null || echo "Coverage merge not available"

# Merge coverage from multiple runs
llvm-profdata merge -o coverage.profdata coverage-*.profraw
```

---

## LICH-008: Wotan L1 Cache Race Condition Harness

### Objective
Expose concurrency bugs in Wotan L1 memory-mapped cache under concurrent access patterns, focusing on TOCTOU (time-of-check-time-of-use) violations and memory ordering issues.

### Target
- Concurrent cache access with flow label keys and memory barriers
- Located at: `cmd/dashboard-backend/internal/ebpf/wotan_cache.rs` (Wotan helpers)

### Running the Fuzzer

#### Single-threaded baseline
```bash
cd /path/to/unheaded
cargo +nightly fuzz run lich_008_wotan_cache -- -max_len=2048
```

#### Multi-threaded with ThreadSanitizer
```bash
# ThreadSanitizer requires LLVM instrumentation
# Set environment variables for race detection
RUSTFLAGS="-Zsanitizer=thread" \
TSAN_OPTIONS="halt_on_error=1,track_origins=1" \
  cargo +nightly fuzz run lich_008_wotan_cache \
  -j 16 \
  ebpf/fuzz/seeds/lich_008_wotan_cache \
  -max_len=2048 \
  -max_total_time=259200  # 72 hours
```

#### Without ThreadSanitizer (faster)
```bash
cargo +nightly fuzz run lich_008_wotan_cache \
  -j 8 \
  ebpf/fuzz/seeds/lich_008_wotan_cache \
  -max_len=2048 \
  -max_total_time=259200
```

#### With memory barrier coverage tracking
```bash
cargo +nightly fuzz run lich_008_wotan_cache \
  -j 12 \
  ebpf/fuzz/seeds/lich_008_wotan_cache \
  -artifact_prefix=wotan_races/ \
  -max_total_time=259200
```

### Seed Corpus

Location: `ebpf/fuzz/seeds/lich_008_wotan_cache/` (20 seeds, variable size)

Seed breakdown:
- `seed_000-002`: Single operations (read, write, CAS)
- `seed_003-005`: Sequential and mixed patterns
- `seed_006-008`: High-contention and CAS-heavy workloads
- `seed_009-012`: Address boundary and collision patterns
- `seed_013-016`: Longer sequences and cache warming
- `seed_017-019`: Sparse access and burst traffic patterns

### Oracle Verification

The fuzzer checks for violations:
1. **Load-store reordering** detected via generation counter mismatches
2. **Store buffering** verified: all stores visible to subsequent loads
3. **Memory barrier correctness** checked (wmb, rmb, mb semantics)
4. **CAS atomicity** verified: Compare-and-Swap never leaves intermediate states
5. **Cache coherency** maintained across flows

### Expected Findings

- Load-store reordering violations
- Store buffering failures
- Lost updates due to missing memory barriers
- CAS atomicity gaps
- Cache coherency violations from collisions

### Success Criteria (S21 Assessment)

- ≥3 race conditions confirmed by ThreadSanitizer
- Happens-before violations documented
- Atomic operation misuse identified
- All races reproducible with deterministic scheduling
- Patches reduce race detector warnings by ≥95%

### Monitoring with ThreadSanitizer

```bash
# Extract race conditions from TSAN output
grep -E "WARNING: ThreadSanitizer|Race on" fuzz/artifacts/lich_008_wotan_cache/*

# Analyze specific race condition
# TSan output includes lock set analysis and happens-before edges
```

---

## LICH-009: Flow Label Birthday Attack Harness

### Objective
Exploit the limited entropy in Wotan L1 cache key derivation (20-bit flow label space) via birthday attack to force hash collisions and trigger cache eviction/coherency bugs.

### Target
- IPv4 5-tuple hash function with 20-bit flow label space
- Located at: `cmd/dashboard-backend/internal/ebpf/flow_label.rs` (flow labeling)

### Running the Fuzzer

#### Single-threaded
```bash
cd /path/to/unheaded
cargo +nightly fuzz run lich_009_flow_collision -- -max_len=1024
```

#### High-parallelism (recommended for this campaign)
```bash
cargo +nightly fuzz run lich_009_flow_collision \
  -j 16 \
  ebpf/fuzz/seeds/lich_009_flow_collision \
  -max_len=1024 \
  -max_total_time=86400  # 24 hours
```

#### With collision rate monitoring
```bash
# Run multiple parallel instances with different seeds
for i in {1..4}; do
  cargo +nightly fuzz run lich_009_flow_collision \
    -j 4 \
    ebpf/fuzz/seeds/lich_009_flow_collision \
    -artifact_prefix=collision_run_${i}/ \
    -max_total_time=86400 &
done
wait
```

#### Crash minimization mode
```bash
# After finding crashes, minimize them
cargo +nightly fuzz cmin lich_009_flow_collision \
  fuzz/artifacts/lich_009_flow_collision
```

### Seed Corpus

Location: `ebpf/fuzz/seeds/lich_009_flow_collision/` (50 seeds, 800 bytes total)

Seed breakdown:
- IPv4 5-tuple seeds: [src_ip:4][dst_ip:4][src_port:2][dst_port:2][proto:1][pad:3]
- `seed_000-009`: Diverse address classes (loopback, private ranges, multicast, broadcast)
- `seed_010-014`: Edge cases (all zeros, all ones, sequential)
- `seed_015-049`: Birthday attack candidates with high collision probability

### Oracle Verification

The fuzzer checks for violations:
1. **Collision rate analysis** based on birthday paradox
2. **Flow isolation bypass** detection (flow A reads data intended for flow B)
3. **Cache coherency** after collision-induced evictions
4. **Throughput degradation** quantified (cache thrashing)
5. **Timing side-channel** detection via cache hit/miss patterns

### Expected Findings

- ≥50 collision pairs identified (birthday attack predicts ~707 for 1M values)
- Proof-of-concept showing coherency violation via collision
- Cache hit rates quantified (<20% for adversarial sequence)
- Evidence that 20-bit key space is insufficient

### Success Criteria (S21 Assessment)

- ≥50 collision pairs identified
- Proof-of-concept showing coherency violation
- Cache hit rates quantified (<20% for adversarial sequence)
- Recommendation documented: extend key to ≥64 bits

### Analysis Tools

```bash
# Extract collision statistics
grep -o "Collision count: [0-9]*" fuzz/artifacts/lich_009_flow_collision/*

# Birthday attack verification
python3 << 'EOF'
import math
# Birthday attack formula for 20-bit space (2^20 = 1M possible values)
n = 2**20
expected_collisions = math.sqrt(math.pi * n / 2)
print(f"For {n} values, birthday attack expects ~{expected_collisions:.0f} collisions")
# Result: ~707 collisions for 50% probability
EOF
```

---

## LICH-010: WAL Compaction Race Harness

### Objective
Expose race conditions and data corruption in Wotan's Write-Ahead Log (WAL) during concurrent write and compaction operations, verifying atomicity and durability guarantees.

### Target
- WAL write/compaction interleaving with seqno monotonicity verification
- Located at: `cmd/dashboard-backend/internal/ebpf/anamnesis.rs` (WAL implementation)

### Running the Fuzzer

#### Single-threaded baseline
```bash
cd /path/to/unheaded
cargo +nightly fuzz run lich_010_wal_integrity -- -max_len=4096
```

#### Multi-threaded with all checks enabled
```bash
cargo +nightly fuzz run lich_010_wal_integrity \
  -j 8 \
  ebpf/fuzz/seeds/lich_010_wal_integrity \
  -max_len=4096 \
  -max_total_time=172800  # 48 hours
```

#### High-frequency monitoring (detect data loss immediately)
```bash
cargo +nightly fuzz run lich_010_wal_integrity \
  -j 8 \
  ebpf/fuzz/seeds/lich_010_wal_integrity \
  -timeout=5 \
  -print_final_stats=1 \
  -max_total_time=172800
```

#### Crash reproduction (after finding failures)
```bash
# Reproduce a specific crash
cargo +nightly fuzz run lich_010_wal_integrity \
  path/to/crash_file
```

### Seed Corpus

Location: `ebpf/fuzz/seeds/lich_010_wal_integrity/` (30 seeds, variable size)

Each seed contains Anamnesis WAL events (32 bytes each):
- [timestamp_ns:8][event_type:1][hop_index:1][reserved:2][monad_before:20]

Seed breakdown:
- `seed_000-001`: Single events with edge-case timestamps
- `seed_002-006`: Sequential and rapid-fire events, wrap-around (max u64)
- `seed_007-009`: All event types and hop indices
- `seed_010-014`: Concurrent writer simulation and compaction triggers
- `seed_015-029`: Stress patterns (increasing, decreasing, bursts, random, clustered)

### Oracle Verification

The fuzzer checks for violations:
1. **No data loss** after replay (Seqno monotonicity)
2. **HMAC-SHA256 validation** of all WAL records
3. **Compaction exclusivity** enforced (only one at a time)
4. **Crash-and-recover scenarios** validate correctly
5. **Recovery time** <100ms for 10k-record WAL

### Expected Findings

- Lost writes (compaction drops entries)
- Partial WAL record corruption
- Replay divergence after compaction
- CAS-race allowing concurrent compaction
- WAL segment pointer inconsistency
- Double-free or use-after-free in segment recycling

### Success Criteria (S21 Assessment)

- Zero data loss after replay
- All WAL records validated with HMAC-SHA256
- Compaction exclusivity enforced (1 at a time)
- Crash-and-recover scenarios validate correctly
- WAL seqno monotonicity guaranteed
- Recovery SLA: <100ms for 10k-record WAL

### Monitoring

```bash
# Check for seqno violations
grep -i "seqno gap\|data loss\|corruption" fuzz/artifacts/lich_010_wal_integrity/*

# Measure recovery time
grep -i "recovery\|replay" fuzz/artifacts/lich_010_wal_integrity/*

# Verify HMAC checksums
grep -i "hmac\|invalid\|checksum" fuzz/artifacts/lich_010_wal_integrity/*
```

---

## Go Protocol Encoding Fuzzers

### Overview

These are native Go fuzzers (Go 1.18+ compatible) for protocol encoding functions.

### Targets

1. **FuzzVarintRoundtrip** - Varint encoding/decoding
2. **FuzzExponentRoundtrip** - Floating-point exponent encoding
3. **FuzzCRC16CCITT** - CRC-16-CCITT computation
4. **FuzzTLVRoundtrip** - Type-Length-Value encoding

### Running the Fuzzers

#### Run all Go fuzzers
```bash
cd /path/to/unheaded
go test -fuzz=. -fuzztime=300s -parallel=4 ./pkg/protocol/fuzz
```

#### Run specific fuzzer
```bash
go test -fuzz=FuzzVarintRoundtrip -fuzztime=300s ./pkg/protocol/fuzz
```

#### With coverage
```bash
go test -fuzz=. -fuzztime=300s -cover ./pkg/protocol/fuzz
```

#### Continuous fuzzing (1 hour)
```bash
go test -fuzz=. -fuzztime=3600s -parallel=8 ./pkg/protocol/fuzz
```

#### Reproduce specific crash
```bash
# Go automatically saves crash inputs in testdata/fuzz/<FuzzName>/
go test -run=FuzzVarintRoundtrip/hash-value-from-failure ./pkg/protocol/fuzz
```

### Corpus Management

Go automatically manages corpus in `testdata/fuzz/` subdirectories:

```bash
# View discovered test cases
find pkg/protocol/fuzz/testdata -name "*" -type f

# Add custom seeds
mkdir -p pkg/protocol/fuzz/testdata/fuzz/FuzzVarintRoundtrip
echo "custom_seed_data" > pkg/protocol/fuzz/testdata/fuzz/FuzzVarintRoundtrip/seed_001
```

---

## Combined Campaign Execution

### Sequential Execution (Uses 1 machine, duration ~190 hours)

```bash
#!/bin/bash
set -e

cd /path/to/unheaded

echo "Starting LICH Campaign Sequence..."
echo "Total expected duration: ~190 hours"
echo ""

# LICH-007: 48 hours
echo "[1/4] LICH-007 (MBC) - Est. 48 hours..."
cargo +nightly fuzz run lich_007_mbc \
  -j 4 \
  ebpf/fuzz/seeds/lich_007_mbc \
  -max_total_time=172800

# LICH-008: 72 hours
echo "[2/4] LICH-008 (Cache) - Est. 72 hours..."
RUSTFLAGS="-Zsanitizer=thread" TSAN_OPTIONS="halt_on_error=1" \
  cargo +nightly fuzz run lich_008_wotan_cache \
  -j 8 \
  ebpf/fuzz/seeds/lich_008_wotan_cache \
  -max_total_time=259200

# LICH-009: 24 hours
echo "[3/4] LICH-009 (Flow Collision) - Est. 24 hours..."
cargo +nightly fuzz run lich_009_flow_collision \
  -j 8 \
  ebpf/fuzz/seeds/lich_009_flow_collision \
  -max_total_time=86400

# LICH-010: 48 hours
echo "[4/4] LICH-010 (WAL) - Est. 48 hours..."
cargo +nightly fuzz run lich_010_wal_integrity \
  -j 4 \
  ebpf/fuzz/seeds/lich_010_wal_integrity \
  -max_total_time=172800

echo "All campaigns completed!"
```

### Parallel Execution (Uses 4 machines, duration ~48 hours)

```bash
#!/bin/bash
# Run on 4 separate machines (16 cores total)

# Machine 1: LICH-007
cargo +nightly fuzz run lich_007_mbc \
  -j 4 ebpf/fuzz/seeds/lich_007_mbc \
  -max_total_time=172800 &

# Machine 2: LICH-008
RUSTFLAGS="-Zsanitizer=thread" cargo +nightly fuzz run lich_008_wotan_cache \
  -j 8 ebpf/fuzz/seeds/lich_008_wotan_cache \
  -max_total_time=259200 &

# Machine 3: LICH-009
cargo +nightly fuzz run lich_009_flow_collision \
  -j 4 ebpf/fuzz/seeds/lich_009_flow_collision \
  -max_total_time=86400 &

# Machine 4: LICH-010 + Go Fuzzers
(cargo +nightly fuzz run lich_010_wal_integrity \
  -j 4 ebpf/fuzz/seeds/lich_010_wal_integrity \
  -max_total_time=172800 &)
(go test -fuzz=. -fuzztime=3600s -parallel=4 ./pkg/protocol/fuzz &)

wait
```

---

## Interpreting Results

### Crashes and Hangs

Crashes are saved to `fuzz/artifacts/lich_00X/`:

```bash
# List all crashes
ls -lh fuzz/artifacts/lich_007_mbc/
ls -lh fuzz/artifacts/lich_008_wotan_cache/
ls -lh fuzz/artifacts/lich_009_flow_collision/
ls -lh fuzz/artifacts/lich_010_wal_integrity/

# Analyze a specific crash
cargo +nightly fuzz run lich_007_mbc fuzz/artifacts/lich_007_mbc/crash-abc123def456

# Get stack trace
cargo +nightly fuzz run lich_007_mbc -print_stacktrace \
  fuzz/artifacts/lich_007_mbc/crash-abc123def456
```

### Coverage Analysis

```bash
# Generate coverage for a campaign
cargo +nightly fuzz cov lich_007_mbc

# View coverage report
open coverage/index.html
```

### Performance Metrics

```bash
# Extract performance stats
grep -E "cov:|exec/s:|execs:|leak" fuzz/artifacts/lich_007_mbc/*.log

# Example output:
# cov: 234 ft: 567 corp: 89 lim: 1024 exec/s: 890 rss: 450Mb
#   cov    = code coverage edge count
#   ft     = feature count (edges covered)
#   corp   = corpus size
#   exec/s = executions per second
#   rss    = resident set size (memory usage)
```

---

## Troubleshooting

### Fuzzer fails to start

```bash
# Ensure nightly toolchain is installed
rustup toolchain install nightly --force

# Update cargo-fuzz
cargo +nightly install cargo-fuzz --force

# Try building the harness first
cargo +nightly fuzz build lich_007_mbc
```

### Low throughput (exec/s < 100)

- **Reduce max_len:** Smaller inputs are faster
- **Increase parallelism:** `-j 8` or `-j 16`
- **Check system load:** `top`, ensure sufficient CPU available
- **Disable ThreadSanitizer temporarily:** TSAN overhead is ~50%

### ThreadSanitizer errors

```bash
# Enable verbose TSan output
TSAN_OPTIONS="verbosity=1:log_path=tsan.log" \
  cargo +nightly fuzz run lich_008_wotan_cache
```

### Out of memory

- **Reduce parallelism:** Lower `-j` value
- **Reduce max_len:** Smaller input size limit
- **Monitor RSS:** Fuzzer memory usage grows with corpus size
- **Check for memory leaks:** Use `-detect_leaks=1`

---

## Integration with Dark Grimoire

Findings from these campaigns feed into the Dark Grimoire attack surface taxonomy:

```
docs/security/dark-grimoire-addendum.md
  ├── LICH-007 Findings
  │   ├── MBC Instruction Decoding Flaws
  │   └── Integer Overflow Vulnerabilities
  ├── LICH-008 Findings
  │   ├── Race Conditions
  │   └── Memory Barrier Violations
  ├── LICH-009 Findings
  │   ├── Flow Label Collisions
  │   └── Cache Side-Channels
  └── LICH-010 Findings
      ├── WAL Data Loss
      └── Compaction Race Conditions
```

Each crash or race becomes:
- A documented attack vector
- A permanent regression test
- Input for specification patches
- Evidence for security recommendations

---

## Next Steps

1. **Start Seed Generation** (already completed)
   - Run: `python3 ebpf/fuzz/generate_seeds.py`
   - Verify: 120 seeds generated across 4 campaigns

2. **Execute Campaigns** (in order of priority)
   - Priority 1 (24 hours): LICH-009 (Flow Collision) - highest impact
   - Priority 2 (48 hours): LICH-007 (MBC) - baseline stability
   - Priority 3 (72 hours): LICH-008 (Cache) - requires ThreadSanitizer
   - Priority 4 (48 hours): LICH-010 (WAL) - data integrity focus

3. **Monitor Results**
   - Check crash artifacts daily
   - Triage and categorize findings
   - Document root causes
   - File security patches

4. **Generate Report**
   - Record results in `docs/security/lich-results-S24.md`
   - Quantify improvements from patches
   - Recommendations for next phases

---

## References

- LICH Campaign Documentation: `docs/security/lich-campaigns.md`
- Dark Grimoire Addendum: `docs/security/dark-grimoire-addendum.md`
- Rust Fuzzing Book: https://rust-fuzz.github.io/
- Go Fuzzing Guide: https://pkg.go.dev/testing#hdr-Fuzzing
