#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-image-digests.sh: third-party images in docker-compose.yml and
# monitoring/docker-compose.yml name their bytes, tag@sha256:<64 hex>. A tag is
# a label the publisher can move; a digest cannot be moved under the same name.
#
# Images this repo builds (unheaded*, ${...}) are out of scope. The rest, where
# the tag has moved on from what west holds, are in
# docs/policy/image-digest-baseline.txt as "<file> <image>". That set may only
# shrink: a listed image that now has a digest fails too.
#
# Exact version tags themselves are check-image-tags.sh (ADR-102).
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="${REPO_ROOT}/docs/policy/image-digest-baseline.txt"
cd "$REPO_ROOT" || exit 2
# shellcheck source=scripts/lib/sets.sh
source "${REPO_ROOT}/scripts/lib/sets.sh"
[[ -f "$BASELINE" ]] || { echo "[FAIL] ${BASELINE#"$REPO_ROOT"/} is missing" >&2; exit 2; }

files=(docker-compose.yml monitoring/docker-compose.yml)
refs="$(for f in "${files[@]}"; do
	sed -nE 's/^[[:space:]-]*image:[[:space:]]*["'"'"']?([^"'"'"'[:space:]#]+).*/\1/p' "$f" | sed "s|^|$f |"
done | LC_ALL=C sort -u)"
[[ -n "$refs" ]] || { echo "check-image-digests: no images found; refusing to pass vacuously" >&2; exit 2; }

pinned=0
current=""
while read -r f img; do
	# shellcheck disable=SC2016 # a literal ${ marks an image the compose file interpolates
	case "$img" in
	*'${'* | unheaded-* | unheaded/*) continue ;;
	esac
	if [[ "$img" =~ @sha256:[0-9a-f]{64}$ ]]; then
		pinned=$((pinned + 1))
	else
		current+="$f $img"$'\n'
	fi
done <<<"$refs"
current="$(printf '%s' "$current" | LC_ALL=C sort -u)"
allowed="$(grep -vE '^(#|$)' "$BASELINE" | LC_ALL=C sort -u)"

added="$(set_comm -23 "$current" "$allowed")"
stale="$(set_comm -13 "$current" "$allowed")"
rc=0
if [[ -n "$added" ]]; then
	echo "[FAIL] image without a digest; read it with: docker buildx imagetools inspect <image>" >&2
	printf '%s\n' "$added" | sed 's/^/         /' >&2
	rc=1
fi
if [[ -n "$stale" ]]; then
	echo "[FAIL] ${BASELINE#"$REPO_ROOT"/} lists images that are pinned or gone; remove them:" >&2
	printf '%s\n' "$stale" | sed 's/^/         /' >&2
	rc=1
fi
((rc == 0)) || exit "$rc"
echo "[PASS] $pinned images digest-pinned, $(printf '%s\n' "$allowed" | grep -c .) baselined"
