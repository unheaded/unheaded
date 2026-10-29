#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-dep-staleness.sh — unmaintained is a risk class of its own
# (S-LOW-FRUIT Phase 10). Every direct Go module whose latest release is
# more than 24 months old must have a row in docs/policy/dependency-watch.md
# with a decision (keep-pinned, replace-by or vendor); past 36 months the row
# must also name its findings-register entry (FND-NNN), because an advisory
# against a dead module never gets a fixed release.
#
# Needs the module proxy (cmd/depaudit asks proxy.golang.org); CI already
# reaches it for `go mod download`.
set -euo pipefail
cd "$(dirname "$0")/.."
watch=docs/policy/dependency-watch.md
[[ -f $watch ]] || { echo "FAIL: ${watch} missing"; exit 1; }

report=$(go list -m -json all | go run ./cmd/depaudit --max-age-months 24) || {
	echo "FAIL: depaudit could not complete (proxy unreachable?)"; echo "$report" | grep ERROR | head; exit 1; }

fail=0 stale=0
while read -r mod months; do
	stale=$((stale + 1))
	line=$(grep -F "| ${mod} |" "$watch" | head -1 || true)
	if [[ -z $line ]]; then
		echo "FAIL ${mod}: ${months} months since its last release and no row in ${watch}"; fail=1; continue
	fi
	if ! grep -qE 'keep-pinned|replace-by|vendor' <<<"$line"; then
		echo "FAIL ${mod}: row in ${watch} has no decision (keep-pinned / replace-by / vendor)"; fail=1
	fi
	if (( months > 36 )) && ! grep -qE 'FND-[0-9]+' <<<"$line"; then
		echo "FAIL ${mod}: ${months} months (>36) and its row names no findings-register entry (FND-NNN)"; fail=1
	fi
done < <(awk '$NF ~ /months\)/ && /STALE/ && / direct / {print $1, $5}' <<<"$report")
(( fail )) && exit 1
echo "OK: ${stale} stale direct modules, each with a recorded decision"
