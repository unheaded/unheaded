#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/three-crowns/phase2-docker.sh
# Phase 2: Docker Runtime — Dual Host
#
# Note: EAST has no Docker installed. EAST services run natively.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

tc_header "PHASE 2: DOCKER RUNTIME — DUAL HOST"

# ── 2.1 Build Docker images on WEST ────────────────────────────────────
tc_log "2.1 Building Docker images on WEST..."
cd "${PROJECT_ROOT}"
sudo docker compose build
tc_ok "Docker images built"

# ── 2.2 Start WEST services (full stack) ───────────────────────────────
tc_log "2.2 Starting WEST Docker services..."
sudo docker compose up -d
# Start kanban explicitly (depends_on chain may not auto-start it)
sleep 5
sudo docker start unheaded-kanban 2>/dev/null || true
tc_ok "WEST Docker services started"

# ── 2.3 Start EAST services (native — no Docker on EAST) ───────────────
tc_log "2.3 Starting EAST services natively (no Docker on EAST)..."
start_east_native

# ── 2.4 Wait for health on both hosts ──────────────────────────────────
tc_log "2.4 Waiting for services to become healthy..."
sleep 10

tc_log "  WEST health checks:"
west_ok=true
health_check_ports "http://localhost" "${WEST_HEALTH_PORTS[@]}" || west_ok=false
tc_record "PHASE_DOCKER_WEST" "$($west_ok && echo PASS || echo FAIL)"

tc_log "  EAST health checks (via tunnel):"
east_ok=true
health_check_east_tunnel "${EAST_HEALTH_PORTS[@]}" || {
    tc_log "  Falling back to SSH health checks..."
    health_check_east_ssh "${EAST_HEALTH_PORTS[@]}" || east_ok=false
}
tc_record "PHASE_DOCKER_EAST" "$($east_ok && echo PASS || echo FAIL)"

# ── 2.5 Set up socat for EAST dashboard ────────────────────────────────
tc_log "2.5 Setting up EAST dashboard forward on WEST :20080..."
start_east_socat

# ── 2.6 USER GATE: Verify Web UI ───────────────────────────────────────
user_gate "Docker" \
    "http://localhost:20000 (WEST Dashboard)" \
    "http://localhost:20001 (WEST Kanban)" \
    "http://localhost:20080 (EAST Dashboard via socat)" || true

# ── 2.7 Docker teardown — BOTH hosts ───────────────────────────────────
tc_log "2.7 Tearing down Docker — BOTH hosts..."

stop_socat
stop_east_native

cd "${PROJECT_ROOT}"
sudo docker compose down 2>/dev/null || tc_warn "WEST Docker teardown had errors"
tc_ok "Docker teardown complete"

# ── 2.8 Verify teardown — BOTH hosts ───────────────────────────────────
tc_log "2.8 Verifying Docker teardown..."

teardown_clean=true

west_remaining=$(sudo docker ps --filter name=unheaded -q 2>/dev/null || true)
if [[ -n "$west_remaining" ]]; then
    tc_fail "WEST still has Docker containers running"
    teardown_clean=false
else
    tc_ok "WEST: No Docker containers"
fi

doom_range_scan "local" || teardown_clean=false
doom_range_scan "${EAST_HOST}" || teardown_clean=false

tc_record "PHASE_DOCKER_TEARDOWN" "$($teardown_clean && echo CLEAN || echo DIRTY)"

tc_header "PHASE 2 (DOCKER) COMPLETE"
