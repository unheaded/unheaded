#!/usr/bin/env bash
set -euo pipefail

echo "========================================"
echo "  Unheaded Kanban Demo"
echo "========================================"
echo ""

KANBAN_BIN="${KANBAN_BIN:-./bin/kanban-app}"
KANBAN_PORT="${KANBAN_PORT:-8081}"
KANBAN_URL="http://localhost:${KANBAN_PORT}"
HEALTH_ENDPOINT="${KANBAN_URL}/health"
MAX_RETRIES=30
RETRY_INTERVAL=1

# Start kanban-app binary
echo "Starting kanban-app (${KANBAN_BIN}) on port ${KANBAN_PORT}..."
if [ ! -f "$KANBAN_BIN" ]; then
    echo "ERROR: kanban-app binary not found at ${KANBAN_BIN}"
    echo "Run 'make build-services' first."
    exit 1
fi

"$KANBAN_BIN" &
KANBAN_PID=$!
echo "kanban-app started with PID ${KANBAN_PID}"

# Wait for health check
echo "Waiting for health check at ${HEALTH_ENDPOINT}..."
RETRIES=0
while [ "$RETRIES" -lt "$MAX_RETRIES" ]; do
    if curl -sf "${HEALTH_ENDPOINT}" > /dev/null 2>&1; then
        echo "Health check passed."
        break
    fi
    RETRIES=$((RETRIES + 1))
    sleep "$RETRY_INTERVAL"
done

if [ "$RETRIES" -ge "$MAX_RETRIES" ]; then
    echo "ERROR: Health check failed after ${MAX_RETRIES} retries."
    kill "$KANBAN_PID" 2>/dev/null || true
    exit 1
fi

# Open browser
echo "Opening browser at ${KANBAN_URL}..."
if command -v xdg-open &>/dev/null; then
    xdg-open "$KANBAN_URL"
elif command -v open &>/dev/null; then
    open "$KANBAN_URL"
else
    echo "Please open ${KANBAN_URL} in your browser."
fi

echo ""
echo "========================================="
echo "  WIP: This script is a stub."
echo "  Full demo logic is not yet implemented."
echo "========================================="
echo ""
echo "Press Ctrl+C to stop the demo."

# Wait for kanban-app to exit or user interrupt
trap "echo 'Stopping kanban-app...'; kill $KANBAN_PID 2>/dev/null; exit 0" INT TERM
wait "$KANBAN_PID"
