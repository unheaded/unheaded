#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Every unit file in deploy/systemd is read by systemd exactly as written.
# systemd does not fail on a directive it cannot use: it logs "... is not
# known, ignoring" and runs the service without it. That is how 13 units shipped
# `SystemCallFilter=~@privileged ~@resources ...`, where only @privileged was
# ever denied (FND-003, ADR-098). This gate runs `systemd-analyze verify` on
# each unit and fails on any complaint about the unit itself, except that its
# binary is not installed on this machine.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIR="${1:-${REPO_ROOT}/deploy/systemd}"
command -v systemd-analyze >/dev/null || { echo "check-systemd-units: systemd-analyze not found" >&2; exit 2; }

shopt -s nullglob
units=("$DIR"/*.service "$DIR"/*.timer "$DIR"/*.path "$DIR"/*.socket)
[[ ${#units[@]} -gt 0 ]] || { echo "check-systemd-units: no units in $DIR; refusing to pass vacuously" >&2; exit 2; }

fail=0
for u in "${units[@]}"; do
	# Only lines about this file; the binary is expected to be absent here.
	problems="$(systemd-analyze verify "$u" 2>&1 | grep -F "$u" | grep -v -e 'is not executable' -e 'No such file or directory' || true)"
	if [[ -n "$problems" ]]; then
		echo "[FAIL] $(basename "$u")"
		printf '%s\n' "$problems" | sed 's/^/       /'
		fail=1
	fi
done
if [[ $fail -ne 0 ]]; then
	echo "check-systemd-units: systemd would silently ignore the directives above"
	exit 1
fi
echo "[PASS] ${#units[@]} unit files verify cleanly"
