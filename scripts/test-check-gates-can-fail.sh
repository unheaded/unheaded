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

# Shards: every registered gate is in exactly one, and slow rows are spread.
git_clone reset -q --hard HEAD~1
gates_listed() { "$C/scripts/check-gates-can-fail.sh" "$@" | tail -n +2; }
gates_listed --list | sort >"$T/all.list"
for k in 1 2 3; do gates_listed --list --shard "$k/3" >"$T/shard$k.list"; done
sort "$T/shard1.list" "$T/shard2.list" "$T/shard3.list" >"$T/shards.list"
ok "the registry lists something" [ -s "$T/all.list" ]
ok "three shards together are every gate, once each" cmp -s "$T/all.list" "$T/shards.list"
shard_slow_rows() { grep -c ' slow ' "$T/shard$1.list"; }
ok "no shard carries all the slow gates" [ "$(shard_slow_rows 1)" -lt "$(grep -c ' slow ' "$T/all.list")" ]
ok "slow gates differ by at most one between shards" [ "$(($(shard_slow_rows 1) - $(shard_slow_rows 3)))" -le 1 ]
for bad in 4/3 0/3 three 1/0; do
	"$C/scripts/check-gates-can-fail.sh" --list --shard "$bad" >/dev/null 2>&1
	ok "--shard $bad is refused" [ $? -eq 2 ]
done
"$C/scripts/check-gates-can-fail.sh" --no-such-flag >/dev/null 2>&1
ok "an unknown flag is refused, not run as a full pass" [ $? -eq 2 ]

# check-ebpf-insns on a kernel its baseline was not measured on, which is what
# a hosted runner is: the row still proves the gate, needs no load, and says so.
mkdir "$T/elsewhere"
printf '#!/bin/sh\n[ "$1" = -r ] && { echo 0.0.0-elsewhere; exit 0; }\nexec /usr/bin/uname "$@"\n' >"$T/elsewhere/uname"
printf '#!/bin/sh\nexit 1\n' >"$T/elsewhere/sudo"
chmod +x "$T/elsewhere/uname" "$T/elsewhere/sudo"
rc=0
CI=true PATH="$T/elsewhere:$PATH" GATE_LOG_DIR="$T/kernel-logs" "$C/scripts/check-gates-can-fail.sh" --only check-ebpf-insns >"$T/kernel.out" 2>&1 || rc=$?
ok "in CI off the baseline's kernel the insns row still passes" [ "$rc" -eq 0 ]
ok "and it says the gate bit" grep -q 'check-ebpf-insns .*OK — bites when provoked' "$T/kernel.out"
ok "and that nothing was loaded" grep -q 'on the recorded numbers, no load' "$T/kernel.out"

# The same kernel outside CI: this host could load, and a baseline compared with
# itself proves nothing, so the row stays red. A stub stands in for the load.
printf '#!/bin/sh\necho "the load ran"\nexit 1\n' >"$C/scripts/check-ebpf-loads.sh"
git_clone commit -qam "a load that cannot run"
rc=0
env -u CI PATH="$T/elsewhere:$PATH" GATE_LOG_DIR="$T/host-logs" "$C/scripts/check-gates-can-fail.sh" --only check-ebpf-insns >"$T/host.out" 2>&1 || rc=$?
ok "outside CI a changed kernel fails the row" [ "$rc" -ne 0 ]
ok "and it says INCONCLUSIVE, not bit" grep -q 'check-ebpf-insns .*INCONCLUSIVE' "$T/host.out"
ok "and the load was tried" grep -q 'the load ran' "$T/host.out"
git_clone reset -q --hard HEAD~1

# The recorded-numbers settings stop at the insns row. The next row's gate is a
# stub that writes down what it was given; --only is widened to reach it.
next_row="$(gates_listed --list | awk '/^ *check-ebpf-insns /{getline; print $1}')"
ok "a row follows check-ebpf-insns" [ -n "$next_row" ]
sed -i "s/\[ \"\${name}\" != \"\${ONLY}\" \]/[ \"\${name}\" != \"\${ONLY}\" ] \&\& [ \"\${name}\" != \"$next_row\" ]/" "$C/scripts/check-gates-can-fail.sh"
printf '#!/bin/sh\necho "from=${EBPF_INSNS_FROM:-} any=${EBPF_INSNS_ANY_KERNEL:-}" >>"%s"\nexit 1\n' "$T/next-row" >"$C/scripts/$next_row.sh"
git_clone commit -qam "a row that writes down its environment"
CI=true PATH="$T/elsewhere:$PATH" GATE_LOG_DIR="$T/next-logs" "$C/scripts/check-gates-can-fail.sh" --only check-ebpf-insns >"$T/next.out" 2>&1
ok "the next row ran" [ -s "$T/next-row" ]
ok "and got none of the settings" grep -qx 'from= any=' "$T/next-row"
git_clone reset -q --hard HEAD~1

# A run leaves no empty log directory, however it ends.
mkdir "$T/tmp"
TMPDIR="$T/tmp" "$C/scripts/check-gates-can-fail.sh" --list >/dev/null 2>&1
ok "--list leaves no log directory" [ -z "$(ls -A "$T/tmp")" ]
: >"$C/dirty"
TMPDIR="$T/tmp" "$C/scripts/check-gates-can-fail.sh" --only check-unit-flags >/dev/null 2>&1
ok "a refused dirty tree exits 2" [ $? -eq 2 ]
ok "and leaves no log directory" [ -z "$(ls -A "$T/tmp")" ]

[ "$fail" -eq 0 ] || { tail -n 12 "$T/red.out" "$T/error.out" "$T/fail.out" "$T/kernel.out" "$T/host.out" "$T/next.out" | sed 's/^/  | /'; exit 1; }
echo "[PASS] meta-gate rows can be diagnosed"
