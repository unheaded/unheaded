#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# node-guard.sh — a dead-man switch for lab work on a remote node whose one
# network link is also the only way to reach it (east: enp2s0). S-ASCEND-2 E3.
#
# `arm` starts the guard as its own transient systemd unit on the node, so it
# keeps running if the SSH session that armed it drops. The guard remediates,
# once, when either:
#   - the peer (west, 192.168.13.2 by default) misses FAIL_AFTER pings in a row
#     on the guarded interface, or
#   - the lease runs out (default 30 min) without a `renew`.
# Remediation is a fixed list, never a command from the caller:
#   1. detach any XDP program from the interface (generic, native, offload),
#   2. kill processes whose command line is under the lab directory
#      (default /tmp/s6),
#   3. remove the lab bpffs directory (default /sys/fs/bpf/unheaded-lab).
# Then it logs what it did and exits. `disarm` stops it without remediating.
#
# Usage (as root):
#   node-guard.sh arm [--iface IF] [--peer ADDR] [--lease SECS] [--fail-after N]
#                     [--lab-dir DIR] [--bpffs DIR] [--netns NAME]
#   node-guard.sh renew | disarm | status
#   node-guard.sh run ...        (the loop itself; `arm` starts it)
# --netns runs the guard inside a network namespace (for testing on a lab node).
set -euo pipefail

UNIT=unheaded-node-guard
STATE_DIR=/run/unheaded-node-guard
IFACE=enp2s0 PEER=192.168.13.2 LEASE=1800 FAIL_AFTER=5 LABDIR=/tmp/s6 BPFFS=/sys/fs/bpf/unheaded-lab NETNS=""

die() { echo "node-guard: $*" >&2; exit 2; }

