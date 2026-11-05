#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# The meta-gate's rows can be diagnosed: a gate already red on a clean tree
# gets an INCONCLUSIVE row that names a file holding its output and shows the
# tail, and a clean verdict leaves no file behind. Run in a clone, because the
# meta-gate needs a clean tree and this needs a red gate. The meta-gate under
# test is this checkout's, committed there.
set -uo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
T="$(mktemp -d "${TMPDIR:-/var/tmp}/test-gates-can-fail.XXXXXX")"
trap 'rm -rf "$T"' EXIT
C="$T/clone"
git_clone() { command git -C "$C" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false "$@"; }

command git clone -q --shared --no-tags "$REPO_ROOT" "$C" || exit 1
cp "$REPO_ROOT/scripts/check-gates-can-fail.sh" "$C/scripts/check-gates-can-fail.sh"
git_clone commit -qam "the meta-gate under test" --allow-empty

fail=0
ok() { # ok DESC COMMAND...
	local desc=$1
	shift
	if "$@"; then printf '[ok] %s\n' "$desc"; else printf '[FAIL] %s\n' "$desc"; fail=1; fi
}

ok "the gate passes in the clone" env GATE_LOG_DIR="$T/clean-logs" "$C/scripts/check-gates-can-fail.sh" --only check-unit-flags >"$T/clean.out" 2>&1
ok "a clean verdict leaves no output behind" [ -z "$(ls -A "$T/clean-logs" 2>/dev/null)" ]

sed -i 's|^    --http-port 18000 \\$|    --no-such-flag \\\n&|' "$C/deploy/systemd/unheaded-wotan.service"
git_clone commit -qam "a red gate"
rc=0
GATE_LOG_DIR="$T/red-logs" "$C/scripts/check-gates-can-fail.sh" --only check-unit-flags >"$T/red.out" 2>&1 || rc=$?
ok "a gate red on a clean tree fails the meta-gate" [ "$rc" -ne 0 ]
ok "and its row says INCONCLUSIVE" grep -q 'INCONCLUSIVE' "$T/red.out"
log="$(sed -n 's/^ *output: //p' "$T/red.out" | head -n 1)"
ok "the row names a file that exists" [ -f "${log:-/nonexistent}" ]
ok "the file holds the gate's own output" grep -q 'wotan has no flag -no-such-flag' "${log:-/dev/null}"
ok "the row shows its tail, for a run whose disk is gone" grep -q '| .*wotan has no flag -no-such-flag' "$T/red.out"

[ "$fail" -eq 0 ] || { sed 's/^/  | /' "$T/red.out" | tail -n 12; exit 1; }
echo "[PASS] meta-gate rows can be diagnosed"
