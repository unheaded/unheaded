#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# check-spdx.sh — every first-party source file carries an SPDX licence header.
#
# Scans tracked *.go, *.rs, *.py, *.sh files (scope and exclusions in
# scripts/lib/spdx.sh: vendored llama.cpp/, doom/, crates/xv6-mbc/upstream/,
# build output, test fixtures, docs/archive/). A header is an
# "SPDX-License-Identifier: <id>" line in a file's first 25 lines.
#
#   scripts/check-spdx.sh                    # go,rs: the languages CI gates
#   scripts/check-spdx.sh --langs py,sh      # report other languages (go,rs,py,sh)
#   scripts/check-spdx.sh --forbid MIT       # also fail on a header declaring MIT
#
# --forbid ID: a header line that declares exactly ID fails too, unless the
# path is listed in docs/legal/MIT-KEEP.txt (ID MIT only). First-party code
# was relicensed MIT -> GPL-3.0-or-later (decision 12, 2026-10-27); this keeps
# the MIT count at the KEEP list.
#
# Prints each offending file, then one summary line.
# Exit: 0 clean, 1 a header missing (or forbidden), 2 usage error.
# Fix: scripts/spdx-backfill.sh --apply --langs <same langs>.
set -euo pipefail

cd "$(dirname "$0")/.."
# shellcheck source=scripts/lib/spdx.sh
. scripts/lib/spdx.sh

usage() { echo "usage: $0 [--langs go,rs,py,sh] [--forbid ID]   (default --langs go,rs)" >&2; exit 2; }
langs="go,rs"
forbid=""
while [ $# -gt 0 ]; do
	case "$1" in
	--langs) [ $# -ge 2 ] || usage; langs=$2; shift 2 ;;
	--langs=*) langs=${1#--langs=}; shift ;;
	--forbid) [ $# -ge 2 ] || usage; forbid=$2; shift 2 ;;
	--forbid=*) forbid=${1#--forbid=}; shift ;;
	-h | --help) sed -n '4,23p' "$0"; exit 0 ;;
	*) usage ;;
	esac
done
[[ -z $forbid || $forbid =~ ^[A-Za-z0-9.+-]+$ ]] || usage

# keep_listed PATH: 0 if docs/legal/MIT-KEEP.txt names PATH or a parent dir.
keep_listed() {
	local p=$1 k
	[ "$forbid" = MIT ] && [ -f docs/legal/MIT-KEEP.txt ] || return 1
	while IFS= read -r k; do
		k=${k%%#*}
		k=${k//[[:space:]]/}
		[ -n "$k" ] || continue
		if [ "$p" = "$k" ] || [[ $p == "${k%/}/"* ]]; then return 0; fi
	done <docs/legal/MIT-KEEP.txt
	return 1
}

# A declaration, not a mention: the identifier alone on a comment line.
forbid_re="^[[:space:]]*(#|//|/\\*|\\*)?[[:space:]]*SPDX-License-Identifier:[[:space:]]*${forbid//./\\.}[[:space:]]*(\\*/)?[[:space:]]*\$"

files=$(spdx_files "$langs") || exit 2
total=0
missing=0
forbidden=0
while IFS= read -r f; do
	if [ -z "$f" ] || [ ! -f "$f" ]; then continue; fi
	total=$((total + 1))
	if ! spdx_has_header "$f"; then
		echo "$f"
		missing=$((missing + 1))
	elif [ -n "$forbid" ] && head -n "$SPDX_HEAD_LINES" -- "$f" | grep -qE "$forbid_re" && ! keep_listed "$f"; then
		echo "$f	declares $forbid"
		forbidden=$((forbidden + 1))
	fi
done <<<"$files"

if [ "$missing" -gt 0 ] || [ "$forbidden" -gt 0 ]; then
	echo "check-spdx: FAIL $missing of $total file(s) missing an SPDX header${forbid:+, $forbidden declaring $forbid} (langs: $langs); fix: scripts/spdx-backfill.sh --apply --langs $langs"
	exit 1
fi
echo "check-spdx: OK $total file(s) carry an SPDX header${forbid:+, none declares $forbid} (langs: $langs)"
