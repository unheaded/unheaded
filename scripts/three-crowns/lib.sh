#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/three-crowns/lib.sh — Shared functions for Operation Three Crowns
#
# Lessons learned from first run (2026-03-19):
#   - EAST needs explicit SSH key (-i ~/.ssh/id_workflow_key)
#   - EAST has no Docker, no internet — services run natively
#   - NixOS LXD containers need /lib64/ld-linux-x86-64.so.2 symlink for glibc binaries
#   - containerd network must avoid 10.10.10.0/24 (LXD bridge conflict)
#   - socat is the fastest way to expose EAST dashboard on WEST
#   - FRR/BIRD need explicit stop — systemctl stop doesn't always stick first try
#
# shellcheck disable=SC2034
# This file is sourced, never executed: its whole contract is to define constants
# and functions for the phase*.sh scripts. shellcheck analyses one file at a time,
# so every constant here reads as unused no matter how many consumers it has —
# BIN_DIR (3 consumers), WEST_HEALTH_PORTS (4), EAST_HEALTH_PORTS (3), SVC_BIN_MAP
# (2), EAST_P2P (2), WEST_P2P, WEST_WG and EAST_BIN_DIR (1 each) are all live.
# The directive states what the file is; it is not blanket-suppression. Anything
# genuinely dead gets deleted rather than covered by this (see EAST_SERVICES,
# removed 2026-08-03).

# ANSI colors
readonly TC_RED='\033[0;31m'
readonly TC_GREEN='\033[0;32m'
readonly TC_YELLOW='\033[1;33m'
readonly TC_BLUE='\033[0;34m'
readonly TC_CYAN='\033[0;36m'
readonly TC_BOLD='\033[1m'
readonly TC_NC='\033[0m'

# Project paths
readonly PROJECT_ROOT="${PROJECT_ROOT:-$HOME/tmp/unheaded}"
readonly BIN_DIR="${PROJECT_ROOT}/bin"
readonly EAST_USER="govan"
readonly EAST_HOST="east"
readonly EAST_P2P="192.168.13.1"
readonly WEST_P2P="192.168.13.2"
readonly EAST_WG="fd00:dead:beef::2"
readonly WEST_WG="fd00:dead:beef::1"
readonly EAST_BIN_DIR="/tmp/unheaded-bin"
readonly EAST_SSH_KEY="${HOME}/.ssh/id_workflow_key"

# Doom Range
readonly DOOM_RANGE_LOW=16666
readonly DOOM_RANGE_HIGH=26666

# Health endpoints (port:label)
readonly WEST_HEALTH_PORTS=(
    "18000:wotan"
    "19000:timeguru"
    "19001:architect"
    "19002:captain"
    "19003:micromanager"
    "19004:monad"
    "19005:sophia"
    "20000:dashboard"
    "20001:kanban"
)

readonly EAST_HEALTH_PORTS=(
    "18000:wotan"
    "19004:monad"
    "19005:sophia"
    "20000:dashboard"
)

# Service -> binary name mapping
declare -A SVC_BIN_MAP=(
    [wotan]=wotan [timeguru]=timeguru [captain]=captain
    [architect]=architect [micromanager]=micromanager
    [monad]=monad [sophia]=sophia
    [dashboard]=dashboard-backend [kanban]=kanban-app
)

# Report state file
readonly REPORT_FILE="/tmp/three-crowns-report.env"

# Socat PID file
readonly SOCAT_PID_FILE="/tmp/three-crowns-socat.pid"

# ── Logging ─────────────────────────────────────────────────────────────
tc_log()     { echo -e "${TC_BLUE}[$(date '+%H:%M:%S')]${TC_NC} $*"; }
tc_ok()      { echo -e "${TC_GREEN}[PASS]${TC_NC} $*"; }
tc_fail()    { echo -e "${TC_RED}[FAIL]${TC_NC} $*"; }
tc_warn()    { echo -e "${TC_YELLOW}[WARN]${TC_NC} $*"; }
tc_header()  { echo -e "\n${TC_CYAN}${TC_BOLD}=== $* ===${TC_NC}\n"; }
tc_gate()    { echo -e "\n${TC_BOLD}--- GATE: $* ---${TC_NC}\n"; }

