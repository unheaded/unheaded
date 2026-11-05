#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-gitleaks.sh — gitleaks over every tracked file (GATING).
#
# The CI step used to curl gitleaks 8.28.0 and run it with no checksum, so a
# swapped release asset would have run with the repo checked out. Now the
# version and sha256 are pinned in lib/tool-pins.sh and checked on every run
# (lib/fetch-tool.sh), and scripts/check-tool-freshness.sh fails once a newer
# release has been out 7 days.
#
# Scans the tracked files only, in a temporary copy: that is what a CI
# checkout holds, and on a host it keeps untracked files such as the repo
# .env out of the verdict (they are never committed; this gate is about what
# is). .gitleaksignore (tracked, shrink-only via check-secrets-baseline.sh)
# applies as usual. Rules: gitleaks' defaults plus ours in .gitleaks.toml
# (unheaded-api-key: uhk_ keys, ADR-103 §5).
#
#   scripts/check-gitleaks.sh             tracked files, GATING
#   scripts/check-gitleaks.sh --history   every commit, findings report only
#
# --history reports rather than fails on what it finds: the findings in the
# history are the accepted set in docs/security/PRE-PUBLIC-BLOCKERS.md. Only
# findings get that treatment. A tool that cannot be fetched, a scan that
# errors or a shallow clone exits 2, so a scan that did not run cannot read as
# one that found nothing.
set -euo pipefail

MODE="${1:-}"
case "$MODE" in
"" | --history) ;;
*)
	echo "usage: $0 [--history]" >&2
	exit 2
	;;
esac

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
# shellcheck source=scripts/lib/tool-pins.sh
source scripts/lib/tool-pins.sh
# shellcheck source=scripts/lib/fetch-tool.sh
source scripts/lib/fetch-tool.sh
BIN="$(fetch_tool gitleaks "$GITLEAKS_VERSION" \
	"https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz" \
	"$GITLEAKS_SHA256" gitleaks)"

if [[ "$MODE" == --history ]]; then
	[[ "$(git rev-parse --is-shallow-repository)" == false ]] || {
		echo "[FAIL] shallow clone: there is no history to scan (fetch-depth: 0)" >&2
		exit 2
	}
	# Its own exit code for findings: gitleaks also exits 1 on an error.
	LEAKS_RC=3
	rc=0
	"$BIN" git --no-banner --redact --exit-code "$LEAKS_RC" --config .gitleaks.toml . || rc=$?
	case "$rc" in
	0) echo "[PASS] gitleaks ${GITLEAKS_VERSION}: no findings in $(git rev-list --count HEAD) commits" ;;
	"$LEAKS_RC") echo "::warning title=gitleaks history::findings in git history outside .gitleaksignore; see the count above and docs/security/PRE-PUBLIC-BLOCKERS.md" ;;
	*)
		echo "[FAIL] gitleaks could not scan the history (exit ${rc})" >&2
		exit 2
		;;
	esac
	exit 0
fi

TREE="$(mktemp -d)"
trap 'rm -rf "$TREE"' EXIT
git ls-files -z | tar --null -T - -cf - | tar -xf - -C "$TREE"
n="$(git ls-files | wc -l)"
[[ "$n" -gt 0 ]] || { echo "[FAIL] no tracked files; refusing to pass vacuously" >&2; exit 2; }
cd "$TREE"
"$BIN" dir --no-banner --redact --exit-code 1 --config "$REPO_ROOT/.gitleaks.toml" .
echo "[PASS] gitleaks ${GITLEAKS_VERSION}: no new secrets in ${n} tracked files"
