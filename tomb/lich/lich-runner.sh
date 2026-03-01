#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
#
# LICH Framework — Master Fuzzing Orchestrator
#
# Compiles all Go fuzz harnesses, launches parallel fuzzing campaigns,
# monitors for crashes, and generates a summary report.
#
# Usage:
#   ./lich-runner.sh                    # Run all harnesses (60s each)
#   ./lich-runner.sh --time 300s        # Run all harnesses (5 min each)
#   ./lich-runner.sh --harness 001      # Run specific harness
#   ./lich-runner.sh --parallel 4       # Max parallel fuzzers
#   ./lich-runner.sh --report-only      # Just generate report from existing results

set -euo pipefail

# ============================================================================
# Configuration
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
HARNESS_DIR="${SCRIPT_DIR}/harnesses"
SEEDS_DIR="${SCRIPT_DIR}/seeds"
CONFIG_FILE="${SCRIPT_DIR}/config/lich.yaml"

# Defaults (overridden by config/flags)
FUZZ_TIME="${FUZZ_TIME:-60s}"
MAX_PARALLEL="${MAX_PARALLEL:-2}"
OUTPUT_DIR="${SCRIPT_DIR}/results/$(date +%Y%m%d_%H%M%S)"
LOG_DIR="${OUTPUT_DIR}/logs"
CRASH_DIR="${OUTPUT_DIR}/crashes"
SPECIFIC_HARNESS=""
REPORT_ONLY=false

# ANSI colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# ============================================================================
# Argument Parsing
# ============================================================================

while [[ $# -gt 0 ]]; do
    case $1 in
        --time)
            FUZZ_TIME="$2"
            shift 2
            ;;
        --parallel)
            MAX_PARALLEL="$2"
            shift 2
            ;;
        --harness)
            SPECIFIC_HARNESS="$2"
            shift 2
            ;;
        --report-only)
            REPORT_ONLY=true
            shift
            ;;
        --output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --help|-h)
            echo "LICH Framework — Master Fuzzing Orchestrator"
            echo ""
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --time DURATION     Fuzz time per harness (default: 60s)"
            echo "  --parallel N        Max parallel fuzzers (default: 2)"
            echo "  --harness NNN       Run specific harness (e.g., 001, 007)"
            echo "  --output DIR        Output directory"
            echo "  --report-only       Generate report from existing results"
            echo "  --help              Show this help"
            echo ""
            echo "Harnesses:"
            echo "  LICH-001  Monad wire format"
            echo "  LICH-002  Monad encoder/decoder"
            echo "  LICH-003  CRC-16 collision hunting"
            echo "  LICH-004  MBC bytecode parser"
            echo "  LICH-005  eBPF kernel interface boundaries"
            echo "  LICH-006  Monad state machine transitions"
            echo "  LICH-007  MBC eBPF program execution"
            echo "  LICH-008  Wotan ring buffer/cache operations"
            echo "  LICH-009  Flow ID collision detection"
            echo "  LICH-010  WAL integrity verification"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# ============================================================================
# Harness Registry
# ============================================================================

# Each entry: "LICH_ID:FUZZ_FUNC_NAME:DESCRIPTION"
declare -a HARNESSES=(
    "001:FuzzMonadWireParse:Monad wire format parsing"
    "001:FuzzMonadWireRoundtrip:Monad wire format roundtrip"
    "002:FuzzMonadCodecVarintStress:Varint codec stress"
    "002:FuzzMonadCodecExponentOverflow:Exponent overflow hunting"
    "002:FuzzMonadCodecTLVChain:TLV chain encode/decode"
    "002:FuzzMonadCodecTLVSequenceStress:TLV sequence stress"
    "002:FuzzMonadCodecCombined:Combined codec pipeline"
    "003:FuzzCRC16CollisionPair:CRC-16 collision pair search"
    "003:FuzzCRC16StealthBitFlip:CRC-16 stealth bit flip"
    "003:FuzzCRC16MonadCollision:CRC-16 Monad header collision"
    "003:FuzzCRC16LengthExtension:CRC-16 length extension"
    "003:FuzzCRC32MPEG2Collision:CRC-32 collision search"
    "004:FuzzMBCParsing:MBC bytecode parsing"
    "004:FuzzMBCValidation:MBC program validation"
    "005:FuzzBPFMapCreateAttr:BPF map create attributes"
    "005:FuzzELFSectionClassifier:ELF section classification"
    "005:FuzzBPFMapKeyValueSerialization:BPF map key/value"
    "006:FuzzStateMachineTransitions:State machine transitions"
    "006:FuzzGoawayMonotonicity:GOAWAY monotonicity"
    "006:FuzzStateMachineGoawayIntegration:State machine + GOAWAY"
    "007:FuzzMBCExecution:MBC VM execution"
    "007:FuzzMBCArithmeticEdgeCases:MBC arithmetic edge cases"
    "008:FuzzCacheRingBufferPush:Ring buffer push"
    "008:FuzzCacheRingBufferQuery:Ring buffer query"
    "008:FuzzCacheRingBufferInterleaved:Ring buffer interleaved ops"
    "008:FuzzCacheRingBufferCapacityBoundary:Ring buffer capacity boundary"
    "009:FuzzFlowIDCollision:Flow ID collision detection"
    "009:FuzzIPv6FlowLabelRoundtrip:IPv6 flow label roundtrip"
    "009:FuzzFlowLabelHashDistribution:Flow label hash distribution"
    "009:FuzzFlowSequenceMonotonicity:Flow sequence monotonicity"
    "010:FuzzWALEntryRoundtrip:WAL entry roundtrip"
    "010:FuzzWALEntryCorruption:WAL corruption detection"
    "010:FuzzWALSequenceGaps:WAL sequence gap detection"
    "010:FuzzWALCRC32Collision:WAL CRC-32 collision"
    "010:FuzzWALEntryChain:WAL entry chain parsing"
)

