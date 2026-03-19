#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/three-crowns/phase3b-nixos-lxd.sh
# Phase 3b: NixOS on LXD Runtime — WEST only
#
# Builds a minimal NixOS LXD image via nixos-generators, launches containers,
# pushes Go binaries in, fixes the glibc dynamic linker, and runs services.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

tc_header "PHASE 3b: NIXOS ON LXD — WEST"

NIXOS_SERVICES=(wotan timeguru captain architect micromanager monad sophia dashboard kanban)
NIXOS_IMAGE_ALIAS="nixos-unheaded"

# ── 3b.0 Check prerequisites ───────────────────────────────────────────
tc_log "3b.0 Checking nix and LXD..."

if ! command -v nix &>/dev/null; then
    tc_fail "nix not installed — cannot build NixOS image"
    tc_record "PHASE_NIXOS_LXD_WEST" "SKIP"
    exit 1
fi

if ! lxc info >/dev/null 2>&1; then
    tc_fail "LXD not available"
    tc_record "PHASE_NIXOS_LXD_WEST" "SKIP"
    exit 1
fi

tc_ok "nix and LXD available"

# ── 3b.1 Build NixOS LXD image ─────────────────────────────────────────
tc_log "3b.1 Building NixOS LXD image..."

# Check if image already exists
if lxc image list "${NIXOS_IMAGE_ALIAS}" -cn --format csv 2>/dev/null | grep -q "${NIXOS_IMAGE_ALIAS}"; then
    tc_ok "NixOS image already imported"
