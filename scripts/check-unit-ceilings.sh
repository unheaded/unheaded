#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-unit-ceilings.sh — every systemd unit that runs one of our Go
# binaries has a memory ceiling the Go runtime knows about (S-LOW-FRUIT
# Phase 3): MemoryMax= and Environment=GOMEMLIMIT=. Without GOMEMLIMIT the
# GC ignores the cgroup limit and the kernel OOM-kills a heap the GC could
# have shrunk.
#
#   scripts/check-unit-ceilings.sh [UNIT_FILE...]   default: systemd/*.service deploy/systemd/*.service
set -euo pipefail
cd "$(dirname "$0")/.."

units=("$@")
if [[ ${#units[@]} -eq 0 ]]; then
	shopt -s nullglob
	units=(systemd/*.service deploy/systemd/*.service)
fi
[[ ${#units[@]} -eq 0 ]] && { echo "FAIL: no unit files found"; exit 1; }

# Names of our Go programs: the last path element of every main package.
mapfile -t gomains < <(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./... 2>/dev/null | sed 's#.*/##' | sort -u)
[[ ${#gomains[@]} -eq 0 ]] && { echo "FAIL: go list found no main packages"; exit 1; }
is_go() { local n=$1 m; for m in "${gomains[@]}"; do [[ $m == "$n" ]] && return 0; done; return 1; }

checked=0 fail=0
for u in "${units[@]}"; do
	exe=$(grep -m1 '^ExecStart=' "$u" | sed -E 's/^ExecStart=[-@+!:]*//' | awk '{print $1}')
	[[ -z $exe ]] && continue
	name=$(basename "$exe")
	is_go "$name" || continue
	checked=$((checked + 1))
	grep -q '^MemoryMax=' "$u" || { echo "FAIL ${u}: runs Go binary ${name} without MemoryMax="; fail=1; }
	grep -q '^Environment=GOMEMLIMIT=' "$u" || { echo "FAIL ${u}: runs Go binary ${name} without Environment=GOMEMLIMIT="; fail=1; }
done
(( checked == 0 )) && { echo "FAIL: no unit runs a Go binary (wrong directory?)"; exit 1; }
(( fail )) && exit 1
echo "OK: ${checked} Go units carry MemoryMax and GOMEMLIMIT"
