#!/bin/bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
#
# Full stack startup for conference demo
# Usage: ./demo-start.sh [--east] [--trace-demo]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EAST_HOST="govan@east"
START_EAST=false
TRACE_DEMO=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --east) START_EAST=true; shift ;;
        --trace-demo) TRACE_DEMO=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

echo "=== Starting Unheaded Demo Environment ==="
echo "Host: $(hostname)"
echo "Date: $(date)"
echo ""

# 1. Build services if needed
echo "1. Checking builds..."
if [[ ! -f "$SCRIPT_DIR/cmd/wotan/wotan" ]]; then
    echo "   Building Go services..."
    cd "$SCRIPT_DIR" && go build ./...
else
    echo "   Builds up to date"
fi

# 2. Start Wotan (message bus)
echo "2. Starting Wotan (port 18000/18001)..."
if pgrep -f "wotan" > /dev/null 2>&1; then
    echo "   Wotan already running"
else
    cd "$SCRIPT_DIR" && go run ./cmd/wotan/ &
    sleep 2
fi
curl -sf http://localhost:18000/health > /dev/null && echo "   Wotan: HEALTHY" || echo "   Wotan: WAITING..."

# 3. Start unheaded-daemon (control plane)
echo "3. Starting unheaded-daemon (port 17000/17001)..."
if pgrep -f "unheaded-daemon" > /dev/null 2>&1; then
    echo "   unheaded-daemon already running"
else
    cd "$SCRIPT_DIR" && go run ./cmd/unheaded-daemon/ &
    sleep 2
fi

# 4. Start dashboard-backend
echo "4. Starting dashboard-backend (port 20000)..."
if pgrep -f "dashboard-backend" > /dev/null 2>&1; then
    echo "   dashboard-backend already running"
else
    cd "$SCRIPT_DIR" && go run ./cmd/dashboard-backend/ &
    sleep 2
fi

# 5. Start kanban-app
echo "5. Starting kanban-app (port 20001)..."
if pgrep -f "kanban-app" > /dev/null 2>&1; then
    echo "   kanban-app already running"
else
    cd "$SCRIPT_DIR" && go run ./cmd/kanban-app/ &
    sleep 2
fi

# 6. Start trace-collector in demo mode (optional)
if [[ "$TRACE_DEMO" == "true" ]]; then
    echo "6. Starting trace-collector-go (demo mode)..."
    if pgrep -f "trace-collector-go" > /dev/null 2>&1; then
        echo "   trace-collector-go already running"
    else
        cd "$SCRIPT_DIR" && go run ./cmd/trace-collector-go/ -demo &
        sleep 3
    fi
fi

# 7. Start EAST services (optional)
if [[ "$START_EAST" == "true" ]]; then
    echo "7. Starting EAST services..."
    ssh "$EAST_HOST" "sudo systemctl start wotan unheaded-daemon timeguru dashboard-backend kanban-app" 2>/dev/null || true
    sleep 3

    echo "   EAST service health:"
    for svc in wotan unheaded-daemon timeguru dashboard-backend kanban-app; do
        STATUS=$(ssh "$EAST_HOST" "systemctl is-active $svc" 2>/dev/null || echo "unknown")
        echo "   - $svc: $STATUS"
    done
fi

echo ""
echo "=== Demo Environment Status ==="
echo ""

# Health checks
SERVICES=("wotan:18000" "unheaded-daemon:17000" "dashboard-backend:20000" "kanban-app:20001")
ALL_HEALTHY=true

for entry in "${SERVICES[@]}"; do
    SVC="${entry%%:*}"
    PORT="${entry##*:}"
    if curl -sf "http://localhost:$PORT/health" > /dev/null 2>&1; then
        echo "  [OK] $SVC (port $PORT)"
    else
        echo "  [--] $SVC (port $PORT) - not responding"
        ALL_HEALTHY=false
    fi
done

echo ""
if [[ "$ALL_HEALTHY" == "true" ]]; then
    echo "=== Demo environment READY ==="
else
    echo "=== Some services not responding (may still be starting) ==="
fi

echo ""
echo "Dashboard: http://localhost:20000"
echo "Wotan:     http://localhost:18000/stats"
echo "Kanban:    http://localhost:20001"