else
    # Create minimal NixOS config for LXD
    NIXOS_CONFIG=$(mktemp /tmp/nixos-lxd-XXXXXX.nix)
    cat > "$NIXOS_CONFIG" <<'NIXEOF'
{ config, pkgs, ... }:
{
  boot.isContainer = true;
  networking.hostName = "unheaded-nixos";
  networking.firewall.enable = false;
  environment.systemPackages = with pkgs; [ curl bash ];
  systemd.services.health-beacon = {
    description = "NixOS Health Beacon";
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      ExecStart = "${pkgs.python3}/bin/python3 -m http.server 20099";
      Restart = "always";
    };
  };
  system.stateVersion = "24.11";
}
NIXEOF

    tc_log "  Building rootfs tarball..."
    rootfs_path=$(nix run nixpkgs#nixos-generators -- --format lxc --configuration "$NIXOS_CONFIG" 2>/dev/null | tail -1)

    tc_log "  Building metadata tarball..."
    meta_path=$(nix run nixpkgs#nixos-generators -- --format lxc-metadata --configuration "$NIXOS_CONFIG" 2>/dev/null | tail -1)

    rm -f "$NIXOS_CONFIG"

    if [[ -z "$rootfs_path" || -z "$meta_path" ]]; then
        tc_fail "NixOS image build failed"
        tc_record "PHASE_NIXOS_LXD_WEST" "FAIL"
        exit 1
    fi

    tc_log "  Importing into LXD..."
    lxc image import "$meta_path" "$rootfs_path" --alias "${NIXOS_IMAGE_ALIAS}" 2>/dev/null
    tc_ok "NixOS image imported"
fi

# ── 3b.2 Launch NixOS containers ───────────────────────────────────────
tc_log "3b.2 Launching NixOS LXD containers..."

for svc in "${NIXOS_SERVICES[@]}"; do
    name="unheaded-${svc}"
    if lxc info "$name" >/dev/null 2>&1; then
        tc_log "  ${name} already exists, skipping"
        continue
    fi
    lxc launch "${NIXOS_IMAGE_ALIAS}" "$name" \
        --config limits.cpu=2 \
        --config limits.memory=512MB \
        --config environment.SERVICE_NAME="${svc}" 2>/dev/null &
done
wait

# Wait for all containers to be RUNNING
tc_log "  Waiting for containers to start..."
for attempt in $(seq 1 12); do
    running=$(lxc list unheaded- -cn,s --format csv 2>/dev/null | grep RUNNING | wc -l)
    if [[ $running -ge ${#NIXOS_SERVICES[@]} ]]; then
        break
    fi
    # Try starting any that are stopped
    lxc list unheaded- -cn,s --format csv 2>/dev/null | grep STOPPED | cut -d, -f1 | while read -r ctr; do
        lxc start "$ctr" 2>/dev/null || true
    done
    sleep 5
done

running=$(lxc list unheaded- -cn,s --format csv 2>/dev/null | grep RUNNING | wc -l)
tc_ok "${running}/${#NIXOS_SERVICES[@]} NixOS containers running"

# ── 3b.3 Fix dynamic linker + push binaries + start services ───────────
tc_log "3b.3 Fixing glibc linker, pushing binaries, starting services..."

for svc in "${NIXOS_SERVICES[@]}"; do
    name="unheaded-${svc}"
    bin_name="${SVC_BIN_MAP[$svc]:-$svc}"

    # Fix NixOS dynamic linker (Ubuntu-built binaries need /lib64/ld-linux-x86-64.so.2)
    nixos_fix_linker "$name"

    # Push binary
    lxc exec "$name" -- mkdir -p /opt/unheaded/bin 2>/dev/null || true
    lxc file push "${BIN_DIR}/${bin_name}" "${name}/opt/unheaded/bin/${bin_name}" --mode=0755 2>/dev/null || {
        tc_warn "Failed to push ${bin_name} to ${name}"
        continue
    }

    # Start service
    lxc exec "$name" -- bash -c \
        "nohup /opt/unheaded/bin/${bin_name} > /tmp/${bin_name}.log 2>&1 &" 2>/dev/null
done

# Wotan needs to be healthy before others connect
sleep 3

# ── 3b.4 Add proxy devices ─────────────────────────────────────────────
tc_log "3b.4 Adding proxy devices..."

declare -A PROXY_MAP=(
    [unheaded-wotan]=18000 [unheaded-timeguru]=19000 [unheaded-architect]=19001
    [unheaded-captain]=19002 [unheaded-micromanager]=19003 [unheaded-monad]=19004
    [unheaded-sophia]=19005 [unheaded-dashboard]=20000 [unheaded-kanban]=20001
)
for ctr in "${!PROXY_MAP[@]}"; do
    port="${PROXY_MAP[$ctr]}"
    lxc config device add "$ctr" "proxy${port}" proxy \
        listen=tcp:0.0.0.0:${port} connect=tcp:127.0.0.1:${port} 2>/dev/null || true
done

# ── 3b.5 Verify health ─────────────────────────────────────────────────
tc_log "3b.5 Verifying health..."
sleep 5

west_ok=true
health_check_ports "http://localhost" "${WEST_HEALTH_PORTS[@]}" || west_ok=false
tc_record "PHASE_NIXOS_LXD_WEST" "$($west_ok && echo PASS || echo FAIL)"

# ── 3b.6 USER GATE ─────────────────────────────────────────────────────
user_gate "NixOS_LXD" \
    "http://localhost:20000 (WEST Dashboard — NixOS containers)" \
    "http://localhost:20001 (WEST Kanban — NixOS containers)" || true

# ── 3b.7 Teardown ──────────────────────────────────────────────────────
tc_log "3b.7 Tearing down NixOS LXD containers..."

for svc in "${NIXOS_SERVICES[@]}"; do
    lxc stop --force "unheaded-${svc}" 2>/dev/null || true
    lxc delete --force "unheaded-${svc}" 2>/dev/null || true
done

teardown_clean=true
lxd_left=$(lxc list unheaded- -cn --format csv 2>/dev/null || true)
[[ -n "$lxd_left" ]] && teardown_clean=false
doom_range_scan "local" || teardown_clean=false

tc_record "PHASE_NIXOS_LXD_TEARDOWN" "$($teardown_clean && echo CLEAN || echo DIRTY)"

tc_header "PHASE 3b (NIXOS LXD) COMPLETE"
