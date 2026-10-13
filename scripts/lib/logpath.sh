#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# logpath.sh <service> [file.log] — print the durable log path for <service>
# and create its directory (ADR-092).
#
#   nohup ./bin/wotan > "$(scripts/lib/logpath.sh wotan)" 2>&1 &
#   tail -f "$(scripts/lib/logpath.sh doom doom-runner.log)"
#
# Resolution, first match wins:
#   1. $UNHEADED_LOG_DIR/<service>/             (explicit override)
#   2. /var/log/unheaded/<service>/             (when the caller can write it:
#                                                root, or a provisioned host)
#   3. ${XDG_STATE_HOME:-~/.local/state}/unheaded/log/<service>/
#                                               (an operator on a dev box)
#
# Never /tmp: world-writable (anyone can pre-create or symlink a log a
# privileged process later opens), cleared on reboot (post-mortem evidence
# gone exactly when it is wanted), tmpfs on some hosts. All three candidates
# survive a reboot and are writable only by their owner.
#
# The directory is created 0750. A symlinked directory or log file is refused:
# writing through one is the attack /tmp invited, and it is no safer here.
set -euo pipefail

usage() { echo "usage: logpath.sh <service> [file.log]" >&2; exit 2; }

svc="${1:-}"
file="${2:-${svc}.log}"
[[ "$svc" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || usage
[[ "$file" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*\.log$ ]] || usage

if [[ -n "${UNHEADED_LOG_DIR:-}" ]]; then
    base="$UNHEADED_LOG_DIR"
elif [[ -w "/var/log/unheaded/$svc" || ( ! -e "/var/log/unheaded/$svc" && -w /var/log/unheaded ) ]]; then
    base=/var/log/unheaded
else
    base="${XDG_STATE_HOME:-$HOME/.local/state}/unheaded/log"
fi
[[ "$base" == /* ]] || { echo "logpath.sh: log base must be absolute: $base" >&2; exit 1; }

dir="$base/$svc"
if [[ -L "$dir" ]]; then
    echo "logpath.sh: refusing symlinked log directory: $dir" >&2
    exit 1
fi
(umask 027 && mkdir -p "$dir")
if [[ -L "$dir/$file" ]]; then
    echo "logpath.sh: refusing symlinked log file: $dir/$file" >&2
    exit 1
fi
printf '%s\n' "$dir/$file"
