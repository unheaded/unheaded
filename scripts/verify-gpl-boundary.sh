#!/bin/bash
# SPDX-License-Identifier: MIT
# verify-gpl-boundary.sh - Scan project for GPL/AGPL license contamination
# Part of Unheaded S77 Phase 3: SBOM + CI/CD Fortress
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RESULTS_DIR="${REPO_ROOT}/sbom-results"
REPORT="${RESULTS_DIR}/gpl-boundary-report.txt"

mkdir -p "${RESULTS_DIR}"

# First-party Cargo manifests — these are intentionally GPL by project license,
# so their `license = "GPL-..."` field is correct and must NOT be flagged as
# third-party contamination. Match by repo-relative path glob via case.
is_first_party_cargo() {
    local rel="$1"
    case "${rel}" in
        crates/*/Cargo.toml) return 0 ;;
        ebpf/Cargo.toml|ebpf/*/Cargo.toml) return 0 ;;
        cmd/ebpf-collector/Cargo.toml|cmd/ebpf-collector/*/Cargo.toml) return 0 ;;
        cmd/ebpf-loader/Cargo.toml) return 0 ;;
        cmd/trace-collector/Cargo.toml) return 0 ;;
    esac
    return 1
}

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
    # Portable SPDX header extraction: avoid grep -oP \K (PCRE-only, BSD grep
    # silently fails). Use awk to read the first 5 lines and pull the token
    # after `SPDX-License-Identifier:` from the first matching line.
    HEADER=$(awk 'NR<=5 && /SPDX-License-Identifier:/{
        sub(/.*SPDX-License-Identifier:[[:space:]]*/, "");
        sub(/[[:space:]].*/, "");
        print; exit
    }' "$f")
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

# Project is GPL-3.0-or-later — first-party Go files SHOULD carry GPL SPDX
# headers. Only AGPL is true contamination here (different obligations than
# our chosen license). Missing headers are tracked separately as a hygiene
# signal (S37 / SPDX coverage), not a contamination FAIL.
if [ "${AGPL_COUNT}" -gt 0 ]; then
    echo "FAIL: ${AGPL_COUNT} AGPL SPDX header(s) found in Go source files (license escalation)" >> "${REPORT}"
    FAIL=1
elif [ "${LGPL_COUNT}" -gt 0 ]; then
    echo "WARN: ${LGPL_COUNT} LGPL SPDX header(s) found in Go source files (different obligations from GPL-3.0+; verify intent)" >> "${REPORT}"
fi
if [ "${GPL_COUNT}" -gt 0 ]; then
    echo "INFO: ${GPL_COUNT} GPL SPDX header(s) found in Go source files (expected — project license is GPL-3.0-or-later)" >> "${REPORT}"
fi
if [ "${NO_HEADER_COUNT}" -gt 0 ]; then
    echo "WARN: ${NO_HEADER_COUNT} Go file(s) without SPDX header (S37 hygiene)" >> "${REPORT}"
fi
if [ "${AGPL_COUNT}" -eq 0 ] && [ "${LGPL_COUNT}" -eq 0 ]; then
    echo "PASS: No AGPL/LGPL contamination in Go source SPDX headers" >> "${REPORT}"
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
FIRST_PARTY_GPL_COUNT=0
while IFS= read -r -d '' cargo_toml; do
    rel_path="${cargo_toml#${REPO_ROOT}/}"
    # Check the license field in Cargo.toml for GPL/AGPL
    LICENSE_FIELD=$(grep -iE '^\s*license\s*=' "${cargo_toml}" | head -1 || true)
    if echo "${LICENSE_FIELD}" | grep -qiE '(GPL|AGPL)'; then
        # First-party crates are intentionally GPL — count them but do NOT
        # flag as contamination. Only third-party Cargo.toml under our tree
        # is suspect (we vendor nothing today, so any new one is a red flag).
        if is_first_party_cargo "${rel_path}"; then
            FIRST_PARTY_GPL_COUNT=$((FIRST_PARTY_GPL_COUNT + 1))
        else
            echo "FAIL: GPL/AGPL license in non-first-party ${rel_path}: ${LICENSE_FIELD}" >> "${REPORT}"
            CARGO_GPL_FOUND=1
            FAIL=1
        fi
    fi

    # If cargo-license is available, do a deeper check (this scans
    # dependencies via Cargo.lock — the actually meaningful contamination
    # signal — and applies to first-party crates too).
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

if [ "${FIRST_PARTY_GPL_COUNT}" -gt 0 ]; then
    echo "INFO: ${FIRST_PARTY_GPL_COUNT} first-party Cargo manifests are GPL-licensed (intentional, project license)" >> "${REPORT}"
fi

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
