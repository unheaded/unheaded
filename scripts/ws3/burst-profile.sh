#!/usr/bin/env bash
# WS3 Phase 4: Burst Injection Profiling
#
# Tests Netflix-model burst injection: fire N packets with zero delay,
# wait for XDP ring drain, repeat. Expected: 2-3x throughput improvement.
#
# Usage:
#   sudo ./scripts/ws3/burst-profile.sh
#   sudo ./scripts/ws3/burst-profile.sh --batches "50 100 200 500"
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
BATCHES="50 100 200 500"
IFACE="veth01"
NAMESPACE="monad0"
SUSTAINED_SECONDS=60

# Parse arguments.
while [[ $# -gt 0 ]]; do
    case "$1" in
        --count) COUNT="$2"; shift 2 ;;
        --batches) BATCHES="$2"; shift 2 ;;
        --iface) IFACE="$2"; shift 2 ;;
        --namespace) NAMESPACE="$2"; shift 2 ;;
        --sustained) SUSTAINED_SECONDS="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 [--count N] [--batches 'B1 B2 ...'] [--sustained SECS]"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

echo "=== WS3 Phase 4: Burst Injection Profiling ==="
echo "  Count per test: $COUNT"
echo "  Batch sizes:    $BATCHES"
echo "  Sustained test: ${SUSTAINED_SECONDS}s"
echo ""

HAS_NAMESPACE=false
if ip netns list 2>/dev/null | grep -q "$NAMESPACE"; then
    HAS_NAMESPACE=true
fi

# ----- Step 4.1-4.3: Binary-search optimal batch size -----
echo "--- Steps 4.1-4.3: Burst Size Sweep ---"

declare -A BURST_RESULTS
OPTIMAL_BATCH=100
OPTIMAL_PPS=0

for BATCH in $BATCHES; do
    echo "  Testing batch_size=$BATCH..."
    LOG_FILE="$RESULTS_DIR/burst-test-${BATCH}.log"

    if [ "$HAS_NAMESPACE" = true ]; then
        ip netns exec "$NAMESPACE" python3 "$PROJECT_ROOT/scripts/doom/inject.py" \
            --count "$COUNT" \
            --mode burst \
            --batch-size "$BATCH" \
            --iface "$IFACE" \
            2>&1 | tee "$LOG_FILE"

        sleep 2

        # Check for faults.
        if grep -q "ROM FAULT" "$LOG_FILE" 2>/dev/null; then
            echo "    FAULT detected at batch_size=$BATCH"
            BURST_RESULTS[$BATCH]="FAULT:0"
            break
        fi

        # Extract PPS.
        PPS=$(grep "Injected" "$LOG_FILE" 2>/dev/null | grep -oP '\d+ pkt/s' | grep -oP '^\d+' || echo "0")
        BURST_RESULTS[$BATCH]="OK:$PPS"
        echo "    Throughput: $PPS pps"

        if [ "$PPS" -gt "$OPTIMAL_PPS" ] 2>/dev/null; then
            OPTIMAL_PPS="$PPS"
            OPTIMAL_BATCH="$BATCH"
        fi
    else
        echo "    [SKIPPED] Namespace not available"
        BURST_RESULTS[$BATCH]="SKIPPED:0"
    fi
done

echo ""
echo "  Optimal batch size: $OPTIMAL_BATCH ($OPTIMAL_PPS pps)"
echo ""

# ----- Step 4.4: Sustained Burst Test -----
echo "--- Step 4.4: Sustained Burst Test (${SUSTAINED_SECONDS}s) ---"

SUSTAINED_LOG="$RESULTS_DIR/burst-test-sustained-${SUSTAINED_SECONDS}s.log"

