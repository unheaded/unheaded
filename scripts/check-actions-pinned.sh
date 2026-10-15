#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# ADR-095 track two, step 4: every GitHub Action is pinned to a full commit
# SHA. A tag or branch (`@v5`, `@master`) can be moved by whoever controls
# that repository, and the next run executes the new code with this repo's
# token: the tag-hijack class, closed outright by pinning. The comment after
# the SHA records the tag it was taken from (`@<sha> # v5`). Local actions
# (`uses: ./...`) and docker:// images are out of scope here.
set -euo pipefail
cd "$(dirname "$0")/.."

shopt -s nullglob
files=(.github/workflows/*.yml .github/workflows/*.yaml)
((${#files[@]} > 0)) || { echo "check-actions-pinned: no workflows found; refusing to pass vacuously" >&2; exit 2; }

bad=$(grep -nE '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*' "${files[@]}" |
	grep -vE 'uses:[[:space:]]*(\./|docker://)' |
	grep -vE 'uses:[[:space:]]*[^@[:space:]]+@[0-9a-f]{40}([[:space:]]|$)' || true)
total=$(grep -chE '^[[:space:]]*(-[[:space:]]+)?uses:' "${files[@]}" | awk '{s+=$1} END {print s+0}')
((total > 0)) || { echo "check-actions-pinned: no uses: lines found; refusing to pass vacuously" >&2; exit 2; }

if [[ -n "$bad" ]]; then
	echo "check-actions-pinned: actions not pinned to a commit SHA:"
	printf '%s\n' "$bad" | sed 's/^/  /'
	echo "Pin with: git ls-remote https://github.com/<owner>/<repo> 'refs/tags/<tag>^{}' and write @<sha> # <tag>"
	exit 1
fi
echo "[PASS] all $total uses: lines pinned to a commit SHA"
