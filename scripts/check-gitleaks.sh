#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-gitleaks.sh — gitleaks over every tracked file (GATING).
#
# The CI step used to curl gitleaks 8.28.0 and run it with no checksum, so a
# swapped release asset would have run with the repo checked out. Now the
# version and sha256 are pinned here and checked on every run (the same as
# oxlint in check-js-lint.sh), and scripts/check-tool-freshness.sh fails once
# a newer release has been out 7 days.
#
# Scans the tracked files only, in a temporary copy: that is what a CI
# checkout holds, and on a host it keeps untracked files such as the repo
# .env out of the verdict (they are never committed; this gate is about what
# is). .gitleaksignore (tracked, shrink-only via check-secrets-baseline.sh)
# applies as usual.
#
#   scripts/check-gitleaks.sh
set -euo pipefail

GITLEAKS_VERSION="8.30.1"
GITLEAKS_SHA256="551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb" # gitleaks_<v>_linux_x64.tar.gz

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
[[ "$(uname -sm)" == "Linux x86_64" ]] || { echo "[FAIL] pinned for Linux x86_64 only, not $(uname -sm)" >&2; exit 2; }

CACHE="${GITLEAKS_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/unheaded-tools/gitleaks-${GITLEAKS_VERSION}}"
TARBALL="$CACHE/gitleaks.tar.gz"
mkdir -p "$CACHE"
if [[ ! -f "$TARBALL" ]]; then
	curl -fsSL --retry 3 -o "$TARBALL.part" \
		"https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz"
	mv "$TARBALL.part" "$TARBALL"
fi
# Every run, not only after a download: a swapped cache file must not run.
echo "${GITLEAKS_SHA256}  ${TARBALL}" | sha256sum -c --quiet - || {
	echo "[FAIL] gitleaks ${GITLEAKS_VERSION} tarball does not match its pinned sha256; deleted" >&2
	rm -f "$TARBALL"
	exit 2
}
tar -xzf "$TARBALL" -C "$CACHE" gitleaks

TREE="$(mktemp -d)"
trap 'rm -rf "$TREE"' EXIT
git ls-files -z | tar --null -T - -cf - | tar -xf - -C "$TREE"
n="$(git ls-files | wc -l)"
[[ "$n" -gt 0 ]] || { echo "[FAIL] no tracked files; refusing to pass vacuously" >&2; exit 2; }
cd "$TREE"
"$CACHE/gitleaks" dir --no-banner --redact --exit-code 1 .
echo "[PASS] gitleaks ${GITLEAKS_VERSION}: no new secrets in ${n} tracked files"
