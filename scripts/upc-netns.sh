#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# upc-netns.sh — the network namespace upc-bootctl boots into (S-ASCEND-2
# Items 1-2; the 2026-05-13 isolation brief).
#
# Creates netns `upc0` with BOTH ends of the veth pair inside it:
#   veth-upc0   XDP attach point (bootctl attaches monad_cpu here)
#   veth-upc0p  trigger port (bootctl sends trigger frames out of it)
# Nothing the guest emits can reach the root namespace: there is no link to
# it. A forward-drop rule inside upc0 (table inet unheaded_upc) is defence in
# depth should forwarding ever be enabled there. The root namespace is not
# touched: no interface, route, address or firewall rule is added to it.
#
# bootctl then needs no root: CAP_BPF + CAP_NET_ADMIN (+ CAP_NET_RAW for the
# AF_PACKET trigger socket) inside upc0 (unheaded-upc-bootctl@.service uses
# NetworkNamespacePath=/run/netns/upc0; by hand: ip netns exec upc0 …).
#
# Usage: upc-netns.sh up|down|status    (needs root; idempotent)
# Exit: 0 ok; 1 failure; 2 usage. `status` exits 1 when anything is missing.
set -euo pipefail

NS="${UPC_NETNS:-upc0}"
XDP_IF=veth-upc0
TRIG_IF=veth-upc0p
TABLE=unheaded_upc

in_ns() { ip netns exec "$NS" "$@"; }

ns_exists() { ip netns list | awk '{print $1}' | grep -qx "$NS"; }

rule() {
    cat <<NFT
table inet ${TABLE} {
    chain forward {
        type filter hook forward priority 0; policy accept;
        iifname { "${XDP_IF}", "${TRIG_IF}" } counter drop
        oifname { "${XDP_IF}", "${TRIG_IF}" } counter drop
    }
}
NFT
}

up() {
    ns_exists || ip netns add "$NS"
    in_ns ip link set lo up
    if ! in_ns ip link show "$XDP_IF" >/dev/null 2>&1; then
        in_ns ip link add "$XDP_IF" type veth peer name "$TRIG_IF"
    fi
    in_ns ip link set "$XDP_IF" up
    in_ns ip link set "$TRIG_IF" up
    # The addresses bootctl's old setup used, so a guest sees the same link.
    in_ns ip -6 addr replace fd00:dead:beef:dada::1/64 dev "$XDP_IF" nodad
    in_ns ip -6 addr replace fd00:dead:beef:dada::de/64 dev "$TRIG_IF" nodad
    in_ns nft delete table inet "$TABLE" 2>/dev/null || true
    rule | in_ns nft -f -
    echo "up: netns $NS, $XDP_IF + $TRIG_IF, table inet $TABLE"
}

down() {
    if ns_exists; then
        ip netns del "$NS"
        echo "down: netns $NS removed (its interfaces and rules go with it)"
    else
        echo "down: netns $NS not present"
    fi
}

status() {
    local rc=0
    if ! ns_exists; then
        echo "netns $NS: absent"
        return 1
    fi
    echo "netns $NS: present"
    for i in "$XDP_IF" "$TRIG_IF"; do
        if in_ns ip link show "$i" 2>/dev/null | grep -q 'state UP\|,UP'; then
            echo "  $i: up"
        else
            echo "  $i: missing or down"
            rc=1
        fi
    done
    if in_ns nft list table inet "$TABLE" >/dev/null 2>&1; then
        echo "  table inet $TABLE: $(in_ns nft list table inet "$TABLE" | grep -c 'counter') forward rules"
        in_ns nft list table inet "$TABLE" | grep -E 'counter' | sed 's/^[[:space:]]*/    /'
    else
        echo "  table inet $TABLE: missing"
        rc=1
    fi
    echo "  ipv6 forwarding in $NS: $(in_ns sysctl -n net.ipv6.conf.all.forwarding)"
    return $rc
}

case "${1:-}" in
    up) up ;;
    down) down ;;
    status) status ;;
    *) echo "usage: $0 up|down|status" >&2; exit 2 ;;
esac
