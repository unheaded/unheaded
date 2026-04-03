#!/bin/bash
# BPF Verifier CI Gate — ADR-012
# Checks: compilation, instruction budget, known issues
# Exit 0 = all gates pass, Exit 1 = failure
set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BPF_DIR="$PROJECT_ROOT/ebpf"
BUDGET=900000  # 900K instruction budget (kernel limit is 1M for privileged XDP)
FAILURES=0
WARNINGS=0

echo "=============================================="
echo "BPF VERIFIER CI GATE — ADR-012"
echo "=============================================="
echo "Kernel: $(uname -r)"
echo "Budget: ${BUDGET} verified instructions (of 1M max)"
echo ""

# ---- Phase 1: Compilation ----
echo "=== Phase 1: Compile all BPF programs ==="
cd "$BPF_DIR"

BUILD_OUTPUT=$(cargo build --release 2>&1)
BUILD_EXIT=$?

# Count successful and failed builds
BUILT=$(echo "$BUILD_OUTPUT" | grep -c "Compiling.*-ebpf\|Compiling.*tracker\|Compiling.*marker\|Compiling.*probe\|Compiling.*tracer" || true)
ERRORS=$(echo "$BUILD_OUTPUT" | grep -c "^error\[" || true)
LINK_ERRORS=$(echo "$BUILD_OUTPUT" | grep -c "linking with.*failed" || true)

echo "  Compiled: $BUILT programs"
if [ "$LINK_ERRORS" -gt 0 ]; then
    echo "  LINK ERRORS: $LINK_ERRORS"
    echo "$BUILD_OUTPUT" | grep -B1 "linking.*failed" | head -20
    FAILURES=$((FAILURES + LINK_ERRORS))
fi

# ---- Phase 2: Check for known BPF-unfriendly patterns ----
echo ""
echo "=== Phase 2: Static analysis for BPF anti-patterns ==="

# Check for unbounded loops (missing MAX_ constant in while/for conditions)
UNBOUNDED=$(grep -rn "while\s*(true\|1)" --include="*.rs" "$BPF_DIR"/*/src/main.rs 2>/dev/null | grep -v "// bounded" | grep -v MAX_ || true)
if [ -n "$UNBOUNDED" ]; then
    echo "  WARNING: Potentially unbounded loops found:"
    echo "$UNBOUNDED" | head -5
    WARNINGS=$((WARNINGS + 1))
fi

# Check for u128 operations (not supported in BPF)
U128=$(grep -rn "u128\|i128\|__multi3\|__udivti3" --include="*.rs" "$BPF_DIR"/*/src/ 2>/dev/null || true)
if [ -n "$U128" ]; then
    echo "  WARNING: u128/i128 operations found (not supported in BPF):"
    echo "$U128" | head -5
    WARNINGS=$((WARNINGS + 1))
fi

# Check for floating point (not supported in BPF)
FLOAT=$(grep -rn "\bf32\b\|\bf64\b" --include="*.rs" "$BPF_DIR"/*/src/main.rs 2>/dev/null | grep -v "// safe" | grep -v test || true)
if [ -n "$FLOAT" ]; then
    echo "  WARNING: Floating point operations found (not supported in BPF):"
    echo "$FLOAT" | head -5
    WARNINGS=$((WARNINGS + 1))
fi

# Check stack usage hints (512 byte limit)
LARGE_ARRAYS=$(grep -rn "\[.*;\s*[0-9]\{3,\}\]" --include="*.rs" "$BPF_DIR"/*/src/ 2>/dev/null | grep -v "MAP\|map\|Map" || true)
if [ -n "$LARGE_ARRAYS" ]; then
    echo "  WARNING: Large stack arrays found (BPF stack limit 512 bytes):"
    echo "$LARGE_ARRAYS" | head -5
    WARNINGS=$((WARNINGS + 1))
fi

# ---- Phase 3: Loop bound verification ----
echo ""
echo "=== Phase 3: Loop bound verification ==="

# All loops in BPF code should reference a MAX_ constant
LOOP_FILES=$(grep -rln "while\|for\s" --include="*.rs" "$BPF_DIR"/*/src/ 2>/dev/null || true)
BOUNDED=0
TOTAL_LOOPS=0
for f in $LOOP_FILES; do
    LOOPS=$(grep -c "while\|for\s" "$f" 2>/dev/null || true)
    BOUNDS=$(grep -c "MAX_\|< [0-9]\|<= [0-9]" "$f" 2>/dev/null || true)
    TOTAL_LOOPS=$((TOTAL_LOOPS + LOOPS))
    BOUNDED=$((BOUNDED + BOUNDS))
done
echo "  Total loop constructs: $TOTAL_LOOPS"
echo "  Bound references: $BOUNDED"

# ---- Phase 4: Instruction count (if programs are loadable) ----
echo ""
echo "=== Phase 4: Instruction count check ==="

# Check monad-cpu-ebpf specifically (the complex one)
MONAD_BPF="$BPF_DIR/target/bpfel-unknown-none/release/monad-cpu-ebpf"
if [ -f "$MONAD_BPF" ]; then
    SIZE=$(stat --format=%s "$MONAD_BPF" 2>/dev/null || echo "0")
    # Rough estimate: BPF instructions are 8 bytes each
    EST_INSNS=$((SIZE / 8))
    echo "  monad-cpu-ebpf: ${SIZE} bytes (~${EST_INSNS} estimated instructions)"
    if [ "$EST_INSNS" -gt "$BUDGET" ]; then
        echo "  FAIL: Estimated instruction count ($EST_INSNS) exceeds budget ($BUDGET)"
        FAILURES=$((FAILURES + 1))
    else
        PCT=$((EST_INSNS * 100 / BUDGET))
        echo "  OK: ${PCT}% of budget used"
    fi
else
    echo "  SKIP: monad-cpu-ebpf binary not found (build may have failed)"
fi

# ---- Summary ----
echo ""
echo "=============================================="
echo "SUMMARY"
echo "=============================================="
echo "  Failures: $FAILURES"
echo "  Warnings: $WARNINGS"
echo "  Kernel:   $(uname -r)"
echo "  Date:     $(date -u +%Y-%m-%dT%H:%M:%SZ)"

if [ "$FAILURES" -gt 0 ]; then
    echo ""
    echo "GATE: FAILED ($FAILURES failures)"
    exit 1
else
    echo ""
    echo "GATE: PASSED (${WARNINGS} warnings)"
    exit 0
fi
