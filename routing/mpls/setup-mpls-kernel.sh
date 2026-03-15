#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# Kernel MPLS setup — BARE METAL ONLY
# This script enables MPLS kernel forwarding.
# DO NOT run in VM or container (kernel modules required).
#
# CRITICAL: MPLS label stack is outer; IPv6+HbH (Monad) is inner.
# MPLS forwarding does NOT process inner IPv6 headers.
# Monad HbH extension headers survive MPLS label push/pop intact.
#
# Usage: sudo ./routing/mpls/setup-mpls-kernel.sh [wg0|lo|br-unheaded]
set -euo pipefail

IFACE="${1:-wg0}"

echo "=== Enabling kernel MPLS forwarding ==="

# Load MPLS kernel modules
for module in mpls_router mpls_gso mpls_iptunnel; do
    if ! lsmod | grep -q "^${module}"; then
        echo "Loading module: ${module}"
        modprobe "${module}" || { echo "WARN: ${module} not available"; }
    else
        echo "OK: ${module} already loaded"
    fi
done

# Enable MPLS input on all required interfaces
for iface in lo "${IFACE}" br-unheaded; do
    if ip link show "${iface}" &>/dev/null; then
        sysctl -w "net.mpls.conf.${iface}.input=1"
        echo "OK: MPLS input enabled on ${iface}"
    else
        echo "WARN: ${iface} not found — skipping"
    fi
done

# Set platform label range
sysctl -w net.mpls.platform_labels=100000
echo "OK: platform_labels=100000"

# Enable IPv4 MPLS forwarding
sysctl -w net.ipv4.conf.all.forwarding=1
sysctl -w net.ipv6.conf.all.forwarding=1

echo ""
echo "=== MPLS kernel setup complete ==="
echo "Verify: cat /proc/net/mpls_route"
echo "NOTE: MPLS forwarding does NOT strip inner IPv6 HbH headers (Monad safe)"
