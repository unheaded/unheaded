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

# Every User= is an account deploy/sysusers.d/unheaded.conf creates (its
# header promised this check; it did not exist until 2026-10-19), and
# Group= is that account's own group: a group two services share is a
# shared account. A unit whose user has no line fails to start on a fresh
# host ("Failed to determine user credentials").
SYSUSERS="${REPO_ROOT}/deploy/sysusers.d/unheaded.conf"
declare -A OTHER_ACCOUNTS=(
	[well-backup-copy]="east only, created by useradd with its chroot home (unheaded-well-offsite.service)"
)
accounts=0
for u in "$DIR"/*.service; do
	user="$(sed -n 's/^User=//p' "$u" | tail -1)"
	[[ -n "$user" ]] || continue
	grep -q '^DynamicUser=yes' "$u" && continue
	accounts=$((accounts + 1))
	if [[ -z "${OTHER_ACCOUNTS[$user]:-}" ]] && ! awk -v n="$user" '$1 == "u" && $2 == n { f = 1 } END { exit !f }' "$SYSUSERS"; then
		echo "[FAIL] $(basename "$u"): User=$user has no line in ${SYSUSERS#"$REPO_ROOT"/}"
		fail=1
	fi
	group="$(sed -n 's/^Group=//p' "$u" | tail -1)"
	if [[ -n "$group" && "$group" != "$user" ]]; then
		echo "[FAIL] $(basename "$u"): Group=$group is not User=$user's own group"
		fail=1
	fi
done
if [[ $fail -ne 0 ]]; then
	exit 1
fi
echo "[PASS] ${#units[@]} unit files verify cleanly; ${accounts} run as their own account (sysusers, or named above)"
