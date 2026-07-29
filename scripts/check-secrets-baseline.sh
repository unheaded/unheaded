#!/bin/bash
# SPDX-License-Identifier: MIT
# check-secrets-baseline.sh — the gitleaks baseline may only SHRINK.
#
# .gitleaksignore lists fingerprints of secret findings that predate the
# 2026-07-29 sweep. Without this guard the file is an open door: anyone hitting
# a gitleaks failure can silence it by appending the new fingerprint, and the
# gate keeps reporting green while a fresh credential sits in the tree.
#
# That is the same shape as the three gates this repo already had which could
# not fail (cargo-audit `|| true`, //nolint annotations gosec ignores, trivy
# scanners never enabled). A suppression list guarded only by good intentions
# becomes the next one.
#
# Policy, per Stevie 2026-07-29: credentials are NEVER stored in the repo. The
# baselined entries are one-off lab credentials on a non-internet-facing dev
# system — an accepted, documented risk, not a licence to add more.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="${REPO_ROOT}/.gitleaksignore"
LIMIT_FILE="${REPO_ROOT}/docs/security/gitleaks-baseline-count.txt"

[ -f "${BASELINE}" ] || { echo "PASS: no .gitleaksignore — nothing baselined."; exit 0; }

CURRENT="$(grep -cE '^[a-zA-Z0-9]' "${BASELINE}" 2>/dev/null || echo 0)"

if [ ! -f "${LIMIT_FILE}" ]; then
    echo "${CURRENT}" > "${LIMIT_FILE}"
    echo "PASS: baseline ceiling initialised at ${CURRENT}."
    exit 0
fi

ALLOWED="$(tr -dc '0-9' < "${LIMIT_FILE}")"
: "${ALLOWED:=0}"

if [ "${CURRENT}" -gt "${ALLOWED}" ]; then
    echo "============================================================"
    echo "  FAIL: the secret baseline GREW (${ALLOWED} -> ${CURRENT})"
    echo "============================================================"
    echo
    echo "  A new secret was baselined instead of removed. Credentials are"
    echo "  never stored in this repo — see CLAUDE.md."
    echo
    echo "  Fix the finding rather than the baseline:"
    echo "    1. Take the value out of the tree; read it from the environment"
    echo "       (see scripts/bare-metal/validate-host-a.sh for the pattern:"
    echo "       a ':?' guard so the script fails loudly when the var is unset)."
    echo "    2. Rotate it if it ever granted real access."
    echo "    3. Remove its line from .gitleaksignore."
    echo
    echo "  The ceiling lives in docs/security/gitleaks-baseline-count.txt and"
    echo "  should only ever be lowered."
    echo "============================================================"
    exit 1
fi

if [ "${CURRENT}" -lt "${ALLOWED}" ]; then
    echo "${CURRENT}" > "${LIMIT_FILE}"
    echo "PASS: baseline shrank ${ALLOWED} -> ${CURRENT}. Ceiling lowered."
    exit 0
fi

echo "PASS: secret baseline holding at ${CURRENT} entries."
