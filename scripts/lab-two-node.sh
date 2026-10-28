#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# lab-two-node.sh — two "nodes" on one L2 segment, entirely inside network
# namespaces (S-ASCEND-2 Phase 8, the fallback when east is not a node).
#
#   lab-a  --veth-a--+
#                    |  br-lab (in lab-hub, fd00:1ab::1/64)
#   lab-b  --veth-b--+
#
# Link-local multicast (ff02::/16) crosses the bridge between lab-a and lab-b.
# A scratch service bound in lab-hub (e.g. Wotan on [fd00:1ab::1]) is reachable
# from both nodes. The root namespace is not touched. Evidence from this
# topology must say "2 netns on one bridge, not across hosts".
#
# Usage: lab-two-node.sh up|down|status    (needs root; idempotent)
set -euo pipefail

HUB=lab-hub A=lab-a B=lab-b BR=br-lab
NET=fd00:1ab

ns_exists() { ip netns list | awk '{print $1}' | grep -qx "$1"; }
nsx() { local n="$1"; shift; ip netns exec "$n" "$@"; }

link_node() { # link_node <ns> <suffix> <addr>
    local ns="$1" s="$2" addr="$3"
    if ! nsx "$ns" ip link show "veth-$s" >/dev/null 2>&1; then
        nsx "$HUB" ip link add "hub-$s" type veth peer name "veth-$s"
        nsx "$HUB" ip link set "veth-$s" netns "$ns"
        nsx "$HUB" ip link set "hub-$s" master "$BR" up
    fi
    nsx "$ns" ip link set lo up
    nsx "$ns" ip link set "veth-$s" up
    nsx "$ns" ip -6 addr replace "$NET::$addr/64" dev "veth-$s" nodad
}

up() {
    for n in "$HUB" "$A" "$B"; do ns_exists "$n" || ip netns add "$n"; done
    nsx "$HUB" ip link set lo up
    nsx "$HUB" ip link show "$BR" >/dev/null 2>&1 || nsx "$HUB" ip link add "$BR" type bridge mcast_snooping 0
    nsx "$HUB" ip link set "$BR" up
    nsx "$HUB" ip -6 addr replace "$NET::1/64" dev "$BR" nodad
    link_node "$A" a a
    link_node "$B" b b
    echo "up: $A ($NET::a) and $B ($NET::b) on $BR in $HUB ($NET::1)"
}

down() {
    for n in "$A" "$B" "$HUB"; do
        if ns_exists "$n"; then ip netns del "$n"; fi
    done
    echo "down: $HUB, $A, $B removed"
}

status() {
    local rc=0
    for n in "$HUB" "$A" "$B"; do
        if ns_exists "$n"; then echo "netns $n: present"; else echo "netns $n: absent"; rc=1; fi
    done
    [[ $rc -eq 0 ]] || return 1
    nsx "$A" ip -6 -br addr show dev veth-a
    nsx "$B" ip -6 -br addr show dev veth-b
    return 0
}

case "${1:-}" in
    up) up ;;
    down) down ;;
    status) status ;;
    *) echo "usage: $0 up|down|status" >&2; exit 2 ;;
esac
