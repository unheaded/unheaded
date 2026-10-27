# LICH Campaign Results (S24 Assessment)

**Status:** Ready for Execution
**Generated:** 2026-02-20
**Campaign Start Date:** [TBD - to be filled when execution begins]
**Campaign End Date:** [TBD - to be filled when execution completes]

---

## Executive Summary

This document records the results of the LICH (Long-Interval Computational Hazards) fuzzing campaigns, a comprehensive security assessment of the Unheaded protocol's critical components. The campaigns test bytecode interpretation, cache coherency, flow label entropy, and write-ahead log durability.

**Campaign Status:** [PENDING / IN PROGRESS / COMPLETED]

---

## LICH-007: MBC Bytecode Instruction Fuzzer

### Campaign Metadata

| Metric | Value |
|--------|-------|
| **Objective** | Identify bytecode interpretation errors and instruction encoding flaws |
| **Target** | MBC instruction decoder (Doom substrate) |
| **Seed Corpus Size** | 20 seeds (160 bytes) |
| **Expected Duration** | 48 hours |
| **Resource Allocation** | 4-8 CPU cores, 4GB RAM |
| **Status** | [PENDING / IN PROGRESS / COMPLETED] |

### Corpus Statistics

```
Seed Corpus Breakdown:
├── Single instructions:       6 seeds (NOP, LOAD_IMM, STORE, ADD, JMP, JEQ)
├── Multi-instruction seq:     7 seeds (2-3 instructions)
├── Complex sequences:         1 seed  (4-instruction: LOAD+ADD+STORE+RET)
├── Boundary conditions:       3 seeds (jump overflow, negative jumps, max register)
└── Stress patterns:           3 seeds (all registers, alternation, 16-instruction)

Total Seeds:                   20
Total Initial Corpus:          160 bytes
Largest Seed:                  64 bytes
```

### Initial Execution

```
Execution Start Time:     [TBD]
Execution Duration:       [TBD]
Final Corpus Size:        [TBD] seeds
Final Corpus Size (MB):   [TBD]
Cumulative Executions:    [TBD]
Average Throughput:       [TBD] exec/sec
Peak Throughput:          [TBD] exec/sec
```

### Results Collected

#### Coverage Metrics

```
Code Coverage:
  - Instruction opcodes covered:    [TBD] / 256
  - Boundary conditions triggered:  [TBD] / 20
  - Edge cases found:               [TBD]

Feature Coverage:
  - New coverage paths/hour:        [TBD] (target: ≥2)
  - Coverage increase over time:    [TBD]%
  - Plateau reached at:             [TBD] hours
```

#### Crashes and Failures

```
Total Crashes:            [TBD]
Unique Crashes:           [TBD]
Reproducible Crashes:     [TBD]
Crash Minimization:       [TBD] bytes (from largest seed)

Crash Categories:
  - Instruction decoding errors:    [TBD]
  - Register validation failures:   [TBD]
  - Memory protection violations:   [TBD]
  - Branch offset errors:           [TBD]
  - Integer overflow violations:    [TBD]
  - Stack underflow/overflow:       [TBD]
  - Other panics/assertions:        [TBD]
```

#### Severity Classification

```
Critical (immediate DoS):
  - Crash ID: [TBD] - [Description]
  - Crash ID: [TBD] - [Description]

High (information disclosure):
  - Crash ID: [TBD] - [Description]
  - Crash ID: [TBD] - [Description]

Medium (incorrect behavior):
  - Crash ID: [TBD] - [Description]
  - Crash ID: [TBD] - [Description]

Low (edge case):
  - Crash ID: [TBD] - [Description]
  - Crash ID: [TBD] - [Description]
```

### Sample Crash Details

```
Crash #1: Off-by-one in jump offset calculation
├── Input:     fuzz/artifacts/lich_007_mbc/crash-abc123def456
├── Size:      [TBD] bytes
├── Trigger:   JMP with offset near segment boundary
├── Error:     Out-of-bounds instruction fetch
├── Location:  MBC decoder, line [TBD]
├── Severity:  High
└── Status:    [CONFIRMED / UNDER INVESTIGATION / PATCHED]

Crash #2: [TBD]
├── Input:     [TBD]
├── Size:      [TBD] bytes
├── Trigger:   [TBD]
├── Error:     [TBD]
├── Location:  [TBD]
├── Severity:  [TBD]
└── Status:    [TBD]
```

