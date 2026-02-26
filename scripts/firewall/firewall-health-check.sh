#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# firewall-health-check.sh — Verify firewall + routing stack health
# Run from either host to check both firewalls

set -euo pipefail

HOST_A_FW="${HOST_A_FW:-10.20.0.1}"   # OPNsense LAN IP
HOST_B_FW="${HOST_B_FW:-10.20.0.1}"   # IPFire LAN IP (from host-b)
WG_PEER_A="fd00:dead:beef::1"
WG_PEER_B="fd00:dead:beef::2"

PASS=0; FAIL=0

check() {
    local name="$1"; local cmd="$2"
    if eval "$cmd" &>/dev/null; then
        echo "  ✓ $name"
        PASS=$((PASS+1))
    else
        echo "  ✗ $name [FAIL]"
        FAIL=$((FAIL+1))
    fi
}

echo "========================================"
echo "Unheaded Firewall + Routing Health Check"
echo "========================================"

echo ""
echo "[ OPNsense (host-a) ]"
check "OPNsense API reachable" "curl -sk -u root:opnsense https://${HOST_A_FW}/api/core/firmware/info"
check "WireGuard UDP 51820 open" "nc -zu ${HOST_A_FW} 51820"
check "HTTPS 443 open" "nc -z ${HOST_A_FW} 443"
check "Monad HbH rule active" "curl -sk -u root:opnsense https://${HOST_A_FW}/api/firewall/filter/searchRule | grep -qi 'HbH'"

echo ""
echo "[ FRR BGP (host-a) ]"
check "BGP daemon running" "pgrep bgpd"
check "IS-IS daemon running" "pgrep isisd"
check "BFD daemon running" "pgrep bfdd"
check "WireGuard interface up" "ip link show wg0 | grep -q UP"
check "BGP peer reachable" "ping6 -c1 -W2 ${WG_PEER_B}"
check "VXLAN VTEPs created" "ip link show vxlan10001"
check "FRR BGP session" "vtysh -c 'show bgp summary' 2>/dev/null | grep -q 'Established'"
check "EVPN routes received" "vtysh -c 'show bgp l2vpn evpn' 2>/dev/null | grep -q 'Type'"

echo ""
echo "[ BIRD BGP (host-b — via WireGuard) ]"
check "BIRD peer reachable" "ping6 -c1 -W5 ${WG_PEER_B}"
# If running on host-b directly:
check "BIRD process running" "pgrep bird"
check "BIRD BGP forge session" "birdc show protocols forge 2>/dev/null | grep -q 'Established'"
check "BIRD BFD session" "birdc show bfd sessions 2>/dev/null | grep -q 'Up'"

echo ""
echo "[ Monad HbH End-to-End ]"
echo "  Testing IPv6 HbH header passthrough across firewall..."
# Send test HbH packet and verify it arrives
check "IPv6 HbH passthrough (OPNsense)" \
    "timeout 5 python3 -c \"
from scapy.all import *
pkt = IPv6(dst='fd00:dead:beef:1::1')/IPv6ExtHdrHopByHop(options=[HBHOptUnknown(otype=0x1E,optdata=b'\\x00'*20)])/ICMPv6EchoRequest()
ans,_=sr(pkt,timeout=3,verbose=0)
exit(0 if ans else 1)
\" 2>/dev/null"

echo ""
echo "[ Network Reachability ]"
check "Host-B reachable from Host-A (WireGuard)" "ping6 -c1 -W3 ${WG_PEER_B}"
check "Host-A reachable from Host-B (WireGuard)" "ping6 -c1 -W3 ${WG_PEER_A}"
check "BGP route to host-b subnet" "ip -6 route show | grep -q 'fd00:dead:beef:2'"

echo ""
echo "========================================"
echo "Results: ${PASS} passed, ${FAIL} failed"
[[ $FAIL -eq 0 ]] && echo "✓ ALL CHECKS PASSED" || echo "✗ ${FAIL} CHECKS FAILED"
echo "========================================"
exit $FAIL
