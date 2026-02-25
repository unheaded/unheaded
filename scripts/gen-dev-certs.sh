#!/usr/bin/env bash
# gen-dev-certs.sh — Generate development TLS certificates for the Unheaded service mesh.
#
# Usage:
#   ./scripts/gen-dev-certs.sh                     # Generate all service certs
#   ./scripts/gen-dev-certs.sh timeguru,captain     # Generate specific service certs
#   CERT_DIR=/tmp/certs ./scripts/gen-dev-certs.sh  # Custom output directory
#
# Requires: Go toolchain (builds cmd/cert-gen on the fly).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CERT_DIR="${CERT_DIR:-${PROJECT_DIR}/dev-certs}"
SERVICES="${1:-}"
TTL="${CERT_TTL:-24h}"

echo "=== Unheaded Dev Certificate Generator ==="
echo "  Project: ${PROJECT_DIR}"
echo "  Output:  ${CERT_DIR}"
echo "  TTL:     ${TTL}"
echo ""

# Build cert-gen tool
echo "Building cert-gen..."
go build -o "${PROJECT_DIR}/bin/cert-gen" "${PROJECT_DIR}/cmd/cert-gen"

# Run cert-gen
ARGS="-out ${CERT_DIR} -ttl ${TTL}"
if [ -n "${SERVICES}" ]; then
    ARGS="${ARGS} -services ${SERVICES}"
fi

echo "Generating certificates..."
"${PROJECT_DIR}/bin/cert-gen" ${ARGS}

echo ""
echo "=== Done ==="
echo ""
echo "To enable mTLS in services, set these environment variables:"
echo "  export UNHEADED_TLS_ENABLED=true"
echo "  export UNHEADED_TLS_CA_CERT=${CERT_DIR}/ca.pem"
echo "  export UNHEADED_TLS_CERT=${CERT_DIR}/<service>.pem"
echo "  export UNHEADED_TLS_KEY_FILE=${CERT_DIR}/<service>-key.pem"
