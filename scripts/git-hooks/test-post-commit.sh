#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# test-post-commit.sh — tests for scripts/git-hooks/post-commit, in a temp repo
# with a stub bin/zhen-codeindex that records its arguments.
#
#   T1: commit on develop           → the stub runs with --repo.
#   T2: commit on another branch    → the stub does not run.
#   T3: stub missing                → the commit still exits 0.
#   T4: git merge --no-ff on develop → the stub runs (post-merge).
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

exit $((fails > 0))
