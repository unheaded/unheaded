#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# module-listener-guard.sh: the live half of the module bind rule (ADR-149
# decision 6, D5 (a)). Every 5 minutes (unheaded-module-guard.timer) it reads
# the listening sockets (ss -ltnH) and, for each module's ports, reports any
# listener on a wildcard, link-local, globally routable or unlisted private
# address to the textfile directory:
#
#   unheaded_module_listener_public{module,port,addr} 1     one per offending listener
#   unheaded_module_listener_public_total{module}           their count
#   unheaded_module_listener_guard_last_run_timestamp_seconds
#
# It reads and changes nothing else: the alert ModuleListenerPublic acts.
# Account unheaded-module-guard, uid 16717. The rule is shared with the static
# gate: scripts/lib/module_binds.py (installed beside this script's directory
# as ../lib/module_binds.py). The host's allowlist is module.yaml bind_allow.<host>.
#
# Usage: module-listener-guard.sh <modules-src> <textfile-dir>
#   GUARD_HOST overrides the host name (default: hostname -s).
set -uo pipefail

if [ "$#" -ne 2 ]; then
    echo "usage: module-listener-guard.sh <modules-src> <textfile-dir>" >&2
    exit 2
fi
src="$1" textfile="$2"
[ -d "$src" ] || { echo "module-listener-guard: no directory $src" >&2; exit 2; }
if [ ! -d "$textfile" ] || [ ! -w "$textfile" ]; then
    echo "module-listener-guard: textfile directory not writable: $textfile" >&2
    exit 2
fi

lib="$(cd "$(dirname "$0")/.." && pwd)/lib/module_binds.py"
host="${GUARD_HOST:-$(hostname -s)}"
exec python3 -B "$lib" guard "$src" "$textfile" "$host"
