#!/usr/bin/env bash
# WS3 Phase 6: Sustained Playability Test
#
# Runs Doom for 60+ seconds at the best available injection rate,
# monitors for corruption (ROM faults, stalls), and reports FPS.
#
# Usage:
#   sudo ./scripts/ws3/sustained-test.sh
#   sudo ./scripts/ws3/sustained-test.sh --duration 120 --injector go
#
# Prerequisites:
#   - Doom ring deployed with ROM + WAD loaded
#   - monad0 namespace active

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
RESULTS_DIR="/tmp/ws3-results"
mkdir -p "$RESULTS_DIR"

DURATION=60
BATCH=100
IFACE="veth01"
NAMESPACE="monad0"
INJECTOR_TYPE="python"  # python or go
GO_INJECTOR="/tmp/doom-go-injector"

# Parse arguments.
while [[ $# -gt 0 ]]; do
    case "$1" in
        --duration) DURATION="$2"; shift 2 ;;
        --batch) BATCH="$2"; shift 2 ;;
        --iface) IFACE="$2"; shift 2 ;;
        --namespace) NAMESPACE="$2"; shift 2 ;;
        --injector) INJECTOR_TYPE="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 [--duration SECS] [--batch N] [--injector python|go]"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

echo "=== WS3 Phase 6: Sustained Playability Test ==="
echo "  Duration:  ${DURATION}s"
echo "  Batch:     $BATCH"
echo "  Injector:  $INJECTOR_TYPE"
echo ""

HAS_NAMESPACE=false
if ip netns list 2>/dev/null | grep -q "$NAMESPACE"; then
    HAS_NAMESPACE=true
fi

if [ "$HAS_NAMESPACE" = false ]; then
    echo "WARNING: Namespace $NAMESPACE not available."
    echo "This script requires a running doom ring."
    echo "To deploy: sudo $PROJECT_ROOT/scripts/doom-ring.sh"
    echo ""
    echo "Generating report template only..."
fi

# ----- Step 6.1: Start Doom Execution Driver -----
echo "--- Step 6.1: Start Doom Execution Driver ---"

DOOM_LOG="$RESULTS_DIR/doom-run-${DURATION}s.log"
DOOM_PID=""

if [ "$HAS_NAMESPACE" = true ] && [ -f "$PROJECT_ROOT/scripts/doom/run.py" ]; then
    python3 "$PROJECT_ROOT/scripts/doom/run.py" \
        --batch "$BATCH" \
        --forever \
        --namespace "$NAMESPACE" \
        --auto-restart \
        2>&1 > "$DOOM_LOG" &
    DOOM_PID=$!
    echo "  Doom driver PID: $DOOM_PID"
    sleep 5
else
    echo "  [SKIPPED] Doom driver not available"
fi

# ----- Step 6.2: Run Optimized Injector -----
echo ""
echo "--- Step 6.2: Inject for ${DURATION}s ---"

INJECT_LOG="$RESULTS_DIR/inject-sustained-${DURATION}s.log"
INJECT_COUNT=$((DURATION * 2000))  # Assume ~2000 pps max throughput.

if [ "$HAS_NAMESPACE" = true ]; then
    START_TIME=$(date +%s)

    if [ "$INJECTOR_TYPE" = "go" ] && [ -x "$GO_INJECTOR" ]; then
        echo "  Using Go injector: $GO_INJECTOR"
        timeout $((DURATION + 5)) \
            ip netns exec "$NAMESPACE" "$GO_INJECTOR" \
                -count "$INJECT_COUNT" \
                -batch "$BATCH" \
                -mode burst \
                -iface "$IFACE" \
                2>&1 | tee "$INJECT_LOG" || true
    else
        echo "  Using Python injector"
        timeout $((DURATION + 5)) \
            ip netns exec "$NAMESPACE" python3 "$PROJECT_ROOT/scripts/doom/inject.py" \
                --count "$INJECT_COUNT" \
                --mode burst \
                --batch-size "$BATCH" \
                --iface "$IFACE" \
                2>&1 | tee "$INJECT_LOG" || true
    fi

    ACTUAL_END=$(date +%s)
    ACTUAL_ELAPSED=$((ACTUAL_END - START_TIME))
    echo "  Actual elapsed: ${ACTUAL_ELAPSED}s"