parse() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --iface) IFACE="$2"; shift 2 ;;
            --peer) PEER="$2"; shift 2 ;;
            --lease) LEASE="$2"; shift 2 ;;
            --fail-after) FAIL_AFTER="$2"; shift 2 ;;
            --lab-dir) LABDIR="$2"; shift 2 ;;
            --bpffs) BPFFS="$2"; shift 2 ;;
            --netns) NETNS="$2"; shift 2 ;;
            *) die "unknown option $1" ;;
        esac
    done
    [[ "$IFACE" =~ ^[A-Za-z0-9._-]{1,15}$ ]] || die "bad --iface"
    [[ "$LEASE" =~ ^[0-9]+$ && "$FAIL_AFTER" =~ ^[0-9]+$ && "$FAIL_AFTER" -ge 1 ]] || die "bad --lease/--fail-after"
    [[ "$LABDIR" == /tmp/* && "$LABDIR" != *..* ]] || die "--lab-dir must be under /tmp"
    [[ "$BPFFS" == /sys/fs/bpf/unheaded-lab* && "$BPFFS" != *..* ]] || die "--bpffs must be /sys/fs/bpf/unheaded-lab*"
}

remediate() {
    local why="$1" ifindex id
    echo "node-guard: REMEDIATE ($why) on $IFACE"
    # 1. The lab processes first: an XDP program attached through a BPF link
    #    (aya's default) goes away when its owner's link fd closes, and netlink
    #    cannot detach a link-based attachment.
    if pkill -KILL -f "^$LABDIR/" 2>/dev/null; then echo "node-guard: killed processes under $LABDIR"; else echo "node-guard: no process under $LABDIR"; fi
    # 2. Pins keep links and programs alive past their process.
    if [[ -d "$BPFFS" ]]; then rm -rf "$BPFFS" && echo "node-guard: removed $BPFFS"; fi
    sleep 1
    # 3. Netlink-attached XDP (ip link ... xdp ...).
    ip link set dev "$IFACE" xdpgeneric off 2>/dev/null || true
    ip link set dev "$IFACE" xdpdrv off 2>/dev/null || true
    ip link set dev "$IFACE" xdpoffload off 2>/dev/null || true
    # 4. Any XDP link still bound to the interface (owned by something else).
    ifindex="$(cat "/sys/class/net/$IFACE/ifindex" 2>/dev/null || true)"
    if [[ -n "$ifindex" ]] && command -v bpftool >/dev/null; then
        for id in $(bpftool -j link show 2>/dev/null | python3 -c 'import json,sys
ifx=int(sys.argv[1])
for l in json.load(sys.stdin):
    if l.get("type")=="xdp" and l.get("ifindex")==ifx: print(l["id"])' "$ifindex"); do
            bpftool link detach id "$id" 2>/dev/null && echo "node-guard: detached XDP link $id"
        done
    fi
    if ip -d link show dev "$IFACE" | grep -q 'prog/xdp'; then
        echo "node-guard: WARNING: XDP still attached on $IFACE"
    else
        echo "node-guard: no XDP on $IFACE"
    fi
    echo "node-guard: done"
}

run() {
    parse "$@"
    mkdir -p "$STATE_DIR"
    local deadline=$(( $(date +%s) + LEASE )) misses=0
    echo "$deadline" > "$STATE_DIR/deadline"
    echo "node-guard: armed iface=$IFACE peer=$PEER lease=${LEASE}s fail-after=$FAIL_AFTER lab=$LABDIR"
    while true; do
        [[ -f "$STATE_DIR/deadline" ]] && deadline="$(cat "$STATE_DIR/deadline")"
        if (( $(date +%s) >= deadline )); then remediate "lease expired"; exit 0; fi
        if ping -c 1 -W 1 -I "$IFACE" "$PEER" >/dev/null 2>&1; then
            misses=0
        else
            misses=$(( misses + 1 ))
            echo "node-guard: peer $PEER missed ($misses/$FAIL_AFTER)"
            if (( misses >= FAIL_AFTER )); then remediate "peer unreachable"; exit 0; fi
        fi
        sleep 2
    done
}

arm() {
    parse "$@"
    [[ $EUID -eq 0 ]] || die "arm needs root"
    if systemctl is-active --quiet "$UNIT"; then die "$UNIT is already running (disarm first)"; fi
    local self
    self="$(readlink -f "$0")"
    local nsprop=()
    [[ -n "$NETNS" ]] && nsprop=(-p "NetworkNamespacePath=/run/netns/$NETNS")
    systemd-run --unit="$UNIT" --collect "${nsprop[@]}" /bin/bash "$self" run \
        --iface "$IFACE" --peer "$PEER" --lease "$LEASE" --fail-after "$FAIL_AFTER" \
        --lab-dir "$LABDIR" --bpffs "$BPFFS" >/dev/null
    sleep 1
    systemctl is-active --quiet "$UNIT" || die "guard did not start (journalctl -u $UNIT)"
    echo "node-guard: armed as $UNIT (journalctl -u $UNIT)"
}

case "${1:-}" in
    run) shift; run "$@" ;;
    arm) shift; arm "$@" ;;
    renew)
        [[ -f "$STATE_DIR/deadline" ]] || die "not armed"
        lease="${2:-1800}"; [[ "$lease" =~ ^[0-9]+$ ]] || die "bad lease"
        echo $(( $(date +%s) + lease )) > "$STATE_DIR/deadline"; echo "node-guard: lease renewed for ${lease}s" ;;
    disarm) systemctl stop "$UNIT" 2>/dev/null || true; rm -f "$STATE_DIR/deadline"; echo "node-guard: disarmed (no remediation)" ;;
    status) systemctl is-active "$UNIT" || true; [[ -f "$STATE_DIR/deadline" ]] && echo "lease ends in $(( $(cat "$STATE_DIR/deadline") - $(date +%s) ))s" ;;
    *) echo "usage: $0 arm|renew [SECS]|disarm|status|run ..." >&2; exit 2 ;;
esac
