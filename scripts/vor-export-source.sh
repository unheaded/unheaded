#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Export the tracked files of one commit for vor (DP-712). A hardened unit
# cannot read /home, and a working tree holds .env; git archive carries
# tracked files only.
#   scripts/vor-export-source.sh <destination directory> [ref]
#
# ref is HEAD unless given; the post-commit hook passes "develop". The commit
# is resolved once, and its full hash is written twice:
#   <destination>/.vor-export   inside the export, which also marks the
#                               directory as one this script may replace
#   <destination>.exported      beside it, written last and by rename. This is
#                               the stamp unheaded-vor-refresh.path watches: it
#                               changes only when a whole new export is in
#                               place, and a reader sees the old hash or the
#                               new one, never part of one.
set -euo pipefail
dest="${1:?usage: vor-export-source.sh <destination directory> [ref]}"
ref="${2:-HEAD}"
[[ "$dest" == /*/unheaded ]] || { echo "vor-export-source: destination must be an absolute path ending in /unheaded" >&2; exit 2; }
if [[ -e "$dest" && ! -e "$dest/.vor-export" ]]; then
	echo "vor-export-source: $dest is not an earlier export; refusing to replace it" >&2
	exit 2
fi
repo="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
parent="$(dirname "$dest")"
[[ -d "$parent" ]] || { echo "vor-export-source: no such directory: $parent" >&2; exit 2; }
# One export at a time into this directory: two commits in a row start two
# hooks, and the second must not swap while the first is between its rm and
# its mv. The lock is on the directory itself, so it leaves no file. The ref
# is resolved under the lock, so the export that runs last is of the newest
# commit.
exec 9<"$parent"
flock 9
# Before anything is written: an unknown ref leaves the old export alone.
commit="$(git -C "$repo" rev-parse --verify --quiet --end-of-options "${ref}^{commit}")" || {
	echo "vor-export-source: no such commit: $ref" >&2
	exit 1
}
tmp="$(mktemp -d "$parent/.vor-export.XXXXXX")"
stamp_tmp=""
trap 'rm -rf -- "$tmp" ${stamp_tmp:+"$stamp_tmp"}' EXIT
git -C "$repo" archive --format=tar "$commit" | tar -x -C "$tmp"
printf '%s\n' "$commit" >"$tmp/.vor-export"
chmod -R a+rX "$tmp"
rm -rf -- "$dest"
mv -T -- "$tmp" "$dest"
stamp_tmp="$(mktemp "$parent/.vor-exported.XXXXXX")"
printf '%s\n' "$commit" >"$stamp_tmp"
chmod 0644 "$stamp_tmp"
mv -f -- "$stamp_tmp" "$dest.exported"
trap - EXIT
echo "exported $(git -C "$repo" rev-parse --short "$commit") to $dest"
