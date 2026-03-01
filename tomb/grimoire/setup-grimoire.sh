#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# setup-grimoire.sh — Grimoire deployment for the Tomb of Knowledge
#
# Installs the Grimoire knowledge base on the Tomb VM:
#   1. Creates /opt/tomb/grimoire/ directory structure
#   2. Copies Kingdom docs from the deployment package
#   3. Stages MITRE ATT&CK data (pre-downloaded)
#   4. Installs ChromaDB and sentence-transformers
#   5. Indexes all documents into the vector store
#
# Usage:
#   ./setup-grimoire.sh [--skip-index] [--force]
#
# Prerequisites:
#   - Python 3.10+ with pip
#   - Pre-downloaded MITRE ATT&CK data in mitre-attck/ (run fetch-mitre-data.sh first)
#   - Kingdom docs collected in kingdom-docs/ (run collect-kingdom-docs.sh first)
#
# Designed for air-gapped Kali Linux VM (Tomb of Knowledge).

set -euo pipefail

# --- Configuration ---
GRIMOIRE_ROOT="/opt/tomb/grimoire"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
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
                echo "  --skip-index  Skip ChromaDB indexing (install only)"
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

    mkdir -p "${GRIMOIRE_ROOT}"/{kingdom-docs,mitre-attck,nvd-cves,cwe,threat-models,history,rag/{chroma.db,embeddings}}
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
    info "  ${GRIMOIRE_ROOT}/rag/             — RAG vector index (ChromaDB)"
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

# --- Phase 5: Install Python Dependencies ---

