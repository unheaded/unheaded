#!/usr/bin/env bash
# WS3 Phase 3: Delay Profiling
#
# Binary-search for the minimum safe inter-packet delay.
# Tests delays: 3000, 2000, 1500, 1000, 750, 500 microseconds.
#
# Success criterion: No ROM faults, no instruction stalls.
#
# Usage:
#   sudo ./scripts/ws3/delay-profile.sh
#   sudo ./scripts/ws3/delay-profile.sh --count 500   # fewer packets per test
#   sudo ./scripts/ws3/delay-profile.sh --delays "3000 1000 500"  # custom delays
#
# Prerequisites:
#   - Doom ring deployed
#   - Running inside or with access to monad0 namespace

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
RESULTS_DIR="/tmp/ws3-results"
mkdir -p "$RESULTS_DIR"

# Defaults.
COUNT=1000
DELAYS="3000 2000 1500 1000 750 500"
IFACE="veth01"
NAMESPACE="monad0"

# Parse arguments.
while [[ $# -gt 0 ]]; do
    case "$1" in
        --count) COUNT="$2"; shift 2 ;;
        --delays) DELAYS="$2"; shift 2 ;;
        --iface) IFACE="$2"; shift 2 ;;
        --namespace) NAMESPACE="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 [--count N] [--delays 'D1 D2 ...'] [--iface IF] [--namespace NS]"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

echo "=== WS3 Phase 3: Delay Profiling ==="
echo "  Count per test: $COUNT"
echo "  Delays (us):    $DELAYS"
echo "  Interface:      $IFACE"
echo "  Namespace:      $NAMESPACE"
echo ""

# ----- Step 3.1: Define Test Matrix -----
echo "--- Step 3.1: Test Matrix ---"
MATRIX_FILE="$RESULTS_DIR/delay-test-matrix.txt"
cat > "$MATRIX_FILE" << EOF
Test Matrix: Delay Profiling
Date: $(date +%Y-%m-%d)

Config:
  - Packets per test: $COUNT
  - Interface: $IFACE
  - Namespace: $NAMESPACE
  - Success criterion: No ROM faults, no instruction stalls

Delays to test (us): $DELAYS

Expected rates (theoretical):
  3000us -> ~333 pps  -> ~6.7 fps
  2000us -> ~500 pps  -> ~10 fps
  1500us -> ~667 pps  -> ~13 fps
  1000us -> ~1000 pps -> ~20 fps
  750us  -> ~1333 pps -> ~27 fps
  500us  -> ~2000 pps -> ~40 fps

Note: Actual rates depend on Python socket.send() overhead and CPU execution efficiency.
EOF
cat "$MATRIX_FILE"
echo ""

# ----- Steps 3.2-3.7: Run Delay Tests -----
TEST_LABEL=A
declare -A RESULTS

for DELAY in $DELAYS; do
    echo "--- Step 3.${TEST_LABEL}: Delay Test $TEST_LABEL (${DELAY}us) ---"

    LOG_FILE="$RESULTS_DIR/delay-test-${TEST_LABEL}-${DELAY}us.log"

    # Check if namespace exists before attempting injection.
    if ip netns list 2>/dev/null | grep -q "$NAMESPACE"; then
        ip netns exec "$NAMESPACE" python3 "$PROJECT_ROOT/scripts/doom/inject.py" \
            --count "$COUNT" \
            --delay "$DELAY" \
            --mode steady \
            --iface "$IFACE" \
            2>&1 | tee "$LOG_FILE"

        sleep 2

        # Check for ROM faults.
        if grep -q "ROM FAULT" "$LOG_FILE" 2>/dev/null; then
            echo "  WARNING: ROM fault detected at ${DELAY}us!"
            RESULTS[$TEST_LABEL]="${DELAY}:FAULT"
        else
            # Extract PPS.
            PPS=$(grep "Injected" "$LOG_FILE" 2>/dev/null | grep -oP '\d+ pkt/s' | grep -oP '^\d+' || echo "0")
            RESULTS[$TEST_LABEL]="${DELAY}:OK:${PPS}"
            echo "  Result: OK, ${PPS} pps"
        fi
    else
        echo "  [SKIPPED] Namespace $NAMESPACE not available"
        RESULTS[$TEST_LABEL]="${DELAY}:SKIPPED"
    fi

    echo ""

    # Advance label A -> B -> C -> ...
    TEST_LABEL=$(echo "$TEST_LABEL" | tr 'A-E' 'B-F')
done

# ----- Step 3.8: Analysis -----
echo "--- Step 3.8: Analyze Results ---"

ANALYSIS_FILE="$RESULTS_DIR/delay-analysis.txt"
{
    echo "=== Delay Profile Results ==="
    echo ""
    printf "%-6s %-10s %-10s %-10s %-10s\n" "Test" "Delay(us)" "Status" "PPS" "Est.FPS"
    printf "%s\n" "----------------------------------------------"

    LABEL=A
    for DELAY in $DELAYS; do
        VAL="${RESULTS[$LABEL]:-}"
        if [ -n "$VAL" ]; then
            IFS=: read -r D STATUS PPS <<< "$VAL"
            if [ "$STATUS" = "OK" ] && [ -n "$PPS" ] && [ "$PPS" != "0" ]; then
                # Estimate FPS: (PPS * 4080) / 1,470,000 insns per frame
                EST_FPS=$(echo "scale=1; $PPS * 4080 / 1470000" | bc -l 2>/dev/null || echo "?")
                printf "%-6s %-10s %-10s %-10s %-10s\n" "$LABEL" "$D" "$STATUS" "$PPS" "$EST_FPS"
            else
                printf "%-6s %-10s %-10s %-10s %-10s\n" "$LABEL" "$D" "$STATUS" "-" "-"
            fi
        else
            printf "%-6s %-10s %-10s %-10s %-10s\n" "$LABEL" "$DELAY" "N/A" "-" "-"
        fi
        LABEL=$(echo "$LABEL" | tr 'A-E' 'B-F')
    done
} | tee "$ANALYSIS_FILE"

# ----- Step 3.9: Document Findings -----
echo ""
echo "--- Step 3.9: Findings ---"

FINDINGS_FILE="$RESULTS_DIR/ws3-phase3-findings.txt"
cat > "$FINDINGS_FILE" << EOF
=== Phase 3: Delay Profile Findings ===
Date: $(date +%Y-%m-%d)

Testing protocol: $COUNT-packet injections at [$DELAYS]us delays.

Hypothesis: H1 (delay compensates for Python overhead, not XDP limits)
Status: [To be filled after reviewing $ANALYSIS_FILE]

If all delays work without fault:
  -> H1 CONFIRMED: Python socket.send() is the bottleneck.
  -> Recommendation: Proceed to Phase 4 (burst) and Phase 5 (Go rewrite).

If faults occur above threshold:
  -> H1 PARTIALLY CONFIRMED: XDP has cycle limits.
  -> Minimum safe delay: [threshold]
  -> Recommendation: Stay within margin. Burst model must respect bounce cycle.

If even 3000us (baseline) faults:
  -> H1 REJECTED: Different bottleneck identified.
  -> Debug: Check CPU_MAP state, STATS counters, RAM_MAP alignment.
EOF

echo "  Findings template written to $FINDINGS_FILE"
echo ""
echo "=== Phase 3 Complete ==="
echo "Results saved to $RESULTS_DIR/"
