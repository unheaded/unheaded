#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-http-auth.sh — an HTTP listener in a package that does not use pkg/auth
# is a recorded decision, not an accident (GATING).
#
# A non-test Go file is a listener if it builds an http.Server, calls
# ListenAndServe, ListenAndServeTLS or ServeTLS on anything, or calls Serve on
# net/http, under whatever name the file imports it, or if it calls pkg/tls's
# MaybeWrapServer. Its package either imports pkg/auth, or the file is listed in
# docs/policy/http-auth-baseline.txt. A listed file that has gone or now has
# pkg/auth in its package fails until its row is removed.
#
# It reads source text: a wrapper of net/http outside these names, or a server
# from another library, is not seen. It proves a package imports pkg/auth, not
# that a given route is behind it. A row says only that nobody has wired this
# one, not that it is safe: many rows are health or metrics listeners. A new
# row beside a new listener passes here, so the list grows only as far as
# review of that diff allows. Compares the path SET, not a count, for the
# reason in check-tmp-log-baseline.sh.
#
#   scripts/check-http-auth.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
# shellcheck source=scripts/lib/sets.sh
source scripts/lib/sets.sh

BASELINE="docs/policy/http-auth-baseline.txt"
[[ -f "$BASELINE" ]] || { echo "[FAIL] $BASELINE is missing" >&2; exit 2; }

# Tracked and untracked: a new listener must be caught before it is committed.
all_go="$(git ls-files --cached --others --exclude-standard '*.go' | grep -v '_test\.go$' || true)"
[[ -n "$all_go" ]] || { echo "[FAIL] no Go files found; refusing to pass vacuously" >&2; exit 2; }

http_auth_is_listener() { # $1 file
	local name
	name="$(sed -nE 's/^[[:space:]]*(import[[:space:]]+)?([A-Za-z][A-Za-z0-9_]*)[[:space:]]+"net\/http".*$/\2/p' "$1" | grep -vx import | head -1 || true)"
	grep -qE "\b(http${name:+|$name})\.(Serve\(|Server\{)|\.(ListenAndServe|ListenAndServeTLS|ServeTLS)\(|\bMaybeWrapServer\(" "$1"
}

listeners=""
total=0
while IFS= read -r f; do
	[[ -f "$f" ]] || continue
	http_auth_is_listener "$f" || continue
	total=$((total + 1))
	wired=0
	for g in "$(dirname "$f")"/*.go; do
		case "$g" in *_test.go) continue ;; esac
		if grep -qE '^[[:space:]]*(import[[:space:]]+)?([A-Za-z_.]+[[:space:]]+)?"[^"]*/pkg/auth"' "$g"; then
			wired=1
			break
		fi
	done
	[[ "$wired" -eq 1 ]] || listeners+="$f"$'\n'
done <<<"$all_go"
((total > 0)) || { echo "[FAIL] no HTTP listeners found; refusing to pass vacuously" >&2; exit 2; }

current="$(printf '%s' "$listeners" | sort -u)"
baseline="$({ grep -vE '^[[:space:]]*(#|$)' "$BASELINE" || true; } | sort -u)"
new="$(set_comm -23 "$current" "$baseline")"
stale="$(set_comm -13 "$current" "$baseline")"

if [[ -n "$new" ]]; then
	echo "[FAIL] HTTP listeners with no pkg/auth in their package, not in $BASELINE:"
	printf '%s\n' "$new" | sed 's/^/  /'
	echo "Put the handler behind auth.Middleware, or take a row to review saying why it needs none."
fi
if [[ -n "$stale" ]]; then
	echo "[FAIL] rows in $BASELINE that no longer apply; delete them:"
	printf '%s\n' "$stale" | sed 's/^/  /'
fi
[[ -z "$new" && -z "$stale" ]] || exit 1
echo "[PASS] ${total} HTTP listeners: every one has pkg/auth in its package or a baseline row"
