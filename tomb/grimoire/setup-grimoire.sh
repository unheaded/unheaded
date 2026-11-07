#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# setup-grimoire.sh — Grimoire deployment for the Tomb of Knowledge
#
# Installs the Grimoire knowledge base on the Tomb VM:
#   1. Creates /opt/tomb/grimoire/ directory structure
#   2. Copies Kingdom docs from the deployment package
#   3. Stages MITRE ATT&CK data (pre-downloaded)
#   4. Installs zhen-codeindex, one static binary
#   5. Indexes all documents for word search (no Python, no model, no network)
#
# Usage:
#   ./setup-grimoire.sh [--skip-index] [--force]
#
# Prerequisites:
#   - zhen-codeindex built for the VM beside this script in bin/ (make stage-grimoire,
#     or ZHEN_CODEINDEX=/path/to/zhen-codeindex)
#   - Pre-downloaded MITRE ATT&CK data in mitre-attck/ (run fetch-mitre-data.sh first)
#   - Kingdom docs collected in kingdom-docs/ (run collect-kingdom-docs.sh first)
#
# Designed for air-gapped Kali Linux VM (Tomb of Knowledge).

set -euo pipefail

# --- Configuration ---
GRIMOIRE_ROOT="/opt/tomb/grimoire"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CODEINDEX_SRC="${ZHEN_CODEINDEX:-${SCRIPT_DIR}/bin/zhen-codeindex}"
CODEINDEX="${GRIMOIRE_ROOT}/bin/zhen-codeindex"
INDEX_DIR="${GRIMOIRE_ROOT}/index"
SKIP_INDEX=false
FORCE=false
LOG_FILE="/var/log/tomb/grimoire-setup.log"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# --- Functions ---

log() {
    local level="$1"
    shift
    local timestamp
    timestamp="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
    echo -e "${CYAN}[${timestamp}]${NC} ${level} $*"
    echo "[${timestamp}] ${level} $*" >> "${LOG_FILE}" 2>/dev/null || true
}

info()  { log "${GREEN}[INFO]${NC}" "$@"; }
warn()  { log "${YELLOW}[WARN]${NC}" "$@"; }
error() { log "${RED}[ERROR]${NC}" "$@"; }

die() {
    error "$@"
    exit 1
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        die "This script must be run as root (creates /opt/tomb/grimoire/)"
    fi
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --skip-index)
                SKIP_INDEX=true
                shift
                ;;
            --force)
                FORCE=true
                shift
                ;;
            --help|-h)
                echo "Usage: $0 [--skip-index] [--force]"
                echo ""
                echo "Options:"
                echo "  --skip-index  Skip building the corpus index (install only)"
                echo "  --force       Overwrite existing Grimoire installation"
                echo "  --help        Show this help message"
                exit 0
                ;;
            *)
                die "Unknown option: $1"
                ;;
        esac
    done
}

# --- Phase 1: Directory Structure ---

create_directory_structure() {
    info "Phase 1: Creating Grimoire directory structure at ${GRIMOIRE_ROOT}"

    if [[ -d "${GRIMOIRE_ROOT}" && "${FORCE}" == "false" ]]; then
        warn "Grimoire root ${GRIMOIRE_ROOT} already exists. Use --force to overwrite."
        return 0
    fi

    mkdir -p "${GRIMOIRE_ROOT}"/{kingdom-docs,mitre-attck,nvd-cves,cwe,threat-models,history,bin,index}
    mkdir -p /var/log/tomb

    # Set permissions: root-owned, grimoire group readable
    chown -R root:root "${GRIMOIRE_ROOT}"
    chmod -R 755 "${GRIMOIRE_ROOT}"

    info "Directory structure created:"
    info "  ${GRIMOIRE_ROOT}/kingdom-docs/    — Kingdom architecture and config docs"
    info "  ${GRIMOIRE_ROOT}/mitre-attck/     — MITRE ATT&CK framework data (JSON)"
    info "  ${GRIMOIRE_ROOT}/nvd-cves/        — NVD CVE records (future)"
    info "  ${GRIMOIRE_ROOT}/cwe/             — CWE mappings (future)"
    info "  ${GRIMOIRE_ROOT}/threat-models/   — Kingdom threat models"
    info "  ${GRIMOIRE_ROOT}/history/         — Historical findings and reports"
    info "  ${GRIMOIRE_ROOT}/bin/             — zhen-codeindex"
    info "  ${GRIMOIRE_ROOT}/index/           — Corpus index (word search)"
}

# --- Phase 2: Copy Kingdom Docs ---

