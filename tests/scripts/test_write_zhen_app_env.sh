#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test_write_zhen_app_env.sh — scripts/write-zhen-app-env.sh against a
# throwaway compose.env and a throwaway target. No root, nothing under /etc.
#
# Holds the script to: the password is in the file and in nothing it prints;
# mode 0640 and the owner it asks for; fail loud, and write nothing, when
# there is no compose.env, no password, or a password systemd would alter.

set -euo pipefail
cd "$(dirname "$0")/../.."

script=scripts/write-zhen-app-env.sh
fail() { echo "FAIL: $*" >&2; exit 1; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# A made-up value, not a credential: it exists only in this temp directory.
secret="fixture-$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')"
{
    echo '# compose.env fixture'
    echo 'OPS_READER_PASSWORD=other-fixture-value'
    echo "APP_ZHEN_PASSWORD=$secret"
    echo 'WELL_PORT=5432'
} >"$tmp/compose.env"

run() { bash "$script" --compose-env "$tmp/compose.env" --out "$tmp/zhen-app.env" "$@"; }

# ── the good path ────────────────────────────────────────────────────────
run --db-host 127.0.0.1:5432 --allowed-hosts west.lan,10.10.20.2 >"$tmp/stdout" 2>"$tmp/stderr" \
    || fail "the script failed: $(cat "$tmp/stderr")"

grep -qx "ZHEN_DB_PASSWORD=$secret" "$tmp/zhen-app.env" || fail "the password is not in the file"
grep -qx 'ZHEN_DB_HOST=127.0.0.1' "$tmp/zhen-app.env" || fail "ZHEN_DB_HOST"
grep -qx 'ZHEN_DB_PORT=5432' "$tmp/zhen-app.env" || fail "ZHEN_DB_PORT"
grep -qx 'ZHEN_DB_SSLMODE=verify-full' "$tmp/zhen-app.env" || fail "ZHEN_DB_SSLMODE"
grep -qx 'ZHEN_DB_SSLROOTCERT=/etc/unheaded/well-tls/ca.crt' "$tmp/zhen-app.env" || fail "ZHEN_DB_SSLROOTCERT"
# REV-2: Zhen's rows live in unheaded_zhen (ADR-149, migration 052); the
# board's kanban_tasks stays in unheaded_app.
grep -qx 'ZHEN_DB_NAME=unheaded_zhen' "$tmp/zhen-app.env" || fail "ZHEN_DB_NAME"
grep -qx 'ZHEN_BOARD_DB_NAME=unheaded_app' "$tmp/zhen-app.env" || fail "ZHEN_BOARD_DB_NAME"
grep -qx 'ZHEN_ALLOWED_HOSTS=west.lan,10.10.20.2' "$tmp/zhen-app.env" || fail "ZHEN_ALLOWED_HOSTS"
grep -Eqx 'ZHEN_EMBEDDER_SHA256=[0-9a-f]{64}' "$tmp/zhen-app.env" || fail "ZHEN_EMBEDDER_SHA256"
grep -Eqx 'ZHEN_EMBEDDER_REVISION=[0-9a-f]{40}' "$tmp/zhen-app.env" || fail "ZHEN_EMBEDDER_REVISION"
if grep -q 'OPS_READER\|other-fixture-value' "$tmp/zhen-app.env"; then fail "another key's value was copied"; fi

if grep -qF "$secret" "$tmp/stdout" "$tmp/stderr"; then fail "the password was printed"; fi
grep -q 'keys: ZHEN_DB_PASSWORD ' "$tmp/stdout" || fail "the key names were not listed"
grep -q 'mode 0640' "$tmp/stdout" || fail "the mode was not reported"
grep -q 'owner root:unheaded-zhen-app' "$tmp/stdout" || fail "the owner asked for was not reported"

mode="$(stat -c '%a' "$tmp/zhen-app.env")"
[ "$mode" = 640 ] || fail "mode is $mode, want 640"
left="$(find "$tmp" -name 'zhen-app.env.*' | wc -l)"
[ "$left" -eq 0 ] || fail "a temporary file was left beside the target"

# The keys the example documents are the keys the script writes.
want="$(grep -oE '^[A-Z_]+=' deploy/systemd/zhen-app.env.example | sort)"
got="$(grep -oE '^[A-Z_]+=' "$tmp/zhen-app.env" | sort)"
[ "$want" = "$got" ] || fail "keys differ from deploy/systemd/zhen-app.env.example"

# ── an IPv6 Well, and a quoted password ──────────────────────────────────
printf 'APP_ZHEN_PASSWORD="%s"\n' "$secret" >"$tmp/compose.env"
run --db-host '[fd00:dead:beef::1]:5433' >"$tmp/stdout" 2>"$tmp/stderr" || fail "IPv6 --db-host refused: $(cat "$tmp/stderr")"
grep -qx 'ZHEN_DB_HOST=fd00:dead:beef::1' "$tmp/zhen-app.env" || fail "IPv6 host"
grep -qx 'ZHEN_DB_PORT=5433' "$tmp/zhen-app.env" || fail "IPv6 port"
grep -qx "ZHEN_DB_PASSWORD=$secret" "$tmp/zhen-app.env" || fail "quotes were not taken off the password"
grep -qx 'ZHEN_ALLOWED_HOSTS=' "$tmp/zhen-app.env" || fail "no --allowed-hosts should write an empty list"

# ── it refuses, loudly, and writes nothing ───────────────────────────────
refused() {  # refused WHY EXPECTED-RC ARGS...
    local why="$1" want_rc="$2" rc=0
    shift 2
    rm -f "$tmp/zhen-app.env"
    run "$@" >"$tmp/stdout" 2>"$tmp/stderr" || rc=$?
    [ "$rc" -eq "$want_rc" ] || fail "$why: exit $rc, want $want_rc"
    [ -s "$tmp/stderr" ] || fail "$why: refused without saying why"
    [ ! -e "$tmp/zhen-app.env" ] || fail "$why: a file was written"
    if grep -qF "$secret" "$tmp/stdout" "$tmp/stderr"; then fail "$why: the password was printed"; fi
    left="$(find "$tmp" -name 'zhen-app.env.*' | wc -l)"
    [ "$left" -eq 0 ] || fail "$why: a temporary file was left"
}

echo 'APP_ZHEN_PASSWORD=' >"$tmp/compose.env"
refused "empty password" 1 --db-host 127.0.0.1:5432
echo 'WELL_PORT=5432' >"$tmp/compose.env"
refused "no password key" 1 --db-host 127.0.0.1:5432
printf 'APP_ZHEN_PASSWORD=%s with space\n' "$secret" >"$tmp/compose.env"
refused "a password with a space" 1 --db-host 127.0.0.1:5432
printf 'APP_ZHEN_PASSWORD=%s\\\\x\n' "$secret" >"$tmp/compose.env"
refused "a password with a backslash" 1 --db-host 127.0.0.1:5432
rm -f "$tmp/compose.env"
refused "no compose.env" 1 --db-host 127.0.0.1:5432
echo "APP_ZHEN_PASSWORD=$secret" >"$tmp/compose.env"
refused "no --db-host" 2
refused "a port that is not a number" 2 --db-host 127.0.0.1:well
refused "a host with a shell character" 2 --db-host 'a;b:5432'
refused "an allowed host with a space" 2 --db-host 127.0.0.1:5432 --allowed-hosts 'a b'

# Without the test flags it wants root, and says so before reading anything.
if [ "$(id -u)" != 0 ]; then
    rc=0
    bash "$script" --db-host 127.0.0.1:5432 >"$tmp/stdout" 2>"$tmp/stderr" || rc=$?
    [ "$rc" -eq 2 ] || fail "not root: exit $rc, want 2"
    grep -q 'run as root' "$tmp/stderr" || fail "not root: no reason given"
fi

echo "PASS: write-zhen-app-env (the password is in the file and in nothing printed)"
