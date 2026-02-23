#!/usr/bin/env bash
# WS3 Phase 5: Python vs Go Injector Comparison
#
# Runs the same burst injection test with both the Python inject.py and
# the Go doom-go-injector, then compares throughput.
#
# Usage:
#   sudo ./scripts/ws3/compare-injectors.sh
#
# Prerequisites:
#   - Go injector built: go build -o /tmp/doom-go-injector ./cmd/doom-go-injector
#   - Doom ring deployed
#   - monad0 namespace active

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
RESULTS_DIR="/tmp/ws3-results"
mkdir -p "$RESULTS_DIR"

COUNT=1000
BATCH=100
IFACE="veth01"
NAMESPACE="monad0"

# Build Go injector if not present.
GO_INJECTOR="/tmp/doom-go-injector"
if [ ! -x "$GO_INJECTOR" ]; then
    echo "Building Go injector..."
    (cd "$PROJECT_ROOT" && go build -o "$GO_INJECTOR" ./cmd/doom-go-injector/)
    echo "Built: $GO_INJECTOR"
fi

echo "=== WS3 Phase 5: Python vs Go Injector Comparison ==="
echo "  Count:      $COUNT"
echo "  Batch size: $BATCH"
echo "  Interface:  $IFACE"
echo ""

HAS_NAMESPACE=false
if ip netns list 2>/dev/null | grep -q "$NAMESPACE"; then
    HAS_NAMESPACE=true
fi

# ----- Python burst test -----
echo "--- Python Injector (burst, batch=$BATCH) ---"
PY_LOG="$RESULTS_DIR/compare-python-burst.log"
PY_PPS=0

if [ "$HAS_NAMESPACE" = true ]; then
    ip netns exec "$NAMESPACE" python3 "$PROJECT_ROOT/scripts/doom/inject.py" \
        --count "$COUNT" \
        --mode burst \
        --batch-size "$BATCH" \
        --iface "$IFACE" \
        2>&1 | tee "$PY_LOG"

    sleep 2
    PY_PPS=$(grep "Injected" "$PY_LOG" 2>/dev/null | grep -oP '\d+ pkt/s' | grep -oP '^\d+' || echo "0")
    echo "  Python PPS: $PY_PPS"
else
    echo "  [SKIPPED] Namespace not available"
fi

echo ""

# ----- Go burst test -----
echo "--- Go Injector (burst, batch=$BATCH) ---"
GO_LOG="$RESULTS_DIR/compare-go-burst.log"
GO_PPS=0

if [ "$HAS_NAMESPACE" = true ]; then
    ip netns exec "$NAMESPACE" "$GO_INJECTOR" \
        -count "$COUNT" \
        -batch "$BATCH" \
        -mode burst \
        -iface "$IFACE" \
        2>&1 | tee "$GO_LOG"

    sleep 2
    GO_PPS=$(grep "Injected" "$GO_LOG" 2>/dev/null | grep -oP '\d+ pkt/s' | grep -oP '^\d+' || echo "0")
    echo "  Go PPS: $GO_PPS"
else
    echo "  [SKIPPED] Namespace not available"
fi

echo ""

# ----- Comparison -----
echo "--- Comparison ---"
COMP_FILE="$RESULTS_DIR/injector-comparison.txt"
{
    echo "=== Python vs Go Injector Comparison ==="
    echo "Date: $(date +%Y-%m-%d)"
    echo ""
    echo "Test parameters:"
    echo "  Count: $COUNT packets"
    echo "  Mode: burst (batch=$BATCH)"
    echo "  Interface: $IFACE"
    echo ""
    echo "Results:"
    echo "  Python injector: $PY_PPS pps"
    echo "  Go injector:     $GO_PPS pps"

    if [ "$PY_PPS" != "0" ] && [ "$GO_PPS" != "0" ]; then
        IMPROVEMENT=$(echo "scale=0; ($GO_PPS * 100 / $PY_PPS) - 100" | bc -l 2>/dev/null || echo "?")
        RATIO=$(echo "scale=1; $GO_PPS / $PY_PPS" | bc -l 2>/dev/null || echo "?")
        echo "  Improvement:     +${IMPROVEMENT}% (${RATIO}x)"
        echo ""
        echo "H3 Validation (Go yields 10x+ throughput):"
        if [ "$(echo "$RATIO >= 10" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
            echo "  H3 CONFIRMED: Go yields 10x+ throughput"
        elif [ "$(echo "$RATIO >= 5" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
            echo "  H3 PARTIAL: Go yields ${RATIO}x (target 10x+)"
        else
            echo "  H3 NEEDS WORK: Go only ${RATIO}x improvement"
            echo "  Note: AF_PACKET sendto is faster than Python socket.send()"
            echo "  but further optimization (sendmmsg, TPACKET_V3) may be needed."
        fi
    else
        echo ""
        echo "  [Cannot compute improvement: missing data]"
    fi
} | tee "$COMP_FILE"

echo ""
echo "=== Phase 5 Comparison Complete ==="
echo "Results saved to $COMP_FILE"
