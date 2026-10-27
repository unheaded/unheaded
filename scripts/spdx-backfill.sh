#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# spdx-backfill.sh — add the two-line SPDX header to first-party source files
# that lack one. The fixer for scripts/check-spdx.sh; same scope (exclusions
# for vendored code, build output, fixtures: scripts/lib/spdx.sh).
#
#   scripts/spdx-backfill.sh [--dry-run]           # list "path<TAB>identifier"
#   scripts/spdx-backfill.sh --apply               # edit them
#   scripts/spdx-backfill.sh --apply --langs py,sh # default langs: go,rs
#
# Header: "SPDX-License-Identifier: <id>" + "Copyright (c) 2024-2026 Stevie
# Bellis." in the file's comment syntax. Go and Rust: lines 1-2 (above any
# //! doc block or #![...] attribute; comments may precede both). Shell and
# Python: after a #! line and a Python coding line, if present.
#
# Idempotent: a file that already has a header is never touched. Never edits:
#   - a file with a copyright or licence notice that does not name Stevie
#     Bellis (third-party text; reported as SKIP-THIRD-PARTY);
#   - test inputs and archives (SKIP-NO-EDIT, SPDX_NO_EDIT_RE in the lib);
#   - Doom files (DEFER: Doom is the lowest-priority track and is touched
#     only by a step that names it); they stay listed by check-spdx.sh.
# Dry-run prints only the files it would edit, one per line (wc -l = count);
# skips and the summary go to stderr.
set -euo pipefail

cd "$(dirname "$0")/.."
# shellcheck source=scripts/lib/spdx.sh
. scripts/lib/spdx.sh

DEFER_RE='(^|/)doom([-_/]|\.)'
THIRD_PARTY_RE='(Copyright|\([cC]\) *[0-9]{4}|©|Licensed under|SPDX-FileCopyrightText)'

mode=dry-run
langs="go,rs"
usage() { echo "usage: $0 [--dry-run|--apply] [--langs go,rs,py,sh]" >&2; exit 2; }
while [ $# -gt 0 ]; do
	case "$1" in
	--dry-run) mode=dry-run; shift ;;
	--apply) mode=apply; shift ;;
	--langs) [ $# -ge 2 ] || usage; langs=$2; shift 2 ;;
	--langs=*) langs=${1#--langs=}; shift ;;
	-h | --help) sed -n '4,25p' "$0"; exit 0 ;;
	*) usage ;;
	esac
done

# insert_header FILE ID: rewrite FILE in place (keeps mode and inode).
insert_header() {
	local f=$1 id=$2 c skip=0 tmp
	case "$f" in
	*.go | *.rs) c='//' ;;
	*) c='#' ;;
	esac
	if [ "$c" = '#' ]; then
		if head -n 1 -- "$f" | grep -q '^#!'; then skip=1; fi
		# PEP 263: a coding declaration must stay on line 1 or 2.
		if [ "$skip" -eq 1 ] && sed -n '2p' -- "$f" | grep -qE '^#.*coding[:=]'; then skip=2; fi
		if [ "$skip" -eq 0 ] && head -n 1 -- "$f" | grep -qE '^#.*coding[:=]'; then skip=1; fi
	fi
	tmp=$(mktemp "${TMPDIR:-/tmp}/spdx-backfill.XXXXXX")
	{
		[ "$skip" -gt 0 ] && head -n "$skip" -- "$f"
		printf '%s SPDX-License-Identifier: %s\n%s %s\n' "$c" "$id" "$c" "$SPDX_COPYRIGHT"
		tail -n +"$((skip + 1))" -- "$f"
	} >"$tmp"
	cat "$tmp" >"$f"
	rm -f "$tmp"
}

files=$(spdx_files "$langs") || exit 2
todo=0
done_n=0
skipped=0
deferred=0
while IFS= read -r f; do
	if [ -z "$f" ] || [ ! -f "$f" ]; then continue; fi
	spdx_has_header "$f" && continue
	if [[ $f =~ $DEFER_RE ]]; then
		echo "DEFER	$f	doom (lowest priority; header added by a step that names Doom)" >&2
		deferred=$((deferred + 1))
		continue
	fi
	if [[ $f =~ $SPDX_NO_EDIT_RE ]]; then
		echo "SKIP-NO-EDIT	$f	test input or archive (scripts/lib/spdx.sh SPDX_NO_EDIT_RE)" >&2
		skipped=$((skipped + 1))
		continue
	fi
	if grep -E "$THIRD_PARTY_RE" -- "$f" | grep -qv 'Stevie Bellis'; then
		echo "SKIP-THIRD-PARTY	$f	$(grep -m1 -E "$THIRD_PARTY_RE" -- "$f" | cut -c1-80)" >&2
		skipped=$((skipped + 1))
		continue
	fi
	id=$(spdx_id_for "$f")
	todo=$((todo + 1))
	if [ "$mode" = apply ]; then
		insert_header "$f" "$id"
		done_n=$((done_n + 1))
		echo "added	$f	$id"
	else
		echo "$f	$id"
	fi
done <<<"$files"

echo "spdx-backfill: $mode langs=$langs to-add=$todo added=$done_n skipped=$skipped deferred=$deferred" >&2