# ── SSH wrapper (always uses the correct key) ───────────────────────────
east_ssh() {
    ssh -i "${EAST_SSH_KEY}" -o ConnectTimeout=10 "${EAST_USER}@${EAST_HOST}" "$@"
}

east_scp() {
    scp -i "${EAST_SSH_KEY}" "$@"
}

# ── Record a result for the final report ────────────────────────────────
tc_record() {
    local key="$1" val="$2"
    echo "${key}=${val}" >> "$REPORT_FILE"
}

# ── Doom Range port scan ────────────────────────────────────────────────
doom_range_scan() {
    local host="$1"
    local orphans

    if [[ "$host" == "local" ]]; then
        orphans=$(ss -tlnp 2>/dev/null | awk '{print $4}' | grep -oP ':\K[0-9]+$' | sort -un | \
            awk -v lo="$DOOM_RANGE_LOW" -v hi="$DOOM_RANGE_HIGH" '$1 >= lo && $1 <= hi {print $1}' || true)
    else
        orphans=$(east_ssh \
            "ss -tlnp 2>/dev/null | awk '{print \$4}' | grep -oP ':\K[0-9]+\$' | sort -un | \
            awk '\$1 >= ${DOOM_RANGE_LOW} && \$1 <= ${DOOM_RANGE_HIGH} {print \$1}'" 2>/dev/null || true)
    fi

    if [[ -n "$orphans" ]]; then
        tc_warn "Orphan listeners on ${host}:"
        echo "$orphans" | while read -r port; do
            echo "  ORPHAN :${port}"
        done
        return 1
    else
        tc_ok "${host}: Doom Range clear"
        return 0
    fi
}

# ── Health check a set of endpoints ─────────────────────────────────────
health_check_ports() {
    local base_url="$1"
    shift
    local ports=("$@")
    local pass=0 fail=0

    for entry in "${ports[@]}"; do
        local port="${entry%%:*}"
        local label="${entry##*:}"
        if curl -sf "${base_url}:${port}/health" -o /dev/null --max-time 5 2>/dev/null; then
            tc_ok "${label} (:${port})"
            ((pass++)) || true
        else
            tc_fail "${label} (:${port})"
            ((fail++)) || true
        fi
    done

    echo "  ${pass} healthy, ${fail} failed"
    [[ $fail -eq 0 ]]
}

# ── Health check EAST via SSH ───────────────────────────────────────────
health_check_east_ssh() {
    local ports=("$@")
    local pass=0 fail=0

    for entry in "${ports[@]}"; do
        local port="${entry%%:*}"
        local label="${entry##*:}"
        if east_ssh "curl -sf http://localhost:${port}/health -o /dev/null --max-time 5" 2>/dev/null; then
            tc_ok "EAST ${label} (:${port})"
            ((pass++)) || true
        else
            tc_fail "EAST ${label} (:${port})"
            ((fail++)) || true
        fi
    done

    echo "  EAST: ${pass} healthy, ${fail} failed"
    [[ $fail -eq 0 ]]
}

# ── Health check EAST via WireGuard tunnel ──────────────────────────────
health_check_east_tunnel() {
    local ports=("$@")
    local pass=0 fail=0

    for entry in "${ports[@]}"; do
        local port="${entry%%:*}"
        local label="${entry##*:}"
        if curl -sf "http://[${EAST_WG}]:${port}/health" -o /dev/null --max-time 5 2>/dev/null; then
            tc_ok "EAST(tunnel) ${label} (:${port})"
            ((pass++)) || true
        else
            tc_fail "EAST(tunnel) ${label} (:${port})"
            ((fail++)) || true
        fi
    done

    echo "  EAST(tunnel): ${pass} healthy, ${fail} failed"
    [[ $fail -eq 0 ]]
}

# ── Wait for health endpoint ────────────────────────────────────────────
wait_for_health() {
    local url="$1"
    local timeout="${2:-30}"
    local elapsed=0

    while [[ $elapsed -lt $timeout ]]; do
        if curl -sf "$url" -o /dev/null --max-time 3 2>/dev/null; then
            return 0
        fi
        sleep 2
        ((elapsed += 2)) || true
    done
    return 1
}

