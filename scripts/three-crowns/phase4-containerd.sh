#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/three-crowns/phase4-containerd.sh
# Phase 4: containerd Runtime — Dual Host
#
# Notes from first run:
#   - containerd network must NOT use 10.10.10.0/24 (conflicts with LXD bridge)
#   - timeguru needs writable /tmp workdir (uses ./data/ relative path)
#   - captain needs writable /var/lib/unheaded
#   - Docker image names are "unheaded-dev-<svc>" from compose build

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

tc_header "PHASE 4: CONTAINERD RUNTIME — DUAL HOST"

CONTAINERD_SUBNET="10.10.20.0/24"  # Avoid 10.10.10.0/24 (LXD bridge conflict)

# ── 4.0 Verify nerdctl + containerd ────────────────────────────────────
tc_log "4.0 Verifying containerd and nerdctl..."

if ! command -v nerdctl &>/dev/null; then
    tc_fail "nerdctl not installed"
    tc_record "PHASE_CONTAINERD_WEST" "SKIP"
    exit 1
fi
tc_ok "nerdctl: $(nerdctl --version 2>/dev/null)"

if ! systemctl is-active --quiet containerd 2>/dev/null; then
    sudo systemctl start containerd
    sleep 2
fi
tc_ok "containerd is active"

# ── 4.1 Import Docker images into containerd ───────────────────────────
tc_log "4.1 Importing Docker images into containerd..."

cd "${PROJECT_ROOT}"
OCI_TARGETS=(wotan timeguru captain architect micromanager monad sophia dashboard-backend kanban-app)

for target in "${OCI_TARGETS[@]}"; do
    tc_log "  Importing unheaded-dev-${target}..."
    sudo docker save "unheaded-dev-${target}:latest" 2>/dev/null | \
        sudo ctr -n default images import - 2>/dev/null || \
        tc_warn "Failed to import ${target} (may need docker compose build first)"
done
tc_ok "Images imported"

# ── 4.2 Create containerd network ──────────────────────────────────────
tc_log "4.2 Creating containerd network..."
sudo nerdctl network rm unheaded 2>/dev/null || true
sudo nerdctl network create unheaded --subnet "${CONTAINERD_SUBNET}" 2>/dev/null || \
    tc_warn "Network creation failed (may already exist)"

# ── 4.3 Start WEST services ────────────────────────────────────────────
tc_log "4.3 Starting WEST services via nerdctl..."

# Service port mappings
declare -A SVC_PORTS=(
    [wotan]="-p 18000:18000 -p 18001:18001"
    [timeguru]="-p 19000:19000"
    [architect]="-p 19001:19001"
    [captain]="-p 19002:19002"
    [micromanager]="-p 19003:19003"
    [monad]="-p 19004:19004"
    [sophia]="-p 19005:19005"
    [dashboard-backend]="-p 20000:20000"
    [kanban-app]="-p 20001:20001"
)

# Wotan first
tc_log "  Starting wotan..."
# shellcheck disable=SC2086  # SVC_PORTS entry is an argument list; must split
sudo nerdctl run -d --name unheaded-wotan --network unheaded \
    ${SVC_PORTS[wotan]} \
    -e LOG_LEVEL=info \
    docker.io/library/unheaded-dev-wotan:latest 2>/dev/null || tc_warn "wotan start failed"

wait_for_health "http://localhost:18000/health" 30 && tc_ok "Wotan healthy" || tc_warn "Wotan slow to start"

# Remaining services — timeguru and captain need writable dirs
for svc in timeguru captain architect micromanager monad sophia dashboard-backend kanban-app; do
    tc_log "  Starting ${svc}..."
    extra_args=""
    # timeguru uses ./data/ relative path, captain uses /var/lib/unheaded
    if [[ "$svc" == "timeguru" ]]; then
        extra_args="--tmpfs /tmp --tmpfs /data --tmpfs /var/lib/unheaded -w /tmp"
    elif [[ "$svc" == "captain" ]]; then
        extra_args="--tmpfs /var/lib/unheaded --tmpfs /data"
    fi

    # SVC_PORTS entries and extra_args are argument lists; they must word-split.
    # The directive has to be the LAST comment line before the command: shellcheck
    # rejects one placed after a line continuation (SC1126), and it parses a
    # following comment line as a continuation of the directive itself.
    # shellcheck disable=SC2086
    sudo nerdctl run -d --name "unheaded-${svc}" --network unheaded \
        ${SVC_PORTS[$svc]} \
        -e LOG_LEVEL=info \
        -e WOTAN_ADDR=unheaded-wotan:18001 \
        ${extra_args} \
        "docker.io/library/unheaded-dev-${svc}:latest" 2>/dev/null || tc_warn "${svc} start failed"
done
tc_ok "WEST containerd services started"

# ── 4.4 Start EAST services ────────────────────────────────────────────
tc_log "4.4 Starting EAST services natively..."
start_east_native
start_east_socat

# ── 4.5 Verify health ──────────────────────────────────────────────────
tc_log "4.5 Verifying health..."
sleep 5

west_ok=true
health_check_ports "http://localhost" "${WEST_HEALTH_PORTS[@]}" || west_ok=false
tc_record "PHASE_CONTAINERD_WEST" "$($west_ok && echo PASS || echo FAIL)"

east_ok=true
health_check_east_ssh "${EAST_HEALTH_PORTS[@]}" || east_ok=false
tc_record "PHASE_CONTAINERD_EAST" "$($east_ok && echo PASS || echo FAIL)"

# ── 4.6 USER GATE ──────────────────────────────────────────────────────
user_gate "containerd" \
    "http://localhost:20000 (WEST Dashboard)" \
    "http://localhost:20001 (WEST Kanban)" \
    "http://localhost:20080 (EAST Dashboard via socat)" || true

# ── 4.7 Teardown ───────────────────────────────────────────────────────
tc_log "4.7 Tearing down containerd — BOTH hosts..."

stop_socat
stop_east_native

sudo nerdctl ps -aq --filter name=unheaded 2>/dev/null | xargs -r sudo nerdctl rm -f 2>/dev/null || true
sudo nerdctl network rm unheaded 2>/dev/null || true

teardown_clean=true
nerdctl_left=$(sudo nerdctl ps -a 2>/dev/null | grep unheaded || true)
[[ -n "$nerdctl_left" ]] && teardown_clean=false
doom_range_scan "local" || teardown_clean=false
doom_range_scan "${EAST_HOST}" || teardown_clean=false

tc_record "PHASE_CONTAINERD_TEARDOWN" "$($teardown_clean && echo CLEAN || echo DIRTY)"

tc_header "PHASE 4 (CONTAINERD) COMPLETE"
