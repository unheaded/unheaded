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
	mkdir -p "${WORK}/tree/.github" "${WORK}/tree/scripts/lib" "${WORK}/tree/nix/yggdrasil"
	cp -r "${REPO_ROOT}/.github/workflows" "${WORK}/tree/.github/workflows"
	cp -r "${REPO_ROOT}/jenkins" "${WORK}/tree/jenkins"
	cp "${REPO_ROOT}/Jenkinsfile" "${REPO_ROOT}/Makefile" "${WORK}/tree/"
	cp "${REPO_ROOT}/nix/yggdrasil/Jenkinsfile" "${WORK}/tree/nix/yggdrasil/"
	cp "${REPO_ROOT}/scripts/lib/tool-pins.sh" "${WORK}/tree/scripts/lib/"
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

echo "--- history secret scan (M5) ---"
ci_verdict_expect_bite "the step softened" history-scan "${SEC}" \
	's/^( +)run: \.\/scripts\/check-gitleaks\.sh --history$/\1continue-on-error: true\n&/'
ci_verdict_expect_bite "the step removed" history-scan "${SEC}" \
	's/check-gitleaks\.sh --history$/check-gitleaks.sh/'
ci_verdict_expect_bite "a binary path nothing creates" history-scan "${SEC}" \
	's/^( +)run: \.\/scripts\/check-secrets-baseline\.sh$/\1run: \/tmp\/gitleaks git ./'
ci_verdict_expect_bite "history not fetched" history-scan "${SEC}" \
	's/fetch-depth: 0   # full history — see note below/fetch-depth: 1/'

echo "--- bpf-linker (M22) ---"
ci_verdict_expect_bite "release.yml unpinned" bpf-linker ".github/workflows/release.yml" \
	's/cargo install bpf-linker@[0-9.]+ --locked/cargo install bpf-linker --no-default-features/'
ci_verdict_expect_bite "ebpf.yml without --locked" bpf-linker ".github/workflows/ebpf.yml" \
	's/(cargo install bpf-linker@[0-9.]+) --locked/\1/'
ci_verdict_expect_bite "the workflows disagree" bpf-linker ".github/workflows/ebpf.yml" \
	's/cargo install bpf-linker@[0-9.]+ --locked/cargo install bpf-linker@0.11.0 --locked/'

echo "--- steps that could not fail (L21) ---"
ci_verdict_expect_bite "the vuln gate reports only" trivy "${SEC}" \
	"s/^( +)exit-code: '1'\$/\1exit-code: '0'/"
ci_verdict_expect_bite "a scanner step with no exit-code" trivy "${SEC}" \
	"/^          exit-code: '0'\$/d"
ci_verdict_expect_bite "the vuln gate moved to sarif" trivy "${SEC}" \
	's/^( +)format: table$/\1format: sarif/'
ci_verdict_expect_bite "the vuln gate reads HIGH no more" trivy "${SEC}" \
	'/severity: CRITICAL,HIGH$/{N;s/CRITICAL,HIGH(\n +ignore-unfixed)/CRITICAL\1/}'
ci_verdict_expect_bite "the listener check replaced by a note" auth-listeners "${SEC}" \
	's/run: \.\/scripts\/check-http-auth\.sh/run: echo manual review the servers/'

echo "--- tool fetches (L22) ---"
ci_verdict_expect_bite "@latest in a workflow" tool-fetch "${SEC}" \
	's/(govulncheck)@v[0-9.]+/\1@latest/'
ci_verdict_expect_bite "@latest in the Jenkinsfile" tool-fetch "Jenkinsfile" \
	's/(go-licenses)@v[0-9.]+/\1@latest/'
ci_verdict_expect_bite "@latest in jenkins/vars" tool-fetch "jenkins/vars/unheadedPipeline.groovy" \
	's/(govulncheck)@v[0-9.]+/\1@latest/'
ci_verdict_expect_bite "a bare cargo install" tool-fetch "${SEC}" \
	's/cargo install cargo-audit@[0-9.]+ --locked/cargo install cargo-audit/'
ci_verdict_expect_bite "cargo install without --locked" tool-fetch "jenkins/Jenkinsfile.protocol" \
	's/(cargo install cargo-audit@[0-9.]+) --locked/\1/'
ci_verdict_expect_bite "an install whose failure is swallowed" tool-fetch ".github/workflows/gpl-boundary.yml" \
	's/(cargo install cargo-license@[0-9.]+ --locked)/\1 || true/'
ci_verdict_expect_bite "two versions of one tool" tool-fetch "${SEC}" \
	's/(govulncheck)@v[0-9.]+/\1@v1.8.0/'
ci_verdict_expect_bite "an install script read from a branch" tool-fetch "jenkins/Jenkinsfile.security" \
	's#aquasecurity/trivy/v[0-9.]+/#aquasecurity/trivy/main/#'
ci_verdict_expect_bite "the trivy pin moved, Jenkins did not" tool-fetch "scripts/lib/tool-pins.sh" \
	's/^TRIVY_VERSION="[^"]+"/TRIVY_VERSION="0.1.0"/'
ci_verdict_expect_bite "a Go version nothing reads" tool-fetch "Jenkinsfile" \
	"s/^( +)GOFLAGS = /\1GO_VERSION = '1.24'\n\1GOFLAGS = /"

echo
echo "passed: ${PASSED}  failed: ${FAILED}"
[ "${FAILED}" -eq 0 ] || exit 1
echo "RESULT: PASS"