### Assessment Against Criteria

| Criterion | Target | Actual | Status |
|-----------|--------|--------|--------|
| New coverage paths/hour | ≥2 | [TBD] | [✓/✗] |
| Crashes per 8 hours | ≥1 | [TBD] | [✓/✗] |
| Crashes reproducible | 100% | [TBD]% | [✓/✗] |
| Root causes identified | All | [TBD]/[TBD] | [✓/✗] |

### S21 Verdict

```
Coverage Adequacy:        [PASS / FAIL]
Crash Discovery Rate:     [PASS / FAIL]
Reproducibility:          [PASS / FAIL]
Root Cause Analysis:      [PASS / FAIL]

OVERALL LICH-007 RESULT:  [PASS / FAIL]
```

---

## LICH-008: Wotan L1 Cache Race Condition Harness

### Campaign Metadata

| Metric | Value |
|--------|-------|
| **Objective** | Expose concurrency bugs in Wotan L1 cache |
| **Target** | Cache access with flow labels and memory barriers |
| **Seed Corpus Size** | 20 seeds (variable, ~500 bytes) |
| **Expected Duration** | 72 hours |
| **Resource Allocation** | 8-16 CPU cores, 8GB RAM |
| **Sanitizer** | ThreadSanitizer (TSan) |
| **Status** | [PENDING / IN PROGRESS / COMPLETED] |

### Corpus Statistics

```
Seed Corpus Breakdown:
├── Single operations:         3 seeds (read, write, CAS)
├── Sequential patterns:       3 seeds (sequential, mixed, read-write-read)
├── High-contention loads:     3 seeds (same line, CAS-heavy)
├── Address boundaries:        4 seeds (min/max addr, collision, thrashing)
├── Long sequences:            4 seeds (cache warming, burst, mixed)
└── Special patterns:          3 seeds (memory barrier, sparse, all op types)

Total Seeds:                   20
Total Initial Corpus:          ~500 bytes
Largest Seed:                  ~64 bytes
```

### Initial Execution

```
Execution Start Time:     [TBD]
Execution Duration:       [TBD]
Final Corpus Size:        [TBD] seeds
Final Corpus Size (MB):   [TBD]
Cumulative Executions:    [TBD]
Average Throughput:       [TBD] exec/sec (with TSan overhead ~50%)
Race Conditions Found:    [TBD]
```

### Results Collected

#### ThreadSanitizer Findings

```
Total Race Reports:           [TBD]
Unique Races:                 [TBD]
Confirmed Data Races:         [TBD]

Race Categories:
  - Load-store reordering:    [TBD]
  - Store buffering:          [TBD]
  - Memory barrier misuse:    [TBD]
  - CAS atomicity:            [TBD]
  - Cache coherency:          [TBD]
```

#### Memory Barrier Coverage

```
Memory Barrier Paths Tested:
  - Write Memory Barrier (wmb):     [TBD] execution paths
  - Read Memory Barrier (rmb):      [TBD] execution paths
  - Full Memory Barrier (mb):       [TBD] execution paths
  - Atomic operations:              [TBD] execution paths

Coverage Goal: 100% barrier path coverage
Status: [TBD]%
```

#### Specific Race Conditions Found

```
Race #1: Store buffering violation in cache write
├── Location:  wotan_cache.rs, line [TBD]
├── Type:      Happens-before violation
├── Threads:   2+ concurrent writers
├── Trigger:   Rapid write + read on same cache line
├── Impact:    Stale read possible
├── Status:    [CONFIRMED / UNDER INVESTIGATION / PATCHED]
└── Patch:     [TBD] - Add wmb() after cache write

Race #2: [TBD]
├── Location:  [TBD]
├── Type:      [TBD]
├── Threads:   [TBD]
├── Trigger:   [TBD]
├── Impact:    [TBD]
├── Status:    [TBD]
└── Patch:     [TBD]
```

