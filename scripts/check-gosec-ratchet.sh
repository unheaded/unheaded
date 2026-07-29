#!/bin/bash
# SPDX-License-Identifier: MIT
# check-gosec-ratchet.sh — enforce that the gosec exclusion list only SHRINKS.
#
# The gate in .github/workflows/security.yml suppresses gosec rules by name
# while their findings are worked through
# (docs/security/findings-remediation-2026-07-29.md). That list is a security
# liability: nothing in YAML stops someone appending a rule to make a red
# build go green, and the diff looks like a one-word config tweak.
#
# This session found THREE gates that were green because they could not fail
# (cargo-audit `|| true`, gosec `//nolint` annotations it ignores, trivy
# scanners never enabled). A suppression list guarded only by good intentions
# is the same bug waiting to happen a fourth time.
#
# Contract:
#   workflow exclusions MUST be a subset of the baseline.
#   Removing a rule (remediated)  -> PASS, and the baseline should be updated.
#   Adding a rule (new suppression) -> FAIL. Fix the finding instead.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKFLOW="${REPO_ROOT}/.github/workflows/security.yml"
BASELINE="${REPO_ROOT}/docs/security/gosec-ratchet-baseline.txt"

[ -f "${WORKFLOW}" ] || { echo "FAIL: missing ${WORKFLOW}"; exit 1; }
[ -f "${BASELINE}" ] || { echo "FAIL: missing ${BASELINE}"; exit 1; }

CURRENT="$(grep -oP '(?<=-exclude=)[A-Z0-9,]+' "${WORKFLOW}" | tr ',' '\n' | grep -E '^G[0-9]+$' | sort -u)"
ALLOWED="$(grep -E '^G[0-9]+$' "${BASELINE}" | sort -u)"

if [ -z "${CURRENT}" ]; then
    echo "PASS: gosec runs with no rule exclusions — ratchet fully closed."
    exit 0
fi

# Any rule excluded in the workflow but absent from the baseline is a NEW
# suppression. That is the thing this script exists to stop.
ADDED="$(comm -23 <(printf '%s\n' "${CURRENT}") <(printf '%s\n' "${ALLOWED}"))"
REMOVED="$(comm -13 <(printf '%s\n' "${CURRENT}") <(printf '%s\n' "${ALLOWED}"))"

if [ -n "${ADDED}" ]; then
    echo "============================================================"
    echo "  FAIL: gosec ratchet went BACKWARDS"
    echo "============================================================"
    echo
    echo "  New rule suppressions not present in the baseline:"
    printf '    %s\n' ${ADDED}
    echo
    echo "  The exclusion list may only shrink. Suppressing a new rule"
    echo "  hides findings rather than fixing them."
    echo
    echo "  If a finding is a genuine false positive, annotate the SITE"
    echo "  with '// #nosec Gxxx -- <reason>' (note: gosec ignores"
    echo "  //nolint:gosec) rather than disabling the rule tree-wide."
    echo
    echo "  Baseline: docs/security/gosec-ratchet-baseline.txt"
    echo "  Plan:     docs/security/findings-remediation-2026-07-29.md"
    echo "============================================================"
    exit 1
fi

echo "PASS: gosec ratchet intact — $(printf '%s\n' "${CURRENT}" | wc -l)/$(printf '%s\n' "${ALLOWED}" | wc -l) baseline rules still excluded."
if [ -n "${REMOVED}" ]; then
    echo
    echo "  Progress — these rules are now ENFORCED and can never be re-added:"
    printf '    %s\n' ${REMOVED}
    echo
    echo "  Update the baseline to lock the win in:"
    echo "    ./scripts/check-gosec-ratchet.sh --update"
fi

if [ "${1:-}" = "--update" ]; then
    printf '%s\n' "${CURRENT}" > "${BASELINE}"
    echo "  Baseline updated -> $(printf '%s\n' "${CURRENT}" | wc -l) rules."
fi
exit 0