else
    echo "  [SKIPPED] Namespace not available"
fi

# ----- Step 6.3: Monitor for Corruption -----
echo ""
echo "--- Step 6.3: Corruption Check ---"

FAULT_COUNT=0
STALL_COUNT=0

if [ -f "$DOOM_LOG" ]; then
    FAULT_COUNT=$(grep -c "ROM FAULT" "$DOOM_LOG" 2>/dev/null || echo 0)
    STALL_COUNT=$(grep -c "STALL DETECTED" "$DOOM_LOG" 2>/dev/null || echo 0)
fi

echo "  ROM faults:  $FAULT_COUNT"
echo "  Stalls:      $STALL_COUNT"

if [ "$FAULT_COUNT" -eq 0 ] && [ "$STALL_COUNT" -eq 0 ]; then
    echo "  No corruption detected"
else
    echo "  CORRUPTION DETECTED -- check $DOOM_LOG"
fi

# ----- Step 6.4: Extract FPS Metrics -----
echo ""
echo "--- Step 6.4: FPS Metrics ---"

FRAMES=0
FINAL_FPS="?"

if [ -f "$DOOM_LOG" ]; then
    FRAMES=$(grep -oP 'Frame \K\d+' "$DOOM_LOG" 2>/dev/null | tail -1 || echo "0")
    FINAL_FPS=$(grep -oP 'FPS:\s*\K[\d.]+' "$DOOM_LOG" 2>/dev/null | tail -1 || echo "?")
fi

echo "  Frames completed: $FRAMES"
echo "  Final FPS:        $FINAL_FPS"

# ----- Stop Doom driver -----
if [ -n "$DOOM_PID" ]; then
    kill "$DOOM_PID" 2>/dev/null || true
    wait "$DOOM_PID" 2>/dev/null || true
fi

# ----- Step 6.6: Generate Final Report -----
echo ""
echo "--- Step 6.6: Final Report ---"

REPORT_FILE="$RESULTS_DIR/ws3-final-report.txt"
cat > "$REPORT_FILE" << EOF
=== WS3: Scale to Playable -- Final Report ===

Date: $(date +%Y-%m-%d)
Target: 15+ fps sustained, zero corruption
Duration: ${DURATION}s

Phases Completed:
  [1] Baseline measurement:            [see baseline-report.txt]
  [2] BPF timestamp instrumentation:   [see bounce-timestamps-design.md]
  [3] Delay profile (3000-500us):      [see delay-analysis.txt]
  [4] Burst injection (50-500 batch):  [see burst-comparison.txt]
  [5] Go injector prototype:           [see injector-comparison.txt]
  [6] Sustained playability (${DURATION}s):  [this report]

Hypothesis Validation:
  H1 (delay as safety margin): [see phase3 findings]
  H2 (burst 2-3x gain):       [see phase4 comparison]
  H3 (Go 10x+ gain):          [see phase5 comparison]

Key Metrics:
  Baseline PPS (3000us):   ~333 pps
  Optimal delay found:     [from phase3]
  Burst PPS (optimal):     [from phase4]
  Go injector PPS:         [from phase5]
  Sustained FPS (${DURATION}s): $FINAL_FPS
  ROM faults (${DURATION}s):    $FAULT_COUNT
  Instruction stalls:      $STALL_COUNT
  Corruption detected:     $([ "$FAULT_COUNT" -eq 0 ] && [ "$STALL_COUNT" -eq 0 ] && echo "NO" || echo "YES")

Definition of Done:
  [ ] 15+ fps achieved
  [ ] Sustained for ${DURATION}+ seconds
  [ ] Zero ROM faults
  [ ] Zero instruction stalls
  [ ] Browser playback smooth
  [ ] All hypotheses validated

Recommendations:
  1. If Python injector >= 15fps: production ready with burst mode
  2. If only Go injector >= 15fps: deploy Go injector binary
  3. If neither >= 15fps: investigate XDP bounce cycle limits
  4. Next: WS4 Documentation & Conference Prep

Signed: Warmonger
Date: $(date +%Y-%m-%d)
EOF

echo "  Final report written to $REPORT_FILE"
cat "$REPORT_FILE"

echo ""
echo "=== Phase 6 Complete ==="
echo "All WS3 results saved to $RESULTS_DIR/"
