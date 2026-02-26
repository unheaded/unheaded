#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# bird-check.sh — Verify BIRD BGP peering and routing on host-b

echo "=== BIRD Protocol Status ===" && birdc show protocols all
echo ""
echo "=== BGP Forge Session ===" && birdc show protocols forge
echo ""
echo "=== EVPN Routes from host-a ===" && birdc show route protocol forge
echo ""
echo "=== IPv6 Routes ===" && birdc show route where net ~ fd00:dead:beef::/32
echo ""
echo "=== BFD Sessions ===" && birdc show bfd sessions
echo ""
echo "=== Kernel Routes ===" && ip -6 route show | grep dead:beef
