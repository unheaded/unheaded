#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-check-nosec-ratchet.sh: the suppression ratchet refuses the gosec options
# that drop findings without touching its count, and leaves other tools' flags
# alone. Each case is a throwaway repository with a copy of the gate, two Go
# files and a matching baseline.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
PASSED=0
FAILED=0

# $1 label, $2 exit status wanted, stdin the workflow under test.
ratchet_case() {
	local dir rc=0
	dir="$(mktemp -d "${WORK}/case.XXXXXX")"
	mkdir -p "${dir}/scripts" "${dir}/docs/security" "${dir}/.github/workflows"
	cp "${REPO_ROOT}/scripts/check-nosec-ratchet.sh" "${dir}/scripts/"
	printf 'package a\n\nvar _ = 1 // #nosec G101\n' >"${dir}/a.go"
	printf 'package a\n\nvar _ = 2 // #nosec G101\n' >"${dir}/b.go"
	printf 'a.go 1\nb.go 1\n' >"${dir}/docs/security/nosec-baseline.txt"
	cat >"${dir}/.github/workflows/scan.yml"
	command git -C "${dir}" init -q
	command git -C "${dir}" add a.go b.go .github
	"${dir}/scripts/check-nosec-ratchet.sh" >/dev/null 2>&1 || rc=$?
	if [ "${rc}" -eq "$2" ]; then
		printf '  PASS  %s\n' "$1"
		PASSED=$((PASSED + 1))
	else
		printf '  FAIL  %s (exit %s, wanted %s)\n' "$1" "${rc}" "$2"
		FAILED=$((FAILED + 1))
	fi
}

echo "--- these leave the count honest ---"
ratchet_case "plain gosec" 0 <<'YAML'
run: gosec -fmt sarif -out gosec-results.sarif ./...
YAML
ratchet_case "another tool's --exclude-dir" 0 <<'YAML'
run: |
  grep -rn \
    --exclude-dir=vendor \
    pattern .
YAML

echo "--- these do not ---"
ratchet_case "-nosec-tag" 1 <<'YAML'
run: gosec -nosec-tag=falsepositive ./...
YAML
ratchet_case "-conf" 1 <<'YAML'
run: gosec -conf gosec.json ./...
YAML
ratchet_case "-exclude-dir" 1 <<'YAML'
run: gosec -exclude-dir=pkg/database ./...
YAML
ratchet_case "-exclude-generated" 1 <<'YAML'
run: gosec -exclude-generated ./...
YAML
ratchet_case "a flag on a continuation line" 1 <<'YAML'
run: |
  "$RUNNER_TEMP/gosec" -fmt sarif \
    -conf gosec.json \
    ./...
YAML
ratchet_case "NoSecAlternative in a tracked file" 1 <<'YAML'
run: gosec ./... # {"global": {"NoSecAlternative": "#falsepositive"}}
YAML

printf '\n%s passed, %s failed\n' "${PASSED}" "${FAILED}"
[ "${FAILED}" -eq 0 ]
