# LICH Fuzzing Campaign Harnesses

This directory contains Rust fuzzing harnesses for the LICH (Long-Interval Computational Hazards) campaigns, part of the S21 security assessment for the Unheaded protocol.

## Harnesses

### LICH-007: MBC Bytecode Instruction Fuzzer (`lich_007_mbc.rs`)

**Objective:** Identify bytecode interpretation errors, instruction encoding flaws, and correctness violations in the Doom substrate's MBC (Minimal Bytecode Compiler) implementation.

**Target:** MBC instruction decoder with random bytecode sequences

**What it fuzzes:**
- Instruction parsing with all 256 opcodes
- Register validation (must be 0-15)
- Stack machine operations (PUSH/POP)
- Memory load/store with bounds checking
- Arithmetic operations with overflow detection
- Branch/jump offset calculation with bounds validation

**Expected to catch:**
- Instruction decoding off-by-one errors
- Register width mismatches
- Stack underflow/overflow
- Incorrect memory protection
- Branch offset calculation errors
- Integer overflow violations

**Running the fuzzer:**
```bash
cargo +nightly fuzz -C ebpf/fuzz run lich_007_mbc
```

---

### LICH-008: Wotan L1 Cache Race Condition Harness (`lich_008_wotan_cache.rs`)

**Objective:** Expose concurrency bugs in Wotan L1 memory-mapped cache under concurrent access patterns, focusing on TOCTOU (time-of-check-time-of-use) violations and memory ordering issues.

**Target:** Concurrent cache access with flow label keys and memory barriers

**What it fuzzes:**
- Concurrent read/write patterns on same cache line
- Cache coherency with multiple flow labels
- Memory barrier sequences (wmb, rmb, mb)
- CAS (Compare-And-Swap) atomicity
- Lock-free data structure invariants

**Expected to catch:**
- Load-store reordering violations
- Store buffering failures
- Lost updates due to missing memory barriers
- CAS atomicity gaps
- Cache coherency violations

**Running the fuzzer:**
```bash
cargo +nightly fuzz -C ebpf/fuzz run lich_008_wotan_cache
```

---

### LICH-009: Flow Label Birthday Attack Harness (`lich_009_flow_collision.rs`)

**Objective:** Exploit the limited entropy in Wotan L1 cache key derivation (20-bit flow label space) via birthday attack to force hash collisions and trigger cache eviction/coherency bugs.

**Target:** IPv6 flow label generation with 20-bit space analysis

**What it fuzzes:**
- Flow label distribution and collision rates
- Birthday attack probability analysis
- Cache eviction patterns under collisions
- Flow isolation bypass detection
- Timing side-channel attacks via cache hit/miss patterns