install_python_deps() {
    info "Phase 5: Installing Python dependencies for RAG pipeline"

    # Check Python version
    if ! command -v python3 &>/dev/null; then
        die "Python 3 is required but not found. Install python3 first."
    fi

    local python_version
    python_version=$(python3 -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')
    info "Python version: ${python_version}"

    local major minor
    major=$(echo "${python_version}" | cut -d. -f1)
    minor=$(echo "${python_version}" | cut -d. -f2)

    if [[ "${major}" -lt 3 ]] || { [[ "${major}" -eq 3 ]] && [[ "${minor}" -lt 10 ]]; }; then
        die "Python 3.10+ required, found ${python_version}"
    fi

    # Create virtual environment for Grimoire
    local venv_dir="${GRIMOIRE_ROOT}/rag/.venv"

    if [[ -d "${venv_dir}" && "${FORCE}" == "false" ]]; then
        info "Virtual environment already exists at ${venv_dir}"
    else
        info "Creating virtual environment at ${venv_dir}"
        python3 -m venv "${venv_dir}"
    fi

    # Install from requirements.txt
    local requirements="${SCRIPT_DIR}/rag/requirements.txt"

    if [[ ! -f "${requirements}" ]]; then
        die "requirements.txt not found at ${requirements}"
    fi

    info "Installing Python packages (this may take several minutes on first run)..."
    "${venv_dir}/bin/pip" install --upgrade pip --quiet
    "${venv_dir}/bin/pip" install -r "${requirements}" --quiet

    info "Python dependencies installed successfully"

    # Copy RAG scripts to deployment location
    cp "${SCRIPT_DIR}/rag/rag-index.py" "${GRIMOIRE_ROOT}/rag/"
    cp "${SCRIPT_DIR}/rag/rag-query.py" "${GRIMOIRE_ROOT}/rag/"
    cp "${SCRIPT_DIR}/rag/rag-config.json" "${GRIMOIRE_ROOT}/rag/"
    cp "${SCRIPT_DIR}/rag/requirements.txt" "${GRIMOIRE_ROOT}/rag/"
    chmod +x "${GRIMOIRE_ROOT}/rag/rag-index.py"
    chmod +x "${GRIMOIRE_ROOT}/rag/rag-query.py"

    info "RAG pipeline scripts deployed to ${GRIMOIRE_ROOT}/rag/"
}

# --- Phase 6: Index Documents ---

index_documents() {
    if [[ "${SKIP_INDEX}" == "true" ]]; then
        info "Phase 6: Skipping document indexing (--skip-index)"
        return 0
    fi

    info "Phase 6: Indexing all documents into ChromaDB"

    local venv_dir="${GRIMOIRE_ROOT}/rag/.venv"
    local index_script="${GRIMOIRE_ROOT}/rag/rag-index.py"
    local config_file="${GRIMOIRE_ROOT}/rag/rag-config.json"

    if [[ ! -f "${index_script}" ]]; then
        die "rag-index.py not found at ${index_script}"
    fi

    if [[ ! -f "${config_file}" ]]; then
        die "rag-config.json not found at ${config_file}"
    fi

    info "Indexing Kingdom docs..."
    "${venv_dir}/bin/python" "${index_script}" \
        --config "${config_file}" \
        --source-dir "${GRIMOIRE_ROOT}/kingdom-docs" \
        --collection "kingdom-docs" \
        --source-type "kingdom"

    # Index MITRE ATT&CK data if present
    local mitre_dir="${GRIMOIRE_ROOT}/mitre-attck"
    if [[ -d "${mitre_dir}" ]] && find "${mitre_dir}" -name '*.json' -print -quit | grep -q .; then
        info "Indexing MITRE ATT&CK data..."
        "${venv_dir}/bin/python" "${index_script}" \
            --config "${config_file}" \
            --source-dir "${mitre_dir}" \
            --collection "mitre-attck" \
            --source-type "mitre"
    fi

    # Index threat models if present
    local threat_dir="${GRIMOIRE_ROOT}/threat-models"
    if [[ -d "${threat_dir}" ]] && find "${threat_dir}" -name '*.md' -print -quit | grep -q .; then
        info "Indexing threat models..."
        "${venv_dir}/bin/python" "${index_script}" \
            --config "${config_file}" \
            --source-dir "${threat_dir}" \
            --collection "threat-models" \
            --source-type "kingdom"
    fi

    info "Document indexing complete"
}

# --- Phase 7: Verification ---

verify_installation() {
    info "Phase 7: Verifying Grimoire installation"

    local errors=0

    # Check directory structure
    for dir in kingdom-docs mitre-attck threat-models rag rag/chroma.db; do
        if [[ ! -d "${GRIMOIRE_ROOT}/${dir}" ]]; then
            error "Missing directory: ${GRIMOIRE_ROOT}/${dir}"
            ((errors++))
        fi
    done

    # Check RAG scripts
    for script in rag/rag-index.py rag/rag-query.py rag/rag-config.json; do
        if [[ ! -f "${GRIMOIRE_ROOT}/${script}" ]]; then
            error "Missing file: ${GRIMOIRE_ROOT}/${script}"
            ((errors++))
        fi
    done

    # Check virtual environment
    if [[ ! -f "${GRIMOIRE_ROOT}/rag/.venv/bin/python" ]]; then
        error "Python virtual environment not found"
        ((errors++))
    fi

    # Check ChromaDB importability
    if "${GRIMOIRE_ROOT}/rag/.venv/bin/python" -c "import chromadb" 2>/dev/null; then
        info "ChromaDB: importable"
    else
        error "ChromaDB: not importable"
        ((errors++))
    fi

    # Check sentence-transformers importability
    if "${GRIMOIRE_ROOT}/rag/.venv/bin/python" -c "from sentence_transformers import SentenceTransformer" 2>/dev/null; then
        info "sentence-transformers: importable"
    else
        error "sentence-transformers: not importable"
        ((errors++))
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
    echo "  RAG pipeline:      installed"
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
    echo "    Query:  ${GRIMOIRE_ROOT}/rag/.venv/bin/python ${GRIMOIRE_ROOT}/rag/rag-query.py --query 'your question'"
    echo "    Index:  ${GRIMOIRE_ROOT}/rag/.venv/bin/python ${GRIMOIRE_ROOT}/rag/rag-index.py --source-dir /path/to/docs"
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
    install_python_deps
    index_documents
    verify_installation
}

main "$@"
