#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# heartbeat.sh: one dead-man's-switch heartbeat to the other host's
# alert-mailer (ADR-100; deploy/systemd/unheaded-heartbeat.service runs it
# every two minutes). The token comes from systemd's credential directory
# (LoadCredential=heartbeat-token) and reaches curl on stdin, never in argv,
# the environment or the unit.
#
#   HEARTBEAT_URL   http://192.168.13.2:19131/heartbeat/east
#   CREDENTIALS_DIRECTORY/heartbeat-token   set by systemd
set -euo pipefail

: "${HEARTBEAT_URL:?set HEARTBEAT_URL (e.g. http://192.168.13.2:19131/heartbeat/east)}"
token_file="${CREDENTIALS_DIRECTORY:?run under systemd with LoadCredential=heartbeat-token}/heartbeat-token"
[[ -r "$token_file" ]] || { echo "heartbeat: no token at $token_file" >&2; exit 1; }
token="$(tr -d '[:space:]' < "$token_file")"
[[ ${#token} -ge 32 ]] || { echo "heartbeat: token shorter than 32 bytes" >&2; exit 1; }

# printf is a builtin: the token is on curl's stdin, not on its command line.
printf 'header = "X-Heartbeat-Token: %s"\n' "$token" |
    curl --config - -fsS -m 10 -o /dev/null -X POST "$HEARTBEAT_URL"
