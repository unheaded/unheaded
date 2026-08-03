#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# collect-kingdom-docs.sh — Collect all relevant docs from the Unheaded repo
#
# Copies all .md files, architecture docs, security docs, protocol specs,
# and configuration references from the Unheaded monorepo into a deployable
# package for the Grimoire.
#
# Run this on the development machine (WEST) where the repo lives,
# then transfer the output to the Tomb VM via air-gap media.
#
# Usage:
#   ./collect-kingdom-docs.sh [--repo-root /path/to/unheaded] [--output-dir ./output]
#
# Output structure:
#   output/
#   ├── architecture/     — Architecture and design docs
#   ├── security/         — Security assessments and audits
#   ├── protocol/         — Protocol specs and RFCs
#   ├── legal/            — License, IP, compliance docs
#   ├── infrastructure/   — Deployment and infrastructure docs
#   ├── adr/              — Architecture Decision Records
#   ├── services/         — Per-service documentation
#   ├── references/       — Timeline, calendar, strategic refs
#   ├── battle-plans/     — Sprint plans and runbooks
#   ├── configs/          — Service configurations (YAML/TOML)
#   ├── nix/              — NixOS container definitions
#   └── root-docs/        — Top-level repo docs (CLAUDE.md, README, etc.)

set -euo pipefail

# --- Configuration ---
REPO_ROOT=""
OUTPUT_DIR=""
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- Colors ---
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }

# --- Argument Parsing ---

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --repo-root)
                REPO_ROOT="$2"
                shift 2
                ;;
            --output-dir)
                OUTPUT_DIR="$2"
                shift 2
                ;;
            --help|-h)
                echo "Usage: $0 [--repo-root /path/to/unheaded] [--output-dir ./output]"
                echo ""
                echo "Options:"
                echo "  --repo-root   Path to the Unheaded monorepo (default: auto-detect)"
                echo "  --output-dir  Where to write collected docs (default: same dir as script)"
                exit 0
                ;;
            *)
                echo "Unknown option: $1" >&2
                exit 1
                ;;
        esac
    done

    # Auto-detect repo root: walk up from script location
    if [[ -z "${REPO_ROOT}" ]]; then
        local candidate="${SCRIPT_DIR}"
        while [[ "${candidate}" != "/" ]]; do
            if [[ -f "${candidate}/go.mod" && -f "${candidate}/CLAUDE.md" ]]; then
                REPO_ROOT="${candidate}"
                break
            fi
            candidate="$(dirname "${candidate}")"
        done

        if [[ -z "${REPO_ROOT}" ]]; then
            echo "ERROR: Could not auto-detect repo root. Use --repo-root." >&2
            exit 1
        fi
    fi

    if [[ -z "${OUTPUT_DIR}" ]]; then
        OUTPUT_DIR="${SCRIPT_DIR}"
    fi
}

# --- Collection Functions ---

# Copy files matching a pattern from source to dest, preserving structure
collect() {
    local label="$1"
    local src="$2"
    local dest="$3"
    shift 3
    local patterns=("$@")

    if [[ ! -d "${src}" ]]; then
        warn "Source directory not found: ${src} (skipping ${label})"
        return 0
    fi

    mkdir -p "${dest}"

    local count=0
    for pattern in "${patterns[@]}"; do
        while IFS= read -r -d '' file; do
            local rel_path="${file#${src}/}"
            local target_dir
            target_dir="${dest}/$(dirname "${rel_path}")"
            mkdir -p "${target_dir}"
            cp "${file}" "${target_dir}/"
            ((count++))
        done < <(find "${src}" -type f -name "${pattern}" -print0 2>/dev/null)
    done

    info "${label}: ${count} files collected"
}

# --- Main Collection ---

