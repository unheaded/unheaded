#!/bin/bash
# build-sealed-cask.sh — Build a deterministic, immutable Unheaded deployment image
# ADR-010: Sealed Cask Deployment (Soul Vessel Model)
#
# Creates a tarball with:
#   - All compiled Go binaries
#   - All eBPF programs
#   - Configuration files
#   - NixOS container definitions
#   - SHA256 manifest (Binding Rune)
#
# The output is a self-contained deployment package that can be verified
# before deployment via verify-binding-rune.sh
#
# Usage:
#   ./scripts/build-sealed-cask.sh [--output /path/to/cask.tar.gz]

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TIMESTAMP=$(date -u +%Y%m%d-%H%M%S)
OUTPUT="${1:-${PROJECT_ROOT}/deploy/cask-${TIMESTAMP}.tar.gz}"
STAGING="/tmp/sealed-cask-${TIMESTAMP}"
MANIFEST="${STAGING}/BINDING_RUNE.sha256"

echo "=============================================="
echo "SEALED CASK BUILD — ADR-010"
echo "=============================================="
echo "Project: ${PROJECT_ROOT}"
echo "Output:  ${OUTPUT}"
echo "Time:    $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

# --- Phase 1: Build all binaries ---
echo "=== Phase 1: Build Go binaries ==="
cd "${PROJECT_ROOT}"
go build ./...
echo "  Go build: OK"

# --- Phase 2: Build eBPF programs ---
echo "=== Phase 2: Build eBPF programs ==="
cd "${PROJECT_ROOT}/ebpf"
cargo build --release 2>&1 | tail -3
echo "  eBPF build: OK"

# --- Phase 3: Stage artifacts ---
echo "=== Phase 3: Stage artifacts ==="
mkdir -p "${STAGING}"/{bin,ebpf,configs,nix,runbooks,db}

# Go binaries
echo "  Staging Go binaries..."
for dir in cmd/*/; do
    BIN=$(basename "$dir")
    if [ -f "${PROJECT_ROOT}/${dir}${BIN}" ]; then
        cp "${PROJECT_ROOT}/${dir}${BIN}" "${STAGING}/bin/"
    fi
done
# Service binaries
for dir in services/*/cmd/*/; do
    BIN=$(basename "$dir")
    if [ -f "${PROJECT_ROOT}/${dir}${BIN}" ]; then
        cp "${PROJECT_ROOT}/${dir}${BIN}" "${STAGING}/bin/"
    fi
done

# eBPF objects
echo "  Staging eBPF objects..."
find "${PROJECT_ROOT}/ebpf/target/bpfel-unknown-none/release/" -maxdepth 1 -type f 2>/dev/null | while read f; do
    cp "$f" "${STAGING}/ebpf/" 2>/dev/null || true
done

# Configs
echo "  Staging configs..."
cp -r "${PROJECT_ROOT}/configs/" "${STAGING}/configs/" 2>/dev/null || true
cp "${PROJECT_ROOT}/docker-compose.yml" "${STAGING}/configs/" 2>/dev/null || true

# NixOS definitions
echo "  Staging NixOS definitions..."
cp -r "${PROJECT_ROOT}/nix/" "${STAGING}/nix/" 2>/dev/null || true

# Runbooks
echo "  Staging runbooks..."
cp -r "${PROJECT_ROOT}/runbooks/" "${STAGING}/runbooks/"

# Database migrations
echo "  Staging migrations..."
cp -r "${PROJECT_ROOT}/db/migrations/" "${STAGING}/db/" 2>/dev/null || true

# --- Phase 4: Generate Binding Rune (SHA256 manifest) ---
echo "=== Phase 4: Generate Binding Rune ==="
cd "${STAGING}"
find . -type f -not -name "BINDING_RUNE.sha256" | sort | xargs sha256sum > "${MANIFEST}"
TOTAL=$(wc -l < "${MANIFEST}")
RUNE_HASH=$(sha256sum "${MANIFEST}" | awk '{print $1}')
echo "  Files: ${TOTAL}"
echo "  Rune hash: ${RUNE_HASH}"

# --- Phase 5: Package ---
echo "=== Phase 5: Package ==="
mkdir -p "$(dirname "${OUTPUT}")"
tar czf "${OUTPUT}" -C "$(dirname "${STAGING}")" "$(basename "${STAGING}")"
SIZE=$(du -h "${OUTPUT}" | awk '{print $1}')
echo "  Output: ${OUTPUT} (${SIZE})"

# --- Phase 6: Cleanup ---
rm -rf "${STAGING}"

echo ""
echo "=============================================="
echo "SEALED CASK BUILT"
echo "=============================================="
echo "  Package:    ${OUTPUT}"
echo "  Size:       ${SIZE}"
echo "  Files:      ${TOTAL}"
echo "  Rune hash:  ${RUNE_HASH}"
echo "  Verify:     ./scripts/verify-binding-rune.sh ${OUTPUT}"
echo ""