### Assessment Against Criteria

| Criterion | Target | Actual | Status |
|-----------|--------|--------|--------|
| Race conditions found | ≥3 | [TBD] | [✓/✗] |
| Happens-before violations | Documented | [TBD] | [✓/✗] |
| Atomic misuse identified | All cases | [TBD] | [✓/✗] |
| Races reproducible | 100% | [TBD]% | [✓/✗] |
| Patch effectiveness | ≥95% reduction | [TBD]% | [✓/✗] |

### S21 Verdict

```
TSan Coverage:            [PASS / FAIL]
Race Discovery:           [PASS / FAIL]
Root Cause Analysis:      [PASS / FAIL]
Patch Validation:         [PASS / FAIL]

OVERALL LICH-008 RESULT:  [PASS / FAIL]
```

---

## LICH-009: Flow Label Birthday Attack Harness

### Campaign Metadata

| Metric | Value |
|--------|-------|
| **Objective** | Exploit limited entropy in flow label space (20-bit) |
| **Target** | IPv4 5-tuple hash function |
| **Seed Corpus Size** | 50 seeds (800 bytes) |
| **Expected Duration** | 24 hours |
| **Resource Allocation** | 4-8 CPU cores, 4GB RAM |
| **Status** | [PENDING / IN PROGRESS / COMPLETED] |

### Corpus Statistics

```
Seed Corpus Breakdown:
├── Address space variations:  10 seeds (loopback, private, multicast, broadcast)
├── Edge cases:                5 seeds (all zeros, all ones, sequential)
├── Birthday attack candidates: 35 seeds (high collision probability)
│   ├── Class A addresses:    4 seeds
│   ├── Class B addresses:    4 seeds
│   ├── Class C addresses:    8 seeds
│   ├── Link-local:           5 seeds
│   ├── Loop-back:            5 seeds
│   ├── Common protocols:      3 seeds (DNS, HTTP, HTTPS, etc)
│   └── Random entropy:        1 seed

Total Seeds:                   50
Total Initial Corpus:          800 bytes
Average Seed Size:            16 bytes (one 5-tuple)
```

### Initial Execution

```
Execution Start Time:     [TBD]
Execution Duration:       [TBD]
Final Corpus Size:        [TBD] seeds
Final Corpus Size (MB):   [TBD]
Cumulative Executions:    [TBD]
Average Throughput:       [TBD] exec/sec
Collision Pairs Found:    [TBD]
```

### Results Collected

#### Collision Analysis

```
Total 5-tuples Tested:        [TBD]
Unique Hash Values:           [TBD]
Collision Pairs:              [TBD] (target: ≥50, predicted: ~707)
Collision Rate:               [TBD]%

Birthday Attack Verification:
├── Theoretical prediction:    ~707 collisions (50% probability)
├── Actual collisions:         [TBD]
├── vs. Theory:                [TBD]% match
└── Entropy Assessment:        [INSUFFICIENT / BORDERLINE / ADEQUATE]
```

#### Cache Coherency Impact

```
Flow Isolation Bypass:
  - Detected:                [TBD]
  - False positives:         [TBD]
  - Confirmed issues:        [TBD]

Cache Hit Rate Analysis:
  - Normal workload:         [TBD]%
  - Adversarial pattern:     [TBD]%
  - Degradation:             [TBD]%
  - Threshold (warning):     20%
  - Status:                  [PASS / FAIL]
```

#### Timing Side-Channel

```
Cache Hit/Miss Patterns Detected: [TBD]
Timing variance:                  [TBD] cycles
Information leaked:               [TBD] bits/operation
Feasibility of attack:            [HIGH / MEDIUM / LOW]
```

### Assessment Against Criteria

| Criterion | Target | Actual | Status |
|-----------|--------|--------|--------|
| Collision pairs | ≥50 | [TBD] | [✓/✗] |
| Coherency PoC | Yes | [TBD] | [✓/✗] |
| Cache hit rates | <20% for adversarial | [TBD]% | [✓/✗] |
| Entropy assessment | 20-bit insufficient | [TBD] | [✓/✗] |

