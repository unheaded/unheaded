#!/bin/bash
# SPDX-License-Identifier: MIT
# setup-wireguard.sh — Deploy WireGuard IPv6 overlay between WEST and EAST
# Phase 2: S77 Age 2 Acceleration — WireGuard IPv6 Overlay
set -euo pipefail

###############################################################################
# Configuration
###############################################################################
WEST_P2P="192.168.13.2"
EAST_P2P="192.168.13.1"
WG_PORT=51820
WG_IFACE="wg0"
WEST_WG_ADDR="fd00:dead:beef::1/64"
EAST_WG_ADDR="fd00:dead:beef::2/64"
KEY_DIR="/tmp/wireguard-keys"
CONF_DIR="/tmp/wireguard-confs"
REMOTE_USER="govan"
AGE_AVAILABLE=false

###############################################################################
# Helpers
###############################################################################
log()  { printf '[%s] %s\n' "$(date '+%H:%M:%S')" "$*"; }
die()  { log "FATAL: $*"; exit 1; }
ok()   { log "OK: $*"; }
warn() { log "WARN: $*"; }

check_deps() {
    for cmd in wg ssh ping6; do
        command -v "$cmd" >/dev/null 2>&1 || die "Missing dependency: $cmd"
    done
    if command -v age >/dev/null 2>&1; then
        AGE_AVAILABLE=true
        log "age encryption available — keys will be encrypted at rest"
    else
        warn "age not found — keys stored in plaintext (install age for encryption)"
    fi
}

###############################################################################
# Idempotency check
###############################################################################
is_configured() {
    local host="$1"
    if [ "$host" = "local" ]; then
        ip link show "$WG_IFACE" >/dev/null 2>&1 && return 0
    else
        ssh -o ConnectTimeout=5 "${REMOTE_USER}@${host}" \
            "ip link show ${WG_IFACE}" >/dev/null 2>&1 && return 0
    fi
    return 1
}

check_already_running() {
    local west_up=false east_up=false

    if is_configured "local"; then
        west_up=true
        log "WEST wg0 already exists"
    fi
    if is_configured "$EAST_P2P"; then
        east_up=true
        log "EAST wg0 already exists"
    fi

    if $west_up && $east_up; then
        log "WireGuard already configured on both hosts"
        log "Verifying connectivity..."
        if ping6 -c 1 -W 2 "fd00:dead:beef::2" >/dev/null 2>&1; then
            ok "IPv6 overlay fully operational — nothing to do"
            exit 0
        else
            warn "Interfaces exist but connectivity failed — will redeploy"
            teardown_existing
        fi
    elif $west_up || $east_up; then
        warn "Partial deployment detected — tearing down and redeploying"
        teardown_existing
    fi
}

teardown_existing() {
    log "Tearing down existing WireGuard configuration..."
    sudo wg-quick down "$WG_IFACE" 2>/dev/null || true
    ssh -o ConnectTimeout=5 "${REMOTE_USER}@${EAST_P2P}" \
        "sudo wg-quick down ${WG_IFACE}" 2>/dev/null || true
}

###############################################################################
# Key generation
###############################################################################
generate_keys() {
    log "Generating WireGuard keypairs..."
    mkdir -p "$KEY_DIR"
    chmod 700 "$KEY_DIR"

    # WEST keys
    wg genkey | tee "${KEY_DIR}/west_private" | wg pubkey > "${KEY_DIR}/west_public"
    chmod 600 "${KEY_DIR}/west_private"

    # EAST keys
    wg genkey | tee "${KEY_DIR}/east_private" | wg pubkey > "${KEY_DIR}/east_public"
    chmod 600 "${KEY_DIR}/east_private"

    # Preshared key for additional security
    wg genpsk > "${KEY_DIR}/preshared"
    chmod 600 "${KEY_DIR}/preshared"

    ok "Keys generated in ${KEY_DIR}"

    # Encrypt with age if available
    if $AGE_AVAILABLE; then
        log "Encrypting private keys with age..."
        for f in west_private east_private preshared; do
            age -p -o "${KEY_DIR}/${f}.age" "${KEY_DIR}/${f}" 2>/dev/null || \
                warn "age encryption skipped for ${f} (no passphrase provided)"
        done
    fi
}

