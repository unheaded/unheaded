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
#   scripts/check-gitleaks.sh                 tracked files, GATING
#   scripts/check-gitleaks.sh --history       every commit, GATING on new findings
#   scripts/check-gitleaks.sh --history-rows  the baseline row of each finding
#
# --history warns on a finding that has a row in
# docs/policy/gitleaks-history-baseline.txt and fails on one that does not, so
# a new leak in history is told apart from the ones already known. A row means
# known, not accepted: docs/security/PRE-PUBLIC-BLOCKERS.md says what those
# findings still need, and what the 2026-07-29 decision covers and until when.
# A tool that cannot be fetched, a scan that errors or a shallow clone exits 2,
# so a scan that did not run cannot read as one that found nothing.
#
# A row is a sha256 of author date, file, rule and line. Not gitleaks' own
# fingerprint: that holds the commit id, which scripts/resign-unpushed.py
# changes. Hashed so the file is not a list of where to look. The script can
# demand nothing of a new row: adding one is a review's call, and a row that
# no scan reports is left alone, since what a scan sees depends on the refs
# fetched.
set -euo pipefail

MODE="${1:-}"
case "$MODE" in
"" | --history | --history-rows) ;;
*)
	echo "usage: $0 [--history | --history-rows]" >&2
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

if [[ -n "$MODE" ]]; then
	BASELINE="docs/policy/gitleaks-history-baseline.txt"
	[[ "$(git rev-parse --is-shallow-repository)" == false ]] || {
		echo "[FAIL] shallow clone: there is no history to scan (fetch-depth: 0)" >&2
		exit 2
	}
	command -v jq >/dev/null || { echo "[FAIL] jq not found; this gate does not pass by skipping" >&2; exit 2; }
	[[ -f "$BASELINE" ]] || { echo "[FAIL] $BASELINE is missing" >&2; exit 2; }
	REPORT="$(mktemp)"
	trap 'rm -f "$REPORT"' EXIT
	# Its own exit code for findings: gitleaks also exits 1 on an error.
	LEAKS_RC=3
	rc=0
	"$BIN" git --no-banner --redact --exit-code "$LEAKS_RC" --config .gitleaks.toml \
		--report-format json --report-path "$REPORT" . || rc=$?
	case "$rc" in
	0)
		[[ "$MODE" == --history-rows ]] || echo "[PASS] gitleaks ${GITLEAKS_VERSION}: no findings in $(git rev-list --count HEAD) commits"
		exit 0
		;;
	"$LEAKS_RC") ;;
	*)
		echo "[FAIL] gitleaks could not scan the history (exit ${rc})" >&2
		exit 2
		;;
	esac

	known=0
	new=0
	while IFS=$'\t' read -r key where; do
		row="$(printf '%s' "$key" | sha256sum | cut -d' ' -f1)"
		if [[ "$MODE" == --history-rows ]]; then
			printf '%s\n' "$row"
			new=$((new + 1))
		elif grep -qxF "$row" "$BASELINE"; then
			known=$((known + 1))
		else
			echo "[FAIL] in history, no row in ${BASELINE}: ${where}"
			new=$((new + 1))
		fi
	done < <(jq -r '.[] | ["\(.Date)|\(.File)|\(.RuleID)|\(.StartLine)", "\(.Commit[0:8]) \(.File):\(.StartLine) \(.RuleID)"] | @tsv' "$REPORT")
	((known + new > 0)) || { echo "[FAIL] gitleaks reported findings and none could be read" >&2; exit 2; }
	[[ "$MODE" == --history ]] || exit 0
	if ((new > 0)); then
		echo "Take the value out and rotate it if it ever granted access; a row only records that a person looked."
		exit 1
	fi
	echo "::warning title=gitleaks history::${known} known findings in git history outside .gitleaksignore, none new; see docs/security/PRE-PUBLIC-BLOCKERS.md"
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
