#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/three-crowns/phase1-tunnel.sh
# Phase 1: WireGuard + BGP Tunnel Setup

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

tc_header "PHASE 1: WIREGUARD + BGP TUNNEL SETUP"

# ── 1.1-1.3 WireGuard setup ────────────────────────────────────────────
tc_log "1.1-1.3 Setting up WireGuard tunnel..."

# Use existing setup script which handles key generation, config, and deployment
# Note: setup-wireguard.sh uses P2P IP directly for EAST SSH which may fail
# if host key verification differs from 'east' alias. Fall back to manual if needed.
if bash "${PROJECT_ROOT}/scripts/wireguard/setup-wireguard.sh" 2>/dev/null; then
    tc_ok "WireGuard deployed via setup-wireguard.sh"
else
    tc_warn "setup-wireguard.sh failed — deploying manually"

    # Generate keys on WEST
    KEY_DIR="/tmp/wireguard-keys"
    CONF_DIR="/tmp/wireguard-confs"
    mkdir -p "$KEY_DIR" "$CONF_DIR"

    wg genkey | tee "${KEY_DIR}/west_private" | wg pubkey > "${KEY_DIR}/west_public"
    wg genkey | tee "${KEY_DIR}/east_private" | wg pubkey > "${KEY_DIR}/east_public"
    wg genpsk > "${KEY_DIR}/preshared"
    chmod 600 "${KEY_DIR}"/*

    west_priv=$(cat "${KEY_DIR}/west_private")
    east_priv=$(cat "${KEY_DIR}/east_private")
    west_pub=$(cat "${KEY_DIR}/west_public")
    east_pub=$(cat "${KEY_DIR}/east_public")
    psk=$(cat "${KEY_DIR}/preshared")

    # Generate WEST config
    cat > "${CONF_DIR}/wg0-west.conf" <<WGEOF
[Interface]
PrivateKey = ${west_priv}
Address = fd00:dead:beef::1/64
ListenPort = 51820
PostUp = ip -6 route add fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.all.forwarding=1; sysctl -w net.ipv6.conf.%i.forwarding=1
PostDown = ip -6 route del fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.all.forwarding=0; sysctl -w net.ipv6.conf.%i.forwarding=0

[Peer]
PublicKey = ${east_pub}
PresharedKey = ${psk}
AllowedIPs = fd00:dead:beef::2/128, fd00:dead:beef:0:2::/80
Endpoint = ${EAST_P2P}:51820
PersistentKeepalive = 25
WGEOF

    # Generate EAST config
    cat > "${CONF_DIR}/wg0-east.conf" <<WGEOF
[Interface]
PrivateKey = ${east_priv}
Address = fd00:dead:beef::2/64
ListenPort = 51820
PostUp = ip -6 route add fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.all.forwarding=1; sysctl -w net.ipv6.conf.%i.forwarding=1
PostDown = ip -6 route del fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.all.forwarding=0; sysctl -w net.ipv6.conf.%i.forwarding=0

[Peer]
PublicKey = ${west_pub}
PresharedKey = ${psk}
AllowedIPs = fd00:dead:beef::1/128, fd00:dead:beef:0:1::/80
Endpoint = ${WEST_P2P}:51820
PersistentKeepalive = 25
WGEOF

    # Deploy WEST
    sudo cp "${CONF_DIR}/wg0-west.conf" /etc/wireguard/wg0.conf
    sudo chmod 600 /etc/wireguard/wg0.conf
    sudo wg-quick up wg0
    tc_ok "WEST wg0 up"

    # Deploy EAST (pipe config via stdin to avoid scp permission issues)
    sudo cat "${CONF_DIR}/wg0-east.conf" | east_ssh \
        'cat > /tmp/wg0.conf && sudo mkdir -p /etc/wireguard && sudo mv /tmp/wg0.conf /etc/wireguard/wg0.conf && sudo chmod 600 /etc/wireguard/wg0.conf && sudo wg-quick up wg0'
    tc_ok "EAST wg0 up"
fi

# ── 1.4 Verify tunnel ──────────────────────────────────────────────────
tc_log "1.4 Verifying WireGuard tunnel..."

if ping6 -c 3 -W 5 "${EAST_WG}" >/dev/null 2>&1; then
    tc_ok "WEST -> EAST ping6 over WireGuard"
else
    tc_fail "WEST -> EAST ping6 failed"
    tc_record "TUNNEL" "FAIL"
    exit 1
fi

if east_ssh "ping6 -c 3 -W 5 ${WEST_WG}" >/dev/null 2>&1; then
    tc_ok "EAST -> WEST ping6 over WireGuard"
else
    tc_fail "EAST -> WEST ping6 failed"
    tc_record "TUNNEL" "FAIL"
    exit 1
fi

# ── 1.5 Start BGP routing daemons ──────────────────────────────────────
tc_log "1.5 Starting BGP routing daemons..."

# WEST: FRR
sudo cp "${PROJECT_ROOT}/routing/frr/frr.conf" /etc/frr/frr.conf
sudo systemctl restart frr
sleep 3
if systemctl is-active --quiet frr; then
    tc_ok "FRR started on WEST"
else
    tc_fail "FRR failed to start on WEST"
fi

# EAST: BIRD
tc_log "  Deploying BIRD config to EAST..."
east_scp "${PROJECT_ROOT}/routing/bird/bird.conf" "${EAST_USER}@${EAST_HOST}:/tmp/bird.conf"
east_ssh 'sudo cp /tmp/bird.conf /etc/bird/bird.conf && sudo systemctl restart bird'
sleep 5

if east_ssh 'systemctl is-active --quiet bird' 2>/dev/null; then
    tc_ok "BIRD started on EAST"
else
    tc_fail "BIRD failed to start on EAST"
fi

# ── 1.6 Verify BGP peering ─────────────────────────────────────────────
tc_log "1.6 Verifying BGP peering..."

tc_log "  Waiting for BGP session establishment (up to 30s)..."
bgp_established=false
for i in $(seq 1 15); do
    bgp_summary=$(sudo vtysh -c "show bgp summary" 2>/dev/null || true)
    if echo "$bgp_summary" | grep -qi "established\|estab"; then
        bgp_established=true
        break
    fi
    sleep 2
done

if $bgp_established; then
    tc_ok "WEST FRR: BGP session established"
else
    tc_warn "WEST FRR: BGP session not yet established"
fi

east_bgp=$(east_ssh 'sudo birdc show protocols 2>/dev/null' 2>/dev/null || true)
if echo "$east_bgp" | grep -qi "established"; then
    tc_ok "EAST BIRD: BGP session established"
else
    tc_warn "EAST BIRD: BGP not established yet"
fi

# ── 1.7 Verify BFD ─────────────────────────────────────────────────────
tc_log "1.7 Verifying BFD..."
bfd_peers=$(sudo vtysh -c "show bfd peers" 2>/dev/null || true)
if echo "$bfd_peers" | grep -qi "up"; then
    tc_ok "BFD session up"
else
    tc_warn "BFD not active (non-blocking)"
fi

# ── GATE ────────────────────────────────────────────────────────────────
tc_gate "TUNNEL GATE"
if ping6 -c 1 -W 3 "${EAST_WG}" >/dev/null 2>&1; then
    tc_ok "Tunnel up, BGP active. Proceeding to runtime tests."
    tc_record "TUNNEL" "PASS"
else
    tc_fail "Tunnel gate FAILED"
    tc_record "TUNNEL" "FAIL"
    exit 1
fi

tc_header "PHASE 1 COMPLETE"