###############################################################################
# Config generation
###############################################################################
generate_configs() {
    log "Generating WireGuard configurations..."
    mkdir -p "$CONF_DIR"

    local west_priv east_priv west_pub east_pub psk
    west_priv=$(cat "${KEY_DIR}/west_private")
    east_priv=$(cat "${KEY_DIR}/east_private")
    west_pub=$(cat "${KEY_DIR}/west_public")
    east_pub=$(cat "${KEY_DIR}/east_public")
    psk=$(cat "${KEY_DIR}/preshared")

    # WEST config (local host — 192.168.13.2)
    cat > "${CONF_DIR}/wg0-west.conf" <<EOF
# WireGuard config for WEST (${WEST_P2P})
# Unheaded S77 — IPv6 overlay fd00:dead:beef::/48
[Interface]
PrivateKey = ${west_priv}
Address = ${WEST_WG_ADDR}
ListenPort = ${WG_PORT}
PostUp = ip -6 route add fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.all.forwarding=1; sysctl -w net.ipv6.conf.%i.forwarding=1
PostDown = ip -6 route del fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.all.forwarding=0; sysctl -w net.ipv6.conf.%i.forwarding=0

[Peer]
# EAST
PublicKey = ${east_pub}
PresharedKey = ${psk}
AllowedIPs = fd00:dead:beef::2/128, fd00:dead:beef:0:2::/80
Endpoint = ${EAST_P2P}:${WG_PORT}
PersistentKeepalive = 25
EOF

    # EAST config (remote host — 192.168.13.1)
    cat > "${CONF_DIR}/wg0-east.conf" <<EOF
# WireGuard config for EAST (${EAST_P2P})
# Unheaded S77 — IPv6 overlay fd00:dead:beef::/48
[Interface]
PrivateKey = ${east_priv}
Address = ${EAST_WG_ADDR}
ListenPort = ${WG_PORT}
PostUp = ip -6 route add fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.all.forwarding=1; sysctl -w net.ipv6.conf.%i.forwarding=1
PostDown = ip -6 route del fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.all.forwarding=0; sysctl -w net.ipv6.conf.%i.forwarding=0

[Peer]
# WEST
PublicKey = ${west_pub}
PresharedKey = ${psk}
AllowedIPs = fd00:dead:beef::1/128, fd00:dead:beef:0:1::/80
Endpoint = ${WEST_P2P}:${WG_PORT}
PersistentKeepalive = 25
EOF

    chmod 600 "${CONF_DIR}"/wg0-*.conf
    ok "Configs written to ${CONF_DIR}"
}

###############################################################################
# Deployment
###############################################################################
deploy_west() {
    log "Deploying WireGuard on WEST (local)..."
    sudo cp "${CONF_DIR}/wg0-west.conf" "/etc/wireguard/${WG_IFACE}.conf"
    sudo chmod 600 "/etc/wireguard/${WG_IFACE}.conf"
    sudo systemctl enable "wg-quick@${WG_IFACE}" 2>/dev/null || true
    sudo wg-quick up "$WG_IFACE"
    ok "WEST wg0 is up"
}

deploy_east() {
    log "Deploying WireGuard on EAST (${EAST_P2P})..."

    # Ensure /etc/wireguard exists on EAST
    ssh "${REMOTE_USER}@${EAST_P2P}" "sudo mkdir -p /etc/wireguard"

    # Copy config
    scp -q "${CONF_DIR}/wg0-east.conf" "${REMOTE_USER}@${EAST_P2P}:/tmp/wg0.conf"
    ssh "${REMOTE_USER}@${EAST_P2P}" "sudo mv /tmp/wg0.conf /etc/wireguard/${WG_IFACE}.conf && sudo chmod 600 /etc/wireguard/${WG_IFACE}.conf"

    # Enable and start
    ssh "${REMOTE_USER}@${EAST_P2P}" "sudo systemctl enable wg-quick@${WG_IFACE} 2>/dev/null || true; sudo wg-quick up ${WG_IFACE}"
    ok "EAST wg0 is up"
}

###############################################################################
# Verification
###############################################################################
verify_connectivity() {
    log "Testing IPv6 connectivity over WireGuard..."

    # WEST → EAST
    if ping6 -c 3 -W 3 "fd00:dead:beef::2" >/dev/null 2>&1; then
        ok "WEST → EAST ping6 successful"
    else
        die "WEST → EAST ping6 FAILED"
    fi

    # EAST → WEST
    if ssh "${REMOTE_USER}@${EAST_P2P}" "ping6 -c 3 -W 3 fd00:dead:beef::1" >/dev/null 2>&1; then
        ok "EAST → WEST ping6 successful"
    else
        die "EAST → WEST ping6 FAILED"
    fi

    # Show wg status
    log "=== WEST wg show ==="
    sudo wg show "$WG_IFACE"
    echo

    log "=== EAST wg show ==="
    ssh "${REMOTE_USER}@${EAST_P2P}" "sudo wg show ${WG_IFACE}"
    echo

    ok "WireGuard IPv6 overlay is fully operational"
}

###############################################################################
# Cleanup
###############################################################################
cleanup_keys() {
    log "Cleaning up plaintext keys..."
    if $AGE_AVAILABLE; then
        # Keep encrypted versions, remove plaintext
        rm -f "${KEY_DIR}/west_private" "${KEY_DIR}/east_private" "${KEY_DIR}/preshared"
        ok "Plaintext keys removed (age-encrypted copies retained)"
    else
        warn "No age encryption — plaintext keys retained in ${KEY_DIR}"
        warn "Secure these files or install age and re-run"
    fi
}

###############################################################################
# Main
###############################################################################
main() {
    log "=========================================="
    log "Unheaded S77 — WireGuard IPv6 Overlay Setup"
    log "WEST: ${WEST_P2P} → ${WEST_WG_ADDR}"
    log "EAST: ${EAST_P2P} → ${EAST_WG_ADDR}"
    log "=========================================="

    check_deps
    check_already_running
    generate_keys
    generate_configs
    deploy_west
    deploy_east
    verify_connectivity
    cleanup_keys

    log "=========================================="
    log "Deployment complete."
    log "  WEST: fd00:dead:beef::1"
    log "  EAST: fd00:dead:beef::2"
    log "  Port: ${WG_PORT}/UDP"
    log "=========================================="
}

main "$@"
