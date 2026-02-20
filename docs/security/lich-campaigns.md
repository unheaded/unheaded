# LICH Campaigns - S21 Assessment

This document specifies security testing campaigns from the S21 assessment against the Unheaded protocol. Each campaign targets specific attack surfaces using coverage-guided fuzzing, concurrency testing, and specialized oracle techniques.

## LICH-007: MBC Bytecode Instruction Fuzzing

**Objective:** Identify bytecode interpretation errors, instruction encoding flaws, and correctness violations in the Doom substrate's MBC (Minimal Bytecode Compiler) implementation.

**Target:** `monad-mbc` crate (bytecode interpreter and verification functions)

**Technique:** AFL++ coverage-guided fuzzing with persistent mode

**Tool:** AFL++ 4.09c+ with libFuzzer interface

**Corpus Seeds:**
- `valid_instructions/`: All 256 MBC opcodes with valid register encodings (R0-R15)
- `valid_instructions/control_flow`: Branch, jump, call sequences (PC-relative offsets)
- `valid_instructions/memory_ops`: Load/store with bounds-valid addresses
- `mutations/`: Bit-flip mutations of valid instructions at byte boundaries
- `mutations/register_chaos/`: Invalid register numbers (R16+, mixed widths)
- `mutations/offset_overflow/`: Branch offsets > i32 range, negative jumps to invalid PCs

**Expected Findings:**
- Instruction decoding off-by-one errors
- Register width mismatches (8-bit vs 16-bit instruction variants)
- Stack underflow on stack machine variants
- Incorrect memory protection on mmapped regions
- Branch offset calculation errors leading to privilege escape
- Integer overflow in offset arithmetic

**Duration:** 48 hours continuous fuzzing

**Success Criteria:**
- ≥2 new coverage paths discovered per hour (decreasing)
- ≥1 crash or assertion failure per 8 hours of fuzzing
- All crashes reproducible and categorized by severity
- Root cause identified for crash triggers
- New corpus seeds extracted from crashes for regression suite

**Monitoring:**
```
- Watch for: MAP_SHARED violations, integer wraps, PC out of bounds
- Sync coverage with corpora/$CAMPAIGN_ID/crashes
- Daily triage of new hangs (timeout = 1 second per input)
```

---

## LICH-008: Wotan L1 Cache Line Race Condition Fuzzing

**Objective:** Expose concurrency bugs in the Wotan L1 memory-mapped cache under concurrent access patterns, focusing on TOCTOU (time-of-check-time-of-use) violations and memory ordering issues.

**Target:** `bpf_wotan_read()` and `bpf_wotan_write()` BPF helper functions and their underlying memory barriers

**Technique:** Concurrent access fuzzing with thread-local eBPF programs

**Tool:** libFuzzer with custom pthread harness + TSan (ThreadSanitizer)

**Corpus Seeds:**
- `base_read_write/`: Single-threaded R/W pairs on cache lines
- `concurrent_same_key/`: Multiple threads R/W to identical flow label keys
- `concurrent_range/`: Overlapping byte-range accesses on same 64-byte cache line
- `memory_order_chaos/`: Relaxed, release, acquire semantics crossing thread boundaries
- `barrier_sequences/`: Wmb, rmb, mb combinations interspersed with accesses

**Expected Findings:**
- Load-store reordering violations (stale reads after write)
- Store buffering failures on non-sequentially-consistent hardware
- Lost updates due to missing memory barriers
- Premature eviction during ongoing access
- CAS (Compare-And-Swap) instruction atomicity gaps
- Lock-free data structure invariant violations (queue head/tail desync)

**Duration:** 72 hours with 8-16 concurrent threads

**Success Criteria:**
- ≥3 race conditions confirmed by TSan with thread stacks
- Happens-before violations documented with memory timeline
- Atomic operation misuse identified (non-atomic CAS, missing __sync_* calls)
- All races reproducible with deterministic thread scheduling (via GDB)
- Patches reduce race detector warnings by ≥95%

**Monitoring:**
```
- ThreadSanitizer: Enable halt_on_error=true, track_origins=true
- Coverage goal: 100% of barrier paths in wotan helpers
- Parallel execution: 16 instances × 4 threads = 64 concurrent mini-programs
```

---

## LICH-009: Cross-Flow Composite Key Collision Testing

**Objective:** Exploit the limited entropy in Wotan L1 cache key derivation (currently flow_label only, 20 bits) via birthday attack to force hash collisions and trigger cache eviction/coherency bugs.

**Target:** Wotan L1 cache key derivation function and flow label field (20 bits)

**Technique:** Birthday attack / collision search with oracle-guided mutation

