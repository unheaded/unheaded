#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# fetch-mitre-data.sh — Download MITRE ATT&CK Enterprise JSON data
#
# Run this on an internet-connected host. The output is saved locally
# for air-gap transfer to the Tomb VM.
#
# Downloads:
#   - Enterprise ATT&CK (STIX 2.1 JSON bundle)
#   - ICS ATT&CK (STIX 2.1 JSON bundle)
#   - Mobile ATT&CK (STIX 2.1 JSON bundle)
#
# The MITRE ATT&CK data is published as STIX 2.1 bundles from the
# official MITRE/cti GitHub repository.
#
# Usage:
#   ./fetch-mitre-data.sh [--output-dir ./mitre-attck]
#
# Prerequisites:
#   - curl or wget
#   - jq (optional, for validation)
#   - Internet connectivity

set -euo pipefail

# --- Configuration ---
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}"

# MITRE CTI GitHub raw URLs (STIX 2.1 bundles)
# Source: https://github.com/mitre/cti
MITRE_BASE_URL="https://raw.githubusercontent.com/mitre/cti/master"

declare -A DATASETS=(
    ["enterprise-attack"]="${MITRE_BASE_URL}/enterprise-attack/enterprise-attack.json"
    ["ics-attack"]="${MITRE_BASE_URL}/ics-attack/ics-attack.json"
    ["mobile-attack"]="${MITRE_BASE_URL}/mobile-attack/mobile-attack.json"
)

# MITRE ATT&CK STIX relationship mapping URL
MITRE_ATTACK_URL="https://attack.mitre.org"

# --- Colors ---
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; }

# --- Argument Parsing ---

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --output-dir)
                OUTPUT_DIR="$2"
                shift 2
                ;;
            --help|-h)
                echo "Usage: $0 [--output-dir ./mitre-attck]"
                echo ""
                echo "Downloads MITRE ATT&CK STIX 2.1 JSON bundles for offline use."
                echo ""
                echo "Options:"
                echo "  --output-dir   Where to save downloaded data (default: script directory)"
                exit 0
                ;;
            *)
                echo "Unknown option: $1" >&2
                exit 1
                ;;
        esac
    done
}

# --- Download Function ---

download_file() {
    local name="$1"
    local url="$2"
    local output_file="${OUTPUT_DIR}/${name}.json"

    info "Downloading ${name}..."
    info "  URL: ${url}"
    info "  Dest: ${output_file}"

    if command -v curl &>/dev/null; then
        if curl -fSL --connect-timeout 30 --max-time 300 -o "${output_file}" "${url}"; then
            local size
            size=$(du -h "${output_file}" | cut -f1)
            info "  Downloaded: ${size}"
        else
            error "  Failed to download ${name}"
            return 1
        fi
    elif command -v wget &>/dev/null; then
        if wget -q --timeout=30 -O "${output_file}" "${url}"; then
            local size
            size=$(du -h "${output_file}" | cut -f1)
            info "  Downloaded: ${size}"
        else
            error "  Failed to download ${name}"
            return 1
        fi
    else
        error "Neither curl nor wget found. Install one of them."
        return 1
    fi
}

# --- Validation ---

validate_stix_bundle() {
    local file="$1"
    local name="$2"

    if [[ ! -f "${file}" ]]; then
        error "File not found: ${file}"
        return 1
    fi

    # Basic JSON validation
    if command -v jq &>/dev/null; then
        if jq -e '.type == "bundle"' "${file}" >/dev/null 2>&1; then
            local object_count
            object_count=$(jq '.objects | length' "${file}" 2>/dev/null || echo "unknown")
            info "  Validated ${name}: STIX bundle with ${object_count} objects"
        else
            warn "  ${name}: JSON is valid but may not be a STIX bundle"
        fi
    else
        # Without jq, just check it is valid JSON (python fallback)
        if python3 -c "import json; json.load(open('${file}'))" 2>/dev/null; then
            info "  ${name}: Valid JSON (install jq for full validation)"
        else
            error "  ${name}: Invalid JSON"
            return 1
        fi
    fi
}

# --- Extract Technique Summary ---

