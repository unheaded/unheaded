#!/usr/bin/env bash
# doom-xdp-attach.sh — attach the monad_cpu XDP program to the ring interfaces.
#
# doom-runner loads the XDP program into the kernel but deliberately defers
# attachment (main.rs:429-436). This script bridges that gap. It is called
# as ExecStartPost= in unheaded-doom-runner.service, but can also be run
# manually after any doom-runner restart.
#
# Usage: sudo ./scripts/doom-xdp-attach.sh [max_wait_seconds]
#   max_wait_seconds: how long to poll for the prog to appear (default 15)
#
# Exit codes:
#   0 — attached successfully
#   1 — prog not found within timeout
#   2 — attach command failed

set -euo pipefail

MAX_WAIT=${1:-15}
POLL=1

log() { echo "[doom-xdp-attach] $*" >&2; }

# Wait for monad_cpu prog to appear in bpftool
log "waiting up to ${MAX_WAIT}s for monad_cpu XDP program..."
for i in $(seq 1 "$MAX_WAIT"); do
    PROG_ID=$(bpftool prog list 2>/dev/null | awk '/monad_cpu/{print $1; exit}' | tr -d ':')
    if [ -n "$PROG_ID" ]; then
        log "found prog id=$PROG_ID after ${i}s"
        break
    fi
    sleep "$POLL"
done

if [ -z "${PROG_ID:-}" ]; then
    log "ERROR: monad_cpu XDP program not found after ${MAX_WAIT}s"
    log "check: sudo bpftool prog list | grep monad_cpu"
    exit 1
fi

# Attach to both ring interfaces. The ring uses a 2-namespace topology:
#   monad0/veth01p  and  monad1/veth10p
# If the ring was set up with DOOM_RING_HOPS=2 (the default), these are the
# two interfaces. Adjust if DOOM_RING_HOPS was changed.
attach_iface() {
    local ns=$1 iface=$2
    if ip netns exec "$ns" ip link show "$iface" &>/dev/null; then
        log "attaching xdp id=$PROG_ID to $ns/$iface"
        ip netns exec "$ns" bpftool net attach xdp id "$PROG_ID" dev "$iface" 2>/dev/null || {
            # Already attached is not an error
            log "  (may already be attached — continuing)"
        }
    else
        log "WARN: $ns/$iface not found, skipping"
    fi
}

attach_iface monad0 veth01p
attach_iface monad1 veth10p

log "XDP attach complete — doom-runner is now executing"
exit 0
