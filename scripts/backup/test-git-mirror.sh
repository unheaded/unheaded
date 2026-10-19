#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# Exercises scripts/backup/git-mirror.sh against throwaway repositories:
# init, push, check (green and red), a rewritten branch keeps its old tip on
# the mirror, a deleted branch stays on the mirror, an uninitialised
# destination is refused, and GIT_MIRROR_DEST unset fails loudly.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
MIRROR="$HERE/git-mirror.sh"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
export GIT_MIRROR_REPO="$T/src" GIT_MIRROR_DEST="$T/east/unheaded.git" XDG_RUNTIME_DIR="$T"
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.invalid GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.invalid

fail() { echo "[FAIL] $*" >&2; exit 1; }
pass() { echo "[ok] $*"; }

git init --quiet -b develop "$T/src"
git -C "$T/src" commit --quiet --allow-empty -m one
git -C "$T/src" branch feature

( unset GIT_MIRROR_DEST; "$MIRROR" push ) >/dev/null 2>&1 && fail "ran with GIT_MIRROR_DEST unset"
pass "GIT_MIRROR_DEST unset refused"

git init --quiet --bare "$GIT_MIRROR_DEST.raw"
GIT_MIRROR_DEST="$GIT_MIRROR_DEST.raw" "$MIRROR" push >/dev/null 2>&1 && fail "pushed to a destination nobody initialised"
pass "uninitialised destination refused"

mkdir -p "$T/east"
"$MIRROR" init >/dev/null
"$MIRROR" push >/dev/null
"$MIRROR" check >/dev/null || fail "check red right after push"
pass "init + push + check"

git -C "$T/src" commit --quiet --allow-empty -m two
"$MIRROR" check >/dev/null 2>&1 && fail "check green with an unmirrored commit"
pass "check catches an unmirrored commit"

old="$(git -C "$T/src" rev-parse develop)"
"$MIRROR" push >/dev/null
git -C "$T/src" commit --quiet --amend --allow-empty -m two-resigned
"$MIRROR" push >/dev/null
"$MIRROR" check >/dev/null || fail "check red after pushing a rewritten branch"
git -C "$GIT_MIRROR_DEST" reflog show refs/heads/develop | grep -q "${old:0:7}" || fail "rewritten tip ${old:0:7} lost from the mirror's reflog"
git -C "$GIT_MIRROR_DEST" cat-file -e "$old^{commit}" || fail "rewritten commit object gone"
pass "rewritten branch: old tip kept in the mirror's reflog"

git -C "$T/src" branch -D --quiet feature
"$MIRROR" push >/dev/null
git -C "$GIT_MIRROR_DEST" rev-parse --verify --quiet refs/heads/feature >/dev/null || fail "deleted branch pruned from the mirror"
"$MIRROR" check >/dev/null || fail "check red because the mirror has an extra ref"
pass "deleted branch kept on the mirror"

git -C "$GIT_MIRROR_DEST" config gc.reflogExpire 90.days
"$MIRROR" push >/dev/null 2>&1 && fail "pushed to a mirror whose reflogs expire"
pass "mirror with expiring reflogs refused"

echo "[PASS] git-mirror: 7 checks"
