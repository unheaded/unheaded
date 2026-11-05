#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-check-ci-verdicts.sh — every rule of check-ci-verdicts.sh bites.
#
# check-gates-can-fail.sh plants one violation per gate. This gate has a rule
# per CI verdict, so each gets its own: a copy of the CI files is broken one
# way at a time, and the gate must exit 1 and name the rule that caught it.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GATE="${REPO_ROOT}/scripts/check-ci-verdicts.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
PASSED=0
FAILED=0

ci_verdict_fresh_copy() {
	rm -rf "${WORK}/tree"
	mkdir -p "${WORK}/tree/.github"
	cp -r "${REPO_ROOT}/.github/workflows" "${WORK}/tree/.github/workflows"
}

# $1 label, $2 the rule name the gate must print, $3 file, $4 sed expression.
ci_verdict_expect_bite() {
	local label="$1" rule="$2" file="$3" expr="$4" out rc=0
	ci_verdict_fresh_copy
	sed -i -E "${expr}" "${WORK}/tree/${file}"
	if cmp -s "${REPO_ROOT}/${file}" "${WORK}/tree/${file}"; then
		printf '  FAIL  %s (the edit changed nothing; fix this test)\n' "${label}"
		FAILED=$((FAILED + 1))
		return
	fi
	out="$(CI_VERDICTS_ROOT="${WORK}/tree" "${GATE}" 2>&1)" || rc=$?
	if [ "${rc}" -eq 1 ] && grep -q "^\[FAIL\] ${rule}:" <<<"${out}"; then
		printf '  PASS  %s\n' "${label}"
		PASSED=$((PASSED + 1))
	else
		printf '  FAIL  %s (exit %s, wanted rule %s)\n' "${label}" "${rc}" "${rule}"
		printf '%s\n' "${out}" | sed 's/^/        /'
		FAILED=$((FAILED + 1))
	fi
}

echo "--- the tree as committed ---"
ci_verdict_fresh_copy
if CI_VERDICTS_ROOT="${WORK}/tree" "${GATE}" >/dev/null 2>&1; then
	echo "  PASS  an unbroken copy passes"
	PASSED=$((PASSED + 1))
else
	echo "  FAIL  an unbroken copy fails"
	FAILED=$((FAILED + 1))
fi

echo "--- Security Gate (M4) ---"
SEC=".github/workflows/security.yml"
ci_verdict_expect_bite "a need dropped from the list" security-gate "${SEC}" \
	's/needs: \[govulncheck, gosec, gitleaks, cargo-audit, trivy-fs\]/needs: [govulncheck, gosec, cargo-audit, trivy-fs]/'
ci_verdict_expect_bite "if: always() removed" security-gate "${SEC}" '/^    if: always\(\)$/d'
ci_verdict_expect_bite "results read but never judged" security-gate "${SEC}" \
	's/if \[ "\$result" != "success" \]; then/if [ "$result" = "never" ]; then/'
ci_verdict_expect_bite "a failed need recorded, then forgotten" security-gate "${SEC}" \
	's/^( +)fail=1$/\1fail=0/'
ci_verdict_expect_bite "needs context replaced by a hand list" security-gate "${SEC}" \
	's/NEEDS_JSON: \$\{\{ toJSON\(needs\) \}\}/NEEDS_JSON: ${{ toJSON(needs.gosec) }}/'

echo
echo "passed: ${PASSED}  failed: ${FAILED}"
[ "${FAILED}" -eq 0 ] || exit 1
echo "RESULT: PASS"
