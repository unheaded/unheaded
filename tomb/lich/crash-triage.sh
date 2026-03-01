#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
#
# LICH Framework — Crash Severity Categorizer
#
# Analyzes crash files from LICH fuzzing campaigns, categorizes them
# by severity, and generates a triage report for the security team.
#
# Severity Levels (aligned with Kingdom consensus model):
#   CRITICAL  - Memory safety violation (panic, nil deref, buffer overflow)
#   HIGH      - Logic error that can be exploited (CRC bypass, state corruption)
#   MEDIUM    - Correctness issue (roundtrip failure, assertion violation)
#   LOW       - Performance issue or cosmetic (slow input, non-exploitable)
#   INFO      - Expected finding (CRC-16 collision by design, overflow rejection)
#
# Usage:
#   ./crash-triage.sh                          # Triage latest results
#   ./crash-triage.sh results/20260228_120000  # Triage specific run

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${1:-}"

# Colors
RED='\033[0;31m'
ORANGE='\033[0;33m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
GREEN='\033[0;32m'
GRAY='\033[0;90m'
NC='\033[0m'

# ============================================================================
# Find results directory
# ============================================================================

if [[ -z "${RESULTS_DIR}" ]]; then
    # Find latest results
    LATEST=$(ls -td "${SCRIPT_DIR}"/results/*/ 2>/dev/null | head -1)
    if [[ -z "${LATEST}" ]]; then
        echo "No results found in ${SCRIPT_DIR}/results/"
        echo "Run lich-runner.sh first, then re-run this script."
        exit 1
    fi
    RESULTS_DIR="${LATEST}"
fi

if [[ ! -d "${RESULTS_DIR}" ]]; then
    echo "Results directory not found: ${RESULTS_DIR}"
    exit 1
fi

CRASH_DIR="${RESULTS_DIR}/crashes"
LOG_DIR="${RESULTS_DIR}/logs"
TRIAGE_FILE="${RESULTS_DIR}/TRIAGE-REPORT.txt"

echo ""
echo "  +-------------------------------------------+"
echo "  |   LICH Crash Triage — Severity Analysis    |"
echo "  +-------------------------------------------+"
echo ""
echo "Results: ${RESULTS_DIR}"
echo ""

# ============================================================================
# Severity Classification Functions
# ============================================================================

classify_crash() {
    local log_file="$1"
    local crash_file="$2"
    local harness_id="$3"

    # Read log for crash indicators
    local log_content=""
    if [[ -f "${log_file}" ]]; then
        log_content=$(cat "${log_file}" 2>/dev/null || true)
    fi

    # CRITICAL: Memory safety violations
    if echo "${log_content}" | grep -qiE "panic:|runtime error:|nil pointer dereference|index out of range|slice bounds|invalid memory|segfault|SIGSEGV"; then
        echo "CRITICAL"
        return
    fi

    # CRITICAL: Buffer overflow indicators
    if echo "${log_content}" | grep -qiE "buffer overflow|stack overflow|heap corruption|out of bounds"; then
        echo "CRITICAL"
        return
    fi

    # HIGH: Logic errors that bypass safety checks
    if echo "${log_content}" | grep -qiE "CRC mismatch after successful|accepted unknown version|should error|bypass|escalation"; then
        echo "HIGH"
        return
    fi

    # HIGH: State corruption
    if echo "${log_content}" | grep -qiE "state corruption|invalid state|monotonicity violation.*false|roundtrip.*mismatch.*fatal"; then
        echo "HIGH"
        return
    fi

    # MEDIUM: Correctness failures
    if echo "${log_content}" | grep -qiE "roundtrip.*mismatch|assertion.*failed|consumed.*bytes.*want|length.*mismatch"; then
        echo "MEDIUM"
        return
    fi

    # LOW: Non-critical issues
    if echo "${log_content}" | grep -qiE "timeout|exceeded.*max|slow"; then
        echo "LOW"
        return
    fi

    # INFO: Expected findings (CRC collisions, overflow rejections)
    if echo "${log_content}" | grep -qiE "COLLISION|STEALTH|LENGTH-EXTENSION|overflow.*expected"; then
        echo "INFO"
        return
    fi

    # Default: unclassified crash
    echo "MEDIUM"
}

severity_color() {
    case "$1" in
        CRITICAL) echo "${RED}" ;;
        HIGH)     echo "${ORANGE}" ;;
        MEDIUM)   echo "${YELLOW}" ;;
        LOW)      echo "${CYAN}" ;;
        INFO)     echo "${GRAY}" ;;
        *)        echo "${NC}" ;;
    esac
}

severity_icon() {
    case "$1" in
        CRITICAL) echo "[!!!]" ;;
        HIGH)     echo "[!! ]" ;;
        MEDIUM)   echo "[!  ]" ;;
        LOW)      echo "[.  ]" ;;
        INFO)     echo "[i  ]" ;;
        *)        echo "[?  ]" ;;
    esac
}

# ============================================================================
# Triage
# ============================================================================

declare -A SEVERITY_COUNTS
SEVERITY_COUNTS[CRITICAL]=0
SEVERITY_COUNTS[HIGH]=0
SEVERITY_COUNTS[MEDIUM]=0
SEVERITY_COUNTS[LOW]=0
SEVERITY_COUNTS[INFO]=0

declare -a FINDINGS=()

# Analyze crash files
if [[ -d "${CRASH_DIR}" ]] && [[ "$(ls -A "${CRASH_DIR}" 2>/dev/null)" ]]; then
    echo "Analyzing crash files..."
    echo ""

    find "${CRASH_DIR}" -type f | sort | while read -r crash_file; do
        # Extract harness info from path
        dirname=$(basename "$(dirname "${crash_file}")")
        harness_id=$(echo "${dirname}" | grep -oP 'lich_\d{3}' || echo "unknown")
        fuzz_func=$(echo "${dirname}" | sed "s/lich_[0-9]*_//" || echo "unknown")

        # Find corresponding log
        log_file="${LOG_DIR}/${dirname}.log"

        severity=$(classify_crash "${log_file}" "${crash_file}" "${harness_id}")
        color=$(severity_color "${severity}")
        icon=$(severity_icon "${severity}")

        echo -e "  ${color}${icon} ${severity}${NC}  ${harness_id}/${fuzz_func}"
        echo "         Crash: ${crash_file}"

        SEVERITY_COUNTS[${severity}]=$((${SEVERITY_COUNTS[${severity}]} + 1))
    done
else
    echo "No crash files found."
fi

echo ""

# Analyze log files for non-crash findings
echo "Analyzing log files for findings..."
echo ""

if [[ -d "${LOG_DIR}" ]]; then
    for log_file in "${LOG_DIR}"/*.log; do
        [[ -f "${log_file}" ]] || continue

        basename=$(basename "${log_file}" .log)
        exitcode_file="${LOG_DIR}/${basename}.exitcode"

        # Check exit code
        if [[ -f "${exitcode_file}" ]]; then
            code=$(cat "${exitcode_file}")
            if [[ "${code}" != "0" ]]; then
                severity=$(classify_crash "${log_file}" "" "${basename}")
                color=$(severity_color "${severity}")
                icon=$(severity_icon "${severity}")

                echo -e "  ${color}${icon} ${severity}${NC}  ${basename}"

                # Extract first error line
                first_error=$(grep -m1 -iE "FAIL|Error|panic|mismatch" "${log_file}" 2>/dev/null || echo "(see log)")
                echo "         ${first_error}"

                SEVERITY_COUNTS[${severity}]=$((${SEVERITY_COUNTS[${severity}]} + 1))
            fi
        fi

        # Check for interesting findings (even in passing tests)
        collision_count=$(grep -c "COLLISION" "${log_file}" 2>/dev/null || true)
        stealth_count=$(grep -c "STEALTH" "${log_file}" 2>/dev/null || true)

        if [[ "${collision_count}" -gt 0 ]]; then
            echo -e "  ${GRAY}[i  ] INFO${NC}  ${basename}: ${collision_count} CRC collision(s) found"
            SEVERITY_COUNTS[INFO]=$((${SEVERITY_COUNTS[INFO]} + collision_count))
        fi
        if [[ "${stealth_count}" -gt 0 ]]; then
            echo -e "  ${GRAY}[i  ] INFO${NC}  ${basename}: ${stealth_count} stealth modification(s) found"
            SEVERITY_COUNTS[INFO]=$((${SEVERITY_COUNTS[INFO]} + stealth_count))
        fi
    done
fi

# ============================================================================
# Generate Triage Report
# ============================================================================

{
    echo "================================================================="
    echo "  LICH Adversary Framework — Crash Triage Report"
    echo "================================================================="
    echo ""
    echo "Date:        $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
    echo "Results Dir: ${RESULTS_DIR}"
    echo ""
    echo "-----------------------------------------------------------------"
    echo "  Severity Distribution"
    echo "-----------------------------------------------------------------"
    echo ""
    printf "  CRITICAL: %d\n" "${SEVERITY_COUNTS[CRITICAL]}"
    printf "  HIGH:     %d\n" "${SEVERITY_COUNTS[HIGH]}"
    printf "  MEDIUM:   %d\n" "${SEVERITY_COUNTS[MEDIUM]}"
    printf "  LOW:      %d\n" "${SEVERITY_COUNTS[LOW]}"
    printf "  INFO:     %d\n" "${SEVERITY_COUNTS[INFO]}"
    echo ""

    total=$((SEVERITY_COUNTS[CRITICAL] + SEVERITY_COUNTS[HIGH] + SEVERITY_COUNTS[MEDIUM] + SEVERITY_COUNTS[LOW] + SEVERITY_COUNTS[INFO]))
    echo "  Total findings: ${total}"
    echo ""

    # Risk assessment
    echo "-----------------------------------------------------------------"
    echo "  Risk Assessment"
    echo "-----------------------------------------------------------------"
    echo ""
    if [[ ${SEVERITY_COUNTS[CRITICAL]} -gt 0 ]]; then
        echo "  RISK: BLOCK RELEASE"
        echo "  ${SEVERITY_COUNTS[CRITICAL]} critical finding(s) require immediate remediation."
        echo "  Memory safety violations must be fixed before any deployment."
    elif [[ ${SEVERITY_COUNTS[HIGH]} -gt 0 ]]; then
        echo "  RISK: REVIEW REQUIRED"
        echo "  ${SEVERITY_COUNTS[HIGH]} high-severity finding(s) need security review."
        echo "  These may be exploitable depending on deployment context."
    elif [[ ${SEVERITY_COUNTS[MEDIUM]} -gt 0 ]]; then
        echo "  RISK: ACCEPTABLE WITH TRACKING"
        echo "  ${SEVERITY_COUNTS[MEDIUM]} medium-severity finding(s) should be tracked."
        echo "  Correctness issues -- fix in next sprint."
    else
        echo "  RISK: LOW"
        echo "  No critical, high, or medium findings."
        echo "  Protocol hardening is effective."
    fi

    echo ""
    echo "-----------------------------------------------------------------"
    echo "  Recommendations"
    echo "-----------------------------------------------------------------"
    echo ""
    echo "  1. Review all CRITICAL findings immediately"
    echo "  2. File GitHub issues for HIGH findings"
    echo "  3. Track MEDIUM findings in sprint backlog"
    echo "  4. CRC-16 collisions (INFO) are expected -- HMAC required for auth"
    echo "  5. Re-run with longer --time for deeper coverage"
    echo ""
    echo "================================================================="
    echo "  END OF TRIAGE REPORT"
    echo "================================================================="
} > "${TRIAGE_FILE}"

echo ""
echo "-----------------------------------------------------------------"
echo "  Summary"
echo "-----------------------------------------------------------------"
echo ""
printf "  ${RED}CRITICAL:${NC} %d\n" "${SEVERITY_COUNTS[CRITICAL]}"
printf "  ${ORANGE}HIGH:${NC}     %d\n" "${SEVERITY_COUNTS[HIGH]}"
printf "  ${YELLOW}MEDIUM:${NC}   %d\n" "${SEVERITY_COUNTS[MEDIUM]}"
printf "  ${CYAN}LOW:${NC}      %d\n" "${SEVERITY_COUNTS[LOW]}"
printf "  ${GRAY}INFO:${NC}     %d\n" "${SEVERITY_COUNTS[INFO]}"
echo ""
echo "Triage report: ${TRIAGE_FILE}"
echo ""
