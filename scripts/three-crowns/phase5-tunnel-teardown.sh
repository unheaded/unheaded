#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/three-crowns/phase5-tunnel-teardown.sh
# Phase 5: WireGuard + BGP Teardown

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

tc_header "PHASE 5: WIREGUARD + BGP TEARDOWN"

# ── 5.1 Stop BGP daemons (retry-safe) ──────────────────────────────────
tc_log "5.1 Stopping BGP daemons..."
stop_bgp
tc_ok "BGP daemons stopped"

# ── 5.2 Tear down WireGuard tunnel ─────────────────────────────────────
tc_log "5.2 Tearing down WireGuard..."

sudo wg-quick down wg0 2>/dev/null && tc_ok "WEST wg0 down" || tc_warn "WEST wg0 was not up"
east_ssh 'sudo wg-quick down wg0 2>/dev/null' 2>/dev/null && tc_ok "EAST wg0 down" || tc_warn "EAST wg0 was not up"

# ── 5.3 Verify tunnel down ─────────────────────────────────────────────
tc_log "5.3 Verifying tunnel teardown..."

teardown_clean=true

if ip link show wg0 2>/dev/null; then
    tc_fail "WEST: wg0 still exists"
    teardown_clean=false
else
    tc_ok "WEST: wg0 interface gone"
fi

east_wg=$(east_ssh 'ip link show wg0 2>/dev/null && echo "STILL_UP" || echo "DOWN"' 2>/dev/null || echo "DOWN")
if [[ "$east_wg" == *"STILL_UP"* ]]; then
    tc_fail "EAST: wg0 still exists"
    teardown_clean=false
else
    tc_ok "EAST: wg0 interface gone"
fi

# Verify BGP really stopped (double-check)
if systemctl is-active --quiet frr 2>/dev/null; then
    tc_warn "FRR still active — force killing"
    sudo systemctl kill frr 2>/dev/null || true
    teardown_clean=false
fi

east_bird=$(east_ssh 'systemctl is-active bird 2>/dev/null' 2>/dev/null || echo "inactive")
if [[ "$east_bird" == "active" ]]; then
    tc_warn "BIRD still active — force killing"
    east_ssh 'sudo systemctl kill bird 2>/dev/null' || true
    teardown_clean=false
fi

tc_record "TUNNEL_TEARDOWN" "$($teardown_clean && echo CLEAN || echo DIRTY)"

tc_header "PHASE 5 COMPLETE"
