#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test_zhen_lan_proof.sh — the planning half of scripts/zhen-lan-proof.sh.
#
# The live half needs the front and its hybrid TLS, so it runs on west
# (S-ZHEN-FRONT Phase 11). This checks what can be checked anywhere: the plan
# asks every route three ways, the open list is the app's, path parameters are
# filled in, and a bad invocation is a usage error. Sends nothing.

set -euo pipefail
cd "$(dirname "$0")/../.."

script=scripts/zhen-lan-proof.sh
golden=raft/tests/zhen_routes.golden
fail() { echo "FAIL: $*" >&2; exit 1; }

plan="$(bash "$script" --dry-run https://127.0.0.1:20103)"

routes="$(grep -c . "$golden")"
lines="$(printf '%s\n' "$plan" | grep -c .)"
[ "$lines" -eq $((routes * 3)) ] || fail "plan has $lines requests, want 3 x $routes routes"

for who in bare same-origin cross-site; do
    n="$(printf '%s\n' "$plan" | awk -v w="$who" '$3 == w' | grep -c .)"
    [ "$n" -eq "$routes" ] || fail "$who: $n requests, want $routes"
done

open="$(printf '%s\n' "$plan" | awk '$4 == "open" { print $1, $2 }' | sort -u)"
want_open="$(printf '%s\n' 'GET /' 'GET /api/v1/highlight.css' 'GET /auth/me' 'GET /health' 'POST /auth/login' 'POST /auth/logout' | sort -u)"
[ "$open" = "$want_open" ] || fail "open list is: $(printf '%s' "$open" | tr '\n' ',')"

printf '%s\n' "$plan" | grep -q '^POST /api/v1/highlight bare 401|403$' || fail "the highlighter is not on the closed list"
printf '%s\n' "$plan" | grep -q '^POST /api/v1/query cross-site 401|403$' || fail "chat is not on the closed list"
if printf '%s\n' "$plan" | grep -q '[<>]'; then fail "a path parameter was left in the plan"; fi
printf '%s\n' "$plan" | grep -q '^POST /api/v1/approvals/1/approve ' || fail "an int parameter did not become 1"
printf '%s\n' "$plan" | grep -q '^GET /api/v1/runbooks/x ' || fail "a name parameter did not become x"

# A route added to the list is in the plan, closed, without touching the script.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
{ cat "$golden"; echo 'POST /api/v1/brand-new'; } >"$tmp/routes"
plan2="$(ZHEN_ROUTES="$tmp/routes" bash "$script" --dry-run https://127.0.0.1:20103)"
printf '%s\n' "$plan2" | grep -q '^POST /api/v1/brand-new same-origin 401|403$' || fail "a new route is not closed by default"

rc=0; bash "$script" >/dev/null 2>&1 || rc=$?
[ "$rc" -eq 2 ] || fail "no BASE_URL: exit $rc, want 2"
rc=0; bash "$script" --nope https://x >/dev/null 2>&1 || rc=$?
[ "$rc" -eq 2 ] || fail "unknown flag: exit $rc, want 2"

echo "PASS: zhen-lan-proof plan ($lines requests over $routes routes)"