# ── User gate (interactive UI check) ────────────────────────────────────
user_gate() {
    local runtime="$1"
    shift
    local urls=("$@")

    tc_gate "USER VERIFICATION — ${runtime}"
    echo -e "${TC_BOLD}Please verify the following URLs in your browser:${TC_NC}"
    for url in "${urls[@]}"; do
        echo "  -> ${url}"
    done
    echo ""
    read -r -p "All UIs look good? [y/N] " confirm
    if [[ "$confirm" == "y" || "$confirm" == "Y" ]]; then
        tc_ok "User confirmed ${runtime} UI"
        tc_record "UI_${runtime}" "PASS"
        return 0
    else
        tc_fail "User rejected ${runtime} UI"
        tc_record "UI_${runtime}" "FAIL"
        return 1
    fi
}

# ── Verify no unheaded processes ────────────────────────────────────────
verify_no_processes() {
    local host="$1"
    local procs

    if [[ "$host" == "local" ]]; then
        procs=$(pgrep -af "wotan|timeguru|captain|dashboard-backend|kanban-app|monad|sophia|architect|micromanager|wiki-server|socat" 2>/dev/null | grep -v grep || true)
    else
        procs=$(east_ssh \
            'pgrep -af "wotan|timeguru|captain|dashboard-backend|kanban-app|monad|sophia" 2>/dev/null | grep -v grep' 2>/dev/null || true)
    fi

    if [[ -n "$procs" ]]; then
        tc_warn "Processes still running on ${host}:"
        echo "$procs"
        return 1
    else
        tc_ok "${host}: No unheaded processes"
        return 0
    fi
}

# ── Start EAST services natively ────────────────────────────────────────
# EAST has no Docker/LXD — always runs services as native binaries
start_east_native() {
    tc_log "Starting EAST services natively..."
    east_ssh bash <<'REMOTE'
BIN_DIR="/tmp/unheaded-bin"
LOG_DIR="/tmp/unheaded-logs"
mkdir -p "$LOG_DIR"
for svc_port in "wotan:18000" "monad:19004" "sophia:19005" "dashboard-backend:20000"; do
    svc="${svc_port%%:*}"
    port="${svc_port##*:}"
    if pgrep -f "${BIN_DIR}/${svc}" >/dev/null 2>&1; then
        echo "${svc} already running"
        continue
    fi
    nohup "${BIN_DIR}/${svc}" > "${LOG_DIR}/${svc}.log" 2>&1 &
    sleep 2
    curl -sf "http://localhost:${port}/health" -o /dev/null --max-time 5 && echo "${svc} OK" || echo "${svc} FAIL"
done
REMOTE
}

# ── Stop EAST services ──────────────────────────────────────────────────
stop_east_native() {
    tc_log "Stopping EAST services..."
    east_ssh 'pkill -f /tmp/unheaded-bin/ 2>/dev/null || true' 2>/dev/null
}

# ── Start socat forward for EAST dashboard on WEST :20080 ───────────────
start_east_socat() {
    pkill socat 2>/dev/null || true
    nohup socat TCP-LISTEN:20080,fork,reuseaddr TCP6:[${EAST_WG}]:20000 > /tmp/socat-east-dash.log 2>&1 &
    echo $! > "$SOCAT_PID_FILE"
    sleep 1
    if curl -sf http://localhost:20080/health -o /dev/null --max-time 5 2>/dev/null; then
        tc_ok "EAST dashboard forwarded to WEST :20080"
    else
        tc_warn "EAST dashboard socat forward may have failed"
    fi
}

# ── Stop socat ──────────────────────────────────────────────────────────
stop_socat() {
    pkill socat 2>/dev/null || true
    rm -f "$SOCAT_PID_FILE"
}

# ── Fix NixOS dynamic linker for glibc binaries ────────────────────────
# NixOS doesn't have /lib64/ld-linux-x86-64.so.2 — binaries built on
# Ubuntu need this symlink to run inside NixOS containers
nixos_fix_linker() {
    local container="$1"
    local glibc_path
    glibc_path=$(lxc exec "$container" -- bash -c \
        'ls /nix/store/*/lib64/ld-linux-x86-64.so.2 2>/dev/null | head -1' 2>/dev/null || true)
    if [[ -n "$glibc_path" ]]; then
        lxc exec "$container" -- bash -c \
            "mkdir -p /lib64 && ln -sf '${glibc_path}' /lib64/ld-linux-x86-64.so.2" 2>/dev/null
    else
        tc_warn "${container}: Could not find glibc ld-linux in nix store"
    fi
}