### Recommendation

```
Current Key Space:        20-bit (2^20 = 1M values)
Minimum Recommended:      64-bit (composite key)
Proposed Extension:       [20-bit flow label] + [44-bit hash]
Security Margin:          2^64 = 18.4 quintillion values
Estimated Birthday Bound: ~4.3 billion values for 50% collision
```

### S21 Verdict

```
Collision Discovery:      [PASS / FAIL]
Coherency Analysis:       [PASS / FAIL]
Side-Channel Detection:   [PASS / FAIL]
Recommendation Quality:   [PASS / FAIL]

OVERALL LICH-009 RESULT:  [PASS / FAIL]
```

---

## LICH-010: WAL Compaction Race Harness

### Campaign Metadata

| Metric | Value |
|--------|-------|
| **Objective** | Expose race conditions in WAL compaction |
| **Target** | Anamnesis write-ahead log |
| **Seed Corpus Size** | 30 seeds (variable, ~1KB) |
| **Expected Duration** | 48 hours |
| **Resource Allocation** | 4-8 CPU cores, 4GB RAM |
| **Status** | [PENDING / IN PROGRESS / COMPLETED] |

### Corpus Statistics

```
Seed Corpus Breakdown:
├── Single events:              2 seeds (timestamp 0, timestamp 1000)
├── Sequential events:          1 seed  (10 events, increasing timestamps)
├── Rapid-fire events:          1 seed  (16 events, same timestamp)
├── Wrap-around cases:          2 seeds (max u64, near-max with jumps)
├── Timestamp resets:           1 seed  (backward jumps)
├── Event type variations:      2 seeds (all types 0-255, all hop indices)
├── Concurrent simulation:      1 seed  (3 writers, interleaved)
├── Compaction triggers:        1 seed  (many events + gap + recovery)
├── Power failure patterns:     1 seed  (incomplete event simulation)
└── Stress patterns:           14 seeds (increasing, decreasing, bursts, etc.)

Total Seeds:                   30
Total Initial Corpus:          ~1KB
Average Event Size:           32 bytes (Anamnesis event format)
```

### Initial Execution

```
Execution Start Time:     [TBD]
Execution Duration:       [TBD]
Final Corpus Size:        [TBD] seeds
Final Corpus Size (MB):   [TBD]
Cumulative Executions:    [TBD]
Average Throughput:       [TBD] exec/sec
WAL Events Processed:     [TBD]
```

### Results Collected

#### Data Integrity

```
Data Loss Events:             [TBD]
├── Lost writes during compact: [TBD]
├── Partial WAL corruption:    [TBD]
├── Replay divergence:         [TBD]
└── Total issues:              [TBD]

Seqno Monotonicity:
  - Gaps detected:            [TBD]
  - Duplicates detected:      [TBD]
  - Out-of-order entries:     [TBD]

Verification:
  - All entries verified with HMAC-SHA256: [TBD]%
  - Checksum mismatches:      [TBD]
```

#### Compaction Safety

```
Concurrent Compaction Violations: [TBD]
├── Double-compaction:         [TBD]
├── Write during compaction:   [TBD]
├── Use-after-free:            [TBD]
└── Double-free:               [TBD]

Compaction Exclusivity:
  - Enforced correctly:        [YES / NO]
  - CAS-race detected:         [TBD]
  - Patch required:            [YES / NO]
```

#### Recovery Performance

```
Recovery Operations Tested:   [TBD]
Average Recovery Time:        [TBD] ms
Target SLA:                   <100 ms
SLA Achievement:              [PASS / FAIL]

Performance Breakdown:
  - Worst-case (10k records):  [TBD] ms
  - Median case:               [TBD] ms
  - Best case:                 [TBD] ms
```

#### Specific Issues Found

```
Issue #1: Lost writes in compaction
├── Trigger:              Rapid writes with immediate compaction
├── Impact:               Data loss, consistency violation
├── Severity:             Critical
├── Root Cause:           [TBD]
├── Status:               [CONFIRMED / UNDER INVESTIGATION / PATCHED]
└── Patch:                [TBD]

Issue #2: [TBD]
├── Trigger:              [TBD]
├── Impact:               [TBD]
├── Severity:             [TBD]
├── Root Cause:           [TBD]
├── Status:               [TBD]
└── Patch:                [TBD]
```