**Expected to catch:**
- Excessive collisions indicating weak entropy
- Flow isolation bypass (flow A reads flow B's data)
- Cache coherency violations from collisions
- Cache line thrashing reducing throughput
- Timing-based side-channel vulnerabilities

**Key finding:** 20-bit flow label insufficient entropy; recommend composite key (20 bits + 44-bit hash = 64 bits)

**Running the fuzzer:**
```bash
cargo +nightly fuzz -C ebpf/fuzz run lich_009_flow_collision
```

---

### LICH-010: WAL Compaction Race Harness (`lich_010_wal_integrity.rs`)

**Objective:** Expose race conditions and data corruption in Wotan's Write-Ahead Log (WAL) during concurrent write and compaction operations, verifying atomicity and durability guarantees.

**Target:** WAL write/compaction interleaving with seqno monotonicity verification

**What it fuzzes:**
- Rapid writes followed by compaction
- Interleaved write/compaction at byte granularity
- Concurrent reads during compaction
- Power failure recovery scenarios
- WAL segment boundary edge cases
- CAS-race in concurrent compaction
- Seqno validation with HMAC-SHA256

**Expected to catch:**
- Lost writes (compaction drops entries)
- Partial WAL record corruption
- Replay divergence after compaction
- CAS-race allowing concurrent compaction
- WAL segment pointer inconsistency
- Double-free or use-after-free in segment recycling

**Oracle criteria:**
- Zero data loss after replay
- WAL records validated with HMAC-SHA256
- Compaction exclusivity (only one at a time)
- Crash-and-recover scenarios validate correctly
- WAL seqno monotonicity guaranteed
- Recovery SLA: <100ms for 10k-record WAL

**Running the fuzzer:**
```bash
cargo +nightly fuzz -C ebpf/fuzz run lich_010_wal_integrity
```

---

## Building and Running

### Prerequisites
- Rust nightly toolchain: `rustup toolchain install nightly`
- libFuzzer integration: `cargo +nightly install cargo-fuzz`

### Build all harnesses
```bash
cd ebpf/fuzz
cargo +nightly fuzz build
```

### Run a specific harness
```bash
cargo +nightly fuzz run lich_007_mbc
cargo +nightly fuzz run lich_008_wotan_cache
cargo +nightly fuzz run lich_009_flow_collision
cargo +nightly fuzz run lich_010_wal_integrity
```

### Run with corpus
```bash
mkdir -p corpus/lich_007_mbc
# Add seed corpus files to corpus/lich_007_mbc/
cargo +nightly fuzz run lich_007_mbc corpus/lich_007_mbc
```

### Run with coverage
```bash
LLVM_PROFILE_FILE="coverage-%p.profraw" cargo +nightly fuzz run lich_007_mbc
```

---

## Success Criteria (S21 Assessment)

### LICH-007
- ≥2 new coverage paths discovered per hour (decreasing trend)
- ≥1 crash or assertion failure per 8 hours
- All crashes reproducible and categorized by severity
- Root cause identified for crash triggers

### LICH-008
- ≥3 race conditions confirmed by TSan
- Happens-before violations documented
- Atomic operation misuse identified
- All races reproducible with deterministic scheduling
- Patches reduce race detector warnings by ≥95%

### LICH-009
- ≥50 collision pairs identified (birthday attack predicts ~707)
- Proof-of-concept showing coherency violation
- Cache hit rates quantified (<20% for adversarial sequence)
- Recommendation: extend key to ≥64 bits

### LICH-010
- Zero data loss after replay
- All WAL records validated with HMAC-SHA256
- Compaction exclusivity enforced (1 at a time)
- Crash-and-recover scenarios validate correctly
- WAL seqno monotonicity guaranteed
- Recovery time <100ms for 10k-record WAL

---

## Corpus Preparation

Each campaign requires seed corpus:

### LICH-007 corpus
- `valid_instructions/`: All 256 opcodes with valid register encodings (R0-R15)
- `valid_instructions/control_flow/`: Branch, jump, call sequences
- `valid_instructions/memory_ops/`: Load/store with bounds-valid addresses
- `mutations/register_chaos/`: Invalid register numbers (R16+)
- `mutations/offset_overflow/`: Branch offsets > i32 range

### LICH-008 corpus
- `base_read_write/`: Single-threaded R/W pairs
- `concurrent_same_key/`: Multiple threads on same key
- `concurrent_range/`: Overlapping byte-range accesses
- `memory_order_chaos/`: Relaxed, release, acquire semantics

### LICH-009 corpus
- `flow_label_space/`: All 2^20 possible values (1M seeds)
- `sparse_subset/`: 10k carefully selected flow labels
- `collision_candidates/`: High-collision-probability pairs (Hamming distance ≤ 4)
- `sequential_ranges/`: 0x00000-0xFFFFF in increments

### LICH-010 corpus
- `rapid_writes/`: 1000+ writes followed by compaction
- `interleaved_writes_compaction/`: Write/compact at byte granularity
- `concurrent_reads_during_compact/`: Replay threads during compaction
- `power_failure_recovery/`: Incomplete compaction + crash + replay
- `segment_boundary_chaos/`: Writes on 4KB segment boundaries

---

## Integration with Dark Grimoire

Findings from these campaigns feed into the Dark Grimoire attack surface taxonomy (see `docs/security/dark-grimoire-addendum.md`). Each crash or race becomes:
- A documented attack vector
- A permanent regression test
- Input for specification patches
- Evidence for security recommendations

---

## Duration and Resource Requirements

- **LICH-007:** 48 hours continuous fuzzing
- **LICH-008:** 72 hours with 8-16 concurrent threads
- **LICH-009:** 24 hours (high parallelism; search space manageable)
- **LICH-010:** 48 hours continuous fuzzing

Total campaign: ~190 hours of fuzzing across all campaigns.

---

## Monitoring and Metrics

### LICH-007
- Watch for: MAP_SHARED violations, integer wraps, PC out of bounds
- Sync coverage with corpora/$CAMPAIGN_ID/crashes
- Daily triage of new hangs (timeout = 1 second per input)

### LICH-008
- ThreadSanitizer: Enable halt_on_error=true, track_origins=true
- Coverage goal: 100% of barrier paths in wotan helpers
- Parallel execution: 16 instances × 4 threads = 64 concurrent mini-programs

### LICH-009
- BPF perf counters: L1 cache hits/misses, evictions
- Flow label distribution: Ensure no clustering artifacts
- Collision count reaching theoretical birthday attack bound

### LICH-010
- Trace all WAL operations: alloc, write, append, compact, free
- Verify seqno continuity: gaps indicate lost records
- Checksum every segment before/after compaction
- Measure compaction latency vs concurrent write throughput

