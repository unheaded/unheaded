#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-http-timeouts.sh — every non-test http.Server literal sets
# ReadHeaderTimeout (Slowloris; S-LOW-FRUIT Phase 2). A server without it
# holds a connection open for as long as a client trickles header bytes.
#
#   scripts/check-http-timeouts.sh [DIR...]   default: cmd services pkg internal
#
# Looks 15 lines past each "http.Server{" for the field (literals here are
# all shorter); prints each offender as FILE:LINE and exits 1 if any.
set -euo pipefail
cd "$(dirname "$0")/.."
dirs=("$@")
[[ ${#dirs[@]} -eq 0 ]] && dirs=(cmd services pkg internal)
existing=()
for d in "${dirs[@]}"; do [[ -e $d ]] && existing+=("$d"); done
[[ ${#existing[@]} -eq 0 ]] && { echo "FAIL: none of ${dirs[*]} exists"; exit 1; }

offenders=0 total=0
while IFS=: read -r f l _; do
	total=$((total + 1))
	if ! sed -n "${l},$((l + 15))p" "$f" | grep -q 'ReadHeaderTimeout'; then
		echo "FAIL ${f}:${l}: http.Server without ReadHeaderTimeout"
		offenders=$((offenders + 1))
	fi
done < <(grep -rn --include='*.go' 'http\.Server{' "${existing[@]}" | grep -v '_test\.go:')

if (( total == 0 )); then
	echo "FAIL: no http.Server literal found under ${existing[*]} (wrong directory?)"
	exit 1
fi
if (( offenders )); then
	echo "${offenders} of ${total} http.Server literals lack ReadHeaderTimeout"
	exit 1
fi
echo "OK: ${total} http.Server literals set ReadHeaderTimeout"
