#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# check-well-lint.sh — The Well's schema lint as a ratchet (ERDA 3.5).
#
# scripts/well/well_lint.py (vendored from the ERDA skill) reads every
# tracked .sql/.go/.py/.conf file for ledger, lock, schema, index, access
# and config smells. Most of today's findings sit in migrations that are
# applied and can never change; the ratchet keeps them from growing:
# per (rule, file), the count may not exceed docs/policy/well-lint-baseline.tsv,
# and a (rule, file) pair not in the baseline fails. Counts, not line
# numbers, so editing a file does not churn the baseline.
#
#   scripts/check-well-lint.sh            # gate
#   scripts/check-well-lint.sh --update   # rewrite the baseline (review the diff:
#                                         # it should only ever shrink)
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="$REPO_ROOT/docs/policy/well-lint-baseline.tsv"
cd "$REPO_ROOT"

# The linter itself is excluded: its self-test fixtures are deliberate findings.
# eval/champion-runbooks/build.py is excluded too: it quotes the Champion rule
# "never run DROP TABLE/DATABASE/..." as prompt text for the eval, which the
# linter reads as DDL in application code. It runs no SQL, and the prompt text
# is frozen (prompts.jsonl, PREREG), so it can't carry a -- well:ignore marker.
mapfile -d '' FILES < <(git ls-files -z -- '*.sql' '*.go' '*.py' '*.conf' ':!scripts/well/well_lint.py' ':!eval/champion-runbooks/build.py')
current="$(python3 scripts/well/well_lint.py --format json --fail-on NONE "${FILES[@]}" |
	python3 -c '
import collections, json, sys
c = collections.Counter((f["rule"], f["file"]) for f in json.load(sys.stdin)["findings"])
for (rule, path), n in sorted(c.items()):
    print(f"{rule}\t{path}\t{n}")
')"

if [[ "${1:-}" == --update ]]; then
	{
		echo "# The Well lint ratchet (scripts/check-well-lint.sh): rule, file, count."
		echo "# Counts may only go down. Regenerate with --update after a fix."
		printf '%s\n' "$current"
	} >"$BASELINE"
	echo "[UPDATED] ${BASELINE#"$REPO_ROOT"/} ($(printf '%s\n' "$current" | grep -c .) entries)"
	exit 0
fi
[[ -f "$BASELINE" ]] || { echo "[FAIL] ${BASELINE#"$REPO_ROOT"/} is missing; create it with: $0 --update" >&2; exit 1; }

rc=0 shrunk=0
declare -A base
while IFS=$'\t' read -r rule path n; do
	[[ -z "$rule" || "$rule" == \#* ]] && continue
	base["$rule"$'\t'"$path"]=$n
done <"$BASELINE"
declare -A seen
while IFS=$'\t' read -r rule path n; do
	[[ -z "$rule" ]] && continue
	k="$rule"$'\t'"$path"
	seen["$k"]=1
	b="${base[$k]:-0}"
	if ((n > b)); then
		echo "[FAIL] $rule in $path: $n finding(s), baseline $b" >&2
		rc=1
	fi
done <<<"$current"
for k in "${!base[@]}"; do
	if [[ -z "${seen[$k]:-}" ]] || (($(awk -F'\t' -v r="${k%%$'\t'*}" -v p="${k#*$'\t'}" '$1==r && $2==p {print $3}' <<<"$current") < ${base[$k]})); then
		shrunk=1
	fi
done
if ((rc)); then
	echo "       Fix the new finding, or suppress one deliberately in SQL with" >&2
	echo "       '-- well:ignore <rule> <why>' on the statement. Do not grow the baseline." >&2
	exit 1
fi
((shrunk)) && echo "[NOTE] findings went down: run $0 --update to lock the improvement in"
echo "[PASS] The Well lint: no (rule, file) above its baseline ($(printf '%s\n' "$current" | grep -c .) pairs)"
