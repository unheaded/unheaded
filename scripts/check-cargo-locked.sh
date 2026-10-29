#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-cargo-locked.sh — every cargo build/test/clippy/run in the Makefiles
# and CI workflows passes --locked (S-LOW-FRUIT Phase 10): a build must fail
# rather than silently resolve a different dependency than Cargo.lock says.
# The Rust half of GOFLAGS=-mod=readonly. Comment lines and scripts/dev-*
# are not checked; a line that uses $(CARGO_BUILD_FLAGS) counts as locked
# when the Makefile's CARGO_BUILD_FLAGS carries --locked.
set -euo pipefail
cd "$(dirname "$0")/.."
mapfile -t files < <(git ls-files 'Makefile' '*/Makefile' '*/Makefile.*' '.github/workflows/*.yml' | grep -v '^scripts/dev-')
flags_locked=0
grep -qE '^CARGO_BUILD_FLAGS\s*:?=.*--locked' Makefile && flags_locked=1

fail=0 n=0
while IFS= read -r hit; do
	file=${hit%%:*}; rest=${hit#*:}; line=${rest#*:}
	[[ $line =~ ^[[:space:]]*# ]] && continue
	n=$((n + 1))
	[[ $line == *--locked* ]] && continue
	if [[ $line == *'$(CARGO_BUILD_FLAGS)'* && $flags_locked -eq 1 && $file == Makefile ]]; then continue; fi
	echo "FAIL ${file}:${rest%%:*}: cargo without --locked: ${line#"${line%%[![:space:]]*}"}"
	fail=1
done < <(grep -nE '\bcargo (build|test|clippy|run)\b' "${files[@]}" /dev/null || true)
(( n == 0 )) && { echo "FAIL: found no cargo invocations (wrong directory?)"; exit 1; }
(( fail )) && exit 1
echo "OK: ${n} cargo invocations are --locked"
