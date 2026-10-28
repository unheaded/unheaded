#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
# verify-binding-rune.sh — Verify a Sealed Cask deployment package
# ADR-010: Sealed Cask Deployment (Soul Vessel Model)
#
# Delegates to cask-verify (cmd/cask-verify, pkg/sealedcask): the Binding Rune
# must carry the build key's ML-DSA-65 seal, every file must match it, and the
# tarball is read as hostile input (no links, devices, "..", absolute names,
# oversize members or trailing bytes). Nothing is extracted.
#
# Usage:
#   ./scripts/verify-binding-rune.sh [--pubkey build.pubkey]... <cask.tar.gz>
#   ./scripts/verify-binding-rune.sh --insecure-sha-only <cask.tar.gz>
#
#   --pubkey   trusted build public key, repeatable during a rotation overlap
#              (default: $UNHEADED_BUILD_PUBKEY, else compliance/keys/build.pubkey)
#   --insecure-sha-only
#              the pre-2026-10-27 check: sha256 of each file against the
#              manifest, no signature. Anyone who can write the cask can
#              rewrite the manifest too, so this proves nothing about origin.
#
# Environment: CASK_VERIFY names the cask-verify binary (default: on PATH,
# else `go run ./cmd/cask-verify`).

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PUBKEYS=()
INSECURE=0
CASK=""
while [ $# -gt 0 ]; do
    case "$1" in
        --pubkey) PUBKEYS+=("${2:?--pubkey needs a path}"); shift 2 ;;
        --insecure-sha-only) INSECURE=1; shift ;;
        -h|--help) sed -n '2,22p' "$0"; exit 0 ;;
        -*) echo "verify-binding-rune: unknown option $1" >&2; exit 2 ;;
        *) [ -z "${CASK}" ] || { echo "verify-binding-rune: one cask" >&2; exit 2; }
           CASK="$1"; shift ;;
    esac
done
[ -n "${CASK}" ] || { echo "Usage: verify-binding-rune.sh [--pubkey build.pubkey]... <cask.tar.gz>" >&2; exit 2; }

echo "=============================================="
echo "BINDING RUNE VERIFICATION — ADR-010"
echo "=============================================="
echo "Cask:  ${CASK}"
echo "Time:  $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

if [ ! -f "${CASK}" ]; then
    echo "ERROR: Cask file not found: ${CASK}"
    exit 1
fi

if [ "${INSECURE}" -eq 0 ]; then
    if [ "${#PUBKEYS[@]}" -eq 0 ]; then
        PUBKEYS=("${UNHEADED_BUILD_PUBKEY:-${PROJECT_ROOT}/compliance/keys/build.pubkey}")
    fi
    ARGS=(verify --cask "${CASK}")
    for k in "${PUBKEYS[@]}"; do
        if [ ! -f "${k}" ]; then
            echo "ERROR: build public key not found: ${k}"
            echo "  (compliance/keys/build.pubkey is published after the build-key ceremony, S-IRON-GATE E1)"
            exit 2
        fi
        ARGS+=(--pubkey "${k}")
    done
    if [ -n "${CASK_VERIFY:-}" ]; then
        CV=("${CASK_VERIFY}")
    elif command -v cask-verify >/dev/null 2>&1; then
        CV=(cask-verify)
    else
        CV=(go -C "${PROJECT_ROOT}" run ./cmd/cask-verify)
    fi
    if "${CV[@]}" "${ARGS[@]}"; then
        echo ""
        echo "VERDICT: BINDING RUNE INTACT — signed by a trusted build key, every file matches"
        exit 0
    fi
    echo ""
    echo "VERDICT: BINDING RUNE BROKEN OR UNSIGNED — DO NOT DEPLOY this cask. Rebuild from source."
    exit 1
fi

# ---------------------------------------------------------------------------
# --insecure-sha-only: the legacy hash-only check
# ---------------------------------------------------------------------------
echo "##############################################################"
echo "## INSECURE: --insecure-sha-only checks hashes, NOT the     ##"
echo "## signature. A tampered cask with a rewritten manifest     ##"
echo "## passes this. Never use it to decide a deployment.        ##"
echo "##############################################################"
echo ""

VERIFY_DIR=$(mktemp -d "${TMPDIR:-/tmp}/verify-cask.XXXXXXXX")
trap 'rm -rf "${VERIFY_DIR}"' EXIT

echo "=== Extracting ==="
tar xzf "${CASK}" -C "${VERIFY_DIR}" --no-same-owner --no-same-permissions
CASK_DIR=$(find "${VERIFY_DIR}" -maxdepth 1 -type d -name "sealed-cask-*" | head -1)
if [ -z "${CASK_DIR}" ]; then
    echo "ERROR: No sealed-cask directory found in archive"
    exit 1
fi
echo "  Extracted to: ${CASK_DIR}"

MANIFEST="${CASK_DIR}/BINDING_RUNE.sha256"
if [ ! -f "${MANIFEST}" ]; then
    echo "ERROR: BINDING_RUNE.sha256 not found — cask may be corrupted or tampered"
    exit 1
fi

echo ""
echo "=== Verifying file integrity (hash only) ==="
cd "${CASK_DIR}"

TOTAL=$(wc -l < "${MANIFEST}")
PASS=0
FAIL=0
MISSING=0

while IFS= read -r line; do
    EXPECTED_HASH="${line%%  *}"
    FILE="${line#*  }"

    if [ ! -f "${FILE}" ] || [ -L "${FILE}" ]; then
        echo "  [MISSING] ${FILE}"
        MISSING=$((MISSING + 1))
        continue
    fi

    ACTUAL_HASH=$(sha256sum "${FILE}" | awk '{print $1}')
    if [ "${EXPECTED_HASH}" = "${ACTUAL_HASH}" ]; then
        PASS=$((PASS + 1))
    else
        echo "  [MODIFIED] ${FILE}"
        FAIL=$((FAIL + 1))
    fi
done < "${MANIFEST}"

EXTRA=$(find . ! -type d ! -name BINDING_RUNE.sha256 ! -name BINDING_RUNE.sig | sort | while read -r f; do
    if ! awk -v f="$f" 'substr($0, 67) == f { found = 1 } END { exit !found }' "${MANIFEST}"; then
        echo "  [EXTRA] $f"
    fi
done)
EXTRA_COUNT=$(echo "$EXTRA" | grep -c "EXTRA" || true)

echo ""
echo "  Total files:  ${TOTAL}"
echo "  Passed:       ${PASS}"
echo "  Modified:     ${FAIL}"
echo "  Missing:      ${MISSING}"
echo "  Extra:        ${EXTRA_COUNT}"
[ -z "$EXTRA" ] || { echo ""; echo "$EXTRA"; }

echo ""
if [ "${FAIL}" -eq 0 ] && [ "${MISSING}" -eq 0 ] && [ "${EXTRA_COUNT}" -eq 0 ]; then
    echo "VERDICT (INSECURE, hash only): hashes match the manifest; origin NOT verified"
    exit 0
fi
echo "VERDICT: BINDING RUNE BROKEN — DO NOT DEPLOY this cask. Rebuild from source."
exit 1
