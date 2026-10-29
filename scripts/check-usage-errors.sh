#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-usage-errors.sh — every Go binary answers an unknown option the
# ADR-111 way: "NAME: MESSAGE" and the --help hint on stderr, exit 2, and
# never the flag package's raw "Usage of" dump.
#
#   scripts/check-usage-errors.sh BIN...          check these binaries
#   scripts/check-usage-errors.sh --bin-dir DIR   check DIR/<name> for every main package under ./cmd/
#   scripts/check-usage-errors.sh                 build every main under ./cmd/ into a temp dir, then check them
#
# Each binary runs once with a made-up option, stdin closed, under a 5 s
# timeout (a binary that ignored the option and started serving fails).
set -euo pipefail
cd "$(dirname "$0")/.."

mains() { go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./cmd/... | sed 's#.*/##'; }

bins=()
if [[ $# -eq 0 ]]; then
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	go build -trimpath -o "$tmp/" ./cmd/...
	while read -r n; do bins+=("$tmp/$n"); done < <(mains)
elif [[ $1 == --bin-dir ]]; then
	dir=${2:?--bin-dir needs a directory}
	while read -r n; do bins+=("$dir/$n"); done < <(mains)
else
	bins=("$@")
fi

opt=--no-such-option-adr111
fail=0
for b in "${bins[@]}"; do
	name=$(basename "$b")
	if [[ ! -x $b ]]; then
		echo "FAIL $name: no binary at $b"; fail=1; continue
	fi
	rc=0
	out=$(timeout 5 "$b" "$opt" </dev/null 2>&1 >/dev/null) || rc=$?
	if [[ $out == *"Usage of"* || $out == *"flag provided but not defined"* ]]; then
		echo "FAIL $name: prints the flag package's raw usage dump"; fail=1; continue
	fi
	if (( rc != 2 )); then
		echo "FAIL $name: exit $rc on $opt, want 2: ${out%%$'\n'*}"; fail=1; continue
	fi
	if [[ $out != *"Try '"*"--help' for more information."* ]]; then
		echo "FAIL $name: no --help hint in: ${out%%$'\n'*}"; fail=1
	fi
done
if (( fail )); then exit 1; fi
echo "OK: ${#bins[@]} binaries refuse an unknown option with one line and the --help hint"
