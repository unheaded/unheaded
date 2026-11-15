#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# test-post-commit.sh — tests for scripts/git-hooks/post-commit, in a temp repo
# with a stub bin/zhen-codeindex that records its arguments, and a copy of
# scripts/vor-export-source.sh exporting to a temp directory.
#
#   T1: commit on develop           → the stub runs with --repo.
#   T2: commit on another branch    → the stub does not run.
#   T3: stub missing                → the commit still exits 0.
#   T4: git merge --no-ff on develop → the stub runs (post-merge).
#   T5: commit on develop, VOR_EXPORT_DEST an earlier export → develop is
#       exported (the stamp changes within 5 s).
#   T6: commit on another branch    → nothing is exported.
#   T7: VOR_EXPORT_DEST not an export → the commit exits 0, nothing written.
#
# VOR_EXPORT_DEST is set for every test. Unset, the hook's default is the live
# export on a Kingdom host, and a test repository must never be exported there.
#
# Run: bash scripts/git-hooks/test-post-commit.sh

set -euo pipefail

HOOK="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/post-commit"

# Run from a real hook or a linked worktree, these would point every git call
# below at the caller's repository.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export MARK="$TMP/mark"
export ZHEN_CODE_INDEX_DIR="$TMP/index"
# Not an export until T5 makes it one, so T1-T4 export nothing.
export VOR_EXPORT_DEST="$TMP/vor/unheaded"
VOR_STAMP="$VOR_EXPORT_DEST.exported"
EXPORTER="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/vor-export-source.sh"
REPO="$TMP/repo"
STUB="$REPO/bin/zhen-codeindex"

g() { git -c commit.gpgsign=false -c user.name=t -c user.email=t@example.invalid "$@"; }

commit() { # commit FILE: one new commit touching FILE
	echo "$1" >>"$1"
	g add -- "$1"
	g commit -q -m "$1"
}

# Up to 3 seconds for the backgrounded stub to leave a line with --repo.
called() {
	local i
	for ((i = 0; i < 30; i++)); do
		if grep -q -- '--repo' "$MARK" 2>/dev/null; then
			return 0
		fi
		sleep 0.1
	done
	return 1
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
ln -s "$HOOK" .git/hooks/post-commit
ln -s "$HOOK" .git/hooks/post-merge
mkdir bin
cat >"$STUB" <<'SH'
#!/bin/sh
echo "$@" >> "$MARK"
SH
chmod +x "$STUB"

# T1
: >"$MARK"
commit a
rc=0
called || rc=1
result T1 "$rc"

# T2: the full poll window, so a slow hook cannot pass by being late.
g checkout -q -b x
: >"$MARK"
commit b
rc=0
if called; then rc=1; fi
result T2 "$rc"
g checkout -q develop

# T3
mv "$STUB" "$TMP/stub"
: >"$MARK"
rc=0
commit c || rc=1
result T3 "$rc"
mv "$TMP/stub" "$STUB"

# T4
: >"$MARK"
g merge -q --no-ff x -m merge
rc=0
called || rc=1
result T4 "$rc"

# Up to 5 seconds for the backgrounded export to stamp HASH.
stamped() { # stamped HASH
	local i
	for ((i = 0; i < 50; i++)); do
		if [[ "$(cat "$VOR_STAMP" 2>/dev/null)" == "$1" ]]; then
			return 0
		fi
		sleep 0.1
	done
	return 1
}

# The hook runs the exporter of the repository it is in. The copy is not
# tracked, like bin/.
mkdir scripts
cp "$EXPORTER" scripts/vor-export-source.sh

# T1-T4 ran with a destination that is not an export: nothing was written.
rc=0
[[ ! -e "$TMP/vor" ]] || rc=1
result "T1-T4 exported nothing" "$rc"

# T5: make the destination an earlier export, then commit on develop.
mkdir -p "$TMP/vor"
bash scripts/vor-export-source.sh "$VOR_EXPORT_DEST" develop >/dev/null
first="$(cat "$VOR_STAMP")"
commit d
rc=0
stamped "$(g rev-parse develop)" || rc=1
[[ "$(cat "$VOR_STAMP")" != "$first" ]] || rc=1
[[ -f "$VOR_EXPORT_DEST/d" ]] || rc=1
[[ "$(cat "$VOR_EXPORT_DEST/.vor-export" 2>/dev/null)" == "$(g rev-parse develop)" ]] || rc=1
result T5 "$rc"

# T6: on another branch nothing is exported. The full poll window, so a slow
# hook cannot pass by being late.
before="$(cat "$VOR_STAMP")"
g checkout -q x
commit e
rc=0
if stamped "$(g rev-parse HEAD)"; then rc=1; fi
[[ "$(cat "$VOR_STAMP")" == "$before" ]] || rc=1
[[ ! -e "$VOR_EXPORT_DEST/e" ]] || rc=1
result T6 "$rc"
g checkout -q develop

# T7: a destination that no export made is left alone, and the commit
# succeeds.
export VOR_EXPORT_DEST="$TMP/other/unheaded"
mkdir -p "$VOR_EXPORT_DEST"
echo keep >"$VOR_EXPORT_DEST/mine.txt"
rc=0
commit f || rc=1
sleep 1
[[ "$(cat "$VOR_EXPORT_DEST/mine.txt")" == keep ]] || rc=1
[[ ! -e "$VOR_EXPORT_DEST/.vor-export" ]] || rc=1
[[ ! -e "$VOR_EXPORT_DEST.exported" ]] || rc=1
[[ "$(find "$TMP/other" -mindepth 1 | sort | tr '\n' ' ')" == "$VOR_EXPORT_DEST $VOR_EXPORT_DEST/mine.txt " ]] || rc=1
# No export still running when the temp directory is removed.
[[ "$(cat "$VOR_STAMP")" == "$before" ]] || rc=1
result T7 "$rc"

exit $((fails > 0))
