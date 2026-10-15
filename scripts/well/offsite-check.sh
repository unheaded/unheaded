#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# East's side of the offsite copies (ERDA 1.2): every set under
# $WELL_OFFSITE_DIR/sets must verify against its SHA256SUMS, the newest must
# be younger than $WELL_OFFSITE_MAX_AGE_H hours, and all but the newest
# $WELL_OFFSITE_KEEP sets are removed (the sender cannot delete). By count,
# not age: west is not up every night, and an age cut would empty east
# after a long enough power-off.
# Exit 1 on any bad or missing set, so the unit fails visibly.
#
# Run weekly by unheaded-well-offsite-check.service as well-backup-copy.
set -euo pipefail
ROOT="${WELL_OFFSITE_DIR:-/var/backups/unheaded-well-west}/sets"
KEEP="${WELL_OFFSITE_KEEP:-14}"
[[ "$KEEP" =~ ^[1-9][0-9]*$ ]] || { echo "[offsite-check] WELL_OFFSITE_KEEP must be a positive integer" >&2; exit 2; }
MAX_AGE_H="${WELL_OFFSITE_MAX_AGE_H:-36}"

mapfile -t SETS < <(find "$ROOT" -mindepth 2 -maxdepth 2 -type d -name 'well-[0-9][0-9][0-9][0-9][0-9][0-9]Z' \
	-path "$ROOT/[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]/*" | sort -r)
for old in "${SETS[@]:KEEP}"; do
	rm -rf -- "$old"
	rmdir --ignore-fail-on-non-empty -- "$(dirname "$old")"
	echo "[offsite-check] pruned ${old#"$ROOT"/}"
done

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