extract_technique_summary() {
    local input_file="${OUTPUT_DIR}/enterprise-attack.json"
    local output_file="${OUTPUT_DIR}/enterprise-techniques-summary.json"

    if [[ ! -f "${input_file}" ]]; then
        warn "enterprise-attack.json not found, skipping technique summary extraction"
        return 0
    fi

    if ! command -v jq &>/dev/null; then
        warn "jq not found, skipping technique summary extraction"
        return 0
    fi

    info "Extracting technique summary from Enterprise ATT&CK..."

    jq '[
        .objects[]
        | select(.type == "attack-pattern")
        | {
            id: .id,
            name: .name,
            description: (.description // "No description"),
            external_id: (
                .external_references[]?
                | select(.source_name == "mitre-attack")
                | .external_id
            ),
            kill_chain_phases: [
                .kill_chain_phases[]?
                | .phase_name
            ],
            platforms: (.x_mitre_platforms // []),
            detection: (.x_mitre_detection // "No detection guidance"),
            created: .created,
            modified: .modified
        }
    ] | sort_by(.external_id)' "${input_file}" > "${output_file}" 2>/dev/null

    local technique_count
    technique_count=$(jq 'length' "${output_file}" 2>/dev/null || echo "unknown")
    info "  Extracted ${technique_count} techniques to enterprise-techniques-summary.json"
}

# --- Extract Tactic Summary ---

extract_tactic_summary() {
    local input_file="${OUTPUT_DIR}/enterprise-attack.json"
    local output_file="${OUTPUT_DIR}/enterprise-tactics-summary.json"

    if [[ ! -f "${input_file}" ]]; then
        return 0
    fi

    if ! command -v jq &>/dev/null; then
        return 0
    fi

    info "Extracting tactic summary from Enterprise ATT&CK..."

    jq '[
        .objects[]
        | select(.type == "x-mitre-tactic")
        | {
            id: .id,
            name: .name,
            description: (.description // "No description"),
            external_id: (
                .external_references[]?
                | select(.source_name == "mitre-attack")
                | .external_id
            ),
            shortname: .x_mitre_shortname
        }
    ] | sort_by(.external_id)' "${input_file}" > "${output_file}" 2>/dev/null

    local tactic_count
    tactic_count=$(jq 'length' "${output_file}" 2>/dev/null || echo "unknown")
    info "  Extracted ${tactic_count} tactics to enterprise-tactics-summary.json"
}

# --- Main ---

main() {
    parse_args "$@"

    echo ""
    echo "======================================"
    echo "  MITRE ATT&CK Data Fetcher"
    echo "  Output: ${OUTPUT_DIR}"
    echo "======================================"
    echo ""

    mkdir -p "${OUTPUT_DIR}"

    local failures=0

    # Download all datasets
    for name in "${!DATASETS[@]}"; do
        local url="${DATASETS[${name}]}"
        if ! download_file "${name}" "${url}"; then
            ((failures++))
        fi
    done

    echo ""

    # Validate downloads
    info "Validating downloaded files..."
    for name in "${!DATASETS[@]}"; do
        local file="${OUTPUT_DIR}/${name}.json"
        if [[ -f "${file}" ]]; then
            validate_stix_bundle "${file}" "${name}"
        fi
    done

    echo ""

    # Extract summaries for faster RAG indexing
    extract_technique_summary
    extract_tactic_summary

    # Write metadata
    local meta_file="${OUTPUT_DIR}/fetch-metadata.json"
    {
        echo "{"
        echo "  \"fetched_at\": \"$(date -u '+%Y-%m-%dT%H:%M:%SZ')\","
        echo "  \"source\": \"https://github.com/mitre/cti\","
        echo "  \"format\": \"STIX 2.1\","
        echo "  \"datasets\": ["
        local first=true
        for name in "${!DATASETS[@]}"; do
            local file="${OUTPUT_DIR}/${name}.json"
            if [[ -f "${file}" ]]; then
                local size_bytes
                size_bytes=$(stat --format='%s' "${file}" 2>/dev/null || stat -f '%z' "${file}" 2>/dev/null || echo 0)
                if [[ "${first}" != "true" ]]; then
                    echo ","
                fi
                echo -n "    {\"name\": \"${name}\", \"file\": \"${name}.json\", \"size_bytes\": ${size_bytes}}"
                first=false
            fi
        done
        echo ""
        echo "  ],"
        echo "  \"notes\": \"Download for air-gap transfer to Tomb of Knowledge\""
        echo "}"
    } > "${meta_file}"

    echo ""
    echo "======================================"
    echo "  DOWNLOAD COMPLETE"
    echo "======================================"

    local total_size
    total_size=$(du -sh "${OUTPUT_DIR}" 2>/dev/null | cut -f1)
    echo "  Total size: ${total_size}"
    echo "  Failures:   ${failures}"
    echo ""
    echo "  Files:"
    ls -lh "${OUTPUT_DIR}"/*.json 2>/dev/null | while read -r line; do
        echo "    ${line}"
    done
    echo ""
    echo "  Next steps:"
    echo "    1. Transfer ${OUTPUT_DIR}/ to air-gap media (USB)"
    echo "    2. Copy to Tomb VM at /opt/tomb/grimoire/mitre-attck/"
    echo "    3. Run setup-grimoire.sh to index data"
    echo "======================================"
    echo ""

    return "${failures}"
}

main "$@"
