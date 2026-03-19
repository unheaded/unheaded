#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/three-crowns.sh — OPERATION THREE CROWNS
# Dual-Host Runtime Interoperability Validation
#
# Proves the full Unheaded stack works on Docker, LXD/LXC (Ubuntu + NixOS),
# and containerd spanning WEST (Forge) and EAST (Outpost) over WireGuard IPv6 + BGP.
#
# Usage:
#   ./scripts/three-crowns.sh                  # Run all phases
#   ./scripts/three-crowns.sh --phase 2        # Run Phase 2 (Docker) only
#   ./scripts/three-crowns.sh --from 3         # Run from Phase 3 onward
#   ./scripts/three-crowns.sh --skip-tunnel    # Skip tunnel setup/teardown
#   ./scripts/three-crowns.sh --dry-run        # Show what would run
#   ./scripts/three-crowns.sh --report         # Print last report

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TC_DIR="${SCRIPT_DIR}/three-crowns"
source "${TC_DIR}/lib.sh"

# Parse arguments
PHASE=""
FROM_PHASE=0
SKIP_TUNNEL=false
DRY_RUN=false
REPORT_ONLY=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --phase)   PHASE="$2"; shift 2 ;;
        --from)    FROM_PHASE="$2"; shift 2 ;;
        --skip-tunnel) SKIP_TUNNEL=true; shift ;;
        --dry-run) DRY_RUN=true; shift ;;
        --report)  REPORT_ONLY=true; shift ;;
        -h|--help)
            echo "OPERATION THREE CROWNS — Dual-Host Runtime Interoperability Validation"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --phase N        Run only phase N (0-6)"
            echo "  --from N         Run from phase N through 6"
            echo "  --skip-tunnel    Skip WireGuard/BGP setup and teardown (phases 1, 5)"
            echo "  --dry-run        Print phases without executing"
            echo "  --report         Print the last report and exit"
            echo "  -h, --help       Show this help"
            echo ""
            echo "Phases:"
            echo "  0  Prerequisites (clean slate, build, install nerdctl)"
            echo "  1  WireGuard + BGP tunnel setup"
            echo "  2  Docker runtime — dual host"
            echo "  3  LXD/LXC runtime (Ubuntu) — dual host"
            echo "  3b NixOS on LXD — WEST only"
            echo "  4  containerd runtime — dual host"
            echo "  5  WireGuard + BGP teardown"
            echo "  6  Final scorched earth verification"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Report-only mode
if $REPORT_ONLY; then
    if [[ -f "$REPORT_FILE" ]]; then
        print_report
    else
        echo "No report found. Run the operation first."
    fi
    exit 0
fi

# Should this phase run?
should_run() {
    local phase=$1
    if [[ -n "$PHASE" ]]; then
        [[ "$phase" == "$PHASE" ]]
    else
        [[ "$phase" -ge "$FROM_PHASE" ]]
    fi
}

# Banner
echo -e "${TC_CYAN}${TC_BOLD}"
cat <<'BANNER'

     ___  ___  ___ ___    _  _____ ___ ___  _  _
    / _ \| _ \| __| _ \  /_\|_   _|_ _/ _ \| \| |
   | (_) |  _/| _||   / / _ \ | |  | | (_) | .` |
    \___/|_|  |___|_|_\/_/ \_\|_| |___\___/|_|\_|

    _____ _  _ ___ ___ ___    ___ ___  _____      ___  _ ___
   |_   _| || | _ \ __| __|  / __| _ \/ _ \ \    / / \| / __|
     | | | __ |   / _|| _|  | (__|   / (_) \ \/\/ /| .` \__ \
     |_| |_||_|_|_\___|___|  \___|_|_\\___/ \_/\_/ |_|\_|___/

   Dual-Host Runtime Interoperability Validation
   WEST (The Forge) <== WireGuard IPv6 + BGP ==> EAST (The Outpost)
   Runtimes: Docker | LXD/Ubuntu | NixOS/LXD | containerd

BANNER
echo -e "${TC_NC}"

# Initialize report
init_report

# Phase dispatch
run_phase() {
    local phase=$1
    local script="${TC_DIR}/$2"

    if ! should_run "$phase"; then
        tc_log "Phase ${phase}: SKIPPED (not in range)"
        return 0
    fi

    if $DRY_RUN; then
        tc_log "Phase ${phase}: WOULD RUN ${script}"
        return 0
    fi

    if [[ ! -f "$script" ]]; then
        tc_fail "Phase ${phase}: Script not found: ${script}"
        return 1
    fi

    bash "$script"
}

# Skip tunnel phases if requested
if $SKIP_TUNNEL; then
    tc_log "Tunnel phases (1, 5) will be skipped (--skip-tunnel)"
    tc_record "TUNNEL" "SKIP"
    tc_record "TUNNEL_TEARDOWN" "SKIP"
fi

# Execute phases
run_phase 0 "phase0-prerequisites.sh"

if ! $SKIP_TUNNEL; then
    run_phase 1 "phase1-tunnel.sh"
fi

run_phase 2 "phase2-docker.sh"
run_phase 3 "phase3-lxd.sh"

# Phase 3b uses string matching, not numeric comparison
if [[ -z "$PHASE" || "$PHASE" == "3b" ]] && [[ "$FROM_PHASE" -le 3 ]]; then
    if $DRY_RUN; then
        tc_log "Phase 3b: WOULD RUN phase3b-nixos-lxd.sh"
    else
        bash "${TC_DIR}/phase3b-nixos-lxd.sh"
    fi
fi

run_phase 4 "phase4-containerd.sh"

if ! $SKIP_TUNNEL; then
    run_phase 5 "phase5-tunnel-teardown.sh"
fi

run_phase 6 "phase6-final-sweep.sh"

# If we ran a single phase, don't print the full report (phase6 handles it)
if [[ -n "$PHASE" && "$PHASE" != "6" ]]; then
    tc_log "Phase ${PHASE} complete. Run with --report to see full report."
fi
