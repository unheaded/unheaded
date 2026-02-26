#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# setup-vtep.sh — Create VXLAN VTEPs for FRR EVPN on host-a
# Run before frr.service starts
set -euo pipefail

VTEP_IP="10.20.255.1"
BRIDGE="br-unheaded"
VXLAN_PORT=4789

# Service overlay VNIs
VNIS=(10001 10002 10100)

for VNI in "${VNIS[@]}"; do
    VXLAN_IF="vxlan${VNI}"
    BR_IF="br-vxlan${VNI}"
    
    # Create VXLAN interface if not exists
    if ! ip link show "$VXLAN_IF" &>/dev/null; then
        ip link add "$VXLAN_IF" type vxlan \
            id "$VNI" \
            local "$VTEP_IP" \
            dstport "$VXLAN_PORT" \
            nolearning \
            dev "$BRIDGE"
        echo "[VTEP] Created $VXLAN_IF (VNI $VNI)"
    fi
    
    # Create per-VNI bridge
    if ! ip link show "$BR_IF" &>/dev/null; then
        ip link add "$BR_IF" type bridge
        ip link set "$BR_IF" up
        echo "[VTEP] Created bridge $BR_IF"
    fi
    
    # Attach VXLAN to bridge
    ip link set "$VXLAN_IF" master "$BR_IF" up
    echo "[VTEP] VNI $VNI: $VXLAN_IF attached to $BR_IF"
done

echo "[VTEP] VXLAN VTEP setup complete. FRR will discover VTEPs via zebra."
