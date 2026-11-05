#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# The meta-gate's rows can be diagnosed: an INCONCLUSIVE, ERROR or FAIL row
# names a file holding the output behind it and shows the tail, a clean verdict
# leaves no file behind, and no exit leaves an empty log directory. Run in a clone, because the
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
row_diagnosed() { # row_diagnosed WORD TEXT OUT: its row names a file with TEXT and shows it
	local log
	log="$(sed -n 's/^ *output: //p' "$3" | head -n 1)"
	ok "$1: the row names a file that exists" [ -f "${log:-/nonexistent}" ]
	ok "$1: the file holds the gate's own output" grep -q "$2" "${log:-/dev/null}"
	ok "$1: the row shows its tail, for a run whose disk is gone" grep -q "| .*$2" "$3"
}
row_diagnosed INCONCLUSIVE 'wotan has no flag -no-such-flag' "$T/red.out"

# ERROR: the provocation's anchor is gone, and the gate still passes clean.
git_clone reset -q --hard HEAD~1
sed -i 's|^    --http-port 18000 \\$|    --http-port=18000 \\|' "$C/deploy/systemd/unheaded-wotan.service"
git_clone commit -qam "an anchor gone"
GATE_LOG_DIR="$T/error-logs" "$C/scripts/check-gates-can-fail.sh" --only check-unit-flags >"$T/error.out" 2>&1
ok "a provocation that cannot be applied fails the meta-gate" [ $? -ne 0 ]
ok "and its row says ERROR" grep -q 'ERROR' "$T/error.out"
row_diagnosed ERROR 'provocation anchor missing' "$T/error.out"

# FAIL: a gate that passes whatever it is shown.
git_clone reset -q --hard HEAD~1
sed -i 's|^set -euo pipefail$|&\necho "fixture: passes regardless"; exit 0|' "$C/scripts/check-unit-flags.sh"
git_clone commit -qam "a gate that cannot fail"
GATE_LOG_DIR="$T/fail-logs" "$C/scripts/check-gates-can-fail.sh" --only check-unit-flags >"$T/fail.out" 2>&1
ok "a gate that passes while violated fails the meta-gate" [ $? -ne 0 ]
ok "and its row says FAIL" grep -q 'FAIL — PASSED WHILE VIOLATED' "$T/fail.out"
row_diagnosed FAIL 'fixture: passes regardless' "$T/fail.out"

# A run leaves no empty log directory, however it ends.
git_clone reset -q --hard HEAD~1
mkdir "$T/tmp"
TMPDIR="$T/tmp" "$C/scripts/check-gates-can-fail.sh" --list >/dev/null 2>&1
ok "--list leaves no log directory" [ -z "$(ls -A "$T/tmp")" ]
: >"$C/dirty"
TMPDIR="$T/tmp" "$C/scripts/check-gates-can-fail.sh" --only check-unit-flags >/dev/null 2>&1
ok "a refused dirty tree exits 2" [ $? -eq 2 ]
ok "and leaves no log directory" [ -z "$(ls -A "$T/tmp")" ]

[ "$fail" -eq 0 ] || { tail -n 12 "$T/red.out" "$T/error.out" "$T/fail.out" | sed 's/^/  | /'; exit 1; }
echo "[PASS] meta-gate rows can be diagnosed"
