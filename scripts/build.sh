#!/bin/bash
# Unheaded Build Script
# ======================
# Builds all binaries with version metadata injection
# Usage: ./scripts/build.sh [output-dir]

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="${1:-${PROJECT_ROOT}/bin}"
VERBOSE="${VERBOSE:-0}"

# Create output directory
mkdir -p "${OUTPUT_DIR}"

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Version metadata
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
COMMIT_FULL=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_USER="${USER:-unknown}"
BUILD_HOST="${HOSTNAME:-unknown}"

log_info "Unheaded Build System"
log_info "====================="
log_info "Version:     ${VERSION}"
log_info "Commit:      ${COMMIT} (${COMMIT_FULL})"
log_info "Build Date:  ${BUILD_DATE}"
log_info "Build User:  ${BUILD_USER}@${BUILD_HOST}"
log_info "Output Dir:  ${OUTPUT_DIR}"
log_info ""

# Go build flags
# -v: verbose
# -trimpath: trim file paths in build output for reproducibility
# -ldflags: linker flags for injecting version metadata
GO_BUILD_FLAGS="-v -trimpath"
GO_LDFLAGS=(
    "-s"                                    # Strip symbol table
    "-w"                                    # Omit DWARF symbol table
    "-X main.Version=${VERSION}"
    "-X main.Commit=${COMMIT}"
    "-X main.CommitFull=${COMMIT_FULL}"
    "-X main.BuildDate=${BUILD_DATE}"
    "-X main.BuildUser=${BUILD_USER}"
    "-X main.BuildHost=${BUILD_HOST}"
)

# Convert array to string
GO_LDFLAGS_STR="${GO_LDFLAGS[*]}"

# Function to build a binary
build_binary() {
    local binary_name="$1"
    local source_dir="$2"
    local output_path="${OUTPUT_DIR}/${binary_name}"

    log_info "Building ${binary_name}..."

    if [ "${VERBOSE}" = "1" ]; then
        log_info "  Source:  ${source_dir}"
        log_info "  Output:  ${output_path}"
        log_info "  Flags:   ${GO_BUILD_FLAGS}"
        log_info "  LDFlags: ${GO_LDFLAGS_STR}"
    fi

    # Build with error handling
    if cd "${source_dir}" && go build \
        ${GO_BUILD_FLAGS} \
        -ldflags "${GO_LDFLAGS_STR}" \
        -o "${output_path}"; then

        # Verify binary was created and is executable
        if [ -f "${output_path}" ] && [ -x "${output_path}" ]; then
            local size=$(du -h "${output_path}" | cut -f1)
            log_success "${binary_name} built successfully (${size})"
            return 0
        else
            log_error "${binary_name} binary not found or not executable at ${output_path}"
            return 1
        fi
    else
        log_error "Failed to build ${binary_name}"
        return 1
    fi
}

# Function to build all Go cmd/ binaries
build_all_go_binaries() {
    log_info ""
    log_info "Building Go Binaries"
    log_info "===================="

    local build_errors=0

    # Core daemon
    if ! build_binary "unheaded-daemon" "${PROJECT_ROOT}/cmd/unheaded-daemon"; then
        ((build_errors++))
    fi

    # CLI
    if ! build_binary "unheaded-cli" "${PROJECT_ROOT}/cmd/unheaded-cli"; then
        ((build_errors++))
    fi

    # Application binaries
    if ! build_binary "kanban-app" "${PROJECT_ROOT}/cmd/kanban-app"; then
        ((build_errors++))
    fi

    if ! build_binary "dashboard-backend" "${PROJECT_ROOT}/cmd/dashboard-backend"; then
        ((build_errors++))
    fi

    if ! build_binary "monad" "${PROJECT_ROOT}/cmd/monad"; then
        ((build_errors++))
    fi

    if ! build_binary "sophia" "${PROJECT_ROOT}/cmd/sophia"; then
        ((build_errors++))
    fi

    # Service binaries
    if ! build_binary "wotan" "${PROJECT_ROOT}/services/wotan/cmd/wotan"; then
        ((build_errors++))
    fi

    if ! build_binary "timeguru" "${PROJECT_ROOT}/services/timeguru/cmd/timeguru"; then
        ((build_errors++))
    fi

    if ! build_binary "captain" "${PROJECT_ROOT}/services/captain/cmd/captain"; then
        ((build_errors++))
    fi

    if ! build_binary "architect" "${PROJECT_ROOT}/services/architect/cmd/architect"; then
        ((build_errors++))
    fi

    if ! build_binary "micromanager" "${PROJECT_ROOT}/services/micromanager/cmd/micromanager"; then
        ((build_errors++))
    fi

    if ! build_binary "gateway" "${PROJECT_ROOT}/services/gateway/cmd"; then
        ((build_errors++))
    fi

    return $build_errors
}

# Function to build Rust binaries if available
build_rust_binaries() {
    if [ ! -d "${PROJECT_ROOT}/ebpf" ]; then
        log_warn "eBPF directory not found, skipping Rust builds"
        return 0
    fi

    log_info ""
    log_info "Building Rust Binaries"
    log_info "======================"

    local build_errors=0

    # Check if cargo is available
    if ! command -v cargo &> /dev/null; then
        log_warn "Cargo not found, skipping Rust builds"
        return 0
    fi

    # Build trace-collector if it exists
    if [ -d "${PROJECT_ROOT}/cmd/trace-collector" ]; then
        log_info "Building trace-collector..."
        if cd "${PROJECT_ROOT}/cmd/trace-collector" && cargo build --release; then
            if cp target/release/trace-collector "${OUTPUT_DIR}/"; then
                log_success "trace-collector built successfully"
            else
                log_error "Failed to copy trace-collector binary"
                ((build_errors++))
            fi
        else
            log_error "Failed to build trace-collector"
            ((build_errors++))
        fi
    fi

    # Build eBPF programs (note: these are libraries, not binaries)
    if [ -d "${PROJECT_ROOT}/ebpf" ]; then
        log_info "Building eBPF programs..."
        if cd "${PROJECT_ROOT}/ebpf" && cargo build --release --target=bpfel-unknown-none; then
            log_success "eBPF programs built successfully"
        else
            log_warn "Failed to build eBPF programs (this is optional)"
            # Don't count as error - eBPF is optional
        fi
    fi

    return $build_errors
}

# Main build process
main() {
    cd "${PROJECT_ROOT}"

    local total_errors=0

    # Build Go binaries
    if ! build_all_go_binaries; then
        ((total_errors++))
    fi

    # Build Rust binaries (optional)
    if ! build_rust_binaries; then
        ((total_errors++))
    fi

    # Summary
    log_info ""
    log_info "Build Summary"
    log_info "============="

    if [ -d "${OUTPUT_DIR}" ]; then
        local binary_count=$(find "${OUTPUT_DIR}" -type f -executable 2>/dev/null | wc -l || echo "0")
        log_info "Binaries created: ${binary_count}"
        log_info "Output directory: ${OUTPUT_DIR}"

        if [ "${binary_count}" -gt 0 ]; then
            log_info ""
            log_info "Built binaries:"
            find "${OUTPUT_DIR}" -type f -executable -exec basename {} \; 2>/dev/null | sort | sed 's/^/  - /'
        fi
    fi

    if [ ${total_errors} -eq 0 ]; then
        log_success "Build completed successfully"
        return 0
    else
        log_error "Build completed with ${total_errors} error(s)"
        return 1
    fi
}

# Execute main function
main "$@"
