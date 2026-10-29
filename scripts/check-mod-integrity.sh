#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-mod-integrity.sh — module integrity (S-LOW-FRUIT Phase 10):
#   1. go mod verify: every module in the cache matches what was downloaded
#   2. go.mod, go.sum and every tracked Cargo.lock are as committed (a
#      build or tool must not rewrite lock files on the side)
# Run it after a build in CI, or before committing a dependency change
# (then commit the lock change first).
set -euo pipefail
cd "$(dirname "$0")/.."
fail=0
if ! out=$(go mod verify 2>&1); then
	echo "FAIL go mod verify:"; echo "$out" | head -20; fail=1
fi
mapfile -t locks < <(git ls-files go.mod go.sum '*Cargo.lock')
if ! git diff --quiet -- "${locks[@]}"; then
	echo "FAIL: lock files differ from the commit:"
	git diff --stat -- "${locks[@]}"
	fail=1
fi
(( fail )) && exit 1
echo "OK: go mod verify; ${#locks[@]} lock files as committed"