**Tool:** Custom Python harness + bpf-perf for L1 cache instrumentation

**Corpus Seeds:**
- `flow_label_space/`: All 2^20 possible 20-bit flow label values (1M seeds)
- `sparse_subset/`: 10k carefully selected flow labels (max parity dispersion)
- `collision_candidates/`: Flow label pairs with high collision probability (Hamming distance ≤ 4 bits)
- `sequential_ranges/`: Flow labels 0x00000 - 0xFFFFF in incremental steps

**Expected Findings:**
- Multiple simultaneous flows evicting each other (false cache misses)
- Flow isolation bypass: flow A reads cached data intended for flow B
- Coherency violations: stale flow_B data visible to flow_A due to collision
- Cache line thrashing reducing throughput by ≥30% for adversarial label sequence
- Possibility of timing-based side-channel attacks via cache hit/miss patterns

**Duration:** 24 hours (high parallelism; search space manageable)

**Success Criteria:**
- ≥50 collision pairs identified (birthday attack predicts ~707 collisions for 2^20 with 50% probability)
- Proof-of-concept flow label sequence demonstrating coherency violation
- Cache hit rates quantified (expected <20% for adversarial sequence)
- Extension to composite key (flow + src/dst hash) evaluated for entropy improvement
- Recommendation: Extend key to ≥64 bits (flow_label 20 bits + tuple hash 44 bits)

**Monitoring:**
```
- BPF perf counters: L1 cache hits/misses, evictions
- Flow label distribution: Ensure no clustering artifacts
- Collision count reaching theoretical birthday attack bound
```

---

## LICH-010: WAL Integrity / Compaction Race Testing

**Objective:** Expose race conditions and data corruption in Wotan's Write-Ahead Log during concurrent write and compaction operations, verifying atomicity and durability guarantees.

**Target:** Wotan WAL module (write, append, compact, replay functions)

**Technique:** Concurrent operation interleavings with simulated compaction during writes

**Tool:** libFuzzer with WAIT API instrumentation for ordered event sequences

**Corpus Seeds:**
- `rapid_writes/`: Burst of 1000+ writes followed by compaction
- `interleaved_writes_compaction/`: Write then compact interleaved at byte granularity
- `concurrent_reads_during_compact/`: Replay threads reading while compaction progresses
- `power_failure_recovery/`: Incomplete compaction + crash, followed by replay validation
- `segment_boundary_chaos/`: Writes landing exactly on WAL segment boundaries (4KB chunks)

**Expected Findings:**
- Lost writes: compaction drops recent entries not yet persisted
- Corruption: partial WAL record writes due to lack of atomicity
- Replay divergence: restored state differs from pre-crash state
- CAS-race in compaction: multiple threads begin compaction simultaneously
- WAL segment pointer inconsistency (metadata v.s. actual file size)
- Double-free or use-after-free in segment recycling

**Duration:** 48 hours continuous

**Success Criteria:**
- Zero data loss after replay of found race interleavings
- All WAL records verified with HMAC-SHA256 (add to spec)
- Compaction exclusivity enforced: only one compaction at a time
- Crash-and-recover scenarios all validate correctly
- WAL seqno monotonicity guaranteed for all test runs
- Recovery time documented (SLA: <100ms for 10k-record WAL)

**Monitoring:**
```
- Trace all WAL operations: alloc, write, append, compact, free
- Verify seqno continuity: gaps indicate lost records
- Checksum every segment before/after compaction
- Measure compaction latency vs. concurrent write throughput
```

---

## Campaign Execution Workflow

Each LICH campaign follows this workflow:

1. **Corpus Preparation** (2-4 hours)
   - Seed corpus validation (all seeds must pass oracle checks)
   - Import existing regression suite into initial corpus
   - Calculate corpus size and coverage baseline

2. **Fuzzing Execution** (24-72 hours)
   - Start fuzzer instances with distinct RNG seeds
   - Monitor coverage growth and crash discovery rate
   - Triage new crashes hourly

3. **Crash Analysis** (ongoing)
   - Reproduce each crash deterministically
   - Extract minimal test case (delta-debugging)
   - Categorize by root cause (instruction decode, concurrency, etc.)
   - Verify fix on patched code

4. **Report Generation** (4 hours)
   - Summarize findings by severity (critical/high/medium/low)
   - Provide proof-of-concept inputs for each bug
   - Recommend patches or specification clarifications

---

## Integration with Dark Grimoire

Findings from these LICH campaigns feed directly into the Dark Grimoire attack surface taxonomy (see `dark-grimoire-addendum.md`). Each crash or race condition discovered here becomes a documented attack vector and a permanent regression test.

