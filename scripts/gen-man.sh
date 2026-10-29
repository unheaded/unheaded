#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# gen-man.sh — write man/NAME.1 for every Go binary from its hidden --man
# (pkg/cmdline.Man, S-LOW-FRUIT Phase 5). Native: no outside generator.
#
#   scripts/gen-man.sh           check: build every main under ./cmd/ into a
#                                temp dir; fail if man/ is out of date
#   scripts/gen-man.sh --write   regenerate man/ (make man)
#
# SOURCE_DATE_EPOCH = the last commit's time, so pages are reproducible.
# Binaries listed in docs/policy/man-missing-baseline.txt may not answer
# yet; the list only shrinks: a new binary without --man fails, and so does
# a listed one that now answers.
set -euo pipefail
cd "$(dirname "$0")/.."
check=1
[[ ${1:-} == --write ]] && check=0

baseline=docs/policy/man-missing-baseline.txt
mapfile -t missing_ok < <(grep -vE '^\s*(#|$)' "$baseline")
listed() { local n=$1 m; for m in "${missing_ok[@]}"; do [[ $m == "$n" ]] && return 0; done; return 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
ver=$(git describe --tags --always 2>/dev/null || echo dev)
go build -trimpath -ldflags "-X unheaded/pkg/cmdline.Version=${ver}" -o "$tmp/bin/" ./cmd/...
export SOURCE_DATE_EPOCH
SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)

mkdir -p "$tmp/man"
fail=0 pages=0
while read -r n; do
	if out=$("$tmp/bin/$n" --man 2>/dev/null) && [[ $out == .TH* ]]; then
		if listed "$n"; then
			echo "FAIL $n: answers --man now; remove it from $baseline"
			fail=1
		fi
		printf '%s\n' "$out" >"$tmp/man/$n.1"
		pages=$((pages + 1))
	elif ! listed "$n"; then
		echo "FAIL $n: does not answer --man (use cmdline.Parse/ParseSet, or call cmdline.Man)"
		fail=1
	fi
done < <(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./cmd/... | sed 's#.*/##')
(( fail )) && exit 1

if (( check )); then
	# The .TH line carries the commit date and git describe, which move with
	# every commit; compare everything below it.
	stale=0
	for f in "$tmp"/man/*.1; do
		n=$(basename "$f")
		if [[ ! -f man/$n ]] || ! cmp -s <(sed 1d "$f") <(sed 1d "man/$n"); then
			echo "stale: man/$n"; stale=1
		fi
	done
	for f in man/*.1; do [[ -f $tmp/man/$(basename "$f") ]] || { echo "extra: $f"; stale=1; }; done
	if (( stale )); then
		echo "FAIL: man/ is out of date; run scripts/gen-man.sh"
		exit 1
	fi
	echo "OK: man/ matches ${pages} binaries"
	exit 0
fi
rm -rf man
cp -r "$tmp/man" man
echo "OK: ${pages} man pages in man/ (${#missing_ok[@]} binaries still listed in ${baseline})"
