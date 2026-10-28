#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-python-audit.sh — known vulnerabilities in pinned Python
# requirements may only SHRINK (ADR-096 §3: OSV for Python).
#
# Go has govulncheck and Rust has cargo audit in security.yml; Python had
# nothing. pip-audit checks every fully pinned requirements*.txt (every
# requirement is name==version) against the PyPI/OSV advisory data.
# Files with unpinned (>=) requirements are not audited: what they install
# changes by the day. ADR-095 lists them for pinning.
#
# A finding that is not in docs/policy/python-audit-accepted.txt fails. An
# accepted line that no longer matches a finding fails too, so the list
# shrinks as fixes land. Each accepted line carries its reason.
#
#   PIP_AUDIT=pip-audit scripts/check-python-audit.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ACCEPTED="${REPO_ROOT}/docs/policy/python-audit-accepted.txt"
PIP_AUDIT="${PIP_AUDIT:-pip-audit}"
cd "$REPO_ROOT" || exit 2
# shellcheck source=scripts/lib/sets.sh
source "${REPO_ROOT}/scripts/lib/sets.sh"

command -v "$PIP_AUDIT" >/dev/null || { echo "[FAIL] $PIP_AUDIT not found; this gate does not pass by skipping" >&2; exit 2; }
[[ -f "$ACCEPTED" ]] || { echo "[FAIL] ${ACCEPTED#"$REPO_ROOT"/} is missing" >&2; exit 1; }

files=()
while IFS= read -r f; do
	# Fully pinned: every requirement line is name==version.
	if ! grep -vE '^[[:space:]]*(#|$|-)' "$f" | grep -qvE '^[A-Za-z0-9_.-]+(\[[^]]*\])?==[^=[:space:]]+'; then
		files+=("$f")
	fi
done < <(git ls-files '*requirements*.txt' 'sbom/python-pip-freeze.txt')
# sbom/python-pip-freeze.txt is the Zhen venv as installed (pip freeze); it
# is named so GitHub's dependency graph does not read it as a manifest, and
# listed here by name so pip-audit still covers what the venv really runs.
[[ ${#files[@]} -gt 0 ]] || { echo "[FAIL] no fully pinned requirements file found; refusing to pass vacuously" >&2; exit 2; }

found=""
for f in "${files[@]}"; do
	json="$("$PIP_AUDIT" --no-deps --disable-pip --progress-spinner off -f json -r "$f" 2>/dev/null)"
	rc=$?
	# pip-audit exits 1 when it finds something; anything else is an error.
	if [[ $rc -ne 0 && $rc -ne 1 ]] || [[ -z "$json" ]]; then
		echo "[FAIL] pip-audit could not audit $f (exit $rc)" >&2
		exit 2
	fi
	found+="$(python3 -I -c '
import json, sys
for dep in json.load(sys.stdin)["dependencies"]:
    for v in dep.get("vulns", []):
        print(dep["name"].lower(), v["id"])
' <<<"$json")"$'\n'
done
current="$(printf '%s' "$found" | grep . | sort -u)"
accepted="$(sed 's/#.*//' "$ACCEPTED" | awk 'NF >= 2 { print tolower($1), $2 }' | sort -u)"

new="$(set_comm -23 "$current" "$accepted")"
gone="$(set_comm -13 "$current" "$accepted")"
rc=0
if [[ -n "$new" ]]; then
	echo "[FAIL] known vulnerabilities in pinned Python requirements (${files[*]}):" >&2
	printf '%s\n' "$new" | sed 's/^/         /' >&2
	echo "       Bump to a fixed version. Accepting one instead means a line in" >&2
	echo "       ${ACCEPTED#"$REPO_ROOT"/} with the reason and a finding that tracks it." >&2
	rc=1
fi
if [[ -n "$gone" ]]; then
	echo "[FAIL] accepted but no longer found; remove from ${ACCEPTED#"$REPO_ROOT"/}:" >&2
	printf '%s\n' "$gone" | sed 's/^/         /' >&2
	rc=1
fi
[[ $rc -eq 0 ]] && echo "[PASS] ${#files[@]} pinned requirements files; $(printf '%s' "$current" | grep -c .) known vulnerabilities, all accepted with a reason"
exit "$rc"
