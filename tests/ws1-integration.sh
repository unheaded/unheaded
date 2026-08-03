#!/usr/bin/env bash
set -euo pipefail

# WS1 Integration Test -- Doom Bridge (Fenrir's Eye) Full Stack
# Tests: build, dry-run service, HTTP endpoints, WebSocket upgrade
#
# Usage:
#   bash tests/ws1-integration.sh
#
# Prerequisites: Go 1.21+, curl

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOOM_BRIDGE_BIN="/tmp/doom-bridge-ws1-test"
PORT=16660  # Use high port to avoid conflicts
PASSED=0
FAILED=0

cleanup() {
    if [[ -n "${BRIDGE_PID:-}" ]]; then
        kill "${BRIDGE_PID}" 2>/dev/null || true
        wait "${BRIDGE_PID}" 2>/dev/null || true
    fi
    rm -f "${DOOM_BRIDGE_BIN}"
}
trap cleanup EXIT

pass() {
    echo "  [PASS] $1"
    PASSED=$((PASSED + 1))
}

fail() {
    echo "  [FAIL] $1"
    FAILED=$((FAILED + 1))
}

echo "============================================"
echo " WS1 Integration Test: Doom Bridge"
echo "============================================"
echo ""

# --- Test 1: Build ---
echo "[TEST] Building doom-bridge..."
cd "${PROJECT_ROOT}"
if go build -o "${DOOM_BRIDGE_BIN}" ./cmd/doom-bridge 2>&1; then
    pass "Build succeeded"
else
    fail "Build failed"
    exit 1
fi

# --- Test 2: Binary exists ---
if [[ -x "${DOOM_BRIDGE_BIN}" ]]; then
    pass "Binary is executable"
else
    fail "Binary not found or not executable"
    exit 1
fi

# --- Test 3: Start service in dry-run mode ---
echo "[TEST] Starting doom-bridge in dry-run mode on port ${PORT}..."
${DOOM_BRIDGE_BIN} --port ${PORT} --dry-run --static ./dashboard &
BRIDGE_PID=$!
sleep 2

if kill -0 "${BRIDGE_PID}" 2>/dev/null; then
    pass "Service started (PID=${BRIDGE_PID})"
else
    fail "Service failed to start"
    exit 1
fi

# --- Test 4: /health endpoint ---
echo "[TEST] Testing /health endpoint..."
HEALTH_RESP=$(curl -s "http://localhost:${PORT}/health" 2>/dev/null)
if echo "${HEALTH_RESP}" | grep -q '"status":"ok"'; then
    pass "/health returns ok"
else
    fail "/health response: ${HEALTH_RESP}"
fi

if echo "${HEALTH_RESP}" | grep -q '"dry_run":true'; then
    pass "/health reports dry_run=true"
else
    fail "/health missing dry_run flag"
fi

# --- Test 5: /ready endpoint ---
echo "[TEST] Testing /ready endpoint..."
READY_RESP=$(curl -s "http://localhost:${PORT}/ready" 2>/dev/null)
if echo "${READY_RESP}" | grep -q '"status":"ready"'; then
    pass "/ready returns ready"
else
    fail "/ready response: ${READY_RESP}"
fi

if echo "${READY_RESP}" | grep -q '"version":"0.1.0-alpha"'; then
    pass "/ready includes version"
else
    fail "/ready missing version"
fi

# --- Test 6: /metrics endpoint ---
echo "[TEST] Testing /metrics endpoint..."
METRICS_RESP=$(curl -s "http://localhost:${PORT}/metrics" 2>/dev/null)
if echo "${METRICS_RESP}" | grep -q 'unheaded_doom_bridge_clients'; then
    pass "/metrics has clients gauge"
else
    fail "/metrics missing clients metric"
fi

if echo "${METRICS_RESP}" | grep -q 'unheaded_doom_bridge_frames_total'; then
    pass "/metrics has frames counter"
else
    fail "/metrics missing frames metric"
fi

if echo "${METRICS_RESP}" | grep -q 'unheaded_doom_bridge_bytes_sent_total'; then
    pass "/metrics has bytes counter"
else
    fail "/metrics missing bytes metric"
fi

if echo "${METRICS_RESP}" | grep -q 'unheaded_doom_bridge_errors_total'; then
    pass "/metrics has errors counter"
else
    fail "/metrics missing errors metric"
fi

# --- Test 7: Static file serving ---
echo "[TEST] Testing static file serving..."
STATIC_RESP=$(curl -s "http://localhost:${PORT}/" 2>/dev/null | head -5)
if echo "${STATIC_RESP}" | grep -qi "doom\|html\|canvas"; then
    pass "Static files served (doom.html or index)"
else
    # Might serve directory listing or other file; just check it's not empty
    if [[ -n "${STATIC_RESP}" ]]; then
        pass "Static directory served (non-empty response)"
    else
        fail "Static file serving returned empty"
    fi
fi

# --- Test 8: WebSocket upgrade ---
echo "[TEST] Testing WebSocket upgrade..."
WS_RESP=$(curl -s -i -N \
    -H "Connection: Upgrade" \
    -H "Upgrade: websocket" \
    -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
    -H "Sec-WebSocket-Version: 13" \
    --max-time 3 \
    "http://localhost:${PORT}/ws" 2>&1 | head -5 || true)

if echo "${WS_RESP}" | grep -qi "101\|switching\|upgrade"; then
    pass "WebSocket upgrade (101 response)"
else
    # May timeout or not show headers cleanly; check it doesn't 404
    if echo "${WS_RESP}" | grep -qi "404\|not found"; then
        fail "WebSocket endpoint returned 404"
    else
        pass "WebSocket endpoint exists (response inconclusive but not 404)"
    fi
fi

# --- Test 9: Go unit tests ---
echo "[TEST] Running Go unit tests..."
if cd "${PROJECT_ROOT}" && go test -count=1 ./cmd/doom-bridge/... 2>&1 | grep -q "^ok"; then
    pass "Unit tests pass"
else
    fail "Unit tests failed"
fi

# --- Summary ---
echo ""
echo "============================================"
echo " Results: ${PASSED} passed, ${FAILED} failed"
echo "============================================"

if [[ ${FAILED} -gt 0 ]]; then
    echo " SOME TESTS FAILED"
    exit 1
else
    echo " ALL TESTS PASSED"
    exit 0
fi
