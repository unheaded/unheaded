# LICH-007: 72-Hour Fuzz Campaign Results

**Campaign ID:** LICH-007
**Session:** S30
**Start Time:** 2026-02-21 03:38 UTC
**Campaign Duration:** 72 hours
**Targets:** 3 (fuzz_mbc_decode, fuzz_mbc_execute, fuzz_mbc_roundtrip)
**Status:** In Progress

---

## Campaign Configuration

| Target | Fuzzer | Strategy | Duration | Start Timestamp |
|--------|--------|----------|----------|-----------------|
| fuzz_mbc_decode | libfuzzer | coverage-guided | 72h | 2026-02-21 03:38 UTC |
| fuzz_mbc_execute | libfuzzer | coverage-guided | 72h | 2026-02-21 03:38 UTC |
| fuzz_mbc_roundtrip | libfuzzer | coverage-guided | 72h | 2026-02-21 03:38 UTC |

---

## Final Results

| Target | Total Executions | Exec/sec (avg) | Peak Exec/sec | Edge Coverage | Corpus Size | Crashes | Timeouts |
|--------|------------------|----------------|---------------|---------------|-------------|---------|----------|
| fuzz_mbc_decode | __FILL__ | __FILL__ | __FILL__ | __FILL__ | __FILL__ | __FILL__ | __FILL__ |
| fuzz_mbc_execute | __FILL__ | __FILL__ | __FILL__ | __FILL__ | __FILL__ | __FILL__ | __FILL__ |
| fuzz_mbc_roundtrip | __FILL__ | __FILL__ | __FILL__ | __FILL__ | __FILL__ | __FILL__ | __FILL__ |

---

## Initial Readings (S30 Snapshot - Before 72h Campaign)

### fuzz_mbc_decode
- **Total Executions:** 536M+
- **Avg Throughput:** 1.57M/s
- **Edge Coverage:** 14 edges
- **Corpus Size:** 1
- **Crashes:** 0

### fuzz_mbc_execute
- **Total Executions:** 9.4M+
- **Avg Throughput:** 13.7K/s
- **Edge Coverage:** 366 edges
- **Corpus Size:** 659
- **Crashes:** 0

### fuzz_mbc_roundtrip
- **Total Executions:** 536M+
- **Avg Throughput:** 1.26M/s
- **Edge Coverage:** 62 edges
- **Corpus Size:** 41
- **Crashes:** 0

---

## Coverage Analysis

Coverage data will be collected using:

```bash
cargo +nightly fuzz coverage fuzz_mbc_execute
```

**Instructions:**

1. After campaign completion, run coverage analysis on primary target (fuzz_mbc_execute)
2. Compare edge coverage growth against baseline (366 edges at start)
3. Identify uncovered code paths in monad-mbc decode/execute/roundtrip logic
4. Generate coverage report for merge requirements

**Expected Coverage Metrics:**
- Line coverage: __FILL__%
- Function coverage: __FILL__%
- New paths discovered: __FILL__

---

## Crash Analysis

**Expected Result:** No crashes (monad-mbc is hardened against adversarial input)

### Crashes Found
__FILL__ (expected: none)

### Notable Timeouts
__FILL__ (if any)

### Memory Issues
__FILL__ (heap corruption, OOM, etc - expected: none)

---

## Corpus Analysis

### Corpus Growth Trajectory
| Target | Initial Corpus | Final Corpus | New Seeds | Growth % |
|--------|----------------|--------------|-----------|----------|
| fuzz_mbc_decode | 1 | __FILL__ | __FILL__ | __FILL__%
| fuzz_mbc_execute | 659 | __FILL__ | __FILL__ | __FILL__%
| fuzz_mbc_roundtrip | 41 | __FILL__ | __FILL__ | __FILL__%

### Artifact Analysis
Check artifacts directory for interesting failure cases:

```bash
ls -la crates/monad-mbc/fuzz/artifacts/
```

**Interesting Cases Found:**
- __FILL__ (document any novel input patterns)

### Corpus Size Impact
```bash
du -sh crates/monad-mbc/fuzz/corpus/
```

Final size: __FILL__

---

## Severity Assessment

### Risk Rating: __FILL__ (GREEN/YELLOW/RED)

**Justification:**
- Crash count: __FILL__ (0 expected)
- Memory safety issues: __FILL__ (none expected)
- Logic errors: __FILL__ (none expected)
- Performance degradation: __FILL__ (none expected)

### Affected Components
- monad-mbc decode: __FILL__
- monad-mbc execute: __FILL__
- monad-mbc roundtrip: __FILL__

---

## Recommendations for Next Campaign

### LICH-008 (Post-Analysis)

**Scope Expansion:**
- [ ] Add fuzz_message_serialization target (wire format fuzzing)
- [ ] Add fuzz_state_machine target (state transition fuzzing)
- [ ] Increase duration to 96h for deeper coverage
- [ ] Add cross-target fuzzing (combining decode/execute output)

**Hardening Priorities:**
1. __FILL__ (based on coverage gaps)
2. __FILL__
3. __FILL__

**Monitor:**
- Edge coverage growth rate (diminishing returns expected)
- Corpus diversity (ensure new seeds are semantically different)
- Throughput stability (watch for memory leaks slowing fuzzer)

---

## Commands to Collect Final Stats

Run these on dev machine after campaign completes:

```bash
# Get final execution counts from log files
tail -3 /tmp/lich007-decode.log
tail -3 /tmp/lich007-execute.log
tail -3 /tmp/lich007-roundtrip.log

# Check artifacts (crashes/timeouts)
ls -la crates/monad-mbc/fuzz/artifacts/

# Check corpus sizes
du -sh crates/monad-mbc/fuzz/corpus/fuzz_mbc_decode/
du -sh crates/monad-mbc/fuzz/corpus/fuzz_mbc_execute/
du -sh crates/monad-mbc/fuzz/corpus/fuzz_mbc_roundtrip/

# Generate coverage report
cargo +nightly fuzz coverage fuzz_mbc_execute
```

---

## Campaign Notes

**Observations during campaign:**
- __FILL__

**Environmental factors:**
- System stability: __FILL__
- CPU/memory utilization: __FILL__
- Thermal issues: __FILL__

**Next session actions:**
- [ ] Review coverage gaps
- [ ] Plan LICH-008 scope
- [ ] Archive corpus snapshots
- [ ] Update mutation strategy if needed

---

**Last Updated:** 2026-02-21
**Campaign Status:** Running (ended 2026-02-24 03:38 UTC)
**Results Finalized:** __FILL__

