#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# Routing option selector — switches between BGP-EVPN, OSPFv3, IS-IS, MPLS
# Usage: ./scripts/routing/select-routing.sh <bgp-evpn|ospf|isis|mpls>
# Effect: symlinks /etc/frr/frr.conf → appropriate config, validates, prints restart command
set -euo pipefail

ROUTING_BASE="/etc/unheaded/routing"
FRR_CONF="/etc/frr/frr.conf"
FRR_DAEMON_CONF="/etc/frr/daemons"

OPTION="${1:-}"

print_usage() {
    cat <<'USAGE_EOF'
Usage: $0 <option>

Options:
  bgp-evpn   BGP EVPN-VXLAN (default, L2 extension, EVPN)
  ospf       OSPFv3 (simple L3, no AS numbers, fast SPF)
  isis       IS-IS + SR-MPLS (segment routing, traffic engineering)
  mpls       MPLS LDP (label-switched paths, traffic engineering)

Example:
  sudo $0 ospf
  sudo systemctl restart frr

Current config:
  $(readlink -f "${FRR_CONF}" 2>/dev/null || echo "not set")
USAGE_EOF
}

if [ -z "${OPTION}" ]; then
    print_usage
    exit 1
fi

# Map option to config file
case "${OPTION}" in
    bgp-evpn)
        CONF_SRC="${ROUTING_BASE}/frr/frr.conf"
        DAEMON_SRC="${ROUTING_BASE}/frr/daemons"
        ;;
    ospf)
        CONF_SRC="${ROUTING_BASE}/ospf/frr-ospf.conf"
        DAEMON_SRC="${ROUTING_BASE}/ospf/daemons-ospf"
        ;;
    isis)
        CONF_SRC="${ROUTING_BASE}/isis/frr-isis-ha.conf"
        DAEMON_SRC="${ROUTING_BASE}/isis/daemons-isis"
        ;;
    mpls)
        CONF_SRC="${ROUTING_BASE}/mpls/frr-mpls.conf"
        DAEMON_SRC="${ROUTING_BASE}/mpls/daemons-mpls"
        ;;
    *)
        echo "ERROR: Unknown routing option: ${OPTION}"
        echo "Valid options: bgp-evpn, ospf, isis, mpls"
        exit 1
        ;;
esac

# Validate source files exist
if [ ! -f "${CONF_SRC}" ]; then
    echo "ERROR: Config file not found: ${CONF_SRC}"
    echo "Deploy config files first: make install-routing-configs"
    exit 1
fi

# FRR dry-run validation (if frr is available)
if command -v vtysh &>/dev/null; then
    echo "Validating config with FRR..."
    if vtysh --dryrun -f "${CONF_SRC}" 2>&1; then
        echo "OK: Config validated"
    else
        echo "WARN: FRR validation failed — check config before applying"
    fi
else
    echo "INFO: vtysh not available — skipping dry-run validation"
fi

# Create symlink (requires sudo)
if [ "$(id -u)" -ne 0 ]; then
    echo "ERROR: Must run as root (sudo) to update /etc/frr/frr.conf"
    exit 1
fi

# Backup current config
if [ -f "${FRR_CONF}" ] && [ ! -L "${FRR_CONF}" ]; then
    BACKUP="${FRR_CONF}.bak.$(date +%Y%m%d_%H%M%S)"
    cp "${FRR_CONF}" "${BACKUP}"
    echo "OK: Backed up existing config to ${BACKUP}"
fi

# Symlink the new config
ln -sf "${CONF_SRC}" "${FRR_CONF}"
echo "OK: Switched to ${OPTION}: ${FRR_CONF} -> ${CONF_SRC}"

if [ -f "${DAEMON_SRC}" ]; then
    ln -sf "${DAEMON_SRC}" "${FRR_DAEMON_CONF}"
    echo "OK: Daemon config -> ${DAEMON_SRC}"
fi

echo ""
echo "=== Switched to routing option: ${OPTION} ==="
echo ""
echo "To activate:"
echo "  sudo systemctl restart frr"
echo ""
echo "To verify:"
echo "  vtysh -c 'show running-config'"
echo "  vtysh -c 'show ip route'"
echo "  vtysh -c 'show ipv6 route'"