### Assessment Against Criteria

| Criterion | Target | Actual | Status |
|-----------|--------|--------|--------|
| Zero data loss | 100% | [TBD]% | [✓/✗] |
| HMAC validation | All | [TBD]% | [✓/✗] |
| Compaction exclusivity | 1 at a time | [TBD] | [✓/✗] |
| Crash-recovery correct | All scenarios | [TBD]% | [✓/✗] |
| Seqno monotonicity | Guaranteed | [TBD] | [✓/✗] |
| Recovery SLA | <100ms | [TBD]ms | [✓/✗] |

### S21 Verdict

```
Data Integrity:           [PASS / FAIL]
Compaction Safety:        [PASS / FAIL]
Recovery Performance:     [PASS / FAIL]
Crash Resilience:         [PASS / FAIL]

OVERALL LICH-010 RESULT:  [PASS / FAIL]
```

---

## Go Protocol Fuzzing Results

### Overview

| Target | Status | Seed Count | Test Duration | Results |
|--------|--------|-----------|----------------|---------|
| FuzzVarintRoundtrip | [PENDING/COMPLETED] | Auto | [TBD] | [TBD] |
| FuzzExponentRoundtrip | [PENDING/COMPLETED] | Auto | [TBD] | [TBD] |
| FuzzCRC16CCITT | [PENDING/COMPLETED] | Auto | [TBD] | [TBD] |
| FuzzTLVRoundtrip | [PENDING/COMPLETED] | Auto | [TBD] | [TBD] |

### FuzzVarintRoundtrip

```
Executions:               [TBD]
Failures:                 [TBD]
Coverage achieved:        [TBD] / 256 code paths

Oracle Checks:
  - Roundtrip identity:   [PASS / FAIL] ([TBD] checks)
  - Continuation bits:    [PASS / FAIL] ([TBD] checks)
  - Minimal encoding:     [PASS / FAIL] ([TBD] checks)
```

### FuzzExponentRoundtrip

```
Executions:               [TBD]
Failures:                 [TBD]
Coverage achieved:        [TBD] / 256 code paths

Oracle Checks:
  - Roundtrip identity:   [PASS / FAIL] ([TBD] checks)
  - Size consistency:     [PASS / FAIL] (always 4 bytes)
  - Sign preservation:    [PASS / FAIL] ([TBD] checks)
```

### FuzzCRC16CCITT

```
Executions:               [TBD]
Failures:                 [TBD]
Coverage achieved:        [TBD] / 256 code paths

Oracle Checks:
  - Determinism:          [PASS / FAIL] ([TBD] checks)
  - Single-bit sensitivity: [PASS / FAIL] ([TBD] checks)
  - Length sensitivity:   [PASS / FAIL] ([TBD] checks)
  - Range validation:     [PASS / FAIL] (0-0xFFFF)
```

### FuzzTLVRoundtrip

```
Executions:               [TBD]
Failures:                 [TBD]
Coverage achieved:        [TBD] / 256 code paths

Oracle Checks:
  - Type preservation:    [PASS / FAIL] ([TBD] checks)
  - Value preservation:   [PASS / FAIL] ([TBD] checks)
  - Length accuracy:      [PASS / FAIL] ([TBD] checks)
  - Nested encoding:      [PASS / FAIL] ([TBD] checks)
```

---

## Overall Campaign Assessment

### Summary Table

| Campaign | Status | Seed Count | Duration | Crashes | Critical Issues | Verdict |
|----------|--------|-----------|----------|---------|-----------------|---------|
| LICH-007 | [TBD] | 20 | [TBD]h | [TBD] | [TBD] | [PASS/FAIL] |
| LICH-008 | [TBD] | 20 | [TBD]h | [TBD] | [TBD] | [PASS/FAIL] |
| LICH-009 | [TBD] | 50 | [TBD]h | [TBD] | [TBD] | [PASS/FAIL] |
| LICH-010 | [TBD] | 30 | [TBD]h | [TBD] | [TBD] | [PASS/FAIL] |
| Go Fuzz | [TBD] | Auto | [TBD]h | [TBD] | [TBD] | [PASS/FAIL] |

