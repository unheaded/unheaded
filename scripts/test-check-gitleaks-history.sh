#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-check-gitleaks-history.sh — the history scan's verdicts.
#
# check-gitleaks.sh --history warns on a finding that has a row in
# docs/policy/gitleaks-history-baseline.txt, so the job stays green over the
# known set, and fails on one that has none. A scan that never ran fails too:
# a tool that is absent or refused, a scan that errors, a baseline that is
# gone, or a clone with no history.
# Each case runs in a throwaway repository holding a copy of the script.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
PASSED=0
FAILED=0

history_case_result() { # $1 label, $2 0 if it held
	if [ "$2" -eq 0 ]; then
		printf '  PASS  %s\n' "$1"
		PASSED=$((PASSED + 1))
	else
		printf '  FAIL  %s\n' "$1"
		FAILED=$((FAILED + 1))
	fi
}

history_case_repo() { # $1 directory: a repo with one clean commit
	mkdir -p "$1/scripts/lib" "$1/docs/policy"
	cp "${REPO_ROOT}/scripts/check-gitleaks.sh" "$1/scripts/"
	cp "${REPO_ROOT}/scripts/lib/tool-pins.sh" "${REPO_ROOT}/scripts/lib/fetch-tool.sh" "$1/scripts/lib/"
	cp "${REPO_ROOT}/.gitleaks.toml" "$1/"
	echo "# known findings" >"$1/docs/policy/gitleaks-history-baseline.txt"
	echo "nothing to see" >"$1/notes.txt"
	command git -C "$1" init -q
	command git -C "$1" add -A
	command git -C "$1" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false commit -qm first
}

echo "--- a known finding warns, a new one fails ---"
R="${WORK}/clean"
history_case_repo "${R}"
out="$("${R}/scripts/check-gitleaks.sh" --history 2>&1)"
rc=$?
[ "${rc}" -eq 0 ] && grep -q '^\[PASS\]' <<<"${out}"
history_case_result "a clean history passes" $?

printf 'aws_access_key_id = %s%s\n' "AKIA" "Q7ZK3M5R2TXW4P6N" >>"${R}/notes.txt"
command git -C "${R}" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false commit -qam second
printf 'nothing to see\n' >"${R}/notes.txt"
command git -C "${R}" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false commit -qam third
out="$("${R}/scripts/check-gitleaks.sh" --history 2>&1)"
rc=$?
[ "${rc}" -eq 1 ] && grep -q '^\[FAIL\] in history, no row' <<<"${out}" && ! grep -q 'Q7ZK3M5R2TXW4P6N' <<<"${out}"
history_case_result "a finding with no row fails, and its value is not printed" $?

"${R}/scripts/check-gitleaks.sh" --history-rows >>"${R}/docs/policy/gitleaks-history-baseline.txt"
out="$("${R}/scripts/check-gitleaks.sh" --history 2>&1)"
rc=$?
[ "${rc}" -eq 0 ] && grep -q '^::warning.*1 known' <<<"${out}"
history_case_result "the same finding with a row warns, exit 0" $?

command git -C "${R}" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false commit -qam rows
before="$(command git -C "${R}" rev-parse HEAD)"
command git -C "${R}" -c user.name=other -c user.email=o@example.invalid -c commit.gpgsign=false rebase -q --force-rebase --root
after="$(command git -C "${R}" rev-parse HEAD)"
out="$("${R}/scripts/check-gitleaks.sh" --history 2>&1)"
rc=$?
[ "${before}" != "${after}" ] && [ "${rc}" -eq 0 ]
history_case_result "a row survives its commit being rewritten, as re-signing does" $?

printf 'aws_access_key_id = %s%s\n' "AKIA" "Q7ZK3M5R2TXW4P6Y" >"${R}/more.txt"
command git -C "${R}" add more.txt
command git -C "${R}" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false commit -qm fourth
command git -C "${R}" rm -q more.txt
command git -C "${R}" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false commit -qm fifth
"${R}/scripts/check-gitleaks.sh" --history >/dev/null 2>&1
[ $? -eq 1 ]
history_case_result "a second finding fails beside a known one" $?

"${R}/scripts/check-gitleaks.sh" >/dev/null 2>&1
history_case_result "the same finding is not in the tree, so the tree scan still passes" $?

echo "--- a scan that did not run fails ---"
mkdir -p "${WORK}/bin"
printf '#!/bin/sh\nexit 22\n' >"${WORK}/bin/curl"
chmod +x "${WORK}/bin/curl"
XDG_CACHE_HOME="${WORK}/empty-cache" PATH="${WORK}/bin:${PATH}" "${R}/scripts/check-gitleaks.sh" --history >/dev/null 2>&1
[ $? -ne 0 ]
history_case_result "the tool cannot be fetched" $?

# shellcheck source=scripts/lib/tool-pins.sh
source "${REPO_ROOT}/scripts/lib/tool-pins.sh"
mkdir -p "${WORK}/swapped/unheaded-tools/gitleaks-${GITLEAKS_VERSION}"
echo "not the release" >"${WORK}/swapped/unheaded-tools/gitleaks-${GITLEAKS_VERSION}/gitleaks.tar.gz"
XDG_CACHE_HOME="${WORK}/swapped" PATH="${WORK}/bin:${PATH}" "${R}/scripts/check-gitleaks.sh" --history >/dev/null 2>&1
[ $? -ne 0 ]
history_case_result "the cached tool fails its sha256" $?

B="${WORK}/broken"
history_case_repo "${B}"
rm "${B}/.gitleaks.toml"
"${B}/scripts/check-gitleaks.sh" --history >/dev/null 2>&1
[ $? -eq 2 ]
history_case_result "the scan itself errors (config missing)" $?

N="${WORK}/no-baseline"
history_case_repo "${N}"
rm "${N}/docs/policy/gitleaks-history-baseline.txt"
"${N}/scripts/check-gitleaks.sh" --history >/dev/null 2>&1
[ $? -eq 2 ]
history_case_result "the baseline is gone" $?

command git clone -q --depth 1 "file://${R}" "${WORK}/shallow" 2>/dev/null
"${WORK}/shallow/scripts/check-gitleaks.sh" --history >/dev/null 2>&1
[ $? -ne 0 ]
history_case_result "a shallow clone has no history to scan" $?

echo
echo "passed: ${PASSED}  failed: ${FAILED}"
[ "${FAILED}" -eq 0 ] || exit 1
echo "RESULT: PASS"
