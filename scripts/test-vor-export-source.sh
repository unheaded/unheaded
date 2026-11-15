#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-vor-export-source.sh: tests for scripts/vor-export-source.sh, in a temp
# repository with a temp destination. Nothing outside the temp directory is
# read or written.
#
#   T1: export of HEAD writes the tree; .vor-export and <dest>.exported both
#       hold the full commit hash.
#   T2: "<dest> develop" exports develop while HEAD is on another branch.
#   T3: a destination that is not an earlier export is refused (exit 2) and
#       no stamp is written.
#   T4: the stamp is written after the swap and by rename: a reader sees the
#       old hash or the new one, never part of one.
#   T5: an unknown ref exits non-zero and leaves the export and the stamp as
#       they were.
#
# Run: bash scripts/test-vor-export-source.sh

set -euo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/vor-export-source.sh"

# Run from a real hook or a linked worktree, these would point every git call
# below at the caller's repository.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

REPO="$TMP/repo"
OUT="$TMP/out"
DEST="$OUT/unheaded"
STAMP="$DEST.exported"
mkdir -p "$OUT"

g() { git -c commit.gpgsign=false -c user.name=t -c user.email=t@example.invalid "$@"; }

commit() { # commit FILE: one new commit touching FILE
	echo "$1" >>"$1"
	g add -- "$1"
	g commit -q -m "$1"
}

fails=0
result() { # result NAME STATUS
	if [[ "$2" == 0 ]]; then
		echo "[PASS] $1"
	else
		echo "[FAIL] $1"
		fails=$((fails + 1))
	fi
}

git init -q -b develop "$REPO"
cd "$REPO"
# The script exports the repository it sits in, so the test runs a copy of it
# from inside the temp repository. It is not tracked there, like bin/ in the
# hook's test.
mkdir scripts
cp "$SCRIPT" scripts/vor-export-source.sh
export_to() { bash "$REPO/scripts/vor-export-source.sh" "$@" >"$TMP/stdout" 2>"$TMP/stderr"; }

mkdir docs
commit docs/a.md
first="$(g rev-parse HEAD)"

# T1
rc=0
export_to "$DEST" || rc=1
[[ -f "$DEST/docs/a.md" ]] || rc=1
[[ ! -e "$DEST/scripts" ]] || rc=1 # untracked files are not exported
[[ "$(cat "$DEST/.vor-export" 2>/dev/null)" == "$first" ]] || rc=1
[[ "$(cat "$STAMP" 2>/dev/null)" == "$first" ]] || rc=1
[[ "${#first}" == 40 ]] || rc=1
result T1 "$rc"

# T2: develop moves on, HEAD is on a branch with a file develop lacks.
commit docs/b.md
develop="$(g rev-parse HEAD)"
g checkout -q -b side
commit docs/side.md
rc=0
export_to "$DEST" develop || rc=1
[[ -f "$DEST/docs/b.md" ]] || rc=1
[[ ! -e "$DEST/docs/side.md" ]] || rc=1
[[ "$(cat "$DEST/.vor-export" 2>/dev/null)" == "$develop" ]] || rc=1
[[ "$(cat "$STAMP" 2>/dev/null)" == "$develop" ]] || rc=1
result T2 "$rc"

# T3: a directory that no export made, and no stamp beside it afterwards.
mkdir -p "$TMP/other/unheaded"
echo keep >"$TMP/other/unheaded/mine.txt"
rc=0
got=0
export_to "$TMP/other/unheaded" develop || got=$?
[[ "$got" == 2 ]] || rc=1
[[ "$(cat "$TMP/other/unheaded/mine.txt")" == keep ]] || rc=1
[[ ! -e "$TMP/other/unheaded.exported" ]] || rc=1
[[ ! -e "$TMP/other/unheaded/.vor-export" ]] || rc=1
result T3 "$rc"

# T4: the stamp is not older than the export it names, it is a new file each
# time (a rename, so the old one is whole until the new one is whole), and no
# temp file is left beside it.
before_inode="$(stat -c %i "$STAMP")"
rc=0
export_to "$DEST" side || rc=1
side="$(g rev-parse side)"
[[ "$(cat "$STAMP")" == "$side" ]] || rc=1
[[ "$(stat -c %i "$STAMP")" != "$before_inode" ]] || rc=1
[[ ! "$STAMP" -ot "$DEST/.vor-export" ]] || rc=1
[[ "$(find "$OUT" -mindepth 1 -maxdepth 1 | sort | tr '\n' ' ')" == "$DEST $STAMP " ]] || rc=1
[[ "$(wc -l <"$STAMP")" == 1 ]] || rc=1
result T4 "$rc"

# T5: an unknown ref; a ref that is an option; both leave everything alone.
before_stamp="$(cat "$STAMP")"
before_inode="$(stat -c %i "$STAMP")"
rc=0
for bad in no-such-ref --help ''; do
	got=0
	if [[ -n "$bad" ]]; then
		export_to "$DEST" "$bad" || got=$?
	else
		export_to "$DEST" "refs/heads/" || got=$?
	fi
	[[ "$got" != 0 ]] || rc=1
done
[[ "$(cat "$STAMP")" == "$before_stamp" ]] || rc=1
[[ "$(stat -c %i "$STAMP")" == "$before_inode" ]] || rc=1
[[ "$(cat "$DEST/.vor-export")" == "$side" ]] || rc=1
[[ -f "$DEST/docs/side.md" ]] || rc=1
[[ "$(find "$OUT" -mindepth 1 -maxdepth 1 | sort | tr '\n' ' ')" == "$DEST $STAMP " ]] || rc=1
result T5 "$rc"

exit $((fails > 0))
