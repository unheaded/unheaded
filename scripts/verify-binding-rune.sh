#!/bin/bash
# verify-binding-rune.sh — Verify integrity of a Sealed Cask deployment package
# ADR-010: Sealed Cask Deployment (Soul Vessel Model)
#
# Extracts the cask, verifies all file hashes against the Binding Rune manifest,
# and reports any modifications, additions, or deletions.
#
# Usage:
#   ./scripts/verify-binding-rune.sh /path/to/cask.tar.gz

set -euo pipefail

CASK="${1:?Usage: verify-binding-rune.sh <cask.tar.gz>}"
VERIFY_DIR="/tmp/verify-cask-$$"

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

# --- Extract ---
echo "=== Extracting ==="
mkdir -p "${VERIFY_DIR}"
tar xzf "${CASK}" -C "${VERIFY_DIR}"
CASK_DIR=$(find "${VERIFY_DIR}" -maxdepth 1 -type d -name "sealed-cask-*" | head -1)
if [ -z "${CASK_DIR}" ]; then
    echo "ERROR: No sealed-cask directory found in archive"
    rm -rf "${VERIFY_DIR}"
    exit 1
fi
echo "  Extracted to: ${CASK_DIR}"

# --- Verify Binding Rune ---
MANIFEST="${CASK_DIR}/BINDING_RUNE.sha256"
if [ ! -f "${MANIFEST}" ]; then
    echo "ERROR: BINDING_RUNE.sha256 not found — cask may be corrupted or tampered"
    rm -rf "${VERIFY_DIR}"
    exit 1
fi

echo ""
echo "=== Verifying file integrity ==="
cd "${CASK_DIR}"

TOTAL=$(wc -l < "${MANIFEST}")
PASS=0
FAIL=0
MISSING=0

while IFS= read -r line; do
    EXPECTED_HASH=$(echo "$line" | awk '{print $1}')
    FILE=$(echo "$line" | awk '{print $2}')

    if [ ! -f "${FILE}" ]; then
        echo "  [MISSING] ${FILE}"
        MISSING=$((MISSING + 1))
        continue
    fi

    ACTUAL_HASH=$(sha256sum "${FILE}" | awk '{print $1}')
    if [ "${EXPECTED_HASH}" = "${ACTUAL_HASH}" ]; then
        PASS=$((PASS + 1))
    else
        echo "  [MODIFIED] ${FILE}"
        echo "    Expected: ${EXPECTED_HASH}"
        echo "    Actual:   ${ACTUAL_HASH}"
        FAIL=$((FAIL + 1))
    fi
done < "${MANIFEST}"

# Check for extra files not in manifest
EXTRA=$(find . -type f -not -name "BINDING_RUNE.sha256" | sort | while read f; do
    if ! grep -q "$f" "${MANIFEST}" 2>/dev/null; then
        echo "  [EXTRA] $f"
    fi
done)
EXTRA_COUNT=$(echo "$EXTRA" | grep -c "EXTRA" || true)

# --- Report ---
echo ""
echo "=============================================="
echo "VERIFICATION RESULT"
echo "=============================================="
echo "  Total files:  ${TOTAL}"
echo "  Passed:       ${PASS}"
echo "  Modified:     ${FAIL}"
echo "  Missing:      ${MISSING}"
echo "  Extra:        ${EXTRA_COUNT}"

if [ -n "$EXTRA" ]; then
    echo ""
    echo "$EXTRA"
fi

# --- Cleanup ---
rm -rf "${VERIFY_DIR}"

# --- Verdict ---
echo ""
if [ "${FAIL}" -eq 0 ] && [ "${MISSING}" -eq 0 ] && [ "${EXTRA_COUNT}" -eq 0 ]; then
    echo "VERDICT: BINDING RUNE INTACT — cask is authentic and unmodified"
    exit 0
else
    echo "VERDICT: BINDING RUNE BROKEN — cask has been modified or corrupted"
    echo "  DO NOT DEPLOY this cask. Rebuild from source."
    exit 1
fi
