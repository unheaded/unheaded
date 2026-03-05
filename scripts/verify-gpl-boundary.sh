#!/bin/bash
# SPDX-License-Identifier: MIT
# verify-gpl-boundary.sh - Scan project for GPL/AGPL license contamination
# Part of Unheaded S77 Phase 3: SBOM + CI/CD Fortress
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RESULTS_DIR="${REPO_ROOT}/sbom-results"
REPORT="${RESULTS_DIR}/gpl-boundary-report.txt"

mkdir -p "${RESULTS_DIR}"

FAIL=0
TOTAL_FILES=0
MIT_COUNT=0
APACHE_COUNT=0
BSD_COUNT=0
GPL_COUNT=0
AGPL_COUNT=0
LGPL_COUNT=0
OTHER_COUNT=0
NO_HEADER_COUNT=0

echo "========================================" > "${REPORT}"
echo "  GPL Boundary Verification Report" >> "${REPORT}"
echo "  Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "${REPORT}"
echo "========================================" >> "${REPORT}"
echo "" >> "${REPORT}"

# -------------------------------------------------------------------
# 1. Scan Go files for SPDX-License-Identifier headers
# -------------------------------------------------------------------
echo "--- SPDX Header Scan (Go files) ---" >> "${REPORT}"

while IFS= read -r -d '' f; do
    TOTAL_FILES=$((TOTAL_FILES + 1))
    HEADER=$(head -5 "$f" | grep -oP 'SPDX-License-Identifier:\s*\K\S+' || true)
    if [ -z "${HEADER}" ]; then
        NO_HEADER_COUNT=$((NO_HEADER_COUNT + 1))
    else
        case "${HEADER}" in
            MIT)           MIT_COUNT=$((MIT_COUNT + 1)) ;;
            Apache-2.0)    APACHE_COUNT=$((APACHE_COUNT + 1)) ;;
            BSD-2-Clause|BSD-3-Clause) BSD_COUNT=$((BSD_COUNT + 1)) ;;
            GPL-2.0*|GPL-3.0*) GPL_COUNT=$((GPL_COUNT + 1)) ;;
            AGPL-3.0*)     AGPL_COUNT=$((AGPL_COUNT + 1)) ;;
            LGPL-2.1*|LGPL-3.0*) LGPL_COUNT=$((LGPL_COUNT + 1)) ;;
            *)             OTHER_COUNT=$((OTHER_COUNT + 1)) ;;
        esac
    fi
done < <(find "${REPO_ROOT}" -name '*.go' -not -path '*/vendor/*' -print0)

{
    echo "Total Go files scanned: ${TOTAL_FILES}"
    echo "  MIT:            ${MIT_COUNT}"
    echo "  Apache-2.0:     ${APACHE_COUNT}"
    echo "  BSD:            ${BSD_COUNT}"
    echo "  GPL:            ${GPL_COUNT}"
    echo "  AGPL:           ${AGPL_COUNT}"
    echo "  LGPL:           ${LGPL_COUNT}"
    echo "  Other:          ${OTHER_COUNT}"
    echo "  No header:      ${NO_HEADER_COUNT}"
    echo ""
} >> "${REPORT}"

if [ "${GPL_COUNT}" -gt 0 ] || [ "${AGPL_COUNT}" -gt 0 ]; then
    echo "FAIL: GPL/AGPL SPDX headers found in Go source files" >> "${REPORT}"
    FAIL=1
else
    echo "PASS: No GPL/AGPL SPDX headers in Go source files" >> "${REPORT}"
fi
echo "" >> "${REPORT}"

# -------------------------------------------------------------------
# 2. Check go.mod dependencies for GPL/AGPL licenses
# -------------------------------------------------------------------
echo "--- go.mod Dependency License Check ---" >> "${REPORT}"

