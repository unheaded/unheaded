#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# unreached-evidence.sh: one TSV row of evidence per Go package that no
# binary imports (S-REAL-STEEL Phase 0, ADR-090 input). Read-only: it lists,
# greps and counts; it never edits, builds or deletes anything.
#
# Usage: scripts/sweep/unreached-evidence.sh <unreached.txt>
#   <unreached.txt>: one import path per line (unheaded/...), as produced by
#   the regeneration command in docs/UNREACHED-GO-PACKAGES.md.
#
# Columns:
#   package         import path
#   test_importers  _test.go files OUTSIDE the package that import it
#   prod_importers  non-test .go files outside the package that import it
#                   (for an unreached package these are other unreached ones)
#   fake_markers    simulat|placeholder|in (a )?real implementation|would (be|actually)
#                   |not yet implemented, in the package's non-test files
#                   ("n/a" when the package has no non-test .go file)
#   exported_refl   reflect.|//go:linkname|//export |cgo hits (invisible to the
#                   compiler: ADR-090 "build passes is not sufficient")
#   last_touched    date of the last commit touching the directory
#   lines           non-test line count (reported only in the sweep ledger,
#                   which ADR-090 allows for a deletion campaign)

set -euo pipefail

list="${1:?usage: $0 <unreached.txt>}"
[ -r "$list" ] || { echo "cannot read $list" >&2; exit 2; }

root="$(git rev-parse --show-toplevel)"
cd "$root"

fake_re='simulat|placeholder|in (a )?real implementation|would (be|actually)|not yet implemented'
refl_re='reflect\.|//go:linkname|//export |import "C"'

printf 'package\ttest_importers\tprod_importers\tfake_markers\texported_refl\tlast_touched\tlines\n'

while IFS= read -r pkg; do
  [ -n "$pkg" ] || continue
  case "$pkg" in unheaded/*) ;; *) echo "skip non-module path: $pkg" >&2; continue ;; esac
  dir="${pkg#unheaded/}"
  if [ ! -d "$dir" ]; then
    printf '%s\tn/a\tn/a\tn/a\tn/a\tn/a\tn/a\n' "$pkg"
    continue
  fi

  # Files of this package only (not sub-packages).
  mapfile -t prod < <(find "$dir" -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' | sort)

  # Importers: exact quoted import path, outside this directory.
  ti=$(grep -rl --include='*_test.go' -F "\"$pkg\"" . 2>/dev/null \
        | grep -v -E "^\./$dir/[^/]+$" | grep -c -v '/target/' || true)
  pi=$(grep -rl --include='*.go' --exclude='*_test.go' -F "\"$pkg\"" . 2>/dev/null \
        | grep -v -E "^\./$dir/[^/]+$" | grep -c -v '/target/' || true)

  if [ "${#prod[@]}" -eq 0 ]; then
    fm="n/a"; er="n/a"; lines=0
  else
    # grep -c exits 1 when a file has no match; that is a count of 0, not an error.
    fm=$( (grep -i -E -c "$fake_re" "${prod[@]}" 2>/dev/null || true) | awk -F: '{s+=$NF} END{print s+0}')
    er=$( (grep -E -c "$refl_re" "${prod[@]}" 2>/dev/null || true) | awk -F: '{s+=$NF} END{print s+0}')
    lines=$(cat "${prod[@]}" | wc -l)
  fi
  lt=$(git log -1 --format=%cs -- "$dir" 2>/dev/null || true)

  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$pkg" "$ti" "$pi" "$fm" "$er" "${lt:-n/a}" "$lines"
done < "$list"
