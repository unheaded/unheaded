#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# Copies every ref of the working repository to a bare repository on east
# (FND-005): commits that are not on GitHub yet exist on west's disk only.
#
# It is a push of all refs, not `git push --mirror`: a ref deleted or
# rewritten on west stays recoverable on east. Nothing is pruned, the bare
# repo refuses deletions (receive.denyDeletes), and its reflogs never expire,
# so the tip a force-push replaced (a re-signed branch) is still there.
# Working-tree changes that are not committed are not covered.
#
#   GIT_MIRROR_DEST=govan@east:backups/unheaded.git scripts/backup/git-mirror.sh init
#   GIT_MIRROR_DEST=... scripts/backup/git-mirror.sh push    # what the timer runs
#   GIT_MIRROR_DEST=... scripts/backup/git-mirror.sh check   # exit 1 if east lags
#
# GIT_MIRROR_REPO defaults to the repository this script is in. With
# GIT_MIRROR_METRICS set to a directory, push writes
# unheaded_git_mirror_last_success_timestamp_seconds there (Huginn textfile).
set -euo pipefail

REPO="${GIT_MIRROR_REPO:-$(cd "$(dirname "$0")/../.." && pwd)}"
DEST="${GIT_MIRROR_DEST:?set GIT_MIRROR_DEST (user@host:path/repo.git, or a local path)}"
METRICS="${GIT_MIRROR_METRICS:-}"
export GIT_SSH_COMMAND="${GIT_SSH_COMMAND:-ssh -o BatchMode=yes -o ConnectTimeout=15}"

git -C "$REPO" rev-parse --git-dir >/dev/null 2>&1 || { echo "[git-mirror] not a git repository: $REPO" >&2; exit 2; }

# The bare repository's settings, applied by init. Kept here so check can
# confirm east was not set up some other way.
BARE_SETTINGS=(
	"core.logAllRefUpdates always"
	"gc.reflogExpire never"
	"gc.reflogExpireUnreachable never"
	"receive.denyDeletes true"
	"receive.denyNonFastForwards false"
	"receive.fsckObjects true"
)

# Run a command in the destination: over ssh for host:path, locally otherwise.
on_dest() {
	if [[ "$DEST" == *:* && "$DEST" != /* ]]; then
		local host="${DEST%%:*}" path="${DEST#*:}"
		# shellcheck disable=SC2029 # path and args are expanded here on purpose
		ssh -o BatchMode=yes -o ConnectTimeout=15 "$host" "cd $(printf '%q' "$path") && $(printf '%q ' "$@")"
	else
		(cd "$DEST" && "$@")
	fi
}

cmd_init() {
	if [[ "$DEST" == *:* && "$DEST" != /* ]]; then
		local host="${DEST%%:*}" path="${DEST#*:}"
		ssh -o BatchMode=yes -o ConnectTimeout=15 "$host" "git init --quiet --bare $(printf '%q' "$path")"
	else
		git init --quiet --bare "$DEST"
	fi
	local kv
	for kv in "${BARE_SETTINGS[@]}"; do
		# shellcheck disable=SC2086 # "key value" splits into two arguments
		on_dest git config $kv
	done
	echo "[git-mirror] bare repository ready: $DEST"
}

cmd_push() {
	local lock="${XDG_RUNTIME_DIR:-/tmp}/unheaded-git-mirror.lock"
	exec 9>"$lock"
	flock -n 9 || { echo "[git-mirror] another run holds $lock; skipping"; exit 0; }
	# Fail on a destination nobody initialised, rather than pushing into a
	# repository whose reflogs expire.
	local kv
	for kv in "${BARE_SETTINGS[@]}"; do
		local key="${kv%% *}" want="${kv#* }" got
		got="$(on_dest git config --get "$key" || true)"
		[[ "$got" == "$want" ]] || { echo "[git-mirror] $DEST has $key='$got', want '$want': run init" >&2; exit 1; }
	done
	git -C "$REPO" push --quiet --force "$DEST" '+refs/*:refs/*'
	local now
	now="$(date +%s)"
	echo "[git-mirror] $(git -C "$REPO" for-each-ref | wc -l) refs -> $DEST"
	if [[ -n "$METRICS" ]]; then
		printf '# HELP unheaded_git_mirror_last_success_timestamp_seconds Last push of every repository ref to east (FND-005).\n# TYPE unheaded_git_mirror_last_success_timestamp_seconds gauge\nunheaded_git_mirror_last_success_timestamp_seconds %s\n' "$now" >"$METRICS/.git_mirror.prom.tmp"
		chmod 0644 "$METRICS/.git_mirror.prom.tmp"
		mv -f "$METRICS/.git_mirror.prom.tmp" "$METRICS/git_mirror.prom"
	fi
}

# Every local ref must be on east at the same commit. Refs only east has
# (deleted or renamed here) are fine: that is the point.
cmd_check() {
	local remote missing=0 name sha
	remote="$(git -C "$REPO" ls-remote "$DEST")"
	while read -r sha name; do
		if ! grep -qxF "$sha	$name" <<<"$remote"; then
			echo "[git-mirror] behind: $name ($(git -C "$REPO" log -1 --format='%cr' "$sha"))"
			missing=$((missing + 1))
		fi
	done < <(git -C "$REPO" for-each-ref --format='%(objectname) %(refname)')
	if [[ $missing -gt 0 ]]; then
		echo "[git-mirror] FAIL: $missing ref(s) not on $DEST at west's commit" >&2
		return 1
	fi
	echo "[git-mirror] PASS: every ref is on $DEST"
}

case "${1:-push}" in
init) cmd_init ;;
push) cmd_push ;;
check) cmd_check ;;
*) echo "usage: $0 [init|push|check]" >&2; exit 2 ;;
esac
