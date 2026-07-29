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
        # ASCEND-LINUX boot tooling per references/battle-plan-ascend-linux-2026-05-08.md
        cmd/upc-bootctl/Cargo.toml) return 0 ;;
        cmd/waf/Cargo.toml) return 0 ;;
    esac
    return 1
}

# First-party crate NAMES, as they appear in `cargo license` output. Every
# crate in this repo is GPL by project license, so `cargo license` reports our
# OWN crates as copyleft. That is the project license working as intended, not
# third-party contamination — the dependency scan below must skip them.
FIRST_PARTY_CRATES="$(
    find "${REPO_ROOT}" -name 'Cargo.toml' -not -path '*/target/*' -print0 \
    | xargs -0 -n1 awk '
        /^\[package\]/ { in_pkg = 1; next }
        /^\[/          { in_pkg = 0 }
        in_pkg && /^[[:space:]]*name[[:space:]]*=/ {
            sub(/.*=[[:space:]]*"?/, ""); sub(/".*/, ""); print; exit
        }' \
    | sort -u
)"

is_first_party_crate() {
    printf '%s\n' "${FIRST_PARTY_CRATES}" | grep -qxF "$1"
}

# Classify an SPDX license expression into PERMISSIVE / GPL / LGPL / AGPL.
#
# A dual license offering a permissive alternative — e.g. r-efi's
# "Apache-2.0 OR LGPL-2.1-or-later OR MIT" — imposes no copyleft obligation on
# us, because we elect the permissive branch. Only when EVERY alternative is
# copyleft does the obligation actually bind. A plain substring grep for "GPL"
# gets this wrong in both directions, which is what this function exists to fix.
spdx_class() {
    local expr="$1" rest operand
    if printf '%s' "${expr}" | grep -q ' OR '; then
        rest="${expr}"
        while [ -n "${rest}" ]; do
            operand="${rest%% OR *}"
            if [ "${operand}" = "${rest}" ]; then
                rest=""
            else
                rest="${rest#* OR }"
            fi
            operand="$(printf '%s' "${operand}" | tr -d '()' | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
            case "${operand}" in
                MIT|MIT-0|ISC|Zlib|0BSD|BSD-2-Clause|BSD-3-Clause|Unlicense|CC0-1.0|Apache-2.0*)
                    echo "PERMISSIVE"; return ;;
            esac
        done
    fi
    case "${expr}" in
        *AGPL*) echo "AGPL" ;;
        *LGPL*) echo "LGPL" ;;
        # GPL-2.0-only is ONE-WAY INCOMPATIBLE with this project.
        # The Kingdom is GPL-3.0-or-later. A GPL-2.0-*or-later* dependency is
        # fine — we elect 3.0. A GPL-2.0-*only* dependency cannot be combined
        # with GPL-3.0 code at all, and we cannot downgrade to satisfy it.
        # Bare "GPL-2.0" is treated as -only: SPDX deprecated the ambiguous
        # form, and assuming the permissive reading of an ambiguous grant is
        # exactly the assumption that loses.
        # First-party crates (monad-common, ebpf/*) are GPL-2.0 because the
        # kernel requires it, and are skipped before this function is reached.
        *GPL-2.0-or-later*|*GPL-2.0+*) echo "GPL" ;;
        *GPL-2.0*)                     echo "GPL2_ONLY" ;;
        *GPL*)                         echo "GPL" ;;
        *)                             echo "PERMISSIVE" ;;
    esac
}

# Sourcing this script with GPL_BOUNDARY_LIB_ONLY=1 loads the classifier
# helpers above without running the scan, so they can be unit-tested directly.
# See scripts/test-gpl-boundary-classifier.sh.
if [ "${GPL_BOUNDARY_LIB_ONLY:-0}" = "1" ]; then
    return 0 2>/dev/null || exit 0
fi

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
CARGO_GPL_DEP_COUNT=0
CARGO_LGPL_COUNT=0
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

    # If cargo-license is available, do a deeper check over the RESOLVED
    # dependency graph (via Cargo.lock) — the actually meaningful contamination
    # signal. Policy matches the Go scan above: the project is GPL-3.0-or-later,
    # so a GPL dependency is expected and fine; AGPL is an escalation of our
    # obligations and fails; LGPL-only differs enough to warrant a warning.
    CARGO_DIR=$(dirname "${cargo_toml}")
    if command -v cargo-license &>/dev/null && [ -f "${CARGO_DIR}/Cargo.lock" ]; then
        while IFS= read -r dep_line; do
            # `cargo license -d` emits one crate per line: `name: version, "SPDX",`
            dep_name="${dep_line%%:*}"
            [ -n "${dep_name}" ] || continue
            is_first_party_crate "${dep_name}" && continue
            dep_lic="$(printf '%s' "${dep_line}" | sed 's/^[^"]*"//; s/",[[:space:]]*$//')"
            case "$(spdx_class "${dep_lic}")" in
                AGPL)
                    echo "FAIL: AGPL dependency ${dep_name} (${dep_lic}) in ${rel_path}" >> "${REPORT}"
                    CARGO_GPL_FOUND=1
                    FAIL=1
                    ;;
                GPL2_ONLY)
                    echo "FAIL: GPL-2.0-only dependency ${dep_name} (${dep_lic}) in ${rel_path} — incompatible with GPL-3.0-or-later; cannot be combined" >> "${REPORT}"
                    CARGO_GPL_FOUND=1
                    FAIL=1
                    ;;
                LGPL)
                    echo "WARN: LGPL-only dependency ${dep_name} (${dep_lic}) in ${rel_path}" >> "${REPORT}"
                    CARGO_LGPL_COUNT=$((CARGO_LGPL_COUNT + 1))
                    ;;
                GPL)
                    echo "INFO: GPL dependency ${dep_name} (${dep_lic}) in ${rel_path} (compatible — project is GPL-3.0-or-later)" >> "${REPORT}"
                    CARGO_GPL_DEP_COUNT=$((CARGO_GPL_DEP_COUNT + 1))
                    ;;
            esac
        done < <(cd "${CARGO_DIR}" && cargo license -d 2>/dev/null || true)
    fi
done < <(find "${REPO_ROOT}" -name 'Cargo.toml' -not -path '*/target/*' -print0)

if [ "${FIRST_PARTY_GPL_COUNT}" -gt 0 ]; then
    echo "INFO: ${FIRST_PARTY_GPL_COUNT} first-party Cargo manifests are GPL-licensed (intentional, project license)" >> "${REPORT}"
fi

if [ "${CARGO_GPL_DEP_COUNT}" -gt 0 ] || [ "${CARGO_LGPL_COUNT}" -gt 0 ]; then
    echo "INFO: third-party copyleft deps — GPL: ${CARGO_GPL_DEP_COUNT}, LGPL-only: ${CARGO_LGPL_COUNT}" >> "${REPORT}"
fi

if [ "${CARGO_GPL_FOUND}" -eq 0 ]; then
    echo "PASS: No AGPL contamination in Cargo dependency graphs" >> "${REPORT}"
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
