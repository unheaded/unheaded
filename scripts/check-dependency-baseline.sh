#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-dependency-baseline.sh — the set of third-party packages the build
# pulls in may only SHRINK (ADR-096 §3, ADR-095 track two).
#
# ADR-096: no dependency gets in because an assistant suggested it, and a new
# module in go.sum or Cargo.lock without a recorded review fails the build.
# ADR-095 measured the tree against ADR-004's register and found it does not
# match (113 Go modules, 411 crates), so the register cannot be the gate yet.
# This holds the line meanwhile: what is in now is listed in
# docs/policy/dependency-baseline.txt, removals pass and lower the ceiling,
# and an addition fails until a person has done ADR-096 §2 for it and added
# its line in the same commit, where review sees it.
#
# One line per package identity, "<ecosystem> <name>":
#   go     every module path in a tracked go.sum (the full graph)
#   cargo  every registry or git package in a tracked Cargo.lock (no path/workspace crates)
#   pypi   every requirement in a tracked requirements*.txt (name normalised per PEP 503)
#   npm    every dependency and devDependency in a tracked package.json
#
# Identity, not version: a bump is not caught here (ADR-096 asks for review of
# those too; that is the register's job once it exists). Compares the SET, not
# a count (scripts/check-secrets-baseline.sh explains why).
#
#   scripts/check-dependency-baseline.sh            # check
#   scripts/check-dependency-baseline.sh --list     # print the current set
#   scripts/check-dependency-baseline.sh --update   # rewrite the baseline; refuses additions
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="${DEPENDENCY_BASELINE:-${REPO_ROOT}/docs/policy/dependency-baseline.txt}"
# shellcheck source=scripts/lib/sets.sh
source "${REPO_ROOT}/scripts/lib/sets.sh"
cd "$REPO_ROOT" || exit 2

current_set() {
	local f
	git ls-files -z '*go.sum' | while IFS= read -r -d '' f; do
		awk 'NF >= 3 { print "go " $1 }' "$f"
	done
	git ls-files -z '*Cargo.lock' | while IFS= read -r -d '' f; do
		awk '
			/^\[\[package\]\]/ { if (name != "" && src != "") print "cargo " name; name = ""; src = "" }
			/^name = /   { name = $3; gsub(/"/, "", name) }
			/^source = / { src = $3 }
			END          { if (name != "" && src != "") print "cargo " name }
		' "$f"
	done
	git ls-files -z '*requirements*.txt' | while IFS= read -r -d '' f; do
		# Drop comments, options (-r, -e, --hash...), env markers and extras;
		# keep the name; PEP 503: lowercase, runs of -_. become one -.
		sed -e 's/#.*//' -e 's/;.*//' "$f" \
			| awk '!/^[[:space:]]*-/ && NF { print $1 }' \
			| sed -E 's/[[<>=!~ ].*//; s/\[.*//' \
			| tr '[:upper:]' '[:lower:]' | sed -E 's/[-_.]+/-/g; s/^/pypi /'
	done
	git ls-files -z '*package.json' | while IFS= read -r -d '' f; do
		python3 -I -c '
import json, sys
d = json.load(open(sys.argv[1]))
for k in ("dependencies", "devDependencies", "optionalDependencies", "peerDependencies"):
    for name in d.get(k, {}):
        print("npm " + name)
' "$f"
	done
}

current="$(current_set | sort -u)"
if [[ -z "$current" ]]; then
	echo "[FAIL] found no dependencies at all; the parsers are broken, refusing to pass vacuously" >&2
	exit 2
fi

if [[ "${1:-}" == "--list" ]]; then
	printf '%s\n' "$current"
	exit 0
fi

baseline=""
if [[ -f "$BASELINE" ]]; then
	baseline="$(grep -v '^#' "$BASELINE" | grep -v '^[[:space:]]*$' | sort -u)"
fi
added="$(set_comm -23 "$current" "$baseline")"
removed="$(set_comm -13 "$current" "$baseline")"

if [[ "${1:-}" == "--update" ]]; then
	if [[ -f "$BASELINE" && -n "$added" ]]; then
		echo "[REFUSED] --update only shrinks the baseline. New package(s):" >&2
		printf '%s\n' "$added" | sed 's/^/         /' >&2
		echo "       Review each (ADR-096 §2), then add its line to ${BASELINE#"$REPO_ROOT"/} by hand." >&2
		exit 1
	fi
	{
		echo "# Third-party packages the build pulls in (ADR-096 §3). This set may only SHRINK."
		echo "# A new line is a reviewed admission: a person named it, checked the registry, read"
		echo "# what ADR-096 §2 says to read. Add it by hand in the commit that adds the dependency."
		echo "# Remove lines with: scripts/check-dependency-baseline.sh --update"
		printf '%s\n' "$current"
	} >"$BASELINE"
	echo "[UPDATED] ${BASELINE#"$REPO_ROOT"/} ($(printf '%s\n' "$current" | grep -c .) packages)"
	exit 0
fi

if [[ ! -f "$BASELINE" ]]; then
	echo "[FAIL] ${BASELINE#"$REPO_ROOT"/} is missing" >&2
	exit 1
fi

if [[ -n "$added" ]]; then
	echo "[FAIL] dependency not in the baseline (ADR-096):" >&2
	printf '%s\n' "$added" | sed 's/^/         /' >&2
	echo >&2
	echo "       First ask whether the standard library or pkg/ already does it." >&2
	echo "       If not: a person names it, confirms it on the registry (owner, history)," >&2
	echo "       reads what ADR-096 §2 says to read, then adds the line above to" >&2
	echo "       ${BASELINE#"$REPO_ROOT"/} in the same commit. Not because a tool suggested it." >&2
	exit 1
fi

n="$(printf '%s\n' "$current" | grep -c .)"
if [[ -n "$removed" ]]; then
	echo "[FAIL] gone from the build but still in the baseline; lower the ceiling with $0 --update:" >&2
	printf '%s\n' "$removed" | sed 's/^/         /' >&2
	exit 1
fi
echo "[PASS] ${n} third-party packages, all in the baseline"
