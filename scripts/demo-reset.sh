#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
#
# Reset all state between demo runs
# Usage: scripts/demo-reset.sh [--full] [--east]

set -euo pipefail

FULL_RESET=false
RESET_EAST=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --full) FULL_RESET=true; shift ;;
        --east) RESET_EAST=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

echo "=== Resetting Unheaded Demo State ==="

# 1. Reset Wotan topics (clear event counts)
echo "1. Resetting Wotan topics..."
# Wotan has no reset endpoint (services/wotan/cmd/wotan/main.go routes only
# /health, /ready, /metrics and /api/v1/*); its buffers clear on restart.
echo "   Wotan has no topic reset; restart it (or use --full) to clear topics"

# 2. Clear BPF maps if they exist
echo "2. Clearing BPF maps..."
if [[ -d /sys/fs/bpf/unheaded ]]; then
    sudo find /sys/fs/bpf/unheaded -type f -delete 2>/dev/null && echo "   BPF maps cleared" || echo "   No BPF maps to clear"
else
    echo "   No BPF pin directory (skip)"
fi

# 3. Full reset: stop all services via kingdom-shutdown
if [[ "$FULL_RESET" == "true" ]]; then
    echo "3. Full reset: stopping all services..."
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    bash "$SCRIPT_DIR/kingdom-shutdown.sh" --force --keep-bpf
    echo "   Run scripts/demo-start.sh to restart"
fi

# 4. Reset EAST if requested
if [[ "$RESET_EAST" == "true" ]]; then
    echo "4. Resetting EAST services..."
    # .deb unit names: unheaded-<binary> (scripts/build-all-debs.sh)
    ssh govan@east "sudo systemctl restart unheaded-wotan unheaded-unheaded-daemon unheaded-timeguru unheaded-dashboard-backend unheaded-kanban-app" 2>/dev/null && echo "   EAST services restarted" || echo "   EAST not reachable"
fi

echo ""
echo "=== Reset complete ==="
echo "Run scripts/demo-start.sh to start fresh demo environment"
