#!/usr/bin/env bash
# verify-mbc-pipeline.sh — Run all MBC assembler pipeline verification tests
# Phase 5 of S31 DOOM Battle Plan (Steps 136-160)
#
# Prerequisites:
#   - doom-ring.sh setup with BPF maps pinned
#   - mbc-asm binary built (cargo build --manifest-path crates/monad-mbc/Cargo.toml --bin mbc-asm --release)
#
# Usage: sudo ./scripts/verify-mbc-pipeline.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
ASM_DIR="$PROJECT_DIR/tests/mbc-pipeline"
MBC_ASM="$PROJECT_DIR/crates/monad-mbc/target/release/mbc-asm"
TMPDIR="/tmp/mbc-pipeline-test"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASS=0
FAIL=0

mkdir -p "$TMPDIR"

log_pass() { echo -e "${GREEN}[PASS]${NC} $1"; ((PASS++)); }
log_fail() { echo -e "${RED}[FAIL]${NC} $1"; ((FAIL++)); }
log_info() { echo -e "${YELLOW}[INFO]${NC} $1"; }

# Assemble, load, reset, inject, return CPU state
run_test() {
    local name="$1"
    local asm="$ASM_DIR/$name.asm"
    local mbc="$TMPDIR/$name.mbc"

    log_info "Testing: $name"

    # Assemble
    "$MBC_ASM" "$asm" -o "$mbc" 2>/dev/null || { log_fail "$name: assembly failed"; return 1; }

    # Load ROM
    python3 "$SCRIPT_DIR/load_rom.py" "$mbc" > /dev/null 2>&1

    # Reset CPU
    python3 "$SCRIPT_DIR/reset_cpu.py" > /dev/null 2>&1

    # Inject packet
    "$SCRIPT_DIR/doom-ring.sh" inject 0xDE > /dev/null 2>&1
    sleep 0.3

    # Read state
    python3 "$SCRIPT_DIR/read_cpu.py" 2>&1
}

echo "=== MBC Pipeline Verification (Phase 5, S31) ==="
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

# Test 1: Fibonacci
log_info "=== TEST 1: FIBONACCI ==="
STATE=$(run_test "test-fibonacci")
echo "$STATE"
if echo "$STATE" | grep -q "r0 = 55"; then
    log_pass "FIBONACCI GATE: r0 = 55 (fib(10))"
else
    log_fail "FIBONACCI GATE: r0 != 55"
fi
if echo "$STATE" | grep -q "halted = 1"; then
    log_pass "FIBONACCI: halted = 1"
else
    log_fail "FIBONACCI: not halted"
fi
echo ""

# Test 2: Memory
log_info "=== TEST 2: MEMORY ==="
STATE=$(run_test "test-memory")
echo "$STATE"
if echo "$STATE" | grep -q "r0 = 57005"; then
    log_pass "MEMORY GATE: r0 = 0xDEAD (57005)"
else
    log_fail "MEMORY GATE: r0 != 57005"
fi
echo ""

# Test 3: Keyboard
log_info "=== TEST 3: KEYBOARD ==="
# Inject key state first
sudo bpftool map update pinned /sys/fs/bpf/unheaded/doom-ring/maps/KBD_MAP key 0 0 0 0 value 131 0 0 0 2>/dev/null
STATE=$(run_test "test-keyboard")
echo "$STATE"
if echo "$STATE" | grep -q "r0 = 65" && echo "$STATE" | grep -q "r1 = 1"; then
    log_pass "KEYBOARD GATE: r0 = 0x41 (65), r1 = 1"
else
    log_fail "KEYBOARD GATE: unexpected r0/r1"
fi
echo ""

# Test 4: Branches
log_info "=== TEST 4: BRANCHES ==="
STATE=$(run_test "test-branches")
echo "$STATE"
if echo "$STATE" | grep -q "r0 = 4"; then
    log_pass "BRANCHES: all 4 conditional tests passed"
else
    log_fail "BRANCHES: r0 != 4"
fi
echo ""

# Test 5: Comprehensive
log_info "=== TEST 5: COMPREHENSIVE ISA ==="
STATE=$(run_test "test-comprehensive")
echo "$STATE"
if echo "$STATE" | grep -q "r0 = 42" && echo "$STATE" | grep -q "r1 = 7" && echo "$STATE" | grep -q "r2 = 2" && echo "$STATE" | grep -q "r7 = 99"; then
    log_pass "COMPREHENSIVE: MUL, DIV, MOD, nested CALL all correct"
else
    log_fail "COMPREHENSIVE: unexpected values"
fi
echo ""

echo "=== SUMMARY ==="
echo -e "  ${GREEN}Passed: $PASS${NC}"
echo -e "  ${RED}Failed: $FAIL${NC}"
echo ""

if [ "$FAIL" -eq 0 ]; then
    echo -e "${GREEN}ALL VERIFICATION GATES PASSED${NC}"
    exit 0
else
    echo -e "${RED}SOME GATES FAILED${NC}"
    exit 1
fi
