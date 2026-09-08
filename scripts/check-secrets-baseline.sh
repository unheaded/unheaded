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
#
# THIS GATE COMPARES THE FINGERPRINT SET, NOT A COUNT.
#
# The first version compared totals only, which did not deliver the invariant
# stated above: deleting one stale fingerprint and appending a live one keeps
# the total identical, so a fresh credential passed the gate green. Membership
# is the property that matters — no fingerprint may APPEAR that is not already
# in the manifest. Removals are always allowed and lower the ceiling.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="${REPO_ROOT}/.gitleaksignore"
MANIFEST="${REPO_ROOT}/docs/security/gitleaks-baseline-fingerprints.txt"

[ -f "${BASELINE}" ] || { echo "PASS: no .gitleaksignore — nothing baselined."; exit 0; }

# grep -c exits 1 on zero matches, so `|| echo 0` would append a SECOND line and
# every later [ -gt ] would abort with "integer expression expected" — a rc=2
# that reads as false and falls through to PASS. Collect the lines, then count.
CURRENT_SET="$(grep -E '^[a-zA-Z0-9]' "${BASELINE}" | sort -u || true)"
CURRENT_N="$(printf '%s' "${CURRENT_SET}" | grep -c . || true)"
: "${CURRENT_N:=0}"

# A missing manifest must FAIL, never self-initialise. Writing the current state
# and exiting 0 means deleting one unremarkable text file silently re-baselines
# the gate to whatever is in the tree at that moment — the exact "gate that
# cannot fail" pattern this script exists to eliminate.
if [ ! -f "${MANIFEST}" ]; then
    echo "============================================================"
    echo "  FAIL: ${MANIFEST#"${REPO_ROOT}/"} is missing."
    echo "============================================================"
    echo
    echo "  This file IS the ceiling. Without it there is nothing to"
    echo "  ratchet against, so its absence is a failure and never a"
    echo "  fresh start. Restore it from git:"
    echo
    echo "    git checkout -- docs/security/gitleaks-baseline-fingerprints.txt"
    echo
    echo "  If you are genuinely establishing a baseline for the first"
    echo "  time, create it deliberately and say so in the commit:"
    echo
    echo "    grep -E '^[a-zA-Z0-9]' .gitleaksignore | sort -u > \\"
    echo "      docs/security/gitleaks-baseline-fingerprints.txt"
    echo "============================================================"
    exit 1
fi

ALLOWED_SET="$(grep -E '^[a-zA-Z0-9]' "${MANIFEST}" | sort -u || true)"
ALLOWED_N="$(printf '%s' "${ALLOWED_SET}" | grep -c . || true)"
: "${ALLOWED_N:=0}"

# Set difference: present in .gitleaksignore, absent from the manifest.
ADDED="$(comm -23 <(printf '%s\n' "${CURRENT_SET}") <(printf '%s\n' "${ALLOWED_SET}") | grep -E '^[a-zA-Z0-9]' || true)"

if [ -n "${ADDED}" ]; then
    echo "============================================================"
    echo "  FAIL: new fingerprints were added to the secret baseline"
    echo "============================================================"
    echo
    printf '%s\n' "${ADDED}" | sed 's/^/    + /'
    echo
    echo "  A new secret was baselined instead of removed. Credentials are"
    echo "  never stored in this repo — see CLAUDE.md."
    echo
    echo "  Note this fails even if the TOTAL did not grow: swapping a stale"
    echo "  fingerprint for a live one is exactly the bypass this gate closes."
    echo
    echo "  Fix the finding rather than the baseline:"
    echo "    1. Take the value out of the tree; read it from the environment"
    echo "       (see scripts/bare-metal/validate-host-a.sh for the pattern:"
    echo "       a ':?' guard so the script fails loudly when the var is unset)."
    echo "    2. Rotate it if it ever granted real access."
    echo "    3. Remove its line from .gitleaksignore."
    echo "============================================================"
    exit 1
fi

if [ "${CURRENT_N}" -lt "${ALLOWED_N}" ]; then
    printf '%s\n' "${CURRENT_SET}" | grep -E '^[a-zA-Z0-9]' > "${MANIFEST}" || true
    echo "PASS: baseline shrank ${ALLOWED_N} -> ${CURRENT_N}. Ceiling lowered."
    exit 0
fi

echo "PASS: secret baseline holding at ${CURRENT_N} entries."
