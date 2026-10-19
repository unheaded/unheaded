#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-image-tags.sh — container images are pinned to exact versions
# (ADR-102); the set of places that still float may only SHRINK.
#
# Looks at every tracked compose file and Kubernetes manifest (`image:`
# lines) and Dockerfile `FROM` lines. Exact means a full version
# (nginx:1.25.5-alpine, v3.3.7, 24.12.6.70) or an Alpine release line
# (alpine:3.24, as ADR-102 pins it), or a PostgreSQL major.minor
# (postgres:16.15-alpine: its versions have two parts). Floating means latest, a missing tag,
# a major or minor only (haproxy:2.9-alpine, busybox:1.36), or a branch
# name (open-webui:main). Images this repo builds (unheaded-*, ${VERSION},
# :dev) and Helm templates are out of scope; so is lxd/, whose ubuntu:24.04
# are LXD image aliases, not OCI tags.
#
# Each entry is "<file> <image>", so the same floating image copied into a
# new file fails too. Remove entries with --update as images are pinned;
# --update refuses additions.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="${REPO_ROOT}/docs/policy/image-tag-baseline.txt"
cd "$REPO_ROOT" || exit 2
# shellcheck source=scripts/lib/sets.sh
source "${REPO_ROOT}/scripts/lib/sets.sh"

refs() {
	git ls-files '*.yml' '*.yaml' ':!lxd/**' ':!helm/**' ':!**/templates/**' | while IFS= read -r f; do
		sed -nE 's/^[[:space:]-]*image:[[:space:]]*["'"'"']?([^"'"'"'[:space:]#]+).*/\1/p' "$f" | sed "s|^|$f |"
	done
	git ls-files '*Dockerfile*' | while IFS= read -r f; do
		# FROM [--platform=..] image [AS name]; skip stage references and scratch.
		awk -v f="$f" '
			toupper($1) == "FROM" {
				i = 2; if ($2 ~ /^--/) i = 3
				img = $i
				if (tolower($(i+1)) == "as") stage[tolower($(i+2))] = 1
				if (img == "scratch" || (tolower(img) in stage) || img ~ /^\$/) next
				print f " " img
			}' "$f"
	done
}

floating() {
	while read -r f img; do
		# shellcheck disable=SC2016 # a literal ${ marks an image tag the compose file interpolates
		case "$img" in
		*'${'* | *'{{'* | unheaded-* | unheaded/* | ghcr.io/stevenrbellis/* | ghcr.io/unheaded/* | *:dev) continue ;;
		esac
		name="${img%@*}"
		[[ "$img" == *@sha256:* ]] && continue # a digest is stricter than a tag
		tag=""
		[[ "${name##*/}" == *:* ]] && tag="${name##*:}"
		# PostgreSQL versions have two parts since 10 (16.15 is a patch release).
		if [[ "$tag" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+ ]] || [[ "$name" =~ (^|/)alpine:3\.[0-9]+$ ]] ||
			[[ "$name" =~ (^|/)postgres:[0-9]{2}\.[0-9]+(-|$) ]]; then
			continue
		fi
		echo "$f $img"
	done < <(refs)
}

current="$(floating | sort -u)"
if [[ "${1:-}" == "--list" ]]; then
	printf '%s\n' "$current"
	exit 0
fi
baseline=""
[[ -f "$BASELINE" ]] && baseline="$(grep -v '^#' "$BASELINE" | grep -v '^[[:space:]]*$' | sort -u)"
added="$(set_comm -23 "$current" "$baseline")"
removed="$(set_comm -13 "$current" "$baseline")"

if [[ "${1:-}" == "--update" ]]; then
	if [[ -f "$BASELINE" && -n "$added" ]]; then
		echo "[REFUSED] --update only shrinks. New floating image(s):" >&2
		printf '%s\n' "$added" | sed 's/^/         /' >&2
		exit 1
	fi
	{
		echo "# Images not yet pinned to an exact version (ADR-102). This set may only SHRINK."
		echo "# <file> <image>. Pin, then remove the line: scripts/check-image-tags.sh --update"
		printf '%s\n' "$current"
	} >"$BASELINE"
	echo "[UPDATED] ${BASELINE#"$REPO_ROOT"/} ($(printf '%s\n' "$current" | grep -c .) entries)"
	exit 0
fi

[[ -f "$BASELINE" ]] || { echo "[FAIL] ${BASELINE#"$REPO_ROOT"/} is missing" >&2; exit 1; }
rc=0
if [[ -n "$added" ]]; then
	echo "[FAIL] image without an exact version (ADR-102):" >&2
	printf '%s\n' "$added" | sed 's/^/         /' >&2
	echo "       Pin the version that runs today (docker image inspect), e.g. nginx:1.25.5-alpine." >&2
	rc=1
fi
if [[ -n "$removed" ]]; then
	echo "[FAIL] pinned or gone, still in the baseline; lower it with $0 --update:" >&2
	printf '%s\n' "$removed" | sed 's/^/         /' >&2
	rc=1
fi
[[ $rc -eq 0 ]] && echo "[PASS] $(printf '%s\n' "$current" | grep -c .) floating image references, all in the baseline"
exit "$rc"