copy_kingdom_docs() {
    info "Phase 2: Copying Kingdom documentation"

    local source_dir="${SCRIPT_DIR}/kingdom-docs"

    if [[ ! -d "${source_dir}" ]]; then
        warn "No kingdom-docs/ directory found at ${source_dir}"
        warn "Run collect-kingdom-docs.sh first to gather docs from the Unheaded repo"
        return 1
    fi

    local doc_count
    doc_count=$(find "${source_dir}" -type f -name '*.md' | wc -l)

    if [[ "${doc_count}" -eq 0 ]]; then
        warn "No .md files found in ${source_dir}"
        return 1
    fi

    # Copy all docs preserving directory structure
    cp -r "${source_dir}"/* "${GRIMOIRE_ROOT}/kingdom-docs/"

    info "Copied ${doc_count} Kingdom documents to ${GRIMOIRE_ROOT}/kingdom-docs/"
}

# --- Phase 3: Stage MITRE ATT&CK Data ---

stage_mitre_data() {
    info "Phase 3: Staging MITRE ATT&CK data"

    local mitre_source="${SCRIPT_DIR}/mitre-attck"

    if [[ ! -d "${mitre_source}" ]]; then
        warn "No mitre-attck/ directory found at ${mitre_source}"
        warn "Run fetch-mitre-data.sh on an internet-connected host first"
        return 1
    fi

    local json_count
    json_count=$(find "${mitre_source}" -type f -name '*.json' | wc -l)

    if [[ "${json_count}" -eq 0 ]]; then
        warn "No JSON files found in ${mitre_source}"
        warn "Run fetch-mitre-data.sh to download MITRE ATT&CK data"
        return 1
    fi

    cp -r "${mitre_source}"/* "${GRIMOIRE_ROOT}/mitre-attck/"

    info "Staged ${json_count} MITRE ATT&CK JSON files to ${GRIMOIRE_ROOT}/mitre-attck/"
}

# --- Phase 4: Copy Threat Models ---

copy_threat_models() {
    info "Phase 4: Copying threat models"

    local threat_source="${SCRIPT_DIR}/threat-models"

    if [[ ! -d "${threat_source}" ]]; then
        warn "No threat-models/ directory found at ${threat_source}"
        return 1
    fi

    cp -r "${threat_source}"/* "${GRIMOIRE_ROOT}/threat-models/"

    local model_count
    model_count=$(find "${threat_source}" -type f -name '*.md' | wc -l)
    info "Copied ${model_count} threat model documents"
}

# --- Phase 5: Install zhen-codeindex ---

install_codeindex() {
    info "Phase 5: Installing zhen-codeindex"

    if [[ ! -f "${CODEINDEX_SRC}" ]]; then
        die "zhen-codeindex not found at ${CODEINDEX_SRC} (make stage-grimoire, or set ZHEN_CODEINDEX)"
    fi

    install -m 0755 "${CODEINDEX_SRC}" "${CODEINDEX}"

    info "zhen-codeindex installed at ${CODEINDEX}"
}

# --- Phase 6: Index Documents ---

index_documents() {
    if [[ "${SKIP_INDEX}" == "true" ]]; then
        info "Phase 6: Skipping document indexing (--skip-index)"
        return 0
    fi

    info "Phase 6: Indexing the corpus under ${GRIMOIRE_ROOT}"

    "${CODEINDEX}" --corpus "${GRIMOIRE_ROOT}" --out "${INDEX_DIR}"

    info "Document indexing complete"
}

# --- Phase 7: Verification ---

verify_installation() {
    info "Phase 7: Verifying Grimoire installation"

    local errors=0

    # Check directory structure
    for dir in kingdom-docs mitre-attck threat-models bin index; do
        if [[ ! -d "${GRIMOIRE_ROOT}/${dir}" ]]; then
            error "Missing directory: ${GRIMOIRE_ROOT}/${dir}"
            errors=$((errors + 1))
        fi
    done

    if [[ ! -x "${CODEINDEX}" ]]; then
        error "zhen-codeindex not executable: ${CODEINDEX}"
        errors=$((errors + 1))
    fi

    # A search that answers proves the index and the binary together
    if [[ "${SKIP_INDEX}" == "false" ]]; then
        if "${CODEINDEX}" --out "${INDEX_DIR}" --search "threat" --top-k 1 | grep -q '"source"'; then
            info "Corpus index: answers a search"
        else
            error "Corpus index: no answer to a search"
            errors=$((errors + 1))
        fi
    fi

    # Summary
    local kingdom_count mitre_count threat_count
    kingdom_count=$(find "${GRIMOIRE_ROOT}/kingdom-docs" -type f 2>/dev/null | wc -l)
    mitre_count=$(find "${GRIMOIRE_ROOT}/mitre-attck" -type f -name '*.json' 2>/dev/null | wc -l)
    threat_count=$(find "${GRIMOIRE_ROOT}/threat-models" -type f -name '*.md' 2>/dev/null | wc -l)

    echo ""
    echo "============================================"
    echo "  GRIMOIRE INSTALLATION SUMMARY"
    echo "============================================"
    echo "  Root:              ${GRIMOIRE_ROOT}"
    echo "  Kingdom docs:      ${kingdom_count} files"
    echo "  MITRE ATT&CK:      ${mitre_count} JSON files"
    echo "  Threat models:     ${threat_count} documents"
    echo "  Search:            ${CODEINDEX}"
    echo "  Errors:            ${errors}"
    echo "============================================"
    echo ""

    if [[ "${errors}" -gt 0 ]]; then
        error "Installation completed with ${errors} error(s)"
        return 1
    fi

    info "Grimoire installation verified successfully"
    echo ""
    echo "  Usage:"
    echo "    Query:  ${CODEINDEX} --out ${INDEX_DIR} --search 'your terms'"
    echo "    Index:  ${CODEINDEX} --corpus ${GRIMOIRE_ROOT} --out ${INDEX_DIR}"
    echo ""
}

# --- Main ---

main() {
    parse_args "$@"
    check_root

    echo ""
    echo "======================================"
    echo "  THE GRIMOIRE — Knowledge Base Setup"
    echo "  Tomb of Knowledge Deployment"
    echo "======================================"
    echo ""

    create_directory_structure
    copy_kingdom_docs
    stage_mitre_data
    copy_threat_models
    install_codeindex
    index_documents
    verify_installation
}

main "$@"
