#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# zhen-lan-proof.sh — prove that no Zhen route answers a stranger (D-01).
#
# Walks raft/tests/zhen_routes.golden, the list the raft tests hold the app
# to, so this cannot test a stale list. Each route is asked three ways:
#
#   bare         no browser headers at all (curl, a script)
#   same-origin  this origin's Origin and Sec-Fetch-Site, the page's CSRF
#                cookie and header, and no session
#   cross-site   another origin's Origin and Sec-Fetch-Site
#
# Every route off the open list must answer 401 or 403 to all three. The open
# list is the page, the probe, the stylesheet and the login routes. /health
# must not carry a session id.
#
# Usage: zhen-lan-proof.sh [--dry-run] BASE_URL
#   BASE_URL   e.g. https://127.0.0.1:20103 or https://west.lan:20103
#   --dry-run  print the request plan (METHOD PATH WHO EXPECT) and send nothing
#
# Environment:
#   ZHEN_CA      CA bundle for the front (default /etc/unheaded/tls/ca.pem)
#   ZHEN_ROUTES  route list (default raft/tests/zhen_routes.golden)
#
# Takes no password, token or cookie from the operator, and logs nobody in.
# Exit: 0 pass, 1 a route answered a stranger, 2 usage or missing tool.

set -euo pipefail

dry_run=0
base=""
for arg in "$@"; do
    case "$arg" in
        --dry-run) dry_run=1 ;;
        -h|--help) sed -n '5,28p' "$0"; exit 0 ;;
        -*) echo "zhen-lan-proof: unknown flag $arg" >&2; exit 2 ;;
        *) base="${arg%/}" ;;
    esac
done
[ -n "$base" ] || { echo "usage: zhen-lan-proof.sh [--dry-run] BASE_URL" >&2; exit 2; }

root="$(cd "$(dirname "$0")/.." && pwd -P)"
routes="${ZHEN_ROUTES:-$root/raft/tests/zhen_routes.golden}"
ca="${ZHEN_CA:-/etc/unheaded/tls/ca.pem}"
[ -r "$routes" ] || { echo "zhen-lan-proof: cannot read $routes" >&2; exit 2; }

# The open list. Keep it equal to _LOGIN_OPEN in raft/zhen_app.py;
# raft/tests/test_zhen_app_gate.py holds the two together.
is_open() {
    case "$1" in
        "GET /"|"GET /health"|"POST /auth/login"|"POST /auth/logout"|"GET /auth/me"|"GET /api/v1/highlight.css") return 0 ;;
        *) return 1 ;;
    esac
}

# <int:id> becomes 1, any other <name> becomes x.
concrete() {
    printf '%s' "$1" | sed -E 's/<int:[^>]+>/1/g; s/<[^>]+>/x/g'
}

plan() {
    local method path expect who
    while read -r method path; do
        [ -n "$method" ] || continue
        if is_open "$method $path"; then expect="open"; else expect="401|403"; fi
        for who in bare same-origin cross-site; do
            printf '%s %s %s %s\n' "$method" "$(concrete "$path")" "$who" "$expect"
        done
    done <"$routes"
}

if [ "$dry_run" = 1 ]; then
    plan
    exit 0
fi

command -v curl >/dev/null || { echo "zhen-lan-proof: curl not found" >&2; exit 2; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
jar="$tmp/jar"

curl_base=(curl --silent --max-time 10 --curves X25519MLKEM768)
case "$base" in
    https://*) curl_base+=(--cacert "$ca") ;;
esac

# The page's CSRF cookie, as a browser on this origin would hold it. No session.
"${curl_base[@]}" --output /dev/null --cookie-jar "$jar" "$base/" || {
    echo "FAIL: $base/ did not answer" >&2
    exit 1
}
csrf="$(awk '$6 == "zhen_csrf" { print $7 }' "$jar" | tail -1)"
[ -n "$csrf" ] || { echo "FAIL: GET / set no zhen_csrf cookie" >&2; exit 1; }

total=0
passed=0
failures=()
while read -r method path who expect; do
    args=("${curl_base[@]}" --output /dev/null --write-out '%{http_code}' --request "$method")
    [ "$method" = GET ] || args+=(--header 'Content-Type: application/json' --data '{}')
    case "$who" in
        same-origin) args+=(--cookie "$jar" --header "Origin: $base" --header 'Sec-Fetch-Site: same-origin' --header "X-Zhen-CSRF: $csrf") ;;
        cross-site) args+=(--header 'Origin: https://stranger.invalid' --header 'Sec-Fetch-Site: cross-site') ;;
    esac
    code="$("${args[@]}" "$base$path" || true)"
    total=$((total + 1))
    if [ "$expect" = open ]; then
        # An open route has to answer; what it answers is its own business.
        case "$code" in
            000|5*) failures+=("$method $path ($who): open route answered $code") ;;
            *) passed=$((passed + 1)) ;;
        esac
    else
        case "$code" in
            401|403) passed=$((passed + 1)) ;;
            *) failures+=("$method $path ($who): answered $code, want 401 or 403") ;;
        esac
    fi
done < <(plan)

health="$("${curl_base[@]}" "$base/health" || true)"
total=$((total + 1))
case "$health" in
    *session_id*) failures+=("GET /health: the body names a session id") ;;
    "") failures+=("GET /health: no body") ;;
    *) passed=$((passed + 1)) ;;
esac

if [ "${#failures[@]}" -gt 0 ]; then
    printf 'FAIL: %s\n' "${failures[@]}"
    echo "FAIL $passed/$total"
    exit 1
fi
echo "PASS $passed/$total"
