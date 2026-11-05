#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# scripts/lib/fetch-tool.sh against a local fixture: a binary one caller was
# handed keeps running while other callers fetch the same tool. The cache is
# shared by every gate on a host, and gates run side by side, so the binary
# must be replaced in one rename, for the archive form and the bare form.
set -uo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/fetch-tool.sh
source "$REPO_ROOT/scripts/lib/fetch-tool.sh"

T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
export XDG_CACHE_HOME="$T/cache"
fail=0
URL="https://fetch-tool-test.invalid/none" # the fixture is cached: nothing is fetched

# Native code, so a write in progress shows as the kernel refusing the exec.
fixture="$(command -v bash)"
mkdir "$T/src"
cp "$fixture" "$T/src/fixture"
tar -czf "$T/fixture.tar.gz" -C "$T/src" fixture

survives_refetch() { # NAME MEMBER: run the binary while another caller refetches
	local name=$1 member=$2 src=$3 sha bin writer i bad=0 rc
	sha="$(sha256sum "$src" | cut -d' ' -f1)"
	mkdir -p "$XDG_CACHE_HOME/unheaded-tools/$name-1"
	cp "$src" "$XDG_CACHE_HOME/unheaded-tools/$name-1/$name.tar.gz"
	bin="$(fetch_tool "$name" 1 "$URL" "$sha" "$member")" || { echo "[FAIL] $name: first fetch failed"; fail=1; return; }
	(for i in $(seq 1 60); do fetch_tool "$name" 1 "$URL" "$sha" "$member" >/dev/null || exit 1; done) &
	writer=$!
	while kill -0 "$writer" 2>/dev/null; do
		rc=0
		"$bin" -c 'exit 0' 2>/dev/null || rc=$?
		[ "$rc" -eq 0 ] || bad=$((bad + 1))
	done
	wait "$writer" || { echo "[FAIL] $name: a refetch failed"; fail=1; }
	if [ "$bad" -ne 0 ]; then
		echo "[FAIL] $name: $bad runs refused while another caller fetched"
		fail=1
	else
		echo "[ok] $name keeps running through refetches"
	fi
	if compgen -G "$XDG_CACHE_HOME/unheaded-tools/$name-1/.extract.*" >/dev/null; then
		echo "[FAIL] $name: a temporary directory was left in the cache"
		fail=1
	fi
}

survives_refetch archive fixture "$T/fixture.tar.gz"
survives_refetch bare - "$fixture"

# A swapped archive still never runs.
mkdir -p "$XDG_CACHE_HOME/unheaded-tools/swapped-1"
echo "not the release" >"$XDG_CACHE_HOME/unheaded-tools/swapped-1/swapped.tar.gz"
if fetch_tool swapped 1 "$URL" 0000 fixture >/dev/null 2>&1; then
	echo "[FAIL] a swapped archive was accepted"
	fail=1
else
	echo "[ok] a swapped archive is refused"
fi

[ "$fail" -eq 0 ] || exit 1
echo "[PASS] fetch-tool keeps a running binary whole"
