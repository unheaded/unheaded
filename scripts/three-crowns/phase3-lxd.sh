#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/three-crowns/phase3-lxd.sh
# Phase 3: LXD/LXC Runtime (Ubuntu) — Dual Host
#
# Note: EAST has no LXD. EAST services run natively.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

tc_header "PHASE 3: LXD/LXC RUNTIME (UBUNTU) — DUAL HOST"

WEST_LXD_SERVICES=(wotan timeguru captain architect micromanager monad sophia dashboard kanban)

# ── 3.0 Ensure LXD initialized ─────────────────────────────────────────
tc_log "3.0 Verifying LXD on WEST..."
if lxc info >/dev/null 2>&1; then
    tc_ok "WEST: LXD initialized"
else
    tc_fail "WEST: LXD not initialized"
    exit 1
fi

# ── 3.1 Apply LXD profiles ─────────────────────────────────────────────
tc_log "3.1 Applying LXD profiles on WEST..."
cd "${PROJECT_ROOT}"
for profile_dir in containers/lxd/profiles lxd/profiles; do
    if [[ -d "$profile_dir" ]]; then
        for f in "${profile_dir}"/*.yaml; do
            [[ -f "$f" ]] || continue
            name=$(basename "$f" .yaml)
            lxc profile create "$name" 2>/dev/null || true
            cat "$f" | lxc profile edit "$name" 2>/dev/null && tc_log "  Profile: ${name}" || true
        done
    fi
done

# ── 3.2 Launch WEST LXD containers ─────────────────────────────────────
tc_log "3.2 Launching WEST LXD containers (Ubuntu)..."

for svc in "${WEST_LXD_SERVICES[@]}"; do
    container_name="unheaded-${svc}"
    if lxc info "$container_name" >/dev/null 2>&1; then
        tc_log "  ${container_name} already exists, skipping"
        continue
    fi
    tc_log "  Launching ${container_name}..."
    lxc launch ubuntu:24.04 "${container_name}" \
        --config limits.cpu=2 \
        --config limits.memory=512MB \
        --config environment.SERVICE_NAME="${svc}" \
        --config environment.LOG_LEVEL=info 2>/dev/null &
done
wait
sleep 10

running=$(lxc list unheaded- -cn,s --format csv | grep RUNNING | wc -l)
tc_ok "${running}/${#WEST_LXD_SERVICES[@]} containers running"

# ── 3.3 Push binaries and start services on WEST ───────────────────────
tc_log "3.3 Pushing binaries and starting services..."

for svc in "${WEST_LXD_SERVICES[@]}"; do
    bin_name="${SVC_BIN_MAP[$svc]:-$svc}"
    container_name="unheaded-${svc}"

    if [[ ! -f "${BIN_DIR}/${bin_name}" ]]; then
        tc_warn "Binary ${bin_name} not found, skipping"
        continue
    fi

    lxc exec "${container_name}" -- mkdir -p /opt/unheaded/bin 2>/dev/null || true
    lxc file push "${BIN_DIR}/${bin_name}" "${container_name}/opt/unheaded/bin/${bin_name}" --mode=0755 2>/dev/null || {
        tc_warn "Failed to push ${bin_name}"
        continue
    }
    lxc exec "${container_name}" -- bash -c \
        "nohup /opt/unheaded/bin/${bin_name} > /var/log/${bin_name}.log 2>&1 &" 2>/dev/null
done

# Start wotan first, wait, then the rest are already started above
sleep 5

# ── 3.4 Add proxy devices ──────────────────────────────────────────────
tc_log "3.4 Adding proxy devices..."

declare -A PROXY_MAP=(
    [unheaded-wotan]=18000 [unheaded-timeguru]=19000 [unheaded-architect]=19001
    [unheaded-captain]=19002 [unheaded-micromanager]=19003 [unheaded-monad]=19004
    [unheaded-sophia]=19005 [unheaded-dashboard]=20000 [unheaded-kanban]=20001
)
for ctr in "${!PROXY_MAP[@]}"; do
    port="${PROXY_MAP[$ctr]}"
    lxc config device add "$ctr" "proxy${port}" proxy \
        "listen=tcp:0.0.0.0:${port}" "connect=tcp:127.0.0.1:${port}" 2>/dev/null || true
done

# ── 3.5 Start EAST services ────────────────────────────────────────────
tc_log "3.5 Starting EAST services natively..."
start_east_native
start_east_socat

# ── 3.6 Verify health ──────────────────────────────────────────────────
tc_log "3.6 Verifying health..."
sleep 5

west_ok=true
health_check_ports "http://localhost" "${WEST_HEALTH_PORTS[@]}" || west_ok=false
tc_record "PHASE_LXD_WEST" "$($west_ok && echo PASS || echo FAIL)"

east_ok=true
health_check_east_ssh "${EAST_HEALTH_PORTS[@]}" || east_ok=false
tc_record "PHASE_LXD_EAST" "$($east_ok && echo PASS || echo FAIL)"

# ── 3.7 USER GATE ──────────────────────────────────────────────────────
user_gate "LXD" \
    "http://localhost:20000 (WEST Dashboard)" \
    "http://localhost:20001 (WEST Kanban)" \
    "http://localhost:20080 (EAST Dashboard via socat)" || true

# ── 3.8 LXD teardown ───────────────────────────────────────────────────
tc_log "3.8 Tearing down LXD — BOTH hosts..."

stop_socat
stop_east_native

for svc in "${WEST_LXD_SERVICES[@]}"; do
    lxc stop --force "unheaded-${svc}" 2>/dev/null || true
    lxc delete --force "unheaded-${svc}" 2>/dev/null || true
done

teardown_clean=true
lxd_left=$(lxc list unheaded- -cn --format csv 2>/dev/null || true)
[[ -n "$lxd_left" ]] && teardown_clean=false
doom_range_scan "local" || teardown_clean=false
doom_range_scan "${EAST_HOST}" || teardown_clean=false

tc_record "PHASE_LXD_TEARDOWN" "$($teardown_clean && echo CLEAN || echo DIRTY)"

tc_header "PHASE 3 (LXD/UBUNTU) COMPLETE"
