#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# write-zhen-app-env.sh — write /etc/unheaded/zhen-app.env, unseen.
#
# The Zhen app's unit (deploy/systemd/unheaded-zhen-app.service) reads the
# Well's password for app_zhen from this file. The password already lives in
# /etc/unheaded/compose.env as APP_ZHEN_PASSWORD; this script copies it across
# without showing it: it is never printed, never an argument, never in a
# shell variable that is echoed. The output names the keys written and nothing
# else.
#
# Usage (as root):
#   scripts/write-zhen-app-env.sh --db-host HOST:PORT [--allowed-hosts LIST]
#
#   --db-host HOST:PORT    where the Well listens for this host (IPv6 in
#                          brackets: [fd00::1]:5432)
#   --allowed-hosts LIST   comma list of the names or addresses browsers use
#                          past loopback (ZHEN_ALLOWED_HOSTS); default none
#
# For the test (tests/scripts/test_write_zhen_app_env.sh), not for a host:
#   --compose-env FILE     read APP_ZHEN_PASSWORD from FILE
#   --out FILE             write FILE; root and the chown are then not required
#
# The file is written to a temporary name beside the target and moved into
# place, so a reader never sees half of it. Mode 0640, owner
# root:unheaded-zhen-app: the app's account reads it and cannot change it.
#
# Exit: 0 written, 1 refused (no compose.env, no password in it, a password
# systemd would alter), 2 usage.

set -euo pipefail

compose_env=/etc/unheaded/compose.env
out=/etc/unheaded/zhen-app.env
db_host=""
allowed_hosts=""
test_mode=0

while [ $# -gt 0 ]; do
    case "$1" in
        --db-host) db_host="${2:-}"; shift 2 ;;
        --allowed-hosts) allowed_hosts="${2:-}"; shift 2 ;;
        --compose-env) compose_env="${2:-}"; test_mode=1; shift 2 ;;
        --out) out="${2:-}"; test_mode=1; shift 2 ;;
        -h|--help) sed -n '5,31p' "$0"; exit 0 ;;
        *) echo "write-zhen-app-env: unknown argument $1" >&2; exit 2 ;;
    esac
done

usage() { echo "usage: write-zhen-app-env.sh --db-host HOST:PORT [--allowed-hosts LIST]" >&2; exit 2; }
[ -n "$db_host" ] || usage

# HOST:PORT, with an IPv6 host in brackets.
port="${db_host##*:}"
host="${db_host%:*}"
host="${host#[}"
host="${host%]}"
case "$port" in
    ''|*[!0-9]*) echo "write-zhen-app-env: --db-host needs HOST:PORT, with a numeric port" >&2; exit 2 ;;
esac
case "$host" in
    ''|*[!A-Za-z0-9.:_-]*) echo "write-zhen-app-env: --db-host has a host that is not a name or an address" >&2; exit 2 ;;
esac
case "$allowed_hosts" in
    *[!A-Za-z0-9.:,_-]*) echo "write-zhen-app-env: --allowed-hosts takes a comma list of names or addresses" >&2; exit 2 ;;
esac

if [ "$test_mode" = 0 ] && [ "$(id -u)" != 0 ]; then
    echo "write-zhen-app-env: run as root (it reads $compose_env and writes $out)" >&2
    exit 2
fi

[ -r "$compose_env" ] || {
    echo "write-zhen-app-env: cannot read $compose_env: credentials are not stored in this repo, and there is nothing to copy" >&2
    exit 1
}

# The last APP_ZHEN_PASSWORD line wins, as it does for compose. The value is
# held in one variable and written once, below; it is never echoed.
password=""
found=0
while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
        APP_ZHEN_PASSWORD=*) password="${line#APP_ZHEN_PASSWORD=}"; found=1 ;;
    esac
done <"$compose_env"

# One layer of matching quotes, as compose reads them.
case "$password" in
    \"*\") password="${password#\"}"; password="${password%\"}" ;;
    \'*\') password="${password#\'}"; password="${password%\'}" ;;
esac

if [ "$found" = 0 ] || [ -z "$password" ]; then
    echo "write-zhen-app-env: APP_ZHEN_PASSWORD is missing or empty in $compose_env; nothing written" >&2
    exit 1
fi
# systemd's EnvironmentFile treats quotes, backslashes, whitespace and a
# leading comment mark specially. A password with one would reach the app
# changed, and the Well would refuse it with no hint why. Refuse here instead.
case "$password" in
    *[[:space:]\"\'\\\#]*)
        echo "write-zhen-app-env: APP_ZHEN_PASSWORD has a character systemd's EnvironmentFile would alter (a quote, a backslash, a space or #); rotate it to one without, then run this again. Nothing written" >&2
        exit 1
        ;;
esac

root="$(cd "$(dirname "$0")/.." && pwd -P)"
# The memory embedder's pin: public constants, the same ones raft/start-zhen.sh exports.
# shellcheck source=scripts/lib/model-pins.sh
. "$root/scripts/lib/model-pins.sh"

umask 077
tmp="$(mktemp "$out.XXXXXX")"
trap 'rm -f "$tmp"' EXIT

{
    printf '# Written by scripts/write-zhen-app-env.sh. Keys: deploy/systemd/zhen-app.env.example.\n'
    printf 'ZHEN_DB_PASSWORD=%s\n' "$password"
    printf 'ZHEN_DB_NAME=unheaded_zhen\n'
    printf 'ZHEN_BOARD_DB_NAME=unheaded_app\n'
    printf 'ZHEN_DB_HOST=%s\n' "$host"
    printf 'ZHEN_DB_PORT=%s\n' "$port"
    printf 'ZHEN_DB_SSLMODE=verify-full\n'
    printf 'ZHEN_DB_SSLROOTCERT=/etc/unheaded/well-tls/ca.crt\n'
    printf 'ZHEN_ALLOWED_HOSTS=%s\n' "$allowed_hosts"
    printf 'ZHEN_EMBEDDER_SHA256=%s\n' "$EMBEDDER_SHA256"
    printf 'ZHEN_EMBEDDER_REVISION=%s\n' "$EMBEDDER_REVISION"
} >"$tmp"
password=""

chmod 0640 "$tmp"
if [ "$(id -u)" = 0 ]; then
    chown root:unheaded-zhen-app "$tmp"
    owner_note="owner root:unheaded-zhen-app"
else
    owner_note="owner root:unheaded-zhen-app wanted, not set (not root)"
fi
mv -f "$tmp" "$out"
trap - EXIT

echo "wrote $out, mode 0640, $owner_note"
echo "keys: ZHEN_DB_PASSWORD ZHEN_DB_NAME ZHEN_BOARD_DB_NAME ZHEN_DB_HOSTZHEN_DB_PORT ZHEN_DB_SSLMODE ZHEN_DB_SSLROOTCERT ZHEN_ALLOWED_HOSTS ZHEN_EMBEDDER_SHA256 ZHEN_EMBEDDER_REVISION"