if [ "$HAS_NAMESPACE" = true ]; then
    SUSTAINED_COUNT=$((OPTIMAL_PPS * SUSTAINED_SECONDS * 2))
    if [ "$SUSTAINED_COUNT" -lt 10000 ]; then
        SUSTAINED_COUNT=10000
    fi

    echo "  Injecting ~$SUSTAINED_COUNT packets (batch=$OPTIMAL_BATCH) for ${SUSTAINED_SECONDS}s..."

    START_TIME=$(date +%s)
    timeout $((SUSTAINED_SECONDS + 5)) \
        ip netns exec "$NAMESPACE" python3 "$PROJECT_ROOT/scripts/doom/inject.py" \
            --count "$SUSTAINED_COUNT" \
            --mode burst \
            --batch-size "$OPTIMAL_BATCH" \
            --iface "$IFACE" \
            2>&1 | tee "$SUSTAINED_LOG" || true
    END_TIME=$(date +%s)
    ACTUAL_ELAPSED=$((END_TIME - START_TIME))

    echo ""
    echo "  Actual elapsed: ${ACTUAL_ELAPSED}s"

    FAULT_COUNT=$(grep -c "ROM FAULT" "$SUSTAINED_LOG" 2>/dev/null || echo 0)
    if [ "$FAULT_COUNT" -gt 0 ]; then
        echo "  WARNING: $FAULT_COUNT ROM faults during sustained run"
    else
        echo "  OK: No ROM faults during sustained run"
    fi
else
    echo "  [SKIPPED] Namespace not available"
fi

# ----- Step 4.5: Compare Burst vs Steady-State -----
echo ""
echo "--- Step 4.5: Burst vs Steady-State Comparison ---"

COMPARISON_FILE="$RESULTS_DIR/burst-comparison.txt"
{
    echo "=== Burst vs Steady-State Comparison ==="
    echo ""

    # Read baseline PPS if available.
    BASELINE_LOG="$RESULTS_DIR/baseline-inject-3000us.log"
    BASELINE_PPS=0
    if [ -f "$BASELINE_LOG" ]; then
        BASELINE_PPS=$(grep "Injected" "$BASELINE_LOG" 2>/dev/null | grep -oP '\d+ pkt/s' | grep -oP '^\d+' || echo "0")
    fi

    echo "Baseline (3000us steady): $BASELINE_PPS pps"
    echo ""

    printf "%-12s %-10s %-10s %-10s\n" "Batch Size" "Status" "PPS" "vs Baseline"
    printf "%s\n" "-------------------------------------------"

    for BATCH in $BATCHES; do
        VAL="${BURST_RESULTS[$BATCH]:-}"
        if [ -n "$VAL" ]; then
            IFS=: read -r STATUS PPS <<< "$VAL"
            if [ "$STATUS" = "OK" ] && [ "$PPS" != "0" ] && [ "$BASELINE_PPS" != "0" ]; then
                IMPROVEMENT=$(echo "scale=0; ($PPS * 100 / $BASELINE_PPS) - 100" | bc -l 2>/dev/null || echo "?")
                printf "%-12s %-10s %-10s +%s%%\n" "$BATCH" "$STATUS" "$PPS" "$IMPROVEMENT"
            else
                printf "%-12s %-10s %-10s %-10s\n" "$BATCH" "$STATUS" "$PPS" "-"
            fi
        fi
    done

    echo ""
    echo "H2 Validation (burst yields 2-3x improvement):"
    if [ "$BASELINE_PPS" != "0" ] && [ "$OPTIMAL_PPS" != "0" ]; then
        RATIO=$(echo "scale=1; $OPTIMAL_PPS / $BASELINE_PPS" | bc -l 2>/dev/null || echo "?")
        echo "  Best burst: $OPTIMAL_PPS pps (${RATIO}x baseline)"
        if [ "$(echo "$RATIO >= 2" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
            echo "  H2 CONFIRMED: Burst yields 2x+ throughput"
        elif [ "$(echo "$RATIO >= 1.5" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
            echo "  H2 PARTIAL: Burst yields ${RATIO}x (target 2x+)"
        else
            echo "  H2 UNRESOLVED: Burst only ${RATIO}x improvement"
        fi
    else
        echo "  [Cannot validate: missing baseline or burst data]"
    fi
} | tee "$COMPARISON_FILE"

echo ""
echo "=== Phase 4 Complete ==="
echo "Results saved to $RESULTS_DIR/"