# ============================================================================
# Functions
# ============================================================================

log_info() {
    echo -e "${CYAN}[LICH]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[LICH]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[LICH]${NC} $1"
}

log_error() {
    echo -e "${RED}[LICH]${NC} $1"
}

setup_directories() {
    mkdir -p "${OUTPUT_DIR}" "${LOG_DIR}" "${CRASH_DIR}"
    log_info "Output directory: ${OUTPUT_DIR}"
}

verify_go() {
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed. Install Go 1.21+ to run fuzz harnesses."
        exit 1
    fi

    GO_VERSION=$(go version | grep -oP 'go\d+\.\d+')
    log_info "Go version: ${GO_VERSION}"
}

compile_check() {
    log_info "Verifying harness compilation..."
    cd "${REPO_ROOT}"

    if go build ./tomb/lich/harnesses/ 2>"${LOG_DIR}/compile.log"; then
        log_success "All harnesses compile successfully"
    else
        log_error "Compilation failed. Check ${LOG_DIR}/compile.log"
        cat "${LOG_DIR}/compile.log"
        exit 1
    fi
}

run_fuzz_harness() {
    local lich_id="$1"
    local fuzz_func="$2"
    local description="$3"
    local log_file="${LOG_DIR}/lich_${lich_id}_${fuzz_func}.log"
    local start_time=$(date +%s)

    log_info "Starting LICH-${lich_id}: ${fuzz_func} (${description}) [${FUZZ_TIME}]"

    cd "${REPO_ROOT}"

    local exit_code=0
    go test -fuzz="${fuzz_func}" \
        -fuzztime="${FUZZ_TIME}" \
        -count=1 \
        ./tomb/lich/harnesses/ \
        > "${log_file}" 2>&1 || exit_code=$?

    local end_time=$(date +%s)
    local duration=$((end_time - start_time))

    if [[ ${exit_code} -eq 0 ]]; then
        log_success "LICH-${lich_id}: ${fuzz_func} completed (${duration}s) -- no crashes"
    else
        log_error "LICH-${lich_id}: ${fuzz_func} FOUND CRASH (${duration}s) -- see ${log_file}"

        # Copy crash files to crash directory
        local testdata_dir="${REPO_ROOT}/tomb/lich/harnesses/testdata/fuzz/${fuzz_func}"
        if [[ -d "${testdata_dir}" ]]; then
            cp -r "${testdata_dir}" "${CRASH_DIR}/lich_${lich_id}_${fuzz_func}/"
        fi
    fi

    # Return exit code for summary
    echo "${exit_code}" > "${LOG_DIR}/lich_${lich_id}_${fuzz_func}.exitcode"
}

