#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/three-crowns/phase0-prerequisites.sh
# Phase 0: Clean slate verification, build, copy binaries, install nerdctl

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

tc_header "PHASE 0: PREREQUISITES"

# ── 0.1 Verify clean slate on WEST ──────────────────────────────────────
tc_log "0.1 Verifying clean slate on WEST..."

doom_range_scan "local" || tc_warn "WEST has orphan listeners — consider running kingdom-shutdown.sh first"

west_docker=$(sudo docker ps -a --filter name=unheaded -q 2>/dev/null || true)
if [[ -n "$west_docker" ]]; then
    tc_warn "WEST has unheaded Docker containers:"
    echo "$west_docker"
else
    tc_ok "WEST: No unheaded Docker containers"
fi

west_lxd=$(lxc list unheaded- -cn --format csv 2>/dev/null || true)
if [[ -n "$west_lxd" ]]; then
    tc_warn "WEST has unheaded LXD containers:"
    echo "$west_lxd"
else
    tc_ok "WEST: No unheaded LXD containers"
fi

verify_no_processes "local" || tc_warn "WEST has running unheaded processes"

# ── 0.2 Verify clean slate on EAST ──────────────────────────────────────
tc_log "0.2 Verifying clean slate on EAST..."

doom_range_scan "${EAST_HOST}" || tc_warn "EAST has orphan listeners"
verify_no_processes "${EAST_HOST}" || tc_warn "EAST has running unheaded processes"

# ── 0.3 Build all Go binaries on WEST ───────────────────────────────────
tc_log "0.3 Building Go binaries..."

cd "${PROJECT_ROOT}"
make build

# Verify critical binaries exist
REQUIRED_BINS=(wotan timeguru captain architect micromanager monad sophia dashboard-backend kanban-app gateway)
missing=0
for bin in "${REQUIRED_BINS[@]}"; do
    if [[ ! -f "${BIN_DIR}/${bin}" ]]; then
        tc_fail "Missing binary: ${bin}"
        ((missing++)) || true
    fi
done

if [[ $missing -eq 0 ]]; then
    tc_ok "All ${#REQUIRED_BINS[@]} required binaries built"
else
    tc_fail "${missing} binaries missing — cannot proceed"
    exit 1
fi

# ── 0.4 Copy binaries to EAST ──────────────────────────────────────────
tc_log "0.4 Copying binaries to EAST..."

EAST_BINS=(wotan monad sophia dashboard-backend kanban-app gateway)

east_ssh "mkdir -p ${EAST_BIN_DIR}"

for bin in "${EAST_BINS[@]}"; do
    tc_log "  Copying ${bin}..."
    east_scp "${BIN_DIR}/${bin}" "${EAST_USER}@${EAST_HOST}:${EAST_BIN_DIR}/${bin}"
done
tc_ok "Binaries copied to EAST:${EAST_BIN_DIR}"

# ── 0.5 Install nerdctl on WEST (for Phase 4) ──────────────────────────
tc_log "0.5 Checking nerdctl..."

if command -v nerdctl &>/dev/null; then
    tc_ok "nerdctl already installed: $(nerdctl --version 2>/dev/null || echo 'unknown version')"
else
    tc_log "nerdctl not found — running installer..."
    bash "${SCRIPT_DIR}/../install-nerdctl.sh"
    if command -v nerdctl &>/dev/null; then
        tc_ok "nerdctl installed successfully"
    else
        tc_warn "nerdctl installation failed — Phase 4 (containerd) will be skipped"
    fi
fi

# Verify containerd is running
if systemctl is-active --quiet containerd 2>/dev/null; then
    tc_ok "containerd is active"
else
    tc_warn "containerd is not running — Phase 4 may fail"
fi

# ── 0.6 Install wireguard-tools on EAST if missing ─────────────────────
tc_log "0.5b Checking wireguard-tools on EAST..."

if east_ssh 'command -v wg >/dev/null 2>&1' 2>/dev/null; then
    tc_ok "EAST: wireguard-tools installed"
else
    tc_log "  Installing wg + wg-quick on EAST from WEST..."
    east_scp /usr/bin/wg "${EAST_USER}@${EAST_HOST}:/tmp/wg"
    east_scp /usr/bin/wg-quick "${EAST_USER}@${EAST_HOST}:/tmp/wg-quick"
    east_ssh 'sudo mv /tmp/wg /usr/bin/wg && sudo chmod 755 /usr/bin/wg && sudo mv /tmp/wg-quick /usr/bin/wg-quick && sudo chmod 755 /usr/bin/wg-quick'
    tc_ok "EAST: wireguard-tools installed from WEST"
fi

# ── 0.7 Install BIRD on EAST if missing ────────────────────────────────
tc_log "0.5c Checking BIRD on EAST..."

if east_ssh 'command -v birdc >/dev/null 2>&1' 2>/dev/null; then
    tc_ok "EAST: BIRD installed"
else
    tc_log "  Downloading BIRD deb on WEST for EAST..."
    BIRD_DEB=$(apt-cache show bird2 2>/dev/null | grep "^Filename:" | head -1 | awk '{print $2}')
    if [[ -n "$BIRD_DEB" ]]; then
        (cd /tmp && apt-get download bird2 libssh-4 2>/dev/null)
        east_scp /tmp/bird2*.deb "${EAST_USER}@${EAST_HOST}:/tmp/"
        east_scp /tmp/libssh*.deb "${EAST_USER}@${EAST_HOST}:/tmp/"
        east_ssh 'sudo dpkg -i /tmp/libssh*.deb /tmp/bird2*.deb 2>/dev/null' || true
        rm -f /tmp/bird2*.deb /tmp/libssh*.deb
        tc_ok "EAST: BIRD installed"
    else
        tc_warn "Could not download BIRD for EAST"
    fi
fi

# ── 0.8 Verify EAST is reachable ───────────────────────────────────────
tc_log "0.6 Verifying EAST reachability..."

if ping -c 3 -W 3 "${EAST_P2P}" >/dev/null 2>&1; then
    tc_ok "EAST reachable via P2P (${EAST_P2P})"
else
    tc_fail "Cannot reach EAST at ${EAST_P2P}"
    exit 1
fi

east_hostname=$(east_ssh 'hostname' 2>/dev/null || true)
if [[ -n "$east_hostname" ]]; then
    tc_ok "SSH to EAST works (hostname: ${east_hostname})"
else
    tc_fail "SSH to EAST failed"
    exit 1
fi

tc_header "PHASE 0 COMPLETE"
tc_ok "Prerequisites verified. Ready for Phase 1 (tunnel setup)."
