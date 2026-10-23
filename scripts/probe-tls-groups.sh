#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# probe-tls-groups.sh — print the TLS key-exchange group an endpoint
# negotiates, once for a hybrid offer and once for a classical-only offer,
# using OpenSSL (>= 3.5 knows the ML-KEM hybrids). A dev tool, not a gate:
# the Go handshake tests (pkg/tls/pqtest, ADR-105) are the gate.
#
#   probe-tls-groups.sh host:port [--starttls postgres]
#
# Output, one line per offer:
#   offer=hybrid    negotiated=X25519MLKEM768 protocol=TLSv1.3
#   offer=classical negotiated=none (handshake refused)
# A Strict (hybrid-only) server refuses the classical offer; a Preferred
# server falls back to X25519. Reads handshake metadata only; sends no data.
set -euo pipefail

usage() { echo "usage: $0 host:port [--starttls postgres]" >&2; exit 2; }

[ $# -ge 1 ] || usage
target=$1
shift
if ! [[ "$target" =~ ^[A-Za-z0-9.:_-]+:[0-9]{1,5}$ ]] && ! [[ "$target" =~ ^\[[0-9A-Fa-f:]+\]:[0-9]{1,5}$ ]]; then
    echo "probe-tls-groups: bad target '$target' (want host:port or [v6]:port)" >&2
    exit 2
fi
starttls=()
if [ $# -gt 0 ]; then
    if [ $# -ne 2 ] || [ "$1" != "--starttls" ] || [ "$2" != "postgres" ]; then
        usage
    fi
    starttls=(-starttls postgres)
fi
command -v openssl >/dev/null || { echo "probe-tls-groups: openssl not found" >&2; exit 2; }

host=${target%:*}
host=${host#[}
host=${host%]}

probe() { # $1 label, $2 OpenSSL group list
    local out group proto
    out=$(timeout 15 openssl s_client -connect "$target" -servername "$host" "${starttls[@]}" \
        -groups "$2" </dev/null 2>&1 || true)
    proto=$(printf '%s\n' "$out" | sed -n 's/^ *Protocol *: *//p' | head -1)
    # OpenSSL names a hybrid or PQ group on this line; a classical group shows
    # as the peer's temporary key instead.
    group=$(printf '%s\n' "$out" | sed -n 's/^Negotiated TLS1.3 group: //p' | head -1)
    if [ -z "$group" ] || [ "$group" = "<NULL>" ]; then
        group=$(printf '%s\n' "$out" | sed -n 's/^Peer Temp Key: \([^,]*\),.*/\1/p' | head -1)
    fi
    if [ -z "$group" ] || [ "$group" = "<NULL>" ]; then
        printf 'offer=%-9s negotiated=none (handshake refused)\n' "$1"
    else
        printf 'offer=%-9s negotiated=%s protocol=%s\n' "$1" "$group" "${proto:-unknown}"
    fi
}

probe hybrid "X25519MLKEM768:X25519"
probe classical "X25519:prime256v1"