run_all_harnesses() {
    local active_jobs=0
    local total=0
    local started=0

    for entry in "${HARNESSES[@]}"; do
        IFS=':' read -r lich_id fuzz_func description <<< "${entry}"

        # Filter by specific harness if requested
        if [[ -n "${SPECIFIC_HARNESS}" && "${lich_id}" != "${SPECIFIC_HARNESS}" ]]; then
            continue
        fi

        total=$((total + 1))
    done

    log_info "Running ${total} fuzz targets with --parallel=${MAX_PARALLEL} --time=${FUZZ_TIME}"
    echo ""

    for entry in "${HARNESSES[@]}"; do
        IFS=':' read -r lich_id fuzz_func description <<< "${entry}"

        if [[ -n "${SPECIFIC_HARNESS}" && "${lich_id}" != "${SPECIFIC_HARNESS}" ]]; then
            continue
        fi

        # Wait if at max parallel jobs
        while [[ $(jobs -r -p | wc -l) -ge ${MAX_PARALLEL} ]]; do
            sleep 1
        done

        run_fuzz_harness "${lich_id}" "${fuzz_func}" "${description}" &
        started=$((started + 1))
    done

    # Wait for all jobs to complete
    wait
    echo ""
    log_info "All ${started} fuzz targets completed"
}

generate_report() {
    local report_file="${OUTPUT_DIR}/LICH-REPORT.txt"

    log_info "Generating report: ${report_file}"

    {
        echo "================================================================="
        echo "  LICH Adversary Framework — Fuzzing Campaign Report"
        echo "================================================================="
        echo ""
        echo "Date:       $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
        echo "Duration:   ${FUZZ_TIME} per target"
        echo "Parallel:   ${MAX_PARALLEL}"
        echo "Go Version: $(go version 2>/dev/null || echo 'unknown')"
        echo ""
        echo "-----------------------------------------------------------------"
        echo "  Results Summary"
        echo "-----------------------------------------------------------------"
        echo ""

        local pass=0
        local fail=0
        local skip=0

        for entry in "${HARNESSES[@]}"; do
            IFS=':' read -r lich_id fuzz_func description <<< "${entry}"

            if [[ -n "${SPECIFIC_HARNESS}" && "${lich_id}" != "${SPECIFIC_HARNESS}" ]]; then
                skip=$((skip + 1))
                continue
            fi

            local exitcode_file="${LOG_DIR}/lich_${lich_id}_${fuzz_func}.exitcode"
            if [[ -f "${exitcode_file}" ]]; then
                local code=$(cat "${exitcode_file}")
                if [[ "${code}" == "0" ]]; then
                    printf "  [PASS] LICH-%s %-40s %s\n" "${lich_id}" "${fuzz_func}" "${description}"
                    pass=$((pass + 1))
                else
                    printf "  [FAIL] LICH-%s %-40s %s\n" "${lich_id}" "${fuzz_func}" "${description}"
                    fail=$((fail + 1))
                fi
            else
                printf "  [SKIP] LICH-%s %-40s %s\n" "${lich_id}" "${fuzz_func}" "${description}"
                skip=$((skip + 1))
            fi
        done

        echo ""
        echo "-----------------------------------------------------------------"
        echo "  Totals: ${pass} passed, ${fail} failed, ${skip} skipped"
        echo "-----------------------------------------------------------------"

        # List crash files if any
        if [[ -d "${CRASH_DIR}" ]] && [[ "$(ls -A "${CRASH_DIR}" 2>/dev/null)" ]]; then
            echo ""
            echo "-----------------------------------------------------------------"
            echo "  Crash Files"
            echo "-----------------------------------------------------------------"
            echo ""
            find "${CRASH_DIR}" -type f | sort | while read -r crash_file; do
                echo "  ${crash_file}"
            done
        fi

        echo ""
        echo "-----------------------------------------------------------------"
        echo "  Log Files"
        echo "-----------------------------------------------------------------"
        echo ""
        if [[ -d "${LOG_DIR}" ]]; then
            ls -la "${LOG_DIR}"/*.log 2>/dev/null | while read -r line; do
                echo "  ${line}"
            done
        fi

        echo ""
        echo "================================================================="
        echo "  END OF REPORT"
        echo "================================================================="
    } > "${report_file}"

    cat "${report_file}"
}

# ============================================================================
# Main
# ============================================================================

main() {
    echo ""
    echo "  +-----------------------------------------+"
    echo "  |   LICH Adversary Framework v1.0          |"
    echo "  |   Tomb of Knowledge — Security Fuzzing   |"
    echo "  +-----------------------------------------+"
    echo ""

    if [[ "${REPORT_ONLY}" == "true" ]]; then
        if [[ -z "${OUTPUT_DIR}" ]]; then
            log_error "No output directory specified for --report-only"
            exit 1
        fi
        generate_report
        exit 0
    fi

    verify_go
    setup_directories
    compile_check
    run_all_harnesses
    generate_report

    echo ""
    log_info "Campaign complete. Results: ${OUTPUT_DIR}"
}

main "$@"
