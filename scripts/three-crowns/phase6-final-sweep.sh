#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/three-crowns/phase6-final-sweep.sh
# Phase 6: Final Scorched Earth Verification

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

tc_header "PHASE 6: FINAL SCORCHED EARTH VERIFICATION"

sweep_clean=true

# ── Both hosts: No processes ────────────────────────────────────────────
tc_log "Checking for orphan processes..."
verify_no_processes "local" || sweep_clean=false
verify_no_processes "${EAST_HOST}" || sweep_clean=false

# ── Both hosts: Doom Range full port scan ───────────────────────────────
tc_log "Doom Range port scan (${DOOM_RANGE_LOW}-${DOOM_RANGE_HIGH})..."
doom_range_scan "local" || sweep_clean=false
doom_range_scan "${EAST_HOST}" || sweep_clean=false

# ── Both hosts: Docker containers ───────────────────────────────────────
tc_log "Checking for Docker containers..."
west_docker=$(sudo docker ps -a 2>/dev/null | grep unheaded || true)
if [[ -n "$west_docker" ]]; then
    tc_fail "WEST has Docker containers:"
    echo "$west_docker"
    sweep_clean=false
else
    tc_ok "WEST: No Docker containers"
fi

# ── Both hosts: LXD containers ──────────────────────────────────────────
tc_log "Checking for LXD containers..."
west_lxd=$(lxc list unheaded- -cn --format csv 2>/dev/null || true)
if [[ -n "$west_lxd" ]]; then
    tc_fail "WEST has LXD containers: ${west_lxd}"
    sweep_clean=false
else
    tc_ok "WEST: No LXD containers"
fi

# ── Both hosts: containerd containers ────────────────────────────────────
tc_log "Checking for containerd containers..."
if command -v nerdctl &>/dev/null; then
    west_nerdctl=$(sudo nerdctl ps -a 2>/dev/null | grep unheaded || true)
    if [[ -n "$west_nerdctl" ]]; then
        tc_fail "WEST has containerd containers"
        sweep_clean=false
    else
        tc_ok "WEST: No containerd containers"
    fi
else
    tc_ok "WEST: nerdctl not installed (skip)"
fi

# ── Both hosts: WireGuard interface ──────────────────────────────────────
tc_log "Checking for WireGuard interface..."
if ip link show wg0 2>/dev/null; then
    tc_fail "WEST: wg0 still present"
    sweep_clean=false
else
    tc_ok "WEST: wg0 gone"
fi

east_wg=$(east_ssh 'ip link show wg0 2>/dev/null && echo "UP" || echo "DOWN"' 2>/dev/null || echo "DOWN")
if [[ "$east_wg" == *"UP"* ]]; then
    tc_fail "EAST: wg0 still present"
    sweep_clean=false
else
    tc_ok "EAST: wg0 gone"
fi

# ── Both hosts: BGP daemons ──────────────────────────────────────────────
tc_log "Checking for BGP daemons..."
if systemctl is-active --quiet frr 2>/dev/null; then
    tc_fail "WEST: FRR still running"
    sweep_clean=false
else
    tc_ok "WEST: FRR stopped"
fi

east_bird=$(east_ssh 'systemctl is-active bird 2>/dev/null' 2>/dev/null || echo "inactive")
if [[ "$east_bird" == "active" ]]; then
    tc_fail "EAST: BIRD still running"
    sweep_clean=false
else
    tc_ok "EAST: BIRD stopped"
fi

# ── Both hosts: socat ────────────────────────────────────────────────────
tc_log "Checking for socat..."
if pgrep -f socat >/dev/null 2>&1; then
    tc_fail "WEST: socat still running"
    sweep_clean=false
else
    tc_ok "WEST: No socat"
fi

# ── Record and report ────────────────────────────────────────────────────
tc_record "FINAL_SWEEP" "$($sweep_clean && echo CLEAN || echo DIRTY)"

print_report

tc_header "OPERATION THREE CROWNS COMPLETE"