GO_MOD="${REPO_ROOT}/go.mod"
GPL_GO_DEPS=()
if [ -f "${GO_MOD}" ]; then
    # Extract module paths from require blocks (skip replace, indirect comments kept)
    while IFS= read -r mod; do
        # Known GPL Go modules (common patterns)
        mod_lower=$(echo "${mod}" | tr '[:upper:]' '[:lower:]')
        if echo "${mod_lower}" | grep -qiE '(gpl|gnu|copyleft|fsf\.org)'; then
            GPL_GO_DEPS+=("${mod}")
        fi
    done < <(grep -E '^\t' "${GO_MOD}" | awk '{print $1}' | grep -v '^//')

    # Also try go-licenses if available
    if command -v go-licenses &>/dev/null; then
        echo "  (go-licenses available, running deep scan)" >> "${REPORT}"
        GPL_DEEP=$(cd "${REPO_ROOT}" && go-licenses check ./... 2>&1 | grep -iE 'GPL|AGPL' || true)
        if [ -n "${GPL_DEEP}" ]; then
            echo "  go-licenses found GPL/AGPL references:" >> "${REPORT}"
            echo "  ${GPL_DEEP}" >> "${REPORT}"
            FAIL=1
        fi
    fi

    if [ ${#GPL_GO_DEPS[@]} -gt 0 ]; then
        echo "FAIL: Potential GPL/AGPL Go dependencies found:" >> "${REPORT}"
        for dep in "${GPL_GO_DEPS[@]}"; do
            echo "  - ${dep}" >> "${REPORT}"
        done
        FAIL=1
    else
        echo "PASS: No GPL/AGPL patterns detected in go.mod dependencies" >> "${REPORT}"
    fi
else
    echo "SKIP: go.mod not found" >> "${REPORT}"
fi
echo "" >> "${REPORT}"

# -------------------------------------------------------------------
# 3. Check Cargo.toml dependencies for GPL/AGPL licenses
# -------------------------------------------------------------------
echo "--- Cargo.toml Dependency License Check ---" >> "${REPORT}"

CARGO_GPL_FOUND=0
while IFS= read -r -d '' cargo_toml; do
    rel_path="${cargo_toml#${REPO_ROOT}/}"
    # Check the license field in Cargo.toml for GPL/AGPL
    LICENSE_FIELD=$(grep -iE '^\s*license\s*=' "${cargo_toml}" | head -1 || true)
    if echo "${LICENSE_FIELD}" | grep -qiE '(GPL|AGPL)'; then
        echo "FAIL: GPL/AGPL license in ${rel_path}: ${LICENSE_FIELD}" >> "${REPORT}"
        CARGO_GPL_FOUND=1
        FAIL=1
    fi

    # If cargo-license is available, do a deeper check
    CARGO_DIR=$(dirname "${cargo_toml}")
    if command -v cargo-license &>/dev/null && [ -f "${CARGO_DIR}/Cargo.lock" ]; then
        CARGO_GPL=$(cd "${CARGO_DIR}" && cargo license 2>/dev/null | grep -iE 'GPL|AGPL' || true)
        if [ -n "${CARGO_GPL}" ]; then
            echo "FAIL: GPL/AGPL dependency in ${rel_path}:" >> "${REPORT}"
            echo "  ${CARGO_GPL}" >> "${REPORT}"
            CARGO_GPL_FOUND=1
            FAIL=1
        fi
    fi
done < <(find "${REPO_ROOT}" -name 'Cargo.toml' -not -path '*/target/*' -print0)

if [ "${CARGO_GPL_FOUND}" -eq 0 ]; then
    echo "PASS: No GPL/AGPL patterns detected in Cargo.toml files" >> "${REPORT}"
fi
echo "" >> "${REPORT}"

# -------------------------------------------------------------------
# Summary
# -------------------------------------------------------------------
echo "========================================" >> "${REPORT}"
if [ "${FAIL}" -eq 0 ]; then
    echo "  RESULT: PASS" >> "${REPORT}"
    echo "  No GPL/AGPL contamination detected" >> "${REPORT}"
else
    echo "  RESULT: FAIL" >> "${REPORT}"
    echo "  GPL/AGPL contamination detected!" >> "${REPORT}"
fi
echo "========================================" >> "${REPORT}"

# Print report to stdout
cat "${REPORT}"

exit "${FAIL}"
