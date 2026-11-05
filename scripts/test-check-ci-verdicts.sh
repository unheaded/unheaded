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
	cp "${REPO_ROOT}/Jenkinsfile" "${REPO_ROOT}/Makefile" "${REPO_ROOT}/go.mod" "${WORK}/tree/"
	cp "${REPO_ROOT}/.github/BRANCH_PROTECTION.md" "${WORK}/tree/.github/"
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
	out="$(CI_VERDICTS_ROOT="${WORK}/tree" CI_VERDICTS_TREE="${REPO_ROOT}" "${GATE}" 2>&1)" || rc=$?
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
if CI_VERDICTS_ROOT="${WORK}/tree" CI_VERDICTS_TREE="${REPO_ROOT}" "${GATE}" >/dev/null 2>&1; then
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
ci_verdict_expect_bite "a needed job that cannot fail" security-gate "${SEC}" \
	's/^(    name: Trivy Filesystem Scan)$/\1\n    continue-on-error: true/'

echo "--- what a merge waits for ---"
BP=".github/BRANCH_PROTECTION.md"
ci_verdict_expect_bite "Security Gate dropped from the command" branch-protection "${BP}" \
	'/^      "Security Gate",$/d'
ci_verdict_expect_bite "Security Gate dropped from the prose" branch-protection "${BP}" \
	'/^- \[x\] `Security Gate`/d'
ci_verdict_expect_bite "a job id where the check is its name" branch-protection "${BP}" \
	's/^      "CI Gate",$/      "CI Gate",\n      "go-build",/'
ci_verdict_expect_bite "the summary job renamed in the workflow" branch-protection "${SEC}" \
	's/^    name: Security Gate$/    name: Security Summary/'

echo "--- history secret scan (M5) ---"
ci_verdict_expect_bite "the step softened" history-scan "${SEC}" \
	's/^( +)run: \.\/scripts\/check-gitleaks\.sh --history$/\1continue-on-error: true\n&/'
ci_verdict_expect_bite "the step removed" history-scan "${SEC}" \
	's/check-gitleaks\.sh --history$/check-gitleaks.sh/'
ci_verdict_expect_bite "a binary path nothing creates" history-scan "${SEC}" \
	's/^( +)run: \.\/scripts\/check-secrets-baseline\.sh$/\1run: \/tmp\/gitleaks git ./'
ci_verdict_expect_bite "the step made conditional" history-scan "${SEC}" \
	's/^( +)run: \.\/scripts\/check-gitleaks\.sh --history$/\1if: false\n&/'
ci_verdict_expect_bite "the scan's own test removed" history-scan "${SEC}" \
	'/test-check-gitleaks-history\.sh$/d;/The history scan.s verdicts hold/d'
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
ci_verdict_expect_bite "the vuln gate softened" trivy "${SEC}" \
	's/^( +)- name: Trivy vulnerability gate \(GATING\)$/&\n\1  continue-on-error: true/'
ci_verdict_expect_bite "the vuln gate switched off" trivy "${SEC}" \
	'/Trivy vulnerability gate \(GATING\)$/{n;s/if: always\(\)/if: false/}'
ci_verdict_expect_bite "the listener check switched off" auth-listeners "${SEC}" \
	's/^( +)run: \.\/scripts\/check-http-auth\.sh$/\1if: false\n&/'
ci_verdict_expect_bite "the listener check replaced by a note" auth-listeners "${SEC}" \
	's/run: \.\/scripts\/check-http-auth\.sh/run: echo manual review the servers/'

echo "--- tool fetches (L22) ---"
ci_verdict_expect_bite "@latest in a workflow" tool-fetch "${SEC}" \
	's/(govulncheck)@v[0-9.]+/\1@latest/'
ci_verdict_expect_bite "@latest behind a flag" tool-fetch "${SEC}" \
	's/go install (golang\.org\/x\/vuln\/cmd\/govulncheck)@v[0-9.]+/go install -v \1@latest/'
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
ci_verdict_expect_bite "an install script piped to a shell" tool-fetch "jenkins/Jenkinsfile.security" \
	's#^( +)\./scripts/install-trivy\.sh .*$#\1curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/v0.76.1/contrib/install.sh | sh -s -- -b /usr/local/bin v0.76.1#'
ci_verdict_expect_bite "no sha256 for the trivy tarball" tool-fetch "scripts/lib/tool-pins.sh" \
	'/^TRIVY_SHA256=/d'
ci_verdict_expect_bite "a pip version that floats" tool-fetch ".github/workflows/raft-python.yml" \
	's/pytest==[0-9.]+/pytest==8.*/'
ci_verdict_expect_bite "pip upgraded to whatever is newest" tool-fetch ".github/workflows/raft-python.yml" \
	's/pip install pip==[0-9.]+/pip install --upgrade pip/'
ci_verdict_expect_bite "a Go version nothing reads" tool-fetch "Jenkinsfile" \
	"s/^( +)GOFLAGS = /\1GO_VERSION = '1.24'\n\1GOFLAGS = /"

echo "--- what stops a job before it scans anything ---"
ci_verdict_expect_bite "an empty environment block" jenkins "jenkins/Jenkinsfile.security" \
	's/^  stages \{$/  environment {\n    \/\/ nothing left\n  }\n\n&/'
ci_verdict_expect_bite "an agent older than go.mod" jenkins "jenkins/Jenkinsfile.protocol" \
	"s/image 'golang:[^']+'/image 'golang:1.25'/"
ci_verdict_expect_bite "a target rustup cannot install" rust-target ".github/workflows/release.yml" \
	's/^( +)components: rust-src$/&\n\1targets: bpfel-unknown-none/'

echo "--- paths a step names ---"
DOCKER_WF=".github/workflows/docker.yml"
ci_verdict_expect_bite "a script nothing tracks" workflow-paths "${SEC}" \
	's#^( +)run: \./scripts/check-well-lint\.sh$#\1run: ./scripts/check-well-lint-gone.sh#'
ci_verdict_expect_bite "a script reached through bash" workflow-paths ".github/workflows/static-analysis.yml" \
	's#bash tests/sealed-cask/round_trip_test\.sh#bash tests/sealed-cask/round_trip_gone.sh#'
ci_verdict_expect_bite "a script run directly without its mode" workflow-paths "${SEC}" \
	's#^( +)source scripts/lib/tool-pins\.sh$#\1scripts/lib/tool-pins.sh#'
ci_verdict_expect_bite "a working directory that left" workflow-paths ".github/workflows/ebpf.yml" \
	's#working-directory: crates/svc-token#working-directory: crates/svc-token-gone#'
ci_verdict_expect_bite "a lockfile the cache key hashes that left" workflow-paths ".github/workflows/ebpf.yml" \
	"s#hashFiles\('ebpf/Cargo.lock'\)#hashFiles('ebpf-gone/Cargo.lock')#"
ci_verdict_expect_bite "a Dockerfile that left" workflow-paths "${DOCKER_WF}" \
	's#file: \./Dockerfile#file: ./wotan/Dockerfile#'
ci_verdict_expect_bite "a build context that left" workflow-paths "${DOCKER_WF}" \
	's#context: \.$#context: ./unheaded#'
ci_verdict_expect_bite "a stage the Dockerfile lacks" workflow-paths "${DOCKER_WF}" \
	's#target: wotan#target: wotan-gone#'

echo
echo "passed: ${PASSED}  failed: ${FAILED}"
[ "${FAILED}" -eq 0 ] || exit 1
echo "RESULT: PASS"
