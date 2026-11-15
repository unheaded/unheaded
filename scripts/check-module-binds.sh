#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-module-binds.sh: no module unit in the repository listens where the
# public side of a host can reach it (ADR-149 decision 6, ADR-147 decision 15
# (c), S-ZHEN-SWITCH D5 (a)). For each modules/*/module.yaml it reads every
# unit the module names in deploy/systemd: the value after -host, --host,
# -listen, --listen, -metrics-addr, --metrics-addr, --metrics (space or =),
# and every Environment= value whose name ends in _ADDR or _LISTEN. Loopback
# passes; a private address passes only if a host's bind_allow lists it; a
# wildcard, a link-local and a globally routable address fail always. The
# rule is scripts/lib/module_binds.py, shared with the live guard.
#
# Usage: scripts/check-module-binds.sh [--root DIR]    exit 0 pass, 1 fail, 2 usage
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LIB="${ROOT}/scripts/lib/module_binds.py"
while [ "$#" -gt 0 ]; do
    case "$1" in
        --root)
            [ "$#" -ge 2 ] || { echo "usage: check-module-binds.sh [--root DIR]" >&2; exit 2; }
            ROOT="$2"; shift 2 ;;
        *) echo "usage: check-module-binds.sh [--root DIR]" >&2; exit 2 ;;
    esac
done
[ -d "$ROOT" ] || { echo "check-module-binds: no directory $ROOT" >&2; exit 2; }
exec python3 -B "$LIB" units "$ROOT"