# ── Stop BGP daemons (retry-safe) ──────────────────────────────────────
stop_bgp() {
    sudo systemctl stop frr 2>/dev/null || true
    # Double-check
    if systemctl is-active --quiet frr 2>/dev/null; then
        sudo systemctl kill frr 2>/dev/null || true
    fi

    east_ssh 'sudo systemctl stop bird 2>/dev/null || true' 2>/dev/null
    east_ssh 'systemctl is-active --quiet bird 2>/dev/null && sudo systemctl kill bird 2>/dev/null || true' 2>/dev/null
}

# ── Initialize report file ─────────────────────────────────────────────
init_report() {
    cat > "$REPORT_FILE" <<'EOF'
# Operation Three Crowns — Report State
# Generated by lib.sh
TUNNEL=PENDING
PHASE_DOCKER_WEST=PENDING
PHASE_DOCKER_EAST=PENDING
PHASE_DOCKER_TEARDOWN=PENDING
UI_Docker=PENDING
PHASE_LXD_WEST=PENDING
PHASE_LXD_EAST=PENDING
PHASE_LXD_TEARDOWN=PENDING
UI_LXD=PENDING
PHASE_NIXOS_LXD_WEST=PENDING
UI_NixOS_LXD=PENDING
PHASE_NIXOS_LXD_TEARDOWN=PENDING
PHASE_CONTAINERD_WEST=PENDING
PHASE_CONTAINERD_EAST=PENDING
PHASE_CONTAINERD_TEARDOWN=PENDING
UI_containerd=PENDING
TUNNEL_TEARDOWN=PENDING
FINAL_SWEEP=PENDING
EOF
}

# ── Print the final report ──────────────────────────────────────────────
print_report() {
    source "$REPORT_FILE"
    echo ""
    echo -e "${TC_CYAN}${TC_BOLD}"
    echo "============================================================="
    echo "  OPERATION THREE CROWNS — FINAL REPORT"
    echo "============================================================="
    echo -e "${TC_NC}"
    printf "  %-28s %s\n" "Tunnel (WireGuard+BGP):" "${TUNNEL}"
    printf "  %-28s WEST [%s]  EAST [%s]  UI [%s]  Teardown [%s]\n" \
        "Phase 2 (Docker):" "${PHASE_DOCKER_WEST}" "${PHASE_DOCKER_EAST}" "${UI_Docker}" "${PHASE_DOCKER_TEARDOWN}"
    printf "  %-28s WEST [%s]  EAST [%s]  UI [%s]  Teardown [%s]\n" \
        "Phase 3 (LXD/Ubuntu):" "${PHASE_LXD_WEST}" "${PHASE_LXD_EAST}" "${UI_LXD}" "${PHASE_LXD_TEARDOWN}"
    printf "  %-28s WEST [%s]  UI [%s]  Teardown [%s]\n" \
        "Phase 3b (LXD/NixOS):" "${PHASE_NIXOS_LXD_WEST}" "${UI_NixOS_LXD}" "${PHASE_NIXOS_LXD_TEARDOWN}"
    printf "  %-28s WEST [%s]  EAST [%s]  UI [%s]  Teardown [%s]\n" \
        "Phase 4 (containerd):" "${PHASE_CONTAINERD_WEST}" "${PHASE_CONTAINERD_EAST}" "${UI_containerd}" "${PHASE_CONTAINERD_TEARDOWN}"
    printf "  %-28s %s\n" "Tunnel Teardown:" "${TUNNEL_TEARDOWN}"
    printf "  %-28s %s\n" "Final Sweep:" "${FINAL_SWEEP}"
    echo ""
    echo "============================================================="
    echo "  Notes:"
    echo "    EAST runs services natively (no Docker/LXD — needs WEST NAT for internet)"
    echo "    NixOS containers need glibc linker symlink for Ubuntu-built binaries"
    echo "    EAST dashboard accessible via socat on WEST :20080"
    echo "============================================================="
}
