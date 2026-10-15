#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# East's side of the offsite copies (ERDA 1.2): every set under
# $WELL_OFFSITE_DIR/sets must verify against its SHA256SUMS, the newest must
# be younger than $WELL_OFFSITE_MAX_AGE_H hours, and sets older than
# $WELL_OFFSITE_KEEP_DAYS days are removed (the sender cannot delete).
# Exit 1 on any bad or missing set, so the unit fails visibly.
#
# Run weekly by unheaded-well-offsite-check.service as well-backup-copy.
set -euo pipefail
ROOT="${WELL_OFFSITE_DIR:-/var/backups/unheaded-well-west}/sets"
KEEP_DAYS="${WELL_OFFSITE_KEEP_DAYS:-30}"
MAX_AGE_H="${WELL_OFFSITE_MAX_AGE_H:-36}"

find "$ROOT" -mindepth 1 -maxdepth 1 -type d -regextype posix-extended \
	-regex '.*/[0-9]{4}-[0-9]{2}-[0-9]{2}' -mtime +"$KEEP_DAYS" -print -exec rm -rf -- {} + |
	sed 's/^/[offsite-check] pruned /'

bad=0 n=0
while IFS= read -r -d '' sums; do
	set_dir="$(dirname "$sums")"
	n=$((n + 1))
	if ! (cd "$set_dir" && sha256sum --quiet --strict -c SHA256SUMS); then
		echo "[offsite-check] BAD ${set_dir#"$ROOT"/}" >&2
		bad=$((bad + 1))
	fi
done < <(find "$ROOT" -mindepth 3 -maxdepth 3 -name SHA256SUMS -print0)
((n > 0)) || { echo "[offsite-check] no sets in $ROOT" >&2; exit 1; }

latest="$(cat "$ROOT/LATEST" 2>/dev/null || true)"
[[ -f "$ROOT/$latest/SHA256SUMS" ]] || { echo "[offsite-check] LATEST ($latest) is not here" >&2; exit 1; }
age_h=$((($(date +%s) - $(stat -c %Y "$ROOT/$latest/SHA256SUMS")) / 3600))
((age_h <= MAX_AGE_H)) || { echo "[offsite-check] newest set $latest is ${age_h}h old (> ${MAX_AGE_H}h)" >&2; bad=$((bad + 1)); }
((bad == 0)) || exit 1
echo "[offsite-check] $n set(s) verified; newest $latest (${age_h}h)"
