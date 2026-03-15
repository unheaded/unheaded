#!/bin/bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
#
# Reset all state between demo runs
# Usage: ./demo-reset.sh [--full] [--east]

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
curl -sf -X POST http://localhost:18000/reset-topics > /dev/null 2>&1 && echo "   Wotan topics reset" || echo "   Wotan not responding (skip)"

# 2. Clear BPF maps if they exist
echo "2. Clearing BPF maps..."
if [[ -d /sys/fs/bpf/unheaded ]]; then
    sudo find /sys/fs/bpf/unheaded -type f -delete 2>/dev/null && echo "   BPF maps cleared" || echo "   No BPF maps to clear"
else
    echo "   No BPF pin directory (skip)"
fi

# 3. Clear trace-collector state
echo "3. Clearing trace-collector state..."
rm -f /tmp/trace-collector*.log 2>/dev/null
rm -f /tmp/wotan_events.log 2>/dev/null
echo "   Trace logs cleared"

# 4. Full reset: stop all services via kingdom-shutdown
if [[ "$FULL_RESET" == "true" ]]; then
    echo "4. Full reset: stopping all services..."
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    bash "$SCRIPT_DIR/scripts/kingdom-shutdown.sh" --force --keep-bpf
    echo "   Run ./demo-start.sh to restart"
fi

# 5. Reset EAST if requested
if [[ "$RESET_EAST" == "true" ]]; then
    echo "5. Resetting EAST services..."
    ssh govan@east "sudo systemctl restart wotan unheaded-daemon timeguru dashboard-backend kanban-app" 2>/dev/null && echo "   EAST services restarted" || echo "   EAST not reachable"
fi

echo ""
echo "=== Reset complete ==="
echo "Run ./demo-start.sh to start fresh demo environment"