main() {
    parse_args "$@"

    echo ""
    echo "======================================"
    echo "  Kingdom Document Collector"
    echo "  Repo: ${REPO_ROOT}"
    echo "  Output: ${OUTPUT_DIR}"
    echo "======================================"
    echo ""

    # --- Root-level docs ---
    info "Collecting root-level documentation..."
    mkdir -p "${OUTPUT_DIR}/root-docs"
    for file in CLAUDE.md README.md SECURITY.md QUICKSTART.md VISION.md \
                CONTRIBUTOR-GUIDE.md THIRD_PARTY.md CHANGELOG.md \
                RELEASE_CHECKLIST.md LICENSE LICENSE-PROTOCOLS; do
        if [[ -f "${REPO_ROOT}/${file}" ]]; then
            cp "${REPO_ROOT}/${file}" "${OUTPUT_DIR}/root-docs/"
        fi
    done
    local root_count
    root_count=$(find "${OUTPUT_DIR}/root-docs" -type f | wc -l)
    info "Root docs: ${root_count} files"

    # --- Architecture docs ---
    collect "Architecture" \
        "${REPO_ROOT}/docs/architecture" \
        "${OUTPUT_DIR}/architecture" \
        "*.md"

    # --- Security docs ---
    collect "Security" \
        "${REPO_ROOT}/docs/security" \
        "${OUTPUT_DIR}/security" \
        "*.md"

    # --- Protocol specs ---
    collect "Protocol" \
        "${REPO_ROOT}/docs/protocol" \
        "${OUTPUT_DIR}/protocol" \
        "*.md"

    # --- Legal and compliance ---
    collect "Legal" \
        "${REPO_ROOT}/docs/legal" \
        "${OUTPUT_DIR}/legal" \
        "*.md"

    # --- Infrastructure docs ---
    collect "Infrastructure" \
        "${REPO_ROOT}/docs/infrastructure" \
        "${OUTPUT_DIR}/infrastructure" \
        "*.md"

    # --- ADRs ---
    collect "ADRs" \
        "${REPO_ROOT}/docs/adr" \
        "${OUTPUT_DIR}/adr" \
        "*.md"

    # --- Battle plans ---
    collect "Battle Plans" \
        "${REPO_ROOT}/docs/battle-plans" \
        "${OUTPUT_DIR}/battle-plans" \
        "*.md"

    # --- Bare metal docs ---
    collect "Bare Metal" \
        "${REPO_ROOT}/docs/bare-metal" \
        "${OUTPUT_DIR}/bare-metal" \
        "*.md"

    # --- Network docs ---
    collect "Network" \
        "${REPO_ROOT}/docs/network" \
        "${OUTPUT_DIR}/network" \
        "*.md"

    # --- Monitoring docs ---
    collect "Monitoring" \
        "${REPO_ROOT}/docs/monitoring" \
        "${OUTPUT_DIR}/monitoring" \
        "*.md"

    # --- Research docs ---
    collect "Research" \
        "${REPO_ROOT}/docs/research" \
        "${OUTPUT_DIR}/research" \
        "*.md"

    # --- References (timeline, calendar, strategic) ---
    collect "References" \
        "${REPO_ROOT}/references" \
        "${OUTPUT_DIR}/references" \
        "*.md"

    # --- Service configurations ---
    info "Collecting service configurations..."
    mkdir -p "${OUTPUT_DIR}/configs"
    for ext in yaml yml toml json; do
        while IFS= read -r -d '' file; do
            local rel_path="${file#${REPO_ROOT}/}"
            local target_dir
            target_dir="${OUTPUT_DIR}/configs/$(dirname "${rel_path}")"
            mkdir -p "${target_dir}"
            cp "${file}" "${target_dir}/"
        done < <(find "${REPO_ROOT}/configs" -type f -name "*.${ext}" -print0 2>/dev/null)
    done
    local config_count
    config_count=$(find "${OUTPUT_DIR}/configs" -type f 2>/dev/null | wc -l)
    info "Configs: ${config_count} files"

    # --- NixOS container definitions ---
    collect "NixOS Definitions" \
        "${REPO_ROOT}/nix" \
        "${OUTPUT_DIR}/nix" \
        "*.nix"

    # --- Docker and container configs ---
    info "Collecting container configs..."
    mkdir -p "${OUTPUT_DIR}/containers"
    if [[ -f "${REPO_ROOT}/docker-compose.yml" ]]; then
        cp "${REPO_ROOT}/docker-compose.yml" "${OUTPUT_DIR}/containers/"
    fi
    if [[ -f "${REPO_ROOT}/Dockerfile" ]]; then
        cp "${REPO_ROOT}/Dockerfile" "${OUTPUT_DIR}/containers/"
    fi
    collect "Container Definitions" \
        "${REPO_ROOT}/containers" \
        "${OUTPUT_DIR}/containers" \
        "*.nix" "*.yml" "*.yaml" "*.toml"

    # --- Lore docs (Kingdom lore and sessions) ---
    collect "Lore" \
        "${REPO_ROOT}/docs/lore" \
        "${OUTPUT_DIR}/lore" \
        "*.md"

    # --- Session history ---
    collect "Sessions" \
        "${REPO_ROOT}/docs/sessions" \
        "${OUTPUT_DIR}/sessions" \
        "*.md"

    # --- Service-specific docs ---
    info "Collecting per-service README files..."
    mkdir -p "${OUTPUT_DIR}/services"
    for svc_dir in "${REPO_ROOT}"/services/*/; do
        if [[ -d "${svc_dir}" ]]; then
            local svc_name
            svc_name=$(basename "${svc_dir}")
            for doc in README.md ARCHITECTURE.md DESIGN.md; do
                if [[ -f "${svc_dir}/${doc}" ]]; then
                    mkdir -p "${OUTPUT_DIR}/services/${svc_name}"
                    cp "${svc_dir}/${doc}" "${OUTPUT_DIR}/services/${svc_name}/"
                fi
            done
        fi
    done

    # --- Pkg documentation ---
    info "Collecting pkg README files..."
    mkdir -p "${OUTPUT_DIR}/pkg"
    for pkg_dir in "${REPO_ROOT}"/pkg/*/; do
        if [[ -d "${pkg_dir}" ]]; then
            local pkg_name
            pkg_name=$(basename "${pkg_dir}")
            for doc in README.md; do
                if [[ -f "${pkg_dir}/${doc}" ]]; then
                    mkdir -p "${OUTPUT_DIR}/pkg/${pkg_name}"
                    cp "${pkg_dir}/${doc}" "${OUTPUT_DIR}/pkg/${pkg_name}/"
                fi
            done
        fi
    done

    # --- Generate manifest ---
    info "Generating collection manifest..."
    local manifest="${OUTPUT_DIR}/MANIFEST.txt"
    {
        echo "# Grimoire Kingdom Docs Collection Manifest"
        echo "# Generated: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
        echo "# Source repo: ${REPO_ROOT}"
        echo "# Git commit: $(cd "${REPO_ROOT}" && git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
        echo ""
        find "${OUTPUT_DIR}" -type f -not -name 'MANIFEST.txt' -not -name 'collect-kingdom-docs.sh' | \
            sort | \
            sed "s|${OUTPUT_DIR}/||"
    } > "${manifest}"

    local total_count
    total_count=$(find "${OUTPUT_DIR}" -type f -not -name 'MANIFEST.txt' -not -name 'collect-kingdom-docs.sh' | wc -l)
    local total_size
    total_size=$(du -sh "${OUTPUT_DIR}" 2>/dev/null | cut -f1)

    echo ""
    echo "======================================"
    echo "  COLLECTION COMPLETE"
    echo "======================================"
    echo "  Total files: ${total_count}"
    echo "  Total size:  ${total_size}"
    echo "  Manifest:    ${manifest}"
    echo ""
    echo "  Next steps:"
    echo "    1. Transfer ${OUTPUT_DIR} to Tomb VM via air-gap media"
    echo "    2. Run setup-grimoire.sh on the Tomb VM"
    echo "======================================"
    echo ""
}

main "$@"