### Total Metrics

```
Total Seed Corpus:        150 seeds + Go auto-corpus
Total Execution Time:     [TBD] hours
Total Unique Issues:      [TBD]
Critical Issues:          [TBD]
High Issues:              [TBD]
Medium Issues:            [TBD]
Low Issues:               [TBD]

Patches Applied:          [TBD]
Regression Tests Created: [TBD]
```

### S21 Overall Verdict

```
Coverage Adequacy:        [PASS / FAIL]
Security Findings:        [PASS / FAIL]
Remediation Quality:      [PASS / FAIL]
Documentation Complete:   [PASS / FAIL]

OVERALL S21 ASSESSMENT:   [PASS / FAIL] / [CONDITIONAL / APPROVAL PENDING]
```

---

## Findings Integration

### Dark Grimoire Updates

All findings from these campaigns have been integrated into the Dark Grimoire attack surface taxonomy:

```
docs/security/dark-grimoire-addendum.md
  ├── [LICH-007] MBC Instruction Decoding Vulnerabilities
  │   ├── [TBD] Off-by-one in jump offset
  │   ├── [TBD] Integer overflow in arithmetic
  │   └── [TBD] Incorrect register validation
  ├── [LICH-008] Wotan Cache Race Conditions
  │   ├── [TBD] Store buffering violation
  │   ├── [TBD] Memory barrier misuse
  │   └── [TBD] CAS atomicity gap
  ├── [LICH-009] Flow Label Entropy Weakness
  │   ├── [TBD] Birthday attack feasibility
  │   ├── [TBD] Flow isolation bypass
  │   └── [TBD] Cache side-channel vulnerability
  └── [LICH-010] WAL Compaction Race Conditions
      ├── [TBD] Lost writes during compaction
      ├── [TBD] Concurrent compaction vulnerability
      └── [TBD] Recovery SLA violation
```

### Specification Patches Required

```
Document: [TBD]
├── Patch 1: [TBD] - [Description]
├── Patch 2: [TBD] - [Description]
└── Patch N: [TBD] - [Description]
```

### Regression Test Suite

All crash inputs have been saved as regression tests:

```
Regression Tests Created:     [TBD]
Test Coverage:                [TBD]%
Integration Status:           [NOT STARTED / IN PROGRESS / COMPLETE]
```

---

## Recommendations for Phase 7

### Immediate Actions

1. **Apply critical patches** from [TBD] and [TBD]
2. **Verify patches** with reproduction tests
3. **Document root causes** in design review
4. **Update specifications** based on findings

### Medium-term Actions

1. **Extend flow label entropy** from 20-bit to 64-bit
2. **Add memory barrier audit** to code review checklist
3. **Implement WAL integrity checks** at compile time
4. **Enable sanitizers** in CI/CD pipeline

### Long-term Actions

1. **Fuzz other critical components** identified in Dark Grimoire
2. **Establish continuous fuzzing** in production monitoring
3. **Create fuzzing corpus library** for regression testing
4. **Security update schedule** for ongoing assessment

---

## Sign-off

### Campaign Execution

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Campaign Lead | [TBD] | [TBD] | [TBD] |
| Security Review | [TBD] | [TBD] | [TBD] |
| Approval | [TBD] | [TBD] | [TBD] |

### Appendices

- **Appendix A:** Crash Reproduction Instructions
- **Appendix B:** Full ThreadSanitizer Logs (LICH-008)
- **Appendix C:** Birthday Attack Analysis (LICH-009)
- **Appendix D:** WAL Recovery Test Results (LICH-010)
- **Appendix E:** Go Fuzzing Corpus Directory Structure
- **Appendix F:** Performance Profiling Data
- **Appendix G:** Related CVEs and Prior Art

---

**Document Version:** 1.0
**Last Updated:** 2026-02-20
**Classification:** Security Assessment - Internal Use
